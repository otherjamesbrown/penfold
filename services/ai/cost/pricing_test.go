package cost

import (
	"testing"
)

func TestNewPricingTable(t *testing.T) {
	pt := NewPricingTable()
	if pt == nil {
		t.Fatal("Expected pricing table to be created")
	}

	// Check that default models are loaded
	pricing := pt.GetPricing("gemini-1.5-pro", "")
	if pricing == nil {
		t.Fatal("Expected gemini-1.5-pro pricing to exist")
	}

	if pricing.InputCostPer1K <= 0 {
		t.Error("Expected positive input cost for gemini-1.5-pro")
	}

	// Check local model
	localPricing := pt.GetPricing("llama3.2", "")
	if !localPricing.IsLocal {
		t.Error("Expected llama3.2 to be marked as local")
	}
	if localPricing.InputCostPer1K != 0 {
		t.Error("Expected local model to have zero cost")
	}
}

func TestModelPricing_CalculateCost(t *testing.T) {
	tests := []struct {
		name         string
		pricing      *ModelPricing
		inputTokens  int
		outputTokens int
		expectedCost float64
	}{
		{
			name: "cloud model with tokens",
			pricing: &ModelPricing{
				Model:           "test-model",
				InputCostPer1K:  0.001,  // $1 per 1M tokens
				OutputCostPer1K: 0.002,  // $2 per 1M tokens
				IsLocal:         false,
			},
			inputTokens:  1000,
			outputTokens: 500,
			expectedCost: 0.001 + 0.001, // 0.002
		},
		{
			name: "local model (free)",
			pricing: &ModelPricing{
				Model:   "local-model",
				IsLocal: true,
			},
			inputTokens:  10000,
			outputTokens: 5000,
			expectedCost: 0,
		},
		{
			name: "zero tokens",
			pricing: &ModelPricing{
				Model:           "test-model",
				InputCostPer1K:  0.001,
				OutputCostPer1K: 0.002,
				IsLocal:         false,
			},
			inputTokens:  0,
			outputTokens: 0,
			expectedCost: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := tt.pricing.CalculateCost(tt.inputTokens, tt.outputTokens)
			if cost != tt.expectedCost {
				t.Errorf("Expected cost %f, got %f", tt.expectedCost, cost)
			}
		})
	}
}

func TestPricingTable_TenantOverride(t *testing.T) {
	pt := NewPricingTable()

	// Get base pricing
	basePricing := pt.GetPricing("gemini-1.5-pro", "")
	baseInputCost := basePricing.InputCostPer1K

	// Add tenant override with 50% discount
	pt.SetOverride(&TenantPricingOverride{
		TenantID:        "premium-tenant",
		Model:           "gemini-1.5-pro",
		DiscountPercent: 50,
	})

	// Get tenant-specific pricing
	tenantPricing := pt.GetPricing("gemini-1.5-pro", "premium-tenant")

	expectedCost := baseInputCost * 0.5
	if tenantPricing.InputCostPer1K != expectedCost {
		t.Errorf("Expected discounted input cost %f, got %f", expectedCost, tenantPricing.InputCostPer1K)
	}

	// Non-premium tenant should get base pricing
	regularPricing := pt.GetPricing("gemini-1.5-pro", "regular-tenant")
	if regularPricing.InputCostPer1K != baseInputCost {
		t.Errorf("Regular tenant should get base pricing, expected %f, got %f", baseInputCost, regularPricing.InputCostPer1K)
	}
}

func TestPricingTable_CustomPricing(t *testing.T) {
	pt := NewPricingTable()

	customInput := 0.0005
	customOutput := 0.001

	pt.SetOverride(&TenantPricingOverride{
		TenantID:              "custom-tenant",
		Model:                 "gemini-1.5-pro",
		CustomInputCostPer1K:  &customInput,
		CustomOutputCostPer1K: &customOutput,
	})

	pricing := pt.GetPricing("gemini-1.5-pro", "custom-tenant")

	if pricing.InputCostPer1K != customInput {
		t.Errorf("Expected custom input cost %f, got %f", customInput, pricing.InputCostPer1K)
	}

	if pricing.OutputCostPer1K != customOutput {
		t.Errorf("Expected custom output cost %f, got %f", customOutput, pricing.OutputCostPer1K)
	}
}

func TestPricingTable_UnknownModel(t *testing.T) {
	pt := NewPricingTable()

	pricing := pt.GetPricing("unknown-model-xyz", "")
	if pricing == nil {
		t.Fatal("Expected pricing to be returned for unknown model")
	}

	if pricing.Model != "unknown-model-xyz" {
		t.Errorf("Expected model name to be preserved")
	}

	if pricing.Provider != "unknown" {
		t.Errorf("Expected provider to be 'unknown', got '%s'", pricing.Provider)
	}
}

func TestPricingTable_EstimateCost(t *testing.T) {
	pt := NewPricingTable()

	estimate := pt.EstimateCost("gemini-1.5-flash", "", 2000, 1000)

	if estimate == nil {
		t.Fatal("Expected estimate to be returned")
	}

	if estimate.Model != "gemini-1.5-flash" {
		t.Errorf("Expected model 'gemini-1.5-flash', got '%s'", estimate.Model)
	}

	if estimate.EstimatedInputTokens != 2000 {
		t.Errorf("Expected 2000 input tokens, got %d", estimate.EstimatedInputTokens)
	}

	if estimate.EstimatedOutputTokens != 1000 {
		t.Errorf("Expected 1000 output tokens, got %d", estimate.EstimatedOutputTokens)
	}

	// Cost should be positive for cloud model
	if estimate.EstimatedCost <= 0 {
		t.Error("Expected positive estimated cost for cloud model")
	}

	if estimate.Confidence != 0.8 {
		t.Errorf("Expected confidence 0.8, got %f", estimate.Confidence)
	}
}

func TestPricingTable_ListOperations(t *testing.T) {
	pt := NewPricingTable()

	// List pricing
	pricingList := pt.ListPricing()
	if len(pricingList) == 0 {
		t.Error("Expected pricing list to contain items")
	}

	// Add override and list
	pt.SetOverride(&TenantPricingOverride{
		TenantID:        "test-tenant",
		Model:           "gemini-1.5-pro",
		DiscountPercent: 10,
	})

	overrides := pt.ListOverrides()
	if len(overrides) != 1 {
		t.Errorf("Expected 1 override, got %d", len(overrides))
	}

	// Get tenant overrides
	tenantOverrides := pt.GetTenantOverrides("test-tenant")
	if len(tenantOverrides) != 1 {
		t.Errorf("Expected 1 tenant override, got %d", len(tenantOverrides))
	}

	// Remove override
	pt.RemoveOverride("test-tenant", "gemini-1.5-pro")
	overrides = pt.ListOverrides()
	if len(overrides) != 0 {
		t.Errorf("Expected 0 overrides after removal, got %d", len(overrides))
	}
}
