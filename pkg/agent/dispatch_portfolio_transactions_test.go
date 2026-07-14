package agent

import (
	"context"
	"testing"

	"auris/pkg/llm"
	"auris/pkg/portfolio"
)

// ---- portfolio_add_lot / portfolio_add_instrument: cash debit ----------------

func TestDispatch_PortfolioAddLot_DebitsCash(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 1000
	p.Instruments = []portfolio.Instrument{
		{ID: "ins-AAPL", Symbol: "AAPL", Name: "AAPL", Type: portfolio.InstrumentHolding},
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"symbol": "AAPL", "quantity": 5.0, "price": 100.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_add_lot", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Cash != 500 {
		t.Errorf("Cash = %v, want 500", reloaded.Cash)
	}
	if len(reloaded.Transactions) != 1 {
		t.Fatalf("want 1 transaction, got %d", len(reloaded.Transactions))
	}
	tx := reloaded.Transactions[0]
	if tx.Type != portfolio.TransactionBuy || tx.CashDelta != -500 || tx.Symbol != "AAPL" {
		t.Errorf("transaction = %+v, want Type=buy CashDelta=-500 Symbol=AAPL", tx)
	}
}

func TestDispatch_PortfolioAddInstrument_WithInitialLot_DebitCashTrue_DebitsCash(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 1000
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"symbol": "MSFT", "name": "Microsoft", "instrument_type": "holding",
		"quantity": 2.0, "price": 300.0, "debit_cash": true,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_add_instrument", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Cash != 400 {
		t.Errorf("Cash = %v, want 400", reloaded.Cash)
	}
	if len(reloaded.Transactions) != 1 || reloaded.Transactions[0].Type != portfolio.TransactionBuy {
		t.Errorf("Transactions = %+v, want 1 buy transaction", reloaded.Transactions)
	}
}

func TestDispatch_PortfolioAddInstrument_WithInitialLot_DefaultDoesNotDebitCash(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 1000
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"symbol": "MSFT", "name": "Microsoft", "instrument_type": "holding",
		"quantity": 2.0, "price": 300.0,
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_add_instrument", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Cash != 1000 {
		t.Errorf("Cash = %v, want unchanged 1000 (debit_cash defaults to false)", reloaded.Cash)
	}
	if len(reloaded.Transactions) != 0 {
		t.Errorf("Transactions = %+v, want 0 transactions when debit_cash is not set", reloaded.Transactions)
	}
	if len(reloaded.Instruments) != 1 || len(reloaded.Instruments[0].Lots) != 1 {
		t.Fatalf("want 1 instrument with 1 lot, got %+v", reloaded.Instruments)
	}
	lot := reloaded.Instruments[0].Lots[0]
	if lot.Quantity != 2 || lot.Price != 300 {
		t.Errorf("lot = %+v, want Quantity=2 Price=300 (cost basis still recorded)", lot)
	}
}

func TestDispatch_PortfolioAddInstrument_WatchlistNoLot_NoTransaction(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 1000
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"symbol": "TSLA", "name": "Tesla", "instrument_type": "watchlist",
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_add_instrument", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Cash != 1000 {
		t.Errorf("Cash = %v, want unchanged 1000", reloaded.Cash)
	}
	if len(reloaded.Transactions) != 0 {
		t.Errorf("want 0 transactions for a watchlist add, got %d", len(reloaded.Transactions))
	}
}

// ---- portfolio_sell: cash credit + consumed lots -----------------------------

func TestDispatch_PortfolioSell_CreditsCashAndRecordsConsumedLots(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 0
	p.Instruments = []portfolio.Instrument{
		{
			ID: "ins-AAPL", Symbol: "AAPL", Name: "AAPL", Type: portfolio.InstrumentHolding,
			Lots: []portfolio.Lot{
				{ID: "lot-1", Quantity: 10, Price: 100},
				{ID: "lot-2", Quantity: 5, Price: 120},
			},
		},
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"symbol": "AAPL", "quantity": 12.0, "sell_price": 150.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_sell", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Cash != 12*150 {
		t.Errorf("Cash = %v, want %v", reloaded.Cash, 12*150.0)
	}
	wantPnL := 10*(150.0-100.0) + 2*(150.0-120.0)
	if reloaded.RealizedPnL != wantPnL {
		t.Errorf("RealizedPnL = %v, want %v", reloaded.RealizedPnL, wantPnL)
	}
	if len(reloaded.Transactions) != 1 {
		t.Fatalf("want 1 transaction, got %d", len(reloaded.Transactions))
	}
	tx := reloaded.Transactions[0]
	if tx.Type != portfolio.TransactionSell || tx.CashDelta != 12*150 || tx.RealizedPnL != wantPnL {
		t.Errorf("transaction = %+v, want Type=sell CashDelta=%v RealizedPnL=%v", tx, 12*150.0, wantPnL)
	}
	if len(tx.ConsumedLots) != 2 {
		t.Fatalf("want 2 consumed lots, got %d", len(tx.ConsumedLots))
	}
	if tx.ConsumedLots[0].LotID != "lot-1" || tx.ConsumedLots[1].LotID != "lot-2" {
		t.Errorf("ConsumedLots = %+v, want lot-1 then lot-2", tx.ConsumedLots)
	}
}

