// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// DailyReviewInput is the input for daily review workflows.
type DailyReviewInput = pkgtemporal.DailyReviewInput

// DailyReviewResult is the result of daily review workflows.
type DailyReviewResult = pkgtemporal.DailyReviewResult

// DailyReviewWorkflowStatus tracks the status of daily review generation.
type DailyReviewWorkflowStatus = pkgtemporal.WorkflowStatus

// Signal and query names for DailyReviewWorkflow.
const (
	DailyReviewStatusQuery  = "daily_review_status"
	DailyReviewPauseSignal  = "daily_review_pause"
	DailyReviewCancelSignal = "daily_review_cancel"
)

// Activity input types for daily review.
type (
	// GatherEmailItemsInput is the input for the GatherEmailItems activity.
	GatherEmailItemsInput struct {
		TenantID   string    `json:"tenant_id"`
		UserID     string    `json:"user_id,omitempty"`
		ReviewDate time.Time `json:"review_date"`
		MaxItems   int       `json:"max_items"`
	}

	// GatherEmailItemsOutput is the output from the GatherEmailItems activity.
	GatherEmailItemsOutput struct {
		Items             []ReviewItem `json:"items"`
		TotalCount        int          `json:"total_count"`
		HighPriorityCount int          `json:"high_priority_count"`
	}

	// GatherDocumentItemsInput is the input for the GatherDocumentItems activity.
	GatherDocumentItemsInput struct {
		TenantID   string    `json:"tenant_id"`
		UserID     string    `json:"user_id,omitempty"`
		ReviewDate time.Time `json:"review_date"`
		MaxItems   int       `json:"max_items"`
	}

	// GatherDocumentItemsOutput is the output from the GatherDocumentItems activity.
	GatherDocumentItemsOutput struct {
		Items             []ReviewItem `json:"items"`
		TotalCount        int          `json:"total_count"`
		HighPriorityCount int          `json:"high_priority_count"`
	}

	// GatherAssertionItemsInput is the input for the GatherAssertionItems activity.
	GatherAssertionItemsInput struct {
		TenantID   string    `json:"tenant_id"`
		UserID     string    `json:"user_id,omitempty"`
		ReviewDate time.Time `json:"review_date"`
		MaxItems   int       `json:"max_items"`
	}

	// GatherAssertionItemsOutput is the output from the GatherAssertionItems activity.
	GatherAssertionItemsOutput struct {
		Items             []ReviewItem `json:"items"`
		TotalCount        int          `json:"total_count"`
		HighPriorityCount int          `json:"high_priority_count"`
	}

	// PrioritizeReviewItemsInput is the input for the PrioritizeReviewItems activity.
	PrioritizeReviewItemsInput struct {
		TenantID string       `json:"tenant_id"`
		UserID   string       `json:"user_id,omitempty"`
		JobID    string       `json:"job_id"`
		Items    []ReviewItem `json:"items"`
	}

	// PrioritizeReviewItemsOutput is the output from the PrioritizeReviewItems activity.
	PrioritizeReviewItemsOutput struct {
		Items             []ReviewItem                   `json:"items"`
		HighPriorityCount int                            `json:"high_priority_count"`
		Categories        []pkgtemporal.ReviewCategory   `json:"categories"`
	}

	// GenerateReviewSummaryInput is the input for the GenerateReviewSummary activity.
	GenerateReviewSummaryInput struct {
		TenantID   string       `json:"tenant_id"`
		UserID     string       `json:"user_id,omitempty"`
		JobID      string       `json:"job_id"`
		ReviewDate time.Time    `json:"review_date"`
		Items      []ReviewItem `json:"items"`
	}

	// GenerateReviewSummaryOutput is the output from the GenerateReviewSummary activity.
	GenerateReviewSummaryOutput struct {
		ReviewID string `json:"review_id"`
		Summary  string `json:"summary"`
	}

	// SaveDailyReviewInput is the input for the SaveDailyReview activity.
	SaveDailyReviewInput struct {
		TenantID          string                       `json:"tenant_id"`
		ReviewID          string                       `json:"review_id"`
		ReviewDate        time.Time                    `json:"review_date"`
		UserID            string                       `json:"user_id,omitempty"`
		ItemCount         int                          `json:"item_count"`
		HighPriorityCount int                          `json:"high_priority_count"`
		Categories        []pkgtemporal.ReviewCategory `json:"categories"`
		Summary           string                       `json:"summary"`
	}
)

