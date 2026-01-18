// Package credentials provides secure credential storage for the penf CLI.
// It stores API keys and JWT tokens in ~/.penf/credentials.yaml
// with encryption for sensitive data at rest.
package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Credential storage constants.
const (
	DefaultCredentialsDir  = ".penf"
	DefaultCredentialsFile = "credentials.yaml"

	// AuthTypeAPIKey represents API key authentication.
	AuthTypeAPIKey = "api_key"
	// AuthTypeToken represents JWT token authentication.
	AuthTypeToken = "token"
)

// Common errors.
var (
	// ErrNoCredentials is returned when no credentials are stored.
	ErrNoCredentials = errors.New("no credentials stored")
	// ErrExpiredToken is returned when the stored token has expired.
	ErrExpiredToken = errors.New("stored token has expired")
	// ErrInvalidCredentials is returned when stored credentials are malformed.
	ErrInvalidCredentials = errors.New("invalid credentials format")
	// ErrEncryptionFailed is returned when encryption/decryption fails.
	ErrEncryptionFailed = errors.New("encryption failed")
)

// Credentials holds the stored authentication credentials.
type Credentials struct {
	// AuthType is the type of authentication ("api_key" or "token").
	AuthType string `yaml:"auth_type"`
	// APIKey is the stored API key (encrypted at rest).
	APIKey string `yaml:"api_key,omitempty"`
	// Token is the stored JWT token (encrypted at rest).
	Token string `yaml:"token,omitempty"`
	// RefreshToken is the refresh token for token renewal.
	RefreshToken string `yaml:"refresh_token,omitempty"`
	// ExpiresAt is the token expiration time.
	ExpiresAt time.Time `yaml:"expires_at,omitempty"`
	// ServerAddress is the server this credential is for.
	ServerAddress string `yaml:"server_address,omitempty"`
	// Subject is the authenticated user/service identifier.
	Subject string `yaml:"subject,omitempty"`
	// LastUpdated is when the credentials were last updated.
	LastUpdated time.Time `yaml:"last_updated"`
}

// Store manages credential storage operations.
type Store struct {
	// credentialsDir is the directory containing credentials.
	credentialsDir string
	// encryptionKey is derived from machine-specific data.
	encryptionKey []byte
}

// NewStore creates a new credential store with default settings.
func NewStore() (*Store, error) {
	dir, err := CredentialsDir()
	if err != nil {
		return nil, fmt.Errorf("getting credentials directory: %w", err)
	}

	key, err := deriveEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("deriving encryption key: %w", err)
	}

	return &Store{
		credentialsDir: dir,
		encryptionKey:  key,
	}, nil
}

// CredentialsDir returns the credentials directory path.
// Uses $PENF_CONFIG_DIR if set, otherwise ~/.penf
func CredentialsDir() (string, error) {
	if dir := os.Getenv("PENF_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	return filepath.Join(home, DefaultCredentialsDir), nil
}

// CredentialsPath returns the full path to the credentials file.
func CredentialsPath() (string, error) {
	dir, err := CredentialsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DefaultCredentialsFile), nil
}

// deriveEncryptionKey creates a machine-specific encryption key.
// This provides basic protection against copying credential files to other machines.
func deriveEncryptionKey() ([]byte, error) {
	// Combine machine-specific data for key derivation.
	var keyMaterial strings.Builder

	// Add hostname
	hostname, _ := os.Hostname()
	keyMaterial.WriteString(hostname)

	// Add username
	keyMaterial.WriteString(os.Getenv("USER"))
	if keyMaterial.Len() == 0 {
		keyMaterial.WriteString(os.Getenv("USERNAME"))
	}

	// Add OS-specific identifier
	keyMaterial.WriteString(runtime.GOOS)
	keyMaterial.WriteString(runtime.GOARCH)

	// Add home directory path as part of key material
	home, _ := os.UserHomeDir()
	keyMaterial.WriteString(home)

	// Derive a 32-byte key using SHA-256
	hash := sha256.Sum256([]byte(keyMaterial.String()))
	return hash[:], nil
}

// Save stores credentials to the credentials file.
func (s *Store) Save(creds *Credentials) error {
	if err := s.ensureDir(); err != nil {
		return fmt.Errorf("creating credentials directory: %w", err)
	}

	// Encrypt sensitive fields
	storageCreds := *creds
	storageCreds.LastUpdated = time.Now()

	if storageCreds.APIKey != "" {
		encrypted, err := s.encrypt(storageCreds.APIKey)
		if err != nil {
			return fmt.Errorf("encrypting API key: %w", err)
		}
		storageCreds.APIKey = encrypted
	}

	if storageCreds.Token != "" {
		encrypted, err := s.encrypt(storageCreds.Token)
		if err != nil {
			return fmt.Errorf("encrypting token: %w", err)
		}
		storageCreds.Token = encrypted
	}

	if storageCreds.RefreshToken != "" {
		encrypted, err := s.encrypt(storageCreds.RefreshToken)
		if err != nil {
			return fmt.Errorf("encrypting refresh token: %w", err)
		}
		storageCreds.RefreshToken = encrypted
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&storageCreds)
	if err != nil {
		return fmt.Errorf("marshaling credentials: %w", err)
	}

	// Write with restrictive permissions
	credPath := filepath.Join(s.credentialsDir, DefaultCredentialsFile)
	if err := os.WriteFile(credPath, data, 0600); err != nil {
		return fmt.Errorf("writing credentials file: %w", err)
	}

	return nil
}

