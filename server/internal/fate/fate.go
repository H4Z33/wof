// Package fate turns a player's luck and karma into events, the way a
// tabletop game master would.
//
// Ahead of time, an Oracle (the LLM agent) fills a small deck of cards for a
// player: for each slot the server asks for (trigger, omen, magnitude) it
// picks an allowed action and writes the narration. During play the zone
// rolls the dice and draws a matching card, so the agent is never on the
// critical path. The Director runs deals in the background and replaces any
// invalid card with a fallback.
package fate

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Omen int

const (
	Fortune Omen = iota + 1
	Misfortune
)

func (o Omen) String() string {
	if o == Fortune {
		return "fortune"
	}
	return "misfortune"
}

// Triggers: moments in play when fate may strike.
const (
	TriggerStrike  = "strike"
	TriggerKill    = "kill"
	TriggerTravel  = "travel"
	TriggerRest    = "rest"
	TriggerWounded = "wounded"
)

var triggerWords = map[string]string{
	TriggerStrike:  "the instant the cultivator strikes an enemy",
	TriggerKill:    "right after the cultivator defeats a foe",
	TriggerTravel:  "while the cultivator is on the road",
	TriggerRest:    "while the cultivator stands still, resting",
	TriggerWounded: "when the cultivator is badly wounded",
}

type Item struct {
	Key  string
	Name string
}

// Slot is one card the server wants prepared.
type Slot struct {
	Trigger   string
	Omen      Omen
	Magnitude int // 1-3
}

// DeckRequest is what the zone knows about a player when asking for cards.
type DeckRequest struct {
	CharacterID int64
	Name        string
	Level       int
	Luck        int
	Karma       int
	HPPercent   int
	// Where the player is, in words ("near a mountain bandit camp").
	Place string
	// Recent deeds, in words ("defeated 3 foes").
	Recent []string
	// Earlier narrations for this character, to avoid repeating itself.
	History []string

	Slots   []Slot
	Actions []Action
	// Items treasure, spoils and hidden caches may grant, by magnitude.
	Loot map[int][]Item
}

// Action is one entry of the fate catalog offered to the oracle.
type Action struct {
	Name     string
	Omen     Omen
	Triggers []string
}

// ActionDescriptions tell the oracle what each action kind does.
var ActionDescriptions = map[string]string{
	"treasure":     "the cultivator finds an item",
	"blessing":     "the cultivator's wounds are fully healed",
	"insight":      "the cultivator gains understanding (experience)",
	"critical":     "this very blow lands with devastating force",
	"weakness":     "the enemy's weak point is exposed; it takes more damage for a while",
	"vigor_aura":   "an aura of vigor surrounds the cultivator; their blows grow stronger for a while",
	"hidden_cache": "the cultivator notices a hidden cache nearby, which they must reach to claim",
	"spoils":       "the defeated foe leaves behind something valuable",
	"ambush":       "enemies appear around the cultivator and attack",
	"mishap":       "an accident hurts the cultivator (never lethal)",
	"fumble":       "this very blow misses",
	"enrage":       "the enemy becomes enraged and hits harder for a while",
	"ill_omen":     "an ill omen weakens the cultivator's blows for a while",
}

// actionsFor lists the actions allowed for a slot.
func (r DeckRequest) actionsFor(s Slot) []string {
	var out []string
	for _, a := range r.Actions {
		if a.Omen == s.Omen && slices.Contains(a.Triggers, s.Trigger) {
			out = append(out, a.Name)
		}
	}
	return out
}

// needsItem reports whether an action grants an item chosen on the card.
func needsItem(action string) bool {
	return action == "treasure" || action == "spoils" || action == "hidden_cache"
}

// CardChoice is the oracle's raw answer for one slot, not yet trusted.
type CardChoice struct {
	Slot      int    `json:"slot"`
	Action    string `json:"action"`
	Item      string `json:"item"`
	Narration string `json:"narration"`
}

// Card is a validated fate, ready to be drawn.
type Card struct {
	Slot
	Action    string
	ItemKey   string
	Narration string
	// "agent" or "fallback".
	Source string
}

type Deck struct {
	CharacterID int64
	Cards       []Card
}

type Oracle interface {
	Deal(ctx context.Context, req DeckRequest) ([]CardChoice, error)
}

type job struct {
	req   DeckRequest
	reply func(Deck)
}

