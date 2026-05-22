package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
)

// menuItem represents a single navigable entry in the menu. If subItems is
// non-nil, selecting this item opens a submenu instead of emitting a command.
type menuItem struct {
	labelKey string
	cmdName  string     // "exit", "back", or a slash command name; empty for submenu parents
	subItems []menuItem // non-nil = entering this item opens a nested menu
}

var configMenuItems = []menuItem{
	{"menu.edit_profile", "profile", nil},
	{"menu.change_language", "language", nil},
	{"menu.change_theme", "theme", nil},
	{"menu.change_model", "model", nil},
	{"menu.back", "back", nil},
}

var mainMenuItems = []menuItem{
	{"menu.agent_mode", "agent", nil},
	{"menu.configuration", "", configMenuItems},
	{"menu.exit", "exit", nil},
}

// MenuModel renders the main menu. It supports two interaction modes:
//
//   - Nav mode: arrow keys move the cursor, Enter confirms the focused option.
//   - Command mode: activated by pressing "/"; a text input accepts slash
//     commands (e.g. "/theme dark", "/language es"). Press Esc to cancel.
type MenuModel struct {
	cursor         int
	items          []menuItem // currently visible items (main or submenu)
	parentItems    []menuItem // non-nil when inside a submenu
	titleKey       string     // locale key for the current screen title
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
	return &MenuModel{
		styles:         s,
		cmdInput:       ti,
		agentAvailable: agentAvailable,
		items:          mainMenuItems,
		titleKey:       "menu.title",
	}
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
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case tea.KeyEnter:
		return m.selectItem(m.items[m.cursor])
	case tea.KeyEsc:
		// Go back to the parent menu when inside a submenu.
		if m.parentItems != nil {
			m.items = m.parentItems
			m.parentItems = nil
			m.titleKey = "menu.title"
			m.cursor = 0
			m.err = ""
		}
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
	// Submenu: push current items and enter the nested list.
	if item.subItems != nil {
		m.parentItems = m.items
		m.items = item.subItems
		m.titleKey = item.labelKey
		m.cursor = 0
		m.err = ""
		return m, nil
	}
	// Back: return to the parent menu.
	if item.cmdName == "back" {
		m.items = m.parentItems
		m.parentItems = nil
		m.titleKey = "menu.title"
		m.cursor = 0
		m.err = ""
		return m, nil
	}
	// Exit.
	if item.cmdName == "exit" {
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
	title := m.styles.Title.Render(locale.T(m.titleKey))

	var rows []string
	for i, item := range m.items {
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
		hintKey := "menu.hint"
		if m.parentItems != nil {
			hintKey = "menu.hint_submenu"
		}
		parts = append(parts, m.styles.Hint.Render(locale.T(hintKey)))
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
