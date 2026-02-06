//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSLMPipeline_FullOutcomeVerification is a comprehensive E2E test that
// verifies the full SLM pipeline produces actual output: assertions, embeddings,
// and correct pipeline stage records. This test would have caught pf-f9ce32
// (gateway routing to old workflow) because the old workflow doesn't produce
// assertions.
func TestSLMPipeline_FullOutcomeVerification(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Terminate stale workflows from previous test runs before cleanup
	TerminateTestWorkflowsStandalone(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Step 1: Create a high-importance email that triage will NOT skip.
	// Security alerts and decision emails reliably trigger full extraction.
	emailPath := createTempEmail(t,
		"URGENT: Critical security incident - immediate action required",
		`Team,

We have confirmed a critical security breach in the production environment.

Key findings:
- Unauthorized access detected on the customer database at 02:15 UTC
- Approximately 15,000 user records may have been exposed
- The attack vector was an unpatched API endpoint in the authentication service
- Sarah Chen from the security team has been leading the investigation

Immediate actions required:
1. All engineering teams must rotate their API credentials by end of day
2. Marcus Johnson will coordinate the incident response
3. A full post-mortem is scheduled for Friday with the executive team

Risk assessment: HIGH
Estimated impact: All customer-facing services
Decision: We are initiating a mandatory security review of all production endpoints

This is the highest priority item for the entire engineering organization.

-Security Team`)

	sourceTag := fmt.Sprintf("e2e-outcome-%d", time.Now().UnixNano())

	// Step 2: Ingest via CLI
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
	require.Equal(t, 0, result.ExitCode, "ingest should succeed: %s", result.Stderr)
	t.Logf("Ingest output: %s", result.Stdout)

	// Step 3: Trigger pipeline processing
	kickResult := env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)
	t.Logf("Pipeline kick output: %s", kickResult.Stdout)

	// Step 4: Get the source ID and wait for completion
	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err, "should find source with tag %s", sourceTag)
	t.Logf("Source ID: %d", sourceID)

	err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
	require.NoError(t, err, "pipeline should complete within timeout")

	// ============================================================
	// Step 5: Verify pipeline stages completed
	// ============================================================
	assertStageCompleted(t, env, sourceID, "embed")
	assertStageCompleted(t, env, sourceID, "extract_ner")
	assertStageCompleted(t, env, sourceID, "extract_semantic")

	// Log all pipeline runs for debugging
	var stageCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM pipeline_runs WHERE source_id = $1
	`, sourceID).Scan(&stageCount)
	require.NoError(t, err)
	t.Logf("Total pipeline stages recorded: %d", stageCount)

	// ============================================================
	// Step 6: Verify source status = completed
	// ============================================================
	var status string
	err = env.DB.QueryRow(ctx,
		"SELECT processing_status FROM sources WHERE id = $1", sourceID).Scan(&status)
	require.NoError(t, err)
	require.Equal(t, "completed", status, "source processing_status should be 'completed'")

	// ============================================================
	// Step 7: Verify assertions were created (THE CRITICAL GAP)
	// The old ContentIngestionWorkflow did NOT produce assertions.
	// If this fails, the pipeline is routing to the wrong workflow.
	// ============================================================
	assertions, err := getAssertionsForSource(env, sourceID)
	require.NoError(t, err, "should be able to query assertions")
	require.Greater(t, len(assertions), 0,
		"pipeline MUST produce assertions for high-importance content; "+
			"if zero, check that gateway routes to SLMPipelineWorkflow not ContentIngestionWorkflow")

	t.Logf("Assertions created: %d", len(assertions))
	for _, a := range assertions {
		t.Logf("  [%s] %s (confidence: %.2f, current: %v)",
			a["type"], a["description"], a["confidence"], a["is_current"])
	}

	// Verify at least one assertion is marked as current
	var hasCurrent bool
	for _, a := range assertions {
		if current, ok := a["is_current"].(bool); ok && current {
			hasCurrent = true
			break
		}
	}
	assert.True(t, hasCurrent, "at least one assertion should be marked is_current=true")

	// ============================================================
	// Step 8: Verify embeddings were created
	// ============================================================
	embeddings, err := countEmbeddingsForSource(env, sourceID)
	require.NoError(t, err, "should be able to query embeddings")

	totalEmbeddings := 0
	for repType, count := range embeddings {
		t.Logf("  Embeddings [%s]: %d", repType, count)
		totalEmbeddings += count
	}
	require.Greater(t, totalEmbeddings, 0,
		"pipeline MUST produce embeddings; if zero, embed stage may have silently failed")
}
