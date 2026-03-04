// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/otherjamesbrown/penfold/pkg/graph"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// ADSyncInput is the input for Azure AD directory synchronization workflows.
type ADSyncInput = pkgtemporal.ADSyncInput

// ADSyncResult is the result of Azure AD directory synchronization workflows.
type ADSyncResult = pkgtemporal.ADSyncResult

// ADSyncWorkflowStatus tracks the status of AD synchronization.
type ADSyncWorkflowStatus = pkgtemporal.WorkflowStatus

// Signal and query names for ADSyncWorkflow.
const (
	ADSyncStatusQuery  = "ad_sync_status"
	ADSyncPauseSignal  = "ad_sync_pause"
	ADSyncCancelSignal = "ad_sync_cancel"
)

// Activity input types for AD synchronization.
type (
	// FetchADUsersInput is the input for the FetchADUsers activity.
	FetchADUsersInput struct {
		TenantID      string `json:"tenant_id"`
		IntegrationID int64  `json:"integration_id"`
	}

	// FetchADUsersOutput is the result of fetching AD users.
	FetchADUsersOutput struct {
		Users     []graph.ADUser `json:"users"`
		UserCount int            `json:"user_count"`
	}

	// SyncPeopleFromADInput is the input for the SyncPeopleFromAD activity.
	SyncPeopleFromADInput struct {
		TenantID string         `json:"tenant_id"`
		Users    []graph.ADUser `json:"users"`
	}

	// SyncPeopleFromADOutput is the result of syncing people from AD.
	SyncPeopleFromADOutput struct {
		Created     int `json:"created"`
		Updated     int `json:"updated"`
		Skipped     int `json:"skipped"`
		ManagersSet int `json:"managers_set"`
	}

	// FetchADGroupsInput is the input for the FetchADGroups activity.
	FetchADGroupsInput struct {
		TenantID      string `json:"tenant_id"`
		IntegrationID int64  `json:"integration_id"`
	}

	// FetchADGroupsOutput is the result of fetching AD groups.
	FetchADGroupsOutput struct {
		Groups     []graph.ADGroup `json:"groups"`
		GroupCount int             `json:"group_count"`
	}

	// SyncTeamsFromADInput is the input for the SyncTeamsFromAD activity.
	SyncTeamsFromADInput struct {
		TenantID string          `json:"tenant_id"`
		Groups   []graph.ADGroup `json:"groups"`
	}

	// SyncTeamsFromADOutput is the result of syncing teams from AD.
	SyncTeamsFromADOutput struct {
		TeamsCreated   int `json:"teams_created"`
		TeamsUpdated   int `json:"teams_updated"`
		MembersAdded   int `json:"members_added"`
		MembersRemoved int `json:"members_removed"`
	}

	// UpdateADSyncStateInput is the input for the UpdateADSyncState activity.
	UpdateADSyncStateInput struct {
		TenantID      string    `json:"tenant_id"`
		IntegrationID int64     `json:"integration_id"`
		UsersSynced   int       `json:"users_synced"`
		GroupsSynced  int       `json:"groups_synced"`
		SyncedAt      time.Time `json:"synced_at"`
	}
)

// adSyncState maintains the internal state of the workflow.
type adSyncState struct {
	status          ADSyncWorkflowStatus
	result          *ADSyncResult
	paused          bool
	cancelRequested bool
	cancelReason    string
}

