package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
)

// WelcomeModel is the first screen shown on every launch.
// It displays the application title and waits for the user to press Enter.
type WelcomeModel struct {
	styles *Styles
}

// newWelcomeModel constructs a [WelcomeModel] with the given style set.
func newWelcomeModel(s *Styles) *WelcomeModel {
	return &WelcomeModel{styles: s}
}

// Init implements [tea.Model]; the welcome screen needs no I/O.
func (m *WelcomeModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model]. Pressing Enter emits [ScreenDoneMsg].
func (m *WelcomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenWelcome}
		}
	}
	return m, nil
}

// View implements [tea.Model].
func (m *WelcomeModel) View() string {
	title   := m.styles.Title.Render(locale.T("welcome.title"))
	warning := m.styles.Warning.Width(PanelWidth).Render(locale.T("welcome.warning"))
	hint    := m.styles.Hint.Render(locale.T("welcome.hint"))
	return lipgloss.JoinVertical(lipgloss.Center, title, warning, hint)
}
