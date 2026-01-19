// Package engine provides search engine implementations for the Search service.
// It includes vector similarity search using pgvector for semantic matching.
package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ErrEmbeddingServiceUnavailable indicates the embedding service cannot be reached.
var ErrEmbeddingServiceUnavailable = errors.New("embedding service unavailable")

// ErrEmptyQuery indicates an empty search query was provided.
var ErrEmptyQuery = errors.New("search query cannot be empty")

// ErrInvalidTenantID indicates an empty or invalid tenant ID was provided.
var ErrInvalidTenantID = errors.New("tenant_id is required")

// EmbeddingClient defines the interface for generating text embeddings.
// Implementations should connect to an embedding model (local or remote)
// and return vector representations of text.
type EmbeddingClient interface {
	// Embed generates a vector embedding for the given text.
	// Returns the embedding as a slice of float32 values.
	// Returns ErrEmbeddingServiceUnavailable if the service cannot be reached.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Dimensions returns the number of dimensions in the embedding vectors.
	Dimensions() int
}

// VectorSearchFilters contains optional filters for vector search queries.
type VectorSearchFilters struct {
	// ContentTypes filters by content type (e.g., "email", "meeting", "document").
	ContentTypes []string

	// SourceIDs filters by specific source identifiers.
	SourceIDs []string

	// DateFrom filters documents created on or after this time.
	DateFrom *time.Time

	// DateTo filters documents created on or before this time.
	DateTo *time.Time

	// Tags filters by document tags.
	Tags []string

	// EntityIDs filters by referenced entity identifiers.
	EntityIDs []string

	// Sources filters by source system (e.g., "gmail", "slack").
	Sources []string

	// ExcludeIDs excludes specific document IDs from results.
	ExcludeIDs []string
}

// VectorSearchResult represents a single search result with similarity score.
type VectorSearchResult struct {
	// DocumentID is the unique identifier of the matched document.
	DocumentID string

	// ContentType is the type of content (email, meeting, document, etc.).
	ContentType string

	// SourceID links to the original content source.
	SourceID string

	// Title is the document title or subject (if available).
	Title *string

	// Snippet is a text excerpt from the document content.
	Snippet string

	// SimilarityScore is the cosine similarity (0.0 to 1.0, higher is more similar).
	SimilarityScore float64

	// CreatedAt is when the document was originally created.
	CreatedAt time.Time

	// UpdatedAt is when the document was last updated.
	UpdatedAt time.Time

	// Metadata contains additional document metadata.
	Metadata map[string]string
}

// VectorSearchResponse contains the results of a vector search operation.
type VectorSearchResponse struct {
	// Results is the list of matching documents ordered by similarity.
	Results []VectorSearchResult

	// TotalCount is the total number of matching documents.
	TotalCount int64

	// QueryTimeMs is the time taken to execute the search in milliseconds.
	QueryTimeMs float64
}

// VectorEngineConfig holds configuration for the VectorEngine.
type VectorEngineConfig struct {
	// TableName is the name of the documents table with embeddings.
	TableName string

	// EmbeddingColumn is the column name for the vector embedding.
	EmbeddingColumn string

	// DefaultLimit is the default number of results to return.
	DefaultLimit int

	// MaxLimit is the maximum number of results allowed.
	MaxLimit int

	// MinSimilarity is the minimum similarity threshold for results.
	MinSimilarity float64

	// SnippetLength is the maximum length of content snippets.
	SnippetLength int
}

// DefaultVectorEngineConfig returns configuration with sensible defaults.
func DefaultVectorEngineConfig() *VectorEngineConfig {
	return &VectorEngineConfig{
		TableName:       "search_documents",
		EmbeddingColumn: "embedding",
		DefaultLimit:    10,
		MaxLimit:        100,
		MinSimilarity:   0.0,
		SnippetLength:   200,
	}
}

