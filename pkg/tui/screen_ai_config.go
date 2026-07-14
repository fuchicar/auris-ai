package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/llm"
	"auris/pkg/locale"
	"auris/pkg/registry"
)

const aiConnectTimeout = 15 * time.Second

// aiConfigStep tracks which sub-step the configuration screen is on.
type aiConfigStep int

const (
	aiStepOllamaMode aiConfigStep = iota // Ollama only: local vs remote selection
	aiStepBaseURL                        // Ollama remote, or openai_compatible: enter server URL
	aiStepAPIKey                         // API key (required for most providers, optional for Ollama)
	aiStepModelName                      // openai_compatible only: free-text default model ID
	aiStepConnecting                     // async connect + list models
	aiStepError                          // show error; Enter returns to the failed step to edit it, Esc cancels
)

// aiConnectResultMsg carries the outcome of an async connect + ListModels call.
type aiConnectResultMsg struct {
	models []llm.Model
	err    error
}

// AIProviderConfigModel collects configuration for a single AI provider,
// connects to it, lists its available models, and emits [ScreenDoneMsg] on success.
type AIProviderConfigModel struct {
	entry      registry.LLMEntry
	step       aiConfigStep
	errStep    aiConfigStep // step to return to for editing after a connect error
	modeSelect int          // cursor for local/remote option (Ollama only)
	baseURL    string
	apiKey     string
	modelName  string // openai_compatible only: user-typed default model ID
	input      textinput.Model
	spin       spinner.Model
	err        string
	styles     *Styles
}

// newAIProviderConfigModel constructs an [AIProviderConfigModel] for the given
// provider. Ollama starts at the local/remote selection step; openai_compatible
// starts at the base URL step (it has no local/remote concept — the base URL is
// always required); other providers start directly at the API key step.
func newAIProviderConfigModel(entry registry.LLMEntry, s *Styles) *AIProviderConfigModel {
	ti := textinput.New()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = s.Spinner

	m := &AIProviderConfigModel{
		entry:  entry,
		input:  ti,
		spin:   sp,
		styles: s,
	}

	switch entry.Key {
	case "ollama":
		m.step = aiStepOllamaMode
	case "openai_compatible":
		m.step = aiStepBaseURL
		m.input.Placeholder = "https://api.example.com/v1"
		m.input.Focus()
	default:
		m.step = aiStepAPIKey
		m.input.Placeholder = "API key"
		m.input.EchoMode = textinput.EchoPassword
		m.input.Focus()
	}
	return m
}

// echoModeFor returns the [textinput.EchoMode] appropriate for step — masked
// for the API key step (a secret), plain for every other step (URLs and model
// names are not sensitive and are useful to proofread on screen).
func echoModeFor(step aiConfigStep) textinput.EchoMode {
	if step == aiStepAPIKey {
		return textinput.EchoPassword
	}
	return textinput.EchoNormal
}

// Init implements [tea.Model].
func (m *AIProviderConfigModel) Init() tea.Cmd {
	if m.step == aiStepAPIKey || m.step == aiStepBaseURL {
		return textinput.Blink
	}
	return nil
}

// Update implements [tea.Model].
func (m *AIProviderConfigModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case aiConnectResultMsg:
		if msg.err != nil {
			m.err = locale.Tp("setup.ai.config.error", map[string]any{"Error": msg.err.Error()})
			m.step = aiStepError
			return m, nil
		}
		result := AIProviderConfigResult{
			Key:     m.entry.Key,
			BaseURL: m.baseURL,
			APIKey:  m.apiKey,
			Models:  msg.models,
		}
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenAIProviderConfig, Result: result}
		}

	case spinner.TickMsg:
		if m.step == aiStepConnecting {
			sp, cmd := m.spin.Update(msg)
			m.spin = sp
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	// Non-key messages (blink timer, etc.) go to the text input when active.
	if m.step == aiStepBaseURL || m.step == aiStepAPIKey || m.step == aiStepModelName {
		in, cmd := m.input.Update(msg)
		m.input = in
		return m, cmd
	}
	return m, nil
}

