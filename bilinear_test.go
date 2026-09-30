package contour

import (
	"math"
	"testing"
)

func TestBilinearFullyDefined(t *testing.T) {
	// 2x2 单元：四个角点 0,10,20,30。
	g, _ := NewGrid(2, 2, []float64{0, 10, 20, 30})
	cases := []struct {
		x, y, want float64
	}{
		{0, 0, 0}, {1, 0, 10}, {0, 1, 20}, {1, 1, 30}, {0.5, 0.5, 15},
	}
	for _, tc := range cases {
		got, ok := bilinearAt(g, tc.x, tc.y)
		if !ok || math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("(%v,%v)=%v ok=%v want %v", tc.x, tc.y, got, ok, tc.want)
		}
	}
}

func TestBilinearPartiallyUndefined(t *testing.T) {
	nan := math.NaN()
	// 左上未定义；右上=10，左下=20，右下=30。
	g, _ := NewGrid(2, 2, []float64{nan, 10, 20, 30})

	// 三角片1（TL-TR-BR）含未定义角点：拒绝。
	if _, ok := bilinearAt(g, 0.75, 0.5); ok {
		t.Fatal("point in undefined triangle should be rejected")
	}
	// 三角片2（TL-BR-BL）也含未定义 TL：拒绝。
	if _, ok := bilinearAt(g, 0.25, 0.5); ok {
		t.Fatal("point in triangle sharing undefined TL should be rejected")
	}
}

func TestBilinearUndefinedInOtherTriangle(t *testing.T) {
	nan := math.NaN()
	// 仅左下未定义；左上=0，右上=10，右下=30。三角片1 全部定义。
	g, _ := NewGrid(2, 2, []float64{0, 10, nan, 30})
	// 点位于三角片1 (tx+ty >= 1)：应按三角线性插值。
	got, ok := bilinearAt(g, 0.75, 0.5)
	if !ok {
		t.Fatal("point in fully-defined triangle 1 rejected")
	}
	// 权重 TL=0.25, TR=0.25, BR=0.5 => 0*.25 + 10*.25 + 30*.5 = 17.5
	if math.Abs(got-17.5) > 1e-9 {
		t.Fatalf("tri1 interpolation = %v want 17.5", got)
	}
	// 点位于三角片2（含未定义 BL）：拒绝。
	if _, ok := bilinearAt(g, 0.25, 0.5); ok {
		t.Fatal("point in triangle 2 with undefined BL accepted")
	}
}

func TestBilinearOutsideDomain(t *testing.T) {
	g, _ := NewGrid(2, 2, []float64{0, 1, 2, 3})
	if _, ok := bilinearAt(g, -0.1, 0); ok {
		t.Fatal("outside point accepted")
	}
	if _, ok := bilinearAt(g, 1.1, 0); ok {
		t.Fatal("outside point accepted")
	}
}
