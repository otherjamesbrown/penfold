//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateSourceStatus_SourceSystemNotPersisted_Integration is a reproduction test for bug pf-6e5961.
//
// BUG: The Activities.UpdateSourceStatus implementation in activities.go has direct SQL
// that performs JSONB merge (lines 412-434). This SQL conditional guard (line 412) checks
// 4 fields but NOT SourceSystem:
//
//   if input.TriageCategory != "" || input.TriageImportance != "" || input.SkipDeep != nil || input.ContentSubtype != ""
//
// And the JSONB merge block (lines 415-420) writes 4 fields but NOT source_system:
//
//   jsonb_build_object(
//       'triage_category', $3::text,
//       'triage_importance', $4::text,
//       'skip_deep', $5::boolean,
//       'content_subtype', $6::text
//   )
//
// IMPACT: When source_system is set in the input, it is NOT persisted to ingestion_metadata.
// This means the classification work done in Stage 1 (auto_reply, human_email, jira, etc.)
// is computed but never saved to the database.
//
// ROOT CAUSE:
// 1. Line 412 does not check: || input.SourceSystem != ""
// 2. Lines 415-420 do not include: 'source_system', $7::text
// 3. Line 428 does not pass: input.SourceSystem
//
// VERIFICATION: This test:
// 1. Creates a test source
// 2. Calls UpdateSourceStatus with SourceSystem="human_email"
// 3. Reads the ingestion_metadata JSONB from the database
// 4. Asserts that source_system is present in the JSONB
// 5. THIS TEST WILL FAIL because the SQL does not persist source_system
func TestUpdateSourceStatus_SourceSystemNotPersisted_Integration(t *testing.T) {
	ctx := context.Background()

	// Setup database connection
	pool := setupTestDBForSourceSystem(t)
	defer pool.Close()

	tenantID := IntegrationTestTenantID
	logger := logging.MustGlobal()

	// Create Activities instance with real database connection
	activities := NewActivities(logger)
	activities.db = pool

	// Create a test source
	sourceID := createTestSourceForSourceSystem(t, ctx, pool, tenantID)
	defer cleanupTestSourceForSourceSystem(t, ctx, pool, sourceID)

	// Call UpdateSourceStatus with ONLY SourceSystem set
	// This exposes the bug: the condition on line 412 does NOT check SourceSystem,
	// so the entire metadata block is skipped
	input := workflows.UpdateSourceStatusInput{
		TenantID:     tenantID,
		SourceID:     sourceID,
		Status:       "completed",
		SourceSystem: "human_email", // Should be persisted to ingestion_metadata
	}

	err := activities.UpdateSourceStatus(ctx, input)
	require.NoError(t, err, "UpdateSourceStatus should succeed")

	// Query the database to verify source_system was persisted
	var ingestionMetadata map[string]interface{}
	query := `SELECT ingestion_metadata FROM sources WHERE id = $1 AND tenant_id = $2::uuid`
	var metadataBytes []byte
	err = pool.QueryRow(ctx, query, sourceID, tenantID).Scan(&metadataBytes)
	require.NoError(t, err, "Should read ingestion_metadata from database")

	// Parse JSONB to map
	if metadataBytes != nil {
		err = json.Unmarshal(metadataBytes, &ingestionMetadata)
		require.NoError(t, err, "Should parse ingestion_metadata JSONB")
	}

	// BUG ASSERTION: source_system should be in ingestion_metadata
	// EXPECTED: {"source_system": "human_email"}
	// ACTUAL: {} or null (because the SQL conditional on line 412 skipped the metadata block)
	//
	// The condition checks 4 fields but NOT SourceSystem, so when ONLY SourceSystem is set,
	// the entire block from lines 412-434 is skipped.
	//
	// Even if we set another field (e.g., TriageCategory) to trigger the block,
	// source_system would STILL be missing from the JSONB because lines 415-420 don't include it.
	if ingestionMetadata == nil || ingestionMetadata["source_system"] == nil {
		t.Errorf("BUG REPRODUCED: source_system is missing from ingestion_metadata")
		t.Errorf("Expected: source_system = %q", "human_email")
		t.Errorf("Actual: ingestion_metadata = %+v", ingestionMetadata)
		t.Errorf("Root cause: activities.go line 412 does not check input.SourceSystem")
		t.Errorf("Root cause: activities.go lines 415-420 do not include 'source_system' in jsonb_build_object")
	}

	require.NotNil(t, ingestionMetadata, "ingestion_metadata should not be null")
	require.Contains(t, ingestionMetadata, "source_system", "source_system should be in ingestion_metadata")
	assert.Equal(t, "human_email", ingestionMetadata["source_system"], "source_system should be 'human_email'")
}

