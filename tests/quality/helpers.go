//go:build quality

package quality

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/db"
	"github.com/otherjamesbrown/penfold/pkg/testfixtures"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestTenantID is the tenant ID for quality tests.
// Distinct from E2E (001) and integration (002).
const TestTenantID = "00000000-0000-0000-0000-000000000003"

// QualityEnv holds the test environment for quality tests.
type QualityEnv struct {
	DB         *pgxpool.Pool
	TenantID   string
	GatewayURL string
	FixtureDir string
	CLI        *CLIRunner
	t          *testing.T
}

// SetupQualityEnvironment creates the quality test environment.
func SetupQualityEnvironment(t *testing.T) *QualityEnv {
	t.Helper()

	host := getEnvOrDefault("PENFOLD_DB_HOST", "dev02.brown.chat")
	port := getEnvOrDefault("PENFOLD_DB_PORT", "5432")
	user := getEnvOrDefault("PENFOLD_DB_USER", "penfold")
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold_test")
	gatewayURL := getEnvOrDefault("GATEWAY_URL", "http://dev02.brown.chat:8080")

	// Build connection string with SSL cert auth
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}
	sslCert := filepath.Join(homeDir, ".postgresql", "postgresql.crt")
	sslKey := filepath.Join(homeDir, ".postgresql", "postgresql.key")
	sslRootCert := filepath.Join(homeDir, ".postgresql", "root.crt")

	if _, err := os.Stat(sslCert); os.IsNotExist(err) {
		t.Skip("SSL certs not found in ~/.postgresql/ — skipping quality test")
	}

	connStr := fmt.Sprintf(
		"postgres://%s@%s:%s/%s?sslmode=verify-full&sslcert=%s&sslkey=%s&sslrootcert=%s",
		user, host, port, dbName, sslCert, sslKey, sslRootCert,
	)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to connect to database")

	err = pool.Ping(ctx)
	require.NoError(t, err, "failed to ping database")

	// Run migrations to ensure schema is up to date.
	// Find migrations directory relative to the test file.
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

	if migrationsDir == "" {
		t.Skip("migrations directory not found - skipping quality test")
	}

	_, err = db.RunMigrations(ctx, pool, migrationsDir)
	require.NoError(t, err, "failed to run migrations")

	cli := NewCLIRunner(t)

	cli.SetEnv("DB_HOST", host)
	cli.SetEnv("DB_PORT", port)
	cli.SetEnv("DB_USER", user)
	cli.SetEnv("DB_NAME", dbName)
	cli.SetEnv("DB_SSLMODE", "verify-full")
	cli.SetEnv("DB_SSLCERT", sslCert)
	cli.SetEnv("DB_SSLKEY", sslKey)
	cli.SetEnv("DB_SSLROOTCERT", sslRootCert)
	cli.SetEnv("GATEWAY_URL", gatewayURL)

	redisHost := getEnvOrDefault("PENFOLD_REDIS_HOST", "localhost")
	redisPort := getEnvOrDefault("PENFOLD_REDIS_PORT", "6379")
	cli.SetEnv("REDIS_HOST", redisHost)
	cli.SetEnv("REDIS_PORT", redisPort)

	cli.SetEnv("PENF_TENANT_ID", TestTenantID)

	fixtureDir := filepath.Join(cli.WorkDir, "tests", "fixtures", "acme-corp")

	env := &QualityEnv{
		DB:         pool,
		TenantID:   TestTenantID,
		GatewayURL: gatewayURL,
		FixtureDir: fixtureDir,
		CLI:        cli,
		t:          t,
	}

	if !env.GatewayLLMAvailable() {
		pool.Close()
		t.Skip("Gateway LLM service not available — skipping quality test")
	}

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

// GatewayLLMAvailable checks if the gateway and LLM service are healthy.
func (env *QualityEnv) GatewayLLMAvailable() bool {
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

	for _, svc := range health.Services {
		if svc.Name == "llm" {
			return svc.Status == "healthy"
		}
	}

	return false
}