func TestDispatch_PortfolioSell_InsufficientLots_NoSideEffects(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 50
	p.Instruments = []portfolio.Instrument{
		{
			ID: "ins-AAPL", Symbol: "AAPL", Name: "AAPL", Type: portfolio.InstrumentHolding,
			Lots: []portfolio.Lot{{ID: "lot-1", Quantity: 3, Price: 100}},
		},
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"symbol": "AAPL", "quantity": 10.0, "sell_price": 150.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_sell", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Fatalf("expected error for insufficient lots, got: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Cash != 50 {
		t.Errorf("Cash = %v, want unchanged 50", reloaded.Cash)
	}
	if len(reloaded.Transactions) != 0 {
		t.Errorf("want 0 transactions after a failed sell, got %d", len(reloaded.Transactions))
	}
	if len(reloaded.Instruments[0].Lots) != 1 || reloaded.Instruments[0].Lots[0].Quantity != 3 {
		t.Errorf("lots mutated after a failed sell: %+v", reloaded.Instruments[0].Lots)
	}
}

// ---- portfolio_set_cash: adjustment transaction, not deposit/withdrawal -----

func TestDispatch_PortfolioSetCash_RecordsAdjustmentNotDepositWithdrawal(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 200
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"cash": 350.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_set_cash", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Cash != 350 {
		t.Errorf("Cash = %v, want 350", reloaded.Cash)
	}
	if len(reloaded.Transactions) != 1 {
		t.Fatalf("want 1 transaction, got %d", len(reloaded.Transactions))
	}
	tx := reloaded.Transactions[0]
	if tx.Type != portfolio.TransactionAdjustment || tx.CashDelta != 150 {
		t.Errorf("transaction = %+v, want Type=adjustment CashDelta=150", tx)
	}
}

func TestDispatch_PortfolioSetCash_NegativeDeltaStillAdjustment(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 500
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"cash": 300.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_set_cash", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	tx := reloaded.Transactions[0]
	if tx.Type != portfolio.TransactionAdjustment || tx.CashDelta != -200 {
		t.Errorf("transaction = %+v, want Type=adjustment CashDelta=-200", tx)
	}
}

// ---- portfolio_deposit_cash / portfolio_withdraw_cash ------------------------

func TestDispatch_PortfolioDepositCash_OK(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 100
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"amount": 250.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_deposit_cash", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Cash != 350 {
		t.Errorf("Cash = %v, want 350", reloaded.Cash)
	}
	if len(reloaded.Transactions) != 1 || reloaded.Transactions[0].Type != portfolio.TransactionDeposit {
		t.Errorf("Transactions = %+v, want 1 deposit transaction", reloaded.Transactions)
	}
}

func TestDispatch_PortfolioDepositCash_RejectsNonPositiveAmount(t *testing.T) {
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
	args := toolCallArgs(t, map[string]any{"amount": 0.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_deposit_cash", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Fatalf("expected error for non-positive amount, got: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Transactions) != 0 {
		t.Errorf("want 0 transactions after a rejected deposit, got %d", len(reloaded.Transactions))
	}
}

func TestDispatch_PortfolioWithdrawCash_AllowsNegativeCash(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 100
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"amount": 500.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_withdraw_cash", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Cash != -400 {
		t.Errorf("Cash = %v, want -400 (withdrawals are never blocked)", reloaded.Cash)
	}
	if len(reloaded.Transactions) != 1 || reloaded.Transactions[0].Type != portfolio.TransactionWithdrawal {
		t.Errorf("Transactions = %+v, want 1 withdrawal transaction", reloaded.Transactions)
	}
}

// ---- portfolio_record_dividend ------------------------------------------------

func TestDispatch_PortfolioRecordDividend_OK(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 0
	p.Instruments = []portfolio.Instrument{
		{ID: "ins-AAPL", Symbol: "AAPL", Name: "AAPL", Type: portfolio.InstrumentHolding},
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"symbol": "AAPL", "amount": 42.5})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_record_dividend", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Cash != 42.5 {
		t.Errorf("Cash = %v, want 42.5", reloaded.Cash)
	}
	if len(reloaded.Transactions) != 1 {
		t.Fatalf("want 1 transaction, got %d", len(reloaded.Transactions))
	}
	tx := reloaded.Transactions[0]
	if tx.Type != portfolio.TransactionDividend || tx.Symbol != "AAPL" || tx.CashDelta != 42.5 {
		t.Errorf("transaction = %+v, want Type=dividend Symbol=AAPL CashDelta=42.5", tx)
	}
}

func TestDispatch_PortfolioRecordDividend_RejectsUnknownSymbol(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 0
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"symbol": "NOPE", "amount": 10.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_record_dividend", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Fatalf("expected error for unknown symbol, got: %s", result)
	}

	reloaded, err := portfolio.LoadPortfolio(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Cash != 0 {
		t.Errorf("Cash = %v, want unchanged 0", reloaded.Cash)
	}
	if len(reloaded.Transactions) != 0 {
		t.Errorf("want 0 transactions after a rejected dividend, got %d", len(reloaded.Transactions))
	}
}

func TestDispatch_PortfolioRecordDividend_RejectsWatchlistSymbol(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Instruments = []portfolio.Instrument{
		{ID: "ins-TSLA", Symbol: "TSLA", Name: "Tesla", Type: portfolio.InstrumentWatchlist},
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"symbol": "TSLA", "amount": 10.0})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_record_dividend", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Fatalf("expected error for watchlist-only symbol, got: %s", result)
	}
}
