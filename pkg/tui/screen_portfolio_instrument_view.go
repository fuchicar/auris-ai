package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fuchicar/auris-ai/pkg/finance"
	"github.com/fuchicar/auris-ai/pkg/locale"
	"github.com/fuchicar/auris-ai/pkg/market"
	"github.com/fuchicar/auris-ai/pkg/portfolio"
)

// PortfolioInstrumentViewResult is emitted when the user exits the instrument view.
type PortfolioInstrumentViewResult struct {
	Action    string               // "back" | "deleted"
	Portfolio *portfolio.Portfolio // updated copy (already saved)
}

// instrumentViewMode tracks which inline form is active.
type instrumentViewMode int

const (
	ivModeMenu          instrumentViewMode = iota // main action menu
	ivModeSellQty                                 // entering sell quantity
	ivModeSellPrice                               // entering sell price
	ivModeAddQty                                  // entering lot quantity
	ivModeAddPrice                                // entering lot price
	ivModeAddDate                                 // entering lot date
	ivModeConfirmDelete                           // confirming instrument removal
	ivModeConfirmType                             // confirming type change
)

var instrumentActions = []string{
	"portfolio.instrument.action.add_lot",
	"portfolio.instrument.action.sell",
	"portfolio.instrument.action.change_type",
	"portfolio.instrument.action.delete",
}

var watchlistInstrumentActions = []string{
	"portfolio.instrument.action.buy",
	"portfolio.instrument.action.change_type",
	"portfolio.instrument.action.delete",
}

// portfolioInstrumentViewModel shows a single instrument with lot details and inline actions.
type portfolioInstrumentViewModel struct {
	portfolio  *portfolio.Portfolio
	instrument *portfolio.Instrument // pointer into portfolio.Instruments
	mode       instrumentViewMode
	cursor     int
	// lotCursor / lotScrollOff track the windowed position inside the per-lot
	// table; separate from cursor, which navigates the action menu. They live
	// here because the lot list is only meaningful in the menu mode — when the
	// user enters a sell/add input flow they go back to (0, 0) so the next
	// time the menu renders, the table starts at the top.
	lotCursor   int
	lotScrollOff int
	infoMsg     string
	infoIsErr   bool
	price       float64 // live price if available
	loadingPx   bool

	candles        []market.Candle // recent daily candles for the chart
	loadingCandles bool

	// sell inputs
	sellQtyInput   textinput.Model
	sellPriceInput textinput.Model
	sellQtyErr     string
	sellPriceErr   string

	// add lot inputs
	addQtyInput   textinput.Model
	addPriceInput textinput.Model
	addDateInput  textinput.Model
	addQtyErr     string
	addPriceErr   string
	addDateErr    string

	width  int // terminal width (updated by WindowSizeMsg)
	height int // terminal height (updated by WindowSizeMsg)
	spin   spinner.Model
	mp     market.ProviderAPI
	styles *Styles
}

func newPortfolioInstrumentViewModel(
	p *portfolio.Portfolio,
	ins *portfolio.Instrument,
	mp market.ProviderAPI,
	s *Styles,
	width, height int,
) *portfolioInstrumentViewModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = s.Spinner

	newInput := func(placeholder string) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.CharLimit = 30
		return ti
	}

	return &portfolioInstrumentViewModel{
		portfolio:      p,
		instrument:     ins,
		mp:             mp,
		styles:         s,
		spin:           sp,
		loadingPx:      mp != nil && ins.Type == portfolio.InstrumentHolding,
		loadingCandles: mp != nil,
		sellQtyInput:   newInput("10"),
		sellPriceInput: newInput("150.00"),
		addQtyInput:    newInput("10"),
		addPriceInput:  newInput("150.00"),
		addDateInput:   newInput(time.Now().Format("2006-01-02")),
		width:          width,
		height:         height,
	}
}

func (m *portfolioInstrumentViewModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.loadingPx || m.loadingCandles {
		cmds = append(cmds, m.spin.Tick)
	}
	if m.loadingPx {
		cmds = append(cmds, m.fetchPriceCmd())
	}
	if m.loadingCandles {
		cmds = append(cmds, m.fetchCandlesCmd())
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *portfolioInstrumentViewModel) fetchPriceCmd() tea.Cmd {
	sym := m.instrument.Symbol
	mp := m.mp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if !mp.IsConnected() {
			_ = mp.Connect(ctx)
		}
		q, err := mp.GetQuote(ctx, sym)
		if err != nil || q.Last == 0 {
			return portfolioPricesMsg{prices: map[string]float64{}}
		}
		return portfolioPricesMsg{prices: map[string]float64{sym: q.Last}}
	}
}

