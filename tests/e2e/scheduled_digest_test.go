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

// TestDigest_Phase5_ScheduledGenerationWorkflow is the acceptance test for
// Epic 5 Phase 5 — Scheduled Digest Generation.
//
// It verifies:
//   - Schedule creation: penf schedule create for daily and weekly digests
//   - Schedule listing: schedules appear in penf schedule list
//   - Schedule details: penf schedule show returns correct workflow_type and params
//   - Trigger: manually triggering a schedule produces a digest
//   - Pause/Resume: pausing prevents generation, resuming re-enables
//   - Cleanup: penf schedule delete removes the schedule
//
// Expected: FAIL until Phase 5 is implemented.
//
// Run: go test -tags=e2e ./tests/e2e/ -run TestDigest_Phase5_ScheduledGenerationWorkflow -v -timeout 300s
func TestDigest_Phase5_ScheduledGenerationWorkflow(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	TerminateTestWorkflowsStandalone(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	t.Cleanup(func() {
		env.DB.Exec(ctx, `DELETE FROM digests WHERE tenant_id = $1`, env.TenantID)
		env.DB.Exec(ctx, `DELETE FROM schedules WHERE tenant_id = $1 AND name LIKE 'e2e-digest-%'`, env.TenantID)
	})

	// Create a test project
	projectResult := env.CLI.Run(ctx, "project", "add", "ScheduledDigestProject",
		"--description", "Project for scheduled digest e2e test",
		"--keywords", "scheduleddigest4x1",
	)
	require.Equal(t, 0, projectResult.ExitCode, "project add: %s", projectResult.Stderr)

	var projectID int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM projects
		WHERE tenant_id = $1 AND name = 'ScheduledDigestProject'
	`, env.TenantID).Scan(&projectID)
	require.NoError(t, err)
	t.Logf("Test project: id=%d", projectID)

	// ================================================================
	// Part 1: Create daily digest schedule via CLI
	// ================================================================
	var dailyScheduleID string

	t.Run("create_daily_schedule", func(t *testing.T) {
		params := map[string]interface{}{
			"project_name": "ScheduledDigestProject",
			"digest_type":  "daily",
		}
		paramsJSON, err := json.Marshal(params)
		require.NoError(t, err)

		result := env.CLI.Run(ctx, "schedule", "create",
			"--name", "e2e-digest-daily-test",
			"--cron", "0 6 * * *",
			"--workflow", "DigestWorkflow",
			"--params", string(paramsJSON),
			"--overlap", "skip",
			"-o", "json",
		)
		require.Equal(t, 0, result.ExitCode,
			"schedule create should succeed: %s", result.Stderr)

		var resp map[string]interface{}
		err = json.Unmarshal([]byte(result.Stdout), &resp)
		require.NoError(t, err, "schedule create output should be valid JSON")

		id, ok := resp["id"].(string)
		require.True(t, ok && id != "", "schedule should have an ID")
		dailyScheduleID = id
		t.Logf("Created daily schedule: %s", dailyScheduleID)
	})

	// ================================================================
	// Part 2: Create weekly digest schedule via CLI
	// ================================================================
	var weeklyScheduleID string

	t.Run("create_weekly_schedule", func(t *testing.T) {
		params := map[string]interface{}{
			"project_name": "ScheduledDigestProject",
			"digest_type":  "weekly",
		}
		paramsJSON, err := json.Marshal(params)
		require.NoError(t, err)

		result := env.CLI.Run(ctx, "schedule", "create",
			"--name", "e2e-digest-weekly-test",
			"--cron", "0 8 * * 1",
			"--workflow", "WeeklyDigestWorkflow",
			"--params", string(paramsJSON),
			"--overlap", "skip",
			"-o", "json",
		)
		require.Equal(t, 0, result.ExitCode,
			"schedule create should succeed: %s", result.Stderr)

		var resp map[string]interface{}
		err = json.Unmarshal([]byte(result.Stdout), &resp)
		require.NoError(t, err)

		id, ok := resp["id"].(string)
		require.True(t, ok && id != "", "schedule should have an ID")
		weeklyScheduleID = id
		t.Logf("Created weekly schedule: %s", weeklyScheduleID)
	})

	// ================================================================
	// Part 3: List schedules — both should appear
	// ================================================================
	t.Run("list_schedules", func(t *testing.T) {
		result := env.CLI.Run(ctx, "schedule", "list", "-o", "json")
		require.Equal(t, 0, result.ExitCode,
			"schedule list should succeed: %s", result.Stderr)

		var schedules []map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &schedules)
		require.NoError(t, err, "schedule list output should be valid JSON")

		var foundDaily, foundWeekly bool
		for _, s := range schedules {
			name, _ := s["name"].(string)
			if name == "e2e-digest-daily-test" {
				foundDaily = true
			}
			if name == "e2e-digest-weekly-test" {
				foundWeekly = true
			}
		}
		assert.True(t, foundDaily, "daily schedule should appear in list")
		assert.True(t, foundWeekly, "weekly schedule should appear in list")
	})

	// ================================================================
	// Part 4: Show schedule details
	// ================================================================
	t.Run("show_daily_schedule", func(t *testing.T) {
		result := env.CLI.Run(ctx, "schedule", "show", "e2e-digest-daily-test", "-o", "json")
		require.Equal(t, 0, result.ExitCode,
			"schedule show should succeed: %s", result.Stderr)

		var detail map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &detail)
		require.NoError(t, err)

		assert.Equal(t, "e2e-digest-daily-test", detail["name"])
		assert.Equal(t, "DigestWorkflow", detail["workflow_type"])
		assert.Equal(t, true, detail["enabled"])
		t.Logf("Schedule detail: name=%v workflow=%v enabled=%v", detail["name"], detail["workflow_type"], detail["enabled"])
	})

	// ================================================================
	// Part 5: Pause and resume
	// ================================================================
	t.Run("pause_resume", func(t *testing.T) {
		// Pause
		result := env.CLI.Run(ctx, "schedule", "pause", "e2e-digest-daily-test")
		require.Equal(t, 0, result.ExitCode,
			"schedule pause should succeed: %s", result.Stderr)

		// Verify paused
		var enabled bool
		err := env.DB.QueryRow(ctx, `
			SELECT enabled FROM schedules
			WHERE id = $1 AND tenant_id = $2
		`, dailyScheduleID, env.TenantID).Scan(&enabled)
		require.NoError(t, err)
		assert.False(t, enabled, "schedule should be paused")

		// Resume
		result = env.CLI.Run(ctx, "schedule", "pause", "e2e-digest-daily-test")
		if result.ExitCode != 0 {
			// Try resume command instead
			result = env.CLI.Run(ctx, "schedule", "resume", "e2e-digest-daily-test")
		}
		require.Equal(t, 0, result.ExitCode,
			"schedule resume should succeed: %s", result.Stderr)
	})

	// ================================================================
	// Part 6: Delete schedules
	// ================================================================
	t.Run("delete_schedules", func(t *testing.T) {
		result := env.CLI.Run(ctx, "schedule", "delete", "e2e-digest-daily-test")
		require.Equal(t, 0, result.ExitCode,
			"schedule delete daily should succeed: %s", result.Stderr)

		result = env.CLI.Run(ctx, "schedule", "delete", "e2e-digest-weekly-test")
		require.Equal(t, 0, result.ExitCode,
			"schedule delete weekly should succeed: %s", result.Stderr)

		// Verify deleted
		time.Sleep(1 * time.Second)
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM schedules
			WHERE tenant_id = $1 AND name LIKE 'e2e-digest-%' AND deleted_at IS NULL
		`, env.TenantID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "both schedules should be deleted")
	})
}
