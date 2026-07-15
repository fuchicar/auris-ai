package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/fuchicar/auris-ai/pkg/agent"
	"github.com/fuchicar/auris-ai/pkg/config"
	"github.com/fuchicar/auris-ai/pkg/drivers/simulation"
	"github.com/fuchicar/auris-ai/pkg/llm"
	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/market"
	"github.com/fuchicar/auris-ai/pkg/news"
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

// agentChromeHeight is the number of terminal lines occupied by fixed chrome
// (title, two separators, hint). The input row's height is variable (see
// maxInputLines) and is added separately. The viewport fills the rest.
const agentChromeHeight = 4

// maxInputLines caps how many rows the input box can grow to before it
// starts scrolling internally instead of consuming more of the screen.
const maxInputLines = 6

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
	{"marketproviders", "/marketproviders", "agent.cmd.marketproviders", true},
	{"aiproviders", "/aiproviders", "agent.cmd.aiproviders", true},
	{"simulation", "/simulation", "agent.cmd.simulation", true},
	{"export", "/export [filename]", "agent.cmd.export", true},
	{"info", "/info", "agent.cmd.info", true},
	{"theme", "/theme [light|dark|greenlight|greendark|boxlight|boxdark]", "agent.cmd.theme", false},
	{"language", "/language [en|es]", "agent.cmd.language", false},
	{"exit", "/exit", "agent.cmd.exit", true},
}

// agentExportMsg carries the result of an /export command (file path or error).
type agentExportMsg struct {
	path string
	err  error
}

// agentConnectResultMsg carries the outcome of the provider Connect() call.
type agentConnectResultMsg struct{ err error }

// agentResponseMsg carries the final assistant message returned by agent.Chat.
type agentResponseMsg struct {
	content string
	err     error
}

// agentProgressMsg is emitted while the agent executes tools, one per distinct tool call.
type agentProgressMsg struct{ name, args string }

// agentChartMsg carries a chart already rendered to a string (possibly "" if
// there wasn't enough candle data) for a market_render_price_chart call.
type agentChartMsg struct{ chart string }

// agentStreamChunkMsg carries one incremental text fragment from the agent's
// streaming LLM call.
type agentStreamChunkMsg struct{ text string }

// activeSessionChangedMsg notifies AppModel that the active session ID changed
// so it can persist the updated config.
type activeSessionChangedMsg struct{ id string }

// inferenceTickMsg drives the animation and timer while the agent is running.
type inferenceTickMsg struct{}

// AgentModel is the chat UI screen. It runs the agentic loop via agent.Agent
// and persists the conversation as a [config.Session] file after each exchange.
type AgentModel struct {
	state         agentState
	session       *config.Session
	viewport      viewport.Model
	input         textarea.Model
	spin          spinner.Model
	messages      []llm.Message // multi-turn context sent to the LLM
	provider      llm.AIProvider
	mp            market.ProviderAPI
	ag            *agent.Agent
	streaming     bool
	streambuf     strings.Builder
	modelID       string
	styles        *Styles
	err           string
	ready         bool // true once the viewport has been sized
	width         int
	height        int
	progressCh    chan agent.ProgressEvent
	streamCh      chan string           // delivers incremental LLM text fragments to streambuf
	chartCh       chan agent.ChartEvent // delivers candle data for market_render_price_chart calls
	toolLogs      []string              // display-only tool activity lines for the current turn
	toolLogsTurn  int                   // index into session.History of the user message that owns toolLogs
	pendingCharts []string              // rendered ASCII chart blocks accumulated during the current turn

	// simulationMode is true when the active market provider is the
	// synthetic simulation driver (config.AurisConfig.SimulationMode) — shown
	// as a persistent notice/badge so the user never mistakes fabricated data
	// for real market data.
	simulationMode bool

	// infoMsg is a transient status line shown after /export (cleared on next send).
	infoMsg string

	// infoBlock is a transient multi-line info panel shown after /info (cleared on next send).
	infoBlock string

	// Markdown renderer — recreated when viewport width or light/dark base changes.
	renderer      *glamour.TermRenderer
	rendererWidth int
	rendererLight bool

	// Command palette state.
	showCmdPalette bool
	cmdCursor      int
	cmdOffset      int
	cmdMatches     []agentCmd

	// Inference timer and animation.
	inferenceStart   time.Time
	inferenceElapsed time.Duration
	inferenceCancel  context.CancelFunc
	boxFrame         int // 0–9, progressive-fill cycle
}

