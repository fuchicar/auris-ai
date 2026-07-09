package finance

import (
	"math"
	"strings"
	"testing"
)

func TestCalcStressTest_Normal(t *testing.T) {
	r, err := CalcStressTest(10000, []float64{-10, -20, -30, -40}, "portfolio")
	if err != nil {
		t.Fatal(err)
	}
	if r.CurrentValue != 10000 {
		t.Errorf("CurrentValue: want 10000, got %v", r.CurrentValue)
	}
	if len(r.Scenarios) != 4 {
		t.Fatalf("Scenarios: want 4, got %d", len(r.Scenarios))
	}
	// -10% → 9000
	if !approxEqual(r.Scenarios[0].ResultingValue, 9000, 0.01) {
		t.Errorf("Scenarios[0].ResultingValue: want 9000, got %v", r.Scenarios[0].ResultingValue)
	}
	if !approxEqual(r.Scenarios[0].ChangeAbsolute, -1000, 0.01) {
		t.Errorf("Scenarios[0].ChangeAbsolute: want -1000, got %v", r.Scenarios[0].ChangeAbsolute)
	}
	// -40% → 6000 must be the worst case.
	if !approxEqual(r.WorstCase.ResultingValue, 6000, 0.01) {
		t.Errorf("WorstCase.ResultingValue: want 6000, got %v", r.WorstCase.ResultingValue)
	}
	if r.WorstCase.ShockPercent != -40 {
		t.Errorf("WorstCase.ShockPercent: want -40, got %v", r.WorstCase.ShockPercent)
	}
	if !strings.Contains(r.Summary, "portfolio") {
		t.Errorf("summary should mention the label, got %q", r.Summary)
	}
}

func TestCalcStressTest_DefaultLabel(t *testing.T) {
	r, err := CalcStressTest(100, []float64{-5}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Summary, "value") {
		t.Errorf("summary should fall back to a generic label, got %q", r.Summary)
	}
}

func TestCalcStressTest_PositiveShock(t *testing.T) {
	r, err := CalcStressTest(100, []float64{10}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.Scenarios[0].ResultingValue, 110, 0.01) {
		t.Errorf("ResultingValue: want 110, got %v", r.Scenarios[0].ResultingValue)
	}
}

func TestCalcStressTest_ZeroCurrentValue(t *testing.T) {
	_, err := CalcStressTest(0, []float64{-10}, "")
	if err == nil {
		t.Error("expected error for current_value <= 0")
	}
}

func TestCalcStressTest_EmptyShocks(t *testing.T) {
	_, err := CalcStressTest(100, nil, "")
	if err == nil {
		t.Error("expected error for empty shocks_percent")
	}
}

// ---- CalcMonteCarloSimulation ------------------------------------------------

func TestCalcMonteCarloSimulation_ZeroDriftAndVolatility(t *testing.T) {
	// With no drift and no volatility, every path is flat: all final prices
	// equal last_price, so mean/P5/P50/P95 collapse to it and nothing is
	// ever strictly above the start.
	r, err := CalcMonteCarloSimulation(100, 0, 0, 30, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.MeanFinalPrice, 100, 0.001) {
		t.Errorf("MeanFinalPrice: want 100, got %v", r.MeanFinalPrice)
	}
	if !approxEqual(r.MedianFinalPrice, 100, 0.001) {
		t.Errorf("MedianFinalPrice: want 100, got %v", r.MedianFinalPrice)
	}
	if !approxEqual(r.P5FinalPrice, 100, 0.001) || !approxEqual(r.P95FinalPrice, 100, 0.001) {
		t.Errorf("P5/P95: want 100/100, got %v/%v", r.P5FinalPrice, r.P95FinalPrice)
	}
	if r.ProbAbovePercent != 0 {
		t.Errorf("ProbAbovePercent: want 0, got %v", r.ProbAbovePercent)
	}
	if r.Days != 30 || r.NumSimulations != 1000 {
		t.Errorf("Days/NumSimulations should echo input, got %d/%d", r.Days, r.NumSimulations)
	}
}

