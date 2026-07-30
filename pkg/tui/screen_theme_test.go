package tui

import "testing"

// TestNewStylesForWidth (issues #37 follow-up) — sub-screens like the
// theme picker need one Styles per theme, each inheriting the parent's
// already-resolved PanelWidth. [NewStylesForWidth] accepts that value
// directly, bypassing the maxWidth → panelWidth derivation that would
// otherwise shrink the preview by another panelMargin cols.
func TestNewStylesForWidth(t *testing.T) {
	cases := []struct {
		name string
		pw   int
		want int
	}{
		{"PanelWidthMax passes through", PanelWidthMax, PanelWidthMax},
		{"PanelWidthMin passes through", PanelWidthMin, PanelWidthMin},
		{"60-col-equivalent passes through", 56, 56},
		{"too small clamps to PanelWidthMin", 4, PanelWidthMin},
		{"too large clamps to PanelWidthMax", 200, PanelWidthMax},
		{"zero clamps to PanelWidthMin", 0, PanelWidthMin},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStylesForWidth(ThemeDark, tc.pw)
			if s.PanelWidth != tc.want {
				t.Errorf("NewStylesForWidth(ThemeDark, %d).PanelWidth: want %d, got %d", tc.pw, tc.want, s.PanelWidth)
			}
		})
	}
}

// TestNewStyles_AndNewStylesForWidth_Disagree (issues #37 follow-up) —
// the regression that bit the theme picker: calling NewStyles(t, pw)
// re-runs panelWidth() and shrinks the styles by panelMargin cols, so
// sub-screens that pass a parent's resolved PanelWidth end up with a
// narrower style than the parent. NewStylesForWidth exists precisely
// to skip that derivation and stay at pw.
func TestNewStyles_AndNewStylesForWidth_Disagree(t *testing.T) {
	parent := NewStyles(ThemeDark, 80) // PanelWidth=72 on 80-col terminal
	if parent.PanelWidth != PanelWidthMax {
		t.Fatalf("expected parent PanelWidth=%d on 80-col terminal, got %d", PanelWidthMax, parent.PanelWidth)
	}

	wrong := NewStyles(ThemeDark, parent.PanelWidth) // what screen_theme.go used to do
	ok := NewStylesForWidth(ThemeDark, parent.PanelWidth)

	if wrong.PanelWidth == parent.PanelWidth {
		t.Errorf("expected NewStyles(t, parent.PanelWidth) to shrink, but it matched: %d", wrong.PanelWidth)
	}
	if ok.PanelWidth != parent.PanelWidth {
		t.Errorf("expected NewStylesForWidth(t, parent.PanelWidth) to match parent (%d), got %d", parent.PanelWidth, ok.PanelWidth)
	}

	// Sanity: the difference is exactly panelMargin (the derivation's
	// only source of shrinkage at the ceiling).
	if wrong.PanelWidth != parent.PanelWidth-panelMargin {
		t.Errorf("expected NewStyles shrinkage to be exactly panelMargin=%d cols, got %d - %d = %d",
			panelMargin, parent.PanelWidth, wrong.PanelWidth, parent.PanelWidth-wrong.PanelWidth)
	}
}

// TestNewThemeModel_PreviewWidthsMatchParent (issues #37 follow-up) —
// the actual regression in screen_theme.go: every theme preview built
// by newThemeModel must inherit the parent's PanelWidth exactly. At an
// 80-col terminal the parent is at PanelWidthMax=72; before the fix
// each preview was stuck at 68 (72 - panelMargin) and visibly narrower
// than the rest of the chrome.
func TestNewThemeModel_PreviewWidthsMatchParent(t *testing.T) {
	cases := []struct {
		name string
		maxW int
	}{
		{"80-col terminal (regression case)", 80},
		{"120-col terminal", 120},
		{"60-col terminal", 60},
		{"40-col terminal", 40},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent := NewStyles(ThemeDark, tc.maxW)
			m := newThemeModel(parent, false)
			for i, p := range m.previews {
				if p.PanelWidth != parent.PanelWidth {
					t.Errorf("preview[%d].PanelWidth=%d, want %d (matches parent)", i, p.PanelWidth, parent.PanelWidth)
				}
			}
		})
	}
}
