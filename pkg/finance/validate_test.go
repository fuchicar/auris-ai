package finance

import (
	"math"
	"testing"
)

// ---- ValidateFinite / ValidateFiniteAll ---------------------------------------

func TestValidateFinite_NaN(t *testing.T) {
	if err := ValidateFinite("x", math.NaN()); err == nil {
		t.Error("expected error for NaN")
	}
}

func TestValidateFinite_PositiveInf(t *testing.T) {
	if err := ValidateFinite("x", math.Inf(1)); err == nil {
		t.Error("expected error for +Inf")
	}
}

func TestValidateFinite_NegativeInf(t *testing.T) {
	if err := ValidateFinite("x", math.Inf(-1)); err == nil {
		t.Error("expected error for -Inf")
	}
}

func TestValidateFinite_OrdinaryValues(t *testing.T) {
	for _, v := range []float64{0, 1, -1, 1e300, -1e300} {
		if err := ValidateFinite("x", v); err != nil {
			t.Errorf("ValidateFinite(%v): unexpected error: %v", v, err)
		}
	}
}

func TestValidateFiniteAll_RejectsNaNAtIndex(t *testing.T) {
	err := ValidateFiniteAll("values", []float64{1, 2, math.NaN(), 4})
	if err == nil {
		t.Fatal("expected error for NaN in slice")
	}
}

// ---- ValidatePositive / ValidatePositiveAll -----------------------------------

func TestValidatePositive_RejectsZeroAndNegative(t *testing.T) {
	for _, v := range []float64{0, -1, -0.0001} {
		if err := ValidatePositive("price", v); err == nil {
			t.Errorf("ValidatePositive(%v): expected error", v)
		}
	}
}

func TestValidatePositive_RejectsNaNAndInf(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := ValidatePositive("price", v); err == nil {
			t.Errorf("ValidatePositive(%v): expected error", v)
		}
	}
}

func TestValidatePositive_AcceptsPositive(t *testing.T) {
	if err := ValidatePositive("price", 0.01); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidatePositiveAll_RejectsNonPositiveElement(t *testing.T) {
	if err := ValidatePositiveAll("prices", []float64{100, 105, 0, 110}); err == nil {
		t.Fatal("expected error for zero price in slice")
	}
	if err := ValidatePositiveAll("prices", []float64{100, 105, -5, 110}); err == nil {
		t.Fatal("expected error for negative price in slice")
	}
}

// ---- ValidateNonNegative / ValidateNonNegativeAll -----------------------------

func TestValidateNonNegative_AcceptsZero(t *testing.T) {
	if err := ValidateNonNegative("shares_outstanding", 0); err != nil {
		t.Errorf("unexpected error for zero: %v", err)
	}
}

func TestValidateNonNegative_RejectsNegative(t *testing.T) {
	if err := ValidateNonNegative("shares_outstanding", -1); err == nil {
		t.Error("expected error for negative value")
	}
}

func TestValidateNonNegativeAll_RejectsNegativeElement(t *testing.T) {
	if err := ValidateNonNegativeAll("dividends", []float64{0.5, 0.6, -0.1}); err == nil {
		t.Fatal("expected error for negative element")
	}
}

// ---- ValidateReturn / ValidateReturnSlice -------------------------------------

func TestValidateReturn_Accepts1000PercentGrowth(t *testing.T) {
	// A value multiplying 10x is a legitimate +900% return; the plausibility
	// ceiling must not reject this, nor an even rounder "1000%" (10.0) input.
	if err := ValidateReturn("returns[0]", 9.0); err != nil {
		t.Errorf("a 10x move (900%% return) must be accepted, got error: %v", err)
	}
	if err := ValidateReturn("returns[0]", 10.0); err != nil {
		t.Errorf("a 1000%% return must be accepted, got error: %v", err)
	}
}

func TestValidateReturn_RejectsTotalLossBeyondFloor(t *testing.T) {
	if err := ValidateReturn("returns[0]", -1.5); err == nil {
		t.Error("expected error for a return below -100% (can't lose more than the whole position)")
	}
}

func TestValidateReturn_RejectsImplausibleMagnitude(t *testing.T) {
	if err := ValidateReturn("returns[0]", 1e6); err == nil {
		t.Error("expected error for an obviously corrupted return magnitude")
	}
}

func TestValidateReturn_RejectsNaNAndInf(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := ValidateReturn("returns[0]", v); err == nil {
			t.Errorf("ValidateReturn(%v): expected error", v)
		}
	}
}

func TestValidateReturnSlice_RejectsOffendingElement(t *testing.T) {
	if err := ValidateReturnSlice("returns", []float64{0.01, -0.02, math.NaN(), 0.03}); err == nil {
		t.Fatal("expected error for NaN element")
	}
}

// ---- ValidateReturnPercent / ValidateReturnPercentSlice -----------------------

func TestValidateReturnPercent_AcceptsThousandPercent(t *testing.T) {
	if err := ValidateReturnPercent("shocks_percent[0]", 1000); err != nil {
		t.Errorf("a +1000%% shock must be accepted, got error: %v", err)
	}
}

func TestValidateReturnPercent_RejectsBeyondTotalLoss(t *testing.T) {
	if err := ValidateReturnPercent("shocks_percent[0]", -150); err == nil {
		t.Error("expected error for a shock below -100%")
	}
}

// ---- ValidateRate --------------------------------------------------------------

func TestValidateRate_AcceptsTypicalRates(t *testing.T) {
	for _, v := range []float64{-0.5, 0, 0.04, 1.0, 5.0} {
		if err := ValidateRate("discount_rate", v); err != nil {
			t.Errorf("ValidateRate(%v): unexpected error: %v", v, err)
		}
	}
}

func TestValidateRate_RejectsImplausibleMagnitude(t *testing.T) {
	if err := ValidateRate("discount_rate", 50.0); err == nil {
		t.Error("expected error for a 5,000% annualised rate")
	}
}

func TestValidateRate_RejectsNaNAndInf(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := ValidateRate("discount_rate", v); err == nil {
			t.Errorf("ValidateRate(%v): expected error", v)
		}
	}
}
