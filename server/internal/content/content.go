// Package content loads game data (YAML) and scripts (Lua) from the content
// directory and validates cross-references, so a typo fails at startup
// instead of in the middle of a fight.
package content

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Range [2]int

type Skill struct {
	ID          uint32             `yaml:"id"`
	Key         string             `yaml:"key"`
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Target      string             `yaml:"target"` // enemy or self
	Range       float64            `yaml:"range"`
	CooldownMS  int                `yaml:"cooldown_ms"`
	Cost        int                `yaml:"cost"`
	Gain        int                `yaml:"gain"`
	Damage      Range              `yaml:"damage"`
	Heal        Range              `yaml:"heal"`
	Radius      float64            `yaml:"radius"`
	HealRadius  float64            `yaml:"heal_radius"`
	Apply       *SkillEffect       `yaml:"apply"`
	Anim        string             `yaml:"anim"`
	VFX         string             `yaml:"vfx"`
	Projectile  bool               `yaml:"projectile"`
	Script      string             `yaml:"script"`
	Params      map[string]float64 `yaml:"params"`

	// ScriptSource is the Lua text of Script.
	ScriptSource string `yaml:"-"`
}

// SkillEffect is a timed effect a skill applies.
type SkillEffect struct {
	Effect    string  `yaml:"effect"`
	DurationS float64 `yaml:"duration_s"`
	// Meaning depends on the effect: a multiplier (weakened 1.3, ward 0.6,
	// slowed 0.5) or an amount per second (burning, regen).
	Value float64 `yaml:"value"`
}

// SkillEffects are the effect names skills may apply.
var SkillEffects = map[string]bool{
	"weakened": true, "vigor": true, "ward": true, "burning": true, "regen": true, "stunned": true, "slowed": true,
}

// Class is a playable class: one archetype in one world.
type Class struct {
	Key         string `yaml:"key"`
	Name        string `yaml:"name"`
	World       string `yaml:"world"`
	Archetype   string `yaml:"archetype"`
	Description string `yaml:"description"`
	HP          struct {
		Base     int `yaml:"base"`
		PerLevel int `yaml:"per_level"`
	} `yaml:"hp"`
	Resource struct {
		Key       string  `yaml:"key"`
		Name      string  `yaml:"name"`
		Max       int     `yaml:"max"`
		Start     int     `yaml:"start"`
		RegenPerS float64 `yaml:"regen_per_s"`
		DecayPerS float64 `yaml:"decay_per_s"`
		Color     string  `yaml:"color"`
	} `yaml:"resource"`
	Skills []string `yaml:"skills"`

	// SkillIDs resolves Skills, in order.
	SkillIDs []uint32 `yaml:"-"`
}

// MaxHP is the class's maximum health at a level.
func (c *Class) MaxHP(level int) int { return c.HP.Base + c.HP.PerLevel*(level-1) }

// Worlds are the three colliding realms.
var Worlds = map[string]string{"jade": "Jade Realm", "brass": "Brass Realm", "myth": "Myth Realm"}

// DefaultClass is given to characters created before classes existed.
const DefaultClass = "sword_cultivator"

type NPCAttack struct {
	Damage     Range  `yaml:"damage"`
	CooldownMS int    `yaml:"cooldown_ms"`
	Anim       string `yaml:"anim"`
}

type NPC struct {
	Key       string             `yaml:"key"`
	Name      string             `yaml:"name"`
	Model     string             `yaml:"model"`
	Level     int                `yaml:"level"`
	HP        int                `yaml:"hp"`
	Speed     float64            `yaml:"speed"`
	Exp       int                `yaml:"exp"`
	RespawnMS int                `yaml:"respawn_ms"`
	AI        string             `yaml:"ai"`
	Stats     map[string]float64 `yaml:"stats"`
	Attack    NPCAttack          `yaml:"attack"`

	// AISource is the Lua script text, loaded from AI.
	AISource string `yaml:"-"`
}

type NPCGroup struct {
	NPC    string     `yaml:"npc"`
	Count  int        `yaml:"count"`
	Center [2]float64 `yaml:"center"`
	Radius float64    `yaml:"radius"`
}

type Zone struct {
	ID          string     `yaml:"id"`
	HalfExtent  float64    `yaml:"half_extent"`
	Spawn       [2]float64 `yaml:"spawn"`
	SpawnRadius float64    `yaml:"spawn_radius"`
	NPCGroups   []NPCGroup `yaml:"npc_groups"`
	// Terrain is nil for a flat zone.
	Terrain *ZoneTerrain `yaml:"terrain"`
}

