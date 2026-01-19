package categorization

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/prometheus/client_golang/prometheus"
)

// Common errors for categorization.
var (
	ErrNoCategories     = errors.New("no categories available")
	ErrCategorizeFailed = errors.New("categorization failed")
	ErrNilContent       = errors.New("content is nil")
)

// CategoryResult represents a categorization result.
type CategoryResult struct {
	// CategoryID is the matched category.
	CategoryID string `json:"category_id"`

	// CategoryName is the human-readable name.
	CategoryName string `json:"category_name"`

	// CategoryPath is the full path (for hierarchical categories).
	CategoryPath string `json:"category_path,omitempty"`

	// Confidence is the classification confidence (0.0 to 1.0).
	Confidence float64 `json:"confidence"`

	// Source indicates how this category was determined.
	Source string `json:"source"`

	// MatchedTerms are the terms that led to this categorization.
	MatchedTerms []string `json:"matched_terms,omitempty"`

	// Metadata contains additional classification details.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// CategorizationResult contains the complete categorization output.
type CategorizationResult struct {
	// Categories are all matched categories, sorted by confidence.
	Categories []CategoryResult `json:"categories"`

	// Primary is the highest confidence category.
	Primary *CategoryResult `json:"primary,omitempty"`

	// ProcessingTime is how long categorization took.
	ProcessingTime time.Duration `json:"processing_time"`

	// TotalCandidates is the number of categories considered.
	TotalCandidates int `json:"total_candidates"`

	// RulesEvaluated is the number of rules evaluated.
	RulesEvaluated int `json:"rules_evaluated,omitempty"`

	// MLUsed indicates whether ML classification was used.
	MLUsed bool `json:"ml_used"`
}

// Categorizer is the interface for content categorization.
type Categorizer interface {
	// Categorize classifies content into categories.
	Categorize(ctx context.Context, content *ContentData) (*CategorizationResult, error)

	// CategorizeMultiple classifies multiple content items.
	CategorizeMultiple(ctx context.Context, contents []*ContentData) ([]*CategorizationResult, error)

	// SuggestCategories suggests new categories based on content.
	SuggestCategories(ctx context.Context, content *ContentData) ([]string, error)
}

// =============================================================================
// Categorizer Metrics
// =============================================================================

// CategorizerMetrics holds Prometheus metrics for categorization.
type CategorizerMetrics struct {
	CategorizationTotal    *prometheus.CounterVec
	CategorizationDuration prometheus.Histogram
	CategoriesAssigned     *prometheus.HistogramVec
	ConfidenceDistribution *prometheus.HistogramVec
	RulesEvaluated         prometheus.Histogram
	MLFallbacks            *prometheus.CounterVec
}

// NewCategorizerMetrics creates metrics for categorization.
func NewCategorizerMetrics(namespace string) *CategorizerMetrics {
	return &CategorizerMetrics{
		CategorizationTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "categorization_total",
				Help:      "Total categorization operations",
			},
			[]string{"categorizer_type", "status"},
		),
		CategorizationDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "categorization_duration_seconds",
				Help:      "Duration of categorization operations",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
			},
		),
		CategoriesAssigned: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "categories_assigned",
				Help:      "Number of categories assigned per content",
				Buckets:   []float64{0, 1, 2, 3, 5, 10},
			},
			[]string{"categorizer_type"},
		),
		ConfidenceDistribution: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "categorization_confidence",
				Help:      "Confidence distribution of categorizations",
				Buckets:   []float64{.1, .2, .3, .4, .5, .6, .7, .8, .9, 1.0},
			},
			[]string{"categorizer_type"},
		),
		RulesEvaluated: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "rules_evaluated",
				Help:      "Number of rules evaluated per categorization",
				Buckets:   []float64{1, 5, 10, 25, 50, 100},
			},
		),
		MLFallbacks: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "ml_fallbacks_total",
				Help:      "Count of fallbacks to ML classification",
			},
			[]string{"reason"},
		),
	}
}

