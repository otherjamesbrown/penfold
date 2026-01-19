package workflows

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// WorkflowStatus represents the status of a workflow execution.
type WorkflowStatus struct {
	WorkflowID        string    `json:"workflow_id"`
	RunID             string    `json:"run_id"`
	WorkflowType      string    `json:"workflow_type"`
	Status            string    `json:"status"`
	TaskQueue         string    `json:"task_queue"`
	StartTime         time.Time `json:"start_time,omitempty"`
	CloseTime         time.Time `json:"close_time,omitempty"`
	PendingActivities int       `json:"pending_activities"`
	HistoryLength     int64     `json:"history_length"`
}

// WorkflowInfo represents summarized workflow information for listing.
type WorkflowInfo struct {
	WorkflowID   string    `json:"workflow_id"`
	RunID        string    `json:"run_id"`
	WorkflowType string    `json:"workflow_type"`
	Status       string    `json:"status"`
	StartTime    time.Time `json:"start_time"`
}

// WorkflowFilter provides filtering options for listing workflows.
type WorkflowFilter struct {
	WorkflowType string    `json:"workflow_type,omitempty"`
	Status       string    `json:"status,omitempty"`
	StartTimeMin time.Time `json:"start_time_min,omitempty"`
	StartTimeMax time.Time `json:"start_time_max,omitempty"`
	PageSize     int       `json:"page_size,omitempty"`
	NextPageToken []byte   `json:"next_page_token,omitempty"`
}

// ListWorkflowsResult contains the results of listing workflows.
type ListWorkflowsResult struct {
	Workflows     []WorkflowInfo `json:"workflows"`
	NextPageToken []byte         `json:"next_page_token,omitempty"`
}

// StatusService provides methods for querying workflow status.
type StatusService struct {
	temporalClient client.Client
	config         *Config
	logger         logging.Logger
	metrics        *StatusMetrics
}

// StatusMetrics holds Prometheus metrics for the status service.
type StatusMetrics struct {
	statusQueries     *prometheus.CounterVec
	statusQueryErrors *prometheus.CounterVec
	statusQueryLatency *prometheus.HistogramVec
	listQueries       *prometheus.CounterVec
	cancelRequests    *prometheus.CounterVec
}

// NewStatusService creates a new StatusService.
func NewStatusService(temporalClient client.Client, cfg *Config, logger logging.Logger) *StatusService {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = logging.MustGlobal()
	}

	s := &StatusService{
		temporalClient: temporalClient,
		config:         cfg,
		logger:         logger,
	}

	if cfg.MetricsEnabled {
		s.metrics = newStatusMetrics(cfg.MetricsPrefix)
	}

	return s
}

// newStatusMetrics creates new Prometheus metrics for the status service.
func newStatusMetrics(prefix string) *StatusMetrics {
	return &StatusMetrics{
		statusQueries: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "_status_queries_total",
				Help: "Total number of workflow status queries",
			},
			[]string{"status"},
		),
		statusQueryErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "_status_query_errors_total",
				Help: "Total number of workflow status query errors",
			},
			[]string{"error_type"},
		),
		statusQueryLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    prefix + "_status_query_latency_seconds",
				Help:    "Latency of workflow status queries in seconds",
				Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
			},
			[]string{"operation"},
		),
		listQueries: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "_list_queries_total",
				Help: "Total number of workflow list queries",
			},
			[]string{"status"},
		),
		cancelRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "_cancel_requests_total",
				Help: "Total number of workflow cancel requests",
			},
			[]string{"status"},
		),
	}
}

// RegisterMetrics registers the Prometheus metrics with the default registry.
func (s *StatusService) RegisterMetrics() error {
	if s.metrics == nil {
		return nil
	}

	collectors := []prometheus.Collector{
		s.metrics.statusQueries,
		s.metrics.statusQueryErrors,
		s.metrics.statusQueryLatency,
		s.metrics.listQueries,
		s.metrics.cancelRequests,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return fmt.Errorf("failed to register metric: %w", err)
			}
		}
	}

	return nil
}

