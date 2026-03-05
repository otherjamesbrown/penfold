package registry

import (
	"strconv"
	"strings"
	"time"
)

// RoutingRule defines how tasks are routed to models based on task type.
// It maps directly to the ai_routing_rules database table.
type RoutingRule struct {
	// ID is the unique identifier for this rule (UUID).
	ID string `json:"id"`

	// Name is the unique human-readable name for the rule.
	Name string `json:"name"`

	// TaskType is the task category this rule applies to.
	// One of: embedding, summarization, extraction, classification
	TaskType string `json:"task_type"`

	// PreferredModels is an ordered list of model IDs to try first.
	PreferredModels []string `json:"preferred_models"`

	// FallbackModels are models to use when preferred ones are unavailable.
	FallbackModels []string `json:"fallback_models"`

	// RequireLocal if true, only local models (ollama, mlx) are considered.
	RequireLocal bool `json:"require_local"`

	// MaxCostPerRequest is an optional cost ceiling per request in USD.
	// Nil means unlimited.
	MaxCostPerRequest *float64 `json:"max_cost_per_request,omitempty"`

	// OptimizationMode determines the selection strategy.
	// One of: latency, quality, cost, balanced
	OptimizationMode string `json:"optimization_mode"`

	// Priority determines rule evaluation order (higher = checked first).
	Priority int `json:"priority"`

	// IsEnabled controls whether this rule is active.
	IsEnabled bool `json:"is_enabled"`

	// Conditions is a map of field names to allowed values for conditional matching.
	// All fields must match (AND), values within a field are alternatives (OR).
	// Empty/nil conditions means this is a default rule (matches everything).
	Conditions map[string][]string `json:"conditions,omitempty"`

	// CreatedAt is when this rule was created.
	CreatedAt time.Time `json:"created_at"`
}

// TaskType constants match the database CHECK constraint.
const (
	TaskTypeEmbedding       = "embedding"
	TaskTypeSummarization   = "summarization"
	TaskTypeExtraction      = "extraction"
	TaskTypeClassification  = "classification"
	TaskTypeDeepAnalysis    = "deep_analysis"
)

// OptimizationMode constants match the database CHECK constraint.
const (
	OptimizationModeLatency  = "latency"
	OptimizationModeQuality  = "quality"
	OptimizationModeCost     = "cost"
	OptimizationModeBalanced = "balanced"
)

// Validate checks if the routing rule is valid.
func (r *RoutingRule) Validate() error {
	if r.Name == "" {
		return ErrInvalidRoutingRule
	}
	if r.TaskType == "" {
		return ErrInvalidRoutingRule
	}
	// Validate task type
	switch r.TaskType {
	case TaskTypeEmbedding, TaskTypeSummarization, TaskTypeExtraction, TaskTypeClassification, TaskTypeDeepAnalysis:
		// Valid
	default:
		return ErrInvalidRoutingRule
	}
	// Validate optimization mode
	switch r.OptimizationMode {
	case OptimizationModeLatency, OptimizationModeQuality, OptimizationModeCost, OptimizationModeBalanced, "":
		// Valid (empty defaults to balanced in DB)
	default:
		return ErrInvalidRoutingRule
	}
	if len(r.PreferredModels) == 0 {
		return ErrInvalidRoutingRule
	}
	return nil
}

// Clone creates a deep copy of the routing rule.
func (r *RoutingRule) Clone() *RoutingRule {
	clone := *r
	clone.PreferredModels = make([]string, len(r.PreferredModels))
	copy(clone.PreferredModels, r.PreferredModels)
	clone.FallbackModels = make([]string, len(r.FallbackModels))
	copy(clone.FallbackModels, r.FallbackModels)
	if r.MaxCostPerRequest != nil {
		cost := *r.MaxCostPerRequest
		clone.MaxCostPerRequest = &cost
	}
	if r.Conditions != nil {
		clone.Conditions = make(map[string][]string, len(r.Conditions))
		for k, v := range r.Conditions {
			vals := make([]string, len(v))
			copy(vals, v)
			clone.Conditions[k] = vals
		}
	}
	return &clone
}

// MatchesConditions checks if the given attributes satisfy a rule's conditions.
// Semantics: all condition fields must match (AND), values within a field are
// alternatives (OR). Empty/nil conditions = matches everything (default rule).
//
// Numeric range conditions: if the condition key starts with "min_" or "max_",
// the key is stripped of its prefix to look up the attribute value, and a numeric
// comparison is performed. For example, condition key "max_content_chars" looks up
// attrs["content_chars"] and checks actual <= threshold.
func MatchesConditions(conditions map[string][]string, attrs map[string]string) bool {
	if len(conditions) == 0 {
		return true
	}
	for field, allowedValues := range conditions {
		// Numeric range conditions (min_/max_ prefix).
		if strings.HasPrefix(field, "min_") || strings.HasPrefix(field, "max_") {
			var prefix, attrKey string
			if strings.HasPrefix(field, "min_") {
				prefix = "min_"
				attrKey = field[len("min_"):]
			} else {
				prefix = "max_"
				attrKey = field[len("max_"):]
			}
			actual, ok := attrs[attrKey]
			if !ok {
				return false
			}
			actualVal, err := strconv.ParseInt(actual, 10, 64)
			if err != nil {
				return false
			}
			if len(allowedValues) == 0 {
				return false
			}
			thresholdVal, err := strconv.ParseInt(allowedValues[0], 10, 64)
			if err != nil {
				return false
			}
			if prefix == "max_" && actualVal > thresholdVal {
				return false
			}
			if prefix == "min_" && actualVal < thresholdVal {
				return false
			}
			continue
		}

		// String equality conditions (existing behaviour).
		actual, ok := attrs[field]
		if !ok {
			return false
		}
		found := false
		for _, v := range allowedValues {
			if strings.EqualFold(actual, v) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
