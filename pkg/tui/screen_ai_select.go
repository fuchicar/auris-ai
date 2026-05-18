package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
	"auris/pkg/registry"
)

// AIProviderSelectModel lets the user choose which AI providers to configure.
// Any number of providers may be selected (including none, to skip AI setup).
type AIProviderSelectModel struct {
	entries  []registry.LLMEntry
	selected map[int]bool
	cursor   int
	styles   *Styles
}

// newAIProviderSelectModel constructs an [AIProviderSelectModel] with all
// registered LLM providers pre-loaded.
func newAIProviderSelectModel(s *Styles) *AIProviderSelectModel {
	return &AIProviderSelectModel{
		entries:  registry.AllLLM(),
		selected: make(map[int]bool),
		styles:   s,
	}
}

// Init implements [tea.Model].
func (m *AIProviderSelectModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model]. Space toggles selection; Enter confirms.
func (m *AIProviderSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(m.entries)-1 {
			m.cursor++
		}
	case tea.KeySpace:
		m.selected[m.cursor] = !m.selected[m.cursor]
	case tea.KeyEnter:
		var keys []string
		for i, e := range m.entries {
			if m.selected[i] {
				keys = append(keys, e.Key)
			}
		}
		return m, func() tea.Msg {
			return ScreenDoneMsg{
				From:   ScreenAIProviderSelect,
				Result: AIProviderSelectResult{Keys: keys},
			}
		}
	}
	return m, nil
}

// View implements [tea.Model].
func (m *AIProviderSelectModel) View() string {
	title := m.styles.Subtitle.Render(locale.T("setup.ai.select.label"))

	var rows []string
	for i, e := range m.entries {
		box := "[ ]"
		if m.selected[i] {
			box = "[x]"
		}
		checkbox := m.styles.Checkbox.Render(box)
		cursor := "  "
		if i == m.cursor {
			cursor = m.styles.Cursor.Render(">")
		}
		label := m.styles.Unselected.Render(e.DisplayName)
		if i == m.cursor {
			label = m.styles.Selected.Render(e.DisplayName)
		}
		rows = append(rows, fmt.Sprintf("%s %s %s", cursor, checkbox, label))
	}

	hint := m.styles.Hint.Render(locale.T("setup.ai.select.hint"))
	parts := append([]string{title}, rows...)
	parts = append(parts, "", hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
