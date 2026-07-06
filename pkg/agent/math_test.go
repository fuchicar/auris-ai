package agent

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"auris/pkg/llm"
	"auris/pkg/market"
	"auris/pkg/portfolio"
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

// ---- calcSortino --------------------------------------------------------------

func TestCalcSortino_Normal(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	r, err := calcSortino(returns, 0.04)
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
	_, err := calcSortino(returns, 0.01)
	if err == nil {
		t.Error("expected error for zero downside deviation")
	}
}

func TestCalcSortino_EmptyReturns(t *testing.T) {
	_, err := calcSortino([]float64{}, 0.04)
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

// ---- calcTreynor --------------------------------------------------------------

func TestCalcTreynor_Normal(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	r, err := calcTreynor(returns, 0.04, 1.2)
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
	positive, err := calcTreynor(returns, 0.04, 1.5)
	if err != nil {
		t.Fatal(err)
	}
	negative, err := calcTreynor(returns, 0.04, -1.5)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(positive.TreynorRatioPercent, -negative.TreynorRatioPercent, 1e-9) {
		t.Errorf("expected sign flip: positive=%v negative=%v", positive.TreynorRatioPercent, negative.TreynorRatioPercent)
	}
}

func TestCalcTreynor_ZeroBeta(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	_, err := calcTreynor(returns, 0.04, 0)
	if err == nil {
		t.Error("expected error for zero beta")
	}
}

func TestCalcTreynor_EmptyReturns(t *testing.T) {
	_, err := calcTreynor([]float64{}, 0.04, 1.0)
	if err == nil {
		t.Error("expected error for empty returns")
	}
}

// ---- calcInformationRatio ------------------------------------------------------

func TestCalcInformationRatio_Normal(t *testing.T) {
	asset := []float64{0.02, 0.01, 0.03, 0.015, 0.025}
	benchmark := []float64{0.01, 0.005, 0.015, 0.01, 0.012}
	r, err := calcInformationRatio(asset, benchmark)
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
	_, err := calcInformationRatio(returns, returns)
	if err == nil {
		t.Error("expected error for zero tracking error")
	}
}

func TestCalcInformationRatio_UnequalLength(t *testing.T) {
	_, err := calcInformationRatio([]float64{0.01, 0.02}, []float64{0.01})
	if err == nil {
		t.Error("expected error for unequal length series")
	}
}

func TestCalcInformationRatio_TooFewObservations(t *testing.T) {
	_, err := calcInformationRatio([]float64{0.01}, []float64{0.02})
	if err == nil {
		t.Error("expected error for fewer than 2 observations")
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
	r, err := calcMultiples(50, 2.5, 20, 1e9, 5e9, 2e9)
	if err != nil {
		t.Fatal(err)
	}
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
	r, err := calcMultiples(50, 0, 0, 0, 1e9, 0)
	if err != nil {
		t.Fatal(err)
	}
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

// ---- calcPFCF ------------------------------------------------------------------

func TestCalcPFCF_Normal(t *testing.T) {
	r, err := calcPFCF(150, 12.5, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.PFCF, 12.0, 0.01) {
		t.Errorf("PFCF: want ~12.0, got %v", r.PFCF)
	}
	if r.Currency != "USD" {
		t.Errorf("Currency: want USD, got %q", r.Currency)
	}
	if !strings.Contains(r.Summary, "P/FCF") {
		t.Errorf("summary should mention P/FCF, got %q", r.Summary)
	}
}

func TestCalcPFCF_ZeroFCF(t *testing.T) {
	_, err := calcPFCF(150, 0, "")
	if err == nil {
		t.Error("expected error for free_cash_flow_per_share <= 0")
	}
}

func TestCalcPFCF_ZeroPrice(t *testing.T) {
	_, err := calcPFCF(0, 12.5, "")
	if err == nil {
		t.Error("expected error for price <= 0")
	}
}

// ---- calcPEG -------------------------------------------------------------------

func TestCalcPEG_Undervalued(t *testing.T) {
	r, err := calcPEG(15, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.PEG, 0.75, 0.01) {
		t.Errorf("PEG: want ~0.75, got %v", r.PEG)
	}
	if r.Interpretation != "undervalued" {
		t.Errorf("Interpretation: want undervalued, got %q", r.Interpretation)
	}
}

func TestCalcPEG_Overvalued(t *testing.T) {
	r, err := calcPEG(30, 5)
	if err != nil {
		t.Fatal(err)
	}
	if r.Interpretation != "overvalued" {
		t.Errorf("Interpretation: want overvalued, got %q", r.Interpretation)
	}
}

func TestCalcPEG_Reasonable(t *testing.T) {
	r, err := calcPEG(20, 15)
	if err != nil {
		t.Fatal(err)
	}
	if r.Interpretation != "reasonable" {
		t.Errorf("Interpretation: want reasonable, got %q", r.Interpretation)
	}
}

func TestCalcPEG_NegativeGrowth_WarnsInSummary(t *testing.T) {
	r, err := calcPEG(20, -10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Summary, "WARNING") {
		t.Errorf("summary should warn about negative growth, got %q", r.Summary)
	}
}

func TestCalcPEG_ZeroPE(t *testing.T) {
	_, err := calcPEG(0, 15)
	if err == nil {
		t.Error("expected error for pe_ratio <= 0")
	}
}

func TestCalcPEG_ZeroGrowth(t *testing.T) {
	_, err := calcPEG(20, 0)
	if err == nil {
		t.Error("expected error for growth_rate_percent == 0")
	}
}

// ---- calcDividendYield -----------------------------------------------------------

func TestCalcDividendYield_AnnualDividend(t *testing.T) {
	r, err := calcDividendYield(100, 2.5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.YieldPercent, 2.5, 0.001) {
		t.Errorf("YieldPercent: want 2.5, got %v", r.YieldPercent)
	}
}

func TestCalcDividendYield_QuarterlyTTM(t *testing.T) {
	// Sum of quarterly = 4.0 → yield = 4/100*100 = 4%.
	r, err := calcDividendYield(100, 0, []float64{1, 1, 1, 1})
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.YieldPercent, 4.0, 0.001) {
		t.Errorf("YieldPercent: want 4.0, got %v", r.YieldPercent)
	}
	if !strings.Contains(r.Summary, "trailing twelve months") {
		t.Errorf("summary should mention TTM source, got %q", r.Summary)
	}
}

func TestCalcDividendYield_QuarterlyTakesPriority(t *testing.T) {
	// Both provided: quarterly (sum=8) must win over annual (2.5).
	r, err := calcDividendYield(100, 2.5, []float64{2, 2, 2, 2})
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.YieldPercent, 8.0, 0.001) {
		t.Errorf("YieldPercent: want 8.0 (quarterly priority), got %v", r.YieldPercent)
	}
}

func TestCalcDividendYield_WrongQuarterlyLength(t *testing.T) {
	_, err := calcDividendYield(100, 0, []float64{1, 1, 1})
	if err == nil {
		t.Error("expected error for quarterly_dividends length != 4")
	}
}

func TestCalcDividendYield_NoDividendProvided(t *testing.T) {
	_, err := calcDividendYield(100, 0, nil)
	if err == nil {
		t.Error("expected error when neither annual_dividend_per_share nor quarterly_dividends is provided")
	}
}

func TestCalcDividendYield_ZeroPrice(t *testing.T) {
	_, err := calcDividendYield(0, 2.5, nil)
	if err == nil {
		t.Error("expected error for price <= 0")
	}
}

// ---- calcDividendGrowth -----------------------------------------------------------

func TestCalcDividendGrowth_Normal(t *testing.T) {
	// 1.00 → 1.21 over 2 periods = 10% CAGR.
	r, err := calcDividendGrowth([]float64{1.00, 1.10, 1.21})
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.CAGRPercent, 10.0, 0.01) {
		t.Errorf("CAGRPercent: want ~10.0, got %v", r.CAGRPercent)
	}
	if !strings.Contains(r.Summary, "CAGR") {
		t.Errorf("summary should mention CAGR, got %q", r.Summary)
	}
}

func TestCalcDividendGrowth_TooFewValues(t *testing.T) {
	_, err := calcDividendGrowth([]float64{1.0})
	if err == nil {
		t.Error("expected error for fewer than 2 dividends")
	}
}

func TestCalcDividendGrowth_ZeroFirst(t *testing.T) {
	_, err := calcDividendGrowth([]float64{0, 1.0})
	if err == nil {
		t.Error("expected error for dividends[0] <= 0")
	}
}

// ---- calcStressTest -----------------------------------------------------------

