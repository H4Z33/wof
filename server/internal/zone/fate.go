package zone

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"

	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
	"convergence/server/internal/fate"
)

// Rough guesses of how often each trigger happens per minute of play, used
// to decide which slots a deck should cover.
var triggerRate = map[string]float64{
	fate.TriggerStrike:  12,
	fate.TriggerKill:    2,
	fate.TriggerTravel:  3,
	fate.TriggerRest:    1,
	fate.TriggerWounded: 0.5,
}

const (
	cacheClaimRadius = 2.0
	fateGraceMS      = 10_000 // no fate in the first seconds after joining
	historyKept      = 4
)

func (z *Zone) initFate(p *player) {
	p.entropy = fate.NewEntropy(z.fateSecret ^ uint64(p.charID)*0x9E3779B97F4A7C15)
	p.travelPos = p.e.pos
	p.stillTick = z.tick.Load()
	p.woundArmed = true
	p.fateReady = z.tick.Load() + z.ticks(fateGraceMS)
	if z.content.Fate != nil {
		z.requestDeal(p) // prefetch right away, so cards are ready when play starts
	}
}

func (z *Zone) addKarma(p *player, delta int) {
	lo, hi := -100, 100
	if f := z.content.Fate; f != nil {
		lo, hi = f.Karma.Min, f.Karma.Max
	}
	p.karma = max(lo, min(hi, p.karma+delta))
	p.dirty = true
}

// --- Dealing ------------------------------------------------------------------

func karmaBand(k int) string {
	switch {
	case k <= -30:
		return "wicked"
	case k <= -5:
		return "questionable"
	case k < 5:
		return "neutral"
	case k < 30:
		return "good"
	default:
		return "righteous"
	}
}

// situation summarizes what the deck was written for; when it changes, the
// cards no longer fit and a new deal is requested.
func (z *Zone) situation(p *player) string {
	return fmt.Sprintf("%s|%d|%s", z.placeOf(p.e.pos), p.e.level, karmaBand(p.karma))
}

func (z *Zone) placeOf(pos [3]float64) string {
	for _, pl := range z.places {
		if math.Hypot(pos[0]-pl.center[0], pos[2]-pl.center[1]) <= pl.radius {
			return pl.name
		}
	}
	return "on the open " + strings.ReplaceAll(z.cfg.ID, "_", " ")
}

func (z *Zone) maybeRefill(p *player, tick uint32) {
	f := z.content.Fate
	if p.dealPending || tick-p.lastDeal < z.ticks(int(f.Deck.MinRefillS*1000)) {
		return
	}
	if len(p.deck) < f.Deck.RefillBelow || z.situation(p) != p.dealKey {
		z.requestDeal(p)
	}
}

func (z *Zone) requestDeal(p *player) {
	req := z.deckRequest(p, z.content.Fate.Deck.Size)
	p.dealPending = true
	p.lastDeal = z.tick.Load()
	p.dealKey = z.situation(p)
	if z.fate == nil || !z.fate.Deal(req, z.deliverDeck) {
		z.receiveDeck(fate.FallbackDeck(req))
	}
}

// deliverDeck runs on a fate worker goroutine and hands the deck to the zone.
func (z *Zone) deliverDeck(d fate.Deck) {
	select {
	case z.inbox <- d:
	case <-z.done:
	}
}

func (z *Zone) receiveDeck(d fate.Deck) {
	eid, ok := z.byChar[d.CharacterID]
	if !ok {
		return // logged out meanwhile
	}
	p := z.players[eid]
	p.dealPending = false
	// The new cards were written for the latest situation; old ones go.
	p.deck = d.Cards
}

func (z *Zone) catalog() []fate.Action {
	f := z.content.Fate
	names := make([]string, 0, len(f.Actions))
	for name := range f.Actions {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]fate.Action, 0, len(names))
	for _, name := range names {
		a := f.Actions[name]
		omen := fate.Fortune
		if a.Omen == "misfortune" {
			omen = fate.Misfortune
		}
		out = append(out, fate.Action{Name: name, Omen: omen, Triggers: a.Triggers})
	}
	return out
}

func (z *Zone) loot() map[int][]fate.Item {
	out := map[int][]fate.Item{}
	for mag, keys := range z.content.Fate.Loot {
		for _, k := range keys {
			out[mag] = append(out[mag], fate.Item{Key: k, Name: z.content.Items[k].Name})
		}
	}
	return out
}

