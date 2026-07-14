package tui

import (
	"strings"
	"testing"

	"auris/pkg/config"
)

// TestRenderHistory_ToolLogs_ShowsConcreteCall exercises the actual render path
// (not just formatToolCall in isolation): a tool log line produced the way
// Update's agentProgressMsg case produces it must show up, ⚙-prefixed, in the
// rendered turn it belongs to. This covers the FEAT-21 regression where
// portfolio_ tool calls rendered nothing at all.
func TestRenderHistory_ToolLogs_ShowsConcreteCall(t *testing.T) {
	m := &AgentModel{
		styles: NewStyles(ThemeDark),
		width:  100,
		session: &config.Session{
			History: []config.ChatTurn{
				{Role: "user", Content: "add 10 AAPL at 150 to my portfolio"},
				{Role: "assistant", Content: "Done."},
			},
		},
		toolLogs:     []string{formatToolCall("portfolio_add_lot", `{"symbol":"AAPL","quantity":10,"price":150}`)},
		toolLogsTurn: 0,
	}
	out := m.renderHistory()
	want := "⚙ portfolio_add_lot(price=150, quantity=10, symbol=AAPL)"
	if !strings.Contains(out, want) {
		t.Errorf("rendered history missing tool call line %q, got:\n%s", want, out)
	}
}
