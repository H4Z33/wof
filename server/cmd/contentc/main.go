// Command contentc validates the game content and writes the client bundle:
// display data the client needs before (or without) talking to the server,
// such as classes for character creation, skill tooltips and item names.
// Hidden data (AI scripts, drop tables, Fate odds) never goes to the client.
//
//	go run ./cmd/contentc            # validate and write ../client/gen/content.json
//	go run ./cmd/contentc -check     # validate only (CI)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"convergence/server/internal/content"
)

type skillJSON struct {
	ID          uint32  `json:"id"`
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Target      string  `json:"target"`
	Range       float64 `json:"range"`
	CooldownMS  int     `json:"cooldown_ms"`
	Cost        int     `json:"cost"`
	Radius      float64 `json:"radius,omitempty"`
	VFX         string  `json:"vfx"`
	Anim        string  `json:"anim"`
	Projectile  bool    `json:"projectile"`
}

type classJSON struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	World       string `json:"world"`
	WorldName   string `json:"world_name"`
	Archetype   string `json:"archetype"`
	Description string `json:"description"`
	HP          int    `json:"hp"`
	Resource    struct {
		Name  string `json:"name"`
		Color string `json:"color"`
		Max   int    `json:"max"`
	} `json:"resource"`
	Skills []skillJSON `json:"skills"`
	Weapon struct {
		Name         string  `json:"name"`
		Kind         string  `json:"kind"`
		Range        float64 `json:"range"`
		Anim         string  `json:"anim"`
		FlourishAnim string  `json:"flourish_anim"`
		VFX          string  `json:"vfx"`
	} `json:"weapon"`
}

type bundle struct {
	Classes []classJSON       `json:"classes"`
	Items   map[string]string `json:"items"` // key -> name
}

func main() {
	contentDir := flag.String("content", "../content", "content directory")
	out := flag.String("out", "../client/gen/content.json", "client bundle path")
	check := flag.Bool("check", false, "validate only")
	flag.Parse()

	c, err := content.Load(*contentDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "content invalid:\n"+err.Error())
		os.Exit(1)
	}
	fmt.Printf("content ok: %d classes, %d skills, %d npcs, %d items, %d zones\n",
		len(c.Classes), len(c.Skills), len(c.NPCs), len(c.Items), len(c.Zones))
	if *check {
		return
	}
	b := build(c)
	data, _ := json.MarshalIndent(b, "", "  ")
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", *out)
}

func build(c *content.Content) bundle {
	b := bundle{Items: map[string]string{}}
	for k, it := range c.Items {
		b.Items[k] = it.Name
	}
	keys := make([]string, 0, len(c.Classes))
	for k := range c.Classes {
		keys = append(keys, k)
	}
	// Jade, Brass, Myth order, then by name.
	order := map[string]int{"jade": 0, "brass": 1, "myth": 2}
	sort.Slice(keys, func(i, j int) bool {
		a, b := c.Classes[keys[i]], c.Classes[keys[j]]
		if order[a.World] != order[b.World] {
			return order[a.World] < order[b.World]
		}
		return a.Name < b.Name
	})
	for _, k := range keys {
		cl := c.Classes[k]
		cj := classJSON{Key: cl.Key, Name: cl.Name, World: cl.World, WorldName: content.Worlds[cl.World],
			Archetype: cl.Archetype, Description: cl.Description, HP: cl.MaxHP(1)}
		cj.Resource.Name, cj.Resource.Color, cj.Resource.Max = cl.Resource.Name, cl.Resource.Color, cl.Resource.Max
		w := cl.Weapon
		cj.Weapon.Name, cj.Weapon.Kind, cj.Weapon.Range = w.Name, w.Kind, w.Range
		cj.Weapon.Anim, cj.Weapon.FlourishAnim, cj.Weapon.VFX = w.Anim, w.FlourishAnim, w.VFX
		for _, id := range cl.SkillIDs {
			s := c.Skills[id]
			cj.Skills = append(cj.Skills, skillJSON{ID: s.ID, Key: s.Key, Name: s.Name, Description: s.Description,
				Target: s.Target, Range: s.Range, CooldownMS: s.CooldownMS, Cost: s.Cost, Radius: s.Radius, VFX: s.VFX,
				Anim: s.Anim, Projectile: s.Projectile})
		}
		b.Classes = append(b.Classes, cj)
	}
	return b
}
