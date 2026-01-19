// Package pattern provides pattern detection and learning for the review service.
// It enables automatic pattern discovery from user behavior and integrates with
// the automation engine for rule suggestions.
package pattern

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Common errors for pattern operations.
var (
	ErrPatternNotFound     = errors.New("pattern not found")
	ErrInvalidPattern      = errors.New("invalid pattern configuration")
	ErrPatternExists       = errors.New("pattern already exists")
	ErrInsufficientData    = errors.New("insufficient data for pattern detection")
	ErrConditionNotMatched = errors.New("condition not matched")
)

// PatternType represents the type of pattern being detected.
type PatternType string

const (
	// PatternSender detects patterns based on message sender.
	PatternSender PatternType = "sender"

	// PatternTopic detects patterns based on content topic or category.
	PatternTopic PatternType = "topic"

	// PatternTime detects patterns based on time of day/week.
	PatternTime PatternType = "time"

	// PatternContent detects patterns based on content characteristics.
	PatternContent PatternType = "content"

	// PatternAction detects patterns based on user actions taken.
	PatternAction PatternType = "action"

	// PatternSource detects patterns based on content source.
	PatternSource PatternType = "source"

	// PatternPriority detects patterns based on priority levels.
	PatternPriority PatternType = "priority"

	// PatternComposite combines multiple pattern types.
	PatternComposite PatternType = "composite"
)

// ValidPatternTypes returns all valid pattern types.
func ValidPatternTypes() []PatternType {
	return []PatternType{
		PatternSender,
		PatternTopic,
		PatternTime,
		PatternContent,
		PatternAction,
		PatternSource,
		PatternPriority,
		PatternComposite,
	}
}

// IsValid returns true if the pattern type is valid.
func (pt PatternType) IsValid() bool {
	for _, valid := range ValidPatternTypes() {
		if pt == valid {
			return true
		}
	}
	return false
}

// MatchOperator represents the comparison operator for pattern matching.
type MatchOperator string

const (
	OpEquals     MatchOperator = "eq"
	OpNotEquals  MatchOperator = "neq"
	OpContains   MatchOperator = "contains"
	OpStartsWith MatchOperator = "starts_with"
	OpEndsWith   MatchOperator = "ends_with"
	OpMatches    MatchOperator = "matches"
	OpGreaterThan MatchOperator = "gt"
	OpGreaterOrEqual MatchOperator = "gte"
	OpLessThan   MatchOperator = "lt"
	OpLessOrEqual MatchOperator = "lte"
	OpIn         MatchOperator = "in"
	OpNotIn      MatchOperator = "not_in"
	OpBetween    MatchOperator = "between"
)

// PatternCondition represents a single condition that must be met for a pattern to match.
type PatternCondition struct {
	// Field is the field to evaluate.
	Field string `json:"field"`

	// Operator is the comparison operator.
	Operator MatchOperator `json:"operator"`

	// Value is the expected value or values.
	Value interface{} `json:"value"`

	// CaseSensitive indicates if string matching should be case-sensitive.
	CaseSensitive bool `json:"case_sensitive,omitempty"`

	// Weight determines the importance of this condition in scoring (0.0-1.0).
	Weight float64 `json:"weight,omitempty"`
}

// NewPatternCondition creates a new PatternCondition.
func NewPatternCondition(field string, operator MatchOperator, value interface{}) PatternCondition {
	return PatternCondition{
		Field:    field,
		Operator: operator,
		Value:    value,
		Weight:   1.0,
	}
}

