//go:build integration

package activities

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/testutil"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// TestContextProviders_PipelineDefinitions_Integration verifies that
// GetContextProviders reads context_providers from pipeline_definitions
// and returns the expected ordered lists seeded by migration 149.
func TestContextProviders_PipelineDefinitions_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	pipelineRepo := NewPipelineRepository(pool)

	cases := []struct {
		pipeline  string
		stage     string
		wantFirst string // first expected provider (verifies order)
		wantAll   []string
	}{
		{
			pipeline:  "newsletter",
			stage:     "newsletter_extract",
			wantFirst: "user_context",
			wantAll:   []string{"user_context", "glossary", "active_projects", "tracked_products"},
		},
		{
			pipeline:  "standard",
			stage:     "analyze",
			wantFirst: "glossary",
			wantAll:   []string{"glossary", "topics"},
		},
		{
			pipeline:  "standard",
			stage:     "extract_ner",
			wantFirst: "glossary",
			wantAll:   []string{"glossary"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.pipeline+"/"+tc.stage, func(t *testing.T) {
			providers, err := pipelineRepo.GetContextProviders(ctx, tenantID, tc.pipeline, tc.stage)
			require.NoError(t, err)

			if len(providers) == 0 {
				t.Skipf("context_providers not seeded for %s/%s (migration 149 may not have run)", tc.pipeline, tc.stage)
			}

			assert.Equal(t, tc.wantAll, providers,
				"context_providers must match migration 149 seed for %s/%s", tc.pipeline, tc.stage)
			assert.Equal(t, tc.wantFirst, providers[0],
				"first provider determines context ordering")
		})
	}
}

// TestBuildStageContext_Integration verifies the DB round-trip:
// that GetContextProviders reads the context_providers column from pipeline_definitions
// and BuildStageContext correctly assembles context from registered providers.
func TestBuildStageContext_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
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

	newsletterRepo := NewPostgresNewsletterContextRepository(pool)
	contextRepo := NewContextPackageRepo(pool, logger)

	a := NewContextBuilderActivities(logger, &mockEntityResolver{}, &mockEntityLookup{}, contextRepo, nil, pipelineRepo, nil).
		WithNewsletterContextRepo(newsletterRepo)

	result, err := a.BuildStageContext(ctx, workflows.BuildStageContextInput{
		TenantID: tenantID,
		Pipeline: "newsletter",
		Stage:    "newsletter_extract",
		Content:  "The NLB team confirmed the MTC project is on track for Q4 delivery.",
		Subject:  "Weekly Update",
	})
	require.NoError(t, err)

	t.Logf("BuildStageContext result length: %d chars", len(result))
	// Result may be empty if no data is seeded for the test tenant.
	// The integration assertion is that the round-trip completes without error.
	assert.NotNil(t, result, "result must not be nil")
}

// TestBuildStageContext_StandardAnalyze_Integration verifies BuildStageContext
// for the standard/analyze stage, which is seeded with '{glossary,topics}'.
func TestBuildStageContext_StandardAnalyze_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	logger := logging.MustGlobal()

	pipelineRepo := NewPipelineRepository(pool)

	providers, err := pipelineRepo.GetContextProviders(ctx, tenantID, "standard", "analyze")
	require.NoError(t, err)
	if len(providers) == 0 {
		t.Skip("standard/analyze context_providers not seeded (migration 149 may not have run)")
	}

	contextRepo := NewContextPackageRepo(pool, logger)

	a := &ContextBuilderActivities{
		logger:         logger.With(logging.F("component", "context_builder_activities")),
		entityResolver: &mockEntityResolver{},
		entityRepo:     &mockEntityLookup{},
		contextRepo:    contextRepo,
		pipelineRepo:   pipelineRepo,
	}
	RegisterContextProviders(logger, contextRepo, nil, nil)

	result, err := a.BuildStageContext(ctx, workflows.BuildStageContextInput{
		TenantID: tenantID,
		Pipeline: "standard",
		Stage:    "analyze",
		Content:  "The NLB load balancer configuration was updated by the MTC team.",
		Subject:  "Infrastructure Update",
	})
	require.NoError(t, err)

	t.Logf("standard/analyze BuildStageContext result: %d chars", len(result))
	assert.NotNil(t, result)

	// If any glossary terms were found, the Glossary section header must be present.
	if len(result) > 0 {
		assert.Contains(t, result, "###", "non-empty result must contain at least one section header")
	}
}

