package portfolio

import (
	"math"
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
