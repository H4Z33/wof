package zone

import (
	"maps"
	"math"
	"math/rand/v2"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/model"
	"convergence/server/internal/rules"
)

// Disciplines and builds (see content/disciplines.yaml). Damage dealt earns
// experience in the discipline of the weapon or skill that dealt it; that
// experience buys attribute ranks, cheaper and higher in the class's own
// discipline; enough ranks there unlock a specialization.

const (
	critMult        = 1.8
	critPerAccuracy = 0.25 // crit chance per accuracy point (0.06 per rank -> 1.5 %)
	dodgePerAgility = 1.0  // dodge chance per agility point (0.015 per rank -> 1.5 %)
	blockPerRank    = 0.01 // each toughness rank lets 1 % less through a block
	minBlockFactor  = 0.05
	progressionSend = 20 // send build updates at most once a second
)

func cloneProgression(p model.Progression) model.Progression {
	out := model.Progression{XP: maps.Clone(p.XP), Ranks: maps.Clone(p.Ranks), Spec: p.Spec}
	if out.XP == nil {
		out.XP = map[string]int64{}
	}
	if out.Ranks == nil {
		out.Ranks = map[string]int{}
	}
	return out
}

func (z *Zone) attrValue(prog *model.Progression, key string) float64 {
	d := z.content.Disciplines
	if d == nil {
		return 0
	}
	a, ok := d.Attributes[key]
	if !ok {
		return 0
	}
	return float64(prog.Ranks[key]) * a.PerRank
}

func specOf(class *content.Class, prog *model.Progression) *content.Specialization {
	if class == nil || prog.Spec == "" {
		return nil
	}
	return class.Spec(prog.Spec)
}

// specMod is a specialization modifier (1, or 0 for crit_chance, if none).
func specMod(class *content.Class, prog *model.Progression, key string) float64 {
	def := 1.0
	if key == "crit_chance" {
		def = 0
	}
	if s := specOf(class, prog); s != nil {
		if v, ok := s.Modifiers[key]; ok {
			return v
		}
	}
	return def
}

func (z *Zone) maxHPFor(class *content.Class, prog *model.Progression, level int) int {
	base := rules.MaxHP(level)
	if class != nil {
		base = class.MaxHP(level)
	}
	f := (1 + z.attrValue(prog, "toughness")) * specMod(class, prog, "max_hp")
	return int(math.Round(float64(base) * f))
}

func (z *Zone) maxHP(p *player, level int) int { return z.maxHPFor(p.class, &p.prog, level) }

func (z *Zone) resourceMax(p *player) float64 {
	if p.class == nil {
		return 0
	}
	return float64(p.class.Resource.Max) * (1 + z.attrValue(&p.prog, "focus"))
}

func (z *Zone) moveSpeed(p *player) float64 {
	return z.cfg.MoveSpeed * (1 + z.attrValue(&p.prog, "agility"))
}

// damageMult scales a player's outgoing damage by attributes and spec.
func (z *Zone) damageMult(p *player, discipline string) float64 {
	attr := map[string]string{"strength": "physique", "skill": "accuracy", "will": "spirit"}[discipline]
	return (1 + z.attrValue(&p.prog, attr)) * specMod(p.class, &p.prog, "damage_dealt")
}

// rollCrit decides a critical hit from accuracy and the spec.
func (z *Zone) rollCrit(p *player) bool {
	chance := z.attrValue(&p.prog, "accuracy")*critPerAccuracy + specMod(p.class, &p.prog, "crit_chance")
	return chance > 0 && rand.Float64() < chance
}

func (z *Zone) dodgeChance(p *player) float64 {
	return z.attrValue(&p.prog, "agility") * dodgePerAgility
}

func (z *Zone) blockFactorFor(p *player) float64 {
	return max(minBlockFactor, blockFactor-blockPerRank*float64(p.prog.Ranks["toughness"]))
}

func (z *Zone) healPower(p *player) float64 {
	return (1 + z.attrValue(&p.prog, "spirit")) * specMod(p.class, &p.prog, "heal_power")
}

func (p *player) affinity() string {
	if p.class != nil {
		return p.class.Weapon.Discipline
	}
	return "strength"
}

func (z *Zone) attrCap(p *player, a *content.Attribute) int {
	d := z.content.Disciplines
	if a.Discipline == p.affinity() {
		return d.Caps.Affinity
	}
	return d.Caps.Other
}

func (z *Zone) nextCost(p *player, a *content.Attribute) int64 {
	d := z.content.Disciplines
	c := d.Cost.Base * float64(p.prog.Ranks[a.Key]+1)
	if a.Discipline != p.affinity() {
		c *= d.Cost.OffAffinity
	}
	return int64(math.Round(c))
}

