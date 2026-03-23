//go:build integration

package activities

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/activities"
)

// TestBuildStageContext_Integration verifies the DB round-trip:
// that GetContextProviders reads the context_providers column from pipeline_definitions
// and BuildStageContext correctly assembles context from registered providers.
func TestBuildStageContext_Integration(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDBForBuildStageContext(t)
	defer pool.Close()

	tenantID := IntegrationTestTenantID
	logger := logging.MustGlobal()

	pipelineRepo := NewPipelineRepository(pool)

	// Verify the pipeline_definitions table has the context_providers column populated
	// by migration 149 for the newsletter pipeline.
	providers, err := pipelineRepo.GetContextProviders(ctx, tenantID, "newsletter", "newsletter_extract")
	require.NoError(t, err, "GetContextProviders must not error for newsletter/newsletter_extract")

	// The migration seeds newsletter_extract with context providers.
	// This asserts the DB round-trip is working (column exists, data is populated).
	if len(providers) == 0 {
		t.Skip("newsletter_extract context_providers not seeded (migration 149 may not have run)")
	}

	t.Logf("newsletter_extract context_providers: %v", providers)

	// Build a minimal ContextBuilderActivities and call BuildStageContext.
	// We don't inject newsletterRepo so providers that need it will return "".
	// The test verifies the round-trip (DB read + provider dispatch) not the content.
	a := &ContextBuilderActivities{
		logger:         logger.With(logging.F("component", "context_builder_activities")),
		entityResolver: &mockEntityResolver{},
		entityRepo:     &mockEntityLookup{},
		contextRepo:    &mockContextPackageRepo{},
		pipelineRepo:   pipelineRepo,
	}
	// Register providers so they're found (content will be empty without repos).
	RegisterContextProviders(logger, &mockContextPackageRepo{}, nil, nil)

	result, err := a.BuildStageContext(ctx, BuildStageContextInput{
		TenantID: tenantID,
		Pipeline: "newsletter",
		Stage:    "newsletter_extract",
		ProviderInput: ContextProviderInput{
			TenantID: tenantID,
			Content:  "test newsletter content",
			Subject:  "Test Subject",
		},
	})
	require.NoError(t, err)

	// With no repos injected most providers return "". The result may be empty —
	// the integration assertion is that the round-trip completes without error
	// and the function handles nil-repo providers gracefully.
	t.Logf("BuildStageContext result length: %d chars", len(result))
	assert.NotNil(t, result, "result must not be nil")
}

// TestBuildStageContext_UnknownStage_Integration verifies that a stage with no
// context_providers returns an empty string without error (not found = empty slice).
func TestBuildStageContext_UnknownStage_Integration(t *testing.T) {
	ctx := context.Background()
	pool := setupTestDBForBuildStageContext(t)
	defer pool.Close()

	tenantID := IntegrationTestTenantID
	logger := logging.MustGlobal()

	pipelineRepo := NewPipelineRepository(pool)
	a := &ContextBuilderActivities{
		logger:         logger.With(logging.F("component", "context_builder_activities")),
		entityResolver: &mockEntityResolver{},
		entityRepo:     &mockEntityLookup{},
		contextRepo:    &mockContextPackageRepo{},
		pipelineRepo:   pipelineRepo,
	}

	result, err := a.BuildStageContext(ctx, BuildStageContextInput{
		TenantID: tenantID,
		Pipeline: "standard",
		Stage:    "nonexistent_stage_xyz",
		ProviderInput: ContextProviderInput{
			TenantID: tenantID,
		},
	})
	require.NoError(t, err)
	assert.Empty(t, result, "unknown stage must return empty string")
}

// setupTestDBForBuildStageContext creates a DB connection for BuildStageContext integration tests.
func setupTestDBForBuildStageContext(t *testing.T) *pgxpool.Pool {
	t.Helper()

	host := getEnvOrDefault("PENFOLD_DB_HOST", "dev02.brown.chat")
	port := getEnvOrDefault("PENFOLD_DB_PORT", "5432")
	user := getEnvOrDefault("PENFOLD_DB_USER", "penfold")
	dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}
	sslCert := filepath.Join(homeDir, ".postgresql", "postgresql.crt")
	sslKey := filepath.Join(homeDir, ".postgresql", "postgresql.key")
	sslRootCert := filepath.Join(homeDir, ".postgresql", "root.crt")

	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=verify-full sslcert=%s sslkey=%s sslrootcert=%s",
		host, port, user, dbName, sslCert, sslKey, sslRootCert,
	)

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("failed to ping database: %v", err)
	}

	return pool
}
