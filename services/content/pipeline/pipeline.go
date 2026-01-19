// Package pipeline provides a content processing pipeline with configurable stages.
// It processes content through preprocess -> chunk -> analyze -> score -> store stages.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/contentv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Common errors for the pipeline.
var (
	ErrPipelineNotRunning = errors.New("pipeline not running")
	ErrStageNotFound      = errors.New("stage not found")
	ErrEmptyContent       = errors.New("content is empty")
	ErrStageFailed        = errors.New("stage processing failed")
	ErrRollbackFailed     = errors.New("rollback failed")
	ErrDryRunMode         = errors.New("dry run mode - changes not persisted")
)

// StageType identifies the type of processing stage.
type StageType string

const (
	StageTypePreprocess StageType = "preprocess"
	StageTypeChunk      StageType = "chunk"
	StageTypeAnalyze    StageType = "analyze"
	StageTypeScore      StageType = "score"
	StageTypeStore      StageType = "store"
)

// Stage represents a single processing stage in the pipeline.
// Implementations must be safe for concurrent use.
type Stage interface {
	// Name returns the unique name of this stage.
	Name() string

	// Type returns the stage type for ordering and configuration.
	Type() StageType

	// Process executes the stage on the given content.
	// Returns the modified content and any error encountered.
	Process(ctx context.Context, content *ProcessedContent) (*ProcessedContent, error)

	// Rollback reverts changes made by this stage.
	// Called when a subsequent stage fails and cleanup is needed.
	Rollback(ctx context.Context, content *ProcessedContent) error

	// SupportsParallel returns true if this stage can run in parallel with other stages.
	SupportsParallel() bool
}

// ProcessedContent represents content at any stage of processing.
// It accumulates data as it moves through the pipeline.
type ProcessedContent struct {
	// Original content item from the source
	Item *contentv1.ContentItem

	// Normalized text after preprocessing
	NormalizedText string

	// Chunks after chunking stage
	Chunks []Chunk

	// Analysis results after analyze stage
	Analysis *AnalysisResult

	// Scores after scoring stage
	Scores *ScoreResult

	// Processing metadata
	JobID       string
	TenantID    string
	StartedAt   time.Time
	CompletedAt time.Time
	DryRun      bool

	// Stage completion tracking
	CompletedStages []StageType
	CurrentStage    StageType
	Errors          []StageError
}

// Chunk represents a semantic chunk of content.
type Chunk struct {
	ID        string
	Index     int
	Text      string
	StartPos  int
	EndPos    int
	Embedding []float32
	Metadata  map[string]string
}

// AnalysisResult contains metadata extracted during analysis.
type AnalysisResult struct {
	Language         string
	DetectedLanguage float64 // Confidence score
	Entities         []Entity
	Topics           []string
	Keywords         []string
	ContentType      string
	Metadata         map[string]string
}

// Entity represents an extracted entity from content.
type Entity struct {
	Type       string
	Value      string
	Confidence float64
	Position   int
}

// ScoreResult contains relevance and importance scores.
type ScoreResult struct {
	ImportanceScore float64
	RelevanceScore  float64
	QualityScore    float64
	RecencyScore    float64
	OverallScore    float64
	Details         map[string]float64
}

// StageError captures an error from a specific stage.
type StageError struct {
	Stage     StageType
	StageName string
	Error     error
	Timestamp time.Time
	Retryable bool
}

// Config holds configuration for the pipeline.
type Config struct {
	// MaxConcurrentProcessing limits parallel processing.
	MaxConcurrentProcessing int

	// StageTimeout is the default timeout for each stage.
	StageTimeout time.Duration

	// EnableParallelStages allows parallel execution where supported.
	EnableParallelStages bool

	// DefaultDryRun sets default dry-run mode.
	DefaultDryRun bool

	// RetryAttempts for failed stages.
	RetryAttempts int

	// RetryDelay between retry attempts.
	RetryDelay time.Duration

	// StageOrder defines the order of stage types to execute.
	// If nil, uses default order: preprocess -> chunk -> analyze -> score -> store
	StageOrder []StageType
}

// DefaultConfig returns configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MaxConcurrentProcessing: 10,
		StageTimeout:            5 * time.Minute,
		EnableParallelStages:    true,
		DefaultDryRun:           false,
		RetryAttempts:           3,
		RetryDelay:              1 * time.Second,
		StageOrder: []StageType{
			StageTypePreprocess,
			StageTypeChunk,
			StageTypeAnalyze,
			StageTypeScore,
			StageTypeStore,
		},
	}
}

