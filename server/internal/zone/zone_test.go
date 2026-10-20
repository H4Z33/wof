package zone

import (
	"io"
	"log/slog"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/model"
	"convergence/server/internal/rules"
)

type fakeConn struct {
	msgs   []*pb.ServerMessage
	closed string
}

func (c *fakeConn) Send(b []byte) bool {
	var m pb.ServerMessage
	if err := proto.Unmarshal(b, &m); err != nil {
		panic(err)
	}
	c.msgs = append(c.msgs, &m)
	return true
}

func (c *fakeConn) Close(reason string) { c.closed = reason }

func (c *fakeConn) lastSnapshot() *pb.Snapshot {
	for i := len(c.msgs) - 1; i >= 0; i-- {
		if s := c.msgs[i].GetSnapshot(); s != nil {
			return s
		}
	}
	return nil
}

// events returns every combat event this client received.
func (c *fakeConn) events() []*pb.CombatEvent {
	var out []*pb.CombatEvent
	for _, m := range c.msgs {
		if s := m.GetSnapshot(); s != nil {
			out = append(out, s.Events...)
		}
	}
	return out
}

func (c *fakeConn) rejections() []string {
	var out []string
	for _, m := range c.msgs {
		if r := m.GetSkillRejected(); r != nil {
			out = append(out, r.Reason)
		}
	}
	return out
}

type fakeSaver struct{ saved []model.Character }

func (s *fakeSaver) Save(c model.Character) { s.saved = append(s.saved, c) }

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func testConfig() Config {
	cfg := DefaultConfig()
	cfg.SpawnRadius = 0 // deterministic positions
	return cfg
}

func newTestZone(t *testing.T) *Zone {
	t.Helper()
	z, err := New(testConfig(), nil, nil, nil, quiet)
	if err != nil {
		t.Fatal(err)
	}
	return z
}

