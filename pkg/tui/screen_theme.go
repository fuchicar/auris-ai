package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
)

// themeOptions lists the available themes in display order.
// Index 0 = Light, Index 1 = Dark — must match previews slice order.
var themeOptions = []struct {
	key      Theme
	labelKey string
}{
	{ThemeLight, "setup.theme.light"},
	{ThemeDark, "setup.theme.dark"},
}

// ThemeModel lets the user choose between the light and dark themes.
// A live preview panel updates immediately as the cursor moves so the user
// can see the visual difference before confirming.
type ThemeModel struct {
	cursor   int
	previews [2]*Styles // one pre-built Styles per theme option
	styles   *Styles    // current UI style set (for the screen chrome itself)
}

// newThemeModel constructs a [ThemeModel]. Both theme previews are built once
// at construction time so cursor movement has no allocation cost.
func newThemeModel(s *Styles) *ThemeModel {
	return &ThemeModel{
		cursor:   1, // default cursor on Dark
		previews: [2]*Styles{NewStyles(ThemeLight), NewStyles(ThemeDark)},
		styles:   s,
	}
}

// Init implements [tea.Model].
func (m *ThemeModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model]. Arrow keys move the cursor; Enter confirms.
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
// It shows representative elements so the user can judge contrast and colours.
func renderPreview(s *Styles) string {
	title := s.Title.Render("Auris AI")
	selected := fmt.Sprintf("%s %s", s.Cursor.Render(">"), s.Selected.Render("Selected option"))
	unselected := fmt.Sprintf("  %s", s.Unselected.Render("Unselected option"))
	checkbox := fmt.Sprintf("  %s %s", s.Checkbox.Render("[x]"), s.Unselected.Render("Checked item"))
	errLine := s.Error.Render("✗ Error message example")
	hintLine := s.Hint.Render("↑↓ navigate · Enter select")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		selected,
		unselected,
		checkbox,
		errLine,
		hintLine,
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
	hint := m.styles.Hint.Render(locale.T("setup.theme.hint"))

	parts := []string{label}
	parts = append(parts, rows...)
	parts = append(parts, "", preview, hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
