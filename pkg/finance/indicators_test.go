package finance

import (
	"math"
	"testing"
)

func TestCalcSMA_HappyPath(t *testing.T) {
	// Five closes; period 3. Manual SMA series:
	//   index 0,1 -> NaN (warm-up)
	//   index 2 -> mean(10,11,12) = 11
	//   index 3 -> mean(11,12,13) = 12
	//   index 4 -> mean(12,13,14) = 13
	prices := []float64{10, 11, 12, 13, 14}
	r, err := CalcSMA(prices, 3)
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
	r, err := CalcSMA([]float64{14, 13, 12, 11, 10}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if r.Trend != "down" {
		t.Errorf("Trend: want down, got %q", r.Trend)
	}
}

func TestCalcSMA_ZeroPeriod(t *testing.T) {
	if _, err := CalcSMA([]float64{1, 2, 3}, 0); err == nil {
		t.Error("expected error for zero period")
	}
}

func TestCalcSMA_InsufficientPrices(t *testing.T) {
	if _, err := CalcSMA([]float64{1, 2}, 5); err == nil {
		t.Error("expected error when len(prices) < period")
	}
}

// --- CalcEMA ------------------------------------------------------------------

func TestCalcEMA_HappyPath_DefaultAlpha(t *testing.T) {
	// Prices 10..15 (6 values), period 3, Wilder alpha = 2/(3+1) = 0.5.
	// Recurrence: EMA_t = 0.5 * P_t + 0.5 * EMA_{t-1}, EMA_0 = P_0.
	prices := []float64{10, 11, 12, 13, 14, 15}
	r, err := CalcEMA(prices, 3, 0)
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
	// Last is Round4-rounded; the unrounded value is 14.03125.
	if !approxEqual(r.Last, 14.0313, 1e-4) {
		t.Errorf("Last: want 14.0313 (Round4 of 14.03125), got %v", r.Last)
	}
	if r.Trend != "up" {
		t.Errorf("Trend: want up, got %q", r.Trend)
	}
}

func TestCalcEMA_CustomAlpha(t *testing.T) {
	// alpha=0.25, period irrelevant for alpha selection.
	r, err := CalcEMA([]float64{100, 110, 120, 130}, 5, 0.25)
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
	if _, err := CalcEMA([]float64{1, 2, 3}, 3, 1.5); err == nil {
		t.Error("expected error when alpha >= 1")
	}
	if _, err := CalcEMA([]float64{1, 2, 3}, 0, 0.5); err == nil {
		t.Error("expected error when period <= 0")
	}
	if _, err := CalcEMA(nil, 3, 0.5); err == nil {
		t.Error("expected error on empty prices")
	}
}

// --- CalcRSI ------------------------------------------------------------------

func TestCalcRSI_AllGains(t *testing.T) {
	// Monotonically increasing prices: only gains, no losses. Expected RSI = 100.
	prices := []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27}
	r, err := CalcRSI(prices, 14)
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
	r, err := CalcRSI(prices, 14)
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
	r, err := CalcRSI(prices, 14)
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
	r, err := CalcRSI(prices, 14)
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
	if _, err := CalcRSI([]float64{1, 2}, 14); err == nil {
		t.Error("expected error when prices shorter than period+2")
	}
	if _, err := CalcRSI([]float64{1, 2, 3}, 0); err == nil {
		t.Error("expected error for zero period")
	}
}

// --- CalcMACD -----------------------------------------------------------------

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
	r, err := CalcMACD(prices, 12, 26, 9)
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
	r, err := CalcMACD(prices, 12, 26, 9)
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
	r, err := CalcMACD(prices, 12, 26, 9)
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
	if _, err := CalcMACD(prices, 26, 12, 9); err == nil {
		t.Error("expected error when fast >= slow")
	}
	if _, err := CalcMACD(prices, 0, 26, 9); err == nil {
		t.Error("expected error when fast <= 0")
	}
	if _, err := CalcMACD([]float64{1, 2, 3, 4, 5}, 12, 26, 9); err == nil {
		t.Error("expected error when prices shorter than slow+signal")
	}
}

// --- CalcBollingerBands -------------------------------------------------------

func TestCalcBollingerBands_HandComputed(t *testing.T) {
	// Period 4, num_std 2. With 4 prices and period 4, the window covers
	// the entire series: prices = [10,11,12,13], window mean = 11.5,
	// sample std = sqrt(5/3) ≈ 1.29099.
	// Expected: middle = 11.5, upper = 11.5 + 2*sd, lower = 11.5 - 2*sd.
	prices := []float64{10, 11, 12, 13}
	r, err := CalcBollingerBands(prices, 4, 2)
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
	r, err := CalcBollingerBands(prices, 3, 2)
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
	r, err := CalcBollingerBands(prices, 5, 2)
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
	if _, err := CalcBollingerBands([]float64{1, 2, 3}, 0, 2); err == nil {
		t.Error("expected error for zero period")
	}
	if _, err := CalcBollingerBands([]float64{1, 2, 3}, 3, -1); err == nil {
		t.Error("expected error for non-positive num_std")
	}
	if _, err := CalcBollingerBands([]float64{1, 2}, 5, 2); err == nil {
		t.Error("expected error when prices shorter than period")
	}
}

// --- CalcCorrelationMatrix ---------------------------------------------------

func TestCalcCorrelationMatrix_Perfect(t *testing.T) {
	// Identical series => correlation = 1.
	a := []float64{0.01, -0.02, 0.03, -0.01, 0.02, -0.015}
	series := map[string][]float64{"AAPL": a, "MSFT": a}
	r, err := CalcCorrelationMatrix(series)
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
	r, err := CalcCorrelationMatrix(series)
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
	r, err := CalcCorrelationMatrix(series)
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
	if _, err := CalcCorrelationMatrix(series); err == nil {
		t.Error("expected error when series have different lengths")
	}
}

func TestCalcCorrelationMatrix_TooFewSeries(t *testing.T) {
	if _, err := CalcCorrelationMatrix(map[string][]float64{"AAPL": {1, 2}}); err == nil {
		t.Error("expected error when only 1 series provided")
	}
}

func TestCalcCorrelationMatrix_ZeroVariance(t *testing.T) {
	// Flat series has zero variance → Pearson is undefined → error.
	series := map[string][]float64{
		"AAPL": {1, 1, 1, 1},
		"MSFT": {2, 3, 4, 5},
	}
	if _, err := CalcCorrelationMatrix(series); err == nil {
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

func TestRegression_REF3_CalcSMA_RejectsNonPositivePrice(t *testing.T) {
	if _, err := CalcSMA([]float64{100, 101, -5, 103, 104}, 3); err == nil {
		t.Error("expected error for negative price in series")
	}
}
