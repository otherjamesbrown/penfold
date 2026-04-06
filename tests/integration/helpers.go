//go:build integration

// Package integration provides test helpers for integration tests.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/db"
	"github.com/otherjamesbrown/penfold/pkg/testfixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// testRunTenantID is generated once per test binary execution.
// Using a per-run UUID prevents data collisions when multiple developers or
// CI pipelines run integration tests concurrently against the shared live database.
var testRunTenantID = uuid.New().String()

// DefaultTestTenantID is the database default tenant used by repositories
// that don't explicitly specify tenant_id. This matches the schema default.
const DefaultTestTenantID = "00000001-0000-0000-0000-000000000001"

// TestDB holds the database connection for integration tests.
type TestDB struct {
	Pool     *pgxpool.Pool
	Name     string
	TenantID string
}

// SetupTestDB creates a connection to the test database.
// Uses the production database with tenant isolation.
// It reads configuration from environment variables:
//   - PENFOLD_DB_HOST (default: dev02.brown.chat)
//   - PENFOLD_DB_PORT (default: 5432)
//   - PENFOLD_DB_USER (default: penfold)
//   - PENFOLD_DB_NAME (default: penfold - uses production DB with tenant isolation)
func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()

	host := getEnvOrDefault("PENFOLD_DB_HOST", "dev02.brown.chat")
	port := getEnvOrDefault("PENFOLD_DB_PORT", "5432")
	user := getEnvOrDefault("PENFOLD_DB_USER", "penfold")
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold")

	// Build connection string with SSL cert auth
	// Certs are expected in ~/.postgresql/ (standard libpq location)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}
	sslCert := filepath.Join(homeDir, ".postgresql", "postgresql.crt")
	sslKey := filepath.Join(homeDir, ".postgresql", "postgresql.key")
	sslRootCert := filepath.Join(homeDir, ".postgresql", "root.crt")

	// Check if SSL certs exist
	if _, err := os.Stat(sslCert); os.IsNotExist(err) {
		t.Skip("SSL certs not found in ~/.postgresql/ - skipping integration test")
	}

	connStr := fmt.Sprintf(
		"postgres://%s@%s:%s/%s?sslmode=verify-full&sslcert=%s&sslkey=%s&sslrootcert=%s",
		user, host, port, dbName, sslCert, sslKey, sslRootCert,
	)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to connect to test database")

	// Verify connection
	err = pool.Ping(ctx)
	require.NoError(t, err, "failed to ping test database")

	// Sanity-check: ensure the shared DB has no pending migrations.
	// With the live shared DB approach, migrations are NOT applied automatically
	// during test runs — they must be applied manually before running tests.
	// This check fails fast if the DB is behind, so tests don't silently pass
	// against stale schema.
	cwd, err := os.Getwd()
	require.NoError(t, err, "failed to get working directory")

	// Walk up to find migrations directory
	migrationsDir := ""
	for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			migrationsDir = candidate
			break
		}
	}

	if migrationsDir != "" {
		pending, err := db.GetPendingMigrations(ctx, pool, migrationsDir)
		require.NoError(t, err, "failed to check migration status")
		if len(pending) > 0 {
			var names []string
			for _, m := range pending {
				names = append(names, m.Name)
			}
			t.Fatalf(
				"DB has %d pending migration(s) — apply them before running tests:\n\n"+
					"  penf migrate up\n\n"+
					"Pending migrations:\n  %s\n\n"+
					"See docs/testing-framework/LOCAL-SETUP.md for the migration workflow.",
				len(pending), strings.Join(names, "\n  "),
			)
		}
	}

	testDB := &TestDB{
		Pool:     pool,
		Name:     dbName,
		TenantID: testRunTenantID,
	}

	// Register cleanup
	t.Cleanup(func() {
		pool.Close()
	})

	return testDB
}


