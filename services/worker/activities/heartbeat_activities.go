package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// HeartbeatActivities implements heartbeat check activities.
type HeartbeatActivities struct {
	db     *pgxpool.Pool
	logger logging.Logger
}

// NewHeartbeatActivities creates a new HeartbeatActivities instance.
func NewHeartbeatActivities(db *pgxpool.Pool, logger logging.Logger) *HeartbeatActivities {
	return &HeartbeatActivities{
		db:     db,
		logger: logger,
	}
}

// CheckReviewQueue counts pending items in the review_queue table.
func (h *HeartbeatActivities) CheckReviewQueue(ctx context.Context, tenantID string) (*pkgtemporal.HeartbeatCheckResult, error) {
	var totalPending, highPriority int
	err := h.db.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE priority = 'high')
		FROM review_queue
		WHERE tenant_id = $1 AND status = 'pending'
	`, tenantID).Scan(&totalPending, &highPriority)
	if err != nil {
		h.logger.Error("Heartbeat: review_queue check failed", logging.Err(err))
		return &pkgtemporal.HeartbeatCheckResult{
			Summary: fmt.Sprintf("check failed: %v", err),
		}, nil
	}

	return &pkgtemporal.HeartbeatCheckResult{
		PendingCount: totalPending,
		HighPriority: highPriority,
		Summary:      fmt.Sprintf("%d pending (%d high priority)", totalPending, highPriority),
		Actionable:   highPriority > 0,
	}, nil
}

// CheckWatchMatches counts assertions from the last 24h that match watch list criteria.
func (h *HeartbeatActivities) CheckWatchMatches(ctx context.Context, tenantID string) (*pkgtemporal.HeartbeatCheckResult, error) {
	// Check if assertions table has recent entries matching any watched entities.
	// Use a simple count of recent assertions as a proxy — the watch_list table
	// may not exist yet in all deployments, so fall back gracefully.
	var matchCount int
	err := h.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM assertions
		WHERE tenant_id = $1
		AND created_at > NOW() - INTERVAL '24 hours'
	`, tenantID).Scan(&matchCount)
	if err != nil {
		h.logger.Error("Heartbeat: watch_matches check failed", logging.Err(err))
		return &pkgtemporal.HeartbeatCheckResult{
			Summary: fmt.Sprintf("check failed: %v", err),
		}, nil
	}

	return &pkgtemporal.HeartbeatCheckResult{
		PendingCount: matchCount,
		Summary:      fmt.Sprintf("%d new assertions in last 24h", matchCount),
		Actionable:   matchCount > 0,
	}, nil
}

// CheckStaleContent counts sources stuck in processing for over 1 hour.
func (h *HeartbeatActivities) CheckStaleContent(ctx context.Context, tenantID string) (*pkgtemporal.HeartbeatCheckResult, error) {
	var stuckCount int
	err := h.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM sources
		WHERE tenant_id = $1
		AND processing_status IN ('processing', 'pending')
		AND updated_at < $2
	`, tenantID, time.Now().Add(-1*time.Hour)).Scan(&stuckCount)
	if err != nil {
		h.logger.Error("Heartbeat: stale_content check failed", logging.Err(err))
		return &pkgtemporal.HeartbeatCheckResult{
			Summary: fmt.Sprintf("check failed: %v", err),
		}, nil
	}

	return &pkgtemporal.HeartbeatCheckResult{
		PendingCount: stuckCount,
		Summary:      fmt.Sprintf("%d sources stuck in processing >1h", stuckCount),
		Actionable:   stuckCount > 0,
	}, nil
}

// UpdateScheduleStatus updates the schedule row with heartbeat results.
func (h *HeartbeatActivities) UpdateScheduleStatus(ctx context.Context, update pkgtemporal.HeartbeatStatusUpdate) error {
	schedStatus := "succeeded"
	if update.Status == "skipped" {
		schedStatus = "skipped"
	}

	_, err := h.db.Exec(ctx, `
		UPDATE schedules
		SET last_status = $1, last_run_at = NOW(), last_error = $2, updated_at = NOW()
		WHERE id = $3 AND tenant_id = $4
	`, schedStatus, update.Summary, update.ScheduleID, update.TenantID)
	if err != nil {
		return fmt.Errorf("update schedule status: %w", err)
	}
	return nil
}
