package contour

import "math"

// algorithmVersion tags the deterministic extraction semantics so that stored
// historical results can be reproduced with the same code and configuration.
const algorithmVersion = "marching-squares-v1"

// edgeInterp returns the interpolation parameter t in [0,1] of the level
// crossing along an edge with endpoint values va, vb, and whether the edge is
// crossed. A vertex whose value equals the level is classified inside
// (value >= level), so the contour passes exactly through such a vertex.
func edgeInterp(va, vb, level float64) (t float64, crosses bool) {
	aIn := va >= level
	bIn := vb >= level
	if aIn == bIn {
		return 0, false
	}
	// Exactly one endpoint is inside.
	if aIn && vb < level {
		// crossing from va (in) toward vb (out)
		t = (va - level) / (va - vb)
	} else {
		t = (level - va) / (vb - va)
	}
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return t, true
}

// resolveSaddle returns the deterministic pairing choice (0 or 1) for a saddle
// cell (diagonal inside corners), based on the bilinear value at the cell
// center.
//
// vals order: [bottom-left, bottom-right, top-right, top-left].
// Choice 0 connects the two corners that are at/above the level (the cell
// center is classified above the level, so the inside region is connected
// through the center); choice 1 connects the two below-level corners.
// The center itself equals the level is resolved as inside ("center-in"),
// which keeps the rule deterministic and fully specified.
func resolveSaddle(vals [4]float64, level float64) int {
	center := 0.0
	for _, v := range vals {
		center += v
	}
	center /= 4
	if math.IsNaN(center) {
		// Callers must skip cells touching NaN; treat defensively as outside.
		return 1
	}
	if center >= level {
		return 0
	}
	return 1
}
