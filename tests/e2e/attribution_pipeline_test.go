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

// TestAttributionPipeline_ChannelMapping verifies that content ingested from a
// source with a project_source_mapping gets its assertions attributed to the
// project automatically via the attribute_project pipeline stage.
//
// Acceptance test for: Epic 3 Phase 2 — Pipeline attribution stage (channel path)
// Run: go test -tags=e2e ./tests/e2e/ -run TestAttributionPipeline_ChannelMapping -v
func TestAttributionPipeline_ChannelMapping(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	TerminateTestWorkflowsStandalone(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// ============================================================
	// Step 1: Create project and a channel source mapping
	// ============================================================
	t.Log("=== Step 1: Create project with channel mapping ===")

	projectResult := env.CLI.Run(ctx, "project", "add", "AttrChannelProject",
		"--description", "Project for channel-based attribution e2e test",
	)
	require.Equal(t, 0, projectResult.ExitCode, "project add: %s", projectResult.Stderr)

	var projectID int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM projects
		WHERE tenant_id = $1 AND name = 'AttrChannelProject'
	`, env.TenantID).Scan(&projectID)
	require.NoError(t, err)
	t.Logf("Project ID: %d", projectID)

	// Tag the source identifier that will be used for content ingest
	sourceTag := fmt.Sprintf("e2e-attr-channel-%d", time.Now().UnixNano())
	tagResult := env.CLI.Run(ctx, "source", "tag", sourceTag,
		"--project", "AttrChannelProject",
		"--type", "channel",
		"--match", "exact",
	)
	require.Equal(t, 0, tagResult.ExitCode, "source tag: %s", tagResult.Stderr)
	t.Logf("Mapped source '%s' → AttrChannelProject", sourceTag)

	// ============================================================
	// Step 2: Ingest content tagged with the mapped source identifier
	// ============================================================
	t.Log("=== Step 2: Ingest email from mapped source ===")

	emailPath := createTempEmail(t,
		"Weekly Status - AttrChannelProject",
		`Team,

Quick status update for this week.

ACTION: Alice to deploy the new configuration by Friday.
RISK: Integration tests are failing intermittently — needs investigation.
DECISION: We will postpone the migration until next quarter.

Please review and add any missing items.

Regards,
Project Lead`)

	ingestResult := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
	require.Equal(t, 0, ingestResult.ExitCode, "ingest: %s", ingestResult.Stderr)

	// Trigger pipeline and wait
	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err, "should find source with tag %s", sourceTag)
	t.Logf("Source ID: %d", sourceID)

	err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
	require.NoError(t, err, "pipeline should complete")

	// ============================================================
	// Step 3: Verify assertions are attributed to the project
	// ============================================================
	t.Log("=== Step 3: Verify assertion-level attribution ===")

	var attributedAssertionCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM assertions
		WHERE tenant_id = $1
		  AND source_id = $2
		  AND project_id = $3
		  AND is_current = true
	`, env.TenantID, sourceID, projectID).Scan(&attributedAssertionCount)
	require.NoError(t, err)
	t.Logf("Assertions attributed to AttrChannelProject: %d", attributedAssertionCount)

	assert.Greater(t, attributedAssertionCount, 0,
		"expected at least one assertion attributed to project %d via channel mapping. "+
			"The attribute_project stage should set assertions.project_id when the source "+
			"matches a project_source_mapping.", projectID)

	// Verify attribution source is recorded
	var rows []struct {
		id     int64
		src    string
		conf   float64
	}
	dbRows, err := env.DB.Query(ctx, `
		SELECT id, attribution_source, attribution_confidence
		FROM assertions
		WHERE tenant_id = $1
		  AND source_id = $2
		  AND project_id = $3
		  AND is_current = true
		ORDER BY id
	`, env.TenantID, sourceID, projectID)
	require.NoError(t, err)
	defer dbRows.Close()
	for dbRows.Next() {
		var row struct {
			id   int64
			src  string
			conf float64
		}
		require.NoError(t, dbRows.Scan(&row.id, &row.src, &row.conf))
		rows = append(rows, row)
		t.Logf("  Assertion %d: source=%s confidence=%.2f", row.id, row.src, row.conf)
		assert.Equal(t, "channel_mapping", row.src,
			"attribution_source should be 'channel_mapping' for channel-tagged content")
		assert.GreaterOrEqual(t, row.conf, float64(0.9),
			"channel mapping confidence should be ≥ 0.9")
	}

	// ============================================================
	// Step 4: Verify content-level attribution on sources table
	// ============================================================
	t.Log("=== Step 4: Verify content-level attributed_project_ids ===")

	var attributedProjectIDs []int64
	err = env.DB.QueryRow(ctx, `
		SELECT attributed_project_ids FROM sources WHERE id = $1
	`, sourceID).Scan(&attributedProjectIDs)
	require.NoError(t, err)
	t.Logf("sources.attributed_project_ids: %v", attributedProjectIDs)

	assert.Contains(t, attributedProjectIDs, projectID,
		"sources.attributed_project_ids should include the attributed project ID")

	// ============================================================
	// Step 5: Verify via CLI — penf content show shows attribution
	// ============================================================
	t.Log("=== Step 5: Verify via CLI content show ===")

	showResult := env.CLI.Run(ctx, "content", "show", fmt.Sprintf("%d", sourceID), "-o", "json")
	require.Equal(t, 0, showResult.ExitCode, "content show: %s", showResult.Stderr)

	var contentJSON map[string]interface{}
	err = json.Unmarshal([]byte(showResult.Stdout), &contentJSON)
	require.NoError(t, err, "content show output should be valid JSON")

	// content show -o json wraps in {"item": {...}}
	itemRaw, hasItem := contentJSON["item"]
	require.True(t, hasItem, "content show -o json should have top-level 'item' key")
	itemMap, ok := itemRaw.(map[string]interface{})
	require.True(t, ok, "item should be a JSON object")

	attrIDs, exists := itemMap["attributed_project_ids"]
	assert.True(t, exists, "content show item should include attributed_project_ids field")
	if exists {
		t.Logf("content show attributed_project_ids: %v", attrIDs)
	}
}

