package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"auris/pkg/llm"
	"auris/pkg/portfolio"
)

func TestDispatch_PortfolioCalculateTaxPnL_OK(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	buyDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	sellDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	p := portfolio.NewPortfolio("test")
	p.Transactions = []portfolio.Transaction{
		{
			Type: portfolio.TransactionSell, Symbol: "AAPL", Quantity: 10, Price: 150, Date: sellDate,
			ConsumedLots: []portfolio.LotConsumption{
				{LotID: "lot1", Quantity: 10, Price: 100, Date: buyDate},
			},
		},
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_tax_pnl", Arguments: "{}"},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	var res portfolio.TaxPnLResult
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if len(res.Lots) != 1 {
		t.Fatalf("expected 1 lot, got %d", len(res.Lots))
	}
	if res.Lots[0].Term != portfolio.TaxTermLong {
		t.Errorf("expected long_term (held ~17 months), got %s", res.Lots[0].Term)
	}
	if res.TotalLongTermPnL != 500 {
		t.Errorf("expected total long term pnl 500, got %v", res.TotalLongTermPnL)
	}
	if res.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
}

func TestDispatch_PortfolioCalculateTaxPnL_NoTransactions_EmptyResult(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}
	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_tax_pnl", Arguments: "{}"},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var res portfolio.TaxPnLResult
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if len(res.Lots) != 0 {
		t.Errorf("expected empty result for portfolio with no transactions, got %+v", res)
	}
}

func TestDispatch_PortfolioCalculateTaxPnL_DateRangeFilter(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	buyDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	saleInRange := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	saleOutOfRange := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	p := portfolio.NewPortfolio("test")
	p.Transactions = []portfolio.Transaction{
		{Type: portfolio.TransactionSell, Symbol: "AAPL", Quantity: 1, Price: 110, Date: saleInRange,
			ConsumedLots: []portfolio.LotConsumption{{LotID: "in-range", Quantity: 1, Price: 100, Date: buyDate}}},
		{Type: portfolio.TransactionSell, Symbol: "AAPL", Quantity: 1, Price: 120, Date: saleOutOfRange,
			ConsumedLots: []portfolio.LotConsumption{{LotID: "out-of-range", Quantity: 1, Price: 100, Date: buyDate}}},
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}
	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"from": "2026-01-01", "to": "2026-12-31"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_tax_pnl", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var res portfolio.TaxPnLResult
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if len(res.Lots) != 1 || res.Lots[0].LotID != "in-range" {
		t.Fatalf("expected only the in-range lot, got %+v", res.Lots)
	}
}

func TestDispatch_PortfolioCalculateTaxPnL_PortfolioNotFound(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	a := New(&mockLLM{}, &mockMarket{}, "")

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"portfolio_id": "does-not-exist"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_tax_pnl", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("expected error for missing portfolio, got %s", result)
	}
}

func TestDispatch_PortfolioCalculateTaxPnL_NoPortfolioIDAndNoCurrent(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	a := New(&mockLLM{}, &mockMarket{}, "")

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_calculate_tax_pnl", Arguments: "{}"},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("expected error when no portfolio_id and no current portfolio, got %s", result)
	}
}
