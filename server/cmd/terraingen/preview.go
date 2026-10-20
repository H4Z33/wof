package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"convergence/server/internal/content"
	"convergence/server/internal/terrain"
)

// writePreview draws a top-down hillshade: water blue, ground green to
// brown by height, steep (unwalkable) slopes grey, spawn and camps marked.
// North (-Z) is up.
func writePreview(t *terrain.Terrain, zc *content.Zone, path string) error {
	img := image.NewRGBA(image.Rect(0, 0, t.Width, t.Depth))
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, h := range t.Heights {
		lo, hi = min(lo, float64(h)), max(hi, float64(h))
	}
	light := [3]float64{-0.5, 0.7, -0.5}
	for iz := range t.Depth {
		for ix := range t.Width {
			x := t.OriginX + float64(ix)*t.Spacing
			z := t.OriginZ + float64(iz)*t.Spacing
			h := t.Height(x, z)
			dx := t.Height(x+t.Spacing, z) - t.Height(x-t.Spacing, z)
			dz := t.Height(x, z+t.Spacing) - t.Height(x, z-t.Spacing)
			nx, ny, nz := -dx, 2*t.Spacing, -dz
			l := math.Sqrt(nx*nx + ny*ny + nz*nz)
			shade := max(0.25, (nx*light[0]+ny*light[1]+nz*light[2])/l)

			var r, g, b float64
			switch {
			case h < t.Rules.WaterLevel:
				depth := math.Min(1, (t.Rules.WaterLevel-h)/3)
				r, g, b = 40-20*depth, 110-40*depth, 170-30*depth
				shade = 1
			case !t.Walkable(x, z):
				r, g, b = 130, 125, 120
			default:
				f := (h - lo) / (hi - lo)
				r, g, b = 70+120*f, 130+40*f, 60+30*f
			}
			img.Set(ix, iz, color.RGBA{uint8(min(255, r*shade)), uint8(min(255, g*shade)), uint8(min(255, b*shade)), 255})
		}
	}
	mark := func(wx, wz, radius float64, c color.RGBA) {
		cx, cz := (wx-t.OriginX)/t.Spacing, (wz-t.OriginZ)/t.Spacing
		rr := radius / t.Spacing
		for a := 0.0; a < 2*math.Pi; a += 0.02 {
			img.Set(int(cx+rr*math.Cos(a)), int(cz+rr*math.Sin(a)), c)
		}
	}
	mark(zc.Spawn[0], zc.Spawn[1], zc.SpawnRadius+2, color.RGBA{255, 230, 80, 255})
	for _, g := range zc.NPCGroups {
		mark(g.Center[0], g.Center[1], g.Radius, color.RGBA{220, 50, 40, 255})
	}
	mark(0, 0, 0, color.RGBA{})
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
