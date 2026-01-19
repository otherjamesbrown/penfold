package workflows

import (
	"fmt"

	gatewayv1 "github.com/otherjamesbrown/penfold/api/proto/gateway/v1"
	commonv1 "github.com/otherjamesbrown/penfold/api/proto/common/v1"
)

// MapProcessEmailToIngestRequest maps a ProcessEmailRequest to an IngestRequest.
func MapProcessEmailToIngestRequest(req *gatewayv1.ProcessEmailRequest) (*IngestRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	if req.TenantId == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if req.MessageId == "" {
		return nil, fmt.Errorf("message_id is required")
	}

	metadata := make(map[string]string)
	for k, v := range req.Metadata {
		metadata[k] = v
	}

	// Add email-specific metadata
	metadata["from_email"] = req.FromEmail
	if req.FromName != nil {
		metadata["from_name"] = *req.FromName
	}
	if req.Subject != nil {
		metadata["subject"] = *req.Subject
	}
	if req.ThreadId != "" {
		metadata["thread_id"] = req.ThreadId
	}

	return &IngestRequest{
		TenantID:   req.TenantId,
		SourceType: "email",
		SourceID:   req.MessageId,
		Metadata:   metadata,
	}, nil
}

// MapIngestResultToProcessEmailResponse maps a workflow result to ProcessEmailResponse.
func MapIngestResultToProcessEmailResponse(workflowID string, sourceID int64, err error) *gatewayv1.ProcessEmailResponse {
	resp := &gatewayv1.ProcessEmailResponse{
		WorkflowId: workflowID,
		SourceId:   sourceID,
	}

	if err != nil {
		resp.Status = commonv1.Status_STATUS_FAILED
		resp.Message = err.Error()
	} else {
		resp.Status = commonv1.Status_STATUS_PENDING
		resp.Message = "Workflow started successfully"
	}

	return resp
}

// WorkflowAPIRequest is a generic interface for workflow API requests.
type WorkflowAPIRequest interface {
	GetTenantId() string
}

// WorkflowAPIResponse is a generic interface for workflow API responses.
type WorkflowAPIResponse interface {
	GetWorkflowId() string
}

// MapWorkflowStatusToAPIStatus maps internal workflow status to common API status.
func MapWorkflowStatusToAPIStatus(status string) commonv1.Status {
	switch status {
	case "Running":
		return commonv1.Status_STATUS_IN_PROGRESS
	case "Completed":
		return commonv1.Status_STATUS_COMPLETED
	case "Failed":
		return commonv1.Status_STATUS_FAILED
	case "Canceled":
		return commonv1.Status_STATUS_CANCELLED
	case "Terminated":
		return commonv1.Status_STATUS_FAILED
	case "TimedOut":
		return commonv1.Status_STATUS_FAILED
	default:
		return commonv1.Status_STATUS_UNKNOWN
	}
}

// MapAPIStatusToString maps common API status to a human-readable string.
func MapAPIStatusToString(status commonv1.Status) string {
	switch status {
	case commonv1.Status_STATUS_UNKNOWN:
		return "unknown"
	case commonv1.Status_STATUS_PENDING:
		return "pending"
	case commonv1.Status_STATUS_IN_PROGRESS:
		return "in_progress"
	case commonv1.Status_STATUS_COMPLETED:
		return "completed"
	case commonv1.Status_STATUS_FAILED:
		return "failed"
	case commonv1.Status_STATUS_CANCELLED:
		return "cancelled"
	default:
		return "unknown"
	}
}

// IngestRequestOptions provides additional options for ingestion requests.
type IngestRequestOptions struct {
	Priority    int               `json:"priority,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	CallbackURL string            `json:"callback_url,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
}

// SyncRequestOptions provides additional options for sync requests.
type SyncRequestOptions struct {
	Force       bool              `json:"force,omitempty"`
	DryRun      bool              `json:"dry_run,omitempty"`
	BatchSize   int               `json:"batch_size,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
}

// ReviewRequestOptions provides additional options for review requests.
type ReviewRequestOptions struct {
	IncludeArchived bool              `json:"include_archived,omitempty"`
	MaxItems        int               `json:"max_items,omitempty"`
	Categories      []string          `json:"categories,omitempty"`
	Options         map[string]string `json:"options,omitempty"`
}

// AnalysisRequestOptions provides additional options for analysis requests.
type AnalysisRequestOptions struct {
	Depth       int               `json:"depth,omitempty"`
	Confidence  float64           `json:"confidence,omitempty"`
	IncludeRaw  bool              `json:"include_raw,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
}

