package workflows

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// consolidateEntriesInput matches activities.ConsolidateEntriesInput for serialization.
type consolidateEntriesInput struct {
	TenantID    string `json:"tenant_id"`
	CutoffHours int    `json:"cutoff_hours"`
	MinEntries  int    `json:"min_entries"`
}

// consolidateEntriesOutput matches activities.ConsolidateEntriesOutput for deserialization.
type consolidateEntriesOutput struct {
	SessionsChecked       int `json:"sessions_checked"`
	SessionsSkipped       int `json:"sessions_skipped"`
	ConsolidationsCreated int `json:"consolidations_created"`
	EntriesConsolidated   int `json:"entries_consolidated"`
	Failed                int `json:"failed"`
}

// SessionLedgerConsolidationWorkflow runs daily consolidation of session ledger entries.
// Groups unconsolidated entries by session, generates LLM narrative summaries, and
// writes consolidation records with extracted decisions and identified patterns.
func SessionLedgerConsolidationWorkflow(ctx workflow.Context, input pkgtemporal.SessionLedgerConsolidationInput) (*pkgtemporal.SessionLedgerConsolidationResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting session ledger consolidation workflow",
		"tenant_id", input.TenantID,
		"cutoff_hours", input.CutoffHours,
		"min_entries", input.MinEntries,
	)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout:    15 * time.Minute,
		ScheduleToCloseTimeout: 20 * time.Minute,
		HeartbeatTimeout:       3 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    2,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	activityInput := consolidateEntriesInput{
		TenantID:    input.TenantID,
		CutoffHours: input.CutoffHours,
		MinEntries:  input.MinEntries,
	}

	var output consolidateEntriesOutput
	err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityConsolidateEntries, activityInput).Get(ctx, &output)
	if err != nil {
		logger.Error("Consolidation activity failed", "error", err)
		return &pkgtemporal.SessionLedgerConsolidationResult{
			Status: "failed",
			Error:  err.Error(),
		}, nil
	}

	logger.Info("Session ledger consolidation complete",
		"sessions_checked", output.SessionsChecked,
		"consolidations_created", output.ConsolidationsCreated,
		"entries_consolidated", output.EntriesConsolidated,
		"failed", output.Failed,
	)

	return &pkgtemporal.SessionLedgerConsolidationResult{
		SessionsChecked:       output.SessionsChecked,
		SessionsSkipped:       output.SessionsSkipped,
		ConsolidationsCreated: output.ConsolidationsCreated,
		EntriesConsolidated:   output.EntriesConsolidated,
		Failed:                output.Failed,
		Status:                "completed",
	}, nil
}
