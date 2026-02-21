package searchservice

import (
	"context"
	"testing"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
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
