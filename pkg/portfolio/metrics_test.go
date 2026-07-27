package portfolio

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// makePortfolio is a small builder used by every test in this file. Keeps
// test setup below the function bodies for readability.
func makePortfolio(symbols ...string) *Portfolio {
	now := time.Now()
	p := &Portfolio{
		ID:        "test-001",
		Name:      "test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	for _, sym := range symbols {
		p.Instruments = append(p.Instruments, Instrument{
			ID:     "ins-" + sym,
			Symbol: sym,
			Name:   sym,
			Type:   InstrumentHolding,
			Lots:   []Lot{{ID: "lot-" + sym, Quantity: 10, Price: 100, Date: now}},
		})
	}
	return p
}

func TestComputeMetrics_HappyPath(t *testing.T) {
	p := makePortfolio("AAPL", "MSFT")
	quotes := map[string]Quote{
		"AAPL": {Last: 150},
		"MSFT": {Last: 200},
	}
	m, err := ComputeMetrics(p, quotes)
	if err != nil {
		t.Fatal(err)
	}
	if m.HoldingCount != 2 || m.LotCount != 2 {
		t.Errorf("counts: want holdings=2 lots=2, got %d/%d", m.HoldingCount, m.LotCount)
	}
	// Cost basis: 2 holdings × 10 shares × $100 = 2000.
	if m.CostBasis != 2000 {
		t.Errorf("CostBasis: want 2000, got %v", m.CostBasis)
	}
	// Current value: 10*150 + 10*200 = 1500 + 2000 = 3500.
	if m.CurrentValue != 3500 {
		t.Errorf("CurrentValue: want 3500, got %v", m.CurrentValue)
	}
	// Unrealised P&L: 3500 - 2000 = 1500.
	if m.UnrealisedPnL != 1500 {
		t.Errorf("UnrealisedPnL: want 1500, got %v", m.UnrealisedPnL)
	}
	if len(m.WeightBySymbol) != 2 {
		t.Fatalf("WeightBySymbol length: want 2, got %d", len(m.WeightBySymbol))
	}
	// Sorted by value desc: MSFT (2000) first, AAPL (1500) second.
	if m.WeightBySymbol[0].Symbol != "MSFT" || m.WeightBySymbol[1].Symbol != "AAPL" {
		t.Errorf("WeightBySymbol must be sorted by value desc, got %s then %s",
			m.WeightBySymbol[0].Symbol, m.WeightBySymbol[1].Symbol)
	}
	// HHI = (2000/3500)^2 + (1500/3500)^2 ≈ 0.3265 + 0.1837 ≈ 0.5102.
	if !approxEqualP(m.HHI, 0.5102, 1e-3) {
		t.Errorf("HHI: want ~0.5102, got %v", m.HHI)
	}
	if m.HHIInterpretation != "concentrated" {
		t.Errorf("HHIInterpretation: want concentrated, got %q", m.HHIInterpretation)
	}
}

func TestComputeMetrics_RealisedOnly(t *testing.T) {
	// Realised P&L flows from the portfolio, not from quotes.
	p := makePortfolio("AAPL")
	p.RealizedPnL = 250
	m, err := ComputeMetrics(p, map[string]Quote{"AAPL": {Last: 150}})
	if err != nil {
		t.Fatal(err)
	}
	if m.RealisedPnL != 250 {
		t.Errorf("RealisedPnL: want 250, got %v", m.RealisedPnL)
	}
	// Total = realised + unrealised = 250 + 500 = 750.
	if m.TotalPnLAbsolute != 750 {
		t.Errorf("TotalPnLAbsolute: want 750, got %v", m.TotalPnLAbsolute)
	}
}

