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

// InstrumentSearchResult is emitted when the user has selected an instrument.
type InstrumentSearchResult struct {
	Symbol string
	Name   string
	Type   portfolio.InstrumentType
	Lot    *portfolio.Lot // nil if skipped or watchlist
}

// instrumentSearchStep tracks the current sub-step.
type instrumentSearchStep int

const (
	searchStepInput    instrumentSearchStep = iota // text input with debounce
	searchStepResults                              // list of search results
	searchStepType                                 // holding or watchlist selection
	searchStepLotQty                               // optional lot quantity
	searchStepLotPrice                             // optional lot price
	searchStepLotDate                              // optional lot date
	// manual entry (no market provider)
	searchStepManualSymbol
	searchStepManualName
)

// searchTickMsg drives debounce — only triggers search when version matches.
type searchTickMsg struct{ version int }

// searchResultMsg carries results from the market API.
type searchResultMsg struct {
	version int
	results []market.Instrument
	err     error
}

// instrumentSearchModel drives the multi-step instrument add flow.
type instrumentSearchModel struct {
	mp            market.ProviderAPI
	step          instrumentSearchStep
	searchVersion int
	query         string
	results       []market.Instrument
	resultCursor  int
	searching     bool
	searchErr     string

	// selected instrument
	selectedSymbol string
	selectedName   string
	selectedType   portfolio.InstrumentType
	typeCursor     int // 0=holding, 1=watchlist

	// lot inputs
	lotQtyInput   textinput.Model
	lotPriceInput textinput.Model
	lotDateInput  textinput.Model
	lotQtyErr     string
	lotPriceErr   string
	lotDateErr    string

	// manual inputs
	manualSymbolInput textinput.Model
	manualNameInput   textinput.Model

	queryInput textinput.Model
	spin       spinner.Model
	styles     *Styles
}

func newInstrumentSearchModel(mp market.ProviderAPI, s *Styles) *instrumentSearchModel {
	queryInput := textinput.New()
	queryInput.Placeholder = locale.T("portfolio.search.hint")
	queryInput.CharLimit = 80

	lotQty := textinput.New()
	lotQty.Placeholder = "10"
	lotQty.CharLimit = 20

	lotPrice := textinput.New()
	lotPrice.Placeholder = "150.00"
	lotPrice.CharLimit = 20

	lotDate := textinput.New()
	lotDate.Placeholder = time.Now().Format("2006-01-02")
	lotDate.CharLimit = 10

	manualSym := textinput.New()
	manualSym.Placeholder = "AAPL"
	manualSym.CharLimit = 20

	manualName := textinput.New()
	manualName.Placeholder = "Apple Inc."
	manualName.CharLimit = 100

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = s.Spinner

	m := &instrumentSearchModel{
		mp:                mp,
		lotQtyInput:       lotQty,
		lotPriceInput:     lotPrice,
		lotDateInput:      lotDate,
		manualSymbolInput: manualSym,
		manualNameInput:   manualName,
		queryInput:        queryInput,
		spin:              sp,
		styles:            s,
	}

	if mp == nil {
		m.step = searchStepManualSymbol
		manualSym.Focus()
		m.manualSymbolInput = manualSym
	} else {
		queryInput.Focus()
		m.queryInput = queryInput
	}
	return m
}

func (m *instrumentSearchModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *instrumentSearchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.searching {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case searchTickMsg:
		if msg.version != m.searchVersion || !m.searching {
			return m, nil
		}
		query := m.query
		version := m.searchVersion
		mp := m.mp
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if !mp.IsConnected() {
				_ = mp.Connect(ctx)
			}
			results, err := mp.SearchInstrument(ctx, query)
			return searchResultMsg{version: version, results: results, err: err}
		}

	case searchResultMsg:
		if msg.version != m.searchVersion {
			return m, nil // stale result
		}
		m.searching = false
		if msg.err != nil {
			m.searchErr = locale.Tp("portfolio.search.error", map[string]any{"Error": msg.err.Error()})
		} else {
			m.searchErr = ""
			m.results = msg.results
			m.resultCursor = 0
		}
		m.step = searchStepResults
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *instrumentSearchModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case searchStepInput:
		return m.handleSearchInput(key)
	case searchStepResults:
		return m.handleResults(key)
	case searchStepType:
		return m.handleType(key)
	case searchStepLotQty:
		return m.handleLotQty(key)
	case searchStepLotPrice:
		return m.handleLotPrice(key)
	case searchStepLotDate:
		return m.handleLotDate(key)
	case searchStepManualSymbol:
		return m.handleManualSymbol(key)
	case searchStepManualName:
		return m.handleManualName(key)
	}
	return m, nil
}

