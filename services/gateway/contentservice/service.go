// Package contentservice implements the ContentProcessorService gRPC server.
package contentservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	"github.com/otherjamesbrown/penfold/pkg/tenant"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
	"github.com/otherjamesbrown/penfold/services/gateway/internal/langfuse"
)

// Repository defines the interface for content storage operations.
type Repository interface {
	GetByContentID(ctx context.Context, contentID string) (*ContentItemRecord, error)
	ListByTenant(ctx context.Context, filter ListFilter) ([]*ContentItemRecord, error)
	DeleteByContentID(ctx context.Context, contentID string) error
	DeleteByFilters(ctx context.Context, tenantID string, sourceType, processingStatus *string) (int64, []string, error)
	GetStats(ctx context.Context, tenantID string) (*StatsRecord, error)
	GetContentText(ctx context.Context, contentID string) (*ContentTextRecord, error)
	ListAvailableInsights(ctx context.Context, contentID string) (*InsightsAvailabilityRecord, error)
	GetInsights(ctx context.Context, contentID string, types []string) ([]*InsightRecord, error)
}

// ContentItemRecord represents a content item from the database.
type ContentItemRecord struct {
	ID               int64
	TenantID         string
	SourceSystem     string
	ContentID        string
	ProcessingStatus string
	ContentSize      int32
	CreatedAt        time.Time
	UpdatedAt        time.Time
	EmbeddingCount   int
	AssertionCount   int
	FailureCategory  *string
	FailureReason    *string
	Metadata         map[string]interface{}
}

// ListFilter represents filter criteria for listing content items.
type ListFilter struct {
	TenantID         string
	SourceType       *string
	ProcessingStatus *string
	PageSize         int
	PageToken        string
}

// StatsRecord represents aggregate statistics for content.
type StatsRecord struct {
	TotalCount        int64
	CountByType       map[string]int64
	CountByStatus     map[string]int64
	EmbeddedCount     int64
	TotalStorageBytes int64
}

// ContentTextRecord represents the raw content text and metadata.
type ContentTextRecord struct {
	ContentID   string
	ContentType string
	Text        string
	CreatedAt   time.Time
	Metadata    map[string]interface{}
}

// InsightsAvailabilityRecord represents available insights for a content item.
type InsightsAvailabilityRecord struct {
	ContentID   string
	ContentType string
	Available   []string
	Extracted   []string
	Pending     []string
}

// InsightRecord represents a single extracted insight.
type InsightRecord struct {
	Type          string
	Data          map[string]interface{}
	ExtractedAt   time.Time
	ModelVersion  string
}

// repositoryImpl implements Repository using pgxpool.
type repositoryImpl struct {
	db     *pgxpool.Pool
	logger logging.Logger
}

// newRepository creates a new repository implementation.
func newRepository(db *pgxpool.Pool, logger logging.Logger) Repository {
	return &repositoryImpl{
		db:     db,
		logger: logger,
	}
}

// GetByContentID retrieves a single source by content_id.
func (r *repositoryImpl) GetByContentID(ctx context.Context, contentID string) (*ContentItemRecord, error) {
	query := `
		SELECT
			s.id,
			s.tenant_id,
			s.source_system,
			s.content_id,
			COALESCE(s.processing_status, 'pending') AS processing_status,
			s.content_size,
			s.created_at,
			s.updated_at,
			COUNT(DISTINCT e.id) AS embedding_count,
			COUNT(DISTINCT a.id) AS assertion_count,
			s.failure_category,
			s.failure_reason,
			s.ingestion_metadata
		FROM sources s
		LEFT JOIN embeddings e ON s.id = e.source_id
		LEFT JOIN assertions a ON s.id = a.source_id
		WHERE s.content_id = $1 AND (s.is_deleted IS NULL OR s.is_deleted = false)
		GROUP BY s.id
	`

	var rec ContentItemRecord
	var metadataJSON []byte
	err := r.db.QueryRow(ctx, query, contentID).Scan(
		&rec.ID,
		&rec.TenantID,
		&rec.SourceSystem,
		&rec.ContentID,
		&rec.ProcessingStatus,
		&rec.ContentSize,
		&rec.CreatedAt,
		&rec.UpdatedAt,
		&rec.EmbeddingCount,
		&rec.AssertionCount,
		&rec.FailureCategory,
		&rec.FailureReason,
		&metadataJSON,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get content item: %w", err)
	}

	// Parse metadata JSON
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &rec.Metadata); err != nil {
			r.logger.Warn("Failed to parse metadata JSON", logging.Err(err), logging.F("content_id", contentID))
			rec.Metadata = make(map[string]interface{})
		}
	}

	return &rec, nil
}

