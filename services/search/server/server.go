// Package server implements the gRPC SearchService.
// It provides hybrid search functionality combining full-text and vector similarity search.
package server

import (
	"context"

	searchv1 "github.com/otherjamesbrown/penfold/api/proto/searchv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/search/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SearchServer implements the SearchService gRPC interface.
type SearchServer struct {
	searchv1.UnimplementedSearchServiceServer

	cfg     *config.Config
	logger  logging.Logger
	metrics *metrics.Metrics
}

// NewSearchServer creates a new SearchServer instance.
func NewSearchServer(cfg *config.Config, logger logging.Logger, m *metrics.Metrics) *SearchServer {
	return &SearchServer{
		cfg:     cfg,
		logger:  logger.With(logging.F("component", "search_server")),
		metrics: m,
	}
}

// Search performs a hybrid search combining full-text and vector similarity.
// This is the recommended search method for most use cases.
func (s *SearchServer) Search(ctx context.Context, req *searchv1.SearchRequest) (*searchv1.SearchResponse, error) {
	s.logger.Debug("Search request received",
		logging.F("query", req.GetQuery()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("limit", req.GetLimit()),
	)

	// TODO: Implement hybrid search
	// 1. Generate embedding for the query
	// 2. Execute full-text search against PostgreSQL
	// 3. Execute vector search against vector database
	// 4. Combine and rank results using configured weights
	// 5. Return merged results

	return nil, status.Error(codes.Unimplemented, "Search not yet implemented")
}

// SemanticSearch performs vector-only similarity search.
// Best for finding conceptually related content regardless of exact keywords.
func (s *SearchServer) SemanticSearch(ctx context.Context, req *searchv1.SemanticSearchRequest) (*searchv1.SearchResponse, error) {
	s.logger.Debug("SemanticSearch request received",
		logging.F("query", req.GetQuery()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("limit", req.GetLimit()),
	)

	// TODO: Implement semantic search
	// 1. Generate embedding for the query
	// 2. Execute vector similarity search
	// 3. Apply filters and return results

	return nil, status.Error(codes.Unimplemented, "SemanticSearch not yet implemented")
}

// KeywordSearch performs full-text only search.
// Best for exact phrase matching or when semantic similarity is not needed.
func (s *SearchServer) KeywordSearch(ctx context.Context, req *searchv1.KeywordSearchRequest) (*searchv1.SearchResponse, error) {
	s.logger.Debug("KeywordSearch request received",
		logging.F("query", req.GetQuery()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("limit", req.GetLimit()),
	)

	// TODO: Implement keyword search
	// 1. Execute full-text search against PostgreSQL using ts_vector
	// 2. Apply filters and return results

	return nil, status.Error(codes.Unimplemented, "KeywordSearch not yet implemented")
}

// IndexDocument adds or updates a document in the search index.
// Documents are automatically indexed for both full-text and vector search.
func (s *SearchServer) IndexDocument(ctx context.Context, req *searchv1.IndexDocumentRequest) (*searchv1.IndexDocumentResponse, error) {
	s.logger.Debug("IndexDocument request received",
		logging.F("document_id", req.GetDocumentId()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("content_type", req.GetContentType()),
	)

	// TODO: Implement document indexing
	// 1. Validate document
	// 2. Generate embedding if not provided
	// 3. Store document in PostgreSQL with full-text indexing
	// 4. Store embedding in vector database
	// 5. Return indexing result

	return nil, status.Error(codes.Unimplemented, "IndexDocument not yet implemented")
}

// DeleteDocument removes a document from the search index.
func (s *SearchServer) DeleteDocument(ctx context.Context, req *searchv1.DeleteDocumentRequest) (*searchv1.DeleteDocumentResponse, error) {
	s.logger.Debug("DeleteDocument request received",
		logging.F("document_id", req.GetDocumentId()),
		logging.F("tenant_id", req.GetTenantId()),
	)

	// TODO: Implement document deletion
	// 1. Remove from PostgreSQL
	// 2. Remove from vector database
	// 3. Return deletion result

	return nil, status.Error(codes.Unimplemented, "DeleteDocument not yet implemented")
}

// GetSearchStats returns statistics about the search index.
func (s *SearchServer) GetSearchStats(ctx context.Context, req *searchv1.GetSearchStatsRequest) (*searchv1.GetSearchStatsResponse, error) {
	s.logger.Debug("GetSearchStats request received",
		logging.F("tenant_id", req.GetTenantId()),
	)

	// TODO: Implement statistics retrieval
	// 1. Query document counts from PostgreSQL
	// 2. Query vector index stats
	// 3. Aggregate and return stats

	return nil, status.Error(codes.Unimplemented, "GetSearchStats not yet implemented")
}
