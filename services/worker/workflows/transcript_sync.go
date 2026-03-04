package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// TranscriptSyncInput is the input for transcript synchronization workflows.
type TranscriptSyncInput = pkgtemporal.TranscriptSyncInput

// TranscriptSyncResult is the result of transcript synchronization workflows.
type TranscriptSyncResult = pkgtemporal.TranscriptSyncResult

// TranscriptSyncWorkflowStatus tracks the status of transcript synchronization.
type TranscriptSyncWorkflowStatus = pkgtemporal.WorkflowStatus

// Signal and query names for TranscriptSyncWorkflow.
const (
	TranscriptSyncStatusQuery  = "transcript_sync_status"
	TranscriptSyncPauseSignal  = "transcript_sync_pause"
	TranscriptSyncCancelSignal = "transcript_sync_cancel"
)

// Activity input types for transcript synchronization.
type (
	// FetchMeetingTranscriptsInput is the input for the FetchMeetingTranscripts activity.
	FetchMeetingTranscriptsInput struct {
		TenantID      string `json:"tenant_id"`
		IntegrationID int64  `json:"integration_id"`
		UserID        string `json:"user_id"`
		SinceDate     string `json:"since_date"` // ISO8601
	}

	// TranscriptInfo contains metadata about a meeting transcript.
	TranscriptInfo struct {
		MeetingID      string    `json:"meeting_id"`
		TranscriptID   string    `json:"transcript_id"`
		MeetingSubject string    `json:"meeting_subject,omitempty"`
		StartDateTime  time.Time `json:"start_date_time"`
		EndDateTime    time.Time `json:"end_date_time"`
		OrganizerName  string    `json:"organizer_name,omitempty"`
	}

	// FetchMeetingTranscriptsOutput is the result of fetching meeting transcripts.
	FetchMeetingTranscriptsOutput struct {
		Transcripts     []TranscriptInfo `json:"transcripts"`
		MeetingsChecked int              `json:"meetings_checked"`
	}

	// ProcessTranscriptContentInput is the input for the ProcessTranscriptContent activity.
	ProcessTranscriptContentInput struct {
		TenantID       string `json:"tenant_id"`
		IntegrationID  int64  `json:"integration_id"`
		UserID         string `json:"user_id"`
		MeetingID      string `json:"meeting_id"`
		TranscriptID   string `json:"transcript_id"`
		MeetingSubject string `json:"meeting_subject,omitempty"`
		StartDateTime  string `json:"start_date_time,omitempty"`
		EndDateTime    string `json:"end_date_time,omitempty"`
		OrganizerName  string `json:"organizer_name,omitempty"`
	}

	// ProcessTranscriptContentOutput is the result of processing a single transcript.
	ProcessTranscriptContentOutput struct {
		SourceID  int64    `json:"source_id"`
		ContentID string   `json:"content_id,omitempty"`
		Action    string   `json:"action"` // created, skipped
		Speakers  []string `json:"speakers,omitempty"`
	}

	// UpdateTranscriptSyncStateInput is the input for the UpdateTranscriptSyncState activity.
	UpdateTranscriptSyncStateInput struct {
		TenantID          string    `json:"tenant_id"`
		IntegrationID     int64     `json:"integration_id"`
		TranscriptsSynced int       `json:"transcripts_synced"`
		SyncedAt          time.Time `json:"synced_at"`
	}

	// RollbackTranscriptSyncInput is the input for the RollbackTranscriptSync compensation activity.
	RollbackTranscriptSyncInput struct {
		TenantID  string  `json:"tenant_id"`
		SourceIDs []int64 `json:"source_ids"`
	}
)

// transcriptSyncState maintains the internal state of the workflow.
type transcriptSyncState struct {
	status           TranscriptSyncWorkflowStatus
	result           *TranscriptSyncResult
	paused           bool
	cancelRequested  bool
	cancelReason     string
	createdSourceIDs []int64
}

