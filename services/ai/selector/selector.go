package selector

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/prometheus/client_golang/prometheus"
)

// Errors returned by the model selector.
var (
	ErrNoModelsRegistered = errors.New("no models registered")
	ErrNoSuitableModel    = errors.New("no suitable model found for criteria")
	ErrModelNotFound      = errors.New("model not found")
)

// ModelConfig represents a fully configured model ready for use.
type ModelConfig struct {
	// ModelID is the unique identifier (e.g., "Qwen2.5-32B", "gemini-1.5-pro").
	ModelID string

	// Provider is the model provider (e.g., "mlx", "gemini").
	Provider ModelProvider

	// Endpoint is the API endpoint for the model.
	Endpoint string

	// APIKey is the authentication key (for cloud models).
	APIKey string

	// Capabilities describes the model's performance characteristics.
	Capabilities *ModelCapabilities

	// Parameters holds model-specific configuration.
	Parameters map[string]interface{}
}

// SelectionResult contains the result of a model selection operation.
type SelectionResult struct {
	// SelectedModel is the chosen model configuration.
	SelectedModel *ModelConfig

	// Score is the model's fitness score.
	Score ModelScore

	// Alternatives lists other candidate models and their scores.
	Alternatives []ModelScore

	// SelectionReason explains why this model was selected.
	SelectionReason string

	// SelectionDuration is how long the selection took.
	SelectionDuration time.Duration
}

// ModelSelector manages AI model selection based on task requirements.
type ModelSelector struct {
	mu sync.RWMutex

	// models stores registered model configurations.
	models map[string]*ModelConfig

	// logger for selection operations.
	logger logging.Logger

	// metrics for selection operations.
	metrics *SelectorMetrics

	// warmupManager tracks model warmup state.
	warmupManager *WarmupManager

	// defaultOptimizationMode is used when criteria doesn't specify one.
	defaultOptimizationMode OptimizationMode

	// warmupThreshold is how long before a model is considered cold.
	warmupThreshold time.Duration
}

// SelectorMetrics holds Prometheus metrics for the selector.
type SelectorMetrics struct {
	SelectionTotal    *prometheus.CounterVec
	SelectionDuration prometheus.Histogram
	ModelSelectCount  *prometheus.CounterVec
	FallbackCount     *prometheus.CounterVec
	NoSuitableModel   prometheus.Counter
	WarmupTriggered   *prometheus.CounterVec
	RegisteredModels  prometheus.Gauge
}

// NewSelectorMetrics creates Prometheus metrics for the model selector.
func NewSelectorMetrics(namespace string) *SelectorMetrics {
	return &SelectorMetrics{
		SelectionTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "model_selection_total",
				Help:      "Total number of model selection operations",
			},
			[]string{"task_type", "optimization_mode", "result"},
		),
		SelectionDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "model_selection_duration_seconds",
				Help:      "Duration of model selection operations",
				Buckets:   []float64{.0001, .0005, .001, .005, .01, .05, .1},
			},
		),
		ModelSelectCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "model_selected_total",
				Help:      "Count of times each model was selected",
			},
			[]string{"model_id", "provider", "task_type"},
		),
		FallbackCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "model_fallback_total",
				Help:      "Count of fallbacks from local to cloud models",
			},
			[]string{"task_type", "reason"},
		),
		NoSuitableModel: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "no_suitable_model_total",
				Help:      "Count of selection failures due to no suitable model",
			},
		),
		WarmupTriggered: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "model_warmup_triggered_total",
				Help:      "Count of model warmup triggers",
			},
			[]string{"model_id"},
		),
		RegisteredModels: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "registered_models",
				Help:      "Number of registered models",
			},
		),
	}
}

