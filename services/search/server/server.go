// Package server implements the gRPC SearchService.
// It provides hybrid search functionality combining full-text and vector similarity search.
package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/searchv1"
	"github.com/otherjamesbrown/penfold/pkg/embeddings"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/search/cache"
	"github.com/otherjamesbrown/penfold/services/search/config"
	"github.com/otherjamesbrown/penfold/services/search/engine"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SearchServer implements the SearchService gRPC interface.
type SearchServer struct {
	searchv1.UnimplementedSearchServiceServer

	cfg             *config.Config
	logger          logging.Logger
	metrics         *metrics.Metrics
	rrfEngine       *engine.RRFEngine
	vectorEngine    *engine.VectorEngine
	bm25Engine      *engine.BM25Engine
	embeddingClient embeddings.Client
	cache           *cache.SearchCache
	db              *pgxpool.Pool
}

// ServerOption is a functional option for configuring SearchServer.
type ServerOption func(*SearchServer)

// WithRRFEngine sets the hybrid search engine.
func WithRRFEngine(e *engine.RRFEngine) ServerOption {
	return func(s *SearchServer) {
		s.rrfEngine = e
	}
}

// WithVectorEngine sets the vector search engine.
func WithVectorEngine(e *engine.VectorEngine) ServerOption {
	return func(s *SearchServer) {
		s.vectorEngine = e
	}
}

// WithBM25Engine sets the BM25 search engine.
func WithBM25Engine(e *engine.BM25Engine) ServerOption {
	return func(s *SearchServer) {
		s.bm25Engine = e
	}
}

// WithEmbeddingClient sets the embedding client.
func WithEmbeddingClient(c embeddings.Client) ServerOption {
	return func(s *SearchServer) {
		s.embeddingClient = c
	}
}

// WithCache sets the search cache.
func WithCache(c *cache.SearchCache) ServerOption {
	return func(s *SearchServer) {
		s.cache = c
	}
}

// WithDB sets the database connection pool.
func WithDB(db *pgxpool.Pool) ServerOption {
	return func(s *SearchServer) {
		s.db = db
	}
}

