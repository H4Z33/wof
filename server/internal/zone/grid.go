package zone

import "math"

// cellKey identifies one square cell of the interest grid.
type cellKey struct{ x, z int32 }

// grid buckets in-world entities by cell. A player sees the entities in its
// own cell and the 8 around it, so bandwidth depends on local density, not
// on how many players the zone holds.
type grid struct {
	size  float64
	cells map[cellKey]map[uint32]*entity
}

func newGrid(size float64) *grid {
	return &grid{size: size, cells: make(map[cellKey]map[uint32]*entity)}
}

func (g *grid) key(pos [3]float64) cellKey {
	return cellKey{int32(math.Floor(pos[0] / g.size)), int32(math.Floor(pos[2] / g.size))}
}

func (g *grid) add(e *entity) {
	e.cell = g.key(e.pos)
	c := g.cells[e.cell]
	if c == nil {
		c = make(map[uint32]*entity)
		g.cells[e.cell] = c
	}
	c[e.id] = e
	e.inWorld = true
}

func (g *grid) remove(e *entity) {
	if c := g.cells[e.cell]; c != nil {
		delete(c, e.id)
		if len(c) == 0 {
			delete(g.cells, e.cell)
		}
	}
	e.inWorld = false
}

// moved re-buckets e if it crossed into another cell.
func (g *grid) moved(e *entity) {
	if !e.inWorld || g.key(e.pos) == e.cell {
		return
	}
	g.remove(e)
	g.add(e)
}

// around calls fn for every entity in the 3x3 cells centered on pos.
func (g *grid) around(pos [3]float64, fn func(*entity)) {
	k := g.key(pos)
	for dx := int32(-1); dx <= 1; dx++ {
		for dz := int32(-1); dz <= 1; dz++ {
			for _, e := range g.cells[cellKey{k.x + dx, k.z + dz}] {
				fn(e)
			}
		}
	}
}