// RegisterMetrics registers all selector metrics with Prometheus.
func (m *SelectorMetrics) RegisterMetrics() error {
	collectors := []prometheus.Collector{
		m.SelectionTotal,
		m.SelectionDuration,
		m.ModelSelectCount,
		m.FallbackCount,
		m.NoSuitableModel,
		m.WarmupTriggered,
		m.RegisteredModels,
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

// SelectorConfig holds configuration for the ModelSelector.
type SelectorConfig struct {
	// DefaultOptimizationMode is used when criteria doesn't specify one.
	DefaultOptimizationMode OptimizationMode

	// WarmupThreshold is how long before a model is considered cold.
	WarmupThreshold time.Duration

	// EnableMetrics enables Prometheus metrics collection.
	EnableMetrics bool

	// MetricsNamespace is the Prometheus namespace for metrics.
	MetricsNamespace string
}

// DefaultSelectorConfig returns a configuration with sensible defaults.
func DefaultSelectorConfig() *SelectorConfig {
	return &SelectorConfig{
		DefaultOptimizationMode: OptimizationModeBalanced,
		WarmupThreshold:         5 * time.Minute,
		EnableMetrics:           true,
		MetricsNamespace:        "penfold_ai",
	}
}

// NewModelSelector creates a new model selector with the given configuration.
func NewModelSelector(cfg *SelectorConfig, logger logging.Logger) *ModelSelector {
	if cfg == nil {
		cfg = DefaultSelectorConfig()
	}

	selector := &ModelSelector{
		models:                  make(map[string]*ModelConfig),
		logger:                  logger.With(logging.F("component", "model_selector")),
		warmupThreshold:         cfg.WarmupThreshold,
		defaultOptimizationMode: cfg.DefaultOptimizationMode,
	}

	if cfg.EnableMetrics {
		selector.metrics = NewSelectorMetrics(cfg.MetricsNamespace)
	}

	selector.warmupManager = NewWarmupManager(selector, logger)

	return selector
}

// RegisterModel adds or updates a model in the selector's registry.
func (s *ModelSelector) RegisterModel(config *ModelConfig) error {
	if config == nil {
		return errors.New("model config cannot be nil")
	}
	if config.ModelID == "" {
		return errors.New("model ID cannot be empty")
	}
	if config.Capabilities == nil {
		return errors.New("model capabilities cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.models[config.ModelID] = config

	s.logger.Info("model registered",
		logging.F("model_id", config.ModelID),
		logging.F("provider", string(config.Provider)),
		logging.F("is_local", config.Capabilities.IsLocal),
		logging.F("supported_tasks", config.Capabilities.SupportedTasks),
	)

	if s.metrics != nil {
		s.metrics.RegisteredModels.Set(float64(len(s.models)))
	}

	return nil
}

// UnregisterModel removes a model from the registry.
func (s *ModelSelector) UnregisterModel(modelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.models[modelID]; !exists {
		return ErrModelNotFound
	}

	delete(s.models, modelID)

	s.logger.Info("model unregistered", logging.F("model_id", modelID))

	if s.metrics != nil {
		s.metrics.RegisteredModels.Set(float64(len(s.models)))
	}

	return nil
}

// GetModel retrieves a specific model configuration by ID.
func (s *ModelSelector) GetModel(modelID string) (*ModelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	model, exists := s.models[modelID]
	if !exists {
		return nil, ErrModelNotFound
	}

	return model, nil
}

// ListModels returns all registered models.
func (s *ModelSelector) ListModels() []*ModelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	models := make([]*ModelConfig, 0, len(s.models))
	for _, m := range s.models {
		models = append(models, m)
	}

	return models
}

// SelectModel selects the optimal model for the given criteria.
func (s *ModelSelector) SelectModel(ctx context.Context, criteria *SelectionCriteria) (*SelectionResult, error) {
	startTime := time.Now()

	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.models) == 0 {
		if s.metrics != nil {
			s.metrics.NoSuitableModel.Inc()
			s.metrics.SelectionTotal.WithLabelValues(string(criteria.TaskType), string(criteria.OptimizationMode), "no_models").Inc()
		}
		return nil, ErrNoModelsRegistered
	}

	// Apply default optimization mode if not specified
	if criteria.OptimizationMode == "" {
		criteria.OptimizationMode = s.defaultOptimizationMode
	}

	// Apply default min quality if not specified
	if criteria.MinQuality == 0 {
		criteria.MinQuality = 0.5
	}

	// Check for preferred model first
	if criteria.PreferredModel != "" {
		if model, exists := s.models[criteria.PreferredModel]; exists {
			if model.Capabilities.IsAvailable && model.Capabilities.SupportsTask(criteria.TaskType) {
				result := s.buildResult(model, criteria, startTime, "preferred model requested")
				s.recordMetrics(criteria, result, "preferred")
				return result, nil
			}
		}
		s.logger.Debug("preferred model not available, selecting alternative",
			logging.F("preferred_model", criteria.PreferredModel),
		)
	}

	// Score all candidate models
	weights := GetWeightsForMode(criteria.OptimizationMode)
	scores := s.scoreAllModels(criteria, weights)

	if len(scores) == 0 {
		if s.metrics != nil {
			s.metrics.NoSuitableModel.Inc()
			s.metrics.SelectionTotal.WithLabelValues(string(criteria.TaskType), string(criteria.OptimizationMode), "no_suitable").Inc()
		}
		return nil, ErrNoSuitableModel
	}

	// Sort by score (highest first)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].TotalScore > scores[j].TotalScore
	})

	// Select the top scoring model
	bestScore := scores[0]
	bestModel := s.models[bestScore.ModelID]

	// Check for local-to-cloud fallback
	selectionReason := "highest scoring model"
	if !bestModel.Capabilities.IsLocal {
		// Check if we fell back from local
		for _, score := range scores {
			if model := s.models[score.ModelID]; model.Capabilities.IsLocal {
				selectionReason = "cloud fallback (local models unsuitable)"
				if s.metrics != nil {
					s.metrics.FallbackCount.WithLabelValues(string(criteria.TaskType), "local_unsuitable").Inc()
				}
				break
			}
		}
	}

	result := &SelectionResult{
		SelectedModel:     bestModel,
		Score:             bestScore,
		Alternatives:      scores[1:],
		SelectionReason:   selectionReason,
		SelectionDuration: time.Since(startTime),
	}

	// Update last used time for warmup tracking
	bestModel.Capabilities.LastUsed = time.Now()

	// Trigger warmup for frequently used models
	s.warmupManager.ConsiderWarmup(bestModel)

	s.recordMetrics(criteria, result, "success")

	s.logger.Debug("model selected",
		logging.F("model_id", bestModel.ModelID),
		logging.F("provider", string(bestModel.Provider)),
		logging.F("score", bestScore.TotalScore),
		logging.F("task_type", string(criteria.TaskType)),
		logging.F("selection_reason", selectionReason),
		logging.F("duration_ms", result.SelectionDuration.Milliseconds()),
	)

	return result, nil
}

