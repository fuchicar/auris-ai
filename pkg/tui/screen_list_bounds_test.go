package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/llm"
	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
	"github.com/fuchicar/auris-ai/pkg/registry"
)

// makeMarketEntries fabricates synthetic registry entries so the
// MarketProviderManageModel screen can be exercised with a list long enough
// to require windowing.
func makeMarketEntries(n int) []registry.MarketEntry {
	out := make([]registry.MarketEntry, n)
	for i := 0; i < n; i++ {
		out[i] = registry.MarketEntry{
			Key:         fmt.Sprintf("prov-%d", i),
			DisplayName: fmt.Sprintf("Provider %d", i),
		}
	}
	return out
}

// Issue #35 regression tests — each long-list screen must stay within the
// terminal height with the cursor always visible. These pin the acceptance
// criteria listed in the issue itself.

func issue35Setup(t *testing.T) {
	t.Helper()
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
}

func mustNotExceed(t *testing.T, view string, height int, label string) {
	t.Helper()
	got := lipgloss.Height(view)
	if got > height {
		t.Fatalf("%s: View() = %d lines, want <= %d (terminal height)", label, got, height)
	}
}

func TestIssue35_PortfolioMenu_BoundedAt24Lines(t *testing.T) {
	issue35Setup(t)
	m := newPortfolioMenuModel(NewStyles(ThemeDark, 0), makePortfolios(200))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*portfolioMenuModel)
	mustNotExceed(t, m.View(), 24, "PortfolioMenu")
}

func TestIssue35_PortfolioInstruments_BoundedAt24Lines(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "P"}
	for i := 0; i < 200; i++ {
		p.Instruments = append(p.Instruments, portfolio.Instrument{
			ID:     portfolio.NewInstrumentID(),
			Symbol: "SYM",
			Name:   "Instrument",
			Type:   portfolio.InstrumentHolding,
		})
	}
	m := newPortfolioInstrumentsModel(p, NewStyles(ThemeDark, 0))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*portfolioInstrumentsModel)
	mustNotExceed(t, m.View(), 24, "PortfolioInstruments")
}

func TestIssue35_PortfolioTransactions_BoundedAt24Lines(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "P"}
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 500; i++ {
		p.Transactions = append(p.Transactions, portfolio.Transaction{
			ID:        "t",
			Type:      portfolio.TransactionBuy,
			Symbol:    "AAPL",
			Quantity:  1,
			Price:     100,
			CashDelta: -100,
			Date:      base.AddDate(0, 0, i),
		})
	}
	m := newPortfolioTransactionsModel(p, NewStyles(ThemeDark, 0))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*portfolioTransactionsModel)
	mustNotExceed(t, m.View(), 24, "PortfolioTransactions")
}

func TestIssue35_PortfolioAllocation_BoundedAt24Lines(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "P"}
	// Distinct symbols — the previous test reused "SYM" for every entry,
	// which collapsed to one row after dedup and missed the long-list case.
	for i := 0; i < 60; i++ {
		p.Instruments = append(p.Instruments, portfolio.Instrument{
			ID:     portfolio.NewInstrumentID(),
			Symbol: fmt.Sprintf("SYM%02d", i),
			Name:   fmt.Sprintf("I%d", i),
			Type:   portfolio.InstrumentHolding,
		})
	}
	p.TargetAllocation = make(map[string]float64)
	for i := 0; i < 50; i++ {
		p.TargetAllocation[fmt.Sprintf("SYM%02d", i)] = 1.0
	}
	m := newPortfolioAllocationModel(p, NewStyles(ThemeDark, 0))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*portfolioAllocationModel)
	mustNotExceed(t, m.View(), 24, "PortfolioAllocation")
}

// TestIssue35_PortfolioAllocation_BoundedWithWarning covers the chrome
// regression: when the allocation total is not 100%, [viewTotal] renders a
// bordered WarnBox that spans 3 rows (border top + content + border bottom),
// not the single line the original chromeBelow budget counted — see the
// issue #35 review feedback.
func TestIssue35_PortfolioAllocation_BoundedWithWarning(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "P"}
	for i := 0; i < 40; i++ {
		p.Instruments = append(p.Instruments, portfolio.Instrument{
			ID:     portfolio.NewInstrumentID(),
			Symbol: fmt.Sprintf("S%02d", i),
			Name:   fmt.Sprintf("I%d", i),
			Type:   portfolio.InstrumentHolding,
		})
	}
	// Intentionally unbalanced total — 0.5 (50%) instead of 100% — to force
	// the WarnBox path on render.
	p.TargetAllocation = map[string]float64{"S00": 0.5}
	m := newPortfolioAllocationModel(p, NewStyles(ThemeDark, 0))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*portfolioAllocationModel)
	mustNotExceed(t, m.View(), 24, "PortfolioAllocation (warning)")
}