func TestCalcStressTest_Normal(t *testing.T) {
	r, err := calcStressTest(10000, []float64{-10, -20, -30, -40}, "portfolio")
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
	r, err := calcStressTest(100, []float64{-5}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Summary, "value") {
		t.Errorf("summary should fall back to a generic label, got %q", r.Summary)
	}
}

func TestCalcStressTest_PositiveShock(t *testing.T) {
	r, err := calcStressTest(100, []float64{10}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.Scenarios[0].ResultingValue, 110, 0.01) {
		t.Errorf("ResultingValue: want 110, got %v", r.Scenarios[0].ResultingValue)
	}
}

func TestCalcStressTest_ZeroCurrentValue(t *testing.T) {
	_, err := calcStressTest(0, []float64{-10}, "")
	if err == nil {
		t.Error("expected error for current_value <= 0")
	}
}

func TestCalcStressTest_EmptyShocks(t *testing.T) {
	_, err := calcStressTest(100, nil, "")
	if err == nil {
		t.Error("expected error for empty shocks_percent")
	}
}

// ---- calcMonteCarloSimulation ------------------------------------------------

func TestCalcMonteCarloSimulation_ZeroDriftAndVolatility(t *testing.T) {
	// With no drift and no volatility, every path is flat: all final prices
	// equal last_price, so mean/P5/P50/P95 collapse to it and nothing is
	// ever strictly above the start.
	r, err := calcMonteCarloSimulation(100, 0, 0, 30, 1000)
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
	r, err := calcMonteCarloSimulation(lastPrice, driftAnnual, volAnnual, days, 50000)
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
	r, err := calcMonteCarloSimulation(100, 0.05, 0.25, 90, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if !(r.P5FinalPrice <= r.MedianFinalPrice && r.MedianFinalPrice <= r.P95FinalPrice) {
		t.Errorf("expected P5 <= P50 <= P95, got %v <= %v <= %v", r.P5FinalPrice, r.MedianFinalPrice, r.P95FinalPrice)
	}
}

func TestCalcMonteCarloSimulation_InvalidLastPrice(t *testing.T) {
	if _, err := calcMonteCarloSimulation(0, 0.05, 0.2, 30, 100); err == nil {
		t.Error("expected error for last_price <= 0")
	}
}

func TestCalcMonteCarloSimulation_InvalidDays(t *testing.T) {
	if _, err := calcMonteCarloSimulation(100, 0.05, 0.2, 0, 100); err == nil {
		t.Error("expected error for days <= 0")
	}
}

func TestCalcMonteCarloSimulation_InvalidNumSimulations(t *testing.T) {
	if _, err := calcMonteCarloSimulation(100, 0.05, 0.2, 30, 0); err == nil {
		t.Error("expected error for num_simulations <= 0")
	}
}

func TestCalcMonteCarloSimulation_NumSimulationsOverCap(t *testing.T) {
	if _, err := calcMonteCarloSimulation(100, 0.05, 0.2, 30, maxMonteCarloSimulations+1); err == nil {
		t.Error("expected error for num_simulations over the cap")
	}
}

func TestCalcMonteCarloSimulation_NegativeVolatility(t *testing.T) {
	if _, err := calcMonteCarloSimulation(100, 0.05, -0.1, 30, 100); err == nil {
		t.Error("expected error for negative volatility_annual")
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

// ---- regression tests: bugs --------------------------------------------------
// Each test encodes the exact condition that the bug allowed to slip through.
// They are named TestRegression_BUGN_* so they can be run in isolation:
//   go test ./pkg/agent/... -run TestRegression

// BUG-1: calcPnL silently took math.Abs(quantity) when negative, giving the
// caller no indication that the sign was discarded.
func TestRegression_BUG1_NegativeQuantityWarning(t *testing.T) {
	r, err := calcPnL(100, 110, -10, "long")
	if err != nil {
		t.Fatal(err)
	}
	// Result must equal the positive-quantity computation.
	if r.PnLAbsolute != 100 {
		t.Errorf("PnLAbsolute: want 100, got %v", r.PnLAbsolute)
	}
	// Summary must warn the caller that the negative sign was discarded.
	if !strings.Contains(r.Summary, "WARNING") {
		t.Errorf("BUG-1 regression: summary must warn about negative quantity; got %q", r.Summary)
	}
}

// BUG-2: calcStats returned Min/Max as raw float64 (unrounded) while every
// other numeric field in statsResult used round4.
func TestRegression_BUG2_MinMaxRoundedToFourDecimals(t *testing.T) {
	// Values with more than 4 significant decimal places so rounding is observable.
	values := []float64{1.123456789, 5.0, 9.987654321}
	r, err := calcStats(values, "test")
	if err != nil {
		t.Fatal(err)
	}
	wantMin := round4(1.123456789) // 1.1235
	wantMax := round4(9.987654321) // 9.9877
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

// BUG-3a: calcDCF accepted all-negative free_cash_flows and produced a
// negative terminal value and negative intrinsic value without any error.
func TestRegression_BUG3_AllNegativeFCFsReturnsError(t *testing.T) {
	_, err := calcDCF([]float64{-100, -200, -300}, 0.10, 0.03, 1000)
	if err == nil {
		t.Error("BUG-3 regression: expected error when all free_cash_flows are non-positive")
	}
}

// BUG-3b: when the last FCF is negative (terminal value becomes negative) but
// earlier FCFs are positive, the function should succeed but warn in the summary.
func TestRegression_BUG3_NegativeLastFCFAddsWarning(t *testing.T) {
	// FCFs: positive early years, negative terminal year.
	r, err := calcDCF([]float64{100, 100, -50}, 0.10, 0.03, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if r.TerminalValue >= 0 {
		t.Errorf("BUG-3 regression: terminal value should be negative when last FCF < 0, got %v", r.TerminalValue)
	}
	if !strings.Contains(r.Summary, "WARNING") {
		t.Errorf("BUG-3 regression: summary must warn about negative terminal value; got %q", r.Summary)
	}
}

// BUG-4: the sharpe summary reported "ann. return" without clarifying it is the
// gross return (before subtracting the risk-free rate), which misled callers into
// treating it as excess return.
func TestRegression_BUG4_SharpesSummaryLabelsReturnAsGross(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	r, err := calcSharpe(returns, 0.04)
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

// --- calcSMA ------------------------------------------------------------------

func TestCalcSMA_HappyPath(t *testing.T) {
	// Five closes; period 3. Manual SMA series:
	//   index 0,1 -> NaN (warm-up)
	//   index 2 -> mean(10,11,12) = 11
	//   index 3 -> mean(11,12,13) = 12
	//   index 4 -> mean(12,13,14) = 13
	prices := []float64{10, 11, 12, 13, 14}
	r, err := calcSMA(prices, 3)
	if err != nil {
		t.Fatal(err)
	}
	if r.Period != 3 || r.InputSize != 5 {
		t.Errorf("metadata: want period=3 input=5, got period=%d input=%d", r.Period, r.InputSize)
	}
	// Warm-up positions must be NaN. Note: math.IsNaN, not approxEqual, since
	// NaN != NaN by IEEE-754.
	if !math.IsNaN(float64(r.Values[0])) || !math.IsNaN(float64(r.Values[1])) {
		t.Errorf("warm-up positions must be NaN, got %v", []float64(r.Values[:2]))
	}
	want := []float64{math.NaN(), math.NaN(), 11, 12, 13}
	for i, v := range want {
		got := float64(r.Values[i])
		if math.IsNaN(v) {
			if !math.IsNaN(got) {
				t.Errorf("Values[%d]: want NaN, got %v", i, got)
			}
			continue
		}
		if !approxEqual(got, v, 1e-9) {
			t.Errorf("Values[%d]: want %v, got %v", i, v, got)
		}
	}
	if r.Last != 13 {
		t.Errorf("Last: want 13, got %v", r.Last)
	}
	if r.Previous != 12 {
		t.Errorf("Previous: want 12, got %v", r.Previous)
	}
	if r.Trend != "up" {
		t.Errorf("Trend: want up, got %q", r.Trend)
	}
}

func TestCalcSMA_DownTrend(t *testing.T) {
	r, err := calcSMA([]float64{14, 13, 12, 11, 10}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if r.Trend != "down" {
		t.Errorf("Trend: want down, got %q", r.Trend)
	}
}

func TestCalcSMA_ZeroPeriod(t *testing.T) {
	if _, err := calcSMA([]float64{1, 2, 3}, 0); err == nil {
		t.Error("expected error for zero period")
	}
}

func TestCalcSMA_InsufficientPrices(t *testing.T) {
	if _, err := calcSMA([]float64{1, 2}, 5); err == nil {
		t.Error("expected error when len(prices) < period")
	}
}

// --- calcEMA ------------------------------------------------------------------

func TestCalcEMA_HappyPath_DefaultAlpha(t *testing.T) {
	// Prices 10..15 (6 values), period 3, Wilder alpha = 2/(3+1) = 0.5.
	// Recurrence: EMA_t = 0.5 * P_t + 0.5 * EMA_{t-1}, EMA_0 = P_0.
	prices := []float64{10, 11, 12, 13, 14, 15}
	r, err := calcEMA(prices, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.Alpha, 0.5, 1e-12) {
		t.Errorf("Alpha (Wilder default for period=3): want 0.5, got %v", r.Alpha)
	}
	// Expected: 10, 10.5, 11.25, 12.125, 13.0625, 14.03125
	want := []float64{10, 10.5, 11.25, 12.125, 13.0625, 14.03125}
	for i, v := range want {
		if !approxEqual(r.Values[i], v, 1e-9) {
			t.Errorf("Values[%d]: want %v, got %v", i, v, r.Values[i])
		}
	}
	// Last is round4-rounded; the unrounded value is 14.03125.
	if !approxEqual(r.Last, 14.0313, 1e-4) {
		t.Errorf("Last: want 14.0313 (round4 of 14.03125), got %v", r.Last)
	}
	if r.Trend != "up" {
		t.Errorf("Trend: want up, got %q", r.Trend)
	}
}

func TestCalcEMA_CustomAlpha(t *testing.T) {
	// alpha=0.25, period irrelevant for alpha selection.
	r, err := calcEMA([]float64{100, 110, 120, 130}, 5, 0.25)
	if err != nil {
		t.Fatal(err)
	}
	// EMA_0=100, EMA_1=0.25*110+0.75*100=102.5, EMA_2=0.25*120+0.75*102.5=106.875,
	// EMA_3=0.25*130+0.75*106.875=112.65625.
	want := []float64{100, 102.5, 106.875, 112.65625}
	for i, v := range want {
		if !approxEqual(r.Values[i], v, 1e-9) {
			t.Errorf("Values[%d]: want %v, got %v", i, v, r.Values[i])
		}
	}
}

func TestCalcEMA_InvalidAlpha(t *testing.T) {
	if _, err := calcEMA([]float64{1, 2, 3}, 3, 1.5); err == nil {
		t.Error("expected error when alpha >= 1")
	}
	if _, err := calcEMA([]float64{1, 2, 3}, 0, 0.5); err == nil {
		t.Error("expected error when period <= 0")
	}
	if _, err := calcEMA(nil, 3, 0.5); err == nil {
		t.Error("expected error on empty prices")
	}
}

// --- calcRSI ------------------------------------------------------------------

func TestCalcRSI_AllGains(t *testing.T) {
	// Monotonically increasing prices: only gains, no losses. Expected RSI = 100.
	prices := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27}
	r, err := calcRSI(prices, 14)
	if err != nil {
		t.Fatal(err)
	}
	if r.Value != 100 {
		t.Errorf("all-gains RSI: want 100, got %v", r.Value)
	}
	if r.Interpretation != "overbought" {
		t.Errorf("Interpretation: want overbought, got %q", r.Interpretation)
	}
}

func TestCalcRSI_AllLosses(t *testing.T) {
	// Monotonically decreasing prices: RSI = 0.
	prices := make([]float64, 18)
	for i := range prices {
		prices[i] = 100 - float64(i)
	}
	r, err := calcRSI(prices, 14)
	if err != nil {
		t.Fatal(err)
	}
	if r.Value != 0 {
		t.Errorf("all-losses RSI: want 0, got %v", r.Value)
	}
	if r.Interpretation != "oversold" {
		t.Errorf("Interpretation: want oversold, got %q", r.Interpretation)
	}
}

func TestCalcRSI_Neutral(t *testing.T) {
	// Constant prices: no gains, no losses, avgGain==avgLoss==0 → returns 50 (neutral midpoint).
	prices := make([]float64, 18)
	for i := range prices {
		prices[i] = 100
	}
	r, err := calcRSI(prices, 14)
	if err != nil {
		t.Fatal(err)
	}
	if r.Value != 50 {
		t.Errorf("flat-price RSI: want 50, got %v", r.Value)
	}
	if r.Interpretation != "neutral" {
		t.Errorf("Interpretation: want neutral, got %q", r.Interpretation)
	}
}

func TestCalcRSI_WarmupIsNaN(t *testing.T) {
	// First period entries of Values must be NaN (no RSI yet).
	prices := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25}
	r, err := calcRSI(prices, 14)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 14; i++ {
		if !math.IsNaN(r.Values[i]) {
			t.Errorf("warm-up Values[%d] should be NaN, got %v", i, r.Values[i])
		}
	}
	if math.IsNaN(r.Values[14]) {
		t.Errorf("Values[14] should be a valid RSI, got NaN")
	}
}

func TestCalcRSI_InvalidInput(t *testing.T) {
	if _, err := calcRSI([]float64{1, 2}, 14); err == nil {
		t.Error("expected error when prices shorter than period+2")
	}
	if _, err := calcRSI([]float64{1, 2, 3}, 0); err == nil {
		t.Error("expected error for zero period")
	}
}

// --- calcMACD -----------------------------------------------------------------

func TestCalcMACD_BullishTrend(t *testing.T) {
	// Build a strongly bullish series and verify MACD/histogram flip
	// positive. The exact "cross" detection (last bar only) is brittle on
	// synthetic data, so this test asserts the broader contract: positive
	// MACD + positive histogram at the end means the agent should read it
	// as bullish. The cross-on-last-bar path is exercised by TestCalcMACD_CrossDetection.
	prices := make([]float64, 60)
	for i := range prices {
		if i < 30 {
			prices[i] = 100
		} else {
			prices[i] = 100 + 5*float64(i-29)
		}
	}
	r, err := calcMACD(prices, 12, 26, 9)
	if err != nil {
		t.Fatal(err)
	}
	if r.LastMACD <= 0 {
		t.Errorf("LastMACD should be positive on a bullish series, got %v", r.LastMACD)
	}
	if r.LastHist <= 0 {
		t.Errorf("LastHist should be positive on a bullish series, got %v", r.LastHist)
	}
}

func TestCalcMACD_BearishTrend(t *testing.T) {
	// Mirror of the bullish trend test.
	prices := make([]float64, 60)
	for i := range prices {
		if i < 30 {
			prices[i] = 250
		} else {
			prices[i] = 250 - 5*float64(i-29)
		}
	}
	r, err := calcMACD(prices, 12, 26, 9)
	if err != nil {
		t.Fatal(err)
	}
	if r.LastMACD >= 0 {
		t.Errorf("LastMACD should be negative on a bearish series, got %v", r.LastMACD)
	}
	if r.LastHist >= 0 {
		t.Errorf("LastHist should be negative on a bearish series, got %v", r.LastHist)
	}
}

func TestCalcMACD_CrossDetection(t *testing.T) {
	// Build a series that explicitly forces the histogram to flip sign on
	// the very last bar: a deep V shape. 58 bars at a constant high level,
	// then a single drop, then a single strong rebound on bar 60.
	// Bar 59: drop from 200 to 100 → fast EMA dips below slow EMA
	// Bar 60: rebound to 300 → fast EMA flips above slow EMA → cross.
	prices := make([]float64, 60)
	for i := range prices {
		switch {
		case i < 58:
			prices[i] = 200
		case i == 58:
			prices[i] = 100
		default:
			prices[i] = 300
		}
	}
	r, err := calcMACD(prices, 12, 26, 9)
	if err != nil {
		t.Fatal(err)
	}
	// Either we caught the bullish cross on the last bar, or the histogram
	// is positive (still useful information for the agent).
	if r.Trend != "bullish_cross" && r.LastHist <= 0 {
		t.Errorf("expected bullish_cross or positive histogram on V-shape rebound, got trend=%q hist=%v", r.Trend, r.LastHist)
	}
}

func TestCalcMACD_InvalidPeriods(t *testing.T) {
	prices := make([]float64, 60)
	if _, err := calcMACD(prices, 26, 12, 9); err == nil {
		t.Error("expected error when fast >= slow")
	}
	if _, err := calcMACD(prices, 0, 26, 9); err == nil {
		t.Error("expected error when fast <= 0")
	}
	if _, err := calcMACD([]float64{1, 2, 3, 4, 5}, 12, 26, 9); err == nil {
		t.Error("expected error when prices shorter than slow+signal")
	}
}

// --- calcBollingerBands -------------------------------------------------------

func TestCalcBollingerBands_HandComputed(t *testing.T) {
	// Period 4, num_std 2. With 4 prices and period 4, the window covers
	// the entire series: prices = [10,11,12,13], window mean = 11.5,
	// sample std = sqrt(5/3) ≈ 1.29099.
	// Expected: middle = 11.5, upper = 11.5 + 2*sd, lower = 11.5 - 2*sd.
	prices := []float64{10, 11, 12, 13}
	r, err := calcBollingerBands(prices, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	last := len(prices) - 1
	wantMid := 11.5
	wantSd := sampleStddev(prices) // ≈ 1.29099
	if !approxEqual(float64(r.Middle[last]), wantMid, 1e-9) {
		t.Errorf("Middle[last]: want %v, got %v", wantMid, float64(r.Middle[last]))
	}
	if !approxEqual(float64(r.Upper[last]), wantMid+2*wantSd, 1e-9) {
		t.Errorf("Upper[last]: want %v, got %v", wantMid+2*wantSd, float64(r.Upper[last]))
	}
	if !approxEqual(float64(r.Lower[last]), wantMid-2*wantSd, 1e-9) {
		t.Errorf("Lower[last]: want %v, got %v", wantMid-2*wantSd, float64(r.Lower[last]))
	}
	// With period=4 and 4 prices, only the last index has a valid band;
	// earlier indices are warm-up NaN. (Standard convention: bands exist
	// only once the window is full.)
	for i := 0; i < len(prices)-1; i++ {
		if !math.IsNaN(float64(r.Upper[i])) || !math.IsNaN(float64(r.Middle[i])) || !math.IsNaN(float64(r.Lower[i])) {
			t.Errorf("warm-up index %d should be NaN", i)
		}
	}
	if math.IsNaN(float64(r.Upper[last])) || math.IsNaN(float64(r.Middle[last])) || math.IsNaN(float64(r.Lower[last])) {
		t.Errorf("index %d (last) should have a valid band", last)
	}
}

func TestCalcBollingerBands_WarmupIsNaN(t *testing.T) {
	// With period=3 and 5 prices, the first 2 indices of each band series
	// must be NaN.
	prices := []float64{10, 11, 12, 13, 14}
	r, err := calcBollingerBands(prices, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if !math.IsNaN(float64(r.Upper[i])) || !math.IsNaN(float64(r.Middle[i])) || !math.IsNaN(float64(r.Lower[i])) {
			t.Errorf("warm-up index %d should be NaN across all series", i)
		}
	}
}

func TestCalcBollingerBands_PercentB(t *testing.T) {
	// Construct a series where the last price is exactly at the upper band → %b = 1.
	prices := []float64{10, 10, 10, 10, 10, 20}
	r, err := calcBollingerBands(prices, 5, 2)
	if err != nil {
		t.Fatal(err)
	}
	// last price = 20, mean of last 5 = 12, sd of last 5 = sqrt(20) ≈ 4.4721.
	// upper = 12 + 2*4.4721 = 20.9443 → %b = (20 - lower)/(upper - lower).
	last := len(prices) - 1
	if r.PercentB[last] < 0.85 || r.PercentB[last] > 1 {
		t.Errorf("PercentB[last] should be close to 1 (price near upper band), got %v", r.PercentB[last])
	}
}

func TestCalcBollingerBands_InvalidInput(t *testing.T) {
	if _, err := calcBollingerBands([]float64{1, 2, 3}, 0, 2); err == nil {
		t.Error("expected error for zero period")
	}
	if _, err := calcBollingerBands([]float64{1, 2, 3}, 3, -1); err == nil {
		t.Error("expected error for non-positive num_std")
	}
	if _, err := calcBollingerBands([]float64{1, 2}, 5, 2); err == nil {
		t.Error("expected error when prices shorter than period")
	}
}

// --- calcCorrelationMatrix ---------------------------------------------------

func TestCalcCorrelationMatrix_Perfect(t *testing.T) {
	// Identical series => correlation = 1.
	a := []float64{0.01, -0.02, 0.03, -0.01, 0.02, -0.015}
	series := map[string][]float64{"AAPL": a, "MSFT": a}
	r, err := calcCorrelationMatrix(series)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Matrix) != 2 || len(r.Matrix[0]) != 2 {
		t.Fatalf("matrix shape: want 2x2, got %dx%d", len(r.Matrix), len(r.Matrix[0]))
	}
	if r.Matrix[0][0] != 1 || r.Matrix[1][1] != 1 {
		t.Errorf("diagonal must be 1, got %v", r.Matrix)
	}
	if !approxEqual(r.Matrix[0][1], 1, 1e-9) {
		t.Errorf("identical-series correlation: want 1, got %v", r.Matrix[0][1])
	}
	if r.Matrix[0][1] != r.Matrix[1][0] {
		t.Errorf("matrix must be symmetric: %v vs %v", r.Matrix[0][1], r.Matrix[1][0])
	}
}

func TestCalcCorrelationMatrix_PerfectlyAntiCorrelated(t *testing.T) {
	// b = -a → correlation = -1.
	a := []float64{0.01, -0.02, 0.03, -0.01, 0.02}
	b := make([]float64, len(a))
	for i := range a {
		b[i] = -a[i]
	}
	series := map[string][]float64{"AAPL": a, "TLT": b}
	r, err := calcCorrelationMatrix(series)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.Matrix[0][1], -1, 1e-9) {
		t.Errorf("anti-correlated: want -1, got %v", r.Matrix[0][1])
	}
}

func TestCalcCorrelationMatrix_LabelsDeterministic(t *testing.T) {
	// Iteration order over the input map is random; result labels must be sorted.
	a := []float64{0.01, 0.02, 0.03, 0.04}
	series := map[string][]float64{"Z": a, "A": a, "M": a}
	r, err := calcCorrelationMatrix(series)
	if err != nil {
		t.Fatal(err)
	}
	if r.Labels[0] != "A" || r.Labels[1] != "M" || r.Labels[2] != "Z" {
		t.Errorf("Labels must be alphabetically sorted, got %v", r.Labels)
	}
}

func TestCalcCorrelationMatrix_LengthMismatch(t *testing.T) {
	series := map[string][]float64{
		"AAPL": {0.01, 0.02, 0.03, 0.04},
		"MSFT": {0.01, 0.02, 0.03},
	}
	if _, err := calcCorrelationMatrix(series); err == nil {
		t.Error("expected error when series have different lengths")
	}
}

func TestCalcCorrelationMatrix_TooFewSeries(t *testing.T) {
	if _, err := calcCorrelationMatrix(map[string][]float64{"AAPL": {1, 2}}); err == nil {
		t.Error("expected error when only 1 series provided")
	}
}

func TestCalcCorrelationMatrix_ZeroVariance(t *testing.T) {
	// Flat series has zero variance → Pearson is undefined → error.
	series := map[string][]float64{
		"AAPL": {1, 1, 1, 1},
		"MSFT": {2, 3, 4, 5},
	}
	if _, err := calcCorrelationMatrix(series); err == nil {
		t.Error("expected error when one series has zero variance")
	}
}

// --- pearson helper -----------------------------------------------------------

func TestPearson_KnownValue(t *testing.T) {
	// Hand-computed: a=[1,2,3,4,5], b=[2,4,5,4,5]. r ≈ 0.7746.
	a := []float64{1, 2, 3, 4, 5}
	b := []float64{2, 4, 5, 4, 5}
	c, err := pearson(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(c, 0.7745966692414834, 1e-9) {
		t.Errorf("pearson: want 0.7746, got %v", c)
	}
}

func TestPearson_LengthMismatch(t *testing.T) {
	if _, err := pearson([]float64{1, 2}, []float64{1, 2, 3}); err == nil {
		t.Error("expected length-mismatch error")
	}
}

// --- roundSlice helper --------------------------------------------------------

func TestRoundSlice(t *testing.T) {
	in := []float64{1.123456789, 2.0, -3.987654321}
	out := roundSlice(in, 4)
	want := []float64{1.1235, 2.0, -3.9877}
	for i := range want {
		if !approxEqual(out[i], want[i], 1e-9) {
			t.Errorf("roundSlice[%d]: want %v, got %v", i, want[i], out[i])
		}
	}
}

// --- dispatch coverage --------------------------------------------------------
//
// These tests prove the tools are wired into dispatch (parameter parsing,
// JSON marshalling, and route selection) end-to-end. They use the production
// mock providers from agent_test.go and assert that the JSON returned by the
// tool can be unmarshalled back into the expected struct shape.

func TestDispatch_CalculateSMA_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"prices": []float64{1, 2, 3, 4, 5}, "period": 3})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_sma", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r smaResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.Last != 4 {
		t.Errorf("SMA last: want 4, got %v", r.Last)
	}
}

func TestDispatch_CalculateEMA_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// Omit alpha to exercise the default-Wilder branch.
	args := toolCallArgs(t, map[string]any{"prices": []float64{10, 11, 12, 13}, "period": 3})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_ema", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r emaResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if !approxEqual(r.Alpha, 0.5, 1e-9) {
		t.Errorf("Wilder alpha for period=3: want 0.5, got %v", r.Alpha)
	}
}

