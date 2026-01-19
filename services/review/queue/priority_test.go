package queue

import (
	"testing"
	"time"

	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/reviewv1"
)

// =============================================================================
// TimePriority Tests
// =============================================================================

func TestNewTimePriority(t *testing.T) {
	p := NewTimePriority()

	if p.BaseWeight != 1.0 {
		t.Errorf("Expected BaseWeight 1.0, got %f", p.BaseWeight)
	}
	if p.DecayHalfLife != 24*time.Hour {
		t.Errorf("Expected DecayHalfLife 24h, got %v", p.DecayHalfLife)
	}
}

func TestNewTimePriorityWithConfig(t *testing.T) {
	t.Run("CustomValues", func(t *testing.T) {
		p := NewTimePriorityWithConfig(2.0, 12*time.Hour)
		if p.BaseWeight != 2.0 {
			t.Errorf("Expected BaseWeight 2.0, got %f", p.BaseWeight)
		}
		if p.DecayHalfLife != 12*time.Hour {
			t.Errorf("Expected DecayHalfLife 12h, got %v", p.DecayHalfLife)
		}
	})

	t.Run("InvalidValues", func(t *testing.T) {
		p := NewTimePriorityWithConfig(-1.0, -time.Hour)
		if p.BaseWeight != 1.0 {
			t.Errorf("Expected BaseWeight 1.0 for invalid input, got %f", p.BaseWeight)
		}
		if p.DecayHalfLife != 24*time.Hour {
			t.Errorf("Expected DecayHalfLife 24h for invalid input, got %v", p.DecayHalfLife)
		}
	})
}

func TestTimePriority_Calculate(t *testing.T) {
	p := NewTimePriority()

	t.Run("NilItem", func(t *testing.T) {
		priority := p.Calculate(nil)
		if priority != 0 {
			t.Errorf("Expected 0 for nil item, got %f", priority)
		}
	})

	t.Run("NewItem", func(t *testing.T) {
		item := &QueueItem{
			EnqueuedAt: time.Now(),
		}
		priority := p.Calculate(item)
		// Should be close to BaseWeight for new items
		if priority < 0.9 || priority > 1.1 {
			t.Errorf("Expected priority ~1.0 for new item, got %f", priority)
		}
	})

	t.Run("OldItem", func(t *testing.T) {
		item := &QueueItem{
			EnqueuedAt: time.Now().Add(-24 * time.Hour),
		}
		priority := p.Calculate(item)
		// Should be ~2.0 after one half-life (24h)
		if priority < 1.9 || priority > 2.1 {
			t.Errorf("Expected priority ~2.0 after one half-life, got %f", priority)
		}
	})

	t.Run("VeryOldItem", func(t *testing.T) {
		item := &QueueItem{
			EnqueuedAt: time.Now().Add(-48 * time.Hour),
		}
		priority := p.Calculate(item)
		// Should be ~4.0 after two half-lives
		if priority < 3.8 || priority > 4.2 {
			t.Errorf("Expected priority ~4.0 after two half-lives, got %f", priority)
		}
	})

	t.Run("FutureEnqueueTime", func(t *testing.T) {
		item := &QueueItem{
			EnqueuedAt: time.Now().Add(time.Hour),
		}
		priority := p.Calculate(item)
		// Should handle future times gracefully (age = 0)
		if priority < 0.9 || priority > 1.1 {
			t.Errorf("Expected priority ~1.0 for future enqueue time, got %f", priority)
		}
	})
}

func TestTimePriority_Name(t *testing.T) {
	p := NewTimePriority()
	if p.Name() != "time_priority" {
		t.Errorf("Expected 'time_priority', got '%s'", p.Name())
	}
}

// =============================================================================
// ImportancePriority Tests
// =============================================================================

func TestNewImportancePriority(t *testing.T) {
	p := NewImportancePriority()

	// Check default priority weights
	if p.PriorityWeights[reviewv1.Priority_PRIORITY_LOW] != 2.0 {
		t.Errorf("Expected LOW priority weight 2.0, got %f", p.PriorityWeights[reviewv1.Priority_PRIORITY_LOW])
	}
	if p.PriorityWeights[reviewv1.Priority_PRIORITY_URGENT] != 16.0 {
		t.Errorf("Expected URGENT priority weight 16.0, got %f", p.PriorityWeights[reviewv1.Priority_PRIORITY_URGENT])
	}

	// Check default content type weights
	if p.ContentTypeWeights["email"] != 1.0 {
		t.Errorf("Expected email weight 1.0, got %f", p.ContentTypeWeights["email"])
	}
	if p.ContentTypeWeights["alert"] != 2.0 {
		t.Errorf("Expected alert weight 2.0, got %f", p.ContentTypeWeights["alert"])
	}
}

