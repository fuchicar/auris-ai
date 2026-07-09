package finance

import (
	"math"
	"strings"
	"testing"
)

func TestCalcROI_Profit(t *testing.T) {
	r, err := CalcROI(100, 120)
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
	r, err := CalcROI(100, 80)
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
	_, err := CalcROI(0, 100)
	if err == nil {
		t.Error("expected error for zero cost_basis")
	}
}

// ---- CalcCAGR ----------------------------------------------------------------

func TestCalcCAGR_Normal(t *testing.T) {
	// 1000 → 2000 in 5 years ≈ 14.87% CAGR
	r, err := CalcCAGR(1000, 2000, 5)
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
	_, err := CalcCAGR(1000, 2000, 0)
	if err == nil {
		t.Error("expected error for years=0")
	}
}

func TestCalcCAGR_NegativeInitial(t *testing.T) {
	_, err := CalcCAGR(-500, 2000, 5)
	if err == nil {
		t.Error("expected error for initial_value <= 0")
	}
}

// ---- CalcVolatility ----------------------------------------------------------

func TestCalcPnL_LongProfit(t *testing.T) {
	r, err := CalcPnL(100, 110, 10, "long")
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
	r, err := CalcPnL(100, 90, 10, "short")
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
	r, err := CalcPnL(100, 90, 10, "long")
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
	_, err := CalcPnL(100, 110, 10, "buy")
	if err == nil {
		t.Error("expected error for invalid position_type")
	}
}

func TestCalcPnL_ZeroEntryPrice(t *testing.T) {
	_, err := CalcPnL(0, 110, 10, "long")
	if err == nil {
		t.Error("expected error for zero entry_price")
	}
}

// ---- CalcBeta ----------------------------------------------------------------

func TestCalcDCF_Normal(t *testing.T) {
	// 5 years of FCF = 100 each, discount = 10%, terminal growth = 3%.
	fcf := []float64{100, 100, 100, 100, 100}
	r, err := CalcDCF(fcf, 0.10, 0.03, 1000)
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
	_, err := CalcDCF([]float64{100, 100}, 0.03, 0.05, 1000)
	if err == nil {
		t.Error("expected error when discount_rate <= terminal_growth_rate")
	}
}

