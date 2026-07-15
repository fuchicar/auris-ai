package simulation

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/fuchicar/auris-ai/pkg/market"
	"github.com/fuchicar/auris-ai/pkg/news"
)

var _ market.ProviderAPI = (*Driver)(nil)
var _ market.CapabilityReporter = (*Driver)(nil)
var _ news.Source = (*NewsSource)(nil)

func newConnectedDriver(t *testing.T) *Driver {
	t.Helper()
	d := New()
	if err := d.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Disconnect(context.Background()) })
	return d
}

func TestName_NonEmpty(t *testing.T) {
	if New().Name() == "" {
		t.Fatal("Name() is empty")
	}
}

func TestDescription_NonEmpty(t *testing.T) {
	if New().Description() == "" {
		t.Fatal("Description() is empty")
	}
}

func TestUnsupportedTools_Empty(t *testing.T) {
	if tools := New().UnsupportedTools(); tools != nil {
		t.Fatalf("expected nil (fully capable), got %v", tools)
	}
}

func TestConnect_LoadsUniverse(t *testing.T) {
	d := newConnectedDriver(t)
	if !d.IsConnected() {
		t.Fatal("expected IsConnected() == true after Connect")
	}
	instruments, err := d.ListInstruments(context.Background(), market.AssetTypeStock)
	if err != nil {
		t.Fatalf("ListInstruments: %v", err)
	}
	if len(instruments) < 30 {
		t.Fatalf("expected at least 30 stock instruments, got %d", len(instruments))
	}
}

func TestNotConnected_ReturnsErrNotConnected(t *testing.T) {
	d := New()
	_, err := d.GetQuote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error before Connect")
	}
}

func TestListInstruments_SameSetRegardlessOfQuery(t *testing.T) {
	d := newConnectedDriver(t)
	// ListInstruments takes only an AssetType, no country — so the same fixed
	// universe is always returned no matter which country a caller has in mind.
	crypto, err := d.ListInstruments(context.Background(), market.AssetTypeCrypto)
	if err != nil {
		t.Fatalf("ListInstruments crypto: %v", err)
	}
	if len(crypto) < 3 {
		t.Fatalf("expected at least 3 crypto instruments, got %d", len(crypto))
	}
}

func TestListInstruments_ETFs(t *testing.T) {
	d := newConnectedDriver(t)
	etfs, err := d.ListInstruments(context.Background(), market.AssetTypeETF)
	if err != nil {
		t.Fatalf("ListInstruments etf: %v", err)
	}
	if len(etfs) < 5 {
		t.Fatalf("expected at least 5 ETF instruments (S&P 500, MSCI World, emerging markets, IBEX 35), got %d", len(etfs))
	}
}

func TestGetQuote_ETF(t *testing.T) {
	d := newConnectedDriver(t)
	for _, symbol := range []string{"SPY", "URTH", "EEM", "IBEX35.MC"} {
		q, err := d.GetQuote(context.Background(), symbol)
		if err != nil {
			t.Fatalf("GetQuote %q: %v", symbol, err)
		}
		if q.Last <= 0 {
			t.Fatalf("GetQuote %q: expected positive Last, got %v", symbol, q.Last)
		}
	}
}

func TestSearchInstrument_FindsRealAndFakeNames(t *testing.T) {
	d := newConnectedDriver(t)
	apple, err := d.SearchInstrument(context.Background(), "Apple")
	if err != nil || len(apple) == 0 {
		t.Fatalf("expected to find Apple, got %v err=%v", apple, err)
	}
	fake, err := d.SearchInstrument(context.Background(), "Nováx")
	if err != nil || len(fake) == 0 {
		t.Fatalf("expected to find invented company, got %v err=%v", fake, err)
	}
}

func TestGetInstrument_UnknownSymbol_ErrNotFound(t *testing.T) {
	d := newConnectedDriver(t)
	if _, err := d.GetInstrument(context.Background(), "DOESNOTEXIST"); err == nil {
		t.Fatal("expected ErrNotFound for unknown symbol")
	}
}

func TestGetCandles_Deterministic(t *testing.T) {
	d := newConnectedDriver(t)
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	c1, err := d.GetCandles(context.Background(), "AAPL", from, to, market.Timeframe1d)
	if err != nil {
		t.Fatalf("GetCandles: %v", err)
	}
	c2, err := d.GetCandles(context.Background(), "AAPL", from, to, market.Timeframe1d)
	if err != nil {
		t.Fatalf("GetCandles (2nd call): %v", err)
	}
	if len(c1) == 0 || len(c1) != len(c2) {
		t.Fatalf("expected identical non-empty candle series, got %d vs %d", len(c1), len(c2))
	}
	for i := range c1 {
		if c1[i] != c2[i] {
			t.Fatalf("candle %d differs between calls: %+v vs %+v", i, c1[i], c2[i])
		}
	}
}

