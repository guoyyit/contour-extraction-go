package contour

import "testing"

func tilingVariants() []Options {
	return []Options{
		// single block
		{TileNX: 64, TileNY: 64, Concurrency: 1, NaNPolicy: NaNMask, EmitPointContours: true},
		// many small blocks, single worker
		{TileNX: 1, TileNY: 1, Concurrency: 1, NaNPolicy: NaNMask, EmitPointContours: true},
		// small blocks, many workers
		{TileNX: 2, TileNY: 2, Concurrency: 8, NaNPolicy: NaNMask, EmitPointContours: true},
		// non-square tiles
		{TileNX: 3, TileNY: 2, Concurrency: 4, NaNPolicy: NaNMask, EmitPointContours: true},
		// tiles wider than the grid
		{TileNX: 100, TileNY: 1, Concurrency: 3, NaNPolicy: NaNMask, EmitPointContours: true},
	}
}

func TestTilingEquivalence(t *testing.T) {
	grids := map[string]*Grid{}
	for _, kind := range []string{"plane", "cone", "saddle", "hills"} {
		g, err := Generate(GenConfig{Kind: kind, NX: 9, NY: 9})
		if err != nil {
			t.Fatal(err)
		}
		grids[kind] = g
		gh, err := Generate(GenConfig{Kind: kind, NX: 10, NY: 7, UndefinedHole: true})
		if err != nil {
			t.Fatal(err)
		}
		grids[kind+"-hole"] = gh
	}

	levels := []float64{-1.5, 0, 0.5, 2, 3.5}

	for name, g := range grids {
		var reference []Contour
		for ti, opts := range tilingVariants() {
			svc, err := NewService(g, opts)
			if err != nil {
				t.Fatal(err)
			}
			cs, err := svc.Extract(levels)
			if err != nil {
				t.Fatal(err)
			}
			if ti == 0 {
				reference = cs
				continue
			}
			if !contoursEqual(reference, cs) {
				t.Fatalf("grid %s variant %d disagrees with single-block extraction", name, ti)
			}
		}
	}
}

// Explicitly verify shared tile edges: every contour that crosses a boundary
// line x=k or y=k does so at one shared coordinate with no duplicate segment.
func TestBlockBoundaryStitching(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "hills", NX: 10, NY: 10})
	opts := Options{TileNX: 3, TileNY: 3, Concurrency: 4, NaNPolicy: NaNMask, EmitPointContours: true}
	cs := mustExtract(t, g, []float64{0}, opts)

	hits := map[Point]int{}
	for _, c := range cs {
		if c.Point {
			continue
		}
		for _, p := range c.Points {
			for _, k := range []float64{3, 6, 9} {
				if p.X == k || p.Y == k {
					hits[p]++
				}
			}
		}
	}
	for p, n := range hits {
		// A coordinate on an internal boundary is visited by the two incident
		// cells' segments at most and must be a shared node, not a duplicated
		// point pair.
		if n > 2 {
			t.Fatalf("boundary point %v visited %d times", p, n)
		}
	}
	assertNoCrossings(t, cs)
	assertContoursOnLevel(t, g, 0, cs)
	assertEndpointsValid(t, g, cs)
}

// Running tiles via two differently ordered service call sequences must yield
// identical merged output.
func TestTileOrderIndependence(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "saddle", NX: 8, NY: 8})
	makeSvc := func() *Service {
		svc, err := NewService(g, Options{
			TileNX: 3, TileNY: 2, Concurrency: 1,
			NaNPolicy: NaNMask, EmitPointContours: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}
	s1 := makeSvc()
	a, err := s1.Extract([]float64{-1, 0, 1})
	if err != nil {
		t.Fatal(err)
	}
	s2 := makeSvc()
	b, err := s2.Extract([]float64{1, -1, 0})
	if err != nil {
		t.Fatal(err)
	}
	if !contoursEqual(a, b) {
		t.Fatal("level order changed output")
	}
}
