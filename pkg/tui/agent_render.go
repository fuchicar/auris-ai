package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderAgentBlock renders a complete agent turn (label + styled body) using the
// theme-specific style. content should be glamour-rendered markdown or plain text.
func RenderAgentBlock(s *Styles, content string, w int) string {
	content = strings.TrimSpace(content)
	switch s.Theme {
	case ThemeGreenLight, ThemeGreenDark:
		return renderAgentBadge(s, content)
	case ThemeBoxLight, ThemeBoxDark:
		return renderAgentBox(s, content, w)
	default: // ThemeLight, ThemeDark
		return renderAgentTinted(s, content, w)
	}
}

// renderAgentTinted renders agent messages with a soft tinted background block.
func renderAgentTinted(s *Styles, content string, w int) string {
	var bg, fg lipgloss.Color
	if s.IsLight() {
		bg, fg = "#F5F0FF", "#2D1B69"
	} else {
		bg, fg = "#1A1625", "#DDD6FE"
	}
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Render("Auris:")
	body := lipgloss.NewStyle().
		Background(bg).Foreground(fg).
		Width(w).Padding(0, 1).
		Render(content)
	return label + "\n" + body
}

// renderAgentBadge renders agent messages with a green role badge.
func renderAgentBadge(s *Styles, content string) string {
	badge := lipgloss.NewStyle().
		Background(lipgloss.Color("#059669")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).Padding(0, 1).
		Render(" Auris ")
	var fg lipgloss.Color
	if s.IsLight() {
		fg = "#064E3B"
	} else {
		fg = "#D1FAE5"
	}
	body := lipgloss.NewStyle().Foreground(fg).Render(content)
	return badge + "\n" + body
}

// renderAgentBox renders agent messages inside a rounded border box.
func renderAgentBox(s *Styles, content string, w int) string {
	var borderC, bg, fg lipgloss.Color
	if s.IsLight() {
		borderC, bg, fg = "#D4D4D8", "#FAFAFA", "#18181B"
	} else {
		borderC, bg, fg = "#3F3F46", "#18181B", "#E4E4E7"
	}
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Render("Auris:")
	innerW := w - 4 // border (1 each side) + padding (1 each side)
	if innerW < 1 {
		innerW = 1
	}
	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderC).
		Background(bg).Foreground(fg).
		Width(innerW).Padding(0, 1).
		Render(content)
	return label + "\n" + box
}
