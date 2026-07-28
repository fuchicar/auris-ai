package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fuchicar/auris-ai/pkg/config"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

// setupPortfolioAgentTest sandboxes portfolio and session storage in temp
// directories and returns a freshly-saved portfolio to exercise against.
func setupPortfolioAgentTest(t *testing.T) *portfolio.Portfolio {
	t.Helper()
	prev := portfolio.SetPortfoliosDirForTest(t.TempDir())
	t.Cleanup(func() { portfolio.SetPortfoliosDirForTest(prev) })
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	p := portfolio.NewPortfolio("Test Portfolio")
	p.Cash = 1000
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatalf("SavePortfolio: %v", err)
	}
	return p
}

// simulateAgentToolWrite mimics an agent portfolio tool (pkg/agent/tools.go)
// mutating the portfolio and saving it directly to disk, bypassing whatever
// in-memory *portfolio.Portfolio pointer the TUI is holding.
func simulateAgentToolWrite(t *testing.T, id string, cash float64) {
	t.Helper()
	fresh, err := portfolio.LoadPortfolio(id)
	if err != nil {
		t.Fatalf("LoadPortfolio: %v", err)
	}
	fresh.Cash = cash
	if err := portfolio.SavePortfolio(fresh); err != nil {
		t.Fatalf("SavePortfolio: %v", err)
	}
}

func assertPersistedCash(t *testing.T, id string, want float64) {
	t.Helper()
	got, err := portfolio.LoadPortfolio(id)
	if err != nil {
		t.Fatalf("LoadPortfolio: %v", err)
	}
	if got.Cash != want {
		t.Errorf("persisted Cash = %v, want %v (agent tool write must survive)", got.Cash, want)
	}
}

func TestHandleAgentCommand_NewPreservesToolWrites(t *testing.T) {
	p := setupPortfolioAgentTest(t)
	simulateAgentToolWrite(t, p.ID, 500)

	a := &AppModel{
		cfg:             &config.AurisConfig{},
		flowContext:     FlowPortfolio,
		activePortfolio: p, // stale: still holds Cash=1000
	}
	a.handleAgentCommand(CommandResult{Cmd: "new"})

	assertPersistedCash(t, p.ID, 500)
}

