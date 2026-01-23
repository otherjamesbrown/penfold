// Package engine provides search engine implementations for the Search service.
// It includes BM25 full-text search using PostgreSQL ts_vector and ts_query.
package engine

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// FieldWeights defines configurable weights for different document fields.
type FieldWeights struct {
	Title float64 // Weight for title field (default: 1.0)
	Body  float64 // Weight for body/content field (default: 0.5)
}

// DefaultFieldWeights returns the default field weights.
func DefaultFieldWeights() FieldWeights {
	return FieldWeights{
		Title: 1.0,
		Body:  0.5,
	}
}

// SearchFilters defines the filter criteria for BM25 search.
type SearchFilters struct {
	// ContentTypes filters by document type (email, document, meeting, etc.)
	ContentTypes []string

	// DateFrom is the inclusive start date for filtering
	DateFrom *time.Time

	// DateTo is the inclusive end date for filtering
	DateTo *time.Time

	// TenantID is CRITICAL for multi-tenant isolation
	TenantID string

	// ExcludeIDs excludes specific document IDs from results
	ExcludeIDs []string

	// Tags filters by document tags
	Tags []string

	// SourceIDs filters by source identifiers
	SourceIDs []string
}

// SearchResult represents a single search result from BM25 search.
type SearchResult struct {
	DocumentID  string            // Unique document identifier
	ContentType string            // Type of content (email, document, etc.)
	SourceID    string            // Source identifier
	Title       *string           // Document title (optional)
	Snippet     string            // Text snippet with context
	Highlights  []string          // Highlighted portions with <em> tags
	Score       float64           // Normalized score (0.0 to 1.0)
	RawScore    float64           // Raw ts_rank score
	CreatedAt   time.Time         // Document creation timestamp
	UpdatedAt   time.Time         // Document update timestamp
	Metadata    map[string]string // Additional metadata
}

// SearchResponse wraps the search results with metadata.
type SearchResponse struct {
	Results     []SearchResult // Search results ordered by relevance
	TotalCount  int64          // Total matching documents
	QueryTimeMs float64        // Time taken to execute the query
}

// MentionExpander provides mention-based search expansion.
type MentionExpander interface {
	// ExpandSearchTerms returns content IDs that have mentions resolved to entities matching the search term.
	ExpandSearchTerms(ctx context.Context, tenantID string, searchTerms []string) ([]string, error)
}

// BM25Engine implements PostgreSQL full-text search using ts_vector and ts_rank.
type BM25Engine struct {
	pool            *pgxpool.Pool
	logger          logging.Logger
	weights         FieldWeights
	mentionExpander MentionExpander
}

// BM25Config holds configuration for the BM25 engine.
type BM25Config struct {
	Weights FieldWeights
}

// NewBM25Engine creates a new BM25 search engine instance.
func NewBM25Engine(pool *pgxpool.Pool, logger logging.Logger, cfg *BM25Config) *BM25Engine {
	weights := DefaultFieldWeights()
	if cfg != nil {
		weights = cfg.Weights
	}

	return &BM25Engine{
		pool:    pool,
		logger:  logger.With(logging.F("component", "bm25_engine")),
		weights: weights,
	}
}

// Search performs a full-text search using PostgreSQL ts_vector and ts_rank.
// It preprocesses the query, applies filters (including critical tenant isolation),
// and returns scored results with highlighted snippets.
func (e *BM25Engine) Search(ctx context.Context, query string, filters SearchFilters, limit, offset int) (*SearchResponse, error) {
	start := time.Now()

	// Validate tenant isolation - this is CRITICAL
	if filters.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required for search (security: tenant isolation)")
	}

	// Preprocess the query
	processedQuery := e.preprocessQuery(query)
	if processedQuery == "" {
		return &SearchResponse{
			Results:     []SearchResult{},
			TotalCount:  0,
			QueryTimeMs: float64(time.Since(start).Milliseconds()),
		}, nil
	}

	// Build the SQL query with filters
	sql, args := e.buildSearchQuery(processedQuery, filters, limit, offset)

	e.logger.Debug("Executing BM25 search",
		logging.F("query", query),
		logging.F("processed_query", processedQuery),
		logging.F("tenant_id", filters.TenantID),
		logging.F("limit", limit),
		logging.F("offset", offset),
	)

	// Execute the search query
	rows, err := e.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("executing search query: %w", err)
	}
	defer rows.Close()

	results, maxScore, totalCount, err := e.scanResults(rows, processedQuery)
	if err != nil {
		return nil, fmt.Errorf("scanning results: %w", err)
	}

	// Normalize scores to 0-1 range
	e.normalizeScores(results, maxScore)

	queryTimeMs := float64(time.Since(start).Milliseconds())

	e.logger.Debug("BM25 search completed",
		logging.F("results_count", len(results)),
		logging.F("total_count", totalCount),
		logging.F("query_time_ms", queryTimeMs),
	)

	return &SearchResponse{
		Results:     results,
		TotalCount:  totalCount,
		QueryTimeMs: queryTimeMs,
	}, nil
}

