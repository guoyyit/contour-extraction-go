package contour

import "testing"

func TestEdgeInterp(t *testing.T) {
	if _, ok := edgeInterp(1, 2, 0.5); ok {
		t.Fatal("both above level must not cross")
	}
	if _, ok := edgeInterp(-2, -1, 0.5); ok {
		t.Fatal("both below level must not cross")
	}
	if tt, ok := edgeInterp(0, -1, 0); !ok || tt != 0 {
		t.Fatalf("level at first endpoint: tt=%v ok=%v", tt, ok)
	}
	if tt, ok := edgeInterp(-1, 0, 0); !ok || tt != 1 {
		t.Fatalf("level at second endpoint: tt=%v ok=%v", tt, ok)
	}
	if tt, ok := edgeInterp(-1, 1, 0); !ok || tt != 0.5 {
		t.Fatalf("mid crossing tt=%v ok=%v", tt, ok)
	}
	if tt, ok := edgeInterp(0, 3, 2); !ok || tt != 2.0/3.0 {
		t.Fatalf("asymmetric crossing tt=%v ok=%v", tt, ok)
	}
}

func TestResolveSaddle(t *testing.T) {
	if got := resolveSaddle([4]float64{1, -1, 1, -1}, 0); got != 0 {
		t.Fatalf("center above level => 0, got %d", got)
	}
	if got := resolveSaddle([4]float64{-1, 1, -1, 0}, 0); got != 1 {
		t.Fatalf("center below level => 1, got %d", got)
	}
	if got := resolveSaddle([4]float64{2, -1, 2, -3}, 0); got != 0 {
		t.Fatalf("center at level => 0, got %d", got)
	}
}
