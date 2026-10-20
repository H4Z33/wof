package zone

import (
	"testing"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/model"
)

// newClassZone has n bandits standing in a tight group around (x, 0).
func newClassZone(t *testing.T, bandits int, x float64) *Zone {
	t.Helper()
	zc := &content.Zone{ID: "test", HalfExtent: 60, NPCGroups: []content.NPCGroup{
		{NPC: "mountain_bandit", Count: bandits, Center: [2]float64{x, 0}, Radius: 1},
	}}
	c := loadContent(t)
	c.Fate = nil // no fate surprises in skill tests
	z, err := New(testConfig(), c, zc, nil, quiet)
	if err != nil {
		t.Fatal(err)
	}
	return z
}

func joinClass(t *testing.T, z *Zone, class string) (*player, *fakeConn) {
	t.Helper()
	nextCharID++
	return joinChar(t, z, model.Character{ID: nextCharID, Name: class, Class: class, ZoneID: z.cfg.ID, Level: 1}) // HP 0: join at full health
}

func useSkillKey(z *Zone, p *player, skill string, target uint32) {
	z.useSkill(p, &pb.UseSkill{SkillId: z.content.SkillByKey(skill).ID, TargetId: target})
}

func countEvents(c *fakeConn, kind pb.CombatEventKind) int {
	n := 0
	for _, ev := range c.events() {
		if ev.Kind == kind {
			n++
		}
	}
	return n
}

func TestClassSetsHPResourceAndSkills(t *testing.T) {
	z := newClassZone(t, 0, 0)
	p, c := joinClass(t, z, "rail_gunner")
	w := c.msgs[0].GetWelcome()
	if w.ClassInfo.GetKey() != "rail_gunner" || len(w.Skills) != 4 || w.Skills[0].Name != "Piston Shot" {
		t.Fatalf("welcome class=%v skills=%v", w.ClassInfo, w.Skills)
	}
	if p.e.maxHP != p.class.MaxHP(1) || p.resource != 0 {
		t.Errorf("hp=%d resource=%v", p.e.maxHP, p.resource)
	}
	// A Sword Cultivator skill is not usable by a Rail Gunner.
	useSkillKey(z, p, "jade_slash", 0)
	if r := c.rejections(); len(r) != 1 || r[0] != "unknown_skill" {
		t.Errorf("rejections = %v", r)
	}
}

func TestAreaSkillCostsResourceAndHitsTheGroup(t *testing.T) {
	z := newClassZone(t, 3, 5)
	p, c := joinClass(t, z, "sword_cultivator")
	for _, n := range z.npcs {
		teleport(z, n.e, 5, 0) // all within the 4 m radius
	}
	useSkillKey(z, p, "sword_qi_wave", z.npcs[0].e.id)
	if p.resource != 100-35 {
		t.Errorf("qi = %v, want 65", p.resource)
	}
	hurt := 0
	for _, n := range z.npcs {
		if n.e.hp < n.e.maxHP {
			hurt++
		}
	}
	if hurt != 3 {
		t.Errorf("%d bandits hit, want 3", hurt)
	}
	p.resource = 10
	p.cooldowns = map[uint32]uint32{}
	useSkillKey(z, p, "sword_qi_wave", z.npcs[0].e.id)
	if r := c.rejections(); len(r) != 1 || r[0] != "not_enough_resource" {
		t.Errorf("rejections = %v", r)
	}
}

func TestScriptedSkills(t *testing.T) {
	z := newClassZone(t, 1, 2)
	n := z.npcs[0]
	n.e.hp, n.e.maxHP = 500, 500

	sword, sc := joinClass(t, z, "sword_cultivator")
	useSkillKey(z, sword, "flowing_strikes", n.e.id)
	if got := countEvents(sc, 0); got != 0 { // events go out with the next snapshot
		t.Fatal("events sent early")
	}
	z.step()
	if got := countEvents(sc, pb.CombatEventKind_COMBAT_EVENT_KIND_DAMAGE); got < 3 {
		t.Errorf("flowing strikes: %d damage events, want 3", got)
	}

	gun, gc := joinClass(t, z, "rail_gunner")
	useSkillKey(z, gun, "overcharge_shot", n.e.id)
	if r := gc.rejections(); len(r) != 1 || r[0] != "not_enough_resource" {
		t.Fatalf("overcharge with no charge: rejections %v", r)
	}
	gun.resource = 100
	before := n.e.hp
	useSkillKey(z, gun, "overcharge_shot", n.e.id)
	if gun.resource != 0 {
		t.Errorf("charge left %v, want all spent", gun.resource)
	}
	// Base 10-14 tripled at full charge.
	if dealt := before - n.e.hp; dealt < 30 || dealt > 42 {
		t.Errorf("overcharge dealt %d, want 30-42", dealt)
	}
}

