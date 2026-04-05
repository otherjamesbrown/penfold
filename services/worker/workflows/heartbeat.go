package workflows

import (
	"fmt"
	"strings"

	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// HeartbeatWorkflow orchestrates heartbeat checks, evaluates urgency, and updates schedule status.
// Temporal's data converter handles JSON serialisation of the typed input/output.
func HeartbeatWorkflow(ctx workflow.Context, input pkgtemporal.HeartbeatInput) (*pkgtemporal.HeartbeatResult, error) {
	logger := workflow.GetLogger(ctx)

	hbInput := input

	logger.Info("Starting heartbeat workflow",
		"tenant_id", hbInput.TenantID,
		"schedule_id", hbInput.ScheduleID,
		"checks", hbInput.Checks,
	)

	if len(hbInput.Checks) == 0 {
		return &pkgtemporal.HeartbeatResult{
			Status:  pkgtemporal.HeartbeatStatusSkipped,
			Summary: "no checks configured",
		}, nil
	}

	// Apply defaults for configurable thresholds
	staleThreshold := hbInput.StaleContentThresholdHours
	if staleThreshold <= 0 {
		staleThreshold = pkgtemporal.DefaultStaleContentThresholdHours
	}
	watchWindow := hbInput.WatchWindowHours
	if watchWindow <= 0 {
		watchWindow = pkgtemporal.DefaultWatchWindowHours
	}

	// 2. Configure activity options — fast reads, minimal retries
	ctx = workflow.WithActivityOptions(ctx, pkgtemporal.FastActivityOptions())

	// 3. Run each configured check in parallel
	result := &pkgtemporal.HeartbeatResult{}
	type checkFuture struct {
		name   string
		future workflow.Future
	}
	var futures []checkFuture

	for _, check := range hbInput.Checks {
		switch check {
		case pkgtemporal.HeartbeatCheckReviewQueue:
			f := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityHeartbeatCheckReviewQueue, hbInput.TenantID)
			futures = append(futures, checkFuture{name: check, future: f})
		case pkgtemporal.HeartbeatCheckWatchMatches:
			f := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityHeartbeatCheckWatchMatches, hbInput.TenantID, watchWindow)
			futures = append(futures, checkFuture{name: check, future: f})
		case pkgtemporal.HeartbeatCheckStaleContent:
			f := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityHeartbeatCheckStaleContent, hbInput.TenantID, staleThreshold)
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
		case pkgtemporal.HeartbeatCheckReviewQueue:
			result.ReviewQueue = &checkResult
		case pkgtemporal.HeartbeatCheckWatchMatches:
			result.WatchMatches = &checkResult
		case pkgtemporal.HeartbeatCheckStaleContent:
			result.StaleContent = &checkResult
		}
	}

	// 5. Evaluate urgency
	result.Status = pkgtemporal.HeartbeatStatusOK
	if (result.ReviewQueue != nil && result.ReviewQueue.Actionable) ||
		(result.WatchMatches != nil && result.WatchMatches.Actionable) ||
		(result.StaleContent != nil && result.StaleContent.Actionable) {
		result.Status = pkgtemporal.HeartbeatStatusActionable
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

	// 8. Return result
	return result, nil
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
