package minimax

import (
	"context"
	"net/http"

	"auris/pkg/drivers/anthropic"
	"auris/pkg/llm"
)

const defaultBaseURL = "https://api.minimax.io/anthropic"

// Option configures a Driver. It is an alias of anthropic.Option so callers
// can use minimax.WithBaseURL / minimax.WithHTTPClient without conversion.
type Option = anthropic.Option

// WithBaseURL overrides the MiniMax API base URL.
var WithBaseURL = anthropic.WithBaseURL

// WithHTTPClient injects a custom HTTP client.
var WithHTTPClient func(*http.Client) Option = anthropic.WithHTTPClient

// Driver implements llm.AIProvider for MiniMax models by delegating to the
// Anthropic driver with the MiniMax base URL pre-configured.
type Driver struct {
	inner *anthropic.Driver
}

// New creates a MiniMax driver. The default base URL points to the MiniMax
// Anthropic-compatible endpoint; opts can override it via WithBaseURL.
func New(apiKey string, opts ...Option) *Driver {
	all := append([]Option{WithBaseURL(defaultBaseURL)}, opts...)
	return &Driver{inner: anthropic.New(apiKey, all...)}
}

func (d *Driver) Name() string        { return "MiniMax" }
func (d *Driver) Description() string { return "MiniMax models via Anthropic-compatible API" }

func (d *Driver) Connect(ctx context.Context) error    { return d.inner.Connect(ctx) }
func (d *Driver) Disconnect(ctx context.Context) error { return d.inner.Disconnect(ctx) }
func (d *Driver) IsConnected() bool                    { return d.inner.IsConnected() }
func (d *Driver) Ping(ctx context.Context) error       { return d.inner.Ping(ctx) }

func (d *Driver) ListModels(ctx context.Context) ([]llm.Model, error) {
	return d.inner.ListModels(ctx)
}

func (d *Driver) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	return d.inner.Complete(ctx, req)
}

func (d *Driver) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamChunk, error) {
	return d.inner.Stream(ctx, req)
}