// Pipeline orchestrates content processing through configurable stages.
type Pipeline struct {
	stages      map[StageType][]Stage
	stageOrder  []StageType
	config      *Config
	logger      logging.Logger
	metrics     *PipelineMetrics
	running     bool
	mu          sync.RWMutex
	processSem  chan struct{}
}

// PipelineMetrics holds Prometheus metrics for the pipeline.
type PipelineMetrics struct {
	// ProcessingDuration tracks time spent in each stage
	ProcessingDuration *prometheus.HistogramVec

	// StageErrors counts errors by stage
	StageErrors *prometheus.CounterVec

	// ProcessedItems counts total items processed
	ProcessedItems *prometheus.CounterVec

	// ActiveProcessing tracks current concurrent processing
	ActiveProcessing prometheus.Gauge

	// Throughput tracks items per second
	Throughput prometheus.Gauge
}

// NewPipelineMetrics creates metrics for pipeline monitoring.
func NewPipelineMetrics(namespace, serviceName string) *PipelineMetrics {
	return &PipelineMetrics{
		ProcessingDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "pipeline_stage_duration_seconds",
				Help:      "Duration of pipeline stage processing in seconds",
				Buckets:   []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"stage", "status"},
		),
		StageErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "pipeline_stage_errors_total",
				Help:      "Total number of pipeline stage errors",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"stage", "error_type"},
		),
		ProcessedItems: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "pipeline_items_processed_total",
				Help:      "Total number of items processed through the pipeline",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
			[]string{"status"},
		),
		ActiveProcessing: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "pipeline_active_processing",
				Help:      "Number of items currently being processed",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
		),
		Throughput: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "pipeline_throughput_items_per_second",
				Help:      "Current throughput in items per second",
				ConstLabels: prometheus.Labels{
					"service": serviceName,
				},
			},
		),
	}
}

