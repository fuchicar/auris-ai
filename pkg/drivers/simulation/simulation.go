// Package simulation implements a fully synthetic market.ProviderAPI driver
// used by Auris's "simulation mode" (demo mode): a fixed, coherent, entirely
// fabricated universe of instruments so the agent can be evaluated with no
// real market-data API key. Prices are not random noise per symbol — every
// instrument shares a daily "market factor" that correlates their moves, the
// way a real market's constituents tend to move together (see generator.go).
//
// Unlike FMP/EODHD, this driver requires no credentials and no network
// access: Connect only loads the instrument universe (see data.go). Because
// everything is synthetic, the driver can fully implement order book and
// tick data too — market.CapabilityReporter.UnsupportedTools returns nil.
//
// Simulation mode is deliberately not registered in pkg/registry/market.go:
// it is not "one more provider" a user picks from a cascade, it is a
// config.AurisConfig.SimulationMode flag that replaces the whole market
// provider chain (see buildMarketProvider in pkg/tui/app.go) — see DD-7 in
// doc/adr.md.
package simulation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fuchicar/auris-ai/pkg/market"
)

const defaultPollInterval = 5 * time.Second

// Driver implements market.ProviderAPI with fully synthetic data.
type Driver struct {
	mu           sync.RWMutex
	connected    bool
	gen          *generator
	pollInterval time.Duration
}

// Option configures a Driver at construction time.
type Option func(*Driver)

// WithPollInterval overrides the polling interval used by the Subscribe*
// methods (default 5s). Mainly useful in tests.
func WithPollInterval(d time.Duration) Option {
	return func(drv *Driver) { drv.pollInterval = d }
}

// New creates a simulation Driver. Unlike real drivers it needs no API key —
// Connect loads the simulation universe file instead of authenticating.
func New(opts ...Option) *Driver {
	d := &Driver{pollInterval: defaultPollInterval}
	for _, o := range opts {
		o(d)
	}
	return d
}

// generator returns the loaded generator, or ErrNotConnected if Connect
// hasn't run yet.
func (d *Driver) generator() (*generator, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if !d.connected || d.gen == nil {
		return nil, fmt.Errorf("simulation: %w", market.ErrNotConnected)
	}
	return d.gen, nil
}

// findInstrument resolves a symbol against the loaded universe.
func findInstrument(gen *generator, symbol string) (instrumentDef, error) {
	inst, ok := gen.find(symbol)
	if !ok {
		return instrumentDef{}, fmt.Errorf("%w", market.ErrNotFound)
	}
	return inst, nil
}

// Name returns the short human-readable display name of the driver.
func (d *Driver) Name() string { return "Simulación" }

// DocsURL returns an empty string: simulation mode needs no signup/API docs.
func (d *Driver) DocsURL() string { return "" }

// Description returns a prose description of the driver.
func (d *Driver) Description() string {
	return "Datos de mercado simulados con fines de demostración — no son datos reales."
}

// Connect loads the simulation universe file (see data.go). It performs no
// network access.
func (d *Driver) Connect(_ context.Context) error {
	u, err := loadUniverse()
	if err != nil {
		return fmt.Errorf("simulation: Connect: %w", err)
	}
	epoch, err := parseEpoch(u.Epoch)
	if err != nil {
		return fmt.Errorf("simulation: Connect: %w", err)
	}
	d.mu.Lock()
	d.gen = newGenerator(u, epoch)
	d.connected = true
	d.mu.Unlock()
	return nil
}

// Disconnect marks the driver as disconnected. No resources to release.
func (d *Driver) Disconnect(_ context.Context) error {
	d.mu.Lock()
	d.connected = false
	d.mu.Unlock()
	return nil
}

// RefreshToken is a no-op: simulation mode has no credentials to refresh.
func (d *Driver) RefreshToken(_ context.Context) error { return nil }

// IsConnected reports the current connection state.
func (d *Driver) IsConnected() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.connected
}

// Ping reports whether the universe file has been loaded.
func (d *Driver) Ping(_ context.Context) error {
	_, err := d.generator()
	return err
}

// UnsupportedTools implements market.CapabilityReporter. Because all data is
// synthetic, this driver can fulfil every agent tool — including order book
// and ticks, which real drivers (FMP/EODHD) cannot — so nothing is filtered.
func (d *Driver) UnsupportedTools() []string { return nil }

// NewsSource returns a news.Source that generates synthetic headlines
// derived from this driver's simulated price moves (see news.go). Safe to
// call before Connect; HandleFetchNews resolves the generator lazily.
func (d *Driver) NewsSource() *NewsSource {
	return &NewsSource{driver: d}
}

func (d *Driver) SearchInstrument(_ context.Context, query string) ([]market.Instrument, error) {
	gen, err := d.generator()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]market.Instrument, 0)
	for _, inst := range gen.instruments {
		if q == "" || strings.Contains(strings.ToLower(inst.Symbol), q) || strings.Contains(strings.ToLower(inst.Name), q) {
			out = append(out, inst.toInstrument())
		}
	}
	return out, nil
}

func (d *Driver) GetInstrument(_ context.Context, symbol string) (market.Instrument, error) {
	gen, err := d.generator()
	if err != nil {
		return market.Instrument{}, err
	}
	inst, err := findInstrument(gen, symbol)
	if err != nil {
		return market.Instrument{}, fmt.Errorf("simulation: GetInstrument %q: %w", symbol, err)
	}
	return inst.toInstrument(), nil
}

