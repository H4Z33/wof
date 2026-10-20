package zone

import (
	"math"
	"math/rand/v2"
	"sort"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/fate"
	"convergence/server/internal/rules"
)

// Player action state machine. It advances once per MoveInput, timed in
// input sequence numbers (one input = one tick), so the client can predict
// it exactly from its own inputs. Keep in sync with world.gd _step_action().
const (
	chargeTicks       = 8  // hold this long after a swing to charge a flourish
	flourishExtra     = 4  // a flourish lasts this much longer than a swing
	rollTicks         = 8  // a roll lasts 0.4 s
	rollCooldownTicks = 16 // and can be used again 0.8 s after it started
	rollIFrameTicks   = 6  // invulnerable for the first 0.3 s
	rollSpeed         = 2.4
	blockSpeed        = 0.45
	swingSpeed        = 0.35
	chargeSpeed       = 0.6

	rewindTicks    = 3    // lag compensation: client view of NPCs is this far behind
	historyLen     = 8    // NPC positions kept for rewinding
	targetRadius   = 0.5  // NPC body radius for hit checks
	shotWidth      = 0.9  // ranged attacks hit within this distance of the aim ray
	lockAimDeg     = 50.0 // a locked target is aimed at if roughly in front
	blockArcDeg    = 75.0 // blocks stop hits from within this angle of facing
	blockFactor    = 0.2  // damage that gets through a block
	npcWindupMS    = 450
	npcSlamWindup  = 800
	strikeReach    = 1.0 // NPC melee strikes still land if the target is this much beyond range
	comboStep      = 0.1
	maxMultiplier  = 5.0
	meleeMaxTarget = 2 // plain swings hit at most this many enemies (flourishes hit all)
)

var fists = &content.Weapon{
	Name: "Fists", Kind: "melee", Discipline: "strength", Damage: content.Range{3, 5}, Range: 2,
	ArcDeg: 120, SwingMS: 500, HitMS: 200, FlourishMult: 2, FlourishArcDeg: 200, Anim: "Punch_Cross", FlourishAnim: "Punch_Cross",
}

func (p *player) weapon() *content.Weapon {
	if p.class != nil && p.class.Weapon.Kind != "" {
		return &p.class.Weapon
	}
	return fists
}

func (z *Zone) actionRules(p *player) *pb.ActionRules {
	w := p.weapon()
	swing := z.ticks(w.SwingMS)
	return &pb.ActionRules{
		SwingTicks: swing, FlourishTicks: swing + flourishExtra, ChargeTicks: chargeTicks,
		RollTicks: rollTicks, RollCooldownTicks: rollCooldownTicks,
		RollSpeed: rollSpeed, BlockSpeed: blockSpeed, SwingSpeed: swingSpeed, ChargeSpeed: chargeSpeed,
	}
}

func weaponInfo(w *content.Weapon) *pb.WeaponInfo {
	return &pb.WeaponInfo{Name: w.Name, Kind: w.Kind, Range: float32(w.Range), ArcDeg: float32(w.ArcDeg),
		Anim: w.Anim, FlourishAnim: w.FlourishAnim, Vfx: w.VFX}
}

func (z *Zone) setAction(e *entity, a pb.ActionState) {
	if e.action != a {
		e.action = a
		e.actionTick = z.tick.Load()
	}
}

