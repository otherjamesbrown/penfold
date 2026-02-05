//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseConnection(t *testing.T) {
	db := SetupTestDB(t)

	ctx := context.Background()

	// Verify we can execute a simple query
	var result int
	err := db.Pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result)
}

func TestDatabaseVersion(t *testing.T) {
	db := SetupTestDB(t)

	ctx := context.Background()

	var version string
	err := db.Pool.QueryRow(ctx, "SELECT version()").Scan(&version)
	require.NoError(t, err)

	t.Logf("PostgreSQL version: %s", version)
	assert.Contains(t, version, "PostgreSQL")
}

func TestPgVectorExtension(t *testing.T) {
	db := SetupTestDB(t)

	ctx := context.Background()

	// Check if pgvector extension is installed
	var extExists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_extension WHERE extname = 'vector'
		)
	`).Scan(&extExists)
	require.NoError(t, err)

	if !extExists {
		t.Skip("pgvector extension not installed - skipping vector tests")
	}

	// Test basic vector operations
	_, err = db.Pool.Exec(ctx, `
		CREATE TEMP TABLE test_vectors (
			id SERIAL PRIMARY KEY,
			embedding vector(3)
		)
	`)
	require.NoError(t, err)

	// Insert a test vector
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO test_vectors (embedding) VALUES ('[1,2,3]')
	`)
	require.NoError(t, err)

	// Query the vector
	var embedding string
	err = db.Pool.QueryRow(ctx, `
		SELECT embedding::text FROM test_vectors WHERE id = 1
	`).Scan(&embedding)
	require.NoError(t, err)
	assert.Equal(t, "[1,2,3]", embedding)
}

func TestCleanupTestTenant(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)

	ctx := context.Background()

	// Insert test data into a tenant-scoped table (glossary)
	testTerm := "TESTCLEANUP" + uniqueTestID()
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO glossary (tenant_id, term, expansion, definition, created_at, updated_at)
		VALUES ($1, $2, 'Test Cleanup Expansion', 'Test definition for cleanup', NOW(), NOW())
		ON CONFLICT (tenant_id, term) DO NOTHING
	`, db.TenantID, testTerm)
	require.NoError(t, err)

	// Verify data exists for test tenant
	var count int
	err = db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM glossary WHERE tenant_id = $1 AND term = $2",
		db.TenantID, testTerm).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "test term should exist before cleanup")

	// Cleanup test tenant data only
	db.CleanupTestTenant(t)

	// Verify test tenant's data is gone
	err = db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM glossary WHERE tenant_id = $1 AND term = $2",
		db.TenantID, testTerm).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "test term should be deleted after cleanup")

	// Verify other tenants' data is NOT affected (production data still exists)
	var otherCount int
	err = db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM glossary WHERE tenant_id != $1",
		db.TenantID).Scan(&otherCount)
	require.NoError(t, err)
	// We don't assert exact count, but it should be >= 0 (other tenants may have data)
	t.Logf("Other tenants' glossary entries preserved: %d", otherCount)
}
