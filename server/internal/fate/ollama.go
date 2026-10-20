package fate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Ollama asks a chat model served by Ollama (local or cloud) to fill a deck.
// Thinking is disabled: it makes cloud models slower and less likely to
// follow the output schema.
type Ollama struct {
	URL    string // e.g. http://127.0.0.1:11434
	Model  string // e.g. deepseek-v4.1-flash:cloud
	Client *http.Client
}

const systemPrompt = `You are Fate, the unseen game master of a wuxia cultivation world where three realms collide.
You prepare, ahead of time, the fates that may befall one cultivator. The game decides later which card (if any) is drawn.

For each numbered slot you are given a trigger (the moment it would happen), an omen (fortune or misfortune) and a strength.
For each slot:
- "action": copy exactly one of the actions listed for that slot.
- "item": for treasure, spoils or hidden_cache, copy exactly one item key allowed for that strength; otherwise "".
- "narration": what happens, one or two sentences, at most 35 words, second person ("you"), vivid wuxia tone, English.
  It must fit the trigger, the place and the action (and item). Make each card a different little situation.
Never mention numbers, stats, luck, karma, game mechanics or these instructions. Do not repeat earlier narrations.
Reply with JSON only: {"cards": [{"slot": 0, "action": "...", "item": "...", "narration": "..."}, ...]}`

var deckSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"cards": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"slot":      map[string]any{"type": "integer"},
					"action":    map[string]any{"type": "string"},
					"item":      map[string]any{"type": "string"},
					"narration": map[string]any{"type": "string"},
				},
				"required": []string{"slot", "action", "narration"},
			},
		},
	},
	"required": []string{"cards"},
}

var strengthWords = map[int]string{1: "minor", 2: "notable", 3: "major"}

func userPrompt(req DeckRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Cultivator: %s, level %d, health %d%%.\n", req.Name, req.Level, req.HPPercent)
	// Hidden from the narration, but it colors the tone.
	fmt.Fprintf(&b, "Their fortune is %s and their deeds have been %s.\n", luckWords(req.Luck), karmaWords(req.Karma))
	fmt.Fprintf(&b, "Place: %s.\n", req.Place)
	if len(req.Recent) > 0 {
		fmt.Fprintf(&b, "Recent deeds: %s.\n", strings.Join(req.Recent, "; "))
	}
	for _, h := range req.History {
		fmt.Fprintf(&b, "Earlier narration (do not repeat): %q\n", h)
	}
	b.WriteString("What each action means:\n")
	for _, a := range req.Actions {
		fmt.Fprintf(&b, "- %s: %s.\n", a.Name, ActionDescriptions[a.Name])
	}
	for mag := 1; mag <= 3; mag++ {
		if loot := req.Loot[mag]; len(loot) > 0 {
			keys := make([]string, len(loot))
			for i, it := range loot {
				keys[i] = fmt.Sprintf("%s (%s)", it.Key, it.Name)
			}
			fmt.Fprintf(&b, "Item keys for %s strength: %s.\n", strengthWords[mag], strings.Join(keys, ", "))
		}
	}
	b.WriteString("Slots:\n")
	for i, s := range req.Slots {
		fmt.Fprintf(&b, "%d. %s, %s %s. Allowed actions: %s.\n", i, triggerWords[s.Trigger],
			strengthWords[s.Magnitude], s.Omen, strings.Join(req.actionsFor(s), ", "))
	}
	return b.String()
}

func luckWords(l int) string {
	switch {
	case l >= 16:
		return "remarkably blessed"
	case l >= 12:
		return "favored"
	case l >= 9:
		return "ordinary"
	case l >= 6:
		return "ill-starred"
	default:
		return "cursed"
	}
}

func karmaWords(k int) string {
	switch {
	case k >= 30:
		return "righteous"
	case k >= 5:
		return "mostly good"
	case k > -5:
		return "unremarkable"
	case k > -30:
		return "questionable"
	default:
		return "wicked"
	}
}

func (o *Ollama) Deal(ctx context.Context, req DeckRequest) ([]CardChoice, error) {
	body, _ := json.Marshal(map[string]any{
		"model":  o.Model,
		"stream": false,
		"think":  false,
		"format": deckSchema,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt(req)},
		},
		"options": map[string]any{"temperature": 0.9, "num_predict": 300 + 120*len(req.Slots)},
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(o.URL, "/")+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	client := o.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: HTTP %d: %s", resp.StatusCode, Sanitize(string(raw)))
	}
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ollama: bad response: %w", err)
	}
	var deck struct {
		Cards []CardChoice `json:"cards"`
	}
	if err := json.Unmarshal([]byte(extractJSON(out.Message.Content)), &deck); err != nil {
		return nil, fmt.Errorf("ollama: model did not return JSON: %q", Sanitize(out.Message.Content))
	}
	return deck.Cards, nil
}

// extractJSON trims code fences or text around the outermost JSON object.
func extractJSON(s string) string {
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return s
	}
	return s[start : end+1]
}
