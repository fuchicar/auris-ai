package anthropic_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"auris/pkg/drivers/anthropic"
	"auris/pkg/llm"
)

var _ llm.AIProvider = (*anthropic.Driver)(nil)
var _ llm.ContextWindowReporter = (*anthropic.Driver)(nil)

func TestInterfaceCompliance(t *testing.T) {
	var _ llm.AIProvider = (*anthropic.Driver)(nil)
}

// --- Unit tests (no network) ---

func TestName_NonEmpty(t *testing.T) {
	d := anthropic.New("key")
	if d.Name() == "" {
		t.Error("Name() returned empty string")
	}
}

func TestDescription_NonEmpty(t *testing.T) {
	d := anthropic.New("key")
	if d.Description() == "" {
		t.Error("Description() returned empty string")
	}
}

func TestIsConnected_InitiallyFalse(t *testing.T) {
	d := anthropic.New("key")
	if d.IsConnected() {
		t.Error("expected IsConnected() == false before Connect")
	}
}

func TestComplete_NotConnected(t *testing.T) {
	d := anthropic.New("key")
	_, err := d.Complete(context.Background(), llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if !errors.Is(err, llm.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

func TestStream_NotConnected(t *testing.T) {
	d := anthropic.New("key")
	_, err := d.Stream(context.Background(), llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if !errors.Is(err, llm.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

func TestListModels_NotConnected(t *testing.T) {
	d := anthropic.New("key")
	_, err := d.ListModels(context.Background())
	if !errors.Is(err, llm.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

func TestPing_NotConnected(t *testing.T) {
	d := anthropic.New("key")
	err := d.Ping(context.Background())
	if !errors.Is(err, llm.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

// --- Integration test helpers ---

func readAPIKey(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("test_data/anthropic_api_key")
	if err != nil {
		t.Skip("no API key in test_data/anthropic_api_key; skipping integration test")
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		t.Skip("empty API key in test_data/anthropic_api_key; skipping integration test")
	}
	return key
}

func skipIfRateLimited(t *testing.T, err error) {
	t.Helper()
	if errors.Is(err, llm.ErrRateLimit) {
		t.Skip("API rate limit reached; re-run after a moment")
	}
}

func newConnectedDriver(t *testing.T) *anthropic.Driver {
	t.Helper()
	d := anthropic.New(readAPIKey(t))
	if err := d.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Disconnect(context.Background()) })
	return d
}

// cheapestModel returns the cheapest model available, preferring haiku then sonnet.
func cheapestModel(t *testing.T, d *anthropic.Driver) llm.Model {
	t.Helper()
	models, err := d.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Skip("no models available; skipping integration test")
	}
	for _, pref := range []string{"haiku", "sonnet"} {
		for _, m := range models {
			if strings.Contains(strings.ToLower(m.ID), pref) {
				return m
			}
		}
	}
	return models[0]
}

// --- Integration tests ---

func TestConnect_IsConnected(t *testing.T) {
	d := newConnectedDriver(t)
	if !d.IsConnected() {
		t.Error("expected IsConnected() == true after Connect")
	}
}

func TestDisconnect(t *testing.T) {
	d := newConnectedDriver(t)
	if err := d.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if d.IsConnected() {
		t.Error("expected IsConnected() == false after Disconnect")
	}
}

func TestPing_Connected(t *testing.T) {
	d := newConnectedDriver(t)
	if err := d.Ping(context.Background()); err != nil {
		skipIfRateLimited(t, err)
		t.Fatalf("Ping: %v", err)
	}
}

func TestListModels(t *testing.T) {
	d := newConnectedDriver(t)
	models, err := d.ListModels(context.Background())
	if err != nil {
		skipIfRateLimited(t, err)
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}
	for _, m := range models {
		if m.ID == "" {
			t.Errorf("model has empty ID: %+v", m)
		}
	}
}

func TestComplete_CheapestModel(t *testing.T) {
	d := newConnectedDriver(t)
	model := cheapestModel(t, d)

	resp, err := d.Complete(context.Background(), llm.CompletionRequest{
		Model: model.ID,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Reply with a single word: hello"},
		},
	})
	if err != nil {
		skipIfRateLimited(t, err)
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Role != llm.RoleAssistant {
		t.Errorf("expected RoleAssistant, got %q", resp.Message.Role)
	}
	if resp.Message.Content == "" {
		t.Error("expected non-empty Content")
	}
}

func TestStream_CheapestModel(t *testing.T) {
	d := newConnectedDriver(t)
	model := cheapestModel(t, d)

	ch, err := d.Stream(context.Background(), llm.CompletionRequest{
		Model: model.ID,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Reply with a single word: hello"},
		},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var content strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			skipIfRateLimited(t, chunk.Err)
			t.Fatalf("stream error: %v", chunk.Err)
		}
		content.WriteString(chunk.Content)
		if chunk.Done {
			break
		}
	}
	if content.Len() == 0 {
		t.Error("expected non-empty streamed content")
	}
}

func TestStream_ContextCancellation(t *testing.T) {
	d := newConnectedDriver(t)
	model := cheapestModel(t, d)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := d.Stream(ctx, llm.CompletionRequest{
		Model: model.ID,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Count from 1 to 100, one number per line."},
		},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	// Cancel after the first chunk arrives.
	for range ch {
		cancel()
		break
	}

	// Verify the channel closes within a reasonable timeout.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch {
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("channel did not close within 5s after context cancellation")
	}
}

func TestComplete_ToolCall(t *testing.T) {
	d := newConnectedDriver(t)
	model := cheapestModel(t, d)

	tool := llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_weather",
			Description: "Get the current weather for a location.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{
						"type":        "string",
						"description": "City name",
					},
				},
				"required": []any{"location"},
			},
		},
	}

	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "What is the weather in Madrid?"},
	}

	// First turn: model should invoke the tool.
	resp, err := d.Complete(context.Background(), llm.CompletionRequest{
		Model:    model.ID,
		Messages: msgs,
		Tools:    []llm.Tool{tool},
	})
	if err != nil {
		skipIfRateLimited(t, err)
		t.Fatalf("Complete (first turn): %v", err)
	}
	if resp.StopReason != "tool_calls" {
		t.Skipf("model did not invoke tool (stop_reason=%q); skipping round-trip assertion", resp.StopReason)
	}
	if len(resp.Message.ToolCalls) == 0 {
		t.Fatal("expected at least one ToolCall")
	}

	tc := resp.Message.ToolCalls[0]
	if tc.ID == "" {
		t.Error("ToolCall.ID must not be empty")
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected tool name %q, got %q", "get_weather", tc.Function.Name)
	}

	// Append assistant message and tool result, then do the second turn.
	msgs = append(msgs, resp.Message)
	msgs = append(msgs, llm.Message{
		Role:       llm.RoleTool,
		Content:    `{"temperature": "22°C", "condition": "sunny"}`,
		ToolCallID: tc.ID,
	})

	resp2, err := d.Complete(context.Background(), llm.CompletionRequest{
		Model:    model.ID,
		Messages: msgs,
		Tools:    []llm.Tool{tool},
	})
	if err != nil {
		skipIfRateLimited(t, err)
		t.Fatalf("Complete (second turn): %v", err)
	}
	if resp2.StopReason == "tool_calls" {
		t.Error("expected final text response in second turn, got another tool call")
	}
	if resp2.Message.Content == "" {
		t.Error("expected non-empty Content in final response")
	}
}

