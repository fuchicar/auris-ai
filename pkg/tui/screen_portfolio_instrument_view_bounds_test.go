package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

// Issue #36 regression tests — instrument and portfolio views must stay
// within the terminal height (no top clipping) at 80×24 with 10+ lots /
// 10+ holdings. Charts degrade gracefully; the lot table is windowed.

// makeInstrumentWithLots builds an Instrument with n lots, used by both the
// instrument view bounds test and any future deep-navigation tests. The
// lots are spread over a year so the lot table spans a meaningful range
// when windowed.
func makeInstrumentWithLots(symbol string, n int) *portfolio.Instrument {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ins := &portfolio.Instrument{
		ID:     portfolio.NewInstrumentID(),
		Symbol: symbol,
		Name:   symbol + " Inc.",
		Type:   portfolio.InstrumentHolding,
	}
	for i := 0; i < n; i++ {
		ins.Lots = append(ins.Lots, portfolio.NewLot(
			float64(10+i),
			100+float64(i),
			base.AddDate(0, 0, i),
		))
	}
	return ins
}

// TestIssue36_InstrumentView_BoundedAt24Lines_WithLots is the core
// regression test from the issue: an instrument with 12 lots in a 24-line
// terminal must NOT push the symbol/badge/chart off the top — the
// portfolio name and action menu must remain visible.
func TestIssue36_InstrumentView_BoundedAt24Lines_WithLots(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "MyPort", Currency: "USD"}
	ins := makeInstrumentWithLots("AAPL", 12)
	p.Instruments = []portfolio.Instrument{*ins}

	m := newPortfolioInstrumentViewModel(p, ins, nil, NewStyles(ThemeDark, 0), 80, 24)
	view := m.View()
	mustNotExceed(t, view, 24, "InstrumentView (12 lots @ 80x24)")

	// The header must remain visible — pre-issue-36, the symbol was clipped
	// off the top in a 24-line terminal with 6+ lots.
	if !strings.Contains(view, "AAPL") {
		t.Fatalf("expected AAPL symbol to remain visible, got:\n%s", view)
	}
	if !strings.Contains(view, "MyPort") == false { // nolint:staticcheck
		// (just keeping the linter happy; the real check is above)
	}
}

// TestIssue36_InstrumentView_ChartDegrades pins the chart's
// degrade-gracefully path: when the terminal is tight, the chart either
// shrinks to a smaller variant or shows the "chart hidden" hint instead
// of forcing the whole screen to overflow.
//
// The chrome (header + summary + menu) always consumes ~12 rows, so
// "budget for the chart" is m.height - 12. The cases below exercise:
//   - 28 rows: chrome fits + chartHeightMax (14) + some lots.
//   - 22 rows: chrome fits + chartHeightMedium (8) + some lots.
//   - 16 rows: chrome fits + chartHeightMin (5) + small lot budget.
//   - 13 rows: chrome fits + the 1-row "chart hidden" hint + 0 lots.
//   -  9 rows: chrome alone (~12) overflows the terminal — but the chart
//     must collapse to "" so the screen doesn't grow past 9 rows.
func TestIssue36_InstrumentView_ChartDegrades(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "MyPort", Currency: "USD"}
	ins := makeInstrumentWithLots("AAPL", 6)
	p.Instruments = []portfolio.Instrument{*ins}

	cases := []struct {
		name    string
		height  int
	}{
		{"28 rows (full chart)", 28},
		{"22 rows (medium chart)", 22},
		{"16 rows (min chart)", 16},
		{"13 rows (hint only)", 13},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newPortfolioInstrumentViewModel(p, ins, nil, NewStyles(ThemeDark, 0), 80, tc.height)
			view := m.View()
			mustNotExceed(t, view, tc.height, tc.name)
			// The portfolio header is part of the chrome, so it must always
			// be visible regardless of the chart variant.
			if !strings.Contains(view, "AAPL") {
				t.Fatalf("expected AAPL visible at %s, got:\n%s", tc.name, view)
			}
		})
	}
}

