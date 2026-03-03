package main

import (
	"context"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/schedule"
)

// reconcileSchedules ensures Temporal schedules match DB state.
// Called once on worker startup — creates missing Temporal schedules,
// updates existing ones to match DB spec.
func reconcileSchedules(ctx context.Context, repo *schedule.Repository, scheduler *schedule.TemporalScheduler, tenantID string, logger logging.Logger) {
	dbSchedules, err := repo.ListEnabled(ctx, tenantID)
	if err != nil {
		logger.Error("Failed to list enabled schedules for reconciliation", logging.Err(err))
		return
	}

	logger.Info("Reconciling schedules",
		logging.F("tenant_id", tenantID),
		logging.F("count", len(dbSchedules)),
	)

	for _, sched := range dbSchedules {
		if scheduler.Exists(ctx, sched.ID) {
			// Schedule exists in Temporal — update to match DB
			if err := scheduler.Update(ctx, sched); err != nil {
				logger.Error("Failed to update Temporal schedule during reconciliation",
					logging.F("schedule_id", sched.ID),
					logging.F("name", sched.Name),
					logging.Err(err),
				)
			} else {
				logger.Debug("Reconciled existing Temporal schedule",
					logging.F("schedule_id", sched.ID),
					logging.F("name", sched.Name),
				)
			}
		} else {
			// Schedule missing in Temporal — create it
			if err := scheduler.Create(ctx, sched); err != nil {
				logger.Error("Failed to create Temporal schedule during reconciliation",
					logging.F("schedule_id", sched.ID),
					logging.F("name", sched.Name),
					logging.Err(err),
				)
			} else {
				logger.Info("Created missing Temporal schedule during reconciliation",
					logging.F("schedule_id", sched.ID),
					logging.F("name", sched.Name),
					logging.F("cron", sched.ScheduleExpr),
				)
			}
		}
	}

	logger.Info("Schedule reconciliation complete",
		logging.F("tenant_id", tenantID),
		logging.F("schedules_processed", len(dbSchedules)),
	)
}
