package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
	"auris/pkg/market"
	"auris/pkg/portfolio"
)

// PortfolioViewResult is emitted when the user chooses an action on the portfolio screen.
type PortfolioViewResult struct {
	Action    string               // "agent"|"instruments"|"allocation"|"add"|"edit"|"deleted"
	Portfolio *portfolio.Portfolio // always set
}

// portfolioPricesMsg carries live quotes fetched for all holdings.
type portfolioPricesMsg struct {
	prices map[string]float64 // symbol → last price
	err    error
}

var portfolioViewActions = []string{
	"portfolio.view.action.agent",
	"portfolio.view.action.instruments",
	"portfolio.view.action.allocation",
	"portfolio.view.action.add",
	"portfolio.view.action.edit",
	"portfolio.view.action.delete",
}

// portfolioViewModel shows a summary panel and an action menu for a single portfolio.
type portfolioViewModel struct {
	portfolio     *portfolio.Portfolio
	cursor        int
	confirming    bool // true when delete confirmation is shown
	prices        map[string]float64
	loadingPrices bool
	priceErr      string
	infoMsg       string
	spin          spinner.Model
	mp            market.ProviderAPI
	styles        *Styles
}

func newPortfolioViewModel(p *portfolio.Portfolio, mp market.ProviderAPI, s *Styles) *portfolioViewModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = s.Spinner

	hasHoldings := false
	for _, ins := range p.Instruments {
		if ins.Type == portfolio.InstrumentHolding && len(ins.Lots) > 0 {
			hasHoldings = true
			break
		}
	}

	return &portfolioViewModel{
		portfolio:     p,
		mp:            mp,
		styles:        s,
		spin:          sp,
		loadingPrices: mp != nil && hasHoldings,
	}
}

func (m *portfolioViewModel) Init() tea.Cmd {
	if m.loadingPrices {
		return tea.Batch(m.spin.Tick, m.fetchPricesCmd())
	}
	return nil
}

func (m *portfolioViewModel) fetchPricesCmd() tea.Cmd {
	symbols := make([]string, 0)
	seen := make(map[string]bool)
	for _, ins := range m.portfolio.Instruments {
		if ins.Type == portfolio.InstrumentHolding && !seen[ins.Symbol] {
			symbols = append(symbols, ins.Symbol)
			seen[ins.Symbol] = true
		}
	}
	mp := m.mp
	return func() tea.Msg {
		prices := make(map[string]float64, len(symbols))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if !mp.IsConnected() {
			_ = mp.Connect(ctx)
		}
		for _, sym := range symbols {
			q, err := mp.GetQuote(ctx, sym)
			if err == nil && q.Last > 0 {
				prices[sym] = q.Last
			}
		}
		return portfolioPricesMsg{prices: prices}
	}
}

func (m *portfolioViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.loadingPrices {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case portfolioPricesMsg:
		m.loadingPrices = false
		if msg.err == nil {
			m.prices = msg.prices
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *portfolioViewModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirming {
		switch {
		case key.Type == tea.KeyEsc:
			m.confirming = false
		case key.Type == tea.KeyRunes && (key.String() == "y" || key.String() == "s"):
			if err := portfolio.DeletePortfolio(m.portfolio.ID); err != nil {
				m.infoMsg = locale.Tp("portfolio.error.save", map[string]any{"Error": err.Error()})
				m.confirming = false
				return m, nil
			}
			p := m.portfolio
			return m, func() tea.Msg {
				return ScreenDoneMsg{From: ScreenPortfolioView, Result: PortfolioViewResult{Action: "deleted", Portfolio: p}}
			}
		default:
			m.confirming = false
		}
		return m, nil
	}

	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(portfolioViewActions)-1 {
			m.cursor++
		}
	case tea.KeyEnter:
		return m.selectAction()
	case tea.KeyEsc:
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioView, Result: nil}
		}
	}
	return m, nil
}

func (m *portfolioViewModel) selectAction() (tea.Model, tea.Cmd) {
	p := m.portfolio
	switch m.cursor {
	case 0: // agent
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioView, Result: PortfolioViewResult{Action: "agent", Portfolio: p}}
		}
	case 1: // instruments
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioView, Result: PortfolioViewResult{Action: "instruments", Portfolio: p}}
		}
	case 2: // allocation
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioView, Result: PortfolioViewResult{Action: "allocation", Portfolio: p}}
		}
	case 3: // add
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioView, Result: PortfolioViewResult{Action: "add", Portfolio: p}}
		}
	case 4: // edit
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioView, Result: PortfolioViewResult{Action: "edit", Portfolio: p}}
		}
	case 5: // delete
		m.confirming = true
	}
	return m, nil
}

