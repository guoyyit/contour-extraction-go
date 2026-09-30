package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	contour "github.com/guoyyit/contour-extraction-go"
)

func parseLevels(s string) ([]float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("levels are required")
	}
	var out []float64
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		v, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			return nil, fmt.Errorf("bad level %q: %w", tok, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "contour:", err)
	os.Exit(1)
}

func main() {
	var (
		gridPath  = flag.String("grid", "", "local grid file (first line \"NX NY\", then NY rows); use -gen to create instead")
		genKind   = flag.String("gen", "", "generate a synthetic grid: plane|cone|saddle|hills|flat")
		genNX     = flag.Int("nx", 64, "generated grid columns")
		genNY     = flag.Int("ny", 64, "generated grid rows")
		genLevel  = flag.Float64("gen-level", 0, "level parameter of the generated grid")
		hole      = flag.Bool("nan-hole", false, "insert a 2x2 NaN block into the generated grid")
		saveGrid  = flag.String("save-grid", "", "write the generated/loaded grid to this local file")
		levelsStr = flag.String("levels", "", "comma-separated contour levels")
		tileNX    = flag.Int("tile-nx", 64, "block width in cells (<=0: single block)")
		tileNY    = flag.Int("tile-ny", 64, "block height in cells (<=0: single block)")
		workers   = flag.Int("concurrency", 0, "parallel block workers (<=0: NumCPU)")
		precision = flag.Float64("simplify", 0, "topology-preserving simplification precision (<=0 disabled)")
		nanPolicy = flag.String("nan-policy", "mask", "undefined-value policy: mask|fill")
		fill      = flag.Float64("fill-value", 0, "fill value when -nan-policy=fill")
		points    = flag.Bool("point-contours", false, "emit degenerate point contours")
		out       = flag.String("out", "", "save results (JSON) to this local file")
	)
	flag.Parse()

	levels, err := parseLevels(*levelsStr)
	if err != nil {
		fail(err)
	}

	var grid *contour.Grid
	if *genKind != "" {
		grid, err = contour.Generate(contour.GenConfig{
			Kind:          *genKind,
			NX:            *genNX,
			NY:            *genNY,
			Level:         *genLevel,
			UndefinedHole: *hole,
		})
	} else if *gridPath != "" {
		grid, err = contour.LoadGrid(*gridPath)
	} else {
		fail(fmt.Errorf("either -grid or -gen is required"))
	}
	if err != nil {
		fail(err)
	}
	if *saveGrid != "" {
		if err := grid.Save(*saveGrid); err != nil {
			fail(err)
		}
	}

	opts := contour.Options{
		TileNX:            *tileNX,
		TileNY:            *tileNY,
		Concurrency:       *workers,
		SimplifyPrecision: *precision,
		NaNPolicy:         contour.NaNPolicy(*nanPolicy),
		FillValue:         *fill,
		EmitPointContours: *points,
	}
	svc, err := contour.NewService(grid, opts)
	if err != nil {
		fail(err)
	}

	// First pass: any levels already loaded from a stored result could be
	// skipped; here we simply extract. Levels are independent, so passing a
	// subset never recomputes earlier ones.
	cs, err := svc.Extract(levels)
	if err != nil {
		fail(err)
	}

	nLines, nPoints := 0, 0
	for _, c := range cs {
		if c.Point {
			nPoints++
		} else {
			nLines++
		}
	}
	fmt.Printf("extracted %d contour line(s), %d point contour(s) for %d level(s)\n",
		nLines, nPoints, len(levels))

	if *out != "" {
		if err := svc.SaveResults(*out); err != nil {
			fail(err)
		}
		fmt.Println("results saved to", *out)
	}
}
