package finance

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// smaResult holds the simple moving average series and headline figures.
//
// Values are aligned to the input: the first len(prices)-period+1 entries of
// Values are non-nil; the rest are NaN so callers can detect warm-up.
type SmaResult struct {
	Period        int        `json:"period"`
	Values        FloatSlice `json:"values"`
	Last          float64    `json:"last"`
	Previous      Float      `json:"previous"` // null during warm-up (< period prices)
	Trend         string     `json:"trend"`    // "up", "down", or "flat"
	Summary       string     `json:"summary"`
	InputSize     int        `json:"input_size"`
	InputsSummary string     `json:"inputs_summary"`
	ComputedAt    string     `json:"computed_at"`
}

// CalcSMA computes the simple moving average of closing prices over a sliding
// window of length period. The first period-1 entries of the returned series
// are NaN to reflect the warm-up period.
func CalcSMA(prices []float64, period int) (SmaResult, error) {
	if err := ValidatePositiveAll("prices", prices); err != nil {
		return SmaResult{}, err
	}
	values, err := SMA(prices, period)
	if err != nil {
		return SmaResult{}, err
	}
	last := values[len(values)-1]
	previous := math.NaN()
	trend := "flat"
	if len(values) >= 2 {
		previous = values[len(values)-2]
		switch {
		case last > previous:
			trend = "up"
		case last < previous:
			trend = "down"
		}
	}
	return SmaResult{
		Period:    period,
		Values:    values, // []float64 → FloatSlice (same underlying type)
		Last:      Round4(last),
		Previous:  Float(Round4(previous)),
		Trend:     trend,
		InputSize: len(prices),
		Summary:   fmt.Sprintf("SMA(%d) last=%.4f (prev=%.4f, trend=%s) over %d prices", period, last, previous, trend, len(prices)),
	}, nil
}

// emaResult holds the exponential moving average series and headline figures.
//
// Values are aligned to the input: the first len(prices) entries are valid
// because EMA seeds with the first observation, unlike SMA.
type EmaResult struct {
	Period        int       `json:"period"`
	Alpha         float64   `json:"alpha"`
	Values        []float64 `json:"values"`
	Last          float64   `json:"last"`
	Previous      Float     `json:"previous"` // null when only 1 price provided
	Trend         string    `json:"trend"`
	Summary       string    `json:"summary"`
	InputSize     int       `json:"input_size"`
	InputsSummary string    `json:"inputs_summary"`
	ComputedAt    string    `json:"computed_at"`
}

// CalcEMA computes the exponential moving average of prices using the recursive
// formula EMA_t = alpha * P_t + (1-alpha) * EMA_{t-1}, seeded with the first
// observation. If alpha is zero or negative, it defaults to 2 / (period + 1),
// the standard "Wilder smoothing" convention.
func CalcEMA(prices []float64, period int, alpha float64) (EmaResult, error) {
	if period <= 0 {
		return EmaResult{}, fmt.Errorf("period must be greater than zero, got %d", period)
	}
	if len(prices) == 0 {
		return EmaResult{}, errors.New("prices must not be empty")
	}
	if err := ValidatePositiveAll("prices", prices); err != nil {
		return EmaResult{}, err
	}
	if err := ValidateFinite("alpha", alpha); err != nil {
		return EmaResult{}, err
	}
	if alpha <= 0 {
		alpha = 2.0 / (float64(period) + 1)
	}
	if alpha >= 1 {
		return EmaResult{}, fmt.Errorf("alpha must be less than 1, got %.4f", alpha)
	}
	values := make([]float64, len(prices))
	values[0] = prices[0]
	for i := 1; i < len(prices); i++ {
		values[i] = alpha*prices[i] + (1-alpha)*values[i-1]
	}
	last := values[len(values)-1]
	previous := math.NaN()
	trend := "flat"
	if len(values) >= 2 {
		previous = values[len(values)-2]
		switch {
		case last > previous:
			trend = "up"
		case last < previous:
			trend = "down"
		}
	}
	return EmaResult{
		Period:    period,
		Alpha:     Round4(alpha),
		Values:    roundSlice(values, 6),
		Last:      Round4(last),
		Previous:  Float(Round4(previous)),
		Trend:     trend,
		InputSize: len(prices),
		Summary:   fmt.Sprintf("EMA(%d, alpha=%.4f) last=%.4f (prev=%.4f, trend=%s) over %d prices", period, alpha, last, previous, trend, len(prices)),
	}, nil
}

