//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
)

// ============================================================================
// Prompt Override E2E Tests
// ============================================================================
//
// These tests verify that the prompt_override field flows end-to-end from
// pipeline_definitions through the runtime stack to the AI coordinator,
// and that pipeline_runs.prompt_version records the version actually used.
//
// The prompt_override column exists in the pipeline_definitions table and
// the pipeline proto, but the runtime wiring must carry it through:
//
//   pipeline_definitions → FetchPipelineDefinition → PipelineStageConfig
//   → stage input structs → gRPC request → AI coordinator → pipeline_runs
//
// NOTE: prompt_override for triage only takes effect when the pipeline name
// is known before triage runs (i.e., on reprocess, when the pipeline name is
// carried from the previous run). On first processing, triage uses the active
// prompt because the pipeline isn't determined until after triage completes.
// This test simulates a reprocess by setting Pipeline on the workflow input.
//
// ============================================================================

// promptOverridePipelineInput mirrors the real PipelineInput with the Pipeline
// field that the shared e2e PipelineInput struct is missing.
type promptOverridePipelineInput struct {
	TenantID    string `json:"tenant_id"`
	SourceID    int64  `json:"source_id"`
	ContentID   string `json:"content_id,omitempty"`
	JobID       string `json:"job_id"`
	ContentType string `json:"content_type"`
	ContentHash string `json:"content_hash,omitempty"`
	Pipeline    string `json:"pipeline,omitempty"`
}

// TestPromptOverride verifies that prompt_override flows from pipeline_definitions
// through to the AI coordinator and is recorded in pipeline_runs.prompt_version.
func TestPromptOverride(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Clean slate
	err := env.CleanupTestTenant()
	require.NoError(t, err, "cleanup test tenant")
	err = env.EnsureTenantExists()
	require.NoError(t, err, "ensure tenant exists")
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err, "load fixture")

	const testPromptVersion = 99
	const triageStage = "triage"
	const testPipeline = "standard"

	// Step 1: Read the current active triage prompt content.
	// We'll insert a copy as version 99 — same content, different version number.
	// The test doesn't care about prompt content; it cares that the version flows.
	var activeContent string
	err = env.DB.QueryRow(ctx, `
		SELECT content FROM prompt_templates
		WHERE stage = $1 AND is_active = true
		ORDER BY version DESC LIMIT 1
	`, triageStage).Scan(&activeContent)
	require.NoError(t, err, "should find active triage prompt")

	// Step 2: Insert test prompt version 99 (inactive — accessed by explicit version).
	_, err = env.DB.Exec(ctx, `
		INSERT INTO prompt_templates (stage, version, content, is_active, created_at)
		VALUES ($1, $2, $3, false, NOW())
		ON CONFLICT (stage, version) DO UPDATE SET content = EXCLUDED.content
	`, triageStage, testPromptVersion, activeContent+" E2E_MARKER_PROMPT_OVERRIDE_TEST")
	require.NoError(t, err, "should insert test prompt version")
	t.Cleanup(func() {
		_, _ = env.DB.Exec(ctx, `
			DELETE FROM prompt_templates WHERE stage = $1 AND version = $2
		`, triageStage, testPromptVersion)
	})

	// Step 3: Set prompt_override on the standard pipeline's triage stage.
	var savedOverride *int
	err = env.DB.QueryRow(ctx, `
		SELECT prompt_override FROM pipeline_definitions
		WHERE tenant_id = $1 AND pipeline = $2 AND stage = $3
	`, env.TenantID, testPipeline, triageStage).Scan(&savedOverride)
	require.NoError(t, err, "standard pipeline triage definition should exist")

	_, err = env.DB.Exec(ctx, `
		UPDATE pipeline_definitions SET prompt_override = $1
		WHERE tenant_id = $2 AND pipeline = $3 AND stage = $4
	`, testPromptVersion, env.TenantID, testPipeline, triageStage)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = env.DB.Exec(ctx, `
			UPDATE pipeline_definitions SET prompt_override = $1
			WHERE tenant_id = $2 AND pipeline = $3 AND stage = $4
		`, savedOverride, env.TenantID, testPipeline, triageStage)
	})

	t.Logf("Set prompt_override=%d on %s/%s pipeline definition", testPromptVersion, testPipeline, triageStage)

	// Step 4: Ingest a test email.
	sourceID := createEmailSourceCLI(t, env, emailSourceOpts{
		MessageID:   fmt.Sprintf("<prompt-override-test-%d@e2e.test>", time.Now().UnixNano()),
		Subject:     "Prompt Override E2E Test",
		FromAddress: "test@example.com",
		FromName:    "E2E Tester",
		Body:        "This is a test email to verify prompt_override flows end-to-end through the pipeline runtime stack.",
		ExternalID:  fmt.Sprintf("prompt-override-%d", time.Now().UnixNano()),
	})
	require.Greater(t, sourceID, int64(0), "should create source")

	// Step 5: Run pipeline with Pipeline set (simulates reprocess where pipeline
	// name is known from previous run, enabling early FetchPipelineDefinition).
	input := promptOverridePipelineInput{
		TenantID: env.TenantID,
		SourceID: sourceID,
		JobID:    fmt.Sprintf("e2e-prompt-override-%d-%d", sourceID, time.Now().UnixNano()),
		Pipeline: testPipeline,
	}
	wfOpts := client.StartWorkflowOptions{
		ID:        fmt.Sprintf("e2e-pipeline-%d-%d", sourceID, time.Now().UnixNano()),
		TaskQueue: "penfold-main",
	}
	_, err = env.TemporalClient.ExecuteWorkflow(ctx, wfOpts, "SLMPipelineWorkflow", input)
	require.NoError(t, err, "should start pipeline workflow")

	err = env.waitForPipelineComplete(ctx, sourceID, 120*time.Second)
	require.NoError(t, err, "pipeline should complete")

	// Step 6: Verify pipeline completed.
	var status string
	err = env.DB.QueryRow(ctx, `
		SELECT processing_status FROM sources WHERE id = $1
	`, sourceID).Scan(&status)
	require.NoError(t, err)
	require.Equal(t, "completed", status, "pipeline should complete successfully")

	// Step 7: Verify prompt_version was recorded in pipeline_runs for the triage stage.
	// This is the core assertion: prompt_override from pipeline_definitions must
	// flow through to the AI coordinator, and pipeline_runs must record the version used.
	var promptVersion *int
	err = env.DB.QueryRow(ctx, `
		SELECT prompt_version FROM pipeline_runs
		WHERE source_id = $1 AND stage = $2 AND status = 'completed'
		ORDER BY created_at DESC LIMIT 1
	`, sourceID, triageStage).Scan(&promptVersion)
	require.NoError(t, err, "should find triage pipeline_run")
	require.NotNil(t, promptVersion, "prompt_version should be recorded in pipeline_runs (not NULL)")
	assert.Equal(t, testPromptVersion, *promptVersion,
		"triage stage should use prompt version %d from prompt_override, got %d",
		testPromptVersion, *promptVersion)
}