func TestNewImportancePriorityWithWeights(t *testing.T) {
	customPriority := map[reviewv1.Priority]float64{
		reviewv1.Priority_PRIORITY_LOW: 1.0,
	}
	customContent := map[string]float64{
		"custom": 5.0,
	}
	customCategory := map[string]float64{
		"custom_cat": 3.0,
	}

	p := NewImportancePriorityWithWeights(customPriority, customContent, customCategory)

	if p.PriorityWeights[reviewv1.Priority_PRIORITY_LOW] != 1.0 {
		t.Errorf("Expected custom LOW weight 1.0, got %f", p.PriorityWeights[reviewv1.Priority_PRIORITY_LOW])
	}
	if p.ContentTypeWeights["custom"] != 5.0 {
		t.Errorf("Expected custom content weight 5.0, got %f", p.ContentTypeWeights["custom"])
	}
	if p.CategoryWeights["custom_cat"] != 3.0 {
		t.Errorf("Expected custom category weight 3.0, got %f", p.CategoryWeights["custom_cat"])
	}
}

func TestImportancePriority_Calculate(t *testing.T) {
	p := NewImportancePriority()

	t.Run("NilItem", func(t *testing.T) {
		priority := p.Calculate(nil)
		if priority != 0 {
			t.Errorf("Expected 0 for nil item, got %f", priority)
		}
	})

	t.Run("LowPriorityEmail", func(t *testing.T) {
		item := &QueueItem{
			OriginalPriority: reviewv1.Priority_PRIORITY_LOW,
			ContentType:      "email",
			Category:         "communications",
		}
		priority := p.Calculate(item)
		// LOW (2.0) * email (1.0) * communications (1.0) = 2.0
		if priority != 2.0 {
			t.Errorf("Expected priority 2.0, got %f", priority)
		}
	})

	t.Run("HighPriorityTask", func(t *testing.T) {
		item := &QueueItem{
			OriginalPriority: reviewv1.Priority_PRIORITY_HIGH,
			ContentType:      "task",
			Category:         "tasks",
		}
		priority := p.Calculate(item)
		// HIGH (8.0) * task (1.5) * tasks (1.2) = 14.4
		// Use tolerance for floating point comparison
		expected := 14.4
		if priority < expected-0.001 || priority > expected+0.001 {
			t.Errorf("Expected priority ~14.4, got %f", priority)
		}
	})

	t.Run("UrgentAlert", func(t *testing.T) {
		item := &QueueItem{
			OriginalPriority: reviewv1.Priority_PRIORITY_URGENT,
			ContentType:      "alert",
			Category:         "alerts",
		}
		priority := p.Calculate(item)
		// URGENT (16.0) * alert (2.0) * alerts (1.5) = 48.0
		if priority != 48.0 {
			t.Errorf("Expected priority 48.0, got %f", priority)
		}
	})

	t.Run("UnknownTypes", func(t *testing.T) {
		item := &QueueItem{
			OriginalPriority: reviewv1.Priority_PRIORITY_MEDIUM,
			ContentType:      "unknown_type",
			Category:         "unknown_category",
		}
		priority := p.Calculate(item)
		// MEDIUM (4.0) * unknown (1.0) * unknown (1.0) = 4.0
		if priority != 4.0 {
			t.Errorf("Expected priority 4.0, got %f", priority)
		}
	})
}

func TestImportancePriority_Name(t *testing.T) {
	p := NewImportancePriority()
	if p.Name() != "importance_priority" {
		t.Errorf("Expected 'importance_priority', got '%s'", p.Name())
	}
}

// =============================================================================
// DeferralPenalty Tests
// =============================================================================