// EnsureTenantExists creates the quality test tenant if it doesn't exist.
func (env *QualityEnv) EnsureTenantExists() error {
	ctx := context.Background()
	_, err := env.DB.Exec(ctx, `
		INSERT INTO tenants (id, name, display_name, slug, owner_email, created_at, updated_at)
		VALUES ($1, 'quality_test', 'Quality Test', 'quality-test', 'quality@test.local', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, env.TenantID)
	return err
}

// LoadFixtures loads the Acme Corp fixture data for the quality tenant.
func (env *QualityEnv) LoadFixtures() error {
	if err := env.EnsureTenantExists(); err != nil {
		return fmt.Errorf("ensure tenant exists: %w", err)
	}

	loader := testfixtures.NewLoaderWithTenant(env.DB, env.FixtureDir, env.TenantID)
	return loader.LoadAcmeCorp(context.Background())
}

// CleanupTestTenant deletes all data for the quality test tenant.
// Tables are deleted in FK-safe order.
func (env *QualityEnv) CleanupTestTenant() error {
	ctx := context.Background()

	cleanupOrder := []string{
		"enrichment_stages",
		"pipeline_runs",
		"extraction_feedback",
		"extraction_experiment_results",
		"ingest_errors",
		"jira_ticket_changes",
		"meeting_events",
		"meeting_attendees",
		"meeting_mentions",
		"meeting_participants",
		"thread_messages",
		"link_sources",
		"link_enrichment",
		"person_aliases",
		"team_members",
		"assertion_references",
		"watch_list",
		"resolution_decisions",
		"resolution_llm_calls",
		"resolution_trace_stages",
		"resolution_traces",
		"resolution_comparison_decisions",
		"resolution_comparisons",
		"relationship_conflicts",
		"relationship_evidence",
		"relationship_feedback",
		"relationship_versions",
		"product_aliases",
		"product_team_roles",
		"product_event_links",
		"product_events",
		"product_teams",
		"automation_decisions",
		"automation_patterns",
		"rule_effectiveness",
		"rule_conflicts",
		"automation_rule_versions",
		"project_members",
		"cross_tenant_person_links",
		"user_feedback",
		"review_items",
		"batch_operations",
		"search_query_records",
		"processing_jobs",
		"embeddings",
		"content_sentiment",
		"email_attachments",
		"archived_files",
		"extraction_runs",
		"extraction_experiments",
		"relationships",
		"content_enrichment",
		"jira_tickets",
		"extracted_links",
		"email_threads",
		"ingest_jobs",
		"review_sessions",
		"search_sessions",
		"processing_events",
		"automation_rules",
		"products",
		"assertions",
		"sources",
		"meetings",
		"meeting_series",
		"projects",
		"people",
		"teams",
		"learning_rules",
		"review_analytics",
		"search_analytics",
		"query_suggestions",
		"tenant_sessions",
		"progressive_settings",
		"confidence_thresholds",
		"glossary",
		"insight_type_registry",
		"mention_patterns",
	}

	for _, table := range cleanupOrder {
		_, err := env.DB.Exec(ctx, fmt.Sprintf(
			"DELETE FROM %s WHERE tenant_id = $1", table), env.TenantID)
		if err != nil {
			if strings.Contains(err.Error(), "does not exist") {
				continue
			}
			return fmt.Errorf("failed to delete from %s: %w", table, err)
		}
	}

	return nil
}

// FixturePath returns the full path to a fixture file.
func (env *QualityEnv) FixturePath(relativePath string) string {
	return filepath.Join(env.FixtureDir, relativePath)
}

// --- Pipeline query helpers ---

// waitForProcessingComplete polls sources.processing_status until completed/failed.
func waitForProcessingComplete(t *testing.T, env *QualityEnv, sourceID int64, timeout time.Duration) error {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		var status string
		err := env.DB.QueryRow(ctx, `
			SELECT processing_status FROM sources WHERE id = $1
		`, sourceID).Scan(&status)

		if err == nil {
			if status == "completed" {
				return nil
			}
			if status == "failed" {
				return fmt.Errorf("processing failed for source %d", sourceID)
			}
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for processing after %v (source %d)", timeout, sourceID)
}

// getLatestSourceByTag gets the most recent source with the given source_tag.
func getLatestSourceByTag(env *QualityEnv, sourceTag string) (int64, error) {
	ctx := context.Background()
	var sourceID int64
	err := env.DB.QueryRow(ctx, `
		SELECT s.id FROM sources s
		JOIN ingest_jobs j ON s.tenant_id = j.tenant_id
		WHERE j.source_tag = $1 AND s.tenant_id = $2
		ORDER BY s.created_at DESC
		LIMIT 1
	`, sourceTag, env.TenantID).Scan(&sourceID)
	return sourceID, err
}

// getAssertionsForSource queries assertions for a source.
func getAssertionsForSource(env *QualityEnv, sourceID int64) ([]ActualAssertion, error) {
	ctx := context.Background()
	rows, err := env.DB.Query(ctx, `
		SELECT id, assertion_type, description, confidence, is_current
		FROM assertions
		WHERE source_id = $1
		ORDER BY created_at
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assertions []ActualAssertion
	for rows.Next() {
		var a ActualAssertion
		err := rows.Scan(&a.ID, &a.AssertionType, &a.Description, &a.Confidence, &a.IsCurrent)
		if err != nil {
			return nil, err
		}
		assertions = append(assertions, a)
	}
	return assertions, rows.Err()
}

// getAutoCreatedPeopleForSource queries auto-created people associated with a source
// via content_mentions or person_aliases discovered during processing.
func getAutoCreatedPeopleForSource(env *QualityEnv, sourceID int64) ([]ActualPerson, error) {
	ctx := context.Background()

	// Get all people for the tenant (both fixture and auto-created).
	// The pipeline may resolve mentions to existing fixture people or create new ones.
	rows, err := env.DB.Query(ctx, `
		SELECT DISTINCT p.id, p.canonical_name,
			COALESCE(p.job_title, '') as job_title,
			p.auto_created
		FROM people p
		WHERE p.tenant_id = $1
		ORDER BY p.canonical_name
	`, env.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var people []ActualPerson
	for rows.Next() {
		var p ActualPerson
		err := rows.Scan(&p.ID, &p.CanonicalName, &p.Title, &p.AutoCreated)
		if err != nil {
			return nil, err
		}
		people = append(people, p)
	}
	return people, rows.Err()
}

// getPipelineRunParsedData gets the parsed_data JSONB for a specific pipeline stage.
func getPipelineRunParsedData(env *QualityEnv, sourceID int64, stage string) (json.RawMessage, error) {
	ctx := context.Background()
	var data json.RawMessage
	err := env.DB.QueryRow(ctx, `
		SELECT parsed_data
		FROM pipeline_runs
		WHERE source_id = $1 AND stage = $2 AND status = 'completed'
		ORDER BY created_at DESC
		LIMIT 1
	`, sourceID, stage).Scan(&data)
	return data, err
}

// getTriageResult extracts triage importance/category from pipeline_runs.parsed_data.
func getTriageResult(env *QualityEnv, sourceID int64) (*ActualTriageResult, error) {
	data, err := getPipelineRunParsedData(env, sourceID, "triage")
	if err != nil {
		return nil, fmt.Errorf("query triage parsed_data: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("triage parsed_data is null for source %d", sourceID)
	}

	var result ActualTriageResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal triage parsed_data: %w", err)
	}
	return &result, nil
}

// assertStageCompleted checks that a pipeline stage completed for a source.
func assertStageCompleted(t *testing.T, env *QualityEnv, sourceID int64, stageName string) {
	t.Helper()
	ctx := context.Background()

	var status string
	var durationMs *int

	err := env.DB.QueryRow(ctx, `
		SELECT status, duration_ms
		FROM pipeline_runs
		WHERE source_id = $1 AND stage = $2
	`, sourceID, stageName).Scan(&status, &durationMs)

	require.NoError(t, err, "stage %s should have pipeline_runs entry", stageName)
	require.Equal(t, "completed", status, "stage %s should be completed", stageName)

	if durationMs != nil {
		t.Logf("  stage %s completed in %dms", stageName, *durationMs)
	}
}

// getProjectsForSource queries projects linked to assertions for a source.
func getProjectsForSource(env *QualityEnv, sourceID int64) ([]ActualProject, error) {
	ctx := context.Background()
	rows, err := env.DB.Query(ctx, `
		SELECT DISTINCT p.id, p.name
		FROM projects p
		JOIN assertions a ON a.project_id = p.id
		WHERE a.source_id = $1
		ORDER BY p.name
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []ActualProject
	for rows.Next() {
		var p ActualProject
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// LoadGoldenFile loads and parses a golden YAML file.
func LoadGoldenFile(path string) (*GoldenExpectation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read golden file %s: %w", path, err)
	}

	var golden GoldenExpectation
	if err := yaml.Unmarshal(data, &golden); err != nil {
		return nil, fmt.Errorf("unmarshal golden file %s: %w", path, err)
	}

	return &golden, nil
}

// --- CLI Runner (duplicated from e2e, different build tag) ---

var (
	builtBinaryPath string
	buildOnce       sync.Once
	buildErr        error
)

// CLIRunner executes penf CLI commands and captures output.
type CLIRunner struct {
	BinaryPath string
	WorkDir    string
	Env        []string
}

// NewCLIRunner creates a CLI runner, building the binary if needed.
func NewCLIRunner(t *testing.T) *CLIRunner {
	t.Helper()

	workDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Walk up to find cmd/penf
	for workDir != "/" {
		cmdPath := filepath.Join(workDir, "cmd", "penf")
		if _, err := os.Stat(cmdPath); err == nil {
			break
		}
		workDir = filepath.Dir(workDir)
	}

	if workDir == "/" {
		t.Fatalf("could not find project root (looking for cmd/penf)")
	}

	buildOnce.Do(func() {
		binaryPath := filepath.Join(workDir, "penf")
		cmdPath := filepath.Join(workDir, "cmd", "penf")

		if _, err := os.Stat(cmdPath); err == nil {
			t.Log("Building penf CLI...")
			cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/penf")
			cmd.Dir = workDir
			if output, err := cmd.CombinedOutput(); err != nil {
				buildErr = fmt.Errorf("failed to build penf: %v\n%s", err, output)
				return
			}
			builtBinaryPath = binaryPath
		} else {
			buildErr = fmt.Errorf("cmd/penf directory not found")
		}
	})

	if buildErr != nil {
		t.Fatalf("failed to build penf: %v", buildErr)
	}

	return &CLIRunner{
		BinaryPath: builtBinaryPath,
		WorkDir:    workDir,
		Env:        os.Environ(),
	}
}

// RunResult holds the result of a CLI command execution.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Err      error
}

// Run executes a penf command and returns the result.
func (r *CLIRunner) Run(ctx context.Context, args ...string) *RunResult {
	start := time.Now()

	cmd := exec.CommandContext(ctx, r.BinaryPath, args...)
	cmd.Dir = r.WorkDir
	cmd.Env = r.Env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	return &RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: duration,
		Err:      err,
	}
}

// SetEnv adds or updates an environment variable for CLI commands.
func (r *CLIRunner) SetEnv(key, value string) {
	prefix := key + "="
	for i, env := range r.Env {
		if len(env) >= len(prefix) && env[:len(prefix)] == prefix {
			r.Env[i] = prefix + value
			return
		}
	}
	r.Env = append(r.Env, prefix+value)
}

// --- Utilities ---

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
