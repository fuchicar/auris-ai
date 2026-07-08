package tui

import (
	"context"
	"errors"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"auris/pkg/locale"
	"auris/pkg/market"
	"auris/pkg/portfolio"
)

// PortfolioExportResult is emitted when the user exits the export screen.
type PortfolioExportResult struct {
	Action    string               // "back"
	Portfolio *portfolio.Portfolio // unchanged — export never mutates the portfolio
}

// portfolioExportQuotesMsg carries live quotes+fundamentals fetched for all
// holdings, used to enrich PortfolioMetrics before writing the export files.
type portfolioExportQuotesMsg struct {
	quotes map[string]portfolio.Quote
}

// portfolioExportWrittenMsg carries the outcome of writing the export files.
type portfolioExportWrittenMsg struct {
	paths []string
	err   error
}

// portfolioExportModel drives the "export" action from the portfolio view:
// fetch live quotes on open (same pattern as portfolioViewModel/
// portfolioWatchlistModel), compute PortfolioMetrics, then write a JSON
// export plus three CSVs (positions, lots, metrics) to the current working
// directory, and report the result.
type portfolioExportModel struct {
	portfolio *portfolio.Portfolio

	loading bool
	paths   []string
	errMsg  string
	done    bool

	spin   spinner.Model
	mp     market.ProviderAPI
	styles *Styles
}

func newPortfolioExportModel(p *portfolio.Portfolio, mp market.ProviderAPI, s *Styles) *portfolioExportModel {
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

	return &portfolioExportModel{
		portfolio: p,
		mp:        mp,
		styles:    s,
		spin:      sp,
		loading:   mp != nil && hasHoldings,
	}
}

func (m *portfolioExportModel) Init() tea.Cmd {
	if m.loading {
		return tea.Batch(m.spin.Tick, m.fetchQuotesCmd())
	}
	return m.writeCmd(map[string]portfolio.Quote{})
}

// fetchQuotesCmd fetches a quote+fundamentals per distinct holding symbol,
// mirroring the auto-fetch loop the portfolio_calculate_metrics tool uses in
// pkg/agent/tools.go (GetQuote, then GetFundamentals for dividend yield and
// beta; either failing just leaves that symbol out of valuation, non-fatal).
// Duplicated here rather than shared with pkg/agent since pkg/tui must not
// depend on the agent package.
func (m *portfolioExportModel) fetchQuotesCmd() tea.Cmd {
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
		quotes := make(map[string]portfolio.Quote, len(symbols))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if !mp.IsConnected() {
			_ = mp.Connect(ctx)
		}
		for _, sym := range symbols {
			q, err := mp.GetQuote(ctx, sym)
			if err != nil || q.Last <= 0 {
				continue
			}
			pq := portfolio.Quote{Last: q.Last}
			if f, fErr := mp.GetFundamentals(ctx, sym); fErr == nil {
				pq.DividendYieldTTM = f.DividendYieldTTM
				pq.Beta = f.Beta
			}
			quotes[sym] = pq
		}
		return portfolioExportQuotesMsg{quotes: quotes}
	}
}

// writeCmd computes PortfolioMetrics from the given quotes (empty/nil means
// an offline export — cost basis and realised P&L only) and writes the
// export files to the current working directory.
func (m *portfolioExportModel) writeCmd(quotes map[string]portfolio.Quote) tea.Cmd {
	p := m.portfolio
	return func() tea.Msg {
		metrics, err := portfolio.ComputeMetrics(p, quotes)
		if err != nil {
			return portfolioExportWrittenMsg{err: err}
		}
		metrics.ComputedAt = time.Now().UTC().Format(time.RFC3339)
		paths, err := portfolio.ExportFiles(".", p, metrics, time.Now())
		if err != nil {
			return portfolioExportWrittenMsg{err: err}
		}
		return portfolioExportWrittenMsg{paths: paths}
	}
}

func (m *portfolioExportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case portfolioExportQuotesMsg:
		m.loading = false
		return m, m.writeCmd(msg.quotes)

	case portfolioExportWrittenMsg:
		m.done = true
		m.paths = msg.paths
		switch {
		case errors.Is(msg.err, portfolio.ErrNoHoldings):
			m.errMsg = locale.T("portfolio.export.empty")
		case msg.err != nil:
			m.errMsg = msg.err.Error()
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *portfolioExportModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Type == tea.KeyEsc {
		p := m.portfolio
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioExport, Result: PortfolioExportResult{Action: "back", Portfolio: p}}
		}
	}
	return m, nil
}

func (m *portfolioExportModel) View() string {
	var parts []string
	parts = append(parts, m.styles.Selected.Render(locale.T("portfolio.export.title")), "")

	switch {
	case !m.done:
		parts = append(parts, m.spin.View()+" "+locale.T("portfolio.export.loading"))
	case m.errMsg != "":
		parts = append(parts, m.styles.Error.Render("✗ "+m.errMsg))
	default:
		for _, path := range m.paths {
			parts = append(parts, m.styles.Hint.Render("✓ "+path))
		}
	}

	parts = append(parts, "", m.styles.Hint.Render(locale.T("portfolio.export.hint")))
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