// instrumentCandlesMsg carries the recent daily candles fetched for the
// chart shown in the instrument detail view.
type instrumentCandlesMsg struct {
	symbol  string
	candles []market.Candle
	err     error
}

func (m *portfolioInstrumentViewModel) fetchCandlesCmd() tea.Cmd {
	sym := m.instrument.Symbol
	mp := m.mp
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if !mp.IsConnected() {
			_ = mp.Connect(ctx)
		}
		to := time.Now()
		from := to.AddDate(0, -3, 0) // ~3 months daily: SMA(20) warm-up + readable candle width
		candles, err := mp.GetCandles(ctx, sym, from, to, market.Timeframe1d)
		return instrumentCandlesMsg{symbol: sym, candles: candles, err: err}
	}
}

func (m *portfolioInstrumentViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		if m.loadingPx || m.loadingCandles {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case portfolioPricesMsg:
		m.loadingPx = false
		if p, ok := msg.prices[m.instrument.Symbol]; ok {
			m.price = p
		}
		return m, nil

	case instrumentCandlesMsg:
		m.loadingCandles = false
		if msg.err == nil && msg.symbol == m.instrument.Symbol {
			m.candles = msg.candles
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *portfolioInstrumentViewModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case ivModeMenu:
		return m.handleMenu(key)
	case ivModeSellQty:
		return m.handleSellQty(key)
	case ivModeSellPrice:
		return m.handleSellPrice(key)
	case ivModeAddQty:
		return m.handleAddQty(key)
	case ivModeAddPrice:
		return m.handleAddPrice(key)
	case ivModeAddDate:
		return m.handleAddDate(key)
	case ivModeConfirmDelete:
		return m.handleConfirmDelete(key)
	case ivModeConfirmType:
		return m.handleConfirmType(key)
	}
	return m, nil
}

func (m *portfolioInstrumentViewModel) handleMenu(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Only show sell/add_lot for holdings.
	actions := m.visibleActions()
	// Lot-table navigation keys (j/k, PageUp/PageDown, g/G) are intercepted
	// when there's something to scroll — they don't move the action cursor
	// nor trigger any menu action, so the user can walk the table without
	// accidentally firing the delete/change_type confirmations.
	if m.instrument.Type == portfolio.InstrumentHolding && len(m.instrument.Lots) > 0 {
		switch key.Type {
		case tea.KeyRunes:
			switch key.String() {
			case "j":
				if m.lotCursor < len(m.instrument.Lots)-1 {
					m.lotCursor++
				}
				return m, nil
			case "k":
				if m.lotCursor > 0 {
					m.lotCursor--
				}
				return m, nil
			case "g":
				m.lotCursor = 0
				return m, nil
			case "G":
				m.lotCursor = len(m.instrument.Lots) - 1
				return m, nil
			}
		case tea.KeyPgDown:
			m.lotCursor += 5
			if m.lotCursor > len(m.instrument.Lots)-1 {
				m.lotCursor = len(m.instrument.Lots) - 1
			}
			return m, nil
		case tea.KeyPgUp:
			m.lotCursor -= 5
			if m.lotCursor < 0 {
				m.lotCursor = 0
			}
			return m, nil
		}
	}
	switch key.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor < len(actions)-1 {
			m.cursor++
		}
	case tea.KeyEsc:
		p := m.portfolio
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioInstrumentView, Result: PortfolioInstrumentViewResult{Action: "back", Portfolio: p}}
		}
	case tea.KeyEnter:
		return m.selectAction(actions[m.cursor])
	}
	return m, nil
}

// visibleActions returns the action keys visible for this instrument type.
func (m *portfolioInstrumentViewModel) visibleActions() []string {
	if m.instrument.Type == portfolio.InstrumentHolding {
		return instrumentActions // all 4
	}
	// Watchlist: offer buy instead of add_lot/sell.
	return watchlistInstrumentActions
}

func (m *portfolioInstrumentViewModel) selectAction(actionKey string) (tea.Model, tea.Cmd) {
	switch actionKey {
	case "portfolio.instrument.action.add_lot", "portfolio.instrument.action.buy":
		m.mode = ivModeAddQty
		m.addQtyInput.Focus()
		return m, textinput.Blink
	case "portfolio.instrument.action.sell":
		m.mode = ivModeSellQty
		m.sellQtyInput.Focus()
		return m, textinput.Blink
	case "portfolio.instrument.action.change_type":
		m.mode = ivModeConfirmType
	case "portfolio.instrument.action.delete":
		m.mode = ivModeConfirmDelete
	}
	return m, nil
}

// ─── Sell flow ────────────────────────────────────────────────────────────────

