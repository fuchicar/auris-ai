// Package portfolio — transaction.go
//
// Transaction is the append-only record of every cash-flow or position event
// on a portfolio: buys, sells, dividends received, deposits/withdrawals, and
// corrective cash adjustments. Transactions are the system of record for
// Portfolio.Cash — every mutation of Cash must go through RecordTransaction
// so the running balance and the log can never drift apart.
package portfolio

import "time"

// TransactionType classifies a cash-flow or position event recorded against
// a portfolio.
type TransactionType string

const (
	TransactionBuy        TransactionType = "buy"
	TransactionSell       TransactionType = "sell"
	TransactionDividend   TransactionType = "dividend"
	TransactionDeposit    TransactionType = "deposit"
	TransactionWithdrawal TransactionType = "withdrawal"
	// TransactionAdjustment records either a corrective portfolio_set_cash
	// overwrite, or a portfolio_add_lot call with debit_cash=false cataloging
	// a lot already owned (CashDelta left at 0) — kept distinct from
	// Deposit/Withdrawal/Buy so neither a real-world cash movement the user
	// reports, nor a real purchase, is ever conflated with a non-cash
	// bookkeeping event.
	TransactionAdjustment TransactionType = "adjustment"
)

// LotConsumption records the portion of a single lot consumed by a sell, so a
// sell Transaction carries enough detail to reconstruct short-term vs
// long-term realized P&L later without re-deriving it from a RemainingLots
// diff.
type LotConsumption struct {
	LotID    string    `json:"lot_id"`
	Quantity float64   `json:"quantity"`
	Price    float64   `json:"price"` // the consumed lot's original purchase price/unit
	Date     time.Time `json:"date"`  // the consumed lot's original purchase date
}

// Transaction is a single recorded cash-flow or position event on a
// portfolio: a buy, a sell, a dividend receipt, a deposit/withdrawal, or a
// corrective cash adjustment.
type Transaction struct {
	ID           string           `json:"id"`
	Type         TransactionType  `json:"type"`
	Symbol       string           `json:"symbol,omitempty"`        // empty for deposit/withdrawal/portfolio_set_cash adjustment
	Quantity     float64          `json:"quantity,omitempty"`      // buy/sell, and adjustment from portfolio_add_lot(debit_cash=false)
	Price        float64          `json:"price,omitempty"`         // per-unit price: buy/sell, and adjustment from portfolio_add_lot(debit_cash=false)
	CashDelta    float64          `json:"cash_delta"`              // signed effect on Cash: +credit / -debit
	RealizedPnL  float64          `json:"realized_pnl,omitempty"`  // sell only
	ConsumedLots []LotConsumption `json:"consumed_lots,omitempty"` // sell only
	Note         string           `json:"note,omitempty"`
	Date         time.Time        `json:"date"`
}

// RecordTransaction assigns a generated ID to tx, appends it to the
// portfolio's transaction log, and applies tx.CashDelta to Cash — the one
// place both mutations happen together, so no call site can update Cash
// without leaving a matching record (or vice versa). Returns the finalized
// transaction (with its generated ID) for the caller to surface in a tool
// response.
func (p *Portfolio) RecordTransaction(tx Transaction) Transaction {
	tx.ID = newTransactionID()
	p.Transactions = append(p.Transactions, tx)
	p.Cash += tx.CashDelta
	return tx
}
