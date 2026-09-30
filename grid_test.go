package contour

import (
	"math"
	"testing"
)

func TestNewGridValidation(t *testing.T) {
	if _, err := NewGrid(0, 2, nil); err == nil {
		t.Fatal("expected error for zero rows")
	}
	if _, err := NewGrid(2, 2, []float64{1, 2, 3}); err == nil {
		t.Fatal("expected error for mismatched values length")
	}
	g, err := NewGrid(2, 3, []float64{1, 2, 3, 4, 5, math.NaN()})
	if err != nil {
		t.Fatal(err)
	}
	if g.Value(1, 2) == g.Value(1, 2) {
		t.Fatal("expected NaN")
	}
	if !g.IsUndefined(1, 2) || g.IsUndefined(0, 0) {
		t.Fatal("IsUndefined incorrect")
	}
	if g.CellRows() != 1 || g.CellCols() != 2 {
		t.Fatalf("cell dims wrong: %d %d", g.CellRows(), g.CellCols())
	}
}

func TestGenerateDeterministic(t *testing.T) {
	for _, kind := range []string{"gaussian", "plane", "flat", "singlepeak", "random"} {
		a, err := GenerateGrid(GenerateSpec{Rows: 12, Cols: 10, Kind: kind, Seed: 42, NoDataPct: 0.1})
		if err != nil {
			t.Fatal(kind, err)
		}
		b, _ := GenerateGrid(GenerateSpec{Rows: 12, Cols: 10, Kind: kind, Seed: 42, NoDataPct: 0.1})
		for i := range a.values {
			va, vb := a.values[i], b.values[i]
			if math.IsNaN(va) != math.IsNaN(vb) || (!math.IsNaN(va) && va != vb) {
				t.Fatalf("%s: nondeterministic at %d: %v vs %v", kind, i, va, vb)
			}
		}
	}
}

func TestGenerateUnknownKind(t *testing.T) {
	if _, err := GenerateGrid(GenerateSpec{Rows: 4, Cols: 4, Kind: "nope"}); err == nil {
		t.Fatal("expected unknown kind error")
	}
	if _, err := GenerateGrid(GenerateSpec{Rows: 1, Cols: 4}); err == nil {
		t.Fatal("expected small-grid error")
	}
}

func TestGridCloneIsolation(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 5, Cols: 5, Kind: "plane", Seed: 1})
	h := g.Clone()
	h.values[0] = 999
	if g.values[0] == 999 {
		t.Fatal("clone shares backing array")
	}
}
