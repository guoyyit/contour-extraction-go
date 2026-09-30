package contour

import "math"

// Point 是格网坐标中的点（x=列方向，y=行方向，格点间距为 1）。
type Point struct {
	X float64
	Y float64
}

// endKey 是线段端点的结构化身份：格点或格边。
// 相邻单元产生的同一端点具有相同的键，与计算顺序、分块方式无关。
//
// kind 取值：1=格点(a=节点id)，2=横边(a=r,b=左侧c)，
// 3=纵边(a=顶部r,b=c)，4=单元对角线(a=r,b=c，左上单元)。
// barrier=true 表示该点是已定义/未定义交界边上的屏障中点。
type endKey struct {
	kind    uint8
	barrier bool
	a       int32
	b       int32
}

// Segment 是位于一个三角片内的等值线段。
type Segment struct {
	Level  float64
	A, B   Point
	KA, KB endKey
}

// CellRange 是单元（cell）矩形范围，行列为半开区间 [R0,R1)x[C0,C1)。
type CellRange struct {
	R0, C0, R1, C1 int
}

func hedgeKey(r, c int) endKey { return endKey{kind: 2, a: int32(r), b: int32(c)} }

func vedgeKey(r, c int) endKey { return endKey{kind: 3, a: int32(r), b: int32(c)} }

func diagKey(r, c int) endKey { return endKey{kind: 4, a: int32(r), b: int32(c)} }

// pointForKey 返回端点键对应的几何点（x=列, y=行）。
func pointForKey(k endKey, g *Grid) Point {
	switch k.kind {
	case 1:
		id := int(k.a)
		return Point{X: float64(id % g.cols), Y: float64(id / g.cols)}
	case 2:
		return Point{X: float64(k.b) + 0.5, Y: float64(k.a)} // 横边：(c+0.5, r)
	case 3:
		return Point{X: float64(k.b), Y: float64(k.a) + 0.5} // 竖边：(c, r+0.5)
	default:
		return Point{X: float64(k.b) + 0.5, Y: float64(k.a) + 0.5}
	}
}

// isDataBoundaryKey 判断端点是否位于格网外边界。
// barrier 端点位于未定义区域内边界，不属于数据外边界。
func isDataBoundaryKey(k endKey, g *Grid) bool {
	if k.barrier {
		return false
	}
	switch k.kind {
	case 1:
		id := int(k.a)
		r, c := id/g.cols, id%g.cols
		return r == 0 || r == g.rows-1 || c == 0 || c == g.cols-1
	case 2:
		return int(k.a) == 0 || int(k.a) == g.rows-1
	case 3:
		return int(k.b) == 0 || int(k.b) == g.cols-1
	default:
		return false
	}
}

// isEdgeMidpointAdjacentToNaN 判断点是否是“一端未定义”的格边中点。
func isEdgeMidpointAdjacentToNaN(p Point, g *Grid) bool {
	// 水平格边中点 (c+0.5, r)。
	r := int(math.Round(p.Y))
	if frac := p.X - math.Floor(p.X); math.Abs(frac-0.5) < 1e-12 &&
		r >= 0 && r < g.rows {
		c := int(math.Floor(p.X))
		if c >= 0 && c+1 < g.cols && g.IsUndefined(r, c) != g.IsUndefined(r, c+1) {
			return true
		}
	}
	// 竖直格边中点 (c, r+0.5)。
	c := int(math.Round(p.X))
	if frac := p.Y - math.Floor(p.Y); math.Abs(frac-0.5) < 1e-12 &&
		c >= 0 && c < g.cols {
		rr := int(math.Floor(p.Y))
		if rr >= 0 && rr+1 < g.rows && g.IsUndefined(rr, c) != g.IsUndefined(rr+1, c) {
			return true
		}
	}
	// 单元对角线中点 (c+0.5, r+0.5)。
	if fx, fy := p.X-math.Floor(p.X), p.Y-math.Floor(p.Y); math.Abs(fx-0.5) < 1e-12 && math.Abs(fy-0.5) < 1e-12 {
		r := int(math.Floor(p.Y))
		c0 := int(math.Floor(p.X))
		if r >= 0 && r+1 < g.rows && c0 >= 0 && c0+1 < g.cols &&
			g.IsUndefined(r, c0) != g.IsUndefined(r+1, c0+1) {
			return true
		}
	}
	return false
}

type gridNode struct {
	id    int
	r, c  int
	v     float64
	undef bool
}