// ListByTenant retrieves content items with pagination and filters.
func (r *repositoryImpl) ListByTenant(ctx context.Context, filter ListFilter) ([]*ContentItemRecord, error) {
	// Build WHERE clause
	whereClauses := []string{"(s.is_deleted IS NULL OR s.is_deleted = false)"}
	args := []interface{}{}
	argCount := 1

	// Tenant filter (required)
	whereClauses = append(whereClauses, fmt.Sprintf("s.tenant_id = $%d", argCount))
	tenantUUID, err := uuid.Parse(filter.TenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant_id: %w", err)
	}
	args = append(args, tenantUUID)
	argCount++

	// Optional source_type filter
	if filter.SourceType != nil && *filter.SourceType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("s.source_system = $%d", argCount))
		args = append(args, *filter.SourceType)
		argCount++
	}

	// Optional processing_status filter
	if filter.ProcessingStatus != nil && *filter.ProcessingStatus != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("COALESCE(s.processing_status, 'pending') = $%d", argCount))
		args = append(args, *filter.ProcessingStatus)
		argCount++
	}

	// Build final query
	query := fmt.Sprintf(`
		SELECT
			s.id,
			s.tenant_id,
			s.source_system,
			s.content_id,
			COALESCE(s.processing_status, 'pending') AS processing_status,
			s.content_size,
			s.created_at,
			s.updated_at,
			COUNT(DISTINCT e.id) AS embedding_count,
			COUNT(DISTINCT a.id) AS assertion_count,
			s.failure_category,
			s.failure_reason,
			s.ingestion_metadata
		FROM sources s
		LEFT JOIN embeddings e ON s.id = e.source_id
		LEFT JOIN assertions a ON s.id = a.source_id
		WHERE %s
		GROUP BY s.id
		ORDER BY s.created_at DESC
		LIMIT $%d
	`, joinWhere(whereClauses), argCount)

	// Set page size limit
	pageSize := filter.PageSize
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 50
	}
	args = append(args, pageSize)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list content items: %w", err)
	}
	defer rows.Close()

	var results []*ContentItemRecord
	for rows.Next() {
		var rec ContentItemRecord
		var metadataJSON []byte
		err := rows.Scan(
			&rec.ID,
			&rec.TenantID,
			&rec.SourceSystem,
			&rec.ContentID,
			&rec.ProcessingStatus,
			&rec.ContentSize,
			&rec.CreatedAt,
			&rec.UpdatedAt,
			&rec.EmbeddingCount,
			&rec.AssertionCount,
			&rec.FailureCategory,
			&rec.FailureReason,
			&metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan content item: %w", err)
		}
		// Parse metadata JSON
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &rec.Metadata); err != nil {
				r.logger.Warn("Failed to parse metadata JSON", logging.Err(err), logging.F("content_id", rec.ContentID))
				rec.Metadata = make(map[string]interface{})
			}
		}
		results = append(results, &rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating content items: %w", err)
	}

	return results, nil
}

// DeleteByContentID soft-deletes a single source by content_id.
func (r *repositoryImpl) DeleteByContentID(ctx context.Context, contentID string) error {
	query := `
		UPDATE sources
		SET is_deleted = true, deleted_at = NOW()
		WHERE content_id = $1 AND (is_deleted IS NULL OR is_deleted = false)
	`

	result, err := r.db.Exec(ctx, query, contentID)
	if err != nil {
		return fmt.Errorf("failed to delete content item: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("content item not found or already deleted")
	}

	return nil
}

// DeleteByFilters bulk soft-deletes sources matching filters.
func (r *repositoryImpl) DeleteByFilters(ctx context.Context, tenantID string, sourceType, processingStatus *string) (int64, []string, error) {
	// Build WHERE clause
	whereClauses := []string{"(is_deleted IS NULL OR is_deleted = false)"}
	args := []interface{}{}
	argCount := 1

	// Tenant filter (required)
	whereClauses = append(whereClauses, fmt.Sprintf("tenant_id = $%d", argCount))
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid tenant_id: %w", err)
	}
	args = append(args, tenantUUID)
	argCount++

	// Optional source_type filter
	if sourceType != nil && *sourceType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("source_system = $%d", argCount))
		args = append(args, *sourceType)
		argCount++
	}

	// Optional processing_status filter
	if processingStatus != nil && *processingStatus != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("COALESCE(processing_status, 'pending') = $%d", argCount))
		args = append(args, *processingStatus)
		argCount++
	}

	// First, get the content_ids that will be deleted
	selectQuery := fmt.Sprintf(`
		SELECT content_id
		FROM sources
		WHERE %s
	`, joinWhere(whereClauses))

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to query content items for deletion: %w", err)
	}

	var deletedIDs []string
	for rows.Next() {
		var contentID string
		if err := rows.Scan(&contentID); err != nil {
			rows.Close()
			return 0, nil, fmt.Errorf("failed to scan content_id: %w", err)
		}
		deletedIDs = append(deletedIDs, contentID)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("error iterating content items: %w", err)
	}

	// Now perform the soft delete
	updateQuery := fmt.Sprintf(`
		UPDATE sources
		SET is_deleted = true, deleted_at = NOW()
		WHERE %s
	`, joinWhere(whereClauses))

	result, err := r.db.Exec(ctx, updateQuery, args...)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to delete content items: %w", err)
	}

	return result.RowsAffected(), deletedIDs, nil
}

