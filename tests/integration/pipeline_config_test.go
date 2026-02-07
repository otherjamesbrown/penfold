//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipelineConfig_TableExists(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'pipeline_config'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists, "pipeline_config table should exist")
}

func TestPipelineConfig_AllDefaultsPresent(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	expectedKeys := []string{
		"timeout.ai_client.request",
		"timeout.activity.fast.start_to_close",
		"timeout.activity.fast.heartbeat",
		"timeout.activity.embedding.start_to_close",
		"timeout.activity.embedding.heartbeat",
		"timeout.activity.llm.start_to_close",
		"timeout.activity.llm.heartbeat",
		"timeout.activity.batch.start_to_close",
		"timeout.activity.batch.heartbeat",
		"timeout.http.backend.gemini",
		"timeout.http.backend.mlx",
		"timeout.schedule_to_close.default",
	}

	// Count total rows
	var count int
	err := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM pipeline_config").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 12, count, "expected 12 config rows")

	// Verify each key exists with parseable duration value
	for _, key := range expectedKeys {
		var value, valueType string
		err := db.Pool.QueryRow(ctx,
			"SELECT value, value_type FROM pipeline_config WHERE key = $1",
			key,
		).Scan(&value, &valueType)
		require.NoError(t, err, "key %s should exist", key)
		assert.Equal(t, "duration", valueType, "key %s should be type duration", key)

		_, parseErr := time.ParseDuration(value)
		assert.NoError(t, parseErr, "key %s value '%s' should be a parseable duration", key, value)
	}
}

func TestPipelineConfig_UpdateAndRead(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	const testKey = "timeout.ai_client.request"

	// Read original value
	var originalValue string
	err := db.Pool.QueryRow(ctx,
		"SELECT value FROM pipeline_config WHERE key = $1", testKey,
	).Scan(&originalValue)
	require.NoError(t, err)

	// Update to a new value
	newValue := "180s"
	_, err = db.Pool.Exec(ctx,
		"UPDATE pipeline_config SET value = $1, updated_at = now(), updated_by = 'integration_test' WHERE key = $2",
		newValue, testKey,
	)
	require.NoError(t, err)

	// Read back and verify
	var readBack string
	err = db.Pool.QueryRow(ctx,
		"SELECT value FROM pipeline_config WHERE key = $1", testKey,
	).Scan(&readBack)
	require.NoError(t, err)
	assert.Equal(t, newValue, readBack)

	// Restore original value
	_, err = db.Pool.Exec(ctx,
		"UPDATE pipeline_config SET value = $1, updated_at = now(), updated_by = 'integration_test_cleanup' WHERE key = $2",
		originalValue, testKey,
	)
	require.NoError(t, err)
}

func TestPipelineConfig_MinMaxEnforced(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// For every row, min < value < max (seed data should satisfy this)
	rows, err := db.Pool.Query(ctx, `
		SELECT key, value, min_value, max_value
		FROM pipeline_config
		WHERE value_type = 'duration' AND min_value IS NOT NULL AND max_value IS NOT NULL
	`)
	require.NoError(t, err)
	defer rows.Close()

	count := 0
	for rows.Next() {
		var key, valStr, minStr, maxStr string
		err := rows.Scan(&key, &valStr, &minStr, &maxStr)
		require.NoError(t, err)

		val, err := time.ParseDuration(valStr)
		require.NoError(t, err, "key %s: bad value %s", key, valStr)

		minVal, err := time.ParseDuration(minStr)
		require.NoError(t, err, "key %s: bad min %s", key, minStr)

		maxVal, err := time.ParseDuration(maxStr)
		require.NoError(t, err, "key %s: bad max %s", key, maxStr)

		assert.True(t, val >= minVal, "key %s: value %v should be >= min %v", key, val, minVal)
		assert.True(t, val <= maxVal, "key %s: value %v should be <= max %v", key, val, maxVal)
		count++
	}
	require.NoError(t, rows.Err())
	assert.True(t, count >= 12, "expected at least 12 rows with min/max, got %d", count)
}

func TestDeployHistory_TableExists(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'deploy_history'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists, "deploy_history table should exist")
}

func TestDeployHistory_InsertAndQuery(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	testID := uniqueTestID()
	serviceName := "integration-test-" + testID

	// INSERT a record
	var id int
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO deploy_history (service_name, commit, previous_commit, version, deployed_by, changes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, serviceName, "abc123", "def456", "v0.test.1", "integration_test", "test deployment").Scan(&id)
	require.NoError(t, err)
	assert.True(t, id > 0, "expected positive id, got %d", id)

	// Query it back
	var readService, readCommit, readVersion string
	err = db.Pool.QueryRow(ctx,
		"SELECT service_name, commit, version FROM deploy_history WHERE id = $1", id,
	).Scan(&readService, &readCommit, &readVersion)
	require.NoError(t, err)
	assert.Equal(t, serviceName, readService)
	assert.Equal(t, "abc123", readCommit)
	assert.Equal(t, "v0.test.1", readVersion)

	// Cleanup
	_, err = db.Pool.Exec(ctx, "DELETE FROM deploy_history WHERE id = $1", id)
	require.NoError(t, err)
}