// RegisterMetrics registers all pipeline metrics with the provided registry.
func (m *PipelineMetrics) RegisterMetrics(reg *prometheus.Registry) error {
	collectors := []prometheus.Collector{
		m.ProcessingDuration,
		m.StageErrors,
		m.ProcessedItems,
		m.ActiveProcessing,
		m.Throughput,
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

// NewPipeline creates a new processing pipeline.
func NewPipeline(cfg *Config, logger logging.Logger, metrics *PipelineMetrics) *Pipeline {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if cfg.StageOrder == nil {
		cfg.StageOrder = DefaultConfig().StageOrder
	}

	p := &Pipeline{
		stages:     make(map[StageType][]Stage),
		stageOrder: cfg.StageOrder,
		config:     cfg,
		logger:     logger.With(logging.F("component", "pipeline")),
		metrics:    metrics,
		running:    true,
		processSem: make(chan struct{}, cfg.MaxConcurrentProcessing),
	}

	// Initialize the semaphore
	for i := 0; i < cfg.MaxConcurrentProcessing; i++ {
		p.processSem <- struct{}{}
	}

	return p
}

// RegisterStage adds a stage to the pipeline at its designated type position.
// Multiple stages can be registered for the same type and will run in order.
func (p *Pipeline) RegisterStage(stage Stage) {
	p.mu.Lock()
	defer p.mu.Unlock()

	stageType := stage.Type()
	p.stages[stageType] = append(p.stages[stageType], stage)

	p.logger.Debug("Registered pipeline stage",
		logging.F("stage_name", stage.Name()),
		logging.F("stage_type", string(stageType)),
		logging.F("total_stages_for_type", len(p.stages[stageType])),
	)
}

// SetStageOrder configures the order in which stage types are executed.
func (p *Pipeline) SetStageOrder(order []StageType) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stageOrder = order
}

// Process executes the full pipeline on the given content.
// Returns ProcessedContent with all accumulated data from stages.
func (p *Pipeline) Process(ctx context.Context, item *contentv1.ContentItem) (*ProcessedContent, error) {
	return p.ProcessWithOptions(ctx, item, nil)
}

// ProcessOptions configures a single processing run.
type ProcessOptions struct {
	DryRun       bool
	SkipStages   []StageType
	OnlyStages   []StageType
	JobID        string
	StageTimeout time.Duration
}

// ProcessWithOptions executes the pipeline with custom options.
func (p *Pipeline) ProcessWithOptions(ctx context.Context, item *contentv1.ContentItem, opts *ProcessOptions) (*ProcessedContent, error) {
	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return nil, ErrPipelineNotRunning
	}
	p.mu.RUnlock()

	// Acquire processing slot
	select {
	case <-p.processSem:
		defer func() { p.processSem <- struct{}{} }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Track active processing
	if p.metrics != nil {
		p.metrics.ActiveProcessing.Inc()
		defer p.metrics.ActiveProcessing.Dec()
	}

	if item == nil || item.GetRawContent() == "" {
		return nil, ErrEmptyContent
	}

	// Initialize options
	if opts == nil {
		opts = &ProcessOptions{
			DryRun: p.config.DefaultDryRun,
		}
	}

	stageTimeout := p.config.StageTimeout
	if opts.StageTimeout > 0 {
		stageTimeout = opts.StageTimeout
	}

	// Initialize processed content
	content := &ProcessedContent{
		Item:            item,
		NormalizedText:  item.GetRawContent(),
		JobID:           opts.JobID,
		TenantID:        item.GetTenantId(),
		StartedAt:       time.Now(),
		DryRun:          opts.DryRun,
		CompletedStages: make([]StageType, 0),
		Errors:          make([]StageError, 0),
		Chunks:          make([]Chunk, 0),
	}

	p.logger.Debug("Starting pipeline processing",
		logging.F("content_id", item.GetId()),
		logging.F("job_id", opts.JobID),
		logging.F("dry_run", opts.DryRun),
		logging.F("content_length", len(item.GetRawContent())),
	)

	// Build stage execution list
	stagesToRun := p.buildStageList(opts)

	// Execute stages in order
	var completedStages []Stage
	var err error

	for _, stageType := range p.stageOrder {
		// Check if we should skip this stage type
		if !p.shouldRunStageType(stageType, opts) {
			continue
		}

		stages, ok := stagesToRun[stageType]
		if !ok || len(stages) == 0 {
			continue
		}

		content.CurrentStage = stageType

		// Run stages of this type
		for _, stage := range stages {
			stageCtx, cancel := context.WithTimeout(ctx, stageTimeout)

			start := time.Now()
			content, err = p.executeStageWithRetry(stageCtx, stage, content)
			duration := time.Since(start)
			cancel()

			// Record metrics
			if p.metrics != nil {
				status := "success"
				if err != nil {
					status = "error"
				}
				p.metrics.ProcessingDuration.WithLabelValues(string(stageType), status).Observe(duration.Seconds())
			}

			if err != nil {
				// Record error
				content.Errors = append(content.Errors, StageError{
					Stage:     stageType,
					StageName: stage.Name(),
					Error:     err,
					Timestamp: time.Now(),
					Retryable: p.isRetryableError(err),
				})

				if p.metrics != nil {
					p.metrics.StageErrors.WithLabelValues(string(stageType), "execution").Inc()
				}

				p.logger.Error("Stage execution failed",
					logging.F("stage_name", stage.Name()),
					logging.F("stage_type", string(stageType)),
					logging.F("content_id", item.GetId()),
					logging.Err(err),
				)

				// Rollback completed stages
				if rollbackErr := p.rollbackStages(ctx, completedStages, content); rollbackErr != nil {
					p.logger.Error("Rollback failed",
						logging.F("content_id", item.GetId()),
						logging.Err(rollbackErr),
					)
				}

				if p.metrics != nil {
					p.metrics.ProcessedItems.WithLabelValues("failed").Inc()
				}

				return content, fmt.Errorf("%w: stage %s: %v", ErrStageFailed, stage.Name(), err)
			}

			completedStages = append(completedStages, stage)
			p.logger.Debug("Stage completed",
				logging.F("stage_name", stage.Name()),
				logging.F("duration_ms", duration.Milliseconds()),
			)
		}

		content.CompletedStages = append(content.CompletedStages, stageType)
	}

	content.CompletedAt = time.Now()
	content.CurrentStage = ""

	// Record success metrics
	if p.metrics != nil {
		p.metrics.ProcessedItems.WithLabelValues("success").Inc()
		totalDuration := content.CompletedAt.Sub(content.StartedAt).Seconds()
		if totalDuration > 0 {
			p.metrics.Throughput.Set(1.0 / totalDuration)
		}
	}

	p.logger.Info("Pipeline processing completed",
		logging.F("content_id", item.GetId()),
		logging.F("job_id", opts.JobID),
		logging.F("total_duration_ms", content.CompletedAt.Sub(content.StartedAt).Milliseconds()),
		logging.F("stages_completed", len(content.CompletedStages)),
		logging.F("chunks_created", len(content.Chunks)),
		logging.F("dry_run", opts.DryRun),
	)

	// In dry-run mode, return a marker error along with the content
	if opts.DryRun {
		return content, ErrDryRunMode
	}

	return content, nil
}

// buildStageList builds the map of stages to run based on options.
func (p *Pipeline) buildStageList(opts *ProcessOptions) map[StageType][]Stage {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[StageType][]Stage)

	for stageType, stages := range p.stages {
		// Skip if in skip list
		if opts != nil && contains(opts.SkipStages, stageType) {
			continue
		}

		// If OnlyStages is specified, only include those
		if opts != nil && len(opts.OnlyStages) > 0 && !contains(opts.OnlyStages, stageType) {
			continue
		}

		result[stageType] = append(result[stageType], stages...)
	}

	return result
}

