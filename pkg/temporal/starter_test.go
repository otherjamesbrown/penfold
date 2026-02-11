package temporal

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"go.temporal.io/sdk/client"
)

// mockTemporalClient is a test-only implementation that captures StartWorkflowOptions.
// It implements only the ExecuteWorkflow method; other methods are not used in these tests.
type mockTemporalClient struct {
	// We embed a nil client to satisfy the interface requirement
	client.Client
	executeWorkflowCallCount int
	capturedOptions          *client.StartWorkflowOptions
	capturedWorkflow         interface{}
	capturedInput            interface{}
}

// ExecuteWorkflow captures the options and returns a mock workflow run.
func (m *mockTemporalClient) ExecuteWorkflow(
	ctx context.Context,
	options client.StartWorkflowOptions,
	workflow interface{},
	args ...interface{},
) (client.WorkflowRun, error) {
	m.executeWorkflowCallCount++
	// Make a copy of the options struct to capture its state
	m.capturedOptions = &client.StartWorkflowOptions{
		ID:                       options.ID,
		TaskQueue:                options.TaskQueue,
		WorkflowIDReusePolicy:    options.WorkflowIDReusePolicy,
		WorkflowIDConflictPolicy: options.WorkflowIDConflictPolicy,
	}
	m.capturedWorkflow = workflow
	if len(args) > 0 {
		m.capturedInput = args[0]
	}
	return &mockWorkflowRun{
		workflowID: options.ID,
		runID:      "test-run-id",
	}, nil
}

// mockWorkflowRun is a test-only implementation of client.WorkflowRun.
type mockWorkflowRun struct {
	// Embed a nil WorkflowRun to satisfy the interface
	client.WorkflowRun
	workflowID string
	runID      string
}

func (m *mockWorkflowRun) GetID() string {
	return m.workflowID
}

func (m *mockWorkflowRun) GetRunID() string {
	return m.runID
}

func (m *mockWorkflowRun) Get(ctx context.Context, valuePtr interface{}) error {
	return nil
}

func TestGenerateWorkflowID(t *testing.T) {
	t.Run("BasicGeneration", func(t *testing.T) {
		prefix := "test-workflow"
		id := GenerateWorkflowID(prefix)

		if !strings.HasPrefix(id, prefix+"-") {
			t.Errorf("expected ID to start with %s-, got %s", prefix, id)
		}

		// Should start with prefix and contain a UUID
		// Format: prefix-uuid (where uuid is 36 chars)
		expectedMinLen := len(prefix) + 1 + 36 // prefix + hyphen + UUID
		if len(id) != expectedMinLen {
			t.Errorf("expected ID length %d, got %d for ID: %s", expectedMinLen, len(id), id)
		}
	})

	t.Run("Uniqueness", func(t *testing.T) {
		ids := make(map[string]bool)
		prefix := "unique-test"

		// Generate 100 IDs and ensure all are unique
		for i := 0; i < 100; i++ {
			id := GenerateWorkflowID(prefix)
			if ids[id] {
				t.Errorf("duplicate ID generated: %s", id)
			}
			ids[id] = true
		}
	})

	t.Run("EmptyPrefix", func(t *testing.T) {
		id := GenerateWorkflowID("")
		// Should still generate a valid ID starting with -
		if !strings.HasPrefix(id, "-") {
			t.Errorf("expected ID to start with -, got %s", id)
		}
	})

	t.Run("SpecialCharactersInPrefix", func(t *testing.T) {
		prefix := "my_workflow_v2"
		id := GenerateWorkflowID(prefix)

		if !strings.HasPrefix(id, prefix+"-") {
			t.Errorf("expected ID to start with %s-, got %s", prefix, id)
		}
	})

	t.Run("UUIDFormat", func(t *testing.T) {
		id := GenerateWorkflowID("test")
		// Extract the UUID part
		parts := strings.SplitN(id, "-", 2)
		if len(parts) != 2 {
			t.Fatalf("expected 2 parts, got %d", len(parts))
		}

		uuid := parts[1]
		// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
		uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
		if !uuidRegex.MatchString(uuid) {
			t.Errorf("expected valid UUID format, got %s", uuid)
		}
	})
}