// RegisterMetrics registers all categorizer metrics.
func (m *CategorizerMetrics) RegisterMetrics(reg *prometheus.Registry) error {
	collectors := []prometheus.Collector{
		m.CategorizationTotal,
		m.CategorizationDuration,
		m.CategoriesAssigned,
		m.ConfidenceDistribution,
		m.RulesEvaluated,
		m.MLFallbacks,
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

// =============================================================================
// Rule-Based Categorizer
// =============================================================================

// RuleBasedCategorizer classifies content using pattern and keyword rules.
type RuleBasedCategorizer struct {
	taxonomy     *TaxonomyTree
	ruleEngine   *RuleEngine
	logger       logging.Logger
	metrics      *CategorizerMetrics
	minConfidence float64
	maxCategories int
}

// RuleBasedConfig configures the rule-based categorizer.
type RuleBasedConfig struct {
	Taxonomy      *TaxonomyTree
	Logger        logging.Logger
	Metrics       *CategorizerMetrics
	MinConfidence float64
	MaxCategories int
}

// NewRuleBasedCategorizer creates a new rule-based categorizer.
func NewRuleBasedCategorizer(config *RuleBasedConfig) (*RuleBasedCategorizer, error) {
	if config.Taxonomy == nil {
		return nil, ErrEmptyTaxonomy
	}
	if config.MinConfidence == 0 {
		config.MinConfidence = 0.5
	}
	if config.MaxCategories == 0 {
		config.MaxCategories = 5
	}

	categorizer := &RuleBasedCategorizer{
		taxonomy:      config.Taxonomy,
		ruleEngine:    NewRuleEngine(),
		logger:        config.Logger.With(logging.F("categorizer", "rule-based")),
		metrics:       config.Metrics,
		minConfidence: config.MinConfidence,
		maxCategories: config.MaxCategories,
	}

	// Build rules from taxonomy
	rules, err := BuildRulesFromTaxonomy(config.Taxonomy)
	if err != nil {
		return nil, err
	}
	categorizer.ruleEngine.AddRules(rules...)

	return categorizer, nil
}

// AddRule adds a custom rule to the categorizer.
func (c *RuleBasedCategorizer) AddRule(rule Rule) {
	c.ruleEngine.AddRule(rule)
}

// Categorize classifies content using rules.
func (c *RuleBasedCategorizer) Categorize(ctx context.Context, content *ContentData) (*CategorizationResult, error) {
	startTime := time.Now()

	if content == nil {
		return nil, ErrNilContent
	}

	// Evaluate rules
	evalResult := c.ruleEngine.Evaluate(ctx, content, &EvaluateOptions{
		MinConfidence:       c.minConfidence,
		AggregateByCategory: true,
	})

	result := &CategorizationResult{
		Categories:      make([]CategoryResult, 0),
		ProcessingTime:  time.Since(startTime),
		TotalCandidates: c.taxonomy.Size(),
		RulesEvaluated:  evalResult.RulesEvaluated,
		MLUsed:          false,
	}

	// Convert rule matches to category results
	for _, match := range evalResult.Matches {
		cat, err := c.taxonomy.GetCategory(match.CategoryID)
		if err != nil {
			continue
		}

		catPath, _ := c.taxonomy.GetPathString(match.CategoryID, "/")

		catResult := CategoryResult{
			CategoryID:   match.CategoryID,
			CategoryName: cat.Name,
			CategoryPath: catPath,
			Confidence:   match.Confidence,
			Source:       string(match.RuleType),
			MatchedTerms: match.MatchedTerms,
			Metadata:     make(map[string]string),
		}

		result.Categories = append(result.Categories, catResult)
	}

	// Sort by confidence
	sort.Slice(result.Categories, func(i, j int) bool {
		return result.Categories[i].Confidence > result.Categories[j].Confidence
	})

	// Limit to max categories
	if len(result.Categories) > c.maxCategories {
		result.Categories = result.Categories[:c.maxCategories]
	}

	// Set primary
	if len(result.Categories) > 0 {
		result.Primary = &result.Categories[0]
	}

	// Record metrics
	if c.metrics != nil {
		c.metrics.CategorizationTotal.WithLabelValues("rule-based", "success").Inc()
		c.metrics.CategorizationDuration.Observe(result.ProcessingTime.Seconds())
		c.metrics.CategoriesAssigned.WithLabelValues("rule-based").Observe(float64(len(result.Categories)))
		c.metrics.RulesEvaluated.Observe(float64(evalResult.RulesEvaluated))
		if result.Primary != nil {
			c.metrics.ConfidenceDistribution.WithLabelValues("rule-based").Observe(result.Primary.Confidence)
		}
	}

	c.logger.Debug("categorization completed",
		logging.F("categories_found", len(result.Categories)),
		logging.F("rules_evaluated", evalResult.RulesEvaluated),
		logging.F("duration_ms", result.ProcessingTime.Milliseconds()),
	)

	return result, nil
}

// CategorizeMultiple classifies multiple content items.
func (c *RuleBasedCategorizer) CategorizeMultiple(ctx context.Context, contents []*ContentData) ([]*CategorizationResult, error) {
	results := make([]*CategorizationResult, len(contents))

	for i, content := range contents {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result, err := c.Categorize(ctx, content)
		if err != nil {
			results[i] = &CategorizationResult{Categories: []CategoryResult{}}
			continue
		}
		results[i] = result
	}

	return results, nil
}

// SuggestCategories suggests new categories based on content keywords.
func (c *RuleBasedCategorizer) SuggestCategories(ctx context.Context, content *ContentData) ([]string, error) {
	if content == nil {
		return nil, ErrNilContent
	}

	// Return keywords that don't match any existing category
	existingKeywords := make(map[string]bool)
	for _, cat := range c.taxonomy.GetAllCategories() {
		for _, kw := range cat.Keywords {
			existingKeywords[kw] = true
		}
	}

	suggestions := make([]string, 0)
	for _, kw := range content.Keywords {
		if !existingKeywords[kw] {
			suggestions = append(suggestions, kw)
		}
	}

	return suggestions, nil
}

// =============================================================================
// ML Categorizer
// =============================================================================

// MLClassifier is the interface for ML-based classification.
type MLClassifier interface {
	// Classify performs classification on the given text.
	Classify(ctx context.Context, text string, candidateLabels []string) ([]ClassificationScore, error)
}

// ClassificationScore represents an ML classification result.
type ClassificationScore struct {
	Label      string
	Confidence float64
}

// MLCategorizer uses ML models for classification.
type MLCategorizer struct {
	classifier    MLClassifier
	taxonomy      *TaxonomyTree
	logger        logging.Logger
	metrics       *CategorizerMetrics
	minConfidence float64
	maxCategories int
	mu            sync.RWMutex
}

// MLCategorizerConfig configures the ML categorizer.
type MLCategorizerConfig struct {
	Classifier    MLClassifier
	Taxonomy      *TaxonomyTree
	Logger        logging.Logger
	Metrics       *CategorizerMetrics
	MinConfidence float64
	MaxCategories int
}

// NewMLCategorizer creates a new ML-based categorizer.
func NewMLCategorizer(config *MLCategorizerConfig) (*MLCategorizer, error) {
	if config.Classifier == nil {
		return nil, errors.New("ML classifier is required")
	}
	if config.Taxonomy == nil {
		return nil, ErrEmptyTaxonomy
	}
	if config.MinConfidence == 0 {
		config.MinConfidence = 0.3
	}
	if config.MaxCategories == 0 {
		config.MaxCategories = 5
	}

	return &MLCategorizer{
		classifier:    config.Classifier,
		taxonomy:      config.Taxonomy,
		logger:        config.Logger.With(logging.F("categorizer", "ml")),
		metrics:       config.Metrics,
		minConfidence: config.MinConfidence,
		maxCategories: config.MaxCategories,
	}, nil
}

// Categorize classifies content using ML.
func (c *MLCategorizer) Categorize(ctx context.Context, content *ContentData) (*CategorizationResult, error) {
	startTime := time.Now()

	if content == nil {
		return nil, ErrNilContent
	}

	// Get candidate labels from taxonomy
	categories := c.taxonomy.GetEnabledCategories()
	if len(categories) == 0 {
		return nil, ErrNoCategories
	}

	labels := make([]string, len(categories))
	labelToCategory := make(map[string]*Category)
	for i, cat := range categories {
		labels[i] = cat.Name
		labelToCategory[cat.Name] = cat
	}

	// Run classification
	scores, err := c.classifier.Classify(ctx, content.Text, labels)
	if err != nil {
		if c.metrics != nil {
			c.metrics.CategorizationTotal.WithLabelValues("ml", "error").Inc()
		}
		return nil, err
	}

	result := &CategorizationResult{
		Categories:      make([]CategoryResult, 0),
		ProcessingTime:  time.Since(startTime),
		TotalCandidates: len(categories),
		MLUsed:          true,
	}

	// Convert scores to category results
	for _, score := range scores {
		if score.Confidence < c.minConfidence {
			continue
		}

		cat, exists := labelToCategory[score.Label]
		if !exists {
			continue
		}

		catPath, _ := c.taxonomy.GetPathString(cat.ID, "/")

		catResult := CategoryResult{
			CategoryID:   cat.ID,
			CategoryName: cat.Name,
			CategoryPath: catPath,
			Confidence:   score.Confidence,
			Source:       "ml",
			Metadata:     make(map[string]string),
		}

		result.Categories = append(result.Categories, catResult)
	}

	// Sort by confidence
	sort.Slice(result.Categories, func(i, j int) bool {
		return result.Categories[i].Confidence > result.Categories[j].Confidence
	})

	// Limit to max categories
	if len(result.Categories) > c.maxCategories {
		result.Categories = result.Categories[:c.maxCategories]
	}

	// Set primary
	if len(result.Categories) > 0 {
		result.Primary = &result.Categories[0]
	}

	// Record metrics
	if c.metrics != nil {
		c.metrics.CategorizationTotal.WithLabelValues("ml", "success").Inc()
		c.metrics.CategorizationDuration.Observe(result.ProcessingTime.Seconds())
		c.metrics.CategoriesAssigned.WithLabelValues("ml").Observe(float64(len(result.Categories)))
		if result.Primary != nil {
			c.metrics.ConfidenceDistribution.WithLabelValues("ml").Observe(result.Primary.Confidence)
		}
	}

	return result, nil
}

// CategorizeMultiple classifies multiple content items.
func (c *MLCategorizer) CategorizeMultiple(ctx context.Context, contents []*ContentData) ([]*CategorizationResult, error) {
	results := make([]*CategorizationResult, len(contents))

	for i, content := range contents {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result, err := c.Categorize(ctx, content)
		if err != nil {
			results[i] = &CategorizationResult{Categories: []CategoryResult{}}
			continue
		}
		results[i] = result
	}

	return results, nil
}

// SuggestCategories suggests categories based on ML similarity.
func (c *MLCategorizer) SuggestCategories(ctx context.Context, content *ContentData) ([]string, error) {
	// For ML, we can't really suggest new categories without additional models
	return nil, nil
}

// =============================================================================
// Hybrid Categorizer
// =============================================================================

// HybridCategorizer combines rule-based and ML classification.
type HybridCategorizer struct {
	ruleBased     *RuleBasedCategorizer
	mlCategorizer *MLCategorizer
	logger        logging.Logger
	metrics       *CategorizerMetrics

	// mlFallbackThreshold triggers ML if rule confidence is below this.
	mlFallbackThreshold float64

	// combineResults if true, merges results from both approaches.
	combineResults bool

	// ruleWeight is the weight for rule-based results in combination.
	ruleWeight float64

	// mlWeight is the weight for ML results in combination.
	mlWeight float64
}

// HybridCategorizerConfig configures the hybrid categorizer.
type HybridCategorizerConfig struct {
	RuleBased           *RuleBasedCategorizer
	MLCategorizer       *MLCategorizer
	Logger              logging.Logger
	Metrics             *CategorizerMetrics
	MLFallbackThreshold float64
	CombineResults      bool
	RuleWeight          float64
	MLWeight            float64
}

// NewHybridCategorizer creates a new hybrid categorizer.
func NewHybridCategorizer(config *HybridCategorizerConfig) (*HybridCategorizer, error) {
	if config.RuleBased == nil {
		return nil, errors.New("rule-based categorizer is required")
	}
	if config.MLFallbackThreshold == 0 {
		config.MLFallbackThreshold = 0.5
	}
	if config.RuleWeight == 0 {
		config.RuleWeight = 0.6
	}
	if config.MLWeight == 0 {
		config.MLWeight = 0.4
	}

	return &HybridCategorizer{
		ruleBased:           config.RuleBased,
		mlCategorizer:       config.MLCategorizer,
		logger:              config.Logger.With(logging.F("categorizer", "hybrid")),
		metrics:             config.Metrics,
		mlFallbackThreshold: config.MLFallbackThreshold,
		combineResults:      config.CombineResults,
		ruleWeight:          config.RuleWeight,
		mlWeight:            config.MLWeight,
	}, nil
}

// Categorize classifies content using hybrid approach.
func (c *HybridCategorizer) Categorize(ctx context.Context, content *ContentData) (*CategorizationResult, error) {
	startTime := time.Now()

	if content == nil {
		return nil, ErrNilContent
	}

	// Try rule-based first
	ruleResult, err := c.ruleBased.Categorize(ctx, content)
	if err != nil {
		c.logger.Warn("rule-based categorization failed", logging.Err(err))
	}

	// Determine if we need ML fallback
	needMLFallback := false
	if ruleResult == nil || len(ruleResult.Categories) == 0 {
		needMLFallback = true
	} else if ruleResult.Primary != nil && ruleResult.Primary.Confidence < c.mlFallbackThreshold {
		needMLFallback = true
	}

	// If we should combine or need fallback, try ML
	var mlResult *CategorizationResult
	if c.mlCategorizer != nil && (c.combineResults || needMLFallback) {
		mlResult, err = c.mlCategorizer.Categorize(ctx, content)
		if err != nil {
			c.logger.Debug("ML categorization failed", logging.Err(err))
		}

		if c.metrics != nil && needMLFallback {
			reason := "low_confidence"
			if ruleResult == nil || len(ruleResult.Categories) == 0 {
				reason = "no_rules_matched"
			}
			c.metrics.MLFallbacks.WithLabelValues(reason).Inc()
		}
	}

	// Determine final result
	var finalResult *CategorizationResult
	if c.combineResults && ruleResult != nil && mlResult != nil {
		finalResult = c.combineCategorizationResults(ruleResult, mlResult)
	} else if mlResult != nil && (ruleResult == nil || needMLFallback) {
		finalResult = mlResult
	} else {
		finalResult = ruleResult
	}

	if finalResult == nil {
		finalResult = &CategorizationResult{Categories: []CategoryResult{}}
	}

	finalResult.ProcessingTime = time.Since(startTime)

	// Record metrics
	if c.metrics != nil {
		c.metrics.CategorizationTotal.WithLabelValues("hybrid", "success").Inc()
		c.metrics.CategorizationDuration.Observe(finalResult.ProcessingTime.Seconds())
		c.metrics.CategoriesAssigned.WithLabelValues("hybrid").Observe(float64(len(finalResult.Categories)))
		if finalResult.Primary != nil {
			c.metrics.ConfidenceDistribution.WithLabelValues("hybrid").Observe(finalResult.Primary.Confidence)
		}
	}

	return finalResult, nil
}

// combineCategorizationResults merges results from both approaches.
func (c *HybridCategorizer) combineCategorizationResults(rule, ml *CategorizationResult) *CategorizationResult {
	combined := &CategorizationResult{
		Categories:      make([]CategoryResult, 0),
		TotalCandidates: rule.TotalCandidates,
		RulesEvaluated:  rule.RulesEvaluated,
		MLUsed:          true,
	}

	// Create a map of all categories with combined scores
	categoryScores := make(map[string]*CategoryResult)

	// Add rule-based results
	for _, cat := range rule.Categories {
		weightedConfidence := cat.Confidence * c.ruleWeight
		existing, exists := categoryScores[cat.CategoryID]
		if exists {
			existing.Confidence += weightedConfidence
		} else {
			catCopy := cat
			catCopy.Confidence = weightedConfidence
			catCopy.Source = "rule+ml"
			categoryScores[cat.CategoryID] = &catCopy
		}
	}

	// Add ML results
	for _, cat := range ml.Categories {
		weightedConfidence := cat.Confidence * c.mlWeight
		existing, exists := categoryScores[cat.CategoryID]
		if exists {
			existing.Confidence += weightedConfidence
		} else {
			catCopy := cat
			catCopy.Confidence = weightedConfidence
			catCopy.Source = "ml"
			categoryScores[cat.CategoryID] = &catCopy
		}
	}

	// Convert map to slice
	for _, cat := range categoryScores {
		// Normalize confidence (cap at 1.0)
		if cat.Confidence > 1.0 {
			cat.Confidence = 1.0
		}
		combined.Categories = append(combined.Categories, *cat)
	}

	// Sort by confidence
	sort.Slice(combined.Categories, func(i, j int) bool {
		return combined.Categories[i].Confidence > combined.Categories[j].Confidence
	})

	// Set primary
	if len(combined.Categories) > 0 {
		combined.Primary = &combined.Categories[0]
	}

	return combined
}

// CategorizeMultiple classifies multiple content items.
func (c *HybridCategorizer) CategorizeMultiple(ctx context.Context, contents []*ContentData) ([]*CategorizationResult, error) {
	results := make([]*CategorizationResult, len(contents))

	for i, content := range contents {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result, err := c.Categorize(ctx, content)
		if err != nil {
			results[i] = &CategorizationResult{Categories: []CategoryResult{}}
			continue
		}
		results[i] = result
	}

	return results, nil
}

// SuggestCategories combines suggestions from both approaches.
func (c *HybridCategorizer) SuggestCategories(ctx context.Context, content *ContentData) ([]string, error) {
	return c.ruleBased.SuggestCategories(ctx, content)
}

// =============================================================================
// Hierarchical Categorizer
// =============================================================================

// HierarchicalCategorizer classifies content through a multi-level taxonomy.
type HierarchicalCategorizer struct {
	baseCategorizer Categorizer
	taxonomy        *TaxonomyTree
	logger          logging.Logger
	metrics         *CategorizerMetrics

	// inheritConfidence if true, child confidence is multiplied by parent.
	inheritConfidence bool

	// maxDepth limits how deep in the hierarchy to classify.
	maxDepth int

	// requireParentMatch if true, only classify children if parent matches.
	requireParentMatch bool
}

// HierarchicalCategorizerConfig configures the hierarchical categorizer.
type HierarchicalCategorizerConfig struct {
	BaseCategorizer    Categorizer
	Taxonomy           *TaxonomyTree
	Logger             logging.Logger
	Metrics            *CategorizerMetrics
	InheritConfidence  bool
	MaxDepth           int
	RequireParentMatch bool
}

// NewHierarchicalCategorizer creates a new hierarchical categorizer.
func NewHierarchicalCategorizer(config *HierarchicalCategorizerConfig) (*HierarchicalCategorizer, error) {
	if config.BaseCategorizer == nil {
		return nil, errors.New("base categorizer is required")
	}
	if config.Taxonomy == nil {
		return nil, ErrEmptyTaxonomy
	}
	if config.MaxDepth == 0 {
		config.MaxDepth = 10 // Reasonable default
	}

	return &HierarchicalCategorizer{
		baseCategorizer:    config.BaseCategorizer,
		taxonomy:           config.Taxonomy,
		logger:             config.Logger.With(logging.F("categorizer", "hierarchical")),
		metrics:            config.Metrics,
		inheritConfidence:  config.InheritConfidence,
		maxDepth:           config.MaxDepth,
		requireParentMatch: config.RequireParentMatch,
	}, nil
}

// Categorize classifies content hierarchically.
func (c *HierarchicalCategorizer) Categorize(ctx context.Context, content *ContentData) (*CategorizationResult, error) {
	startTime := time.Now()

	if content == nil {
		return nil, ErrNilContent
	}

	// Get base categorization
	baseResult, err := c.baseCategorizer.Categorize(ctx, content)
	if err != nil {
		return nil, err
	}

	result := &CategorizationResult{
		Categories:      make([]CategoryResult, 0),
		ProcessingTime:  time.Since(startTime),
		TotalCandidates: baseResult.TotalCandidates,
		RulesEvaluated:  baseResult.RulesEvaluated,
		MLUsed:          baseResult.MLUsed,
	}

	// Build hierarchy for each matched category
	matchedCategories := make(map[string]float64)
	for _, cat := range baseResult.Categories {
		matchedCategories[cat.CategoryID] = cat.Confidence
	}

	// For each matched category, include its ancestors
	for _, cat := range baseResult.Categories {
		// Get the full path
		path, err := c.taxonomy.GetPath(cat.CategoryID)
		if err != nil {
			continue
		}

		// Add path categories with confidence inheritance
		confidence := cat.Confidence
		for i := len(path) - 1; i >= 0; i-- {
			pathCat := path[i]

			// Check if already added with higher confidence
			if existingConf, exists := matchedCategories[pathCat.ID]; exists && existingConf >= confidence {
				continue
			}

			catPath, _ := c.taxonomy.GetPathString(pathCat.ID, "/")
			depth, _ := c.taxonomy.GetDepth(pathCat.ID)

			catResult := CategoryResult{
				CategoryID:   pathCat.ID,
				CategoryName: pathCat.Name,
				CategoryPath: catPath,
				Confidence:   confidence,
				Source:       cat.Source,
				MatchedTerms: cat.MatchedTerms,
				Metadata: map[string]string{
					"depth": string(rune('0' + depth)),
				},
			}

			result.Categories = append(result.Categories, catResult)
			matchedCategories[pathCat.ID] = confidence

			// Reduce confidence as we go up the hierarchy
			if c.inheritConfidence {
				confidence *= 0.9
			}
		}
	}

	// Sort by confidence
	sort.Slice(result.Categories, func(i, j int) bool {
		return result.Categories[i].Confidence > result.Categories[j].Confidence
	})

	// Set primary (prefer leaf nodes)
	if len(result.Categories) > 0 {
		// Find the deepest category with highest confidence
		for _, cat := range result.Categories {
			children, _ := c.taxonomy.GetChildren(cat.CategoryID)
			isLeaf := len(children) == 0
			if isLeaf || result.Primary == nil {
				result.Primary = &cat
				if isLeaf {
					break
				}
			}
		}
	}

	result.ProcessingTime = time.Since(startTime)

	// Record metrics
	if c.metrics != nil {
		c.metrics.CategorizationTotal.WithLabelValues("hierarchical", "success").Inc()
		c.metrics.CategorizationDuration.Observe(result.ProcessingTime.Seconds())
		c.metrics.CategoriesAssigned.WithLabelValues("hierarchical").Observe(float64(len(result.Categories)))
		if result.Primary != nil {
			c.metrics.ConfidenceDistribution.WithLabelValues("hierarchical").Observe(result.Primary.Confidence)
		}
	}

	return result, nil
}

// CategorizeMultiple classifies multiple content items.
func (c *HierarchicalCategorizer) CategorizeMultiple(ctx context.Context, contents []*ContentData) ([]*CategorizationResult, error) {
	results := make([]*CategorizationResult, len(contents))

	for i, content := range contents {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result, err := c.Categorize(ctx, content)
		if err != nil {
			results[i] = &CategorizationResult{Categories: []CategoryResult{}}
			continue
		}
		results[i] = result
	}

	return results, nil
}

// SuggestCategories delegates to the base categorizer.
func (c *HierarchicalCategorizer) SuggestCategories(ctx context.Context, content *ContentData) ([]string, error) {
	return c.baseCategorizer.SuggestCategories(ctx, content)
}

// =============================================================================
// Multi-Tenant Categorizer
// =============================================================================

// MultiTenantCategorizer manages per-tenant categorization.
type MultiTenantCategorizer struct {
	mu            sync.RWMutex
	categorizers  map[string]Categorizer
	defaultCat    Categorizer
	taxonomyReg   *TaxonomyRegistry
	logger        logging.Logger
	metrics       *CategorizerMetrics
	createFunc    func(taxonomy *TaxonomyTree) (Categorizer, error)
}

// MultiTenantCategorizerConfig configures the multi-tenant categorizer.
type MultiTenantCategorizerConfig struct {
	DefaultCategorizer Categorizer
	TaxonomyRegistry   *TaxonomyRegistry
	Logger             logging.Logger
	Metrics            *CategorizerMetrics
	CreateFunc         func(taxonomy *TaxonomyTree) (Categorizer, error)
}

// NewMultiTenantCategorizer creates a new multi-tenant categorizer.
func NewMultiTenantCategorizer(config *MultiTenantCategorizerConfig) *MultiTenantCategorizer {
	return &MultiTenantCategorizer{
		categorizers: make(map[string]Categorizer),
		defaultCat:   config.DefaultCategorizer,
		taxonomyReg:  config.TaxonomyRegistry,
		logger:       config.Logger.With(logging.F("categorizer", "multi-tenant")),
		metrics:      config.Metrics,
		createFunc:   config.CreateFunc,
	}
}

// GetCategorizer returns the categorizer for a tenant.
func (c *MultiTenantCategorizer) GetCategorizer(tenantID string) Categorizer {
	c.mu.RLock()
	cat, exists := c.categorizers[tenantID]
	c.mu.RUnlock()

	if exists {
		return cat
	}

	// Fall back to default
	return c.defaultCat
}

// SetCategorizer sets a custom categorizer for a tenant.
func (c *MultiTenantCategorizer) SetCategorizer(tenantID string, categorizer Categorizer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.categorizers[tenantID] = categorizer
}

// Categorize routes to the appropriate tenant categorizer.
func (c *MultiTenantCategorizer) Categorize(ctx context.Context, tenantID string, content *ContentData) (*CategorizationResult, error) {
	cat := c.GetCategorizer(tenantID)
	if cat == nil {
		return nil, errors.New("no categorizer available")
	}
	return cat.Categorize(ctx, content)
}