// newAgentModel constructs an [AgentModel]. session provides the persisted chat
// log and metadata; modelID selects which model to use for completions.
// width and height are the current terminal dimensions; passing them allows the
// viewport to be initialised immediately without waiting for a WindowSizeMsg.
// sysMsg overrides the default system prompt when non-nil (e.g. portfolio agent).
// portfolioID, when non-empty, scopes portfolio management tools to that portfolio.
// simulationMode, when true, uses the synthetic simulation news source
// instead of real RSS feeds and shows the simulation-mode notice/badge.
func newAgentModel(provider llm.AIProvider, mp market.ProviderAPI, session *config.Session, modelID string, s *Styles, width, height int, profile *config.FinancialProfile, newsFeeds []news.FeedConfig, debugLogger *log.Logger, sysMsg *llm.Message, portfolioID string, simulationMode bool) *AgentModel {
	ti := textarea.New()
	ti.Placeholder = locale.T("agent.placeholder")
	ti.ShowLineNumbers = false
	ti.SetPromptFunc(2, func(lineIdx int) string {
		if lineIdx == 0 {
			return "> "
		}
		return "  "
	})
	ti.FocusedStyle.Prompt = s.Cursor
	ti.BlurredStyle.Prompt = s.Cursor
	ti.SetWidth(width)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = s.Spinner

	// Reconstruct the LLM message context from the persisted history.
	messages := make([]llm.Message, 0, len(session.History)+1)
	effectiveSysMsg := sysMsg
	if effectiveSysMsg == nil {
		effectiveSysMsg = agent.BuildSystemMessage(llm.TaskChat, profile)
	}
	if effectiveSysMsg != nil {
		messages = append(messages, *effectiveSysMsg)
	}
	for _, turn := range session.History {
		messages = append(messages, llm.Message{
			Role:    llm.Role(turn.Role),
			Content: turn.Content,
		})
	}

	m := &AgentModel{
		state:          agentStateConnecting,
		session:        session,
		input:          ti,
		spin:           sp,
		messages:       messages,
		provider:       provider,
		mp:             mp,
		ag:             agent.New(provider, mp, modelID, agent.WithDebugLogger(debugLogger), agent.WithPortfolioID(portfolioID)),
		modelID:        modelID,
		styles:         s,
		width:          width,
		height:         height,
		simulationMode: simulationMode,
	}

	var newsSource news.Source = news.NewProvider(newsFeeds)
	if simulationMode {
		if simDriver, ok := mp.(*simulation.Driver); ok {
			newsSource = simDriver.NewsSource()
		}
	}
	m.ag.SetNewsProvider(newsSource)

	// Initialise the viewport now so it is ready as soon as the connection
	// succeeds. BubbleTea only sends WindowSizeMsg once (at startup), so new
	// screens created later must seed their own dimensions.
	if width > 0 && height > 0 {
		vpHeight := height - agentChromeHeight - m.input.Height()
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

// startAgentStreamCmd runs the agentic loop via ag.ChatStream, forwarding each
// text fragment onto chunkCh as it arrives, and returns the same
// agentResponseMsg contract as a blocking call once the loop completes.
func startAgentStreamCmd(ag *agent.Agent, messages []llm.Message, ctx context.Context, chunkCh chan<- string) tea.Cmd {
	return func() tea.Msg {
		onDelta := func(s string) {
			select {
			case chunkCh <- s:
			case <-ctx.Done():
			}
		}
		msg, err := ag.ChatStream(ctx, messages, onDelta)
		return agentResponseMsg{content: msg.Content, err: err}
	}
}

func inferenceTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return inferenceTickMsg{}
	})
}