// stepAction advances p's state machine for one input and returns the
// movement speed factor and direction override (rolls) for that input.
func (z *Zone) stepAction(p *player, in *pb.MoveInput) (speed float64, dir [2]float64, override bool) {
	e := p.e
	b, prev := in.Buttons, p.prevButtons
	p.prevButtons = b
	p.lockTarget = in.LockTarget
	seq := in.Seq
	held := b&uint32(pb.InputButton_INPUT_BUTTON_ATTACK) != 0
	pressed := held && prev&uint32(pb.InputButton_INPUT_BUTTON_ATTACK) == 0
	released := !held && prev&uint32(pb.InputButton_INPUT_BUTTON_ATTACK) != 0
	block := b&uint32(pb.InputButton_INPUT_BUTTON_BLOCK) != 0
	roll := b&uint32(pb.InputButton_INPUT_BUTTON_ROLL) != 0

	busy := e.action == pb.ActionState_ACTION_STATE_SWING || e.action == pb.ActionState_ACTION_STATE_FLOURISH
	if busy && pressed {
		p.queuedAttack = true // chain the next swing as soon as this one ends
	}
	// Timed actions end.
	if (busy || e.action == pb.ActionState_ACTION_STATE_ROLL) && seq >= p.actEnd {
		switch {
		case e.action != pb.ActionState_ACTION_STATE_ROLL && p.queuedAttack:
			p.queuedAttack = false
			z.startSwing(p, in, false)
		case e.action != pb.ActionState_ACTION_STATE_ROLL && held:
			z.setAction(e, pb.ActionState_ACTION_STATE_CHARGING)
			p.chargeSeq = seq
		default:
			z.setAction(e, pb.ActionState_ACTION_STATE_UNSPECIFIED)
		}
		busy = e.action == pb.ActionState_ACTION_STATE_SWING || e.action == pb.ActionState_ACTION_STATE_FLOURISH
	}

	if roll && !busy && e.action != pb.ActionState_ACTION_STATE_ROLL && seq >= p.rollReady {
		d := [2]float64{float64(in.DirX), float64(in.DirZ)}
		if l := math.Hypot(d[0], d[1]); l > 0.01 {
			d = [2]float64{d[0] / l, d[1] / l}
		} else { // standing still: roll backwards
			d = [2]float64{-math.Sin(e.yaw), -math.Cos(e.yaw)}
		}
		p.rollDir = d
		p.actEnd = seq + rollTicks
		p.rollReady = seq + rollCooldownTicks
		p.iframeUntil = z.tick.Load() + rollIFrameTicks
		p.queuedAttack = false
		z.setAction(e, pb.ActionState_ACTION_STATE_ROLL)
	}

	switch e.action {
	case pb.ActionState_ACTION_STATE_UNSPECIFIED, pb.ActionState_ACTION_STATE_BLOCK:
		switch {
		case pressed:
			z.startSwing(p, in, false)
		case block:
			z.setAction(e, pb.ActionState_ACTION_STATE_BLOCK)
		default:
			z.setAction(e, pb.ActionState_ACTION_STATE_UNSPECIFIED)
		}
	case pb.ActionState_ACTION_STATE_CHARGING:
		if released {
			if seq-p.chargeSeq >= chargeTicks {
				z.startSwing(p, in, true)
			} else {
				z.setAction(e, pb.ActionState_ACTION_STATE_UNSPECIFIED)
			}
		}
	}

	switch e.action {
	case pb.ActionState_ACTION_STATE_ROLL:
		return rollSpeed, p.rollDir, true
	case pb.ActionState_ACTION_STATE_BLOCK:
		return blockSpeed, dir, false
	case pb.ActionState_ACTION_STATE_SWING, pb.ActionState_ACTION_STATE_FLOURISH:
		return swingSpeed, dir, false
	case pb.ActionState_ACTION_STATE_CHARGING:
		return chargeSpeed, dir, false
	}
	return 1, dir, false
}

type pendingHit struct {
	attacker uint32 // entity id
	at       uint32 // server tick the hit resolves
	flourish bool
	facing   float64
	lock     uint32 // locked target when the attack started
}

func (z *Zone) startSwing(p *player, in *pb.MoveInput, flourish bool) {
	w := p.weapon()
	swing := z.ticks(w.SwingMS)
	hit := z.ticks(w.HitMS)
	a := pb.ActionState_ACTION_STATE_SWING
	if flourish {
		a = pb.ActionState_ACTION_STATE_FLOURISH
		swing += flourishExtra
		hit += 2
	}
	p.actEnd = in.Seq + swing
	p.e.action = pb.ActionState_ACTION_STATE_UNSPECIFIED // force a fresh actionTick for chained swings
	z.setAction(p.e, a)
	p.entropy.Mix(uint64(in.Seq), uint64(math.Float32bits(in.Yaw)), 0xA77AC)
	z.pendingHits = append(z.pendingHits, pendingHit{attacker: p.e.id, at: z.tick.Load() + hit, flourish: flourish, facing: float64(in.Yaw), lock: in.LockTarget})
}

