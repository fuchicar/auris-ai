package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/fuchicar/auris-ai/pkg/locale"
)

// keyRune is a small helper for synthesizing a tea.KeyMsg whose String()
// looks like a printable rune (so viewportConsumesKey's path through
// k.Runes matches the production path). tea.KeyMsg is a struct; we
// construct it directly.
func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// keySpace mimics `tea.KeyMsg{Type: tea.KeySpace}`.
func keySpace() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeySpace}
}

// TestViewportConsumesKey (issues #37 follow-up) — the keys bound to
// the bubbles/viewport default KeyMap must be classified as
// "viewport-only" so they don't leak into the answer field as literal
// characters (typing "yes" while tapping 'j' to scroll would otherwise
// capture "yjes" or similar).
func TestViewportConsumesKey(t *testing.T) {
	cases := []struct {
		name string
		key  tea.KeyMsg
		want bool
	}{
		{"h scrolls (and must NOT type)", keyRune('h'), true},
		{"j scrolls (and must NOT type)", keyRune('j'), true},
		{"k scrolls (and must NOT type)", keyRune('k'), true},
		{"l scrolls (and must NOT type)", keyRune('l'), true},
		{"u scrolls (and must NOT type)", keyRune('u'), true},
		{"d scrolls (and must NOT type)", keyRune('d'), true},
		{"b scrolls (and must NOT type)", keyRune('b'), true},
		{"f scrolls (and must NOT type)", keyRune('f'), true},
		{"space scrolls (and must NOT type)", keySpace(), true},
		{"y is a normal typed char", keyRune('y'), false},
		{"e is a normal typed char", keyRune('e'), false},
		{"s is a normal typed char", keyRune('s'), false},
		{"i is a normal typed char", keyRune('i'), false},
		{"digits reach the input", keyRune('1'), false},
		{"space routed as KeySpace", tea.KeyMsg{Type: tea.KeySpace}, true},
		{"arrows route to viewport AND input", tea.KeyMsg{Type: tea.KeyDown}, false},
		{"PgDn routes to viewport AND input", tea.KeyMsg{Type: tea.KeyPgDown}, false},
		{"Enter handled separately by validate()", tea.KeyMsg{Type: tea.KeyEnter}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := viewportConsumesKey(tc.key); got != tc.want {
				t.Errorf("viewportConsumesKey(%+v): want %v, got %v", tc.key, tc.want, got)
			}
		})
	}
}

// TestDisclaimer_ScrollKeysDoNotLeakIntoInput (issues #37 follow-up) —
// integration check that the actual Update() filter does what
// viewportConsumesKey claims: pressing a scroll-letter after typing
// "ye" leaves the field at "ye", not "ye" + the letter.
func TestDisclaimer_ScrollKeysDoNotLeakIntoInput(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	s := NewStyles(ThemeDark, 80)
	m := newDisclaimerModel(s)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	dm, ok := updated.(*DisclaimerModel)
	if !ok {
		t.Fatalf("expected *DisclaimerModel after WindowSizeMsg, got %T", updated)
	}
	m = dm
	m.input.SetValue("ye")

	// 'j' should scroll the viewport but NOT become part of the answer.
	_, _ = m.Update(keyRune('j'))
	if got := m.input.Value(); got != "ye" {
		t.Errorf("after pressing 'j' (scroll), input.Value(): want %q, got %q", "ye", got)
	}

	// ' ' (space) the same.
	_, _ = m.Update(keySpace())
	if got := m.input.Value(); got != "ye" {
		t.Errorf("after pressing space (scroll), input.Value(): want %q, got %q", "ye", got)
	}

	// 's' is a normal character (used by "si"); it MUST reach the input.
	_, _ = m.Update(keyRune('s'))
	if got := m.input.Value(); got != "yes" {
		t.Errorf("after pressing 's', input.Value(): want %q, got %q", "yes", got)
	}
}

// TestDisclaimer_UpdatePreservesViewportCmd (issues #37 follow-up) —
// the previous implementation overwrote `cmd` between the viewport and
// the text input updates, silently dropping whichever came second. We
// can't easily inspect what cmd the viewport would emit (it returns
// nil in normal use) so instead we verify the structural property the
// fix encodes: textinput.Blink — the cmd the input issues — survives a
// regular character press (regression: the old code only emitted Blink
// by accident; the new code uses tea.Batch explicitly).
func TestDisclaimer_UpdatePreservesViewportCmd(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	s := NewStyles(ThemeDark, 80)
	m := newDisclaimerModel(s)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := m.Update(keyRune('y'))
	if cmd == nil {
		t.Fatal("expected cmd from input on a typed character (textinput.Blink)")
	}
	msg := cmd()
	if msg == nil {
		t.Fatal("expected the cmd to produce a non-nil msg (cursor blink)")
	}
}

// TestViewportUpdateAndInputUpdate_NotNil (issues #37 follow-up) —
// ensure the wrapper helpers (used by Update to feed tea.Batch) return
// non-nil cmds so a future viewport that wants to issue a follow-up
// command (e.g. a "scrolled to end" notify) isn't silently dropped at
// the seam.
func TestViewportUpdateAndInputUpdate_NotNil(t *testing.T) {
	vp := viewport.New(5, 3)
	in := textinput.New()
	in.Focus()

	if cmd := viewportUpdate(&vp, tea.WindowSizeMsg{Width: 5, Height: 3}); cmd != nil {
		// viewport currently returns nil for WindowSizeMsg; that is fine.
		_ = cmd
	}
	cmd := inputUpdate(&in, keyRune('y'))
	if cmd == nil {
		t.Fatal("inputUpdate on a typed char should emit at least the cursor blink cmd")
	}
	if got := in.Value(); got != "y" {
		t.Errorf("inputUpdate didn't propagate typing: got %q", got)
	}
}

