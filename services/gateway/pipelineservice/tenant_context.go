package pipelineservice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// CreateTenantContext creates a new tenant context entry with optional trigger conditions.
func (s *Service) CreateTenantContext(ctx context.Context, req *pipelinev1.CreateTenantContextRequest) (*pipelinev1.CreateTenantContextResponse, error) {
	s.logger.Debug("CreateTenantContext called",
		logging.F("tenant_id", req.TenantId),
		logging.F("category", req.Category),
		logging.F("label", req.Label),
	)

	if req.Category == "" {
		return nil, status.Error(codes.InvalidArgument, "category is required")
	}
	if req.Label == "" {
		return nil, status.Error(codes.InvalidArgument, "label is required")
	}
	if !req.AlwaysInject && len(req.Conditions) == 0 {
		s.logger.Warn("CreateTenantContext: no conditions and always_inject=false — entry will never be injected",
			logging.F("category", req.Category),
			logging.F("label", req.Label),
		)
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	details := req.DetailsJson
	if details == "" {
		details = "{}"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to begin transaction: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var entryID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO tenant_context
			(tenant_id, category, label, details, always_inject, active)
		VALUES ($1, $2, $3, $4::jsonb, $5, true)
		RETURNING id
	`, tenantID, req.Category, req.Label, details, req.AlwaysInject).Scan(&entryID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, status.Errorf(codes.AlreadyExists, "tenant context %q/%q already exists for tenant", req.Category, req.Label)
		}
		s.logger.Error("Error inserting tenant context", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create tenant context: %v", err)
	}

	for _, cond := range req.Conditions {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO tenant_context_conditions (context_id, field, match_type, value, case_sensitive)
			VALUES ($1, $2, $3, $4, $5)
		`, entryID, cond.Field, cond.MatchType, cond.Value, cond.CaseSensitive); err != nil {
			s.logger.Error("Error inserting tenant context condition", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to create condition: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	entries, err := s.queryTenantContext(ctx, tenantID, entryID)
	if err != nil {
		s.logger.Error("Error querying created tenant context", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "entry created but failed to fetch: %v", err)
	}
	if len(entries) == 0 {
		return nil, status.Error(codes.Internal, "entry created but not found on re-query")
	}

	return &pipelinev1.CreateTenantContextResponse{Entry: entries[0]}, nil
}

// ListTenantContext lists all tenant context entries with optional category filter.
func (s *Service) ListTenantContext(ctx context.Context, req *pipelinev1.ListTenantContextRequest) (*pipelinev1.ListTenantContextResponse, error) {
	s.logger.Debug("ListTenantContext called",
		logging.F("tenant_id", req.TenantId),
		logging.F("category", req.Category),
	)

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	query := `
		SELECT
			tc.id,
			tc.category,
			tc.label,
			tc.details::text,
			tc.always_inject,
			tc.active,
			tcc.id,
			tcc.field,
			tcc.match_type,
			tcc.value,
			tcc.case_sensitive
		FROM tenant_context tc
		LEFT JOIN tenant_context_conditions tcc ON tcc.context_id = tc.id
		WHERE tc.tenant_id = $1
	`
	args := []interface{}{tenantID}

	if req.Category != "" {
		query += " AND tc.category = $2"
		args = append(args, req.Category)
	}

	query += " ORDER BY tc.category ASC, tc.label ASC, tc.id ASC, tcc.id ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		s.logger.Error("Error listing tenant context", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list tenant context: %v", err)
	}
	defer rows.Close() //nolint:errcheck

	entries, err := scanTenantContextRows(rows)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to scan tenant context: %v", err)
	}

	return &pipelinev1.ListTenantContextResponse{Entries: entries}, nil
}

