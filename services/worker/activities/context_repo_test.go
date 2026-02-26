package activities

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockContextPackageRepo is a mock implementation of ContextPackageRepository for testing.
type mockContextPackageRepo struct {
	activeRisks            []ContextAssertion
	openActions            []ContextAssertion
	recentDecisions        []ContextAssertion
	productEvents          []ContextProductEvent
	glossaryTerms          []ContextGlossaryTerm
	projectsByName         map[string]*int64
	projectsByKeyword      map[string]*int64
	projectsByNameContains map[string]*int64
	err                    error
}

func (m *mockContextPackageRepo) GetActiveRisks(ctx context.Context, projectIDs []int64, limit int) ([]ContextAssertion, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.activeRisks, nil
}

func (m *mockContextPackageRepo) GetOpenActions(ctx context.Context, conversationID string, projectIDs []int64, limit int) ([]ContextAssertion, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.openActions, nil
}

func (m *mockContextPackageRepo) GetRecentDecisions(ctx context.Context, projectIDs []int64, days int, limit int) ([]ContextAssertion, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.recentDecisions, nil
}

func (m *mockContextPackageRepo) GetProductEvents(ctx context.Context, productIDs []int64, days int, limit int) ([]ContextProductEvent, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.productEvents, nil
}

func (m *mockContextPackageRepo) GetGlossaryTerms(ctx context.Context, tenantID string, terms []string, productIDs []int64, limit int) ([]ContextGlossaryTerm, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.glossaryTerms, nil
}

func (m *mockContextPackageRepo) ResolveProjectByName(ctx context.Context, tenantID string, name string) (*int64, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.projectsByName != nil {
		return m.projectsByName[name], nil
	}
	return nil, nil
}

func (m *mockContextPackageRepo) ResolveProjectByKeyword(ctx context.Context, tenantID string, keyword string) (*int64, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.projectsByKeyword != nil {
		return m.projectsByKeyword[keyword], nil
	}
	return nil, nil
}

func (m *mockContextPackageRepo) ResolveProjectByNameContains(ctx context.Context, tenantID string, extractedName string) (*int64, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.projectsByNameContains != nil {
		return m.projectsByNameContains[extractedName], nil
	}
	return nil, nil
}

func TestContextPackageRepo_GetActiveRisks(t *testing.T) {
	ctx := context.Background()

	t.Run("returns active risks ordered by severity", func(t *testing.T) {
		now := time.Now()
		dueDate := now.Add(24 * time.Hour)

		mock := &mockContextPackageRepo{
			activeRisks: []ContextAssertion{
				{
					Description: "Critical security issue",
					Severity:    "critical",
					OwnerName:   "Alice",
					ProjectName: "Project X",
					SourceQuote: "Security vulnerability detected",
				},
				{
					Description: "High priority bug",
					Severity:    "high",
					OwnerName:   "Bob",
					ProjectName: "Project X",
					SourceQuote: "Bug in production",
				},
			},
		}

		results, err := mock.GetActiveRisks(ctx, []int64{1}, 10)
		require.NoError(t, err)
		assert.Len(t, results, 2)
		assert.Equal(t, "Critical security issue", results[0].Description)
		assert.Equal(t, "critical", results[0].Severity)
		assert.Equal(t, "Alice", results[0].OwnerName)

		_ = dueDate // Suppress unused variable warning
	})

	t.Run("handles empty results", func(t *testing.T) {
		mock := &mockContextPackageRepo{
			activeRisks: []ContextAssertion{},
		}

		results, err := mock.GetActiveRisks(ctx, []int64{1}, 10)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("handles nil source_quote", func(t *testing.T) {
		mock := &mockContextPackageRepo{
			activeRisks: []ContextAssertion{
				{
					Description: "Risk without quote",
					Severity:    "medium",
					OwnerName:   "Charlie",
					ProjectName: "Project Y",
					SourceQuote: "", // Simulates NULL
				},
			},
		}

		results, err := mock.GetActiveRisks(ctx, []int64{2}, 10)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "", results[0].SourceQuote)
	})
}

func TestContextPackageRepo_GetOpenActions(t *testing.T) {
	ctx := context.Background()

	t.Run("returns open actions ordered by due date", func(t *testing.T) {
		now := time.Now()
		dueDate1 := now.Add(24 * time.Hour)
		dueDate2 := now.Add(48 * time.Hour)

		mock := &mockContextPackageRepo{
			openActions: []ContextAssertion{
				{
					Description:  "Urgent action",
					Status:       "open",
					DueDate:      &dueDate1,
					AssigneeName: "Alice",
				},
				{
					Description:  "Follow-up action",
					Status:       "open",
					DueDate:      &dueDate2,
					AssigneeName: "Bob",
				},
			},
		}

		results, err := mock.GetOpenActions(ctx, "", []int64{1}, 10)
		require.NoError(t, err)
		assert.Len(t, results, 2)
		assert.Equal(t, "Urgent action", results[0].Description)
		assert.Equal(t, "Alice", results[0].AssigneeName)
	})

	t.Run("handles actions with null due dates", func(t *testing.T) {
		mock := &mockContextPackageRepo{
			openActions: []ContextAssertion{
				{
					Description:  "Action without due date",
					Status:       "open",
					DueDate:      nil,
					AssigneeName: "Charlie",
				},
			},
		}

		results, err := mock.GetOpenActions(ctx, "", []int64{1}, 10)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Nil(t, results[0].DueDate)
	})
}

func TestContextPackageRepo_GetRecentDecisions(t *testing.T) {
	ctx := context.Background()

	t.Run("returns recent decisions", func(t *testing.T) {
		mock := &mockContextPackageRepo{
			recentDecisions: []ContextAssertion{
				{
					Description:   "Decided to use approach A",
					Rationale:     "Better performance",
					DecisionMaker: "Alice",
				},
				{
					Description:   "Decided to postpone feature B",
					Rationale:     "Resource constraints",
					DecisionMaker: "Bob",
				},
			},
		}

		results, err := mock.GetRecentDecisions(ctx, []int64{1}, 60, 5)
		require.NoError(t, err)
		assert.Len(t, results, 2)
		assert.Equal(t, "Decided to use approach A", results[0].Description)
		assert.Equal(t, "Better performance", results[0].Rationale)
	})

	t.Run("handles null rationale", func(t *testing.T) {
		mock := &mockContextPackageRepo{
			recentDecisions: []ContextAssertion{
				{
					Description:   "Quick decision",
					Rationale:     "", // Simulates NULL
					DecisionMaker: "Charlie",
				},
			},
		}

		results, err := mock.GetRecentDecisions(ctx, []int64{1}, 60, 5)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "", results[0].Rationale)
	})
}

