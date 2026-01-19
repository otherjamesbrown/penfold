// Package summarization provides content summarization with multiple strategies.
// It supports extractive (key sentence extraction), abstractive (AI-generated),
// hybrid (combined), and hierarchical (multi-level) summarization approaches.
package summarization

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Common errors for the summarization package.
var (
	ErrEmptyContent       = errors.New("content is empty")
	ErrStrategyNotFound   = errors.New("strategy not found")
	ErrAIClientRequired   = errors.New("AI client required for abstractive summarization")
	ErrSummarizationFailed = errors.New("summarization failed")
	ErrContentTooShort    = errors.New("content too short to summarize")
	ErrCacheNotConfigured = errors.New("cache not configured")
)

// StrategyType identifies the summarization strategy.
type StrategyType string

const (
	// StrategyTypeExtractive uses key sentence extraction (TextRank-style).
	StrategyTypeExtractive StrategyType = "extractive"

	// StrategyTypeAbstractive uses AI-generated summaries.
	StrategyTypeAbstractive StrategyType = "abstractive"

	// StrategyTypeHybrid combines extractive and abstractive approaches.
	StrategyTypeHybrid StrategyType = "hybrid"

	// StrategyTypeHierarchical generates multi-level summaries.
	StrategyTypeHierarchical StrategyType = "hierarchical"
)

// SummaryLength defines the target length for summaries.
type SummaryLength string

const (
	// SummaryLengthOneLine produces a single sentence summary (~20 words).
	SummaryLengthOneLine SummaryLength = "one_line"

	// SummaryLengthParagraph produces a paragraph summary (~50-100 words).
	SummaryLengthParagraph SummaryLength = "paragraph"

	// SummaryLengthFull produces a detailed summary (~200-300 words).
	SummaryLengthFull SummaryLength = "full"
)

// SummaryFormat defines the output format of summaries.
type SummaryFormat string

const (
	// SummaryFormatProse generates narrative text.
	SummaryFormatProse SummaryFormat = "prose"

	// SummaryFormatBulletPoints generates bullet point lists.
	SummaryFormatBulletPoints SummaryFormat = "bullet_points"

	// SummaryFormatKeyPoints extracts key facts and points.
	SummaryFormatKeyPoints SummaryFormat = "key_points"

	// SummaryFormatExecutive generates executive summary format.
	SummaryFormatExecutive SummaryFormat = "executive"
)

// SummaryRequest contains parameters for a summarization request.
type SummaryRequest struct {
	// Content is the text to summarize (required).
	Content string

	// Title is optional context for the content.
	Title string

	// Strategy specifies which summarization approach to use.
	// Defaults to StrategyTypeHybrid if not specified.
	Strategy StrategyType

	// Length is the target summary length.
	// Defaults to SummaryLengthParagraph if not specified.
	Length SummaryLength

	// Format is the output format.
	// Defaults to SummaryFormatProse if not specified.
	Format SummaryFormat

	// MaxTokens limits the output length (takes precedence over Length).
	// If zero, Length is used to determine target tokens.
	MaxTokens int

	// PreserveEntities indicates whether to preserve named entities.
	PreserveEntities bool

	// PreserveFacts indicates whether to preserve factual statements.
	PreserveFacts bool

	// TenantID for multi-tenant environments.
	TenantID string

	// ModelPreference allows specifying a preferred AI model.
	ModelPreference string

	// UseCache enables caching of results (defaults to true).
	UseCache bool
}

// SetDefaults applies default values to the request.
func (r *SummaryRequest) SetDefaults() {
	if r.Strategy == "" {
		r.Strategy = StrategyTypeHybrid
	}
	if r.Length == "" {
		r.Length = SummaryLengthParagraph
	}
	if r.Format == "" {
		r.Format = SummaryFormatProse
	}
}

// Validate checks if the request is valid.
func (r *SummaryRequest) Validate() error {
	if r.Content == "" {
		return ErrEmptyContent
	}
	return nil
}

// TargetTokens returns the target number of tokens based on Length.
func (r *SummaryRequest) TargetTokens() int {
	if r.MaxTokens > 0 {
		return r.MaxTokens
	}
	switch r.Length {
	case SummaryLengthOneLine:
		return 30
	case SummaryLengthParagraph:
		return 100
	case SummaryLengthFull:
		return 300
	default:
		return 100
	}
}

