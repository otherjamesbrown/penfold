package pipelineservice

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// batchPipelineInput mirrors services/worker/workflows.BatchPipelineInput via JSON tags.
// Since we cannot import the worker package from gateway, we define a local struct
// with matching JSON field names so Temporal can deserialize correctly.
type batchPipelineInput struct {
	TenantID      string  `json:"tenant_id"`
	TenantName    string  `json:"tenant_name"`
	ContentIDs    []int64 `json:"content_ids"`
	MaxConcurrent int     `json:"max_concurrent"`
	BatchID       string  `json:"batch_id"`
}

// batchProgress mirrors services/worker/workflows.BatchProgress for query results.
type batchProgress struct {
	TotalItems     int     `json:"total_items"`
	CompletedItems int     `json:"completed_items"`
	FailedItems    int     `json:"failed_items"`
	ActiveSourceIDs []int64 `json:"active_source_ids"`
}

// StartBatchPipeline creates a batch processing job.
func (s *Service) StartBatchPipeline(ctx context.Context, req *pipelinev1.StartBatchPipelineRequest) (*pipelinev1.StartBatchPipelineResponse, error) {
	s.logger.Info("StartBatchPipeline called",
		logging.F("tenant_id", req.TenantId),
		logging.F("content_ids_count", len(req.ContentIds)),
		logging.F("max_concurrent", req.MaxConcurrent),
		logging.F("source_tag", req.SourceTag),
	)

	if s.temporalClient == nil {
		return nil, status.Error(codes.Unavailable, "Temporal client not configured")
	}

	tenantID := req.TenantId

	// Determine the set of source IDs to process.
	contentIDs := req.ContentIds
	if len(contentIDs) == 0 {
		// Query pending source IDs from DB.
		ids, err := s.getPendingSourceIDs(ctx, req.SourceTag, 0)
		if err != nil {
			s.logger.Error("Error querying pending source IDs", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to query pending sources: %v", err)
		}
		contentIDs = ids
	}

	if len(contentIDs) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "no pending sources to process")
	}

	// Determine max_concurrent: use request value or fall back to config.
	maxConcurrent := int(req.MaxConcurrent)
	if maxConcurrent <= 0 {
		cfg, err := s.getMaxConcurrent(ctx)
		if err != nil {
			s.logger.Warn("Could not read max_concurrent from config, defaulting to 1", logging.Err(err))
			cfg = 1
		}
		maxConcurrent = cfg
	}

	// Generate IDs.
	batchID := uuid.NewString()
	workflowID := fmt.Sprintf("batch-%s-%s", tenantID, batchID)

	// Create batch record in DB.
	batch := &pipeline.Batch{
		TenantID:       tenantID,
		WorkflowID:     workflowID,
		TotalItems:     len(contentIDs),
		CompletedItems: 0,
		FailedItems:    0,
		Status:         "running",
	}
	createdID, err := s.repo.CreateBatch(ctx, batch)
	if err != nil {
		s.logger.Error("Error creating batch record", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create batch record: %v", err)
	}
	// Use the DB-generated UUID (should match batchID we passed, but use returned value).
	batchID = createdID

	// Build workflow input using local struct that matches JSON tags of worker's BatchPipelineInput.
	input := batchPipelineInput{
		TenantID:      tenantID,
		TenantName:    "", // Could be resolved from tenant registry; omitted for now
		ContentIDs:    contentIDs,
		MaxConcurrent: maxConcurrent,
		BatchID:       batchID,
	}

	opts := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: "penfold-main",
	}
	_, err = s.temporalClient.ExecuteWorkflow(ctx, opts, "BatchPipelineWorkflow", input)
	if err != nil {
		// Roll back batch record to cancelled state on workflow start failure.
		if updateErr := s.repo.UpdateBatchStatus(ctx, batchID, "cancelled"); updateErr != nil {
			s.logger.Warn("Failed to cancel batch after workflow start error",
				logging.F("batch_id", batchID),
				logging.Err(updateErr),
			)
		}
		s.logger.Error("Error starting BatchPipelineWorkflow",
			logging.F("workflow_id", workflowID),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to start batch workflow: %v", err)
	}

	s.logger.Info("BatchPipelineWorkflow started",
		logging.F("batch_id", batchID),
		logging.F("workflow_id", workflowID),
		logging.F("total_items", len(contentIDs)),
		logging.F("max_concurrent", maxConcurrent),
	)

	return &pipelinev1.StartBatchPipelineResponse{
		BatchId:       batchID,
		WorkflowId:    workflowID,
		TotalItems:    int32(len(contentIDs)),
		MaxConcurrent: int32(maxConcurrent),
		Message:       fmt.Sprintf("Batch started with %d items (max_concurrent: %d)", len(contentIDs), maxConcurrent),
	}, nil
}

