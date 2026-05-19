package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/config"
	"auris/pkg/llm"
	"auris/pkg/locale"
)

// agentTitleMsg carries an AI-generated session title back to the agent model.
type agentTitleMsg struct{ title string }

// agentState tracks the lifecycle of the agent screen.
type agentState int

const (
	agentStateConnecting agentState = iota
	agentStateReady
	agentStateError
)

// agentFixedHeight is the number of terminal lines occupied by chrome
// (title, two separators, input row, hint). The viewport fills the rest.
const agentFixedHeight = 5

// maxPaletteLines is the maximum number of command palette items visible at once.
const maxPaletteLines = 5

// agentCmd describes a slash command available inside the agent screen.
type agentCmd struct {
	name    string // e.g. "theme"
	display string // e.g. "/theme [light|dark]"
	descKey string // i18n key for the one-line description
	noArgs  bool   // if true, execute immediately when selected from the palette
}

// agentCommands lists the slash commands available in agent mode.
// /agent is excluded — it makes no sense from inside the agent screen.
var agentCommands = []agentCmd{
	{"menu", "/menu", "agent.cmd.menu", true},
	{"new", "/new", "agent.cmd.new", true},
	{"session", "/session", "agent.cmd.session", true},
	{"theme", "/theme [light|dark]", "agent.cmd.theme", false},
	{"language", "/language [en|es]", "agent.cmd.language", false},
	{"exit", "/exit", "agent.cmd.exit", true},
}

// agentConnectResultMsg carries the outcome of the provider Connect() call.
type agentConnectResultMsg struct{ err error }

// streamStartMsg carries the channel returned by AIProvider.Stream() so it
// can be stored on the model without a data race.
type streamStartMsg struct{ ch <-chan llm.StreamChunk }

// streamChunkMsg wraps a single frame from the streaming channel.
type streamChunkMsg struct{ chunk llm.StreamChunk }

// activeSessionChangedMsg notifies AppModel that the active session ID changed
// so it can persist the updated config.
type activeSessionChangedMsg struct{ id string }

// AgentModel is the chat UI screen. It streams responses from the configured
// LLM provider and persists the conversation as a [config.Session] file after
// each completed exchange.
type AgentModel struct {
	state     agentState
	session   *config.Session
	viewport  viewport.Model
	input     textinput.Model
	spin      spinner.Model
	messages  []llm.Message // multi-turn context sent to the LLM
	provider  llm.AIProvider
	streaming bool
	streambuf strings.Builder
	streamCh  <-chan llm.StreamChunk
	modelID   string
	styles    *Styles
	err       string
	ready     bool // true once the viewport has been sized
	width     int
	height    int

	// Command palette state.
	showCmdPalette bool
	cmdCursor      int
	cmdOffset      int
	cmdMatches     []agentCmd
}

// newAgentModel constructs an [AgentModel]. session provides the persisted chat
// log and metadata; modelID selects which model to use for completions.
// width and height are the current terminal dimensions; passing them allows the
// viewport to be initialised immediately without waiting for a WindowSizeMsg.
func newAgentModel(provider llm.AIProvider, session *config.Session, modelID string, s *Styles, width, height int) *AgentModel {
	ti := textinput.New()
	ti.Placeholder = locale.T("agent.placeholder")
	ti.Prompt = "" // the ">" prefix is rendered manually in View()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = s.Spinner

	// Reconstruct the LLM message context from the persisted history.
	messages := make([]llm.Message, 0, len(session.History))
	for _, turn := range session.History {
		messages = append(messages, llm.Message{
			Role:    llm.Role(turn.Role),
			Content: turn.Content,
		})
	}

	m := &AgentModel{
		state:    agentStateConnecting,
		session:  session,
		input:    ti,
		spin:     sp,
		messages: messages,
		provider: provider,
		modelID:  modelID,
		styles:   s,
		width:    width,
		height:   height,
	}

	// Initialise the viewport now so it is ready as soon as the connection
	// succeeds. BubbleTea only sends WindowSizeMsg once (at startup), so new
	// screens created later must seed their own dimensions.
	if width > 0 && height > 0 {
		vpHeight := height - agentFixedHeight
		if vpHeight < 1 {
			vpHeight = 1
		}
		m.viewport = viewport.New(width, vpHeight)
		m.viewport.SetContent(m.renderHistory())
		m.ready = true
	}

	return m
}

// Init implements [tea.Model]. Kicks off the provider connection.
func (m *AgentModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, agentConnectCmd(m.provider))
}

func agentConnectCmd(provider llm.AIProvider) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return agentConnectResultMsg{err: provider.Connect(ctx)}
	}
}

