//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrations_AllApplied verifies that the shared DB has no pending migrations.
// This is the canonical pre-test migration sanity check: if the DB is behind,
// the test fails immediately with an actionable error rather than letting tests
// run against stale schema and produce confusing failures.
func TestMigrations_AllApplied(t *testing.T) {
	testDB := SetupTestDB(t)
	migrationsDir := findMigrationsDir(t)

	pending, err := db.GetPendingMigrations(context.Background(), testDB.Pool, migrationsDir)
	require.NoError(t, err, "failed to check migration status")

	if len(pending) > 0 {
		var names []string
		for _, m := range pending {
			names = append(names, m.Name)
		}
		t.Fatalf(
			"DB has %d pending migration(s) — apply them before running tests:\n\n"+
				"  penf migrate up\n\nPending:\n  %s",
			len(pending), strings.Join(names, "\n  "),
		)
	}

	t.Logf("All migrations applied — DB schema is up to date")
}

// TestMigrations_VersionSequence verifies migration versions are sequential with no gaps.
func TestMigrations_VersionSequence(t *testing.T) {
	available := getAvailableMigrations(t)
	require.NotEmpty(t, available, "should have migration files")

	// Sort by version
	sort.Slice(available, func(i, j int) bool {
		return available[i].Version < available[j].Version
	})

	// Check for gaps (allow starting from non-1)
	var gaps []string
	for i := 1; i < len(available); i++ {
		expected := available[i-1].Version + 1
		actual := available[i].Version
		if actual != expected {
			gaps = append(gaps, fmt.Sprintf("gap between %d and %d", available[i-1].Version, actual))
		}
	}

	if len(gaps) > 0 {
		t.Errorf("Migration version gaps found:\n  %s", strings.Join(gaps, "\n  "))
	}
}

// TestMigrations_ContentID_ColumnExists specifically tests the content_id migration
// that caused production issues.
func TestMigrations_ContentID_ColumnExists(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// Check if content_id column exists in sources table
	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'sources'
			AND column_name = 'content_id'
		)
	`).Scan(&exists)
	require.NoError(t, err)

	if !exists {
		t.Fatal("sources.content_id column does not exist - migration 021 not applied")
	}

	// Verify the index exists
	var indexExists bool
	err = db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE tablename = 'sources'
			AND indexname = 'idx_sources_content_id'
		)
	`).Scan(&indexExists)
	require.NoError(t, err)

	if !indexExists {
		t.Error("idx_sources_content_id index does not exist")
	}
}

// TestMigrations_TenantsTable verifies the tenants table exists with correct schema.
func TestMigrations_TenantsTable(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// Check table exists
	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'tenants'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	require.True(t, exists, "tenants table should exist")

	// Check required columns
	requiredColumns := []string{"id", "slug", "name"}
	for _, col := range requiredColumns {
		var colExists bool
		err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'tenants'
				AND column_name = $1
			)
		`, col).Scan(&colExists)
		require.NoError(t, err)
		assert.True(t, colExists, "tenants.%s column should exist", col)
	}
}

// TestMigrations_SourcesTable verifies the sources table has all expected columns.
func TestMigrations_SourcesTable(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	requiredColumns := []string{
		"id",
		"tenant_id",
		"source_system",
		"external_id",
		"content_hash",
		"raw_content",
		"content_type",
		"processing_status",
		"created_at",
		"content_id", // Added by migration 021
	}

	for _, col := range requiredColumns {
		var exists bool
		err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'sources'
				AND column_name = $1
			)
		`, col).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "sources.%s column should exist", col)
	}
}

// TestMigrations_IngestJobsTable verifies the ingest_jobs table exists.
func TestMigrations_IngestJobsTable(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	var exists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'ingest_jobs'
		)
	`).Scan(&exists)
	require.NoError(t, err)
	require.True(t, exists, "ingest_jobs table should exist")

	// Check tenant_id column is UUID type
	var dataType string
	err = db.Pool.QueryRow(ctx, `
		SELECT data_type FROM information_schema.columns
		WHERE table_name = 'ingest_jobs'
		AND column_name = 'tenant_id'
	`).Scan(&dataType)
	require.NoError(t, err)
	assert.Equal(t, "uuid", dataType, "ingest_jobs.tenant_id should be UUID type")
}

// Migration represents a migration file
type Migration struct {
	Version int
	Name    string
	HasDown bool
}

// getAvailableMigrations reads migration files from the migrations directory
func getAvailableMigrations(t *testing.T) []Migration {
	t.Helper()

	// Find migrations directory
	migrationsDir := findMigrationsDir(t)

	files, err := os.ReadDir(migrationsDir)
	require.NoError(t, err, "failed to read migrations directory")

	// Pattern: NNN_name.sql
	pattern := regexp.MustCompile(`^(\d{3})_(.+)\.sql$`)

	var migrations []Migration
	seen := make(map[int]bool)

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		matches := pattern.FindStringSubmatch(f.Name())
		if matches == nil {
			continue
		}

		version, err := strconv.Atoi(matches[1])
		require.NoError(t, err)

		if seen[version] {
			continue // Already processed this version
		}
		seen[version] = true

		// Check if migration has down section
		content, err := os.ReadFile(filepath.Join(migrationsDir, f.Name()))
		require.NoError(t, err)

		hasDown := strings.Contains(string(content), "+goose Down") ||
			strings.Contains(string(content), "-- +goose Down")

		migrations = append(migrations, Migration{
			Version: version,
			Name:    matches[2],
			HasDown: hasDown,
		})
	}

	return migrations
}

// findMigrationsDir locates the migrations directory
func findMigrationsDir(t *testing.T) string {
	t.Helper()

	// Try relative paths from test location
	candidates := []string{
		"../../migrations",
		"../../../migrations",
		"migrations",
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}

	// Try from working directory
	wd, _ := os.Getwd()
	for dir := wd; dir != "/"; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "migrations")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	t.Fatal("migrations directory not found")
	return ""
}
