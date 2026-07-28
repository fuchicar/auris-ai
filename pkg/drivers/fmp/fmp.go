// Package fmp implementa el driver de Financial Modeling Prep (FMP) para la interfaz market.ProviderAPI.
// FMP es una API REST con autenticación por API key; no dispone de WebSockets nativos.
// El streaming se implementa mediante polling periódico configurable.
package fmp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/fuchicar/auris-ai/pkg/market"
)

const (
	defaultBaseURL      = "https://financialmodelingprep.com/stable"
	defaultHTTPTimeout  = 10 * time.Second
	defaultPollInterval = 5 * time.Second
)

// Driver implementa market.ProviderAPI para Financial Modeling Prep.
type Driver struct {
	apiKey       string
	baseURL      string
	httpClient   *http.Client
	pollInterval time.Duration
	connected    bool
	mu           sync.RWMutex // protege únicamente el campo connected
}

// Option permite configurar el Driver en la construcción.
type Option func(*Driver)

// WithBaseURL sobreescribe la URL base de la API (útil en tests).
func WithBaseURL(u string) Option {
	return func(d *Driver) { d.baseURL = u }
}

// WithHTTPClient reemplaza el cliente HTTP por defecto.
func WithHTTPClient(c *http.Client) Option {
	return func(d *Driver) { d.httpClient = c }
}

// WithPollInterval configura el intervalo de polling para SubscribeQuotes.
func WithPollInterval(interval time.Duration) Option {
	return func(d *Driver) { d.pollInterval = interval }
}

