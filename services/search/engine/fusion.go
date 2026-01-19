// Package engine provides search engine implementations for the Search service.
// This file implements Reciprocal Rank Fusion (RRF) for combining BM25 and vector search results.
package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// SearchMode defines the type of search to perform.
type SearchMode int

const (
	// SearchModeHybrid combines BM25 and vector search using RRF fusion.
	SearchModeHybrid SearchMode = iota
	// SearchModeKeyword uses only BM25 full-text search.
	SearchModeKeyword
	// SearchModeSemantic uses only vector similarity search.
	SearchModeSemantic
)

// String returns the string representation of a SearchMode.
func (m SearchMode) String() string {
	switch m {
	case SearchModeHybrid:
		return "hybrid"
	case SearchModeKeyword:
		return "keyword"
	case SearchModeSemantic:
		return "semantic"
	default:
		return "hybrid"
	}
}

// ParseSearchMode converts a string to SearchMode.
func ParseSearchMode(s string) SearchMode {
	switch s {
	case "keyword", "bm25", "fulltext", "text":
		return SearchModeKeyword
	case "semantic", "vector", "embedding":
		return SearchModeSemantic
	default:
		return SearchModeHybrid
	}
}

// DefaultRRFK is the standard RRF constant that balances between favoring
// high-ranked documents while still giving weight to lower-ranked ones.
// A value of 60 is commonly used in literature and practice.
const DefaultRRFK = 60

// RRFConfig holds configuration for the RRF fusion engine.
type RRFConfig struct {
	// K is the RRF constant used in the formula: score = 1/(k + rank)
	// Higher k values reduce the impact of rank differences.
	// Default: 60
	K int

	// BM25Weight is the weight applied to BM25 results (0.0 to 1.0).
	// Default: 0.5
	BM25Weight float64

	// VectorWeight is the weight applied to vector results (0.0 to 1.0).
	// Default: 0.5
	VectorWeight float64

	// DefaultSearchMode is the search mode when none is specified.
	// Default: SearchModeHybrid
	DefaultSearchMode SearchMode

	// GracefulDegradation allows search to continue if one engine fails.
	// Default: true
	GracefulDegradation bool

	// ParallelExecution enables parallel BM25 and vector searches.
	// Default: true
	ParallelExecution bool
}

// DefaultRRFConfig returns configuration with sensible defaults.
func DefaultRRFConfig() *RRFConfig {
	return &RRFConfig{
		K:                   DefaultRRFK,
		BM25Weight:          0.5,
		VectorWeight:        0.5,
		DefaultSearchMode:   SearchModeHybrid,
		GracefulDegradation: true,
		ParallelExecution:   true,
	}
}

// HybridSearchFilters contains filter options for hybrid search.
// This is a unified filter structure that works with both BM25 and vector engines.
type HybridSearchFilters struct {
	// ContentTypes filters by content type (email, document, meeting, etc.)
	ContentTypes []string

	// DateFrom is the inclusive start date for filtering
	DateFrom *time.Time

	// DateTo is the inclusive end date for filtering
	DateTo *time.Time

	// Tags filters by document tags
	Tags []string

	// SourceIDs filters by source identifiers
	SourceIDs []string

	// ExcludeIDs excludes specific document IDs from results
	ExcludeIDs []string

	// SearchMode specifies keyword, semantic, or hybrid search
	SearchMode SearchMode
}

// HybridSearchResult represents a fused search result from RRF.
type HybridSearchResult struct {
	// DocumentID is the unique document identifier
	DocumentID string

	// ContentType is the type of content (email, document, etc.)
	ContentType string

	// SourceID links to the original content source
	SourceID string

	// Title is the document title (optional)
	Title *string

	// Snippet is a text excerpt from the document
	Snippet string

	// Highlights contains highlighted portions with <em> tags
	Highlights []string

	// Score is the final RRF-fused score (0.0 to 1.0)
	Score float64

	// BM25Score is the original BM25 score (if available)
	BM25Score float64

	// VectorScore is the original vector similarity score (if available)
	VectorScore float64

	// BM25Rank is the rank in BM25 results (0 if not found)
	BM25Rank int

	// VectorRank is the rank in vector results (0 if not found)
	VectorRank int

	// CreatedAt is the document creation timestamp
	CreatedAt time.Time

	// UpdatedAt is the document update timestamp
	UpdatedAt time.Time

	// Metadata contains additional document metadata
	Metadata map[string]string
}

