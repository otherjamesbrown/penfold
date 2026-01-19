// Package automation provides rule-based automation for the review service.
// It enables automatic processing of review items based on configurable rules,
// with support for learning from user decisions and A/B testing.
package automation

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Common errors for rule operations.
var (
	ErrRuleNotFound      = errors.New("rule not found")
	ErrRuleDisabled      = errors.New("rule is disabled")
	ErrInvalidRule       = errors.New("invalid rule configuration")
	ErrConditionNotMet   = errors.New("condition not met")
	ErrActionFailed      = errors.New("action execution failed")
	ErrConflictingRules  = errors.New("conflicting rules detected")
	ErrMaxActionsReached = errors.New("maximum actions per item reached")
)

// ActionType represents the type of action to take on a review item.
type ActionType string

const (
	// ActionAccept automatically accepts/approves the item.
	ActionAccept ActionType = "accept"

	// ActionReject automatically rejects the item.
	ActionReject ActionType = "reject"

	// ActionDefer defers the item for later review.
	ActionDefer ActionType = "defer"

	// ActionEscalate escalates the item to a higher priority queue.
	ActionEscalate ActionType = "escalate"

	// ActionTag adds tags to the item for categorization.
	ActionTag ActionType = "tag"

	// ActionNotify sends a notification about the item.
	ActionNotify ActionType = "notify"

	// ActionRoute routes the item to a specific reviewer or queue.
	ActionRoute ActionType = "route"
)

// ValidActionTypes returns all valid action types.
func ValidActionTypes() []ActionType {
	return []ActionType{
		ActionAccept,
		ActionReject,
		ActionDefer,
		ActionEscalate,
		ActionTag,
		ActionNotify,
		ActionRoute,
	}
}

// IsValid returns true if the action type is valid.
func (a ActionType) IsValid() bool {
	for _, valid := range ValidActionTypes() {
		if a == valid {
			return true
		}
	}
	return false
}

// ConditionType represents the type of condition to evaluate.
type ConditionType string

const (
	// ConditionPattern matches content against a regex pattern.
	ConditionPattern ConditionType = "pattern"

	// ConditionSender matches based on sender identity.
	ConditionSender ConditionType = "sender"

	// ConditionCategory matches based on item category.
	ConditionCategory ConditionType = "category"

	// ConditionKeywords matches based on presence of keywords.
	ConditionKeywords ConditionType = "keywords"

	// ConditionConfidence matches based on AI confidence score.
	ConditionConfidence ConditionType = "confidence"

	// ConditionSource matches based on item source.
	ConditionSource ConditionType = "source"

	// ConditionContentType matches based on content type.
	ConditionContentType ConditionType = "content_type"

	// ConditionPriority matches based on item priority level.
	ConditionPriority ConditionType = "priority"

	// ConditionAge matches based on item age.
	ConditionAge ConditionType = "age"

	// ConditionAnd combines multiple conditions with AND logic.
	ConditionAnd ConditionType = "and"

	// ConditionOr combines multiple conditions with OR logic.
	ConditionOr ConditionType = "or"

	// ConditionNot negates a condition.
	ConditionNot ConditionType = "not"

	// ConditionThreshold matches based on numeric thresholds.
	ConditionThreshold ConditionType = "threshold"
)

// Condition represents a rule condition that must be met for the rule to apply.
type Condition struct {
	// Type is the condition type.
	Type ConditionType `json:"type"`

	// Field is the field to evaluate (for field-based conditions).
	Field string `json:"field,omitempty"`

	// Operator is the comparison operator (eq, neq, gt, gte, lt, lte, contains, matches).
	Operator string `json:"operator,omitempty"`

	// Value is the value to compare against.
	Value interface{} `json:"value,omitempty"`

	// Pattern is the regex pattern for pattern matching.
	Pattern string `json:"pattern,omitempty"`

	// Keywords is the list of keywords to match.
	Keywords []string `json:"keywords,omitempty"`

	// MinConfidence is the minimum confidence score (0.0-1.0).
	MinConfidence float64 `json:"min_confidence,omitempty"`

	// MaxConfidence is the maximum confidence score (0.0-1.0).
	MaxConfidence float64 `json:"max_confidence,omitempty"`

	// Threshold is a numeric threshold value.
	Threshold float64 `json:"threshold,omitempty"`

	// SubConditions holds nested conditions for AND/OR/NOT.
	SubConditions []Condition `json:"sub_conditions,omitempty"`

	// CaseSensitive indicates if string matching should be case-sensitive.
	CaseSensitive bool `json:"case_sensitive,omitempty"`
}

