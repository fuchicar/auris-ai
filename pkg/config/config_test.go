package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempConfig redirects all config I/O to a temp file for the duration of the test.
func withTempConfig(t *testing.T) {
	t.Helper()
	configPathOverride = filepath.Join(t.TempDir(), "config.json")
	t.Cleanup(func() { configPathOverride = "" })
}

func TestLoad_FileNotExist(t *testing.T) {
	withTempConfig(t)

	cfg, err := Load("anypassphrase")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil AurisConfig")
	}
	if cfg.ActiveProvider != "" {
		t.Errorf("expected empty ActiveProvider, got %q", cfg.ActiveProvider)
	}
	if cfg.Providers == nil {
		t.Fatal("expected non-nil Providers map")
	}
	if len(cfg.Providers) != 0 {
		t.Errorf("expected empty Providers, got %d entries", len(cfg.Providers))
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	configPathOverride = filepath.Join(base, "subdir", "nested", "config.json")
	t.Cleanup(func() { configPathOverride = "" })

	if err := Save(&AurisConfig{}, "pass"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(configPathOverride)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected file permissions 0600, got %o", perm)
	}

	dirInfo, err := os.Stat(filepath.Dir(configPathOverride))
	if err != nil {
		t.Fatalf("config dir not created: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("expected dir permissions 0700, got %o", perm)
	}
}

func TestRoundtrip_SingleProvider(t *testing.T) {
	withTempConfig(t)

	original := &AurisConfig{
		ActiveProvider: "fmp",
		Providers: map[string]*ProviderConfig{
			"fmp": {APIKey: "my-secret-api-key"},
		},
	}
	if err := Save(original, "hunter2"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("hunter2")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ActiveProvider != "fmp" {
		t.Errorf("ActiveProvider: got %q, want %q", loaded.ActiveProvider, "fmp")
	}
	got := loaded.Providers["fmp"]
	if got == nil {
		t.Fatal("provider 'fmp' not found after Load")
	}
	if got.APIKey != "my-secret-api-key" {
		t.Errorf("APIKey: got %q, want %q", got.APIKey, "my-secret-api-key")
	}
}

func TestRoundtrip_MultipleProviders(t *testing.T) {
	withTempConfig(t)

	original := &AurisConfig{
		ActiveProvider: "fmp",
		Providers: map[string]*ProviderConfig{
			"fmp":   {APIKey: "fmp-key-abc"},
			"alpha": {APIKey: "alpha-key-xyz"},
		},
	}
	if err := Save(original, "passphrase"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("passphrase")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for name, want := range original.Providers {
		got, ok := loaded.Providers[name]
		if !ok {
			t.Errorf("provider %q missing after Load", name)
			continue
		}
		if got.APIKey != want.APIKey {
			t.Errorf("provider %q APIKey: got %q, want %q", name, got.APIKey, want.APIKey)
		}
	}
}

func TestRoundtrip_ProviderOrder(t *testing.T) {
	withTempConfig(t)

	original := &AurisConfig{
		ActiveProvider: "eodhd",
		Providers: map[string]*ProviderConfig{
			"eodhd": {APIKey: "eodhd-key"},
		},
		ProviderOrder: []string{"eodhd", "fmp"},
	}
	if err := Save(original, "passphrase"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("passphrase")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"eodhd", "fmp"}
	if len(loaded.ProviderOrder) != len(want) {
		t.Fatalf("ProviderOrder: got %v, want %v", loaded.ProviderOrder, want)
	}
	for i, k := range want {
		if loaded.ProviderOrder[i] != k {
			t.Errorf("ProviderOrder[%d]: got %q, want %q", i, loaded.ProviderOrder[i], k)
		}
	}
}

func TestRoundtrip_ProviderOrderEmpty(t *testing.T) {
	withTempConfig(t)

	original := &AurisConfig{Providers: map[string]*ProviderConfig{}}
	if err := Save(original, "passphrase"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("passphrase")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.ProviderOrder) != 0 {
		t.Errorf("ProviderOrder: got %v, want empty", loaded.ProviderOrder)
	}
}

func TestRoundtrip_EmptyAPIKey(t *testing.T) {
	withTempConfig(t)

	original := &AurisConfig{
		Providers: map[string]*ProviderConfig{
			"fmp": {APIKey: ""},
		},
	}
	if err := Save(original, "pass"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("pass")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := loaded.Providers["fmp"]
	if got == nil {
		t.Fatal("provider 'fmp' missing after Load")
	}
	if got.APIKey != "" {
		t.Errorf("APIKey: got %q, want %q", got.APIKey, "")
	}
}

func TestLoad_WrongPassphrase(t *testing.T) {
	withTempConfig(t)

	cfg := &AurisConfig{
		Providers: map[string]*ProviderConfig{
			"fmp": {APIKey: "secret"},
		},
	}
	if err := Save(cfg, "correct"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err := Load("wrong")
	if err == nil {
		t.Fatal("expected error with wrong passphrase, got nil")
	}
	if !strings.Contains(err.Error(), "config: Load:") {
		t.Errorf("error should contain 'config: Load:', got: %v", err)
	}
}

// TestLoad_WrongPassphrase_NoAPIKeys covers issue #23: with no provider API
// keys stored at all (e.g. an Ollama-only setup), there is no encrypted
// field whose decryption would incidentally fail on a wrong passphrase.
// Load must still reject it via the PassphraseCheck value.
func TestLoad_WrongPassphrase_NoAPIKeys(t *testing.T) {
	withTempConfig(t)

	cfg := &AurisConfig{ActiveAIProvider: "ollama"}
	if err := Save(cfg, "correct"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err := Load("wrong")
	if err == nil {
		t.Fatal("expected error with wrong passphrase, got nil")
	}
	if !errors.Is(err, ErrIncorrectPassphrase) {
		t.Errorf("expected ErrIncorrectPassphrase, got: %v", err)
	}

	if _, err := Load("correct"); err != nil {
		t.Errorf("Load with correct passphrase: %v", err)
	}
}

// TestLoad_LegacyConfig_NoPassphraseCheck_StillLoads simulates a config
// file written before PassphraseCheck existed (empty/omitted field). Load
// must not regress for these — the check is simply skipped.
func TestLoad_LegacyConfig_NoPassphraseCheck_StillLoads(t *testing.T) {
	withTempConfig(t)

	params, err := newKDFParams()
	if err != nil {
		t.Fatalf("newKDFParams: %v", err)
	}
	legacy := diskConfig{
		ActiveProvider: "fmp",
		KDF: diskKDF{
			Salt:    base64.StdEncoding.EncodeToString(params.Salt),
			Time:    params.Time,
			Memory:  params.Memory,
			Threads: params.Threads,
			KeyLen:  params.KeyLen,
		},
		// PassphraseCheck intentionally left empty, simulating a pre-fix file.
	}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy config: %v", err)
	}
	if err := os.WriteFile(configPathOverride, data, 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	loaded, err := Load("anypassphrase")
	if err != nil {
		t.Fatalf("Load legacy config: %v", err)
	}
	if loaded.ActiveProvider != "fmp" {
		t.Errorf("ActiveProvider: got %q, want %q", loaded.ActiveProvider, "fmp")
	}
}

// TestSave_UpgradesLegacyConfig_WithPassphraseCheck confirms the migration
// path from the issue: a legacy config (no PassphraseCheck) that goes
// through a normal Load-then-Save cycle (as AppModel.saveConfig does)
// gets upgraded, and a wrong passphrase is rejected from then on.
func TestSave_UpgradesLegacyConfig_WithPassphraseCheck(t *testing.T) {
	withTempConfig(t)

	params, err := newKDFParams()
	if err != nil {
		t.Fatalf("newKDFParams: %v", err)
	}
	legacy := diskConfig{
		KDF: diskKDF{
			Salt:    base64.StdEncoding.EncodeToString(params.Salt),
			Time:    params.Time,
			Memory:  params.Memory,
			Threads: params.Threads,
			KeyLen:  params.KeyLen,
		},
	}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy config: %v", err)
	}
	if err := os.WriteFile(configPathOverride, data, 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := Load("mypassphrase")
	if err != nil {
		t.Fatalf("initial Load of legacy config: %v", err)
	}
	if err := Save(cfg, "mypassphrase"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	upgraded, err := os.ReadFile(configPathOverride)
	if err != nil {
		t.Fatalf("read upgraded config: %v", err)
	}
	var disk diskConfig
	if err := json.Unmarshal(upgraded, &disk); err != nil {
		t.Fatalf("unmarshal upgraded config: %v", err)
	}
	if disk.PassphraseCheck == "" {
		t.Fatal("expected PassphraseCheck to be populated after Save")
	}

	if _, err := Load("wrongpassphrase"); !errors.Is(err, ErrIncorrectPassphrase) {
		t.Errorf("expected ErrIncorrectPassphrase after upgrade, got: %v", err)
	}
}

func TestSave_PassphraseCheckDiffersEachCall(t *testing.T) {
	withTempConfig(t)

	cfg := &AurisConfig{ActiveProvider: "fmp"}

	if err := Save(cfg, "pass"); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	data1, err := os.ReadFile(configPathOverride)
	if err != nil {
		t.Fatalf("read first save: %v", err)
	}

	if err := Save(cfg, "pass"); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	data2, err := os.ReadFile(configPathOverride)
	if err != nil {
		t.Fatalf("read second save: %v", err)
	}

	var disk1, disk2 diskConfig
	json.Unmarshal(data1, &disk1)
	json.Unmarshal(data2, &disk2)

	if disk1.PassphraseCheck == "" || disk2.PassphraseCheck == "" {
		t.Fatal("expected PassphraseCheck to be populated on both saves")
	}
	if disk1.PassphraseCheck == disk2.PassphraseCheck {
		t.Error("expected different PassphraseCheck values on each Save, got the same")
	}
}

func TestPassphraseCheck_RoundtripAndWrongKey(t *testing.T) {
	params, err := newKDFParams()
	if err != nil {
		t.Fatalf("newKDFParams: %v", err)
	}
	key := deriveKey("passphrase", params)

	blob, err := newPassphraseCheck(key)
	if err != nil {
		t.Fatalf("newPassphraseCheck: %v", err)
	}
	if !verifyPassphraseCheck(key, blob) {
		t.Error("verifyPassphraseCheck: expected true for the correct key")
	}

	wrongKey := deriveKey("different-passphrase", params)
	if verifyPassphraseCheck(wrongKey, blob) {
		t.Error("verifyPassphraseCheck: expected false for the wrong key")
	}
}

func TestSave_DifferentSaltEachCall(t *testing.T) {
	withTempConfig(t)

	cfg := &AurisConfig{ActiveProvider: "fmp"}

	if err := Save(cfg, "pass"); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	data1, err := os.ReadFile(configPathOverride)
	if err != nil {
		t.Fatalf("read first save: %v", err)
	}

	if err := Save(cfg, "pass"); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	data2, err := os.ReadFile(configPathOverride)
	if err != nil {
		t.Fatalf("read second save: %v", err)
	}

	var disk1, disk2 diskConfig
	json.Unmarshal(data1, &disk1)
	json.Unmarshal(data2, &disk2)

	if disk1.KDF.Salt == disk2.KDF.Salt {
		t.Error("expected different salts on each Save, got the same")
	}
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	params, err := newKDFParams()
	if err != nil {
		t.Fatalf("newKDFParams: %v", err)
	}
	key := deriveKey("passphrase", params)

	blob, err := encryptField(key, "plaintext-value")
	if err != nil {
		t.Fatalf("encryptField: %v", err)
	}
	if blob == "" {
		t.Fatal("encryptField returned empty blob for non-empty plaintext")
	}

	result, err := decryptField(key, blob)
	if err != nil {
		t.Fatalf("decryptField: %v", err)
	}
	if result != "plaintext-value" {
		t.Errorf("got %q, want %q", result, "plaintext-value")
	}
}

func TestEncryptDecrypt_EmptyString(t *testing.T) {
	params, err := newKDFParams()
	if err != nil {
		t.Fatalf("newKDFParams: %v", err)
	}
	key := deriveKey("pass", params)

	blob, err := encryptField(key, "")
	if err != nil {
		t.Fatalf("encryptField: %v", err)
	}
	if blob != "" {
		t.Errorf("expected empty blob for empty plaintext, got %q", blob)
	}

	result, err := decryptField(key, "")
	if err != nil {
		t.Fatalf("decryptField: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result for empty blob, got %q", result)
	}
}

func TestDecryptField_TruncatedBlob(t *testing.T) {
	params, err := newKDFParams()
	if err != nil {
		t.Fatalf("newKDFParams: %v", err)
	}
	key := deriveKey("pass", params)

	// 8 bytes < 12-byte nonce minimum
	shortBlob := "AAAAAAAAAAA=" // base64 of ~8 bytes
	_, err = decryptField(key, shortBlob)
	if err == nil {
		t.Fatal("expected error for truncated blob, got nil")
	}
}

func TestLoad_FileNotExist_AIProvidersMapsNotNil(t *testing.T) {
	withTempConfig(t)

	cfg, err := Load("anypassphrase")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AIProviders == nil {
		t.Error("AIProviders should be non-nil when file does not exist")
	}
	if cfg.AITaskRoutes == nil {
		t.Error("AITaskRoutes should be non-nil when file does not exist")
	}
}

func TestRoundtrip_AIProvider(t *testing.T) {
	withTempConfig(t)

	original := &AurisConfig{
		ActiveAIProvider: "ollama",
		DefaultAIModel:   "llama3.2:latest",
		AIProviders: map[string]*AIProviderConfig{
			"ollama": {BaseURL: "http://localhost:11434", APIKey: "secret-token"},
		},
	}
	if err := Save(original, "pass"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("pass")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ActiveAIProvider != "ollama" {
		t.Errorf("ActiveAIProvider: got %q, want %q", loaded.ActiveAIProvider, "ollama")
	}
	if loaded.DefaultAIModel != "llama3.2:latest" {
		t.Errorf("DefaultAIModel: got %q, want %q", loaded.DefaultAIModel, "llama3.2:latest")
	}
	got, ok := loaded.AIProviders["ollama"]
	if !ok {
		t.Fatal("ai provider 'ollama' missing after Load")
	}
	if got.BaseURL != "http://localhost:11434" {
		t.Errorf("BaseURL: got %q, want %q", got.BaseURL, "http://localhost:11434")
	}
	if got.APIKey != "secret-token" {
		t.Errorf("APIKey: got %q, want %q", got.APIKey, "secret-token")
	}
}

func TestRoundtrip_AIProviderEmptyAPIKey(t *testing.T) {
	withTempConfig(t)

	original := &AurisConfig{
		AIProviders: map[string]*AIProviderConfig{
			"ollama": {BaseURL: "http://localhost:11434", APIKey: ""},
		},
	}
	if err := Save(original, "pass"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("pass")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := loaded.AIProviders["ollama"]
	if !ok {
		t.Fatal("ai provider 'ollama' missing after Load")
	}
	if got.APIKey != "" {
		t.Errorf("APIKey: got %q, want empty string", got.APIKey)
	}
}

func TestRoundtrip_AITaskRoutes(t *testing.T) {
	withTempConfig(t)

	original := &AurisConfig{
		AITaskRoutes: map[string]AITaskRoute{
			"chat":               {Provider: "ollama", Model: "llama3.2:latest"},
			"financial_analysis": {Provider: "ollama", Model: "llama3.2:latest"},
			"summary":            {Provider: "ollama", Model: "mistral:latest"},
		},
	}
	if err := Save(original, "pass"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("pass")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for task, want := range original.AITaskRoutes {
		got, ok := loaded.AITaskRoutes[task]
		if !ok {
			t.Errorf("task route %q missing after Load", task)
			continue
		}
		if got.Provider != want.Provider {
			t.Errorf("task %q Provider: got %q, want %q", task, got.Provider, want.Provider)
		}
		if got.Model != want.Model {
			t.Errorf("task %q Model: got %q, want %q", task, got.Model, want.Model)
		}
	}
}

// TestRoundtrip_AIProviderName covers issue #3: a dynamically-named
// OpenAI-Compatible instance (keyed "openai_compatible:<slug>") must
// round-trip its user-chosen Name label, alongside the usual BaseURL/APIKey,
// through an encrypted Save/Load cycle.
func TestRoundtrip_AIProviderName(t *testing.T) {
	withTempConfig(t)

	original := &AurisConfig{
		AIProviders: map[string]*AIProviderConfig{
			"openai_compatible:deepseek": {BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-test", Name: "DeepSeek"},
			"ollama":                     {BaseURL: "http://localhost:11434"},
		},
	}
	if err := Save(original, "pass"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("pass")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := loaded.AIProviders["openai_compatible:deepseek"]
	if !ok {
		t.Fatal("expected the dynamic instance key to round-trip")
	}
	if got.Name != "DeepSeek" {
		t.Errorf("Name = %q, want %q", got.Name, "DeepSeek")
	}
	if got.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want %q", got.APIKey, "sk-test")
	}
	if got.BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("BaseURL = %q, want %q", got.BaseURL, "https://api.deepseek.com/v1")
	}
	if loaded.AIProviders["ollama"].Name != "" {
		t.Errorf("ollama Name = %q, want empty (singleton provider)", loaded.AIProviders["ollama"].Name)
	}
}

func TestRoundtrip_NewFields(t *testing.T) {
	withTempConfig(t)

	fp := &FinancialProfile{
		LifeStage:            "under35",
		IncomeStability:      "stable",
		InvestmentGoals:      []string{"retirement", "wealth_growth"},
		InvestmentGoalsOther: "custom goal",
		TimeHorizon:          "3_7y",
		MaxAcceptableLoss:    "25pct",
		FinancialExperience:  []string{"stocks_etfs", "funds"},
		InvestmentPriority:   "returns",
		Restrictions:         []string{"no_crypto", "country_only"},
		RestrictionsCountry:  "Spain",
	}

	original := &AurisConfig{
		ActiveProvider:   "fmp",
		Providers:        map[string]*ProviderConfig{"fmp": {APIKey: "key"}},
		Locale:           "es",
		Theme:            "dark",
		FinancialProfile: fp,
	}

	if err := Save(original, "pass"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("pass")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Locale != "es" {
		t.Errorf("Locale: got %q, want %q", loaded.Locale, "es")
	}
	if loaded.Theme != "dark" {
		t.Errorf("Theme: got %q, want %q", loaded.Theme, "dark")
	}
	if loaded.FinancialProfile == nil {
		t.Fatal("FinancialProfile is nil after Load")
	}
	if loaded.FinancialProfile.LifeStage != "under35" {
		t.Errorf("LifeStage: got %q, want %q", loaded.FinancialProfile.LifeStage, "under35")
	}
	if len(loaded.FinancialProfile.InvestmentGoals) != 2 {
		t.Errorf("InvestmentGoals length: got %d, want 2", len(loaded.FinancialProfile.InvestmentGoals))
	}
	if loaded.FinancialProfile.InvestmentGoalsOther != "custom goal" {
		t.Errorf("InvestmentGoalsOther: got %q, want %q", loaded.FinancialProfile.InvestmentGoalsOther, "custom goal")
	}
	if loaded.FinancialProfile.RestrictionsCountry != "Spain" {
		t.Errorf("RestrictionsCountry: got %q, want %q", loaded.FinancialProfile.RestrictionsCountry, "Spain")
	}
}

func TestRoundtrip_StorageEncryption(t *testing.T) {
	withTempConfig(t)

	salt, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	original := &AurisConfig{
		EncryptStorage: true,
		StorageSalt:    salt,
	}
	if err := Save(original, "pass"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("pass")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.EncryptStorage {
		t.Error("EncryptStorage: got false, want true")
	}
	if string(loaded.StorageSalt) != string(salt) {
		t.Errorf("StorageSalt: got %x, want %x", loaded.StorageSalt, salt)
	}
}

func TestRoundtrip_StorageEncryption_Disabled(t *testing.T) {
	withTempConfig(t)

	if err := Save(&AurisConfig{}, "pass"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load("pass")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.EncryptStorage {
		t.Error("EncryptStorage: got true, want false")
	}
	if len(loaded.StorageSalt) != 0 {
		t.Errorf("StorageSalt: got %x, want empty", loaded.StorageSalt)
	}
}

func TestDeriveStorageKey_Deterministic(t *testing.T) {
	salt, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	k1 := DeriveStorageKey("passphrase", salt)
	k2 := DeriveStorageKey("passphrase", salt)
	if string(k1) != string(k2) {
		t.Error("DeriveStorageKey: same passphrase+salt produced different keys")
	}

	other, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	k3 := DeriveStorageKey("passphrase", other)
	if string(k1) == string(k3) {
		t.Error("DeriveStorageKey: different salts produced the same key")
	}
}

func TestEncryptDecryptBytes_Roundtrip(t *testing.T) {
	salt, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	key := DeriveStorageKey("pass", salt)

	blob, err := EncryptBytes(key, []byte(`{"foo":"bar"}`))
	if err != nil {
		t.Fatalf("EncryptBytes: %v", err)
	}
	got, err := DecryptBytes(key, blob)
	if err != nil {
		t.Fatalf("DecryptBytes: %v", err)
	}
	if string(got) != `{"foo":"bar"}` {
		t.Errorf("got %q, want %q", got, `{"foo":"bar"}`)
	}

	wrongKey := DeriveStorageKey("wrong", salt)
	if _, err := DecryptBytes(wrongKey, blob); err == nil {
		t.Error("expected error decrypting with wrong key, got nil")
	}
}

type testEncryptable struct {
	Name string `json:"name"`
}

func TestMarshalUnmarshalEncryptable_Encrypted(t *testing.T) {
	salt, err := NewSalt()
	if err != nil {
		t.Fatalf("NewSalt: %v", err)
	}
	key := DeriveStorageKey("pass", salt)

	data, err := MarshalEncryptable(&testEncryptable{Name: "portfolio1"}, key)
	if err != nil {
		t.Fatalf("MarshalEncryptable: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if raw["encrypted"] != true {
		t.Errorf("expected encrypted envelope, got %v", raw)
	}
	if _, ok := raw["name"]; ok {
		t.Error("plaintext field leaked into on-disk envelope")
	}

	var out testEncryptable
	if err := UnmarshalEncryptable(data, &out, key); err != nil {
		t.Fatalf("UnmarshalEncryptable: %v", err)
	}
	if out.Name != "portfolio1" {
		t.Errorf("Name: got %q, want %q", out.Name, "portfolio1")
	}

	if err := UnmarshalEncryptable(data, &out, nil); err == nil {
		t.Error("expected error unmarshalling encrypted data with nil key, got nil")
	}
}

func TestMarshalUnmarshalEncryptable_Plaintext(t *testing.T) {
	data, err := MarshalEncryptable(&testEncryptable{Name: "portfolio1"}, nil)
	if err != nil {
		t.Fatalf("MarshalEncryptable: %v", err)
	}
	if !strings.Contains(string(data), "portfolio1") {
		t.Errorf("expected plaintext JSON, got %q", data)
	}

	var out testEncryptable
	if err := UnmarshalEncryptable(data, &out, nil); err != nil {
		t.Fatalf("UnmarshalEncryptable: %v", err)
	}
	if out.Name != "portfolio1" {
		t.Errorf("Name: got %q, want %q", out.Name, "portfolio1")
	}
}

func TestUnmarshalEncryptable_LegacyPlaintextCompatibleWithKeySet(t *testing.T) {
	// A pre-FEAT-16 plaintext file has no "encrypted"/"data" keys. It must
	// still parse correctly even when a storage key is available (i.e.
	// encryption is enabled but this particular file predates it).
	legacy := []byte(`{"name":"legacy-portfolio"}`)
	key := DeriveStorageKey("pass", []byte("0123456789abcdef"))

	var out testEncryptable
	if err := UnmarshalEncryptable(legacy, &out, key); err != nil {
		t.Fatalf("UnmarshalEncryptable: %v", err)
	}
	if out.Name != "legacy-portfolio" {
		t.Errorf("Name: got %q, want %q", out.Name, "legacy-portfolio")
	}
}

func TestSetGetStorageKey(t *testing.T) {
	t.Cleanup(func() { SetStorageKey(nil) })

	if StorageKey() != nil {
		t.Fatal("expected StorageKey to start nil")
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	SetStorageKey(key)
	if string(StorageKey()) != string(key) {
		t.Error("StorageKey did not return the key set by SetStorageKey")
	}
}
