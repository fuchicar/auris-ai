package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/llm"
	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/registry"
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
	entries        []registry.LLMEntry    // configured providers in order
	modelsByProv   map[string][]llm.Model // models per provider key
	step           aiModelStep
	provCursor     int
	provScrollOff  int // first visible provider index
	modelCursor    int
	modelScrollOff int // first visible model index
	selProvider    string
	height         int // terminal height (updated by WindowSizeMsg)
	styles         *Styles
	canGoBack      bool
}

// newAIDefaultModelModel constructs an [AIDefaultModelModel].
// entries contains only the providers that were successfully configured.
// modelsByProv maps each provider key to its discovered model list.
func newAIDefaultModelModel(entries []registry.LLMEntry, modelsByProv map[string][]llm.Model, s *Styles, canGoBack bool) *AIDefaultModelModel {
	m := &AIDefaultModelModel{
		entries:      entries,
		modelsByProv: modelsByProv,
		styles:       s,
		canGoBack:    canGoBack,
	}
	// If there is only one provider, skip directly to model selection.
	if len(entries) == 1 {
		m.selProvider = entries[0].Key
		m.step = aiModelStepModel
	}
	return m
}

// maxVisible returns the number of list items that fit in the terminal.
func (m *AIDefaultModelModel) maxVisible() int {
	if m.height == 0 {
		return 20
	}
	n := m.height - 5 // overhead: title + hint + scroll indicators + margin
	if n < 3 {
		return 3
	}
	return n
}

// clampScrollOff adjusts scrollOff so that cursor remains in the visible window.
func clampScrollOff(cursor, scrollOff, maxVis int) int {
	if cursor < scrollOff {
		return cursor
	}
	if cursor >= scrollOff+maxVis {
		return cursor - maxVis + 1
	}
	return scrollOff
}

// Init implements [tea.Model].
func (m *AIDefaultModelModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model].
func (m *AIDefaultModelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = ws.Height
		return m, nil
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	maxVis := m.maxVisible()

	switch m.step {
	case aiModelStepProvider:
		switch key.Type {
		case tea.KeyUp:
			if m.provCursor > 0 {
				m.provCursor--
				m.provScrollOff = clampScrollOff(m.provCursor, m.provScrollOff, maxVis)
			}
		case tea.KeyDown:
			if m.provCursor < len(m.entries)-1 {
				m.provCursor++
				m.provScrollOff = clampScrollOff(m.provCursor, m.provScrollOff, maxVis)
			}
		case tea.KeyEsc:
			if m.canGoBack {
				return m, func() tea.Msg {
					return ScreenDoneMsg{From: ScreenAIDefaultModel, Result: nil}
				}
			}
		case tea.KeyEnter:
			m.selProvider = m.entries[m.provCursor].Key
			m.modelCursor = 0
			m.modelScrollOff = 0
			m.step = aiModelStepModel
		}

	case aiModelStepModel:
		models := m.modelsByProv[m.selProvider]
		switch key.Type {
		case tea.KeyUp:
			if m.modelCursor > 0 {
				m.modelCursor--
				m.modelScrollOff = clampScrollOff(m.modelCursor, m.modelScrollOff, maxVis)
			}
		case tea.KeyDown:
			if m.modelCursor < len(models)-1 {
				m.modelCursor++
				m.modelScrollOff = clampScrollOff(m.modelCursor, m.modelScrollOff, maxVis)
			}
		case tea.KeyEsc:
			if len(m.entries) > 1 {
				// Multiple providers: go back to provider selection step.
				m.step = aiModelStepProvider
			} else if m.canGoBack {
				// Single provider: escape goes back to the calling screen.
				return m, func() tea.Msg {
					return ScreenDoneMsg{From: ScreenAIDefaultModel, Result: nil}
				}
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

// renderScrollList renders a windowed list of items with optional scroll indicators.
func (m *AIDefaultModelModel) renderScrollList(items []string, cursor, scrollOff int) []string {
	maxVis := m.maxVisible()
	end := scrollOff + maxVis
	if end > len(items) {
		end = len(items)
	}

	var rows []string
	if scrollOff > 0 {
		rows = append(rows, m.styles.Hint.Render(locale.T("setup.ai.model.scroll_up")))
	}
	for i := scrollOff; i < end; i++ {
		if i == cursor {
			rows = append(rows, fmt.Sprintf("%s %s",
				m.styles.Cursor.Render(">"),
				m.styles.Selected.Render(items[i]),
			))
		} else {
			rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(items[i])))
		}
	}
	if end < len(items) {
		rows = append(rows, m.styles.Hint.Render(locale.T("setup.ai.model.scroll_down")))
	}
	return rows
}

// View implements [tea.Model].
func (m *AIDefaultModelModel) View() string {
	switch m.step {
	case aiModelStepProvider:
		title := m.styles.Subtitle.Render(locale.T("setup.ai.model.provider.label"))
		labels := make([]string, len(m.entries))
		for i, e := range m.entries {
			labels[i] = e.DisplayName
		}
		rows := m.renderScrollList(labels, m.provCursor, m.provScrollOff)
		hintText := locale.T("setup.ai.model.hint")
		if m.canGoBack {
			hintText += "  " + locale.T("hint.esc_back")
		}
		hint := m.styles.Hint.Render(hintText)
		parts := append([]string{title}, rows...)
		parts = append(parts, hint)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)

	case aiModelStepModel:
		var provName string
		for _, e := range m.entries {
			if e.Key == m.selProvider {
				provName = e.DisplayName
				break
			}
		}
		titleText := locale.T("setup.ai.model.model.label")
		if provName != "" {
			titleText = fmt.Sprintf("%s — %s", titleText, provName)
		}
		title := m.styles.Subtitle.Render(titleText)
		models := m.modelsByProv[m.selProvider]

		var rows []string
		if len(models) == 0 {
			rows = append(rows, m.styles.Hint.Render(locale.T("setup.ai.model.no_models")))
		} else {
			labels := make([]string, len(models))
			for i, mod := range models {
				if mod.Name != "" {
					labels[i] = mod.Name
				} else {
					labels[i] = mod.ID
				}
			}
			rows = m.renderScrollList(labels, m.modelCursor, m.modelScrollOff)
		}

		var hintText string
		if len(m.entries) > 1 {
			hintText = locale.T("setup.ai.model.esc_back")
		} else {
			hintText = locale.T("setup.ai.model.hint")
			if m.canGoBack {
				hintText += "  " + locale.T("hint.esc_back")
			}
		}
		hint := m.styles.Hint.Render(hintText)
		parts := append([]string{title}, rows...)
		parts = append(parts, hint)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	return ""
}
