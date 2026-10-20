package zone

import (
	"testing"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/fate"
)

// fakeDirector records deals and answers them only when told to, like a
// slow agent.
type fakeDirector struct {
	reqs    []fate.DeckRequest
	replies []func(fate.Deck)
}

func (d *fakeDirector) Deal(req fate.DeckRequest, reply func(fate.Deck)) bool {
	d.reqs = append(d.reqs, req)
	d.replies = append(d.replies, reply)
	return true
}

func newFateZone(t *testing.T) (*Zone, *fakeDirector) {
	t.Helper()
	zc := &content.Zone{ID: "test", HalfExtent: 60, NPCGroups: []content.NPCGroup{
		{NPC: "mountain_bandit", Count: 1, Center: [2]float64{40, 40}},
	}}
	z, err := New(testConfig(), loadContent(t), zc, nil, quiet)
	if err != nil {
		t.Fatal(err)
	}
	d := &fakeDirector{}
	z.SetFate(d)
	return z, d
}

func fateEvents(c *fakeConn) []*pb.FateEvent {
	var out []*pb.FateEvent
	for _, m := range c.msgs {
		if f := m.GetFate(); f != nil {
			out = append(out, f)
		}
	}
	return out
}

func TestDeckIsPrefetchedOnJoinAndCoversTheCatalog(t *testing.T) {
	z, d := newFateZone(t)
	p, _ := joinTest(t, z, "alice")
	if len(d.reqs) != 1 {
		t.Fatalf("%d deals on join, want 1", len(d.reqs))
	}
	req := d.reqs[0]
	if len(req.Slots) != z.content.Fate.Deck.Size {
		t.Errorf("%d slots, want %d", len(req.Slots), z.content.Fate.Deck.Size)
	}
	for _, s := range req.Slots {
		if !z.slotFillable(req.Actions, s) {
			t.Errorf("slot %+v has no allowed action", s)
		}
	}
	// The agent answers later; the zone keeps running meanwhile.
	steps(z, 10)
	d.replies[0](fate.FallbackDeck(req))
	z.handleCmd(<-z.inbox)
	if len(p.deck) != len(req.Slots) {
		t.Errorf("deck has %d cards after the deal, want %d", len(p.deck), len(req.Slots))
	}
}

func TestTriggerDrawsMatchingCardAndAppliesIt(t *testing.T) {
	z, _ := newFateZone(t)
	p, c := joinTest(t, z, "alice")
	z.content.Fate.Triggers[fate.TriggerRest] = content.FateTrigger{Chance: 1, AfterS: 0.1}
	p.fateReady = 0
	z.content.Fate.FortuneOdds.Min, z.content.Fate.FortuneOdds.Max = 1, 1 // always fortune
	p.e.hp = 10
	// Only blessing cards in the deck, for every magnitude.
	p.deck = nil
	for mag := 1; mag <= 3; mag++ {
		p.deck = append(p.deck, fate.Card{
			Slot: fate.Slot{Trigger: fate.TriggerRest, Omen: fate.Fortune, Magnitude: mag}, Action: "blessing",
			Narration: "A crane cries.", Source: "agent",
		})
	}
	p.dealPending = true // keep the test deck
	move(p, 1, 1, 0)
	z.step()
	steps(z, 10) // stand still long enough to rest
	evs := fateEvents(c)
	if len(evs) != 1 {
		t.Fatalf("%d fate events, want 1", len(evs))
	}
	if evs[0].Action != "blessing" || evs[0].Narration != "A crane cries." {
		t.Errorf("event = %+v", evs[0])
	}
	if p.e.hp != p.e.maxHP {
		t.Errorf("hp %d/%d after blessing", p.e.hp, p.e.maxHP)
	}
	if len(p.deck) != 2 {
		t.Errorf("deck has %d cards, want 2 (one drawn)", len(p.deck))
	}
}

