// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// GmailSyncTickerInput is the input for GmailSyncTickerWorkflow.
type GmailSyncTickerInput = pkgtemporal.GmailSyncTickerInput

// GmailSyncTickerWorkflow is a thin cron-driven workflow that calls the gateway's
// GmailConnectorService.SyncEmails RPC for a single tenant, then exits.
//
// Decision (pf-830415): Option 1 — wraps the proven SyncEmails production path
// (gateway → Gmail service → SLMPipelineWorkflow per new message on penfold-main).
// Avoids the user_id ambiguity of GmailSyncWorkflow (Option 2), which has never
// fired in production and whose activities are not registered with the worker.
//
// Registered on penfold-main (same queue the TemporalScheduler targets).
// Scheduled via ScheduleService with cron "0 * * * *" (hourly on the hour).
func GmailSyncTickerWorkflow(ctx workflow.Context, input GmailSyncTickerInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("GmailSyncTickerWorkflow starting", "tenant_id", input.TenantID)

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Minute,
		HeartbeatTimeout:    5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	err := workflow.ExecuteActivity(ctx, pkgtemporal.ActivityGmailSyncTick, input).Get(ctx, nil)
	if err != nil {
		logger.Error("GmailSyncTickerWorkflow failed", "tenant_id", input.TenantID, "error", err)
		return err
	}

	logger.Info("GmailSyncTickerWorkflow completed", "tenant_id", input.TenantID)
	return nil
}
