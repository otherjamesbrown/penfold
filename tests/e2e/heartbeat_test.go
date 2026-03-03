//go:build e2e

// Phase 3 Acceptance Test: Heartbeat Job Type (Epic 0)
//
// This test defines "done" for Phase 3 of the Scheduling Infrastructure epic.
// It exercises the WorkflowHeartbeat Temporal workflow, verifies that configured
// checks run against real infrastructure (review queue, watchlist, stale content),
// and confirms results are stored and queryable via the schedule's status fields.
//
// Phase 3 scope:
//   - WorkflowHeartbeat Temporal workflow
//   - Heartbeat activities: review_queue, watch_matches, stale_content
//   - Results stored on the schedule row (last_status, last_error as JSON summary)
//   - Heartbeat schedule type works with Phase 1 infrastructure
//
// NOTE: This test is expected to FAIL until Phase 3 is implemented.

package e2e

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	schedulev1 "github.com/otherjamesbrown/penfold/api/proto/schedule/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func TestE2E_Heartbeat_Phase3(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	err := env.EnsureTenantExists()
	require.NoError(t, err)

	// gRPC client for schedule RPCs
	gatewayAddr := os.Getenv("GATEWAY_GRPC_ADDR")
	if gatewayAddr == "" {
		gatewayAddr = "dev02.brown.chat:50051"
	}
	conn, err := grpc.NewClient(gatewayAddr, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	schedClient := schedulev1.NewScheduleServiceClient(conn)

	// Cleanup heartbeat test schedules
	t.Cleanup(func() {
		cleanupHeartbeatTestSchedules(t, env, schedClient)
	})

	var heartbeatScheduleID string

	// =====================================================================
	// Subtest: create_heartbeat_schedule
	// =====================================================================
	t.Run("create_heartbeat_schedule", func(t *testing.T) {
		// Create a heartbeat-type schedule with configured checks
		checks := map[string]interface{}{
			"checks": []string{"review_queue", "watch_matches", "stale_content"},
		}
		checksJSON, err := json.Marshal(checks)
		require.NoError(t, err)

		resp, err := schedClient.CreateSchedule(ctx, &schedulev1.CreateScheduleRequest{
			TenantId:       env.TenantID,
			Name:           "e2e-test-heartbeat",
			Description:    "E2E test heartbeat — checks review queue, watch matches, stale content",
			Type:           "heartbeat",
			CronExpr:       "*/5 * * * *",
			WorkflowType:   "HeartbeatWorkflow",
			WorkflowParams: string(checksJSON),
			OverlapPolicy:  "skip",
		})
		require.NoError(t, err, "CreateSchedule for heartbeat should succeed")
		require.NotEmpty(t, resp.GetScheduleId())
		heartbeatScheduleID = resp.GetScheduleId()
		t.Logf("Created heartbeat schedule: id=%s", heartbeatScheduleID)

		// Verify DB row has schedule_type = 'heartbeat'
		var dbType string
		err = env.DB.QueryRow(ctx, `
			SELECT schedule_type FROM schedules
			WHERE id = $1 AND tenant_id = $2
		`, heartbeatScheduleID, env.TenantID).Scan(&dbType)
		require.NoError(t, err)
		assert.Equal(t, "heartbeat", dbType)
	})

	if heartbeatScheduleID == "" {
		t.Fatal("create_heartbeat_schedule must pass before remaining subtests")
	}

	// =====================================================================
	// Subtest: execute_heartbeat_workflow
	// =====================================================================
	t.Run("execute_heartbeat_workflow", func(t *testing.T) {
		// Trigger HeartbeatWorkflow directly via Temporal (simulating schedule execution)
		workflowID := fmt.Sprintf("e2e-heartbeat-%d", time.Now().UnixNano())
		input := map[string]interface{}{
			"tenant_id":   env.TenantID,
			"schedule_id": heartbeatScheduleID,
			"checks":      []string{"review_queue", "watch_matches", "stale_content"},
		}
		inputJSON, err := json.Marshal(input)
		require.NoError(t, err)

		opts := client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: "penfold-main",
		}

		run, err := env.TemporalClient.ExecuteWorkflow(ctx, opts, "HeartbeatWorkflow", json.RawMessage(inputJSON))
		require.NoError(t, err, "HeartbeatWorkflow should start successfully")
		t.Logf("Started HeartbeatWorkflow: workflowID=%s, runID=%s", run.GetID(), run.GetRunID())

		// Wait for workflow completion (should be fast — just DB queries)
		var result json.RawMessage
		err = run.Get(ctx, &result)
		require.NoError(t, err, "HeartbeatWorkflow should complete successfully")
		t.Logf("HeartbeatWorkflow result: %s", string(result))

		// Parse result — should have findings per check
		var findings map[string]interface{}
		err = json.Unmarshal(result, &findings)
		require.NoError(t, err, "result should be valid JSON")

		// Each configured check should have a result entry
		assert.Contains(t, findings, "review_queue", "findings should include review_queue check")
		assert.Contains(t, findings, "watch_matches", "findings should include watch_matches check")
		assert.Contains(t, findings, "stale_content", "findings should include stale_content check")

		// Overall status should be present
		assert.Contains(t, findings, "status", "findings should include overall status")
		status := findings["status"].(string)
		assert.Contains(t, []string{"ok", "actionable", "skipped"}, status,
			"status should be ok, actionable, or skipped")
		t.Logf("Heartbeat status: %s", status)
	})

	// =====================================================================
	// Subtest: heartbeat_updates_schedule_status
	// =====================================================================
	t.Run("heartbeat_updates_schedule_status", func(t *testing.T) {
		// After the heartbeat runs, the schedule row should be updated
		// The execution callback should set last_run_at, last_status, and
		// store findings summary in last_error (or a dedicated column).
		//
		// We trigger the heartbeat via the schedule's Temporal mechanism
		// and verify the DB is updated.

		// Trigger via Temporal ScheduleClient (simulate what the cron would do)
		handle := env.TemporalClient.ScheduleClient().GetHandle(ctx, heartbeatScheduleID)
		err := handle.Trigger(ctx, client.ScheduleTriggerOptions{})
		require.NoError(t, err, "manual schedule trigger should succeed")

		// Wait for the workflow to complete and update the schedule row
		deadline := time.Now().Add(30 * time.Second)
		var lastStatus string
		for time.Now().Before(deadline) {
			err = env.DB.QueryRow(ctx, `
				SELECT COALESCE(last_status, '') FROM schedules
				WHERE id = $1 AND tenant_id = $2
			`, heartbeatScheduleID, env.TenantID).Scan(&lastStatus)
			require.NoError(t, err)

			if lastStatus == "succeeded" || lastStatus == "skipped" {
				break
			}
			time.Sleep(1 * time.Second)
		}

		assert.Contains(t, []string{"succeeded", "skipped"}, lastStatus,
			"schedule last_status should be succeeded or skipped after heartbeat runs")

		// last_run_at should be set
		var lastRunAt *time.Time
		err = env.DB.QueryRow(ctx, `
			SELECT last_run_at FROM schedules
			WHERE id = $1 AND tenant_id = $2
		`, heartbeatScheduleID, env.TenantID).Scan(&lastRunAt)
		require.NoError(t, err)
		assert.NotNil(t, lastRunAt, "last_run_at should be set after heartbeat execution")

		t.Logf("Schedule updated: last_status=%s, last_run_at=%v", lastStatus, lastRunAt)
	})

	// =====================================================================
	// Subtest: heartbeat_review_queue_check
	// =====================================================================
	t.Run("heartbeat_review_queue_check", func(t *testing.T) {
		// Insert a pending review item so the heartbeat has something to find
		_, err := env.DB.Exec(ctx, `
			INSERT INTO review_queue (tenant_id, question_type, priority, question, status, created_at, updated_at)
			VALUES ($1, 'acronym', 'high', 'What does ACME stand for?', 'pending', NOW(), NOW())
		`, env.TenantID)
		require.NoError(t, err)
		t.Cleanup(func() {
			env.DB.Exec(ctx, `DELETE FROM review_queue WHERE tenant_id = $1 AND question = 'What does ACME stand for?'`, env.TenantID)
		})

		// Run heartbeat with just review_queue check
		workflowID := fmt.Sprintf("e2e-heartbeat-review-%d", time.Now().UnixNano())
		input := map[string]interface{}{
			"tenant_id":   env.TenantID,
			"schedule_id": heartbeatScheduleID,
			"checks":      []string{"review_queue"},
		}
		inputJSON, _ := json.Marshal(input)

		run, err := env.TemporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: "penfold-main",
		}, "HeartbeatWorkflow", json.RawMessage(inputJSON))
		require.NoError(t, err)

		var result json.RawMessage
		err = run.Get(ctx, &result)
		require.NoError(t, err)

		var findings map[string]interface{}
		json.Unmarshal(result, &findings)

		// review_queue check should report pending items
		rqResult, ok := findings["review_queue"].(map[string]interface{})
		require.True(t, ok, "review_queue result should be a map")
		assert.Greater(t, rqResult["pending_count"].(float64), float64(0),
			"review_queue should report pending items")
		t.Logf("Review queue check found %v pending items", rqResult["pending_count"])
	})
}

// cleanupHeartbeatTestSchedules removes e2e-test-heartbeat schedules.
func cleanupHeartbeatTestSchedules(t *testing.T, env *PipelineE2EEnv, client schedulev1.ScheduleServiceClient) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := env.DB.Query(ctx, `
		SELECT id, name FROM schedules
		WHERE tenant_id = $1 AND name LIKE 'e2e-test-heartbeat%'
	`, env.TenantID)
	if err != nil {
		t.Logf("heartbeat cleanup: could not query schedules: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		_, err := client.DeleteSchedule(ctx, &schedulev1.DeleteScheduleRequest{
			TenantId:   env.TenantID,
			ScheduleId: id,
		})
		if err != nil {
			// Fallback: direct DB delete
			env.DB.Exec(ctx, "DELETE FROM schedules WHERE id = $1", id)
			handle := env.TemporalClient.ScheduleClient().GetHandle(ctx, id)
			handle.Delete(ctx)
		}
		t.Logf("heartbeat cleanup: deleted %s (%s)", name, id)
	}
}
