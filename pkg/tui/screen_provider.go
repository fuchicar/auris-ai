package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/registry"
)

// ProviderModel lets the user choose a market data provider from the registry.
type ProviderModel struct {
	entries   []registry.MarketEntry
	cursor    int
	scrollOff int
	height    int // terminal height (updated by WindowSizeMsg)
	styles    *Styles
}

// newProviderModel constructs a [ProviderModel] pre-loaded with all registered
// providers from [registry.All].
func newProviderModel(s *Styles) *ProviderModel {
	return &ProviderModel{
		entries: registry.AllMarket(),
		styles:  s,
	}
}

// Init implements [tea.Model].
func (m *ProviderModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model]. Arrow keys navigate; Enter confirms selection.
func (m *ProviderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = ws.Height
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
				m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
			}
		case tea.KeyDown:
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
			}
		case tea.KeyEsc:
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenProvider, Result: nil}
			}
		case tea.KeyEnter:
			entry := m.entries[m.cursor]
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenProvider, Result: ProviderResult{Entry: entry}}
			}
		}
	}
	return m, nil
}

// maxVisible returns the number of provider rows that fit in the terminal
// after the title, wrapped explain text, blank separator, hint, and worst-case
// two scroll indicators are accounted for.
func (m *ProviderModel) maxVisible() int {
	if m.height == 0 {
		return 20
	}
	// chrome below: hint (1) = 1.
	n := m.height - m.chromeAbove() - 1 - 2 // chrome-above + 1 chrome-below + 2 indicator reserve
	if n < 0 {
		return 0
	}
	return n
}

// chromeAbove measures the actual chrome above the provider list: title and
// the wrapped explain paragraph. Rendered and measured with lipgloss.Height
// instead of a guessed constant, since the explain text wraps to a different
// number of lines per locale (en vs es translations, future copy edits).
func (m *ProviderModel) chromeAbove() int {
	title := m.styles.Subtitle.Render(locale.T("setup.provider.label"))
	explain := m.styles.Help.Render(locale.T("setup.provider.explain"))
	return lipgloss.Height(title) + lipgloss.Height(explain) + 1 // blank row above the list
}

// View implements [tea.Model].
func (m *ProviderModel) View() string {
	title := m.styles.Subtitle.Render(locale.T("setup.provider.label"))
	explain := m.styles.Help.Render(locale.T("setup.provider.explain"))

	rows := windowedRows(WindowedRowOpts{
		Height:           m.height,
		ScrollOff:        m.scrollOff,
		Cursor:           m.cursor,
		Total:            len(m.entries),
		ChromeAbove:      m.chromeAbove(),
		ChromeBelow:      1, // hint only
		IndicatorReserve: 2,
		RenderRow:        m.renderRow,
		HintRender:       func(s string) string { return m.styles.Hint.Render(s) },
	})

	hint := m.styles.Hint.Render(locale.T("setup.provider.hint") + "  " + locale.T("hint.esc_back"))
	parts := []string{title, explain, ""}
	parts = append(parts, rows...)
	parts = append(parts, hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderRow returns the visible provider line for index i. Pure rendering —
// no chrome/header concern here, just the row.
func (m *ProviderModel) renderRow(i int) string {
	e := m.entries[i]
	if i == m.cursor {
		return fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(e.DisplayName))
	}
	return fmt.Sprintf("  %s", m.styles.Unselected.Render(e.DisplayName))
}
