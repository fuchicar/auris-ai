package ollama

import (
	"encoding/json"
	"strings"
	"testing"

	"auris/pkg/llm"
)

// TestBuildChatRequest_NumCtxDefault verifies that the driver passes its
// default num_ctx in the options block. Without this override, Ollama
// truncates conversations at 2048-4096 tokens and tool-calling loops break
// on the first follow-up turn (the model loses the system prompt + user
// question and reverts to a generic greeting).
func TestBuildChatRequest_NumCtxDefault(t *testing.T) {
	d := New()
	req := d.buildChatRequest(llm.CompletionRequest{
		Model: "gemma4:e2b",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "hello"},
		},
	}, false)

	if req.Options == nil {
		t.Fatal("Options must not be nil — Ollama would fall back to its small default num_ctx")
	}
	if req.Options.NumCtx != defaultNumCtx {
		t.Errorf("Options.NumCtx = %d, want %d", req.Options.NumCtx, defaultNumCtx)
	}

	body, _ := json.Marshal(req)
	if !strings.Contains(string(body), `"num_ctx":`) {
		t.Errorf("serialized request must contain num_ctx: %s", body)
	}
}

// TestBuildChatRequest_NumCtxOverride verifies that WithContextSize
// overrides the default — the path used by AURIS_OLLAMA_NUM_CTX.
func TestBuildChatRequest_NumCtxOverride(t *testing.T) {
	d := New(WithContextSize(131072))
	req := d.buildChatRequest(llm.CompletionRequest{
		Model:    "gemma4:e2b",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	}, false)

	if req.Options == nil || req.Options.NumCtx != 131072 {
		got := 0
		if req.Options != nil {
			got = req.Options.NumCtx
		}
		t.Errorf("Options.NumCtx = %d, want 131072", got)
	}
}

// TestWithContextSize_IgnoresNonPositive verifies that bogus values (0 or
// negative) don't clobber the default.
func TestWithContextSize_IgnoresNonPositive(t *testing.T) {
	for _, n := range []int{0, -1, -100} {
		d := New(WithContextSize(n))
		req := d.buildChatRequest(llm.CompletionRequest{
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "x"}},
		}, false)
		if req.Options == nil || req.Options.NumCtx != defaultNumCtx {
			t.Errorf("WithContextSize(%d) clobbered default: got %+v", n, req.Options)
		}
	}
}