func (m *portfolioInstrumentViewModel) handleSellQty(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		m.mode = ivModeMenu
		m.sellQtyInput.Blur()
		m.sellQtyInput.SetValue("")
		return m, nil
	case tea.KeyEnter:
		val := strings.TrimSpace(m.sellQtyInput.Value())
		qty, err := strconv.ParseFloat(val, 64)
		maxQty := m.instrument.TotalQuantity()
		if err != nil || qty <= 0 || qty > maxQty+1e-9 {
			m.sellQtyErr = locale.Tp("portfolio.instrument.sell.qty.error", map[string]any{"Max": fmt.Sprintf("%.6g", maxQty)})
			return m, nil
		}
		m.sellQtyErr = ""
		m.mode = ivModeSellPrice
		m.sellQtyInput.Blur()
		m.sellPriceInput.Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.sellQtyInput, cmd = m.sellQtyInput.Update(key)
	return m, cmd
}

func (m *portfolioInstrumentViewModel) handleSellPrice(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		m.mode = ivModeSellQty
		m.sellPriceInput.Blur()
		m.sellQtyInput.Focus()
		return m, textinput.Blink
	case tea.KeyEnter:
		val := strings.TrimSpace(m.sellPriceInput.Value())
		price, err := strconv.ParseFloat(val, 64)
		if err != nil || price <= 0 {
			m.sellPriceErr = locale.T("portfolio.instrument.sell.price.error")
			return m, nil
		}
		qty, _ := strconv.ParseFloat(strings.TrimSpace(m.sellQtyInput.Value()), 64)
		res, err := portfolio.ApplyFIFOSell(m.instrument.Lots, qty, price)
		if err != nil {
			m.sellPriceErr = locale.T("portfolio.error.insufficient_lots")
			return m, nil
		}
		m.instrument.Lots = res.RemainingLots
		m.portfolio.RealizedPnL += res.RealizedPnL
		m.portfolio.RecordTransaction(portfolio.Transaction{
			Type: portfolio.TransactionSell, Symbol: m.instrument.Symbol,
			Quantity: qty, Price: price, CashDelta: qty * price,
			RealizedPnL: res.RealizedPnL, ConsumedLots: res.ConsumedLots,
			Date: time.Now(),
		})
		if err := portfolio.SavePortfolio(m.portfolio); err != nil {
			m.infoMsg = locale.Tp("portfolio.error.save", map[string]any{"Error": err.Error()})
			m.infoIsErr = true
		} else {
			m.infoMsg = locale.Tp("portfolio.instrument.sell.success", map[string]any{
				"PnL": finance.FormatMoneySigned(res.RealizedPnL, m.portfolio.Currency),
			})
			m.infoIsErr = false
		}
		m.mode = ivModeMenu
		m.sellPriceInput.Blur()
		m.sellPriceInput.SetValue("")
		m.sellQtyInput.SetValue("")
		m.sellPriceErr = ""
		m.cursor = 0
		return m, nil
	}
	var cmd tea.Cmd
	m.sellPriceInput, cmd = m.sellPriceInput.Update(key)
	return m, cmd
}

// ─── Add lot flow ─────────────────────────────────────────────────────────────

func (m *portfolioInstrumentViewModel) handleAddQty(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		m.mode = ivModeMenu
		m.addQtyInput.Blur()
		m.addQtyInput.SetValue("")
		return m, nil
	case tea.KeyEnter:
		val := strings.TrimSpace(m.addQtyInput.Value())
		qty, err := strconv.ParseFloat(val, 64)
		if err != nil || qty <= 0 {
			m.addQtyErr = locale.T("portfolio.search.step.qty.error")
			return m, nil
		}
		m.addQtyErr = ""
		m.mode = ivModeAddPrice
		m.addQtyInput.Blur()
		m.addPriceInput.Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.addQtyInput, cmd = m.addQtyInput.Update(key)
	return m, cmd
}

func (m *portfolioInstrumentViewModel) handleAddPrice(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		m.mode = ivModeAddQty
		m.addPriceInput.Blur()
		m.addQtyInput.Focus()
		return m, textinput.Blink
	case tea.KeyEnter:
		val := strings.TrimSpace(m.addPriceInput.Value())
		price, err := strconv.ParseFloat(val, 64)
		if err != nil || price <= 0 {
			m.addPriceErr = locale.T("portfolio.search.step.qty.error")
			return m, nil
		}
		m.addPriceErr = ""
		m.mode = ivModeAddDate
		m.addPriceInput.Blur()
		m.addDateInput.SetValue(time.Now().Format("2006-01-02"))
		m.addDateInput.Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.addPriceInput, cmd = m.addPriceInput.Update(key)
	return m, cmd
}

