// Package searchservice implements the SearchService gRPC for the Gateway.
// It queries PostgreSQL directly using full-text search, without requiring
// a separate search service backend.
package searchservice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the SearchServiceServer using direct database queries.
type Service struct {
	searchv1.UnimplementedSearchServiceServer

	db     *pgxpool.Pool
	logger logging.Logger
}

// NewService creates a new search service with database access.
func NewService(db *pgxpool.Pool, logger logging.Logger) *Service {
	if logger == nil {
		logger = logging.NewLogger(logging.DefaultConfig())
	}
	return &Service{
		db:     db,
		logger: logger,
	}
}

// Search performs a hybrid search combining full-text and basic relevance scoring.
func (s *Service) Search(ctx context.Context, req *searchv1.SearchRequest) (*searchv1.SearchResponse, error) {
	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not configured")
	}

	query := strings.TrimSpace(req.GetQuery())
	if query == "" {
		return nil, status.Error(codes.InvalidArgument, "query cannot be empty")
	}

	tenantID := req.GetTenantId()
	limit := req.GetLimit()
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.GetOffset()
	if offset < 0 {
		offset = 0
	}

	s.logger.Debug("Search request",
		logging.F("tenant_id", tenantID),
		logging.F("query", query),
		logging.F("limit", limit),
		logging.F("offset", offset),
	)

	startTime := time.Now()

	// Query sources table with full-text search
	// Using plainto_tsquery for simple queries, ts_rank for relevance
	rows, err := s.db.Query(ctx, `
		SELECT
			s.id,
			s.external_id,
			s.source_system,
			COALESCE(m.title, s.content_type, 'Untitled') as title,
			LEFT(s.raw_content, 500) as snippet,
			ts_rank(to_tsvector('english', COALESCE(s.raw_content, '')), plainto_tsquery('english', $1)) as score,
			s.content_type,
			s.source_timestamp,
			s.created_at
		FROM sources s
		LEFT JOIN meetings m ON s.meeting_id = m.id
		WHERE s.tenant_id = $2
			AND to_tsvector('english', COALESCE(s.raw_content, '')) @@ plainto_tsquery('english', $1)
		ORDER BY score DESC, s.created_at DESC
		LIMIT $3 OFFSET $4
	`, query, tenantID, limit, offset)
	if err != nil {
		s.logger.Error("Search query failed", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "search query failed: %v", err)
	}
	defer rows.Close()

	var results []*searchv1.SearchResult
	for rows.Next() {
		var (
			id              int64
			externalID      string
			sourceSystem    string
			title           string
			snippet         string
			score           float32
			contentType     *string
			sourceTimestamp *time.Time
			createdAt       time.Time
		)

		if err := rows.Scan(&id, &externalID, &sourceSystem, &title, &snippet, &score, &contentType, &sourceTimestamp, &createdAt); err != nil {
			s.logger.Error("Failed to scan search result", logging.Err(err))
			continue
		}

		result := &searchv1.SearchResult{
			DocumentId:  fmt.Sprintf("%d", id),
			SourceId:    externalID,
			ContentType: derefString(contentType, sourceSystem),
			Title:       &title,
			Snippet:     highlightSnippet(snippet, query),
			Score:       score,
			TextScore:   &score,
			CreatedAt:   timestamppb.New(createdAt),
			Metadata:    map[string]string{"source_system": sourceSystem},
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("Error iterating search results", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "error reading results: %v", err)
	}

	// Get total count
	var totalCount int64
	err = s.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM sources
		WHERE tenant_id = $1
			AND to_tsvector('english', COALESCE(raw_content, '')) @@ plainto_tsquery('english', $2)
	`, tenantID, query).Scan(&totalCount)
	if err != nil {
		s.logger.Warn("Failed to get total count", logging.Err(err))
		totalCount = int64(len(results))
	}

	queryTimeMs := float64(time.Since(startTime).Microseconds()) / 1000.0

	s.logger.Debug("Search completed",
		logging.F("total_count", totalCount),
		logging.F("result_count", len(results)),
		logging.F("query_time_ms", queryTimeMs),
	)

	return &searchv1.SearchResponse{
		Results:     results,
		TotalCount:  totalCount,
		QueryTimeMs: queryTimeMs,
	}, nil
}

// SemanticSearch performs semantic search (falls back to keyword search for now).
func (s *Service) SemanticSearch(ctx context.Context, req *searchv1.SemanticSearchRequest) (*searchv1.SearchResponse, error) {
	// For now, fall back to keyword search
	// TODO: Implement vector search with embeddings when available
	s.logger.Debug("SemanticSearch falling back to keyword search")
	return s.Search(ctx, &searchv1.SearchRequest{
		Query:    req.GetQuery(),
		TenantId: req.GetTenantId(),
		Limit:    req.GetLimit(),
		Offset:   req.GetOffset(),
		Filters:  req.GetFilters(),
	})
}

// KeywordSearch performs keyword-only search.
func (s *Service) KeywordSearch(ctx context.Context, req *searchv1.KeywordSearchRequest) (*searchv1.SearchResponse, error) {
	return s.Search(ctx, &searchv1.SearchRequest{
		Query:    req.GetQuery(),
		TenantId: req.GetTenantId(),
		Limit:    req.GetLimit(),
		Offset:   req.GetOffset(),
		Filters:  req.GetFilters(),
	})
}

// IndexDocument adds or updates a document (not implemented - handled by ingest pipeline).
func (s *Service) IndexDocument(ctx context.Context, req *searchv1.IndexDocumentRequest) (*searchv1.IndexDocumentResponse, error) {
	return nil, status.Error(codes.Unimplemented, "indexing is handled by the ingest pipeline")
}

// DeleteDocument removes a document (not implemented).
func (s *Service) DeleteDocument(ctx context.Context, req *searchv1.DeleteDocumentRequest) (*searchv1.DeleteDocumentResponse, error) {
	return nil, status.Error(codes.Unimplemented, "delete is not supported")
}

// GetSearchStats returns search index statistics.
func (s *Service) GetSearchStats(ctx context.Context, req *searchv1.GetSearchStatsRequest) (*searchv1.GetSearchStatsResponse, error) {
	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not configured")
	}

	tenantID := req.GetTenantId()

	var totalDocs int64
	var query string
	var args []interface{}

	if tenantID != "" {
		query = "SELECT COUNT(*) FROM sources WHERE tenant_id = $1"
		args = []interface{}{tenantID}
	} else {
		query = "SELECT COUNT(*) FROM sources"
	}

	if err := s.db.QueryRow(ctx, query, args...).Scan(&totalDocs); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get stats: %v", err)
	}

	return &searchv1.GetSearchStatsResponse{
		TotalDocuments: totalDocs,
		LastIndexedAt:  timestamppb.Now(),
	}, nil
}

// highlightSnippet adds basic highlighting to search results.
func highlightSnippet(snippet, query string) string {
	// Simple highlighting - wrap query terms in <em> tags
	words := strings.Fields(strings.ToLower(query))
	result := snippet
	for _, word := range words {
		if len(word) > 2 { // Only highlight words longer than 2 chars
			result = strings.ReplaceAll(result, word, "<em>"+word+"</em>")
			// Also try capitalized version
			capitalized := strings.Title(word)
			result = strings.ReplaceAll(result, capitalized, "<em>"+capitalized+"</em>")
		}
	}
	return result
}

// derefString returns the dereferenced string or a default value.
func derefString(s *string, defaultVal string) string {
	if s != nil && *s != "" {
		return *s
	}
	return defaultVal
}
