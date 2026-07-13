package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"auris/pkg/llm"
)

const (
	defaultBaseURL     = "http://localhost:11434"
	defaultHTTPTimeout = 30 * time.Second
	// defaultNumCtx is the per-request context window passed to Ollama via
	// options.num_ctx. Ollama's own default is 2048-4096, which truncates
	// tool-calling conversations on the first follow-up turn. 32768 gives
	// generous headroom while keeping KV-cache RAM use moderate on small
	// models (~2GB for a 2B-param model). Override with AURIS_OLLAMA_NUM_CTX.
	defaultNumCtx = 32768
)

// Driver implements llm.AIProvider against the Ollama REST API.
type Driver struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	numCtx     int
	connected  bool
	mu         sync.RWMutex
}

// Option configures a Driver.
type Option func(*Driver)

func WithBaseURL(u string) Option          { return func(d *Driver) { d.baseURL = u } }
func WithHTTPClient(c *http.Client) Option { return func(d *Driver) { d.httpClient = c } }
func WithTimeout(t time.Duration) Option   { return func(d *Driver) { d.httpClient.Timeout = t } }

// WithAPIKey sets an optional Bearer token used when connecting to a protected
// remote Ollama instance. Leave empty for local (unauthenticated) Ollama.
func WithAPIKey(k string) Option { return func(d *Driver) { d.apiKey = k } }

// WithContextSize overrides the per-request num_ctx sent to Ollama. Values <= 0
// are ignored (the default is kept). See [defaultNumCtx] for the rationale.
func WithContextSize(n int) Option {
	return func(d *Driver) {
		if n > 0 {
			d.numCtx = n
		}
	}
}

// ContextWindow implements llm.ContextWindowReporter. It reports the
// configured effective num_ctx rather than a per-model native maximum: Ollama
// truncates every request to numCtx regardless of what the model natively
// supports, so this is the number that actually bounds usable context.
func (d *Driver) ContextWindow(model string) int { return d.numCtx }

// New constructs a Driver. All options are optional; sensible defaults are applied.
func New(opts ...Option) *Driver {
	d := &Driver{
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
		numCtx:     defaultNumCtx,
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Name returns the display name of this AI provider.
func (d *Driver) Name() string { return "Ollama" }

// Description returns a prose description of this AI provider.
func (d *Driver) Description() string { return "Local LLM inference via Ollama" }

// Connect validates the connection by calling GET /api/tags.
func (d *Driver) Connect(ctx context.Context) error {
	var resp ollamaTagsResponse
	if err := d.doGet(ctx, "/api/tags", &resp); err != nil {
		return fmt.Errorf("ollama: Connect: %w", err)
	}
	d.mu.Lock()
	d.connected = true
	d.mu.Unlock()
	return nil
}

// Disconnect marks the driver as disconnected. No network call is made.
func (d *Driver) Disconnect(_ context.Context) error {
	d.mu.Lock()
	d.connected = false
	d.mu.Unlock()
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
	if err := d.checkConnected(); err != nil {
		return err
	}
	var resp ollamaTagsResponse
	if err := d.doGet(ctx, "/api/tags", &resp); err != nil {
		return fmt.Errorf("ollama: Ping: %w", err)
	}
	return nil
}

// ListModels returns all models available on this Ollama instance.
func (d *Driver) ListModels(ctx context.Context) ([]llm.Model, error) {
	if err := d.checkConnected(); err != nil {
		return nil, err
	}
	var resp ollamaTagsResponse
	if err := d.doGet(ctx, "/api/tags", &resp); err != nil {
		return nil, fmt.Errorf("ollama: ListModels: %w", err)
	}
	models := make([]llm.Model, len(resp.Models))
	for i, m := range resp.Models {
		models[i] = llm.Model{ID: m.Name, Name: m.Name, Size: m.Size}
	}
	return models, nil
}

// Complete sends a non-streaming completion request and returns the full response.
func (d *Driver) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	if err := d.checkConnected(); err != nil {
		return llm.CompletionResponse{}, err
	}

	body, err := json.Marshal(d.buildChatRequest(req, false))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: Complete: marshal: %w", err)
	}

	var chatResp ollamaChatResponse
	if err := d.doPost(ctx, "/api/chat", body, &chatResp); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: Complete: %w", err)
	}

	return llm.CompletionResponse{
		Message: mapMessage(chatResp.Message),
		Usage: llm.TokenUsage{
			PromptTokens:     chatResp.PromptEvalCount,
			CompletionTokens: chatResp.EvalCount,
		},
		StopReason: stopReason(chatResp),
	}, nil
}

