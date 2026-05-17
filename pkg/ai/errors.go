package ai

import "errors"

var (
	ErrNotConnected   = errors.New("AI provider is not connected")
	ErrUnauthorized   = errors.New("invalid or missing AI provider credentials")
	ErrModelNotFound  = errors.New("model not found on AI provider")
	ErrContextTooLong = errors.New("prompt exceeds model context window")
	ErrRateLimit      = errors.New("AI provider rate limit exceeded")
	ErrNotSupported   = errors.New("operation not supported by this AI provider")
)
