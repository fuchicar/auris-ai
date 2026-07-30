package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// typeString feeds a string into the model one rune at a time, as bubbletea
// would from real keystrokes.
func typeIntoChangePassphrase(m *ChangePassphraseModel, s string) {
	for _, r := range s {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestChangePassphraseModel_WrongCurrentRejected(t *testing.T) {
	m := newChangePassphraseModel(NewStyles(ThemeDark, 0), "correct-horse", true)

	typeIntoChangePassphrase(m, "wrong-passphrase")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	typeIntoChangePassphrase(m, "new-passphrase-1")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	typeIntoChangePassphrase(m, "new-passphrase-1")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		if _, ok := cmd().(ScreenDoneMsg); ok {
			t.Fatal("expected no ScreenDoneMsg for a wrong current passphrase")
		}
	}
	if m.err == "" {
		t.Fatal("expected an error message to be set")
	}
	if m.step != 0 {
		t.Errorf("step = %d, want 0 (reset to current field)", m.step)
	}
	if m.current.Value() != "" || m.newPass.Value() != "" || m.confirm.Value() != "" {
		t.Error("expected all fields to be cleared after a failed validation")
	}
}

func TestChangePassphraseModel_NewTooShortRejected(t *testing.T) {
	m := newChangePassphraseModel(NewStyles(ThemeDark, 0), "correct-horse", true)

	typeIntoChangePassphrase(m, "correct-horse")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	typeIntoChangePassphrase(m, "short")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	typeIntoChangePassphrase(m, "short")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		if _, ok := cmd().(ScreenDoneMsg); ok {
			t.Fatal("expected no ScreenDoneMsg for a too-short new passphrase")
		}
	}
	if m.err == "" {
		t.Fatal("expected an error message to be set")
	}
	if m.step != 1 {
		t.Errorf("step = %d, want 1 (reset to new-passphrase field)", m.step)
	}
}

func TestChangePassphraseModel_MismatchRejected(t *testing.T) {
	m := newChangePassphraseModel(NewStyles(ThemeDark, 0), "correct-horse", true)

	typeIntoChangePassphrase(m, "correct-horse")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	typeIntoChangePassphrase(m, "new-passphrase-1")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	typeIntoChangePassphrase(m, "new-passphrase-2")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		if _, ok := cmd().(ScreenDoneMsg); ok {
			t.Fatal("expected no ScreenDoneMsg for mismatched new/confirm passphrases")
		}
	}
	if m.err == "" {
		t.Fatal("expected an error message to be set")
	}
	if m.step != 1 {
		t.Errorf("step = %d, want 1 (reset to new-passphrase field)", m.step)
	}
}

func TestChangePassphraseModel_SuccessEmitsResult(t *testing.T) {
	m := newChangePassphraseModel(NewStyles(ThemeDark, 0), "correct-horse", true)

	typeIntoChangePassphrase(m, "correct-horse")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	typeIntoChangePassphrase(m, "new-passphrase-1")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	typeIntoChangePassphrase(m, "new-passphrase-1")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("expected a command emitting ScreenDoneMsg, got nil")
	}
	msg, ok := cmd().(ScreenDoneMsg)
	if !ok {
		t.Fatalf("expected ScreenDoneMsg, got %T", msg)
	}
	if msg.From != ScreenChangePassphrase {
		t.Errorf("From = %v, want ScreenChangePassphrase", msg.From)
	}
	res, ok := msg.Result.(ChangePassphraseResult)
	if !ok {
		t.Fatalf("expected ChangePassphraseResult, got %T", msg.Result)
	}
	if res.NewPassphrase != "new-passphrase-1" {
		t.Errorf("NewPassphrase = %q, want %q", res.NewPassphrase, "new-passphrase-1")
	}
}

func TestChangePassphraseModel_EscCancels(t *testing.T) {
	m := newChangePassphraseModel(NewStyles(ThemeDark, 0), "correct-horse", true)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected a command emitting ScreenDoneMsg, got nil")
	}
	msg, ok := cmd().(ScreenDoneMsg)
	if !ok {
		t.Fatalf("expected ScreenDoneMsg, got %T", msg)
	}
	if msg.From != ScreenChangePassphrase {
		t.Errorf("From = %v, want ScreenChangePassphrase", msg.From)
	}
	if msg.Result != nil {
		t.Errorf("Result = %v, want nil (cancel)", msg.Result)
	}
}

func TestChangePassphraseModel_EscNoOpWhenCannotGoBack(t *testing.T) {
	m := newChangePassphraseModel(NewStyles(ThemeDark, 0), "correct-horse", false)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		if _, ok := cmd().(ScreenDoneMsg); ok {
			t.Fatal("Esc should not cancel when canGoBack is false")
		}
	}
}