// resolveHits lands weapon attacks whose active moment has come.
func (z *Zone) resolveHits(tick uint32) {
	kept := z.pendingHits[:0]
	for _, h := range z.pendingHits {
		if tick < h.at {
			kept = append(kept, h)
			continue
		}
		e, ok := z.entities[h.attacker]
		if !ok || e.player == nil || !e.alive() || e.stunned(tick) {
			continue
		}
		z.landWeapon(e.player, h)
	}
	z.pendingHits = kept
}

func angleDiff(a, b float64) float64 {
	return math.Abs(math.Remainder(a-b, 2*math.Pi))
}

// rewound is where the attacking client saw an NPC: a few ticks back.
func (z *Zone) rewound(e *entity) [3]float64 {
	if e.npc == nil || !e.histFull && z.tick.Load() < rewindTicks {
		return e.pos
	}
	return e.hist[(z.tick.Load()-rewindTicks)%historyLen]
}

func (z *Zone) recordHistory(tick uint32) {
	for _, n := range z.npcs {
		n.e.hist[tick%historyLen] = n.e.pos
		if tick%historyLen == historyLen-1 {
			n.e.histFull = true
		}
	}
}

func (z *Zone) landWeapon(p *player, h pendingHit) {
	w := p.weapon()
	e := p.e
	var targets []*entity
	if w.Kind == "melee" {
		arc := w.ArcDeg
		if h.flourish && w.FlourishArcDeg > 0 {
			arc = w.FlourishArcDeg
		}
		reach := w.Range + targetRadius
		type cand struct {
			e *entity
			d float64
		}
		var cs []cand
		z.grid.around(e.pos, func(o *entity) {
			if o.npc == nil || !o.alive() {
				return
			}
			pos := z.rewound(o)
			d := hdist(e.pos, pos)
			if d <= reach && (d < 0.6 || angleDiff(h.facing, yawToward(e.pos, pos)) <= arc*math.Pi/360) {
				cs = append(cs, cand{o, d})
			}
		})
		sort.Slice(cs, func(i, j int) bool { return cs[i].d < cs[j].d })
		for i, c := range cs {
			if !h.flourish && i >= meleeMaxTarget {
				break
			}
			targets = append(targets, c.e)
		}
	} else {
		reach := w.Range
		if h.flourish && w.FlourishRange > 0 {
			reach = w.FlourishRange
		}
		if t := z.aimTarget(p, h.lock, h.facing, reach); t != nil {
			targets = append(targets, t)
			if h.flourish && w.FlourishRadius > 0 {
				z.grid.around(t.pos, func(o *entity) {
					if o != t && o.npc != nil && o.alive() && hdist(o.pos, t.pos) <= w.FlourishRadius {
						targets = append(targets, o)
					}
				})
			}
		}
	}
	if len(targets) == 0 {
		return
	}

	mult := z.damageMult(p, w.Discipline)
	if h.flourish {
		mult *= w.FlourishMult * specMod(p.class, &p.prog, "flourish_mult")
	}
	critical := z.rollCrit(p)
	if critical {
		mult *= critMult
	}
	if card, ok := z.fateTrigger(p, fate.TriggerStrike, targets[0]); ok {
		m, miss := z.applyCard(p, card, targets[0])
		if miss {
			z.events = append(z.events, &pb.CombatEvent{
				Kind: pb.CombatEventKind_COMBAT_EVENT_KIND_MISS, SourceId: e.id, TargetId: targets[0].id, Weapon: true, Flourish: h.flourish,
			})
			return
		}
		mult *= m
		critical = critical || m > 1
	}
	z.weaponHit, z.flourishHit, z.hitDiscipline = true, h.flourish, w.Discipline
	for _, t := range targets {
		z.damage(e, t, 0, float64(roll(w.Damage))*rules.DamageMultiplier(e.level)*mult, critical)
	}
	z.weaponHit, z.flourishHit, z.hitDiscipline = false, false, ""
	p.combo++
	if w.Gain > 0 && p.class != nil {
		p.resource = min(z.resourceMax(p), p.resource+float64(w.Gain)*specMod(p.class, &p.prog, "resource_gain"))
	}
}