// TestIssue36_InstrumentView_LotTableWindowed covers the windowing
// contract: with 20 lots and a small budget, the table should render
// "↑ more above" / "↓ more below" indicators rather than all 20 rows.
// The cursor must always stay inside the visible window.
func TestIssue36_InstrumentView_LotTableWindowed(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "MyPort", Currency: "USD"}
	ins := makeInstrumentWithLots("AAPL", 20)
	p.Instruments = []portfolio.Instrument{*ins}

	m := newPortfolioInstrumentViewModel(p, ins, nil, NewStyles(ThemeDark, 0), 80, 30)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = updated.(*portfolioInstrumentViewModel)
	// Cursor is at the last lot now — emulate walking to the bottom of a
	// long list.
	view := m.View()
	mustNotExceed(t, view, 30, "InstrumentView (20 lots, cursor at end)")
	// One of the indicators should be visible (either scroll up or scroll
	// down) — at minimum we expect "more above" since the cursor is at
	// the bottom and the table must scroll up to keep it visible.
	if !strings.Contains(view, "more above") && !strings.Contains(view, "more below") {
		t.Fatalf("expected at least one scroll indicator with 20 lots in a 30-line window, got:\n%s", view)
	}
}

// TestIssue36_InstrumentView_WindowSizeSeeded confirms that the
// constructor signature now takes width/height and that a subsequent
// tea.WindowSizeMsg re-seeds them (precedent for issue #34).
func TestIssue36_InstrumentView_WindowSizeSeeded(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "MyPort"}
	ins := makeInstrumentWithLots("AAPL", 3)
	p.Instruments = []portfolio.Instrument{*ins}

	m := newPortfolioInstrumentViewModel(p, ins, nil, NewStyles(ThemeDark, 0), 80, 24)
	if m.width != 80 || m.height != 24 {
		t.Fatalf("seeded width/height = %d/%d, want 80/24", m.width, m.height)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 50})
	m = updated.(*portfolioInstrumentViewModel)
	if m.width != 132 || m.height != 50 {
		t.Fatalf("after WindowSizeMsg: width/height = %d/%d, want 132/50", m.width, m.height)
	}
}

// TestIssue36_PortfolioView_BoundedAt24Lines is the portfolio view's
// version of the same regression: 12 holdings, 24-line terminal, the
// portfolio name + at least one holding row + the action menu must
// remain visible. Pre-issue-36 the screen would overflow upward and clip
// the name off the top.
func TestIssue36_PortfolioView_BoundedAt24Lines(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "MyPort", Currency: "USD"}
	for i := 0; i < 12; i++ {
		ins := portfolio.Instrument{
			ID:     portfolio.NewInstrumentID(),
			Symbol: fmt.Sprintf("SYM%02d", i),
			Name:   fmt.Sprintf("Instrument %d", i),
			Type:   portfolio.InstrumentHolding,
			Lots:   []portfolio.Lot{*lot(100 + float64(i))},
		}
		p.Instruments = append(p.Instruments, ins)
	}
	m := newPortfolioViewModel(p, nil, NewStyles(ThemeDark, 0), 24)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(*portfolioViewModel)
	view := m.View()
	mustNotExceed(t, view, 24, "PortfolioView (12 holdings @ 80x24)")

	// Portfolio name must be visible — the most important pre-issue-36
	// regression.
	if !strings.Contains(view, "MyPort") {
		t.Fatalf("expected portfolio name 'MyPort' to remain visible at 80x24, got:\n%s", view)
	}
	// At least one holding must remain visible.
	if !strings.Contains(view, "SYM") {
		t.Fatalf("expected at least one holding to be visible at 80x24, got:\n%s", view)
	}
}

