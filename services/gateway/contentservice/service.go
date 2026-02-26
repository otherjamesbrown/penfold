// Package contentservice implements the ContentProcessorService gRPC server.
package contentservice

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	pkgconfig "github.com/otherjamesbrown/penfold/pkg/config"
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
	PurgeByContentID(ctx context.Context, contentID string) error
	PurgeByFilters(ctx context.Context, tenantID string, sourceType *string, limit int) (int64, []string, error)
	GetStats(ctx context.Context, tenantID string) (*StatsRecord, error)
	GetContentText(ctx context.Context, contentID string) (*ContentTextRecord, error)
	ListAvailableInsights(ctx context.Context, contentID string) (*InsightsAvailabilityRecord, error)
	GetInsights(ctx context.Context, contentID string, types []string) ([]*InsightRecord, error)
	GetAssertions(ctx context.Context, contentID string, assertionType *string) ([]*AssertionRecord, error)
	ClearErrorByContentID(ctx context.Context, contentID string) error
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
	LangfuseTraceID  *string
	Metadata         map[string]interface{}

	// Enrichment fields (from LEFT JOIN content_enrichment).
	EnrichmentContentType     *string // content_enrichment.content_type
	EnrichmentContentSubtype  *string // content_enrichment.content_subtype
	EnrichmentSourceSystem    *string // content_enrichment.source_system
	EnrichmentContentStructure *string // content_enrichment.content_structure
}

