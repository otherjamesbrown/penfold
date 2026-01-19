// Package oauth provides OAuth2 authentication with PKCE for Gmail API access.
package oauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TokenStorage defines the interface for token persistence.
type TokenStorage interface {
	// StoreToken stores or updates a token for a tenant.
	StoreToken(ctx context.Context, token *Token) error

	// GetToken retrieves a token for a tenant.
	GetToken(ctx context.Context, tenantID string) (*Token, error)

	// DeleteToken removes a token for a tenant.
	DeleteToken(ctx context.Context, tenantID string) error

	// ListTokens lists all tokens (for admin purposes).
	ListTokens(ctx context.Context) ([]*Token, error)
}

// PostgresTokenStorage implements TokenStorage using PostgreSQL.
type PostgresTokenStorage struct {
	pool      *pgxpool.Pool
	encryptor *TokenEncryptor
	tableName string
}

// PostgresTokenStorageConfig holds configuration for PostgresTokenStorage.
type PostgresTokenStorageConfig struct {
	// Pool is the PostgreSQL connection pool.
	Pool *pgxpool.Pool

	// EncryptionKey is the 32-byte key for AES-256-GCM encryption.
	EncryptionKey []byte

	// TableName is the table name for storing tokens.
	// Defaults to "oauth_tokens".
	TableName string
}

// NewPostgresTokenStorage creates a new PostgresTokenStorage.
func NewPostgresTokenStorage(cfg *PostgresTokenStorageConfig) (*PostgresTokenStorage, error) {
	if cfg.Pool == nil {
		return nil, fmt.Errorf("database pool is required")
	}

	if len(cfg.EncryptionKey) == 0 {
		return nil, fmt.Errorf("encryption key is required")
	}

	encryptor, err := NewTokenEncryptor(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("creating encryptor: %w", err)
	}

	tableName := cfg.TableName
	if tableName == "" {
		tableName = "oauth_tokens"
	}

	return &PostgresTokenStorage{
		pool:      cfg.Pool,
		encryptor: encryptor,
		tableName: tableName,
	}, nil
}

// EnsureTable creates the tokens table if it doesn't exist.
func (s *PostgresTokenStorage) EnsureTable(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			tenant_id VARCHAR(255) PRIMARY KEY,
			encrypted_data BYTEA NOT NULL,
			scopes TEXT[] NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_%s_expires_at ON %s (expires_at);
	`, s.tableName, s.tableName, s.tableName)

	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("creating oauth tokens table: %w", err)
	}

	return nil
}

// StoreToken stores or updates a token for a tenant.
func (s *PostgresTokenStorage) StoreToken(ctx context.Context, token *Token) error {
	// Serialize the token to JSON.
	tokenJSON, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("marshaling token: %w", err)
	}

	// Encrypt the token data.
	encryptedData, err := s.encryptor.Encrypt(tokenJSON)
	if err != nil {
		return fmt.Errorf("encrypting token: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (tenant_id, encrypted_data, scopes, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id) DO UPDATE SET
			encrypted_data = EXCLUDED.encrypted_data,
			scopes = EXCLUDED.scopes,
			expires_at = EXCLUDED.expires_at,
			updated_at = EXCLUDED.updated_at
	`, s.tableName)

	_, err = s.pool.Exec(ctx, query,
		token.TenantID,
		encryptedData,
		token.Scopes,
		token.ExpiresAt,
		token.CreatedAt,
		token.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("storing token: %w", err)
	}

	return nil
}

// GetToken retrieves a token for a tenant.
func (s *PostgresTokenStorage) GetToken(ctx context.Context, tenantID string) (*Token, error) {
	query := fmt.Sprintf(`
		SELECT encrypted_data FROM %s WHERE tenant_id = $1
	`, s.tableName)

	var encryptedData []byte
	err := s.pool.QueryRow(ctx, query, tenantID).Scan(&encryptedData)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("querying token: %w", err)
	}

	// Decrypt the token data.
	tokenJSON, err := s.encryptor.Decrypt(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("decrypting token: %w", err)
	}

	// Deserialize the token.
	var token Token
	if err := json.Unmarshal(tokenJSON, &token); err != nil {
		return nil, fmt.Errorf("unmarshaling token: %w", err)
	}

	return &token, nil
}

