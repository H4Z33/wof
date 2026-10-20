package zone

import (
	"context"
	"io/fs"
	"log/slog"
	"path/filepath"
	"time"

	"convergence/server/internal/content"
)

type reloadReq struct{ c *content.Content }

// Reload swaps in new content without restarting: skill and class numbers,
// NPC stats, items, Fate config, and AI/skill Lua scripts. Zone layout,
// terrain and spawn groups are not reloaded. If a script fails to compile
// the zone keeps the old content.
func (z *Zone) Reload(c *content.Content) {
	select {
	case z.inbox <- reloadReq{c: c}:
	case <-z.done:
	}
}

func (z *Zone) applyReload(c *content.Content) {
	old := z.content
	z.content = c
	if err := z.reloadScripts(); err != nil {
		z.log.Error("content reload rejected; keeping the previous version", "err", err)
		z.content = old
		z.reloadScripts()
		return
	}
	for _, n := range z.npcs {
		if def, ok := c.NPCs[n.def.Key]; ok {
			n.def = def
		}
	}
	for _, p := range z.players {
		if p.class != nil {
			p.class = c.Class(p.class.Key)
		}
	}
	z.log.Info("content reloaded", "skills", len(c.Skills), "npcs", len(c.NPCs), "classes", len(c.Classes))
}

func (z *Zone) reloadScripts() error {
	if err := z.loadLibs(); err != nil {
		return err
	}
	for _, def := range z.content.NPCs {
		if !z.aiLoaded[def.AI] {
			continue // loaded lazily when first used
		}
		if err := z.lua.Load(def.AI, def.AISource); err != nil {
			return err
		}
	}
	return z.loadSkillScripts()
}

// WatchContent reloads content into the zone whenever a file under dir
// changes. Meant for development (-dev).
func WatchContent(ctx context.Context, dir string, z *Zone, log *slog.Logger) {
	last := latestChange(dir)
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m := latestChange(dir)
			if !m.After(last) {
				continue
			}
			last = m
			c, err := content.Load(dir)
			if err != nil {
				log.Error("content changed but is invalid; not reloading", "err", err)
				continue
			}
			z.Reload(c)
		}
	}
}

func latestChange(dir string) time.Time {
	var latest time.Time
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	return latest
}
