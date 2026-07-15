package fmp_test

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fuchicar/auris-ai/pkg/drivers/fmp"
	"github.com/fuchicar/auris-ai/pkg/market"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers de setup
// ─────────────────────────────────────────────────────────────────────────────

// readAPIKey lee la API key del fichero de test_data.
// Si el fichero no existe o está vacío, omite el test.
func readAPIKey(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("test_data/fmp_api_key")
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("test_data/fmp_api_key not found; skipping integration test")
	}
	if err != nil {
		t.Fatalf("read api key: %v", err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		t.Skip("test_data/fmp_api_key is empty; skipping integration test")
	}
	return key
}

// newConnectedDriver crea un Driver conectado y registra Disconnect en el cleanup.
func newConnectedDriver(t *testing.T) *fmp.Driver {
	t.Helper()
	key := readAPIKey(t)
	d := fmp.New(key)
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
	var _ market.ProviderAPI = (*fmp.Driver)(nil)
	var _ market.CapabilityReporter = (*fmp.Driver)(nil)
}

func TestDescription_NonEmpty(t *testing.T) {
	d := fmp.New("dummy")
	if d.Description() == "" {
		t.Error("Description() returned empty string")
	}
}

func TestUnsupportedTools(t *testing.T) {
	d := fmp.New("dummy")
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
	d := fmp.New("invalid_key_xxxx")
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
	readAPIKey(t)
	d := fmp.New(readAPIKey(t))
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

func TestSearchInstrument(t *testing.T) {
	d := newConnectedDriver(t)
	// Buscamos por ticker, que FMP soporta mejor en el plan gratuito.
	results, err := d.SearchInstrument(bg(), "AAPL")
	if errors.Is(err, market.ErrSubscriptionRequired) {
		t.Skip("SearchInstrument requires a higher subscription plan")
	}
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

func TestGetInstrument_AAPL(t *testing.T) {
	d := newConnectedDriver(t)
	inst, err := d.GetInstrument(bg(), "AAPL")
	if err != nil {
		t.Fatalf("GetInstrument AAPL: %v", err)
	}
	if inst.Symbol != "AAPL" {
		t.Errorf("Symbol: got %q, want %q", inst.Symbol, "AAPL")
	}
	if inst.Currency != "USD" {
		t.Errorf("Currency: got %q, want %q", inst.Currency, "USD")
	}
	if inst.Exchange == "" {
		t.Error("Exchange is empty")
	}
}

func TestGetInstrument_NotFound(t *testing.T) {
	d := newConnectedDriver(t)
	_, err := d.GetInstrument(bg(), "ZZZZNOTREAL999")
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
	if errors.Is(err, market.ErrSubscriptionRequired) {
		t.Skip("stock-list requires a higher subscription plan")
	}
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

func TestListInstruments_ETF(t *testing.T) {
	d := newConnectedDriver(t)
	list, err := d.ListInstruments(bg(), market.AssetTypeETF)
	if errors.Is(err, market.ErrSubscriptionRequired) {
		t.Skip("stock-list requires a higher subscription plan")
	}
	if err != nil {
		t.Fatalf("ListInstruments ETF: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least one ETF")
	}
	for _, inst := range list {
		if inst.Type != market.AssetTypeETF {
			t.Errorf("got type %q, want AssetTypeETF (symbol=%s)", inst.Type, inst.Symbol)
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

func TestGetCandles_Daily(t *testing.T) {
	d := newConnectedDriver(t)
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -30)
	candles, err := d.GetCandles(bg(), "AAPL", from, to, market.Timeframe1d)
	if errors.Is(err, market.ErrSubscriptionRequired) {
		t.Skip("historical-price-eod requires a higher subscription plan")
	}
	if err != nil {
		t.Fatalf("GetCandles daily: %v", err)
	}
	if len(candles) == 0 {
		t.Fatal("expected at least one daily candle")
	}
	assertAscendingTime(t, candles)
	assertCandlesNonZero(t, candles)
}

func TestGetCandles_1m(t *testing.T) {
	d := newConnectedDriver(t)
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -2)
	candles, err := d.GetCandles(bg(), "AAPL", from, to, market.Timeframe1m)
	if errors.Is(err, market.ErrSubscriptionRequired) {
		t.Skip("intraday candles require a higher subscription plan")
	}
	if err != nil {
		t.Fatalf("GetCandles 1m: %v", err)
	}
	if len(candles) == 0 {
		t.Fatal("expected at least one 1m candle")
	}
	assertAscendingTime(t, candles)
}

func TestGetCandles_5m(t *testing.T) {
	d := newConnectedDriver(t)
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -2)
	candles, err := d.GetCandles(bg(), "AAPL", from, to, market.Timeframe5m)
	if errors.Is(err, market.ErrSubscriptionRequired) {
		t.Skip("intraday candles require a higher subscription plan")
	}
	if err != nil {
		t.Fatalf("GetCandles 5m: %v", err)
	}
	if len(candles) == 0 {
		t.Fatal("expected at least one 5m candle")
	}
	assertAscendingTime(t, candles)
}

func TestGetCandles_1h(t *testing.T) {
	d := newConnectedDriver(t)
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -5)
	candles, err := d.GetCandles(bg(), "AAPL", from, to, market.Timeframe1h)
	if errors.Is(err, market.ErrSubscriptionRequired) {
		t.Skip("intraday candles require a higher subscription plan")
	}
	if err != nil {
		t.Fatalf("GetCandles 1h: %v", err)
	}
	if len(candles) == 0 {
		t.Fatal("expected at least one 1h candle")
	}
	assertAscendingTime(t, candles)
}

func TestGetTicks_ErrNotSupported(t *testing.T) {
	d := newConnectedDriver(t)
	_, err := d.GetTicks(bg(), "AAPL", time.Now().AddDate(0, 0, -1), time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, market.ErrNotSupported) {
		t.Errorf("expected ErrNotSupported, got: %v", err)
	}
}

func TestGetCorporateActions_AAPL(t *testing.T) {
	d := newConnectedDriver(t)
	// AAPL tiene dividendos históricamente
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Now().UTC()
	actions, err := d.GetCorporateActions(bg(), "AAPL", from, to)
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
	// Rango muy reciente y estrecho: improbable que haya eventos
	from := time.Now().UTC().AddDate(0, 0, -3)
	to := time.Now().UTC().AddDate(0, 0, -1)
	actions, err := d.GetCorporateActions(bg(), "AAPL", from, to)
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
	q, err := d.GetQuote(bg(), "AAPL")
	if err != nil {
		t.Fatalf("GetQuote AAPL: %v", err)
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

func TestGetQuote_NotFound(t *testing.T) {
	d := newConnectedDriver(t)
	_, err := d.GetQuote(bg(), "ZZZZNOTREAL999")
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
	// FMP puede devolver 402 (premium check) o 404 dependiendo del plan.
	if !errors.Is(err, market.ErrNotFound) && !errors.Is(err, market.ErrSubscriptionRequired) {
		t.Errorf("expected ErrNotFound or ErrSubscriptionRequired, got: %v", err)
	}
}

func TestGetOrderBook_ErrNotSupported(t *testing.T) {
	d := newConnectedDriver(t)
	_, err := d.GetOrderBook(bg(), "AAPL", 10)
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
	// Polling rápido para que el test no tarde demasiado
	d := fmp.New(key, fmp.WithPollInterval(2*time.Second))
	if err := d.Connect(bg()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Disconnect(bg()) })

	ctx, cancel := context.WithTimeout(bg(), 12*time.Second)
	defer cancel()

	ch, err := d.SubscribeQuotes(ctx, "AAPL")
	if err != nil {
		t.Fatalf("SubscribeQuotes: %v", err)
	}

	// Esperar al menos una quote
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

	// Cancelar y verificar que el canal se cierra
	cancel()
	// Drenar hasta que se cierre
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // canal cerrado correctamente
			}
		case <-timeout:
			t.Error("channel was not closed after context cancellation")
			return
		}
	}
}