// GetStats retrieves aggregate statistics for content.
func (r *repositoryImpl) GetStats(ctx context.Context, tenantID string) (*StatsRecord, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant_id: %w", err)
	}

	stats := &StatsRecord{
		CountByType:   make(map[string]int64),
		CountByStatus: make(map[string]int64),
	}

	// Total count and storage
	query := `
		SELECT
			COUNT(*) AS total_count,
			COALESCE(SUM(content_size), 0) AS total_storage_bytes
		FROM sources
		WHERE tenant_id = $1 AND (is_deleted IS NULL OR is_deleted = false)
	`
	err = r.db.QueryRow(ctx, query, tenantUUID).Scan(&stats.TotalCount, &stats.TotalStorageBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to get total stats: %w", err)
	}

	// Count by source_system
	query = `
		SELECT source_system, COUNT(*) AS count
		FROM sources
		WHERE tenant_id = $1 AND (is_deleted IS NULL OR is_deleted = false)
		GROUP BY source_system
	`
	rows, err := r.db.Query(ctx, query, tenantUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get count by type: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sourceSystem string
		var count int64
		if err := rows.Scan(&sourceSystem, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count by type: %w", err)
		}
		stats.CountByType[sourceSystem] = count
	}
	rows.Close()

	// Count by processing_status
	query = `
		SELECT COALESCE(processing_status, 'pending') AS status, COUNT(*) AS count
		FROM sources
		WHERE tenant_id = $1 AND (is_deleted IS NULL OR is_deleted = false)
		GROUP BY COALESCE(processing_status, 'pending')
	`
	rows, err = r.db.Query(ctx, query, tenantUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get count by status: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count by status: %w", err)
		}
		stats.CountByStatus[status] = count
	}
	rows.Close()

	// Count of items with embeddings
	query = `
		SELECT COUNT(DISTINCT s.id) AS embedded_count
		FROM sources s
		INNER JOIN embeddings e ON s.id = e.source_id
		WHERE s.tenant_id = $1 AND (s.is_deleted IS NULL OR s.is_deleted = false)
	`
	err = r.db.QueryRow(ctx, query, tenantUUID).Scan(&stats.EmbeddedCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedded count: %w", err)
	}

	return stats, nil
}

// GetContentText retrieves raw content text and metadata for a content item.
func (r *repositoryImpl) GetContentText(ctx context.Context, contentID string) (*ContentTextRecord, error) {
	query := `
		SELECT
			s.content_id,
			s.source_system AS content_type,
			COALESCE(s.raw_content, '') AS text,
			s.created_at,
			s.ingestion_metadata
		FROM sources s
		WHERE s.content_id = $1 AND (s.is_deleted IS NULL OR s.is_deleted = false)
	`

	var rec ContentTextRecord
	var metadataJSON []byte
	err := r.db.QueryRow(ctx, query, contentID).Scan(
		&rec.ContentID,
		&rec.ContentType,
		&rec.Text,
		&rec.CreatedAt,
		&metadataJSON,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get content text: %w", err)
	}

	// Parse metadata JSON
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &rec.Metadata); err != nil {
			r.logger.Warn("Failed to parse metadata JSON", logging.Err(err), logging.F("content_id", contentID))
			rec.Metadata = make(map[string]interface{})
		}
	} else {
		rec.Metadata = make(map[string]interface{})
	}

	return &rec, nil
}

// ListAvailableInsights retrieves available insights for a content item.
func (r *repositoryImpl) ListAvailableInsights(ctx context.Context, contentID string) (*InsightsAvailabilityRecord, error) {
	// First, get the content type from sources
	var contentType string
	err := r.db.QueryRow(ctx, `
		SELECT source_system
		FROM sources
		WHERE content_id = $1 AND (is_deleted IS NULL OR is_deleted = false)
	`, contentID).Scan(&contentType)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get content type: %w", err)
	}

	// Get available insight types from registry
	availableQuery := `
		SELECT insight_type
		FROM insight_type_registry
		WHERE content_type = $1
		ORDER BY display_order
	`
	rows, err := r.db.Query(ctx, availableQuery, contentType)
	if err != nil {
		return nil, fmt.Errorf("failed to query available insights: %w", err)
	}
	defer rows.Close()

	var available []string
	for rows.Next() {
		var insightType string
		if err := rows.Scan(&insightType); err != nil {
			return nil, fmt.Errorf("failed to scan insight type: %w", err)
		}
		available = append(available, insightType)
	}
	rows.Close()

	// Get extracted insight types from content_insights
	extractedQuery := `
		SELECT insight_type
		FROM content_insights
		WHERE content_id = $1
		ORDER BY extracted_at DESC
	`
	rows, err = r.db.Query(ctx, extractedQuery, contentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query extracted insights: %w", err)
	}
	defer rows.Close()

	var extracted []string
	extractedMap := make(map[string]bool)
	for rows.Next() {
		var insightType string
		if err := rows.Scan(&insightType); err != nil {
			return nil, fmt.Errorf("failed to scan extracted insight type: %w", err)
		}
		extracted = append(extracted, insightType)
		extractedMap[insightType] = true
	}
	rows.Close()

	// Calculate pending (available but not extracted)
	var pending []string
	for _, t := range available {
		if !extractedMap[t] {
			pending = append(pending, t)
		}
	}

	return &InsightsAvailabilityRecord{
		ContentID:   contentID,
		ContentType: contentType,
		Available:   available,
		Extracted:   extracted,
		Pending:     pending,
	}, nil
}