// ADSyncWorkflow orchestrates the synchronization of Azure AD users and groups
// into the existing people and teams tables.
//
// Steps:
// 1. Check Graph API auth token validity
// 2. Fetch AD users and sync into people table
// 3. (Optional) Fetch AD groups and sync into teams/team_members
// 4. Update sync state
func ADSyncWorkflow(ctx workflow.Context, input ADSyncInput) (*ADSyncResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting AD sync workflow",
		"tenant_id", input.TenantID,
		"integration_id", input.IntegrationID,
		"job_id", input.JobID,
		"sync_groups", input.SyncGroups,
		"sync_hierarchy", input.SyncHierarchy,
	)

	totalSteps := 3 // auth + users + update
	if input.SyncGroups {
		totalSteps = 4 // + groups
	}

	// Initialize workflow state
	state := &adSyncState{
		status: ADSyncWorkflowStatus{
			Stage:          "initializing",
			StepsCompleted: 0,
			TotalSteps:     totalSteps,
			LastActivity:   "",
			StartedAt:      workflow.Now(ctx),
			LastUpdated:    workflow.Now(ctx),
		},
		result: &ADSyncResult{
			TenantID: input.TenantID,
		},
	}

	// Register query handler for status
	if err := workflow.SetQueryHandler(ctx, ADSyncStatusQuery, func() (ADSyncWorkflowStatus, error) {
		return state.status, nil
	}); err != nil {
		logger.Error("Failed to register status query handler", "error", err)
	}

	// Setup signal channels
	pauseSignalChan := workflow.GetSignalChannel(ctx, ADSyncPauseSignal)
	cancelSignalChan := workflow.GetSignalChannel(ctx, ADSyncCancelSignal)

	// Activity option presets
	fastOpts := pkgtemporal.FastActivityOptions()
	batchOpts := pkgtemporal.BatchActivityOptions()

	fastOpts = pkgtemporal.WithNonRetryableErrors(fastOpts, pkgtemporal.NonRetryableErrors()...)
	batchOpts = pkgtemporal.WithNonRetryableErrors(batchOpts, pkgtemporal.NonRetryableErrors()...)

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
		return state.result, nil
	}

	// Step 2: Fetch AD users and sync into people table
	updateStatus("fetching_users", "FetchADUsers")
	var fetchUsersOutput FetchADUsersOutput
	ctx2 := workflow.WithActivityOptions(ctx, batchOpts)
	err = workflow.ExecuteActivity(ctx2, "FetchADUsers", FetchADUsersInput{
		TenantID:      input.TenantID,
		IntegrationID: input.IntegrationID,
	}).Get(ctx, &fetchUsersOutput)
	if err != nil {
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("fetch_users: %v", err)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Failed to fetch AD users", "error", err)
		return state.result, nil
	}

	updateStatus("syncing_people", "SyncPeopleFromAD")
	var syncPeopleOutput SyncPeopleFromADOutput
	ctx3 := workflow.WithActivityOptions(ctx, batchOpts)
	err = workflow.ExecuteActivity(ctx3, "SyncPeopleFromAD", SyncPeopleFromADInput{
		TenantID: input.TenantID,
		Users:    fetchUsersOutput.Users,
	}).Get(ctx, &syncPeopleOutput)
	if err != nil {
		state.result.Status = "partial"
		state.result.Error = fmt.Sprintf("sync_people: %v", err)
		logger.Warn("Failed to sync people from AD", "error", err)
		// Continue to update sync state
	} else {
		state.result.UsersSynced = syncPeopleOutput.Created + syncPeopleOutput.Updated
		state.result.UsersCreated = syncPeopleOutput.Created
		state.result.UsersUpdated = syncPeopleOutput.Updated
		state.result.ManagersSet = syncPeopleOutput.ManagersSet
	}
	state.status.StepsCompleted = 2

	if checkControls() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		return state.result, nil
	}

	// Step 3 (optional): Fetch AD groups and sync into teams
	if input.SyncGroups {
		updateStatus("fetching_groups", "FetchADGroups")
		var fetchGroupsOutput FetchADGroupsOutput
		ctx4 := workflow.WithActivityOptions(ctx, batchOpts)
		err = workflow.ExecuteActivity(ctx4, "FetchADGroups", FetchADGroupsInput{
			TenantID:      input.TenantID,
			IntegrationID: input.IntegrationID,
		}).Get(ctx, &fetchGroupsOutput)
		if err != nil {
			logger.Warn("Failed to fetch AD groups", "error", err)
			// Non-fatal: we already synced users
		} else {
			updateStatus("syncing_teams", "SyncTeamsFromAD")
			var syncTeamsOutput SyncTeamsFromADOutput
			ctx5 := workflow.WithActivityOptions(ctx, batchOpts)
			err = workflow.ExecuteActivity(ctx5, "SyncTeamsFromAD", SyncTeamsFromADInput{
				TenantID: input.TenantID,
				Groups:   fetchGroupsOutput.Groups,
			}).Get(ctx, &syncTeamsOutput)
			if err != nil {
				logger.Warn("Failed to sync teams from AD", "error", err)
			} else {
				state.result.GroupsSynced = syncTeamsOutput.TeamsCreated + syncTeamsOutput.TeamsUpdated
				state.result.MembersSynced = syncTeamsOutput.MembersAdded
			}
		}
		state.status.StepsCompleted = 3
	}

	// Final step: Update sync state
	updateStatus("updating_sync_state", "UpdateADSyncState")
	ctxFinal := workflow.WithActivityOptions(ctx, fastOpts)
	err = workflow.ExecuteActivity(ctxFinal, "UpdateADSyncState", UpdateADSyncStateInput{
		TenantID:      input.TenantID,
		IntegrationID: input.IntegrationID,
		UsersSynced:   state.result.UsersSynced,
		GroupsSynced:  state.result.GroupsSynced,
		SyncedAt:      workflow.Now(ctx),
	}).Get(ctx, nil)
	if err != nil {
		logger.Warn("Failed to update AD sync state", "error", err)
	}
	state.status.StepsCompleted = totalSteps

	// Finalize result
	updateStatus("completed", "")
	state.result.SyncedAt = workflow.Now(ctx)

	if state.result.Status == "" {
		state.result.Status = "completed"
	}

	logger.Info("AD sync workflow completed",
		"tenant_id", input.TenantID,
		"users_synced", state.result.UsersSynced,
		"users_created", state.result.UsersCreated,
		"users_updated", state.result.UsersUpdated,
		"groups_synced", state.result.GroupsSynced,
		"members_synced", state.result.MembersSynced,
		"managers_set", state.result.ManagersSet,
	)

	return state.result, nil
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
var _ = time.Second