func TestSubscribeTrades_ErrNotSupported(t *testing.T) {
	d := newConnectedDriver(t)
	_, err := d.SubscribeTrades(bg(), "AAPL")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, market.ErrNotSupported) {
		t.Errorf("expected ErrNotSupported, got: %v", err)
	}
}

func TestSubscribeOrderBook_ErrNotSupported(t *testing.T) {
	d := newConnectedDriver(t)
	_, err := d.SubscribeOrderBook(bg(), "AAPL", 10)
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

func TestGetFundamentals_AAPL(t *testing.T) {
	d := newConnectedDriver(t)
	f, err := d.GetFundamentals(bg(), "AAPL")
	if errors.Is(err, market.ErrSubscriptionRequired) {
		t.Skip("key-metrics-ttm requires a higher subscription plan")
	}
	if err != nil {
		t.Fatalf("GetFundamentals AAPL: %v", err)
	}
	if f.MarketCap <= 0 {
		t.Errorf("MarketCap should be > 0, got %f", f.MarketCap)
	}
}

func TestGetFundamentals_NotFound(t *testing.T) {
	d := newConnectedDriver(t)
	_, err := d.GetFundamentals(bg(), "ZZZZNOTREAL999")
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
	// FMP puede devolver 402 (premium check) o 404 dependiendo del plan.
	if !errors.Is(err, market.ErrNotFound) && !errors.Is(err, market.ErrSubscriptionRequired) {
		t.Errorf("expected ErrNotFound or ErrSubscriptionRequired, got: %v", err)
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
			_, err := d.GetTicks(bg(), "AAPL", now.AddDate(0, 0, -1), now)
			return err
		}},
		{"GetOrderBook", func() error {
			_, err := d.GetOrderBook(bg(), "AAPL", 10)
			return err
		}},
		{"SubscribeTrades", func() error {
			_, err := d.SubscribeTrades(bg(), "AAPL")
			return err
		}},
		{"SubscribeOrderBook", func() error {
			_, err := d.SubscribeOrderBook(bg(), "AAPL", 10)
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
	readAPIKey(t)
	d := fmp.New(readAPIKey(t)) // nunca llamamos a Connect
	_, err := d.GetQuote(bg(), "AAPL")
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
			return // al menos una vela con valores válidos
		}
	}
	t.Error("no candle with Open > 0 and Close > 0")
}
