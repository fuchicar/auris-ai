package agent

import (
	"math"
	"strings"
	"testing"
)

// ---- helpers -----------------------------------------------------------------

func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// ---- calcROI -----------------------------------------------------------------

func TestCalcROI_Profit(t *testing.T) {
	r, err := calcROI(100, 120)
	if err != nil {
		t.Fatal(err)
	}
	if r.ROIPercent != 20 {
		t.Errorf("ROIPercent: want 20, got %v", r.ROIPercent)
	}
	if r.ProfitLoss != 20 {
		t.Errorf("ProfitLoss: want 20, got %v", r.ProfitLoss)
	}
	if !strings.Contains(r.Summary, "profit") {
		t.Errorf("summary should mention profit, got %q", r.Summary)
	}
}

func TestCalcROI_Loss(t *testing.T) {
	r, err := calcROI(100, 80)
	if err != nil {
		t.Fatal(err)
	}
	if r.ROIPercent != -20 {
		t.Errorf("ROIPercent: want -20, got %v", r.ROIPercent)
	}
	if r.ProfitLoss != -20 {
		t.Errorf("ProfitLoss: want -20, got %v", r.ProfitLoss)
	}
	if !strings.Contains(r.Summary, "loss") {
		t.Errorf("summary should mention loss, got %q", r.Summary)
	}
}

func TestCalcROI_ZeroCostBasis(t *testing.T) {
	_, err := calcROI(0, 100)
	if err == nil {
		t.Error("expected error for zero cost_basis")
	}
}

// ---- calcCAGR ----------------------------------------------------------------

func TestCalcCAGR_Normal(t *testing.T) {
	// 1000 → 2000 in 5 years ≈ 14.87% CAGR
	r, err := calcCAGR(1000, 2000, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.CAGRPercent, 14.87, 0.01) {
		t.Errorf("CAGRPercent: want ~14.87, got %v", r.CAGRPercent)
	}
	if !strings.Contains(r.Summary, "CAGR") {
		t.Errorf("summary should mention CAGR, got %q", r.Summary)
	}
}

func TestCalcCAGR_ZeroYears(t *testing.T) {
	_, err := calcCAGR(1000, 2000, 0)
	if err == nil {
		t.Error("expected error for years=0")
	}
}

func TestCalcCAGR_NegativeInitial(t *testing.T) {
	_, err := calcCAGR(-500, 2000, 5)
	if err == nil {
		t.Error("expected error for initial_value <= 0")
	}
}

// ---- calcVolatility ----------------------------------------------------------

func TestCalcVolatility_Normal(t *testing.T) {
	// Five prices with variation — just verify no error and annual > daily.
	prices := []float64{100, 102, 98, 105, 103}
	r, err := calcVolatility(prices)
	if err != nil {
		t.Fatal(err)
	}
	if r.VolatilityAnnualPercent <= r.VolatilityDailyPercent {
		t.Errorf("annual vol (%v) should be greater than daily vol (%v)", r.VolatilityAnnualPercent, r.VolatilityDailyPercent)
	}
	if r.VolatilityAnnualPercent <= 0 {
		t.Errorf("expected positive annual volatility, got %v", r.VolatilityAnnualPercent)
	}
}

func TestCalcVolatility_TooFewPrices(t *testing.T) {
	_, err := calcVolatility([]float64{100})
	if err == nil {
		t.Error("expected error for fewer than 2 prices")
	}
}

func TestCalcVolatility_EmptyPrices(t *testing.T) {
	_, err := calcVolatility([]float64{})
	if err == nil {
		t.Error("expected error for empty price slice")
	}
}

// ---- calcSharpe --------------------------------------------------------------

func TestCalcSharpe_Normal(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	r, err := calcSharpe(returns, 0.04)
	if err != nil {
		t.Fatal(err)
	}
	// Positive excess returns → positive Sharpe.
	if r.SharpeRatio <= 0 {
		t.Errorf("expected positive Sharpe ratio, got %v", r.SharpeRatio)
	}
	if r.AnnualizedVolatilityPercent <= 0 {
		t.Errorf("expected positive annualised volatility, got %v", r.AnnualizedVolatilityPercent)
	}
}

func TestCalcSharpe_ZeroVolatility(t *testing.T) {
	// Constant returns → sample std dev = 0 → error.
	returns := []float64{0.001, 0.001, 0.001}
	_, err := calcSharpe(returns, 0.04)
	if err == nil {
		t.Error("expected error for zero volatility")
	}
}

func TestCalcSharpe_EmptyReturns(t *testing.T) {
	_, err := calcSharpe([]float64{}, 0.04)
	if err == nil {
		t.Error("expected error for empty returns")
	}
}

