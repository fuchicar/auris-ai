package tui

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

// PortfolioAllocationResult is emitted when the user exits the allocation screen.
type PortfolioAllocationResult struct {
	Action    string               // "back"
	Portfolio *portfolio.Portfolio // updated copy (already saved)
}

// allocationMode tracks which inline form is active.
type allocationMode int

const (
	allocModeList allocationMode = iota // row list, cursor navigation
	allocModeEdit                       // editing one row's weight
)

// allocationRow is one line in the editor: a symbol and its target weight.
// Held is false for symbols that have a target weight but are no longer
// held in the portfolio (e.g. the position was sold or removed).
type allocationRow struct {
	Symbol string
	Weight float64 // fraction, e.g. 0.4 = 40%
	Held   bool
}

// portfolioAllocationModel lets the user edit a portfolio's target allocation.
type portfolioAllocationModel struct {
	portfolio *portfolio.Portfolio
	rows      []allocationRow
	cursor    int
	mode      allocationMode

	weightInput textinput.Model
	weightErr   string

	infoMsg   string
	infoIsErr bool

	styles *Styles
}

func newPortfolioAllocationModel(p *portfolio.Portfolio, s *Styles) *portfolioAllocationModel {
	seen := make(map[string]bool)
	var rows []allocationRow
	for _, ins := range p.Instruments {
		if ins.Type != portfolio.InstrumentHolding || seen[ins.Symbol] {
			continue
		}
		seen[ins.Symbol] = true
		rows = append(rows, allocationRow{
			Symbol: ins.Symbol,
			Weight: p.TargetAllocation[ins.Symbol],
			Held:   true,
		})
	}
	for symbol, weight := range p.TargetAllocation {
		if seen[symbol] {
			continue
		}
		rows = append(rows, allocationRow{Symbol: symbol, Weight: weight, Held: false})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Symbol < rows[j].Symbol })

	ti := textinput.New()
	ti.Placeholder = "40"
	ti.CharLimit = 6

	return &portfolioAllocationModel{
		portfolio:   p,
		rows:        rows,
		weightInput: ti,
		styles:      s,
	}
}

func (m *portfolioAllocationModel) Init() tea.Cmd {
	return nil
}

func (m *portfolioAllocationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		return m.handleKey(key)
	}
	return m, nil
}

func (m *portfolioAllocationModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case allocModeList:
		return m.handleList(key)
	case allocModeEdit:
		return m.handleEdit(key)
	}
	return m, nil
}

func (m *portfolioAllocationModel) handleList(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case tea.KeyEnter:
		if len(m.rows) == 0 {
			return m, nil
		}
		row := m.rows[m.cursor]
		if row.Weight > 0 {
			m.weightInput.SetValue(trimFloat(row.Weight * 100))
		} else {
			m.weightInput.SetValue("")
		}
		m.mode = allocModeEdit
		m.weightInput.Focus()
		return m, textinput.Blink
	case tea.KeyEsc:
		p := m.portfolio
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioAllocation, Result: PortfolioAllocationResult{Action: "back", Portfolio: p}}
		}
	}
	return m, nil
}

func (m *portfolioAllocationModel) handleEdit(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		m.mode = allocModeList
		m.weightInput.Blur()
		m.weightInput.SetValue("")
		m.weightErr = ""
		return m, nil
	case tea.KeyEnter:
		val := strings.TrimSpace(m.weightInput.Value())
		var pct float64
		if val != "" {
			p, err := strconv.ParseFloat(val, 64)
			if err != nil || p < 0 || p > 100 {
				m.weightErr = locale.T("portfolio.allocation.weight.error")
				return m, nil
			}
			pct = p
		}
		m.weightErr = ""
		m.rows[m.cursor].Weight = pct / 100
		m.commit()
		m.mode = allocModeList
		m.weightInput.Blur()
		m.weightInput.SetValue("")
		return m, nil
	}
	var cmd tea.Cmd
	m.weightInput, cmd = m.weightInput.Update(key)
	return m, cmd
}

// commit rebuilds p.TargetAllocation from the in-memory rows (dropping any
// symbol whose weight was cleared to zero) and persists it immediately.
func (m *portfolioAllocationModel) commit() {
	alloc := make(map[string]float64, len(m.rows))
	for _, r := range m.rows {
		if r.Weight > 1e-9 {
			alloc[r.Symbol] = r.Weight
		}
	}
	m.portfolio.TargetAllocation = alloc
	if err := portfolio.SavePortfolio(m.portfolio); err != nil {
		m.infoMsg = locale.Tp("portfolio.error.save", map[string]any{"Error": err.Error()})
		m.infoIsErr = true
		return
	}
	m.infoMsg = locale.T("portfolio.allocation.saved")
	m.infoIsErr = false
}

func (m *portfolioAllocationModel) View() string {
	var parts []string
	parts = append(parts, m.styles.Selected.Render(locale.T("portfolio.allocation.title")), "")

	if len(m.rows) == 0 {
		parts = append(parts, m.styles.Hint.Render(locale.T("portfolio.allocation.empty")))
	} else {
		parts = append(parts, m.viewRows()...)
		parts = append(parts, "", m.viewTotal())
	}

	if m.mode == allocModeEdit {
		parts = append(parts, "")
		parts = append(parts, m.viewEdit()...)
	}

	parts = append(parts, "", m.styles.Hint.Render(locale.T("portfolio.allocation.hint")))

	if m.infoMsg != "" {
		if m.infoIsErr {
			parts = append(parts, m.styles.Error.Render(m.infoMsg))
		} else {
			parts = append(parts, m.styles.Hint.Render(m.infoMsg))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *portfolioAllocationModel) viewRows() []string {
	rows := make([]string, 0, len(m.rows))
	for i, r := range m.rows {
		badge := ""
		if !r.Held {
			badge = " " + m.styles.Hint.Render("("+locale.T("portfolio.allocation.not_held_badge")+")")
		}
		line := fmt.Sprintf("%-10s %6.1f%%%s", r.Symbol, r.Weight*100, badge)
		if i == m.cursor {
			rows = append(rows, fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(line)))
		} else {
			rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(line)))
		}
	}
	return rows
}

func (m *portfolioAllocationModel) viewTotal() string {
	sum := 0.0
	for _, r := range m.rows {
		sum += r.Weight
	}
	line := fmt.Sprintf("%s: %.1f%%", locale.T("portfolio.allocation.total.label"), sum*100)
	if sum > 0 && math.Abs(sum-1.0) > 0.01 {
		warning := locale.Tp("portfolio.allocation.total.warning", map[string]any{"Total": fmt.Sprintf("%.1f", sum*100)})
		return m.styles.WarnBox.Render(line + "\n" + warning)
	}
	return m.styles.Hint.Render(line)
}

func (m *portfolioAllocationModel) viewEdit() []string {
	label := m.styles.Unselected.Render(fmt.Sprintf("%s (%s)", locale.T("portfolio.allocation.weight.label"), m.rows[m.cursor].Symbol))
	input := m.styles.Input.Render(m.weightInput.View())
	hint := m.styles.Hint.Render("Enter save · Esc cancel")
	rows := []string{label, input, hint}
	if m.weightErr != "" {
		rows = append(rows, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.weightErr)))
	}
	return rows
}

// trimFloat formats a float without trailing zeros, e.g. 40 not 40.000000.
func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
