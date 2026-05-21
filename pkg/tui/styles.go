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
	DocsURL    lipgloss.Style // clickable-looking URL
	Checkbox   lipgloss.Style // "[x]" / "[ ]" for multi-select
	Spinner    lipgloss.Style
	Preview    lipgloss.Style // border box used in the theme preview
	Warning    lipgloss.Style // amber bold text for advisory warnings
	WarnBox    lipgloss.Style // amber rounded-border box for warning containers
}

// NewStyles builds a complete [Styles] set for the given [Theme].
// Green and Box variants share the base light/dark palette; only s.Theme differs,
// which controls how agent messages are rendered in the chat screen.
func NewStyles(t Theme) *Styles {
	var s *Styles
	switch t {
	case ThemeLight, ThemeGreenLight, ThemeBoxLight:
		s = newLightStyles()
	default:
		s = newDarkStyles()
	}
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

func newDarkStyles() *Styles {
	accent := lipgloss.Color("#7C3AED")
	selected := lipgloss.Color("#A78BFA")
	errorC := lipgloss.Color("#F87171")
	hint := lipgloss.Color("#6B7280")
	url := lipgloss.Color("#60A5FA")
	warn := lipgloss.Color("#F59E0B")

	return &Styles{
		Theme:      ThemeDark,
		PanelWidth: PanelWidth,
		Title:      lipgloss.NewStyle().Bold(true).Foreground(accent).Padding(1, 0),
		Subtitle:   lipgloss.NewStyle().Foreground(selected).MarginBottom(1),
		Cursor:     lipgloss.NewStyle().Foreground(accent).Bold(true),
		Selected:   lipgloss.NewStyle().Foreground(selected).Bold(true),
		Unselected: lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")),
		Input:      lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1).Width(PanelWidth - 4),
		Error:      lipgloss.NewStyle().Foreground(errorC).Bold(true),
		Hint:       lipgloss.NewStyle().Foreground(hint).Italic(true),
		DocsURL:    lipgloss.NewStyle().Foreground(url).Underline(true),
		Checkbox:   lipgloss.NewStyle().Foreground(selected),
		Spinner:    lipgloss.NewStyle().Foreground(accent),
		Preview:    lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1).Width(PanelWidth - 4),
		Warning:    lipgloss.NewStyle().Foreground(warn).Bold(true),
		WarnBox:    lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(warn).Padding(0, 1).Width(PanelWidth - 4),
	}
}

func newLightStyles() *Styles {
	accent := lipgloss.Color("#5B21B6")
	selected := lipgloss.Color("#7C3AED")
	errorC := lipgloss.Color("#DC2626")
	hint := lipgloss.Color("#9CA3AF")
	url := lipgloss.Color("#2563EB")
	warn := lipgloss.Color("#D97706")

	return &Styles{
		Theme:      ThemeLight,
		PanelWidth: PanelWidth,
		Title:      lipgloss.NewStyle().Bold(true).Foreground(accent).Padding(1, 0),
		Subtitle:   lipgloss.NewStyle().Foreground(selected).MarginBottom(1),
		Cursor:     lipgloss.NewStyle().Foreground(accent).Bold(true),
		Selected:   lipgloss.NewStyle().Foreground(selected).Bold(true),
		Unselected: lipgloss.NewStyle().Foreground(lipgloss.Color("#374151")),
		Input:      lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1).Width(PanelWidth - 4),
		Error:      lipgloss.NewStyle().Foreground(errorC).Bold(true),
		Hint:       lipgloss.NewStyle().Foreground(hint).Italic(true),
		DocsURL:    lipgloss.NewStyle().Foreground(url).Underline(true),
		Checkbox:   lipgloss.NewStyle().Foreground(selected),
		Spinner:    lipgloss.NewStyle().Foreground(accent),
		Preview:    lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1).Width(PanelWidth - 4),
		Warning:    lipgloss.NewStyle().Foreground(warn).Bold(true),
		WarnBox:    lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(warn).Padding(0, 1).Width(PanelWidth - 4),
	}
}