// Stream sends a streaming completion request and returns a channel of chunks.
// The channel is always closed after a Done==true chunk, even on error or cancellation.
func (d *Driver) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamChunk, error) {
	if err := d.checkConnected(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(d.buildChatRequest(req, true))
	if err != nil {
		return nil, fmt.Errorf("ollama: Stream: marshal: %w", err)
	}

	ch := make(chan llm.StreamChunk, 16)
	go func() {
		defer close(ch)

		resp, err := d.doStream(ctx, "/api/chat", body)
		if err != nil {
			ch <- llm.StreamChunk{Done: true, Err: fmt.Errorf("ollama: Stream: %w", err)}
			return
		}
		defer resp.Body.Close()

		// Close the body when ctx is cancelled so scanner.Scan() unblocks immediately
		// instead of waiting for the next token from a still-running Ollama model.
		bodyDone := make(chan struct{})
		defer close(bodyDone)
		go func() {
			select {
			case <-ctx.Done():
				resp.Body.Close()
			case <-bodyDone:
			}
		}()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 4096), 512*1024)

		var toolCalls []llm.ToolCall
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var frame ollamaChatResponse
			if err := json.Unmarshal(line, &frame); err != nil {
				ch <- llm.StreamChunk{Done: true, Err: fmt.Errorf("ollama: Stream: decode: %w", err)}
				return
			}

			// Ollama delivers tool calls as a complete object in a single frame
			// (no incremental JSON to assemble across frames), which may or may
			// not be the same frame that carries Done.
			if len(frame.Message.ToolCalls) > 0 {
				toolCalls = mapMessage(frame.Message).ToolCalls
			}

			chunk := llm.StreamChunk{
				Content: frame.Message.Content,
				Done:    frame.Done,
			}
			if frame.Done {
				chunk.Usage = llm.TokenUsage{
					PromptTokens:     frame.PromptEvalCount,
					CompletionTokens: frame.EvalCount,
				}
				if len(toolCalls) > 0 {
					chunk.ToolCalls = toolCalls
					chunk.StopReason = "tool_calls"
				} else {
					chunk.StopReason = "stop"
				}
			}
			ch <- chunk

			if frame.Done {
				return
			}
			if ctx.Err() != nil {
				ch <- llm.StreamChunk{Done: true, Err: ctx.Err()}
				return
			}
		}

		// Closing resp.Body on cancellation makes scanner.Err() return an I/O error.
		// Prefer ctx.Err() over the raw I/O error in that case.
		if err := scanner.Err(); err != nil {
			if ctx.Err() != nil {
				ch <- llm.StreamChunk{Done: true, Err: ctx.Err()}
			} else {
				ch <- llm.StreamChunk{Done: true, Err: fmt.Errorf("ollama: Stream: scan: %w", err)}
			}
		} else if ctx.Err() != nil {
			ch <- llm.StreamChunk{Done: true, Err: ctx.Err()}
		}
	}()

	return ch, nil
}

// checkConnected returns ErrNotConnected if the driver has not been connected.
func (d *Driver) checkConnected() error {
	d.mu.RLock()
	ok := d.connected
	d.mu.RUnlock()
	if !ok {
		return fmt.Errorf("ollama: %w", llm.ErrNotConnected)
	}
	return nil
}

