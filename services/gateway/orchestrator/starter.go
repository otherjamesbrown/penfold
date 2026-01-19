// Package orchestrator provides a bridge between gRPC services and Temporal workflows.
// It wraps the pkg/temporal.WorkflowStarter to provide workflow-specific methods
// for starting and managing workflows from the gateway service.
package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/api/enums/v1"

	orchestratorv1 "github.com/otherjamesbrown/penfold/api/proto/orchestrator/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/temporal"
)

// Workflow type constants match the proto enum string values.
const (
	WorkflowTypeEmailProcessing       = "email_processing"
	WorkflowTypeContentIngestion      = "content_ingestion"
	WorkflowTypeDailyReview           = "daily_review"
	WorkflowTypeRelationshipDiscovery = "relationship_discovery"
)

// Workflow function names (placeholders for actual workflow functions).
// These will be registered by the worker service.
const (
	EmailWorkflowName        = "EmailProcessingWorkflow"
	ContentWorkflowName      = "ContentIngestionWorkflow"
	ReviewWorkflowName       = "DailyReviewWorkflow"
	RelationshipWorkflowName = "RelationshipDiscoveryWorkflow"
)

// EmailWorkflowInput represents the input for email processing workflows.
type EmailWorkflowInput struct {
	TenantID  string `json:"tenant_id"`
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id,omitempty"`
	FromEmail string `json:"from_email"`
	Subject   string `json:"subject,omitempty"`
}

// ContentWorkflowInput represents the input for content ingestion workflows.
type ContentWorkflowInput struct {
	TenantID   string `json:"tenant_id"`
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	ContentURL string `json:"content_url,omitempty"`
	MimeType   string `json:"mime_type,omitempty"`
}

// ReviewWorkflowInput represents the input for daily review workflows.
type ReviewWorkflowInput struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	Date     string `json:"date"` // Format: YYYY-MM-DD
}

