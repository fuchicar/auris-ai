package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/config"
	"auris/pkg/locale"
)

// sessionFixedHeight is the number of terminal lines occupied by the session
// screen chrome (title, separator, hint row).
const sessionFixedHeight = 3

// SessionSelectModel is a full-terminal session picker. The user navigates the
// session list with Up/Down, selects with Enter, and deletes with Delete (with
// an inline y/N confirmation prompt).
type SessionSelectModel struct {
	sessions  []*config.Session
	cursor    int
	currentID string // ID of the session that is currently active in the agent
	styles    *Styles
	width     int
	height    int

	// confirming is set when the user pressed Delete; the next key confirms or
	// cancels the deletion.
	confirming bool
}

func newSessionSelectModel(sessions []*config.Session, currentID string, s *Styles, width, height int) *SessionSelectModel {
	return &SessionSelectModel{
		sessions:  sessions,
		currentID: currentID,
		styles:    s,
		width:     width,
		height:    height,
	}
}

// Init implements [tea.Model].
func (m *SessionSelectModel) Init() tea.Cmd { return nil }

// Update implements [tea.Model].
func (m *SessionSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *SessionSelectModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirming {
		switch msg.String() {
		case "y", "Y", "s", "S":
			return m.deleteSelected()
		default:
			m.confirming = false
		}
		return m, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg { return ScreenDoneMsg{From: ScreenSessionSelect, Result: nil} }

	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case tea.KeyDown:
		if m.cursor < len(m.sessions)-1 {
			m.cursor++
		}
		return m, nil

	case tea.KeyEnter:
		if len(m.sessions) == 0 {
			return m, func() tea.Msg { return ScreenDoneMsg{From: ScreenSessionSelect, Result: nil} }
		}
		id := m.sessions[m.cursor].ID
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenSessionSelect, Result: SessionSelectResult{ID: id}}
		}

	case tea.KeyDelete:
		if len(m.sessions) > 0 {
			m.confirming = true
		}
		return m, nil
	}

	return m, nil
}

func (m *SessionSelectModel) deleteSelected() (tea.Model, tea.Cmd) {
	m.confirming = false
	if len(m.sessions) == 0 {
		return m, nil
	}

	toDelete := m.sessions[m.cursor]
	_ = config.DeleteSession(toDelete.ID)

	// Rebuild session list.
	updated := make([]*config.Session, 0, len(m.sessions)-1)
	for _, s := range m.sessions {
		if s.ID != toDelete.ID {
			updated = append(updated, s)
		}
	}
	m.sessions = updated

	if m.cursor >= len(m.sessions) && m.cursor > 0 {
		m.cursor--
	}

	// If we deleted the active session, switch to the next available one.
	if toDelete.ID == m.currentID && len(m.sessions) > 0 {
		m.currentID = m.sessions[0].ID
	}

	return m, nil
}

// View implements [tea.Model].
func (m *SessionSelectModel) View() string {
	title := m.styles.Selected.Render(locale.T("session.title"))
	sep := strings.Repeat("─", m.width)

	var body string
	if len(m.sessions) == 0 {
		body = m.styles.Hint.Render(locale.T("session.empty"))
	} else {
		visibleRows := m.height - sessionFixedHeight
		if visibleRows < 1 {
			visibleRows = 1
		}

		// Determine the visible window around the cursor.
		start := m.cursor - visibleRows/2
		if start < 0 {
			start = 0
		}
		end := start + visibleRows
		if end > len(m.sessions) {
			end = len(m.sessions)
			start = end - visibleRows
			if start < 0 {
				start = 0
			}
		}

		lines := make([]string, 0, end-start)
		for i := start; i < end; i++ {
			s := m.sessions[i]
			label := fmt.Sprintf("%-16s  %s", s.UpdatedAt.Format("2006-01-02 15:04"), s.Title)
			if s.ID == m.currentID {
				label += "  " + m.styles.Hint.Render("(active)")
			}
			if i == m.cursor {
				lines = append(lines, m.styles.Cursor.Render(">")+" "+m.styles.Selected.Render(label))
			} else {
				lines = append(lines, "  "+label)
			}
		}
		body = strings.Join(lines, "\n")
	}

	var hint string
	if m.confirming {
		hint = m.styles.Error.Render(locale.T("session.delete_confirm"))
	} else {
		hint = m.styles.Hint.Render(locale.T("session.hint"))
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		sep,
		body,
		hint,
	)
}
