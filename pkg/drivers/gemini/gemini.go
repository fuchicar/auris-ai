package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"google.golang.org/genai"

	"github.com/fuchicar/auris-ai/pkg/llm"
)

// Driver implements llm.AIProvider using the official google.golang.org/genai SDK.
type Driver struct {
	apiKey     string
	httpClient *http.Client // optional; passed to ClientConfig.HTTPClient
	baseURL    string       // optional; passed to ClientConfig.HTTPOptions.BaseURL
	client     *genai.Client
	connected  bool
	mu         sync.RWMutex
}

// Option configures a Driver.
type Option func(*Driver)

// WithBaseURL overrides the API endpoint (useful for testing or proxies).
func WithBaseURL(u string) Option {
	return func(d *Driver) { d.baseURL = u }
}

// WithHTTPClient replaces the HTTP client used by the SDK.
func WithHTTPClient(c *http.Client) Option {
	return func(d *Driver) { d.httpClient = c }
}

// New constructs a Driver. apiKey is validated on Connect.
func New(apiKey string, opts ...Option) *Driver {
	d := &Driver{apiKey: apiKey}
	for _, o := range opts {
		o(d)
	}
	return d
}

func (d *Driver) Name() string        { return "Google Gemini" }
func (d *Driver) Description() string { return "Google's Gemini language models via the GenAI SDK" }

// Connect creates the SDK client and validates the API key by listing models.
func (d *Driver) Connect(ctx context.Context) error {
	cc := &genai.ClientConfig{APIKey: d.apiKey}
	if d.httpClient != nil {
		cc.HTTPClient = d.httpClient
	}
	if d.baseURL != "" {
		cc.HTTPOptions.BaseURL = d.baseURL
	}
	client, err := genai.NewClient(ctx, cc)
	if err != nil {
		return fmt.Errorf("gemini: Connect: %w", mapErr(err))
	}
	if _, err := client.Models.List(ctx, nil); err != nil {
		return fmt.Errorf("gemini: Connect: %w", mapErr(err))
	}
	d.mu.Lock()
	d.client = client
	d.connected = true
	d.mu.Unlock()
	return nil
}

// Disconnect marks the driver as disconnected.
func (d *Driver) Disconnect(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.client = nil
	d.connected = false
	return nil
}

// IsConnected reports whether the driver is in a connected state.
func (d *Driver) IsConnected() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.connected
}

// Ping checks that the connection is still alive without modifying state.
func (d *Driver) Ping(ctx context.Context) error {
	client, err := d.getClient()
	if err != nil {
		return err
	}
	if _, err := client.Models.List(ctx, nil); err != nil {
		return fmt.Errorf("gemini: Ping: %w", mapErr(err))
	}
	return nil
}

// ListModels returns all Gemini models that support content generation.
func (d *Driver) ListModels(ctx context.Context) ([]llm.Model, error) {
	client, err := d.getClient()
	if err != nil {
		return nil, err
	}
	var models []llm.Model
	for m, err := range client.Models.All(ctx) {
		if err != nil {
			return nil, fmt.Errorf("gemini: ListModels: %w", mapErr(err))
		}
		if slices.Contains(m.SupportedActions, "generateContent") {
			models = append(models, llm.Model{ID: m.Name, Name: m.DisplayName})
		}
	}
	return models, nil
}

// contextWindows maps known Gemini model-family prefixes to their published
// input token limit. Google's model-list API doesn't surface this on the
// [genai.Model] returned by client.Models.All, so it's hardcoded here;
// revisit if a new model family ships with a different window.
var contextWindows = []struct {
	prefix string
	tokens int
}{
	{"gemini-2.5-pro", 1_048_576},
	{"gemini-2.5-flash", 1_048_576},
	{"gemini-2.0-flash", 1_048_576},
	{"gemini-1.5-pro", 2_097_152},
	{"gemini-1.5-flash", 1_048_576},
}

// ContextWindow implements llm.ContextWindowReporter. model may be a bare
// model ID or a "models/<id>" resource name as returned by ListModels; both
// forms are matched by prefix. Returns 0 for unrecognized models.
func (d *Driver) ContextWindow(model string) int {
	name := strings.TrimPrefix(model, "models/")
	for _, cw := range contextWindows {
		if strings.HasPrefix(name, cw.prefix) {
			return cw.tokens
		}
	}
	return 0
}

