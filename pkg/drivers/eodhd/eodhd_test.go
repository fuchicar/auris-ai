package eodhd_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"auris/pkg/drivers/eodhd"
	"auris/pkg/market"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers de setup (suite hermética — sin red real, sin test_data/eodhd_api_key)
// ─────────────────────────────────────────────────────────────────────────────

type mockRoutes map[string]http.HandlerFunc

// newMockServer levanta un httptest.Server que enruta por path exacto.
func newMockServer(t *testing.T, routes mockRoutes) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, h := range routes {
		mux.HandleFunc(path, h)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func jsonHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func statusHandler(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
	}
}

// quoteAAPLFixture es la respuesta que Connect/Ping/SubscribeQuotes esperan en
// /real-time/AAPL.US — casi todos los tests "conectados" la necesitan porque
// Driver.Connect siempre golpea ese endpoint fijo para validar la key.
const quoteAAPLFixture = `{"code":"AAPL.US","timestamp":1700000000,"close":150.25,"change_p":1.23}`

// connectedDriver crea un Driver apuntando a srv y lo conecta; falla el test si Connect falla.
func connectedDriver(t *testing.T, srv *httptest.Server) *eodhd.Driver {
	t.Helper()
	d := eodhd.New("test-key", eodhd.WithBaseURL(srv.URL))
	if err := d.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Disconnect(context.Background()) })
	return d
}

func bg() context.Context { return context.Background() }

// ─────────────────────────────────────────────────────────────────────────────
// Tests sin red (compilan y corren siempre, sin servidor mock)
// ─────────────────────────────────────────────────────────────────────────────

// TestInterfaceCompliance verifica en tiempo de compilación que *Driver implementa ProviderAPI.
func TestInterfaceCompliance(t *testing.T) {
	var _ market.ProviderAPI = (*eodhd.Driver)(nil)
	var _ market.CapabilityReporter = (*eodhd.Driver)(nil)
}

func TestDescription_NonEmpty(t *testing.T) {
	d := eodhd.New("dummy")
	if d.Description() == "" {
		t.Error("Description() returned empty string")
	}
}

func TestUnsupportedTools(t *testing.T) {
	d := eodhd.New("dummy")
	got := d.UnsupportedTools()
	want := []string{market.ToolGetOrderBook, market.ToolGetTicks}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected %v, got %v", want, got)
			break
		}
	}
}

func TestName_NonEmpty(t *testing.T) {
	d := eodhd.New("dummy")
	if d.Name() == "" {
		t.Error("Name() returned empty string")
	}
}

// GetTicks/GetOrderBook/SubscribeTrades/SubscribeOrderBook devuelven
// ErrNotSupported sin llamar a checkConnected ni hacer ninguna petición HTTP.
func TestGetTicks_ErrNotSupported(t *testing.T) {
	d := eodhd.New("dummy")
	_, err := d.GetTicks(bg(), "AAPL.US", time.Now().AddDate(0, 0, -1), time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, market.ErrNotSupported) {
		t.Errorf("expected ErrNotSupported, got: %v", err)
	}
}

func TestGetOrderBook_ErrNotSupported(t *testing.T) {
	d := eodhd.New("dummy")
	_, err := d.GetOrderBook(bg(), "AAPL.US", 10)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, market.ErrNotSupported) {
		t.Errorf("expected ErrNotSupported, got: %v", err)
	}
}

func TestSubscribeTrades_ErrNotSupported(t *testing.T) {
	d := eodhd.New("dummy")
	_, err := d.SubscribeTrades(bg(), "AAPL.US")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, market.ErrNotSupported) {
		t.Errorf("expected ErrNotSupported, got: %v", err)
	}
}

func TestSubscribeOrderBook_ErrNotSupported(t *testing.T) {
	d := eodhd.New("dummy")
	_, err := d.SubscribeOrderBook(bg(), "AAPL.US", 10)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, market.ErrNotSupported) {
		t.Errorf("expected ErrNotSupported, got: %v", err)
	}
}