func TestComputeMetrics_MissingQuote(t *testing.T) {
	p := makePortfolio("AAPL", "MSFT")
	// Only AAPL has a quote; MSFT is skipped.
	m, err := ComputeMetrics(p, map[string]Quote{"AAPL": {Last: 150}})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.WeightBySymbol) != 1 {
		t.Errorf("WeightBySymbol: want 1 entry (AAPL only), got %d", len(m.WeightBySymbol))
	}
	if len(m.MissingQuotes) != 1 || m.MissingQuotes[0] != "MSFT" {
		t.Errorf("MissingQuotes: want [MSFT], got %v", m.MissingQuotes)
	}
	// Cost basis must still include MSFT even though we have no quote.
	if m.CostBasis != 2000 {
		t.Errorf("CostBasis must include unquoted holdings: want 2000, got %v", m.CostBasis)
	}
	// Current value is only AAPL: 10*150 = 1500.
	if m.CurrentValue != 1500 {
		t.Errorf("CurrentValue: want 1500, got %v", m.CurrentValue)
	}
}

func TestComputeMetrics_ZeroLastPrice(t *testing.T) {
	// A quote with Last=0 is treated the same as missing.
	p := makePortfolio("AAPL")
	m, err := ComputeMetrics(p, map[string]Quote{"AAPL": {Last: 0}})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.WeightBySymbol) != 0 {
		t.Errorf("zero Last should be treated as missing quote, got %d entries", len(m.WeightBySymbol))
	}
	if len(m.MissingQuotes) != 1 {
		t.Errorf("zero Last should appear in MissingQuotes, got %v", m.MissingQuotes)
	}
}

