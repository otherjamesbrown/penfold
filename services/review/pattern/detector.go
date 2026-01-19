// Package pattern provides the main PatternDetector for behavioral pattern detection.
package pattern

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/review/automation"
	"github.com/prometheus/client_golang/prometheus"
)

// DetectorConfig holds configuration for the PatternDetector.
type DetectorConfig struct {
	// MinConfidence is the minimum confidence score for pattern suggestions.
	MinConfidence float64

	// MinSupportCount is the minimum number of observations for pattern detection.
	MinSupportCount int

	// MaxPatterns limits the number of patterns per tenant.
	MaxPatterns int

	// MaxSuggestions limits the number of suggestions returned.
	MaxSuggestions int

	// EnableAutoDiscovery enables automatic pattern discovery.
	EnableAutoDiscovery bool

	// DiscoveryInterval is how often to run pattern discovery.
	DiscoveryInterval time.Duration

	// CacheHotPatterns enables caching of frequently used patterns.
	CacheHotPatterns bool

	// HotPatternThreshold is the minimum match count to be considered "hot".
	HotPatternThreshold int64
}

// DefaultDetectorConfig returns sensible defaults.
func DefaultDetectorConfig() *DetectorConfig {
	return &DetectorConfig{
		MinConfidence:       0.7,
		MinSupportCount:     5,
		MaxPatterns:         100,
		MaxSuggestions:      10,
		EnableAutoDiscovery: true,
		DiscoveryInterval:   time.Hour,
		CacheHotPatterns:    true,
		HotPatternThreshold: 100,
	}
}

// DetectorMetrics holds Prometheus metrics for the pattern detector.
type DetectorMetrics struct {
	patternsEvaluated  *prometheus.CounterVec
	patternsMatched    *prometheus.CounterVec
	detectionsTotal    *prometheus.CounterVec
	detectionDuration  *prometheus.HistogramVec
	activePatterns     *prometheus.GaugeVec
	suggestionsCreated *prometheus.CounterVec
	learningEvents     prometheus.Counter
}

// NewDetectorMetrics creates metrics for the pattern detector.
func NewDetectorMetrics(namespace string) *DetectorMetrics {
	return &DetectorMetrics{
		patternsEvaluated: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "pattern",
				Name:      "patterns_evaluated_total",
				Help:      "Total number of patterns evaluated",
			},
			[]string{"tenant_id", "pattern_type"},
		),
		patternsMatched: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "pattern",
				Name:      "patterns_matched_total",
				Help:      "Total number of patterns that matched",
			},
			[]string{"tenant_id", "pattern_type", "pattern_id"},
		),
		detectionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "pattern",
				Name:      "detections_total",
				Help:      "Total number of pattern detection runs",
			},
			[]string{"tenant_id"},
		),
		detectionDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "pattern",
				Name:      "detection_duration_seconds",
				Help:      "Time spent detecting patterns",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25},
			},
			[]string{"tenant_id"},
		),
		activePatterns: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "pattern",
				Name:      "active_patterns",
				Help:      "Number of active patterns",
			},
			[]string{"tenant_id", "pattern_type"},
		),
		suggestionsCreated: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "pattern",
				Name:      "suggestions_created_total",
				Help:      "Total number of pattern suggestions created",
			},
			[]string{"tenant_id"},
		),
		learningEvents: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "pattern",
				Name:      "learning_events_total",
				Help:      "Total number of learning events processed",
			},
		),
	}
}

