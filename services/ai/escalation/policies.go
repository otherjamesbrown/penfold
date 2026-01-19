package escalation

import (
	"context"
	"sync"
	"time"
)

// PolicyType identifies the type of escalation policy.
type PolicyType string

const (
	// PolicyTypeLowConfidence escalates when confidence is below threshold.
	PolicyTypeLowConfidence PolicyType = "low_confidence"

	// PolicyTypeHighComplexity escalates for complex requests.
	PolicyTypeHighComplexity PolicyType = "high_complexity"

	// PolicyTypeCriticalTask always uses premium models.
	PolicyTypeCriticalTask PolicyType = "critical_task"

	// PolicyTypeCostAware escalates within budget constraints.
	PolicyTypeCostAware PolicyType = "cost_aware"

	// PolicyTypeTimeBased escalates based on time-of-day or load.
	PolicyTypeTimeBased PolicyType = "time_based"

	// PolicyTypeTenant escalates based on tenant configuration.
	PolicyTypeTenant PolicyType = "tenant"
)

// PolicyDecision represents the decision made by a policy.
type PolicyDecision struct {
	// Allow indicates if escalation is permitted.
	Allow bool

	// Deny indicates if escalation is explicitly denied.
	Deny bool

	// Reason explains the decision.
	Reason string

	// RecommendedTier is the suggested tier to escalate to.
	RecommendedTier TierLevel

	// MaxTier is the maximum tier this policy allows.
	MaxTier TierLevel

	// CostEstimate is the estimated cost of escalation.
	CostEstimate float64

	// Metadata contains additional policy-specific information.
	Metadata map[string]interface{}
}

// Policy defines the interface for escalation policies.
type Policy interface {
	// Type returns the policy type.
	Type() PolicyType

	// Evaluate evaluates the policy for the given request.
	Evaluate(ctx context.Context, request *EscalationRequest, triggerResult *TriggerResult) PolicyDecision

	// Name returns a human-readable name for this policy.
	Name() string

	// Priority returns the policy priority (higher = more important).
	Priority() int
}

// LowConfidencePolicy escalates requests with low confidence scores.
type LowConfidencePolicy struct {
	// ConfidenceThresholds maps tier levels to minimum confidence thresholds.
	ConfidenceThresholds map[TierLevel]float64

	// GradualEscalation enables step-by-step escalation rather than jumping to premium.
	GradualEscalation bool

	// priority is the policy priority.
	priority int
}

// NewLowConfidencePolicy creates a new low confidence escalation policy.
func NewLowConfidencePolicy() *LowConfidencePolicy {
	return &LowConfidencePolicy{
		ConfidenceThresholds: map[TierLevel]float64{
			TierLevelLocal:         0.7,
			TierLevelCloudStandard: 0.8,
			TierLevelCloudPremium:  0.9,
		},
		GradualEscalation: true,
		priority:          100,
	}
}

// Type returns the policy type.
func (p *LowConfidencePolicy) Type() PolicyType {
	return PolicyTypeLowConfidence
}

// Name returns a human-readable name for this policy.
func (p *LowConfidencePolicy) Name() string {
	return "Low Confidence Escalation Policy"
}

// Priority returns the policy priority.
func (p *LowConfidencePolicy) Priority() int {
	return p.priority
}

// Evaluate checks if escalation should occur based on confidence.
func (p *LowConfidencePolicy) Evaluate(ctx context.Context, request *EscalationRequest, triggerResult *TriggerResult) PolicyDecision {
	decision := PolicyDecision{
		Metadata: make(map[string]interface{}),
	}

	// Get threshold for current tier
	threshold, ok := p.ConfidenceThresholds[request.CurrentTier]
	if !ok {
		threshold = 0.7 // Default threshold
	}

	// If confidence is above threshold, no escalation needed
	if triggerResult.Confidence >= threshold {
		decision.Allow = false
		decision.Reason = "confidence meets threshold"
		return decision
	}

	decision.Allow = true
	decision.Metadata["confidence"] = triggerResult.Confidence
	decision.Metadata["threshold"] = threshold

	// Determine recommended tier
	if p.GradualEscalation {
		// Escalate one tier at a time
		nextTier := request.CurrentTier + 1
		if nextTier > TierLevelCloudPremium {
			nextTier = TierLevelCloudPremium
		}
		decision.RecommendedTier = nextTier
		decision.MaxTier = TierLevelCloudPremium
		decision.Reason = "gradual escalation due to low confidence"
	} else {
		// Jump directly based on confidence level
		if triggerResult.Confidence < 0.4 {
			decision.RecommendedTier = TierLevelCloudPremium
		} else {
			decision.RecommendedTier = TierLevelCloudStandard
		}
		decision.MaxTier = TierLevelCloudPremium
		decision.Reason = "direct escalation due to low confidence"
	}

	return decision
}