func TestStream_ToolCall(t *testing.T) {
	d := newConnectedDriver(t)
	model := cheapestModel(t, d)

	tool := llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "get_weather",
			Description: "Get the current weather for a location.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{
						"type":        "string",
						"description": "City name",
					},
				},
				"required": []any{"location"},
			},
		},
	}

	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "What is the weather in Madrid?"},
	}

	// First turn: model should invoke the tool, streamed.
	ch, err := d.Stream(context.Background(), llm.CompletionRequest{
		Model:    model.ID,
		Messages: msgs,
		Tools:    []llm.Tool{tool},
	})
	if err != nil {
		t.Fatalf("Stream (first turn): %v", err)
	}

	var content strings.Builder
	var final llm.StreamChunk
	for chunk := range ch {
		if chunk.Err != nil {
			skipIfRateLimited(t, chunk.Err)
			t.Fatalf("stream error: %v", chunk.Err)
		}
		content.WriteString(chunk.Content)
		if chunk.Done {
			final = chunk
		}
	}

	if final.StopReason != "tool_calls" {
		t.Skipf("model did not invoke tool (stop_reason=%q); skipping round-trip assertion", final.StopReason)
	}
	if len(final.ToolCalls) == 0 {
		t.Fatal("expected at least one ToolCall on the terminal chunk")
	}

	tc := final.ToolCalls[0]
	if tc.ID == "" {
		t.Error("ToolCall.ID must not be empty")
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected tool name %q, got %q", "get_weather", tc.Function.Name)
	}

	// Reconstruct the assistant message the way runLoopStream would, then
	// replay it into a second turn — this is what actually exercises the
	// Extra["anthropic:content"] round trip for a streamed tool-call turn.
	assistantMsg := llm.Message{
		Role:      llm.RoleAssistant,
		Content:   content.String(),
		ToolCalls: final.ToolCalls,
		Extra:     final.Extra,
	}
	msgs = append(msgs, assistantMsg)
	msgs = append(msgs, llm.Message{
		Role:       llm.RoleTool,
		Content:    `{"temperature": "22°C", "condition": "sunny"}`,
		ToolCallID: tc.ID,
	})

	resp2, err := d.Complete(context.Background(), llm.CompletionRequest{
		Model:    model.ID,
		Messages: msgs,
		Tools:    []llm.Tool{tool},
	})
	if err != nil {
		skipIfRateLimited(t, err)
		t.Fatalf("Complete (second turn): %v", err)
	}
	if resp2.StopReason == "tool_calls" {
		t.Error("expected final text response in second turn, got another tool call")
	}
	if resp2.Message.Content == "" {
		t.Error("expected non-empty Content in final response")
	}
}