func (m *portfolioInstrumentViewModel) handleAddDate(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		return m.finishAddLot(time.Now()) // use today if escaped
	case tea.KeyEnter:
		val := strings.TrimSpace(m.addDateInput.Value())
		if val == "" {
			return m.finishAddLot(time.Now())
		}
		t, err := time.Parse("2006-01-02", val)
		if err != nil {
			m.addDateErr = locale.T("portfolio.instrument.add_lot.date.error")
			return m, nil
		}
		return m.finishAddLot(t)
	}
	var cmd tea.Cmd
	m.addDateInput, cmd = m.addDateInput.Update(key)
	return m, cmd
}

func (m *portfolioInstrumentViewModel) finishAddLot(date time.Time) (tea.Model, tea.Cmd) {
	qty, _ := strconv.ParseFloat(strings.TrimSpace(m.addQtyInput.Value()), 64)
	price, _ := strconv.ParseFloat(strings.TrimSpace(m.addPriceInput.Value()), 64)
	wasWatchlist := m.instrument.Type == portfolio.InstrumentWatchlist
	lot := portfolio.NewLot(qty, price, date)
	m.instrument.Lots = append(m.instrument.Lots, lot)
	if wasWatchlist {
		m.instrument.Type = portfolio.InstrumentHolding
	}
	m.portfolio.RecordTransaction(portfolio.Transaction{
		Type: portfolio.TransactionBuy, Symbol: m.instrument.Symbol,
		Quantity: qty, Price: price, CashDelta: -qty * price, Date: date,
	})
	if err := portfolio.SavePortfolio(m.portfolio); err != nil {
		m.infoMsg = locale.Tp("portfolio.error.save", map[string]any{"Error": err.Error()})
		m.infoIsErr = true
	} else if wasWatchlist {
		m.infoMsg = locale.T("portfolio.instrument.buy.success")
		m.infoIsErr = false
	} else {
		m.infoMsg = locale.T("portfolio.instrument.add_lot.success")
		m.infoIsErr = false
	}
	m.mode = ivModeMenu
	m.addQtyInput.Blur()
	m.addQtyInput.SetValue("")
	m.addPriceInput.SetValue("")
	m.addDateInput.SetValue("")
	m.addDateErr = ""
	m.cursor = 0
	return m, nil
}

// ─── Type change / delete ─────────────────────────────────────────────────────

func (m *portfolioInstrumentViewModel) handleConfirmType(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Type == tea.KeyEsc:
		m.mode = ivModeMenu
	case key.Type == tea.KeyRunes && (key.String() == "y" || key.String() == "s"):
		if m.instrument.Type == portfolio.InstrumentHolding {
			m.instrument.Type = portfolio.InstrumentWatchlist
			m.instrument.Lots = nil
			m.infoMsg = locale.T("portfolio.instrument.change_type.to_watchlist")
		} else {
			m.instrument.Type = portfolio.InstrumentHolding
			m.infoMsg = locale.T("portfolio.instrument.change_type.to_holding")
		}
		m.infoIsErr = false
		_ = portfolio.SavePortfolio(m.portfolio)
		m.mode = ivModeMenu
		m.cursor = 0
	default:
		m.mode = ivModeMenu
	}
	return m, nil
}

func (m *portfolioInstrumentViewModel) handleConfirmDelete(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Type == tea.KeyEsc:
		m.mode = ivModeMenu
	case key.Type == tea.KeyRunes && (key.String() == "y" || key.String() == "s"):
		// Remove instrument from portfolio.
		newInstruments := make([]portfolio.Instrument, 0, len(m.portfolio.Instruments)-1)
		for _, ins := range m.portfolio.Instruments {
			if ins.ID != m.instrument.ID {
				newInstruments = append(newInstruments, ins)
			}
		}
		m.portfolio.Instruments = newInstruments
		_ = portfolio.SavePortfolio(m.portfolio)
		p := m.portfolio
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenPortfolioInstrumentView, Result: PortfolioInstrumentViewResult{Action: "deleted", Portfolio: p}}
		}
	default:
		m.mode = ivModeMenu
	}
	return m, nil
}

// ─── View ─────────────────────────────────────────────────────────────────────

// lotMoneyWidth is the fixed width money columns are right-aligned to in the
// per-lot table (kept narrower than moneyColWidth since per-lot amounts are
// smaller than portfolio-level totals).
const lotMoneyWidth = 14

