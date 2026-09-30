package contour

import (
	"errors"
	"math"
	"sort"
)

// UndefinedPolicy 定义未定义格点（NaN）的处理语义。
type UndefinedPolicy string

const (
	// UndefinedBarrier（唯一策略，默认）：未定义格点视为屏障，
	// 包含未定义格点的三角片不参与提取，等值线止于已定义/未定义交界的边中点，
	// 绝不穿入未定义区域。
	UndefinedBarrier UndefinedPolicy = "barrier"
)

// ExactLevelPolicy 定义取值恰好等于等值级别的格点的处理语义。
type ExactLevelPolicy string

const (
	// ExactPerturb：符号扰动，按格点坐标做确定性决胜，保证连接唯一。
	ExactPerturb ExactLevelPolicy = "perturb"
	// ExactStrict：严格按 f > level 分类，等值线可止于恰为级别的格点。
	ExactStrict ExactLevelPolicy = "strict"
)

// FlatPolicy 定义整片取值恰为级别的平坦区域的处理语义。
type FlatPolicy string

const (
	// FlatSuppress：全部角点恰为级别的单元内不生成线段（默认），
	// 避免在平坦高原内部产生伪闭合；高原边界上的线段仍会保留并成为
	// 止于高原边界的开放折线。
	FlatSuppress FlatPolicy = "suppress"
	// FlatDraw：按符号扰动在平坦区域内生成确定性连接线，
	// 平坦高原内部会出现之字形等值线，仅在需要该语义时使用。
	FlatDraw FlatPolicy = "draw"
)

// Config 是等值线提取服务的全部配置。
type Config struct {
	// Levels 为待提取的等值级别；服务也支持增量追加。
	Levels []float64 `json:"levels"`
	// Concurrency 为分块并行的 worker 数；<=0 表示 runtime.NumCPU。
	Concurrency int `json:"concurrency"`
	// TileCells 为每个分块覆盖的单元边长；<=0 表示按规模自动选择。
	TileCells int `json:"tile_cells"`

	Undefined  UndefinedPolicy  `json:"undefined"`
	ExactLevel ExactLevelPolicy `json:"exact_level"`
	Flat       FlatPolicy       `json:"flat"`

	Simplify SimplifyConfig `json:"simplify"`

	// simplifyGrid/simplifyLevel 为内部字段，供简化阶段访问源数据。
	simplifyGrid  *Grid
	simplifyLevel float64
}

// SimplifyConfig 配置拓扑保持简化。
type SimplifyConfig struct {
	Enabled bool `json:"enabled"`
	// Tolerance 为简化容差（格网坐标单位），必须 > 0。
	Tolerance float64 `json:"tolerance"`
}

func DefaultConfig() Config {
	return Config{
		Concurrency: 0,
		TileCells:   0,
		Undefined:   UndefinedBarrier,
		ExactLevel:  ExactPerturb,
		Flat:        FlatSuppress,
	}
}

// normalized 返回填好默认值、级别去重排序后的配置副本。
func (c Config) normalized() (Config, error) {
	out := c
	if out.Undefined == "" {
		out.Undefined = UndefinedBarrier
	}
	if out.ExactLevel == "" {
		out.ExactLevel = ExactPerturb
	}
	if out.Flat == "" {
		out.Flat = FlatSuppress
	}
	if err := out.validate(); err != nil {
		return out, err
	}
	out.Levels = sortedUniqueLevels(out.Levels)
	return out, nil
}

func (c *Config) validate() error {
	switch c.Undefined {
	case UndefinedBarrier, "":
	default:
		return errors.New("config: invalid Undefined policy: " + string(c.Undefined))
	}
	switch c.ExactLevel {
	case ExactPerturb, ExactStrict, "":
	default:
		return errors.New("config: invalid ExactLevel policy: " + string(c.ExactLevel))
	}
	switch c.Flat {
	case FlatSuppress, FlatDraw, "":
	default:
		return errors.New("config: invalid Flat policy: " + string(c.Flat))
	}
	if c.Simplify.Enabled && c.Simplify.Tolerance < 0 {
		return errors.New("config: simplify tolerance must be >= 0")
	}
	if c.Concurrency < 0 {
		return errors.New("config: concurrency must be >= 0")
	}
	if c.TileCells < 0 {
		return errors.New("config: tile cells must be >= 0")
	}
	for _, l := range c.Levels {
		if math.IsNaN(l) {
			return errors.New("config: level must not be NaN")
		}
	}
	return nil
}

// sortedUniqueLevels 对级别去重排序，保证相同集合无论输入顺序都产生相同结果。
func sortedUniqueLevels(levels []float64) []float64 {
	if len(levels) == 0 {
		return nil
	}
	cp := append([]float64(nil), levels...)
	sort.Float64s(cp)
	out := cp[:0]
	for _, l := range cp {
		if len(out) == 0 || l != out[len(out)-1] {
			out = append(out, l)
		}
	}
	return out
}
