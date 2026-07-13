package registry

import (
	"os"
	"strconv"

	"auris/pkg/drivers/anthropic"
	"auris/pkg/drivers/gemini"
	"auris/pkg/drivers/minimax"
	"auris/pkg/drivers/ollama"
	"auris/pkg/drivers/openai"
	"auris/pkg/llm"
)

// envOllamaNumCtx, if set to a positive integer, overrides the Ollama driver's
// default context window (num_ctx). Documented in `auris -h`.
const envOllamaNumCtx = "AURIS_OLLAMA_NUM_CTX"

// envOpenAIBaseURL and envOpenAIAPIKey mirror the environment variable
// convention used by OpenAI's own SDKs (no AURIS_ prefix, deliberately), so
// any OpenAI-compatible third party (MiniMax, DeepSeek, Groq, OpenRouter, a
// self-hosted proxy, ...) that a user already has configured via these
// variables works with the openai driver without extra setup. They are only
// used as a fallback when the corresponding field is left empty in Auris's
// own config. Documented in `auris -h`.
const (
	envOpenAIBaseURL = "OPENAI_BASE_URL"
	envOpenAIAPIKey  = "OPENAI_API_KEY"
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
		{
			Key:         "minimax",
			DisplayName: "MiniMax",
			New: func(baseURL, apiKey string) llm.AIProvider {
				var opts []minimax.Option
				if baseURL != "" {
					opts = append(opts, minimax.WithBaseURL(baseURL))
				}
				return minimax.New(apiKey, opts...)
			},
		},
		{
			Key:         "openai",
			DisplayName: "OpenAI",
			New: func(baseURL, apiKey string) llm.AIProvider {
				var opts []openai.Option
				effectiveBaseURL := baseURL
				if effectiveBaseURL == "" {
					effectiveBaseURL = os.Getenv(envOpenAIBaseURL)
				}
				if effectiveBaseURL != "" {
					opts = append(opts, openai.WithBaseURL(effectiveBaseURL))
				}
				effectiveAPIKey := apiKey
				if effectiveAPIKey == "" {
					effectiveAPIKey = os.Getenv(envOpenAIAPIKey)
				}
				return openai.New(effectiveAPIKey, opts...)
			},
		},
		{
			Key:         "openai_compatible",
			DisplayName: "OpenAI-Compatible",
			// Deliberately no OPENAI_BASE_URL/OPENAI_API_KEY fallback here — this
			// entry exists precisely so a third-party endpoint can be configured
			// independently of (and without colliding with) the plain "openai"
			// entry above. Both baseURL and apiKey must come from Auris's own
			// config (set via the setup wizard, where base URL is a required
			// field for this provider — see screen_ai_config.go).
			New: func(baseURL, apiKey string) llm.AIProvider {
				var opts []openai.Option
				if baseURL != "" {
					opts = append(opts, openai.WithBaseURL(baseURL))
				}
				return openai.New(apiKey, opts...)
			},
		},
	}
}
