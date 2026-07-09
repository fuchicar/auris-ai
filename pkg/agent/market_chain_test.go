package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"auris/pkg/market"
)

// stubMarket is a minimal, fully error-injectable market.ProviderAPI used only
// to exercise marketChain's cascade logic in isolation (distinct from the
// richer mockMarket in agent_test.go, which is geared towards dispatch tests).
type stubMarket struct {
	connectErr error
	connected  bool

	quote    market.Quote
	quoteErr error
	calls    int // GetQuote call counter, to assert a provider was/wasn't reached

	fundamental market.Fundamental
	fundErr     error
}

func (s *stubMarket) Name() string        { return "stub" }
func (s *stubMarket) DocsURL() string     { return "https://stub.example/docs" }
func (s *stubMarket) Description() string { return "stub provider" }

func (s *stubMarket) Connect(_ context.Context) error {
	if s.connectErr != nil {
		return s.connectErr
	}
	s.connected = true
	return nil
}
func (s *stubMarket) Disconnect(_ context.Context) error {
	s.connected = false
	return nil
}
func (s *stubMarket) RefreshToken(_ context.Context) error { return nil }
func (s *stubMarket) IsConnected() bool                    { return s.connected }
func (s *stubMarket) Ping(_ context.Context) error {
	if !s.connected {
		return market.ErrNotConnected
	}
	return nil
}

func (s *stubMarket) SearchInstrument(_ context.Context, _ string) ([]market.Instrument, error) {
	return nil, market.ErrNotSupported
}
func (s *stubMarket) GetInstrument(_ context.Context, _ string) (market.Instrument, error) {
	return market.Instrument{}, market.ErrNotSupported
}
func (s *stubMarket) ListInstruments(_ context.Context, _ market.AssetType) ([]market.Instrument, error) {
	return nil, market.ErrNotSupported
}
func (s *stubMarket) GetCandles(_ context.Context, _ string, _, _ time.Time, _ market.Timeframe) ([]market.Candle, error) {
	return nil, market.ErrNotSupported
}
func (s *stubMarket) GetTicks(_ context.Context, _ string, _, _ time.Time) ([]market.Tick, error) {
	return nil, market.ErrNotSupported
}
func (s *stubMarket) GetCorporateActions(_ context.Context, _ string, _, _ time.Time) ([]market.CorporateAction, error) {
	return nil, market.ErrNotSupported
}
func (s *stubMarket) GetQuote(_ context.Context, _ string) (market.Quote, error) {
	s.calls++
	return s.quote, s.quoteErr
}
func (s *stubMarket) GetOrderBook(_ context.Context, _ string, _ int) (market.OrderBook, error) {
	return market.OrderBook{}, market.ErrNotSupported
}
func (s *stubMarket) SubscribeQuotes(_ context.Context, _ string) (<-chan market.Quote, error) {
	return nil, market.ErrNotSupported
}
func (s *stubMarket) SubscribeTrades(_ context.Context, _ string) (<-chan market.Tick, error) {
	return nil, market.ErrNotSupported
}
func (s *stubMarket) SubscribeOrderBook(_ context.Context, _ string, _ int) (<-chan market.OrderBook, error) {
	return nil, market.ErrNotSupported
}
func (s *stubMarket) GetFundamentals(_ context.Context, _ string) (market.Fundamental, error) {
	return s.fundamental, s.fundErr
}

var _ market.ProviderAPI = (*stubMarket)(nil)

func TestMarketChain_InterfaceCompliance(t *testing.T) {
	var _ market.ProviderAPI = (*marketChain)(nil)
}

func TestMarketChain_SucceedsOnPrimary_SecondaryNeverCalled(t *testing.T) {
	primary := &stubMarket{quote: market.Quote{Last: 100}}
	secondary := &stubMarket{quote: market.Quote{Last: 999}}
	chain := NewMarketChain(primary, secondary)

	q, err := chain.GetQuote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if q.Last != 100 {
		t.Errorf("Last = %v, want 100 (from primary)", q.Last)
	}
	if secondary.calls != 0 {
		t.Errorf("secondary.calls = %d, want 0 (should not be reached)", secondary.calls)
	}
}

func TestMarketChain_FallsBackOn(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"ErrNotFound", market.ErrNotFound},
		{"ErrNotSupported", market.ErrNotSupported},
		{"ErrRateLimit", market.ErrRateLimit},
		{"ErrSubscriptionRequired", market.ErrSubscriptionRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			primary := &stubMarket{quoteErr: tc.err}
			secondary := &stubMarket{quote: market.Quote{Last: 42}}
			chain := NewMarketChain(primary, secondary)

			q, err := chain.GetQuote(context.Background(), "BKT.MC")
			if err != nil {
				t.Fatalf("GetQuote: %v", err)
			}
			if q.Last != 42 {
				t.Errorf("Last = %v, want 42 (from secondary)", q.Last)
			}
			if secondary.calls != 1 {
				t.Errorf("secondary.calls = %d, want 1", secondary.calls)
			}
		})
	}
}

