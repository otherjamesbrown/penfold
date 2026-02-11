package workflows

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// testLogger creates a logger for testing.
func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = logging.LevelDebug
	return logging.NewLogger(cfg)
}

func TestNewStarter(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()

	// Test with nil client (validation only)
	starter := NewStarter(nil, cfg, logger)
	require.NotNil(t, starter)
	assert.Equal(t, cfg, starter.config)
}

func TestNewStarter_DefaultConfig(t *testing.T) {
	logger := testLogger()

	// Test with nil config - should use default
	starter := NewStarter(nil, nil, logger)
	require.NotNil(t, starter)
	assert.NotNil(t, starter.config)
	assert.Equal(t, DefaultTemporalHostPort, starter.config.TemporalHostPort)
}

func TestNewStarter_NilLogger(t *testing.T) {
	cfg := DefaultConfig()

	// Test with nil logger - should use global
	starter := NewStarter(nil, cfg, nil)
	require.NotNil(t, starter)
}

func TestNewStarter_MetricsDisabled(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	cfg.MetricsEnabled = false

	starter := NewStarter(nil, cfg, logger)
	require.NotNil(t, starter)
	assert.Nil(t, starter.metrics)
}

func TestNewStarter_MetricsEnabled(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	cfg.MetricsEnabled = true

	starter := NewStarter(nil, cfg, logger)
	require.NotNil(t, starter)
	assert.NotNil(t, starter.metrics)
}