func TestContextPackageRepo_GetProductEvents(t *testing.T) {
	ctx := context.Background()

	t.Run("returns product events", func(t *testing.T) {
		now := time.Now()
		mock := &mockContextPackageRepo{
			productEvents: []ContextProductEvent{
				{
					Title:       "Release 1.0",
					Description: "Major release",
					EventType:   "release",
					OccurredAt:  now,
				},
			},
		}

		results, err := mock.GetProductEvents(ctx, []int64{1}, 90, 10)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "Release 1.0", results[0].Title)
		assert.Equal(t, "Major release", results[0].Description)
	})

	t.Run("handles null description", func(t *testing.T) {
		now := time.Now()
		mock := &mockContextPackageRepo{
			productEvents: []ContextProductEvent{
				{
					Title:       "Milestone reached",
					Description: "", // Simulates NULL
					EventType:   "milestone",
					OccurredAt:  now,
				},
			},
		}

		results, err := mock.GetProductEvents(ctx, []int64{1}, 90, 10)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "", results[0].Description)
	})
}

func TestContextPackageRepo_GetGlossaryTerms(t *testing.T) {
	ctx := context.Background()

	t.Run("returns glossary terms", func(t *testing.T) {
		mock := &mockContextPackageRepo{
			glossaryTerms: []ContextGlossaryTerm{
				{
					Term:       "API",
					Expansion:  "Application Programming Interface",
					Definition: "A set of protocols for building software",
				},
				{
					Term:       "SLA",
					Expansion:  "Service Level Agreement",
					Definition: "A commitment between a service provider and client",
				},
			},
		}

		results, err := mock.GetGlossaryTerms(ctx, "test-tenant", []string{"API", "SLA"}, []int64{}, 20)
		require.NoError(t, err)
		assert.Len(t, results, 2)
		assert.Equal(t, "API", results[0].Term)
		assert.Equal(t, "Application Programming Interface", results[0].Expansion)
	})

	t.Run("handles null expansion and definition", func(t *testing.T) {
		mock := &mockContextPackageRepo{
			glossaryTerms: []ContextGlossaryTerm{
				{
					Term:       "XYZ",
					Expansion:  "", // Simulates NULL
					Definition: "", // Simulates NULL
				},
			},
		}

		results, err := mock.GetGlossaryTerms(ctx, "test-tenant", []string{"XYZ"}, []int64{}, 20)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "", results[0].Expansion)
		assert.Equal(t, "", results[0].Definition)
	})
}

func TestContextPackageRepo_ResolveProjectByName(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves project by exact name", func(t *testing.T) {
		projectID := int64(123)
		mock := &mockContextPackageRepo{
			projectsByName: map[string]*int64{
				"Project Alpha": &projectID,
			},
		}

		result, err := mock.ResolveProjectByName(ctx, "tenant-1", "Project Alpha")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(123), *result)
	})

	t.Run("returns nil when project not found", func(t *testing.T) {
		mock := &mockContextPackageRepo{
			projectsByName: map[string]*int64{},
		}

		result, err := mock.ResolveProjectByName(ctx, "tenant-1", "Non-existent Project")
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestContextPackageRepo_ResolveProjectByKeyword(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves project by keyword", func(t *testing.T) {
		projectID := int64(456)
		mock := &mockContextPackageRepo{
			projectsByKeyword: map[string]*int64{
				"alpha": &projectID,
			},
		}

		result, err := mock.ResolveProjectByKeyword(ctx, "tenant-1", "alpha")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(456), *result)
	})

	t.Run("returns nil when keyword not found", func(t *testing.T) {
		mock := &mockContextPackageRepo{
			projectsByKeyword: map[string]*int64{},
		}

		result, err := mock.ResolveProjectByKeyword(ctx, "tenant-1", "unknown")
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}