func TestMarketChain_DoesNotFallBackOn(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"ErrUnauthorized", market.ErrUnauthorized},
		{"ErrNotConnected", market.ErrNotConnected},
		{"generic error", errors.New("boom")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			primary := &stubMarket{quoteErr: tc.err}
			secondary := &stubMarket{quote: market.Quote{Last: 42}}
			chain := NewMarketChain(primary, secondary)

			_, err := chain.GetQuote(context.Background(), "AAPL")
			if !errors.Is(err, tc.err) && err.Error() != tc.err.Error() {
				t.Errorf("got err %v, want %v", err, tc.err)
			}
			if secondary.calls != 0 {
				t.Errorf("secondary.calls = %d, want 0 (should not cascade)", secondary.calls)
			}
		})
	}
}

func TestMarketChain_ReturnsPrimaryErrorWhenNoneCover(t *testing.T) {
	primary := &stubMarket{quoteErr: market.ErrNotFound}
	secondary := &stubMarket{quoteErr: market.ErrSubscriptionRequired}
	chain := NewMarketChain(primary, secondary)

	_, err := chain.GetQuote(context.Background(), "ZZZZ")
	if !errors.Is(err, market.ErrNotFound) {
		t.Errorf("expected the primary's ErrNotFound, got: %v", err)
	}
	if errors.Is(err, market.ErrSubscriptionRequired) {
		t.Error("should not surface the secondary's error when the primary's is cascadable but exhausted")
	}
}

func TestMarketChain_AppliesUniformlyToOtherMethods(t *testing.T) {
	// GetFundamentals exercises the same chainCall path as GetQuote — confirms
	// the cascade isn't special-cased to a single method.
	primary := &stubMarket{fundErr: market.ErrSubscriptionRequired}
	secondary := &stubMarket{fundamental: market.Fundamental{Symbol: "AAPL", PERatio: 30}}
	chain := NewMarketChain(primary, secondary)

	f, err := chain.GetFundamentals(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetFundamentals: %v", err)
	}
	if f.PERatio != 30 {
		t.Errorf("PERatio = %v, want 30 (from secondary)", f.PERatio)
	}
}

func TestMarketChain_Connect_PrimaryFailureIsFatal(t *testing.T) {
	primary := &stubMarket{connectErr: errors.New("bad key")}
	secondary := &stubMarket{}
	chain := NewMarketChain(primary, secondary)

	if err := chain.Connect(context.Background()); err == nil {
		t.Fatal("expected error when primary fails to connect")
	}
	if chain.IsConnected() {
		t.Error("IsConnected() should be false when primary failed to connect")
	}
}

func TestMarketChain_Connect_SecondaryFailureIsNonFatal(t *testing.T) {
	primary := &stubMarket{}
	secondary := &stubMarket{connectErr: errors.New("secondary down")}
	chain := NewMarketChain(primary, secondary)

	if err := chain.Connect(context.Background()); err != nil {
		t.Fatalf("Connect should succeed when only the secondary fails: %v", err)
	}
	if !chain.IsConnected() {
		t.Error("IsConnected() should be true (primary connected)")
	}

	// A secondary that never connected fails its calls with ErrNotConnected,
	// which is not cascadable, so a primary's cascadable error stops there.
	unconnectedSecondary := &stubMarket{quoteErr: market.ErrNotConnected}
	primary2 := &stubMarket{connected: true, quoteErr: market.ErrNotFound}
	chain2 := NewMarketChain(primary2, unconnectedSecondary)
	_, err := chain2.GetQuote(context.Background(), "AAPL")
	if !errors.Is(err, market.ErrNotFound) {
		t.Errorf("expected the primary's ErrNotFound (secondary never connected), got: %v", err)
	}
}

func TestMarketChain_IsConnected_DelegatesToPrimary(t *testing.T) {
	primary := &stubMarket{connected: true}
	secondary := &stubMarket{connected: false}
	chain := NewMarketChain(primary, secondary)
	if !chain.IsConnected() {
		t.Error("IsConnected() should delegate to the primary (true)")
	}
}

func TestMarketChain_Ping_DelegatesToPrimary(t *testing.T) {
	primary := &stubMarket{connected: true}
	secondary := &stubMarket{connected: false}
	chain := NewMarketChain(primary, secondary)
	if err := chain.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestMarketChain_SingleProvider_Passthrough(t *testing.T) {
	primary := &stubMarket{quote: market.Quote{Last: 7}}
	chain := NewMarketChain(primary)

	q, err := chain.GetQuote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if q.Last != 7 {
		t.Errorf("Last = %v, want 7", q.Last)
	}
}

func TestMarketChain_NameAndDescription_JoinProviders(t *testing.T) {
	chain := NewMarketChain(&stubMarket{}, &stubMarket{})
	if got := chain.Name(); got != "stub + stub" {
		t.Errorf("Name() = %q, want %q", got, "stub + stub")
	}
	if got := chain.Description(); got != "stub provider + stub provider" {
		t.Errorf("Description() = %q, want %q", got, "stub provider + stub provider")
	}
}

func TestMarketChain_DocsURL_ReturnsPrimarys(t *testing.T) {
	chain := NewMarketChain(&stubMarket{}, &stubMarket{})
	if got := chain.DocsURL(); got != "https://stub.example/docs" {
		t.Errorf("DocsURL() = %q, want primary's URL", got)
	}
}
