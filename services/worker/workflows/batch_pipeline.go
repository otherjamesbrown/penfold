// Package workflows provides workflow definitions for the Temporal worker.
package workflows

import (
	"fmt"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/workflow"
)

// BatchPipelineInput is the input for the BatchPipelineWorkflow.
type BatchPipelineInput struct {
	TenantID      string  `json:"tenant_id"`
	TenantName    string  `json:"tenant_name,omitempty"`
	ContentIDs    []int64 `json:"content_ids"`   // Source IDs to process
	MaxConcurrent int     `json:"max_concurrent"` // Sliding window size
	BatchID       string  `json:"batch_id"`       // UUID for tracking
}

// BatchPipelineResult is the output of the BatchPipelineWorkflow.
type BatchPipelineResult struct {
	BatchID        string `json:"batch_id"`
	TotalItems     int    `json:"total_items"`
	CompletedItems int    `json:"completed_items"`
	FailedItems    int    `json:"failed_items"`
	Status         string `json:"status"` // completed, cancelled, failed
}

// BatchProgress is returned by the "batch-progress" query handler.
type BatchProgress struct {
	BatchID     string  `json:"batch_id"`
	Total       int     `json:"total"`
	Pending     int     `json:"pending"`
	Active      int     `json:"active"`
	Completed   int     `json:"completed"`
	Failed      int     `json:"failed"`
	ActiveItems []int64 `json:"active_items"` // Source IDs currently processing
	Status      string  `json:"status"`       // running, completed, cancelled
}

// Signal names for BatchPipelineWorkflow.
const (
	BatchCancelSignal            = "batch-cancel"
	BatchUpdateConcurrencySignal = "batch-update-concurrency"
)

// Query names for BatchPipelineWorkflow.
const (
	BatchProgressQuery = "batch-progress"
)

// batchItemState tracks state for a single item in the batch.
type batchItemState struct {
	sourceID  int64
	future    workflow.ChildWorkflowFuture
}

