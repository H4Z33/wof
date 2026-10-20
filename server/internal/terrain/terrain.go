// Package terrain holds a zone's heightmap and the walking rules that the
// server and the client (client/game/terrain.gd) both apply, so client-side
// prediction matches what the server allows.
//
// File format (little-endian):
//
//	magic   [4]byte "CVTR"
//	version uint16  (1)
//	width   uint16  samples along X
//	depth   uint16  samples along Z
//	_       uint16  reserved
//	spacing float32 meters between samples
//	originX float32 world X of sample (0, 0)
//	originZ float32 world Z of sample (0, 0)
//	heights [width*depth]float32, row-major by Z then X
package terrain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
)

const (
	magic   = "CVTR"
	version = 1
)

// Rules are the walking limits; they are sent to the client in Welcome.
type Rules struct {
	// MaxSlope is the steepest climb allowed, as rise over run (tan of the angle).
	MaxSlope float64
	// WaterLevel is the lake surface height.
	WaterLevel float64
	// WadeDepth is how deep players may walk into water.
	WadeDepth float64
}

type Terrain struct {
	Width, Depth     int
	Spacing          float64
	OriginX, OriginZ float64
	Heights          []float32
	Rules            Rules
	// SHA256 of the file, so clients can check they have the same terrain.
	Hash string
}

// Load reads a terrain file.
func Load(path string, rules Rules) (*Terrain, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	t, err := Decode(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	t.Rules = rules
	return t, nil
}

// Decode parses terrain file bytes.
func Decode(b []byte) (*Terrain, error) {
	r := bytes.NewReader(b)
	var hdr struct {
		Magic                     [4]byte
		Version, Width, Depth, _  uint16
		Spacing, OriginX, OriginZ float32
	}
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return nil, err
	}
	if string(hdr.Magic[:]) != magic || hdr.Version != version {
		return nil, errors.New("not a version 1 terrain file")
	}
	if hdr.Width < 2 || hdr.Depth < 2 || hdr.Spacing <= 0 {
		return nil, errors.New("bad terrain dimensions")
	}
	t := &Terrain{
		Width: int(hdr.Width), Depth: int(hdr.Depth), Spacing: float64(hdr.Spacing),
		OriginX: float64(hdr.OriginX), OriginZ: float64(hdr.OriginZ),
		Heights: make([]float32, int(hdr.Width)*int(hdr.Depth)),
	}
	if err := binary.Read(r, binary.LittleEndian, t.Heights); err != nil {
		return nil, fmt.Errorf("heights: %w", err)
	}
	if _, err := r.ReadByte(); err != io.EOF {
		return nil, errors.New("trailing bytes after heights")
	}
	sum := sha256.Sum256(b)
	t.Hash = hex.EncodeToString(sum[:])
	return t, nil
}

// Encode writes the terrain in the file format.
func (t *Terrain) Encode() []byte {
	var buf bytes.Buffer
	buf.WriteString(magic)
	binary.Write(&buf, binary.LittleEndian, []uint16{version, uint16(t.Width), uint16(t.Depth), 0})
	binary.Write(&buf, binary.LittleEndian, []float32{float32(t.Spacing), float32(t.OriginX), float32(t.OriginZ)})
	binary.Write(&buf, binary.LittleEndian, t.Heights)
	return buf.Bytes()
}

func (t *Terrain) at(ix, iz int) float64 {
	ix = max(0, min(t.Width-1, ix))
	iz = max(0, min(t.Depth-1, iz))
	return float64(t.Heights[iz*t.Width+ix])
}

// Height is the ground height at (x, z), bilinearly interpolated.
// Keep in sync with terrain.gd height().
func (t *Terrain) Height(x, z float64) float64 {
	fx := (x - t.OriginX) / t.Spacing
	fz := (z - t.OriginZ) / t.Spacing
	ix, iz := int(math.Floor(fx)), int(math.Floor(fz))
	tx, tz := fx-float64(ix), fz-float64(iz)
	h00, h10 := t.at(ix, iz), t.at(ix+1, iz)
	h01, h11 := t.at(ix, iz+1), t.at(ix+1, iz+1)
	a := h00 + (h10-h00)*tx
	b := h01 + (h11-h01)*tx
	return a + (b-a)*tz
}

// CanStep reports whether walking from a to b (ground positions, y ignored)
// is allowed: not too steep uphill and not into deep water.
// Keep in sync with terrain.gd can_step().
func (t *Terrain) CanStep(ax, az, bx, bz float64) bool {
	hb := t.Height(bx, bz)
	if hb < t.Rules.WaterLevel-t.Rules.WadeDepth {
		return false
	}
	run := math.Hypot(bx-ax, bz-az)
	if run == 0 {
		return true
	}
	rise := hb - t.Height(ax, az)
	return rise <= run*t.Rules.MaxSlope
}

// Move walks from (x, z) by (dx, dz). If the full step is blocked it slides
// along X or Z alone; if both are blocked it stays. Returns the new ground
// position. Keep in sync with terrain.gd move().
func (t *Terrain) Move(x, z, dx, dz float64) (float64, float64) {
	switch {
	case t.CanStep(x, z, x+dx, z+dz):
		return x + dx, z + dz
	case dx != 0 && t.CanStep(x, z, x+dx, z):
		return x + dx, z
	case dz != 0 && t.CanStep(x, z, x, z+dz):
		return x, z + dz
	}
	return x, z
}

// Walkable reports whether (x, z) is good ground to stand on and to place
// things: not deep water, and the local slope is within the climbing limit.
// (Steeper ground can still be crossed going downhill.)
func (t *Terrain) Walkable(x, z float64) bool {
	if t.Height(x, z) < t.Rules.WaterLevel-t.Rules.WadeDepth {
		return false
	}
	d := t.Spacing
	gx := (t.Height(x+d, z) - t.Height(x-d, z)) / (2 * d)
	gz := (t.Height(x, z+d) - t.Height(x, z-d)) / (2 * d)
	return math.Hypot(gx, gz) <= t.Rules.MaxSlope
}

// RandomWalkable picks a walkable point within radius of (cx, cz); ok is
// false if none was found in a few tries.
func (t *Terrain) RandomWalkable(cx, cz, radius float64) (x, z float64, ok bool) {
	for range 30 {
		a := rand.Float64() * 2 * math.Pi
		r := radius * math.Sqrt(rand.Float64())
		x, z = cx+r*math.Cos(a), cz+r*math.Sin(a)
		if t.Walkable(x, z) {
			return x, z, true
		}
	}
	return cx, cz, false
}
