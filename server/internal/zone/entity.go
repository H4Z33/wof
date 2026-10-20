package zone

import (
	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/fate"
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
	effects         []effect

	player *player // set for players
	npc    *npc    // set for NPCs
	cache  *cache  // set for hidden caches
}

// effect is a timed modifier, e.g. a Fate aura or an exposed weakness.
type effect struct {
	flag  pb.EffectFlag
	dealt float64 // multiplier on damage dealt (1 = none)
	taken float64 // multiplier on damage taken (1 = none)
	until uint32
}

func (e *entity) addEffect(ef effect) {
	for i := range e.effects {
		if e.effects[i].flag == ef.flag {
			e.effects[i] = ef // refresh, do not stack
			return
		}
	}
	e.effects = append(e.effects, ef)
}

// multipliers returns the combined damage modifiers active at tick.
func (e *entity) multipliers(tick uint32) (dealt, taken float64) {
	dealt, taken = 1, 1
	for _, ef := range e.effects {
		if tick < ef.until {
			dealt *= ef.dealt
			taken *= ef.taken
		}
	}
	return dealt, taken
}

func (e *entity) effectFlags(tick uint32) uint32 {
	var flags uint32
	for _, ef := range e.effects {
		if tick < ef.until {
			flags |= uint32(ef.flag)
		}
	}
	return flags
}

// cache is a hidden cache revealed by Fate; its owner claims it by walking
// up to it.
type cache struct {
	owner     int64 // character id
	itemKey   string
	magnitude int
	expireAt  uint32
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

	// Fate.
	luck        int
	karma       int
	inventory   map[string]int
	entropy     fate.Entropy // fed by this player's inputs; drives their rolls
	deck        []fate.Card  // cards the Fate agent prepared
	dealPending bool         // a deal is in flight
	lastDeal    uint32       // tick of the last deal request
	dealKey     string       // situation the deck was dealt for
	fateReady   uint32       // first tick another fate event may happen
	kills       int          // since the last fate event
	deaths      int          // since the last fate event
	fateHistory []string
	// Trigger bookkeeping.
	travelPos  [3]float64 // where the current travel stretch began
	stillTick  uint32     // tick the player last moved
	restArmed  bool       // a rest roll is due once still long enough
	woundArmed bool       // a wounded roll is due on falling below the threshold

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

	// Planned walk around terrain (see steer).
	path     [][2]float64
	pathGoal [2]float64
	pathTick uint32

	// Temporary NPCs (Fate ambushes) never respawn and vanish at expireAt.
	temporary bool
	expireAt  uint32
}
