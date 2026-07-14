package tui

import "testing"

func TestWizardStep_NotInSetupFlow(t *testing.T) {
	a := NewApp(AppOptions{})
	a.flowContext = FlowMenu
	a.screen = ScreenTheme
	if _, _, ok := a.wizardStep(); ok {
		t.Fatal("expected ok=false when flowContext is not FlowSetup")
	}
}

func TestWizardStep_ScreenNotInWizard(t *testing.T) {
	a := NewApp(AppOptions{})
	a.screen = ScreenMenu
	if _, _, ok := a.wizardStep(); ok {
		t.Fatal("expected ok=false for a screen outside the wizard")
	}
}

func TestWizardStep_TotalShrinksInSimulationMode(t *testing.T) {
	a := NewApp(AppOptions{})
	a.screen = ScreenDataMode

	_, totalReal, ok := a.wizardStep()
	if !ok {
		t.Fatal("expected ok=true")
	}

	a.cfg.SimulationMode = true
	_, totalSim, ok := a.wizardStep()
	if !ok {
		t.Fatal("expected ok=true")
	}

	if totalSim >= totalReal {
		t.Errorf("total in simulation mode (%d) should be less than in real-provider mode (%d)", totalSim, totalReal)
	}
}

func TestWizardStep_LocaleAddsAStep(t *testing.T) {
	a := NewApp(AppOptions{})
	a.screen = ScreenTheme

	_, totalWithout, ok := a.wizardStep()
	if !ok {
		t.Fatal("expected ok=true")
	}

	a.showLocaleSelect = true
	_, totalWith, ok := a.wizardStep()
	if !ok {
		t.Fatal("expected ok=true")
	}

	if totalWith != totalWithout+1 {
		t.Errorf("total = %d, want %d (showLocaleSelect should add exactly one step)", totalWith, totalWithout+1)
	}
}

// TestWizardStep_CurrentAdvancesInOrder walks the default (real-provider,
// no-locale-prompt) wizard path and checks the reported current step
// increases by exactly one at each screen, in the order AppModel.transition
// actually navigates them.
func TestWizardStep_CurrentAdvancesInOrder(t *testing.T) {
	a := NewApp(AppOptions{})
	order := []Screen{
		ScreenDisclaimer,
		ScreenTheme,
		ScreenPassphrase,
		ScreenProfile,
		ScreenDataMode,
		ScreenProvider,
		ScreenAPIKey,
		ScreenAPIKeySecondary,
		ScreenAIProviderSelect,
		ScreenAIProviderConfig,
		ScreenAIDefaultModel,
	}
	var last int
	for _, scr := range order {
		a.screen = scr
		current, total, ok := a.wizardStep()
		if !ok {
			t.Fatalf("screen %v: expected ok=true", scr)
		}
		if current != last+1 {
			t.Errorf("screen %v: current = %d, want %d", scr, current, last+1)
		}
		if total < current {
			t.Errorf("screen %v: total (%d) < current (%d)", scr, total, current)
		}
		last = current
	}
}
