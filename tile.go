package contour

import (
	"runtime"
	"sync"
)

type tile struct {
	r0, c0, r1, c1 int
}

// Stats 报告一次提取的执行统计。
type Stats struct {
	Tiles    int
	Workers  int
	Segments int
}

// planTiles 把单元矩形切成互不重叠的分块。
// 块边界处的格边仍由唯一的单元三角片产生穿越点，因此不需要重叠区。
func planTiles(cellRows, cellCols, tileCells int) []tile {
	if tileCells <= 0 {
		// 自动选择：目标每块约 256x256 单元，但保证块数 >= 1。
		tileCells = 256
		if cellRows > 0 && cellCols > 0 {
			nr := (cellRows + tileCells - 1) / tileCells
			nc := (cellCols + tileCells - 1) / tileCells
			if nr*nc > runtime.NumCPU()*4 {
				// 让块数大致与 CPU 数匹配时，放大分块。
				tileCells = cellRows / runtime.NumCPU()
				if t := cellCols / runtime.NumCPU(); t > tileCells {
					tileCells = t
				}
				if tileCells < 64 {
					tileCells = 64
				}
			}
		}
	}
	var tiles []tile
	for r := 0; r < cellRows; r += tileCells {
		r1 := r + tileCells
		if r1 > cellRows {
			r1 = cellRows
		}
		for c := 0; c < cellCols; c += tileCells {
			c1 := c + tileCells
			if c1 > cellCols {
				c1 = cellCols
			}
			tiles = append(tiles, tile{r, c, r1, c1})
		}
	}
	return tiles
}

// ExtractLevelTiled 以分块并行方式提取一个级别的等值线。
// 每个 worker 独立处理一个分块并把线段写入分片缓冲（无共享写、无锁竞争），
// 最后按分块规范顺序合并、全局拼接。由于分块互不重叠且端点键是结构化的
// 全局身份，结果与 ExtractLevel 的单块结果逐点一致，且与分块大小、
// worker 数、调度顺序无关。
func ExtractLevelTiled(g *Grid, level float64, cfg Config) ([]Polyline, Stats) {
	nc, err := cfg.normalized()
	if err != nil {
		panic("contour: invalid config: " + err.Error())
	}
	tiles := planTiles(g.CellRows(), g.CellCols(), nc.TileCells)
	workers := nc.Concurrency
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > len(tiles) {
		workers = len(tiles)
	}
	if workers < 1 {
		workers = 1
	}

	bufs := make([][]Segment, len(tiles))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				t := tiles[i]
				bufs[i] = extractSegments(g, level,
					CellRange{R0: t.r0, C0: t.c0, R1: t.r1, C1: t.c1}, &nc)
			}
		}()
	}
	for i := range tiles {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	// 规范合并：分块按 (r0,c0) 排列（planTiles 已按行优先生成）。
	total := 0
	for _, b := range bufs {
		total += len(b)
	}
	all := make([]Segment, 0, total)
	for _, b := range bufs {
		all = append(all, b...)
	}
	lines := stitch(level, all, g, &nc)
	if nc.Simplify.Enabled {
		nc.simplifyGrid = g
		nc.simplifyLevel = level
		lines = simplifyPolylines(lines, nc)
	}
	return lines, Stats{Tiles: len(tiles), Workers: workers, Segments: total}
}
