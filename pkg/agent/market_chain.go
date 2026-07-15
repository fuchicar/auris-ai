package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fuchicar/auris-ai/pkg/market"
)

// marketChain wraps an ordered list of market.ProviderAPI and cascades through
// them on each call. providers[0] is the primary; the rest are fallbacks tried
// in order. See FEAT-7 in doc/task_completed.md for the product rationale (FMP primary +
// EODHD secondary, to cover the BME symbols FMP's free tier doesn't serve).
type marketChain struct {
	providers []market.ProviderAPI
}

// NewMarketChain wraps providers into a single market.ProviderAPI that tries
// each in order on every data call. It falls back to the next provider only on
// errors that mean "I don't cover this" (ErrNotFound, ErrNotSupported,
// ErrRateLimit, ErrSubscriptionRequired) — not on ErrUnauthorized,
// ErrNotConnected, or other errors, which indicate a configuration problem that
// should surface directly rather than being silently masked by a fallback. If
// no provider resolves a call, the primary's (providers[0]) error is returned,
// never the last-tried provider's.
//
// Passing a single provider is a harmless no-op: the chain degrades to a
// transparent passthrough.
func NewMarketChain(providers ...market.ProviderAPI) market.ProviderAPI {
	return &marketChain{providers: providers}
}

var _ market.ProviderAPI = (*marketChain)(nil)
var _ market.CapabilityReporter = (*marketChain)(nil)

// UnsupportedTools implements market.CapabilityReporter for the chain: a
// tool is unsupported by the chain only if every provider in it declares the
// tool unsupported. If any provider doesn't implement CapabilityReporter its
// capabilities are unknown, so nothing is filtered — see FEAT-8 in doc/task_completed.md.
func (c *marketChain) UnsupportedTools() []string {
	if len(c.providers) == 0 {
		return nil
	}
	sets := make([][]string, 0, len(c.providers))
	for _, p := range c.providers {
		cr, ok := p.(market.CapabilityReporter)
		if !ok {
			return nil
		}
		sets = append(sets, cr.UnsupportedTools())
	}
	return intersectToolNames(sets)
}

// intersectToolNames returns the tool names present in every set.
func intersectToolNames(sets [][]string) []string {
	counts := make(map[string]int)
	for _, set := range sets {
		seen := make(map[string]bool, len(set))
		for _, name := range set {
			if !seen[name] {
				counts[name]++
				seen[name] = true
			}
		}
	}
	var result []string
	for name, count := range counts {
		if count == len(sets) {
			result = append(result, name)
		}
	}
	return result
}

// isCascadable reports whether err should trigger trying the next provider in
// the chain rather than being returned immediately.
func isCascadable(err error) bool {
	return errors.Is(err, market.ErrNotFound) ||
		errors.Is(err, market.ErrNotSupported) ||
		errors.Is(err, market.ErrRateLimit) ||
		errors.Is(err, market.ErrSubscriptionRequired)
}

