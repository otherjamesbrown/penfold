package categorization

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// RulePriority defines the execution order of rules.
type RulePriority int

const (
	// PriorityHigh for rules that should be evaluated first.
	PriorityHigh RulePriority = 100

	// PriorityNormal for standard rules.
	PriorityNormal RulePriority = 50

	// PriorityLow for fallback rules.
	PriorityLow RulePriority = 10
)

// RuleType identifies the type of rule.
type RuleType string

const (
	RuleTypeKeyword  RuleType = "keyword"
	RuleTypePattern  RuleType = "pattern"
	RuleTypeMetadata RuleType = "metadata"
	RuleTypeCustom   RuleType = "custom"
)

// RuleMatch represents a single rule match result.
type RuleMatch struct {
	// CategoryID is the matched category.
	CategoryID string

	// RuleID identifies which rule produced this match.
	RuleID string

	// RuleType is the type of rule that matched.
	RuleType RuleType

	// Confidence is the match confidence (0.0 to 1.0).
	Confidence float64

	// MatchedTerms contains the terms that triggered the match.
	MatchedTerms []string

	// Position is the position in content where match was found.
	Position int

	// Context is the surrounding text (for pattern matches).
	Context string
}

// Rule is the interface for all categorization rules.
type Rule interface {
	// ID returns the unique identifier for this rule.
	ID() string

	// Type returns the rule type.
	Type() RuleType

	// Priority returns the rule priority for ordering.
	Priority() RulePriority

	// CategoryID returns the category this rule targets.
	CategoryID() string

	// Match evaluates the rule against content and returns matches.
	Match(ctx context.Context, content *ContentData) []RuleMatch

	// IsEnabled returns whether the rule is active.
	IsEnabled() bool

	// SetEnabled enables or disables the rule.
	SetEnabled(enabled bool)
}

// ContentData holds the content to be categorized.
type ContentData struct {
	// Text is the main content text (normalized).
	Text string

	// Title is the content title if available.
	Title string

	// SourceType indicates the content source (email, document, etc.).
	SourceType string

	// Metadata contains additional content attributes.
	Metadata map[string]string

	// Sender for email content.
	Sender string

	// Recipients for email content.
	Recipients []string

	// Keywords are pre-extracted keywords from analysis.
	Keywords []string

	// Entities are pre-extracted entities.
	Entities []string
}

// =============================================================================
// Keyword Rule
// =============================================================================

// KeywordRule matches content based on keyword presence.
type KeywordRule struct {
	id          string
	categoryID  string
	priority    RulePriority
	enabled     bool
	mu          sync.RWMutex

	// Keywords to match (case-insensitive).
	keywords []string

	// RequireAll if true, all keywords must match.
	requireAll bool

	// MinMatches is the minimum number of keywords required.
	minMatches int

	// BaseConfidence is the starting confidence for a match.
	baseConfidence float64

	// ConfidenceBoost per additional keyword match.
	confidenceBoost float64

	// MatchTitle if true, also matches against the title.
	matchTitle bool

	// precomputed lowercase keywords
	lowerKeywords []string
}

// KeywordRuleConfig configures a KeywordRule.
type KeywordRuleConfig struct {
	ID              string
	CategoryID      string
	Priority        RulePriority
	Keywords        []string
	RequireAll      bool
	MinMatches      int
	BaseConfidence  float64
	ConfidenceBoost float64
	MatchTitle      bool
}

// NewKeywordRule creates a new keyword matching rule.
func NewKeywordRule(config *KeywordRuleConfig) *KeywordRule {
	if config.Priority == 0 {
		config.Priority = PriorityNormal
	}
	if config.BaseConfidence == 0 {
		config.BaseConfidence = 0.5
	}
	if config.ConfidenceBoost == 0 {
		config.ConfidenceBoost = 0.1
	}
	if config.MinMatches == 0 {
		config.MinMatches = 1
	}

	// Precompute lowercase keywords
	lowerKeywords := make([]string, len(config.Keywords))
	for i, kw := range config.Keywords {
		lowerKeywords[i] = strings.ToLower(kw)
	}

	return &KeywordRule{
		id:              config.ID,
		categoryID:      config.CategoryID,
		priority:        config.Priority,
		enabled:         true,
		keywords:        config.Keywords,
		lowerKeywords:   lowerKeywords,
		requireAll:      config.RequireAll,
		minMatches:      config.MinMatches,
		baseConfidence:  config.BaseConfidence,
		confidenceBoost: config.ConfidenceBoost,
		matchTitle:      config.MatchTitle,
	}
}