func TestErrNotSupported_Wrapping(t *testing.T) {
	d := eodhd.New("dummy")
	now := time.Now()

	cases := []struct {
		name string
		fn   func() error
	}{
		{"GetTicks", func() error {
			_, err := d.GetTicks(bg(), "AAPL.US", now.AddDate(0, 0, -1), now)
			return err
		}},
		{"GetOrderBook", func() error {
			_, err := d.GetOrderBook(bg(), "AAPL.US", 10)
			return err
		}},
		{"SubscribeTrades", func() error {
			_, err := d.SubscribeTrades(bg(), "AAPL.US")
			return err
		}},
		{"SubscribeOrderBook", func() error {
			_, err := d.SubscribeOrderBook(bg(), "AAPL.US", 10)
			return err
		}},
	}

	for _, tc := range cases {
		err := tc.fn()
		if err == nil {
			t.Errorf("%s: expected error, got nil", tc.name)
			continue
		}
		if !errors.Is(err, market.ErrNotSupported) {
			t.Errorf("%s: expected ErrNotSupported, got: %v", tc.name, err)
		}
	}
}

// checkConnected falla antes de tocar la red, así que estos tampoco necesitan servidor mock.
func TestErrNotConnected_Wrapping(t *testing.T) {
	d := eodhd.New("dummy") // nunca llamamos a Connect
	_, err := d.GetQuote(bg(), "AAPL.US")
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if !errors.Is(err, market.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestPing_NotConnected(t *testing.T) {
	d := eodhd.New("dummy") // nunca llamamos a Connect
	err := d.Ping(bg())
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if !errors.Is(err, market.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Auth & lifecycle
// ─────────────────────────────────────────────────────────────────────────────

func TestConnect_ValidKey(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
	})
	d := connectedDriver(t, srv)
	if !d.IsConnected() {
		t.Error("IsConnected() should be true after Connect")
	}
}

func TestConnect_InvalidKey(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": statusHandler(http.StatusUnauthorized),
	})
	d := eodhd.New("invalid_key_xxxx", eodhd.WithBaseURL(srv.URL))
	err := d.Connect(bg())
	if err == nil {
		t.Fatal("expected error with invalid key, got nil")
	}
	if !errors.Is(err, market.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
	if d.IsConnected() {
		t.Error("IsConnected() should be false after failed Connect")
	}
}

func TestDisconnect(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
	})
	d := connectedDriver(t, srv)
	if err := d.Disconnect(bg()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if d.IsConnected() {
		t.Error("IsConnected() should be false after Disconnect")
	}
}

func TestRefreshToken_NoOp(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
	})
	d := connectedDriver(t, srv)
	if err := d.RefreshToken(bg()); err != nil {
		t.Errorf("RefreshToken should be no-op, got: %v", err)
	}
}

func TestPing_Connected(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
	})
	d := connectedDriver(t, srv)
	if err := d.Ping(bg()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Descubrimiento de instrumentos
// ─────────────────────────────────────────────────────────────────────────────

func TestSearchInstrument_US(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
		"/search/AAPL": jsonHandler(`[
			{"Code":"AAPL","Exchange":"US","Name":"Apple Inc","Type":"Common Stock","Currency":"USD","ISIN":"US0378331005"}
		]`),
	})
	d := connectedDriver(t, srv)
	results, err := d.SearchInstrument(bg(), "AAPL")
	if err != nil {
		t.Fatalf("SearchInstrument: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'AAPL'")
	}
	for _, r := range results {
		if r.Symbol == "" {
			t.Error("result with empty Symbol")
		}
		if r.Name == "" {
			t.Error("result with empty Name")
		}
	}
}

// TestSearchInstrument_BME es el caso de uso central de este driver: cobertura
// de la bolsa de Madrid, que FMP no ofrece en el free tier.
func TestSearchInstrument_BME(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
		"/search/BKT": jsonHandler(`[
			{"Code":"BKT","Exchange":"MC","Name":"Bankinter SA","Type":"Common Stock","Currency":"EUR","ISIN":"ES0113679I37"}
		]`),
	})
	d := connectedDriver(t, srv)
	results, err := d.SearchInstrument(bg(), "BKT")
	if err != nil {
		t.Fatalf("SearchInstrument: %v", err)
	}
	found := false
	for _, r := range results {
		if r.Symbol == "BKT.MC" {
			found = true
			if r.Currency != "EUR" {
				t.Errorf("Currency: got %q, want EUR", r.Currency)
			}
		}
	}
	if !found {
		t.Error("expected BKT.MC (Bankinter, Madrid) in search results")
	}
}