// NewSearchServer creates a new SearchServer instance.
func NewSearchServer(cfg *config.Config, logger logging.Logger, m *metrics.Metrics, opts ...ServerOption) *SearchServer {
	s := &SearchServer{
		cfg:     cfg,
		logger:  logger.With(logging.F("component", "search_server")),
		metrics: m,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Search performs a hybrid search combining full-text and vector similarity.
// This is the recommended search method for most use cases.
func (s *SearchServer) Search(ctx context.Context, req *searchv1.SearchRequest) (*searchv1.SearchResponse, error) {
	s.logger.Debug("Search request received",
		logging.F("query", req.GetQuery()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("limit", req.GetLimit()),
	)

	// Validate request
	if err := s.validateSearchRequest(req.GetQuery(), req.GetTenantId()); err != nil {
		return nil, err
	}

	// Check engine availability
	if s.rrfEngine == nil {
		return nil, status.Error(codes.Unavailable, "hybrid search engine not configured")
	}

	// Check cache first
	if s.cache != nil {
		cacheKey := cache.GenerateKeyForHybridSearch(req)
		if cached := s.cache.Get(cacheKey); cached != nil {
			s.logger.Debug("Cache hit for hybrid search")
			return cached, nil
		}
	}

	// Configure weights from request or use defaults
	if req.TextWeight != nil || req.VectorWeight != nil {
		textWeight := float64(0.5)
		vectorWeight := float64(0.5)
		if req.TextWeight != nil {
			textWeight = float64(*req.TextWeight)
		}
		if req.VectorWeight != nil {
			vectorWeight = float64(*req.VectorWeight)
		}
		if err := s.rrfEngine.SetWeights(textWeight, vectorWeight); err != nil {
			s.logger.Warn("Failed to set weights, using defaults", logging.Err(err))
		}
	}

	// Build filters
	filters := s.toHybridFilters(req.GetFilters())

	// Apply limit constraints
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = s.cfg.Search.DefaultLimit
	}
	if limit > s.cfg.Search.MaxLimit {
		limit = s.cfg.Search.MaxLimit
	}
	offset := int(req.GetOffset())
	if offset < 0 {
		offset = 0
	}

	// Execute hybrid search
	response, err := s.rrfEngine.HybridSearch(ctx, req.GetTenantId(), req.GetQuery(), filters, limit, offset)
	if err != nil {
		return nil, s.mapEngineError(err)
	}

	// Convert to proto response
	protoResp := s.toSearchResponse(response)

	// Cache the result
	if s.cache != nil {
		cacheKey := cache.GenerateKeyForHybridSearch(req)
		s.cache.Set(cacheKey, protoResp, req.GetTenantId(), cache.DefaultTTL)
	}

	return protoResp, nil
}

// SemanticSearch performs vector-only similarity search.
// Best for finding conceptually related content regardless of exact keywords.
func (s *SearchServer) SemanticSearch(ctx context.Context, req *searchv1.SemanticSearchRequest) (*searchv1.SearchResponse, error) {
	s.logger.Debug("SemanticSearch request received",
		logging.F("query", req.GetQuery()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("limit", req.GetLimit()),
	)

	// Validate request
	if err := s.validateSearchRequest(req.GetQuery(), req.GetTenantId()); err != nil {
		return nil, err
	}

	// Check engine availability
	if s.vectorEngine == nil {
		return nil, status.Error(codes.Unavailable, "vector search engine not configured")
	}

	// Check cache first
	if s.cache != nil {
		cacheKey := cache.GenerateKeyForSemanticSearch(req)
		if cached := s.cache.Get(cacheKey); cached != nil {
			s.logger.Debug("Cache hit for semantic search")
			return cached, nil
		}
	}

	// Build filters
	filters := s.toVectorFilters(req.GetFilters())

	// Apply limit constraints
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = s.cfg.Search.DefaultLimit
	}
	if limit > s.cfg.Search.MaxLimit {
		limit = s.cfg.Search.MaxLimit
	}
	offset := int(req.GetOffset())
	if offset < 0 {
		offset = 0
	}

	// Execute vector search
	response, err := s.vectorEngine.Search(ctx, req.GetTenantId(), req.GetQuery(), filters, limit, offset)
	if err != nil {
		return nil, s.mapEngineError(err)
	}

	// Convert to proto response
	protoResp := s.vectorSearchToResponse(response)

	// Cache the result
	if s.cache != nil {
		cacheKey := cache.GenerateKeyForSemanticSearch(req)
		s.cache.Set(cacheKey, protoResp, req.GetTenantId(), cache.DefaultTTL)
	}

	return protoResp, nil
}

// KeywordSearch performs full-text only search.
// Best for exact phrase matching or when semantic similarity is not needed.
func (s *SearchServer) KeywordSearch(ctx context.Context, req *searchv1.KeywordSearchRequest) (*searchv1.SearchResponse, error) {
	s.logger.Debug("KeywordSearch request received",
		logging.F("query", req.GetQuery()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("limit", req.GetLimit()),
	)

	// Validate request
	if err := s.validateSearchRequest(req.GetQuery(), req.GetTenantId()); err != nil {
		return nil, err
	}

	// Check engine availability
	if s.bm25Engine == nil {
		return nil, status.Error(codes.Unavailable, "keyword search engine not configured")
	}

	// Check cache first
	if s.cache != nil {
		cacheKey := cache.GenerateKeyForKeywordSearch(req)
		if cached := s.cache.Get(cacheKey); cached != nil {
			s.logger.Debug("Cache hit for keyword search")
			return cached, nil
		}
	}

	// Build filters
	filters := s.toBM25Filters(req.GetTenantId(), req.GetFilters())

	// Apply limit constraints
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = s.cfg.Search.DefaultLimit
	}
	if limit > s.cfg.Search.MaxLimit {
		limit = s.cfg.Search.MaxLimit
	}
	offset := int(req.GetOffset())
	if offset < 0 {
		offset = 0
	}

	// Execute BM25 search
	response, err := s.bm25Engine.Search(ctx, req.GetQuery(), filters, limit, offset)
	if err != nil {
		return nil, s.mapEngineError(err)
	}

	// Convert to proto response
	protoResp := s.bm25SearchToResponse(response)

	// Cache the result
	if s.cache != nil {
		cacheKey := cache.GenerateKeyForKeywordSearch(req)
		s.cache.Set(cacheKey, protoResp, req.GetTenantId(), cache.DefaultTTL)
	}

	return protoResp, nil
}

// IndexDocument adds or updates a document in the search index.
// Documents are automatically indexed for both full-text and vector search.
func (s *SearchServer) IndexDocument(ctx context.Context, req *searchv1.IndexDocumentRequest) (*searchv1.IndexDocumentResponse, error) {
	s.logger.Debug("IndexDocument request received",
		logging.F("document_id", req.GetDocumentId()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("content_type", req.GetContentType()),
	)

	// Validate required fields
	if req.GetDocumentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "document_id is required")
	}
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.GetContent() == "" {
		return nil, status.Error(codes.InvalidArgument, "content is required")
	}

	// Check database availability
	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database connection not configured")
	}

	// Generate embedding if not provided
	embedding := req.GetEmbedding()
	if len(embedding) == 0 {
		if s.embeddingClient == nil {
			return nil, status.Error(codes.Unavailable, "embedding client not configured and no embedding provided")
		}
		var err error
		embedding, err = s.embeddingClient.Embed(ctx, req.GetContent())
		if err != nil {
			s.logger.Error("Failed to generate embedding",
				logging.F("document_id", req.GetDocumentId()),
				logging.Err(err),
			)
			return nil, s.mapEngineError(err)
		}
	}

	// Prepare timestamps
	now := time.Now()
	createdAt := now
	if req.GetCreatedAt() != nil {
		createdAt = req.GetCreatedAt().AsTime()
	}
	updatedAt := now
	if req.GetUpdatedAt() != nil {
		updatedAt = req.GetUpdatedAt().AsTime()
	}

	// Upsert document into database
	query := `
		INSERT INTO search_documents (
			document_id, tenant_id, content_type, source_id, title, content,
			embedding, created_at, updated_at, metadata, tags, entity_ids
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7::vector, $8, $9, $10, $11, $12
		)
		ON CONFLICT (document_id, tenant_id) DO UPDATE SET
			content_type = EXCLUDED.content_type,
			source_id = EXCLUDED.source_id,
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			embedding = EXCLUDED.embedding,
			updated_at = EXCLUDED.updated_at,
			metadata = EXCLUDED.metadata,
			tags = EXCLUDED.tags,
			entity_ids = EXCLUDED.entity_ids
		RETURNING (xmax = 0) AS created
	`

	var title *string
	if t := req.GetTitle(); t != "" {
		title = &t
	}

	var created bool
	err := s.db.QueryRow(ctx, query,
		req.GetDocumentId(),
		req.GetTenantId(),
		req.GetContentType(),
		req.GetSourceId(),
		title,
		req.GetContent(),
		pgVectorFormat(embedding),
		createdAt,
		updatedAt,
		req.GetMetadata(),
		req.GetTags(),
		req.GetEntityIds(),
	).Scan(&created)

	if err != nil {
		s.logger.Error("Failed to index document",
			logging.F("document_id", req.GetDocumentId()),
			logging.Err(err),
		)
		return nil, status.Error(codes.Internal, "failed to index document")
	}

	// Invalidate cache for this tenant
	if s.cache != nil {
		s.cache.InvalidateForTenant(req.GetTenantId())
	}

	s.logger.Info("Document indexed",
		logging.F("document_id", req.GetDocumentId()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("created", created),
	)

	return &searchv1.IndexDocumentResponse{
		DocumentId: req.GetDocumentId(),
		Created:    created,
		IndexedAt:  timestamppb.Now(),
	}, nil
}

// DeleteDocument removes a document from the search index.
func (s *SearchServer) DeleteDocument(ctx context.Context, req *searchv1.DeleteDocumentRequest) (*searchv1.DeleteDocumentResponse, error) {
	s.logger.Debug("DeleteDocument request received",
		logging.F("document_id", req.GetDocumentId()),
		logging.F("tenant_id", req.GetTenantId()),
	)

	// Validate required fields
	if req.GetDocumentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "document_id is required")
	}
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Check database availability
	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database connection not configured")
	}

	// Delete from database (tenant_id ensures tenant isolation)
	query := `DELETE FROM search_documents WHERE document_id = $1 AND tenant_id = $2`
	result, err := s.db.Exec(ctx, query, req.GetDocumentId(), req.GetTenantId())
	if err != nil {
		s.logger.Error("Failed to delete document",
			logging.F("document_id", req.GetDocumentId()),
			logging.Err(err),
		)
		return nil, status.Error(codes.Internal, "failed to delete document")
	}

	deleted := result.RowsAffected() > 0

	// Invalidate cache for this tenant
	if s.cache != nil && deleted {
		s.cache.InvalidateForTenant(req.GetTenantId())
	}

	s.logger.Info("Document deletion completed",
		logging.F("document_id", req.GetDocumentId()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("deleted", deleted),
	)

	return &searchv1.DeleteDocumentResponse{
		Deleted: deleted,
	}, nil
}

