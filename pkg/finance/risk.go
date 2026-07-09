package finance

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// --- Volatility ----------------------------------------------------------------

type VolatilityResult struct {
	VolatilityAnnualPercent float64 `json:"volatility_annual_percent"`
	VolatilityDailyPercent  float64 `json:"volatility_daily_percent"`
	Summary                 string  `json:"summary"`
}

func CalcVolatility(prices []float64) (VolatilityResult, error) {
	if len(prices) < 2 {
		return VolatilityResult{}, errors.New("at least 2 prices required")
	}
	if err := ValidatePositiveAll("prices", prices); err != nil {
		return VolatilityResult{}, err
	}
	returns := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		returns[i-1] = math.Log(prices[i] / prices[i-1])
	}
	sd := sampleStddev(returns)
	daily := sd * 100
	annual := sd * math.Sqrt(252) * 100
	return VolatilityResult{
		VolatilityAnnualPercent: Round2(annual),
		VolatilityDailyPercent:  Round4(daily),
		Summary:                 fmt.Sprintf("Annual volatility: %.2f%% (daily: %.4f%%)", annual, daily),
	}, nil
}

// --- Sharpe --------------------------------------------------------------------

type SharpeResult struct {
	SharpeRatio                 float64 `json:"sharpe_ratio"`
	AnnualizedReturnPercent     float64 `json:"annualized_return_percent"`
	AnnualizedVolatilityPercent float64 `json:"annualized_volatility_percent"`
	Summary                     string  `json:"summary"`
}

// CalcSharpe computes the annualised Sharpe ratio.
// returns: daily returns in decimal (e.g. 0.01 = 1 %).
// riskFreeAnnual: annual risk-free rate in decimal (e.g. 0.04 = 4 %).
func CalcSharpe(returns []float64, riskFreeAnnual float64) (SharpeResult, error) {
	if len(returns) == 0 {
		return SharpeResult{}, errors.New("returns must not be empty")
	}
	if err := ValidateReturnSlice("returns", returns); err != nil {
		return SharpeResult{}, err
	}
	if err := ValidateRate("risk_free_rate_annual", riskFreeAnnual); err != nil {
		return SharpeResult{}, err
	}
	dailyRF := math.Pow(1+riskFreeAnnual, 1.0/252) - 1
	m := meanFloat(returns)
	sd := sampleStddev(returns)
	if sd == 0 {
		return SharpeResult{}, errors.New("volatility is zero; Sharpe ratio is undefined")
	}
	sharpe := (m - dailyRF) / sd * math.Sqrt(252)
	annReturn := (math.Pow(1+m, 252) - 1) * 100
	annVol := sd * math.Sqrt(252) * 100
	return SharpeResult{
		SharpeRatio:                 Round2(sharpe),
		AnnualizedReturnPercent:     Round2(annReturn),
		AnnualizedVolatilityPercent: Round2(annVol),
		Summary:                     fmt.Sprintf("Sharpe ratio: %.2f (ann. gross return: %.2f%% [before subtracting risk-free rate], ann. vol: %.2f%%)", sharpe, annReturn, annVol),
	}, nil
}

// --- Sortino -------------------------------------------------------------------

type SortinoResult struct {
	SortinoRatio                       float64 `json:"sortino_ratio"`
	AnnualizedReturnPercent            float64 `json:"annualized_return_percent"`
	AnnualizedDownsideDeviationPercent float64 `json:"annualized_downside_deviation_percent"`
	Summary                            string  `json:"summary"`
}

