// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// AnalysisInput is the input for content analysis workflows.
type AnalysisInput = pkgtemporal.AnalysisInput

// AnalysisResult is the result of content analysis workflows.
type AnalysisResult = pkgtemporal.AnalysisResult

// AnalysisWorkflowStatus tracks the status of content analysis.
type AnalysisWorkflowStatus = pkgtemporal.WorkflowStatus

// ExtractedEntity represents an entity extracted from content.
type ExtractedEntity = pkgtemporal.ExtractedEntity

// Signal and query names for AnalysisWorkflow.
const (
	AnalysisStatusQuery  = "analysis_status"
	AnalysisPauseSignal  = "analysis_pause"
	AnalysisCancelSignal = "analysis_cancel"
)

// Analysis type constants.
const (
	AnalysisTypeCategorize       = "categorize"
	AnalysisTypeSummarize        = "summarize"
	AnalysisTypeExtractEntities  = "extract_entities"
	AnalysisTypeSentiment        = "sentiment"
)

// Activity input types for content analysis.
type (
	// CategorizeContentInput is the input for the CategorizeContent activity.
	CategorizeContentInput struct {
		TenantID   string `json:"tenant_id"`
		JobID      string `json:"job_id"`
		SourceID   int64  `json:"source_id"`
		SourceType string `json:"source_type"`
		Content    string `json:"content"`
	}

	// CategorizeContentOutput is the output from the CategorizeContent activity.
	CategorizeContentOutput struct {
		Categories  []string `json:"categories"`
		Confidence  float64  `json:"confidence"`
		PrimaryCategory string `json:"primary_category"`
	}

	// SummarizeContentInput is the input for the SummarizeContent activity.
	SummarizeContentInput struct {
		TenantID   string `json:"tenant_id"`
		JobID      string `json:"job_id"`
		SourceID   int64  `json:"source_id"`
		Content    string `json:"content"`
		MaxLength  int    `json:"max_length,omitempty"`
	}

	// SummarizeContentOutput is the output from the SummarizeContent activity.
	SummarizeContentOutput struct {
		SummaryID int64  `json:"summary_id"`
		Summary   string `json:"summary"`
		WordCount int    `json:"word_count"`
	}

	// ExtractEntitiesFromContentInput is the input for the ExtractEntitiesFromContent activity.
	ExtractEntitiesFromContentInput struct {
		TenantID   string   `json:"tenant_id"`
		JobID      string   `json:"job_id"`
		SourceID   int64    `json:"source_id"`
		Content    string   `json:"content"`
		EntityTypes []string `json:"entity_types,omitempty"` // Filter to specific entity types
	}

	// ExtractEntitiesFromContentOutput is the output from the ExtractEntitiesFromContent activity.
	ExtractEntitiesFromContentOutput struct {
		Entities []ExtractedEntity `json:"entities"`
		Count    int               `json:"count"`
	}

	// AnalyzeSentimentInput is the input for the AnalyzeSentiment activity.
	AnalyzeSentimentInput struct {
		TenantID string `json:"tenant_id"`
		JobID    string `json:"job_id"`
		SourceID int64  `json:"source_id"`
		Content  string `json:"content"`
	}

	// AnalyzeSentimentOutput is the output from the AnalyzeSentiment activity.
	AnalyzeSentimentOutput struct {
		Score     float64 `json:"score"` // -1.0 to 1.0
		Sentiment string  `json:"sentiment"` // positive, negative, neutral
		Confidence float64 `json:"confidence"`
	}

	// StoreAnalysisResultsInput is the input for the StoreAnalysisResults activity.
	StoreAnalysisResultsInput struct {
		TenantID       string            `json:"tenant_id"`
		JobID          string            `json:"job_id"`
		SourceID       int64             `json:"source_id"`
		Categories     []string          `json:"categories,omitempty"`
		SummaryID      *int64            `json:"summary_id,omitempty"`
		Entities       []ExtractedEntity `json:"entities,omitempty"`
		SentimentScore *float64          `json:"sentiment_score,omitempty"`
		Sentiment      string            `json:"sentiment,omitempty"`
	}

	// RollbackAnalysisInput is the input for the RollbackAnalysis compensation activity.
	RollbackAnalysisInput struct {
		TenantID  string `json:"tenant_id"`
		JobID     string `json:"job_id"`
		SourceID  int64  `json:"source_id"`
		SummaryID *int64 `json:"summary_id,omitempty"`
	}
)

