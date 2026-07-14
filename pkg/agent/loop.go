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
	for iter := 1; iter <= maxLoopIterations; iter++ {
		a.debugf("[iter=%d] sending: model=%s msgs=%d (%s) tools=[%s]",
			iter, a.model, len(msgs), summarizeRoles(msgs), summarizeToolNames(a.tools))
		if preview := lastUserOrToolPreview(msgs); preview != "" {
			a.debugf("[iter=%d] last_msg: %s", iter, preview)
		}

		resp, err := a.llm.Complete(ctx, llm.CompletionRequest{
			Model:    a.model,
			Messages: msgs,
			Tools:    a.tools,
		})
		if err != nil {
			a.debugf("[iter=%d] error: %v", iter, err)
			return llm.Message{}, fmt.Errorf("agent: llm: %w", err)
		}
		a.setLastUsage(resp.Usage)

		a.debugf("[iter=%d] response: stop=%s tool_calls=%d [%s] content=%q usage=p=%d c=%d",
			iter,
			resp.StopReason,
			len(resp.Message.ToolCalls),
			summarizeToolCalls(resp.Message.ToolCalls),
			truncate(resp.Message.Content, 300),
			resp.Usage.PromptTokens,
			resp.Usage.CompletionTokens,
		)

		msgs = append(msgs, resp.Message)

		if resp.StopReason != "tool_calls" {
			a.debugf("[loop] no tool calls, exiting")
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

	a.debugf("[loop] exceeded %d iterations", maxLoopIterations)
	return llm.Message{}, fmt.Errorf("agent: exceeded %d iterations without a final answer", maxLoopIterations)
}