// TranscriptSyncWorkflow orchestrates the synchronization of Teams meeting transcripts.
// Steps:
// 1. Check Graph API auth token validity
// 2. Fetch meetings with transcripts (poll-based discovery)
// 3. For each transcript, download WebVTT content, parse, and create source
// 4. Update sync state
func TranscriptSyncWorkflow(ctx workflow.Context, input TranscriptSyncInput) (*TranscriptSyncResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting transcript sync workflow",
		"tenant_id", input.TenantID,
		"integration_id", input.IntegrationID,
		"user_id", input.UserID,
		"job_id", input.JobID,
		"sync_mode", input.SyncMode,
	)

	// Initialize workflow state
	state := &transcriptSyncState{
		status: TranscriptSyncWorkflowStatus{
			Stage:          "initializing",
			StepsCompleted: 0,
			TotalSteps:     4,
			LastActivity:   "",
			StartedAt:      workflow.Now(ctx),
			LastUpdated:    workflow.Now(ctx),
		},
		result: &TranscriptSyncResult{
			TenantID: input.TenantID,
		},
		createdSourceIDs: make([]int64, 0),
	}

	// Register query handler for status
	if err := workflow.SetQueryHandler(ctx, TranscriptSyncStatusQuery, func() (TranscriptSyncWorkflowStatus, error) {
		return state.status, nil
	}); err != nil {
		logger.Error("Failed to register status query handler", "error", err)
	}

	// Setup signal channels
	pauseSignalChan := workflow.GetSignalChannel(ctx, TranscriptSyncPauseSignal)
	cancelSignalChan := workflow.GetSignalChannel(ctx, TranscriptSyncCancelSignal)

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
	updateStatus := func(stage, activityName string) {
		state.status.Stage = stage
		state.status.LastActivity = activityName
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

	// Step 2: Fetch meetings with transcripts
	updateStatus("fetching_transcripts", "FetchMeetingTranscripts")

	sinceDate := workflow.Now(ctx).Add(-30 * 24 * time.Hour).Format(time.RFC3339)

	var fetchOutput FetchMeetingTranscriptsOutput
	ctx2 := workflow.WithActivityOptions(ctx, batchOpts)
	err = workflow.ExecuteActivity(ctx2, "FetchMeetingTranscripts", FetchMeetingTranscriptsInput{
		TenantID:      input.TenantID,
		IntegrationID: input.IntegrationID,
		UserID:        input.UserID,
		SinceDate:     sinceDate,
	}).Get(ctx, &fetchOutput)
	if err != nil {
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("fetch_transcripts: %v", err)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Failed to fetch meeting transcripts", "error", err)
		return state.result, nil
	}
	state.result.MeetingsChecked = fetchOutput.MeetingsChecked
	state.status.StepsCompleted = 2

	if len(fetchOutput.Transcripts) == 0 {
		state.result.Status = "completed"
		state.result.SyncedAt = workflow.Now(ctx)
		logger.Info("No transcripts to sync")
		return state.result, nil
	}

	// Step 3: Process each transcript
	updateStatus("processing_transcripts", "ProcessTranscriptContent")

	for _, t := range fetchOutput.Transcripts {
		if checkControls() {
			state.result.Status = "partial"
			state.result.Error = "cancelled during processing"
			break
		}

		var processOutput ProcessTranscriptContentOutput
		ctx3 := workflow.WithActivityOptions(ctx, fastOpts)
		err = workflow.ExecuteActivity(ctx3, "ProcessTranscriptContent", ProcessTranscriptContentInput{
			TenantID:       input.TenantID,
			IntegrationID:  input.IntegrationID,
			UserID:         input.UserID,
			MeetingID:      t.MeetingID,
			TranscriptID:   t.TranscriptID,
			MeetingSubject: t.MeetingSubject,
			StartDateTime:  t.StartDateTime.Format(time.RFC3339),
			EndDateTime:    t.EndDateTime.Format(time.RFC3339),
			OrganizerName:  t.OrganizerName,
		}).Get(ctx, &processOutput)
		if err != nil {
			logger.Warn("Failed to process transcript",
				"meeting_id", t.MeetingID,
				"transcript_id", t.TranscriptID,
				"error", err,
			)
			continue
		}

		switch processOutput.Action {
		case "created":
			state.result.NewTranscripts++
			state.createdSourceIDs = append(state.createdSourceIDs, processOutput.SourceID)
		case "skipped":
			state.result.SkippedTranscripts++
		}
		state.result.TranscriptsSynced++
	}
	state.status.StepsCompleted = 3

	// Add compensation for rollback
	if len(state.createdSourceIDs) > 0 {
		compensations = append(compensations, func(ctx workflow.Context) error {
			return workflow.ExecuteActivity(
				workflow.WithActivityOptions(ctx, batchOpts),
				"RollbackTranscriptSync",
				RollbackTranscriptSyncInput{
					TenantID:  input.TenantID,
					SourceIDs: state.createdSourceIDs,
				},
			).Get(ctx, nil)
		})
	}

	// Step 4: Update sync state
	updateStatus("updating_sync_state", "UpdateTranscriptSyncState")
	ctx4 := workflow.WithActivityOptions(ctx, fastOpts)
	err = workflow.ExecuteActivity(ctx4, "UpdateTranscriptSyncState", UpdateTranscriptSyncStateInput{
		TenantID:          input.TenantID,
		IntegrationID:     input.IntegrationID,
		TranscriptsSynced: state.result.TranscriptsSynced,
		SyncedAt:          workflow.Now(ctx),
	}).Get(ctx, nil)
	if err != nil {
		logger.Warn("Failed to update transcript sync state", "error", err)
	}
	state.status.StepsCompleted = 4

	// Finalize result
	updateStatus("completed", "")
	state.result.SyncedAt = workflow.Now(ctx)

	if state.result.Status == "" {
		state.result.Status = "completed"
	}

	logger.Info("Transcript sync workflow completed",
		"tenant_id", input.TenantID,
		"meetings_checked", state.result.MeetingsChecked,
		"transcripts_synced", state.result.TranscriptsSynced,
		"new_transcripts", state.result.NewTranscripts,
		"skipped", state.result.SkippedTranscripts,
	)

	return state.result, nil
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
var _ = time.Second
