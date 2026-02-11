package pipelineservice

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// GetPipelineErrors retrieves pipeline errors with filtering.
func (s *Service) GetPipelineErrors(ctx context.Context, req *pipelinev1.GetPipelineErrorsRequest) (*pipelinev1.GetPipelineErrorsResponse, error) {
	s.logger.Debug("GetPipelineErrors called",
		logging.F("error_code", req.ErrorCode),
		logging.F("source_id", req.SourceId),
		logging.F("limit", req.Limit),
	)

	// Default limit
	limit := int(req.Limit)
	if limit == 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	offset := int(req.Offset)

	// Build query
	query := `
		SELECT
			pr.id,
			pr.source_id,
			pr.stage,
			pr.error_code,
			pr.status,
			pr.created_at,
			s.external_id
		FROM pipeline_runs pr
		LEFT JOIN sources s ON pr.source_id = s.id
		WHERE pr.error_code IS NOT NULL
	`
	args := []interface{}{}
	argIdx := 1

	// Filter by time range
	if req.Since != nil {
		query += fmt.Sprintf(" AND pr.created_at >= $%d", argIdx)
		args = append(args, req.Since.AsTime())
		argIdx++
	}

	if req.Until != nil {
		query += fmt.Sprintf(" AND pr.created_at <= $%d", argIdx)
		args = append(args, req.Until.AsTime())
		argIdx++
	}

	// Filter by error code
	if req.ErrorCode != "" {
		query += fmt.Sprintf(" AND pr.error_code = $%d", argIdx)
		args = append(args, req.ErrorCode)
		argIdx++
	}

	// Filter by source ID
	if req.SourceId != "" {
		query += fmt.Sprintf(" AND s.external_id = $%d", argIdx)
		args = append(args, req.SourceId)
		argIdx++
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM (" + query + ") AS filtered"
	var totalCount int64
	err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		s.logger.Error("Error counting pipeline errors", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to count errors: %v", err)
	}

	// Add ordering and pagination
	query += " ORDER BY pr.created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	// Execute query
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		s.logger.Error("Error querying pipeline errors", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to query errors: %v", err)
	}
	defer rows.Close()

	// Build response
	var events []*pipelinev1.PipelineErrorEvent
	for rows.Next() {
		var (
			runID      int64
			sourceID   int64
			stage      string
			errorCode  *string
			runStatus  string
			createdAt  time.Time
			externalID *string
		)

		if err := rows.Scan(&runID, &sourceID, &stage, &errorCode, &runStatus, &createdAt, &externalID); err != nil {
			s.logger.Error("Error scanning pipeline error row", logging.Err(err))
			continue
		}

		// Skip rows without error codes
		if errorCode == nil || *errorCode == "" {
			continue
		}

		code := perrors.ErrorCode(*errorCode)

		// Build event
		event := &pipelinev1.PipelineErrorEvent{
			Code:            *errorCode,
			Stage:           stage,
			Message:         perrors.GetDescription(code),
			Retryable:       perrors.IsRetryable(code),
			SuggestedAction: perrors.GetSuggestedAction(code),
			Details:         make(map[string]string),
		}

		// Set occurred_at timestamp
		if !createdAt.IsZero() {
			event.OccurredAt = timestamppb.New(createdAt)
		}

		// Set source ID if available
		if externalID != nil && *externalID != "" {
			event.SourceId = *externalID
		}

		// Add run ID to details
		event.Details["run_id"] = fmt.Sprintf("%d", runID)
		event.Details["source_id"] = fmt.Sprintf("%d", sourceID)

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("Error iterating pipeline error rows", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to read errors: %v", err)
	}

	return &pipelinev1.GetPipelineErrorsResponse{
		Errors:     events,
		TotalCount: totalCount,
	}, nil
}
