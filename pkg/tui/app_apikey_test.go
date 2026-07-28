package tui

import (
	"testing"

	"github.com/fuchicar/auris-ai/pkg/config"
)

// TestTransition_APIKeyEscInMarketProvidersFlow_ReturnsToChecklist verifies
// issue #26: Esc from the API-key screen opened by /marketproviders must
// return to the provider checklist instead of being silently treated as
// "provider configured, advance the queue."
func TestTransition_APIKeyEscInMarketProvidersFlow_ReturnsToChecklist(t *testing.T) {
	a := &AppModel{
		cfg:                     &config.AurisConfig{Providers: map[string]*config.ProviderConfig{}},
		screen:                  ScreenAPIKey,
		flowContext:             FlowMenu,
		styles:                  NewStyles(ThemeDark),
		managingMarketProviders: true,
		pendingMarketProviders:  []string{"fmp"},
		pendingMarketIdx:        0,
	}

	updated, _ := a.transition(ScreenDoneMsg{From: ScreenAPIKey, Result: nil})
	got := updated.(*AppModel)

	if got.screen != ScreenMarketProviderManage {
		t.Errorf("screen = %v, want ScreenMarketProviderManage", got.screen)
	}
	if !got.managingMarketProviders {
		t.Error("managingMarketProviders should stay true (still mid-management)")
	}
	if got.pendingMarketProviders != nil {
		t.Errorf("pendingMarketProviders = %v, want nil (queue abandoned)", got.pendingMarketProviders)
	}
	if _, ok := got.current.(*MarketProviderManageModel); !ok {
		t.Errorf("current = %T, want *MarketProviderManageModel", got.current)
	}
}

// TestTransition_APIKeySuccessInMarketProvidersFlow_AdvancesQueue is the
// control case: a real APIKeyResult must still advance pendingMarketIdx as
// before, unaffected by the Esc-handling fix above.
func TestTransition_APIKeySuccessInMarketProvidersFlow_AdvancesQueue(t *testing.T) {
	a := &AppModel{
		cfg:                     &config.AurisConfig{Providers: map[string]*config.ProviderConfig{}},
		screen:                  ScreenAPIKey,
		flowContext:             FlowMenu,
		styles:                  NewStyles(ThemeDark),
		managingMarketProviders: true,
		pendingMarketProviders:  []string{"fmp", "eodhd"},
		pendingMarketIdx:        0,
	}

	entry, ok := findMarketEntry("fmp")
	if !ok {
		t.Fatal("findMarketEntry(fmp) failed")
	}

	updated, _ := a.transition(ScreenDoneMsg{
		From:   ScreenAPIKey,
		Result: APIKeyResult{Entry: entry, APIKey: "some-key"},
	})
	got := updated.(*AppModel)

	if got.pendingMarketIdx != 1 {
		t.Errorf("pendingMarketIdx = %d, want 1", got.pendingMarketIdx)
	}
	if got.screen != ScreenAPIKey {
		t.Errorf("screen = %v, want ScreenAPIKey (still advancing queue)", got.screen)
	}
	if _, ok := got.cfg.Providers["fmp"]; !ok {
		t.Error("expected fmp to be saved to cfg.Providers")
	}
	m, ok := got.current.(*APIKeyModel)
	if !ok {
		t.Fatalf("current = %T, want *APIKeyModel", got.current)
	}
	if !m.canGoBack {
		t.Error("continuation APIKeyModel should have canGoBack=true so Esc works on every queue step")
	}
}
