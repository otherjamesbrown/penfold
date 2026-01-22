//go:build integration

// Package integration provides test helpers for integration tests.
package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/testfixtures"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestDB holds the database connection for integration tests.
type TestDB struct {
	Pool *pgxpool.Pool
	Name string
}

// SetupTestDB creates a connection to the test database.
// It reads configuration from environment variables:
//   - PENFOLD_DB_HOST (default: home-01.brown.chat)
//   - PENFOLD_DB_PORT (default: 5432)
//   - PENFOLD_DB_USER (default: penfold)
//   - PENFOLD_DB_PASSWORD (required)
//   - PENFOLD_DB_NAME (default: penfold_test_integration)
func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()

	host := getEnvOrDefault("PENFOLD_DB_HOST", "home-01.brown.chat")
	port := getEnvOrDefault("PENFOLD_DB_PORT", "5432")
	user := getEnvOrDefault("PENFOLD_DB_USER", "penfold")
	password := os.Getenv("PENFOLD_DB_PASSWORD")
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold_test_integration")

	if password == "" {
		t.Skip("PENFOLD_DB_PASSWORD not set - skipping integration test")
	}

	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		user, password, host, port, dbName,
	)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to connect to test database")

	// Verify connection
	err = pool.Ping(ctx)
	require.NoError(t, err, "failed to ping test database")

	testDB := &TestDB{
		Pool: pool,
		Name: dbName,
	}

	// Register cleanup
	t.Cleanup(func() {
		pool.Close()
	})

	return testDB
}

// TruncateAllTables truncates all user tables in the database.
// This is used to reset state between tests.
func (db *TestDB) TruncateAllTables(t *testing.T) {
	t.Helper()

	ctx := context.Background()

	// Get all table names from the public schema
	rows, err := db.Pool.Query(ctx, `
		SELECT tablename
		FROM pg_tables
		WHERE schemaname = 'public'
		AND tablename NOT LIKE 'pg_%'
		AND tablename NOT LIKE 'sql_%'
		AND tablename != 'schema_migrations'
	`)
	require.NoError(t, err, "failed to query table names")
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		err := rows.Scan(&tableName)
		require.NoError(t, err)
		tables = append(tables, tableName)
	}

	if len(tables) == 0 {
		return
	}

	// Truncate all tables with CASCADE
	for _, table := range tables {
		_, err := db.Pool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			// Log but don't fail - some tables might have constraints
			t.Logf("warning: could not truncate %s: %v", table, err)
		}
	}
}

// TruncateTables truncates specific tables.
func (db *TestDB) TruncateTables(t *testing.T, tables ...string) {
	t.Helper()

	ctx := context.Background()
	for _, table := range tables {
		_, err := db.Pool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		require.NoError(t, err, "failed to truncate table %s", table)
	}
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

// FixtureLoader returns a fixture loader for the Acme Corp fixtures.
func (db *TestDB) FixtureLoader() *testfixtures.Loader {
	fixtureDir := filepath.Join("..", "fixtures", "acme-corp")
	return testfixtures.NewLoader(db.Pool, fixtureDir)
}