func TestComputeMetrics_WatchlistIgnored(t *testing.T) {
	p := makePortfolio("AAPL")
	p.Instruments = append(p.Instruments, Instrument{
		ID:     "ins-wl",
		Symbol: "WATCH",
		Name:   "Watchlist item",
		Type:   InstrumentWatchlist,
	})
	m, err := ComputeMetrics(p, map[string]Quote{
		"AAPL":  {Last: 150},
		"WATCH": {Last: 999},
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.HoldingCount != 1 {
		t.Errorf("watchlist must not count as holding, got %d", m.HoldingCount)
	}
	if len(m.WeightBySymbol) != 1 {
		t.Errorf("watchlist quote must be ignored, got %d weight entries", len(m.WeightBySymbol))
	}
}

func TestComputeMetrics_DividendYieldWeighted(t *testing.T) {
	// AAPL and MSFT equal weight (both 1000), AAPL 2.5% yield, MSFT 0%.
	// Weighted yield = (0.025*1000 + 0*1000) / 2000 = 0.0125 = 1.25%.
	p := makePortfolio("AAPL", "MSFT")
	quotes := map[string]Quote{
		"AAPL": {Last: 100, DividendYieldTTM: 0.025},
		"MSFT": {Last: 100},
	}
	m, err := ComputeMetrics(p, quotes)
	if err != nil {
		t.Fatal(err)
	}
	// Stored as percent.
	if !approxEqualP(m.DividendYield, 1.25, 1e-6) {
		t.Errorf("weighted dividend yield: want 1.25%%, got %v", m.DividendYield)
	}
	// The summary must quote the same percent value as the field (BUG-7:
	// it used to render the raw fraction, "div_yield=0.0125%").
	if !strings.Contains(m.Summary, "div_yield=1.2500%") {
		t.Errorf("summary should contain div_yield=1.2500%%, got %q", m.Summary)
	}
}

func TestComputeMetrics_WeightedBeta(t *testing.T) {
	// Two equal-weighted holdings, betas 1.2 and 0.8 → weighted beta = 1.0.
	p := makePortfolio("AAPL", "MSFT")
	quotes := map[string]Quote{
		"AAPL": {Last: 100, Beta: 1.2},
		"MSFT": {Last: 100, Beta: 0.8},
	}
	m, err := ComputeMetrics(p, quotes)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqualP(m.WeightedBeta, 1.0, 1e-9) {
		t.Errorf("weighted beta: want 1.0, got %v", m.WeightedBeta)
	}
}

func TestComputeMetrics_HHI_ConcentrationThresholds(t *testing.T) {
	// Single holding: HHI = 1.0 → "concentrated".
	p1 := makePortfolio("ONLY")
	m, err := ComputeMetrics(p1, map[string]Quote{"ONLY": {Last: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if m.HHI != 1 {
		t.Errorf("single-holding HHI: want 1.0, got %v", m.HHI)
	}
	if m.HHIInterpretation != "concentrated" {
		t.Errorf("single holding should be concentrated, got %q", m.HHIInterpretation)
	}
}

func TestComputeMetrics_HHI_Diversified(t *testing.T) {
	// Five equal-weighted holdings → HHI = 5*(1/5)^2 = 0.2 → "diversified".
	p := &Portfolio{ID: "p", Name: "p"}
	for _, s := range []string{"A", "B", "C", "D", "E"} {
		p.Instruments = append(p.Instruments, Instrument{
			ID: s, Symbol: s, Name: s, Type: InstrumentHolding,
			Lots: []Lot{{ID: s, Quantity: 1, Price: 100, Date: time.Now()}},
		})
	}
	quotes := map[string]Quote{"A": {Last: 100}, "B": {Last: 100}, "C": {Last: 100}, "D": {Last: 100}, "E": {Last: 100}}
	m, err := ComputeMetrics(p, quotes)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqualP(m.HHI, 0.2, 1e-9) {
		t.Errorf("5-equal HHI: want 0.2, got %v", m.HHI)
	}
	if m.HHIInterpretation != "diversified" {
		t.Errorf("HHI 0.2 should be diversified, got %q", m.HHIInterpretation)
	}
}

func TestComputeMetrics_NoHoldings(t *testing.T) {
	p := &Portfolio{ID: "empty", Name: "empty"}
	_, err := ComputeMetrics(p, nil)
	if err != ErrNoHoldings {
		t.Errorf("expected ErrNoHoldings, got %v", err)
	}
}

func TestComputeMetrics_NilPortfolio(t *testing.T) {
	if _, err := ComputeMetrics(nil, nil); err == nil {
		t.Error("expected error for nil portfolio")
	}
}

func TestComputeMetrics_SummaryContainsKey(t *testing.T) {
	p := makePortfolio("AAPL")
	m, err := ComputeMetrics(p, map[string]Quote{"AAPL": {Last: 150}})
	if err != nil {
		t.Fatal(err)
	}
	if m.Summary == "" {
		t.Error("summary should not be empty")
	}
}

// approxEqualP is a local helper for tolerance-based float comparison.
func approxEqualP(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestSuggestRebalance_NoTargetAllocation(t *testing.T) {
	p := makePortfolio("AAPL")
	_, err := SuggestRebalance(p, map[string]Quote{"AAPL": {Last: 150}}, 0)
	if !errors.Is(err, ErrNoTargetAllocation) {
		t.Errorf("expected ErrNoTargetAllocation, got %v", err)
	}
}

func TestSuggestRebalance_NegativeMaxDrift(t *testing.T) {
	p := makePortfolio("AAPL")
	p.TargetAllocation = map[string]float64{"AAPL": 1.0}
	_, err := SuggestRebalance(p, map[string]Quote{"AAPL": {Last: 150}}, -1)
	if err == nil {
		t.Error("expected error for negative max_drift_percent")
	}
}

func TestSuggestRebalance_NoBasis(t *testing.T) {
	// No quotes and no cash: nothing to compute a rebalance basis from.
	p := makePortfolio("AAPL")
	p.TargetAllocation = map[string]float64{"AAPL": 1.0}
	_, err := SuggestRebalance(p, nil, 0)
	if !errors.Is(err, ErrNoRebalanceBasis) {
		t.Errorf("expected ErrNoRebalanceBasis, got %v", err)
	}
}

func TestSuggestRebalance_Overweight_SuggestsSell(t *testing.T) {
	// AAPL: 10 shares @ 150 = 1500 current value, no cash → basis 1500.
	// Target 50% → target value 750. diff = 750 - 1500 = -750 → sell 750/150 = 5 shares.
	p := makePortfolio("AAPL")
	p.TargetAllocation = map[string]float64{"AAPL": 0.5}
	r, err := SuggestRebalance(p, map[string]Quote{"AAPL": {Last: 150}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.TotalValue != 1500 {
		t.Errorf("TotalValue: want 1500, got %v", r.TotalValue)
	}
	if len(r.Operations) != 1 {
		t.Fatalf("Operations: want 1, got %d", len(r.Operations))
	}
	op := r.Operations[0]
	if op.Symbol != "AAPL" || op.Action != "sell" {
		t.Errorf("op: want sell AAPL, got %+v", op)
	}
	if !approxEqualP(op.Quantity, 5, 1e-9) {
		t.Errorf("Quantity: want 5, got %v", op.Quantity)
	}
}

func TestSuggestRebalance_Underweight_SuggestsBuy(t *testing.T) {
	// AAPL: 10 shares @ 150 = 1500, cash 1500, basis = 3000. Target 100% AAPL
	// → target value 3000, diff = 1500 → buy 1500/150 = 10 shares.
	p := makePortfolio("AAPL")
	p.Cash = 1500
	p.TargetAllocation = map[string]float64{"AAPL": 1.0}
	r, err := SuggestRebalance(p, map[string]Quote{"AAPL": {Last: 150}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Operations) != 1 {
		t.Fatalf("Operations: want 1, got %d", len(r.Operations))
	}
	op := r.Operations[0]
	if op.Action != "buy" {
		t.Errorf("Action: want buy, got %s", op.Action)
	}
	if !approxEqualP(op.Quantity, 10, 1e-9) {
		t.Errorf("Quantity: want 10, got %v", op.Quantity)
	}
}

func TestSuggestRebalance_NewTargetSymbol_NotCurrentlyHeld(t *testing.T) {
	// Portfolio holds only AAPL but targets a 50/50 split with MSFT, a symbol
	// not currently held at all.
	p := makePortfolio("AAPL")
	p.TargetAllocation = map[string]float64{"AAPL": 0.5, "MSFT": 0.5}
	quotes := map[string]Quote{"AAPL": {Last: 150}, "MSFT": {Last: 200}}
	r, err := SuggestRebalance(p, quotes, 0)
	if err != nil {
		t.Fatal(err)
	}
	var msft *RebalanceOp
	for i := range r.Operations {
		if r.Operations[i].Symbol == "MSFT" {
			msft = &r.Operations[i]
		}
	}
	if msft == nil {
		t.Fatal("expected a suggested operation for MSFT")
	}
	if msft.Action != "buy" {
		t.Errorf("MSFT action: want buy, got %s", msft.Action)
	}
	// basis = 1500 (AAPL only, no cash), target value for MSFT = 750, price 200 → 3.75 shares.
	if !approxEqualP(msft.Quantity, 3.75, 1e-9) {
		t.Errorf("MSFT quantity: want 3.75, got %v", msft.Quantity)
	}
}

func TestSuggestRebalance_HeldSymbolAbsentFromTarget_SellsToZero(t *testing.T) {
	// AAPL is held but not present in target_allocation at all → full sell.
	p := makePortfolio("AAPL")
	p.TargetAllocation = map[string]float64{"MSFT": 1.0}
	quotes := map[string]Quote{"AAPL": {Last: 150}, "MSFT": {Last: 200}}
	r, err := SuggestRebalance(p, quotes, 0)
	if err != nil {
		t.Fatal(err)
	}
	var aapl *RebalanceOp
	for i := range r.Operations {
		if r.Operations[i].Symbol == "AAPL" {
			aapl = &r.Operations[i]
		}
	}
	if aapl == nil {
		t.Fatal("expected a suggested operation for AAPL")
	}
	if aapl.Action != "sell" {
		t.Errorf("AAPL action: want sell, got %s", aapl.Action)
	}
	if !approxEqualP(aapl.Quantity, 10, 1e-9) {
		t.Errorf("AAPL quantity: want full 10-share position, got %v", aapl.Quantity)
	}
}

func TestSuggestRebalance_WithinTolerance_NoOperations(t *testing.T) {
	// AAPL is exactly at target weight (100%, no cash) → zero drift, no ops
	// regardless of max_drift_percent.
	p := makePortfolio("AAPL")
	p.TargetAllocation = map[string]float64{"AAPL": 1.0}
	r, err := SuggestRebalance(p, map[string]Quote{"AAPL": {Last: 150}}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Operations) != 0 {
		t.Errorf("Operations: want 0 (within tolerance), got %d: %+v", len(r.Operations), r.Operations)
	}
}

func TestSuggestRebalance_MaxDriftPercent_SuppressesSmallDrift(t *testing.T) {
	// AAPL 1500, MSFT 1500, basis 3000. Target AAPL 55%, MSFT 45% → drift is
	// 5 percentage points on each side, which a 10% tolerance should suppress.
	p := makePortfolio("AAPL", "MSFT")
	p.TargetAllocation = map[string]float64{"AAPL": 0.55, "MSFT": 0.45}
	quotes := map[string]Quote{"AAPL": {Last: 150}, "MSFT": {Last: 150}}
	r, err := SuggestRebalance(p, quotes, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Operations) != 0 {
		t.Errorf("Operations: want 0 (within 10%% tolerance), got %d: %+v", len(r.Operations), r.Operations)
	}
}

func TestSuggestRebalance_MissingQuote_Excluded(t *testing.T) {
	// MSFT has no quote at all; it should be skipped and reported as missing,
	// while AAPL is still rebalanced normally.
	p := makePortfolio("AAPL", "MSFT")
	p.TargetAllocation = map[string]float64{"AAPL": 0.5, "MSFT": 0.5}
	r, err := SuggestRebalance(p, map[string]Quote{"AAPL": {Last: 150}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.MissingQuotes) != 1 || r.MissingQuotes[0] != "MSFT" {
		t.Errorf("MissingQuotes: want [MSFT], got %v", r.MissingQuotes)
	}
	for _, op := range r.Operations {
		if op.Symbol == "MSFT" {
			t.Errorf("MSFT should not have a suggested operation without a quote, got %+v", op)
		}
	}
}

func TestSuggestRebalance_SellsSortedBeforeBuys(t *testing.T) {
	p := makePortfolio("AAPL", "MSFT")
	// AAPL overweight (target 0 → sell), MSFT underweight relative to a big
	// cash-funded target (target 100% of basis incl. cash → buy).
	p.Cash = 1000
	p.TargetAllocation = map[string]float64{"MSFT": 1.0}
	quotes := map[string]Quote{"AAPL": {Last: 150}, "MSFT": {Last: 150}}
	r, err := SuggestRebalance(p, quotes, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Operations) != 2 {
		t.Fatalf("Operations: want 2, got %d", len(r.Operations))
	}
	if r.Operations[0].Action != "sell" || r.Operations[1].Action != "buy" {
		t.Errorf("expected sell before buy, got %s then %s", r.Operations[0].Action, r.Operations[1].Action)
	}
}

// --- HoldingsAsOf / CashAsOf -------------------------------------------------

func TestHoldingsAsOf_ReplaysBuysAndSells(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{Type: TransactionBuy, Symbol: "AAPL", Quantity: 10, Date: base},
		{Type: TransactionBuy, Symbol: "AAPL", Quantity: 5, Date: base.AddDate(0, 0, 10)},
		{Type: TransactionSell, Symbol: "AAPL", Quantity: 3, Date: base.AddDate(0, 0, 20)},
	}}
	// Before any transaction: nothing held.
	if qty := HoldingsAsOf(p, base.AddDate(0, 0, -1)); qty["AAPL"] != 0 {
		t.Errorf("before first buy: want 0, got %v", qty["AAPL"])
	}
	// Between the two buys: only the first lot counted.
	if qty := HoldingsAsOf(p, base.AddDate(0, 0, 5)); qty["AAPL"] != 10 {
		t.Errorf("between buys: want 10, got %v", qty["AAPL"])
	}
	// After all transactions: 10 + 5 - 3 = 12.
	if qty := HoldingsAsOf(p, base.AddDate(0, 0, 30)); qty["AAPL"] != 12 {
		t.Errorf("after all txs: want 12, got %v", qty["AAPL"])
	}
}

// TestHoldingsAsOf_ReplaysAdjustmentTransactions is a regression test for
// GitHub issue #28: a lot catalogued via portfolio_add_lot/
// portfolio_add_instrument (debit_cash=false) records a zero-cash-delta
// TransactionAdjustment with Symbol/Quantity set, and HoldingsAsOf must count
// it like a buy — otherwise it stays invisible to start-of-period
// reconstruction and portfolio_compare_benchmark inflates returns. A
// portfolio_set_cash-style adjustment (empty Symbol) must remain a no-op.
func TestHoldingsAsOf_ReplaysAdjustmentTransactions(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{Type: TransactionAdjustment, Symbol: "AAPL", Quantity: 10, Price: 90, CashDelta: 0, Date: base},
		{Type: TransactionAdjustment, Symbol: "", CashDelta: 500, Date: base.AddDate(0, 0, 5)},
	}}
	// Before the catalogued lot: nothing held.
	if qty := HoldingsAsOf(p, base.AddDate(0, 0, -1)); qty["AAPL"] != 0 {
		t.Errorf("before catalogued lot: want 0, got %v", qty["AAPL"])
	}
	// After the catalogued lot and the cash-only adjustment: 10, no stray
	// entry for the empty-Symbol adjustment.
	qty := HoldingsAsOf(p, base.AddDate(0, 0, 10))
	if qty["AAPL"] != 10 {
		t.Errorf("after catalogued lot: want 10, got %v", qty["AAPL"])
	}
	if v, ok := qty[""]; ok {
		t.Errorf("cash-only adjustment must not create a holdings entry, got qty[\"\"]=%v", v)
	}
}

func TestHoldingsAsOf_NoTransactions_EmptyMap(t *testing.T) {
	p := &Portfolio{ID: "test"}
	qty := HoldingsAsOf(p, time.Now())
	if len(qty) != 0 {
		t.Errorf("want empty map, got %v", qty)
	}
}

func TestCashAsOf_UndoesLaterTransactions(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &Portfolio{ID: "test", Cash: 200, Transactions: []Transaction{
		{Type: TransactionDeposit, CashDelta: 1000, Date: base},
		{Type: TransactionBuy, Symbol: "AAPL", CashDelta: -500, Date: base.AddDate(0, 0, 10)},
		{Type: TransactionWithdrawal, CashDelta: -300, Date: base.AddDate(0, 0, 20)},
	}}
	// Cash right after the deposit, before the buy and withdrawal: 1000.
	if c := CashAsOf(p, base.AddDate(0, 0, 5)); c != 1000 {
		t.Errorf("after deposit only: want 1000, got %v", c)
	}
	// Cash after the buy, before the withdrawal: 1000 - 500 = 500.
	if c := CashAsOf(p, base.AddDate(0, 0, 15)); c != 500 {
		t.Errorf("after buy: want 500, got %v", c)
	}
	// Cash now (after everything): matches p.Cash.
	if c := CashAsOf(p, base.AddDate(0, 0, 30)); c != 200 {
		t.Errorf("after all txs: want 200, got %v", c)
	}
}

// --- ComputePeriodReturn ------------------------------------------------------

func TestComputePeriodReturn_NoExternalFlows_SimpleReturn(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(1, 0, 0)
	p := &Portfolio{ID: "test"}
	r, err := ComputePeriodReturn(p, from, to, 1000, 1100)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqualP(r.ReturnPercent, 10.0, 0.001) {
		t.Errorf("ReturnPercent: want 10, got %v", r.ReturnPercent)
	}
	if r.NetExternalFlow != 0 {
		t.Errorf("NetExternalFlow: want 0, got %v", r.NetExternalFlow)
	}
}

func TestComputePeriodReturn_DepositAtStart_ExcludedFromGain(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 100)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		// Deposited right at the start: fully time-weighted into the denominator.
		{Type: TransactionDeposit, CashDelta: 500, Date: from},
	}}
	// Start 1000, deposit 500 immediately, end 1500 with zero investment gain.
	r, err := ComputePeriodReturn(p, from, to, 1000, 1500)
	if err != nil {
		t.Fatal(err)
	}
	if !approxEqualP(r.ReturnPercent, 0.0, 0.01) {
		t.Errorf("ReturnPercent: want ~0 (deposit fully explains the gain), got %v", r.ReturnPercent)
	}
	if r.NetExternalFlow != 500 {
		t.Errorf("NetExternalFlow: want 500, got %v", r.NetExternalFlow)
	}
}

func TestComputePeriodReturn_DepositAtEnd_WeightedNearZero(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 100)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		// Deposited on the very last day: barely time-weighted, so it should
		// only mildly dilute the denominator (weight ≈ 0), unlike the
		// day-one deposit case above.
		{Type: TransactionDeposit, CashDelta: 500, Date: to},
	}}
	r, err := ComputePeriodReturn(p, from, to, 1000, 1500)
	if err != nil {
		t.Fatal(err)
	}
	// (1500 - 1000 - 500) / (1000 + ~0) ≈ 0%, since the deposit itself
	// accounts for the entire gain and it's barely weighted into the
	// denominator either way — but unlike the day-one case, a genuine
	// investment gain here would NOT be diluted by a near-zero-weighted flow.
	if !approxEqualP(r.ReturnPercent, 0.0, 0.5) {
		t.Errorf("ReturnPercent: want ~0, got %v", r.ReturnPercent)
	}
}