// SummaryResult contains the output of a summarization operation.
type SummaryResult struct {
	// Summary is the generated summary text.
	Summary string

	// KeyPoints are extracted key points (if requested).
	KeyPoints []string

	// Entities are preserved named entities (if requested).
	Entities []Entity

	// Facts are preserved factual statements (if requested).
	Facts []string

	// Strategy is the strategy that was used.
	Strategy StrategyType

	// ModelUsed is the AI model used (for abstractive/hybrid).
	ModelUsed string

	// ProcessingTime is how long summarization took.
	ProcessingTime time.Duration

	// InputTokens is the estimated input token count.
	InputTokens int

	// OutputTokens is the output token count.
	OutputTokens int

	// CompressionRatio is original length / summary length.
	CompressionRatio float64

	// Cached indicates if the result was from cache.
	Cached bool

	// Confidence is a score from 0-1 indicating summary quality.
	Confidence float64
}

// Entity represents a named entity preserved from the original content.
type Entity struct {
	// Type is the entity type (person, organization, location, etc.).
	Type string

	// Value is the entity text.
	Value string

	// MentionCount is how many times it appears in the original.
	MentionCount int
}

// HierarchicalSummary contains multi-level summaries.
type HierarchicalSummary struct {
	// OneLine is the shortest summary (~1 sentence).
	OneLine string

	// Paragraph is a medium-length summary (~1 paragraph).
	Paragraph string

	// Full is the detailed summary.
	Full string

	// KeyPoints are extracted across all levels.
	KeyPoints []string

	// ModelUsed for generation.
	ModelUsed string

	// ProcessingTime total time for all levels.
	ProcessingTime time.Duration
}

// Strategy defines the interface for summarization strategies.
type Strategy interface {
	// Name returns the strategy name.
	Name() string

	// Type returns the strategy type.
	Type() StrategyType

	// Summarize generates a summary for the given request.
	Summarize(ctx context.Context, req *SummaryRequest) (*SummaryResult, error)

	// SupportsBatch indicates if the strategy supports batch processing.
	SupportsBatch() bool

	// SummarizeBatch processes multiple requests (if supported).
	SummarizeBatch(ctx context.Context, reqs []*SummaryRequest) ([]*SummaryResult, error)
}

// AIClient defines the interface for AI model interactions.
type AIClient interface {
	// GenerateCompletion generates a text completion.
	GenerateCompletion(ctx context.Context, prompt string, maxTokens int) (string, error)

	// GenerateCompletionWithModel uses a specific model.
	GenerateCompletionWithModel(ctx context.Context, prompt string, maxTokens int, model string) (string, error)

	// GetDefaultModel returns the default model ID.
	GetDefaultModel() string

	// EstimateTokens estimates token count for text.
	EstimateTokens(text string) int
}

// SummarizerMetrics holds Prometheus metrics for the summarizer.
type SummarizerMetrics struct {
	// SummarizationDuration tracks time per strategy.
	SummarizationDuration *prometheus.HistogramVec

	// SummarizationTotal counts summarizations by strategy and status.
	SummarizationTotal *prometheus.CounterVec

	// CacheHits tracks cache hit rate.
	CacheHits prometheus.Counter

	// CacheMisses tracks cache misses.
	CacheMisses prometheus.Counter

	// InputTokensTotal tracks total input tokens processed.
	InputTokensTotal prometheus.Counter

	// OutputTokensTotal tracks total output tokens generated.
	OutputTokensTotal prometheus.Counter

	// CompressionRatio tracks average compression ratio.
	CompressionRatio prometheus.Histogram
}

// NewSummarizerMetrics creates metrics for the summarizer.
func NewSummarizerMetrics(namespace string) *SummarizerMetrics {
	return &SummarizerMetrics{
		SummarizationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "summarization_duration_seconds",
				Help:      "Duration of summarization operations",
				Buckets:   []float64{.1, .25, .5, 1, 2.5, 5, 10, 30},
			},
			[]string{"strategy", "length"},
		),
		SummarizationTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "summarization_total",
				Help:      "Total number of summarization operations",
			},
			[]string{"strategy", "status"},
		),
		CacheHits: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "summarization_cache_hits_total",
				Help:      "Total number of cache hits",
			},
		),
		CacheMisses: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "summarization_cache_misses_total",
				Help:      "Total number of cache misses",
			},
		),
		InputTokensTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "summarization_input_tokens_total",
				Help:      "Total input tokens processed",
			},
		),
		OutputTokensTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "summarization_output_tokens_total",
				Help:      "Total output tokens generated",
			},
		),
		CompressionRatio: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "summarization_compression_ratio",
				Help:      "Compression ratio of summaries",
				Buckets:   []float64{2, 5, 10, 20, 50, 100},
			},
		),
	}
}

