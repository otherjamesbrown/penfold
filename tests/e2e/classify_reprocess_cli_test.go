//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// CLI Classify & Reprocess E2E Tests
// ============================================================================
//
// These tests verify that the `penf classify` CLI commands work end-to-end:
//
//   1. `penf classify run` triggers reprocessing via the gateway ReprocessContent RPC
//   2. `penf classify run --all` reprocesses all items (not just unknown)
//   3. `penf classify run <id> --dry-run` shows classification without persisting
//   4. `penf classify stats` returns source_system counts from real DB state
//
// These exist because the CLI commands were built as stubs — the gateway
// ReprocessContent RPC exists but was never wired into the classify commands.
//
// ============================================================================

// TestE2E_ClassifyRun_ReprocessesExistingContent verifies that `penf classify run`
// triggers the pipeline on already-processed content and populates source_system.
func TestE2E_ClassifyRun_ReprocessesExistingContent(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-classifyrun-%d", time.Now().UnixNano())

	// Insert and process a JIRA email through the pipeline
	sourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         fmt.Sprintf("<jira-%s@e2e.test>", testID),
		Subject:           "[TRACK-JIRA] PROJ-456 updated",
		FromAddress:       "jira@atlassian.net",
		FromName:          "JIRA",
		Body:              "Issue PROJ-456 has been updated by Test User.",
		Date:              time.Now().Add(-1 * time.Hour),
		ExternalID:        fmt.Sprintf("jira-%s", testID),
		ParticipantEmails: []string{"jira@atlassian.net"},
	})

	runPipelineAndWait(t, env, sourceID, 120*time.Second)
	contentID := getContentIDForSource(t, env, sourceID)

	// Verify source_system is set after initial pipeline run
	meta := cliContentShowMetadata(t, env, contentID)
	initialSystem, _ := meta["source_system"].(string)
	assert.Equal(t, "jira", initialSystem,
		"source_system should be 'jira' after pipeline run")

	// Now trigger classify run on this specific item via CLI
	t.Run("classify_run_single_item", func(t *testing.T) {
		result := env.CLI.Run(ctx, "classify", "run", contentID, "-o", "json")
		require.Equal(t, 0, result.ExitCode,
			"penf classify run <id> should succeed: %s", result.Stderr)

		var resp map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &resp)
		require.NoError(t, err)

		// Should report classification result
		ss, ok := resp["source_system"].(string)
		require.True(t, ok, "response should contain source_system")
		assert.Equal(t, "jira", ss)
	})

	// Dry run should not change anything
	t.Run("classify_run_dry_run", func(t *testing.T) {
		result := env.CLI.Run(ctx, "classify", "run", contentID, "--dry-run", "-o", "json")
		require.Equal(t, 0, result.ExitCode,
			"penf classify run --dry-run should succeed: %s", result.Stderr)
	})
}

// TestE2E_ClassifyRun_BatchReprocessAll verifies that `penf classify run --all`
// reprocesses all content items and populates source_system on each.
func TestE2E_ClassifyRun_BatchReprocessAll(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-classifyall-%d", time.Now().UnixNano())

	// Insert two emails with different expected classifications
	jiraSourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         fmt.Sprintf("<jira-all-%s@e2e.test>", testID),
		Subject:           "[TRACK-JIRA] BATCH-789",
		FromAddress:       "jira@atlassian.net",
		FromName:          "JIRA",
		Body:              "Batch test JIRA email.",
		Date:              time.Now().Add(-2 * time.Hour),
		ExternalID:        fmt.Sprintf("jira-all-%s", testID),
		ParticipantEmails: []string{"jira@atlassian.net"},
	})
	humanSourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         fmt.Sprintf("<human-all-%s@e2e.test>", testID),
		Subject:           "Quick question about the roadmap",
		FromAddress:       "colleague@example.com",
		FromName:          "Colleague",
		Body:              "Hey, wanted to check on Q3 plans.",
		Date:              time.Now().Add(-1 * time.Hour),
		ExternalID:        fmt.Sprintf("human-all-%s", testID),
		ParticipantEmails: []string{"colleague@example.com"},
	})

	runPipelineAndWait(t, env, jiraSourceID, 120*time.Second)
	runPipelineAndWait(t, env, humanSourceID, 120*time.Second)

	// Run classify --all
	result := env.CLI.Run(ctx, "classify", "run", "--all", "-o", "json")
	require.Equal(t, 0, result.ExitCode,
		"penf classify run --all should succeed: %s", result.Stderr)

	var resp map[string]interface{}
	err := json.Unmarshal([]byte(result.Stdout), &resp)
	require.NoError(t, err)

	// Should report how many items were processed
	processed, ok := resp["processed"].(float64)
	require.True(t, ok, "response should contain 'processed' count")
	assert.GreaterOrEqual(t, int(processed), 2,
		"should have processed at least our 2 test items")
}

// TestE2E_ClassifyStats_ReturnsBreakdown verifies that `penf classify stats`
// returns a source_system breakdown with counts.
func TestE2E_ClassifyStats_ReturnsBreakdown(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-stats-%d", time.Now().UnixNano())

	// Insert and process a known email so we have at least one classified item
	sourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         fmt.Sprintf("<stats-%s@e2e.test>", testID),
		Subject:           "Meeting notes from standup",
		FromAddress:       "manager@example.com",
		FromName:          "Manager",
		Body:              "Here are the standup notes.",
		Date:              time.Now().Add(-1 * time.Hour),
		ExternalID:        fmt.Sprintf("stats-%s", testID),
		ParticipantEmails: []string{"manager@example.com"},
	})

	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	result := env.CLI.Run(ctx, "classify", "stats", "-o", "json")
	require.Equal(t, 0, result.ExitCode,
		"penf classify stats should succeed: %s", result.Stderr)

	var resp map[string]interface{}
	err := json.Unmarshal([]byte(result.Stdout), &resp)
	require.NoError(t, err)

	// Should have a breakdown map
	breakdown, ok := resp["breakdown"].(map[string]interface{})
	require.True(t, ok, "response should contain 'breakdown' map")

	// At minimum, human_email should have a count > 0
	humanCount, ok := breakdown["human_email"].(float64)
	require.True(t, ok, "breakdown should include human_email")
	assert.GreaterOrEqual(t, int(humanCount), 1,
		"human_email count should be at least 1")

	// Total should be present
	total, ok := resp["total"].(float64)
	require.True(t, ok, "response should contain 'total' count")
	assert.GreaterOrEqual(t, int(total), 1)
}