func (m *AIProviderConfigModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {

	case aiStepOllamaMode:
		switch msg.Type {
		case tea.KeyUp:
			if m.modeSelect > 0 {
				m.modeSelect--
			}
		case tea.KeyDown:
			if m.modeSelect < 1 {
				m.modeSelect++
			}
		case tea.KeyEsc:
			return m, cancelConfigCmd()
		case tea.KeyEnter:
			if m.modeSelect == 0 {
				// Local: connect immediately with defaults
				m.errStep = aiStepOllamaMode
				return m.startConnecting()
			}
			// Remote: ask for server URL next
			m.step = aiStepBaseURL
			m.input.Placeholder = "http://my-server:11434"
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
		}
		return m, nil

	case aiStepBaseURL:
		if msg.Type == tea.KeyEsc {
			return m, cancelConfigCmd()
		}
		if msg.Type == tea.KeyEnter && m.input.Value() != "" {
			m.baseURL = m.input.Value()
			m.step = aiStepAPIKey
			if m.entry.Key == "ollama" {
				m.input.Placeholder = "API key (optional)"
			} else {
				m.input.Placeholder = "API key"
			}
			m.input.EchoMode = echoModeFor(m.step)
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
		}
		in, cmd := m.input.Update(msg)
		m.input = in
		return m, cmd

	case aiStepAPIKey:
		switch msg.Type {
		case tea.KeyEnter:
			if m.entry.Key != "ollama" && m.input.Value() == "" {
				return m, nil // required for non-Ollama providers
			}
			m.apiKey = m.input.Value()
			if m.entry.Key == "openai_compatible" {
				m.step = aiStepModelName
				m.input.Placeholder = "gpt-4o-mini"
				m.input.EchoMode = echoModeFor(m.step)
				m.input.SetValue("")
				m.input.Focus()
				return m, textinput.Blink
			}
			m.input.Blur()
			m.errStep = aiStepAPIKey
			return m.startConnecting()
		case tea.KeyEsc:
			if m.entry.Key == "ollama" {
				// API key is optional for Ollama
				m.apiKey = ""
				m.input.Blur()
				m.errStep = aiStepAPIKey
				return m.startConnecting()
			}
			return m, cancelConfigCmd()
		}
		in, cmd := m.input.Update(msg)
		m.input = in
		return m, cmd

	case aiStepModelName:
		switch msg.Type {
		case tea.KeyEnter:
			if m.input.Value() == "" {
				return m, nil // default model is required
			}
			m.modelName = m.input.Value()
			m.input.Blur()
			m.errStep = aiStepModelName
			return m.startConnecting()
		case tea.KeyEsc:
			return m, cancelConfigCmd()
		}
		in, cmd := m.input.Update(msg)
		m.input = in
		return m, cmd

	case aiStepError:
		switch msg.Type {
		case tea.KeyEnter:
			// Return to the step that failed so the user can correct what
			// they typed, instead of blindly resubmitting the same value.
			m.err = ""
			m.step = m.errStep
			if m.step == aiStepBaseURL || m.step == aiStepAPIKey || m.step == aiStepModelName {
				m.input.EchoMode = echoModeFor(m.step)
				m.input.Focus()
				return m, textinput.Blink
			}
			return m, nil
		case tea.KeyEsc:
			return m, cancelConfigCmd()
		}
		return m, nil
	}

	return m, nil
}

// cancelConfigCmd emits the "cancelled via Esc" signal. app.go's transition
// treats a nil Result from ScreenAIProviderConfig as "back one level to
// provider selection", mirroring the same convention already used by
// ScreenAIProviderSelect and ScreenMarketProviderManage.
func cancelConfigCmd() tea.Cmd {
	return func() tea.Msg {
		return ScreenDoneMsg{From: ScreenAIProviderConfig, Result: nil}
	}
}

func (m *AIProviderConfigModel) startConnecting() (tea.Model, tea.Cmd) {
	m.step = aiStepConnecting
	entry := m.entry
	baseURL := m.baseURL
	apiKey := m.apiKey
	modelName := m.modelName
	return m, tea.Batch(m.spin.Tick, aiConnectCmd(entry, baseURL, apiKey, modelName))
}