// chainCall tries fn against each provider in order, cascading only on
// isCascadable errors, and returns the primary's error if none succeed.
func chainCall[T any](c *marketChain, fn func(market.ProviderAPI) (T, error)) (T, error) {
	var firstErr error
	for _, p := range c.providers {
		result, err := fn(p)
		if err == nil {
			return result, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if !isCascadable(err) {
			break
		}
	}
	var zero T
	return zero, firstErr
}

func (c *marketChain) Name() string {
	names := make([]string, len(c.providers))
	for i, p := range c.providers {
		names[i] = p.Name()
	}
	return strings.Join(names, " + ")
}

func (c *marketChain) DocsURL() string {
	if len(c.providers) == 0 {
		return ""
	}
	return c.providers[0].DocsURL()
}

func (c *marketChain) Description() string {
	descs := make([]string, len(c.providers))
	for i, p := range c.providers {
		descs[i] = p.Description()
	}
	return strings.Join(descs, " + ")
}

// Connect connects every provider in order. Only the primary's failure is
// fatal; a secondary that fails to connect is non-fatal — its calls will fail
// with ErrNotConnected afterwards, which isCascadable excludes, so it simply
// drops out of the chain without needing extra bookkeeping.
func (c *marketChain) Connect(ctx context.Context) error {
	if len(c.providers) == 0 {
		return fmt.Errorf("market_chain: Connect: %w", market.ErrNotConnected)
	}
	if err := c.providers[0].Connect(ctx); err != nil {
		return err
	}
	for _, p := range c.providers[1:] {
		_ = p.Connect(ctx)
	}
	return nil
}

// Disconnect disconnects every provider, returning the first error encountered (if any).
func (c *marketChain) Disconnect(ctx context.Context) error {
	var firstErr error
	for _, p := range c.providers {
		if err := p.Disconnect(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// RefreshToken refreshes every provider, returning the first error encountered (if any).
func (c *marketChain) RefreshToken(ctx context.Context) error {
	var firstErr error
	for _, p := range c.providers {
		if err := p.RefreshToken(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// IsConnected delegates to the primary provider: it defines the chain's
// connected state, since it is mandatory (Connect fails the whole chain if the
// primary fails); the secondary is best-effort.
func (c *marketChain) IsConnected() bool {
	if len(c.providers) == 0 {
		return false
	}
	return c.providers[0].IsConnected()
}

// Ping delegates to the primary provider, same rationale as IsConnected.
func (c *marketChain) Ping(ctx context.Context) error {
	if len(c.providers) == 0 {
		return fmt.Errorf("market_chain: Ping: %w", market.ErrNotConnected)
	}
	return c.providers[0].Ping(ctx)
}

func (c *marketChain) SearchInstrument(ctx context.Context, query string) ([]market.Instrument, error) {
	return chainCall(c, func(p market.ProviderAPI) ([]market.Instrument, error) {
		return p.SearchInstrument(ctx, query)
	})
}

func (c *marketChain) GetInstrument(ctx context.Context, symbol string) (market.Instrument, error) {
	return chainCall(c, func(p market.ProviderAPI) (market.Instrument, error) {
		return p.GetInstrument(ctx, symbol)
	})
}

func (c *marketChain) ListInstruments(ctx context.Context, assetType market.AssetType) ([]market.Instrument, error) {
	return chainCall(c, func(p market.ProviderAPI) ([]market.Instrument, error) {
		return p.ListInstruments(ctx, assetType)
	})
}

func (c *marketChain) GetCandles(ctx context.Context, symbol string, from, to time.Time, tf market.Timeframe) ([]market.Candle, error) {
	return chainCall(c, func(p market.ProviderAPI) ([]market.Candle, error) {
		return p.GetCandles(ctx, symbol, from, to, tf)
	})
}

func (c *marketChain) GetTicks(ctx context.Context, symbol string, from, to time.Time) ([]market.Tick, error) {
	return chainCall(c, func(p market.ProviderAPI) ([]market.Tick, error) {
		return p.GetTicks(ctx, symbol, from, to)
	})
}

func (c *marketChain) GetCorporateActions(ctx context.Context, symbol string, from, to time.Time) ([]market.CorporateAction, error) {
	return chainCall(c, func(p market.ProviderAPI) ([]market.CorporateAction, error) {
		return p.GetCorporateActions(ctx, symbol, from, to)
	})
}

func (c *marketChain) GetQuote(ctx context.Context, symbol string) (market.Quote, error) {
	return chainCall(c, func(p market.ProviderAPI) (market.Quote, error) {
		return p.GetQuote(ctx, symbol)
	})
}

func (c *marketChain) GetOrderBook(ctx context.Context, symbol string, depth int) (market.OrderBook, error) {
	return chainCall(c, func(p market.ProviderAPI) (market.OrderBook, error) {
		return p.GetOrderBook(ctx, symbol, depth)
	})
}

func (c *marketChain) SubscribeQuotes(ctx context.Context, symbol string) (<-chan market.Quote, error) {
	return chainCall(c, func(p market.ProviderAPI) (<-chan market.Quote, error) {
		return p.SubscribeQuotes(ctx, symbol)
	})
}

func (c *marketChain) SubscribeTrades(ctx context.Context, symbol string) (<-chan market.Tick, error) {
	return chainCall(c, func(p market.ProviderAPI) (<-chan market.Tick, error) {
		return p.SubscribeTrades(ctx, symbol)
	})
}

func (c *marketChain) SubscribeOrderBook(ctx context.Context, symbol string, depth int) (<-chan market.OrderBook, error) {
	return chainCall(c, func(p market.ProviderAPI) (<-chan market.OrderBook, error) {
		return p.SubscribeOrderBook(ctx, symbol, depth)
	})
}

func (c *marketChain) GetFundamentals(ctx context.Context, symbol string) (market.Fundamental, error) {
	return chainCall(c, func(p market.ProviderAPI) (market.Fundamental, error) {
		return p.GetFundamentals(ctx, symbol)
	})
}
