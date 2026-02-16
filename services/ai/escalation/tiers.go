// Package escalation provides automatic escalation management for AI model requests.
// It handles low confidence responses, model failures, and cost-benefit analysis
// for escalating requests from local to cloud models.
package escalation

import (
	"errors"
	"time"
)

// Errors returned by the escalation package.
var (
	ErrNoTiersConfigured   = errors.New("no model tiers configured")
	ErrTierNotFound        = errors.New("tier not found")
	ErrNoEscalationPath    = errors.New("no escalation path available")
	ErrAlreadyAtMaxTier    = errors.New("already at maximum tier")
	ErrInvalidTierLevel    = errors.New("invalid tier level")
	ErrBudgetExceeded      = errors.New("escalation would exceed budget")
	ErrPolicyViolation     = errors.New("escalation violates policy")
)

// TierLevel represents the level of a model tier in the escalation hierarchy.
type TierLevel int

const (
	// TierLevelLocal represents local models (lowest tier).
	TierLevelLocal TierLevel = iota

	// TierLevelCloudStandard represents standard cloud models.
	TierLevelCloudStandard

	// TierLevelCloudPremium represents premium cloud models (highest tier).
	TierLevelCloudPremium
)

// String returns the string representation of a tier level.
func (t TierLevel) String() string {
	switch t {
	case TierLevelLocal:
		return "local"
	case TierLevelCloudStandard:
		return "cloud-standard"
	case TierLevelCloudPremium:
		return "cloud-premium"
	default:
		return "unknown"
	}
}

// ModelTier represents a tier in the model escalation hierarchy.
type ModelTier struct {
	// Level is the tier level in the escalation hierarchy.
	Level TierLevel

	// Name is a human-readable name for this tier.
	Name string

	// Models is the list of model IDs available at this tier.
	Models []string

	// PreferredModel is the default model to use for this tier.
	PreferredModel string

	// CostMultiplier is the cost multiplier relative to the base tier.
	CostMultiplier float64

	// Capabilities lists the capabilities available at this tier.
	Capabilities []string

	// MaxLatency is the expected maximum latency for this tier.
	MaxLatency time.Duration

	// MinConfidence is the minimum confidence threshold expected from this tier.
	MinConfidence float64

	// QualityScore is the average quality score for models in this tier.
	QualityScore float64

	// IsLocal indicates if this tier uses local models.
	IsLocal bool

	// Description provides additional context about this tier.
	Description string
}

// CanHandle checks if this tier can handle the given capabilities.
func (t *ModelTier) CanHandle(requiredCapabilities []string) bool {
	if len(requiredCapabilities) == 0 {
		return true
	}

	capMap := make(map[string]bool)
	for _, c := range t.Capabilities {
		capMap[c] = true
	}

	for _, req := range requiredCapabilities {
		if !capMap[req] {
			return false
		}
	}
	return true
}

// HasModel checks if a specific model is available in this tier.
func (t *ModelTier) HasModel(modelID string) bool {
	for _, m := range t.Models {
		if m == modelID {
			return true
		}
	}
	return false
}

// TierConfig holds the configuration for all model tiers.
type TierConfig struct {
	// Tiers is the ordered list of tiers from lowest to highest.
	Tiers []*ModelTier

	// DefaultTier is the starting tier for new requests.
	DefaultTier TierLevel

	// MaxTier is the maximum tier that can be escalated to.
	MaxTier TierLevel

	// EnableCostAwareEscalation enables cost-benefit analysis before escalation.
	EnableCostAwareEscalation bool

	// MaxCostMultiplier is the maximum cost multiplier allowed for escalation.
	MaxCostMultiplier float64

	// EscalationCooldown is the minimum time between escalations for the same request type.
	EscalationCooldown time.Duration
}

// DefaultTierConfig returns a default tier configuration with local and cloud tiers.
func DefaultTierConfig() *TierConfig {
	return &TierConfig{
		Tiers: []*ModelTier{
			{
				Level:          TierLevelLocal,
				Name:           "Local",
				Models:         []string{"mlx-community/Qwen2.5-7B-Instruct-4bit", "mxbai-embed-large"},
				PreferredModel: "mlx-community/Qwen2.5-7B-Instruct-4bit",
				CostMultiplier: 0.0,
				Capabilities:   []string{"embedding", "summarization", "extraction", "classification"},
				MaxLatency:     500 * time.Millisecond,
				MinConfidence:  0.6,
				QualityScore:   0.70,
				IsLocal:        true,
				Description:    "Local MLX models for privacy-sensitive and high-volume tasks",
			},
			{
				Level:          TierLevelCloudStandard,
				Name:           "Cloud Standard",
				Models:         []string{"gemini-2.0-flash", "gemini-2.5-pro", "text-embedding-004"},
				PreferredModel: "gemini-2.0-flash",
				CostMultiplier: 1.0,
				Capabilities:   []string{"embedding", "summarization", "extraction", "classification", "chat"},
				MaxLatency:     2 * time.Second,
				MinConfidence:  0.75,
				QualityScore:   0.85,
				IsLocal:        false,
				Description:    "Standard cloud models for balanced performance and cost",
			},
			{
				Level:          TierLevelCloudPremium,
				Name:           "Cloud Premium",
				Models:         []string{"gemini-2.5-pro", "gemini-2.0-pro", "claude-3-opus"},
				PreferredModel: "gemini-2.5-pro",
				CostMultiplier: 10.0,
				Capabilities:   []string{"embedding", "summarization", "extraction", "classification", "chat", "reasoning", "code_generation"},
				MaxLatency:     10 * time.Second,
				MinConfidence:  0.90,
				QualityScore:   0.95,
				IsLocal:        false,
				Description:    "Premium cloud models for complex and critical tasks",
			},
		},
		DefaultTier:               TierLevelLocal,
		MaxTier:                   TierLevelCloudPremium,
		EnableCostAwareEscalation: true,
		MaxCostMultiplier:         100.0,
		EscalationCooldown:        1 * time.Minute,
	}
}

