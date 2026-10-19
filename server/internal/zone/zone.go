// Package zone runs one map's simulation. All state of a zone is owned by a
// single goroutine (Run); other goroutines talk to it through channels, so the
// game logic needs no locks.
package zone

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/auth"
)

// Conn is the zone's view of a client connection. Send must not block.
type Conn interface {
	Send(msg []byte) bool
	Close(reason string)
}

type Config struct {
	ID       string
	TickRate int
	// MoveSpeed in meters per second.
	MoveSpeed float64
	// The walkable area is the square [-HalfExtent, HalfExtent] on X and Z.
	HalfExtent float64
	Spawn      [3]float64
	// Players appear at a random point within this radius of Spawn so they
	// do not stack on top of each other.
	SpawnRadius float64
	// Input credit: each tick earns one MoveInput; unused credit accumulates
	// up to this cap so a client can catch up after network jitter, but can
	// never move faster than one input per tick on average.
	MaxInputCredit int
	MaxPlayers     int
}

func DefaultConfig() Config {
	return Config{
		ID:             "jade_terraces",
		TickRate:       20,
		MoveSpeed:      5,
		HalfExtent:     60,
		SpawnRadius:    3,
		MaxInputCredit: 5,
		MaxPlayers:     500,
	}
}

var (
	ErrZoneFull    = errors.New("zone full")
	ErrZoneStopped = errors.New("zone stopped")
)

type player struct {
	id     uint32
	name   string
	model  string
	conn   Conn
	pos    [3]float64
	yaw    float64
	moving bool

	pending   []*pb.MoveInput
	credit    int
	lastSeq   uint32
	idleTicks int
	// Entities this client has been told about (spawned and not despawned).
	known map[uint32]bool
}

type joinReq struct {
	id    auth.Identity
	conn  Conn
	reply chan joinResult
}

type joinResult struct {
	handle *Handle
	err    error
}

type leaveReq struct{ id uint32 }

type inputMsg struct {
	id uint32
	in *pb.MoveInput
}

type Zone struct {
	cfg    Config
	log    *slog.Logger
	inbox  chan any
	inputs chan inputMsg
	done   chan struct{} // closed when Run returns
	tick   atomic.Uint32

	// Owned by the Run goroutine.
	players map[uint32]*player
	nextID  uint32
}

func New(cfg Config, log *slog.Logger) *Zone {
	return &Zone{
		cfg:     cfg,
		log:     log.With("zone", cfg.ID),
		inbox:   make(chan any, 64),
		inputs:  make(chan inputMsg, 4096),
		done:    make(chan struct{}),
		players: make(map[uint32]*player),
	}
}

func (z *Zone) Config() Config { return z.cfg }

// Tick is the current server tick. Safe from any goroutine.
func (z *Zone) Tick() uint32 { return z.tick.Load() }

