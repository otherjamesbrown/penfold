package pipelineservice

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// =============================================================================
// Tenant Context RPCs
// =============================================================================

// CreateTenantContext creates a new tenant context entry with optional conditions.
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

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	detailsJSON := req.DetailsJson
	if detailsJSON == "" {
		detailsJSON = "{}"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to begin transaction: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var contextID int32
	err = tx.QueryRowContext(ctx, `
		INSERT INTO tenant_context (tenant_id, category, label, details, always_inject, active)
		VALUES ($1, $2, $3, $4::jsonb, $5, true)
		RETURNING id
	`, tenantID, req.Category, req.Label, detailsJSON, req.AlwaysInject).Scan(&contextID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, status.Errorf(codes.AlreadyExists, "tenant context entry %q/%q already exists", req.Category, req.Label)
		}
		s.logger.Error("Error inserting tenant context entry", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create tenant context entry: %v", err)
	}

	for _, cond := range req.Conditions {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO tenant_context_conditions (context_id, field, match_type, value, case_sensitive)
			VALUES ($1, $2, $3, $4, $5)
		`, contextID, cond.Field, cond.MatchType, cond.Value, cond.CaseSensitive); err != nil {
			s.logger.Error("Error inserting tenant context condition", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to create condition: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	entry, err := s.queryTenantContextEntry(ctx, tenantID, contextID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "entry created but failed to fetch: %v", err)
	}

	return &pipelinev1.CreateTenantContextResponse{Entry: entry}, nil
}

// ListTenantContext lists all tenant context entries for a tenant.
func (s *Service) ListTenantContext(ctx context.Context, req *pipelinev1.ListTenantContextRequest) (*pipelinev1.ListTenantContextResponse, error) {
	s.logger.Debug("ListTenantContext called", logging.F("tenant_id", req.TenantId))

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	query := `
		SELECT tc.id, tc.category, tc.label, tc.details::text, tc.always_inject, tc.active,
		       tcc.id, tcc.field, tcc.match_type, tcc.value, tcc.case_sensitive
		FROM tenant_context tc
		LEFT JOIN tenant_context_conditions tcc ON tcc.context_id = tc.id
		WHERE tc.tenant_id = $1`
	args := []interface{}{tenantID}

	if req.Category != "" {
		query += " AND tc.category = $2"
		args = append(args, req.Category)
	}
	query += " ORDER BY tc.id ASC, tcc.id ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list tenant context entries: %v", err)
	}
	defer rows.Close()

	entries, err := scanTenantContextRows(rows)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to scan tenant context entries: %v", err)
	}

	return &pipelinev1.ListTenantContextResponse{Entries: entries}, nil
}

// GetTenantContext retrieves a single tenant context entry by ID.
func (s *Service) GetTenantContext(ctx context.Context, req *pipelinev1.GetTenantContextRequest) (*pipelinev1.GetTenantContextResponse, error) {
	s.logger.Debug("GetTenantContext called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	entry, err := s.queryTenantContextEntry(ctx, tenantID, req.Id)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, status.Errorf(codes.NotFound, "tenant context entry %d not found", req.Id)
	}

	return &pipelinev1.GetTenantContextResponse{Entry: entry}, nil
}

// DeleteTenantContext deletes a tenant context entry (conditions cascade).
func (s *Service) DeleteTenantContext(ctx context.Context, req *pipelinev1.DeleteTenantContextRequest) (*pipelinev1.DeleteTenantContextResponse, error) {
	s.logger.Debug("DeleteTenantContext called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	result, err := s.db.ExecContext(ctx,
		`DELETE FROM tenant_context WHERE tenant_id = $1 AND id = $2`,
		tenantID, req.Id,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete tenant context entry: %v", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get rows affected: %v", err)
	}
	if n == 0 {
		return nil, status.Errorf(codes.NotFound, "tenant context entry %d not found", req.Id)
	}

	return &pipelinev1.DeleteTenantContextResponse{DeletedCount: int32(n)}, nil
}

// AddTenantContextCondition adds a trigger condition to an existing context entry.
func (s *Service) AddTenantContextCondition(ctx context.Context, req *pipelinev1.AddTenantContextConditionRequest) (*pipelinev1.AddTenantContextConditionResponse, error) {
	s.logger.Debug("AddTenantContextCondition called",
		logging.F("tenant_id", req.TenantId),
		logging.F("context_id", req.ContextId),
	)

	if req.Condition == nil {
		return nil, status.Error(codes.InvalidArgument, "condition is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	// Verify the context entry belongs to this tenant.
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT true FROM tenant_context WHERE id = $1 AND tenant_id = $2`,
		req.ContextId, tenantID,
	).Scan(&exists)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "tenant context entry %d not found", req.ContextId)
	}

	var condID int32
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO tenant_context_conditions (context_id, field, match_type, value, case_sensitive)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, req.ContextId, req.Condition.Field, req.Condition.MatchType, req.Condition.Value, req.Condition.CaseSensitive).Scan(&condID)
	if err != nil {
		s.logger.Error("Error inserting tenant context condition", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to add condition: %v", err)
	}

	return &pipelinev1.AddTenantContextConditionResponse{
		Condition: &pipelinev1.TenantContextCondition{
			Id:            condID,
			Field:         req.Condition.Field,
			MatchType:     req.Condition.MatchType,
			Value:         req.Condition.Value,
			CaseSensitive: req.Condition.CaseSensitive,
		},
	}, nil
}

