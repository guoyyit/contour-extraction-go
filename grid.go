package contour

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// Grid is a regular scalar grid. Vertex (i, j) occupies geometric position
// (i, j): i is the column (x) index in [0, NX-1], j is the row (y) index in
// [0, NY-1]. An undefined sample is represented by math.NaN().
type Grid struct {
	NX int
	NY int
	V  []float64
}

// GenConfig describes a locally generated synthetic grid.
type GenConfig struct {
	Kind          string // "plane", "cone", "saddle", "hills", "flat"
	NX            int
	NY            int
	Level         float64
	UndefinedHole bool
}

// NewGrid creates a grid from row-major values (j outer, i inner). It panics
// when dimensions or the value slice are inconsistent.
func NewGrid(nx, ny int, v []float64) *Grid {
	if nx <= 0 || ny <= 0 {
		panic(fmt.Sprintf("contour: invalid grid dimensions %dx%d", nx, ny))
	}
	if len(v) != nx*ny {
		panic(fmt.Sprintf("contour: expected %d values, got %d", nx*ny, len(v)))
	}
	cp := make([]float64, len(v))
	copy(cp, v)
	return &Grid{NX: nx, NY: ny, V: cp}
}

// Contains reports whether vertex (i, j) is inside the grid.
func (g *Grid) Contains(i, j int) bool {
	return i >= 0 && j >= 0 && i < g.NX && j < g.NY
}

// At returns the sample value at vertex (i, j). Out-of-range access returns
// NaN so that callers never read memory outside the grid.
func (g *Grid) At(i, j int) float64 {
	if !g.Contains(i, j) {
		return math.NaN()
	}
	return g.V[j*g.NX+i]
}

// Generate builds a synthetic grid locally, without any external service.
//
// Supported kinds:
//   - "plane":  f = x - Level
//   - "cone":   f = maxDim - distance(center), a single peak
//   - "saddle": f = (x-cx)^2 - (y-cy)^2
//   - "hills":  f = sin(x)+cos(y), multiple extrema
//   - "flat":   f = Level everywhere
//
// When UndefinedHole is true, a 2x2 block of NaN samples is placed at the
// center to exercise the undefined-value semantics.
func Generate(cfg GenConfig) (*Grid, error) {
	if cfg.NX < 2 || cfg.NY < 2 {
		return nil, fmt.Errorf("contour: generated grid must be at least 2x2, got %dx%d", cfg.NX, cfg.NY)
	}
	nx, ny := cfg.NX, cfg.NY
	v := make([]float64, nx*ny)
	cx := float64(nx-1) / 2
	cy := float64(ny-1) / 2
	maxDim := math.Max(cx, cy)
	for j := 0; j < ny; j++ {
		for i := 0; i < nx; i++ {
			x := float64(i)
			y := float64(j)
			var f float64
			switch cfg.Kind {
			case "plane":
				f = x - cfg.Level
			case "cone":
				f = maxDim - math.Hypot(x-cx, y-cy)
			case "saddle":
				f = (x-cx)*(x-cx) - (y-cy)*(y-cy)
			case "hills":
				f = math.Sin(x*0.7) + math.Cos(y*0.6)
			case "flat":
				f = cfg.Level
			default:
				return nil, fmt.Errorf("contour: unknown generated grid kind %q", cfg.Kind)
			}
			v[j*nx+i] = f
		}
	}
	if cfg.UndefinedHole && nx >= 4 && ny >= 4 {
		ic := nx / 2
		jc := ny / 2
		for _, dj := range []int{0, 1} {
			for _, di := range []int{0, 1} {
				v[(jc+dj)*nx+(ic+di)] = math.NaN()
			}
		}
	}
	return &Grid{NX: nx, NY: ny, V: v}, nil
}

// LoadGrid reads a grid from a local text file:
// first line "NX NY", followed by NY whitespace-separated rows of NX numbers
// (tokens "nan"/"NaN" denote undefined samples).
func LoadGrid(path string) (*Grid, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	var nums []float64
	nx, ny := -1, -1
	row := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if nx < 0 {
			if len(fields) != 2 {
				return nil, errors.New("contour: first non-empty line must be \"NX NY\"")
			}
			nx, err = strconv.Atoi(fields[0])
			if err != nil {
				return nil, fmt.Errorf("contour: bad NX: %w", err)
			}
			ny, err = strconv.Atoi(fields[1])
			if err != nil {
				return nil, fmt.Errorf("contour: bad NY: %w", err)
			}
			if nx <= 0 || ny <= 0 {
				return nil, fmt.Errorf("contour: invalid dimensions %dx%d", nx, ny)
			}
			nums = make([]float64, 0, nx*ny)
			continue
		}
		if len(fields) != nx {
			return nil, fmt.Errorf("contour: row %d has %d values, expected %d", row, len(fields), nx)
		}
		for _, tok := range fields {
			x, err := strconv.ParseFloat(tok, 64)
			if err != nil {
				return nil, fmt.Errorf("contour: bad value %q on row %d: %w", tok, row, err)
			}
			nums = append(nums, x)
		}
		row++
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if ny < 0 {
		return nil, errors.New("contour: empty grid file")
	}
	if row != ny {
		return nil, fmt.Errorf("contour: expected %d rows, got %d", ny, row)
	}
	return &Grid{NX: nx, NY: ny, V: nums}, nil
}

// Save writes the grid to a local text file.
func (g *Grid) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if _, err := fmt.Fprintf(w, "%d %d\n", g.NX, g.NY); err != nil {
		return err
	}
	for j := 0; j < g.NY; j++ {
		for i := 0; i < g.NX; i++ {
			x := g.At(i, j)
			tok := strconv.FormatFloat(x, 'g', -1, 64)
			if i > 0 {
				tok = " " + tok
			}
			if _, err := w.WriteString(tok); err != nil {
				return err
			}
		}
		if _, err := w.WriteString("\n"); err != nil {
			return err
		}
	}
	return w.Flush()
}
