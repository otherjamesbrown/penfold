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

// TestProjectTagging_KeywordMatchCreatesContentMention verifies that when a
// project has keywords defined, the pipeline creates content_mentions with
// entity_type='project' for content that contains those keywords.
//
// Bug: The data model supports project mentions (content_mentions.entity_type
// includes 'project', the CLI has --project-id filtering on mentions), but the
// pipeline stages do not perform project keyword matching. After ingestion and
// processing, no content_mentions rows are created with entity_type='project'.
//
// This test defines the expected behavior: creating a project with keywords,
// ingesting content that contains those keywords, and verifying that the
// pipeline links the content to the project via content_mentions.
//
// Expected: FAIL (pipeline does not implement project keyword matching yet).
func TestProjectTagging_KeywordMatchCreatesContentMention(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Terminate stale workflows from previous test runs before cleanup
	TerminateTestWorkflowsStandalone(t)

	// Setup: Clean slate with entity fixtures
	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// ============================================================
	// Step 1: Create a project with keywords via CLI
	// ============================================================
	t.Log("=== Step 1: Create project with keywords ===")

	projectResult := env.CLI.Run(ctx, "project", "add", "ManagedTrafficController",
		"--description", "Managed Traffic Controller project",
		"--keywords", "mtc,managed traffic controller,traffic-controller",
	)
	if projectResult.ExitCode != 0 {
		t.Logf("project add stderr: %s", projectResult.Stderr)
		t.Logf("project add stdout: %s", projectResult.Stdout)
	}
	require.Equal(t, 0, projectResult.ExitCode,
		"project add should succeed: %s", projectResult.Stderr)
	t.Logf("Project created: %s", projectResult.Stdout)

	// Verify the project was created in the database with keywords
	var projectID int64
	var projectName string
	var keywords []string
	err = env.DB.QueryRow(ctx, `
		SELECT id, name, keywords
		FROM projects
		WHERE tenant_id = $1 AND name = 'ManagedTrafficController'
	`, env.TenantID).Scan(&projectID, &projectName, &keywords)
	require.NoError(t, err, "project should exist in database")
	t.Logf("Project ID: %d, Name: %s, Keywords: %v", projectID, projectName, keywords)
	require.Greater(t, len(keywords), 0, "project should have keywords set")

	// ============================================================
	// Step 2: Ingest an email that contains the project keywords
	// ============================================================
	t.Log("=== Step 2: Ingest email with project keywords ===")

	// Create a temp email that mentions the project keywords multiple times
	// to make keyword matching unambiguous.
	emailPath := createTempEmail(t,
		"MTC Status Update - Managed Traffic Controller Phase 2",
		`Hi team,

Here is this week's update on the MTC project (Managed Traffic Controller).

Key progress:
- The traffic-controller routing module passed integration tests
- MTC dashboard v2 is now deployed to staging
- Managed traffic controller config migration is 80% complete

Next steps:
- Complete the MTC API documentation
- Run load tests on the traffic-controller ingress path
- Final review of managed traffic controller failover behavior

Please review and flag any blockers.

Thanks,
Engineering Lead`)

	sourceTag := fmt.Sprintf("e2e-project-tag-%d", time.Now().UnixNano())

	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
	if result.ExitCode != 0 {
		t.Logf("Ingest stderr: %s", result.Stderr)
		t.Logf("Ingest stdout: %s", result.Stdout)
	}
	require.Equal(t, 0, result.ExitCode,
		"ingest should succeed: %s", result.Stderr)
	t.Logf("Ingest completed in %v", result.Duration)

	// ============================================================
	// Step 3: Trigger pipeline processing and wait for completion
	// ============================================================
	t.Log("=== Step 3: Trigger pipeline and wait for completion ===")

	kickResult := env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)
	t.Logf("Pipeline kick output: %s", kickResult.Stdout)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err, "should find source with tag %s", sourceTag)
	t.Logf("Source ID: %d", sourceID)

	err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
	require.NoError(t, err, "pipeline should complete within timeout")

	// Verify pipeline completed successfully
	var processingStatus string
	err = env.DB.QueryRow(ctx,
		"SELECT processing_status FROM sources WHERE id = $1", sourceID,
	).Scan(&processingStatus)
	require.NoError(t, err)
	require.Equal(t, "completed", processingStatus, "source should be fully processed")
	t.Logf("Pipeline completed: status=%s", processingStatus)

	// ============================================================
	// Step 4: Verify content_mentions exist with entity_type=project
	// ============================================================
	t.Log("=== Step 4: Verify project mentions in content_mentions ===")

	// Query for any content_mentions with entity_type='project' for this content
	var projectMentionCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM content_mentions
		WHERE tenant_id = $1
		  AND content_id = $2
		  AND entity_type = 'project'
	`, env.TenantID, sourceID).Scan(&projectMentionCount)
	require.NoError(t, err, "should be able to query content_mentions")

	t.Logf("Project mentions found: %d", projectMentionCount)

	// THE KEY ASSERTION: pipeline should create project mentions from keyword matching
	assert.Greater(t, projectMentionCount, 0,
		"pipeline should create content_mentions with entity_type='project' "+
			"when content contains project keywords (mtc, managed traffic controller). "+
			"This is the bug: project keyword matching is not implemented in the pipeline.")

	// If we do have mentions, verify they resolve to the correct project
	if projectMentionCount > 0 {
		rows, err := env.DB.Query(ctx, `
			SELECT mentioned_text, resolved_entity_id, resolution_confidence, status
			FROM content_mentions
			WHERE tenant_id = $1
			  AND content_id = $2
			  AND entity_type = 'project'
			ORDER BY id
		`, env.TenantID, sourceID)
		require.NoError(t, err)
		defer rows.Close()

		for rows.Next() {
			var text, status string
			var resolvedID *int64
			var confidence *float64
			err := rows.Scan(&text, &resolvedID, &confidence, &status)
			require.NoError(t, err)

			t.Logf("  Project mention: %q resolved_entity_id=%v confidence=%v status=%s",
				text, resolvedID, confidence, status)

			// If resolved, it should point to our project
			if resolvedID != nil {
				assert.Equal(t, projectID, *resolvedID,
					"resolved project mention should point to the ManagedTrafficController project (id=%d)", projectID)
			}
		}
	}

	// Also log all mentions for debugging to show what the pipeline DID create
	t.Log("=== All content_mentions for this source (for debugging) ===")
	allRows, err := env.DB.Query(ctx, `
		SELECT mentioned_text, entity_type::text, status::text, resolved_entity_id
		FROM content_mentions
		WHERE tenant_id = $1 AND content_id = $2
		ORDER BY id
	`, env.TenantID, sourceID)
	if err == nil {
		defer allRows.Close()
		found := false
		for allRows.Next() {
			found = true
			var text, entityType, status string
			var resolvedID *int64
			allRows.Scan(&text, &entityType, &status, &resolvedID)
			resolvedStr := "<nil>"
			if resolvedID != nil {
				resolvedStr = fmt.Sprintf("%d", *resolvedID)
			}
			t.Logf("  Mention: %q entity_type=%s status=%s resolved_entity_id=%s",
				text, entityType, status, resolvedStr)
		}
		if !found {
			t.Log("  (no mentions found at all)")
		}
	}
}

// TestProjectTagging_ReprocessCreatesProjectMention verifies that reprocessing
// existing content after a project is created picks up the project keywords
// and creates content_mentions for the project.
//
// This covers the scenario where content was ingested before the project existed,
// then the project is created, and content is reprocessed.
//
// Expected: FAIL (same underlying bug — pipeline doesn't do project keyword matching).
func TestProjectTagging_ReprocessCreatesProjectMention(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Terminate stale workflows from previous test runs before cleanup
	TerminateTestWorkflowsStandalone(t)

	// Setup: Clean slate with entity fixtures
	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// ============================================================
	// Step 1: Ingest email FIRST (before the project exists)
	// ============================================================
	t.Log("=== Step 1: Ingest email before project exists ===")

	emailPath := createTempEmail(t,
		"Phoenix Platform Launch Planning",
		`Team,