// RegisterMetricsWithRegistry registers the Prometheus metrics with a custom registry.
func (s *StatusService) RegisterMetricsWithRegistry(reg *prometheus.Registry) error {
	if s.metrics == nil {
		return nil
	}

	collectors := []prometheus.Collector{
		s.metrics.statusQueries,
		s.metrics.statusQueryErrors,
		s.metrics.statusQueryLatency,
		s.metrics.listQueries,
		s.metrics.cancelRequests,
	}

	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return fmt.Errorf("failed to register metric: %w", err)
			}
		}
	}

	return nil
}

// GetWorkflowStatus retrieves the status of a specific workflow.
func (s *StatusService) GetWorkflowStatus(ctx context.Context, workflowID string) (*WorkflowStatus, error) {
	if workflowID == "" {
		return nil, fmt.Errorf("workflow_id is required")
	}

	startTime := time.Now()

	s.logger.Debug("Getting workflow status",
		logging.F("workflow_id", workflowID),
	)

	desc, err := s.temporalClient.DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil {
		s.recordStatusError("describe_error")
		s.logger.Error("Failed to describe workflow",
			logging.F("workflow_id", workflowID),
			logging.Err(err),
		)
		return nil, fmt.Errorf("failed to get workflow status: %w", err)
	}

	status := &WorkflowStatus{
		WorkflowID:        workflowID,
		RunID:             desc.WorkflowExecutionInfo.Execution.RunId,
		WorkflowType:      desc.WorkflowExecutionInfo.Type.Name,
		Status:            mapWorkflowStatus(desc.WorkflowExecutionInfo.Status),
		TaskQueue:         desc.WorkflowExecutionInfo.TaskQueue,
		PendingActivities: len(desc.PendingActivities),
		HistoryLength:     desc.WorkflowExecutionInfo.HistoryLength,
	}

	if desc.WorkflowExecutionInfo.StartTime != nil {
		status.StartTime = desc.WorkflowExecutionInfo.StartTime.AsTime()
	}

	if desc.WorkflowExecutionInfo.CloseTime != nil {
		status.CloseTime = desc.WorkflowExecutionInfo.CloseTime.AsTime()
	}

	s.recordStatusSuccess(status.Status, startTime)

	return status, nil
}

// ListWorkflows lists workflows matching the given filter criteria.
func (s *StatusService) ListWorkflows(ctx context.Context, filter WorkflowFilter) (*ListWorkflowsResult, error) {
	startTime := time.Now()

	// Build the query string
	query := buildWorkflowQuery(filter)

	pageSize := filter.PageSize
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 100 // Default page size
	}

	s.logger.Debug("Listing workflows",
		logging.F("query", query),
		logging.F("page_size", pageSize),
	)

	req := &workflowservice.ListWorkflowExecutionsRequest{
		Namespace:     s.config.TemporalNamespace,
		Query:         query,
		PageSize:      int32(pageSize),
		NextPageToken: filter.NextPageToken,
	}

	resp, err := s.temporalClient.ListWorkflow(ctx, req)
	if err != nil {
		s.recordListError("list_error")
		s.logger.Error("Failed to list workflows",
			logging.F("query", query),
			logging.Err(err),
		)
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	workflows := make([]WorkflowInfo, 0, len(resp.Executions))
	for _, exec := range resp.Executions {
		info := WorkflowInfo{
			WorkflowID:   exec.Execution.WorkflowId,
			RunID:        exec.Execution.RunId,
			WorkflowType: exec.Type.Name,
			Status:       mapWorkflowStatus(exec.Status),
		}

		if exec.StartTime != nil {
			info.StartTime = exec.StartTime.AsTime()
		}

		workflows = append(workflows, info)
	}

	s.recordListSuccess(startTime)

	return &ListWorkflowsResult{
		Workflows:     workflows,
		NextPageToken: resp.NextPageToken,
	}, nil
}

