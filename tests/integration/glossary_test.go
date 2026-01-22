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
	db.TruncateTables(t, "glossary")

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
				Expansion:  "Minimum Viable Product",
				Definition: "A version of a product with just enough features to satisfy early customers",
			},
			wantErr: false,
		},
		{
			name: "create term with aliases",
			input: glossary.TermInput{
				Term:      "API",
				Expansion: "Application Programming Interface",
				Aliases:   []string{"apis", "interface"},
			},
			wantErr: false,
		},
		{
			name: "create term with context",
			input: glossary.TermInput{
				Term:      "P0",
				Expansion: "Priority Zero",
				Context:   []string{"incident"},
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
			if tt.input.Expansion != "" {
				assert.Equal(t, tt.input.Expansion, term.Expansion)
			}
		})
	}
}

func TestGlossaryRepository_GetByTerm(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create a term first
	input := glossary.TermInput{
		Term:       "TER",
		Expansion:  "Technical Execution Review",
		Definition: "Weekly engineering review meeting",
		Context:    []string{"meeting"},
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
	db.TruncateTables(t, "glossary")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create multiple terms
	terms := []glossary.TermInput{
		{Term: "MVP", Expansion: "Minimum Viable Product"},
		{Term: "API", Expansion: "Application Programming Interface"},
		{Term: "P0", Expansion: "Priority Zero", Context: []string{"incident"}},
		{Term: "P1", Expansion: "Priority One", Context: []string{"incident"}},
		{Term: "TER", Expansion: "Technical Execution Review", Context: []string{"meeting"}},
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
			filter:    glossary.TermFilter{Context: []string{"incident"}},
			wantCount: 2,
		},
		{
			name:      "filter by context - meeting",
			filter:    glossary.TermFilter{Context: []string{"meeting"}},
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
	db.TruncateTables(t, "glossary")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create a term
	created, err := repo.Create(ctx, glossary.TermInput{
		Term:       "MVP",
		Expansion:  "Minimum Viable Product",
		Definition: "Original definition",
	})
	require.NoError(t, err)

	// Update the term
	updated, err := repo.Update(ctx, created.ID, glossary.TermInput{
		Term:       "MVP",
		Expansion:  "Minimum Viable Product",
		Definition: "Updated definition with more detail",
		Aliases:    []string{"minimum viable", "mvp"},
	})
	require.NoError(t, err)

	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, "Updated definition with more detail", updated.Definition)
	assert.Contains(t, updated.Aliases, "minimum viable")
}

func TestGlossaryRepository_Delete(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create a term
	created, err := repo.Create(ctx, glossary.TermInput{
		Term:      "TEMP",
		Expansion: "Temporary Term",
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
	db.TruncateTables(t, "glossary")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create terms with aliases
	_, err := repo.Create(ctx, glossary.TermInput{
		Term:      "MVP",
		Expansion: "Minimum Viable Product",
		Aliases:   []string{"minimum viable", "early release"},
	})
	require.NoError(t, err)

	_, err = repo.Create(ctx, glossary.TermInput{
		Term:      "API",
		Expansion: "Application Programming Interface",
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
				assert.Len(t, expansion.ExpandedTerms, tt.wantTermsFound)
			} else {
				assert.Empty(t, expansion.ExpandedTerms)
			}
		})
	}
}

func TestGlossaryRepository_DeleteByTerm(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create a term
	_, err := repo.Create(ctx, glossary.TermInput{
		Term:      "DELETEME",
		Expansion: "Term To Delete",
	})
	require.NoError(t, err)

	// Verify it exists
	term, err := repo.GetByTerm(ctx, "DELETEME")
	require.NoError(t, err)
	assert.NotNil(t, term)

	// Delete by term string
	err = repo.DeleteByTerm(ctx, "DELETEME")
	require.NoError(t, err)

	// Verify it's gone
	term, err = repo.GetByTerm(ctx, "DELETEME")
	require.NoError(t, err)
	assert.Nil(t, term)

	// Deleting non-existent term should error
	err = repo.DeleteByTerm(ctx, "NONEXISTENT")
	assert.Error(t, err)
}

func TestGlossaryRepository_LookupTerm(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create a term with aliases
	_, err := repo.Create(ctx, glossary.TermInput{
		Term:      "MVP",
		Expansion: "Minimum Viable Product",
		Aliases:   []string{"minimum viable", "early release"},
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		lookup  string
		wantNil bool
	}{
		{
			name:    "find by exact term",
			lookup:  "MVP",
			wantNil: false,
		},
		{
			name:    "find by alias",
			lookup:  "minimum viable",
			wantNil: false,
		},
		{
			name:    "term not found",
			lookup:  "NONEXISTENT",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term, err := repo.LookupTerm(ctx, tt.lookup)
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, term)
			} else {
				assert.NotNil(t, term)
				assert.Equal(t, "MVP", term.Term)
			}
		})
	}
}

