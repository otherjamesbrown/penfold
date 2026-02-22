package workflows

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// checkStaleConversationsInput matches activities.CheckStaleConversationsInput for serialization.
type checkStaleConversationsInput struct {
	TenantID  string `json:"tenant_id"`
	StaleDays int    `json:"stale_days"`
	Limit     int    `json:"limit"`
}

// checkStaleConversationsOutput matches activities.CheckStaleConversationsOutput for deserialization.
type checkStaleConversationsOutput struct {
	Checked      int `json:"checked"`
	Transitioned int `json:"transitioned"`
	Failed       int `json:"failed"`
}

// backfillConversationSummariesInput matches activities.BackfillConversationSummariesInput for serialization.
type backfillConversationSummariesInput struct {
	TenantID string `json:"tenant_id"`
	Limit    int    `json:"limit"`
}

// backfillConversationSummariesOutput matches activities.BackfillConversationSummariesOutput for deserialization.
type backfillConversationSummariesOutput struct {
	Processed int `json:"processed"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

// ConversationMaintenanceWorkflow runs stale conversation detection.
// Designed to be triggered by a Temporal schedule (daily).
func ConversationMaintenanceWorkflow(ctx workflow.Context, input pkgtemporal.ConversationMaintenanceInput) (*pkgtemporal.ConversationMaintenanceResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting conversation maintenance workflow",
		"tenant_id", input.TenantID,
		"stale_days", input.StaleDays,
	)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout:    10 * time.Minute,
		ScheduleToCloseTimeout: 15 * time.Minute,
		HeartbeatTimeout:       2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    5 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    2,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	activityInput := checkStaleConversationsInput{
		TenantID:  input.TenantID,
		StaleDays: input.StaleDays,
		Limit:     input.Limit,
	}

	var output checkStaleConversationsOutput
	err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityCheckStaleConversations, activityInput).Get(ctx, &output)
	if err != nil {
		logger.Error("Stale conversation check failed", "error", err)
		return &pkgtemporal.ConversationMaintenanceResult{
			Status: "failed",
			Error:  err.Error(),
		}, nil
	}

	logger.Info("Conversation maintenance complete",
		"checked", output.Checked,
		"transitioned", output.Transitioned,
		"failed", output.Failed,
	)

	return &pkgtemporal.ConversationMaintenanceResult{
		Checked:      output.Checked,
		Transitioned: output.Transitioned,
		Failed:       output.Failed,
		Status:       "completed",
	}, nil
}

// ConversationBackfillWorkflow runs summary backfill for unsummarized conversations.
// Designed to be triggered as a one-shot workflow via CLI or API.
func ConversationBackfillWorkflow(ctx workflow.Context, input pkgtemporal.ConversationBackfillInput) (*pkgtemporal.ConversationBackfillResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting conversation backfill workflow",
		"tenant_id", input.TenantID,
		"limit", input.Limit,
	)

	// Backfill processes many conversations with LLM calls — allow generous time.
	ao := workflow.ActivityOptions{
		StartToCloseTimeout:    30 * time.Minute,
		ScheduleToCloseTimeout: 45 * time.Minute,
		HeartbeatTimeout:       2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    5 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    1, // Don't retry — backfill is idempotent but expensive
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	activityInput := backfillConversationSummariesInput{
		TenantID: input.TenantID,
		Limit:    input.Limit,
	}

	var output backfillConversationSummariesOutput
	err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityBackfillConversationSummaries, activityInput).Get(ctx, &output)
	if err != nil {
		logger.Error("Conversation backfill failed", "error", err)
		return &pkgtemporal.ConversationBackfillResult{
			Status: "failed",
			Error:  err.Error(),
		}, nil
	}

	logger.Info("Conversation backfill complete",
		"processed", output.Processed,
		"failed", output.Failed,
		"skipped", output.Skipped,
	)

	return &pkgtemporal.ConversationBackfillResult{
		Processed: output.Processed,
		Failed:    output.Failed,
		Skipped:   output.Skipped,
		Status:    "completed",
	}, nil
}
