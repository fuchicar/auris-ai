package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fuchicar/auris-ai/pkg/news"
)

// AurisConfig is the in-memory representation of the agent configuration.
// Sensitive fields (e.g. APIKey) are held as plaintext strings.
type AurisConfig struct {
	ActiveProvider string
	Providers      map[string]*ProviderConfig
	// ProviderOrder is the user's chosen market-provider priority order (all
	// registry.AllMarket() keys, active or not — deactivating a provider does
	// not lose its position). Empty means no preference has been saved yet;
	// callers fall back to registry.AllMarket()'s natural order.
	ProviderOrder    []string
	Locale           string            // BCP-47 tag, e.g. "en" or "es"; empty means auto-detected at runtime
	Theme            string            // "light" | "dark"
	FinancialProfile *FinancialProfile // nil until the setup questionnaire is completed

	// AI provider fields — all optional; zero value means AI is not yet configured.
	ActiveAIProvider string
	DefaultAIModel   string
	AIProviders      map[string]*AIProviderConfig
	AITaskRoutes     map[string]AITaskRoute // keyed by TaskType string

	// ChatHistory stores the persistent conversation log for agent mode.
	// Stored as plaintext (same policy as FinancialProfile).
	// Deprecated: kept for migration only; active sessions are stored in the
	// sessions directory. New code should use Session / ActiveSessionID.
	ChatHistory []ChatTurn

	// ActiveSessionID is the ID of the session file currently in use.
	ActiveSessionID string

	// NewsFeeds is the list of RSS/Atom feeds used by the fetch_news tool.
	// Stored as plaintext JSON (no credentials). Defaults to DefaultFeeds on
	// first run.
	NewsFeeds []news.FeedConfig

	// EncryptStorage, when true, encrypts portfolio and chat session files at
	// rest with a key derived from the user's passphrase (FEAT-16). Defaults
	// to true for new setups (opt-out, not opt-in) — see pkg/tui's
	// ScreenPassphrase transition. StorageSalt is generated once when first
	// enabled and, unlike the credential-encryption salt below (which
	// rotates on every Save), stays stable across saves so the derived key
	// doesn't change underneath already-encrypted files.
	EncryptStorage bool
	StorageSalt    []byte

	// SimulationMode, when true, replaces the real market-provider cascade
	// with the synthetic pkg/drivers/simulation driver (see buildMarketProvider
	// in pkg/tui/app.go) so the agent can be evaluated without any market data
	// API key. Real provider configs in Providers are preserved untouched so
	// disabling simulation mode restores them without re-entering credentials.
	SimulationMode bool
}

// ChatTurn is a single message in the agent-mode conversation history.
type ChatTurn struct {
	Role    string   `json:"role"` // "user" | "assistant"
	Content string   `json:"content"`
	Charts  []string `json:"charts,omitempty"` // pre-rendered ASCII chart blocks (with ANSI styling) attached to this turn
}

// FinancialProfile holds the user's financial background collected during the setup
// questionnaire. All fields are stored as plaintext JSON — they are not credentials.
type FinancialProfile struct {
	// Q1: single-select life-stage
	LifeStage string `json:"life_stage,omitempty"`
	// Q2: single-select income stability
	IncomeStability string `json:"income_stability,omitempty"`
	// Q3: single-select emergency fund status
	EmergencyFund string `json:"emergency_fund,omitempty"`
	// Q4: multi-select investment goals
	InvestmentGoals []string `json:"investment_goals,omitempty"`
	// Q4: free text when "other" is selected
	InvestmentGoalsOther string `json:"investment_goals_other,omitempty"`
	// Q5: single-select time horizon
	TimeHorizon string `json:"time_horizon,omitempty"`
	// Q6: single-select reaction to a 25% portfolio loss
	LossScenario string `json:"loss_scenario,omitempty"`
	// Q7: single-select maximum acceptable loss percentage
	MaxAcceptableLoss string `json:"max_acceptable_loss,omitempty"`
	// Q8: multi-select prior financial experience
	FinancialExperience []string `json:"financial_experience,omitempty"`
	// Q9: single-select investment priority
	InvestmentPriority string `json:"investment_priority,omitempty"`
	// Q10: multi-select investment restrictions
	Restrictions []string `json:"restrictions,omitempty"`
	// Q10: country name when "country_only" restriction is selected
	RestrictionsCountry string `json:"restrictions_country,omitempty"`
}

