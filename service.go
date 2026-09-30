package contour

import (
	"bufio"
	"encoding/json"
	"errors"
	"math"
	"os"
	"runtime"
	"sort"
	"sync"
)

// NaNPolicy defines how undefined (NaN) samples are handled.
type NaNPolicy string

const (
	// NaNMask treats undefined samples as a mask: cells touching a NaN vertex
	// emit no segments; contours may terminate at the mask boundary.
	NaNMask NaNPolicy = "mask"
	// NaNFill replaces undefined samples with Options.FillValue before
	// extraction.
	NaNFill NaNPolicy = "fill"
)

// Options configures extraction semantics.
type Options struct {
	// TileNX/TileNY bound per-task work; <= 0 means a single block.
	TileNX int
	TileNY int
	// Concurrency bounds simultaneous block workers; <= 0 uses NumCPU.
	Concurrency int
	// SimplifyPrecision (<=0 disables) is the maximum geometric tolerance for
	// topology-preserving simplification.
	SimplifyPrecision float64
	NaNPolicy         NaNPolicy
	FillValue         float64
	// EmitPointContours keeps degenerate single-point contours produced when a
	// vertex equals the level (e.g. an extremum sampled exactly at the level).
	EmitPointContours bool
}

// Service incrementally extracts and stores contour results.
type Service struct {
	grid    *Grid
	eff     *Grid
	opts    Options
	mu      sync.Mutex
	results map[float64][]Contour
}

// ResultRecord is one serialized, reproducible result set.
type ResultRecord struct {
	Algorithm string    `json:"algorithm"`
	Options   Options   `json:"options"`
	Contours  []Contour `json:"contours"`
}

// DefaultOptions returns the documented default configuration.
func DefaultOptions() Options {
	return Options{
		TileNX:            64,
		TileNY:            64,
		Concurrency:       runtime.NumCPU(),
		SimplifyPrecision: 0,
		NaNPolicy:         NaNMask,
		EmitPointContours: false,
	}
}

// NewService validates options and constructs a service.
func NewService(g *Grid, opts Options) (*Service, error) {
	if g == nil {
		return nil, errors.New("contour: nil grid")
	}
	if opts.NaNPolicy == "" {
		opts.NaNPolicy = NaNMask
	}
	switch opts.NaNPolicy {
	case NaNMask, NaNFill:
	default:
		return nil, errors.New("contour: unknown NaNPolicy")
	}
	if opts.TileNX == 0 {
		opts.TileNX = 64
	}
	if opts.TileNY == 0 {
		opts.TileNY = 64
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = runtime.NumCPU()
	}
	eff, _ := effectiveGrid(g, opts)
	return &Service{
		grid:    g,
		eff:     eff,
		opts:    opts,
		results: make(map[float64][]Contour),
	}, nil
}

// Extract extracts any not-yet-computed levels and returns all contours for
// the requested levels. Existing levels are never recomputed.
func (s *Service) Extract(levels []float64) ([]Contour, error) {
	for _, level := range levels {
		if math.IsNaN(level) {
			return nil, errors.New("contour: level must not be NaN")
		}
	}
	for _, level := range levels {
		s.mu.Lock()
		_, done := s.results[level]
		s.mu.Unlock()
		if done {
			continue
		}
		cs, err := s.extractLevel(level)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.results[level] = cs
		s.mu.Unlock()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	ls := make([]float64, len(levels))
	copy(ls, levels)
	sort.Float64s(ls)
	out := make([]Contour, 0)
	for _, l := range ls {
		out = append(out, s.results[l]...)
	}
	return out, nil
}

// HasLevel reports whether a level was already extracted.
func (s *Service) HasLevel(level float64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.results[level]
	return ok
}

// Results returns the stored contours for a level (nil if absent).
func (s *Service) Results(level float64) []Contour {
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := s.results[level]
	if cs == nil {
		return nil
	}
	out := make([]Contour, len(cs))
	copy(out, cs)
	return out
}

// Levels returns extracted levels in ascending order.
func (s *Service) Levels() []float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	ls := make([]float64, 0, len(s.results))
	for l := range s.results {
		ls = append(ls, l)
	}
	sort.Float64s(ls)
	return ls
}

// SaveResults writes all stored results deterministically to path.
func (s *Service) SaveResults(path string) error {
	s.mu.Lock()
	ls := make([]float64, 0, len(s.results))
	for l := range s.results {
		ls = append(ls, l)
	}
	sort.Float64s(ls)
	all := make([]Contour, 0)
	for _, l := range ls {
		all = append(all, s.results[l]...)
	}
	rec := ResultRecord{Algorithm: algorithmVersion, Options: s.opts, Contours: all}
	s.mu.Unlock()

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rec); err != nil {
		return err
	}
	return w.Flush()
}

// LoadResults reads a previously saved result record.
func LoadResults(path string) (*ResultRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rec ResultRecord
	if err := json.NewDecoder(bufio.NewReader(f)).Decode(&rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// tileRanges partitions the grid cells into deterministic row-major blocks.
func tileRanges(g *Grid, opts Options) []tileRange {
	tw, th := opts.TileNX, opts.TileNY
	if tw <= 0 {
		tw = g.NX - 1
	}
	if th <= 0 {
		th = g.NY - 1
	}
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	tiles := make([]tileRange, 0)
	for r0 := 0; r0 < g.NY-1; r0 += th {
		r1 := r0 + th
		if r1 > g.NY-1 {
			r1 = g.NY - 1
		}
		for c0 := 0; c0 < g.NX-1; c0 += tw {
			c1 := c0 + tw
			if c1 > g.NX-1 {
				c1 = g.NX - 1
			}
			tiles = append(tiles, tileRange{c0: c0, c1: c1, r0: r0, r1: r1})
		}
	}
	return tiles
}

type tileOutput struct {
	locals []localContour
	segs   []seg
	nodes  map[edgeKey]Point
}

// extractLevel runs block extraction concurrently and merges the results. The
// merge depends only on deterministic per-tile contents (cell order) and the
// key space, so output is identical for any tile count, any concurrency, and
// any scheduling order, and bit-identical to single-block extraction.
func (s *Service) extractLevel(level float64) ([]Contour, error) {
	tiles := tileRanges(s.eff, s.opts)
	outs := make([]tileOutput, len(tiles))

	workers := s.opts.Concurrency
	if workers > len(tiles) {
		workers = len(tiles)
	}
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	tileCh := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range tileCh {
				sg, nd := extractTile(s.eff, tiles[idx], level, s.opts)
				out := tileOutput{locals: chainLocal(level, sg, nd, s.opts)}
				if s.opts.SimplifyPrecision > 0 {
					out.segs = sg
					out.nodes = nd
				}
				outs[idx] = out
			}
		}()
	}
	for idx := range tiles {
		tileCh <- idx
	}
	close(tileCh)
	wg.Wait()

	// Serial phase only stitches boundary ports; its input size is
	// proportional to the total tile boundary length, not the cell count.
	tileLocals := make([][]localContour, len(outs))
	for i, out := range outs {
		tileLocals[i] = out.locals
	}
	contours := stitchContours(level, tileLocals, s.opts)
	if s.opts.SimplifyPrecision > 0 {
		var allSegs []seg
		allNodes := make(map[edgeKey]Point)
		for _, out := range outs {
			allSegs = append(allSegs, out.segs...)
			for k, p := range out.nodes {
				allNodes[k] = p
			}
		}
		contours = simplifyLevel(contours, s.opts.SimplifyPrecision, allSegs, allNodes, s.eff, level)
	}
	return contours, nil
}
