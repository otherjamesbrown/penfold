//go:build e2e

// Package e2e provides helpers for end-to-end tests.
package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/testfixtures"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// E2EEnv holds the test environment for E2E tests.
type E2EEnv struct {
	DB         *pgxpool.Pool
	DBName     string
	LLMURL     string
	FixtureDir string
	t          *testing.T
}

// SetupE2EEnvironment creates the E2E test environment.
// It requires:
//   - Database connection (same as integration tests)
//   - Local LLM server running at LLM_URL
func SetupE2EEnvironment(t *testing.T) *E2EEnv {
	t.Helper()

	// Setup database connection
	host := getEnvOrDefault("PENFOLD_DB_HOST", "home-01.brown.chat")
	port := getEnvOrDefault("PENFOLD_DB_PORT", "5432")
	user := getEnvOrDefault("PENFOLD_DB_USER", "penfold")
	password := os.Getenv("PENFOLD_DB_PASSWORD")
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold_test_e2e")
	llmURL := getEnvOrDefault("LLM_URL", "http://localhost:8080")

	if password == "" {
		t.Skip("PENFOLD_DB_PASSWORD not set - skipping E2E test")
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

	env := &E2EEnv{
		DB:         pool,
		DBName:     dbName,
		LLMURL:     llmURL,
		FixtureDir: filepath.Join("..", "fixtures", "acme-corp"),
		t:          t,
	}

	// Check LLM availability
	if !env.LLMAvailable() {
		pool.Close()
		t.Skip("Local LLM not available - skipping E2E test")
	}

	// Register cleanup
	t.Cleanup(func() {
		pool.Close()
	})

	return env
}

// LLMAvailable checks if the local LLM server is running.
func (env *E2EEnv) LLMAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.LLMURL+"/v1/models", nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// LoadFixture loads the Acme Corp fixture data into the database.
func (env *E2EEnv) LoadFixture(name string) error {
	if name != "acme-corp" {
		return fmt.Errorf("unknown fixture: %s", name)
	}

	loader := testfixtures.NewLoader(env.DB, env.FixtureDir)
	return loader.LoadAcmeCorp(context.Background())
}

// FixtureLoader returns a fixture loader for the test environment.
func (env *E2EEnv) FixtureLoader() *testfixtures.Loader {
	return testfixtures.NewLoader(env.DB, env.FixtureDir)
}

// TruncateAllTables truncates all user tables.
func (env *E2EEnv) TruncateAllTables() error {
	ctx := context.Background()

	rows, err := env.DB.Query(ctx, `
		SELECT tablename
		FROM pg_tables
		WHERE schemaname = 'public'
		AND tablename NOT LIKE 'pg_%'
		AND tablename NOT LIKE 'sql_%'
		AND tablename != 'schema_migrations'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return err
		}
		tables = append(tables, tableName)
	}

	for _, table := range tables {
		_, err := env.DB.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			// Log but continue - some tables might have constraints
			env.t.Logf("warning: could not truncate %s: %v", table, err)
		}
	}

	return nil
}

// LoadYAMLFile loads and parses a YAML file.
func LoadYAMLFile[T any](path string) (T, error) {
	var result T

	data, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal YAML %s: %w", path, err)
	}

	return result, nil
}

// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// FixturePath returns the full path to a fixture file.
func (env *E2EEnv) FixturePath(relativePath string) string {
	return filepath.Join(env.FixtureDir, relativePath)
}
