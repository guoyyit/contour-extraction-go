package contour

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestNewGridAndAt(t *testing.T) {
	g := NewGrid(3, 2, []float64{
		1, 2, 3,
		4, 5, 6,
	})
	if got := g.At(0, 0); got != 1 {
		t.Fatalf("At(0,0)=%v want 1", got)
	}
	if got := g.At(2, 1); got != 6 {
		t.Fatalf("At(2,1)=%v want 6", got)
	}
	if !g.Contains(2, 1) || g.Contains(3, 1) || g.Contains(0, 2) {
		t.Fatal("Contains mismatch")
	}
	if !math.IsNaN(g.At(-1, 0)) {
		t.Fatal("out-of-range At must return NaN")
	}
}

func TestNewGridPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on bad dimensions")
		}
	}()
	NewGrid(0, 2, nil)
}

func TestGenerateKinds(t *testing.T) {
	for _, kind := range []string{"plane", "cone", "saddle", "hills", "flat"} {
		g, err := Generate(GenConfig{Kind: kind, NX: 8, NY: 6, Level: 0})
		if err != nil {
			t.Fatalf("kind %s: %v", kind, err)
		}
		if g.NX != 8 || g.NY != 6 || len(g.V) != 48 {
			t.Fatalf("kind %s: bad shape", kind)
		}
	}
	if _, err := Generate(GenConfig{Kind: "nope", NX: 4, NY: 4}); err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if _, err := Generate(GenConfig{Kind: "flat", NX: 1, NY: 4}); err == nil {
		t.Fatal("expected error for too-small grid")
	}
}

func TestGenerateUndefinedHole(t *testing.T) {
	g, err := Generate(GenConfig{Kind: "saddle", NX: 8, NY: 8, UndefinedHole: true})
	if err != nil {
		t.Fatal(err)
	}
	nNaN := 0
	for _, v := range g.V {
		if math.IsNaN(v) {
			nNaN++
		}
	}
	if nNaN != 4 {
		t.Fatalf("want 4 NaN samples, got %d", nNaN)
	}
}

func TestGridRoundTrip(t *testing.T) {
	g, _ := Generate(GenConfig{Kind: "saddle", NX: 5, NY: 4, UndefinedHole: true})
	dir := t.TempDir()
	path := filepath.Join(dir, "g.txt")
	if err := g.Save(path); err != nil {
		t.Fatal(err)
	}
	g2, err := LoadGrid(path)
	if err != nil {
		t.Fatal(err)
	}
	if g2.NX != g.NX || g2.NY != g.NY {
		t.Fatal("shape mismatch after round trip")
	}
	for k := range g.V {
		a, b := g.V[k], g2.V[k]
		if math.IsNaN(a) != math.IsNaN(b) || (!math.IsNaN(a) && a != b) {
			t.Fatalf("value %d mismatch: %v vs %v", k, a, b)
		}
	}
}

func TestLoadGridErrors(t *testing.T) {
	dir := t.TempDir()

	bad1 := filepath.Join(dir, "bad1.txt")
	if err := os.WriteFile(bad1, []byte("2 2\n1 x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadGrid(bad1); err == nil {
		t.Fatal("expected parse error")
	}

	bad2 := filepath.Join(dir, "bad2.txt")
	if err := os.WriteFile(bad2, []byte("3 2\n1 2 3\n1 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadGrid(bad2); err == nil {
		t.Fatal("expected row-width error")
	}

	if _, err := LoadGrid(filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatal("expected missing-file error")
	}
}
