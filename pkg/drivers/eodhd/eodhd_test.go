package eodhd_test

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"auris/pkg/drivers/eodhd"
	"auris/pkg/market"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers de setup
// ─────────────────────────────────────────────────────────────────────────────

// readAPIKey lee la API key del fichero de test_data.
// Si el fichero no existe o está vacío, omite el test.
func readAPIKey(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("test_data/eodhd_api_key")
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("test_data/eodhd_api_key not found; skipping integration test")
	}
	if err != nil {
		t.Fatalf("read api key: %v", err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		t.Skip("test_data/eodhd_api_key is empty; skipping integration test")
	}
	return key
}

// newConnectedDriver crea un Driver conectado y registra Disconnect en el cleanup.
func newConnectedDriver(t *testing.T) *eodhd.Driver {
	t.Helper()
	key := readAPIKey(t)
	d := eodhd.New(key)
	ctx := context.Background()
	if err := d.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Disconnect(context.Background())
	})
	return d
}

func bg() context.Context { return context.Background() }

// ─────────────────────────────────────────────────────────────────────────────
// Tests sin API key (siempre corren)
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

// ─────────────────────────────────────────────────────────────────────────────
// Auth & lifecycle
// ─────────────────────────────────────────────────────────────────────────────

func TestConnect_ValidKey(t *testing.T) {
	d := newConnectedDriver(t)
	if !d.IsConnected() {
		t.Error("IsConnected() should be true after Connect")
	}
}

func TestConnect_InvalidKey(t *testing.T) {
	readAPIKey(t) // asegura que hay acceso a internet
	d := eodhd.New("invalid_key_xxxx")
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
	d := newConnectedDriver(t)
	if err := d.Disconnect(bg()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if d.IsConnected() {
		t.Error("IsConnected() should be false after Disconnect")
	}
}

func TestRefreshToken_NoOp(t *testing.T) {
	d := newConnectedDriver(t)
	if err := d.RefreshToken(bg()); err != nil {
		t.Errorf("RefreshToken should be no-op, got: %v", err)
	}
}

func TestPing_Connected(t *testing.T) {
	d := newConnectedDriver(t)
	if err := d.Ping(bg()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestPing_NotConnected(t *testing.T) {
	d := eodhd.New(readAPIKey(t)) // nunca llamamos a Connect
	err := d.Ping(bg())
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if !errors.Is(err, market.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Descubrimiento de instrumentos
// ─────────────────────────────────────────────────────────────────────────────

func TestSearchInstrument_US(t *testing.T) {
	d := newConnectedDriver(t)
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
	d := newConnectedDriver(t)
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
	d := newConnectedDriver(t)
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
	d := newConnectedDriver(t)
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
	d := newConnectedDriver(t)
	_, err := d.GetInstrument(bg(), "ZZZZNOTREAL999.US")
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
	if !errors.Is(err, market.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestListInstruments_Stock(t *testing.T) {
	d := newConnectedDriver(t)
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
	d := newConnectedDriver(t)
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
	d := newConnectedDriver(t)
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
	d := newConnectedDriver(t)
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
// intervalo, verificado en vivo.
func TestGetCandles_Intraday_RequiresSubscription(t *testing.T) {
	d := newConnectedDriver(t)
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

func TestGetTicks_ErrNotSupported(t *testing.T) {
	d := newConnectedDriver(t)
	_, err := d.GetTicks(bg(), "AAPL.US", time.Now().AddDate(0, 0, -1), time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, market.ErrNotSupported) {
		t.Errorf("expected ErrNotSupported, got: %v", err)
	}
}

func TestGetCorporateActions_AAPL(t *testing.T) {
	d := newConnectedDriver(t)
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
	d := newConnectedDriver(t)
	from := time.Now().UTC().AddDate(0, 0, -3)
	to := time.Now().UTC().AddDate(0, 0, -1)
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
	d := newConnectedDriver(t)
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
	d := newConnectedDriver(t)
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
	d := newConnectedDriver(t)
	_, err := d.GetQuote(bg(), "ZZZZNOTREAL999.US")
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
	if !errors.Is(err, market.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetOrderBook_ErrNotSupported(t *testing.T) {
	d := newConnectedDriver(t)
	_, err := d.GetOrderBook(bg(), "AAPL.US", 10)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, market.ErrNotSupported) {
		t.Errorf("expected ErrNotSupported, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Streaming
// ─────────────────────────────────────────────────────────────────────────────

func TestSubscribeQuotes(t *testing.T) {
	key := readAPIKey(t)
	d := eodhd.New(key, eodhd.WithPollInterval(2*time.Second))
	if err := d.Connect(bg()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Disconnect(bg()) })

	ctx, cancel := context.WithTimeout(bg(), 12*time.Second)
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
	timeout := time.After(5 * time.Second)
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

func TestSubscribeTrades_ErrNotSupported(t *testing.T) {
	d := newConnectedDriver(t)
	_, err := d.SubscribeTrades(bg(), "AAPL.US")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, market.ErrNotSupported) {
		t.Errorf("expected ErrNotSupported, got: %v", err)
	}
}

func TestSubscribeOrderBook_ErrNotSupported(t *testing.T) {
	d := newConnectedDriver(t)
	_, err := d.SubscribeOrderBook(bg(), "AAPL.US", 10)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, market.ErrNotSupported) {
		t.Errorf("expected ErrNotSupported, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Fundamentales
// ─────────────────────────────────────────────────────────────────────────────

// TestGetFundamentals_RequiresSubscription documenta la restricción real del
// tier gratuito: /api/fundamentals devuelve HTTP 403, verificado en vivo.
func TestGetFundamentals_RequiresSubscription(t *testing.T) {
	d := newConnectedDriver(t)
	_, err := d.GetFundamentals(bg(), "AAPL.US")
	if err == nil {
		t.Fatal("expected error: free tier does not allow fundamentals data")
	}
	if !errors.Is(err, market.ErrSubscriptionRequired) {
		t.Errorf("expected ErrSubscriptionRequired, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Error wrapping
// ─────────────────────────────────────────────────────────────────────────────

func TestErrNotSupported_Wrapping(t *testing.T) {
	d := newConnectedDriver(t)
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

func TestErrNotConnected_Wrapping(t *testing.T) {
	d := eodhd.New(readAPIKey(t)) // nunca llamamos a Connect
	_, err := d.GetQuote(bg(), "AAPL.US")
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if !errors.Is(err, market.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
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