// TestAttributionPipeline_KeywordMatch verifies that the attribute_project stage
// sets assertions.project_id for content that matches project keywords, even
// when there's no explicit source mapping.
//
// Note: See also TestProjectTagging_KeywordMatchCreatesContentMention which
// tests the content_mentions path. This test verifies the assertion-level attribution.
//
// Acceptance test for: Epic 3 Phase 2 — Pipeline attribution stage (keyword path)
// Run: go test -tags=e2e ./tests/e2e/ -run TestAttributionPipeline_KeywordMatch -v
func TestAttributionPipeline_KeywordMatch(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	TerminateTestWorkflowsStandalone(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// ============================================================
	// Step 1: Create project with distinctive keywords
	// ============================================================
	t.Log("=== Step 1: Create project with keywords ===")

	projectResult := env.CLI.Run(ctx, "project", "add", "AttrKeywordProject",
		"--description", "Project for keyword-based attribution e2e test",
		"--keywords", "attrtest9x7,attr-keyword-project",
	)
	require.Equal(t, 0, projectResult.ExitCode, "project add: %s", projectResult.Stderr)

	var projectID int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM projects
		WHERE tenant_id = $1 AND name = 'AttrKeywordProject'
	`, env.TenantID).Scan(&projectID)
	require.NoError(t, err)
	t.Logf("Project ID: %d", projectID)

	// ============================================================
	// Step 2: Ingest email containing the project keywords (no source mapping)
	// ============================================================
	t.Log("=== Step 2: Ingest keyword-matching email ===")

	sourceTag := fmt.Sprintf("e2e-attr-kw-%d", time.Now().UnixNano())
	emailPath := createTempEmail(t,
		"attrtest9x7 Update — Week 12",
		`Hi,

Update on the attr-keyword-project initiative.

ACTION: Complete the attrtest9x7 rollout by end of month.
DECISION: The attr-keyword-project roadmap has been approved.
RISK: attrtest9x7 depends on the legacy migration being finished first.

Thanks,
Team Lead`)

	ingestResult := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
	require.Equal(t, 0, ingestResult.ExitCode, "ingest: %s", ingestResult.Stderr)

	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err)
	t.Logf("Source ID: %d", sourceID)

	err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
	require.NoError(t, err, "pipeline should complete")

	// ============================================================
	// Step 3: Verify keyword-based assertion attribution
	// ============================================================
	t.Log("=== Step 3: Verify keyword-based attribution ===")

	var attributedCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM assertions
		WHERE tenant_id = $1
		  AND source_id = $2
		  AND project_id = $3
		  AND is_current = true
	`, env.TenantID, sourceID, projectID).Scan(&attributedCount)
	require.NoError(t, err)
	t.Logf("Assertions attributed via keyword match: %d", attributedCount)

	assert.Greater(t, attributedCount, 0,
		"expected at least one assertion attributed to project %d via keyword matching. "+
			"The attribute_project stage should match 'attrtest9x7' and 'attr-keyword-project' "+
			"against projects.keywords[] and set assertions.project_id.", projectID)

	// Check attribution source is 'keyword' for these
	dbRows, err := env.DB.Query(ctx, `
		SELECT id, attribution_source, attribution_confidence
		FROM assertions
		WHERE tenant_id = $1
		  AND source_id = $2
		  AND project_id = $3
		  AND is_current = true
	`, env.TenantID, sourceID, projectID)
	require.NoError(t, err)
	defer dbRows.Close()
	for dbRows.Next() {
		var id int64
		var src string
		var conf float64
		require.NoError(t, dbRows.Scan(&id, &src, &conf))
		t.Logf("  Assertion %d: source=%s confidence=%.2f", id, src, conf)
		assert.Contains(t, []string{"keyword", "llm"},
			src, "attribution_source should be 'keyword' or 'llm' for keyword-matched content")
	}
}