// ReviewItem represents an item to be reviewed.
type ReviewItem struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // email, document, assertion
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Priority    int       `json:"priority"` // 0=low, 1=normal, 2=high, 3=urgent
	SourceID    int64     `json:"source_id"`
	CreatedAt   time.Time `json:"created_at"`
	Category    string    `json:"category,omitempty"`
	ActionItems []string  `json:"action_items,omitempty"`
}

// dailyReviewState maintains the internal state of the workflow.
type dailyReviewState struct {
	status          DailyReviewWorkflowStatus
	result          *DailyReviewResult
	paused          bool
	cancelRequested bool
	cancelReason    string
	allItems        []ReviewItem
}

// DailyReviewWorkflow orchestrates the generation of a daily review.
// It performs the following steps:
// 1. Gather email items (parallel)
// 2. Gather document items (parallel)
// 3. Gather assertion items (parallel)
// 4. Prioritize and categorize items
// 5. Generate review summary via LLM
// 6. Save daily review
//
// The workflow supports pause/resume signals for user control.
func DailyReviewWorkflow(ctx workflow.Context, input DailyReviewInput) (*DailyReviewResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting daily review workflow",
		"tenant_id", input.TenantID,
		"review_date", input.ReviewDate,
		"user_id", input.UserID,
		"job_id", input.JobID,
	)

	// Initialize workflow state
	state := &dailyReviewState{
		status: DailyReviewWorkflowStatus{
			Stage:          "initializing",
			StepsCompleted: 0,
			TotalSteps:     6,
			LastActivity:   "",
			StartedAt:      workflow.Now(ctx),
			LastUpdated:    workflow.Now(ctx),
		},
		result: &DailyReviewResult{
			TenantID:   input.TenantID,
			ReviewDate: input.ReviewDate,
		},
		allItems: make([]ReviewItem, 0),
	}

	// Register query handler for status
	if err := workflow.SetQueryHandler(ctx, DailyReviewStatusQuery, func() (DailyReviewWorkflowStatus, error) {
		return state.status, nil
	}); err != nil {
		logger.Error("Failed to register status query handler", "error", err)
	}

	// Setup signal channel for pause/resume
	pauseSignalChan := workflow.GetSignalChannel(ctx, DailyReviewPauseSignal)

	// Setup signal channel for cancellation
	cancelSignalChan := workflow.GetSignalChannel(ctx, DailyReviewCancelSignal)

	// Handle pause signal
	handlePauseSignal := func() {
		for {
			if !state.paused {
				return
			}
			// Wait for resume signal
			selector := workflow.NewSelector(ctx)
			selector.AddReceive(pauseSignalChan, func(c workflow.ReceiveChannel, more bool) {
				var signal pkgtemporal.PauseResumeSignal
				c.Receive(ctx, &signal)
				state.paused = signal.Paused
				logger.Info("Received pause/resume signal", "paused", signal.Paused, "reason", signal.Reason)
			})
			selector.AddReceive(cancelSignalChan, func(c workflow.ReceiveChannel, more bool) {
				var signal pkgtemporal.CancelWithCompensationSignal
				c.Receive(ctx, &signal)
				state.cancelRequested = true
				state.cancelReason = signal.Reason
				state.paused = false // Unpause to allow cancellation
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
			logger.Info("Received pause/resume signal", "paused", signal.Paused)
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

	// Activity option presets
	fastOpts := pkgtemporal.FastActivityOptions()
	batchOpts := pkgtemporal.BatchActivityOptions()
	llmOpts := pkgtemporal.LLMActivityOptions()

	// Add non-retryable errors
	fastOpts = pkgtemporal.WithNonRetryableErrors(fastOpts, pkgtemporal.NonRetryableErrors()...)
	batchOpts = pkgtemporal.WithNonRetryableErrors(batchOpts, pkgtemporal.NonRetryableErrors()...)
	llmOpts = pkgtemporal.WithNonRetryableErrors(llmOpts, pkgtemporal.NonRetryableErrors()...)

	// Update status helper
	updateStatus := func(stage, activity string) {
		state.status.Stage = stage
		state.status.LastActivity = activity
		state.status.LastUpdated = workflow.Now(ctx)
	}

	// Check cancellation/pause helper
	checkControls := func() bool {
		drainSignals()
		handlePauseSignal()
		return state.cancelRequested
	}

	// Calculate max items per category
	maxItemsPerCategory := input.MaxItems / 3
	if maxItemsPerCategory < 10 {
		maxItemsPerCategory = 10
	}

	// Steps 1-3: Gather items in parallel
	updateStatus("gathering_items", "GatherItems")

	var (
		emailOutput     GatherEmailItemsOutput
		documentOutput  GatherDocumentItemsOutput
		assertionOutput GatherAssertionItemsOutput
		emailErr        error
		documentErr     error
		assertionErr    error
	)

	// Execute gathering activities in parallel
	var gatherFutures []workflow.Future

	if input.IncludeEmail {
		ctx1 := workflow.WithActivityOptions(ctx, batchOpts)
		emailFuture := workflow.ExecuteActivity(ctx1, "GatherEmailItems", GatherEmailItemsInput{
			TenantID:   input.TenantID,
			UserID:     input.UserID,
			ReviewDate: input.ReviewDate,
			MaxItems:   maxItemsPerCategory,
		})
		gatherFutures = append(gatherFutures, emailFuture)
	}

	if input.IncludeDocuments {
		ctx2 := workflow.WithActivityOptions(ctx, batchOpts)
		documentFuture := workflow.ExecuteActivity(ctx2, "GatherDocumentItems", GatherDocumentItemsInput{
			TenantID:   input.TenantID,
			UserID:     input.UserID,
			ReviewDate: input.ReviewDate,
			MaxItems:   maxItemsPerCategory,
		})
		gatherFutures = append(gatherFutures, documentFuture)
	}

	if input.IncludeAssertions {
		ctx3 := workflow.WithActivityOptions(ctx, batchOpts)
		assertionFuture := workflow.ExecuteActivity(ctx3, "GatherAssertionItems", GatherAssertionItemsInput{
			TenantID:   input.TenantID,
			UserID:     input.UserID,
			ReviewDate: input.ReviewDate,
			MaxItems:   maxItemsPerCategory,
		})
		gatherFutures = append(gatherFutures, assertionFuture)
	}

	// Wait for all gathering activities to complete
	futureIndex := 0
	if input.IncludeEmail {
		emailErr = gatherFutures[futureIndex].Get(ctx, &emailOutput)
		futureIndex++
		if emailErr != nil {
			logger.Warn("Failed to gather email items", "error", emailErr)
		} else {
			state.allItems = append(state.allItems, emailOutput.Items...)
		}
	}

	if input.IncludeDocuments {
		documentErr = gatherFutures[futureIndex].Get(ctx, &documentOutput)
		futureIndex++
		if documentErr != nil {
			logger.Warn("Failed to gather document items", "error", documentErr)
		} else {
			state.allItems = append(state.allItems, documentOutput.Items...)
		}
	}

	if input.IncludeAssertions {
		assertionErr = gatherFutures[futureIndex].Get(ctx, &assertionOutput)
		if assertionErr != nil {
			logger.Warn("Failed to gather assertion items", "error", assertionErr)
		} else {
			state.allItems = append(state.allItems, assertionOutput.Items...)
		}
	}

	state.status.StepsCompleted = 3

	// Check if all gathering failed
	if emailErr != nil && documentErr != nil && assertionErr != nil {
		state.result.Status = "failed"
		state.result.Error = "all gathering activities failed"
		state.status.ErrorMessage = state.result.Error
		logger.Error("All gathering activities failed")
		return state.result, nil
	}

	if checkControls() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after gathering")
		return state.result, nil
	}

	// Check if no items were gathered
	if len(state.allItems) == 0 {
		state.result.Status = "completed"
		state.result.ItemCount = 0
		state.result.GeneratedAt = workflow.Now(ctx)
		logger.Info("No items to review", "tenant_id", input.TenantID, "review_date", input.ReviewDate)
		return state.result, nil
	}

	// Step 4: Prioritize and categorize items
	updateStatus("prioritizing_items", "PrioritizeReviewItems")
	var prioritizeOutput PrioritizeReviewItemsOutput
	ctx4 := workflow.WithActivityOptions(ctx, llmOpts)
	err := workflow.ExecuteActivity(ctx4, "PrioritizeReviewItems", PrioritizeReviewItemsInput{
		TenantID: input.TenantID,
		UserID:   input.UserID,
		JobID:    input.JobID,
		Items:    state.allItems,
	}).Get(ctx, &prioritizeOutput)
	if err != nil {
		logger.Warn("Item prioritization failed, using default ordering", "error", err)
		// Continue with unprioritized items
		prioritizeOutput.Items = state.allItems
		prioritizeOutput.HighPriorityCount = 0
	} else {
		state.allItems = prioritizeOutput.Items
	}
	state.status.StepsCompleted = 4

	if checkControls() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after prioritization")
		return state.result, nil
	}

	// Step 5: Generate review summary
	updateStatus("generating_summary", "GenerateReviewSummary")
	var summaryOutput GenerateReviewSummaryOutput
	ctx5 := workflow.WithActivityOptions(ctx, llmOpts)
	err = workflow.ExecuteActivity(ctx5, "GenerateReviewSummary", GenerateReviewSummaryInput{
		TenantID:   input.TenantID,
		UserID:     input.UserID,
		JobID:      input.JobID,
		ReviewDate: input.ReviewDate,
		Items:      state.allItems,
	}).Get(ctx, &summaryOutput)
	if err != nil {
		logger.Warn("Review summary generation failed, continuing without summary", "error", err)
		summaryOutput.ReviewID = fmt.Sprintf("review-%s-%s", input.TenantID, input.ReviewDate.Format("2006-01-02"))
	}
	state.status.StepsCompleted = 5

	if checkControls() {
		state.result.Status = "cancelled"
		state.result.Error = state.cancelReason
		logger.Info("Workflow cancelled after summary generation")
		return state.result, nil
	}

	// Step 6: Save daily review
	updateStatus("saving_review", "SaveDailyReview")
	ctx6 := workflow.WithActivityOptions(ctx, fastOpts)
	err = workflow.ExecuteActivity(ctx6, "SaveDailyReview", SaveDailyReviewInput{
		TenantID:          input.TenantID,
		ReviewID:          summaryOutput.ReviewID,
		ReviewDate:        input.ReviewDate,
		UserID:            input.UserID,
		ItemCount:         len(state.allItems),
		HighPriorityCount: prioritizeOutput.HighPriorityCount,
		Categories:        prioritizeOutput.Categories,
		Summary:           summaryOutput.Summary,
	}).Get(ctx, nil)
	if err != nil {
		logger.Warn("Failed to save daily review", "error", err)
	}
	state.status.StepsCompleted = 6

	// Populate final result
	updateStatus("completed", "")
	state.result.ReviewID = summaryOutput.ReviewID
	state.result.ItemCount = len(state.allItems)
	state.result.HighPriorityCount = prioritizeOutput.HighPriorityCount
	state.result.Categories = prioritizeOutput.Categories
	state.result.GeneratedAt = workflow.Now(ctx)
	state.result.Status = "completed"

	logger.Info("Daily review workflow completed",
		"tenant_id", input.TenantID,
		"review_id", state.result.ReviewID,
		"item_count", state.result.ItemCount,
		"high_priority_count", state.result.HighPriorityCount,
		"category_count", len(state.result.Categories),
	)

	return state.result, nil
}

// Ensure temporal package is used to avoid import errors during development.
var _ = temporal.RetryPolicy{}
var _ = time.Second