func (r *KeywordRule) ID() string          { return r.id }
func (r *KeywordRule) Type() RuleType      { return RuleTypeKeyword }
func (r *KeywordRule) Priority() RulePriority { return r.priority }
func (r *KeywordRule) CategoryID() string  { return r.categoryID }

func (r *KeywordRule) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enabled
}

func (r *KeywordRule) SetEnabled(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = enabled
}

func (r *KeywordRule) Match(ctx context.Context, content *ContentData) []RuleMatch {
	r.mu.RLock()
	if !r.enabled {
		r.mu.RUnlock()
		return nil
	}
	r.mu.RUnlock()

	if content == nil || content.Text == "" {
		return nil
	}

	// Prepare content for matching
	lowerText := strings.ToLower(content.Text)
	lowerTitle := ""
	if r.matchTitle && content.Title != "" {
		lowerTitle = strings.ToLower(content.Title)
	}

	// Count keyword matches
	matchedTerms := make([]string, 0)
	for i, kw := range r.lowerKeywords {
		if strings.Contains(lowerText, kw) || (lowerTitle != "" && strings.Contains(lowerTitle, kw)) {
			matchedTerms = append(matchedTerms, r.keywords[i])
		}
	}

	// Check match requirements
	if r.requireAll && len(matchedTerms) != len(r.keywords) {
		return nil
	}
	if len(matchedTerms) < r.minMatches {
		return nil
	}

	// Calculate confidence
	confidence := r.baseConfidence
	// Add boost for extra matches beyond minimum
	extraMatches := len(matchedTerms) - r.minMatches
	if extraMatches > 0 {
		confidence += float64(extraMatches) * r.confidenceBoost
	}
	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return []RuleMatch{{
		CategoryID:   r.categoryID,
		RuleID:       r.id,
		RuleType:     RuleTypeKeyword,
		Confidence:   confidence,
		MatchedTerms: matchedTerms,
	}}
}

// =============================================================================
// Pattern Rule
// =============================================================================

// PatternRule matches content based on regex patterns.
type PatternRule struct {
	id         string
	categoryID string
	priority   RulePriority
	enabled    bool
	mu         sync.RWMutex

	// patterns are the compiled regex patterns.
	patterns []*regexp.Regexp

	// patternStrings are the original pattern strings.
	patternStrings []string

	// BaseConfidence for pattern matches.
	baseConfidence float64

	// CaptureContext if true, captures surrounding text.
	captureContext bool

	// ContextSize is characters to capture around match.
	contextSize int
}

// PatternRuleConfig configures a PatternRule.
type PatternRuleConfig struct {
	ID             string
	CategoryID     string
	Priority       RulePriority
	Patterns       []string
	BaseConfidence float64
	CaptureContext bool
	ContextSize    int
}

// NewPatternRule creates a new pattern matching rule.
func NewPatternRule(config *PatternRuleConfig) (*PatternRule, error) {
	if config.Priority == 0 {
		config.Priority = PriorityNormal
	}
	if config.BaseConfidence == 0 {
		config.BaseConfidence = 0.7
	}
	if config.ContextSize == 0 {
		config.ContextSize = 50
	}

	// Compile patterns
	patterns := make([]*regexp.Regexp, 0, len(config.Patterns))
	for _, p := range config.Patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, re)
	}

	return &PatternRule{
		id:             config.ID,
		categoryID:     config.CategoryID,
		priority:       config.Priority,
		enabled:        true,
		patterns:       patterns,
		patternStrings: config.Patterns,
		baseConfidence: config.BaseConfidence,
		captureContext: config.CaptureContext,
		contextSize:    config.ContextSize,
	}, nil
}

func (r *PatternRule) ID() string          { return r.id }
func (r *PatternRule) Type() RuleType      { return RuleTypePattern }
func (r *PatternRule) Priority() RulePriority { return r.priority }
func (r *PatternRule) CategoryID() string  { return r.categoryID }

func (r *PatternRule) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enabled
}

func (r *PatternRule) SetEnabled(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = enabled
}

