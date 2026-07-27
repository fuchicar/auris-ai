package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuchicar/auris-ai/pkg/llm"
	"github.com/fuchicar/auris-ai/pkg/market"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

// ---- alignedDailyReturns -----------------------------------------------------

func candle(date string, close float64) market.Candle {
	t, _ := time.Parse("2006-01-02", date)
	return market.Candle{Time: t, Close: close}
}

func TestAlignedDailyReturns_HappyPath(t *testing.T) {
	seriesBySymbol := map[string][]market.Candle{
		"AAPL": {candle("2026-01-01", 100), candle("2026-01-02", 110), candle("2026-01-03", 99)},
	}
	benchmark := []market.Candle{candle("2026-01-01", 400), candle("2026-01-02", 404), candle("2026-01-03", 396)}

	symReturns, benchReturns := alignedDailyReturns(seriesBySymbol, benchmark)
	if len(benchReturns) != 2 {
		t.Fatalf("benchReturns: want 2, got %d (%v)", len(benchReturns), benchReturns)
	}
	if !approxEqual(benchReturns[0], 0.01, 1e-9) {
		t.Errorf("benchReturns[0]: want 0.01, got %v", benchReturns[0])
	}
	if len(symReturns["AAPL"]) != 2 {
		t.Fatalf("symReturns[AAPL]: want 2, got %d", len(symReturns["AAPL"]))
	}
	if !approxEqual(symReturns["AAPL"][0], 0.10, 1e-9) {
		t.Errorf("symReturns[AAPL][0]: want 0.10, got %v", symReturns["AAPL"][0])
	}
}

func TestAlignedDailyReturns_MismatchedCalendars_UsesIntersection(t *testing.T) {
	// AAPL is missing 2026-01-02 (e.g. a trading halt); that date must be
	// dropped from the common set entirely, not just from AAPL's series.
	seriesBySymbol := map[string][]market.Candle{
		"AAPL": {candle("2026-01-01", 100), candle("2026-01-03", 105)},
	}
	benchmark := []market.Candle{candle("2026-01-01", 400), candle("2026-01-02", 404), candle("2026-01-03", 408)}

	symReturns, benchReturns := alignedDailyReturns(seriesBySymbol, benchmark)
	if len(benchReturns) != 1 {
		t.Fatalf("benchReturns: want 1 (only 01-01→01-03 survives), got %d (%v)", len(benchReturns), benchReturns)
	}
	if len(symReturns["AAPL"]) != 1 {
		t.Fatalf("symReturns[AAPL]: want 1, got %d", len(symReturns["AAPL"]))
	}
}

func TestAlignedDailyReturns_TooFewCommonDays_ReturnsNil(t *testing.T) {
	seriesBySymbol := map[string][]market.Candle{
		"AAPL": {candle("2026-01-01", 100)},
	}
	benchmark := []market.Candle{candle("2026-01-01", 400)}

	symReturns, benchReturns := alignedDailyReturns(seriesBySymbol, benchmark)
	if symReturns != nil || benchReturns != nil {
		t.Errorf("want nil, nil for a single common day, got %v, %v", symReturns, benchReturns)
	}
}

// ---- buildBenchmarkComparison -------------------------------------------------

func TestBuildBenchmarkComparison_ComputesAlpha(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	res := buildBenchmarkComparison("p1", "SPY", from, to, 12.0, 8.0, "modified_dietz", nil, nil, nil)
	if !approxEqual(res.AlphaPercent, 4.0, 1e-9) {
		t.Errorf("AlphaPercent: want 4.0, got %v", res.AlphaPercent)
	}
	if res.Beta != nil {
		t.Errorf("Beta: want nil without a return series, got %v", *res.Beta)
	}
	if res.From != "2026-01-01" || res.To != "2026-02-01" {
		t.Errorf("From/To formatting: got %s / %s", res.From, res.To)
	}
}