func (m *instrumentSearchModel) handleSearchInput(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenInstrumentSearch, Result: nil}
		}
	case tea.KeyEnter:
		// Trigger search immediately on Enter.
		q := strings.TrimSpace(m.queryInput.Value())
		if q == "" {
			return m, nil
		}
		return m.triggerSearch(q)
	default:
		var cmd tea.Cmd
		m.queryInput, cmd = m.queryInput.Update(key)
		newQuery := strings.TrimSpace(m.queryInput.Value())
		if newQuery != m.query {
			m.query = newQuery
			m.searchVersion++
			version := m.searchVersion
			m.searching = true
			debounceCmd := func() tea.Msg {
				time.Sleep(300 * time.Millisecond)
				return searchTickMsg{version: version}
			}
			return m, tea.Batch(cmd, debounceCmd, m.spin.Tick)
		}
		return m, cmd
	}
}

func (m *instrumentSearchModel) triggerSearch(query string) (tea.Model, tea.Cmd) {
	m.query = query
	m.searchVersion++
	version := m.searchVersion
	m.searching = true
	mp := m.mp
	return m, tea.Batch(m.spin.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if !mp.IsConnected() {
			_ = mp.Connect(ctx)
		}
		results, err := mp.SearchInstrument(ctx, query)
		return searchResultMsg{version: version, results: results, err: err}
	})
}

func (m *instrumentSearchModel) handleResults(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyUp:
		if m.resultCursor > 0 {
			m.resultCursor--
		}
	case tea.KeyDown:
		if m.resultCursor < len(m.results)-1 {
			m.resultCursor++
		}
	case tea.KeyEsc:
		m.step = searchStepInput
		m.queryInput.Focus()
		return m, textinput.Blink
	case tea.KeyEnter:
		if len(m.results) == 0 {
			return m, nil
		}
		ins := m.results[m.resultCursor]
		m.selectedSymbol = ins.Symbol
		m.selectedName = ins.Name
		m.step = searchStepType
	}
	return m, nil
}

func (m *instrumentSearchModel) handleType(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyUp:
		if m.typeCursor > 0 {
			m.typeCursor--
		}
	case tea.KeyDown:
		if m.typeCursor < 1 {
			m.typeCursor++
		}
	case tea.KeyEsc:
		if m.mp != nil {
			m.step = searchStepResults
		} else {
			m.step = searchStepManualName
			m.manualNameInput.Focus()
			return m, textinput.Blink
		}
	case tea.KeyEnter:
		if m.typeCursor == 0 {
			m.selectedType = portfolio.InstrumentHolding
			m.step = searchStepLotQty
			m.lotQtyInput.Focus()
			return m, textinput.Blink
		}
		// Watchlist: emit result directly.
		m.selectedType = portfolio.InstrumentWatchlist
		return m.emitResult(nil)
	}
	return m, nil
}

func (m *instrumentSearchModel) handleLotQty(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		// Skip lot entirely.
		return m.emitResult(nil)
	case tea.KeyEnter:
		val := strings.TrimSpace(m.lotQtyInput.Value())
		if val == "" {
			return m.emitResult(nil) // skip
		}
		qty, err := strconv.ParseFloat(val, 64)
		if err != nil || qty <= 0 {
			m.lotQtyErr = locale.T("portfolio.search.step.qty.error")
			return m, nil
		}
		m.lotQtyErr = ""
		m.step = searchStepLotPrice
		m.lotQtyInput.Blur()
		m.lotPriceInput.Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.lotQtyInput, cmd = m.lotQtyInput.Update(key)
	return m, cmd
}

func (m *instrumentSearchModel) handleLotPrice(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		m.step = searchStepLotQty
		m.lotPriceInput.Blur()
		m.lotQtyInput.Focus()
		return m, textinput.Blink
	case tea.KeyEnter:
		val := strings.TrimSpace(m.lotPriceInput.Value())
		if val == "" {
			return m.emitResult(nil) // skip lot
		}
		price, err := strconv.ParseFloat(val, 64)
		if err != nil || price <= 0 {
			m.lotPriceErr = locale.T("portfolio.search.step.qty.error")
			return m, nil
		}
		m.lotPriceErr = ""
		m.step = searchStepLotDate
		m.lotPriceInput.Blur()
		m.lotDateInput.SetValue(time.Now().Format("2006-01-02"))
		m.lotDateInput.Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.lotPriceInput, cmd = m.lotPriceInput.Update(key)
	return m, cmd
}