// isAbove 按配置语义对格点分类。NaN 不调用本函数。
//
// ExactPerturb 使用字典序无穷小扰动 v' = v + ε*c + ε²*r
// （0 < ε² << ε << 任意两个不同浮点值的差）。该扰动是符号计算，
// 不修改任何输入值，并且数学上保证：
//   - 任意格点的 v' 都不等于 level，等值线在每个三角片内是一段直线；
//   - 连接关系由格点坐标唯一决定，与处理顺序、分块方式无关；
//   - 不存在孤立断点；取值恰为级别的格点仅作为穿越点落在格边上，
//     而不会使等值线“终止”在该格点。
func isAbove(v, level float64, r, c int, cfg *Config) bool {
	if v != level {
		return v > level
	}
	if cfg.ExactLevel == ExactStrict {
		return false
	}
	if c != 0 {
		return true
	}
	return r > 0
}

// edgeCrossing 计算等值线在一条格边上的穿越点及其身份键。
// above0/above1 为两端按策略得到的分类。
func edgeCrossing(level float64, n0, n1 gridNode, above0, above1 bool, k endKey) (Point, endKey, bool) {
	switch {
	case n0.undef || n1.undef:
		// 已定义/未定义交界：等值线止于边中点（barrier 语义）。
		k.barrier = true
		p := Point{X: float64(n0.c+n1.c) / 2, Y: float64(n0.r+n1.r) / 2}
		return p, k, true
	case n0.v == level && n1.v == level && above0 != above1:
		// 两端都恰为级别但扰动后分列两侧：穿越点取边中点。
		p := Point{X: float64(n0.c+n1.c) / 2, Y: float64(n0.r+n1.r) / 2}
		return p, k, true
	default:
		// 某一端恰为级别时，线性插值参数 t 为 0 或 1，穿越点退化为
		// 该格点。键仍然保留“所在边”的身份：多条边可在几何上交汇于
		// 同一格点，但只有共享同一条边的三角片才应彼此连接，因此这里
		// 绝不把不同边归并成同一个格点键。
		t := (level - n0.v) / (n1.v - n0.v)
		p := Point{
			X: float64(n0.c) + t*float64(n1.c-n0.c),
			Y: float64(n0.r) + t*float64(n1.r-n0.r),
		}
		return p, k, true
	}
}

// canonicalEnd 把几何上恰好落在格点上的穿越点归并为格点键：
// 多条不同格边（及对角线）在恰为级别的格点处几何重合时，它们必须在
// 同一个点上连接；真正位于边内部的穿越点保留其边键，不会被错误归并。
func canonicalEnd(p Point, k endKey, g *Grid) (Point, endKey) {
	if k.barrier {
		return p, k
	}
	ri := int(math.Round(p.Y))
	ci := int(math.Round(p.X))
	if ri < 0 || ri >= g.rows || ci < 0 || ci >= g.cols {
		return p, k
	}
	if math.Abs(p.X-float64(ci)) < 1e-12 && math.Abs(p.Y-float64(ri)) < 1e-12 {
		return p, endKey{kind: 1, a: int32(ri*g.cols + ci)}
	}
	return p, k
}

// extractSegments 在给定单元范围内逐三角片提取等值线段。
// 每个单元沿 左上-右下 对角线切成两片，消除鞍部二义性；
// 线段全部位于互不重叠的三角片内部，故线之间不会交叉。
func extractSegments(g *Grid, level float64, cr CellRange, cfg *Config) []Segment {
	segs := make([]Segment, 0, 16)
	for r := cr.R0; r < cr.R1; r++ {
		for c := cr.C0; c < cr.C1; c++ {
			a := gridNode{id: r*g.cols + c, r: r, c: c, v: g.Value(r, c), undef: g.IsUndefined(r, c)}
			b := gridNode{id: r*g.cols + c + 1, r: r, c: c + 1, v: g.Value(r, c+1), undef: g.IsUndefined(r, c+1)}
			d := gridNode{id: (r+1)*g.cols + c, r: r + 1, c: c, v: g.Value(r+1, c), undef: g.IsUndefined(r+1, c)}
			e := gridNode{id: (r+1)*g.cols + c + 1, r: r + 1, c: c + 1, v: g.Value(r+1, c+1), undef: g.IsUndefined(r+1, c+1)}

			// FlatSuppress：四个角点全部恰为级别的平坦单元内不生成线段。
			if cfg.Flat == FlatSuppress &&
				!a.undef && !b.undef && !d.undef && !e.undef &&
				a.v == level && b.v == level && d.v == level && e.v == level {
				continue
			}

			// 三角片1：左上-右上-右下；三角片2：左上-右下-左下。
			segs = extractTriangle(segs, level, cfg, a, b, e,
				hedgeKey(r, c), vedgeKey(r, c+1), diagKey(r, c))
			segs = extractTriangle(segs, level, cfg, a, e, d,
				diagKey(r, c), hedgeKey(r+1, c), vedgeKey(r, c))
		}
	}
	for i := range segs {
		segs[i].A, segs[i].KA = canonicalEnd(segs[i].A, segs[i].KA, g)
		segs[i].B, segs[i].KB = canonicalEnd(segs[i].B, segs[i].KB, g)
	}
	// 键归并后可能出现两端同键（零长）或重复段，做一次确定的去重。
	segs = dedupeSegments(segs)
	return segs
}