func TestGetInstrument_AAPL(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
		"/search/AAPL": jsonHandler(`[
			{"Code":"AAPL","Exchange":"US","Name":"Apple Inc","Type":"Common Stock","Currency":"USD","ISIN":"US0378331005"}
		]`),
	})
	d := connectedDriver(t, srv)
	inst, err := d.GetInstrument(bg(), "AAPL.US")
	if err != nil {
		t.Fatalf("GetInstrument AAPL.US: %v", err)
	}
	if inst.Symbol != "AAPL.US" {
		t.Errorf("Symbol: got %q, want %q", inst.Symbol, "AAPL.US")
	}
	if inst.Currency != "USD" {
		t.Errorf("Currency: got %q, want %q", inst.Currency, "USD")
	}
	if inst.Exchange == "" {
		t.Error("Exchange is empty")
	}
}

func TestGetInstrument_BME(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
		"/search/BKT": jsonHandler(`[
			{"Code":"BKT","Exchange":"MC","Name":"Bankinter SA","Type":"Common Stock","Currency":"EUR","ISIN":"ES0113679I37"}
		]`),
	})
	d := connectedDriver(t, srv)
	inst, err := d.GetInstrument(bg(), "BKT.MC")
	if err != nil {
		t.Fatalf("GetInstrument BKT.MC: %v", err)
	}
	if inst.Symbol != "BKT.MC" {
		t.Errorf("Symbol: got %q, want %q", inst.Symbol, "BKT.MC")
	}
	if inst.Currency != "EUR" {
		t.Errorf("Currency: got %q, want EUR", inst.Currency)
	}
}

func TestGetInstrument_NotFound(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US":     jsonHandler(quoteAAPLFixture),
		"/search/ZZZZNOTREAL999": jsonHandler(`[]`),
	})
	d := connectedDriver(t, srv)
	_, err := d.GetInstrument(bg(), "ZZZZNOTREAL999.US")
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
	if !errors.Is(err, market.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestListInstruments_Stock(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
		"/exchange-symbol-list/US": jsonHandler(`[
			{"Code":"AAPL","Name":"Apple Inc","Exchange":"US","Currency":"USD","Type":"Common Stock","Isin":"US0378331005"},
			{"Code":"SPY","Name":"SPDR S&P 500","Exchange":"US","Currency":"USD","Type":"ETF","Isin":"US78462F1030"}
		]`),
	})
	d := connectedDriver(t, srv)
	list, err := d.ListInstruments(bg(), market.AssetTypeStock)
	if err != nil {
		t.Fatalf("ListInstruments stock: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least one stock")
	}
	for _, inst := range list {
		if inst.Type != market.AssetTypeStock {
			t.Errorf("got type %q, want AssetTypeStock (symbol=%s)", inst.Type, inst.Symbol)
			break
		}
	}
}

func TestListInstruments_Future(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
	})
	d := connectedDriver(t, srv)
	list, err := d.ListInstruments(bg(), market.AssetTypeFuture)
	if err != nil {
		t.Fatalf("ListInstruments Future: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty slice for futures, got %d entries", len(list))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Datos históricos
// ─────────────────────────────────────────────────────────────────────────────

func TestGetCandles_Daily_AAPL(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
		"/eod/AAPL.US": jsonHandler(`[
			{"date":"2024-01-02","open":185.1,"high":186.2,"low":184.5,"close":185.9,"volume":1000000},
			{"date":"2024-01-03","open":185.9,"high":187.0,"low":185.0,"close":186.4,"volume":1100000},
			{"date":"2024-01-04","open":186.4,"high":188.0,"low":186.0,"close":187.5,"volume":1200000}
		]`),
	})
	d := connectedDriver(t, srv)
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -30)
	candles, err := d.GetCandles(bg(), "AAPL.US", from, to, market.Timeframe1d)
	if err != nil {
		t.Fatalf("GetCandles daily: %v", err)
	}
	if len(candles) == 0 {
		t.Fatal("expected at least one daily candle")
	}
	assertAscendingTime(t, candles)
	assertCandlesNonZero(t, candles)
}

