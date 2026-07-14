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
	"time"
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
		hhiBucket(hhi), divYield*100, beta, len(weights), cashSuffix)
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

// RebalanceOp is a single suggested trade to move a holding's weight toward
// its target.
type RebalanceOp struct {
	Symbol        string  `json:"symbol"`
	Action        string  `json:"action"` // "buy" or "sell"
	Quantity      float64 `json:"quantity"`
	Price         float64 `json:"price"`
	Amount        float64 `json:"amount"` // quantity * price
	CurrentValue  float64 `json:"current_value"`
	TargetValue   float64 `json:"target_value"`
	CurrentWeight float64 `json:"current_weight"` // fraction of basis, in [0, 1]
	TargetWeight  float64 `json:"target_weight"`  // fraction of basis, in [0, 1]
	DriftPercent  float64 `json:"drift_percent"`  // (current_weight - target_weight) * 100
}

// RebalanceResult is the outcome of suggesting a rebalance for a portfolio.
type RebalanceResult struct {
	PortfolioID     string        `json:"portfolio_id"`
	TotalValue      float64       `json:"total_value"` // basis used for target amounts: quoted holdings + cash
	Cash            float64       `json:"cash"`
	MaxDriftPercent float64       `json:"max_drift_percent"`
	Operations      []RebalanceOp `json:"operations"`
	MissingQuotes   []string      `json:"missing_quotes,omitempty"`
	Summary         string        `json:"summary"`
	ComputedAt      string        `json:"computed_at"`
}

// ErrNoTargetAllocation is returned when SuggestRebalance is called on a
// portfolio whose TargetAllocation is unset or empty.
var ErrNoTargetAllocation = errors.New("portfolio: no target_allocation set; call portfolio_set_target_allocation first")

// ErrNoRebalanceBasis is returned when the portfolio has no priced value
// (quoted holdings + cash) to base target amounts on.
var ErrNoRebalanceBasis = errors.New("portfolio: no computable value (missing quotes and no cash) to base a rebalance on")

