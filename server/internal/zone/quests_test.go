package zone

import (
	"testing"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/model"
)

func newVillageZone(t *testing.T) (*Zone, *npc) {
	t.Helper()
	z := newMonsterZone(t,
		content.NPCGroup{NPC: "elder_mei", Count: 1, Center: [2]float64{2, 0}},
		content.NPCGroup{NPC: "mountain_bandit", Count: 6, Center: [2]float64{40, 40}, Radius: 2},
		content.NPCGroup{NPC: "villager", Count: 1, Center: [2]float64{-3, 0}},
	)
	return z, z.npcs[0]
}

func lastDialog(c *fakeConn) *pb.QuestDialog {
	for i := len(c.msgs) - 1; i >= 0; i-- {
		if d := c.msgs[i].GetQuestDialog(); d != nil {
			return d
		}
	}
	return nil
}

func lastLog(c *fakeConn) *pb.QuestLog {
	for i := len(c.msgs) - 1; i >= 0; i-- {
		if l := c.msgs[i].GetQuestLog(); l != nil {
			return l
		}
	}
	return nil
}

func TestQuestAcceptProgressAndComplete(t *testing.T) {
	z, mei := newVillageZone(t)
	p, c := joinClass(t, z, "sword_cultivator")
	z.interact(p, mei.e.id)
	d := lastDialog(c)
	if d == nil || len(d.Offers) < 2 {
		t.Fatalf("dialog = %+v, want offers", d)
	}
	for _, o := range d.Offers {
		if o.Key == "stolen_herbs" {
			t.Error("offered a quest whose requirement is not done")
		}
	}
	z.questAction(p, &pb.QuestAction{Quest: "bandit_trouble", Action: "accept", NpcId: mei.e.id})
	if p.quests["bandit_trouble"].State != questActive {
		t.Fatal("quest not accepted")
	}
	for range 5 {
		z.questKill(p, "mountain_bandit")
	}
	z.questKill(p, "mountain_bandit") // extra kills do not overflow
	if l := lastLog(c); l.Quests[0].State != "ready" || l.Quests[0].Objectives[0].Progress != 5 {
		t.Errorf("log = %+v", l.Quests[0])
	}
	renown, exp := p.renown, p.exp
	z.questAction(p, &pb.QuestAction{Quest: "bandit_trouble", Action: "complete", NpcId: mei.e.id})
	if p.quests["bandit_trouble"].State != questStateDone || p.renown != renown+30 || p.exp <= exp || p.inventory["jade_token"] != 1 {
		t.Errorf("rewards: state=%v renown %d->%d exp %d->%d inv %v", p.quests["bandit_trouble"].State, renown, p.renown, exp, p.exp, p.inventory)
	}
	// Now the follow-up is offered.
	z.interact(p, mei.e.id)
	found := false
	for _, o := range lastDialog(c).Offers {
		found = found || o.Key == "stolen_herbs"
	}
	if !found {
		t.Error("follow-up quest not offered after its requirement")
	}
}

func TestChoiceQuestGoodAndEvil(t *testing.T) {
	for _, tc := range []struct {
		choice string
		karma  int
	}{{"return", 15}, {"sell", -20}} {
		z, mei := newVillageZone(t)
		p, _ := joinClass(t, z, "mystic")
		p.quests["bandit_trouble"] = questDone()
		z.questAction(p, &pb.QuestAction{Quest: "stolen_herbs", Action: "accept", NpcId: mei.e.id})
		for range 3 {
			z.questKill(p, "shadow_assassin")
		}
		z.questAction(p, &pb.QuestAction{Quest: "stolen_herbs", Action: "complete", NpcId: mei.e.id})
		if p.quests["stolen_herbs"].State == questStateDone {
			t.Fatal("completed a choice quest without choosing")
		}
		z.questAction(p, &pb.QuestAction{Quest: "stolen_herbs", Action: "complete", Choice: tc.choice, NpcId: mei.e.id})
		if p.karma != tc.karma {
			t.Errorf("%s: karma %d, want %d", tc.choice, p.karma, tc.karma)
		}
	}
}

func TestQuestNeedsTheGiverNearby(t *testing.T) {
	z, mei := newVillageZone(t)
	p, _ := joinClass(t, z, "rail_gunner")
	teleport(z, p.e, 30, 0)
	z.questAction(p, &pb.QuestAction{Quest: "bandit_trouble", Action: "accept", NpcId: mei.e.id})
	if _, ok := p.quests["bandit_trouble"]; ok {
		t.Error("accepted a quest from across the map")
	}
}

func TestReachObjective(t *testing.T) {
	z, mei := newVillageZone(t)
	p, c := joinClass(t, z, "rail_gunner")
	z.questAction(p, &pb.QuestAction{Quest: "lights_by_the_lake", Action: "accept", NpcId: mei.e.id})
	teleport(z, p.e, 12, 36)
	steps(z, 10)
	if l := lastLog(c); l == nil || l.Quests[0].Objectives[0].Progress != 1 {
		t.Errorf("reach objective not completed: %+v", l)
	}
}

func TestKillingVillagersIsWicked(t *testing.T) {
	z, _ := newVillageZone(t)
	p, _ := joinClass(t, z, "sword_cultivator")
	v := z.npcs[len(z.npcs)-1]
	exp := p.exp
	z.damage(p.e, v.e, 0, 1000, false)
	if p.karma != friendlyKarma || p.exp != exp {
		t.Errorf("karma %d exp %d->%d after killing a villager", p.karma, exp, p.exp)
	}
}

func TestRenownTitlesAndVillagerReactions(t *testing.T) {
	z, _ := newVillageZone(t)
	p, c := joinClass(t, z, "sword_cultivator")
	v := z.npcs[len(z.npcs)-1]
	teleport(z, p.e, -3, 2) // next to the villager
	p.renown, p.karma = 400, 50
	z.chooseTitle(p, "hero")
	if z.titleName(p) != "Hero of the Terraces" {
		t.Fatalf("title %q", z.titleName(p))
	}
	z.chooseTitle(p, "dreaded") // not unlocked for a good hero
	if p.title != "hero" {
		t.Error("chose a locked title")
	}
	steps(z, 20)
	if v.e.action != pb.ActionState_ACTION_STATE_CHEER {
		t.Errorf("villager action %v, want CHEER for a famous good hero", v.e.action)
	}
	spoke := false
	for _, m := range c.msgs {
		if s := m.GetSnapshot(); s != nil && len(s.Speech) > 0 {
			spoke = true
		}
	}
	if !spoke {
		t.Error("villager never spoke")
	}
	// Turned wicked: the hero title no longer shows, and villagers cower.
	p.karma = -60
	steps(z, 20)
	if z.titleName(p) != "" || v.e.action != pb.ActionState_ACTION_STATE_COWER {
		t.Errorf("wicked: title %q villager %v", z.titleName(p), v.e.action)
	}
}

func questDone() model.QuestState { return model.QuestState{State: questStateDone} }
