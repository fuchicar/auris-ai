package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
)

// themeOptions lists the available themes in display order.
var themeOptions = []struct {
	key      Theme
	labelKey string
}{
	{ThemeLight,      "setup.theme.light"},
	{ThemeDark,       "setup.theme.dark"},
	{ThemeGreenLight, "setup.theme.greenlight"},
	{ThemeGreenDark,  "setup.theme.greendark"},
	{ThemeBoxLight,   "setup.theme.boxlight"},
	{ThemeBoxDark,    "setup.theme.boxdark"},
}

// ThemeModel lets the user choose a display theme.
// A live preview panel updates immediately as the cursor moves so the user
// can see the visual difference before confirming.
type ThemeModel struct {
	cursor     int
	previews   []*Styles // one pre-built Styles per theme option
	styles     *Styles   // current UI style set (for the screen chrome itself)
	canGoBack  bool
}

// newThemeModel constructs a [ThemeModel]. All theme previews are built once
// at construction time so cursor movement has no allocation cost.
func newThemeModel(s *Styles, canGoBack bool) *ThemeModel {
	previews := make([]*Styles, len(themeOptions))
	for i, opt := range themeOptions {
		previews[i] = NewStyles(opt.key)
	}
	return &ThemeModel{
		cursor:    1, // default cursor on Dark
		previews:  previews,
		styles:    s,
		canGoBack: canGoBack,
	}
}

// Init implements [tea.Model].
func (m *ThemeModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model]. Arrow keys move the cursor; Enter confirms; Esc goes back.
func (m *ThemeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(themeOptions)-1 {
				m.cursor++
			}
		case tea.KeyEsc:
			if m.canGoBack {
				return m, func() tea.Msg {
					return ScreenDoneMsg{From: ScreenTheme, Result: nil}
				}
			}
		case tea.KeyEnter:
			chosen := string(themeOptions[m.cursor].key)
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenTheme, Result: ThemeResult{Theme: chosen}}
			}
		}
	}
	return m, nil
}

// renderPreview builds a bordered sample box rendered with the given Styles.
// It shows representative elements so the user can judge contrast and colours,
// including a sample agent message to preview the chat appearance.
func renderPreview(s *Styles) string {
	title := s.Title.Render("Auris AI")
	selected := fmt.Sprintf("%s %s", s.Cursor.Render(">"), s.Selected.Render("Selected option"))
	unselected := fmt.Sprintf("  %s", s.Unselected.Render("Unselected option"))
	hintLine := s.Hint.Render("↑↓ navigate · Enter select")

	previewW := PanelWidth - 4 // account for Preview border + padding
	youLine := lipgloss.NewStyle().Width(previewW).Render(
		s.Selected.Render("You: ") + "What is the P/E ratio of AAPL?",
	)
	agentBlock := RenderAgentBlock(s, "Apple's P/E ratio is ~28.5x, above\nthe sector average of ~25x.", previewW)

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		selected,
		unselected,
		hintLine,
		"",
		youLine,
		agentBlock,
	)
	return s.Preview.Render(content)
}

// View implements [tea.Model].
func (m *ThemeModel) View() string {
	label := m.styles.Subtitle.Render(locale.T("setup.theme.label"))

	var rows []string
	for i, opt := range themeOptions {
		name := locale.T(opt.labelKey)
		var row string
		if i == m.cursor {
			row = fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(name))
		} else {
			row = fmt.Sprintf("  %s", m.styles.Unselected.Render(name))
		}
		rows = append(rows, row)
	}

	preview := renderPreview(m.previews[m.cursor])
	hintText := locale.T("setup.theme.hint")
	if m.canGoBack {
		hintText += "  " + locale.T("hint.esc_back")
	}
	hint := m.styles.Hint.Render(hintText)

	parts := []string{label}
	parts = append(parts, rows...)
	parts = append(parts, "", preview, hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