// preprocessQuery converts a user query into a PostgreSQL tsquery format.
// It handles tokenization, stemming (via PostgreSQL), and stop word handling.
func (e *BM25Engine) preprocessQuery(query string) string {
	// Trim whitespace
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}

	// Convert to lowercase for consistent processing
	query = strings.ToLower(query)

	// Remove special characters that might break tsquery, keeping alphanumeric and spaces
	re := regexp.MustCompile(`[^a-z0-9\s\-_]`)
	query = re.ReplaceAllString(query, " ")

	// Split into words and filter out empty strings
	words := strings.Fields(query)
	if len(words) == 0 {
		return ""
	}

	// Apply query expansion for common terms
	words = e.expandQuery(words)

	// Build tsquery with AND operator for better precision
	// PostgreSQL will handle stemming via the english configuration
	tsqueryParts := make([]string, 0, len(words))
	for _, word := range words {
		if len(word) >= 2 { // Skip single-character words
			// Use prefix matching for partial word support
			tsqueryParts = append(tsqueryParts, word+":*")
		}
	}

	if len(tsqueryParts) == 0 {
		return ""
	}

	// Join with & (AND) for more precise results
	return strings.Join(tsqueryParts, " & ")
}

// expandQuery performs query expansion for common terms.
// This improves recall by adding synonyms and related terms.
func (e *BM25Engine) expandQuery(words []string) []string {
	// Common expansions map
	expansions := map[string][]string{
		"email":    {"mail", "message"},
		"mail":     {"email", "message"},
		"doc":      {"document", "file"},
		"document": {"doc", "file"},
		"meeting":  {"call", "session"},
		"msg":      {"message"},
	}

	result := make([]string, 0, len(words)*2)
	seen := make(map[string]bool)

	for _, word := range words {
		if !seen[word] {
			result = append(result, word)
			seen[word] = true
		}

		// Add expansions (limited to avoid query explosion)
		if expanded, ok := expansions[word]; ok {
			for _, exp := range expanded {
				if !seen[exp] {
					result = append(result, exp)
					seen[exp] = true
				}
			}
		}
	}

	return result
}

