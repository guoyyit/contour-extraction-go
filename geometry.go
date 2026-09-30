package contour

import "math"

// Point is a geometric position in grid-index coordinate space.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func cross(ox, oy, ax, ay, bx, by float64) float64 {
	return (ax-ox)*(by-oy) - (ay-oy)*(bx-ox)
}

func orient(a, b, p Point) float64 {
	return cross(a.X, a.Y, b.X, b.Y, p.X, p.Y)
}

func samePoint(a, b Point) bool {
	return a.X == b.X && a.Y == b.Y
}

// onSegment reports whether collinear point p lies on segment a-b.
func onSegment(a, b, p Point) bool {
	if orient(a, b, p) != 0 {
		return false
	}
	return p.X >= min(a.X, b.X) && p.X <= max(a.X, b.X) &&
		p.Y >= min(a.Y, b.Y) && p.Y <= max(a.Y, b.Y)
}

// segmentsIntersect reports whether open segment a-b and c-d cross at a point
// that is not a shared endpoint. Touching only at identical endpoints is not
// considered an intersection.
func segmentsIntersect(a, b, c, d Point) bool {
	// Degenerate (point) segments never produce crossings.
	if samePoint(a, b) || samePoint(c, d) {
		return false
	}
	o1 := orient(a, b, c)
	o2 := orient(a, b, d)
	o3 := orient(c, d, a)
	o4 := orient(c, d, b)
	if ((o1 > 0 && o2 < 0) || (o1 < 0 && o2 > 0)) &&
		((o3 > 0 && o4 < 0) || (o3 < 0 && o4 > 0)) {
		return true
	}
	// T-junction or collinear overlap where one segment's endpoint lies in
	// the strict interior of the other.
	if onSegment(a, b, c) && !samePoint(c, a) && !samePoint(c, b) {
		return true
	}
	if onSegment(a, b, d) && !samePoint(d, a) && !samePoint(d, b) {
		return true
	}
	if onSegment(c, d, a) && !samePoint(a, c) && !samePoint(a, d) {
		return true
	}
	if onSegment(c, d, b) && !samePoint(b, c) && !samePoint(b, d) {
		return true
	}
	return false
}

// pointInRing reports whether p is inside a closed ring (ray casting).
func pointInRing(p Point, ring []Point) bool {
	inside := false
	n := len(ring)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		a, b := ring[j], ring[i]
		if onSegment(a, b, p) {
			return true
		}
		if (a.Y > p.Y) != (b.Y > p.Y) {
			xAt := a.X + (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y)
			if xAt > p.X {
				inside = !inside
			}
		}
	}
	return inside
}

// distPointSegment returns the Euclidean distance from p to segment a-b.
func distPointSegment(p, a, b Point) float64 {
	if samePoint(a, b) {
		dx, dy := p.X-a.X, p.Y-a.Y
		return math.Hypot(dx, dy)
	}
	vx, vy := b.X-a.X, b.Y-a.Y
	wx, wy := p.X-a.X, p.Y-a.Y
	t := (wx*vx + wy*vy) / (vx*vx + vy*vy)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	qx, qy := a.X+t*vx, a.Y+t*vy
	return math.Hypot(p.X-qx, p.Y-qy)
}
