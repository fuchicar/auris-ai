package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"auris/pkg/registry"
)

func TestAPIKeyModel_OptionalEscSkips(t *testing.T) {
	entry := registry.MarketEntry{Key: "eodhd", DisplayName: "EODHD"}
	m := newAPIKeyModel(entry, NewStyles(ThemeDark), ScreenAPIKeySecondary, true)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command emitting ScreenDoneMsg, got nil")
	}
	msg, ok := cmd().(ScreenDoneMsg)
	if !ok {
		t.Fatalf("expected ScreenDoneMsg, got %T", msg)
	}
	if msg.From != ScreenAPIKeySecondary {
		t.Errorf("From = %v, want ScreenAPIKeySecondary", msg.From)
	}
	res, ok := msg.Result.(APIKeyResult)
	if !ok {
		t.Fatalf("expected APIKeyResult, got %T", msg.Result)
	}
	if res.APIKey != "" {
		t.Errorf("APIKey = %q, want empty (skipped)", res.APIKey)
	}
	if res.Entry.Key != "eodhd" {
		t.Errorf("Entry.Key = %q, want %q", res.Entry.Key, "eodhd")
	}
}

func TestAPIKeyModel_MandatoryEscDoesNothing(t *testing.T) {
	entry := registry.MarketEntry{Key: "fmp", DisplayName: "FMP"}
	m := newAPIKeyModel(entry, NewStyles(ThemeDark), ScreenAPIKey, false)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		if _, ok := cmd().(ScreenDoneMsg); ok {
			t.Fatal("Esc should not skip the mandatory primary-provider screen")
		}
	}
}

func TestAPIKeyModel_OptionalView_ShowsSkipHint(t *testing.T) {
	entry := registry.MarketEntry{Key: "eodhd", DisplayName: "EODHD"}
	m := newAPIKeyModel(entry, NewStyles(ThemeDark), ScreenAPIKeySecondary, true)
	if got := m.View(); got == "" {
		t.Fatal("View() returned empty string")
	}
}