func (d *Driver) ListInstruments(_ context.Context, assetType market.AssetType) ([]market.Instrument, error) {
	gen, err := d.generator()
	if err != nil {
		return nil, err
	}
	out := make([]market.Instrument, 0)
	for _, inst := range gen.instruments {
		if inst.assetType() == assetType {
			out = append(out, inst.toInstrument())
		}
	}
	return out, nil
}

func (d *Driver) GetCandles(_ context.Context, symbol string, from, to time.Time, tf market.Timeframe) ([]market.Candle, error) {
	gen, err := d.generator()
	if err != nil {
		return nil, err
	}
	inst, err := findInstrument(gen, symbol)
	if err != nil {
		return nil, fmt.Errorf("simulation: GetCandles %q: %w", symbol, err)
	}
	if tf == market.Timeframe1d {
		return gen.dailyCandles(inst, from, to), nil
	}
	candles, err := gen.intradayCandles(inst, from, to, tf)
	if err != nil {
		return nil, fmt.Errorf("simulation: GetCandles %q: %w", symbol, err)
	}
	return candles, nil
}

func (d *Driver) GetTicks(_ context.Context, symbol string, from, to time.Time) ([]market.Tick, error) {
	gen, err := d.generator()
	if err != nil {
		return nil, err
	}
	inst, err := findInstrument(gen, symbol)
	if err != nil {
		return nil, fmt.Errorf("simulation: GetTicks %q: %w", symbol, err)
	}
	return gen.ticks(inst, from, to), nil
}

func (d *Driver) GetCorporateActions(_ context.Context, symbol string, from, to time.Time) ([]market.CorporateAction, error) {
	gen, err := d.generator()
	if err != nil {
		return nil, err
	}
	inst, err := findInstrument(gen, symbol)
	if err != nil {
		return nil, fmt.Errorf("simulation: GetCorporateActions %q: %w", symbol, err)
	}
	return gen.corporateActions(inst, from, to), nil
}

func (d *Driver) GetQuote(_ context.Context, symbol string) (market.Quote, error) {
	gen, err := d.generator()
	if err != nil {
		return market.Quote{}, err
	}
	inst, err := findInstrument(gen, symbol)
	if err != nil {
		return market.Quote{}, fmt.Errorf("simulation: GetQuote %q: %w", symbol, err)
	}
	return gen.quote(inst, time.Now()), nil
}

func (d *Driver) GetOrderBook(_ context.Context, symbol string, depth int) (market.OrderBook, error) {
	gen, err := d.generator()
	if err != nil {
		return market.OrderBook{}, err
	}
	inst, err := findInstrument(gen, symbol)
	if err != nil {
		return market.OrderBook{}, fmt.Errorf("simulation: GetOrderBook %q: %w", symbol, err)
	}
	return gen.orderBook(inst, time.Now(), depth), nil
}

func (d *Driver) GetFundamentals(_ context.Context, symbol string) (market.Fundamental, error) {
	gen, err := d.generator()
	if err != nil {
		return market.Fundamental{}, err
	}
	inst, err := findInstrument(gen, symbol)
	if err != nil {
		return market.Fundamental{}, fmt.Errorf("simulation: GetFundamentals %q: %w", symbol, err)
	}
	return gen.fundamentals(inst, time.Now()), nil
}

// SubscribeQuotes emits a synthetic quote every pollInterval until ctx is
// cancelled — same goroutine/ticker/buffered-channel skeleton FMP and EODHD
// use for real polling, just generating data instead of calling an API.
func (d *Driver) SubscribeQuotes(ctx context.Context, symbol string) (<-chan market.Quote, error) {
	gen, err := d.generator()
	if err != nil {
		return nil, err
	}
	if _, err := findInstrument(gen, symbol); err != nil {
		return nil, fmt.Errorf("simulation: SubscribeQuotes %q: %w", symbol, err)
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
				default:
				}
			}
		}
	}()
	return ch, nil
}

// SubscribeTrades emits a synthetic trade print every pollInterval until ctx
// is cancelled.
func (d *Driver) SubscribeTrades(ctx context.Context, symbol string) (<-chan market.Tick, error) {
	gen, err := d.generator()
	if err != nil {
		return nil, err
	}
	inst, err := findInstrument(gen, symbol)
	if err != nil {
		return nil, fmt.Errorf("simulation: SubscribeTrades %q: %w", symbol, err)
	}
	ch := make(chan market.Tick, 1)
	go func() {
		defer close(ch)
		ticker := time.NewTicker(d.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				ticks := gen.ticks(inst, now.Add(-d.pollInterval), now)
				if len(ticks) == 0 {
					continue
				}
				select {
				case ch <- ticks[len(ticks)-1]:
				default:
				}
			}
		}
	}()
	return ch, nil
}

// SubscribeOrderBook emits a synthetic order book snapshot every
// pollInterval until ctx is cancelled.
func (d *Driver) SubscribeOrderBook(ctx context.Context, symbol string, depth int) (<-chan market.OrderBook, error) {
	gen, err := d.generator()
	if err != nil {
		return nil, err
	}
	inst, err := findInstrument(gen, symbol)
	if err != nil {
		return nil, fmt.Errorf("simulation: SubscribeOrderBook %q: %w", symbol, err)
	}
	ch := make(chan market.OrderBook, 1)
	go func() {
		defer close(ch)
		ticker := time.NewTicker(d.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ob := gen.orderBook(inst, time.Now(), depth)
				select {
				case ch <- ob:
				default:
				}
			}
		}
	}()
	return ch, nil
}
