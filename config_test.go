package contour

import "testing"

func TestDefaultConfigPolicies(t *testing.T) {
	c := DefaultConfig()
	if c.Undefined != UndefinedBarrier || c.ExactLevel != ExactPerturb || c.Flat != FlatSuppress {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestConfigValidation(t *testing.T) {
	bad := DefaultConfig()
	bad.Undefined = "nope"
	if _, err := bad.normalized(); err == nil {
		t.Fatal("expected invalid Undefined error")
	}
	bad = DefaultConfig()
	bad.ExactLevel = "nope"
	if _, err := bad.normalized(); err == nil {
		t.Fatal("expected invalid ExactLevel error")
	}
	bad = DefaultConfig()
	bad.Flat = "nope"
	if _, err := bad.normalized(); err == nil {
		t.Fatal("expected invalid Flat error")
	}
	bad = DefaultConfig()
	bad.Simplify.Enabled = true
	bad.Simplify.Tolerance = -1
	if _, err := bad.normalized(); err == nil {
		t.Fatal("expected tolerance error")
	}
}

func TestNormalizedLevelsSortedUnique(t *testing.T) {
	c := DefaultConfig()
	c.Levels = []float64{3, 1, 2, 1}
	n, err := c.normalized()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Levels) != 3 || n.Levels[0] != 1 || n.Levels[1] != 2 || n.Levels[2] != 3 {
		t.Fatalf("levels not sorted/unique: %v", n.Levels)
	}
}