// CalcSortino computes the annualised Sortino ratio, a Sharpe variant that
// only penalises downside volatility (returns below a minimum acceptable
// return), leaving upside volatility unpenalised.
// returns: daily returns in decimal (e.g. 0.01 = 1 %).
// riskFreeAnnual: annual risk-free rate in decimal (e.g. 0.04 = 4 %), used
// both as the excess-return benchmark (like CalcSharpe) and as the minimum
// acceptable return (MAR) below which a daily return counts as "downside".
func CalcSortino(returns []float64, riskFreeAnnual float64) (SortinoResult, error) {
	if len(returns) == 0 {
		return SortinoResult{}, errors.New("returns must not be empty")
	}
	if err := ValidateReturnSlice("returns", returns); err != nil {
		return SortinoResult{}, err
	}
	if err := ValidateRate("risk_free_rate_annual", riskFreeAnnual); err != nil {
		return SortinoResult{}, err
	}
	dailyRF := math.Pow(1+riskFreeAnnual, 1.0/252) - 1
	m := meanFloat(returns)
	dd := downsideDeviation(returns, dailyRF)
	if dd == 0 {
		return SortinoResult{}, errors.New("downside deviation is zero; Sortino ratio is undefined")
	}
	sortino := (m - dailyRF) / dd * math.Sqrt(252)
	annReturn := (math.Pow(1+m, 252) - 1) * 100
	annDD := dd * math.Sqrt(252) * 100
	return SortinoResult{
		SortinoRatio:                       Round2(sortino),
		AnnualizedReturnPercent:            Round2(annReturn),
		AnnualizedDownsideDeviationPercent: Round2(annDD),
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

type MaxDrawdownResult struct {
	MaxDrawdownPercent    float64 `json:"max_drawdown_percent"`
	PeakPrice             float64 `json:"peak_price"`
	TroughPrice           float64 `json:"trough_price"`
	RecoveryNeededPercent float64 `json:"recovery_needed_percent"`
	Summary               string  `json:"summary"`
}

func CalcMaxDrawdown(prices []float64) (MaxDrawdownResult, error) {
	if len(prices) < 2 {
		return MaxDrawdownResult{}, errors.New("at least 2 prices required")
	}
	if err := ValidatePositiveAll("prices", prices); err != nil {
		return MaxDrawdownResult{}, err
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
	return MaxDrawdownResult{
		MaxDrawdownPercent:    Round2(maxDD),
		PeakPrice:             ddPeak,
		TroughPrice:           ddTrough,
		RecoveryNeededPercent: Round2(recovery),
		Summary:               fmt.Sprintf("Max drawdown: %.2f%% (peak: %.2f → trough: %.2f; recovery needed: %.2f%%)", maxDD, ddPeak, ddTrough, recovery),
	}, nil
}

// --- Beta ----------------------------------------------------------------------

type BetaResult struct {
	Beta           float64 `json:"beta"`
	Correlation    float64 `json:"correlation"`
	Interpretation string  `json:"interpretation"`
	Summary        string  `json:"summary"`
}

func CalcBeta(assetReturns, benchmarkReturns []float64) (BetaResult, error) {
	n := len(assetReturns)
	if n != len(benchmarkReturns) {
		return BetaResult{}, errors.New("asset_returns and benchmark_returns must have the same length")
	}
	if n < 2 {
		return BetaResult{}, errors.New("at least 2 return observations required")
	}
	if err := ValidateReturnSlice("asset_returns", assetReturns); err != nil {
		return BetaResult{}, err
	}
	if err := ValidateReturnSlice("benchmark_returns", benchmarkReturns); err != nil {
		return BetaResult{}, err
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
		return BetaResult{}, errors.New("benchmark variance is zero; beta is undefined")
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
	return BetaResult{
		Beta:           Round4(beta),
		Correlation:    Round4(corr),
		Interpretation: interp,
		Summary:        fmt.Sprintf("Beta: %.4f (%s), correlation with benchmark: %.4f", beta, interp, corr),
	}, nil
}

// --- Treynor -------------------------------------------------------------------

type TreynorResult struct {
	TreynorRatioPercent     float64 `json:"treynor_ratio_percent"`
	AnnualizedReturnPercent float64 `json:"annualized_return_percent"`
	Beta                    float64 `json:"beta"`
	Summary                 string  `json:"summary"`
}

// CalcTreynor computes the Treynor ratio: annualised excess return per unit
// of systematic risk (beta), rather than per unit of total volatility like
// CalcSharpe/CalcSortino. Unlike those two, the Treynor ratio is not
// dimensionless — beta has no units, so the % units of the excess return
// don't cancel out — hence it's reported as a percentage.
// returns: daily returns of the asset in decimal (e.g. 0.01 = 1 %).
// riskFreeAnnual: annual risk-free rate in decimal (e.g. 0.04 = 4 %).
// beta: asset beta relative to its benchmark (e.g. from CalcBeta); can
// legitimately be negative (inverse correlation) or above 2 (highly
// levered/aggressive), so it's only checked for being finite and non-zero,
// never range-bound.
func CalcTreynor(returns []float64, riskFreeAnnual, beta float64) (TreynorResult, error) {
	if len(returns) == 0 {
		return TreynorResult{}, errors.New("returns must not be empty")
	}
	if err := ValidateReturnSlice("returns", returns); err != nil {
		return TreynorResult{}, err
	}
	if err := ValidateRate("risk_free_rate_annual", riskFreeAnnual); err != nil {
		return TreynorResult{}, err
	}
	if err := ValidateFinite("beta", beta); err != nil {
		return TreynorResult{}, err
	}
	if beta == 0 {
		return TreynorResult{}, errors.New("beta is zero; Treynor ratio is undefined")
	}
	m := meanFloat(returns)
	annReturn := math.Pow(1+m, 252) - 1
	excess := annReturn - riskFreeAnnual
	treynor := excess / beta * 100
	return TreynorResult{
		TreynorRatioPercent:     Round2(treynor),
		AnnualizedReturnPercent: Round2(annReturn * 100),
		Beta:                    Round4(beta),
		Summary:                 fmt.Sprintf("Treynor ratio: %.2f%% (ann. return: %.2f%%, risk-free: %.2f%%, beta: %.4f)", treynor, annReturn*100, riskFreeAnnual*100, beta),
	}, nil
}

// --- Information Ratio ----------------------------------------------------------

type InformationRatioResult struct {
	InformationRatio               float64 `json:"information_ratio"`
	AnnualizedActiveReturnPercent  float64 `json:"annualized_active_return_percent"`
	AnnualizedTrackingErrorPercent float64 `json:"annualized_tracking_error_percent"`
	Summary                        string  `json:"summary"`
}

// CalcInformationRatio computes the information ratio: annualised active
// return (asset minus benchmark, period by period) divided by the annualised
// tracking error (the standard deviation of that active-return series). It
// measures the quality/consistency of active management versus a benchmark.
//
// Unlike CalcSharpe/CalcSortino, the active-return series is annualised
// arithmetically (×252) rather than via compounding (math.Pow): a spread of
// daily excess returns isn't an actual investable return series, so
// compounding it has no clean economic meaning. This also keeps the
// displayed figures numerically consistent with the ratio itself, since
// (meanDiff·252)/(stddev(diff)·√252) reduces to the same √252 shortcut used
// for the ratio.
func CalcInformationRatio(assetReturns, benchmarkReturns []float64) (InformationRatioResult, error) {
	n := len(assetReturns)
	if n != len(benchmarkReturns) {
		return InformationRatioResult{}, errors.New("asset_returns and benchmark_returns must have the same length")
	}
	if n < 2 {
		return InformationRatioResult{}, errors.New("at least 2 return observations required")
	}
	if err := ValidateReturnSlice("asset_returns", assetReturns); err != nil {
		return InformationRatioResult{}, err
	}
	if err := ValidateReturnSlice("benchmark_returns", benchmarkReturns); err != nil {
		return InformationRatioResult{}, err
	}
	diff := make([]float64, n)
	for i := range assetReturns {
		diff[i] = assetReturns[i] - benchmarkReturns[i]
	}
	m := meanFloat(diff)
	te := sampleStddev(diff)
	if te == 0 {
		return InformationRatioResult{}, errors.New("tracking error is zero; information ratio is undefined")
	}
	ir := m / te * math.Sqrt(252)
	annActive := m * 252 * 100
	annTE := te * math.Sqrt(252) * 100
	return InformationRatioResult{
		InformationRatio:               Round2(ir),
		AnnualizedActiveReturnPercent:  Round2(annActive),
		AnnualizedTrackingErrorPercent: Round2(annTE),
		Summary:                        fmt.Sprintf("Information ratio: %.2f (ann. active return: %.2f%%, ann. tracking error: %.2f%%)", ir, annActive, annTE),
	}, nil
}

// --- VaR -----------------------------------------------------------------------

type VarResult struct {
	VaRAbsolute  float64 `json:"var_absolute"`
	VaRPercent   float64 `json:"var_percent"`
	CVaRAbsolute float64 `json:"cvar_absolute"`
	Method       string  `json:"method"`
	Summary      string  `json:"summary"`
}

func CalcVaR(returns []float64, confidenceLevel, portfolioValue float64, method string) (VarResult, error) {
	if len(returns) == 0 {
		return VarResult{}, errors.New("returns must not be empty")
	}
	if err := ValidateReturnSlice("returns", returns); err != nil {
		return VarResult{}, err
	}
	if err := ValidateFinite("confidence_level", confidenceLevel); err != nil {
		return VarResult{}, err
	}
	if confidenceLevel <= 0 || confidenceLevel >= 1 {
		return VarResult{}, fmt.Errorf("confidence_level must be between 0 and 1 exclusive, got %.4f", confidenceLevel)
	}
	if err := ValidatePositive("portfolio_value", portfolioValue); err != nil {
		return VarResult{}, err
	}
	if method != "parametric" && method != "historical" {
		return VarResult{}, fmt.Errorf("method must be \"parametric\" or \"historical\", got %q", method)
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
	return VarResult{
		VaRAbsolute:  Round2(varAbs),
		VaRPercent:   Round4(varPct * 100),
		CVaRAbsolute: Round2(cvarAbs),
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