// shouldRunStageType checks if a stage type should be executed.
func (p *Pipeline) shouldRunStageType(stageType StageType, opts *ProcessOptions) bool {
	if opts == nil {
		return true
	}

	// Check skip list
	if contains(opts.SkipStages, stageType) {
		return false
	}

	// Check only list
	if len(opts.OnlyStages) > 0 && !contains(opts.OnlyStages, stageType) {
		return false
	}

	return true
}

// executeStageWithRetry executes a stage with retry logic.
func (p *Pipeline) executeStageWithRetry(ctx context.Context, stage Stage, content *ProcessedContent) (*ProcessedContent, error) {
	var lastErr error
	var result *ProcessedContent

	for attempt := 0; attempt <= p.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			p.logger.Debug("Retrying stage",
				logging.F("stage_name", stage.Name()),
				logging.F("attempt", attempt),
			)

			select {
			case <-ctx.Done():
				return content, ctx.Err()
			case <-time.After(p.config.RetryDelay):
			}
		}

		result, lastErr = stage.Process(ctx, content)
		if lastErr == nil {
			return result, nil
		}

		if !p.isRetryableError(lastErr) {
			return content, lastErr
		}

		p.logger.Warn("Stage failed, will retry",
			logging.F("stage_name", stage.Name()),
			logging.F("attempt", attempt),
			logging.Err(lastErr),
		)
	}

	return content, lastErr
}

// rollbackStages performs rollback on completed stages in reverse order.
func (p *Pipeline) rollbackStages(ctx context.Context, stages []Stage, content *ProcessedContent) error {
	var errs []error

	// Rollback in reverse order
	for i := len(stages) - 1; i >= 0; i-- {
		stage := stages[i]

		if err := stage.Rollback(ctx, content); err != nil {
			p.logger.Error("Stage rollback failed",
				logging.F("stage_name", stage.Name()),
				logging.Err(err),
			)
			errs = append(errs, err)
		} else {
			p.logger.Debug("Stage rolled back",
				logging.F("stage_name", stage.Name()),
			)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %d stage(s) failed rollback", ErrRollbackFailed, len(errs))
	}

	return nil
}

// isRetryableError determines if an error should trigger a retry.
func (p *Pipeline) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Context errors are not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check for specific non-retryable errors
	if errors.Is(err, ErrEmptyContent) {
		return false
	}

	// Default to retryable for transient errors
	return true
}

// Stop gracefully stops the pipeline.
func (p *Pipeline) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running = false
}

// IsRunning returns whether the pipeline is accepting new work.
func (p *Pipeline) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

// GetStages returns all registered stages.
func (p *Pipeline) GetStages() map[StageType][]Stage {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[StageType][]Stage)
	for k, v := range p.stages {
		result[k] = append(result[k], v...)
	}
	return result
}

// GetConfig returns the pipeline configuration.
func (p *Pipeline) GetConfig() *Config {
	return p.config
}

// contains checks if a slice contains a value.
func contains(slice []StageType, val StageType) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