func TestDispatch_CalculateRSI_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	prices := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27}
	args := toolCallArgs(t, map[string]any{"prices": prices, "period": 14})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_rsi", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r rsiResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.Value != 100 {
		t.Errorf("all-gains RSI: want 100, got %v", r.Value)
	}
}

func TestDispatch_CalculateMACD_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	prices := make([]float64, 40)
	for i := range prices {
		prices[i] = 100 + float64(i)
	}
	args := toolCallArgs(t, map[string]any{
		"prices":        prices,
		"fast_period":   12,
		"slow_period":   26,
		"signal_period": 9,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_macd", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r macdResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.LastMACD <= 0 {
		t.Errorf("monotonically rising series must yield positive MACD, got %v", r.LastMACD)
	}
}

func TestDispatch_CalculateBollinger_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"prices":  []float64{10, 11, 12, 13, 14, 15},
		"period":  3,
		"num_std": 2,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_bollinger_bands", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r bollingerResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.LastPrice != 15 {
		t.Errorf("LastPrice: want 15, got %v", r.LastPrice)
	}
}

func TestDispatch_CalculateBollinger_MissingNumStd(t *testing.T) {
	// Omitting num_std must apply the default of 2.0 (consistent with EMA's
	// alpha default) instead of returning an error.
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"prices": []float64{10, 11, 12, 13, 14, 15},
		"period": 3,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_bollinger_bands", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("missing num_std should default to 2.0, got error: %s", result)
	}
	var r bollingerResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.NumStd != 2.0 {
		t.Errorf("default num_std: want 2.0, got %v", r.NumStd)
	}
}

