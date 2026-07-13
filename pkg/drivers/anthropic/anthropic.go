package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"auris/pkg/llm"
)

const defaultMaxTokens = 8192

// contextWindowDefault is the standard context window shared by every
// current Claude 3+/4 model family. Anthropic's model-list API does not
// report this, so it's hardcoded here; revisit if a future model ships with
// a different (e.g. long-context beta) window.
const contextWindowDefault = 200_000

// Option configures a Driver.
type Option func(*Driver)

// WithBaseURL overrides the Anthropic API base URL.
func WithBaseURL(u string) Option {
	return func(d *Driver) { d.baseURL = u }
}

// WithHTTPClient injects a custom HTTP client into the SDK.
func WithHTTPClient(c *http.Client) Option {
	return func(d *Driver) { d.httpClient = c }
}

// Driver implements llm.AIProvider for Anthropic Claude models.
type Driver struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	client     *sdk.Client
	connected  bool
	mu         sync.RWMutex
}

// New creates a new Anthropic driver. The API key is required; options are optional.
func New(apiKey string, opts ...Option) *Driver {
	d := &Driver{apiKey: apiKey}
	for _, o := range opts {
		o(d)
	}
	return d
}

func (d *Driver) Name() string        { return "Anthropic Claude" }
func (d *Driver) Description() string { return "Anthropic's Claude models via the Messages API" }

// Connect validates the API key by listing available models.
func (d *Driver) Connect(ctx context.Context) error {
	cliOpts := []option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(d.apiKey),
	}
	if d.baseURL != "" {
		cliOpts = append(cliOpts, option.WithBaseURL(d.baseURL))
	}
	if d.httpClient != nil {
		cliOpts = append(cliOpts, option.WithHTTPClient(d.httpClient))
	}

	client := sdk.NewClient(cliOpts...)
	if _, err := client.Models.List(ctx, sdk.ModelListParams{}); err != nil {
		return fmt.Errorf("anthropic: Connect: %w", mapErr(err))
	}

	d.mu.Lock()
	d.client = &client
	d.connected = true
	d.mu.Unlock()
	return nil
}

// Disconnect clears the client and marks the driver as disconnected.
func (d *Driver) Disconnect(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.client = nil
	d.connected = false
	return nil
}

// IsConnected reports whether Connect has been called successfully.
func (d *Driver) IsConnected() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.connected
}

// Ping validates the connection by listing models.
func (d *Driver) Ping(ctx context.Context) error {
	client, err := d.getClient()
	if err != nil {
		return err
	}
	if _, err := client.Models.List(ctx, sdk.ModelListParams{}); err != nil {
		return fmt.Errorf("anthropic: Ping: %w", mapErr(err))
	}
	return nil
}

// ListModels returns all Claude models available to the account.
func (d *Driver) ListModels(ctx context.Context) ([]llm.Model, error) {
	client, err := d.getClient()
	if err != nil {
		return nil, err
	}
	pager := client.Models.ListAutoPaging(ctx, sdk.ModelListParams{})
	var models []llm.Model
	for pager.Next() {
		m := pager.Current()
		models = append(models, llm.Model{ID: m.ID, Name: m.DisplayName})
	}
	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("anthropic: ListModels: %w", mapErr(err))
	}
	return models, nil
}

// ContextWindow implements llm.ContextWindowReporter. Every known Claude
// model ID reports the shared 200K window; unrecognized IDs report 0.
func (d *Driver) ContextWindow(model string) int {
	if strings.HasPrefix(model, "claude-") {
		return contextWindowDefault
	}
	return 0
}

// Complete sends a blocking completion request and returns the full response.
func (d *Driver) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	client, err := d.getClient()
	if err != nil {
		return llm.CompletionResponse{}, err
	}
	params, err := buildParams(req)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: Complete: %w", err)
	}
	resp, err := client.Messages.New(ctx, params)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: Complete: %w", mapErr(err))
	}
	return mapResponse(resp), nil
}

