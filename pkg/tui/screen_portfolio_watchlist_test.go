package tui

import (
	"strings"
	"testing"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/market"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

func TestNewPortfolioWatchlistModel_FiltersToWatchlistOnly(t *testing.T) {
	p := &portfolio.Portfolio{
		Instruments: []portfolio.Instrument{
			{ID: "1", Symbol: "AAPL", Type: portfolio.InstrumentHolding},
			{ID: "2", Symbol: "TSLA", Type: portfolio.InstrumentWatchlist},
			{ID: "3", Symbol: "MSFT", Type: portfolio.InstrumentWatchlist},
		},
	}
	m := newPortfolioWatchlistModel(p, nil, NewStyles(ThemeDark))
	if len(m.rows) != 2 {
		t.Fatalf("want 2 watchlist rows, got %d", len(m.rows))
	}
	for _, ins := range m.rows {
		if ins.Type != portfolio.InstrumentWatchlist {
			t.Errorf("row %s has type %s, want watchlist", ins.Symbol, ins.Type)
		}
	}
}

func TestNewPortfolioWatchlistModel_DedupesSymbols(t *testing.T) {
	p := &portfolio.Portfolio{
		Instruments: []portfolio.Instrument{
			{ID: "1", Symbol: "TSLA", Type: portfolio.InstrumentWatchlist},
			{ID: "2", Symbol: "TSLA", Type: portfolio.InstrumentWatchlist},
		},
	}
	m := newPortfolioWatchlistModel(p, nil, NewStyles(ThemeDark))
	if len(m.rows) != 1 {
		t.Fatalf("want 1 deduped row, got %d", len(m.rows))
	}
}

func TestNewPortfolioWatchlistModel_Empty(t *testing.T) {
	p := &portfolio.Portfolio{}
	m := newPortfolioWatchlistModel(p, nil, NewStyles(ThemeDark))
	if len(m.rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(m.rows))
	}
	if m.loadingPrices {
		t.Error("empty watchlist should not trigger a price fetch")
	}
}

func TestNewPortfolioWatchlistModel_NilProviderDoesNotLoad(t *testing.T) {
	p := &portfolio.Portfolio{
		Instruments: []portfolio.Instrument{{Symbol: "TSLA", Type: portfolio.InstrumentWatchlist}},
	}
	m := newPortfolioWatchlistModel(p, nil, NewStyles(ThemeDark))
	if m.loadingPrices {
		t.Error("nil market provider should not trigger loadingPrices")
	}
}

func TestFormatWatchlistRow_WithQuote(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatal(err)
	}
	ins := portfolio.Instrument{Symbol: "TSLA", Name: "Tesla Inc"}
	q := market.Quote{Last: 245.67, ChangePercent: 1.23}
	line, change, nonNeg := formatWatchlistRow(ins, q, true)
	if !strings.Contains(line, "TSLA") || !strings.Contains(line, "245.67") {
		t.Errorf("line = %q, want symbol and price", line)
	}
	if change != "+1.23%" {
		t.Errorf("change = %q, want +1.23%%", change)
	}
	if !nonNeg {
		t.Error("positive change should be nonNeg=true")
	}
}

func TestFormatWatchlistRow_NegativeChange(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatal(err)
	}
	ins := portfolio.Instrument{Symbol: "TSLA", Name: "Tesla Inc"}
	q := market.Quote{Last: 240.0, ChangePercent: -0.87}
	_, change, nonNeg := formatWatchlistRow(ins, q, true)
	if change != "-0.87%" {
		t.Errorf("change = %q, want -0.87%%", change)
	}
	if nonNeg {
		t.Error("negative change should be nonNeg=false")
	}
}

func TestFormatWatchlistRow_ZeroChangeIsNonNegative(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatal(err)
	}
	ins := portfolio.Instrument{Symbol: "TSLA"}
	q := market.Quote{Last: 240.0, ChangePercent: 0}
	_, change, nonNeg := formatWatchlistRow(ins, q, true)
	if change != "+0.00%" {
		t.Errorf("change = %q, want +0.00%%", change)
	}
	if !nonNeg {
		t.Error("zero change should render as Bull (nonNeg=true)")
	}
}

func TestFormatWatchlistRow_NoQuoteShowsPlaceholder(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatal(err)
	}
	ins := portfolio.Instrument{Symbol: "TSLA", Name: "Tesla Inc"}
	line, change, _ := formatWatchlistRow(ins, market.Quote{}, false)
	if !strings.Contains(line, "—") {
		t.Errorf("line = %q, want a placeholder dash for missing price", line)
	}
	if change != "—" {
		t.Errorf("change = %q, want placeholder dash", change)
	}
}
