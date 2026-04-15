// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/activity"

	gmailv1 "github.com/otherjamesbrown/penfold/api/proto/gmailv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// GmailSyncTickerActivities holds the Gmail connector client used by
// GmailSyncTickerWorkflow to trigger hourly syncs.
type GmailSyncTickerActivities struct {
	gmailClient gmailv1.GmailConnectorServiceClient
	logger      logging.Logger
}

// NewGmailSyncTickerActivities creates a new GmailSyncTickerActivities.
// gmailClient should be a GmailConnectorServiceClient pointed at the gateway.
func NewGmailSyncTickerActivities(client gmailv1.GmailConnectorServiceClient, logger logging.Logger) *GmailSyncTickerActivities {
	return &GmailSyncTickerActivities{
		gmailClient: client,
		logger:      logger.With(logging.F("component", "gmail_sync_ticker")),
	}
}

// GmailSyncTick calls GmailConnectorService.SyncEmails for the given tenant.
// This is the sole activity executed by GmailSyncTickerWorkflow.
// The gateway proxy routes the call to the Gmail service which handles
// incremental sync state, OAuth token refresh, and per-message SLMPipelineWorkflow starts.
func (a *GmailSyncTickerActivities) GmailSyncTick(ctx context.Context, input pkgtemporal.GmailSyncTickerInput) error {
	if input.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}

	a.logger.Info("GmailSyncTick starting", logging.F("tenant_id", input.TenantID))
	activity.RecordHeartbeat(ctx, "starting gmail sync")

	req := &gmailv1.SyncEmailsRequest{
		TenantId: input.TenantID,
	}

	resp, err := a.gmailClient.SyncEmails(ctx, req)
	if err != nil {
		a.logger.Error("GmailSyncTick failed",
			logging.F("tenant_id", input.TenantID),
			logging.Err(err),
		)
		return fmt.Errorf("SyncEmails RPC: %w", err)
	}

	a.logger.Info("GmailSyncTick completed",
		logging.F("tenant_id", input.TenantID),
		logging.F("sync_id", resp.GetSyncId()),
		logging.F("message", resp.GetMessage()),
	)
	return nil
}