// GetSearchStats returns statistics about the search index.
func (s *SearchServer) GetSearchStats(ctx context.Context, req *searchv1.GetSearchStatsRequest) (*searchv1.GetSearchStatsResponse, error) {
	s.logger.Debug("GetSearchStats request received",
		logging.F("tenant_id", req.GetTenantId()),
	)

	// Check database availability
	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database connection not configured")
	}

	// Query document statistics
	var query string
	var args []interface{}

	if tenantID := req.GetTenantId(); tenantID != "" {
		query = `
			SELECT
				COUNT(*) as total_documents,
				MIN(created_at) as oldest_document,
				MAX(created_at) as newest_document,
				MAX(updated_at) as last_indexed_at
			FROM search_documents
			WHERE tenant_id = $1
		`
		args = append(args, tenantID)
	} else {
		query = `
			SELECT
				COUNT(*) as total_documents,
				MIN(created_at) as oldest_document,
				MAX(created_at) as newest_document,
				MAX(updated_at) as last_indexed_at
			FROM search_documents
		`
	}

	var totalDocs int64
	var oldestDoc, newestDoc, lastIndexedAt *time.Time
	err := s.db.QueryRow(ctx, query, args...).Scan(&totalDocs, &oldestDoc, &newestDoc, &lastIndexedAt)
	if err != nil {
		s.logger.Error("Failed to get search stats", logging.Err(err))
		return nil, status.Error(codes.Internal, "failed to get search stats")
	}

	resp := &searchv1.GetSearchStatsResponse{
		TotalDocuments: totalDocs,
	}

	if oldestDoc != nil {
		resp.OldestDocument = timestamppb.New(*oldestDoc)
	}
	if newestDoc != nil {
		resp.NewestDocument = timestamppb.New(*newestDoc)
	}
	if lastIndexedAt != nil {
		resp.LastIndexedAt = timestamppb.New(*lastIndexedAt)
	}

	// Set embedding dimensions from config
	if s.cfg != nil {
		resp.EmbeddingDimensions = int32(s.cfg.Embedding.Dimensions)
	}

	return resp, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// validateSearchRequest performs common validation for search requests.