// buildSearchQuery constructs the PostgreSQL search query with filters.
// Uses COUNT(*) OVER() window function to get total count in a single query.
func (e *BM25Engine) buildSearchQuery(processedQuery string, filters SearchFilters, limit, offset int) (string, []interface{}) {
	args := make([]interface{}, 0, 10)
	argIndex := 1

	// Build the weighted ts_rank expression
	// PostgreSQL ts_rank weights: {D, C, B, A} where A is highest
	// We map title to 'A' weight and body to 'D' weight
	weightArray := fmt.Sprintf("'{%.2f, 0.4, 0.2, %.2f}'", e.weights.Body, e.weights.Title)

	sql := fmt.Sprintf(`
		SELECT
			d.id,
			d.content_type,
			d.source_id,
			d.title,
			ts_headline('english', d.content, to_tsquery('english', $%d),
				'StartSel=<em>, StopSel=</em>, MaxWords=35, MinWords=15, MaxFragments=3') as snippet,
			ts_headline('english', COALESCE(d.title, '') || ' ' || d.content, to_tsquery('english', $%d),
				'StartSel=<em>, StopSel=</em>, MaxWords=20, MinWords=5, MaxFragments=5') as highlights,
			ts_rank(%s::float4[],
				setweight(to_tsvector('english', COALESCE(d.title, '')), 'A') ||
				setweight(to_tsvector('english', d.content), 'D'),
				to_tsquery('english', $%d)
			) as score,
			d.created_at,
			d.updated_at,
			d.metadata,
			COUNT(*) OVER() as total_count
		FROM documents d
		WHERE d.tenant_id = $%d
		AND (
			setweight(to_tsvector('english', COALESCE(d.title, '')), 'A') ||
			setweight(to_tsvector('english', d.content), 'D')
		) @@ to_tsquery('english', $%d)
	`, argIndex, argIndex, weightArray, argIndex, argIndex+1, argIndex)

	args = append(args, processedQuery)
	argIndex++
	args = append(args, filters.TenantID)
	argIndex++

	// Add content type filter
	if len(filters.ContentTypes) > 0 {
		sql += fmt.Sprintf(" AND d.content_type = ANY($%d)", argIndex)
		args = append(args, filters.ContentTypes)
		argIndex++
	}

	// Add date range filters
	if filters.DateFrom != nil {
		sql += fmt.Sprintf(" AND d.created_at >= $%d", argIndex)
		args = append(args, *filters.DateFrom)
		argIndex++
	}
	if filters.DateTo != nil {
		sql += fmt.Sprintf(" AND d.created_at <= $%d", argIndex)
		args = append(args, *filters.DateTo)
		argIndex++
	}

	// Add exclude IDs filter
	if len(filters.ExcludeIDs) > 0 {
		sql += fmt.Sprintf(" AND d.id != ALL($%d)", argIndex)
		args = append(args, filters.ExcludeIDs)
		argIndex++
	}

	// Add tags filter
	if len(filters.Tags) > 0 {
		sql += fmt.Sprintf(" AND d.tags && $%d", argIndex)
		args = append(args, filters.Tags)
		argIndex++
	}

	// Add source IDs filter
	if len(filters.SourceIDs) > 0 {
		sql += fmt.Sprintf(" AND d.source_id = ANY($%d)", argIndex)
		args = append(args, filters.SourceIDs)
		argIndex++
	}

	// Order by score (highest first) and apply pagination
	sql += fmt.Sprintf(`
		ORDER BY score DESC, d.created_at DESC
		LIMIT $%d OFFSET $%d
	`, argIndex, argIndex+1)
	args = append(args, limit, offset)

	return sql, args
}

// scanResults reads the query results and extracts search results.
// Returns results, max score for normalization, and total count from window function.
func (e *BM25Engine) scanResults(rows pgx.Rows, processedQuery string) ([]SearchResult, float64, int64, error) {
	results := make([]SearchResult, 0)
	var maxScore float64
	var totalCount int64

	for rows.Next() {
		var (
			id          string
			contentType string
			sourceID    string
			title       *string
			snippet     string
			highlights  string
			score       float64
			createdAt   time.Time
			updatedAt   time.Time
			metadata    map[string]string
			rowTotal    int64
		)

		err := rows.Scan(&id, &contentType, &sourceID, &title, &snippet, &highlights, &score, &createdAt, &updatedAt, &metadata, &rowTotal)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("scanning row: %w", err)
		}

		// Capture total count from window function (same on all rows)
		totalCount = rowTotal

		// Track max score for normalization
		if score > maxScore {
			maxScore = score
		}

		// Parse highlights into array (split by newline or sentence boundary)
		highlightList := e.parseHighlights(highlights)

		results = append(results, SearchResult{
			DocumentID:  id,
			ContentType: contentType,
			SourceID:    sourceID,
			Title:       title,
			Snippet:     snippet,
			Highlights:  highlightList,
			RawScore:    score,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
			Metadata:    metadata,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, 0, 0, fmt.Errorf("iterating rows: %w", err)
	}

	return results, maxScore, totalCount, nil
}

// parseHighlights splits the highlight string into individual highlighted fragments.
func (e *BM25Engine) parseHighlights(highlights string) []string {
	// Split on common fragment delimiters
	parts := strings.Split(highlights, " ... ")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && strings.Contains(part, "<em>") {
			result = append(result, part)
		}
	}

	return result
}