func TestNewDeferralPenalty(t *testing.T) {
	p := NewDeferralPenalty()

	if p.PenaltyPerDefer != 0.8 {
		t.Errorf("Expected PenaltyPerDefer 0.8, got %f", p.PenaltyPerDefer)
	}
	if p.MaxPenalty != 0.25 {
		t.Errorf("Expected MaxPenalty 0.25, got %f", p.MaxPenalty)
	}
}

func TestNewDeferralPenaltyWithConfig(t *testing.T) {
	t.Run("CustomValues", func(t *testing.T) {
		p := NewDeferralPenaltyWithConfig(0.9, 0.1)
		if p.PenaltyPerDefer != 0.9 {
			t.Errorf("Expected PenaltyPerDefer 0.9, got %f", p.PenaltyPerDefer)
		}
		if p.MaxPenalty != 0.1 {
			t.Errorf("Expected MaxPenalty 0.1, got %f", p.MaxPenalty)
		}
	})

	t.Run("InvalidValues", func(t *testing.T) {
		p := NewDeferralPenaltyWithConfig(-0.5, 2.0)
		if p.PenaltyPerDefer != 0.8 {
			t.Errorf("Expected default PenaltyPerDefer 0.8, got %f", p.PenaltyPerDefer)
		}
		if p.MaxPenalty != 0.25 {
			t.Errorf("Expected default MaxPenalty 0.25, got %f", p.MaxPenalty)
		}
	})
}

func TestDeferralPenalty_Calculate(t *testing.T) {
	p := NewDeferralPenalty()

	t.Run("NilItem", func(t *testing.T) {
		result := p.Calculate(nil)
		if result != 1.0 {
			t.Errorf("Expected 1.0 for nil item, got %f", result)
		}
	})

	t.Run("NoDeferrals", func(t *testing.T) {
		item := &QueueItem{DeferCount: 0}
		result := p.Calculate(item)
		if result != 1.0 {
			t.Errorf("Expected 1.0 for no deferrals, got %f", result)
		}
	})

	t.Run("OneDeferral", func(t *testing.T) {
		item := &QueueItem{DeferCount: 1}
		result := p.Calculate(item)
		// 0.8^1 = 0.8
		if result != 0.8 {
			t.Errorf("Expected 0.8 for one deferral, got %f", result)
		}
	})

	t.Run("TwoDeferrals", func(t *testing.T) {
		item := &QueueItem{DeferCount: 2}
		result := p.Calculate(item)
		// 0.8^2 = 0.64
		// Use tolerance for floating point comparison
		expected := 0.64
		if result < expected-0.001 || result > expected+0.001 {
			t.Errorf("Expected ~0.64 for two deferrals, got %f", result)
		}
	})

	t.Run("ManyDeferrals_MaxPenalty", func(t *testing.T) {
		item := &QueueItem{DeferCount: 10}
		result := p.Calculate(item)
		// 0.8^10 = ~0.107, but should be capped at MaxPenalty (0.25)
		if result != 0.25 {
			t.Errorf("Expected 0.25 (max penalty) for many deferrals, got %f", result)
		}
	})
}

func TestDeferralPenalty_Name(t *testing.T) {
	p := NewDeferralPenalty()
	if p.Name() != "deferral_penalty" {
		t.Errorf("Expected 'deferral_penalty', got '%s'", p.Name())
	}
}

// =============================================================================
// MixedPriority Tests
// =============================================================================

func TestNewMixedPriority(t *testing.T) {
	t.Run("NormalWeights", func(t *testing.T) {
		p := NewMixedPriority(0.4, 0.4, 0.2)
		if p.TimeWeight != 0.4 {
			t.Errorf("Expected TimeWeight 0.4, got %f", p.TimeWeight)
		}
		if p.ImportanceWeight != 0.4 {
			t.Errorf("Expected ImportanceWeight 0.4, got %f", p.ImportanceWeight)
		}
		if p.DeferralWeight != 0.2 {
			t.Errorf("Expected DeferralWeight 0.2, got %f", p.DeferralWeight)
		}
	})

	t.Run("WeightsNormalized", func(t *testing.T) {
		p := NewMixedPriority(2.0, 2.0, 1.0)
		// Total = 5.0, so weights should be 0.4, 0.4, 0.2
		if p.TimeWeight != 0.4 {
			t.Errorf("Expected normalized TimeWeight 0.4, got %f", p.TimeWeight)
		}
	})

	t.Run("ZeroWeights", func(t *testing.T) {
		p := NewMixedPriority(0, 0, 0)
		// Should use defaults when all weights are zero
		total := p.TimeWeight + p.ImportanceWeight + p.DeferralWeight
		if total < 0.99 || total > 1.01 {
			t.Errorf("Expected weights to sum to 1.0, got %f", total)
		}
	})
}

