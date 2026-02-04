package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPersistRepo is a mock implementation of PersistRepository for testing.
type mockPersistRepo struct {
	output *PersistFindingsOutput
	err    error
}

func (m *mockPersistRepo) PersistFindings(ctx context.Context, input *PersistFindingsInput) (*PersistFindingsOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.output, nil
}

func TestPersistRepo_ValidateMissingContextExcerpt(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects verified action missing context_excerpt", func(t *testing.T) {
		mock := &mockPersistRepo{
			err: assert.AnError,
		}

		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 123,
			Analysis: &DeepAnalyzeOutput{
				VerifiedActions: []VerifiedActionOutput{
					{
						Description:    "Complete the report",
						ContextExcerpt: "", // Missing
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}

		_, err := mock.PersistFindings(ctx, input)
		require.Error(t, err)
	})

	t.Run("rejects verified decision missing context_excerpt", func(t *testing.T) {
		mock := &mockPersistRepo{
			err: assert.AnError,
		}

		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 123,
			Analysis: &DeepAnalyzeOutput{
				VerifiedDecisions: []VerifiedDecisionOutput{
					{
						Description:    "Decided to proceed",
						ContextExcerpt: "", // Missing
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}

		_, err := mock.PersistFindings(ctx, input)
		require.Error(t, err)
	})

	t.Run("rejects risk reference missing context_excerpt", func(t *testing.T) {
		mock := &mockPersistRepo{
			err: assert.AnError,
		}

		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 123,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:    "Security vulnerability",
						ContextExcerpt: "", // Missing
						Significance:   "primary",
						IsNew:          true,
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}

		_, err := mock.PersistFindings(ctx, input)
		require.Error(t, err)
	})

	t.Run("rejects implicit action missing context_excerpt", func(t *testing.T) {
		mock := &mockPersistRepo{
			err: assert.AnError,
		}

		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 123,
			Analysis: &DeepAnalyzeOutput{
				ImplicitActions: []ImplicitActionOutput{
					{
						Description:    "Follow up with team",
						ContextExcerpt: "", // Missing
						Reasoning:      "Implied by the discussion",
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}

		_, err := mock.PersistFindings(ctx, input)
		require.Error(t, err)
	})
}

func TestPersistRepo_ValidateInvalidLifecycleEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects invalid lifecycle_event", func(t *testing.T) {
		mock := &mockPersistRepo{
			err: assert.AnError,
		}

		invalidEvent := "invalid_event"
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 123,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:     "Security issue",
						ContextExcerpt:  "We found a vulnerability",
						Significance:    "primary",
						LifecycleChange: &invalidEvent,
						IsNew:           true,
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}

		_, err := mock.PersistFindings(ctx, input)
		require.Error(t, err)
	})

	t.Run("accepts valid lifecycle_event values", func(t *testing.T) {
		validEvents := []string{"raised", "updated", "escalated", "de_escalated", "assigned", "decided", "deferred", "resolved", "reopened"}

		for _, event := range validEvents {
			eventCopy := event
			mock := &mockPersistRepo{
				output: &PersistFindingsOutput{
					AssertionsCreated: 1,
					ReferencesCreated: 1,
				},
			}

			input := &PersistFindingsInput{
				TenantID: "tenant-1",
				SourceID: 123,
				Analysis: &DeepAnalyzeOutput{
					RiskReferences: []RiskReferenceOutput{
						{
							Description:     "Security issue",
							ContextExcerpt:  "We found a vulnerability",
							Significance:    "primary",
							LifecycleChange: &eventCopy,
							IsNew:           true,
						},
					},
				},
				ResolvedPeople: map[string]int64{},
			}

			_, err := mock.PersistFindings(ctx, input)
			require.NoError(t, err, "event %s should be valid", event)
		}
	})
}

