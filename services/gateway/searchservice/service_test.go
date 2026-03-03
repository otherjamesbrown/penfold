package searchservice

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
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

// applySortOrder sorts results in-place using the same logic as hybridSearch/keywordOnlySearch.
// This mirrors the sort.Slice logic in the service so tests can verify ordering without a DB.
func applySortOrder(results []*searchv1.SearchResult, sortOrder searchv1.SortOrder) {
	switch sortOrder {
	case searchv1.SortOrder_SORT_ORDER_DATE_DESC:
		sort.Slice(results, func(i, j int) bool {
			ti := results[i].CreatedAt.AsTime()
			tj := results[j].CreatedAt.AsTime()
			if ti.Equal(tj) {
				return results[i].Score > results[j].Score
			}
			return ti.After(tj)
		})
	case searchv1.SortOrder_SORT_ORDER_DATE_ASC:
		sort.Slice(results, func(i, j int) bool {
			ti := results[i].CreatedAt.AsTime()
			tj := results[j].CreatedAt.AsTime()
			if ti.Equal(tj) {
				return results[i].Score > results[j].Score
			}
			return ti.Before(tj)
		})
	default:
		// SORT_ORDER_UNSPECIFIED and SORT_ORDER_RELEVANCE: rank by blended score
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})
	}
}

func makeResultWithDateAndScore(created time.Time, score float32) *searchv1.SearchResult {
	return &searchv1.SearchResult{
		Score:     score,
		CreatedAt: timestamppb.New(created),
	}
}

func TestSortOrder_Relevance(t *testing.T) {
	now := time.Now()
	results := []*searchv1.SearchResult{
		makeResultWithDateAndScore(now.Add(-2*time.Hour), 0.3),
		makeResultWithDateAndScore(now.Add(-1*time.Hour), 0.9),
		makeResultWithDateAndScore(now, 0.6),
	}

	// Default (unspecified) should return score-ordered results
	applySortOrder(results, searchv1.SortOrder_SORT_ORDER_UNSPECIFIED)

	assert.Equal(t, float32(0.9), results[0].Score, "highest score should be first")
	assert.Equal(t, float32(0.6), results[1].Score, "second highest score should be second")
	assert.Equal(t, float32(0.3), results[2].Score, "lowest score should be last")
}

func TestSortOrder_RelevanceExplicit(t *testing.T) {
	now := time.Now()
	results := []*searchv1.SearchResult{
		makeResultWithDateAndScore(now.Add(-2*time.Hour), 0.3),
		makeResultWithDateAndScore(now.Add(-1*time.Hour), 0.9),
		makeResultWithDateAndScore(now, 0.6),
	}

	// Explicit RELEVANCE should also return score-ordered results
	applySortOrder(results, searchv1.SortOrder_SORT_ORDER_RELEVANCE)

	assert.Equal(t, float32(0.9), results[0].Score, "highest score should be first")
	assert.Equal(t, float32(0.6), results[1].Score, "second highest score should be second")
	assert.Equal(t, float32(0.3), results[2].Score, "lowest score should be last")
}

func TestSortOrder_DateDesc(t *testing.T) {
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Insert in random order to verify sort actually changes ordering
	results := []*searchv1.SearchResult{
		makeResultWithDateAndScore(old, 0.9),    // old but high score
		makeResultWithDateAndScore(recent, 0.3), // newest but low score
		makeResultWithDateAndScore(mid, 0.6),    // middle
	}

	applySortOrder(results, searchv1.SortOrder_SORT_ORDER_DATE_DESC)

	// Date desc: newest first regardless of score
	assert.Equal(t, recent, results[0].CreatedAt.AsTime(), "newest should be first")
	assert.Equal(t, mid, results[1].CreatedAt.AsTime(), "middle date should be second")
	assert.Equal(t, old, results[2].CreatedAt.AsTime(), "oldest should be last")
}

func TestSortOrder_DateAsc(t *testing.T) {
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Insert in random order to verify sort actually changes ordering
	results := []*searchv1.SearchResult{
		makeResultWithDateAndScore(recent, 0.9), // newest but high score
		makeResultWithDateAndScore(old, 0.3),    // oldest but low score
		makeResultWithDateAndScore(mid, 0.6),    // middle
	}

	applySortOrder(results, searchv1.SortOrder_SORT_ORDER_DATE_ASC)

	// Date asc: oldest first regardless of score
	assert.Equal(t, old, results[0].CreatedAt.AsTime(), "oldest should be first")
	assert.Equal(t, mid, results[1].CreatedAt.AsTime(), "middle date should be second")
	assert.Equal(t, recent, results[2].CreatedAt.AsTime(), "newest should be last")
}

