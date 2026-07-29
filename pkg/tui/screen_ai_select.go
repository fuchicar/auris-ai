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

// Init implements [tea.Model].
func (m *AIProviderSelectModel) Init() tea.Cmd { return nil }

// maxVisible returns the number of provider rows that fit in the terminal
// after the title, wrapped explain text, blank separators, hint, and the
// worst-case two scroll indicators are accounted for. The explain paragraph
// is rendered once and measured with lipgloss.Height to keep the budget
// accurate for both short and long translations (issue #35).
func (m *AIProviderSelectModel) maxVisible() int {
	if m.height == 0 {
		return 20
	}
	chrome := m.chromeLines()
	n := m.height - chrome
	if n < 0 {
		return 0
	}
	return n
}

// chromeLines measures the actual chrome above+below the AI-provider list:
// title (Subtitle content + MarginBottom), wrapped Help paragraph, blank
// separators, hint, and a 2-line reserve for both up + down scroll
// indicators. Measuring the rendered Help with lipgloss.Height keeps the
// budget correct even when the explain paragraph wraps to a different
// number of lines (en vs es translations, future copy edits, etc.).
func (m *AIProviderSelectModel) chromeLines() int {
	chrome := 0
	chrome += lipgloss.Height(m.styles.Subtitle.Render(locale.T("setup.ai.select.label")))
	chrome += lipgloss.Height(m.styles.Help.Render(locale.T("setup.ai.select.explain")))
	// Blank row above the list, blank row above the hint, hint line itself.
	chrome += 3
	if m.canGoBack {
		// CanGoBack adds the "Esc back" suffix to the hint — still one line.
		// No extra rows; ensure hint line is at least one.
		_ = locale.T("hint.esc_back")
	}
	// Reserve 2 lines for both up + down scroll indicators at once.
	chrome += 2
	return chrome
}

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
	maxVis := m.maxVisible()
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, maxVis)
		}
	case tea.KeyDown:
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, maxVis)
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

// renderRow returns the visible provider line for index i. Pure rendering —
// no chrome/header concern here, just the row.
func (m *AIProviderSelectModel) renderRow(i int) string {
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
	return fmt.Sprintf("%s %s %s", cursor, checkbox, label)
}

// View implements [tea.Model].
func (m *AIProviderSelectModel) View() string {
	title := m.styles.Subtitle.Render(locale.T("setup.ai.select.label"))
	explain := m.styles.Help.Render(locale.T("setup.ai.select.explain"))

	// Chrome bookkeeping has to use the same numbers [maxVisible] computed
	// from, so we re-measure here to keep them in sync.
	chromeAbove := lipgloss.Height(title) + lipgloss.Height(explain) + 1 // blank
	chromeBelow := 1 + 1 + 2                                            // blank + hint + 2 indicator lines reserved
	if m.canGoBack {
		chromeBelow = 1 + 1 + 2 // canGoBack only changes hint copy, not rows
	}

	rows := windowedRows(WindowedRowOpts{
		Height:           m.height,
		ScrollOff:        m.scrollOff,
		Cursor:           m.cursor,
		Total:            len(m.entries),
		ChromeAbove:      chromeAbove,
		ChromeBelow:      chromeBelow,
		IndicatorReserve: 2,
		RenderRow:        m.renderRow,
		HintRender:       func(s string) string { return m.styles.Hint.Render(s) },
	})

	hintText := locale.T("setup.ai.select.hint")
	if m.canGoBack {
		hintText += "  " + locale.T("hint.esc_back")
	}
	hint := m.styles.Hint.Render(hintText)
	parts := []string{title, explain, ""}
	parts = append(parts, rows...)
	parts = append(parts, "", hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