// TestGetCandles_Daily_BME prueba el caso de uso central: velas diarias de
// un valor del BME que FMP no cubre.
func TestGetCandles_Daily_BME(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
		"/eod/BKT.MC": jsonHandler(`[
			{"date":"2024-01-02","open":6.10,"high":6.25,"low":6.05,"close":6.20,"volume":500000},
			{"date":"2024-01-03","open":6.20,"high":6.30,"low":6.15,"close":6.28,"volume":520000}
		]`),
	})
	d := connectedDriver(t, srv)
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -30)
	candles, err := d.GetCandles(bg(), "BKT.MC", from, to, market.Timeframe1d)
	if err != nil {
		t.Fatalf("GetCandles daily BKT.MC: %v", err)
	}
	if len(candles) == 0 {
		t.Fatal("expected at least one daily candle for BKT.MC")
	}
	assertAscendingTime(t, candles)
	assertCandlesNonZero(t, candles)
}

// TestGetCandles_Intraday_RequiresSubscription documenta la restricción real
// del tier gratuito de EODHD: /api/intraday devuelve HTTP 403 para cualquier
// intervalo, verificado en vivo (ver test equivalente TestLive_* en
// eodhd_integration_test.go).
func TestGetCandles_Intraday_RequiresSubscription(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
		"/intraday/AAPL.US":  statusHandler(http.StatusForbidden),
	})
	d := connectedDriver(t, srv)
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -2)
	_, err := d.GetCandles(bg(), "AAPL.US", from, to, market.Timeframe1h)
	if err == nil {
		t.Fatal("expected error: free tier does not allow intraday data")
	}
	if !errors.Is(err, market.ErrSubscriptionRequired) {
		t.Errorf("expected ErrSubscriptionRequired, got: %v", err)
	}
}

func TestGetCorporateActions_AAPL(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
		"/div/AAPL.US":       jsonHandler(`[{"date":"2023-02-10","value":0.24}]`),
		"/splits/AAPL.US":    jsonHandler(`[]`),
	})
	d := connectedDriver(t, srv)
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Now().UTC()
	actions, err := d.GetCorporateActions(bg(), "AAPL.US", from, to)
	if err != nil {
		t.Fatalf("GetCorporateActions: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("expected at least one corporate action for AAPL")
	}
	hasDividend := false
	for _, a := range actions {
		if a.Type == "dividend" && a.Value > 0 {
			hasDividend = true
			break
		}
	}
	if !hasDividend {
		t.Error("expected at least one dividend with Value > 0")
	}
}

func TestGetCorporateActions_DateFilter(t *testing.T) {
	from := time.Now().UTC().AddDate(0, 0, -3)
	to := time.Now().UTC().AddDate(0, 0, -1)
	inRange := from.AddDate(0, 0, 1).Format(time.DateOnly)
	beforeRange := from.AddDate(0, 0, -5).Format(time.DateOnly)
	afterRange := to.AddDate(0, 0, 5).Format(time.DateOnly)

	dividends := fmt.Sprintf(`[
		{"date":%q,"value":0.10},
		{"date":%q,"value":0.20},
		{"date":%q,"value":0.30}
	]`, beforeRange, inRange, afterRange)

	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
		"/div/AAPL.US":       jsonHandler(dividends),
		"/splits/AAPL.US":    jsonHandler(`[]`),
	})
	d := connectedDriver(t, srv)
	actions, err := d.GetCorporateActions(bg(), "AAPL.US", from, to)
	if err != nil {
		t.Fatalf("GetCorporateActions date filter: %v", err)
	}
	for _, a := range actions {
		if a.Date.Before(from) || a.Date.After(to) {
			t.Errorf("action %v outside requested range [%v, %v]", a.Date, from, to)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Snapshot
// ─────────────────────────────────────────────────────────────────────────────

func TestGetQuote_AAPL(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
	})
	d := connectedDriver(t, srv)
	q, err := d.GetQuote(bg(), "AAPL.US")
	if err != nil {
		t.Fatalf("GetQuote AAPL.US: %v", err)
	}
	if q.Last <= 0 {
		t.Errorf("Last price should be > 0, got %f", q.Last)
	}
	if q.Time.IsZero() {
		t.Error("Time should not be zero")
	}
	if math.IsNaN(q.ChangePercent) || math.IsInf(q.ChangePercent, 0) {
		t.Errorf("ChangePercent should be a finite number, got %v", q.ChangePercent)
	}
}

