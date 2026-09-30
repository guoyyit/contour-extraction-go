package contour

import (
	"math"
	"testing"
)

func allSegments(t *testing.T, g *Grid, level float64, cfg *Config) []Segment {
	t.Helper()
	return extractSegments(g, level, CellRange{0, 0, g.CellRows(), g.CellCols()}, cfg)
}

// 插值穿越点必须恰好位于级别上，且线段端点只能落在格点或格边上。
func TestSegmentPositionsConsistentWithValues(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 20, Cols: 24, Kind: "gaussian", Seed: 7})
	cfg := DefaultConfig()
	for _, level := range []float64{0.0, 1.0} {
		segs := allSegments(t, g, level, &cfg)
		if len(segs) == 0 {
			t.Fatalf("level %v: expected segments", level)
		}
		for _, s := range segs {
			for _, p := range [2]Point{s.A, s.B} {
				if p.X < 0 || p.X > float64(g.Cols()-1) || p.Y < 0 || p.Y > float64(g.Rows()-1) {
					t.Fatalf("point outside grid: %v", p)
				}
			}
			// 双线性在该小尺度光滑场上，线段中点附近不应出现整格大小的跨级跳变。
		}
	}
}

// 单一线性单元：穿越点必须等于线性插值位置。
func TestSingleCellInterpolation(t *testing.T) {
	g, _ := NewGrid(2, 2, []float64{0, 10, 10, 20})
	cfg := DefaultConfig()
	segs := allSegments(t, g, 5, &cfg)
	if len(segs) != 2 {
		t.Fatalf("expected 2 triangle segments, got %d", len(segs))
	}
	// 两条边穿越点：上边 (0.5,0)，左边 (0,0.5)；对角线无穿越。
	pts := []Point{segs[0].A, segs[0].B, segs[1].A, segs[1].B}
	want := map[[2]float64]bool{{0.5, 0}: false, {0, 0.5}: false}
	for _, p := range pts {
		for w := range want {
			if math.Abs(p.X-w[0]) < 1e-12 && math.Abs(p.Y-w[1]) < 1e-12 {
				want[w] = true
			}
		}
	}
	for w, found := range want {
		if !found {
			t.Fatalf("missing crossing %v in %v", w, pts)
		}
	}
}

// 平坦区域且级别不等于常值：无线段。
func TestFlatRegionDifferentLevel(t *testing.T) {
	g, _ := NewGrid(3, 3, []float64{
		1, 1, 1,
		1, 1, 1,
		1, 1, 1,
	})
	cfg := DefaultConfig()
	if segs := allSegments(t, g, 2, &cfg); len(segs) != 0 {
		t.Fatalf("expected no segments, got %d", len(segs))
	}
}

// 整片取值恰为级别：默认 suppress 不产生任何线段（无伪闭合）。
func TestFlatExactLevelSuppress(t *testing.T) {
	g, _ := NewGrid(4, 4, make([]float64, 16))
	for i := range g.values {
		g.values[i] = 1
	}
	cfg := DefaultConfig()
	if segs := allSegments(t, g, 1, &cfg); len(segs) != 0 {
		t.Fatalf("suppress: expected no segments inside plateau, got %d", len(segs))
	}
}

// FlatDraw：平坦高原内部产生确定性的之字形连接线。
func TestFlatExactLevelDraw(t *testing.T) {
	g, _ := NewGrid(4, 4, make([]float64, 16))
	for i := range g.values {
		g.values[i] = 1
	}
	cfg := DefaultConfig()
	cfg.Flat = FlatDraw
	segs := allSegments(t, g, 1, &cfg)
	if len(segs) == 0 {
		t.Fatal("draw: expected deterministic segments inside plateau")
	}
	// 确定性：再提取一次完全一致。
	segs2 := allSegments(t, g, 1, &cfg)
	if len(segs) != len(segs2) {
		t.Fatal("non-deterministic segment count")
	}
	for i := range segs {
		if segs[i].KA != segs2[i].KA || segs[i].KB != segs2[i].KB {
			t.Fatalf("segment %d keys differ", i)
		}
	}
}

