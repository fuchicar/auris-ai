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
- Ollama LLM driver: requires a local Ollama instance running
- Tests `t.Skip` automatically when credentials are absent.

## Architecture

Auris is a terminal-based financial AI advisor. The two core abstractions are **market data** (`pkg/market/`) and **AI inference** (`pkg/llm/`). Both follow the same driver pattern — an interface defined in the abstraction package, implementations in `pkg/drivers/<name>/`, and a static registry in `pkg/registry/`.

```
pkg/market/          — ProviderAPI interface + shared market types + sentinel errors
pkg/llm/             — AIProvider interface + shared LLM types + sentinel errors
pkg/drivers/fmp/     — Market data: Financial Modeling Prep REST API
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

### AI abstraction (`pkg/llm/`)

- `provider.go` — `AIProvider` interface: `Connect`/`Disconnect`/`IsConnected`/`Ping`, `ListModels`, `Complete` (blocking), `Stream` (channel-based).
- `types.go` — `Message`, `Role`, `ToolCall`, `Tool`, `CompletionRequest/Response`, `StreamChunk`, `Model`, `TaskType`.
- `errors.go` — Sentinel errors: `ErrNotConnected`, `ErrUnauthorized`, `ErrModelNotFound`, `ErrContextTooLong`, `ErrRateLimit`, `ErrNotSupported`.

`Stream` always closes its channel after a `Done == true` chunk (even on cancellation) — callers can safely `range` over it.

### FMP driver (`pkg/drivers/fmp/fmp.go`)

Implements `market.ProviderAPI` against `https://financialmodelingprep.com/stable`. Key notes:

- Auth is API-key via query param `apikey=`. `Connect` validates by calling `/stable/profile?symbol=AAPL`. `Disconnect` is local state only. `RefreshToken` is a no-op.
- `SubscribeQuotes` polls (default 5s, configurable with `WithPollInterval`). `SubscribeTrades` and `SubscribeOrderBook` return `ErrNotSupported`.
- `GetOrderBook` and `GetTicks` return `ErrNotSupported`.
- `GetCorporateActions` calls `/stable/dividends` and `/stable/splits` sequentially, then filters by date client-side (FMP ignores `from`/`to` for these endpoints).
- Daily candles: `/stable/historical-price-eod/full` (flat array). Intraday: `/stable/historical-chart/{interval}`. Both return newest-first and are reversed before returning.
- `key-metrics-ttm` JSON fields: `marketCap` (no TTM suffix), `peRatioTTM`, `netIncomePerShareTTM`.
- HTTP 402 → `ErrSubscriptionRequired`.

### Ollama driver (`pkg/drivers/ollama/`)

Implements `llm.AIProvider` against the Ollama REST API (default `http://localhost:11434`). Supports `WithAPIKey` for protected remote instances. `Connect` validates by calling `GET /api/tags`.

### Gemini driver (`pkg/drivers/gemini/`)

Implements `llm.AIProvider` using `google.golang.org/genai`. `Connect` validates the API key by listing models.

### Registry (`pkg/registry/`)

- `market.go` — `AllMarket() []MarketEntry` — ordered list of market providers; currently only FMP.
- `llm.go` — `AllLLM() []LLMEntry` — ordered list of LLM providers; currently Ollama and Gemini.
- Each entry carries a `Key` (stable config identifier), `DisplayName`, and a `New` factory function.

### Agent (`pkg/agent/`)

`Agent` combines an `llm.AIProvider` and a `market.ProviderAPI`, exposing market operations as tools the LLM can call. Created with `agent.New(llmProvider, marketProvider, model)`.

- `loop.go` — `runLoop` drives the ReAct loop up to `maxLoopIterations` (10). Exits when `StopReason != "tool_calls"`.
- `tools.go` — `buildTools()` declares all tool schemas; `dispatch()` routes tool calls to market methods or built-in time tools (`time_now`, `time_today`, `time_yesterday`).
- `math.go` — `calc*` functions backing the calculation tools (Sharpe, VaR, DCF, technical indicators, Monte Carlo, etc.).
- `validate.go` — Shared numeric-input validation helpers used by every `calc*` function (see "Validating numeric tool inputs" below).
- `prompts.go` — System prompt construction.

### TUI (`pkg/tui/`)

Built with Bubble Tea. `AppModel` is the root model; it owns the active screen and shared session state.

**Screen flow** (controlled by the `transition` method via `ScreenDoneMsg`):

- **First run / setup**: Welcome → Disclaimer → (Locale?) → Theme → Passphrase → Profile → Provider → APIKey → AIProviderSelect → AIProviderConfig(×N) → AIDefaultModel → Menu
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