// ListFilter represents filter criteria for listing content items.
type ListFilter struct {
	TenantID         string
	SourceType       *string
	ProcessingStatus *string
	ContentType      *contentv1.ContentType // filter by content type enum
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

// AssertionRecord represents a single extracted assertion.
type AssertionRecord struct {
	ID              int64
	AssertionType   string
	Description     string
	SourceQuote     *string
	Confidence      *float32
	ExtractionModel *string
	CreatedAt       time.Time
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
			s.langfuse_trace_id,
			s.ingestion_metadata,
			ce.content_type AS enrichment_content_type,
			ce.content_subtype AS enrichment_content_subtype,
			ce.source_system AS enrichment_source_system,
			ce.content_structure AS enrichment_content_structure
		FROM sources s
		LEFT JOIN embeddings e ON s.id = e.source_id
		LEFT JOIN assertions a ON s.id = a.source_id
		LEFT JOIN content_enrichment ce ON s.id = ce.source_id
		WHERE s.content_id = $1 AND (s.is_deleted IS NULL OR s.is_deleted = false)
		GROUP BY s.id, ce.content_type, ce.content_subtype, ce.source_system, ce.content_structure
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
		&rec.LangfuseTraceID,
		&metadataJSON,
		&rec.EnrichmentContentType,
		&rec.EnrichmentContentSubtype,
		&rec.EnrichmentSourceSystem,
		&rec.EnrichmentContentStructure,
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

	// Optional content_type filter (maps proto enum to DB string via content_enrichment).
	if filter.ContentType != nil {
		dbContentType := protoContentTypeToDBString(*filter.ContentType)
		if dbContentType != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("ce.content_type = $%d", argCount))
			args = append(args, dbContentType)
			argCount++
		}
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
			s.langfuse_trace_id,
			s.ingestion_metadata,
			ce.content_type AS enrichment_content_type,
			ce.content_subtype AS enrichment_content_subtype,
			ce.source_system AS enrichment_source_system,
			ce.content_structure AS enrichment_content_structure
		FROM sources s
		LEFT JOIN embeddings e ON s.id = e.source_id
		LEFT JOIN assertions a ON s.id = a.source_id
		LEFT JOIN content_enrichment ce ON s.id = ce.source_id
		WHERE %s
		GROUP BY s.id, ce.content_type, ce.content_subtype, ce.source_system, ce.content_structure
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
			&rec.LangfuseTraceID,
			&metadataJSON,
			&rec.EnrichmentContentType,
			&rec.EnrichmentContentSubtype,
			&rec.EnrichmentSourceSystem,
			&rec.EnrichmentContentStructure,
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

// PurgeByContentID hard-deletes a source and all related data by content_id.
// Only purges sources that are already soft-deleted (is_deleted = true).
func (r *repositoryImpl) PurgeByContentID(ctx context.Context, contentID string) error {
	// Start transaction
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Look up the source and verify it's soft-deleted
	var sourceID int64
	var isDeleted bool
	err = tx.QueryRow(ctx, `
		SELECT id, COALESCE(is_deleted, false)
		FROM sources
		WHERE content_id = $1
	`, contentID).Scan(&sourceID, &isDeleted)

	if err == pgx.ErrNoRows {
		return fmt.Errorf("content item not found: %s", contentID)
	}
	if err != nil {
		return fmt.Errorf("failed to look up source: %w", err)
	}

	// Safety check: only purge soft-deleted sources
	if !isDeleted {
		return fmt.Errorf("content item not soft-deleted: %s (soft-delete first)", contentID)
	}

	// Delete in FK dependency order
	// 1. Delete pipeline_runs
	_, err = tx.Exec(ctx, "DELETE FROM pipeline_runs WHERE source_id = $1", sourceID)
	if err != nil {
		return fmt.Errorf("failed to delete pipeline_runs: %w", err)
	}

	// 2. Delete embeddings referencing assertions from this source
	_, err = tx.Exec(ctx, `
		DELETE FROM embeddings
		WHERE assertion_id IN (SELECT id FROM assertions WHERE source_id = $1)
	`, sourceID)
	if err != nil {
		return fmt.Errorf("failed to delete assertion embeddings: %w", err)
	}

	// 3. Delete embeddings referencing this source directly
	_, err = tx.Exec(ctx, "DELETE FROM embeddings WHERE source_id = $1", sourceID)
	if err != nil {
		return fmt.Errorf("failed to delete source embeddings: %w", err)
	}

	// 4. Delete assertions (CASCADE will handle assertion_references)
	_, err = tx.Exec(ctx, "DELETE FROM assertions WHERE source_id = $1", sourceID)
	if err != nil {
		return fmt.Errorf("failed to delete assertions: %w", err)
	}

	// 5. Delete review_items
	_, err = tx.Exec(ctx, "DELETE FROM review_items WHERE source_id = $1", sourceID)
	if err != nil {
		return fmt.Errorf("failed to delete review_items: %w", err)
	}

	// 6. Update relationship_evidence (SET NULL via FK, but do it explicitly for clarity)
	_, err = tx.Exec(ctx, "UPDATE relationship_evidence SET source_id = NULL WHERE source_id = $1", sourceID)
	if err != nil {
		return fmt.Errorf("failed to update relationship_evidence: %w", err)
	}

	// 7. CASCADE handles: archived_files, email_attachments, meeting_mentions
	// 8. Delete the source itself
	result, err := tx.Exec(ctx, "DELETE FROM sources WHERE id = $1", sourceID)
	if err != nil {
		return fmt.Errorf("failed to delete source: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("source not found for deletion (concurrent modification?)")
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	r.logger.Info("Purged content item",
		logging.F("content_id", contentID),
		logging.F("source_id", sourceID),
	)

	return nil
}

// PurgeByFilters bulk hard-deletes sources matching filters.
// Only purges sources that are already soft-deleted (is_deleted = true).
func (r *repositoryImpl) PurgeByFilters(ctx context.Context, tenantID string, sourceType *string, limit int) (int64, []string, error) {
	// Build WHERE clause
	whereClauses := []string{"is_deleted = true"}
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

	// Apply limit for safety
	if limit <= 0 {
		limit = 100 // default safety limit
	}

	// Get the sources to purge
	selectQuery := fmt.Sprintf(`
		SELECT id, content_id
		FROM sources
		WHERE %s
		ORDER BY id
		LIMIT $%d
	`, joinWhere(whereClauses), argCount)
	args = append(args, limit)

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to query sources for purge: %w", err)
	}

	type sourceRecord struct {
		ID        int64
		ContentID string
	}

	var sourcesToPurge []sourceRecord
	for rows.Next() {
		var rec sourceRecord
		if err := rows.Scan(&rec.ID, &rec.ContentID); err != nil {
			rows.Close()
			return 0, nil, fmt.Errorf("failed to scan source: %w", err)
		}
		sourcesToPurge = append(sourcesToPurge, rec)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("error iterating sources: %w", err)
	}

	if len(sourcesToPurge) == 0 {
		return 0, []string{}, nil
	}

	// Collect source IDs for batch deletion
	sourceIDs := make([]int64, len(sourcesToPurge))
	contentIDs := make([]string, len(sourcesToPurge))
	for i, s := range sourcesToPurge {
		sourceIDs[i] = s.ID
		contentIDs[i] = s.ContentID
	}

	// Start transaction
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Delete in FK dependency order (batch operations)
	// 1. Delete pipeline_runs
	_, err = tx.Exec(ctx, "DELETE FROM pipeline_runs WHERE source_id = ANY($1)", sourceIDs)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to delete pipeline_runs: %w", err)
	}

	// 2. Delete embeddings referencing assertions from these sources
	_, err = tx.Exec(ctx, `
		DELETE FROM embeddings
		WHERE assertion_id IN (SELECT id FROM assertions WHERE source_id = ANY($1))
	`, sourceIDs)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to delete assertion embeddings: %w", err)
	}

	// 3. Delete embeddings referencing these sources directly
	_, err = tx.Exec(ctx, "DELETE FROM embeddings WHERE source_id = ANY($1)", sourceIDs)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to delete source embeddings: %w", err)
	}

	// 4. Delete assertions (CASCADE will handle assertion_references)
	_, err = tx.Exec(ctx, "DELETE FROM assertions WHERE source_id = ANY($1)", sourceIDs)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to delete assertions: %w", err)
	}

	// 5. Delete review_items
	_, err = tx.Exec(ctx, "DELETE FROM review_items WHERE source_id = ANY($1)", sourceIDs)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to delete review_items: %w", err)
	}

	// 6. Update relationship_evidence (SET NULL)
	_, err = tx.Exec(ctx, "UPDATE relationship_evidence SET source_id = NULL WHERE source_id = ANY($1)", sourceIDs)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to update relationship_evidence: %w", err)
	}

	// 7. CASCADE handles: archived_files, email_attachments, meeting_mentions
	// 8. Delete the sources themselves
	result, err := tx.Exec(ctx, "DELETE FROM sources WHERE id = ANY($1)", sourceIDs)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to delete sources: %w", err)
	}

	purgedCount := result.RowsAffected()

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return 0, nil, fmt.Errorf("committing transaction: %w", err)
	}

	r.logger.Info("Bulk purged content items",
		logging.F("tenant_id", tenantID),
		logging.F("purged_count", purgedCount),
	)

	return purgedCount, contentIDs, nil
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