// CancelWorkflow cancels a running workflow.
func (s *StatusService) CancelWorkflow(ctx context.Context, workflowID string) error {
	if workflowID == "" {
		return fmt.Errorf("workflow_id is required")
	}

	s.logger.Info("Canceling workflow",
		logging.F("workflow_id", workflowID),
	)

	err := s.temporalClient.CancelWorkflow(ctx, workflowID, "")
	if err != nil {
		s.recordCancelError()
		s.logger.Error("Failed to cancel workflow",
			logging.F("workflow_id", workflowID),
			logging.Err(err),
		)
		return fmt.Errorf("failed to cancel workflow: %w", err)
	}

	s.recordCancelSuccess()
	s.logger.Info("Workflow canceled successfully",
		logging.F("workflow_id", workflowID),
	)

	return nil
}

// buildWorkflowQuery builds a Temporal visibility query from the filter.
func buildWorkflowQuery(filter WorkflowFilter) string {
	var conditions []string

	if filter.WorkflowType != "" {
		conditions = append(conditions, fmt.Sprintf("WorkflowType = '%s'", filter.WorkflowType))
	}

	if filter.Status != "" {
		statusEnum := mapStatusToEnum(filter.Status)
		if statusEnum != "" {
			conditions = append(conditions, fmt.Sprintf("ExecutionStatus = '%s'", statusEnum))
		}
	}

	if !filter.StartTimeMin.IsZero() {
		conditions = append(conditions, fmt.Sprintf("StartTime >= '%s'", filter.StartTimeMin.Format(time.RFC3339)))
	}

	if !filter.StartTimeMax.IsZero() {
		conditions = append(conditions, fmt.Sprintf("StartTime <= '%s'", filter.StartTimeMax.Format(time.RFC3339)))
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

// mapStatusToEnum maps a status string to Temporal's execution status enum string.
func mapStatusToEnum(status string) string {
	switch status {
	case "Running":
		return "Running"
	case "Completed":
		return "Completed"
	case "Failed":
		return "Failed"
	case "Canceled":
		return "Canceled"
	case "Terminated":
		return "Terminated"
	case "TimedOut":
		return "TimedOut"
	case "ContinuedAsNew":
		return "ContinuedAsNew"
	default:
		return ""
	}
}

// mapWorkflowStatus maps Temporal workflow execution status to a string.
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

// Metrics recording helpers

func (s *StatusService) recordStatusSuccess(status string, startTime time.Time) {
	if s.metrics == nil {
		return
	}
	s.metrics.statusQueries.WithLabelValues("success").Inc()
	s.metrics.statusQueryLatency.WithLabelValues("get_status").Observe(time.Since(startTime).Seconds())
}

func (s *StatusService) recordStatusError(errorType string) {
	if s.metrics == nil {
		return
	}
	s.metrics.statusQueries.WithLabelValues("error").Inc()
	s.metrics.statusQueryErrors.WithLabelValues(errorType).Inc()
}

func (s *StatusService) recordListSuccess(startTime time.Time) {
	if s.metrics == nil {
		return
	}
	s.metrics.listQueries.WithLabelValues("success").Inc()
	s.metrics.statusQueryLatency.WithLabelValues("list").Observe(time.Since(startTime).Seconds())
}

func (s *StatusService) recordListError(errorType string) {
	if s.metrics == nil {
		return
	}
	s.metrics.listQueries.WithLabelValues("error").Inc()
	s.metrics.statusQueryErrors.WithLabelValues(errorType).Inc()
}

func (s *StatusService) recordCancelSuccess() {
	if s.metrics == nil {
		return
	}
	s.metrics.cancelRequests.WithLabelValues("success").Inc()
}

func (s *StatusService) recordCancelError() {
	if s.metrics == nil {
		return
	}
	s.metrics.cancelRequests.WithLabelValues("error").Inc()
}
