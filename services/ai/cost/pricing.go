package cost

import (
	"sync"
)

// ModelPricing defines pricing for a specific model.
type ModelPricing struct {
	// Model is the model identifier.
	Model string `json:"model"`

	// Provider is the provider name.
	Provider string `json:"provider"`

	// InputCostPer1K is the cost per 1000 input tokens in USD.
	InputCostPer1K float64 `json:"input_cost_per_1k"`

	// OutputCostPer1K is the cost per 1000 output tokens in USD.
	OutputCostPer1K float64 `json:"output_cost_per_1k"`

	// EmbeddingCostPer1K is the cost per 1000 tokens for embeddings.
	EmbeddingCostPer1K float64 `json:"embedding_cost_per_1k,omitempty"`

	// IsLocal indicates if this is a local model (typically free).
	IsLocal bool `json:"is_local"`
}

// CalculateCost calculates the cost for given token counts.
func (p *ModelPricing) CalculateCost(inputTokens, outputTokens int) float64 {
	if p.IsLocal {
		return 0
	}
	inputCost := float64(inputTokens) * p.InputCostPer1K / 1000.0
	outputCost := float64(outputTokens) * p.OutputCostPer1K / 1000.0
	return inputCost + outputCost
}

// CalculateEmbeddingCost calculates the cost for embeddings.
func (p *ModelPricing) CalculateEmbeddingCost(tokens int) float64 {
	if p.IsLocal {
		return 0
	}
	return float64(tokens) * p.EmbeddingCostPer1K / 1000.0
}

// TenantPricingOverride allows tenants to have custom pricing.
type TenantPricingOverride struct {
	// TenantID is the tenant this override applies to.
	TenantID string `json:"tenant_id"`

	// Model is the model this override applies to.
	Model string `json:"model"`

	// DiscountPercent is the discount percentage (0-100).
	DiscountPercent float64 `json:"discount_percent"`

	// CustomInputCostPer1K overrides the input cost (if set).
	CustomInputCostPer1K *float64 `json:"custom_input_cost_per_1k,omitempty"`

	// CustomOutputCostPer1K overrides the output cost (if set).
	CustomOutputCostPer1K *float64 `json:"custom_output_cost_per_1k,omitempty"`
}

// PricingTable manages pricing for all supported models.
type PricingTable struct {
	mu sync.RWMutex

	// pricing stores base pricing by model name.
	pricing map[string]*ModelPricing

	// overrides stores tenant-specific pricing overrides.
	// Key format: "tenant_id:model"
	overrides map[string]*TenantPricingOverride
}

// NewPricingTable creates a new pricing table with default pricing.
func NewPricingTable() *PricingTable {
	pt := &PricingTable{
		pricing:   make(map[string]*ModelPricing),
		overrides: make(map[string]*TenantPricingOverride),
	}
	pt.loadDefaultPricing()
	return pt
}

