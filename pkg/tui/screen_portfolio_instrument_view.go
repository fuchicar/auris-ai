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

	"auris/pkg/locale"
	"auris/pkg/market"
	"auris/pkg/portfolio"
)

// PortfolioInstrumentViewResult is emitted when the user exits the instrument view.
type PortfolioInstrumentViewResult struct {
	Action    string              // "back" | "deleted"
	Portfolio *portfolio.Portfolio // updated copy (already saved)
}

// instrumentViewMode tracks which inline form is active.
type instrumentViewMode int

const (
	ivModeMenu     instrumentViewMode = iota // main action menu
	ivModeSellQty                           // entering sell quantity
	ivModeSellPrice                         // entering sell price
	ivModeAddQty                            // entering lot quantity
	ivModeAddPrice                          // entering lot price
	ivModeAddDate                           // entering lot date
	ivModeConfirmDelete                     // confirming instrument removal
	ivModeConfirmType                       // confirming type change
)

var instrumentActions = []string{
	"portfolio.instrument.action.add_lot",
	"portfolio.instrument.action.sell",
	"portfolio.instrument.action.change_type",
	"portfolio.instrument.action.delete",
}

// portfolioInstrumentViewModel shows a single instrument with lot details and inline actions.
type portfolioInstrumentViewModel struct {
	portfolio  *portfolio.Portfolio
	instrument *portfolio.Instrument // pointer into portfolio.Instruments
	mode       instrumentViewMode
	cursor     int
	infoMsg    string
	infoIsErr  bool
	price      float64 // live price if available
	loadingPx  bool

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

	spin   spinner.Model
	mp     market.ProviderAPI
	styles *Styles
}

func newPortfolioInstrumentViewModel(
	p *portfolio.Portfolio,
	ins *portfolio.Instrument,
	mp market.ProviderAPI,
	s *Styles,
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
		sellQtyInput:   newInput("10"),
		sellPriceInput: newInput("150.00"),
		addQtyInput:    newInput("10"),
		addPriceInput:  newInput("150.00"),
		addDateInput:   newInput(time.Now().Format("2006-01-02")),
	}
}

func (m *portfolioInstrumentViewModel) Init() tea.Cmd {
	if m.loadingPx {
		return tea.Batch(m.spin.Tick, m.fetchPriceCmd())
	}
	return nil
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

func (m *portfolioInstrumentViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.loadingPx {
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
	// Watchlist: hide add_lot and sell.
	return instrumentActions[2:] // change_type + delete
}

func (m *portfolioInstrumentViewModel) selectAction(actionKey string) (tea.Model, tea.Cmd) {
	switch actionKey {
	case "portfolio.instrument.action.add_lot":
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
		if err := portfolio.SavePortfolio(m.portfolio); err != nil {
			m.infoMsg = locale.Tp("portfolio.error.save", map[string]any{"Error": err.Error()})
			m.infoIsErr = true
		} else {
			m.infoMsg = locale.Tp("portfolio.instrument.sell.success", map[string]any{
				"PnL": fmt.Sprintf("%+.2f", res.RealizedPnL),
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
	lot := portfolio.NewLot(qty, price, date)
	m.instrument.Lots = append(m.instrument.Lots, lot)
	if err := portfolio.SavePortfolio(m.portfolio); err != nil {
		m.infoMsg = locale.Tp("portfolio.error.save", map[string]any{"Error": err.Error()})
		m.infoIsErr = true
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

func (m *portfolioInstrumentViewModel) View() string {
	var badgeKey string
	switch m.instrument.Type {
	case portfolio.InstrumentHolding:
		badgeKey = "portfolio.instrument.badge.holding"
	default:
		badgeKey = "portfolio.instrument.badge.watchlist"
	}
	badge := m.styles.Checkbox.Render(fmt.Sprintf("[%s]", locale.T(badgeKey)))
	header := fmt.Sprintf("%s %s — %s", badge, m.styles.Selected.Render(m.instrument.Symbol), m.instrument.Name)

	var parts []string
	parts = append(parts, header, "")

	if m.instrument.Type == portfolio.InstrumentHolding {
		parts = append(parts, m.viewLotTable()...)
		parts = append(parts, "")
		parts = append(parts, m.viewSummary()...)
		parts = append(parts, "")
	}

	switch m.mode {
	case ivModeMenu:
		parts = append(parts, m.viewMenu()...)
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

func (m *portfolioInstrumentViewModel) viewLotTable() []string {
	if len(m.instrument.Lots) == 0 {
		return []string{m.styles.Hint.Render(locale.T("portfolio.instrument.no_lots"))}
	}
	header := m.styles.Hint.Render(locale.T("portfolio.instrument.lot_header"))
	var rows []string
	rows = append(rows, header)
	for _, l := range m.instrument.Lots {
		value := locale.T("portfolio.view.price_unavailable")
		pnlStr := locale.T("portfolio.view.price_unavailable")
		if m.price > 0 {
			value = fmt.Sprintf("%.2f", l.Quantity*m.price)
			pnl := l.Quantity * (m.price - l.Price)
			pnlStr = formatPnL(pnl)
		} else if m.loadingPx {
			value = m.spin.View()
			pnlStr = "…"
		}
		row := fmt.Sprintf("%-16s %-12.6g %-12.2f %-12s %s",
			l.Date.Format("2006-01-02"),
			l.Quantity,
			l.Price,
			value,
			pnlStr,
		)
		rows = append(rows, m.styles.Unselected.Render(row))
	}
	return rows
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

	lines := []string{
		fmt.Sprintf("  %-22s %.6g", locale.T("portfolio.instrument.summary.qty"), qty),
		fmt.Sprintf("  %-22s %.2f", locale.T("portfolio.instrument.summary.avg_cost"), avgCost),
		fmt.Sprintf("  %-22s %.2f", locale.T("portfolio.instrument.summary.invested"), totalCost),
	}
	if m.price > 0 {
		cv := qty * m.price
		pnl := cv - totalCost
		lines = append(lines,
			fmt.Sprintf("  %-22s %.2f", locale.T("portfolio.instrument.summary.current"), cv),
			fmt.Sprintf("  %-22s %s", locale.T("portfolio.instrument.summary.pnl"), formatPnL(pnl)),
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
