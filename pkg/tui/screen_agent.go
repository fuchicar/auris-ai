package tui

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/agent"
	"auris/pkg/config"
	"auris/pkg/llm"
	"auris/pkg/locale"
	"auris/pkg/market"
	"auris/pkg/news"
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
	{"model", "/model", "agent.cmd.model", true},
	{"theme", "/theme [light|dark|greenlight|greendark|boxlight|boxdark]", "agent.cmd.theme", false},
	{"language", "/language [en|es]", "agent.cmd.language", false},
	{"exit", "/exit", "agent.cmd.exit", true},
}

// agentConnectResultMsg carries the outcome of the provider Connect() call.
type agentConnectResultMsg struct{ err error }

// agentResponseMsg carries the final assistant message returned by agent.Chat.
type agentResponseMsg struct {
	content string
	err     error
}

// agentProgressMsg is emitted while the agent executes tools, one per unique ProgressKind.
type agentProgressMsg struct{ kind agent.ProgressKind }

// activeSessionChangedMsg notifies AppModel that the active session ID changed
// so it can persist the updated config.
type activeSessionChangedMsg struct{ id string }

// AgentModel is the chat UI screen. It runs the agentic loop via agent.Agent
// and persists the conversation as a [config.Session] file after each exchange.
type AgentModel struct {
	state     agentState
	session   *config.Session
	viewport  viewport.Model
	input     textinput.Model
	spin      spinner.Model
	messages  []llm.Message // multi-turn context sent to the LLM
	provider  llm.AIProvider
	mp        market.ProviderAPI
	ag        *agent.Agent
	streaming    bool
	streambuf    strings.Builder
	modelID      string
	styles       *Styles
	err          string
	ready        bool // true once the viewport has been sized
	width        int
	height       int
	progressCh   chan agent.ProgressEvent
	toolLogs     []string // display-only tool activity lines for the current turn
	toolLogsTurn int      // index into session.History of the user message that owns toolLogs

	// Markdown renderer — recreated when viewport width or light/dark base changes.
	renderer      *glamour.TermRenderer
	rendererWidth int
	rendererLight bool

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
func newAgentModel(provider llm.AIProvider, mp market.ProviderAPI, session *config.Session, modelID string, s *Styles, width, height int, profile *config.FinancialProfile, newsFeeds []news.FeedConfig, debugLogger *log.Logger) *AgentModel {
	ti := textinput.New()
	ti.Placeholder = locale.T("agent.placeholder")
	ti.Prompt = "" // the ">" prefix is rendered manually in View()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = s.Spinner

	// Reconstruct the LLM message context from the persisted history.
	messages := make([]llm.Message, 0, len(session.History)+1)
	if sysMsg := agent.BuildSystemMessage(llm.TaskChat, profile); sysMsg != nil {
		messages = append(messages, *sysMsg)
	}
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
		mp:       mp,
		ag:      agent.New(provider, mp, modelID, agent.WithDebugLogger(debugLogger)),
		modelID: modelID,
		styles:   s,
		width:    width,
		height:   height,
	}

	m.ag.SetNewsProvider(news.NewProvider(newsFeeds))

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
	return tea.Batch(m.spin.Tick, agentConnectCmd(m.provider, m.mp))
}

func agentConnectCmd(provider llm.AIProvider, mp market.ProviderAPI) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := provider.Connect(ctx); err != nil {
			return agentConnectResultMsg{err: err}
		}
		if mp != nil {
			_ = mp.Connect(ctx) // best-effort; errors surface through tool call results
		}
		return agentConnectResultMsg{err: nil}
	}
}

func startAgentCmd(ag *agent.Agent, messages []llm.Message) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		msg, err := ag.Chat(ctx, messages)
		return agentResponseMsg{content: msg.Content, err: err}
	}
}

