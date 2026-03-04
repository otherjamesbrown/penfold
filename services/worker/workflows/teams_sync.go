// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// TeamsSyncInput is the input for Teams synchronization workflows.
type TeamsSyncInput = pkgtemporal.TeamsSyncInput

// TeamsSyncResult is the result of Teams synchronization workflows.
type TeamsSyncResult = pkgtemporal.TeamsSyncResult

// TeamsSyncWorkflowStatus tracks the status of Teams synchronization.
type TeamsSyncWorkflowStatus = pkgtemporal.WorkflowStatus

// Signal and query names for TeamsSyncWorkflow.
const (
	TeamsSyncStatusQuery  = "teams_sync_status"
	TeamsSyncPauseSignal  = "teams_sync_pause"
	TeamsSyncCancelSignal = "teams_sync_cancel"
)

// Activity input types for Teams synchronization.
type (
	// CheckGraphAuthInput is the input for the CheckGraphAuth activity.
	CheckGraphAuthInput struct {
		TenantID      string `json:"tenant_id"`
		IntegrationID int64  `json:"integration_id"`
	}

	// CheckGraphAuthOutput is the output from the CheckGraphAuth activity.
	CheckGraphAuthOutput struct {
		IsValid   bool      `json:"is_valid"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	// FetchTeamChannelsInput is the input for the FetchTeamChannels activity.
	FetchTeamChannelsInput struct {
		TenantID      string   `json:"tenant_id"`
		IntegrationID int64    `json:"integration_id"`
		TeamIDs       []string `json:"team_ids"`
		ChannelIDs    []string `json:"channel_ids,omitempty"` // empty = all
	}

	// TeamChannelInfo contains channel details with its parent team.
	TeamChannelInfo struct {
		TeamID      string `json:"team_id"`
		ChannelID   string `json:"channel_id"`
		ChannelName string `json:"channel_name"`
		ProjectID   string `json:"project_id,omitempty"` // From channel→project mapping
	}

	// FetchTeamChannelsOutput is the output from the FetchTeamChannels activity.
	FetchTeamChannelsOutput struct {
		Channels []TeamChannelInfo `json:"channels"`
	}

	// FetchChannelMessagesInput is the input for the FetchChannelMessages activity.
	FetchChannelMessagesInput struct {
		TenantID      string `json:"tenant_id"`
		IntegrationID int64  `json:"integration_id"`
		TeamID        string `json:"team_id"`
		ChannelID     string `json:"channel_id"`
		DeltaToken    string `json:"delta_token,omitempty"`
		MaxMessages   int    `json:"max_messages"`
	}

	// FetchChannelMessagesOutput is the output from the FetchChannelMessages activity.
	FetchChannelMessagesOutput struct {
		// ThreadIDs contains the IDs of root messages (threads) found.
		ThreadIDs     []string `json:"thread_ids"`
		DeltaToken    string   `json:"delta_token,omitempty"`
		MessageCount  int      `json:"message_count"`
	}

	// ProcessTeamsThreadInput is the input for the ProcessTeamsThread activity.
	ProcessTeamsThreadInput struct {
		TenantID      string `json:"tenant_id"`
		IntegrationID int64  `json:"integration_id"`
		TeamID        string `json:"team_id"`
		ChannelID     string `json:"channel_id"`
		ChannelName   string `json:"channel_name"`
		MessageID     string `json:"message_id"`
		ProjectID     string `json:"project_id,omitempty"`
	}

	// ProcessTeamsThreadOutput is the output from the ProcessTeamsThread activity.
	ProcessTeamsThreadOutput struct {
		SourceID     int64  `json:"source_id"`
		ContentID    string `json:"content_id,omitempty"`
		Action       string `json:"action"` // created, updated, skipped
		ReplyCount   int    `json:"reply_count"`
	}

	// UpdateTeamsSyncStateInput is the input for the UpdateTeamsSyncState activity.
	UpdateTeamsSyncStateInput struct {
		TenantID      string            `json:"tenant_id"`
		IntegrationID int64             `json:"integration_id"`
		DeltaTokens   map[string]string `json:"delta_tokens"` // channelID → deltaToken
		ChannelsSynced int              `json:"channels_synced"`
		ThreadsSynced  int              `json:"threads_synced"`
		SyncedAt       time.Time        `json:"synced_at"`
	}

	// RollbackTeamsSyncInput is the input for the RollbackTeamsSync compensation activity.
	RollbackTeamsSyncInput struct {
		TenantID  string  `json:"tenant_id"`
		SourceIDs []int64 `json:"source_ids"`
	}
)

// teamsSyncState maintains the internal state of the workflow.
type teamsSyncState struct {
	status           TeamsSyncWorkflowStatus
	result           *TeamsSyncResult
	paused           bool
	cancelRequested  bool
	cancelReason     string
	createdSourceIDs []int64
	deltaTokens      map[string]string // channelID → deltaToken
}

// TeamsSyncWorkflow orchestrates the synchronization of Teams channel messages.
// It performs the following steps:
// 1. Check Graph API auth token validity
// 2. Fetch channels for configured teams (with project mapping)
// 3. For each channel, fetch messages (delta or full)
// 4. For each thread (root + replies), parse and create source
// 5. Update sync state (delta tokens per channel)
func TeamsSyncWorkflow(ctx workflow.Context, input TeamsSyncInput) (*TeamsSyncResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting Teams sync workflow",
		"tenant_id", input.TenantID,
		"integration_id", input.IntegrationID,
		"job_id", input.JobID,
		"sync_mode", input.SyncMode,
		"team_count", len(input.TeamIDs),
	)

	// Initialize workflow state
	state := &teamsSyncState{
		status: TeamsSyncWorkflowStatus{
			Stage:          "initializing",
			StepsCompleted: 0,
			TotalSteps:     5,
			LastActivity:   "",
			StartedAt:      workflow.Now(ctx),
			LastUpdated:    workflow.Now(ctx),
		},
		result: &TeamsSyncResult{
			TenantID: input.TenantID,
		},
		createdSourceIDs: make([]int64, 0),
		deltaTokens:      make(map[string]string),
	}

	// Register query handler for status
	if err := workflow.SetQueryHandler(ctx, TeamsSyncStatusQuery, func() (TeamsSyncWorkflowStatus, error) {
		return state.status, nil
	}); err != nil {
		logger.Error("Failed to register status query handler", "error", err)
	}

	// Setup signal channels
	pauseSignalChan := workflow.GetSignalChannel(ctx, TeamsSyncPauseSignal)
	cancelSignalChan := workflow.GetSignalChannel(ctx, TeamsSyncCancelSignal)

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

	// Step 2: Fetch channels for configured teams
	updateStatus("fetching_channels", "FetchTeamChannels")
	var channelsOutput FetchTeamChannelsOutput
	ctx2 := workflow.WithActivityOptions(ctx, fastOpts)
	err = workflow.ExecuteActivity(ctx2, "FetchTeamChannels", FetchTeamChannelsInput{
		TenantID:      input.TenantID,
		IntegrationID: input.IntegrationID,
		TeamIDs:       input.TeamIDs,
		ChannelIDs:    input.ChannelIDs,
	}).Get(ctx, &channelsOutput)
	if err != nil {
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("fetch_channels: %v", err)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Failed to fetch channels", "error", err)
		return state.result, nil
	}
	state.status.StepsCompleted = 2

	if len(channelsOutput.Channels) == 0 {
		state.result.Status = "completed"
		state.result.SyncedAt = workflow.Now(ctx)
		logger.Info("No channels to sync")
		return state.result, nil
	}

	// Step 3: Fetch and process messages per channel
	updateStatus("syncing_messages", "FetchChannelMessages")

	for _, ch := range channelsOutput.Channels {
		if checkControls() {
			state.result.Status = "partial"
			state.result.Error = "cancelled during sync"
			break
		}

		// Fetch messages for this channel
		var messagesOutput FetchChannelMessagesOutput
		ctx3 := workflow.WithActivityOptions(ctx, batchOpts)
		err = workflow.ExecuteActivity(ctx3, "FetchChannelMessages", FetchChannelMessagesInput{
			TenantID:      input.TenantID,
			IntegrationID: input.IntegrationID,
			TeamID:        ch.TeamID,
			ChannelID:     ch.ChannelID,
			MaxMessages:   50,
		}).Get(ctx, &messagesOutput)
		if err != nil {
			logger.Warn("Failed to fetch messages for channel",
				"channel_id", ch.ChannelID,
				"error", err,
			)
			continue
		}

		// Save delta token for this channel
		if messagesOutput.DeltaToken != "" {
			state.deltaTokens[ch.ChannelID] = messagesOutput.DeltaToken
		}

		state.result.MessagesSynced += messagesOutput.MessageCount

		// Process each thread (root message)
		updateStatus("processing_threads", "ProcessTeamsThread")
		for _, threadID := range messagesOutput.ThreadIDs {
			if checkControls() {
				break
			}

			var threadOutput ProcessTeamsThreadOutput
			ctx4 := workflow.WithActivityOptions(ctx, fastOpts)
			err = workflow.ExecuteActivity(ctx4, "ProcessTeamsThread", ProcessTeamsThreadInput{
				TenantID:      input.TenantID,
				IntegrationID: input.IntegrationID,
				TeamID:        ch.TeamID,
				ChannelID:     ch.ChannelID,
				ChannelName:   ch.ChannelName,
				MessageID:     threadID,
				ProjectID:     ch.ProjectID,
			}).Get(ctx, &threadOutput)
			if err != nil {
				logger.Warn("Failed to process thread",
					"message_id", threadID,
					"error", err,
				)
				continue
			}

			switch threadOutput.Action {
			case "created":
				state.result.NewThreads++
				state.createdSourceIDs = append(state.createdSourceIDs, threadOutput.SourceID)
			case "updated":
				state.result.UpdatedThreads++
			}
			state.result.ThreadsSynced++
		}

		state.result.ChannelsSynced++
	}
	state.status.StepsCompleted = 3

	// Add compensation for rollback
	if len(state.createdSourceIDs) > 0 {
		compensations = append(compensations, func(ctx workflow.Context) error {
			return workflow.ExecuteActivity(
				workflow.WithActivityOptions(ctx, batchOpts),
				"RollbackTeamsSync",
				RollbackTeamsSyncInput{
					TenantID:  input.TenantID,
					SourceIDs: state.createdSourceIDs,
				},
			).Get(ctx, nil)
		})
	}

	// Step 4: Update sync state (delta tokens)
	updateStatus("updating_sync_state", "UpdateTeamsSyncState")
	ctx5 := workflow.WithActivityOptions(ctx, fastOpts)
	err = workflow.ExecuteActivity(ctx5, "UpdateTeamsSyncState", UpdateTeamsSyncStateInput{
		TenantID:       input.TenantID,
		IntegrationID:  input.IntegrationID,
		DeltaTokens:    state.deltaTokens,
		ChannelsSynced: state.result.ChannelsSynced,
		ThreadsSynced:  state.result.ThreadsSynced,
		SyncedAt:       workflow.Now(ctx),
	}).Get(ctx, nil)
	if err != nil {
		logger.Warn("Failed to update sync state", "error", err)
		// Don't fail the workflow, just log
	}
	state.status.StepsCompleted = 4

	// Finalize
	state.status.StepsCompleted = 5
	updateStatus("completed", "")
	state.result.SyncedAt = workflow.Now(ctx)

	if state.result.Status == "" {
		state.result.Status = "completed"
	}

	logger.Info("Teams sync workflow completed",
		"tenant_id", input.TenantID,
		"channels_synced", state.result.ChannelsSynced,
		"threads_synced", state.result.ThreadsSynced,
		"messages_synced", state.result.MessagesSynced,
		"new_threads", state.result.NewThreads,
		"updated_threads", state.result.UpdatedThreads,
	)

	return state.result, nil
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
var _ = time.Second