func TestComputePeriodReturn_DividendsAndTradesExcludedFromFlows(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 100)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		{Type: TransactionDividend, CashDelta: 50, Date: from.AddDate(0, 0, 10)},
		{Type: TransactionBuy, Symbol: "AAPL", CashDelta: -200, Date: from.AddDate(0, 0, 20)},
		{Type: TransactionSell, Symbol: "AAPL", CashDelta: 250, Date: from.AddDate(0, 0, 30)},
	}}
	r, err := ComputePeriodReturn(p, from, to, 1000, 1100)
	if err != nil {
		t.Fatal(err)
	}
	if r.NetExternalFlow != 0 {
		t.Errorf("NetExternalFlow: dividends/buys/sells must not count as external flows, got %v", r.NetExternalFlow)
	}
	if !approxEqualP(r.ReturnPercent, 10.0, 0.001) {
		t.Errorf("ReturnPercent: want 10 (unaffected by internal flows), got %v", r.ReturnPercent)
	}
}

func TestComputePeriodReturn_InvalidRange(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := &Portfolio{ID: "test"}
	if _, err := ComputePeriodReturn(p, from, from, 1000, 1100); !errors.Is(err, ErrPeriodInvalidRange) {
		t.Errorf("want ErrPeriodInvalidRange, got %v", err)
	}
	if _, err := ComputePeriodReturn(p, from, from.AddDate(0, 0, -1), 1000, 1100); !errors.Is(err, ErrPeriodInvalidRange) {
		t.Errorf("want ErrPeriodInvalidRange, got %v", err)
	}
}

func TestComputePeriodReturn_ZeroDenominator(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 100)
	p := &Portfolio{ID: "test", Transactions: []Transaction{
		// A large withdrawal right at the start makes the time-weighted
		// denominator zero/negative.
		{Type: TransactionWithdrawal, CashDelta: -1000, Date: from},
	}}
	if _, err := ComputePeriodReturn(p, from, to, 1000, 100); !errors.Is(err, ErrPeriodZeroDenominator) {
		t.Errorf("want ErrPeriodZeroDenominator, got %v", err)
	}
}

func TestComputePeriodReturn_NilPortfolio(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)
	if _, err := ComputePeriodReturn(nil, from, to, 1000, 1100); err == nil {
		t.Error("want error for nil portfolio")
	}
}