func TestSortOrder_DateDesc_TieBreakByScore(t *testing.T) {
	sameTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	results := []*searchv1.SearchResult{
		makeResultWithDateAndScore(sameTime, 0.3),
		makeResultWithDateAndScore(sameTime, 0.9),
		makeResultWithDateAndScore(sameTime, 0.6),
	}

	applySortOrder(results, searchv1.SortOrder_SORT_ORDER_DATE_DESC)

	// Same date: tie-break by score desc
	assert.Equal(t, float32(0.9), results[0].Score, "highest score wins tie")
	assert.Equal(t, float32(0.6), results[1].Score, "second highest score is second")
	assert.Equal(t, float32(0.3), results[2].Score, "lowest score is last")
}

func TestSortOrder_DateAsc_TieBreakByScore(t *testing.T) {
	sameTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	results := []*searchv1.SearchResult{
		makeResultWithDateAndScore(sameTime, 0.3),
		makeResultWithDateAndScore(sameTime, 0.9),
		makeResultWithDateAndScore(sameTime, 0.6),
	}

	applySortOrder(results, searchv1.SortOrder_SORT_ORDER_DATE_ASC)

	// Same date: tie-break by score desc
	assert.Equal(t, float32(0.9), results[0].Score, "highest score wins tie")
	assert.Equal(t, float32(0.6), results[1].Score, "second highest score is second")
	assert.Equal(t, float32(0.3), results[2].Score, "lowest score is last")
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

// ----------------------------------------------------------------------------
// buildSQL tests
// ----------------------------------------------------------------------------

func TestBuildSQL_NilFilter(t *testing.T) {
	var rf *resolvedFilter
	sql, args := rf.buildSQL("tenant-abc", 1)
	assert.Equal(t, "", sql)
	assert.Nil(t, args)
}

func TestBuildSQL_EmptyFilter(t *testing.T) {
	rf := &resolvedFilter{}
	sql, args := rf.buildSQL("tenant-abc", 1)
	assert.Equal(t, "", sql)
	assert.Nil(t, args)
}

func TestBuildSQL_Impossible(t *testing.T) {
	rf := &resolvedFilter{impossible: true}
	sql, args := rf.buildSQL("tenant-abc", 1)
	assert.Equal(t, "\n\t\t\t\tAND FALSE", sql)
	assert.Nil(t, args)
}

func TestBuildSQL_ContentTypes(t *testing.T) {
	rf := &resolvedFilter{
		contentTypes: []string{"email", "meeting"},
	}
	sql, args := rf.buildSQL("tenant-abc", 3)

	assert.Contains(t, sql, "s.content_type = ANY($3)")
	require.Len(t, args, 1)
	assert.Equal(t, []string{"email", "meeting"}, args[0])
}

func TestBuildSQL_DateRange(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)
	rf := &resolvedFilter{
		dateFrom: &from,
		dateTo:   &to,
	}
	sql, args := rf.buildSQL("tenant-abc", 5)

	assert.Contains(t, sql, "s.source_timestamp >= $5")
	assert.Contains(t, sql, "s.source_timestamp <= $6")
	require.Len(t, args, 2)
	assert.Equal(t, from, args[0])
	assert.Equal(t, to, args[1])
}

func TestBuildSQL_Sources(t *testing.T) {
	rf := &resolvedFilter{
		sources: []string{"gmail", "slack"},
	}
	sql, args := rf.buildSQL("tenant-abc", 1)

	assert.Contains(t, sql, "s.source_system = ANY($1)")
	require.Len(t, args, 1)
	assert.Equal(t, []string{"gmail", "slack"}, args[0])
}

func TestBuildSQL_ExcludeIDs(t *testing.T) {
	rf := &resolvedFilter{
		excludeIDs: []string{"em-abc123", "em-xyz789"},
	}
	sql, args := rf.buildSQL("tenant-abc", 2)

	assert.Contains(t, sql, "s.content_id != ALL($2)")
	require.Len(t, args, 1)
	assert.Equal(t, []string{"em-abc123", "em-xyz789"}, args[0])
}