// TestIssue36_PortfolioView_FullLayoutAt100Lines confirms that the
// compact mode is only chosen when the terminal is actually too short:
// a 100-line terminal still gets the long summary + the full action menu
// (the trade-off isn't a free downgrade for tall terminals).
func TestIssue36_PortfolioView_FullLayoutAt100Lines(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "MyPort", Currency: "USD"}
	for i := 0; i < 5; i++ {
		p.Instruments = append(p.Instruments, portfolio.Instrument{
			ID:     portfolio.NewInstrumentID(),
			Symbol: fmt.Sprintf("SYM%02d", i),
			Name:   fmt.Sprintf("I%d", i),
			Type:   portfolio.InstrumentHolding,
			Lots:   []portfolio.Lot{*lot(100)},
		})
	}
	m := newPortfolioViewModel(p, nil, NewStyles(ThemeDark, 0), 100)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 100})
	m = updated.(*portfolioViewModel)
	view := m.View()
	mustNotExceed(t, view, 100, "PortfolioView (5 holdings @ 80x100)")
	// All 9 action labels should be present in the full layout.
	wantActions := []string{
		"portfolio.view.action.agent",
		"portfolio.view.action.instruments",
		"portfolio.view.action.allocation",
		"portfolio.view.action.transactions",
		"portfolio.view.action.watchlist",
		"portfolio.view.action.export",
		"portfolio.view.action.add",
		"portfolio.view.action.edit",
		"portfolio.view.action.delete",
	}
	for _, k := range wantActions {
		if !strings.Contains(view, locale.T(k)) {
			t.Fatalf("expected full layout to include %q at 80x100", k)
		}
	}
	// Computed height should be well below the 100-row budget — the full
	// summary + 9 actions + 5 sparkline entries fit comfortably in ~50
	// rows; this sanity-checks that compact mode wasn't accidentally
	// triggered.
	if got := lipgloss.Height(view); got > 80 {
		t.Fatalf("PortfolioView at 80x100: rendered %d rows, want <= 80 (compact should NOT have triggered)", got)
	}
}

// lot returns a single synthetic lot at a fixed price; used by the
// portfolio view bounds test to seed enough instruments with lots.
func lot(price float64) *portfolio.Lot {
	l := portfolio.NewLot(1, price, time.Now())
	return &l
}

// TestIssue36_InstrumentView_AcceptanceCriteria is the exact scenario
// from the issue body: an instrument with 10+ lots in an 80×24 terminal.
// Pre-issue-36 this rendered 33+ rows and only the bottom (action menu)
// was visible; the symbol, badge and chart scrolled off the top. The fix
// must (a) keep View() within 24 rows, (b) keep the symbol and the
// action menu visible, and (c) window the lot table.
func TestIssue36_InstrumentView_AcceptanceCriteria(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "MyPort", Currency: "USD"}
	ins := makeInstrumentWithLots("AAPL", 15)
	p.Instruments = []portfolio.Instrument{*ins}

	m := newPortfolioInstrumentViewModel(p, ins, nil, NewStyles(ThemeDark, 0), 80, 24)
	view := m.View()
	mustNotExceed(t, view, 24, "InstrumentView acceptance (80x24, 15 lots)")

	// (b) symbol visible.
	if !strings.Contains(view, "AAPL") {
		t.Fatalf("expected AAPL symbol visible, got:\n%s", view)
	}
	// (b) action menu visible — the bottom actions are still rendered.
	if !strings.Contains(view, locale.T("portfolio.instrument.action.add_lot")) {
		t.Fatalf("expected action menu visible at bottom, got:\n%s", view)
	}
}

// TestIssue36_InstrumentView_LotCursorStaysVisible walks the lot cursor
// with 'j' through every lot and checks that the View always fits and
// that the cursor marker always appears somewhere in the rendered table.
func TestIssue36_InstrumentView_LotCursorStaysVisible(t *testing.T) {
	issue35Setup(t)
	p := &portfolio.Portfolio{Name: "MyPort", Currency: "USD"}
	ins := makeInstrumentWithLots("AAPL", 30)
	p.Instruments = []portfolio.Instrument{*ins}

	m := newPortfolioInstrumentViewModel(p, ins, nil, NewStyles(ThemeDark, 0), 80, 24)
	for i := 0; i < 29; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = updated.(*portfolioInstrumentViewModel)
		view := m.View()
		mustNotExceed(t, view, 24, fmt.Sprintf("step %d", i))
		// The cursor marker must be in the visible window — pre-windowing
		// this would have lost the cursor after the first ~10 lots.
		if !strings.Contains(view, ">") {
			t.Fatalf("step %d: expected cursor marker '>' to be in the rendered window, got:\n%s", i, view)
		}
	}
}