// HybridSearchResponse wraps the fused search results with metadata.
type HybridSearchResponse struct {
	// Results are the fused search results ordered by combined score
	Results []HybridSearchResult

	// TotalCount is an estimate of total matching documents
	TotalCount int64

	// QueryTimeMs is the total time taken to execute the search
	QueryTimeMs float64

	// BM25TimeMs is the time taken for BM25 search
	BM25TimeMs float64

	// VectorTimeMs is the time taken for vector search
	VectorTimeMs float64

	// SearchMode indicates which search mode was used
	SearchMode SearchMode

	// BM25Error contains error if BM25 search failed (and graceful degradation enabled)
	BM25Error error

	// VectorError contains error if vector search failed (and graceful degradation enabled)
	VectorError error
}

// RRFEngine implements hybrid search using Reciprocal Rank Fusion.
// It combines results from BM25 full-text search and vector similarity search.
type RRFEngine struct {
	bm25Engine   *BM25Engine
	vectorEngine *VectorEngine
	config       *RRFConfig
	logger       logging.Logger
}

// NewRRFEngine creates a new RRF fusion engine.
func NewRRFEngine(
	bm25Engine *BM25Engine,
	vectorEngine *VectorEngine,
	cfg *RRFConfig,
	logger logging.Logger,
) *RRFEngine {
	if cfg == nil {
		cfg = DefaultRRFConfig()
	}
	if cfg.K <= 0 {
		cfg.K = DefaultRRFK
	}

	return &RRFEngine{
		bm25Engine:   bm25Engine,
		vectorEngine: vectorEngine,
		config:       cfg,
		logger:       logger.With(logging.F("component", "rrf_engine")),
	}
}

// HybridSearch performs a hybrid search combining BM25 and vector results.
// The search mode can be specified in filters, or determined from query hints.
func (e *RRFEngine) HybridSearch(
	ctx context.Context,
	tenantID string,
	query string,
	filters *HybridSearchFilters,
	limit int,
	offset int,
) (*HybridSearchResponse, error) {
	start := time.Now()

	// Validate tenant ID (critical for security)
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	// Initialize filters if nil
	if filters == nil {
		filters = &HybridSearchFilters{
			SearchMode: e.config.DefaultSearchMode,
		}
	}

	// Detect search mode from query hints if not explicitly set
	searchMode := e.detectSearchMode(query, filters.SearchMode)

	e.logger.Debug("Starting hybrid search",
		logging.F("tenant_id", tenantID),
		logging.F("query_length", len(query)),
		logging.F("search_mode", searchMode.String()),
		logging.F("limit", limit),
		logging.F("offset", offset),
	)

	var response *HybridSearchResponse
	var err error

	switch searchMode {
	case SearchModeKeyword:
		response, err = e.keywordOnlySearch(ctx, tenantID, query, filters, limit, offset)
	case SearchModeSemantic:
		response, err = e.semanticOnlySearch(ctx, tenantID, query, filters, limit, offset)
	default:
		response, err = e.hybridSearchInternal(ctx, tenantID, query, filters, limit, offset)
	}

	if err != nil {
		return nil, err
	}

	response.SearchMode = searchMode
	response.QueryTimeMs = float64(time.Since(start).Microseconds()) / 1000.0

	e.logger.Debug("Hybrid search completed",
		logging.F("results_count", len(response.Results)),
		logging.F("total_count", response.TotalCount),
		logging.F("query_time_ms", response.QueryTimeMs),
		logging.F("search_mode", searchMode.String()),
	)

	return response, nil
}

// detectSearchMode examines the query for hints about which search mode to use.
func (e *RRFEngine) detectSearchMode(query string, providedMode SearchMode) SearchMode {
	// If explicitly set to non-default, use that
	if providedMode != SearchModeHybrid {
		return providedMode
	}

	// TODO: Integrate with query parser to detect mode hints
	// For now, default to hybrid
	return e.config.DefaultSearchMode
}