// ProviderConfig holds per-provider settings.
// APIKey is plaintext in memory; it is encrypted when written to disk.
type ProviderConfig struct {
	APIKey string
}

// AIProviderConfig holds per-AI-provider settings.
// BaseURL is stored unencrypted (contains no secrets).
// APIKey is plaintext in memory; it is encrypted when written to disk.
type AIProviderConfig struct {
	BaseURL string // e.g. "http://localhost:11434"; empty means provider default
	APIKey  string // optional; empty for local providers like Ollama
}

// AITaskRoute maps a TaskType string to a specific provider key and model ID.
type AITaskRoute struct {
	Provider string // key in AIProviders (e.g. "ollama")
	Model    string // model ID (e.g. "llama3.2:latest")
}

// diskConfig is the JSON-serializable shadow of AurisConfig.
type diskConfig struct {
	ActiveProvider   string                   `json:"active_provider"`
	KDF              diskKDF                  `json:"kdf"`
	Providers        map[string]*diskProvider `json:"providers,omitempty"`
	ProviderOrder    []string                 `json:"provider_order,omitempty"`
	Locale           string                   `json:"locale,omitempty"`
	Theme            string                   `json:"theme,omitempty"`
	FinancialProfile *FinancialProfile        `json:"financial_profile,omitempty"`

	ActiveAIProvider string                     `json:"active_ai_provider,omitempty"`
	DefaultAIModel   string                     `json:"default_ai_model,omitempty"`
	AIProviders      map[string]*diskAIProvider `json:"ai_providers,omitempty"`
	AITaskRoutes     map[string]AITaskRoute     `json:"ai_task_routes,omitempty"`
	ChatHistory      []ChatTurn                 `json:"chat_history,omitempty"`
	ActiveSessionID  string                     `json:"active_session_id,omitempty"`
	NewsFeeds        []news.FeedConfig          `json:"news_feeds,omitempty"`
	EncryptStorage   bool                       `json:"encrypt_storage,omitempty"`
	StorageSalt      string                     `json:"storage_salt,omitempty"` // base64
	SimulationMode   bool                       `json:"simulation_mode,omitempty"`
}

type diskAIProvider struct {
	BaseURL string `json:"base_url,omitempty"` // plaintext; not a secret
	APIKey  string `json:"api_key,omitempty"`  // base64(nonce[12]+ciphertext) or ""
}

type diskKDF struct {
	Salt    string `json:"salt"` // base64(16 random bytes)
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
	KeyLen  uint32 `json:"key_len"`
}

type diskProvider struct {
	APIKey string `json:"api_key"` // base64(nonce[12]+ciphertext) or ""
}

