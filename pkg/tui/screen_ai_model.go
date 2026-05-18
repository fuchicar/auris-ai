package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/llm"
	"auris/pkg/locale"
	"auris/pkg/registry"
)

// aiModelStep tracks which sub-step the default model selection is on.
type aiModelStep int

const (
	aiModelStepProvider aiModelStep = iota // select which provider to use by default
	aiModelStepModel                       // select a model from the chosen provider
)

// AIDefaultModelModel lets the user pick a default AI provider and then a
// default model from that provider's available model list.
type AIDefaultModelModel struct {
	entries      []registry.LLMEntry        // configured providers in order
	modelsByProv map[string][]llm.Model     // models per provider key
	step         aiModelStep
	provCursor   int
	modelCursor  int
	selProvider  string
	styles       *Styles
}

// newAIDefaultModelModel constructs an [AIDefaultModelModel].
// entries contains only the providers that were successfully configured.
// modelsByProv maps each provider key to its discovered model list.
func newAIDefaultModelModel(entries []registry.LLMEntry, modelsByProv map[string][]llm.Model, s *Styles) *AIDefaultModelModel {
	m := &AIDefaultModelModel{
		entries:      entries,
		modelsByProv: modelsByProv,
		styles:       s,
	}
	// If there is only one provider, skip directly to model selection.
	if len(entries) == 1 {
		m.selProvider = entries[0].Key
		m.step = aiModelStepModel
	}
	return m
}

// Init implements [tea.Model].
func (m *AIDefaultModelModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model].
func (m *AIDefaultModelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch m.step {
	case aiModelStepProvider:
		switch key.Type {
		case tea.KeyUp:
			if m.provCursor > 0 {
				m.provCursor--
			}
		case tea.KeyDown:
			if m.provCursor < len(m.entries)-1 {
				m.provCursor++
			}
		case tea.KeyEnter:
			m.selProvider = m.entries[m.provCursor].Key
			m.modelCursor = 0
			m.step = aiModelStepModel
		}

	case aiModelStepModel:
		models := m.modelsByProv[m.selProvider]
		switch key.Type {
		case tea.KeyUp:
			if m.modelCursor > 0 {
				m.modelCursor--
			}
		case tea.KeyDown:
			if m.modelCursor < len(models)-1 {
				m.modelCursor++
			}
		case tea.KeyEsc:
			if len(m.entries) > 1 {
				m.step = aiModelStepProvider
			}
		case tea.KeyEnter:
			if len(models) == 0 {
				return m, nil
			}
			modelID := models[m.modelCursor].ID
			prov := m.selProvider
			return m, func() tea.Msg {
				return ScreenDoneMsg{
					From:   ScreenAIDefaultModel,
					Result: AIDefaultModelResult{Provider: prov, Model: modelID},
				}
			}
		}
	}

	return m, nil
}

// View implements [tea.Model].
func (m *AIDefaultModelModel) View() string {
	switch m.step {
	case aiModelStepProvider:
		title := m.styles.Subtitle.Render(locale.T("setup.ai.model.provider.label"))
		var rows []string
		for i, e := range m.entries {
			if i == m.provCursor {
				rows = append(rows, fmt.Sprintf("%s %s",
					m.styles.Cursor.Render(">"),
					m.styles.Selected.Render(e.DisplayName),
				))
			} else {
				rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(e.DisplayName)))
			}
		}
		hint := m.styles.Hint.Render(locale.T("setup.ai.model.hint"))
		parts := append([]string{title}, rows...)
		parts = append(parts, hint)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)

	case aiModelStepModel:
		title := m.styles.Subtitle.Render(locale.T("setup.ai.model.model.label"))
		models := m.modelsByProv[m.selProvider]
		var rows []string
		for i, mod := range models {
			label := mod.Name
			if label == "" {
				label = mod.ID
			}
			if i == m.modelCursor {
				rows = append(rows, fmt.Sprintf("%s %s",
					m.styles.Cursor.Render(">"),
					m.styles.Selected.Render(label),
				))
			} else {
				rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(label)))
			}
		}
		if len(models) == 0 {
			rows = append(rows, m.styles.Hint.Render(locale.T("setup.ai.model.no_models")))
		}
		var hintKey string
		if len(m.entries) > 1 {
			hintKey = "setup.ai.model.esc_back"
		} else {
			hintKey = "setup.ai.model.hint"
		}
		hint := m.styles.Hint.Render(locale.T(hintKey))
		parts := append([]string{title}, rows...)
		parts = append(parts, hint)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	return ""
}
