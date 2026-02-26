package main

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	pkgconfig "github.com/otherjamesbrown/penfold/pkg/config"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
	"github.com/otherjamesbrown/penfold/services/worker/config"
)

const (
	// ScheduleIDStaleConversations is the Temporal schedule ID for stale conversation detection.
	ScheduleIDStaleConversations = "conversation-stale-check"

	// ScheduleIDLedgerConsolidation is the Temporal schedule ID for daily session ledger consolidation.
	ScheduleIDLedgerConsolidation = "ledger-consolidation"
)

// ensureSchedules creates or updates Temporal schedules for recurring maintenance tasks.
// This is idempotent — safe to call on every worker startup.
func ensureSchedules(ctx context.Context, temporalClient client.Client, logger logging.Logger) {
	if err := ensureStaleConversationSchedule(ctx, temporalClient, logger); err != nil {
		logger.Error("Failed to ensure stale conversation schedule", logging.Err(err))
	}
	if err := ensureLedgerConsolidationSchedule(ctx, temporalClient, logger); err != nil {
		logger.Error("Failed to ensure ledger consolidation schedule", logging.Err(err))
	}
}

// ensureStaleConversationSchedule creates or updates the daily stale conversation detection schedule.
func ensureStaleConversationSchedule(ctx context.Context, temporalClient client.Client, logger logging.Logger) error {
	scheduleClient := temporalClient.ScheduleClient()

	input := pkgtemporal.ConversationMaintenanceInput{
		TenantID:  pkgconfig.DefaultTenantID().String(),
		StaleDays: 14,
		Limit:     100,
	}

	scheduleOpts := client.ScheduleOptions{
		ID: ScheduleIDStaleConversations,
		Spec: client.ScheduleSpec{
			CronExpressions: []string{"0 3 * * *"}, // Daily at 3 AM
		},
		Action: &client.ScheduleWorkflowAction{
			ID:        "conversation-stale-check-run",
			Workflow:  pkgtemporal.WorkflowConversationMaintenance,
			TaskQueue: config.MainTaskQueue,
			Args:      []interface{}{input},
		},
		Overlap: enums.SCHEDULE_OVERLAP_POLICY_SKIP,
	}

	// Try to create the schedule. If it already exists, update it.
	_, err := scheduleClient.Create(ctx, scheduleOpts)
	if err != nil {
		// Check if schedule already exists — if so, update it
		handle := scheduleClient.GetHandle(ctx, ScheduleIDStaleConversations)
		desc, descErr := handle.Describe(ctx)
		if descErr != nil {
			return fmt.Errorf("schedule create failed and could not describe existing: create=%w, describe=%v", err, descErr)
		}

		// Schedule exists — update it with new spec and action
		updateErr := handle.Update(ctx, client.ScheduleUpdateOptions{
			DoUpdate: func(input client.ScheduleUpdateInput) (*client.ScheduleUpdate, error) {
				input.Description.Schedule.Spec = &client.ScheduleSpec{
					CronExpressions: []string{"0 3 * * *"},
				}
				input.Description.Schedule.Action = scheduleOpts.Action
				if input.Description.Schedule.Policy == nil {
					input.Description.Schedule.Policy = &client.SchedulePolicies{}
				}
				input.Description.Schedule.Policy.Overlap = enums.SCHEDULE_OVERLAP_POLICY_SKIP
				return &client.ScheduleUpdate{
					Schedule: &input.Description.Schedule,
				}, nil
			},
		})
		if updateErr != nil {
			return fmt.Errorf("failed to update existing schedule: %w", updateErr)
		}

		logger.Info("Updated existing stale conversation schedule",
			logging.F("schedule_id", ScheduleIDStaleConversations),
			logging.F("next_run", formatNextRun(desc)),
		)
		return nil
	}

	logger.Info("Created stale conversation detection schedule",
		logging.F("schedule_id", ScheduleIDStaleConversations),
		logging.F("cron", "0 3 * * *"),
		logging.F("stale_days", 14),
	)
	return nil
}

// ensureLedgerConsolidationSchedule creates or updates the daily session ledger consolidation schedule.
func ensureLedgerConsolidationSchedule(ctx context.Context, temporalClient client.Client, logger logging.Logger) error {
	scheduleClient := temporalClient.ScheduleClient()

	input := pkgtemporal.SessionLedgerConsolidationInput{
		TenantID:    pkgconfig.DefaultTenantID().String(),
		CutoffHours: 48,
		MinEntries:  3,
	}

	scheduleOpts := client.ScheduleOptions{
		ID: ScheduleIDLedgerConsolidation,
		Spec: client.ScheduleSpec{
			CronExpressions: []string{"0 3 * * *"}, // Daily at 3 AM UTC
		},
		Action: &client.ScheduleWorkflowAction{
			ID:        "ledger-consolidation-run",
			Workflow:  pkgtemporal.WorkflowSessionLedgerConsolidation,
			TaskQueue: config.MainTaskQueue,
			Args:      []interface{}{input},
		},
		Overlap: enums.SCHEDULE_OVERLAP_POLICY_SKIP,
	}

	_, err := scheduleClient.Create(ctx, scheduleOpts)
	if err != nil {
		handle := scheduleClient.GetHandle(ctx, ScheduleIDLedgerConsolidation)
		desc, descErr := handle.Describe(ctx)
		if descErr != nil {
			return fmt.Errorf("schedule create failed and could not describe existing: create=%w, describe=%v", err, descErr)
		}

		updateErr := handle.Update(ctx, client.ScheduleUpdateOptions{
			DoUpdate: func(input client.ScheduleUpdateInput) (*client.ScheduleUpdate, error) {
				input.Description.Schedule.Spec = &client.ScheduleSpec{
					CronExpressions: []string{"0 3 * * *"},
				}
				input.Description.Schedule.Action = scheduleOpts.Action
				if input.Description.Schedule.Policy == nil {
					input.Description.Schedule.Policy = &client.SchedulePolicies{}
				}
				input.Description.Schedule.Policy.Overlap = enums.SCHEDULE_OVERLAP_POLICY_SKIP
				return &client.ScheduleUpdate{
					Schedule: &input.Description.Schedule,
				}, nil
			},
		})
		if updateErr != nil {
			return fmt.Errorf("failed to update existing schedule: %w", updateErr)
		}

		logger.Info("Updated existing ledger consolidation schedule",
			logging.F("schedule_id", ScheduleIDLedgerConsolidation),
			logging.F("next_run", formatNextRun(desc)),
		)
		return nil
	}

	logger.Info("Created ledger consolidation schedule",
		logging.F("schedule_id", ScheduleIDLedgerConsolidation),
		logging.F("cron", "0 3 * * *"),
		logging.F("cutoff_hours", 48),
		logging.F("min_entries", 3),
	)
	return nil
}

func formatNextRun(desc *client.ScheduleDescription) string {
	if len(desc.Info.NextActionTimes) > 0 {
		return desc.Info.NextActionTimes[0].Format(time.RFC3339)
	}
	return "unknown"
}
