package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/registry"
)

// MarketProviderManageModel lets the user choose which market data providers
// are active and in what priority order the fallback cascade (see
// agent.NewMarketChain) should try them. Row position IS priority — the top
// row is primary. Unlike AIProviderSelectModel (checkbox-only), this screen
// also supports reordering since market providers form a cascade, not a
// single "active" choice.
type MarketProviderManageModel struct {
	entries   []registry.MarketEntry // current order; mutated in place by +/-
	selected  map[string]bool        // keyed by Entry.Key, independent of position
	cursor    int
	scrollOff int
	height    int // terminal height (updated by WindowSizeMsg)
	styles    *Styles
	canGoBack bool
}

// newMarketProviderManageModel constructs a [MarketProviderManageModel].
// order is the starting priority order (typically AppModel.marketProviderOrder());
// preSelected marks which provider keys are currently configured/active.
func newMarketProviderManageModel(s *Styles, order []registry.MarketEntry, preSelected map[string]bool, canGoBack bool) *MarketProviderManageModel {
	entries := make([]registry.MarketEntry, len(order))
	copy(entries, order)
	sel := make(map[string]bool, len(preSelected))
	for k, v := range preSelected {
		sel[k] = v
	}
	return &MarketProviderManageModel{
		entries:   entries,
		selected:  sel,
		styles:    s,
		canGoBack: canGoBack,
	}
}

// Init implements [tea.Model].
func (m *MarketProviderManageModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model]. Up/Down move the cursor; Space toggles
// active/inactive; +/- reorder the entry under the cursor; Enter confirms;
// Esc cancels (if canGoBack).
func (m *MarketProviderManageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if len(m.entries) > 0 {
			k := m.entries[m.cursor].Key
			m.selected[k] = !m.selected[k]
		}
	case tea.KeyEsc:
		if m.canGoBack {
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenMarketProviderManage, Result: nil}
			}
		}
	case tea.KeyEnter:
		order := make([]string, len(m.entries))
		var selected []string
		for i, e := range m.entries {
			order[i] = e.Key
			if m.selected[e.Key] {
				selected = append(selected, e.Key)
			}
		}
		return m, func() tea.Msg {
			return ScreenDoneMsg{
				From:   ScreenMarketProviderManage,
				Result: MarketProviderManageResult{Order: order, Selected: selected},
			}
		}
	case tea.KeyRunes:
		switch key.String() {
		case "+", "=":
			m.moveUp()
		case "-":
			m.moveDown()
		}
	}
	return m, nil
}

// moveUp swaps the entry under the cursor with its predecessor, raising its priority.
func (m *MarketProviderManageModel) moveUp() {
	if m.cursor <= 0 {
		return
	}
	m.entries[m.cursor-1], m.entries[m.cursor] = m.entries[m.cursor], m.entries[m.cursor-1]
	m.cursor--
}

// moveDown swaps the entry under the cursor with its successor, lowering its priority.
func (m *MarketProviderManageModel) moveDown() {
	if m.cursor >= len(m.entries)-1 {
		return
	}
	m.entries[m.cursor+1], m.entries[m.cursor] = m.entries[m.cursor], m.entries[m.cursor+1]
	m.cursor++
}

// maxVisible returns the number of provider rows that fit in the terminal
// after the title, separator, hint, and worst-case two scroll indicators are
// accounted for.
func (m *MarketProviderManageModel) maxVisible() int {
	if m.height == 0 {
		return 20
	}
	// chrome above: Subtitle (2 — content + MarginBottom(1)). chrome below:
	// blank (1) + hint (1) = 2.
	n := m.height - 2 - 2 - 2 // 2 chrome-above + 2 chrome-below + 2 indicator reserve
	if n < 0 {
		return 0
	}
	return n
}

// View implements [tea.Model].
func (m *MarketProviderManageModel) View() string {
	title := m.styles.Subtitle.Render(locale.T("setup.market.manage.label"))

	rows := windowedRows(WindowedRowOpts{
		Height:           m.height,
		ScrollOff:        m.scrollOff,
		Cursor:           m.cursor,
		Total:            len(m.entries),
		ChromeAbove:      2, // Subtitle (content + MarginBottom)
		ChromeBelow:      2, // blank + hint
		IndicatorReserve: 2,
		RenderRow:        m.renderRow,
		HintRender:       func(s string) string { return m.styles.Hint.Render(s) },
	})

	hintText := locale.T("setup.market.manage.hint")
	if m.canGoBack {
		hintText += "  " + locale.T("hint.esc_back")
	}
	hint := m.styles.Hint.Render(hintText)
	parts := append([]string{title}, rows...)
	parts = append(parts, "", hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderRow returns the visible provider line for index i. Pure rendering —
// no chrome/header concern here, just the row.
func (m *MarketProviderManageModel) renderRow(i int) string {
	e := m.entries[i]
	box := "[ ]"
	if m.selected[e.Key] {
		box = "[x]"
	}
	checkbox := m.styles.Checkbox.Render(box)
	cursor := "  "
	label := m.styles.Unselected.Render(e.DisplayName)
	if i == m.cursor {
		cursor = m.styles.Cursor.Render(">")
		label = m.styles.Selected.Render(e.DisplayName)
	}
	return fmt.Sprintf("%s %d. %s %s", cursor, i+1, checkbox, label)
}
