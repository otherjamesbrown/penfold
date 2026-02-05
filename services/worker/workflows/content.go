// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// ContentIngestionInput is the input for content ingestion workflows.
type ContentIngestionInput = pkgtemporal.ContentIngestionInput

// ContentIngestionResult is the result of content ingestion workflows.
type ContentIngestionResult = pkgtemporal.ContentIngestionResult

// ContentIngestionWorkflowStatus tracks the status of content ingestion.
type ContentIngestionWorkflowStatus = pkgtemporal.WorkflowStatus

// Signal and query names for ContentIngestionWorkflow.
const (
	ContentIngestionStatusQuery    = "content_ingestion_status"
	ContentIngestionPrioritySignal = "content_ingestion_priority"
	ContentIngestionCancelSignal   = "content_ingestion_cancel"
)

// Activity input types for content ingestion.
type (
	// FetchContentInput is the input for the FetchContent activity.
	FetchContentInput struct {
		TenantID string `json:"tenant_id"`
		SourceID int64  `json:"source_id"`
	}

	// FetchContentOutput is the output from the FetchContent activity.
	// Note: Field names must match FetchSourceOutput since the same activity implementation
	// (Activities.FetchSource) is registered under both "FetchContent" and "FetchSource" names.
	FetchContentOutput struct {
		ContentText string `json:"content_text"`
		ContentType string `json:"content_type"`
	}

	// ExtractEntitiesInput is the input for the ExtractEntities activity.
	ExtractEntitiesInput struct {
		TenantID string `json:"tenant_id"`
		SourceID int64  `json:"source_id"`
		// ContentID is the unique content identifier for tracing (format: <type:2>-<base62:8>)
		ContentID string `json:"content_id,omitempty"`
		JobID     string `json:"job_id"`
		Content   string `json:"content"`
	}

	// ExtractTopicsInput is the input for the ExtractTopics activity.
	ExtractTopicsInput struct {
		TenantID string `json:"tenant_id"`
		SourceID int64  `json:"source_id"`
		JobID    string `json:"job_id"`
		Content  string `json:"content"`
	}

	// ExtractMentionsInput is the input for the ExtractMentions activity.
	ExtractMentionsInput struct {
		TenantID    string `json:"tenant_id"`
		SourceID    int64  `json:"source_id"`
		ContentID   int64  `json:"content_id"`
		// ContentTraceID is the unique content identifier for tracing (format: <type:2>-<base62:8>)
		ContentTraceID string `json:"content_trace_id,omitempty"`
		ContentType    string `json:"content_type"`
		Content        string `json:"content"`
		ProjectID      *int64 `json:"project_id,omitempty"`
		Subject        string `json:"subject,omitempty"`
		JobID          string `json:"job_id,omitempty"`
	}

	// ExtractMentionsOutput is the output from the ExtractMentions activity.
	ExtractMentionsOutput struct {
		TraceID          string `json:"trace_id"`
		MentionsFound    int    `json:"mentions_found"`
		AutoResolved     int    `json:"auto_resolved"`
		QueuedForReview  int    `json:"queued_for_review"`
		NewEntities      int    `json:"new_entities_suggested"`
		ProcessingTimeMs int    `json:"processing_time_ms"`
	}

	// ExtractEntitiesOutput is the output from the ExtractEntities activity.
	// This matches the activities.ExtractEntitiesOutput structure for JSON deserialization.
	ExtractEntitiesOutput struct {
		People               []PersonExtracted  `json:"people"`
		Dates                []DateExtracted    `json:"dates"`
		Projects             []string           `json:"projects"`
		Organisations        []string           `json:"organisations"`
		ActionItems          []ActionExtracted  `json:"action_items"`
		Decisions            []string           `json:"decisions"`
		Risks                []string           `json:"risks"`
		QualityGateTriggered bool               `json:"quality_gate_triggered"`
		ModelUsed            string             `json:"model_used"`
	}

	// PersonExtracted represents a person from entity extraction.
	PersonExtracted struct {
		Name string `json:"name"`
		Role string `json:"role,omitempty"`
	}

	// DateExtracted represents a date from entity extraction.
	DateExtracted struct {
		Date    string `json:"date"`
		Context string `json:"context,omitempty"`
	}

	// ActionExtracted represents an action item from entity extraction.
	ActionExtracted struct {
		Assignee string `json:"assignee,omitempty"`
		Action   string `json:"action"`
		Due      string `json:"due,omitempty"`
	}

	// ValidateContentInput is the input for the ValidateContent activity.
	ValidateContentInput struct {
		TenantID string `json:"tenant_id"`
		SourceID int64  `json:"source_id"`
	}

	// ValidateContentOutput is the result of content validation.
	ValidateContentOutput struct {
		Valid           bool   `json:"valid"`
		FailureCategory string `json:"failure_category,omitempty"`
		FailureReason   string `json:"failure_reason,omitempty"`
	}

	// UpdateContentStatusInput is the input for the UpdateContentStatus activity.
	UpdateContentStatusInput struct {
		TenantID        string `json:"tenant_id"`
		SourceID        int64  `json:"source_id"`
		Status          string `json:"status"`
		FailureCategory string `json:"failure_category,omitempty"`
		FailureReason   string `json:"failure_reason,omitempty"`
	}

	// RollbackContentInput is the input for the RollbackContent compensation activity.
	RollbackContentInput struct {
		TenantID    string `json:"tenant_id"`
		SourceID    int64  `json:"source_id"`
		EmbeddingID *int64 `json:"embedding_id,omitempty"`
		SummaryID   *int64 `json:"summary_id,omitempty"`
	}
)

