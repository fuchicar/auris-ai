package finance

import (
	"math"
	"testing"
)

func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestSMA_HappyPath(t *testing.T) {
	values := []float64{10, 11, 12, 13, 14}
	got, err := SMA(values, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(values) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(values))
	}
	if !math.IsNaN(got[0]) || !math.IsNaN(got[1]) {
		t.Errorf("warm-up positions must be NaN, got %v", got[:2])
	}
	want := []float64{math.NaN(), math.NaN(), 11, 12, 13}
	for i, w := range want {
		if math.IsNaN(w) {
			if !math.IsNaN(got[i]) {
				t.Errorf("got[%d]: want NaN, got %v", i, got[i])
			}
			continue
		}
		if !approxEqual(got[i], w, 1e-9) {
			t.Errorf("got[%d]: want %v, got %v", i, w, got[i])
		}
	}
}

func TestSMA_ZeroPeriod(t *testing.T) {
	if _, err := SMA([]float64{1, 2, 3}, 0); err == nil {
		t.Error("expected error for zero period")
	}
}

func TestSMA_NegativePeriod(t *testing.T) {
	if _, err := SMA([]float64{1, 2, 3}, -1); err == nil {
		t.Error("expected error for negative period")
	}
}

func TestSMA_InsufficientValues(t *testing.T) {
	if _, err := SMA([]float64{1, 2}, 5); err == nil {
		t.Error("expected error when len(values) < period")
	}
}