// ---- calcMaxDrawdown ---------------------------------------------------------

func TestCalcMaxDrawdown_Normal(t *testing.T) {
	// Peak at 120, trough at 80 → drawdown = (120-80)/120 * 100 ≈ 33.33%
	prices := []float64{100, 120, 80, 90}
	r, err := calcMaxDrawdown(prices)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.MaxDrawdownPercent, 33.33, 0.01) {
		t.Errorf("MaxDrawdownPercent: want ~33.33, got %v", r.MaxDrawdownPercent)
	}
	if r.PeakPrice != 120 {
		t.Errorf("PeakPrice: want 120, got %v", r.PeakPrice)
	}
	if r.TroughPrice != 80 {
		t.Errorf("TroughPrice: want 80, got %v", r.TroughPrice)
	}
	// Recovery: (120-80)/80 * 100 = 50%
	if !approxEqual(r.RecoveryNeededPercent, 50, 0.01) {
		t.Errorf("RecoveryNeededPercent: want 50, got %v", r.RecoveryNeededPercent)
	}
}

func TestCalcMaxDrawdown_MonotonicallyIncreasing(t *testing.T) {
	r, err := calcMaxDrawdown([]float64{100, 110, 120, 130})
	if err != nil {
		t.Fatal(err)
	}
	if r.MaxDrawdownPercent != 0 {
		t.Errorf("expected 0%% drawdown for monotonically rising prices, got %v", r.MaxDrawdownPercent)
	}
}

func TestCalcMaxDrawdown_TooFewPrices(t *testing.T) {
	_, err := calcMaxDrawdown([]float64{100})
	if err == nil {
		t.Error("expected error for fewer than 2 prices")
	}
}

// ---- calcPnL -----------------------------------------------------------------

func TestCalcPnL_LongProfit(t *testing.T) {
	r, err := calcPnL(100, 110, 10, "long")
	if err != nil {
		t.Fatal(err)
	}
	if r.PnLAbsolute != 100 {
		t.Errorf("PnLAbsolute: want 100, got %v", r.PnLAbsolute)
	}
	if r.PnLPercent != 10 {
		t.Errorf("PnLPercent: want 10, got %v", r.PnLPercent)
	}
	if r.PositionValue != 1100 {
		t.Errorf("PositionValue: want 1100, got %v", r.PositionValue)
	}
	if !strings.Contains(r.Summary, "profit") {
		t.Errorf("summary should mention profit, got %q", r.Summary)
	}
}

func TestCalcPnL_ShortProfit(t *testing.T) {
	r, err := calcPnL(100, 90, 10, "short")
	if err != nil {
		t.Fatal(err)
	}
	if r.PnLAbsolute != 100 {
		t.Errorf("PnLAbsolute: want 100, got %v", r.PnLAbsolute)
	}
	if r.PnLPercent != 10 {
		t.Errorf("PnLPercent: want 10, got %v", r.PnLPercent)
	}
}

func TestCalcPnL_LongLoss(t *testing.T) {
	r, err := calcPnL(100, 90, 10, "long")
	if err != nil {
		t.Fatal(err)
	}
	if r.PnLAbsolute != -100 {
		t.Errorf("PnLAbsolute: want -100, got %v", r.PnLAbsolute)
	}
	if !strings.Contains(r.Summary, "loss") {
		t.Errorf("summary should mention loss, got %q", r.Summary)
	}
}

func TestCalcPnL_InvalidPositionType(t *testing.T) {
	_, err := calcPnL(100, 110, 10, "buy")
	if err == nil {
		t.Error("expected error for invalid position_type")
	}
}

func TestCalcPnL_ZeroEntryPrice(t *testing.T) {
	_, err := calcPnL(0, 110, 10, "long")
	if err == nil {
		t.Error("expected error for zero entry_price")
	}
}

// ---- calcBeta ----------------------------------------------------------------

func TestCalcBeta_Normal(t *testing.T) {
	// Asset that perfectly tracks benchmark → beta = 1, correlation = 1.
	returns := []float64{0.01, -0.02, 0.015, -0.005, 0.02}
	r, err := calcBeta(returns, returns)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.Beta, 1.0, 1e-9) {
		t.Errorf("Beta: want 1.0, got %v", r.Beta)
	}
	if !approxEqual(r.Correlation, 1.0, 1e-9) {
		t.Errorf("Correlation: want 1.0, got %v", r.Correlation)
	}
	if r.Interpretation != "neutral" {
		t.Errorf("Interpretation: want neutral, got %q", r.Interpretation)
	}
}