// RegisterMetrics registers all detector metrics.
func (m *DetectorMetrics) RegisterMetrics() error {
	collectors := []prometheus.Collector{
		m.patternsEvaluated,
		m.patternsMatched,
		m.detectionsTotal,
		m.detectionDuration,
		m.activePatterns,
		m.suggestionsCreated,
		m.learningEvents,
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

// PatternDetector is the main component for detecting and managing patterns.
type PatternDetector struct {
	config          *DetectorConfig
	patternSets     map[string]*PatternSet // tenantID -> PatternSet
	store           PatternStore
	analyzer        *PatternAnalyzer
	learner         *PatternLearner
	compiledRegex   map[string]*regexp.Regexp
	logger          logging.Logger
	metrics         *DetectorMetrics
	mu              sync.RWMutex
}

// NewPatternDetector creates a new PatternDetector.
func NewPatternDetector(config *DetectorConfig, store PatternStore, logger logging.Logger) *PatternDetector {
	if config == nil {
		config = DefaultDetectorConfig()
	}

	detector := &PatternDetector{
		config:        config,
		patternSets:   make(map[string]*PatternSet),
		store:         store,
		compiledRegex: make(map[string]*regexp.Regexp),
		logger:        logger.With(logging.F("component", "pattern_detector")),
		metrics:       NewDetectorMetrics("penfold"),
	}

	// Initialize analyzer and learner
	detector.analyzer = NewPatternAnalyzer(detector)
	detector.learner = NewPatternLearner(DefaultLearnerConfig(), detector)

	return detector
}

// SetStore sets the pattern store.
func (d *PatternDetector) SetStore(store PatternStore) {
	d.store = store
}

// GetPatternSet returns the pattern set for a tenant.
func (d *PatternDetector) GetPatternSet(tenantID string) *PatternSet {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.patternSets[tenantID]
}

// SetPatternSet sets the pattern set for a tenant.
func (d *PatternDetector) SetPatternSet(patternSet *PatternSet) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.patternSets[patternSet.TenantID] = patternSet

	// Update metrics
	if d.metrics != nil {
		for _, pt := range ValidPatternTypes() {
			count := len(patternSet.GetPatternsByType(pt))
			d.metrics.activePatterns.WithLabelValues(patternSet.TenantID, string(pt)).Set(float64(count))
		}
	}
}

// AddPattern adds a pattern to a tenant's pattern set.
func (d *PatternDetector) AddPattern(pattern *Pattern) error {
	if err := pattern.Validate(); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	patternSet, exists := d.patternSets[pattern.TenantID]
	if !exists {
		patternSet = NewPatternSet(pattern.TenantID)
		d.patternSets[pattern.TenantID] = patternSet
	}

	patternSet.AddPattern(pattern)

	// Persist if store is configured
	if d.store != nil {
		ctx := context.Background()
		if err := d.store.Save(ctx, pattern); err != nil {
			d.logger.Warn("failed to persist pattern",
				logging.Err(err),
				logging.F("pattern_id", pattern.ID),
			)
		}
	}

	d.logger.Info("pattern added",
		logging.F("pattern_id", pattern.ID),
		logging.F("pattern_name", pattern.Name),
		logging.F("tenant_id", pattern.TenantID),
		logging.F("type", pattern.Type),
	)

	return nil
}

// RemovePattern removes a pattern from a tenant's pattern set.
func (d *PatternDetector) RemovePattern(tenantID, patternID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	patternSet, exists := d.patternSets[tenantID]
	if !exists {
		return false
	}

	removed := patternSet.RemovePattern(patternID)
	if removed && d.store != nil {
		ctx := context.Background()
		if err := d.store.Delete(ctx, tenantID, patternID); err != nil {
			d.logger.Warn("failed to delete pattern from store",
				logging.Err(err),
				logging.F("pattern_id", patternID),
			)
		}
	}

	return removed
}

// DetectPatterns evaluates all patterns against a list of review items and returns matches.
func (d *PatternDetector) DetectPatterns(items []*automation.ReviewItem) []*Pattern {
	if len(items) == 0 {
		return []*Pattern{}
	}

	startTime := time.Now()
	matchedPatterns := make(map[string]*Pattern)

	// Process items and find matching patterns
	for _, item := range items {
		d.mu.RLock()
		patternSet, exists := d.patternSets[item.TenantID]
		d.mu.RUnlock()

		if !exists || !patternSet.Enabled {
			continue
		}

		for _, pattern := range patternSet.GetEnabledPatterns() {
			if d.MatchPattern(item, pattern) {
				matchedPatterns[pattern.ID] = pattern

				if d.metrics != nil {
					d.metrics.patternsMatched.WithLabelValues(
						item.TenantID,
						string(pattern.Type),
						pattern.ID,
					).Inc()
				}
			}

			if d.metrics != nil {
				d.metrics.patternsEvaluated.WithLabelValues(
					item.TenantID,
					string(pattern.Type),
				).Inc()
			}
		}
	}

	// Convert map to slice
	result := make([]*Pattern, 0, len(matchedPatterns))
	for _, p := range matchedPatterns {
		result = append(result, p)
	}

	// Record metrics
	if d.metrics != nil && len(items) > 0 {
		d.metrics.detectionsTotal.WithLabelValues(items[0].TenantID).Inc()
		d.metrics.detectionDuration.WithLabelValues(items[0].TenantID).Observe(time.Since(startTime).Seconds())
	}

	return result
}

// MatchPattern checks if a review item matches a specific pattern.
func (d *PatternDetector) MatchPattern(item *automation.ReviewItem, pattern *Pattern) bool {
	if item == nil || pattern == nil {
		return false
	}

	// All conditions must match for the pattern to match
	for _, condition := range pattern.Conditions {
		if !d.evaluateCondition(item, condition) {
			return false
		}
	}

	// Update pattern stats
	pattern.UpdateStats(true, false, 0)

	return true
}

// evaluateCondition evaluates a single condition against a review item.
func (d *PatternDetector) evaluateCondition(item *automation.ReviewItem, condition PatternCondition) bool {
	// Get the field value from the item
	fieldValue := d.getFieldValue(item, condition.Field)
	if fieldValue == nil {
		return false
	}

	// Evaluate based on operator
	switch condition.Operator {
	case OpEquals:
		return d.compareEquals(fieldValue, condition.Value, condition.CaseSensitive)
	case OpNotEquals:
		return !d.compareEquals(fieldValue, condition.Value, condition.CaseSensitive)
	case OpContains:
		return d.compareContains(fieldValue, condition.Value, condition.CaseSensitive)
	case OpStartsWith:
		return d.compareStartsWith(fieldValue, condition.Value, condition.CaseSensitive)
	case OpEndsWith:
		return d.compareEndsWith(fieldValue, condition.Value, condition.CaseSensitive)
	case OpMatches:
		return d.compareMatches(fieldValue, condition.Value)
	case OpGreaterThan:
		return d.compareNumeric(fieldValue, condition.Value, ">")
	case OpGreaterOrEqual:
		return d.compareNumeric(fieldValue, condition.Value, ">=")
	case OpLessThan:
		return d.compareNumeric(fieldValue, condition.Value, "<")
	case OpLessOrEqual:
		return d.compareNumeric(fieldValue, condition.Value, "<=")
	case OpIn:
		return d.compareIn(fieldValue, condition.Value, condition.CaseSensitive)
	case OpNotIn:
		return !d.compareIn(fieldValue, condition.Value, condition.CaseSensitive)
	case OpBetween:
		return d.compareBetween(fieldValue, condition.Value)
	default:
		return d.compareEquals(fieldValue, condition.Value, condition.CaseSensitive)
	}
}

// getFieldValue extracts the value of a field from a review item.
func (d *PatternDetector) getFieldValue(item *automation.ReviewItem, field string) interface{} {
	switch field {
	case "sender":
		return item.Sender
	case "subject":
		return item.Subject
	case "body":
		return item.Body
	case "summary":
		return item.Summary
	case "category":
		return item.Category
	case "source":
		return item.Source
	case "content_type":
		return item.ContentType
	case "priority":
		return item.Priority
	case "confidence":
		return item.Confidence
	case "content":
		return item.Subject + " " + item.Body + " " + item.Summary
	case "keywords":
		return item.Keywords
	case "tags":
		return item.Tags
	case "age_hours":
		return item.Age().Hours()
	default:
		// Check metadata
		if item.Metadata != nil {
			if v, ok := item.Metadata[field]; ok {
				return v
			}
		}
		return nil
	}
}

// compareEquals compares values for equality.
func (d *PatternDetector) compareEquals(fieldValue, expectedValue interface{}, caseSensitive bool) bool {
	fStr, fOk := fieldValue.(string)
	eStr, eOk := expectedValue.(string)

	if fOk && eOk {
		if caseSensitive {
			return fStr == eStr
		}
		return strings.EqualFold(fStr, eStr)
	}

	return fieldValue == expectedValue
}

// compareContains checks if field value contains expected value.
func (d *PatternDetector) compareContains(fieldValue, expectedValue interface{}, caseSensitive bool) bool {
	fStr, fOk := fieldValue.(string)
	eStr, eOk := expectedValue.(string)

	if !fOk || !eOk {
		return false
	}

	if caseSensitive {
		return strings.Contains(fStr, eStr)
	}
	return strings.Contains(strings.ToLower(fStr), strings.ToLower(eStr))
}

// compareStartsWith checks if field value starts with expected value.
func (d *PatternDetector) compareStartsWith(fieldValue, expectedValue interface{}, caseSensitive bool) bool {
	fStr, fOk := fieldValue.(string)
	eStr, eOk := expectedValue.(string)

	if !fOk || !eOk {
		return false
	}

	if caseSensitive {
		return strings.HasPrefix(fStr, eStr)
	}
	return strings.HasPrefix(strings.ToLower(fStr), strings.ToLower(eStr))
}

// compareEndsWith checks if field value ends with expected value.
func (d *PatternDetector) compareEndsWith(fieldValue, expectedValue interface{}, caseSensitive bool) bool {
	fStr, fOk := fieldValue.(string)
	eStr, eOk := expectedValue.(string)

	if !fOk || !eOk {
		return false
	}

	if caseSensitive {
		return strings.HasSuffix(fStr, eStr)
	}
	return strings.HasSuffix(strings.ToLower(fStr), strings.ToLower(eStr))
}

// compareMatches evaluates a regex pattern match.
func (d *PatternDetector) compareMatches(fieldValue, patternValue interface{}) bool {
	fStr, fOk := fieldValue.(string)
	pStr, pOk := patternValue.(string)

	if !fOk || !pOk {
		return false
	}

	// Check for cached regex
	d.mu.RLock()
	re, exists := d.compiledRegex[pStr]
	d.mu.RUnlock()

	if !exists {
		var err error
		re, err = regexp.Compile(pStr)
		if err != nil {
			return false
		}
		d.mu.Lock()
		d.compiledRegex[pStr] = re
		d.mu.Unlock()
	}

	return re.MatchString(fStr)
}

// compareNumeric compares numeric values.
func (d *PatternDetector) compareNumeric(fieldValue, expectedValue interface{}, op string) bool {
	fFloat := toFloat64(fieldValue)
	eFloat := toFloat64(expectedValue)

	switch op {
	case ">":
		return fFloat > eFloat
	case ">=":
		return fFloat >= eFloat
	case "<":
		return fFloat < eFloat
	case "<=":
		return fFloat <= eFloat
	default:
		return fFloat == eFloat
	}
}

// compareIn checks if field value is in a list.
func (d *PatternDetector) compareIn(fieldValue, listValue interface{}, caseSensitive bool) bool {
	fStr, fOk := fieldValue.(string)
	if !fOk {
		return false
	}

	switch v := listValue.(type) {
	case []string:
		for _, item := range v {
			if caseSensitive {
				if fStr == item {
					return true
				}
			} else {
				if strings.EqualFold(fStr, item) {
					return true
				}
			}
		}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				if caseSensitive {
					if fStr == s {
						return true
					}
				} else {
					if strings.EqualFold(fStr, s) {
						return true
					}
				}
			}
		}
	case string:
		// Comma-separated list
		items := strings.Split(v, ",")
		for _, item := range items {
			trimmed := strings.TrimSpace(item)
			if caseSensitive {
				if fStr == trimmed {
					return true
				}
			} else {
				if strings.EqualFold(fStr, trimmed) {
					return true
				}
			}
		}
	}

	return false
}

