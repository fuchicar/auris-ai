package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"auris/pkg/llm"
	"auris/pkg/market"
)

func TestChatStream_DirectAnswer(t *testing.T) {
	mlm := &mockLLM{streamResponses: [][]llm.StreamChunk{
		{
			{Content: "AAPL "},
			{Content: "is trading well."},
			{Done: true, StopReason: "stop"},
		},
	}}
	a := New(mlm, &mockMarket{}, "")

	var deltas []string
	msg, err := a.ChatStream(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "How is AAPL doing?"},
	}, func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "AAPL is trading well." {
		t.Errorf("unexpected content: %q", msg.Content)
	}
	if got := strings.Join(deltas, ""); got != "AAPL is trading well." {
		t.Errorf("onDelta did not receive the full text in order: %q", got)
	}
	if len(deltas) != 2 {
		t.Errorf("expected 2 onDelta calls, got %d: %v", len(deltas), deltas)
	}
}

func TestChatStream_OnDeltaNil(t *testing.T) {
	mlm := &mockLLM{streamResponses: [][]llm.StreamChunk{
		{
			{Content: "hello"},
			{Done: true, StopReason: "stop"},
		},
	}}
	a := New(mlm, &mockMarket{}, "")

	msg, err := a.ChatStream(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "hello" {
		t.Errorf("unexpected content: %q", msg.Content)
	}
}

func TestChatStream_SingleToolCall(t *testing.T) {
	quote := market.Quote{Last: 182.50}
	mp := &mockMarket{quote: quote}

	mlm := &mockLLM{streamResponses: [][]llm.StreamChunk{
		{
			{
				Done:       true,
				StopReason: "tool_calls",
				ToolCalls: []llm.ToolCall{{
					ID: "call_1",
					Function: llm.ToolCallFunction{
						Name:      "market_get_quote",
						Arguments: toolCallArgs(t, map[string]any{"symbol": "AAPL"}),
					},
				}},
			},
		},
		{
			{Content: "AAPL last: 182.50"},
			{Done: true, StopReason: "stop"},
		},
	}}

	a := New(mlm, mp, "")
	msg, err := a.ChatStream(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "What is the current price of AAPL?"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "AAPL last: 182.50" {
		t.Errorf("unexpected content: %q", msg.Content)
	}
	if mlm.streamCalls != 2 {
		t.Errorf("expected 2 stream calls, got %d", mlm.streamCalls)
	}
}

func TestChatStream_MaxIterations(t *testing.T) {
	endless := make([][]llm.StreamChunk, maxLoopIterations+2)
	for i := range endless {
		endless[i] = []llm.StreamChunk{
			{
				Done:       true,
				StopReason: "tool_calls",
				ToolCalls: []llm.ToolCall{{
					ID: "call_loop",
					Function: llm.ToolCallFunction{
						Name:      "market_get_quote",
						Arguments: toolCallArgs(t, map[string]any{"symbol": "X"}),
					},
				}},
			},
		}
	}

	a := New(&mockLLM{streamResponses: endless}, &mockMarket{}, "")
	_, err := a.ChatStream(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "loop"},
	}, nil)
	if err == nil {
		t.Error("expected error when max iterations exceeded")
	}
}

func TestChatStream_StreamError(t *testing.T) {
	mlm := &mockLLM{streamResponses: [][]llm.StreamChunk{
		{
			{Content: "partial"},
			{Done: true, Err: errors.New("boom")},
		},
	}}
	a := New(mlm, &mockMarket{}, "")

	_, err := a.ChatStream(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
	}, nil)
	if err == nil {
		t.Error("expected error to propagate from stream chunk")
	}
}
