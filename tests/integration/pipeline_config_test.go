//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTenantID = "c3170310-78bd-409c-b186-126f40bfa6ad"

func TestPipelineOperationalConfig_TableExists(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'pipeline_operational_config'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists, "pipeline_operational_config table should exist")
}

func TestPipelineOperationalConfig_OldTableDropped(t *testing.T) {
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
	assert.False(t, exists, "pipeline_config table should have been dropped by migration 078")
}

func TestPipelineOperationalConfig_AllDefaultsPresent(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	expectedKeys := []string{
		"pipeline.max_concurrent",
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

	// Count total rows for this tenant
	var count int
	err := db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM pipeline_operational_config WHERE tenant_id = $1",
		testTenantID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 13, count, "expected 13 config rows (5 active + 8 dead activity timeouts)")

	// Verify each expected key exists
	for _, key := range expectedKeys {
		var value string
		err := db.Pool.QueryRow(ctx,
			"SELECT value FROM pipeline_operational_config WHERE tenant_id = $1 AND key = $2",
			testTenantID, key,
		).Scan(&value)
		require.NoError(t, err, "key %s should exist", key)
		assert.NotEmpty(t, value, "key %s should have a non-empty value", key)
	}
}

func TestPipelineOperationalConfig_UpdateAndRead(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	const testKey = "timeout.ai_client.request"

	// Read original value
	var originalValue string
	err := db.Pool.QueryRow(ctx,
		"SELECT value FROM pipeline_operational_config WHERE tenant_id = $1 AND key = $2",
		testTenantID, testKey,
	).Scan(&originalValue)
	require.NoError(t, err)

	// Update to a new value
	newValue := "180s"
	_, err = db.Pool.Exec(ctx,
		"UPDATE pipeline_operational_config SET value = $1, updated_at = now() WHERE tenant_id = $2 AND key = $3",
		newValue, testTenantID, testKey,
	)
	require.NoError(t, err)

	// Read back and verify
	var readBack string
	err = db.Pool.QueryRow(ctx,
		"SELECT value FROM pipeline_operational_config WHERE tenant_id = $1 AND key = $2",
		testTenantID, testKey,
	).Scan(&readBack)
	require.NoError(t, err)
	assert.Equal(t, newValue, readBack)

	// Restore original value
	_, err = db.Pool.Exec(ctx,
		"UPDATE pipeline_operational_config SET value = $1, updated_at = now() WHERE tenant_id = $2 AND key = $3",
		originalValue, testTenantID, testKey,
	)
	require.NoError(t, err)
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

// TestNotificationPipelinePromptOverrides verifies that all notification pipeline stages
// have the expected prompt_override values. pf-c4682e: summarize and extract_semantic
// must have prompt_override=2 (notification-tailored), which migration 142 sets.
func TestNotificationPipelinePromptOverrides(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	ctx := context.Background()

	// Expected prompt_override for each notification pipeline stage.
	// NULL means no override (use active prompt version).
	// 2 means use notification-tailored prompt version.
	type stageExpectation struct {
		stage          string
		wantOverride   *int
		mustHaveOverride bool
	}

	notificationOverride := 2
	expectations := []stageExpectation{
		{stage: "triage",           wantOverride: &notificationOverride, mustHaveOverride: true},
		{stage: "summarize",        wantOverride: &notificationOverride, mustHaveOverride: true},
		{stage: "extract_semantic", wantOverride: &notificationOverride, mustHaveOverride: true},
	}

	for _, exp := range expectations {
		t.Run(exp.stage, func(t *testing.T) {
			var promptOverride *int
			err := db.Pool.QueryRow(ctx, `
				SELECT prompt_override
				FROM pipeline_definitions
				WHERE tenant_id = $1
				  AND pipeline = 'notification'
				  AND stage = $2
			`, testTenantID, exp.stage).Scan(&promptOverride)
			require.NoError(t, err, "stage %s should exist in notification pipeline for tenant %s", exp.stage, testTenantID)

			if exp.mustHaveOverride {
				require.NotNil(t, promptOverride, "notification pipeline stage %q should have prompt_override set (pf-c4682e)", exp.stage)
				assert.Equal(t, *exp.wantOverride, *promptOverride,
					"notification pipeline stage %q should have prompt_override=%d", exp.stage, *exp.wantOverride)
			} else {
				assert.Nil(t, promptOverride, "notification pipeline stage %q should have NULL prompt_override", exp.stage)
			}
		})
	}
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
