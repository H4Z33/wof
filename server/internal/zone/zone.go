// Package zone runs one map's simulation. All state of a zone is owned by a
// single goroutine (Run); other goroutines talk to it through channels, so the
// game logic needs no locks.
package zone

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"math/rand/v2"
	"path/filepath"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/fate"
	"convergence/server/internal/model"
	"convergence/server/internal/nav"
	"convergence/server/internal/rules"
	"convergence/server/internal/script"
	"convergence/server/internal/terrain"
)

// Conn is the zone's view of a client connection. Send must not block.
type Conn interface {
	Send(msg []byte) bool
	Close(reason string)
}

// Saver persists characters without blocking the zone.
type Saver interface {
	Save(model.Character)
}

// FateDirector deals fate decks in the background (see package fate).
type FateDirector interface {
	Deal(req fate.DeckRequest, reply func(fate.Deck)) bool
}

type Config struct {
	ID       string
	TickRate int
	// MoveSpeed in meters per second (players).
	MoveSpeed float64
	// The walkable area is the square [-HalfExtent, HalfExtent] on X and Z.
	HalfExtent float64
	Spawn      [3]float64
	// Players appear within this radius of Spawn so they do not stack.
	SpawnRadius float64
	// Input credit: each tick earns one MoveInput; unused credit accumulates
	// up to this cap so a client can catch up after network jitter, but can
	// never move faster than one input per tick on average.
	MaxInputCredit int
	MaxPlayers     int
	// Interest grid cell size; players see entities up to 1-2 cells away.
	CellSize float64
	// Terrain is the zone's heightmap; nil means flat ground at y=0.
	Terrain     *terrain.Terrain
	TerrainFile string
	// Dirty characters are saved at least this often.
	SaveInterval time.Duration
	// How long a dead player waits before respawning at Spawn.
	RespawnDelay time.Duration
	// Out of combat for this long, players regenerate health.
	RegenDelay time.Duration
	// NPC AI runs every ThinkEvery ticks, within LuaBudget per tick.
	ThinkEvery int
	LuaBudget  time.Duration
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
		CellSize:       24,
		SaveInterval:   30 * time.Second,
		RespawnDelay:   5 * time.Second,
		RegenDelay:     8 * time.Second,
		ThinkEvery:     4,
		LuaBudget:      5 * time.Millisecond,
	}
}

// ConfigFor applies a zone's content on top of the defaults.
func ConfigFor(z *content.Zone) Config {
	cfg := DefaultConfig()
	cfg.ID = z.ID
	cfg.HalfExtent = z.HalfExtent
	cfg.Spawn = [3]float64{z.Spawn[0], 0, z.Spawn[1]}
	cfg.SpawnRadius = z.SpawnRadius
	return cfg
}

// LoadTerrain loads the zone's heightmap from the content directory, if it
// has one.
func LoadTerrain(cfg *Config, z *content.Zone, contentDir string) error {
	if z.Terrain == nil {
		return nil
	}
	t, err := terrain.Load(filepath.Join(contentDir, "zones", z.Terrain.File), terrain.Rules{
		MaxSlope:   degToSlope(z.Terrain.MaxSlopeDeg),
		WaterLevel: z.Terrain.WaterLevel,
		WadeDepth:  z.Terrain.WadeDepth,
	})
	if err != nil {
		return fmt.Errorf("zone %s terrain: %w (run cmd/terraingen)", z.ID, err)
	}
	cfg.Terrain, cfg.TerrainFile = t, z.Terrain.File
	cfg.Spawn[1] = t.Height(cfg.Spawn[0], cfg.Spawn[2])
	return nil
}

var (
	ErrZoneFull    = errors.New("zone full")
	ErrZoneStopped = errors.New("zone stopped")
)

type joinReq struct {
	char  model.Character
	conn  Conn
	reply chan joinResult
}

type joinResult struct {
	handle *Handle
	err    error
}

type leaveReq struct{ id uint32 }

