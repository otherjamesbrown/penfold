// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// OutlookSyncInput is the input for Outlook email synchronization workflows.
type OutlookSyncInput = pkgtemporal.OutlookSyncInput

// OutlookSyncResult is the result of Outlook email synchronization workflows.
type OutlookSyncResult = pkgtemporal.OutlookSyncResult

// OutlookSyncWorkflowStatus tracks the status of Outlook email synchronization.
type OutlookSyncWorkflowStatus = pkgtemporal.WorkflowStatus

// Signal and query names for OutlookSyncWorkflow.
const (
	OutlookSyncStatusQuery  = "outlook_sync_status"
	OutlookSyncPauseSignal  = "outlook_sync_pause"
	OutlookSyncCancelSignal = "outlook_sync_cancel"
)

// Activity input types for Outlook synchronization.
type (
	// FetchOutlookMessagesInput is the input for the FetchOutlookMessages activity.
	FetchOutlookMessagesInput struct {
		TenantID      string    `json:"tenant_id"`
		IntegrationID int64     `json:"integration_id"`
		UserID        string    `json:"user_id"`
		SinceTime     time.Time `json:"since_time,omitempty"` // for first sync / incremental
		BatchSize     int       `json:"batch_size"`
		PageToken     string    `json:"page_token,omitempty"` // nextLink for pagination
	}

	// OutlookMessageInfo contains basic message metadata from list query.
	OutlookMessageInfo struct {
		MessageID         string    `json:"message_id"`
		InternetMessageID string    `json:"internet_message_id,omitempty"`
		ReceivedAt        time.Time `json:"received_at"`
	}

	// FetchOutlookMessagesOutput is the result of fetching messages.
	FetchOutlookMessagesOutput struct {
		Messages      []OutlookMessageInfo `json:"messages"`
		NextPageToken string               `json:"next_page_token,omitempty"`
		HasMore       bool                 `json:"has_more"`
		MessageCount  int                  `json:"message_count"`
	}

	// ProcessOutlookMessageInput is the input for the ProcessOutlookMessage activity.
	ProcessOutlookMessageInput struct {
		TenantID      string `json:"tenant_id"`
		IntegrationID int64  `json:"integration_id"`
		UserID        string `json:"user_id"`
		MessageID     string `json:"message_id"`
	}

	// ProcessOutlookMessageOutput is the result of processing a single message.
	ProcessOutlookMessageOutput struct {
		SourceID  int64  `json:"source_id"`
		ContentID string `json:"content_id,omitempty"`
		Action    string `json:"action"` // created, updated, skipped
	}

	// UpdateOutlookSyncStateInput is the input for the UpdateOutlookSyncState activity.
	UpdateOutlookSyncStateInput struct {
		TenantID       string    `json:"tenant_id"`
		IntegrationID  int64     `json:"integration_id"`
		MessagesSynced int       `json:"messages_synced"`
		SyncedAt       time.Time `json:"synced_at"`
	}

	// RollbackOutlookSyncInput is the input for the RollbackOutlookSync compensation activity.
	RollbackOutlookSyncInput struct {
		TenantID  string  `json:"tenant_id"`
		SourceIDs []int64 `json:"source_ids"`
	}
)

// outlookSyncState maintains the internal state of the workflow.
type outlookSyncState struct {
	status          OutlookSyncWorkflowStatus
	result          *OutlookSyncResult
	paused          bool
	cancelRequested bool
	cancelReason    string
	createdSourceIDs []int64
}

