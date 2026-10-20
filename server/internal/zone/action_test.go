package zone

import (
	"math"
	"testing"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
)

const (
	btnAttack = uint32(pb.InputButton_INPUT_BUTTON_ATTACK)
	btnBlock  = uint32(pb.InputButton_INPUT_BUTTON_BLOCK)
	btnRoll   = uint32(pb.InputButton_INPUT_BUTTON_ROLL)
)

// actor drives one player with per-tick inputs, like a client would.
type actor struct {
	z    *Zone
	p    *player
	conn *fakeConn
	seq  uint32
}

func newActor(t *testing.T, z *Zone, class string) *actor {
	p, c := joinClass(t, z, class)
	p.e.hp, p.e.maxHP = 5000, 5000
	return &actor{z: z, p: p, conn: c}
}

// tick sends one input and advances the zone one tick.
func (a *actor) tick(buttons uint32, yaw float64, dx, dz float32) {
	a.seq++
	a.p.pending = append(a.p.pending, &pb.MoveInput{Seq: a.seq, DirX: dx, DirZ: dz, Yaw: float32(yaw), Buttons: buttons})
	a.z.step()
}

func (a *actor) ticks(n int, buttons uint32, yaw float64) {
	for range n {
		a.tick(buttons, yaw, 0, 0)
	}
}

func (a *actor) weaponHits() (plain, flourish int, dealt int) {
	for _, ev := range a.conn.events() {
		if ev.Kind == pb.CombatEventKind_COMBAT_EVENT_KIND_DAMAGE && ev.Weapon && ev.SourceId == a.p.e.id {
			if ev.Flourish {
				flourish++
			} else {
				plain++
			}
			dealt += int(ev.Amount)
		}
	}
	return
}

func eventsOf(c *fakeConn, kind pb.CombatEventKind) int { return countEvents(c, kind) }

// faceYaw is the yaw from the origin toward (x, z).
func faceYaw(x, z float64) float64 { return math.Atan2(x, z) }

func dummy(t *testing.T, z *Zone, x, zz float64) *npc {
	t.Helper()
	n := z.npcs[0]
	teleport(z, n.e, x, zz)
	n.e.hp, n.e.maxHP = 5000, 5000
	return n
}

func newDuelZone(t *testing.T, npcKey string) *Zone {
	t.Helper()
	z := newMonsterZone(t, content.NPCGroup{NPC: npcKey, Count: 1, Center: [2]float64{30, 30}})
	// Keep the NPC from fighting back unless a test wants it to.
	z.npcs[0].def = &content.NPC{Key: "dummy", HP: 5000, Speed: 0.001, Stats: map[string]float64{"attack_range": 0, "aggro_radius": 0, "leash_radius": 1000, "wander_radius": 0},
		Attack: content.NPCAttack{Damage: content.Range{10, 10}, CooldownMS: 2000}, AI: z.npcs[0].def.AI}
	return z
}

func TestSwingHitsInFrontNotBehind(t *testing.T) {
	z := newDuelZone(t, "mountain_bandit")
	a := newActor(t, z, "sword_cultivator")
	dummy(t, z, 0, 2) // in front when facing +Z (yaw 0)
	a.tick(btnAttack, 0, 0, 0)
	a.ticks(12, 0, 0)
	if plain, _, _ := a.weaponHits(); plain != 1 {
		t.Fatalf("%d hits on the enemy in front, want 1", plain)
	}
	dummy(t, z, 0, -2) // now behind
	a.tick(btnAttack, 0, 0, 0)
	a.ticks(12, 0, 0)
	if plain, _, _ := a.weaponHits(); plain != 1 {
		t.Errorf("hit an enemy behind (total %d)", plain)
	}
}

