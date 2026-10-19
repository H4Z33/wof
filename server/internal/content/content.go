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
	ID         uint32  `yaml:"id"`
	Key        string  `yaml:"key"`
	Name       string  `yaml:"name"`
	Range      float64 `yaml:"range"`
	CooldownMS int     `yaml:"cooldown_ms"`
	Damage     Range   `yaml:"damage"`
	Anim       string  `yaml:"anim"`
	Projectile bool    `yaml:"projectile"`
}

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
}

type Content struct {
	Skills map[uint32]*Skill
	NPCs   map[string]*NPC
	Zones  map[string]*Zone
}

// Load reads dir/skills.yaml, dir/npcs/*.yaml, dir/zones/*.yaml and the AI
// scripts they reference.
func Load(dir string) (*Content, error) {
	c := &Content{Skills: map[uint32]*Skill{}, NPCs: map[string]*NPC{}, Zones: map[string]*Zone{}}

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
		c.Skills[s.ID] = s
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

	return c, c.validate()
}

func (c *Content) validate() error {
	var errs []error
	for id, s := range c.Skills {
		if s.Range <= 0 || s.CooldownMS <= 0 || s.Damage[0] <= 0 || s.Damage[1] < s.Damage[0] {
			errs = append(errs, fmt.Errorf("skill %d (%s): range, cooldown and damage must be positive", id, s.Key))
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
		for i, g := range z.NPCGroups {
			if _, ok := c.NPCs[g.NPC]; !ok {
				errs = append(errs, fmt.Errorf("zone %s group %d: unknown npc %q", id, i, g.NPC))
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
