package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
)

// ChangePassphraseModel lets an already-unlocked user replace their master
// passphrase. It reuses the same length/match rules as [PassphraseModel],
// plus requires the current passphrase to prove the session owner still
// knows it before the config is re-encrypted.
type ChangePassphraseModel struct {
	step            int // 0 = current, 1 = new, 2 = confirm
	current         textinput.Model
	newPass         textinput.Model
	confirm         textinput.Model
	expectedCurrent string
	err             string
	styles          *Styles
	canGoBack       bool
}

// newChangePassphraseModel constructs a [ChangePassphraseModel]. expectedCurrent
// is the passphrase already held in memory for the unlocked session, checked
// against the "current passphrase" field before any change is accepted.
func newChangePassphraseModel(s *Styles, expectedCurrent string, canGoBack bool) *ChangePassphraseModel {
	newField := func(placeholder string) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
		return ti
	}

	current := newField("current passphrase")
	current.Focus()

	return &ChangePassphraseModel{
		step:            0,
		current:         current,
		newPass:         newField("new passphrase"),
		confirm:         newField("confirm new passphrase"),
		expectedCurrent: expectedCurrent,
		styles:          s,
		canGoBack:       canGoBack,
	}
}

// Init implements [tea.Model]; starts the cursor blink on the focused field.
func (m *ChangePassphraseModel) Init() tea.Cmd { return textinput.Blink }

// Update implements [tea.Model]. Tab/Shift+Tab cycles between the three
// fields; Enter on the last field validates and, on success, emits
// [ScreenDoneMsg]. Esc cancels back to the menu when canGoBack is true.
func (m *ChangePassphraseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyTab, tea.KeyShiftTab:
			return m.moveField(key.Type == tea.KeyShiftTab)
		case tea.KeyEnter:
			if m.step < 2 {
				return m.moveField(false)
			}
			return m.validate()
		case tea.KeyEsc:
			if m.canGoBack {
				return m, func() tea.Msg {
					return ScreenDoneMsg{From: ScreenChangePassphrase, Result: nil}
				}
			}
		}
	}

	var cmd tea.Cmd
	switch m.step {
	case 0:
		m.current, cmd = m.current.Update(msg)
	case 1:
		m.newPass, cmd = m.newPass.Update(msg)
	default:
		m.confirm, cmd = m.confirm.Update(msg)
	}
	return m, cmd
}

// moveField switches focus to the previous or next field, wrapping at the ends.
func (m *ChangePassphraseModel) moveField(backward bool) (tea.Model, tea.Cmd) {
	fields := []*textinput.Model{&m.current, &m.newPass, &m.confirm}
	fields[m.step].Blur()
	if backward {
		m.step = (m.step + len(fields) - 1) % len(fields)
	} else {
		m.step = (m.step + 1) % len(fields)
	}
	fields[m.step].Focus()
	return m, textinput.Blink
}

// resetTo clears all fields and moves focus to step, used after a
// validation failure so the user retypes rather than resubmits stale values.
func (m *ChangePassphraseModel) resetTo(step int) (tea.Model, tea.Cmd) {
	m.current.Blur()
	m.newPass.Blur()
	m.confirm.Blur()
	m.current.SetValue("")
	m.newPass.SetValue("")
	m.confirm.SetValue("")
	m.step = step
	switch step {
	case 0:
		m.current.Focus()
	case 1:
		m.newPass.Focus()
	default:
		m.confirm.Focus()
	}
	return m, textinput.Blink
}

// validate checks the current passphrase, then the new passphrase's length
// and match constraints, emitting [ScreenDoneMsg] only on full success.
func (m *ChangePassphraseModel) validate() (tea.Model, tea.Cmd) {
	if m.current.Value() != m.expectedCurrent {
		m.err = locale.T("changepass.wrong_current")
		return m.resetTo(0)
	}

	newP := m.newPass.Value()
	if len(newP) < minPassphraseLen {
		m.err = locale.T("setup.passphrase.short")
		return m.resetTo(1)
	}
	if newP != m.confirm.Value() {
		m.err = locale.T("setup.passphrase.mismatch")
		return m.resetTo(1)
	}

	return m, func() tea.Msg {
		return ScreenDoneMsg{From: ScreenChangePassphrase, Result: ChangePassphraseResult{NewPassphrase: newP}}
	}
}

// View implements [tea.Model].
func (m *ChangePassphraseModel) View() string {
	title := m.styles.Subtitle.Render(locale.T("changepass.title"))
	currentLabel := m.styles.Unselected.Render(locale.T("changepass.current"))
	fCurrent := m.styles.Input.Render(m.current.View())
	newLabel := m.styles.Unselected.Render(locale.T("changepass.new"))
	fNew := m.styles.Input.Render(m.newPass.View())
	confirmLabel := m.styles.Unselected.Render(locale.T("changepass.confirm"))
	fConfirm := m.styles.Input.Render(m.confirm.View())

	hintText := locale.T("changepass.hint")
	if m.canGoBack {
		hintText += "  " + locale.T("hint.esc_back")
	}
	hint := m.styles.Hint.Render(hintText)

	parts := []string{title, currentLabel, fCurrent, newLabel, fNew, confirmLabel, fConfirm, hint}
	if m.err != "" {
		parts = append(parts, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.err)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
