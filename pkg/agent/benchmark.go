package agent

import (
	"fmt"
	"time"

	"auris/pkg/finance"
)

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
// already-computed numbers, so the assembly logic (alpha, beta via
// finance.CalcBeta, summary wording) is unit-testable without a market
// provider. The dispatch case in tools.go owns fetching candles/quotes and
// reconstructing start-of-period holdings; this function only combines the
// results.
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
		PortfolioReturnPercent:  finance.Round4(portfolioReturnPercent),
		BenchmarkReturnPercent:  finance.Round4(benchmarkReturnPercent),
		AlphaPercent:            finance.Round4(alpha),
		ReturnMethod:            returnMethod,
		MissingHistoricalPrices: missingHistorical,
	}

	betaNote := "beta not available (insufficient overlapping daily data)"
	if len(portfolioReturns) >= 2 && len(portfolioReturns) == len(benchmarkReturns) {
		if b, err := finance.CalcBeta(portfolioReturns, benchmarkReturns); err == nil {
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
