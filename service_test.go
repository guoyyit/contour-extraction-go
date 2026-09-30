package contour

import (
	"os"
	"path/filepath"
	"testing"
)

// Repeated runs with different concurrency and level extraction orders must be
// byte-identical.
func TestDeterminism(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "hills", NX: 15, NY: 13, UndefinedHole: true})
	levels := []float64{-0.8, -0.2, 0.3, 0.9}

	var ref []Contour
	for run := 0; run < 4; run++ {
		opts := Options{NaNPolicy: NaNMask, TileNX: 4, TileNY: 3, EmitPointContours: true}
		opts.Concurrency = run + 1
		svc, err := NewService(g, opts)
		if err != nil {
			t.Fatal(err)
		}
		var cs []Contour
		if run%2 == 0 {
			cs, err = svc.Extract(levels)
		} else {
			// reversed request order
			rev := make([]float64, len(levels))
			for i, l := range levels {
				rev[len(levels)-1-i] = l
			}
			cs, err = svc.Extract(rev)
		}
		if err != nil {
			t.Fatal(err)
		}
		if run == 0 {
			ref = cs
		} else if !contoursEqual(ref, cs) {
			t.Fatalf("run %d not deterministic", run)
		}
	}
}

// Incremental extraction: adding a level never recomputes stored levels, and
// the combined result equals a one-shot full extraction.
func TestIncrementalLevels(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "hills", NX: 12, NY: 12})
	opts := Options{NaNPolicy: NaNMask, TileNX: 5, TileNY: 5, Concurrency: 3}

	inc, err := NewService(g, opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Extract([]float64{0}); err != nil {
		t.Fatal(err)
	}
	if !inc.HasLevel(0) || inc.HasLevel(1) {
		t.Fatal("HasLevel state incorrect")
	}
	r0 := inc.Results(0)

	if _, err := inc.Extract([]float64{0, 0.5, -0.5}); err != nil {
		t.Fatal(err)
	}
	if !contoursEqual(r0, inc.Results(0)) {
		t.Fatal("existing level was recomputed/changed")
	}

	all, err := NewService(g, opts)
	if err != nil {
		t.Fatal(err)
	}
	want, err := all.Extract([]float64{-0.5, 0, 0.5})
	if err != nil {
		t.Fatal(err)
	}
	got, err := inc.Extract([]float64{-0.5, 0, 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if !contoursEqual(want, got) {
		t.Fatal("incremental result differs from one-shot extraction")
	}

	ls := inc.Levels()
	if len(ls) != 3 || ls[0] != -0.5 || ls[2] != 0.5 {
		t.Fatalf("Levels() = %v", ls)
	}
}

func TestResultsPersistAndReload(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "saddle", NX: 10, NY: 10})
	opts := Options{NaNPolicy: NaNMask, TileNX: 4, TileNY: 4, Concurrency: 2, SimplifyPrecision: 0.2}
	svc, err := NewService(g, opts)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := svc.Extract([]float64{-1, 0, 1})
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "results.json")
	if err := svc.SaveResults(path); err != nil {
		t.Fatal(err)
	}
	rec, err := LoadResults(path)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Algorithm != algorithmVersion {
		t.Fatalf("algorithm tag = %q want %q", rec.Algorithm, algorithmVersion)
	}
	if !contoursEqual(cs, rec.Contours) {
		t.Fatal("reloaded contours differ from produced contours")
	}

	// Saving twice to the same content must produce identical bytes (historical
	// reproducibility).
	path2 := filepath.Join(t.TempDir(), "results2.json")
	if err := svc.SaveResults(path2); err != nil {
		t.Fatal(err)
	}
	b1 := readFile(t, path)
	b2 := readFile(t, path2)
	if len(b1) != len(b2) {
		t.Fatal("serialized result length differs across runs")
	}
	for i := range b1 {
		if b1[i] != b2[i] {
			t.Fatalf("serialized bytes differ at %d", i)
		}
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNewServiceValidation(t *testing.T) {
	if _, err := NewService(nil, Options{}); err == nil {
		t.Fatal("nil grid must error")
	}
	g, _ := Generate(GenConfig{Kind: "flat", NX: 3, NY: 3})
	svc, err := NewService(g, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if svc.opts.NaNPolicy != NaNMask {
		t.Fatal("default NaN policy must be mask")
	}
	if svc.opts.Concurrency <= 0 {
		t.Fatal("default concurrency must be positive")
	}
}
