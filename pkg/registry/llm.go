package registry

import (
	"auris/pkg/drivers/gemini"
	"auris/pkg/drivers/ollama"
	"auris/pkg/llm"
)

// LLMEntry describes a registered AI provider.
type LLMEntry struct {
	// Key is the stable lowercase identifier stored in the config (e.g. "ollama").
	Key string
	// DisplayName is the human-readable provider name shown in the UI.
	DisplayName string
	// New constructs a fresh driver instance from the stored baseURL and apiKey.
	// Either may be empty; each driver applies its own defaults.
	New func(baseURL, apiKey string) llm.AIProvider
}

// AllLLM returns the ordered list of all registered AI provider entries.
func AllLLM() []LLMEntry {
	return []LLMEntry{
		{
			Key:         "ollama",
			DisplayName: "Ollama (local)",
			New: func(baseURL, apiKey string) llm.AIProvider {
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
			New: func(baseURL, apiKey string) llm.AIProvider {
				var opts []gemini.Option
				if baseURL != "" {
					opts = append(opts, gemini.WithBaseURL(baseURL))
				}
				return gemini.New(apiKey, opts...)
			},
		},
	}
}
