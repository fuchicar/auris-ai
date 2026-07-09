package finance

import (
	"math"
	"strings"
	"testing"
)

func TestCalcVolatility_Normal(t *testing.T) {
	// Five prices with variation — just verify no error and annual > daily.
	prices := []float64{100, 102, 98, 105, 103}
	r, err := CalcVolatility(prices)
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
	_, err := CalcVolatility([]float64{100})
	if err == nil {
		t.Error("expected error for fewer than 2 prices")
	}
}

func TestCalcVolatility_EmptyPrices(t *testing.T) {
	_, err := CalcVolatility([]float64{})
	if err == nil {
		t.Error("expected error for empty price slice")
	}
}

// ---- CalcSharpe --------------------------------------------------------------

func TestCalcSharpe_Normal(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	r, err := CalcSharpe(returns, 0.04)
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
	_, err := CalcSharpe(returns, 0.04)
	if err == nil {
		t.Error("expected error for zero volatility")
	}
}

func TestCalcSharpe_EmptyReturns(t *testing.T) {
	_, err := CalcSharpe([]float64{}, 0.04)
	if err == nil {
		t.Error("expected error for empty returns")
	}
}

// ---- CalcSortino --------------------------------------------------------------

func TestCalcSortino_Normal(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	r, err := CalcSortino(returns, 0.04)
	if err != nil {
		t.Fatal(err)
	}
	// Positive excess returns over a low risk-free rate → positive Sortino.
	if r.SortinoRatio <= 0 {
		t.Errorf("expected positive Sortino ratio, got %v", r.SortinoRatio)
	}
	if r.AnnualizedDownsideDeviationPercent <= 0 {
		t.Errorf("expected positive annualised downside deviation, got %v", r.AnnualizedDownsideDeviationPercent)
	}
}

func TestCalcSortino_ZeroDownsideDeviation(t *testing.T) {
	// Every daily return is well above the daily risk-free rate implied by a
	// 1% annual rate, so no observation falls below the MAR → downside
	// deviation is zero and the ratio is undefined.
	returns := []float64{0.05, 0.06, 0.055}
	_, err := CalcSortino(returns, 0.01)
	if err == nil {
		t.Error("expected error for zero downside deviation")
	}
}

func TestCalcSortino_EmptyReturns(t *testing.T) {
	_, err := CalcSortino([]float64{}, 0.04)
	if err == nil {
		t.Error("expected error for empty returns")
	}
}

// ---- CalcMaxDrawdown ---------------------------------------------------------

func TestCalcMaxDrawdown_Normal(t *testing.T) {
	// Peak at 120, trough at 80 → drawdown = (120-80)/120 * 100 ≈ 33.33%
	prices := []float64{100, 120, 80, 90}
	r, err := CalcMaxDrawdown(prices)
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
	r, err := CalcMaxDrawdown([]float64{100, 110, 120, 130})
	if err != nil {
		t.Fatal(err)
	}
	if r.MaxDrawdownPercent != 0 {
		t.Errorf("expected 0%% drawdown for monotonically rising prices, got %v", r.MaxDrawdownPercent)
	}
}

func TestCalcMaxDrawdown_TooFewPrices(t *testing.T) {
	_, err := CalcMaxDrawdown([]float64{100})
	if err == nil {
		t.Error("expected error for fewer than 2 prices")
	}
}

// ---- CalcPnL -----------------------------------------------------------------

func TestCalcBeta_Normal(t *testing.T) {
	// Asset that perfectly tracks benchmark → beta = 1, correlation = 1.
	returns := []float64{0.01, -0.02, 0.015, -0.005, 0.02}
	r, err := CalcBeta(returns, returns)
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
	r, err := CalcBeta(asset, bench)
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
	_, err := CalcBeta([]float64{0.01, 0.02}, []float64{0.01})
	if err == nil {
		t.Error("expected error for unequal-length series")
	}
}

func TestCalcBeta_ZeroBenchmarkVariance(t *testing.T) {
	constant := []float64{0.01, 0.01, 0.01}
	_, err := CalcBeta([]float64{0.01, 0.02, 0.03}, constant)
	if err == nil {
		t.Error("expected error for zero benchmark variance")
	}
}

// ---- CalcTreynor --------------------------------------------------------------