### Adding a new LLM driver

1. Create `pkg/drivers/<name>/` package.
2. Implement all methods of `llm.AIProvider`.
3. Wrap sentinel errors from `llm` with `fmt.Errorf`.
4. Add compile-time check: `var _ llm.AIProvider = (*Driver)(nil)`.
5. Register in `pkg/registry/llm.go`.
6. Integration tests must `t.Skip` if credentials are absent.

### Adding or removing agent tools

When `buildTools()` in `pkg/agent/tools.go` is modified, update the hardcoded count in `TestBuildTools_Count` (`pkg/agent/agent_test.go`) to match the new total.

### Validating numeric tool inputs (`pkg/agent/validate.go`)

Every numeric parameter of a `calc*` function in `pkg/agent/math.go` must be validated with the matching helper from `pkg/agent/validate.go`, as the first statements of the function, before any computation. Validation lives in `calc*` itself (not in `tools.go` dispatch) so direct callers and tests get the same guarantee.

Pick the helper by the parameter's real-world domain — don't inline ad hoc `math.IsNaN`/range checks in `math.go` or `tools.go`; add a new helper to `validate.go` if none of the existing ones fit:

- **Must-be-positive monetary value** (a price, `last_price`, `portfolio_value`, `principal`, `pe_ratio`...) → `validatePositive`/`validatePositiveAll`. All `prices []float64` series (used by volatility, max drawdown, and every technical indicator) fall in this category.
- **Legitimately zero but never negative** (a dividend, `shares_outstanding` as an "unset" sentinel, `final_value` in a growth calc) → `validateNonNegative`/`validateNonNegativeAll`.
- **Periodic/cumulative return** expressed as a decimal fraction (e.g. 0.05 = 5%), such as a `returns[]` series → `validateReturn`/`validateReturnSlice`. Bounded to a deliberately generous `[-100%, +10,000%]` — a legitimate 10x–100x move must never be rejected. The percent-unit equivalent (e.g. `shocks_percent`) is `validateReturnPercent`/`validateReturnPercentSlice`.
- **Annualised rate** (discount rate, risk-free rate, an interest/growth-rate assumption) → `validateRate`. Bounded tighter, to `[-100%, +1,000%]`, since no real-world annualised rate assumption approaches that even in extreme scenarios.
- **Domain-agnostic value that can legitimately be negative** (EPS, EBITDA, book value, free cash flow, a currency amount) → `validateFinite`/`validateFiniteAll` only — reject NaN/±Inf, but don't impose a sign or range bound the domain doesn't guarantee.

When adding a new `calc*` function, its dispatch `case` in `tools.go` needs no additional validation — the function itself is the enforcement point.

### Keeping the TFM presentation in sync (`doc/presentacion.html`)

`doc/presentacion.html` is the TFM defense deck. It quotes real numbers from this codebase, so when a change alters one of them, update **only the affected figure(s)** in the HTML — edit the text in place; do not restructure slides, rewrite copy, rename CSS classes, or touch the styling/layout/JS. The deck's look and feel deliberately mirrors the TUI dark theme and must stay intact.

Code-derived figures the deck quotes (and where the truth lives):

- **56 tools** and the per-category counts (21 calculation, 15 portfolio, 9 market, 6 technical indicators, 3 time, 1 news, 1 currency) → `buildTools()` in `pkg/agent/tools.go` / `TestBuildTools_Count`. If you change the tool count, also rescale the category bar widths (`.toolcat .bar i`, sized relative to the largest category).
- **468 test functions / 261 in pkg/agent** → recount with `grep -rn "func Test" --include="*_test.go" | wc -l`.
- **4 AI drivers (Anthropic · Gemini · Ollama · MiniMax) and 1 market driver (FMP)** → `pkg/registry/`. A new driver changes slides 5, 6 and possibly 3/15.
- **~23,500 LOC · 14 packages · Go 1.25 · 24 TUI screens · 2 locales · 6 themes** → recount when they drift meaningfully.
- **≤ 10 ReAct iterations · 3 TaskTypes** → `maxLoopIterations` in `pkg/agent/agent.go`, `llm.TaskType`.
- **Monte Carlo up to 100,000 paths** → `pkg/agent/math.go`.
- **Argon2id (64 MiB, 16 B salt) + AES-256-GCM (12 B nonce)** → `pkg/config/crypto.go`.
- **FIFO lots · 6 transaction types · HHI · rebalancing** → `pkg/portfolio/`.

Roadmap items on slide 14 (chat streaming, alerts, more asset classes, broker drivers) are described as *future* work — if one ships, move it from roadmap phrasing to a factual claim instead of leaving it stale.