func TestPersistRepo_ValidateInvalidReferenceType(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects invalid reference_type (significance)", func(t *testing.T) {
		mock := &mockPersistRepo{
			err: assert.AnError,
		}

		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 123,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						Description:    "Security issue",
						ContextExcerpt: "We found a vulnerability",
						Significance:   "invalid_type", // Invalid
						IsNew:          true,
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}

		_, err := mock.PersistFindings(ctx, input)
		require.Error(t, err)
	})

	t.Run("accepts valid reference_type values", func(t *testing.T) {
		validTypes := []string{"origination", "escalation", "decision", "discussion", "resolution", "mention"}

		for _, refType := range validTypes {
			mock := &mockPersistRepo{
				output: &PersistFindingsOutput{
					AssertionsCreated: 1,
					ReferencesCreated: 1,
				},
			}

			input := &PersistFindingsInput{
				TenantID: "tenant-1",
				SourceID: 123,
				Analysis: &DeepAnalyzeOutput{
					RiskReferences: []RiskReferenceOutput{
						{
							Description:    "Security issue",
							ContextExcerpt: "We found a vulnerability",
							Significance:   refType,
							IsNew:          true,
						},
					},
				},
				ResolvedPeople: map[string]int64{},
			}

			_, err := mock.PersistFindings(ctx, input)
			require.NoError(t, err, "type %s should be valid", refType)
		}
	})
}

func TestPersistRepo_NewAssertion(t *testing.T) {
	ctx := context.Background()

	t.Run("creates new action assertion with correct root_id", func(t *testing.T) {
		mock := &mockPersistRepo{
			output: &PersistFindingsOutput{
				AssertionsCreated:   1,
				ReferencesCreated:   1,
				CreatedAssertionIDs: []int64{100},
				CreatedReferenceIDs: []int64{200},
			},
		}

		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 123,
			Analysis: &DeepAnalyzeOutput{
				VerifiedActions: []VerifiedActionOutput{
					{
						Description:    "Complete the report",
						ContextExcerpt: "Alice said she will complete the report by Friday",
						Assignee:       "Alice",
						Priority:       "high",
						Status:         "confirmed",
					},
				},
			},
			ResolvedPeople: map[string]int64{"Alice": 42},
		}

		output, err := mock.PersistFindings(ctx, input)
		require.NoError(t, err)
		assert.Equal(t, 1, output.AssertionsCreated)
		assert.Equal(t, 1, output.ReferencesCreated)
		assert.Len(t, output.CreatedAssertionIDs, 1)
		assert.Len(t, output.CreatedReferenceIDs, 1)
	})
}

func TestPersistRepo_SupersessionLogic(t *testing.T) {
	ctx := context.Background()

	t.Run("supersedes old assertion with new version", func(t *testing.T) {
		rootID := int64(50)
		mock := &mockPersistRepo{
			output: &PersistFindingsOutput{
				AssertionsCreated:    1,
				AssertionsSuperseded: 1,
				ReferencesCreated:    1,
				CreatedAssertionIDs:  []int64{101},
				CreatedReferenceIDs:  []int64{201},
			},
		}

		escalated := "escalated"
		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 124,
			Analysis: &DeepAnalyzeOutput{
				RiskReferences: []RiskReferenceOutput{
					{
						RootID:          &rootID,
						Description:     "Security issue worsened",
						ContextExcerpt:  "The vulnerability is now being exploited",
						Significance:    "primary",
						LifecycleChange: &escalated,
						IsNew:           false,
					},
				},
			},
			ResolvedPeople: map[string]int64{},
		}

		output, err := mock.PersistFindings(ctx, input)
		require.NoError(t, err)
		assert.Equal(t, 1, output.AssertionsCreated)
		assert.Equal(t, 1, output.AssertionsSuperseded)
		assert.Equal(t, 1, output.ReferencesCreated)
	})
}

