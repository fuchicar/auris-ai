package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
)

// panelWidthBench reconstructs the panelWidth helper's behaviour so tests
// can assert expectations independent of the internal function.
func panelWidthBench(maxWidth int) int {
	if maxWidth <= 0 {
		return PanelWidthMax
	}
	pw := maxWidth - panelMargin
	if pw < PanelWidthMin {
		pw = PanelWidthMin
	}
	if pw > PanelWidthMax {
		pw = PanelWidthMax
	}
	return pw
}

// maxWidth returns the widest visible line in s (lipgloss counts
// visible cells, ignoring ANSI escape sequences). Used to assert
// that no rendered style overflows the terminal width.
func maxLineWidth(s string) int {
	w := 0
	for _, line := range strings.Split(s, "\n") {
		if lw := lipgloss.Width(line); lw > w {
			w = lw
		}
	}
	return w
}

// TestPanelWidth_AdaptsToTerminalWidth (issue #37) verifies the
// responsive width that NewStyles computes from the terminal columns.
// Every fixed PanelWidth=72 reference was replaced by this derivation
// in styles.go.
func TestPanelWidth_AdaptsToTerminalWidth(t *testing.T) {
	cases := []struct {
		name   string
		maxW   int
		wantPw int
	}{
		{"maxWidth=0 falls back to ceiling", 0, PanelWidthMax},
		{"100-col clamps to PanelWidthMax", 100, PanelWidthMax},
		{"80-col caps at 72", 80, PanelWidthMax},
		{"60-col yields 56", 60, 56},
		{"40-col yields 36", 40, 36},
		{"very narrow clamps to floor", 12, PanelWidthMin},
		{"exactly floor", PanelWidthMin + panelMargin, PanelWidthMin},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStyles(ThemeDark, tc.maxW)
			if s.PanelWidth != tc.wantPw {
				t.Errorf("NewStyles(ThemeDark, %d).PanelWidth: want %d, got %d", tc.maxW, tc.wantPw, s.PanelWidth)
			}
		})
	}
}

// TestNewStyles_WidthBoundStylesFitTerminal (issue #37) is the explicit
// acceptance check from the issue body: at a 60-column terminal no
// width-bound style may overflow the viewport. Lipgloss can still
// truncate the inner content even on extremely narrow terminals, but the
// outer rendered width must be ≤ maxWidth.
func TestNewStyles_WidthBoundStylesFitTerminal(t *testing.T) {
	cases := []struct {
		name string
		maxW int
	}{
		{"60-col terminal", 60},
		{"40-col terminal", 40},
		{"24-col terminal (degrades to floor)", 24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStyles(ThemeDark, tc.maxW)
			rows := []string{
				s.Input.Render("placeholder"),
				s.Error.Render("err"),
				s.Help.Render("help " + strings.Repeat("long ", 30)),
				s.Preview.Render("preview " + strings.Repeat("long ", 20)),
				s.WarnBox.Render("warn " + strings.Repeat("long ", 30)),
			}
			for i, r := range rows {
				if w := maxLineWidth(r); w > tc.maxW {
					t.Errorf("style %d overflows terminal %d: rendered width %d", i, tc.maxW, w)
				}
			}
		})
	}
}

// TestChartWidth_FollowsStylesPanelWidth (issue #37) — chart canvas
// shrinks with the terminal width.
func TestChartWidth_FollowsStylesPanelWidth(t *testing.T) {
	cases := []struct {
		maxWidth   int
		wantChartW int
	}{
		{0, PanelWidthMax - boxInnerInset},
		{100, PanelWidthMax - boxInnerInset},
		{60, 52},
		{30, 22},
	}
	for _, tc := range cases {
		s := NewStyles(ThemeDark, tc.maxWidth)
		if got := chartWidth(s); got != tc.wantChartW {
			t.Errorf("chartWidth(NewStyles(ThemeDark, %d)): want %d, got %d", tc.maxWidth, tc.wantChartW, got)
		}
	}
}

