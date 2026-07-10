# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build ./...

# Vet
go vet ./...

# Run all tests (integration tests require API keys — see below)
go test ./... -timeout 120s

# Run a single test
go test ./pkg/drivers/fmp/... -run TestGetQuote_AAPL -v -timeout 30s

# Run only non-integration tests
go test ./pkg/drivers/fmp/... -run TestInterfaceCompliance -v
```

Integration test credentials:
- FMP market driver: `pkg/drivers/fmp/test_data/fmp_api_key`
- EODHD market driver: `pkg/drivers/eodhd/test_data/eodhd_api_key`
- Ollama LLM driver: requires a local Ollama instance running
- Tests `t.Skip` automatically when credentials are absent.

## Architecture

Auris is a terminal-based financial AI advisor. The two core abstractions are **market data** (`pkg/market/`) and **AI inference** (`pkg/llm/`). Both follow the same driver pattern — an interface defined in the abstraction package, implementations in `pkg/drivers/<name>/`, and a static registry in `pkg/registry/`.

```
pkg/market/          — ProviderAPI interface + shared market types + sentinel errors
pkg/llm/             — AIProvider interface + shared LLM types + sentinel errors
pkg/drivers/fmp/     — Market data: Financial Modeling Prep REST API
pkg/drivers/eodhd/   — Market data: EOD Historical Data (EODHD) REST API
pkg/drivers/ollama/  — LLM: Ollama local inference REST API
pkg/drivers/gemini/  — LLM: Google Gemini via google.golang.org/genai SDK
pkg/registry/        — Static registration of all market and LLM providers
pkg/agent/           — Agentic loop (LLM + tool dispatch over market provider)
pkg/tui/             — BubbleTea terminal UI (setup wizard + agent chat)
pkg/config/          — Encrypted config (Argon2id + AES-GCM) and session files
pkg/locale/          — i18n via go-i18n (en/es)
cmd/auris/           — Entry point
```

### Market abstraction (`pkg/market/`)

- `providerapi.go` — `ProviderAPI` interface: auth lifecycle, instrument discovery, historical data, snapshots, streaming, and fundamentals.
- `instrument.go` — Shared value types: `Instrument`, `Candle`, `Tick`, `Quote`, `OrderBook`, `CorporateAction`, `Fundamental`, plus `AssetType` and `Timeframe` enums.
- `errors.go` — Sentinel errors: `ErrNotSupported`, `ErrNotFound`, `ErrUnauthorized`, `ErrRateLimit`, `ErrNotConnected`, `ErrSubscriptionRequired`. Drivers wrap with `fmt.Errorf("driver: method: %w", market.ErrXxx)`.
- `capability.go` — `CapabilityReporter` (optional interface, `UnsupportedTools() []string`) lets a driver statically declare agent tool names it can never fulfil, plus the `ToolGetOrderBook`/`ToolGetTicks` name constants both `pkg/agent/tools.go` and the drivers reference (single source of truth for the string). See "Capability filtering" under Agent below.

### AI abstraction (`pkg/llm/`)

- `provider.go` — `AIProvider` interface: `Connect`/`Disconnect`/`IsConnected`/`Ping`, `ListModels`, `Complete` (blocking), `Stream` (channel-based).
- `types.go` — `Message`, `Role`, `ToolCall`, `Tool`, `CompletionRequest/Response`, `StreamChunk`, `Model`, `TaskType`.
- `errors.go` — Sentinel errors: `ErrNotConnected`, `ErrUnauthorized`, `ErrModelNotFound`, `ErrContextTooLong`, `ErrRateLimit`, `ErrNotSupported`.

`Stream` always closes its channel after a `Done == true` chunk (even on cancellation) — callers can safely `range` over it. `StreamChunk.ToolCalls`/`StopReason`/`Extra` mirror `CompletionResponse`'s fields and are populated only on that terminal chunk, so a caller can reconstruct the exact same `llm.Message` from a streamed call as it would get from `Complete`.

### FMP driver (`pkg/drivers/fmp/fmp.go`)

Implements `market.ProviderAPI` against `https://financialmodelingprep.com/stable`. Key notes:

- Auth is API-key via query param `apikey=`. `Connect` validates by calling `/stable/profile?symbol=AAPL`. `Disconnect` is local state only. `RefreshToken` is a no-op.
- `SubscribeQuotes` polls (default 5s, configurable with `WithPollInterval`). `SubscribeTrades` and `SubscribeOrderBook` return `ErrNotSupported`.
- `GetOrderBook` and `GetTicks` return `ErrNotSupported`.
- `GetCorporateActions` calls `/stable/dividends` and `/stable/splits` sequentially, then filters by date client-side (FMP ignores `from`/`to` for these endpoints).
- Daily candles: `/stable/historical-price-eod/full` (flat array). Intraday: `/stable/historical-chart/{interval}`. Both return newest-first and are reversed before returning.
- `key-metrics-ttm` JSON fields: `marketCap` (no TTM suffix), `peRatioTTM`, `netIncomePerShareTTM`.
- HTTP 402 → `ErrSubscriptionRequired`.

### EODHD driver (`pkg/drivers/eodhd/eodhd.go`)

Implements `market.ProviderAPI` against `https://eodhd.com/api`. Selected as the second market provider (FEAT-7) for its BME (Madrid) coverage, which FMP's free tier lacks. Key notes:

- Auth is API-key via query param `api_token=`; `fmt=json` is also forced on every request (some endpoints default to CSV without it). The symbol/query is part of the URL **path** (e.g. `/eod/AAPL.US`), not a query param — unlike FMP.
- Symbols carry an explicit exchange suffix (`BKT.MC` for Madrid); a bare symbol (`AAPL`) implicitly resolves to `.US`.
- HTTP 401 → `ErrUnauthorized`. HTTP 403 → `ErrSubscriptionRequired` (EODHD's free tier returns this for `/fundamentals` and `/intraday` — "Only EOD data allowed for free users" — a plan restriction, not an auth failure). HTTP 404 → `ErrNotFound`. HTTP 429 → `ErrRateLimit`.
- `GetQuote` (`/real-time/{symbol}`) returns **HTTP 200** with numeric fields as the string `"NA"` for an unknown symbol, instead of HTTP 404 — the driver decodes those fields as `any` and detects `"NA"` explicitly, translating it to `ErrNotFound`.
- Daily candles (`/eod/{symbol}`, `period=d&order=a`) come back ascending — no reversal needed, unlike FMP. Intraday (`/intraday/{symbol}`) requires `from`/`to` as **unix timestamps**, not ISO dates (passing a date string returns HTTP 422 before the plan-tier 403 even applies); always `ErrSubscriptionRequired` in practice on the free tier.
- `SearchInstrument`/`GetInstrument` use `/search/{query}` (covers every exchange, including BME); `ListInstruments` uses `/exchange-symbol-list/US`. No hardcoded ticker list is needed — verified live that `/api/exchanges/{MIC}` (e.g. `XMAD`) 404s on the free tier, but `/api/exchange-symbol-list/{code}` (e.g. `MC`) and `/api/search/` both work.
- `GetTicks`, `GetOrderBook`, `SubscribeTrades`, `SubscribeOrderBook` → `ErrNotSupported` (no tick/order-book data in EODHD's REST API).

### Ollama driver (`pkg/drivers/ollama/`)

Implements `llm.AIProvider` against the Ollama REST API (default `http://localhost:11434`). Supports `WithAPIKey` for protected remote instances. `Connect` validates by calling `GET /api/tags`.

### Gemini driver (`pkg/drivers/gemini/`)

Implements `llm.AIProvider` using `google.golang.org/genai`. `Connect` validates the API key by listing models.

### Registry (`pkg/registry/`)

- `market.go` — `AllMarket() []MarketEntry` — ordered list of market providers: FMP, then EODHD. This order defines the fallback priority of the market chain built by `agent.NewMarketChain` (FMP primary, EODHD secondary — see FEAT-7 in `doc/task_completed.md`, DD-5 in `doc/adr.md`).
- `llm.go` — `AllLLM() []LLMEntry` — ordered list of LLM providers; currently Ollama and Gemini.
- Each entry carries a `Key` (stable config identifier), `DisplayName`, and a `New` factory function.

### Finance (`pkg/finance/`)

Pure numeric finance/valuation/risk/indicator functions with no dependency on `Agent`, `a.market`, or any other agent/market-only symbol. Used both by the agent's calculation tools (`pkg/agent/tools.go` dispatch, via `finance.CalcXxx(...)`) and directly by the TUI (`pkg/tui`), so the TUI can compute metrics/indicators without going through the LLM (see REF-7 in `doc/task_completed.md`).

- `types.go` — `Float`/`FloatSlice`, JSON wrapper types that marshal NaN/±Inf as `null` (used by indicator series with a warm-up period).
- `util.go` — `Round2`/`Round4` (also used directly by `pkg/agent/tools.go`'s portfolio dispatch code, not just by `Calc*` functions) plus unexported numeric helpers (`meanFloat`, `sampleStddev`, `percentileInterp`).
- `validate.go` — Shared numeric-input validation helpers (see "Validating numeric tool inputs" below).
- `valuation.go` — `CalcROI`, `CalcCAGR`, `CalcPnL`, `CalcDCF`, `CalcMultiples`, `CalcPFCF`, `CalcPEG`, `CalcDividendYield`, `CalcDividendGrowth`.
- `risk.go` — `CalcVolatility`, `CalcSharpe`, `CalcSortino`, `CalcMaxDrawdown`, `CalcBeta`, `CalcTreynor`, `CalcInformationRatio`, `CalcVaR`.
- `indicators.go` — `CalcSMA` (calls `SMA`), `CalcEMA`, `CalcRSI`, `CalcMACD`, `CalcBollingerBands`, `CalcCorrelationMatrix`.
- `sma.go` — `SMA`, the low-level moving-average primitive `CalcSMA` and `pkg/tui/chart.go` both call directly.
- `simulation.go` — `CalcStressTest`, `CalcMonteCarloSimulation`, `CalcCurrencyConversion`, `CalcCompoundInterest`.
- `stats.go` — `CalcStats`.

### Agent (`pkg/agent/`)

`Agent` combines an `llm.AIProvider` and a `market.ProviderAPI`, exposing market operations as tools the LLM can call. Created with `agent.New(llmProvider, marketProvider, model)`.

- `market_chain.go` — `NewMarketChain(providers ...market.ProviderAPI) market.ProviderAPI` wraps an ordered list of providers (`providers[0]` primary) into a single `market.ProviderAPI`. Each call is tried against providers in order; it falls back to the next only on `ErrNotFound`/`ErrNotSupported`/`ErrRateLimit`/`ErrSubscriptionRequired` — other errors (`ErrUnauthorized`, `ErrNotConnected`, ...) surface immediately rather than being masked by a fallback. If none resolve the call, the **primary's** error is returned (never the last-tried provider's). `pkg/tui/app.go`'s `buildMarketProvider` constructs this chain from every market provider the user configured, in `registry.AllMarket()` order (see FEAT-7 in `doc/task_completed.md`).
- `loop.go` — `runLoop` drives the ReAct loop up to `maxLoopIterations` (10). Exits when `StopReason != "tool_calls"`.
- `tools.go` — `buildTools()` declares all tool schemas (always the full set, provider-independent); `filterTools()` removes tools by name; `dispatch()` routes tool calls to `finance.CalcXxx` functions, market methods, or built-in time tools (`time_now`, `time_today`, `time_yesterday`).
- **Capability filtering** (FEAT-8): `agent.New` calls `buildTools()` and then, if the `market.ProviderAPI` passed in also implements the optional `market.CapabilityReporter` interface (`UnsupportedTools() []string`), filters out any tool named in that list before building `Agent.tools` — e.g. `market_get_order_book`/`market_get_ticks`, which both FMP and EODHD always return `ErrNotSupported` for, so the LLM never burns a `maxLoopIterations` iteration calling something that can't work. `fmp.Driver`/`eodhd.Driver` implement it with a static slice; `marketChain` implements it as the **intersection** of its providers' declared-unsupported sets (a tool is excluded only if *every* provider in the chain lacks it) and returns `nil` (nothing filtered) if any provider in the chain doesn't implement `CapabilityReporter` at all — capability-unknown is the conservative default, never over-filtering.
- `benchmark.go` — `buildBenchmarkComparison` assembles the `portfolio_compare_benchmark` result (alpha, beta via `finance.CalcBeta`) from numbers the dispatch case in `tools.go` already fetched (candles/quotes, start-of-period holdings).
- `prompts.go` — System prompt construction.

### TUI (`pkg/tui/`)

Built with Bubble Tea. `AppModel` is the root model; it owns the active screen and shared session state.

**Screen flow** (controlled by the `transition` method via `ScreenDoneMsg`):

- **First run / setup**: Welcome → Disclaimer → (Locale?) → Theme → Passphrase → Profile → Provider → APIKey → (APIKeySecondary, optional — any other registered market provider not yet configured) → AIProviderSelect → AIProviderConfig(×N) → AIDefaultModel → Menu
- **Returning user**: Welcome → Unlock → Menu
- **From menu**: Menu ↔ Agent (toggle with Shift+Tab); settings screens return to Menu via `FlowMenu` context.

Each screen emits a typed `ScreenDoneMsg.Result` (e.g. `UnlockResult`, `APIKeyResult`). `AppModel.transition` switches on `msg.From` to advance state. Setup screens are centred with `lipgloss.Place`; the agent screen uses the full terminal.

### Config (`pkg/config/`)

- The config file lives at the platform-standard OS config path (see `path.go`).
- **Encryption**: API keys are encrypted at rest with AES-256-GCM. The key is derived from the user's passphrase via Argon2id (`golang.org/x/crypto`). A new random salt is generated on every `Save`.
- `FinancialProfile` and chat sessions are stored as plaintext JSON (not credentials).
- **Sessions**: each chat session is a separate JSON file in a `sessions/` subdirectory alongside the config file. `config.NewSession()` / `SaveSession` / `LoadSession` / `ListSessions` manage them. `AurisConfig.ChatHistory` is deprecated (legacy migration path).

### Adding a new market driver

1. Create `pkg/drivers/<name>/` package.
2. Implement all methods of `market.ProviderAPI`.
3. Wrap sentinel errors: `fmt.Errorf("fmp: GetQuote: %w", market.ErrNotFound)`.
4. Add compile-time check in tests: `var _ market.ProviderAPI = (*Driver)(nil)`.
5. Register in `pkg/registry/market.go`.
6. Integration tests must `t.Skip` if credentials are absent.
7. If any method **always** returns `ErrNotSupported` regardless of symbol or account tier (not a plan/subscription restriction — see `market.ErrSubscriptionRequired` for that case), implement `market.CapabilityReporter.UnsupportedTools() []string` returning the corresponding agent tool name(s) (e.g. `market.ToolGetOrderBook`), so `agent.New` excludes them from the LLM's tool list instead of letting it burn a loop iteration on a call that can never succeed (see FEAT-8 in `doc/task_completed.md`).

### Adding a new LLM driver

1. Create `pkg/drivers/<name>/` package.
2. Implement all methods of `llm.AIProvider`.
3. Wrap sentinel errors from `llm` with `fmt.Errorf`.
4. Add compile-time check: `var _ llm.AIProvider = (*Driver)(nil)`.
5. Register in `pkg/registry/llm.go`.
6. Integration tests must `t.Skip` if credentials are absent.
7. If the provider supports tool calling, `Stream` must populate `StreamChunk.ToolCalls`/`StopReason`/`Extra` on the terminal chunk exactly as `Complete` populates `CompletionResponse`/`Message.Extra` — reuse the same mapping helper in both code paths rather than re-deriving the parsing (see `mapResponse` in the anthropic/gemini drivers, or the SDK's own accumulator where available, e.g. `sdk.Message.Accumulate` for Anthropic).

### Adding or removing agent tools

When `buildTools()` in `pkg/agent/tools.go` is modified, update the hardcoded count in `TestBuildTools_Count` (`pkg/agent/agent_test.go`) to match the new total.

### Validating numeric tool inputs (`pkg/finance/validate.go`)

Every numeric parameter of a `Calc*` function in `pkg/finance/` must be validated with the matching helper from `pkg/finance/validate.go`, as the first statements of the function, before any computation. Validation lives in `Calc*` itself (not in `tools.go` dispatch) so direct callers and tests get the same guarantee.

Pick the helper by the parameter's real-world domain — don't inline ad hoc `math.IsNaN`/range checks in `pkg/finance` or `tools.go`; add a new helper to `validate.go` if none of the existing ones fit:

- **Must-be-positive monetary value** (a price, `last_price`, `portfolio_value`, `principal`, `pe_ratio`...) → `ValidatePositive`/`ValidatePositiveAll`. All `prices []float64` series (used by volatility, max drawdown, and every technical indicator) fall in this category.
- **Legitimately zero but never negative** (a dividend, `shares_outstanding` as an "unset" sentinel, `final_value` in a growth calc) → `ValidateNonNegative`/`ValidateNonNegativeAll`.
- **Periodic/cumulative return** expressed as a decimal fraction (e.g. 0.05 = 5%), such as a `returns[]` series → `ValidateReturn`/`ValidateReturnSlice`. Bounded to a deliberately generous `[-100%, +10,000%]` — a legitimate 10x–100x move must never be rejected. The percent-unit equivalent (e.g. `shocks_percent`) is `ValidateReturnPercent`/`ValidateReturnPercentSlice`.
- **Annualised rate** (discount rate, risk-free rate, an interest/growth-rate assumption) → `ValidateRate`. Bounded tighter, to `[-100%, +1,000%]`, since no real-world annualised rate assumption approaches that even in extreme scenarios.
- **Domain-agnostic value that can legitimately be negative** (EPS, EBITDA, book value, free cash flow, a currency amount) → `ValidateFinite`/`ValidateFiniteAll` only — reject NaN/±Inf, but don't impose a sign or range bound the domain doesn't guarantee.

When adding a new `Calc*` function, its dispatch `case` in `tools.go` needs no additional validation — the function itself is the enforcement point.

### Keeping the TFM presentation in sync (`doc/presentacion.html`)

`doc/presentacion.html` is the TFM defense deck. It quotes real numbers from this codebase, so when a change alters one of them, update **only the affected figure(s)** in the HTML — edit the text in place; do not restructure slides, rewrite copy, rename CSS classes, or touch the styling/layout/JS. The deck's look and feel deliberately mirrors the TUI dark theme and must stay intact.

Code-derived figures the deck quotes (and where the truth lives):

- **57 tools** and the per-category counts (21 calculation, 16 portfolio, 9 market, 6 technical indicators, 3 time, 1 news, 1 currency) → `buildTools()` in `pkg/agent/tools.go` / `TestBuildTools_Count`. If you change the tool count, also rescale the category bar widths (`.toolcat .bar i`, sized relative to the largest category).
- **594 test functions / 171 in pkg/finance** → recount with `grep -rn "func Test" --include="*_test.go" | wc -l`. The calculation-engine tests live in `pkg/finance` since REF-7 (previously counted under `pkg/agent`).
- **4 AI drivers (Anthropic · Gemini · Ollama · MiniMax) and 2 market drivers (FMP · EODHD, cascade with fallback)** → `pkg/registry/`. A new driver changes slides 5, 6 and possibly 3/15.
- **~23,500 LOC · 14 packages · Go 1.25 · 24 TUI screens · 2 locales · 6 themes** → recount when they drift meaningfully.
- **≤ 10 ReAct iterations · 3 TaskTypes** → `maxLoopIterations` in `pkg/agent/agent.go`, `llm.TaskType`.
- **Monte Carlo up to 100,000 paths** → `pkg/finance/simulation.go`.
- **Argon2id (64 MiB, 16 B salt) + AES-256-GCM (12 B nonce)** → `pkg/config/crypto.go`.
- **FIFO lots · 6 transaction types · HHI · rebalancing** → `pkg/portfolio/`.

Roadmap items on slide 14 (chat streaming, alerts, more asset classes, broker drivers) are described as *future* work — if one ships, move it from roadmap phrasing to a factual claim instead of leaving it stale.
