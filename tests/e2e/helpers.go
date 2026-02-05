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

// TestTenantID is the default tenant ID for E2E tests.
// Must match testfixtures.DefaultTestTenantID.
const TestTenantID = "00000000-0000-0000-0000-000000000001"

// E2EEnv holds the test environment for E2E tests.
type E2EEnv struct {
	DB         *pgxpool.Pool
	DBName     string
	TenantID   string
	LLMURL     string
	FixtureDir string
	CLI        *CLIRunner
	t          *testing.T
}

// SetupE2EEnvironment creates the E2E test environment.
// It requires:
//   - Database connection with SSL certs (same as integration tests)
//   - Local LLM server running at LLM_URL
func SetupE2EEnvironment(t *testing.T) *E2EEnv {
	t.Helper()

	// Setup database connection
	host := getEnvOrDefault("PENFOLD_DB_HOST", "dev02.brown.chat")
	port := getEnvOrDefault("PENFOLD_DB_PORT", "5432")
	user := getEnvOrDefault("PENFOLD_DB_USER", "penfold")
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold") // Use production DB with tenant isolation
	llmURL := getEnvOrDefault("LLM_URL", "http://dev01.brown.chat:8080")

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
		t.Skip("SSL certs not found in ~/.postgresql/ - skipping E2E test")
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

	// Initialize CLI runner
	cli := NewCLIRunner(t)

	// Set environment variables for CLI commands
	// CLI uses DB_* env vars (see cmd/penf/cmd/ingest_email.go)
	cli.SetEnv("DB_HOST", host)
	cli.SetEnv("DB_PORT", port)
	cli.SetEnv("DB_USER", user)
	cli.SetEnv("DB_NAME", dbName)
	cli.SetEnv("DB_SSLMODE", "verify-full")
	cli.SetEnv("DB_SSLCERT", sslCert)
	cli.SetEnv("DB_SSLKEY", sslKey)
	cli.SetEnv("DB_SSLROOTCERT", sslRootCert)
	cli.SetEnv("LLM_URL", llmURL)

	// Redis configuration (CLI uses REDIS_* env vars)
	redisHost := getEnvOrDefault("PENFOLD_REDIS_HOST", "localhost")
	redisPort := getEnvOrDefault("PENFOLD_REDIS_PORT", "6379")
	cli.SetEnv("REDIS_HOST", redisHost)
	cli.SetEnv("REDIS_PORT", redisPort)

	// Set test tenant ID for CLI commands
	cli.SetEnv("PENF_TENANT_ID", TestTenantID)

	// Use absolute path for fixtures (CLI runs from project root)
	fixtureDir := filepath.Join(cli.WorkDir, "tests", "fixtures", "acme-corp")

	env := &E2EEnv{
		DB:         pool,
		DBName:     dbName,
		TenantID:   TestTenantID,
		LLMURL:     llmURL,
		FixtureDir: fixtureDir,
		CLI:        cli,
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

// EnsureTenantExists creates the test tenant if it doesn't exist.
func (env *E2EEnv) EnsureTenantExists() error {
	ctx := context.Background()
	_, err := env.DB.Exec(ctx, `
		INSERT INTO tenants (id, name, display_name, slug, owner_email, created_at, updated_at)
		VALUES ($1, 'test_tenant', 'Test Tenant', 'test', 'test@example.com', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, env.TenantID)
	return err
}

// LoadFixture loads the Acme Corp fixture data into the database.
func (env *E2EEnv) LoadFixture(name string) error {
	if name != "acme-corp" {
		return fmt.Errorf("unknown fixture: %s", name)
	}

	// Ensure tenant exists before loading fixtures
	if err := env.EnsureTenantExists(); err != nil {
		return fmt.Errorf("ensure tenant exists: %w", err)
	}

	loader := testfixtures.NewLoader(env.DB, env.FixtureDir)
	return loader.LoadAcmeCorp(context.Background())
}

// FixtureLoader returns a fixture loader for the test environment.
func (env *E2EEnv) FixtureLoader() *testfixtures.Loader {
	return testfixtures.NewLoader(env.DB, env.FixtureDir)
}

// CleanupTestTenant deletes all data for the test tenant only.
// This preserves other tenants' data in the shared database.
func (env *E2EEnv) CleanupTestTenant() error {
	ctx := context.Background()

	// Find all tables with a tenant_id column (except tenants itself)
	rows, err := env.DB.Query(ctx, `
		SELECT DISTINCT c.table_name
		FROM information_schema.columns c
		JOIN information_schema.tables t ON c.table_name = t.table_name
		WHERE c.table_schema = 'public'
		AND t.table_schema = 'public'
		AND t.table_type = 'BASE TABLE'
		AND c.column_name = 'tenant_id'
		AND c.table_name != 'tenants'
	`)
	if err != nil {
		return fmt.Errorf("querying tenant tables: %w", err)
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

	// Delete test tenant's data from each table
	for _, table := range tables {
		_, err := env.DB.Exec(ctx, fmt.Sprintf(
			"DELETE FROM %s WHERE tenant_id = $1", table), env.TenantID)
		if err != nil {
			// Log but continue - some tables might have FK constraints
			env.t.Logf("warning: could not clean %s: %v", table, err)
		}
	}

	return nil
}

// TruncateAllTables is deprecated - use CleanupTestTenant instead.
// This method only cleans up the test tenant's data, not all data.
func (env *E2EEnv) TruncateAllTables() error {
	return env.CleanupTestTenant()
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
