package contour

import (
	"reflect"
	"testing"
)

func TestTileEquivalence(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 40, Cols: 48, Kind: "gaussian", Seed: 13, NoDataPct: 0.04})
	for _, level := range []float64{0.0, 1.0, 2.5} {
		ref := ExtractLevel(g, level, DefaultConfig())
		for _, tc := range []int{1, 2, 3, 5, 7, 16, 64} {
			for _, w := range []int{1, 2, 4, 8} {
				cfg := DefaultConfig()
				cfg.TileCells = tc
				cfg.Concurrency = w
				got, stats := ExtractLevelTiled(g, level, cfg)
				if stats.Tiles == 0 {
					t.Fatalf("tile=%d workers=%d: zero tiles", tc, w)
				}
				if !reflect.DeepEqual(got, ref) {
					t.Fatalf("level %v tile=%d workers=%d differs from single-block", level, tc, w)
				}
			}
		}
	}
}

func TestTileEquivalenceDegenerate(t *testing.T) {
	// 近乎平坦但含个别高/低值点。
	vals := make([]float64, 25*25)
	for i := range vals {
		vals[i] = 1
	}
	vals[10*25+10] = 0
	vals[10*25+11] = 2
	g, _ := NewGrid(25, 25, vals)
	ref := ExtractLevel(g, 1, DefaultConfig())
	for _, tc := range []int{1, 4, 10} {
		cfg := DefaultConfig()
		cfg.TileCells = tc
		cfg.Concurrency = 1
		got, _ := ExtractLevelTiled(g, 1, cfg)
		if !reflect.DeepEqual(got, ref) {
			t.Fatalf("degenerate: tile=%d differs", tc)
		}
	}
}

// 块边界无缝：把块边界精确放在已知会产生穿越点的格边上，
// 拼接结果不得在边界处断开。
func TestTileSeamContinuous(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 30, Cols: 30, Kind: "gaussian", Seed: 2})
	ref := ExtractLevel(g, 1.0, DefaultConfig())
	// 强制分块边界落在每一行/列上：tile=1 时分块最多，最容易暴露缝隙。
	cfg := DefaultConfig()
	cfg.TileCells = 1
	cfg.Concurrency = 4
	got, _ := ExtractLevelTiled(g, 1.0, cfg)
	if !reflect.DeepEqual(got, ref) {
		t.Fatalf("tile=1 result differs: got %d lines vs ref %d", len(got), len(ref))
	}
	// 每一条开放线都必须真的连到边界，不允许在内部终止。
	for _, l := range got {
		if l.Closed {
			continue
		}
		for _, p := range []Point{l.Points[0], l.Points[len(l.Points)-1]} {
			if !onOuterBoundary(p, g) && !l.barrier {
				t.Fatalf("open line ends internally at %v", p)
			}
		}
	}
}

// 分块覆盖完备且互不重叠。
func TestPlanTilesPartition(t *testing.T) {
	for _, dims := range [][2]int{{1, 1}, {3, 5}, {10, 10}, {33, 17}} {
		tiles := planTiles(dims[0], dims[1], 3)
		cover := make([][]bool, dims[0])
		for r := range cover {
			cover[r] = make([]bool, dims[1])
		}
		for _, ti := range tiles {
			for r := ti.r0; r < ti.r1; r++ {
				for c := ti.c0; c < ti.c1; c++ {
					if cover[r][c] {
						t.Fatalf("cell (%d,%d) covered twice for dims %v", r, c, dims)
					}
					cover[r][c] = true
				}
			}
		}
		for r := 0; r < dims[0]; r++ {
			for c := 0; c < dims[1]; c++ {
				if !cover[r][c] {
					t.Fatalf("cell (%d,%d) not covered for dims %v", r, c, dims)
				}
			}
		}
	}
}

// 开启简化时，分块结果仍与单块逐点一致。
func TestTileEquivalenceWithSimplify(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 50, Cols: 46, Kind: "gaussian", Seed: 77, NoDataPct: 0.03})
	mk := func(tile, workers int) Config {
		c := DefaultConfig()
		c.TileCells = tile
		c.Concurrency = workers
		c.Simplify = SimplifyConfig{Enabled: true, Tolerance: 0.45}
		return c
	}
	ref := ExtractLevel(g, 1.0, mk(0, 1))
	for _, tc := range []int{1, 4, 17, 128} {
		for _, w := range []int{1, 4} {
			got, _ := ExtractLevelTiled(g, 1.0, mk(tc, w))
			if !reflect.DeepEqual(got, ref) {
				t.Fatalf("simplified: tile=%d w=%d differs from single block", tc, w)
			}
		}
	}
}
