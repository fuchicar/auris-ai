package tui

import (
	"fmt"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

// PortfolioTransactionsResult is emitted when the user exits the transaction
// history screen.
type PortfolioTransactionsResult struct {
	Action    string               // "back"
	Portfolio *portfolio.Portfolio // unchanged — read-only screen
}

// portfolioTransactionsModel shows a read-only, newest-first list of a
// portfolio's recorded transactions (buys, sells, dividends,
// deposits/withdrawals, adjustments).
type portfolioTransactionsModel struct {
	portfolio *portfolio.Portfolio
	rows      []portfolio.Transaction // newest-first, built once in the constructor
	cursor    int
	scrollOff int
	height    int // terminal height (updated by WindowSizeMsg)
	styles    *Styles
}

func newPortfolioTransactionsModel(p *portfolio.Portfolio, s *Styles) *portfolioTransactionsModel {
	rows := make([]portfolio.Transaction, len(p.Transactions))
	copy(rows, p.Transactions)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Date.After(rows[j].Date) })
	return &portfolioTransactionsModel{portfolio: p, rows: rows, styles: s}
}

func (m *portfolioTransactionsModel) Init() tea.Cmd {
	return nil
}

func (m *portfolioTransactionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = ws.Height
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		return m.handleKey(key)
	}
	return m, nil
}

func (m *portfolioTransactionsModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
		}
	case tea.KeyDown:
		if m.cursor < len(m.rows)-1 {
			m.cursor++
			m.scrollOff = clampScrollOff(m.cursor, m.scrollOff, m.maxVisible())
		}
	case tea.KeyEsc:
		p := m.portfolio
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioTransactions, Result: PortfolioTransactionsResult{Action: "back", Portfolio: p}}
		}
	}
	return m, nil
}

// txTypeLabel renders a TransactionType through locale instead of surfacing
// the raw internal identifier, matching this package's convention for every
// other enum-like field.
func txTypeLabel(t portfolio.TransactionType) string {
	return locale.T("portfolio.transactions.type." + string(t))
}

// formatTransactionRow renders a single transaction as a fixed-width table
// row, independent of cursor/style state, so it can be tested as a pure
// function.
func formatTransactionRow(tx portfolio.Transaction) string {
	pnl := "—"
	if tx.Type == portfolio.TransactionSell {
		pnl = formatPnL(tx.RealizedPnL)
	}
	qty, price := "—", "—"
	if tx.Type == portfolio.TransactionBuy || tx.Type == portfolio.TransactionSell {
		qty = fmt.Sprintf("%.6g", tx.Quantity)
		price = fmt.Sprintf("%.2f", tx.Price)
	}
	return fmt.Sprintf("%-10s %-11s %-6s %12s %10s %12s %12s",
		tx.Date.Format("2006-01-02"),
		txTypeLabel(tx.Type),
		tx.Symbol,
		qty,
		price,
		formatPnL(tx.CashDelta),
		pnl,
	)
}

// maxVisible returns the number of transaction rows that fit in the terminal
// after the title, blank separator, table header, hint, and the worst-case
// two scroll indicators are accounted for.
func (m *portfolioTransactionsModel) maxVisible() int {
	if m.height == 0 {
		return 20
	}
	// chrome above: title (1) + blank (1) + header (1) = 3.
	// chrome below: blank (1) + hint (1) = 2.
	n := m.height - 3 - 2 - 2 // reserve 2 lines for up+down indicators
	if n < 0 {
		return 0
	}
	return n
}

func (m *portfolioTransactionsModel) View() string {
	title := m.styles.Selected.Render(locale.T("portfolio.transactions.title"))

	if len(m.rows) == 0 {
		hint := m.styles.Hint.Render(locale.T("portfolio.transactions.empty"))
		navHint := m.styles.Hint.Render(locale.T("portfolio.transactions.hint"))
		return lipgloss.JoinVertical(lipgloss.Left, title, "", hint, "", navHint)
	}

	rows := windowedRows(WindowedRowOpts{
		Height:           m.height,
		ScrollOff:        m.scrollOff,
		Cursor:           m.cursor,
		Total:            len(m.rows),
		ChromeAbove:      3, // title (1) + blank (1) + header (1)
		ChromeBelow:      2, // blank (1) + hint (1)
		IndicatorReserve: 2,
		RenderRow:        m.renderRow,
		HintRender:       func(s string) string { return m.styles.Hint.Render(s) },
	})

	hint := m.styles.Hint.Render(locale.T("portfolio.transactions.hint"))
	parts := []string{title, "", m.viewHeader()}
	parts = append(parts, rows...)
	parts = append(parts, "", hint)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *portfolioTransactionsModel) viewHeader() string {
	header := fmt.Sprintf("%-10s %-11s %-6s %12s %10s %12s %12s",
		locale.T("portfolio.transactions.col.date"),
		locale.T("portfolio.transactions.col.type"),
		locale.T("portfolio.transactions.col.symbol"),
		locale.T("portfolio.transactions.col.qty"),
		locale.T("portfolio.transactions.col.price"),
		locale.T("portfolio.transactions.col.cash_delta"),
		locale.T("portfolio.transactions.col.realized_pnl"),
	)
	return m.styles.Hint.Render(header)
}

// renderRow returns the visible line for transaction index i, with the cursor
// arrow and styling applied.
func (m *portfolioTransactionsModel) renderRow(i int) string {
	line := formatTransactionRow(m.rows[i])
	if i == m.cursor {
		return fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(line))
	}
	return fmt.Sprintf("  %s", m.styles.Unselected.Render(line))
}