// deckRequest asks for size cards whose slots are sampled from this
// player's own odds, so the deck covers what is likely to be drawn.
func (z *Zone) deckRequest(p *player, size int) fate.DeckRequest {
	req := fate.DeckRequest{
		CharacterID: p.charID,
		Name:        p.e.name,
		Level:       p.e.level,
		Luck:        p.luck,
		Karma:       p.karma,
		HPPercent:   100 * p.e.hp / max(p.e.maxHP, 1),
		Place:       z.placeOf(p.e.pos),
		History:     p.fateHistory,
		Actions:     z.catalog(),
		Loot:        z.loot(),
	}
	if p.kills > 0 {
		req.Recent = append(req.Recent, fmt.Sprintf("defeated %d foes", p.kills))
	}
	if p.deaths > 0 {
		req.Recent = append(req.Recent, fmt.Sprintf("fell in battle %d times", p.deaths))
	}

	triggers, weights := []string{}, []float64{}
	total := 0.0
	for name, t := range z.content.Fate.Triggers {
		w := t.Chance * triggerRate[name]
		triggers, weights = append(triggers, name), append(weights, w)
		total += w
	}
	for len(req.Slots) < size && total > 0 {
		for range 20 { // resample combinations no action can fill
			r, trigger := rand.Float64()*total, triggers[len(triggers)-1]
			for i, w := range weights {
				if r < w {
					trigger = triggers[i]
					break
				}
				r -= w
			}
			omen, mag := z.rollOmen(p, rand.Float64)
			s := fate.Slot{Trigger: trigger, Omen: omen, Magnitude: mag}
			if z.slotFillable(req.Actions, s) {
				req.Slots = append(req.Slots, s)
				break
			}
		}
	}
	return req
}

func (z *Zone) slotFillable(actions []fate.Action, s fate.Slot) bool {
	for _, a := range actions {
		if a.Omen == s.Omen {
			for _, t := range a.Triggers {
				if t == s.Trigger {
					return true
				}
			}
		}
	}
	return false
}

// rollOmen decides fortune or misfortune and its magnitude. Luck and karma
// tilt the odds; luck also makes fortunes bigger and misfortunes smaller.
func (z *Zone) rollOmen(p *player, rnd func() float64) (fate.Omen, int) {
	odds := z.content.Fate.FortuneOdds
	pFortune := 0.5 + odds.LuckWeight*float64(p.luck-10) + odds.KarmaWeight*float64(p.karma)
	pFortune = max(odds.Min, min(odds.Max, pFortune))
	omen := fate.Misfortune
	if rnd() < pFortune {
		omen = fate.Fortune
	}
	edge := float64(p.luck-10) / 20
	if omen == fate.Misfortune {
		edge = -edge
	}
	mag := 1
	if rnd() < 0.35+edge {
		mag++
		if rnd() < 0.3+edge {
			mag++
		}
	}
	return omen, mag
}

// --- Triggers and draws --------------------------------------------------------

// updateFate checks travel and rest triggers, claims hidden caches and keeps
// decks topped up. Called every tick.
func (z *Zone) updateFate(tick uint32) {
	f := z.content.Fate
	if f == nil {
		return
	}
	for _, p := range z.players {
		e := p.e
		if !e.alive() {
			continue
		}
		if t, ok := f.Triggers[fate.TriggerTravel]; ok && hdist(e.pos, p.travelPos) >= t.EveryM {
			p.travelPos = e.pos
			if card, ok := z.fateTrigger(p, fate.TriggerTravel, nil); ok {
				z.applyCard(p, card, nil)
			}
		}
		if e.moving {
			p.stillTick, p.restArmed = tick, true
		} else if t, ok := f.Triggers[fate.TriggerRest]; ok && p.restArmed && tick-p.stillTick >= z.ticks(int(t.AfterS*1000)) {
			p.restArmed = false
			if card, ok := z.fateTrigger(p, fate.TriggerRest, nil); ok {
				z.applyCard(p, card, nil)
			}
		}
		if t := f.Triggers[fate.TriggerWounded]; float64(e.hp) > (t.BelowHP+0.2)*float64(e.maxHP) {
			p.woundArmed = true
		}
		z.maybeRefill(p, tick)
	}
	z.updateCaches(tick)
}

