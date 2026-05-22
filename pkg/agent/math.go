package agent

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

// --- ROI -----------------------------------------------------------------------

type roiResult struct {
	ROIPercent float64 `json:"roi_percent"`
	ProfitLoss float64 `json:"profit_loss"`
	Summary    string  `json:"summary"`
}

func calcROI(costBasis, currentValue float64) (roiResult, error) {
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
	if initialValue <= 0 {
		return cagrResult{}, errors.New("initial_value must be greater than zero")
	}
	if years <= 0 {
		return cagrResult{}, errors.New("years must be greater than zero")
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
	returns := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		if prices[i-1] <= 0 {
			return volatilityResult{}, errors.New("all prices must be positive")
		}
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
		Summary:                     fmt.Sprintf("Sharpe ratio: %.2f (ann. return: %.2f%%, ann. vol: %.2f%%)", sharpe, annReturn, annVol),
	}, nil
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
	if entryPrice == 0 {
		return pnlResult{}, errors.New("entry_price cannot be zero")
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
	return pnlResult{
		PnLAbsolute:   round2(pnl),
		PnLPercent:    round2(pnlPct),
		PositionValue: round2(posValue),
		Summary:       fmt.Sprintf("%s position: %.2f%% %s (P&L: %.2f, position value: %.2f)", positionType, math.Abs(pnlPct), dir, pnl, posValue),
	}, nil
}

// --- internal helpers ----------------------------------------------------------

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
	if confidenceLevel <= 0 || confidenceLevel >= 1 {
		return varResult{}, fmt.Errorf("confidence_level must be between 0 and 1 exclusive, got %.4f", confidenceLevel)
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

// probit approximates the standard normal inverse CDF (A&S 26.2.17, error < 4.5e-4).
func probit(p float64) float64 {
	const (
		c0, c1, c2 = 2.515517, 0.802853, 0.010328
		d1, d2, d3 = 1.432788, 0.189269, 0.001308
	)
	if p <= 0 {
		return math.Inf(-1)
	}
	if p >= 1 {
		return math.Inf(1)
	}
	sign := 1.0
	q := p
	if q > 0.5 {
		q = 1 - p
		sign = -1
	}
	t := math.Sqrt(-2 * math.Log(q))
	z := t - (c0+c1*t+c2*t*t)/(1+d1*t+d2*t*t+d3*t*t*t)
	return sign * z
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
	if discountRate <= terminalGrowthRate {
		return dcfResult{}, errors.New("discount_rate must be greater than terminal_growth_rate to avoid infinite terminal value")
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
	return dcfResult{
		IntrinsicValueTotal:    round2(total),
		IntrinsicValuePerShare: round2(perShare),
		TerminalValue:          round2(tv),
		PVOfCashflows:          round2(pvFCF),
		Summary: fmt.Sprintf("DCF intrinsic value: %.2f (%.2f/share); PV of FCFs: %.2f, PV of terminal value: %.2f",
			total, perShare, pvFCF, pvTV),
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

func calcMultiples(price, eps, bookValuePerShare, ebitda, enterpriseValue, revenue float64) multiplesResult {
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
	return r
}

func f64ptr(v float64) *float64 { r := round2(v); return &r }

// --- Currency conversion -------------------------------------------------------

type currencyResult struct {
	ConvertedAmount float64 `json:"converted_amount"`
	RateUsed        float64 `json:"rate_used"`
	Summary         string  `json:"summary"`
}

func calcCurrencyConversion(amount float64, fromCurrency, toCurrency string, exchangeRate float64) (currencyResult, error) {
	if exchangeRate <= 0 {
		return currencyResult{}, errors.New("exchange_rate must be greater than zero")
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
		Min:          s[0],
		Max:          s[n-1],
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
