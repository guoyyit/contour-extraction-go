package contour

import (
	"math"
	"testing"
)

// A fully flat region never produces contours; level above and below behave
// identically (no contours).
func TestFlatRegion(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "flat", NX: 8, NY: 8, Level: 0})
	for _, lvl := range []float64{-1.0, 0.0, 1.0} {
		cs := mustExtract(t, g, []float64{lvl}, testOpts())
		if len(cs) != 0 {
			t.Fatalf("flat grid at level %v: want 0 contours, got %d", lvl, len(cs))
		}
	}
}

// Flat plateau above the level is surrounded by lower values; it yields a
// single closed contour and no dangling pieces.
func TestFlatPlateauClosed(t *testing.T) {
	n := 7
	v := make([]float64, n*n)
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			if i >= 2 && i <= 4 && j >= 2 && j <= 4 {
				v[j*n+i] = 2
			} else {
				v[j*n+i] = -1
			}
		}
	}
	g := NewGrid(n, n, v)
	cs := mustExtract(t, g, []float64{0}, testOpts())
	nClosed, nOpen := 0, 0
	for _, c := range cs {
		if c.Closed {
			nClosed++
		} else {
			nOpen++
		}
	}
	if nClosed != 1 || nOpen != 0 {
		t.Fatalf("plateau: want exactly 1 closed, got %d closed/%d open", nClosed, nOpen)
	}
	assertNoCrossings(t, cs)
	assertContoursOnLevel(t, g, 0, cs)
}

// A level equal to an integer row of a plane f=i places contour points exactly
// on grid vertices. Extraction must still produce connected, non-crossing
// lines with no dangling interior segments.
func TestLevelExactlyOnVertices(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "plane", NX: 6, NY: 6})
	cs := mustExtract(t, g, []float64{2}, testOpts())
	var open []Contour
	for _, c := range cs {
		if c.Closed {
			t.Fatal("integer-level plane contour must be open")
		}
		open = append(open, c)
	}
	// Exactly one connected line along x=2.
	if len(open) != 1 {
		t.Fatalf("want 1 open line, got %d", len(open))
	}
	for _, p := range open[0].Points {
		if p.X != 2 {
			t.Fatalf("contour leaves x=2: %v", p)
		}
	}
	assertNoCrossings(t, cs)
	assertEndpointsValid(t, g, cs)
}

// A single-point plateau (one vertex at/above the level, all neighbors below)
// sampled exactly at the level yields a tiny closed ring; with point emission
// enabled it is exposed as a degenerate point contour when the ring collapses to
// the single level-valued vertex.
func TestSinglePointExtremum(t *testing.T) {
	n := 5
	v := make([]float64, n*n)
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			v[j*n+i] = -1
		}
	}
	v[2*n+2] = 0 // exactly the level
	g := NewGrid(n, n, v)

	off := testOpts()
	off.EmitPointContours = false
	csOff := mustExtract(t, g, []float64{0}, off)
	for _, c := range csOff {
		if c.Point {
			t.Fatal("point contour must be suppressed when disabled")
		}
	}

	on := testOpts()
	on.EmitPointContours = true
	cs := mustExtract(t, g, []float64{0}, on)
	var points []Contour
	for _, c := range cs {
		if c.Point {
			points = append(points, c)
		}
	}
	if len(points) != 1 {
		t.Fatalf("want exactly 1 point contour, got %d (%+v)", len(points), cs)
	}
	if points[0].Points[0] != (Point{X: 2, Y: 2}) {
		t.Fatalf("point contour at %v, want (2,2)", points[0].Points[0])
	}
}

// A NaN hole under mask policy yields contours that terminate at the mask
// boundary, never pass through the hole, and never close spuriously.
func TestNaNHoleMask(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "hills", NX: 12, NY: 12, UndefinedHole: true})
	cs := mustExtract(t, g, []float64{0}, testOpts())
	assertNoCrossings(t, cs)
	assertEndpointsValid(t, g, cs)
	for _, c := range cs {
		for _, p := range c.Points {
			// No contour point should be placed inside the 2x2 NaN block.
			ic := g.NX / 2
			jc := g.NY / 2
			ix, iy := int(p.X+0.5), int(p.Y+0.5)
			if (ix == ic || ix == ic+1) && (iy == jc || iy == jc+1) &&
				p.X == float64(ix) && p.Y == float64(iy) {
				t.Fatalf("contour enters NaN block: %v", p)
			}
		}
	}
}

// Under fill policy, NaN samples are replaced and no grid cell is masked.
func TestNaNFillPolicy(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "hills", NX: 12, NY: 12, UndefinedHole: true})
	opts := testOpts()
	opts.NaNPolicy = NaNFill
	opts.FillValue = -10
	cs := mustExtract(t, g, []float64{0}, opts)
	assertNoCrossings(t, cs)
	// Every open endpoint must lie on the outer data boundary only.
	for _, c := range cs {
		if c.Closed {
			continue
		}
		for _, idx := range []int{0, len(c.Points) - 1} {
			p := c.Points[idx]
			outer := p.X == 0 || p.Y == 0 ||
				p.X == float64(g.NX-1) || p.Y == float64(g.NY-1)
			if !outer {
				t.Fatalf("fill policy: endpoint %v is not on outer boundary", p)
			}
		}
	}
}

func TestBadNaNPolicy(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "flat", NX: 4, NY: 4})
	if _, err := NewService(g, Options{NaNPolicy: "bogus"}); err == nil {
		t.Fatal("expected error for unknown NaN policy")
	}
}

func TestNaNLevelRejected(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "flat", NX: 4, NY: 4})
	svc, _ := NewService(g, testOpts())
	if _, err := svc.Extract([]float64{0, math.NaN()}); err == nil {
		t.Fatal("expected error for NaN level")
	}
}