// viewChromeless returns the static top and bottom chrome the screen always
// renders (header line, separator, and the action menu / input / confirm
// prompt depending on m.mode). Used by View() to compute the vertical
// budget left for the chart and the lot table — see View() for the full
// priority order. The chrome slice has at most the items View() always
// shows: header + blank + (menu OR input block OR confirm) + (info message).
// The menu block is len(visibleActions()) + 1 blank + 1 hint ≈ 6 rows for a
// holding; the input blocks are 3 rows; confirms are 2. infoMsg adds 1.
func (m *portfolioInstrumentViewModel) viewChromeless() []string {
	var chrome []string
	chrome = append(chrome, m.viewHeader())
	chrome = append(chrome, "")
	switch m.mode {
	case ivModeMenu:
		chrome = append(chrome, m.viewMenu()...)
	case ivModeSellQty:
		chrome = append(chrome, m.viewSellQty()...)
	case ivModeSellPrice:
		chrome = append(chrome, m.viewSellPrice()...)
	case ivModeAddQty:
		chrome = append(chrome, m.viewAddQty()...)
	case ivModeAddPrice:
		chrome = append(chrome, m.viewAddPrice()...)
	case ivModeAddDate:
		chrome = append(chrome, m.viewAddDate()...)
	case ivModeConfirmDelete:
		chrome = append(chrome, m.styles.Warning.Render(locale.T("portfolio.instrument.delete_confirm")))
	case ivModeConfirmType:
		if m.instrument.Type == portfolio.InstrumentHolding {
			chrome = append(chrome, m.styles.Warning.Render(locale.T("portfolio.instrument.change_type.to_watchlist")))
		} else {
			chrome = append(chrome, m.styles.Warning.Render(locale.T("portfolio.instrument.change_type.to_holding")))
		}
		chrome = append(chrome, m.styles.Hint.Render("y/s confirm · other cancel"))
	}
	if m.infoMsg != "" {
		if m.infoIsErr {
			chrome = append(chrome, m.styles.Error.Render(m.infoMsg))
		} else {
			chrome = append(chrome, m.styles.Hint.Render(m.infoMsg))
		}
	}
	return chrome
}

// viewHeader renders the badge + symbol + name line that sits at the top of
// the screen. Extracted so the header is also accounted for in the vertical
// budget — without it, View()'s first "header + blank" could itself exceed
// the terminal and still push chrome off the top.
func (m *portfolioInstrumentViewModel) viewHeader() string {
	var badgeKey string
	switch m.instrument.Type {
	case portfolio.InstrumentHolding:
		badgeKey = "portfolio.instrument.badge.holding"
	default:
		badgeKey = "portfolio.instrument.badge.watchlist"
	}
	badge := m.styles.Checkbox.Render(fmt.Sprintf("[%s]", locale.T(badgeKey)))
	return fmt.Sprintf("%s %s — %s", badge, m.styles.Selected.Render(m.instrument.Symbol), m.instrument.Name)
}