func TestCriticalAndWeaknessChangeDamage(t *testing.T) {
	z, _ := newFateZone(t)
	p, _ := joinTest(t, z, "alice")
	n := z.npcs[0]
	teleport(z, n.e, 2, 0)
	a := z.content.Fate.Actions

	mult, miss := z.applyCard(p, fate.Card{Slot: fate.Slot{Trigger: "strike", Omen: fate.Fortune, Magnitude: 2}, Action: "critical"}, n.e)
	if mult != 3 || miss {
		t.Errorf("critical: mult=%v miss=%v", mult, miss)
	}
	_, miss = z.applyCard(p, fate.Card{Slot: fate.Slot{Trigger: "strike", Omen: fate.Misfortune, Magnitude: 1}, Action: "fumble"}, n.e)
	if !miss {
		t.Error("fumble did not miss")
	}

	z.applyCard(p, fate.Card{Slot: fate.Slot{Trigger: "strike", Omen: fate.Fortune, Magnitude: 2}, Action: "weakness"}, n.e)
	_, taken := n.e.multipliers(z.tick.Load())
	if want := 1 + 2*a["weakness"].BonusPerMagnitude; taken != want {
		t.Errorf("weakened target takes x%v, want x%v", taken, want)
	}
	before := n.e.hp
	z.damage(p.e, n.e, 1, 10, false)
	if got := before - n.e.hp; got != int(10*taken+0.5) {
		t.Errorf("damage %d, want %d", got, int(10*taken+0.5))
	}
	// Effects wear off.
	steps(z, int(z.ticks(int(a["weakness"].DurationS*1000)))+1)
	if _, taken := n.e.multipliers(z.tick.Load()); taken != 1 {
		t.Errorf("weakness still active: x%v", taken)
	}
}

func TestHiddenCacheIsClaimedByWalkingToIt(t *testing.T) {
	z, _ := newFateZone(t)
	p, c := joinTest(t, z, "alice")
	z.applyCard(p, fate.Card{Slot: fate.Slot{Trigger: "travel", Omen: fate.Fortune, Magnitude: 2}, Action: "hidden_cache",
		ItemKey: "jade_token", Narration: "A glimmer among the rocks."}, nil)
	if len(z.caches) != 1 {
		t.Fatalf("%d caches, want 1", len(z.caches))
	}
	cacheEnt := z.caches[0]
	z.step()
	if !p.known[cacheEnt.id] {
		t.Error("cache not replicated to its owner")
	}
	teleport(z, p.e, cacheEnt.pos[0]+1, cacheEnt.pos[2])
	z.step()
	if p.inventory["jade_token"] != 1 || len(z.caches) != 0 {
		t.Errorf("inventory=%v caches=%d", p.inventory, len(z.caches))
	}
	evs := fateEvents(c)
	if last := evs[len(evs)-1]; last.Action != "cache_found" || last.ItemName != "Jade Token" {
		t.Errorf("last event = %+v", last)
	}
}

func TestAmbushSpawnsTemporaryHunters(t *testing.T) {
	z, _ := newFateZone(t)
	p, _ := joinTest(t, z, "alice")
	before := len(z.npcs)
	z.applyCard(p, fate.Card{Slot: fate.Slot{Trigger: "travel", Omen: fate.Misfortune, Magnitude: 3}, Action: "ambush"}, nil)
	if len(z.npcs) != before+3 {
		t.Fatalf("%d npcs, want %d", len(z.npcs), before+3)
	}
	for _, n := range z.npcs[before:] {
		if !n.temporary || n.target != p.e.id {
			t.Errorf("ambusher temporary=%v target=%d", n.temporary, n.target)
		}
	}
	steps(z, int(z.ticks(90_000))+1)
	if len(z.npcs) != before {
		t.Errorf("ambushers did not vanish: %d npcs", len(z.npcs))
	}
}

func TestMishapIsNeverLethal(t *testing.T) {
	z, _ := newFateZone(t)
	p, _ := joinTest(t, z, "alice")
	p.e.hp = 3
	z.applyCard(p, fate.Card{Slot: fate.Slot{Trigger: "travel", Omen: fate.Misfortune, Magnitude: 3}, Action: "mishap"}, nil)
	if p.e.hp != 1 {
		t.Errorf("hp = %d, want 1", p.e.hp)
	}
}

func TestLuckTiltsTheOdds(t *testing.T) {
	z, _ := newFateZone(t)
	lucky, _ := joinTest(t, z, "lucky")
	cursed, _ := joinTest(t, z, "cursed")
	lucky.luck, cursed.luck = 18, 4
	count := func(p *player) (fortunes, bigFortunes int) {
		for range 4000 {
			omen, mag := z.rollOmen(p, p.entropy.Float)
			if omen == fate.Fortune {
				fortunes++
				if mag == 3 {
					bigFortunes++
				}
			}
		}
		return
	}
	lf, lb := count(lucky)
	cf, cb := count(cursed)
	if lf <= cf || lb <= cb {
		t.Errorf("lucky %d fortunes (%d major) vs cursed %d (%d major)", lf, lb, cf, cb)
	}
}