// New crea un nuevo Driver con la API key proporcionada.
func New(apiKey string, opts ...Option) *Driver {
	d := &Driver{
		apiKey:       apiKey,
		baseURL:      defaultBaseURL,
		httpClient:   &http.Client{Timeout: defaultHTTPTimeout},
		pollInterval: defaultPollInterval,
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers internos
// ─────────────────────────────────────────────────────────────────────────────

// buildURL construye la URL completa añadiendo la API key a los parámetros.
func (d *Driver) buildURL(path string, params url.Values) string {
	if params == nil {
		params = url.Values{}
	}
	params.Set("apikey", d.apiKey)
	return d.baseURL + path + "?" + params.Encode()
}

// doGet realiza una petición GET, decodifica el JSON en dest y mapea los errores HTTP.
func (d *Driver) doGet(ctx context.Context, path string, params url.Values, dest any) error {
	rawURL := d.buildURL(path, params)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", market.RedactURLError(err, "apikey"))
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// continuar
	case http.StatusUnauthorized, http.StatusForbidden:
		return market.ErrUnauthorized
	case http.StatusNotFound:
		return market.ErrNotFound
	case http.StatusPaymentRequired:
		return market.ErrSubscriptionRequired
	case http.StatusTooManyRequests:
		return market.ErrRateLimit
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("unexpected HTTP %d: %s", resp.StatusCode, body)
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// checkConnected devuelve ErrNotConnected si el driver no está conectado.
func (d *Driver) checkConnected() error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if !d.connected {
		return fmt.Errorf("fmp: %w", market.ErrNotConnected)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// JSON response structs (unexported)
// ─────────────────────────────────────────────────────────────────────────────

type fmpProfile struct {
	Symbol      string  `json:"symbol"`
	CompanyName string  `json:"companyName"`
	Exchange    string  `json:"exchange"`
	Currency    string  `json:"currency"`
	Isin        string  `json:"isin"`
	IsEtf       bool    `json:"isEtf"`
	IsFund      bool    `json:"isFund"`
	Beta        float64 `json:"beta"`
}

type fmpSearchResult struct {
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	Currency      string `json:"currency"`
	StockExchange string `json:"stockExchange"`
}

type fmpListEntry struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Exchange string `json:"exchange"`
	Type     string `json:"type"`
}

type fmpQuote struct {
	Symbol           string  `json:"symbol"`
	Price            float64 `json:"price"`
	Timestamp        int64   `json:"timestamp"`
	ChangePercentage float64 `json:"changePercentage"`
}

type fmpDailyCandle struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type fmpIntradayCandle struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type fmpDividend struct {
	Symbol   string  `json:"symbol"`
	Date     string  `json:"date"`
	Dividend float64 `json:"dividend"`
	Label    string  `json:"label"`
}

type fmpSplit struct {
	Symbol      string  `json:"symbol"`
	Date        string  `json:"date"`
	Numerator   float64 `json:"numerator"`
	Denominator float64 `json:"denominator"`
	Label       string  `json:"label"`
}

type fmpKeyMetricsTTM struct {
	Symbol           string  `json:"symbol"`
	MarketCap        float64 `json:"marketCap"` // FMP no añade sufijo TTM a este campo
	PeRatio          float64 `json:"peRatioTTM"`
	EPS              float64 `json:"netIncomePerShareTTM"`
	DividendYieldTTM float64 `json:"dividendYieldTTM"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers de mapeo de tipos
// ─────────────────────────────────────────────────────────────────────────────

func profileToInstrument(p fmpProfile) market.Instrument {
	assetType := market.AssetTypeStock
	if p.IsEtf || p.IsFund {
		assetType = market.AssetTypeETF
	}
	return market.Instrument{
		Symbol:   p.Symbol,
		ISIN:     p.Isin,
		Name:     p.CompanyName,
		Exchange: p.Exchange,
		Currency: p.Currency,
		Type:     assetType,
	}
}

func listEntryToAssetType(t string) market.AssetType {
	switch t {
	case "etf", "fund":
		return market.AssetTypeETF
	default:
		return market.AssetTypeStock
	}
}

func timeframeToFMPInterval(tf market.Timeframe) (string, bool) {
	m := map[market.Timeframe]string{
		market.Timeframe1m:  "1min",
		market.Timeframe5m:  "5min",
		market.Timeframe15m: "15min",
		market.Timeframe1h:  "1hour",
		market.Timeframe4h:  "4hour",
	}
	v, ok := m[tf]
	return v, ok
}

// reverseCandles invierte un slice de Candle in-place (FMP devuelve newest-first).
func reverseCandles(s []market.Candle) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// market.ProviderAPI — implementación
// ─────────────────────────────────────────────────────────────────────────────

// Name returns the human-readable display name of the FMP provider.
func (d *Driver) Name() string {
	return "Financial Modeling Prep"
}

// DocsURL returns the URL to the FMP API key documentation page.
func (d *Driver) DocsURL() string {
	return "https://site.financialmodelingprep.com/developer/docs"
}

// Description returns a prose description of the FMP provider.
func (d *Driver) Description() string {
	return "Financial Modeling Prep (FMP) market data provider"
}

// Connect valida la API key haciendo una petición real a FMP.
func (d *Driver) Connect(ctx context.Context) error {
	params := url.Values{"symbol": {"AAPL"}}
	var profiles []fmpProfile
	if err := d.doGet(ctx, "/profile", params, &profiles); err != nil {
		return fmt.Errorf("fmp: Connect: %w", err)
	}
	d.mu.Lock()
	d.connected = true
	d.mu.Unlock()
	return nil
}

// Disconnect marca el driver como desconectado sin llamar a la API.
func (d *Driver) Disconnect(_ context.Context) error {
	d.mu.Lock()
	d.connected = false
	d.mu.Unlock()
	return nil
}

// RefreshToken es un no-op porque FMP usa API keys estáticas.
func (d *Driver) RefreshToken(_ context.Context) error {
	return nil
}

// IsConnected devuelve el estado de conexión actual.
func (d *Driver) IsConnected() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.connected
}

// Ping comprueba que la conexión sigue activa.
func (d *Driver) Ping(ctx context.Context) error {
	if err := d.checkConnected(); err != nil {
		return err
	}
	params := url.Values{"symbol": {"AAPL"}}
	var profiles []fmpProfile
	if err := d.doGet(ctx, "/profile", params, &profiles); err != nil {
		return fmt.Errorf("fmp: Ping: %w", err)
	}
	return nil
}

// SearchInstrument busca instrumentos por nombre o ticker.
func (d *Driver) SearchInstrument(ctx context.Context, query string) ([]market.Instrument, error) {
	if err := d.checkConnected(); err != nil {
		return nil, err
	}
	params := url.Values{"query": {query}}
	var results []fmpSearchResult
	if err := d.doGet(ctx, "/search-symbol", params, &results); err != nil {
		return nil, fmt.Errorf("fmp: SearchInstrument: %w", err)
	}
	instruments := make([]market.Instrument, 0, len(results))
	for _, r := range results {
		instruments = append(instruments, market.Instrument{
			Symbol:   r.Symbol,
			Name:     r.Name,
			Exchange: r.StockExchange,
			Currency: r.Currency,
			Type:     market.AssetTypeStock,
		})
	}
	return instruments, nil
}

// GetInstrument devuelve los metadatos de un símbolo concreto.
func (d *Driver) GetInstrument(ctx context.Context, symbol string) (market.Instrument, error) {
	if err := d.checkConnected(); err != nil {
		return market.Instrument{}, err
	}
	params := url.Values{"symbol": {symbol}}
	var profiles []fmpProfile
	if err := d.doGet(ctx, "/profile", params, &profiles); err != nil {
		return market.Instrument{}, fmt.Errorf("fmp: GetInstrument %q: %w", symbol, err)
	}
	if len(profiles) == 0 {
		return market.Instrument{}, fmt.Errorf("fmp: GetInstrument %q: %w", symbol, market.ErrNotFound)
	}
	return profileToInstrument(profiles[0]), nil
}

// ListInstruments devuelve todos los instrumentos disponibles filtrados por tipo.
// Para tipos no presentes en FMP (futures, forex, crypto) devuelve slice vacío sin error.
func (d *Driver) ListInstruments(ctx context.Context, assetType market.AssetType) ([]market.Instrument, error) {
	if err := d.checkConnected(); err != nil {
		return nil, err
	}
	if assetType == market.AssetTypeFuture || assetType == market.AssetTypeForex || assetType == market.AssetTypeCrypto {
		return []market.Instrument{}, nil
	}

	var entries []fmpListEntry
	if err := d.doGet(ctx, "/stock-list", nil, &entries); err != nil {
		return nil, fmt.Errorf("fmp: ListInstruments: %w", err)
	}

	instruments := make([]market.Instrument, 0)
	for _, e := range entries {
		t := listEntryToAssetType(e.Type)
		if t != assetType {
			continue
		}
		instruments = append(instruments, market.Instrument{
			Symbol:   e.Symbol,
			Name:     e.Name,
			Exchange: e.Exchange,
			Type:     t,
		})
	}
	return instruments, nil
}

// GetCandles devuelve velas OHLCV para el símbolo, rango temporal y granularidad indicados.
// Nota: FMP devuelve timestamps en hora del mercado sin zona horaria; se parsean como UTC.
func (d *Driver) GetCandles(ctx context.Context, symbol string, from, to time.Time, tf market.Timeframe) ([]market.Candle, error) {
	if err := d.checkConnected(); err != nil {
		return nil, err
	}

	fromStr := from.Format(time.DateOnly)
	toStr := to.Format(time.DateOnly)

	if tf == market.Timeframe1d {
		return d.getDailyCandles(ctx, symbol, fromStr, toStr)
	}
	return d.getIntradayCandles(ctx, symbol, fromStr, toStr, tf)
}

func (d *Driver) getDailyCandles(ctx context.Context, symbol, from, to string) ([]market.Candle, error) {
	params := url.Values{
		"symbol": {symbol},
		"from":   {from},
		"to":     {to},
	}
	// El endpoint stable devuelve un array plano, no un objeto con campo "historical".
	var raw []fmpDailyCandle
	if err := d.doGet(ctx, "/historical-price-eod/full", params, &raw); err != nil {
		return nil, fmt.Errorf("fmp: GetCandles daily %q: %w", symbol, err)
	}

	candles := make([]market.Candle, 0, len(raw))
	for _, c := range raw {
		t, err := time.Parse(time.DateOnly, c.Date)
		if err != nil {
			continue
		}
		candles = append(candles, market.Candle{
			Time:   t.UTC(),
			Open:   c.Open,
			High:   c.High,
			Low:    c.Low,
			Close:  c.Close,
			Volume: c.Volume,
		})
	}
	reverseCandles(candles)
	return candles, nil
}

func (d *Driver) getIntradayCandles(ctx context.Context, symbol, from, to string, tf market.Timeframe) ([]market.Candle, error) {
	interval, ok := timeframeToFMPInterval(tf)
	if !ok {
		return nil, fmt.Errorf("fmp: GetCandles: unsupported timeframe %q", tf)
	}
	params := url.Values{
		"symbol": {symbol},
		"from":   {from},
		"to":     {to},
	}
	var raw []fmpIntradayCandle
	if err := d.doGet(ctx, "/historical-chart/"+interval, params, &raw); err != nil {
		return nil, fmt.Errorf("fmp: GetCandles %s %q: %w", interval, symbol, err)
	}

	candles := make([]market.Candle, 0, len(raw))
	for _, c := range raw {
		t, err := time.Parse("2006-01-02 15:04:05", c.Date)
		if err != nil {
			continue
		}
		candles = append(candles, market.Candle{
			Time:   t.UTC(),
			Open:   c.Open,
			High:   c.High,
			Low:    c.Low,
			Close:  c.Close,
			Volume: c.Volume,
		})
	}
	reverseCandles(candles)
	return candles, nil
}

// GetTicks no está soportado por FMP.
func (d *Driver) GetTicks(_ context.Context, _ string, _, _ time.Time) ([]market.Tick, error) {
	return nil, fmt.Errorf("fmp: GetTicks: %w", market.ErrNotSupported)
}

// UnsupportedTools implements market.CapabilityReporter: FMP has no
// order-book or tick-level data in its REST API — see FEAT-8 in doc/task_completed.md.
func (d *Driver) UnsupportedTools() []string {
	return []string{market.ToolGetOrderBook, market.ToolGetTicks}
}

// GetCorporateActions devuelve dividendos y splits para el símbolo y rango indicados.
func (d *Driver) GetCorporateActions(ctx context.Context, symbol string, from, to time.Time) ([]market.CorporateAction, error) {
	if err := d.checkConnected(); err != nil {
		return nil, err
	}

	params := url.Values{
		"symbol": {symbol},
		"from":   {from.Format(time.DateOnly)},
		"to":     {to.Format(time.DateOnly)},
	}

	var actions []market.CorporateAction

	// Dividendos — FMP puede ignorar from/to, se filtra client-side.
	var dividends []fmpDividend
	if err := d.doGet(ctx, "/dividends", params, &dividends); err != nil && !errors.Is(err, market.ErrNotFound) {
		return nil, fmt.Errorf("fmp: GetCorporateActions dividends %q: %w", symbol, err)
	}
	for _, div := range dividends {
		t, err := time.Parse(time.DateOnly, div.Date)
		if err != nil {
			continue
		}
		if t.Before(from) || t.After(to) {
			continue
		}
		actions = append(actions, market.CorporateAction{
			Date:        t.UTC(),
			Type:        "dividend",
			Value:       div.Dividend,
			Description: div.Label,
		})
	}

	// Splits — ídem filtrado client-side.
	var splits []fmpSplit
	if err := d.doGet(ctx, "/splits", params, &splits); err != nil && !errors.Is(err, market.ErrNotFound) {
		return nil, fmt.Errorf("fmp: GetCorporateActions splits %q: %w", symbol, err)
	}
	for _, s := range splits {
		t, err := time.Parse(time.DateOnly, s.Date)
		if err != nil {
			continue
		}
		if t.Before(from) || t.After(to) {
			continue
		}
		value := 0.0
		if s.Denominator != 0 {
			value = s.Numerator / s.Denominator
		}
		actions = append(actions, market.CorporateAction{
			Date:        t.UTC(),
			Type:        "split",
			Value:       value,
			Description: s.Label,
		})
	}

	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Date.Before(actions[j].Date)
	})
	return actions, nil
}

// GetQuote devuelve la cotización actual del símbolo.
// FMP no proporciona bid/ask en el endpoint stable/quote; Bid y Ask se devuelven como 0.
func (d *Driver) GetQuote(ctx context.Context, symbol string) (market.Quote, error) {
	if err := d.checkConnected(); err != nil {
		return market.Quote{}, err
	}
	params := url.Values{"symbol": {symbol}}
	var quotes []fmpQuote
	if err := d.doGet(ctx, "/quote", params, &quotes); err != nil {
		return market.Quote{}, fmt.Errorf("fmp: GetQuote %q: %w", symbol, err)
	}
	if len(quotes) == 0 {
		return market.Quote{}, fmt.Errorf("fmp: GetQuote %q: %w", symbol, market.ErrNotFound)
	}
	q := quotes[0]
	return market.Quote{
		Time:          time.Unix(q.Timestamp, 0).UTC(),
		Last:          q.Price,
		ChangePercent: q.ChangePercentage,
	}, nil
}

// GetOrderBook no está soportado por FMP.
func (d *Driver) GetOrderBook(_ context.Context, _ string, _ int) (market.OrderBook, error) {
	return market.OrderBook{}, fmt.Errorf("fmp: GetOrderBook: %w", market.ErrNotSupported)
}

// SubscribeQuotes suscribe al feed de quotes mediante polling periódico.
// El canal se cierra cuando se cancela el contexto.
func (d *Driver) SubscribeQuotes(ctx context.Context, symbol string) (<-chan market.Quote, error) {
	if err := d.checkConnected(); err != nil {
		return nil, err
	}
	ch := make(chan market.Quote, 1)
	go func() {
		defer close(ch)
		ticker := time.NewTicker(d.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				q, err := d.GetQuote(ctx, symbol)
				if err != nil {
					continue
				}
				select {
				case ch <- q:
				default: // descarta si el consumidor no ha leído el valor anterior
				}
			}
		}
	}()
	return ch, nil
}

// SubscribeTrades no está soportado por FMP.
func (d *Driver) SubscribeTrades(_ context.Context, _ string) (<-chan market.Tick, error) {
	return nil, fmt.Errorf("fmp: SubscribeTrades: %w", market.ErrNotSupported)
}

// SubscribeOrderBook no está soportado por FMP.
func (d *Driver) SubscribeOrderBook(_ context.Context, _ string, _ int) (<-chan market.OrderBook, error) {
	return nil, fmt.Errorf("fmp: SubscribeOrderBook: %w", market.ErrNotSupported)
}

// GetFundamentals devuelve métricas fundamentales TTM del símbolo.
// DividendYieldTTM viene de /key-metrics-ttm; Beta de /profile (fallo no-fatal).
func (d *Driver) GetFundamentals(ctx context.Context, symbol string) (market.Fundamental, error) {
	if err := d.checkConnected(); err != nil {
		return market.Fundamental{}, err
	}
	params := url.Values{"symbol": {symbol}}
	var metrics []fmpKeyMetricsTTM
	if err := d.doGet(ctx, "/key-metrics-ttm", params, &metrics); err != nil {
		return market.Fundamental{}, fmt.Errorf("fmp: GetFundamentals %q: %w", symbol, err)
	}
	if len(metrics) == 0 {
		return market.Fundamental{}, fmt.Errorf("fmp: GetFundamentals %q: %w", symbol, market.ErrNotFound)
	}
	m := metrics[0]
	f := market.Fundamental{
		Symbol:           m.Symbol,
		PERatio:          m.PeRatio,
		EPS:              m.EPS,
		MarketCap:        m.MarketCap,
		DividendYieldTTM: m.DividendYieldTTM,
	}
	// Beta is not in key-metrics-ttm; fetch it from /profile. Failure is
	// non-fatal: we return the TTM metrics we already have and leave Beta=0.
	var profiles []fmpProfile
	if err := d.doGet(ctx, "/profile", params, &profiles); err == nil && len(profiles) > 0 {
		f.Beta = profiles[0].Beta
	}
	return f, nil
}
