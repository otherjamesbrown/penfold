package ensemble

import (
	"time"
)

// EnsembleConfig defines the complete configuration for an ensemble.
type EnsembleConfig struct {
	// Name is the identifier for this ensemble configuration.
	Name string

	// Description describes the purpose of this ensemble.
	Description string

	// Models lists the model IDs to include in this ensemble.
	Models []string

	// Strategy is the ensemble strategy to use.
	Strategy StrategyType

	// Timeout is the maximum time for the ensemble operation.
	Timeout time.Duration

	// MinResponses is the minimum number of successful responses required.
	MinResponses int

	// ConsensusThreshold is the minimum agreement ratio for consensus.
	ConsensusThreshold float64

	// RequireConsensus fails the request if consensus is not reached.
	RequireConsensus bool

	// MaxCost is the maximum acceptable total cost.
	MaxCost float64

	// StrategyOptions contains strategy-specific configuration.
	StrategyOptions map[string]interface{}

	// Fallback is the strategy to use if the primary strategy fails.
	Fallback StrategyType

	// Priority determines the order when selecting ensembles.
	Priority int

	// Tags are labels for filtering and organization.
	Tags []string

	// Metadata contains additional configuration data.
	Metadata map[string]interface{}
}

// DefaultEnsembleConfig returns a configuration with sensible defaults.
func DefaultEnsembleConfig() *EnsembleConfig {
	return &EnsembleConfig{
		Strategy:           StrategyParallel,
		Timeout:            30 * time.Second,
		MinResponses:       1,
		ConsensusThreshold: 0.7,
		RequireConsensus:   false,
		StrategyOptions:    make(map[string]interface{}),
		Metadata:           make(map[string]interface{}),
	}
}

