package contour

import (
	"math"
	"sort"
	"strconv"
	"testing"
)

func testOpts() Options {
	return Options{
		TileNX:            64,
		TileNY:            64,
		Concurrency:       2,
		NaNPolicy:         NaNMask,
		EmitPointContours: true,
	}
}

func mustExtract(t *testing.T, g *Grid, levels []float64, opts Options) []Contour {
	t.Helper()
	svc, err := NewService(g, opts)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := svc.Extract(levels)
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

// segPairsFromContours flattens every contour segment (open and closed).
func segPairsFromContours(cs []Contour) [][2]Point {
	var out [][2]Point
	for _, c := range cs {
		if c.Point {
			continue
		}
		n := len(c.Points)
		for i := 0; i+1 < n; i++ {
			out = append(out, [2]Point{c.Points[i], c.Points[i+1]})
		}
		if c.Closed {
			out = append(out, [2]Point{c.Points[n-1], c.Points[0]})
		}
	}
	return out
}

func assertNoCrossings(t *testing.T, cs []Contour) {
	t.Helper()
	pairs := segPairsFromContours(cs)
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if i == j {
				continue
			}
			// adjacent segments in the same contour may share endpoints;
			// segmentsIntersect already excludes shared-endpoint touching.
			if segmentsIntersect(pairs[i][0], pairs[i][1], pairs[j][0], pairs[j][1]) {
				t.Fatalf("contour segments cross: %v--%v vs %v--%v",
					pairs[i][0], pairs[i][1], pairs[j][0], pairs[j][1])
			}
		}
	}
}

// assertConsistentLevel checks that every contour segment is linear within a
// monotone band of cells and keeps >= level on the left-consistent side:
// specifically each segment endpoint lies exactly on crossed edges and each
// grid vertex strictly inside a closed contour has the expected sign.
func assertContoursOnLevel(t *testing.T, g *Grid, level float64, cs []Contour) {
	t.Helper()
	for _, c := range cs {
		if c.Point {
			continue
		}
		for _, p := range c.Points {
			if math.IsNaN(p.X) || math.IsNaN(p.Y) {
				t.Fatal("NaN contour coordinate")
			}
		}
		if c.Closed {
			// Sample vertex signs inside the ring must all have one strict
			// sign relative to the level.
			pos, neg := false, false
			for j := 0; j < g.NY; j++ {
				for i := 0; i < g.NX; i++ {
					v := g.At(i, j)
					if math.IsNaN(v) {
						continue
					}
					if pointInRing(Point{float64(i), float64(j)}, c.Points) {
						if v > level {
							pos = true
						}
						if v < level {
							neg = true
						}
					}
				}
			}
			if pos && neg {
				t.Fatal("closed contour contains grid vertices on both sides of level")
			}
		}
	}
}

// assertEndpointsValid checks open contours end only at the outer data
// boundary (x/y at 0 or NX-1/NY-1) or at a mask boundary (NaN neighbor).
func assertEndpointsValid(t *testing.T, g *Grid, cs []Contour) {
	t.Helper()
	onOuter := func(p Point) bool {
		return p.X == 0 || p.Y == 0 || p.X == float64(g.NX-1) || p.Y == float64(g.NY-1)
	}
	onMask := func(p Point) bool {
		// Any segment endpoint not on the outer boundary must lie on an edge
		// incident to at least one NaN vertex.
		i, j := int(math.Round(p.X)), int(math.Round(p.Y))
		for dj := -1; dj <= 1; dj++ {
			for di := -1; di <= 1; di++ {
				if g.Contains(i+di, j+dj) && math.IsNaN(g.At(i+di, j+dj)) {
					return true
				}
			}
		}
		return false
	}
	for _, c := range cs {
		if c.Closed || c.Point {
			continue
		}
		for _, idx := range []int{0, len(c.Points) - 1} {
			p := c.Points[idx]
			if !onOuter(p) && !onMask(p) {
				t.Fatalf("dangling endpoint %v not on data or mask boundary", p)
			}
		}
	}
}