// GetBatchStatus retrieves the status of a batch processing job.
func (s *Service) GetBatchStatus(ctx context.Context, req *pipelinev1.GetBatchStatusRequest) (*pipelinev1.GetBatchStatusResponse, error) {
	s.logger.Debug("GetBatchStatus called",
		logging.F("batch_id", req.BatchId),
	)

	if req.BatchId == "" {
		return nil, status.Error(codes.InvalidArgument, "batch_id is required")
	}

	// Retrieve batch record from DB.
	batch, err := s.repo.GetBatch(ctx, req.BatchId)
	if err != nil {
		if errors.Is(err, pipeline.ErrBatchNotFound) {
			return nil, status.Errorf(codes.NotFound, "batch not found: %s", req.BatchId)
		}
		s.logger.Error("Error getting batch", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get batch: %v", err)
	}

	// Build base response from DB record.
	resp := &pipelinev1.GetBatchStatusResponse{
		BatchId:        batch.ID,
		Status:         batch.Status,
		TotalItems:     int32(batch.TotalItems),
		CompletedItems: int32(batch.CompletedItems),
		FailedItems:    int32(batch.FailedItems),
		PendingItems:   int32(batch.TotalItems - batch.CompletedItems - batch.FailedItems),
		CreatedAt:      timestamppb.New(batch.CreatedAt),
	}

	// Try to query the live workflow for real-time progress (active source IDs, etc.).
	// This is best-effort; ignore errors if the workflow is completed or unreachable.
	if s.temporalClient != nil && batch.Status == "running" {
		queryResp, queryErr := s.temporalClient.QueryWorkflow(ctx, batch.WorkflowID, "", "batch-progress")
		if queryErr == nil {
			var progress batchProgress
			if decodeErr := queryResp.Get(&progress); decodeErr == nil {
				// Override with live data.
				resp.CompletedItems = int32(progress.CompletedItems)
				resp.FailedItems = int32(progress.FailedItems)
				resp.ActiveItems = int32(len(progress.ActiveSourceIDs))
				resp.ActiveSourceIds = progress.ActiveSourceIDs
				pending := progress.TotalItems - progress.CompletedItems - progress.FailedItems - len(progress.ActiveSourceIDs)
				if pending < 0 {
					pending = 0
				}
				resp.PendingItems = int32(pending)
			}
		}
		// If query fails (e.g., workflow completed before we queried), fall back to DB data.
	}

	return resp, nil
}

// ListBatches lists recent batch processing jobs.
func (s *Service) ListBatches(ctx context.Context, req *pipelinev1.ListBatchesRequest) (*pipelinev1.ListBatchesResponse, error) {
	s.logger.Debug("ListBatches called",
		logging.F("tenant_id", req.TenantId),
		logging.F("limit", req.Limit),
	)

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}

	batches, err := s.repo.ListBatches(ctx, req.TenantId, limit)
	if err != nil {
		s.logger.Error("Error listing batches", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list batches: %v", err)
	}

	summaries := make([]*pipelinev1.BatchSummary, len(batches))
	for i, b := range batches {
		summaries[i] = batchToProto(b)
	}

	return &pipelinev1.ListBatchesResponse{
		Batches: summaries,
	}, nil
}

// CancelBatch cancels a running batch processing job.
func (s *Service) CancelBatch(ctx context.Context, req *pipelinev1.CancelBatchRequest) (*pipelinev1.CancelBatchResponse, error) {
	s.logger.Info("CancelBatch called",
		logging.F("batch_id", req.BatchId),
	)

	if req.BatchId == "" {
		return nil, status.Error(codes.InvalidArgument, "batch_id is required")
	}

	// Retrieve batch to find workflow ID.
	batch, err := s.repo.GetBatch(ctx, req.BatchId)
	if err != nil {
		if errors.Is(err, pipeline.ErrBatchNotFound) {
			return nil, status.Errorf(codes.NotFound, "batch not found: %s", req.BatchId)
		}
		s.logger.Error("Error getting batch for cancellation", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get batch: %v", err)
	}

	if batch.Status != "running" {
		return &pipelinev1.CancelBatchResponse{
			Cancelled: false,
			Message:   fmt.Sprintf("batch is not running (status: %s)", batch.Status),
		}, nil
	}

	// Send cancel signal to Temporal workflow.
	if s.temporalClient != nil {
		signalErr := s.temporalClient.SignalWorkflow(ctx, batch.WorkflowID, "", "batch-cancel", nil)
		if signalErr != nil {
			s.logger.Warn("Failed to send cancel signal to workflow",
				logging.F("workflow_id", batch.WorkflowID),
				logging.Err(signalErr),
			)
			// Continue anyway to mark batch as cancelled in DB.
		}
	}

	// Update batch status in DB.
	if updateErr := s.repo.UpdateBatchStatus(ctx, req.BatchId, "cancelled"); updateErr != nil {
		s.logger.Error("Error updating batch status to cancelled",
			logging.F("batch_id", req.BatchId),
			logging.Err(updateErr),
		)
		return nil, status.Errorf(codes.Internal, "failed to update batch status: %v", updateErr)
	}

	s.logger.Info("Batch cancelled",
		logging.F("batch_id", req.BatchId),
		logging.F("workflow_id", batch.WorkflowID),
	)

	return &pipelinev1.CancelBatchResponse{
		Cancelled: true,
		Message:   fmt.Sprintf("Batch %s cancelled", req.BatchId),
	}, nil
}

// =============================================================================
// Helper functions
// =============================================================================

// getPendingSourceIDs returns IDs of sources with processing_status = 'pending'.
// If sourceTag is non-empty, filters by that tag. limit=0 means no limit.
func (s *Service) getPendingSourceIDs(ctx context.Context, sourceTag string, limit int) ([]int64, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	query := `SELECT id FROM sources WHERE processing_status = 'pending' AND is_deleted = false`
	args := []interface{}{}
	argIdx := 1

	if sourceTag != "" {
		query += fmt.Sprintf(` AND ingestion_metadata->>'source_tag' = $%d`, argIdx)
		args = append(args, sourceTag)
		argIdx++
	}

	query += ` ORDER BY created_at ASC`

	if limit > 0 {
		query += fmt.Sprintf(` LIMIT $%d`, argIdx)
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying pending source IDs: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning source ID: %w", err)
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

// batchToProto converts a pipeline.Batch to a BatchSummary proto.
func batchToProto(b *pipeline.Batch) *pipelinev1.BatchSummary {
	if b == nil {
		return nil
	}
	return &pipelinev1.BatchSummary{
		Id:             b.ID,
		WorkflowId:     b.WorkflowID,
		Status:         b.Status,
		TotalItems:     int32(b.TotalItems),
		CompletedItems: int32(b.CompletedItems),
		FailedItems:    int32(b.FailedItems),
		CreatedAt:      timestamppb.New(b.CreatedAt),
		UpdatedAt:      timestamppb.New(b.UpdatedAt),
	}
}
