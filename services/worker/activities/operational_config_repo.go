package activities

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresOperationalConfigReader reads from pipeline_operational_config using pgx.
// Includes a per-tenant TTL cache to avoid redundant DB queries for stable config values.
type PostgresOperationalConfigReader struct {
	pool *pgxpool.Pool

	mu    sync.RWMutex
	cache map[string]cachedEntry // key: "tenantID:configKey"
	ttl   time.Duration
}

type cachedEntry struct {
	value     string
	err       error
	expiresAt time.Time
}

// NewPostgresOperationalConfigReader creates a new operational config reader.
func NewPostgresOperationalConfigReader(pool *pgxpool.Pool) *PostgresOperationalConfigReader {
	return &PostgresOperationalConfigReader{
		pool:  pool,
		cache: make(map[string]cachedEntry),
		ttl:   60 * time.Second,
	}
}

// GetString reads a single string value from pipeline_operational_config.
// Results are cached per tenant+key for 60 seconds.
func (r *PostgresOperationalConfigReader) GetString(ctx context.Context, tenantID, key string) (string, error) {
	cacheKey := tenantID + ":" + key

	// Check cache
	r.mu.RLock()
	if entry, ok := r.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		r.mu.RUnlock()
		return entry.value, entry.err
	}
	r.mu.RUnlock()

	// Cache miss — query DB
	var value string
	err := r.pool.QueryRow(ctx, `
		SELECT value FROM pipeline_operational_config
		WHERE tenant_id = $1 AND key = $2
	`, tenantID, key).Scan(&value)

	var resultErr error
	if errors.Is(err, pgx.ErrNoRows) {
		resultErr = fmt.Errorf("config key %q not found for tenant %s", key, tenantID)
	} else if err != nil {
		// Don't cache DB errors — retry next call
		return "", fmt.Errorf("query operational config %q: %w", key, err)
	}

	// Cache the result (including not-found)
	r.mu.Lock()
	r.cache[cacheKey] = cachedEntry{value: value, err: resultErr, expiresAt: time.Now().Add(r.ttl)}
	r.mu.Unlock()

	return value, resultErr
}
