package contour

import "sort"

// localContour is a contour traced inside one tile. Open traces carry the
// edge keys of their two end nodes (ports on the tile boundary); closed traces
// carry closed=true.
type localContour struct {
	points []Point
	closed bool
	pa     edgeKey
	pb     edgeKey
}

// chainLocal works exactly like chainContours but additionally records the
// endpoint edge keys needed for cross-tile stitching.
func chainLocal(level float64, segments []seg, nodes map[edgeKey]Point, opts Options) []localContour {
	adj := make(map[edgeKey][]int, len(nodes)*2)
	for idx, s := range segments {
		adj[s.a] = append(adj[s.a], idx)
		adj[s.b] = append(adj[s.b], idx)
	}

	used := make([]bool, len(segments))
	out := make([]localContour, 0)

	type walkResult struct {
		points []Point
		start  edgeKey
		end    edgeKey
		closed bool
	}
	walk := func(startNode edgeKey, firstSeg int) walkResult {
		points := []Point{nodes[startNode]}
		prevNode := startNode
		curSeg := firstSeg
		endNode := startNode
		closed := false
		for {
			used[curSeg] = true
			s := segments[curSeg]
			nextNode := s.b
			if nextNode == prevNode {
				nextNode = s.a
			}
			points = append(points, nodes[nextNode])
			endNode = nextNode
			if nextNode == startNode {
				closed = true
				break
			}
			if len(adj[nextNode]) == 1 {
				break
			}
			neighbors := adj[nextNode]
			next := -1
			for _, idx := range neighbors {
				if !used[idx] {
					next = idx
					break
				}
			}
			if next < 0 {
				closed = true
				break
			}
			prevNode = nextNode
			curSeg = next
		}
		return walkResult{points: points, start: startNode, end: endNode, closed: closed}
	}

	normalize := func(w walkResult) (localContour, bool) {
		pts := w.points
		if w.closed && len(pts) > 1 {
			allSame := true
			for _, p := range pts[1:] {
				if !samePoint(p, pts[0]) {
					allSame = false
					break
				}
			}
			if allSame {
				pts = pts[:1]
			}
		}
		if len(pts) == 1 {
			if !opts.EmitPointContours {
				return localContour{}, false
			}
			return localContour{points: pts, closed: true}, true
		}
		if w.closed {
			if samePoint(pts[0], pts[len(pts)-1]) {
				pts = pts[:len(pts)-1]
			}
			return localContour{points: pts, closed: true}, true
		}
		return localContour{points: pts, closed: false, pa: w.start, pb: w.end}, true
	}

	boundary := make([]edgeKey, 0)
	for k, es := range adj {
		if len(es) == 1 {
			boundary = append(boundary, k)
		}
	}
	sort.Slice(boundary, func(a, b int) bool { return boundary[a] < boundary[b] })
	for _, k := range boundary {
		es := adj[k]
		if len(es) != 1 || used[es[0]] {
			continue
		}
		if lc, ok := normalize(walk(k, es[0])); ok {
			out = append(out, lc)
		}
	}

	remaining := make([]edgeKey, 0)
	for k, es := range adj {
		for _, idx := range es {
			if !used[idx] {
				remaining = append(remaining, k)
				break
			}
		}
	}
	sort.Slice(remaining, func(a, b int) bool { return remaining[a] < remaining[b] })
	for _, k := range remaining {
		es := adj[k]
		first := -1
		for _, idx := range es {
			if !used[idx] {
				first = idx
				break
			}
		}
		if first < 0 {
			continue
		}
		if lc, ok := normalize(walk(k, first)); ok {
			out = append(out, lc)
		}
	}

	return out
}

// pathEdge is one local open trace viewed as an edge between two ports.
type pathEdge struct {
	lc localContour
}

