package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
)

// encryptionOptions lists the two states in display order.
var encryptionOptions = []struct {
	enabled  bool
	labelKey string
}{
	{true, "encryption.enabled"},
	{false, "encryption.disabled"},
}

// EncryptionModel lets the user toggle at-rest encryption of portfolios and
// chat sessions (FEAT-16). Reached only from the Configuration menu — never
// during first-run setup, which defaults new configs to encrypted already.
type EncryptionModel struct {
	cursor int
	styles *Styles
	err    string
}

// newEncryptionModel constructs an [EncryptionModel], preselecting the
// currently active state. errMsg, if non-empty, is shown as an inline error
// — used when a prior toggle attempt failed partway through re-encrypting.
func newEncryptionModel(s *Styles, currentlyEnabled bool, errMsg string) *EncryptionModel {
	cursor := 1
	if currentlyEnabled {
		cursor = 0
	}
	return &EncryptionModel{cursor: cursor, styles: s, err: errMsg}
}

// Init implements [tea.Model].
func (m *EncryptionModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model]. Arrow keys move the cursor; Enter confirms; Esc cancels.
func (m *EncryptionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(encryptionOptions)-1 {
				m.cursor++
			}
		case tea.KeyEsc:
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenEncryption, Result: nil}
			}
		case tea.KeyEnter:
			chosen := encryptionOptions[m.cursor].enabled
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenEncryption, Result: EncryptionResult{Enabled: chosen}}
			}
		}
	}
	return m, nil
}

// View implements [tea.Model].
func (m *EncryptionModel) View() string {
	title := m.styles.Subtitle.Render(locale.T("encryption.title"))

	var rows []string
	for i, opt := range encryptionOptions {
		name := locale.T(opt.labelKey)
		var row string
		if i == m.cursor {
			row = fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(name))
		} else {
			row = fmt.Sprintf("  %s", m.styles.Unselected.Render(name))
		}
		rows = append(rows, row)
	}

	hintText := locale.T("encryption.hint") + "  " + locale.T("hint.esc_back")
	hint := m.styles.Hint.Render(hintText)

	parts := []string{title}
	parts = append(parts, rows...)
	parts = append(parts, hint)
	if m.err != "" {
		parts = append(parts, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.err)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