// SuggestTenantContextTriggers calls the AI service to suggest trigger conditions for a context entry.
func (s *Service) SuggestTenantContextTriggers(ctx context.Context, req *pipelinev1.SuggestTenantContextTriggersRequest) (*pipelinev1.SuggestTenantContextTriggersResponse, error) {
	s.logger.Debug("SuggestTenantContextTriggers called",
		logging.F("tenant_id", req.TenantId),
		logging.F("category", req.Category),
		logging.F("label", req.Label),
	)

	if s.aiClient == nil {
		return nil, status.Error(codes.Unavailable, "AI service not available")
	}

	aiResp, err := s.aiClient.SuggestContextTriggers(ctx, &aiv1.SuggestContextTriggersRequest{
		TenantId:    &req.TenantId,
		Category:    req.Category,
		Label:       req.Label,
		DetailsJson: req.DetailsJson,
	})
	if err != nil {
		s.logger.Error("Error calling AI SuggestContextTriggers", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to suggest triggers: %v", err)
	}

	conditions := make([]*pipelinev1.TenantContextCondition, 0, len(aiResp.Conditions))
	for _, c := range aiResp.Conditions {
		conditions = append(conditions, &pipelinev1.TenantContextCondition{
			Field:     c.Field,
			MatchType: c.MatchType,
			Value:     c.Value,
		})
	}

	return &pipelinev1.SuggestTenantContextTriggersResponse{
		Conditions: conditions,
		ModelUsed:  aiResp.ModelUsed,
	}, nil
}

// queryTenantContextEntry fetches a single context entry by tenant and ID.
// Returns nil (not an error) when not found.
func (s *Service) queryTenantContextEntry(ctx context.Context, tenantID string, id int32) (*pipelinev1.TenantContextEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT tc.id, tc.category, tc.label, tc.details::text, tc.always_inject, tc.active,
		       tcc.id, tcc.field, tcc.match_type, tcc.value, tcc.case_sensitive
		FROM tenant_context tc
		LEFT JOIN tenant_context_conditions tcc ON tcc.context_id = tc.id
		WHERE tc.tenant_id = $1 AND tc.id = $2
		ORDER BY tcc.id ASC
	`, tenantID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries, err := scanTenantContextRows(rows)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return entries[0], nil
}

// scanTenantContextRows scans database rows into TenantContextEntry proto messages.
// Handles the LEFT JOIN pattern: multiple rows per entry (one per condition).
func scanTenantContextRows(rows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
}) ([]*pipelinev1.TenantContextEntry, error) {
	entryMap := make(map[int32]*pipelinev1.TenantContextEntry)
	var order []int32

	for rows.Next() {
		var (
			entryID      int32
			category     string
			label        string
			detailsJSON  string
			alwaysInject bool
			active       bool
			condID       *int32
			condField    *string
			condType     *string
			condValue    *string
			condCase     *bool
		)
		if err := rows.Scan(
			&entryID, &category, &label, &detailsJSON,
			&alwaysInject, &active,
			&condID, &condField, &condType, &condValue, &condCase,
		); err != nil {
			return nil, err
		}

		if _, seen := entryMap[entryID]; !seen {
			// Normalise details JSON
			if !json.Valid([]byte(detailsJSON)) {
				detailsJSON = "{}"
			}
			entryMap[entryID] = &pipelinev1.TenantContextEntry{
				Id:           entryID,
				Category:     category,
				Label:        label,
				DetailsJson:  detailsJSON,
				AlwaysInject: alwaysInject,
				Active:       active,
			}
			order = append(order, entryID)
		}

		if condID != nil {
			entryMap[entryID].Conditions = append(entryMap[entryID].Conditions, &pipelinev1.TenantContextCondition{
				Id:            *condID,
				Field:         *condField,
				MatchType:     *condType,
				Value:         *condValue,
				CaseSensitive: *condCase,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]*pipelinev1.TenantContextEntry, 0, len(order))
	for _, id := range order {
		result = append(result, entryMap[id])
	}
	return result, nil
}
