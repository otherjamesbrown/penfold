package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TemporalScheduler wraps Temporal's ScheduleClient with Penfold-specific logic.
type TemporalScheduler struct {
	client    client.Client
	taskQueue string
	logger    logging.Logger
}

// NewTemporalScheduler creates a new Temporal schedule wrapper.
func NewTemporalScheduler(c client.Client, taskQueue string, logger logging.Logger) *TemporalScheduler {
	return &TemporalScheduler{
		client:    c,
		taskQueue: taskQueue,
		logger:    logger,
	}
}

// Create creates a Temporal schedule from a DB schedule model.
func (ts *TemporalScheduler) Create(ctx context.Context, s *Schedule) error {
	// Parse workflow params into a generic map for Temporal args
	var params map[string]interface{}
	if len(s.WorkflowParams) > 0 {
		if err := json.Unmarshal(s.WorkflowParams, &params); err != nil {
			return fmt.Errorf("unmarshal workflow params: %w", err)
		}
	}

	opts := client.ScheduleOptions{
		ID: s.ID,
		Spec: client.ScheduleSpec{
			CronExpressions: []string{s.ScheduleExpr},
		},
		Action: &client.ScheduleWorkflowAction{
			ID:        s.ID + "-run",
			Workflow:  s.WorkflowType,
			TaskQueue: ts.taskQueue,
			Args:      []interface{}{params},
		},
		Overlap: overlapPolicyToTemporal(s.OverlapPolicy),
	}

	_, err := ts.client.ScheduleClient().Create(ctx, opts)
	if err != nil {
		return fmt.Errorf("create temporal schedule %s: %w", s.ID, err)
	}

	ts.logger.Info("Created Temporal schedule",
		logging.F("schedule_id", s.ID),
		logging.F("workflow_type", s.WorkflowType),
		logging.F("cron", s.ScheduleExpr),
	)
	return nil
}

// Update updates a Temporal schedule's spec and action.
func (ts *TemporalScheduler) Update(ctx context.Context, s *Schedule) error {
	handle := ts.client.ScheduleClient().GetHandle(ctx, s.ID)

	var params map[string]interface{}
	if len(s.WorkflowParams) > 0 {
		if err := json.Unmarshal(s.WorkflowParams, &params); err != nil {
			return fmt.Errorf("unmarshal workflow params: %w", err)
		}
	}

	err := handle.Update(ctx, client.ScheduleUpdateOptions{
		DoUpdate: func(input client.ScheduleUpdateInput) (*client.ScheduleUpdate, error) {
			input.Description.Schedule.Spec = &client.ScheduleSpec{
				CronExpressions: []string{s.ScheduleExpr},
			}
			input.Description.Schedule.Action = &client.ScheduleWorkflowAction{
				ID:        s.ID + "-run",
				Workflow:  s.WorkflowType,
				TaskQueue: ts.taskQueue,
				Args:      []interface{}{params},
			}
			if input.Description.Schedule.Policy == nil {
				input.Description.Schedule.Policy = &client.SchedulePolicies{}
			}
			input.Description.Schedule.Policy.Overlap = overlapPolicyToTemporal(s.OverlapPolicy)
			return &client.ScheduleUpdate{
				Schedule: &input.Description.Schedule,
			}, nil
		},
	})
	if err != nil {
		return fmt.Errorf("update temporal schedule %s: %w", s.ID, err)
	}
	return nil
}

// Delete deletes a Temporal schedule.
func (ts *TemporalScheduler) Delete(ctx context.Context, scheduleID string) error {
	handle := ts.client.ScheduleClient().GetHandle(ctx, scheduleID)
	if err := handle.Delete(ctx); err != nil {
		return fmt.Errorf("delete temporal schedule %s: %w", scheduleID, err)
	}
	return nil
}