func TestGetQuote_BME(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
		"/real-time/BKT.MC":  jsonHandler(`{"code":"BKT.MC","timestamp":1700000000,"close":6.28,"change_p":0.45}`),
	})
	d := connectedDriver(t, srv)
	q, err := d.GetQuote(bg(), "BKT.MC")
	if err != nil {
		t.Fatalf("GetQuote BKT.MC: %v", err)
	}
	if q.Last <= 0 {
		t.Errorf("Last price should be > 0, got %f", q.Last)
	}
}

// TestGetQuote_NotFound cubre el caso "NA": /api/real-time devuelve HTTP 200
// con campos "NA" para un símbolo inexistente en vez de HTTP 404 — el driver
// debe detectarlo y traducirlo a ErrNotFound explícitamente.
func TestGetQuote_NotFound(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US":           jsonHandler(quoteAAPLFixture),
		"/real-time/ZZZZNOTREAL999.US": jsonHandler(`{"code":"NA","timestamp":"NA","close":"NA","change_p":"NA"}`),
	})
	d := connectedDriver(t, srv)
	_, err := d.GetQuote(bg(), "ZZZZNOTREAL999.US")
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
	if !errors.Is(err, market.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Streaming
// ─────────────────────────────────────────────────────────────────────────────

func TestSubscribeQuotes(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US": jsonHandler(quoteAAPLFixture),
	})
	d := eodhd.New("test-key", eodhd.WithBaseURL(srv.URL), eodhd.WithPollInterval(50*time.Millisecond))
	if err := d.Connect(bg()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Disconnect(bg()) })

	ctx, cancel := context.WithTimeout(bg(), 2*time.Second)
	defer cancel()

	ch, err := d.SubscribeQuotes(ctx, "AAPL.US")
	if err != nil {
		t.Fatalf("SubscribeQuotes: %v", err)
	}

	select {
	case q, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before receiving any quote")
		}
		if q.Last <= 0 {
			t.Errorf("received quote with Last <= 0: %f", q.Last)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for first quote")
	}

	cancel()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timeout:
			t.Error("channel was not closed after context cancellation")
			return
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Fundamentales
// ─────────────────────────────────────────────────────────────────────────────

// TestGetFundamentals_RequiresSubscription documenta la restricción real del
// tier gratuito: /api/fundamentals devuelve HTTP 403, verificado en vivo (ver
// TestLive_* en eodhd_integration_test.go).
func TestGetFundamentals_RequiresSubscription(t *testing.T) {
	srv := newMockServer(t, mockRoutes{
		"/real-time/AAPL.US":    jsonHandler(quoteAAPLFixture),
		"/fundamentals/AAPL.US": statusHandler(http.StatusForbidden),
	})
	d := connectedDriver(t, srv)
	_, err := d.GetFundamentals(bg(), "AAPL.US")
	if err == nil {
		t.Fatal("expected error: free tier does not allow fundamentals data")
	}
	if !errors.Is(err, market.ErrSubscriptionRequired) {
		t.Errorf("expected ErrSubscriptionRequired, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers de aserción
// ─────────────────────────────────────────────────────────────────────────────

func assertAscendingTime(t *testing.T, candles []market.Candle) {
	t.Helper()
	for i := 1; i < len(candles); i++ {
		if candles[i].Time.Before(candles[i-1].Time) {
			t.Errorf("candles not in ascending order: index %d (%v) before index %d (%v)",
				i, candles[i].Time, i-1, candles[i-1].Time)
			return
		}
	}
}

func assertCandlesNonZero(t *testing.T, candles []market.Candle) {
	t.Helper()
	for _, c := range candles {
		if c.Open > 0 && c.Close > 0 {
			return
		}
	}
	t.Error("no candle with Open > 0 and Close > 0")
}
