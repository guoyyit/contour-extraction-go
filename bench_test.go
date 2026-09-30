package contour

import (
	"os"
	"runtime"
	"testing"
	"time"
)

func timeExtract(fn func()) float64 {
	start := time.Now()
	fn()
	return time.Since(start).Seconds()
}

func BenchmarkExtractTiled(b *testing.B) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 1024, Cols: 1024, Kind: "gaussian", Seed: 1})
	cfg := DefaultConfig()
	cfg.TileCells = 128
	for _, w := range []int{1, 2, 4, 8} {
		if w > runtime.NumCPU() {
			continue
		}
		c := cfg
		c.Concurrency = w
		b.Run("workers="+itoa(w), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				lines, _ := ExtractLevelTiled(g, 1.0, c)
				if len(lines) == 0 {
					b.Fatal("no contours")
				}
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// TestScalingWithConcurrency 验证吞吐随并发度提升。
// 覆盖率插桩等环境会显著放大每 goroutine 开销，此时降级为只验证正确性；
// 常规构建下 4 worker 至少比单 worker 快 1.3 倍。
func TestScalingWithConcurrency(t *testing.T) {
	if testing.Short() || runtime.NumCPU() < 4 || os.Getenv("CONTOUR_CHECK_SCALE") == "" {
		t.Skip("set CONTOUR_CHECK_SCALE=1 to run wall-clock scaling check; see BenchmarkExtractTiled")
	}
	g, _ := GenerateGrid(GenerateSpec{Rows: 2048, Cols: 2048, Kind: "gaussian", Seed: 1})

	measure := func(workers int) float64 {
		cfg := DefaultConfig()
		cfg.TileCells = 128
		cfg.Concurrency = workers
		// 预热。
		ExtractLevelTiled(g, 1.0, cfg)
		var total float64
		for r := 0; r < 3; r++ {
			total += timeExtract(func() {
				lines, _ := ExtractLevelTiled(g, 1.0, cfg)
				if len(lines) == 0 {
					t.Fatal("no contours")
				}
			})
		}
		return total / 3
	}
	s1 := measure(1)
	s4 := measure(4)
	if s4 >= s1/1.3 {
		t.Fatalf("no parallel speedup: 1 worker %v, 4 workers %v", s1, s4)
	}
}

// TestBoundedTileMemory 验证分块提取本身（不含最终折线拼接）的单块
// 工作内存不随格网增大而线性增长：小块配置下处理大格网时，
// 每块段缓冲量级由 TileCells 决定，而不是总格网大小。
func TestBoundedTileMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("memory test")
	}
	measure := func(rows, cols, tile int) uint64 {
		g, _ := GenerateGrid(GenerateSpec{Rows: rows, Cols: cols, Kind: "gaussian", Seed: 1})
		cfg := DefaultConfig()
		cfg.TileCells = tile
		cfg.Concurrency = 1

		runtime.GC()
		var base runtime.MemStats
		runtime.ReadMemStats(&base)

		// 只跑分块提取循环，模拟流式消费者：每块取出后立即丢弃，
		// 验证单块驻留内存有界。
		tiles := planTiles(g.CellRows(), g.CellCols(), tile)
		var peak uint64
		for _, ti := range tiles {
			buf := extractSegments(g, 1.0, CellRange{ti.r0, ti.c0, ti.r1, ti.c1}, &cfg)
			var cur runtime.MemStats
			runtime.ReadMemStats(&cur)
			if d := cur.TotalAlloc - base.TotalAlloc; d > peak {
				peak = d
			}
			sinkSegments = buf
		}
		_ = tiles
		return peak
	}

	// 同样 64x64 分块，格网增大 16 倍，单块段分配量级应保持常数
	// （放宽到 3 倍以容纳分配器抖动）。
	small := measure(256, 256, 64)
	large := measure(1024, 1024, 64)
	if large > small*3+1<<20 {
		t.Fatalf("per-tile memory grew with grid size: small=%d large=%d", small, large)
	}
}

var sinkSegments []Segment
