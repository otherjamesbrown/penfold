// Package ensemble provides ensemble processing capabilities for AI model coordination.
// It enables improved accuracy through consensus, fallback on disagreement, and
// configurable latency vs accuracy tradeoffs.
package ensemble

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/prometheus/client_golang/prometheus"
)

// Errors returned by the ensemble processor.
var (
	ErrNoModels           = errors.New("no models configured for ensemble")
	ErrNoResults          = errors.New("no results received from any model")
	ErrInsufficientQuorum = errors.New("insufficient responses for quorum")
	ErrContextCanceled    = errors.New("context canceled during processing")
	ErrStrategyNotFound   = errors.New("ensemble strategy not found")
	ErrModelNotFound      = errors.New("model not found in ensemble")
	ErrInvalidRequest     = errors.New("invalid ensemble request")
)

// ModelResult represents the result from a single model in the ensemble.
type ModelResult struct {
	// ModelID is the identifier of the model that produced this result.
	ModelID string

	// Provider is the model provider (e.g., "mlx", "gemini").
	Provider string

	// Response is the model's response data.
	Response interface{}

	// Confidence is the model's confidence in its response (0.0 to 1.0).
	Confidence float64

	// ProcessingTime is how long the model took to respond.
	ProcessingTime time.Duration

	// TokensUsed is the number of tokens consumed.
	TokensUsed int

	// Error is set if the model failed to respond.
	Error error

	// Metadata contains additional model-specific information.
	Metadata map[string]interface{}
}

// IsSuccess returns true if the model produced a valid result.
func (r *ModelResult) IsSuccess() bool {
	return r.Error == nil && r.Response != nil
}

// EnsembleRequest represents a request to be processed by the ensemble.
type EnsembleRequest struct {
	// ID is a unique identifier for this request.
	ID string

	// Type specifies the request type (embedding, summary, extraction, classification).
	Type RequestType

	// Content is the input content to process.
	Content string

	// Strategy specifies which ensemble strategy to use.
	Strategy StrategyType

	// Options contains strategy-specific options.
	Options map[string]interface{}

	// Timeout overrides the default request timeout.
	Timeout time.Duration

	// MinResponses is the minimum number of model responses required.
	// If zero, defaults to 1.
	MinResponses int

	// RequireConsensus requires models to agree before returning.
	RequireConsensus bool

	// ConsensusThreshold is the minimum agreement ratio (0.0 to 1.0).
	ConsensusThreshold float64

	// MaxCost is the maximum acceptable total cost for this request.
	MaxCost float64

	// Metadata contains additional request metadata.
	Metadata map[string]interface{}
}

// RequestType identifies the type of AI request.
type RequestType string

const (
	RequestTypeEmbedding      RequestType = "embedding"
	RequestTypeSummary        RequestType = "summary"
	RequestTypeExtraction     RequestType = "extraction"
	RequestTypeClassification RequestType = "classification"
	RequestTypeChat           RequestType = "chat"
)

// EnsembleResult represents the aggregated result from the ensemble.
type EnsembleResult struct {
	// RequestID links back to the original request.
	RequestID string

	// FinalResponse is the aggregated/selected response.
	FinalResponse interface{}

	// SelectedModel is the model whose response was selected (for single-model strategies).
	SelectedModel string

	// ConsensusScore indicates how much the models agreed (0.0 to 1.0).
	ConsensusScore float64

	// IndividualResults contains results from each model.
	IndividualResults []ModelResult

	// TotalProcessingTime is the total time for ensemble processing.
	TotalProcessingTime time.Duration

	// TotalTokensUsed is the sum of tokens used across all models.
	TotalTokensUsed int

	// EstimatedCost is the estimated total cost of this ensemble operation.
	EstimatedCost float64

	// Disagreements describes any significant disagreements between models.
	Disagreements []Disagreement

	// Explanation describes how the final response was determined.
	Explanation string

	// Strategy is the strategy that was used.
	Strategy StrategyType

	// Metadata contains additional result metadata.
	Metadata map[string]interface{}
}