// hybridSearchInternal performs the actual hybrid search with RRF fusion.
func (e *RRFEngine) hybridSearchInternal(
	ctx context.Context,
	tenantID string,
	query string,
	filters *HybridSearchFilters,
	limit int,
	offset int,
) (*HybridSearchResponse, error) {
	// Request more results from each engine than needed for better fusion
	fetchLimit := limit * 2
	if fetchLimit < 20 {
		fetchLimit = 20
	}

	var bm25Results *SearchResponse
	var vectorResults *VectorSearchResponse
	var bm25Err, vectorErr error
	var bm25TimeMs, vectorTimeMs float64

	// Convert filters for each engine
	bm25Filters := e.toBM25Filters(tenantID, filters)
	vectorFilters := e.toVectorFilters(filters)

	if e.config.ParallelExecution {
		// Execute searches in parallel
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			start := time.Now()
			bm25Results, bm25Err = e.bm25Engine.Search(ctx, query, bm25Filters, fetchLimit, 0)
			bm25TimeMs = float64(time.Since(start).Microseconds()) / 1000.0
		}()

		go func() {
			defer wg.Done()
			start := time.Now()
			vectorResults, vectorErr = e.vectorEngine.Search(ctx, tenantID, query, vectorFilters, fetchLimit, 0)
			vectorTimeMs = float64(time.Since(start).Microseconds()) / 1000.0
		}()

		wg.Wait()
	} else {
		// Execute sequentially
		start := time.Now()
		bm25Results, bm25Err = e.bm25Engine.Search(ctx, query, bm25Filters, fetchLimit, 0)
		bm25TimeMs = float64(time.Since(start).Microseconds()) / 1000.0

		start = time.Now()
		vectorResults, vectorErr = e.vectorEngine.Search(ctx, tenantID, query, vectorFilters, fetchLimit, 0)
		vectorTimeMs = float64(time.Since(start).Microseconds()) / 1000.0
	}

	// Handle errors with graceful degradation
	if !e.config.GracefulDegradation {
		if bm25Err != nil {
			return nil, fmt.Errorf("BM25 search failed: %w", bm25Err)
		}
		if vectorErr != nil {
			return nil, fmt.Errorf("vector search failed: %w", vectorErr)
		}
	}

	// Check if both searches failed
	if bm25Err != nil && vectorErr != nil {
		return nil, fmt.Errorf("both searches failed: BM25: %v, Vector: %v", bm25Err, vectorErr)
	}

	// Fuse results
	fusedResults := e.fuseResults(bm25Results, vectorResults, e.config.K)

	// Apply pagination
	start := offset
	end := offset + limit
	if start > len(fusedResults) {
		start = len(fusedResults)
	}
	if end > len(fusedResults) {
		end = len(fusedResults)
	}
	paginatedResults := fusedResults[start:end]

	// Estimate total count
	totalCount := e.estimateTotalCount(bm25Results, vectorResults)

	return &HybridSearchResponse{
		Results:      paginatedResults,
		TotalCount:   totalCount,
		BM25TimeMs:   bm25TimeMs,
		VectorTimeMs: vectorTimeMs,
		BM25Error:    bm25Err,
		VectorError:  vectorErr,
	}, nil
}

// keywordOnlySearch performs BM25-only search.
func (e *RRFEngine) keywordOnlySearch(
	ctx context.Context,
	tenantID string,
	query string,
	filters *HybridSearchFilters,
	limit int,
	offset int,
) (*HybridSearchResponse, error) {
	start := time.Now()

	bm25Filters := e.toBM25Filters(tenantID, filters)
	results, err := e.bm25Engine.Search(ctx, query, bm25Filters, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("BM25 search failed: %w", err)
	}

	bm25TimeMs := float64(time.Since(start).Microseconds()) / 1000.0

	// Convert BM25 results to hybrid format
	hybridResults := make([]HybridSearchResult, len(results.Results))
	for i, r := range results.Results {
		hybridResults[i] = HybridSearchResult{
			DocumentID:  r.DocumentID,
			ContentType: r.ContentType,
			SourceID:    r.SourceID,
			Title:       r.Title,
			Snippet:     r.Snippet,
			Highlights:  r.Highlights,
			Score:       r.Score,
			BM25Score:   r.Score,
			BM25Rank:    i + 1,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
			Metadata:    r.Metadata,
		}
	}

	return &HybridSearchResponse{
		Results:    hybridResults,
		TotalCount: results.TotalCount,
		BM25TimeMs: bm25TimeMs,
	}, nil
}

