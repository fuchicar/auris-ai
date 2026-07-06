// Package portfolio — tax.go
//
// CalculateTaxPnL classifies every realized sale in a portfolio's
// transaction log into short-term (held <= TaxHoldingPeriodDays) and
// long-term (held > TaxHoldingPeriodDays) capital gains, the holding-period
// split most tax jurisdictions use to separate ordinary-income-rate gains
// from preferential long-term-rate gains. It relies entirely on the
// per-lot detail already captured in Transaction.ConsumedLots (see FEAT-2,
// transaction.go) and, like the rest of this package, never calls an
// external service.
package portfolio

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

// TaxHoldingPeriodDays is the holding-period threshold, in days, that
// separates short-term from long-term capital gains: a lot held for
// TaxHoldingPeriodDays or fewer is short-term; strictly more is long-term.
const TaxHoldingPeriodDays = 365

// Term labels used in TaxLotDetail.Term.
const (
	TaxTermShort = "short_term"
	TaxTermLong  = "long_term"
)

// TaxLotDetail is a single consumed lot's realized P&L, classified by
// holding period. One is emitted per LotConsumption entry across every
// in-scope sell Transaction.
type TaxLotDetail struct {
	Symbol        string    `json:"symbol"`
	LotID         string    `json:"lot_id"`
	Quantity      float64   `json:"quantity"`
	PurchaseDate  time.Time `json:"purchase_date"`
	PurchasePrice float64   `json:"purchase_price"`
	SaleDate      time.Time `json:"sale_date"`
	SalePrice     float64   `json:"sale_price"`
	HoldingDays   int       `json:"holding_days"`
	Term          string    `json:"term"` // TaxTermShort or TaxTermLong
	RealizedPnL   float64   `json:"realized_pnl"`
}

// TaxSymbolSummary aggregates TaxLotDetail rows for one symbol into
// short-term / long-term / total subtotals.
type TaxSymbolSummary struct {
	Symbol        string  `json:"symbol"`
	ShortTermPnL  float64 `json:"short_term_pnl"`
	LongTermPnL   float64 `json:"long_term_pnl"`
	TotalPnL      float64 `json:"total_pnl"`
	ShortTermLots int     `json:"short_term_lots"`
	LongTermLots  int     `json:"long_term_lots"`
}

// TaxPnLResult is the outcome of CalculateTaxPnL: every consumed lot's
// realized P&L, classified by holding period, grouped by symbol, with grand
// totals over the whole portfolio (or the requested sale-date window).
type TaxPnLResult struct {
	PortfolioID       string             `json:"portfolio_id"`
	From              string             `json:"from,omitempty"` // ISO 8601; empty means unbounded
	To                string             `json:"to,omitempty"`   // ISO 8601; empty means unbounded
	Lots              []TaxLotDetail     `json:"lots"`
	BySymbol          []TaxSymbolSummary `json:"by_symbol"`
	TotalShortTermPnL float64            `json:"total_short_term_pnl"`
	TotalLongTermPnL  float64            `json:"total_long_term_pnl"`
	TotalPnL          float64            `json:"total_pnl"`
	Summary           string             `json:"summary"`
	ComputedAt        string             `json:"computed_at"`
}