// listenProgressCmd blocks on a single read from ch and returns the event.
// It returns nil when the channel is closed, ending the listener chain.
func listenProgressCmd(ch <-chan agent.ProgressEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return agentProgressMsg{name: ev.Name, args: ev.Args}
	}
}

// formatToolCall renders a tool call for the chat log, e.g. "market_get_quote(AAPL)"
// for a single argument, or "portfolio_add_lot(price=100, quantity=5, symbol=AAPL)"
// for several (sorted by key for determinism — map iteration order is not stable).
func formatToolCall(name, argsJSON string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil || len(args) == 0 {
		return name + "()"
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 1 {
		return name + "(" + formatArgValue(args[keys[0]]) + ")"
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + formatArgValue(args[k])
	}
	return name + "(" + strings.Join(parts, ", ") + ")"
}

func formatArgValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case []any:
		return fmt.Sprintf("[%d]", len(x))
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", x)
	}
}

// listenChartCmd blocks on a single read from ch, renders the candle data via
// renderCandleChart, and returns the result. It returns nil when the channel
// is closed, ending the listener chain.
func listenChartCmd(ch <-chan agent.ChartEvent, s *Styles) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return agentChartMsg{chart: renderCandleChart(ev.Candles, s)}
	}
}

// listenStreamCmd blocks on a single read from ch and returns the fragment.
// It returns nil when the channel is closed, ending the listener chain.
func listenStreamCmd(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		text, ok := <-ch
		if !ok {
			return nil
		}
		return agentStreamChunkMsg{text: text}
	}
}

