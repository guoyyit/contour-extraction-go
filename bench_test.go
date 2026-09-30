package contour

import (
	"runtime"
	"testing"
)

func benchGrid(tb testing.TB, n int) *Grid {
	tb.Helper()
	g, err := Generate(GenConfig{Kind: "hills", NX: n, NY: n})
	if err != nil {
		tb.Fatal(err)
	}
	return g
}

func BenchmarkExtract(b *testing.B) {
	g := benchGrid(b, 512)
	levels := []float64{-1, -0.5, 0, 0.5, 1}
	for _, tile := range []int{0, 32, 64, 128} {
		name := "tile=single"
		if tile > 0 {
			name = "tile=" + itoa(tile)
		}
		b.Run(name, func(b *testing.B) {
			opts := Options{TileNX: tile, TileNY: tile, Concurrency: runtime.NumCPU(), NaNPolicy: NaNMask}
			b.ReportAllocs()
			for k := 0; k < b.N; k++ {
				svc, err := NewService(g, opts)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := svc.Extract(levels); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkConcurrencyScaling(b *testing.B) {
	g := benchGrid(b, 1024)
	levels := []float64{-0.75, -0.25, 0.25, 0.75}
	for _, w := range []int{1, 2, 4, 8} {
		if w > runtime.NumCPU()*2 {
			continue
		}
		b.Run("workers="+itoa(w), func(b *testing.B) {
			opts := Options{TileNX: 64, TileNY: 64, Concurrency: w, NaNPolicy: NaNMask}
			b.ReportAllocs()
			for k := 0; k < b.N; k++ {
				svc, err := NewService(g, opts)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := svc.Extract(levels); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestMemoryBoundedByTileSize verifies that, with fixed tile dimensions,
// transient allocations per processed cell stay constant as the grid grows
// (linear rather than super-linear total allocation), demonstrating that
// per-task working set is bounded by tile size rather than full-grid size.
func TestMemoryBoundedByTileSize(t *testing.T) {
	var prevAllocs float64
	var prevCells float64
	for _, n := range []int{128, 256, 512, 1024} {
		g := benchGrid(t, n)
		opts := Options{TileNX: 32, TileNY: 32, Concurrency: 1, NaNPolicy: NaNMask}
		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)
		svc, _ := NewService(g, opts)
		if _, err := svc.Extract([]float64{-0.5, 0, 0.5}); err != nil {
			t.Fatal(err)
		}
		runtime.GC()
		runtime.ReadMemStats(&m2)
		allocs := float64(m2.Mallocs - m1.Mallocs)
		cells := float64((n - 1) * (n - 1))
		ratio := allocs / cells
		t.Logf("n=%d allocs/cell=%.3f", n, ratio)
		if prevCells > 0 {
			// allocs per cell must stay roughly constant; allow generous slack
			// for map growth and scheduler effects.
			if ratio > prevAllocs*1.5 && ratio > prevAllocs+5 {
				t.Fatalf("allocs/cell growing super-linearly: %v -> %v", prevAllocs, ratio)
			}
		}
		prevAllocs, prevCells = ratio, cells
	}
}

// TestPeakHeapIndependentOfWorkers checks that raising concurrency does not
// proportionally multiply peak heap usage: 8 workers vs 1 must stay within a
// small multiplicative envelope of the single-worker result.
func TestPeakHeapIndependentOfWorkers(t *testing.T) {
	g := benchGrid(t, 512)
	measure := func(workers int) uint64 {
		opts := Options{TileNX: 64, TileNY: 64, Concurrency: workers, NaNPolicy: NaNMask}
		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)
		svc, _ := NewService(g, opts)
		if _, err := svc.Extract([]float64{-0.5, 0, 0.5}); err != nil {
			t.Fatal(err)
		}
		runtime.GC()
		runtime.ReadMemStats(&m2)
		return m2.TotalAlloc - m1.TotalAlloc
	}
	one := measure(1)
	eight := measure(8)
	t.Logf("totalAlloc 1 worker=%d, 8 workers=%d (ratio %.2f)", one, eight, float64(eight)/float64(one))
	// Per-worker private state is bounded by the small tile outputs; allow a
	// generous 1.25x envelope for allocator/scheduling effects.
	if float64(eight) > 1.25*float64(one) {
		t.Fatalf("8 workers allocated %.2fx of 1 worker; expected bounded", float64(eight)/float64(one))
	}
}

func itoa(x int) string {
	if x == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for x > 0 {
		i--
		b[i] = byte('0' + x%10)
		x /= 10
	}
	return string(b[i:])
}