// Action represents an action to be taken when a rule matches.
type Action struct {
	// ID is the unique identifier for this action instance.
	ID string `json:"id"`

	// Type is the action type.
	Type ActionType `json:"type"`

	// RuleID is the ID of the rule that triggered this action.
	RuleID string `json:"rule_id"`

	// ItemID is the ID of the review item this action applies to.
	ItemID string `json:"item_id"`

	// Parameters contains action-specific parameters.
	Parameters map[string]interface{} `json:"parameters,omitempty"`

	// Tags is the list of tags to add (for tag action).
	Tags []string `json:"tags,omitempty"`

	// DeferDuration is how long to defer (for defer action).
	DeferDuration time.Duration `json:"defer_duration,omitempty"`

	// EscalationLevel is the escalation priority level.
	EscalationLevel int `json:"escalation_level,omitempty"`

	// RouteTo is the target for routing actions.
	RouteTo string `json:"route_to,omitempty"`

	// NotificationTarget is the target for notifications.
	NotificationTarget string `json:"notification_target,omitempty"`

	// Message is a human-readable description of the action.
	Message string `json:"message,omitempty"`

	// Executed indicates if this action has been executed.
	Executed bool `json:"executed"`

	// ExecutedAt is when the action was executed.
	ExecutedAt *time.Time `json:"executed_at,omitempty"`

	// Error holds any error from execution.
	Error string `json:"error,omitempty"`

	// DryRun indicates this is a dry-run (no actual changes).
	DryRun bool `json:"dry_run"`

	// CreatedAt is when this action was created.
	CreatedAt time.Time `json:"created_at"`
}

// NewAction creates a new Action with the given type and rule ID.
func NewAction(actionType ActionType, ruleID, itemID string) *Action {
	return &Action{
		ID:         uuid.New().String(),
		Type:       actionType,
		RuleID:     ruleID,
		ItemID:     itemID,
		Parameters: make(map[string]interface{}),
		CreatedAt:  time.Now().UTC(),
	}
}

// Rule represents an automation rule for processing review items.
type Rule struct {
	// ID is the unique identifier for the rule.
	ID string `json:"id"`

	// TenantID is the tenant this rule belongs to.
	TenantID string `json:"tenant_id"`

	// Name is a human-readable name for the rule.
	Name string `json:"name"`

	// Description describes what the rule does.
	Description string `json:"description,omitempty"`

	// Condition is the condition that must be met for the rule to trigger.
	Condition Condition `json:"condition"`

	// ActionType is the type of action to take when the rule matches.
	ActionType ActionType `json:"action_type"`

	// ActionParameters contains parameters for the action.
	ActionParameters map[string]interface{} `json:"action_parameters,omitempty"`

	// Priority determines rule evaluation order (higher = evaluated first).
	Priority int `json:"priority"`

	// Enabled indicates if the rule is active.
	Enabled bool `json:"enabled"`

	// StopOnMatch indicates if rule processing should stop after this rule matches.
	StopOnMatch bool `json:"stop_on_match"`

	// CreatedAt is when the rule was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the rule was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// CreatedBy is the user who created the rule.
	CreatedBy string `json:"created_by,omitempty"`

	// Source indicates how the rule was created (manual, learned, imported).
	Source string `json:"source,omitempty"`

	// Metadata holds additional rule metadata.
	Metadata map[string]string `json:"metadata,omitempty"`

	// ABTestGroup is the A/B test group this rule belongs to (if any).
	ABTestGroup string `json:"ab_test_group,omitempty"`

	// ABTestVariant is the variant within the A/B test.
	ABTestVariant string `json:"ab_test_variant,omitempty"`

	// Tags are labels for organizing rules.
	Tags []string `json:"tags,omitempty"`
}

// NewRule creates a new Rule with the given parameters.
func NewRule(tenantID, name string, condition Condition, actionType ActionType) *Rule {
	now := time.Now().UTC()
	return &Rule{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		Name:             name,
		Condition:        condition,
		ActionType:       actionType,
		ActionParameters: make(map[string]interface{}),
		Priority:         0,
		Enabled:          true,
		CreatedAt:        now,
		UpdatedAt:        now,
		Source:           "manual",
		Metadata:         make(map[string]string),
		Tags:             make([]string, 0),
	}
}