func TestGenerateTimestampedWorkflowID(t *testing.T) {
	t.Run("BasicGeneration", func(t *testing.T) {
		prefix := "batch"
		id := GenerateTimestampedWorkflowID(prefix)

		if !strings.HasPrefix(id, prefix+"-") {
			t.Errorf("expected ID to start with %s-, got %s", prefix, id)
		}
	})

	t.Run("FormatValidation", func(t *testing.T) {
		prefix := "batch"
		id := GenerateTimestampedWorkflowID(prefix)

		// Format: prefix-YYYYMMDD-HHMMSS-uuid8
		parts := strings.Split(id, "-")
		if len(parts) < 4 {
			t.Errorf("expected at least 4 parts separated by -, got %d", len(parts))
		}

		// Check timestamp format (YYYYMMDD)
		if len(parts[1]) != 8 {
			t.Errorf("expected date part to be 8 chars (YYYYMMDD), got %d", len(parts[1]))
		}

		// Check timestamp format (HHMMSS)
		if len(parts[2]) != 6 {
			t.Errorf("expected time part to be 6 chars (HHMMSS), got %d", len(parts[2]))
		}

		// Check short UUID (8 chars)
		if len(parts[3]) != 8 {
			t.Errorf("expected UUID part to be 8 chars, got %d", len(parts[3]))
		}
	})

	t.Run("DatePartIsNumeric", func(t *testing.T) {
		id := GenerateTimestampedWorkflowID("test")
		parts := strings.Split(id, "-")

		dateRegex := regexp.MustCompile(`^\d{8}$`)
		if !dateRegex.MatchString(parts[1]) {
			t.Errorf("expected date part to be numeric YYYYMMDD, got %s", parts[1])
		}

		timeRegex := regexp.MustCompile(`^\d{6}$`)
		if !timeRegex.MatchString(parts[2]) {
			t.Errorf("expected time part to be numeric HHMMSS, got %s", parts[2])
		}
	})

	t.Run("UniqueTimestampedIDs", func(t *testing.T) {
		// Even with same prefix, IDs should be unique due to UUID portion
		id1 := GenerateTimestampedWorkflowID("batch")
		id2 := GenerateTimestampedWorkflowID("batch")

		if id1 == id2 {
			t.Error("expected different IDs for consecutive calls")
		}
	})
}

func TestGenerateDeterministicWorkflowID(t *testing.T) {
	t.Run("BasicGeneration", func(t *testing.T) {
		id := GenerateDeterministicWorkflowID("process", "email", "msg123")

		expected := "process-email-msg123"
		if id != expected {
			t.Errorf("expected %s, got %s", expected, id)
		}
	})

	t.Run("Idempotency", func(t *testing.T) {
		// Same inputs should always produce same ID
		for i := 0; i < 10; i++ {
			id := GenerateDeterministicWorkflowID("process", "email", "msg123")
			expected := "process-email-msg123"
			if id != expected {
				t.Errorf("iteration %d: expected %s, got %s", i, expected, id)
			}
		}
	})

	t.Run("DifferentInputsDifferentIDs", func(t *testing.T) {
		id1 := GenerateDeterministicWorkflowID("process", "email", "msg123")
		id2 := GenerateDeterministicWorkflowID("process", "email", "msg456")
		id3 := GenerateDeterministicWorkflowID("process", "document", "msg123")
		id4 := GenerateDeterministicWorkflowID("analyze", "email", "msg123")

		ids := []string{id1, id2, id3, id4}
		seen := make(map[string]bool)
		for _, id := range ids {
			if seen[id] {
				t.Errorf("duplicate ID found: %s", id)
			}
			seen[id] = true
		}
	})

	t.Run("EmptyInputs", func(t *testing.T) {
		id := GenerateDeterministicWorkflowID("", "", "")
		expected := "--"
		if id != expected {
			t.Errorf("expected %s, got %s", expected, id)
		}
	})

	t.Run("SpecialCharactersPreserved", func(t *testing.T) {
		id := GenerateDeterministicWorkflowID("my_prefix", "entity_type", "id_123")
		expected := "my_prefix-entity_type-id_123"
		if id != expected {
			t.Errorf("expected %s, got %s", expected, id)
		}
	})
}

func TestGenerateEmailWorkflowID(t *testing.T) {
	t.Run("BasicGeneration", func(t *testing.T) {
		id := GenerateEmailWorkflowID("tenant1", "msg123")

		expected := "email-tenant1-msg123"
		if id != expected {
			t.Errorf("expected %s, got %s", expected, id)
		}
	})

	t.Run("Idempotency", func(t *testing.T) {
		id1 := GenerateEmailWorkflowID("tenant1", "msg123")
		id2 := GenerateEmailWorkflowID("tenant1", "msg123")
		if id1 != id2 {
			t.Error("expected same ID for same inputs")
		}
	})

	t.Run("DifferentTenantsUnique", func(t *testing.T) {
		id1 := GenerateEmailWorkflowID("tenant1", "msg123")
		id2 := GenerateEmailWorkflowID("tenant2", "msg123")
		if id1 == id2 {
			t.Error("expected different IDs for different tenants")
		}
	})

	t.Run("DifferentMessagesUnique", func(t *testing.T) {
		id1 := GenerateEmailWorkflowID("tenant1", "msg123")
		id2 := GenerateEmailWorkflowID("tenant1", "msg456")
		if id1 == id2 {
			t.Error("expected different IDs for different messages")
		}
	})

	t.Run("PrefixIsEmail", func(t *testing.T) {
		id := GenerateEmailWorkflowID("any", "any")
		if !strings.HasPrefix(id, "email-") {
			t.Errorf("expected ID to start with email-, got %s", id)
		}
	})
}

