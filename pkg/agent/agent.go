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

// Agent combines an LLM provider with a market data provider, exposing market
// operations as tools the model can call autonomously.
type Agent struct {
	llm                llm.AIProvider
	market             market.ProviderAPI
	news               *news.Provider
	model              string
	tools              []llm.Tool
	progressCh         chan<- ProgressEvent
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

// SetNewsProvider attaches a news provider used by the fetch_news tool.
// Pass nil to detach.
func (a *Agent) SetNewsProvider(p *news.Provider) {
	a.news = p
}

// Chat runs the agentic loop and returns the final assistant message.
func (a *Agent) Chat(ctx context.Context, messages []llm.Message) (llm.Message, error) {
	return runLoop(ctx, a, messages)
}
