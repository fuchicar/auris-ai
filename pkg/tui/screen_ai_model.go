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

// chromeLines measures the chrome rendered above+below both lists in this
// screen. Both sub-steps share the same shape (Subtitle + Help paragraph +
// blank + blank + hint) so they get the same budget. The Help text changes
// slightly per step but is roughly the same height; measure per step via
// maxVisible to avoid surprises.
func (m *AIDefaultModelModel) maxVisible() int {
	if m.height == 0 {
		return 20
	}
	var title, explain string
	if m.step == aiModelStepProvider {
		title = m.styles.Subtitle.Render(locale.T("setup.ai.model.provider.label"))
		explain = m.styles.Help.Render(locale.T("setup.ai.model.provider.explain"))
	} else {
		title = m.styles.Subtitle.Render(locale.T("setup.ai.model.model.label"))
		explain = m.styles.Help.Render(locale.T("setup.ai.model.model.explain"))
	}
	// Title + Explain + blank above + blank below + hint + 2 indicator reserve.
	chrome := lipgloss.Height(title) + lipgloss.Height(explain) + 1 + 1 + 1 + 2
	n := m.height - chrome
	if n < 0 {
		return 0
	}
	return n
}

// chromeAbove returns the height of the chrome above the row list at the
// current step. Kept in sync with maxVisible for the visibleCount fallback.
func (m *AIDefaultModelModel) chromeAbove() int {
	var title, explain string
	if m.step == aiModelStepProvider {
		title = m.styles.Subtitle.Render(locale.T("setup.ai.model.provider.label"))
		explain = m.styles.Help.Render(locale.T("setup.ai.model.provider.explain"))
	} else {
		title = m.styles.Subtitle.Render(locale.T("setup.ai.model.model.label"))
		explain = m.styles.Help.Render(locale.T("setup.ai.model.model.explain"))
	}
	return lipgloss.Height(title) + lipgloss.Height(explain) + 1
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

// View implements [tea.Model].
func (m *AIDefaultModelModel) View() string {
	switch m.step {
	case aiModelStepProvider:
		title := m.styles.Subtitle.Render(locale.T("setup.ai.model.provider.label"))
		explain := m.styles.Help.Render(locale.T("setup.ai.model.provider.explain"))
		labels := make([]string, len(m.entries))
		for i, e := range m.entries {
			labels[i] = e.DisplayName
		}
		rows := windowedRows(WindowedRowOpts{
			Height:           m.height,
			ScrollOff:        m.provScrollOff,
			Cursor:           m.provCursor,
			Total:            len(m.entries),
			ChromeAbove:      m.chromeAbove(),
			ChromeBelow:      1 + 1 + 2, // blank + hint + 2 indicator reserve
			IndicatorReserve: 2,
			RenderRow:        func(i int) string { return m.renderRow(labels, i) },
			HintRender:       func(s string) string { return m.styles.Hint.Render(s) },
		})
		hintText := locale.T("setup.ai.model.hint")
		if m.canGoBack {
			hintText += "  " + locale.T("hint.esc_back")
		}
		hint := m.styles.Hint.Render(hintText)
		parts := []string{title, explain, ""}
		parts = append(parts, rows...)
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
		explain := m.styles.Help.Render(locale.T("setup.ai.model.model.explain"))
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
			rows = windowedRows(WindowedRowOpts{
				Height:           m.height,
				ScrollOff:        m.modelScrollOff,
				Cursor:           m.modelCursor,
				Total:            len(models),
				ChromeAbove:      m.chromeAbove(),
				ChromeBelow:      1 + 1 + 2, // blank + hint + 2 indicator reserve
				IndicatorReserve: 2,
				RenderRow:        func(i int) string { return m.renderRow(labels, i) },
				HintRender:       func(s string) string { return m.styles.Hint.Render(s) },
			})
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
		parts := []string{title, explain, ""}
		parts = append(parts, rows...)
		parts = append(parts, hint)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	return ""
}

// renderRow returns the visible line for the row at index i, using the
// labels slice for the option text. Pure rendering — no chrome concern.
func (m *AIDefaultModelModel) renderRow(labels []string, i int) string {
	if i == m.currentForStep() {
		return fmt.Sprintf("%s %s",
			m.styles.Cursor.Render(">"),
			m.styles.Selected.Render(labels[i]),
		)
	}
	return fmt.Sprintf("  %s", m.styles.Unselected.Render(labels[i]))
}

// currentForStep returns whichever cursor the current step renders, so
// renderRow can highlight the right index without needing step context.
func (m *AIDefaultModelModel) currentForStep() int {
	if m.step == aiModelStepProvider {
		return m.provCursor
	}
	return m.modelCursor
}
