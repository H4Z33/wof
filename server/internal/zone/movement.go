package zone

import "math"

const maxPending = 32

// applyInputs runs the queued MoveInputs of a player, within its input credit.
// Must stay in sync with the client's prediction (world.gd _simulate).
func (z *Zone) applyInputs(p *player, dt float64) {
	e := p.e
	p.credit = min(p.credit+1, z.cfg.MaxInputCredit)
	applied := false
	for len(p.pending) > 0 && p.credit > 0 {
		in := p.pending[0]
		p.pending = p.pending[1:]
		if in.Seq <= p.lastSeq {
			continue // duplicate or out of order
		}
		p.credit--
		p.lastSeq = in.Seq
		if !e.alive() {
			continue // acknowledged, but the dead do not walk
		}
		applied = true

		dx, dz := float64(in.DirX), float64(in.DirZ)
		if l := math.Hypot(dx, dz); l > 1 {
			dx, dz = dx/l, dz/l
		} else if math.IsNaN(l) {
			dx, dz = 0, 0
		}
		step := z.cfg.MoveSpeed * dt
		lim := z.cfg.HalfExtent
		e.pos[0] = clamp(e.pos[0]+dx*step, -lim, lim)
		e.pos[2] = clamp(e.pos[2]+dz*step, -lim, lim)
		if y := float64(in.Yaw); !math.IsNaN(y) && !math.IsInf(y, 0) {
			e.yaw = math.Remainder(y, 2*math.Pi)
		}
		e.moving = dx != 0 || dz != 0
		p.dirty = true
	}
	if applied {
		p.idleTicks = 0
		z.grid.moved(e)
	} else if p.idleTicks++; p.idleTicks > 3 || !e.alive() {
		// Short grace period so a late packet does not flicker the animation.
		e.moving = false
	}
}

// moveNPC walks an NPC toward its chase target or goal point.
func (z *Zone) moveNPC(n *npc, dt float64) {
	e := n.e
	if !e.alive() {
		e.moving = false
		return
	}
	var goal [3]float64
	var stop float64
	switch {
	case n.chaseID != 0:
		t, ok := z.entities[n.chaseID]
		if !ok || !t.alive() {
			n.chaseID = 0
			e.moving = false
			return
		}
		goal, stop = t.pos, n.chaseStop
	case n.hasGoal:
		goal, stop = [3]float64{n.goal[0], 0, n.goal[1]}, 0.1
	default:
		e.moving = false
		return
	}
	d := hdist(e.pos, goal)
	if d <= stop {
		e.moving = false
		n.hasGoal = false
		if n.chaseID != 0 {
			e.yaw = yawToward(e.pos, goal)
		}
		return
	}
	step := min(n.def.Speed*dt, d-stop)
	lim := z.cfg.HalfExtent
	e.yaw = yawToward(e.pos, goal)
	e.pos[0] = clamp(e.pos[0]+(goal[0]-e.pos[0])/d*step, -lim, lim)
	e.pos[2] = clamp(e.pos[2]+(goal[2]-e.pos[2])/d*step, -lim, lim)
	e.moving = true
	z.grid.moved(e)
}
