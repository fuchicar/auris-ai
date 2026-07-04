// Package portfolio — metrics.go
//
// Aggregation functions that compute derived quantities (current value, P&L,
// concentration, dividend yield, weighted beta) for a Portfolio. Kept here
// rather than in pkg/agent so the same calculations can be reused by the TUI
// or batch jobs without going through the LLM loop.
//
// The functions in this file are pure: they do not call any external service.
// Market prices and dividend streams are passed in by the caller (which in the
// agent path is the dispatcher, after it has fetched quotes from the market
// provider). This keeps the package free of provider dependencies and easy
// to test.
package portfolio

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// Quote is the minimum market data slice needed to compute a portfolio's
// current valuation. The agent constructs these from market.Quote values it
// has already fetched; keeping a local type avoids importing pkg/market here.
type Quote struct {
	Last             float64 // last traded price
	DividendYieldTTM float64 // annualised trailing-twelve-month dividend yield as a fraction (e.g. 0.025 for 2.5 %)
	Beta             float64 // asset beta vs benchmark; 0 if unknown
}

// PortfolioMetrics is the aggregated view of a portfolio at a point in time.
//
// RealisedPnL and CostBasis are derived from the portfolio's lots and never
// need external data. All other monetary fields (CurrentValue, UnrealisedPnL,
// WeightBySymbol) require Quotes; when a symbol has no quote, its contribution
// is skipped and MissingQuotes is populated so the caller can warn the LLM.
type PortfolioMetrics struct {
	PortfolioID       string         `json:"portfolio_id"`
	HoldingCount      int            `json:"holding_count"`
	LotCount          int            `json:"lot_count"`
	CostBasis         float64        `json:"cost_basis"`
	RealisedPnL       float64        `json:"realised_pnl"`
	CurrentValue      float64        `json:"current_value"`      // sum of qty*last_price over holdings with quotes
	Cash              float64        `json:"cash"`               // portfolio's liquid cash balance
	TotalValue        float64        `json:"total_value"`        // current_value + cash
	UnrealisedPnL     float64        `json:"unrealised_pnl"`     // current_value - cost_basis_of_quoted_holdings
	TotalPnLAbsolute  float64        `json:"total_pnl_absolute"` // realised + unrealised
	TotalPnLPercent   float64        `json:"total_pnl_percent"`  // total / cost_basis * 100; 0 if cost_basis == 0
	WeightBySymbol    []SymbolWeight `json:"weight_by_symbol"`
	HHI               float64        `json:"hhi"`                // Herfindahl-Hirschman Index in [0, 1]
	HHIInterpretation string         `json:"hhi_interpretation"` // "concentrated", "moderate", "diversified"
	DividendYield     float64        `json:"dividend_yield"`     // weighted by current value; 0 if no quotes
	WeightedBeta      float64        `json:"weighted_beta"`      // weighted by current value; 0 if no quotes with beta
	MissingQuotes     []string       `json:"missing_quotes,omitempty"`
	Summary           string         `json:"summary"`
	ComputedAt        string         `json:"computed_at"` // ISO 8601 timestamp set by caller
}

// SymbolWeight represents a single holding's contribution to the portfolio's
// current value. Values are absolute (currency) and relative (fraction of
// the quoted total). Sorted descending by Value.
type SymbolWeight struct {
	Symbol     string  `json:"symbol"`
	Quantity   float64 `json:"quantity"`
	LastPrice  float64 `json:"last_price"`
	Value      float64 `json:"value"`
	Weight     float64 `json:"weight"`     // fraction of total quoted value in [0, 1]
	CostBasis  float64 `json:"cost_basis"` // sum of qty*lot_price
	Unrealised float64 `json:"unrealised"` // value - cost_basis
}

// ErrNoHoldings is returned when ComputeMetrics is called on a portfolio with
// no instruments of type InstrumentHolding.
var ErrNoHoldings = errors.New("portfolio: no holdings to compute metrics for")

