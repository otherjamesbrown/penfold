//go:build e2e

// Phase 1 Acceptance Test: Scheduling Infrastructure (Epic 0)
//
// This test defines "done" for Phase 1 of the Scheduling Infrastructure epic.
// It exercises the new ScheduleService gRPC RPCs, verifies the schedules table
// in Postgres, and confirms Temporal schedules are created/updated/deleted in sync.
//
// Phase 1 scope:
//   - schedules table + migration
//   - Gateway gRPC: CreateSchedule, UpdateSchedule, DeleteSchedule, ListSchedules,
//     PauseSchedule, ResumeSchedule, GetScheduleHistory
//   - Temporal ScheduleClient wrapper
//   - Reconciliation on worker startup
//   - Migration of 2 existing hardcoded schedules into the DB
//   - Schedule execution callback (last_run_at, last_status, last_error, next_run_at)
//
// Phase 2 (CLI commands) is separate and not tested here.
//
// NOTE: This test is expected to FAIL until Phase 1 is implemented.
// The proto, RPCs, and DB table do not exist yet. Mycroft uses this as the
// acceptance gate — when this test passes, Phase 1 is done.

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"testing"
	"time"

	schedulev1 "github.com/otherjamesbrown/penfold/api/proto/schedule/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// TestE2E_SchedulingInfra_Phase1 is the acceptance test for the Scheduling
