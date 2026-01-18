package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/enums/v1"

	orchestratorv1 "github.com/otherjamesbrown/penfold/api/proto/orchestrator/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/temporal"
)

// MockWorkflowRun is a mock implementation of client.WorkflowRun.
type MockWorkflowRun struct {
	workflowID string
	runID      string
}

func NewMockWorkflowRun(workflowID, runID string) *MockWorkflowRun {
	return &MockWorkflowRun{
		workflowID: workflowID,
		runID:      runID,
	}
}

func (m *MockWorkflowRun) GetID() string {
	return m.workflowID
}

func (m *MockWorkflowRun) GetRunID() string {
	return m.runID
}

// testLogger creates a logger for testing.
func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = logging.LevelDebug
	return logging.NewLogger(cfg)
}

// TestEmailWorkflowInput_Validation tests the input validation for email workflows.
func TestEmailWorkflowInput_Validation(t *testing.T) {
	tests := []struct {
		name        string
		input       *EmailWorkflowInput
		wantErr     bool
		errContains string
	}{
		{
			name: "valid input with nil starter",
			input: &EmailWorkflowInput{
				TenantID:  "tenant1",
				MessageID: "msg123",
				FromEmail: "test@example.com",
				Subject:   "Test Subject",
			},
			wantErr:     true,
			errContains: "starter is not initialized", // Validation passes but starter check fails
		},
		{
			name:        "nil input",
			input:       nil,
			wantErr:     true,
			errContains: "input is required",
		},
		{
			name: "missing tenant_id",
			input: &EmailWorkflowInput{
				MessageID: "msg123",
			},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name: "missing message_id",
			input: &EmailWorkflowInput{
				TenantID: "tenant1",
			},
			wantErr:     true,
			errContains: "message_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testLogger()

			// Create a bridge with nil starter to test validation only
			// The validation happens before the starter is used
			bridge := NewWorkflowBridge(nil, logger)

			_, err := bridge.StartEmailWorkflow(context.Background(), tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

// TestContentWorkflowInput_Validation tests the input validation for content workflows.
func TestContentWorkflowInput_Validation(t *testing.T) {
	tests := []struct {
		name        string
		input       *ContentWorkflowInput
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil input",
			input:       nil,
			wantErr:     true,
			errContains: "input is required",
		},
		{
			name: "missing tenant_id",
			input: &ContentWorkflowInput{
				SourceType: "document",
				SourceID:   "doc123",
			},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name: "missing source_type",
			input: &ContentWorkflowInput{
				TenantID: "tenant1",
				SourceID: "doc123",
			},
			wantErr:     true,
			errContains: "source_type is required",
		},
		{
			name: "missing source_id",
			input: &ContentWorkflowInput{
				TenantID:   "tenant1",
				SourceType: "document",
			},
			wantErr:     true,
			errContains: "source_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testLogger()
			bridge := NewWorkflowBridge(nil, logger)

			_, err := bridge.StartContentWorkflow(context.Background(), tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

// TestReviewWorkflowInput_Validation tests the input validation for review workflows.
func TestReviewWorkflowInput_Validation(t *testing.T) {
	tests := []struct {
		name        string
		input       *ReviewWorkflowInput
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil input",
			input:       nil,
			wantErr:     true,
			errContains: "input is required",
		},
		{
			name: "missing tenant_id",
			input: &ReviewWorkflowInput{
				UserID: "user123",
				Date:   "2024-01-18",
			},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name: "missing user_id",
			input: &ReviewWorkflowInput{
				TenantID: "tenant1",
				Date:     "2024-01-18",
			},
			wantErr:     true,
			errContains: "user_id is required",
		},
		{
			name: "missing date",
			input: &ReviewWorkflowInput{
				TenantID: "tenant1",
				UserID:   "user123",
			},
			wantErr:     true,
			errContains: "date is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testLogger()
			bridge := NewWorkflowBridge(nil, logger)

			_, err := bridge.StartReviewWorkflow(context.Background(), tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

// TestRelationshipWorkflowInput_Validation tests the input validation for relationship workflows.
func TestRelationshipWorkflowInput_Validation(t *testing.T) {
	tests := []struct {
		name        string
		input       *RelationshipWorkflowInput
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil input",
			input:       nil,
			wantErr:     true,
			errContains: "input is required",
		},
		{
			name: "missing tenant_id",
			input: &RelationshipWorkflowInput{
				EntityType: "contact",
				EntityID:   "contact123",
			},
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name: "missing entity_type",
			input: &RelationshipWorkflowInput{
				TenantID: "tenant1",
				EntityID: "contact123",
			},
			wantErr:     true,
			errContains: "entity_type is required",
		},
		{
			name: "missing entity_id",
			input: &RelationshipWorkflowInput{
				TenantID:   "tenant1",
				EntityType: "contact",
			},
			wantErr:     true,
			errContains: "entity_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := testLogger()
			bridge := NewWorkflowBridge(nil, logger)

			_, err := bridge.StartRelationshipWorkflow(context.Background(), tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

// TestCancelWorkflow_Validation tests the input validation for cancel workflow.
func TestCancelWorkflow_Validation(t *testing.T) {
	logger := testLogger()
	bridge := NewWorkflowBridge(nil, logger)

	// Test empty workflow ID
	err := bridge.CancelWorkflow(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow_id is required")
}

// TestGetWorkflowStatus_Validation tests the input validation for get workflow status.
func TestGetWorkflowStatus_Validation(t *testing.T) {
	logger := testLogger()
	bridge := NewWorkflowBridge(nil, logger)

	// Test empty workflow ID
	_, err := bridge.GetWorkflowStatus(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow_id is required")
}

// TestGenerateIdempotencyKey tests the idempotency key generation.
func TestGenerateIdempotencyKey(t *testing.T) {
	// Same input should produce same key
	input1 := []byte(`{"message_id": "msg123"}`)
	key1 := GenerateIdempotencyKey("email_processing", input1)
	key2 := GenerateIdempotencyKey("email_processing", input1)
	assert.Equal(t, key1, key2, "same input should produce same key")

	// Different input should produce different key
	input2 := []byte(`{"message_id": "msg456"}`)
	key3 := GenerateIdempotencyKey("email_processing", input2)
	assert.NotEqual(t, key1, key3, "different input should produce different key")

	// Different workflow type should produce different key
	key4 := GenerateIdempotencyKey("content_ingestion", input1)
	assert.NotEqual(t, key1, key4, "different workflow type should produce different key")

	// Key should be 16 characters
	assert.Len(t, key1, 16)
}

// TestOrchestratorService_StartWorkflow_Validation tests request validation.
func TestOrchestratorService_StartWorkflow_Validation(t *testing.T) {
	logger := testLogger()
	bridge := NewWorkflowBridge(nil, logger)
	service := NewOrchestratorService(bridge, logger)

	// Test nil request
	resp, err := service.StartWorkflow(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request is required")
	assert.Nil(t, resp)

	// Test missing workflow type
	resp, err = service.StartWorkflow(context.Background(), &orchestratorv1.StartWorkflowRequest{
		TenantId: "test-tenant",
		Input:    []byte(`{}`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow_type is required")

	// Test missing tenant ID
	resp, err = service.StartWorkflow(context.Background(), &orchestratorv1.StartWorkflowRequest{
		WorkflowType: WorkflowTypeEmailProcessing,
		Input:        []byte(`{}`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant_id is required")

	// Test unsupported workflow type
	resp, err = service.StartWorkflow(context.Background(), &orchestratorv1.StartWorkflowRequest{
		WorkflowType: "unknown_type",
		TenantId:     "test-tenant",
		Input:        []byte(`{}`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported workflow type")
}

// TestOrchestratorService_GetWorkflowStatus_Validation tests request validation.
func TestOrchestratorService_GetWorkflowStatus_Validation(t *testing.T) {
	logger := testLogger()
	bridge := NewWorkflowBridge(nil, logger)
	service := NewOrchestratorService(bridge, logger)

	// Test nil request
	resp, err := service.GetWorkflowStatus(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request is required")
	assert.Nil(t, resp)

	// Test missing workflow ID
	resp, err = service.GetWorkflowStatus(context.Background(), &orchestratorv1.GetWorkflowStatusRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow_id is required")
}

// TestOrchestratorService_CancelWorkflow_Validation tests request validation.
func TestOrchestratorService_CancelWorkflow_Validation(t *testing.T) {
	logger := testLogger()
	bridge := NewWorkflowBridge(nil, logger)
	service := NewOrchestratorService(bridge, logger)

	// Test nil request
	resp, err := service.CancelWorkflow(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request is required")
	assert.Nil(t, resp)

	// Test missing workflow ID
	resp, err = service.CancelWorkflow(context.Background(), &orchestratorv1.CancelWorkflowRequest{
		Reason: "test reason",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow_id is required")
}

// TestOrchestratorService_Health tests the Health method.
func TestOrchestratorService_Health(t *testing.T) {
	logger := testLogger()
	starter := temporal.NewWorkflowStarter(nil, "test-queue")
	bridge := NewWorkflowBridge(starter, logger)
	service := NewOrchestratorService(bridge, logger)

	// Wait a bit to ensure uptime is > 0
	time.Sleep(10 * time.Millisecond)

	resp, err := service.Health(context.Background(), &orchestratorv1.HealthRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "0.1.0", resp.Version)
	assert.GreaterOrEqual(t, resp.UptimeSeconds, int64(0))
	assert.Len(t, resp.Components, 1)
	assert.Equal(t, "temporal", resp.Components[0].Name)
	// With nil client, health should report not connected
	assert.False(t, resp.Healthy)
	assert.Equal(t, "not connected", resp.Components[0].Message)
}

// TestMapWorkflowStatus tests the status mapping function.
func TestMapWorkflowStatus(t *testing.T) {
	tests := []struct {
		status   enums.WorkflowExecutionStatus
		expected string
	}{
		{enums.WORKFLOW_EXECUTION_STATUS_RUNNING, "Running"},
		{enums.WORKFLOW_EXECUTION_STATUS_COMPLETED, "Completed"},
		{enums.WORKFLOW_EXECUTION_STATUS_FAILED, "Failed"},
		{enums.WORKFLOW_EXECUTION_STATUS_CANCELED, "Canceled"},
		{enums.WORKFLOW_EXECUTION_STATUS_TERMINATED, "Terminated"},
		{enums.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW, "ContinuedAsNew"},
		{enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT, "TimedOut"},
		{enums.WORKFLOW_EXECUTION_STATUS_UNSPECIFIED, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := mapWorkflowStatus(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestWorkflowBridge_Idempotency tests that the same input produces the same workflow ID.
func TestWorkflowBridge_Idempotency(t *testing.T) {
	// Email workflow ID should be deterministic
	id1 := temporal.GenerateEmailWorkflowID("tenant1", "msg123")
	id2 := temporal.GenerateEmailWorkflowID("tenant1", "msg123")
	assert.Equal(t, id1, id2, "same input should produce same workflow ID")

	// Different input should produce different ID
	id3 := temporal.GenerateEmailWorkflowID("tenant1", "msg456")
	assert.NotEqual(t, id1, id3, "different input should produce different workflow ID")

	// Content workflow ID should be deterministic
	contentID1 := temporal.GenerateIngestWorkflowID("tenant1", "document", "doc123")
	contentID2 := temporal.GenerateIngestWorkflowID("tenant1", "document", "doc123")
	assert.Equal(t, contentID1, contentID2, "same input should produce same workflow ID")
}

// TestWorkflowTypeConstants verifies workflow type constants match expected values.
func TestWorkflowTypeConstants(t *testing.T) {
	assert.Equal(t, "email_processing", WorkflowTypeEmailProcessing)
	assert.Equal(t, "content_ingestion", WorkflowTypeContentIngestion)
	assert.Equal(t, "daily_review", WorkflowTypeDailyReview)
	assert.Equal(t, "relationship_discovery", WorkflowTypeRelationshipDiscovery)
}

// TestWorkflowNameConstants verifies workflow name constants.
func TestWorkflowNameConstants(t *testing.T) {
	assert.Equal(t, "EmailProcessingWorkflow", EmailWorkflowName)
	assert.Equal(t, "ContentIngestionWorkflow", ContentWorkflowName)
	assert.Equal(t, "DailyReviewWorkflow", ReviewWorkflowName)
	assert.Equal(t, "RelationshipDiscoveryWorkflow", RelationshipWorkflowName)
}

// TestNewWorkflowBridge tests the WorkflowBridge constructor.
func TestNewWorkflowBridge(t *testing.T) {
	logger := testLogger()
	starter := temporal.NewWorkflowStarter(nil, "test-queue")

	bridge := NewWorkflowBridge(starter, logger)
	require.NotNil(t, bridge)
	assert.NotNil(t, bridge.starter)
	assert.NotNil(t, bridge.logger)
}

// TestNewWorkflowBridge_NilLogger tests the WorkflowBridge constructor with nil logger.
func TestNewWorkflowBridge_NilLogger(t *testing.T) {
	starter := temporal.NewWorkflowStarter(nil, "test-queue")

	// Should not panic and should use a default logger
	bridge := NewWorkflowBridge(starter, nil)
	require.NotNil(t, bridge)
	assert.NotNil(t, bridge.logger)
}

// TestNewOrchestratorService tests the OrchestratorService constructor.
func TestNewOrchestratorService(t *testing.T) {
	logger := testLogger()
	starter := temporal.NewWorkflowStarter(nil, "test-queue")
	bridge := NewWorkflowBridge(starter, logger)

	service := NewOrchestratorService(bridge, logger)
	require.NotNil(t, service)
	assert.NotNil(t, service.bridge)
	assert.NotNil(t, service.logger)
}

// TestNewOrchestratorService_NilLogger tests the OrchestratorService constructor with nil logger.
func TestNewOrchestratorService_NilLogger(t *testing.T) {
	starter := temporal.NewWorkflowStarter(nil, "test-queue")
	bridge := NewWorkflowBridge(starter, nil)

	// Should not panic and should use a default logger
	service := NewOrchestratorService(bridge, nil)
	require.NotNil(t, service)
	assert.NotNil(t, service.logger)
}

// TestWorkflowInputSerialization tests that workflow inputs can be properly serialized.
func TestWorkflowInputSerialization(t *testing.T) {
	// Test EmailWorkflowInput
	emailInput := &EmailWorkflowInput{
		TenantID:  "tenant1",
		MessageID: "msg123",
		ThreadID:  "thread456",
		FromEmail: "test@example.com",
		Subject:   "Test Subject",
	}
	emailJSON, err := json.Marshal(emailInput)
	require.NoError(t, err)

	var emailDecoded EmailWorkflowInput
	err = json.Unmarshal(emailJSON, &emailDecoded)
	require.NoError(t, err)
	assert.Equal(t, emailInput.TenantID, emailDecoded.TenantID)
	assert.Equal(t, emailInput.MessageID, emailDecoded.MessageID)

	// Test ContentWorkflowInput
	contentInput := &ContentWorkflowInput{
		TenantID:   "tenant1",
		SourceType: "document",
		SourceID:   "doc456",
		ContentURL: "https://example.com/doc.pdf",
		MimeType:   "application/pdf",
	}
	contentJSON, err := json.Marshal(contentInput)
	require.NoError(t, err)

	var contentDecoded ContentWorkflowInput
	err = json.Unmarshal(contentJSON, &contentDecoded)
	require.NoError(t, err)
	assert.Equal(t, contentInput.SourceType, contentDecoded.SourceType)

	// Test ReviewWorkflowInput
	reviewInput := &ReviewWorkflowInput{
		TenantID: "tenant1",
		UserID:   "user123",
		Date:     "2024-01-18",
	}
	reviewJSON, err := json.Marshal(reviewInput)
	require.NoError(t, err)

	var reviewDecoded ReviewWorkflowInput
	err = json.Unmarshal(reviewJSON, &reviewDecoded)
	require.NoError(t, err)
	assert.Equal(t, reviewInput.Date, reviewDecoded.Date)

	// Test RelationshipWorkflowInput
	relationshipInput := &RelationshipWorkflowInput{
		TenantID:   "tenant1",
		EntityType: "contact",
		EntityID:   "contact123",
		Scope:      "local",
	}
	relationshipJSON, err := json.Marshal(relationshipInput)
	require.NoError(t, err)

	var relationshipDecoded RelationshipWorkflowInput
	err = json.Unmarshal(relationshipJSON, &relationshipDecoded)
	require.NoError(t, err)
	assert.Equal(t, relationshipInput.EntityType, relationshipDecoded.EntityType)
}

// TestOrchestratorService_StartWorkflow_InputParsing tests that input is correctly parsed.
func TestOrchestratorService_StartWorkflow_InputParsing(t *testing.T) {
	logger := testLogger()
	bridge := NewWorkflowBridge(nil, logger)
	service := NewOrchestratorService(bridge, logger)

	// Test invalid JSON for email workflow
	resp, err := service.StartWorkflow(context.Background(), &orchestratorv1.StartWorkflowRequest{
		WorkflowType: WorkflowTypeEmailProcessing,
		TenantId:     "test-tenant",
		Input:        []byte(`{invalid json`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal")
	assert.Nil(t, resp)

	// Test invalid JSON for content workflow
	resp, err = service.StartWorkflow(context.Background(), &orchestratorv1.StartWorkflowRequest{
		WorkflowType: WorkflowTypeContentIngestion,
		TenantId:     "test-tenant",
		Input:        []byte(`{invalid json`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal")

	// Test invalid JSON for review workflow
	resp, err = service.StartWorkflow(context.Background(), &orchestratorv1.StartWorkflowRequest{
		WorkflowType: WorkflowTypeDailyReview,
		TenantId:     "test-tenant",
		Input:        []byte(`{invalid json`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal")

	// Test invalid JSON for relationship workflow
	resp, err = service.StartWorkflow(context.Background(), &orchestratorv1.StartWorkflowRequest{
		WorkflowType: WorkflowTypeRelationshipDiscovery,
		TenantId:     "test-tenant",
		Input:        []byte(`{invalid json`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal")
}

// mockError is a simple error for testing.
var mockError = errors.New("mock error")

// TestWorkflowResult tests the WorkflowResult struct.
func TestWorkflowResult(t *testing.T) {
	result := &WorkflowResult{
		WorkflowID: "test-workflow-id",
		RunID:      "test-run-id",
	}

	assert.Equal(t, "test-workflow-id", result.WorkflowID)
	assert.Equal(t, "test-run-id", result.RunID)
}
