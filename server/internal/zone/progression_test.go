package zone

import (
	"testing"

	pb "convergence/server/gen/convergence/v1"
)

func TestDamageEarnsDisciplineXPByWeapon(t *testing.T) {
	z := newDuelZone(t, "mountain_bandit")
	sword := newActor(t, z, "sword_cultivator")
	dummy(t, z, 0, 2)
	sword.tick(btnAttack, 0, 0, 0)
	sword.ticks(12, 0, 0)
	if sword.p.prog.XP["strength"] <= 0 || sword.p.prog.XP["skill"] != 0 {
		t.Errorf("sword xp = %v, want strength only", sword.p.prog.XP)
	}
	mystic := newActor(t, z, "mystic")
	teleport(z, mystic.p.e, 0, 10)
	useSkillKey(z, mystic.p, "spirit_lance", z.npcs[0].e.id)
	if mystic.p.prog.XP["will"] <= 0 {
		t.Errorf("mystic xp = %v, want will", mystic.p.prog.XP)
	}
}

func TestAffinityMakesBuildsDiverge(t *testing.T) {
	z := newClassZone(t, 0, 0)
	p, _ := joinClass(t, z, "sword_cultivator")
	d := z.content.Disciplines
	phys, acc := d.Attributes["physique"], d.Attributes["accuracy"]
	if z.nextCost(p, acc) <= z.nextCost(p, phys) {
		t.Errorf("off-affinity cost %d not above affinity cost %d", z.nextCost(p, acc), z.nextCost(p, phys))
	}
	if z.attrCap(p, acc) >= z.attrCap(p, phys) {
		t.Error("off-affinity cap is not lower")
	}
	p.prog.XP["skill"] = 1_000_000
	for range 30 {
		z.raiseAttribute(p, "accuracy")
	}
	if got := p.prog.Ranks["accuracy"]; got != d.Caps.Other {
		t.Errorf("accuracy rank %d, want capped at %d", got, d.Caps.Other)
	}
	// Not enough experience: nothing happens.
	p.prog.XP["strength"] = 10
	z.raiseAttribute(p, "physique")
	if p.prog.Ranks["physique"] != 0 || p.prog.XP["strength"] != 10 {
		t.Error("raised without enough experience")
	}
}

func TestToughnessAndSpecialization(t *testing.T) {
	z := newClassZone(t, 0, 0)
	p, c := joinClass(t, z, "sword_cultivator")
	base := p.e.maxHP
	p.prog.XP["strength"] = 1_000_000
	z.raiseAttribute(p, "toughness")
	if p.e.maxHP <= base || p.e.hp != p.e.maxHP {
		t.Errorf("toughness: hp %d/%d (base %d)", p.e.hp, p.e.maxHP, base)
	}
	// Too early for a specialization.
	z.chooseSpec(p, "iron_monk")
	if p.prog.Spec != "" {
		t.Fatal("chose a specialization below the unlock ranks")
	}
	for range z.content.Disciplines.SpecUnlockRanks {
		z.raiseAttribute(p, "physique")
	}
	var offered *pb.Progression
	for _, m := range c.msgs {
		if pr := m.GetProgression(); pr != nil {
			offered = pr
		}
	}
	if offered == nil || len(offered.SpecOptions) != 2 {
		t.Fatalf("no specialization offered: %+v", offered)
	}
	hpBefore := p.e.maxHP
	z.chooseSpec(p, "iron_monk")
	if p.prog.Spec != "iron_monk" || p.e.maxHP <= hpBefore {
		t.Errorf("spec %q maxHP %d (before %d)", p.prog.Spec, p.e.maxHP, hpBefore)
	}
	if !z.hasSkill(p, z.content.SkillByKey("mountain_stance").ID) || len(z.skillInfos(p)) != 5 {
		t.Error("specialization skill not granted")
	}
	z.chooseSpec(p, "blade_saint")
	if p.prog.Spec != "iron_monk" {
		t.Error("specialization changed after being chosen")
	}
	// The build is saved with the character.
	if rec := z.record(p); rec.Progression.Spec != "iron_monk" || rec.Progression.Ranks["toughness"] != 1 {
		t.Errorf("record = %+v", rec.Progression)
	}
}

func TestAgilitySpeedsUpMovement(t *testing.T) {
	z := newClassZone(t, 0, 0)
	p, _ := joinClass(t, z, "rail_gunner")
	base := z.moveSpeed(p)
	p.prog.XP["skill"] = 1_000_000
	for range 5 {
		z.raiseAttribute(p, "agility")
	}
	if z.moveSpeed(p) <= base {
		t.Error("agility did not raise movement speed")
	}
}
