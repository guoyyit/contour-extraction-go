package contour

import "math"

// simplifyPolylines 对每条折线做拓扑保持的 Douglas-Peucker 简化。
//
// 每条候选弦只有同时满足以下条件才被接受，否则退化为逐段保留：
//  1. 不与同级别其他等值线的原始边相交（无线间交叉）；
//  2. 沿弦在每个穿过的单元内用双线性插值密集采样，
//     不得有样本明确落到等值线另一侧（不穿越被分隔的异值区域）；
//  3. 同一条线内候选弦也不允许与其它候选弦相交（无自交）——
//     由于弦始终是“原折线子链”的捷径，且只在 DP 递归中替换
//     相邻子链，自交由不跨区域保证，另在成环贪心时逐弦检测。
//
// 简化结果仍是原等值线的子集拓扑：只会拉直，不会改变区域邻接关系。
func simplifyPolylines(lines []Polyline, cfg Config) []Polyline {
	if !cfg.Simplify.Enabled || cfg.Simplify.Tolerance <= 0 || cfg.simplifyGrid == nil {
		return lines
	}
	tol := cfg.Simplify.Tolerance
	g := cfg.simplifyGrid

	type refSeg struct {
		line int
		a, b Point
	}
	var global []refSeg
	for li, l := range lines {
		n := len(l.Points)
		for i := 0; i+1 < n; i++ {
			global = append(global, refSeg{li, l.Points[i], l.Points[i+1]})
		}
		if l.Closed && n > 2 {
			global = append(global, refSeg{li, l.Points[n-1], l.Points[0]})
		}
	}
	intersectsOthers := func(li int, p, q Point) bool {
		for _, rs := range global {
			if rs.line == li {
				continue
			}
			if segmentsCross(p, q, rs.a, rs.b) {
				return true
			}
		}
		return false
	}
	sideOK := func(p, q Point) bool {
		return chordStaysOnSide(g, cfg.simplifyLevel, tol, p, q)
	}

	out := make([]Polyline, len(lines))
	for li := range lines {
		if lines[li].barrier {
			// 终止于未定义区的线不做弦简化，避免弦进入无插值定义的区域。
			out[li] = lines[li]
			continue
		}
		out[li] = simplifyOne(lines[li], li, tol, intersectsOthers, sideOK)
	}
	return out
}

type othersTest func(li int, p, q Point) bool
type chordSideTest func(p, q Point) bool

func simplifyOne(l Polyline, lineIdx int, tol float64,
	intersectsOthers othersTest, sideOK chordSideTest) Polyline {
	pts := l.Points
	n := len(pts)
	if n < 3 {
		return l
	}
	validChord := func(i, j int) bool {
		p, q := pts[i], pts[j]
		if intersectsOthers(lineIdx, p, q) {
			return false
		}
		if !sideOK(p, q) {
			return false
		}
		return true
	}

	if l.Closed {
		rp := simplifyClosed(pts, tol, validChord)
		if len(rp) < 3 {
			return l
		}
		return Polyline{Level: l.Level, Closed: true, Points: canonicalLoop(rp)}
	}

	// dpRec 返回区间 [i,j] 简化后的下标序列（含两端）。
	var dpRec func(i, j int) []int
	dpRec = func(i, j int) []int {
		if j == i+1 {
			return []int{i, j}
		}
		// 整条子链已在容差内且弦拓扑合法：直接用弦替代。
		dmax := -1.0
		idx := -1
		for k := i + 1; k < j; k++ {
			d := pointSegmentDistance(pts[k], pts[i], pts[j])
			if d > dmax {
				dmax, idx = d, k
			}
		}
		if dmax <= tol && validChord(i, j) {
			return []int{i, j}
		}
		// 否则在最大偏差点拆分；若连最大偏差点弦都非法（典型为子链
		// 绕未定义区域），则退化为逐原始段保留，保证拓扑不被破坏。
		if idx < 0 {
			out := make([]int, 0, j-i+1)
			for k := i; k <= j; k++ {
				out = append(out, k)
			}
			return out
		}
		// 若两侧弦中有任何一侧非法，说明该区间不能跨过 idx 简化，
		// 退化为 i..idx 与 idx..j 各自递归（最终回退到原始段）。
		left := dpRec(i, idx)
		right := dpRec(idx, j)
		return append(left, right[1:]...)
	}
	idxs := dpRec(0, n-1)
	rp := make([]Point, 0, len(idxs))
	for _, i := range idxs {
		rp = append(rp, pts[i])
	}
	if len(rp) < 2 {
		return l
	}
	return Polyline{Level: l.Level, Closed: false, Points: rp}
}

