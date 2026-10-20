// Command terraingen generates a zone's heightmap from its content file and
// writes it for both the server (content/zones) and the client
// (client/assets/terrain). It is deterministic for a given seed.
//
//	go run ./cmd/terraingen -zone jade_terraces
//
// It also checks the result: every NPC camp must be reachable on foot from
// the player spawn and back, using the same walking rules as the game.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"convergence/server/internal/content"
	"convergence/server/internal/terrain"
)

func main() {
	contentDir := flag.String("content", "../content", "content directory")
	clientDir := flag.String("client", "../client/assets/terrain", "client terrain directory")
	zoneID := flag.String("zone", "jade_terraces", "zone to generate")
	preview := flag.String("preview", "", "also write a shaded PNG preview to this path")
	flag.Parse()

	c, err := content.Load(*contentDir)
	if err != nil {
		fail(err)
	}
	zc, ok := c.Zones[*zoneID]
	if !ok {
		fail(fmt.Errorf("unknown zone %q", *zoneID))
	}
	if zc.Terrain == nil {
		fail(fmt.Errorf("zone %s has no terrain section", zc.ID))
	}
	t := generate(zc)
	if err := check(t, zc); err != nil {
		fail(err)
	}
	if *preview != "" {
		if err := writePreview(t, zc, *preview); err != nil {
			fail(err)
		}
		fmt.Println("wrote", *preview)
	}
	data := t.Encode()
	for _, dir := range []string{filepath.Join(*contentDir, "zones"), *clientDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail(err)
		}
		path := filepath.Join(dir, zc.Terrain.File)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fail(err)
		}
		fmt.Println("wrote", path)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "terraingen:", err)
	os.Exit(1)
}

// area is a circle flattened to its average height (spawn plaza, camps).
type area struct {
	x, z, r float64
	name    string
}

func generate(zc *content.Zone) *terrain.Terrain {
	tc := zc.Terrain
	n := newNoise(tc.Seed)
	size := 2 * (zc.HalfExtent + tc.Margin)
	samples := int(size/tc.Spacing) + 1
	t := &terrain.Terrain{
		Width: samples, Depth: samples, Spacing: tc.Spacing,
		OriginX: -size / 2, OriginZ: -size / 2,
		Heights: make([]float32, samples*samples),
		Rules:   rulesFor(zc),
	}

	base := func(x, z float64) float64 {
		h := tc.HillHeight*n.fbm(x/tc.HillScale, z/tc.HillScale, 4) +
			0.35*tc.HillHeight*n.fbm(x/(tc.HillScale*0.4)+100, z/(tc.HillScale*0.4)+100, 3)
		// Terraces: flat steps joined by short ramps, like rice paddies.
		if tc.TerraceStep > 0 {
			q := h / tc.TerraceStep
			f := q - math.Floor(q)
			stepped := (math.Floor(q) + smoothstep(0.55, 1, f)) * tc.TerraceStep
			h += (stepped - h) * tc.TerraceStrength
		}
		return h
	}

	var areas []area
	areas = append(areas, area{zc.Spawn[0], zc.Spawn[1], zc.SpawnRadius + 6, "spawn"})
	for i, g := range zc.NPCGroups {
		areas = append(areas, area{g.Center[0], g.Center[1], g.Radius + 3, fmt.Sprintf("camp %d (%s)", i, g.NPC)})
	}
	flatHeight := make([]float64, len(areas))
	for i, a := range areas {
		flatHeight[i] = base(a.x, a.z)
	}

	for iz := range samples {
		for ix := range samples {
			x := t.OriginX + float64(ix)*tc.Spacing
			z := t.OriginZ + float64(iz)*tc.Spacing
			h := base(x, z)
			for _, l := range tc.Lakes {
				d := math.Hypot(x-l.Center[0], z-l.Center[1])
				h += (l.Depth - h) * (1 - smoothstep(l.Radius*0.5, l.Radius, d))
			}
			for i, a := range areas {
				d := math.Hypot(x-a.x, z-a.z)
				h += (flatHeight[i] - h) * (1 - smoothstep(a.r, a.r+8, d))
			}
			// Mountains past the walkable edge make a natural border.
			edge := max(math.Abs(x), math.Abs(z))
			h += tc.BorderHeight * smoothstep(zc.HalfExtent-8, zc.HalfExtent+tc.Margin*0.5, edge)
			t.Heights[iz*samples+ix] = float32(h)
		}
	}
	// Re-read through the file format so hashes and float32 rounding match
	// exactly what the game loads.
	out, err := terrain.Decode(t.Encode())
	if err != nil {
		fail(err)
	}
	out.Rules = t.Rules
	return out
}

func rulesFor(zc *content.Zone) terrain.Rules {
	return terrain.Rules{
		MaxSlope:   math.Tan(zc.Terrain.MaxSlopeDeg * math.Pi / 180),
		WaterLevel: zc.Terrain.WaterLevel,
		WadeDepth:  zc.Terrain.WadeDepth,
	}
}

