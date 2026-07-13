package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	sdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"

	"auris/pkg/llm"
)

// Option configures a Driver.
type Option func(*Driver)

// WithBaseURL overrides the OpenAI API base URL. Used to point the driver at
// any OpenAI-compatible endpoint (MiniMax, DeepSeek, Groq, OpenRouter, a
// self-hosted proxy, ...).
func WithBaseURL(u string) Option {
	return func(d *Driver) { d.baseURL = u }
}

// WithHTTPClient injects a custom HTTP client into the SDK.
func WithHTTPClient(c *http.Client) Option {
	return func(d *Driver) { d.httpClient = c }
}

// Driver implements llm.AIProvider for OpenAI's GPT models, and any
// third-party service that copies OpenAI's Chat Completions wire format.
type Driver struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	client     *sdk.Client
	connected  bool
	mu         sync.RWMutex
}

// New creates a new OpenAI driver. The API key is required; options are optional.
func New(apiKey string, opts ...Option) *Driver {
	d := &Driver{apiKey: apiKey}
	for _, o := range opts {
		o(d)
	}
	return d
}

func (d *Driver) Name() string { return "OpenAI" }
func (d *Driver) Description() string {
	return "OpenAI's GPT models (and OpenAI-compatible APIs) via the Chat Completions API"
}

// Connect validates the API key by listing available models.
func (d *Driver) Connect(ctx context.Context) error {
	cliOpts := []option.RequestOption{option.WithAPIKey(d.apiKey)}
	if d.baseURL != "" {
		cliOpts = append(cliOpts, option.WithBaseURL(d.baseURL))
	}
	if d.httpClient != nil {
		cliOpts = append(cliOpts, option.WithHTTPClient(d.httpClient))
	}

	client := sdk.NewClient(cliOpts...)
	if _, err := client.Models.List(ctx); err != nil {
		return fmt.Errorf("openai: Connect: %w", mapErr(err))
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
	if _, err := client.Models.List(ctx); err != nil {
		return fmt.Errorf("openai: Ping: %w", mapErr(err))
	}
	return nil
}

// ListModels returns all models available on this provider.
func (d *Driver) ListModels(ctx context.Context) ([]llm.Model, error) {
	client, err := d.getClient()
	if err != nil {
		return nil, err
	}
	pager := client.Models.ListAutoPaging(ctx)
	var models []llm.Model
	for pager.Next() {
		m := pager.Current()
		// The OpenAI wire format has no separate display name, unlike Anthropic's
		// DisplayName field; mirrors ollama.go's ListModels (Name == ID).
		models = append(models, llm.Model{ID: m.ID, Name: m.ID})
	}
	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("openai: ListModels: %w", mapErr(err))
	}
	return models, nil
}

// Complete sends a blocking completion request and returns the full response.
func (d *Driver) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	client, err := d.getClient()
	if err != nil {
		return llm.CompletionResponse{}, err
	}
	params, err := buildParams(req)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("openai: Complete: %w", err)
	}
	resp, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("openai: Complete: %w", mapErr(err))
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
		return nil, fmt.Errorf("openai: Stream: %w", err)
	}

	stream := client.Chat.Completions.NewStreaming(ctx, params)
	ch := make(chan llm.StreamChunk, 64)

	go func() {
		defer close(ch)
		defer stream.Close()

		// acc accumulates the full ChatCompletion across chunks (including
		// tool_calls assembled from per-index deltas) using the SDK's own
		// accumulator, so mapResponse — the exact function Complete uses — can
		// be reused verbatim for the terminal chunk instead of re-deriving
		// tool-call parsing here.
		var acc sdk.ChatCompletionAccumulator

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			if len(chunk.Choices) == 0 {
				continue
			}
			if content := chunk.Choices[0].Delta.Content; content != "" {
				ch <- llm.StreamChunk{Content: content}
			}
			if chunk.Choices[0].FinishReason != "" {
				resp := mapResponse(&acc.ChatCompletion)
				ch <- llm.StreamChunk{
					Done:       true,
					Usage:      resp.Usage,
					StopReason: resp.StopReason,
					ToolCalls:  resp.Message.ToolCalls,
				}
				return
			}
		}

		if err := stream.Err(); err != nil {
			if ctx.Err() != nil {
				ch <- llm.StreamChunk{Done: true, Err: ctx.Err()}
			} else {
				ch <- llm.StreamChunk{Done: true, Err: fmt.Errorf("openai: Stream: %w", mapErr(err))}
			}
			return
		}

		// Defensive fallback: the stream ended without an explicit finish_reason.
		resp := mapResponse(&acc.ChatCompletion)
		ch <- llm.StreamChunk{
			Done:       true,
			Usage:      resp.Usage,
			StopReason: resp.StopReason,
			ToolCalls:  resp.Message.ToolCalls,
		}
	}()

	return ch, nil
}

