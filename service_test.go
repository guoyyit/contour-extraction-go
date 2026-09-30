package contour

import (
	"reflect"
	"testing"
)

func TestIncrementalEqualsBatch(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 30, Cols: 34, Kind: "gaussian", Seed: 17, NoDataPct: 0.03})
	levels := []float64{2.0, 0.5, 1.5}

	// 一次性全量。
	batch := NewService(g, DefaultConfig())
	batch.AddLevels(levels...)

	// 乱序、逐个增量加入，且重复加入已有级别。
	incr := NewService(g, DefaultConfig())
	incr.AddLevels(2.0)
	incr.AddLevels(0.5)
	incr.AddLevels(0.5) // 重复，不应重算
	incr.AddLevels(1.5)

	if !reflect.DeepEqual(incr.Levels(), []float64{0.5, 1.5, 2.0}) {
		t.Fatalf("levels: %v", incr.Levels())
	}
	if !reflect.DeepEqual(incr.All(), batch.All()) {
		t.Fatal("incremental result differs from batch extraction")
	}

	// 追加新级别后，原有级别的结果必须不变。
	before := incr.Contours(1.5)
	incr.AddLevels(2.75)
	after := incr.Contours(1.5)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("existing level recomputed after adding a new level")
	}
	// 新级别结果与单独一次性提取一致。
	single := ExtractLevel(g, 2.75, DefaultConfig())
	if !reflect.DeepEqual(incr.Contours(2.75), single) {
		t.Fatal("new level differs from standalone extraction")
	}
}

func TestServiceDeterminism(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 24, Cols: 28, Kind: "gaussian", Seed: 100, NoDataPct: 0.06})
	cfg := DefaultConfig()
	cfg.Concurrency = 2
	cfg.TileCells = 5

	a := NewService(g, cfg)
	a.AddLevels(0.3, 1.1, 2.2)

	// 不同并发度/分块也必须得到同一结果。
	cfg2 := DefaultConfig()
	cfg2.Concurrency = 8
	cfg2.TileCells = 13
	b := NewService(g, cfg2)
	b.AddLevels(2.2, 0.3, 1.1)

	if !reflect.DeepEqual(a.Snapshot().Lines, b.Snapshot().Lines) {
		t.Fatal("results differ across concurrency/tile settings or level order")
	}

	// 快照可复现：重建服务并比较。
	c := NewService(g, DefaultConfig())
	c.AddLevels(a.Levels()...)
	if !reflect.DeepEqual(a.Snapshot().Lines, c.Snapshot().Lines) {
		t.Fatal("snapshot not reproducible")
	}
}

func TestAddLevelsReturnsOnlyNew(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 10, Cols: 10, Kind: "plane", Seed: 1})
	s := NewService(g, DefaultConfig())
	added1 := s.AddLevels(1, 2, 2, 3)
	if !reflect.DeepEqual(added1, []float64{1, 2, 3}) {
		t.Fatalf("first add: %v", added1)
	}
	added2 := s.AddLevels(2, 4)
	if !reflect.DeepEqual(added2, []float64{4}) {
		t.Fatalf("second add: %v", added2)
	}
}
