package agent

import (
	"context"
	"log"
	"sync"

	"auris/pkg/llm"
	"auris/pkg/market"
	"auris/pkg/news"
)

const maxLoopIterations = 10

// ProgressKind classifies a tool call for the purpose of deciding whether it
// should surface a ProgressEvent at all (time_* tools, for instance, never
// do). It is also reused as the dedup key type in dispatch/dispatchInner,
// where it holds the full "name:args" signature of the last emitted call
// rather than a bare category — see the dedup logic in tools.go.
type ProgressKind string

const (
	ProgressFinancial   ProgressKind = "financial"
	ProgressCalculation ProgressKind = "calculation"
	ProgressNews        ProgressKind = "news"
	ProgressPortfolio   ProgressKind = "portfolio"
)

// ProgressEvent is sent on the progress channel before each tool execution,
// identifying the concrete call so the UI can render it (e.g.
// "market_get_quote(AAPL)"). Only exact repeats of the same call are
// deduplicated by the loop.
type ProgressEvent struct {
	Name string // tool name, e.g. "market_get_quote"
	Args string // raw JSON arguments exactly as sent by the model
}

// ChartEvent carries the candle data for a chart the model explicitly
// requested via market_render_price_chart. Unlike ProgressEvent, delivery is
// guaranteed (blocking send) and never deduplicated — every explicit chart
// request must reach the caller.
type ChartEvent struct {
	Symbol  string
	Candles []market.Candle
}

// Agent combines an LLM provider with a market data provider, exposing market
// operations as tools the model can call autonomously.
type Agent struct {
	llm                llm.AIProvider
	market             market.ProviderAPI
	news               news.Source
	model              string
	tools              []llm.Tool
	progressCh         chan<- ProgressEvent
	chartCh            chan<- ChartEvent
	debugLogger        *log.Logger
	currentPortfolioID string
	lastUsage          llm.TokenUsage
	// usageMu protects only lastUsage. It is written from the background
	// goroutine driving the ReAct loop (runLoop/runLoopStream) while read
	// concurrently from the TUI's Update goroutine (/info command) — see
	// BUG-9.
	usageMu sync.RWMutex
}

// New creates an Agent. model selects which model to use; empty string uses the
// provider's default. Optional functional options (e.g. [WithDebugLogger])
// configure additional behaviour.
func New(llmProvider llm.AIProvider, mp market.ProviderAPI, model string, opts ...Option) *Agent {
	tools := buildTools()
	if cr, ok := mp.(market.CapabilityReporter); ok {
		tools = filterTools(tools, cr.UnsupportedTools())
	}
	a := &Agent{
		llm:    llmProvider,
		market: mp,
		model:  model,
		tools:  tools,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// SetProgressCh attaches a channel that receives a ProgressEvent before each
// tool execution. Pass nil to detach.
func (a *Agent) SetProgressCh(ch chan<- ProgressEvent) {
	a.progressCh = ch
}

// SetChartCh attaches a channel that receives a ChartEvent whenever
// market_render_price_chart is dispatched successfully. Pass nil to detach.
func (a *Agent) SetChartCh(ch chan<- ChartEvent) {
	a.chartCh = ch
}

// SetNewsProvider attaches a news source used by the fetch_news tool.
// Pass nil to detach.
func (a *Agent) SetNewsProvider(p news.Source) {
	a.news = p
}

// LastUsage returns the token usage reported by the most recent LLM
// completion within this Agent's lifetime (the final ReAct iteration of the
// last Chat/ChatStream call). Zero value if no completion has happened yet.
func (a *Agent) LastUsage() llm.TokenUsage {
	a.usageMu.RLock()
	defer a.usageMu.RUnlock()
	return a.lastUsage
}

// setLastUsage records the token usage from the most recent LLM completion.
func (a *Agent) setLastUsage(u llm.TokenUsage) {
	a.usageMu.Lock()
	defer a.usageMu.Unlock()
	a.lastUsage = u
}

// Chat runs the agentic loop and returns the final assistant message.
func (a *Agent) Chat(ctx context.Context, messages []llm.Message) (llm.Message, error) {
	return runLoop(ctx, a, messages)
}

// ChatStream runs the agentic loop using the LLM's Stream API instead of
// Complete, invoking onDelta with each text fragment as it arrives (onDelta
// may be nil). It returns the same contract as Chat: the final assistant
// message once the ReAct loop reaches a non-tool-call stop, or an error.
func (a *Agent) ChatStream(ctx context.Context, messages []llm.Message, onDelta func(string)) (llm.Message, error) {
	return runLoopStream(ctx, a, messages, onDelta)
}
