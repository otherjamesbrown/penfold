// Package searchservice implements the SearchService gRPC for the Gateway.
// It queries PostgreSQL directly using hybrid search (full-text + vector similarity).
package searchservice

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tenant"
)

// EmbeddingGenerator generates vector embeddings for query text.
// Implemented by *ai.Client.
type EmbeddingGenerator interface {
	GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error)
}

// QueryExpander expands a search query using glossary aliases and synonyms.
// Implemented by *glossary.Repository.
type QueryExpander interface {
	ExpandQuery(ctx context.Context, tenantID string, query string) (*glossary.QueryExpansion, error)
}

// Service implements the SearchServiceServer using direct database queries.
type Service struct {
	searchv1.UnimplementedSearchServiceServer

	db         *pgxpool.Pool
	tenantRepo *tenant.Repository
	embedder   EmbeddingGenerator
	expander   QueryExpander
	logger     logging.Logger
}

// NewService creates a new search service with database access.
func NewService(db *pgxpool.Pool, logger logging.Logger) *Service {
	if logger == nil {
		logger = logging.NewLogger(logging.DefaultConfig())
	}
	return &Service{
		db:         db,
		tenantRepo: tenant.NewRepository(db),
		logger:     logger,
	}
}

// SetEmbedder configures an optional embedding generator for hybrid search.
// When set, Search() blends keyword and vector similarity scores.
// When nil, Search() uses keyword-only scoring (ts_rank_cd).
func (s *Service) SetEmbedder(e EmbeddingGenerator) {
	s.embedder = e
}

// SetQueryExpander configures an optional glossary expander for alias-based query expansion.
// When set, Search() expands the keyword query to include glossary aliases.
// When nil, Search() uses the original query unchanged.
func (s *Service) SetQueryExpander(e QueryExpander) {
	s.expander = e
}

// buildAliasQuery builds a websearch_to_tsquery-compatible OR query from glossary expansion.
// It includes the original query, the canonical term name, and all aliases.
// Each term is double-quoted (phrase match) and any embedded double-quotes are stripped.
func buildAliasQuery(originalQuery string, expansion *glossary.QueryExpansion) string {
	seen := map[string]bool{}
	parts := []string{}

	addPart := func(s string) {
		// Strip embedded double-quotes to avoid malformed websearch_to_tsquery input
		s = strings.ReplaceAll(s, `"`, "")
		s = strings.TrimSpace(s)
		lower := strings.ToLower(s)
		if s != "" && !seen[lower] {
			seen[lower] = true
			parts = append(parts, `"`+s+`"`)
		}
	}

	addPart(originalQuery)
	for _, term := range expansion.ExpandedTerms {
		addPart(term.TermName)
		addPart(term.OriginalTerm)
		for _, alias := range term.Aliases {
			addPart(alias)
		}
	}

	if len(parts) == 0 {
		return originalQuery
	}
	return strings.Join(parts, " OR ")
}

// buildExpansionInfo formats a human-readable description of the expansion applied.
// Example: "36QDD expanded via Juniper Border Routers (3 aliases)"
func buildExpansionInfo(expansion *glossary.QueryExpansion) string {
	if len(expansion.ExpandedTerms) == 0 {
		return ""
	}
	parts := make([]string, 0, len(expansion.ExpandedTerms))
	for _, term := range expansion.ExpandedTerms {
		aliasCount := len(term.Aliases)
		if aliasCount == 1 {
			parts = append(parts, fmt.Sprintf("%s expanded via %s (1 alias)", expansion.OriginalQuery, term.TermName))
		} else {
			parts = append(parts, fmt.Sprintf("%s expanded via %s (%d aliases)", expansion.OriginalQuery, term.TermName, aliasCount))
		}
	}
	return strings.Join(parts, "; ")
}

// resolveTenantID resolves a tenant reference (slug or UUID) to a UUID.
func (s *Service) resolveTenantID(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}

	// Check if it's already a UUID
	if _, err := uuid.Parse(ref); err == nil {
		return ref, nil
	}

	// Try to resolve as slug
	if s.tenantRepo == nil {
		return "", fmt.Errorf("tenant repository not configured")
	}

	t, err := s.tenantRepo.GetByRef(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("tenant not found: %s", ref)
	}
	if t == nil {
		return "", fmt.Errorf("tenant not found: %s", ref)
	}

	return t.ID, nil
}

