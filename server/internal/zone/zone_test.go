package zone

import (
	"io"
	"log/slog"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/auth"
)

type fakeConn struct{ msgs []*pb.ServerMessage }

func (c *fakeConn) Send(b []byte) bool {
	var m pb.ServerMessage
	if err := proto.Unmarshal(b, &m); err != nil {
		panic(err)
	}
	c.msgs = append(c.msgs, &m)
	return true
}

func (c *fakeConn) Close(string) {}

func (c *fakeConn) lastSnapshot() *pb.Snapshot {
	for i := len(c.msgs) - 1; i >= 0; i-- {
		if s := c.msgs[i].GetSnapshot(); s != nil {
			return s
		}
	}
	return nil
}

func newTestZone() *Zone {
	cfg := DefaultConfig()
	cfg.SpawnRadius = 0 // deterministic positions
	return New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func joinTest(t *testing.T, z *Zone, name string) (*player, *fakeConn) {
	t.Helper()
	c := &fakeConn{}
	h, err := z.join(auth.Identity{Name: name, Model: "mannequin_m"}, c)
	if err != nil {
		t.Fatal(err)
	}
	return z.players[h.id], c
}

func move(p *player, seq uint32, dx, dz float32) {
	p.pending = append(p.pending, &pb.MoveInput{Seq: seq, DirX: dx, DirZ: dz})
}

func TestWelcomeThenSpawnsBothWays(t *testing.T) {
	z := newTestZone()
	_, ca := joinTest(t, z, "alice")
	_, cb := joinTest(t, z, "bob")
	if ca.msgs[0].GetWelcome() == nil {
		t.Fatal("first message must be Welcome")
	}
	z.step()
	for name, c := range map[string]*fakeConn{"alice": ca, "bob": cb} {
		if got := len(c.lastSnapshot().Spawns); got != 2 {
			t.Errorf("%s: %d spawns, want 2", name, got)
		}
	}
	z.step()
	if s := ca.lastSnapshot(); len(s.Spawns) != 0 || len(s.Entities) != 2 {
		t.Errorf("second snapshot: spawns=%d entities=%d, want 0 and 2", len(s.Spawns), len(s.Entities))
	}
}

func TestDespawnOnLeave(t *testing.T) {
	z := newTestZone()
	_, ca := joinTest(t, z, "alice")
	b, _ := joinTest(t, z, "bob")
	z.step()
	z.handleCmd(leaveReq{id: b.id})
	z.step()
	if d := ca.lastSnapshot().Despawns; len(d) != 1 || d[0] != b.id {
		t.Errorf("despawns = %v, want [%d]", d, b.id)
	}
}

func TestMovementSpeedAndAck(t *testing.T) {
	z := newTestZone()
	p, c := joinTest(t, z, "alice")
	for i := uint32(1); i <= 20; i++ { // one second of input, one per tick
		move(p, i, 1, 0)
		z.step()
	}
	if want := z.cfg.MoveSpeed; math.Abs(p.pos[0]-want) > 1e-9 {
		t.Errorf("x = %v after 1s, want %v", p.pos[0], want)
	}
	if ack := c.lastSnapshot().AckInputSeq; ack != 20 {
		t.Errorf("ack = %d, want 20", ack)
	}
}

func TestSpeedHackIsCapped(t *testing.T) {
	z := newTestZone()
	p, _ := joinTest(t, z, "cheater")
	seq := uint32(0)
	for range 20 { // one second, but 5 inputs per tick
		for range 5 {
			seq++
			move(p, seq, 1, 0)
		}
		z.step()
	}
	// Credit allows a burst of MaxInputCredit, then one input per tick.
	maxDist := z.cfg.MoveSpeed * float64(20+z.cfg.MaxInputCredit) / float64(z.cfg.TickRate)
	if p.pos[0] > maxDist+1e-9 {
		t.Errorf("moved %v in 1s, max allowed %v", p.pos[0], maxDist)
	}
}

func TestOversizedAndBadInput(t *testing.T) {
	z := newTestZone()
	p, _ := joinTest(t, z, "alice")
	move(p, 1, 100, 0) // direction longer than 1 is normalized
	z.step()
	if want := z.cfg.MoveSpeed / float64(z.cfg.TickRate); math.Abs(p.pos[0]-want) > 1e-6 {
		t.Errorf("x = %v, want %v", p.pos[0], want)
	}
	move(p, 1, 1, 0) // replayed seq is ignored
	move(p, 2, float32(math.NaN()), 0)
	z.step()
	if math.IsNaN(p.pos[0]) || p.lastSeq != 2 {
		t.Errorf("pos=%v lastSeq=%d", p.pos, p.lastSeq)
	}
}

func TestSpawnWithinRadius(t *testing.T) {
	z := New(DefaultConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	for range 100 {
		p := z.spawnPoint()
		if d := math.Hypot(p[0]-z.cfg.Spawn[0], p[2]-z.cfg.Spawn[2]); d > z.cfg.SpawnRadius {
			t.Fatalf("spawn %v is %v from center, radius %v", p, d, z.cfg.SpawnRadius)
		}
	}
}

func TestBoundsClamp(t *testing.T) {
	z := newTestZone()
	p, _ := joinTest(t, z, "alice")
	for i := uint32(1); i <= 1000; i++ {
		move(p, i, 0, -1)
		z.step()
	}
	if p.pos[2] != -z.cfg.HalfExtent {
		t.Errorf("z = %v, want %v", p.pos[2], -z.cfg.HalfExtent)
	}
}