func TestCalcMonteCarloSimulation_PositiveDrift_MeanTracksExpectedGrowth(t *testing.T) {
	// Large sample to keep the test stable without a fixed seed. Expected
	// mean of GBM's terminal price is last_price * exp(drift_annual * T).
	const lastPrice, driftAnnual, volAnnual = 100.0, 0.20, 0.10
	const days = 252
	r, err := CalcMonteCarloSimulation(lastPrice, driftAnnual, volAnnual, days, 50000)
	if err != nil {
		t.Fatal(err)
	}
	want := lastPrice * math.Exp(driftAnnual*float64(days)/252.0)
	// Generous tolerance: this is a statistical check, not exact arithmetic.
	if !approxEqual(r.MeanFinalPrice, want, want*0.05) {
		t.Errorf("MeanFinalPrice: want ~%.2f (±5%%), got %v", want, r.MeanFinalPrice)
	}
	if r.ProbAbovePercent <= 50 {
		t.Errorf("positive drift should push most paths above the start, got ProbAbovePercent=%v", r.ProbAbovePercent)
	}
}

func TestCalcMonteCarloSimulation_PercentileOrdering(t *testing.T) {
	r, err := CalcMonteCarloSimulation(100, 0.05, 0.25, 90, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if !(r.P5FinalPrice <= r.MedianFinalPrice && r.MedianFinalPrice <= r.P95FinalPrice) {
		t.Errorf("expected P5 <= P50 <= P95, got %v <= %v <= %v", r.P5FinalPrice, r.MedianFinalPrice, r.P95FinalPrice)
	}
}

func TestCalcMonteCarloSimulation_InvalidLastPrice(t *testing.T) {
	if _, err := CalcMonteCarloSimulation(0, 0.05, 0.2, 30, 100); err == nil {
		t.Error("expected error for last_price <= 0")
	}
}

func TestCalcMonteCarloSimulation_InvalidDays(t *testing.T) {
	if _, err := CalcMonteCarloSimulation(100, 0.05, 0.2, 0, 100); err == nil {
		t.Error("expected error for days <= 0")
	}
}

func TestCalcMonteCarloSimulation_InvalidNumSimulations(t *testing.T) {
	if _, err := CalcMonteCarloSimulation(100, 0.05, 0.2, 30, 0); err == nil {
		t.Error("expected error for num_simulations <= 0")
	}
}

func TestCalcMonteCarloSimulation_NumSimulationsOverCap(t *testing.T) {
	if _, err := CalcMonteCarloSimulation(100, 0.05, 0.2, 30, maxMonteCarloSimulations+1); err == nil {
		t.Error("expected error for num_simulations over the cap")
	}
}

func TestCalcMonteCarloSimulation_NegativeVolatility(t *testing.T) {
	if _, err := CalcMonteCarloSimulation(100, 0.05, -0.1, 30, 100); err == nil {
		t.Error("expected error for negative volatility_annual")
	}
}

// ---- CalcCurrencyConversion --------------------------------------------------

func TestCalcCurrencyConversion_Normal(t *testing.T) {
	r, err := CalcCurrencyConversion(100, "USD", "EUR", 0.92)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.ConvertedAmount, 92.0, 0.01) {
		t.Errorf("ConvertedAmount: want 92, got %v", r.ConvertedAmount)
	}
	if r.RateUsed != 0.92 {
		t.Errorf("RateUsed: want 0.92, got %v", r.RateUsed)
	}
	if !strings.Contains(r.Summary, "USD") || !strings.Contains(r.Summary, "EUR") {
		t.Errorf("summary should mention currencies, got %q", r.Summary)
	}
}

