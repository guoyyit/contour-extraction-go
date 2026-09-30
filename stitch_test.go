package contour

import (
	"math"
	"testing"
)

func extractLines(t *testing.T, g *Grid, level float64, cfg Config) []Polyline {
	t.Helper()
	segs := extractSegments(g, level, CellRange{0, 0, g.CellRows(), g.CellCols()}, &cfg)
	return stitch(level, segs, g, &cfg)
}

// 单峰必须得到一条闭合环，且不接触数据边界。
func TestStitchSinglePeak(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 25, Cols: 25, Kind: "singlepeak", Seed: 1})
	cfg := DefaultConfig()
	lines := extractLines(t, g, 0.5, cfg)
	if len(lines) != 1 || !lines[0].Closed {
		t.Fatalf("expected exactly one closed ring, got %+v", summarize(lines))
	}
	l := lines[0]
	for _, p := range l.Points {
		if p.X <= 0 || p.Y <= 0 || p.X >= float64(g.Cols()-1) || p.Y >= float64(g.Rows()-1) {
			t.Fatalf("closed ring touches boundary: %v", p)
		}
	}
	if len(l.Points) < 4 {
		t.Fatalf("ring too short: %d", len(l.Points))
	}
}

// 单调斜坡：每条等值线必须开放，且两端点都在数据外边界上。
func TestStitchPlaneOpen(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 10, Cols: 12, Kind: "plane", Seed: 1})
	cfg := DefaultConfig()
	for _, level := range []float64{1.0, 3.5, 6.0} {
		lines := extractLines(t, g, level, cfg)
		if len(lines) == 0 {
			t.Fatalf("level %v: expected lines", level)
		}
		for _, l := range lines {
			if l.Closed {
				t.Fatalf("level %v: unexpected closed line", level)
			}
			if len(l.Points) < 2 {
				t.Fatal("degenerate polyline")
			}
			for i, p := range []Point{l.Points[0], l.Points[len(l.Points)-1]} {
				if !onOuterBoundary(p, g) {
					t.Fatalf("level %v endpoint %d %v not on outer boundary", level, i, p)
				}
			}
		}
	}
}

// 拼接后不允许任何线段丢失或重复：折线覆盖的段数 == 提取段数。
func TestStitchNoDanglingNoDropped(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 18, Cols: 21, Kind: "gaussian", Seed: 99, NoDataPct: 0.05})
	cfg := DefaultConfig()
	level := 1.0
	segs := extractSegments(g, level, CellRange{0, 0, g.CellRows(), g.CellCols()}, &cfg)
	lines := stitch(level, segs, g, &cfg)
	covered := 0
	for _, l := range lines {
		covered += len(l.Points) - 1
		if l.Closed {
			covered++ // 闭合环比点段数多一条闭合边
		}
	}
	if covered != len(segs) {
		t.Fatalf("covered %d segments but extracted %d (dangling or dropped)", covered, len(segs))
	}
}

// 未定义孔洞：开放线端点要么在数据外边界，要么在已定义/未定义交界处。
func TestStitchUndefinedEndpoints(t *testing.T) {
	nan := math.NaN()
	vals := make([]float64, 15*15)
	for r := 0; r < 15; r++ {
		for c := 0; c < 15; c++ {
			vals[r*15+c] = float64(r + c)
		}
	}
	vals[7*15+7] = nan
	g, _ := NewGrid(15, 15, vals)
	cfg := DefaultConfig()
	lines := extractLines(t, g, 13, cfg)
	if len(lines) == 0 {
		t.Fatal("expected lines near undefined hole")
	}
	foundBarrier := false
	for _, l := range lines {
		if l.Closed {
			t.Fatal("line around hole should be open under barrier semantics")
		}
		for _, p := range []Point{l.Points[0], l.Points[len(l.Points)-1]} {
			if !onOuterBoundary(p, g) && !isEdgeMidpointAdjacentToNaN(p, g) {
				t.Fatalf("endpoint %v neither outer boundary nor undefined boundary", p)
			}
			if isEdgeMidpointAdjacentToNaN(p, g) {
				foundBarrier = true
			}
		}
	}
	if !foundBarrier {
		t.Fatal("expected at least one contour ending on the undefined hole boundary")
	}
}

// 确定性：两次拼接结果必须逐点一致。
func TestStitchDeterministic(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 16, Cols: 16, Kind: "gaussian", Seed: 5, NoDataPct: 0.08})
	cfg := DefaultConfig()
	a := extractLines(t, g, 1.2, cfg)
	b := extractLines(t, g, 1.2, cfg)
	if len(a) != len(b) {
		t.Fatalf("line count differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Closed != b[i].Closed || len(a[i].Points) != len(b[i].Points) {
			t.Fatalf("line %d shape differs", i)
		}
		for j := range a[i].Points {
			if a[i].Points[j] != b[i].Points[j] {
				t.Fatalf("line %d point %d differs: %v vs %v", i, j, a[i].Points[j], b[i].Points[j])
			}
		}
	}
}

func onOuterBoundary(p Point, g *Grid) bool {
	const eps = 1e-9
	return p.X <= eps || p.Y <= eps ||
		p.X >= float64(g.Cols()-1)-eps || p.Y >= float64(g.Rows()-1)-eps
}

func summarize(lines []Polyline) string {
	out := ""
	for _, l := range lines {
		out += "("
		if l.Closed {
			out += "closed "
		}
		out += string(rune('0'+len(l.Points)%10)) + ") "
	}
	return out
}