// SuggestRebalance compares a portfolio's current holdings against its
// TargetAllocation and returns suggested buy/sell operations to close the
// gap. It never mutates the portfolio or executes trades.
//
//   - quotes supplies last prices (and, incidentally, dividend/beta — unused
//     here) for both currently-held symbols and any symbol that appears only
//     in TargetAllocation (a new position not yet held). Symbols missing a
//     usable quote are excluded from Operations and reported in
//     MissingQuotes.
//   - Symbols held but absent from TargetAllocation are treated as target
//     weight 0 (a suggestion to liquidate the position).
//   - maxDriftPercent (percentage points, e.g. 5 for 5%) suppresses
//     operations for symbols whose current weight is already within
//     tolerance of their target; must be >= 0.
//   - The basis for target dollar amounts is TotalValue (quoted current
//     holdings value + cash), so target weights are interpreted as a
//     fraction of the whole portfolio, not just its invested holdings.
func SuggestRebalance(p *Portfolio, quotes map[string]Quote, maxDriftPercent float64) (RebalanceResult, error) {
	if p == nil {
		return RebalanceResult{}, errors.New("portfolio: nil portfolio")
	}
	if len(p.TargetAllocation) == 0 {
		return RebalanceResult{}, ErrNoTargetAllocation
	}
	if maxDriftPercent < 0 {
		return RebalanceResult{}, errors.New("portfolio: max_drift_percent must be >= 0")
	}

	m, err := ComputeMetrics(p, quotes)
	if err != nil && !errors.Is(err, ErrNoHoldings) {
		return RebalanceResult{}, err
	}
	basis := m.CurrentValue + p.Cash
	if basis <= 0 {
		return RebalanceResult{}, ErrNoRebalanceBasis
	}

	// Index currently-held, quoted symbols for O(1) lookup, and the quantity
	// each holding currently has (needed to cap suggested sell quantities).
	type held struct {
		value    float64
		price    float64
		quantity float64
	}
	heldBySymbol := make(map[string]held, len(m.WeightBySymbol))
	for _, w := range m.WeightBySymbol {
		heldBySymbol[w.Symbol] = held{value: w.Value, price: w.LastPrice, quantity: w.Quantity}
	}

	// Union of currently-held (quoted) symbols and target symbols.
	symbols := make(map[string]bool, len(heldBySymbol)+len(p.TargetAllocation))
	for sym := range heldBySymbol {
		symbols[sym] = true
	}
	for sym := range p.TargetAllocation {
		symbols[sym] = true
	}

	missingSet := make(map[string]bool, len(m.MissingQuotes))
	for _, sym := range m.MissingQuotes {
		missingSet[sym] = true
	}
	ops := make([]RebalanceOp, 0, len(symbols))
	for sym := range symbols {
		h, isHeld := heldBySymbol[sym]
		targetWeight := p.TargetAllocation[sym]
		targetValue := basis * targetWeight

		price := h.price
		if !isHeld {
			if q, ok := quotes[sym]; ok && q.Last > 0 {
				price = q.Last
			} else {
				missingSet[sym] = true
				continue
			}
		}

		currentValue := h.value
		currentWeight := 0.0
		if basis > 0 {
			currentWeight = currentValue / basis
		}
		driftPercent := (currentWeight - targetWeight) * 100
		if math.Abs(driftPercent) <= maxDriftPercent {
			continue
		}

		diff := targetValue - currentValue
		op := RebalanceOp{
			Symbol:        sym,
			Price:         round2(price),
			CurrentValue:  round2(currentValue),
			TargetValue:   round2(targetValue),
			CurrentWeight: round4(currentWeight),
			TargetWeight:  round4(targetWeight),
			DriftPercent:  round4(driftPercent),
		}
		if diff > 0 {
			op.Action = "buy"
			op.Quantity = round4(diff / price)
		} else {
			op.Action = "sell"
			qty := -diff / price
			if isHeld && qty > h.quantity {
				qty = h.quantity
			}
			op.Quantity = round4(qty)
		}
		op.Amount = round2(op.Quantity * price)
		ops = append(ops, op)
	}
	missing := make([]string, 0, len(missingSet))
	for sym := range missingSet {
		missing = append(missing, sym)
	}
	sort.Strings(missing)

	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Action != ops[j].Action {
			return ops[i].Action == "sell" // sells before buys
		}
		return ops[i].Amount > ops[j].Amount
	})

	buys, sells := 0, 0
	turnover := 0.0
	for _, op := range ops {
		if op.Action == "buy" {
			buys++
		} else {
			sells++
		}
		turnover += op.Amount
	}
	var summary string
	if len(ops) == 0 {
		summary = fmt.Sprintf("portfolio already within target allocation (basis=%.2f, max_drift=%.2f%%)", basis, maxDriftPercent)
	} else {
		summary = fmt.Sprintf("rebalance vs target: %d buy(s), %d sell(s), turnover=%.2f (basis=%.2f, max_drift=%.2f%%)",
			buys, sells, turnover, basis, maxDriftPercent)
	}

	return RebalanceResult{
		PortfolioID:     p.ID,
		TotalValue:      round2(basis),
		Cash:            round2(p.Cash),
		MaxDriftPercent: round4(maxDriftPercent),
		Operations:      ops,
		MissingQuotes:   missing,
		Summary:         summary,
	}, nil
}

