// Package eodhd implementa el driver de EOD Historical Data (EODHD) para la interfaz
// market.ProviderAPI. EODHD es una API REST con autenticación por API key; el símbolo
// forma parte del path de cada endpoint (a diferencia de FMP, que lo pasa como query
// param). No dispone de WebSockets en el tier gratuito; el streaming se implementa
// mediante polling periódico configurable, igual que el driver de FMP.
package eodhd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fuchicar/auris-ai/pkg/market"
)

const (
	defaultBaseURL      = "https://eodhd.com/api"
	defaultHTTPTimeout  = 10 * time.Second
	defaultPollInterval = 5 * time.Second
)

// Driver implementa market.ProviderAPI para EOD Historical Data.
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

// buildURL construye la URL completa añadiendo la API key y fmt=json a los parámetros.
// EODHD puede devolver CSV por defecto en algunos endpoints sin fmt=json explícito.
func (d *Driver) buildURL(path string, params url.Values) string {
	if params == nil {
		params = url.Values{}
	}
	params.Set("api_token", d.apiKey)
	params.Set("fmt", "json")
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
		return fmt.Errorf("http: %w", market.RedactURLError(err, "api_token"))
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// continuar
	case http.StatusUnauthorized:
		return market.ErrUnauthorized
	case http.StatusForbidden:
		// EODHD usa 403 para restricciones de plan (p.ej. fundamentals/intraday en el
		// tier gratuito), no para credenciales inválidas — eso es 401. A diferencia de
		// FMP, donde 401/403 significan ambos "unauthorized" y 402 es "subscription
		// required".
		return market.ErrSubscriptionRequired
	case http.StatusNotFound:
		return market.ErrNotFound
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
		return fmt.Errorf("eodhd: %w", market.ErrNotConnected)
	}
	return nil
}

// splitSymbol separa un símbolo estilo EODHD ("BKT.MC") en código y bolsa.
// Sin sufijo, EODHD asume la bolsa "US" por defecto (verificado: /real-time/AAPL
// y /real-time/AAPL.US devuelven el mismo resultado).
func splitSymbol(symbol string) (code, exchange string) {
	if i := strings.LastIndex(symbol, "."); i >= 0 {
		return symbol[:i], symbol[i+1:]
	}
	return symbol, "US"
}

// parseSplitRatio parsea el formato "4.000000/1.000000" que devuelve /api/splits.
func parseSplitRatio(s string) (numerator, denominator float64, ok bool) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return num, den, true
}