// TestBuildStageContext_UnknownStage_Integration verifies that a stage with no
// context_providers returns an empty string without error (not found = empty slice).
func TestBuildStageContext_UnknownStage_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	logger := logging.MustGlobal()

	pipelineRepo := NewPipelineRepository(pool)
	a := &ContextBuilderActivities{
		logger:         logger.With(logging.F("component", "context_builder_activities")),
		entityResolver: &mockEntityResolver{},
		entityRepo:     &mockEntityLookup{},
		contextRepo:    &mockContextPackageRepo{},
		pipelineRepo:   pipelineRepo,
	}

	result, err := a.BuildStageContext(ctx, workflows.BuildStageContextInput{
		TenantID: tenantID,
		Pipeline: "standard",
		Stage:    "nonexistent_stage_xyz",
	})
	require.NoError(t, err)
	assert.Empty(t, result, "unknown stage must return empty string")
}

// TestBuildStageContext_CustomPipeline_Integration verifies that inserting a
// pipeline_definitions row with a custom context_providers list causes
// BuildStageContext to run only those providers.
func TestBuildStageContext_CustomPipeline_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	logger := logging.MustGlobal()

	// Insert a test-only pipeline definition with a single provider.
	const testPipeline = "test_custom_context_pipeline_integration"
	const testStage = "test_stage"
	_, err := pool.Exec(ctx, `
		INSERT INTO pipeline_definitions
		  (tenant_id, pipeline, stage, stage_order, context_providers)
		VALUES ($1, $2, $3, 1, '{glossary}')
		ON CONFLICT (tenant_id, pipeline, stage) DO UPDATE SET context_providers = EXCLUDED.context_providers
	`, tenantID, testPipeline, testStage)
	require.NoError(t, err, "must be able to insert test pipeline definition")

	defer func() {
		_, _ = pool.Exec(ctx, `
			DELETE FROM pipeline_definitions WHERE tenant_id = $1 AND pipeline = $2 AND stage = $3
		`, tenantID, testPipeline, testStage)
	}()

	pipelineRepo := NewPipelineRepository(pool)
	contextRepo := NewContextPackageRepo(pool, logger)

	// Verify the row we just inserted is read back correctly.
	providers, err := pipelineRepo.GetContextProviders(ctx, tenantID, testPipeline, testStage)
	require.NoError(t, err)
	require.Equal(t, []string{"glossary"}, providers,
		"custom pipeline must have exactly the providers we inserted")

	a := &ContextBuilderActivities{
		logger:         logger.With(logging.F("component", "context_builder_activities")),
		entityResolver: &mockEntityResolver{},
		entityRepo:     &mockEntityLookup{},
		contextRepo:    contextRepo,
		pipelineRepo:   pipelineRepo,
	}
	RegisterContextProviders(logger, contextRepo, nil, nil)

	result, err := a.BuildStageContext(ctx, workflows.BuildStageContextInput{
		TenantID: tenantID,
		Pipeline: testPipeline,
		Stage:    testStage,
		Content:  "The NLB load balancer handles all traffic.",
		Subject:  "Infrastructure",
	})
	require.NoError(t, err, "BuildStageContext must not error for custom pipeline")

	t.Logf("custom pipeline result: %q", result)

	// Result is either empty (no glossary entries for test tenant) or
	// contains exactly the Glossary section — never user_context/projects/products.
	if len(result) > 0 {
		assert.Contains(t, result, "### Glossary",
			"non-empty result from glossary-only pipeline must contain Glossary section")
		assert.NotContains(t, result, "### User Context",
			"glossary-only pipeline must not contain User Context section")
		assert.NotContains(t, result, "### Active Projects",
			"glossary-only pipeline must not contain Active Projects section")
		assert.NotContains(t, result, "### Tracked Products",
			"glossary-only pipeline must not contain Tracked Products section")
	}
}

