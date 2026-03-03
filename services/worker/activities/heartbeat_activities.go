package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/reviewqueue"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// ReviewQueueStatsProvider provides review queue statistics.
type ReviewQueueStatsProvider interface {
	GetStats(ctx context.Context, tenantID string) (*reviewqueue.QueueStats, error)
}

// ScheduleUpdater updates schedule status.
type ScheduleUpdater interface {
	Update(ctx context.Context, tenantID, id string, updates map[string]interface{}) error
}

// HeartbeatQuerier runs heartbeat queries that don't have existing repositories.
type HeartbeatQuerier interface {
	CountWatchMatches(ctx context.Context, tenantID string, windowHours int) (int, error)
	CountStaleContent(ctx context.Context, tenantID string, thresholdHours int) (int, error)
}

// HeartbeatActivities implements heartbeat check activities.
type HeartbeatActivities struct {
	logger          logging.Logger
	reviewQueueRepo ReviewQueueStatsProvider
	scheduleRepo    ScheduleUpdater
	querier         HeartbeatQuerier
}

// NewHeartbeatActivities creates a new HeartbeatActivities instance.
func NewHeartbeatActivities(
	logger logging.Logger,
	reviewQueueRepo ReviewQueueStatsProvider,
	scheduleRepo ScheduleUpdater,
	querier HeartbeatQuerier,
) *HeartbeatActivities {
	if logger == nil {
		panic("NewHeartbeatActivities: logger is required")
	}
	if reviewQueueRepo == nil {
		panic("NewHeartbeatActivities: reviewQueueRepo is required")
	}
	if scheduleRepo == nil {
		panic("NewHeartbeatActivities: scheduleRepo is required")
	}
	if querier == nil {
		panic("NewHeartbeatActivities: querier is required")
	}
	return &HeartbeatActivities{
		logger:          logger.With(logging.F("component", "heartbeat_activities")),
		reviewQueueRepo: reviewQueueRepo,
		scheduleRepo:    scheduleRepo,
		querier:         querier,
	}
}

// CheckReviewQueue counts pending items in the review_queue table.
func (h *HeartbeatActivities) CheckReviewQueue(ctx context.Context, tenantID string) (*pkgtemporal.HeartbeatCheckResult, error) {
	stats, err := h.reviewQueueRepo.GetStats(ctx, tenantID)
	if err != nil {
		h.logger.Error("Heartbeat: review_queue check failed", logging.Err(err))
		return &pkgtemporal.HeartbeatCheckResult{
			Summary: fmt.Sprintf("check failed: %v", err),
		}, nil
	}

	highPriority := stats.ByPriority["high"]

	return &pkgtemporal.HeartbeatCheckResult{
		PendingCount: stats.TotalPending,
		HighPriority: highPriority,
		Summary:      fmt.Sprintf("%d pending (%d high priority)", stats.TotalPending, highPriority),
		Actionable:   highPriority > 0,
	}, nil
}

// CheckWatchMatches counts assertions from the configured watch window.
func (h *HeartbeatActivities) CheckWatchMatches(ctx context.Context, tenantID string, watchWindowHours int) (*pkgtemporal.HeartbeatCheckResult, error) {
	if watchWindowHours <= 0 {
		watchWindowHours = pkgtemporal.DefaultWatchWindowHours
	}

	matchCount, err := h.querier.CountWatchMatches(ctx, tenantID, watchWindowHours)
	if err != nil {
		h.logger.Error("Heartbeat: watch_matches check failed", logging.Err(err))
		return &pkgtemporal.HeartbeatCheckResult{
			Summary: fmt.Sprintf("check failed: %v", err),
		}, nil
	}

	return &pkgtemporal.HeartbeatCheckResult{
		PendingCount: matchCount,
		Summary:      fmt.Sprintf("%d new assertions in last %dh", matchCount, watchWindowHours),
		Actionable:   matchCount > 0,
	}, nil
}

// CheckStaleContent counts sources stuck in processing for over the configured threshold.
func (h *HeartbeatActivities) CheckStaleContent(ctx context.Context, tenantID string, thresholdHours int) (*pkgtemporal.HeartbeatCheckResult, error) {
	if thresholdHours <= 0 {
		thresholdHours = pkgtemporal.DefaultStaleContentThresholdHours
	}

	stuckCount, err := h.querier.CountStaleContent(ctx, tenantID, thresholdHours)
	if err != nil {
		h.logger.Error("Heartbeat: stale_content check failed", logging.Err(err))
		return &pkgtemporal.HeartbeatCheckResult{
			Summary: fmt.Sprintf("check failed: %v", err),
		}, nil
	}

	return &pkgtemporal.HeartbeatCheckResult{
		PendingCount: stuckCount,
		Summary:      fmt.Sprintf("%d sources stuck in processing >%dh", stuckCount, thresholdHours),
		Actionable:   stuckCount > 0,
	}, nil
}

// UpdateScheduleStatus updates the schedule row with heartbeat results.
func (h *HeartbeatActivities) UpdateScheduleStatus(ctx context.Context, update pkgtemporal.HeartbeatStatusUpdate) error {
	schedStatus := pkgtemporal.HeartbeatScheduleStatusSucceeded
	if update.Status == pkgtemporal.HeartbeatStatusSkipped {
		schedStatus = pkgtemporal.HeartbeatScheduleStatusSkipped
	}

	return h.scheduleRepo.Update(ctx, update.TenantID, update.ScheduleID, map[string]interface{}{
		"last_status": schedStatus,
		"last_run_at": time.Now(),
		"last_error":  update.Summary,
	})
}
