package finance

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
)

// --- Stress test -------------------------------------------------------------------

type stressScenario struct {
	ShockPercent   float64 `json:"shock_percent"`
	ResultingValue float64 `json:"resulting_value"`
	ChangeAbsolute float64 `json:"change_absolute"`
}

type StressTestResult struct {
	CurrentValue float64          `json:"current_value"`
	Scenarios    []stressScenario `json:"scenarios"`
	WorstCase    stressScenario   `json:"worst_case"`
	Summary      string           `json:"summary"`
	ComputedAt   string           `json:"computed_at"`
}

func CalcStressTest(currentValue float64, shocksPercent []float64, label string) (StressTestResult, error) {
	if err := ValidatePositive("current_value", currentValue); err != nil {
		return StressTestResult{}, err
	}
	if len(shocksPercent) == 0 {
		return StressTestResult{}, errors.New("shocks_percent must contain at least 1 value")
	}
	if err := ValidateReturnPercentSlice("shocks_percent", shocksPercent); err != nil {
		return StressTestResult{}, err
	}
	scenarios := make([]stressScenario, len(shocksPercent))
	worst := 0
	for i, shock := range shocksPercent {
		resulting := Round2(currentValue * (1 + shock/100))
		scenarios[i] = stressScenario{
			ShockPercent:   shock,
			ResultingValue: resulting,
			ChangeAbsolute: Round2(resulting - currentValue),
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
	return StressTestResult{
		CurrentValue: Round2(currentValue),
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
// used by CalcVolatility/CalcSharpe, since drift_annual/volatility_annual are
// expected to come from those same tools (DD-3).
const monteCarloTradingDaysPerYear = 252.0

type MonteCarloResult struct {
	MeanFinalPrice   float64 `json:"mean_final_price"`
	MedianFinalPrice float64 `json:"median_final_price"` // P50
	P5FinalPrice     float64 `json:"p5_final_price"`
	P95FinalPrice    float64 `json:"p95_final_price"`
	ProbAbovePercent float64 `json:"prob_above_start_percent"`
	Days             int     `json:"days"`
	NumSimulations   int     `json:"num_simulations"`
	Summary          string  `json:"summary"`
	ComputedAt       string  `json:"computed_at"`
}

// CalcMonteCarloSimulation simulates the terminal price distribution under
// Geometric Brownian Motion using the closed-form solution
// S(T) = S(0)·exp((μ − ½σ²)·T + σ·√T·Z), Z ~ N(0,1) — only the final-price
// distribution is needed, so this avoids day-by-day path stepping.
func CalcMonteCarloSimulation(lastPrice, driftAnnual, volatilityAnnual float64, days, numSimulations int) (MonteCarloResult, error) {
	if err := ValidatePositive("last_price", lastPrice); err != nil {
		return MonteCarloResult{}, err
	}
	if days <= 0 {
		return MonteCarloResult{}, errors.New("days must be greater than zero")
	}
	if err := ValidateRate("drift_annual", driftAnnual); err != nil {
		return MonteCarloResult{}, err
	}
	if err := ValidateNonNegative("volatility_annual", volatilityAnnual); err != nil {
		return MonteCarloResult{}, err
	}
	if numSimulations <= 0 {
		return MonteCarloResult{}, errors.New("num_simulations must be greater than zero")
	}
	if numSimulations > maxMonteCarloSimulations {
		return MonteCarloResult{}, fmt.Errorf("num_simulations must be <= %d", maxMonteCarloSimulations)
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

	return MonteCarloResult{
		MeanFinalPrice:   Round2(mean),
		MedianFinalPrice: Round2(p50),
		P5FinalPrice:     Round2(p5),
		P95FinalPrice:    Round2(p95),
		ProbAbovePercent: Round4(probAbove),
		Days:             days,
		NumSimulations:   numSimulations,
		Summary: fmt.Sprintf("Monte Carlo (GBM, %d sims, %d days): mean=%.2f, P5=%.2f, P50=%.2f, P95=%.2f, P(up)=%.2f%%",
			numSimulations, days, mean, p5, p50, p95, probAbove),
	}, nil
}

// --- Currency conversion -------------------------------------------------------

type CurrencyResult struct {
	ConvertedAmount float64 `json:"converted_amount"`
	RateUsed        float64 `json:"rate_used"`
	Summary         string  `json:"summary"`
	ComputedAt      string  `json:"computed_at"`
}

func CalcCurrencyConversion(amount float64, fromCurrency, toCurrency string, exchangeRate float64) (CurrencyResult, error) {
	if err := ValidateFinite("amount", amount); err != nil {
		return CurrencyResult{}, err
	}
	if err := ValidatePositive("exchange_rate", exchangeRate); err != nil {
		return CurrencyResult{}, err
	}
	converted := amount * exchangeRate
	return CurrencyResult{
		ConvertedAmount: Round2(converted),
		RateUsed:        exchangeRate,
		Summary:         fmt.Sprintf("%.2f %s = %.2f %s (rate: %.6f)", amount, fromCurrency, converted, toCurrency, exchangeRate),
	}, nil
}

// --- Compound interest ---------------------------------------------------------

type CompoundInterestResult struct {
	FinalAmount         float64 `json:"final_amount"`
	TotalInterest       float64 `json:"total_interest"`
	EffectiveAnnualRate float64 `json:"effective_annual_rate"`
	Summary             string  `json:"summary"`
	ComputedAt          string  `json:"computed_at"`
}

func CalcCompoundInterest(principal, annualRate, years float64, compoundsPerYear int) (CompoundInterestResult, error) {
	if err := ValidatePositive("principal", principal); err != nil {
		return CompoundInterestResult{}, err
	}
	if err := ValidateRate("annual_rate", annualRate); err != nil {
		return CompoundInterestResult{}, err
	}
	if err := ValidateFinite("years", years); err != nil {
		return CompoundInterestResult{}, err
	}
	if compoundsPerYear <= 0 {
		return CompoundInterestResult{}, errors.New("compounds_per_year must be at least 1")
	}
	if years < 0 {
		return CompoundInterestResult{}, errors.New("years must be non-negative")
	}
	n := float64(compoundsPerYear)
	final := principal * math.Pow(1+annualRate/n, n*years)
	interest := final - principal
	ear := (math.Pow(1+annualRate/n, n) - 1) * 100
	return CompoundInterestResult{
		FinalAmount:         Round2(final),
		TotalInterest:       Round2(interest),
		EffectiveAnnualRate: Round4(ear),
		Summary: fmt.Sprintf("Principal %.2f at %.2f%% p.a. × %.1f years (%dx/year) → %.2f (interest: %.2f, EAR: %.4f%%)",
			principal, annualRate*100, years, compoundsPerYear, final, interest, ear),
	}, nil
}