// HoldingsAsOf reconstructs each symbol's held quantity as of a past date by
// replaying Transactions with Date <= at (buys add, sells subtract). Only
// meaningful for portfolios with transaction history (see FEAT-2): a
// portfolio created before that field existed has no Transactions to replay,
// so this returns an empty map — callers must check len(p.Transactions) > 0
// before trusting the result and fall back to treating current holdings as
// unchanged over the period otherwise (a buy-and-hold approximation).
func HoldingsAsOf(p *Portfolio, at time.Time) map[string]float64 {
	qty := make(map[string]float64)
	for _, tx := range p.Transactions {
		if tx.Date.After(at) {
			continue
		}
		switch tx.Type {
		case TransactionBuy:
			qty[tx.Symbol] += tx.Quantity
		case TransactionSell:
			qty[tx.Symbol] -= tx.Quantity
		}
	}
	return qty
}

// CashAsOf reconstructs the cash balance as of a past date by undoing every
// transaction recorded after that date from the current balance. Same
// transaction-history caveat as HoldingsAsOf applies.
func CashAsOf(p *Portfolio, at time.Time) float64 {
	cash := p.Cash
	for _, tx := range p.Transactions {
		if tx.Date.After(at) {
			cash -= tx.CashDelta
		}
	}
	return cash
}

// ErrPeriodInvalidRange is returned by ComputePeriodReturn when to is not
// strictly after from.
var ErrPeriodInvalidRange = errors.New("portfolio: to must be after from")

// ErrPeriodZeroDenominator is returned by ComputePeriodReturn when the
// Modified Dietz denominator (start value adjusted for time-weighted external
// flows) is zero or negative, making the period return undefined.
var ErrPeriodZeroDenominator = errors.New("portfolio: modified Dietz denominator is zero or negative; cannot compute period return")

// PeriodReturnResult is the outcome of ComputePeriodReturn.
type PeriodReturnResult struct {
	ReturnPercent   float64 `json:"return_percent"`
	StartValue      float64 `json:"start_value"`
	EndValue        float64 `json:"end_value"`
	NetExternalFlow float64 `json:"net_external_flow"` // sum of deposit/withdrawal/adjustment cash deltas within (from, to]
}

// ComputePeriodReturn computes a portfolio's real return over (from, to]
// using the Modified Dietz method: external cash flows (deposits,
// withdrawals, and corrective portfolio_set_cash adjustments — never buys,
// sells, or dividends, which are internal to the portfolio's own
// performance) are weighted by how much of the period remained when they
// occurred, so money added on day one counts almost fully toward the
// denominator while money added on the last day barely does.
//
// startValue and endValue are the portfolio's total value (holdings + cash)
// at the start and end of the period; the caller computes these (start
// typically via HoldingsAsOf/CashAsOf plus historical prices, end via
// ComputeMetrics plus current quotes), since pricing requires a market
// provider this package does not depend on.
func ComputePeriodReturn(p *Portfolio, from, to time.Time, startValue, endValue float64) (PeriodReturnResult, error) {
	if p == nil {
		return PeriodReturnResult{}, errors.New("portfolio: nil portfolio")
	}
	if !to.After(from) {
		return PeriodReturnResult{}, ErrPeriodInvalidRange
	}
	totalDays := to.Sub(from).Hours() / 24
	netFlow := 0.0
	weightedFlow := 0.0
	for _, tx := range p.Transactions {
		if tx.Type != TransactionDeposit && tx.Type != TransactionWithdrawal && tx.Type != TransactionAdjustment {
			continue
		}
		if tx.Date.Before(from) || tx.Date.After(to) {
			continue
		}
		netFlow += tx.CashDelta
		weight := 0.0
		if totalDays > 0 {
			weight = to.Sub(tx.Date).Hours() / 24 / totalDays
		}
		weightedFlow += tx.CashDelta * weight
	}
	denominator := startValue + weightedFlow
	if denominator <= 0 {
		return PeriodReturnResult{}, ErrPeriodZeroDenominator
	}
	ret := (endValue - startValue - netFlow) / denominator * 100
	return PeriodReturnResult{
		ReturnPercent:   round4(ret),
		StartValue:      round2(startValue),
		EndValue:        round2(endValue),
		NetExternalFlow: round2(netFlow),
	}, nil
}
