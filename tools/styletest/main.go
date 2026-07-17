// styletest is a throwaway program to compare visual proposals for agent-message
// styling. Press space to cycle through proposals; q or Ctrl+C to exit.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/tui"
)

// ── sample conversation ───────────────────────────────────────────────────────

type turn struct {
	role    string // "user" | "assistant"
	content string
}

var sample = []turn{
	{"user", "¿Cuál es el precio actual de Apple (AAPL)?"},
	{"assistant", "El precio actual de AAPL es $182.50.\n\n  • Bid:    $182.45\n  • Ask:    $182.55\n  • Cambio: +2.3% hoy\n\nLa subida coincide con los resultados del último trimestre."},
	{"user", "¿Y su ratio P/E comparado con el sector tecnológico?"},
	{"assistant", "El P/E de Apple es aprox. 28.5x, por encima de la media\ndel sector (~25x), lo que refleja altas expectativas.\n\n  • Apple:     28.5x\n  • Microsoft: 35.2x\n  • Alphabet:  23.1x\n\nConsidera este dato junto con el crecimiento esperado de EPS."},
}

// ── proposals ─────────────────────────────────────────────────────────────────

type proposal struct {
	name   string
	render func(s *tui.Styles, role, content string, w int) string
}

var proposals = []proposal{
	// ── 0: estado actual ─────────────────────────────────────────────────────
	{
		name: "Actual (referencia)",
		render: func(s *tui.Styles, role, content string, w int) string {
			wrap := lipgloss.NewStyle().Width(w).Render
			if role == "user" {
				return wrap(s.Selected.Render("You: ") + content)
			}
			return s.Hint.Render("Auris:") + "\n" + wrap(content)
		},
	},

	// ── 1: fondo tintado ─────────────────────────────────────────────────────
	{
		name: "Fondo tintado",
		render: func(s *tui.Styles, role, content string, w int) string {
			wrap := lipgloss.NewStyle().Width(w).Render
			if role == "user" {
				return wrap(s.Selected.Render("You: ") + content)
			}
			var bg, fg lipgloss.Color
			if s.Theme == tui.ThemeDark {
				bg, fg = "#1A1625", "#DDD6FE"
			} else {
				bg, fg = "#F5F0FF", "#2D1B69"
			}
			label := lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Render("Auris:")
			body := lipgloss.NewStyle().
				Background(bg).Foreground(fg).
				Width(w).Padding(0, 1).
				Render(content)
			return label + "\n" + body
		},
	},

	// ── 2: borde izquierdo ───────────────────────────────────────────────────
	{
		name: "Borde izquierdo",
		render: func(s *tui.Styles, role, content string, w int) string {
			wrap := lipgloss.NewStyle().Width(w).Render
			if role == "user" {
				return wrap(s.Selected.Render("You: ") + content)
			}
			var borderC, textC lipgloss.Color
			if s.Theme == tui.ThemeDark {
				borderC, textC = "#7C3AED", "#E2E8F0"
			} else {
				borderC, textC = "#5B21B6", "#1F2937"
			}
			bar := lipgloss.NewStyle().Foreground(borderC).Bold(true).Render("▎")
			textStyle := lipgloss.NewStyle().Foreground(textC)
			label := lipgloss.NewStyle().Foreground(borderC).Bold(true).Render("Auris:")
			var sb strings.Builder
			for _, line := range strings.Split(content, "\n") {
				sb.WriteString(bar + " " + textStyle.Render(line) + "\n")
			}
			return label + "\n" + strings.TrimRight(sb.String(), "\n")
		},
	},

	// ── 3: badges de rol ─────────────────────────────────────────────────────
	{
		name: "Badges de rol",
		render: func(s *tui.Styles, role, content string, w int) string {
			if role == "user" {
				badge := lipgloss.NewStyle().
					Background(lipgloss.Color("#7C3AED")).
					Foreground(lipgloss.Color("#FFFFFF")).
					Bold(true).Padding(0, 1).Render(" Tú ")
				return badge + "  " + content
			}
			badge := lipgloss.NewStyle().
				Background(lipgloss.Color("#059669")).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).Padding(0, 1).Render(" Auris ")
			var fg lipgloss.Color
			if s.Theme == tui.ThemeDark {
				fg = "#D1FAE5"
			} else {
				fg = "#064E3B"
			}
			body := lipgloss.NewStyle().Foreground(fg).Render(content)
			return badge + "\n" + body
		},
	},

	// ── 4: caja redondeada ───────────────────────────────────────────────────
	{
		name: "Caja redondeada",
		render: func(s *tui.Styles, role, content string, w int) string {
			wrap := lipgloss.NewStyle().Width(w).Render
			if role == "user" {
				return wrap(s.Selected.Render("You: ") + content)
			}
			var borderC, bg, fg lipgloss.Color
			if s.Theme == tui.ThemeDark {
				borderC, bg, fg = "#3F3F46", "#18181B", "#E4E4E7"
			} else {
				borderC, bg, fg = "#D4D4D8", "#FAFAFA", "#18181B"
			}
			label := lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true).Render("Auris:")
			// Width(w-4): content width; +2 padding + 2 border = w total.
			box := lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(borderC).
				Background(bg).Foreground(fg).
				Width(w - 4).Padding(0, 1).
				Render(content)
			return label + "\n" + box
		},
	},
}

// ── BubbleTea model ───────────────────────────────────────────────────────────

type styleModel struct {
	idx   int
	dark  *tui.Styles
	light *tui.Styles
	width int
}

func (m styleModel) Init() tea.Cmd { return nil }

func (m styleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case " ":
			m.idx = (m.idx + 1) % len(proposals)
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m styleModel) View() string {
	if m.width == 0 {
		return ""
	}

	w := m.width - 2 // 1 char margin each side
	p := proposals[m.idx]
	sep := strings.Repeat("─", m.width)

	// ── headers ──────────────────────────────────────────────────────────────
	darkHeader := lipgloss.NewStyle().
		Background(lipgloss.Color("#1A0B2E")).
		Foreground(lipgloss.Color("#A78BFA")).
		Bold(true).Width(m.width).Padding(0, 1).
		Render("▌ TEMA OSCURO")

	lightHeader := lipgloss.NewStyle().
		Background(lipgloss.Color("#EDE9FE")).
		Foreground(lipgloss.Color("#5B21B6")).
		Bold(true).Width(m.width).Padding(0, 1).
		Render("▌ TEMA CLARO")

	// ── footer ───────────────────────────────────────────────────────────────
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280")).Italic(true).
		Render(fmt.Sprintf(" [%d/%d] %s   space: siguiente · q: salir",
			m.idx+1, len(proposals), p.name))

	// ── render conversations ──────────────────────────────────────────────────
	renderConv := func(s *tui.Styles) string {
		var sb strings.Builder
		for i, t := range sample {
			sb.WriteString(p.render(s, t.role, t.content, w))
			if i < len(sample)-1 {
				sb.WriteString("\n\n")
			}
		}
		return sb.String()
	}

	parts := []string{
		darkHeader,
		"",
		renderConv(m.dark),
		"",
		sep,
		lightHeader,
		"",
		renderConv(m.light),
		"",
		sep,
		footer,
	}
	return strings.Join(parts, "\n")
}

func main() {
	m := styleModel{
		dark:  tui.NewStyles(tui.ThemeDark),
		light: tui.NewStyles(tui.ThemeLight),
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
