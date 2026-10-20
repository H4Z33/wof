package zone

import (
	pb "convergence/server/gen/convergence/v1"
	"convergence/server/internal/content"
)

// Renown and titles (content/titles.yaml). Renown grows with deeds; titles
// unlock with renown and, for some, karma. The chosen title shows under the
// name for everyone.

func (z *Zone) addRenown(p *player, n int) {
	if n == 0 || z.content.Titles == nil {
		return
	}
	p.renown = max(0, p.renown+n)
	p.dirty, p.progDirty = true, true
}

func (z *Zone) unlockedTitles(p *player) []content.Title {
	if z.content.Titles == nil {
		return nil
	}
	var out []content.Title
	for _, t := range z.content.Titles.Titles {
		if t.Unlocked(p.renown, p.karma) {
			out = append(out, t)
		}
	}
	return out
}

// titleName is the title shown for p, or "" (also when it no longer fits
// p's karma, like a hero turned wicked).
func (z *Zone) titleName(p *player) string {
	if p.title == "" || z.content.Titles == nil {
		return ""
	}
	for _, t := range z.content.Titles.Titles {
		if t.Key == p.title && t.Unlocked(p.renown, p.karma) {
			return t.Name
		}
	}
	return ""
}

func (z *Zone) chooseTitle(p *player, key string) {
	if key != "" {
		ok := false
		for _, t := range z.unlockedTitles(p) {
			ok = ok || t.Key == key
		}
		if !ok {
			return
		}
	}
	p.title = key
	p.dirty = true
	z.refreshLook(p.e)
	z.sendProgression(p)
}

// refreshLook makes every client that sees e receive its spawn data again
// (name, title), since those are only sent on spawn.
func (z *Zone) refreshLook(e *entity) {
	for _, o := range z.players {
		delete(o.known, e.id)
	}
}

func (z *Zone) titleInfos(p *player) []*pb.TitleInfo {
	var out []*pb.TitleInfo
	for _, t := range z.unlockedTitles(p) {
		out = append(out, &pb.TitleInfo{Key: t.Key, Name: t.Name})
	}
	return out
}
