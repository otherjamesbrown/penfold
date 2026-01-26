// Package workflowservice implements the WorkflowService gRPC server.
// It provides endpoints for listing, querying, and managing Temporal workflow executions.
package workflowservice

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/api/enums/v1"
	temporalworkflow "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	workflowv1 "github.com/otherjamesbrown/penfold/api/proto/workflow/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the WorkflowService gRPC server.
type Service struct {
	workflowv1.UnimplementedWorkflowServiceServer
	temporalClient client.Client
	namespace      string
	logger         logging.Logger
}

// NewService creates a new workflow service.
func NewService(temporalClient client.Client, namespace string, logger logging.Logger) *Service {
	if namespace == "" {
		namespace = "default"
	}
	return &Service{
		temporalClient: temporalClient,
		namespace:      namespace,
		logger:         logger,
	}
}

// ListWorkflows lists workflows matching the given filter criteria.
func (s *Service) ListWorkflows(ctx context.Context, req *workflowv1.ListWorkflowsRequest) (*workflowv1.ListWorkflowsResponse, error) {
	s.logger.Debug("ListWorkflows called",
		logging.F("workflow_type", req.GetWorkflowType()),
		logging.F("status", req.GetStatus().String()),
		logging.F("page_size", req.GetPageSize()),
	)

	// Build the visibility query
	query := s.buildListQuery(req)

	pageSize := int32(req.GetPageSize())
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	listReq := &workflowservice.ListWorkflowExecutionsRequest{
		Namespace:     s.namespace,
		Query:         query,
		PageSize:      pageSize,
		NextPageToken: req.GetNextPageToken(),
	}

	resp, err := s.temporalClient.ListWorkflow(ctx, listReq)
	if err != nil {
		s.logger.Error("Failed to list workflows",
			logging.F("query", query),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to list workflows: %v", err)
	}

	workflows := make([]*workflowv1.WorkflowInfo, 0, len(resp.GetExecutions()))
	for _, exec := range resp.GetExecutions() {
		workflows = append(workflows, s.executionInfoToProto(exec))
	}

	return &workflowv1.ListWorkflowsResponse{
		Workflows:     workflows,
		NextPageToken: resp.GetNextPageToken(),
	}, nil
}

// GetWorkflowStatus retrieves the status of a specific workflow.
func (s *Service) GetWorkflowStatus(ctx context.Context, req *workflowv1.GetWorkflowStatusRequest) (*workflowv1.GetWorkflowStatusResponse, error) {
	if req.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id is required")
	}

	s.logger.Debug("GetWorkflowStatus called",
		logging.F("workflow_id", req.GetWorkflowId()),
		logging.F("run_id", req.GetRunId()),
	)

	desc, err := s.temporalClient.DescribeWorkflowExecution(ctx, req.GetWorkflowId(), req.GetRunId())
	if err != nil {
		s.logger.Error("Failed to describe workflow",
			logging.F("workflow_id", req.GetWorkflowId()),
			logging.Err(err),
		)

		// Check if it's a not found error
		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "workflow not found: %s", req.GetWorkflowId())
		}
		return nil, status.Errorf(codes.Internal, "failed to get workflow status: %v", err)
	}

	statusDetails := s.describeToStatusDetails(desc)

	return &workflowv1.GetWorkflowStatusResponse{
		Status: statusDetails,
	}, nil
}

// CancelWorkflow cancels a running workflow.
func (s *Service) CancelWorkflow(ctx context.Context, req *workflowv1.CancelWorkflowRequest) (*workflowv1.CancelWorkflowResponse, error) {
	if req.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id is required")
	}

	s.logger.Info("CancelWorkflow called",
		logging.F("workflow_id", req.GetWorkflowId()),
		logging.F("run_id", req.GetRunId()),
		logging.F("reason", req.GetReason()),
	)

	err := s.temporalClient.CancelWorkflow(ctx, req.GetWorkflowId(), req.GetRunId())
	if err != nil {
		s.logger.Error("Failed to cancel workflow",
			logging.F("workflow_id", req.GetWorkflowId()),
			logging.Err(err),
		)

		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "workflow not found: %s", req.GetWorkflowId())
		}
		return nil, status.Errorf(codes.Internal, "failed to cancel workflow: %v", err)
	}

	s.logger.Info("Workflow cancellation requested",
		logging.F("workflow_id", req.GetWorkflowId()),
	)

	return &workflowv1.CancelWorkflowResponse{
		Accepted: true,
		Message:  fmt.Sprintf("Cancellation requested for workflow %s", req.GetWorkflowId()),
	}, nil
}