func TestCalcBeta_Aggressive(t *testing.T) {
	// Asset with 2× benchmark returns → beta = 2, correlation = 1.
	bench := []float64{0.01, -0.02, 0.015, -0.005, 0.02}
	asset := make([]float64, len(bench))
	for i, v := range bench {
		asset[i] = 2 * v
	}
	r, err := calcBeta(asset, bench)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.Beta, 2.0, 1e-9) {
		t.Errorf("Beta: want 2.0, got %v", r.Beta)
	}
	if r.Interpretation != "aggressive" {
		t.Errorf("Interpretation: want aggressive, got %q", r.Interpretation)
	}
}

func TestCalcBeta_UnequalLength(t *testing.T) {
	_, err := calcBeta([]float64{0.01, 0.02}, []float64{0.01})
	if err == nil {
		t.Error("expected error for unequal-length series")
	}
}

func TestCalcBeta_ZeroBenchmarkVariance(t *testing.T) {
	constant := []float64{0.01, 0.01, 0.01}
	_, err := calcBeta([]float64{0.01, 0.02, 0.03}, constant)
	if err == nil {
		t.Error("expected error for zero benchmark variance")
	}
}

// ---- calcVaR -----------------------------------------------------------------

func TestCalcVaR_Parametric(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015, -0.02, 0.008, -0.012, 0.018, -0.006}
	r, err := calcVaR(returns, 0.95, 100000, "parametric")
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "parametric" {
		t.Errorf("Method: want parametric, got %q", r.Method)
	}
	// CVaR must be >= VaR (both expressed as losses, so larger absolute value).
	if r.CVaRAbsolute < r.VaRAbsolute {
		t.Errorf("CVaR (%.2f) should be >= VaR (%.2f)", r.CVaRAbsolute, r.VaRAbsolute)
	}
}

func TestCalcVaR_Historical(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015, -0.02, 0.008, -0.012, 0.018, -0.006}
	r, err := calcVaR(returns, 0.90, 100000, "historical")
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "historical" {
		t.Errorf("Method: want historical, got %q", r.Method)
	}
	if r.CVaRAbsolute < r.VaRAbsolute {
		t.Errorf("CVaR (%.2f) should be >= VaR (%.2f)", r.CVaRAbsolute, r.VaRAbsolute)
	}
}

func TestCalcVaR_EmptyReturns(t *testing.T) {
	_, err := calcVaR([]float64{}, 0.95, 100000, "parametric")
	if err == nil {
		t.Error("expected error for empty returns")
	}
}

func TestCalcVaR_InvalidConfidence(t *testing.T) {
	_, err := calcVaR([]float64{0.01, -0.01}, 1.5, 100000, "parametric")
	if err == nil {
		t.Error("expected error for confidence_level out of range")
	}
}

// ---- calcDCF -----------------------------------------------------------------

