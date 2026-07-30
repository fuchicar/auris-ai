package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

// Regression tests for issue #35 (screen_portfolio_menu.go): long portfolio
// lists must stay within terminal height with the cursor always visible.

func makePortfolios(n int) []*portfolio.Portfolio {
	out := make([]*portfolio.Portfolio, n)
	for i := 0; i < n; i++ {
		out[i] = &portfolio.Portfolio{
			ID:   "p-" + strings.Repeat("a", i+1),
			Name: "Port " + strings.Repeat("X", i+1),
		}
	}
	return out
}

func TestPortfolioMenu_WindowedViewStaysWithinHeight(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	m := newPortfolioMenuModel(NewStyles(ThemeDark, 0), makePortfolios(100))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*portfolioMenuModel)
	if m.height != 24 {
		t.Fatalf("height not seeded from WindowSizeMsg, got %d", m.height)
	}

	view := m.View()
	got := lipgloss.Height(view)
	if got > 24 {
		t.Fatalf("View() = %d lines, want <= 24 (terminal height)", got)
	}
	// Should clip with a "more below" indicator since 100 items > maxVisible.
	if !strings.Contains(view, "more below") {
		t.Fatalf("expected ↓ more below indicator in windowed view; got:\n%s", view)
	}
	if strings.Contains(view, "Port XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX") {
		t.Fatalf("first portfolio should not be fully rendered when clipped from above")
	}
}

func TestPortfolioMenu_CursorStaysVisibleWhileNavigating(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	m := newPortfolioMenuModel(NewStyles(ThemeDark, 0), makePortfolios(80))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = updated.(*portfolioMenuModel)

	// Walk the cursor all the way to the bottom and verify each step keeps
	// the cursor visible (rendered) and the View height bounded.
	for i := 0; i < 79; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(*portfolioMenuModel)
	}
	if m.cursor != 79 {
		t.Fatalf("cursor should be at last index, got %d", m.cursor)
	}
	view := m.View()
	if lipgloss.Height(view) > 20 {
		t.Fatalf("View() grew to %d lines, want <= 20", lipgloss.Height(view))
	}
	if !strings.Contains(view, "more above") {
		t.Fatalf("expected ↑ more above indicator after scrolling to bottom, got:\n%s", view)
	}
}

func TestPortfolioMenu_EmptyListRendersEmptyHint(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	m := newPortfolioMenuModel(NewStyles(ThemeDark, 0), nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(*portfolioMenuModel)
	view := m.View()
	if !strings.Contains(view, locale.T("portfolio.menu.empty")) {
		t.Fatalf("expected empty hint in View, got:\n%s", view)
	}
	if strings.Contains(view, "more below") {
		t.Fatalf("empty list should not show a ↓ more below indicator, got:\n%s", view)
	}
}