func TestGetCandles_NeverExtendsIntoTheFuture(t *testing.T) {
	d := newConnectedDriver(t)
	from := time.Now().AddDate(0, -1, 0)
	to := time.Now().AddDate(1, 0, 0) // one year in the future
	candles, err := d.GetCandles(context.Background(), "MSFT", from, to, market.Timeframe1d)
	if err != nil {
		t.Fatalf("GetCandles: %v", err)
	}
	now := truncateDay(time.Now())
	for _, c := range candles {
		if c.Time.After(now) {
			t.Fatalf("candle dated %v is after now (%v)", c.Time, now)
		}
	}
}

func TestInstruments_CorrelateOnBigMarketDays(t *testing.T) {
	// The shared market factor should move most instruments the same
	// direction on days where it has a large magnitude — verifying the
	// "coherent, not independent noise" requirement.
	u, err := loadUniverse()
	if err != nil {
		t.Fatalf("loadUniverse: %v", err)
	}
	epoch, err := parseEpoch(u.Epoch)
	if err != nil {
		t.Fatalf("parseEpoch: %v", err)
	}
	gen := newGenerator(u, epoch)

	// Find a trading day with a comparatively large market-factor draw
	// (marketFactorDailyVol is 0.008, so 1.5 stdev ~= 0.012 happens often
	// enough within a year of trading days to find reliably).
	var bigDay time.Time
	for d := epoch.AddDate(0, 0, 1); d.Before(epoch.AddDate(1, 0, 0)); d = d.AddDate(0, 0, 1) {
		if !isTradingDay(d) {
			continue
		}
		if mf := gen.marketFactorReturn(d); math.Abs(mf) > 0.012 {
			bigDay = d
			break
		}
	}
	if bigDay.IsZero() {
		t.Fatal("no sufficiently large market-factor day found within a year — check marketFactorDailyVol")
	}
	mf := gen.marketFactorReturn(bigDay)

	agree := 0
	total := 0
	for _, inst := range gen.instruments {
		if inst.Beta <= 0 {
			continue
		}
		ret := gen.dailyReturn(inst, bigDay)
		total++
		if (ret > 0) == (mf > 0) {
			agree++
		}
	}
	if total == 0 {
		t.Fatal("no positive-beta instruments to check")
	}
	// Most positive-beta instruments should move in the same direction as
	// the shared market factor on a day where it dominates.
	if float64(agree)/float64(total) < 0.6 {
		t.Fatalf("expected most instruments to move with the market factor, agreement=%d/%d", agree, total)
	}
}

func TestGetFundamentals_Deterministic(t *testing.T) {
	d := newConnectedDriver(t)
	f1, err := d.GetFundamentals(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetFundamentals: %v", err)
	}
	f2, err := d.GetFundamentals(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetFundamentals (2nd call): %v", err)
	}
	if f1 != f2 {
		t.Fatalf("expected identical fundamentals within the same process run, got %+v vs %+v", f1, f2)
	}
	if f1.MarketCap <= 0 {
		t.Fatalf("expected positive MarketCap, got %v", f1.MarketCap)
	}
}

func TestGetCorporateActions_OnlyDividends(t *testing.T) {
	d := newConnectedDriver(t)
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Now()
	actions, err := d.GetCorporateActions(context.Background(), "KO", from, to)
	if err != nil {
		t.Fatalf("GetCorporateActions: %v", err)
	}
	for _, a := range actions {
		if a.Type != "dividend" {
			t.Fatalf("expected only dividend actions, got %q", a.Type)
		}
	}
}

func TestGetOrderBook_And_GetTicks_Supported(t *testing.T) {
	d := newConnectedDriver(t)
	ob, err := d.GetOrderBook(context.Background(), "AAPL", 5)
	if err != nil {
		t.Fatalf("GetOrderBook: %v (should be supported — synthetic driver)", err)
	}
	if len(ob.Bids) != 5 || len(ob.Asks) != 5 {
		t.Fatalf("expected 5 bid/ask levels, got %d/%d", len(ob.Bids), len(ob.Asks))
	}
	ticks, err := d.GetTicks(context.Background(), "AAPL", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("GetTicks: %v (should be supported — synthetic driver)", err)
	}
	if len(ticks) == 0 {
		t.Fatal("expected at least one synthetic tick")
	}
}

func TestSubscribeQuotes_EmitsAndClosesOnCancel(t *testing.T) {
	d := New(WithPollInterval(50 * time.Millisecond))
	if err := d.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := d.SubscribeQuotes(ctx, "AAPL")
	if err != nil {
		t.Fatalf("SubscribeQuotes: %v", err)
	}
	select {
	case q := <-ch:
		if q.Last <= 0 {
			t.Fatalf("expected positive Last price, got %v", q.Last)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first quote")
	}
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			// drain until closed
			for range ch {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after cancel")
	}
}

func TestNewsSource_ProducesHeadlines(t *testing.T) {
	d := newConnectedDriver(t)
	src := d.NewsSource()
	items, err := src.HandleFetchNews(context.Background(), news.FetchNewsParams{MaxAgeHours: 24 * 30 * 6, MaxResults: 10})
	if err != nil {
		t.Fatalf("HandleFetchNews: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one synthetic news item")
	}
	for _, it := range items {
		if it.Title == "" || it.Source == "" {
			t.Fatalf("incomplete news item: %+v", it)
		}
	}
}