// analysisState maintains the internal state of the workflow.
type analysisState struct {
	status          AnalysisWorkflowStatus
	result          *AnalysisResult
	paused          bool
	cancelRequested bool
	cancelReason    string
	analysisTypes   map[string]bool
}

// AnalysisWorkflow orchestrates the analysis of content.
// It performs the following steps based on requested analysis types:
// 1. Categorize content (if requested)
// 2. Summarize content (if requested)
// 3. Extract entities (if requested)
// 4. Analyze sentiment (if requested)
// 5. Store analysis results
//
// The workflow runs requested analyses in parallel where possible and
// implements the saga pattern with compensation for rollback on failure.
func AnalysisWorkflow(ctx workflow.Context, input AnalysisInput) (*AnalysisResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting analysis workflow",
		"tenant_id", input.TenantID,
		"job_id", input.JobID,
		"source_id", input.SourceID,
		"source_type", input.SourceType,
		"analysis_types", input.AnalysisType,
	)

	// Initialize workflow state
	state := &analysisState{
		status: AnalysisWorkflowStatus{
			Stage:          "initializing",
			StepsCompleted: 0,
			TotalSteps:     len(input.AnalysisType) + 1, // +1 for store step
			LastActivity:   "",
			StartedAt:      workflow.Now(ctx),
			LastUpdated:    workflow.Now(ctx),
		},
		result: &AnalysisResult{
			SourceID: input.SourceID,
		},
		analysisTypes: make(map[string]bool),
	}

	// Build analysis types map
	for _, at := range input.AnalysisType {
		state.analysisTypes[at] = true
	}

	// Register query handler for status
	if err := workflow.SetQueryHandler(ctx, AnalysisStatusQuery, func() (AnalysisWorkflowStatus, error) {
		return state.status, nil
	}); err != nil {
		logger.Error("Failed to register status query handler", "error", err)
	}

	// Setup signal channels
	pauseSignalChan := workflow.GetSignalChannel(ctx, AnalysisPauseSignal)
	cancelSignalChan := workflow.GetSignalChannel(ctx, AnalysisCancelSignal)

	// Activity option presets
	fastOpts := pkgtemporal.FastActivityOptions()
	llmOpts := pkgtemporal.LLMActivityOptions()

	// Add non-retryable errors
	fastOpts = pkgtemporal.WithNonRetryableErrors(fastOpts, pkgtemporal.NonRetryableErrors()...)
	llmOpts = pkgtemporal.WithNonRetryableErrors(llmOpts, pkgtemporal.NonRetryableErrors()...)

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

	// Validate input
	if len(input.AnalysisType) == 0 {
		state.result.Status = "failed"
		state.result.Error = "no analysis types specified"
		state.status.ErrorMessage = state.result.Error
		logger.Error("No analysis types specified")
		return state.result, nil
	}

	if input.Content == "" {
		state.result.Status = "failed"
		state.result.Error = "content is required"
		state.status.ErrorMessage = state.result.Error
		logger.Error("Content is required for analysis")
		return state.result, nil
	}

	// Run analyses in parallel where possible
	updateStatus("analyzing", "ParallelAnalysis")

	type analysisResult struct {
		analysisType string
		result       interface{}
		err          error
	}

	// Launch parallel analysis activities
	var futures []workflow.Future
	var futureTypes []string

	if state.analysisTypes[AnalysisTypeCategorize] {
		ctx1 := workflow.WithActivityOptions(ctx, llmOpts)
		future := workflow.ExecuteActivity(ctx1, "CategorizeContent", CategorizeContentInput{
			TenantID:   input.TenantID,
			JobID:      input.JobID,
			SourceID:   input.SourceID,
			SourceType: input.SourceType,
			Content:    input.Content,
		})
		futures = append(futures, future)
		futureTypes = append(futureTypes, AnalysisTypeCategorize)
	}

	if state.analysisTypes[AnalysisTypeSummarize] {
		ctx2 := workflow.WithActivityOptions(ctx, llmOpts)
		future := workflow.ExecuteActivity(ctx2, "SummarizeContent", SummarizeContentInput{
			TenantID: input.TenantID,
			JobID:    input.JobID,
			SourceID: input.SourceID,
			Content:  input.Content,
		})
		futures = append(futures, future)
		futureTypes = append(futureTypes, AnalysisTypeSummarize)
	}

	if state.analysisTypes[AnalysisTypeExtractEntities] {
		ctx3 := workflow.WithActivityOptions(ctx, llmOpts)
		future := workflow.ExecuteActivity(ctx3, "ExtractEntitiesFromContent", ExtractEntitiesFromContentInput{
			TenantID: input.TenantID,
			JobID:    input.JobID,
			SourceID: input.SourceID,
			Content:  input.Content,
		})
		futures = append(futures, future)
		futureTypes = append(futureTypes, AnalysisTypeExtractEntities)
	}

	if state.analysisTypes[AnalysisTypeSentiment] {
		ctx4 := workflow.WithActivityOptions(ctx, llmOpts)
		future := workflow.ExecuteActivity(ctx4, "AnalyzeSentiment", AnalyzeSentimentInput{
			TenantID: input.TenantID,
			JobID:    input.JobID,
			SourceID: input.SourceID,
			Content:  input.Content,
		})
		futures = append(futures, future)
		futureTypes = append(futureTypes, AnalysisTypeSentiment)
	}

	// Collect results from parallel activities
	successCount := 0
	for i, future := range futures {
		analysisType := futureTypes[i]

		switch analysisType {
		case AnalysisTypeCategorize:
			var output CategorizeContentOutput
			if err := future.Get(ctx, &output); err != nil {
				logger.Warn("Categorization failed", "error", err)
			} else {
				state.result.Categories = output.Categories
				successCount++
				logger.Debug("Categorization complete", "categories", output.Categories)
			}

		case AnalysisTypeSummarize:
			var output SummarizeContentOutput
			if err := future.Get(ctx, &output); err != nil {
				logger.Warn("Summarization failed", "error", err)
			} else {
				state.result.SummaryID = &output.SummaryID
				state.result.Summary = output.Summary
				successCount++
				// Add compensation to delete summary if later steps fail
				summaryID := output.SummaryID
				compensations = append(compensations, func(ctx workflow.Context) error {
					return workflow.ExecuteActivity(
						workflow.WithActivityOptions(ctx, fastOpts),
						"DeleteSummary",
						summaryID,
					).Get(ctx, nil)
				})
				logger.Debug("Summarization complete", "summary_id", output.SummaryID)
			}

		case AnalysisTypeExtractEntities:
			var output ExtractEntitiesFromContentOutput
			if err := future.Get(ctx, &output); err != nil {
				logger.Warn("Entity extraction failed", "error", err)
			} else {
				state.result.Entities = output.Entities
				successCount++
				logger.Debug("Entity extraction complete", "count", output.Count)
			}

		case AnalysisTypeSentiment:
			var output AnalyzeSentimentOutput
			if err := future.Get(ctx, &output); err != nil {
				logger.Warn("Sentiment analysis failed", "error", err)
			} else {
				state.result.SentimentScore = &output.Score
				state.result.Sentiment = output.Sentiment
				successCount++
				logger.Debug("Sentiment analysis complete", "sentiment", output.Sentiment, "score", output.Score)
			}
		}

		state.status.StepsCompleted++
	}

	// Check if all analyses failed
	if successCount == 0 {
		state.result.Status = "failed"
		state.result.Error = "all analysis activities failed"
		state.status.ErrorMessage = state.result.Error
		runCompensation(ctx)
		logger.Error("All analysis activities failed")
		return state.result, nil
	}

	if checkControls() {
		runCompensation(ctx)
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after analysis")
		return state.result, nil
	}

	// Store analysis results
	updateStatus("storing_results", "StoreAnalysisResults")
	ctx5 := workflow.WithActivityOptions(ctx, fastOpts)
	err := workflow.ExecuteActivity(ctx5, "StoreAnalysisResults", StoreAnalysisResultsInput{
		TenantID:       input.TenantID,
		JobID:          input.JobID,
		SourceID:       input.SourceID,
		Categories:     state.result.Categories,
		SummaryID:      state.result.SummaryID,
		Entities:       state.result.Entities,
		SentimentScore: state.result.SentimentScore,
		Sentiment:      state.result.Sentiment,
	}).Get(ctx, nil)
	if err != nil {
		logger.Warn("Failed to store analysis results", "error", err)
		// Don't fail - analysis was successful, just storage failed
	}
	state.status.StepsCompleted++

	// Finalize result
	updateStatus("completed", "")
	state.result.Status = "completed"
	state.result.AnalyzedAt = workflow.Now(ctx)

	logger.Info("Analysis workflow completed",
		"tenant_id", input.TenantID,
		"source_id", input.SourceID,
		"categories", len(state.result.Categories),
		"has_summary", state.result.SummaryID != nil,
		"entities", len(state.result.Entities),
		"sentiment", state.result.Sentiment,
	)

	return state.result, nil
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
var _ = time.Second