// Disagreement represents a significant disagreement between models.
type Disagreement struct {
	// ModelA is the first model in the disagreement.
	ModelA string

	// ModelB is the second model in the disagreement.
	ModelB string

	// Description describes the nature of the disagreement.
	Description string

	// Severity indicates how significant the disagreement is (0.0 to 1.0).
	Severity float64
}

// ModelBackend represents an AI model backend that can process requests.
type ModelBackend interface {
	// Name returns the unique identifier for this backend.
	Name() string

	// Provider returns the provider name (e.g., "mlx", "gemini", "openai").
	Provider() string

	// IsLocal returns true if this is a local model.
	IsLocal() bool

	// Process handles an AI request and returns the response.
	Process(ctx context.Context, content string, reqType RequestType, options map[string]interface{}) (*ModelResult, error)

	// CostPer1KTokens returns the estimated cost per 1000 tokens.
	CostPer1KTokens() float64

	// QualityScore returns the model's quality rating (0.0 to 1.0).
	QualityScore() float64
}

// EnsembleProcessor manages ensemble AI model processing.
type EnsembleProcessor struct {
	mu sync.RWMutex

	// models stores registered model backends.
	models map[string]ModelBackend

	// strategies stores registered ensemble strategies.
	strategies map[StrategyType]Strategy

	// config holds processor configuration.
	config *ProcessorConfig

	// orchestrator handles request orchestration.
	orchestrator *Orchestrator

	// aggregator handles result aggregation.
	aggregator *Aggregator

	// logger for processing operations.
	logger logging.Logger

	// metrics for processor operations.
	metrics *ProcessorMetrics
}

// ProcessorConfig holds configuration for the EnsembleProcessor.
type ProcessorConfig struct {
	// DefaultTimeout is the default timeout for ensemble requests.
	DefaultTimeout time.Duration

	// DefaultStrategy is the default ensemble strategy.
	DefaultStrategy StrategyType

	// DefaultMinResponses is the default minimum responses required.
	DefaultMinResponses int

	// DefaultConsensusThreshold is the default consensus threshold.
	DefaultConsensusThreshold float64

	// MaxConcurrentModels is the maximum models to query in parallel.
	MaxConcurrentModels int

	// EnableMetrics enables Prometheus metrics collection.
	EnableMetrics bool

	// MetricsNamespace is the Prometheus namespace for metrics.
	MetricsNamespace string

	// RetryFailedModels enables retrying individual model failures.
	RetryFailedModels bool

	// MaxRetries is the maximum retries per model.
	MaxRetries int
}

// DefaultProcessorConfig returns a configuration with sensible defaults.
func DefaultProcessorConfig() *ProcessorConfig {
	return &ProcessorConfig{
		DefaultTimeout:            30 * time.Second,
		DefaultStrategy:           StrategyParallel,
		DefaultMinResponses:       1,
		DefaultConsensusThreshold: 0.7,
		MaxConcurrentModels:       5,
		EnableMetrics:             true,
		MetricsNamespace:          "penfold_ai",
		RetryFailedModels:         true,
		MaxRetries:                2,
	}
}

// ProcessorMetrics holds Prometheus metrics for the processor.
type ProcessorMetrics struct {
	RequestsTotal       *prometheus.CounterVec
	ProcessingDuration  prometheus.Histogram
	ModelsQueried       prometheus.Histogram
	ConsensusScore      prometheus.Histogram
	DisagreementCount   prometheus.Histogram
	ModelResponseTime   *prometheus.HistogramVec
	ModelSuccessRate    *prometheus.CounterVec
	StrategyUsage       *prometheus.CounterVec
	EstimatedCost       prometheus.Counter
	InsufficientQuorum  prometheus.Counter
}

