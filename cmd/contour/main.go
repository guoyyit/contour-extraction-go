// Command contour 在本地运行等值线提取：从本地文件读取格网或本地生成
// 合成数据，按指定级别提取，输出 JSON 等值线结果。不依赖任何外部服务。
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	contour "github.com/guoyyit/contour-extraction-go"
)

func main() {
	var (
		in       = flag.String("in", "", "本地格网文本文件（rows cols + 数值，nan=未定义）")
		out      = flag.String("out", "", "等值线 JSON 输出路径；缺省输出到 stdout")
		kind     = flag.String("kind", "gaussian", "本地生成数据类型: gaussian|plane|flat|singlepeak|random")
		rows     = flag.Int("rows", 64, "生成数据行数")
		cols     = flag.Int("cols", 64, "生成数据列数")
		seed     = flag.Int64("seed", 1, "生成数据随机种子（确定性）")
		nodatap  = flag.Float64("nodata-pct", 0, "未定义格点比例 [0,1)")
		levels   = flag.String("levels", "0,1,2", "逗号分隔的等值级别")
		workers  = flag.Int("workers", 0, "并行 worker 数，0=CPU 数")
		tile     = flag.Int("tile", 0, "分块单元边长，0=自动")
		tol      = flag.Float64("simplify-tol", 0, "拓扑保持简化容差，0=不简化")
		exact    = flag.String("exact-level", string(contour.ExactPerturb), "perturb|strict")
		flat     = flag.String("flat", string(contour.FlatSuppress), "suppress|draw")
		writeGrd = flag.String("write-grid", "", "把所用格网写成本地文本文件（便于复现）")
	)
	flag.Parse()

	var g *contour.Grid
	var err error
	if *in != "" {
		g, err = contour.ReadGridFile(*in)
	} else {
		g, err = contour.GenerateGrid(contour.GenerateSpec{
			Rows: *rows, Cols: *cols, Kind: *kind, Seed: *seed, NoDataPct: *nodatap,
		})
	}
	if err != nil {
		fatal(err)
	}
	if *writeGrd != "" {
		if err := contour.WriteGridFile(*writeGrd, g); err != nil {
			fatal(err)
		}
	}

	lv, err := parseLevels(*levels)
	if err != nil {
		fatal(err)
	}
	cfg := contour.DefaultConfig()
	cfg.Levels = lv
	cfg.Concurrency = *workers
	cfg.TileCells = *tile
	cfg.ExactLevel = contour.ExactLevelPolicy(*exact)
	cfg.Flat = contour.FlatPolicy(*flat)
	if *tol > 0 {
		cfg.Simplify = contour.SimplifyConfig{Enabled: true, Tolerance: *tol}
	}

	svc := contour.NewService(g, cfg)
	svc.AddLevels(lv...)
	snap := svc.Snapshot()

	if *out == "" {
		fmt.Printf("levels=%v lines=%d\n", snap.Levels, len(snap.Lines))
		for _, l := range snap.Lines {
			shape := "open"
			if l.Closed {
				shape = "closed"
			}
			fmt.Printf("  level=%g %s points=%d\n", l.Level, shape, len(l.Points))
		}
		return
	}
	if err := contour.WriteContoursJSON(*out, snap); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %d contour lines to %s\n", len(snap.Lines), *out)
}

func parseLevels(s string) ([]float64, error) {
	var out []float64
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		v, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid level %q: %w", tok, err)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no levels given")
	}
	return out, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "contour:", err)
	os.Exit(1)
}