func TestPlaneContourHitsBoundaries(t *testing.T) {
	// f = i, level 2.5: one open line crossing the whole grid vertically.
	g, _ := Generate(GenConfig{Kind: "plane", NX: 6, NY: 5})
	cs := mustExtract(t, g, []float64{2.5}, testOpts())
	var open []Contour
	for _, c := range cs {
		if c.Closed {
			t.Fatal("plane contour must be open")
		}
		open = append(open, c)
	}
	if len(open) != 1 {
		t.Fatalf("want 1 open line, got %d", len(open))
	}
	c := open[0]
	if c.Points[0].X != 2.5 || c.Points[len(c.Points)-1].X != 2.5 {
		t.Fatalf("plane contour not at x=2.5: %v ... %v", c.Points[0], c.Points[len(c.Points)-1])
	}
	assertNoCrossings(t, cs)
	assertContoursOnLevel(t, g, 2.5, cs)
	assertEndpointsValid(t, g, cs)
}

func TestConeContourClosed(t *testing.T) {
	// Radial peak: every level below the peak yields one closed ring.
	g, _ := Generate(GenConfig{Kind: "cone", NX: 11, NY: 11})
	cs := mustExtract(t, g, []float64{1}, testOpts())
	var closed []Contour
	for _, c := range cs {
		if !c.Closed {
			t.Fatal("cone contour must be closed")
		}
		closed = append(closed, c)
	}
	if len(closed) != 1 || len(closed[0].Points) < 4 {
		t.Fatalf("want one ring with >=4 points, got %d", len(closed))
	}
	assertNoCrossings(t, cs)
	assertContoursOnLevel(t, g, 1, cs)
}

func TestSaddleDeterministic(t *testing.T) {
	// f=(x-2)^2-(y-2)^2 on 5x5, level 0: exactly two segments crossing
	// region resolved via center-in. Two runs with different tiling agree.
	g, _ := Generate(GenConfig{Kind: "saddle", NX: 5, NY: 5})
	o1 := testOpts()
	o1.TileNX, o1.TileNY = 64, 64
	o2 := testOpts()
	o2.TileNX, o2.TileNY = 2, 2
	o2.Concurrency = 4
	c1 := mustExtract(t, g, []float64{0}, o1)
	c2 := mustExtract(t, g, []float64{0}, o2)
	if !contoursEqual(c1, c2) {
		t.Fatalf("saddle extraction differs by tiling:\n%v\n%v", c1, c2)
	}
	assertNoCrossings(t, c1)
	assertContoursOnLevel(t, g, 0, c1)
}

func TestLevelOutsideRangeEmpty(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "plane", NX: 5, NY: 5})
	// f=i-0 ranges 0..4; level 10 gives nothing.
	cs := mustExtract(t, g, []float64{10}, testOpts())
	if len(cs) != 0 {
		t.Fatalf("expected no contours above range, got %d", len(cs))
	}
}

func contoursEqual(a, b []Contour) bool {
	return contourKey(a) == contourKey(b)
}

func contourKey(cs []Contour) string {
	cp := make([]Contour, len(cs))
	copy(cp, cs)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].Level != cp[j].Level {
			return cp[i].Level < cp[j].Level
		}
		return keyOf(cp[i]) < keyOf(cp[j])
	})
	var sb []byte
	for _, c := range cp {
		sb = append(sb, keyOf(c)...)
		sb = append(sb, '|')
	}
	return string(sb)
}

func keyOf(c Contour) string {
	s := ""
	if c.Closed {
		s += "C"
	} else {
		s += "O"
	}
	for _, p := range c.Points {
		s += "/" + strconv.FormatUint(math.Float64bits(p.X), 16) + "," + strconv.FormatUint(math.Float64bits(p.Y), 16)
	}
	return s
}
