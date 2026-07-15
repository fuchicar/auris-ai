package tui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/finance"
	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/market"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

// moneyColWidth is the fixed width money values are right-aligned to in the
// portfolio/instrument summary panels, wide enough for a symbol-prefixed,
// thousands-grouped 9-figure amount (e.g. "-$999,999,999.99").
const moneyColWidth = 17

// colorMoney right-pads s to moneyColWidth (so alignment survives the ANSI
// escape codes Render adds) then colors it Bull/Bear by the sign of v.
func colorMoney(s string, v float64, styles *Styles) string {
	padded := fmt.Sprintf("%*s", moneyColWidth, s)
	style := styles.Bull
	if v < 0 {
		style = styles.Bear
	}
	return style.Render(padded)
}

// PortfolioViewResult is emitted when the user chooses an action on the portfolio screen.
type PortfolioViewResult struct {
	Action    string               // "agent"|"instruments"|"allocation"|"transactions"|"watchlist"|"export"|"add"|"edit"|"deleted"
	Portfolio *portfolio.Portfolio // always set
}

// portfolioPricesMsg carries live quotes fetched for all holdings.
type portfolioPricesMsg struct {
	prices map[string]float64 // symbol → last price
	err    error
}

// portfolioSparklinesMsg carries recent daily closes fetched for all
// holdings, used to render a per-holding sparkline.
type portfolioSparklinesMsg struct {
	data map[string][]float64 // symbol → closes, oldest first
}

var portfolioViewActions = []string{
	"portfolio.view.action.agent",
	"portfolio.view.action.instruments",
	"portfolio.view.action.allocation",
	"portfolio.view.action.transactions",
	"portfolio.view.action.watchlist",
	"portfolio.view.action.export",
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

	sparklineData     map[string][]float64
	loadingSparklines bool

	infoMsg string
	spin    spinner.Model
	mp      market.ProviderAPI
	styles  *Styles
	height  int // terminal height (updated by WindowSizeMsg)
}

func newPortfolioViewModel(p *portfolio.Portfolio, mp market.ProviderAPI, s *Styles, height int) *portfolioViewModel {
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
		portfolio:         p,
		mp:                mp,
		styles:            s,
		spin:              sp,
		loadingPrices:     mp != nil && hasHoldings,
		loadingSparklines: mp != nil && hasHoldings,
		height:            height,
	}
}