// BatchPipelineWorkflow orchestrates a batch of SLMPipelineWorkflow child executions
// with a sliding concurrency window. It supports real-time progress queries and
// signals for cancellation and concurrency adjustment.
func BatchPipelineWorkflow(ctx workflow.Context, input BatchPipelineInput) (*BatchPipelineResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("BatchPipelineWorkflow started",
		"batch_id", input.BatchID,
		"total_items", len(input.ContentIDs),
		"max_concurrent", input.MaxConcurrent,
	)

	// Mutable state — read by query handler
	progress := BatchProgress{
		BatchID:     input.BatchID,
		Total:       len(input.ContentIDs),
		Pending:     len(input.ContentIDs),
		Active:      0,
		Completed:   0,
		Failed:      0,
		ActiveItems: []int64{},
		Status:      "running",
	}

	cancelRequested := false
	maxConcurrent := input.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	// Register query handler for real-time progress.
	if err := workflow.SetQueryHandler(ctx, BatchProgressQuery, func() (BatchProgress, error) {
		return progress, nil
	}); err != nil {
		logger.Error("Failed to register batch-progress query handler", "error", err)
	}

	// Signal channels.
	cancelChan := workflow.GetSignalChannel(ctx, BatchCancelSignal)
	concurrencyChan := workflow.GetSignalChannel(ctx, BatchUpdateConcurrencySignal)

	// Pending items to process (index into ContentIDs).
	pendingIdx := 0
	totalItems := len(input.ContentIDs)

	// Track active children: sourceID -> future.
	activeItems := make(map[int64]workflow.ChildWorkflowFuture, maxConcurrent)

	// Helper: build the active items list for the progress snapshot.
	activeItemsList := func() []int64 {
		ids := make([]int64, 0, len(activeItems))
		for id := range activeItems {
			ids = append(ids, id)
		}
		return ids
	}

	// updateProgress rebuilds the progress snapshot from current state.
	updateProgress := func() {
		progress.Active = len(activeItems)
		progress.Pending = totalItems - pendingIdx - len(activeItems)
		if progress.Pending < 0 {
			progress.Pending = 0
		}
		progress.ActiveItems = activeItemsList()
	}

	// startNextItems fills active slots up to maxConcurrent.
	startNextItems := func() {
		for pendingIdx < totalItems && len(activeItems) < maxConcurrent && !cancelRequested {
			sourceID := input.ContentIDs[pendingIdx]
			pendingIdx++

			childWorkflowID := fmt.Sprintf("batch-%s-src-%d", input.BatchID, sourceID)
			childOpts := workflow.ChildWorkflowOptions{
				WorkflowID: childWorkflowID,
				TaskQueue:  "penfold-main",
				// Abandon children when parent closes — if parent is cancelled,
				// already-running children are allowed to finish independently.
				ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_ABANDON,
			}
			childCtx := workflow.WithChildOptions(ctx, childOpts)

			childInput := PipelineInput{
				TenantID:   input.TenantID,
				TenantName: input.TenantName,
				SourceID:   sourceID,
				JobID:      childWorkflowID,
			}

			future := workflow.ExecuteChildWorkflow(childCtx, SLMPipelineWorkflow, childInput)
			activeItems[sourceID] = future

			logger.Info("Started child pipeline workflow",
				"batch_id", input.BatchID,
				"source_id", sourceID,
				"child_workflow_id", childWorkflowID,
			)
		}
		updateProgress()
	}

	// Handle empty batch immediately.
	if totalItems == 0 {
		progress.Status = "completed"
		return &BatchPipelineResult{
			BatchID:        input.BatchID,
			TotalItems:     0,
			CompletedItems: 0,
			FailedItems:    0,
			Status:         "completed",
		}, nil
	}

	// Seed the initial window.
	startNextItems()

	// Main loop: drain active children using a selector, refilling the window each time.
	for len(activeItems) > 0 {
		selector := workflow.NewSelector(ctx)

		// Add all active child futures.
		for srcID, fut := range activeItems {
			srcID := srcID // capture for closure
			fut := fut     // capture for closure
			selector.AddFuture(fut, func(f workflow.Future) {
				var result PipelineResult
				err := f.Get(ctx, &result)
				if err != nil {
					logger.Warn("Child pipeline workflow failed",
						"batch_id", input.BatchID,
						"source_id", srcID,
						"error", err,
					)
					progress.Failed++
				} else {
					logger.Info("Child pipeline workflow completed",
						"batch_id", input.BatchID,
						"source_id", srcID,
						"status", result.Status,
					)
					if result.Status == "failed" {
						progress.Failed++
					} else {
						progress.Completed++
					}
				}
				delete(activeItems, srcID)
				updateProgress()
			})
		}

		// Watch for cancel signal.
		selector.AddReceive(cancelChan, func(c workflow.ReceiveChannel, more bool) {
			c.Receive(ctx, nil) // drain the signal value
			logger.Info("BatchPipelineWorkflow received cancel signal", "batch_id", input.BatchID)
			cancelRequested = true
			progress.Status = "cancelled"
		})

		// Watch for concurrency update signal.
		selector.AddReceive(concurrencyChan, func(c workflow.ReceiveChannel, more bool) {
			var newMax int
			c.Receive(ctx, &newMax)
			if newMax > 0 {
				logger.Info("BatchPipelineWorkflow updating max concurrency",
					"batch_id", input.BatchID,
					"old_max", maxConcurrent,
					"new_max", newMax,
				)
				maxConcurrent = newMax
			}
		})

		// Wait for exactly one event (child completion or signal).
		selector.Select(ctx)

		// After handling an event, attempt to fill new slots — unless cancelled.
		if !cancelRequested {
			startNextItems()
		}

		// Drain any other immediately-ready events (e.g. multiple children done at once).
		for selector.HasPending() {
			selector.Select(ctx)
			if !cancelRequested {
				startNextItems()
			}
		}
	}

	// Determine final status.
	finalStatus := "completed"
	if cancelRequested {
		finalStatus = "cancelled"
		progress.Status = "cancelled"
	} else {
		progress.Status = "completed"
	}

	logger.Info("BatchPipelineWorkflow finished",
		"batch_id", input.BatchID,
		"status", finalStatus,
		"completed", progress.Completed,
		"failed", progress.Failed,
	)

	return &BatchPipelineResult{
		BatchID:        input.BatchID,
		TotalItems:     totalItems,
		CompletedItems: progress.Completed,
		FailedItems:    progress.Failed,
		Status:         finalStatus,
	}, nil
}