// TerminateWorkflow forcefully terminates a workflow.
func (s *Service) TerminateWorkflow(ctx context.Context, req *workflowv1.TerminateWorkflowRequest) (*workflowv1.TerminateWorkflowResponse, error) {
	if req.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_id is required")
	}

	s.logger.Info("TerminateWorkflow called",
		logging.F("workflow_id", req.GetWorkflowId()),
		logging.F("run_id", req.GetRunId()),
		logging.F("reason", req.GetReason()),
	)

	reason := req.GetReason()
	if reason == "" {
		reason = "Terminated via API"
	}

	err := s.temporalClient.TerminateWorkflow(ctx, req.GetWorkflowId(), req.GetRunId(), reason)
	if err != nil {
		s.logger.Error("Failed to terminate workflow",
			logging.F("workflow_id", req.GetWorkflowId()),
			logging.Err(err),
		)

		if isNotFoundError(err) {
			return nil, status.Errorf(codes.NotFound, "workflow not found: %s", req.GetWorkflowId())
		}
		return nil, status.Errorf(codes.Internal, "failed to terminate workflow: %v", err)
	}

	s.logger.Info("Workflow terminated",
		logging.F("workflow_id", req.GetWorkflowId()),
		logging.F("reason", reason),
	)

	return &workflowv1.TerminateWorkflowResponse{
		Accepted: true,
		Message:  fmt.Sprintf("Workflow %s terminated: %s", req.GetWorkflowId(), reason),
	}, nil
}

// buildListQuery constructs a Temporal visibility query from the request.
func (s *Service) buildListQuery(req *workflowv1.ListWorkflowsRequest) string {
	var conditions []string

	// Use the raw query if provided
	if req.GetQuery() != "" {
		return req.GetQuery()
	}

	if req.GetWorkflowType() != "" {
		conditions = append(conditions, fmt.Sprintf("WorkflowType = '%s'", req.GetWorkflowType()))
	}

	if req.GetStatus() != workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_UNSPECIFIED {
		temporalStatus := protoStatusToTemporalString(req.GetStatus())
		if temporalStatus != "" {
			conditions = append(conditions, fmt.Sprintf("ExecutionStatus = '%s'", temporalStatus))
		}
	}

	if req.GetStartTimeMin() != nil {
		conditions = append(conditions, fmt.Sprintf("StartTime >= '%s'", req.GetStartTimeMin().AsTime().Format(time.RFC3339)))
	}

	if req.GetStartTimeMax() != nil {
		conditions = append(conditions, fmt.Sprintf("StartTime <= '%s'", req.GetStartTimeMax().AsTime().Format(time.RFC3339)))
	}

	if len(conditions) == 0 {
		return ""
	}

	query := conditions[0]
	for i := 1; i < len(conditions); i++ {
		query += " AND " + conditions[i]
	}

	return query
}

// executionInfoToProto converts a Temporal execution info to proto.
func (s *Service) executionInfoToProto(exec *temporalworkflow.WorkflowExecutionInfo) *workflowv1.WorkflowInfo {
	if exec == nil {
		return nil
	}

	info := &workflowv1.WorkflowInfo{
		WorkflowId:   exec.GetExecution().GetWorkflowId(),
		RunId:        exec.GetExecution().GetRunId(),
		WorkflowType: exec.GetType().GetName(),
		Status:       temporalStatusToProto(exec.GetStatus()),
		TaskQueue:    exec.GetTaskQueue(),
	}

	if exec.GetStartTime() != nil {
		info.StartTime = timestamppb.New(exec.GetStartTime().AsTime())
	}

	if exec.GetCloseTime() != nil {
		info.CloseTime = timestamppb.New(exec.GetCloseTime().AsTime())
	}

	return info
}