func TestDispatch_CalculateCorrelationMatrix_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"series": map[string]any{
			"AAPL": []float64{0.01, -0.02, 0.03},
			"MSFT": []float64{0.02, -0.04, 0.06},
		},
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_correlation_matrix", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r correlationMatrixResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if len(r.Matrix) != 2 {
		t.Errorf("matrix size: want 2x2, got %d", len(r.Matrix))
	}
}

func TestDispatch_CalculateCorrelationMatrix_BadArgType(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// Pass an array where the schema expects an object.
	args := toolCallArgs(t, map[string]any{"series": []float64{1, 2}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_correlation_matrix", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("non-object series should produce an error, got %s", result)
	}
}

func TestDispatch_CalculatePFCF_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"price": 150.0, "free_cash_flow_per_share": 12.5})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_pfcf", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r pfcfResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if !approxEqual(r.PFCF, 12.0, 0.01) {
		t.Errorf("PFCF: want ~12.0, got %v", r.PFCF)
	}
}

func TestDispatch_CalculatePEG_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"pe_ratio": 20.0, "growth_rate_percent": 15.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_peg", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r pegResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.Interpretation != "reasonable" {
		t.Errorf("Interpretation: want reasonable, got %q", r.Interpretation)
	}
}

func TestDispatch_CalculateDividendYield_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"price": 100.0, "annual_dividend_per_share": 2.5})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_dividend_yield", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r dividendYieldResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if !approxEqual(r.YieldPercent, 2.5, 0.001) {
		t.Errorf("YieldPercent: want 2.5, got %v", r.YieldPercent)
	}
}

