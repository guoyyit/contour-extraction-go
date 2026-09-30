package contour

import (
	"math"
	"sort"
)

// Contour is one extracted contour line.
type Contour struct {
	Level  float64 `json:"level"`
	Closed bool    `json:"closed"`
	Points []Point `json:"points"`
	Point  bool    `json:"point,omitempty"`
}

// canonicalizeContours removes every dependency of the output ordering on
// segment traversal and block order: point contours first, then open paths in
// canonical start->end orientation, then closed rings rotated and oriented
// canonically. All comparisons are bitwise, so output is byte-stable across
// tile counts, worker counts and scheduling order.
func canonicalizeContours(cs []Contour) {
	for i := range cs {
		c := &cs[i]
		if c.Point || len(c.Points) <= 1 {
			continue
		}
		if c.Closed {
			c.Points = canonicalRing(c.Points)
		} else {
			c.Points = dedupeConsecutive(c.Points)
			if pointLess(c.Points[len(c.Points)-1], c.Points[0]) {
				rev := make([]Point, len(c.Points))
				for k, p := range c.Points {
					rev[len(c.Points)-1-k] = p
				}
				c.Points = rev
			}
		}
	}
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.Point != b.Point {
			return a.Point
		}
		return polyLess(a.Points, b.Points)
	})
}

func pointKey(p Point) [2]uint64 {
	return [2]uint64{math.Float64bits(p.X), math.Float64bits(p.Y)}
}

func pointLess(a, b Point) bool {
	ka, kb := pointKey(a), pointKey(b)
	if ka[0] != kb[0] {
		return ka[0] < kb[0]
	}
	return ka[1] < kb[1]
}

// polyLess compares point sequences lexicographically using bitwise float
// ordering, without allocating.
func polyLess(a, b []Point) bool {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if pointLess(a[i], b[i]) {
			return true
		}
		if pointLess(b[i], a[i]) {
			return false
		}
	}
	return len(a) < len(b)
}

// canonicalRing rotates a closed ring to start at its lexicographically
// smallest point and chooses the smaller direction between the two traversal
// orientations.
func canonicalRing(ring []Point) []Point {
	if len(ring) <= 1 {
		return ring
	}
	ring = dedupeConsecutive(ring)
	minIdx := 0
	for i := 1; i < len(ring); i++ {
		if pointLess(ring[i], ring[minIdx]) {
			minIdx = i
		}
	}
	fwd := make([]Point, len(ring))
	rev := make([]Point, len(ring))
	n := len(ring)
	for k := 0; k < n; k++ {
		pf := ring[(minIdx+k)%n]
		pr := ring[(minIdx-k+n)%n]
		fwd[k] = pf
		rev[k] = pr
	}
	if polyLess(rev, fwd) {
		return rev
	}
	return fwd
}

// dedupeConsecutive removes adjacent equal points, including a closing point
// equal to the first. At least the first point is retained.
func dedupeConsecutive(p []Point) []Point {
	if len(p) <= 1 {
		return p
	}
	out := p[:1]
	for i := 1; i < len(p); i++ {
		if !samePoint(p[i], out[len(out)-1]) {
			out = append(out, p[i])
		}
	}
	if len(out) > 1 && samePoint(out[0], out[len(out)-1]) {
		out = out[:len(out)-1]
	}
	return out
}
