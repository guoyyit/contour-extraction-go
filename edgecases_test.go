package contour

import (
	"math"
	"reflect"
	"testing"
)

func TestMinimalGrid(t *testing.T) {
	g, err := NewGrid(2, 2, []float64{0, 10, 10, 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, level := range []float64{-1.0, 5.0, 21.0} {
		ref := ExtractLevel(g, level, DefaultConfig())
		for _, tc := range []int{1, 2} {
			cfg := DefaultConfig()
			cfg.TileCells = tc
			got, stats := ExtractLevelTiled(g, level, cfg)
			if stats.Tiles != 1 {
				t.Fatalf("level %v: expected 1 tile, got %d", level, stats.Tiles)
			}
			if !reflect.DeepEqual(got, ref) {
				t.Fatalf("level %v tile=%d mismatch", level, tc)
			}
		}
	}
}

func TestStripGrids(t *testing.T) {
	// 1 行单元、多列单元，反之亦然。
	vals1 := []float64{0, 1, 2, 3, 4}
	g, _ := NewGrid(1, 5, vals1)
	lines := ExtractLevel(g, 2.0, DefaultConfig())
	for _, l := range lines {
		if l.Closed {
			t.Fatal("strip grid cannot contain a closed loop")
		}
	}
	vals2 := []float64{0, 1, 2, 3, 4}
	g2, _ := NewGrid(5, 1, vals2)
	lines2 := ExtractLevel(g2, 2.0, DefaultConfig())
	for _, l := range lines2 {
		if l.Closed {
			t.Fatal("strip grid cannot contain a closed loop")
		}
	}
}

func TestSingleRowAllUndefinedTiles(t *testing.T) {
	nan := math.NaN()
	vals := []float64{nan, nan, nan, nan}
	g, _ := NewGrid(2, 2, vals)
	got, stats := ExtractLevelTiled(g, 0, DefaultConfig())
	if stats.Segments != 0 || len(got) != 0 {
		t.Fatalf("all-undefined grid produced output: %+v", got)
	}
}

func TestFlatDifferentLevelAllPolicies(t *testing.T) {
	g, _ := NewGrid(4, 4, []float64{
		5, 5, 5, 5,
		5, 5, 5, 5,
		5, 5, 5, 5,
		5, 5, 5, 5,
	})
	for _, pol := range []FlatPolicy{FlatSuppress, FlatDraw} {
		cfg := DefaultConfig()
		cfg.Flat = pol
		if lines := ExtractLevel(g, 1, cfg); len(lines) != 0 {
			t.Fatalf("%s: lines at non-level constant", pol)
		}
	}
}

func TestStrictPolicyNoDanglingOnSmoothField(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 20, Cols: 24, Kind: "gaussian", Seed: 19, NoDataPct: 0.05})
	cfg := DefaultConfig()
	cfg.ExactLevel = ExactStrict
	// 选取不太可能恰好命中格点的级别。
	for _, level := range []float64{0.123, 1.234, 2.345} {
		lines := ExtractLevel(g, level, cfg)
		for _, l := range lines {
			if l.Closed {
				continue
			}
			for _, p := range []Point{l.Points[0], l.Points[len(l.Points)-1]} {
				if onOuterBoundary(p, g) || (l.barrier && isEdgeMidpointAdjacentToNaN(p, g)) {
					continue
				}
				t.Fatalf("strict level %v: dangling endpoint %v", level, p)
			}
		}
	}
}
