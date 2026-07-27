package tui

import (
	"testing"

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
