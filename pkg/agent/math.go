package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	"auris/pkg/finance"
)

// --- ROI -----------------------------------------------------------------------

type roiResult struct {
	ROIPercent float64 `json:"roi_percent"`
	ProfitLoss float64 `json:"profit_loss"`
	Summary    string  `json:"summary"`
}

func calcROI(costBasis, currentValue float64) (roiResult, error) {
	if err := validateFinite("cost_basis", costBasis); err != nil {
		return roiResult{}, err
	}
	if err := validateFinite("current_value", currentValue); err != nil {
		return roiResult{}, err
	}
	if costBasis == 0 {
		return roiResult{}, errors.New("cost_basis cannot be zero")
	}
	pl := currentValue - costBasis
	roi := pl / costBasis * 100
	dir := "profit"
	if pl < 0 {
		dir = "loss"
	}
	return roiResult{
		ROIPercent: round2(roi),
		ProfitLoss: round2(pl),
		Summary:    fmt.Sprintf("%.2f%% ROI — %s of %.2f on cost basis %.2f", roi, dir, math.Abs(pl), costBasis),
	}, nil
}

// --- CAGR ----------------------------------------------------------------------

type cagrResult struct {
	CAGRPercent float64 `json:"cagr_percent"`
	Summary     string  `json:"summary"`
}

func calcCAGR(initialValue, finalValue, years float64) (cagrResult, error) {
	if err := validatePositive("initial_value", initialValue); err != nil {
		return cagrResult{}, err
	}
	if err := validateNonNegative("final_value", finalValue); err != nil {
		return cagrResult{}, err
	}
	if err := validatePositive("years", years); err != nil {
		return cagrResult{}, err
	}
	cagr := (math.Pow(finalValue/initialValue, 1/years) - 1) * 100
	return cagrResult{
		CAGRPercent: round2(cagr),
		Summary:     fmt.Sprintf("%.2f%% CAGR over %.1f years (%.2f → %.2f)", cagr, years, initialValue, finalValue),
	}, nil
}

// --- Volatility ----------------------------------------------------------------

type volatilityResult struct {
	VolatilityAnnualPercent float64 `json:"volatility_annual_percent"`
	VolatilityDailyPercent  float64 `json:"volatility_daily_percent"`
	Summary                 string  `json:"summary"`
}

func calcVolatility(prices []float64) (volatilityResult, error) {
	if len(prices) < 2 {
		return volatilityResult{}, errors.New("at least 2 prices required")
	}
	if err := validatePositiveAll("prices", prices); err != nil {
		return volatilityResult{}, err
	}
	returns := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		returns[i-1] = math.Log(prices[i] / prices[i-1])
	}
	sd := sampleStddev(returns)
	daily := sd * 100
	annual := sd * math.Sqrt(252) * 100
	return volatilityResult{
		VolatilityAnnualPercent: round2(annual),
		VolatilityDailyPercent:  round4(daily),
		Summary:                 fmt.Sprintf("Annual volatility: %.2f%% (daily: %.4f%%)", annual, daily),
	}, nil
}

// --- Sharpe --------------------------------------------------------------------

type sharpeResult struct {
	SharpeRatio                 float64 `json:"sharpe_ratio"`
	AnnualizedReturnPercent     float64 `json:"annualized_return_percent"`
	AnnualizedVolatilityPercent float64 `json:"annualized_volatility_percent"`
	Summary                     string  `json:"summary"`
}

// calcSharpe computes the annualised Sharpe ratio.
// returns: daily returns in decimal (e.g. 0.01 = 1 %).
// riskFreeAnnual: annual risk-free rate in decimal (e.g. 0.04 = 4 %).
func calcSharpe(returns []float64, riskFreeAnnual float64) (sharpeResult, error) {
	if len(returns) == 0 {
		return sharpeResult{}, errors.New("returns must not be empty")
	}
	if err := validateReturnSlice("returns", returns); err != nil {
		return sharpeResult{}, err
	}
	if err := validateRate("risk_free_rate_annual", riskFreeAnnual); err != nil {
		return sharpeResult{}, err
	}
	dailyRF := math.Pow(1+riskFreeAnnual, 1.0/252) - 1
	m := meanFloat(returns)
	sd := sampleStddev(returns)
	if sd == 0 {
		return sharpeResult{}, errors.New("volatility is zero; Sharpe ratio is undefined")
	}
	sharpe := (m - dailyRF) / sd * math.Sqrt(252)
	annReturn := (math.Pow(1+m, 252) - 1) * 100
	annVol := sd * math.Sqrt(252) * 100
	return sharpeResult{
		SharpeRatio:                 round2(sharpe),
		AnnualizedReturnPercent:     round2(annReturn),
		AnnualizedVolatilityPercent: round2(annVol),
		Summary:                     fmt.Sprintf("Sharpe ratio: %.2f (ann. gross return: %.2f%% [before subtracting risk-free rate], ann. vol: %.2f%%)", sharpe, annReturn, annVol),
	}, nil
}

// --- Sortino -------------------------------------------------------------------

type sortinoResult struct {
	SortinoRatio                       float64 `json:"sortino_ratio"`
	AnnualizedReturnPercent            float64 `json:"annualized_return_percent"`
	AnnualizedDownsideDeviationPercent float64 `json:"annualized_downside_deviation_percent"`
	Summary                            string  `json:"summary"`
}

// calcSortino computes the annualised Sortino ratio, a Sharpe variant that
// only penalises downside volatility (returns below a minimum acceptable
// return), leaving upside volatility unpenalised.
// returns: daily returns in decimal (e.g. 0.01 = 1 %).
// riskFreeAnnual: annual risk-free rate in decimal (e.g. 0.04 = 4 %), used
// both as the excess-return benchmark (like calcSharpe) and as the minimum
// acceptable return (MAR) below which a daily return counts as "downside".
func calcSortino(returns []float64, riskFreeAnnual float64) (sortinoResult, error) {
	if len(returns) == 0 {
		return sortinoResult{}, errors.New("returns must not be empty")
	}
	if err := validateReturnSlice("returns", returns); err != nil {
		return sortinoResult{}, err
	}
	if err := validateRate("risk_free_rate_annual", riskFreeAnnual); err != nil {
		return sortinoResult{}, err
	}
	dailyRF := math.Pow(1+riskFreeAnnual, 1.0/252) - 1
	m := meanFloat(returns)
	dd := downsideDeviation(returns, dailyRF)
	if dd == 0 {
		return sortinoResult{}, errors.New("downside deviation is zero; Sortino ratio is undefined")
	}
	sortino := (m - dailyRF) / dd * math.Sqrt(252)
	annReturn := (math.Pow(1+m, 252) - 1) * 100
	annDD := dd * math.Sqrt(252) * 100
	return sortinoResult{
		SortinoRatio:                       round2(sortino),
		AnnualizedReturnPercent:            round2(annReturn),
		AnnualizedDownsideDeviationPercent: round2(annDD),
		Summary:                            fmt.Sprintf("Sortino ratio: %.2f (ann. gross return: %.2f%% [before subtracting risk-free rate], ann. downside deviation: %.2f%%)", sortino, annReturn, annDD),
	}, nil
}