// OutlookSyncWorkflow orchestrates the synchronization of Outlook email messages via Microsoft Graph.
// It performs the following steps:
// 1. Check Graph API auth token validity
// 2. Fetch messages (paginated via nextLink / delta query)
// 3. For each message, download MIME and ingest into the pipeline
// 4. Update sync state (last_sync_at)
//
// This workflow supports both incremental and full sync modes and implements
// the saga pattern with compensation for rollback on failure.
func OutlookSyncWorkflow(ctx workflow.Context, input OutlookSyncInput) (*OutlookSyncResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting Outlook sync workflow",
		"tenant_id", input.TenantID,
		"integration_id", input.IntegrationID,
		"user_id", input.UserID,
		"job_id", input.JobID,
		"sync_mode", input.SyncMode,
	)

	// Initialize workflow state
	state := &outlookSyncState{
		status: OutlookSyncWorkflowStatus{
			Stage:          "initializing",
			StepsCompleted: 0,
			TotalSteps:     4,
			LastActivity:   "",
			StartedAt:      workflow.Now(ctx),
			LastUpdated:    workflow.Now(ctx),
		},
		result: &OutlookSyncResult{
			TenantID: input.TenantID,
		},
		createdSourceIDs: make([]int64, 0),
	}

	// Register query handler for status
	if err := workflow.SetQueryHandler(ctx, OutlookSyncStatusQuery, func() (OutlookSyncWorkflowStatus, error) {
		return state.status, nil
	}); err != nil {
		logger.Error("Failed to register status query handler", "error", err)
	}

	// Setup signal channels
	pauseSignalChan := workflow.GetSignalChannel(ctx, OutlookSyncPauseSignal)
	cancelSignalChan := workflow.GetSignalChannel(ctx, OutlookSyncCancelSignal)

	// Activity option presets
	fastOpts := pkgtemporal.FastActivityOptions()
	batchOpts := pkgtemporal.BatchActivityOptions()

	fastOpts = pkgtemporal.WithNonRetryableErrors(fastOpts, pkgtemporal.NonRetryableErrors()...)
	batchOpts = pkgtemporal.WithNonRetryableErrors(batchOpts, pkgtemporal.NonRetryableErrors()...)

	// Saga compensation stack
	var compensations []func(workflow.Context) error

	// Drain signals helper
	drainSignals := func() {
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(pauseSignalChan, func(c workflow.ReceiveChannel, more bool) {
			var signal pkgtemporal.PauseResumeSignal
			c.Receive(ctx, &signal)
			state.paused = signal.Paused
		})
		selector.AddReceive(cancelSignalChan, func(c workflow.ReceiveChannel, more bool) {
			var signal pkgtemporal.CancelWithCompensationSignal
			c.Receive(ctx, &signal)
			state.cancelRequested = true
			state.cancelReason = signal.Reason
		})
		for selector.HasPending() {
			selector.Select(ctx)
		}
	}

	// Handle pause signal
	handlePauseSignal := func() {
		for {
			if !state.paused {
				return
			}
			selector := workflow.NewSelector(ctx)
			selector.AddReceive(pauseSignalChan, func(c workflow.ReceiveChannel, more bool) {
				var signal pkgtemporal.PauseResumeSignal
				c.Receive(ctx, &signal)
				state.paused = signal.Paused
			})
			selector.AddReceive(cancelSignalChan, func(c workflow.ReceiveChannel, more bool) {
				var signal pkgtemporal.CancelWithCompensationSignal
				c.Receive(ctx, &signal)
				state.cancelRequested = true
				state.cancelReason = signal.Reason
				state.paused = false
			})
			selector.Select(ctx)
		}
	}

	// Check controls helper
	checkControls := func() bool {
		drainSignals()
		handlePauseSignal()
		return state.cancelRequested
	}

	// Run compensation helper
	runCompensation := func(ctx workflow.Context) {
		if len(compensations) == 0 {
			return
		}
		logger.Info("Running saga compensation", "compensation_count", len(compensations))
		state.status.CompensationRan = true
		for i := len(compensations) - 1; i >= 0; i-- {
			if err := compensations[i](ctx); err != nil {
				logger.Warn("Compensation failed", "index", i, "error", err)
			}
		}
	}

	// Update status helper
	updateStatus := func(stage, activity string) {
		state.status.Stage = stage
		state.status.LastActivity = activity
		state.status.LastUpdated = workflow.Now(ctx)
	}

	// Step 1: Check Graph auth token
	updateStatus("checking_auth", "CheckGraphAuth")
	var authOutput CheckGraphAuthOutput
	ctx1 := workflow.WithActivityOptions(ctx, fastOpts)
	err := workflow.ExecuteActivity(ctx1, "CheckGraphAuth", CheckGraphAuthInput{
		TenantID:      input.TenantID,
		IntegrationID: input.IntegrationID,
	}).Get(ctx, &authOutput)
	if err != nil {
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("check_auth: %v", err)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Failed to check Graph auth", "error", err)
		return state.result, nil
	}
	if !authOutput.IsValid {
		state.result.Status = "failed"
		state.result.Error = "graph auth token is invalid or expired"
		state.status.ErrorMessage = state.result.Error
		return state.result, nil
	}
	state.status.StepsCompleted = 1

	if checkControls() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		runCompensation(ctx)
		return state.result, nil
	}

	// Step 2: Fetch and process messages in pages
	updateStatus("fetching_messages", "FetchOutlookMessages")

	pageToken := ""
	batchSize := 50
	if input.BatchSize > 0 {
		batchSize = input.BatchSize
	}

	totalProcessed := 0
	for {
		// Fetch a page of messages
		var fetchOutput FetchOutlookMessagesOutput
		ctx3 := workflow.WithActivityOptions(ctx, batchOpts)
		err = workflow.ExecuteActivity(ctx3, "FetchOutlookMessages", FetchOutlookMessagesInput{
			TenantID:      input.TenantID,
			IntegrationID: input.IntegrationID,
			UserID:        input.UserID,
			BatchSize:     batchSize,
			PageToken:     pageToken,
		}).Get(ctx, &fetchOutput)
		if err != nil {
			// Partial failure — save what we have if any messages already processed
			if totalProcessed > 0 {
				state.result.Status = "partial"
				state.result.Error = fmt.Sprintf("fetch_messages: %v (partial sync)", err)
				logger.Warn("Partial sync due to fetch error", "error", err, "processed", totalProcessed)
				break
			}
			state.result.Status = "failed"
			state.result.Error = fmt.Sprintf("fetch_messages: %v", err)
			state.status.ErrorMessage = state.result.Error
			runCompensation(ctx)
			logger.Error("Failed to fetch Outlook messages", "error", err)
			return state.result, nil
		}

		// Process each message
		updateStatus("processing_messages", "ProcessOutlookMessage")
		for _, msg := range fetchOutput.Messages {
			if checkControls() {
				state.result.Status = "partial"
				state.result.Error = "cancelled during processing"
				break
			}

			var processOutput ProcessOutlookMessageOutput
			ctx4 := workflow.WithActivityOptions(ctx, fastOpts)
			err = workflow.ExecuteActivity(ctx4, "ProcessOutlookMessage", ProcessOutlookMessageInput{
				TenantID:      input.TenantID,
				IntegrationID: input.IntegrationID,
				UserID:        input.UserID,
				MessageID:     msg.MessageID,
			}).Get(ctx, &processOutput)
			if err != nil {
				logger.Warn("Failed to process message", "message_id", msg.MessageID, "error", err)
				continue // Skip failed messages
			}

			switch processOutput.Action {
			case "created":
				state.result.NewMessages++
				state.createdSourceIDs = append(state.createdSourceIDs, processOutput.SourceID)
			case "updated":
				state.result.UpdatedMsgs++
			case "skipped":
				state.result.SkippedMsgs++
			}
			state.result.MessagesSynced++
			totalProcessed++
		}

		if state.cancelRequested || !fetchOutput.HasMore {
			break
		}
		pageToken = fetchOutput.NextPageToken
	}
	state.status.StepsCompleted = 2

	// Add compensation for rollback
	if len(state.createdSourceIDs) > 0 {
		compensations = append(compensations, func(ctx workflow.Context) error {
			return workflow.ExecuteActivity(
				workflow.WithActivityOptions(ctx, batchOpts),
				"RollbackOutlookSync",
				RollbackOutlookSyncInput{
					TenantID:  input.TenantID,
					SourceIDs: state.createdSourceIDs,
				},
			).Get(ctx, nil)
		})
	}

	// Step 3: Update sync state
	updateStatus("updating_sync_state", "UpdateOutlookSyncState")
	ctx5 := workflow.WithActivityOptions(ctx, fastOpts)
	err = workflow.ExecuteActivity(ctx5, "UpdateOutlookSyncState", UpdateOutlookSyncStateInput{
		TenantID:       input.TenantID,
		IntegrationID:  input.IntegrationID,
		MessagesSynced: state.result.MessagesSynced,
		SyncedAt:       workflow.Now(ctx),
	}).Get(ctx, nil)
	if err != nil {
		logger.Warn("Failed to update sync state", "error", err)
		// Don't fail the workflow, just log
	}
	state.status.StepsCompleted = 3

	// Finalize result
	state.status.StepsCompleted = 4
	updateStatus("completed", "")
	state.result.SyncedAt = workflow.Now(ctx)

	if state.result.Status == "" {
		state.result.Status = "completed"
	}

	logger.Info("Outlook sync workflow completed",
		"tenant_id", input.TenantID,
		"integration_id", input.IntegrationID,
		"user_id", input.UserID,
		"messages_synced", state.result.MessagesSynced,
		"new", state.result.NewMessages,
		"updated", state.result.UpdatedMsgs,
		"skipped", state.result.SkippedMsgs,
	)

	return state.result, nil
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
var _ = time.Second