// GetInsights retrieves cached insights for a content item.
func (r *repositoryImpl) GetInsights(ctx context.Context, contentID string, types []string) ([]*InsightRecord, error) {
	// Build query with optional type filter
	query := `
		SELECT
			insight_type,
			data,
			extracted_at,
			COALESCE(model_version, '') AS model_version
		FROM content_insights
		WHERE content_id = $1
	`

	args := []interface{}{contentID}

	// Add type filter if specified
	if len(types) > 0 {
		query += " AND insight_type = ANY($2)"
		args = append(args, types)
	}

	query += " ORDER BY extracted_at DESC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query insights: %w", err)
	}
	defer rows.Close()

	var insights []*InsightRecord
	for rows.Next() {
		var rec InsightRecord
		var dataJSON []byte
		err := rows.Scan(
			&rec.Type,
			&dataJSON,
			&rec.ExtractedAt,
			&rec.ModelVersion,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan insight: %w", err)
		}

		// Parse data JSON
		if len(dataJSON) > 0 {
			if err := json.Unmarshal(dataJSON, &rec.Data); err != nil {
				r.logger.Warn("Failed to parse insight data JSON",
					logging.Err(err),
					logging.F("content_id", contentID),
					logging.F("insight_type", rec.Type),
				)
				rec.Data = make(map[string]interface{})
			}
		} else {
			rec.Data = make(map[string]interface{})
		}

		insights = append(insights, &rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating insights: %w", err)
	}

	return insights, nil
}

// joinWhere joins WHERE clauses with AND.
func joinWhere(clauses []string) string {
	result := ""
	for i, clause := range clauses {
		if i > 0 {
			result += " AND "
		}
		result += clause
	}
	return result
}

// Service implements the ContentProcessorService gRPC server.
type Service struct {
	contentv1.UnimplementedContentProcessorServiceServer
	repo           Repository
	tenantRepo     *tenant.Repository
	pipelineRepo   *pipeline.Repository
	temporalClient client.Client
	logger         logging.Logger
	langfuseClient *langfuse.Client
}

// NewService creates a new content service.
func NewService(db *pgxpool.Pool, tenantRepo *tenant.Repository, logger logging.Logger, langfuseClient *langfuse.Client) *Service {
	return &Service{
		repo:           newRepository(db, logger),
		tenantRepo:     tenantRepo,
		logger:         logger,
		langfuseClient: langfuseClient,
	}
}

// SetPipelineRepo sets the pipeline repository (for reprocessing).
func (s *Service) SetPipelineRepo(repo *pipeline.Repository) {
	s.pipelineRepo = repo
}

// SetTemporalClient sets the Temporal client (for workflow execution).
func (s *Service) SetTemporalClient(client client.Client) {
	s.temporalClient = client
}