// TestAttributionPipeline_MultiProject verifies that content matching multiple
// projects gets attributed to all of them (multi-project content is valid).
//
// Acceptance test for: Epic 3 Phase 2 — multi-project attribution
// Run: go test -tags=e2e ./tests/e2e/ -run TestAttributionPipeline_MultiProject -v
func TestAttributionPipeline_MultiProject(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	TerminateTestWorkflowsStandalone(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Create two projects with different keywords
	env.CLI.Run(ctx, "project", "add", "MultiAttrAlpha", "--keywords", "multialpha9x7")
	env.CLI.Run(ctx, "project", "add", "MultiAttrBeta", "--keywords", "multibeta9x7")

	var alphaID, betaID int64
	env.DB.QueryRow(ctx, `SELECT id FROM projects WHERE tenant_id = $1 AND name = 'MultiAttrAlpha'`, env.TenantID).Scan(&alphaID)
	env.DB.QueryRow(ctx, `SELECT id FROM projects WHERE tenant_id = $1 AND name = 'MultiAttrBeta'`, env.TenantID).Scan(&betaID)
	require.NotZero(t, alphaID)
	require.NotZero(t, betaID)
	t.Logf("Alpha ID: %d, Beta ID: %d", alphaID, betaID)

	// Ingest email mentioning both keywords
	sourceTag := fmt.Sprintf("e2e-multi-attr-%d", time.Now().UnixNano())
	emailPath := createTempEmail(t,
		"Cross-project alignment — multialpha9x7 and multibeta9x7",
		`Team,

ACTION: Sync the multialpha9x7 team with multibeta9x7 on the shared API surface.
DECISION: multialpha9x7 will own the upstream interface; multibeta9x7 will consume it.
RISK: Dependency between multialpha9x7 and multibeta9x7 delivery timelines.

Let's align by end of week.`)

	ingestResult := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
	require.Equal(t, 0, ingestResult.ExitCode, "ingest: %s", ingestResult.Stderr)

	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err)

	err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
	require.NoError(t, err)

	// Verify assertions attributed to BOTH projects
	var alphaCount, betaCount int
	env.DB.QueryRow(ctx, `SELECT COUNT(*) FROM assertions WHERE tenant_id = $1 AND source_id = $2 AND project_id = $3 AND is_current = true`, env.TenantID, sourceID, alphaID).Scan(&alphaCount)
	env.DB.QueryRow(ctx, `SELECT COUNT(*) FROM assertions WHERE tenant_id = $1 AND source_id = $2 AND project_id = $3 AND is_current = true`, env.TenantID, sourceID, betaID).Scan(&betaCount)

	t.Logf("Assertions attributed to Alpha: %d, Beta: %d", alphaCount, betaCount)

	assert.Greater(t, alphaCount, 0,
		"expected assertions attributed to MultiAttrAlpha (id=%d) via keyword 'multialpha9x7'", alphaID)
	assert.Greater(t, betaCount, 0,
		"expected assertions attributed to MultiAttrBeta (id=%d) via keyword 'multibeta9x7'", betaID)

	// Verify sources.attributed_project_ids contains both
	var attributedProjectIDs []int64
	err = env.DB.QueryRow(ctx, `SELECT attributed_project_ids FROM sources WHERE id = $1`, sourceID).Scan(&attributedProjectIDs)
	require.NoError(t, err)
	t.Logf("sources.attributed_project_ids: %v", attributedProjectIDs)

	assert.Contains(t, attributedProjectIDs, alphaID, "attributed_project_ids should include Alpha")
	assert.Contains(t, attributedProjectIDs, betaID, "attributed_project_ids should include Beta")
}