func (r *PatternRule) Match(ctx context.Context, content *ContentData) []RuleMatch {
	r.mu.RLock()
	if !r.enabled {
		r.mu.RUnlock()
		return nil
	}
	r.mu.RUnlock()

	if content == nil || content.Text == "" {
		return nil
	}

	var matches []RuleMatch

	for i, pattern := range r.patterns {
		loc := pattern.FindStringIndex(content.Text)
		if loc == nil {
			continue
		}

		match := RuleMatch{
			CategoryID:   r.categoryID,
			RuleID:       r.id,
			RuleType:     RuleTypePattern,
			Confidence:   r.baseConfidence,
			MatchedTerms: []string{r.patternStrings[i]},
			Position:     loc[0],
		}

		// Capture context if enabled
		if r.captureContext {
			start := loc[0] - r.contextSize
			if start < 0 {
				start = 0
			}
			end := loc[1] + r.contextSize
			if end > len(content.Text) {
				end = len(content.Text)
			}
			match.Context = content.Text[start:end]
		}

		matches = append(matches, match)
	}

	return matches
}

// =============================================================================
// Metadata Rule
// =============================================================================

// MetadataRule matches content based on metadata fields.
type MetadataRule struct {
	id         string
	categoryID string
	priority   RulePriority
	enabled    bool
	mu         sync.RWMutex

	// conditions maps metadata keys to required values.
	conditions map[string]string

	// conditionPatterns maps metadata keys to regex patterns.
	conditionPatterns map[string]*regexp.Regexp

	// requireAll if true, all conditions must match.
	requireAll bool

	// BaseConfidence for metadata matches.
	baseConfidence float64
}

// MetadataRuleConfig configures a MetadataRule.
type MetadataRuleConfig struct {
	ID                string
	CategoryID        string
	Priority          RulePriority
	Conditions        map[string]string
	ConditionPatterns map[string]string
	RequireAll        bool
	BaseConfidence    float64
}

// NewMetadataRule creates a new metadata matching rule.
func NewMetadataRule(config *MetadataRuleConfig) (*MetadataRule, error) {
	if config.Priority == 0 {
		config.Priority = PriorityNormal
	}
	if config.BaseConfidence == 0 {
		config.BaseConfidence = 0.8
	}

	// Compile condition patterns
	conditionPatterns := make(map[string]*regexp.Regexp)
	for key, pattern := range config.ConditionPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		conditionPatterns[key] = re
	}

	conditions := config.Conditions
	if conditions == nil {
		conditions = make(map[string]string)
	}

	return &MetadataRule{
		id:                config.ID,
		categoryID:        config.CategoryID,
		priority:          config.Priority,
		enabled:           true,
		conditions:        conditions,
		conditionPatterns: conditionPatterns,
		requireAll:        config.RequireAll,
		baseConfidence:    config.BaseConfidence,
	}, nil
}

func (r *MetadataRule) ID() string          { return r.id }
func (r *MetadataRule) Type() RuleType      { return RuleTypeMetadata }
func (r *MetadataRule) Priority() RulePriority { return r.priority }
func (r *MetadataRule) CategoryID() string  { return r.categoryID }

func (r *MetadataRule) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enabled
}

func (r *MetadataRule) SetEnabled(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = enabled
}

func (r *MetadataRule) Match(ctx context.Context, content *ContentData) []RuleMatch {
	r.mu.RLock()
	if !r.enabled {
		r.mu.RUnlock()
		return nil
	}
	r.mu.RUnlock()

	if content == nil || content.Metadata == nil {
		return nil
	}

	matchedTerms := make([]string, 0)
	totalConditions := len(r.conditions) + len(r.conditionPatterns)

	// Check exact value conditions
	for key, expectedValue := range r.conditions {
		if actualValue, exists := content.Metadata[key]; exists {
			if strings.EqualFold(actualValue, expectedValue) {
				matchedTerms = append(matchedTerms, key+"="+expectedValue)
			}
		}
	}

	// Check pattern conditions
	for key, pattern := range r.conditionPatterns {
		if actualValue, exists := content.Metadata[key]; exists {
			if pattern.MatchString(actualValue) {
				matchedTerms = append(matchedTerms, key+"~pattern")
			}
		}
	}

	// Check match requirements
	if len(matchedTerms) == 0 {
		return nil
	}
	if r.requireAll && len(matchedTerms) != totalConditions {
		return nil
	}

	// Calculate confidence based on match ratio
	confidence := r.baseConfidence * (float64(len(matchedTerms)) / float64(totalConditions))

	return []RuleMatch{{
		CategoryID:   r.categoryID,
		RuleID:       r.id,
		RuleType:     RuleTypeMetadata,
		Confidence:   confidence,
		MatchedTerms: matchedTerms,
	}}
}

// =============================================================================
// Custom Function Rule
// =============================================================================

// CustomMatchFunc is a function type for custom matching logic.
type CustomMatchFunc func(ctx context.Context, content *ContentData) []RuleMatch

