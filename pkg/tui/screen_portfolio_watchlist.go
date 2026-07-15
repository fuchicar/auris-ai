package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/market"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

// PortfolioWatchlistResult is emitted when the user exits the watchlist screen.
type PortfolioWatchlistResult struct {
	Action    string               // "back"
	Portfolio *portfolio.Portfolio // unchanged — read-only screen
}

// watchlistQuotesMsg carries live quotes fetched for all watchlist symbols.
type watchlistQuotesMsg struct {
	quotes map[string]market.Quote // symbol → quote
}

// portfolioWatchlistModel shows a read-only, on-demand list of a portfolio's
// watchlist instruments with their live price and %/day change, fetched once
// when the screen opens (same fetch pattern as portfolioViewModel's holdings).
type portfolioWatchlistModel struct {
	portfolio *portfolio.Portfolio
	rows      []portfolio.Instrument // distinct InstrumentWatchlist entries, in portfolio order
	cursor    int

	quotes        map[string]market.Quote
	loadingPrices bool

	spin   spinner.Model
	mp     market.ProviderAPI
	styles *Styles
}

func newPortfolioWatchlistModel(p *portfolio.Portfolio, mp market.ProviderAPI, s *Styles) *portfolioWatchlistModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = s.Spinner

	var rows []portfolio.Instrument
	seen := make(map[string]bool)
	for _, ins := range p.Instruments {
		if ins.Type == portfolio.InstrumentWatchlist && !seen[ins.Symbol] {
			rows = append(rows, ins)
			seen[ins.Symbol] = true
		}
	}

	return &portfolioWatchlistModel{
		portfolio:     p,
		rows:          rows,
		mp:            mp,
		styles:        s,
		spin:          sp,
		loadingPrices: mp != nil && len(rows) > 0,
	}
}

func (m *portfolioWatchlistModel) Init() tea.Cmd {
	if !m.loadingPrices {
		return nil
	}
	return tea.Batch(m.spin.Tick, m.fetchQuotesCmd())
}

func (m *portfolioWatchlistModel) fetchQuotesCmd() tea.Cmd {
	symbols := make([]string, len(m.rows))
	for i, ins := range m.rows {
		symbols[i] = ins.Symbol
	}
	mp := m.mp
	return func() tea.Msg {
		quotes := make(map[string]market.Quote, len(symbols))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if !mp.IsConnected() {
			_ = mp.Connect(ctx)
		}
		for _, sym := range symbols {
			q, err := mp.GetQuote(ctx, sym)
			if err == nil && q.Last > 0 {
				quotes[sym] = q
			}
		}
		return watchlistQuotesMsg{quotes: quotes}
	}
}

func (m *portfolioWatchlistModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.loadingPrices {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case watchlistQuotesMsg:
		m.loadingPrices = false
		m.quotes = msg.quotes
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *portfolioWatchlistModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case tea.KeyEsc:
		p := m.portfolio
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioWatchlist, Result: PortfolioWatchlistResult{Action: "back", Portfolio: p}}
		}
	}
	return m, nil
}

// formatWatchlistRow renders a single watchlist row's symbol/name/price as
// fixed-width plain text and the %/day change as a separate string, as a pure
// function independent of cursor/color state, so it can be tested by string
// equality. hasQuote distinguishes "quote not fetched yet / unavailable"
// (renders placeholders) from an actual zero-change quote.
func formatWatchlistRow(ins portfolio.Instrument, q market.Quote, hasQuote bool) (line string, changeText string, changeNonNegative bool) {
	price := locale.T("portfolio.watchlist.price_unavailable")
	change := locale.T("portfolio.watchlist.price_unavailable")
	nonNeg := true
	if hasQuote {
		price = fmt.Sprintf("%.2f", q.Last)
		change = fmt.Sprintf("%+.2f%%", q.ChangePercent)
		nonNeg = q.ChangePercent >= 0
	}
	line = fmt.Sprintf("%-8s %-24s %10s", ins.Symbol, ins.Name, price)
	return line, change, nonNeg
}

func (m *portfolioWatchlistModel) viewRows() []string {
	rows := make([]string, 0, len(m.rows))
	for i, ins := range m.rows {
		q, ok := m.quotes[ins.Symbol]
		line, changeText, nonNeg := formatWatchlistRow(ins, q, ok)

		changeStyle := m.styles.Bear
		if nonNeg {
			changeStyle = m.styles.Bull
		}
		coloredChange := changeStyle.Render(fmt.Sprintf("%10s", changeText))

		var lineRendered string
		if i == m.cursor {
			lineRendered = fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(line))
		} else {
			lineRendered = fmt.Sprintf("  %s", m.styles.Unselected.Render(line))
		}
		rows = append(rows, lineRendered+" "+coloredChange)
	}
	return rows
}

func (m *portfolioWatchlistModel) viewHeader() string {
	header := fmt.Sprintf("%-8s %-24s %10s %10s",
		locale.T("portfolio.watchlist.col.symbol"),
		locale.T("portfolio.watchlist.col.name"),
		locale.T("portfolio.watchlist.col.price"),
		locale.T("portfolio.watchlist.col.change"),
	)
	return m.styles.Hint.Render(header)
}

func (m *portfolioWatchlistModel) View() string {
	var parts []string
	parts = append(parts, m.styles.Selected.Render(locale.T("portfolio.watchlist.title")), "")

	if len(m.rows) == 0 {
		parts = append(parts, m.styles.Hint.Render(locale.T("portfolio.watchlist.empty")))
	} else {
		if m.loadingPrices {
			parts = append(parts,
				fmt.Sprintf("%s %s", locale.T("portfolio.watchlist.price_loading"), m.spin.View()),
				"",
			)
		}
		parts = append(parts, m.viewHeader())
		parts = append(parts, m.viewRows()...)
	}

	parts = append(parts, "", m.styles.Hint.Render(locale.T("portfolio.watchlist.hint")))
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