// Validate checks if the ensemble configuration is valid.
func (c *EnsembleConfig) Validate() error {
	if c.Name == "" {
		return ErrInvalidRequest
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.MinResponses <= 0 {
		c.MinResponses = 1
	}
	if c.ConsensusThreshold <= 0 || c.ConsensusThreshold > 1 {
		c.ConsensusThreshold = 0.7
	}
	return nil
}

// Clone creates a deep copy of the configuration.
func (c *EnsembleConfig) Clone() *EnsembleConfig {
	clone := *c
	clone.Models = make([]string, len(c.Models))
	copy(clone.Models, c.Models)
	clone.Tags = make([]string, len(c.Tags))
	copy(clone.Tags, c.Tags)
	clone.StrategyOptions = make(map[string]interface{})
	for k, v := range c.StrategyOptions {
		clone.StrategyOptions[k] = v
	}
	clone.Metadata = make(map[string]interface{})
	for k, v := range c.Metadata {
		clone.Metadata[k] = v
	}
	return &clone
}

// TaskEnsembleConfigs provides pre-configured ensembles for common tasks.
var TaskEnsembleConfigs = map[RequestType]*EnsembleConfig{
	RequestTypeClassification: {
		Name:               "classification-ensemble",
		Description:        "Optimized for classification tasks with high consensus",
		Strategy:           StrategyMajorityVoting,
		Timeout:            15 * time.Second,
		MinResponses:       2,
		ConsensusThreshold: 0.8,
		RequireConsensus:   true,
		StrategyOptions: map[string]interface{}{
			"require_unanimous": false,
		},
		Tags: []string{"classification", "high-consensus"},
	},
	RequestTypeSummary: {
		Name:               "summary-ensemble",
		Description:        "Optimized for summarization with quality weighting",
		Strategy:           StrategyWeightedVoting,
		Timeout:            30 * time.Second,
		MinResponses:       2,
		ConsensusThreshold: 0.6,
		RequireConsensus:   false,
		StrategyOptions: map[string]interface{}{
			"prefer_detailed": true,
		},
		Tags: []string{"summarization", "quality-weighted"},
	},
	RequestTypeExtraction: {
		Name:               "extraction-ensemble",
		Description:        "Optimized for information extraction with verification",
		Strategy:           StrategyParallel,
		Timeout:            45 * time.Second,
		MinResponses:       2,
		ConsensusThreshold: 0.7,
		RequireConsensus:   false,
		StrategyOptions: map[string]interface{}{
			"merge_extractions": true,
		},
		Tags: []string{"extraction", "verification"},
	},
	RequestTypeEmbedding: {
		Name:               "embedding-ensemble",
		Description:        "Cascade for embeddings - fast local, fallback to cloud",
		Strategy:           StrategyCascade,
		Timeout:            10 * time.Second,
		MinResponses:       1,
		ConsensusThreshold: 0.9,
		RequireConsensus:   false,
		StrategyOptions: map[string]interface{}{
			"prefer_local":   true,
			"min_confidence": 0.8,
		},
		Tags: []string{"embedding", "cascade"},
	},
	RequestTypeChat: {
		Name:               "chat-ensemble",
		Description:        "Interactive chat with debate for complex questions",
		Strategy:           StrategyDebate,
		Timeout:            60 * time.Second,
		MinResponses:       2,
		ConsensusThreshold: 0.6,
		RequireConsensus:   false,
		StrategyOptions: map[string]interface{}{
			"max_rounds": 2,
		},
		Fallback: StrategyCascade,
		Tags:     []string{"chat", "debate"},
	},
}

// GetTaskEnsembleConfig returns the default ensemble config for a task type.
func GetTaskEnsembleConfig(taskType RequestType) *EnsembleConfig {
	if cfg, ok := TaskEnsembleConfigs[taskType]; ok {
		return cfg.Clone()
	}
	// Return default config if no specific config exists
	return DefaultEnsembleConfig()
}

// ModelSelectionConfig configures how models are selected for ensembles.
type ModelSelectionConfig struct {
	// PreferLocal prioritizes local models over cloud.
	PreferLocal bool

	// MaxModels is the maximum number of models to include.
	MaxModels int

	// MinModels is the minimum number of models required.
	MinModels int

	// RequiredCapabilities are capabilities all models must have.
	RequiredCapabilities []string

	// ExcludeProviders lists providers to exclude.
	ExcludeProviders []string

	// IncludeProviders limits to specific providers.
	IncludeProviders []string

	// MaxCostPerModel is the maximum cost per model.
	MaxCostPerModel float64

	// MinQualityScore is the minimum quality score required.
	MinQualityScore float64

	// SortBy determines the model ordering.
	SortBy ModelSortCriteria
}

// ModelSortCriteria defines how models are sorted.
type ModelSortCriteria string

const (
	// SortByQuality sorts by quality score (highest first).
	SortByQuality ModelSortCriteria = "quality"

	// SortByCost sorts by cost (lowest first).
	SortByCost ModelSortCriteria = "cost"

	// SortBySpeed sorts by latency (lowest first).
	SortBySpeed ModelSortCriteria = "speed"

	// SortByLocal sorts local models first.
	SortByLocal ModelSortCriteria = "local"
)

// DefaultModelSelectionConfig returns default model selection settings.
func DefaultModelSelectionConfig() *ModelSelectionConfig {
	return &ModelSelectionConfig{
		PreferLocal:     true,
		MaxModels:       5,
		MinModels:       1,
		MinQualityScore: 0.5,
		SortBy:          SortByQuality,
	}
}

// CostConfig defines cost constraints and tracking.
type CostConfig struct {
	// MaxTotalCost is the maximum total cost for an operation.
	MaxTotalCost float64

	// MaxCostPerRequest is the maximum cost per individual request.
	MaxCostPerRequest float64

	// Budget is the remaining budget for cost tracking.
	Budget float64

	// CostMultipliers apply adjustments to base costs.
	CostMultipliers map[string]float64

	// TrackCosts enables cost tracking.
	TrackCosts bool
}

// DefaultCostConfig returns default cost settings.
func DefaultCostConfig() *CostConfig {
	return &CostConfig{
		MaxTotalCost:      1.0, // $1.00
		MaxCostPerRequest: 0.1, // $0.10
		CostMultipliers:   make(map[string]float64),
		TrackCosts:        true,
	}
}

// RetryConfig configures retry behavior for failed requests.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int

	// RetryDelay is the initial delay between retries.
	RetryDelay time.Duration

	// BackoffMultiplier increases the delay for each retry.
	BackoffMultiplier float64

	// MaxRetryDelay is the maximum delay between retries.
	MaxRetryDelay time.Duration

	// RetryableErrors lists error types that should be retried.
	RetryableErrors []string

	// RetryOnTimeout enables retry on timeout errors.
	RetryOnTimeout bool
}

// DefaultRetryConfig returns default retry settings.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:        2,
		RetryDelay:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
		MaxRetryDelay:     5 * time.Second,
		RetryOnTimeout:    true,
		RetryableErrors:   []string{"timeout", "connection_refused", "rate_limited"},
	}
}

