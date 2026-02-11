package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindMigrations(t *testing.T) {
	// Create a temporary directory with test migration files
	tmpDir, err := os.MkdirTemp("", "migrations_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test migration files
	files := []string{
		"001_create_users.sql",
		"002_add_email_column.sql",
		"003_create_posts.sql",
		"README.md", // Should be ignored
	}

	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("-- test"), 0644); err != nil {
			t.Fatalf("failed to create test file %s: %v", f, err)
		}
	}

	migrations, err := findMigrations(tmpDir)
	if err != nil {
		t.Fatalf("findMigrations failed: %v", err)
	}

	if len(migrations) != 3 {
		t.Errorf("expected 3 migrations, got %d", len(migrations))
	}

	// Verify order
	expectedVersions := []string{"001_create_users", "002_add_email_column", "003_create_posts"}
	for i, m := range migrations {
		if m.Version != expectedVersions[i] {
			t.Errorf("migration %d: expected version '%s', got '%s'", i, expectedVersions[i], m.Version)
		}
	}
}

func TestFindMigrations_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrations_empty")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	migrations, err := findMigrations(tmpDir)
	if err != nil {
		t.Fatalf("findMigrations failed: %v", err)
	}

	if len(migrations) != 0 {
		t.Errorf("expected 0 migrations, got %d", len(migrations))
	}
}

func TestFindMigrations_NonExistentDir(t *testing.T) {
	_, err := findMigrations("/nonexistent/path/to/migrations")
	if err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}

func TestRunMigrations_NilPool(t *testing.T) {
	_, err := RunMigrations(nil, nil, "/tmp")
	if err == nil {
		t.Error("expected error for nil pool, got nil")
	}
}

func TestGetPendingMigrations_NilPool(t *testing.T) {
	_, err := GetPendingMigrations(nil, nil, "/tmp")
	if err == nil {
		t.Error("expected error for nil pool, got nil")
	}
}

func TestRunMigrationsToTarget_NilPool(t *testing.T) {
	_, err := RunMigrationsToTarget(nil, nil, "/tmp", "001_test")
	if err == nil {
		t.Error("expected error for nil pool, got nil")
	}
}

func TestGetMigrationStatus_NilPool(t *testing.T) {
	_, err := GetMigrationStatus(nil, nil, "/tmp")
	if err == nil {
		t.Error("expected error for nil pool, got nil")
	}
}

// TestRunMigrationsToTarget tests running migrations up to a specific target version.
func TestRunMigrationsToTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	// Create a temporary directory with test migration files
	tmpDir, err := os.MkdirTemp("", "migrations_target_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test migration files
	migrations := map[string]string{
		"001_create_test_table.sql": "CREATE TABLE test_table_001 (id INT);",
		"002_add_column.sql":        "ALTER TABLE test_table_001 ADD COLUMN name TEXT;",
		"003_create_another.sql":    "CREATE TABLE test_table_003 (id INT);",
	}

	for filename, content := range migrations {
		err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
		require.NoError(t, err)
	}

	// Run migrations up to version 002
	result, err := RunMigrationsToTarget(ctx, pool, tmpDir, "002_add_column")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have applied migrations 001 and 002
	assert.Len(t, result.Applied, 2)
	assert.Contains(t, result.Applied, "001_create_test_table")
	assert.Contains(t, result.Applied, "002_add_column")

	// Migration 003 should not be applied
	assert.NotContains(t, result.Applied, "003_create_another")

	// Verify that only migrations 001 and 002 are in the database
	applied, err := getAppliedMigrations(ctx, pool)
	require.NoError(t, err)
	assert.True(t, applied["001_create_test_table"])
	assert.True(t, applied["002_add_column"])
	assert.False(t, applied["003_create_another"])

	// Clean up test tables
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS test_table_001")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS test_table_003")
	_, _ = pool.Exec(ctx, "DELETE FROM schema_migrations WHERE version LIKE '00%_test%' OR version LIKE '00%_add%' OR version LIKE '00%_create%'")
}

// TestRunMigrationsToTarget_AlreadyApplied tests that running to a target that's already applied doesn't rerun migrations.
func TestRunMigrationsToTarget_AlreadyApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	// Create a temporary directory with test migration files
	tmpDir, err := os.MkdirTemp("", "migrations_already_applied_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test migration files
	migrations := map[string]string{
		"001_test_already.sql": "CREATE TABLE test_already_001 (id INT);",
		"002_test_already.sql": "CREATE TABLE test_already_002 (id INT);",
	}

	for filename, content := range migrations {
		err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
		require.NoError(t, err)
	}

	// First run: apply all migrations
	result1, err := RunMigrationsToTarget(ctx, pool, tmpDir, "002_test_already")
	require.NoError(t, err)
	assert.Len(t, result1.Applied, 2)

	// Second run: run to same target
	result2, err := RunMigrationsToTarget(ctx, pool, tmpDir, "002_test_already")
	require.NoError(t, err)

	// Should have no new applied migrations
	assert.Len(t, result2.Applied, 0)
	// Should have skipped both
	assert.Len(t, result2.Skipped, 2)

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS test_already_001")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS test_already_002")
	_, _ = pool.Exec(ctx, "DELETE FROM schema_migrations WHERE version LIKE '00%_test_already'")
}

