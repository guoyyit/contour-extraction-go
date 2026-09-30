package contour

// edgeKey uniquely identifies a grid edge:
// even keys encode horizontal edges, odd keys encode vertical edges.
type edgeKey int64

// seg is a contour segment inside one cell, connecting two crossed edges.
type seg struct {
	a edgeKey
	b edgeKey
}

// tileRange is an inclusive vertex range of a processing block.
type tileRange struct {
	c0, c1 int // column range [c0, c1]
	r0, r1 int // row range [r0, r1]
}

// Edge key layout: horizontal edge (i, j)-(i+1, j) -> 2*(int64(j)*nx+i);
// vertical edge (i, j)-(i, j+1)     -> 2*(int64(j)*nx+i)+1.
func hKey(i, j, nx int) edgeKey {
	return edgeKey(2 * (int64(j)*int64(nx) + int64(i)))
}

func vKey(i, j, nx int) edgeKey {
	return edgeKey(2*(int64(j)*int64(nx)+int64(i)) + 1)
}

// Crossing edge indices local to a cell:
// 0: bottom (i,j)-(i+1,j), 1: right (i+1,j)-(i+1,j+1),
// 2: top (i,j+1)-(i+1,j+1), 3: left (i,j)-(i,j+1).

// msPairs gives the deterministic segment pairings for the 16 marching
// squares cases (corners bit order: BL=1, BR=2, TR=4, TL=8). Saddle cases 5
// and 10 use pairing 0; the caller flips to pairing 1 when the resolved saddle
// choice demands it.
var msPairs = [16][][2]int{
	0:  nil,
	1:  {{3, 0}},
	2:  {{0, 1}},
	3:  {{3, 1}},
	4:  {{1, 2}},
	5:  {{3, 0}, {1, 2}},
	6:  {{0, 2}},
	7:  {{3, 2}},
	8:  {{2, 3}},
	9:  {{0, 2}},
	10: {{3, 0}, {1, 2}},
	11: {{1, 2}},
	12: {{3, 1}},
	13: {{0, 1}},
	14: {{3, 0}},
	15: nil,
}

// saddlePairs is the alternative pairing (pairing 1) for saddle cases.
var saddlePairs = map[int][][2]int{
	5:  {{0, 1}, {2, 3}},
	10: {{0, 3}, {1, 2}},
}

func hasNaN(v [4]float64) bool {
	for _, x := range v {
		if x != x {
			return true
		}
	}
	return false
}

// effectiveGrid returns the grid used for extraction after applying the NaN
// policy. With NaNMask the original grid is returned (cells touching NaN are
// skipped); with NaNFill a copied grid has NaN samples replaced.
func effectiveGrid(g *Grid, opts Options) (*Grid, bool) {
	if opts.NaNPolicy != NaNFill {
		return g, false
	}
	v := make([]float64, len(g.V))
	copy(v, g.V)
	replaced := false
	for i, x := range v {
		if x != x {
			v[i] = opts.FillValue
			replaced = true
		}
	}
	return &Grid{NX: g.NX, NY: g.NY, V: v}, replaced
}

// extractTile extracts all segments of one level from one block of cells.
// It returns the segments in a deterministic (row, column) order and the
// geometric crossing positions keyed by edge.
func extractTile(g *Grid, tr tileRange, level float64, opts Options) ([]seg, map[edgeKey]Point) {
	nx := g.NX
	segments := make([]seg, 0)
	nodes := make(map[edgeKey]Point)

	// nodeAt records the crossing point of a crossed edge. Crossing positions
	// depend only on the edge values, so recomputing over shared tile
	// boundaries yields identical keys and coordinates.
	nodeAt := func(key edgeKey, t float64, ax, ay, bx, by float64) {
		nodes[key] = Point{X: ax + (bx-ax)*t, Y: ay + (by-ay)*t}
	}

	for j := tr.r0; j < tr.r1; j++ {
		for i := tr.c0; i < tr.c1; i++ {
			vBL := g.At(i, j)
			vBR := g.At(i+1, j)
			vTR := g.At(i+1, j+1)
			vTL := g.At(i, j+1)
			vals := [4]float64{vBL, vBR, vTR, vTL}
			if hasNaN(vals) {
				// Masked cell: it contributes no segments. Contours from
				// neighboring cells end on the surrounding edges, which thus
				// become mask-boundary endpoints.
				continue
			}

			keys := [4]edgeKey{
				hKey(i, j, nx),
				vKey(i+1, j, nx),
				hKey(i, j+1, nx),
				vKey(i, j, nx),
			}
			ep := [4][2]Point{
				{{float64(i), float64(j)}, {float64(i + 1), float64(j)}},
				{{float64(i + 1), float64(j)}, {float64(i + 1), float64(j + 1)}},
				{{float64(i), float64(j + 1)}, {float64(i + 1), float64(j + 1)}},
				{{float64(i), float64(j)}, {float64(i), float64(j + 1)}},
			}
			ev := [4][2]float64{
				{vBL, vBR},
				{vBR, vTR},
				{vTL, vTR},
				{vBL, vTL},
			}

			corners := 0
			if vBL >= level {
				corners |= 1
			}
			if vBR >= level {
				corners |= 2
			}
			if vTR >= level {
				corners |= 4
			}
			if vTL >= level {
				corners |= 8
			}
			if corners == 0 || corners == 15 {
				continue
			}

			crossed := make([]int, 0, 4)
			for e := 0; e < 4; e++ {
				if t, ok := edgeInterp(ev[e][0], ev[e][1], level); ok {
					crossed = append(crossed, e)
					a, b := ep[e][0], ep[e][1]
					nodeAt(keys[e], t, a.X, a.Y, b.X, b.Y)
				}
			}

			pairs := msPairs[corners]
			if (corners == 5 || corners == 10) && resolveSaddle(vals, level) == 1 {
				pairs = saddlePairs[corners]
			}
			for _, pr := range pairs {
				segments = append(segments, seg{a: keys[pr[0]], b: keys[pr[1]]})
			}
		}
	}
	return segments, nodes
}