// stitchContours joins local per-tile traces. A port (crossed edge key) on a
// shared tile boundary is visited by exactly the two incident tiles, so the
// global trace graph again contains only cycles and boundary-to-boundary
// paths. The result depends solely on the (deterministically ordered) local
// traces and is identical for every tiling.
func stitchContours(level float64, tileLocals [][]localContour, opts Options) []Contour {
	type entry struct {
		edge int
		port edgeKey
	}
	adj := make(map[edgeKey][]entry)
	var edges []pathEdge
	var closedIn []localContour

	for _, locals := range tileLocals {
		for _, lc := range locals {
			if lc.closed {
				closedIn = append(closedIn, lc)
				continue
			}
			id := len(edges)
			edges = append(edges, pathEdge{lc: lc})
			adj[lc.pa] = append(adj[lc.pa], entry{edge: id, port: lc.pa})
			adj[lc.pb] = append(adj[lc.pb], entry{edge: id, port: lc.pb})
		}
	}

	usedEdge := make([]bool, len(edges))
	result := make([]Contour, 0, len(closedIn)+len(adj)/2)

	emitClosed := func(lc localContour) {
		c := Contour{Level: level, Closed: true, Points: lc.points}
		if len(lc.points) == 1 {
			c.Point = true
		}
		result = append(result, c)
	}

	// appendTrace appends an oriented edge's points to acc, dropping the
	// duplicated junction point; returns the port reached at the other end.
	appendTrace := func(acc []Point, edgeID int, enterAt edgeKey) ([]Point, edgeKey) {
		lc := edges[edgeID].lc
		var pts []Point
		var exit edgeKey
		if enterAt == lc.pa {
			pts = lc.points
			exit = lc.pb
		} else {
			pts = reversePoints(lc.points)
			exit = lc.pa
		}
		start := 0
		if len(acc) > 0 && len(pts) > 0 && samePoint(acc[len(acc)-1], pts[0]) {
			start = 1
		}
		return append(acc, pts[start:]...), exit
	}

	traverse := func(startPort edgeKey, firstEdge int, firstEnter edgeKey, closed bool) Contour {
		acc := []Point{}
		port := firstEnter
		edgeID := firstEdge
		startCoord := portCoord(edges, firstEdge, firstEnter)
		acc = append(acc, startCoord)
		for {
			usedEdge[edgeID] = true
			var nextPort edgeKey
			acc, nextPort = appendTrace(acc, edgeID, port)
			if nextPort == startPort {
				closed = true
				break
			}
			others := adj[nextPort]
			next := -1
			for _, e := range others {
				if !usedEdge[e.edge] {
					next = e.edge
					port = nextPort
					break
				}
			}
			if next < 0 {
				break
			}
			edgeID = next
		}
		if closed && len(acc) > 1 && samePoint(acc[0], acc[len(acc)-1]) {
			acc = acc[:len(acc)-1]
		}
		c := Contour{Level: level, Closed: closed, Points: acc}
		if len(acc) == 1 {
			c.Point = true
		}
		return c
	}

	ports := make([]edgeKey, 0, len(adj))
	for p := range adj {
		ports = append(ports, p)
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })

	// Open global contours start from degree-one ports (outer/mask boundary).
	for _, p := range ports {
		es := adj[p]
		if len(es) != 1 || usedEdge[es[0].edge] {
			continue
		}
		result = append(result, traverse(p, es[0].edge, p, false))
	}
	// Remaining unused edges are closed global contours.
	for _, p := range ports {
		es := adj[p]
		first := -1
		for _, e := range es {
			if !usedEdge[e.edge] {
				first = e.edge
				break
			}
		}
		if first < 0 {
			continue
		}
		result = append(result, traverse(p, first, p, false))
	}
	// Closed contours local to a single tile never crossed a tile boundary.
	for _, lc := range closedIn {
		emitClosed(lc)
	}

	canonicalizeContours(result)
	return result
}

func portCoord(edges []pathEdge, edgeID int, enterAt edgeKey) Point {
	lc := edges[edgeID].lc
	if enterAt == lc.pa {
		return lc.points[0]
	}
	return lc.points[len(lc.points)-1]
}

func reversePoints(p []Point) []Point {
	r := make([]Point, len(p))
	for i, q := range p {
		r[len(p)-1-i] = q
	}
	return r
}