func TestCalcTreynor_Normal(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	r, err := CalcTreynor(returns, 0.04, 1.2)
	if err != nil {
		t.Fatal(err)
	}
	if r.Beta != 1.2 {
		t.Errorf("Beta: want echoed input 1.2, got %v", r.Beta)
	}
	if r.AnnualizedReturnPercent == 0 {
		t.Error("expected non-zero annualised return")
	}
}

func TestCalcTreynor_NegativeBeta(t *testing.T) {
	// A negative beta (inverse-correlated asset) must not be rejected — it
	// simply flips the sign of the ratio relative to the same returns with a
	// positive beta.
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	positive, err := CalcTreynor(returns, 0.04, 1.5)
	if err != nil {
		t.Fatal(err)
	}
	negative, err := CalcTreynor(returns, 0.04, -1.5)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(positive.TreynorRatioPercent, -negative.TreynorRatioPercent, 1e-9) {
		t.Errorf("expected sign flip: positive=%v negative=%v", positive.TreynorRatioPercent, negative.TreynorRatioPercent)
	}
}

func TestCalcTreynor_ZeroBeta(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	_, err := CalcTreynor(returns, 0.04, 0)
	if err == nil {
		t.Error("expected error for zero beta")
	}
}

func TestCalcTreynor_EmptyReturns(t *testing.T) {
	_, err := CalcTreynor([]float64{}, 0.04, 1.0)
	if err == nil {
		t.Error("expected error for empty returns")
	}
}

// ---- CalcInformationRatio ------------------------------------------------------

func TestCalcInformationRatio_Normal(t *testing.T) {
	asset := []float64{0.02, 0.01, 0.03, 0.015, 0.025}
	benchmark := []float64{0.01, 0.005, 0.015, 0.01, 0.012}
	r, err := CalcInformationRatio(asset, benchmark)
	if err != nil {
		t.Fatal(err)
	}
	// Asset consistently outperforms benchmark → positive information ratio.
	if r.InformationRatio <= 0 {
		t.Errorf("expected positive information ratio, got %v", r.InformationRatio)
	}
	if r.AnnualizedTrackingErrorPercent <= 0 {
		t.Errorf("expected positive tracking error, got %v", r.AnnualizedTrackingErrorPercent)
	}
}

func TestCalcInformationRatio_ZeroTrackingError(t *testing.T) {
	// asset_returns identical to benchmark_returns → the difference series is
	// all zero → tracking error is zero → undefined.
	returns := []float64{0.01, 0.02, -0.01, 0.015}
	_, err := CalcInformationRatio(returns, returns)
	if err == nil {
		t.Error("expected error for zero tracking error")
	}
}

func TestCalcInformationRatio_UnequalLength(t *testing.T) {
	_, err := CalcInformationRatio([]float64{0.01, 0.02}, []float64{0.01})
	if err == nil {
		t.Error("expected error for unequal length series")
	}
}

func TestCalcInformationRatio_TooFewObservations(t *testing.T) {
	_, err := CalcInformationRatio([]float64{0.01}, []float64{0.02})
	if err == nil {
		t.Error("expected error for fewer than 2 observations")
	}
}

// ---- CalcVaR -----------------------------------------------------------------

func TestCalcVaR_Parametric(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015, -0.02, 0.008, -0.012, 0.018, -0.006}
	r, err := CalcVaR(returns, 0.95, 100000, "parametric")
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
	r, err := CalcVaR(returns, 0.90, 100000, "historical")
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
	_, err := CalcVaR([]float64{}, 0.95, 100000, "parametric")
	if err == nil {
		t.Error("expected error for empty returns")
	}
}

func TestCalcVaR_InvalidConfidence(t *testing.T) {
	_, err := CalcVaR([]float64{0.01, -0.01}, 1.5, 100000, "parametric")
	if err == nil {
		t.Error("expected error for confidence_level out of range")
	}
}

// ---- CalcDCF -----------------------------------------------------------------

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

// BUG-4: the sharpe summary reported "ann. return" without clarifying it is the
// gross return (before subtracting the risk-free rate), which misled callers into
// treating it as excess return.
func TestRegression_BUG4_SharpesSummaryLabelsReturnAsGross(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	r, err := CalcSharpe(returns, 0.04)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Summary, "gross") {
		t.Errorf("BUG-4 regression: summary must label annualized return as gross (pre risk-free); got %q", r.Summary)
	}
}