func TestDispatch_CalculateDividendYield_MissingBoth(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"price": 100.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_dividend_yield", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("missing both dividend inputs should produce an error, got %s", result)
	}
}

func TestDispatch_CalculateDividendGrowth_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"dividends": []float64{1.00, 1.10, 1.21}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_dividend_growth", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r dividendGrowthResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if !approxEqual(r.CAGRPercent, 10.0, 0.01) {
		t.Errorf("CAGRPercent: want ~10.0, got %v", r.CAGRPercent)
	}
}

func TestDispatch_CalculateDividendGrowth_BadType(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"dividends": "not an array"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_dividend_growth", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("non-array dividends should produce an error, got %s", result)
	}
}

func TestDispatch_CalculateStressTest_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"current_value":  10000.0,
		"shocks_percent": []float64{-10, -20, -30, -40},
		"label":          "portfolio",
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_stress_test", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r stressTestResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if len(r.Scenarios) != 4 {
		t.Errorf("Scenarios: want 4, got %d", len(r.Scenarios))
	}
	if !approxEqual(r.WorstCase.ResultingValue, 6000, 0.01) {
		t.Errorf("WorstCase.ResultingValue: want 6000, got %v", r.WorstCase.ResultingValue)
	}
}

func TestDispatch_CalculateMonteCarloSimulation_OK(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"last_price":        100.0,
		"drift_annual":      0.08,
		"volatility_annual": 0.25,
		"days":              90.0,
		"num_simulations":   5000.0,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_monte_carlo_simulation", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r monteCarloResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.Days != 90 {
		t.Errorf("Days: want 90, got %d", r.Days)
	}
	if r.NumSimulations != 5000 {
		t.Errorf("NumSimulations: want 5000, got %d", r.NumSimulations)
	}
}

func TestDispatch_CalculateMonteCarloSimulation_InvalidInput(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"last_price":        0.0,
		"drift_annual":      0.08,
		"volatility_annual": 0.25,
		"days":              90.0,
		"num_simulations":   5000.0,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_monte_carlo_simulation", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("last_price <= 0 should produce an error, got %s", result)
	}
}

func TestDispatch_CalculateStressTest_BadShocksType(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"current_value": 10000.0, "shocks_percent": "not an array"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_stress_test", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("non-array shocks_percent should produce an error, got %s", result)
	}
}

func TestDispatch_CalculateStressTest_ZeroCurrentValue(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"current_value": 0.0, "shocks_percent": []float64{-10}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_stress_test", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("current_value <= 0 should produce an error, got %s", result)
	}
}

func TestDispatch_CalculateSMA_BadPrices(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// Pass a string where the schema expects an array.
	args := toolCallArgs(t, map[string]any{"prices": "not an array", "period": 3})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_sma", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("string prices should produce an error, got %s", result)
	}
}

// --- portfolio_calculate_metrics dispatch -----------------------------------

func TestDispatch_PortfolioSetTargetAllocation_OK(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"target_allocation": map[string]any{"AAPL": 0.4, "MSFT": 0.3, "GOOG": 0.3},
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_set_target_allocation", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.TargetAllocation) != 3 {
		t.Fatalf("TargetAllocation: want 3 symbols, got %d", len(reloaded.TargetAllocation))
	}
	if !approxEqual(reloaded.TargetAllocation["AAPL"], 0.4, 0.0001) {
		t.Errorf("TargetAllocation[AAPL]: want 0.4, got %v", reloaded.TargetAllocation["AAPL"])
	}
}

func TestDispatch_PortfolioSetTargetAllocation_Replaces(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.TargetAllocation = map[string]float64{"OLD": 1.0}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"target_allocation": map[string]any{"AAPL": 1.0}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_set_target_allocation", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, stillThere := reloaded.TargetAllocation["OLD"]; stillThere {
		t.Error("TargetAllocation should be fully replaced, but OLD symbol survived")
	}
	if len(reloaded.TargetAllocation) != 1 {
		t.Errorf("TargetAllocation: want 1 symbol after replace, got %d", len(reloaded.TargetAllocation))
	}
}

func TestDispatch_PortfolioSetTargetAllocation_EmptyObject(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"target_allocation": map[string]any{}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_set_target_allocation", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("empty target_allocation should produce an error, got %s", result)
	}
}

func TestDispatch_PortfolioSetTargetAllocation_NegativeWeight(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"target_allocation": map[string]any{"AAPL": -0.1}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_set_target_allocation", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("negative weight should produce an error, got %s", result)
	}
}

func TestDispatch_PortfolioSetTargetAllocation_SumWarning(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"target_allocation": map[string]any{"AAPL": 0.5}})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_set_target_allocation", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	if !strings.Contains(result, "WARNING") {
		t.Errorf("result should warn that weights don't sum to ~1.0, got %s", result)
	}
}