func (s *SearchServer) validateSearchRequest(query, tenantID string) error {
	if strings.TrimSpace(tenantID) == "" {
		return status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if strings.TrimSpace(query) == "" {
		return status.Error(codes.InvalidArgument, "query is required")
	}
	return nil
}

// mapEngineError converts engine errors to appropriate gRPC status codes.
func (s *SearchServer) mapEngineError(err error) error {
	if err == nil {
		return nil
	}

	// Check for known engine errors
	switch {
	case errors.Is(err, engine.ErrInvalidTenantID):
		return status.Error(codes.InvalidArgument, "invalid tenant_id")
	case errors.Is(err, engine.ErrEmptyQuery):
		return status.Error(codes.InvalidArgument, "query cannot be empty")
	case errors.Is(err, engine.ErrEmbeddingServiceUnavailable):
		return status.Error(codes.Unavailable, "embedding service unavailable")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "search operation timed out")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "search operation canceled")
	default:
		s.logger.Error("Search engine error", logging.Err(err))
		return status.Error(codes.Internal, "internal search error")
	}
}

// toHybridFilters converts proto FilterOptions to engine HybridSearchFilters.
func (s *SearchServer) toHybridFilters(f *searchv1.FilterOptions) *engine.HybridSearchFilters {
	if f == nil {
		return nil
	}

	filters := &engine.HybridSearchFilters{
		ContentTypes: f.GetContentTypes(),
		Tags:         f.GetTags(),
		SourceIDs:    f.GetSourceIds(),
		ExcludeIDs:   f.GetExcludeIds(),
	}

	if f.GetDateFrom() != nil {
		t := f.GetDateFrom().AsTime()
		filters.DateFrom = &t
	}
	if f.GetDateTo() != nil {
		t := f.GetDateTo().AsTime()
		filters.DateTo = &t
	}

	return filters
}

// toVectorFilters converts proto FilterOptions to engine VectorSearchFilters.
func (s *SearchServer) toVectorFilters(f *searchv1.FilterOptions) *engine.VectorSearchFilters {
	if f == nil {
		return nil
	}

	filters := &engine.VectorSearchFilters{
		ContentTypes: f.GetContentTypes(),
		SourceIDs:    f.GetSourceIds(),
		Tags:         f.GetTags(),
		EntityIDs:    f.GetEntityIds(),
		Sources:      f.GetSources(),
		ExcludeIDs:   f.GetExcludeIds(),
	}

	if f.GetDateFrom() != nil {
		t := f.GetDateFrom().AsTime()
		filters.DateFrom = &t
	}
	if f.GetDateTo() != nil {
		t := f.GetDateTo().AsTime()
		filters.DateTo = &t
	}

	return filters
}