func (m *portfolioViewModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.loadingPrices || m.loadingSparklines {
		cmds = append(cmds, m.spin.Tick)
	}
	if m.loadingPrices {
		cmds = append(cmds, m.fetchPricesCmd())
	}
	if m.loadingSparklines {
		cmds = append(cmds, m.fetchSparklinesCmd())
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
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

// fetchSparklinesCmd fetches a short window of recent daily closes per
// distinct held symbol, for the sparkline shown next to each holding.
// Note: this issues one GetCandles call per symbol, on top of the one
// GetQuote call fetchPricesCmd already makes — opening this screen with N
// holdings costs roughly 2N market API calls. The window is kept short (~1
// month) to keep the added cost small under FMP's free-tier rate limit.
func (m *portfolioViewModel) fetchSparklinesCmd() tea.Cmd {
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
		data := make(map[string][]float64, len(symbols))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if !mp.IsConnected() {
			_ = mp.Connect(ctx)
		}
		to := time.Now()
		from := to.AddDate(0, -1, 0)
		for _, sym := range symbols {
			candles, err := mp.GetCandles(ctx, sym, from, to, market.Timeframe1d)
			if err != nil || len(candles) == 0 {
				continue
			}
			closes := make([]float64, len(candles))
			for i, c := range candles {
				closes[i] = c.Close
			}
			data[sym] = closes
		}
		return portfolioSparklinesMsg{data: data}
	}
}

func (m *portfolioViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		if m.loadingPrices || m.loadingSparklines {
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

	case portfolioSparklinesMsg:
		m.loadingSparklines = false
		m.sparklineData = msg.data
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
	case 3: // transactions
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioView, Result: PortfolioViewResult{Action: "transactions", Portfolio: p}}
		}
	case 4: // watchlist
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioView, Result: PortfolioViewResult{Action: "watchlist", Portfolio: p}}
		}
	case 5: // export
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioView, Result: PortfolioViewResult{Action: "export", Portfolio: p}}
		}
	case 6: // add
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioView, Result: PortfolioViewResult{Action: "add", Portfolio: p}}
		}
	case 7: // edit
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioView, Result: PortfolioViewResult{Action: "edit", Portfolio: p}}
		}
	case 8: // delete
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

	cur := m.portfolio.Currency
	summaryLines = append(summaryLines,
		fmt.Sprintf("  %-22s %*s", locale.T("portfolio.view.summary.invested"), moneyColWidth, finance.FormatMoney(totalInvested, cur)),
		fmt.Sprintf("  %-22s %*s", locale.T("portfolio.view.summary.cash"), moneyColWidth, finance.FormatMoney(m.portfolio.Cash, cur)),
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
		pnlStr := colorMoney(finance.FormatMoneySigned(unrealizedPnL, cur), unrealizedPnL, m.styles)
		summaryLines = append(summaryLines,
			fmt.Sprintf("  %-22s %*s", locale.T("portfolio.view.summary.current_value"), moneyColWidth, finance.FormatMoney(totalCurrent, cur)),
			fmt.Sprintf("  %-22s %s", locale.T("portfolio.view.summary.unrealized_pnl"), pnlStr),
		)
	}

	realizedStr := colorMoney(finance.FormatMoneySigned(m.portfolio.RealizedPnL, cur), m.portfolio.RealizedPnL, m.styles)
	summaryLines = append(summaryLines,
		fmt.Sprintf("  %-22s %s", locale.T("portfolio.view.summary.realized_pnl"), realizedStr),
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

	// Trailing block (delete confirmation, or an info message plus the hint
	// line) — built ahead of the holdings panel so its height can be
	// subtracted from the holdings row budget below.
	var trailing []string
	if m.confirming {
		trailing = append(trailing, m.styles.Warning.Render(locale.T("portfolio.view.delete_confirm")))
	} else {
		if m.infoMsg != "" {
			trailing = append(trailing, m.styles.Error.Render(m.infoMsg))
		}
		trailing = append(trailing, m.styles.Hint.Render(locale.T("portfolio.view.hint")))
	}

	// How many holdings rows fit before the screen overflows the terminal.
	// The summary panel, action menu, and trailing block always render in
	// full; only the holdings panel shrinks. Unknown height (e.g. a screen
	// built without ever receiving a WindowSizeMsg, as in tests) means
	// "don't truncate".
	holdingsBudget := math.MaxInt
	if m.height > 0 {
		const separators = 3           // blank lines before the holdings panel, the menu, and the trailing block
		const holdingsBorderOverhead = 2 // Preview style's RoundedBorder top+bottom
		const safetyMargin = 1
		holdingsBudget = m.height - lipgloss.Height(summary) - len(rows) - len(trailing) - separators - holdingsBorderOverhead - safetyMargin
		if holdingsBudget < 1 {
			holdingsBudget = 1
		}
	}

	parts := []string{summary}
	if holdingsPanel := m.viewHoldingsSparklines(holdingsBudget); holdingsPanel != "" {
		parts = append(parts, "", holdingsPanel)
	}
	parts = append(parts, "")
	parts = append(parts, rows...)
	parts = append(parts, "")
	parts = append(parts, trailing...)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// viewHoldingsSparklines renders one entry per distinct held symbol with its
// last price beside a recent-price sparkline. renderSparkline's output is
// sparklineHeight (3) rows tall, so each entry — not each line — spans
// several terminal rows; entries are joined horizontally (not interpolated
// into a single %s line) so the symbol/price label stays aligned next to the
// chart instead of being clobbered by its embedded newlines. Returns "" if
// there are no holdings to show. If the full list would exceed maxRows
// terminal rows, trailing entries are dropped and a final "+N more" row
// points the user at the full-list menu action instead.
func (m *portfolioViewModel) viewHoldingsSparklines(maxRows int) string {
	var entries []string
	seen := make(map[string]bool)
	for _, ins := range m.portfolio.Instruments {
		if ins.Type != portfolio.InstrumentHolding || seen[ins.Symbol] {
			continue
		}
		seen[ins.Symbol] = true
		priceStr := locale.T("portfolio.view.price_unavailable")
		if p, ok := m.prices[ins.Symbol]; ok {
			priceStr = finance.FormatMoney(p, m.portfolio.Currency)
		}
		header := fmt.Sprintf("  %-8s %12s  ", ins.Symbol, priceStr)
		entry := header
		if spark := renderSparkline(m.sparklineData[ins.Symbol], m.styles); spark != "" {
			entry = lipgloss.JoinHorizontal(lipgloss.Center, header, spark)
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return ""
	}

	var shown []string
	usedRows := 0
	for i, e := range entries {
		h := lipgloss.Height(e)
		if usedRows+h > maxRows && len(shown) > 0 {
			remaining := len(entries) - i
			shown = append(shown, "  "+m.styles.Hint.Render(locale.Tp("portfolio.view.holdings_more", map[string]any{"Count": remaining})))
			return m.styles.Preview.Render(strings.Join(shown, "\n"))
		}
		shown = append(shown, e)
		usedRows += h
	}
	return m.styles.Preview.Render(strings.Join(shown, "\n"))
}

// formatPnL formats a P&L value with a sign prefix.
func formatPnL(v float64) string {
	if v > 0 {
		return fmt.Sprintf("+%.2f", v)
	}
	return fmt.Sprintf("%.2f", v)
}
