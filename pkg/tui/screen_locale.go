package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
)

// localeOptions lists the available languages in display order.
// Add new entries here to extend language support without touching LocaleModel.
var localeOptions = []struct {
	tag      string
	labelKey string
}{
	{"en", "setup.locale.english"},
	{"es", "setup.locale.spanish"},
}

// LocaleModel lets the user explicitly choose the application language.
// It is shown during first-run setup when automatic detection is uncertain,
// and can also be reached from the main menu via the /language command.
type LocaleModel struct {
	cursor    int
	styles    *Styles
	canGoBack bool
}

// newLocaleModel constructs a [LocaleModel].
func newLocaleModel(s *Styles, canGoBack bool) *LocaleModel {
	return &LocaleModel{styles: s, canGoBack: canGoBack}
}

// Init implements [tea.Model].
func (m *LocaleModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model]. Arrow keys navigate; Enter confirms; Esc goes back.
func (m *LocaleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(localeOptions)-1 {
				m.cursor++
			}
		case tea.KeyEsc:
			if m.canGoBack {
				return m, func() tea.Msg {
					return ScreenDoneMsg{From: ScreenLocale, Result: nil}
				}
			}
		case tea.KeyEnter:
			chosen := localeOptions[m.cursor].tag
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenLocale, Result: LocaleResult{Locale: chosen}}
			}
		}
	}
	return m, nil
}

// View implements [tea.Model].
func (m *LocaleModel) View() string {
	label := m.styles.Subtitle.Render(locale.T("setup.locale.label"))

	var rows []string
	for i, opt := range localeOptions {
		name := locale.T(opt.labelKey)
		var row string
		if i == m.cursor {
			row = fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(name))
		} else {
			row = fmt.Sprintf("  %s", m.styles.Unselected.Render(name))
		}
		rows = append(rows, row)
	}

	hintText := locale.T("setup.locale.hint")
	if m.canGoBack {
		hintText += "  " + locale.T("hint.esc_back")
	}
	hint := m.styles.Hint.Render(hintText)
	parts := append([]string{label}, rows...)
	parts = append(parts, hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