// ZoneTerrain describes the heightmap file and the knobs terraingen used to
// build it; the walking rules (slope, water) are applied at runtime.
type ZoneTerrain struct {
	File        string  `yaml:"file"`
	MaxSlopeDeg float64 `yaml:"max_slope_deg"`
	WaterLevel  float64 `yaml:"water_level"`
	WadeDepth   float64 `yaml:"wade_depth"`

	// Generation (cmd/terraingen).
	Seed            uint64  `yaml:"seed"`
	Spacing         float64 `yaml:"spacing"`
	Margin          float64 `yaml:"margin"`
	HillHeight      float64 `yaml:"hill_height"`
	HillScale       float64 `yaml:"hill_scale"`
	TerraceStep     float64 `yaml:"terrace_step"`
	TerraceStrength float64 `yaml:"terrace_strength"`
	BorderHeight    float64 `yaml:"border_height"`
	Lakes           []Lake  `yaml:"lakes"`
}

type Lake struct {
	Center [2]float64 `yaml:"center"`
	Radius float64    `yaml:"radius"`
	Depth  float64    `yaml:"depth"`
}

type Item struct {
	Key    string `yaml:"key"`
	Name   string `yaml:"name"`
	Rarity string `yaml:"rarity"`
}

// Fate configures the luck/karma event system (see content/fate.yaml).
type FateTrigger struct {
	Chance float64 `yaml:"chance"`
	// travel: meters walked between rolls.
	EveryM float64 `yaml:"every_m"`
	// rest: seconds standing still before a roll.
	AfterS float64 `yaml:"after_s"`
	// wounded: fraction of max hp to fall below.
	BelowHP float64 `yaml:"below_hp"`
}

type Fate struct {
	Deck struct {
		Size        int     `yaml:"size"`
		RefillBelow int     `yaml:"refill_below"`
		MinRefillS  float64 `yaml:"min_refill_s"`
	} `yaml:"deck"`
	CooldownS   float64                `yaml:"cooldown_s"`
	Triggers    map[string]FateTrigger `yaml:"triggers"`
	FortuneOdds struct {
		LuckWeight  float64 `yaml:"luck_weight"`
		KarmaWeight float64 `yaml:"karma_weight"`
		Min         float64 `yaml:"min"`
		Max         float64 `yaml:"max"`
	} `yaml:"fortune_odds"`
	Karma struct {
		KillHostileNPC int `yaml:"kill_hostile_npc"`
		Min            int `yaml:"min"`
		Max            int `yaml:"max"`
	} `yaml:"karma"`
	// Items by magnitude (1-3) for treasure, spoils and hidden caches.
	Loot    map[int][]string       `yaml:"loot"`
	Actions map[string]*FateAction `yaml:"actions"`
}

// FateAction is one entry of the catalog in content/fate.yaml. Only the
// fields its kind uses are set.
type FateAction struct {
	Omen     string   `yaml:"omen"` // fortune or misfortune
	Triggers []string `yaml:"triggers"`

	ExpPerMagnitude    int        `yaml:"exp_per_magnitude"`
	DurationS          float64    `yaml:"duration_s"`
	BonusPerMagnitude  float64    `yaml:"bonus_per_magnitude"`
	HPLossPerMagnitude float64    `yaml:"hp_loss_per_magnitude"`
	DistanceM          [2]float64 `yaml:"distance_m"`
	ExpireS            float64    `yaml:"expire_s"`
	NPC                string     `yaml:"npc"`
}

// Action kinds the zone knows how to carry out, and the triggers it raises.
var (
	FateActionKinds = map[string]bool{
		"treasure": true, "blessing": true, "insight": true, "critical": true, "weakness": true,
		"vigor_aura": true, "hidden_cache": true, "spoils": true,
		"ambush": true, "mishap": true, "fumble": true, "enrage": true, "ill_omen": true,
	}
	FateTriggers = map[string]bool{"strike": true, "kill": true, "travel": true, "rest": true, "wounded": true}
)

type Content struct {
	Skills map[uint32]*Skill
	NPCs   map[string]*NPC
	Zones  map[string]*Zone
	Items  map[string]*Item
	// Classes by key; empty when content/classes is absent.
	Classes map[string]*Class
	// Fate is nil when content/fate.yaml is absent.
	Fate *Fate
}

