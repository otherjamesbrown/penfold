// Package db provides database helper functions for gateway services.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
)

// GetIntConfig reads an integer-typed config value from pipeline_config.
// Returns the parsed integer value and any error encountered.
func GetIntConfig(ctx context.Context, db *sql.DB, key string) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("database not available")
	}

	var valueStr string
	err := db.QueryRowContext(ctx, `
		SELECT value
		FROM pipeline_config
		WHERE key = $1 AND value_type = 'integer'
	`, key).Scan(&valueStr)

	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("config key %s not found", key)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to query config key %s: %w", key, err)
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse config value for %s as integer: %w", key, err)
	}

	return value, nil
}

// SetIntConfig updates an integer-typed config value in pipeline_config.
// Returns any error encountered.
func SetIntConfig(ctx context.Context, db *sql.DB, key string, value int, updatedBy string) error {
	if db == nil {
		return fmt.Errorf("database not available")
	}

	if updatedBy == "" {
		return fmt.Errorf("updated_by is required")
	}

	result, err := db.ExecContext(ctx, `
		UPDATE pipeline_config
		SET value = $1, updated_at = NOW(), updated_by = $2
		WHERE key = $3 AND value_type = 'integer'
	`, strconv.Itoa(value), updatedBy, key)

	if err != nil {
		return fmt.Errorf("failed to update config key %s: %w", key, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("config key %s not found or not of type integer", key)
	}

	return nil
}
