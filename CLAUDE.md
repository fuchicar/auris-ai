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