// TimeoutConfig configures various timeout settings.
type TimeoutConfig struct {
	// RequestTimeout is the timeout for individual requests.
	RequestTimeout time.Duration

	// EnsembleTimeout is the total timeout for the ensemble operation.
	EnsembleTimeout time.Duration

	// ModelTimeout is the timeout for individual model calls.
	ModelTimeout time.Duration

	// AggregationTimeout is the timeout for result aggregation.
	AggregationTimeout time.Duration
}

// DefaultTimeoutConfig returns default timeout settings.
func DefaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		RequestTimeout:     30 * time.Second,
		EnsembleTimeout:    60 * time.Second,
		ModelTimeout:       20 * time.Second,
		AggregationTimeout: 5 * time.Second,
	}
}

// DebugConfig configures debug and tracing options.
type DebugConfig struct {
	// EnableTracing enables distributed tracing.
	EnableTracing bool

	// EnableDetailedLogs enables verbose logging.
	EnableDetailedLogs bool

	// LogResponses logs full model responses.
	LogResponses bool

	// LogTimings logs detailed timing information.
	LogTimings bool

	// RecordHistory records request/response history.
	RecordHistory bool

	// MaxHistorySize limits the history buffer size.
	MaxHistorySize int
}

// DefaultDebugConfig returns default debug settings.
func DefaultDebugConfig() *DebugConfig {
	return &DebugConfig{
		EnableTracing:      false,
		EnableDetailedLogs: false,
		LogResponses:       false,
		LogTimings:         true,
		RecordHistory:      false,
		MaxHistorySize:     100,
	}
}

// ConfigBuilder provides a fluent interface for building ensemble configs.
type ConfigBuilder struct {
	config *EnsembleConfig
}

// NewConfigBuilder creates a new configuration builder.
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		config: DefaultEnsembleConfig(),
	}
}

// WithName sets the ensemble name.
func (b *ConfigBuilder) WithName(name string) *ConfigBuilder {
	b.config.Name = name
	return b
}

// WithDescription sets the description.
func (b *ConfigBuilder) WithDescription(desc string) *ConfigBuilder {
	b.config.Description = desc
	return b
}

// WithModels sets the models to use.
func (b *ConfigBuilder) WithModels(models ...string) *ConfigBuilder {
	b.config.Models = models
	return b
}

// WithStrategy sets the ensemble strategy.
func (b *ConfigBuilder) WithStrategy(strategy StrategyType) *ConfigBuilder {
	b.config.Strategy = strategy
	return b
}

// WithTimeout sets the operation timeout.
func (b *ConfigBuilder) WithTimeout(timeout time.Duration) *ConfigBuilder {
	b.config.Timeout = timeout
	return b
}

// WithMinResponses sets the minimum required responses.
func (b *ConfigBuilder) WithMinResponses(min int) *ConfigBuilder {
	b.config.MinResponses = min
	return b
}

// WithConsensusThreshold sets the consensus threshold.
func (b *ConfigBuilder) WithConsensusThreshold(threshold float64) *ConfigBuilder {
	b.config.ConsensusThreshold = threshold
	return b
}

// WithRequireConsensus enables/disables consensus requirement.
func (b *ConfigBuilder) WithRequireConsensus(require bool) *ConfigBuilder {
	b.config.RequireConsensus = require
	return b
}

// WithMaxCost sets the maximum cost.
func (b *ConfigBuilder) WithMaxCost(cost float64) *ConfigBuilder {
	b.config.MaxCost = cost
	return b
}

// WithOption adds a strategy option.
func (b *ConfigBuilder) WithOption(key string, value interface{}) *ConfigBuilder {
	if b.config.StrategyOptions == nil {
		b.config.StrategyOptions = make(map[string]interface{})
	}
	b.config.StrategyOptions[key] = value
	return b
}

// WithFallback sets the fallback strategy.
func (b *ConfigBuilder) WithFallback(fallback StrategyType) *ConfigBuilder {
	b.config.Fallback = fallback
	return b
}

// WithTags adds tags.
func (b *ConfigBuilder) WithTags(tags ...string) *ConfigBuilder {
	b.config.Tags = append(b.config.Tags, tags...)
	return b
}

// Build returns the constructed configuration.
func (b *ConfigBuilder) Build() *EnsembleConfig {
	return b.config.Clone()
}