// TestRunMigrationsToTarget_InvalidTarget tests error handling for invalid target version.
func TestRunMigrationsToTarget_InvalidTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	// Create a temporary directory with test migration files
	tmpDir, err := os.MkdirTemp("", "migrations_invalid_target_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test migration file
	err = os.WriteFile(filepath.Join(tmpDir, "001_test_invalid.sql"), []byte("CREATE TABLE test_invalid (id INT);"), 0644)
	require.NoError(t, err)

	// Try to run to a non-existent target
	_, err = RunMigrationsToTarget(ctx, pool, tmpDir, "999_nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target version")
}

// TestGetMigrationStatus tests retrieving migration status with applied, pending, and drift.
func TestGetMigrationStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	// Create a temporary directory with test migration files
	tmpDir, err := os.MkdirTemp("", "migrations_status_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test migration files
	migrations := map[string]string{
		"001_status_test.sql": "CREATE TABLE status_test_001 (id INT);",
		"002_status_test.sql": "CREATE TABLE status_test_002 (id INT);",
		"003_status_test.sql": "CREATE TABLE status_test_003 (id INT);",
	}

	for filename, content := range migrations {
		err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
		require.NoError(t, err)
	}

	// Apply only migration 001
	result, err := RunMigrationsToTarget(ctx, pool, tmpDir, "001_status_test")
	require.NoError(t, err)
	assert.Len(t, result.Applied, 1)

	// Manually insert a "drift" migration that has no corresponding file
	_, err = pool.Exec(ctx, "INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2)", "999_drift_migration", time.Now())
	require.NoError(t, err)

	// Get migration status
	status, err := GetMigrationStatus(ctx, pool, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, status)

	// Verify applied migrations (should include 001 and the drift migration 999)
	assert.GreaterOrEqual(t, len(status.Applied), 2)
	foundApplied := false
	for _, m := range status.Applied {
		if m.Version == "001_status_test" {
			foundApplied = true
			assert.NotNil(t, m.AppliedAt)
		}
	}
	assert.True(t, foundApplied, "migration 001_status_test should be in applied list")

	// Verify pending migrations (should include 002 and 003)
	assert.GreaterOrEqual(t, len(status.Pending), 2)
	foundPending := 0
	for _, m := range status.Pending {
		if m.Version == "002_status_test" || m.Version == "003_status_test" {
			foundPending++
		}
	}
	assert.Equal(t, 2, foundPending, "migrations 002 and 003 should be pending")

	// Verify drift migrations (applied but no file)
	assert.GreaterOrEqual(t, len(status.Drift), 1)
	foundDrift := false
	for _, m := range status.Drift {
		if m.Version == "999_drift_migration" {
			foundDrift = true
			assert.NotNil(t, m.AppliedAt)
		}
	}
	assert.True(t, foundDrift, "migration 999_drift_migration should be in drift list")

	// Clean up
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS status_test_001")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS status_test_002")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS status_test_003")
	_, _ = pool.Exec(ctx, "DELETE FROM schema_migrations WHERE version LIKE '00%_status_test' OR version = '999_drift_migration'")
}

// TestGetMigrationStatus_Empty tests migration status with no migrations.
func TestGetMigrationStatus_Empty(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	pool := setupTestDB(t)
	defer pool.Close()

	// Create an empty temporary directory
	tmpDir, err := os.MkdirTemp("", "migrations_status_empty_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Get migration status
	status, err := GetMigrationStatus(ctx, pool, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, status)

	// Pending should be empty (no migration files)
	assert.Len(t, status.Pending, 0)

	// Applied and Drift may contain migrations from other tests,
	// but that's okay - we're just testing that the function works with no files
}

// setupTestDB creates a test database connection pool.
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	// Use DATABASE_URL from environment or default to dev02
	dbURL := getTestDatabaseURL(t)

	config, err := pgxpool.ParseConfig(dbURL)
	require.NoError(t, err)

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	require.NoError(t, err)

	// Verify connection
	err = pool.Ping(context.Background())
	require.NoError(t, err)

	return pool
}

// getTestDatabaseURL returns the database URL for testing.
func getTestDatabaseURL(t *testing.T) string {
	t.Helper()

	dbURL := "postgres://penfold:penfold123@dev02.brown.chat:5432/penfold?sslmode=disable"
	return dbURL
}