func startStreamCmd(provider llm.AIProvider, modelID string, messages []llm.Message) tea.Cmd {
	return func() tea.Msg {
		ch, err := provider.Stream(context.Background(), llm.CompletionRequest{
			Model:    modelID,
			Messages: messages,
		})
		if err != nil {
			return streamChunkMsg{chunk: llm.StreamChunk{Done: true, Err: err}}
		}
		return streamStartMsg{ch: ch}
	}
}

func waitForChunk(ch <-chan llm.StreamChunk) tea.Cmd {
	return func() tea.Msg { return streamChunkMsg{chunk: <-ch} }
}

// Update implements [tea.Model].
func (m *AgentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			vpHeight := m.height - agentFixedHeight - m.paletteHeight()
			if vpHeight < 1 {
				vpHeight = 1
			}
			m.viewport = viewport.New(m.width, vpHeight)
			m.viewport.SetContent(m.renderHistory())
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.updateViewportHeight()
			m.viewport.SetContent(m.renderHistory())
		}
		return m, nil

	case agentConnectResultMsg:
		if msg.err != nil {
			m.state = agentStateError
			m.err = locale.Tp("agent.connect_error", map[string]any{"Error": msg.err.Error()})
			return m, nil
		}
		m.state = agentStateReady
		if m.ready {
			m.viewport.SetContent(m.renderHistory())
			m.viewport.GotoBottom()
		}
		m.input.Focus()
		return m, textinput.Blink

	case streamStartMsg:
		m.streamCh = msg.ch
		return m, waitForChunk(m.streamCh)

	case streamChunkMsg:
		return m.handleStreamChunk(msg.chunk)

	case agentTitleMsg:
		m.session.Title = msg.title
		_ = config.SaveSession(m.session)
		return m, nil

	case spinner.TickMsg:
		if m.state == agentStateConnecting || m.streaming {
			sp, cmd := m.spin.Update(msg)
			m.spin = sp
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Delegate to input or viewport for other message types.
	if m.state == agentStateReady {
		if m.streaming {
			vp, cmd := m.viewport.Update(msg)
			m.viewport = vp
			return m, cmd
		}
		in, cmd := m.input.Update(msg)
		m.input = in
		return m, cmd
	}
	return m, nil
}

func (m *AgentModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.state == agentStateError {
		if msg.Type == tea.KeyEsc {
			return m, func() tea.Msg { return ScreenDoneMsg{From: ScreenAgent} }
		}
		return m, nil
	}

	if m.state != agentStateReady {
		return m, nil
	}

	// PgUp/PgDn always scroll the viewport regardless of palette or streaming state.
	switch msg.Type {
	case tea.KeyPgUp, tea.KeyPgDown:
		vp, cmd := m.viewport.Update(msg)
		m.viewport = vp
		return m, cmd
	}

	if m.streaming {
		// During streaming only scroll keys work.
		switch msg.Type {
		case tea.KeyUp, tea.KeyDown:
			vp, cmd := m.viewport.Update(msg)
			m.viewport = vp
			return m, cmd
		}
		return m, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		if m.showCmdPalette {
			m.showCmdPalette = false
			m.cmdMatches = nil
			m.cmdCursor = 0
			m.cmdOffset = 0
			m.input.SetValue("")
			m.updateViewportHeight()
		}
		return m, nil

	case tea.KeyUp:
		if m.showCmdPalette && len(m.cmdMatches) > 0 {
			if m.cmdCursor > 0 {
				m.cmdCursor--
				m.scrollPaletteToVisible()
			}
			return m, nil
		}
		vp, cmd := m.viewport.Update(msg)
		m.viewport = vp
		return m, cmd

	case tea.KeyDown:
		if m.showCmdPalette && len(m.cmdMatches) > 0 {
			if m.cmdCursor < len(m.cmdMatches)-1 {
				m.cmdCursor++
				m.scrollPaletteToVisible()
			}
			return m, nil
		}
		vp, cmd := m.viewport.Update(msg)
		m.viewport = vp
		return m, cmd

	case tea.KeyEnter:
		return m.handleEnter()
	}

	in, cmd := m.input.Update(msg)
	m.input = in
	m.updatePalette()
	return m, cmd
}

// updatePalette recomputes cmdMatches and showCmdPalette from the current input
// value, then resizes the viewport to account for the palette height change.
func (m *AgentModel) updatePalette() {
	val := m.input.Value()
	if !strings.HasPrefix(val, "/") || strings.Contains(val[1:], " ") {
		m.showCmdPalette = false
		m.cmdMatches = nil
		m.cmdCursor = 0
		m.cmdOffset = 0
		m.updateViewportHeight()
		return
	}

	filter := strings.ToLower(val[1:])
	var matches []agentCmd
	for _, cmd := range agentCommands {
		if strings.HasPrefix(cmd.name, filter) {
			matches = append(matches, cmd)
		}
	}
	m.cmdMatches = matches
	m.showCmdPalette = len(m.cmdMatches) > 0
	if m.cmdCursor >= len(m.cmdMatches) {
		m.cmdCursor = 0
		m.cmdOffset = 0
	}
	m.scrollPaletteToVisible()
	m.updateViewportHeight()
}

