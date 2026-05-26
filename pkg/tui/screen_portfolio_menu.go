package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
	"auris/pkg/portfolio"
)

// PortfolioMenuResult is emitted by portfolioMenuModel when the user selects an action.
type PortfolioMenuResult struct {
	Action    string              // "create" | "view"
	Portfolio *portfolio.Portfolio // non-nil when Action == "view"
}

// portfolioMenuModel shows the list of portfolios with a "Create" option at the top.
type portfolioMenuModel struct {
	portfolios []*portfolio.Portfolio
	cursor     int // 0 = create, 1..N = portfolios[i-1]
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
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < m.itemCount()-1 {
			m.cursor++
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

func (m *portfolioMenuModel) View() string {
	title := m.styles.Title.Render(locale.T("portfolio.menu.title"))

	var rows []string

	// "Create" item at index 0.
	createLabel := locale.T("portfolio.menu.create")
	if m.cursor == 0 {
		rows = append(rows, fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(createLabel)))
	} else {
		rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(createLabel)))
	}

	// Separator between create and portfolio list.
	if len(m.portfolios) > 0 {
		rows = append(rows, m.styles.Hint.Render(strings.Repeat("─", 30)))
	}

	// Portfolio entries.
	for i, p := range m.portfolios {
		idx := i + 1 // cursor index for this portfolio
		label := p.Name
		if p.Description != "" {
			label = fmt.Sprintf("%-24s %s", p.Name, m.styles.Hint.Render(p.Description))
		}
		if m.cursor == idx {
			rows = append(rows, fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(label)))
		} else {
			rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(label)))
		}
	}

	if len(m.portfolios) == 0 {
		rows = append(rows, m.styles.Hint.Render(locale.T("portfolio.menu.empty")))
	}

	hint := m.styles.Hint.Render(locale.T("portfolio.menu.hint"))

	parts := []string{title}
	parts = append(parts, rows...)
	parts = append(parts, "", hint)
	if m.err != "" {
		parts = append(parts, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.err)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
