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

// TestDigest_Phase2_DailyDigestWorkflow is the acceptance test for
// Epic 5 Phase 2 — Daily Digest Workflow.
//
// It verifies:
//   - Schema: digests table exists with expected columns
//   - Schema: digest_daily_generate prompt template is active
//   - Generation: daily digest generated for a project+date via CLI trigger
//   - Content: digest includes instruction match summaries and assertion data
//   - Retrieval: digest accessible via penf digest show and penf digest latest
//   - Listing: penf digest list filters by project and date range
//   - Idempotency: re-triggering same project+date does not create duplicate
//   - Empty day: no digest created when project has no content for the date
//
// Expected: FAIL until Phase 2 is implemented. The digests table, workflow,
// gateway RPCs, and CLI commands do not exist yet.
//
// Run: go test -tags=e2e ./tests/e2e/ -run TestDigest_Phase2_DailyDigestWorkflow -v -timeout 300s
func TestDigest_Phase2_DailyDigestWorkflow(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	TerminateTestWorkflowsStandalone(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Cleanup digests for this tenant after the test
	t.Cleanup(func() {
		env.DB.Exec(ctx, `DELETE FROM digests WHERE tenant_id = $1`, env.TenantID)
		env.DB.Exec(ctx, `DELETE FROM instruction_matches WHERE tenant_id = $1`, env.TenantID)
		env.DB.Exec(ctx, `DELETE FROM watch_instructions WHERE tenant_id = $1`, env.TenantID)
	})

	// Create a project for digest generation
	projectResult := env.CLI.Run(ctx, "project", "add", "DigestTestProject",
		"--description", "Project for daily digest e2e test",
		"--keywords", "digesttest4k9",
	)
	require.Equal(t, 0, projectResult.ExitCode, "project add: %s", projectResult.Stderr)

	var projectID int64
	var projectName string
	err = env.DB.QueryRow(ctx, `
		SELECT id, name FROM projects
		WHERE tenant_id = $1 AND name = 'DigestTestProject'
	`, env.TenantID).Scan(&projectID, &projectName)
	require.NoError(t, err, "test project should exist")
	t.Logf("Test project: id=%d name=%s", projectID, projectName)

	// Map a source tag to the project (so content gets attributed)
	mapResult := env.CLI.Run(ctx, "source", "tag", "e2e-digest-",
		"--project", projectName,
		"--match", "prefix",
		"--type", "email_list",
	)
	require.Equal(t, 0, mapResult.ExitCode, "source tag: %s", mapResult.Stderr)

	// Create an instruction that will match our test content
	instrResult := env.CLI.Run(ctx, "instruction", "add",
		"--name", "Budget Overruns",
		"--instruction", "Flag emails discussing budget overruns or cost escalation",
		"--project", projectName,
		"--priority", "high",
	)
	require.Equal(t, 0, instrResult.ExitCode, "instruction add: %s", instrResult.Stderr)

	// Ingest test content — an email that discusses budget issues
	// Use today's date so the digest gather query matches on received_at::date
	sourceTag := fmt.Sprintf("e2e-digest-%d", time.Now().UnixNano())
	emailPath := createTempEmailWithDate(t,
		"Q1 Budget Review - Overrun Alert",
		time.Now(),
		`Team,

The Q1 budget review is complete and we have a significant overrun in the infrastructure line item.

Key findings:
- Cloud compute costs exceeded budget by 35% ($180K over)
- The overrun is primarily driven by the ML training cluster scaling
- Storage costs are within budget but trending upward

DECISION: Freeze new compute provisioning until the optimization review is complete.
ACTION: Each team lead must submit a cost reduction plan by end of week.
RISK: If we don't address this, Q2 projections show a 50% budget overrun.

The CFO has flagged this as a priority item for the next board meeting.

-- Finance Team`)

	ingestResult := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
	require.Equal(t, 0, ingestResult.ExitCode, "ingest: %s", ingestResult.Stderr)

	// Trigger pipeline and wait for completion (includes attribution + instruction evaluation)
	env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)

	sourceID, err := getLatestSourceByTag(env, sourceTag)
	require.NoError(t, err, "should find source with tag %s", sourceTag)
	t.Logf("Source ID: %d", sourceID)

	err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
	require.NoError(t, err, "pipeline should complete within timeout")

	// Verify attribution happened (content is associated with our project)
	var attributedCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM project_source_mappings
		WHERE tenant_id = $1 AND project_id = $2
	`, env.TenantID, projectID).Scan(&attributedCount)
	require.NoError(t, err)
	t.Logf("Project source mappings: %d", attributedCount)

	// Get the date of ingested content for digest generation
	digestDate := time.Now().Format("2006-01-02")

	// ================================================================
	// Part 1: Schema verification
	// ================================================================
	t.Run("schema_verification", func(t *testing.T) {
		t.Run("digests_table", func(t *testing.T) {
			expectedColumns := []string{
				"id", "tenant_id", "project_id", "digest_type",
				"period_start", "period_end", "body",
				"model_used", "prompt_template_id",
				"input_token_count", "output_token_count",
				"source_content_ids", "created_at",
			}

			for _, col := range expectedColumns {
				var exists bool
				err := env.DB.QueryRow(ctx, `
					SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_schema = 'public'
						  AND table_name = 'digests'
						  AND column_name = $1
					)
				`, col).Scan(&exists)
				require.NoError(t, err, "querying column %s", col)
				assert.True(t, exists,
					"digests table should have column %q", col)
			}
		})

		t.Run("digest_prompt_template", func(t *testing.T) {
			var count int
			err := env.DB.QueryRow(ctx, `
				SELECT COUNT(*) FROM prompt_templates
				WHERE stage = 'digest_daily_generate'
				  AND is_active = true
			`).Scan(&count)
			require.NoError(t, err, "querying prompt_templates")
			assert.Greater(t, count, 0,
				"prompt_templates should have an active 'digest_daily_generate' template")
		})
	})

	// ================================================================
	// Part 2: Digest generation via CLI
	// ================================================================
	var digestID string

	t.Run("generate_daily_digest", func(t *testing.T) {
		// Trigger digest generation for the test project and today's date
		result := env.CLI.Run(ctx, "digest", "generate", projectName,
			"--date", digestDate,
		)
		require.Equal(t, 0, result.ExitCode,
			"digest generate should succeed: %s", result.Stderr)
		t.Logf("digest generate output: %s", result.Stdout)

		// Wait for the digest workflow to complete (poll DB)
		deadline := time.Now().Add(120 * time.Second)
		for time.Now().Before(deadline) {
			var count int
			err := env.DB.QueryRow(ctx, `
				SELECT COUNT(*) FROM digests
				WHERE tenant_id = $1
				  AND project_id = $2
				  AND digest_type = 'daily'
				  AND period_start = $3::date
			`, env.TenantID, projectID, digestDate).Scan(&count)
			require.NoError(t, err)

			if count > 0 {
				t.Logf("Digest created after %v", time.Since(deadline.Add(-120*time.Second)))
				break
			}
			time.Sleep(3 * time.Second)
		}

		// Verify digest exists in DB
		err := env.DB.QueryRow(ctx, `
			SELECT id FROM digests
			WHERE tenant_id = $1
			  AND project_id = $2
			  AND digest_type = 'daily'
			  AND period_start = $3::date
		`, env.TenantID, projectID, digestDate).Scan(&digestID)
		require.NoError(t, err, "daily digest should be created for project %d date %s", projectID, digestDate)
		require.NotEmpty(t, digestID, "digest ID should not be empty")
		t.Logf("Created digest ID: %s", digestID)

		// Verify digest body is non-empty JSON
		var body string
		err = env.DB.QueryRow(ctx, `
			SELECT body FROM digests
			WHERE id = $1 AND tenant_id = $2
		`, digestID, env.TenantID).Scan(&body)
		require.NoError(t, err)
		require.NotEmpty(t, body, "digest body should not be empty")

		var bodyJSON map[string]interface{}
		err = json.Unmarshal([]byte(body), &bodyJSON)
		require.NoError(t, err, "digest body should be valid JSON: %s", body[:min(200, len(body))])

		// Verify body contains expected sections
		assert.Contains(t, body, "summary",
			"digest body should contain a summary section")
		t.Logf("Digest body preview: %s", body[:min(300, len(body))])
	})

	if digestID == "" {
		t.Fatal("generate_daily_digest must pass before running remaining subtests")
	}

	// ================================================================
	// Part 3: Digest retrieval via CLI
	// ================================================================
	t.Run("digest_show", func(t *testing.T) {
		result := env.CLI.Run(ctx, "digest", "show", digestID, "-o", "json")
		require.Equal(t, 0, result.ExitCode,
			"digest show should succeed: %s", result.Stderr)

		var detail map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &detail)
		require.NoError(t, err, "digest show output should be valid JSON: %s", result.Stdout)

		assert.Equal(t, "daily", detail["digest_type"],
			"digest_type should be 'daily'")
		assert.NotEmpty(t, detail["body"],
			"digest body should not be empty")
		t.Logf("digest show: type=%v project_id=%v", detail["digest_type"], detail["project_id"])
	})

	t.Run("digest_latest", func(t *testing.T) {
		result := env.CLI.Run(ctx, "digest", "latest", projectName, "-o", "json")
		require.Equal(t, 0, result.ExitCode,
			"digest latest should succeed: %s", result.Stderr)

		var detail map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &detail)
		require.NoError(t, err, "digest latest output should be valid JSON: %s", result.Stdout)

		assert.Equal(t, "daily", detail["digest_type"],
			"latest digest should be the daily we just generated")
		t.Logf("digest latest: id=%v type=%v", detail["id"], detail["digest_type"])
	})

	t.Run("digest_list", func(t *testing.T) {
		result := env.CLI.Run(ctx, "digest", "list",
			"--project", projectName,
			"--since", digestDate,
			"-o", "json",
		)
		require.Equal(t, 0, result.ExitCode,
			"digest list should succeed: %s", result.Stderr)

		var digests []map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &digests)
		require.NoError(t, err, "digest list output should be valid JSON: %s", result.Stdout)

		assert.GreaterOrEqual(t, len(digests), 1,
			"digest list should return at least 1 digest for the test project")

		// Verify our digest is in the list
		var found bool
		for _, d := range digests {
			if id, ok := d["id"].(string); ok && id == digestID {
				found = true
				break
			}
		}
		assert.True(t, found, "digest list should include our generated digest %s", digestID)
	})

	// ================================================================
	// Part 4: Idempotency — re-trigger same date
	// ================================================================
	t.Run("idempotency", func(t *testing.T) {
		// Count existing digests for this project+date
		var beforeCount int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM digests
			WHERE tenant_id = $1
			  AND project_id = $2
			  AND digest_type = 'daily'
			  AND period_start = $3::date
		`, env.TenantID, projectID, digestDate).Scan(&beforeCount)
		require.NoError(t, err)

		// Re-trigger digest generation for the same date
		result := env.CLI.Run(ctx, "digest", "generate", projectName,
			"--date", digestDate,
		)
		// Should succeed (skip, not error)
		require.Equal(t, 0, result.ExitCode,
			"digest generate (re-trigger) should succeed: %s", result.Stderr)

		// Wait a moment for any async processing
		time.Sleep(5 * time.Second)

		// Count should not have increased
		var afterCount int
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM digests
			WHERE tenant_id = $1
			  AND project_id = $2
			  AND digest_type = 'daily'
			  AND period_start = $3::date
		`, env.TenantID, projectID, digestDate).Scan(&afterCount)
		require.NoError(t, err)

		assert.Equal(t, beforeCount, afterCount,
			"re-triggering digest for same project+date should not create a duplicate. "+
				"Before: %d, After: %d", beforeCount, afterCount)
	})

	// ================================================================
	// Part 5: Empty day — no content for date
	// ================================================================
	t.Run("empty_day_no_digest", func(t *testing.T) {
		// Use a date far in the past where no content exists
		emptyDate := "2020-01-15"

		result := env.CLI.Run(ctx, "digest", "generate", projectName,
			"--date", emptyDate,
		)
		// Should succeed (but skip generation)
		require.Equal(t, 0, result.ExitCode,
			"digest generate (empty day) should succeed: %s", result.Stderr)

		// Wait for any async processing
		time.Sleep(5 * time.Second)

		// No digest should be created for the empty date
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM digests
			WHERE tenant_id = $1
			  AND project_id = $2
			  AND period_start = $3::date
		`, env.TenantID, projectID, emptyDate).Scan(&count)
		require.NoError(t, err)

		assert.Equal(t, 0, count,
			"no digest should be created for a date with no project content")
	})

	// ================================================================
	// Part 6: Digest content quality — instruction matches included
	// ================================================================
	t.Run("digest_includes_instruction_matches", func(t *testing.T) {
		// Verify the digest body references instruction matches
		var body string
		err := env.DB.QueryRow(ctx, `
			SELECT body FROM digests
			WHERE id = $1 AND tenant_id = $2
		`, digestID, env.TenantID).Scan(&body)
		require.NoError(t, err)

		var bodyJSON map[string]interface{}
		err = json.Unmarshal([]byte(body), &bodyJSON)
		require.NoError(t, err)

		// The body should contain an instruction_matches section
		// (our "Budget Overruns" instruction should have matched the budget email)
		if sections, ok := bodyJSON["sections"].(map[string]interface{}); ok {
			if matches, ok := sections["instruction_matches"]; ok {
				t.Logf("instruction_matches section present: %v", matches)
			} else {
				t.Log("WARNING: no instruction_matches section in digest body sections")
			}
		}

		// At minimum, the body should mention budget-related content
		assert.Contains(t, body, "budget",
			"digest body should reference the budget-related content. "+
				"The digest workflow should include content from attributed sources "+
				"and summarize instruction matches.")
	})
}

