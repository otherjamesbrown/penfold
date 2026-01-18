// Package oauth provides OAuth2 authentication with PKCE for Gmail API access.
package oauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// AES-256 key size in bytes.
const aes256KeySize = 32

// TokenEncryptor provides AES-256-GCM encryption for OAuth tokens.
type TokenEncryptor struct {
	gcm cipher.AEAD
}

// NewTokenEncryptor creates a new TokenEncryptor with the given encryption key.
// The key must be exactly 32 bytes for AES-256.
func NewTokenEncryptor(key []byte) (*TokenEncryptor, error) {
	if len(key) != aes256KeySize {
		return nil, fmt.Errorf("encryption key must be exactly %d bytes, got %d", aes256KeySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM cipher: %w", err)
	}

	return &TokenEncryptor{gcm: gcm}, nil
}

// Encrypt encrypts the given plaintext using AES-256-GCM.
// Returns the nonce prepended to the ciphertext.
func (e *TokenEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	// Generate a random nonce.
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	// Encrypt and authenticate.
	// The nonce is prepended to the ciphertext.
	ciphertext := e.gcm.Seal(nonce, nonce, plaintext, nil)

	return ciphertext, nil
}

// Decrypt decrypts the given ciphertext using AES-256-GCM.
// Expects the nonce to be prepended to the ciphertext.
func (e *TokenEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := e.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract the nonce from the beginning.
	nonce := ciphertext[:nonceSize]
	ciphertext = ciphertext[nonceSize:]

	// Decrypt and verify authentication.
	plaintext, err := e.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}

	return plaintext, nil
}

// EncryptString encrypts a string and returns a base64-encoded result.
func (e *TokenEncryptor) EncryptString(plaintext string) (string, error) {
	encrypted, err := e.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// DecryptString decrypts a base64-encoded ciphertext and returns the plaintext string.
func (e *TokenEncryptor) DecryptString(ciphertext string) (string, error) {
	encrypted, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decoding base64: %w", err)
	}
	decrypted, err := e.Decrypt(encrypted)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}

// GenerateEncryptionKey generates a cryptographically secure random 32-byte key.
func GenerateEncryptionKey() ([]byte, error) {
	key := make([]byte, aes256KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generating encryption key: %w", err)
	}
	return key, nil
}

// EncryptionKeyFromBase64 decodes a base64-encoded encryption key.
func EncryptionKeyFromBase64(encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decoding encryption key: %w", err)
	}
	if len(key) != aes256KeySize {
		return nil, fmt.Errorf("decoded key must be exactly %d bytes, got %d", aes256KeySize, len(key))
	}
	return key, nil
}

// EncryptionKeyToBase64 encodes an encryption key to base64.
func EncryptionKeyToBase64(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}