// VectorEngine performs vector similarity search using pgvector.
// It generates query embeddings via an EmbeddingClient and executes
// cosine similarity searches against PostgreSQL with pgvector extension.
type VectorEngine struct {
	db              *pgxpool.Pool
	embeddingClient EmbeddingClient
	config          *VectorEngineConfig
	logger          logging.Logger
}

// NewVectorEngine creates a new VectorEngine instance.
// The database pool must be connected to a PostgreSQL database with pgvector extension enabled.
func NewVectorEngine(db *pgxpool.Pool, embeddingClient EmbeddingClient, cfg *VectorEngineConfig, logger logging.Logger) *VectorEngine {
	if cfg == nil {
		cfg = DefaultVectorEngineConfig()
	}
	return &VectorEngine{
		db:              db,
		embeddingClient: embeddingClient,
		config:          cfg,
		logger:          logger.With(logging.F("component", "vector_engine")),
	}
}

// Search performs a vector similarity search for the given query.
// It generates an embedding for the query text and finds similar documents
// using pgvector's cosine distance operator (<=>).
//
// The search always filters by tenant_id first for security and performance.
// Additional filters can be applied via the filters parameter.
func (e *VectorEngine) Search(
	ctx context.Context,
	tenantID string,
	query string,
	filters *VectorSearchFilters,
	limit int,
	offset int,
) (*VectorSearchResponse, error) {
	start := time.Now()

	// Validate inputs
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrEmptyQuery
	}

	// Apply limit constraints
	if limit <= 0 {
		limit = e.config.DefaultLimit
	}
	if limit > e.config.MaxLimit {
		limit = e.config.MaxLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Generate embedding for the query
	embedding, err := e.embeddingClient.Embed(ctx, query)
	if err != nil {
		e.logger.Error("Failed to generate query embedding",
			logging.F("query_length", len(query)),
			logging.Err(err),
		)
		// Return a more specific error if embedding service is down
		if errors.Is(err, ErrEmbeddingServiceUnavailable) {
			return nil, ErrEmbeddingServiceUnavailable
		}
		return nil, fmt.Errorf("generating query embedding: %w", err)
	}

	e.logger.Debug("Generated query embedding",
		logging.F("query_length", len(query)),
		logging.F("embedding_dims", len(embedding)),
	)

	// Build and execute the search query
	results, totalCount, err := e.executeSearch(ctx, tenantID, embedding, filters, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("executing vector search: %w", err)
	}

	queryTimeMs := float64(time.Since(start).Microseconds()) / 1000.0

	e.logger.Debug("Vector search completed",
		logging.F("tenant_id", tenantID),
		logging.F("results", len(results)),
		logging.F("total_count", totalCount),
		logging.F("query_time_ms", queryTimeMs),
	)

	return &VectorSearchResponse{
		Results:     results,
		TotalCount:  totalCount,
		QueryTimeMs: queryTimeMs,
	}, nil
}

// executeSearch builds and executes the vector similarity SQL query.
func (e *VectorEngine) executeSearch(
	ctx context.Context,
	tenantID string,
	embedding []float32,
	filters *VectorSearchFilters,
	limit int,
	offset int,
) ([]VectorSearchResult, int64, error) {
	// Build the query with pgvector cosine similarity
	// Using <=> operator for cosine distance (1 - cosine similarity)
	// We convert to similarity by using (1 - distance)
	query, args := e.buildSearchQuery(tenantID, embedding, filters, limit, offset)

	e.logger.Debug("Executing vector search query",
		logging.F("tenant_id", tenantID),
		logging.F("limit", limit),
		logging.F("offset", offset),
	)

	// Execute the main search query
	rows, err := e.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	var results []VectorSearchResult
	for rows.Next() {
		var result VectorSearchResult
		var title *string
		var metadata map[string]string
		var cosineDistance float64

		err := rows.Scan(
			&result.DocumentID,
			&result.ContentType,
			&result.SourceID,
			&title,
			&result.Snippet,
			&cosineDistance,
			&result.CreatedAt,
			&result.UpdatedAt,
			&metadata,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning result row: %w", err)
		}

		result.Title = title
		result.Metadata = metadata
		// Convert cosine distance to similarity (1 - distance)
		result.SimilarityScore = 1.0 - cosineDistance

		// Apply minimum similarity filter
		if result.SimilarityScore < e.config.MinSimilarity {
			continue
		}

		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating results: %w", err)
	}

	// Get total count
	totalCount, err := e.getMatchCount(ctx, tenantID, embedding, filters)
	if err != nil {
		e.logger.Warn("Failed to get total count, returning result count",
			logging.Err(err),
		)
		totalCount = int64(len(results))
	}

	return results, totalCount, nil
}

