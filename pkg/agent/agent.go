package agent

import (
	"context"
	"log"

	"auris/pkg/llm"
	"auris/pkg/market"
	"auris/pkg/news"
)

const maxLoopIterations = 10

// ProgressKind identifies the category of tool being executed.
type ProgressKind string

const (
	ProgressFinancial   ProgressKind = "financial"
	ProgressCalculation ProgressKind = "calculation"
	ProgressNews        ProgressKind = "news"
	ProgressPortfolio   ProgressKind = "portfolio"
)

// ProgressEvent is sent on the progress channel before each tool execution.
// Consecutive events with the same Kind are deduplicated by the loop.
type ProgressEvent struct {
	Kind ProgressKind
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
	news               *news.Provider
	model              string
	tools              []llm.Tool
	progressCh         chan<- ProgressEvent
	chartCh            chan<- ChartEvent
	debugLogger        *log.Logger
	currentPortfolioID string
}

// New creates an Agent. model selects which model to use; empty string uses the
// provider's default. Optional functional options (e.g. [WithDebugLogger])
// configure additional behaviour.
func New(llmProvider llm.AIProvider, mp market.ProviderAPI, model string, opts ...Option) *Agent {
	a := &Agent{
		llm:    llmProvider,
		market: mp,
		model:  model,
		tools:  buildTools(),
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

// SetNewsProvider attaches a news provider used by the fetch_news tool.
// Pass nil to detach.
func (a *Agent) SetNewsProvider(p *news.Provider) {
	a.news = p
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