// WorkflowResult represents a generic workflow result.
type WorkflowResult struct {
	WorkflowID string                 `json:"workflow_id"`
	RunID      string                 `json:"run_id"`
	Status     string                 `json:"status"`
	Output     map[string]interface{} `json:"output,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

// MapWorkflowResultToAPIResponse creates a generic API response from a workflow result.
func MapWorkflowResultToAPIResponse(result *WorkflowResult) map[string]interface{} {
	resp := map[string]interface{}{
		"workflow_id": result.WorkflowID,
		"run_id":      result.RunID,
		"status":      result.Status,
	}

	if result.Output != nil {
		resp["output"] = result.Output
	}

	if result.Error != "" {
		resp["error"] = result.Error
	}

	return resp
}

// RetryConfig provides retry configuration for workflow operations.
type RetryConfig struct {
	MaxAttempts     int     `json:"max_attempts"`
	BackoffFactor   float64 `json:"backoff_factor"`
	InitialInterval int     `json:"initial_interval_ms"`
	MaxInterval     int     `json:"max_interval_ms"`
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:     3,
		BackoffFactor:   2.0,
		InitialInterval: 1000,  // 1 second
		MaxInterval:     60000, // 60 seconds
	}
}

// ErrorResponse represents a structured error response.
type ErrorResponse struct {
	Code       string            `json:"code"`
	Message    string            `json:"message"`
	Details    map[string]string `json:"details,omitempty"`
	Retryable  bool              `json:"retryable"`
	RetryAfter int               `json:"retry_after_ms,omitempty"`
}

// NewErrorResponse creates a new ErrorResponse.
func NewErrorResponse(code, message string, retryable bool) *ErrorResponse {
	return &ErrorResponse{
		Code:      code,
		Message:   message,
		Retryable: retryable,
	}
}

// WithDetails adds details to the error response.
func (e *ErrorResponse) WithDetails(details map[string]string) *ErrorResponse {
	e.Details = details
	return e
}

// WithRetryAfter sets the retry-after duration.
func (e *ErrorResponse) WithRetryAfter(ms int) *ErrorResponse {
	e.RetryAfter = ms
	return e
}

// Error implements the error interface.
func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Common error codes for workflow operations.
const (
	ErrCodeInvalidInput     = "INVALID_INPUT"
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodeAlreadyExists    = "ALREADY_EXISTS"
	ErrCodeTimeout          = "TIMEOUT"
	ErrCodeInternal         = "INTERNAL_ERROR"
	ErrCodeUnavailable      = "UNAVAILABLE"
	ErrCodePermissionDenied = "PERMISSION_DENIED"
	ErrCodeRateLimited      = "RATE_LIMITED"
)

// MapErrorToResponse maps an error to an ErrorResponse.
func MapErrorToResponse(err error) *ErrorResponse {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	switch {
	case contains(errMsg, "not found"):
		return NewErrorResponse(ErrCodeNotFound, errMsg, false)
	case contains(errMsg, "already exists") || contains(errMsg, "already started"):
		return NewErrorResponse(ErrCodeAlreadyExists, errMsg, false)
	case contains(errMsg, "timeout"):
		return NewErrorResponse(ErrCodeTimeout, errMsg, true).WithRetryAfter(5000)
	case contains(errMsg, "invalid") || contains(errMsg, "required"):
		return NewErrorResponse(ErrCodeInvalidInput, errMsg, false)
	case contains(errMsg, "unavailable") || contains(errMsg, "connection"):
		return NewErrorResponse(ErrCodeUnavailable, errMsg, true).WithRetryAfter(10000)
	case contains(errMsg, "permission") || contains(errMsg, "denied"):
		return NewErrorResponse(ErrCodePermissionDenied, errMsg, false)
	case contains(errMsg, "rate limit"):
		return NewErrorResponse(ErrCodeRateLimited, errMsg, true).WithRetryAfter(60000)
	default:
		return NewErrorResponse(ErrCodeInternal, errMsg, false)
	}
}