// NewProcessorMetrics creates Prometheus metrics for the ensemble processor.
func NewProcessorMetrics(namespace string) *ProcessorMetrics {
	return &ProcessorMetrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "ensemble",
				Name:      "requests_total",
				Help:      "Total number of ensemble requests by strategy and status",
			},
			[]string{"strategy", "request_type", "status"},
		),
		ProcessingDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "ensemble",
				Name:      "processing_duration_seconds",
				Help:      "Duration of ensemble processing operations",
				Buckets:   []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60},
			},
		),
		ModelsQueried: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "ensemble",
				Name:      "models_queried",
				Help:      "Number of models queried per ensemble request",
				Buckets:   []float64{1, 2, 3, 4, 5, 7, 10},
			},
		),
		ConsensusScore: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "ensemble",
				Name:      "consensus_score",
				Help:      "Consensus score distribution",
				Buckets:   []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
			},
		),
		DisagreementCount: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "ensemble",
				Name:      "disagreement_count",
				Help:      "Number of disagreements per ensemble request",
				Buckets:   []float64{0, 1, 2, 3, 5, 10},
			},
		),
		ModelResponseTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "ensemble",
				Name:      "model_response_seconds",
				Help:      "Response time per model in ensemble",
				Buckets:   []float64{.1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"model_id", "provider"},
		),
		ModelSuccessRate: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "ensemble",
				Name:      "model_responses_total",
				Help:      "Total model responses by model and status",
			},
			[]string{"model_id", "status"},
		),
		StrategyUsage: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "ensemble",
				Name:      "strategy_usage_total",
				Help:      "Usage count per ensemble strategy",
			},
			[]string{"strategy"},
		),
		EstimatedCost: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "ensemble",
				Name:      "estimated_cost_total",
				Help:      "Total estimated cost across all ensemble operations",
			},
		),
		InsufficientQuorum: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "ensemble",
				Name:      "insufficient_quorum_total",
				Help:      "Count of requests failing due to insufficient quorum",
			},
		),
	}
}