// HighComplexityPolicy escalates complex requests to higher tiers.
type HighComplexityPolicy struct {
	// ComplexityThreshold is the minimum complexity score for escalation.
	ComplexityThreshold float64

	// ContentLengthThreshold triggers escalation for long content.
	ContentLengthThreshold int

	// ComplexTaskTypes are task types considered inherently complex.
	ComplexTaskTypes map[string]TierLevel

	// priority is the policy priority.
	priority int
}

// NewHighComplexityPolicy creates a new high complexity escalation policy.
func NewHighComplexityPolicy() *HighComplexityPolicy {
	return &HighComplexityPolicy{
		ComplexityThreshold:    0.7,
		ContentLengthThreshold: 50000,
		ComplexTaskTypes: map[string]TierLevel{
			"reasoning":       TierLevelCloudPremium,
			"code_generation": TierLevelCloudPremium,
			"multi_step":      TierLevelCloudStandard,
			"extraction":      TierLevelCloudStandard,
		},
		priority: 80,
	}
}

// Type returns the policy type.
func (p *HighComplexityPolicy) Type() PolicyType {
	return PolicyTypeHighComplexity
}

// Name returns a human-readable name for this policy.
func (p *HighComplexityPolicy) Name() string {
	return "High Complexity Escalation Policy"
}

// Priority returns the policy priority.
func (p *HighComplexityPolicy) Priority() int {
	return p.priority
}

// Evaluate checks if escalation should occur based on complexity.
func (p *HighComplexityPolicy) Evaluate(ctx context.Context, request *EscalationRequest, triggerResult *TriggerResult) PolicyDecision {
	decision := PolicyDecision{
		Metadata: make(map[string]interface{}),
	}

	// Check if task type requires specific tier
	if minTier, ok := p.ComplexTaskTypes[request.TaskType]; ok {
		if request.CurrentTier < minTier {
			decision.Allow = true
			decision.RecommendedTier = minTier
			decision.MaxTier = TierLevelCloudPremium
			decision.Reason = "complex task type requires higher tier"
			decision.Metadata["task_type"] = request.TaskType
			return decision
		}
	}

	// Check content length
	if len(request.Content) > p.ContentLengthThreshold {
		decision.Allow = true
		decision.RecommendedTier = TierLevelCloudStandard
		if len(request.Content) > p.ContentLengthThreshold*2 {
			decision.RecommendedTier = TierLevelCloudPremium
		}
		decision.MaxTier = TierLevelCloudPremium
		decision.Reason = "content length exceeds threshold"
		decision.Metadata["content_length"] = len(request.Content)
		return decision
	}

	decision.Allow = false
	decision.Reason = "complexity within acceptable range"
	return decision
}

// CriticalTaskPolicy always uses premium models for critical tasks.
type CriticalTaskPolicy struct {
	// CriticalTaskTypes are task types that always require premium.
	CriticalTaskTypes []string

	// CriticalContentTypes are content types that always require premium.
	CriticalContentTypes []string

	// priority is the policy priority.
	priority int
}

// NewCriticalTaskPolicy creates a new critical task escalation policy.
func NewCriticalTaskPolicy() *CriticalTaskPolicy {
	return &CriticalTaskPolicy{
		CriticalTaskTypes:    []string{"legal_analysis", "medical_advice", "financial_analysis"},
		CriticalContentTypes: []string{"legal", "medical", "financial", "pii"},
		priority:             200, // High priority
	}
}