// toFloatOrNA convierte un valor decodificado en interface{} a float64. Devuelve
// ok=false tanto para nil como para el string "NA" que EODHD usa en /real-time
// cuando el símbolo no existe (HTTP 200, no HTTP 404 — este caso no lo detecta
// el status-code mapping de doGet, hay que comprobarlo explícitamente).
func toFloatOrNA(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

// ─────────────────────────────────────────────────────────────────────────────
// JSON response structs (unexported)
// ─────────────────────────────────────────────────────────────────────────────

type eodhdRealTimeQuote struct {
	Code      string `json:"code"`
	Timestamp any    `json:"timestamp"` // número (unix seconds) o "NA" si el símbolo no existe
	Close     any    `json:"close"`
	ChangeP   any    `json:"change_p"`
}

type eodhdEODCandle struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type eodhdIntradayCandle struct {
	Datetime string  `json:"datetime"`
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	Volume   float64 `json:"volume"`
}

type eodhdDividend struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

type eodhdSplit struct {
	Date  string `json:"date"`
	Split string `json:"split"` // "4.000000/1.000000"
}

type eodhdSearchResult struct {
	Code     string `json:"Code"`
	Exchange string `json:"Exchange"`
	Name     string `json:"Name"`
	Type     string `json:"Type"`
	Currency string `json:"Currency"`
	ISIN     string `json:"ISIN"` // puede venir null; el decoder deja "" sin error
}

type eodhdExchangeSymbol struct {
	Code     string `json:"Code"`
	Name     string `json:"Name"`
	Exchange string `json:"Exchange"`
	Currency string `json:"Currency"`
	Type     string `json:"Type"`
	Isin     string `json:"Isin"` // puede venir null; el decoder deja "" sin error
}

// eodhdFundamentals sigue el esquema documentado por EODHD (secciones Highlights/
// Technicals). No verificado contra una respuesta real: el tier gratuito siempre
// devuelve 403 en este endpoint (confirmado), así que GetFundamentals nunca llega
// a decodificar este struct en la práctica — revisar si se dispone de un token de
// pago.
type eodhdFundamentals struct {
	Highlights struct {
		MarketCapitalization float64 `json:"MarketCapitalization"`
		PERatio              float64 `json:"PERatio"`
		EPS                  float64 `json:"EPS"`
		DividendYield        float64 `json:"DividendYield"`
	} `json:"Highlights"`
	Technicals struct {
		Beta float64 `json:"Beta"`
	} `json:"Technicals"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers de mapeo de tipos
// ─────────────────────────────────────────────────────────────────────────────

func eodhdTypeToAssetType(t string) market.AssetType {
	switch t {
	case "ETF", "FUND", "Mutual Fund":
		return market.AssetTypeETF
	default:
		return market.AssetTypeStock
	}
}

func timeframeToEODHDInterval(tf market.Timeframe) (string, bool) {
	m := map[market.Timeframe]string{
		market.Timeframe1m: "1m",
		market.Timeframe5m: "5m",
		market.Timeframe1h: "1h",
	}
	v, ok := m[tf]
	return v, ok
}

// ─────────────────────────────────────────────────────────────────────────────
// market.ProviderAPI — implementación
// ─────────────────────────────────────────────────────────────────────────────

// Name returns the short human-readable display name of the EODHD provider.
func (d *Driver) Name() string {
	return "EODHD"
}

// DocsURL returns the URL to the EODHD API key documentation page.
func (d *Driver) DocsURL() string {
	return "https://eodhd.com/financial-apis/"
}

// Description returns a prose description of the EODHD provider.
func (d *Driver) Description() string {
	return "EOD Historical Data (EODHD) market data provider"
}

// Connect valida la API key haciendo una petición real a EODHD.
func (d *Driver) Connect(ctx context.Context) error {
	var q eodhdRealTimeQuote
	if err := d.doGet(ctx, "/real-time/AAPL.US", nil, &q); err != nil {
		return fmt.Errorf("eodhd: Connect: %w", err)
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

// RefreshToken es un no-op porque EODHD usa API keys estáticas.
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
	var q eodhdRealTimeQuote
	if err := d.doGet(ctx, "/real-time/AAPL.US", nil, &q); err != nil {
		return fmt.Errorf("eodhd: Ping: %w", err)
	}
	return nil
}

// SearchInstrument busca instrumentos por nombre o ticker en todas las bolsas.
func (d *Driver) SearchInstrument(ctx context.Context, query string) ([]market.Instrument, error) {
	if err := d.checkConnected(); err != nil {
		return nil, err
	}
	var results []eodhdSearchResult
	if err := d.doGet(ctx, "/search/"+url.PathEscape(query), nil, &results); err != nil {
		return nil, fmt.Errorf("eodhd: SearchInstrument: %w", err)
	}
	instruments := make([]market.Instrument, 0, len(results))
	for _, r := range results {
		instruments = append(instruments, market.Instrument{
			Symbol:   r.Code + "." + r.Exchange,
			ISIN:     r.ISIN,
			Name:     r.Name,
			Exchange: r.Exchange,
			Currency: r.Currency,
			Type:     eodhdTypeToAssetType(r.Type),
		})
	}
	return instruments, nil
}

// GetInstrument devuelve los metadatos de un símbolo concreto vía /api/search,
// filtrando por coincidencia exacta de código y bolsa.
func (d *Driver) GetInstrument(ctx context.Context, symbol string) (market.Instrument, error) {
	if err := d.checkConnected(); err != nil {
		return market.Instrument{}, err
	}
	code, exchange := splitSymbol(symbol)
	var results []eodhdSearchResult
	if err := d.doGet(ctx, "/search/"+url.PathEscape(code), nil, &results); err != nil {
		return market.Instrument{}, fmt.Errorf("eodhd: GetInstrument %q: %w", symbol, err)
	}
	for _, r := range results {
		if strings.EqualFold(r.Code, code) && strings.EqualFold(r.Exchange, exchange) {
			return market.Instrument{
				Symbol:   r.Code + "." + r.Exchange,
				ISIN:     r.ISIN,
				Name:     r.Name,
				Exchange: r.Exchange,
				Currency: r.Currency,
				Type:     eodhdTypeToAssetType(r.Type),
			}, nil
		}
	}
	return market.Instrument{}, fmt.Errorf("eodhd: GetInstrument %q: %w", symbol, market.ErrNotFound)
}

// ListInstruments devuelve los instrumentos de la bolsa US filtrados por tipo
// (mismo alcance que el /stock-list de FMP). Para tipos no presentes en EODHD
// (futures, forex, crypto) devuelve slice vacío sin error.
func (d *Driver) ListInstruments(ctx context.Context, assetType market.AssetType) ([]market.Instrument, error) {
	if err := d.checkConnected(); err != nil {
		return nil, err
	}
	if assetType == market.AssetTypeFuture || assetType == market.AssetTypeForex || assetType == market.AssetTypeCrypto {
		return []market.Instrument{}, nil
	}

	var entries []eodhdExchangeSymbol
	if err := d.doGet(ctx, "/exchange-symbol-list/US", nil, &entries); err != nil {
		return nil, fmt.Errorf("eodhd: ListInstruments: %w", err)
	}

	instruments := make([]market.Instrument, 0)
	for _, e := range entries {
		t := eodhdTypeToAssetType(e.Type)
		if t != assetType {
			continue
		}
		instruments = append(instruments, market.Instrument{
			Symbol:   e.Code + "." + e.Exchange,
			ISIN:     e.Isin,
			Name:     e.Name,
			Exchange: e.Exchange,
			Currency: e.Currency,
			Type:     t,
		})
	}
	return instruments, nil
}

// GetCandles devuelve velas OHLCV para el símbolo, rango temporal y granularidad indicados.
func (d *Driver) GetCandles(ctx context.Context, symbol string, from, to time.Time, tf market.Timeframe) ([]market.Candle, error) {
	if err := d.checkConnected(); err != nil {
		return nil, err
	}

	if tf == market.Timeframe1d {
		return d.getDailyCandles(ctx, symbol, from.Format(time.DateOnly), to.Format(time.DateOnly))
	}
	return d.getIntradayCandles(ctx, symbol, from, to, tf)
}

func (d *Driver) getDailyCandles(ctx context.Context, symbol, from, to string) ([]market.Candle, error) {
	params := url.Values{
		"period": {"d"},
		"order":  {"a"}, // ascendente — verificado que es también el orden por defecto
		"from":   {from},
		"to":     {to},
	}
	var raw []eodhdEODCandle
	if err := d.doGet(ctx, "/eod/"+url.PathEscape(symbol), params, &raw); err != nil {
		return nil, fmt.Errorf("eodhd: GetCandles daily %q: %w", symbol, err)
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
	return candles, nil
}

// getIntradayCandles siempre devuelve ErrSubscriptionRequired en el tier gratuito
// (HTTP 403 en /api/intraday, verificado) — comportamiento esperado, sin lógica
// especial: el mapeo genérico de doGet ya lo cubre.
// A diferencia de /api/eod, este endpoint exige from/to como timestamps unix
// (no fechas ISO) — pasar una fecha ISO devuelve HTTP 422, verificado en vivo.
func (d *Driver) getIntradayCandles(ctx context.Context, symbol string, from, to time.Time, tf market.Timeframe) ([]market.Candle, error) {
	interval, ok := timeframeToEODHDInterval(tf)
	if !ok {
		return nil, fmt.Errorf("eodhd: GetCandles: unsupported timeframe %q", tf)
	}
	params := url.Values{
		"interval": {interval},
		"from":     {strconv.FormatInt(from.Unix(), 10)},
		"to":       {strconv.FormatInt(to.Unix(), 10)},
	}
	var raw []eodhdIntradayCandle
	if err := d.doGet(ctx, "/intraday/"+url.PathEscape(symbol), params, &raw); err != nil {
		return nil, fmt.Errorf("eodhd: GetCandles %s %q: %w", interval, symbol, err)
	}

	candles := make([]market.Candle, 0, len(raw))
	for _, c := range raw {
		t, err := time.Parse("2006-01-02 15:04:05", c.Datetime)
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
	return candles, nil
}

// GetTicks no está soportado por EODHD (sin datos tick-a-tick en su REST API estándar).
func (d *Driver) GetTicks(_ context.Context, _ string, _, _ time.Time) ([]market.Tick, error) {
	return nil, fmt.Errorf("eodhd: GetTicks: %w", market.ErrNotSupported)
}

// UnsupportedTools implements market.CapabilityReporter: EODHD has no
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
		"from": {from.Format(time.DateOnly)},
		"to":   {to.Format(time.DateOnly)},
	}

	var actions []market.CorporateAction

	var dividends []eodhdDividend
	if err := d.doGet(ctx, "/div/"+url.PathEscape(symbol), params, &dividends); err != nil && !errors.Is(err, market.ErrNotFound) {
		return nil, fmt.Errorf("eodhd: GetCorporateActions dividends %q: %w", symbol, err)
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
			Date:  t.UTC(),
			Type:  "dividend",
			Value: div.Value,
		})
	}

	var splits []eodhdSplit
	if err := d.doGet(ctx, "/splits/"+url.PathEscape(symbol), params, &splits); err != nil && !errors.Is(err, market.ErrNotFound) {
		return nil, fmt.Errorf("eodhd: GetCorporateActions splits %q: %w", symbol, err)
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
		if num, den, ok := parseSplitRatio(s.Split); ok && den != 0 {
			value = num / den
		}
		actions = append(actions, market.CorporateAction{
			Date:        t.UTC(),
			Type:        "split",
			Value:       value,
			Description: s.Split,
		})
	}

	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Date.Before(actions[j].Date)
	})
	return actions, nil
}

// GetQuote devuelve la cotización actual del símbolo.
// EODHD no proporciona bid/ask en /api/real-time; Bid y Ask se devuelven como 0.
// Un símbolo inexistente responde HTTP 200 con los campos numéricos como el
// string "NA" en vez de HTTP 404 — se detecta explícitamente y se traduce a
// ErrNotFound.
func (d *Driver) GetQuote(ctx context.Context, symbol string) (market.Quote, error) {
	if err := d.checkConnected(); err != nil {
		return market.Quote{}, err
	}
	var q eodhdRealTimeQuote
	if err := d.doGet(ctx, "/real-time/"+url.PathEscape(symbol), nil, &q); err != nil {
		return market.Quote{}, fmt.Errorf("eodhd: GetQuote %q: %w", symbol, err)
	}
	closeVal, ok := toFloatOrNA(q.Close)
	if !ok {
		return market.Quote{}, fmt.Errorf("eodhd: GetQuote %q: %w", symbol, market.ErrNotFound)
	}
	ts, _ := toFloatOrNA(q.Timestamp)
	changeP, _ := toFloatOrNA(q.ChangeP)
	return market.Quote{
		Time:          time.Unix(int64(ts), 0).UTC(),
		Last:          closeVal,
		ChangePercent: changeP,
	}, nil
}

// GetOrderBook no está soportado por EODHD.
func (d *Driver) GetOrderBook(_ context.Context, _ string, _ int) (market.OrderBook, error) {
	return market.OrderBook{}, fmt.Errorf("eodhd: GetOrderBook: %w", market.ErrNotSupported)
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

// SubscribeTrades no está soportado por EODHD.
func (d *Driver) SubscribeTrades(_ context.Context, _ string) (<-chan market.Tick, error) {
	return nil, fmt.Errorf("eodhd: SubscribeTrades: %w", market.ErrNotSupported)
}

// SubscribeOrderBook no está soportado por EODHD.
func (d *Driver) SubscribeOrderBook(_ context.Context, _ string, _ int) (<-chan market.OrderBook, error) {
	return nil, fmt.Errorf("eodhd: SubscribeOrderBook: %w", market.ErrNotSupported)
}

// GetFundamentals devuelve métricas fundamentales del símbolo.
// En el tier gratuito siempre devuelve ErrSubscriptionRequired (HTTP 403,
// verificado) — comportamiento esperado, no un fallo del driver.
func (d *Driver) GetFundamentals(ctx context.Context, symbol string) (market.Fundamental, error) {
	if err := d.checkConnected(); err != nil {
		return market.Fundamental{}, err
	}
	var raw eodhdFundamentals
	if err := d.doGet(ctx, "/fundamentals/"+url.PathEscape(symbol), nil, &raw); err != nil {
		return market.Fundamental{}, fmt.Errorf("eodhd: GetFundamentals %q: %w", symbol, err)
	}
	return market.Fundamental{
		Symbol:           symbol,
		PERatio:          raw.Highlights.PERatio,
		EPS:              raw.Highlights.EPS,
		MarketCap:        raw.Highlights.MarketCapitalization,
		DividendYieldTTM: raw.Highlights.DividendYield,
		Beta:             raw.Technicals.Beta,
	}, nil
}