type inputMsg struct {
	id    uint32
	move  *pb.MoveInput
	skill *pb.UseSkill
}

type Zone struct {
	cfg     Config
	content *content.Content
	saver   Saver
	log     *slog.Logger
	inbox   chan any
	inputs  chan inputMsg
	done    chan struct{} // closed when Run returns
	tick    atomic.Uint32

	// Owned by the Run goroutine.
	entities map[uint32]*entity
	players  map[uint32]*player // by entity id
	byChar   map[int64]uint32   // character id -> entity id
	npcs     []*npc
	grid     *grid
	lua      *script.Engine
	events   []*pb.CombatEvent
	nextID   uint32
	fate     FateDirector
	places   []place
	aiLoaded map[string]bool
	caches   []*entity
	nav      *nav.Grid // nil on flat zones
	// Mixed into every player's fate entropy so rolls cannot be predicted
	// from inputs alone.
	fateSecret uint64
}

// place is a named area used to describe where a player is.
type place struct {
	center [2]float64
	radius float64
	name   string
}

// New builds a zone and spawns its NPCs. zc may be nil (no NPCs); saver may
// be nil (nothing is persisted).
func New(cfg Config, c *content.Content, zc *content.Zone, saver Saver, log *slog.Logger) (*Zone, error) {
	if c == nil {
		c = &content.Content{}
	}
	z := &Zone{
		cfg:      cfg,
		content:  c,
		saver:    saver,
		log:      log.With("zone", cfg.ID),
		inbox:    make(chan any, 64),
		inputs:   make(chan inputMsg, 4096),
		done:     make(chan struct{}),
		entities: make(map[uint32]*entity),
		players:  make(map[uint32]*player),
		byChar:   make(map[int64]uint32),
		grid:     newGrid(cfg.CellSize),
		lua:      script.New(log),
		aiLoaded: make(map[string]bool),
	}
	if cfg.Terrain != nil {
		z.nav = nav.Build(cfg.Terrain, cfg.HalfExtent, 1)
	}
	var seed [8]byte
	crand.Read(seed[:])
	z.fateSecret = binary.LittleEndian.Uint64(seed[:])
	if err := z.loadSkillScripts(); err != nil {
		z.lua.Close()
		return nil, err
	}
	if zc != nil {
		if err := z.spawnGroups(zc); err != nil {
			z.lua.Close()
			return nil, err
		}
	}
	return z, nil
}

func (z *Zone) Config() Config { return z.cfg }

// SetFate connects the Fate agent. Call before Run. Without it, fate events
// are resolved immediately with fallback choices.
func (z *Zone) SetFate(d FateDirector) { z.fate = d }

// Tick is the current server tick. Safe from any goroutine.
func (z *Zone) Tick() uint32 { return z.tick.Load() }

// Done is closed once Run has returned and every player has been saved.
func (z *Zone) Done() <-chan struct{} { return z.done }

// Join adds a character and returns the handle the gateway uses to forward
// input. A character already online is disconnected first.
func (z *Zone) Join(ctx context.Context, c model.Character, conn Conn) (*Handle, error) {
	req := joinReq{char: c, conn: conn, reply: make(chan joinResult, 1)}
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
	case h.z.inputs <- inputMsg{id: h.id, move: in}:
	default:
	}
}

