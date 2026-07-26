package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	defaultArgonTime    uint32 = 1
	defaultArgonMemory  uint32 = 64 * 1024 // 64 MiB
	defaultArgonThreads uint8  = 4
	defaultArgonKeyLen  uint32 = 32 // AES-256
)

type kdfParams struct {
	Salt    []byte
	Time    uint32
	Memory  uint32
	Threads uint8
	KeyLen  uint32
}

// NewSalt generates a fresh random 16-byte salt, suitable for a new kdfParams
// or for AurisConfig.StorageSalt.
func NewSalt() ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("config: NewSalt: read random: %w", err)
	}
	return salt, nil
}

// newKDFParams generates a kdfParams with a fresh random 16-byte salt and default Argon2id tuning.
func newKDFParams() (kdfParams, error) {
	salt, err := NewSalt()
	if err != nil {
		return kdfParams{}, fmt.Errorf("config: newKDFParams: %w", err)
	}
	return kdfParams{
		Salt:    salt,
		Time:    defaultArgonTime,
		Memory:  defaultArgonMemory,
		Threads: defaultArgonThreads,
		KeyLen:  defaultArgonKeyLen,
	}, nil
}

// deriveKey runs Argon2id with the given passphrase and params, returning a 32-byte AES key.
func deriveKey(passphrase string, p kdfParams) []byte {
	return argon2.IDKey([]byte(passphrase), p.Salt, p.Time, p.Memory, p.Threads, p.KeyLen)
}

// DeriveStorageKey derives a 32-byte AES key from a passphrase and salt using
// the same Argon2id tuning as credential encryption (FEAT-16). Unlike the
// config file's own KDF salt (which is regenerated on every Save), the salt
// passed here (AurisConfig.StorageSalt) is meant to stay stable across saves.
func DeriveStorageKey(passphrase string, salt []byte) []byte {
	return deriveKey(passphrase, kdfParams{
		Salt:    salt,
		Time:    defaultArgonTime,
		Memory:  defaultArgonMemory,
		Threads: defaultArgonThreads,
		KeyLen:  defaultArgonKeyLen,
	})
}

// encryptField encrypts plaintext with AES-256-GCM using the provided key.
// Returns base64(nonce[12] + ciphertext). Returns "" if plaintext is "".
func encryptField(key []byte, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("config: encryptField: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("config: encryptField: new gcm: %w", err)
	}

	// Allocate nonce; Seal will append ciphertext+tag after it.
	nonce := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("config: encryptField: read nonce: %w", err)
	}

	blob := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(blob), nil
}

// decryptField decrypts a base64(nonce[12]+ciphertext) blob produced by encryptField.
// Returns "" if blob is "".
func decryptField(key []byte, blob string) (string, error) {
	if blob == "" {
		return "", nil
	}

	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return "", fmt.Errorf("config: decryptField: base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("config: decryptField: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("config: decryptField: new gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("config: decryptField: blob too short")
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("config: decryptField: decrypt: %w", err)
	}
	return string(plaintext), nil
}

// EncryptBytes encrypts plaintext with AES-256-GCM using the provided key.
// Returns base64(nonce[12] + ciphertext). Unlike encryptField, empty input is
// encrypted normally rather than passed through — a marshaled struct is never
// empty, so callers (MarshalEncryptable) don't need that special case.
func EncryptBytes(key []byte, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("config: EncryptBytes: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("config: EncryptBytes: new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("config: EncryptBytes: read nonce: %w", err)
	}

	blob := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(blob), nil
}

// newPassphraseCheck generates a fresh 16-byte random value, appends its
// CRC32 checksum, and encrypts the pair with key. A fixed/known plaintext is
// deliberately avoided here — each Save produces a unique blob, so there is
// no constant ciphertext/plaintext pair for an attacker to collect across
// configs; wrong-passphrase detection instead relies on AES-GCM's built-in
// auth-tag failure plus this CRC as a redundant, explicit check.
func newPassphraseCheck(key []byte) (string, error) {
	random := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, random); err != nil {
		return "", fmt.Errorf("config: newPassphraseCheck: read random: %w", err)
	}
	payload := binary.BigEndian.AppendUint32(random, crc32.ChecksumIEEE(random))
	blob, err := EncryptBytes(key, payload)
	if err != nil {
		return "", fmt.Errorf("config: newPassphraseCheck: %w", err)
	}
	return blob, nil
}

// verifyPassphraseCheck decrypts blob with key and confirms the trailing
// CRC32 matches the leading random bytes. False means either the key is
// wrong (GCM auth failure) or the payload was corrupted/tampered with.
func verifyPassphraseCheck(key []byte, blob string) bool {
	payload, err := DecryptBytes(key, blob)
	if err != nil || len(payload) != 20 {
		return false
	}
	return binary.BigEndian.Uint32(payload[16:]) == crc32.ChecksumIEEE(payload[:16])
}

// DecryptBytes decrypts a base64(nonce[12]+ciphertext) blob produced by EncryptBytes.
func DecryptBytes(key []byte, blob string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return nil, fmt.Errorf("config: DecryptBytes: base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("config: DecryptBytes: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("config: DecryptBytes: new gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return nil, fmt.Errorf("config: DecryptBytes: blob too short")
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("config: DecryptBytes: decrypt: %w", err)
	}
	return plaintext, nil
}