// normalizeScores normalizes all scores to the 0-1 range.
func (e *BM25Engine) normalizeScores(results []SearchResult, maxScore float64) {
	if maxScore <= 0 {
		// No valid scores, set all to 0
		for i := range results {
			results[i].Score = 0
		}
		return
	}

	for i := range results {
		// Normalize to 0-1 range
		normalized := results[i].RawScore / maxScore

		// Apply sigmoid-like transformation for better score distribution
		// This prevents most scores from clustering near 1.0
		normalized = 1 / (1 + math.Exp(-5*(normalized-0.5)))

		results[i].Score = normalized
	}
}

// SetWeights updates the field weights used for scoring.
func (e *BM25Engine) SetWeights(weights FieldWeights) {
	e.weights = weights
}

// GetWeights returns the current field weights.
func (e *BM25Engine) GetWeights() FieldWeights {
	return e.weights
}

// SetMentionExpander sets the mention expander for search expansion.
func (e *BM25Engine) SetMentionExpander(expander MentionExpander) {
	e.mentionExpander = expander
}

// SearchWithMentions performs a full-text search that also includes content
// where mentions were resolved to entities matching the search terms.
func (e *BM25Engine) SearchWithMentions(ctx context.Context, query string, filters SearchFilters, limit, offset int) (*SearchResponse, error) {
	// First, get standard BM25 results
	response, err := e.Search(ctx, query, filters, limit, offset)
	if err != nil {
		return nil, err
	}

	// If no mention expander, return standard results
	if e.mentionExpander == nil {
		return response, nil
	}

	// Extract search terms from query
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return response, nil
	}

	// Get content IDs from mention expansion
	mentionContentIDs, err := e.mentionExpander.ExpandSearchTerms(ctx, filters.TenantID, words)
	if err != nil {
		e.logger.Warn("Failed to expand search via mentions", logging.Err(err))
		// Continue with standard results
		return response, nil
	}

	if len(mentionContentIDs) == 0 {
		return response, nil
	}

	// Get documents by mention-expanded content IDs
	mentionResults, err := e.getDocumentsBySourceIDs(ctx, filters.TenantID, mentionContentIDs, limit)
	if err != nil {
		e.logger.Warn("Failed to get mention-expanded documents", logging.Err(err))
		return response, nil
	}

	// Merge results, avoiding duplicates
	existingIDs := make(map[string]bool)
	for _, r := range response.Results {
		existingIDs[r.DocumentID] = true
	}

	for _, r := range mentionResults {
		if !existingIDs[r.DocumentID] {
			// Mark mention-expanded results with lower score
			r.Score = r.Score * 0.8 // Slightly lower priority than direct matches
			response.Results = append(response.Results, r)
			existingIDs[r.DocumentID] = true
		}
	}

	// Update total count to include mention expansions
	response.TotalCount += int64(len(mentionResults))

	return response, nil
}

// getDocumentsBySourceIDs retrieves documents by their source IDs.
func (e *BM25Engine) getDocumentsBySourceIDs(ctx context.Context, tenantID string, sourceIDs []string, limit int) ([]SearchResult, error) {
	if len(sourceIDs) == 0 {
		return nil, nil
	}

	sql := `
		SELECT
			d.id,
			d.content_type,
			d.source_id,
			d.title,
			LEFT(d.content, 200) as snippet,
			'' as highlights,
			0.5 as score,
			d.created_at,
			d.updated_at,
			d.metadata
		FROM documents d
		WHERE d.tenant_id = $1
		AND d.source_id = ANY($2)
		LIMIT $3
	`

	rows, err := e.pool.Query(ctx, sql, tenantID, sourceIDs, limit)
	if err != nil {
		return nil, fmt.Errorf("querying documents by source IDs: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var (
			id          string
			contentType string
			sourceID    string
			title       *string
			snippet     string
			highlights  string
			score       float64
			createdAt   time.Time
			updatedAt   time.Time
			metadata    map[string]string
		)

		err := rows.Scan(&id, &contentType, &sourceID, &title, &snippet, &highlights, &score, &createdAt, &updatedAt, &metadata)
		if err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		results = append(results, SearchResult{
			DocumentID:  id,
			ContentType: contentType,
			SourceID:    sourceID,
			Title:       title,
			Snippet:     snippet + "...",
			Score:       score,
			RawScore:    score,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
			Metadata:    metadata,
		})
	}

	return results, nil
}