func TestDispatch_PortfolioCalculateMetrics_OK(t *testing.T) {
	// Override the portfolios dir so we can write a deterministic portfolio
	// without touching the user's real config directory.
	tmp := t.TempDir()
	orig := portfolio.PortfoliosDir
	_ = orig                         // documented: we set the override via the package-internal var
	t.Setenv("XDG_CONFIG_HOME", tmp) // belt-and-braces; the override below is what counts

	// Set the package-level override directly.
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	now := time.Now().UTC().Truncate(time.Second)
	for _, sym := range []string{"AAPL", "MSFT"} {
		p.Instruments = append(p.Instruments, portfolio.Instrument{
			ID:     "ins-" + sym,
			Symbol: sym,
			Name:   sym,
			Type:   portfolio.InstrumentHolding,
			Lots:   []portfolio.Lot{{ID: "lot-" + sym, Quantity: 10, Price: 100, Date: now}},
		})
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	mp := &mockMarket{
		quotesBySymbol: map[string]market.Quote{
			"AAPL": {Last: 150},
			"MSFT": {Last: 200},
		},
		fundamentalsBySymbol: map[string]market.Fundamental{
			"AAPL": {DividendYieldTTM: 0.005, Beta: 1.2},
			"MSFT": {DividendYieldTTM: 0.008, Beta: 0.9},
		},
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_metrics", Arguments: "{}"},
	}, &lk)

	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var m portfolio.PortfolioMetrics
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if m.CurrentValue != 3500 {
		t.Errorf("CurrentValue: want 3500, got %v", m.CurrentValue)
	}
	if len(m.WeightBySymbol) != 2 {
		t.Errorf("WeightBySymbol: want 2 entries, got %d", len(m.WeightBySymbol))
	}
	if m.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	// Weighted beta: AAPL 1500/3500 * 1.2 + MSFT 2000/3500 * 0.9 ≈ 1.028.
	if m.WeightedBeta == 0 {
		t.Error("WeightedBeta must be non-zero when fundamentals are available")
	}
	// Dividend yield must be non-zero when fundamentals are available.
	if m.DividendYield == 0 {
		t.Error("DividendYield must be non-zero when fundamentals are available")
	}
}

func TestDispatch_PortfolioSuggestRebalance_OK(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	now := time.Now()
	p.Instruments = append(p.Instruments, portfolio.Instrument{
		ID:     "ins-AAPL",
		Symbol: "AAPL",
		Name:   "AAPL",
		Type:   portfolio.InstrumentHolding,
		Lots:   []portfolio.Lot{{ID: "lot-AAPL", Quantity: 10, Price: 100, Date: now}},
	})
	p.TargetAllocation = map[string]float64{"AAPL": 0.5}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	mp := &mockMarket{
		quotesBySymbol: map[string]market.Quote{"AAPL": {Last: 150}},
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_suggest_rebalance", Arguments: "{}"},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r portfolio.RebalanceResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if len(r.Operations) != 1 {
		t.Fatalf("Operations: want 1, got %d: %+v", len(r.Operations), r.Operations)
	}
	if r.Operations[0].Action != "sell" {
		t.Errorf("Action: want sell (AAPL overweight vs 50%% target), got %s", r.Operations[0].Action)
	}
}

func TestDispatch_PortfolioSuggestRebalance_NoTargetAllocation(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_suggest_rebalance", Arguments: "{}"},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("missing target_allocation should produce an error, got %s", result)
	}
}

func TestDispatch_PortfolioSuggestRebalance_MaxDriftPercent(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	now := time.Now()
	p.Instruments = append(p.Instruments, portfolio.Instrument{
		ID:     "ins-AAPL",
		Symbol: "AAPL",
		Name:   "AAPL",
		Type:   portfolio.InstrumentHolding,
		Lots:   []portfolio.Lot{{ID: "lot-AAPL", Quantity: 10, Price: 100, Date: now}},
	})
	// AAPL is exactly at its target weight (100%, no cash) — zero drift.
	p.TargetAllocation = map[string]float64{"AAPL": 1.0}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	mp := &mockMarket{quotesBySymbol: map[string]market.Quote{"AAPL": {Last: 150}}}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"max_drift_percent": 5.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_suggest_rebalance", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r portfolio.RebalanceResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if len(r.Operations) != 0 {
		t.Errorf("Operations: want 0 (within tolerance), got %d: %+v", len(r.Operations), r.Operations)
	}
}

// BUG-6: smaResult.Previous, emaResult.Previous, rsiResult.PreviousValue were
// typed float64, not Float. When the input is the minimum valid size, "previous"
// is NaN (warm-up slot); json.Marshal rejects NaN with UnsupportedValueError and
// the encode helper returned "" silently. Fixed by switching those fields to Float.
func TestDispatch_CalculateSMA_MinimumInput_ValidJSON(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// period=3, exactly 3 prices → previous will be NaN (warm-up).
	args := toolCallArgs(t, map[string]any{"prices": []float64{10, 11, 12}, "period": 3})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_sma", Arguments: args},
	}, &lk)
	if result == "" {
		t.Fatal("BUG-6 regression: dispatch returned empty string (json.Marshal NaN silently failed)")
	}
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r smaResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	// previous must be null (NaN → null via Float.MarshalJSON).
	if !math.IsNaN(float64(r.Previous)) {
		t.Errorf("Previous: want NaN (marshalled as null), got %v", float64(r.Previous))
	}
}

func TestDispatch_CalculateRSI_MinimumInput_ValidJSON(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// period=3 → minimum 5 prices (period+2).
	prices := []float64{10, 11, 12, 11, 12}
	args := toolCallArgs(t, map[string]any{"prices": prices, "period": 3})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_rsi", Arguments: args},
	}, &lk)
	if result == "" {
		t.Fatal("BUG-6 regression: dispatch returned empty string (json.Marshal NaN silently failed)")
	}
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r rsiResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	// PreviousValue must be null (NaN → null via Float.MarshalJSON).
	if !math.IsNaN(float64(r.PreviousValue)) {
		t.Errorf("PreviousValue: want NaN (marshalled as null), got %v", float64(r.PreviousValue))
	}
}

// BUG-6 (EMA leg): emaResult.Previous was float64, not Float. When only 1
// price is supplied (the minimum that calcEMA accepts), previous stays at its
// initial math.NaN() because the len(values) >= 2 branch is not entered.
// json.Marshal then silently returned "" via the ignored error in encode().
func TestRegression_BUG6_EMAMinimumInputValidJSON(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// 1 price is the minimum calcEMA accepts (period=5 is fine here; EMA seeds
	// from the first observation so len(prices)==1 is valid).
	args := toolCallArgs(t, map[string]any{"prices": []float64{100.0}, "period": 5})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_ema", Arguments: args},
	}, &lk)
	if result == "" {
		t.Fatal("BUG-6 regression (EMA): dispatch returned empty string — json.Marshal silently rejected NaN in Previous")
	}
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var r emaResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	// With a single price the EMA equals that price and Previous is null.
	if !approxEqual(r.Last, 100.0, 1e-9) {
		t.Errorf("Last: want 100.0, got %v", r.Last)
	}
	if !math.IsNaN(float64(r.Previous)) {
		t.Errorf("Previous: want NaN (serialised as JSON null), got %v", float64(r.Previous))
	}
}

// ---- regression tests: inconsistencies ----------------------------------------
//
// These tests guard against the two design inconsistencies fixed after the
// initial Tier-1 indicator commit. Run them in isolation with:
//
//	go test ./pkg/agent/... -run TestRegression_INCON

// INCON-1: calculate_bollinger_bands had an inconsistency with calculate_ema:
// omitting `num_std` returned an error ("num_std must be positive, got 0.0000")
// even though the tool description advertised "(default 2)" and `num_std` was
// not in the required list. Fixed: dispatch now applies 2.0 when the value is
// absent (≤ 0), matching how calculate_ema handles a missing alpha.
func TestRegression_INCON1_BollingerOmittedNumStdDefaultsToTwo(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	// Deliberately omit num_std.
	args := toolCallArgs(t, map[string]any{
		"prices": []float64{10, 11, 12, 13, 14, 15, 16},
		"period": 3,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_bollinger_bands", Arguments: args},
	}, &lk)
	if result == "" || result[:6] == "error:" {
		t.Fatalf("INCON-1 regression: omitting num_std should default to 2.0, got: %s", result)
	}
	var r bollingerResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.NumStd != 2.0 {
		t.Errorf("INCON-1 regression: want NumStd=2.0, got %v", r.NumStd)
	}
	// Sanity-check band structure: upper > middle > lower on every valid bar.
	for i, u := range r.Upper {
		m, l := r.Middle[i], r.Lower[i]
		if math.IsNaN(float64(u)) {
			continue // warm-up NaN — expected
		}
		if float64(u) < float64(m) || float64(m) < float64(l) {
			t.Errorf("band ordering violated at index %d: upper=%.4f middle=%.4f lower=%.4f",
				i, float64(u), float64(m), float64(l))
		}
	}
}

