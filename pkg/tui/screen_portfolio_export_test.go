package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

func TestNewPortfolioExportModel_NilProviderDoesNotLoad(t *testing.T) {
	p := &portfolio.Portfolio{
		Instruments: []portfolio.Instrument{
			{ID: "1", Symbol: "AAPL", Type: portfolio.InstrumentHolding, Lots: []portfolio.Lot{{ID: "l1", Quantity: 1, Price: 100, Date: time.Now()}}},
		},
	}
	m := newPortfolioExportModel(p, nil, NewStyles(ThemeDark, 0))
	if m.loading {
		t.Error("nil market provider should not trigger loading")
	}
}

func TestNewPortfolioExportModel_NoHoldingsDoesNotLoad(t *testing.T) {
	p := &portfolio.Portfolio{
		Instruments: []portfolio.Instrument{
			{ID: "1", Symbol: "TSLA", Type: portfolio.InstrumentWatchlist},
		},
	}
	m := newPortfolioExportModel(p, nil, NewStyles(ThemeDark, 0))
	if m.loading {
		t.Error("portfolio with no holdings (watchlist-only) should not trigger loading")
	}
}

func TestNewPortfolioExportModel_HoldingWithNoLotsDoesNotLoad(t *testing.T) {
	p := &portfolio.Portfolio{
		Instruments: []portfolio.Instrument{
			{ID: "1", Symbol: "AAPL", Type: portfolio.InstrumentHolding},
		},
	}
	m := newPortfolioExportModel(p, nil, NewStyles(ThemeDark, 0))
	if m.loading {
		t.Error("holding instrument with zero lots should not trigger loading")
	}
}

func TestPortfolioExportModel_HandleKeyEsc_EmitsBackResult(t *testing.T) {
	p := &portfolio.Portfolio{ID: "port-1"}
	m := newPortfolioExportModel(p, nil, NewStyles(ThemeDark, 0))
	newModel, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if newModel != m {
		t.Fatal("handleKey should return the same model instance")
	}
	if cmd == nil {
		t.Fatal("Esc should emit a ScreenDoneMsg command")
	}
	msg := cmd()
	done, ok := msg.(ScreenDoneMsg)
	if !ok {
		t.Fatalf("want ScreenDoneMsg, got %T", msg)
	}
	result, ok := done.Result.(PortfolioExportResult)
	if !ok {
		t.Fatalf("want PortfolioExportResult, got %T", done.Result)
	}
	if result.Action != "back" || result.Portfolio != p {
		t.Errorf("want back action with the same portfolio, got %+v", result)
	}
}