func TestCalcCurrencyConversion_ZeroRate(t *testing.T) {
	_, err := CalcCurrencyConversion(100, "USD", "EUR", 0)
	if err == nil {
		t.Error("expected error for zero exchange_rate")
	}
}

func TestCalcCurrencyConversion_NegativeRate(t *testing.T) {
	_, err := CalcCurrencyConversion(100, "USD", "EUR", -1.5)
	if err == nil {
		t.Error("expected error for negative exchange_rate")
	}
}

// ---- CalcCompoundInterest ----------------------------------------------------

func TestCalcCompoundInterest_Annual(t *testing.T) {
	// 1000 at 10% annual for 1 year, compounded annually → 1100.
	r, err := CalcCompoundInterest(1000, 0.10, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.FinalAmount, 1100, 0.01) {
		t.Errorf("FinalAmount: want 1100, got %v", r.FinalAmount)
	}
	if !approxEqual(r.TotalInterest, 100, 0.01) {
		t.Errorf("TotalInterest: want 100, got %v", r.TotalInterest)
	}
	// EAR equals nominal rate when compounding is annual.
	if !approxEqual(r.EffectiveAnnualRate, 10.0, 0.01) {
		t.Errorf("EffectiveAnnualRate: want 10.00, got %v", r.EffectiveAnnualRate)
	}
}

func TestCalcCompoundInterest_Monthly(t *testing.T) {
	// Monthly compounding produces higher EAR than nominal rate.
	r, err := CalcCompoundInterest(1000, 0.12, 1, 12)
	if err != nil {
		t.Fatal(err)
	}
	// EAR for 12% monthly = (1 + 0.01)^12 - 1 ≈ 12.6825%.
	if !approxEqual(r.EffectiveAnnualRate, 12.6825, 0.01) {
		t.Errorf("EffectiveAnnualRate: want ~12.68, got %v", r.EffectiveAnnualRate)
	}
	if r.FinalAmount <= 1120 {
		t.Errorf("monthly compounding should produce more than annual; got %v", r.FinalAmount)
	}
}

func TestCalcCompoundInterest_ZeroCompounds(t *testing.T) {
	_, err := CalcCompoundInterest(1000, 0.10, 1, 0)
	if err == nil {
		t.Error("expected error for compounds_per_year=0")
	}
}

// ---- CalcStats ---------------------------------------------------------------

func TestRegression_REF3_CalcCompoundInterest_RejectsNaNPrincipalOrRate(t *testing.T) {
	if _, err := CalcCompoundInterest(math.NaN(), 0.05, 5, 1); err == nil {
		t.Error("expected error for NaN principal")
	}
	if _, err := CalcCompoundInterest(1000, math.NaN(), 5, 1); err == nil {
		t.Error("expected error for NaN annual_rate")
	}
}

func TestRegression_REF3_CalcCompoundInterest_RejectsImplausibleRate(t *testing.T) {
	if _, err := CalcCompoundInterest(1000, 50.0, 5, 1); err == nil {
		t.Error("expected error for a 5,000% annual_rate")
	}
}

func TestRegression_REF3_CalcCompoundInterest_AcceptsHighButPlausibleRate(t *testing.T) {
	if _, err := CalcCompoundInterest(1000, 2.0, 1, 1); err != nil {
		t.Errorf("a 200%% annual_rate must be accepted, got error: %v", err)
	}
}

func TestRegression_REF3_CalcStressTest_Accepts1000PercentShock(t *testing.T) {
	if _, err := CalcStressTest(10000, []float64{-20, 1000}, "portfolio"); err != nil {
		t.Errorf("a +1000%% shock must be accepted, got error: %v", err)
	}
}

func TestRegression_REF3_CalcStressTest_RejectsShockBelowTotalLoss(t *testing.T) {
	if _, err := CalcStressTest(10000, []float64{-150}, "portfolio"); err == nil {
		t.Error("expected error for a shock below -100%")
	}
}