// rsiResult holds the Relative Strength Index (Wilder) and its interpretation.
type RsiResult struct {
	Period         int        `json:"period"`
	Value          float64    `json:"value"`
	PreviousValue  Float      `json:"previous_value"` // null at minimum input size (period+2 prices)
	Interpretation string     `json:"interpretation"` // "oversold", "neutral", "overbought"
	Values         FloatSlice `json:"values"`
	Summary        string     `json:"summary"`
	InputSize      int        `json:"input_size"`
	InputsSummary  string     `json:"inputs_summary"`
	ComputedAt     string     `json:"computed_at"`
}

// CalcRSI computes the Relative Strength Index using Wilder's smoothing
// (equivalent to an EMA with alpha = 1/period). Interpretation:
//   - value < 30 → "oversold"
//   - value > 70 → "overbought"
//   - otherwise → "neutral"
func CalcRSI(prices []float64, period int) (RsiResult, error) {
	if period <= 0 {
		return RsiResult{}, fmt.Errorf("period must be greater than zero, got %d", period)
	}
	// Need at least period+1 prices to compute period changes, then a second
	// value to populate PreviousValue.
	if len(prices) < period+2 {
		return RsiResult{}, fmt.Errorf("at least %d prices required for RSI(%d), got %d", period+2, period, len(prices))
	}
	if err := ValidatePositiveAll("prices", prices); err != nil {
		return RsiResult{}, err
	}
	changes := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		changes[i-1] = prices[i] - prices[i-1]
	}
	// Wilder's smoothing uses SMA for the first average, then runs an EMA.
	var gain, loss float64
	for i := 0; i < period; i++ {
		if changes[i] > 0 {
			gain += changes[i]
		} else {
			loss -= changes[i]
		}
	}
	avgGain := gain / float64(period)
	avgLoss := loss / float64(period)
	values := make([]float64, len(changes))
	// The first index with a valid RSI is `period` (after `period` changes).
	// Earlier slots are NaN to mirror common charting libraries.
	for i := range values {
		values[i] = math.NaN()
	}
	rsi := rsiFromAvg(avgGain, avgLoss)
	values[period] = rsi
	for i := period + 1; i < len(changes); i++ {
		ch := changes[i]
		g, l := 0.0, 0.0
		if ch > 0 {
			g = ch
		} else {
			l = -ch
		}
		avgGain = (avgGain*float64(period-1) + g) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + l) / float64(period)
		values[i] = rsiFromAvg(avgGain, avgLoss)
	}
	last := values[len(values)-1]
	previous := math.NaN()
	if len(values) >= 2 {
		previous = values[len(values)-2]
	}
	interp := "neutral"
	switch {
	case last < 30:
		interp = "oversold"
	case last > 70:
		interp = "overbought"
	}
	return RsiResult{
		Period:         period,
		Value:          Round4(last),
		PreviousValue:  Float(Round4(previous)),
		Interpretation: interp,
		Values:         values, // []float64 → FloatSlice (same underlying type)
		InputSize:      len(prices),
		Summary:        fmt.Sprintf("RSI(%d)=%.2f (%s) on %d prices", period, last, interp, len(prices)),
	}, nil
}