// Validate checks if the rule configuration is valid.
func (r *Rule) Validate() error {
	if r.ID == "" {
		return errors.New("rule ID is required")
	}
	if r.TenantID == "" {
		return errors.New("tenant ID is required")
	}
	if r.Name == "" {
		return errors.New("rule name is required")
	}
	if !r.ActionType.IsValid() {
		return errors.New("invalid action type")
	}
	return nil
}

// Clone creates a deep copy of the rule.
func (r *Rule) Clone() *Rule {
	clone := &Rule{
		ID:               r.ID,
		TenantID:         r.TenantID,
		Name:             r.Name,
		Description:      r.Description,
		Condition:        r.Condition, // Note: shallow copy of condition
		ActionType:       r.ActionType,
		ActionParameters: make(map[string]interface{}, len(r.ActionParameters)),
		Priority:         r.Priority,
		Enabled:          r.Enabled,
		StopOnMatch:      r.StopOnMatch,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		CreatedBy:        r.CreatedBy,
		Source:           r.Source,
		Metadata:         make(map[string]string, len(r.Metadata)),
		ABTestGroup:      r.ABTestGroup,
		ABTestVariant:    r.ABTestVariant,
		Tags:             make([]string, len(r.Tags)),
	}

	for k, v := range r.ActionParameters {
		clone.ActionParameters[k] = v
	}
	for k, v := range r.Metadata {
		clone.Metadata[k] = v
	}
	copy(clone.Tags, r.Tags)

	return clone
}