// aiConnectCmd constructs the driver, calls Connect, lists models, and returns
// the result as an [aiConnectResultMsg]. When modelName is non-empty (the
// openai_compatible provider, where the user typed the default model by hand),
// ListModels is skipped entirely — the chosen model doesn't depend on the
// third-party endpoint implementing model discovery.
func aiConnectCmd(entry registry.LLMEntry, baseURL, apiKey, modelName string) tea.Cmd {
	return func() tea.Msg {
		drv := entry.New(baseURL, apiKey)
		ctx, cancel := context.WithTimeout(context.Background(), aiConnectTimeout)
		defer cancel()
		if err := drv.Connect(ctx); err != nil {
			return aiConnectResultMsg{err: err}
		}
		if modelName != "" {
			return aiConnectResultMsg{models: []llm.Model{{ID: modelName, Name: modelName}}}
		}
		models, err := drv.ListModels(ctx)
		return aiConnectResultMsg{models: models, err: err}
	}
}

// View implements [tea.Model].
func (m *AIProviderConfigModel) View() string {
	title := m.styles.Subtitle.Render(m.entry.DisplayName)
	escHint := locale.T("hint.esc_back")

	switch m.step {
	case aiStepOllamaMode:
		label := m.styles.Unselected.Render(locale.T("setup.ai.config.ollama.mode"))
		options := []string{
			locale.T("setup.ai.config.ollama.local"),
			locale.T("setup.ai.config.ollama.remote"),
		}
		var rows []string
		for i, opt := range options {
			if i == m.modeSelect {
				rows = append(rows, fmt.Sprintf("%s %s",
					m.styles.Cursor.Render(">"),
					m.styles.Selected.Render(opt),
				))
			} else {
				rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(opt)))
			}
		}
		hint := m.styles.Hint.Render(locale.T("setup.ai.model.hint") + "  " + escHint)
		parts := append([]string{title, "", label}, rows...)
		parts = append(parts, hint)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)

	case aiStepBaseURL:
		labelKey, hintKey := "setup.ai.config.baseurl.label", "setup.ai.config.baseurl.hint"
		if m.entry.Key != "ollama" {
			labelKey, hintKey = "setup.ai.config.baseurl.generic.label", "setup.ai.config.baseurl.generic.hint"
		}
		label := m.styles.Unselected.Render(locale.T(labelKey))
		inp := m.styles.Input.Render(m.input.View())
		hint := m.styles.Hint.Render(locale.T(hintKey) + "  " + escHint)
		return lipgloss.JoinVertical(lipgloss.Left, title, "", label, inp, hint)

	case aiStepAPIKey:
		var labelKey, hintKey string
		if m.entry.Key == "ollama" {
			labelKey = "setup.ai.config.apikey.label"
			hintKey = "setup.ai.config.apikey.hint"
		} else {
			labelKey = "setup.ai.config.apikey.required.label"
			hintKey = "setup.ai.config.apikey.required.hint"
		}
		label := m.styles.Unselected.Render(locale.T(labelKey))
		inp := m.styles.Input.Render(m.input.View())
		hint := m.styles.Hint.Render(locale.T(hintKey) + "  " + escHint)
		return lipgloss.JoinVertical(lipgloss.Left, title, "", label, inp, hint)

	case aiStepModelName:
		label := m.styles.Unselected.Render(locale.T("setup.ai.config.model.label"))
		inp := m.styles.Input.Render(m.input.View())
		hint := m.styles.Hint.Render(locale.T("setup.ai.config.model.hint") + "  " + escHint)
		return lipgloss.JoinVertical(lipgloss.Left, title, "", label, inp, hint)

	case aiStepConnecting:
		status := m.styles.Hint.Render(
			fmt.Sprintf("%s %s", m.spin.View(), locale.T("setup.ai.config.connecting")),
		)
		return lipgloss.JoinVertical(lipgloss.Left, title, "", status)

	case aiStepError:
		errMsg := m.styles.Error.Render(fmt.Sprintf("✗ %s", m.err))
		hint := m.styles.Hint.Render(locale.T("setup.ai.config.retry") + "  " + escHint)
		return lipgloss.JoinVertical(lipgloss.Left, title, "", errMsg, hint)
	}

	return title
}