// UseSkill queues a skill request. Never blocks.
func (h *Handle) UseSkill(req *pb.UseSkill) {
	select {
	case h.z.inputs <- inputMsg{id: h.id, skill: req}:
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
	defer z.lua.Close()
	z.log.Info("zone started", "tick_rate", z.cfg.TickRate, "npcs", len(z.npcs))
	for {
		select {
		case <-ctx.Done():
			for _, p := range z.players {
				z.save(p)
				p.conn.Close("server shutting down")
			}
			z.log.Info("zone stopped")
			return
		case cmd := <-z.inbox:
			z.handleCmd(cmd)
		case m := <-z.inputs:
			z.handleInput(m)
		case <-ticker.C:
			z.step()
		}
	}
}

func (z *Zone) handleCmd(cmd any) {
	switch c := cmd.(type) {
	case joinReq:
		h, err := z.join(c.char, c.conn)
		c.reply <- joinResult{handle: h, err: err}
	case fate.Deck:
		z.receiveDeck(c)
	case reloadReq:
		z.applyReload(c.c)
	case leaveReq:
		if p, ok := z.players[c.id]; ok {
			z.removePlayer(p)
			z.log.Info("player left", "entity", c.id, "name", p.e.name, "players", len(z.players))
		}
	}
}

func (z *Zone) newID() uint32 {
	z.nextID++
	return z.nextID
}

func (z *Zone) join(c model.Character, conn Conn) (*Handle, error) {
	if old, ok := z.byChar[c.ID]; ok {
		p := z.players[old]
		z.log.Info("character logged in again, dropping old session", "character", c.ID)
		p.conn.Close("logged in from another place")
		z.removePlayer(p)
		// The new session continues from the old one's latest state.
		c = z.record(p)
	}
	if len(z.players) >= z.cfg.MaxPlayers {
		return nil, ErrZoneFull
	}
	level := max(c.Level, 1)
	class := z.content.Class(c.Class)
	e := &entity{
		id:    z.newID(),
		kind:  pb.EntityKind_ENTITY_KIND_PLAYER,
		name:  c.Name,
		model: c.Model,
		level: level,
		pos:   c.Pos,
		yaw:   c.Yaw,
		hp:    c.HP,
		maxHP: rules.MaxHP(level),
	}
	if class != nil {
		e.maxHP = class.MaxHP(level)
	}
	if c.ZoneID != z.cfg.ID || !z.inBounds(e.pos) || !z.standable(e.pos) {
		e.pos = z.spawnPoint()
	}
	e.pos[1] = z.groundY(e.pos[0], e.pos[2]) // saved on older or flat terrain
	if e.hp <= 0 || e.hp > e.maxHP {
		// Logged out dead (or data from an older formula): start fresh at spawn.
		if e.hp <= 0 {
			e.pos = z.spawnPoint()
		}
		e.hp = e.maxHP
	}
	p := &player{
		e:         e,
		charID:    c.ID,
		accountID: c.AccountID,
		conn:      conn,
		credit:    z.cfg.MaxInputCredit,
		exp:       c.Exp,
		cooldowns: make(map[uint32]uint32),
		lastSave:  z.tick.Load(),
		known:     make(map[uint32]bool),
		visible:   make(map[uint32]*entity),
		luck:      c.Luck,
		karma:     c.Karma,
		inventory: maps.Clone(c.Inventory),
		class:     class,
	}
	if class != nil {
		p.resource = float64(class.Resource.Start)
	}
	if p.inventory == nil {
		p.inventory = make(map[string]int)
	}
	z.initFate(p)
	e.player = p
	z.entities[e.id] = e
	z.players[e.id] = p
	z.byChar[c.ID] = e.id
	z.grid.add(e)

	z.send(p, &pb.ServerMessage{Payload: &pb.ServerMessage_Welcome{Welcome: &pb.Welcome{
		EntityId:      e.id,
		ZoneId:        z.cfg.ID,
		TickRate:      uint32(z.cfg.TickRate),
		ServerTick:    z.tick.Load(),
		MoveSpeed:     float32(z.cfg.MoveSpeed),
		Spawn:         vec(e.pos),
		HalfExtent:    float32(z.cfg.HalfExtent),
		Terrain:       z.terrainInfo(),
		Skills:        z.skillInfos(p),
		ClassInfo:     classInfo(class),
		Status:        z.status(p),
		CharacterName: c.Name,
		Inventory:     z.inventoryList(p),
	}}})
	z.log.Info("player joined", "entity", e.id, "name", e.name, "character", c.ID, "players", len(z.players))
	return &Handle{z: z, id: e.id}, nil
}

func (z *Zone) removePlayer(p *player) {
	z.save(p)
	z.grid.remove(p.e)
	delete(z.entities, p.e.id)
	delete(z.players, p.e.id)
	if z.byChar[p.charID] == p.e.id {
		delete(z.byChar, p.charID)
	}
}

// record is the character as it should be stored.
func (z *Zone) record(p *player) model.Character {
	return model.Character{
		ID:        p.charID,
		AccountID: p.accountID,
		Name:      p.e.name,
		Model:     p.e.model,
		Class:     p.classKey(),
		ZoneID:    z.cfg.ID,
		Pos:       p.e.pos,
		Yaw:       p.e.yaw,
		Level:     p.e.level,
		Exp:       p.exp,
		HP:        p.e.hp,
		Luck:      p.luck,
		Karma:     p.karma,
		// Copied: the saver reads it on another goroutine.
		Inventory: maps.Clone(p.inventory),
	}
}

func (z *Zone) save(p *player) {
	p.dirty = false
	p.lastSave = z.tick.Load()
	if z.saver != nil {
		z.saver.Save(z.record(p))
	}
}

func (z *Zone) handleInput(m inputMsg) {
	p, ok := z.players[m.id]
	if !ok {
		return
	}
	switch {
	case m.move != nil:
		if len(p.pending) < maxPending {
			p.pending = append(p.pending, m.move)
		}
	case m.skill != nil:
		z.useSkill(p, m.skill)
	}
}

func (z *Zone) step() {
	tick := z.tick.Add(1)
	dt := 1 / float64(z.cfg.TickRate)
	for _, p := range z.players {
		z.applyInputs(p, dt)
	}
	z.thinkNPCs(tick)
	for _, n := range z.npcs {
		z.moveNPC(n, dt)
	}
	z.updateResources(tick)
	z.tickEffects(tick)
	z.updateLife(tick)
	z.updateFate(tick)
	z.replicate(tick)
	z.autosave(tick)
}

func (z *Zone) autosave(tick uint32) {
	every := z.ticks(int(z.cfg.SaveInterval / time.Millisecond))
	for _, p := range z.players {
		if p.dirty && tick-p.lastSave >= every {
			z.save(p)
		}
	}
}

// ticks converts milliseconds to whole ticks, rounding up.
func (z *Zone) ticks(ms int) uint32 {
	return uint32((ms*z.cfg.TickRate + 999) / 1000)
}

func (z *Zone) inBounds(p [3]float64) bool {
	return math.Abs(p[0]) <= z.cfg.HalfExtent && math.Abs(p[2]) <= z.cfg.HalfExtent
}

func (z *Zone) spawnPoint() [3]float64 {
	return z.pointNear(z.cfg.Spawn[0], z.cfg.Spawn[2], z.cfg.SpawnRadius)
}

func randomAround(c [3]float64, radius float64) [3]float64 {
	a := rand.Float64() * 2 * math.Pi
	r := radius * math.Sqrt(rand.Float64())
	return [3]float64{c[0] + r*math.Cos(a), c[1], c[2] + r*math.Sin(a)}
}

func (z *Zone) send(p *player, msg *pb.ServerMessage) {
	b, err := proto.Marshal(msg)
	if err != nil {
		z.log.Error("marshal", "err", err)
		return
	}
	p.conn.Send(b)
}

func vec(v [3]float64) *pb.Vec3 {
	return &pb.Vec3{X: float32(v[0]), Y: float32(v[1]), Z: float32(v[2])}
}

func clamp(v, lo, hi float64) float64 { return max(lo, min(hi, v)) }

func hdist(a, b [3]float64) float64 { return math.Hypot(a[0]-b[0], a[2]-b[2]) }

func yawToward(from, to [3]float64) float64 { return math.Atan2(to[0]-from[0], to[2]-from[2]) }

func describe(e *entity) string { return fmt.Sprintf("%s#%d", e.name, e.id) }
