package gemini_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"auris/pkg/drivers/gemini"
	"auris/pkg/llm"
)

// skipIfRateLimited calls t.Skip when the error is ErrRateLimit.
// The free Gemini tier has strict RPM limits that integration tests can hit
// when several tests run back-to-back.
func skipIfRateLimited(t *testing.T, err error) {
	t.Helper()
	if errors.Is(err, llm.ErrRateLimit) {
		t.Skip("API rate limit reached; skipping (re-run after a minute)")
	}
}

// ── Compile-time interface compliance ────────────────────────────────────────

func TestInterfaceCompliance(t *testing.T) {
	var _ llm.AIProvider = (*gemini.Driver)(nil)
}

// ── Unit tests (no network) ──────────────────────────────────────────────────

func TestName_NonEmpty(t *testing.T) {
	d := gemini.New("dummy-key")
	if d.Name() == "" {
		t.Error("Name() returned empty string")
	}
}

func TestDescription_NonEmpty(t *testing.T) {
	d := gemini.New("dummy-key")
	if d.Description() == "" {
		t.Error("Description() returned empty string")
	}
}

func TestIsConnected_InitiallyFalse(t *testing.T) {
	d := gemini.New("dummy-key")
	if d.IsConnected() {
		t.Error("IsConnected() should be false before Connect")
	}
}

func TestComplete_NotConnected(t *testing.T) {
	d := gemini.New("dummy-key")
	_, err := d.Complete(context.Background(), llm.CompletionRequest{Model: "any"})
	if !errors.Is(err, llm.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestStream_NotConnected(t *testing.T) {
	d := gemini.New("dummy-key")
	_, err := d.Stream(context.Background(), llm.CompletionRequest{Model: "any"})
	if !errors.Is(err, llm.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestListModels_NotConnected(t *testing.T) {
	d := gemini.New("dummy-key")
	_, err := d.ListModels(context.Background())
	if !errors.Is(err, llm.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestPing_NotConnected(t *testing.T) {
	d := gemini.New("dummy-key")
	err := d.Ping(context.Background())
	if !errors.Is(err, llm.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

// ── Integration tests (skip if no API key is present) ────────────────────────

func readAPIKey(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("test_data/google_api_key")
	if err != nil {
		t.Skip("no API key in test_data/google_api_key; skipping integration test")
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		t.Skip("empty API key in test_data/google_api_key; skipping integration test")
	}
	return key
}

func newConnectedDriver(t *testing.T) *gemini.Driver {
	t.Helper()
	apiKey := readAPIKey(t)
	d := gemini.New(apiKey)
	if err := d.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Disconnect(context.Background()) })
	return d
}

// cheapestModel selects the smallest/cheapest model to keep integration tests
// fast and inexpensive. Prefers flash-lite > flash-8b > flash > first available.
func cheapestModel(t *testing.T, d *gemini.Driver) llm.Model {
	t.Helper()
	models, err := d.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Skip("no models available; skipping integration test")
	}
	preferences := []string{"flash-lite-latest", "flash-8b", "flash-latest", "flash-lite", "flash"}
	for _, pref := range preferences {
		for _, m := range models {
			if strings.Contains(strings.ToLower(m.ID), pref) {
				return m
			}
		}
	}
	return models[0]
}

func TestConnect_IsConnected(t *testing.T) {
	d := newConnectedDriver(t)
	if !d.IsConnected() {
		t.Error("IsConnected() should be true after Connect")
	}
}

func TestDisconnect(t *testing.T) {
	apiKey := readAPIKey(t)
	d := gemini.New(apiKey)
	if err := d.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := d.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if d.IsConnected() {
		t.Error("IsConnected() should be false after Disconnect")
	}
}

func TestPing_Connected(t *testing.T) {
	d := newConnectedDriver(t)
	if err := d.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestListModels(t *testing.T) {
	d := newConnectedDriver(t)
	models, err := d.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Error("expected at least one model")
	}
	for _, m := range models {
		if m.ID == "" {
			t.Error("model with empty ID returned")
		}
		if !strings.HasPrefix(m.ID, "models/") {
			t.Errorf("model ID %q does not start with 'models/'", m.ID)
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
	skipIfRateLimited(t, err)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Role != llm.RoleAssistant {
		t.Errorf("expected role assistant, got %q", resp.Message.Role)
	}
	if resp.Message.Content == "" {
		t.Error("expected non-empty response content")
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
	skipIfRateLimited(t, err)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var sb strings.Builder
	for chunk := range ch {
		skipIfRateLimited(t, chunk.Err)
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		sb.WriteString(chunk.Content)
		if chunk.Done {
			break
		}
	}
	for range ch {
	}

	if sb.Len() == 0 {
		t.Error("accumulated stream content is empty")
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
			{Role: llm.RoleUser, Content: "Count from 1 to 1000, one number per line."},
		},
	})
	skipIfRateLimited(t, err)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	for range ch {
		cancel()
		break
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Error("channel did not close within 5 seconds after context cancellation")
			return
		}
	}
}

func weatherTool() llm.Tool {
	return llm.Tool{
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
}

func TestComplete_ToolCall(t *testing.T) {
	d := newConnectedDriver(t)
	model := cheapestModel(t, d)
	tool := weatherTool()

	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "What is the weather in Madrid?"},
	}

	resp, err := d.Complete(context.Background(), llm.CompletionRequest{
		Model:    model.ID,
		Messages: msgs,
		Tools:    []llm.Tool{tool},
	})
	skipIfRateLimited(t, err)
	if err != nil {
		t.Fatalf("Complete (first turn): %v", err)
	}
	if resp.StopReason != "tool_calls" {
		t.Skipf("model did not invoke tool (stop_reason=%q); skipping round-trip assertion", resp.StopReason)
	}
	if len(resp.Message.ToolCalls) == 0 {
		t.Fatal("expected at least one ToolCall")
	}

	tc := resp.Message.ToolCalls[0]
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected tool name %q, got %q", "get_weather", tc.Function.Name)
	}

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
	skipIfRateLimited(t, err)
	if err != nil {
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
	tool := weatherTool()

	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "What is the weather in Madrid?"},
	}

	ch, err := d.Stream(context.Background(), llm.CompletionRequest{
		Model:    model.ID,
		Messages: msgs,
		Tools:    []llm.Tool{tool},
	})
	skipIfRateLimited(t, err)
	if err != nil {
		t.Fatalf("Stream (first turn): %v", err)
	}

	var content strings.Builder
	var final llm.StreamChunk
	for chunk := range ch {
		skipIfRateLimited(t, chunk.Err)
		if chunk.Err != nil {
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
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected tool name %q, got %q", "get_weather", tc.Function.Name)
	}

	// Reconstruct the assistant message the way runLoopStream would, then
	// replay it into a second turn — this exercises the
	// Extra["gemini:content"] round trip for a streamed tool-call turn.
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
	skipIfRateLimited(t, err)
	if err != nil {
		t.Fatalf("Complete (second turn): %v", err)
	}
	if resp2.StopReason == "tool_calls" {
		t.Error("expected final text response in second turn, got another tool call")
	}
	if resp2.Message.Content == "" {
		t.Error("expected non-empty Content in final response")
	}
}