// compareBetween checks if value is between two bounds.
func (d *PatternDetector) compareBetween(fieldValue, boundsValue interface{}) bool {
	fFloat := toFloat64(fieldValue)

	switch v := boundsValue.(type) {
	case []float64:
		if len(v) >= 2 {
			return fFloat >= v[0] && fFloat <= v[1]
		}
	case []interface{}:
		if len(v) >= 2 {
			low := toFloat64(v[0])
			high := toFloat64(v[1])
			return fFloat >= low && fFloat <= high
		}
	}

	return false
}

// toFloat64 converts various numeric types to float64.
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	default:
		return 0
	}
}

// LearnPattern creates a pattern from a set of user decisions.
func (d *PatternDetector) LearnPattern(decisions []*automation.Decision) *Pattern {
	if len(decisions) == 0 {
		return nil
	}

	return d.learner.ExtractPattern(decisions)
}

// SuggestPatterns returns suggested patterns based on observed behavior.
func (d *PatternDetector) SuggestPatterns() []*PatternSuggestion {
	return d.learner.SuggestPatterns()
}

// SuggestPatternsForTenant returns suggestions for a specific tenant.
func (d *PatternDetector) SuggestPatternsForTenant(ctx context.Context, tenantID string) ([]*PatternSuggestion, error) {
	suggestions, err := d.learner.SuggestPatternsForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	if d.metrics != nil {
		d.metrics.suggestionsCreated.WithLabelValues(tenantID).Add(float64(len(suggestions)))
	}

	return suggestions, nil
}

