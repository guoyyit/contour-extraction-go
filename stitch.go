package contour

import (
	"math"
	"sort"
)

// Polyline 是一条等值线折线；Closed=true 表示闭合环。
// 闭合环 Points 不含重复末点；开放线由一端数据边界（或未定义区边界）
// 走到另一端，不存在内部悬空端点。
type Polyline struct {
	Level  float64 `json:"level"`
	Closed bool    `json:"closed"`
	Points []Point `json:"points"`

	// barrier 为内部字段：线的任一端点终止于已定义/未定义交界处。
	// 这种线的终止段不参与简化（其几何在未定义三角片内无插值定义）。
	barrier bool
}

type incident struct {
	angle float64
	seg   int
	tie   endKey
}

// stitch 按端点键拼接线段为折线，连接规则见 README“连接规则”一节：
//
//   - 键相同则相连：相邻三角片在共享格点/格边中点上连接，与分块无关。
//   - 一个内部键上有 2 条入射边时直接相连；3 条及以上（鞍部、恰为级别的
//     格点等退化情形）按出射方向极角升序配对相邻方向 (0-1,2-3,...)，
//     落单的一条成为开放端点，结束于该恰为级别的格点。
//   - 位于数据外边界的键、以及 barrier 键（未定义区内边界）不做配对，
//     每条入射边都是一条开放等值线的端点。
//   - 结果规范化排序，相同输入输出逐位一致。
func stitch(level float64, segs []Segment, g *Grid, cfg *Config) []Polyline {
	if len(segs) == 0 {
		return []Polyline{}
	}
	at := make(map[endKey][]incident, len(segs)*2)
	for i := range segs {
		s := &segs[i]
		at[s.KA] = append(at[s.KA], incident{
			angle: math.Atan2(s.A.Y-s.B.Y, s.A.X-s.B.X), seg: i, tie: s.KB})
		at[s.KB] = append(at[s.KB], incident{
			angle: math.Atan2(s.B.Y-s.A.Y, s.B.X-s.A.X), seg: i, tie: s.KA})
	}

	// next[k][segIdx] = 在键 k 处与 segIdx 相连的下一条线段；-1 表示端点。
	next := make(map[endKey]map[int]int, len(at))
	var openKeys []endKey
	for k, inc := range at {
		row := make(map[int]int, len(inc))
		for _, e := range inc {
			row[e.seg] = -1
		}
		boundary := isDataBoundaryKey(k, g) || k.barrier
		if !boundary && len(inc) >= 2 {
			sort.Slice(inc, func(i, j int) bool {
				if inc[i].angle != inc[j].angle {
					return inc[i].angle < inc[j].angle
				}
				return keyLess(inc[i].tie, inc[j].tie)
			})
			for i := 0; i+1 < len(inc); i += 2 {
				row[inc[i].seg] = inc[i+1].seg
				row[inc[i+1].seg] = inc[i].seg
			}
		}
		next[k] = row
		for _, e := range inc {
			if row[e.seg] < 0 {
				openKeys = append(openKeys, k)
				break
			}
		}
	}

	used := make([]bool, len(segs))
	var lines []Polyline

	sortKeys(openKeys)
	for _, k := range openKeys {
		inc := at[k]
		// 开放端点按“其第一条线段”的角度顺序展开。
		sort.Slice(inc, func(i, j int) bool {
			if inc[i].angle != inc[j].angle {
				return inc[i].angle < inc[j].angle
			}
			return inc[i].seg < inc[j].seg
		})
		for _, e := range inc {
			if next[k][e.seg] < 0 && !used[e.seg] {
				lines = append(lines, walkChain(segs, used, next, k, e.seg))
			}
		}
	}

	for start := 0; start < len(segs); start++ {
		if used[start] {
			continue
		}
		lines = append(lines, walkLoop(segs, used, next, start))
	}

	for i := range lines {
		lines[i].Level = level
	}
	return canonicalize(lines)
}

