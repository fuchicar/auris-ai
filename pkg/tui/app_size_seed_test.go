package tui

import (
	"errors"
	"testing"

	"github.com/fuchicar/auris-ai/pkg/locale"
)

// Regression tests for issue #34: AppModel.Update must re-inject the known
// terminal size into any screen it swaps a.current to, since BubbleTea only
// sends tea.WindowSizeMsg once at startup (and on resize) — not every time a
// new screen model is created.

// TestUpdate_ScreenDoneMsg_SeedsSizeIntoNewScreen drives the real ScreenMenu
// -> "/news" command path (Update -> transition -> handleCommand), which
// swaps a.current to a fresh *NewsFeedsModel outside of any constructor that
// takes width/height. Before the fix, that screen's height field stayed 0
// (its maxVisible() fallback) until an actual terminal resize arrived.
func TestUpdate_ScreenDoneMsg_SeedsSizeIntoNewScreen(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	a := NewApp(AppOptions{})
	a.styles = NewStyles(ThemeDark)
	a.width, a.height = 100, 42
	a.screen = ScreenMenu
	a.current = newMenuModel(a.styles, false, false)

	updated, _ := a.Update(ScreenDoneMsg{From: ScreenMenu, Result: CommandResult{Cmd: "news"}})
	a = updated.(*AppModel)

	m, ok := a.current.(*NewsFeedsModel)
	if !ok {
		t.Fatalf("expected *NewsFeedsModel, got %T", a.current)
	}
	if m.height != a.height {
		t.Fatalf("NewsFeedsModel.height = %d, want %d (seeded from terminal size)", m.height, a.height)
	}
}

// TestUpdate_ModelsLoadedMsg_SeedsSizeIntoNewScreen covers the third
// a.current-swap site named in issue #34: the modelsLoadedMsg handler, which
// sets a.current directly rather than going through transition().
func TestUpdate_ModelsLoadedMsg_SeedsSizeIntoNewScreen(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	a := NewApp(AppOptions{})
	a.styles = NewStyles(ThemeDark)
	a.width, a.height = 100, 42
	a.pendingPortfolioCreate = true
	a.screen = ScreenPortfolioMenu
	a.current = newPortfolioMenuModel(a.styles, nil)

	updated, _ := a.Update(modelsLoadedMsg{})
	a = updated.(*AppModel)

	m, ok := a.current.(*portfolioCreateModel)
	if !ok {
		t.Fatalf("expected *portfolioCreateModel, got %T", a.current)
	}
	if m.height != a.height {
		t.Fatalf("portfolioCreateModel.height = %d, want %d (seeded from terminal size)", m.height, a.height)
	}
}

// TestUpdate_SameScreen_DoesNotReinjectSize is the control case: when a.current
// is not replaced, Update must not synthesize an extra WindowSizeMsg dispatch.
// modelsLoadedMsg's error branch surfaces an error on the *current* screen
// without swapping it — a NewsFeedsModel constructed with height already at
// its zero value should stay untouched by the defer (it only fires on an
// actual screen swap; sending it a WindowSizeMsg here would also be harmless,
// but this test pins the "no swap -> no-op" half of the contract instead).
func TestUpdate_SameScreen_DoesNotReinjectSize(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	a := NewApp(AppOptions{})
	a.styles = NewStyles(ThemeDark)
	a.width, a.height = 100, 42
	a.pendingPortfolioCreate = true
	a.screen = ScreenPortfolioMenu
	before := newPortfolioMenuModel(a.styles, nil)
	a.current = before

	updated, _ := a.Update(modelsLoadedMsg{err: errors.New("boom")})
	a = updated.(*AppModel)

	if a.current != before {
		t.Fatalf("expected a.current to remain the same *portfolioMenuModel instance")
	}
}