func (m *portfolioInstrumentViewModel) View() string {
	// Outside menu mode the screen is small (input/confirm), so it never
	// overflows on its own — render the chrome plus whatever lot/summary
	// data the user can still glance at without making the screen scroll.
	if m.mode != ivModeMenu {
		parts := []string{m.viewHeader(), ""}
		if m.instrument.Type == portfolio.InstrumentHolding {
			parts = append(parts, m.viewLotTableWindowed(m.height)...)
			parts = append(parts, "")
			parts = append(parts, m.viewSummary()...)
			parts = append(parts, "")
		}
		switch m.mode {
		case ivModeSellQty:
			parts = append(parts, m.viewSellQty()...)
		case ivModeSellPrice:
			parts = append(parts, m.viewSellPrice()...)
		case ivModeAddQty:
			parts = append(parts, m.viewAddQty()...)
		case ivModeAddPrice:
			parts = append(parts, m.viewAddPrice()...)
		case ivModeAddDate:
			parts = append(parts, m.viewAddDate()...)
		case ivModeConfirmDelete:
			parts = append(parts, m.styles.Warning.Render(locale.T("portfolio.instrument.delete_confirm")))
		case ivModeConfirmType:
			if m.instrument.Type == portfolio.InstrumentHolding {
				parts = append(parts, m.styles.Warning.Render(locale.T("portfolio.instrument.change_type.to_watchlist")))
			} else {
				parts = append(parts, m.styles.Warning.Render(locale.T("portfolio.instrument.change_type.to_holding")))
			}
			parts = append(parts, m.styles.Hint.Render("y/s confirm · other cancel"))
		}
		if m.infoMsg != "" {
			if m.infoIsErr {
				parts = append(parts, m.styles.Error.Render(m.infoMsg))
			} else {
				parts = append(parts, m.styles.Hint.Render(m.infoMsg))
			}
		}
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	// Menu mode: build the screen in priority order, then shrink chart and
	// window lots until the result fits.
	//
	// Priority:
	//   1. Header + summary + menu + info (always; the chrome).
	//   2. Chart (degrade: full → medium → min → hint → "").
	//   3. Lot table (windowed).
	//
	// The trick is to compute the chrome height from the rendered strings
	// (not estimates) so we don't miscount blanks. Once we know the chrome
	// height, the chart+lot budget is m.height - chromeH minus one blank
	// line for each non-empty block that follows.
	header := m.viewHeader()
	summary := m.viewSummary()
	menuLines := m.viewMenu()
	infoLine := m.infoLine()

	chrome := []string{header}
	if len(summary) > 0 {
		chrome = append(chrome, "")
		chrome = append(chrome, summary...)
	}
	if len(menuLines) > 0 {
		chrome = append(chrome, "")
		chrome = append(chrome, menuLines...)
	}
	if infoLine != "" {
		chrome = append(chrome, "")
		chrome = append(chrome, infoLine)
	}
	chromeH := lipgloss.Height(strings.Join(chrome, "\n"))

	// If we don't know the terminal height, render the legacy layout — no
	// windowing, full chart — to preserve backward compat for tests that
	// construct the model without ever sending WindowSizeMsg.
	if m.height <= 0 {
		parts := []string{header, ""}
		if chart, _ := m.viewChartSized(chartHeightMax); chart != "" {
			parts = append(parts, chart, "")
		}
		if m.instrument.Type == portfolio.InstrumentHolding {
			parts = append(parts, m.viewLotTableWindowed(0)...)
			parts = append(parts, "")
			parts = append(parts, summary...)
			parts = append(parts, "")
		}
		parts = append(parts, menuLines...)
		if infoLine != "" {
			parts = append(parts, "", infoLine)
		}
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	// Allocate rows to chart + lot. If the chrome alone exceeds the
	// terminal (very tight), the chart and lot both collapse and the chrome
	// keeps its priority order.
	chartH, lotH := m.allocateChartAndLot(m.height - chromeH)
	chart := ""
	if chartH > 0 {
		chart, _ = m.viewChartSized(chartH)
	}

	parts := []string{header}
	if len(summary) > 0 {
		parts = append(parts, "")
		parts = append(parts, summary...)
	}
	if chart != "" {
		parts = append(parts, "")
		parts = append(parts, chart)
	}
	if m.instrument.Type == portfolio.InstrumentHolding && lotH > 0 {
		parts = append(parts, "")
		parts = append(parts, m.viewLotTableWindowed(lotH)...)
	}
	if len(menuLines) > 0 {
		parts = append(parts, "")
		parts = append(parts, menuLines...)
	}
	if infoLine != "" {
		parts = append(parts, "")
		parts = append(parts, infoLine)
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// infoLine returns the current info message rendered (or "" when there is
// no message). Encapsulates the error/hint styling switch.
func (m *portfolioInstrumentViewModel) infoLine() string {
	if m.infoMsg == "" {
		return ""
	}
	if m.infoIsErr {
		return m.styles.Error.Render(m.infoMsg)
	}
	return m.styles.Hint.Render(m.infoMsg)
}

// allocateChartAndLot splits the available budget (already net of chrome)
// between the chart and the lot table. The chart tries to claim the
// biggest variant that fits in the budget; whatever's left goes to the
// lot table. Both blocks include one blank line above them in the parts
// list, so the effective budget is reduced by 1 for each block that's
// actually rendered.
//
// The chart-degrade ladder (descending):
//   chartHeightMax (14) → chartHeightMedium (8) → chartHeightMin (5) → hint (1)
// Lots use whatever's left, also deducting one separator line for the
// blank above the block.
func (m *portfolioInstrumentViewModel) allocateChartAndLot(budget int) (chartH, lotH int) {
	if budget <= 1 {
		// Even one separator + one line can't fit both — drop the chart
		// entirely and let the lot table take the single separator.
		return 0, budget - 1
	}
	// Try the biggest chart that fits, accounting for the separator the
	// chart itself will need above it.
	candidates := []int{chartHeightMax, chartHeightMedium, chartHeightMin}
	for _, c := range candidates {
		if c+1 <= budget {
			return c, budget - c - 1
		}
	}
	// Can't fit a real chart variant — degrade to the 1-row "hidden" hint.
	if budget >= 2 {
		return 1, budget - 2
	}
	return 0, 0
}

// viewChart returns the rendered chart at the requested height (or "" when
// nothing should render). The rendered string and the number of terminal
// rows it actually consumes are returned in a pair so View() can subtract
// that from the lot-table budget.
func (m *portfolioInstrumentViewModel) viewChartSized(budget int) (string, int) {
	if budget <= 0 {
		return "", 0
	}
	if m.loadingCandles {
		// Spinner line is always 1 row — give it that if any rows remain.
		return m.styles.Hint.Render(locale.T("portfolio.instrument.chart.loading") + " " + m.spin.View()), 1
	}
	if len(m.candles) == 0 {
		return "", 0
	}
	// Pick the largest variant that fits the budget.
	var h int
	switch {
	case budget >= chartHeightMax:
		h = chartHeightMax
	case budget >= chartHeightMedium:
		h = chartHeightMedium
	case budget >= chartHeightMin:
		h = chartHeightMin
	default:
		// Below chartHeightMin: degrade to the "chart hidden" hint so the
		// user at least sees a placeholder rather than nothing — they know
		// the data is there but the terminal is too short to draw it.
		return m.styles.Hint.Render(locale.T("portfolio.instrument.chart_too_short")), 1
	}
	return renderCandleChart(m.candles, m.styles, chartWidth, h), h
}

// viewLotTableWindowed returns the lot table clipped to fit within maxRows
// terminal rows. When maxRows is 0, the table renders in full (the
// pre-issue-36 behavior, used by tests that don't seed a height).
// Allocation rules:
//   - 1 row is reserved for the lot header.
//   - At most (maxRows - 1) rows of actual lots are rendered.
//   - The windowed helper reserves 2 indicator slots ("↑ more above" / "↓
//     more below"); the helper itself floors visibleCount at 0, so a tight
//     budget gracefully collapses to header + 0 lot rows instead of
//     overflowing.
//   - The cursor is kept inside the visible window so the user can always
//     see what they're walking.
func (m *portfolioInstrumentViewModel) viewLotTableWindowed(maxRows int) []string {
	if len(m.instrument.Lots) == 0 {
		return []string{m.styles.Hint.Render(locale.T("portfolio.instrument.no_lots"))}
	}
	header := m.styles.Hint.Render(locale.T("portfolio.instrument.lot_header"))
	cur := m.portfolio.Currency

	// Clamp cursor into the legal range so a deletion or race doesn't send
	// it out of bounds.
	if m.lotCursor < 0 {
		m.lotCursor = 0
	}
	if m.lotCursor > len(m.instrument.Lots)-1 {
		m.lotCursor = len(m.instrument.Lots) - 1
	}

	if maxRows <= 0 {
		// Full table (no windowing) — preserves the pre-issue-36 look.
		rows := []string{header}
		rows = append(rows, m.renderLotRows(0, len(m.instrument.Lots), m.instrument.Lots, cur)...)
		return rows
	}

	// Lot header takes 1 row; reserve up to 2 for window indicators; the
	// rest are actual lot rows.
	const headerReserve = 1
	const indicatorReserve = 2
	rowsBudget := maxRows - headerReserve - indicatorReserve
	if rowsBudget < 0 {
		rowsBudget = 0
	}

	total := len(m.instrument.Lots)
	opts := WindowedRowOpts{
		Height:           rowsBudget + indicatorReserve, // header is chromeAbove
		ScrollOff:        m.lotScrollOff,
		Cursor:           m.lotCursor,
		Total:            total,
		ChromeAbove:      0,
		ChromeBelow:      0,
		IndicatorReserve: indicatorReserve,
		RenderRow: func(i int) string {
			return m.renderLotRow(i, m.instrument.Lots[i], cur)
		},
		HintRender: func(s string) string { return m.styles.Hint.Render(s) },
	}
	window := windowedRows(opts)
	m.lotScrollOff = clampScrollOff(m.lotCursor, m.lotScrollOff, len(window)-indicatorReserve)

	out := []string{header}
	if len(window) == 0 {
		return out
	}
	out = append(out, window...)
	return out
}

// renderLotRows renders the [from, to) lot range. Used for the
// pre-windowing full-table path; windowedRows calls renderLotRow directly.
func (m *portfolioInstrumentViewModel) renderLotRows(from, to int, lots []portfolio.Lot, cur string) []string {
	out := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		out = append(out, m.renderLotRow(i, lots[i], cur))
	}
	return out
}

// renderLotRow renders one lot row in the per-lot table. Row i gets the
// "cursor" prefix ("> ") when the user has navigated to it via j/k/g/G/
// PageUp/PageDown; this gives them a visual anchor when the table is
// windowed and the lot they care about isn't the topmost rendered row.
func (m *portfolioInstrumentViewModel) renderLotRow(i int, l portfolio.Lot, cur string) string {
	value := locale.T("portfolio.view.price_unavailable")
	pnlText := locale.T("portfolio.view.price_unavailable")
	havePnL := false
	var pnl float64
	if m.price > 0 {
		value = finance.FormatMoney(l.Quantity*m.price, cur)
		pnl = l.Quantity * (m.price - l.Price)
		pnlText = finance.FormatMoneySigned(pnl, cur)
		havePnL = true
	} else if m.loadingPx {
		value = m.spin.View()
		pnlText = "…"
	}
	row := fmt.Sprintf("%-16s %-12.6g %*s %*s",
		l.Date.Format("2006-01-02"),
		l.Quantity,
		lotMoneyWidth, finance.FormatMoney(l.Price, cur),
		lotMoneyWidth, value,
	)
	style := m.styles.Unselected
	prefix := "  "
	if i == m.lotCursor {
		style = m.styles.Selected
		prefix = m.styles.Cursor.Render(">") + " "
	}
	rendered := style.Render(row)

	padded := fmt.Sprintf("%*s", lotMoneyWidth, pnlText)
	pnlStyle := style
	if havePnL {
		pnlStyle = m.styles.Bull
		if pnl < 0 {
			pnlStyle = m.styles.Bear
		}
	}
	return prefix + rendered + " " + pnlStyle.Render(padded)
}

func (m *portfolioInstrumentViewModel) viewSummary() []string {
	qty := m.instrument.TotalQuantity()
	var totalCost float64
	for _, l := range m.instrument.Lots {
		totalCost += l.Quantity * l.Price
	}
	var avgCost float64
	if qty > 0 {
		avgCost = totalCost / qty
	}

	cur := m.portfolio.Currency
	lines := []string{
		fmt.Sprintf("  %-22s %.6g", locale.T("portfolio.instrument.summary.qty"), qty),
		fmt.Sprintf("  %-22s %*s", locale.T("portfolio.instrument.summary.avg_cost"), moneyColWidth, finance.FormatMoney(avgCost, cur)),
		fmt.Sprintf("  %-22s %*s", locale.T("portfolio.instrument.summary.invested"), moneyColWidth, finance.FormatMoney(totalCost, cur)),
	}
	if m.price > 0 {
		cv := qty * m.price
		pnl := cv - totalCost
		pnlStr := colorMoney(finance.FormatMoneySigned(pnl, cur), pnl, m.styles)
		lines = append(lines,
			fmt.Sprintf("  %-22s %*s", locale.T("portfolio.instrument.summary.current"), moneyColWidth, finance.FormatMoney(cv, cur)),
			fmt.Sprintf("  %-22s %s", locale.T("portfolio.instrument.summary.pnl"), pnlStr),
		)
	}
	return lines
}

func (m *portfolioInstrumentViewModel) viewMenu() []string {
	actions := m.visibleActions()
	var rows []string
	for i, key := range actions {
		label := locale.T(key)
		if i == m.cursor {
			rows = append(rows, fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(label)))
		} else {
			rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(label)))
		}
	}
	rows = append(rows, "", m.styles.Hint.Render(locale.T("portfolio.instrument.hint")))
	return rows
}