// downsideDeviation returns the semi-deviation of xs below target, using the
// full-population definition (Sortino & Price, 1994): the denominator is the
// total observation count N, not just the count of sub-target observations,
// and there is no Bessel correction (N-1) because target is an externally
// supplied constant, not estimated from the sample — unlike sampleStddev's
// mean, which is.
func downsideDeviation(xs []float64, target float64) float64 {
	sumSq := 0.0
	for _, x := range xs {
		if d := x - target; d < 0 {
			sumSq += d * d
		}
	}
	return math.Sqrt(sumSq / float64(len(xs)))
}

// --- Max Drawdown --------------------------------------------------------------

type maxDrawdownResult struct {
	MaxDrawdownPercent    float64 `json:"max_drawdown_percent"`
	PeakPrice             float64 `json:"peak_price"`
	TroughPrice           float64 `json:"trough_price"`
	RecoveryNeededPercent float64 `json:"recovery_needed_percent"`
	Summary               string  `json:"summary"`
}

func calcMaxDrawdown(prices []float64) (maxDrawdownResult, error) {
	if len(prices) < 2 {
		return maxDrawdownResult{}, errors.New("at least 2 prices required")
	}
	if err := validatePositiveAll("prices", prices); err != nil {
		return maxDrawdownResult{}, err
	}
	runPeak := prices[0]
	maxDD := 0.0
	ddPeak := prices[0]
	ddTrough := prices[0]

	for _, p := range prices {
		if p > runPeak {
			runPeak = p
		}
		if runPeak > 0 {
			dd := (runPeak - p) / runPeak * 100
			if dd > maxDD {
				maxDD = dd
				ddPeak = runPeak
				ddTrough = p
			}
		}
	}

	recovery := 0.0
	if ddTrough > 0 {
		recovery = (ddPeak - ddTrough) / ddTrough * 100
	}
	return maxDrawdownResult{
		MaxDrawdownPercent:    round2(maxDD),
		PeakPrice:             ddPeak,
		TroughPrice:           ddTrough,
		RecoveryNeededPercent: round2(recovery),
		Summary:               fmt.Sprintf("Max drawdown: %.2f%% (peak: %.2f → trough: %.2f; recovery needed: %.2f%%)", maxDD, ddPeak, ddTrough, recovery),
	}, nil
}

// --- P&L -----------------------------------------------------------------------

type pnlResult struct {
	PnLAbsolute   float64 `json:"pnl_absolute"`
	PnLPercent    float64 `json:"pnl_percent"`
	PositionValue float64 `json:"position_value"`
	Summary       string  `json:"summary"`
}

func calcPnL(entryPrice, currentPrice, quantity float64, positionType string) (pnlResult, error) {
	if positionType != "long" && positionType != "short" {
		return pnlResult{}, fmt.Errorf("position_type must be \"long\" or \"short\", got %q", positionType)
	}
	if err := validatePositive("entry_price", entryPrice); err != nil {
		return pnlResult{}, err
	}
	if err := validatePositive("current_price", currentPrice); err != nil {
		return pnlResult{}, err
	}
	if err := validateFinite("quantity", quantity); err != nil {
		return pnlResult{}, err
	}
	qty := math.Abs(quantity)
	if qty == 0 {
		return pnlResult{}, errors.New("quantity cannot be zero")
	}
	var pnl float64
	if positionType == "long" {
		pnl = (currentPrice - entryPrice) * qty
	} else {
		pnl = (entryPrice - currentPrice) * qty
	}
	posValue := currentPrice * qty
	pnlPct := pnl / (entryPrice * qty) * 100
	dir := "profit"
	if pnl < 0 {
		dir = "loss"
	}
	summary := fmt.Sprintf("%s position: %.2f%% %s (P&L: %.2f, position value: %.2f)", positionType, math.Abs(pnlPct), dir, pnl, posValue)
	if quantity < 0 {
		summary += " [WARNING: quantity was negative and has been treated as positive]"
	}
	return pnlResult{
		PnLAbsolute:   round2(pnl),
		PnLPercent:    round2(pnlPct),
		PositionValue: round2(posValue),
		Summary:       summary,
	}, nil
}

// --- internal helpers ----------------------------------------------------------

// Float is a float64 that marshals NaN and ±Inf as JSON null and parses JSON
// null back to math.NaN. Use it only on fields that can legitimately be
// undefined (e.g. the warm-up positions of an indicator series). Using it on
// ordinary numeric fields would silently coerce bad data to NaN.
type Float float64

// MarshalJSON encodes the value as a JSON number when finite, or null when
// NaN/±Inf. This keeps indicator output JSON-clean even though the warm-up
// positions of a moving average are mathematically undefined.
func (f Float) MarshalJSON() ([]byte, error) {
	v := float64(f)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return []byte("null"), nil
	}
	return json.Marshal(v)
}

// UnmarshalJSON parses a JSON number into Float, or maps JSON null to NaN.
// Any other JSON type (string, bool, object) is rejected so callers fail
// loudly on schema violations rather than getting NaN by surprise.
func (f *Float) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*f = Float(math.NaN())
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*f = Float(v)
	return nil
}

// FloatSlice marshals/unmarshals a []float64 where individual elements may be
// NaN/±Inf. JSON null elements round-trip to math.NaN.
type FloatSlice []float64

// MarshalJSON emits a JSON array, mapping each non-finite element to null.
func (s FloatSlice) MarshalJSON() ([]byte, error) {
	out := make([]byte, 0, len(s)*8)
	out = append(out, '[')
	for i, v := range s {
		if i > 0 {
			out = append(out, ',')
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			out = append(out, "null"...)
		} else {
			b, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			out = append(out, b...)
		}
	}
	out = append(out, ']')
	return out, nil
}

// UnmarshalJSON accepts a JSON array of numbers/nulls. Nulls become math.NaN.
func (s *FloatSlice) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	out := make([]float64, len(raw))
	for i, r := range raw {
		if string(r) == "null" {
			out[i] = math.NaN()
			continue
		}
		var v float64
		if err := json.Unmarshal(r, &v); err != nil {
			return err
		}
		out[i] = v
	}
	*s = out
	return nil
}

