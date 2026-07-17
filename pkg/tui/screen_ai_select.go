package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/registry"
)

// AIProviderSelectModel lets the user choose which AI providers to configure.
// Any number of providers may be selected (including none, to skip AI setup).
// Checking "OpenAI-Compatible" always adds a brand new named instance rather
// than editing an existing one — see extraEntries below.
type AIProviderSelectModel struct {
	entries   []registry.LLMEntry
	selected  map[int]bool
	cursor    int
	scrollOff int
	height    int // terminal height (updated by WindowSizeMsg)
	styles    *Styles
	canGoBack bool
}

// newAIProviderSelectModel constructs an [AIProviderSelectModel] with all
// registered LLM providers pre-loaded, plus extraEntries — dynamically named
// provider instances already present in config (e.g. multiple
// OpenAI-Compatible endpoints, each keyed "openai_compatible:<slug>" with its
// own DisplayName). preSelected is a set of provider keys that should be
// checked by default (e.g. already-configured providers).
func newAIProviderSelectModel(s *Styles, preSelected map[string]bool, extraEntries []registry.LLMEntry, canGoBack bool) *AIProviderSelectModel {
	entries := append(append([]registry.LLMEntry{}, registry.AllLLM()...), extraEntries...)
	sel := make(map[int]bool, len(preSelected))
	for i, e := range entries {
		if preSelected[e.Key] {
			sel[i] = true
		}
	}
	return &AIProviderSelectModel{
		entries:   entries,
		selected:  sel,
		styles:    s,
		canGoBack: canGoBack,
	}
}

// maxVisible returns the number of list items that fit in the terminal.
func (m *AIProviderSelectModel) maxVisible() int {
	if m.height == 0 {
		return 20
	}
	n := m.height - 5 // overhead: title + explain + hint + scroll indicators + margin
	if n < 3 {
		return 3
	}
	return n
}

// Init implements [tea.Model].
func (m *AIProviderSelectModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model]. Space toggles selection; Enter confirms; Esc goes back.
func (m *AIProviderSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = ws.Height
		return m, nil
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
		}
	case tea.KeyDown:
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
		}
	case tea.KeySpace:
		m.selected[m.cursor] = !m.selected[m.cursor]
	case tea.KeyEsc:
		if m.canGoBack {
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenAIProviderSelect, Result: nil}
			}
		}
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
	explain := m.styles.Help.Render(locale.T("setup.ai.select.explain"))

	maxVis := m.maxVisible()
	end := m.scrollOff + maxVis
	if end > len(m.entries) {
		end = len(m.entries)
	}

	var rows []string
	if m.scrollOff > 0 {
		rows = append(rows, m.styles.Hint.Render(locale.T("setup.ai.model.scroll_up")))
	}
	for i := m.scrollOff; i < end; i++ {
		e := m.entries[i]
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
	if end < len(m.entries) {
		rows = append(rows, m.styles.Hint.Render(locale.T("setup.ai.model.scroll_down")))
	}

	hintText := locale.T("setup.ai.select.hint")
	if m.canGoBack {
		hintText += "  " + locale.T("hint.esc_back")
	}
	hint := m.styles.Hint.Render(hintText)
	parts := append([]string{title, explain, ""}, rows...)
	parts = append(parts, "", hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