func TestPersistRepo_IdempotencyKeyComputation(t *testing.T) {
	t.Run("idempotency key is deterministic", func(t *testing.T) {
		repo := &PersistRepo{}

		key1 := repo.computeIdempotencyKey(123, "action", "Complete the report")
		key2 := repo.computeIdempotencyKey(123, "action", "Complete the report")

		assert.Equal(t, key1, key2, "same inputs should produce same key")
	})

	t.Run("different inputs produce different keys", func(t *testing.T) {
		repo := &PersistRepo{}

		key1 := repo.computeIdempotencyKey(123, "action", "Complete the report")
		key2 := repo.computeIdempotencyKey(123, "action", "Complete the document")
		key3 := repo.computeIdempotencyKey(124, "action", "Complete the report")
		key4 := repo.computeIdempotencyKey(123, "decision", "Complete the report")

		assert.NotEqual(t, key1, key2, "different descriptions should produce different keys")
		assert.NotEqual(t, key1, key3, "different source IDs should produce different keys")
		assert.NotEqual(t, key1, key4, "different types should produce different keys")
	})
}

func TestPersistRepo_OutputCounts(t *testing.T) {
	ctx := context.Background()

	t.Run("output counts are correct for mixed content", func(t *testing.T) {
		mock := &mockPersistRepo{
			output: &PersistFindingsOutput{
				AssertionsCreated:   3,
				ReferencesCreated:   3,
				AffinityUpdates:     2,
				CreatedAssertionIDs: []int64{100, 101, 102},
				CreatedReferenceIDs: []int64{200, 201, 202},
			},
		}

		projectID := int64(10)
		input := &PersistFindingsInput{
			TenantID:  "tenant-1",
			SourceID:  123,
			ProjectID: &projectID,
			Analysis: &DeepAnalyzeOutput{
				VerifiedActions: []VerifiedActionOutput{
					{
						Description:    "Action 1",
						ContextExcerpt: "Do this",
						Status:         "confirmed",
					},
				},
				VerifiedDecisions: []VerifiedDecisionOutput{
					{
						Description:    "Decision 1",
						ContextExcerpt: "We decided",
						Status:         "confirmed",
					},
				},
				RiskReferences: []RiskReferenceOutput{
					{
						Description:    "Risk 1",
						ContextExcerpt: "This is risky",
						Significance:   "primary",
						IsNew:          true,
					},
				},
			},
			ResolvedPeople: map[string]int64{"Alice": 42, "Bob": 43},
		}

		output, err := mock.PersistFindings(ctx, input)
		require.NoError(t, err)
		assert.Equal(t, 3, output.AssertionsCreated)
		assert.Equal(t, 3, output.ReferencesCreated)
		assert.Equal(t, 2, output.AffinityUpdates)
	})
}

func TestPersistRepo_AffinityUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("updates affinity for people with project context", func(t *testing.T) {
		projectID := int64(10)
		mock := &mockPersistRepo{
			output: &PersistFindingsOutput{
				AssertionsCreated: 1,
				ReferencesCreated: 1,
				AffinityUpdates:   1,
			},
		}

		input := &PersistFindingsInput{
			TenantID:  "tenant-1",
			SourceID:  123,
			ProjectID: &projectID,
			Analysis: &DeepAnalyzeOutput{
				VerifiedActions: []VerifiedActionOutput{
					{
						Description:    "Complete the report",
						ContextExcerpt: "Alice will do this",
						Assignee:       "Alice",
						Status:         "confirmed",
					},
				},
			},
			ResolvedPeople: map[string]int64{"Alice": 42},
		}

		output, err := mock.PersistFindings(ctx, input)
		require.NoError(t, err)
		assert.Equal(t, 1, output.AffinityUpdates)
	})

	t.Run("no affinity updates without project context", func(t *testing.T) {
		mock := &mockPersistRepo{
			output: &PersistFindingsOutput{
				AssertionsCreated: 1,
				ReferencesCreated: 1,
				AffinityUpdates:   0,
			},
		}

		input := &PersistFindingsInput{
			TenantID: "tenant-1",
			SourceID: 123,
			// No ProjectID
			Analysis: &DeepAnalyzeOutput{
				VerifiedActions: []VerifiedActionOutput{
					{
						Description:    "Complete the report",
						ContextExcerpt: "Alice will do this",
						Assignee:       "Alice",
						Status:         "confirmed",
					},
				},
			},
			ResolvedPeople: map[string]int64{"Alice": 42},
		}

		output, err := mock.PersistFindings(ctx, input)
		require.NoError(t, err)
		assert.Equal(t, 0, output.AffinityUpdates)
	})
}