// CustomRule allows arbitrary matching logic.
type CustomRule struct {
	id         string
	categoryID string
	priority   RulePriority
	enabled    bool
	mu         sync.RWMutex
	matchFunc  CustomMatchFunc
}

// NewCustomRule creates a new custom rule.
func NewCustomRule(id, categoryID string, priority RulePriority, matchFunc CustomMatchFunc) *CustomRule {
	if priority == 0 {
		priority = PriorityNormal
	}

	return &CustomRule{
		id:         id,
		categoryID: categoryID,
		priority:   priority,
		enabled:    true,
		matchFunc:  matchFunc,
	}
}

func (r *CustomRule) ID() string          { return r.id }
func (r *CustomRule) Type() RuleType      { return RuleTypeCustom }
func (r *CustomRule) Priority() RulePriority { return r.priority }
func (r *CustomRule) CategoryID() string  { return r.categoryID }

func (r *CustomRule) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enabled
}

func (r *CustomRule) SetEnabled(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = enabled
}

func (r *CustomRule) Match(ctx context.Context, content *ContentData) []RuleMatch {
	r.mu.RLock()
	if !r.enabled || r.matchFunc == nil {
		r.mu.RUnlock()
		return nil
	}
	matchFunc := r.matchFunc
	r.mu.RUnlock()

	return matchFunc(ctx, content)
}

// =============================================================================
// Rule Engine
// =============================================================================

// RuleEngine manages and executes categorization rules.
type RuleEngine struct {
	mu    sync.RWMutex
	rules []Rule
}

// NewRuleEngine creates a new rule engine.
func NewRuleEngine() *RuleEngine {
	return &RuleEngine{
		rules: make([]Rule, 0),
	}
}

// AddRule adds a rule to the engine.
func (e *RuleEngine) AddRule(rule Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules = append(e.rules, rule)
	e.sortRules()
}

// AddRules adds multiple rules to the engine.
func (e *RuleEngine) AddRules(rules ...Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules = append(e.rules, rules...)
	e.sortRules()
}

// RemoveRule removes a rule by ID.
func (e *RuleEngine) RemoveRule(ruleID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i, r := range e.rules {
		if r.ID() == ruleID {
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			return true
		}
	}

	return false
}

// GetRule retrieves a rule by ID.
func (e *RuleEngine) GetRule(ruleID string) Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, r := range e.rules {
		if r.ID() == ruleID {
			return r
		}
	}

	return nil
}

// GetRulesForCategory returns all rules for a specific category.
func (e *RuleEngine) GetRulesForCategory(categoryID string) []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var rules []Rule
	for _, r := range e.rules {
		if r.CategoryID() == categoryID {
			rules = append(rules, r)
		}
	}

	return rules
}

// RuleCount returns the number of rules.
func (e *RuleEngine) RuleCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.rules)
}

// sortRules sorts rules by priority (highest first).
func (e *RuleEngine) sortRules() {
	sort.Slice(e.rules, func(i, j int) bool {
		return e.rules[i].Priority() > e.rules[j].Priority()
	})
}

// EvaluateOptions configures rule evaluation behavior.
type EvaluateOptions struct {
	// MaxMatches limits the number of matches returned (0 = unlimited).
	MaxMatches int

	// MinConfidence filters out matches below this threshold.
	MinConfidence float64

	// CategoryFilter only evaluates rules for these categories.
	CategoryFilter []string

	// StopOnFirstMatch stops after finding the first match.
	StopOnFirstMatch bool

	// AggregateByCategory combines matches for the same category.
	AggregateByCategory bool
}

// DefaultEvaluateOptions returns default evaluation options.
func DefaultEvaluateOptions() *EvaluateOptions {
	return &EvaluateOptions{
		MaxMatches:          0,
		MinConfidence:       0.0,
		AggregateByCategory: true,
	}
}

// EvaluateResult contains the results of rule evaluation.
type EvaluateResult struct {
	// Matches contains all rule matches.
	Matches []RuleMatch

	// RulesEvaluated is the number of rules evaluated.
	RulesEvaluated int

	// RulesMatched is the number of rules that matched.
	RulesMatched int

	// BestMatch is the highest confidence match.
	BestMatch *RuleMatch
}