// embedQuery generates a query embedding vector as a pgvector-format string.
// Returns "" if embedding is unavailable (no embedder configured, or call fails).
func (s *Service) embedQuery(ctx context.Context, query, tenantID string) string {
	if s.embedder == nil {
		return ""
	}

	resp, err := s.embedder.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
		Text:     query,
		TenantId: &tenantID,
	})
	if err != nil {
		s.logger.Warn("Failed to generate query embedding, falling back to keyword-only",
			logging.Err(err),
		)
		return ""
	}

	if len(resp.GetVector()) == 0 {
		return ""
	}

	return vecToString(resp.GetVector())
}

// vecToString converts a float32 slice to pgvector text format: "[0.1,0.2,0.3]".
func vecToString(v []float32) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = strconv.FormatFloat(float64(x), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Search performs a hybrid search combining full-text and vector similarity.
func (s *Service) Search(ctx context.Context, req *searchv1.SearchRequest) (*searchv1.SearchResponse, error) {
	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not configured")
	}

	query := strings.TrimSpace(req.GetQuery())
	if query == "" {
		return nil, status.Error(codes.InvalidArgument, "query cannot be empty")
	}

	tenantID, err := s.resolveTenantID(ctx, req.GetTenantId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant: %v", err)
	}
	if tenantID == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

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

	// Resolve blending weights (default 0.5/0.5)
	textWeight := float64(0.5)
	vectorWeight := float64(0.5)
	if req.TextWeight != nil {
		textWeight = float64(*req.TextWeight)
	}
	if req.VectorWeight != nil {
		vectorWeight = float64(*req.VectorWeight)
	}

	sortOrder := req.GetSortOrder()

	s.logger.Debug("Search request",
		logging.F("tenant_id", tenantID),
		logging.F("query", query),
		logging.F("limit", limit),
		logging.F("offset", offset),
		logging.F("text_weight", textWeight),
		logging.F("vector_weight", vectorWeight),
		logging.F("sort_order", sortOrder.String()),
	)

	startTime := time.Now()

	// Expand the keyword query via glossary aliases (optional).
	// The vector embedding always uses the original query for best semantic recall.
	keywordQuery := query
	expansionInfo := ""
	if s.expander != nil {
		expansion, expErr := s.expander.ExpandQuery(ctx, tenantID, query)
		if expErr != nil {
			s.logger.Warn("Glossary expansion failed, using original query", logging.Err(expErr))
		} else if len(expansion.ExpandedTerms) > 0 {
			keywordQuery = buildAliasQuery(query, expansion)
			expansionInfo = buildExpansionInfo(expansion)
			s.logger.Debug("Query expanded via glossary",
				logging.F("original", query),
				logging.F("expanded", keywordQuery),
				logging.F("expansion_info", expansionInfo),
			)
		}
	}

	// Try to embed the ORIGINAL query for hybrid scoring (not the expanded one).
	queryVecStr := s.embedQuery(ctx, query, tenantID)
	useHybrid := queryVecStr != ""

	var results []*searchv1.SearchResult

	if useHybrid {
		results, err = s.hybridSearch(ctx, keywordQuery, tenantID, queryVecStr, textWeight, vectorWeight, limit, offset, sortOrder)
	} else {
		results, err = s.keywordOnlySearch(ctx, keywordQuery, tenantID, limit, offset, sortOrder)
	}
	if err != nil {
		return nil, err
	}

	// Get total count (keyword match count, independent of vector scoring)
	var totalCount int64
	err = s.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM sources
		WHERE tenant_id = $1
			AND to_tsvector('english', COALESCE(raw_content, '')) @@ websearch_to_tsquery('english', $2)
	`, tenantID, keywordQuery).Scan(&totalCount)
	if err != nil {
		s.logger.Warn("Failed to get total count", logging.Err(err))
		totalCount = int64(len(results))
	}

	queryTimeMs := float64(time.Since(startTime).Microseconds()) / 1000.0

	s.logger.Debug("Search completed",
		logging.F("total_count", totalCount),
		logging.F("result_count", len(results)),
		logging.F("query_time_ms", queryTimeMs),
		logging.F("hybrid", useHybrid),
	)

	return &searchv1.SearchResponse{
		Results:       results,
		TotalCount:    totalCount,
		QueryTimeMs:   queryTimeMs,
		ExpansionInfo: expansionInfo,
	}, nil
}

// hybridSearch runs keyword + vector blend. Text matches are required (keyword
// match gate), but vector similarity re-ranks results for much better
// score differentiation.
func (s *Service) hybridSearch(ctx context.Context, query, tenantID string, queryVecStr string, textWeight, vectorWeight float64, limit, offset int32, sortOrder searchv1.SortOrder) ([]*searchv1.SearchResult, error) {
	// Determine SQL ORDER BY clause based on requested sort order.
	// For relevance (default), order by blended score; for date sorts, order by created_at.
	var sqlOrderBy string
	switch sortOrder {
	case searchv1.SortOrder_SORT_ORDER_DATE_DESC:
		sqlOrderBy = "tm.created_at DESC, ($6::float8 * tm.text_score + $7::float8 * COALESCE(bv.vector_score, 0)) DESC"
	case searchv1.SortOrder_SORT_ORDER_DATE_ASC:
		sqlOrderBy = "tm.created_at ASC, ($6::float8 * tm.text_score + $7::float8 * COALESCE(bv.vector_score, 0)) DESC"
	default:
		// SORT_ORDER_UNSPECIFIED and SORT_ORDER_RELEVANCE both rank by score
		sqlOrderBy = "($6::float8 * tm.text_score + $7::float8 * COALESCE(bv.vector_score, 0)) DESC, tm.created_at DESC"
	}

	// CTE approach:
	// 1. text_matches: find keyword-matching sources with ts_rank_cd
	// 2. best_vectors: for those sources, compute best cosine similarity from embeddings
	// 3. Blend and re-rank
	sqlQuery := fmt.Sprintf(`
		WITH text_matches AS (
			SELECT
				s.id,
				s.external_id,
				s.source_system,
				COALESCE(m.title, s.ingestion_metadata->>'title', s.ingestion_metadata->>'subject', s.content_type, 'Untitled') as title,
				LEFT(s.raw_content, 500) as snippet,
				ts_rank_cd(to_tsvector('english', COALESCE(s.raw_content, '')), websearch_to_tsquery('english', $1)) as text_score,
				s.content_type,
				s.source_timestamp,
				s.created_at
			FROM sources s
			LEFT JOIN meetings m ON s.meeting_id = m.id
			WHERE s.tenant_id = $2
				AND to_tsvector('english', COALESCE(s.raw_content, '')) @@ websearch_to_tsquery('english', $1)
		),
		best_vectors AS (
			SELECT DISTINCT ON (e.entity_id)
				e.entity_id as source_id,
				1 - (e.embedding_vec <=> $5::vector) as vector_score
			FROM embeddings e
			WHERE e.tenant_id = $2
				AND e.entity_type = 'source'
				AND e.embedding_vec IS NOT NULL
				AND e.entity_id IN (SELECT id FROM text_matches)
			ORDER BY e.entity_id, e.embedding_vec <=> $5::vector
		)
		SELECT
			tm.id,
			tm.external_id,
			tm.source_system,
			tm.title,
			tm.snippet,
			tm.text_score,
			bv.vector_score,
			tm.content_type,
			tm.source_timestamp,
			tm.created_at
		FROM text_matches tm
		LEFT JOIN best_vectors bv ON tm.id = bv.source_id
		ORDER BY %s
		LIMIT $3 OFFSET $4
	`, sqlOrderBy)

	rows, err := s.db.Query(ctx, sqlQuery, query, tenantID, limit, offset, queryVecStr, textWeight, vectorWeight)
	if err != nil {
		s.logger.Error("Hybrid search query failed", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "search query failed: %v", err)
	}
	defer rows.Close()

	var results []*searchv1.SearchResult
	var maxTextScore float32
	for rows.Next() {
		var (
			id              int64
			externalID      string
			sourceSystem    string
			title           string
			snippet         string
			textScore       float32
			vectorScore     *float32
			contentType     *string
			sourceTimestamp *time.Time
			createdAt       time.Time
		)

		if err := rows.Scan(&id, &externalID, &sourceSystem, &title, &snippet, &textScore, &vectorScore, &contentType, &sourceTimestamp, &createdAt); err != nil {
			s.logger.Error("Failed to scan hybrid search result", logging.Err(err))
			continue
		}

		if textScore > maxTextScore {
			maxTextScore = textScore
		}

		result := &searchv1.SearchResult{
			DocumentId:  fmt.Sprintf("%d", id),
			SourceId:    externalID,
			ContentType: derefString(contentType, sourceSystem),
			Title:       &title,
			Snippet:     highlightSnippet(snippet, query),
			TextScore:   &textScore,
			VectorScore: vectorScore,
			CreatedAt:   timestamppb.New(createdAt),
			Metadata:    map[string]string{"source_system": sourceSystem},
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("Error iterating hybrid search results", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "error reading results: %v", err)
	}

	// Normalize scores: text_score to [0,1] by dividing by max, then blend.
	// Re-sort since normalization changes relative ordering after normalization.
	normalizeAndBlend(results, maxTextScore, float32(textWeight), float32(vectorWeight))
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

	return results, nil
}

// keywordOnlySearch runs ts_rank_cd without vector scoring.
// Used when the embedder is unavailable.
func (s *Service) keywordOnlySearch(ctx context.Context, query, tenantID string, limit, offset int32, sortOrder searchv1.SortOrder) ([]*searchv1.SearchResult, error) {
	// Determine SQL ORDER BY clause based on requested sort order.
	var sqlOrderBy string
	switch sortOrder {
	case searchv1.SortOrder_SORT_ORDER_DATE_DESC:
		sqlOrderBy = "s.created_at DESC, score DESC"
	case searchv1.SortOrder_SORT_ORDER_DATE_ASC:
		sqlOrderBy = "s.created_at ASC, score DESC"
	default:
		// SORT_ORDER_UNSPECIFIED and SORT_ORDER_RELEVANCE: rank by score
		sqlOrderBy = "score DESC, s.created_at DESC"
	}

	sqlQuery := fmt.Sprintf(`
		SELECT
			s.id,
			s.external_id,
			s.source_system,
			COALESCE(m.title, s.ingestion_metadata->>'title', s.ingestion_metadata->>'subject', s.content_type, 'Untitled') as title,
			LEFT(s.raw_content, 500) as snippet,
			ts_rank_cd(to_tsvector('english', COALESCE(s.raw_content, '')), websearch_to_tsquery('english', $1)) as score,
			s.content_type,
			s.source_timestamp,
			s.created_at
		FROM sources s
		LEFT JOIN meetings m ON s.meeting_id = m.id
		WHERE s.tenant_id = $2
			AND to_tsvector('english', COALESCE(s.raw_content, '')) @@ websearch_to_tsquery('english', $1)
		ORDER BY %s
		LIMIT $3 OFFSET $4
	`, sqlOrderBy)

	rows, err := s.db.Query(ctx, sqlQuery, query, tenantID, limit, offset)
	if err != nil {
		s.logger.Error("Keyword search query failed", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "search query failed: %v", err)
	}
	defer rows.Close()

	var results []*searchv1.SearchResult
	var maxScore float32
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

		if score > maxScore {
			maxScore = score
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

	// Normalize text scores to [0,1] for the Score field.
	// Note: normalization may reorder equal scores, so re-sort if needed.
	if maxScore > 0 {
		for _, r := range results {
			r.Score = r.Score / maxScore
		}
	}

	// Re-sort after normalization when date sort is requested (score field changed).
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
	}
	// For relevance sort (default), SQL already ordered by score DESC; normalization
	// preserves relative ordering, so no re-sort needed.

	return results, nil
}

// normalizeAndBlend computes the final blended Score for each result.
// Text scores are normalized to [0,1] by dividing by max. Vector scores
// are already in [0,1] (cosine similarity). The blended score is:
//
//	score = tw * norm_text + vw * vector_score
func normalizeAndBlend(results []*searchv1.SearchResult, maxTextScore, tw, vw float32) {
	if maxTextScore <= 0 {
		maxTextScore = 1
	}
	for _, r := range results {
		normText := float32(0)
		if r.TextScore != nil {
			normText = *r.TextScore / maxTextScore
		}
		vs := float32(0)
		if r.VectorScore != nil {
			vs = *r.VectorScore
		}
		r.Score = tw*normText + vw*vs
		// Clamp to [0, 1]
		if r.Score > 1 {
			r.Score = 1
		}
		if r.Score < 0 {
			r.Score = 0
		}
	}
}

// SemanticSearch performs pure vector similarity search.
func (s *Service) SemanticSearch(ctx context.Context, req *searchv1.SemanticSearchRequest) (*searchv1.SearchResponse, error) {
	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not configured")
	}

	query := strings.TrimSpace(req.GetQuery())
	if query == "" {
		return nil, status.Error(codes.InvalidArgument, "query cannot be empty")
	}

	tenantID, err := s.resolveTenantID(ctx, req.GetTenantId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant: %v", err)
	}
	if tenantID == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	queryVecStr := s.embedQuery(ctx, query, tenantID)
	if queryVecStr == "" {
		// Can't do semantic search without embeddings; fall back to keyword
		s.logger.Warn("SemanticSearch: embedder unavailable, falling back to keyword search")
		return s.Search(ctx, &searchv1.SearchRequest{
			Query:    req.GetQuery(),
			TenantId: req.GetTenantId(),
			Limit:    req.GetLimit(),
			Offset:   req.GetOffset(),
			Filters:  req.GetFilters(),
		})
	}

	limit := req.GetLimit()
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	minSimilarity := float64(0)
	if req.MinSimilarity != nil {
		minSimilarity = float64(*req.MinSimilarity)
	}

	startTime := time.Now()

	// Pure vector search using HNSW index on embedding_vec.
	// Join back to sources to get metadata.
	rows, err := s.db.Query(ctx, `
		WITH vector_results AS (
			SELECT
				e.entity_id as source_id,
				1 - (e.embedding_vec <=> $1::vector) as similarity,
				ROW_NUMBER() OVER (PARTITION BY e.entity_id ORDER BY e.embedding_vec <=> $1::vector) as rn
			FROM embeddings e
			WHERE e.tenant_id = $2
				AND e.entity_type = 'source'
				AND e.embedding_vec IS NOT NULL
				AND 1 - (e.embedding_vec <=> $1::vector) >= $4::float8
			ORDER BY e.embedding_vec <=> $1::vector
			LIMIT $3 * 3
		)
		SELECT
			s.id,
			s.external_id,
			s.source_system,
			COALESCE(m.title, s.ingestion_metadata->>'title', s.ingestion_metadata->>'subject', s.content_type, 'Untitled') as title,
			LEFT(s.raw_content, 500) as snippet,
			vr.similarity as score,
			s.content_type,
			s.source_timestamp,
			s.created_at
		FROM vector_results vr
		JOIN sources s ON s.id = vr.source_id AND s.tenant_id = $2
		LEFT JOIN meetings m ON s.meeting_id = m.id
		WHERE vr.rn = 1
		ORDER BY vr.similarity DESC
		LIMIT $3
	`, queryVecStr, tenantID, limit, minSimilarity)
	if err != nil {
		s.logger.Error("Semantic search query failed", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "semantic search failed: %v", err)
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
			s.logger.Error("Failed to scan semantic search result", logging.Err(err))
			continue
		}

		vs := score
		result := &searchv1.SearchResult{
			DocumentId:  fmt.Sprintf("%d", id),
			SourceId:    externalID,
			ContentType: derefString(contentType, sourceSystem),
			Title:       &title,
			Snippet:     snippet,
			Score:       score,
			VectorScore: &vs,
			CreatedAt:   timestamppb.New(createdAt),
			Metadata:    map[string]string{"source_system": sourceSystem},
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("Error iterating semantic search results", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "error reading results: %v", err)
	}

	queryTimeMs := float64(time.Since(startTime).Microseconds()) / 1000.0

	return &searchv1.SearchResponse{
		Results:     results,
		TotalCount:  int64(len(results)),
		QueryTimeMs: queryTimeMs,
	}, nil
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

	tenantRef := req.GetTenantId()
	var tenantID string
	var err error
	if tenantRef != "" {
		tenantID, err = s.resolveTenantID(ctx, tenantRef)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid tenant: %v", err)
		}
	}

	var totalDocs int64
	var q string
	var args []interface{}

	if tenantID != "" {
		q = "SELECT COUNT(*) FROM sources WHERE tenant_id = $1"
		args = []interface{}{tenantID}
	} else {
		q = "SELECT COUNT(*) FROM sources"
	}

	if err := s.db.QueryRow(ctx, q, args...).Scan(&totalDocs); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get stats: %v", err)
	}

	return &searchv1.GetSearchStatsResponse{
		TotalDocuments: totalDocs,
		LastIndexedAt:  timestamppb.Now(),
	}, nil
}

// highlightSnippet adds basic highlighting to search results.
func highlightSnippet(snippet, query string) string {
	words := strings.Fields(strings.ToLower(query))
	result := snippet
	for _, word := range words {
		if len(word) > 2 {
			result = strings.ReplaceAll(result, word, "<em>"+word+"</em>")
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

