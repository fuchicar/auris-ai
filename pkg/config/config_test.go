package config

import (
	"encoding/json"
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