// updateViewportHeight resizes the viewport to fill available space after
// subtracting the fixed chrome and any visible palette rows.
func (m *AgentModel) updateViewportHeight() {
	if !m.ready {
		return
	}
	vpHeight := m.height - agentFixedHeight - m.paletteHeight()
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewport.Height = vpHeight
}

// paletteHeight returns the number of terminal lines the palette occupies.
func (m *AgentModel) paletteHeight() int {
	if !m.showCmdPalette || len(m.cmdMatches) == 0 {
		return 0
	}
	n := len(m.cmdMatches)
	if n > maxPaletteLines {
		n = maxPaletteLines
	}
	return n
}

// scrollPaletteToVisible adjusts cmdOffset so cmdCursor stays within the
// visible window of the palette.
func (m *AgentModel) scrollPaletteToVisible() {
	if m.cmdCursor < m.cmdOffset {
		m.cmdOffset = m.cmdCursor
	}
	if m.cmdCursor >= m.cmdOffset+maxPaletteLines {
		m.cmdOffset = m.cmdCursor - maxPaletteLines + 1
	}
}

// renderPalette builds the palette overlay string inserted between the second
// separator and the input row.
func (m *AgentModel) renderPalette() string {
	end := m.cmdOffset + maxPaletteLines
	if end > len(m.cmdMatches) {
		end = len(m.cmdMatches)
	}
	lines := make([]string, 0, end-m.cmdOffset)
	for i := m.cmdOffset; i < end; i++ {
		cmd := m.cmdMatches[i]
		desc := locale.T(cmd.descKey)
		text := fmt.Sprintf("%-25s %s", cmd.display, desc)
		if i == m.cmdCursor {
			line := m.styles.Cursor.Render(">") + " " + m.styles.Selected.Render(text)
			lines = append(lines, line)
		} else {
			lines = append(lines, "  "+text)
		}
	}
	return strings.Join(lines, "\n")
}

// handleEnter processes the Enter key. When the palette is visible it either
// executes a no-args command or autocompletes the name into the input field.
// When the palette is hidden it executes or sends the typed text to the agent.
func (m *AgentModel) handleEnter() (tea.Model, tea.Cmd) {
	if m.showCmdPalette && len(m.cmdMatches) > 0 {
		cmd := m.cmdMatches[m.cmdCursor]
		if cmd.noArgs {
			return m.emitCommand(cmd.name, nil)
		}
		// Autocomplete: put "/name " in the input so the user can type the args.
		m.input.SetValue("/" + cmd.name + " ")
		m.input.CursorEnd()
		m.showCmdPalette = false
		m.cmdMatches = nil
		m.cmdCursor = 0
		m.cmdOffset = 0
		m.updateViewportHeight()
		return m, nil
	}

	val := strings.TrimSpace(m.input.Value())
	if val == "" {
		return m, nil
	}

	// Try to parse a slash command (palette hidden, user typed it directly).
	if strings.HasPrefix(val, "/") {
		parts := strings.Fields(val[1:])
		if len(parts) > 0 {
			name := parts[0]
			args := parts[1:]
			for _, cmd := range agentCommands {
				if cmd.name == name {
					return m.emitCommand(name, args)
				}
			}
		}
		// Unknown slash-command: fall through and send as regular text.
	}

	return m.sendMessage(val)
}

// emitCommand resets palette state and emits a CommandResult ScreenDoneMsg.
func (m *AgentModel) emitCommand(name string, args []string) (tea.Model, tea.Cmd) {
	m.showCmdPalette = false
	m.cmdMatches = nil
	m.cmdCursor = 0
	m.cmdOffset = 0
	m.input.SetValue("")
	m.updateViewportHeight()
	return m, func() tea.Msg {
		return ScreenDoneMsg{
			From:   ScreenAgent,
			Result: CommandResult{Cmd: name, Args: args},
		}
	}
}

