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

// ConfigEntry represents a single configuration entry from pipeline_config.
type ConfigEntry struct {
	Key          string
	Value        time.Duration
	ValueType    string
	Description  string
	MinValue     time.Duration
	MaxValue     time.Duration
	DefaultValue time.Duration
	UpdatedAt    time.Time
	UpdatedBy    string
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
	onChange []func(key string, oldVal, newVal time.Duration)
}

// hardcodedDefaults provides fallback values when DB is unavailable.
var hardcodedDefaults = map[string]ConfigEntry{
	"timeout.ai_client.request": {
		Key:          "timeout.ai_client.request",
		Value:        120 * time.Second,
		ValueType:    "duration",
		Description:  "AI gRPC client per-request timeout",
		MinValue:     10 * time.Second,
		MaxValue:     600 * time.Second,
		DefaultValue: 120 * time.Second,
	},
	"timeout.activity.fast.start_to_close": {
		Key:          "timeout.activity.fast.start_to_close",
		Value:        30 * time.Second,
		ValueType:    "duration",
		Description:  "Fast activity StartToClose timeout",
		MinValue:     5 * time.Second,
		MaxValue:     120 * time.Second,
		DefaultValue: 30 * time.Second,
	},
	"timeout.activity.fast.heartbeat": {
		Key:          "timeout.activity.fast.heartbeat",
		Value:        10 * time.Second,
		ValueType:    "duration",
		Description:  "Fast activity heartbeat timeout",
		MinValue:     3 * time.Second,
		MaxValue:     60 * time.Second,
		DefaultValue: 10 * time.Second,
	},
	"timeout.activity.embedding.start_to_close": {
		Key:          "timeout.activity.embedding.start_to_close",
		Value:        120 * time.Second,
		ValueType:    "duration",
		Description:  "Embedding activity StartToClose timeout",
		MinValue:     30 * time.Second,
		MaxValue:     600 * time.Second,
		DefaultValue: 120 * time.Second,
	},
	"timeout.activity.embedding.heartbeat": {
		Key:          "timeout.activity.embedding.heartbeat",
		Value:        30 * time.Second,
		ValueType:    "duration",
		Description:  "Embedding activity heartbeat timeout",
		MinValue:     10 * time.Second,
		MaxValue:     120 * time.Second,
		DefaultValue: 30 * time.Second,
	},
	"timeout.activity.llm.start_to_close": {
		Key:          "timeout.activity.llm.start_to_close",
		Value:        600 * time.Second,
		ValueType:    "duration",
		Description:  "LLM activity StartToClose timeout",
		MinValue:     60 * time.Second,
		MaxValue:     1800 * time.Second,
		DefaultValue: 600 * time.Second,
	},
	"timeout.activity.llm.heartbeat": {
		Key:          "timeout.activity.llm.heartbeat",
		Value:        300 * time.Second,
		ValueType:    "duration",
		Description:  "LLM activity heartbeat timeout",
		MinValue:     30 * time.Second,
		MaxValue:     900 * time.Second,
		DefaultValue: 300 * time.Second,
	},
	"timeout.activity.batch.start_to_close": {
		Key:          "timeout.activity.batch.start_to_close",
		Value:        1800 * time.Second,
		ValueType:    "duration",
		Description:  "Batch activity StartToClose timeout",
		MinValue:     300 * time.Second,
		MaxValue:     7200 * time.Second,
		DefaultValue: 1800 * time.Second,
	},
	"timeout.activity.batch.heartbeat": {
		Key:          "timeout.activity.batch.heartbeat",
		Value:        300 * time.Second,
		ValueType:    "duration",
		Description:  "Batch activity heartbeat timeout",
		MinValue:     60 * time.Second,
		MaxValue:     1800 * time.Second,
		DefaultValue: 300 * time.Second,
	},
	"timeout.http.backend.gemini": {
		Key:          "timeout.http.backend.gemini",
		Value:        120 * time.Second,
		ValueType:    "duration",
		Description:  "Gemini backend HTTP timeout",
		MinValue:     10 * time.Second,
		MaxValue:     600 * time.Second,
		DefaultValue: 120 * time.Second,
	},
	"timeout.http.backend.mlx": {
		Key:          "timeout.http.backend.mlx",
		Value:        60 * time.Second,
		ValueType:    "duration",
		Description:  "MLX backend HTTP timeout",
		MinValue:     5 * time.Second,
		MaxValue:     300 * time.Second,
		DefaultValue: 60 * time.Second,
	},
	"timeout.schedule_to_close.default": {
		Key:          "timeout.schedule_to_close.default",
		Value:        3600 * time.Second,
		ValueType:    "duration",
		Description:  "Default ScheduleToClose timeout",
		MinValue:     600 * time.Second,
		MaxValue:     14400 * time.Second,
		DefaultValue: 3600 * time.Second,
	},
}

// New creates a new Config with hardcoded defaults.
// If db is nil, only defaults will be available.
func New(db DB) *Config {
	// Copy defaults to entries
	entries := make(map[string]ConfigEntry)
	for k, v := range hardcodedDefaults {
		entries[k] = v
	}

	return &Config{
		entries:  entries,
		defaults: hardcodedDefaults,
		db:       db,
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
func (c *Config) Set(ctx context.Context, key string, value time.Duration, updatedBy string) error {
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
		UPDATE pipeline_config
		SET value = $1, updated_at = now(), updated_by = $2
		WHERE key = $3
	`, value.String(), updatedBy, key)
	if err != nil {
		return fmt.Errorf("failed to update config in database: %w", err)
	}

	// Update local cache
	oldValue := entry.Value
	entry.Value = value
	entry.UpdatedAt = time.Now()
	entry.UpdatedBy = updatedBy

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
		SELECT key, value, value_type, description,
		       min_value, max_value, default_value,
		       COALESCE(updated_at, now()), COALESCE(updated_by, '')
		FROM pipeline_config
		WHERE value_type = 'duration'
		ORDER BY key
	`)
	if err != nil {
		return fmt.Errorf("failed to query pipeline_config: %w", err)
	}
	defer rows.Close()

	newEntries := make(map[string]ConfigEntry)

	for rows.Next() {
		var entry ConfigEntry
		var valueStr, minStr, maxStr, defaultStr string

		err := rows.Scan(
			&entry.Key,
			&valueStr,
			&entry.ValueType,
			&entry.Description,
			&minStr,
			&maxStr,
			&defaultStr,
			&entry.UpdatedAt,
			&entry.UpdatedBy,
		)
		if err != nil {
			return fmt.Errorf("failed to scan config entry: %w", err)
		}

		// Parse durations
		entry.Value, err = time.ParseDuration(valueStr)
		if err != nil {
			return fmt.Errorf("failed to parse value for %s: %w", entry.Key, err)
		}

		if minStr != "" {
			entry.MinValue, err = time.ParseDuration(minStr)
			if err != nil {
				return fmt.Errorf("failed to parse min_value for %s: %w", entry.Key, err)
			}
		}

		if maxStr != "" {
			entry.MaxValue, err = time.ParseDuration(maxStr)
			if err != nil {
				return fmt.Errorf("failed to parse max_value for %s: %w", entry.Key, err)
			}
		}

		entry.DefaultValue, err = time.ParseDuration(defaultStr)
		if err != nil {
			return fmt.Errorf("failed to parse default_value for %s: %w", entry.Key, err)
		}

		newEntries[entry.Key] = entry
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
