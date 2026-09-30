package contour

import "math"

// simplifyLevel performs topology-preserving simplification of all contours of
// one level. Precision <= 0 disables simplification.
//
// A candidate chord replacing a contour sub-path is accepted only when:
//
//  1. every removed point is within precision of the chord (DP tolerance);
//  2. the chord does not properly cross any original contour segment of the
//     level (shared endpoints are allowed), which prevents self-intersection
//     and crossings between contours;
//  3. the lens enclosed by the removed sub-path and the chord contains grid
//     vertices of only one strict sign relative to the level, so the chord
//     never passes through the value region on the other side of the contour.
func simplifyLevel(contours []Contour, precision float64, allSegments []seg, nodes map[edgeKey]Point, grid *Grid, level float64) []Contour {
	if precision <= 0 {
		return contours
	}

	barriers := make([][2]Point, 0, len(allSegments))
	for _, s := range allSegments {
		a, ok1 := nodes[s.a]
		b, ok2 := nodes[s.b]
		if ok1 && ok2 {
			barriers = append(barriers, [2]Point{a, b})
		}
	}

	chordsByContour := make([][][2]Point, len(contours))

	for ci, c := range contours {
		if c.Point || len(c.Points) < 3 {
			continue
		}
		poly := c.Points
		if c.Closed {
			closed := make([]Point, len(poly)+1)
			copy(closed, poly)
			closed[len(poly)] = poly[0]
			poly = closed
		}
		kept := dpSimplify(poly, precision, barriers, grid, level)
		for k := 0; k+1 < len(kept); k++ {
			if !samePoint(kept[k], kept[k+1]) {
				chordsByContour[ci] = append(chordsByContour[ci], [2]Point{kept[k], kept[k+1]})
			}
		}
	}

	// Global pairwise chord check. If two chords properly cross, deterministically
	// fall back to the original polyline for the larger-indexed contour; original
	// segments never cross each other by construction.
	fallback := make(map[int]bool)
	for i := 0; i < len(contours); i++ {
		for j := i + 1; j < len(contours); j++ {
			if fallback[j] {
				continue
			}
			for _, x := range chordsByContour[i] {
				for _, y := range chordsByContour[j] {
					if segmentsIntersect(x[0], x[1], y[0], y[1]) {
						fallback[j] = true
					}
				}
				if fallback[j] {
					break
				}
			}
		}
		cs := chordsByContour[i]
		for a := 0; a < len(cs); a++ {
			for b := a + 1; b < len(cs); b++ {
				if segmentsIntersect(cs[a][0], cs[a][1], cs[b][0], cs[b][1]) {
					fallback[i] = true
				}
			}
		}
	}

	out := make([]Contour, len(contours))
	for ci, c := range contours {
		if fallback[ci] || c.Point || len(c.Points) < 3 || len(chordsByContour[ci]) == 0 {
			out[ci] = c
			continue
		}
		poly := c.Points
		if c.Closed {
			closed := make([]Point, len(poly)+1)
			copy(closed, poly)
			closed[len(poly)] = poly[0]
			poly = closed
		}
		kept := dpSimplify(poly, precision, barriers, grid, level)
		pts := kept
		if c.Closed && len(pts) > 1 && samePoint(pts[0], pts[len(pts)-1]) {
			pts = pts[:len(pts)-1]
		}
		if c.Closed && len(pts) < 3 {
			out[ci] = c
			continue
		}
		out[ci] = Contour{Level: c.Level, Closed: c.Closed, Points: pts}
	}
	return out
}

// dpSimplify runs Douglas-Peucker with topology validation. The returned slice
// always starts at poly[0] and ends at poly[len(poly)-1].
func dpSimplify(poly []Point, precision float64, barriers [][2]Point, grid *Grid, level float64) []Point {
	if len(poly) <= 2 {
		return poly
	}
	keep := make([]bool, len(poly))
	keep[0] = true
	keep[len(poly)-1] = true

	var stack [][2]int
	stack = append(stack, [2]int{0, len(poly) - 1})
	for len(stack) > 0 {
		span := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		lo, hi := span[0], span[1]
		if hi-lo < 2 {
			continue
		}
		a, b := poly[lo], poly[hi]
		maxD := -1.0
		idx := -1
		for k := lo + 1; k < hi; k++ {
			d := distPointSegment(poly[k], a, b)
			if d > maxD {
				maxD, idx = d, k
			}
		}
		if maxD <= precision && chordValid(a, b, poly[lo:hi+1], barriers, grid, level) {
			continue
		}
		keep[idx] = true
		stack = append(stack, [2]int{lo, idx}, [2]int{idx, hi})
	}

	out := make([]Point, 0, len(poly))
	for i, p := range poly {
		if keep[i] {
			out = append(out, p)
		}
	}
	return out
}

// chordValid applies topology checks (2) and (3) documented at simplifyLevel.
func chordValid(a, b Point, subpath []Point, barriers [][2]Point, grid *Grid, level float64) bool {
	if samePoint(a, b) {
		return false
	}
	for _, bar := range barriers {
		if segmentsIntersect(a, b, bar[0], bar[1]) {
			return false
		}
	}

	// Lens polygon: removed sub-path from a back to b, closed by the chord b-a.
	lens := make([]Point, 0, len(subpath)+1)
	lens = append(lens, subpath...)
	lens = append(lens, a)

	minX, maxX := a.X, a.X
	minY, maxY := a.Y, a.Y
	for _, p := range subpath {
		minX = math.Min(minX, p.X)
		maxX = math.Max(maxX, p.X)
		minY = math.Min(minY, p.Y)
		maxY = math.Max(maxY, p.Y)
	}
	i0 := int(math.Ceil(minX)) - 1
	i1 := int(math.Floor(maxX)) + 1
	j0 := int(math.Ceil(minY)) - 1
	j1 := int(math.Floor(maxY)) + 1
	if i0 < 0 {
		i0 = 0
	}
	if j0 < 0 {
		j0 = 0
	}
	if i1 > grid.NX-1 {
		i1 = grid.NX - 1
	}
	if j1 > grid.NY-1 {
		j1 = grid.NY - 1
	}
	pos, neg := false, false
	for j := j0; j <= j1; j++ {
		for i := i0; i <= i1; i++ {
			v := grid.At(i, j)
			if math.IsNaN(v) {
				return false
			}
			p := Point{X: float64(i), Y: float64(j)}
			if !strictPointInRing(p, lens) {
				continue
			}
			switch {
			case v > level:
				pos = true
			case v < level:
				neg = true
			}
			if pos && neg {
				return false
			}
		}
	}
	return true
}

// strictPointInRing reports whether p is strictly inside the ring; points on
// the ring boundary are reported as outside.
func strictPointInRing(p Point, ring []Point) bool {
	n := len(ring)
	for i := 0; i < n; i++ {
		a := ring[i]
		b := ring[(i+1)%n]
		if onSegment(a, b, p) {
			return false
		}
	}
	return pointInRing(p, ring)
}
