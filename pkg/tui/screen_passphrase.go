package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
)

const minPassphraseLen = 8

// PassphraseModel guides the user through creating a master passphrase.
// It shows two password fields and validates that they match and meet the
// minimum length requirement before proceeding.
type PassphraseModel struct {
	step    int // 0 = first field active, 1 = confirm field active
	first   textinput.Model
	confirm textinput.Model
	err     string
	styles  *Styles
}

// newPassphraseModel constructs a [PassphraseModel] with the first field focused.
func newPassphraseModel(s *Styles) *PassphraseModel {
	newField := func(placeholder string) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
		return ti
	}

	first := newField("passphrase")
	first.Focus()

	return &PassphraseModel{
		step:    0,
		first:   first,
		confirm: newField("confirm passphrase"),
		styles:  s,
	}
}

// Init implements [tea.Model]; starts the cursor blink on the first field.
func (m *PassphraseModel) Init() tea.Cmd { return textinput.Blink }

// Update implements [tea.Model]. Tab switches between fields; Enter on the
// confirm field validates and advances the setup flow.
func (m *PassphraseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyTab, tea.KeyShiftTab:
			return m.toggleField()
		case tea.KeyEnter:
			if m.step == 0 {
				return m.toggleField()
			}
			return m.validate()
		}
	}

	var cmd tea.Cmd
	if m.step == 0 {
		m.first, cmd = m.first.Update(msg)
	} else {
		m.confirm, cmd = m.confirm.Update(msg)
	}
	return m, cmd
}

// toggleField switches focus between the two password inputs.
func (m *PassphraseModel) toggleField() (tea.Model, tea.Cmd) {
	if m.step == 0 {
		m.step = 1
		m.first.Blur()
		m.confirm.Focus()
	} else {
		m.step = 0
		m.confirm.Blur()
		m.first.Focus()
	}
	return m, textinput.Blink
}

// validate checks length and match constraints and emits [ScreenDoneMsg] on success.
func (m *PassphraseModel) validate() (tea.Model, tea.Cmd) {
	p := m.first.Value()
	if len(p) < minPassphraseLen {
		m.err = locale.T("setup.passphrase.short")
		m.first.SetValue("")
		m.confirm.SetValue("")
		m.step = 0
		m.first.Focus()
		m.confirm.Blur()
		return m, textinput.Blink
	}
	if p != m.confirm.Value() {
		m.err = locale.T("setup.passphrase.mismatch")
		m.first.SetValue("")
		m.confirm.SetValue("")
		m.step = 0
		m.first.Focus()
		m.confirm.Blur()
		return m, textinput.Blink
	}

	return m, func() tea.Msg {
		return ScreenDoneMsg{From: ScreenPassphrase, Result: PassphraseResult{Passphrase: p}}
	}
}

// View implements [tea.Model].
func (m *PassphraseModel) View() string {
	prompt := m.styles.Subtitle.Render(locale.T("setup.passphrase.prompt"))
	confirmLabel := m.styles.Unselected.Render(locale.T("setup.passphrase.confirm"))
	f1 := m.styles.Input.Render(m.first.View())
	f2 := m.styles.Input.Render(m.confirm.View())
	hint := m.styles.Hint.Render(locale.T("setup.passphrase.hint"))

	parts := []string{prompt, f1, confirmLabel, f2, hint}
	if m.err != "" {
		parts = append(parts, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.err)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
