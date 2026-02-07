package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
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

func TestCollectAnalysisTexts(t *testing.T) {
	repo := &PersistRepo{
		logger: logging.NewLogger(&logging.Config{Level: logging.LevelError}),
	}

	t.Run("nil analysis returns nil", func(t *testing.T) {
		texts := repo.collectAnalysisTexts(nil)
		assert.Nil(t, texts)
	})

	t.Run("empty analysis returns nil", func(t *testing.T) {
		texts := repo.collectAnalysisTexts(&DeepAnalyzeOutput{})
		assert.Nil(t, texts)
	})

	t.Run("collects from all finding types", func(t *testing.T) {
		analysis := &DeepAnalyzeOutput{
			Summary: "Meeting summary about the KPI dashboard",
			VerifiedActions: []VerifiedActionOutput{
				{Description: "Deploy the API gateway", ContextExcerpt: "We need to deploy the API"},
			},
			VerifiedDecisions: []VerifiedDecisionOutput{
				{Description: "Use AWS for hosting", ContextExcerpt: "Decided on AWS"},
			},
			RiskReferences: []RiskReferenceOutput{
				{Description: "SLA breach risk", ContextExcerpt: "If SLA is missed"},
			},
			ImplicitActions: []ImplicitActionOutput{
				{Description: "Review the RFC", ContextExcerpt: "The RFC needs review"},
			},
			Insights: []string{"The team is aligned on OKR priorities"},
		}

		texts := repo.collectAnalysisTexts(analysis)

		// 2 from verified actions + 2 from decisions + 2 from risks + 2 from implicit + 1 summary + 1 insight = 10
		assert.Len(t, texts, 10)
		assert.Contains(t, texts, "Deploy the API gateway")
		assert.Contains(t, texts, "Use AWS for hosting")
		assert.Contains(t, texts, "SLA breach risk")
		assert.Contains(t, texts, "Review the RFC")
		assert.Contains(t, texts, "Meeting summary about the KPI dashboard")
		assert.Contains(t, texts, "The team is aligned on OKR priorities")
	})
}