func TestGenerateIngestWorkflowID(t *testing.T) {
	t.Run("BasicGeneration", func(t *testing.T) {
		id := GenerateIngestWorkflowID("tenant1", "document", "doc456")

		expected := "ingest-tenant1-document-doc456"
		if id != expected {
			t.Errorf("expected %s, got %s", expected, id)
		}
	})

	t.Run("Idempotency", func(t *testing.T) {
		id1 := GenerateIngestWorkflowID("tenant1", "document", "doc456")
		id2 := GenerateIngestWorkflowID("tenant1", "document", "doc456")
		if id1 != id2 {
			t.Error("expected same ID for same inputs")
		}
	})

	t.Run("DifferentSourceTypesUnique", func(t *testing.T) {
		id1 := GenerateIngestWorkflowID("tenant1", "document", "id1")
		id2 := GenerateIngestWorkflowID("tenant1", "email", "id1")
		if id1 == id2 {
			t.Error("expected different IDs for different source types")
		}
	})

	t.Run("PrefixIsIngest", func(t *testing.T) {
		id := GenerateIngestWorkflowID("any", "any", "any")
		if !strings.HasPrefix(id, "ingest-") {
			t.Errorf("expected ID to start with ingest-, got %s", id)
		}
	})

	t.Run("MultipleSourceTypes", func(t *testing.T) {
		sourceTypes := []string{"document", "email", "slack", "meeting", "note"}
		ids := make(map[string]bool)

		for _, st := range sourceTypes {
			id := GenerateIngestWorkflowID("tenant", st, "id123")
			if ids[id] {
				t.Errorf("duplicate ID for source type %s", st)
			}
			ids[id] = true
		}
	})
}

func TestNewWorkflowStarter(t *testing.T) {
	t.Run("BasicCreation", func(t *testing.T) {
		starter := NewWorkflowStarter(nil, "test-queue")

		if starter == nil {
			t.Fatal("expected non-nil starter")
		}

		if starter.TaskQueue() != "test-queue" {
			t.Errorf("expected task queue test-queue, got %s", starter.TaskQueue())
		}

		if starter.Client() != nil {
			t.Error("expected nil client since we passed nil")
		}
	})

	t.Run("EmptyTaskQueue", func(t *testing.T) {
		starter := NewWorkflowStarter(nil, "")

		if starter.TaskQueue() != "" {
			t.Errorf("expected empty task queue, got %s", starter.TaskQueue())
		}
	})

	t.Run("SpecialCharactersInTaskQueue", func(t *testing.T) {
		queues := []string{
			"penfold-ai-processing",
			"my_queue_v2",
			"queue.with.dots",
		}

		for _, q := range queues {
			starter := NewWorkflowStarter(nil, q)
			if starter.TaskQueue() != q {
				t.Errorf("expected task queue %s, got %s", q, starter.TaskQueue())
			}
		}
	})
}

func TestWorkflowStarterAccessors(t *testing.T) {
	t.Run("Client", func(t *testing.T) {
		starter := NewWorkflowStarter(nil, "queue")
		if starter.Client() != nil {
			t.Error("expected nil client")
		}
	})

	t.Run("TaskQueue", func(t *testing.T) {
		starter := NewWorkflowStarter(nil, "my-task-queue")
		if starter.TaskQueue() != "my-task-queue" {
			t.Errorf("expected my-task-queue, got %s", starter.TaskQueue())
		}
	})
}

// Benchmark tests for workflow ID generation
func BenchmarkGenerateWorkflowID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateWorkflowID("benchmark-prefix")
	}
}

func BenchmarkGenerateTimestampedWorkflowID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateTimestampedWorkflowID("benchmark-prefix")
	}
}

func BenchmarkGenerateDeterministicWorkflowID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateDeterministicWorkflowID("prefix", "entity", "id")
	}
}

func BenchmarkGenerateEmailWorkflowID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateEmailWorkflowID("tenant", "message")
	}
}

func BenchmarkGenerateIngestWorkflowID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateIngestWorkflowID("tenant", "document", "doc123")
	}
}

// TestStartWorkflow_SetsReusePolicy verifies that StartWorkflow sets WorkflowIDReusePolicy.
// This test reproduces bug pf-02b536 - it will FAIL because the policy is not currently set.
func TestStartWorkflow_SetsReusePolicy(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockTemporalClient{}

	starter := NewWorkflowStarter(mockClient, "test-queue")

	_, err := starter.StartWorkflow(ctx, "test-workflow-id", "TestWorkflow", nil)

	// Should succeed (we're checking the options, not workflow execution)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ExecuteWorkflow was called
	if mockClient.executeWorkflowCallCount != 1 {
		t.Errorf("expected ExecuteWorkflow to be called once, got %d", mockClient.executeWorkflowCallCount)
	}

	// Verify the options were captured
	if mockClient.capturedOptions == nil {
		t.Fatal("expected options to be captured")
	}

	// BUG: This assertion will FAIL because WorkflowIDReusePolicy is not currently set
	// Expected: WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE
	// Actual: 0 (unset)
	if mockClient.capturedOptions.WorkflowIDReusePolicy == 0 {
		t.Error("WorkflowIDReusePolicy not set - this is the bug we're reproducing")
	}

	// Also verify WorkflowIDConflictPolicy is set
	// BUG: This will also FAIL
	if mockClient.capturedOptions.WorkflowIDConflictPolicy == 0 {
		t.Error("WorkflowIDConflictPolicy not set - this is the bug we're reproducing")
	}
}