// RegisterMetrics registers all summarizer metrics with Prometheus.
func (m *SummarizerMetrics) RegisterMetrics() error {
	collectors := []prometheus.Collector{
		m.SummarizationDuration,
		m.SummarizationTotal,
		m.CacheHits,
		m.CacheMisses,
		m.InputTokensTotal,
		m.OutputTokensTotal,
		m.CompressionRatio,
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

// Config holds configuration for the Summarizer.
type Config struct {
	// DefaultStrategy is used when not specified in request.
	DefaultStrategy StrategyType

	// EnableCache enables result caching.
	EnableCache bool

	// CacheTTL is the default cache TTL.
	CacheTTL time.Duration

	// MaxContentLength is the maximum content length to process.
	MaxContentLength int

	// MinContentLength is the minimum content length for summarization.
	MinContentLength int

	// DefaultModel is the default AI model to use.
	DefaultModel string

	// EnableMetrics enables Prometheus metrics.
	EnableMetrics bool

	// MetricsNamespace is the Prometheus namespace.
	MetricsNamespace string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DefaultStrategy:  StrategyTypeHybrid,
		EnableCache:      true,
		CacheTTL:         1 * time.Hour,
		MaxContentLength: 100000, // ~25K tokens
		MinContentLength: 100,    // Minimum 100 chars
		DefaultModel:     "llama3.2",
		EnableMetrics:    true,
		MetricsNamespace: "penfold_content",
	}
}

// Summarizer orchestrates content summarization using configurable strategies.
type Summarizer struct {
	strategies map[StrategyType]Strategy
	config     *Config
	logger     logging.Logger
	metrics    *SummarizerMetrics
	cache      SummaryCache
	aiClient   AIClient
	mu         sync.RWMutex
}

// NewSummarizer creates a new Summarizer with the given configuration.
func NewSummarizer(cfg *Config, logger logging.Logger, aiClient AIClient) *Summarizer {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	s := &Summarizer{
		strategies: make(map[StrategyType]Strategy),
		config:     cfg,
		logger:     logger.With(logging.F("component", "summarizer")),
		aiClient:   aiClient,
	}

	if cfg.EnableMetrics {
		s.metrics = NewSummarizerMetrics(cfg.MetricsNamespace)
	}

	// Register default strategies
	s.RegisterStrategy(NewExtractiveStrategy(logger))
	if aiClient != nil {
		s.RegisterStrategy(NewAbstractiveStrategy(aiClient, logger))
		s.RegisterStrategy(NewHybridStrategy(aiClient, logger))
		s.RegisterStrategy(NewHierarchicalStrategy(aiClient, logger))
	}

	return s
}

// RegisterStrategy adds a strategy to the summarizer.
func (s *Summarizer) RegisterStrategy(strategy Strategy) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.strategies[strategy.Type()] = strategy

	s.logger.Debug("Registered summarization strategy",
		logging.F("strategy_name", strategy.Name()),
		logging.F("strategy_type", string(strategy.Type())),
	)
}

// SetCache configures the summary cache.
func (s *Summarizer) SetCache(cache SummaryCache) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = cache
}

// Summarize generates a summary for the given request.
func (s *Summarizer) Summarize(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
	start := time.Now()

	// Apply defaults and validate
	req.SetDefaults()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Check content length constraints
	if len(req.Content) < s.config.MinContentLength {
		return nil, fmt.Errorf("%w: minimum length is %d characters", ErrContentTooShort, s.config.MinContentLength)
	}
	if len(req.Content) > s.config.MaxContentLength {
		s.logger.Warn("Content exceeds max length, truncating",
			logging.F("original_length", len(req.Content)),
			logging.F("max_length", s.config.MaxContentLength),
		)
		req.Content = req.Content[:s.config.MaxContentLength]
	}

	// Check cache first
	if s.config.EnableCache && req.UseCache && s.cache != nil {
		if result := s.checkCache(ctx, req); result != nil {
			if s.metrics != nil {
				s.metrics.CacheHits.Inc()
			}
			return result, nil
		}
		if s.metrics != nil {
			s.metrics.CacheMisses.Inc()
		}
	}

	// Get the appropriate strategy
	strategy, err := s.getStrategy(req.Strategy)
	if err != nil {
		return nil, err
	}

	s.logger.Debug("Starting summarization",
		logging.F("strategy", string(req.Strategy)),
		logging.F("length", string(req.Length)),
		logging.F("content_length", len(req.Content)),
	)

	// Execute summarization
	result, err := strategy.Summarize(ctx, req)
	if err != nil {
		if s.metrics != nil {
			s.metrics.SummarizationTotal.WithLabelValues(string(req.Strategy), "error").Inc()
		}
		return nil, fmt.Errorf("%w: %v", ErrSummarizationFailed, err)
	}

	result.ProcessingTime = time.Since(start)
	result.Strategy = req.Strategy

	// Calculate compression ratio
	if len(result.Summary) > 0 {
		result.CompressionRatio = float64(len(req.Content)) / float64(len(result.Summary))
	}

	// Store in cache
	if s.config.EnableCache && req.UseCache && s.cache != nil {
		s.storeInCache(ctx, req, result)
	}

	// Record metrics
	if s.metrics != nil {
		s.metrics.SummarizationDuration.WithLabelValues(string(req.Strategy), string(req.Length)).Observe(result.ProcessingTime.Seconds())
		s.metrics.SummarizationTotal.WithLabelValues(string(req.Strategy), "success").Inc()
		s.metrics.InputTokensTotal.Add(float64(result.InputTokens))
		s.metrics.OutputTokensTotal.Add(float64(result.OutputTokens))
		s.metrics.CompressionRatio.Observe(result.CompressionRatio)
	}

	s.logger.Info("Summarization completed",
		logging.F("strategy", string(req.Strategy)),
		logging.F("processing_time_ms", result.ProcessingTime.Milliseconds()),
		logging.F("compression_ratio", result.CompressionRatio),
		logging.F("model_used", result.ModelUsed),
	)

	return result, nil
}