// describeToStatusDetails converts a DescribeWorkflowExecution response to status details.
func (s *Service) describeToStatusDetails(desc *workflowservice.DescribeWorkflowExecutionResponse) *workflowv1.WorkflowStatusDetails {
	if desc == nil {
		return nil
	}

	execInfo := desc.GetWorkflowExecutionInfo()
	info := &workflowv1.WorkflowInfo{
		WorkflowId:   execInfo.GetExecution().GetWorkflowId(),
		RunId:        execInfo.GetExecution().GetRunId(),
		WorkflowType: execInfo.GetType().GetName(),
		Status:       temporalStatusToProto(execInfo.GetStatus()),
		TaskQueue:    execInfo.GetTaskQueue(),
	}

	if execInfo.GetStartTime() != nil {
		info.StartTime = timestamppb.New(execInfo.GetStartTime().AsTime())
	}

	if execInfo.GetCloseTime() != nil {
		info.CloseTime = timestamppb.New(execInfo.GetCloseTime().AsTime())
	}

	details := &workflowv1.WorkflowStatusDetails{
		Info:              info,
		PendingActivities: int32(len(desc.GetPendingActivities())),
		PendingChildren:   int32(len(desc.GetPendingChildren())),
		HistoryLength:     execInfo.GetHistoryLength(),
	}

	// Calculate execution duration if workflow has closed
	if execInfo.GetStartTime() != nil && execInfo.GetCloseTime() != nil {
		duration := execInfo.GetCloseTime().AsTime().Sub(execInfo.GetStartTime().AsTime())
		details.ExecutionDurationMs = duration.Milliseconds()
	}

	// Extract memo if present
	if execInfo.GetMemo() != nil && len(execInfo.GetMemo().GetFields()) > 0 {
		details.Memo = make(map[string]string)
		for k, v := range execInfo.GetMemo().GetFields() {
			// Convert payload to string representation
			if v != nil && len(v.GetData()) > 0 {
				details.Memo[k] = string(v.GetData())
			}
		}
	}

	// Extract search attributes if present
	if execInfo.GetSearchAttributes() != nil && len(execInfo.GetSearchAttributes().GetIndexedFields()) > 0 {
		details.SearchAttributes = make(map[string]string)
		for k, v := range execInfo.GetSearchAttributes().GetIndexedFields() {
			if v != nil && len(v.GetData()) > 0 {
				details.SearchAttributes[k] = string(v.GetData())
			}
		}
	}

	return details
}

// temporalStatusToProto converts Temporal workflow execution status to proto enum.
func temporalStatusToProto(status enums.WorkflowExecutionStatus) workflowv1.WorkflowExecutionStatus {
	switch status {
	case enums.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_RUNNING
	case enums.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED
	case enums.WORKFLOW_EXECUTION_STATUS_FAILED:
		return workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED
	case enums.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_CANCELED
	case enums.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_TERMINATED
	case enums.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW
	case enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_TIMED_OUT
	default:
		return workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_UNSPECIFIED
	}
}

// protoStatusToTemporalString converts proto status enum to Temporal query string.
func protoStatusToTemporalString(status workflowv1.WorkflowExecutionStatus) string {
	switch status {
	case workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_RUNNING:
		return "Running"
	case workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return "Completed"
	case workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_FAILED:
		return "Failed"
	case workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_CANCELED:
		return "Canceled"
	case workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return "Terminated"
	case workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return "ContinuedAsNew"
	case workflowv1.WorkflowExecutionStatus_WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return "TimedOut"
	default:
		return ""
	}
}

// isNotFoundError checks if the error indicates a workflow was not found.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return contains(errMsg, "not found") || contains(errMsg, "NotFound")
}

// contains checks if s contains substr (case-sensitive).
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
