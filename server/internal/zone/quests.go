package zone

import (
	"fmt"
	"slices"
	"sort"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/model"
)

// Quests from quest givers (content/quests). Objectives count kills of an
// NPC kind or reaching a place; turning in grants rewards, and some quests
// end with a good or evil choice.

const (
	interactRange  = 4.0
	questStateDone = "done"
	questActive    = "active"
	friendlyKarma  = -10 // killing a villager
	speechCooldown = 60  // ticks between lines from the same NPC (3 s)
	maxSpeechLen   = 140
)

func cloneQuests(q map[string]model.QuestState) map[string]model.QuestState {
	out := make(map[string]model.QuestState, len(q))
	for k, v := range q {
		out[k] = model.QuestState{State: v.State, Progress: slices.Clone(v.Progress)}
	}
	return out
}

func (z *Zone) questComplete(p *player, q *content.Quest) bool {
	st, ok := p.quests[q.Key]
	if !ok || st.State != questActive {
		return false
	}
	for i, o := range q.Objectives {
		if i >= len(st.Progress) || st.Progress[i] < o.Target() {
			return false
		}
	}
	return true
}

func (z *Zone) questAvailable(p *player, q *content.Quest) bool {
	if _, taken := p.quests[q.Key]; taken {
		return false
	}
	for _, r := range q.Requires {
		if p.quests[r].State != questStateDone {
			return false
		}
	}
	return true
}

func (z *Zone) questInfo(p *player, q *content.Quest, text string) *pb.QuestInfo {
	info := &pb.QuestInfo{Key: q.Key, Name: q.Name, Summary: q.Summary, Text: text, Giver: q.Giver}
	st, taken := p.quests[q.Key]
	switch {
	case !taken:
		info.State = ""
	case st.State == questStateDone:
		info.State = questStateDone
	case z.questComplete(p, q):
		info.State = "ready"
	default:
		info.State = questActive
	}
	for i, o := range q.Objectives {
		oi := &pb.QuestObjectiveInfo{Target: uint32(o.Target())}
		if i < len(st.Progress) {
			oi.Progress = uint32(min(st.Progress[i], o.Target()))
		}
		switch {
		case o.Label != "":
			oi.Text = o.Label
		case o.IsReach():
			oi.Text = "Reach the place"
		default:
			name := o.Kill
			if def := z.content.NPCs[o.Kill]; def != nil {
				name = def.Name
			}
			oi.Text = fmt.Sprintf("Defeat %s", name)
		}
		info.Objectives = append(info.Objectives, oi)
	}
	for _, ch := range q.Choices {
		info.Choices = append(info.Choices, &pb.QuestChoiceInfo{Key: ch.Key, Text: ch.Text})
	}
	return info
}

func (z *Zone) sortedQuests() []*content.Quest {
	qs := make([]*content.Quest, 0, len(z.content.Quests))
	for _, q := range z.content.Quests {
		qs = append(qs, q)
	}
	sort.Slice(qs, func(i, j int) bool { return qs[i].Key < qs[j].Key })
	return qs
}

func (z *Zone) sendQuestLog(p *player, withInventory bool) {
	log := &pb.QuestLog{}
	for _, q := range z.sortedQuests() {
		if _, taken := p.quests[q.Key]; taken {
			log.Quests = append(log.Quests, z.questInfo(p, q, ""))
		}
	}
	if withInventory {
		log.Inventory = z.inventoryList(p)
	}
	z.send(p, &pb.ServerMessage{Payload: &pb.ServerMessage_QuestLog{QuestLog: log}})
}

// giverNear returns the quest-giving NPC id if p stands close to it.
func (z *Zone) giverNear(p *player, npcID uint32) *entity {
	n, ok := z.entities[npcID]
	if !ok || n.npc == nil || !n.alive() || !n.npc.def.QuestGiver || hdist(n.pos, p.e.pos) > interactRange+1 {
		return nil
	}
	return n
}

func greeting(p *player) string {
	switch {
	case p.karma <= -30:
		return "...What do you want? Say it and leave."
	case p.renown >= 150 && p.karma >= 30:
		return "Ah, the hero of the terraces honors us! What can an old woman do for you?"
	case p.renown >= 60:
		return "Your name travels fast, cultivator. Welcome."
	default:
		return "Welcome, traveler. The terraces could use someone like you."
	}
}

func (z *Zone) interact(p *player, npcID uint32) {
	n := z.giverNear(p, npcID)
	if n == nil || !p.e.alive() {
		return
	}
	z.sendDialog(p, n, "")
}