// Load reads credentials from the credentials file.
func (s *Store) Load() (*Credentials, error) {
	credPath := filepath.Join(s.credentialsDir, DefaultCredentialsFile)

	data, err := os.ReadFile(credPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoCredentials
		}
		return nil, fmt.Errorf("reading credentials file: %w", err)
	}

	var creds Credentials
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	// Decrypt sensitive fields
	if creds.APIKey != "" {
		decrypted, err := s.decrypt(creds.APIKey)
		if err != nil {
			return nil, fmt.Errorf("decrypting API key: %w", err)
		}
		creds.APIKey = decrypted
	}

	if creds.Token != "" {
		decrypted, err := s.decrypt(creds.Token)
		if err != nil {
			return nil, fmt.Errorf("decrypting token: %w", err)
		}
		creds.Token = decrypted
	}

	if creds.RefreshToken != "" {
		decrypted, err := s.decrypt(creds.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("decrypting refresh token: %w", err)
		}
		creds.RefreshToken = decrypted
	}

	return &creds, nil
}

// Delete removes stored credentials.
func (s *Store) Delete() error {
	credPath := filepath.Join(s.credentialsDir, DefaultCredentialsFile)

	if err := os.Remove(credPath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("removing credentials file: %w", err)
	}

	return nil
}

// Exists checks if credentials file exists.
func (s *Store) Exists() bool {
	credPath := filepath.Join(s.credentialsDir, DefaultCredentialsFile)
	_, err := os.Stat(credPath)
	return err == nil
}

// ensureDir creates the credentials directory if it doesn't exist.
func (s *Store) ensureDir() error {
	return os.MkdirAll(s.credentialsDir, 0700)
}

// encrypt encrypts a string using AES-GCM.
func (s *Store) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("%w: creating cipher: %v", ErrEncryptionFailed, err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("%w: creating GCM: %v", ErrEncryptionFailed, err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("%w: generating nonce: %v", ErrEncryptionFailed, err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts an AES-GCM encrypted string.
func (s *Store) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("%w: decoding base64: %v", ErrEncryptionFailed, err)
	}

	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("%w: creating cipher: %v", ErrEncryptionFailed, err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("%w: creating GCM: %v", ErrEncryptionFailed, err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("%w: ciphertext too short", ErrEncryptionFailed)
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("%w: decryption failed: %v", ErrEncryptionFailed, err)
	}

	return string(plaintext), nil
}

// GetActiveCredential returns the currently active credential.
// It checks environment variables first, then falls back to stored credentials.
func (s *Store) GetActiveCredential() (*Credentials, error) {
	// Check environment variables first
	if apiKey := os.Getenv("PENF_API_KEY"); apiKey != "" {
		return &Credentials{
			AuthType: AuthTypeAPIKey,
			APIKey:   apiKey,
		}, nil
	}

	if token := os.Getenv("PENF_TOKEN"); token != "" {
		return &Credentials{
			AuthType: AuthTypeToken,
			Token:    token,
		}, nil
	}

	// Fall back to stored credentials
	creds, err := s.Load()
	if err != nil {
		return nil, err
	}

	// Check if token is expired
	if creds.AuthType == AuthTypeToken && !creds.ExpiresAt.IsZero() {
		if time.Now().After(creds.ExpiresAt) {
			return nil, ErrExpiredToken
		}
	}

	return creds, nil
}

// MaskCredential returns a masked version of the credential for display.
func MaskCredential(cred string) string {
	if len(cred) <= 8 {
		return strings.Repeat("*", len(cred))
	}
	return cred[:4] + strings.Repeat("*", len(cred)-8) + cred[len(cred)-4:]
}

// MaskAPIKey returns a masked API key showing only prefix.
func MaskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return strings.Repeat("*", len(apiKey))
	}
	// Show format like "pf-****...****" where prefix is visible
	if strings.HasPrefix(apiKey, "pf-") {
		return "pf-" + strings.Repeat("*", 8) + "..." + strings.Repeat("*", 4)
	}
	return apiKey[:4] + strings.Repeat("*", 8) + "..."
}

// MaskToken returns a masked token with first/last few characters visible.
func MaskToken(token string) string {
	if len(token) <= 20 {
		return strings.Repeat("*", len(token))
	}
	return token[:8] + "..." + token[len(token)-8:]
}

// FormatExpiry formats the expiry time for display.
func FormatExpiry(expiresAt time.Time) string {
	if expiresAt.IsZero() {
		return "never"
	}

	remaining := time.Until(expiresAt)
	if remaining < 0 {
		return "expired"
	}

	if remaining < time.Hour {
		return fmt.Sprintf("%d minutes", int(remaining.Minutes()))
	}
	if remaining < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(remaining.Hours()))
	}
	return fmt.Sprintf("%d days", int(remaining.Hours()/24))
}

// GenerateAPIKeyID creates a short ID for an API key (for display purposes).
func GenerateAPIKeyID(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:4])
}