// simplifyClosed 用贪心 DP 思想简化闭合环：从一条初始弦开始，
// 不断在偏差最大且候选弦合法的位置加入顶点；任何非法弦区间原样保留。
func simplifyClosed(pts []Point, tol float64, validChord func(i, j int) bool) []Point {
	n := len(pts)
	keep := make([]bool, n)
	// 初始以字典序最小点与对径点把环切成两段开放链。
	start := 0
	for i := 1; i < n; i++ {
		if pointLess(pts[i], pts[start]) {
			start = i
		}
	}
	opp := (start + n/2) % n
	keep[start], keep[opp] = true, true

	// 迭代处理每一条当前弦，直到所有弦偏差均不超过容差或全部保留。
	changed := true
	for changed {
		changed = false
		i := start
		for {
			j := nextKept(keep, i)
			dmax := -1.0
			idx := -1
			for k := (i + 1) % n; k != j; k = (k + 1) % n {
				d := pointSegmentDistance(pts[k], pts[i], pts[j])
				if d > dmax {
					dmax, idx = d, k
				}
			}
			if dmax > tol && idx >= 0 {
				if validChord(i, idx) && validChord(idx, j) {
					keep[idx] = true
					changed = true
				} else {
					// 该弦区间无法安全拉直：原样保留区间内全部点。
					for k := (i + 1) % n; k != j; k = (k + 1) % n {
						keep[k] = true
					}
					changed = true
				}
			}
			i = j
			if i == start {
				break
			}
		}
	}

	rp := make([]Point, 0, n)
	for i, k := range keep {
		if k {
			rp = append(rp, pts[i])
		}
	}
	return rp
}

func nextKept(keep []bool, i int) int {
	n := len(keep)
	for s := 1; s <= n; s++ {
		j := (i + s) % n
		if keep[j] {
			return j
		}
	}
	return i
}

// chordStaysOnSide 沿候选弦每 0.2 个格距采样一次双线性插值。
// 弦两端都在等值线上（v==level），合法的弦只能沿等值线附近行走；
// 一旦某样本明显位于等值线另一侧（超过 tol 的数值裕量），说明弦穿越了
// 被等值线分隔的异值区域，拒绝该弦。
func chordStaysOnSide(g *Grid, level, tol float64, p, q Point) bool {
	dist := math.Hypot(q.X-p.X, q.Y-p.Y)
	steps := int(math.Ceil(dist/0.2)) + 1
	margin := 1.5 * tol
	// 基准侧由第一个明显偏离级别的样本确定；其后样本不得出现在反侧。
	var sideSet bool
	var side bool
	for s := 1; s < steps; s++ {
		t := float64(s) / float64(steps)
		x := p.X + (q.X-p.X)*t
		y := p.Y + (q.Y-p.Y)*t
		v, defined := bilinearAt(g, x, y)
		if !defined {
			return false
		}
		d := v - level
		if math.Abs(d) > margin {
			sb := math.Signbit(d)
			if !sideSet {
				side, sideSet = sb, true
			} else if sb != side {
				return false
			}
		}
	}
	return true
}

