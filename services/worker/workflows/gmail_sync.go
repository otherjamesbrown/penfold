// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// GmailSyncInput is the input for Gmail synchronization workflows.
type GmailSyncInput = pkgtemporal.GmailSyncInput

// GmailSyncResult is the result of Gmail synchronization workflows.
type GmailSyncResult = pkgtemporal.GmailSyncResult

// GmailSyncWorkflowStatus tracks the status of Gmail synchronization.
type GmailSyncWorkflowStatus = pkgtemporal.WorkflowStatus

// Signal and query names for GmailSyncWorkflow.
const (
	GmailSyncStatusQuery  = "gmail_sync_status"
	GmailSyncPauseSignal  = "gmail_sync_pause"
	GmailSyncCancelSignal = "gmail_sync_cancel"
)

// Activity input types for Gmail synchronization.
type (
	// CheckGmailAuthInput is the input for the CheckGmailAuth activity.
	CheckGmailAuthInput struct {
		TenantID string `json:"tenant_id"`
		UserID   string `json:"user_id"`
	}

	// CheckGmailAuthOutput is the output from the CheckGmailAuth activity.
	CheckGmailAuthOutput struct {
		IsValid     bool   `json:"is_valid"`
		AccessToken string `json:"access_token"`
		ExpiresAt   time.Time `json:"expires_at"`
	}

	// RefreshGmailTokenInput is the input for the RefreshGmailToken activity.
	RefreshGmailTokenInput struct {
		TenantID string `json:"tenant_id"`
		UserID   string `json:"user_id"`
	}

	// RefreshGmailTokenOutput is the output from the RefreshGmailToken activity.
	RefreshGmailTokenOutput struct {
		AccessToken string    `json:"access_token"`
		ExpiresAt   time.Time `json:"expires_at"`
	}

	// FetchGmailMessagesInput is the input for the FetchGmailMessages activity.
	FetchGmailMessagesInput struct {
		TenantID      string    `json:"tenant_id"`
		UserID        string    `json:"user_id"`
		SyncMode      string    `json:"sync_mode"`
		SyncToken     string    `json:"sync_token,omitempty"`
		PageToken     string    `json:"page_token,omitempty"`
		MaxResults    int       `json:"max_results"`
		LabelFilters  []string  `json:"label_filters,omitempty"`
		StartDate     time.Time `json:"start_date,omitempty"`
	}

	// FetchGmailMessagesOutput is the output from the FetchGmailMessages activity.
	FetchGmailMessagesOutput struct {
		Messages      []GmailMessageInfo `json:"messages"`
		NextPageToken string             `json:"next_page_token,omitempty"`
		NextSyncToken string             `json:"next_sync_token,omitempty"`
		ResultCount   int                `json:"result_count"`
		HasMore       bool               `json:"has_more"`
	}

	// GmailMessageInfo contains basic Gmail message information.
	GmailMessageInfo struct {
		MessageID string `json:"message_id"`
		ThreadID  string `json:"thread_id"`
		IsNew     bool   `json:"is_new"`
		IsDeleted bool   `json:"is_deleted"`
	}

	// ProcessGmailMessageInput is the input for the ProcessGmailMessage activity.
	ProcessGmailMessageInput struct {
		TenantID       string `json:"tenant_id"`
		UserID         string `json:"user_id"`
		MessageID      string `json:"message_id"`
		ThreadID       string `json:"thread_id"`
		IncludeHeaders bool   `json:"include_headers"`
	}

	// ProcessGmailMessageOutput is the output from the ProcessGmailMessage activity.
	ProcessGmailMessageOutput struct {
		SourceID  int64  `json:"source_id"`
		Action    string `json:"action"` // created, updated, deleted, skipped
		MessageID string `json:"message_id"`
	}

	// UpdateGmailSyncStateInput is the input for the UpdateGmailSyncState activity.
	UpdateGmailSyncStateInput struct {
		TenantID       string    `json:"tenant_id"`
		UserID         string    `json:"user_id"`
		SyncToken      string    `json:"sync_token"`
		MessagesSynced int       `json:"messages_synced"`
		ThreadsSynced  int       `json:"threads_synced"`
		SyncedAt       time.Time `json:"synced_at"`
	}

	// RollbackGmailSyncInput is the input for the RollbackGmailSync compensation activity.
	RollbackGmailSyncInput struct {
		TenantID  string  `json:"tenant_id"`
		UserID    string  `json:"user_id"`
		SourceIDs []int64 `json:"source_ids"`
	}
)

// gmailSyncState maintains the internal state of the workflow.
type gmailSyncState struct {
	status                GmailSyncWorkflowStatus
	result                *GmailSyncResult
	paused                bool
	cancelRequested       bool
	cancelReason          string
	processedMessageIDs   []string
	createdSourceIDs      []int64
	uniqueThreads         map[string]bool
	currentSyncToken      string
}