func TestHoldingChargesAFlourish(t *testing.T) {
	z := newDuelZone(t, "mountain_bandit")
	a := newActor(t, z, "sword_cultivator")
	dummy(t, z, 0, 2)
	// Press and hold through the swing, then charge, then release.
	a.ticks(10+chargeTicks+1, btnAttack, 0)
	a.ticks(16, 0, 0)
	plain, flourish, _ := a.weaponHits()
	if plain != 1 || flourish != 1 {
		t.Fatalf("plain=%d flourish=%d, want 1 and 1", plain, flourish)
	}
	// A short hold after the swing does not flourish.
	a.ticks(10+chargeTicks-3, btnAttack, 0)
	a.ticks(16, 0, 0)
	if _, f, _ := a.weaponHits(); f != 1 {
		t.Errorf("a short hold made a flourish (%d)", f)
	}
}

func TestClickDuringSwingChainsAnother(t *testing.T) {
	z := newDuelZone(t, "mountain_bandit")
	a := newActor(t, z, "sword_cultivator")
	dummy(t, z, 0, 2)
	a.tick(btnAttack, 0, 0, 0)
	a.tick(0, 0, 0, 0)
	a.tick(btnAttack, 0, 0, 0) // clicked again mid-swing
	a.ticks(30, 0, 0)
	if plain, _, _ := a.weaponHits(); plain != 2 {
		t.Errorf("%d swings, want 2 (the second click is queued)", plain)
	}
}

func TestRollDodgesAndMovesFurther(t *testing.T) {
	z := newDuelZone(t, "mountain_bandit")
	a := newActor(t, z, "sword_cultivator")
	start := a.p.e.pos
	a.tick(btnRoll, 0, 0, 1)
	for range rollTicks - 1 {
		a.tick(0, 0, 0, 1)
	}
	want := z.cfg.MoveSpeed * rollSpeed * rollTicks / float64(z.cfg.TickRate)
	if got := a.p.e.pos[2] - start[2]; math.Abs(got-want) > 0.01 {
		t.Errorf("rolled %.2f m, want %.2f", got, want)
	}
	// An NPC strike landing during the invulnerable frames is dodged.
	n := z.npcs[0]
	teleport(z, n.e, a.p.e.pos[0], a.p.e.pos[2]+1)
	n.def.Stats["attack_range"] = 3
	a.ticks(rollCooldownTicks, 0, 0) // wait out the roll cooldown
	a.tick(btnRoll, 0, 0, -1)
	z.strikePlayer(n, a.p.e, 50)
	if eventsOf(a.conn, pb.CombatEventKind_COMBAT_EVENT_KIND_DODGED) == 0 {
		z.step()
	}
	z.step()
	if eventsOf(a.conn, pb.CombatEventKind_COMBAT_EVENT_KIND_DODGED) != 1 || a.p.e.hp != a.p.e.maxHP {
		t.Errorf("strike during a roll: dodged=%d hp=%d", eventsOf(a.conn, pb.CombatEventKind_COMBAT_EVENT_KIND_DODGED), a.p.e.hp)
	}
}

func TestBlockStopsFrontalHitsOnly(t *testing.T) {
	z := newDuelZone(t, "mountain_bandit")
	a := newActor(t, z, "sword_cultivator")
	n := z.npcs[0]
	a.ticks(2, btnBlock, 0) // facing +Z
	teleport(z, n.e, 0, 2)
	z.strikePlayer(n, a.p.e, 100)
	if got := a.p.e.maxHP - a.p.e.hp; got != int(100*blockFactor) {
		t.Errorf("blocked hit took %d, want %d", got, int(100*blockFactor))
	}
	a.p.e.hp = a.p.e.maxHP
	teleport(z, n.e, 0, -2) // from behind
	z.strikePlayer(n, a.p.e, 100)
	if got := a.p.e.maxHP - a.p.e.hp; got != 100 {
		t.Errorf("hit from behind took %d, want 100", got)
	}
}