// SkillByKey finds a skill by its key.
func (c *Content) SkillByKey(key string) *Skill {
	for _, s := range c.Skills {
		if s.Key == key {
			return s
		}
	}
	return nil
}

// Class returns the class, or the default one for unknown keys.
func (c *Content) Class(key string) *Class {
	if cl, ok := c.Classes[key]; ok {
		return cl
	}
	return c.Classes[DefaultClass]
}

// Load reads dir/skills.yaml, dir/npcs/*.yaml, dir/zones/*.yaml and the AI
// scripts they reference.
func Load(dir string) (*Content, error) {
	c := &Content{Skills: map[uint32]*Skill{}, NPCs: map[string]*NPC{}, Zones: map[string]*Zone{}, Items: map[string]*Item{}, Classes: map[string]*Class{}}

	var skills struct {
		Skills []*Skill `yaml:"skills"`
	}
	if err := readYAML(filepath.Join(dir, "skills.yaml"), &skills); err != nil {
		return nil, err
	}
	for _, s := range skills.Skills {
		if _, dup := c.Skills[s.ID]; dup {
			return nil, fmt.Errorf("skills.yaml: duplicate skill id %d", s.ID)
		}
		if s.Script != "" {
			src, err := os.ReadFile(filepath.Join(dir, s.Script))
			if err != nil {
				return nil, fmt.Errorf("skill %s: script: %w", s.Key, err)
			}
			s.ScriptSource = string(src)
		}
		c.Skills[s.ID] = s
	}

	classFiles, _ := filepath.Glob(filepath.Join(dir, "classes", "*.yaml"))
	for _, f := range classFiles {
		cl := &Class{}
		if err := readYAML(f, cl); err != nil {
			return nil, err
		}
		c.Classes[cl.Key] = cl
	}

	npcFiles, _ := filepath.Glob(filepath.Join(dir, "npcs", "*.yaml"))
	for _, f := range npcFiles {
		n := &NPC{}
		if err := readYAML(f, n); err != nil {
			return nil, err
		}
		src, err := os.ReadFile(filepath.Join(dir, n.AI))
		if err != nil {
			return nil, fmt.Errorf("%s: ai script: %w", f, err)
		}
		n.AISource = string(src)
		c.NPCs[n.Key] = n
	}

	zoneFiles, _ := filepath.Glob(filepath.Join(dir, "zones", "*.yaml"))
	for _, f := range zoneFiles {
		z := &Zone{}
		if err := readYAML(f, z); err != nil {
			return nil, err
		}
		c.Zones[z.ID] = z
	}

	var items struct {
		Items []*Item `yaml:"items"`
	}
	if err := readYAML(filepath.Join(dir, "items.yaml"), &items); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, it := range items.Items {
		c.Items[it.Key] = it
	}

	fate := &Fate{}
	switch err := readYAML(filepath.Join(dir, "fate.yaml"), fate); {
	case err == nil:
		c.Fate = fate
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}

	return c, c.validate()
}