// GmailSyncWorkflow orchestrates the synchronization of Gmail messages.
// It performs the following steps:
// 1. Check Gmail auth token validity
// 2. Refresh token if needed
// 3. Fetch messages (paginated)
// 4. Process each message (create/update/delete)
// 5. Update sync state
//
// This workflow supports both incremental and full sync modes and implements
// the saga pattern with compensation for rollback on failure.
func GmailSyncWorkflow(ctx workflow.Context, input GmailSyncInput) (*GmailSyncResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting Gmail sync workflow",
		"tenant_id", input.TenantID,
		"user_id", input.UserID,
		"job_id", input.JobID,
		"sync_mode", input.SyncMode,
	)

	// Initialize workflow state
	state := &gmailSyncState{
		status: GmailSyncWorkflowStatus{
			Stage:          "initializing",
			StepsCompleted: 0,
			TotalSteps:     5,
			LastActivity:   "",
			StartedAt:      workflow.Now(ctx),
			LastUpdated:    workflow.Now(ctx),
		},
		result: &GmailSyncResult{
			TenantID: input.TenantID,
			UserID:   input.UserID,
		},
		processedMessageIDs: make([]string, 0),
		createdSourceIDs:    make([]int64, 0),
		uniqueThreads:       make(map[string]bool),
		currentSyncToken:    input.LastSyncToken,
	}

	// Register query handler for status
	if err := workflow.SetQueryHandler(ctx, GmailSyncStatusQuery, func() (GmailSyncWorkflowStatus, error) {
		return state.status, nil
	}); err != nil {
		logger.Error("Failed to register status query handler", "error", err)
	}

	// Setup signal channels
	pauseSignalChan := workflow.GetSignalChannel(ctx, GmailSyncPauseSignal)
	cancelSignalChan := workflow.GetSignalChannel(ctx, GmailSyncCancelSignal)

	// Activity option presets
	fastOpts := pkgtemporal.FastActivityOptions()
	batchOpts := pkgtemporal.BatchActivityOptions()

	// Add non-retryable errors
	fastOpts = pkgtemporal.WithNonRetryableErrors(fastOpts, pkgtemporal.NonRetryableErrors()...)
	batchOpts = pkgtemporal.WithNonRetryableErrors(batchOpts, pkgtemporal.NonRetryableErrors()...)

	// Saga compensation stack
	var compensations []func(workflow.Context) error

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
				logger.Info("Received pause/resume signal", "paused", signal.Paused)
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

	// Step 1: Check Gmail auth token
	updateStatus("checking_auth", "CheckGmailAuth")
	var authOutput CheckGmailAuthOutput
	ctx1 := workflow.WithActivityOptions(ctx, fastOpts)
	err := workflow.ExecuteActivity(ctx1, "CheckGmailAuth", CheckGmailAuthInput{
		TenantID: input.TenantID,
		UserID:   input.UserID,
	}).Get(ctx, &authOutput)
	if err != nil {
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("check_auth: %v", err)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Failed to check Gmail auth", "error", err)
		return state.result, nil
	}
	state.status.StepsCompleted = 1

	// Step 2: Refresh token if needed
	if !authOutput.IsValid || authOutput.ExpiresAt.Before(workflow.Now(ctx).Add(5*time.Minute)) {
		updateStatus("refreshing_token", "RefreshGmailToken")
		var refreshOutput RefreshGmailTokenOutput
		ctx2 := workflow.WithActivityOptions(ctx, fastOpts)
		err = workflow.ExecuteActivity(ctx2, "RefreshGmailToken", RefreshGmailTokenInput{
			TenantID: input.TenantID,
			UserID:   input.UserID,
		}).Get(ctx, &refreshOutput)
		if err != nil {
			state.result.Status = "failed"
			state.result.Error = fmt.Sprintf("refresh_token: %v", err)
			state.status.ErrorMessage = state.result.Error
			logger.Error("Failed to refresh Gmail token", "error", err)
			return state.result, nil
		}
		logger.Info("Gmail token refreshed", "expires_at", refreshOutput.ExpiresAt)
	}
	state.status.StepsCompleted = 2

	if checkControls() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after auth check")
		return state.result, nil
	}

	// Step 3: Fetch and process messages in pages
	updateStatus("fetching_messages", "FetchGmailMessages")

	pageToken := ""
	maxMessagesPerPage := 100
	if input.MaxMessages > 0 && input.MaxMessages < maxMessagesPerPage {
		maxMessagesPerPage = input.MaxMessages
	}

	totalProcessed := 0
	for {
		// Fetch a page of messages
		var fetchOutput FetchGmailMessagesOutput
		ctx3 := workflow.WithActivityOptions(ctx, batchOpts)
		err = workflow.ExecuteActivity(ctx3, "FetchGmailMessages", FetchGmailMessagesInput{
			TenantID:     input.TenantID,
			UserID:       input.UserID,
			SyncMode:     input.SyncMode,
			SyncToken:    state.currentSyncToken,
			PageToken:    pageToken,
			MaxResults:   maxMessagesPerPage,
			LabelFilters: input.LabelFilters,
			StartDate:    input.StartDate,
		}).Get(ctx, &fetchOutput)
		if err != nil {
			// Partial failure - save what we have
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
			logger.Error("Failed to fetch Gmail messages", "error", err)
			return state.result, nil
		}

		// Update sync token for next incremental sync
		if fetchOutput.NextSyncToken != "" {
			state.currentSyncToken = fetchOutput.NextSyncToken
		}

		// Process each message
		updateStatus("processing_messages", "ProcessGmailMessage")
		for _, msg := range fetchOutput.Messages {
			if checkControls() {
				// Complete with partial results
				state.result.Status = "partial"
				state.result.Error = "cancelled during processing"
				break
			}

			var processOutput ProcessGmailMessageOutput
			ctx4 := workflow.WithActivityOptions(ctx, fastOpts)
			err = workflow.ExecuteActivity(ctx4, "ProcessGmailMessage", ProcessGmailMessageInput{
				TenantID:       input.TenantID,
				UserID:         input.UserID,
				MessageID:      msg.MessageID,
				ThreadID:       msg.ThreadID,
				IncludeHeaders: input.IncludeHeaders,
			}).Get(ctx, &processOutput)
			if err != nil {
				logger.Warn("Failed to process message", "message_id", msg.MessageID, "error", err)
				continue // Skip failed messages
			}

			state.processedMessageIDs = append(state.processedMessageIDs, msg.MessageID)
			state.uniqueThreads[msg.ThreadID] = true

			switch processOutput.Action {
			case "created":
				state.result.NewMessages++
				state.createdSourceIDs = append(state.createdSourceIDs, processOutput.SourceID)
			case "updated":
				state.result.UpdatedMsgs++
			case "deleted":
				state.result.DeletedMsgs++
			}
			totalProcessed++

			// Check if we've reached max messages
			if input.MaxMessages > 0 && totalProcessed >= input.MaxMessages {
				break
			}
		}

		state.result.MessagesSynced = len(state.processedMessageIDs)
		state.result.ThreadsSynced = len(state.uniqueThreads)

		// Check for early exit conditions
		if state.cancelRequested || !fetchOutput.HasMore ||
			(input.MaxMessages > 0 && totalProcessed >= input.MaxMessages) {
			break
		}

		pageToken = fetchOutput.NextPageToken
	}
	state.status.StepsCompleted = 3

	// Add compensation for rollback
	if len(state.createdSourceIDs) > 0 {
		compensations = append(compensations, func(ctx workflow.Context) error {
			return workflow.ExecuteActivity(
				workflow.WithActivityOptions(ctx, batchOpts),
				"RollbackGmailSync",
				RollbackGmailSyncInput{
					TenantID:  input.TenantID,
					UserID:    input.UserID,
					SourceIDs: state.createdSourceIDs,
				},
			).Get(ctx, nil)
		})
	}

	// Step 4: Update sync state
	updateStatus("updating_sync_state", "UpdateGmailSyncState")
	ctx5 := workflow.WithActivityOptions(ctx, fastOpts)
	err = workflow.ExecuteActivity(ctx5, "UpdateGmailSyncState", UpdateGmailSyncStateInput{
		TenantID:       input.TenantID,
		UserID:         input.UserID,
		SyncToken:      state.currentSyncToken,
		MessagesSynced: state.result.MessagesSynced,
		ThreadsSynced:  state.result.ThreadsSynced,
		SyncedAt:       workflow.Now(ctx),
	}).Get(ctx, nil)
	if err != nil {
		logger.Warn("Failed to update sync state", "error", err)
		// Don't fail the workflow, just log
	}
	state.status.StepsCompleted = 4

	// Finalize result
	state.status.StepsCompleted = 5
	updateStatus("completed", "")
	state.result.NextSyncToken = state.currentSyncToken
	state.result.SyncedAt = workflow.Now(ctx)

	if state.result.Status == "" {
		state.result.Status = "completed"
	}

	logger.Info("Gmail sync workflow completed",
		"tenant_id", input.TenantID,
		"user_id", input.UserID,
		"messages_synced", state.result.MessagesSynced,
		"threads_synced", state.result.ThreadsSynced,
		"new", state.result.NewMessages,
		"updated", state.result.UpdatedMsgs,
		"deleted", state.result.DeletedMsgs,
	)

	return state.result, nil
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
var _ = time.Second
