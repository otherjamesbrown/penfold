package activities

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestBug_pf322dc7_LifecycleEventValidation reproduces bug pf-322dc7.
// Root cause: PersistFindings validates lifecycle_change values against a hardcoded map
// (validLifecycleEvents in persist_repo.go:55-66) with 10 allowed values.
// The LLM may generate "identified" or "confirmed" as lifecycle_change values, which are
// rejected by the validation at line 184. The entire transaction is rejected when these appear.
//
// Expected behavior after fix:
// - "identified" and "confirmed" should be added to validLifecycleEvents map
// - Risk references with lifecycle_event="identified" or "confirmed" should pass validation
// - These legitimate LLM outputs should be persisted successfully
func TestBug_pf322dc7_LifecycleEventValidation(t *testing.T) {
	repo := &PersistRepo{
		logger: logging.NewLogger(&logging.Config{Level: logging.LevelError}),
	}

	t.Run("lifecycle_event 'identified' should be accepted", func(t *testing.T) {
		// BUG REPRODUCTION: LLM generates lifecycle_event='identified' for newly identified risks
		// but validateInput rejects it as invalid

		identifiedEvent := "identified"
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 123,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:     "Security vulnerability identified in auth module",
						ContextExcerpt:  "Alice identified a potential SQL injection vulnerability",
						Significance:    "primary",
						LifecycleChange: &identifiedEvent, // LLM output: risk was "identified"
						IsNew:           true,
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}

		// Call validateInput directly (same validation used by PersistFindings)
		err := repo.validateInput(input)

		// DESIRED BEHAVIOR: This validation should SUCCEED
		// This test will FAIL until the fix is applied
		require.NoError(t, err, "lifecycle_event='identified' should be ACCEPTED (test will fail until fix applied)")

		t.Logf("DESIRED: validateInput accepts lifecycle_event='identified'")
		t.Logf("FIX REQUIRED: Add 'identified' to validLifecycleEvents map in persist_repo.go:55-66")
	})

	t.Run("lifecycle_event 'confirmed' should be accepted", func(t *testing.T) {
		// BUG REPRODUCTION: LLM generates lifecycle_event='confirmed' for confirmed risks
		// but validateInput rejects it as invalid

		confirmedEvent := "confirmed"
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 456,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:     "Performance degradation confirmed in production",
						ContextExcerpt:  "Bob confirmed the latency issue affects all users",
						Significance:    "secondary",
						LifecycleChange: &confirmedEvent, // LLM output: risk was "confirmed"
						IsNew:           false,
						RootID:          int64Ptr(50),
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}

		// Call validateInput directly
		err := repo.validateInput(input)

		// DESIRED BEHAVIOR: This validation should SUCCEED
		// This test will FAIL until the fix is applied
		require.NoError(t, err, "lifecycle_event='confirmed' should be ACCEPTED (test will fail until fix applied)")

		t.Logf("DESIRED: validateInput accepts lifecycle_event='confirmed'")
		t.Logf("FIX REQUIRED: Add 'confirmed' to validLifecycleEvents map in persist_repo.go:55-66")
	})

	t.Run("control test: 'escalated' should still be accepted", func(t *testing.T) {
		// Control test: verify that existing valid lifecycle events still work
		// This should PASS both before and after the fix

		escalatedEvent := "escalated"
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 789,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:     "Risk escalated to management",
						ContextExcerpt:  "This issue has been escalated to senior leadership",
						Significance:    "primary",
						LifecycleChange: &escalatedEvent, // Valid existing value
						IsNew:           false,
						RootID:          int64Ptr(75),
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}

		err := repo.validateInput(input)

		// This should always pass (control)
		require.NoError(t, err, "lifecycle_event='escalated' should be ACCEPTED (existing valid value)")

		t.Logf("CONTROL: 'escalated' is already a valid lifecycle event and should continue to work")
	})

	t.Run("invalid lifecycle events should still be rejected", func(t *testing.T) {
		// Ensure validation still catches truly invalid values
		// This should PASS both before and after the fix

		invalidEvent := "totally_made_up_value"
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 999,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:     "Some risk",
						ContextExcerpt:  "Context here",
						Significance:    "primary",
						LifecycleChange: &invalidEvent, // Invalid value
						IsNew:           true,
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}

		err := repo.validateInput(input)

		// This should always fail (validation should still catch invalid values)
		require.Error(t, err, "lifecycle_event='totally_made_up_value' should be REJECTED")
		assert.Contains(t, err.Error(), "invalid lifecycle_event", "Error should mention invalid lifecycle_event")

		t.Logf("CONTROL: Truly invalid lifecycle events should still be rejected after fix")
	})

	t.Run("documented valid lifecycle events after fix", func(t *testing.T) {
		// This test documents the complete set of valid lifecycle events AFTER the fix

		// AFTER FIX: All these lifecycle events should be valid
		expectedValidEventsAfterFix := []string{
			"new",          // Existing (added in pf-4a3aba fix)
			"identified",   // MISSING - needs to be added for pf-322dc7
			"confirmed",    // MISSING - needs to be added for pf-322dc7
			"raised",       // Existing
			"updated",      // Existing
			"escalated",    // Existing
			"de_escalated", // Existing
			"assigned",     // Existing
			"decided",      // Existing
			"deferred",     // Existing
			"resolved",     // Existing
			"reopened",     // Existing
		}

		// Test each expected event
		for _, event := range expectedValidEventsAfterFix {
			eventCopy := event
			input := &PersistFindingsInput{
				TenantID: "tenant-1",
				SourceID: 123,
				Analysis: &DeepAnalyzeOutput{
					RiskReferences: []RiskReferenceOutput{
						{
							Description:     "Test risk for " + event,
							ContextExcerpt:  "Context for " + event,
							Significance:    "primary",
							LifecycleChange: &eventCopy,
							IsNew:           true,
						},
					},
				},
				ResolvedPeople: map[string]int64{},
			}

			err := repo.validateInput(input)

			if event == "identified" || event == "confirmed" {
				// These will FAIL until the fix is applied
				assert.NoError(t, err, "After fix: '%s' should be a valid lifecycle event (test will fail until fix applied)", event)
			} else {
				// These should already pass (existing valid values)
				assert.NoError(t, err, "'%s' should be a valid lifecycle event", event)
			}
		}

		t.Logf("AFTER FIX: validLifecycleEvents should include: %v", expectedValidEventsAfterFix)
		t.Logf("FIX: Add 'identified': true and 'confirmed': true to validLifecycleEvents map in persist_repo.go:55-66")
	})

	t.Run("'identified' is not currently in validLifecycleEvents map", func(t *testing.T) {
		// Verify the bug exists: 'identified' is missing from the validation map
		isValid := validLifecycleEvents["identified"]

		// This assertion FAILS (reproduces the bug) - 'identified' is not in the map
		assert.True(t, isValid, "BUG pf-322dc7: 'identified' should be in validLifecycleEvents (will fail until fixed)")

		t.Logf("REPRODUCTION: 'identified' is NOT in validLifecycleEvents map")
		t.Logf("ROOT CAUSE: Map at persist_repo.go:55-66 is missing 'identified' entry")
	})

	t.Run("'confirmed' is not currently in validLifecycleEvents map", func(t *testing.T) {
		// Verify the bug exists: 'confirmed' is missing from the validation map
		isValid := validLifecycleEvents["confirmed"]

		// This assertion FAILS (reproduces the bug) - 'confirmed' is not in the map
		assert.True(t, isValid, "BUG pf-322dc7: 'confirmed' should be in validLifecycleEvents (will fail until fixed)")

		t.Logf("REPRODUCTION: 'confirmed' is NOT in validLifecycleEvents map")
		t.Logf("ROOT CAUSE: Map at persist_repo.go:55-66 is missing 'confirmed' entry")
	})
}

// Helper function to create int64 pointers
func int64Ptr(i int64) *int64 {
	return &i
}
