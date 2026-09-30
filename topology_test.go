package contour

import (
	"math"
	"testing"
)

// Every contour vertex sits on a grid edge whose endpoint values straddle the
// level (or equals a vertex exactly at the level), and every contour segment
// stays inside one grid cell. This directly verifies geometric consistency
// with the source data and rules out crossing cell boundaries mid-edge.
func TestContoursRespectSourceValues(t *testing.T) {
	grids := []*Grid{}
	for _, kind := range []string{"plane", "cone", "saddle", "hills"} {
		g, _ := Generate(GenConfig{Kind: kind, NX: 12, NY: 12})
		grids = append(grids, g)
	}
	for _, level := range []float64{-0.9, 0, 0.4, 1.7} {
		for _, g := range grids {
			opts := testOpts()
			cs := mustExtract(t, g, []float64{level}, opts)
			for _, c := range cs {
				for _, p := range c.Points {
					if !pointOnCrossingEdge(g, p, level) {
						t.Fatalf("kind level %v: contour point %v is not on any level-crossing edge", level, p)
					}
				}
				if c.Point || len(c.Points) < 2 {
					continue
				}
				n := len(c.Points)
				lim := n
				if c.Closed {
					lim = n
				}
				for k := 0; k < lim; k++ {
					a := c.Points[k%n]
					b := c.Points[(k+1)%n]
					if !c.Closed && k == n-1 {
						break
					}
					if !segmentWithinOneCell(a, b) {
						t.Fatalf("segment %v->%v spans more than one cell", a, b)
					}
				}
			}
		}
	}
}

func TestNoRepeatedContourVertices(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "hills", NX: 18, NY: 18, UndefinedHole: true})
	cs := mustExtract(t, g, []float64{-0.7, 0, 0.7}, testOpts())
	for _, c := range cs {
		if c.Point {
			continue
		}
		seen := map[Point]int{}
		for i, p := range c.Points {
			if prev, ok := seen[p]; ok {
				// A closed ring legitimately closes at its first vertex;
				// that was already deduped, so any repeat is an error.
				t.Fatalf("contour repeats vertex %v at positions %d and %d (closed=%v)",
					p, prev, i, c.Closed)
			}
			seen[p] = i
		}
	}
}

// pointOnCrossingEdge reports whether p coincides with the linear-interpolated
// crossing of level on some grid edge whose endpoints straddle the level, or
// equals a grid vertex exactly at the level.
func pointOnCrossingEdge(g *Grid, p Point, level float64) bool {
	onIntVertex := math.Mod(p.X, 1) == 0 && math.Mod(p.Y, 1) == 0 &&
		g.At(int(math.Round(p.X)), int(math.Round(p.Y))) == level
	if onIntVertex {
		return true
	}
	// Horizontal edges: integer y, x in [i, i+1].
	if math.Mod(p.Y, 1) == 0 {
		j := int(math.Round(p.Y))
		i := int(math.Floor(p.X))
		if j >= 0 && j < g.NY && i >= 0 && i < g.NX-1 {
			va, vb := g.At(i, j), g.At(i+1, j)
			if !math.IsNaN(va) && !math.IsNaN(vb) {
				if tt, ok := edgeInterp(va, vb, level); ok {
					q := Point{X: float64(i) + tt, Y: float64(j)}
					if q == p {
						return true
					}
				}
			}
		}
	}
	// Vertical edges: integer x, y in [j, j+1].
	if math.Mod(p.X, 1) == 0 {
		i := int(math.Round(p.X))
		j := int(math.Floor(p.Y))
		if i >= 0 && i < g.NX && j >= 0 && j < g.NY-1 {
			va, vb := g.At(i, j), g.At(i, j+1)
			if !math.IsNaN(va) && !math.IsNaN(vb) {
				if tt, ok := edgeInterp(va, vb, level); ok {
					q := Point{X: float64(i), Y: float64(j) + tt}
					if q == p {
						return true
					}
				}
			}
		}
	}
	return false
}

// segmentWithinOneCell reports whether both endpoints lie on edges of one
// common unit cell (i.e. coordinates floor into the same cell).
func segmentWithinOneCell(a, b Point) bool {
	if samePoint(a, b) {
		return true
	}
	d := math.Max(math.Abs(a.X-b.X), math.Abs(a.Y-b.Y))
	if d > 1.0000000001 {
		return false
	}
	ci := math.Min(math.Floor(a.X), math.Floor(b.X))
	cj := math.Min(math.Floor(a.Y), math.Floor(b.Y))
	ci2 := math.Max(math.Floor(a.X), math.Floor(b.X))
	cj2 := math.Max(math.Floor(a.Y), math.Floor(b.Y))
	// Endpoints on the right/top edges may floor into adjacent cell indices.
	return ci2-ci <= 1 && cj2-cj <= 1 && ci >= 0 && cj >= 0
}