// semanticOnlySearch performs vector-only search.
func (e *RRFEngine) semanticOnlySearch(
	ctx context.Context,
	tenantID string,
	query string,
	filters *HybridSearchFilters,
	limit int,
	offset int,
) (*HybridSearchResponse, error) {
	start := time.Now()

	vectorFilters := e.toVectorFilters(filters)
	results, err := e.vectorEngine.Search(ctx, tenantID, query, vectorFilters, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	vectorTimeMs := float64(time.Since(start).Microseconds()) / 1000.0

	// Convert vector results to hybrid format
	hybridResults := make([]HybridSearchResult, len(results.Results))
	for i, r := range results.Results {
		hybridResults[i] = HybridSearchResult{
			DocumentID:  r.DocumentID,
			ContentType: r.ContentType,
			SourceID:    r.SourceID,
			Title:       r.Title,
			Snippet:     r.Snippet,
			Score:       r.SimilarityScore,
			VectorScore: r.SimilarityScore,
			VectorRank:  i + 1,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
			Metadata:    r.Metadata,
		}
	}

	return &HybridSearchResponse{
		Results:      hybridResults,
		TotalCount:   results.TotalCount,
		VectorTimeMs: vectorTimeMs,
	}, nil
}

// fuseResults combines BM25 and vector results using Reciprocal Rank Fusion.
// The RRF formula is: score = Σ 1/(k + rank)
func (e *RRFEngine) fuseResults(
	bm25Results *SearchResponse,
	vectorResults *VectorSearchResponse,
	k int,
) []HybridSearchResult {
	// Map to track documents and their fusion data
	docMap := make(map[string]*HybridSearchResult)

	// Process BM25 results
	if bm25Results != nil && len(bm25Results.Results) > 0 {
		for rank, r := range bm25Results.Results {
			rrfScore := e.config.BM25Weight * (1.0 / float64(k+rank+1))

			if existing, ok := docMap[r.DocumentID]; ok {
				existing.Score += rrfScore
				existing.BM25Score = r.Score
				existing.BM25Rank = rank + 1
				existing.Highlights = r.Highlights
				if existing.Snippet == "" {
					existing.Snippet = r.Snippet
				}
			} else {
				docMap[r.DocumentID] = &HybridSearchResult{
					DocumentID:  r.DocumentID,
					ContentType: r.ContentType,
					SourceID:    r.SourceID,
					Title:       r.Title,
					Snippet:     r.Snippet,
					Highlights:  r.Highlights,
					Score:       rrfScore,
					BM25Score:   r.Score,
					BM25Rank:    rank + 1,
					CreatedAt:   r.CreatedAt,
					UpdatedAt:   r.UpdatedAt,
					Metadata:    r.Metadata,
				}
			}
		}
	}

	// Process vector results
	if vectorResults != nil && len(vectorResults.Results) > 0 {
		for rank, r := range vectorResults.Results {
			rrfScore := e.config.VectorWeight * (1.0 / float64(k+rank+1))

			if existing, ok := docMap[r.DocumentID]; ok {
				existing.Score += rrfScore
				existing.VectorScore = r.SimilarityScore
				existing.VectorRank = rank + 1
				// Vector results might have better snippets for semantic queries
				if existing.Snippet == "" {
					existing.Snippet = r.Snippet
				}
			} else {
				docMap[r.DocumentID] = &HybridSearchResult{
					DocumentID:  r.DocumentID,
					ContentType: r.ContentType,
					SourceID:    r.SourceID,
					Title:       r.Title,
					Snippet:     r.Snippet,
					Score:       rrfScore,
					VectorScore: r.SimilarityScore,
					VectorRank:  rank + 1,
					CreatedAt:   r.CreatedAt,
					UpdatedAt:   r.UpdatedAt,
					Metadata:    r.Metadata,
				}
			}
		}
	}

	// Convert map to slice and sort by fused score
	results := make([]HybridSearchResult, 0, len(docMap))
	for _, result := range docMap {
		results = append(results, *result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Normalize scores to 0-1 range
	e.normalizeScores(results)

	return results
}

// normalizeScores normalizes the fused scores to 0-1 range.
func (e *RRFEngine) normalizeScores(results []HybridSearchResult) {
	if len(results) == 0 {
		return
	}

	maxScore := results[0].Score // Already sorted descending
	if maxScore <= 0 {
		return
	}

	for i := range results {
		results[i].Score = results[i].Score / maxScore
	}
}

// estimateTotalCount estimates the total number of matching documents.
func (e *RRFEngine) estimateTotalCount(
	bm25Results *SearchResponse,
	vectorResults *VectorSearchResponse,
) int64 {
	var bm25Count, vectorCount int64

	if bm25Results != nil {
		bm25Count = bm25Results.TotalCount
	}
	if vectorResults != nil {
		vectorCount = vectorResults.TotalCount
	}

	// Use the maximum as an upper bound estimate
	if bm25Count > vectorCount {
		return bm25Count
	}
	return vectorCount
}

// toBM25Filters converts HybridSearchFilters to BM25 SearchFilters.
func (e *RRFEngine) toBM25Filters(tenantID string, filters *HybridSearchFilters) SearchFilters {
	if filters == nil {
		return SearchFilters{TenantID: tenantID}
	}

	return SearchFilters{
		TenantID:     tenantID,
		ContentTypes: filters.ContentTypes,
		DateFrom:     filters.DateFrom,
		DateTo:       filters.DateTo,
		Tags:         filters.Tags,
		SourceIDs:    filters.SourceIDs,
		ExcludeIDs:   filters.ExcludeIDs,
	}
}

// toVectorFilters converts HybridSearchFilters to VectorSearchFilters.
func (e *RRFEngine) toVectorFilters(filters *HybridSearchFilters) *VectorSearchFilters {
	if filters == nil {
		return nil
	}

	return &VectorSearchFilters{
		ContentTypes: filters.ContentTypes,
		DateFrom:     filters.DateFrom,
		DateTo:       filters.DateTo,
		Tags:         filters.Tags,
		SourceIDs:    filters.SourceIDs,
		ExcludeIDs:   filters.ExcludeIDs,
	}
}

// SetConfig updates the RRF configuration.
func (e *RRFEngine) SetConfig(cfg *RRFConfig) {
	if cfg == nil {
		return
	}
	e.config = cfg
}

// GetConfig returns the current RRF configuration.
func (e *RRFEngine) GetConfig() *RRFConfig {
	return e.config
}

// SetWeights updates the BM25 and vector weights.
// Weights should sum to 1.0 for balanced results.
func (e *RRFEngine) SetWeights(bm25Weight, vectorWeight float64) error {
	if bm25Weight < 0 || bm25Weight > 1 {
		return errors.New("BM25 weight must be between 0 and 1")
	}
	if vectorWeight < 0 || vectorWeight > 1 {
		return errors.New("vector weight must be between 0 and 1")
	}

	e.config.BM25Weight = bm25Weight
	e.config.VectorWeight = vectorWeight
	return nil
}

// CalculateRRFScore calculates the RRF score for a document given its ranks.
// Returns 0 if the document wasn't found in either result set.
func CalculateRRFScore(bm25Rank, vectorRank, k int, bm25Weight, vectorWeight float64) float64 {
	var score float64

	if bm25Rank > 0 {
		score += bm25Weight * (1.0 / float64(k+bm25Rank))
	}
	if vectorRank > 0 {
		score += vectorWeight * (1.0 / float64(k+vectorRank))
	}

	return score
}
