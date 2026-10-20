package fate

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func testRequest() DeckRequest {
	return DeckRequest{
		CharacterID: 1, Name: "Lin", Level: 3, Luck: 14, Karma: 6, HPPercent: 80,
		Place: "near a mountain bandit camp", Recent: []string{"defeated 2 foes"},
		Slots: []Slot{
			{Trigger: TriggerStrike, Omen: Fortune, Magnitude: 2},
			{Trigger: TriggerKill, Omen: Fortune, Magnitude: 1},
			{Trigger: TriggerTravel, Omen: Misfortune, Magnitude: 2},
			{Trigger: TriggerRest, Omen: Fortune, Magnitude: 3},
		},
		Actions: []Action{
			{Name: "treasure", Omen: Fortune, Triggers: []string{TriggerKill, TriggerTravel, TriggerRest}},
			{Name: "critical", Omen: Fortune, Triggers: []string{TriggerStrike}},
			{Name: "weakness", Omen: Fortune, Triggers: []string{TriggerStrike}},
			{Name: "hidden_cache", Omen: Fortune, Triggers: []string{TriggerTravel, TriggerRest}},
			{Name: "ambush", Omen: Misfortune, Triggers: []string{TriggerTravel, TriggerRest, TriggerKill}},
			{Name: "mishap", Omen: Misfortune, Triggers: []string{TriggerTravel, TriggerRest}},
		},
		Loot: map[int][]Item{
			1: {{"spirit_herb", "Spirit Herb"}},
			2: {{"jade_token", "Jade Token"}},
			3: {{"phoenix_feather", "Phoenix Feather"}},
		},
	}
}

func TestValidateDeckKeepsGoodCardsAndReplacesBadOnes(t *testing.T) {
	req := testRequest()
	deck, problems := ValidateDeck(req, []CardChoice{
		{Slot: 0, Action: "critical", Narration: "Your palm strikes true."},
		{Slot: 1, Action: "treasure", Item: "phoenix_feather", Narration: "A feather!"}, // wrong magnitude loot
		{Slot: 2, Action: "blessing", Narration: "Nice."},                               // not allowed
		// slot 3 missing
	})
	if len(deck.Cards) != 4 {
		t.Fatalf("%d cards, want one per slot", len(deck.Cards))
	}
	if c := deck.Cards[0]; c.Source != "agent" || c.Action != "critical" {
		t.Errorf("good card replaced: %+v", c)
	}
	for i := 1; i < 4; i++ {
		c := deck.Cards[i]
		if c.Source != "fallback" {
			t.Errorf("slot %d kept a bad card: %+v", i, c)
		}
		if allowed := req.actionsFor(c.Slot); !strings.Contains(strings.Join(allowed, ","), c.Action) {
			t.Errorf("slot %d fallback action %q not allowed (%v)", i, c.Action, allowed)
		}
	}
	if len(problems) != 3 {
		t.Errorf("problems = %v", problems)
	}
}

func TestDrawPrefersSameTrigger(t *testing.T) {
	cards := []Card{
		{Slot: Slot{Trigger: TriggerTravel, Omen: Fortune, Magnitude: 1}, Action: "treasure"},
		{Slot: Slot{Trigger: TriggerKill, Omen: Fortune, Magnitude: 1}, Action: "insight"},
		{Slot: Slot{Trigger: TriggerKill, Omen: Misfortune, Magnitude: 1}, Action: "ambush"},
	}
	c, rest, ok := Draw(cards, TriggerKill, Fortune, 1)
	if !ok || c.Action != "insight" || len(rest) != 2 {
		t.Fatalf("drew %+v ok=%v rest=%d", c, ok, len(rest))
	}
	// No magnitude-3 fortune: the non-item card is used at magnitude 3, the
	// treasure card (whose item depends on magnitude) is not.
	c, _, ok = Draw(cards, TriggerTravel, Fortune, 3)
	if !ok || c.Action != "insight" || c.Magnitude != 3 {
		t.Errorf("relaxed draw = %+v ok=%v", c, ok)
	}
	if _, _, ok := Draw(cards[:1], TriggerTravel, Fortune, 3); ok {
		t.Error("treasure card drawn at a different magnitude")
	}
	if len(cards) != 3 {
		t.Error("Draw modified the caller's deck")
	}
}

func TestSanitize(t *testing.T) {
	got := Sanitize("  line one\nline\ttwo \x00\x1b[31m ")
	if got != "line one line two [31m" {
		t.Errorf("got %q", got)
	}
	if long := Sanitize(strings.Repeat("a ", 500)); len([]rune(long)) > maxNarration+1 {
		t.Errorf("not truncated: %d runes", len([]rune(long)))
	}
}

func TestEntropyDependsOnInputs(t *testing.T) {
	a, b := NewEntropy(42), NewEntropy(42)
	a.Mix(1, 2, 3)
	b.Mix(1, 2, 4)
	if a.Float() == b.Float() {
		t.Error("different inputs gave the same roll")
	}
	c, d := NewEntropy(1), NewEntropy(2)
	c.Mix(7)
	d.Mix(7)
	if c.Float() == d.Float() {
		t.Error("different secrets gave the same roll")
	}
	e := NewEntropy(9)
	for range 1000 {
		if f := e.Float(); f < 0 || f >= 1 {
			t.Fatalf("Float out of range: %v", f)
		}
	}
}

// TestOllamaLive asks the real model for a deck. Run with:
//
//	FATE_OLLAMA_MODEL=deepseek-v4.1-flash:cloud go test ./internal/fate -run Live -v
func TestOllamaLive(t *testing.T) {
	model := os.Getenv("FATE_OLLAMA_MODEL")
	if model == "" {
		t.Skip("FATE_OLLAMA_MODEL not set")
	}
	o := &Ollama{URL: "http://127.0.0.1:11434", Model: model}
	req := testRequest()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	choices, err := o.Deal(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	deck, problems := ValidateDeck(req, choices)
	t.Logf("%d choices in %v, problems: %v", len(choices), time.Since(start), problems)
	for _, c := range deck.Cards {
		t.Logf("[%s %s %d] %s %s (%s): %s", c.Trigger, c.Omen, c.Magnitude, c.Action, c.ItemKey, c.Source, c.Narration)
	}
}