func (m *portfolioViewModel) View() string {
	// Summary panel.
	var summaryLines []string
	summaryLines = append(summaryLines, m.styles.Selected.Render(m.portfolio.Name))
	if m.portfolio.Description != "" {
		summaryLines = append(summaryLines, m.styles.Unselected.Render(m.portfolio.Description))
	}
	summaryLines = append(summaryLines, "")

	// Count instruments.
	var holdingCount, watchCount int
	for _, ins := range m.portfolio.Instruments {
		switch ins.Type {
		case portfolio.InstrumentHolding:
			holdingCount++
		case portfolio.InstrumentWatchlist:
			watchCount++
		}
	}
	summaryLines = append(summaryLines,
		fmt.Sprintf("  %-22s %d    %-22s %d",
			locale.T("portfolio.view.summary.holdings"), holdingCount,
			locale.T("portfolio.view.summary.watchlist"), watchCount,
		),
	)

	// Financial totals.
	var totalInvested, totalCurrent float64
	for _, ins := range m.portfolio.Instruments {
		if ins.Type != portfolio.InstrumentHolding {
			continue
		}
		for _, l := range ins.Lots {
			totalInvested += l.Quantity * l.Price
		}
		qty := ins.TotalQuantity()
		if price, ok := m.prices[ins.Symbol]; ok {
			totalCurrent += qty * price
		}
	}
	totalCurrent += m.portfolio.Cash
	unrealizedPnL := totalCurrent - totalInvested - m.portfolio.Cash

	summaryLines = append(summaryLines,
		fmt.Sprintf("  %-22s %.2f", locale.T("portfolio.view.summary.invested"), totalInvested),
		fmt.Sprintf("  %-22s %.2f", locale.T("portfolio.view.summary.cash"), m.portfolio.Cash),
	)

	if m.loadingPrices {
		summaryLines = append(summaryLines,
			fmt.Sprintf("  %-22s %s %s",
				locale.T("portfolio.view.summary.current_value"),
				locale.T("portfolio.view.price_loading"),
				m.spin.View(),
			),
		)
	} else if len(m.prices) > 0 || holdingCount == 0 {
		pnlStr := formatPnL(unrealizedPnL)
		summaryLines = append(summaryLines,
			fmt.Sprintf("  %-22s %.2f", locale.T("portfolio.view.summary.current_value"), totalCurrent),
			fmt.Sprintf("  %-22s %s", locale.T("portfolio.view.summary.unrealized_pnl"), pnlStr),
		)
	}

	summaryLines = append(summaryLines,
		fmt.Sprintf("  %-22s %.2f", locale.T("portfolio.view.summary.realized_pnl"), m.portfolio.RealizedPnL),
	)

	// AI model label.
	modelLabel := m.portfolio.AIModel
	if m.portfolio.AIProvider != "" && m.portfolio.AIModel != "" {
		modelLabel = fmt.Sprintf("%s / %s", m.portfolio.AIProvider, m.portfolio.AIModel)
	}
	summaryLines = append(summaryLines,
		fmt.Sprintf("  %-22s %s", locale.T("portfolio.view.summary.model"), modelLabel),
	)

	summary := m.styles.Preview.Render(strings.Join(summaryLines, "\n"))

	// Action menu.
	var rows []string
	for i, key := range portfolioViewActions {
		label := locale.T(key)
		if i == m.cursor {
			rows = append(rows, fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(label)))
		} else {
			rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(label)))
		}
	}

	parts := []string{summary, ""}
	parts = append(parts, rows...)
	parts = append(parts, "")

	if m.confirming {
		parts = append(parts, m.styles.Warning.Render(locale.T("portfolio.view.delete_confirm")))
	} else {
		if m.infoMsg != "" {
			parts = append(parts, m.styles.Error.Render(m.infoMsg))
		}
		parts = append(parts, m.styles.Hint.Render(locale.T("portfolio.view.hint")))
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// formatPnL formats a P&L value with a sign prefix.
func formatPnL(v float64) string {
	if v > 0 {
		return fmt.Sprintf("+%.2f", v)
	}
	return fmt.Sprintf("%.2f", v)
}
