package agent

import (
	"context"

	"auris/pkg/llm"
	"auris/pkg/market"
)

const maxLoopIterations = 10

// ProgressKind identifies the category of tool being executed.
type ProgressKind string

const (
	ProgressFinancial   ProgressKind = "financial"
	ProgressCalculation ProgressKind = "calculation"
)

// ProgressEvent is sent on the progress channel before each tool execution.
// Consecutive events with the same Kind are deduplicated by the loop.
type ProgressEvent struct {
	Kind ProgressKind
}

// Agent combines an LLM provider with a market data provider, exposing market
// operations as tools the model can call autonomously.
type Agent struct {
	llm        llm.AIProvider
	market     market.ProviderAPI
	model      string
	tools      []llm.Tool
	progressCh chan<- ProgressEvent
}

// New creates an Agent. model selects which model to use; empty string uses the
// provider's default.
func New(llmProvider llm.AIProvider, mp market.ProviderAPI, model string) *Agent {
	return &Agent{
		llm:    llmProvider,
		market: mp,
		model:  model,
		tools:  buildTools(),
	}
}

// SetProgressCh attaches a channel that receives a ProgressEvent before each
// tool execution. Pass nil to detach.
func (a *Agent) SetProgressCh(ch chan<- ProgressEvent) {
	a.progressCh = ch
}

// Chat runs the agentic loop and returns the final assistant message.
func (a *Agent) Chat(ctx context.Context, messages []llm.Message) (llm.Message, error) {
	return runLoop(ctx, a, messages)
}
