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

func TestTruncateAllTables(t *testing.T) {
	db := SetupTestDB(t)

	ctx := context.Background()

	// Create a test table and insert data
	_, err := db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS test_truncate (
			id SERIAL PRIMARY KEY,
			name TEXT
		)
	`)
	require.NoError(t, err)

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO test_truncate (name) VALUES ('test1'), ('test2')
	`)
	require.NoError(t, err)

	// Verify data exists
	var count int
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_truncate").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Truncate all tables
	db.TruncateAllTables(t)

	// Verify data is gone
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_truncate").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Cleanup
	_, _ = db.Pool.Exec(ctx, "DROP TABLE test_truncate")
}
