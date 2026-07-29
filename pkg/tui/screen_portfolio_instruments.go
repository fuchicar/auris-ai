package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

// PortfolioInstrumentsResult is emitted by portfolioInstrumentsModel.
type PortfolioInstrumentsResult struct {
	Action     string              // "view" | "add" | "back"
	Portfolio  *portfolio.Portfolio
	Instrument *portfolio.Instrument // non-nil when Action == "view"
}

// portfolioInstrumentsModel shows all instruments in a portfolio.
//
// Cursor indexes the instruments slice directly (no extra selectable header
// row).
type portfolioInstrumentsModel struct {
	portfolio *portfolio.Portfolio
	cursor    int
	scrollOff int
	height    int // terminal height (updated by WindowSizeMsg)
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
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = ws.Height
		return m, nil
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	instruments := m.portfolio.Instruments
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
		}
	case tea.KeyDown:
		if m.cursor < len(instruments)-1 {
			m.cursor++
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
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

// maxVisible returns the number of instrument rows that fit in the terminal
// after the title, separator, hint, and the two worst-case scroll indicators
// are accounted for.
func (m *portfolioInstrumentsModel) maxVisible() int {
	if m.height == 0 {
		return 20
	}
	// chrome above: title (3) + blank (1). chrome below: blank (1) + hint (1).
	n := m.height - 4 - 2 - 2 // 4 chrome + 2 indicator reserve (both ends clipped)
	if n < 0 {
		return 0
	}
	return n
}

func (m *portfolioInstrumentsModel) View() string {
	title := m.styles.Title.Render(locale.T("portfolio.instruments.title"))

	instruments := m.portfolio.Instruments
	if len(instruments) == 0 {
		hint := m.styles.Hint.Render(locale.T("portfolio.instruments.empty"))
		navHint := m.styles.Hint.Render(locale.T("portfolio.instruments.hint"))
		return lipgloss.JoinVertical(lipgloss.Left, title, "", hint, "", navHint)
	}

	rows := windowedRows(WindowedRowOpts{
		Height:           m.height,
		ScrollOff:        m.scrollOff,
		Cursor:           m.cursor,
		Total:            len(instruments),
		ChromeAbove:      4, // title (3) + the empty line that introduces the list
		ChromeBelow:      2, // blank + hint
		IndicatorReserve: 2,
		RenderRow:        m.renderRow,
		HintRender:       func(s string) string { return m.styles.Hint.Render(s) },
	})

	hint := m.styles.Hint.Render(locale.T("portfolio.instruments.hint"))
	parts := []string{title, ""}
	parts = append(parts, rows...)
	parts = append(parts, "", hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderRow returns the visible line for instrument index i, with the cursor
// arrow and styling applied.
func (m *portfolioInstrumentsModel) renderRow(i int) string {
	ins := m.portfolio.Instruments[i]
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
		return fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(label))
	}
	return fmt.Sprintf("  %s", m.styles.Unselected.Render(label))
}