func meanFloat(xs []float64) float64 {
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// sampleStddev returns the sample standard deviation (denominator n-1).
// Returns 0 for slices with fewer than 2 elements.
func sampleStddev(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := meanFloat(xs)
	variance := 0.0
	for _, x := range xs {
		d := x - m
		variance += d * d
	}
	return math.Sqrt(variance / float64(len(xs)-1))
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// --- Beta ----------------------------------------------------------------------

type betaResult struct {
	Beta           float64 `json:"beta"`
	Correlation    float64 `json:"correlation"`
	Interpretation string  `json:"interpretation"`
	Summary        string  `json:"summary"`
}

func calcBeta(assetReturns, benchmarkReturns []float64) (betaResult, error) {
	n := len(assetReturns)
	if n != len(benchmarkReturns) {
		return betaResult{}, errors.New("asset_returns and benchmark_returns must have the same length")
	}
	if n < 2 {
		return betaResult{}, errors.New("at least 2 return observations required")
	}
	if err := validateReturnSlice("asset_returns", assetReturns); err != nil {
		return betaResult{}, err
	}
	if err := validateReturnSlice("benchmark_returns", benchmarkReturns); err != nil {
		return betaResult{}, err
	}
	meanA := meanFloat(assetReturns)
	meanB := meanFloat(benchmarkReturns)
	cov, varA, varB := 0.0, 0.0, 0.0
	for i := range n {
		da := assetReturns[i] - meanA
		db := benchmarkReturns[i] - meanB
		cov += da * db
		varA += da * da
		varB += db * db
	}
	cov /= float64(n - 1)
	varA /= float64(n - 1)
	varB /= float64(n - 1)
	if varB == 0 {
		return betaResult{}, errors.New("benchmark variance is zero; beta is undefined")
	}
	beta := cov / varB
	corr := 0.0
	if varA > 0 {
		corr = cov / math.Sqrt(varA*varB)
	}
	interp := "neutral"
	if beta > 1.2 {
		interp = "aggressive"
	} else if beta < 0.8 {
		interp = "defensive"
	}
	return betaResult{
		Beta:           round4(beta),
		Correlation:    round4(corr),
		Interpretation: interp,
		Summary:        fmt.Sprintf("Beta: %.4f (%s), correlation with benchmark: %.4f", beta, interp, corr),
	}, nil
}

// --- Treynor -------------------------------------------------------------------

type treynorResult struct {
	TreynorRatioPercent     float64 `json:"treynor_ratio_percent"`
	AnnualizedReturnPercent float64 `json:"annualized_return_percent"`
	Beta                    float64 `json:"beta"`
	Summary                 string  `json:"summary"`
}

// calcTreynor computes the Treynor ratio: annualised excess return per unit
// of systematic risk (beta), rather than per unit of total volatility like
// calcSharpe/calcSortino. Unlike those two, the Treynor ratio is not
// dimensionless — beta has no units, so the % units of the excess return
// don't cancel out — hence it's reported as a percentage.
// returns: daily returns of the asset in decimal (e.g. 0.01 = 1 %).
// riskFreeAnnual: annual risk-free rate in decimal (e.g. 0.04 = 4 %).
// beta: asset beta relative to its benchmark (e.g. from calculate_beta); can
// legitimately be negative (inverse correlation) or above 2 (highly
// levered/aggressive), so it's only checked for being finite and non-zero,
// never range-bound.
func calcTreynor(returns []float64, riskFreeAnnual, beta float64) (treynorResult, error) {
	if len(returns) == 0 {
		return treynorResult{}, errors.New("returns must not be empty")
	}
	if err := validateReturnSlice("returns", returns); err != nil {
		return treynorResult{}, err
	}
	if err := validateRate("risk_free_rate_annual", riskFreeAnnual); err != nil {
		return treynorResult{}, err
	}
	if err := validateFinite("beta", beta); err != nil {
		return treynorResult{}, err
	}
	if beta == 0 {
		return treynorResult{}, errors.New("beta is zero; Treynor ratio is undefined")
	}
	m := meanFloat(returns)
	annReturn := math.Pow(1+m, 252) - 1
	excess := annReturn - riskFreeAnnual
	treynor := excess / beta * 100
	return treynorResult{
		TreynorRatioPercent:     round2(treynor),
		AnnualizedReturnPercent: round2(annReturn * 100),
		Beta:                    round4(beta),
		Summary:                 fmt.Sprintf("Treynor ratio: %.2f%% (ann. return: %.2f%%, risk-free: %.2f%%, beta: %.4f)", treynor, annReturn*100, riskFreeAnnual*100, beta),
	}, nil
}

// --- Information Ratio ----------------------------------------------------------

type informationRatioResult struct {
	InformationRatio               float64 `json:"information_ratio"`
	AnnualizedActiveReturnPercent  float64 `json:"annualized_active_return_percent"`
	AnnualizedTrackingErrorPercent float64 `json:"annualized_tracking_error_percent"`
	Summary                        string  `json:"summary"`
}

// calcInformationRatio computes the information ratio: annualised active
// return (asset minus benchmark, period by period) divided by the annualised
// tracking error (the standard deviation of that active-return series). It
// measures the quality/consistency of active management versus a benchmark.
//
// Unlike calcSharpe/calcSortino, the active-return series is annualised
// arithmetically (×252) rather than via compounding (math.Pow): a spread of
// daily excess returns isn't an actual investable return series, so
// compounding it has no clean economic meaning. This also keeps the
// displayed figures numerically consistent with the ratio itself, since
// (meanDiff·252)/(stddev(diff)·√252) reduces to the same √252 shortcut used
// for the ratio.
func calcInformationRatio(assetReturns, benchmarkReturns []float64) (informationRatioResult, error) {
	n := len(assetReturns)
	if n != len(benchmarkReturns) {
		return informationRatioResult{}, errors.New("asset_returns and benchmark_returns must have the same length")
	}
	if n < 2 {
		return informationRatioResult{}, errors.New("at least 2 return observations required")
	}
	if err := validateReturnSlice("asset_returns", assetReturns); err != nil {
		return informationRatioResult{}, err
	}
	if err := validateReturnSlice("benchmark_returns", benchmarkReturns); err != nil {
		return informationRatioResult{}, err
	}
	diff := make([]float64, n)
	for i := range assetReturns {
		diff[i] = assetReturns[i] - benchmarkReturns[i]
	}
	m := meanFloat(diff)
	te := sampleStddev(diff)
	if te == 0 {
		return informationRatioResult{}, errors.New("tracking error is zero; information ratio is undefined")
	}
	ir := m / te * math.Sqrt(252)
	annActive := m * 252 * 100
	annTE := te * math.Sqrt(252) * 100
	return informationRatioResult{
		InformationRatio:               round2(ir),
		AnnualizedActiveReturnPercent:  round2(annActive),
		AnnualizedTrackingErrorPercent: round2(annTE),
		Summary:                        fmt.Sprintf("Information ratio: %.2f (ann. active return: %.2f%%, ann. tracking error: %.2f%%)", ir, annActive, annTE),
	}, nil
}

// --- Benchmark comparison --------------------------------------------------

// benchmarkComparisonResult is the result of portfolio_compare_benchmark: the
// portfolio's real period return (Modified Dietz, computed by
// pkg/portfolio.ComputePeriodReturn from Portfolio.Transactions) next to a
// benchmark's return over the same window, a simple alpha, and — when enough
// overlapping daily data exists — the portfolio's beta/correlation against
// the benchmark's real daily return series.
type benchmarkComparisonResult struct {
	PortfolioID             string   `json:"portfolio_id"`
	BenchmarkSymbol         string   `json:"benchmark_symbol"`
	From                    string   `json:"from"`
	To                      string   `json:"to"`
	PortfolioReturnPercent  float64  `json:"portfolio_return_percent"`
	BenchmarkReturnPercent  float64  `json:"benchmark_return_percent"`
	AlphaPercent            float64  `json:"alpha_percent"` // simple alpha: portfolio_return - benchmark_return
	Beta                    *float64 `json:"beta,omitempty"`
	Correlation             *float64 `json:"correlation,omitempty"`
	BetaInterpretation      string   `json:"beta_interpretation,omitempty"`
	ReturnMethod            string   `json:"return_method"` // "modified_dietz" | "buy_and_hold_approximation"
	MissingHistoricalPrices []string `json:"missing_historical_prices,omitempty"`
	Summary                 string   `json:"summary"`
	ComputedAt              string   `json:"computed_at"`
}

// buildBenchmarkComparison assembles a benchmarkComparisonResult from
// already-computed numbers, so the assembly logic (alpha, beta via calcBeta,
// summary wording) is unit-testable without a market provider. The dispatch
// case in tools.go owns fetching candles/quotes and reconstructing
// start-of-period holdings; this function only combines the results.
//
// portfolioReturns/benchmarkReturns are an aligned daily return series used
// only for beta/correlation (built from currently-held symbols weighted by
// their current share of portfolio value — see alignedDailyReturns in
// tools.go); pass nil/empty when there isn't enough overlapping data, which
// leaves Beta/Correlation unset.
func buildBenchmarkComparison(portfolioID, benchmarkSymbol string, from, to time.Time,
	portfolioReturnPercent, benchmarkReturnPercent float64, returnMethod string,
	missingHistorical []string, portfolioReturns, benchmarkReturns []float64) benchmarkComparisonResult {

	alpha := portfolioReturnPercent - benchmarkReturnPercent
	res := benchmarkComparisonResult{
		PortfolioID:             portfolioID,
		BenchmarkSymbol:         benchmarkSymbol,
		From:                    from.UTC().Format("2006-01-02"),
		To:                      to.UTC().Format("2006-01-02"),
		PortfolioReturnPercent:  round4(portfolioReturnPercent),
		BenchmarkReturnPercent:  round4(benchmarkReturnPercent),
		AlphaPercent:            round4(alpha),
		ReturnMethod:            returnMethod,
		MissingHistoricalPrices: missingHistorical,
	}

	betaNote := "beta not available (insufficient overlapping daily data)"
	if len(portfolioReturns) >= 2 && len(portfolioReturns) == len(benchmarkReturns) {
		if b, err := calcBeta(portfolioReturns, benchmarkReturns); err == nil {
			beta, corr := b.Beta, b.Correlation
			res.Beta = &beta
			res.Correlation = &corr
			res.BetaInterpretation = b.Interpretation
			betaNote = fmt.Sprintf("β=%.4f (%s, synthetic series from current holdings)", beta, b.Interpretation)
		}
	}

	extra := ""
	if returnMethod == "buy_and_hold_approximation" {
		extra += " [no transaction history — return approximated assuming current holdings were held for the whole period]"
	}
	if len(missingHistorical) > 0 {
		extra += fmt.Sprintf(" [WARNING: no historical price found for %v — start value may be understated]", missingHistorical)
	}
	res.Summary = fmt.Sprintf("portfolio %.2f%% vs %s %.2f%% over %s→%s (alpha %.2f%%), %s%s",
		portfolioReturnPercent, benchmarkSymbol, benchmarkReturnPercent, res.From, res.To, alpha, betaNote, extra)
	return res
}

// --- VaR -----------------------------------------------------------------------

type varResult struct {
	VaRAbsolute  float64 `json:"var_absolute"`
	VaRPercent   float64 `json:"var_percent"`
	CVaRAbsolute float64 `json:"cvar_absolute"`
	Method       string  `json:"method"`
	Summary      string  `json:"summary"`
}

func calcVaR(returns []float64, confidenceLevel, portfolioValue float64, method string) (varResult, error) {
	if len(returns) == 0 {
		return varResult{}, errors.New("returns must not be empty")
	}
	if err := validateReturnSlice("returns", returns); err != nil {
		return varResult{}, err
	}
	if err := validateFinite("confidence_level", confidenceLevel); err != nil {
		return varResult{}, err
	}
	if confidenceLevel <= 0 || confidenceLevel >= 1 {
		return varResult{}, fmt.Errorf("confidence_level must be between 0 and 1 exclusive, got %.4f", confidenceLevel)
	}
	if err := validatePositive("portfolio_value", portfolioValue); err != nil {
		return varResult{}, err
	}
	if method != "parametric" && method != "historical" {
		return varResult{}, fmt.Errorf("method must be \"parametric\" or \"historical\", got %q", method)
	}
	var varPct, cvarPct float64
	if method == "parametric" {
		mu := meanFloat(returns)
		sigma := sampleStddev(returns)
		z := probit(confidenceLevel)
		varPct = -(mu - z*sigma)
		// CVaR for normal distribution: -(mu - sigma*phi(z)/(1-conf))
		phi := math.Exp(-z*z/2) / math.Sqrt(2*math.Pi)
		cvarPct = -(mu - sigma*phi/(1-confidenceLevel))
	} else {
		sorted := make([]float64, len(returns))
		copy(sorted, returns)
		sort.Float64s(sorted)
		n := len(sorted)
		tailCount := max(1, min(n, int(math.Ceil((1-confidenceLevel)*float64(n)))))
		varPct = -sorted[tailCount-1]
		cvarPct = -meanFloat(sorted[:tailCount])
	}
	varAbs := varPct * portfolioValue
	cvarAbs := cvarPct * portfolioValue
	return varResult{
		VaRAbsolute:  round2(varAbs),
		VaRPercent:   round4(varPct * 100),
		CVaRAbsolute: round2(cvarAbs),
		Method:       method,
		Summary: fmt.Sprintf("%s VaR (%.0f%%): %.2f (%.4f%% of portfolio); CVaR: %.2f",
			method, confidenceLevel*100, varAbs, varPct*100, cvarAbs),
	}, nil
}

// probit returns the standard normal inverse CDF (quantile function).
// Uses math.Erfinv for full machine-precision accuracy: Φ⁻¹(p) = √2 · erfinv(2p−1).
func probit(p float64) float64 {
	if p <= 0 {
		return math.Inf(-1)
	}
	if p >= 1 {
		return math.Inf(1)
	}
	return math.Sqrt2 * math.Erfinv(2*p-1)
}

// --- DCF -----------------------------------------------------------------------

type dcfResult struct {
	IntrinsicValueTotal    float64 `json:"intrinsic_value_total"`
	IntrinsicValuePerShare float64 `json:"intrinsic_value_per_share"`
	TerminalValue          float64 `json:"terminal_value"`
	PVOfCashflows          float64 `json:"pv_of_cashflows"`
	Summary                string  `json:"summary"`
}

func calcDCF(freeCashFlows []float64, discountRate, terminalGrowthRate, sharesOutstanding float64) (dcfResult, error) {
	if len(freeCashFlows) == 0 {
		return dcfResult{}, errors.New("free_cash_flows must not be empty")
	}
	if err := validateFiniteAll("free_cash_flows", freeCashFlows); err != nil {
		return dcfResult{}, err
	}
	if err := validateRate("discount_rate", discountRate); err != nil {
		return dcfResult{}, err
	}
	if err := validateRate("terminal_growth_rate", terminalGrowthRate); err != nil {
		return dcfResult{}, err
	}
	if err := validateNonNegative("shares_outstanding", sharesOutstanding); err != nil {
		return dcfResult{}, err
	}
	if discountRate <= terminalGrowthRate {
		return dcfResult{}, errors.New("discount_rate must be greater than terminal_growth_rate to avoid infinite terminal value")
	}
	allNegative := true
	for _, fcf := range freeCashFlows {
		if fcf > 0 {
			allNegative = false
			break
		}
	}
	if allNegative {
		return dcfResult{}, errors.New("all free_cash_flows are negative or zero; DCF intrinsic value would be meaningless — provide at least one positive FCF")
	}
	pvFCF := 0.0
	for i, fcf := range freeCashFlows {
		pvFCF += fcf / math.Pow(1+discountRate, float64(i+1))
	}
	n := len(freeCashFlows)
	lastFCF := freeCashFlows[n-1]
	tv := lastFCF * (1 + terminalGrowthRate) / (discountRate - terminalGrowthRate)
	pvTV := tv / math.Pow(1+discountRate, float64(n))
	total := pvFCF + pvTV
	perShare := 0.0
	if sharesOutstanding > 0 {
		perShare = total / sharesOutstanding
	}
	summary := fmt.Sprintf("DCF intrinsic value: %.2f (%.2f/share); PV of FCFs: %.2f, PV of terminal value: %.2f",
		total, perShare, pvFCF, pvTV)
	if lastFCF < 0 {
		summary += " [WARNING: last FCF is negative, making the terminal value negative — results may not be economically meaningful]"
	}
	return dcfResult{
		IntrinsicValueTotal:    round2(total),
		IntrinsicValuePerShare: round2(perShare),
		TerminalValue:          round2(tv),
		PVOfCashflows:          round2(pvFCF),
		Summary:                summary,
	}, nil
}

// --- Multiples -----------------------------------------------------------------

type multiplesResult struct {
	PER          *float64 `json:"per,omitempty"`
	PBV          *float64 `json:"pbv,omitempty"`
	EVEbitda     *float64 `json:"ev_ebitda,omitempty"`
	EVRevenue    *float64 `json:"ev_revenue,omitempty"`
	PriceToSales *float64 `json:"price_to_sales,omitempty"`
	Summary      string   `json:"summary"`
}

func calcMultiples(price, eps, bookValuePerShare, ebitda, enterpriseValue, revenue float64) (multiplesResult, error) {
	if err := validatePositive("price", price); err != nil {
		return multiplesResult{}, err
	}
	if err := validateFinite("eps", eps); err != nil {
		return multiplesResult{}, err
	}
	if err := validateFinite("book_value_per_share", bookValuePerShare); err != nil {
		return multiplesResult{}, err
	}
	if err := validateFinite("ebitda", ebitda); err != nil {
		return multiplesResult{}, err
	}
	if err := validateFinite("enterprise_value", enterpriseValue); err != nil {
		return multiplesResult{}, err
	}
	if err := validateFinite("revenue", revenue); err != nil {
		return multiplesResult{}, err
	}
	r := multiplesResult{}
	var parts []string
	if eps != 0 {
		r.PER = f64ptr(price / eps)
		parts = append(parts, fmt.Sprintf("P/E=%.2f", *r.PER))
	}
	if bookValuePerShare != 0 {
		r.PBV = f64ptr(price / bookValuePerShare)
		parts = append(parts, fmt.Sprintf("P/BV=%.2f", *r.PBV))
	}
	if ebitda != 0 {
		r.EVEbitda = f64ptr(enterpriseValue / ebitda)
		parts = append(parts, fmt.Sprintf("EV/EBITDA=%.2f", *r.EVEbitda))
	}
	if revenue != 0 {
		r.EVRevenue = f64ptr(enterpriseValue / revenue)
		r.PriceToSales = f64ptr(price / revenue)
		parts = append(parts, fmt.Sprintf("EV/Rev=%.2f P/S=%.2f", *r.EVRevenue, *r.PriceToSales))
	}
	if len(parts) == 0 {
		r.Summary = "no multiples computed: all denominators are zero"
	} else {
		r.Summary = strings.Join(parts, ", ")
	}
	return r, nil
}

func f64ptr(v float64) *float64 { r := round2(v); return &r }

// --- Price / Free Cash Flow -----------------------------------------------------

type pfcfResult struct {
	PFCF     float64 `json:"pfcf"`
	Currency string  `json:"currency,omitempty"`
	Summary  string  `json:"summary"`
}

func calcPFCF(price, fcfPerShare float64, currency string) (pfcfResult, error) {
	if err := validatePositive("price", price); err != nil {
		return pfcfResult{}, err
	}
	if err := validatePositive("free_cash_flow_per_share", fcfPerShare); err != nil {
		return pfcfResult{}, err
	}
	pfcf := round2(price / fcfPerShare)
	return pfcfResult{
		PFCF:     pfcf,
		Currency: currency,
		Summary:  fmt.Sprintf("P/FCF=%.2f", pfcf),
	}, nil
}

// --- PEG ratio -------------------------------------------------------------------

type pegResult struct {
	PEG            float64 `json:"peg"`
	Interpretation string  `json:"interpretation"`
	Summary        string  `json:"summary"`
}

func calcPEG(peRatio, growthRatePercent float64) (pegResult, error) {
	if err := validatePositive("pe_ratio", peRatio); err != nil {
		return pegResult{}, err
	}
	if err := validateFinite("growth_rate_percent", growthRatePercent); err != nil {
		return pegResult{}, err
	}
	if growthRatePercent == 0 {
		return pegResult{}, errors.New("growth_rate_percent cannot be zero")
	}
	peg := round2(peRatio / growthRatePercent)
	interp := "reasonable"
	switch {
	case peg < 1:
		interp = "undervalued"
	case peg > 2:
		interp = "overvalued"
	}
	summary := fmt.Sprintf("PEG=%.2f (%s)", peg, interp)
	if growthRatePercent < 0 {
		summary += " — WARNING: negative growth rate makes PEG uninterpretable as a valuation signal"
	}
	return pegResult{
		PEG:            peg,
		Interpretation: interp,
		Summary:        summary,
	}, nil
}

// --- Dividend yield ---------------------------------------------------------------

type dividendYieldResult struct {
	AnnualDividend float64 `json:"annual_dividend"`
	YieldPercent   float64 `json:"yield_percent"`
	Summary        string  `json:"summary"`
}

func calcDividendYield(price, annualDividendPerShare float64, quarterlyDividends []float64) (dividendYieldResult, error) {
	if err := validatePositive("price", price); err != nil {
		return dividendYieldResult{}, err
	}
	if err := validateNonNegative("annual_dividend_per_share", annualDividendPerShare); err != nil {
		return dividendYieldResult{}, err
	}
	if err := validateNonNegativeAll("quarterly_dividends", quarterlyDividends); err != nil {
		return dividendYieldResult{}, err
	}
	annual := annualDividendPerShare
	source := "annual_dividend_per_share"
	if len(quarterlyDividends) > 0 {
		if len(quarterlyDividends) != 4 {
			return dividendYieldResult{}, errors.New("quarterly_dividends must contain exactly 4 values")
		}
		annual = 0
		for _, d := range quarterlyDividends {
			annual += d
		}
		source = "trailing twelve months (sum of quarterly_dividends)"
	}
	if annual <= 0 {
		return dividendYieldResult{}, errors.New("must provide annual_dividend_per_share or quarterly_dividends")
	}
	y := round4(annual / price * 100)
	return dividendYieldResult{
		AnnualDividend: round4(annual),
		YieldPercent:   y,
		Summary:        fmt.Sprintf("%.4f%% dividend yield, based on %s", y, source),
	}, nil
}

// --- Dividend growth ---------------------------------------------------------------

type dividendGrowthResult struct {
	CAGRPercent float64 `json:"cagr_percent"`
	Summary     string  `json:"summary"`
}

func calcDividendGrowth(dividends []float64) (dividendGrowthResult, error) {
	if len(dividends) < 2 {
		return dividendGrowthResult{}, errors.New("dividends must contain at least 2 chronological values")
	}
	if err := validateNonNegativeAll("dividends", dividends); err != nil {
		return dividendGrowthResult{}, err
	}
	first, last := dividends[0], dividends[len(dividends)-1]
	if first <= 0 {
		return dividendGrowthResult{}, errors.New("dividends[0] must be greater than zero")
	}
	years := float64(len(dividends) - 1)
	cagr := (math.Pow(last/first, 1/years) - 1) * 100
	return dividendGrowthResult{
		CAGRPercent: round2(cagr),
		Summary:     fmt.Sprintf("%.2f%% dividend CAGR over %d periods (%.4f → %.4f)", cagr, len(dividends)-1, first, last),
	}, nil
}

// --- Stress test -------------------------------------------------------------------

type stressScenario struct {
	ShockPercent   float64 `json:"shock_percent"`
	ResultingValue float64 `json:"resulting_value"`
	ChangeAbsolute float64 `json:"change_absolute"`
}

type stressTestResult struct {
	CurrentValue float64          `json:"current_value"`
	Scenarios    []stressScenario `json:"scenarios"`
	WorstCase    stressScenario   `json:"worst_case"`
	Summary      string           `json:"summary"`
}

func calcStressTest(currentValue float64, shocksPercent []float64, label string) (stressTestResult, error) {
	if err := validatePositive("current_value", currentValue); err != nil {
		return stressTestResult{}, err
	}
	if len(shocksPercent) == 0 {
		return stressTestResult{}, errors.New("shocks_percent must contain at least 1 value")
	}
	if err := validateReturnPercentSlice("shocks_percent", shocksPercent); err != nil {
		return stressTestResult{}, err
	}
	scenarios := make([]stressScenario, len(shocksPercent))
	worst := 0
	for i, shock := range shocksPercent {
		resulting := round2(currentValue * (1 + shock/100))
		scenarios[i] = stressScenario{
			ShockPercent:   shock,
			ResultingValue: resulting,
			ChangeAbsolute: round2(resulting - currentValue),
		}
		if scenarios[i].ResultingValue < scenarios[worst].ResultingValue {
			worst = i
		}
	}
	subject := "value"
	if label != "" {
		subject = label
	}
	wc := scenarios[worst]
	return stressTestResult{
		CurrentValue: round2(currentValue),
		Scenarios:    scenarios,
		WorstCase:    wc,
		Summary: fmt.Sprintf("stress test on %s: %d scenarios, worst case %.2f%% → %.2f (Δ%.2f)",
			subject, len(scenarios), wc.ShockPercent, wc.ResultingValue, wc.ChangeAbsolute),
	}, nil
}

// --- Monte Carlo simulation ----------------------------------------------------

// maxMonteCarloSimulations bounds num_simulations. This is a resource guard
// (num_simulations directly controls a loop length supplied by the LLM), not
// a "plausible financial range" check — thousands of simulations already
// give stable percentiles, so 100k leaves ample headroom while keeping the
// loop's time and memory footprint small.
const maxMonteCarloSimulations = 100000

// monteCarloTradingDaysPerYear mirrors the annualisation convention already
// used by calcVolatility/calcSharpe, since drift_annual/volatility_annual are
// expected to come from those same tools (DD-3).
const monteCarloTradingDaysPerYear = 252.0

type monteCarloResult struct {
	MeanFinalPrice   float64 `json:"mean_final_price"`
	MedianFinalPrice float64 `json:"median_final_price"` // P50
	P5FinalPrice     float64 `json:"p5_final_price"`
	P95FinalPrice    float64 `json:"p95_final_price"`
	ProbAbovePercent float64 `json:"prob_above_start_percent"`
	Days             int     `json:"days"`
	NumSimulations   int     `json:"num_simulations"`
	Summary          string  `json:"summary"`
}

// calcMonteCarloSimulation simulates the terminal price distribution under
// Geometric Brownian Motion using the closed-form solution
// S(T) = S(0)·exp((μ − ½σ²)·T + σ·√T·Z), Z ~ N(0,1) — only the final-price
// distribution is needed, so this avoids day-by-day path stepping.
func calcMonteCarloSimulation(lastPrice, driftAnnual, volatilityAnnual float64, days, numSimulations int) (monteCarloResult, error) {
	if err := validatePositive("last_price", lastPrice); err != nil {
		return monteCarloResult{}, err
	}
	if days <= 0 {
		return monteCarloResult{}, errors.New("days must be greater than zero")
	}
	if err := validateRate("drift_annual", driftAnnual); err != nil {
		return monteCarloResult{}, err
	}
	if err := validateNonNegative("volatility_annual", volatilityAnnual); err != nil {
		return monteCarloResult{}, err
	}
	if numSimulations <= 0 {
		return monteCarloResult{}, errors.New("num_simulations must be greater than zero")
	}
	if numSimulations > maxMonteCarloSimulations {
		return monteCarloResult{}, fmt.Errorf("num_simulations must be <= %d", maxMonteCarloSimulations)
	}

	t := float64(days) / monteCarloTradingDaysPerYear
	drift := (driftAnnual - 0.5*volatilityAnnual*volatilityAnnual) * t
	diffusionSD := volatilityAnnual * math.Sqrt(t)

	finals := make([]float64, numSimulations)
	above := 0
	sum := 0.0
	for i := 0; i < numSimulations; i++ {
		z := rand.NormFloat64()
		final := lastPrice * math.Exp(drift+diffusionSD*z)
		finals[i] = final
		sum += final
		if final > lastPrice {
			above++
		}
	}
	sort.Float64s(finals)

	mean := sum / float64(numSimulations)
	p5 := percentileInterp(finals, 0.05)
	p50 := percentileInterp(finals, 0.50)
	p95 := percentileInterp(finals, 0.95)
	probAbove := float64(above) / float64(numSimulations) * 100

	return monteCarloResult{
		MeanFinalPrice:   round2(mean),
		MedianFinalPrice: round2(p50),
		P5FinalPrice:     round2(p5),
		P95FinalPrice:    round2(p95),
		ProbAbovePercent: round4(probAbove),
		Days:             days,
		NumSimulations:   numSimulations,
		Summary: fmt.Sprintf("Monte Carlo (GBM, %d sims, %d days): mean=%.2f, P5=%.2f, P50=%.2f, P95=%.2f, P(up)=%.2f%%",
			numSimulations, days, mean, p5, p50, p95, probAbove),
	}, nil
}

// --- Currency conversion -------------------------------------------------------

type currencyResult struct {
	ConvertedAmount float64 `json:"converted_amount"`
	RateUsed        float64 `json:"rate_used"`
	Summary         string  `json:"summary"`
}

func calcCurrencyConversion(amount float64, fromCurrency, toCurrency string, exchangeRate float64) (currencyResult, error) {
	if err := validateFinite("amount", amount); err != nil {
		return currencyResult{}, err
	}
	if err := validatePositive("exchange_rate", exchangeRate); err != nil {
		return currencyResult{}, err
	}
	converted := amount * exchangeRate
	return currencyResult{
		ConvertedAmount: round2(converted),
		RateUsed:        exchangeRate,
		Summary:         fmt.Sprintf("%.2f %s = %.2f %s (rate: %.6f)", amount, fromCurrency, converted, toCurrency, exchangeRate),
	}, nil
}

// --- Compound interest ---------------------------------------------------------

type compoundInterestResult struct {
	FinalAmount         float64 `json:"final_amount"`
	TotalInterest       float64 `json:"total_interest"`
	EffectiveAnnualRate float64 `json:"effective_annual_rate"`
	Summary             string  `json:"summary"`
}

func calcCompoundInterest(principal, annualRate, years float64, compoundsPerYear int) (compoundInterestResult, error) {
	if err := validatePositive("principal", principal); err != nil {
		return compoundInterestResult{}, err
	}
	if err := validateRate("annual_rate", annualRate); err != nil {
		return compoundInterestResult{}, err
	}
	if err := validateFinite("years", years); err != nil {
		return compoundInterestResult{}, err
	}
	if compoundsPerYear <= 0 {
		return compoundInterestResult{}, errors.New("compounds_per_year must be at least 1")
	}
	if years < 0 {
		return compoundInterestResult{}, errors.New("years must be non-negative")
	}
	n := float64(compoundsPerYear)
	final := principal * math.Pow(1+annualRate/n, n*years)
	interest := final - principal
	ear := (math.Pow(1+annualRate/n, n) - 1) * 100
	return compoundInterestResult{
		FinalAmount:         round2(final),
		TotalInterest:       round2(interest),
		EffectiveAnnualRate: round4(ear),
		Summary: fmt.Sprintf("Principal %.2f at %.2f%% p.a. × %.1f years (%dx/year) → %.2f (interest: %.2f, EAR: %.4f%%)",
			principal, annualRate*100, years, compoundsPerYear, final, interest, ear),
	}, nil
}

// --- Descriptive statistics ----------------------------------------------------

type statsResult struct {
	Mean         float64 `json:"mean"`
	Median       float64 `json:"median"`
	StdDev       float64 `json:"std_dev"`
	Min          float64 `json:"min"`
	Max          float64 `json:"max"`
	Percentile25 float64 `json:"percentile_25"`
	Percentile75 float64 `json:"percentile_75"`
	Count        int     `json:"count"`
	Summary      string  `json:"summary"`
}

func calcStats(values []float64, label string) (statsResult, error) {
	if len(values) == 0 {
		return statsResult{}, errors.New("values must not be empty")
	}
	if err := validateFiniteAll("values", values); err != nil {
		return statsResult{}, err
	}
	s := make([]float64, len(values))
	copy(s, values)
	sort.Float64s(s)
	n := len(s)
	m := meanFloat(s)
	sd := sampleStddev(s)
	median := percentileInterp(s, 0.5)
	p25 := percentileInterp(s, 0.25)
	p75 := percentileInterp(s, 0.75)
	return statsResult{
		Mean:         round4(m),
		Median:       round4(median),
		StdDev:       round4(sd),
		Min:          round4(s[0]),
		Max:          round4(s[n-1]),
		Percentile25: round4(p25),
		Percentile75: round4(p75),
		Count:        n,
		Summary:      fmt.Sprintf("%s (n=%d): mean=%.4f, median=%.4f, std=%.4f, range=[%.4f, %.4f]", label, n, m, median, sd, s[0], s[n-1]),
	}, nil
}

// percentileInterp returns the p-th percentile using linear interpolation (Type 7).
func percentileInterp(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 1 {
		return sorted[0]
	}
	h := p * float64(n-1)
	lo := int(math.Floor(h))
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}
	return sorted[lo] + (h-float64(lo))*(sorted[hi]-sorted[lo])
}

// --- Technical indicators -----------------------------------------------------

// smaResult holds the simple moving average series and headline figures.
//
// Values are aligned to the input: the first len(prices)-period+1 entries of
// Values are non-nil; the rest are NaN so callers can detect warm-up.
type smaResult struct {
	Period    int        `json:"period"`
	Values    FloatSlice `json:"values"`
	Last      float64    `json:"last"`
	Previous  Float      `json:"previous"` // null during warm-up (< period prices)
	Trend     string     `json:"trend"`    // "up", "down", or "flat"
	Summary   string     `json:"summary"`
	InputSize int        `json:"input_size"`
}

// calcSMA computes the simple moving average of closing prices over a sliding
// window of length period. The first period-1 entries of the returned series
// are NaN to reflect the warm-up period.
func calcSMA(prices []float64, period int) (smaResult, error) {
	if err := validatePositiveAll("prices", prices); err != nil {
		return smaResult{}, err
	}
	values, err := finance.SMA(prices, period)
	if err != nil {
		return smaResult{}, err
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
	return smaResult{
		Period:    period,
		Values:    values, // []float64 → FloatSlice (same underlying type)
		Last:      round4(last),
		Previous:  Float(round4(previous)),
		Trend:     trend,
		InputSize: len(prices),
		Summary:   fmt.Sprintf("SMA(%d) last=%.4f (prev=%.4f, trend=%s) over %d prices", period, last, previous, trend, len(prices)),
	}, nil
}

// emaResult holds the exponential moving average series and headline figures.
//
// Values are aligned to the input: the first len(prices) entries are valid
// because EMA seeds with the first observation, unlike SMA.
type emaResult struct {
	Period    int       `json:"period"`
	Alpha     float64   `json:"alpha"`
	Values    []float64 `json:"values"`
	Last      float64   `json:"last"`
	Previous  Float     `json:"previous"` // null when only 1 price provided
	Trend     string    `json:"trend"`
	Summary   string    `json:"summary"`
	InputSize int       `json:"input_size"`
}

// calcEMA computes the exponential moving average of prices using the recursive
// formula EMA_t = alpha * P_t + (1-alpha) * EMA_{t-1}, seeded with the first
// observation. If alpha is zero or negative, it defaults to 2 / (period + 1),
// the standard "Wilder smoothing" convention.
func calcEMA(prices []float64, period int, alpha float64) (emaResult, error) {
	if period <= 0 {
		return emaResult{}, fmt.Errorf("period must be greater than zero, got %d", period)
	}
	if len(prices) == 0 {
		return emaResult{}, errors.New("prices must not be empty")
	}
	if err := validatePositiveAll("prices", prices); err != nil {
		return emaResult{}, err
	}
	if err := validateFinite("alpha", alpha); err != nil {
		return emaResult{}, err
	}
	if alpha <= 0 {
		alpha = 2.0 / (float64(period) + 1)
	}
	if alpha >= 1 {
		return emaResult{}, fmt.Errorf("alpha must be less than 1, got %.4f", alpha)
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
	return emaResult{
		Period:    period,
		Alpha:     round4(alpha),
		Values:    roundSlice(values, 6),
		Last:      round4(last),
		Previous:  Float(round4(previous)),
		Trend:     trend,
		InputSize: len(prices),
		Summary:   fmt.Sprintf("EMA(%d, alpha=%.4f) last=%.4f (prev=%.4f, trend=%s) over %d prices", period, alpha, last, previous, trend, len(prices)),
	}, nil
}

// rsiResult holds the Relative Strength Index (Wilder) and its interpretation.
type rsiResult struct {
	Period         int        `json:"period"`
	Value          float64    `json:"value"`
	PreviousValue  Float      `json:"previous_value"` // null at minimum input size (period+2 prices)
	Interpretation string     `json:"interpretation"` // "oversold", "neutral", "overbought"
	Values         FloatSlice `json:"values"`
	Summary        string     `json:"summary"`
	InputSize      int        `json:"input_size"`
}

// calcRSI computes the Relative Strength Index using Wilder's smoothing
// (equivalent to an EMA with alpha = 1/period). Interpretation:
//   - value < 30 → "oversold"
//   - value > 70 → "overbought"
//   - otherwise → "neutral"
func calcRSI(prices []float64, period int) (rsiResult, error) {
	if period <= 0 {
		return rsiResult{}, fmt.Errorf("period must be greater than zero, got %d", period)
	}
	// Need at least period+1 prices to compute period changes, then a second
	// value to populate PreviousValue.
	if len(prices) < period+2 {
		return rsiResult{}, fmt.Errorf("at least %d prices required for RSI(%d), got %d", period+2, period, len(prices))
	}
	if err := validatePositiveAll("prices", prices); err != nil {
		return rsiResult{}, err
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
	return rsiResult{
		Period:         period,
		Value:          round4(last),
		PreviousValue:  Float(round4(previous)),
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
type macdResult struct {
	FastPeriod   int       `json:"fast_period"`
	SlowPeriod   int       `json:"slow_period"`
	SignalPeriod int       `json:"signal_period"`
	MACDLine     []float64 `json:"macd_line"`
	SignalLine   []float64 `json:"signal_line"`
	Histogram    []float64 `json:"histogram"`
	LastMACD     float64   `json:"last_macd"`
	LastSignal   float64   `json:"last_signal"`
	LastHist     float64   `json:"last_hist"`
	Trend        string    `json:"trend"` // "bullish_cross", "bearish_cross", or "no_cross"
	Summary      string    `json:"summary"`
	InputSize    int       `json:"input_size"`
}

// calcMACD computes the Moving Average Convergence Divergence indicator.
//
// The MACD line is the difference between a fast EMA and a slow EMA of prices.
// The signal line is an EMA of the MACD line itself. The histogram is the
// difference between MACD and signal. A "bullish_cross" is reported when the
// histogram flipped from negative to non-negative on the latest bar; a
// "bearish_cross" is the opposite. If fast ≥ slow the function returns an
// error because the indicator is undefined.
func calcMACD(prices []float64, fastPeriod, slowPeriod, signalPeriod int) (macdResult, error) {
	if fastPeriod <= 0 || slowPeriod <= 0 || signalPeriod <= 0 {
		return macdResult{}, fmt.Errorf("fast_period, slow_period, signal_period must all be positive (got %d, %d, %d)", fastPeriod, slowPeriod, signalPeriod)
	}
	if fastPeriod >= slowPeriod {
		return macdResult{}, fmt.Errorf("fast_period (%d) must be less than slow_period (%d)", fastPeriod, slowPeriod)
	}
	// Need slowPeriod observations to seed both EMAs and signalPeriod more for the signal.
	if len(prices) < slowPeriod+signalPeriod {
		return macdResult{}, fmt.Errorf("at least %d prices required for MACD(%d,%d,%d), got %d",
			slowPeriod+signalPeriod, fastPeriod, slowPeriod, signalPeriod, len(prices))
	}
	if err := validatePositiveAll("prices", prices); err != nil {
		return macdResult{}, err
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
	return macdResult{
		FastPeriod:   fastPeriod,
		SlowPeriod:   slowPeriod,
		SignalPeriod: signalPeriod,
		MACDLine:     roundSlice(macdLine, 6),
		SignalLine:   roundSlice(signalLine, 6),
		Histogram:    roundSlice(histogram, 6),
		LastMACD:     round4(macdLine[len(macdLine)-1]),
		LastSignal:   round4(signalLine[len(signalLine)-1]),
		LastHist:     round4(histogram[len(histogram)-1]),
		Trend:        trend,
		InputSize:    len(prices),
		Summary: fmt.Sprintf("MACD(%d,%d,%d): macd=%.4f, signal=%.4f, hist=%.4f, trend=%s",
			fastPeriod, slowPeriod, signalPeriod,
			macdLine[len(macdLine)-1], signalLine[len(signalLine)-1], histogram[len(histogram)-1], trend),
	}, nil
}

// bollingerResult holds Bollinger Band output for a price series.
type bollingerResult struct {
	Period    int        `json:"period"`
	NumStd    float64    `json:"num_std"`
	Upper     FloatSlice `json:"upper"`
	Middle    FloatSlice `json:"middle"`
	Lower     FloatSlice `json:"lower"`
	Bandwidth FloatSlice `json:"bandwidth"` // (upper - lower) / middle
	PercentB  FloatSlice `json:"percent_b"` // (price - lower) / (upper - lower)
	LastPrice float64    `json:"last_price"`
	LastUpper float64    `json:"last_upper"`
	LastLower float64    `json:"last_lower"`
	LastPctB  float64    `json:"last_percent_b"`
	Summary   string     `json:"summary"`
	InputSize int        `json:"input_size"`
}

// calcBollingerBands computes Bollinger Bands (moving average ± k·σ) for a
// price series. Returns upper/middle/lower/bandwidth/%b series, each entry
// aligned to prices (NaN during the warm-up period).
func calcBollingerBands(prices []float64, period int, numStd float64) (bollingerResult, error) {
	if period <= 0 {
		return bollingerResult{}, fmt.Errorf("period must be greater than zero, got %d", period)
	}
	if err := validateFinite("num_std", numStd); err != nil {
		return bollingerResult{}, err
	}
	if numStd <= 0 {
		return bollingerResult{}, fmt.Errorf("num_std must be positive, got %.4f", numStd)
	}
	if len(prices) < period {
		return bollingerResult{}, fmt.Errorf("at least %d prices required for Bollinger(%d), got %d", period, period, len(prices))
	}
	if err := validatePositiveAll("prices", prices); err != nil {
		return bollingerResult{}, err
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
	return bollingerResult{
		Period:    period,
		NumStd:    round4(numStd),
		Upper:     upper, // []float64 → FloatSlice (same underlying type)
		Middle:    middle,
		Lower:     lower,
		Bandwidth: bandwidth,
		PercentB:  pctB,
		LastPrice: round4(prices[last]),
		LastUpper: round4(upper[last]),
		LastLower: round4(lower[last]),
		LastPctB:  round4(pctB[last]),
		InputSize: len(prices),
		Summary:   summary,
	}, nil
}

// correlationMatrixResult holds an NxN Pearson correlation matrix between
// named return series, plus the diagonal (always 1) and labels for downstream
// rendering.
type correlationMatrixResult struct {
	Labels  []string    `json:"labels"`
	Matrix  [][]float64 `json:"matrix"`
	Scale   string      `json:"scale"` // "[-1, 1]"
	Summary string      `json:"summary"`
}

// calcCorrelationMatrix computes the Pearson correlation between every pair
// of the provided series. Each series must be the same length (typical usage:
// daily returns of N assets).
func calcCorrelationMatrix(series map[string][]float64) (correlationMatrixResult, error) {
	if len(series) < 2 {
		return correlationMatrixResult{}, fmt.Errorf("at least 2 series required, got %d", len(series))
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
			return correlationMatrixResult{}, fmt.Errorf("series %q has length %d, expected %d", k, l, n)
		}
	}
	if n < 2 {
		return correlationMatrixResult{}, fmt.Errorf("each series must have at least 2 observations, got %d", n)
	}
	for _, k := range keys {
		if err := validateFiniteAll(fmt.Sprintf("series[%q]", k), series[k]); err != nil {
			return correlationMatrixResult{}, err
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
				return correlationMatrixResult{}, fmt.Errorf("%s vs %s: %w", ki, kj, err)
			}
			matrix[i][j] = round4(c)
		}
	}
	return correlationMatrixResult{
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
// first observation. Exported only through calcEMA/calcMACD; kept unexported
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
