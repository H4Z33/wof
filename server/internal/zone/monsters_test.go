package zone

import (
	"testing"

	"convergence/server/internal/content"
)

// newMonsterZone has the given NPC groups on flat ground and no Fate.
func newMonsterZone(t *testing.T, groups ...content.NPCGroup) *Zone {
	t.Helper()
	c := loadContent(t)
	c.Fate = nil
	z, err := New(testConfig(), c, &content.Zone{ID: "test", HalfExtent: 60, NPCGroups: groups}, nil, quiet)
	if err != nil {
		t.Fatal(err)
	}
	return z
}

func runTicks(z *Zone, n int, each func()) {
	for range n {
		z.step()
		if each != nil {
			each()
		}
	}
}

func TestWispKeepsItsDistanceAndShoots(t *testing.T) {
	z := newMonsterZone(t, content.NPCGroup{NPC: "jade_wisp", Count: 1, Center: [2]float64{4, 0}})
	p, _ := joinTest(t, z, "alice")
	p.e.hp, p.e.maxHP = 10000, 10000
	w := z.npcs[0]
	runTicks(z, 100, nil)
	if d := hdist(w.e.pos, p.e.pos); d < w.def.Stats["keep_away"]-1 {
		t.Errorf("wisp stayed %.1f m away, wants at least %.0f", d, w.def.Stats["keep_away"])
	}
	if p.e.hp == p.e.maxHP {
		t.Error("wisp never hit from range")
	}
}

func TestAssassinStrikesThenRetreats(t *testing.T) {
	z := newMonsterZone(t, content.NPCGroup{NPC: "shadow_assassin", Count: 1, Center: [2]float64{3, 0}})
	p, _ := joinTest(t, z, "alice")
	p.e.hp, p.e.maxHP = 10000, 10000
	a := z.npcs[0]
	maxGap, hits := 0.0, 0
	last := p.e.hp
	runTicks(z, 200, func() {
		if p.e.hp < last {
			hits++
			last = p.e.hp
		}
		if hits >= 2 {
			maxGap = max(maxGap, hdist(a.e.pos, p.e.pos))
		}
	})
	if hits < 3 {
		t.Errorf("assassin landed %d hits in 10 s", hits)
	}
	if maxGap < 4 {
		t.Errorf("assassin never darted away (max gap %.1f m)", maxGap)
	}
}

func TestGuardianSlamsEveryoneAround(t *testing.T) {
	z := newMonsterZone(t, content.NPCGroup{NPC: "stone_guardian", Count: 1, Center: [2]float64{2, 0}})
	a, _ := joinTest(t, z, "alice")
	b, _ := joinTest(t, z, "bob")
	teleport(z, b.e, 2, 2.5) // close to the guardian, but it is fighting alice
	for _, p := range []*player{a, b} {
		p.e.hp, p.e.maxHP = 10000, 10000
	}
	runTicks(z, 400, nil)
	if b.e.hp == b.e.maxHP {
		t.Error("bystander never caught in a slam")
	}
	g := z.npcs[0]
	g.e.hp = int(float64(g.e.maxHP) * 0.3)
	runTicks(z, 10, nil)
	if dealt, _ := g.e.multipliers(z.tick.Load()); dealt <= 1 {
		t.Error("guardian did not enrage at low health")
	}
}

func TestElderRalliesTheCamp(t *testing.T) {
	z := newMonsterZone(t,
		content.NPCGroup{NPC: "rogue_elder", Count: 1, Center: [2]float64{6, 0}},
		content.NPCGroup{NPC: "mountain_bandit", Count: 3, Center: [2]float64{18, 0}, Radius: 1},
	)
	p, _ := joinTest(t, z, "alice")
	p.e.hp, p.e.maxHP = 10000, 10000
	runTicks(z, 40, nil)
	joined := 0
	for _, n := range z.npcs[1:] {
		if n.target == p.e.id {
			joined++
		}
	}
	// The bandits stand 12 m away: outside their own 8 m aggro, inside the rally radius.
	if joined != 3 {
		t.Errorf("%d of 3 bandits joined the elder's fight", joined)
	}
}