// Update implements [tea.Model].
func (m *AgentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(m.width)
		if !m.ready {
			vpHeight := m.height - agentChromeHeight - m.input.Height() - m.paletteHeight()
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
		return m, textarea.Blink

	case agentProgressMsg:
		m.toolLogs = append(m.toolLogs, formatToolCall(msg.name, msg.args))
		if m.ready {
			m.viewport.SetContent(m.renderHistory())
			m.viewport.GotoBottom()
		}
		return m, listenProgressCmd(m.progressCh)

	case agentChartMsg:
		if msg.chart != "" {
			m.pendingCharts = append(m.pendingCharts, msg.chart)
		}
		return m, listenChartCmd(m.chartCh, m.styles)

	case agentStreamChunkMsg:
		m.streambuf.WriteString(msg.text)
		if m.ready {
			m.viewport.SetContent(m.renderHistory())
			m.viewport.GotoBottom()
		}
		return m, listenStreamCmd(m.streamCh)

	case agentExportMsg:
		if msg.err != nil {
			m.err = locale.Tp("agent.export_error", map[string]any{"Error": msg.err.Error()})
		} else {
			m.infoMsg = locale.Tp("agent.export_success", map[string]any{"Path": msg.path})
		}
		if m.ready {
			m.viewport.SetContent(m.renderHistory())
			m.viewport.GotoBottom()
		}
		return m, nil

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

	case inferenceTickMsg:
		if m.streaming {
			m.boxFrame = (m.boxFrame + 1) % 10
			m.inferenceElapsed = time.Since(m.inferenceStart)
			return m, inferenceTickCmd()
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
		switch msg.Type {
		case tea.KeyEsc:
			if m.inferenceCancel != nil {
				m.inferenceCancel()
				m.inferenceCancel = nil
			}
			return m, nil
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
			m.updateInputHeight()
		}
		return m, nil

	case tea.KeyUp:
		if msg.Alt {
			m.input.CursorUp()
			return m, nil
		}
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
		if msg.Alt {
			m.input.CursorDown()
			return m, nil
		}
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

	case tea.KeyCtrlJ:
		m.insertNewline()
		return m, nil

	case tea.KeyEnter:
		if msg.Alt {
			m.insertNewline()
			return m, nil
		}
		// A trailing "\" is a manual escape for terminals that don't report
		// Alt+Enter/Ctrl+J distinctly: drop the backslash and insert a newline
		// instead of submitting.
		if val := m.input.Value(); strings.HasSuffix(val, `\`) {
			m.input.SetValue(strings.TrimSuffix(val, `\`))
			m.insertNewline()
			return m, nil
		}
		return m.handleEnter()
	}

	in, cmd := m.input.Update(msg)
	m.input = in
	m.updateInputHeight()
	m.updatePalette()
	return m, cmd
}

// insertNewline inserts a literal newline at the cursor without submitting
// the message, then resizes the input box to fit. Used by every newline
// trigger (Ctrl+J, Alt+Enter, the "\"+Enter escape) so they behave identically.
func (m *AgentModel) insertNewline() {
	m.input.InsertRune('\n')
	m.updateInputHeight()
}

// updatePalette recomputes cmdMatches and showCmdPalette from the current input
// value, then resizes the viewport to account for the palette height change.
func (m *AgentModel) updatePalette() {
	val := m.input.Value()
	if !strings.HasPrefix(val, "/") || strings.Contains(val[1:], " ") || strings.Contains(val, "\n") {
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

// updateInputHeight resizes the input box to fit the number of visual rows
// its content currently needs (up to maxInputLines), then resizes the
// viewport to reclaim/yield the difference. LineCount() alone only counts
// hard line breaks (Ctrl+J/Alt+Enter/"\"+Enter); a single long line that
// soft-wraps within the box's width must grow it exactly the same way, so
// the wrapped row count is computed too and the larger of the two wins.
func (m *AgentModel) updateInputHeight() {
	lines := m.input.LineCount()
	if w := m.input.Width(); w > 0 {
		wrapped := ansi.Wordwrap(m.input.Value(), w, "")
		if wc := strings.Count(wrapped, "\n") + 1; wc > lines {
			lines = wc
		}
	}
	if lines > maxInputLines {
		lines = maxInputLines
	}
	m.input.SetHeight(lines)
	m.updateViewportHeight()
}

// updateViewportHeight resizes the viewport to fill available space after
// subtracting the fixed chrome, the input box's current height, and any
// visible palette rows.
func (m *AgentModel) updateViewportHeight() {
	if !m.ready {
		return
	}
	vpHeight := m.height - agentChromeHeight - m.input.Height() - m.paletteHeight()
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
			if cmd.name == "export" {
				return m.handleExportCmd(nil)
			}
			if cmd.name == "info" {
				return m.handleInfoCmd()
			}
			return m.emitCommand(cmd.name, nil)
		}
		// Autocomplete: put "/name " in the input so the user can type the args.
		m.input.SetValue("/" + cmd.name + " ")
		m.input.CursorEnd()
		m.showCmdPalette = false
		m.cmdMatches = nil
		m.cmdCursor = 0
		m.cmdOffset = 0
		m.updateInputHeight()
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
					if name == "export" {
						return m.handleExportCmd(args)
					}
					if name == "info" {
						return m.handleInfoCmd()
					}
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
	m.updateInputHeight()
	return m, func() tea.Msg {
		return ScreenDoneMsg{
			From:   ScreenAgent,
			Result: CommandResult{Cmd: name, Args: args},
		}
	}
}

// handleExportCmd resets palette state and dispatches an export command.
// args[0], if present, is treated as the output filename.
func (m *AgentModel) handleExportCmd(args []string) (tea.Model, tea.Cmd) {
	m.showCmdPalette = false
	m.cmdMatches = nil
	m.cmdCursor = 0
	m.cmdOffset = 0
	m.input.SetValue("")
	m.updateInputHeight()
	m.err = ""
	m.infoMsg = ""
	m.infoBlock = ""
	var filename string
	if len(args) > 0 {
		filename = args[0]
	}
	return m, exportSessionCmd(m.session, filename)
}

// exportSessionCmd writes the session history to a markdown file and returns
// an agentExportMsg with the resolved path or an error.
func exportSessionCmd(session *config.Session, filename string) tea.Cmd {
	return func() tea.Msg {
		if filename == "" {
			ts := time.Now().Format("20060102-150405")
			filename = fmt.Sprintf("auris-export-%s.md", ts)
		} else if !strings.HasSuffix(filename, ".md") && !strings.Contains(filename, ".") {
			filename += ".md"
		}
		absPath, err := filepath.Abs(filename)
		if err != nil {
			return agentExportMsg{err: err}
		}
		f, err := os.Create(absPath)
		if err != nil {
			return agentExportMsg{err: err}
		}
		defer f.Close()

		title := session.Title
		if title == "" || title == "new_session" {
			title = "Untitled"
		}
		fmt.Fprintf(f, "# Auris — %s\n\n", title)
		fmt.Fprintf(f, "**Date:** %s\n\n", session.UpdatedAt.Format("2006-01-02 15:04:05"))
		fmt.Fprintln(f, "---")
		for _, turn := range session.History {
			fmt.Fprintln(f)
			switch turn.Role {
			case "user":
				fmt.Fprintf(f, "**You:** %s\n", turn.Content)
			case "assistant":
				fmt.Fprintf(f, "**Auris:** %s\n", turn.Content)
				for _, chart := range turn.Charts {
					fmt.Fprintf(f, "\n```\n%s\n```\n", ansi.Strip(chart))
				}
			}
		}
		if err := f.Sync(); err != nil {
			return agentExportMsg{err: err}
		}
		return agentExportMsg{path: absPath}
	}
}

// handleInfoCmd resets palette state and renders the /info panel. Unlike
// /export it needs no I/O — provider/model/session data is already in
// memory — so it runs synchronously with no tea.Cmd round-trip.
func (m *AgentModel) handleInfoCmd() (tea.Model, tea.Cmd) {
	m.showCmdPalette = false
	m.cmdMatches = nil
	m.cmdCursor = 0
	m.cmdOffset = 0
	m.input.SetValue("")
	m.updateInputHeight()
	m.err = ""
	m.infoMsg = ""
	m.infoBlock = m.buildInfoBlock()
	if m.ready {
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
	}
	return m, nil
}

// buildInfoBlock composes the multi-line panel shown by /info: which
// provider/model is running (and its context window, if known), plus the
// current session's name, size, and how much of that window the most recent
// turn actually used.
func (m *AgentModel) buildInfoBlock() string {
	marketProviderName := locale.T("agent.info.market_provider_none")
	if m.mp != nil {
		marketProviderName = m.mp.Name()
	}
	lines := []string{
		locale.Tp("agent.info.provider", map[string]any{"Provider": m.provider.Name()}),
		locale.Tp("agent.info.model", map[string]any{"Model": m.modelID}),
		locale.Tp("agent.info.market_provider", map[string]any{"Provider": marketProviderName}),
	}

	var window int
	if cwr, ok := m.provider.(llm.ContextWindowReporter); ok {
		window = cwr.ContextWindow(m.modelID)
	}
	if window > 0 {
		lines = append(lines, locale.Tp("agent.info.context_window", map[string]any{"Tokens": window}))
	} else {
		lines = append(lines, locale.T("agent.info.context_window_unknown"))
	}

	title := m.session.Title
	if title == "" || title == "new_session" {
		title = "Untitled"
	}
	lines = append(lines,
		"",
		locale.Tp("agent.info.session_name", map[string]any{"Title": title}),
		locale.Tp("agent.info.session_messages", map[string]any{"Count": len(m.session.History)}),
		locale.Tp("agent.info.session_created", map[string]any{"Date": m.session.CreatedAt.Format("2006-01-02 15:04")}),
	)

	usage := m.ag.LastUsage()
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		lines = append(lines, locale.T("agent.info.context_used_none"))
	} else {
		total := usage.PromptTokens + usage.CompletionTokens
		if window > 0 {
			pct := float64(usage.PromptTokens) / float64(window) * 100
			lines = append(lines, locale.Tp("agent.info.context_used_pct", map[string]any{
				"Prompt":     usage.PromptTokens,
				"Completion": usage.CompletionTokens,
				"Total":      total,
				"Pct":        fmt.Sprintf("%.1f", pct),
			}))
		} else {
			lines = append(lines, locale.Tp("agent.info.context_used", map[string]any{
				"Prompt":     usage.PromptTokens,
				"Completion": usage.CompletionTokens,
				"Total":      total,
			}))
		}
	}

	return strings.Join(lines, "\n")
}

func (m *AgentModel) handleAgentResponse(msg agentResponseMsg) (tea.Model, tea.Cmd) {
	m.streaming = false
	m.streambuf.Reset()
	m.inferenceElapsed = time.Since(m.inferenceStart)
	if m.inferenceCancel != nil {
		m.inferenceCancel()
		m.inferenceCancel = nil
	}
	m.boxFrame = 0

	// Signal the listener goroutines to stop.
	if m.progressCh != nil {
		close(m.progressCh)
		m.progressCh = nil
		m.ag.SetProgressCh(nil)
	}
	if m.streamCh != nil {
		close(m.streamCh)
		m.streamCh = nil
	}
	if m.chartCh != nil {
		close(m.chartCh)
		m.chartCh = nil
		m.ag.SetChartCh(nil)
	}
	// Capture and clear pendingCharts unconditionally — a chart fetched
	// earlier in the turn must not leak into a later turn if this one ends
	// in an error or cancellation instead of a successful response.
	charts := m.pendingCharts
	m.pendingCharts = nil

	if errors.Is(msg.err, context.Canceled) {
		m.err = ""
		if m.ready {
			m.viewport.SetContent(m.renderHistory())
			m.viewport.GotoBottom()
		}
		m.input.Focus()
		return m, nil
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
	if msg.content != "" || len(charts) > 0 {
		m.session.History = append(m.session.History, config.ChatTurn{Role: "assistant", Content: msg.content, Charts: charts})
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
	m.updateInputHeight()
	m.err = ""
	m.infoMsg = ""
	m.infoBlock = ""
	m.session.History = append(m.session.History, config.ChatTurn{Role: "user", Content: text})
	m.messages = append(m.messages, llm.Message{Role: llm.RoleUser, Content: text})
	m.streaming = true
	m.streambuf.Reset()

	// Reset tool logs and pending charts for this new turn.
	m.toolLogs = nil
	m.toolLogsTurn = len(m.session.History) - 1
	m.pendingCharts = nil

	// Wire up the progress channel so tool activity is reported back to the TUI.
	ch := make(chan agent.ProgressEvent, 8)
	m.progressCh = ch
	m.ag.SetProgressCh(ch)

	// Wire up the chart channel so market_render_price_chart calls are
	// reported back to the TUI and rendered client-side (never through the
	// LLM's own phrasing — see FEAT-4 chat-tool session notes).
	chartCh := make(chan agent.ChartEvent, 8)
	m.chartCh = chartCh
	m.ag.SetChartCh(chartCh)

	// Wire up the stream channel so incremental LLM text is reported back to
	// the TUI. Buffer is larger than progressCh's since text fragments arrive
	// far more frequently and in smaller units.
	chunkCh := make(chan string, 64)
	m.streamCh = chunkCh

	// Start inference timer and animation.
	ctx, cancel := context.WithCancel(context.Background())
	m.inferenceCancel = cancel
	m.inferenceStart = time.Now()
	m.inferenceElapsed = 0
	m.boxFrame = 0

	if m.ready {
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
	}

	msgs := make([]llm.Message, len(m.messages))
	copy(msgs, m.messages)
	return m, tea.Batch(m.spin.Tick, startAgentStreamCmd(m.ag, msgs, ctx, chunkCh), listenProgressCmd(ch), listenChartCmd(chartCh, m.styles), listenStreamCmd(chunkCh), inferenceTickCmd())
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
	wrap := func(s string) string {
		if m.width <= 0 {
			return s
		}
		return lipgloss.NewStyle().Width(m.width).Render(s)
	}

	var notice string
	if m.simulationMode {
		notice = wrap(m.styles.Warning.Render("⚠ "+locale.T("agent.simulation_notice"))) + "\n\n"
	}

	if len(m.session.History) == 0 && !m.streaming && m.err == "" && m.infoMsg == "" && m.infoBlock == "" {
		return notice + m.styles.Hint.Render(locale.T("agent.empty"))
	}

	var sb strings.Builder
	sb.WriteString(notice)
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
			// Chart blocks are pre-styled with ANSI by ntcharts/lipgloss and
			// must never be passed through renderMarkdown/glamour — chroma's
			// code-block highlighter would tokenize and re-color the raw
			// escape sequences, corrupting them.
			for _, chart := range turn.Charts {
				if m.width > 0 && m.width < chartWidth+4 {
					sb.WriteString(wrap(m.styles.Hint.Render(locale.T("agent.chart_too_narrow"))))
				} else {
					sb.WriteString(chart)
				}
				sb.WriteString("\n\n")
			}
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

	if m.infoMsg != "" {
		sb.WriteString("\n")
		sb.WriteString(wrap(m.styles.Hint.Render(fmt.Sprintf("✓ %s", m.infoMsg))))
	}

	if m.infoBlock != "" {
		sb.WriteString("\n")
		sb.WriteString(wrap(m.styles.Hint.Render(m.infoBlock)))
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
	inputRow := m.input.View()

	// Left side: normal help hint, with a persistent simulation-mode badge.
	var leftHint string
	switch {
	case m.showCmdPalette:
		leftHint = m.styles.Hint.Render(locale.T("agent.cmd.hint"))
	default:
		leftHint = m.styles.Hint.Render(locale.T("agent.hint"))
	}
	if m.simulationMode {
		leftHint = m.styles.Warning.Render(locale.T("menu.simulation_badge")) + "  " + leftHint
	}

	// Right side: inference widget (animation + timer, or just timer).
	var rightWidget string
	switch {
	case m.streaming:
		cancelTxt := m.styles.Hint.Render(locale.T("agent.cancel_hint"))
		timeTxt := m.styles.Hint.Render(formatInferenceTime(m.inferenceElapsed))
		rightWidget = cancelTxt + "  " + m.renderInferenceBoxes() + "  " + timeTxt
	case m.inferenceElapsed > 0:
		rightWidget = m.styles.Hint.Render(formatInferenceTime(m.inferenceElapsed))
	}

	// Compose the hint line: left + padding + right, falling back gracefully.
	hint := m.renderHintLine(leftHint, rightWidget)

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

// renderHintLine composes left and right strings into a single terminal-width
// line. If both fit with at least one space of padding between them, they are
// placed on the same line. If there is not enough room, only left is shown
// while streaming falls back to showing only the right widget.
func (m *AgentModel) renderHintLine(left, right string) string {
	if right == "" {
		return left
	}
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := m.width - leftW - rightW
	if gap >= 1 {
		return left + strings.Repeat(" ", gap) + right
	}
	// Not enough room: during streaming the cancel hint is more important.
	if m.streaming {
		return right
	}
	return left
}

// renderInferenceBoxes returns the 5-box progressive-fill animation string.
// Frame 0–5: boxes fill left to right. Frame 6–9: boxes empty left to right.
func (m *AgentModel) renderInferenceBoxes() string {
	boxes := make([]string, 5)
	for i := 0; i < 5; i++ {
		var filled bool
		if m.boxFrame <= 5 {
			filled = i < m.boxFrame
		} else {
			filled = i >= m.boxFrame-5
		}
		if filled {
			boxes[i] = m.styles.Spinner.Render("■")
		} else {
			boxes[i] = m.styles.Hint.Render("□")
		}
	}
	return strings.Join(boxes, " ")
}

// formatInferenceTime formats a duration as "0.0s" or "1:23.4s" for display.
func formatInferenceTime(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := d.Seconds() - float64(mins)*60
	return fmt.Sprintf("%d:%04.1fs", mins, secs)
}
