// Package security provides encryption utilities for AgentOS
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// Encryptor handles AES-256-GCM encryption
type Encryptor struct {
	key []byte
}

// NewEncryptor creates an encryptor with the given master key
// Derives a 256-bit key using SHA-256
func NewEncryptor(masterKey []byte) *Encryptor {
	hash := sha256.Sum256(masterKey)
	return &Encryptor{key: hash[:]}
}

// Encrypt encrypts plaintext using AES-256-GCM
func (e *Encryptor) Encrypt(plaintext []byte) (string, error) {
	if len(plaintext) == 0 {
		return "", fmt.Errorf("plaintext cannot be empty")
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext
func (e *Encryptor) Decrypt(ciphertext string) ([]byte, error) {
	if ciphertext == "" {
		return nil, fmt.Errorf("ciphertext cannot be empty")
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(data) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertextBytes, nil)
}

// GenerateKey generates a random encryption key of the specified length
func GenerateKey(length int) ([]byte, error) {
	key := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	return key, nil
}

// DeriveKey derives a key from a password using PBKDF2-like approach
// In production, use a proper KDF like Argon2 or PBKDF2
func DeriveKey(password string, salt []byte) []byte {
	// Simple derivation - for production use proper KDF
	combined := append([]byte(password), salt...)
	hash := sha256.Sum256(combined)
	return hash[:]
}
