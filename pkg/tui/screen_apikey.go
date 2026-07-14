package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
	"auris/pkg/registry"
)

const apiKeyConnectTimeout = 15 * time.Second

// connectResultMsg carries the outcome of an asynchronous API key validation.
type connectResultMsg struct{ err error }

// APIKeyModel collects the user's API key for the selected provider, validates
// it by calling Connect, and emits [ScreenDoneMsg] on success. When optional is
// true, Esc skips configuration entirely (emits an empty APIKey) instead of
// requiring input — used for secondary market providers in the setup wizard.
// When canGoBack is true (and optional is false), Esc instead emits a nil
// Result meaning "go back one step" — used for the mandatory primary-provider
// screen during first-run setup (see AppModel.transition's ScreenAPIKey case).
type APIKeyModel struct {
	entry      registry.MarketEntry
	from       Screen
	optional   bool
	canGoBack  bool
	input      textinput.Model
	spin       spinner.Model
	connecting bool
	err        string
	styles     *Styles
}

// newAPIKeyModel constructs an [APIKeyModel] for the given provider entry.
// from is echoed back in the emitted [ScreenDoneMsg.From] so the same model can
// serve both the mandatory primary-provider screen and the optional secondary
// one. When optional is true, Esc skips configuration (empty APIKey result).
// When canGoBack is true, Esc instead goes back a step (nil Result) — the two
// are mutually exclusive in practice (optional is only used for the secondary
// provider screen, which does not support going back).
func newAPIKeyModel(entry registry.MarketEntry, s *Styles, from Screen, optional bool, canGoBack bool) *APIKeyModel {
	ti := textinput.New()
	ti.Placeholder = "api key"
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = s.Spinner

	return &APIKeyModel{entry: entry, from: from, optional: optional, canGoBack: canGoBack, input: ti, spin: sp, styles: s}
}

// Init implements [tea.Model]; starts cursor blink on the input field.
func (m *APIKeyModel) Init() tea.Cmd { return textinput.Blink }

// Update implements [tea.Model]. Enter triggers async connection validation.
func (m *APIKeyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case connectResultMsg:
		m.connecting = false
		if msg.err != nil {
			m.err = locale.Tp("setup.apikey.error", map[string]any{"Error": msg.err.Error()})
			m.input.Focus()
			return m, textinput.Blink
		}
		apiKey := m.input.Value()
		return m, func() tea.Msg {
			return ScreenDoneMsg{
				From:   m.from,
				Result: APIKeyResult{Entry: m.entry, APIKey: apiKey},
			}
		}

	case spinner.TickMsg:
		if m.connecting {
			updated, cmd := m.spin.Update(msg)
			m.spin = updated
			return m, cmd
		}

	case tea.KeyMsg:
		if !m.connecting && m.optional && msg.Type == tea.KeyEsc {
			from, entry := m.from, m.entry
			return m, func() tea.Msg {
				return ScreenDoneMsg{
					From:   from,
					Result: APIKeyResult{Entry: entry, APIKey: ""},
				}
			}
		}
		if !m.connecting && m.canGoBack && msg.Type == tea.KeyEsc {
			from := m.from
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: from, Result: nil}
			}
		}
		if !m.connecting && msg.Type == tea.KeyEnter && m.input.Value() != "" {
			m.connecting = true
			m.err = ""
			m.input.Blur()
			apiKey := m.input.Value()
			entry := m.entry
			return m, tea.Batch(m.spin.Tick, connectCmd(entry, apiKey))
		}
	}

	if !m.connecting {
		updated, cmd := m.input.Update(msg)
		m.input = updated
		return m, cmd
	}

	return m, nil
}

// connectCmd returns a [tea.Cmd] that constructs the driver, calls Connect with
// a timeout, and wraps the outcome in a [connectResultMsg].
func connectCmd(entry registry.MarketEntry, apiKey string) tea.Cmd {
	return func() tea.Msg {
		driver := entry.New(apiKey)
		ctx, cancel := context.WithTimeout(context.Background(), apiKeyConnectTimeout)
		defer cancel()
		return connectResultMsg{err: driver.Connect(ctx)}
	}
}

// View implements [tea.Model].
func (m *APIKeyModel) View() string {
	labelKey, hintKey := "setup.apikey.label", "setup.apikey.hint"
	if m.optional {
		labelKey, hintKey = "setup.apikey.optional_label", "setup.apikey.optional_hint"
	}

	providerName := m.styles.Subtitle.Render(m.entry.DisplayName)
	docsLabel := m.styles.Hint.Render(locale.T("setup.apikey.docs_hint"))
	docsLink := m.styles.DocsURL.Render(m.entry.DocsURL)
	inputLabel := m.styles.Unselected.Render(locale.T(labelKey))

	var status string
	if m.connecting {
		status = m.styles.Hint.Render(
			fmt.Sprintf("%s %s", m.spin.View(), locale.T("setup.apikey.connecting")),
		)
	} else if m.err != "" {
		status = m.styles.Error.Render(fmt.Sprintf("✗ %s", m.err))
	} else {
		hintText := locale.T(hintKey)
		if m.canGoBack {
			hintText += "  " + locale.T("hint.esc_back")
		}
		status = m.styles.Hint.Render(hintText)
	}

	inp := m.styles.Input.Render(m.input.View())
	return lipgloss.JoinVertical(lipgloss.Left,
		providerName,
		docsLabel+" "+docsLink,
		"",
		inputLabel,
		inp,
		status,
	)
}