// TierRegistry manages the model tier hierarchy.
type TierRegistry struct {
	config    *TierConfig
	tierByID  map[TierLevel]*ModelTier
	modelTier map[string]TierLevel
}

// NewTierRegistry creates a new tier registry with the given configuration.
func NewTierRegistry(config *TierConfig) *TierRegistry {
	if config == nil {
		config = DefaultTierConfig()
	}

	registry := &TierRegistry{
		config:    config,
		tierByID:  make(map[TierLevel]*ModelTier),
		modelTier: make(map[string]TierLevel),
	}

	// Index tiers by level and models by tier
	for _, tier := range config.Tiers {
		registry.tierByID[tier.Level] = tier
		for _, model := range tier.Models {
			registry.modelTier[model] = tier.Level
		}
	}

	return registry
}

// GetTier returns the tier configuration for the given level.
func (r *TierRegistry) GetTier(level TierLevel) (*ModelTier, error) {
	tier, ok := r.tierByID[level]
	if !ok {
		return nil, ErrTierNotFound
	}
	return tier, nil
}

// GetTierForModel returns the tier level for a specific model.
func (r *TierRegistry) GetTierForModel(modelID string) (TierLevel, error) {
	level, ok := r.modelTier[modelID]
	if !ok {
		return TierLevelLocal, ErrTierNotFound
	}
	return level, nil
}

// GetNextTier returns the next higher tier for escalation.
func (r *TierRegistry) GetNextTier(current TierLevel) (*ModelTier, error) {
	if current >= r.config.MaxTier {
		return nil, ErrAlreadyAtMaxTier
	}

	nextLevel := current + 1
	return r.GetTier(nextLevel)
}

// GetEscalationPath returns the full escalation path from the given starting tier.
func (r *TierRegistry) GetEscalationPath(startLevel TierLevel) ([]*ModelTier, error) {
	if startLevel > r.config.MaxTier {
		return nil, ErrInvalidTierLevel
	}

	var path []*ModelTier
	for level := startLevel; level <= r.config.MaxTier; level++ {
		tier, err := r.GetTier(level)
		if err != nil {
			continue
		}
		path = append(path, tier)
	}

	if len(path) == 0 {
		return nil, ErrNoEscalationPath
	}

	return path, nil
}

// GetEscalationPathForTask returns the escalation path filtered by required capabilities.
func (r *TierRegistry) GetEscalationPathForTask(startLevel TierLevel, requiredCapabilities []string) ([]*ModelTier, error) {
	fullPath, err := r.GetEscalationPath(startLevel)
	if err != nil {
		return nil, err
	}

	var filteredPath []*ModelTier
	for _, tier := range fullPath {
		if tier.CanHandle(requiredCapabilities) {
			filteredPath = append(filteredPath, tier)
		}
	}

	if len(filteredPath) == 0 {
		return nil, ErrNoEscalationPath
	}

	return filteredPath, nil
}

// EstimateEscalationCost estimates the cost multiplier for escalating from one tier to another.
func (r *TierRegistry) EstimateEscalationCost(from, to TierLevel) (float64, error) {
	fromTier, err := r.GetTier(from)
	if err != nil {
		return 0, err
	}

	toTier, err := r.GetTier(to)
	if err != nil {
		return 0, err
	}

	// If moving from local (0 cost), return absolute cost of target
	if fromTier.CostMultiplier == 0 {
		return toTier.CostMultiplier, nil
	}

	// Return relative cost increase
	return toTier.CostMultiplier / fromTier.CostMultiplier, nil
}

// GetAllTiers returns all configured tiers in order.
func (r *TierRegistry) GetAllTiers() []*ModelTier {
	return r.config.Tiers
}

// GetDefaultTier returns the default starting tier.
func (r *TierRegistry) GetDefaultTier() (*ModelTier, error) {
	return r.GetTier(r.config.DefaultTier)
}

// GetMaxTier returns the maximum escalation tier.
func (r *TierRegistry) GetMaxTier() (*ModelTier, error) {
	return r.GetTier(r.config.MaxTier)
}

// IsMaxTier checks if the given level is the maximum tier.
func (r *TierRegistry) IsMaxTier(level TierLevel) bool {
	return level >= r.config.MaxTier
}

// GetConfig returns the tier configuration.
func (r *TierRegistry) GetConfig() *TierConfig {
	return r.config
}