// CleanupTestTenant deletes all test data from all tenant-scoped tables.
// Deletes data for the per-run tenant ID and DefaultTestTenantID.
// This preserves production tenants' data in the shared database.
func (db *TestDB) CleanupTestTenant(t *testing.T) {
	t.Helper()

	ctx := context.Background()

	// Find all tables with a tenant_id column (except tenants itself)
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT c.table_name
		FROM information_schema.columns c
		JOIN information_schema.tables t ON c.table_name = t.table_name
		WHERE c.table_schema = 'public'
		AND t.table_schema = 'public'
		AND t.table_type = 'BASE TABLE'
		AND c.column_name = 'tenant_id'
		AND c.table_name != 'tenants'
	`)
	require.NoError(t, err, "failed to query tenant tables")
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		err := rows.Scan(&tableName)
		require.NoError(t, err)
		tables = append(tables, tableName)
	}

	// Clean both test tenants: the per-run tenant and
	// the default tenant used by repositories without explicit tenant_id.
	tenantIDs := []string{db.TenantID, DefaultTestTenantID}

	// Delete test tenant's data from each table
	for _, table := range tables {
		for _, tenantID := range tenantIDs {
			_, err := db.Pool.Exec(ctx, fmt.Sprintf(
				"DELETE FROM %s WHERE tenant_id = $1", table), tenantID)
			if err != nil {
				// Log but continue - some tables might have FK constraints
				t.Logf("warning: could not clean %s for tenant %s: %v", table, tenantID, err)
			}
		}
	}
}

// TruncateAllTables is deprecated - use CleanupTestTenant instead.
// This method only cleans up the integration test tenant's data, not all data.
func (db *TestDB) TruncateAllTables(t *testing.T) {
	db.CleanupTestTenant(t)
}

// CleanupTables deletes test data from specific tables.
// Deletes data for the per-run tenant ID and DefaultTestTenantID.
// Tables that don't exist or don't have tenant_id are silently skipped.
func (db *TestDB) CleanupTables(t *testing.T, tables ...string) {
	t.Helper()

	ctx := context.Background()

	// Clean both test tenants: the per-run tenant and
	// the default tenant used by repositories without explicit tenant_id.
	tenantIDs := []string{db.TenantID, DefaultTestTenantID}

	for _, table := range tables {
		// Check if table has tenant_id column
		var hasTenantID bool
		err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = $1 AND column_name = 'tenant_id'
			)
		`, table).Scan(&hasTenantID)
		if err != nil {
			t.Logf("Warning: could not check table %s: %v", table, err)
			continue
		}
		if !hasTenantID {
			t.Logf("Warning: table %s has no tenant_id column, skipping", table)
			continue
		}

		for _, tenantID := range tenantIDs {
			_, err = db.Pool.Exec(ctx, fmt.Sprintf(
				"DELETE FROM %s WHERE tenant_id = $1", table), tenantID)
			if err != nil {
				t.Logf("Warning: could not clean %s for tenant %s: %v", table, tenantID, err)
			}
		}
	}
}

// TruncateTables is deprecated - use CleanupTables instead.
func (db *TestDB) TruncateTables(t *testing.T, tables ...string) {
	db.CleanupTables(t, tables...)
}

// Exec executes a SQL statement.
func (db *TestDB) Exec(t *testing.T, sql string, args ...any) {
	t.Helper()

	ctx := context.Background()
	_, err := db.Pool.Exec(ctx, sql, args...)
	require.NoError(t, err, "failed to execute SQL")
}

// LoadYAMLFixture loads a YAML file and unmarshals it into the provided type.
func LoadYAMLFixture[T any](t *testing.T, path string) T {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read fixture file: %s", path)

	var result T
	err = yaml.Unmarshal(data, &result)
	require.NoError(t, err, "failed to unmarshal YAML fixture: %s", path)

	return result
}

// LoadYAMLFixtureFromTestdata loads a YAML fixture from the testdata directory.
func LoadYAMLFixtureFromTestdata[T any](t *testing.T, filename string) T {
	t.Helper()

	path := filepath.Join("testdata", filename)
	return LoadYAMLFixture[T](t, path)
}

// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// RequireEnv fails the test if the environment variable is not set.
func RequireEnv(t *testing.T, key string) string {
	t.Helper()

	value := os.Getenv(key)
	if value == "" {
		t.Skipf("environment variable %s not set - skipping test", key)
	}
	return value
}

// EnsureTenantExists creates the integration test tenant if it doesn't exist.
// The slug is derived from the per-run tenant ID to avoid unique constraint
// conflicts when multiple test runs execute concurrently.
func (db *TestDB) EnsureTenantExists(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	slug := fmt.Sprintf("itest-%s", db.TenantID[:8])
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO tenants (id, name, display_name, slug, owner_email, created_at, updated_at)
		VALUES ($1, $2, 'Integration Test Tenant', $3, 'integration-test@example.com', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, db.TenantID, slug, slug)
	require.NoError(t, err, "failed to ensure tenant exists")
}