// Stream sends a streaming request and returns a channel of incremental chunks.
// The channel is always closed after a chunk with Done == true, even on error.
func (d *Driver) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamChunk, error) {
	client, err := d.getClient()
	if err != nil {
		return nil, err
	}
	params, err := buildParams(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: Stream: %w", err)
	}

	stream := client.Messages.NewStreaming(ctx, params)
	ch := make(chan llm.StreamChunk, 64)

	go func() {
		defer close(ch)
		defer stream.Close()

		// acc accumulates the full SDK Message across events (including tool_use
		// blocks assembled from content_block_start/input_json_delta) using the
		// SDK's own accumulator, so mapResponse — the exact function Complete
		// uses — can be reused verbatim for the terminal chunk instead of
		// re-deriving tool-call/Extra parsing here.
		var acc sdk.Message

		for stream.Next() {
			ev := stream.Current()
			if err := acc.Accumulate(ev); err != nil {
				ch <- llm.StreamChunk{Done: true, Err: fmt.Errorf("anthropic: Stream: accumulate: %w", err)}
				return
			}
			if ev.Type == "content_block_delta" && ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				ch <- llm.StreamChunk{Content: ev.Delta.Text}
			}
			if ev.Type == "message_stop" {
				resp := mapResponse(&acc)
				ch <- llm.StreamChunk{
					Done:       true,
					Usage:      resp.Usage,
					StopReason: resp.StopReason,
					ToolCalls:  resp.Message.ToolCalls,
					Extra:      resp.Message.Extra,
				}
				return
			}
		}

		if err := stream.Err(); err != nil {
			if ctx.Err() != nil {
				ch <- llm.StreamChunk{Done: true, Err: ctx.Err()}
			} else {
				ch <- llm.StreamChunk{Done: true, Err: fmt.Errorf("anthropic: Stream: %w", mapErr(err))}
			}
			return
		}

		// Defensive fallback: the stream ended without an explicit message_stop.
		resp := mapResponse(&acc)
		ch <- llm.StreamChunk{
			Done:       true,
			Usage:      resp.Usage,
			StopReason: resp.StopReason,
			ToolCalls:  resp.Message.ToolCalls,
			Extra:      resp.Message.Extra,
		}
	}()

	return ch, nil
}

func (d *Driver) getClient() (*sdk.Client, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if !d.connected || d.client == nil {
		return nil, fmt.Errorf("anthropic: %w", llm.ErrNotConnected)
	}
	return d.client, nil
}

// buildParams converts an llm.CompletionRequest to sdk.MessageNewParams.
func buildParams(req llm.CompletionRequest) (sdk.MessageNewParams, error) {
	maxTok := int64(req.MaxTokens)
	if maxTok == 0 {
		maxTok = defaultMaxTokens
	}

	params := sdk.MessageNewParams{
		Model:     req.Model,
		MaxTokens: maxTok,
	}

	// Extract the first system message into the top-level System field.
	for _, msg := range req.Messages {
		if msg.Role == llm.RoleSystem {
			params.System = []sdk.TextBlockParam{{Text: msg.Content}}
			break
		}
	}

	if len(req.Tools) > 0 {
		params.Tools = buildTools(req.Tools)
	}

	msgs, err := buildMessages(req.Messages)
	if err != nil {
		return sdk.MessageNewParams{}, err
	}
	params.Messages = msgs

	return params, nil
}

// buildTools converts llm.Tool definitions to SDK tool params.
func buildTools(tools []llm.Tool) []sdk.ToolUnionParam {
	result := make([]sdk.ToolUnionParam, len(tools))
	for i, t := range tools {
		result[i] = sdk.ToolUnionParam{
			OfTool: &sdk.ToolParam{
				Name:        t.Function.Name,
				Description: param.NewOpt(t.Function.Description),
				InputSchema: buildInputSchema(t.Function.Parameters),
			},
		}
	}
	return result
}

// buildInputSchema converts a JSON Schema map to an SDK ToolInputSchemaParam.
func buildInputSchema(parameters map[string]any) sdk.ToolInputSchemaParam {
	if parameters == nil {
		return sdk.ToolInputSchemaParam{}
	}
	schema := sdk.ToolInputSchemaParam{
		Properties: parameters["properties"],
	}
	switch req := parameters["required"].(type) {
	case []string:
		schema.Required = req
	case []any:
		for _, r := range req {
			if s, ok := r.(string); ok {
				schema.Required = append(schema.Required, s)
			}
		}
	}
	return schema
}

