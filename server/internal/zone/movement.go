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
		// Every input the player makes feeds their fate rolls.
		p.entropy.Mix(uint64(in.Seq), uint64(math.Float32bits(in.DirX))<<32|uint64(math.Float32bits(in.DirZ)),
			uint64(math.Float32bits(in.Yaw)), uint64(z.tick.Load()))
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
		speed, rollDir, override := z.stepAction(p, in)
		if override {
			dx, dz = rollDir[0], rollDir[1]
		}
		step := z.moveSpeed(p) * speed * dt
		e.pos = z.walk(e.pos, dx*step, dz*step)
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
	if !e.alive() || e.stunned(z.tick.Load()) || n.strike != nil {
		e.moving = false // winding up an attack: stand and telegraph it
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
	wp, ok := z.steer(n, goal)
	if !ok {
		// Unreachable (cliffs, water): give up on this goal; the AI picks
		// another, and a chase retries as the target moves.
		e.moving = false
		n.hasGoal = false
		return
	}
	toWP := hdist(e.pos, wp)
	step := n.def.Speed * e.speedFactor(z.tick.Load()) * dt
	if len(n.path) <= 1 {
		step = min(step, d-stop) // last leg: stop at the right distance
	}
	step = min(step, toWP)
	if toWP == 0 {
		return
	}
	e.yaw = yawToward(e.pos, wp)
	next := z.walk(e.pos, (wp[0]-e.pos[0])/toWP*step, (wp[2]-e.pos[2])/toWP*step)
	if next[0] == e.pos[0] && next[2] == e.pos[2] {
		n.path = nil // stuck: plan again next tick
		e.moving = false
		return
	}
	e.pos = next
	e.moving = true
	z.grid.moved(e)
}

const (
	waypointReached = 0.4
	repathMoved     = 1.5 // replan a chase when the target moved this far from the planned goal
	repathTicks     = 10  // at most one replan per NPC per half second
)

// steer returns the next point to walk toward on the way to goal, planning
// a path around terrain when there is one.
func (z *Zone) steer(n *npc, goal [3]float64) ([3]float64, bool) {
	if z.nav == nil {
		return goal, true
	}
	tick := z.tick.Load()
	stale := len(n.path) == 0 || math.Hypot(goal[0]-n.pathGoal[0], goal[2]-n.pathGoal[1]) > repathMoved
	if stale && (n.pathTick == 0 || tick-n.pathTick >= repathTicks || len(n.path) == 0) {
		path, ok := z.nav.Path(n.e.pos[0], n.e.pos[2], goal[0], goal[2])
		n.pathTick = tick
		if !ok {
			n.path = nil
			return goal, false
		}
		n.path, n.pathGoal = path, [2]float64{goal[0], goal[2]}
	}
	for len(n.path) > 1 && math.Hypot(n.path[0][0]-n.e.pos[0], n.path[0][1]-n.e.pos[2]) < waypointReached {
		n.path = n.path[1:]
	}
	if len(n.path) == 0 {
		return goal, true
	}
	wp := n.path[0]
	if len(n.path) == 1 {
		return goal, true // final leg aims at the live goal (a moving chase target)
	}
	return [3]float64{wp[0], 0, wp[1]}, true
}