func loadContent(t *testing.T) *content.Content {
	t.Helper()
	c, err := content.Load("../../../content")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// newCombatZone has one bandit at (x, 0).
func newCombatZone(t *testing.T, x float64) (*Zone, *npc) {
	t.Helper()
	zc := &content.Zone{ID: "test", HalfExtent: 60, NPCGroups: []content.NPCGroup{
		{NPC: "mountain_bandit", Count: 1, Center: [2]float64{x, 0}},
	}}
	z, err := New(testConfig(), loadContent(t), zc, nil, quiet)
	if err != nil {
		t.Fatal(err)
	}
	return z, z.npcs[0]
}

var nextCharID int64

func joinTest(t *testing.T, z *Zone, name string) (*player, *fakeConn) {
	t.Helper()
	nextCharID++
	return joinChar(t, z, model.Character{ID: nextCharID, Name: name, Model: "mannequin_m", ZoneID: z.cfg.ID, Level: 1, HP: rules.MaxHP(1)})
}

func joinChar(t *testing.T, z *Zone, c model.Character) (*player, *fakeConn) {
	t.Helper()
	conn := &fakeConn{}
	h, err := z.join(c, conn)
	if err != nil {
		t.Fatal(err)
	}
	return z.players[h.id], conn
}

func move(p *player, seq uint32, dx, dz float32) {
	p.pending = append(p.pending, &pb.MoveInput{Seq: seq, DirX: dx, DirZ: dz})
}

func steps(z *Zone, n int) {
	for range n {
		z.step()
	}
}

func teleport(z *Zone, e *entity, x, zz float64) {
	e.pos = [3]float64{x, 0, zz}
	z.grid.moved(e)
}

// --- Connection and replication ---------------------------------------------

func TestWelcomeThenSpawnsBothWays(t *testing.T) {
	z := newTestZone(t)
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
	z := newTestZone(t)
	_, ca := joinTest(t, z, "alice")
	b, _ := joinTest(t, z, "bob")
	z.step()
	z.handleCmd(leaveReq{id: b.e.id})
	z.step()
	if d := ca.lastSnapshot().Despawns; len(d) != 1 || d[0] != b.e.id {
		t.Errorf("despawns = %v, want [%d]", d, b.e.id)
	}
}

func TestInterestManagement(t *testing.T) {
	z := newTestZone(t)
	a, ca := joinTest(t, z, "alice")
	b, _ := joinTest(t, z, "bob")
	teleport(z, b.e, 55, 55) // more than one cell away
	z.step()
	if a.known[b.e.id] {
		t.Fatal("alice sees bob from across the map")
	}
	teleport(z, b.e, 10, 10)
	z.step()
	spawned := false
	for _, s := range ca.lastSnapshot().Spawns {
		spawned = spawned || s.Id == b.e.id
	}
	if !spawned {
		t.Fatal("bob not spawned for alice after coming close")
	}
	teleport(z, b.e, -55, -55)
	z.step()
	if d := ca.lastSnapshot().Despawns; len(d) != 1 || d[0] != b.e.id {
		t.Errorf("despawns = %v, want bob", d)
	}
}

// --- Movement ----------------------------------------------------------------

func TestMovementSpeedAndAck(t *testing.T) {
	z := newTestZone(t)
	p, c := joinTest(t, z, "alice")
	for i := uint32(1); i <= 20; i++ { // one second of input, one per tick
		move(p, i, 1, 0)
		z.step()
	}
	if want := z.cfg.MoveSpeed; math.Abs(p.e.pos[0]-want) > 1e-9 {
		t.Errorf("x = %v after 1s, want %v", p.e.pos[0], want)
	}
	if ack := c.lastSnapshot().AckInputSeq; ack != 20 {
		t.Errorf("ack = %d, want 20", ack)
	}
}

func TestSpeedHackIsCapped(t *testing.T) {
	z := newTestZone(t)
	p, _ := joinTest(t, z, "cheater")
	seq := uint32(0)
	for range 20 { // one second, but 5 inputs per tick
		for range 5 {
			seq++
			move(p, seq, 1, 0)
		}
		z.step()
	}
	maxDist := z.cfg.MoveSpeed * float64(20+z.cfg.MaxInputCredit) / float64(z.cfg.TickRate)
	if p.e.pos[0] > maxDist+1e-9 {
		t.Errorf("moved %v in 1s, max allowed %v", p.e.pos[0], maxDist)
	}
}

func TestOversizedAndBadInput(t *testing.T) {
	z := newTestZone(t)
	p, _ := joinTest(t, z, "alice")
	move(p, 1, 100, 0) // direction longer than 1 is normalized
	z.step()
	if want := z.cfg.MoveSpeed / float64(z.cfg.TickRate); math.Abs(p.e.pos[0]-want) > 1e-6 {
		t.Errorf("x = %v, want %v", p.e.pos[0], want)
	}
	move(p, 1, 1, 0) // replayed seq is ignored
	move(p, 2, float32(math.NaN()), 0)
	z.step()
	if math.IsNaN(p.e.pos[0]) || p.lastSeq != 2 {
		t.Errorf("pos=%v lastSeq=%d", p.e.pos, p.lastSeq)
	}
}

func TestBoundsClamp(t *testing.T) {
	z := newTestZone(t)
	p, _ := joinTest(t, z, "alice")
	for i := uint32(1); i <= 1000; i++ {
		move(p, i, 0, -1)
		z.step()
	}
	if p.e.pos[2] != -z.cfg.HalfExtent {
		t.Errorf("z = %v, want %v", p.e.pos[2], -z.cfg.HalfExtent)
	}
}

func TestSpawnWithinRadius(t *testing.T) {
	z, err := New(DefaultConfig(), nil, nil, nil, quiet)
	if err != nil {
		t.Fatal(err)
	}
	for range 100 {
		p := z.spawnPoint()
		if d := hdist(p, z.cfg.Spawn); d > z.cfg.SpawnRadius {
			t.Fatalf("spawn %v is %v from center, radius %v", p, d, z.cfg.SpawnRadius)
		}
	}
}

// --- Combat ------------------------------------------------------------------

func strike(z *Zone, p *player, target uint32) {
	z.useSkill(p, &pb.UseSkill{SkillId: 101, TargetId: target}) // Jade Slash
}

func TestStrikeKillsNPCAndGrantsExp(t *testing.T) {
	z, n := newCombatZone(t, 2)
	p, c := joinTest(t, z, "alice")
	id := n.e.id
	for range 200 {
		if !n.e.alive() {
			break
		}
		strike(z, p, id)
		z.step()
	}
	if n.e.alive() {
		t.Fatal("bandit still alive after 10 s of strikes")
	}
	deaths := 0
	for _, ev := range c.events() {
		if ev.Kind == pb.CombatEventKind_COMBAT_EVENT_KIND_DEATH && ev.TargetId == id {
			deaths++
		}
	}
	if deaths != 1 {
		t.Errorf("got %d death events for the bandit, want 1", deaths)
	}
	if p.exp != int64(n.def.Exp) {
		t.Errorf("exp = %d, want %d", p.exp, n.def.Exp)
	}
	// The corpse disappears, then a new bandit spawns under a new id.
	steps(z, int(z.ticks(corpseMS))+1)
	if _, ok := z.entities[id]; ok {
		t.Error("corpse still in the world")
	}
	steps(z, int(z.ticks(n.def.RespawnMS)))
	if !n.e.alive() || n.e.id == id {
		t.Errorf("bandit not respawned (alive=%v id=%d old=%d)", n.e.alive(), n.e.id, id)
	}
}

func TestSkillRejections(t *testing.T) {
	z, n := newCombatZone(t, 2)
	p, c := joinTest(t, z, "alice")
	other, _ := joinTest(t, z, "bob")

	strike(z, p, n.e.id)
	strike(z, p, n.e.id) // still on cooldown
	z.useSkill(p, &pb.UseSkill{SkillId: 99, TargetId: n.e.id})
	strike(z, p, other.e.id) // no PvP
	teleport(z, n.e, 30, 0)
	steps(z, 40) // let the cooldown pass
	strike(z, p, n.e.id)

	want := []string{"cooldown", "unknown_skill", "invalid_target", "out_of_range"}
	got := c.rejections()
	if len(got) != len(want) {
		t.Fatalf("rejections = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rejection %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLevelUp(t *testing.T) {
	z := newTestZone(t)
	p, _ := joinTest(t, z, "alice")
	z.grantExp(p, rules.ExpToNext(1)+10)
	if p.e.level != 2 || p.exp != 10 || p.e.maxHP != rules.MaxHP(2) || p.e.hp != p.e.maxHP {
		t.Errorf("level=%d exp=%d hp=%d/%d", p.e.level, p.exp, p.e.hp, p.e.maxHP)
	}
}

// --- NPC AI (runs the real Lua script) ----------------------------------------

func TestNPCAggroAttackAndLeash(t *testing.T) {
	z, n := newCombatZone(t, 6) // inside the 8 m aggro radius
	p, _ := joinTest(t, z, "alice")
	steps(z, 100) // 5 s
	if n.target != p.e.id {
		t.Fatalf("bandit target = %d, want alice (%d)", n.target, p.e.id)
	}
	if p.e.hp == p.e.maxHP {
		t.Fatal("bandit never hit alice")
	}

	// Alice runs far away: the bandit chases until the leash (25 m from
	// home), gives up, walks back and heals.
	n.e.hp = n.e.maxHP / 2
	teleport(z, p.e, 55, 55)
	steps(z, 400)
	if n.target != 0 {
		t.Error("bandit still targeting after leash")
	}
	// Once home and healed it may wander again, but only within its radius.
	if d := math.Hypot(n.e.pos[0]-n.home[0], n.e.pos[2]-n.home[1]); d > n.def.Stats["wander_radius"]+0.5 {
		t.Errorf("bandit is %.1f m from home", d)
	}
	if n.e.hp != n.e.maxHP {
		t.Errorf("bandit hp %d/%d, want healed on reset", n.e.hp, n.e.maxHP)
	}
}

func TestPlayerDeathAndRespawn(t *testing.T) {
	z, _ := newCombatZone(t, 2)
	p, c := joinTest(t, z, "alice")
	p.e.hp = 1
	steps(z, 60) // the bandit attacks within 3 s
	if p.e.alive() {
		t.Fatal("alice survived")
	}
	if c.lastSnapshot().Status.RespawnInMs == 0 {
		t.Error("status has no respawn timer while dead")
	}
	before := p.e.pos
	move(p, 1, 1, 0)
	z.step()
	if p.e.pos != before || p.lastSeq != 1 {
		t.Error("dead player moved or input not acknowledged")
	}
	for i := 0; p.deadUntil != 0 && i < 1000; i++ {
		z.step()
	}
	// Checked on the respawn tick: the bandit next to spawn attacks again soon after.
	if !p.e.alive() || p.e.hp != p.e.maxHP {
		t.Errorf("not respawned: hp=%d/%d", p.e.hp, p.e.maxHP)
	}
}

// --- Sessions and persistence -------------------------------------------------

func TestDuplicateLoginKicksOldSession(t *testing.T) {
	z := newTestZone(t)
	c := model.Character{ID: 777, Name: "alice", Model: "mannequin_m", ZoneID: z.cfg.ID, Level: 3, HP: 50}
	first, firstConn := joinChar(t, z, c)
	teleport(z, first.e, 7, 7)
	second, _ := joinChar(t, z, c)
	if firstConn.closed == "" {
		t.Error("old connection not closed")
	}
	if len(z.players) != 1 {
		t.Errorf("%d players online, want 1", len(z.players))
	}
	if second.e.pos != first.e.pos || second.e.level != 3 {
		t.Errorf("new session did not continue old state: pos=%v level=%d", second.e.pos, second.e.level)
	}
}

func TestSaveOnLeaveAndAutosave(t *testing.T) {
	s := &fakeSaver{}
	z, err := New(testConfig(), nil, nil, s, quiet)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := joinTest(t, z, "alice")
	move(p, 1, 1, 0)
	steps(z, int(z.ticks(int(z.cfg.SaveInterval.Milliseconds())))+1)
	if len(s.saved) != 1 {
		t.Fatalf("%d autosaves, want 1", len(s.saved))
	}
	z.handleCmd(leaveReq{id: p.e.id})
	if len(s.saved) != 2 || s.saved[1].Pos != p.e.pos || s.saved[1].ID != p.charID {
		t.Errorf("leave save = %+v", s.saved)
	}
}

func TestDeadCharacterLoadsAtSpawnWithFullHP(t *testing.T) {
	z := newTestZone(t)
	p, _ := joinChar(t, z, model.Character{ID: 5, Name: "ghost", ZoneID: z.cfg.ID, Pos: [3]float64{30, 0, 30}, Level: 1, HP: 0})
	if p.e.hp != p.e.maxHP || p.e.pos != z.cfg.Spawn {
		t.Errorf("hp=%d pos=%v", p.e.hp, p.e.pos)
	}
}