// Join adds a player and returns the handle the gateway uses to forward input.
func (z *Zone) Join(ctx context.Context, id auth.Identity, c Conn) (*Handle, error) {
	req := joinReq{id: id, conn: c, reply: make(chan joinResult, 1)}
	select {
	case z.inbox <- req:
	case <-z.done:
		return nil, ErrZoneStopped
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case res := <-req.reply:
		return res.handle, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Handle is a joined player's connection to the zone.
type Handle struct {
	z  *Zone
	id uint32
}

func (h *Handle) EntityID() uint32 { return h.id }

// Input queues a MoveInput. It never blocks; input is dropped if the zone is
// overloaded, and the client's prediction gets corrected.
func (h *Handle) Input(in *pb.MoveInput) {
	select {
	case h.z.inputs <- inputMsg{id: h.id, in: in}:
	default:
	}
}

// Leave removes the player. Safe to call more than once.
func (h *Handle) Leave() {
	select {
	case h.z.inbox <- leaveReq{id: h.id}:
	case <-h.z.done:
	}
}

func (z *Zone) Run(ctx context.Context) {
	dt := time.Second / time.Duration(z.cfg.TickRate)
	ticker := time.NewTicker(dt)
	defer ticker.Stop()
	defer close(z.done)
	z.log.Info("zone started", "tick_rate", z.cfg.TickRate)
	for {
		select {
		case <-ctx.Done():
			for _, p := range z.players {
				p.conn.Close("server shutting down")
			}
			z.log.Info("zone stopped")
			return
		case cmd := <-z.inbox:
			z.handleCmd(cmd)
		case m := <-z.inputs:
			z.queueInput(m)
		case <-ticker.C:
			z.step()
		}
	}
}

func (z *Zone) handleCmd(cmd any) {
	switch c := cmd.(type) {
	case joinReq:
		h, err := z.join(c.id, c.conn)
		c.reply <- joinResult{handle: h, err: err}
	case leaveReq:
		if p, ok := z.players[c.id]; ok {
			delete(z.players, c.id)
			z.log.Info("player left", "entity", c.id, "name", p.name, "players", len(z.players))
		}
	}
}

func (z *Zone) join(id auth.Identity, c Conn) (*Handle, error) {
	if len(z.players) >= z.cfg.MaxPlayers {
		return nil, ErrZoneFull
	}
	z.nextID++
	p := &player{
		id:     z.nextID,
		name:   id.Name,
		model:  id.Model,
		conn:   c,
		pos:    z.spawnPoint(),
		credit: z.cfg.MaxInputCredit,
		known:  make(map[uint32]bool),
	}
	z.players[p.id] = p
	z.send(p, &pb.ServerMessage{Payload: &pb.ServerMessage_Welcome{Welcome: &pb.Welcome{
		EntityId:   p.id,
		ZoneId:     z.cfg.ID,
		TickRate:   uint32(z.cfg.TickRate),
		ServerTick: z.tick.Load(),
		MoveSpeed:  float32(z.cfg.MoveSpeed),
		Spawn:      vec(p.pos),
		HalfExtent: float32(z.cfg.HalfExtent),
	}}})
	z.log.Info("player joined", "entity", p.id, "name", p.name, "model", p.model, "players", len(z.players))
	return &Handle{z: z, id: p.id}, nil
}

const maxPending = 32

func (z *Zone) queueInput(m inputMsg) {
	p, ok := z.players[m.id]
	if !ok || len(p.pending) >= maxPending {
		return
	}
	p.pending = append(p.pending, m.in)
}

func (z *Zone) step() {
	tick := z.tick.Add(1)
	dt := 1 / float64(z.cfg.TickRate)
	for _, p := range z.players {
		z.applyInputs(p, dt)
	}
	z.replicate(tick)
}

func (z *Zone) applyInputs(p *player, dt float64) {
	p.credit = min(p.credit+1, z.cfg.MaxInputCredit)
	applied := false
	for len(p.pending) > 0 && p.credit > 0 {
		in := p.pending[0]
		p.pending = p.pending[1:]
		if in.Seq <= p.lastSeq {
			continue // duplicate or out of order
		}
		p.credit--
		p.lastSeq = in.Seq
		applied = true

		dx, dz := float64(in.DirX), float64(in.DirZ)
		if l := math.Hypot(dx, dz); l > 1 {
			dx, dz = dx/l, dz/l
		} else if math.IsNaN(l) {
			dx, dz = 0, 0
		}
		step := z.cfg.MoveSpeed * dt
		lim := z.cfg.HalfExtent
		p.pos[0] = clamp(p.pos[0]+dx*step, -lim, lim)
		p.pos[2] = clamp(p.pos[2]+dz*step, -lim, lim)
		if y := float64(in.Yaw); !math.IsNaN(y) && !math.IsInf(y, 0) {
			p.yaw = math.Remainder(y, 2*math.Pi)
		}
		p.moving = dx != 0 || dz != 0
	}
	if applied {
		p.idleTicks = 0
	} else if p.idleTicks++; p.idleTicks > 3 {
		// Short grace period so a late packet does not flicker the animation.
		p.moving = false
	}
}

// replicate sends each player its own Snapshot. M0 replicates the whole zone;
// interest management (spatial grid) arrives in M1.
func (z *Zone) replicate(tick uint32) {
	states := make(map[uint32]*pb.EntityState, len(z.players))
	for id, p := range z.players {
		states[id] = &pb.EntityState{Id: id, Pos: vec(p.pos), Yaw: float32(p.yaw), Moving: p.moving}
	}
	for _, p := range z.players {
		snap := &pb.Snapshot{Tick: tick, AckInputSeq: p.lastSeq}
		for id, st := range states {
			if p.known[id] {
				snap.Entities = append(snap.Entities, st)
				continue
			}
			o := z.players[id]
			snap.Spawns = append(snap.Spawns, &pb.EntitySpawn{Id: id, Name: o.name, Model: o.model, State: st})
			p.known[id] = true
		}
		for id := range p.known {
			if _, ok := z.players[id]; !ok {
				snap.Despawns = append(snap.Despawns, id)
				delete(p.known, id)
			}
		}
		z.send(p, &pb.ServerMessage{Payload: &pb.ServerMessage_Snapshot{Snapshot: snap}})
	}
}

func (z *Zone) send(p *player, msg *pb.ServerMessage) {
	b, err := proto.Marshal(msg)
	if err != nil {
		z.log.Error("marshal", "err", err)
		return
	}
	p.conn.Send(b)
}

func (z *Zone) spawnPoint() [3]float64 {
	p := z.cfg.Spawn
	a := rand.Float64() * 2 * math.Pi
	r := z.cfg.SpawnRadius * math.Sqrt(rand.Float64())
	p[0] += r * math.Cos(a)
	p[2] += r * math.Sin(a)
	return p
}

func vec(v [3]float64) *pb.Vec3 {
	return &pb.Vec3{X: float32(v[0]), Y: float32(v[1]), Z: float32(v[2])}
}

func clamp(v, lo, hi float64) float64 { return max(lo, min(hi, v)) }
