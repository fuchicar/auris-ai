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
	aiStepOllamaMode  aiConfigStep = iota // Ollama only: local vs remote selection
	aiStepBaseURL                         // Ollama remote: enter server URL
	aiStepAPIKey                          // API key (required for Gemini, optional for Ollama remote)
	aiStepConnecting                      // async connect + list models
	aiStepError                           // show error with retry option
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
	modeSelect int // cursor for local/remote option (Ollama only)
	baseURL    string
	apiKey     string
	input      textinput.Model
	spin       spinner.Model
	err        string
	styles     *Styles
}

// newAIProviderConfigModel constructs an [AIProviderConfigModel] for the given
// provider. Ollama starts at the local/remote selection step; other providers
// start directly at the API key step.
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

	if entry.Key == "ollama" {
		m.step = aiStepOllamaMode
	} else {
		m.step = aiStepAPIKey
		m.input.Placeholder = "API key"
		m.input.Focus()
	}
	return m
}

// Init implements [tea.Model].
func (m *AIProviderConfigModel) Init() tea.Cmd {
	if m.step == aiStepAPIKey {
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
	if m.step == aiStepBaseURL || m.step == aiStepAPIKey {
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
		case tea.KeyEnter:
			if m.modeSelect == 0 {
				// Local: connect immediately with defaults
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
		if msg.Type == tea.KeyEnter && m.input.Value() != "" {
			m.baseURL = m.input.Value()
			m.step = aiStepAPIKey
			m.input.Placeholder = "API key (optional)"
			m.input.SetValue("")
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
			m.input.Blur()
			return m.startConnecting()
		case tea.KeyEsc:
			if m.entry.Key == "ollama" {
				// API key is optional for Ollama
				m.apiKey = ""
				m.input.Blur()
				return m.startConnecting()
			}
		}
		in, cmd := m.input.Update(msg)
		m.input = in
		return m, cmd

	case aiStepError:
		if msg.Type == tea.KeyEnter {
			m.err = ""
			return m.startConnecting()
		}
		return m, nil
	}

	return m, nil
}

func (m *AIProviderConfigModel) startConnecting() (tea.Model, tea.Cmd) {
	m.step = aiStepConnecting
	entry := m.entry
	baseURL := m.baseURL
	apiKey := m.apiKey
	return m, tea.Batch(m.spin.Tick, aiConnectCmd(entry, baseURL, apiKey))
}

// aiConnectCmd constructs the driver, calls Connect, lists models, and returns
// the result as an [aiConnectResultMsg].
func aiConnectCmd(entry registry.LLMEntry, baseURL, apiKey string) tea.Cmd {
	return func() tea.Msg {
		drv := entry.New(baseURL, apiKey)
		ctx, cancel := context.WithTimeout(context.Background(), aiConnectTimeout)
		defer cancel()
		if err := drv.Connect(ctx); err != nil {
			return aiConnectResultMsg{err: err}
		}
		models, err := drv.ListModels(ctx)
		return aiConnectResultMsg{models: models, err: err}
	}
}

// View implements [tea.Model].
func (m *AIProviderConfigModel) View() string {
	title := m.styles.Subtitle.Render(m.entry.DisplayName)

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
		hint := m.styles.Hint.Render(locale.T("setup.ai.model.hint"))
		parts := append([]string{title, "", label}, rows...)
		parts = append(parts, hint)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)

	case aiStepBaseURL:
		label := m.styles.Unselected.Render(locale.T("setup.ai.config.baseurl.label"))
		inp := m.styles.Input.Render(m.input.View())
		hint := m.styles.Hint.Render(locale.T("setup.ai.config.baseurl.hint"))
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
		hint := m.styles.Hint.Render(locale.T(hintKey))
		return lipgloss.JoinVertical(lipgloss.Left, title, "", label, inp, hint)

	case aiStepConnecting:
		status := m.styles.Hint.Render(
			fmt.Sprintf("%s %s", m.spin.View(), locale.T("setup.ai.config.connecting")),
		)
		return lipgloss.JoinVertical(lipgloss.Left, title, "", status)

	case aiStepError:
		errMsg := m.styles.Error.Render(fmt.Sprintf("✗ %s", m.err))
		hint := m.styles.Hint.Render(locale.T("setup.ai.config.retry"))
		return lipgloss.JoinVertical(lipgloss.Left, title, "", errMsg, hint)
	}

	return title
}
