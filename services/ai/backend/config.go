package backend

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ConcurrencyConfigLoader reads model.concurrency.* keys from pipeline_operational_config.
type ConcurrencyConfigLoader struct {
	pool     *pgxpool.Pool
	tenantID string
}

// NewConcurrencyConfigLoader creates a loader that reads per-provider concurrency limits from DB.
func NewConcurrencyConfigLoader(pool *pgxpool.Pool, tenantID string) *ConcurrencyConfigLoader {
	return &ConcurrencyConfigLoader{pool: pool, tenantID: tenantID}
}

// Load reads all model.concurrency.* keys for the tenant and returns a provider→limit map.
// Returns an error if model.concurrency.default is not present (startup validation).
func (l *ConcurrencyConfigLoader) Load() (map[string]int, error) {
	rows, err := l.pool.Query(context.Background(), `
		SELECT key, value FROM pipeline_operational_config
		WHERE tenant_id = $1 AND key LIKE 'model.concurrency.%'
	`, l.tenantID)
	if err != nil {
		return nil, fmt.Errorf("query concurrency config: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan concurrency config row: %w", err)
		}
		provider := strings.TrimPrefix(key, "model.concurrency.")
		n, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("invalid concurrency value for %s=%q: %w", key, value, err)
		}
		result[provider] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate concurrency config rows: %w", err)
	}

	if _, ok := result["default"]; !ok {
		return nil, fmt.Errorf("model.concurrency.default not found in pipeline_operational_config for tenant %s", l.tenantID)
	}

	return result, nil
}

// Loader returns a closure suitable for CompositeBackend.configLoader.
func (l *ConcurrencyConfigLoader) Loader() func() (map[string]int, error) {
	return l.Load
}
