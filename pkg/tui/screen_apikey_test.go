package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fuchicar/auris-ai/pkg/registry"
)

func TestAPIKeyModel_OptionalEscSkips(t *testing.T) {
	entry := registry.MarketEntry{Key: "eodhd", DisplayName: "EODHD"}
	m := newAPIKeyModel(entry, NewStyles(ThemeDark), ScreenAPIKeySecondary, true, false)

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
	m := newAPIKeyModel(entry, NewStyles(ThemeDark), ScreenAPIKey, false, false)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		if _, ok := cmd().(ScreenDoneMsg); ok {
			t.Fatal("Esc should not skip the mandatory primary-provider screen when canGoBack is false")
		}
	}
}

func TestAPIKeyModel_CanGoBackEscGoesBack(t *testing.T) {
	entry := registry.MarketEntry{Key: "fmp", DisplayName: "FMP"}
	m := newAPIKeyModel(entry, NewStyles(ThemeDark), ScreenAPIKey, false, true)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command emitting ScreenDoneMsg, got nil")
	}
	msg, ok := cmd().(ScreenDoneMsg)
	if !ok {
		t.Fatalf("expected ScreenDoneMsg, got %T", msg)
	}
	if msg.From != ScreenAPIKey {
		t.Errorf("From = %v, want ScreenAPIKey", msg.From)
	}
	if msg.Result != nil {
		t.Errorf("Result = %v, want nil (go back)", msg.Result)
	}
}

func TestAPIKeyModel_OptionalView_ShowsSkipHint(t *testing.T) {
	entry := registry.MarketEntry{Key: "eodhd", DisplayName: "EODHD"}
	m := newAPIKeyModel(entry, NewStyles(ThemeDark), ScreenAPIKeySecondary, true, false)
	if got := m.View(); got == "" {
		t.Fatal("View() returned empty string")
	}
}

func TestAPIKeyModel_CanGoBackEscWorksWhileConnecting(t *testing.T) {
	entry := registry.MarketEntry{Key: "fmp", DisplayName: "FMP"}
	m := newAPIKeyModel(entry, NewStyles(ThemeDark), ScreenAPIKey, false, true)
	m.input.SetValue("some-key")

	// Start a connect attempt (Enter), putting the model in m.connecting.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Fatal("expected a command starting the connect attempt")
	}
	if !m.connecting {
		t.Fatal("expected m.connecting to be true after Enter")
	}

	// Esc must still be honored immediately, without waiting for the
	// in-flight connect to resolve.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command emitting ScreenDoneMsg, got nil")
	}
	msg, ok := cmd().(ScreenDoneMsg)
	if !ok {
		t.Fatalf("expected ScreenDoneMsg, got %T", msg)
	}
	if msg.Result != nil {
		t.Errorf("Result = %v, want nil (go back)", msg.Result)
	}
	if m.connecting {
		t.Error("m.connecting should be false after Esc")
	}
}

func TestAPIKeyModel_StaleConnectResultIgnoredAfterEsc(t *testing.T) {
	entry := registry.MarketEntry{Key: "fmp", DisplayName: "FMP"}
	m := newAPIKeyModel(entry, NewStyles(ThemeDark), ScreenAPIKey, false, true)
	m.input.SetValue("some-key")

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	startedGen := m.connectGen

	// Esc bumps connectGen, invalidating the in-flight attempt.
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.connectGen == startedGen {
		t.Fatal("expected connectGen to change after Esc")
	}

	// The stale result (tagged with the pre-Esc generation) must be ignored.
	updated, cmd := m.Update(connectResultMsg{err: nil, gen: startedGen})
	if cmd != nil {
		t.Error("expected no command for a stale connectResultMsg")
	}
	got := updated.(*APIKeyModel)
	if got.connecting {
		t.Error("stale result must not flip m.connecting")
	}
	if got.err != "" {
		t.Errorf("stale result must not set m.err, got %q", got.err)
	}
}
