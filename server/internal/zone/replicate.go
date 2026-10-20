package zone

import (
	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/rules"
)

// replicate sends each player its own Snapshot: entities in its interest
// area (spawned the first time they appear, despawned when they leave it),
// combat events involving anything it can see, and its own status.
func (z *Zone) replicate(tick uint32) {
	states := make(map[uint32]*pb.EntityState)
	stateOf := func(e *entity) *pb.EntityState {
		if s, ok := states[e.id]; ok {
			return s
		}
		s := &pb.EntityState{
			Id: e.id, Pos: vec(e.pos), Yaw: float32(e.yaw), Moving: e.moving,
			Hp: uint32(e.hp), MaxHp: uint32(e.maxHP), Effects: e.effectFlags(tick),
			Action: e.action, ActionTick: e.actionTick,
		}
		if e.player != nil {
			s.Alignment = int32(e.player.karma)
		}
		states[e.id] = s
		return s
	}

	for _, p := range z.players {
		clear(p.visible)
		z.grid.around(p.e.pos, func(e *entity) { p.visible[e.id] = e })

		snap := &pb.Snapshot{Tick: tick, AckInputSeq: p.lastSeq, Status: z.status(p)}
		for id, e := range p.visible {
			if p.known[id] {
				snap.Entities = append(snap.Entities, stateOf(e))
				continue
			}
			sp := &pb.EntitySpawn{
				Id: id, Name: e.name, Model: e.model, State: stateOf(e), Kind: e.kind, Level: uint32(e.level),
				ClassKey: classKeyOf(e),
			}
			if e.player != nil {
				sp.Title = z.titleName(e.player)
			}
			if e.npc != nil {
				sp.Friendly, sp.QuestGiver = e.npc.def.Friendly, e.npc.def.QuestGiver
			}
			if e.npc != nil {
				a := e.npc.def.Attack
				sp.AttackAnim, sp.AttackVfx, sp.AttackProjectile = a.Anim, a.VFX, a.Projectile
			}
			snap.Spawns = append(snap.Spawns, sp)
			p.known[id] = true
		}
		for id := range p.known {
			if _, ok := p.visible[id]; !ok {
				snap.Despawns = append(snap.Despawns, id)
				delete(p.known, id)
			}
		}
		for _, sp := range z.speech {
			if _, seen := p.visible[sp.EntityId]; seen {
				snap.Speech = append(snap.Speech, sp)
			}
		}
		for _, ev := range z.events {
			_, srcSeen := p.visible[ev.SourceId]
			_, dstSeen := p.visible[ev.TargetId]
			if srcSeen || dstSeen || ev.TargetId == p.e.id {
				snap.Events = append(snap.Events, ev)
			}
		}
		z.send(p, &pb.ServerMessage{Payload: &pb.ServerMessage_Snapshot{Snapshot: snap}})
	}
	z.events = z.events[:0]
	z.speech = z.speech[:0]
}

func (z *Zone) status(p *player) *pb.PlayerStatus {
	s := &pb.PlayerStatus{
		Level:      uint32(p.e.level),
		Exp:        uint64(p.exp),
		ExpToNext:  uint64(rules.ExpToNext(p.e.level)),
		Hp:         uint32(p.e.hp),
		MaxHp:      uint32(p.e.maxHP),
		Luck:       uint32(p.luck),
		Karma:      int32(p.karma),
		Resource:   uint32(p.resource),
		Multiplier: float32(p.multiplier()),
		Combo:      uint32(p.combo),
		Renown:     uint32(p.renown),
	}
	if p.class != nil {
		s.ResourceMax = uint32(p.class.Resource.Max)
	}
	if p.deadUntil != 0 {
		if now := z.tick.Load(); p.deadUntil > now {
			s.RespawnInMs = (p.deadUntil - now) * 1000 / uint32(z.cfg.TickRate)
		}
	}
	return s
}