// Infrastructure Phase 1. It verifies gRPC RPCs, Postgres state, and Temporal
// schedule registration for all CRUD + pause/resume operations.
func TestE2E_SchedulingInfra_Phase1(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Ensure test tenant exists
	err := env.EnsureTenantExists()
	require.NoError(t, err, "tenant must exist before schedule tests")

	// --- gRPC client setup ---
	gatewayAddr := os.Getenv("GATEWAY_GRPC_ADDR")
	if gatewayAddr == "" {
		gatewayAddr = "dev02.brown.chat:50051"
	}
	conn, err := grpc.NewClient(gatewayAddr, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})))
	require.NoError(t, err, "failed to connect to gateway gRPC")
	t.Cleanup(func() { conn.Close() })

	scheduleClient := schedulev1.NewScheduleServiceClient(conn)

	// --- Cleanup: delete any leftover e2e-test-* schedules ---
	t.Cleanup(func() {
		cleanupTestSchedules(t, env, scheduleClient)
	})

	// Track the schedule ID created in the first subtest so subsequent subtests
	// can reference it.
	var createdScheduleID string

	// =====================================================================
	// Subtest: create_schedule
	// =====================================================================
	t.Run("create_schedule", func(t *testing.T) {
		resp, err := scheduleClient.CreateSchedule(ctx, &schedulev1.CreateScheduleRequest{
			TenantId:      env.TenantID,
			Name:          "e2e-test-cron",
			Type:          "cron",
			CronExpr:      "*/5 * * * *",
			WorkflowType:  "ConversationMaintenanceWorkflow",
			WorkflowParams: `{"tenant_id":"` + env.TenantID + `","stale_days":14,"limit":100}`,
			OverlapPolicy: "skip",
		})
		require.NoError(t, err, "CreateSchedule RPC should succeed")
		require.NotEmpty(t, resp.GetScheduleId(), "response must include a schedule_id")

		createdScheduleID = resp.GetScheduleId()
		t.Logf("Created schedule: id=%s", createdScheduleID)

		// Verify DB row
		// Column names match the epic spec schema: schedule_type, schedule_expr, etc.
		var dbName, dbScheduleType, dbScheduleExpr, dbWorkflowType, dbOverlapPolicy string
		var dbEnabled bool
		err = env.DB.QueryRow(ctx, `
			SELECT name, schedule_type, schedule_expr, workflow_type, overlap_policy, enabled
			FROM schedules
			WHERE id = $1 AND tenant_id = $2
		`, createdScheduleID, env.TenantID).Scan(
			&dbName, &dbScheduleType, &dbScheduleExpr, &dbWorkflowType, &dbOverlapPolicy, &dbEnabled,
		)
		require.NoError(t, err, "schedule row must exist in DB")
		assert.Equal(t, "e2e-test-cron", dbName)
		assert.Equal(t, "cron", dbScheduleType)
		assert.Equal(t, "*/5 * * * *", dbScheduleExpr)
		assert.Equal(t, "ConversationMaintenanceWorkflow", dbWorkflowType)
		assert.Equal(t, "skip", dbOverlapPolicy)
		assert.True(t, dbEnabled, "new schedule should be enabled by default")

		// Verify Temporal schedule was registered
		schedHandle := env.TemporalClient.ScheduleClient().GetHandle(ctx, createdScheduleID)
		desc, err := schedHandle.Describe(ctx)
		require.NoError(t, err, "Temporal schedule must be discoverable via GetHandle+Describe")
		require.NotNil(t, desc, "schedule description must not be nil")
		t.Logf("Temporal schedule confirmed: id=%s", createdScheduleID)
	})

	// Guard: remaining subtests depend on a successfully created schedule.
	if createdScheduleID == "" {
		t.Fatal("create_schedule must pass before running remaining subtests")
	}

	// =====================================================================
	// Subtest: list_schedules
	// =====================================================================
	t.Run("list_schedules", func(t *testing.T) {
		resp, err := scheduleClient.ListSchedules(ctx, &schedulev1.ListSchedulesRequest{
			TenantId: env.TenantID,
		})
		require.NoError(t, err, "ListSchedules RPC should succeed")
		require.NotNil(t, resp)

		// Find our schedule in the list
		var found *schedulev1.ScheduleSummary
		for _, s := range resp.GetSchedules() {
			if s.GetName() == "e2e-test-cron" {
				found = s
				break
			}
		}

		require.NotNil(t, found, "e2e-test-cron must appear in ListSchedules response")
		assert.Equal(t, "cron", found.GetType())
		assert.True(t, found.GetEnabled(), "schedule should be enabled")
		assert.Equal(t, "*/5 * * * *", found.GetCronExpr())
		t.Logf("ListSchedules returned %d schedule(s); e2e-test-cron found", len(resp.GetSchedules()))
	})

	// =====================================================================
	// Subtest: pause_resume_schedule
	// =====================================================================
	t.Run("pause_resume_schedule", func(t *testing.T) {
		// --- Pause ---
		_, err := scheduleClient.PauseSchedule(ctx, &schedulev1.PauseScheduleRequest{
			TenantId:   env.TenantID,
			ScheduleId: createdScheduleID,
		})
		require.NoError(t, err, "PauseSchedule RPC should succeed")

		// Verify DB shows disabled
		var dbEnabled bool
		err = env.DB.QueryRow(ctx, `
			SELECT enabled FROM schedules
			WHERE id = $1 AND tenant_id = $2
		`, createdScheduleID, env.TenantID).Scan(&dbEnabled)
		require.NoError(t, err)
		assert.False(t, dbEnabled, "schedule should be disabled after PauseSchedule")

		// Verify Temporal schedule is paused
		handle := env.TemporalClient.ScheduleClient().GetHandle(ctx, createdScheduleID)
		desc, err := handle.Describe(ctx)
		require.NoError(t, err, "Temporal Describe should succeed after pause")
		assert.True(t, desc.Schedule.State.Paused, "Temporal schedule should be paused")
		t.Log("Schedule paused successfully")

		// --- Resume ---
		_, err = scheduleClient.ResumeSchedule(ctx, &schedulev1.ResumeScheduleRequest{
			TenantId:   env.TenantID,
			ScheduleId: createdScheduleID,
		})
		require.NoError(t, err, "ResumeSchedule RPC should succeed")

		// Verify DB shows enabled again
		err = env.DB.QueryRow(ctx, `
			SELECT enabled FROM schedules
			WHERE id = $1 AND tenant_id = $2
		`, createdScheduleID, env.TenantID).Scan(&dbEnabled)
		require.NoError(t, err)
		assert.True(t, dbEnabled, "schedule should be enabled after ResumeSchedule")

		// Verify Temporal schedule is unpaused
		desc, err = handle.Describe(ctx)
		require.NoError(t, err, "Temporal Describe should succeed after resume")
		assert.False(t, desc.Schedule.State.Paused, "Temporal schedule should be unpaused")
		t.Log("Schedule resumed successfully")
	})

	// =====================================================================
	// Subtest: trigger_schedule
	// =====================================================================
	t.Run("trigger_schedule", func(t *testing.T) {
		// Trigger the schedule — should succeed since it's enabled
		_, err := scheduleClient.TriggerSchedule(ctx, &schedulev1.TriggerScheduleRequest{
			TenantId:   env.TenantID,
			ScheduleId: createdScheduleID,
		})
		require.NoError(t, err, "TriggerSchedule RPC should succeed for enabled schedule")
		t.Log("Schedule triggered successfully")

		// Trigger a non-existent schedule — should return NotFound
		_, err = scheduleClient.TriggerSchedule(ctx, &schedulev1.TriggerScheduleRequest{
			TenantId:   env.TenantID,
			ScheduleId: "non-existent-schedule-id",
		})
		require.Error(t, err, "TriggerSchedule should fail for non-existent schedule")
		assert.Contains(t, err.Error(), "not found", "error should indicate schedule not found")

		// Pause the schedule, then try to trigger — should return FailedPrecondition
		_, err = scheduleClient.PauseSchedule(ctx, &schedulev1.PauseScheduleRequest{
			TenantId:   env.TenantID,
			ScheduleId: createdScheduleID,
		})
		require.NoError(t, err, "PauseSchedule should succeed")

		_, err = scheduleClient.TriggerSchedule(ctx, &schedulev1.TriggerScheduleRequest{
			TenantId:   env.TenantID,
			ScheduleId: createdScheduleID,
		})
		require.Error(t, err, "TriggerSchedule should fail for paused schedule")
		assert.Contains(t, err.Error(), "paused", "error should mention schedule is paused")

		// Resume so subsequent tests aren't affected
		_, err = scheduleClient.ResumeSchedule(ctx, &schedulev1.ResumeScheduleRequest{
			TenantId:   env.TenantID,
			ScheduleId: createdScheduleID,
		})
		require.NoError(t, err, "ResumeSchedule should succeed")
		t.Log("Schedule resumed after trigger-paused test")
	})

	// =====================================================================
	// Subtest: update_schedule
	// =====================================================================
	t.Run("update_schedule", func(t *testing.T) {
		_, err := scheduleClient.UpdateSchedule(ctx, &schedulev1.UpdateScheduleRequest{
			TenantId:   env.TenantID,
			ScheduleId: createdScheduleID,
			CronExpr:   "0 4 * * *",
		})
		require.NoError(t, err, "UpdateSchedule RPC should succeed")

		// Verify DB reflects new expression
		var dbScheduleExpr string
		err = env.DB.QueryRow(ctx, `
			SELECT schedule_expr FROM schedules
			WHERE id = $1 AND tenant_id = $2
		`, createdScheduleID, env.TenantID).Scan(&dbScheduleExpr)
		require.NoError(t, err)
		assert.Equal(t, "0 4 * * *", dbScheduleExpr, "DB schedule_expr should be updated")

		// Verify Temporal schedule spec updated
		handle := env.TemporalClient.ScheduleClient().GetHandle(ctx, createdScheduleID)
		desc, err := handle.Describe(ctx)
		require.NoError(t, err, "Temporal Describe should succeed after update")

		// The schedule spec should contain the new cron expression.
		// Note: Temporal server converts CronExpressions to Calendars
		// internally, so we verify via the calendar spec (hour=4 for "0 4 * * *").
		require.NotNil(t, desc.Schedule.Spec, "schedule spec must not be nil")
		require.NotEmpty(t, desc.Schedule.Spec.Calendars, "calendars must not be empty after update")
		require.NotEmpty(t, desc.Schedule.Spec.Calendars[0].Hour, "hour ranges must not be empty")
		assert.Equal(t, 4, desc.Schedule.Spec.Calendars[0].Hour[0].Start,
			"Temporal schedule hour should be updated to 4 (from cron 0 4 * * *)")
		t.Log("Schedule updated: cron_expr changed to 0 4 * * *")
	})

	// =====================================================================
	// Subtest: delete_schedule
	// =====================================================================
	t.Run("delete_schedule", func(t *testing.T) {
		_, err := scheduleClient.DeleteSchedule(ctx, &schedulev1.DeleteScheduleRequest{
			TenantId:   env.TenantID,
			ScheduleId: createdScheduleID,
		})
		require.NoError(t, err, "DeleteSchedule RPC should succeed")

		// Verify no active DB row exists (works for both hard and soft delete)
		var count int
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM schedules
			WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
		`, createdScheduleID, env.TenantID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "schedule should be deleted (or soft-deleted) in DB")

		// Verify Temporal schedule is gone
		handle := env.TemporalClient.ScheduleClient().GetHandle(ctx, createdScheduleID)
		_, err = handle.Describe(ctx)
		assert.Error(t, err, "Temporal schedule should no longer exist after DeleteSchedule")
		t.Logf("Schedule %s deleted from DB and Temporal", createdScheduleID)
	})

	// =====================================================================
	// Subtest: migrated_schedules_exist
	// =====================================================================
	t.Run("migrated_schedules_exist", func(t *testing.T) {
		// The two hardcoded schedules from services/worker/schedules.go should
		// have been migrated into the schedules table by the Phase 1 migration /
		// reconciliation logic.
		migratedSchedules := []struct {
			name         string
			cronExpr     string
			workflowType string
		}{
			{
				name:         "conversation-stale-check",
				cronExpr:     "0 3 * * *",
				workflowType: "ConversationMaintenanceWorkflow",
			},
			{
				name:         "ledger-consolidation",
				cronExpr:     "0 3 * * *",
				workflowType: "SessionLedgerConsolidationWorkflow",
			},
		}

		for _, expected := range migratedSchedules {
			t.Run(expected.name, func(t *testing.T) {
				var dbScheduleExpr, dbWorkflowType string
				var dbEnabled bool
				err := env.DB.QueryRow(ctx, `
					SELECT schedule_expr, workflow_type, enabled
					FROM schedules
					WHERE name = $1 AND tenant_id = $2
				`, expected.name, env.TenantID).Scan(&dbScheduleExpr, &dbWorkflowType, &dbEnabled)
				require.NoError(t, err, "migrated schedule %q must exist in schedules table", expected.name)

				assert.Equal(t, expected.cronExpr, dbScheduleExpr,
					"migrated schedule %q schedule_expr mismatch", expected.name)
				assert.Equal(t, expected.workflowType, dbWorkflowType,
					"migrated schedule %q workflow_type mismatch", expected.name)
				assert.True(t, dbEnabled,
					"migrated schedule %q should be enabled", expected.name)

				t.Logf("Migrated schedule verified: name=%s expr=%s workflow=%s enabled=%v",
					expected.name, dbScheduleExpr, dbWorkflowType, dbEnabled)
			})
		}
	})
}

// cleanupTestSchedules removes any schedules with the "e2e-test-" prefix from
// both the database and Temporal. This is called via t.Cleanup to ensure test
// isolation regardless of pass/fail.
func cleanupTestSchedules(t *testing.T, env *PipelineE2EEnv, client schedulev1.ScheduleServiceClient) {
	cleanupTestSchedulesWithPrefix(t, env, client, "e2e-test-")
}

// cleanupTestSchedulesWithPrefix removes schedules matching the given name prefix
// from both the database and Temporal. Used by schedule and heartbeat tests.
func cleanupTestSchedulesWithPrefix(t *testing.T, env *PipelineE2EEnv, client schedulev1.ScheduleServiceClient, namePrefix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find schedules matching the prefix in the DB
	rows, err := env.DB.Query(ctx, `
		SELECT id, name FROM schedules
		WHERE tenant_id = $1 AND name LIKE $2
	`, env.TenantID, namePrefix+"%")
	if err != nil {
		// Table may not exist yet; that is fine.
		t.Logf("cleanup: could not query schedules table (may not exist yet): %v", err)
		return
	}
	defer rows.Close()

	type schedRow struct {
		id   string
		name string
	}
	var toDelete []schedRow
	for rows.Next() {
		var r schedRow
		if err := rows.Scan(&r.id, &r.name); err != nil {
			t.Logf("cleanup: scan error: %v", err)
			continue
		}
		toDelete = append(toDelete, r)
	}

	for _, r := range toDelete {
		// Try gRPC delete first (handles both DB and Temporal)
		_, err := client.DeleteSchedule(ctx, &schedulev1.DeleteScheduleRequest{
			TenantId:   env.TenantID,
			ScheduleId: r.id,
		})
		if err != nil {
			t.Logf("cleanup: gRPC DeleteSchedule failed for %s (%s), falling back to direct cleanup: %v", r.name, r.id, err)

			// Fallback: delete from DB directly
			_, dbErr := env.DB.Exec(ctx, "DELETE FROM schedules WHERE id = $1 AND tenant_id = $2", r.id, env.TenantID)
			if dbErr != nil {
				t.Logf("cleanup: DB delete failed for %s: %v", r.name, dbErr)
			}

			// Fallback: delete from Temporal directly
			handle := env.TemporalClient.ScheduleClient().GetHandle(ctx, r.id)
			if tErr := handle.Delete(ctx); tErr != nil {
				t.Logf("cleanup: Temporal delete failed for %s: %v", r.name, tErr)
			}
		} else {
			t.Logf("cleanup: deleted test schedule %s (%s)", r.name, r.id)
		}
	}

	if len(toDelete) > 0 {
		t.Logf("cleanup: removed %d %s* schedule(s)", len(toDelete), namePrefix)
	}

	// Also attempt to clean up any Temporal schedules matching the prefix that
	// might exist without a DB row (e.g. if the DB insert failed mid-create).
	cleanupOrphanedTemporalSchedules(t, env, namePrefix)
}

// cleanupOrphanedTemporalSchedules attempts to find and delete Temporal schedules
// whose IDs start with the given prefix but may not have DB rows.
func cleanupOrphanedTemporalSchedules(t *testing.T, env *PipelineE2EEnv, prefix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// List all schedules in Temporal and delete those matching the prefix.
	// The Temporal SDK ScheduleClient does not have a List method that filters
	// by ID prefix, so we attempt known IDs from the test. If the schedule does
	// not exist the delete will fail silently.
	knownTestIDs := []string{
		"e2e-test-cron",
	}
	for _, id := range knownTestIDs {
		handle := env.TemporalClient.ScheduleClient().GetHandle(ctx, id)
		if err := handle.Delete(ctx); err == nil {
			t.Logf("cleanup: deleted orphaned Temporal schedule %s", id)
		}
	}

	// Also scan the DB for any schedule IDs starting with the prefix that were
	// assigned by the server (UUIDs) — those were already handled above.
	_ = fmt.Sprintf("prefix=%s", prefix) // prevent unused import if prefix logic is simplified
}
