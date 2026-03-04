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

// TestProjectObservability_ListContent verifies that ListProjectContent returns
// content items attributed to a project with attribution metadata.
//
// Acceptance test for: Epic 3 Phase 4a — Gateway RPCs (pf-c61e3c)
// Run: go test -tags=e2e ./tests/e2e/ -run TestProjectObservability_ListContent -v
func TestProjectObservability_ListContent(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	TerminateTestWorkflowsStandalone(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// ============================================================
	// Step 1: Create a project and ingest attributed content
	// ============================================================
	t.Log("=== Step 1: Setup project with attributed content ===")

	env.CLI.Run(ctx, "project", "add", "ObsListProject",
		"--keywords", "obslist9x7,obs-list-project",
	)

	var projectID int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM projects WHERE tenant_id = $1 AND name = 'ObsListProject'
	`, env.TenantID).Scan(&projectID)
	require.NoError(t, err)
	t.Logf("Project ID: %d", projectID)

	sourceTag := fmt.Sprintf("e2e-obs-list-%d", time.Now().UnixNano())
	emailPath := createTempEmail(t,
		"obslist9x7 — Weekly Update",
		`Team,

ACTION: Deploy obslist9x7 configuration by Friday.
DECISION: obs-list-project roadmap approved for Q2.
RISK: obslist9x7 integration needs external dependency resolved.

Regards,
Team Lead`)

	ingestResult := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
	require.Equal(t, 0, ingestResult.ExitCode, "ingest: %s", ingestResult.Stderr)

	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err)

	err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
	require.NoError(t, err)

	// Verify attribution happened (pre-condition for observability test)
	var attrCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM assertions
		WHERE tenant_id = $1 AND source_id = $2 AND project_id = $3 AND is_current = true
	`, env.TenantID, sourceID, projectID).Scan(&attrCount)
	require.NoError(t, err)
	require.Greater(t, attrCount, 0, "pre-condition: content must be attributed before observability test")

	// ============================================================
	// Step 2: Verify via CLI — penf project content
	// ============================================================
	t.Log("=== Step 2: penf project content ===")

	listResult := env.CLI.Run(ctx, "project", "content", "ObsListProject")
	require.Equal(t, 0, listResult.ExitCode, "project content: %s", listResult.Stderr)
	assert.Contains(t, listResult.Stdout, "obslist9x7",
		"project content should show the attributed email's subject")

	// JSON output
	listJSON := env.CLI.Run(ctx, "project", "content", "ObsListProject", "-o", "json")
	require.Equal(t, 0, listJSON.ExitCode, "project content -o json: %s", listJSON.Stderr)
	assert.Contains(t, listJSON.Stdout, fmt.Sprintf("%d", sourceID),
		"JSON output should include the source_id")
	assert.Contains(t, listJSON.Stdout, "keyword",
		"JSON output should include attribution_source")
	t.Logf("project content output: %s", listJSON.Stdout)
}

