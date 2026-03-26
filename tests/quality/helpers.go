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
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold")
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
		if svc.Name == "llm" || svc.Name == "ai_service" {
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

// SeedClassificationRules seeds classification rules, pipeline routing,
// and pipeline definitions for the quality test tenant. Safe to call multiple times.
func (env *QualityEnv) SeedClassificationRules(ctx context.Context) error {
	type classRule struct {
		name               string
		priority           int
		subtype            string
		notificationSource *string // nil for newsletters
		field              string
		matchType          string
		value              string
	}

	// Helper for notification_source pointers.
	ns := func(s string) *string { return &s }

	rules := []classRule{
		// Newsletter rules (from migrations 092, 135, 146, 148)
		{"newsletter_ctg", 8, "NEWSLETTER", nil, "from_address", "exact", "ctgcomms@akamai.com"},
		{"newsletter_akamai_wave", 8, "NEWSLETTER", nil, "from_address", "exact", "AkamaiWave@akamai.com"},
		{"newsletter_emea", 8, "NEWSLETTER", nil, "from_address", "exact", "EMEA_newsletter@akamai.com"},
		{"newsletter_englearn", 8, "NEWSLETTER", nil, "from_address", "exact", "EngLearn@akamai.com"},
		{"newsletter_akamai_spark", 8, "NEWSLETTER", nil, "from_address", "exact", "AkamaiSpark@akamai.com"},
		{"newsletter_akamai_wellness", 8, "NEWSLETTER", nil, "from_address", "exact", "AkamaiWellness@akamai.com"},
		{"newsletter_comms_pattern", 85, "NEWSLETTER", nil, "from_address", "glob", "*comms@*"},
		{"newsletter_wave_pattern", 85, "NEWSLETTER", nil, "from_address", "glob", "*wave@*"},
		{"newsletter_addr_pattern", 85, "NEWSLETTER", nil, "from_address", "glob", "*newsletter@*"},
		{"newsletter_internal_corporate", 80, "NEWSLETTER_INTERNAL", nil, "subject", "contains", "Post-Its"},
		{"newsletter_dynamic_signal", 7, "NEWSLETTER_DIGEST", nil, "from_address", "contains", "dynamicsignal"},

		// Notification rules — matching test fixture .eml senders
		{"notif_aha_mailer", 5, "NOTIFICATION", ns("aha"), "from_address", "glob", "*@*.mailer.aha.io"},
		{"notif_aha_support", 5, "NOTIFICATION", ns("aha"), "from_address", "exact", "support@aha.io"},
		{"notif_jira", 5, "NOTIFICATION", ns("jira"), "from_address", "contains", "jira"},
		{"notif_oracle", 5, "NOTIFICATION", ns("oracle"), "from_address", "contains", "oracle-hcm"},
		{"notif_google_security", 5, "NOTIFICATION", ns("google"), "from_address", "exact", "no-reply@accounts.google.com"},
		{"notif_globalsecops", 5, "NOTIFICATION", ns("globalsecops"), "from_address", "exact", "globalsecops@akamai.com"},
		{"notif_bitmovin", 5, "NOTIFICATION", ns("bitmovin"), "from_address", "contains", "bitmovin"},
		{"notif_internal_action", 90, "NOTIFICATION", ns("internal"), "subject", "contains", "ACTION REQUESTED"},
	}

	for _, r := range rules {
		// Insert rule if not exists, then get its ID.
		if _, err := env.DB.Exec(ctx, `
			INSERT INTO classification_rules (tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source, active)
			SELECT $1, $2::text, $3, 'EMAIL', 'EMAIL', $4::text, $5, true
			WHERE NOT EXISTS (
				SELECT 1 FROM classification_rules WHERE tenant_id = $1 AND name = $2::text
			)
		`, env.TenantID, r.name, r.priority, r.subtype, r.notificationSource); err != nil {
			return fmt.Errorf("insert classification rule %s: %w", r.name, err)
		}

		var ruleID int
		err := env.DB.QueryRow(ctx, `
			SELECT id FROM classification_rules WHERE tenant_id = $1 AND name = $2
		`, env.TenantID, r.name).Scan(&ruleID)
		if err != nil {
			return fmt.Errorf("get classification rule %s id: %w", r.name, err)
		}

		// Insert match condition only if the rule has none yet.
		var condCount int
		if err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM classification_match_conditions WHERE rule_id = $1
		`, ruleID).Scan(&condCount); err != nil {
			return fmt.Errorf("count conditions for rule %s: %w", r.name, err)
		}
		if condCount == 0 {
			if _, err := env.DB.Exec(ctx, `
				INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
				VALUES ($1, $2, $3, $4, false)
			`, ruleID, r.field, r.matchType, r.value); err != nil {
				return fmt.Errorf("insert match condition for rule %s: %w", r.name, err)
			}
		}
	}

	// Seed pipeline routing.
	type routing struct {
		subtype  string
		pipeline string
	}
	for _, rt := range []routing{
		{"NEWSLETTER", "newsletter"},
		{"NEWSLETTER_INTERNAL", "newsletter_internal"},
		{"NEWSLETTER_DIGEST", "newsletter_digest"},
		{"NOTIFICATION", "notification"},
	} {
		if _, err := env.DB.Exec(ctx, `
			INSERT INTO pipeline_routing (tenant_id, content_type, content_subtype, pipeline, active)
			SELECT $1, 'EMAIL', $2::text, $3::text, true
			WHERE NOT EXISTS (
				SELECT 1 FROM pipeline_routing
				WHERE tenant_id = $1 AND content_type = 'EMAIL' AND content_subtype = $2::text
			)
		`, env.TenantID, rt.subtype, rt.pipeline); err != nil {
			return fmt.Errorf("insert pipeline routing for %s: %w", rt.pipeline, err)
		}
	}

	// Detect whether pipeline_definitions has the stage_kind column (added in migration 144).
	var hasStageKind bool
	if err := env.DB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'pipeline_definitions' AND column_name = 'stage_kind'
		)
	`).Scan(&hasStageKind); err != nil {
		return fmt.Errorf("check stage_kind column: %w", err)
	}

	type pipelineDef struct {
		pipeline       string
		stage          string
		stageOrder     int
		timeoutSec     int
		promptOverride *int
		stageKind      string
		persistKey     *string
	}

	newsletterKey := "newsletter"
	promptV2 := 2
	promptV3 := 3
	promptV4 := 4

	defs := []pipelineDef{
		// Base newsletter pipeline
		{"newsletter", "parse", 0, 60, nil, "code_only", nil},
		{"newsletter", "triage", 10, 120, nil, "llm", nil},
		{"newsletter", "newsletter_extract", 20, 120, nil, "structured_extract", &newsletterKey},
		{"newsletter", "embed", 30, 60, nil, "embedding", nil},
		// newsletter_internal pipeline (prompt_override = 3 for v3 prompt)
		{"newsletter_internal", "parse", 0, 60, nil, "code_only", nil},
		{"newsletter_internal", "triage", 10, 120, nil, "llm", nil},
		{"newsletter_internal", "newsletter_extract", 20, 120, &promptV3, "structured_extract", &newsletterKey},
		{"newsletter_internal", "embed", 30, 60, nil, "embedding", nil},
		// newsletter_digest pipeline (prompt_override = 3 for triage, 4 for extract)
		{"newsletter_digest", "parse", 0, 60, nil, "code_only", nil},
		{"newsletter_digest", "triage", 10, 120, &promptV3, "llm", nil},
		{"newsletter_digest", "newsletter_extract", 20, 120, &promptV4, "structured_extract", &newsletterKey},
		{"newsletter_digest", "embed", 30, 60, nil, "embedding", nil},
		// Notification pipeline (matches prod: parse, triage, summarize, extract_ner, extract_semantic, persist, embed)
		{"notification", "parse", 0, 30, nil, "code_only", nil},
		{"notification", "triage", 10, 120, &promptV2, "llm", nil},
		{"notification", "summarize", 15, 60, &promptV2, "llm", nil},
		{"notification", "extract_ner", 20, 120, nil, "llm", nil},
		{"notification", "extract_semantic", 25, 120, &promptV2, "llm", nil},
		{"notification", "persist", 50, 60, nil, "code_only", nil},
		{"notification", "embed", 60, 60, nil, "embedding", nil},
	}

	for _, d := range defs {
		var err error
		if hasStageKind {
			_, err = env.DB.Exec(ctx, `
				INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, model_override, prompt_override, stage_kind, persist_key)
				VALUES ($1, $2, $3, $4, true, false, false, $5, NULL, $6, $7, $8)
				ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING
			`, env.TenantID, d.pipeline, d.stage, d.stageOrder, d.timeoutSec, d.promptOverride, d.stageKind, d.persistKey)
		} else {
			_, err = env.DB.Exec(ctx, `
				INSERT INTO pipeline_definitions (tenant_id, pipeline, stage, stage_order, enabled, skip_when_low, optional, timeout_seconds, model_override, prompt_override)
				VALUES ($1, $2, $3, $4, true, false, false, $5, NULL, $6)
				ON CONFLICT (tenant_id, pipeline, stage) DO NOTHING
			`, env.TenantID, d.pipeline, d.stage, d.stageOrder, d.timeoutSec, d.promptOverride)
		}
		if err != nil {
			return fmt.Errorf("insert pipeline definition %s/%s: %w", d.pipeline, d.stage, err)
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

// --- CLI Runner (uses installed penf binary from PATH) ---

// CLIRunner executes penf CLI commands and captures output.
type CLIRunner struct {
	BinaryPath string
	WorkDir    string
	Env        []string
}

// NewCLIRunner creates a CLI runner using the installed penf binary.
// The penf CLI lives in a separate repo (penf-cli) and is installed to PATH.
// Use PENF_BINARY env var to override the binary path for testing.
func NewCLIRunner(t *testing.T) *CLIRunner {
	t.Helper()

	binaryPath := os.Getenv("PENF_BINARY")
	if binaryPath == "" {
		var err error
		binaryPath, err = exec.LookPath("penf")
		if err != nil {
			t.Fatalf("penf binary not found in PATH (install from penf-cli repo or set PENF_BINARY): %v", err)
		}
	}

	// Find penfold repo root for fixtures and golden files
	workDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for workDir != "/" {
		if _, err := os.Stat(filepath.Join(workDir, ".git")); err == nil {
			break
		}
		workDir = filepath.Dir(workDir)
	}
	if workDir == "/" {
		t.Fatalf("could not find penfold repo root (looking for .git)")
	}

	return &CLIRunner{
		BinaryPath: binaryPath,
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
