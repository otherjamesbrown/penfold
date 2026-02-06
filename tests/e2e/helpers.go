//go:build e2e

// Package e2e provides helpers for end-to-end tests.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	GatewayURL string
	FixtureDir string
	CLI        *CLIRunner
	t          *testing.T
}

// SetupE2EEnvironment creates the E2E test environment.
// It requires:
//   - Database connection with SSL certs (same as integration tests)
//   - Gateway accessible with LLM service healthy
func SetupE2EEnvironment(t *testing.T) *E2EEnv {
	t.Helper()

	// Setup database connection
	host := getEnvOrDefault("PENFOLD_DB_HOST", "dev02.brown.chat")
	port := getEnvOrDefault("PENFOLD_DB_PORT", "5432")
	user := getEnvOrDefault("PENFOLD_DB_USER", "penfold")
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold") // Use production DB with tenant isolation
	gatewayURL := getEnvOrDefault("GATEWAY_URL", "http://dev02.brown.chat:8080")

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
	cli.SetEnv("GATEWAY_URL", gatewayURL)

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
		GatewayURL: gatewayURL,
		FixtureDir: fixtureDir,
		CLI:        cli,
		t:          t,
	}

	// Check gateway and LLM availability
	if !env.GatewayLLMAvailable() {
		pool.Close()
		t.Skip("Gateway LLM service not available - skipping E2E test")
	}

	// Register cleanup
	t.Cleanup(func() {
		pool.Close()
	})

	return env
}

