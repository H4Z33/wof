package zone

import (
	"math"
	"math/rand/v2"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/fate"
	"convergence/server/internal/rules"
)

// Effect parameters by name: which flag shows on clients and how the value
// from content is used.
var effectFlags = map[string]pb.EffectFlag{
	"weakened": pb.EffectFlag_EFFECT_FLAG_WEAKENED,
	"vigor":    pb.EffectFlag_EFFECT_FLAG_VIGOR,
	"ward":     pb.EffectFlag_EFFECT_FLAG_WARD,
	"burning":  pb.EffectFlag_EFFECT_FLAG_BURNING,
	"regen":    pb.EffectFlag_EFFECT_FLAG_REGEN,
	"stunned":  pb.EffectFlag_EFFECT_FLAG_STUNNED,
	"slowed":   pb.EffectFlag_EFFECT_FLAG_SLOWED,
}

// loadLibs compiles the shared Lua libraries; they must exist before the
// scripts that call lib(name) at load time.
func (z *Zone) loadLibs() error {
	for name, src := range z.content.AILibs {
		if err := z.lua.LoadLib(name, src); err != nil {
			return err
		}
	}
	return nil
}

func (z *Zone) loadSkillScripts() error {
	if err := z.loadLibs(); err != nil {
		return err
	}
	for _, s := range z.content.Skills {
		if s.Script != "" {
			if err := z.lua.LoadSkill(s.Script, s.ScriptSource); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *player) classKey() string {
	if p.class == nil {
		return ""
	}
	return p.class.Key
}

func classKeyOf(e *entity) string {
	if e.player != nil {
		return e.player.classKey()
	}
	return ""
}

func classInfo(c *content.Class) *pb.ClassInfo {
	if c == nil {
		return nil
	}
	return &pb.ClassInfo{Key: c.Key, Name: c.Name, World: c.World, ResourceName: c.Resource.Name, ResourceColor: c.Resource.Color}
}

// hasSkill reports whether p's class grants the skill (any skill when the
// content has no classes).
func (z *Zone) hasSkill(p *player, id uint32) bool {
	if p.class == nil {
		return true
	}
	for _, s := range p.class.SkillIDs {
		if s == id {
			return true
		}
	}
	if sp := p.class.Spec(p.prog.Spec); sp != nil {
		if s := z.content.SkillByKey(sp.Skill); s != nil && s.ID == id {
			return true
		}
	}
	return false
}

func (z *Zone) skillInfos(p *player) []*pb.SkillInfo {
	var ids []uint32
	if p.class != nil {
		ids = append(ids, p.class.SkillIDs...)
		if sp := p.class.Spec(p.prog.Spec); sp != nil {
			if s := z.content.SkillByKey(sp.Skill); s != nil {
				ids = append(ids, s.ID)
			}
		}
	} else {
		for id := range z.content.Skills {
			ids = append(ids, id)
		}
	}
	out := make([]*pb.SkillInfo, 0, len(ids))
	for _, id := range ids {
		s := z.content.Skills[id]
		out = append(out, &pb.SkillInfo{
			Id: s.ID, Name: s.Name, Range: float32(s.Range), CooldownMs: uint32(s.CooldownMS),
			Anim: s.Anim, Projectile: s.Projectile, Description: s.Description, Cost: uint32(s.Cost),
			TargetSelf: s.Target == "self", Vfx: s.VFX, Radius: float32(s.Radius),
		})
	}
	return out
}

func (z *Zone) useSkill(p *player, req *pb.UseSkill) {
	reject := func(reason string) {
		z.send(p, &pb.ServerMessage{Payload: &pb.ServerMessage_SkillRejected{
			SkillRejected: &pb.SkillRejected{SkillId: req.SkillId, Reason: reason},
		}})
	}
	sk, ok := z.content.Skills[req.SkillId]
	if !ok || !z.hasSkill(p, sk.ID) {
		reject("unknown_skill")
		return
	}
	if !p.e.alive() {
		reject("dead")
		return
	}
	target := p.e
	if sk.Target == "enemy" {
		t, ok := z.entities[req.TargetId]
		if !ok || t.npc == nil || !t.alive() {
			reject("invalid_target") // no PvP yet: enemies are NPCs
			return
		}
		if hdist(p.e.pos, t.pos) > sk.Range+rangeTolerance {
			reject("out_of_range")
			return
		}
		target = t
	}
	tick := z.tick.Load()
	if tick+cooldownSlack < p.cooldowns[sk.ID] {
		reject("cooldown")
		return
	}
	if p.resource < float64(sk.Cost) {
		reject("not_enough_resource")
		return
	}
	if target != p.e {
		p.e.yaw = yawToward(p.e.pos, target.pos)
	}
	p.entropy.Mix(uint64(sk.ID), uint64(target.id), uint64(tick))

	c := &cast{z: z, p: p, skill: sk, target: target, mult: 1}
	// Fate may turn an attack into a critical hit or a miss.
	if sk.Target == "enemy" {
		if card, ok := z.fateTrigger(p, fate.TriggerStrike, target); ok {
			mult, miss := z.applyCard(p, card, target)
			c.mult, c.critical = mult, mult > 1
			if miss {
				p.cooldowns[sk.ID] = tick + z.ticks(sk.CooldownMS)
				p.resource -= float64(sk.Cost)
				z.events = append(z.events, &pb.CombatEvent{
					Kind: pb.CombatEventKind_COMBAT_EVENT_KIND_MISS, SourceId: p.e.id, TargetId: target.id, SkillId: sk.ID,
				})
				return
			}
		}
	}

	if sk.Script != "" {
		ok, reason, err := z.lua.Cast(sk.Script, c)
		if err != nil {
			z.log.Error("skill script failed", "skill", sk.Key, "err", err)
			reject("unknown_skill")
			return
		}
		if !ok {
			reject(reason)
			return
		}
	} else {
		z.applySkillData(c)
	}
	p.cooldowns[sk.ID] = tick + z.ticks(sk.CooldownMS)
	p.resource = max(0, p.resource-float64(sk.Cost))
	if c.hits > 0 && sk.Gain > 0 && p.class != nil {
		p.resource = min(z.resourceMax(p), p.resource+float64(sk.Gain)*specMod(p.class, &p.prog, "resource_gain"))
	}
}

// applySkillData runs a data-only skill: damage (with area), heal (with
// radius), and an effect on everything it touches.
func (z *Zone) applySkillData(c *cast) {
	sk := c.skill
	if sk.Target == "enemy" {
		targets := []*entity{c.target}
		if sk.Radius > 0 {
			z.grid.around(c.target.pos, func(o *entity) {
				if o != c.target && o.npc != nil && o.alive() && hdist(o.pos, c.target.pos) <= sk.Radius {
					targets = append(targets, o)
				}
			})
		}
		for _, t := range targets {
			if sk.Damage[1] > 0 {
				c.Damage(t.id, float64(roll(sk.Damage)))
			}
			if sk.Apply != nil && t.alive() {
				c.Effect(t.id, sk.Apply.Effect, sk.Apply.DurationS, sk.Apply.Value)
			}
		}
		return
	}
	// Self skills.
	targets := []*entity{c.p.e}
	if sk.HealRadius > 0 {
		z.grid.around(c.p.e.pos, func(o *entity) {
			if o != c.p.e && o.player != nil && o.alive() && hdist(o.pos, c.p.e.pos) <= sk.HealRadius {
				targets = append(targets, o)
			}
		})
	}
	for _, t := range targets {
		if sk.Heal[1] > 0 {
			c.Heal(t.id, float64(roll(sk.Heal)))
		}
		if sk.Apply != nil {
			c.Effect(t.id, sk.Apply.Effect, sk.Apply.DurationS, sk.Apply.Value)
		}
	}
}

// heal restores health and reports it to clients.
func (z *Zone) heal(src, t *entity, skillID uint32, amount int) int {
	if !t.alive() {
		return 0
	}
	amount = max(0, min(amount, t.maxHP-t.hp))
	t.hp += amount
	if t.player != nil {
		t.player.dirty = true
	}
	if amount > 0 {
		z.events = append(z.events, &pb.CombatEvent{
			Kind: pb.CombatEventKind_COMBAT_EVENT_KIND_HEAL, SourceId: src.id, TargetId: t.id, SkillId: skillID, Amount: uint32(amount),
		})
	}
	return amount
}

// addSkillEffect applies a named effect from a skill or script.
func (z *Zone) addSkillEffect(src, t *entity, skillID uint32, name string, durationS, value float64) bool {
	flag, ok := effectFlags[name]
	if !ok || !t.alive() || durationS <= 0 {
		return false
	}
	ef := effect{flag: flag, dealt: 1, taken: 1, until: z.tick.Load() + z.ticks(int(durationS*1000)), source: src.id, skill: skillID}
	switch name {
	case "weakened", "ward":
		ef.taken = value
	case "vigor":
		ef.dealt = value
	case "burning":
		ef.dot = value
	case "regen":
		ef.dot = -value
	case "slowed":
		ef.slow = value
	case "stunned":
		ef.stun = true
		if n := t.npc; n != nil {
			n.chaseID, n.hasGoal = 0, false
			t.moving = false
		}
	}
	t.addEffect(ef)
	return true
}

// tickEffects applies damage and healing over time, once per second.
func (z *Zone) tickEffects(tick uint32) {
	if tick%uint32(z.cfg.TickRate) != 0 {
		return
	}
	for _, e := range z.entities {
		for _, ef := range e.effects {
			if tick >= ef.until || ef.dot == 0 || !e.alive() {
				continue
			}
			src, ok := z.entities[ef.source]
			if !ok {
				src = e
			}
			if ef.dot > 0 {
				if s := z.content.Skills[ef.skill]; s != nil {
					z.hitDiscipline = s.Discipline
				}
				z.damage(src, e, ef.skill, ef.dot, false)
				z.hitDiscipline = ""
			} else {
				z.heal(src, e, ef.skill, int(math.Round(-ef.dot)))
			}
		}
	}
}

// updateResources regenerates or drains each player's class resource.
func (z *Zone) updateResources(tick uint32) {
	dt := 1 / float64(z.cfg.TickRate)
	for _, p := range z.players {
		r := p.class
		if r == nil || !p.e.alive() {
			continue
		}
		res := r.Resource
		p.resource += res.RegenPerS * (1 + z.attrValue(&p.prog, "focus")) * dt
		if res.DecayPerS > 0 && tick-p.combatTick > z.ticks(5000) {
			p.resource -= res.DecayPerS * dt
		}
		p.resource = max(0, min(z.resourceMax(p), p.resource))
	}
}

// stunned reports whether e cannot act right now.
func (e *entity) stunned(tick uint32) bool {
	for _, ef := range e.effects {
		if ef.stun && tick < ef.until {
			return true
		}
	}
	return false
}

// speedFactor is the product of active slows (1 = normal speed).
func (e *entity) speedFactor(tick uint32) float64 {
	f := 1.0
	for _, ef := range e.effects {
		if ef.slow > 0 && tick < ef.until {
			f *= ef.slow
		}
	}
	return f
}

// cast is one skill use; it implements script.SkillHost.
type cast struct {
	z        *Zone
	p        *player
	skill    *content.Skill
	target   *entity
	mult     float64 // fate critical multiplier
	critical bool
	hits     int
}

func (c *cast) Caster() uint32 { return c.p.e.id }
func (c *cast) Target() uint32 { return c.target.id }

func (c *cast) Alive(id uint32) bool {
	e, ok := c.z.entities[id]
	return ok && e.alive()
}

func (c *cast) Resource() float64 { return c.p.resource }

func (c *cast) Spend(amount float64) bool {
	if amount < 0 || c.p.resource < amount {
		return false
	}
	c.p.resource -= amount
	return true
}

func (c *cast) Damage(id uint32, amount float64) int {
	t, ok := c.z.entities[id]
	if !ok || t.npc == nil || !t.alive() {
		return 0
	}
	before := t.hp
	mult := c.mult * c.z.damageMult(c.p, c.skill.Discipline)
	crit := c.critical
	if !crit && c.z.rollCrit(c.p) {
		mult, crit = mult*critMult, true
	}
	c.z.hitDiscipline = c.skill.Discipline
	c.z.damage(c.p.e, t, c.skill.ID, amount*rules.DamageMultiplier(c.p.e.level)*mult, crit)
	c.z.hitDiscipline = ""
	c.hits++
	return before - t.hp
}

func (c *cast) Heal(id uint32, amount float64) int {
	t, ok := c.z.entities[id]
	if !ok || t.player == nil {
		return 0
	}
	return c.z.heal(c.p.e, t, c.skill.ID, int(math.Round(amount*c.z.healPower(c.p))))
}

func (c *cast) Effect(id uint32, name string, durationS, value float64) bool {
	t, ok := c.z.entities[id]
	if !ok {
		return false
	}
	return c.z.addSkillEffect(c.p.e, t, c.skill.ID, name, durationS, value)
}

func (c *cast) EnemiesNear(id uint32, radius float64) []uint32 {
	center, ok := c.z.entities[id]
	if !ok {
		return nil
	}
	var out []uint32
	c.z.grid.around(center.pos, func(o *entity) {
		if o.npc != nil && o.alive() && hdist(o.pos, center.pos) <= radius {
			out = append(out, o.id)
		}
	})
	return out
}

func (c *cast) Param(name string) float64 { return c.skill.Params[name] }

func (c *cast) Roll(lo, hi int) int {
	if hi < lo {
		lo, hi = hi, lo
	}
	return lo + rand.IntN(hi-lo+1)
}