func (z *Zone) sendDialog(p *player, n *entity, reply string) {
	d := &pb.QuestDialog{NpcId: n.id, NpcName: n.name, Greeting: greeting(p), Reply: reply}
	for _, q := range z.sortedQuests() {
		if q.Giver != n.npc.def.Key {
			continue
		}
		switch {
		case z.questAvailable(p, q):
			d.Offers = append(d.Offers, z.questInfo(p, q, q.Offer))
		case z.questComplete(p, q):
			d.Ready = append(d.Ready, z.questInfo(p, q, q.Complete))
		}
	}
	z.send(p, &pb.ServerMessage{Payload: &pb.ServerMessage_QuestDialog{QuestDialog: d}})
}

func (z *Zone) questAction(p *player, a *pb.QuestAction) {
	q, ok := z.content.Quests[a.Quest]
	if !ok || !p.e.alive() {
		return
	}
	n := z.giverNear(p, a.NpcId)
	switch a.Action {
	case "accept":
		if n == nil || n.npc.def.Key != q.Giver || !z.questAvailable(p, q) {
			return
		}
		p.quests[q.Key] = model.QuestState{State: questActive, Progress: make([]int, len(q.Objectives))}
		p.dirty = true
		z.updateReach(p)
		z.sendQuestLog(p, false)
		z.sendDialog(p, n, "")
	case "complete":
		if n == nil || n.npc.def.Key != q.Giver || !z.questComplete(p, q) {
			return
		}
		rewards, reply := q.Rewards, ""
		if len(q.Choices) > 0 {
			idx := slices.IndexFunc(q.Choices, func(c content.QuestChoice) bool { return c.Key == a.Choice })
			if idx < 0 {
				return // a choice is required
			}
			rewards, reply = q.Choices[idx].Rewards, q.Choices[idx].Reply
		}
		p.quests[q.Key] = model.QuestState{State: questStateDone}
		z.giveRewards(p, rewards)
		z.log.Info("quest completed", "player", describe(p.e), "quest", q.Key, "choice", a.Choice)
		z.sendQuestLog(p, true)
		z.sendDialog(p, n, reply)
	case "abandon":
		if p.quests[q.Key].State == questActive {
			delete(p.quests, q.Key)
			p.dirty = true
			z.sendQuestLog(p, false)
		}
	}
}

func (z *Zone) giveRewards(p *player, r content.QuestRewards) {
	if r.Exp > 0 {
		z.grantExpRaw(p, int64(r.Exp))
	}
	z.addRenown(p, r.Renown)
	if r.Karma != 0 {
		z.addKarma(p, r.Karma)
	}
	if r.DisciplineXP > 0 && z.content.Disciplines != nil {
		p.prog.XP[p.affinity()] += int64(r.DisciplineXP)
		p.progDirty = true
	}
	for item, n := range r.Items {
		p.inventory[item] += n
	}
	p.dirty = true
	if p.karma != 0 || r.Karma != 0 {
		z.refreshLook(p.e) // titles may change with karma
	}
}

// questKill counts a kill toward the killer's active quests.
func (z *Zone) questKill(p *player, npcKey string) {
	changed := false
	for key, st := range p.quests {
		if st.State != questActive {
			continue
		}
		q := z.content.Quests[key]
		if q == nil {
			continue
		}
		for i, o := range q.Objectives {
			if o.Kill == npcKey && i < len(st.Progress) && st.Progress[i] < o.Count {
				st.Progress[i]++
				changed = true
			}
		}
	}
	if changed {
		p.dirty = true
		z.sendQuestLog(p, false)
	}
}

// updateReach completes "reach" objectives for players standing there.
func (z *Zone) updateReach(p *player) {
	changed := false
	for key, st := range p.quests {
		q := z.content.Quests[key]
		if q == nil || st.State != questActive {
			continue
		}
		for i, o := range q.Objectives {
			if o.IsReach() && i < len(st.Progress) && st.Progress[i] < 1 &&
				hdist(p.e.pos, [3]float64{o.Reach[0], 0, o.Reach[1]}) <= o.Radius {
				st.Progress[i] = 1
				changed = true
			}
		}
	}
	if changed {
		p.dirty = true
		z.sendQuestLog(p, false)
	}
}

func (z *Zone) updateQuests(tick uint32) {
	if tick%10 != 0 || len(z.content.Quests) == 0 {
		return
	}
	for _, p := range z.players {
		if p.e.alive() {
			z.updateReach(p)
		}
	}
}

// say queues a speech bubble for an NPC, rate-limited.
func (z *Zone) say(n *npc, text string) {
	tick := z.tick.Load()
	if text == "" || tick < n.nextSpeech {
		return
	}
	if r := []rune(text); len(r) > maxSpeechLen {
		text = string(r[:maxSpeechLen])
	}
	n.nextSpeech = tick + speechCooldown
	z.speech = append(z.speech, &pb.Speech{EntityId: n.e.id, Text: text})
}
