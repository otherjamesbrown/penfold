// Package timeout provides runtime configuration management for pipeline timeouts.
package timeout

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ConfigEntry represents a single configuration entry from pipeline_operational_config.
type ConfigEntry struct {
	Key   string
	Value time.Duration
	// MinValue and MaxValue are retained for in-memory validation only; they are
	// no longer stored in the database and are populated from hardcodedDefaults.
	MinValue time.Duration
	MaxValue time.Duration
}

// DB defines the interface for database operations, allowing for testing with mocks.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Config provides thread-safe access to timeout configuration.
type Config struct {
	mu       sync.RWMutex
	entries  map[string]ConfigEntry
	defaults map[string]ConfigEntry
	db       DB
	tenantID string
	onChange []func(key string, oldVal, newVal time.Duration)
}

// hardcodedDefaults provides fallback values when DB is unavailable.
// Only active timeout keys are listed here; dead keys (timeout.activity.*,
// timeout.stage.*) were removed when those config rows were deleted.
var hardcodedDefaults = map[string]ConfigEntry{
	"timeout.ai_client.request": {
		Key:      "timeout.ai_client.request",
		Value:    120 * time.Second,
		MinValue: 10 * time.Second,
		MaxValue: 600 * time.Second,
	},
	"timeout.http.backend.gemini": {
		Key:      "timeout.http.backend.gemini",
		Value:    120 * time.Second,
		MinValue: 10 * time.Second,
		MaxValue: 600 * time.Second,
	},
	"timeout.http.backend.mlx": {
		Key:      "timeout.http.backend.mlx",
		Value:    60 * time.Second,
		MinValue: 5 * time.Second,
		MaxValue: 300 * time.Second,
	},
	"timeout.schedule_to_close.default": {
		Key:      "timeout.schedule_to_close.default",
		Value:    3600 * time.Second,
		MinValue: 600 * time.Second,
		MaxValue: 14400 * time.Second,
	},
}

// New creates a new Config with hardcoded defaults.
// tenantID is required for database queries against pipeline_operational_config.
// If db is nil, only defaults will be available.
func New(db DB, tenantID string) *Config {
	// Copy defaults to entries
	entries := make(map[string]ConfigEntry)
	for k, v := range hardcodedDefaults {
		entries[k] = v
	}

	return &Config{
		entries:  entries,
		defaults: hardcodedDefaults,
		db:       db,
		tenantID: tenantID,
		onChange: make([]func(string, time.Duration, time.Duration), 0),
	}
}

// Get returns the duration for the given key.
// Falls back to default if key is not found.
// Returns 0 if key is unknown.
func (c *Config) Get(key string) time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if entry, ok := c.entries[key]; ok {
		return entry.Value
	}

	return 0
}

// GetEntry returns the full ConfigEntry for the given key.
// Returns false if key is not found.
func (c *Config) GetEntry(key string) (ConfigEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	return entry, ok
}

// All returns all configuration entries.
func (c *Config) All() []ConfigEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make([]ConfigEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		entries = append(entries, entry)
	}

	return entries
}

// Set updates a timeout value in the database and local cache.
// Validates that the value is within min/max bounds.
func (c *Config) Set(ctx context.Context, key string, value time.Duration) error {
	if c.db == nil {
		return fmt.Errorf("cannot set config: database not available")
	}

	// Get current entry to check min/max
	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		return fmt.Errorf("unknown config key: %s", key)
	}

	// Validate min/max
	if entry.MinValue > 0 && value < entry.MinValue {
		return fmt.Errorf("value %s is below minimum %s for key %s", value, entry.MinValue, key)
	}
	if entry.MaxValue > 0 && value > entry.MaxValue {
		return fmt.Errorf("value %s is above maximum %s for key %s", value, entry.MaxValue, key)
	}

	// Update in database
	_, err := c.db.Exec(ctx, `
		UPDATE pipeline_operational_config
		SET value = $1, updated_at = now()
		WHERE tenant_id = $2 AND key = $3
	`, value.String(), c.tenantID, key)
	if err != nil {
		return fmt.Errorf("failed to update config in database: %w", err)
	}

	// Update local cache
	oldValue := entry.Value
	entry.Value = value

	c.mu.Lock()
	c.entries[key] = entry
	callbacks := c.onChange
	c.mu.Unlock()

	// Fire callbacks
	for _, cb := range callbacks {
		cb(key, oldValue, value)
	}

	return nil
}

// Refresh reloads all configuration entries from the database.
// If the database is unavailable, keeps existing values and returns error.
func (c *Config) Refresh(ctx context.Context) error {
	if c.db == nil {
		return nil // No database, nothing to refresh
	}

	rows, err := c.db.Query(ctx, `
		SELECT key, value
		FROM pipeline_operational_config
		WHERE tenant_id = $1 AND key LIKE 'timeout.%'
		ORDER BY key
	`, c.tenantID)
	if err != nil {
		return fmt.Errorf("failed to query pipeline_operational_config: %w", err)
	}
	defer rows.Close()

	// Start with hardcoded defaults so any key not in the DB retains its default.
	newEntries := make(map[string]ConfigEntry)
	for k, v := range c.defaults {
		newEntries[k] = v
	}

	for rows.Next() {
		var key, valueStr string

		err := rows.Scan(&key, &valueStr)
		if err != nil {
			return fmt.Errorf("failed to scan config entry: %w", err)
		}

		// Parse duration value
		dur, err := time.ParseDuration(valueStr)
		if err != nil {
			return fmt.Errorf("failed to parse value for %s: %w", key, err)
		}

		// Preserve min/max bounds from defaults if the key is known
		entry := ConfigEntry{Key: key, Value: dur}
		if def, ok := c.defaults[key]; ok {
			entry.MinValue = def.MinValue
			entry.MaxValue = def.MaxValue
		}
		newEntries[key] = entry
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating config rows: %w", err)
	}

	// Update cache and fire callbacks
	c.mu.Lock()
	oldEntries := c.entries
	c.entries = newEntries
	callbacks := c.onChange
	c.mu.Unlock()

	// Fire callbacks for changed values
	for key, newEntry := range newEntries {
		if oldEntry, ok := oldEntries[key]; ok && oldEntry.Value != newEntry.Value {
			for _, cb := range callbacks {
				cb(key, oldEntry.Value, newEntry.Value)
			}
		}
	}

	return nil
}

// OnChange registers a callback function that will be called when a config value changes.
// The callback receives the key, old value, and new value.
func (c *Config) OnChange(fn func(key string, oldVal, newVal time.Duration)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onChange = append(c.onChange, fn)
}

// StartRefreshLoop starts a background goroutine that periodically refreshes configuration.
// The goroutine will exit when ctx is cancelled.
func (c *Config) StartRefreshLoop(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.Refresh(ctx); err != nil {
					// Log error but continue (keeps last-known values)
					// In production, this would use a proper logger
					_ = err
				}
			}
		}
	}()
}