func TestGlossaryRepository_LinkEntity(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create a term
	created, err := repo.Create(ctx, glossary.TermInput{
		Term:      "Widget",
		Expansion: "Widget Product",
	})
	require.NoError(t, err)
	assert.Nil(t, created.LinkedEntityType)
	assert.Nil(t, created.LinkedEntityID)

	// Link to a product
	linked, err := repo.LinkEntity(ctx, created.ID, "product", 42)
	require.NoError(t, err)
	assert.NotNil(t, linked.LinkedEntityType)
	assert.Equal(t, "product", *linked.LinkedEntityType)
	assert.NotNil(t, linked.LinkedEntityID)
	assert.Equal(t, int64(42), *linked.LinkedEntityID)

	// Try invalid entity type
	_, err = repo.LinkEntity(ctx, created.ID, "invalid", 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid entity type")

	// Try non-existent term
	_, err = repo.LinkEntity(ctx, 999999, "product", 1)
	assert.Error(t, err)
}

func TestGlossaryRepository_UnlinkEntity(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create and link a term
	created, err := repo.Create(ctx, glossary.TermInput{
		Term:      "Project Alpha",
		Expansion: "Main Project",
	})
	require.NoError(t, err)

	linked, err := repo.LinkEntity(ctx, created.ID, "project", 100)
	require.NoError(t, err)
	assert.NotNil(t, linked.LinkedEntityID)

	// Unlink the entity
	unlinked, err := repo.UnlinkEntity(ctx, created.ID)
	require.NoError(t, err)
	assert.Nil(t, unlinked.LinkedEntityType)
	assert.Nil(t, unlinked.LinkedEntityID)

	// Try unlinking non-existent term
	_, err = repo.UnlinkEntity(ctx, 999999)
	assert.Error(t, err)
}

func TestGlossaryRepository_ListLinked(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "glossary")

	repo := glossary.NewRepository(db.Pool)
	ctx := context.Background()

	// Create terms with various link states
	term1, err := repo.Create(ctx, glossary.TermInput{Term: "Product1", Expansion: "Prod One"})
	require.NoError(t, err)
	_, err = repo.LinkEntity(ctx, term1.ID, "product", 1)
	require.NoError(t, err)

	term2, err := repo.Create(ctx, glossary.TermInput{Term: "Product2", Expansion: "Prod Two"})
	require.NoError(t, err)
	_, err = repo.LinkEntity(ctx, term2.ID, "product", 2)
	require.NoError(t, err)

	term3, err := repo.Create(ctx, glossary.TermInput{Term: "Project1", Expansion: "Proj One"})
	require.NoError(t, err)
	_, err = repo.LinkEntity(ctx, term3.ID, "project", 1)
	require.NoError(t, err)

	// Create unlinked term
	_, err = repo.Create(ctx, glossary.TermInput{Term: "Unlinked", Expansion: "No Link"})
	require.NoError(t, err)

	tests := []struct {
		name      string
		filter    glossary.LinkedTermFilter
		wantCount int
	}{
		{
			name:      "list all linked",
			filter:    glossary.LinkedTermFilter{},
			wantCount: 3,
		},
		{
			name:      "filter by product type",
			filter:    glossary.LinkedTermFilter{EntityType: "product"},
			wantCount: 2,
		},
		{
			name:      "filter by project type",
			filter:    glossary.LinkedTermFilter{EntityType: "project"},
			wantCount: 1,
		},
		{
			name:      "filter by specific entity",
			filter:    glossary.LinkedTermFilter{EntityType: "product", EntityID: 1},
			wantCount: 1,
		},
		{
			name:      "limit results",
			filter:    glossary.LinkedTermFilter{Limit: 2},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := repo.ListLinked(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.wantCount)
			assert.GreaterOrEqual(t, int(total), tt.wantCount)
		})
	}
}

