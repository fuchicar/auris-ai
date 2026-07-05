package tui

import (
	"strings"
	"testing"
	"time"

	"auris/pkg/locale"
	"auris/pkg/portfolio"
)

func TestNewPortfolioTransactionsModel_NewestFirst(t *testing.T) {
	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	p := &portfolio.Portfolio{
		Transactions: []portfolio.Transaction{
			{ID: "1", Type: portfolio.TransactionBuy, Date: older},
			{ID: "2", Type: portfolio.TransactionSell, Date: newer},
		},
	}

	m := newPortfolioTransactionsModel(p, NewStyles(ThemeDark))

	if len(m.rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(m.rows))
	}
	if m.rows[0].ID != "2" || m.rows[1].ID != "1" {
		t.Errorf("rows not newest-first: got order %s, %s", m.rows[0].ID, m.rows[1].ID)
	}
}

func TestNewPortfolioTransactionsModel_Empty(t *testing.T) {
	p := &portfolio.Portfolio{}
	m := newPortfolioTransactionsModel(p, NewStyles(ThemeDark))
	if len(m.rows) != 0 {
		t.Errorf("want 0 rows for a portfolio with no transactions, got %d", len(m.rows))
	}
}

func TestTxTypeLabel_AllTypesResolve(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatal(err)
	}
	types := []portfolio.TransactionType{
		portfolio.TransactionBuy,
		portfolio.TransactionSell,
		portfolio.TransactionDividend,
		portfolio.TransactionDeposit,
		portfolio.TransactionWithdrawal,
		portfolio.TransactionAdjustment,
	}
	for _, ty := range types {
		label := txTypeLabel(ty)
		if label == "" {
			t.Errorf("txTypeLabel(%q) returned empty string", ty)
		}
		if strings.HasPrefix(label, "portfolio.transactions.type.") {
			t.Errorf("txTypeLabel(%q) = %q, looks like a missing-translation fallback", ty, label)
		}
	}
}

func TestFormatTransactionRow_SellShowsRealizedPnL(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatal(err)
	}
	tx := portfolio.Transaction{
		Type: portfolio.TransactionSell, Symbol: "AAPL",
		Quantity: 10, Price: 150, CashDelta: 1500, RealizedPnL: 42.5,
		Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	row := formatTransactionRow(tx)
	if !strings.Contains(row, "2024-06-01") {
		t.Errorf("row = %q, want it to contain the date 2024-06-01", row)
	}
	if !strings.Contains(row, "AAPL") {
		t.Errorf("row = %q, want it to contain the symbol AAPL", row)
	}
	if !strings.Contains(row, "+42.50") {
		t.Errorf("row = %q, want it to contain the realized P&L +42.50", row)
	}
}

func TestFormatTransactionRow_NonSellOmitsRealizedPnL(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatal(err)
	}
	tx := portfolio.Transaction{
		Type: portfolio.TransactionDeposit, CashDelta: 500,
		Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	row := formatTransactionRow(tx)
	if !strings.Contains(row, "—") {
		t.Errorf("row = %q, want a placeholder dash where realized P&L would be for a non-sell", row)
	}
}