// ToJSON serializes the rule to JSON.
func (r *Rule) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// RuleFromJSON deserializes a rule from JSON.
func RuleFromJSON(data []byte) (*Rule, error) {
	var rule Rule
	if err := json.Unmarshal(data, &rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

// RuleMetrics holds performance metrics for a rule.
type RuleMetrics struct {
	// RuleID is the rule these metrics are for.
	RuleID string `json:"rule_id"`

	// TenantID is the tenant these metrics belong to.
	TenantID string `json:"tenant_id"`

	// EvaluationCount is the total number of times the rule was evaluated.
	EvaluationCount int64 `json:"evaluation_count"`

	// MatchCount is the number of times the rule matched.
	MatchCount int64 `json:"match_count"`

	// ActionExecutedCount is the number of times the action was executed.
	ActionExecutedCount int64 `json:"action_executed_count"`

	// ActionErrorCount is the number of times the action failed.
	ActionErrorCount int64 `json:"action_error_count"`

	// MatchRate is the percentage of evaluations that matched.
	MatchRate float64 `json:"match_rate"`

	// SuccessRate is the percentage of executed actions that succeeded.
	SuccessRate float64 `json:"success_rate"`

	// AverageEvaluationTimeMs is the average time to evaluate the rule.
	AverageEvaluationTimeMs float64 `json:"average_evaluation_time_ms"`

	// TotalEvaluationTimeMs is the total time spent evaluating the rule.
	TotalEvaluationTimeMs int64 `json:"total_evaluation_time_ms"`

	// FeedbackPositive is the count of positive user feedback for actions.
	FeedbackPositive int64 `json:"feedback_positive"`

	// FeedbackNegative is the count of negative user feedback for actions.
	FeedbackNegative int64 `json:"feedback_negative"`

	// OverrideCount is the number of times users overrode this rule's action.
	OverrideCount int64 `json:"override_count"`

	// LastEvaluatedAt is when the rule was last evaluated.
	LastEvaluatedAt *time.Time `json:"last_evaluated_at,omitempty"`

	// LastMatchedAt is when the rule last matched.
	LastMatchedAt *time.Time `json:"last_matched_at,omitempty"`

	// PeriodStart is the start of the metrics period.
	PeriodStart time.Time `json:"period_start"`

	// PeriodEnd is the end of the metrics period.
	PeriodEnd time.Time `json:"period_end"`
}

// NewRuleMetrics creates a new RuleMetrics for the given rule.
func NewRuleMetrics(ruleID, tenantID string) *RuleMetrics {
	now := time.Now().UTC()
	return &RuleMetrics{
		RuleID:      ruleID,
		TenantID:    tenantID,
		PeriodStart: now,
		PeriodEnd:   now,
	}
}

// UpdateRates recalculates derived rates.
func (m *RuleMetrics) UpdateRates() {
	if m.EvaluationCount > 0 {
		m.MatchRate = float64(m.MatchCount) / float64(m.EvaluationCount) * 100
		m.AverageEvaluationTimeMs = float64(m.TotalEvaluationTimeMs) / float64(m.EvaluationCount)
	}
	if m.ActionExecutedCount > 0 {
		successCount := m.ActionExecutedCount - m.ActionErrorCount
		m.SuccessRate = float64(successCount) / float64(m.ActionExecutedCount) * 100
	}
}

// RecordEvaluation records a rule evaluation.
func (m *RuleMetrics) RecordEvaluation(matched bool, evaluationTimeMs int64) {
	m.EvaluationCount++
	m.TotalEvaluationTimeMs += evaluationTimeMs

	if matched {
		m.MatchCount++
		now := time.Now().UTC()
		m.LastMatchedAt = &now
	}

	now := time.Now().UTC()
	m.LastEvaluatedAt = &now
	m.PeriodEnd = now
	m.UpdateRates()
}

// RecordActionExecution records an action execution result.
func (m *RuleMetrics) RecordActionExecution(success bool) {
	m.ActionExecutedCount++
	if !success {
		m.ActionErrorCount++
	}
	m.UpdateRates()
}

// RecordFeedback records user feedback on an automated action.
func (m *RuleMetrics) RecordFeedback(positive bool) {
	if positive {
		m.FeedbackPositive++
	} else {
		m.FeedbackNegative++
	}
}

// RecordOverride records a user override of an automated action.
func (m *RuleMetrics) RecordOverride() {
	m.OverrideCount++
}

// RuleSet represents a collection of rules for a tenant.
type RuleSet struct {
	// TenantID is the tenant this rule set belongs to.
	TenantID string `json:"tenant_id"`

	// Rules is the list of rules in this set.
	Rules []*Rule `json:"rules"`

	// Enabled indicates if the rule set is active.
	Enabled bool `json:"enabled"`

	// MaxActionsPerItem limits actions per item to prevent loops.
	MaxActionsPerItem int `json:"max_actions_per_item"`

	// ConflictResolution defines how to handle conflicting rules.
	// Options: "first", "highest_priority", "merge", "fail"
	ConflictResolution string `json:"conflict_resolution"`

	// DefaultDryRun indicates if all rules should run in dry-run mode by default.
	DefaultDryRun bool `json:"default_dry_run"`
}

// NewRuleSet creates a new RuleSet for a tenant.
func NewRuleSet(tenantID string) *RuleSet {
	return &RuleSet{
		TenantID:           tenantID,
		Rules:              make([]*Rule, 0),
		Enabled:            true,
		MaxActionsPerItem:  10,
		ConflictResolution: "highest_priority",
		DefaultDryRun:      false,
	}
}

// AddRule adds a rule to the set and sorts by priority.
func (rs *RuleSet) AddRule(rule *Rule) {
	rs.Rules = append(rs.Rules, rule)
	rs.SortByPriority()
}

// RemoveRule removes a rule from the set.
func (rs *RuleSet) RemoveRule(ruleID string) bool {
	for i, rule := range rs.Rules {
		if rule.ID == ruleID {
			rs.Rules = append(rs.Rules[:i], rs.Rules[i+1:]...)
			return true
		}
	}
	return false
}

// GetRule returns a rule by ID.
func (rs *RuleSet) GetRule(ruleID string) *Rule {
	for _, rule := range rs.Rules {
		if rule.ID == ruleID {
			return rule
		}
	}
	return nil
}

// GetEnabledRules returns only enabled rules sorted by priority.
func (rs *RuleSet) GetEnabledRules() []*Rule {
	enabled := make([]*Rule, 0, len(rs.Rules))
	for _, rule := range rs.Rules {
		if rule.Enabled {
			enabled = append(enabled, rule)
		}
	}
	return enabled
}

// SortByPriority sorts rules by priority (highest first).
func (rs *RuleSet) SortByPriority() {
	// Simple bubble sort for typically small rule sets
	n := len(rs.Rules)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if rs.Rules[j].Priority < rs.Rules[j+1].Priority {
				rs.Rules[j], rs.Rules[j+1] = rs.Rules[j+1], rs.Rules[j]
			}
		}
	}
}
