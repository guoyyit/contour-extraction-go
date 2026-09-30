package contour

import (
	"math"
	"testing"
)

func countPoints(lines []Polyline) int {
	n := 0
	for _, l := range lines {
		n += len(l.Points)
	}
	return n
}

func allEdges(lines []Polyline) [][2]Point {
	var es [][2]Point
	for _, l := range lines {
		n := len(l.Points)
		for i := 0; i+1 < n; i++ {
			es = append(es, [2]Point{l.Points[i], l.Points[i+1]})
		}
		if l.Closed && n > 2 {
			es = append(es, [2]Point{l.Points[n-1], l.Points[0]})
		}
	}
	return es
}

// 任意两条非相邻边不得横交。
func assertNoCrossing(t *testing.T, lines []Polyline) {
	t.Helper()
	es := allEdges(lines)
	for i := 0; i < len(es); i++ {
		for j := i + 1; j < len(es); j++ {
			if segmentsCross(es[i][0], es[i][1], es[j][0], es[j][1]) {
				t.Fatalf("crossing between %v-%v and %v-%v", es[i][0], es[i][1], es[j][0], es[j][1])
			}
		}
	}
}

// 简化后每条边都必须通过与生产代码相同的区域侧别检查。
func assertNoRegionCrossing(t *testing.T, g *Grid, level float64, lines []Polyline, tol float64) {
	t.Helper()
	// 独立校验：沿边密集采样，使用与提取一致的固定对角三角定义域。
	// 合法的简化弦只能贴着等值线行走：所有样本必须落在级别附近
	// （|v-level| <= 1.5*tol 的数值裕量），且不得进入未定义三角片。
	// 采样循环与判据在本测试中独立书写，仅复用双线性求值。
	const eps = 1e-9
	margin := 1.5*tol + eps
	for _, l := range lines {
		if l.barrier {
			continue
		}
		n := len(l.Points)
		m := n
		if l.Closed {
			m++
		}
		for i := 0; i+1 < m; i++ {
			p := l.Points[i%n]
			q := l.Points[(i+1)%n]
			dx, dy := q.X-p.X, q.Y-p.Y
			steps := int(math.Hypot(dx, dy)/0.05) + 2
			for s := 1; s < steps; s++ {
				tt := float64(s) / float64(steps)
				x := p.X + dx*tt
				y := p.Y + dy*tt
				v, ok := bilinearAt(g, x, y)
				if !ok {
					t.Fatalf("edge %v->%v enters undefined triangle near %v", p, q, Point{x, y})
				}
				d := v - level
				if d > margin || d < -margin {
					t.Fatalf("edge %v->%v cuts through region with value %.3f (level=%v, tol=%v) near %v",
						p, q, v, level, tol, Point{x, y})
				}
			}
		}
	}
}

func TestSimplifyReducesPoints(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 40, Cols: 44, Kind: "gaussian", Seed: 3})
	full := ExtractLevel(g, 1.5, DefaultConfig())
	if countPoints(full) == 0 {
		t.Fatal("expected contours")
	}

	cfg := DefaultConfig()
	cfg.Simplify = SimplifyConfig{Enabled: true, Tolerance: 0.6}
	simp := ExtractLevel(g, 1.5, cfg)
	if countPoints(simp) >= countPoints(full) {
		t.Fatalf("simplify did not reduce points: %d vs %d", countPoints(simp), countPoints(full))
	}
	assertNoCrossing(t, simp)
	assertNoRegionCrossing(t, g, 1.5, simp, 0.6)

	// 闭合环简化后仍闭合。
	fClosed, sClosed := 0, 0
	for _, l := range full {
		if l.Closed {
			fClosed++
		}
	}
	for _, l := range simp {
		if l.Closed {
			sClosed++
		}
	}
	if sClosed != fClosed {
		t.Fatalf("closed ring count changed: %d -> %d", fClosed, sClosed)
	}
}

func TestSimplifyZeroToleranceIsIdentity(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 20, Cols: 24, Kind: "gaussian", Seed: 4})
	full := ExtractLevel(g, 1.0, DefaultConfig())
	cfg := DefaultConfig()
	cfg.Simplify = SimplifyConfig{Enabled: true, Tolerance: 0}
	got := ExtractLevel(g, 1.0, cfg)
	if countPoints(got) != countPoints(full) {
		t.Fatalf("zero tolerance changed geometry: %d vs %d", countPoints(got), countPoints(full))
	}
}

func TestSimplifyDeterministic(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 30, Cols: 30, Kind: "gaussian", Seed: 11})
	cfg := DefaultConfig()
	cfg.Simplify = SimplifyConfig{Enabled: true, Tolerance: 0.4}
	a := ExtractLevel(g, 0.5, cfg)
	b := ExtractLevel(g, 0.5, cfg)
	if len(a) != len(b) {
		t.Fatal("line count differs")
	}
	for i := range a {
		if len(a[i].Points) != len(b[i].Points) {
			t.Fatalf("line %d point count differs", i)
		}
		for j := range a[i].Points {
			if a[i].Points[j] != b[i].Points[j] {
				t.Fatalf("line %d point %d differs", i, j)
			}
		}
	}
}

// 含未定义区域时，简化不得让边进入未定义区。
func TestSimplifyWithUndefined(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 30, Cols: 30, Kind: "gaussian", Seed: 8, NoDataPct: 0.08})
	cfg := DefaultConfig()
	cfg.Simplify = SimplifyConfig{Enabled: true, Tolerance: 0.5}
	simp := ExtractLevel(g, 1.0, cfg)
	assertNoCrossing(t, simp)
	assertNoRegionCrossing(t, g, 1.0, simp, 0.5)
}

// 多级别共存时，简化弦不得与其他级别的线相交（这里仅验证同级别线间，
// 不同级别在光滑场上几何上本身不应交叉）。
func TestSimplifyMultipleRings(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 50, Cols: 50, Kind: "gaussian", Seed: 21})
	cfg := DefaultConfig()
	cfg.Simplify = SimplifyConfig{Enabled: true, Tolerance: 0.8}
	simp := ExtractLevel(g, 2.0, cfg)
	assertNoCrossing(t, simp)
	assertNoRegionCrossing(t, g, 2.0, simp, 0.8)
}

// 简化只能删除顶点、不能移动顶点；闭合环简化后仍必须是原环的循环子序列。
func TestSimplifyIsSubset(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 60, Cols: 60, Kind: "gaussian", Seed: 33})
	full := ExtractLevel(g, 1.5, DefaultConfig())
	cfg := DefaultConfig()
	cfg.Simplify = SimplifyConfig{Enabled: true, Tolerance: 0.7}
	simp := ExtractLevel(g, 1.5, cfg)

	// 用“点多重集合”包含关系验证：简化的每个点都必须在原始结果中出现。
	orig := map[[2]float64]int{}
	for _, l := range full {
		for _, p := range l.Points {
			orig[pointKey(p)]++
		}
	}
	for _, l := range simp {
		for _, p := range l.Points {
			k := pointKey(p)
			if orig[k] == 0 {
				t.Fatalf("simplified point %v not present in original contours", p)
			}
			orig[k]--
		}
	}
}

func pointKey(p Point) [2]float64 { return [2]float64{p.X, p.Y} }