// GetAssertions retrieves assertions for a content item.
func (r *repositoryImpl) GetAssertions(ctx context.Context, contentID string, assertionType *string) ([]*AssertionRecord, error) {
	// Build query with optional type filter
	query := `
		SELECT
			a.id,
			a.assertion_type::TEXT,
			a.description,
			a.source_quote,
			a.confidence,
			a.extraction_model,
			a.created_at
		FROM assertions a
		INNER JOIN sources s ON a.source_id = s.id
		WHERE s.content_id = $1 AND (s.is_deleted IS NULL OR s.is_deleted = false)
	`

	args := []interface{}{contentID}

	// Add type filter if specified
	if assertionType != nil && *assertionType != "" {
		query += " AND a.assertion_type::TEXT = $2"
		args = append(args, *assertionType)
	}

	query += " ORDER BY a.confidence DESC, a.created_at DESC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query assertions: %w", err)
	}
	defer rows.Close()

	var assertions []*AssertionRecord
	for rows.Next() {
		var rec AssertionRecord
		err := rows.Scan(
			&rec.ID,
			&rec.AssertionType,
			&rec.Description,
			&rec.SourceQuote,
			&rec.Confidence,
			&rec.ExtractionModel,
			&rec.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan assertion: %w", err)
		}

		assertions = append(assertions, &rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating assertions: %w", err)
	}

	return assertions, nil
}