type Director struct {
	oracle  Oracle
	timeout time.Duration
	log     *slog.Logger
	jobs    chan job
	wg      sync.WaitGroup
}

// NewDirector starts workers that call oracle; oracle may be nil, in which
// case every card is a fallback.
func NewDirector(oracle Oracle, workers int, timeout time.Duration, log *slog.Logger) *Director {
	d := &Director{oracle: oracle, timeout: timeout, log: log.With("component", "fate"), jobs: make(chan job, 256)}
	for range max(workers, 1) {
		d.wg.Add(1)
		go d.work()
	}
	return d
}

// Deal queues a deck request; reply is called from a worker goroutine.
// Returns false (and never calls reply) if the queue is full.
func (d *Director) Deal(req DeckRequest, reply func(Deck)) bool {
	select {
	case d.jobs <- job{req: req, reply: reply}:
		return true
	default:
		return false
	}
}

// Close stops accepting work and waits for in-flight deals.
func (d *Director) Close() {
	close(d.jobs)
	d.wg.Wait()
}

func (d *Director) work() {
	defer d.wg.Done()
	for j := range d.jobs {
		j.reply(d.deal(j.req))
	}
}

func (d *Director) deal(req DeckRequest) Deck {
	if d.oracle == nil {
		return FallbackDeck(req)
	}
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()
	start := time.Now()
	choices, err := d.oracle.Deal(ctx, req)
	if err != nil {
		d.log.Warn("oracle failed, dealing fallback cards", "character", req.CharacterID, "err", err)
		return FallbackDeck(req)
	}
	deck, problems := ValidateDeck(req, choices)
	if len(problems) > 0 {
		d.log.Info("oracle cards replaced", "character", req.CharacterID, "problems", strings.Join(problems, "; "))
	}
	d.log.Info("deck dealt", "character", req.CharacterID, "cards", len(deck.Cards),
		"from_agent", len(deck.Cards)-len(problems), "ms", time.Since(start).Milliseconds())
	return deck
}

const maxNarration = 240

// ValidateDeck returns one card per requested slot, keeping the oracle's
// card when it is valid and dealing a fallback otherwise, plus what was
// replaced and why.
func ValidateDeck(req DeckRequest, choices []CardChoice) (Deck, []string) {
	bySlot := make(map[int]CardChoice, len(choices))
	for i, c := range choices {
		if c.Slot < 0 || c.Slot >= len(req.Slots) {
			c.Slot = i // tolerate a missing or bad slot number
		}
		if _, dup := bySlot[c.Slot]; !dup {
			bySlot[c.Slot] = c
		}
	}
	deck := Deck{CharacterID: req.CharacterID}
	var problems []string
	for i, slot := range req.Slots {
		c, ok := bySlot[i]
		if !ok {
			problems = append(problems, fmt.Sprintf("slot %d missing", i))
			deck.Cards = append(deck.Cards, FallbackCard(req, slot))
			continue
		}
		card, err := validateCard(req, slot, c)
		if err != nil {
			problems = append(problems, fmt.Sprintf("slot %d: %v", i, err))
			card = FallbackCard(req, slot)
		}
		deck.Cards = append(deck.Cards, card)
	}
	return deck, problems
}

func validateCard(req DeckRequest, slot Slot, c CardChoice) (Card, error) {
	card := Card{Slot: slot, Source: "agent"}
	card.Action = strings.ToLower(strings.TrimSpace(c.Action))
	if !slices.Contains(req.actionsFor(slot), card.Action) {
		return card, fmt.Errorf("action %q not allowed for %s on %s", c.Action, slot.Omen, slot.Trigger)
	}
	if needsItem(card.Action) {
		want := strings.TrimSpace(c.Item)
		for _, it := range req.Loot[slot.Magnitude] {
			if strings.EqualFold(it.Key, want) || strings.EqualFold(it.Name, want) {
				card.ItemKey = it.Key
			}
		}
		if card.ItemKey == "" {
			loot := req.Loot[slot.Magnitude]
			if card.Action == "treasure" || len(loot) == 0 {
				// Treasure narrations name what is found, so a wrong item
				// would contradict the text.
				return card, fmt.Errorf("item %q not in the magnitude %d loot", c.Item, slot.Magnitude)
			}
			// Caches and spoils rarely name the item: pick one ourselves.
			card.ItemKey = loot[rand.IntN(len(loot))].Key
		}
	}
	card.Narration = Sanitize(c.Narration)
	if card.Narration == "" {
		return card, fmt.Errorf("empty narration")
	}
	return card, nil
}