// bilinearAt 在规则格网上做双线性插值。
// 单元完全定义时按双线性取值；单元含未定义角点时，提取使用与
// extractSegments 相同的 左上-右下 固定对角剖分：点若落入含未定义
// 角点的三角片则 ok=false，落入另一（完全定义的）三角片时按该三角形
// 的重心坐标线性插值。由此简化校验与提取的“定义区域”完全一致。
func bilinearAt(g *Grid, x, y float64) (float64, bool) {
	if x < 0 || y < 0 || x > float64(g.cols-1) || y > float64(g.rows-1) {
		return 0, false
	}
	c0 := int(math.Floor(x))
	r0 := int(math.Floor(y))
	c1, r1 := c0+1, r0+1
	if c1 > g.cols-1 {
		c1 = c0
	}
	if r1 > g.rows-1 {
		r1 = r0
	}
	undef := [4]bool{
		g.IsUndefined(r0, c0), g.IsUndefined(r0, c1),
		g.IsUndefined(r1, c0), g.IsUndefined(r1, c1),
	}
	if !undef[0] && !undef[1] && !undef[2] && !undef[3] {
		return bilinear4(g, r0, c0, r1, c1, x, y), true
	}
	tx := x - float64(c0)
	ty := y - float64(r0)
	// 三角片1：左上-右上-右下（对角线 tx+ty>=1 的一侧为右下）。
	// 点在三角片1 内当且仅当 ty >= 1-tx，即 tx+ty >= 1。
	if c0 != c1 && r0 != r1 && math.Abs(tx+ty-1) < 1e-12 {
		tx += 1e-9
	}
	inTri1 := tx+ty >= 1
	if inTri1 {
		// 三角片1 角点：左上(r0,c0)、右上(r0,c1)、右下(r1,c1)。
		if undef[0] || undef[1] || undef[3] {
			return 0, false
		}
		vTL, vTR, vBR := g.Value(r0, c0), g.Value(r0, c1), g.Value(r1, c1)
		// 顶点 TL(0,0), TR(1,0), BR(1,1)，重心权重
		// w_TL=1-tx, w_TR=tx-ty, w_BR=ty（用 TR 解出）。
		v := vTL*(1-tx) + vTR*(tx-ty) + vBR*ty
		return v, true
	}
	// 三角片2：左上(r0,c0)、右下(r1,c1)、左下(r1,c0)。
	if undef[0] || undef[3] || undef[2] {
		return 0, false
	}
	vTL2, vBR2, vBL2 := g.Value(r0, c0), g.Value(r1, c1), g.Value(r1, c0)
	// 顶点 TL(0,0), BR(1,1), BL(0,1)，重心权重
	// w_TL=1-ty, w_BR=tx, w_BL=ty-tx。
	v := vTL2*(1-ty) + vBR2*tx + vBL2*(ty-tx)
	return v, true
}

func bilinear4(g *Grid, r0, c0, r1, c1 int, x, y float64) float64 {
	tx := x - float64(c0)
	ty := y - float64(r0)
	v00 := g.Value(r0, c0)
	v10 := g.Value(r0, c1)
	v01 := g.Value(r1, c0)
	v11 := g.Value(r1, c1)
	v := v00*(1-tx)*(1-ty) + v10*tx*(1-ty) + v01*(1-tx)*ty + v11*tx*ty
	return v
}
func pointSegmentDistance(p, a, b Point) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / l2
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(p.X-(a.X+t*dx), p.Y-(a.Y+t*dy))
}

// segmentsCross 判断两条开线段是否严格横交（仅端点相接不算交叉）。
func segmentsCross(p1, p2, p3, p4 Point) bool {
	d1 := cross2(p4, p3, p1)
	d2 := cross2(p4, p3, p2)
	d3 := cross2(p2, p1, p3)
	d4 := cross2(p2, p1, p4)
	return ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0))
}

func cross2(a, b, p Point) float64 {
	return (b.X-a.X)*(p.Y-a.Y) - (b.Y-a.Y)*(p.X-a.X)
}
