// Package tui provides the terminal user interface for Auris.
// It is built on top of [BubbleTea] and [LipGloss].
package tui

import "github.com/charmbracelet/lipgloss"

// Theme selects the active color palette.
type Theme string

const (
	// ThemeLight uses a bright background and dark text accents.
	ThemeLight Theme = "light"
	// ThemeDark uses a dark background and bright text accents.
	ThemeDark Theme = "dark"
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
}

// NewStyles builds a complete [Styles] set for the given [Theme].
func NewStyles(t Theme) *Styles {
	if t == ThemeLight {
		return newLightStyles()
	}
	return newDarkStyles()
}

func newDarkStyles() *Styles {
	accent := lipgloss.Color("#7C3AED")
	selected := lipgloss.Color("#A78BFA")
	errorC := lipgloss.Color("#F87171")
	hint := lipgloss.Color("#6B7280")
	url := lipgloss.Color("#60A5FA")

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
	}
}

func newLightStyles() *Styles {
	accent := lipgloss.Color("#5B21B6")
	selected := lipgloss.Color("#7C3AED")
	errorC := lipgloss.Color("#DC2626")
	hint := lipgloss.Color("#9CA3AF")
	url := lipgloss.Color("#2563EB")

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
	}
}
