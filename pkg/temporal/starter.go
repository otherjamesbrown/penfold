package temporal

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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

// StartWorkflowWithOptions starts a workflow with custom options.
func (s *WorkflowStarter) StartWorkflowWithOptions(
	ctx context.Context,
	options client.StartWorkflowOptions,
	workflow interface{},
	input interface{},
) (client.WorkflowRun, error) {
	// Ensure task queue is set if not provided
	if options.TaskQueue == "" {
		options.TaskQueue = s.taskQueue
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

// CancelWorkflow cancels a running workflow.
func (s *WorkflowStarter) CancelWorkflow(ctx context.Context, workflowID, runID string) error {
	return s.client.CancelWorkflow(ctx, workflowID, runID)
}

// TerminateWorkflow terminates a workflow with a reason.
func (s *WorkflowStarter) TerminateWorkflow(ctx context.Context, workflowID, runID, reason string) error {
	return s.client.TerminateWorkflow(ctx, workflowID, runID, reason)
}

// SignalWorkflow sends a signal to a running workflow.
func (s *WorkflowStarter) SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg interface{}) error {
	return s.client.SignalWorkflow(ctx, workflowID, runID, signalName, arg)
}

// QueryWorkflow queries a workflow for current state.
func (s *WorkflowStarter) QueryWorkflow(ctx context.Context, workflowID, runID, queryType string, args ...interface{}) (interface{}, error) {
	response, err := s.client.QueryWorkflow(ctx, workflowID, runID, queryType, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query workflow: %w", err)
	}

	var result interface{}
	if err := response.Get(&result); err != nil {
		return nil, fmt.Errorf("failed to decode query result: %w", err)
	}

	return result, nil
}

// Client returns the underlying Temporal client.
func (s *WorkflowStarter) Client() client.Client {
	return s.client
}

// TaskQueue returns the configured task queue.
func (s *WorkflowStarter) TaskQueue() string {
	return s.taskQueue
}

// Workflow ID generation utilities

// GenerateWorkflowID creates a unique workflow ID with a prefix and UUID.
func GenerateWorkflowID(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, uuid.New().String())
}

// GenerateTimestampedWorkflowID creates a workflow ID with prefix and timestamp.
func GenerateTimestampedWorkflowID(prefix string) string {
	return fmt.Sprintf("%s-%s-%s", prefix, time.Now().Format("20060102-150405"), uuid.New().String()[:8])
}

// GenerateDeterministicWorkflowID creates a predictable workflow ID from entity identifiers.
// Useful for idempotent workflow starts where the same input should reuse the same workflow.
func GenerateDeterministicWorkflowID(prefix string, entityType string, entityID string) string {
	return fmt.Sprintf("%s-%s-%s", prefix, entityType, entityID)
}

// GenerateEmailWorkflowID creates a workflow ID for email processing.
func GenerateEmailWorkflowID(tenantID, messageID string) string {
	return GenerateDeterministicWorkflowID("email", tenantID, messageID)
}

// GenerateIngestWorkflowID creates a workflow ID for content ingestion.
func GenerateIngestWorkflowID(tenantID, sourceType, sourceID string) string {
	return fmt.Sprintf("ingest-%s-%s-%s", tenantID, sourceType, sourceID)
}