func TestMixedPriority_Calculate(t *testing.T) {
	p := NewMixedPriority(0.5, 0.5, 0.0) // No deferral weight for simpler testing
	now := time.Now()

	t.Run("NilItem", func(t *testing.T) {
		result := p.Calculate(nil)
		if result != 0 {
			t.Errorf("Expected 0 for nil item, got %f", result)
		}
	})

	t.Run("NewHighPriorityItem", func(t *testing.T) {
		item := &QueueItem{
			EnqueuedAt:       now,
			OriginalPriority: reviewv1.Priority_PRIORITY_HIGH,
			ContentType:      "email",
			Category:         "communications",
		}
		result := p.Calculate(item)
		// Time priority ~1.0, Importance = 8.0
		// Weighted: 0.5*1.0 + 0.5*8.0 = 4.5
		if result < 4.0 || result > 5.0 {
			t.Errorf("Expected priority ~4.5, got %f", result)
		}
	})

	t.Run("OldLowPriorityItem", func(t *testing.T) {
		item := &QueueItem{
			EnqueuedAt:       now.Add(-48 * time.Hour), // 2 days old
			OriginalPriority: reviewv1.Priority_PRIORITY_LOW,
			ContentType:      "document",
			Category:         "communications",
		}
		result := p.Calculate(item)
		// Time priority ~4.0, Importance = 2.0 * 0.8 * 1.0 = 1.6
		// Weighted: 0.5*4.0 + 0.5*1.6 = 2.8
		if result < 2.5 || result > 3.5 {
			t.Errorf("Expected priority ~2.8, got %f", result)
		}
	})
}

func TestMixedPriority_WithDeferral(t *testing.T) {
	p := NewMixedPriority(0.4, 0.4, 0.2)
	now := time.Now()

	item := &QueueItem{
		EnqueuedAt:       now,
		OriginalPriority: reviewv1.Priority_PRIORITY_MEDIUM,
		ContentType:      "email",
		Category:         "communications",
		DeferCount:       0,
	}

	basePriority := p.Calculate(item)

	item.DeferCount = 2
	deferredPriority := p.Calculate(item)

	if deferredPriority >= basePriority {
		t.Errorf("Deferred item should have lower priority: base=%f, deferred=%f", basePriority, deferredPriority)
	}
}

func TestMixedPriority_Name(t *testing.T) {
	p := NewMixedPriority(0.4, 0.4, 0.2)
	if p.Name() != "mixed_priority" {
		t.Errorf("Expected 'mixed_priority', got '%s'", p.Name())
	}
}

// =============================================================================
// SourcePriority Tests
// =============================================================================

func TestNewSourcePriority(t *testing.T) {
	p := NewSourcePriority()

	if p.SourceWeights["gmail"] != 1.2 {
		t.Errorf("Expected gmail weight 1.2, got %f", p.SourceWeights["gmail"])
	}
	if p.SourceWeights["slack"] != 1.5 {
		t.Errorf("Expected slack weight 1.5, got %f", p.SourceWeights["slack"])
	}
	if p.DefaultWeight != 1.0 {
		t.Errorf("Expected DefaultWeight 1.0, got %f", p.DefaultWeight)
	}
}

func TestSourcePriority_Calculate(t *testing.T) {
	p := NewSourcePriority()

	t.Run("NilItem", func(t *testing.T) {
		result := p.Calculate(nil)
		if result != 1.0 {
			t.Errorf("Expected 1.0 (default) for nil item, got %f", result)
		}
	})

	t.Run("KnownSource", func(t *testing.T) {
		item := &QueueItem{Source: "slack"}
		result := p.Calculate(item)
		if result != 1.5 {
			t.Errorf("Expected 1.5 for slack, got %f", result)
		}
	})

	t.Run("UnknownSource", func(t *testing.T) {
		item := &QueueItem{Source: "unknown_source"}
		result := p.Calculate(item)
		if result != 1.0 {
			t.Errorf("Expected 1.0 (default) for unknown source, got %f", result)
		}
	})
}

