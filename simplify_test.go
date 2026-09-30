package contour

import "testing"

// Simplification never increases the point count, stays within precision, and
// preserves all topology invariants.
func TestSimplifyTopology(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "hills", NX: 20, NY: 20})
	baseOpts := Options{
		TileNX: 8, TileNY: 8, Concurrency: 3,
		NaNPolicy: NaNMask, EmitPointContours: true,
	}

	svc0, _ := NewService(g, baseOpts)
	full, err := svc0.Extract([]float64{-0.5, 0, 0.75})
	if err != nil {
		t.Fatal(err)
	}

	for _, prec := range []float64{0.05, 0.25, 0.6} {
		opts := baseOpts
		opts.SimplifyPrecision = prec
		svc, _ := NewService(g, opts)
		cs, err := svc.Extract([]float64{-0.5, 0, 0.75})
		if err != nil {
			t.Fatal(err)
		}

		nFull, nSimple := 0, 0
		for _, c := range full {
			nFull += len(c.Points)
		}
		for _, c := range cs {
			nSimple += len(c.Points)
			if len(c.Points) > 0 && c.Closed && len(c.Points) < 3 {
				t.Fatalf("closed contour collapsed below 3 points at precision %v", prec)
			}
		}
		if nSimple > nFull {
			t.Fatalf("precision %v increased points: %d > %d", prec, nSimple, nFull)
		}

		assertNoCrossings(t, cs)
		for _, lvl := range []float64{-0.5, 0, 0.75} {
			var atLevel []Contour
			for _, c := range cs {
				if c.Level == lvl {
					atLevel = append(atLevel, c)
				}
			}
			assertContoursOnLevel(t, g, lvl, atLevel)
			assertEndpointsValid(t, g, atLevel)
		}

		// Every simplified point is an original vertex (no new geometry).
		assertSubsetOfOriginal(t, full, cs)
	}
}

// Precision bound: simplified polylines must stay within precision of the
// original contour in Hausdorff-ish sense (all removed points close to the
// simplification, guaranteed by DP; we verify endpoints-preservation here).
func TestSimplifyPreservesEndpoints(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "plane", NX: 10, NY: 10})
	opts := Options{SimplifyPrecision: 0.5, TileNX: 3, TileNY: 3, Concurrency: 2, NaNPolicy: NaNMask}
	cs := mustExtract(t, g, []float64{3.5}, opts)
	for _, c := range cs {
		if c.Closed {
			continue
		}
		orig, _ := NewService(g, Options{NaNPolicy: NaNMask})
		o, _ := orig.Extract([]float64{3.5})
		if len(o) != 1 {
			t.Fatalf("want 1 plane contour, got %d", len(o))
		}
		if c.Points[0] != o[0].Points[0] ||
			c.Points[len(c.Points)-1] != o[0].Points[len(o[0].Points)-1] {
			t.Fatal("simplification moved an open contour endpoint")
		}
	}
}

// Zero precision means no simplification at all: byte-identical output.
func TestSimplifyDisabled(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "cone", NX: 12, NY: 12})
	a := mustExtract(t, g, []float64{1}, Options{NaNPolicy: NaNMask, TileNX: 2, TileNY: 2, Concurrency: 2})
	b := mustExtract(t, g, []float64{1}, Options{NaNPolicy: NaNMask, TileNX: 2, TileNY: 2, Concurrency: 2, SimplifyPrecision: 0})
	if !contoursEqual(a, b) {
		t.Fatal("precision 0 must not change output")
	}
}

// Simplification output must also be independent of the block decomposition.
func TestSimplifyTilingIndependent(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "hills", NX: 21, NY: 17, UndefinedHole: true})
	levels := []float64{-0.6, 0.1, 0.8}

	variants := []Options{
		{TileNX: 64, TileNY: 64, Concurrency: 1, SimplifyPrecision: 0.2, NaNPolicy: NaNMask, EmitPointContours: true},
		{TileNX: 3, TileNY: 3, Concurrency: 1, SimplifyPrecision: 0.2, NaNPolicy: NaNMask, EmitPointContours: true},
		{TileNX: 5, TileNY: 2, Concurrency: 6, SimplifyPrecision: 0.2, NaNPolicy: NaNMask, EmitPointContours: true},
		{TileNX: 2, TileNY: 5, Concurrency: 3, SimplifyPrecision: 0.2, NaNPolicy: NaNFill, FillValue: -5, EmitPointContours: true},
	}
	var ref []Contour
	for i, opts := range variants {
		if i == 3 {
			// Fill policy changes semantics; compare it only against an
			// equivalent single-block fill run.
			single := opts
			single.TileNX, single.TileNY, single.Concurrency = 64, 64, 1
			ref = mustExtract(t, g, levels, single)
		}
		cs := mustExtract(t, g, levels, opts)
		if i < 3 && i == 0 {
			ref = cs
		}
		if !contoursEqual(ref, cs) {
			t.Fatalf("simplified tiling variant %d differs", i)
		}
		assertNoCrossings(t, cs)
	}
}

func assertSubsetOfOriginal(t *testing.T, orig, simplified []Contour) {
	t.Helper()
	origPoints := map[Point]bool{}
	for _, c := range orig {
		for _, p := range c.Points {
			origPoints[p] = true
		}
	}
	for _, c := range simplified {
		for _, p := range c.Points {
			if !origPoints[p] {
				t.Fatalf("simplified contour introduced new point %v", p)
			}
		}
	}
}

func TestGeometryHelpers(t *testing.T) {
	if segmentsIntersect(Point{0, 0}, Point{2, 2}, Point{0, 2}, Point{2, 0}) != true {
		t.Fatal("crossing diagonals not detected")
	}
	if segmentsIntersect(Point{0, 0}, Point{1, 1}, Point{1, 1}, Point{2, 2}) {
		t.Fatal("shared-endpoint touch must not count as crossing")
	}
	if !segmentsIntersect(Point{0, 0}, Point{2, 0}, Point{1, 0}, Point{1, 2}) {
		t.Fatal("T-junction with endpoint on interior must count as crossing here")
	}
	if !pointInRing(Point{1, 1}, []Point{{0, 0}, {2, 0}, {2, 2}, {0, 2}}) {
		t.Fatal("point inside square not detected")
	}
	if pointInRing(Point{3, 1}, []Point{{0, 0}, {2, 0}, {2, 2}, {0, 2}}) {
		t.Fatal("point outside square reported inside")
	}
	if d := distPointSegment(Point{1, 1}, Point{0, 0}, Point{2, 0}); d != 1 {
		t.Fatalf("distance = %v want 1", d)
	}
}
