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

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"auris/pkg/llm"
)

// Driver implements llm.AIProvider using the official Google generative-ai-go SDK.
type Driver struct {
	apiKey    string
	extraOpts []option.ClientOption
	client    *genai.Client
	connected bool
	mu        sync.RWMutex
}

// Option configures a Driver.
type Option func(*Driver)

// WithBaseURL overrides the API endpoint (useful for testing or proxies).
func WithBaseURL(u string) Option {
	return func(d *Driver) { d.extraOpts = append(d.extraOpts, option.WithEndpoint(u)) }
}

// WithHTTPClient replaces the HTTP client used by the SDK.
func WithHTTPClient(c *http.Client) Option {
	return func(d *Driver) { d.extraOpts = append(d.extraOpts, option.WithHTTPClient(c)) }
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
func (d *Driver) Description() string { return "Google's Gemini language models via the Generative AI SDK" }

// Connect creates the SDK client and validates the API key by listing models.
func (d *Driver) Connect(ctx context.Context) error {
	clientOpts := append([]option.ClientOption{option.WithAPIKey(d.apiKey)}, d.extraOpts...)
	client, err := genai.NewClient(ctx, clientOpts...)
	if err != nil {
		return fmt.Errorf("gemini: Connect: %w", mapErr(err))
	}
	iter := client.ListModels(ctx)
	if _, err := iter.Next(); err != nil && err != iterator.Done {
		client.Close()
		return fmt.Errorf("gemini: Connect: %w", mapErr(err))
	}
	d.mu.Lock()
	d.client = client
	d.connected = true
	d.mu.Unlock()
	return nil
}

// Disconnect closes the SDK client and marks the driver as disconnected.
func (d *Driver) Disconnect(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.client != nil {
		d.client.Close()
		d.client = nil
	}
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
	iter := client.ListModels(ctx)
	if _, err := iter.Next(); err != nil && err != iterator.Done {
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
	iter := client.ListModels(ctx)
	for {
		m, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("gemini: ListModels: %w", mapErr(err))
		}
		if slices.Contains(m.SupportedGenerationMethods, "generateContent") {
			models = append(models, llm.Model{ID: m.Name, Name: m.DisplayName})
		}
	}
	return models, nil
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
	model := buildModel(client, req)
	cs := model.StartChat()
	cs.History = buildHistory(req.Messages)
	parts := messageToParts(req.Messages[len(req.Messages)-1])
	resp, err := cs.SendMessage(ctx, parts...)
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
	model := buildModel(client, req)
	cs := model.StartChat()
	cs.History = buildHistory(req.Messages)
	parts := messageToParts(req.Messages[len(req.Messages)-1])

	ch := make(chan llm.StreamChunk, 16)
	go func() {
		defer close(ch)
		iter := cs.SendMessageStream(ctx, parts...)
		for {
			resp, err := iter.Next()
			if err == iterator.Done {
				ch <- llm.StreamChunk{Done: true}
				return
			}
			if err != nil {
				ch <- llm.StreamChunk{Done: true, Err: fmt.Errorf("gemini: Stream: %w", mapErr(err))}
				return
			}
			if len(resp.Candidates) == 0 {
				continue
			}
			cand := resp.Candidates[0]
			text := extractText(cand.Content)
			done := cand.FinishReason != genai.FinishReasonUnspecified
			chunk := llm.StreamChunk{Content: text, Done: done}
			if done && resp.UsageMetadata != nil {
				chunk.Usage = llm.TokenUsage{
					PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
					CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
				}
			}
			ch <- chunk
			if done {
				return
			}
			if ctx.Err() != nil {
				ch <- llm.StreamChunk{Done: true, Err: ctx.Err()}
				return
			}
		}
	}()
	return ch, nil
}

// ── Internal helpers ─────────────────────────────────────────────────────────

// getClient returns the SDK client under the read lock, or ErrNotConnected.
func (d *Driver) getClient() (*genai.Client, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if !d.connected || d.client == nil {
		return nil, fmt.Errorf("gemini: %w", llm.ErrNotConnected)
	}
	return d.client, nil
}

// buildModel creates a configured GenerativeModel from a CompletionRequest.
func buildModel(client *genai.Client, req llm.CompletionRequest) *genai.GenerativeModel {
	model := client.GenerativeModel(req.Model)

	for _, msg := range req.Messages {
		if msg.Role == llm.RoleSystem {
			model.SystemInstruction = &genai.Content{
				Parts: []genai.Part{genai.Text(msg.Content)},
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
		model.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
	}

	if req.MaxTokens > 0 {
		n := int32(req.MaxTokens)
		model.MaxOutputTokens = &n
	}

	return model
}

// buildHistory converts all messages except the last to genai chat history.
// System messages are excluded (handled via model.SystemInstruction).
func buildHistory(msgs []llm.Message) []*genai.Content {
	if len(msgs) <= 1 {
		return nil
	}
	var history []*genai.Content
	for _, msg := range msgs[:len(msgs)-1] {
		if msg.Role == llm.RoleSystem {
			continue
		}
		if c := msgToContent(msg); c != nil {
			history = append(history, c)
		}
	}
	return history
}

// msgToContent converts an llm.Message to a *genai.Content for chat history.
func msgToContent(msg llm.Message) *genai.Content {
	switch msg.Role {
	case llm.RoleUser:
		return &genai.Content{Role: "user", Parts: []genai.Part{genai.Text(msg.Content)}}
	case llm.RoleAssistant:
		if len(msg.ToolCalls) > 0 {
			parts := make([]genai.Part, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				var args map[string]any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				parts[i] = genai.FunctionCall{Name: tc.Function.Name, Args: args}
			}
			return &genai.Content{Role: "model", Parts: parts}
		}
		return &genai.Content{Role: "model", Parts: []genai.Part{genai.Text(msg.Content)}}
	case llm.RoleTool:
		return &genai.Content{
			Role: "user",
			Parts: []genai.Part{genai.FunctionResponse{
				Name:     msg.ToolCallID,
				Response: map[string]any{"result": msg.Content},
			}},
		}
	default:
		return nil
	}
}

// messageToParts converts an llm.Message to []genai.Part for SendMessage.
func messageToParts(msg llm.Message) []genai.Part {
	switch msg.Role {
	case llm.RoleUser:
		return []genai.Part{genai.Text(msg.Content)}
	case llm.RoleAssistant:
		if len(msg.ToolCalls) > 0 {
			parts := make([]genai.Part, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				var args map[string]any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				parts[i] = genai.FunctionCall{Name: tc.Function.Name, Args: args}
			}
			return parts
		}
		return []genai.Part{genai.Text(msg.Content)}
	case llm.RoleTool:
		return []genai.Part{genai.FunctionResponse{
			Name:     msg.ToolCallID,
			Response: map[string]any{"result": msg.Content},
		}}
	default:
		return []genai.Part{genai.Text(msg.Content)}
	}
}

// mapResponse converts a genai.GenerateContentResponse to llm.CompletionResponse.
func mapResponse(resp *genai.GenerateContentResponse) (llm.CompletionResponse, error) {
	if len(resp.Candidates) == 0 {
		return llm.CompletionResponse{}, fmt.Errorf("empty candidates in response")
	}
	cand := resp.Candidates[0]
	msg := llm.Message{Role: llm.RoleAssistant}

	var toolCalls []llm.ToolCall
	var textParts []string

	if cand.Content != nil {
		for _, part := range cand.Content.Parts {
			switch p := part.(type) {
			case genai.FunctionCall:
				argsJSON, _ := json.Marshal(p.Args)
				toolCalls = append(toolCalls, llm.ToolCall{
					ID: p.Name,
					Function: llm.ToolCallFunction{
						Name:      p.Name,
						Arguments: string(argsJSON),
					},
				})
			case genai.Text:
				textParts = append(textParts, string(p))
			}
		}
	}

	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	} else {
		msg.Content = strings.Join(textParts, "")
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

// extractText concatenates all Text parts from a Content.
func extractText(c *genai.Content) string {
	if c == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range c.Parts {
		if t, ok := part.(genai.Text); ok {
			sb.WriteString(string(t))
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
	if req, ok := m["required"].([]any); ok {
		for _, r := range req {
			if rs, ok := r.(string); ok {
				s.Required = append(s.Required, rs)
			}
		}
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
		return genai.TypeUnspecified
	}
}

// mapErr converts SDK API errors to llm sentinel errors.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *googleapi.Error
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
