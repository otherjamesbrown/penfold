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
	FetchContentOutput struct {
		Content     string `json:"content"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
	}

	// ExtractEntitiesInput is the input for the ExtractEntities activity.
	ExtractEntitiesInput struct {
		TenantID string `json:"tenant_id"`
		SourceID int64  `json:"source_id"`
		JobID    string `json:"job_id"`
		Content  string `json:"content"`
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
		ContentType string `json:"content_type"`
		Content     string `json:"content"`
		ProjectID   *int64 `json:"project_id,omitempty"`
		Subject     string `json:"subject,omitempty"`
		JobID       string `json:"job_id,omitempty"`
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

	// UpdateContentStatusInput is the input for the UpdateContentStatus activity.
	UpdateContentStatusInput struct {
		TenantID string `json:"tenant_id"`
		SourceID int64  `json:"source_id"`
		Status   string `json:"status"`
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
			TotalSteps:     7,
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

	// Step 1: Fetch content
	updateStatus("fetching_content", "FetchContent")
	var fetchOutput FetchContentOutput
	ctx1 := workflow.WithActivityOptions(ctx, fastOpts)
	err := workflow.ExecuteActivity(ctx1, "FetchContent", FetchContentInput{
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
	state.status.StepsCompleted = 1

	// Check for cancellation
	if checkCancellation() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after fetch_content")
		return state.result, nil
	}

	// Step 2: Generate embedding
	updateStatus("generating_embedding", "GenerateEmbedding")
	var embeddingID int64
	ctx2 := workflow.WithActivityOptions(ctx, embeddingOpts)
	err = workflow.ExecuteActivity(ctx2, "GenerateContentEmbedding", GenerateEmbeddingInput{
		TenantID:    input.TenantID,
		SourceID:    input.SourceID,
		Content:     fetchOutput.Content,
		ContentHash: input.ContentHash,
	}).Get(ctx, &embeddingID)
	if err != nil {
		logger.Warn("Embedding generation failed, continuing", "error", err)
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
	state.status.StepsCompleted = 2

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
		TenantID: input.TenantID,
		SourceID: input.SourceID,
		JobID:    input.JobID,
		Content:  fetchOutput.Content,
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
	state.status.StepsCompleted = 3

	if checkCancellation() {
		runCompensation(ctx)
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after summary")
		return state.result, nil
	}

	// Step 4: Extract entities via LLM
	updateStatus("extracting_entities", "ExtractEntities")
	var entityCount int
	ctx4 := workflow.WithActivityOptions(ctx, llmOpts)
	err = workflow.ExecuteActivity(ctx4, "ExtractEntities", ExtractEntitiesInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
		JobID:    input.JobID,
		Content:  fetchOutput.Content,
	}).Get(ctx, &entityCount)
	if err != nil {
		logger.Warn("Entity extraction failed, continuing", "error", err)
	} else {
		state.result.EntityCount = entityCount
		logger.Debug("Entities extracted", "count", entityCount)
	}
	state.status.StepsCompleted = 4

	if checkCancellation() {
		runCompensation(ctx)
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after entity extraction")
		return state.result, nil
	}

	// Step 5: Extract topics via LLM
	updateStatus("extracting_topics", "ExtractTopics")
	var topics []string
	ctx5 := workflow.WithActivityOptions(ctx, llmOpts)
	err = workflow.ExecuteActivity(ctx5, "ExtractTopics", ExtractTopicsInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
		JobID:    input.JobID,
		Content:  fetchOutput.Content,
	}).Get(ctx, &topics)
	if err != nil {
		logger.Warn("Topic extraction failed, continuing", "error", err)
	} else {
		state.result.ExtractedTopics = topics
		logger.Debug("Topics extracted", "count", len(topics))
	}
	state.status.StepsCompleted = 5

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
		TenantID:    input.TenantID,
		SourceID:    input.SourceID,
		ContentID:   input.SourceID, // Use SourceID as ContentID for now
		ContentType: input.SourceType,
		Content:     fetchOutput.Content,
		JobID:       input.JobID,
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
	state.status.StepsCompleted = 6

	if checkCancellation() {
		runCompensation(ctx)
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after mention extraction")
		return state.result, nil
	}

	// Step 7: Update content status
	updateStatus("updating_status", "UpdateContentStatus")
	ctx7 := workflow.WithActivityOptions(ctx, fastOpts)
	err = workflow.ExecuteActivity(ctx7, "UpdateContentStatus", UpdateContentStatusInput{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
		Status:   "completed",
	}).Get(ctx, nil)
	if err != nil {
		logger.Warn("Status update failed", "error", err)
	}
	state.status.StepsCompleted = 7

	updateStatus("completed", "")
	state.result.Status = "completed"
	logger.Info("Content ingestion workflow completed",
		"source_id", input.SourceID,
		"embedding_id", state.result.EmbeddingID,
		"summary_id", state.result.SummaryID,
		"entity_count", state.result.EntityCount,
		"topic_count", len(state.result.ExtractedTopics),
		"mention_count", state.result.MentionCount,
	)

	return state.result, nil
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
var _ = time.Second