// buildSearchQuery constructs the SQL query for vector similarity search.
func (e *VectorEngine) buildSearchQuery(
	tenantID string,
	embedding []float32,
	filters *VectorSearchFilters,
	limit int,
	offset int,
) (string, []interface{}) {
	// Start building the query
	// CRITICAL: Always filter by tenant_id first for security and index usage
	var sb strings.Builder
	args := make([]interface{}, 0, 10)
	argNum := 1

	sb.WriteString(fmt.Sprintf(`
SELECT
    document_id,
    content_type,
    source_id,
    title,
    LEFT(content, %d) as snippet,
    %s <=> $%d::vector as cosine_distance,
    created_at,
    updated_at,
    metadata
FROM %s
WHERE tenant_id = $%d`,
		e.config.SnippetLength,
		e.config.EmbeddingColumn,
		argNum,
		e.config.TableName,
		argNum+1,
	))
	args = append(args, pgVectorFormat(embedding), tenantID)
	argNum += 2

	// Apply optional filters
	if filters != nil {
		if len(filters.ContentTypes) > 0 {
			sb.WriteString(fmt.Sprintf(" AND content_type = ANY($%d)", argNum))
			args = append(args, filters.ContentTypes)
			argNum++
		}

		if len(filters.SourceIDs) > 0 {
			sb.WriteString(fmt.Sprintf(" AND source_id = ANY($%d)", argNum))
			args = append(args, filters.SourceIDs)
			argNum++
		}

		if filters.DateFrom != nil {
			sb.WriteString(fmt.Sprintf(" AND created_at >= $%d", argNum))
			args = append(args, *filters.DateFrom)
			argNum++
		}

		if filters.DateTo != nil {
			sb.WriteString(fmt.Sprintf(" AND created_at <= $%d", argNum))
			args = append(args, *filters.DateTo)
			argNum++
		}

		if len(filters.Tags) > 0 {
			sb.WriteString(fmt.Sprintf(" AND tags && $%d", argNum))
			args = append(args, filters.Tags)
			argNum++
		}

		if len(filters.EntityIDs) > 0 {
			sb.WriteString(fmt.Sprintf(" AND entity_ids && $%d", argNum))
			args = append(args, filters.EntityIDs)
			argNum++
		}

		if len(filters.Sources) > 0 {
			sb.WriteString(fmt.Sprintf(" AND source = ANY($%d)", argNum))
			args = append(args, filters.Sources)
			argNum++
		}

		if len(filters.ExcludeIDs) > 0 {
			sb.WriteString(fmt.Sprintf(" AND document_id != ALL($%d)", argNum))
			args = append(args, filters.ExcludeIDs)
			argNum++
		}
	}

	// Order by cosine distance (ascending = most similar first)
	sb.WriteString(" ORDER BY cosine_distance ASC")

	// Apply pagination
	sb.WriteString(fmt.Sprintf(" LIMIT $%d OFFSET $%d", argNum, argNum+1))
	args = append(args, limit, offset)

	return sb.String(), args
}

