package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultRRFConfig(t *testing.T) {
	cfg := DefaultRRFConfig()

	if cfg.K != DefaultRRFK {
		t.Errorf("expected K=%d, got %d", DefaultRRFK, cfg.K)
	}
	if cfg.BM25Weight != 0.5 {
		t.Errorf("expected BM25Weight=0.5, got %f", cfg.BM25Weight)
	}
	if cfg.VectorWeight != 0.5 {
		t.Errorf("expected VectorWeight=0.5, got %f", cfg.VectorWeight)
	}
	if cfg.DefaultSearchMode != SearchModeHybrid {
		t.Errorf("expected DefaultSearchMode=hybrid, got %s", cfg.DefaultSearchMode)
	}
	if !cfg.GracefulDegradation {
		t.Error("expected GracefulDegradation=true")
	}
	if !cfg.ParallelExecution {
		t.Error("expected ParallelExecution=true")
	}
}

func TestSearchMode_String(t *testing.T) {
	tests := []struct {
		mode     SearchMode
		expected string
	}{
		{SearchModeHybrid, "hybrid"},
		{SearchModeKeyword, "keyword"},
		{SearchModeSemantic, "semantic"},
		{SearchMode(99), "hybrid"}, // unknown defaults to hybrid
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.expected {
				t.Errorf("SearchMode.String() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestParseSearchMode(t *testing.T) {
	tests := []struct {
		input    string
		expected SearchMode
	}{
		// Keyword mode aliases
		{"keyword", SearchModeKeyword},
		{"bm25", SearchModeKeyword},
		{"fulltext", SearchModeKeyword},
		{"text", SearchModeKeyword},
		// Semantic mode aliases
		{"semantic", SearchModeSemantic},
		{"vector", SearchModeSemantic},
		{"embedding", SearchModeSemantic},
		// Hybrid (default)
		{"hybrid", SearchModeHybrid},
		{"", SearchModeHybrid},
		{"unknown", SearchModeHybrid},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseSearchMode(tt.input); got != tt.expected {
				t.Errorf("ParseSearchMode(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNewRRFEngine(t *testing.T) {
	logger := newMockLogger()
	bm25 := NewBM25Engine(nil, logger, nil)
	vector := NewVectorEngine(nil, newMockEmbeddingClient(384), nil, logger)

	t.Run("with nil config uses defaults", func(t *testing.T) {
		engine := NewRRFEngine(bm25, vector, nil, logger)

		if engine == nil {
			t.Fatal("expected engine to be created")
		}
		if engine.config.K != DefaultRRFK {
			t.Errorf("expected default K=%d, got %d", DefaultRRFK, engine.config.K)
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &RRFConfig{
			K:            100,
			BM25Weight:   0.7,
			VectorWeight: 0.3,
		}

		engine := NewRRFEngine(bm25, vector, cfg, logger)

		if engine.config.K != 100 {
			t.Errorf("expected K=100, got %d", engine.config.K)
		}
		if engine.config.BM25Weight != 0.7 {
			t.Errorf("expected BM25Weight=0.7, got %f", engine.config.BM25Weight)
		}
	})

	t.Run("with zero K uses default", func(t *testing.T) {
		cfg := &RRFConfig{K: 0}
		engine := NewRRFEngine(bm25, vector, cfg, logger)

		if engine.config.K != DefaultRRFK {
			t.Errorf("expected K=%d for zero config, got %d", DefaultRRFK, engine.config.K)
		}
	})

	t.Run("with negative K uses default", func(t *testing.T) {
		cfg := &RRFConfig{K: -10}
		engine := NewRRFEngine(bm25, vector, cfg, logger)

		if engine.config.K != DefaultRRFK {
			t.Errorf("expected K=%d for negative config, got %d", DefaultRRFK, engine.config.K)
		}
	})
}

func TestCalculateRRFScore(t *testing.T) {
	tests := []struct {
		name       string
		bm25Rank   int
		vectorRank int
		k          int
		bm25Weight float64
		vecWeight  float64
		expected   float64
	}{
		{
			name:       "both results at rank 1",
			bm25Rank:   1,
			vectorRank: 1,
			k:          60,
			bm25Weight: 0.5,
			vecWeight:  0.5,
			expected:   0.5*(1.0/61) + 0.5*(1.0/61),
		},
		{
			name:       "only BM25 result",
			bm25Rank:   1,
			vectorRank: 0,
			k:          60,
			bm25Weight: 0.5,
			vecWeight:  0.5,
			expected:   0.5 * (1.0 / 61),
		},
		{
			name:       "only vector result",
			bm25Rank:   0,
			vectorRank: 1,
			k:          60,
			bm25Weight: 0.5,
			vecWeight:  0.5,
			expected:   0.5 * (1.0 / 61),
		},
		{
			name:       "different ranks",
			bm25Rank:   1,
			vectorRank: 5,
			k:          60,
			bm25Weight: 0.5,
			vecWeight:  0.5,
			expected:   0.5*(1.0/61) + 0.5*(1.0/65),
		},
		{
			name:       "weighted towards BM25",
			bm25Rank:   1,
			vectorRank: 1,
			k:          60,
			bm25Weight: 0.8,
			vecWeight:  0.2,
			expected:   0.8*(1.0/61) + 0.2*(1.0/61),
		},
		{
			name:       "neither result",
			bm25Rank:   0,
			vectorRank: 0,
			k:          60,
			bm25Weight: 0.5,
			vecWeight:  0.5,
			expected:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateRRFScore(tt.bm25Rank, tt.vectorRank, tt.k, tt.bm25Weight, tt.vecWeight)

			// Use approximate comparison for floating point
			if diff := got - tt.expected; diff < -0.0001 || diff > 0.0001 {
				t.Errorf("CalculateRRFScore() = %f, want %f", got, tt.expected)
			}
		})
	}
}

func TestRRFEngine_fuseResults(t *testing.T) {
	logger := newMockLogger()
	cfg := &RRFConfig{
		K:            60,
		BM25Weight:   0.5,
		VectorWeight: 0.5,
	}
	engine := NewRRFEngine(nil, nil, cfg, logger)

	title1 := "Document 1"
	title2 := "Document 2"
	title3 := "Document 3"
	now := time.Now()

	t.Run("merges overlapping results", func(t *testing.T) {
		bm25Results := &SearchResponse{
			Results: []SearchResult{
				{DocumentID: "doc-1", Title: &title1, Score: 0.9, ContentType: "email", CreatedAt: now, UpdatedAt: now},
				{DocumentID: "doc-2", Title: &title2, Score: 0.7, ContentType: "email", CreatedAt: now, UpdatedAt: now},
			},
			TotalCount: 2,
		}

		vectorResults := &VectorSearchResponse{
			Results: []VectorSearchResult{
				{DocumentID: "doc-2", Title: &title2, SimilarityScore: 0.95, ContentType: "email", CreatedAt: now, UpdatedAt: now},
				{DocumentID: "doc-3", Title: &title3, SimilarityScore: 0.8, ContentType: "email", CreatedAt: now, UpdatedAt: now},
			},
			TotalCount: 2,
		}

		fused := engine.fuseResults(bm25Results, vectorResults, 60)

		if len(fused) != 3 {
			t.Fatalf("expected 3 unique documents, got %d", len(fused))
		}

		// doc-2 should be ranked highest (appears in both)
		if fused[0].DocumentID != "doc-2" {
			t.Errorf("expected doc-2 to be first (appears in both), got %s", fused[0].DocumentID)
		}

		// doc-2 should have both scores
		if fused[0].BM25Rank == 0 || fused[0].VectorRank == 0 {
			t.Error("doc-2 should have both BM25 and vector ranks")
		}
	})

	t.Run("handles BM25 only results", func(t *testing.T) {
		bm25Results := &SearchResponse{
			Results: []SearchResult{
				{DocumentID: "doc-1", Title: &title1, Score: 0.9, CreatedAt: now, UpdatedAt: now},
			},
		}

		fused := engine.fuseResults(bm25Results, nil, 60)

		if len(fused) != 1 {
			t.Fatalf("expected 1 result, got %d", len(fused))
		}

		if fused[0].BM25Rank != 1 {
			t.Errorf("expected BM25Rank=1, got %d", fused[0].BM25Rank)
		}
		if fused[0].VectorRank != 0 {
			t.Errorf("expected VectorRank=0, got %d", fused[0].VectorRank)
		}
	})

	t.Run("handles vector only results", func(t *testing.T) {
		vectorResults := &VectorSearchResponse{
			Results: []VectorSearchResult{
				{DocumentID: "doc-1", Title: &title1, SimilarityScore: 0.9, CreatedAt: now, UpdatedAt: now},
			},
		}

		fused := engine.fuseResults(nil, vectorResults, 60)

		if len(fused) != 1 {
			t.Fatalf("expected 1 result, got %d", len(fused))
		}

		if fused[0].VectorRank != 1 {
			t.Errorf("expected VectorRank=1, got %d", fused[0].VectorRank)
		}
		if fused[0].BM25Rank != 0 {
			t.Errorf("expected BM25Rank=0, got %d", fused[0].BM25Rank)
		}
	})

	t.Run("handles empty results", func(t *testing.T) {
		fused := engine.fuseResults(nil, nil, 60)

		if len(fused) != 0 {
			t.Fatalf("expected 0 results, got %d", len(fused))
		}
	})

	t.Run("results are sorted by score descending", func(t *testing.T) {
		bm25Results := &SearchResponse{
			Results: []SearchResult{
				{DocumentID: "doc-low", Title: &title1, Score: 0.1, CreatedAt: now, UpdatedAt: now},
				{DocumentID: "doc-high", Title: &title2, Score: 0.9, CreatedAt: now, UpdatedAt: now},
			},
		}

		fused := engine.fuseResults(bm25Results, nil, 60)

		if len(fused) < 2 {
			t.Fatalf("expected at least 2 results")
		}

		if fused[0].Score < fused[1].Score {
			t.Error("results should be sorted by score descending")
		}
	})

	t.Run("scores are normalized to 0-1", func(t *testing.T) {
		bm25Results := &SearchResponse{
			Results: []SearchResult{
				{DocumentID: "doc-1", Score: 0.9, CreatedAt: now, UpdatedAt: now},
				{DocumentID: "doc-2", Score: 0.5, CreatedAt: now, UpdatedAt: now},
			},
		}

		fused := engine.fuseResults(bm25Results, nil, 60)

		for i, r := range fused {
			if r.Score < 0 || r.Score > 1 {
				t.Errorf("result %d has score %f outside [0,1] range", i, r.Score)
			}
		}

		// First result should be normalized to 1.0
		if fused[0].Score != 1.0 {
			t.Errorf("expected max score to be normalized to 1.0, got %f", fused[0].Score)
		}
	})
}

func TestRRFEngine_normalizeScores(t *testing.T) {
	engine := NewRRFEngine(nil, nil, nil, newMockLogger())

	t.Run("normalizes non-empty results", func(t *testing.T) {
		results := []HybridSearchResult{
			{Score: 0.1},
			{Score: 0.05},
			{Score: 0.02},
		}

		engine.normalizeScores(results)

		// Use approximate comparison for floating point
		if diff := results[0].Score - 1.0; diff < -0.0001 || diff > 0.0001 {
			t.Errorf("expected max score 1.0, got %f", results[0].Score)
		}
		if diff := results[1].Score - 0.5; diff < -0.0001 || diff > 0.0001 {
			t.Errorf("expected score 0.5, got %f", results[1].Score)
		}
		if diff := results[2].Score - 0.2; diff < -0.0001 || diff > 0.0001 {
			t.Errorf("expected score 0.2, got %f", results[2].Score)
		}
	})

	t.Run("handles empty results", func(t *testing.T) {
		results := []HybridSearchResult{}
		engine.normalizeScores(results) // Should not panic
	})

	t.Run("handles zero max score", func(t *testing.T) {
		results := []HybridSearchResult{
			{Score: 0},
			{Score: 0},
		}

		engine.normalizeScores(results)

		// Scores should remain 0
		for _, r := range results {
			if r.Score != 0 {
				t.Errorf("expected score 0, got %f", r.Score)
			}
		}
	})
}

func TestRRFEngine_SetWeights(t *testing.T) {
	engine := NewRRFEngine(nil, nil, nil, newMockLogger())

	t.Run("valid weights", func(t *testing.T) {
		err := engine.SetWeights(0.7, 0.3)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if engine.config.BM25Weight != 0.7 {
			t.Errorf("expected BM25Weight=0.7, got %f", engine.config.BM25Weight)
		}
		if engine.config.VectorWeight != 0.3 {
			t.Errorf("expected VectorWeight=0.3, got %f", engine.config.VectorWeight)
		}
	})

	t.Run("invalid BM25 weight negative", func(t *testing.T) {
		err := engine.SetWeights(-0.1, 0.5)
		if err == nil {
			t.Error("expected error for negative BM25 weight")
		}
	})

	t.Run("invalid BM25 weight > 1", func(t *testing.T) {
		err := engine.SetWeights(1.1, 0.5)
		if err == nil {
			t.Error("expected error for BM25 weight > 1")
		}
	})

	t.Run("invalid vector weight negative", func(t *testing.T) {
		err := engine.SetWeights(0.5, -0.1)
		if err == nil {
			t.Error("expected error for negative vector weight")
		}
	})

	t.Run("invalid vector weight > 1", func(t *testing.T) {
		err := engine.SetWeights(0.5, 1.1)
		if err == nil {
			t.Error("expected error for vector weight > 1")
		}
	})
}

func TestRRFEngine_GetConfig(t *testing.T) {
	cfg := &RRFConfig{
		K:            100,
		BM25Weight:   0.6,
		VectorWeight: 0.4,
	}
	engine := NewRRFEngine(nil, nil, cfg, newMockLogger())

	got := engine.GetConfig()

	if got.K != 100 {
		t.Errorf("expected K=100, got %d", got.K)
	}
	if got.BM25Weight != 0.6 {
		t.Errorf("expected BM25Weight=0.6, got %f", got.BM25Weight)
	}
}

func TestRRFEngine_SetConfig(t *testing.T) {
	engine := NewRRFEngine(nil, nil, nil, newMockLogger())

	t.Run("sets new config", func(t *testing.T) {
		newCfg := &RRFConfig{K: 100}
		engine.SetConfig(newCfg)

		if engine.config.K != 100 {
			t.Errorf("expected K=100, got %d", engine.config.K)
		}
	})

	t.Run("ignores nil config", func(t *testing.T) {
		oldK := engine.config.K
		engine.SetConfig(nil)

		if engine.config.K != oldK {
			t.Error("config should not change with nil")
		}
	})
}

func TestRRFEngine_toBM25Filters(t *testing.T) {
	engine := NewRRFEngine(nil, nil, nil, newMockLogger())

	t.Run("converts filters correctly", func(t *testing.T) {
		now := time.Now()
		filters := &HybridSearchFilters{
			ContentTypes: []string{"email", "document"},
			DateFrom:     &now,
			DateTo:       &now,
			Tags:         []string{"important"},
			SourceIDs:    []string{"gmail"},
			ExcludeIDs:   []string{"doc-1"},
		}

		result := engine.toBM25Filters("tenant-123", filters)

		if result.TenantID != "tenant-123" {
			t.Errorf("expected TenantID='tenant-123', got '%s'", result.TenantID)
		}
		if len(result.ContentTypes) != 2 {
			t.Errorf("expected 2 content types, got %d", len(result.ContentTypes))
		}
		if result.DateFrom == nil {
			t.Error("expected DateFrom to be set")
		}
	})

	t.Run("handles nil filters", func(t *testing.T) {
		result := engine.toBM25Filters("tenant-123", nil)

		if result.TenantID != "tenant-123" {
			t.Errorf("expected TenantID='tenant-123', got '%s'", result.TenantID)
		}
	})
}

func TestRRFEngine_toVectorFilters(t *testing.T) {
	engine := NewRRFEngine(nil, nil, nil, newMockLogger())

	t.Run("converts filters correctly", func(t *testing.T) {
		now := time.Now()
		filters := &HybridSearchFilters{
			ContentTypes: []string{"email"},
			DateFrom:     &now,
			SourceIDs:    []string{"gmail"},
		}

		result := engine.toVectorFilters(filters)

		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if len(result.ContentTypes) != 1 {
			t.Errorf("expected 1 content type, got %d", len(result.ContentTypes))
		}
	})

	t.Run("handles nil filters", func(t *testing.T) {
		result := engine.toVectorFilters(nil)

		if result != nil {
			t.Error("expected nil result for nil input")
		}
	})
}

func TestRRFEngine_estimateTotalCount(t *testing.T) {
	engine := NewRRFEngine(nil, nil, nil, newMockLogger())

	t.Run("returns max of both counts", func(t *testing.T) {
		bm25Results := &SearchResponse{TotalCount: 100}
		vectorResults := &VectorSearchResponse{TotalCount: 150}

		count := engine.estimateTotalCount(bm25Results, vectorResults)

		if count != 150 {
			t.Errorf("expected 150, got %d", count)
		}
	})

	t.Run("handles nil BM25", func(t *testing.T) {
		vectorResults := &VectorSearchResponse{TotalCount: 50}

		count := engine.estimateTotalCount(nil, vectorResults)

		if count != 50 {
			t.Errorf("expected 50, got %d", count)
		}
	})

	t.Run("handles nil vector", func(t *testing.T) {
		bm25Results := &SearchResponse{TotalCount: 75}

		count := engine.estimateTotalCount(bm25Results, nil)

		if count != 75 {
			t.Errorf("expected 75, got %d", count)
		}
	})

	t.Run("handles both nil", func(t *testing.T) {
		count := engine.estimateTotalCount(nil, nil)

		if count != 0 {
			t.Errorf("expected 0, got %d", count)
		}
	})
}

func TestRRFEngine_HybridSearch_Validation(t *testing.T) {
	logger := newMockLogger()
	bm25 := NewBM25Engine(nil, logger, nil)
	vector := NewVectorEngine(nil, newMockEmbeddingClient(384), nil, logger)
	engine := NewRRFEngine(bm25, vector, nil, logger)

	t.Run("requires tenant_id", func(t *testing.T) {
		_, err := engine.HybridSearch(context.Background(), "", "test query", nil, 10, 0)

		if !errors.Is(err, ErrInvalidTenantID) {
			t.Errorf("expected ErrInvalidTenantID, got %v", err)
		}
	})
}

func TestHybridSearchFilters(t *testing.T) {
	now := time.Now()

	filters := &HybridSearchFilters{
		ContentTypes: []string{"email", "document"},
		DateFrom:     &now,
		DateTo:       &now,
		Tags:         []string{"important", "work"},
		SourceIDs:    []string{"gmail", "slack"},
		ExcludeIDs:   []string{"doc-1", "doc-2"},
		SearchMode:   SearchModeKeyword,
	}

	if len(filters.ContentTypes) != 2 {
		t.Errorf("expected 2 content types")
	}
	if filters.DateFrom == nil || filters.DateTo == nil {
		t.Error("expected dates to be set")
	}
	if filters.SearchMode != SearchModeKeyword {
		t.Errorf("expected SearchModeKeyword, got %v", filters.SearchMode)
	}
}

func TestHybridSearchResult(t *testing.T) {
	title := "Test Document"
	now := time.Now()

	result := HybridSearchResult{
		DocumentID:  "doc-123",
		ContentType: "email",
		SourceID:    "gmail-456",
		Title:       &title,
		Snippet:     "This is a snippet...",
		Highlights:  []string{"<em>test</em> match"},
		Score:       0.85,
		BM25Score:   0.9,
		VectorScore: 0.8,
		BM25Rank:    1,
		VectorRank:  2,
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata:    map[string]string{"key": "value"},
	}

	if result.DocumentID != "doc-123" {
		t.Error("DocumentID not set correctly")
	}
	if result.Score != 0.85 {
		t.Error("Score not set correctly")
	}
	if result.BM25Rank != 1 || result.VectorRank != 2 {
		t.Error("Ranks not set correctly")
	}
}

func TestHybridSearchResponse(t *testing.T) {
	response := &HybridSearchResponse{
		Results: []HybridSearchResult{
			{DocumentID: "doc-1", Score: 0.9},
			{DocumentID: "doc-2", Score: 0.8},
		},
		TotalCount:   100,
		QueryTimeMs:  25.5,
		BM25TimeMs:   10.2,
		VectorTimeMs: 15.3,
		SearchMode:   SearchModeHybrid,
		BM25Error:    nil,
		VectorError:  nil,
	}

	if len(response.Results) != 2 {
		t.Errorf("expected 2 results")
	}
	if response.TotalCount != 100 {
		t.Error("TotalCount not set correctly")
	}
	if response.SearchMode != SearchModeHybrid {
		t.Error("SearchMode not set correctly")
	}
}

func TestRRFEngine_detectSearchMode(t *testing.T) {
	engine := NewRRFEngine(nil, nil, nil, newMockLogger())

	t.Run("respects explicit mode", func(t *testing.T) {
		mode := engine.detectSearchMode("test query", SearchModeKeyword)
		if mode != SearchModeKeyword {
			t.Errorf("expected SearchModeKeyword, got %v", mode)
		}

		mode = engine.detectSearchMode("test query", SearchModeSemantic)
		if mode != SearchModeSemantic {
			t.Errorf("expected SearchModeSemantic, got %v", mode)
		}
	})

	t.Run("defaults to hybrid", func(t *testing.T) {
		mode := engine.detectSearchMode("test query", SearchModeHybrid)
		if mode != SearchModeHybrid {
			t.Errorf("expected SearchModeHybrid, got %v", mode)
		}
	})
}

func TestRRFEngine_RankOrdering(t *testing.T) {
	// Test that RRF correctly orders results where doc appearing in both
	// lists at high rank beats doc appearing in only one list
	logger := newMockLogger()
	cfg := &RRFConfig{
		K:            60,
		BM25Weight:   0.5,
		VectorWeight: 0.5,
	}
	engine := NewRRFEngine(nil, nil, cfg, logger)

	title1 := "Doc 1"
	title2 := "Doc 2"
	title3 := "Doc 3"
	now := time.Now()

	// doc-overlap appears #2 in both lists
	// doc-bm25-only appears #1 in BM25 only
	// doc-vector-only appears #1 in vector only
	bm25Results := &SearchResponse{
		Results: []SearchResult{
			{DocumentID: "doc-bm25-only", Title: &title1, Score: 0.99, CreatedAt: now, UpdatedAt: now},
			{DocumentID: "doc-overlap", Title: &title2, Score: 0.9, CreatedAt: now, UpdatedAt: now},
		},
	}

	vectorResults := &VectorSearchResponse{
		Results: []VectorSearchResult{
			{DocumentID: "doc-vector-only", Title: &title3, SimilarityScore: 0.99, CreatedAt: now, UpdatedAt: now},
			{DocumentID: "doc-overlap", Title: &title2, SimilarityScore: 0.9, CreatedAt: now, UpdatedAt: now},
		},
	}

	fused := engine.fuseResults(bm25Results, vectorResults, 60)

	// doc-overlap should be first because it appears in both lists
	// RRF score for doc-overlap: 0.5 * 1/62 + 0.5 * 1/62 = 1/62
	// RRF score for doc-bm25-only: 0.5 * 1/61 = 0.5/61
	// RRF score for doc-vector-only: 0.5 * 1/61 = 0.5/61
	// 1/62 > 0.5/61 (0.0161 > 0.0082)

	if len(fused) != 3 {
		t.Fatalf("expected 3 results, got %d", len(fused))
	}

	if fused[0].DocumentID != "doc-overlap" {
		t.Errorf("expected doc-overlap to be first, got %s", fused[0].DocumentID)
	}

	// Both BM25-only and vector-only should have same score, order doesn't matter
	singleDocIDs := map[string]bool{fused[1].DocumentID: true, fused[2].DocumentID: true}
	if !singleDocIDs["doc-bm25-only"] || !singleDocIDs["doc-vector-only"] {
		t.Error("expected both single-source docs to be in results")
	}
}