// Type returns the policy type.
func (p *CriticalTaskPolicy) Type() PolicyType {
	return PolicyTypeCriticalTask
}

// Name returns a human-readable name for this policy.
func (p *CriticalTaskPolicy) Name() string {
	return "Critical Task Escalation Policy"
}

// Priority returns the policy priority.
func (p *CriticalTaskPolicy) Priority() int {
	return p.priority
}

// Evaluate checks if escalation should occur for critical tasks.
func (p *CriticalTaskPolicy) Evaluate(ctx context.Context, request *EscalationRequest, triggerResult *TriggerResult) PolicyDecision {
	decision := PolicyDecision{
		Metadata: make(map[string]interface{}),
	}

	// Check if task type is critical
	for _, criticalType := range p.CriticalTaskTypes {
		if request.TaskType == criticalType {
			decision.Allow = true
			decision.RecommendedTier = TierLevelCloudPremium
			decision.MaxTier = TierLevelCloudPremium
			decision.Reason = "critical task type requires premium tier"
			decision.Metadata["critical_task_type"] = criticalType
			return decision
		}
	}

	// Check if content type is critical
	for _, criticalType := range p.CriticalContentTypes {
		if request.ContentType == criticalType {
			decision.Allow = true
			decision.RecommendedTier = TierLevelCloudPremium
			decision.MaxTier = TierLevelCloudPremium
			decision.Reason = "critical content type requires premium tier"
			decision.Metadata["critical_content_type"] = criticalType
			return decision
		}
	}

	decision.Allow = false
	decision.Reason = "not a critical task"
	return decision
}

// CostAwarePolicy escalates within budget constraints.
type CostAwarePolicy struct {
	mu sync.RWMutex

	// DailyBudget is the daily budget in USD.
	DailyBudget float64

	// DailySpent tracks daily spending.
	DailySpent float64

	// LastReset tracks when spending was last reset.
	LastReset time.Time

	// MaxCostPerRequest is the maximum cost allowed per escalation.
	MaxCostPerRequest float64

	// CostMultiplierLimit is the maximum cost multiplier for escalation.
	CostMultiplierLimit float64

	// priority is the policy priority.
	priority int
}

// NewCostAwarePolicy creates a new cost-aware escalation policy.
func NewCostAwarePolicy(dailyBudget float64) *CostAwarePolicy {
	return &CostAwarePolicy{
		DailyBudget:         dailyBudget,
		DailySpent:          0,
		LastReset:           time.Now(),
		MaxCostPerRequest:   1.0, // $1 max per request
		CostMultiplierLimit: 50.0,
		priority:            150,
	}
}

// Type returns the policy type.
func (p *CostAwarePolicy) Type() PolicyType {
	return PolicyTypeCostAware
}

// Name returns a human-readable name for this policy.
func (p *CostAwarePolicy) Name() string {
	return "Cost Aware Escalation Policy"
}

// Priority returns the policy priority.
func (p *CostAwarePolicy) Priority() int {
	return p.priority
}

// Evaluate checks if escalation is allowed within budget.
func (p *CostAwarePolicy) Evaluate(ctx context.Context, request *EscalationRequest, triggerResult *TriggerResult) PolicyDecision {
	p.mu.Lock()
	defer p.mu.Unlock()

	decision := PolicyDecision{
		Metadata: make(map[string]interface{}),
	}

	// Reset daily spending if needed
	p.resetIfNeeded()

	// Estimate escalation cost
	estimatedCost := p.estimateCost(request, triggerResult.RecommendedTier)
	decision.CostEstimate = estimatedCost
	decision.Metadata["estimated_cost"] = estimatedCost
	decision.Metadata["daily_spent"] = p.DailySpent
	decision.Metadata["daily_budget"] = p.DailyBudget

	// Check if escalation would exceed budget
	if p.DailySpent+estimatedCost > p.DailyBudget {
		decision.Deny = true
		decision.Reason = "escalation would exceed daily budget"
		// Allow escalation to standard tier if premium is too expensive
		if triggerResult.RecommendedTier == TierLevelCloudPremium {
			standardCost := p.estimateCost(request, TierLevelCloudStandard)
			if p.DailySpent+standardCost <= p.DailyBudget {
				decision.Deny = false
				decision.Allow = true
				decision.RecommendedTier = TierLevelCloudStandard
				decision.MaxTier = TierLevelCloudStandard
				decision.Reason = "escalation limited to standard tier due to budget"
				decision.CostEstimate = standardCost
			}
		}
		return decision
	}

	// Check per-request cost limit
	if estimatedCost > p.MaxCostPerRequest {
		decision.Deny = true
		decision.Reason = "escalation exceeds per-request cost limit"
		return decision
	}

	decision.Allow = true
	decision.RecommendedTier = triggerResult.RecommendedTier
	decision.MaxTier = TierLevelCloudPremium
	decision.Reason = "escalation within budget"
	return decision
}

