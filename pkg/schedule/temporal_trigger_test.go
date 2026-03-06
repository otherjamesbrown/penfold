package schedule

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.temporal.io/sdk/client"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockTemporalClient captures ExecuteWorkflow calls for inspection.
type mockTemporalClient struct {
	client.Client
	capturedOptions  *client.StartWorkflowOptions
	capturedWorkflow interface{}
	capturedArgs     []interface{}
	runID            string
}

func (m *mockTemporalClient) ExecuteWorkflow(
	ctx context.Context,
	options client.StartWorkflowOptions,
	workflow interface{},
	args ...interface{},
) (client.WorkflowRun, error) {
	m.capturedOptions = &client.StartWorkflowOptions{
		ID:        options.ID,
		TaskQueue: options.TaskQueue,
	}
	m.capturedWorkflow = workflow
	m.capturedArgs = args
	return &mockWorkflowRun{workflowID: options.ID, runID: m.runID}, nil
}

type mockWorkflowRun struct {
	client.WorkflowRun
	workflowID string
	runID      string
}

func (r *mockWorkflowRun) GetID() string    { return r.workflowID }
func (r *mockWorkflowRun) GetRunID() string { return r.runID }

func newTestScheduler(mc *mockTemporalClient) *TemporalScheduler {
	logger := logging.NewLogger(&logging.Config{Level: logging.LevelDebug})
	return &TemporalScheduler{
		client:    mc,
		taskQueue: "test-queue",
		logger:    logger,
	}
}

// TestTriggerWithParams_MergesOverrides verifies that overrides are merged into
// existing workflow params and passed to ExecuteWorkflow.
func TestTriggerWithParams_MergesOverrides(t *testing.T) {
	mc := &mockTemporalClient{runID: "test-run-id"}
	ts := newTestScheduler(mc)

	sched := &Schedule{
		ID:             "sched-001",
		WorkflowType:   "DigestWorkflow",
		WorkflowParams: json.RawMessage(`{"tenant_id":"t1","content_type":"article"}`),
	}

	overrides := map[string]interface{}{
		"reference_date": "2026-03-01",
	}

	workflowID, err := ts.TriggerWithParams(context.Background(), sched, overrides)
	if err != nil {
		t.Fatalf("TriggerWithParams returned error: %v", err)
	}

	// Workflow ID should start with schedule ID and contain "manual"
	if !strings.HasPrefix(workflowID, "sched-001-manual-") {
		t.Errorf("expected workflow ID to start with %q, got %q", "sched-001-manual-", workflowID)
	}

	// Verify ExecuteWorkflow was called
	if mc.capturedOptions == nil {
		t.Fatal("ExecuteWorkflow was not called")
	}
	if mc.capturedOptions.ID != workflowID {
		t.Errorf("expected workflow ID %q, got %q", workflowID, mc.capturedOptions.ID)
	}
	if mc.capturedOptions.TaskQueue != "test-queue" {
		t.Errorf("expected task queue %q, got %q", "test-queue", mc.capturedOptions.TaskQueue)
	}
	if mc.capturedWorkflow != "DigestWorkflow" {
		t.Errorf("expected workflow type %q, got %v", "DigestWorkflow", mc.capturedWorkflow)
	}

	// Verify merged params contain both original and override values.
	if len(mc.capturedArgs) == 0 {
		t.Fatal("ExecuteWorkflow was called with no args")
	}
	params, ok := mc.capturedArgs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected args[0] to be map[string]interface{}, got %T", mc.capturedArgs[0])
	}
	if params["reference_date"] != "2026-03-01" {
		t.Errorf("expected reference_date %q in params, got %v", "2026-03-01", params["reference_date"])
	}
	if params["tenant_id"] != "t1" {
		t.Errorf("expected tenant_id %q in params, got %v", "t1", params["tenant_id"])
	}
	if params["content_type"] != "article" {
		t.Errorf("expected content_type %q in params, got %v", "article", params["content_type"])
	}
}

// TestTriggerWithParams_NilWorkflowParams verifies that TriggerWithParams works
// when the schedule has no existing workflow params.
func TestTriggerWithParams_NilWorkflowParams(t *testing.T) {
	mc := &mockTemporalClient{runID: "run-99"}
	ts := newTestScheduler(mc)

	sched := &Schedule{
		ID:             "sched-empty",
		WorkflowType:   "SomeWorkflow",
		WorkflowParams: nil,
	}

	overrides := map[string]interface{}{
		"reference_date": "2026-02-28",
	}

	workflowID, err := ts.TriggerWithParams(context.Background(), sched, overrides)
	if err != nil {
		t.Fatalf("TriggerWithParams returned error: %v", err)
	}

	if !strings.HasPrefix(workflowID, "sched-empty-manual-") {
		t.Errorf("expected workflow ID prefix %q, got %q", "sched-empty-manual-", workflowID)
	}

	if len(mc.capturedArgs) == 0 {
		t.Fatal("no args captured")
	}
	params, ok := mc.capturedArgs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", mc.capturedArgs[0])
	}
	if params["reference_date"] != "2026-02-28" {
		t.Errorf("expected reference_date %q, got %v", "2026-02-28", params["reference_date"])
	}
}

// TestTriggerWithParams_OverrideWins verifies that override values replace existing params.
func TestTriggerWithParams_OverrideWins(t *testing.T) {
	mc := &mockTemporalClient{runID: "run-override"}
	ts := newTestScheduler(mc)

	sched := &Schedule{
		ID:             "sched-override",
		WorkflowType:   "DigestWorkflow",
		WorkflowParams: json.RawMessage(`{"reference_date":"2026-01-01"}`),
	}

	overrides := map[string]interface{}{
		"reference_date": "2026-03-05",
	}

	_, err := ts.TriggerWithParams(context.Background(), sched, overrides)
	if err != nil {
		t.Fatalf("TriggerWithParams returned error: %v", err)
	}

	params, ok := mc.capturedArgs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", mc.capturedArgs[0])
	}
	if params["reference_date"] != "2026-03-05" {
		t.Errorf("override should win: expected %q, got %v", "2026-03-05", params["reference_date"])
	}
}
