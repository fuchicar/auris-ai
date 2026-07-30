// Package tui provides the terminal user interface for Auris.
// It is built on top of [BubbleTea] and [LipGloss].
package tui

import "github.com/charmbracelet/lipgloss"

// Theme selects the active color palette.
type Theme string

const (
	// ThemeLight uses a bright background and dark text accents, with a tinted agent block.
	ThemeLight Theme = "light"
	// ThemeDark uses a dark background and bright text accents, with a tinted agent block.
	ThemeDark Theme = "dark"
	// ThemeGreenLight is a light-base theme with green badge-style agent messages.
	ThemeGreenLight Theme = "greenlight"
	// ThemeGreenDark is a dark-base theme with green badge-style agent messages.
	ThemeGreenDark Theme = "greendark"
	// ThemeBoxLight is a light-base theme with rounded-box agent messages.
	ThemeBoxLight Theme = "boxlight"
	// ThemeBoxDark is a dark-base theme with rounded-box agent messages.
	ThemeBoxDark Theme = "boxdark"
)

// Panel width bounds.
//
// PanelWidthMax is the comfortable target width used when the terminal is
// wide enough. PanelWidthMin is the floor below which lipgloss would wrap
// any bordered box harder than the data warrants — we still let lipgloss
// truncate when the terminal is genuinely narrower, but cap the
// floor here so very narrow windows don't render an empty 1-column box.
//
// panelMargin is the gutter (cols on each side) the centred panels
// reserve via lipgloss.Place; widths are derived as `maxWidth -
// panelMargin`, so a 60-col terminal yields 56, a 24-col terminal clamps
// to PanelWidthMin (issues #37).
//
// Lipgloss Width(N) on a style that also has Border + Padding(0, 1)
// means "the content + padding total = N" — the rounded border is
// rendered *outside* Width(N), so the box's outer width is N + 2.
// To make a WarnBox/Input/Preview render with outer width exactly equal
// to PanelWidth, set .Width(PanelWidth - 2). The content area inside
// the box is then `PanelWidth - 2 - 2 = PanelWidth - 4` cols
// (accounting for the 1-col left/right padding). Any content wider than
// that re-wraps inside the box — visible at ~2× height — which is the
// bug the viewport refactor fixed (issues #37).
const (
	PanelWidthMax = 72
	PanelWidthMin = 20
	panelMargin   = 4
	boxOuterInset = 2 // border cols that lipgloss adds *outside* Width(N)
	boxInnerInset = 4 // total reserved cols inside the box: border (1) + padding (1) per side
)