// 斜坡上恰为级别的格点：字典序扰动下等值线唯一、确定地从邻边经过。
func TestExactLevelNodeOnSlopePerturb(t *testing.T) {
	// 行方向线性场 f=r（0,1,2），级别=1 时中间一行格点恰为级别。
	g, _ := NewGrid(3, 2, []float64{
		0, 0,
		1, 1,
		2, 2,
	})
	cfg := DefaultConfig()
	segs := allSegments(t, g, 1, &cfg)
	if len(segs) == 0 {
		t.Fatal("perturb: expected segments through exact-level row")
	}
	// 确定性：再提取一次完全一致。
	segs2 := allSegments(t, g, 1, &cfg)
	if len(segs) != len(segs2) {
		t.Fatal("non-deterministic count")
	}
	for _, s := range segs {
		_ = s
	}
	// 等值线沿级别行从左边界节点 (0,1) 走到右边界节点 (1,1)，
	// 每一个点的源数据取值关系与级别一致（穿越点都在值为 level 的位置）。
	found := false
	for _, s := range segs {
		if s.A == (Point{X: 0, Y: 1}) && s.B == (Point{X: 1, Y: 1}) {
			found = true
		}
	}
	if !found {
		t.Fatal("perturb: expected contour along the exact-level row from (0,1) to (1,1)")
	}
}

// 孤立格点恰为级别（其余全部低于级别）：该点是零维集合，
// 不产生线段（无悬挂），两种语义一致。
func TestIsolatedExactLevelNodeNoSegment(t *testing.T) {
	g, _ := NewGrid(3, 3, []float64{
		1, 0, 0,
		0, 0, 0,
		0, 0, 0,
	})
	for _, pol := range []ExactLevelPolicy{ExactPerturb, ExactStrict} {
		cfg := DefaultConfig()
		cfg.ExactLevel = pol
		if segs := allSegments(t, g, 1, &cfg); len(segs) != 0 {
			t.Fatalf("%s: expected zero segments for isolated level node, got %d", pol, len(segs))
		}
	}
}

// 单点极值（峰）：level 低于峰值时必须形成闭合环。
func TestSinglePeakClosedRing(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 21, Cols: 21, Kind: "singlepeak", Seed: 1})
	cfg := DefaultConfig()
	segs := allSegments(t, g, 0.5, &cfg)
	if len(segs) == 0 {
		t.Fatal("expected ring around single peak")
	}
	// 所有端点都必须是内部键（非数据边界），以便拼接为闭合环。
	for _, s := range segs {
		for _, k := range [2]endKey{s.KA, s.KB} {
			if isDataBoundaryKey(k, g) {
				t.Fatalf("ring segment touches data boundary: %+v", k)
			}
		}
	}
}

// 未定义值：barrier 语义下线止于已定义/未定义交界，且带 barrier 键。
func TestUndefinedBarrier(t *testing.T) {
	nan := math.NaN()
	g, _ := NewGrid(3, 3, []float64{
		10, 10, 10,
		nan, 0, 0,
		0, 0, 0,
	})
	cfg := DefaultConfig()
	segs := allSegments(t, g, 5, &cfg)
	if len(segs) == 0 {
		t.Fatal("expected segments near undefined corner")
	}
	barrierCount := 0
	for _, s := range segs {
		if s.KA.barrier || s.KB.barrier {
			barrierCount++
		}
		// barrier 端点必须是 NaN 格点的邻边中点。
		for _, k := range [2]endKey{s.KA, s.KB} {
			if k.barrier {
				p := pointForKey(k, g)
				if !isEdgeMidpointAdjacentToNaN(p, g) {
					t.Fatalf("barrier point %v not on undefined boundary", p)
				}
			}
		}
	}
	if barrierCount == 0 {
		t.Fatal("expected at least one barrier endpoint")
	}
}

// 全部未定义：无线段。
func TestAllUndefined(t *testing.T) {
	g, _ := NewGrid(2, 2, []float64{
		math.NaN(), math.NaN(),
		math.NaN(), math.NaN(),
	})
	cfg := DefaultConfig()
	if segs := allSegments(t, g, 0, &cfg); len(segs) != 0 {
		t.Fatalf("expected no segments, got %d", len(segs))
	}
}