// scoreAllModels scores all registered models against the criteria.
func (s *ModelSelector) scoreAllModels(criteria *SelectionCriteria, weights ScoreWeights) []ModelScore {
	// Determine reference values for normalization
	maxLatencyRef := criteria.MaxLatency
	if maxLatencyRef == 0 {
		maxLatencyRef = 5 * time.Second // Default reference
	}

	maxCostRef := criteria.MaxCostPerRequest
	if maxCostRef == 0 {
		maxCostRef = 0.01 // $0.01 per request default reference
	}

	var scores []ModelScore

	for _, model := range s.models {
		score := CalculateScore(model.Capabilities, criteria, weights, maxLatencyRef, maxCostRef)

		// Apply provider preference bonus
		if criteria.PreferredProvider != "" && model.Provider == criteria.PreferredProvider {
			score.TotalScore += 0.1
		}

		// Only include models with positive scores (no severe penalties)
		if score.TotalScore > 0 {
			scores = append(scores, score)
		}
	}

	return scores
}

// buildResult creates a SelectionResult for a directly selected model.
func (s *ModelSelector) buildResult(model *ModelConfig, criteria *SelectionCriteria, startTime time.Time, reason string) *SelectionResult {
	weights := GetWeightsForMode(criteria.OptimizationMode)
	maxLatencyRef := criteria.MaxLatency
	if maxLatencyRef == 0 {
		maxLatencyRef = 5 * time.Second
	}
	maxCostRef := criteria.MaxCostPerRequest
	if maxCostRef == 0 {
		maxCostRef = 0.01
	}

	score := CalculateScore(model.Capabilities, criteria, weights, maxLatencyRef, maxCostRef)

	return &SelectionResult{
		SelectedModel:     model,
		Score:             score,
		Alternatives:      nil,
		SelectionReason:   reason,
		SelectionDuration: time.Since(startTime),
	}
}

// recordMetrics records selection metrics.
func (s *ModelSelector) recordMetrics(criteria *SelectionCriteria, result *SelectionResult, outcome string) {
	if s.metrics == nil {
		return
	}

	s.metrics.SelectionTotal.WithLabelValues(
		string(criteria.TaskType),
		string(criteria.OptimizationMode),
		outcome,
	).Inc()

	s.metrics.SelectionDuration.Observe(result.SelectionDuration.Seconds())

	if result.SelectedModel != nil {
		s.metrics.ModelSelectCount.WithLabelValues(
			result.SelectedModel.ModelID,
			string(result.SelectedModel.Provider),
			string(criteria.TaskType),
		).Inc()
	}
}