// Pause pauses a Temporal schedule.
func (ts *TemporalScheduler) Pause(ctx context.Context, scheduleID string) error {
	handle := ts.client.ScheduleClient().GetHandle(ctx, scheduleID)
	if err := handle.Pause(ctx, client.SchedulePauseOptions{Note: "Paused via API"}); err != nil {
		return fmt.Errorf("pause temporal schedule %s: %w", scheduleID, err)
	}
	return nil
}

// Resume resumes a Temporal schedule.
func (ts *TemporalScheduler) Resume(ctx context.Context, scheduleID string) error {
	handle := ts.client.ScheduleClient().GetHandle(ctx, scheduleID)
	if err := handle.Unpause(ctx, client.ScheduleUnpauseOptions{Note: "Resumed via API"}); err != nil {
		return fmt.Errorf("resume temporal schedule %s: %w", scheduleID, err)
	}
	return nil
}

// Trigger forces an immediate execution of a Temporal schedule.
func (ts *TemporalScheduler) Trigger(ctx context.Context, scheduleID string) error {
	handle := ts.client.ScheduleClient().GetHandle(ctx, scheduleID)
	if err := handle.Trigger(ctx, client.ScheduleTriggerOptions{
		Overlap: enums.SCHEDULE_OVERLAP_POLICY_ALLOW_ALL,
	}); err != nil {
		return fmt.Errorf("trigger temporal schedule %s: %w", scheduleID, err)
	}
	return nil
}

// Describe returns the description of a Temporal schedule.
func (ts *TemporalScheduler) Describe(ctx context.Context, scheduleID string) (*client.ScheduleDescription, error) {
	handle := ts.client.ScheduleClient().GetHandle(ctx, scheduleID)
	desc, err := handle.Describe(ctx)
	if err != nil {
		return nil, fmt.Errorf("describe temporal schedule %s: %w", scheduleID, err)
	}
	return desc, nil
}

// TriggerWithParams starts a one-off workflow execution with overridden parameters.
// Unlike Trigger() which uses handle.Trigger() (no per-run args), this starts a
// fresh workflow via client.ExecuteWorkflow() with the merged params.
func (ts *TemporalScheduler) TriggerWithParams(ctx context.Context, s *Schedule, overrides map[string]interface{}) (string, error) {
	// Parse existing workflow params
	var params map[string]interface{}
	if len(s.WorkflowParams) > 0 {
		if err := json.Unmarshal(s.WorkflowParams, &params); err != nil {
			return "", fmt.Errorf("unmarshal workflow params: %w", err)
		}
	}
	if params == nil {
		params = make(map[string]interface{})
	}

	// Merge overrides
	for k, v := range overrides {
		params[k] = v
	}

	// Generate unique workflow ID for one-off execution
	workflowID := fmt.Sprintf("%s-manual-%d", s.ID, time.Now().UnixMilli())

	run, err := ts.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: ts.taskQueue,
	}, s.WorkflowType, params)
	if err != nil {
		return "", fmt.Errorf("trigger workflow with params %s: %w", workflowID, err)
	}

	ts.logger.Info("Triggered one-off workflow",
		logging.F("workflow_id", workflowID),
		logging.F("run_id", run.GetRunID()),
		logging.F("workflow_type", s.WorkflowType),
	)

	return workflowID, nil
}

// Exists checks if a Temporal schedule exists by attempting to Describe it.
func (ts *TemporalScheduler) Exists(ctx context.Context, scheduleID string) bool {
	_, err := ts.Describe(ctx, scheduleID)
	return err == nil
}

func overlapPolicyToTemporal(policy string) enums.ScheduleOverlapPolicy {
	switch strings.ToLower(policy) {
	case "skip":
		return enums.SCHEDULE_OVERLAP_POLICY_SKIP
	case "buffer_one":
		return enums.SCHEDULE_OVERLAP_POLICY_BUFFER_ONE
	case "cancel_other":
		return enums.SCHEDULE_OVERLAP_POLICY_CANCEL_OTHER
	case "allow_all":
		return enums.SCHEDULE_OVERLAP_POLICY_ALLOW_ALL
	default:
		return enums.SCHEDULE_OVERLAP_POLICY_SKIP
	}
}