// rsiFromAvg converts average gains/losses into an RSI value in [0, 100].
// When avgLoss is zero and avgGain is also zero, the price has not moved: RSI is undefined
// and we return 50 as the neutral midpoint.
func rsiFromAvg(avgGain, avgLoss float64) float64 {
	if avgLoss == 0 {
		if avgGain == 0 {
			return 50
		}
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

// macdResult holds the MACD line, signal line, and histogram series.
type MacdResult struct {
	FastPeriod    int       `json:"fast_period"`
	SlowPeriod    int       `json:"slow_period"`
	SignalPeriod  int       `json:"signal_period"`
	MACDLine      []float64 `json:"macd_line"`
	SignalLine    []float64 `json:"signal_line"`
	Histogram     []float64 `json:"histogram"`
	LastMACD      float64   `json:"last_macd"`
	LastSignal    float64   `json:"last_signal"`
	LastHist      float64   `json:"last_hist"`
	Trend         string    `json:"trend"` // "bullish_cross", "bearish_cross", or "no_cross"
	Summary       string    `json:"summary"`
	InputSize     int       `json:"input_size"`
	InputsSummary string    `json:"inputs_summary"`
	ComputedAt    string    `json:"computed_at"`
}

// CalcMACD computes the Moving Average Convergence Divergence indicator.
//
// The MACD line is the difference between a fast EMA and a slow EMA of prices.
// The signal line is an EMA of the MACD line itself. The histogram is the
// difference between MACD and signal. A "bullish_cross" is reported when the
// histogram flipped from negative to non-negative on the latest bar; a
// "bearish_cross" is the opposite. If fast ≥ slow the function returns an
// error because the indicator is undefined.
func CalcMACD(prices []float64, fastPeriod, slowPeriod, signalPeriod int) (MacdResult, error) {
	if fastPeriod <= 0 || slowPeriod <= 0 || signalPeriod <= 0 {
		return MacdResult{}, fmt.Errorf("fast_period, slow_period, signal_period must all be positive (got %d, %d, %d)", fastPeriod, slowPeriod, signalPeriod)
	}
	if fastPeriod >= slowPeriod {
		return MacdResult{}, fmt.Errorf("fast_period (%d) must be less than slow_period (%d)", fastPeriod, slowPeriod)
	}
	// Need slowPeriod observations to seed both EMAs and signalPeriod more for the signal.
	if len(prices) < slowPeriod+signalPeriod {
		return MacdResult{}, fmt.Errorf("at least %d prices required for MACD(%d,%d,%d), got %d",
			slowPeriod+signalPeriod, fastPeriod, slowPeriod, signalPeriod, len(prices))
	}
	if err := ValidatePositiveAll("prices", prices); err != nil {
		return MacdResult{}, err
	}
	fastEMA := emaSeries(prices, fastPeriod)
	slowEMA := emaSeries(prices, slowPeriod)
	macdLine := make([]float64, len(prices))
	for i := range prices {
		macdLine[i] = fastEMA[i] - slowEMA[i]
	}
	signalLine := emaSeries(macdLine, signalPeriod)
	histogram := make([]float64, len(prices))
	for i := range prices {
		histogram[i] = macdLine[i] - signalLine[i]
	}
	trend := "no_cross"
	if len(histogram) >= 2 {
		prev := histogram[len(histogram)-2]
		last := histogram[len(histogram)-1]
		switch {
		case prev < 0 && last >= 0:
			trend = "bullish_cross"
		case prev > 0 && last <= 0:
			trend = "bearish_cross"
		}
	}
	return MacdResult{
		FastPeriod:   fastPeriod,
		SlowPeriod:   slowPeriod,
		SignalPeriod: signalPeriod,
		MACDLine:     roundSlice(macdLine, 6),
		SignalLine:   roundSlice(signalLine, 6),
		Histogram:    roundSlice(histogram, 6),
		LastMACD:     Round4(macdLine[len(macdLine)-1]),
		LastSignal:   Round4(signalLine[len(signalLine)-1]),
		LastHist:     Round4(histogram[len(histogram)-1]),
		Trend:        trend,
		InputSize:    len(prices),
		Summary: fmt.Sprintf("MACD(%d,%d,%d): macd=%.4f, signal=%.4f, hist=%.4f, trend=%s",
			fastPeriod, slowPeriod, signalPeriod,
			macdLine[len(macdLine)-1], signalLine[len(signalLine)-1], histogram[len(histogram)-1], trend),
	}, nil
}

// bollingerResult holds Bollinger Band output for a price series.
type BollingerResult struct {
	Period        int        `json:"period"`
	NumStd        float64    `json:"num_std"`
	Upper         FloatSlice `json:"upper"`
	Middle        FloatSlice `json:"middle"`
	Lower         FloatSlice `json:"lower"`
	Bandwidth     FloatSlice `json:"bandwidth"` // (upper - lower) / middle
	PercentB      FloatSlice `json:"percent_b"` // (price - lower) / (upper - lower)
	LastPrice     float64    `json:"last_price"`
	LastUpper     float64    `json:"last_upper"`
	LastLower     float64    `json:"last_lower"`
	LastPctB      float64    `json:"last_percent_b"`
	Summary       string     `json:"summary"`
	InputSize     int        `json:"input_size"`
	InputsSummary string     `json:"inputs_summary"`
	ComputedAt    string     `json:"computed_at"`
}

// CalcBollingerBands computes Bollinger Bands (moving average ± k·σ) for a
// price series. Returns upper/middle/lower/bandwidth/%b series, each entry
// aligned to prices (NaN during the warm-up period).
func CalcBollingerBands(prices []float64, period int, numStd float64) (BollingerResult, error) {
	if period <= 0 {
		return BollingerResult{}, fmt.Errorf("period must be greater than zero, got %d", period)
	}
	if err := ValidateFinite("num_std", numStd); err != nil {
		return BollingerResult{}, err
	}
	if numStd <= 0 {
		return BollingerResult{}, fmt.Errorf("num_std must be positive, got %.4f", numStd)
	}
	if len(prices) < period {
		return BollingerResult{}, fmt.Errorf("at least %d prices required for Bollinger(%d), got %d", period, period, len(prices))
	}
	if err := ValidatePositiveAll("prices", prices); err != nil {
		return BollingerResult{}, err
	}
	upper := make([]float64, len(prices))
	middle := make([]float64, len(prices))
	lower := make([]float64, len(prices))
	bandwidth := make([]float64, len(prices))
	pctB := make([]float64, len(prices))
	for i := range prices {
		upper[i] = math.NaN()
		middle[i] = math.NaN()
		lower[i] = math.NaN()
		bandwidth[i] = math.NaN()
		pctB[i] = math.NaN()
	}
	for i := period - 1; i < len(prices); i++ {
		window := prices[i-period+1 : i+1]
		m := meanFloat(window)
		sd := sampleStddev(window)
		upper[i] = m + numStd*sd
		middle[i] = m
		lower[i] = m - numStd*sd
		if m != 0 {
			bandwidth[i] = (upper[i] - lower[i]) / m
		}
		span := upper[i] - lower[i]
		if span != 0 {
			pctB[i] = (prices[i] - lower[i]) / span
		}
	}
	last := len(prices) - 1
	summary := fmt.Sprintf("Bollinger(%d, %.2fσ) last: price=%.4f upper=%.4f middle=%.4f lower=%.4f %%b=%.4f",
		period, numStd, prices[last], upper[last], middle[last], lower[last], pctB[last])
	return BollingerResult{
		Period:    period,
		NumStd:    Round4(numStd),
		Upper:     upper, // []float64 → FloatSlice (same underlying type)
		Middle:    middle,
		Lower:     lower,
		Bandwidth: bandwidth,
		PercentB:  pctB,
		LastPrice: Round4(prices[last]),
		LastUpper: Round4(upper[last]),
		LastLower: Round4(lower[last]),
		LastPctB:  Round4(pctB[last]),
		InputSize: len(prices),
		Summary:   summary,
	}, nil
}

// correlationMatrixResult holds an NxN Pearson correlation matrix between
// named return series, plus the diagonal (always 1) and labels for downstream
// rendering.
type CorrelationMatrixResult struct {
	Labels        []string    `json:"labels"`
	Matrix        [][]float64 `json:"matrix"`
	Scale         string      `json:"scale"` // "[-1, 1]"
	Summary       string      `json:"summary"`
	InputsSummary string      `json:"inputs_summary"`
	ComputedAt    string      `json:"computed_at"`
}

// CalcCorrelationMatrix computes the Pearson correlation between every pair
// of the provided series. Each series must be the same length (typical usage:
// daily returns of N assets).
func CalcCorrelationMatrix(series map[string][]float64) (CorrelationMatrixResult, error) {
	if len(series) < 2 {
		return CorrelationMatrixResult{}, fmt.Errorf("at least 2 series required, got %d", len(series))
	}
	// Stable iteration order for deterministic output (sorted by key).
	keys := make([]string, 0, len(series))
	for k := range series {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Validate equal lengths and ≥2 observations.
	n := -1
	for _, k := range keys {
		l := len(series[k])
		if n == -1 {
			n = l
			continue
		}
		if l != n {
			return CorrelationMatrixResult{}, fmt.Errorf("series %q has length %d, expected %d", k, l, n)
		}
	}
	if n < 2 {
		return CorrelationMatrixResult{}, fmt.Errorf("each series must have at least 2 observations, got %d", n)
	}
	for _, k := range keys {
		if err := ValidateFiniteAll(fmt.Sprintf("series[%q]", k), series[k]); err != nil {
			return CorrelationMatrixResult{}, err
		}
	}
	matrix := make([][]float64, len(keys))
	for i := range matrix {
		matrix[i] = make([]float64, len(keys))
	}
	for i, ki := range keys {
		for j, kj := range keys {
			if i == j {
				matrix[i][j] = 1
				continue
			}
			if j < i {
				// Already computed; mirror.
				matrix[i][j] = matrix[j][i]
				continue
			}
			c, err := pearson(series[ki], series[kj])
			if err != nil {
				return CorrelationMatrixResult{}, fmt.Errorf("%s vs %s: %w", ki, kj, err)
			}
			matrix[i][j] = Round4(c)
		}
	}
	return CorrelationMatrixResult{
		Labels:  keys,
		Matrix:  matrix,
		Scale:   "[-1, 1]",
		Summary: fmt.Sprintf("Pearson correlation matrix across %d series, %d observations each", len(keys), n),
	}, nil
}

// pearson returns the Pearson product-moment correlation coefficient between
// two equal-length series. Returns an error when either series has zero variance.
func pearson(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("series length mismatch: %d vs %d", len(a), len(b))
	}
	n := len(a)
	if n < 2 {
		return 0, errors.New("need at least 2 observations")
	}
	meanA := meanFloat(a)
	meanB := meanFloat(b)
	var cov, varA, varB float64
	for i := 0; i < n; i++ {
		da := a[i] - meanA
		db := b[i] - meanB
		cov += da * db
		varA += da * da
		varB += db * db
	}
	if varA == 0 || varB == 0 {
		return 0, errors.New("zero variance in one of the series")
	}
	return cov / math.Sqrt(varA*varB), nil
}

// emaSeries returns the EMA series for the entire price array using Wilder
// smoothing (alpha = 2 / (period + 1)). The first value is seeded with the
// first observation. Exported only through CalcEMA/CalcMACD; kept unexported
// because it does no input validation.
func emaSeries(prices []float64, period int) []float64 {
	alpha := 2.0 / (float64(period) + 1)
	out := make([]float64, len(prices))
	out[0] = prices[0]
	for i := 1; i < len(prices); i++ {
		out[i] = alpha*prices[i] + (1-alpha)*out[i-1]
	}
	return out
}

// roundSlice returns a new slice with every element rounded to `decimals`
// decimal places. Used to keep the indicator series compact in JSON output.
func roundSlice(in []float64, decimals int) []float64 {
	out := make([]float64, len(in))
	mult := math.Pow(10, float64(decimals))
	for i, v := range in {
		out[i] = math.Round(v*mult) / mult
	}
	return out
}