func (m *instrumentSearchModel) handleLotDate(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		// Build lot with quantity + price, skip date (use today).
		return m.buildAndEmitLot(time.Now())
	case tea.KeyEnter:
		val := strings.TrimSpace(m.lotDateInput.Value())
		if val == "" {
			return m.buildAndEmitLot(time.Now())
		}
		t, err := time.Parse("2006-01-02", val)
		if err != nil {
			m.lotDateErr = locale.T("portfolio.search.step.lot.date.error")
			return m, nil
		}
		return m.buildAndEmitLot(t)
	}
	var cmd tea.Cmd
	m.lotDateInput, cmd = m.lotDateInput.Update(key)
	return m, cmd
}

func (m *instrumentSearchModel) buildAndEmitLot(date time.Time) (tea.Model, tea.Cmd) {
	qty, _ := strconv.ParseFloat(strings.TrimSpace(m.lotQtyInput.Value()), 64)
	price, _ := strconv.ParseFloat(strings.TrimSpace(m.lotPriceInput.Value()), 64)
	lot := portfolio.NewLot(qty, price, date)
	return m.emitResult(&lot)
}

func (m *instrumentSearchModel) emitResult(lot *portfolio.Lot) (tea.Model, tea.Cmd) {
	sym := m.selectedSymbol
	name := m.selectedName
	itype := m.selectedType
	return m, func() tea.Msg {
		return ScreenDoneMsg{
			From: ScreenInstrumentSearch,
			Result: InstrumentSearchResult{
				Symbol: sym,
				Name:   name,
				Type:   itype,
				Lot:    lot,
			},
		}
	}
}

// Manual entry handlers (no market provider).
func (m *instrumentSearchModel) handleManualSymbol(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg {
			return ScreenDoneMsg{From: ScreenInstrumentSearch, Result: nil}
		}
	case tea.KeyEnter:
		sym := strings.TrimSpace(strings.ToUpper(m.manualSymbolInput.Value()))
		if sym == "" {
			return m, nil
		}
		m.selectedSymbol = sym
		m.step = searchStepManualName
		m.manualSymbolInput.Blur()
		m.manualNameInput.Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.manualSymbolInput, cmd = m.manualSymbolInput.Update(key)
	return m, cmd
}

func (m *instrumentSearchModel) handleManualName(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		m.step = searchStepManualSymbol
		m.manualNameInput.Blur()
		m.manualSymbolInput.Focus()
		return m, textinput.Blink
	case tea.KeyEnter:
		m.selectedName = strings.TrimSpace(m.manualNameInput.Value())
		if m.selectedName == "" {
			m.selectedName = m.selectedSymbol
		}
		m.step = searchStepType
	}
	var cmd tea.Cmd
	m.manualNameInput, cmd = m.manualNameInput.Update(key)
	return m, cmd
}

func (m *instrumentSearchModel) View() string {
	title := m.styles.Title.Render(locale.T("portfolio.search.title"))

	var body []string
	switch m.step {
	case searchStepInput:
		body = m.viewSearchInput()
	case searchStepResults:
		body = m.viewResults()
	case searchStepType:
		body = m.viewTypeSelect()
	case searchStepLotQty:
		body = m.viewLotQty()
	case searchStepLotPrice:
		body = m.viewLotPrice()
	case searchStepLotDate:
		body = m.viewLotDate()
	case searchStepManualSymbol:
		body = m.viewManualSymbol()
	case searchStepManualName:
		body = m.viewManualName()
	}

	parts := []string{title, ""}
	parts = append(parts, body...)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *instrumentSearchModel) viewSearchInput() []string {
	input := m.styles.Input.Render(m.queryInput.View())
	hint := m.styles.Hint.Render(locale.T("portfolio.search.hint"))
	rows := []string{input}
	if m.searching {
		rows = append(rows, fmt.Sprintf("%s %s", m.spin.View(), locale.T("portfolio.search.searching")))
	}
	rows = append(rows, hint)
	return rows
}

