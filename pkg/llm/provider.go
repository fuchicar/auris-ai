package llm

import "context"

// AIProvider is the interface that all AI backend drivers must implement.
type AIProvider interface {
	Name() string
	Description() string

	// Connect validates the connection to the provider. Must be called before
	// Complete, Stream, or ListModels.
	Connect(ctx context.Context) error

	// Disconnect releases resources held by the driver.
	Disconnect(ctx context.Context) error

	// IsConnected reports whether the provider is in a connected state.
	IsConnected() bool

	// Ping checks that the connection is still alive without modifying state.
	Ping(ctx context.Context) error

	// ListModels returns all models available on this provider.
	ListModels(ctx context.Context) ([]Model, error)

	// Complete sends a completion request and blocks until the full response arrives.
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)

	// Stream sends a completion request and returns a channel of StreamChunk values.
	// The channel is always closed after a chunk with Done == true is sent, even on
	// cancellation, so callers can safely range over it without leaking goroutines.
	// Setup-time errors (e.g. not connected) are returned as the error return value.
	// Runtime streaming errors are carried inside StreamChunk.Err.
	Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)
}

// ContextWindowReporter is an optional interface a driver can implement to
// report a model's token context window synchronously, with no network call.
// Returns 0 if the model is unrecognized or the window is unknown. Mirrors
// the market.CapabilityReporter optional-interface pattern.
type ContextWindowReporter interface {
	ContextWindow(model string) int
}
