//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlossaryRepository_Create(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary_terms")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	tests := []struct {
		name    string
		input   glossary.TermInput
		wantErr bool
	}{
		{
			name: "create simple term",
			input: glossary.TermInput{
				Term:       "MVP",
				Expansion:  strPtr("Minimum Viable Product"),
				Definition: strPtr("A version of a product with just enough features to satisfy early customers"),
			},
			wantErr: false,
		},
		{
			name: "create term with aliases",
			input: glossary.TermInput{
				Term:      "API",
				Expansion: strPtr("Application Programming Interface"),
				Aliases:   []string{"apis", "interface"},
			},
			wantErr: false,
		},
		{
			name: "create term with context",
			input: glossary.TermInput{
				Term:      "P0",
				Expansion: strPtr("Priority Zero"),
				Context:   strPtr("incident"),
				Aliases:   []string{"Sev0", "Critical"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term, err := repo.Create(ctx, tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotZero(t, term.ID)
			assert.Equal(t, tt.input.Term, term.Term)
			if tt.input.Expansion != nil {
				assert.Equal(t, *tt.input.Expansion, *term.Expansion)
			}
		})
	}
}

func TestGlossaryRepository_GetByTerm(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary_terms")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create a term first
	input := glossary.TermInput{
		Term:       "TER",
		Expansion:  strPtr("Technical Execution Review"),
		Definition: strPtr("Weekly engineering review meeting"),
		Context:    strPtr("meeting"),
	}
	created, err := repo.Create(ctx, input)
	require.NoError(t, err)

	tests := []struct {
		name     string
		termStr  string
		wantErr  bool
		wantNil  bool
	}{
		{
			name:    "find existing term",
			termStr: "TER",
			wantErr: false,
			wantNil: false,
		},
		{
			name:    "find with different case",
			termStr: "ter",
			wantErr: false,
			wantNil: false,
		},
		{
			name:    "term not found",
			termStr: "NONEXISTENT",
			wantErr: false,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term, err := repo.GetByTerm(ctx, tt.termStr)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, term)
			} else {
				assert.NotNil(t, term)
				assert.Equal(t, created.ID, term.ID)
			}
		})
	}
}

func TestGlossaryRepository_List(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary_terms")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create multiple terms
	terms := []glossary.TermInput{
		{Term: "MVP", Expansion: strPtr("Minimum Viable Product")},
		{Term: "API", Expansion: strPtr("Application Programming Interface")},
		{Term: "P0", Expansion: strPtr("Priority Zero"), Context: strPtr("incident")},
		{Term: "P1", Expansion: strPtr("Priority One"), Context: strPtr("incident")},
		{Term: "TER", Expansion: strPtr("Technical Execution Review"), Context: strPtr("meeting")},
	}

	for _, input := range terms {
		_, err := repo.Create(ctx, input)
		require.NoError(t, err)
	}

	tests := []struct {
		name      string
		filter    glossary.TermFilter
		wantCount int
	}{
		{
			name:      "list all terms",
			filter:    glossary.TermFilter{},
			wantCount: 5,
		},
		{
			name:      "filter by context - incident",
			filter:    glossary.TermFilter{Context: strPtr("incident")},
			wantCount: 2,
		},
		{
			name:      "filter by context - meeting",
			filter:    glossary.TermFilter{Context: strPtr("meeting")},
			wantCount: 1,
		},
		{
			name:      "limit results",
			filter:    glossary.TermFilter{Limit: 2},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := repo.List(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.wantCount)
		})
	}
}

func TestGlossaryRepository_Update(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary_terms")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create a term
	created, err := repo.Create(ctx, glossary.TermInput{
		Term:       "MVP",
		Expansion:  strPtr("Minimum Viable Product"),
		Definition: strPtr("Original definition"),
	})
	require.NoError(t, err)

	// Update the term
	updated, err := repo.Update(ctx, created.ID, glossary.TermInput{
		Term:       "MVP",
		Expansion:  strPtr("Minimum Viable Product"),
		Definition: strPtr("Updated definition with more detail"),
		Aliases:    []string{"minimum viable", "mvp"},
	})
	require.NoError(t, err)

	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, "Updated definition with more detail", *updated.Definition)
	assert.Contains(t, updated.Aliases, "minimum viable")
}

func TestGlossaryRepository_Delete(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary_terms")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create a term
	created, err := repo.Create(ctx, glossary.TermInput{
		Term:      "TEMP",
		Expansion: strPtr("Temporary Term"),
	})
	require.NoError(t, err)

	// Delete the term
	err = repo.Delete(ctx, created.ID)
	require.NoError(t, err)

	// Verify it's gone
	term, err := repo.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Nil(t, term)
}

func TestGlossaryRepository_ExpandQuery(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary_terms")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create terms with aliases
	_, err := repo.Create(ctx, glossary.TermInput{
		Term:      "MVP",
		Expansion: strPtr("Minimum Viable Product"),
		Aliases:   []string{"minimum viable", "early release"},
	})
	require.NoError(t, err)

	_, err = repo.Create(ctx, glossary.TermInput{
		Term:      "API",
		Expansion: strPtr("Application Programming Interface"),
		Aliases:   []string{"interface", "endpoint"},
	})
	require.NoError(t, err)

	tests := []struct {
		name           string
		query          string
		wantExpanded   bool
		wantTermsFound int
	}{
		{
			name:           "expand query with MVP",
			query:          "What is the MVP for our API?",
			wantExpanded:   true,
			wantTermsFound: 2,
		},
		{
			name:           "no expansion needed",
			query:          "Hello world",
			wantExpanded:   false,
			wantTermsFound: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expansion, err := repo.ExpandQuery(ctx, tt.query)
			require.NoError(t, err)

			if tt.wantExpanded {
				assert.NotEmpty(t, expansion.ExpandedQuery)
				assert.Len(t, expansion.TermsFound, tt.wantTermsFound)
			} else {
				assert.Empty(t, expansion.TermsFound)
			}
		})
	}
}

// Helper function
func strPtr(s string) *string {
	return &s
}