// CalculateTaxPnL walks every sell Transaction in p.Transactions whose sale
// Date falls within [from, to] and classifies each of its ConsumedLots as
// short-term or long-term by the number of days between the lot's original
// purchase Date and the sale's Date. Either from or to may be the zero
// time.Time to mean "unbounded" on that side; passing both zero reports
// every recorded sale.
//
// Portfolios with no Transactions (created before FEAT-2 — see
// HoldingsAsOf/CashAsOf for the same caveat) or with Transactions but no
// sell events in range yield a zero-value result (empty Lots/BySymbol, zero
// totals), not an error: a buy-and-hold portfolio that has never sold, or a
// tax year with no disposals, is a normal and common state, not a failure.
func CalculateTaxPnL(p *Portfolio, from, to time.Time) (TaxPnLResult, error) {
	if p == nil {
		return TaxPnLResult{}, errors.New("portfolio: nil portfolio")
	}
	if !from.IsZero() && !to.IsZero() && !to.After(from) {
		return TaxPnLResult{}, ErrPeriodInvalidRange
	}

	type symTotals struct {
		shortPnL  float64
		longPnL   float64
		shortLots int
		longLots  int
	}
	totalsBySymbol := make(map[string]*symTotals)
	lots := make([]TaxLotDetail, 0)
	var totalShort, totalLong float64

	for _, tx := range p.Transactions {
		if tx.Type != TransactionSell {
			continue
		}
		if !from.IsZero() && tx.Date.Before(from) {
			continue
		}
		if !to.IsZero() && tx.Date.After(to) {
			continue
		}
		for _, lc := range tx.ConsumedLots {
			holdingDaysF := tx.Date.Sub(lc.Date).Hours() / 24
			term := TaxTermShort
			if holdingDaysF > TaxHoldingPeriodDays {
				term = TaxTermLong
			}
			pnl := lc.Quantity * (tx.Price - lc.Price)

			lots = append(lots, TaxLotDetail{
				Symbol:        tx.Symbol,
				LotID:         lc.LotID,
				Quantity:      lc.Quantity,
				PurchaseDate:  lc.Date,
				PurchasePrice: round2(lc.Price),
				SaleDate:      tx.Date,
				SalePrice:     round2(tx.Price),
				HoldingDays:   int(math.Round(holdingDaysF)),
				Term:          term,
				RealizedPnL:   round2(pnl),
			})

			st, ok := totalsBySymbol[tx.Symbol]
			if !ok {
				st = &symTotals{}
				totalsBySymbol[tx.Symbol] = st
			}
			if term == TaxTermLong {
				st.longPnL += pnl
				st.longLots++
				totalLong += pnl
			} else {
				st.shortPnL += pnl
				st.shortLots++
				totalShort += pnl
			}
		}
	}

	bySymbol := make([]TaxSymbolSummary, 0, len(totalsBySymbol))
	for sym, st := range totalsBySymbol {
		bySymbol = append(bySymbol, TaxSymbolSummary{
			Symbol:        sym,
			ShortTermPnL:  round2(st.shortPnL),
			LongTermPnL:   round2(st.longPnL),
			TotalPnL:      round2(st.shortPnL + st.longPnL),
			ShortTermLots: st.shortLots,
			LongTermLots:  st.longLots,
		})
	}
	sort.Slice(bySymbol, func(i, j int) bool { return bySymbol[i].Symbol < bySymbol[j].Symbol })
	sort.Slice(lots, func(i, j int) bool {
		if !lots[i].SaleDate.Equal(lots[j].SaleDate) {
			return lots[i].SaleDate.Before(lots[j].SaleDate)
		}
		return lots[i].Symbol < lots[j].Symbol
	})

	result := TaxPnLResult{
		PortfolioID:       p.ID,
		Lots:              lots,
		BySymbol:          bySymbol,
		TotalShortTermPnL: round2(totalShort),
		TotalLongTermPnL:  round2(totalLong),
		TotalPnL:          round2(totalShort + totalLong),
	}
	if !from.IsZero() {
		result.From = from.Format(time.RFC3339)
	}
	if !to.IsZero() {
		result.To = to.Format(time.RFC3339)
	}
	result.Summary = summariseTaxPnL(result)
	return result, nil
}

// summariseTaxPnL produces a one-line human-readable summary, same purpose
// as the Summary field on other portfolio result types in this package.
func summariseTaxPnL(r TaxPnLResult) string {
	if len(r.Lots) == 0 {
		return "no realized sales in range; nothing to report"
	}
	return fmt.Sprintf("%d lot(s) across %d symbol(s): short_term=%.2f, long_term=%.2f, total=%.2f",
		len(r.Lots), len(r.BySymbol), r.TotalShortTermPnL, r.TotalLongTermPnL, r.TotalPnL)
}
