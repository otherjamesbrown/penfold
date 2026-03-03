package workflows

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// HeartbeatWorkflow orchestrates heartbeat checks, evaluates urgency, and updates schedule status.
// It accepts json.RawMessage as input because Temporal schedules pass workflow_params JSONB from the DB.
func HeartbeatWorkflow(ctx workflow.Context, input json.RawMessage) (json.RawMessage, error) {
	logger := workflow.GetLogger(ctx)

	// 1. Parse input
	var hbInput pkgtemporal.HeartbeatInput
	if err := json.Unmarshal(input, &hbInput); err != nil {
		return nil, fmt.Errorf("unmarshal heartbeat input: %w", err)
	}

	logger.Info("Starting heartbeat workflow",
		"tenant_id", hbInput.TenantID,
		"schedule_id", hbInput.ScheduleID,
		"checks", hbInput.Checks,
	)

	if len(hbInput.Checks) == 0 {
		result := &pkgtemporal.HeartbeatResult{
			Status:  "skipped",
			Summary: "no checks configured",
		}
		resultJSON, _ := json.Marshal(result)
		return resultJSON, nil
	}

	// 2. Configure activity options — fast reads, minimal retries
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// 3. Run each configured check in parallel
	result := &pkgtemporal.HeartbeatResult{}
	type checkFuture struct {
		name   string
		future workflow.Future
	}
	var futures []checkFuture

	for _, check := range hbInput.Checks {
		switch check {
		case "review_queue":
			f := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityHeartbeatCheckReviewQueue, hbInput.TenantID)
			futures = append(futures, checkFuture{name: check, future: f})
		case "watch_matches":
			f := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityHeartbeatCheckWatchMatches, hbInput.TenantID)
			futures = append(futures, checkFuture{name: check, future: f})
		case "stale_content":
			f := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityHeartbeatCheckStaleContent, hbInput.TenantID)
			futures = append(futures, checkFuture{name: check, future: f})
		default:
			logger.Warn("Unknown heartbeat check, skipping", "check", check)
		}
	}

	// 4. Collect results
	for _, cf := range futures {
		var checkResult pkgtemporal.HeartbeatCheckResult
		if err := cf.future.Get(ctx, &checkResult); err != nil {
			logger.Error("Heartbeat check failed", "check", cf.name, "error", err)
			// Don't fail the whole heartbeat — use zero result
			checkResult = pkgtemporal.HeartbeatCheckResult{
				Summary: fmt.Sprintf("check failed: %v", err),
			}
		}

		switch cf.name {
		case "review_queue":
			result.ReviewQueue = &checkResult
		case "watch_matches":
			result.WatchMatches = &checkResult
		case "stale_content":
			result.StaleContent = &checkResult
		}
	}

	// 5. Evaluate urgency
	result.Status = "ok"
	if (result.ReviewQueue != nil && result.ReviewQueue.Actionable) ||
		(result.WatchMatches != nil && result.WatchMatches.Actionable) ||
		(result.StaleContent != nil && result.StaleContent.Actionable) {
		result.Status = "actionable"
	}

	// 6. Build summary
	result.Summary = buildHeartbeatSummary(result)

	// 7. Update schedule status via activity
	statusUpdate := pkgtemporal.HeartbeatStatusUpdate{
		TenantID:   hbInput.TenantID,
		ScheduleID: hbInput.ScheduleID,
		Status:     result.Status,
		Summary:    result.Summary,
	}
	if err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityHeartbeatUpdateStatus, statusUpdate).Get(ctx, nil); err != nil {
		logger.Error("Failed to update schedule status", "error", err)
		// Non-fatal — the heartbeat result is still valid
	}

	// 8. Return result as JSON
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal heartbeat result: %w", err)
	}
	return resultJSON, nil
}

// buildHeartbeatSummary constructs a human-readable summary from check results.
func buildHeartbeatSummary(r *pkgtemporal.HeartbeatResult) string {
	var parts []string
	if r.ReviewQueue != nil {
		parts = append(parts, r.ReviewQueue.Summary)
	}
	if r.WatchMatches != nil {
		parts = append(parts, r.WatchMatches.Summary)
	}
	if r.StaleContent != nil {
		parts = append(parts, r.StaleContent.Summary)
	}
	if len(parts) == 0 {
		return "no checks ran"
	}
	return strings.Join(parts, "; ")
}
