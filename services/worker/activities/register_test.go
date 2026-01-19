// Package activities provides tests for activity registration.
package activities

import (
	"testing"

	"github.com/nexus-rpc/sdk-go/nexus"
	"github.com/rs/zerolog"
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

// TestNewRegistrar_WithActivities verifies NewRegistrar creates a valid Registrar with activities.
func TestNewRegistrar_WithActivities(t *testing.T) {
	logger := zerolog.Nop()
	acts := NewActivities(logger)
	r := NewRegistrar(acts)

	require.NotNil(t, r)
}

// TestNewRegistrar_NilActivities verifies NewRegistrar handles nil activities.
func TestNewRegistrar_NilActivities(t *testing.T) {
	r := NewRegistrar(nil)
	require.NotNil(t, r)
}

// TestActivityCount_MainQueue verifies activity count for main queue.
func TestActivityCount_MainQueue(t *testing.T) {
	logger := zerolog.Nop()
	acts := NewActivities(logger)
	r := NewRegistrar(acts)

	count := r.ActivityCount(config.MainTaskQueue)
	// Currently no activities registered for main queue (besides common)
	require.Equal(t, 0, count)
}

// TestActivityCount_AIQueue verifies activity count for AI queue.
func TestActivityCount_AIQueue(t *testing.T) {
	logger := zerolog.Nop()
	acts := NewActivities(logger)
	r := NewRegistrar(acts)

	count := r.ActivityCount(config.AITaskQueue)
	// GenerateEmbedding, GenerateSummary, ExtractAssertions
	require.Equal(t, 3, count)
}

// TestActivityCount_EmailQueue verifies activity count for email queue.
func TestActivityCount_EmailQueue(t *testing.T) {
	logger := zerolog.Nop()
	acts := NewActivities(logger)
	r := NewRegistrar(acts)

	count := r.ActivityCount(config.EmailTaskQueue)
	// FetchSource, UpdateSourceStatus + AI activities (3) = 5
	require.Equal(t, 5, count)
}

// TestActivityCount_UnknownQueue verifies activity count for unknown queue.
func TestActivityCount_UnknownQueue(t *testing.T) {
	logger := zerolog.Nop()
	acts := NewActivities(logger)
	r := NewRegistrar(acts)

	count := r.ActivityCount("unknown-queue")
	require.Equal(t, 0, count)
}

// TestRegistrar_RegisterAll_EmailQueue_WithActivities verifies registration with activities.
func TestRegistrar_RegisterAll_EmailQueue_WithActivities(t *testing.T) {
	w := newMockWorker()
	logger := zerolog.Nop()
	acts := NewActivities(logger)
	r := NewRegistrar(acts)

	// Should not panic
	require.NotPanics(t, func() {
		r.RegisterAll(w, config.EmailTaskQueue)
	})

	// Verify activities were registered
	require.Contains(t, w.registeredActivities, "FetchSource")
	require.Contains(t, w.registeredActivities, "UpdateSourceStatus")
	require.Contains(t, w.registeredActivities, "GenerateEmbedding")
	require.Contains(t, w.registeredActivities, "GenerateSummary")
	require.Contains(t, w.registeredActivities, "ExtractAssertions")
}

// TestRegistrar_RegisterAll_EmailQueue_NilActivities verifies registration without activities.
func TestRegistrar_RegisterAll_EmailQueue_NilActivities(t *testing.T) {
	w := newMockWorker()
	r := NewRegistrar(nil)

	// Should not panic even with nil activities
	require.NotPanics(t, func() {
		r.RegisterAll(w, config.EmailTaskQueue)
	})

	// No activities should be registered
	require.Empty(t, w.registeredActivities)
}

// TestRegistrar_RegisterAll_AIQueue verifies registration for AI queue.
func TestRegistrar_RegisterAll_AIQueue(t *testing.T) {
	w := newMockWorker()
	logger := zerolog.Nop()
	acts := NewActivities(logger)
	r := NewRegistrar(acts)

	// Should not panic
	require.NotPanics(t, func() {
		r.RegisterAll(w, config.AITaskQueue)
	})

	// Verify AI activities were registered
	require.Contains(t, w.registeredActivities, "GenerateEmbedding")
	require.Contains(t, w.registeredActivities, "GenerateSummary")
	require.Contains(t, w.registeredActivities, "ExtractAssertions")
}

// TestRegistrar_RegisterAll_MainQueue verifies registration for main queue.
func TestRegistrar_RegisterAll_MainQueue(t *testing.T) {
	w := newMockWorker()
	logger := zerolog.Nop()
	acts := NewActivities(logger)
	r := NewRegistrar(acts)

	// Should not panic
	require.NotPanics(t, func() {
		r.RegisterAll(w, config.MainTaskQueue)
	})

	// Main queue registers common activities only (currently none)
}

// TestRegistrar_RegisterAll_UnknownQueue verifies registration for unknown queue.
func TestRegistrar_RegisterAll_UnknownQueue(t *testing.T) {
	w := newMockWorker()
	logger := zerolog.Nop()
	acts := NewActivities(logger)
	r := NewRegistrar(acts)

	// Should register common activities without panic
	require.NotPanics(t, func() {
		r.RegisterAll(w, "unknown-queue")
	})
}

// TestRegistrar_MultipleRegistrations verifies multiple registrations don't cause issues.
func TestRegistrar_MultipleRegistrations(t *testing.T) {
	w := newMockWorker()
	logger := zerolog.Nop()
	acts := NewActivities(logger)
	r := NewRegistrar(acts)

	// Registering for all queues should not cause issues
	// (though in practice a worker polls one queue)
	require.NotPanics(t, func() {
		r.RegisterAll(w, config.MainTaskQueue)
		r.RegisterAll(w, config.AITaskQueue)
		r.RegisterAll(w, config.EmailTaskQueue)
	})
}
