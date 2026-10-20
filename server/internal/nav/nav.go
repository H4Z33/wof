// Package nav finds walking paths over a terrain for NPCs. At load it
// builds a grid over the walkable area and records, for each cell, which of
// its 8 neighbors can be reached on foot under the terrain's rules (uphill
// limits are one-way, so edges are directed). Paths are found with A* and
// then shortened by skipping waypoints that can be walked in a straight line.
package nav

import (
	"container/heap"
	"math"

	"convergence/server/internal/terrain"
)

// Directions in the edge bitmask.
var dirs = [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

type Grid struct {
	t      *terrain.Terrain
	cell   float64
	min    float64 // world coordinate of cell 0 on both axes
	n      int     // cells per side
	edges  []uint8 // per cell: bit d set if neighbor dirs[d] is reachable
	stand  []bool  // per cell: walkable to stand on
	budget int     // max A* expansions per query
}

// Build covers [-halfExtent, halfExtent] on both axes with square cells.
func Build(t *terrain.Terrain, halfExtent, cell float64) *Grid {
	n := int(2*halfExtent/cell) + 1
	g := &Grid{t: t, cell: cell, min: -halfExtent, n: n, edges: make([]uint8, n*n), stand: make([]bool, n*n), budget: 20000}
	for j := range n {
		for i := range n {
			x, z := g.pos(i, j)
			g.stand[j*n+i] = t.Height(x, z) >= t.Rules.WaterLevel-t.Rules.WadeDepth
		}
	}
	for j := range n {
		for i := range n {
			if !g.stand[j*n+i] {
				continue
			}
			x, z := g.pos(i, j)
			var mask uint8
			for d, o := range dirs {
				ni, nj := i+o[0], j+o[1]
				if ni < 0 || nj < 0 || ni >= n || nj >= n || !g.stand[nj*n+ni] {
					continue
				}
				nx, nz := g.pos(ni, nj)
				if g.Straight(x, z, nx, nz) {
					mask |= 1 << d
				}
			}
			g.edges[j*n+i] = mask
		}
	}
	return g
}

func (g *Grid) pos(i, j int) (float64, float64) {
	return g.min + float64(i)*g.cell, g.min + float64(j)*g.cell
}

func (g *Grid) index(x, z float64) (int, int, bool) {
	i := int(math.Round((x - g.min) / g.cell))
	j := int(math.Round((z - g.min) / g.cell))
	return i, j, i >= 0 && j >= 0 && i < g.n && j < g.n
}

// Straight reports whether a walker can go from a to b in a straight line,
// checked in small steps with the terrain rules.
func (g *Grid) Straight(ax, az, bx, bz float64) bool {
	d := math.Hypot(bx-ax, bz-az)
	steps := max(1, int(math.Ceil(d/0.25)))
	px, pz := ax, az
	for s := 1; s <= steps; s++ {
		f := float64(s) / float64(steps)
		x, z := ax+(bx-ax)*f, az+(bz-az)*f
		if !g.t.CanStep(px, pz, x, z) {
			return false
		}
		px, pz = x, z
	}
	return true
}

type node struct {
	idx  int
	f    float64
	heap int
}

type openSet []*node

func (o openSet) Len() int           { return len(o) }
func (o openSet) Less(a, b int) bool { return o[a].f < o[b].f }
func (o openSet) Swap(a, b int)      { o[a], o[b] = o[b], o[a]; o[a].heap = a; o[b].heap = b }
func (o *openSet) Push(x any)        { n := x.(*node); n.heap = len(*o); *o = append(*o, n) }
func (o *openSet) Pop() any          { old := *o; n := old[len(old)-1]; *o = old[:len(old)-1]; return n }
func octile(di, dj int) float64 {
	a, b := abs(di), abs(dj)
	return float64(max(a, b)) + (math.Sqrt2-1)*float64(min(a, b))
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Path returns waypoints from (sx, sz) to (gx, gz), excluding the start and
// ending exactly at the goal. ok is false if the goal is unreachable or too
// far to search.
func (g *Grid) Path(sx, sz, gx, gz float64) ([][2]float64, bool) {
	if g.Straight(sx, sz, gx, gz) {
		return [][2]float64{{gx, gz}}, true
	}
	si, sj, ok1 := g.index(sx, sz)
	gi, gj, ok2 := g.index(gx, gz)
	if !ok1 || !ok2 || !g.stand[gj*g.n+gi] {
		return nil, false
	}
	start, goal := sj*g.n+si, gj*g.n+gi
	cost := map[int]float64{start: 0}
	came := map[int]int{}
	nodes := map[int]*node{}
	open := &openSet{}
	sn := &node{idx: start, f: octile(gi-si, gj-sj)}
	nodes[start] = sn
	heap.Push(open, sn)
	closed := map[int]bool{}
	for open.Len() > 0 && len(closed) < g.budget {
		cur := heap.Pop(open).(*node)
		if cur.idx == goal {
			return g.smooth(sx, sz, gx, gz, g.unwind(came, start, goal)), true
		}
		closed[cur.idx] = true
		ci, cj := cur.idx%g.n, cur.idx/g.n
		mask := g.edges[cur.idx]
		for d, o := range dirs {
			if mask&(1<<d) == 0 {
				continue
			}
			ni := (cj+o[1])*g.n + ci + o[0]
			if closed[ni] {
				continue
			}
			step := 1.0
			if o[0] != 0 && o[1] != 0 {
				step = math.Sqrt2
			}
			c := cost[cur.idx] + step
			if old, seen := cost[ni]; seen && c >= old {
				continue
			}
			cost[ni] = c
			came[ni] = cur.idx
			f := c + octile(gi-(ci+o[0]), gj-(cj+o[1]))
			if n, inOpen := nodes[ni]; inOpen && n.heap >= 0 && n.heap < open.Len() && (*open)[n.heap] == n {
				n.f = f
				heap.Fix(open, n.heap)
			} else {
				n := &node{idx: ni, f: f}
				nodes[ni] = n
				heap.Push(open, n)
			}
		}
	}
	return nil, false
}

func (g *Grid) unwind(came map[int]int, start, goal int) [][2]float64 {
	var rev [][2]float64
	for c := goal; c != start; c = came[c] {
		x, z := g.pos(c%g.n, c/g.n)
		rev = append(rev, [2]float64{x, z})
	}
	out := make([][2]float64, len(rev))
	for i := range rev {
		out[i] = rev[len(rev)-1-i]
	}
	return out
}

// smooth drops waypoints that can be skipped in a straight line, and makes
// the path end at the exact goal.
func (g *Grid) smooth(sx, sz, gx, gz float64, pts [][2]float64) [][2]float64 {
	if len(pts) > 0 {
		pts[len(pts)-1] = [2]float64{gx, gz}
	}
	var out [][2]float64
	cx, cz := sx, sz
	for i := 0; i < len(pts); {
		far := i
		for k := len(pts) - 1; k > i; k-- {
			if g.Straight(cx, cz, pts[k][0], pts[k][1]) {
				far = k
				break
			}
		}
		out = append(out, pts[far])
		cx, cz = pts[far][0], pts[far][1]
		i = far + 1
	}
	return out
}
