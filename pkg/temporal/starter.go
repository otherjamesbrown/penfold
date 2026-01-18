package temporal

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"
)

// WorkflowStarter provides utilities for starting and managing Temporal workflows.
type WorkflowStarter struct {
	client    client.Client
	taskQueue string
}

// NewWorkflowStarter creates a new WorkflowStarter with the given client and task queue.
func NewWorkflowStarter(c client.Client, taskQueue string) *WorkflowStarter {
	return &WorkflowStarter{
		client:    c,
		taskQueue: taskQueue,
	}
}

// StartWorkflow starts a workflow with the given ID and input.
func (s *WorkflowStarter) StartWorkflow(
	ctx context.Context,
	workflowID string,
	workflow interface{},
	input interface{},
) (client.WorkflowRun, error) {
	options := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: s.taskQueue,
	}

	we, err := s.client.ExecuteWorkflow(ctx, options, workflow, input)
	if err != nil {
		return nil, fmt.Errorf("failed to start workflow: %w", err)
	}

	return we, nil
}

// GetWorkflowResult waits for a workflow to complete and returns the result.
func (s *WorkflowStarter) GetWorkflowResult(ctx context.Context, workflowID, runID string, result interface{}) error {
	we := s.client.GetWorkflow(ctx, workflowID, runID)
	return we.Get(ctx, result)
}