// ClearErrorByContentID clears error fields from a source by content_id.
func (r *repositoryImpl) ClearErrorByContentID(ctx context.Context, contentID string) error {
	query := `
		UPDATE sources
		SET failure_category = NULL, failure_reason = NULL, updated_at = NOW()
		WHERE content_id = $1 AND (is_deleted IS NULL OR is_deleted = false)
	`

	result, err := r.db.Exec(ctx, query, contentID)
	if err != nil {
		return fmt.Errorf("failed to clear error fields: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("content item not found")
	}

	return nil
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
	db             *sql.DB
	logger         logging.Logger
	langfuseClient *langfuse.Client
}

// NewService creates a new content service.
func NewService(dbPool *pgxpool.Pool, tenantRepo *tenant.Repository, logger logging.Logger, langfuseClient *langfuse.Client, sqlDB *sql.DB) *Service {
	return &Service{
		repo:           newRepository(dbPool, logger),
		tenantRepo:     tenantRepo,
		db:             sqlDB,
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
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	if s.pipelineRepo == nil {
		return nil, status.Error(codes.Unavailable, "pipeline repository not configured")
	}

	// 1. Get the overall processing state from the content repo.
	rec, err := s.repo.GetByContentID(ctx, req.ContentId)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "content item not found: %s", req.ContentId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get content item: %v", err)
	}
	if rec == nil {
		return nil, status.Errorf(codes.NotFound, "content item not found: %s", req.ContentId)
	}

	// 2. Look up the source to get the source ID for pipeline run queries.
	source, err := s.pipelineRepo.GetSourceByContentID(ctx, req.ContentId)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Content item exists but has no pipeline source record yet.
			return &contentv1.ProcessingStatus{
				ContentId: req.ContentId,
				State:     dbStatusToState(rec.ProcessingStatus),
			}, nil
		}
		return nil, status.Errorf(codes.Internal, "failed to get pipeline source: %v", err)
	}

	// 3. Query all pipeline runs for this source (ordered by created_at DESC).
	runs, err := s.pipelineRepo.ListSourceHistory(ctx, source.ID, "")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch pipeline runs: %v", err)
	}

	// 4. Use the shared helper to build per-stage results and extract contribution info.
	// Since runs are ordered DESC, the first occurrence of each stage is the latest.
	ps := pipeline.BuildProcessingStatus(runs)

	// 5. Build the response using shared helper results.
	result := &contentv1.ProcessingStatus{
		ContentId: req.ContentId,
		SourceId:  source.ID,
		State:     dbStatusToState(rec.ProcessingStatus),
	}
	pipeline.ApplyToProtoProcessingStatus(ps, result)

	return result, nil
}

