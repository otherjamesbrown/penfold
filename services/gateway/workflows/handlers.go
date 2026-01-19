package workflows

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gatewayv1 "github.com/otherjamesbrown/penfold/api/proto/gateway/v1"
	commonv1 "github.com/otherjamesbrown/penfold/api/proto/common/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Handlers provides gRPC handler implementations that bridge API requests to workflows.
type Handlers struct {
	starter       *Starter
	statusService *StatusService
	logger        logging.Logger
	metrics       *HandlerMetrics
}

// HandlerMetrics holds Prometheus metrics for the gRPC handlers.
type HandlerMetrics struct {
	requestsTotal   *prometheus.CounterVec
	requestLatency  *prometheus.HistogramVec
	requestErrors   *prometheus.CounterVec
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(starter *Starter, statusService *StatusService, logger logging.Logger) *Handlers {
	if logger == nil {
		logger = logging.MustGlobal()
	}

	h := &Handlers{
		starter:       starter,
		statusService: statusService,
		logger:        logger,
	}

	// Initialize metrics if starter has metrics enabled
	if starter != nil && starter.config != nil && starter.config.MetricsEnabled {
		h.metrics = newHandlerMetrics(starter.config.MetricsPrefix)
	}

	return h
}

// newHandlerMetrics creates new Prometheus metrics for the handlers.
func newHandlerMetrics(prefix string) *HandlerMetrics {
	return &HandlerMetrics{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "_handler_requests_total",
				Help: "Total number of handler requests by method and status",
			},
			[]string{"method", "status"},
		),
		requestLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    prefix + "_handler_latency_seconds",
				Help:    "Latency of handler requests in seconds",
				Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method"},
		),
		requestErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "_handler_errors_total",
				Help: "Total number of handler errors by method and error_code",
			},
			[]string{"method", "error_code"},
		),
	}
}

