package config

import (
	"encoding/json"
	"fmt"
)

// storageKey is the currently active key for optional at-rest encryption of
// portfolios and chat sessions (FEAT-16). nil means encryption is disabled —
// MarshalEncryptable/UnmarshalEncryptable fall back to plain JSON, matching
// pre-FEAT-16 behavior. Set once by pkg/tui after unlock (or when the user
// toggles the setting), mirroring the portfoliosDirOverride test-override
// convention: package-level state rather than threading a key through every
// SavePortfolio/LoadPortfolio/SaveSession/LoadSession call site.
var storageKey []byte

// SetStorageKey sets the active storage-encryption key. Pass nil to disable
// encryption for subsequent saves/loads.
func SetStorageKey(key []byte) { storageKey = key }

// StorageKey returns the active storage-encryption key, or nil if disabled.
func StorageKey() []byte { return storageKey }

// EncryptedEnvelope is the on-disk wrapper for an optionally-encrypted JSON
// file (a portfolio or a chat session). A legacy plaintext file has no
// "encrypted"/"data" keys, so unmarshalling it into EncryptedEnvelope leaves
// Encrypted false — that's the detection signal UnmarshalEncryptable relies on.
type EncryptedEnvelope struct {
	Encrypted bool   `json:"encrypted,omitempty"`
	Data      string `json:"data,omitempty"`
}

// MarshalEncryptable marshals v to JSON. If key is non-nil, the JSON is
// encrypted and wrapped in an EncryptedEnvelope; otherwise the plain JSON is
// returned unchanged.
func MarshalEncryptable(v any, key []byte) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("config: MarshalEncryptable: marshal: %w", err)
	}
	if key == nil {
		return data, nil
	}
	blob, err := EncryptBytes(key, data)
	if err != nil {
		return nil, fmt.Errorf("config: MarshalEncryptable: encrypt: %w", err)
	}
	env, err := json.MarshalIndent(EncryptedEnvelope{Encrypted: true, Data: blob}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("config: MarshalEncryptable: marshal envelope: %w", err)
	}
	return env, nil
}

// UnmarshalEncryptable unmarshals data into v. data may be a plain JSON
// object (legacy, or encryption disabled) or an EncryptedEnvelope. If the
// file is encrypted, key must be non-nil or an error is returned.
func UnmarshalEncryptable(data []byte, v any, key []byte) error {
	var env EncryptedEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("config: UnmarshalEncryptable: unmarshal envelope: %w", err)
	}
	if !env.Encrypted {
		if err := json.Unmarshal(data, v); err != nil {
			return fmt.Errorf("config: UnmarshalEncryptable: unmarshal: %w", err)
		}
		return nil
	}
	if key == nil {
		return fmt.Errorf("config: UnmarshalEncryptable: file is encrypted but no storage key is set")
	}
	plain, err := DecryptBytes(key, env.Data)
	if err != nil {
		return fmt.Errorf("config: UnmarshalEncryptable: decrypt: %w", err)
	}
	if err := json.Unmarshal(plain, v); err != nil {
		return fmt.Errorf("config: UnmarshalEncryptable: unmarshal decrypted: %w", err)
	}
	return nil
}
