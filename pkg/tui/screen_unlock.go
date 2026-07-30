package tui

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/config"
	"github.com/fuchicar/auris-ai/pkg/locale"
)

// UnlockModel prompts the user for their master passphrase to decrypt an
// existing configuration file. Shown on every non-setup launch.
type UnlockModel struct {
	input  textinput.Model
	err    string
	styles *Styles
}

// newUnlockModel constructs an [UnlockModel] ready for input.
func newUnlockModel(s *Styles) *UnlockModel {
	ti := textinput.New()
	ti.Placeholder = locale.T("unlock.placeholder")
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.Focus()

	return &UnlockModel{input: ti, styles: s}
}

// Init implements [tea.Model]; starts the cursor blink animation.
func (m *UnlockModel) Init() tea.Cmd { return textinput.Blink }

// Update implements [tea.Model]. On Enter it attempts to load the config; on
// failure it clears the input and shows the error inline.
func (m *UnlockModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter {
			passphrase := m.input.Value()
			cfg, err := config.Load(passphrase)
			if err != nil {
				if errors.Is(err, config.ErrIncorrectPassphrase) {
					m.err = locale.T("unlock.error")
				} else {
					// Not a passphrase mismatch (corrupt/unreadable config
					// file, decode failure, ...): telling the user their
					// passphrase is wrong here would send them into an
					// unwinnable retry loop, so surface the real cause.
					m.err = locale.Tp("unlock.load_error", map[string]any{"Error": err.Error()})
				}
				m.input.SetValue("")
				return m, textinput.Blink
			}
			return m, func() tea.Msg {
				return ScreenDoneMsg{
					From:   ScreenUnlock,
					Result: UnlockResult{Passphrase: passphrase, Config: cfg},
				}
			}
		}
	}

	updated, cmd := m.input.Update(msg)
	m.input = updated
	return m, cmd
}

// View implements [tea.Model].
func (m *UnlockModel) View() string {
	prompt := m.styles.Subtitle.Render(locale.T("unlock.prompt"))
	inp := m.styles.Input.Render(m.input.View())
	parts := []string{prompt, inp}
	if m.err != "" {
		parts = append(parts, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.err)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
