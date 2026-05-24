package registry

import (
	"os"
	"strconv"

	"auris/pkg/drivers/anthropic"
	"auris/pkg/drivers/gemini"
	"auris/pkg/drivers/ollama"
	"auris/pkg/llm"
)

// envOllamaNumCtx, if set to a positive integer, overrides the Ollama driver's
// default context window (num_ctx). Documented in `auris -h`.
const envOllamaNumCtx = "AURIS_OLLAMA_NUM_CTX"

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
			DisplayName: "Ollama",
			New: func(baseURL, apiKey string) llm.AIProvider {
				var opts []ollama.Option
				if baseURL != "" {
					opts = append(opts, ollama.WithBaseURL(baseURL))
				}
				if apiKey != "" {
					opts = append(opts, ollama.WithAPIKey(apiKey))
				}
				if v := os.Getenv(envOllamaNumCtx); v != "" {
					if n, err := strconv.Atoi(v); err == nil && n > 0 {
						opts = append(opts, ollama.WithContextSize(n))
					}
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
		{
			Key:         "anthropic",
			DisplayName: "Anthropic Claude",
			New: func(baseURL, apiKey string) llm.AIProvider {
				var opts []anthropic.Option
				if baseURL != "" {
					opts = append(opts, anthropic.WithBaseURL(baseURL))
				}
				return anthropic.New(apiKey, opts...)
			},
		},
	}
}
