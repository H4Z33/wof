package zone

import (
	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/script"
)

// entity is anything with a position that clients can see.
type entity struct {
	id     uint32
	kind   pb.EntityKind
	name   string
	model  string
	level  int
	pos    [3]float64
	yaw    float64
	moving bool
	hp     int
	maxHP  int
	// inWorld is false once removed from the grid (left, or corpse cleared).
	inWorld bool
	cell    cellKey

	lastDamagedTick uint32

	player *player // set for players
	npc    *npc    // set for NPCs
}

func (e *entity) alive() bool { return e.inWorld && e.hp > 0 }

type player struct {
	e         *entity
	charID    int64
	accountID int64
	conn      Conn

	pending   []*pb.MoveInput
	credit    int
	lastSeq   uint32
	idleTicks int

	exp       int64
	cooldowns map[uint32]uint32 // skill id -> first tick it can be used again
	deadUntil uint32            // respawn tick while dead, else 0

	dirty    bool
	lastSave uint32

	// Entities this client has been told about (spawned and not despawned).
	known map[uint32]bool
	// Scratch set rebuilt every tick by replicate.
	visible map[uint32]*entity
}

type npc struct {
	z     *Zone
	e     *entity
	def   *content.NPC
	brain *script.Brain
	// Spawn area this NPC respawns in.
	groupCenter [2]float64
	groupRadius float64
	home        [2]float64

	target     uint32
	chaseID    uint32
	chaseStop  float64
	goal       [2]float64
	hasGoal    bool
	nextAttack uint32

	corpseUntil uint32 // dead: tick when the body disappears
	respawnAt   uint32 // dead: tick when it comes back
}
