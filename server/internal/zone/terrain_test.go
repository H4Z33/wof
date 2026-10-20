package zone

import (
	"math"
	"testing"

	"convergence/server/internal/model"
)

// newTerrainZone uses the real jade_terraces content and heightmap.
func newTerrainZone(t *testing.T) *Zone {
	t.Helper()
	c := loadContent(t)
	zc := c.Zones["jade_terraces"]
	cfg := ConfigFor(zc)
	if err := LoadTerrain(&cfg, zc, "../../../content"); err != nil {
		t.Fatal(err)
	}
	z, err := New(cfg, c, zc, nil, quiet)
	if err != nil {
		t.Fatal(err)
	}
	return z
}

func TestEntitiesStandOnTheGround(t *testing.T) {
	z := newTerrainZone(t)
	p, _ := joinTest(t, z, "alice")
	if got, want := p.e.pos[1], z.groundY(p.e.pos[0], p.e.pos[2]); got != want {
		t.Errorf("player y = %v, ground %v", got, want)
	}
	for _, n := range z.npcs {
		if !z.standable(n.e.pos) || n.e.pos[1] != z.groundY(n.e.pos[0], n.e.pos[2]) {
			t.Errorf("npc at %v not standing on walkable ground", n.e.pos)
		}
	}
	// Walk around for a while: y keeps following the ground.
	for i := uint32(1); i <= 200; i++ {
		move(p, i, float32(math.Cos(float64(i)/20)), float32(math.Sin(float64(i)/20)))
		z.step()
		if p.e.pos[1] != z.groundY(p.e.pos[0], p.e.pos[2]) {
			t.Fatalf("tick %d: y %v off the ground", i, p.e.pos[1])
		}
	}
}

func TestBorderMountainsStopPlayers(t *testing.T) {
	z := newTerrainZone(t)
	p, _ := joinTest(t, z, "alice")
	for i := uint32(1); i <= 400; i++ { // 20 s straight north
		move(p, i, 0, -1)
		z.step()
	}
	if p.e.pos[2] <= -z.cfg.HalfExtent+1 {
		t.Errorf("walked to the map edge (z=%v) through the border mountains", p.e.pos[2])
	}
	if !z.cfg.Terrain.Walkable(p.e.pos[0], p.e.pos[2]) {
		t.Errorf("stopped on unwalkable ground at %v", p.e.pos)
	}
}

func TestJoinOnUnwalkableSavedPositionMovesToSpawn(t *testing.T) {
	z := newTerrainZone(t)
	// Somewhere in the border mountains.
	p, _ := joinChar(t, z, charAt(z, 58, 58))
	if hdist(p.e.pos, z.cfg.Spawn) > z.cfg.SpawnRadius+0.01 {
		t.Errorf("joined at %v, want near spawn %v", p.e.pos, z.cfg.Spawn)
	}
}

func charAt(z *Zone, x, zz float64) (c model.Character) {
	nextCharID++
	c.ID, c.Name, c.ZoneID, c.Level, c.HP = nextCharID, "bob", z.cfg.ID, 1, 100
	c.Pos = [3]float64{x, 0, zz}
	return c
}