// DeleteToken removes a token for a tenant.
func (s *PostgresTokenStorage) DeleteToken(ctx context.Context, tenantID string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE tenant_id = $1`, s.tableName)

	result, err := s.pool.Exec(ctx, query, tenantID)
	if err != nil {
		return fmt.Errorf("deleting token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrTokenNotFound
	}

	return nil
}

// ListTokens lists all tokens (for admin purposes).
// Note: This returns metadata only, not the decrypted tokens.
func (s *PostgresTokenStorage) ListTokens(ctx context.Context) ([]*Token, error) {
	query := fmt.Sprintf(`
		SELECT encrypted_data FROM %s ORDER BY updated_at DESC
	`, s.tableName)

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*Token
	for rows.Next() {
		var encryptedData []byte
		if err := rows.Scan(&encryptedData); err != nil {
			return nil, fmt.Errorf("scanning token row: %w", err)
		}

		tokenJSON, err := s.encryptor.Decrypt(encryptedData)
		if err != nil {
			return nil, fmt.Errorf("decrypting token: %w", err)
		}

		var token Token
		if err := json.Unmarshal(tokenJSON, &token); err != nil {
			return nil, fmt.Errorf("unmarshaling token: %w", err)
		}

		// Clear sensitive data for listing.
		token.AccessToken = "[REDACTED]"
		token.RefreshToken = "[REDACTED]"

		tokens = append(tokens, &token)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating token rows: %w", err)
	}

	return tokens, nil
}

// CleanupExpiredTokens removes tokens that have been expired for more than the given duration.
func (s *PostgresTokenStorage) CleanupExpiredTokens(ctx context.Context, expiredFor time.Duration) (int64, error) {
	cutoff := time.Now().Add(-expiredFor)

	query := fmt.Sprintf(`DELETE FROM %s WHERE expires_at < $1`, s.tableName)

	result, err := s.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleaning up expired tokens: %w", err)
	}

	return result.RowsAffected(), nil
}

// MemoryTokenStorage implements TokenStorage using in-memory storage.
// Useful for testing.
type MemoryTokenStorage struct {
	tokens map[string]*Token
}

// NewMemoryTokenStorage creates a new MemoryTokenStorage.
func NewMemoryTokenStorage() *MemoryTokenStorage {
	return &MemoryTokenStorage{
		tokens: make(map[string]*Token),
	}
}

// StoreToken stores or updates a token.
func (s *MemoryTokenStorage) StoreToken(ctx context.Context, token *Token) error {
	// Make a copy to avoid mutation.
	tokenCopy := *token
	s.tokens[token.TenantID] = &tokenCopy
	return nil
}

// GetToken retrieves a token.
func (s *MemoryTokenStorage) GetToken(ctx context.Context, tenantID string) (*Token, error) {
	token, ok := s.tokens[tenantID]
	if !ok {
		return nil, ErrTokenNotFound
	}
	// Return a copy to avoid mutation.
	tokenCopy := *token
	return &tokenCopy, nil
}

// DeleteToken removes a token.
func (s *MemoryTokenStorage) DeleteToken(ctx context.Context, tenantID string) error {
	if _, ok := s.tokens[tenantID]; !ok {
		return ErrTokenNotFound
	}
	delete(s.tokens, tenantID)
	return nil
}

// ListTokens lists all tokens.
func (s *MemoryTokenStorage) ListTokens(ctx context.Context) ([]*Token, error) {
	tokens := make([]*Token, 0, len(s.tokens))
	for _, token := range s.tokens {
		tokenCopy := *token
		tokenCopy.AccessToken = "[REDACTED]"
		tokenCopy.RefreshToken = "[REDACTED]"
		tokens = append(tokens, &tokenCopy)
	}
	return tokens, nil
}

// TokenRow represents a row in the oauth_tokens table for SQL scanning.
type TokenRow struct {
	TenantID      string
	EncryptedData []byte
	Scopes        []string
	ExpiresAt     time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Ensure interfaces are implemented.
var _ TokenStorage = (*PostgresTokenStorage)(nil)
var _ TokenStorage = (*MemoryTokenStorage)(nil)

// NullTime is a helper for handling nullable timestamps.
type NullTime struct {
	Time  time.Time
	Valid bool
}

// Scan implements the sql.Scanner interface.
func (nt *NullTime) Scan(value interface{}) error {
	if value == nil {
		nt.Time, nt.Valid = time.Time{}, false
		return nil
	}
	nt.Valid = true
	switch v := value.(type) {
	case time.Time:
		nt.Time = v
	default:
		return fmt.Errorf("cannot scan %T into NullTime", value)
	}
	return nil
}

// Value implements the driver.Valuer interface.
func (nt NullTime) Value() (interface{}, error) {
	if !nt.Valid {
		return nil, nil
	}
	return nt.Time, nil
}

// Ensure NullTime implements sql.Scanner.
var _ sql.Scanner = (*NullTime)(nil)