// BUG-5: probit used the A&S 26.2.17 polynomial (max error 4.5e-4), which
// degraded VaR accuracy at extreme confidence levels (99%+). The replacement
// uses math.Erfinv, which achieves machine precision.
func TestRegression_BUG5_ProbitExtremeQuantilePrecision(t *testing.T) {
	// Reference values from standard normal tables (correct to 16 significant figures).
	cases := []struct {
		p    float64
		want float64
	}{
		{0.99, 2.3263478740408408},
		{0.999, 3.0902323061678132},
		{0.9999, 3.7190164854556844},
	}
	// Tolerance far tighter than the old polynomial error of 4.5e-4.
	const tol = 1e-9
	for _, tc := range cases {
		got := probit(tc.p)
		if !approxEqual(got, tc.want, tol) {
			t.Errorf("BUG-5 regression: probit(%.4f): want %.15f, got %.15f (tol %.0e)",
				tc.p, tc.want, got, tol)
		}
	}
}

// ---- Technical indicators -----------------------------------------------------
//
// The golden values in these tests come from canonical technical-analysis
// references (Wilder, 1978; Achelis, 2001). Tolerance is set generously enough
// to absorb floating-point noise but tight enough to catch algorithmic bugs.

// --- CalcSMA ------------------------------------------------------------------

func TestRegression_REF3_CalcSharpe_RejectsNaNReturn(t *testing.T) {
	if _, err := CalcSharpe([]float64{0.01, math.NaN(), 0.02}, 0.04); err == nil {
		t.Error("expected error for NaN in returns")
	}
}

func TestRegression_REF3_CalcSharpe_RejectsInfReturn(t *testing.T) {
	if _, err := CalcSharpe([]float64{0.01, math.Inf(1), 0.02}, 0.04); err == nil {
		t.Error("expected error for +Inf in returns")
	}
}

func TestRegression_REF3_CalcSharpe_Accepts1000PercentReturn(t *testing.T) {
	// A single-period return of +1000% (the position multiplied ~11x) is a
	// real, if extreme, market event and must not be rejected.
	returns := []float64{0.01, -0.02, 10.0, 0.015}
	if _, err := CalcSharpe(returns, 0.04); err != nil {
		t.Errorf("a legitimate 1000%% return must be accepted, got error: %v", err)
	}
}

func TestRegression_REF3_CalcVaR_RejectsNaNReturn(t *testing.T) {
	if _, err := CalcVaR([]float64{0.01, math.NaN()}, 0.95, 100000, "historical"); err == nil {
		t.Error("expected error for NaN in returns")
	}
}

func TestRegression_REF3_CalcVaR_RejectsNonPositivePortfolioValue(t *testing.T) {
	if _, err := CalcVaR([]float64{0.01, -0.01}, 0.95, 0, "historical"); err == nil {
		t.Error("expected error for zero portfolio_value")
	}
	if _, err := CalcVaR([]float64{0.01, -0.01}, 0.95, -100, "historical"); err == nil {
		t.Error("expected error for negative portfolio_value")
	}
}

func TestRegression_REF3_CalcBeta_RejectsInfReturn(t *testing.T) {
	assetReturns := []float64{0.01, math.Inf(-1), 0.02}
	benchmarkReturns := []float64{0.01, 0.02, 0.015}
	if _, err := CalcBeta(assetReturns, benchmarkReturns); err == nil {
		t.Error("expected error for -Inf in asset_returns")
	}
}

func TestRegression_REF3_CalcVolatility_RejectsZeroOrNegativePrice(t *testing.T) {
	if _, err := CalcVolatility([]float64{100, 0, 105}); err == nil {
		t.Error("expected error for zero price in series")
	}
	if _, err := CalcVolatility([]float64{100, -50, 105}); err == nil {
		t.Error("expected error for negative price in series")
	}
}

func TestRegression_REF3_CalcMaxDrawdown_RejectsNonPositivePrice(t *testing.T) {
	// CalcMaxDrawdown previously had zero validation on its prices series.
	if _, err := CalcMaxDrawdown([]float64{100, 0, 90}); err == nil {
		t.Error("expected error for zero price in series")
	}
	if _, err := CalcMaxDrawdown([]float64{100, math.NaN(), 90}); err == nil {
		t.Error("expected error for NaN price in series")
	}
}
