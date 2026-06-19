package ollama_test

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"auris/pkg/drivers/ollama"
	"auris/pkg/llm"
)

// ── Compile-time interface compliance ────────────────────────────────────────

func TestInterfaceCompliance(t *testing.T) {
	var _ llm.AIProvider = (*ollama.Driver)(nil)
}

// ── Unit tests (no network) ──────────────────────────────────────────────────

func TestName_NonEmpty(t *testing.T) {
	d := ollama.New()
	if d.Name() == "" {
		t.Error("Name() returned empty string")
	}
}

func TestDescription_NonEmpty(t *testing.T) {
	d := ollama.New()
	if d.Description() == "" {
		t.Error("Description() returned empty string")
	}
}

func TestIsConnected_InitiallyFalse(t *testing.T) {
	d := ollama.New()
	if d.IsConnected() {
		t.Error("IsConnected() should be false before Connect")
	}
}

func TestComplete_NotConnected(t *testing.T) {
	d := ollama.New()
	_, err := d.Complete(context.Background(), llm.CompletionRequest{Model: "any"})
	if !errors.Is(err, llm.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestStream_NotConnected(t *testing.T) {
	d := ollama.New()
	_, err := d.Stream(context.Background(), llm.CompletionRequest{Model: "any"})
	if !errors.Is(err, llm.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestListModels_NotConnected(t *testing.T) {
	d := ollama.New()
	_, err := d.ListModels(context.Background())
	if !errors.Is(err, llm.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

func TestPing_NotConnected(t *testing.T) {
	d := ollama.New()
	err := d.Ping(context.Background())
	if !errors.Is(err, llm.ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got: %v", err)
	}
}

// ── Integration tests (skip if Ollama is not running) ────────────────────────

func requireOllama(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:11434/api/tags", nil)
	if err != nil {
		t.Skip("could not build Ollama probe request; skipping integration test")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Skip("Ollama not running at localhost:11434; skipping integration test")
	}
	resp.Body.Close()
}

func newConnectedDriver(t *testing.T) *ollama.Driver {
	t.Helper()
	requireOllama(t)
	d := ollama.New()
	if err := d.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Disconnect(context.Background()) })
	return d
}

// firstModel returns the smallest available model by size to keep integration tests fast.
func firstModel(t *testing.T, d *ollama.Driver) llm.Model {
	t.Helper()
	models, err := d.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Skip("no models available in Ollama; skipping integration test")
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Size == 0 {
			return false
		}
		if models[j].Size == 0 {
			return true
		}
		return models[i].Size < models[j].Size
	})
	return models[0]
}

func TestConnect_IsConnected(t *testing.T) {
	d := newConnectedDriver(t)
	if !d.IsConnected() {
		t.Error("IsConnected() should be true after Connect")
	}
}

func TestDisconnect(t *testing.T) {
	requireOllama(t)
	d := ollama.New()
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
		t.Skip("no models available in Ollama; nothing to assert")
	}
	for _, m := range models {
		if m.ID == "" {
			t.Error("model with empty ID returned")
		}
	}
}

func TestComplete_FirstModel(t *testing.T) {
	d := newConnectedDriver(t)
	model := firstModel(t, d)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := d.Complete(ctx, llm.CompletionRequest{
		Model: model.ID,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Reply with a single word: hello"},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			t.Skip("model did not respond within 60s; skipping")
		}
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Role != llm.RoleAssistant {
		t.Errorf("expected role assistant, got %q", resp.Message.Role)
	}
	if resp.Message.Content == "" {
		t.Error("expected non-empty response content")
	}
}

func TestStream_FirstModel(t *testing.T) {
	d := newConnectedDriver(t)
	model := firstModel(t, d)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ch, err := d.Stream(ctx, llm.CompletionRequest{
		Model: model.ID,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Reply with a single word: hello"},
		},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var sb strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			if errors.Is(chunk.Err, context.DeadlineExceeded) {
				t.Skip("model did not respond within 60s; skipping")
			}
			t.Fatalf("stream error: %v", chunk.Err)
		}
		sb.WriteString(chunk.Content)
		if chunk.Done {
			break
		}
	}
	// Drain any remaining chunks (should be none after Done, but be safe).
	for range ch {
	}

	if sb.Len() == 0 {
		t.Error("accumulated stream content is empty")
	}
}

func TestStream_ContextCancellation(t *testing.T) {
	d := newConnectedDriver(t)
	model := firstModel(t, d)

	// outerCtx bounds the wait for the first token; avoids hanging the package timeout.
	outerCtx, outerCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer outerCancel()

	// streamCtx is what we actually cancel to test cancellation propagation.
	streamCtx, streamCancel := context.WithCancel(outerCtx)
	defer streamCancel()

	ch, err := d.Stream(streamCtx, llm.CompletionRequest{
		Model: model.ID,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Count from 1 to 1000, one number per line."},
		},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	// Cancel after receiving the first chunk.
	chunk, ok := <-ch
	if !ok {
		t.Skip("stream closed without producing any chunks")
	}
	if chunk.Err != nil {
		if errors.Is(chunk.Err, context.DeadlineExceeded) {
			t.Skip("model did not start streaming within 60s; skipping")
		}
		t.Fatalf("stream error before cancel: %v", chunk.Err)
	}
	streamCancel()

	// Verify channel drains and closes within 2 seconds.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // channel closed — test passes
			}
		case <-deadline:
			t.Error("channel did not close within 2 seconds after context cancellation")
			return
		}
	}
}
