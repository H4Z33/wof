package zone

import (
	"math"
	"math/rand/v2"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/fate"
	"convergence/server/internal/rules"
)

const (
	// Extra range allowed for player skills: the client sees others ~100 ms
	// in the past, so a target can drift slightly out of range.
	rangeTolerance = 1.5
	// Ticks of slack on cooldowns, for client-side prediction jitter.
	cooldownSlack = 2
	corpseMS      = 3000
)

func roll(r content.Range) int { return r[0] + rand.IntN(r[1]-r[0]+1) }

// damage applies base damage, scaled by both sides' active effects.
func (z *Zone) damage(src, t *entity, skillID uint32, base float64, critical bool) {
	if !t.alive() {
		return
	}
	tick := z.tick.Load()
	if src.player != nil {
		src.player.combatTick = tick
	}
	dealt, _ := src.multipliers(tick)
	_, taken := t.multipliers(tick)
	if tp := t.player; tp != nil {
		taken *= specMod(tp.class, &tp.prog, "damage_taken")
	}
	amount := max(1, min(int(base*dealt*taken+0.5), t.hp))
	t.hp -= amount
	t.lastDamagedTick = tick
	kind := pb.CombatEventKind_COMBAT_EVENT_KIND_DAMAGE
	if z.blockedHit {
		kind = pb.CombatEventKind_COMBAT_EVENT_KIND_BLOCKED
	}
	z.events = append(z.events, &pb.CombatEvent{
		Kind: kind, SourceId: src.id, TargetId: t.id, SkillId: skillID,
		Amount: uint32(amount), Critical: critical, Weapon: z.weaponHit, Flourish: z.flourishHit,
	})
	if sp := src.player; sp != nil && t.npc != nil {
		z.awardDisciplineXP(sp, z.hitDiscipline, amount)
	}
	if p := t.player; p != nil {
		p.dirty = true
		if !z.blockedHit {
			p.combo = 0 // taking a hit breaks the combat multiplier
		}
		if f := z.content.Fate; f != nil && p.woundArmed && t.hp > 0 &&
			float64(t.hp) < f.Triggers[fate.TriggerWounded].BelowHP*float64(t.maxHP) {
			p.woundArmed = false
			if card, ok := z.fateTrigger(p, fate.TriggerWounded, src); ok {
				z.applyCard(p, card, src)
			}
		}
	}
	if t.npc != nil && t.npc.target == 0 && t.hp > 0 {
		t.npc.target = src.id // fight back
	}
	if t.hp == 0 {
		z.events = append(z.events, &pb.CombatEvent{
			Kind: pb.CombatEventKind_COMBAT_EVENT_KIND_DEATH, SourceId: src.id, TargetId: t.id, SkillId: skillID,
		})
		z.kill(src, t)
	}
}

func (z *Zone) kill(killer, t *entity) {
	tick := z.tick.Load()
	t.moving = false
	if n := t.npc; n != nil {
		n.target, n.chaseID, n.hasGoal = 0, 0, false
		n.corpseUntil = tick + z.ticks(corpseMS)
		n.respawnAt = tick + z.ticks(n.def.RespawnMS)
		if kp := killer.player; kp != nil && n.def.Friendly {
			// Murdering villagers: a wicked deed that everyone will see.
			z.addKarma(kp, friendlyKarma)
			z.refreshLook(kp.e)
			z.log.Info("villager killed", "player", describe(kp.e), "npc", n.def.Key)
		} else if kp != nil {
			z.grantExp(kp, int64(n.def.Exp))
			if tc := z.content.Titles; tc != nil {
				z.addRenown(kp, n.def.Level*tc.Renown.KillPerLevel)
			}
			kp.kills++
			if f := z.content.Fate; f != nil {
				z.addKarma(kp, f.Karma.KillHostileNPC)
			}
			z.questKill(kp, n.def.Key)
			if card, ok := z.fateTrigger(kp, fate.TriggerKill, t); ok {
				z.applyCard(kp, card, t)
			}
		}
	}
	if p := t.player; p != nil {
		p.deaths++
		p.combo = 0
		z.setAction(t, pb.ActionState_ACTION_STATE_UNSPECIFIED)
		p.deadUntil = tick + z.ticks(int(z.cfg.RespawnDelay.Milliseconds()))
		p.pending = p.pending[:0]
		z.log.Info("player died", "player", describe(t), "killer", describe(killer))
	}
}

func (z *Zone) grantExp(p *player, amount int64) {
	z.grantExpRaw(p, int64(math.Round(float64(amount)*p.multiplier())))
}

// grantExpRaw gives experience without the combat multiplier (quest rewards).
func (z *Zone) grantExpRaw(p *player, amount int64) {
	p.exp += amount
	p.dirty = true
	leveled := false
	for p.e.level < rules.MaxLevel && p.exp >= rules.ExpToNext(p.e.level) {
		p.exp -= rules.ExpToNext(p.e.level)
		p.e.level++
		leveled = true
	}
	if leveled {
		p.e.maxHP = z.maxHP(p, p.e.level)
		p.e.hp = p.e.maxHP
		z.log.Info("level up", "player", describe(p.e), "level", p.e.level)
		z.save(p)
	}
}

// updateLife handles respawns, corpse removal and health regeneration.
func (z *Zone) updateLife(tick uint32) {
	for _, p := range z.players {
		e := p.e
		switch {
		case p.deadUntil != 0 && tick >= p.deadUntil:
			p.deadUntil = 0
			e.hp = e.maxHP
			e.pos = z.spawnPoint()
			z.grid.moved(e)
			p.dirty = true
		case e.alive() && e.hp < e.maxHP && tick-e.lastDamagedTick >= z.ticks(int(z.cfg.RegenDelay.Milliseconds())) &&
			tick%uint32(z.cfg.TickRate) == 0:
			e.hp = min(e.maxHP, e.hp+max(1, e.maxHP/20))
			p.dirty = true
		}
	}
	if tick%uint32(z.cfg.TickRate) == 0 {
		for _, e := range z.entities {
			pruneEffects(e, tick)
		}
	}
	kept := z.npcs[:0]
	for _, n := range z.npcs {
		e := n.e
		switch {
		case e.inWorld && e.hp == 0 && tick >= n.corpseUntil:
			z.grid.remove(e)
			delete(z.entities, e.id)
		case n.temporary && e.alive() && tick >= n.expireAt:
			z.grid.remove(e) // the ambush gives up and melts away
			delete(z.entities, e.id)
		case !e.inWorld && !n.temporary && tick >= n.respawnAt:
			z.respawnNPC(n)
		}
		if n.temporary && !e.inWorld {
			continue // gone for good
		}
		kept = append(kept, n)
	}
	clear(z.npcs[len(kept):])
	z.npcs = kept
}
