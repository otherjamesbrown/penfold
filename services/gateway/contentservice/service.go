// Package contentservice implements the ContentProcessorService gRPC server.
package contentservice

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tenant"
)

// Repository defines the interface for content storage operations.
type Repository interface {
	GetByContentID(ctx context.Context, contentID string) (*ContentItemRecord, error)
	ListByTenant(ctx context.Context, filter ListFilter) ([]*ContentItemRecord, error)
	DeleteByContentID(ctx context.Context, contentID string) error
	DeleteByFilters(ctx context.Context, tenantID string, sourceType, processingStatus *string) (int64, []string, error)
	GetStats(ctx context.Context, tenantID string) (*StatsRecord, error)
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
			COUNT(DISTINCT a.id) AS assertion_count
		FROM sources s
		LEFT JOIN embeddings e ON s.id = e.source_id
		LEFT JOIN assertions a ON s.id = a.source_id
		WHERE s.content_id = $1 AND (s.is_deleted IS NULL OR s.is_deleted = false)
		GROUP BY s.id
	`

	var rec ContentItemRecord
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
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get content item: %w", err)
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
			COUNT(DISTINCT a.id) AS assertion_count
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
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan content item: %w", err)
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
	repo       Repository
	tenantRepo *tenant.Repository
	logger     logging.Logger
}

// NewService creates a new content service.
func NewService(db *pgxpool.Pool, tenantRepo *tenant.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:       newRepository(db, logger),
		tenantRepo: tenantRepo,
		logger:     logger,
	}
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
	s.logger.Debug("ReprocessContent called",
		logging.F("content_id", req.ContentId),
		logging.F("reason", req.Reason),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// TODO: Implement reprocessing logic
	return nil, status.Error(codes.Unimplemented, "ReprocessContent not yet implemented")
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

	return &contentv1.ContentStats{
		TenantId:          req.TenantId,
		TotalCount:        stats.TotalCount,
		CountByType:       stats.CountByType,
		CountByState:      stats.CountByStatus,
		TotalStorageBytes: stats.TotalStorageBytes,
		EmbeddedCount:     stats.EmbeddedCount,
		SummarizedCount:   0, // Not tracked yet
		ExtractedCount:    0, // Not tracked yet
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

	return &contentv1.ContentItem{
		Id:         rec.ContentID,
		SourceType: rec.SourceSystem,
		SourceId:   fmt.Sprintf("%d", rec.ID),
		TenantId:   rec.TenantID,
		State:      dbStatusToState(rec.ProcessingStatus),
		CreatedAt:  timestamppb.New(rec.CreatedAt),
		UpdatedAt:  timestamppb.New(rec.UpdatedAt),
		Metadata: map[string]string{
			"embedding_count": fmt.Sprintf("%d", rec.EmbeddingCount),
			"assertion_count": fmt.Sprintf("%d", rec.AssertionCount),
		},
	}
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
	default:
		return contentv1.ProcessingState_PROCESSING_STATE_PENDING
	}
}