func TestStartIngestionWorkflow_Validation(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	starter := NewStarter(nil, cfg, logger)

	tests := []struct {
		name        string
		req         IngestRequest
		wantErr     bool
		errContains string
	}{
		{
			name:        "missing tenant_id",
			req:         IngestRequest{SourceType: "email", SourceID: "123"},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name:        "missing source_type",
			req:         IngestRequest{TenantID: "tenant1", SourceID: "123"},
			wantErr:     true,
			errContains: "source_type is required",
		},
		{
			name:        "missing source_id",
			req:         IngestRequest{TenantID: "tenant1", SourceType: "email"},
			wantErr:     true,
			errContains: "source_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := starter.StartIngestionWorkflow(context.Background(), tt.req)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

func TestStartSyncWorkflow_Validation(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	starter := NewStarter(nil, cfg, logger)

	tests := []struct {
		name        string
		req         SyncRequest
		wantErr     bool
		errContains string
	}{
		{
			name:        "missing tenant_id",
			req:         SyncRequest{SourceType: "gmail"},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name:        "missing source_type",
			req:         SyncRequest{TenantID: "tenant1"},
			wantErr:     true,
			errContains: "source_type is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := starter.StartSyncWorkflow(context.Background(), tt.req)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

func TestStartReviewWorkflow_Validation(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	starter := NewStarter(nil, cfg, logger)

	tests := []struct {
		name        string
		req         ReviewRequest
		wantErr     bool
		errContains string
	}{
		{
			name:        "missing tenant_id",
			req:         ReviewRequest{UserID: "user1", Date: "2024-01-18"},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name:        "missing user_id",
			req:         ReviewRequest{TenantID: "tenant1", Date: "2024-01-18"},
			wantErr:     true,
			errContains: "user_id is required",
		},
		{
			name:        "missing date",
			req:         ReviewRequest{TenantID: "tenant1", UserID: "user1"},
			wantErr:     true,
			errContains: "date is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := starter.StartReviewWorkflow(context.Background(), tt.req)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

func TestStartAnalysisWorkflow_Validation(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	starter := NewStarter(nil, cfg, logger)

	tests := []struct {
		name        string
		req         AnalysisRequest
		wantErr     bool
		errContains string
	}{
		{
			name:        "missing tenant_id",
			req:         AnalysisRequest{AnalysisType: "sentiment", TargetIDs: []string{"id1"}},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name:        "missing analysis_type",
			req:         AnalysisRequest{TenantID: "tenant1", TargetIDs: []string{"id1"}},
			wantErr:     true,
			errContains: "analysis_type is required",
		},
		{
			name:        "missing target_ids",
			req:         AnalysisRequest{TenantID: "tenant1", AnalysisType: "sentiment"},
			wantErr:     true,
			errContains: "target_ids is required",
		},
		{
			name:        "empty target_ids",
			req:         AnalysisRequest{TenantID: "tenant1", AnalysisType: "sentiment", TargetIDs: []string{}},
			wantErr:     true,
			errContains: "target_ids is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := starter.StartAnalysisWorkflow(context.Background(), tt.req)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

func TestWorkflowTypeConstants(t *testing.T) {
	assert.Equal(t, "ingestion", WorkflowTypeIngestion)
	assert.Equal(t, "sync", WorkflowTypeSync)
	assert.Equal(t, "review", WorkflowTypeReview)
	assert.Equal(t, "analysis", WorkflowTypeAnalysis)
}

func TestWorkflowNameConstants(t *testing.T) {
	assert.Equal(t, "IngestionWorkflow", IngestionWorkflowName)
	assert.Equal(t, "SyncWorkflow", SyncWorkflowName)
	assert.Equal(t, "ReviewWorkflow", ReviewWorkflowName)
	assert.Equal(t, "AnalysisWorkflow", AnalysisWorkflowName)
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected string
	}{
		{"nil error", "", "none"},
		{"timeout error", "context deadline exceeded timeout", "timeout"},
		{"connection error", "connection refused", "connection"},
		{"duplicate error", "workflow already started", "duplicate"},
		{"not found error", "workflow not found", "not_found"},
		{"invalid input error", "invalid argument", "invalid_input"},
		{"unknown error", "something went wrong", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.errMsg != "" {
				err = &testError{msg: tt.errMsg}
			}
			result := classifyError(err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestStarter_Config(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	cfg.TemporalHostPort = "custom-host:7233"

	starter := NewStarter(nil, cfg, logger)
	assert.Equal(t, cfg, starter.Config())
}

func TestStarter_Client(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()

	starter := NewStarter(nil, cfg, logger)
	assert.Nil(t, starter.Client())
}

func TestIngestRequest_Serialization(t *testing.T) {
	req := IngestRequest{
		TenantID:   "tenant1",
		SourceType: "email",
		SourceID:   "msg123",
		ContentURL: "https://example.com/content",
		MimeType:   "text/plain",
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	assert.Equal(t, "tenant1", req.TenantID)
	assert.Equal(t, "email", req.SourceType)
	assert.Equal(t, "msg123", req.SourceID)
	assert.Equal(t, "https://example.com/content", req.ContentURL)
	assert.Equal(t, "text/plain", req.MimeType)
	assert.Len(t, req.Metadata, 2)
}

func TestSyncRequest_Serialization(t *testing.T) {
	req := SyncRequest{
		TenantID:   "tenant1",
		SourceType: "gmail",
		SourceIDs:  []string{"id1", "id2"},
		FullSync:   true,
		Since:      "2024-01-18T00:00:00Z",
	}

	assert.Equal(t, "tenant1", req.TenantID)
	assert.Equal(t, "gmail", req.SourceType)
	assert.Len(t, req.SourceIDs, 2)
	assert.True(t, req.FullSync)
	assert.Equal(t, "2024-01-18T00:00:00Z", req.Since)
}

func TestReviewRequest_Serialization(t *testing.T) {
	req := ReviewRequest{
		TenantID: "tenant1",
		UserID:   "user123",
		Date:     "2024-01-18",
		Scope:    "weekly",
	}

	assert.Equal(t, "tenant1", req.TenantID)
	assert.Equal(t, "user123", req.UserID)
	assert.Equal(t, "2024-01-18", req.Date)
	assert.Equal(t, "weekly", req.Scope)
}

func TestAnalysisRequest_Serialization(t *testing.T) {
	req := AnalysisRequest{
		TenantID:     "tenant1",
		AnalysisType: "sentiment",
		TargetIDs:    []string{"doc1", "doc2"},
		Options: map[string]string{
			"depth": "deep",
		},
	}

	assert.Equal(t, "tenant1", req.TenantID)
	assert.Equal(t, "sentiment", req.AnalysisType)
	assert.Len(t, req.TargetIDs, 2)
	assert.Equal(t, "deep", req.Options["depth"])
}

func TestWorkflowStartResult_Fields(t *testing.T) {
	result := WorkflowStartResult{
		WorkflowID: "wf-123",
		RunID:      "run-456",
	}

	assert.Equal(t, "wf-123", result.WorkflowID)
	assert.Equal(t, "run-456", result.RunID)
}

// TestStartWorkflow_SetsReusePolicy verifies that startWorkflow sets WorkflowIDReusePolicy.
// This test reproduces bug pf-02b536 - it will FAIL because the policy is not currently set.
func TestStartWorkflow_SetsReusePolicy(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()

	mockClient := &mockTemporalClient{}
	starter := &Starter{
		temporalClient: mockClient,
		config:         cfg,
		logger:         logger,
		metrics:        nil, // Metrics disabled for test
	}

	ctx := context.Background()
	req := IngestRequest{
		TenantID:   "tenant1",
		SourceType: "email",
		SourceID:   "msg123",
	}

	_, err := starter.StartIngestionWorkflow(ctx, req)

	// Should succeed (we're checking the options, not workflow execution)
	require.NoError(t, err)

	// Verify ExecuteWorkflow was called
	require.Equal(t, 1, mockClient.executeWorkflowCallCount)

	// Verify the options were captured
	require.NotNil(t, mockClient.capturedOptions)

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