// resolveTenantID resolves a tenant reference (UUID or slug) to a UUID.
func (s *Service) resolveTenantID(ctx context.Context, tenantRef string) (string, error) {
	if tenantRef == "" {
		return "", status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Try to resolve via tenant repository
	t, err := s.tenantRepo.GetByRef(ctx, tenantRef)
	if err != nil {
		s.logger.Error("Error resolving tenant", logging.Err(err), logging.F("tenant_ref", tenantRef))
		return "", status.Errorf(codes.Internal, "failed to resolve tenant: %v", err)
	}
	if t == nil {
		return "", status.Errorf(codes.NotFound, "tenant not found: %s", tenantRef)
	}

	return t.ID, nil
}

// ProcessContent triggers the content processing pipeline for a specific item.
func (s *Service) ProcessContent(ctx context.Context, req *contentv1.ProcessContentRequest) (*contentv1.ProcessContentResponse, error) {
	s.logger.Debug("ProcessContent called",
		logging.F("content_id", req.ContentId),
	)

	// TODO: Implement processing logic - trigger worker via Temporal
	return nil, status.Error(codes.Unimplemented, "ProcessContent not yet implemented")
}

// GetProcessingStatus retrieves the current processing status of a content item.
func (s *Service) GetProcessingStatus(ctx context.Context, req *contentv1.GetProcessingStatusRequest) (*contentv1.ProcessingStatus, error) {
	s.logger.Debug("GetProcessingStatus called",
		logging.F("content_id", req.ContentId),
		logging.F("job_id", req.JobId),
	)

	// TODO: Implement status retrieval from database
	return nil, status.Error(codes.Unimplemented, "GetProcessingStatus not yet implemented")
}

// GetContentItem retrieves a specific content item by ID.
func (s *Service) GetContentItem(ctx context.Context, req *contentv1.GetContentItemRequest) (*contentv1.ContentItem, error) {
	s.logger.Debug("GetContentItem called",
		logging.F("content_id", req.ContentId),
		logging.F("include_embedding", req.IncludeEmbedding),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	rec, err := s.repo.GetByContentID(ctx, req.ContentId)
	if err != nil {
		s.logger.Error("Failed to get content item",
			logging.Err(err),
			logging.F("content_id", req.ContentId),
		)
		return nil, status.Errorf(codes.Internal, "failed to get content item: %v", err)
	}

	if rec == nil {
		return nil, status.Errorf(codes.NotFound, "content item not found: %s", req.ContentId)
	}

	return recordToProto(rec), nil
}

// ListContentItems returns a paginated list of content items.
func (s *Service) ListContentItems(ctx context.Context, req *contentv1.ListContentItemsRequest) (*contentv1.ListContentItemsResponse, error) {
	s.logger.Debug("ListContentItems called",
		logging.F("tenant_id", req.TenantId),
		logging.F("source_type", req.SourceType),
		logging.F("state", req.State),
		logging.F("page_size", req.PageSize),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Resolve tenant reference to UUID
	tenantID, err := s.resolveTenantID(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	// Build filter
	filter := ListFilter{
		TenantID:  tenantID,
		PageSize:  int(req.PageSize),
		PageToken: req.PageToken,
	}

	if req.SourceType != nil {
		sourceType := *req.SourceType
		filter.SourceType = &sourceType
	}

	if req.State != nil {
		statusStr := stateToDBStatus(*req.State)
		filter.ProcessingStatus = &statusStr
	}

	records, err := s.repo.ListByTenant(ctx, filter)
	if err != nil {
		s.logger.Error("Failed to list content items",
			logging.Err(err),
			logging.F("tenant_id", req.TenantId),
		)
		return nil, status.Errorf(codes.Internal, "failed to list content items: %v", err)
	}

	// Convert to proto
	items := make([]*contentv1.ContentItem, len(records))
	for i, rec := range records {
		items[i] = recordToProto(rec)
	}

	return &contentv1.ListContentItemsResponse{
		Items:         items,
		NextPageToken: "", // TODO: Implement pagination tokens if needed
	}, nil
}

// ReprocessContent triggers reprocessing of an already-processed content item.
func (s *Service) ReprocessContent(ctx context.Context, req *contentv1.ReprocessContentRequest) (*contentv1.ReprocessContentResponse, error) {
	s.logger.Info("ReprocessContent called",
		logging.F("content_id", req.ContentId),
		logging.F("reason", req.Reason),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// Check if pipeline repository is available
	if s.pipelineRepo == nil {
		return nil, status.Error(codes.Unavailable, "pipeline repository not configured")
	}

	// Check if Temporal client is available
	if s.temporalClient == nil {
		return nil, status.Error(codes.Unavailable, "Temporal client not configured")
	}

	// Look up source by content_id
	source, err := s.pipelineRepo.GetSourceByContentID(ctx, req.ContentId)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "content not found: %s", req.ContentId)
		}
		s.logger.Error("Error getting source by content_id", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get source: %v", err)
	}

	// Reset processing_status to 'pending' for this specific source
	err = s.pipelineRepo.ResetSourceStatus(ctx, source.ID)
	if err != nil {
		s.logger.Error("Error resetting processing status", logging.Err(err), logging.F("source_id", source.ID))
		return nil, status.Errorf(codes.Internal, "failed to reset processing status: %v", err)
	}

	// Start ContentIngestionWorkflow via Temporal
	workflowID := pkgtemporal.GenerateIngestWorkflowID(source.TenantID, source.SourceSystem, strconv.FormatInt(source.ID, 10))
	input := pkgtemporal.SLMPipelineInput{
		TenantID:    source.TenantID,
		SourceID:    source.ID,
		ContentID:   source.ContentID,
		ContentHash: source.ContentHash,
	}
	opts := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "penfold-main",
	}
	workflowRun, err := s.temporalClient.ExecuteWorkflow(ctx, opts, "SLMPipelineWorkflow", input)
	if err != nil {
		s.logger.Error("Failed to start workflow",
			logging.F("source_id", source.ID),
			logging.F("workflow_id", workflowID),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to start reprocessing workflow: %v", err)
	}

	// Get the run ID to use as job ID
	jobID := workflowRun.GetRunID()

	s.logger.Info("Started ContentIngestionWorkflow for reprocessing",
		logging.F("workflow_id", workflowID),
		logging.F("run_id", jobID),
		logging.F("source_id", source.ID),
		logging.F("content_id", source.ContentID),
		logging.F("reason", req.Reason),
	)

	return &contentv1.ReprocessContentResponse{
		ContentId: req.ContentId,
		JobId:     jobID,
		Status: &contentv1.ProcessingStatus{
			ContentId: req.ContentId,
			JobId:     jobID,
			State:     contentv1.ProcessingState_PROCESSING_STATE_PENDING,
		},
	}, nil
}

// DeleteContentItem removes a content item and all derived data.
func (s *Service) DeleteContentItem(ctx context.Context, req *contentv1.DeleteContentItemRequest) (*contentv1.DeleteContentItemResponse, error) {
	s.logger.Info("DeleteContentItem called",
		logging.F("content_id", req.ContentId),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// Soft delete the content item
	// Related data (embeddings, assertions, attachments, etc.) has CASCADE or SET NULL constraints
	err := s.repo.DeleteByContentID(ctx, req.ContentId)
	if err != nil {
		s.logger.Error("Failed to delete content item",
			logging.Err(err),
			logging.F("content_id", req.ContentId),
		)
		return nil, status.Errorf(codes.Internal, "failed to delete content item: %v", err)
	}

	s.logger.Info("Content item deleted",
		logging.F("content_id", req.ContentId),
	)

	return &contentv1.DeleteContentItemResponse{
		Success:   true,
		ContentId: req.ContentId,
	}, nil
}

// DeleteContentItems bulk deletes content items matching filters.
func (s *Service) DeleteContentItems(ctx context.Context, req *contentv1.DeleteContentItemsRequest) (*contentv1.DeleteContentItemsResponse, error) {
	s.logger.Info("DeleteContentItems called",
		logging.F("tenant_id", req.TenantId),
		logging.F("source_type", req.SourceType),
		logging.F("state", req.State),
		logging.F("confirm", req.Confirm),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	if !req.Confirm {
		return nil, status.Error(codes.InvalidArgument, "confirm must be true for bulk delete")
	}

	// Resolve tenant reference to UUID
	tenantID, err := s.resolveTenantID(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	// Build filters
	var sourceType *string
	if req.SourceType != nil {
		st := *req.SourceType
		sourceType = &st
	}

	var processingStatus *string
	if req.State != nil {
		statusStr := stateToDBStatus(*req.State)
		processingStatus = &statusStr
	}

	// Execute bulk delete
	count, deletedIDs, err := s.repo.DeleteByFilters(ctx, tenantID, sourceType, processingStatus)
	if err != nil {
		s.logger.Error("Failed to bulk delete content items",
			logging.Err(err),
			logging.F("tenant_id", req.TenantId),
		)
		return nil, status.Errorf(codes.Internal, "failed to delete content items: %v", err)
	}

	s.logger.Info("Bulk delete completed",
		logging.F("tenant_id", req.TenantId),
		logging.F("deleted_count", count),
	)

	return &contentv1.DeleteContentItemsResponse{
		DeletedCount: count,
		DeletedIds:   deletedIDs,
	}, nil
}

// GetContentStats returns content statistics.
func (s *Service) GetContentStats(ctx context.Context, req *contentv1.GetContentStatsRequest) (*contentv1.ContentStats, error) {
	s.logger.Debug("GetContentStats called",
		logging.F("tenant_id", req.TenantId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Resolve tenant reference to UUID
	tenantID, err := s.resolveTenantID(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	stats, err := s.repo.GetStats(ctx, tenantID)
	if err != nil {
		s.logger.Error("Failed to get content stats",
			logging.Err(err),
			logging.F("tenant_id", req.TenantId),
		)
		return nil, status.Errorf(codes.Internal, "failed to get content stats: %v", err)
	}

	// Get summary count
	summarizedCount, err := s.pipelineRepo.CountSummaries(ctx)
	if err != nil {
		s.logger.Error("Failed to count summaries",
			logging.Err(err),
			logging.F("tenant_id", req.TenantId),
		)
		summarizedCount = 0
	}

	// Get extraction count
	extractedCount, err := s.pipelineRepo.CountExtracted(ctx)
	if err != nil {
		s.logger.Error("Failed to count extracted sources",
			logging.Err(err),
			logging.F("tenant_id", req.TenantId),
		)
		extractedCount = 0
	}

	return &contentv1.ContentStats{
		TenantId:          req.TenantId,
		TotalCount:        stats.TotalCount,
		CountByType:       stats.CountByType,
		CountByState:      stats.CountByStatus,
		TotalStorageBytes: stats.TotalStorageBytes,
		EmbeddedCount:     stats.EmbeddedCount,
		SummarizedCount:   summarizedCount,
		ExtractedCount:    extractedCount,
	}, nil
}

// GetContentText retrieves the raw text content for a content item.
func (s *Service) GetContentText(ctx context.Context, req *contentv1.GetContentTextRequest) (*contentv1.GetContentTextResponse, error) {
	s.logger.Debug("GetContentText called",
		logging.F("content_id", req.ContentId),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	rec, err := s.repo.GetContentText(ctx, req.ContentId)
	if err != nil {
		s.logger.Error("Failed to get content text",
			logging.Err(err),
			logging.F("content_id", req.ContentId),
		)
		return nil, status.Errorf(codes.Internal, "failed to get content text: %v", err)
	}

	if rec == nil {
		return nil, status.Errorf(codes.NotFound, "content item not found: %s", req.ContentId)
	}

	// Convert metadata to string map
	metadata := make(map[string]string)
	for k, v := range rec.Metadata {
		switch val := v.(type) {
		case string:
			metadata[k] = val
		default:
			// JSON encode complex values
			if b, err := json.Marshal(val); err == nil {
				metadata[k] = string(b)
			}
		}
	}

	return &contentv1.GetContentTextResponse{
		ContentId:   rec.ContentID,
		ContentType: rec.ContentType,
		Text:        rec.Text,
		CreatedAt:   timestamppb.New(rec.CreatedAt),
		Metadata:    metadata,
	}, nil
}

// ListAvailableInsights returns available insight types for a content item.
func (s *Service) ListAvailableInsights(ctx context.Context, req *contentv1.ListAvailableInsightsRequest) (*contentv1.ListAvailableInsightsResponse, error) {
	s.logger.Debug("ListAvailableInsights called",
		logging.F("content_id", req.ContentId),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	rec, err := s.repo.ListAvailableInsights(ctx, req.ContentId)
	if err != nil {
		s.logger.Error("Failed to list available insights",
			logging.Err(err),
			logging.F("content_id", req.ContentId),
		)
		return nil, status.Errorf(codes.Internal, "failed to list available insights: %v", err)
	}

	if rec == nil {
		return nil, status.Errorf(codes.NotFound, "content item not found: %s", req.ContentId)
	}

	return &contentv1.ListAvailableInsightsResponse{
		ContentId:   rec.ContentID,
		ContentType: rec.ContentType,
		Available:   rec.Available,
		Extracted:   rec.Extracted,
		Pending:     rec.Pending,
	}, nil
}

// GetInsights retrieves cached insights for a content item.
func (s *Service) GetInsights(ctx context.Context, req *contentv1.GetInsightsRequest) (*contentv1.GetInsightsResponse, error) {
	s.logger.Debug("GetInsights called",
		logging.F("content_id", req.ContentId),
		logging.F("types", req.Types),
		logging.F("refresh", req.Refresh),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// Note: refresh flag is for future use to trigger async re-extraction
	// For now, we just return cached insights
	if req.Refresh {
		s.logger.Debug("Refresh requested but not implemented yet",
			logging.F("content_id", req.ContentId),
		)
	}

	insights, err := s.repo.GetInsights(ctx, req.ContentId, req.Types)
	if err != nil {
		s.logger.Error("Failed to get insights",
			logging.Err(err),
			logging.F("content_id", req.ContentId),
		)
		return nil, status.Errorf(codes.Internal, "failed to get insights: %v", err)
	}

	// Convert to proto
	protoInsights := make([]*contentv1.Insight, len(insights))
	for i, rec := range insights {
		// Convert data map to protobuf Struct
		dataStruct, err := convertToStruct(rec.Data)
		if err != nil {
			s.logger.Warn("Failed to convert insight data to Struct",
				logging.Err(err),
				logging.F("content_id", req.ContentId),
				logging.F("insight_type", rec.Type),
			)
			dataStruct = &structpb.Struct{Fields: make(map[string]*structpb.Value)}
		}

		protoInsights[i] = &contentv1.Insight{
			Type:          rec.Type,
			Data:          dataStruct,
			ExtractedAt:   timestamppb.New(rec.ExtractedAt),
			ModelVersion:  rec.ModelVersion,
		}
	}

	return &contentv1.GetInsightsResponse{
		ContentId: req.ContentId,
		Insights:  protoInsights,
	}, nil
}

// =============================================================================
// Conversion Helpers
// =============================================================================

// recordToProto converts a database record to a proto ContentItem.
func recordToProto(rec *ContentItemRecord) *contentv1.ContentItem {
	if rec == nil {
		return nil
	}

	// Build metadata map from source metadata
	metadata := make(map[string]string)
	for k, v := range rec.Metadata {
		switch val := v.(type) {
		case string:
			metadata[k] = val
		default:
			// JSON encode complex values (arrays, objects)
			if b, err := json.Marshal(val); err == nil {
				metadata[k] = string(b)
			}
		}
	}

	// Add derived counts
	metadata["embedding_count"] = fmt.Sprintf("%d", rec.EmbeddingCount)
	metadata["assertion_count"] = fmt.Sprintf("%d", rec.AssertionCount)

	item := &contentv1.ContentItem{
		Id:         rec.ContentID,
		SourceType: rec.SourceSystem,
		SourceId:   fmt.Sprintf("%d", rec.ID),
		TenantId:   rec.TenantID,
		State:      dbStatusToState(rec.ProcessingStatus),
		CreatedAt:  timestamppb.New(rec.CreatedAt),
		UpdatedAt:  timestamppb.New(rec.UpdatedAt),
		Metadata:   metadata,
	}

	// Include failure info for rejected/failed items
	if rec.FailureCategory != nil {
		item.FailureCategory = rec.FailureCategory
	}
	if rec.FailureReason != nil {
		item.FailureReason = rec.FailureReason
	}

	return item
}

// stateToDBStatus converts a proto ProcessingState to database status string.
func stateToDBStatus(state contentv1.ProcessingState) string {
	switch state {
	case contentv1.ProcessingState_PROCESSING_STATE_PENDING:
		return "pending"
	case contentv1.ProcessingState_PROCESSING_STATE_IN_PROGRESS:
		return "processing"
	case contentv1.ProcessingState_PROCESSING_STATE_COMPLETED:
		return "completed"
	case contentv1.ProcessingState_PROCESSING_STATE_FAILED:
		return "failed"
	case contentv1.ProcessingState_PROCESSING_STATE_CANCELLED:
		return "cancelled"
	case contentv1.ProcessingState_PROCESSING_STATE_REJECTED:
		return "rejected"
	case contentv1.ProcessingState_PROCESSING_STATE_SKIPPED:
		return "skipped"
	default:
		return "pending"
	}
}

// dbStatusToState converts a database status string to proto ProcessingState.
func dbStatusToState(status string) contentv1.ProcessingState {
	switch status {
	case "pending":
		return contentv1.ProcessingState_PROCESSING_STATE_PENDING
	case "processing":
		return contentv1.ProcessingState_PROCESSING_STATE_IN_PROGRESS
	case "completed":
		return contentv1.ProcessingState_PROCESSING_STATE_COMPLETED
	case "failed":
		return contentv1.ProcessingState_PROCESSING_STATE_FAILED
	case "cancelled":
		return contentv1.ProcessingState_PROCESSING_STATE_CANCELLED
	case "rejected":
		return contentv1.ProcessingState_PROCESSING_STATE_REJECTED
	case "skipped":
		return contentv1.ProcessingState_PROCESSING_STATE_SKIPPED
	default:
		return contentv1.ProcessingState_PROCESSING_STATE_PENDING
	}
}

// convertToStruct converts a map[string]interface{} to protobuf Struct.
func convertToStruct(data map[string]interface{}) (*structpb.Struct, error) {
	// Use structpb.NewStruct for proper conversion
	return structpb.NewStruct(data)
}

// GetContentTrace retrieves Langfuse traces associated with a content item.
func (s *Service) GetContentTrace(ctx context.Context, req *contentv1.GetContentTraceRequest) (*contentv1.GetContentTraceResponse, error) {
	s.logger.Debug("GetContentTrace called",
		logging.F("content_id", req.ContentId),
		logging.F("environment", req.Environment),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// Check if Langfuse is configured
	if s.langfuseClient == nil {
		// Return empty response if Langfuse is not configured
		s.logger.Debug("Langfuse not configured, returning empty trace response",
			logging.F("content_id", req.ContentId),
		)
		return &contentv1.GetContentTraceResponse{
			ContentId:   req.ContentId,
			Traces:      []*contentv1.LangfuseTrace{},
			LangfuseUrl: "",
		}, nil
	}

	// Get environment filter
	environment := ""
	if req.Environment != nil {
		environment = *req.Environment
	}

	// Query Langfuse API for traces
	traces, err := s.langfuseClient.GetTracesByContentID(ctx, req.ContentId, environment)
	if err != nil {
		s.logger.Error("Failed to get Langfuse traces",
			logging.Err(err),
			logging.F("content_id", req.ContentId),
		)
		return nil, status.Errorf(codes.Internal, "failed to get traces: %v", err)
	}

	// Convert traces to proto format
	protoTraces := make([]*contentv1.LangfuseTrace, len(traces))
	for i, trace := range traces {
		// Get observations for this trace
		observations, err := s.langfuseClient.GetObservations(ctx, trace.ID)
		if err != nil {
			s.logger.Warn("Failed to get observations for trace",
				logging.Err(err),
				logging.F("trace_id", trace.ID),
			)
			observations = []langfuse.Observation{}
		}

		// Convert observations to proto
		protoObservations := make([]*contentv1.LangfuseObservation, len(observations))
		for j, obs := range observations {
			protoObs := &contentv1.LangfuseObservation{
				Id:        obs.ID,
				Type:      obs.Type,
				Name:      obs.Name,
				StartTime: timestamppb.New(obs.StartTime),
				Status:    obs.Level,
			}

			// Add optional fields
			if obs.EndTime != nil {
				protoObs.EndTime = timestamppb.New(*obs.EndTime)
			}
			if obs.Model != nil {
				protoObs.Model = obs.Model
			}
			if obs.StatusMessage != nil {
				protoObs.Error = obs.StatusMessage
			}

			// Token counts - check both old and new field names
			if obs.PromptTokens != nil {
				inputTokens := int32(*obs.PromptTokens)
				protoObs.InputTokens = &inputTokens
			} else if obs.UsageDetails.Input != nil {
				inputTokens := int32(*obs.UsageDetails.Input)
				protoObs.InputTokens = &inputTokens
			}

			if obs.CompletionTokens != nil {
				outputTokens := int32(*obs.CompletionTokens)
				protoObs.OutputTokens = &outputTokens
			} else if obs.UsageDetails.Output != nil {
				outputTokens := int32(*obs.UsageDetails.Output)
				protoObs.OutputTokens = &outputTokens
			}

			if obs.TotalTokens != nil {
				totalTokens := int32(*obs.TotalTokens)
				protoObs.TotalTokens = &totalTokens
			} else if obs.UsageDetails.Total != nil {
				totalTokens := int32(*obs.UsageDetails.Total)
				protoObs.TotalTokens = &totalTokens
			}

			protoObservations[j] = protoObs
		}

		protoTraces[i] = &contentv1.LangfuseTrace{
			TraceId:      trace.ID,
			Name:         trace.Name,
			StartTime:    timestamppb.New(trace.Timestamp),
			Status:       "completed", // Langfuse traces don't have explicit status
			Observations: protoObservations,
		}
	}

	// Build Langfuse UI URL
	langfuseURL := s.langfuseClient.BuildFilterURL(req.ContentId)

	return &contentv1.GetContentTraceResponse{
		ContentId:   req.ContentId,
		Traces:      protoTraces,
		LangfuseUrl: langfuseURL,
	}, nil
}
