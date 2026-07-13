package agent

import (
	"context"
	"fmt"
	"strings"

	"auris/pkg/llm"
)

// runLoopStream mirrors runLoop but drives the ReAct loop via a.llm.Stream
// instead of Complete, forwarding text deltas to onDelta as they arrive.
func runLoopStream(ctx context.Context, a *Agent, messages []llm.Message, onDelta func(string)) (llm.Message, error) {
	msgs := make([]llm.Message, len(messages))
	copy(msgs, messages)

	var lastKind ProgressKind
	for iter := 1; iter <= maxLoopIterations; iter++ {
		a.debugf("[iter=%d] sending (stream): model=%s msgs=%d (%s) tools=[%s]",
			iter, a.model, len(msgs), summarizeRoles(msgs), summarizeToolNames(a.tools))
		if preview := lastUserOrToolPreview(msgs); preview != "" {
			a.debugf("[iter=%d] last_msg: %s", iter, preview)
		}

		ch, err := a.llm.Stream(ctx, llm.CompletionRequest{
			Model:    a.model,
			Messages: msgs,
			Tools:    a.tools,
		})
		if err != nil {
			a.debugf("[iter=%d] stream setup error: %v", iter, err)
			return llm.Message{}, fmt.Errorf("agent: llm: %w", err)
		}

		var text strings.Builder
		var final llm.StreamChunk
		for chunk := range ch {
			if chunk.Err != nil {
				a.debugf("[iter=%d] stream error: %v", iter, chunk.Err)
				return llm.Message{}, fmt.Errorf("agent: llm: %w", chunk.Err)
			}
			if chunk.Content != "" {
				text.WriteString(chunk.Content)
				if onDelta != nil {
					onDelta(chunk.Content)
				}
			}
			if chunk.Done {
				final = chunk
			}
		}
		a.lastUsage = final.Usage

		respMsg := llm.Message{
			Role:      llm.RoleAssistant,
			Content:   text.String(),
			ToolCalls: final.ToolCalls,
			Extra:     final.Extra,
		}
		stopReason := final.StopReason
		if stopReason == "" {
			stopReason = "stop"
		}

		a.debugf("[iter=%d] response (stream): stop=%s tool_calls=%d [%s] content=%q usage=p=%d c=%d",
			iter,
			stopReason,
			len(respMsg.ToolCalls),
			summarizeToolCalls(respMsg.ToolCalls),
			truncate(respMsg.Content, 300),
			final.Usage.PromptTokens,
			final.Usage.CompletionTokens,
		)

		msgs = append(msgs, respMsg)

		if stopReason != "tool_calls" {
			a.debugf("[loop] no tool calls, exiting (stream)")
			return respMsg, nil
		}

		for _, call := range respMsg.ToolCalls {
			result := a.dispatch(ctx, call, &lastKind)
			msgs = append(msgs, llm.Message{
				Role:       llm.RoleTool,
				Content:    result,
				ToolCallID: call.ID,
			})
		}
	}

	a.debugf("[loop] exceeded %d iterations (stream)", maxLoopIterations)
	return llm.Message{}, fmt.Errorf("agent: exceeded %d iterations without a final answer", maxLoopIterations)
}