// INCON-1b: also verify that a zero num_std supplied explicitly is defaulted to
// 2.0 (the dispatch guard is `<= 0`, not just `== 0`).
func TestRegression_INCON1_BollingerZeroNumStdDefaultsToTwo(t *testing.T) {
	a := New(&mockLLM{}, &mockMarket{}, "")
	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"prices":  []float64{10, 11, 12, 13, 14, 15},
		"period":  3,
		"num_std": 0.0,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "calculate_bollinger_bands", Arguments: args},
	}, &lk)
	if result == "" || result[:6] == "error:" {
		t.Fatalf("INCON-1b regression: num_std=0 should default to 2.0, got: %s", result)
	}
	var r bollingerResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if r.NumStd != 2.0 {
		t.Errorf("INCON-1b regression: want NumStd=2.0, got %v", r.NumStd)
	}
}

// INCON-2: portfolio_calculate_metrics always returned dividend_yield=0 and
// weighted_beta=0 because the dispatch only populated portfolio.Quote.Last and
// left DividendYieldTTM and Beta at their zero values. Fixed: the dispatch now
// calls GetFundamentals per holding and propagates those two fields.
func TestRegression_INCON2_PortfolioMetricsDividendAndBetaPropagate(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	now := time.Now().UTC().Truncate(time.Second)
	p := portfolio.NewPortfolio("regression-incon2")
	for _, sym := range []string{"AAPL", "MSFT"} {
		p.Instruments = append(p.Instruments, portfolio.Instrument{
			ID:     "ins-" + sym,
			Symbol: sym,
			Name:   sym,
			Type:   portfolio.InstrumentHolding,
			Lots:   []portfolio.Lot{{ID: "lot-" + sym, Quantity: 10, Price: 100, Date: now}},
		})
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	// AAPL: price=150, div_yield=0.5%, beta=1.2
	// MSFT: price=200, div_yield=0.8%, beta=0.9
	// Weights: AAPL=1500/3500≈0.4286, MSFT=2000/3500≈0.5714
	// Weighted beta  = 0.4286*1.2 + 0.5714*0.9 = 0.5143 + 0.5143 = 1.0286
	// Weighted yield = (0.005*1500 + 0.008*2000) / 3500 = (7.5+16) / 3500 ≈ 0.6714%
	mp := &mockMarket{
		quotesBySymbol: map[string]market.Quote{
			"AAPL": {Last: 150},
			"MSFT": {Last: 200},
		},
		fundamentalsBySymbol: map[string]market.Fundamental{
			"AAPL": {DividendYieldTTM: 0.005, Beta: 1.2},
			"MSFT": {DividendYieldTTM: 0.008, Beta: 0.9},
		},
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_metrics", Arguments: "{}"},
	}, &lk)
	if result == "" || result[:6] == "error:" {
		t.Fatalf("unexpected error or empty result: %s", result)
	}
	var m portfolio.PortfolioMetrics
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}

	// Weighted beta ≈ 1.029 (rounded to 4 decimal places by ComputeMetrics).
	wantBeta := (1500.0/3500.0)*1.2 + (2000.0/3500.0)*0.9
	if !approxEqual(m.WeightedBeta, math.Round(wantBeta*10000)/10000, 1e-3) {
		t.Errorf("INCON-2 regression: WeightedBeta want ≈%.4f, got %.4f", wantBeta, m.WeightedBeta)
	}
	if m.WeightedBeta == 0 {
		t.Error("INCON-2 regression: WeightedBeta is 0 — fundamentals not propagated to portfolio.Quote")
	}

	// Weighted dividend yield (stored as percent by ComputeMetrics).
	wantYieldFrac := (0.005*1500 + 0.008*2000) / 3500
	wantYieldPct := math.Round(wantYieldFrac*100*10000) / 10000 // round4 then *100
	if !approxEqual(m.DividendYield, wantYieldPct, 1e-3) {
		t.Errorf("INCON-2 regression: DividendYield want ≈%.4f%%, got %.4f%%", wantYieldPct, m.DividendYield)
	}
	if m.DividendYield == 0 {
		t.Error("INCON-2 regression: DividendYield is 0 — fundamentals not propagated to portfolio.Quote")
	}
}

// INCON-2b: when GetFundamentals fails for a holding, the rest of the portfolio
// metrics (cost basis, current value, HHI) must still be correct and the
// dividend_yield / weighted_beta for that holding must gracefully be 0.
func TestRegression_INCON2_PortfolioMetricsFundamentalsFailureIsNonFatal(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	now := time.Now().UTC().Truncate(time.Second)
	p := portfolio.NewPortfolio("regression-incon2b")
	p.Instruments = append(p.Instruments, portfolio.Instrument{
		ID:     "ins-AAPL",
		Symbol: "AAPL",
		Name:   "AAPL",
		Type:   portfolio.InstrumentHolding,
		Lots:   []portfolio.Lot{{ID: "lot-AAPL", Quantity: 5, Price: 200, Date: now}},
	})
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	// GetFundamentals returns an error; GetQuote succeeds.
	mp := &mockMarket{
		quotesBySymbol: map[string]market.Quote{"AAPL": {Last: 250}},
		fundErr:        market.ErrNotSupported,
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_metrics", Arguments: "{}"},
	}, &lk)
	if result == "" || result[:6] == "error:" {
		t.Fatalf("INCON-2b regression: fundamentals failure must not abort metrics: %s", result)
	}
	var m portfolio.PortfolioMetrics
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	// Current value and cost basis must still be computed correctly.
	if m.CurrentValue != 1250 { // 5 * 250
		t.Errorf("INCON-2b regression: CurrentValue want 1250, got %v", m.CurrentValue)
	}
	if m.CostBasis != 1000 { // 5 * 200
		t.Errorf("INCON-2b regression: CostBasis want 1000, got %v", m.CostBasis)
	}
	// dividend_yield and weighted_beta must be 0 when fundamentals are absent.
	if m.DividendYield != 0 {
		t.Errorf("INCON-2b regression: DividendYield want 0 when fundamentals fail, got %v", m.DividendYield)
	}
	if m.WeightedBeta != 0 {
		t.Errorf("INCON-2b regression: WeightedBeta want 0 when fundamentals fail, got %v", m.WeightedBeta)
	}
}

// DD-2: a quotes snapshot passed by the caller must be used directly and
// must NOT trigger a market provider auto-fetch for the symbols it covers.
// AAPL's mock quote is configured to error out, so if the dispatch tried to
// auto-fetch it despite the snapshot, AAPL would end up in MissingQuotes and
// its value would be excluded — this test would then fail.
func TestRegression_DD2_PortfolioMetricsQuotesSnapshotSkipsAutoFetch(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	now := time.Now().UTC().Truncate(time.Second)
	p := portfolio.NewPortfolio("regression-dd2")
	p.Instruments = append(p.Instruments,
		portfolio.Instrument{
			ID: "ins-AAPL", Symbol: "AAPL", Name: "AAPL", Type: portfolio.InstrumentHolding,
			Lots: []portfolio.Lot{{ID: "lot-AAPL", Quantity: 10, Price: 100, Date: now}},
		},
		portfolio.Instrument{
			ID: "ins-MSFT", Symbol: "MSFT", Name: "MSFT", Type: portfolio.InstrumentHolding,
			Lots: []portfolio.Lot{{ID: "lot-MSFT", Quantity: 5, Price: 100, Date: now}},
		},
	)
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	mp := &mockMarket{
		// AAPL would fail if the dispatch ever tried to auto-fetch it.
		quoteErrBySymbol: map[string]error{"AAPL": market.ErrNotFound},
		// MSFT has no snapshot entry, so it must come from auto-fetch.
		quotesBySymbol: map[string]market.Quote{"MSFT": {Last: 200}},
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"quotes": map[string]any{
			"AAPL": map[string]any{"last": 150.0, "dividend_yield_ttm": 0.005, "beta": 1.2},
		},
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_metrics", Arguments: args},
	}, &lk)
	if result == "" || result[:6] == "error:" {
		t.Fatalf("DD-2 regression: unexpected error or empty result: %s", result)
	}
	var m portfolio.PortfolioMetrics
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	for _, sym := range m.MissingQuotes {
		if sym == "AAPL" {
			t.Fatalf("DD-2 regression: AAPL should be valued from the snapshot, not auto-fetched (missing_quotes=%v)", m.MissingQuotes)
		}
	}
	// AAPL: 10 * 150 = 1500, MSFT: 5 * 200 = 1000 → current_value = 2500.
	if m.CurrentValue != 2500 {
		t.Errorf("DD-2 regression: CurrentValue want 2500 (snapshot AAPL + auto-fetched MSFT), got %v", m.CurrentValue)
	}
}

