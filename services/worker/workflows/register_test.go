// Package workflows provides tests for workflow registration.
package workflows

import (
	"testing"

	"github.com/nexus-rpc/sdk-go/nexus"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/otherjamesbrown/penfold/services/worker/config"
)

// mockWorker implements worker.Worker for testing registration.
type mockWorker struct {
	registeredWorkflows  []string
	registeredActivities []string
}

func newMockWorker() *mockWorker {
	return &mockWorker{
		registeredWorkflows:  make([]string, 0),
		registeredActivities: make([]string, 0),
	}
}

func (m *mockWorker) RegisterWorkflow(w interface{}) {
	m.registeredWorkflows = append(m.registeredWorkflows, "workflow")
}

func (m *mockWorker) RegisterWorkflowWithOptions(w interface{}, opts workflow.RegisterOptions) {
	name := opts.Name
	if name == "" {
		name = "unnamed"
	}
	m.registeredWorkflows = append(m.registeredWorkflows, name)
}

func (m *mockWorker) RegisterActivity(a interface{}) {
	m.registeredActivities = append(m.registeredActivities, "activity")
}

func (m *mockWorker) RegisterActivityWithOptions(a interface{}, opts activity.RegisterOptions) {
	name := opts.Name
	if name == "" {
		name = "unnamed"
	}
	m.registeredActivities = append(m.registeredActivities, name)
}

func (m *mockWorker) RegisterNexusService(*nexus.Service) {
	// No-op for testing
}

func (m *mockWorker) RegisterDynamicActivity(handler interface{}, opts activity.DynamicRegisterOptions) {
	// No-op for testing
}

func (m *mockWorker) RegisterDynamicWorkflow(handler interface{}, opts workflow.DynamicRegisterOptions) {
	// No-op for testing
}

func (m *mockWorker) Start() error {
	return nil
}

func (m *mockWorker) Stop() {
}

func (m *mockWorker) Run(interruptCh <-chan interface{}) error {
	return nil
}

// Verify mockWorker implements worker.Worker
var _ worker.Worker = (*mockWorker)(nil)

// TestNewRegistrar verifies that NewRegistrar creates a valid Registrar instance.
func TestNewRegistrar(t *testing.T) {
	r := NewRegistrar()
	require.NotNil(t, r)
}

// TestWorkflowCount_MainQueue verifies workflow count for main queue.
func TestWorkflowCount_MainQueue(t *testing.T) {
	r := NewRegistrar()
	count := r.WorkflowCount(config.MainTaskQueue)
	// SLMPipelineWorkflow, ContentIngestionWorkflow, RelationshipDiscoveryWorkflow, DailyReviewWorkflow, BatchPipelineWorkflow, ConversationMaintenanceWorkflow, ConversationBackfillWorkflow, SessionLedgerConsolidationWorkflow, HeartbeatWorkflow, DigestWorkflow, WeeklyDigestWorkflow, JournalDigestWorkflow, DigestRollupWorkflow, JournalRollupWorkflow, AutomationRuleWorkflow, KBDriftScanWorkflow
	require.Equal(t, 16, count)
}

// TestWorkflowCount_AIQueue verifies workflow count for AI queue.
func TestWorkflowCount_AIQueue(t *testing.T) {
	r := NewRegistrar()
	count := r.WorkflowCount(config.AITaskQueue)
	// AnalysisWorkflow
	require.Equal(t, 1, count)
}

// TestWorkflowCount_EmailQueue verifies workflow count for email queue.
func TestWorkflowCount_EmailQueue(t *testing.T) {
	r := NewRegistrar()
	count := r.WorkflowCount(config.EmailTaskQueue)
	// EmailProcessingWorkflow, GmailSyncWorkflow, TeamsSyncWorkflow, OutlookSyncWorkflow, ADSyncWorkflow, TranscriptSyncWorkflow
	require.Equal(t, 6, count)
}

// TestWorkflowCount_UnknownQueue verifies workflow count for unknown queue.
func TestWorkflowCount_UnknownQueue(t *testing.T) {
	r := NewRegistrar()
	count := r.WorkflowCount("unknown-queue")
	require.Equal(t, 0, count)
}

// TestRegistrar_RegisterAll_EmailQueue verifies workflows are registered for email queue.
func TestRegistrar_RegisterAll_EmailQueue(t *testing.T) {
	w := newMockWorker()
	r := NewRegistrar()

	// Should not panic
	require.NotPanics(t, func() {
		r.RegisterAll(w, config.EmailTaskQueue)
	})

	// Verify workflows were registered
	require.Contains(t, w.registeredWorkflows, "EmailProcessingWorkflow")
	require.Contains(t, w.registeredWorkflows, "GmailSyncWorkflow")
	require.Contains(t, w.registeredWorkflows, "TeamsSyncWorkflow")
}

// TestRegistrar_RegisterAll_MainQueue verifies workflows are registered for main queue.
func TestRegistrar_RegisterAll_MainQueue(t *testing.T) {
	w := newMockWorker()
	r := NewRegistrar()

	// Should not panic
	require.NotPanics(t, func() {
		r.RegisterAll(w, config.MainTaskQueue)
	})

	// Main queue currently has no specific workflows (just common)
	// This will change as more workflows are added
}

// TestRegistrar_RegisterAll_AIQueue verifies workflows are registered for AI queue.
func TestRegistrar_RegisterAll_AIQueue(t *testing.T) {
	w := newMockWorker()
	r := NewRegistrar()

	// Should not panic
	require.NotPanics(t, func() {
		r.RegisterAll(w, config.AITaskQueue)
	})

	// AI queue currently has no workflows
}

// TestRegistrar_RegisterAll_UnknownQueue verifies behavior for unknown queue.
func TestRegistrar_RegisterAll_UnknownQueue(t *testing.T) {
	w := newMockWorker()
	r := NewRegistrar()

	// Should register common workflows without panic
	require.NotPanics(t, func() {
		r.RegisterAll(w, "unknown-queue")
	})
}

// TestTaskQueueConstants verifies the task queue constants are defined correctly.
func TestTaskQueueConstants(t *testing.T) {
	require.Equal(t, "penfold-main", config.MainTaskQueue)
	require.Equal(t, "penfold-ai", config.AITaskQueue)
	require.Equal(t, "penfold-email", config.EmailTaskQueue)
}

// TestAllTaskQueues verifies AllTaskQueues returns all queues.
func TestAllTaskQueues(t *testing.T) {
	queues := config.AllTaskQueues()
	require.Len(t, queues, 3)
	require.Contains(t, queues, config.MainTaskQueue)
	require.Contains(t, queues, config.AITaskQueue)
	require.Contains(t, queues, config.EmailTaskQueue)
}