// Complete sends a non-streaming completion request and returns the full response.
func (d *Driver) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	client, err := d.getClient()
	if err != nil {
		return llm.CompletionResponse{}, err
	}
	if len(req.Messages) == 0 {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: Complete: no messages in request")
	}
	contents, config := buildRequest(req)
	resp, err := client.Models.GenerateContent(ctx, req.Model, contents, config)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: Complete: %w", mapErr(err))
	}
	result, err := mapResponse(resp)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: Complete: %w", err)
	}
	return result, nil
}

// Stream sends a streaming completion request and returns a channel of chunks.
// The channel is always closed after a Done==true chunk, even on error or cancellation.
func (d *Driver) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamChunk, error) {
	client, err := d.getClient()
	if err != nil {
		return nil, err
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("gemini: Stream: no messages in request")
	}
	contents, config := buildRequest(req)

	ch := make(chan llm.StreamChunk, 64)
	go func() {
		defer close(ch)
		// mergedParts accumulates content parts across chunks (Gemini streams
		// incremental deltas per candidate) so the terminal chunk can extract
		// ToolCalls and rebuild an Extra["gemini:content"] payload identical in
		// shape to what mapResponse produces for Complete.
		var mergedParts []*genai.Part
		for resp, err := range client.Models.GenerateContentStream(ctx, req.Model, contents, config) {
			if err != nil {
				ch <- llm.StreamChunk{Done: true, Err: fmt.Errorf("gemini: Stream: %w", mapErr(err))}
				return
			}
			if len(resp.Candidates) == 0 {
				continue
			}
			cand := resp.Candidates[0]
			var textDelta string
			if cand.Content != nil {
				textDelta = extractText(cand.Content)
				mergedParts = append(mergedParts, cand.Content.Parts...)
			}
			done := cand.FinishReason != genai.FinishReasonUnspecified
			chunk := llm.StreamChunk{Content: textDelta, Done: done}
			if done {
				terminal := buildTerminalChunk(cand.FinishReason, mergedParts, resp.UsageMetadata)
				chunk.ToolCalls = terminal.ToolCalls
				chunk.Extra = terminal.Extra
				chunk.StopReason = terminal.StopReason
				chunk.Usage = terminal.Usage
			}
			ch <- chunk
			if done || ctx.Err() != nil {
				return
			}
		}
		// The stream ended without any candidate ever reporting a terminal
		// FinishReason (dropped connection, truncated response, ...). Salvage
		// whatever was merged so far instead of emitting a bare, data-losing
		// Done chunk — mirrors the anthropic/openai drivers' own defensive
		// fallback for the same "stream closed before an explicit stop" case.
		ch <- buildTerminalChunk(genai.FinishReasonUnspecified, mergedParts, nil)
	}()
	return ch, nil
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (d *Driver) getClient() (*genai.Client, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if !d.connected || d.client == nil {
		return nil, fmt.Errorf("gemini: %w", llm.ErrNotConnected)
	}
	return d.client, nil
}

// buildRequest converts a CompletionRequest into a content list and config for the API.
func buildRequest(req llm.CompletionRequest) ([]*genai.Content, *genai.GenerateContentConfig) {
	config := &genai.GenerateContentConfig{}

	for _, msg := range req.Messages {
		if msg.Role == llm.RoleSystem {
			config.SystemInstruction = &genai.Content{
				Parts: []*genai.Part{{Text: msg.Content}},
			}
			break
		}
	}

	if len(req.Tools) > 0 {
		decls := make([]*genai.FunctionDeclaration, len(req.Tools))
		for i, t := range req.Tools {
			decls[i] = &genai.FunctionDeclaration{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  schemaFromMap(t.Function.Parameters),
			}
		}
		config.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
	}

	if req.MaxTokens > 0 {
		config.MaxOutputTokens = int32(req.MaxTokens)
	}

	// Build contents, merging consecutive RoleTool messages into a single user
	// turn. The Gemini API requires that all FunctionResponses for a given model
	// turn be in ONE Content with multiple Parts — sending them as separate
	// Content objects violates the spec and produces incoherent responses.
	var contents []*genai.Content
	msgs := req.Messages
	for i := 0; i < len(msgs); {
		msg := msgs[i]
		if msg.Role == llm.RoleSystem {
			i++
			continue
		}
		if msg.Role != llm.RoleTool {
			if c := msgToContent(msg); c != nil {
				contents = append(contents, c)
			}
			i++
			continue
		}
		merged := &genai.Content{Role: "user"}
		for i < len(msgs) && msgs[i].Role == llm.RoleTool {
			merged.Parts = append(merged.Parts, toolResponsePart(msgs[i]))
			i++
		}
		contents = append(contents, merged)
	}
	return contents, config
}

// msgToContent converts an llm.Message to a *genai.Content for the API.
func msgToContent(msg llm.Message) *genai.Content {
	switch msg.Role {
	case llm.RoleUser:
		return &genai.Content{Role: "user", Parts: []*genai.Part{{Text: msg.Content}}}
	case llm.RoleAssistant:
		if len(msg.ToolCalls) > 0 {
			// Use the preserved original Content when available so that
			// provider-specific fields (e.g. thought_signature) are not lost.
			if msg.Extra != nil {
				if raw, ok := msg.Extra["gemini:content"].(*genai.Content); ok && raw != nil {
					return raw
				}
			}
			parts := make([]*genai.Part, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				var args map[string]any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				parts[i] = &genai.Part{FunctionCall: &genai.FunctionCall{Name: tc.Function.Name, Args: args}}
			}
			return &genai.Content{Role: "model", Parts: parts}
		}
		return &genai.Content{Role: "model", Parts: []*genai.Part{{Text: msg.Content}}}
	case llm.RoleTool:
		return &genai.Content{Role: "user", Parts: []*genai.Part{toolResponsePart(msg)}}
	default:
		return nil
	}
}

// toolResponsePart converts a RoleTool message to a FunctionResponse Part.
// Unmarshals the JSON content so the SDK encodes structured data instead of a
// quoted string; falls back to a plain string for non-JSON error messages.
func toolResponsePart(msg llm.Message) *genai.Part {
	var responseVal any
	if err := json.Unmarshal([]byte(msg.Content), &responseVal); err != nil {
		responseVal = msg.Content
	}
	return &genai.Part{
		FunctionResponse: &genai.FunctionResponse{
			Name:     msg.ToolCallID,
			Response: map[string]any{"result": responseVal},
		},
	}
}

// mapResponse converts a GenerateContentResponse to llm.CompletionResponse.
func mapResponse(resp *genai.GenerateContentResponse) (llm.CompletionResponse, error) {
	if len(resp.Candidates) == 0 {
		return llm.CompletionResponse{}, fmt.Errorf("empty candidates in response")
	}
	cand := resp.Candidates[0]
	msg := llm.Message{Role: llm.RoleAssistant}

	var toolCalls []llm.ToolCall
	if cand.Content != nil {
		toolCalls = partsToToolCalls(cand.Content.Parts)
	}

	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
		// Preserve the original Content so that thought_signature in Parts
		// survives round-trips through the conversation history.
		if cand.Content != nil {
			msg.Extra = map[string]any{"gemini:content": cand.Content}
		}
	} else {
		msg.Content = extractText(cand.Content)
	}

	var usage llm.TokenUsage
	if resp.UsageMetadata != nil {
		usage = llm.TokenUsage{
			PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
			CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
		}
	}

	return llm.CompletionResponse{
		Message:    msg,
		Usage:      usage,
		StopReason: mapFinishReason(cand.FinishReason, len(toolCalls) > 0),
	}, nil
}

