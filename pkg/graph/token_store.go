package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TokenStore persists and loads Graph API tokens via the tenant_integrations table.
type TokenStore struct {
	pool *pgxpool.Pool
}

// NewTokenStore creates a new TokenStore.
func NewTokenStore(pool *pgxpool.Pool) *TokenStore {
	return &TokenStore{pool: pool}
}

// StoreToken saves a token in the tenant_integrations config JSONB field
// for the microsoft_graph integration.
func (s *TokenStore) StoreToken(ctx context.Context, tenantID string, token *StoredToken) error {
	// Read current config, merge in token
	cfg, err := s.loadConfig(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("graph: loading config for token store: %w", err)
	}

	cfg.Token = token

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("graph: marshaling config: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE tenant_integrations
		SET config = $1::jsonb, updated_at = NOW()
		WHERE tenant_id = $2 AND integration_type = 'microsoft_graph'
	`, configJSON, tenantID)
	if err != nil {
		return fmt.Errorf("graph: storing token: %w", err)
	}

	return nil
}

// LoadToken retrieves the token from tenant_integrations for a microsoft_graph integration.
func (s *TokenStore) LoadToken(ctx context.Context, tenantID string) (*StoredToken, error) {
	cfg, err := s.loadConfig(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	if cfg.Token == nil {
		return nil, fmt.Errorf("graph: no token stored for tenant %s", tenantID)
	}

	return cfg.Token, nil
}

// LoadConfig retrieves the full IntegrationConfig from tenant_integrations.
func (s *TokenStore) LoadConfig(ctx context.Context, tenantID string) (*IntegrationConfig, error) {
	return s.loadConfig(ctx, tenantID)
}

// RefreshTokenIfNeeded checks if the token expires within the given margin
// and returns whether a refresh is needed. The actual refresh must be done
// by the caller using the appropriate credential flow, since this store
// doesn't hold the refresh logic itself.
func (s *TokenStore) RefreshTokenIfNeeded(ctx context.Context, tenantID string, margin time.Duration) (*StoredToken, bool, error) {
	token, err := s.LoadToken(ctx, tenantID)
	if err != nil {
		return nil, false, err
	}

	needsRefresh := time.Now().Add(margin).After(token.Expiry)
	return token, needsRefresh, nil
}

func (s *TokenStore) loadConfig(ctx context.Context, tenantID string) (*IntegrationConfig, error) {
	var configJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT config FROM tenant_integrations
		WHERE tenant_id = $1 AND integration_type = 'microsoft_graph'
	`, tenantID).Scan(&configJSON)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("graph: no microsoft_graph integration found for tenant %s", tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("graph: querying integration config: %w", err)
	}

	var cfg IntegrationConfig
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return nil, fmt.Errorf("graph: unmarshaling integration config: %w", err)
	}

	return &cfg, nil
}
