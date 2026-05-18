package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
)

// menuItem represents a single navigable entry in the main menu.
type menuItem struct {
	labelKey string
	cmdName  string // slash command to emit when selected (empty = exit)
}

// menuItems lists the main menu options in display order.
var menuItems = []menuItem{
	{"menu.agent_mode", "agent"},
	{"menu.change_theme", "theme"},
	{"menu.change_language", "language"},
	{"menu.exit", ""},
}

// MenuModel renders the main menu. It supports two interaction modes:
//
//   - Nav mode: arrow keys move the cursor, Enter confirms the focused option.
//   - Command mode: activated by pressing "/"; a text input accepts slash
//     commands (e.g. "/theme dark", "/language es"). Press Esc to cancel.
type MenuModel struct {
	cursor         int
	commandMode    bool
	cmdInput       textinput.Model
	err            string
	styles         *Styles
	agentAvailable bool
}

// newMenuModel constructs a [MenuModel] in nav mode.
func newMenuModel(s *Styles, agentAvailable bool) *MenuModel {
	ti := textinput.New()
	ti.Placeholder = "/command [args]"
	return &MenuModel{styles: s, cmdInput: ti, agentAvailable: agentAvailable}
}

// Init implements [tea.Model].
func (m *MenuModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model].
func (m *MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.commandMode {
		return m.updateCommandMode(msg)
	}
	return m.updateNavMode(msg)
}

// updateNavMode handles keystrokes in the default navigation mode.
func (m *MenuModel) updateNavMode(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if m.cursor < len(menuItems)-1 {
			m.cursor++
		}
	case tea.KeyEnter:
		return m.selectItem(menuItems[m.cursor])
	default:
		// "/" activates command mode.
		if key.Type == tea.KeyRunes && key.String() == "/" {
			m.commandMode = true
			m.cmdInput.SetValue("/")
			m.cmdInput.Focus()
			// Move cursor to end of the pre-filled slash.
			m.cmdInput, _ = m.cmdInput.Update(nil)
			return m, textinput.Blink
		}
	}
	return m, nil
}

// updateCommandMode handles keystrokes while the command input is active.
func (m *MenuModel) updateCommandMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			m.commandMode = false
			m.cmdInput.SetValue("")
			m.cmdInput.Blur()
			m.err = ""
			return m, nil
		case tea.KeyEnter:
			cmd, ok := parseCommand(m.cmdInput.Value())
			if !ok {
				m.err = locale.T("menu.command.unknown")
				return m, nil
			}
			m.commandMode = false
			m.cmdInput.SetValue("")
			m.cmdInput.Blur()
			m.err = ""
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenMenu, Result: cmd}
			}
		}
	}
	updated, cmd := m.cmdInput.Update(msg)
	m.cmdInput = updated
	return m, cmd
}

// selectItem is called when the user confirms a menu item in nav mode.
func (m *MenuModel) selectItem(item menuItem) (tea.Model, tea.Cmd) {
	if item.cmdName == "" {
		// Exit item.
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenMenu, Result: nil}
		}
	}
	if item.cmdName == "agent" && !m.agentAvailable {
		m.err = locale.T("menu.agent_unavailable")
		return m, nil
	}
	return m, func() tea.Msg {
		return ScreenDoneMsg{From: ScreenMenu, Result: CommandResult{Cmd: item.cmdName}}
	}
}

// View implements [tea.Model].
func (m *MenuModel) View() string {
	title := m.styles.Title.Render(locale.T("menu.title"))

	var rows []string
	for i, item := range menuItems {
		label := locale.T(item.labelKey)
		var row string
		if i == m.cursor && !m.commandMode {
			row = fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(label))
		} else {
			row = fmt.Sprintf("  %s", m.styles.Unselected.Render(label))
		}
		rows = append(rows, row)
	}

	parts := []string{title}
	parts = append(parts, rows...)
	parts = append(parts, "")

	if m.commandMode {
		prompt := m.styles.Hint.Render(locale.T("menu.command.prompt"))
		inp := m.styles.Input.Render(m.cmdInput.View())
		hint := m.styles.Hint.Render(locale.T("menu.command.hint"))
		parts = append(parts, prompt, inp, hint)
	} else {
		if m.err != "" {
			parts = append(parts, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.err)))
		}
		parts = append(parts, m.styles.Hint.Render(locale.T("menu.hint")))
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