func TestBuildBenchmarkComparison_BetaPopulatedFromSeries(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	portfolioReturns := []float64{0.02, -0.01, 0.03}
	benchmarkReturns := []float64{0.01, -0.005, 0.015}
	res := buildBenchmarkComparison("p1", "SPY", from, to, 5.0, 4.0, "modified_dietz", nil, portfolioReturns, benchmarkReturns)
	if res.Beta == nil {
		t.Fatal("Beta must be populated when a valid aligned series is supplied")
	}
	if res.Correlation == nil {
		t.Fatal("Correlation must be populated alongside Beta")
	}
}

func TestBuildBenchmarkComparison_MismatchedSeriesLength_NoBeta(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	res := buildBenchmarkComparison("p1", "SPY", from, to, 5.0, 4.0, "modified_dietz", nil,
		[]float64{0.01, 0.02}, []float64{0.01})
	if res.Beta != nil {
		t.Errorf("Beta must stay nil when series lengths differ, got %v", *res.Beta)
	}
}

func TestBuildBenchmarkComparison_BuyAndHoldApproximation_NotedInSummary(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	res := buildBenchmarkComparison("p1", "SPY", from, to, 5.0, 4.0, "buy_and_hold_approximation", nil, nil, nil)
	if res.ReturnMethod != "buy_and_hold_approximation" {
		t.Errorf("ReturnMethod: want buy_and_hold_approximation, got %s", res.ReturnMethod)
	}
	if !contains(res.Summary, "no transaction history") {
		t.Errorf("Summary must note the approximation, got: %s", res.Summary)
	}
}