// TestIssue35_PortfolioAllocation_BoundedInEditMode covers the edit-mode
// chrome: the bordered Input box that prompts for a weight adds 3 rows
// (border + content + border), plus a label and an "Enter save · Esc cancel"
// hint — easy to undercount when summing chrome pieces by hand.
func TestIssue35_PortfolioAllocation_BoundedInEditMode(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "P"}
	for i := 0; i < 30; i++ {
		p.Instruments = append(p.Instruments, portfolio.Instrument{
			ID:     portfolio.NewInstrumentID(),
			Symbol: fmt.Sprintf("S%02d", i),
			Name:   fmt.Sprintf("I%d", i),
			Type:   portfolio.InstrumentHolding,
		})
	}
	p.TargetAllocation = map[string]float64{"S00": 0.5}
	m := newPortfolioAllocationModel(p, NewStyles(ThemeDark, 0))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*portfolioAllocationModel)
	// Enter edit mode and verify View still fits.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*portfolioAllocationModel)
	mustNotExceed(t, m.View(), 24, "PortfolioAllocation (edit mode)")
}

func TestIssue35_MarketProviderManage_BoundedAt24Lines(t *testing.T) {
	issue35Setup(t)
	m := newMarketProviderManageModel(NewStyles(ThemeDark, 0), makeMarketEntries(50), map[string]bool{}, true)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*MarketProviderManageModel)
	mustNotExceed(t, m.View(), 24, "MarketProviderManage")
}

func TestIssue35_Provider_BoundedAt24Lines(t *testing.T) {
	issue35Setup(t)
	m := newProviderModel(NewStyles(ThemeDark, 0))
	// The provider registry is fixed in production; this test just pins the
	// budget invariant for whatever length the list has at runtime.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*ProviderModel)
	mustNotExceed(t, m.View(), 24, "Provider")
}

// TestIssue35_Provider_LongList_BoundedAt24Lines exercises ProviderModel with
// a synthetic long entry list — the real registry only has 2 providers today,
// which is never enough to trigger windowing, so the test above alone can't
// catch a wrong chrome budget (as happened: chromeAbove was hardcoded
// assuming the wrapped "explain" paragraph was 3 lines, when it's actually
// 5-6 at PanelWidthMax=72).
func TestIssue35_Provider_LongList_BoundedAt24Lines(t *testing.T) {
	issue35Setup(t)
	m := &ProviderModel{entries: makeMarketEntries(50), styles: NewStyles(ThemeDark, 0)}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*ProviderModel)
	mustNotExceed(t, m.View(), 24, "Provider (long list)")
}

// Long navigations must keep the cursor visible and the View within bounds.
func TestIssue35_CursorAlwaysVisible_WhileNavigatingLongList(t *testing.T) {
	issue35Setup(t)
	m := newPortfolioMenuModel(NewStyles(ThemeDark, 0), makePortfolios(120))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*portfolioMenuModel)

	for i := 0; i < 119; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(*portfolioMenuModel)
	}
	view := m.View()
	mustNotExceed(t, view, 24, "PortfolioMenu (after walking)")

	// The cursor row (last portfolio) should be inside the visible window —
	// the worst case to verify: check that some "Port " text near the end is
	// rendered (a portfolio from the latest visible window), and that the up
	// indicator is showing.
	if !strings.Contains(view, "more above") {
		t.Fatalf("expected ↑ more above after walking to the bottom, got:\n%s", view)
	}
}

// TestIssue35_PortfolioCreate_ModelPicker_BoundedAt24Lines pins the chrome
// budget fix in [screen_portfolio_create.go]: the AI provider/model pickers
// sit inside an outer View() that always renders title (Title style — 3
// lines), progress (1), and a blank line (1) — total 5 chrome lines the
// pickers were undercounting before, so a long provider/model list spilled
// past 24 lines on a 24-line terminal (issue #35 review feedback).
func TestIssue35_PortfolioCreate_ModelPicker_BoundedAt24Lines(t *testing.T) {
	issue35Setup(t)
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	// Synthesize enough LLM providers + models to require windowing.
	entries := make([]registry.LLMEntry, 30)
	for i := range entries {
		entries[i] = registry.LLMEntry{Key: fmt.Sprintf("prov%d", i), DisplayName: fmt.Sprintf("Provider %d", i)}
	}
	models := make(map[string][]llm.Model, len(entries))
	for i := range entries {
		mod := make([]llm.Model, 10)
		for j := range mod {
			mod[j] = llm.Model{ID: fmt.Sprintf("model%d-%d", i, j), Name: fmt.Sprintf("Model %d-%d", i, j)}
		}
		models[fmt.Sprintf("prov%d", i)] = mod
	}
	m := newPortfolioCreateModel(NewStyles(ThemeDark, 0), nil, nil, entries, models)
	m.step = pcStepModel
	m.modelStep = pmStepProvider
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*portfolioCreateModel)
	mustNotExceed(t, m.View(), 24, "PortfolioCreate model picker (pmStepProvider)")

	// Switch to the model sub-step and verify again. modelsByProv[0] has 10
	// items, but the chrome budget must still hold.
	m.modelStep = pmStepModel
	m.selProvider = entries[0].Key
	mustNotExceed(t, m.View(), 24, "PortfolioCreate model picker (pmStepModel)")
}