// contentIngestionState maintains the internal state of the workflow.
type contentIngestionState struct {
	status         ContentIngestionWorkflowStatus
	result         *ContentIngestionResult
	paused         bool
	cancelRequested bool
	cancelReason    string
}

// ContentIngestionWorkflow orchestrates the ingestion and processing of content.
// It performs the following steps:
// 1. Fetch content from storage
// 2. Generate embedding (fast, local MLX)
// 3. Generate summary via LLM (slow)
// 4. Extract entities via LLM (slow)
// 5. Extract topics via LLM (slow)
// 6. Extract and resolve mentions via LLM (persons, terms, products, etc.)
// 7. Update content status
//
// This workflow implements the saga pattern with compensation for rollback on failure.
func ContentIngestionWorkflow(ctx workflow.Context, input ContentIngestionInput) (*ContentIngestionResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting content ingestion workflow",
		"source_id", input.SourceID,
		"tenant_id", input.TenantID,
		"source_type", input.SourceType,
		"job_id", input.JobID,
	)

	// Initialize workflow state
	state := &contentIngestionState{
		status: ContentIngestionWorkflowStatus{
			Stage:          "initializing",
			StepsCompleted: 0,
			TotalSteps:     8, // Updated: validation + 7 existing steps
			LastActivity:   "",
			StartedAt:      workflow.Now(ctx),
			LastUpdated:    workflow.Now(ctx),
		},
		result: &ContentIngestionResult{
			SourceID: input.SourceID,
		},
	}

	// Register query handler for status
	if err := workflow.SetQueryHandler(ctx, ContentIngestionStatusQuery, func() (ContentIngestionWorkflowStatus, error) {
		return state.status, nil
	}); err != nil {
		logger.Error("Failed to register status query handler", "error", err)
	}

	// Setup signal channel for priority updates
	prioritySignalChan := workflow.GetSignalChannel(ctx, ContentIngestionPrioritySignal)

	// Setup signal channel for cancellation with compensation
	cancelSignalChan := workflow.GetSignalChannel(ctx, ContentIngestionCancelSignal)

	// Use a selector to handle signals during workflow execution
	selector := workflow.NewSelector(ctx)

	// Handle priority signal
	selector.AddReceive(prioritySignalChan, func(c workflow.ReceiveChannel, more bool) {
		var signal pkgtemporal.PriorityUpdateSignal
		c.Receive(ctx, &signal)
		logger.Info("Received priority update signal", "new_priority", signal.NewPriority, "reason", signal.Reason)
		// Priority is stored but the actual effect depends on activity scheduling
	})

	// Handle cancellation signal
	selector.AddReceive(cancelSignalChan, func(c workflow.ReceiveChannel, more bool) {
		var signal pkgtemporal.CancelWithCompensationSignal
		c.Receive(ctx, &signal)
		logger.Info("Received cancellation signal", "reason", signal.Reason)
		state.cancelRequested = true
		state.cancelReason = signal.Reason
	})

	// Activity option presets
	fastOpts := pkgtemporal.FastActivityOptions()
	embeddingOpts := pkgtemporal.EmbeddingActivityOptions()
	llmOpts := pkgtemporal.LLMActivityOptions()

	// Add non-retryable errors
	fastOpts = pkgtemporal.WithNonRetryableErrors(fastOpts, pkgtemporal.NonRetryableErrors()...)
	embeddingOpts = pkgtemporal.WithNonRetryableErrors(embeddingOpts, pkgtemporal.NonRetryableErrors()...)
	llmOpts = pkgtemporal.WithNonRetryableErrors(llmOpts, pkgtemporal.NonRetryableErrors()...)

	// Saga compensation stack - tracks activities that need rollback
	var compensations []func(workflow.Context) error

	// Helper to check for cancellation
	checkCancellation := func() bool {
		// Drain any pending signals
		for selector.HasPending() {
			selector.Select(ctx)
		}
		return state.cancelRequested
	}

	// Helper to run compensation on failure
	runCompensation := func(ctx workflow.Context) {
		if len(compensations) == 0 {
			return
		}
		logger.Info("Running saga compensation", "compensation_count", len(compensations))
		state.status.CompensationRan = true

		// Run compensations in reverse order
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

	// Step 0: Validate content (fail fast)
	updateStatus("validating", "ValidateContent")
	var validateOutput ValidateContentOutput
	ctx0 := workflow.WithActivityOptions(ctx, fastOpts)
	err := workflow.ExecuteActivity(ctx0, "ValidateContent", ValidateContentInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
	}).Get(ctx, &validateOutput)
	if err != nil {
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("validation_failed: %v", err)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Content validation activity failed", "error", err)
		return state.result, nil
	}
	if !validateOutput.Valid {
		// Update status to "rejected" with failure info
		logger.Info("Content validation failed, marking as rejected",
			"category", validateOutput.FailureCategory,
			"reason", validateOutput.FailureReason,
		)
		ctx0Update := workflow.WithActivityOptions(ctx, fastOpts)
		err = workflow.ExecuteActivity(ctx0Update, "UpdateContentStatus", UpdateContentStatusInput{
			TenantID:        input.TenantID,
			SourceID:        input.SourceID,
			Status:          "rejected",
			FailureCategory: validateOutput.FailureCategory,
			FailureReason:   validateOutput.FailureReason,
		}).Get(ctx, nil)
		if err != nil {
			logger.Warn("Failed to update status to rejected", "error", err)
		}
		state.result.Status = "rejected"
		state.result.Error = fmt.Sprintf("%s: %s", validateOutput.FailureCategory, validateOutput.FailureReason)
		updateStatus("rejected", "")
		return state.result, nil
	}
	state.status.StepsCompleted = 1

	// Step 1: Fetch content
	updateStatus("fetching_content", "FetchContent")
	var fetchOutput FetchContentOutput
	ctx1 := workflow.WithActivityOptions(ctx, fastOpts)
	err = workflow.ExecuteActivity(ctx1, "FetchContent", FetchContentInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
	}).Get(ctx, &fetchOutput)
	if err != nil {
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("fetch_content: %v", err)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Failed to fetch content", "error", err)
		return state.result, nil
	}
	state.status.StepsCompleted = 2

	// Check for cancellation
	if checkCancellation() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after fetch_content")
		return state.result, nil
	}

	// Step 2: Generate embedding (CRITICAL - required for search)
	updateStatus("generating_embedding", "GenerateEmbedding")
	var embeddingID int64
	var embeddingFailed bool
	var embeddingError string
	ctx2 := workflow.WithActivityOptions(ctx, embeddingOpts)
	err = workflow.ExecuteActivity(ctx2, "GenerateContentEmbedding", GenerateEmbeddingInput{
		TenantID:    input.TenantID,
		SourceID:    input.SourceID,
		ContentID:   input.ContentID,
		Content:     fetchOutput.ContentText,
		ContentHash: input.ContentHash,
	}).Get(ctx, &embeddingID)
	if err != nil {
		embeddingFailed = true
		embeddingError = err.Error()
		logger.Error("Embedding generation failed", "error", err)
	} else {
		state.result.EmbeddingID = &embeddingID
		// Add compensation to delete embedding if later steps fail
		compensations = append(compensations, func(ctx workflow.Context) error {
			return workflow.ExecuteActivity(
				workflow.WithActivityOptions(ctx, fastOpts),
				"DeleteEmbedding",
				embeddingID,
			).Get(ctx, nil)
		})
		logger.Debug("Embedding generated", "embedding_id", embeddingID)
	}
	state.status.StepsCompleted = 3

	if checkCancellation() {
		runCompensation(ctx)
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after embedding")
		return state.result, nil
	}

	// Step 3: Generate summary via LLM
	updateStatus("generating_summary", "GenerateSummary")
	var summaryID int64
	ctx3 := workflow.WithActivityOptions(ctx, llmOpts)
	err = workflow.ExecuteActivity(ctx3, "GenerateContentSummary", GenerateSummaryInput{
		TenantID:  input.TenantID,
		SourceID:  input.SourceID,
		ContentID: input.ContentID,
		JobID:     input.JobID,
		Content:   fetchOutput.ContentText,
	}).Get(ctx, &summaryID)
	if err != nil {
		logger.Warn("Summary generation failed, continuing", "error", err)
	} else {
		state.result.SummaryID = &summaryID
		// Add compensation
		compensations = append(compensations, func(ctx workflow.Context) error {
			return workflow.ExecuteActivity(
				workflow.WithActivityOptions(ctx, fastOpts),
				"DeleteSummary",
				summaryID,
			).Get(ctx, nil)
		})
		logger.Debug("Summary generated", "summary_id", summaryID)
	}
	state.status.StepsCompleted = 4

	if checkCancellation() {
		runCompensation(ctx)
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after summary")
		return state.result, nil
	}

	// Step 4: Extract entities via LLM
	updateStatus("extracting_entities", "ExtractEntities")
	var entityOutput *ExtractEntitiesOutput
	ctx4 := workflow.WithActivityOptions(ctx, llmOpts)
	err = workflow.ExecuteActivity(ctx4, "ExtractEntities", ExtractEntitiesInput{
		TenantID:  input.TenantID,
		SourceID:  input.SourceID,
		ContentID: input.ContentID,
		JobID:     input.JobID,
		Content:   fetchOutput.ContentText,
	}).Get(ctx, &entityOutput)
	if err != nil {
		logger.Warn("Entity extraction failed, continuing", "error", err)
	} else if entityOutput != nil {
		// Count total entities extracted
		entityCount := len(entityOutput.People) + len(entityOutput.Dates) +
			len(entityOutput.Projects) + len(entityOutput.Organisations) +
			len(entityOutput.ActionItems) + len(entityOutput.Decisions) +
			len(entityOutput.Risks)
		state.result.EntityCount = entityCount
		logger.Debug("Entities extracted", "count", entityCount)
	}
	state.status.StepsCompleted = 5

	if checkCancellation() {
		runCompensation(ctx)
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after entity extraction")
		return state.result, nil
	}

	// Step 5: Extract topics via LLM
	// TODO: ExtractTopics activity not yet implemented — skip to avoid
	// scheduling a doomed activity that wastes concurrency slots and
	// generates log noise (ActivityNotRegisteredError).
	logger.Debug("Skipping topic extraction (activity not yet implemented)")
	state.status.StepsCompleted = 6

	if checkCancellation() {
		runCompensation(ctx)
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after topic extraction")
		return state.result, nil
	}

	// Step 6: Extract and resolve mentions via LLM
	updateStatus("extracting_mentions", "ExtractMentions")
	var mentionsOutput ExtractMentionsOutput
	ctx6 := workflow.WithActivityOptions(ctx, llmOpts)
	err = workflow.ExecuteActivity(ctx6, "ExtractMentions", ExtractMentionsInput{
		TenantID:       input.TenantID,
		SourceID:       input.SourceID,
		ContentID:      input.SourceID, // Use SourceID as ContentID for now
		ContentTraceID: input.ContentID,
		ContentType:    input.SourceType,
		Content:        fetchOutput.ContentText,
		JobID:          input.JobID,
	}).Get(ctx, &mentionsOutput)
	if err != nil {
		logger.Warn("Mention extraction failed, continuing", "error", err)
	} else {
		state.result.MentionCount = mentionsOutput.MentionsFound
		logger.Debug("Mentions extracted",
			"found", mentionsOutput.MentionsFound,
			"auto_resolved", mentionsOutput.AutoResolved,
			"queued", mentionsOutput.QueuedForReview,
		)
	}
	state.status.StepsCompleted = 7

	if checkCancellation() {
		runCompensation(ctx)
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after mention extraction")
		return state.result, nil
	}

	// Step 7: Update content status
	// Embedding is critical - without it, the content can't be searched.
	// If embedding failed, mark the source as failed so it can be retried.
	updateStatus("updating_status", "UpdateContentStatus")
	ctx7 := workflow.WithActivityOptions(ctx, fastOpts)

	var finalStatus string
	if embeddingFailed {
		finalStatus = "failed"
		state.result.Status = "failed"
		state.result.Error = fmt.Sprintf("embedding_failed: %s", embeddingError)
		state.status.ErrorMessage = state.result.Error
		logger.Error("Content ingestion failed - embedding is required for search",
			"source_id", input.SourceID,
			"error", embeddingError,
		)
	} else {
		finalStatus = "completed"
		state.result.Status = "completed"
		logger.Info("Content ingestion workflow completed",
			"source_id", input.SourceID,
			"embedding_id", state.result.EmbeddingID,
			"summary_id", state.result.SummaryID,
			"entity_count", state.result.EntityCount,
			"topic_count", len(state.result.ExtractedTopics),
			"mention_count", state.result.MentionCount,
		)
	}

	err = workflow.ExecuteActivity(ctx7, "UpdateContentStatus", UpdateContentStatusInput{
		TenantID:        input.TenantID,
		SourceID:        input.SourceID,
		Status:          finalStatus,
		FailureCategory: "", // No failure for completed/failed status
		FailureReason:   "", // Error is in state.result.Error
	}).Get(ctx, nil)
	if err != nil {
		logger.Warn("Status update failed", "error", err)
	}
	state.status.StepsCompleted = 8

	updateStatus(finalStatus, "")
	return state.result, nil
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
var _ = time.Second