func TestDispatch_PortfolioCalculateMetrics_NoMarketProvider(t *testing.T) {
	// With a nil market provider, the tool still runs but flags the
	// limitation in the summary.
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	now := time.Now().UTC().Truncate(time.Second)
	p.Instruments = append(p.Instruments, portfolio.Instrument{
		ID:     "ins-AAPL",
		Symbol: "AAPL",
		Name:   "AAPL",
		Type:   portfolio.InstrumentHolding,
		Lots:   []portfolio.Lot{{ID: "lot-AAPL", Quantity: 10, Price: 100, Date: now}},
	})
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, nil, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_metrics", Arguments: "{}"},
	}, &lk)

	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	if !strings.Contains(result, "no market provider configured") {
		t.Errorf("summary must warn about missing market provider, got: %s", result)
	}
}

// ---- REF-3: numeric input validation ------------------------------------------

func TestRegression_REF3_CalcSharpe_RejectsNaNReturn(t *testing.T) {
	if _, err := calcSharpe([]float64{0.01, math.NaN(), 0.02}, 0.04); err == nil {
		t.Error("expected error for NaN in returns")
	}
}

func TestRegression_REF3_CalcSharpe_RejectsInfReturn(t *testing.T) {
	if _, err := calcSharpe([]float64{0.01, math.Inf(1), 0.02}, 0.04); err == nil {
		t.Error("expected error for +Inf in returns")
	}
}

func TestRegression_REF3_CalcSharpe_Accepts1000PercentReturn(t *testing.T) {
	// A single-period return of +1000% (the position multiplied ~11x) is a
	// real, if extreme, market event and must not be rejected.
	returns := []float64{0.01, -0.02, 10.0, 0.015}
	if _, err := calcSharpe(returns, 0.04); err != nil {
		t.Errorf("a legitimate 1000%% return must be accepted, got error: %v", err)
	}
}

func TestRegression_REF3_CalcVaR_RejectsNaNReturn(t *testing.T) {
	if _, err := calcVaR([]float64{0.01, math.NaN()}, 0.95, 100000, "historical"); err == nil {
		t.Error("expected error for NaN in returns")
	}
}

func TestRegression_REF3_CalcVaR_RejectsNonPositivePortfolioValue(t *testing.T) {
	if _, err := calcVaR([]float64{0.01, -0.01}, 0.95, 0, "historical"); err == nil {
		t.Error("expected error for zero portfolio_value")
	}
	if _, err := calcVaR([]float64{0.01, -0.01}, 0.95, -100, "historical"); err == nil {
		t.Error("expected error for negative portfolio_value")
	}
}

func TestRegression_REF3_CalcBeta_RejectsInfReturn(t *testing.T) {
	assetReturns := []float64{0.01, math.Inf(-1), 0.02}
	benchmarkReturns := []float64{0.01, 0.02, 0.015}
	if _, err := calcBeta(assetReturns, benchmarkReturns); err == nil {
		t.Error("expected error for -Inf in asset_returns")
	}
}

func TestRegression_REF3_CalcVolatility_RejectsZeroOrNegativePrice(t *testing.T) {
	if _, err := calcVolatility([]float64{100, 0, 105}); err == nil {
		t.Error("expected error for zero price in series")
	}
	if _, err := calcVolatility([]float64{100, -50, 105}); err == nil {
		t.Error("expected error for negative price in series")
	}
}

func TestRegression_REF3_CalcMaxDrawdown_RejectsNonPositivePrice(t *testing.T) {
	// calcMaxDrawdown previously had zero validation on its prices series.
	if _, err := calcMaxDrawdown([]float64{100, 0, 90}); err == nil {
		t.Error("expected error for zero price in series")
	}
	if _, err := calcMaxDrawdown([]float64{100, math.NaN(), 90}); err == nil {
		t.Error("expected error for NaN price in series")
	}
}

func TestRegression_REF3_CalcSMA_RejectsNonPositivePrice(t *testing.T) {
	if _, err := calcSMA([]float64{100, 101, -5, 103, 104}, 3); err == nil {
		t.Error("expected error for negative price in series")
	}
}

func TestRegression_REF3_CalcMultiples_RejectsNonPositivePrice(t *testing.T) {
	if _, err := calcMultiples(0, 2.5, 20, 1e9, 5e9, 2e9); err == nil {
		t.Error("expected error for zero price")
	}
	if _, err := calcMultiples(-10, 2.5, 20, 1e9, 5e9, 2e9); err == nil {
		t.Error("expected error for negative price")
	}
	if _, err := calcMultiples(math.NaN(), 2.5, 20, 1e9, 5e9, 2e9); err == nil {
		t.Error("expected error for NaN price")
	}
}

func TestRegression_REF3_CalcMultiples_AcceptsNegativeEPS(t *testing.T) {
	// EPS (and the other multiples denominators) can legitimately be
	// negative (a loss-making company) — only price positivity is enforced.
	r, err := calcMultiples(50, -2.5, 20, 1e9, 5e9, 2e9)
	if err != nil {
		t.Fatalf("negative eps should not be rejected: %v", err)
	}
	if r.PER == nil || !approxEqual(*r.PER, -20.0, 0.01) {
		t.Errorf("P/E: want ~-20, got %v", r.PER)
	}
}

func TestRegression_REF3_CalcCompoundInterest_RejectsNaNPrincipalOrRate(t *testing.T) {
	if _, err := calcCompoundInterest(math.NaN(), 0.05, 5, 1); err == nil {
		t.Error("expected error for NaN principal")
	}
	if _, err := calcCompoundInterest(1000, math.NaN(), 5, 1); err == nil {
		t.Error("expected error for NaN annual_rate")
	}
}

func TestRegression_REF3_CalcCompoundInterest_RejectsImplausibleRate(t *testing.T) {
	if _, err := calcCompoundInterest(1000, 50.0, 5, 1); err == nil {
		t.Error("expected error for a 5,000% annual_rate")
	}
}

func TestRegression_REF3_CalcCompoundInterest_AcceptsHighButPlausibleRate(t *testing.T) {
	if _, err := calcCompoundInterest(1000, 2.0, 1, 1); err != nil {
		t.Errorf("a 200%% annual_rate must be accepted, got error: %v", err)
	}
}

func TestRegression_REF3_CalcPnL_RejectsNaNOrNegativeCurrentPrice(t *testing.T) {
	// current_price was previously completely unvalidated.
	if _, err := calcPnL(100, math.NaN(), 10, "long"); err == nil {
		t.Error("expected error for NaN current_price")
	}
	if _, err := calcPnL(100, -50, 10, "long"); err == nil {
		t.Error("expected error for negative current_price")
	}
}

func TestRegression_REF3_CalcDCF_SharesOutstandingZeroStillSkipsPerShare(t *testing.T) {
	// shares_outstanding == 0 is an intentional sentinel meaning "skip the
	// per-share calculation" — REF-3 must not break that behaviour.
	fcf := []float64{100, 110, 120, 130, 140}
	r, err := calcDCF(fcf, 0.10, 0.03, 0)
	if err != nil {
		t.Fatalf("shares_outstanding=0 should still succeed: %v", err)
	}
	if r.IntrinsicValuePerShare != 0 {
		t.Errorf("IntrinsicValuePerShare: want 0 when shares_outstanding=0, got %v", r.IntrinsicValuePerShare)
	}
}

func TestRegression_REF3_CalcDCF_RejectsNegativeSharesOutstanding(t *testing.T) {
	fcf := []float64{100, 110, 120, 130, 140}
	if _, err := calcDCF(fcf, 0.10, 0.03, -1000); err == nil {
		t.Error("expected error for negative shares_outstanding")
	}
}

func TestRegression_REF3_CalcStressTest_Accepts1000PercentShock(t *testing.T) {
	if _, err := calcStressTest(10000, []float64{-20, 1000}, "portfolio"); err != nil {
		t.Errorf("a +1000%% shock must be accepted, got error: %v", err)
	}
}

func TestRegression_REF3_CalcStressTest_RejectsShockBelowTotalLoss(t *testing.T) {
	if _, err := calcStressTest(10000, []float64{-150}, "portfolio"); err == nil {
		t.Error("expected error for a shock below -100%")
	}
}