// partsToToolCalls extracts ToolCalls from content parts. Shared by mapResponse
// and Stream so tool-call parsing isn't re-derived per code path.
func partsToToolCalls(parts []*genai.Part) []llm.ToolCall {
	var toolCalls []llm.ToolCall
	for _, part := range parts {
		if part.FunctionCall != nil {
			argsJSON, _ := json.Marshal(part.FunctionCall.Args)
			toolCalls = append(toolCalls, llm.ToolCall{
				ID: part.FunctionCall.Name,
				Function: llm.ToolCallFunction{
					Name:      part.FunctionCall.Name,
					Arguments: string(argsJSON),
				},
			})
		}
	}
	return toolCalls
}

// buildTerminalChunk builds the Done:true StreamChunk for Stream from
// whatever content parts have been merged so far, extracting tool calls and
// deriving StopReason/Usage the same way mapResponse does for Complete. Used
// both for a normal terminal candidate and for the fallback path where the
// stream ends without one (reason == genai.FinishReasonUnspecified, usage
// nil), so a dropped/truncated stream still surfaces any tool call the model
// had already fully emitted instead of a bare, data-losing Done chunk.
func buildTerminalChunk(reason genai.FinishReason, mergedParts []*genai.Part, usage *genai.GenerateContentResponseUsageMetadata) llm.StreamChunk {
	toolCalls := partsToToolCalls(mergedParts)
	chunk := llm.StreamChunk{
		Done:       true,
		StopReason: mapFinishReason(reason, len(toolCalls) > 0),
	}
	if len(toolCalls) > 0 {
		chunk.ToolCalls = toolCalls
		chunk.Extra = map[string]any{
			"gemini:content": &genai.Content{Role: "model", Parts: mergedParts},
		}
	}
	if usage != nil {
		chunk.Usage = llm.TokenUsage{
			PromptTokens:     int(usage.PromptTokenCount),
			CompletionTokens: int(usage.CandidatesTokenCount),
		}
	}
	return chunk
}