// GetTenantContext retrieves a single tenant context entry by ID.
func (s *Service) GetTenantContext(ctx context.Context, req *pipelinev1.GetTenantContextRequest) (*pipelinev1.GetTenantContextResponse, error) {
	s.logger.Debug("GetTenantContext called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	entries, err := s.queryTenantContext(ctx, tenantID, int64(req.Id))
	if err != nil {
		s.logger.Error("Error getting tenant context", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get tenant context: %v", err)
	}

	if len(entries) == 0 {
		return nil, status.Errorf(codes.NotFound, "tenant context %d not found", req.Id)
	}

	return &pipelinev1.GetTenantContextResponse{Entry: entries[0]}, nil
}

// DeleteTenantContext deletes a tenant context entry (conditions cascade).
func (s *Service) DeleteTenantContext(ctx context.Context, req *pipelinev1.DeleteTenantContextRequest) (*pipelinev1.DeleteTenantContextResponse, error) {
	s.logger.Debug("DeleteTenantContext called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	result, err := s.db.ExecContext(ctx,
		`DELETE FROM tenant_context WHERE id = $1 AND tenant_id = $2`,
		req.Id, tenantID,
	)
	if err != nil {
		s.logger.Error("Error deleting tenant context", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete tenant context: %v", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get rows affected: %v", err)
	}
	if n == 0 {
		return nil, status.Errorf(codes.NotFound, "tenant context %d not found", req.Id)
	}

	return &pipelinev1.DeleteTenantContextResponse{DeletedCount: int32(n)}, nil
}

// AddTenantContextCondition adds a trigger condition to a tenant context entry.
func (s *Service) AddTenantContextCondition(ctx context.Context, req *pipelinev1.AddTenantContextConditionRequest) (*pipelinev1.AddTenantContextConditionResponse, error) {
	s.logger.Debug("AddTenantContextCondition called",
		logging.F("tenant_id", req.TenantId),
		logging.F("context_id", req.ContextId),
	)

	if req.ContextId == 0 {
		return nil, status.Error(codes.InvalidArgument, "context_id is required")
	}
	if req.Field == "" {
		return nil, status.Error(codes.InvalidArgument, "field is required")
	}
	if req.MatchType == "" {
		return nil, status.Error(codes.InvalidArgument, "match_type is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	// Verify the context entry belongs to this tenant.
	var ownerCount int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tenant_context WHERE id = $1 AND tenant_id = $2`,
		req.ContextId, tenantID,
	).Scan(&ownerCount)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to verify context ownership: %v", err)
	}
	if ownerCount == 0 {
		return nil, status.Errorf(codes.NotFound, "tenant context %d not found", req.ContextId)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tenant_context_conditions (context_id, field, match_type, value, case_sensitive)
		VALUES ($1, $2, $3, $4, $5)
	`, req.ContextId, req.Field, req.MatchType, req.Value, req.CaseSensitive)
	if err != nil {
		s.logger.Error("Error inserting tenant context condition", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to add condition: %v", err)
	}

	entries, err := s.queryTenantContext(ctx, tenantID, int64(req.ContextId))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "condition added but failed to fetch entry: %v", err)
	}
	if len(entries) == 0 {
		return nil, status.Error(codes.Internal, "condition added but entry not found on re-query")
	}

	return &pipelinev1.AddTenantContextConditionResponse{Entry: entries[0]}, nil
}

// RemoveTenantContextCondition removes a trigger condition from a tenant context entry.
func (s *Service) RemoveTenantContextCondition(ctx context.Context, req *pipelinev1.RemoveTenantContextConditionRequest) (*pipelinev1.RemoveTenantContextConditionResponse, error) {
	s.logger.Debug("RemoveTenantContextCondition called",
		logging.F("tenant_id", req.TenantId),
		logging.F("condition_id", req.ConditionId),
	)

	if req.ConditionId == 0 {
		return nil, status.Error(codes.InvalidArgument, "condition_id is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	// Delete the condition, verifying it belongs to a context owned by this tenant.
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM tenant_context_conditions tcc
		USING tenant_context tc
		WHERE tcc.id = $1
		  AND tcc.context_id = tc.id
		  AND tc.tenant_id = $2
	`, req.ConditionId, tenantID)
	if err != nil {
		s.logger.Error("Error deleting tenant context condition", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to remove condition: %v", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get rows affected: %v", err)
	}

	return &pipelinev1.RemoveTenantContextConditionResponse{DeletedCount: int32(n)}, nil
}

// queryTenantContext queries one or all tenant context entries by ID (0 = all).
func (s *Service) queryTenantContext(ctx context.Context, tenantID string, id int64) ([]*pipelinev1.TenantContextEntry, error) {
	query := `
		SELECT
			tc.id,
			tc.category,
			tc.label,
			tc.details::text,
			tc.always_inject,
			tc.active,
			tcc.id,
			tcc.field,
			tcc.match_type,
			tcc.value,
			tcc.case_sensitive
		FROM tenant_context tc
		LEFT JOIN tenant_context_conditions tcc ON tcc.context_id = tc.id
		WHERE tc.tenant_id = $1
	`
	args := []interface{}{tenantID}

	if id != 0 {
		query += " AND tc.id = $2"
		args = append(args, id)
	}

	query += " ORDER BY tc.category ASC, tc.label ASC, tc.id ASC, tcc.id ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying tenant context: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	return scanTenantContextRows(rows)
}

// scanTenantContextRows scans a LEFT JOIN result of tenant_context + conditions into proto entries.
func scanTenantContextRows(rows *sql.Rows) ([]*pipelinev1.TenantContextEntry, error) {
	entryMap := make(map[int64]*pipelinev1.TenantContextEntry)
	var entryOrder []int64

	for rows.Next() {
		var (
			entryID       int64
			category      string
			label         string
			details       string
			alwaysInject  bool
			active        bool
			condID        sql.NullInt64
			condField     sql.NullString
			condMatchType sql.NullString
			condValue     sql.NullString
			condCaseSens  sql.NullBool
		)

		if err := rows.Scan(
			&entryID, &category, &label, &details, &alwaysInject, &active,
			&condID, &condField, &condMatchType, &condValue, &condCaseSens,
		); err != nil {
			return nil, fmt.Errorf("scanning tenant context row: %w", err)
		}

		entry, exists := entryMap[entryID]
		if !exists {
			entry = &pipelinev1.TenantContextEntry{
				Id:           int32(entryID),
				Category:     category,
				Label:        label,
				DetailsJson:  details,
				AlwaysInject: alwaysInject,
				Active:       active,
			}
			entryMap[entryID] = entry
			entryOrder = append(entryOrder, entryID)
		}

		if condID.Valid {
			entry.Conditions = append(entry.Conditions, &pipelinev1.TenantContextCondition{
				Id:            int32(condID.Int64),
				Field:         condField.String,
				MatchType:     condMatchType.String,
				Value:         condValue.String,
				CaseSensitive: condCaseSens.Bool,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tenant context rows: %w", err)
	}

	result := make([]*pipelinev1.TenantContextEntry, 0, len(entryOrder))
	for _, id := range entryOrder {
		result = append(result, entryMap[id])
	}
	return result, nil
}
