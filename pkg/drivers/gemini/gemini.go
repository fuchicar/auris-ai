package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"auris/pkg/llm"
)

const (
	defaultBaseURL     = "https://generativelanguage.googleapis.com"
	defaultHTTPTimeout = 30 * time.Second
	apiVersion         = "/v1beta"
)

// Driver implements llm.AIProvider against the Google Generative AI (Gemini) REST API.
type Driver struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	connected  bool
	mu         sync.RWMutex
}

// Option configures a Driver.
type Option func(*Driver)

func WithBaseURL(u string) Option          { return func(d *Driver) { d.baseURL = u } }
func WithHTTPClient(c *http.Client) Option { return func(d *Driver) { d.httpClient = c } }
func WithTimeout(t time.Duration) Option   { return func(d *Driver) { d.httpClient.Timeout = t } }

// New constructs a Driver. apiKey is required; it is validated on Connect.
func New(apiKey string, opts ...Option) *Driver {
	d := &Driver{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Name returns the display name of this AI provider.
func (d *Driver) Name() string { return "Google Gemini" }

// Description returns a prose description of this AI provider.
func (d *Driver) Description() string {
	return "Google's Gemini language models via the Generative AI API"
}

// Connect validates the API key by listing available models.
func (d *Driver) Connect(ctx context.Context) error {
	var resp geminiModelList
	if err := d.doGet(ctx, apiVersion+"/models", &resp); err != nil {
		return fmt.Errorf("gemini: Connect: %w", err)
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
	var resp geminiModelList
	if err := d.doGet(ctx, apiVersion+"/models", &resp); err != nil {
		return fmt.Errorf("gemini: Ping: %w", err)
	}
	return nil
}

// ListModels returns all Gemini models that support content generation.
func (d *Driver) ListModels(ctx context.Context) ([]llm.Model, error) {
	if err := d.checkConnected(); err != nil {
		return nil, err
	}
	var resp geminiModelList
	if err := d.doGet(ctx, apiVersion+"/models", &resp); err != nil {
		return nil, fmt.Errorf("gemini: ListModels: %w", err)
	}
	var models []llm.Model
	for _, m := range resp.Models {
		if !supportsGenerateContent(m) {
			continue
		}
		models = append(models, llm.Model{ID: m.Name, Name: m.DisplayName})
	}
	return models, nil
}

// Complete sends a non-streaming completion request and returns the full response.
func (d *Driver) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	if err := d.checkConnected(); err != nil {
		return llm.CompletionResponse{}, err
	}

	body, err := json.Marshal(buildRequest(req))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: Complete: marshal: %w", err)
	}

	url := modelURL(d.baseURL, req.Model, "generateContent")
	var gemResp geminiResponse
	if err := d.doPost(ctx, url, body, &gemResp); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: Complete: %w", err)
	}

	result, err := mapResponse(gemResp)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: Complete: %w", err)
	}
	return result, nil
}

// Stream sends a streaming completion request and returns a channel of chunks.
// The channel is always closed after a Done==true chunk, even on error or cancellation.
func (d *Driver) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamChunk, error) {
	if err := d.checkConnected(); err != nil {
		return nil, err
	}

	body, err := json.Marshal(buildRequest(req))
	if err != nil {
		return nil, fmt.Errorf("gemini: Stream: marshal: %w", err)
	}

	// Append ?alt=sse to receive Server-Sent Events format.
	url := modelURL(d.baseURL, req.Model, "streamGenerateContent") + "?alt=sse"

	ch := make(chan llm.StreamChunk, 16)
	go func() {
		defer close(ch)

		resp, err := d.doStream(ctx, url, body)
		if err != nil {
			ch <- llm.StreamChunk{Done: true, Err: fmt.Errorf("gemini: Stream: %w", err)}
			return
		}
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 4096), 512*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			jsonData := line[len("data: "):]

			var frame geminiResponse
			if err := json.Unmarshal([]byte(jsonData), &frame); err != nil {
				ch <- llm.StreamChunk{Done: true, Err: fmt.Errorf("gemini: Stream: decode: %w", err)}
				return
			}

			if len(frame.Candidates) == 0 {
				continue
			}

			cand := frame.Candidates[0]
			content := extractText(cand.Content.Parts)
			done := cand.FinishReason != ""

			chunk := llm.StreamChunk{Content: content, Done: done}
			if done {
				chunk.Usage = llm.TokenUsage{
					PromptTokens:     frame.UsageMetadata.PromptTokenCount,
					CompletionTokens: frame.UsageMetadata.CandidatesTokenCount,
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

		if err := scanner.Err(); err != nil {
			ch <- llm.StreamChunk{Done: true, Err: fmt.Errorf("gemini: Stream: scan: %w", err)}
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
		return fmt.Errorf("gemini: %w", llm.ErrNotConnected)
	}
	return nil
}

// doGet performs a GET request and decodes the JSON response into dest.
func (d *Driver) doGet(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("x-goog-api-key", d.apiKey)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return mapHTTPError(resp.StatusCode, body)
	}
	return json.Unmarshal(body, dest)
}

// doPost performs a POST request with a JSON body and decodes the JSON response into dest.
// Uses a no-timeout client because LLM inference can take arbitrarily long.
func (d *Driver) doPost(ctx context.Context, url string, body []byte, dest any) error {
	inferenceClient := &http.Client{Transport: d.httpClient.Transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", d.apiKey)

	resp, err := inferenceClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return mapHTTPError(resp.StatusCode, respBody)
	}
	return json.Unmarshal(respBody, dest)
}

// doStream performs a POST request and returns the response with the body still open.
// Uses a separate client with no timeout; cancellation is via the request context.
func (d *Driver) doStream(ctx context.Context, url string, body []byte) (*http.Response, error) {
	streamClient := &http.Client{Transport: d.httpClient.Transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", d.apiKey)

	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, mapHTTPError(resp.StatusCode, errBody)
	}
	return resp, nil
}

// mapHTTPError converts a Gemini HTTP error response to a sentinel error.
func mapHTTPError(code int, body []byte) error {
	var errResp geminiErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		msg := errResp.Error.Message
		switch code {
		case http.StatusBadRequest:
			if strings.Contains(strings.ToLower(msg), "token") {
				return llm.ErrContextTooLong
			}
			return fmt.Errorf("gemini: bad request: %s", msg)
		case http.StatusUnauthorized, http.StatusForbidden:
			return llm.ErrUnauthorized
		case http.StatusNotFound:
			return llm.ErrModelNotFound
		case http.StatusTooManyRequests:
			return llm.ErrRateLimit
		default:
			return fmt.Errorf("gemini: HTTP %d: %s", code, msg)
		}
	}
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		return llm.ErrUnauthorized
	case http.StatusNotFound:
		return llm.ErrModelNotFound
	case http.StatusTooManyRequests:
		return llm.ErrRateLimit
	default:
		return fmt.Errorf("gemini: unexpected HTTP %d", code)
	}
}

