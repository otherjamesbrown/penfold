// Package activities provides tests for persist activities.
package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockPersistRepository is a mock implementation of the PersistRepository interface for testing.
type mockPersistRepository struct {
	persistFindingsFn func(ctx context.Context, input *PersistFindingsInput) (*PersistFindingsOutput, error)
}

func (m *mockPersistRepository) PersistFindings(ctx context.Context, input *PersistFindingsInput) (*PersistFindingsOutput, error) {
	if m.persistFindingsFn != nil {
		return m.persistFindingsFn(ctx, input)
	}
	return &PersistFindingsOutput{}, nil
}

func TestPersistFindings_Success(t *testing.T) {
	logger := logging.NewNopLogger()

	mockRepo := &mockPersistRepository{
		persistFindingsFn: func(ctx context.Context, input *PersistFindingsInput) (*PersistFindingsOutput, error) {
			// Verify input fields
			require.Equal(t, "test-tenant", input.TenantID)
			require.Equal(t, int64(123), input.SourceID)
			require.NotNil(t, input.Analysis)
			require.Len(t, input.Analysis.VerifiedActions, 1)
			require.Len(t, input.Analysis.VerifiedDecisions, 1)
			require.Len(t, input.Analysis.RiskReferences, 1)
			require.Len(t, input.ResolvedPeople, 2)

			// Return success output
			return &PersistFindingsOutput{
				AssertionsCreated:    5,
				AssertionsSuperseded: 1,
				ReferencesCreated:    5,
				ReviewItemsCreated:   2,
				AffinityUpdates:      2,
				CreatedAssertionIDs:  []int64{1, 2, 3, 4, 5},
				CreatedReferenceIDs:  []int64{10, 11, 12, 13, 14},
				CreatedReviewIDs:     []int64{20, 21},
			}, nil
		},
	}

	activities := NewPersistActivities(logger, mockRepo, nil)

	threadID := int64(456)
	projectID := int64(789)

	input := PersistFindingsActivityInput{
		TenantID:  "test-tenant",
		SourceID:  123,
		ThreadID:  &threadID,
		ProjectID: &projectID,
		Analysis: &DeepAnalyzeOutput{
			Summary: "Test analysis summary",
			VerifiedActions: []VerifiedActionOutput{
				{
					Description:    "Complete design review",
					Assignee:       "Alice Smith",
					Due:            "2024-03-10",
					Priority:       "high",
					ContextExcerpt: "Alice will review the design by Friday",
					Status:         "confirmed",
				},
			},
			VerifiedDecisions: []VerifiedDecisionOutput{
				{
					Description:    "Approved budget increase",
					ContextExcerpt: "Team agreed to increase budget by 10%",
					Status:         "confirmed",
				},
			},
			RiskReferences: []RiskReferenceOutput{
				{
					Description:    "Resource constraint may impact timeline",
					Significance:   "secondary",
					ContextExcerpt: "We might face resource issues in April",
					IsNew:          true,
				},
			},
		},
		ResolvedPeople: map[string]int64{
			"Alice Smith": 100,
			"Bob Jones":   101,
		},
	}

	output, err := activities.PersistFindings(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify output fields
	require.Equal(t, 5, output.AssertionsCreated)
	require.Equal(t, 1, output.AssertionsSuperseded)
	require.Equal(t, 5, output.ReferencesCreated)
	require.Equal(t, 2, output.ReviewItemsCreated)
	require.Equal(t, 2, output.AffinityUpdates)
}

func TestPersistFindings_NilAnalysis(t *testing.T) {
	logger := logging.NewNopLogger()
	mockRepo := &mockPersistRepository{}
	activities := NewPersistActivities(logger, mockRepo, nil)

	input := PersistFindingsActivityInput{
		TenantID: "test-tenant",
		SourceID: 123,
		Analysis: nil, // Nil analysis
	}

	output, err := activities.PersistFindings(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, output)
	require.Contains(t, err.Error(), "analysis is required")
}

func TestPersistFindings_InvalidSourceID(t *testing.T) {
	logger := logging.NewNopLogger()
	mockRepo := &mockPersistRepository{}
	activities := NewPersistActivities(logger, mockRepo, nil)

	input := PersistFindingsActivityInput{
		TenantID: "test-tenant",
		SourceID: 0, // Invalid source ID
		Analysis: &DeepAnalyzeOutput{
			Summary: "Test analysis",
		},
	}

	output, err := activities.PersistFindings(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, output)
	require.Contains(t, err.Error(), "source_id must be positive")
}

func TestPersistFindings_RepositoryError(t *testing.T) {
	logger := logging.NewNopLogger()

	expectedErr := errors.New("database connection failed")
	mockRepo := &mockPersistRepository{
		persistFindingsFn: func(ctx context.Context, input *PersistFindingsInput) (*PersistFindingsOutput, error) {
			return nil, expectedErr
		},
	}

	activities := NewPersistActivities(logger, mockRepo, nil)

	input := PersistFindingsActivityInput{
		TenantID: "test-tenant",
		SourceID: 123,
		Analysis: &DeepAnalyzeOutput{
			Summary: "Test analysis",
		},
	}

	output, err := activities.PersistFindings(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, output)
	// Error is now classified, so check for PipelineError
	require.Contains(t, err.Error(), "persist")
	require.Contains(t, err.Error(), expectedErr.Error())
}

func TestPersistFindings_EmptyAnalysis(t *testing.T) {
	logger := logging.NewNopLogger()

	mockRepo := &mockPersistRepository{
		persistFindingsFn: func(ctx context.Context, input *PersistFindingsInput) (*PersistFindingsOutput, error) {
			// Verify that empty analysis produces zero counts
			require.NotNil(t, input.Analysis)
			require.Empty(t, input.Analysis.VerifiedActions)
			require.Empty(t, input.Analysis.VerifiedDecisions)
			require.Empty(t, input.Analysis.RiskReferences)
			require.Empty(t, input.Analysis.ImplicitActions)

			// Return zero counts for empty analysis
			return &PersistFindingsOutput{
				AssertionsCreated:    0,
				AssertionsSuperseded: 0,
				ReferencesCreated:    0,
				ReviewItemsCreated:   0,
				AffinityUpdates:      0,
				CreatedAssertionIDs:  []int64{},
				CreatedReferenceIDs:  []int64{},
				CreatedReviewIDs:     []int64{},
			}, nil
		},
	}

	activities := NewPersistActivities(logger, mockRepo, nil)

	input := PersistFindingsActivityInput{
		TenantID: "test-tenant",
		SourceID: 123,
		Analysis: &DeepAnalyzeOutput{
			Summary: "Empty analysis with no findings",
		},
		ResolvedPeople: map[string]int64{},
	}

	output, err := activities.PersistFindings(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify zero counts
	require.Equal(t, 0, output.AssertionsCreated)
	require.Equal(t, 0, output.AssertionsSuperseded)
	require.Equal(t, 0, output.ReferencesCreated)
	require.Equal(t, 0, output.ReviewItemsCreated)
	require.Equal(t, 0, output.AffinityUpdates)
}