// FixtureLoader returns a fixture loader for the Acme Corp fixtures.
// Uses the integration test tenant ID.
func (db *TestDB) FixtureLoader() *testfixtures.Loader {
	fixtureDir := filepath.Join("..", "fixtures", "acme-corp")
	return testfixtures.NewLoaderWithTenant(db.Pool, fixtureDir, db.TenantID)
}

// Fixture loading state - ensures Acme Corp fixtures are loaded only once per test run.
var (
	fixturesOnce   sync.Once
	fixturesErr    error
	fixturesLoaded bool
)

// EnsureAcmeCorpFixtures loads Acme Corp fixtures exactly once per test run.
// Subsequent calls return immediately. Uses sync.Once for thread safety.
func EnsureAcmeCorpFixtures(t *testing.T, db *TestDB) {
	t.Helper()

	fixturesOnce.Do(func() {
		ctx := context.Background()

		// Ensure tenant exists first
		db.EnsureTenantExists(t)

		// Cleanup any existing test data before loading fixtures
		// This ensures idempotency across test runs
		loader := db.FixtureLoader()
		if err := loader.CleanupAll(ctx); err != nil {
			t.Logf("Warning: cleanup before fixture load: %v", err)
		}

		// Load fixtures using the loader
		fixturesErr = loader.LoadAcmeCorp(ctx)
		if fixturesErr == nil {
			fixturesLoaded = true
			t.Logf("Acme Corp fixtures loaded successfully for tenant %s", db.TenantID)
		}
	})

	if fixturesErr != nil {
		t.Fatalf("Failed to load Acme Corp fixtures: %v", fixturesErr)
	}
}

// runCLI executes the penf CLI with the given arguments and returns stdout, stderr, and error.
// Uses the default config file (~/.penf/config.yaml) which has mTLS settings.
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "penf", args...)
	cmd.Env = os.Environ()

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// runCLIWithJSON executes the penf CLI and parses the JSON output into the given type.
// It automatically adds -o json to the arguments.
func runCLIWithJSON[T any](t *testing.T, args ...string) (T, string, error) {
	t.Helper()

	var result T

	// Add JSON output flag
	argsWithJSON := append(args, "-o", "json")
	stdout, stderr, err := runCLI(t, argsWithJSON...)
	if err != nil {
		return result, stderr, err
	}

	// Parse JSON
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		return result, stderr, fmt.Errorf("failed to parse JSON: %w\nstdout: %s", err, stdout)
	}

	return result, stderr, nil
}

// assertJSONContains verifies that the JSON output contains all specified keys.
func assertJSONContains(t *testing.T, stdout string, keys ...string) {
	t.Helper()

	var data map[string]any
	err := json.Unmarshal([]byte(stdout), &data)
	require.NoError(t, err, "output should be valid JSON")

	for _, key := range keys {
		assert.Contains(t, data, key, "JSON should contain key: %s", key)
	}
}

// assertJSONArrayNotEmpty verifies that a JSON array at the given key is not empty.
func assertJSONArrayNotEmpty(t *testing.T, stdout string, arrayKey string) {
	t.Helper()

	var data map[string]any
	err := json.Unmarshal([]byte(stdout), &data)
	require.NoError(t, err, "output should be valid JSON")

	val, ok := data[arrayKey]
	require.True(t, ok, "JSON should contain key: %s", arrayKey)

	arr, ok := val.([]any)
	require.True(t, ok, "key %s should be an array", arrayKey)
	assert.NotEmpty(t, arr, "array %s should not be empty", arrayKey)
}

// uniqueTestID generates a unique test identifier to avoid collisions between test runs.
func uniqueTestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// cleanupGlossaryTerm removes a glossary term created during testing.
// Errors are ignored since cleanup is best-effort.
func cleanupGlossaryTerm(t *testing.T, term string) {
	t.Helper()
	_, _, _ = runCLI(t, "glossary", "remove", term)
}

// cleanupMeetingSeries removes a meeting series created during testing.
// Errors are ignored since cleanup is best-effort.
func cleanupMeetingSeries(t *testing.T, seriesID string) {
	t.Helper()
	_, _, _ = runCLI(t, "meeting", "series", "delete", seriesID)
}