func TestCalcDCF_Normal(t *testing.T) {
	// 5 years of FCF = 100 each, discount = 10%, terminal growth = 3%.
	fcf := []float64{100, 100, 100, 100, 100}
	r, err := calcDCF(fcf, 0.10, 0.03, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if r.IntrinsicValueTotal <= 0 {
		t.Errorf("IntrinsicValueTotal should be positive, got %v", r.IntrinsicValueTotal)
	}
	// Terminal value should dominate a standard DCF.
	if r.TerminalValue <= r.PVOfCashflows {
		t.Errorf("TerminalValue (%.2f) should exceed PVOfCashflows (%.2f) for long-horizon assets", r.TerminalValue, r.PVOfCashflows)
	}
	if r.IntrinsicValuePerShare <= 0 {
		t.Errorf("IntrinsicValuePerShare should be positive, got %v", r.IntrinsicValuePerShare)
	}
}

func TestCalcDCF_DiscountRateTooLow(t *testing.T) {
	_, err := calcDCF([]float64{100, 100}, 0.03, 0.05, 1000)
	if err == nil {
		t.Error("expected error when discount_rate <= terminal_growth_rate")
	}
}

func TestCalcDCF_ZeroShares(t *testing.T) {
	r, err := calcDCF([]float64{100, 100}, 0.10, 0.03, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.IntrinsicValuePerShare != 0 {
		t.Errorf("expected 0 per-share value when shares_outstanding=0, got %v", r.IntrinsicValuePerShare)
	}
}

// ---- calcMultiples -----------------------------------------------------------

func TestCalcMultiples_AllValid(t *testing.T) {
	r := calcMultiples(50, 2.5, 20, 1e9, 5e9, 2e9)
	if r.PER == nil || !approxEqual(*r.PER, 20.0, 0.01) {
		t.Errorf("P/E: want ~20, got %v", r.PER)
	}
	if r.PBV == nil || !approxEqual(*r.PBV, 2.5, 0.01) {
		t.Errorf("P/BV: want ~2.5, got %v", r.PBV)
	}
	if r.EVEbitda == nil || !approxEqual(*r.EVEbitda, 5.0, 0.01) {
		t.Errorf("EV/EBITDA: want ~5, got %v", r.EVEbitda)
	}
	if r.EVRevenue == nil {
		t.Error("EV/Revenue should be set")
	}
	if r.PriceToSales == nil {
		t.Error("P/S should be set")
	}
}

func TestCalcMultiples_ZeroDenominators(t *testing.T) {
	// All denominators zero → no multiples computed, no error.
	r := calcMultiples(50, 0, 0, 0, 1e9, 0)
	if r.PER != nil {
		t.Error("P/E should be nil when eps=0")
	}
	if r.PBV != nil {
		t.Error("P/BV should be nil when book_value_per_share=0")
	}
	if r.EVEbitda != nil {
		t.Error("EV/EBITDA should be nil when ebitda=0")
	}
	if r.EVRevenue != nil || r.PriceToSales != nil {
		t.Error("EV/Rev and P/S should be nil when revenue=0")
	}
	if !strings.Contains(r.Summary, "no multiples") {
		t.Errorf("summary should note no multiples, got %q", r.Summary)
	}
}

// ---- calcCurrencyConversion --------------------------------------------------

func TestCalcCurrencyConversion_Normal(t *testing.T) {
	r, err := calcCurrencyConversion(100, "USD", "EUR", 0.92)
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
	_, err := calcCurrencyConversion(100, "USD", "EUR", 0)
	if err == nil {
		t.Error("expected error for zero exchange_rate")
	}
}

func TestCalcCurrencyConversion_NegativeRate(t *testing.T) {
	_, err := calcCurrencyConversion(100, "USD", "EUR", -1.5)
	if err == nil {
		t.Error("expected error for negative exchange_rate")
	}
}

// ---- calcCompoundInterest ----------------------------------------------------

func TestCalcCompoundInterest_Annual(t *testing.T) {
	// 1000 at 10% annual for 1 year, compounded annually → 1100.
	r, err := calcCompoundInterest(1000, 0.10, 1, 1)
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
	r, err := calcCompoundInterest(1000, 0.12, 1, 12)
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
	_, err := calcCompoundInterest(1000, 0.10, 1, 0)
	if err == nil {
		t.Error("expected error for compounds_per_year=0")
	}
}

// ---- calcStats ---------------------------------------------------------------

func TestCalcStats_Normal(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	r, err := calcStats(values, "test")
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
	r, err := calcStats([]float64{42}, "single")
	if err != nil {
		t.Fatal(err)
	}
	if r.Mean != 42 || r.Min != 42 || r.Max != 42 {
		t.Errorf("single value: expected all stats=42, got mean=%v min=%v max=%v", r.Mean, r.Min, r.Max)
	}
}

func TestCalcStats_Empty(t *testing.T) {
	_, err := calcStats([]float64{}, "empty")
	if err == nil {
		t.Error("expected error for empty values")
	}
}

func TestCalcStats_LabelInSummary(t *testing.T) {
	r, err := calcStats([]float64{1, 2, 3}, "my_series")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Summary, "my_series") {
		t.Errorf("summary should contain label, got %q", r.Summary)
	}
}

// ---- probit ------------------------------------------------------------------

func TestProbit_BoundaryZero(t *testing.T) {
	if !math.IsInf(probit(0), -1) {
		t.Errorf("probit(0) should be -Inf")
	}
}

func TestProbit_BoundaryOne(t *testing.T) {
	if !math.IsInf(probit(1), 1) {
		t.Errorf("probit(1) should be +Inf")
	}
}

func TestProbit_Midpoint(t *testing.T) {
	// A&S 26.2.17 has a maximum error of ~4.5e-4; midpoint is near 0 but not exact.
	if !approxEqual(probit(0.5), 0.0, 1e-3) {
		t.Errorf("probit(0.5) should be ~0, got %v", probit(0.5))
	}
}

func TestProbit_KnownValues(t *testing.T) {
	cases := []struct{ p, want float64 }{
		{0.90, 1.2816},
		{0.95, 1.6449},
		{0.99, 2.3263},
	}
	for _, tc := range cases {
		got := probit(tc.p)
		if !approxEqual(got, tc.want, 0.001) {
			t.Errorf("probit(%.2f): want ~%.4f, got %.4f", tc.p, tc.want, got)
		}
	}
}

// ---- percentileInterp --------------------------------------------------------

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