func (m *AgentModel) handleStreamChunk(chunk llm.StreamChunk) (tea.Model, tea.Cmd) {
	if chunk.Err != nil {
		m.err = locale.Tp("agent.error", map[string]any{"Error": chunk.Err.Error()})
		m.streaming = false
		m.streamCh = nil
		m.streambuf.Reset()
		if m.ready {
			m.viewport.SetContent(m.renderHistory())
			m.viewport.GotoBottom()
		}
		return m, nil
	}

	if !chunk.Done {
		m.streambuf.WriteString(chunk.Content)
		if m.ready {
			m.viewport.SetContent(m.renderHistory())
			m.viewport.GotoBottom()
		}
		return m, waitForChunk(m.streamCh)
	}

	// Stream finished: commit the assistant message.
	content := m.streambuf.String()
	m.streambuf.Reset()
	m.streaming = false
	m.streamCh = nil
	m.err = ""

	if content != "" {
		m.session.History = append(m.session.History, config.ChatTurn{Role: "assistant", Content: content})
		m.messages = append(m.messages, llm.Message{Role: llm.RoleAssistant, Content: content})
	}
	m.session.UpdatedAt = time.Now()
	_ = config.SaveSession(m.session)

	if m.ready {
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
	}
	m.input.Focus()

	// Request an AI-generated title on the first completed exchange.
	if m.session.Title == "new_session" {
		return m, requestTitleCmd(m.provider, m.modelID, m.session.History)
	}
	return m, nil
}

// requestTitleCmd asks the LLM to produce a short title for the session.
// It always returns an agentTitleMsg — "new_session" on any error.
func requestTitleCmd(provider llm.AIProvider, modelID string, history []config.ChatTurn) tea.Cmd {
	return func() tea.Msg {
		msgs := make([]llm.Message, 0, len(history)+1)
		for _, turn := range history {
			msgs = append(msgs, llm.Message{Role: llm.Role(turn.Role), Content: turn.Content})
		}
		msgs = append(msgs, llm.Message{
			Role:    llm.RoleUser,
			Content: "Provide a short title (3-5 words) for this conversation. Reply with ONLY the title, nothing else.",
		})
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		resp, err := provider.Complete(ctx, llm.CompletionRequest{Model: modelID, Messages: msgs})
		if err != nil {
			return agentTitleMsg{title: "new_session"}
		}
		title := strings.TrimSpace(resp.Message.Content)
		if title == "" {
			title = "new_session"
		}
		return agentTitleMsg{title: title}
	}
}

func (m *AgentModel) sendMessage(text string) (tea.Model, tea.Cmd) {
	m.input.SetValue("")
	m.err = ""
	m.session.History = append(m.session.History, config.ChatTurn{Role: "user", Content: text})
	m.messages = append(m.messages, llm.Message{Role: llm.RoleUser, Content: text})
	m.streaming = true
	m.streambuf.Reset()

	if m.ready {
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
	}

	msgs := make([]llm.Message, len(m.messages))
	copy(msgs, m.messages)
	return m, tea.Batch(m.spin.Tick, startStreamCmd(m.provider, m.modelID, msgs))
}

// renderHistory builds the string content for the viewport.
// Each turn is word-wrapped to m.width so lines never extend beyond the
// visible area (the viewport does not wrap automatically).
func (m *AgentModel) renderHistory() string {
	if len(m.session.History) == 0 && !m.streaming {
		return m.styles.Hint.Render(locale.T("agent.empty"))
	}

	wrap := func(s string) string {
		if m.width <= 0 {
			return s
		}
		return lipgloss.NewStyle().Width(m.width).Render(s)
	}

	var sb strings.Builder
	for _, turn := range m.session.History {
		var line string
		if turn.Role == "user" {
			line = m.styles.Selected.Render("You: ") + turn.Content
		} else {
			line = m.styles.Hint.Render("Auris: ") + turn.Content
		}
		sb.WriteString(wrap(line))
		sb.WriteString("\n\n")
	}

	if m.streaming {
		line := m.styles.Hint.Render("Auris: ") + m.streambuf.String() +
			m.styles.Cursor.Render(locale.T("agent.streaming"))
		sb.WriteString(wrap(line))
	}

	if m.err != "" {
		sb.WriteString("\n")
		sb.WriteString(wrap(m.styles.Error.Render(fmt.Sprintf("✗ %s", m.err))))
	}

	return sb.String()
}

// View implements [tea.Model].
func (m *AgentModel) View() string {
	switch m.state {
	case agentStateConnecting:
		return m.styles.Hint.Render(
			fmt.Sprintf("%s %s", m.spin.View(), locale.T("agent.connecting")),
		)
	case agentStateError:
		return lipgloss.JoinVertical(lipgloss.Left,
			m.styles.Error.Render("✗ "+m.err),
		)
	}

	if !m.ready {
		return ""
	}

	sep := strings.Repeat("─", m.width)
	inputRow := m.styles.Cursor.Render(">") + " " + m.input.View()

	var hint string
	if m.showCmdPalette {
		hint = m.styles.Hint.Render(locale.T("agent.cmd.hint"))
	} else {
		hint = m.styles.Hint.Render(locale.T("agent.hint"))
	}

	parts := []string{
		m.styles.Selected.Render(locale.T("agent.title")),
		sep,
		m.viewport.View(),
		sep,
	}

	if m.showCmdPalette && len(m.cmdMatches) > 0 {
		parts = append(parts, m.renderPalette())
	}

	parts = append(parts, inputRow, hint)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