// resetIfNeeded resets daily spending if a new day has started.
func (p *CostAwarePolicy) resetIfNeeded() {
	now := time.Now()
	if now.Year() != p.LastReset.Year() ||
		now.YearDay() != p.LastReset.YearDay() {
		p.DailySpent = 0
		p.LastReset = now
	}
}

// estimateCost estimates the cost of escalation.
func (p *CostAwarePolicy) estimateCost(request *EscalationRequest, tier TierLevel) float64 {
	// Base cost estimation based on content length and tier
	baseTokens := len(request.Content) / 4 // Rough estimate
	outputTokens := 500                    // Estimated output

	var costPer1K float64
	switch tier {
	case TierLevelLocal:
		costPer1K = 0
	case TierLevelCloudStandard:
		costPer1K = 0.0001 // $0.10 per million tokens
	case TierLevelCloudPremium:
		costPer1K = 0.001 // $1 per million tokens
	}

	totalTokens := float64(baseTokens + outputTokens)
	return (totalTokens / 1000.0) * costPer1K
}

// RecordSpending records actual spending.
func (p *CostAwarePolicy) RecordSpending(amount float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resetIfNeeded()
	p.DailySpent += amount
}

// GetBudgetStatus returns current budget status.
func (p *CostAwarePolicy) GetBudgetStatus() (spent, remaining float64) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.DailySpent, p.DailyBudget - p.DailySpent
}

// TenantPolicy applies tenant-specific escalation rules.
type TenantPolicy struct {
	mu sync.RWMutex

	// TenantConfigs maps tenant IDs to their escalation configurations.
	TenantConfigs map[string]*TenantEscalationConfig

	// DefaultConfig is used when no tenant-specific config exists.
	DefaultConfig *TenantEscalationConfig

	// priority is the policy priority.
	priority int
}

// TenantEscalationConfig holds tenant-specific escalation settings.
type TenantEscalationConfig struct {
	// MaxTier is the maximum tier allowed for this tenant.
	MaxTier TierLevel

	// DefaultTier is the starting tier for this tenant.
	DefaultTier TierLevel

	// DailyBudget is the daily escalation budget.
	DailyBudget float64

	// AllowedTaskTypes are task types that can be escalated.
	AllowedTaskTypes []string

	// AutoEscalate enables automatic escalation.
	AutoEscalate bool

	// RequireApproval requires human approval for escalation.
	RequireApproval bool
}

// NewTenantPolicy creates a new tenant-specific escalation policy.
func NewTenantPolicy() *TenantPolicy {
	return &TenantPolicy{
		TenantConfigs: make(map[string]*TenantEscalationConfig),
		DefaultConfig: &TenantEscalationConfig{
			MaxTier:      TierLevelCloudStandard,
			DefaultTier:  TierLevelLocal,
			DailyBudget:  10.0,
			AutoEscalate: true,
		},
		priority: 90,
	}
}

// Type returns the policy type.
func (p *TenantPolicy) Type() PolicyType {
	return PolicyTypeTenant
}

// Name returns a human-readable name for this policy.
func (p *TenantPolicy) Name() string {
	return "Tenant Escalation Policy"
}

// Priority returns the policy priority.
func (p *TenantPolicy) Priority() int {
	return p.priority
}