// buildMessages converts llm.Message slice to SDK MessageParam slice,
// skipping system messages (handled separately) and grouping consecutive
// tool-result messages into a single user turn (Anthropic requirement).
func buildMessages(msgs []llm.Message) ([]sdk.MessageParam, error) {
	var result []sdk.MessageParam
	for i := 0; i < len(msgs); {
		msg := msgs[i]
		switch msg.Role {
		case llm.RoleSystem:
			i++
		case llm.RoleUser:
			result = append(result, sdk.NewUserMessage(
				sdk.ContentBlockParamUnion{OfText: &sdk.TextBlockParam{Text: msg.Content}},
			))
			i++
		case llm.RoleAssistant:
			result = append(result, buildAssistantMessage(msg))
			i++
		case llm.RoleTool:
			// Anthropic requires all consecutive tool results to be batched
			// into a single user turn.
			var blocks []sdk.ContentBlockParamUnion
			for i < len(msgs) && msgs[i].Role == llm.RoleTool {
				blocks = append(blocks, sdk.ContentBlockParamUnion{
					OfToolResult: &sdk.ToolResultBlockParam{
						ToolUseID: msgs[i].ToolCallID,
						Content: []sdk.ToolResultBlockParamContentUnion{
							{OfText: &sdk.TextBlockParam{Text: msgs[i].Content}},
						},
					},
				})
				i++
			}
			result = append(result, sdk.NewUserMessage(blocks...))
		default:
			return nil, fmt.Errorf("unknown message role %q", msg.Role)
		}
	}
	return result, nil
}

// buildAssistantMessage converts an assistant llm.Message to an SDK MessageParam.
// Uses preserved content blocks from Extra for round-trips when available.
func buildAssistantMessage(msg llm.Message) sdk.MessageParam {
	if len(msg.ToolCalls) > 0 {
		if msg.Extra != nil {
			if raw, ok := msg.Extra["anthropic:content"].([]sdk.ContentBlockParamUnion); ok && len(raw) > 0 {
				return sdk.NewAssistantMessage(raw...)
			}
		}
		// Reconstruct tool_use blocks from ToolCalls when no preserved content exists.
		blocks := make([]sdk.ContentBlockParamUnion, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			var input any
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
			blocks[i] = sdk.ContentBlockParamUnion{
				OfToolUse: &sdk.ToolUseBlockParam{
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: input,
				},
			}
		}
		return sdk.NewAssistantMessage(blocks...)
	}
	return sdk.NewAssistantMessage(
		sdk.ContentBlockParamUnion{OfText: &sdk.TextBlockParam{Text: msg.Content}},
	)
}

// mapResponse converts an SDK Message to an llm.CompletionResponse.
func mapResponse(resp *sdk.Message) llm.CompletionResponse {
	msg := llm.Message{Role: llm.RoleAssistant}
	var textParts []string
	var toolCalls []llm.ToolCall
	var rawBlocks []sdk.ContentBlockParamUnion

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			toolCalls = append(toolCalls, llm.ToolCall{
				ID: block.ID,
				Function: llm.ToolCallFunction{
					Name:      block.Name,
					Arguments: string(block.Input),
				},
			})
			// Preserve the content block for round-trips (re-used in buildAssistantMessage).
			var inputAny any
			_ = json.Unmarshal(block.Input, &inputAny)
			rawBlocks = append(rawBlocks, sdk.ContentBlockParamUnion{
				OfToolUse: &sdk.ToolUseBlockParam{
					ID:    block.ID,
					Name:  block.Name,
					Input: inputAny,
				},
			})
		}
	}

	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
		msg.Extra = map[string]any{"anthropic:content": rawBlocks}
	} else {
		msg.Content = strings.Join(textParts, "")
	}

	return llm.CompletionResponse{
		Message: msg,
		Usage: llm.TokenUsage{
			PromptTokens:     int(resp.Usage.InputTokens),
			CompletionTokens: int(resp.Usage.OutputTokens),
		},
		StopReason: mapStopReason(resp.StopReason),
	}
}

// mapStopReason converts Anthropic stop reasons to canonical strings.
func mapStopReason(reason sdk.StopReason) string {
	switch reason {
	case sdk.StopReasonToolUse:
		return "tool_calls"
	case sdk.StopReasonMaxTokens:
		return "length"
	default:
		return "stop"
	}
}

// mapErr converts SDK API errors to llm sentinel errors.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return llm.ErrUnauthorized
		case http.StatusNotFound:
			return llm.ErrModelNotFound
		case http.StatusTooManyRequests:
			return fmt.Errorf("%w: %s", llm.ErrRateLimit, apiErr.Error())
		case http.StatusBadRequest:
			msg := strings.ToLower(apiErr.Error())
			if strings.Contains(msg, "context") || strings.Contains(msg, "token") || strings.Contains(msg, "length") {
				return llm.ErrContextTooLong
			}
		}
	}
	return err
}