func (c *Content) validate() error {
	var errs []error
	for id, s := range c.Skills {
		switch {
		case s.Target != "enemy" && s.Target != "self":
			errs = append(errs, fmt.Errorf("skill %d (%s): target must be enemy or self", id, s.Key))
		case s.Target == "enemy" && s.Range <= 0:
			errs = append(errs, fmt.Errorf("skill %d (%s): enemy skills need a range", id, s.Key))
		case s.CooldownMS <= 0:
			errs = append(errs, fmt.Errorf("skill %d (%s): cooldown must be positive", id, s.Key))
		case s.Damage[1] < s.Damage[0] || s.Heal[1] < s.Heal[0] || s.Cost < 0:
			errs = append(errs, fmt.Errorf("skill %d (%s): bad damage, heal or cost", id, s.Key))
		case s.Script == "" && s.Damage[1] == 0 && s.Heal[1] == 0 && s.Apply == nil:
			errs = append(errs, fmt.Errorf("skill %d (%s): does nothing (no damage, heal, apply or script)", id, s.Key))
		}
		if s.Apply != nil && (!SkillEffects[s.Apply.Effect] || s.Apply.DurationS <= 0) {
			errs = append(errs, fmt.Errorf("skill %d (%s): unknown effect %q or no duration", id, s.Key, s.Apply.Effect))
		}
	}
	for key, cl := range c.Classes {
		if _, ok := Worlds[cl.World]; !ok {
			errs = append(errs, fmt.Errorf("class %s: unknown world %q", key, cl.World))
		}
		if cl.HP.Base <= 0 || cl.Resource.Max <= 0 || len(cl.Skills) == 0 {
			errs = append(errs, fmt.Errorf("class %s: needs hp, a resource and skills", key))
		}
		cl.SkillIDs = cl.SkillIDs[:0]
		for _, sk := range cl.Skills {
			s := c.SkillByKey(sk)
			if s == nil {
				errs = append(errs, fmt.Errorf("class %s: unknown skill %q", key, sk))
				continue
			}
			cl.SkillIDs = append(cl.SkillIDs, s.ID)
		}
	}
	if len(c.Classes) > 0 {
		if _, ok := c.Classes[DefaultClass]; !ok {
			errs = append(errs, fmt.Errorf("default class %q is missing", DefaultClass))
		}
	}
	for key, n := range c.NPCs {
		if n.HP <= 0 || n.Speed <= 0 || n.Attack.CooldownMS <= 0 || n.Attack.Damage[1] < n.Attack.Damage[0] {
			errs = append(errs, fmt.Errorf("npc %s: hp, speed and attack must be positive", key))
		}
		for _, stat := range []string{"attack_range", "aggro_radius", "leash_radius", "wander_radius"} {
			if _, ok := n.Stats[stat]; !ok {
				errs = append(errs, fmt.Errorf("npc %s: missing stat %q", key, stat))
			}
		}
	}
	for id, z := range c.Zones {
		if z.HalfExtent <= 0 {
			errs = append(errs, fmt.Errorf("zone %s: half_extent must be positive", id))
		}
		if t := z.Terrain; t != nil && (t.File == "" || t.MaxSlopeDeg <= 0 || t.MaxSlopeDeg >= 90 || t.Spacing <= 0) {
			errs = append(errs, fmt.Errorf("zone %s: terrain needs file, spacing and max_slope_deg in (0, 90)", id))
		}
		for i, g := range z.NPCGroups {
			if _, ok := c.NPCs[g.NPC]; !ok {
				errs = append(errs, fmt.Errorf("zone %s group %d: unknown npc %q", id, i, g.NPC))
			}
		}
	}
	if f := c.Fate; f != nil {
		if f.Deck.Size <= 0 || f.Deck.RefillBelow < 0 || f.Deck.MinRefillS < 0 || f.CooldownS < 0 {
			errs = append(errs, errors.New("fate: deck size must be positive; refill and cooldown not negative"))
		}
		for name, t := range f.Triggers {
			if !FateTriggers[name] {
				errs = append(errs, fmt.Errorf("fate: unknown trigger %q", name))
			}
			if t.Chance < 0 || t.Chance > 1 {
				errs = append(errs, fmt.Errorf("fate: trigger %s chance must be in [0,1]", name))
			}
		}
		for name, a := range f.Actions {
			if !FateActionKinds[name] {
				errs = append(errs, fmt.Errorf("fate: action %q is not implemented by the server", name))
			}
			if a.Omen != "fortune" && a.Omen != "misfortune" {
				errs = append(errs, fmt.Errorf("fate: action %s: omen must be fortune or misfortune", name))
			}
			if len(a.Triggers) == 0 {
				errs = append(errs, fmt.Errorf("fate: action %s: needs at least one trigger", name))
			}
			for _, t := range a.Triggers {
				if _, ok := f.Triggers[t]; !ok {
					errs = append(errs, fmt.Errorf("fate: action %s: trigger %q is not configured", name, t))
				}
			}
			if a.NPC != "" {
				if _, ok := c.NPCs[a.NPC]; !ok {
					errs = append(errs, fmt.Errorf("fate: action %s: unknown npc %q", name, a.NPC))
				}
			}
		}
		for mag := 1; mag <= 3; mag++ {
			if len(f.Loot[mag]) == 0 {
				errs = append(errs, fmt.Errorf("fate: loot table for magnitude %d is empty", mag))
			}
			for _, key := range f.Loot[mag] {
				if _, ok := c.Items[key]; !ok {
					errs = append(errs, fmt.Errorf("fate: loot %d: unknown item %q", mag, key))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