// RegisterMetrics registers the Prometheus metrics with the default registry.
func (h *Handlers) RegisterMetrics() error {
	if h.metrics == nil {
		return nil
	}

	collectors := []prometheus.Collector{
		h.metrics.requestsTotal,
		h.metrics.requestLatency,
		h.metrics.requestErrors,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// RegisterMetricsWithRegistry registers the Prometheus metrics with a custom registry.
func (h *Handlers) RegisterMetricsWithRegistry(reg *prometheus.Registry) error {
	if h.metrics == nil {
		return nil
	}

	collectors := []prometheus.Collector{
		h.metrics.requestsTotal,
		h.metrics.requestLatency,
		h.metrics.requestErrors,
	}

	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// ProcessEmail handles the ProcessEmail gRPC request by starting an ingestion workflow.
func (h *Handlers) ProcessEmail(ctx context.Context, req *gatewayv1.ProcessEmailRequest) (*gatewayv1.ProcessEmailResponse, error) {
	startTime := time.Now()
	method := "ProcessEmail"

	h.logger.Info("Processing email request",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("message_id", req.GetMessageId()),
	)

	// Map the request to workflow input
	ingestReq, err := MapProcessEmailToIngestRequest(req)
	if err != nil {
		h.recordError(method, ErrCodeInvalidInput)
		h.logger.Error("Invalid email request",
			logging.F("tenant_id", req.GetTenantId()),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Start the ingestion workflow
	workflowID, err := h.starter.StartIngestionWorkflow(ctx, *ingestReq)
	if err != nil {
		errResp := MapErrorToResponse(err)
		h.recordError(method, errResp.Code)
		h.logger.Error("Failed to start ingestion workflow",
			logging.F("tenant_id", req.GetTenantId()),
			logging.F("message_id", req.GetMessageId()),
			logging.Err(err),
		)

		// Map error to appropriate gRPC status
		grpcCode := mapErrorCodeToGRPC(errResp.Code)
		return nil, status.Errorf(grpcCode, "failed to start workflow: %v", err)
	}

	h.recordSuccess(method, startTime)
	h.logger.Info("Email workflow started",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("message_id", req.GetMessageId()),
		logging.F("workflow_id", workflowID),
	)

	return &gatewayv1.ProcessEmailResponse{
		WorkflowId: workflowID,
		Status:     commonv1.Status_STATUS_PENDING,
		Message:    "Workflow started successfully",
	}, nil
}

// StartIngestionWorkflow handles starting an ingestion workflow.
func (h *Handlers) StartIngestionWorkflow(ctx context.Context, req *IngestRequest) (*WorkflowStartResult, error) {
	startTime := time.Now()
	method := "StartIngestionWorkflow"

	if req == nil {
		h.recordError(method, ErrCodeInvalidInput)
		return nil, status.Errorf(codes.InvalidArgument, "request is required")
	}

	workflowID, err := h.starter.StartIngestionWorkflow(ctx, *req)
	if err != nil {
		errResp := MapErrorToResponse(err)
		h.recordError(method, errResp.Code)
		grpcCode := mapErrorCodeToGRPC(errResp.Code)
		return nil, status.Errorf(grpcCode, "failed to start workflow: %v", err)
	}

	h.recordSuccess(method, startTime)

	return &WorkflowStartResult{
		WorkflowID: workflowID,
	}, nil
}

// StartSyncWorkflow handles starting a sync workflow.
func (h *Handlers) StartSyncWorkflow(ctx context.Context, req *SyncRequest) (*WorkflowStartResult, error) {
	startTime := time.Now()
	method := "StartSyncWorkflow"

	if req == nil {
		h.recordError(method, ErrCodeInvalidInput)
		return nil, status.Errorf(codes.InvalidArgument, "request is required")
	}

	workflowID, err := h.starter.StartSyncWorkflow(ctx, *req)
	if err != nil {
		errResp := MapErrorToResponse(err)
		h.recordError(method, errResp.Code)
		grpcCode := mapErrorCodeToGRPC(errResp.Code)
		return nil, status.Errorf(grpcCode, "failed to start workflow: %v", err)
	}

	h.recordSuccess(method, startTime)

	return &WorkflowStartResult{
		WorkflowID: workflowID,
	}, nil
}

// StartReviewWorkflow handles starting a review workflow.
func (h *Handlers) StartReviewWorkflow(ctx context.Context, req *ReviewRequest) (*WorkflowStartResult, error) {
	startTime := time.Now()
	method := "StartReviewWorkflow"

	if req == nil {
		h.recordError(method, ErrCodeInvalidInput)
		return nil, status.Errorf(codes.InvalidArgument, "request is required")
	}

	workflowID, err := h.starter.StartReviewWorkflow(ctx, *req)
	if err != nil {
		errResp := MapErrorToResponse(err)
		h.recordError(method, errResp.Code)
		grpcCode := mapErrorCodeToGRPC(errResp.Code)
		return nil, status.Errorf(grpcCode, "failed to start workflow: %v", err)
	}

	h.recordSuccess(method, startTime)

	return &WorkflowStartResult{
		WorkflowID: workflowID,
	}, nil
}

// StartAnalysisWorkflow handles starting an analysis workflow.
func (h *Handlers) StartAnalysisWorkflow(ctx context.Context, req *AnalysisRequest) (*WorkflowStartResult, error) {
	startTime := time.Now()
	method := "StartAnalysisWorkflow"

	if req == nil {
		h.recordError(method, ErrCodeInvalidInput)
		return nil, status.Errorf(codes.InvalidArgument, "request is required")
	}

	workflowID, err := h.starter.StartAnalysisWorkflow(ctx, *req)
	if err != nil {
		errResp := MapErrorToResponse(err)
		h.recordError(method, errResp.Code)
		grpcCode := mapErrorCodeToGRPC(errResp.Code)
		return nil, status.Errorf(grpcCode, "failed to start workflow: %v", err)
	}

	h.recordSuccess(method, startTime)

	return &WorkflowStartResult{
		WorkflowID: workflowID,
	}, nil
}

// GetWorkflowStatus handles getting workflow status.
func (h *Handlers) GetWorkflowStatus(ctx context.Context, workflowID string) (*WorkflowStatus, error) {
	startTime := time.Now()
	method := "GetWorkflowStatus"

	if workflowID == "" {
		h.recordError(method, ErrCodeInvalidInput)
		return nil, status.Errorf(codes.InvalidArgument, "workflow_id is required")
	}

	workflowStatus, err := h.statusService.GetWorkflowStatus(ctx, workflowID)
	if err != nil {
		errResp := MapErrorToResponse(err)
		h.recordError(method, errResp.Code)
		grpcCode := mapErrorCodeToGRPC(errResp.Code)
		return nil, status.Errorf(grpcCode, "failed to get workflow status: %v", err)
	}

	h.recordSuccess(method, startTime)

	return workflowStatus, nil
}

// ListWorkflows handles listing workflows.
func (h *Handlers) ListWorkflows(ctx context.Context, filter WorkflowFilter) (*ListWorkflowsResult, error) {
	startTime := time.Now()
	method := "ListWorkflows"

	result, err := h.statusService.ListWorkflows(ctx, filter)
	if err != nil {
		errResp := MapErrorToResponse(err)
		h.recordError(method, errResp.Code)
		grpcCode := mapErrorCodeToGRPC(errResp.Code)
		return nil, status.Errorf(grpcCode, "failed to list workflows: %v", err)
	}

	h.recordSuccess(method, startTime)

	return result, nil
}

// CancelWorkflow handles canceling a workflow.
func (h *Handlers) CancelWorkflow(ctx context.Context, workflowID string) error {
	startTime := time.Now()
	method := "CancelWorkflow"

	if workflowID == "" {
		h.recordError(method, ErrCodeInvalidInput)
		return status.Errorf(codes.InvalidArgument, "workflow_id is required")
	}

	err := h.statusService.CancelWorkflow(ctx, workflowID)
	if err != nil {
		errResp := MapErrorToResponse(err)
		h.recordError(method, errResp.Code)
		grpcCode := mapErrorCodeToGRPC(errResp.Code)
		return status.Errorf(grpcCode, "failed to cancel workflow: %v", err)
	}

	h.recordSuccess(method, startTime)

	return nil
}

// mapErrorCodeToGRPC maps internal error codes to gRPC status codes.
func mapErrorCodeToGRPC(code string) codes.Code {
	switch code {
	case ErrCodeInvalidInput:
		return codes.InvalidArgument
	case ErrCodeNotFound:
		return codes.NotFound
	case ErrCodeAlreadyExists:
		return codes.AlreadyExists
	case ErrCodeTimeout:
		return codes.DeadlineExceeded
	case ErrCodeUnavailable:
		return codes.Unavailable
	case ErrCodePermissionDenied:
		return codes.PermissionDenied
	case ErrCodeRateLimited:
		return codes.ResourceExhausted
	default:
		return codes.Internal
	}
}

// Metrics recording helpers

func (h *Handlers) recordSuccess(method string, startTime time.Time) {
	if h.metrics == nil {
		return
	}
	h.metrics.requestsTotal.WithLabelValues(method, "success").Inc()
	h.metrics.requestLatency.WithLabelValues(method).Observe(time.Since(startTime).Seconds())
}

func (h *Handlers) recordError(method string, errorCode string) {
	if h.metrics == nil {
		return
	}
	h.metrics.requestsTotal.WithLabelValues(method, "error").Inc()
	h.metrics.requestErrors.WithLabelValues(method, errorCode).Inc()
}

// Starter returns the workflow starter.
func (h *Handlers) Starter() *Starter {
	return h.starter
}

// StatusService returns the status service.
func (h *Handlers) StatusService() *StatusService {
	return h.statusService
}
