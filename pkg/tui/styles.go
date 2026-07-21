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

// PanelWidth is the fixed content width used by all screens.
// Keeping it constant ensures consistent layout regardless of terminal size;
// AppModel centres the panel horizontally and vertically via lipgloss.Place.
const PanelWidth = 72

// Styles holds all LipGloss styles for a given theme. Screens receive a
// pointer so that the same pointer can be swapped atomically when the user
// changes the theme during setup.
type Styles struct {
	// Theme identifies which palette is in use.
	Theme Theme

	// PanelWidth is the content width constraint for all screens.
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

// NewStyles builds a complete [Styles] set for the given [Theme].
// All 6 themes share the same adaptive palette (see newBaseStyles); only
// s.Theme differs, which controls how agent messages are rendered in the
// chat screen (tinted block / green badge / rounded box, light or dark).
func NewStyles(t Theme) *Styles {
	s := newBaseStyles()
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

func newBaseStyles() *Styles {
	return &Styles{
		PanelWidth: PanelWidth,
		Title:      lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Padding(1, 0),
		Subtitle:   lipgloss.NewStyle().Foreground(colorSelected).MarginBottom(1),
		Cursor:     lipgloss.NewStyle().Foreground(colorAccent).Bold(true),
		Selected:   lipgloss.NewStyle().Foreground(colorSelected).Bold(true),
		Unselected: lipgloss.NewStyle().Foreground(colorUnselected),
		Input:      lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(colorAccent).Padding(0, 1).Width(PanelWidth - 4),
		Error:      lipgloss.NewStyle().Foreground(colorError).Bold(true).Width(PanelWidth),
		Hint:       lipgloss.NewStyle().Foreground(colorHint).Italic(true),
		Help:       lipgloss.NewStyle().Foreground(colorHint).Width(PanelWidth),
		DocsURL:    lipgloss.NewStyle().Foreground(colorURL).Underline(true),
		Checkbox:   lipgloss.NewStyle().Foreground(colorSelected),
		Spinner:    lipgloss.NewStyle().Foreground(colorAccent),
		Preview:    lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(colorAccent).Padding(0, 1).Width(PanelWidth - 4),
		Warning:    lipgloss.NewStyle().Foreground(colorWarn).Bold(true),
		WarnBox:    lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(colorWarn).Padding(0, 1).Width(PanelWidth - 4),
		Bull:       lipgloss.NewStyle().Foreground(colorBull),
		Bear:       lipgloss.NewStyle().Foreground(colorError),
	}
}