func TestChargeBuildsOnHitAndDrainsOutOfCombat(t *testing.T) {
	z := newClassZone(t, 1, 10)
	z.npcs[0].e.hp, z.npcs[0].e.maxHP = 500, 500
	p, _ := joinClass(t, z, "rail_gunner")
	useSkillKey(z, p, "piston_shot", z.npcs[0].e.id)
	if p.resource != 20 {
		t.Fatalf("charge after a hit = %v, want 20", p.resource)
	}
	steps(z, int(z.ticks(5000))+40) // 5 s grace, then 2 s of decay
	if p.resource >= 20 || p.resource <= 0 {
		t.Errorf("charge = %v, want partly drained", p.resource)
	}
}

func TestStunBurnSlowWardAndRegen(t *testing.T) {
	z := newClassZone(t, 1, 2)
	n := z.npcs[0]
	n.e.hp, n.e.maxHP = 500, 500
	mystic, mc := joinClass(t, z, "mystic")
	gun, _ := joinClass(t, z, "rail_gunner")
	gun.resource = 100

	useSkillKey(z, mystic, "divine_rebuke", n.e.id)
	if !n.e.stunned(z.tick.Load()) || n.Attack(mystic.e.id) {
		t.Error("stunned bandit can still attack")
	}
	useSkillKey(z, gun, "arc_grenade", n.e.id)
	hp := n.e.hp
	steps(z, 40) // 2 s of burning
	if n.e.hp >= hp {
		t.Error("burning did no damage")
	}
	useSkillKey(z, gun, "stasis_coil", n.e.id)
	if f := n.e.speedFactor(z.tick.Load()); f != 0.5 {
		t.Errorf("slowed speed factor %v", f)
	}

	sword, _ := joinClass(t, z, "sword_cultivator")
	useSkillKey(z, sword, "iron_meditation", 0)
	if _, taken := sword.e.multipliers(z.tick.Load()); taken != 0.6 {
		t.Errorf("ward taken multiplier %v", taken)
	}

	// Blessing heals the mystic and a nearby ally, then keeps healing.
	mystic.e.hp, sword.e.hp = 20, 20
	useSkillKey(z, mystic, "blessing_of_dawn", 0)
	if mystic.e.hp <= 20 || sword.e.hp <= 20 {
		t.Errorf("blessing: mystic %d, ally %d", mystic.e.hp, sword.e.hp)
	}
	z.step()
	if countEvents(mc, pb.CombatEventKind_COMBAT_EVENT_KIND_HEAL) < 2 {
		t.Error("no heal events")
	}
	if mystic.e.effectFlags(z.tick.Load())&uint32(pb.EffectFlag_EFFECT_FLAG_REGEN) == 0 {
		t.Error("no regen effect")
	}
}

func TestReloadChangesSkillNumbersAndRejectsBrokenScripts(t *testing.T) {
	z := newClassZone(t, 1, 2)
	p, _ := joinClass(t, z, "sword_cultivator")

	c2 := loadContent(t)
	c2.Fate = nil
	c2.SkillByKey("jade_slash").Damage = content.Range{100, 100}
	z.applyReload(c2)
	n := z.npcs[0]
	n.e.hp, n.e.maxHP = 1000, 1000
	useSkillKey(z, p, "jade_slash", n.e.id)
	if dealt := 1000 - n.e.hp; dealt != 100 {
		t.Errorf("after reload jade slash dealt %d, want 100", dealt)
	}
	if p.class != c2.Classes["sword_cultivator"] {
		t.Error("player still points at the old class")
	}

	broken := loadContent(t)
	broken.SkillByKey("flowing_strikes").ScriptSource = "this is not lua"
	z.applyReload(broken)
	if z.content != c2 {
		t.Error("broken content was applied")
	}
}