// aimTarget picks what a ranged attack hits: the locked target if it is in
// range and roughly in front, else the nearest enemy along the aim ray.
func (z *Zone) aimTarget(p *player, lock uint32, facing, reach float64) *entity {
	e := p.e
	if t, ok := z.entities[lock]; ok && t.npc != nil && t.alive() {
		pos := z.rewound(t)
		if hdist(e.pos, pos) <= reach+targetRadius && angleDiff(facing, yawToward(e.pos, pos)) <= lockAimDeg*math.Pi/180 {
			return t
		}
	}
	fx, fz := math.Sin(facing), math.Cos(facing)
	var best *entity
	bestAlong := math.Inf(1)
	z.grid.around(e.pos, func(o *entity) {
		if o.npc == nil || !o.alive() {
			return
		}
		pos := z.rewound(o)
		dx, dz := pos[0]-e.pos[0], pos[2]-e.pos[2]
		along := dx*fx + dz*fz
		if along <= 0 || along > reach+targetRadius {
			return
		}
		if perp := math.Abs(dx*fz - dz*fx); perp <= shotWidth && along < bestAlong {
			best, bestAlong = o, along
		}
	})
	return best
}

// multiplier is the combat multiplier on experience.
func (p *player) multiplier() float64 {
	return min(maxMultiplier, 1+comboStep*float64(p.combo))
}

// --- NPC attacks: telegraphed wind-up, then the strike lands --------------

type npcStrike struct {
	target uint32
	area   float64 // > 0: everyone within this radius
	at     uint32
}

func (z *Zone) startStrike(n *npc, s *npcStrike, windupMS int, action pb.ActionState) {
	tick := z.tick.Load()
	s.at = tick + z.ticks(windupMS)
	n.strike = s
	n.nextAttack = tick + z.ticks(n.def.Attack.CooldownMS)
	n.chaseID, n.hasGoal = 0, false
	n.e.moving = false
	z.setAction(n.e, action)
}

func (z *Zone) resolveStrikes(tick uint32) {
	for _, n := range z.npcs {
		s := n.strike
		if s == nil || tick < s.at {
			continue
		}
		n.strike = nil
		z.setAction(n.e, pb.ActionState_ACTION_STATE_UNSPECIFIED)
		if !n.e.alive() || n.e.stunned(tick) {
			continue
		}
		dmg := func(t *entity) { z.strikePlayer(n, t, float64(roll(n.def.Attack.Damage))) }
		if s.area > 0 {
			z.grid.around(n.e.pos, func(o *entity) {
				if o.player != nil && o.alive() && hdist(o.pos, n.e.pos) <= s.area {
					dmg(o)
				}
			})
			continue
		}
		t, ok := z.entities[s.target]
		if !ok || t.player == nil || !t.alive() {
			continue
		}
		reach := n.def.Stats["attack_range"] + strikeReach
		if n.def.Attack.Projectile {
			reach += 4 // shots track their target a little
		}
		if hdist(n.e.pos, t.pos) <= reach {
			n.e.yaw = yawToward(n.e.pos, t.pos)
			dmg(t)
		}
	}
}

// strikePlayer applies an NPC hit, honoring rolls and blocks.
func (z *Zone) strikePlayer(n *npc, t *entity, amount float64) {
	p := t.player
	tick := z.tick.Load()
	if tick < p.iframeUntil || rand.Float64() < z.dodgeChance(p) {
		z.events = append(z.events, &pb.CombatEvent{Kind: pb.CombatEventKind_COMBAT_EVENT_KIND_DODGED, SourceId: n.e.id, TargetId: t.id})
		return
	}
	if t.action == pb.ActionState_ACTION_STATE_BLOCK && angleDiff(t.yaw, yawToward(t.pos, n.e.pos)) <= blockArcDeg*math.Pi/180 {
		z.blockedHit = true
		z.damage(n.e, t, 0, amount*z.blockFactorFor(p), false)
		z.blockedHit = false
		return
	}
	z.damage(n.e, t, 0, amount, false)
}