func TestSourcePriority_Name(t *testing.T) {
	p := NewSourcePriority()
	if p.Name() != "source_priority" {
		t.Errorf("Expected 'source_priority', got '%s'", p.Name())
	}
}

// =============================================================================
// CompositePriority Tests
// =============================================================================

func TestNewCompositePriority(t *testing.T) {
	time := NewTimePriority()
	importance := NewImportancePriority()

	p := NewCompositePriority(NormalizeWeightedSum,
		WeightedCalculator{Calculator: time, Weight: 0.5},
		WeightedCalculator{Calculator: importance, Weight: 0.5},
	)

	if len(p.Calculators) != 2 {
		t.Errorf("Expected 2 calculators, got %d", len(p.Calculators))
	}
	if p.Normalizer != NormalizeWeightedSum {
		t.Errorf("Expected NormalizeWeightedSum, got %v", p.Normalizer)
	}
}

func TestCompositePriority_AddCalculator(t *testing.T) {
	p := NewCompositePriority(NormalizeWeightedSum)
	if len(p.Calculators) != 0 {
		t.Errorf("Expected 0 calculators, got %d", len(p.Calculators))
	}

	p.AddCalculator(NewTimePriority(), 0.5)
	if len(p.Calculators) != 1 {
		t.Errorf("Expected 1 calculator, got %d", len(p.Calculators))
	}
}

func TestCompositePriority_Calculate(t *testing.T) {
	now := time.Now()
	item := &QueueItem{
		EnqueuedAt:       now,
		OriginalPriority: reviewv1.Priority_PRIORITY_MEDIUM,
		ContentType:      "email",
		Category:         "communications",
	}

	t.Run("NilItem", func(t *testing.T) {
		p := NewCompositePriority(NormalizeWeightedSum,
			WeightedCalculator{Calculator: NewTimePriority(), Weight: 1.0},
		)
		result := p.Calculate(nil)
		if result != 0 {
			t.Errorf("Expected 0 for nil item, got %f", result)
		}
	})

	t.Run("NoCalculators", func(t *testing.T) {
		p := NewCompositePriority(NormalizeWeightedSum)
		result := p.Calculate(item)
		if result != 0 {
			t.Errorf("Expected 0 for no calculators, got %f", result)
		}
	})

	t.Run("WeightedSum", func(t *testing.T) {
		p := NewCompositePriority(NormalizeWeightedSum,
			WeightedCalculator{Calculator: NewTimePriority(), Weight: 0.5},
			WeightedCalculator{Calculator: NewImportancePriority(), Weight: 0.5},
		)
		result := p.Calculate(item)
		if result <= 0 {
			t.Errorf("Expected positive result for weighted sum, got %f", result)
		}
	})

	t.Run("Max", func(t *testing.T) {
		p := NewCompositePriority(NormalizeMax,
			WeightedCalculator{Calculator: NewTimePriority(), Weight: 1.0},
			WeightedCalculator{Calculator: NewImportancePriority(), Weight: 1.0},
		)
		result := p.Calculate(item)
		// Should be the higher of time (~1.0) and importance (4.0)
		if result < 3.5 {
			t.Errorf("Expected max to be ~4.0, got %f", result)
		}
	})

	t.Run("Min", func(t *testing.T) {
		p := NewCompositePriority(NormalizeMin,
			WeightedCalculator{Calculator: NewTimePriority(), Weight: 1.0},
			WeightedCalculator{Calculator: NewImportancePriority(), Weight: 1.0},
		)
		result := p.Calculate(item)
		// Should be the lower of time (~1.0) and importance (4.0)
		if result > 1.5 {
			t.Errorf("Expected min to be ~1.0, got %f", result)
		}
	})

	t.Run("Product", func(t *testing.T) {
		p := NewCompositePriority(NormalizeProduct,
			WeightedCalculator{Calculator: NewTimePriority(), Weight: 1.0},
			WeightedCalculator{Calculator: NewImportancePriority(), Weight: 1.0},
		)
		result := p.Calculate(item)
		// Should be time (~1.0) * importance (4.0) = ~4.0
		if result < 3.5 || result > 4.5 {
			t.Errorf("Expected product to be ~4.0, got %f", result)
		}
	})
}

