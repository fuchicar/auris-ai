# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build ./...

# Vet
go vet ./...

# Run all tests (integration tests require API key in pkg/drivers/fmp/test_data/fmp_api_key)
go test ./... -timeout 120s

# Run a single test
go test ./pkg/drivers/fmp/... -run TestGetQuote_AAPL -v -timeout 30s

# Run only non-integration tests (no API key needed)
go test ./pkg/drivers/fmp/... -run TestInterfaceCompliance -v
```

## Architecture

The project is a financial AI advisor agent structured around a provider abstraction:

```
pkg/providers/     — Interface + shared types
pkg/drivers/fmp/   — Financial Modeling Prep implementation
cmd/auris/         — Entry point (stub)
```

### Provider abstraction (`pkg/providers/`)

- `providerapi.go` — `ProviderAPI` interface: all market data drivers must implement this. Covers auth lifecycle, instrument discovery, historical data, snapshots, streaming, and fundamentals.
- `instrument.go` — Shared value types: `Instrument`, `Candle`, `Tick`, `Quote`, `OrderBook`, `CorporateAction`, `Fundamental`, plus `AssetType` and `Timeframe` enums.
- `errors.go` — Sentinel errors: `ErrNotSupported`, `ErrNotFound`, `ErrUnauthorized`, `ErrRateLimit`, `ErrNotConnected`, `ErrSubscriptionRequired`. Drivers wrap these with `fmt.Errorf("driver: method: %w", providers.ErrXxx)` so callers use `errors.Is`.

### FMP driver (`pkg/drivers/fmp/fmp.go`)

Implements `ProviderAPI` against the FMP stable REST API (`https://financialmodelingprep.com/stable`). Key design notes:

- Auth is API-key based (query param `apikey=`). `Connect` validates by calling `/stable/profile?symbol=AAPL`. `Disconnect` is a local state change only. `RefreshToken` is a no-op.
- `SubscribeQuotes` is implemented via polling (default 5s, configurable with `WithPollInterval`). `SubscribeTrades` and `SubscribeOrderBook` return `ErrNotSupported`.
- `GetOrderBook` and `GetTicks` return `ErrNotSupported` — FMP does not expose these.
- `GetCorporateActions` calls `/stable/dividends` and `/stable/splits` sequentially, then filters by date client-side (the FMP API ignores `from`/`to` for these endpoints).
- Daily candles use `/stable/historical-price-eod/full` (returns flat array). Intraday candles use `/stable/historical-chart/{interval}`. Both return newest-first and are reversed before returning.
- `key-metrics-ttm` JSON: `marketCap` (no TTM suffix), `peRatioTTM`, `netIncomePerShareTTM`.
- HTTP 402 → `ErrSubscriptionRequired` (several endpoints require a paid plan).

### Adding a new driver

1. Create `pkg/drivers/<name>/` package.
2. Implement all methods of `providers.ProviderAPI`.
3. Wrap sentinel errors from `providers` with `fmt.Errorf`.
4. Add compile-time check in tests: `var _ providers.ProviderAPI = (*Driver)(nil)`.
5. Integration tests should `t.Skip` if credentials are absent.