// extractText concatenates all Text parts from a Content.
func extractText(c *genai.Content) string {
	if c == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range c.Parts {
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
}

// mapFinishReason converts genai.FinishReason to our canonical stop reason strings.
func mapFinishReason(reason genai.FinishReason, hasToolCalls bool) string {
	if hasToolCalls {
		return "tool_calls"
	}
	if reason == genai.FinishReasonMaxTokens {
		return "length"
	}
	return "stop"
}

// schemaFromMap converts a JSON Schema map[string]any to a *genai.Schema recursively.
func schemaFromMap(m map[string]any) *genai.Schema {
	if m == nil {
		return nil
	}
	s := &genai.Schema{}
	if t, ok := m["type"].(string); ok {
		s.Type = parseType(t)
	}
	if v, ok := m["description"].(string); ok {
		s.Description = v
	}
	if v, ok := m["format"].(string); ok {
		s.Format = v
	}
	if props, ok := m["properties"].(map[string]any); ok {
		s.Properties = make(map[string]*genai.Schema, len(props))
		for k, v := range props {
			if vm, ok := v.(map[string]any); ok {
				s.Properties[k] = schemaFromMap(vm)
			}
		}
	}
	switch req := m["required"].(type) {
	case []any:
		for _, r := range req {
			if rs, ok := r.(string); ok {
				s.Required = append(s.Required, rs)
			}
		}
	case []string:
		s.Required = append(s.Required, req...)
	}
	if items, ok := m["items"].(map[string]any); ok {
		s.Items = schemaFromMap(items)
	}
	if enum, ok := m["enum"].([]any); ok {
		for _, e := range enum {
			if es, ok := e.(string); ok {
				s.Enum = append(s.Enum, es)
			}
		}
	}
	return s
}

// parseType maps a JSON Schema type string to genai.Type.
func parseType(t string) genai.Type {
	switch strings.ToLower(t) {
	case "string":
		return genai.TypeString
	case "number":
		return genai.TypeNumber
	case "integer":
		return genai.TypeInteger
	case "boolean":
		return genai.TypeBoolean
	case "array":
		return genai.TypeArray
	case "object":
		return genai.TypeObject
	default:
		return ""
	}
}

// mapErr converts SDK API errors to llm sentinel errors.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case http.StatusUnauthorized, http.StatusForbidden:
			return llm.ErrUnauthorized
		case http.StatusNotFound:
			return llm.ErrModelNotFound
		case http.StatusTooManyRequests:
			if apiErr.Message != "" {
				return fmt.Errorf("%w: %s", llm.ErrRateLimit, apiErr.Message)
			}
			return fmt.Errorf("%w: %s", llm.ErrRateLimit, err.Error())
		case http.StatusBadRequest:
			if strings.Contains(strings.ToLower(apiErr.Message), "token") {
				return llm.ErrContextTooLong
			}
			return fmt.Errorf("gemini: bad request: %s", apiErr.Message)
		}
		return fmt.Errorf("gemini: API error %d: %s", apiErr.Code, apiErr.Message)
	}
	return err
}