// Evaluate runs all rules against the content.
func (e *RuleEngine) Evaluate(ctx context.Context, content *ContentData, opts *EvaluateOptions) *EvaluateResult {
	if opts == nil {
		opts = DefaultEvaluateOptions()
	}

	e.mu.RLock()
	rules := make([]Rule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	result := &EvaluateResult{
		Matches: make([]RuleMatch, 0),
	}

	// Build category filter set
	categoryFilter := make(map[string]bool)
	for _, c := range opts.CategoryFilter {
		categoryFilter[c] = true
	}

	for _, rule := range rules {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return result
		default:
		}

		// Skip disabled rules
		if !rule.IsEnabled() {
			continue
		}

		// Apply category filter
		if len(categoryFilter) > 0 && !categoryFilter[rule.CategoryID()] {
			continue
		}

		result.RulesEvaluated++

		matches := rule.Match(ctx, content)
		if len(matches) == 0 {
			continue
		}

		result.RulesMatched++

		for _, match := range matches {
			// Apply minimum confidence filter
			if match.Confidence < opts.MinConfidence {
				continue
			}

			result.Matches = append(result.Matches, match)

			// Track best match
			if result.BestMatch == nil || match.Confidence > result.BestMatch.Confidence {
				matchCopy := match
				result.BestMatch = &matchCopy
			}

			// Check stop condition
			if opts.StopOnFirstMatch {
				return result
			}

			// Check max matches
			if opts.MaxMatches > 0 && len(result.Matches) >= opts.MaxMatches {
				return result
			}
		}
	}

	// Aggregate by category if requested
	if opts.AggregateByCategory && len(result.Matches) > 1 {
		result.Matches = aggregateMatches(result.Matches)
	}

	// Sort matches by confidence (highest first)
	sort.Slice(result.Matches, func(i, j int) bool {
		return result.Matches[i].Confidence > result.Matches[j].Confidence
	})

	return result
}

// aggregateMatches combines matches for the same category.
func aggregateMatches(matches []RuleMatch) []RuleMatch {
	categoryMatches := make(map[string][]RuleMatch)

	for _, m := range matches {
		categoryMatches[m.CategoryID] = append(categoryMatches[m.CategoryID], m)
	}

	aggregated := make([]RuleMatch, 0, len(categoryMatches))
	for categoryID, catMatches := range categoryMatches {
		if len(catMatches) == 1 {
			aggregated = append(aggregated, catMatches[0])
			continue
		}

		// Combine matches for this category
		combined := RuleMatch{
			CategoryID:   categoryID,
			RuleID:       "aggregated",
			RuleType:     RuleTypeCustom,
			MatchedTerms: make([]string, 0),
		}

		// Collect all matched terms and calculate combined confidence
		termSet := make(map[string]bool)
		var totalConfidence float64
		for _, m := range catMatches {
			totalConfidence += m.Confidence
			for _, term := range m.MatchedTerms {
				if !termSet[term] {
					termSet[term] = true
					combined.MatchedTerms = append(combined.MatchedTerms, term)
				}
			}
		}

		// Use max confidence with a small boost for multiple matches
		maxConfidence := 0.0
		for _, m := range catMatches {
			if m.Confidence > maxConfidence {
				maxConfidence = m.Confidence
			}
		}
		combined.Confidence = maxConfidence + (0.05 * float64(len(catMatches)-1))
		if combined.Confidence > 1.0 {
			combined.Confidence = 1.0
		}

		aggregated = append(aggregated, combined)
	}

	return aggregated
}

// BuildRulesFromTaxonomy creates rules from a taxonomy's categories.
func BuildRulesFromTaxonomy(taxonomy *TaxonomyTree) ([]Rule, error) {
	if taxonomy == nil {
		return nil, ErrEmptyTaxonomy
	}

	categories := taxonomy.GetEnabledCategories()
	rules := make([]Rule, 0)

	for _, cat := range categories {
		// Create keyword rule if keywords are defined
		if len(cat.Keywords) > 0 {
			keywordRule := NewKeywordRule(&KeywordRuleConfig{
				ID:              cat.ID + "-keywords",
				CategoryID:      cat.ID,
				Priority:        PriorityNormal,
				Keywords:        cat.Keywords,
				MinMatches:      1,
				BaseConfidence:  0.5,
				ConfidenceBoost: 0.1,
				MatchTitle:      true,
			})
			rules = append(rules, keywordRule)
		}

		// Create pattern rules if patterns are defined
		if len(cat.Patterns) > 0 {
			patternRule, err := NewPatternRule(&PatternRuleConfig{
				ID:             cat.ID + "-patterns",
				CategoryID:     cat.ID,
				Priority:       PriorityHigh, // Patterns are more specific
				Patterns:       cat.Patterns,
				BaseConfidence: 0.7,
			})
			if err != nil {
				continue // Skip invalid patterns
			}
			rules = append(rules, patternRule)
		}
	}

	return rules, nil
}