func TestTransition_SessionSelectPreservesToolWrites(t *testing.T) {
	p := setupPortfolioAgentTest(t)

	// Establish an existing session to select.
	s := config.NewSession()
	s.PortfolioID = p.ID
	if err := config.SaveSession(s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	simulateAgentToolWrite(t, p.ID, 500)

	a := &AppModel{
		cfg:             &config.AurisConfig{},
		flowContext:     FlowPortfolio,
		activePortfolio: p, // stale: still holds Cash=1000
	}
	a.transition(ScreenDoneMsg{From: ScreenSessionSelect, Result: SessionSelectResult{ID: s.ID}})

	assertPersistedCash(t, p.ID, 500)
}

// TestToggleMode_FromPortfolioAgent_ReturnsToPortfolioView verifies issue #27:
// Shift+Tab (toggleMode) leaving portfolio-agent mode must return to the
// portfolio dashboard (not the root menu) and must reset flowContext away
// from FlowPortfolio, so a subsequent global agent session doesn't get
// mistaken for a portfolio-scoped one.
func TestToggleMode_FromPortfolioAgent_ReturnsToPortfolioView(t *testing.T) {
	p := setupPortfolioAgentTest(t)

	a := &AppModel{
		cfg:             &config.AurisConfig{},
		screen:          ScreenAgent,
		flowContext:     FlowPortfolio,
		activePortfolio: p,
		styles:          NewStyles(ThemeDark),
	}
	updated, _ := a.toggleMode()
	got := updated.(*AppModel)

	if got.screen != ScreenPortfolioView {
		t.Errorf("screen = %v, want ScreenPortfolioView", got.screen)
	}
	if got.flowContext != FlowMenu {
		t.Errorf("flowContext = %v, want FlowMenu (must not stay FlowPortfolio)", got.flowContext)
	}
}

// TestToggleMode_FromGlobalAgent_ReturnsToMenu verifies toggleMode's normal
// (non-portfolio) behavior is unchanged.
func TestToggleMode_FromGlobalAgent_ReturnsToMenu(t *testing.T) {
	a := &AppModel{
		cfg:         &config.AurisConfig{},
		screen:      ScreenAgent,
		flowContext: FlowAgent,
		styles:      NewStyles(ThemeDark),
	}
	updated, _ := a.toggleMode()
	got := updated.(*AppModel)

	if got.screen != ScreenMenu {
		t.Errorf("screen = %v, want ScreenMenu", got.screen)
	}
	if got.flowContext != FlowMenu {
		t.Errorf("flowContext = %v, want FlowMenu", got.flowContext)
	}
}

// TestTransition_ScreenAgentNil_FromPortfolioAgent_ReturnsToPortfolioView
// verifies the Esc/quit-to-menu path (transition's ScreenAgent/nil branch)
// has the same fix as toggleMode.
func TestTransition_ScreenAgentNil_FromPortfolioAgent_ReturnsToPortfolioView(t *testing.T) {
	p := setupPortfolioAgentTest(t)

	a := &AppModel{
		cfg:             &config.AurisConfig{},
		screen:          ScreenAgent,
		flowContext:     FlowPortfolio,
		activePortfolio: p,
		styles:          NewStyles(ThemeDark),
	}
	updated, _ := a.transition(ScreenDoneMsg{From: ScreenAgent, Result: nil})
	got := updated.(*AppModel)

	if got.screen != ScreenPortfolioView {
		t.Errorf("screen = %v, want ScreenPortfolioView", got.screen)
	}
	if got.flowContext != FlowMenu {
		t.Errorf("flowContext = %v, want FlowMenu (must not stay FlowPortfolio)", got.flowContext)
	}
}

// TestHandleAgentCommand_AfterPortfolioExit_MenuBehavesAsGlobal is the direct
// regression test for issue #27: once flowContext has been reset to FlowMenu
// on exit (activePortfolio is sticky and stays non-nil by design), /menu from
// a subsequent global agent session must go to the root menu, not the stale
// portfolio's dashboard.
func TestHandleAgentCommand_AfterPortfolioExit_MenuBehavesAsGlobal(t *testing.T) {
	p := setupPortfolioAgentTest(t)

	a := &AppModel{
		cfg:             &config.AurisConfig{},
		flowContext:     FlowMenu, // reset after leaving portfolio-agent mode
		activePortfolio: p,        // sticky: still set from the earlier portfolio visit
		styles:          NewStyles(ThemeDark),
	}
	updated, _ := a.handleAgentCommand(CommandResult{Cmd: "menu"})
	got := updated.(*AppModel)

	if got.screen != ScreenMenu {
		t.Errorf("screen = %v, want ScreenMenu (global /menu must not route to the stale portfolio)", got.screen)
	}
}

// TestHandleAgentCommand_AfterPortfolioExit_SessionBehavesAsGlobal is the
// /session counterpart of the test above.
func TestHandleAgentCommand_AfterPortfolioExit_SessionBehavesAsGlobal(t *testing.T) {
	p := setupPortfolioAgentTest(t)

	globalSession := config.NewSession()
	if err := config.SaveSession(globalSession); err != nil {
		t.Fatalf("SaveSession (global): %v", err)
	}
	portfolioSession := config.NewSession()
	portfolioSession.PortfolioID = p.ID
	if err := config.SaveSession(portfolioSession); err != nil {
		t.Fatalf("SaveSession (portfolio): %v", err)
	}
	p.ActiveSessionID = portfolioSession.ID
	if err := portfolio.SavePortfolio(p); err != nil {
		t.Fatalf("SavePortfolio: %v", err)
	}

	a := &AppModel{
		cfg: &config.AurisConfig{
			ActiveSessionID: globalSession.ID,
		},
		flowContext:     FlowMenu, // reset after leaving portfolio-agent mode
		activePortfolio: p,        // sticky: still set from the earlier portfolio visit
		styles:          NewStyles(ThemeDark),
	}
	updated, _ := a.handleAgentCommand(CommandResult{Cmd: "session"})
	got := updated.(*AppModel)

	sel, ok := got.current.(*SessionSelectModel)
	if !ok {
		t.Fatalf("current = %T, want *SessionSelectModel", got.current)
	}
	if sel.currentID != globalSession.ID {
		t.Errorf("currentID = %q, want %q (global session, not the portfolio's)", sel.currentID, globalSession.ID)
	}
}

// TestUpdate_ShiftTabDuringStreaming_DoesNotToggle verifies issue #25:
// the global Shift+Tab intercept in AppModel.Update must not discard the
// AgentModel (and its in-flight LLM call) while streaming is true.
func TestUpdate_ShiftTabDuringStreaming_DoesNotToggle(t *testing.T) {
	am := &AgentModel{streaming: true}
	a := &AppModel{
		cfg:     &config.AurisConfig{},
		screen:  ScreenAgent,
		styles:  NewStyles(ThemeDark),
		current: am,
	}

	updated, _ := a.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	got := updated.(*AppModel)

	if got.screen != ScreenAgent {
		t.Errorf("screen = %v, want ScreenAgent (Shift+Tab must be swallowed while streaming)", got.screen)
	}
	if got.current != am {
		t.Errorf("current model was replaced; AgentModel must survive Shift+Tab while streaming")
	}
}

// TestUpdate_ShiftTabNotStreaming_TogglesToMenu verifies the normal
// (non-streaming) Shift+Tab behavior is unchanged.
func TestUpdate_ShiftTabNotStreaming_TogglesToMenu(t *testing.T) {
	am := &AgentModel{streaming: false}
	a := &AppModel{
		cfg:         &config.AurisConfig{},
		screen:      ScreenAgent,
		flowContext: FlowAgent,
		styles:      NewStyles(ThemeDark),
		current:     am,
	}

	updated, _ := a.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	got := updated.(*AppModel)

	if got.screen != ScreenMenu {
		t.Errorf("screen = %v, want ScreenMenu", got.screen)
	}
}

func TestResolvePortfolioSession_ReturnsFreshPortfolio(t *testing.T) {
	p := setupPortfolioAgentTest(t)
	simulateAgentToolWrite(t, p.ID, 500)

	a := &AppModel{
		cfg:             &config.AurisConfig{},
		activePortfolio: p, // stale: still holds Cash=1000
	}
	got, _ := a.resolvePortfolioSession(p)

	if got.Cash != 500 {
		t.Errorf("resolvePortfolioSession returned Cash = %v, want 500 (fresh from disk)", got.Cash)
	}
	if a.activePortfolio.Cash != 500 {
		t.Errorf("a.activePortfolio.Cash = %v, want 500 (should be synced to fresh copy)", a.activePortfolio.Cash)
	}
	assertPersistedCash(t, p.ID, 500)
}