// loadDefaultPricing populates the table with current model pricing.
// Pricing as of 2024 - should be updated periodically.
func (pt *PricingTable) loadDefaultPricing() {
	// Local models (Ollama) - free
	localModels := []string{
		"llama3.2", "llama3.2:8b", "llama3.2:70b",
		"llama3.1", "llama3.1:8b", "llama3.1:70b",
		"nomic-embed-text", "mxbai-embed-large",
		"mistral", "mistral:7b",
		"codellama", "codellama:7b", "codellama:13b",
		"phi3", "phi3:3.8b",
		"qwen2", "qwen2:7b",
	}
	for _, model := range localModels {
		pt.pricing[model] = &ModelPricing{
			Model:              model,
			Provider:           "ollama",
			InputCostPer1K:     0,
			OutputCostPer1K:    0,
			EmbeddingCostPer1K: 0,
			IsLocal:            true,
		}
	}

	// Google Gemini models
	pt.pricing["gemini-1.5-pro"] = &ModelPricing{
		Model:           "gemini-1.5-pro",
		Provider:        "gemini",
		InputCostPer1K:  0.00125, // $1.25 per 1M input tokens
		OutputCostPer1K: 0.005,   // $5.00 per 1M output tokens
		IsLocal:         false,
	}
	pt.pricing["gemini-1.5-flash"] = &ModelPricing{
		Model:           "gemini-1.5-flash",
		Provider:        "gemini",
		InputCostPer1K:  0.000075, // $0.075 per 1M input tokens
		OutputCostPer1K: 0.0003,   // $0.30 per 1M output tokens
		IsLocal:         false,
	}
	pt.pricing["gemini-1.5-flash-8b"] = &ModelPricing{
		Model:           "gemini-1.5-flash-8b",
		Provider:        "gemini",
		InputCostPer1K:  0.0000375, // $0.0375 per 1M input tokens
		OutputCostPer1K: 0.00015,   // $0.15 per 1M output tokens
		IsLocal:         false,
	}
	pt.pricing["text-embedding-004"] = &ModelPricing{
		Model:              "text-embedding-004",
		Provider:           "gemini",
		EmbeddingCostPer1K: 0.00001, // $0.01 per 1M tokens
		IsLocal:            false,
	}
	pt.pricing["gemini-2.0-flash-exp"] = &ModelPricing{
		Model:           "gemini-2.0-flash-exp",
		Provider:        "gemini",
		InputCostPer1K:  0.000075, // Same as 1.5-flash for now
		OutputCostPer1K: 0.0003,
		IsLocal:         false,
	}

	// OpenAI models
	pt.pricing["gpt-4o"] = &ModelPricing{
		Model:           "gpt-4o",
		Provider:        "openai",
		InputCostPer1K:  0.0025,  // $2.50 per 1M input tokens
		OutputCostPer1K: 0.01,    // $10.00 per 1M output tokens
		IsLocal:         false,
	}
	pt.pricing["gpt-4o-mini"] = &ModelPricing{
		Model:           "gpt-4o-mini",
		Provider:        "openai",
		InputCostPer1K:  0.00015, // $0.15 per 1M input tokens
		OutputCostPer1K: 0.0006,  // $0.60 per 1M output tokens
		IsLocal:         false,
	}
	pt.pricing["gpt-4-turbo"] = &ModelPricing{
		Model:           "gpt-4-turbo",
		Provider:        "openai",
		InputCostPer1K:  0.01,    // $10.00 per 1M input tokens
		OutputCostPer1K: 0.03,    // $30.00 per 1M output tokens
		IsLocal:         false,
	}
	pt.pricing["gpt-3.5-turbo"] = &ModelPricing{
		Model:           "gpt-3.5-turbo",
		Provider:        "openai",
		InputCostPer1K:  0.0005,  // $0.50 per 1M input tokens
		OutputCostPer1K: 0.0015,  // $1.50 per 1M output tokens
		IsLocal:         false,
	}
	pt.pricing["text-embedding-3-small"] = &ModelPricing{
		Model:              "text-embedding-3-small",
		Provider:           "openai",
		EmbeddingCostPer1K: 0.00002, // $0.02 per 1M tokens
		IsLocal:            false,
	}
	pt.pricing["text-embedding-3-large"] = &ModelPricing{
		Model:              "text-embedding-3-large",
		Provider:           "openai",
		EmbeddingCostPer1K: 0.00013, // $0.13 per 1M tokens
		IsLocal:            false,
	}

	// Anthropic models
	pt.pricing["claude-3-5-sonnet-20241022"] = &ModelPricing{
		Model:           "claude-3-5-sonnet-20241022",
		Provider:        "anthropic",
		InputCostPer1K:  0.003,   // $3.00 per 1M input tokens
		OutputCostPer1K: 0.015,   // $15.00 per 1M output tokens
		IsLocal:         false,
	}
	pt.pricing["claude-3-5-haiku-20241022"] = &ModelPricing{
		Model:           "claude-3-5-haiku-20241022",
		Provider:        "anthropic",
		InputCostPer1K:  0.001,   // $1.00 per 1M input tokens
		OutputCostPer1K: 0.005,   // $5.00 per 1M output tokens
		IsLocal:         false,
	}
	pt.pricing["claude-3-opus-20240229"] = &ModelPricing{
		Model:           "claude-3-opus-20240229",
		Provider:        "anthropic",
		InputCostPer1K:  0.015,   // $15.00 per 1M input tokens
		OutputCostPer1K: 0.075,   // $75.00 per 1M output tokens
		IsLocal:         false,
	}
}

