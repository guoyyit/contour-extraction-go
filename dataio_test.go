package contour

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGridFileRoundTrip(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 6, Cols: 7, Kind: "gaussian", Seed: 6, NoDataPct: 0.1})
	dir := t.TempDir()
	p := filepath.Join(dir, "grid.txt")
	if err := WriteGridFile(p, g); err != nil {
		t.Fatal(err)
	}
	h, err := ReadGridFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if h.rows != g.rows || h.cols != g.cols {
		t.Fatal("dims mismatch")
	}
	for i := range g.values {
		a, b := g.values[i], h.values[i]
		if math.IsNaN(a) != math.IsNaN(b) || (!math.IsNaN(a) && a != b) {
			t.Fatalf("value %d mismatch: %v vs %v", i, a, b)
		}
	}
}

func TestReadGridFileErrors(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"bad_header": "2\n0 0\n1 1\n",
		"short_row":  "2 2\n0 0\n1\n",
		"bad_number": "2 2\n0 x\n1 1\n",
		"small_dims": "1 2\n0 0\n",
	}
	for name, body := range cases {
		p := filepath.Join(dir, name+".txt")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadGridFile(p); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestContourJSONRoundTrip(t *testing.T) {
	g, _ := GenerateGrid(GenerateSpec{Rows: 20, Cols: 22, Kind: "gaussian", Seed: 9})
	svc := NewService(g, DefaultConfig())
	svc.AddLevels(0.5, 1.5)
	snap := svc.Snapshot()

	dir := t.TempDir()
	p := filepath.Join(dir, "contours.json")
	if err := WriteContoursJSON(p, snap); err != nil {
		t.Fatal(err)
	}
	got, err := ReadContoursJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Levels, snap.Levels) {
		t.Fatalf("levels mismatch: %v vs %v", got.Levels, snap.Levels)
	}
	if !reflect.DeepEqual(got.Lines, snap.Lines) {
		t.Fatal("lines mismatch after JSON round trip")
	}
}