func (z *Zone) disciplineRanks(p *player, discipline string) int {
	n := 0
	for k, a := range z.content.Disciplines.Attributes {
		if a.Discipline == discipline {
			n += p.prog.Ranks[k]
		}
	}
	return n
}

// awardDisciplineXP credits damage dealt to its discipline.
func (z *Zone) awardDisciplineXP(p *player, discipline string, damage int) {
	d := z.content.Disciplines
	if d == nil || discipline == "" || damage <= 0 {
		return
	}
	if _, ok := d.Disciplines[discipline]; !ok {
		return
	}
	p.prog.XP[discipline] += int64(math.Round(float64(damage) * d.XPPerDamage * p.multiplier()))
	p.progDirty = true
	p.dirty = true
}

func (z *Zone) raiseAttribute(p *player, key string) {
	d := z.content.Disciplines
	if d == nil {
		return
	}
	a, ok := d.Attributes[key]
	if !ok || p.prog.Ranks[key] >= z.attrCap(p, a) {
		return
	}
	cost := z.nextCost(p, a)
	if p.prog.XP[a.Discipline] < cost {
		return
	}
	p.prog.XP[a.Discipline] -= cost
	p.prog.Ranks[key]++
	z.refreshDerived(p)
	p.progDirty, p.dirty = true, true
	z.sendProgression(p)
}

func (z *Zone) chooseSpec(p *player, key string) {
	d := z.content.Disciplines
	if d == nil || p.class == nil || p.prog.Spec != "" || p.class.Spec(key) == nil {
		return
	}
	if z.disciplineRanks(p, p.affinity()) < d.SpecUnlockRanks {
		return
	}
	p.prog.Spec = key
	z.refreshDerived(p)
	p.progDirty, p.dirty = true, true
	z.log.Info("specialization chosen", "player", describe(p.e), "spec", key)
	z.sendProgression(p)
}

// refreshDerived updates health and resource limits after a build change.
func (z *Zone) refreshDerived(p *player) {
	old := p.e.maxHP
	p.e.maxHP = z.maxHP(p, p.e.level)
	if p.e.maxHP > old {
		p.e.hp += p.e.maxHP - old // new health comes filled
	}
	p.e.hp = min(p.e.hp, p.e.maxHP)
	p.resource = min(p.resource, z.resourceMax(p))
}

func (z *Zone) progression(p *player) *pb.Progression {
	out := &pb.Progression{Spec: p.prog.Spec, Skills: z.skillInfos(p), MoveSpeed: float32(z.moveSpeed(p)),
		Titles: z.titleInfos(p), Title: p.title}
	d := z.content.Disciplines
	if d == nil {
		return out
	}
	out.SpecUnlockRanks = uint32(d.SpecUnlockRanks)
	for _, key := range []string{"strength", "skill", "will"} {
		info, ok := d.Disciplines[key]
		if !ok {
			continue
		}
		out.Disciplines = append(out.Disciplines, &pb.DisciplineState{
			Key: key, Name: info.Name, Xp: uint64(p.prog.XP[key]),
			Ranks: uint32(z.disciplineRanks(p, key)), Affinity: key == p.affinity(),
		})
	}
	for _, key := range content.AttributeKeys {
		a := d.Attributes[key]
		out.Attributes = append(out.Attributes, &pb.AttributeState{
			Key: key, Name: a.Name, Discipline: a.Discipline, Description: a.Description,
			Rank: uint32(p.prog.Ranks[key]), Cap: uint32(z.attrCap(p, a)), NextCost: uint64(z.nextCost(p, a)),
		})
	}
	if p.class != nil && p.prog.Spec == "" && z.disciplineRanks(p, p.affinity()) >= d.SpecUnlockRanks {
		for _, sp := range p.class.Specializations {
			name := sp.Skill
			if s := z.content.SkillByKey(sp.Skill); s != nil {
				name = s.Name
			}
			out.SpecOptions = append(out.SpecOptions, &pb.SpecInfo{Key: sp.Key, Name: sp.Name, Description: sp.Description, SkillName: name})
		}
	}
	return out
}

func (z *Zone) sendProgression(p *player) {
	p.progDirty = false
	p.progSent = z.tick.Load()
	z.send(p, &pb.ServerMessage{Payload: &pb.ServerMessage_Progression{Progression: z.progression(p)}})
}

// updateProgression sends pending build updates (experience changes often).
func (z *Zone) updateProgression(tick uint32) {
	for _, p := range z.players {
		if p.progDirty && tick-p.progSent >= progressionSend {
			z.sendProgression(p)
		}
	}
}