// fateTrigger rolls whether fate strikes now, using the player's entropy,
// and draws the card for it. target is the enemy involved, if any.
func (z *Zone) fateTrigger(p *player, trigger string, target *entity) (fate.Card, bool) {
	f := z.content.Fate
	if f == nil || !p.e.alive() {
		return fate.Card{}, false
	}
	t, ok := f.Triggers[trigger]
	tick := z.tick.Load()
	if !ok || tick < p.fateReady || p.entropy.Float() >= t.Chance {
		return fate.Card{}, false
	}
	omen, mag := z.rollOmen(p, p.entropy.Float)
	card, rest, found := fate.Draw(p.deck, trigger, omen, mag)
	if found {
		p.deck = rest
	} else {
		slot := fate.Slot{Trigger: trigger, Omen: omen, Magnitude: mag}
		req := fate.DeckRequest{Actions: z.catalog(), Loot: z.loot()}
		if !z.slotFillable(req.Actions, slot) {
			return fate.Card{}, false
		}
		card = fate.FallbackCard(req, slot)
	}
	// Strike-time actions need a living enemy.
	if (card.Action == "weakness" || card.Action == "enrage") && (target == nil || !target.alive()) {
		return fate.Card{}, false
	}
	p.fateReady = tick + z.ticks(int(f.CooldownS*1000))
	return card, true
}

// applyCard carries out a card. For strike-time actions it returns how the
// current blow is changed: a damage multiplier, or a miss.
func (z *Zone) applyCard(p *player, card fate.Card, target *entity) (mult float64, miss bool) {
	mult = 1
	f := z.content.Fate
	a := f.Actions[card.Action]
	if a == nil {
		return mult, false
	}
	tick := z.tick.Load()
	mag := float64(card.Magnitude)
	ev := &pb.FateEvent{
		Magnitude: uint32(card.Magnitude), Action: card.Action, Narration: card.Narration, Trigger: card.Trigger,
	}
	if card.Omen == fate.Fortune {
		ev.Omen = pb.FateOmen_FATE_OMEN_FORTUNE
	} else {
		ev.Omen = pb.FateOmen_FATE_OMEN_MISFORTUNE
	}
	duration := z.ticks(int(a.DurationS * 1000))
	ev.DurationMs = uint32(a.DurationS * 1000)

	switch card.Action {
	case "treasure", "spoils":
		item := z.content.Items[card.ItemKey]
		if item == nil {
			return mult, false
		}
		p.inventory[item.Key]++
		ev.ItemName, ev.Amount, ev.Inventory = item.Name, 1, z.inventoryList(p)
	case "blessing":
		ev.Amount = uint32(p.e.maxHP - p.e.hp)
		p.e.hp = p.e.maxHP
	case "insight":
		amount := int64(a.ExpPerMagnitude) * int64(card.Magnitude)
		ev.Amount = uint32(amount)
		z.grantExp(p, amount)
	case "critical":
		mult = 1 + mag
	case "fumble":
		miss = true
	case "weakness":
		target.addEffect(effect{flag: pb.EffectFlag_EFFECT_FLAG_WEAKENED, dealt: 1, taken: 1 + a.BonusPerMagnitude*mag, until: tick + duration})
	case "enrage":
		target.addEffect(effect{flag: pb.EffectFlag_EFFECT_FLAG_ENRAGED, dealt: 1 + a.BonusPerMagnitude*mag, taken: 1, until: tick + duration})
	case "vigor_aura":
		p.e.addEffect(effect{flag: pb.EffectFlag_EFFECT_FLAG_VIGOR, dealt: 1 + a.BonusPerMagnitude*mag, taken: 1, until: tick + duration})
	case "ill_omen":
		p.e.addEffect(effect{flag: pb.EffectFlag_EFFECT_FLAG_ILL_OMEN, dealt: max(0.2, 1+a.BonusPerMagnitude*mag), taken: 1, until: tick + duration})
	case "hidden_cache":
		item := z.content.Items[card.ItemKey]
		if item == nil {
			return mult, false
		}
		z.spawnCache(p, item.Key, card.Magnitude, a)
		ev.ItemName = item.Name
	case "ambush":
		z.spawnAmbush(p, card.Magnitude, a)
		ev.Amount = uint32(card.Magnitude)
	case "mishap":
		loss := int(math.Ceil(float64(p.e.maxHP) * a.HPLossPerMagnitude * mag))
		loss = max(0, min(loss, p.e.hp-1))
		p.e.hp -= loss
		p.e.lastDamagedTick = tick
		ev.Amount = uint32(loss)
	default:
		return mult, false
	}

	if card.Omen == fate.Fortune && z.content.Titles != nil {
		z.addRenown(p, z.content.Titles.Renown.FateFortune)
	}
	p.kills, p.deaths = 0, 0
	p.fateHistory = append(p.fateHistory, card.Narration)
	if len(p.fateHistory) > historyKept {
		p.fateHistory = p.fateHistory[len(p.fateHistory)-historyKept:]
	}
	p.dirty = true
	z.send(p, &pb.ServerMessage{Payload: &pb.ServerMessage_Fate{Fate: ev}})
	z.log.Info("fate", "player", describe(p.e), "trigger", card.Trigger, "omen", card.Omen,
		"magnitude", card.Magnitude, "action", card.Action, "item", card.ItemKey, "source", card.Source)
	return mult, miss
}

