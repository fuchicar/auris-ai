package airegistry

import (
	"auris/pkg/ai"
	"auris/pkg/aidrivers/gemini"
	"auris/pkg/aidrivers/ollama"
)

// Entry describes a registered AI provider.
type Entry struct {
	// Key is the stable lowercase identifier stored in the config (e.g. "ollama").
	Key string
	// DisplayName is the human-readable provider name shown in the UI.
	DisplayName string
	// New constructs a fresh driver instance from the stored baseURL and apiKey.
	// Either may be empty; each driver applies its own defaults.
	New func(baseURL, apiKey string) ai.AIProvider
}

// All returns the ordered list of all registered AI provider entries.
func All() []Entry {
	return []Entry{
		{
			Key:         "ollama",
			DisplayName: "Ollama (local)",
			New: func(baseURL, apiKey string) ai.AIProvider {
				var opts []ollama.Option
				if baseURL != "" {
					opts = append(opts, ollama.WithBaseURL(baseURL))
				}
				if apiKey != "" {
					opts = append(opts, ollama.WithAPIKey(apiKey))
				}
				return ollama.New(opts...)
			},
		},
		{
			Key:         "gemini",
			DisplayName: "Google Gemini",
			New: func(baseURL, apiKey string) ai.AIProvider {
				var opts []gemini.Option
				if baseURL != "" {
					opts = append(opts, gemini.WithBaseURL(baseURL))
				}
				return gemini.New(apiKey, opts...)
			},
		},
	}
}