// gatewayHealthResponse represents the gateway health endpoint response.
type gatewayHealthResponse struct {
	Status   string `json:"status"`
	Services []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"services"`
}

// GatewayLLMAvailable checks if the gateway is available and the LLM service is healthy.
// This allows E2E tests to run from any machine that can reach the gateway.
func (env *E2EEnv) GatewayLLMAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.GatewayURL+"/health", nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var health gatewayHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return false
	}

	// Check that the LLM service specifically is healthy.
	// Overall status may be "degraded" due to non-critical services (e.g. worker
	// health check failing) while the pipeline can still process content.
	for _, svc := range health.Services {
		if svc.Name == "llm" {
			return svc.Status == "healthy"
		}
	}

	// LLM service not registered in gateway (might not be configured)
	return false
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
// Tables are deleted in FK-safe order (children before parents) to avoid constraint violations.
func (env *E2EEnv) CleanupTestTenant() error {
	ctx := context.Background()

	// FK-safe deletion order: leaf tables first, parent tables last.
	// This order is derived from the FK relationships in the schema.
	// Tables without FK dependencies on other tenant tables can be deleted early.
	cleanupOrder := []string{
		// Deepest leaf tables (no children)
		"enrichment_stages",           // -> content_enrichment
		"pipeline_runs",               // -> pipeline_stages
		"extraction_feedback",         // -> extraction_runs
		"extraction_experiment_results", // -> extraction_runs, extraction_experiments
		"ingest_errors",               // -> ingest_jobs
		"jira_ticket_changes",         // -> jira_tickets
		"meeting_events",              // -> meetings
		"meeting_attendees",           // -> meetings
		"meeting_mentions",            // -> meetings, sources
		"meeting_participants",        // -> meetings
		"thread_messages",             // -> email_threads
		"link_sources",                // -> extracted_links
		"link_enrichment",             // -> extracted_links
		"person_aliases",              // -> people
		"team_members",                // -> teams, people
		"assertion_references",        // -> assertions
		"watch_list",                  // -> assertions, projects
		"resolution_decisions",        // -> resolution_trace_stages
		"resolution_llm_calls",        // -> resolution_trace_stages
		"resolution_trace_stages",     // -> resolution_traces
		"resolution_comparison_decisions", // -> resolution_comparisons
		"relationship_conflicts",      // -> relationships
		"relationship_evidence",       // -> relationships, sources
		"relationship_feedback",       // -> relationships
		"relationship_versions",       // -> relationships
		"product_aliases",             // -> products
		"product_team_roles",          // -> product_teams, people
		"product_event_links",         // -> product_events
		"product_events",              // -> products
		"product_teams",               // -> products, teams
		"automation_decisions",        // -> automation_rules
		"automation_patterns",         // -> automation_rules
		"rule_effectiveness",          // -> automation_rules
		"rule_conflicts",              // -> automation_rules
		"automation_rule_versions",    // -> automation_rules
		"project_members",             // -> projects, people, teams
		"cross_tenant_person_links",   // -> people
		"user_feedback",               // -> review_items, review_sessions, learning_rules
		"review_items",                // -> review_sessions, sources
		"batch_operations",            // -> review_sessions
		"search_query_records",        // -> search_sessions
		"processing_jobs",             // -> processing_events

		// Mid-level tables (have children above, parents below)
		"embeddings",                  // -> sources, people, projects, teams, assertions
		"content_mentions",            // (no tenant_id, skipped)
		"content_sentiment",           // -> extraction_runs
		"email_attachments",           // -> sources
		"archived_files",              // -> sources
		"extraction_runs",             // -> email_threads, extraction_experiments
		"extraction_experiments",      // (tenant_id)
		"resolution_comparisons",      // (tenant_id)
		"resolution_traces",           // (tenant_id)
		"relationships",               // (tenant_id)
		"content_enrichment",          // (tenant_id, source_id not FK)
		"jira_tickets",                // (tenant_id) -> people, projects
		"extracted_links",             // (tenant_id)
		"email_threads",               // (tenant_id) -> projects
		"ingest_jobs",                 // (tenant_id)
		"review_sessions",             // (tenant_id)
		"search_sessions",             // (tenant_id)
		"processing_events",           // (tenant_id)
		"automation_rules",            // (tenant_id, has self-FK)
		"products",                    // (tenant_id, has self-FK parent_id)

		// Parent tables (many children depend on these)
		"assertions",                  // (tenant_id, has self-FK, FK -> sources)
		"sources",                     // -> meetings (but sources.meeting_id -> meetings)
		"meetings",                    // (tenant_id) -> meeting_series
		"meeting_series",              // (tenant_id) -> projects
		"projects",                    // (tenant_id, has self-FK)
		"people",                      // (tenant_id)
		"teams",                       // (tenant_id, has self-FK)

		// Tenant configuration and system tables
		"learning_rules",              // (tenant_id)
		"review_analytics",            // (tenant_id)
		"search_analytics",            // (tenant_id)
		"query_suggestions",           // (tenant_id)
		"tenant_sessions",             // (tenant_id)
		"progressive_settings",        // (tenant_id if exists)
		"confidence_thresholds",       // (tenant_id if exists)
		"glossary",                    // (tenant_id)
		"insight_type_registry",       // (tenant_id if exists)
		"mention_patterns",            // (tenant_id if exists)

		// Note: 'tenants' table is never deleted - we preserve the tenant record itself
	}

	// Track deleted tables to detect any that were missed
	deletedTables := make(map[string]bool)

	// Delete from each table in order
	for _, table := range cleanupOrder {
		_, err := env.DB.Exec(ctx, fmt.Sprintf(
			"DELETE FROM %s WHERE tenant_id = $1", table), env.TenantID)
		if err != nil {
			// Handle "relation does not exist" gracefully - some tables may not exist in all environments
			if strings.Contains(err.Error(), "does not exist") {
				env.t.Logf("table %s does not exist, skipping", table)
				continue
			}
			// Fail loudly on FK violations or other errors
			return fmt.Errorf("failed to delete from %s: %w", table, err)
		}
		deletedTables[table] = true
	}

	// Safety net: check for any tables with tenant_id that we missed
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
		return fmt.Errorf("querying tenant tables for safety check: %w", err)
	}
	defer rows.Close()

	var missedTables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return err
		}
		if !deletedTables[tableName] {
			missedTables = append(missedTables, tableName)
		}
	}

	// Warn if we found tables not in our explicit list
	if len(missedTables) > 0 {
		env.t.Logf("WARNING: Found tables with tenant_id not in cleanup list: %v", missedTables)
		env.t.Logf("These tables were NOT cleaned up and may cause test isolation issues")
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
