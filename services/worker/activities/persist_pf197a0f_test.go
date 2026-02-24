package activities

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestBug_pf197a0f_RiskValidationExpansion reproduces bug pf-197a0f.
// Root cause: PersistFindings rejects valid risk lifecycle_event values
// (newly_identified, mitigated, no change) and significance value (low)
// produced by the analyze stage (gemini-2.5-pro).
func TestBug_pf197a0f_RiskValidationExpansion(t *testing.T) {
	repo := &PersistRepo{
		logger: logging.NewLogger(&logging.Config{Level: logging.LevelError}),
	}

	t.Run("lifecycle_event 'newly_identified' should be accepted", func(t *testing.T) {
		event := "newly_identified"
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 123,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:     "New supply chain risk identified",
						ContextExcerpt:  "Alice flagged a newly identified risk in vendor contracts",
						Significance:    "primary",
						LifecycleChange: &event,
						IsNew:           true,
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}
		err := repo.validateInput(input)
		require.NoError(t, err, "lifecycle_event='newly_identified' should be accepted")
	})

	t.Run("lifecycle_event 'mitigated' should be accepted", func(t *testing.T) {
		event := "mitigated"
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 456,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:     "Security vulnerability mitigated",
						ContextExcerpt:  "The patch was applied and risk mitigated",
						Significance:    "primary",
						LifecycleChange: &event,
						IsNew:           false,
						RootID:          int64Ptr(100),
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}
		err := repo.validateInput(input)
		require.NoError(t, err, "lifecycle_event='mitigated' should be accepted")
	})

	t.Run("lifecycle_event 'no change' with space should be normalized and accepted", func(t *testing.T) {
		event := "no change"
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 789,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:     "Ongoing risk unchanged",
						ContextExcerpt:  "No change in the compliance risk status",
						Significance:    "secondary",
						LifecycleChange: &event,
						IsNew:           false,
						RootID:          int64Ptr(200),
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}
		err := repo.validateInput(input)
		require.NoError(t, err, "lifecycle_event='no change' (with space) should be normalized to 'no_change' and accepted")
		assert.Equal(t, "no_change", event, "should be normalized to underscore form")
	})

	t.Run("significance 'low' should be accepted", func(t *testing.T) {
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 111,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:    "Minor risk noted",
						ContextExcerpt: "Bob mentioned a low-priority risk in passing",
						Significance:   "low",
						IsNew:          true,
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}
		err := repo.validateInput(input)
		require.NoError(t, err, "significance='low' should be accepted")
	})

	t.Run("significance 'medium' should be accepted", func(t *testing.T) {
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 222,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:    "Moderate risk",
						ContextExcerpt: "Medium-severity data quality issue flagged",
						Significance:   "medium",
						IsNew:          true,
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}
		err := repo.validateInput(input)
		require.NoError(t, err, "significance='medium' should be accepted")
	})

	t.Run("significance 'high' should be accepted", func(t *testing.T) {
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 333,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:    "Critical risk",
						ContextExcerpt: "High-severity compliance risk identified",
						Significance:   "high",
						IsNew:          true,
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}
		err := repo.validateInput(input)
		require.NoError(t, err, "significance='high' should be accepted")
	})

	t.Run("existing significance values still work", func(t *testing.T) {
		for _, sig := range []string{"primary", "secondary", "passing"} {
			input := &PersistFindingsInput{
				TenantID: "tenant-1",
				SourceID: 444,
				Analysis: &DeepAnalyzeOutput{
					RiskReferences: []RiskReferenceOutput{
						{
							Description:    "Test risk",
							ContextExcerpt: "Context",
							Significance:   sig,
							IsNew:          true,
						},
					},
				},
				ResolvedPeople: map[string]int64{},
			}
			err := repo.validateInput(input)
			require.NoError(t, err, "significance='%s' should still be accepted", sig)
		}
	})

	t.Run("invalid values still rejected", func(t *testing.T) {
		badEvent := "completely_bogus"
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 555,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:     "Test risk",
						ContextExcerpt:  "Context",
						Significance:    "primary",
						LifecycleChange: &badEvent,
						IsNew:           true,
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}
		err := repo.validateInput(input)
		require.Error(t, err, "invalid lifecycle_event should still be rejected")

		input2 := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 666,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:    "Test risk",
						ContextExcerpt: "Context",
						Significance:   "bogus_sig",
						IsNew:          true,
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}
		err = repo.validateInput(input2)
		require.Error(t, err, "invalid significance should still be rejected")
	})
}
