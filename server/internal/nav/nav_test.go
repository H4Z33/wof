package nav

import (
	"math"
	"testing"

	"convergence/server/internal/terrain"
)

// wallTerrain is flat ground [0,40]² with a 10 m wall along x=20 for z<30,
// so going from (10,10) to (30,10) needs a detour around z=30.
func wallTerrain() *terrain.Terrain {
	const n = 41
	t := &terrain.Terrain{Width: n, Depth: n, Spacing: 1, OriginX: -20, OriginZ: -20, Heights: make([]float32, n*n),
		Rules: terrain.Rules{MaxSlope: math.Tan(40 * math.Pi / 180), WaterLevel: -100}}
	for z := range n {
		for x := range n {
			if x >= 19 && x <= 21 && z < 30 {
				t.Heights[z*n+x] = 10
			}
		}
	}
	return t
}

func TestPathGoesAroundTheWall(t *testing.T) {
	g := Build(wallTerrain(), 20, 1)
	// World coords: terrain origin -20, so the wall is at x in [-1, 1] for z < 10.
	path, ok := g.Path(-10, -10, 10, -10)
	if !ok {
		t.Fatal("no path around the wall")
	}
	if last := path[len(path)-1]; last != [2]float64{10, -10} {
		t.Errorf("path ends at %v", last)
	}
	// Every leg must be walkable, and the path must pass the end of the wall.
	cx, cz := -10.0, -10.0
	passed := false
	for _, p := range path {
		if !g.Straight(cx, cz, p[0], p[1]) {
			t.Fatalf("leg (%v,%v)->%v is not walkable", cx, cz, p)
		}
		if p[1] >= 9 {
			passed = true
		}
		cx, cz = p[0], p[1]
	}
	if !passed {
		t.Errorf("path %v never goes around the wall end", path)
	}
	if len(path) > 4 {
		t.Errorf("path not smoothed: %d waypoints", len(path))
	}
}

func TestStraightPathIsOneWaypoint(t *testing.T) {
	g := Build(wallTerrain(), 20, 1)
	path, ok := g.Path(-10, 15, 10, 15)
	if !ok || len(path) != 1 {
		t.Errorf("path = %v ok=%v, want one straight leg", path, ok)
	}
}

func TestUnreachableGoal(t *testing.T) {
	g := Build(wallTerrain(), 20, 1)
	if _, ok := g.Path(-10, -10, 0, -15); ok { // on top of the wall: cannot climb
		t.Error("found a path up a cliff")
	}
}
