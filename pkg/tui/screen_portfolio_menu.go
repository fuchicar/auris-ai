package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

// PortfolioMenuResult is emitted by portfolioMenuModel when the user selects an action.
type PortfolioMenuResult struct {
	Action    string              // "create" | "view"
	Portfolio *portfolio.Portfolio // non-nil when Action == "view"
}

// portfolioMenuModel shows the list of portfolios with a "Create" option at the top.
//
// Cursor indexing: 0 = Create option, 1..N = portfolios[i-1].
type portfolioMenuModel struct {
	portfolios []*portfolio.Portfolio
	cursor     int
	scrollOff  int
	height     int // terminal height (updated by WindowSizeMsg)
	err        string
	styles     *Styles
}

func newPortfolioMenuModel(s *Styles, portfolios []*portfolio.Portfolio) *portfolioMenuModel {
	return &portfolioMenuModel{
		portfolios: portfolios,
		styles:     s,
	}
}

func (m *portfolioMenuModel) Init() tea.Cmd { return nil }

// itemCount returns the total number of selectable items (1 create + N portfolios).
func (m *portfolioMenuModel) itemCount() int {
	return 1 + len(m.portfolios)
}

func (m *portfolioMenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = ws.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
		}
	case tea.KeyDown:
		if m.cursor < m.itemCount()-1 {
			m.cursor++
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
		}
	case tea.KeyEnter:
		return m.selectCurrent()
	case tea.KeyEsc:
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioMenu, Result: nil}
		}
	}
	return m, nil
}

func (m *portfolioMenuModel) selectCurrent() (tea.Model, tea.Cmd) {
	if m.cursor == 0 {
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioMenu, Result: PortfolioMenuResult{Action: "create"}}
		}
	}
	p := m.portfolios[m.cursor-1]
	return m, func() tea.Msg {
		return ScreenDoneMsg{From: ScreenPortfolioMenu, Result: PortfolioMenuResult{Action: "view", Portfolio: p}}
	}
}

// maxVisible returns the number of selectable rows that fit in the terminal
// after the title, separator, hint, error, and blank separator lines are
// accounted for. Reserves up to 2 lines for both "↑ more above" and
// "↓ more below" indicators simultaneously (worst case when the cursor is
// parked mid-list).
func (m *portfolioMenuModel) maxVisible() int {
	if m.height == 0 {
		return 20
	}
	chrome := 6 // title (3) + separator (1) + blank (1) + hint (1)
	if m.err != "" {
		// Error line replaces one of the chrome-below budget items.
		chrome++
	}
	n := m.height - chrome - 2 // reserve 2 lines for up+down indicators
	if n < 0 {
		return 0
	}
	return n
}

func (m *portfolioMenuModel) View() string {
	title := m.styles.Title.Render(locale.T("portfolio.menu.title"))

	var separator string
	if len(m.portfolios) > 0 {
		separator = m.styles.Hint.Render("──────────────────────────────")
	}

	hint := m.styles.Hint.Render(locale.T("portfolio.menu.hint"))

	var errLine string
	if m.err != "" {
		errLine = m.styles.Error.Render(fmt.Sprintf("✗ %s", m.err))
	}

	rows := windowedRows(WindowedRowOpts{
		Height:      m.height,
		ScrollOff:   m.scrollOff,
		Cursor:      m.cursor,
		Total:       m.itemCount(),
		ChromeAbove: 4, // title (3) + separator (1)
		ChromeBelow: m.errBudget(),
		// The list can scroll past both ends (cursor mid-list on a long
		// portfolio list) — reserve space for both indicators at once.
		IndicatorReserve: 2,
		RenderRow:        m.renderRow,
		HintRender:       func(s string) string { return m.styles.Hint.Render(s) },
	})

	parts := []string{title}
	if separator != "" {
		parts = append(parts, separator)
	}
	if len(m.portfolios) == 0 {
		parts = append(parts, m.styles.Hint.Render(locale.T("portfolio.menu.empty")))
	}
	parts = append(parts, rows...)
	parts = append(parts, "", hint)
	if errLine != "" {
		parts = append(parts, errLine)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// errBudget returns 3 when an error is being shown (blank + hint + error) and
// 2 otherwise (blank + hint). Indicator space is reserved separately by
// maxVisible so this stays narrowly scoped to the literal chrome lines below
// the list window.
func (m *portfolioMenuModel) errBudget() int {
	if m.err != "" {
		return 3
	}
	return 2
}

// renderRow returns the visible line for cursor index i: 0 = "Create",
// 1..N = portfolios[i-1].
func (m *portfolioMenuModel) renderRow(i int) string {
	if i == 0 {
		label := locale.T("portfolio.menu.create")
		if m.cursor == 0 {
			return fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(label))
		}
		return fmt.Sprintf("  %s", m.styles.Unselected.Render(label))
	}
	p := m.portfolios[i-1]
	label := p.Name
	if p.Description != "" {
		label = fmt.Sprintf("%-24s %s", p.Name, m.styles.Hint.Render(p.Description))
	}
	if m.cursor == i {
		return fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(label))
	}
	return fmt.Sprintf("  %s", m.styles.Unselected.Render(label))
}