// RecordDecision records a user decision for pattern learning.
func (d *PatternDetector) RecordDecision(decision *automation.Decision) {
	d.learner.RecordDecision(decision)

	if d.metrics != nil {
		d.metrics.learningEvents.Inc()
	}

	d.logger.Debug("decision recorded for pattern learning",
		logging.F("item_id", decision.ItemID),
		logging.F("action", decision.Action),
	)
}

// GetAnalyzer returns the pattern analyzer.
func (d *PatternDetector) GetAnalyzer() *PatternAnalyzer {
	return d.analyzer
}

// GetLearner returns the pattern learner.
func (d *PatternDetector) GetLearner() *PatternLearner {
	return d.learner
}

// LoadPatterns loads patterns from the store for a tenant.
func (d *PatternDetector) LoadPatterns(ctx context.Context, tenantID string) error {
	if d.store == nil {
		return nil
	}

	patterns, err := d.store.List(ctx, tenantID, 0, d.config.MaxPatterns)
	if err != nil {
		return err
	}

	patternSet := NewPatternSet(tenantID)
	for _, p := range patterns {
		patternSet.AddPattern(p)
	}

	d.SetPatternSet(patternSet)

	d.logger.Info("patterns loaded",
		logging.F("tenant_id", tenantID),
		logging.F("count", len(patterns)),
	)

	return nil
}