func TestCalcDCF_ZeroShares(t *testing.T) {
	r, err := CalcDCF([]float64{100, 100}, 0.10, 0.03, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.IntrinsicValuePerShare != 0 {
		t.Errorf("expected 0 per-share value when shares_outstanding=0, got %v", r.IntrinsicValuePerShare)
	}
}

// ---- CalcMultiples -----------------------------------------------------------

func TestCalcMultiples_AllValid(t *testing.T) {
	r, err := CalcMultiples(50, 2.5, 20, 1e9, 5e9, 2e9)
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
	r, err := CalcMultiples(50, 0, 0, 0, 1e9, 0)
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

// ---- CalcPFCF ------------------------------------------------------------------

func TestCalcPFCF_Normal(t *testing.T) {
	r, err := CalcPFCF(150, 12.5, "USD")
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
	_, err := CalcPFCF(150, 0, "")
	if err == nil {
		t.Error("expected error for free_cash_flow_per_share <= 0")
	}
}

func TestCalcPFCF_ZeroPrice(t *testing.T) {
	_, err := CalcPFCF(0, 12.5, "")
	if err == nil {
		t.Error("expected error for price <= 0")
	}
}

// ---- CalcPEG -------------------------------------------------------------------

func TestCalcPEG_Undervalued(t *testing.T) {
	r, err := CalcPEG(15, 20)
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
	r, err := CalcPEG(30, 5)
	if err != nil {
		t.Fatal(err)
	}
	if r.Interpretation != "overvalued" {
		t.Errorf("Interpretation: want overvalued, got %q", r.Interpretation)
	}
}

func TestCalcPEG_Reasonable(t *testing.T) {
	r, err := CalcPEG(20, 15)
	if err != nil {
		t.Fatal(err)
	}
	if r.Interpretation != "reasonable" {
		t.Errorf("Interpretation: want reasonable, got %q", r.Interpretation)
	}
}

func TestCalcPEG_NegativeGrowth_WarnsInSummary(t *testing.T) {
	r, err := CalcPEG(20, -10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Summary, "WARNING") {
		t.Errorf("summary should warn about negative growth, got %q", r.Summary)
	}
}

func TestCalcPEG_ZeroPE(t *testing.T) {
	_, err := CalcPEG(0, 15)
	if err == nil {
		t.Error("expected error for pe_ratio <= 0")
	}
}

func TestCalcPEG_ZeroGrowth(t *testing.T) {
	_, err := CalcPEG(20, 0)
	if err == nil {
		t.Error("expected error for growth_rate_percent == 0")
	}
}

// ---- CalcDividendYield -----------------------------------------------------------

func TestCalcDividendYield_AnnualDividend(t *testing.T) {
	r, err := CalcDividendYield(100, 2.5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.YieldPercent, 2.5, 0.001) {
		t.Errorf("YieldPercent: want 2.5, got %v", r.YieldPercent)
	}
}

func TestCalcDividendYield_QuarterlyTTM(t *testing.T) {
	// Sum of quarterly = 4.0 → yield = 4/100*100 = 4%.
	r, err := CalcDividendYield(100, 0, []float64{1, 1, 1, 1})
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
	r, err := CalcDividendYield(100, 2.5, []float64{2, 2, 2, 2})
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqual(r.YieldPercent, 8.0, 0.001) {
		t.Errorf("YieldPercent: want 8.0 (quarterly priority), got %v", r.YieldPercent)
	}
}

func TestCalcDividendYield_WrongQuarterlyLength(t *testing.T) {
	_, err := CalcDividendYield(100, 0, []float64{1, 1, 1})
	if err == nil {
		t.Error("expected error for quarterly_dividends length != 4")
	}
}

func TestCalcDividendYield_NoDividendProvided(t *testing.T) {
	_, err := CalcDividendYield(100, 0, nil)
	if err == nil {
		t.Error("expected error when neither annual_dividend_per_share nor quarterly_dividends is provided")
	}
}

func TestCalcDividendYield_ZeroPrice(t *testing.T) {
	_, err := CalcDividendYield(0, 2.5, nil)
	if err == nil {
		t.Error("expected error for price <= 0")
	}
}

// ---- CalcDividendGrowth -----------------------------------------------------------

func TestCalcDividendGrowth_Normal(t *testing.T) {
	// 1.00 → 1.21 over 2 periods = 10% CAGR.
	r, err := CalcDividendGrowth([]float64{1.00, 1.10, 1.21})
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
	_, err := CalcDividendGrowth([]float64{1.0})
	if err == nil {
		t.Error("expected error for fewer than 2 dividends")
	}
}

func TestCalcDividendGrowth_ZeroFirst(t *testing.T) {
	_, err := CalcDividendGrowth([]float64{0, 1.0})
	if err == nil {
		t.Error("expected error for dividends[0] <= 0")
	}
}

// ---- CalcStressTest -----------------------------------------------------------

// BUG-1: CalcPnL silently took math.Abs(quantity) when negative, giving the
// caller no indication that the sign was discarded.
func TestRegression_BUG1_NegativeQuantityWarning(t *testing.T) {
	r, err := CalcPnL(100, 110, -10, "long")
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

// BUG-3a: CalcDCF accepted all-negative free_cash_flows and produced a
// negative terminal value and negative intrinsic value without any error.
func TestRegression_BUG3_AllNegativeFCFsReturnsError(t *testing.T) {
	_, err := CalcDCF([]float64{-100, -200, -300}, 0.10, 0.03, 1000)
	if err == nil {
		t.Error("BUG-3 regression: expected error when all free_cash_flows are non-positive")
	}
}

// BUG-3b: when the last FCF is negative (terminal value becomes negative) but
// earlier FCFs are positive, the function should succeed but warn in the summary.
func TestRegression_BUG3_NegativeLastFCFAddsWarning(t *testing.T) {
	// FCFs: positive early years, negative terminal year.
	r, err := CalcDCF([]float64{100, 100, -50}, 0.10, 0.03, 1000)
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

func TestRegression_REF3_CalcMultiples_RejectsNonPositivePrice(t *testing.T) {
	if _, err := CalcMultiples(0, 2.5, 20, 1e9, 5e9, 2e9); err == nil {
		t.Error("expected error for zero price")
	}
	if _, err := CalcMultiples(-10, 2.5, 20, 1e9, 5e9, 2e9); err == nil {
		t.Error("expected error for negative price")
	}
	if _, err := CalcMultiples(math.NaN(), 2.5, 20, 1e9, 5e9, 2e9); err == nil {
		t.Error("expected error for NaN price")
	}
}

func TestRegression_REF3_CalcMultiples_AcceptsNegativeEPS(t *testing.T) {
	// EPS (and the other multiples denominators) can legitimately be
	// negative (a loss-making company) — only price positivity is enforced.
	r, err := CalcMultiples(50, -2.5, 20, 1e9, 5e9, 2e9)
	if err != nil {
		t.Fatalf("negative eps should not be rejected: %v", err)
	}
	if r.PER == nil || !approxEqual(*r.PER, -20.0, 0.01) {
		t.Errorf("P/E: want ~-20, got %v", r.PER)
	}
}

func TestRegression_REF3_CalcPnL_RejectsNaNOrNegativeCurrentPrice(t *testing.T) {
	// current_price was previously completely unvalidated.
	if _, err := CalcPnL(100, math.NaN(), 10, "long"); err == nil {
		t.Error("expected error for NaN current_price")
	}
	if _, err := CalcPnL(100, -50, 10, "long"); err == nil {
		t.Error("expected error for negative current_price")
	}
}

func TestRegression_REF3_CalcDCF_SharesOutstandingZeroStillSkipsPerShare(t *testing.T) {
	// shares_outstanding == 0 is an intentional sentinel meaning "skip the
	// per-share calculation" — REF-3 must not break that behaviour.
	fcf := []float64{100, 110, 120, 130, 140}
	r, err := CalcDCF(fcf, 0.10, 0.03, 0)
	if err != nil {
		t.Fatalf("shares_outstanding=0 should still succeed: %v", err)
	}
	if r.IntrinsicValuePerShare != 0 {
		t.Errorf("IntrinsicValuePerShare: want 0 when shares_outstanding=0, got %v", r.IntrinsicValuePerShare)
	}
}

func TestRegression_REF3_CalcDCF_RejectsNegativeSharesOutstanding(t *testing.T) {
	fcf := []float64{100, 110, 120, 130, 140}
	if _, err := CalcDCF(fcf, 0.10, 0.03, -1000); err == nil {
		t.Error("expected error for negative shares_outstanding")
	}
}