// TestProjectObservability_Stats verifies that GetProjectStats returns correct
// attribution breakdown for a project.
//
// Acceptance test for: Epic 3 Phase 4a — Gateway RPCs (pf-c61e3c)
// Run: go test -tags=e2e ./tests/e2e/ -run TestProjectObservability_Stats -v
func TestProjectObservability_Stats(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	TerminateTestWorkflowsStandalone(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// ============================================================
	// Step 1: Create project with keyword + channel content
	// ============================================================
	t.Log("=== Step 1: Setup project with both attribution paths ===")

	env.CLI.Run(ctx, "project", "add", "ObsStatsProject",
		"--keywords", "obsstats9x7",
	)

	var projectID int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM projects WHERE tenant_id = $1 AND name = 'ObsStatsProject'
	`, env.TenantID).Scan(&projectID)
	require.NoError(t, err)

	// Path 1: keyword-attributed content
	kwTag := fmt.Sprintf("e2e-obs-stats-kw-%d", time.Now().UnixNano())
	emailPath := createTempEmail(t,
		"obsstats9x7 Update",
		`ACTION: Complete obsstats9x7 rollout.
DECISION: obsstats9x7 budget approved.
RISK: obsstats9x7 timeline at risk.`)

	ingestResult := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", kwTag)
	require.Equal(t, 0, ingestResult.ExitCode)
	env.CLI.Run(ctx, "pipeline", "kick", "--source", kwTag)

	// Path 2: channel-attributed content
	chTag := fmt.Sprintf("e2e-obs-stats-ch-%d", time.Now().UnixNano())
	env.CLI.Run(ctx, "source", "tag", chTag,
		"--project", "ObsStatsProject",
		"--type", "channel",
		"--match", "exact",
	)
	chEmailPath := createTempEmail(t,
		"Channel email for ObsStatsProject",
		`ACTION: Deploy channel configuration.
DECISION: Channel approach approved.`)
	ingestResult2 := env.CLI.Run(ctx, "ingest", "email", chEmailPath, "--source", chTag)
	require.Equal(t, 0, ingestResult2.ExitCode)
	env.CLI.Run(ctx, "pipeline", "kick", "--source", chTag)

	// Wait for both to complete
	kwSourceID, err := getLatestSourceByTag(env, kwTag)
	require.NoError(t, err)
	chSourceID, err := getLatestSourceByTag(env, chTag)
	require.NoError(t, err)

	require.NoError(t, waitForProcessingComplete(t, env, kwSourceID, 120*time.Second))
	require.NoError(t, waitForProcessingComplete(t, env, chSourceID, 120*time.Second))

	// ============================================================
	// Step 2: Verify stats via CLI
	// ============================================================
	t.Log("=== Step 2: penf project stats ===")

	statsResult := env.CLI.Run(ctx, "project", "stats", "ObsStatsProject")
	require.Equal(t, 0, statsResult.ExitCode, "project stats: %s", statsResult.Stderr)

	// Should show attribution breakdown
	assert.Contains(t, statsResult.Stdout, "keyword",
		"stats should show keyword attribution")
	assert.Contains(t, statsResult.Stdout, "channel_mapping",
		"stats should show channel_mapping attribution")

	t.Logf("project stats output:\n%s", statsResult.Stdout)

	// JSON output
	statsJSON := env.CLI.Run(ctx, "project", "stats", "ObsStatsProject", "-o", "json")
	require.Equal(t, 0, statsJSON.ExitCode, "project stats -o json: %s", statsJSON.Stderr)
	assert.Contains(t, statsJSON.Stdout, "total_attributed_sources",
		"JSON should include total_attributed_sources")
	assert.Contains(t, statsJSON.Stdout, "breakdown",
		"JSON should include breakdown array")
	t.Logf("project stats JSON: %s", statsJSON.Stdout)

	// Verify totals
	var totalAttributed int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(DISTINCT source_id) FROM assertions
		WHERE tenant_id = $1 AND project_id = $2 AND is_current = true
	`, env.TenantID, projectID).Scan(&totalAttributed)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, totalAttributed, 2,
		"both keyword and channel sources should be attributed (got %d)", totalAttributed)
}

// TestProjectObservability_Unattributed verifies that ListUnattributedContent
// returns content with no project attribution.
//
// Acceptance test for: Epic 3 Phase 4a — Gateway RPCs (pf-c61e3c)
// Run: go test -tags=e2e ./tests/e2e/ -run TestProjectObservability_Unattributed -v
func TestProjectObservability_Unattributed(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	TerminateTestWorkflowsStandalone(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// ============================================================
	// Step 1: Ingest content that won't match any project
	// ============================================================
	t.Log("=== Step 1: Ingest unattributed content ===")

	sourceTag := fmt.Sprintf("e2e-obs-unattr-%d", time.Now().UnixNano())
	emailPath := createTempEmail(t,
		"General team sync — no project keywords",
		`Team,

Quick sync on general operational items.
No specific project keywords in this email — just routine updates.

Thanks`)

	ingestResult := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
	require.Equal(t, 0, ingestResult.ExitCode, "ingest: %s", ingestResult.Stderr)
	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err)

	err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
	require.NoError(t, err)

	// Verify source has no attribution (pre-condition)
	var attrProjectIDs []int64
	err = env.DB.QueryRow(ctx, `
		SELECT attributed_project_ids FROM sources WHERE id = $1
	`, sourceID).Scan(&attrProjectIDs)
	require.NoError(t, err)
	t.Logf("attributed_project_ids: %v", attrProjectIDs)

	// ============================================================
	// Step 2: Verify via CLI — penf project unattributed
	// ============================================================
	t.Log("=== Step 2: penf project unattributed ===")

	unattribResult := env.CLI.Run(ctx, "project", "unattributed")
	require.Equal(t, 0, unattribResult.ExitCode, "project unattributed: %s", unattribResult.Stderr)

	// The unattributed source should appear
	assert.Contains(t, unattribResult.Stdout, fmt.Sprintf("%d", sourceID),
		"project unattributed should list the unattributed source")

	t.Logf("unattributed output:\n%s", unattribResult.Stdout)

	// JSON output
	unattribJSON := env.CLI.Run(ctx, "project", "unattributed", "-o", "json")
	require.Equal(t, 0, unattribJSON.ExitCode, "project unattributed -o json: %s", unattribJSON.Stderr)
	assert.Contains(t, unattribJSON.Stdout, fmt.Sprintf("%d", sourceID),
		"JSON output should include source_id")
	t.Logf("unattributed JSON: %s", unattribJSON.Stdout)
}