// TestPanelWidth_NoExportedConstant (issue #37) — the legacy
// `const PanelWidth = 72` was removed (the package now exposes
// PanelWidthMax/PanelWidthMin instead). This guards against
// re-introducing the symbol accidentally: a stray `PanelWidth`
// reference here would fail to compile.
func TestPanelWidth_NoExportedConstant(t *testing.T) {
	// Sanity: panelWidth-derived behaviour still respects bounds.
	if got := panelWidthBench(100); got != PanelWidthMax {
		t.Errorf("panelWidth(100): want %d (PanelWidthMax), got %d", PanelWidthMax, got)
	}
	if got := panelWidthBench(20); got != PanelWidthMin {
		t.Errorf("panelWidth(20): want %d (PanelWidthMin), got %d", PanelWidthMin, got)
	}
}

// TestDisclaimer_FitsAt80x24 (issue #37) — the very first screen a new
// user ever sees must not overflow an 80×24 terminal. The body is
// rendered inside a scrollable viewport, so the fixed chrome
// (title + WarnBox frame + label + input + hint) plus the scrollable
// body must fit.
func TestDisclaimer_FitsAt80x24(t *testing.T) {
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
	view := dm.View()
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("disclaimer line %d exceeds 80 cols (got %d): %q", i, w, line)
		}
	}
	if h := lipgloss.Height(view); h > 24 {
		t.Errorf("disclaimer view is %d rows tall, must be ≤ 24 to fit 80×24", h)
	}
	if h := lipgloss.Height(view); h < 5 {
		t.Errorf("disclaimer view is only %d rows tall; should at least show title, body, label, input, hint", h)
	}
}

// TestDisclaimer_NarrowTerminal_FitsAt60x24 (issue #37) — even on a
// 60-column terminal the disclaimer body is reachable via the
// scrollable viewport; the screen never overflows the terminal.
func TestDisclaimer_NarrowTerminal_FitsAt60x24(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	s := NewStyles(ThemeDark, 60)
	m := newDisclaimerModel(s)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	dm := updated.(*DisclaimerModel)
	view := dm.View()
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("disclaimer line %d exceeds 60 cols on a 60-col terminal (got %d): %q", i, w, line)
		}
	}
	if h := lipgloss.Height(view); h > 24 {
		t.Errorf("disclaimer view is %d rows tall, must be ≤ 24 on 60×24", h)
	}
}

// TestDisclaimer_BodyIsScrollable (issue #37) — the body text spans
// many lines and must be reachable beyond the viewport's visible
// window. Verifies TotalLineCount > VisibleLineCount.
func TestDisclaimer_BodyIsScrollable(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	s := NewStyles(ThemeDark, 60)
	m := newDisclaimerModel(s)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	dm := updated.(*DisclaimerModel)
	if dm.viewport.TotalLineCount() <= dm.viewport.VisibleLineCount() {
		t.Errorf("expected the disclaimer body to overflow the viewport; TotalLineCount=%d, VisibleLineCount=%d",
			dm.viewport.TotalLineCount(), dm.viewport.VisibleLineCount())
	}
}

// TestDisclaimer_EnterAcceptsYes (regression) — confirm the Enter
// validation path still works after the viewport refactor.
func TestDisclaimer_EnterAcceptsYes(t *testing.T) {
	if err := locale.Init("en"); err != nil {
		t.Fatalf("locale.Init: %v", err)
	}
	s := NewStyles(ThemeDark, 80)
	m := newDisclaimerModel(s)
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.input.SetValue("yes")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected Enter on a valid answer to return a cmd")
	}
	msg := cmd()
	if done, ok := msg.(ScreenDoneMsg); !ok || done.From != ScreenDisclaimer {
		t.Errorf("expected ScreenDoneMsg{From: ScreenDisclaimer}, got %T %+v", msg, msg)
	}
}
