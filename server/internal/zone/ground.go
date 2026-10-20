package zone

import (
	"math"

	pb "convergence/server/gen/convergence/v1"
)

// groundY is the terrain height at (x, z); 0 on a flat zone.
func (z *Zone) groundY(x, zz float64) float64 {
	if t := z.cfg.Terrain; t != nil {
		return t.Height(x, zz)
	}
	return 0
}

// walk moves a ground position by (dx, dz): first clamped to the zone
// bounds, then through the terrain's walking rules (slope, water), sliding
// along an axis when blocked. Keep in sync with world.gd _simulate().
func (z *Zone) walk(pos [3]float64, dx, dz float64) [3]float64 {
	lim := z.cfg.HalfExtent
	tx := clamp(pos[0]+dx, -lim, lim)
	tz := clamp(pos[2]+dz, -lim, lim)
	x, zz := tx, tz
	if t := z.cfg.Terrain; t != nil {
		x, zz = t.Move(pos[0], pos[2], tx-pos[0], tz-pos[2])
	}
	return [3]float64{x, z.groundY(x, zz), zz}
}

// pointNear picks a standable point within radius of (cx, cz), on the ground.
func (z *Zone) pointNear(cx, cz, radius float64) [3]float64 {
	var x, zz float64
	if t := z.cfg.Terrain; t != nil {
		x, zz, _ = t.RandomWalkable(cx, cz, radius)
	} else {
		p := randomAround([3]float64{cx, 0, cz}, radius)
		x, zz = p[0], p[2]
	}
	lim := z.cfg.HalfExtent
	x, zz = clamp(x, -lim, lim), clamp(zz, -lim, lim)
	return [3]float64{x, z.groundY(x, zz), zz}
}

// standable reports whether a point can be stood on (always, when flat).
func (z *Zone) standable(pos [3]float64) bool {
	if t := z.cfg.Terrain; t != nil {
		return t.Walkable(pos[0], pos[2])
	}
	return true
}

func (z *Zone) terrainInfo() *pb.TerrainInfo {
	t := z.cfg.Terrain
	if t == nil {
		return nil
	}
	return &pb.TerrainInfo{
		File:       z.cfg.TerrainFile,
		Sha256:     t.Hash,
		MaxSlope:   float32(t.Rules.MaxSlope),
		WaterLevel: float32(t.Rules.WaterLevel),
		WadeDepth:  float32(t.Rules.WadeDepth),
	}
}

func degToSlope(deg float64) float64 { return math.Tan(deg * math.Pi / 180) }
