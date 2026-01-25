package registry

import "time"

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

	// CreatedAt is when this rule was created.
	CreatedAt time.Time `json:"created_at"`
}

// TaskType constants match the database CHECK constraint.
const (
	TaskTypeEmbedding       = "embedding"
	TaskTypeSummarization   = "summarization"
	TaskTypeExtraction      = "extraction"
	TaskTypeClassification  = "classification"
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
	case TaskTypeEmbedding, TaskTypeSummarization, TaskTypeExtraction, TaskTypeClassification:
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
	return &clone
}