// RegisterMetrics registers all processor metrics with Prometheus.
func (m *ProcessorMetrics) RegisterMetrics() error {
	collectors := []prometheus.Collector{
		m.RequestsTotal,
		m.ProcessingDuration,
		m.ModelsQueried,
		m.ConsensusScore,
		m.DisagreementCount,
		m.ModelResponseTime,
		m.ModelSuccessRate,
		m.StrategyUsage,
		m.EstimatedCost,
		m.InsufficientQuorum,
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

// NewEnsembleProcessor creates a new ensemble processor with the given configuration.
func NewEnsembleProcessor(cfg *ProcessorConfig, logger logging.Logger) *EnsembleProcessor {
	if cfg == nil {
		cfg = DefaultProcessorConfig()
	}

	if logger == nil {
		logger = logging.MustGlobal()
	}

	ep := &EnsembleProcessor{
		models:     make(map[string]ModelBackend),
		strategies: make(map[StrategyType]Strategy),
		config:     cfg,
		logger:     logger.With(logging.F("component", "ensemble_processor")),
	}

	// Initialize orchestrator
	ep.orchestrator = NewOrchestrator(&OrchestratorConfig{
		DefaultTimeout:      cfg.DefaultTimeout,
		MaxConcurrentModels: cfg.MaxConcurrentModels,
		RetryFailedModels:   cfg.RetryFailedModels,
		MaxRetries:          cfg.MaxRetries,
	}, ep.logger)

	// Initialize aggregator
	ep.aggregator = NewAggregator(&AggregatorConfig{
		DefaultConsensusThreshold: cfg.DefaultConsensusThreshold,
	}, ep.logger)

	// Register built-in strategies
	ep.registerBuiltinStrategies()

	if cfg.EnableMetrics {
		ep.metrics = NewProcessorMetrics(cfg.MetricsNamespace)
		if err := ep.metrics.RegisterMetrics(); err != nil {
			ep.logger.Warn("Failed to register some metrics", logging.Err(err))
		}
	}

	return ep
}

// registerBuiltinStrategies registers the built-in ensemble strategies.
func (ep *EnsembleProcessor) registerBuiltinStrategies() {
	ep.strategies[StrategyMajorityVoting] = NewMajorityVotingStrategy()
	ep.strategies[StrategyWeightedVoting] = NewWeightedVotingStrategy()
	ep.strategies[StrategyCascade] = NewCascadeStrategy()
	ep.strategies[StrategyParallel] = NewParallelStrategy()
	ep.strategies[StrategyDebate] = NewDebateStrategy(ep.logger)
}

// Process processes a request through the ensemble.
func (ep *EnsembleProcessor) Process(ctx context.Context, req *EnsembleRequest) (*EnsembleResult, error) {
	startTime := time.Now()

	if req == nil {
		return nil, ErrInvalidRequest
	}

	// Apply defaults
	req = ep.applyDefaults(req)

	// Get the strategy
	strategy, err := ep.getStrategy(req.Strategy)
	if err != nil {
		return nil, err
	}

	// Get available models
	models := ep.getAvailableModels()
	if len(models) == 0 {
		return nil, ErrNoModels
	}

	// Record strategy usage
	if ep.metrics != nil {
		ep.metrics.StrategyUsage.WithLabelValues(string(req.Strategy)).Inc()
	}

	// Execute the strategy
	result, err := strategy.Execute(ctx, req, models, ep.orchestrator, ep.aggregator)
	if err != nil {
		ep.recordMetrics(req, nil, startTime, err)
		return nil, err
	}

	// Populate result metadata
	result.RequestID = req.ID
	result.Strategy = req.Strategy
	result.TotalProcessingTime = time.Since(startTime)

	// Calculate estimated cost
	for _, r := range result.IndividualResults {
		if r.IsSuccess() {
			if backend, ok := ep.models[r.ModelID]; ok {
				cost := float64(r.TokensUsed) / 1000.0 * backend.CostPer1KTokens()
				result.EstimatedCost += cost
			}
		}
	}

	// Record metrics
	ep.recordMetrics(req, result, startTime, nil)

	ep.logger.Debug("Ensemble processing complete",
		logging.F("request_id", req.ID),
		logging.F("strategy", string(req.Strategy)),
		logging.F("models_queried", len(result.IndividualResults)),
		logging.F("consensus_score", result.ConsensusScore),
		logging.F("processing_time_ms", result.TotalProcessingTime.Milliseconds()),
	)

	return result, nil
}

// applyDefaults applies default values to the request.
func (ep *EnsembleProcessor) applyDefaults(req *EnsembleRequest) *EnsembleRequest {
	// Create a copy to avoid modifying the original
	r := *req

	if r.Strategy == "" {
		r.Strategy = ep.config.DefaultStrategy
	}
	if r.Timeout <= 0 {
		r.Timeout = ep.config.DefaultTimeout
	}
	if r.MinResponses <= 0 {
		r.MinResponses = ep.config.DefaultMinResponses
	}
	if r.ConsensusThreshold <= 0 {
		r.ConsensusThreshold = ep.config.DefaultConsensusThreshold
	}

	return &r
}

// getStrategy retrieves a strategy by type.
func (ep *EnsembleProcessor) getStrategy(strategyType StrategyType) (Strategy, error) {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	strategy, ok := ep.strategies[strategyType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrStrategyNotFound, strategyType)
	}
	return strategy, nil
}

// getAvailableModels returns all registered models.
func (ep *EnsembleProcessor) getAvailableModels() []ModelBackend {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	models := make([]ModelBackend, 0, len(ep.models))
	for _, m := range ep.models {
		models = append(models, m)
	}
	return models
}

// AddModel registers a model backend with the ensemble processor.
func (ep *EnsembleProcessor) AddModel(model ModelBackend) error {
	if model == nil {
		return errors.New("model cannot be nil")
	}

	name := model.Name()
	if name == "" {
		return errors.New("model name cannot be empty")
	}

	ep.mu.Lock()
	defer ep.mu.Unlock()

	ep.models[name] = model

	ep.logger.Info("Model added to ensemble",
		logging.F("model_id", name),
		logging.F("provider", model.Provider()),
		logging.F("is_local", model.IsLocal()),
	)

	return nil
}

// RemoveModel removes a model from the ensemble processor.
func (ep *EnsembleProcessor) RemoveModel(modelID string) error {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	if _, exists := ep.models[modelID]; !exists {
		return ErrModelNotFound
	}

	delete(ep.models, modelID)

	ep.logger.Info("Model removed from ensemble", logging.F("model_id", modelID))

	return nil
}

// GetModels returns all registered models.
func (ep *EnsembleProcessor) GetModels() []ModelBackend {
	return ep.getAvailableModels()
}

// GetModel retrieves a specific model by ID.
func (ep *EnsembleProcessor) GetModel(modelID string) (ModelBackend, error) {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	model, ok := ep.models[modelID]
	if !ok {
		return nil, ErrModelNotFound
	}
	return model, nil
}

// RegisterStrategy registers a custom ensemble strategy.
func (ep *EnsembleProcessor) RegisterStrategy(strategyType StrategyType, strategy Strategy) error {
	if strategy == nil {
		return errors.New("strategy cannot be nil")
	}

	ep.mu.Lock()
	defer ep.mu.Unlock()

	ep.strategies[strategyType] = strategy

	ep.logger.Info("Strategy registered", logging.F("strategy", string(strategyType)))

	return nil
}

// GetSupportedStrategies returns all registered strategy types.
func (ep *EnsembleProcessor) GetSupportedStrategies() []StrategyType {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	strategies := make([]StrategyType, 0, len(ep.strategies))
	for st := range ep.strategies {
		strategies = append(strategies, st)
	}
	return strategies
}

// recordMetrics records metrics for a processed request.
func (ep *EnsembleProcessor) recordMetrics(req *EnsembleRequest, result *EnsembleResult, startTime time.Time, err error) {
	if ep.metrics == nil {
		return
	}

	duration := time.Since(startTime)
	status := "success"
	if err != nil {
		status = "error"
		if errors.Is(err, ErrInsufficientQuorum) {
			ep.metrics.InsufficientQuorum.Inc()
		}
	}

	ep.metrics.RequestsTotal.WithLabelValues(string(req.Strategy), string(req.Type), status).Inc()
	ep.metrics.ProcessingDuration.Observe(duration.Seconds())

	if result != nil {
		ep.metrics.ModelsQueried.Observe(float64(len(result.IndividualResults)))
		ep.metrics.ConsensusScore.Observe(result.ConsensusScore)
		ep.metrics.DisagreementCount.Observe(float64(len(result.Disagreements)))
		ep.metrics.EstimatedCost.Add(result.EstimatedCost)

		for _, r := range result.IndividualResults {
			modelStatus := "success"
			if r.Error != nil {
				modelStatus = "error"
			}
			ep.metrics.ModelSuccessRate.WithLabelValues(r.ModelID, modelStatus).Inc()
			if r.ProcessingTime > 0 {
				ep.metrics.ModelResponseTime.WithLabelValues(r.ModelID, r.Provider).Observe(r.ProcessingTime.Seconds())
			}
		}
	}
}

// UpdateConfig updates the processor configuration.
func (ep *EnsembleProcessor) UpdateConfig(cfg *ProcessorConfig) {
	if cfg == nil {
		return
	}

	ep.mu.Lock()
	defer ep.mu.Unlock()

	ep.config = cfg

	// Update orchestrator config
	if ep.orchestrator != nil {
		ep.orchestrator.UpdateConfig(&OrchestratorConfig{
			DefaultTimeout:      cfg.DefaultTimeout,
			MaxConcurrentModels: cfg.MaxConcurrentModels,
			RetryFailedModels:   cfg.RetryFailedModels,
			MaxRetries:          cfg.MaxRetries,
		})
	}

	// Update aggregator config
	if ep.aggregator != nil {
		ep.aggregator.UpdateConfig(&AggregatorConfig{
			DefaultConsensusThreshold: cfg.DefaultConsensusThreshold,
		})
	}

	ep.logger.Info("Processor configuration updated")
}

// GetConfig returns the current processor configuration.
func (ep *EnsembleProcessor) GetConfig() *ProcessorConfig {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return ep.config
}
