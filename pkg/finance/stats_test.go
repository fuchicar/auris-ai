package finance

import (
	"strings"
	"testing"
)

func TestCalcStats_Normal(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	r, err := CalcStats(values, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.Mean, 5.5, 0.01) {
		t.Errorf("Mean: want 5.5, got %v", r.Mean)
	}
	if !approxEqual(r.Median, 5.5, 0.01) {
		t.Errorf("Median: want 5.5, got %v", r.Median)
	}
	if r.Min != 1 {
		t.Errorf("Min: want 1, got %v", r.Min)
	}
	if r.Max != 10 {
		t.Errorf("Max: want 10, got %v", r.Max)
	}
	if r.Count != 10 {
		t.Errorf("Count: want 10, got %v", r.Count)
	}
	if r.Percentile25 >= r.Median || r.Percentile75 <= r.Median {
		t.Errorf("quartiles out of order: p25=%.4f, median=%.4f, p75=%.4f", r.Percentile25, r.Median, r.Percentile75)
	}
}

func TestCalcStats_SingleValue(t *testing.T) {
	r, err := CalcStats([]float64{42}, "single")
	if err != nil {
		t.Fatal(err)
	}
	if r.Mean != 42 || r.Min != 42 || r.Max != 42 {
		t.Errorf("single value: expected all stats=42, got mean=%v min=%v max=%v", r.Mean, r.Min, r.Max)
	}
}

func TestCalcStats_Empty(t *testing.T) {
	_, err := CalcStats([]float64{}, "empty")
	if err == nil {
		t.Error("expected error for empty values")
	}
}

func TestCalcStats_LabelInSummary(t *testing.T) {
	r, err := CalcStats([]float64{1, 2, 3}, "my_series")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Summary, "my_series") {
		t.Errorf("summary should contain label, got %q", r.Summary)
	}
}

// ---- probit ------------------------------------------------------------------

func TestPercentileInterp_AtOne(t *testing.T) {
	// p=1.0 → lo = n-1, hi = n → boundary branch → return last element.
	s := []float64{10, 20, 30}
	got := percentileInterp(s, 1.0)
	if got != 30 {
		t.Errorf("percentileInterp at p=1: want 30, got %v", got)
	}
}

func TestPercentileInterp_SingleElement(t *testing.T) {
	got := percentileInterp([]float64{42}, 0.5)
	if got != 42 {
		t.Errorf("single-element slice: want 42, got %v", got)
	}
}

// ---- regression tests: bugs --------------------------------------------------
// Each test encodes the exact condition that the bug allowed to slip through.
// They are named TestRegression_BUGN_* so they can be run in isolation:
//   go test ./pkg/agent/... -run TestRegression

// BUG-2: CalcStats returned Min/Max as raw float64 (unrounded) while every
// other numeric field in StatsResult used Round4.
func TestRegression_BUG2_MinMaxRoundedToFourDecimals(t *testing.T) {
	// Values with more than 4 significant decimal places so rounding is observable.
	values := []float64{1.123456789, 5.0, 9.987654321}
	r, err := CalcStats(values, "test")
	if err != nil {
		t.Fatal(err)
	}
	wantMin := Round4(1.123456789) // 1.1235
	wantMax := Round4(9.987654321) // 9.9877
	if r.Min != wantMin {
		t.Errorf("BUG-2 regression: Min not rounded: want %v, got %v", wantMin, r.Min)
	}
	if r.Max != wantMax {
		t.Errorf("BUG-2 regression: Max not rounded: want %v, got %v", wantMax, r.Max)
	}
	// Confirm the raw values differ from the expected rounded ones, so this test
	// would have failed against the old (unrounded) code.
	if wantMin == 1.123456789 || wantMax == 9.987654321 {
		t.Error("test setup error: chosen values do not exercise rounding")
	}
}