// toBM25Filters converts proto FilterOptions to engine SearchFilters.
func (s *SearchServer) toBM25Filters(tenantID string, f *searchv1.FilterOptions) engine.SearchFilters {
	filters := engine.SearchFilters{
		TenantID: tenantID,
	}

	if f == nil {
		return filters
	}

	filters.ContentTypes = f.GetContentTypes()
	filters.Tags = f.GetTags()
	filters.SourceIDs = f.GetSourceIds()
	filters.ExcludeIDs = f.GetExcludeIds()

	if f.GetDateFrom() != nil {
		t := f.GetDateFrom().AsTime()
		filters.DateFrom = &t
	}
	if f.GetDateTo() != nil {
		t := f.GetDateTo().AsTime()
		filters.DateTo = &t
	}

	return filters
}

// toSearchResponse converts engine HybridSearchResponse to proto SearchResponse.
func (s *SearchServer) toSearchResponse(r *engine.HybridSearchResponse) *searchv1.SearchResponse {
	if r == nil {
		return &searchv1.SearchResponse{}
	}

	results := make([]*searchv1.SearchResult, len(r.Results))
	for i, res := range r.Results {
		result := &searchv1.SearchResult{
			DocumentId:  res.DocumentID,
			ContentType: res.ContentType,
			SourceId:    res.SourceID,
			Snippet:     res.Snippet,
			Highlights:  res.Highlights,
			Score:       float32(res.Score),
			CreatedAt:   timestamppb.New(res.CreatedAt),
			UpdatedAt:   timestamppb.New(res.UpdatedAt),
			Metadata:    res.Metadata,
		}
		if res.Title != nil {
			result.Title = res.Title
		}
		if res.BM25Score > 0 {
			textScore := float32(res.BM25Score)
			result.TextScore = &textScore
		}
		if res.VectorScore > 0 {
			vectorScore := float32(res.VectorScore)
			result.VectorScore = &vectorScore
		}
		results[i] = result
	}

	return &searchv1.SearchResponse{
		Results:     results,
		TotalCount:  r.TotalCount,
		QueryTimeMs: r.QueryTimeMs,
	}
}

// vectorSearchToResponse converts engine VectorSearchResponse to proto SearchResponse.
func (s *SearchServer) vectorSearchToResponse(r *engine.VectorSearchResponse) *searchv1.SearchResponse {
	if r == nil {
		return &searchv1.SearchResponse{}
	}

	results := make([]*searchv1.SearchResult, len(r.Results))
	for i, res := range r.Results {
		result := &searchv1.SearchResult{
			DocumentId:  res.DocumentID,
			ContentType: res.ContentType,
			SourceId:    res.SourceID,
			Snippet:     res.Snippet,
			Score:       float32(res.SimilarityScore),
			CreatedAt:   timestamppb.New(res.CreatedAt),
			UpdatedAt:   timestamppb.New(res.UpdatedAt),
			Metadata:    res.Metadata,
		}
		if res.Title != nil {
			result.Title = res.Title
		}
		vectorScore := float32(res.SimilarityScore)
		result.VectorScore = &vectorScore
		results[i] = result
	}

	return &searchv1.SearchResponse{
		Results:     results,
		TotalCount:  r.TotalCount,
		QueryTimeMs: r.QueryTimeMs,
	}
}

// bm25SearchToResponse converts engine SearchResponse to proto SearchResponse.
func (s *SearchServer) bm25SearchToResponse(r *engine.SearchResponse) *searchv1.SearchResponse {
	if r == nil {
		return &searchv1.SearchResponse{}
	}

	results := make([]*searchv1.SearchResult, len(r.Results))
	for i, res := range r.Results {
		result := &searchv1.SearchResult{
			DocumentId:  res.DocumentID,
			ContentType: res.ContentType,
			SourceId:    res.SourceID,
			Snippet:     res.Snippet,
			Highlights:  res.Highlights,
			Score:       float32(res.Score),
			CreatedAt:   timestamppb.New(res.CreatedAt),
			UpdatedAt:   timestamppb.New(res.UpdatedAt),
			Metadata:    res.Metadata,
		}
		if res.Title != nil {
			result.Title = res.Title
		}
		textScore := float32(res.Score)
		result.TextScore = &textScore
		results[i] = result
	}

	return &searchv1.SearchResponse{
		Results:     results,
		TotalCount:  r.TotalCount,
		QueryTimeMs: r.QueryTimeMs,
	}
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
