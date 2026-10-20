package terrain

import (
	"math"
	"testing"
)

// testTerrain is 21x21 samples, 1 m apart, covering x,z in [0, 20]:
// flat at 0 for x < 10, a gentle ramp (0.5 rise per meter) for 10 <= x < 14,
// then a 10 m cliff; a pit (water) around (4, 15).
func testTerrain() *Terrain {
	t := &Terrain{Width: 21, Depth: 21, Spacing: 1, Heights: make([]float32, 21*21),
		Rules: Rules{MaxSlope: math.Tan(40 * math.Pi / 180), WaterLevel: -1, WadeDepth: 0.5}}
	for z := range 21 {
		for x := range 21 {
			h := 0.0
			switch {
			case x >= 15:
				h = 12
			case x >= 10:
				h = 0.5 * float64(x-10)
			}
			if math.Hypot(float64(x-4), float64(z-15)) <= 2 {
				h = -3
			}
			t.Heights[z*21+x] = float32(h)
		}
	}
	return t
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	src := testTerrain()
	got, err := Decode(src.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 21 || got.Depth != 21 || got.Spacing != 1 || len(got.Heights) != 441 || got.Heights[20] != 12 {
		t.Fatalf("decoded %+v", got)
	}
	if len(got.Hash) != 64 {
		t.Errorf("hash %q", got.Hash)
	}
	if _, err := Decode(append(src.Encode(), 0)); err == nil {
		t.Error("trailing bytes accepted")
	}
	if _, err := Decode([]byte("nope")); err == nil {
		t.Error("garbage accepted")
	}
}

func TestHeightInterpolates(t *testing.T) {
	tr := testTerrain()
	if h := tr.Height(11.5, 3); math.Abs(h-0.75) > 1e-6 {
		t.Errorf("Height(11.5) = %v, want 0.75", h)
	}
	if h := tr.Height(-5, -5); h != 0 {
		t.Errorf("outside the map = %v, want edge height 0", h)
	}
}

func TestSlopeAndWater(t *testing.T) {
	tr := testTerrain()
	if !tr.CanStep(10, 3, 10.25, 3) {
		t.Error("cannot climb a 27° ramp")
	}
	if tr.CanStep(14, 3, 14.25, 3) {
		t.Error("climbed a cliff")
	}
	if !tr.CanStep(15.25, 3, 14.75, 3) {
		t.Error("cannot step down a cliff edge")
	}
	if tr.CanStep(4, 12.6, 4, 13.1) {
		t.Error("walked into deep water")
	}
}

func TestMoveSlidesAlongWalls(t *testing.T) {
	tr := testTerrain()
	// Diagonal into the cliff: the X part is blocked, Z still moves.
	x, z := tr.Move(14, 3, 0.25, 0.25)
	if x != 14 || z != 3.25 {
		t.Errorf("slide = (%v, %v), want (14, 3.25)", x, z)
	}
	// Straight into it: no movement.
	if x, z := tr.Move(14, 3, 0.25, 0); x != 14 || z != 3 {
		t.Errorf("moved into a cliff: (%v, %v)", x, z)
	}
}

func TestRandomWalkableAvoidsWater(t *testing.T) {
	tr := testTerrain()
	for range 50 {
		x, z, ok := tr.RandomWalkable(4, 15, 4)
		if ok && !tr.Walkable(x, z) {
			t.Fatalf("(%v, %v) not walkable", x, z)
		}
		if ok && math.Hypot(x-4, z-15) < 1.5 {
			t.Fatalf("(%v, %v) is in the pit", x, z)
		}
	}
}