// panelWidth returns the actual content width to use given the terminal's
// outer width. When maxWidth is 0 (no resize event yet, e.g. tests that
// construct Styles directly) we fall back to the ceiling so screens built
// outside an AppModel still render at the historical default.
func panelWidth(maxWidth int) int {
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

// Styles holds all LipGloss styles for a given theme. Screens receive a
// pointer so that the same pointer can be swapped atomically when the user
// changes the theme during setup or the terminal is resized (issue #37:
// styles used to be built once with a fixed PanelWidth=72, breaking
// terminals narrower than ~72 columns and forcing every box to wrap).
type Styles struct {
	// Theme identifies which palette is in use.
	Theme Theme

	// PanelWidth is the content width constraint for all screens. It is
	// recomputed from the current terminal width whenever AppModel
	// receives a WindowSizeMsg; screens should always read this field
	// instead of any package-level constant.
	PanelWidth int

	Title      lipgloss.Style
	Subtitle   lipgloss.Style
	Cursor     lipgloss.Style // the ">" arrow before a focused row
	Selected   lipgloss.Style // highlighted option
	Unselected lipgloss.Style
	Input      lipgloss.Style
	Error      lipgloss.Style
	Hint       lipgloss.Style // keyboard shortcut hints
	Help       lipgloss.Style // wrapped explanatory text shown before a field
	DocsURL    lipgloss.Style // clickable-looking URL
	Checkbox   lipgloss.Style // "[x]" / "[ ]" for multi-select
	Spinner    lipgloss.Style
	Preview    lipgloss.Style // border box used in the theme preview
	Warning    lipgloss.Style // amber bold text for advisory warnings
	WarnBox    lipgloss.Style // amber rounded-border box for warning containers
	Bull       lipgloss.Style // green — bullish candle (close >= open) / sparkline line
	Bear       lipgloss.Style // red — bearish candle (close < open)
}

// NewStyles builds a complete [Styles] set for the given [Theme] and
// terminal width (issues #37). maxWidth is the outer terminal width in
// columns; pass 0 to fall back to the default PanelWidthMax ceiling
// (useful for tests that don't seed a real terminal). All 6 themes share
// the same adaptive palette (see newBaseStylesFromPanelWidth); only
// s.Theme differs, which controls how agent messages are rendered in the
// chat screen (tinted block / green badge / rounded box, light or dark).
//
// Use [NewStylesForWidth] when a sub-screen needs to inherit the
// parent's already-resolved PanelWidth without re-running the
// maxWidth → panelWidth derivation (which would shrink it by another
// panelMargin cols — issues #37).
func NewStyles(t Theme, maxWidth int) *Styles {
	s := newBaseStylesFromPanelWidth(panelWidth(maxWidth))
	s.Theme = t
	return s
}

// NewStylesForWidth builds styles with the given pre-resolved
// [Styles.PanelWidth], bypassing the maxWidth→panelWidth derivation. Use
// this when a sub-screen has already computed its panel width and wants
// a sibling set of styles (e.g. the theme picker wants one [Styles] per
// theme, each inheriting the parent's panel width — issues #37).
//
// pw is clamped to [PanelWidthMin, PanelWidthMax] exactly once.
func NewStylesForWidth(t Theme, pw int) *Styles {
	if pw < PanelWidthMin {
		pw = PanelWidthMin
	}
	if pw > PanelWidthMax {
		pw = PanelWidthMax
	}
	s := newBaseStylesFromPanelWidth(pw)
	s.Theme = t
	return s
}

// IsLight reports whether the theme uses a light (bright) base palette.
func (s *Styles) IsLight() bool {
	return s.Theme == ThemeLight || s.Theme == ThemeGreenLight || s.Theme == ThemeBoxLight
}

// IsValidTheme reports whether t is a known theme identifier.
func IsValidTheme(t string) bool {
	switch Theme(t) {
	case ThemeLight, ThemeDark, ThemeGreenLight, ThemeGreenDark, ThemeBoxLight, ThemeBoxDark:
		return true
	}
	return false
}

// Adaptive color pairs shared by every theme. Each resolves against the
// terminal's actual detected background (via lipgloss.HasDarkBackground),
// independent of which Theme the user picked, so text stays legible
// regardless of a mismatch between the chosen theme and the real terminal
// (see REF-11 — commit 79dfc56).
var (
	colorAccent     = lipgloss.AdaptiveColor{Light: "#5B21B6", Dark: "#7C3AED"}
	colorSelected   = lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#A78BFA"}
	colorError      = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F87171"}
	colorHint       = lipgloss.AdaptiveColor{Light: "#9CA3AF", Dark: "#6B7280"}
	colorURL        = lipgloss.AdaptiveColor{Light: "#2563EB", Dark: "#60A5FA"}
	colorWarn       = lipgloss.AdaptiveColor{Light: "#D97706", Dark: "#F59E0B"}
	colorBull       = lipgloss.AdaptiveColor{Light: "#059669", Dark: "#10B981"}
	colorUnselected = lipgloss.AdaptiveColor{Light: "#374151", Dark: "#D1D5DB"}
)

// newBaseStylesFromPanelWidth builds the width-bound styles directly
// from the resolved panel width. Box-bordered styles (Input/Preview/
// WarnBox) use `.Width(pw - boxOuterInset)` so the *outer* rendered
// width equals pw (lipgloss adds boxOuterInset=2 cols of border
// OUTSIDE Width). Content placed inside the box (e.g. the disclaimer
// viewport body) must therefore fit in `pw - boxInnerInset` cols or it
// re-wraps inside the box (issues #37).
func newBaseStylesFromPanelWidth(pw int) *Styles {
	frameW := pw - boxOuterInset
	if frameW < 1 {
		frameW = 1
	}
	return &Styles{
		PanelWidth: pw,
		Title:      lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Padding(1, 0),
		Subtitle:   lipgloss.NewStyle().Foreground(colorSelected).MarginBottom(1),
		Cursor:     lipgloss.NewStyle().Foreground(colorAccent).Bold(true),
		Selected:   lipgloss.NewStyle().Foreground(colorSelected).Bold(true),
		Unselected: lipgloss.NewStyle().Foreground(colorUnselected),
		Input:      lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(colorAccent).Padding(0, 1).Width(frameW),
		Error:      lipgloss.NewStyle().Foreground(colorError).Bold(true).Width(pw),
		Hint:       lipgloss.NewStyle().Foreground(colorHint).Italic(true),
		Help:       lipgloss.NewStyle().Foreground(colorHint).Width(pw),
		DocsURL:    lipgloss.NewStyle().Foreground(colorURL).Underline(true),
		Checkbox:   lipgloss.NewStyle().Foreground(colorSelected),
		Spinner:    lipgloss.NewStyle().Foreground(colorAccent),
		Preview:    lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(colorAccent).Padding(0, 1).Width(frameW),
		Warning:    lipgloss.NewStyle().Foreground(colorWarn).Bold(true).Width(pw),
		WarnBox:    lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(colorWarn).Padding(0, 1).Width(frameW),
		Bull:       lipgloss.NewStyle().Foreground(colorBull),
		Bear:       lipgloss.NewStyle().Foreground(colorError),
	}
}