// modelURL builds the full Gemini API URL for inference methods.
// model may or may not include the "models/" prefix.
func modelURL(baseURL, model, method string) string {
	if !strings.HasPrefix(model, "models/") {
		model = "models/" + model
	}
	return baseURL + apiVersion + "/" + model + ":" + method
}

// supportsGenerateContent reports whether a model supports the generateContent method.
func supportsGenerateContent(m geminiModelEntry) bool {
	return slices.Contains(m.SupportedGenerationMethods, "generateContent")
}

// buildRequest converts a llm.CompletionRequest into the Gemini wire format.
func buildRequest(req llm.CompletionRequest) geminiRequest {
	gr := geminiRequest{}

	for _, msg := range req.Messages {
		switch msg.Role {
		case llm.RoleSystem:
			gr.SystemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: msg.Content}},
			}
		case llm.RoleUser:
			gr.Contents = append(gr.Contents, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: msg.Content}},
			})
		case llm.RoleAssistant:
			content := geminiContent{Role: "model"}
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					var args map[string]any
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
					content.Parts = append(content.Parts, geminiPart{
						FunctionCall: &geminiFunctionCall{
							Name: tc.Function.Name,
							Args: args,
						},
					})
				}
			} else {
				content.Parts = []geminiPart{{Text: msg.Content}}
			}
			gr.Contents = append(gr.Contents, content)
		case llm.RoleTool:
			// ToolCallID holds the function name in Gemini's model (Gemini uses
			// function names as identifiers rather than opaque call IDs).
			gr.Contents = append(gr.Contents, geminiContent{
				Role: "user",
				Parts: []geminiPart{{
					FunctionResponse: &geminiFunctionResp{
						Name:     msg.ToolCallID,
						Response: map[string]any{"result": msg.Content},
					},
				}},
			})
		}
	}

	if len(req.Tools) > 0 {
		decls := make([]geminiFunctionDecl, len(req.Tools))
		for i, t := range req.Tools {
			decls[i] = geminiFunctionDecl{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			}
		}
		gr.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}

	if req.MaxTokens > 0 {
		gr.GenerationConfig = &geminiGenConfig{MaxOutputTokens: req.MaxTokens}
	}

	return gr
}

// mapResponse converts a Gemini response into a llm.CompletionResponse.
func mapResponse(resp geminiResponse) (llm.CompletionResponse, error) {
	if len(resp.Candidates) == 0 {
		return llm.CompletionResponse{}, fmt.Errorf("empty candidates in response")
	}
	cand := resp.Candidates[0]

	msg := llm.Message{Role: llm.RoleAssistant}

	var toolCalls []llm.ToolCall
	var textParts []string
	for _, p := range cand.Content.Parts {
		switch {
		case p.FunctionCall != nil:
			argsJSON, _ := json.Marshal(p.FunctionCall.Args)
			toolCalls = append(toolCalls, llm.ToolCall{
				ID: p.FunctionCall.Name,
				Function: llm.ToolCallFunction{
					Name:      p.FunctionCall.Name,
					Arguments: string(argsJSON),
				},
			})
		case p.Text != "":
			textParts = append(textParts, p.Text)
		}
	}

	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	} else {
		msg.Content = strings.Join(textParts, "")
	}

	stopReason := mapFinishReason(cand.FinishReason, len(toolCalls) > 0)

	return llm.CompletionResponse{
		Message: msg,
		Usage: llm.TokenUsage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
		},
		StopReason: stopReason,
	}, nil
}

// extractText concatenates all text parts from a slice of geminiPart.
func extractText(parts []geminiPart) string {
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
	}
	return sb.String()
}

// mapFinishReason converts Gemini's finish reason to our canonical stop reason strings.
func mapFinishReason(reason string, hasToolCalls bool) string {
	if hasToolCalls {
		return "tool_calls"
	}
	switch reason {
	case "MAX_TOKENS":
		return "length"
	default:
		return "stop"
	}
}

// ── Internal JSON types ──────────────────────────────────────────────────────

type geminiModelList struct {
	Models []geminiModelEntry `json:"models"`
}

type geminiModelEntry struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

type geminiRequest struct {
	Contents          []geminiContent  `json:"contents"`
	Tools             []geminiTool     `json:"tools,omitempty"`
	GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`
	SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall   `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResp   `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiFunctionResp struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type geminiGenConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata geminiUsage       `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
}

type geminiErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}