// Load reads the config file from the standard OS path and decrypts sensitive fields.
// If the file does not exist, Load returns an empty *AurisConfig (not nil) and a nil error.
func Load(passphrase string) (*AurisConfig, error) {
	p, err := Path()
	if err != nil {
		return nil, fmt.Errorf("config: Load: %w", err)
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &AurisConfig{
				Providers:    make(map[string]*ProviderConfig),
				AIProviders:  make(map[string]*AIProviderConfig),
				AITaskRoutes: make(map[string]AITaskRoute),
				NewsFeeds:    news.DefaultFeeds,
			}, nil
		}
		return nil, fmt.Errorf("config: Load: read file: %w", err)
	}

	var disk diskConfig
	if err := json.Unmarshal(data, &disk); err != nil {
		return nil, fmt.Errorf("config: Load: unmarshal: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(disk.KDF.Salt)
	if err != nil {
		return nil, fmt.Errorf("config: Load: decode salt: %w", err)
	}
	params := kdfParams{
		Salt:    salt,
		Time:    disk.KDF.Time,
		Memory:  disk.KDF.Memory,
		Threads: disk.KDF.Threads,
		KeyLen:  disk.KDF.KeyLen,
	}
	key := deriveKey(passphrase, params)

	newsFeeds := disk.NewsFeeds
	if len(newsFeeds) == 0 {
		newsFeeds = news.DefaultFeeds
	}

	var storageSalt []byte
	if disk.StorageSalt != "" {
		storageSalt, err = base64.StdEncoding.DecodeString(disk.StorageSalt)
		if err != nil {
			return nil, fmt.Errorf("config: Load: decode storage salt: %w", err)
		}
	}

	cfg := &AurisConfig{
		ActiveProvider:   disk.ActiveProvider,
		Providers:        make(map[string]*ProviderConfig, len(disk.Providers)),
		ProviderOrder:    disk.ProviderOrder,
		Locale:           disk.Locale,
		Theme:            disk.Theme,
		FinancialProfile: disk.FinancialProfile,
		ActiveAIProvider: disk.ActiveAIProvider,
		DefaultAIModel:   disk.DefaultAIModel,
		AITaskRoutes:     disk.AITaskRoutes,
		AIProviders:      make(map[string]*AIProviderConfig),
		ChatHistory:      disk.ChatHistory,
		ActiveSessionID:  disk.ActiveSessionID,
		NewsFeeds:        newsFeeds,
		EncryptStorage:   disk.EncryptStorage,
		StorageSalt:      storageSalt,
		SimulationMode:   disk.SimulationMode,
	}
	if cfg.AITaskRoutes == nil {
		cfg.AITaskRoutes = make(map[string]AITaskRoute)
	}
	for name, dp := range disk.Providers {
		apiKey, err := decryptField(key, dp.APIKey)
		if err != nil {
			return nil, fmt.Errorf("config: Load: provider %q: %w", name, err)
		}
		cfg.Providers[name] = &ProviderConfig{APIKey: apiKey}
	}
	for name, dp := range disk.AIProviders {
		apiKey, err := decryptField(key, dp.APIKey)
		if err != nil {
			return nil, fmt.Errorf("config: Load: ai provider %q: %w", name, err)
		}
		cfg.AIProviders[name] = &AIProviderConfig{BaseURL: dp.BaseURL, APIKey: apiKey}
	}
	return cfg, nil
}

// Save encrypts sensitive fields and writes the config to the standard OS path.
// It creates the config directory if it does not exist.
// A new random salt is generated on each call.
func Save(cfg *AurisConfig, passphrase string) error {
	p, err := Path()
	if err != nil {
		return fmt.Errorf("config: Save: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("config: Save: mkdir: %w", err)
	}

	params, err := newKDFParams()
	if err != nil {
		return fmt.Errorf("config: Save: %w", err)
	}
	key := deriveKey(passphrase, params)

	disk := diskConfig{
		ActiveProvider: cfg.ActiveProvider,
		ProviderOrder:  cfg.ProviderOrder,
		KDF: diskKDF{
			Salt:    base64.StdEncoding.EncodeToString(params.Salt),
			Time:    params.Time,
			Memory:  params.Memory,
			Threads: params.Threads,
			KeyLen:  params.KeyLen,
		},
		Locale:           cfg.Locale,
		Theme:            cfg.Theme,
		FinancialProfile: cfg.FinancialProfile,
		ActiveAIProvider: cfg.ActiveAIProvider,
		DefaultAIModel:   cfg.DefaultAIModel,
		AITaskRoutes:     cfg.AITaskRoutes,
		ChatHistory:      cfg.ChatHistory,
		ActiveSessionID:  cfg.ActiveSessionID,
		NewsFeeds:        cfg.NewsFeeds,
		EncryptStorage:   cfg.EncryptStorage,
		StorageSalt:      base64.StdEncoding.EncodeToString(cfg.StorageSalt),
		SimulationMode:   cfg.SimulationMode,
	}

	if len(cfg.Providers) > 0 {
		disk.Providers = make(map[string]*diskProvider, len(cfg.Providers))
		for name, pc := range cfg.Providers {
			encKey, err := encryptField(key, pc.APIKey)
			if err != nil {
				return fmt.Errorf("config: Save: provider %q: %w", name, err)
			}
			disk.Providers[name] = &diskProvider{APIKey: encKey}
		}
	}

	if len(cfg.AIProviders) > 0 {
		disk.AIProviders = make(map[string]*diskAIProvider, len(cfg.AIProviders))
		for name, pc := range cfg.AIProviders {
			encKey, err := encryptField(key, pc.APIKey)
			if err != nil {
				return fmt.Errorf("config: Save: ai provider %q: %w", name, err)
			}
			disk.AIProviders[name] = &diskAIProvider{BaseURL: pc.BaseURL, APIKey: encKey}
		}
	}

	data, err := json.MarshalIndent(disk, "", "  ")
	if err != nil {
		return fmt.Errorf("config: Save: marshal: %w", err)
	}

	if err := WriteFileAtomic(p, data, 0o600); err != nil {
		return fmt.Errorf("config: Save: write file: %w", err)
	}
	return nil
}