// GetModelsForTask returns all models that support the given task type.
func (s *ModelSelector) GetModelsForTask(taskType TaskType) []*ModelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var models []*ModelConfig
	for _, m := range s.models {
		if m.Capabilities.SupportsTask(taskType) {
			models = append(models, m)
		}
	}

	return models
}

// GetLocalModels returns all registered local models.
func (s *ModelSelector) GetLocalModels() []*ModelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var models []*ModelConfig
	for _, m := range s.models {
		if m.Capabilities.IsLocal {
			models = append(models, m)
		}
	}

	return models
}

// GetCloudModels returns all registered cloud models.
func (s *ModelSelector) GetCloudModels() []*ModelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var models []*ModelConfig
	for _, m := range s.models {
		if !m.Capabilities.IsLocal {
			models = append(models, m)
		}
	}

	return models
}

// UpdateModelAvailability updates the availability status of a model.
func (s *ModelSelector) UpdateModelAvailability(modelID string, available bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	model, exists := s.models[modelID]
	if !exists {
		return ErrModelNotFound
	}

	model.Capabilities.IsAvailable = available

	s.logger.Info("model availability updated",
		logging.F("model_id", modelID),
		logging.F("available", available),
	)

	return nil
}

// WarmupManager handles model warmup for frequently used models.
type WarmupManager struct {
	selector *ModelSelector
	logger   logging.Logger
	mu       sync.Mutex

	// warmupInProgress tracks models currently being warmed up.
	warmupInProgress map[string]bool

	// usageCount tracks recent usage for warmup decisions.
	usageCount map[string]int

	// usageWindow is the time window for usage counting.
	usageWindow time.Duration
}

// NewWarmupManager creates a new warmup manager.
func NewWarmupManager(selector *ModelSelector, logger logging.Logger) *WarmupManager {
	return &WarmupManager{
		selector:         selector,
		logger:           logger.With(logging.F("component", "warmup_manager")),
		warmupInProgress: make(map[string]bool),
		usageCount:       make(map[string]int),
		usageWindow:      10 * time.Minute,
	}
}

// ConsiderWarmup checks if a model should be warmed up based on usage patterns.
func (w *WarmupManager) ConsiderWarmup(model *ModelConfig) {
	if model == nil || !model.Capabilities.WarmupRequired {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Increment usage count
	w.usageCount[model.ModelID]++

	// Trigger warmup if model is used frequently
	if w.usageCount[model.ModelID] >= 3 && !w.warmupInProgress[model.ModelID] {
		if model.Capabilities.NeedsWarmup(w.selector.warmupThreshold) {
			w.warmupInProgress[model.ModelID] = true

			go w.warmupModel(model)
		}
	}
}

// warmupModel performs the actual warmup operation.
func (w *WarmupManager) warmupModel(model *ModelConfig) {
	defer func() {
		w.mu.Lock()
		delete(w.warmupInProgress, model.ModelID)
		w.mu.Unlock()
	}()

	w.logger.Info("warming up model", logging.F("model_id", model.ModelID))

	if w.selector.metrics != nil {
		w.selector.metrics.WarmupTriggered.WithLabelValues(model.ModelID).Inc()
	}

	// In a real implementation, this would send a lightweight request to the model
	// to ensure it's loaded and ready. For now, we just mark it as used.
	model.Capabilities.LastUsed = time.Now()

	w.logger.Info("model warmup complete", logging.F("model_id", model.ModelID))
}

// ResetUsageCount resets the usage count for all models.
// Should be called periodically to maintain accurate frequency data.
func (w *WarmupManager) ResetUsageCount() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.usageCount = make(map[string]int)
}