func TestBuildSQL_EntityRoleWithRoles(t *testing.T) {
	rf := &resolvedFilter{
		entityRoles: []resolvedEntityRole{
			{entityID: 42, roles: []int16{1, 2}},
		},
	}
	sql, args := rf.buildSQL("tenant-abc", 1)

	// Entity role clause: entityID first, then tenantID, then roles
	assert.Contains(t, sql, "content_mentions")
	assert.Contains(t, sql, "cm.resolved_entity_id = $1")
	assert.Contains(t, sql, "cm.tenant_id = $2")
	assert.Contains(t, sql, "cm.participation_role = ANY($3::smallint[])")
	require.Len(t, args, 3)
	assert.Equal(t, int64(42), args[0])
	assert.Equal(t, "tenant-abc", args[1])
	assert.Equal(t, []int16{1, 2}, args[2])
}

func TestBuildSQL_EntityRoleAnyRole(t *testing.T) {
	rf := &resolvedFilter{
		entityRoles: []resolvedEntityRole{
			{entityID: 99, roles: nil},
		},
	}
	sql, args := rf.buildSQL("tenant-abc", 4)

	assert.Contains(t, sql, "content_mentions")
	assert.Contains(t, sql, "cm.resolved_entity_id = $4")
	assert.Contains(t, sql, "cm.tenant_id = $5")
	// No role filter in SQL
	assert.NotContains(t, sql, "participation_role")
	require.Len(t, args, 2)
	assert.Equal(t, int64(99), args[0])
	assert.Equal(t, "tenant-abc", args[1])
}

func TestBuildSQL_MultipleEntityRoles(t *testing.T) {
	rf := &resolvedFilter{
		entityRoles: []resolvedEntityRole{
			{entityID: 10, roles: []int16{3}},
			{entityID: 20, roles: nil},
		},
	}
	sql, args := rf.buildSQL("tenant-abc", 1)

	// Both entity roles generate separate AND clauses
	assert.Contains(t, sql, "cm.resolved_entity_id = $1")
	assert.Contains(t, sql, "cm.tenant_id = $2")
	assert.Contains(t, sql, "participation_role = ANY($3::smallint[])")
	assert.Contains(t, sql, "cm.resolved_entity_id = $4")
	assert.Contains(t, sql, "cm.tenant_id = $5")
	// Second entity has no role filter
	require.Len(t, args, 5)
	assert.Equal(t, int64(10), args[0])
	assert.Equal(t, "tenant-abc", args[1])
	assert.Equal(t, []int16{3}, args[2])
	assert.Equal(t, int64(20), args[3])
	assert.Equal(t, "tenant-abc", args[4])
}

func TestBuildSQL_CombinedFilters(t *testing.T) {
	rf := &resolvedFilter{
		contentTypes: []string{"email"},
		entityRoles: []resolvedEntityRole{
			{entityID: 55, roles: []int16{1}},
		},
	}
	// nextParam starts at 5 (simulating hybrid query with $1-$4 used)
	sql, args := rf.buildSQL("tenant-xyz", 5)

	// contentTypes clause uses $5
	assert.Contains(t, sql, "s.content_type = ANY($5)")
	// entity role: entityID=$6, tenantID=$7, roles=$8
	assert.Contains(t, sql, "cm.resolved_entity_id = $6")
	assert.Contains(t, sql, "cm.tenant_id = $7")
	assert.Contains(t, sql, "participation_role = ANY($8::smallint[])")

	require.Len(t, args, 4)
	assert.Equal(t, []string{"email"}, args[0])
	assert.Equal(t, int64(55), args[1])
	assert.Equal(t, "tenant-xyz", args[2])
	assert.Equal(t, []int16{1}, args[3])
}

func TestBuildSQL_ParamNumbering(t *testing.T) {
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	rf := &resolvedFilter{
		contentTypes: []string{"meeting"},
		dateFrom:     &from,
		dateTo:       &to,
		sources:      []string{"zoom"},
		excludeIDs:   []string{"em-001"},
	}
	// Start at $10 to verify offset is respected
	sql, args := rf.buildSQL("tenant-abc", 10)

	assert.Contains(t, sql, "s.content_type = ANY($10)")
	assert.Contains(t, sql, "s.source_timestamp >= $11")
	assert.Contains(t, sql, "s.source_timestamp <= $12")
	assert.Contains(t, sql, "s.source_system = ANY($13)")
	assert.Contains(t, sql, "s.content_id != ALL($14)")

	require.Len(t, args, 5)
	assert.Equal(t, []string{"meeting"}, args[0])
	assert.Equal(t, from, args[1])
	assert.Equal(t, to, args[2])
	assert.Equal(t, []string{"zoom"}, args[3])
	assert.Equal(t, []string{"em-001"}, args[4])
}