// getMatchCount returns the total count of matching documents.
func (e *VectorEngine) getMatchCount(
	ctx context.Context,
	tenantID string,
	embedding []float32,
	filters *VectorSearchFilters,
) (int64, error) {
	var sb strings.Builder
	args := make([]interface{}, 0, 8)
	argNum := 1

	sb.WriteString(fmt.Sprintf(`
SELECT COUNT(*)
FROM %s
WHERE tenant_id = $%d`,
		e.config.TableName,
		argNum,
	))
	args = append(args, tenantID)
	argNum++

	// Apply minimum similarity filter if configured
	if e.config.MinSimilarity > 0 {
		sb.WriteString(fmt.Sprintf(" AND (1.0 - (%s <=> $%d::vector)) >= $%d",
			e.config.EmbeddingColumn, argNum, argNum+1))
		args = append(args, pgVectorFormat(embedding), e.config.MinSimilarity)
		argNum += 2
	}

	// Apply optional filters
	if filters != nil {
		if len(filters.ContentTypes) > 0 {
			sb.WriteString(fmt.Sprintf(" AND content_type = ANY($%d)", argNum))
			args = append(args, filters.ContentTypes)
			argNum++
		}

		if len(filters.SourceIDs) > 0 {
			sb.WriteString(fmt.Sprintf(" AND source_id = ANY($%d)", argNum))
			args = append(args, filters.SourceIDs)
			argNum++
		}

		if filters.DateFrom != nil {
			sb.WriteString(fmt.Sprintf(" AND created_at >= $%d", argNum))
			args = append(args, *filters.DateFrom)
			argNum++
		}

		if filters.DateTo != nil {
			sb.WriteString(fmt.Sprintf(" AND created_at <= $%d", argNum))
			args = append(args, *filters.DateTo)
			argNum++
		}

		if len(filters.Tags) > 0 {
			sb.WriteString(fmt.Sprintf(" AND tags && $%d", argNum))
			args = append(args, filters.Tags)
			argNum++
		}

		if len(filters.EntityIDs) > 0 {
			sb.WriteString(fmt.Sprintf(" AND entity_ids && $%d", argNum))
			args = append(args, filters.EntityIDs)
			argNum++
		}

		if len(filters.Sources) > 0 {
			sb.WriteString(fmt.Sprintf(" AND source = ANY($%d)", argNum))
			args = append(args, filters.Sources)
			argNum++
		}

		if len(filters.ExcludeIDs) > 0 {
			sb.WriteString(fmt.Sprintf(" AND document_id != ALL($%d)", argNum))
			args = append(args, filters.ExcludeIDs)
			argNum++
		}
	}

	var count int64
	err := e.db.QueryRow(ctx, sb.String(), args...).Scan(&count)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("count query failed: %w", err)
	}

	return count, nil
}

// SearchByVector performs a vector similarity search using a pre-computed embedding.
// This is useful when you already have an embedding and want to find similar documents.
func (e *VectorEngine) SearchByVector(
	ctx context.Context,
	tenantID string,
	embedding []float32,
	filters *VectorSearchFilters,
	limit int,
	offset int,
) (*VectorSearchResponse, error) {
	start := time.Now()

	// Validate inputs
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}
	if len(embedding) == 0 {
		return nil, errors.New("embedding cannot be empty")
	}
	if len(embedding) != e.embeddingClient.Dimensions() {
		return nil, fmt.Errorf("embedding dimension mismatch: got %d, expected %d",
			len(embedding), e.embeddingClient.Dimensions())
	}

	// Apply limit constraints
	if limit <= 0 {
		limit = e.config.DefaultLimit
	}
	if limit > e.config.MaxLimit {
		limit = e.config.MaxLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Execute the search
	results, totalCount, err := e.executeSearch(ctx, tenantID, embedding, filters, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("executing vector search: %w", err)
	}

	queryTimeMs := float64(time.Since(start).Microseconds()) / 1000.0

	return &VectorSearchResponse{
		Results:     results,
		TotalCount:  totalCount,
		QueryTimeMs: queryTimeMs,
	}, nil
}

// pgVectorFormat converts a float32 slice to pgvector format string.
// pgvector expects vectors in the format '[1.0,2.0,3.0]'.
func pgVectorFormat(embedding []float32) string {
	if len(embedding) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[")
	for i, v := range embedding {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%f", v))
	}
	sb.WriteString("]")
	return sb.String()
}