// RegisterDefaultModels registers the default Penfold model configurations.
// This includes MLX local models (via vllm-mlx) and Gemini cloud models.
func RegisterDefaultModels(selector *ModelSelector, mlxLLMURL, mlxEmbeddingsURL, geminiAPIKey string) error {
	// Local MLX embedding model
	if err := selector.RegisterModel(&ModelConfig{
		ModelID:  "mxbai-embed-large-v1",
		Provider: ModelProviderMLX,
		Endpoint: mlxEmbeddingsURL,
		Capabilities: &ModelCapabilities{
			ModelID:             "mxbai-embed-large-v1",
			Provider:            ModelProviderMLX,
			IsLocal:             true,
			SupportedTasks:      []TaskType{TaskTypeEmbedding},
			ContextWindow:       8192,
			EmbeddingDimensions: 1024,
			AvgLatency:          100 * time.Millisecond,
			CostPer1KTokens:     0,
			QualityScore:        0.80,
			SpeedScore:          0.85,
			IsAvailable:         true,
			WarmupRequired:      true,
		},
	}); err != nil {
		return err
	}

	// Local MLX LLM (Qwen 2.5 32B)
	if err := selector.RegisterModel(&ModelConfig{
		ModelID:  "Qwen2.5-32B-Instruct-4bit",
		Provider: ModelProviderMLX,
		Endpoint: mlxLLMURL,
		Capabilities: &ModelCapabilities{
			ModelID:         "Qwen2.5-32B-Instruct-4bit",
			Provider:        ModelProviderMLX,
			IsLocal:         true,
			SupportedTasks:  []TaskType{TaskTypeSummarization, TaskTypeExtraction, TaskTypeClassification},
			ContextWindow:   32768,
			AvgLatency:      500 * time.Millisecond,
			CostPer1KTokens: 0,
			QualityScore:    0.80,
			SpeedScore:      0.75,
			IsAvailable:     true,
			WarmupRequired:  true,
		},
	}); err != nil {
		return err
	}

	// Gemini cloud embedding model
	if geminiAPIKey != "" {
		if err := selector.RegisterModel(&ModelConfig{
			ModelID:  "text-embedding-004",
			Provider: ModelProviderGemini,
			Endpoint: "https://generativelanguage.googleapis.com/v1beta",
			APIKey:   geminiAPIKey,
			Capabilities: &ModelCapabilities{
				ModelID:             "text-embedding-004",
				Provider:            ModelProviderGemini,
				IsLocal:             false,
				SupportedTasks:      []TaskType{TaskTypeEmbedding},
				ContextWindow:       2048,
				EmbeddingDimensions: 768,
				AvgLatency:          200 * time.Millisecond,
				CostPer1KTokens:     0.00001,
				QualityScore:        0.85,
				SpeedScore:          0.70,
				IsAvailable:         true,
				WarmupRequired:      false,
			},
		}); err != nil {
			return err
		}

		// Gemini Pro for complex tasks
		if err := selector.RegisterModel(&ModelConfig{
			ModelID:  "gemini-1.5-pro",
			Provider: ModelProviderGemini,
			Endpoint: "https://generativelanguage.googleapis.com/v1beta",
			APIKey:   geminiAPIKey,
			Capabilities: &ModelCapabilities{
				ModelID:         "gemini-1.5-pro",
				Provider:        ModelProviderGemini,
				IsLocal:         false,
				SupportedTasks:  []TaskType{TaskTypeSummarization, TaskTypeExtraction, TaskTypeClassification},
				ContextWindow:   1000000,
				AvgLatency:      1 * time.Second,
				CostPer1KTokens: 0.00125,
				QualityScore:    0.92,
				SpeedScore:      0.60,
				IsAvailable:     true,
				WarmupRequired:  false,
			},
		}); err != nil {
			return err
		}

		// Gemini Flash for faster cloud tasks
		if err := selector.RegisterModel(&ModelConfig{
			ModelID:  "gemini-1.5-flash",
			Provider: ModelProviderGemini,
			Endpoint: "https://generativelanguage.googleapis.com/v1beta",
			APIKey:   geminiAPIKey,
			Capabilities: &ModelCapabilities{
				ModelID:         "gemini-1.5-flash",
				Provider:        ModelProviderGemini,
				IsLocal:         false,
				SupportedTasks:  []TaskType{TaskTypeSummarization, TaskTypeExtraction, TaskTypeClassification},
				ContextWindow:   1000000,
				AvgLatency:      300 * time.Millisecond,
				CostPer1KTokens: 0.000075,
				QualityScore:    0.85,
				SpeedScore:      0.85,
				IsAvailable:     true,
				WarmupRequired:  false,
			},
		}); err != nil {
			return err
		}
	}

	return nil
}