// Evaluate checks if escalation is allowed for the tenant.
func (p *TenantPolicy) Evaluate(ctx context.Context, request *EscalationRequest, triggerResult *TriggerResult) PolicyDecision {
	p.mu.RLock()
	defer p.mu.RUnlock()

	decision := PolicyDecision{
		Metadata: make(map[string]interface{}),
	}

	// Get tenant config
	config := p.DefaultConfig
	if tenantConfig, ok := p.TenantConfigs[request.TenantID]; ok {
		config = tenantConfig
	}

	decision.Metadata["tenant_id"] = request.TenantID
	decision.Metadata["max_tier"] = config.MaxTier

	// Check if auto-escalation is enabled
	if !config.AutoEscalate && !request.UserRequestedUpgrade {
		decision.Deny = true
		decision.Reason = "automatic escalation disabled for tenant"
		return decision
	}

	// Check if recommended tier exceeds max allowed
	if triggerResult.RecommendedTier > config.MaxTier {
		decision.Allow = true
		decision.RecommendedTier = config.MaxTier
		decision.MaxTier = config.MaxTier
		decision.Reason = "escalation limited to tenant max tier"
		return decision
	}

	decision.Allow = true
	decision.RecommendedTier = triggerResult.RecommendedTier
	decision.MaxTier = config.MaxTier
	decision.Reason = "escalation allowed by tenant policy"
	return decision
}

// SetTenantConfig sets the configuration for a specific tenant.
func (p *TenantPolicy) SetTenantConfig(tenantID string, config *TenantEscalationConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.TenantConfigs[tenantID] = config
}

// GetTenantConfig returns the configuration for a specific tenant.
func (p *TenantPolicy) GetTenantConfig(tenantID string) *TenantEscalationConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if config, ok := p.TenantConfigs[tenantID]; ok {
		return config
	}
	return p.DefaultConfig
}

// PolicyChain evaluates multiple policies and combines their decisions.
type PolicyChain struct {
	policies []Policy
}

// NewPolicyChain creates a new policy chain.
func NewPolicyChain() *PolicyChain {
	return &PolicyChain{
		policies: make([]Policy, 0),
	}
}

// Add adds a policy to the chain.
func (c *PolicyChain) Add(policy Policy) {
	c.policies = append(c.policies, policy)
}

// Evaluate evaluates all policies and returns a combined decision.
func (c *PolicyChain) Evaluate(ctx context.Context, request *EscalationRequest, triggerResult *TriggerResult) PolicyDecision {
	// Sort policies by priority (highest first)
	sortedPolicies := make([]Policy, len(c.policies))
	copy(sortedPolicies, c.policies)
	for i := 0; i < len(sortedPolicies)-1; i++ {
		for j := i + 1; j < len(sortedPolicies); j++ {
			if sortedPolicies[j].Priority() > sortedPolicies[i].Priority() {
				sortedPolicies[i], sortedPolicies[j] = sortedPolicies[j], sortedPolicies[i]
			}
		}
	}

	finalDecision := PolicyDecision{
		Metadata: make(map[string]interface{}),
	}

	// Collect decisions from all policies
	var allowDecisions []PolicyDecision
	for _, policy := range sortedPolicies {
		decision := policy.Evaluate(ctx, request, triggerResult)

		// Deny decisions have highest priority
		if decision.Deny {
			return decision
		}

		if decision.Allow {
			allowDecisions = append(allowDecisions, decision)
		}
	}

	if len(allowDecisions) == 0 {
		finalDecision.Allow = false
		finalDecision.Reason = "no policy allowed escalation"
		return finalDecision
	}

	// Use the first (highest priority) allow decision
	return allowDecisions[0]
}

// DefaultPolicyChain creates a policy chain with all default policies.
func DefaultPolicyChain(dailyBudget float64) *PolicyChain {
	chain := NewPolicyChain()

	// Add policies (order determines priority for same-priority policies)
	chain.Add(NewCriticalTaskPolicy())
	chain.Add(NewCostAwarePolicy(dailyBudget))
	chain.Add(NewLowConfidencePolicy())
	chain.Add(NewTenantPolicy())
	chain.Add(NewHighComplexityPolicy())

	return chain
}