func (m *instrumentSearchModel) viewResults() []string {
	if m.searchErr != "" {
		return []string{
			m.styles.Error.Render(m.searchErr),
			m.styles.Hint.Render(locale.T("portfolio.search.hint")),
		}
	}
	if len(m.results) == 0 {
		return []string{
			m.styles.Hint.Render(locale.T("portfolio.search.empty")),
			m.styles.Hint.Render(locale.T("portfolio.search.hint")),
		}
	}
	var rows []string
	for i, ins := range m.results {
		label := fmt.Sprintf("%-8s %s", ins.Symbol, ins.Name)
		if i == m.resultCursor {
			rows = append(rows, fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(label)))
		} else {
			rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(label)))
		}
	}
	rows = append(rows, "", m.styles.Hint.Render(locale.T("portfolio.search.hint")))
	return rows
}

func (m *instrumentSearchModel) viewTypeSelect() []string {
	label := m.styles.Subtitle.Render(fmt.Sprintf("%s %s", m.selectedSymbol, m.selectedName))
	question := m.styles.Unselected.Render(locale.T("portfolio.search.step.type.label"))
	options := []string{
		locale.T("portfolio.search.step.type.holding"),
		locale.T("portfolio.search.step.type.watchlist"),
	}
	var rows []string
	for i, opt := range options {
		if i == m.typeCursor {
			rows = append(rows, fmt.Sprintf("%s %s", m.styles.Cursor.Render(">"), m.styles.Selected.Render(opt)))
		} else {
			rows = append(rows, fmt.Sprintf("  %s", m.styles.Unselected.Render(opt)))
		}
	}
	hint := m.styles.Hint.Render("↑↓ navigate · Enter select · Esc back")
	return []string{label, question, "", rows[0], rows[1], "", hint}
}

func (m *instrumentSearchModel) viewLotQty() []string {
	title := m.styles.Subtitle.Render(locale.T("portfolio.search.step.lot.title"))
	label := m.styles.Unselected.Render(locale.T("portfolio.search.step.lot.qty.label"))
	input := m.styles.Input.Render(m.lotQtyInput.View())
	hint := m.styles.Hint.Render(locale.T("portfolio.search.step.lot.qty.hint"))
	skip := m.styles.Hint.Render(locale.T("portfolio.search.step.lot.skip_hint"))
	rows := []string{title, label, input, hint, skip}
	if m.lotQtyErr != "" {
		rows = append(rows, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.lotQtyErr)))
	}
	return rows
}

func (m *instrumentSearchModel) viewLotPrice() []string {
	label := m.styles.Unselected.Render(locale.T("portfolio.search.step.lot.price.label"))
	input := m.styles.Input.Render(m.lotPriceInput.View())
	hint := m.styles.Hint.Render(locale.T("portfolio.search.step.lot.price.hint"))
	skip := m.styles.Hint.Render(locale.T("portfolio.search.step.lot.skip_hint"))
	rows := []string{label, input, hint, skip}
	if m.lotPriceErr != "" {
		rows = append(rows, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.lotPriceErr)))
	}
	return rows
}

func (m *instrumentSearchModel) viewLotDate() []string {
	label := m.styles.Unselected.Render(locale.T("portfolio.search.step.lot.date.label"))
	input := m.styles.Input.Render(m.lotDateInput.View())
	hint := m.styles.Hint.Render(locale.T("portfolio.search.step.lot.date.hint"))
	skip := m.styles.Hint.Render(locale.T("portfolio.search.step.lot.skip_hint"))
	rows := []string{label, input, hint, skip}
	if m.lotDateErr != "" {
		rows = append(rows, m.styles.Error.Render(fmt.Sprintf("✗ %s", m.lotDateErr)))
	}
	return rows
}

func (m *instrumentSearchModel) viewManualSymbol() []string {
	hint := m.styles.Hint.Render(locale.T("portfolio.search.manual_hint"))
	label := m.styles.Unselected.Render(locale.T("portfolio.search.manual.symbol.label"))
	input := m.styles.Input.Render(m.manualSymbolInput.View())
	navHint := m.styles.Hint.Render("Enter next · Esc cancel")
	return []string{hint, "", label, input, navHint}
}

func (m *instrumentSearchModel) viewManualName() []string {
	label := m.styles.Unselected.Render(locale.T("portfolio.search.manual.name.label"))
	input := m.styles.Input.Render(m.manualNameInput.View())
	navHint := m.styles.Hint.Render("Enter next · Esc back")
	return []string{label, input, navHint}
}