// dbRunStatusToStageStatus converts a pipeline_run status string to proto StageStatus.
func dbRunStatusToStageStatus(s string) contentv1.StageStatus {
	switch s {
	case "pending":
		return contentv1.StageStatus_STAGE_STATUS_PENDING
	case "running":
		return contentv1.StageStatus_STAGE_STATUS_RUNNING
	case "completed":
		return contentv1.StageStatus_STAGE_STATUS_COMPLETED
	case "failed":
		return contentv1.StageStatus_STAGE_STATUS_FAILED
	case "skipped":
		return contentv1.StageStatus_STAGE_STATUS_SKIPPED
	default:
		return contentv1.StageStatus_STAGE_STATUS_UNSPECIFIED
	}
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

	if req.ContentTypeFilter != nil {
		ct := *req.ContentTypeFilter
		filter.ContentType = &ct
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

	// Check concurrency limit before allowing reprocessing.
	// Reads pipeline.max_concurrent from pipeline_operational_config and counts in-flight
	// sources (processing_status = 'processing'). Returns ResourceExhausted if
	// at or over the limit — same logic as KickProcessing in pipelineservice.
	if s.db != nil {
		maxConcurrent, err := s.getMaxConcurrent(ctx)
		if err != nil {
			s.logger.Error("Error reading max_concurrent config", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to read concurrency config: %v", err)
		}

		inFlightCount, err := s.countInFlightSources(ctx)
		if err != nil {
			s.logger.Error("Error counting in-flight sources", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to count in-flight sources: %v", err)
		}

		if inFlightCount >= maxConcurrent {
			s.logger.Info("ReprocessContent rejected: concurrency limit reached",
				logging.F("content_id", req.ContentId),
				logging.F("in_flight_count", inFlightCount),
				logging.F("max_concurrent", maxConcurrent),
			)
			return nil, status.Errorf(codes.ResourceExhausted,
				"concurrency limit reached (%d/%d in-flight); retry when processing completes",
				inFlightCount, maxConcurrent,
			)
		}
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

	// Clear stale pipeline_run records from previous runs (pf-04a2de).
	// Without this, old SKIPPED/completed entries persist through reprocessing and mask
	// whether the new run actually executed each stage.
	deletedRuns, err := s.pipelineRepo.DeleteRunsBySource(ctx, source.ID)
	if err != nil {
		s.logger.Warn("Failed to clear stale pipeline runs on reprocess (non-fatal)",
			logging.Err(err), logging.F("source_id", source.ID))
	} else if deletedRuns > 0 {
		s.logger.Info("Cleared stale pipeline runs for reprocess",
			logging.F("source_id", source.ID),
			logging.F("deleted_runs", deletedRuns),
		)
	}

	// Read processing overrides from request options
	var modelOverride *string
	var timeoutOverride *int32
	if req.Options != nil {
		// Access the pointer fields directly to preserve optional semantics
		modelOverride = req.Options.ModelId
		timeoutOverride = req.Options.TimeoutSeconds
	}

	// Log overrides when they're set
	if modelOverride != nil {
		s.logger.Info("Reprocessing with model override",
			logging.F("source_id", source.ID),
			logging.F("model_id", *modelOverride),
		)
	}
	if timeoutOverride != nil {
		s.logger.Info("Reprocessing with timeout override",
			logging.F("source_id", source.ID),
			logging.F("timeout_seconds", *timeoutOverride),
		)
	}

	// Look up tenant name for Langfuse environment labelling (best-effort, non-blocking).
	var tenantName string
	if s.tenantRepo != nil {
		if t, err := s.tenantRepo.Get(ctx, source.TenantID); err == nil && t != nil {
			tenantName = t.Name
		}
	}

	// Start ContentIngestionWorkflow via Temporal
	workflowID := pkgtemporal.GenerateIngestWorkflowID(source.TenantID, source.SourceSystem, strconv.FormatInt(source.ID, 10))
	input := pkgtemporal.SLMPipelineInput{
		TenantID:        source.TenantID,
		TenantName:      tenantName,
		SourceID:        source.ID,
		ContentID:       source.ContentID,
		ContentHash:     source.ContentHash,
		JobID:           workflowID, // Use workflow ID as job ID for tracing
	}
	// TODO: Worker layer will add ModelOverride and TimeoutOverride fields to SLMPipelineInput
	// For now, overrides are read and logged but not passed to the workflow
	opts := client.StartWorkflowOptions{
		ID:                       workflowID,
		TaskQueue:                "penfold-main",
		WorkflowIDReusePolicy:    enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
		WorkflowIDConflictPolicy: enums.WORKFLOW_ID_CONFLICT_POLICY_TERMINATE_EXISTING,
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
			State:     contentv1.ProcessingState_PROCESSING_STATE_PENDING,
		},
	}, nil
}

// =============================================================================
// Helper functions for concurrency management
// =============================================================================

// getMaxConcurrent reads the max_concurrent value from pipeline_operational_config.
func (s *Service) getMaxConcurrent(ctx context.Context) (int, error) {
	var valueStr string
	err := s.db.QueryRowContext(ctx, `
		SELECT value
		FROM pipeline_operational_config
		WHERE tenant_id = $1 AND key = 'pipeline.max_concurrent'
	`, pkgconfig.DefaultTenantID().String()).Scan(&valueStr)

	if err == sql.ErrNoRows {
		// Return default value if not found
		return 1, nil
	}
	if err != nil {
		return 1, fmt.Errorf("failed to query max_concurrent: %w", err)
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return 1, fmt.Errorf("failed to parse max_concurrent as integer: %w", err)
	}

	return value, nil
}

// countInFlightSources counts sources with processing_status = 'processing'.
func (s *Service) countInFlightSources(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sources
		WHERE processing_status = 'processing' AND is_deleted = false
	`).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("failed to count in-flight sources: %w", err)
	}

	return count, nil
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

// PurgeContentItem hard-deletes a content item and all related data.
func (s *Service) PurgeContentItem(ctx context.Context, req *contentv1.PurgeContentItemRequest) (*contentv1.PurgeContentItemResponse, error) {
	s.logger.Info("PurgeContentItem called",
		logging.F("content_id", req.ContentId),
		logging.F("reason", req.Reason),
		logging.F("confirm", req.Confirm),
	)

	// Validate confirm flag
	if !req.Confirm {
		return nil, status.Error(codes.InvalidArgument, "confirm must be true for purge operation")
	}

	// Validate content_id
	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// Validate reason
	if req.Reason == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required for purge operation")
	}

	// Purge the content item
	err := s.repo.PurgeByContentID(ctx, req.ContentId)
	if err != nil {
		s.logger.Error("Failed to purge content item",
			logging.Err(err),
			logging.F("content_id", req.ContentId),
			logging.F("reason", req.Reason),
		)
		return nil, status.Errorf(codes.Internal, "failed to purge content item: %v", err)
	}

	s.logger.Info("Content item purged",
		logging.F("content_id", req.ContentId),
		logging.F("reason", req.Reason),
	)

	return &contentv1.PurgeContentItemResponse{
		Success:   true,
		Message:   "Content item permanently deleted",
		ContentId: req.ContentId,
	}, nil
}

// PurgeContentItems bulk hard-deletes content items matching filters.
func (s *Service) PurgeContentItems(ctx context.Context, req *contentv1.PurgeContentItemsRequest) (*contentv1.PurgeContentItemsResponse, error) {
	s.logger.Info("PurgeContentItems called",
		logging.F("tenant_id", req.TenantId),
		logging.F("source_type", req.SourceType),
		logging.F("reason", req.Reason),
		logging.F("confirm", req.Confirm),
		logging.F("limit", req.Limit),
	)

	// Validate confirm flag
	if !req.Confirm {
		return nil, status.Error(codes.InvalidArgument, "confirm must be true for bulk purge operation")
	}

	// Validate tenant_id
	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Validate reason
	if req.Reason == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required for purge operation")
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

	// Set limit (default to 100 if not specified or 0)
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}

	// Execute bulk purge
	count, purgedIDs, err := s.repo.PurgeByFilters(ctx, tenantID, sourceType, limit)
	if err != nil {
		s.logger.Error("Failed to bulk purge content items",
			logging.Err(err),
			logging.F("tenant_id", req.TenantId),
			logging.F("reason", req.Reason),
		)
		return nil, status.Errorf(codes.Internal, "failed to purge content items: %v", err)
	}

	s.logger.Info("Bulk purge completed",
		logging.F("tenant_id", req.TenantId),
		logging.F("purged_count", count),
		logging.F("reason", req.Reason),
	)

	message := fmt.Sprintf("Permanently deleted %d content items", count)

	return &contentv1.PurgeContentItemsResponse{
		PurgedCount: int32(count),
		ContentIds:  purgedIDs,
		Message:     message,
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
		Id: rec.ContentID,
		// SourceType: deprecated — use ContentTypeEnum/ContentSubtypeEnum instead
		SourceId: fmt.Sprintf("%d", rec.ID),
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
	if rec.LangfuseTraceID != nil {
		item.LangfuseTraceId = rec.LangfuseTraceID
	}

	// Populate content classification enums from enrichment data.
	if rec.EnrichmentContentType != nil {
		item.ContentTypeEnum = mapDBContentTypeToProto(*rec.EnrichmentContentType)
	}
	if rec.EnrichmentContentSubtype != nil {
		subtypeEnum, notifSource := mapDBContentSubtypeToProto(*rec.EnrichmentContentSubtype)
		item.ContentSubtypeEnum = subtypeEnum
		if notifSource != "" {
			item.NotificationSource = notifSource
		}
	}
	if rec.EnrichmentContentStructure != nil {
		item.ContentStructure = mapDBContentStructureToProto(*rec.EnrichmentContentStructure)
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

// =============================================================================
// Content classification enum mapping helpers
// =============================================================================

// mapDBContentTypeToProto maps content_enrichment.content_type string to proto enum.
func mapDBContentTypeToProto(dbType string) contentv1.ContentType {
	switch dbType {
	case "email":
		return contentv1.ContentType_CONTENT_TYPE_EMAIL
	case "meeting":
		return contentv1.ContentType_CONTENT_TYPE_MEETING
	case "calendar":
		return contentv1.ContentType_CONTENT_TYPE_CALENDAR
	case "document":
		return contentv1.ContentType_CONTENT_TYPE_DOCUMENT
	case "attachment":
		return contentv1.ContentType_CONTENT_TYPE_ATTACHMENT
	default:
		return contentv1.ContentType_CONTENT_TYPE_UNSPECIFIED
	}
}

// mapDBContentSubtypeToProto maps content_enrichment.content_subtype string to proto enum.
// Returns the subtype enum and, for notification subtypes, the notification source string.
func mapDBContentSubtypeToProto(dbSubtype string) (contentv1.ContentSubtype, string) {
	switch {
	case dbSubtype == "thread" || dbSubtype == "standalone" || dbSubtype == "forward":
		return contentv1.ContentSubtype_CONTENT_SUBTYPE_HUMAN, ""
	case strings.HasPrefix(dbSubtype, "notification/"):
		source := strings.TrimPrefix(dbSubtype, "notification/")
		return contentv1.ContentSubtype_CONTENT_SUBTYPE_NOTIFICATION, source
	case dbSubtype == "notification":
		return contentv1.ContentSubtype_CONTENT_SUBTYPE_NOTIFICATION, ""
	case dbSubtype == "auto_reply":
		return contentv1.ContentSubtype_CONTENT_SUBTYPE_AUTO_REPLY, ""
	case dbSubtype == "newsletter":
		return contentv1.ContentSubtype_CONTENT_SUBTYPE_NEWSLETTER, ""
	case dbSubtype == "invite":
		return contentv1.ContentSubtype_CONTENT_SUBTYPE_INVITE, ""
	case dbSubtype == "cancellation":
		return contentv1.ContentSubtype_CONTENT_SUBTYPE_CANCELLATION, ""
	case dbSubtype == "update":
		return contentv1.ContentSubtype_CONTENT_SUBTYPE_UPDATE, ""
	case dbSubtype == "response":
		return contentv1.ContentSubtype_CONTENT_SUBTYPE_RESPONSE, ""
	case dbSubtype == "transcript":
		return contentv1.ContentSubtype_CONTENT_SUBTYPE_TRANSCRIPT, ""
	default:
		return contentv1.ContentSubtype_CONTENT_SUBTYPE_UNSPECIFIED, ""
	}
}

// mapDBContentStructureToProto maps content_enrichment.content_structure string to proto enum.
func mapDBContentStructureToProto(dbStructure string) contentv1.ContentStructure {
	switch dbStructure {
	case "standalone":
		return contentv1.ContentStructure_CONTENT_STRUCTURE_STANDALONE
	case "reply":
		return contentv1.ContentStructure_CONTENT_STRUCTURE_REPLY
	case "forward":
		return contentv1.ContentStructure_CONTENT_STRUCTURE_FORWARD
	default:
		return contentv1.ContentStructure_CONTENT_STRUCTURE_UNSPECIFIED
	}
}

// protoContentTypeToDBString maps proto ContentType enum to the DB string used in
// content_enrichment.content_type for query filtering.
func protoContentTypeToDBString(ct contentv1.ContentType) string {
	switch ct {
	case contentv1.ContentType_CONTENT_TYPE_EMAIL:
		return "email"
	case contentv1.ContentType_CONTENT_TYPE_MEETING:
		return "meeting"
	case contentv1.ContentType_CONTENT_TYPE_CALENDAR:
		return "calendar"
	case contentv1.ContentType_CONTENT_TYPE_DOCUMENT:
		return "document"
	case contentv1.ContentType_CONTENT_TYPE_ATTACHMENT:
		return "attachment"
	default:
		return ""
	}
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

// GetAssertions retrieves extracted assertions for a content item.
func (s *Service) GetAssertions(ctx context.Context, req *contentv1.GetAssertionsRequest) (*contentv1.GetAssertionsResponse, error) {
	s.logger.Debug("GetAssertions called",
		logging.F("content_id", req.ContentId),
		logging.F("assertion_type", req.AssertionType),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// Prepare type filter
	var assertionType *string
	if req.AssertionType != nil && *req.AssertionType != "" {
		assertionType = req.AssertionType
	}

	// Query assertions
	assertions, err := s.repo.GetAssertions(ctx, req.ContentId, assertionType)
	if err != nil {
		s.logger.Error("Failed to get assertions",
			logging.Err(err),
			logging.F("content_id", req.ContentId),
		)
		return nil, status.Errorf(codes.Internal, "failed to get assertions: %v", err)
	}

	// Convert to proto
	protoAssertions := make([]*contentv1.Assertion, len(assertions))
	for i, rec := range assertions {
		var confidence float32
		if rec.Confidence != nil {
			confidence = *rec.Confidence
		}
		protoAssertions[i] = &contentv1.Assertion{
			Id:            rec.ID,
			AssertionType: rec.AssertionType,
			Description:   rec.Description,
			Confidence:    confidence,
			CreatedAt:     timestamppb.New(rec.CreatedAt),
		}

		// Add optional fields
		if rec.SourceQuote != nil {
			protoAssertions[i].SourceQuote = rec.SourceQuote
		}
		if rec.ExtractionModel != nil {
			protoAssertions[i].ExtractionModel = rec.ExtractionModel
		}
	}

	s.logger.Debug("Retrieved assertions",
		logging.F("content_id", req.ContentId),
		logging.F("count", len(assertions)),
	)

	return &contentv1.GetAssertionsResponse{
		ContentId:   req.ContentId,
		Assertions:  protoAssertions,
		TotalCount:  int32(len(assertions)),
	}, nil
}

// ClearError clears error fields from a successfully reprocessed content item.
func (s *Service) ClearError(ctx context.Context, req *contentv1.ClearErrorRequest) (*contentv1.ClearErrorResponse, error) {
	s.logger.Info("ClearError called",
		logging.F("content_id", req.ContentId),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// Clear error fields
	err := s.repo.ClearErrorByContentID(ctx, req.ContentId)
	if err != nil {
		s.logger.Error("Failed to clear error fields",
			logging.Err(err),
			logging.F("content_id", req.ContentId),
		)
		return nil, status.Errorf(codes.Internal, "failed to clear error fields: %v", err)
	}

	s.logger.Info("Error fields cleared",
		logging.F("content_id", req.ContentId),
	)

	return &contentv1.ClearErrorResponse{
		Success:   true,
		ContentId: req.ContentId,
		Message:   "Error fields cleared successfully",
	}, nil
}