// RelationshipWorkflowInput represents the input for relationship discovery workflows.
type RelationshipWorkflowInput struct {
	TenantID   string `json:"tenant_id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Scope      string `json:"scope,omitempty"` // e.g., "local", "global"
}

// WorkflowResult represents the common result structure returned by workflow operations.
type WorkflowResult struct {
	WorkflowID string
	RunID      string
}

// WorkflowBridge bridges gRPC service requests to Temporal workflow operations.
// It wraps the pkg/temporal.WorkflowStarter to provide higher-level, workflow-specific methods.
type WorkflowBridge struct {
	starter *temporal.WorkflowStarter
	logger  logging.Logger
}

// NewWorkflowBridge creates a new WorkflowBridge with the given WorkflowStarter.
func NewWorkflowBridge(starter *temporal.WorkflowStarter, logger logging.Logger) *WorkflowBridge {
	if logger == nil {
		logger = logging.MustGlobal()
	}
	return &WorkflowBridge{
		starter: starter,
		logger:  logger,
	}
}

// StartEmailWorkflow starts an email processing workflow.
// Returns the workflow ID and run ID.
func (b *WorkflowBridge) StartEmailWorkflow(ctx context.Context, input *EmailWorkflowInput) (*WorkflowResult, error) {
	if input == nil {
		return nil, fmt.Errorf("email workflow input is required")
	}
	if input.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if input.MessageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}
	if b.starter == nil {
		return nil, fmt.Errorf("workflow starter is not initialized")
	}

	// Generate deterministic workflow ID for idempotency
	workflowID := temporal.GenerateEmailWorkflowID(input.TenantID, input.MessageID)

	b.logger.Info("Starting email workflow",
		logging.F("workflow_id", workflowID),
		logging.F("tenant_id", input.TenantID),
		logging.F("message_id", input.MessageID),
	)

	run, err := b.starter.StartWorkflow(ctx, workflowID, EmailWorkflowName, input)
	if err != nil {
		return nil, fmt.Errorf("failed to start email workflow: %w", err)
	}

	return &WorkflowResult{
		WorkflowID: run.GetID(),
		RunID:      run.GetRunID(),
	}, nil
}

// StartContentWorkflow starts a content ingestion workflow.
// Returns the workflow ID and run ID.
func (b *WorkflowBridge) StartContentWorkflow(ctx context.Context, input *ContentWorkflowInput) (*WorkflowResult, error) {
	if input == nil {
		return nil, fmt.Errorf("content workflow input is required")
	}
	if input.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if input.SourceType == "" {
		return nil, fmt.Errorf("source_type is required")
	}
	if input.SourceID == "" {
		return nil, fmt.Errorf("source_id is required")
	}
	if b.starter == nil {
		return nil, fmt.Errorf("workflow starter is not initialized")
	}

	// Generate deterministic workflow ID for idempotency
	workflowID := temporal.GenerateIngestWorkflowID(input.TenantID, input.SourceType, input.SourceID)

	b.logger.Info("Starting content workflow",
		logging.F("workflow_id", workflowID),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_type", input.SourceType),
		logging.F("source_id", input.SourceID),
	)

	run, err := b.starter.StartWorkflow(ctx, workflowID, ContentWorkflowName, input)
	if err != nil {
		return nil, fmt.Errorf("failed to start content workflow: %w", err)
	}

	return &WorkflowResult{
		WorkflowID: run.GetID(),
		RunID:      run.GetRunID(),
	}, nil
}

// StartReviewWorkflow starts a daily review workflow.
// Returns the workflow ID and run ID.
func (b *WorkflowBridge) StartReviewWorkflow(ctx context.Context, input *ReviewWorkflowInput) (*WorkflowResult, error) {
	if input == nil {
		return nil, fmt.Errorf("review workflow input is required")
	}
	if input.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if input.UserID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	if input.Date == "" {
		return nil, fmt.Errorf("date is required")
	}
	if b.starter == nil {
		return nil, fmt.Errorf("workflow starter is not initialized")
	}

	// Generate deterministic workflow ID for idempotency
	// Format: review-{tenant_id}-{user_id}-{date}
	workflowID := temporal.GenerateDeterministicWorkflowID("review", input.TenantID, fmt.Sprintf("%s-%s", input.UserID, input.Date))

	b.logger.Info("Starting review workflow",
		logging.F("workflow_id", workflowID),
		logging.F("tenant_id", input.TenantID),
		logging.F("user_id", input.UserID),
		logging.F("date", input.Date),
	)

	run, err := b.starter.StartWorkflow(ctx, workflowID, ReviewWorkflowName, input)
	if err != nil {
		return nil, fmt.Errorf("failed to start review workflow: %w", err)
	}

	return &WorkflowResult{
		WorkflowID: run.GetID(),
		RunID:      run.GetRunID(),
	}, nil
}

// StartRelationshipWorkflow starts a relationship discovery workflow.
// Returns the workflow ID and run ID.
func (b *WorkflowBridge) StartRelationshipWorkflow(ctx context.Context, input *RelationshipWorkflowInput) (*WorkflowResult, error) {
	if input == nil {
		return nil, fmt.Errorf("relationship workflow input is required")
	}
	if input.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if input.EntityType == "" {
		return nil, fmt.Errorf("entity_type is required")
	}
	if input.EntityID == "" {
		return nil, fmt.Errorf("entity_id is required")
	}
	if b.starter == nil {
		return nil, fmt.Errorf("workflow starter is not initialized")
	}

	// Generate deterministic workflow ID for idempotency
	// Format: relationship-{tenant_id}-{entity_type}-{entity_id}
	workflowID := fmt.Sprintf("relationship-%s-%s-%s", input.TenantID, input.EntityType, input.EntityID)

	b.logger.Info("Starting relationship workflow",
		logging.F("workflow_id", workflowID),
		logging.F("tenant_id", input.TenantID),
		logging.F("entity_type", input.EntityType),
		logging.F("entity_id", input.EntityID),
	)

	run, err := b.starter.StartWorkflow(ctx, workflowID, RelationshipWorkflowName, input)
	if err != nil {
		return nil, fmt.Errorf("failed to start relationship workflow: %w", err)
	}

	return &WorkflowResult{
		WorkflowID: run.GetID(),
		RunID:      run.GetRunID(),
	}, nil
}

// GetWorkflowStatus retrieves the status of a workflow execution.
func (b *WorkflowBridge) GetWorkflowStatus(ctx context.Context, workflowID string) (*orchestratorv1.WorkflowExecution, error) {
	if workflowID == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}

	// Describe the workflow execution using the client directly
	desc, err := b.starter.Client().DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil {
		return nil, fmt.Errorf("failed to describe workflow: %w", err)
	}

	// Map Temporal status to our status string
	status := mapWorkflowStatus(desc.WorkflowExecutionInfo.Status)

	execution := &orchestratorv1.WorkflowExecution{
		WorkflowId:        workflowID,
		RunId:             desc.WorkflowExecutionInfo.Execution.RunId,
		WorkflowType:      desc.WorkflowExecutionInfo.Type.Name,
		Status:            status,
		TaskQueue:         desc.WorkflowExecutionInfo.TaskQueue,
		PendingActivities: int32(len(desc.PendingActivities)),
		HistoryLength:     desc.WorkflowExecutionInfo.HistoryLength,
	}

	// Set start time if available (already a timestamppb.Timestamp from Temporal API)
	if desc.WorkflowExecutionInfo.StartTime != nil {
		execution.StartTime = desc.WorkflowExecutionInfo.StartTime
	}

	// Set close time if available (already a timestamppb.Timestamp from Temporal API)
	if desc.WorkflowExecutionInfo.CloseTime != nil {
		execution.CloseTime = desc.WorkflowExecutionInfo.CloseTime
	}

	return execution, nil
}

// CancelWorkflow cancels a running workflow execution.
func (b *WorkflowBridge) CancelWorkflow(ctx context.Context, workflowID string) error {
	if workflowID == "" {
		return fmt.Errorf("workflow_id is required")
	}

	b.logger.Info("Canceling workflow",
		logging.F("workflow_id", workflowID),
	)

	// Cancel with empty run ID to cancel the latest run
	if err := b.starter.CancelWorkflow(ctx, workflowID, ""); err != nil {
		return fmt.Errorf("failed to cancel workflow: %w", err)
	}

	return nil
}

// mapWorkflowStatus maps Temporal workflow execution status to string representation.
func mapWorkflowStatus(status enums.WorkflowExecutionStatus) string {
	switch status {
	case enums.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return "Running"
	case enums.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return "Completed"
	case enums.WORKFLOW_EXECUTION_STATUS_FAILED:
		return "Failed"
	case enums.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return "Canceled"
	case enums.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return "Terminated"
	case enums.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return "ContinuedAsNew"
	case enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return "TimedOut"
	default:
		return "Unknown"
	}
}

// GenerateIdempotencyKey generates a deterministic key from workflow type and input.
// This can be used to ensure the same request produces the same workflow ID.
func GenerateIdempotencyKey(workflowType string, input []byte) string {
	hash := sha256.New()
	hash.Write([]byte(workflowType))
	hash.Write(input)
	return hex.EncodeToString(hash.Sum(nil))[:16]
}

// OrchestratorService implements the OrchestratorServiceServer interface from the proto.
// It provides the gRPC endpoints for workflow management.
type OrchestratorService struct {
	orchestratorv1.UnimplementedOrchestratorServiceServer
	bridge    *WorkflowBridge
	logger    logging.Logger
	startTime time.Time
}

// NewOrchestratorService creates a new OrchestratorService.
func NewOrchestratorService(bridge *WorkflowBridge, logger logging.Logger) *OrchestratorService {
	if logger == nil {
		logger = logging.MustGlobal()
	}
	return &OrchestratorService{
		bridge:    bridge,
		logger:    logger,
		startTime: time.Now(),
	}
}

// StartWorkflow initiates a new workflow execution based on the request type.
func (s *OrchestratorService) StartWorkflow(ctx context.Context, req *orchestratorv1.StartWorkflowRequest) (*orchestratorv1.StartWorkflowResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	if req.WorkflowType == "" {
		return nil, fmt.Errorf("workflow_type is required")
	}
	if req.TenantId == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	s.logger.Info("StartWorkflow request received",
		logging.F("workflow_type", req.WorkflowType),
		logging.F("tenant_id", req.TenantId),
	)

	var result *WorkflowResult
	var err error

	switch req.WorkflowType {
	case WorkflowTypeEmailProcessing:
		var input EmailWorkflowInput
		if err := json.Unmarshal(req.Input, &input); err != nil {
			return nil, fmt.Errorf("failed to unmarshal email workflow input: %w", err)
		}
		input.TenantID = req.TenantId // Ensure tenant ID is set from request
		result, err = s.bridge.StartEmailWorkflow(ctx, &input)

	case WorkflowTypeContentIngestion:
		var input ContentWorkflowInput
		if err := json.Unmarshal(req.Input, &input); err != nil {
			return nil, fmt.Errorf("failed to unmarshal content workflow input: %w", err)
		}
		input.TenantID = req.TenantId
		result, err = s.bridge.StartContentWorkflow(ctx, &input)

	case WorkflowTypeDailyReview:
		var input ReviewWorkflowInput
		if err := json.Unmarshal(req.Input, &input); err != nil {
			return nil, fmt.Errorf("failed to unmarshal review workflow input: %w", err)
		}
		input.TenantID = req.TenantId
		result, err = s.bridge.StartReviewWorkflow(ctx, &input)

	case WorkflowTypeRelationshipDiscovery:
		var input RelationshipWorkflowInput
		if err := json.Unmarshal(req.Input, &input); err != nil {
			return nil, fmt.Errorf("failed to unmarshal relationship workflow input: %w", err)
		}
		input.TenantID = req.TenantId
		result, err = s.bridge.StartRelationshipWorkflow(ctx, &input)

	default:
		return nil, fmt.Errorf("unsupported workflow type: %s", req.WorkflowType)
	}

	if err != nil {
		s.logger.Error("Failed to start workflow",
			logging.F("workflow_type", req.WorkflowType),
			logging.Err(err),
		)
		return nil, err
	}

	s.logger.Info("Workflow started successfully",
		logging.F("workflow_id", result.WorkflowID),
		logging.F("run_id", result.RunID),
	)

	return &orchestratorv1.StartWorkflowResponse{
		WorkflowId: result.WorkflowID,
		RunId:      result.RunID,
	}, nil
}

// GetWorkflowStatus retrieves the status of a specific workflow execution.
func (s *OrchestratorService) GetWorkflowStatus(ctx context.Context, req *orchestratorv1.GetWorkflowStatusRequest) (*orchestratorv1.GetWorkflowStatusResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	if req.WorkflowId == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}

	s.logger.Debug("GetWorkflowStatus request received",
		logging.F("workflow_id", req.WorkflowId),
	)

	execution, err := s.bridge.GetWorkflowStatus(ctx, req.WorkflowId)
	if err != nil {
		s.logger.Error("Failed to get workflow status",
			logging.F("workflow_id", req.WorkflowId),
			logging.Err(err),
		)
		return nil, err
	}

	return &orchestratorv1.GetWorkflowStatusResponse{
		Execution: execution,
	}, nil
}

// CancelWorkflow requests cancellation of a running workflow.
func (s *OrchestratorService) CancelWorkflow(ctx context.Context, req *orchestratorv1.CancelWorkflowRequest) (*orchestratorv1.CancelWorkflowResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	if req.WorkflowId == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}

	s.logger.Info("CancelWorkflow request received",
		logging.F("workflow_id", req.WorkflowId),
		logging.F("reason", req.Reason),
	)

	if err := s.bridge.CancelWorkflow(ctx, req.WorkflowId); err != nil {
		s.logger.Error("Failed to cancel workflow",
			logging.F("workflow_id", req.WorkflowId),
			logging.Err(err),
		)
		return &orchestratorv1.CancelWorkflowResponse{
			Accepted: false,
			Message:  fmt.Sprintf("failed to cancel workflow: %v", err),
		}, nil
	}

	return &orchestratorv1.CancelWorkflowResponse{
		Accepted: true,
		Message:  "cancellation request accepted",
	}, nil
}

// Health checks the health of the orchestrator service.
func (s *OrchestratorService) Health(ctx context.Context, req *orchestratorv1.HealthRequest) (*orchestratorv1.HealthResponse, error) {
	uptime := int64(time.Since(s.startTime).Seconds())

	// Check Temporal connectivity
	components := make([]*orchestratorv1.ComponentHealth, 0)

	temporalHealthy := s.bridge.starter.Client() != nil
	temporalMessage := "connected"
	if !temporalHealthy {
		temporalMessage = "not connected"
	}

	components = append(components, &orchestratorv1.ComponentHealth{
		Name:    "temporal",
		Healthy: temporalHealthy,
		Message: temporalMessage,
	})

	return &orchestratorv1.HealthResponse{
		Healthy:       temporalHealthy,
		Message:       "orchestrator service is running",
		Components:    components,
		Version:       "0.1.0",
		UptimeSeconds: uptime,
	}, nil
}