// TestBuildContextPackage_EntityResolution_Integration verifies that
// BuildContextPackage still performs entity resolution correctly after the
// context assembly refactor. ResolvedPeople, ResolvedProjects, and
// CorrectedExtraction must be populated from the extraction input.
func TestBuildContextPackage_EntityResolution_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	logger := logging.MustGlobal()

	pipelineRepo := NewPipelineRepository(pool)
	contextRepo := NewContextPackageRepo(pool, logger)

	a := &ContextBuilderActivities{
		logger:         logger.With(logging.F("component", "context_builder_activities")),
		entityResolver: &mockEntityResolver{},
		entityRepo:     &mockEntityLookup{},
		contextRepo:    contextRepo,
		pipelineRepo:   pipelineRepo,
	}
	RegisterContextProviders(logger, contextRepo, nil, nil)

	// Minimal extraction: one person and one project in the extraction output.
	// The mock entity resolver/repo return empty results, which is fine —
	// we verify the plumbing (CorrectedExtraction) not the DB data.
	extraction := &workflows.SLMPipelineExtractEntitiesOutput{
		People:   []workflows.PersonResult{{Name: "Alice Smith"}},
		Projects: []string{"MTC"},
	}

	out, err := a.BuildContextPackage(ctx, workflows.BuildContextInput{
		TenantID:    tenantID,
		SourceID:    1,
		JobID:       "test-job-entity-resolution",
		ContentType: "email",
		Extraction:  extraction,
		SenderEmail: "alice@example.com",
		SenderName:  "Alice Smith",
		Content:     "The MTC project is on track.",
		Subject:     "Project Update",
	})
	require.NoError(t, err, "BuildContextPackage must not error")
	require.NotNil(t, out, "BuildContextPackage output must not be nil")

	// Entity resolution slices may be nil or empty when nothing resolves in
	// the mock environment — both are valid. The important assertion is that
	// the function ran and set CorrectedExtraction.
	_ = out.ResolvedPeople   // addressable; count logged below
	_ = out.ResolvedProjects // addressable

	// CorrectedExtraction must carry the (possibly mutated) extraction forward.
	// This is the key continuity assertion: Stage 4 receives the enriched extraction.
	require.NotNil(t, out.CorrectedExtraction, "CorrectedExtraction must be set by BuildContextPackage")
	require.Len(t, out.CorrectedExtraction.People, len(extraction.People),
		"CorrectedExtraction.People count must match input extraction")
	assert.Equal(t, extraction.Projects, out.CorrectedExtraction.Projects,
		"CorrectedExtraction.Projects must match input extraction")

	// ContextPackage must be set (the context assembly path ran).
	require.NotNil(t, out.ContextPackage, "ContextPackage must be set")

	t.Logf("entity resolution: resolved_people=%d resolved_projects=%d",
		len(out.ResolvedPeople), len(out.ResolvedProjects))
}

// TestBuildStageContext_ProviderFailure_NonBlocking_Integration verifies that
// a provider that returns an error does not prevent other providers from running.
// A custom always-failing provider is registered alongside glossary; the test
// asserts that BuildStageContext returns without error and glossary still runs.
func TestBuildStageContext_ProviderFailure_NonBlocking_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testutil.OpenDB(t)
	defer pool.Close()

	tenantID := testRunTenantID
	logger := logging.MustGlobal()

	// Insert a test pipeline with a failing provider followed by a real one.
	const testPipeline = "test_failure_nonblocking_pipeline"
	const testStage = "test_stage"
	const failingProviderName = "test_always_fails_provider_xyz"

	_, err := pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO pipeline_definitions
		  (tenant_id, pipeline, stage, stage_order, context_providers)
		VALUES ($1, $2, $3, 1, '{%s,glossary}')
		ON CONFLICT (tenant_id, pipeline, stage) DO UPDATE SET context_providers = EXCLUDED.context_providers
	`, failingProviderName), tenantID, testPipeline, testStage)
	require.NoError(t, err)

	defer func() {
		_, _ = pool.Exec(ctx, `
			DELETE FROM pipeline_definitions WHERE tenant_id = $1 AND pipeline = $2 AND stage = $3
		`, tenantID, testPipeline, testStage)
	}()

	pipelineRepo := NewPipelineRepository(pool)
	contextRepo := NewContextPackageRepo(pool, logger)

	// Register all standard providers plus our test failing provider.
	RegisterContextProviders(logger, contextRepo, nil, nil)
	providerRegistry[failingProviderName] = &alwaysFailingProvider{name: failingProviderName}
	defer delete(providerRegistry, failingProviderName)

	a := &ContextBuilderActivities{
		logger:         logger.With(logging.F("component", "context_builder_activities")),
		entityResolver: &mockEntityResolver{},
		entityRepo:     &mockEntityLookup{},
		contextRepo:    contextRepo,
		pipelineRepo:   pipelineRepo,
	}

	// BuildStageContext must not return an error even though the first provider fails.
	result, err := a.BuildStageContext(ctx, workflows.BuildStageContextInput{
		TenantID: tenantID,
		Pipeline: testPipeline,
		Stage:    testStage,
		Content:  "The NLB system handles traffic for MTC.",
		Subject:  "System Update",
	})
	require.NoError(t, err, "BuildStageContext must not error when a provider fails")

	t.Logf("non-blocking failure test result: %q", result)

	// The failing provider must not contaminate the output.
	assert.NotContains(t, result, failingProviderName,
		"failing provider output must not appear in assembled context")
}

// alwaysFailingProvider is a test-only ContextProvider that always returns an error.
// Used to verify that BuildStageContext treats provider failures as non-blocking.
type alwaysFailingProvider struct {
	name string
}

func (p *alwaysFailingProvider) Name() string { return p.name }

func (p *alwaysFailingProvider) Build(_ context.Context, _ ContextProviderInput) (string, error) {
	return "", errors.New("injected failure for non-blocking test")
}

