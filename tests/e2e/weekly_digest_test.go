//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDigest_Phase3_WeeklyRollupWorkflow is the acceptance test for
// Epic 5 Phase 3 — Weekly Rollup + Theme Context.
//
// It verifies:
//   - Prompt: digest_weekly_synthesize prompt template is active
//   - Prerequisite: daily digests exist for the project (seeded via DB insert)
//   - Generation: weekly rollup generated from daily digests via CLI trigger
//   - Content: rollup body references daily digest content and trends
//   - Theme: project-scoped topics have running_context updated after rollup
//   - Retrieval: weekly digest accessible via penf digest show and penf digest latest --type weekly
//   - Listing: penf digest list --type weekly filters correctly
//   - Idempotency: re-triggering same project+week does not create duplicate
//   - Empty week: no rollup created when project has no daily digests for the week
//
// Expected: FAIL until Phase 3 is implemented.
//
// Run: go test -tags=e2e ./tests/e2e/ -run TestDigest_Phase3_WeeklyRollupWorkflow -v -timeout 300s
func TestDigest_Phase3_WeeklyRollupWorkflow(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	TerminateTestWorkflowsStandalone(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	t.Cleanup(func() {
		env.DB.Exec(ctx, `DELETE FROM digests WHERE tenant_id = $1`, env.TenantID)
		env.DB.Exec(ctx, `DELETE FROM topics WHERE tenant_id = $1`, env.TenantID)
	})

	// Create a project for weekly rollup
	projectResult := env.CLI.Run(ctx, "project", "add", "WeeklyRollupProject",
		"--description", "Project for weekly rollup e2e test",
		"--keywords", "weeklyrolluptest7x2",
	)
	require.Equal(t, 0, projectResult.ExitCode, "project add: %s", projectResult.Stderr)

	var projectID int64
	var projectName string
	err = env.DB.QueryRow(ctx, `
		SELECT id, name FROM projects
		WHERE tenant_id = $1 AND name = 'WeeklyRollupProject'
	`, env.TenantID).Scan(&projectID, &projectName)
	require.NoError(t, err, "test project should exist")
	t.Logf("Test project: id=%d name=%s", projectID, projectName)

	// Create a project-scoped topic (theme) for context updates
	_, err = env.DB.Exec(ctx, `
		INSERT INTO topics (tenant_id, name, description, project_id, status, keywords)
		VALUES ($1, 'Budget Pressure', 'Tracking budget overrun concerns', $2, 'active', ARRAY['budget', 'overrun', 'cost'])
	`, env.TenantID, projectID)
	require.NoError(t, err, "should create test topic")

	// Seed daily digests for the past week (direct DB insert to avoid
	// running full daily digest workflows, which are already tested in Phase 2)
	weekStart := mondayOfWeek(time.Now())
	dailyBodies := []string{
		`{"summary":"Monday: Team discussed Q1 budget review. Infrastructure costs exceeded budget by 35%.","sections":{"instruction_matches":[]}}`,
		`{"summary":"Tuesday: Cost reduction proposals submitted. ML cluster identified as primary driver.","sections":{"instruction_matches":[]}}`,
		`{"summary":"Wednesday: CFO meeting scheduled for Friday. Storage costs trending upward.","sections":{"instruction_matches":[]}}`,
		`{"summary":"Thursday: Optimization review started. Freeze on new compute provisioning in effect.","sections":{"instruction_matches":[]}}`,
		`{"summary":"Friday: Board presentation prepared. Q2 projections show risk of 50% overrun if unaddressed.","sections":{"instruction_matches":[]}}`,
	}

	var promptTemplateID int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM prompt_templates
		WHERE stage = 'digest_daily_generate' AND is_active = true
		LIMIT 1
	`).Scan(&promptTemplateID)
	require.NoError(t, err, "should find active daily prompt template")

	for i, body := range dailyBodies {
		day := weekStart.AddDate(0, 0, i)
		_, err = env.DB.Exec(ctx, `
			INSERT INTO digests (tenant_id, project_id, digest_type, period_start, period_end,
				body, model_used, prompt_template_id, input_token_count, output_token_count, source_content_ids)
			VALUES ($1, $2, 'daily', $3, $3, $4::jsonb, 'test-model', $5, 100, 50, '[]'::jsonb)
		`, env.TenantID, projectID, day, body, promptTemplateID)
		require.NoError(t, err, "should seed daily digest for day %d", i)
	}

	t.Logf("Seeded %d daily digests for week starting %s", len(dailyBodies), weekStart.Format("2006-01-02"))

	// Use the Monday of the week as the trigger date
	weekDate := weekStart.Format("2006-01-02")

	// ================================================================
	// Part 1: Prompt template verification
	// ================================================================
	t.Run("prompt_template", func(t *testing.T) {
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM prompt_templates
			WHERE stage = 'digest_weekly_synthesize'
			  AND is_active = true
		`).Scan(&count)
		require.NoError(t, err, "querying prompt_templates")
		assert.Greater(t, count, 0,
			"prompt_templates should have an active 'digest_weekly_synthesize' template")
	})

	// ================================================================
	// Part 2: Weekly rollup generation via CLI
	// ================================================================
	var weeklyDigestID string

	t.Run("generate_weekly_rollup", func(t *testing.T) {
		result := env.CLI.Run(ctx, "digest", "generate", projectName,
			"--date", weekDate,
			"--type", "weekly",
		)
		require.Equal(t, 0, result.ExitCode,
			"digest generate --type weekly should succeed: %s", result.Stderr)
		t.Logf("weekly digest generate output: %s", result.Stdout)

		// Wait for the weekly digest workflow to complete
		digestFound := false
		deadline := time.Now().Add(120 * time.Second)
		for time.Now().Before(deadline) {
			var count int
			err := env.DB.QueryRow(ctx, `
				SELECT COUNT(*) FROM digests
				WHERE tenant_id = $1
				  AND project_id = $2
				  AND digest_type = 'weekly'
				  AND period_start = $3::date
			`, env.TenantID, projectID, weekDate).Scan(&count)
			require.NoError(t, err)

			if count > 0 {
				t.Logf("Weekly digest created after %v", time.Since(deadline.Add(-120*time.Second)))
				digestFound = true
				break
			}
			time.Sleep(3 * time.Second)
		}
		require.True(t, digestFound, "timed out waiting for weekly digest generation (120s)")

		// Verify digest exists
		err := env.DB.QueryRow(ctx, `
			SELECT id FROM digests
			WHERE tenant_id = $1
			  AND project_id = $2
			  AND digest_type = 'weekly'
			  AND period_start = $3::date
		`, env.TenantID, projectID, weekDate).Scan(&weeklyDigestID)
		require.NoError(t, err, "weekly digest should be created")
		require.NotEmpty(t, weeklyDigestID, "weekly digest ID should not be empty")
		t.Logf("Created weekly digest ID: %s", weeklyDigestID)

		// Verify body is valid JSON with expected structure
		var body string
		err = env.DB.QueryRow(ctx, `
			SELECT body FROM digests WHERE id = $1 AND tenant_id = $2
		`, weeklyDigestID, env.TenantID).Scan(&body)
		require.NoError(t, err)
		require.NotEmpty(t, body)

		var bodyJSON map[string]interface{}
		err = json.Unmarshal([]byte(body), &bodyJSON)
		require.NoError(t, err, "weekly digest body should be valid JSON: %s", body[:min(200, len(body))])

		assert.Contains(t, body, "summary",
			"weekly digest body should contain a summary section")
		t.Logf("Weekly digest body preview: %s", body[:min(300, len(body))])

		// Verify period_end covers the week
		var periodEnd time.Time
		err = env.DB.QueryRow(ctx, `
			SELECT period_end FROM digests WHERE id = $1 AND tenant_id = $2
		`, weeklyDigestID, env.TenantID).Scan(&periodEnd)
		require.NoError(t, err)
		expectedEnd := weekStart.AddDate(0, 0, 6)
		assert.Equal(t, expectedEnd.Format("2006-01-02"), periodEnd.Format("2006-01-02"),
			"period_end should be Sunday of the week")
	})

	if weeklyDigestID == "" {
		t.Fatal("generate_weekly_rollup must pass before running remaining subtests")
	}

	// ================================================================
	// Part 3: Theme context update verification
	// ================================================================
	t.Run("theme_context_updated", func(t *testing.T) {
		var runningContext *string
		var lastUpdated *time.Time
		err := env.DB.QueryRow(ctx, `
			SELECT running_context, last_updated_at FROM topics
			WHERE tenant_id = $1 AND project_id = $2 AND name = 'Budget Pressure'
		`, env.TenantID, projectID).Scan(&runningContext, &lastUpdated)
		require.NoError(t, err, "should find project topic")

		assert.NotNil(t, runningContext,
			"topic running_context should be updated after weekly rollup")
		if runningContext != nil {
			assert.NotEmpty(t, *runningContext,
				"topic running_context should not be empty")
			assert.Contains(t, *runningContext, "budget",
				"running_context should reference budget theme from digest content")
			t.Logf("Topic running_context preview: %s", (*runningContext)[:min(200, len(*runningContext))])
		}

		assert.NotNil(t, lastUpdated,
			"topic last_updated_at should be set after context update")
	})

	// ================================================================
	// Part 4: Retrieval via CLI
	// ================================================================
	t.Run("digest_show_weekly", func(t *testing.T) {
		result := env.CLI.Run(ctx, "digest", "show", weeklyDigestID, "-o", "json")
		require.Equal(t, 0, result.ExitCode,
			"digest show should succeed: %s", result.Stderr)

		var detail map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &detail)
		require.NoError(t, err, "digest show output should be valid JSON: %s", result.Stdout)

		assert.Equal(t, "weekly", detail["digest_type"],
			"digest_type should be 'weekly'")
		t.Logf("digest show weekly: type=%v project_id=%v", detail["digest_type"], detail["project_id"])
	})

	t.Run("digest_latest_weekly", func(t *testing.T) {
		result := env.CLI.Run(ctx, "digest", "latest", projectName,
			"--type", "weekly",
			"-o", "json",
		)
		require.Equal(t, 0, result.ExitCode,
			"digest latest --type weekly should succeed: %s", result.Stderr)

		var detail map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &detail)
		require.NoError(t, err, "digest latest output should be valid JSON: %s", result.Stdout)

		assert.Equal(t, "weekly", detail["digest_type"],
			"latest weekly digest should be the one we just generated")
	})

	t.Run("digest_list_weekly", func(t *testing.T) {
		result := env.CLI.Run(ctx, "digest", "list",
			"--project", projectName,
			"--type", "weekly",
			"-o", "json",
		)
		require.Equal(t, 0, result.ExitCode,
			"digest list --type weekly should succeed: %s", result.Stderr)

		var digests []map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &digests)
		require.NoError(t, err, "digest list output should be valid JSON: %s", result.Stdout)

		assert.GreaterOrEqual(t, len(digests), 1,
			"digest list should return at least 1 weekly digest")

		var found bool
		for _, d := range digests {
			if id, ok := d["id"].(string); ok && id == weeklyDigestID {
				found = true
				break
			}
		}
		assert.True(t, found, "digest list should include our generated weekly digest %s", weeklyDigestID)
	})

	// ================================================================
	// Part 5: Idempotency — re-trigger same week
	// ================================================================
	t.Run("idempotency", func(t *testing.T) {
		var beforeCount int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM digests
			WHERE tenant_id = $1
			  AND project_id = $2
			  AND digest_type = 'weekly'
			  AND period_start = $3::date
		`, env.TenantID, projectID, weekDate).Scan(&beforeCount)
		require.NoError(t, err)

		result := env.CLI.Run(ctx, "digest", "generate", projectName,
			"--date", weekDate,
			"--type", "weekly",
		)
		require.Equal(t, 0, result.ExitCode,
			"digest generate weekly (re-trigger) should succeed: %s", result.Stderr)

		time.Sleep(5 * time.Second)

		var afterCount int
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM digests
			WHERE tenant_id = $1
			  AND project_id = $2
			  AND digest_type = 'weekly'
			  AND period_start = $3::date
		`, env.TenantID, projectID, weekDate).Scan(&afterCount)
		require.NoError(t, err)

		assert.Equal(t, beforeCount, afterCount,
			"re-triggering weekly digest for same week should not create duplicate. "+
				"Before: %d, After: %d", beforeCount, afterCount)
	})

	// ================================================================
	// Part 6: Empty week — no daily digests for date range
	// ================================================================
	t.Run("empty_week_no_rollup", func(t *testing.T) {
		emptyWeek := "2020-01-06" // A Monday in the past

		result := env.CLI.Run(ctx, "digest", "generate", projectName,
			"--date", emptyWeek,
			"--type", "weekly",
		)
		require.Equal(t, 0, result.ExitCode,
			"digest generate weekly (empty week) should succeed: %s", result.Stderr)

		time.Sleep(5 * time.Second)

		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM digests
			WHERE tenant_id = $1
			  AND project_id = $2
			  AND digest_type = 'weekly'
			  AND period_start = $3::date
		`, env.TenantID, projectID, emptyWeek).Scan(&count)
		require.NoError(t, err)

		assert.Equal(t, 0, count,
			"no weekly digest should be created for a week with no daily digests")
	})
}

// mondayOfWeek returns the Monday of the current ISO week.
func mondayOfWeek(t time.Time) time.Time {
	weekday := t.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	monday := t.AddDate(0, 0, -int(weekday-time.Monday))
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}