func TestAcronymDetection(t *testing.T) {
	t.Run("common words are excluded", func(t *testing.T) {
		// These should all be excluded
		excluded := []string{"IT", "AM", "PM", "THE", "AND", "FOR", "NOT", "ALL", "CAN", "FYI", "ASAP", "TODO"}
		for _, word := range excluded {
			assert.True(t, commonAcronymExclusions[word], "%s should be excluded", word)
		}
	})

	t.Run("acronym pattern matches uppercase 2-5 char tokens", func(t *testing.T) {
		matches := acronymPattern.FindAllString("The API uses JWT for auth and the SLA is strict", -1)
		assert.Contains(t, matches, "API")
		assert.Contains(t, matches, "JWT")
		assert.Contains(t, matches, "SLA")
	})

	t.Run("acronym pattern matches pure uppercase", func(t *testing.T) {
		text := "Discussed CLIC, TRR, NLB, ECMP, PACE, PLT, PMO, FPM, DRI, MTC, ETG acronyms"
		matches := acronymPattern.FindAllString(text, -1)
		expected := []string{"CLIC", "TRR", "NLB", "ECMP", "PACE", "PLT", "PMO", "FPM", "DRI", "MTC", "ETG"}
		for _, exp := range expected {
			assert.Contains(t, matches, exp, "Should match pure uppercase acronym %s", exp)
		}
	})

	t.Run("acronym pattern matches mixed-case acronyms", func(t *testing.T) {
		text := "We use IaaS and the SteerCo approved PostgreSQL"
		matches := acronymPattern.FindAllString(text, -1)
		assert.Contains(t, matches, "IaaS", "Should match mixed-case IaaS")
		assert.Contains(t, matches, "SteerCo", "Should match mixed-case SteerCo")
		assert.Contains(t, matches, "PostgreSQL", "Should match mixed-case PostgreSQL")
	})

	t.Run("acronym pattern matches digit-containing acronyms", func(t *testing.T) {
		text := "FY26 budget and EC2 instance and R2D2 reference"
		matches := acronymPattern.FindAllString(text, -1)
		assert.Contains(t, matches, "FY26", "Should match fiscal year with digits")
		assert.Contains(t, matches, "EC2", "Should match cloud service with digit")
		assert.Contains(t, matches, "R2D2", "Should match acronym with multiple digits")
	})

	t.Run("acronym pattern accepts longer acronyms", func(t *testing.T) {
		text := "TOOLONG and VERYLONGACRONYM are matched now"
		matches := acronymPattern.FindAllString(text, -1)
		assert.Contains(t, matches, "TOOLONG", "Should match acronyms longer than 5 chars")
		assert.Contains(t, matches, "VERYLONGACRONYM", "Should match very long acronyms")
	})

	t.Run("skips when reviewQueue is nil", func(t *testing.T) {
		repo := &PersistRepo{
			logger: logging.NewLogger(&logging.Config{Level: logging.LevelError}),
			// reviewQueue is nil
		}
		output := &PersistFindingsOutput{}
		input := &PersistFindingsInput{
			SourceID: 1,
			Analysis: &DeepAnalyzeOutput{
				Summary: "We discussed the KPI dashboard and OKR targets",
			},
		}
		repo.detectAndCreateAcronymQuestions(context.Background(), input, output)
		assert.Equal(t, 0, output.ReviewItemsCreated)
	})

	t.Run("rejects hash strings as acronyms", func(t *testing.T) {
		// Hash strings should be filtered by isValidAcronym due to length cap
		hashStrings := []string{
			"MG87xCq4mhRpHGqmJp6G5m49xP8rmWp95wPcPpv1",
			"HJpvxXVhc73F55XXrfCjVHh2Xx3x7jXx3mHM6f71",
		}
		for _, hash := range hashStrings {
			text := "The hash " + hash + " was generated"
			matches := acronymPattern.FindAllString(text, -1)
			// Apply the same filtering logic as detectAndCreateAcronymQuestions
			for _, match := range matches {
				if !commonAcronymExclusions[match] && isValidAcronym(match) {
					assert.Fail(t, "Hash string should be rejected", "Hash fragment %s (length %d) passed isValidAcronym", match, len(match))
				}
			}
		}
	})

	t.Run("rejects proper nouns as acronyms", func(t *testing.T) {
		// Proper nouns should be filtered by isValidAcronym
		properNouns := []string{"TikTok", "LinkedIn", "GitHub", "YouTube", "MacBook", "iPhone"}
		for _, noun := range properNouns {
			text := "We discussed " + noun + " in the meeting"
			matches := acronymPattern.FindAllString(text, -1)
			for _, match := range matches {
				if match == noun && !commonAcronymExclusions[match] {
					// The match should be rejected by isValidAcronym
					assert.False(t, isValidAcronym(match), "Proper noun %s should be rejected by isValidAcronym", noun)
				}
			}
		}
	})

	t.Run("accepts real acronyms including longer ones", func(t *testing.T) {
		// These SHOULD match and pass isValidAcronym
		realAcronyms := []string{"CLIC", "TRR", "NLB", "ECMP", "PACE", "PLT", "MTC", "ETG"}
		for _, acronym := range realAcronyms {
			text := "The " + acronym + " system is ready"
			matches := acronymPattern.FindAllString(text, -1)
			assert.Contains(t, matches, acronym, "Real acronym %s should match pattern", acronym)
			// And it should pass validation
			assert.True(t, isValidAcronym(acronym), "Real acronym %s should pass isValidAcronym", acronym)
		}
	})

	t.Run("accepts mixed-case acronyms", func(t *testing.T) {
		// These SHOULD match and pass isValidAcronym
		mixedCaseAcronyms := []string{"IaaS", "SteerCo", "PMO", "DRI", "FY26"}
		for _, acronym := range mixedCaseAcronyms {
			text := "The " + acronym + " was discussed"
			matches := acronymPattern.FindAllString(text, -1)
			assert.Contains(t, matches, acronym, "Mixed-case acronym %s should match pattern", acronym)
			// And it should pass validation
			assert.True(t, isValidAcronym(acronym), "Mixed-case acronym %s should pass isValidAcronym", acronym)
		}
	})

	t.Run("rejects tokens longer than 10 characters", func(t *testing.T) {
		// Long tokens should be rejected by isValidAcronym
		longTokens := []string{"VERYLONGACRONYM", "TOOLONGTOBEREAL", "UNNECESSARILYLONGUPPER"}
		for _, token := range longTokens {
			// These should fail validation
			assert.False(t, isValidAcronym(token), "Long token %s (length %d) should be rejected", token, len(token))
		}
	})
}