Quick update on Phoenix Platform progress. The phoenix-platform deployment
pipeline is now operational. Key items:

- Phoenix platform load testing passed all benchmarks
- The phoenix-platform API gateway handles 50k req/s
- Final review of Phoenix Platform security posture is next week

Let's discuss at Thursday's standup.

-Ops Team`)

	sourceTag := fmt.Sprintf("e2e-reprocess-tag-%d", time.Now().UnixNano())

	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
	require.Equal(t, 0, result.ExitCode,
		"ingest should succeed: %s", result.Stderr)

	// Trigger and wait for initial processing
	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err, "should find source with tag %s", sourceTag)
	t.Logf("Source ID: %d", sourceID)

	err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
	require.NoError(t, err, "initial pipeline should complete")

	// Confirm no project mentions exist yet (project doesn't exist)
	var mentionsBefore int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM content_mentions
		WHERE tenant_id = $1
		  AND content_id = $2
		  AND entity_type = 'project'
	`, env.TenantID, sourceID).Scan(&mentionsBefore)
	// It's fine if this errors (e.g., no rows) — we just want the count
	if err != nil {
		mentionsBefore = 0
	}
	t.Logf("Project mentions before project creation: %d", mentionsBefore)

	// ============================================================
	// Step 2: Create the project with keywords
	// ============================================================
	t.Log("=== Step 2: Create project with keywords ===")

	projectResult := env.CLI.Run(ctx, "project", "add", "PhoenixPlatform",
		"--description", "Phoenix Platform launch project",
		"--keywords", "phoenix platform,phoenix-platform",
	)
	require.Equal(t, 0, projectResult.ExitCode,
		"project add should succeed: %s", projectResult.Stderr)

	var projectID int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM projects
		WHERE tenant_id = $1 AND name = 'PhoenixPlatform'
	`, env.TenantID).Scan(&projectID)
	require.NoError(t, err, "project should exist in database")
	t.Logf("Project ID: %d", projectID)

	// ============================================================
	// Step 3: Reprocess the content
	// ============================================================
	t.Log("=== Step 3: Reprocess content ===")

	// Reset processing status to allow reprocessing
	_, err = env.DB.Exec(ctx,
		"UPDATE sources SET processing_status = 'pending' WHERE id = $1", sourceID)
	require.NoError(t, err)

	// Trigger reprocessing
	kickResult := env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)
	t.Logf("Pipeline kick output: %s", kickResult.Stdout)

	err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
	require.NoError(t, err, "reprocessing should complete")

	// ============================================================
	// Step 4: Verify project mentions are now created
	// ============================================================
	t.Log("=== Step 4: Verify project mentions after reprocessing ===")

	var mentionsAfter int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM content_mentions
		WHERE tenant_id = $1
		  AND content_id = $2
		  AND entity_type = 'project'
	`, env.TenantID, sourceID).Scan(&mentionsAfter)
	require.NoError(t, err, "should be able to query content_mentions")

	t.Logf("Project mentions after reprocessing: %d", mentionsAfter)

	// THE KEY ASSERTION: reprocessing should create project mentions
	assert.Greater(t, mentionsAfter, 0,
		"after reprocessing, pipeline should create content_mentions with entity_type='project' "+
			"for content that matches project keywords (phoenix platform, phoenix-platform). "+
			"This is the bug: project keyword matching is not implemented in the pipeline.")

	// Verify the mentions resolve to the correct project if they exist
	if mentionsAfter > 0 {
		var resolvedToProject int
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM content_mentions
			WHERE tenant_id = $1
			  AND content_id = $2
			  AND entity_type = 'project'
			  AND resolved_entity_id = $3
		`, env.TenantID, sourceID, projectID).Scan(&resolvedToProject)
		require.NoError(t, err)

		assert.Greater(t, resolvedToProject, 0,
			"project mentions should resolve to the PhoenixPlatform project (id=%d)", projectID)
		t.Logf("Mentions resolved to PhoenixPlatform: %d", resolvedToProject)
	}
}
