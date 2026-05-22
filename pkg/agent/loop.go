package agent

import (
	"context"
	"fmt"

	"auris/pkg/llm"
)

func runLoop(ctx context.Context, a *Agent, messages []llm.Message) (llm.Message, error) {
	msgs := make([]llm.Message, len(messages))
	copy(msgs, messages)

	var lastKind ProgressKind
	for range maxLoopIterations {
		resp, err := a.llm.Complete(ctx, llm.CompletionRequest{
			Model:    a.model,
			Messages: msgs,
			Tools:    a.tools,
		})
		if err != nil {
			return llm.Message{}, fmt.Errorf("agent: llm: %w", err)
		}

		msgs = append(msgs, resp.Message)

		if resp.StopReason != "tool_calls" {
			return resp.Message, nil
		}

		for _, call := range resp.Message.ToolCalls {
			result := a.dispatch(ctx, call, &lastKind)
			msgs = append(msgs, llm.Message{
				Role:       llm.RoleTool,
				Content:    result,
				ToolCallID: call.ID,
			})
		}
	}

	return llm.Message{}, fmt.Errorf("agent: exceeded %d iterations without a final answer", maxLoopIterations)
}
