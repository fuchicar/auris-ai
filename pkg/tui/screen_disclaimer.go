package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
)

// DisclaimerModel is shown once during first-run setup.
// The user must type "yes", "si", or "sí" (case-insensitive) to proceed.
type DisclaimerModel struct {
	input  textinput.Model
	err    string
	styles *Styles
}

func newDisclaimerModel(s *Styles) *DisclaimerModel {
	ti := textinput.New()
	ti.Placeholder = locale.T("disclaimer.input.placeholder")
	ti.CharLimit = 10
	ti.Focus()
	return &DisclaimerModel{input: ti, styles: s}
}

// Init implements [tea.Model].
func (m *DisclaimerModel) Init() tea.Cmd { return textinput.Blink }

// Update implements [tea.Model].
func (m *DisclaimerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		return m.validate()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *DisclaimerModel) validate() (tea.Model, tea.Cmd) {
	v := strings.ToLower(strings.TrimSpace(m.input.Value()))
	if v == "yes" || v == "si" || v == "sí" {
		return m, func() tea.Msg { return ScreenDoneMsg{From: ScreenDisclaimer} }
	}
	m.err = locale.T("disclaimer.input.error")
	m.input.SetValue("")
	return m, textinput.Blink
}

// View implements [tea.Model].
func (m *DisclaimerModel) View() string {
	title := m.styles.Title.Render(locale.T("disclaimer.title"))
	body  := m.styles.WarnBox.Render(locale.T("disclaimer.body"))
	label := m.styles.Warning.Render(locale.T("disclaimer.input.label"))
	inp   := m.styles.Input.Render(m.input.View())

	var bottom string
	if m.err != "" {
		bottom = m.styles.Error.Render("✗ " + m.err)
	} else {
		bottom = m.styles.Hint.Render(locale.T("disclaimer.input.hint"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, title, body, "", label, inp, bottom)
}
