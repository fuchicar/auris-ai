package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
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

// newKDFParams generates a kdfParams with a fresh random 16-byte salt and default Argon2id tuning.
func newKDFParams() (kdfParams, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return kdfParams{}, fmt.Errorf("config: newKDFParams: read random: %w", err)
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
