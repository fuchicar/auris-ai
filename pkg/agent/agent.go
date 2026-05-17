package agent

import (
	"context"

	"auris/pkg/llm"
	"auris/pkg/market"
)

const maxLoopIterations = 10

// Agent combines an LLM provider with a market data provider, exposing market
// operations as tools the model can call autonomously.
type Agent struct {
	llm    llm.AIProvider
	market market.ProviderAPI
	model  string
	tools  []llm.Tool
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

// Chat runs the agentic loop and returns the final assistant message.
func (a *Agent) Chat(ctx context.Context, messages []llm.Message) (llm.Message, error) {
	return runLoop(ctx, a, messages)
}