// walkChain 从一个开放端点沿配对关系走到另一个开放端点。
func walkChain(segs []Segment, used []bool, next map[endKey]map[int]int,
	startKey endKey, startSeg int) Polyline {
	var pts []Point
	endBarrier := false
	prevKey := startKey
	curSeg := startSeg
	for {
		used[curSeg] = true
		s := &segs[curSeg]
		var nextKey endKey
		var curPoint, nextPoint Point
		if s.KA == prevKey {
			curPoint, nextPoint, nextKey = s.A, s.B, s.KB
		} else {
			curPoint, nextPoint, nextKey = s.B, s.A, s.KA
		}
		if len(pts) == 0 {
			pts = append(pts, curPoint)
		}
		pts = append(pts, nextPoint)

		ns := next[nextKey][curSeg]
		if ns < 0 {
			endBarrier = nextKey.barrier
			return Polyline{Closed: false, Points: pts, barrier: startKey.barrier || endBarrier}
		}
		prevKey, curSeg = nextKey, ns
	}
}

// walkLoop 提取一个闭合环。
func walkLoop(segs []Segment, used []bool, next map[endKey]map[int]int,
	startSeg int) Polyline {
	var raw []Point
	prevKey := segs[startSeg].KA
	curSeg := startSeg
	for {
		used[curSeg] = true
		s := &segs[curSeg]
		var nextKey endKey
		var curPoint, nextPoint Point
		if s.KA == prevKey {
			curPoint, nextPoint, nextKey = s.A, s.B, s.KB
		} else {
			curPoint, nextPoint, nextKey = s.B, s.A, s.KA
		}
		if len(raw) == 0 {
			raw = append(raw, curPoint)
		}
		raw = append(raw, nextPoint)

		ns := next[nextKey][curSeg]
		if ns < 0 || used[ns] {
			break
		}
		prevKey, curSeg = nextKey, ns
	}
	if len(raw) > 1 && raw[0] == raw[len(raw)-1] {
		raw = raw[:len(raw)-1]
	}
	return Polyline{Closed: true, Points: canonicalLoop(raw)}
}

// canonicalLoop 以字典序最小点为环起点，并取两个遍历方向中字典序较小者。
func canonicalLoop(pts []Point) []Point {
	n := len(pts)
	if n < 2 {
		return pts
	}
	best := 0
	for i := 1; i < n; i++ {
		if pointLess(pts[i], pts[best]) {
			best = i
		}
	}
	fwd := make([]Point, n)
	rev := make([]Point, n)
	for i := 0; i < n; i++ {
		fwd[i] = pts[(best+i)%n]
		rev[i] = pts[(best-i+n)%n]
	}
	for i := 0; i < n; i++ {
		if pointLess(rev[i], fwd[i]) {
			return rev
		}
		if pointLess(fwd[i], rev[i]) {
			return fwd
		}
	}
	return fwd
}

// canonicalize 将开放折线定向为首点字典序不大于末点，再对全部折线排序。
func canonicalize(lines []Polyline) []Polyline {
	for i := range lines {
		l := &lines[i]
		if !l.Closed && len(l.Points) >= 2 &&
			pointLess(l.Points[len(l.Points)-1], l.Points[0]) {
			for a, b := 0, len(l.Points)-1; a < b; a, b = a+1, b-1 {
				l.Points[a], l.Points[b] = l.Points[b], l.Points[a]
			}
		}
	}
	sort.Slice(lines, func(i, j int) bool {
		a, b := lines[i], lines[j]
		if a.Closed != b.Closed {
			return a.Closed
		}
		n := len(a.Points)
		if len(b.Points) < n {
			n = len(b.Points)
		}
		for k := 0; k < n; k++ {
			if pointLess(a.Points[k], b.Points[k]) {
				return true
			}
			if pointLess(b.Points[k], a.Points[k]) {
				return false
			}
		}
		return len(a.Points) < len(b.Points)
	})
	return lines
}

func pointLess(a, b Point) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	return a.Y < b.Y
}

func keyLess(a, b endKey) bool {
	if a.kind != b.kind {
		return a.kind < b.kind
	}
	if a.barrier != b.barrier {
		return !a.barrier
	}
	if a.a != b.a {
		return a.a < b.a
	}
	return a.b < b.b
}

func sortKeys(ks []endKey) {
	sort.Slice(ks, func(i, j int) bool { return keyLess(ks[i], ks[j]) })
}
