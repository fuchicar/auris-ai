package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
	"auris/pkg/portfolio"
)

// PortfolioInstrumentsResult is emitted by portfolioInstrumentsModel.
type PortfolioInstrumentsResult struct {
	Action     string              // "view" | "add" | "back"
	Portfolio  *portfolio.Portfolio
	Instrument *portfolio.Instrument // non-nil when Action == "view"
}

// portfolioInstrumentsModel shows all instruments in a portfolio.
type portfolioInstrumentsModel struct {
	portfolio *portfolio.Portfolio
	cursor    int
	styles    *Styles
}

func newPortfolioInstrumentsModel(p *portfolio.Portfolio, s *Styles) *portfolioInstrumentsModel {
	return &portfolioInstrumentsModel{
		portfolio: p,
		styles:    s,
	}
}

func (m *portfolioInstrumentsModel) Init() tea.Cmd { return nil }

func (m *portfolioInstrumentsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	instruments := m.portfolio.Instruments
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(instruments)-1 {
			m.cursor++
		}
	case tea.KeyEnter:
		if len(instruments) == 0 {
			return m, nil
		}
		ins := &m.portfolio.Instruments[m.cursor]
		p := m.portfolio
		return m, func() tea.Msg {
			return ScreenDoneMsg{
				From: ScreenPortfolioInstruments,
				Result: PortfolioInstrumentsResult{
					Action:     "view",
					Portfolio:  p,
					Instrument: ins,
				},
			}
		}
	case tea.KeyEsc:
		p := m.portfolio
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioInstruments, Result: PortfolioInstrumentsResult{Action: "back", Portfolio: p}}
		}
	case tea.KeyRunes:
		if key.String() == "a" {
			p := m.portfolio
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenPortfolioInstruments, Result: PortfolioInstrumentsResult{Action: "add", Portfolio: p}}
			}
		}
	}
	return m, nil
}

func (m *portfolioInstrumentsModel) View() string {
	title := m.styles.Title.Render(locale.T("portfolio.instruments.title"))

	instruments := m.portfolio.Instruments
	if len(instruments) == 0 {
		hint := m.styles.Hint.Render(locale.T("portfolio.instruments.empty"))
		navHint := m.styles.Hint.Render(locale.T("portfolio.instruments.hint"))
		return lipgloss.JoinVertical(lipgloss.Left, title, "", hint, "", navHint)
	}

	var rows []string
	for i, ins := range instruments {
		var badgeKey string
		switch ins.Type {
		case portfolio.InstrumentHolding:
			badgeKey = "portfolio.instruments.badge.holding"
		case portfolio.InstrumentWatchlist:
			badgeKey = "portfolio.instruments.badge.watchlist"
		}
		badge := m.styles.Checkbox.Render(fmt.Sprintf("[%s]", locale.T(badgeKey)))
		label := fmt.Sprintf("%s %-8s %s", badge, ins.Symbol, ins.Name)
		if ins.Type == portfolio.InstrumentHolding && len(ins.Lots) > 0 {
			lotInfo := m.styles.Hint.Render(locale.Tp("portfolio.instruments.lots", map[string]any{"Count": len(ins.Lots)}))
			label = fmt.Sprintf("%s  %s", label, lotInfo)
		}
		if i == m.cursor {
			rows = append(rows, fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(label)))
		} else {
			rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(label)))
		}
	}

	hint := m.styles.Hint.Render(locale.T("portfolio.instruments.hint"))
	parts := []string{title, ""}
	parts = append(parts, rows...)
	parts = append(parts, "", hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
