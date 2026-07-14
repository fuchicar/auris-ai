package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
	"auris/pkg/registry"
)

// ProviderModel lets the user choose a market data provider from the registry.
type ProviderModel struct {
	entries []registry.MarketEntry
	cursor  int
	styles  *Styles
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
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.entries)-1 {
				m.cursor++
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

// View implements [tea.Model].
func (m *ProviderModel) View() string {
	title := m.styles.Subtitle.Render(locale.T("setup.provider.label"))

	var rows []string
	for i, e := range m.entries {
		var row string
		if i == m.cursor {
			row = fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(e.DisplayName))
		} else {
			row = fmt.Sprintf("  %s", m.styles.Unselected.Render(e.DisplayName))
		}
		rows = append(rows, row)
	}

	hint := m.styles.Hint.Render(locale.T("setup.provider.hint") + "  " + locale.T("hint.esc_back"))
	parts := append([]string{title}, rows...)
	parts = append(parts, hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
