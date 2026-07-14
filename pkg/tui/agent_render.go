package tui

import (
	"fmt"
	"strconv"
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

// hexToRGB decomposes a "#RRGGBB" color into its components.
func hexToRGB(h string) (r, g, b uint8) {
	h = strings.TrimPrefix(h, "#")
	v, _ := strconv.ParseUint(h, 16, 32)
	return uint8(v >> 16), uint8((v >> 8) & 0xFF), uint8(v & 0xFF)
}

// ansiSeq returns a 24-bit SGR color sequence.
// bg=true → background (48); bg=false → foreground (38).
func ansiSeq(hex string, bg bool) string {
	r, g, b := hexToRGB(hex)
	if bg {
		return fmt.Sprintf("\033[48;2;%d;%d;%dm", r, g, b)
	}
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
}

// persistBg replaces every ANSI reset in glamour output with
// reset + re-apply of block background and foreground, so the
// block background survives inline resets emitted by glamour.
func persistBg(content, bgHex, fgHex string) string {
	reapply := ansiSeq(bgHex, true) + ansiSeq(fgHex, false)
	content = strings.ReplaceAll(content, "\033[0m", "\033[0m"+reapply)
	content = strings.ReplaceAll(content, "\033[m", "\033[m"+reapply)
	return content
}

// agentLabel renders the "Auris:" label shown above the agent body. It sits
// on its own line with no painted background under it, so — unlike the
// bg/fg pairs below, which are explicitly painted and thus keyed to the
// theme's own s.IsLight() — its color must adapt to the real terminal
// background (see REF-11 in TODO.md).
func agentLabel() string {
	return lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("Auris:")
}

// renderAgentTinted renders agent messages with a soft tinted background block.
func renderAgentTinted(s *Styles, content string, w int) string {
	bgHex, fgHex := "#1A1625", "#DDD6FE"
	if s.IsLight() {
		bgHex, fgHex = "#F5F0FF", "#2D1B69"
	}
	body := lipgloss.NewStyle().
		Background(lipgloss.Color(bgHex)).Foreground(lipgloss.Color(fgHex)).
		Width(w).Padding(0, 1).
		Render(persistBg(content, bgHex, fgHex))
	return agentLabel() + "\n" + body
}

// renderAgentBadge renders agent messages with a green role badge.
func renderAgentBadge(s *Styles, content string) string {
	badge := lipgloss.NewStyle().
		Background(lipgloss.Color("#059669")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).Padding(0, 1).
		Render(" Auris ")
	bgHex, fgHex := "#022C22", "#D1FAE5"
	if s.IsLight() {
		bgHex, fgHex = "#ECFDF5", "#064E3B"
	}
	body := lipgloss.NewStyle().
		Background(lipgloss.Color(bgHex)).Foreground(lipgloss.Color(fgHex)).
		Render(persistBg(content, bgHex, fgHex))
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
	innerW := w - 4 // border (1 each side) + padding (1 each side)
	if innerW < 1 {
		innerW = 1
	}
	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderC).
		Background(bg).Foreground(fg).
		Width(innerW).Padding(0, 1).
		Render(persistBg(content, string(bg), string(fg)))
	return agentLabel() + "\n" + box
}