// listenProgressCmd blocks on a single read from ch and returns the event.
// It returns nil when the channel is closed, ending the listener chain.
func listenProgressCmd(ch <-chan agent.ProgressEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return agentProgressMsg{kind: ev.Kind}
	}
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

	case agentProgressMsg:
		var text string
		switch msg.kind {
		case agent.ProgressFinancial:
			text = locale.T("agent.tool_financial")
		case agent.ProgressCalculation:
			text = locale.T("agent.tool_calculation")
		case agent.ProgressNews:
			text = locale.T("agent.tool_news")
		}
		if text != "" {
			m.toolLogs = append(m.toolLogs, text)
			if m.ready {
				m.viewport.SetContent(m.renderHistory())
				m.viewport.GotoBottom()
			}
		}
		return m, listenProgressCmd(m.progressCh)

	case agentResponseMsg:
		return m.handleAgentResponse(msg)

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

func (m *AgentModel) handleAgentResponse(msg agentResponseMsg) (tea.Model, tea.Cmd) {
	m.streaming = false
	m.streambuf.Reset()

	// Signal the listener goroutine to stop.
	if m.progressCh != nil {
		close(m.progressCh)
		m.progressCh = nil
		m.ag.SetProgressCh(nil)
	}

	if msg.err != nil {
		m.err = locale.Tp("agent.error", map[string]any{"Error": msg.err.Error()})
		if m.ready {
			m.viewport.SetContent(m.renderHistory())
			m.viewport.GotoBottom()
		}
		return m, nil
	}

	m.err = ""
	if msg.content != "" {
		m.session.History = append(m.session.History, config.ChatTurn{Role: "assistant", Content: msg.content})
		m.messages = append(m.messages, llm.Message{Role: llm.RoleAssistant, Content: msg.content})
	}
	m.session.UpdatedAt = time.Now()
	_ = config.SaveSession(m.session)

	if m.ready {
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
	}
	m.input.Focus()

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

	// Reset tool logs for this new turn.
	m.toolLogs = nil
	m.toolLogsTurn = len(m.session.History) - 1

	// Wire up the progress channel so tool activity is reported back to the TUI.
	ch := make(chan agent.ProgressEvent, 8)
	m.progressCh = ch
	m.ag.SetProgressCh(ch)

	if m.ready {
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
	}

	msgs := make([]llm.Message, len(m.messages))
	copy(msgs, m.messages)
	return m, tea.Batch(m.spin.Tick, startAgentCmd(m.ag, msgs), listenProgressCmd(ch))
}

// ensureRenderer creates or recreates the TermRenderer when the viewport width or
// the light/dark base of the active theme has changed.
func (m *AgentModel) ensureRenderer() {
	wantLight := m.styles.IsLight()
	if m.renderer != nil && m.rendererWidth == m.width && m.rendererLight == wantLight {
		return
	}
	if m.width <= 0 {
		return
	}
	glamourStyle := "dark"
	if wantLight {
		glamourStyle = "light"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(glamourStyle),
		glamour.WithWordWrap(m.width),
	)
	if err != nil {
		return
	}
	m.renderer = r
	m.rendererWidth = m.width
	m.rendererLight = wantLight
}

// renderMarkdown renders s as Markdown. Falls back to s on any error.
func (m *AgentModel) renderMarkdown(s string) string {
	m.ensureRenderer()
	if m.renderer == nil {
		return s
	}
	out, err := m.renderer.Render(s)
	if err != nil {
		return s
	}
	return out
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
	for i, turn := range m.session.History {
		if turn.Role == "user" {
			sb.WriteString(wrap(m.styles.Selected.Render("You: ") + turn.Content))
			sb.WriteString("\n\n")
			// Render tool activity lines associated with this user message.
			if i == m.toolLogsTurn {
				for _, log := range m.toolLogs {
					sb.WriteString(wrap(m.styles.Warning.Render("⚙ " + log)))
					sb.WriteString("\n\n")
				}
			}
		} else {
			rendered := m.renderMarkdown(turn.Content)
			sb.WriteString(RenderAgentBlock(m.styles, rendered, m.width))
			sb.WriteString("\n\n")
		}
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