// PatternExample represents an example that demonstrates the pattern.
type PatternExample struct {
	// ItemID is the ID of the example review item.
	ItemID string `json:"item_id"`

	// Timestamp is when this example was recorded.
	Timestamp time.Time `json:"timestamp"`

	// Action is the action that was taken on this item.
	Action string `json:"action"`

	// Metadata holds additional context about the example.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// PatternStats holds statistical information about a pattern.
type PatternStats struct {
	// TotalMatches is the total number of times the pattern matched.
	TotalMatches int64 `json:"total_matches"`

	// SuccessfulActions is the number of correct action predictions.
	SuccessfulActions int64 `json:"successful_actions"`

	// OverriddenActions is the number of times users overrode the predicted action.
	OverriddenActions int64 `json:"overridden_actions"`

	// AverageProcessingTimeMs is the average time to evaluate this pattern.
	AverageProcessingTimeMs float64 `json:"average_processing_time_ms"`

	// LastMatchedAt is when the pattern last matched.
	LastMatchedAt *time.Time `json:"last_matched_at,omitempty"`

	// FirstSeenAt is when the pattern was first detected.
	FirstSeenAt time.Time `json:"first_seen_at"`

	// TrendDirection indicates if pattern frequency is increasing (1), stable (0), or decreasing (-1).
	TrendDirection int `json:"trend_direction"`
}

// Pattern represents a detected behavioral pattern.
type Pattern struct {
	// ID is the unique identifier for this pattern.
	ID string `json:"id"`

	// TenantID is the tenant this pattern belongs to.
	TenantID string `json:"tenant_id"`

	// Name is a human-readable name for the pattern.
	Name string `json:"name"`

	// Description describes what the pattern represents.
	Description string `json:"description,omitempty"`

	// Type is the pattern type.
	Type PatternType `json:"type"`

	// Conditions are the conditions that must match for this pattern.
	Conditions []PatternCondition `json:"conditions"`

	// Frequency indicates how often this pattern occurs (matches per day).
	Frequency float64 `json:"frequency"`

	// Confidence is the confidence score for this pattern (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// SuggestedAction is the recommended action when this pattern matches.
	SuggestedAction string `json:"suggested_action,omitempty"`

	// Examples are sample items that match this pattern.
	Examples []PatternExample `json:"examples,omitempty"`

	// Stats holds statistical information about the pattern.
	Stats PatternStats `json:"stats"`

	// Enabled indicates if the pattern is active for detection.
	Enabled bool `json:"enabled"`

	// AutoGenerated indicates if this pattern was automatically discovered.
	AutoGenerated bool `json:"auto_generated"`

	// Source indicates how the pattern was created (manual, learned, imported).
	Source string `json:"source,omitempty"`

	// Priority determines evaluation order (higher = evaluated first).
	Priority int `json:"priority"`

	// Tags are labels for organizing patterns.
	Tags []string `json:"tags,omitempty"`

	// RelatedPatterns contains IDs of related patterns.
	RelatedPatterns []string `json:"related_patterns,omitempty"`

	// Metadata holds additional pattern data.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// CreatedAt is when the pattern was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the pattern was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// CreatedBy is the user who created the pattern.
	CreatedBy string `json:"created_by,omitempty"`
}

// NewPattern creates a new Pattern with the given parameters.
func NewPattern(tenantID, name string, patternType PatternType, conditions []PatternCondition) *Pattern {
	now := time.Now().UTC()
	return &Pattern{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		Name:       name,
		Type:       patternType,
		Conditions: conditions,
		Confidence: 0.0,
		Frequency:  0.0,
		Examples:   make([]PatternExample, 0),
		Stats: PatternStats{
			FirstSeenAt: now,
		},
		Enabled:       true,
		AutoGenerated: false,
		Source:        "manual",
		Priority:      0,
		Tags:          make([]string, 0),
		Metadata:      make(map[string]interface{}),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// Validate checks if the pattern configuration is valid.
func (p *Pattern) Validate() error {
	if p.ID == "" {
		return errors.New("pattern ID is required")
	}
	if p.TenantID == "" {
		return errors.New("tenant ID is required")
	}
	if p.Name == "" {
		return errors.New("pattern name is required")
	}
	if !p.Type.IsValid() {
		return errors.New("invalid pattern type")
	}
	if len(p.Conditions) == 0 {
		return errors.New("at least one condition is required")
	}
	return nil
}

// Clone creates a deep copy of the pattern.
func (p *Pattern) Clone() *Pattern {
	clone := &Pattern{
		ID:              p.ID,
		TenantID:        p.TenantID,
		Name:            p.Name,
		Description:     p.Description,
		Type:            p.Type,
		Conditions:      make([]PatternCondition, len(p.Conditions)),
		Frequency:       p.Frequency,
		Confidence:      p.Confidence,
		SuggestedAction: p.SuggestedAction,
		Examples:        make([]PatternExample, len(p.Examples)),
		Stats:           p.Stats,
		Enabled:         p.Enabled,
		AutoGenerated:   p.AutoGenerated,
		Source:          p.Source,
		Priority:        p.Priority,
		Tags:            make([]string, len(p.Tags)),
		RelatedPatterns: make([]string, len(p.RelatedPatterns)),
		Metadata:        make(map[string]interface{}, len(p.Metadata)),
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
		CreatedBy:       p.CreatedBy,
	}

	copy(clone.Conditions, p.Conditions)
	copy(clone.Examples, p.Examples)
	copy(clone.Tags, p.Tags)
	copy(clone.RelatedPatterns, p.RelatedPatterns)
	for k, v := range p.Metadata {
		clone.Metadata[k] = v
	}

	return clone
}

// AddExample adds an example to the pattern.
func (p *Pattern) AddExample(example PatternExample) {
	p.Examples = append(p.Examples, example)
	p.UpdatedAt = time.Now().UTC()
}

// UpdateStats updates the pattern statistics.
func (p *Pattern) UpdateStats(matched bool, overridden bool, processingTimeMs int64) {
	p.Stats.TotalMatches++

	if matched && !overridden {
		p.Stats.SuccessfulActions++
	}
	if overridden {
		p.Stats.OverriddenActions++
	}

	// Update average processing time
	n := float64(p.Stats.TotalMatches)
	p.Stats.AverageProcessingTimeMs = ((n-1)*p.Stats.AverageProcessingTimeMs + float64(processingTimeMs)) / n

	now := time.Now().UTC()
	p.Stats.LastMatchedAt = &now
	p.UpdatedAt = now
}

// CalculateConfidence calculates the confidence score based on stats.
func (p *Pattern) CalculateConfidence() float64 {
	if p.Stats.TotalMatches == 0 {
		return 0.0
	}

	// Base confidence from success rate
	totalActions := p.Stats.SuccessfulActions + p.Stats.OverriddenActions
	if totalActions == 0 {
		return 0.5 // Neutral confidence if no feedback
	}

	successRate := float64(p.Stats.SuccessfulActions) / float64(totalActions)

	// Adjust for sample size (more matches = more confident)
	sampleSizeFactor := 1.0 - (1.0 / (1.0 + float64(p.Stats.TotalMatches)/10.0))

	p.Confidence = successRate * sampleSizeFactor
	return p.Confidence
}

// ToJSON serializes the pattern to JSON.
func (p *Pattern) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}

// PatternFromJSON deserializes a pattern from JSON.
func PatternFromJSON(data []byte) (*Pattern, error) {
	var pattern Pattern
	if err := json.Unmarshal(data, &pattern); err != nil {
		return nil, err
	}
	return &pattern, nil
}

// PatternSuggestion represents a suggested pattern based on detected behavior.
type PatternSuggestion struct {
	// ID is a unique identifier for this suggestion.
	ID string `json:"id"`

	// TenantID is the tenant this suggestion is for.
	TenantID string `json:"tenant_id"`

	// Pattern is the suggested pattern.
	Pattern *Pattern `json:"pattern"`

	// Confidence is the confidence score for this suggestion (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// SupportCount is the number of observations supporting this pattern.
	SupportCount int `json:"support_count"`

	// Explanation describes why this pattern was suggested.
	Explanation string `json:"explanation"`

	// PotentialImpact estimates how many items this pattern would affect.
	PotentialImpact int `json:"potential_impact"`

	// Status is the current status (pending, approved, rejected, deployed).
	Status string `json:"status"`

	// CreatedAt is when this suggestion was created.
	CreatedAt time.Time `json:"created_at"`

	// ReviewedAt is when this suggestion was reviewed.
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`

	// ReviewedBy is who reviewed this suggestion.
	ReviewedBy string `json:"reviewed_by,omitempty"`
}

// NewPatternSuggestion creates a new PatternSuggestion.
func NewPatternSuggestion(tenantID string, pattern *Pattern, confidence float64, supportCount int) *PatternSuggestion {
	return &PatternSuggestion{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		Pattern:      pattern,
		Confidence:   confidence,
		SupportCount: supportCount,
		Status:       "pending",
		CreatedAt:    time.Now().UTC(),
	}
}

// Approve marks the suggestion as approved.
func (s *PatternSuggestion) Approve(approvedBy string) {
	s.Status = "approved"
	now := time.Now().UTC()
	s.ReviewedAt = &now
	s.ReviewedBy = approvedBy
}

// Reject marks the suggestion as rejected.
func (s *PatternSuggestion) Reject(rejectedBy string) {
	s.Status = "rejected"
	now := time.Now().UTC()
	s.ReviewedAt = &now
	s.ReviewedBy = rejectedBy
}

// Deploy marks the suggestion as deployed.
func (s *PatternSuggestion) Deploy() {
	s.Status = "deployed"
}

// PatternSet represents a collection of patterns for a tenant.
type PatternSet struct {
	// TenantID is the tenant this pattern set belongs to.
	TenantID string `json:"tenant_id"`

	// Patterns is the list of patterns in this set.
	Patterns []*Pattern `json:"patterns"`

	// Enabled indicates if the pattern set is active.
	Enabled bool `json:"enabled"`
}

// NewPatternSet creates a new PatternSet for a tenant.
func NewPatternSet(tenantID string) *PatternSet {
	return &PatternSet{
		TenantID: tenantID,
		Patterns: make([]*Pattern, 0),
		Enabled:  true,
	}
}

// AddPattern adds a pattern to the set.
func (ps *PatternSet) AddPattern(pattern *Pattern) {
	ps.Patterns = append(ps.Patterns, pattern)
	ps.sortByPriority()
}

// RemovePattern removes a pattern from the set.
func (ps *PatternSet) RemovePattern(patternID string) bool {
	for i, p := range ps.Patterns {
		if p.ID == patternID {
			ps.Patterns = append(ps.Patterns[:i], ps.Patterns[i+1:]...)
			return true
		}
	}
	return false
}

// GetPattern returns a pattern by ID.
func (ps *PatternSet) GetPattern(patternID string) *Pattern {
	for _, p := range ps.Patterns {
		if p.ID == patternID {
			return p
		}
	}
	return nil
}

// GetEnabledPatterns returns only enabled patterns sorted by priority.
func (ps *PatternSet) GetEnabledPatterns() []*Pattern {
	enabled := make([]*Pattern, 0, len(ps.Patterns))
	for _, p := range ps.Patterns {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}
	return enabled
}

// GetPatternsByType returns patterns of a specific type.
func (ps *PatternSet) GetPatternsByType(patternType PatternType) []*Pattern {
	result := make([]*Pattern, 0)
	for _, p := range ps.Patterns {
		if p.Type == patternType {
			result = append(result, p)
		}
	}
	return result
}

// sortByPriority sorts patterns by priority (highest first).
func (ps *PatternSet) sortByPriority() {
	n := len(ps.Patterns)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if ps.Patterns[j].Priority < ps.Patterns[j+1].Priority {
				ps.Patterns[j], ps.Patterns[j+1] = ps.Patterns[j+1], ps.Patterns[j]
			}
		}
	}
}