func TestBuildBenchmarkComparison_MissingHistoricalNotedInSummary(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	res := buildBenchmarkComparison("p1", "SPY", from, to, 5.0, 4.0, "modified_dietz", []string{"MSFT"}, nil, nil)
	if !contains(res.Summary, "MSFT") {
		t.Errorf("Summary must mention symbols with no historical price, got: %s", res.Summary)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}

// ---- portfolio_compare_benchmark dispatch -------------------------------------

func TestDispatch_PortfolioCompareBenchmark_OK(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	buyDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	p := portfolio.NewPortfolio("test")
	p.Cash = 100
	p.Instruments = []portfolio.Instrument{
		{
			ID: "ins-AAPL", Symbol: "AAPL", Name: "AAPL", Type: portfolio.InstrumentHolding,
			Lots: []portfolio.Lot{{ID: "lot-AAPL", Quantity: 10, Price: 90, Date: buyDate}},
		},
	}
	p.Transactions = []portfolio.Transaction{
		{Type: portfolio.TransactionBuy, Symbol: "AAPL", Quantity: 10, Price: 90, CashDelta: -900, Date: buyDate},
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	aaplSeries := []market.Candle{
		candle("2026-01-01", 100), candle("2026-01-02", 102), candle("2026-01-03", 101), candle("2026-01-04", 103),
	}
	spySeries := []market.Candle{
		candle("2026-01-01", 400), candle("2026-01-02", 404), candle("2026-01-03", 408), candle("2026-01-04", 440),
	}
	mp := &mockMarket{
		candlesBySymbol: map[string][]market.Candle{
			"AAPL": aaplSeries,
			"SPY":  spySeries,
		},
		quotesBySymbol: map[string]market.Quote{
			"AAPL": {Last: 110},
		},
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"from": "2026-01-01T00:00:00Z", "to": "2026-01-04T00:00:00Z"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_compare_benchmark", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	var res benchmarkComparisonResult
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if res.ReturnMethod != "modified_dietz" {
		t.Errorf("ReturnMethod: want modified_dietz (transaction history present), got %s", res.ReturnMethod)
	}
	// Benchmark return: (440-400)/400 = 10%.
	if !approxEqual(res.BenchmarkReturnPercent, 10.0, 1e-6) {
		t.Errorf("BenchmarkReturnPercent: want 10, got %v", res.BenchmarkReturnPercent)
	}
	// Start value = cash(100) + 10*close_on_from(100) = 1100.
	// End value = cash(100) + 10*last_quote(110) = 1200.
	// No external flows -> return = (1200-1100)/1100*100 ≈ 9.0909.
	if !approxEqual(res.PortfolioReturnPercent, 9.0909, 1e-3) {
		t.Errorf("PortfolioReturnPercent: want ~9.0909, got %v", res.PortfolioReturnPercent)
	}
	if res.Beta == nil {
		t.Error("Beta should be populated: AAPL and SPY share the same 4 trading days")
	}
	if res.ComputedAt == "" {
		t.Error("ComputedAt must be populated by the dispatcher")
	}
	if res.BenchmarkSymbol != "SPY" {
		t.Errorf("BenchmarkSymbol: want default SPY, got %s", res.BenchmarkSymbol)
	}
}

func TestDispatch_PortfolioCompareBenchmark_NoTransactionHistory_BuyAndHold(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 100
	p.Instruments = []portfolio.Instrument{
		{
			ID: "ins-AAPL", Symbol: "AAPL", Name: "AAPL", Type: portfolio.InstrumentHolding,
			Lots: []portfolio.Lot{{ID: "lot-AAPL", Quantity: 10, Price: 90, Date: time.Now()}},
		},
	}
	// No Transactions recorded — simulates a portfolio created before FEAT-2.
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	mp := &mockMarket{
		candlesBySymbol: map[string][]market.Candle{
			"AAPL": {candle("2026-01-01", 100), candle("2026-01-02", 102)},
			"SPY":  {candle("2026-01-01", 400), candle("2026-01-02", 404)},
		},
		quotesBySymbol: map[string]market.Quote{"AAPL": {Last: 110}},
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"from": "2026-01-01T00:00:00Z", "to": "2026-01-02T00:00:00Z"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_compare_benchmark", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var res benchmarkComparisonResult
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if res.ReturnMethod != "buy_and_hold_approximation" {
		t.Errorf("ReturnMethod: want buy_and_hold_approximation, got %s", res.ReturnMethod)
	}
}

func TestDispatch_PortfolioCompareBenchmark_InvalidRange(t *testing.T) {
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
	args := toolCallArgs(t, map[string]any{"from": "2026-01-05T00:00:00Z", "to": "2026-01-01T00:00:00Z"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_compare_benchmark", Arguments: args},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("to before from should produce an error, got %s", result)
	}
}

func TestDispatch_PortfolioCompareBenchmark_BenchmarkFetchFails(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}
	// No candlesBySymbol configured: GetCandles falls back to ErrNotSupported.
	a := New(&mockLLM{}, &mockMarket{}, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_compare_benchmark", Arguments: "{}"},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("unfetchable benchmark data should produce an error, got %s", result)
	}
}

func TestDispatch_PortfolioCompareBenchmark_NoMarketProvider(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}
	a := New(&mockLLM{}, nil, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_compare_benchmark", Arguments: "{}"},
	}, &lk)
	if result[:6] != "error:" {
		t.Errorf("nil market provider should produce an error, got %s", result)
	}
}

func TestDispatch_PortfolioCompareBenchmark_CustomBenchmarkSymbol(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	p := portfolio.NewPortfolio("test")
	p.Cash = 1000
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}
	mp := &mockMarket{
		candlesBySymbol: map[string][]market.Candle{
			"QQQ": {candle("2026-01-01", 300), candle("2026-01-02", 330)},
		},
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{
		"benchmark_symbol": "QQQ",
		"from":             "2026-01-01T00:00:00Z",
		"to":               "2026-01-02T00:00:00Z",
	})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_compare_benchmark", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}
	var res benchmarkComparisonResult
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if res.BenchmarkSymbol != "QQQ" {
		t.Errorf("BenchmarkSymbol: want QQQ, got %s", res.BenchmarkSymbol)
	}
	if !approxEqual(res.BenchmarkReturnPercent, 10.0, 1e-6) {
		t.Errorf("BenchmarkReturnPercent: want 10, got %v", res.BenchmarkReturnPercent)
	}
}

// TestDispatch_PortfolioCompareBenchmark_CataloguedLotNotInflated is a
// regression test for GitHub issue #28: a holding catalogued without a cash
// debit (portfolio_add_instrument/portfolio_add_lot, debit_cash=false) is
// recorded as a zero-cash-delta TransactionAdjustment, not a plain Lot with
// no transaction at all. A single unrelated transaction (here, one deposit)
// is enough to route portfolio_compare_benchmark into the modified_dietz
// branch — before the fix, HoldingsAsOf ignored TransactionAdjustment
// entirely, so the catalogued AAPL position was invisible at the start of
// the period while fully counted at the end, inflating the return from a
// true ~5% to a spurious 110%.
func TestDispatch_PortfolioCompareBenchmark_CataloguedLotNotInflated(t *testing.T) {
	tmp := t.TempDir()
	prev := portfolio.SetPortfoliosDirForTest(tmp)
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })

	catalogueDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	p := portfolio.NewPortfolio("test")
	p.Cash = 1000
	p.Instruments = []portfolio.Instrument{
		{
			ID: "ins-AAPL", Symbol: "AAPL", Name: "AAPL", Type: portfolio.InstrumentHolding,
			Lots: []portfolio.Lot{{ID: "lot-AAPL", Quantity: 10, Price: 90, Date: catalogueDate}},
		},
	}
	p.Transactions = []portfolio.Transaction{
		// The "one unrelated transaction" from the issue: a deposit that has
		// nothing to do with the catalogued AAPL lot.
		{Type: portfolio.TransactionDeposit, CashDelta: 1000, Date: catalogueDate},
		// The catalogued lot itself: no cash movement.
		{Type: portfolio.TransactionAdjustment, Symbol: "AAPL", Quantity: 10, Price: 90, CashDelta: 0, Date: catalogueDate},
	}
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatal(err)
	}

	aaplSeries := []market.Candle{
		candle("2026-01-01", 100), candle("2026-01-02", 102), candle("2026-01-03", 101), candle("2026-01-04", 103),
	}
	spySeries := []market.Candle{
		candle("2026-01-01", 400), candle("2026-01-02", 404), candle("2026-01-03", 408), candle("2026-01-04", 440),
	}
	mp := &mockMarket{
		candlesBySymbol: map[string][]market.Candle{
			"AAPL": aaplSeries,
			"SPY":  spySeries,
		},
		quotesBySymbol: map[string]market.Quote{
			"AAPL": {Last: 110},
		},
	}
	a := New(&mockLLM{}, mp, "")
	a.currentPortfolioID = p.ID

	var lk ProgressKind
	args := toolCallArgs(t, map[string]any{"from": "2026-01-01T00:00:00Z", "to": "2026-01-04T00:00:00Z"})
	result := a.dispatch(context.Background(), llm.ToolCall{
		Function: llm.ToolCallFunction{Name: "portfolio_compare_benchmark", Arguments: args},
	}, &lk)
	if result[:6] == "error:" {
		t.Fatalf("unexpected error: %s", result)
	}

	var res benchmarkComparisonResult
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("invalid JSON: %s — %v", result, err)
	}
	if res.ReturnMethod != "modified_dietz" {
		t.Errorf("ReturnMethod: want modified_dietz (transaction history present), got %s", res.ReturnMethod)
	}
	// Start value = cash(1000, deposit predates the period) + 10*close_on_from(100) = 2000.
	// End value = cash(1000) + 10*last_quote(110) = 2100.
	// Return = (2100-2000)/2000*100 = 5. Before the fix this came out to 110%
	// (the catalogued lot's 1100 of value counted only at the end, against a
	// cash-only start value of 1000).
	if !approxEqual(res.PortfolioReturnPercent, 5.0, 1e-6) {
		t.Errorf("PortfolioReturnPercent: want 5 (catalogued lot counted at both ends), got %v", res.PortfolioReturnPercent)
	}
}
