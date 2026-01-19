package workflows

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.temporal.io/sdk/client"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/temporal"
)

// Workflow type constants.
const (
	WorkflowTypeIngestion = "ingestion"
	WorkflowTypeSync      = "sync"
	WorkflowTypeReview    = "review"
	WorkflowTypeAnalysis  = "analysis"
)

// Workflow function names registered with Temporal.
const (
	IngestionWorkflowName = "IngestionWorkflow"
	SyncWorkflowName      = "SyncWorkflow"
	ReviewWorkflowName    = "ReviewWorkflow"
	AnalysisWorkflowName  = "AnalysisWorkflow"
)

// IngestRequest represents a request to start an ingestion workflow.
type IngestRequest struct {
	TenantID   string            `json:"tenant_id"`
	SourceType string            `json:"source_type"`
	SourceID   string            `json:"source_id"`
	ContentURL string            `json:"content_url,omitempty"`
	MimeType   string            `json:"mime_type,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// SyncRequest represents a request to start a sync workflow.
type SyncRequest struct {
	TenantID   string   `json:"tenant_id"`
	SourceType string   `json:"source_type"`
	SourceIDs  []string `json:"source_ids,omitempty"`
	FullSync   bool     `json:"full_sync"`
	Since      string   `json:"since,omitempty"` // RFC3339 timestamp
}

// ReviewRequest represents a request to start a review workflow.
type ReviewRequest struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	Date     string `json:"date"` // Format: YYYY-MM-DD
	Scope    string `json:"scope,omitempty"`
}

// AnalysisRequest represents a request to start an analysis workflow.
type AnalysisRequest struct {
	TenantID     string   `json:"tenant_id"`
	AnalysisType string   `json:"analysis_type"`
	TargetIDs    []string `json:"target_ids"`
	Options      map[string]string `json:"options,omitempty"`
}

// WorkflowStartResult represents the result of starting a workflow.
type WorkflowStartResult struct {
	WorkflowID string `json:"workflow_id"`
	RunID      string `json:"run_id"`
}

// Starter provides methods for starting Temporal workflows from the Gateway.
type Starter struct {
	temporalClient client.Client
	config         *Config
	logger         logging.Logger
	metrics        *StarterMetrics
}

// StarterMetrics holds Prometheus metrics for the workflow starter.
type StarterMetrics struct {
	workflowsStarted   *prometheus.CounterVec
	workflowStartErrors *prometheus.CounterVec
	workflowStartLatency *prometheus.HistogramVec
}

// NewStarter creates a new workflow Starter.
func NewStarter(temporalClient client.Client, cfg *Config, logger logging.Logger) *Starter {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = logging.MustGlobal()
	}

	s := &Starter{
		temporalClient: temporalClient,
		config:         cfg,
		logger:         logger,
	}

	if cfg.MetricsEnabled {
		s.metrics = newStarterMetrics(cfg.MetricsPrefix)
	}

	return s
}

// newStarterMetrics creates new Prometheus metrics for the starter.
func newStarterMetrics(prefix string) *StarterMetrics {
	return &StarterMetrics{
		workflowsStarted: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "_started_total",
				Help: "Total number of workflows started by type and status",
			},
			[]string{"workflow_type", "status"},
		),
		workflowStartErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "_start_errors_total",
				Help: "Total number of workflow start errors by type and error_type",
			},
			[]string{"workflow_type", "error_type"},
		),
		workflowStartLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    prefix + "_start_latency_seconds",
				Help:    "Latency of workflow start operations in seconds",
				Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"workflow_type"},
		),
	}
}

// RegisterMetrics registers the Prometheus metrics with the default registry.
func (s *Starter) RegisterMetrics() error {
	if s.metrics == nil {
		return nil
	}

	collectors := []prometheus.Collector{
		s.metrics.workflowsStarted,
		s.metrics.workflowStartErrors,
		s.metrics.workflowStartLatency,
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
func (s *Starter) RegisterMetricsWithRegistry(reg *prometheus.Registry) error {
	if s.metrics == nil {
		return nil
	}

	collectors := []prometheus.Collector{
		s.metrics.workflowsStarted,
		s.metrics.workflowStartErrors,
		s.metrics.workflowStartLatency,
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

// StartIngestionWorkflow starts a content ingestion workflow.
func (s *Starter) StartIngestionWorkflow(ctx context.Context, req IngestRequest) (string, error) {
	if req.TenantID == "" {
		return "", fmt.Errorf("tenant_id is required")
	}
	if req.SourceType == "" {
		return "", fmt.Errorf("source_type is required")
	}
	if req.SourceID == "" {
		return "", fmt.Errorf("source_id is required")
	}

	workflowID := temporal.GenerateIngestWorkflowID(req.TenantID, req.SourceType, req.SourceID)
	taskQueue := s.config.TaskQueueForWorkflowType(WorkflowTypeIngestion)

	return s.startWorkflow(ctx, WorkflowTypeIngestion, workflowID, taskQueue, IngestionWorkflowName, req)
}

// StartSyncWorkflow starts a data sync workflow.
func (s *Starter) StartSyncWorkflow(ctx context.Context, req SyncRequest) (string, error) {
	if req.TenantID == "" {
		return "", fmt.Errorf("tenant_id is required")
	}
	if req.SourceType == "" {
		return "", fmt.Errorf("source_type is required")
	}

	workflowID := temporal.GenerateTimestampedWorkflowID(fmt.Sprintf("sync-%s-%s", req.TenantID, req.SourceType))
	taskQueue := s.config.TaskQueueForWorkflowType(WorkflowTypeSync)

	return s.startWorkflow(ctx, WorkflowTypeSync, workflowID, taskQueue, SyncWorkflowName, req)
}

// StartReviewWorkflow starts a daily review workflow.
func (s *Starter) StartReviewWorkflow(ctx context.Context, req ReviewRequest) (string, error) {
	if req.TenantID == "" {
		return "", fmt.Errorf("tenant_id is required")
	}
	if req.UserID == "" {
		return "", fmt.Errorf("user_id is required")
	}
	if req.Date == "" {
		return "", fmt.Errorf("date is required")
	}

	workflowID := temporal.GenerateDeterministicWorkflowID("review", req.TenantID, fmt.Sprintf("%s-%s", req.UserID, req.Date))
	taskQueue := s.config.TaskQueueForWorkflowType(WorkflowTypeReview)

	return s.startWorkflow(ctx, WorkflowTypeReview, workflowID, taskQueue, ReviewWorkflowName, req)
}

// StartAnalysisWorkflow starts an analysis workflow.
func (s *Starter) StartAnalysisWorkflow(ctx context.Context, req AnalysisRequest) (string, error) {
	if req.TenantID == "" {
		return "", fmt.Errorf("tenant_id is required")
	}
	if req.AnalysisType == "" {
		return "", fmt.Errorf("analysis_type is required")
	}
	if len(req.TargetIDs) == 0 {
		return "", fmt.Errorf("target_ids is required")
	}

	workflowID := temporal.GenerateTimestampedWorkflowID(fmt.Sprintf("analysis-%s-%s", req.TenantID, req.AnalysisType))
	taskQueue := s.config.TaskQueueForWorkflowType(WorkflowTypeAnalysis)

	return s.startWorkflow(ctx, WorkflowTypeAnalysis, workflowID, taskQueue, AnalysisWorkflowName, req)
}

// startWorkflow is the internal method that starts a workflow with metrics and logging.
func (s *Starter) startWorkflow(
	ctx context.Context,
	workflowType string,
	workflowID string,
	taskQueue string,
	workflowName string,
	input interface{},
) (string, error) {
	startTime := time.Now()

	s.logger.Info("Starting workflow",
		logging.F("workflow_type", workflowType),
		logging.F("workflow_id", workflowID),
		logging.F("task_queue", taskQueue),
	)

	options := client.StartWorkflowOptions{
		ID:                       workflowID,
		TaskQueue:                taskQueue,
		WorkflowExecutionTimeout: s.config.DefaultExecutionTimeout,
		WorkflowTaskTimeout:      s.config.DefaultTaskTimeout,
	}

	// Create a context with timeout for the start operation
	startCtx, cancel := context.WithTimeout(ctx, s.config.DefaultStartTimeout)
	defer cancel()

	run, err := s.temporalClient.ExecuteWorkflow(startCtx, options, workflowName, input)
	if err != nil {
		s.recordError(workflowType, err)
		s.logger.Error("Failed to start workflow",
			logging.F("workflow_type", workflowType),
			logging.F("workflow_id", workflowID),
			logging.Err(err),
		)
		return "", fmt.Errorf("failed to start %s workflow: %w", workflowType, err)
	}

	s.recordSuccess(workflowType, startTime)
	s.logger.Info("Workflow started successfully",
		logging.F("workflow_type", workflowType),
		logging.F("workflow_id", run.GetID()),
		logging.F("run_id", run.GetRunID()),
	)

	return run.GetID(), nil
}

// recordSuccess records successful workflow start metrics.
func (s *Starter) recordSuccess(workflowType string, startTime time.Time) {
	if s.metrics == nil {
		return
	}

	s.metrics.workflowsStarted.WithLabelValues(workflowType, "success").Inc()
	s.metrics.workflowStartLatency.WithLabelValues(workflowType).Observe(time.Since(startTime).Seconds())
}

// recordError records workflow start error metrics.
func (s *Starter) recordError(workflowType string, err error) {
	if s.metrics == nil {
		return
	}

	errorType := classifyError(err)
	s.metrics.workflowsStarted.WithLabelValues(workflowType, "error").Inc()
	s.metrics.workflowStartErrors.WithLabelValues(workflowType, errorType).Inc()
}

// classifyError classifies an error for metrics purposes.
func classifyError(err error) string {
	if err == nil {
		return "none"
	}

	errMsg := err.Error()
	switch {
	case contains(errMsg, "timeout"):
		return "timeout"
	case contains(errMsg, "connection"):
		return "connection"
	case contains(errMsg, "already started"):
		return "duplicate"
	case contains(errMsg, "not found"):
		return "not_found"
	case contains(errMsg, "invalid"):
		return "invalid_input"
	default:
		return "unknown"
	}
}

// contains checks if s contains substr (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsIgnoreCase(s, substr))
}

// containsIgnoreCase is a simple case-insensitive contains check.
func containsIgnoreCase(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

// equalFold is a simple case-insensitive string equality check.
func equalFold(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if lower(s[i]) != lower(t[i]) {
			return false
		}
	}
	return true
}

// lower converts a byte to lowercase.
func lower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}

// Config returns the starter's configuration.
func (s *Starter) Config() *Config {
	return s.config
}

// Client returns the underlying Temporal client.
func (s *Starter) Client() client.Client {
	return s.temporalClient
}
