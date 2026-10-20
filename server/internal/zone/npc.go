package zone

import (
	"errors"
	"fmt"
	"math"
	"strings"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/script"
)

func (z *Zone) spawnGroups(zc *content.Zone) error {
	for _, g := range zc.NPCGroups {
		def, ok := z.content.NPCs[g.NPC]
		if !ok {
			return fmt.Errorf("zone %s: unknown npc %q", zc.ID, g.NPC)
		}
		z.places = append(z.places, place{center: g.Center, radius: g.Radius + 12, name: "near a " + strings.ToLower(def.Name) + " camp"})
		for range g.Count {
			n, err := z.newNPC(def)
			if err != nil {
				return err
			}
			n.groupCenter, n.groupRadius = g.Center, g.Radius
			z.respawnNPC(n)
			z.npcs = append(z.npcs, n)
		}
	}
	return nil
}

// newNPC creates an NPC with its AI brain, loading the script on first use.
func (z *Zone) newNPC(def *content.NPC) (*npc, error) {
	if !z.aiLoaded[def.AI] {
		if err := z.lua.Load(def.AI, def.AISource); err != nil {
			return nil, err
		}
		z.aiLoaded[def.AI] = true
	}
	n := &npc{z: z, def: def, e: &entity{}}
	brain, err := z.lua.NewBrain(def.AI, n)
	if err != nil {
		return nil, err
	}
	n.brain = brain
	return n, nil
}

// respawnNPC (re)creates the NPC's entity at a random point of its area,
// under a new entity id so clients never confuse it with the old body.
func (z *Zone) respawnNPC(n *npc) {
	z.placeNPC(n, randomAround([3]float64{n.groupCenter[0], 0, n.groupCenter[1]}, n.groupRadius))
}

// placeNPC puts a fresh body for n into the world at pos.
func (z *Zone) placeNPC(n *npc, pos [3]float64) {
	d := n.def
	n.e = &entity{
		id:    z.newID(),
		kind:  pb.EntityKind_ENTITY_KIND_NPC,
		name:  d.Name,
		model: d.Model,
		level: d.Level,
		pos:   pos,
		hp:    d.HP,
		maxHP: d.HP,
		npc:   n,
	}
	n.home = [2]float64{pos[0], pos[2]}
	n.target, n.chaseID, n.hasGoal, n.nextAttack = 0, 0, false, 0
	z.lua.ResetMemory(n.brain)
	z.entities[n.e.id] = n.e
	z.grid.add(n.e)
}

func (z *Zone) thinkNPCs(tick uint32) {
	if len(z.npcs) == 0 {
		return
	}
	z.lua.BeginTick(z.cfg.LuaBudget)
	defer z.lua.EndTick()
	every := uint32(max(z.cfg.ThinkEvery, 1))
	for _, n := range z.npcs {
		if !n.e.alive() || (tick+n.e.id)%every != 0 {
			continue
		}
		if err := z.lua.Think(n.brain); err != nil {
			if errors.Is(err, script.ErrBudget) {
				z.log.Warn("lua budget exceeded; skipping remaining NPCs this tick")
				return
			}
			if n.brain.Fails == 1 || n.brain.Fails%100 == 0 {
				z.log.Error("npc think failed", "npc", describe(n.e), "fails", n.brain.Fails, "err", err)
			}
		}
	}
}

// --- script.NPC implementation ---------------------------------------------

func (n *npc) Position() (float64, float64) { return n.e.pos[0], n.e.pos[2] }
func (n *npc) Home() (float64, float64)     { return n.home[0], n.home[1] }
func (n *npc) HP() (int, int)               { return n.e.hp, n.e.maxHP }
func (n *npc) Target() uint32               { return n.target }

func (n *npc) SetTarget(id uint32) {
	if id == 0 {
		n.target, n.chaseID = 0, 0
		return
	}
	if t, ok := n.z.entities[id]; ok && t.player != nil && t.alive() {
		n.target = id
	}
}

func (n *npc) NearestPlayer(radius float64) (uint32, float64, bool) {
	var best uint32
	bestD := math.Inf(1)
	n.z.grid.around(n.e.pos, func(o *entity) {
		if o.player == nil || !o.alive() {
			return
		}
		if d := hdist(n.e.pos, o.pos); d <= radius && d < bestD {
			best, bestD = o.id, d
		}
	})
	return best, bestD, best != 0
}

func (n *npc) DistanceTo(id uint32) (float64, bool) {
	t, ok := n.z.entities[id]
	if !ok || !t.alive() {
		return 0, false
	}
	return hdist(n.e.pos, t.pos), true
}

func (n *npc) MoveTo(x, z float64) {
	n.chaseID = 0
	n.goal = [2]float64{x, z}
	n.hasGoal = true
}

func (n *npc) Chase(id uint32, stop float64) {
	n.chaseID = id
	n.chaseStop = stop
	n.hasGoal = false
}

func (n *npc) Stop() {
	n.chaseID = 0
	n.hasGoal = false
}

func (n *npc) Attack(id uint32) bool {
	z := n.z
	tick := z.tick.Load()
	t, ok := z.entities[id]
	if !ok || t.player == nil || !t.alive() || !n.e.alive() || tick < n.nextAttack {
		return false
	}
	if hdist(n.e.pos, t.pos) > n.def.Stats["attack_range"]+0.5 {
		return false
	}
	n.nextAttack = tick + z.ticks(n.def.Attack.CooldownMS)
	n.e.yaw = yawToward(n.e.pos, t.pos)
	z.damage(n.e, t, 0, float64(roll(n.def.Attack.Damage)), false)
	return true
}

func (n *npc) Stat(name string) float64 { return n.def.Stats[name] }

func (n *npc) Reset() {
	n.target, n.chaseID = 0, 0
	n.e.hp = n.e.maxHP
}

var _ script.NPC = (*npc)(nil)
