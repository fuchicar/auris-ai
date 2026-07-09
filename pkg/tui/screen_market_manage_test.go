package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"auris/pkg/locale"
	"auris/pkg/registry"
)

func testMarketEntries() []registry.MarketEntry {
	return []registry.MarketEntry{
		{Key: "fmp", DisplayName: "Financial Modeling Prep"},
		{Key: "eodhd", DisplayName: "EODHD"},
	}
}

func TestMarketProviderManageModel_ToggleSelection(t *testing.T) {
	m := newMarketProviderManageModel(NewStyles(ThemeDark), testMarketEntries(), map[string]bool{"fmp": true}, true)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(*MarketProviderManageModel)
	if m.selected["fmp"] {
		t.Error("Space on fmp (cursor 0) should have deselected it")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(*MarketProviderManageModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(*MarketProviderManageModel)
	if !m.selected["eodhd"] {
		t.Error("Space on eodhd (cursor 1) should have selected it")
	}
}

func TestMarketProviderManageModel_MoveUp(t *testing.T) {
	m := newMarketProviderManageModel(NewStyles(ThemeDark), testMarketEntries(), nil, true)

	// Cursor starts at 0 (fmp); move down to eodhd, then move it up.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(*MarketProviderManageModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+")})
	m = updated.(*MarketProviderManageModel)

	if m.entries[0].Key != "eodhd" || m.entries[1].Key != "fmp" {
		t.Errorf("expected [eodhd, fmp] after moving eodhd up, got [%s, %s]", m.entries[0].Key, m.entries[1].Key)
	}
	if m.cursor != 0 {
		t.Errorf("cursor should follow the moved entry to index 0, got %d", m.cursor)
	}
}

func TestMarketProviderManageModel_MoveDown(t *testing.T) {
	m := newMarketProviderManageModel(NewStyles(ThemeDark), testMarketEntries(), nil, true)

	// Cursor starts at 0 (fmp); move it down.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-")})
	m = updated.(*MarketProviderManageModel)

	if m.entries[0].Key != "eodhd" || m.entries[1].Key != "fmp" {
		t.Errorf("expected [eodhd, fmp] after moving fmp down, got [%s, %s]", m.entries[0].Key, m.entries[1].Key)
	}
	if m.cursor != 1 {
		t.Errorf("cursor should follow the moved entry to index 1, got %d", m.cursor)
	}
}

func TestMarketProviderManageModel_MoveUpAtTop_NoOp(t *testing.T) {
	m := newMarketProviderManageModel(NewStyles(ThemeDark), testMarketEntries(), nil, true)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+")})
	m = updated.(*MarketProviderManageModel)
	if m.entries[0].Key != "fmp" || m.entries[1].Key != "eodhd" {
		t.Error("moving the top entry up should be a no-op")
	}
}

func TestMarketProviderManageModel_EnterEmitsOrderAndSelection(t *testing.T) {
	m := newMarketProviderManageModel(NewStyles(ThemeDark), testMarketEntries(), map[string]bool{"fmp": true}, true)

	// Reorder: move eodhd (cursor down) to the top.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(*MarketProviderManageModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+")})
	m = updated.(*MarketProviderManageModel)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command emitting ScreenDoneMsg, got nil")
	}
	msg, ok := cmd().(ScreenDoneMsg)
	if !ok {
		t.Fatalf("expected ScreenDoneMsg, got %T", msg)
	}
	if msg.From != ScreenMarketProviderManage {
		t.Errorf("From = %v, want ScreenMarketProviderManage", msg.From)
	}
	res, ok := msg.Result.(MarketProviderManageResult)
	if !ok {
		t.Fatalf("expected MarketProviderManageResult, got %T", msg.Result)
	}
	if len(res.Order) != 2 || res.Order[0] != "eodhd" || res.Order[1] != "fmp" {
		t.Errorf("Order = %v, want [eodhd fmp]", res.Order)
	}
	if len(res.Selected) != 1 || res.Selected[0] != "fmp" {
		t.Errorf("Selected = %v, want [fmp]", res.Selected)
	}
}

func TestMarketProviderManageModel_EscCancels(t *testing.T) {
	m := newMarketProviderManageModel(NewStyles(ThemeDark), testMarketEntries(), nil, true)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command emitting ScreenDoneMsg, got nil")
	}
	msg, ok := cmd().(ScreenDoneMsg)
	if !ok {
		t.Fatalf("expected ScreenDoneMsg, got %T", msg)
	}
	if msg.Result != nil {
		t.Errorf("Result = %v, want nil (cancelled)", msg.Result)
	}
}

func TestMarketProviderManageModel_EscNoOpWhenCannotGoBack(t *testing.T) {
	m := newMarketProviderManageModel(NewStyles(ThemeDark), testMarketEntries(), nil, false)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		if _, ok := cmd().(ScreenDoneMsg); ok {
			t.Fatal("Esc should not cancel when canGoBack is false")
		}
	}
}

func TestMarketProviderManageModel_View(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	m := newMarketProviderManageModel(NewStyles(ThemeDark), testMarketEntries(), map[string]bool{"fmp": true}, true)
	got := m.View()
	if got == "" {
		t.Fatal("View() returned empty string")
	}
}