func TestCompositePriority_Name(t *testing.T) {
	p := NewCompositePriority(NormalizeWeightedSum)
	if p.Name() != "composite_priority" {
		t.Errorf("Expected 'composite_priority', got '%s'", p.Name())
	}
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestWeightedSum(t *testing.T) {
	t.Run("EqualWeights", func(t *testing.T) {
		scores := []float64{2.0, 4.0}
		weights := []float64{1.0, 1.0}
		result := weightedSum(scores, weights)
		// (1/2)*2 + (1/2)*4 = 3.0
		if result != 3.0 {
			t.Errorf("Expected 3.0, got %f", result)
		}
	})

	t.Run("UnequalWeights", func(t *testing.T) {
		scores := []float64{2.0, 4.0}
		weights := []float64{1.0, 3.0}
		result := weightedSum(scores, weights)
		// (1/4)*2 + (3/4)*4 = 0.5 + 3.0 = 3.5
		if result != 3.5 {
			t.Errorf("Expected 3.5, got %f", result)
		}
	})

	t.Run("ZeroWeights", func(t *testing.T) {
		scores := []float64{2.0, 4.0}
		weights := []float64{0.0, 0.0}
		result := weightedSum(scores, weights)
		// Should use equal weights: (2+4)/2 = 3.0
		if result != 3.0 {
			t.Errorf("Expected 3.0 for zero weights, got %f", result)
		}
	})
}

func TestMaxScore(t *testing.T) {
	t.Run("Normal", func(t *testing.T) {
		scores := []float64{1.0, 5.0, 3.0}
		result := maxScore(scores)
		if result != 5.0 {
			t.Errorf("Expected 5.0, got %f", result)
		}
	})

	t.Run("Empty", func(t *testing.T) {
		result := maxScore([]float64{})
		if result != 0 {
			t.Errorf("Expected 0 for empty slice, got %f", result)
		}
	})
}

func TestMinScore(t *testing.T) {
	t.Run("Normal", func(t *testing.T) {
		scores := []float64{5.0, 1.0, 3.0}
		result := minScore(scores)
		if result != 1.0 {
			t.Errorf("Expected 1.0, got %f", result)
		}
	})

	t.Run("Empty", func(t *testing.T) {
		result := minScore([]float64{})
		if result != 0 {
			t.Errorf("Expected 0 for empty slice, got %f", result)
		}
	})
}

func TestProductScore(t *testing.T) {
	t.Run("Normal", func(t *testing.T) {
		scores := []float64{2.0, 3.0, 4.0}
		result := productScore(scores)
		if result != 24.0 {
			t.Errorf("Expected 24.0, got %f", result)
		}
	})

	t.Run("Empty", func(t *testing.T) {
		result := productScore([]float64{})
		if result != 0 {
			t.Errorf("Expected 0 for empty slice, got %f", result)
		}
	})

	t.Run("WithZero", func(t *testing.T) {
		scores := []float64{2.0, 0.0, 4.0}
		result := productScore(scores)
		if result != 0 {
			t.Errorf("Expected 0 when containing zero, got %f", result)
		}
	})
}

// =============================================================================
// Interface Compliance Tests
// =============================================================================

func TestPriorityCalculator_Interface(t *testing.T) {
	calculators := []PriorityCalculator{
		NewTimePriority(),
		NewImportancePriority(),
		NewDeferralPenalty(),
		NewMixedPriority(0.4, 0.4, 0.2),
		NewSourcePriority(),
		NewCompositePriority(NormalizeWeightedSum),
	}

	item := &QueueItem{
		EnqueuedAt:       time.Now(),
		OriginalPriority: reviewv1.Priority_PRIORITY_MEDIUM,
		ContentType:      "email",
		Category:         "communications",
		Source:           "gmail",
	}

	for _, calc := range calculators {
		t.Run(calc.Name(), func(t *testing.T) {
			// Verify interface methods work
			name := calc.Name()
			if name == "" {
				t.Error("Name() should not return empty string")
			}

			priority := calc.Calculate(item)
			// All calculators should return non-negative values
			if priority < 0 {
				t.Errorf("Calculate() returned negative value: %f", priority)
			}
		})
	}
}