// doGet performs a GET request and decodes the JSON response into dest.
func (d *Driver) doGet(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return mapHTTPError(resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

// doPost performs a POST request with a JSON body and decodes the JSON response into dest.
// It uses a no-timeout client (sharing the transport) because LLM inference can take
// arbitrarily long; cancellation is handled via the request context.
func (d *Driver) doPost(ctx context.Context, path string, body []byte, dest any) error {
	inferenceClient := &http.Client{Transport: d.httpClient.Transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}

	resp, err := inferenceClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return mapHTTPError(resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

// doStream performs a POST request and returns the response with the body still open.
// It uses a separate http.Client with no timeout so long-running streams are not cut off;
// cancellation is handled via the request context.
func (d *Driver) doStream(ctx context.Context, path string, body []byte) (*http.Response, error) {
	streamClient := &http.Client{Transport: d.httpClient.Transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}

	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, mapHTTPError(resp.StatusCode)
	}
	return resp, nil
}

// mapHTTPError converts HTTP status codes to llm sentinel errors.
func mapHTTPError(code int) error {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		return llm.ErrUnauthorized
	case http.StatusNotFound:
		return llm.ErrModelNotFound
	case http.StatusTooManyRequests:
		return llm.ErrRateLimit
	default:
		return fmt.Errorf("unexpected HTTP %d", code)
	}
}

// buildChatRequest converts a llm.CompletionRequest to the Ollama wire format.
// Driver-level settings (e.g. num_ctx) are merged into the request options.
func (d *Driver) buildChatRequest(req llm.CompletionRequest, stream bool) ollamaChatRequest {
	msgs := make([]ollamaMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = ollamaMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
		if len(m.ToolCalls) > 0 {
			calls := make([]ollamaToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				calls[j] = ollamaToolCall{
					Function: ollamaToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: json.RawMessage(tc.Function.Arguments),
					},
				}
			}
			msgs[i].ToolCalls = calls
		}
	}

	var tools []ollamaTool
	if len(req.Tools) > 0 {
		tools = make([]ollamaTool, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = ollamaTool{
				Type: t.Type,
				Function: ollamaToolFunction{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					Parameters:  t.Function.Parameters,
				},
			}
		}
	}

	var opts *ollamaOptions
	if d.numCtx > 0 {
		opts = &ollamaOptions{NumCtx: d.numCtx}
	}

	return ollamaChatRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   stream,
		Tools:    tools,
		Options:  opts,
	}
}

// mapMessage converts an ollamaMessage to a llm.Message.
func mapMessage(m ollamaMessage) llm.Message {
	msg := llm.Message{
		Role:    llm.Role(m.Role),
		Content: m.Content,
	}
	if len(m.ToolCalls) > 0 {
		calls := make([]llm.ToolCall, len(m.ToolCalls))
		for i, tc := range m.ToolCalls {
			calls[i] = llm.ToolCall{
				Function: llm.ToolCallFunction{
					Name:      tc.Function.Name,
					Arguments: string(tc.Function.Arguments),
				},
			}
		}
		msg.ToolCalls = calls
	}
	return msg
}

// stopReason infers the stop reason from a completed Ollama response.
func stopReason(resp ollamaChatResponse) string {
	if len(resp.Message.ToolCalls) > 0 {
		return "tool_calls"
	}
	return "stop"
}

// ── Internal JSON types ──────────────────────────────────────────────────────

type ollamaTagsResponse struct {
	Models []ollamaModelEntry `json:"models"`
}

type ollamaModelEntry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Options  *ollamaOptions  `json:"options,omitempty"`
}

// ollamaOptions maps to the Ollama API's per-request options block. Only the
// fields we actually use are declared.
type ollamaOptions struct {
	// NumCtx is the context window in tokens. Ollama's default is small
	// (2048-4096) and truncates the conversation; we override per request.
	NumCtx int `json:"num_ctx,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaToolCallFunction struct {
	Name string `json:"name"`
	// Arguments may arrive as a JSON string or a JSON object depending on
	// the Ollama version; using json.RawMessage handles both transparently.
	Arguments json.RawMessage `json:"arguments"`
}

type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ollamaChatResponse struct {
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}