func dedupeSegments(segs []Segment) []Segment {
	out := segs[:0]
	type sk struct {
		a, b endKey
	}
	seen := make(map[sk]bool, len(segs))
	for _, s := range segs {
		if s.KA == s.KB {
			continue
		}
		x, y := s.KA, s.KB
		if keyLess(y, x) {
			x, y = y, x
		}
		k := sk{x, y}
		if seen[k] {
			continue
		}
		seen[k] = true
		if s.KA != x {
			s.KA, s.KB = s.KB, s.KA
			s.A, s.B = s.B, s.A
		}
		out = append(out, s)
	}
	return out
}

// extractTriangle 处理一个三角片，三个节点按顺序 n0,n1,n2，
// 对应边 n0-n1、n1-n2、n2-n0 的身份键依次为 k01,k12,k20。
func extractTriangle(out []Segment, level float64, cfg *Config,
	n0, n1, n2 gridNode, k01, k12, k20 endKey) []Segment {
	nodes := [3]gridNode{n0, n1, n2}
	undef := 0
	for _, n := range nodes {
		if n.undef {
			undef++
		}
	}
	if undef > 0 {
		return triangleWithUndefined(out, level, cfg, nodes, [3]endKey{k01, k12, k20}, undef)
	}

	above := [3]bool{
		isAbove(nodes[0].v, level, nodes[0].r, nodes[0].c, cfg),
		isAbove(nodes[1].v, level, nodes[1].r, nodes[1].c, cfg),
		isAbove(nodes[2].v, level, nodes[2].r, nodes[2].c, cfg),
	}
	if above[0] == above[1] && above[1] == above[2] {
		return out
	}
	type triEdge struct {
		i, j int
		k    endKey
	}
	edges := [3]triEdge{{0, 1, k01}, {1, 2, k12}, {2, 0, k20}}
	type cross struct {
		p Point
		k endKey
	}
	var crossings [2]cross
	nc := 0
	for _, ed := range edges {
		if above[ed.i] == above[ed.j] {
			continue
		}
		p, k, ok := edgeCrossing(level, nodes[ed.i], nodes[ed.j], above[ed.i], above[ed.j], ed.k)
		if !ok {
			continue
		}
		crossings[nc] = cross{p, k}
		nc++
	}
	if nc == 2 && crossings[0].k != crossings[1].k {
		// 字典序扰动下两键可能映射到同一个级别格点（零长段），丢弃。
		if crossings[0].p == crossings[1].p {
			return out
		}
		out = append(out, Segment{
			Level: level,
			A:     crossings[0].p, B: crossings[1].p,
			KA: crossings[0].k, KB: crossings[1].k,
		})
	}
	return out
}

// triangleWithUndefined 实现 barrier 语义：恰有一个未定义角点的三角片，
// 仅在两个已定义角点取值分列级别两侧时生成一条线段：从二者之间的真实
// 穿越点连到“未定义角点↔上侧已定义角点”边的中点。等值线因此明确止于
// 已定义/未定义交界处，不穿入未定义区域，也不在交界处凭空造闭合线。
func triangleWithUndefined(out []Segment, level float64, cfg *Config,
	nodes [3]gridNode, keys [3]endKey, undefCount int) []Segment {
	if undefCount != 1 {
		return out
	}
	var def [2]int
	nd := 0
	ux := -1
	for i, n := range nodes {
		if n.undef {
			ux = i
		} else {
			def[nd] = i
			nd++
		}
	}
	i0, i1 := def[0], def[1]
	ab0 := isAbove(nodes[i0].v, level, nodes[i0].r, nodes[i0].c, cfg)
	ab1 := isAbove(nodes[i1].v, level, nodes[i1].r, nodes[i1].c, cfg)
	if ab0 == ab1 {
		return out
	}
	edgeKey := func(i, j int) endKey {
		if (i == 0 && j == 1) || (i == 1 && j == 0) {
			return keys[0]
		}
		if (i == 1 && j == 2) || (i == 2 && j == 1) {
			return keys[1]
		}
		return keys[2]
	}
	pReal, kReal, ok := edgeCrossing(level, nodes[i0], nodes[i1], ab0, ab1, edgeKey(i0, i1))
	if !ok {
		return out
	}
	up := i0
	if ab1 {
		up = i1
	}
	pBar, kBar, ok := edgeCrossing(level, nodes[up], nodes[ux], true, false, edgeKey(up, ux))
	if !ok || kReal == kBar {
		return out
	}
	return append(out, Segment{
		Level: level,
		A:     pReal, B: pBar,
		KA: kReal, KB: kBar,
	})
}
