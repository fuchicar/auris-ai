package gemini

import (
	"testing"

	"github.com/fuchicar/auris-ai/pkg/llm"
)

func TestMsgToContent_ToolRole_JSONArray(t *testing.T) {
	msg := llm.Message{
		Role:       llm.RoleTool,
		ToolCallID: "fetch_news",
		Content:    `[{"title":"Article 1","summary":"body","url":"http://x.com"}]`,
	}
	c := msgToContent(msg)
	if c == nil {
		t.Fatal("msgToContent returned nil")
	}
	if len(c.Parts) == 0 || c.Parts[0].FunctionResponse == nil {
		t.Fatal("expected FunctionResponse part")
	}
	result, ok := c.Parts[0].FunctionResponse.Response["result"]
	if !ok {
		t.Fatal("response map missing 'result' key")
	}
	if _, isString := result.(string); isString {
		t.Error("result should be a parsed value ([]any), not a raw JSON string — double-encoding bug")
	}
	if _, isSlice := result.([]any); !isSlice {
		t.Errorf("result should be []any, got %T", result)
	}
}

func TestMsgToContent_ToolRole_ErrorString(t *testing.T) {
	msg := llm.Message{
		Role:       llm.RoleTool,
		ToolCallID: "fetch_news",
		Content:    "error: no news provider configured",
	}
	c := msgToContent(msg)
	if c == nil {
		t.Fatal("msgToContent returned nil")
	}
	result, ok := c.Parts[0].FunctionResponse.Response["result"]
	if !ok {
		t.Fatal("response map missing 'result' key")
	}
	s, isString := result.(string)
	if !isString {
		t.Errorf("non-JSON content should pass through as string, got %T", result)
	}
	if s != msg.Content {
		t.Errorf("expected content %q, got %q", msg.Content, s)
	}
}

func TestMsgToContent_ToolRole_JSONObject(t *testing.T) {
	msg := llm.Message{
		Role:       llm.RoleTool,
		ToolCallID: "market_get_quote",
		Content:    `{"bid":99.5,"ask":100.0,"last":99.8}`,
	}
	c := msgToContent(msg)
	result := c.Parts[0].FunctionResponse.Response["result"]
	if _, isMap := result.(map[string]any); !isMap {
		t.Errorf("JSON object result should be map[string]any, got %T", result)
	}
}

func TestSchemaFromMap_RequiredStringSlice(t *testing.T) {
	m := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{"keywords", "symbol"},
	}
	s := schemaFromMap(m)
	if len(s.Required) != 2 {
		t.Errorf("expected 2 required fields, got %d: %v", len(s.Required), s.Required)
	}
	found := map[string]bool{}
	for _, r := range s.Required {
		found[r] = true
	}
	if !found["keywords"] || !found["symbol"] {
		t.Errorf("required fields missing: %v", s.Required)
	}
}

func TestSchemaFromMap_RequiredAnySlice(t *testing.T) {
	m := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []any{"query"},
	}
	s := schemaFromMap(m)
	if len(s.Required) != 1 || s.Required[0] != "query" {
		t.Errorf("expected required=[query], got %v", s.Required)
	}
}

func TestBuildRequest_MergesConsecutiveToolMessages(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "system prompt"},
		{Role: llm.RoleUser, Content: "Lee las noticias"},
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{ID: "time_today", Function: llm.ToolCallFunction{Name: "time_today", Arguments: "{}"}},
				{ID: "fetch_news", Function: llm.ToolCallFunction{Name: "fetch_news", Arguments: `{"keywords":[]}`}},
			},
		},
		{Role: llm.RoleTool, ToolCallID: "time_today", Content: `"2026-05-23"`},
		{Role: llm.RoleTool, ToolCallID: "fetch_news", Content: `[{"title":"Noticia 1"}]`},
	}

	req := llm.CompletionRequest{Messages: msgs}
	contents, _ := buildRequest(req)

	// Expect: user, model, user (merged tool responses) — 3 contents total
	if len(contents) != 3 {
		t.Fatalf("expected 3 contents, got %d", len(contents))
	}

	merged := contents[2]
	if merged.Role != "user" {
		t.Errorf("merged content role should be 'user', got %q", merged.Role)
	}
	if len(merged.Parts) != 2 {
		t.Fatalf("merged content should have 2 FunctionResponse parts, got %d", len(merged.Parts))
	}
	if merged.Parts[0].FunctionResponse == nil || merged.Parts[0].FunctionResponse.Name != "time_today" {
		t.Errorf("first part should be FunctionResponse for time_today")
	}
	if merged.Parts[1].FunctionResponse == nil || merged.Parts[1].FunctionResponse.Name != "fetch_news" {
		t.Errorf("second part should be FunctionResponse for fetch_news")
	}
	// Verify the fetch_news result is parsed as []any, not a string
	result := merged.Parts[1].FunctionResponse.Response["result"]
	if _, ok := result.([]any); !ok {
		t.Errorf("fetch_news result should be []any, got %T", result)
	}
}

func TestBuildRequest_SingleToolMessage_NotMerged(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "¿Qué hora es?"},
		{
			Role:      llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{ID: "time_today", Function: llm.ToolCallFunction{Name: "time_today", Arguments: "{}"}}},
		},
		{Role: llm.RoleTool, ToolCallID: "time_today", Content: `"2026-05-23"`},
	}

	req := llm.CompletionRequest{Messages: msgs}
	contents, _ := buildRequest(req)

	if len(contents) != 3 {
		t.Fatalf("expected 3 contents, got %d", len(contents))
	}
	last := contents[2]
	if last.Role != "user" || len(last.Parts) != 1 || last.Parts[0].FunctionResponse == nil {
		t.Error("single tool result should produce one FunctionResponse part in a user Content")
	}
}
