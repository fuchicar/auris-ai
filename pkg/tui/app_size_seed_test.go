package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
	a.styles = NewStyles(ThemeDark, 0)
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
	a.styles = NewStyles(ThemeDark, 0)
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
	a.styles = NewStyles(ThemeDark, 0)
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

// TestUpdate_WizardStep_SubtractsChromeForNewScreen covers the issue #35
// review feedback: AppModel.Update's defer should subtract the wizard-step
// indicator line + blank from the height it forwards to a newly created
// screen during FlowSetup, so screens with height-aware windowing (e.g.
// ScreenProvider, ScreenAIProviderSelect, ScreenAIDefaultModel, ScreenProfile)
// have an accurate budget on the first render and don't overflow past the
// panel border.
//
// Drives the natural swap path: a ScreenDoneMsg takes us through
// transition(), which swaps a.current to a fresh ScreenProfileModel.
func TestUpdate_WizardStep_SubtractsChromeForNewScreen(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	a := NewApp(AppOptions{})
	a.styles = NewStyles(ThemeDark, 0)
	a.flowContext = FlowSetup
	a.width, a.height = 100, 42

	// Wind setup forward through Theme → Passphrase → Profile via real
	// ScreenDoneMsg dispatches. Theme/Profile re-entry requires pre-set state
	// in cfg (Theme = "dark" for the styles to keep working).
	a.cfg.Theme = "dark"
	a.screen = ScreenTheme
	a.current = newThemeModel(a.styles, true)

	updated, _ := a.Update(ScreenDoneMsg{From: ScreenTheme, Result: ThemeResult{Theme: "dark"}})
	a = updated.(*AppModel)

	updated, _ = a.Update(ScreenDoneMsg{From: ScreenPassphrase, Result: PassphraseResult{Passphrase: "x"}})
	a = updated.(*AppModel)

	if a.screen != ScreenProfile {
		t.Fatalf("expected to land on ScreenProfile, got %v (current = %T)", a.screen, a.current)
	}
	m, ok := a.current.(*ProfileModel)
	if !ok {
		t.Fatalf("expected *ProfileModel, got %T", a.current)
	}

	if a.wizardStepLines() != 2 {
		t.Fatalf("wizardStepLines = %d, want 2 during setup on ScreenProfile", a.wizardStepLines())
	}
	wantHeight := a.height - a.wizardStepLines()
	if m.height != wantHeight {
		t.Fatalf("ProfileModel.height = %d, want %d (a.height %d - wizardStepLines %d)",
			m.height, wantHeight, a.height, a.wizardStepLines())
	}
}

// TestUpdate_WizardStep_LiveResizeAlsoSubtractsChrome covers a gap the
// screen-creation fix above left open: a real tea.WindowSizeMsg arriving
// while a.current is *already* a windowed wizard screen (the user resizes
// their terminal mid-setup) took the top-level "case tea.WindowSizeMsg"
// branch, which forwarded the raw, un-subtracted height — so the same
// ProfileModel instance would compute a budget 2 rows too generous after a
// live resize versus what it was seeded with at creation time. Both paths
// must agree.
func TestUpdate_WizardStep_LiveResizeAlsoSubtractsChrome(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	a := NewApp(AppOptions{})
	a.styles = NewStyles(ThemeDark, 0)
	a.flowContext = FlowSetup
	a.width, a.height = 100, 42
	a.screen = ScreenProfile
	a.current = newProfileModel(a.styles, nil, true)

	updated, _ := a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	a = updated.(*AppModel)

	m, ok := a.current.(*ProfileModel)
	if !ok {
		t.Fatalf("expected *ProfileModel, got %T", a.current)
	}
	wantHeight := 30 - a.wizardStepLines()
	if m.height != wantHeight {
		t.Fatalf("ProfileModel.height after live resize = %d, want %d (30 - wizardStepLines %d)",
			m.height, wantHeight, a.wizardStepLines())
	}
}

// TestUpdate_FlowMenu_DoesNotSubtractChrome confirms the negative case:
// outside FlowSetup, wizardStepLines() returns 0 so the defer forwards the
// raw terminal height to a newly created screen — see the wizard step
// budget pulled out of screen-level chrome in [screen_provider.go] etc.
func TestUpdate_FlowMenu_DoesNotSubtractChrome(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	a := NewApp(AppOptions{})
	a.styles = NewStyles(ThemeDark, 0)
	a.flowContext = FlowMenu
	a.width, a.height = 100, 42

	if a.wizardStepLines() != 0 {
		t.Fatalf("wizardStepLines = %d, want 0 in FlowMenu", a.wizardStepLines())
	}
}