func (d *Driver) getClient() (*sdk.Client, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if !d.connected || d.client == nil {
		return nil, fmt.Errorf("openai: %w", llm.ErrNotConnected)
	}
	return d.client, nil
}

// buildParams converts an llm.CompletionRequest to sdk.ChatCompletionNewParams.
func buildParams(req llm.CompletionRequest) (sdk.ChatCompletionNewParams, error) {
	params := sdk.ChatCompletionNewParams{
		Model: shared.ChatModel(req.Model),
	}

	// Unlike Anthropic, max_tokens is optional in Chat Completions; only send
	// it when the caller asked for a specific cap.
	if req.MaxTokens > 0 {
		params.MaxTokens = param.NewOpt(int64(req.MaxTokens))
	}

	if len(req.Tools) > 0 {
		params.Tools = buildTools(req.Tools)
	}

	msgs, err := buildMessages(req.Messages)
	if err != nil {
		return sdk.ChatCompletionNewParams{}, err
	}
	params.Messages = msgs

	return params, nil
}

// buildTools converts llm.Tool definitions to SDK tool params.
func buildTools(tools []llm.Tool) []sdk.ChatCompletionToolParam {
	result := make([]sdk.ChatCompletionToolParam, len(tools))
	for i, t := range tools {
		result[i] = sdk.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        t.Function.Name,
				Description: param.NewOpt(t.Function.Description),
				Parameters:  shared.FunctionParameters(t.Function.Parameters),
			},
		}
	}
	return result
}

// buildMessages converts an llm.Message slice to an SDK message param slice.
// Unlike Anthropic, Chat Completions has no top-level system field or
// tool-result batching requirement: every role maps to its own message in
// the array, in order.
func buildMessages(msgs []llm.Message) ([]sdk.ChatCompletionMessageParamUnion, error) {
	result := make([]sdk.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, msg := range msgs {
		switch msg.Role {
		case llm.RoleSystem:
			result = append(result, sdk.SystemMessage(msg.Content))
		case llm.RoleUser:
			result = append(result, sdk.UserMessage(msg.Content))
		case llm.RoleAssistant:
			result = append(result, buildAssistantMessage(msg))
		case llm.RoleTool:
			result = append(result, sdk.ToolMessage(msg.Content, msg.ToolCallID))
		default:
			return nil, fmt.Errorf("unknown message role %q", msg.Role)
		}
	}
	return result, nil
}

// buildAssistantMessage converts an assistant llm.Message to an SDK message
// param. A tool_call is fully described by ID/Name/Arguments, all of which
// already live on llm.ToolCall, so — unlike Anthropic — no Message.Extra
// round-trip payload is needed to reconstruct it.
func buildAssistantMessage(msg llm.Message) sdk.ChatCompletionMessageParamUnion {
	if len(msg.ToolCalls) > 0 {
		toolCalls := make([]sdk.ChatCompletionMessageToolCallParam, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			toolCalls[i] = sdk.ChatCompletionMessageToolCallParam{
				ID: tc.ID,
				Function: sdk.ChatCompletionMessageToolCallFunctionParam{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
		return sdk.ChatCompletionMessageParamUnion{
			OfAssistant: &sdk.ChatCompletionAssistantMessageParam{ToolCalls: toolCalls},
		}
	}
	return sdk.AssistantMessage(msg.Content)
}

// mapResponse converts an SDK ChatCompletion to an llm.CompletionResponse.
func mapResponse(resp *sdk.ChatCompletion) llm.CompletionResponse {
	usage := llm.TokenUsage{
		PromptTokens:     int(resp.Usage.PromptTokens),
		CompletionTokens: int(resp.Usage.CompletionTokens),
	}
	if len(resp.Choices) == 0 {
		return llm.CompletionResponse{
			Message:    llm.Message{Role: llm.RoleAssistant},
			Usage:      usage,
			StopReason: "stop",
		}
	}

	choice := resp.Choices[0]
	msg := llm.Message{Role: llm.RoleAssistant}

	if len(choice.Message.ToolCalls) > 0 {
		toolCalls := make([]llm.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			toolCalls[i] = llm.ToolCall{
				ID: tc.ID,
				Function: llm.ToolCallFunction{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
		msg.ToolCalls = toolCalls
	} else {
		msg.Content = choice.Message.Content
	}

	return llm.CompletionResponse{
		Message:    msg,
		Usage:      usage,
		StopReason: mapStopReason(choice.FinishReason),
	}
}

// mapStopReason converts OpenAI finish reasons to canonical strings.
func mapStopReason(reason string) string {
	switch reason {
	case "tool_calls":
		return "tool_calls"
	case "length":
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