// TestUpdateSourceStatus_SourceSystemWithOtherFields_Integration verifies that even when
// other triage fields ARE set (triggering the metadata block), source_system is still missing
// from the JSONB because it's not included in the jsonb_build_object.
func TestUpdateSourceStatus_SourceSystemWithOtherFields_Integration(t *testing.T) {
	ctx := context.Background()

	// Setup database connection
	pool := setupTestDBForSourceSystem(t)
	defer pool.Close()

	tenantID := IntegrationTestTenantID
	logger := logging.MustGlobal()

	// Create Activities instance with real database connection
	activities := NewActivities(logger)
	activities.db = pool

	// Create a test source
	sourceID := createTestSourceForSourceSystem(t, ctx, pool, tenantID)
	defer cleanupTestSourceForSourceSystem(t, ctx, pool, sourceID)

	// Call UpdateSourceStatus with SourceSystem AND another triage field
	// This triggers the metadata block (line 412 condition is true because TriageCategory is set)
	// BUT source_system is still missing from jsonb_build_object (lines 415-420)
	input := workflows.UpdateSourceStatusInput{
		TenantID:       tenantID,
		SourceID:       sourceID,
		Status:         "completed",
		TriageCategory: "technical_discussion", // This WILL be persisted
		SourceSystem:   "human_email",          // This will NOT be persisted (BUG)
	}

	err := activities.UpdateSourceStatus(ctx, input)
	require.NoError(t, err, "UpdateSourceStatus should succeed")

	// Query the database to verify what was persisted
	var ingestionMetadata map[string]interface{}
	query := `SELECT ingestion_metadata FROM sources WHERE id = $1 AND tenant_id = $2::uuid`
	var metadataBytes []byte
	err = pool.QueryRow(ctx, query, sourceID, tenantID).Scan(&metadataBytes)
	require.NoError(t, err, "Should read ingestion_metadata from database")

	// Parse JSONB to map
	if metadataBytes != nil {
		err = json.Unmarshal(metadataBytes, &ingestionMetadata)
		require.NoError(t, err, "Should parse ingestion_metadata JSONB")
	}

	// Verify triage_category WAS persisted (baseline check)
	require.NotNil(t, ingestionMetadata, "ingestion_metadata should not be null")
	require.Contains(t, ingestionMetadata, "triage_category", "triage_category should be in ingestion_metadata")
	assert.Equal(t, "technical_discussion", ingestionMetadata["triage_category"])

	// BUG ASSERTION: source_system should ALSO be in ingestion_metadata
	// EXPECTED: {"triage_category": "technical_discussion", "source_system": "human_email"}
	// ACTUAL: {"triage_category": "technical_discussion"} (source_system is missing)
	//
	// This proves that even when the metadata block IS triggered (line 412 condition is true),
	// source_system is STILL missing because it's not in jsonb_build_object (lines 415-420).
	if ingestionMetadata["source_system"] == nil {
		t.Errorf("BUG REPRODUCED: source_system is missing from ingestion_metadata even though metadata block was triggered")
		t.Errorf("Expected: source_system = %q", "human_email")
		t.Errorf("Actual: ingestion_metadata = %+v", ingestionMetadata)
		t.Errorf("Root cause: activities.go lines 415-420 do not include 'source_system' in jsonb_build_object")
		t.Errorf("triage_category WAS persisted, proving the metadata block ran, but source_system is missing")
	}

	require.Contains(t, ingestionMetadata, "source_system", "source_system should be in ingestion_metadata")
	assert.Equal(t, "human_email", ingestionMetadata["source_system"], "source_system should be 'human_email'")
}

// setupTestDBForSourceSystem creates a connection to the test database.
func setupTestDBForSourceSystem(t *testing.T) *pgxpool.Pool {
	t.Helper()

	host := getEnvOrDefault("PENFOLD_DB_HOST", "dev02.brown.chat")
	port := getEnvOrDefault("PENFOLD_DB_PORT", "5432")
	user := getEnvOrDefault("PENFOLD_DB_USER", "penfold")
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold")

	// Build connection string with SSL cert auth
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}
	sslCert := filepath.Join(homeDir, ".postgresql", "postgresql.crt")
	sslKey := filepath.Join(homeDir, ".postgresql", "postgresql.key")
	sslRootCert := filepath.Join(homeDir, ".postgresql", "root.crt")

	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=verify-full sslcert=%s sslkey=%s sslrootcert=%s",
		host, port, user, dbName, sslCert, sslKey, sslRootCert,
	)

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	// Verify connection
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}

	return pool
}

// createTestSourceForSourceSystem creates a test source record in the database.
func createTestSourceForSourceSystem(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string) int64 {
	t.Helper()

	var sourceID int64
	query := `
		INSERT INTO sources (
			tenant_id,
			source_system,
			external_id,
			content_hash,
			content_type,
			processing_status,
			ingestion_metadata
		) VALUES (
			$1::uuid,
			'gmail',
			'test-source-system-' || gen_random_uuid()::text,
			encode(sha256('test content'::bytea), 'hex'),
			'email',
			'pending',
			'{}'::jsonb
		)
		RETURNING id
	`
	err := pool.QueryRow(ctx, query, tenantID).Scan(&sourceID)
	require.NoError(t, err, "Should create test source")
	t.Logf("Created test source: %d", sourceID)
	return sourceID
}

// cleanupTestSourceForSourceSystem deletes the test source.
func cleanupTestSourceForSourceSystem(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sourceID int64) {
	t.Helper()

	query := `DELETE FROM sources WHERE id = $1`
	_, err := pool.Exec(ctx, query, sourceID)
	if err != nil {
		t.Logf("Warning: Failed to cleanup test source %d: %v", sourceID, err)
	} else {
		t.Logf("Cleaned up test source: %d", sourceID)
	}
}