// check walks the map with the game's rules: each camp must be reachable
// from spawn and spawn from each camp.
func check(t *terrain.Terrain, zc *content.Zone) error {
	const cell = 1.0
	lim := zc.HalfExtent
	cells := int(2*lim/cell) + 1
	idx := func(x, z float64) (int, int) {
		return int(math.Round((x + lim) / cell)), int(math.Round((z + lim) / cell))
	}
	pos := func(i, j int) (float64, float64) { return -lim + float64(i)*cell, -lim + float64(j)*cell }
	reach := func(sx, sz float64) []bool {
		seen := make([]bool, cells*cells)
		i0, j0 := idx(sx, sz)
		queue := [][2]int{{i0, j0}}
		seen[j0*cells+i0] = true
		for len(queue) > 0 {
			c := queue[0]
			queue = queue[1:]
			x, z := pos(c[0], c[1])
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
				ni, nj := c[0]+d[0], c[1]+d[1]
				if ni < 0 || nj < 0 || ni >= cells || nj >= cells || seen[nj*cells+ni] {
					continue
				}
				nx, nz := pos(ni, nj)
				if canWalk(t, x, z, nx, nz) {
					seen[nj*cells+ni] = true
					queue = append(queue, [2]int{ni, nj})
				}
			}
		}
		return seen
	}

	fromSpawn := reach(zc.Spawn[0], zc.Spawn[1])
	reachable, walkable := 0, 0
	for j := range cells {
		for i := range cells {
			x, z := pos(i, j)
			if t.Walkable(x, z) {
				walkable++
			}
			if fromSpawn[j*cells+i] {
				reachable++
			}
		}
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, h := range t.Heights {
		lo, hi = min(lo, float64(h)), max(hi, float64(h))
	}
	fmt.Printf("terrain %dx%d at %.2f m, heights %.1f..%.1f m, walkable %.0f%%, reachable from spawn %.0f%%\n",
		t.Width, t.Depth, t.Spacing, lo, hi, 100*float64(walkable)/float64(cells*cells), 100*float64(reachable)/float64(cells*cells))

	for i, g := range zc.NPCGroups {
		gi, gj := idx(g.Center[0], g.Center[1])
		if !fromSpawn[gj*cells+gi] {
			return fmt.Errorf("camp %d (%s at %v) is not reachable from spawn", i, g.NPC, g.Center)
		}
		back := reach(g.Center[0], g.Center[1])
		si, sj := idx(zc.Spawn[0], zc.Spawn[1])
		if !back[sj*cells+si] {
			return fmt.Errorf("spawn is not reachable from camp %d (%s)", i, g.NPC)
		}
	}
	fmt.Println("all camps reachable from spawn and back")
	return nil
}

// canWalk follows the path between two cells in small steps, as a player would.
func canWalk(t *terrain.Terrain, ax, az, bx, bz float64) bool {
	const steps = 4
	for s := range steps {
		x0 := ax + (bx-ax)*float64(s)/steps
		z0 := az + (bz-az)*float64(s)/steps
		x1 := ax + (bx-ax)*float64(s+1)/steps
		z1 := az + (bz-az)*float64(s+1)/steps
		if !t.CanStep(x0, z0, x1, z1) {
			return false
		}
	}
	return true
}

func smoothstep(e0, e1, x float64) float64 {
	t := max(0, min(1, (x-e0)/(e1-e0)))
	return t * t * (3 - 2*t)
}

// noise is seeded 2D value noise.
type noise struct{ seed uint64 }

func newNoise(seed uint64) noise { return noise{seed: seed} }

func (n noise) lattice(ix, iz int64) float64 {
	h := uint64(ix)*0x9E3779B97F4A7C15 ^ uint64(iz)*0xC2B2AE3D27D4EB4F ^ n.seed
	h ^= h >> 31
	h *= 0xBF58476D1CE4E5B9
	h ^= h >> 29
	return float64(h>>11)/(1<<53)*2 - 1
}

func (n noise) value(x, z float64) float64 {
	ix, iz := int64(math.Floor(x)), int64(math.Floor(z))
	tx, tz := x-math.Floor(x), z-math.Floor(z)
	sx, sz := tx*tx*(3-2*tx), tz*tz*(3-2*tz)
	a := n.lattice(ix, iz) + (n.lattice(ix+1, iz)-n.lattice(ix, iz))*sx
	b := n.lattice(ix, iz+1) + (n.lattice(ix+1, iz+1)-n.lattice(ix, iz+1))*sx
	return a + (b-a)*sz
}

// fbm sums octaves of noise, roughly in [-1, 1].
func (n noise) fbm(x, z float64, octaves int) float64 {
	sum, amp, norm := 0.0, 1.0, 0.0
	for range octaves {
		sum += amp * n.value(x, z)
		norm += amp
		amp *= 0.5
		x, z = x*2+17.3, z*2-9.1
	}
	return sum / norm
}
