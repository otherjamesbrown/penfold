package searchservice

import (
	"context"
	"strings"
	"testing"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEmbedder implements EmbeddingGenerator for testing.
type mockEmbedder struct {
	vector []float32
	err    error
}

func (m *mockEmbedder) GenerateEmbedding(_ context.Context, _ *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &aiv1.EmbeddingResponse{
		Vector:     m.vector,
		Dimensions: int32(len(m.vector)),
		ModelUsed:  "test-model",
	}, nil
}

func TestNormalizeAndBlend(t *testing.T) {
	tests := []struct {
		name         string
		textScores   []float32
		vectorScores []*float32
		maxText      float32
		tw, vw       float32
		wantScores   []float32
	}{
		{
			name:         "equal weights, both scores present",
			textScores:   []float32{0.1, 0.05},
			vectorScores: []*float32{pf32(0.8), pf32(0.6)},
			maxText:      0.1,
			tw:           0.5,
			vw:           0.5,
			wantScores:   []float32{0.9, 0.55},
		},
		{
			name:         "text only (no vector scores)",
			textScores:   []float32{0.1, 0.05},
			vectorScores: []*float32{nil, nil},
			maxText:      0.1,
			tw:           0.5,
			vw:           0.5,
			wantScores:   []float32{0.5, 0.25},
		},
		{
			name:         "vector only weight",
			textScores:   []float32{0.1, 0.05},
			vectorScores: []*float32{pf32(0.9), pf32(0.3)},
			maxText:      0.1,
			tw:           0.0,
			vw:           1.0,
			wantScores:   []float32{0.9, 0.3},
		},
		{
			name:         "text only weight",
			textScores:   []float32{0.1, 0.05},
			vectorScores: []*float32{pf32(0.9), pf32(0.3)},
			maxText:      0.1,
			tw:           1.0,
			vw:           0.0,
			wantScores:   []float32{1.0, 0.5},
		},
		{
			name:         "uniform text scores get differentiated by vector",
			textScores:   []float32{0.097, 0.097, 0.097},
			vectorScores: []*float32{pf32(0.95), pf32(0.5), pf32(0.2)},
			maxText:      0.097,
			tw:           0.5,
			vw:           0.5,
			wantScores:   []float32{0.975, 0.75, 0.6},
		},
		{
			name:         "max text score of zero",
			textScores:   []float32{0, 0},
			vectorScores: []*float32{pf32(0.8), pf32(0.4)},
			maxText:      0,
			tw:           0.5,
			vw:           0.5,
			wantScores:   []float32{0.4, 0.2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := make([]*searchv1.SearchResult, len(tt.textScores))
			for i := range results {
				ts := tt.textScores[i]
				results[i] = &searchv1.SearchResult{
					TextScore:   &ts,
					VectorScore: tt.vectorScores[i],
				}
			}

			normalizeAndBlend(results, tt.maxText, tt.tw, tt.vw)

			for i, r := range results {
				assert.InDelta(t, tt.wantScores[i], r.Score, 0.01,
					"result[%d]: expected %.3f, got %.3f", i, tt.wantScores[i], r.Score)
			}
		})
	}
}

func TestNormalizeAndBlend_ScoresClamped(t *testing.T) {
	ts := float32(0.5)
	vs := float32(0.9)
	results := []*searchv1.SearchResult{
		{TextScore: &ts, VectorScore: &vs},
	}

	// Weights that sum to >1 should still clamp at 1.0
	normalizeAndBlend(results, 0.5, 0.8, 0.8)
	assert.LessOrEqual(t, results[0].Score, float32(1.0))
}

func TestService_SearchRequiresDB_BeforeQuery(t *testing.T) {
	// DB nil check happens first, before query validation
	svc := NewService(nil, nil)

	_, err := svc.Search(context.Background(), &searchv1.SearchRequest{
		Query:    "",
		TenantId: "test-tenant",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database not configured")
}

func TestService_SearchRequiresDB(t *testing.T) {
	svc := NewService(nil, nil)

	_, err := svc.Search(context.Background(), &searchv1.SearchRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database not configured")
}

func TestService_SetEmbedder(t *testing.T) {
	svc := NewService(nil, nil)
	assert.Nil(t, svc.embedder)

	mock := &mockEmbedder{vector: []float32{0.1, 0.2}}
	svc.SetEmbedder(mock)
	assert.NotNil(t, svc.embedder)
}

func TestService_EmbedQueryReturnsEmptyWithoutEmbedder(t *testing.T) {
	svc := NewService(nil, nil)
	vec := svc.embedQuery(context.Background(), "test", "tenant")
	assert.Empty(t, vec)
}

func TestService_EmbedQueryReturnsPgvectorString(t *testing.T) {
	mock := &mockEmbedder{vector: []float32{0.1, 0.2, 0.3}}
	svc := NewService(nil, nil)
	svc.SetEmbedder(mock)

	vec := svc.embedQuery(context.Background(), "test", "tenant")
	assert.Equal(t, "[0.1,0.2,0.3]", vec)
}

func TestService_EmbedQueryReturnsEmptyOnError(t *testing.T) {
	mock := &mockEmbedder{err: assert.AnError}
	svc := NewService(nil, nil)
	svc.SetEmbedder(mock)

	vec := svc.embedQuery(context.Background(), "test", "tenant")
	assert.Empty(t, vec)
}

func TestVecToString(t *testing.T) {
	tests := []struct {
		name string
		vec  []float32
		want string
	}{
		{"empty", nil, "[]"},
		{"single", []float32{0.5}, "[0.5]"},
		{"multiple", []float32{0.1, 0.2, 0.3}, "[0.1,0.2,0.3]"},
		{"negative", []float32{-0.5, 0.5}, "[-0.5,0.5]"},
		{"precise", []float32{0.123456}, "[0.123456]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vecToString(tt.vec)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDerefString(t *testing.T) {
	s := "hello"
	assert.Equal(t, "hello", derefString(&s, "default"))
	assert.Equal(t, "default", derefString(nil, "default"))
	empty := ""
	assert.Equal(t, "default", derefString(&empty, "default"))
}

func TestHighlightSnippet(t *testing.T) {
	result := highlightSnippet("The project deadline is next week", "project deadline")
	assert.Contains(t, result, "<em>project</em>")
	assert.Contains(t, result, "<em>deadline</em>")
}

func TestHighlightSnippet_ShortWordsIgnored(t *testing.T) {
	result := highlightSnippet("The cat is on the mat", "is on")
	// Words <= 2 chars should not be highlighted
	assert.NotContains(t, result, "<em>is</em>")
	assert.NotContains(t, result, "<em>on</em>")
}

// pf32 returns a pointer to a float32 value.
func pf32(v float32) *float32 {
	return &v
}

// mockExpander implements QueryExpander for testing.
type mockExpander struct {
	expansion *glossary.QueryExpansion
	err       error
}

func (m *mockExpander) ExpandQuery(_ context.Context, _ string, query string) (*glossary.QueryExpansion, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.expansion != nil {
		return m.expansion, nil
	}
	return &glossary.QueryExpansion{
		OriginalQuery:  query,
		ExpandedTerms:  []glossary.ExpansionResult{},
		ExpandedQuery:  query,
		AliasedQueries: []string{},
	}, nil
}

// ----------------------------------------------------------------------------
// buildAliasQuery tests
// ----------------------------------------------------------------------------

func TestBuildAliasQuery_BasicExpansion(t *testing.T) {
	expansion := &glossary.QueryExpansion{
		OriginalQuery: "36QDD",
		ExpandedTerms: []glossary.ExpansionResult{
			{
				OriginalTerm: "36QDD",
				TermName:     "Juniper Border Routers",
				Expansion:    "QFX5130-36QDD switches used as border routers in the network core",
				Aliases:      []string{"border routers", "Juniper routers", "QFX5130-36QDD"},
			},
		},
	}

	result := buildAliasQuery("36QDD", expansion)

	// Should contain all unique terms, double-quoted
	assert.Contains(t, result, `"36QDD"`)
	assert.Contains(t, result, `"Juniper Border Routers"`)
	assert.Contains(t, result, `"border routers"`)
	assert.Contains(t, result, `"Juniper routers"`)
	assert.Contains(t, result, `"QFX5130-36QDD"`)
	// All joined with OR
	assert.Contains(t, result, " OR ")
}

func TestBuildAliasQuery_NoExpansion(t *testing.T) {
	// With empty ExpandedTerms, buildAliasQuery still quotes the original query.
	// In practice, Search() only calls buildAliasQuery when ExpandedTerms is non-empty.
	expansion := &glossary.QueryExpansion{
		OriginalQuery: "random words",
		ExpandedTerms: []glossary.ExpansionResult{},
	}

	result := buildAliasQuery("random words", expansion)
	// Only the original query, quoted
	assert.Equal(t, `"random words"`, result)
}

func TestBuildAliasQuery_DeduplicatesTerms(t *testing.T) {
	// When the original query matches an alias exactly, no duplicate
	expansion := &glossary.QueryExpansion{
		OriginalQuery: "border routers",
		ExpandedTerms: []glossary.ExpansionResult{
			{
				OriginalTerm: "border routers",
				TermName:     "Juniper Border Routers",
				Expansion:    "QFX5130-36QDD switches",
				Aliases:      []string{"border routers", "Juniper routers"},
			},
		},
	}

	result := buildAliasQuery("border routers", expansion)

	// Count occurrences of "border routers" — should only appear once
	count := strings.Count(result, `"border routers"`)
	assert.Equal(t, 1, count, "border routers should not be duplicated")
}

func TestBuildAliasQuery_DeduplicatesOriginalTermAndTermName(t *testing.T) {
	// When query == TermName, no duplicate
	expansion := &glossary.QueryExpansion{
		OriginalQuery: "Juniper Border Routers",
		ExpandedTerms: []glossary.ExpansionResult{
			{
				OriginalTerm: "Juniper Border Routers",
				TermName:     "Juniper Border Routers",
				Aliases:      []string{"36QDD"},
			},
		},
	}

	result := buildAliasQuery("Juniper Border Routers", expansion)
	count := strings.Count(result, `"Juniper Border Routers"`)
	assert.Equal(t, 1, count, "canonical term should not be duplicated")
}

func TestBuildAliasQuery_StripsEmbeddedDoubleQuotes(t *testing.T) {
	// Aliases containing double-quote chars should be sanitized
	expansion := &glossary.QueryExpansion{
		OriginalQuery: "test",
		ExpandedTerms: []glossary.ExpansionResult{
			{
				OriginalTerm: "test",
				TermName:     `"quoted" term`,
				Aliases:      []string{},
			},
		},
	}

	result := buildAliasQuery("test", expansion)
	// The result should still be valid (no unmatched quotes)
	assert.NotContains(t, result, `""`)
	// The term should be included but with quotes stripped
	assert.Contains(t, result, "quoted")
}

func TestBuildAliasQuery_CaseInsensitiveDedup(t *testing.T) {
	// Same term in different cases should only appear once
	expansion := &glossary.QueryExpansion{
		OriginalQuery: "QFX5130",
		ExpandedTerms: []glossary.ExpansionResult{
			{
				OriginalTerm: "QFX5130",
				TermName:     "QFX5130",
				Aliases:      []string{"qfx5130", "QFX5130"},
			},
		},
	}

	result := buildAliasQuery("QFX5130", expansion)
	// Should appear only once (case-insensitive dedup)
	count := strings.Count(strings.ToLower(result), `"qfx5130"`)
	assert.Equal(t, 1, count, "same term in different cases should not be duplicated")
}

// ----------------------------------------------------------------------------
// buildExpansionInfo tests
// ----------------------------------------------------------------------------

func TestBuildExpansionInfo_SingleTermMultipleAliases(t *testing.T) {
	expansion := &glossary.QueryExpansion{
		OriginalQuery: "36QDD",
		ExpandedTerms: []glossary.ExpansionResult{
			{
				TermName: "Juniper Border Routers",
				Aliases:  []string{"border routers", "Juniper routers", "QFX5130-36QDD"},
			},
		},
	}

	info := buildExpansionInfo(expansion)
	assert.Equal(t, "36QDD expanded via Juniper Border Routers (3 aliases)", info)
}

func TestBuildExpansionInfo_SingleAlias(t *testing.T) {
	expansion := &glossary.QueryExpansion{
		OriginalQuery: "TER",
		ExpandedTerms: []glossary.ExpansionResult{
			{
				TermName: "Technical Execution Review",
				Aliases:  []string{"T.E.R."},
			},
		},
	}

	info := buildExpansionInfo(expansion)
	assert.Equal(t, "TER expanded via Technical Execution Review (1 alias)", info)
}

func TestBuildExpansionInfo_NoExpansion(t *testing.T) {
	expansion := &glossary.QueryExpansion{
		OriginalQuery: "random",
		ExpandedTerms: []glossary.ExpansionResult{},
	}

	info := buildExpansionInfo(expansion)
	assert.Equal(t, "", info)
}

func TestBuildExpansionInfo_MultipleTerms(t *testing.T) {
	expansion := &glossary.QueryExpansion{
		OriginalQuery: "query with two terms",
		ExpandedTerms: []glossary.ExpansionResult{
			{TermName: "Term One", Aliases: []string{"alias1", "alias2"}},
			{TermName: "Term Two", Aliases: []string{"other"}},
		},
	}

	info := buildExpansionInfo(expansion)
	assert.Contains(t, info, "Term One (2 aliases)")
	assert.Contains(t, info, "Term Two (1 alias)")
	assert.Contains(t, info, "; ")
}

// ----------------------------------------------------------------------------
// SetQueryExpander tests
// ----------------------------------------------------------------------------

func TestService_SetQueryExpander(t *testing.T) {
	svc := NewService(nil, nil)
	assert.Nil(t, svc.expander)

	mock := &mockExpander{}
	svc.SetQueryExpander(mock)
	assert.NotNil(t, svc.expander)
}

func TestService_SetQueryExpander_NilClearsExpander(t *testing.T) {
	svc := NewService(nil, nil)
	mock := &mockExpander{}
	svc.SetQueryExpander(mock)
	assert.NotNil(t, svc.expander)

	svc.SetQueryExpander(nil)
	assert.Nil(t, svc.expander)
}

// TestService_SearchWithNilExpanderStillWorks verifies that a nil expander
// does not break the search flow (DB nil check happens before query execution,
// so this just checks that the nil expander path is safe before the DB call).
func TestService_SearchWithNilExpanderStillWorks(t *testing.T) {
	svc := NewService(nil, nil) // nil DB triggers early exit
	assert.Nil(t, svc.expander)

	_, err := svc.Search(context.Background(), &searchv1.SearchRequest{
		Query:    "some query",
		TenantId: "tenant-id",
	})
	// Should return "database not configured", not a nil pointer panic
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database not configured")
}