// ComputeMetrics aggregates a portfolio's metrics.
//
//   - p is the portfolio to analyse.
//   - quotes is a map of ticker → Quote. Symbols not present in the map are
//     skipped from valuation and concentration, but their lots still count
//     toward cost basis and realised P&L. Missing symbols are listed in
//     PortfolioMetrics.MissingQuotes so the caller can surface a warning.
//
// Holdings without any lots (e.g. a watchlist entry mislabelled as a holding)
// contribute zero to cost basis and are skipped from valuation.
func ComputeMetrics(p *Portfolio, quotes map[string]Quote) (PortfolioMetrics, error) {
	if p == nil {
		return PortfolioMetrics{}, errors.New("portfolio: nil portfolio")
	}
	holdings := 0
	lots := 0
	for _, ins := range p.Instruments {
		if ins.Type != InstrumentHolding {
			continue
		}
		holdings++
		lots += len(ins.Lots)
	}
	if holdings == 0 && p.Cash <= 0 {
		return PortfolioMetrics{}, ErrNoHoldings
	}

	// First pass: cost basis per symbol and total cost basis (all holdings).
	// Done before valuation so we can compute unrealised P&L per holding.
	type costRow struct {
		quantity  float64
		costBasis float64
	}
	perSymbolCost := make(map[string]costRow, len(p.Instruments))
	costBasisTotal := 0.0
	for _, ins := range p.Instruments {
		if ins.Type != InstrumentHolding {
			continue
		}
		qty := ins.TotalQuantity()
		var cost float64
		for _, l := range ins.Lots {
			cost += l.Quantity * l.Price
		}
		perSymbolCost[ins.Symbol] = costRow{quantity: qty, costBasis: cost}
		costBasisTotal += cost
	}

	// Second pass: build per-symbol weights using only holdings with quotes.
	weights := make([]SymbolWeight, 0, len(p.Instruments))
	currentValue := 0.0
	unrealised := 0.0
	dividendContribution := 0.0
	weightedBeta := 0.0
	betaDenominator := 0.0
	missing := make([]string, 0)
	for _, ins := range p.Instruments {
		if ins.Type != InstrumentHolding {
			continue
		}
		row := perSymbolCost[ins.Symbol]
		if row.quantity <= 0 {
			// Holding with no lots: skip valuation but keep in cost basis.
			continue
		}
		q, ok := quotes[ins.Symbol]
		if !ok || q.Last <= 0 {
			missing = append(missing, ins.Symbol)
			continue
		}
		value := row.quantity * q.Last
		upnl := value - row.costBasis
		currentValue += value
		unrealised += upnl
		if q.DividendYieldTTM > 0 {
			dividendContribution += q.DividendYieldTTM * value
		}
		if q.Beta != 0 {
			weightedBeta += q.Beta * value
			betaDenominator += value
		}
		weights = append(weights, SymbolWeight{
			Symbol:     ins.Symbol,
			Quantity:   row.quantity,
			LastPrice:  q.Last,
			Value:      value,
			CostBasis:  row.costBasis,
			Unrealised: upnl,
		})
	}
	// Normalise weights to fractions of the quoted total. We do this after
	// summing so weights reflect each holding's share of what we could price.
	for i := range weights {
		if currentValue > 0 {
			weights[i].Weight = weights[i].Value / currentValue
		}
	}
	sort.Slice(weights, func(i, j int) bool {
		return weights[i].Value > weights[j].Value
	})

	// Concentration: Herfindahl-Hirschman Index (sum of squared weights, in
	// [0, 1] when weights are fractions). Bins are common-wisdom thresholds,
	// not regulatory ones — this is a portfolio tool, not an antitrust one.
	hhi := 0.0
	for _, w := range weights {
		hhi += w.Weight * w.Weight
	}
	hhiInterp := "diversified"
	switch {
	case hhi >= 0.5:
		hhiInterp = "concentrated"
	case hhi >= 0.25:
		hhiInterp = "moderate"
	}

	dividendYield := 0.0
	if currentValue > 0 {
		dividendYield = dividendContribution / currentValue
	}
	if betaDenominator > 0 {
		weightedBeta /= betaDenominator
	}

	totalPnL := p.RealizedPnL + unrealised
	totalPnLPct := 0.0
	if costBasisTotal > 0 {
		totalPnLPct = totalPnL / costBasisTotal * 100
	}

	sort.Strings(missing)

	return PortfolioMetrics{
		PortfolioID:       p.ID,
		HoldingCount:      holdings,
		LotCount:          lots,
		CostBasis:         round2(costBasisTotal),
		RealisedPnL:       round2(p.RealizedPnL),
		CurrentValue:      round2(currentValue),
		Cash:              round2(p.Cash),
		TotalValue:        round2(currentValue + p.Cash),
		UnrealisedPnL:     round2(unrealised),
		TotalPnLAbsolute:  round2(totalPnL),
		TotalPnLPercent:   round4(totalPnLPct),
		WeightBySymbol:    weights,
		HHI:               round4(hhi),
		HHIInterpretation: hhiInterp,
		DividendYield:     round4(dividendYield * 100), // store as percent for readability
		WeightedBeta:      round4(weightedBeta),
		MissingQuotes:     missing,
		Summary:           summarise(weights, hhi, dividendYield, weightedBeta, currentValue, p.RealizedPnL, unrealised, p.Cash),
	}, nil
}

// summarise produces a one-line human-readable summary that mirrors the JSON
// fields, intended for tools whose LLM caller prefers prose over structured
// output. Kept short on purpose: the model can elaborate.
func summarise(weights []SymbolWeight, hhi, divYield, beta, currentValue, realised, unrealised, cash float64) string {
	cashSuffix := ""
	if cash > 0 {
		cashSuffix = fmt.Sprintf(", cash=%.2f, total_value=%.2f", cash, currentValue+cash)
	}
	if len(weights) == 0 {
		return "no quoted holdings; realised P&L only" + cashSuffix
	}
	return fmt.Sprintf("value=%.2f, realised=%.2f, unrealised=%.2f, HHI=%.4f (%s), div_yield=%.4f%%, β=%.4f across %d holdings%s",
		currentValue, realised, unrealised, hhi,
		hhiBucket(hhi), divYield, beta, len(weights), cashSuffix)
}

// hhiBucket returns the same label summarise uses, exposed as a helper so
// callers can format it without recomputing the thresholds.
func hhiBucket(hhi float64) string {
	switch {
	case hhi >= 0.5:
		return "concentrated"
	case hhi >= 0.25:
		return "moderate"
	default:
		return "diversified"
	}
}

// round2 mirrors the helper in pkg/agent; duplicated here to avoid an import
// cycle (pkg/agent already imports pkg/portfolio).
func round2(v float64) float64 { return math.Round(v*100) / 100 }

// round4 mirrors the helper in pkg/agent; duplicated for the same reason.
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