// GetPricing returns pricing for a model, considering tenant overrides.
func (pt *PricingTable) GetPricing(model, tenantID string) *ModelPricing {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	// Get base pricing
	basePricing, ok := pt.pricing[model]
	if !ok {
		// Return zero pricing for unknown models
		return &ModelPricing{
			Model:    model,
			Provider: "unknown",
			IsLocal:  false,
		}
	}

	// Check for tenant override
	if tenantID != "" {
		overrideKey := tenantID + ":" + model
		if override, hasOverride := pt.overrides[overrideKey]; hasOverride {
			return pt.applyOverride(basePricing, override)
		}
	}

	return basePricing
}

// applyOverride applies a tenant override to base pricing.
func (pt *PricingTable) applyOverride(base *ModelPricing, override *TenantPricingOverride) *ModelPricing {
	result := &ModelPricing{
		Model:              base.Model,
		Provider:           base.Provider,
		InputCostPer1K:     base.InputCostPer1K,
		OutputCostPer1K:    base.OutputCostPer1K,
		EmbeddingCostPer1K: base.EmbeddingCostPer1K,
		IsLocal:            base.IsLocal,
	}

	// Apply custom pricing if specified
	if override.CustomInputCostPer1K != nil {
		result.InputCostPer1K = *override.CustomInputCostPer1K
	}
	if override.CustomOutputCostPer1K != nil {
		result.OutputCostPer1K = *override.CustomOutputCostPer1K
	}

	// Apply discount
	if override.DiscountPercent > 0 {
		discount := 1 - (override.DiscountPercent / 100)
		result.InputCostPer1K *= discount
		result.OutputCostPer1K *= discount
		result.EmbeddingCostPer1K *= discount
	}

	return result
}

// SetPricing adds or updates pricing for a model.
func (pt *PricingTable) SetPricing(pricing *ModelPricing) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.pricing[pricing.Model] = pricing
}

// SetOverride adds or updates a tenant pricing override.
func (pt *PricingTable) SetOverride(override *TenantPricingOverride) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	key := override.TenantID + ":" + override.Model
	pt.overrides[key] = override
}

// RemoveOverride removes a tenant pricing override.
func (pt *PricingTable) RemoveOverride(tenantID, model string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	key := tenantID + ":" + model
	delete(pt.overrides, key)
}

// ListPricing returns all base pricing.
func (pt *PricingTable) ListPricing() []*ModelPricing {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	result := make([]*ModelPricing, 0, len(pt.pricing))
	for _, p := range pt.pricing {
		result = append(result, p)
	}
	return result
}

// ListOverrides returns all tenant overrides.
func (pt *PricingTable) ListOverrides() []*TenantPricingOverride {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	result := make([]*TenantPricingOverride, 0, len(pt.overrides))
	for _, o := range pt.overrides {
		result = append(result, o)
	}
	return result
}

// GetTenantOverrides returns all overrides for a specific tenant.
func (pt *PricingTable) GetTenantOverrides(tenantID string) []*TenantPricingOverride {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	var result []*TenantPricingOverride
	prefix := tenantID + ":"
	for key, override := range pt.overrides {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, override)
		}
	}
	return result
}

// EstimateCost estimates the cost for a request before execution.
func (pt *PricingTable) EstimateCost(model, tenantID string, estimatedInputTokens, estimatedOutputTokens int) *EstimatedCost {
	pricing := pt.GetPricing(model, tenantID)
	cost := pricing.CalculateCost(estimatedInputTokens, estimatedOutputTokens)

	return &EstimatedCost{
		Model:                 model,
		EstimatedInputTokens:  estimatedInputTokens,
		EstimatedOutputTokens: estimatedOutputTokens,
		EstimatedCost:         cost,
		Confidence:            0.8, // Default confidence
	}
}
