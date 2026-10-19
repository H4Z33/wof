package zone

import (
	"math/rand/v2"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
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

func (z *Zone) useSkill(p *player, req *pb.UseSkill) {
	reject := func(reason string) {
		z.send(p, &pb.ServerMessage{Payload: &pb.ServerMessage_SkillRejected{
			SkillRejected: &pb.SkillRejected{SkillId: req.SkillId, Reason: reason},
		}})
	}
	sk, ok := z.content.Skills[req.SkillId]
	if !ok {
		reject("unknown_skill")
		return
	}
	if !p.e.alive() {
		reject("dead")
		return
	}
	t, ok := z.entities[req.TargetId]
	if !ok || t.npc == nil || !t.alive() {
		// M1 has no PvP: players can only fight NPCs.
		reject("invalid_target")
		return
	}
	tick := z.tick.Load()
	if tick+cooldownSlack < p.cooldowns[sk.ID] {
		reject("cooldown")
		return
	}
	if hdist(p.e.pos, t.pos) > sk.Range+rangeTolerance {
		reject("out_of_range")
		return
	}
	p.cooldowns[sk.ID] = tick + z.ticks(sk.CooldownMS)
	p.e.yaw = yawToward(p.e.pos, t.pos)
	amount := float64(roll(sk.Damage)) * rules.DamageMultiplier(p.e.level)
	z.damage(p.e, t, sk.ID, int(amount+0.5))
}

func roll(r content.Range) int { return r[0] + rand.IntN(r[1]-r[0]+1) }

func (z *Zone) damage(src, t *entity, skillID uint32, amount int) {
	amount = max(1, min(amount, t.hp))
	t.hp -= amount
	t.lastDamagedTick = z.tick.Load()
	z.events = append(z.events, &pb.CombatEvent{
		Kind: pb.CombatEventKind_COMBAT_EVENT_KIND_DAMAGE, SourceId: src.id, TargetId: t.id, SkillId: skillID, Amount: uint32(amount),
	})
	if t.player != nil {
		t.player.dirty = true
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
		if killer.player != nil {
			z.grantExp(killer.player, int64(n.def.Exp))
		}
	}
	if p := t.player; p != nil {
		p.deadUntil = tick + z.ticks(int(z.cfg.RespawnDelay.Milliseconds()))
		p.pending = p.pending[:0]
		z.log.Info("player died", "player", describe(t), "killer", describe(killer))
	}
}

func (z *Zone) grantExp(p *player, amount int64) {
	p.exp += amount
	p.dirty = true
	leveled := false
	for p.e.level < rules.MaxLevel && p.exp >= rules.ExpToNext(p.e.level) {
		p.exp -= rules.ExpToNext(p.e.level)
		p.e.level++
		leveled = true
	}
	if leveled {
		p.e.maxHP = rules.MaxHP(p.e.level)
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
	for _, n := range z.npcs {
		e := n.e
		switch {
		case e.inWorld && e.hp == 0 && tick >= n.corpseUntil:
			z.grid.remove(e)
			delete(z.entities, e.id)
		case !e.inWorld && tick >= n.respawnAt:
			z.respawnNPC(n)
		}
	}
}