func (m *portfolioInstrumentViewModel) viewSellQty() []string {
	label := m.styles.Unselected.Render(locale.T("portfolio.instrument.sell.qty.label"))
	input := m.styles.Input.Render(m.sellQtyInput.View())
	hint := m.styles.Hint.Render(fmt.Sprintf("max %.6g · Enter next · Esc cancel", m.instrument.TotalQuantity()))
	rows := []string{label, input, hint}
	if m.sellQtyErr != "" {
		rows = append(rows, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.sellQtyErr)))
	}
	return rows
}

func (m *portfolioInstrumentViewModel) viewSellPrice() []string {
	label := m.styles.Unselected.Render(locale.T("portfolio.instrument.sell.price.label"))
	input := m.styles.Input.Render(m.sellPriceInput.View())
	hint := m.styles.Hint.Render("Enter confirm · Esc back")
	rows := []string{label, input, hint}
	if m.sellPriceErr != "" {
		rows = append(rows, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.sellPriceErr)))
	}
	return rows
}

func (m *portfolioInstrumentViewModel) viewAddQty() []string {
	label := m.styles.Unselected.Render(locale.T("portfolio.instrument.add_lot.qty.label"))
	input := m.styles.Input.Render(m.addQtyInput.View())
	hint := m.styles.Hint.Render("Enter next · Esc cancel")
	rows := []string{label, input, hint}
	if m.addQtyErr != "" {
		rows = append(rows, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.addQtyErr)))
	}
	return rows
}

func (m *portfolioInstrumentViewModel) viewAddPrice() []string {
	label := m.styles.Unselected.Render(locale.T("portfolio.instrument.add_lot.price.label"))
	input := m.styles.Input.Render(m.addPriceInput.View())
	hint := m.styles.Hint.Render("Enter next · Esc back")
	rows := []string{label, input, hint}
	if m.addPriceErr != "" {
		rows = append(rows, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.addPriceErr)))
	}
	return rows
}

func (m *portfolioInstrumentViewModel) viewAddDate() []string {
	label := m.styles.Unselected.Render(locale.T("portfolio.instrument.add_lot.date.label"))
	input := m.styles.Input.Render(m.addDateInput.View())
	hint := m.styles.Hint.Render("Enter confirm · Esc skip (use today)")
	rows := []string{label, input, hint}
	if m.addDateErr != "" {
		rows = append(rows, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.addDateErr)))
	}
	return rows
}
