package contour

import (
	"errors"
	"math"
)

// Grid 是规则格网标量数据，行优先存储；NaN 表示未定义值。
type Grid struct {
	rows   int
	cols   int
	values []float64
}

// NewGrid 用行优先的值数组构造格网。值为 NaN 表示未定义格点。
func NewGrid(rows, cols int, values []float64) (*Grid, error) {
	if rows < 1 || cols < 1 {
		return nil, errors.New("grid: rows and cols must be >= 1")
	}
	if len(values) != rows*cols {
		return nil, errors.New("grid: values length must equal rows*cols")
	}
	cp := make([]float64, len(values))
	copy(cp, values)
	return &Grid{rows: rows, cols: cols, values: cp}, nil
}

func (g *Grid) Rows() int { return g.rows }

func (g *Grid) Cols() int { return g.cols }

// CellRows / CellCols 返回单元（cell）数，格网顶点数为 rows*cols。
func (g *Grid) CellRows() int { return g.rows - 1 }

func (g *Grid) CellCols() int { return g.cols - 1 }

func (g *Grid) Value(r, c int) float64 {
	return g.values[r*g.cols+c]
}

func (g *Grid) IsUndefined(r, c int) bool {
	return math.IsNaN(g.values[r*g.cols+c])
}

func (g *Grid) Clone() *Grid {
	cp := make([]float64, len(g.values))
	copy(cp, g.values)
	return &Grid{rows: g.rows, cols: g.cols, values: cp}
}

// GenerateSpec 描述本地合成数据。
type GenerateSpec struct {
	Rows int
	Cols int
	// Kind 取值：gaussian（默认，多峰光滑场）、plane、flat、singlepeak、random。
	Kind string
	// Seed 是确定性随机源；相同 Seed 永远产生相同数据。
	Seed      int64
	NoDataPct float64
}

// rng 是确定性 LCG（数值配方），保证跨机器与版本复现。
type rng struct{ state uint64 }

func newRNG(seed int64) *rng {
	s := uint64(seed)
	if s == 0 {
		s = 0x9e3779b97f4a7c15
	}
	return &rng{state: s}
}

func (r *rng) float64() float64 {
	// MMIX LCG 参数。
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return float64(r.state>>11) / float64(uint64(1)<<53)
}

// GenerateGrid 在本地生成规则格网标量数据，不依赖任何外部服务。
func GenerateGrid(spec GenerateSpec) (*Grid, error) {
	if spec.Rows < 2 || spec.Cols < 2 {
		return nil, errors.New("grid: generator requires Rows, Cols >= 2")
	}
	if spec.NoDataPct < 0 || spec.NoDataPct >= 1 {
		return nil, errors.New("grid: NoDataPct must be in [0,1)")
	}
	kind := spec.Kind
	if kind == "" {
		kind = "gaussian"
	}
	rows, cols := spec.Rows, spec.Cols
	vals := make([]float64, rows*cols)
	rnd := newRNG(spec.Seed)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			x := float64(c) / float64(cols-1)
			y := float64(r) / float64(rows-1)
			switch kind {
			case "gaussian":
				v := 0.0
				for _, b := range [][3]float64{
					{0.30, 0.35, 0.12},
					{0.72, 0.28, 0.09},
					{0.55, 0.70, 0.15},
					{0.20, 0.80, 0.07},
					{0.85, 0.80, 0.10},
				} {
					dx, dy := x-b[0], y-b[1]
					v += b[2] * math.Exp(-(dx*dx+dy*dy)/(2*0.03*0.03))
				}
				vals[r*cols+c] = 10*v + 2*math.Sin(6*x)*math.Cos(5*y)
			case "plane":
				vals[r*cols+c] = 5*x + 3*y
			case "flat":
				vals[r*cols+c] = 1
			case "singlepeak":
				dx, dy := x-0.5, y-0.5
				vals[r*cols+c] = math.Exp(-(dx*dx + dy*dy) / 0.01)
			case "random":
				vals[r*cols+c] = rnd.float64()
			default:
				return nil, errors.New("grid: unknown generate kind: " + kind)
			}
		}
	}
	if spec.NoDataPct > 0 {
		holeRnd := newRNG(spec.Seed ^ 0x5deece66d)
		for i := range vals {
			if holeRnd.float64() < spec.NoDataPct {
				vals[i] = math.NaN()
			}
		}
	}
	return &Grid{rows: rows, cols: cols, values: vals}, nil
}