func TestNPCStrikeIsTelegraphedAndCanBeOutrun(t *testing.T) {
	z := newMonsterZone(t, content.NPCGroup{NPC: "mountain_bandit", Count: 1, Center: [2]float64{0, 2}})
	a := newActor(t, z, "sword_cultivator")
	n := z.npcs[0]
	teleport(z, n.e, 0, 2)
	if !n.Attack(a.p.e.id) {
		t.Fatal("bandit could not start an attack in range")
	}
	if n.e.action != pb.ActionState_ACTION_STATE_WINDUP {
		t.Errorf("action %v, want WINDUP", n.e.action)
	}
	z.step()
	if a.p.e.hp != a.p.e.maxHP {
		t.Fatal("hit landed before the wind-up finished")
	}
	// Run away during the wind-up: the strike misses.
	for range 12 {
		a.tick(0, math.Pi, 0, -1)
	}
	if a.p.e.hp != a.p.e.maxHP {
		t.Errorf("strike landed on a target who ran %.1f m away", hdist(a.p.e.pos, n.e.pos))
	}
}

func TestCombatMultiplierBuildsAndBreaks(t *testing.T) {
	z := newDuelZone(t, "mountain_bandit")
	a := newActor(t, z, "sword_cultivator")
	dummy(t, z, 0, 2)
	for range 5 {
		a.tick(btnAttack, 0, 0, 0)
		a.ticks(11, 0, 0)
	}
	if a.p.combo != 5 || math.Abs(a.p.multiplier()-1.5) > 1e-9 {
		t.Fatalf("combo %d multiplier %v, want 5 and 1.5", a.p.combo, a.p.multiplier())
	}
	exp := a.p.exp
	z.grantExp(a.p, 10)
	if a.p.exp-exp != 15 {
		t.Errorf("10 exp at x1.5 gave %d", a.p.exp-exp)
	}
	z.strikePlayer(z.npcs[0], a.p.e, 5)
	if a.p.combo != 0 {
		t.Error("taking a hit did not reset the multiplier")
	}
}

func TestShotHitsFirstEnemyAlongTheAimAndLockOn(t *testing.T) {
	z := newMonsterZone(t,
		content.NPCGroup{NPC: "mountain_bandit", Count: 1, Center: [2]float64{0, 8}},
		content.NPCGroup{NPC: "mountain_bandit", Count: 1, Center: [2]float64{0, 14}},
	)
	near, far := z.npcs[0], z.npcs[1]
	for _, n := range z.npcs {
		n.def = &content.NPC{Key: "dummy", HP: 5000, Speed: 0.001, Stats: map[string]float64{"attack_range": 0, "aggro_radius": 0, "leash_radius": 1000, "wander_radius": 0},
			Attack: content.NPCAttack{Damage: content.Range{1, 1}, CooldownMS: 2000}, AI: n.def.AI}
	}
	teleport(z, near.e, 0, 8)
	teleport(z, far.e, 0.3, 14)
	a := newActor(t, z, "rail_gunner")
	z.step()
	z.step()
	z.step()
	z.step()
	a.tick(btnAttack, 0, 0, 0)
	a.ticks(13, 0, 0)
	if near.e.hp == near.e.maxHP || far.e.hp != far.e.maxHP {
		t.Fatalf("shot: near hp %d, far hp %d (the near one blocks the line)", near.e.hp, far.e.hp)
	}
	// Locked on to the far one, slightly off the aim line: it is aimed at.
	a.seq++
	a.p.pending = append(a.p.pending, &pb.MoveInput{Seq: a.seq, Yaw: float32(faceYaw(3, 14)), Buttons: btnAttack, LockTarget: far.e.id})
	z.step()
	a.ticks(13, 0, faceYaw(3, 14))
	if far.e.hp == far.e.maxHP {
		t.Error("locked target was not hit")
	}
}

func TestMeleeUsesRewoundPositions(t *testing.T) {
	z := newDuelZone(t, "mountain_bandit")
	a := newActor(t, z, "sword_cultivator")
	n := dummy(t, z, 0, 2)
	a.ticks(historyLen, 0, 0) // fill the history at the in-front position
	a.tick(btnAttack, 0, 0, 0)
	// The NPC steps out of reach right before the hit; the client still saw
	// it in front, so the hit counts.
	for range 3 {
		a.ticks(1, 0, 0)
	}
	teleport(z, n.e, 0, 4)
	a.ticks(10, 0, 0)
	if plain, _, _ := a.weaponHits(); plain != 1 {
		t.Errorf("hit with lag compensation: %d, want 1", plain)
	}
}