// Sanitize makes model text safe to show: printable characters only, single
// line, bounded length.
func Sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxNarration {
		s = strings.TrimRightFunc(string(r[:maxNarration]), unicode.IsSpace) + "…"
	}
	return s
}

// Draw takes the best card for what was rolled and returns the remaining
// deck. Preference: same omen and magnitude, then same trigger. If no card
// has the rolled magnitude, a card of the same omen whose action does not
// grant an item is used at the rolled magnitude (an item's value depends on
// its magnitude, so those cards never change it).
func Draw(cards []Card, trigger string, omen Omen, magnitude int) (Card, []Card, bool) {
	score := func(c Card) int {
		if c.Omen != omen {
			return -1
		}
		s := 0
		if c.Magnitude == magnitude {
			s += 4
		} else if needsItem(c.Action) {
			return -1
		}
		if c.Trigger == trigger {
			s += 2
		}
		return s
	}
	best, bestScore := -1, -1
	for i, c := range cards {
		if s := score(c); s > bestScore {
			best, bestScore = i, s
		}
	}
	if best < 0 {
		return Card{}, cards, false
	}
	card := cards[best]
	card.Magnitude = magnitude
	return card, slices.Delete(slices.Clone(cards), best, best+1), true
}

// FallbackDeck deals every slot without the oracle.
func FallbackDeck(req DeckRequest) Deck {
	deck := Deck{CharacterID: req.CharacterID}
	for _, s := range req.Slots {
		deck.Cards = append(deck.Cards, FallbackCard(req, s))
	}
	return deck
}

// FallbackCard fills one slot with a stock narration.
func FallbackCard(req DeckRequest, slot Slot) Card {
	card := Card{Slot: slot, Source: "fallback"}
	actions := req.actionsFor(slot)
	if len(actions) > 0 {
		card.Action = actions[rand.IntN(len(actions))]
	}
	itemName := ""
	if needsItem(card.Action) {
		if loot := req.Loot[slot.Magnitude]; len(loot) > 0 {
			it := loot[rand.IntN(len(loot))]
			card.ItemKey, itemName = it.Key, it.Name
		}
	}
	lines := fallbackLines[card.Action]
	if len(lines) == 0 {
		card.Narration = "Fate stirs around you."
		return card
	}
	card.Narration = lines[rand.IntN(len(lines))]
	if strings.Contains(card.Narration, "%s") {
		card.Narration = fmt.Sprintf(card.Narration, articled(itemName))
	}
	return card
}

var fallbackLines = map[string][]string{
	"treasure": {
		"Something glints among the stones at your feet: %s.",
		"A gust parts the grass and reveals %s, as if the heavens left it for you.",
	},
	"blessing": {
		"Warm qi flows through your meridians; your wounds close as if they never were.",
		"A crane passes overhead and its cry soothes your body and spirit.",
	},
	"insight": {
		"Watching the clouds drift, you grasp something about the Dao you did not know before.",
		"An old saying returns to you, and this time you understand it.",
	},
	"ambush": {
		"Shadows detach from the rocks. You were followed.",
		"A whistle cuts the air: bandits close in from every side.",
	},
	"mishap": {
		"Your foot finds a loose stone and you fall hard.",
		"A careless breath scatters your qi; pain lances through your chest.",
	},
	"critical": {
		"Your qi surges at the perfect instant; the blow lands like thunder.",
	},
	"weakness": {
		"You glimpse a flaw in your foe's stance and press it without mercy.",
	},
	"vigor_aura": {
		"Heat rises from your dantian and wraps you in a shimmering aura.",
	},
	"hidden_cache": {
		"A faint glimmer flickers among the rocks not far away. Something waits there: %s, perhaps.",
	},
	"spoils": {
		"As your foe falls, %s slips from their robes.",
	},
	"fumble": {
		"Your footing betrays you and the blow cuts only air.",
	},
	"enrage": {
		"Your strike only stokes their fury; their eyes burn red.",
	},
	"ill_omen": {
		"A crow lands on a dead branch and caws three times. Your strength ebbs.",
	},
}

func articled(name string) string {
	if name == "" {
		return "something"
	}
	if strings.ContainsRune("AEIOU", rune(name[0])) {
		return "an " + name
	}
	return "a " + name
}