// SummarizeBatch processes multiple summarization requests.
// Strategies that support batching will be used for efficiency.
func (s *Summarizer) SummarizeBatch(ctx context.Context, reqs []*SummaryRequest) ([]*SummaryResult, error) {
	if len(reqs) == 0 {
		return nil, nil
	}

	// Group by strategy
	byStrategy := make(map[StrategyType][]*SummaryRequest)
	for _, req := range reqs {
		req.SetDefaults()
		byStrategy[req.Strategy] = append(byStrategy[req.Strategy], req)
	}

	// Process each group
	results := make([]*SummaryResult, len(reqs))
	reqIndexMap := make(map[*SummaryRequest]int)
	for i, req := range reqs {
		reqIndexMap[req] = i
	}

	for strategyType, strategyReqs := range byStrategy {
		strategy, err := s.getStrategy(strategyType)
		if err != nil {
			// Fall back to individual processing
			for _, req := range strategyReqs {
				result, err := s.Summarize(ctx, req)
				if err != nil {
					return nil, err
				}
				results[reqIndexMap[req]] = result
			}
			continue
		}

		if strategy.SupportsBatch() {
			batchResults, err := strategy.SummarizeBatch(ctx, strategyReqs)
			if err != nil {
				return nil, err
			}
			for i, req := range strategyReqs {
				results[reqIndexMap[req]] = batchResults[i]
			}
		} else {
			for _, req := range strategyReqs {
				result, err := s.Summarize(ctx, req)
				if err != nil {
					return nil, err
				}
				results[reqIndexMap[req]] = result
			}
		}
	}

	return results, nil
}

// SummarizeHierarchical generates multi-level summaries.
func (s *Summarizer) SummarizeHierarchical(ctx context.Context, content string, tenantID string) (*HierarchicalSummary, error) {
	start := time.Now()

	if content == "" {
		return nil, ErrEmptyContent
	}

	strategy, err := s.getStrategy(StrategyTypeHierarchical)
	if err != nil {
		return nil, err
	}

	hs, ok := strategy.(*HierarchicalStrategy)
	if !ok {
		return nil, errors.New("hierarchical strategy not properly configured")
	}

	result, err := hs.SummarizeHierarchical(ctx, content, tenantID)
	if err != nil {
		return nil, err
	}

	result.ProcessingTime = time.Since(start)
	return result, nil
}

// getStrategy returns the strategy for the given type.
func (s *Summarizer) getStrategy(strategyType StrategyType) (Strategy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	strategy, exists := s.strategies[strategyType]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrStrategyNotFound, strategyType)
	}
	return strategy, nil
}

// checkCache attempts to retrieve a cached result.
func (s *Summarizer) checkCache(ctx context.Context, req *SummaryRequest) *SummaryResult {
	if s.cache == nil {
		return nil
	}

	result, err := s.cache.Get(ctx, req)
	if err != nil {
		s.logger.Debug("Cache miss",
			logging.F("strategy", string(req.Strategy)),
			logging.Err(err),
		)
		return nil
	}

	if result != nil {
		result.Cached = true
		s.logger.Debug("Cache hit",
			logging.F("strategy", string(req.Strategy)),
		)
	}

	return result
}

// storeInCache stores a result in the cache.
func (s *Summarizer) storeInCache(ctx context.Context, req *SummaryRequest, result *SummaryResult) {
	if s.cache == nil {
		return
	}

	if err := s.cache.Set(ctx, req, result, s.config.CacheTTL); err != nil {
		s.logger.Warn("Failed to cache summary result",
			logging.Err(err),
		)
	}
}

// GetStrategies returns all registered strategies.
func (s *Summarizer) GetStrategies() []Strategy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	strategies := make([]Strategy, 0, len(s.strategies))
	for _, strategy := range s.strategies {
		strategies = append(strategies, strategy)
	}
	return strategies
}

// GetConfig returns the summarizer configuration.
func (s *Summarizer) GetConfig() *Config {
	return s.config
}