// spawnAmbush surrounds the player with temporary enemies already hunting them.
func (z *Zone) spawnAmbush(p *player, count int, a *content.FateAction) {
	def := z.content.NPCs[a.NPC]
	if def == nil {
		return
	}
	tick := z.tick.Load()
	base := rand.Float64() * 2 * math.Pi
	for i := range count {
		n, err := z.newNPC(def)
		if err != nil {
			z.log.Error("ambush", "err", err)
			return
		}
		angle := base + float64(i)*2*math.Pi/float64(count)
		lim := z.cfg.HalfExtent
		pos := z.pointNear(clamp(p.e.pos[0]+8*math.Cos(angle), -lim, lim), clamp(p.e.pos[2]+8*math.Sin(angle), -lim, lim), 2)
		z.placeNPC(n, pos)
		n.temporary = true
		n.expireAt = tick + z.ticks(90_000)
		n.target = p.e.id
		z.npcs = append(z.npcs, n)
	}
}

// spawnCache hides a cache some distance away that only p can claim.
func (z *Zone) spawnCache(p *player, itemKey string, magnitude int, a *content.FateAction) {
	lo, hi := a.DistanceM[0], a.DistanceM[1]
	d := lo + rand.Float64()*max(0, hi-lo)
	angle := rand.Float64() * 2 * math.Pi
	lim := z.cfg.HalfExtent - 1
	e := &entity{
		id:    z.newID(),
		kind:  pb.EntityKind_ENTITY_KIND_OBJECT,
		name:  "Hidden Cache",
		model: "fate_cache",
		pos:   z.pointNear(clamp(p.e.pos[0]+d*math.Cos(angle), -lim, lim), clamp(p.e.pos[2]+d*math.Sin(angle), -lim, lim), 3),
		hp:    1,
		maxHP: 1,
		level: 1,
		cache: &cache{owner: p.charID, itemKey: itemKey, magnitude: magnitude, expireAt: z.tick.Load() + z.ticks(int(a.ExpireS*1000))},
	}
	z.entities[e.id] = e
	z.grid.add(e)
	z.caches = append(z.caches, e)
}

func (z *Zone) updateCaches(tick uint32) {
	kept := z.caches[:0]
	for _, e := range z.caches {
		c := e.cache
		claimed := false
		if eid, ok := z.byChar[c.owner]; ok {
			p := z.players[eid]
			if p.e.alive() && hdist(p.e.pos, e.pos) <= cacheClaimRadius {
				item := z.content.Items[c.itemKey]
				p.inventory[item.Key]++
				p.dirty = true
				z.send(p, &pb.ServerMessage{Payload: &pb.ServerMessage_Fate{Fate: &pb.FateEvent{
					Omen: pb.FateOmen_FATE_OMEN_FORTUNE, Magnitude: uint32(c.magnitude), Action: "cache_found",
					Narration: "You brush aside the stones and find " + articled(item.Name) + ".",
					ItemName:  item.Name, Amount: 1, Inventory: z.inventoryList(p),
				}}})
				claimed = true
			}
		}
		if claimed || tick >= c.expireAt {
			z.grid.remove(e)
			delete(z.entities, e.id)
			continue
		}
		kept = append(kept, e)
	}
	clear(z.caches[len(kept):])
	z.caches = kept
}

func articled(name string) string {
	if name != "" && strings.ContainsRune("AEIOU", rune(name[0])) {
		return "an " + name
	}
	return "a " + name
}

func (z *Zone) inventoryList(p *player) []*pb.InventoryItem {
	keys := make([]string, 0, len(p.inventory))
	for k, n := range p.inventory {
		if n > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]*pb.InventoryItem, 0, len(keys))
	for _, k := range keys {
		name := k
		if it := z.content.Items[k]; it != nil {
			name = it.Name
		}
		out = append(out, &pb.InventoryItem{Key: k, Name: name, Quantity: uint32(p.inventory[k])})
	}
	return out
}

// pruneEffects drops expired effects.
func pruneEffects(e *entity, tick uint32) {
	kept := e.effects[:0]
	for _, ef := range e.effects {
		if tick < ef.until {
			kept = append(kept, ef)
		}
	}
	e.effects = kept
}
