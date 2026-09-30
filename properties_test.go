package contour

import "testing"

// TestContourTopologicalProperties 对多组数据/级别验证提取结果满足：
//   - 每条线闭合，或两端都在数据外边界 / 未定义区边界；
//   - 任意两条线段不交叉；
//   - 每个顶点都确实位于等值线上（穿越点值==级别）。
func TestContourTopologicalProperties(t *testing.T) {
	specs := []GenerateSpec{
		{Rows: 33, Cols: 37, Kind: "gaussian", Seed: 1},
		{Rows: 33, Cols: 37, Kind: "gaussian", Seed: 2, NoDataPct: 0.07},
		{Rows: 40, Cols: 20, Kind: "plane", Seed: 3},
		{Rows: 30, Cols: 30, Kind: "singlepeak", Seed: 4},
		{Rows: 25, Cols: 25, Kind: "random", Seed: 5},
	}
	levels := []float64{0.0, 0.5, 1.0}
	cfg := DefaultConfig()
	for si, spec := range specs {
		g, err := GenerateGrid(spec)
		if err != nil {
			t.Fatal(si, err)
		}
		for _, level := range levels {
			lines := ExtractLevel(g, level, cfg)
			for li, l := range lines {
				if len(l.Points) < 2 {
					t.Fatalf("spec %d level %v line %d: <2 points", si, level, li)
				}
				if !l.Closed {
					for i, p := range []Point{l.Points[0], l.Points[len(l.Points)-1]} {
						if onOuterBoundary(p, g) {
							continue
						}
						if l.barrier && isEdgeMidpointAdjacentToNaN(p, g) {
							continue
						}
						t.Fatalf("spec %d level %v open endpoint %d=%v not on domain boundary",
							si, level, i, p)
					}
				}
				for _, p := range l.Points {
					if l.barrier && isEdgeMidpointAdjacentToNaN(p, g) {
						continue
					}
					if !pointLiesOnLevel(g, p, level) {
						t.Fatalf("spec %d level %v vertex %v not on contour", si, level, p)
					}
				}
			}
			assertNoCrossing(t, lines)
		}
	}
}

// pointLiesOnLevel 判断顶点是否是某条格边（横、竖或单元对角线）上的
// 等值线穿越点：端点取值跨级别（允许相等），且线性插值在该点等于级别。
func pointLiesOnLevel(g *Grid, p Point, level float64) bool {
	const eps = 1e-9
	check := func(r0, c0, r1, c1 int, u float64) bool {
		// u 为点在该边的归一化参数 [0,1]。
		if u < -eps || u > 1+eps {
			return false
		}
		if g.IsUndefined(r0, c0) || g.IsUndefined(r1, c1) {
			return false
		}
		a, b := g.Value(r0, c0), g.Value(r1, c1)
		if !((a <= level && b >= level) || (a >= level && b <= level)) {
			return false
		}
		v := a + (b-a)*u
		d := v - level
		return d*d < eps
	}

	// 枚举相邻格点构成的横边、竖边与 TL-BR 对角线。
	ri, ci := int(p.Y+0.5), int(p.X+0.5)
	for dr := -1; dr <= 0; dr++ {
		for dc := -1; dc <= 0; dc++ {
			r, c := ri+dr, ci+dc
			// 横边 (r,c)-(r,c+1)
			if r >= 0 && r < g.rows && c >= 0 && c+1 < g.cols {
				if p.Y == float64(r) && check(r, c, r, c+1, p.X-float64(c)) {
					return true
				}
			}
			// 竖边 (r,c)-(r+1,c)
			if r >= 0 && r+1 < g.rows && c >= 0 && c < g.cols {
				if p.X == float64(c) && check(r, c, r+1, c, p.Y-float64(r)) {
					return true
				}
			}
			// 对角线 (r,c)-(r+1,c+1)
			if r >= 0 && r+1 < g.rows && c >= 0 && c+1 < g.cols {
				u := p.X - float64(c)
				vv := p.Y - float64(r)
				if vv >= u-eps && vv <= u+eps && check(r, c, r+1, c+1, u) {
					return true
				}
			}
		}
	}
	return false
}
