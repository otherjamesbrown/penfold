// Package pipelineservice implements the PipelineService gRPC server.
package pipelineservice

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"go.temporal.io/sdk/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	pkgtemporal "github.com/otherjamesbrown/penfold/pkg/temporal"
)

// Service implements the PipelineService gRPC server.
type Service struct {
	pipelinev1.UnimplementedPipelineServiceServer
	repo           *pipeline.Repository
	logger         logging.Logger
	temporalClient client.Client
	db             *sql.DB
	namespace      string
}

// NewService creates a new pipeline service.
func NewService(repo *pipeline.Repository, logger logging.Logger, temporalClient client.Client, db *sql.DB, namespace string) *Service {
	if namespace == "" {
		namespace = "default"
	}
	return &Service{
		repo:           repo,
		logger:         logger,
		temporalClient: temporalClient,
		db:             db,
		namespace:      namespace,
	}
}

// GetStats retrieves overall pipeline statistics.
func (s *Service) GetStats(ctx context.Context, req *pipelinev1.GetStatsRequest) (*pipelinev1.GetStatsResponse, error) {
	s.logger.Debug("GetStats called",
		logging.F("tenant_id", req.TenantId),
	)

	stats, err := s.repo.GetStats(ctx)
	if err != nil {
		s.logger.Error("Error getting pipeline stats", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get pipeline stats: %v", err)
	}

	return &pipelinev1.GetStatsResponse{
		Stats: statsToProto(stats),
	}, nil
}

// GetJob retrieves detailed information about a specific ingest job.
func (s *Service) GetJob(ctx context.Context, req *pipelinev1.GetJobRequest) (*pipelinev1.GetJobResponse, error) {
	s.logger.Debug("GetJob called",
		logging.F("job_id", req.JobId),
	)

	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	job, sources, err := s.repo.GetJob(ctx, req.JobId)
	if err != nil {
		s.logger.Error("Error getting job", logging.Err(err))
		return nil, status.Errorf(codes.NotFound, "job not found: %s", req.JobId)
	}

	return &pipelinev1.GetJobResponse{
		Job:     jobDetailsToProto(job),
		Sources: sourceStatsToProto(sources),
	}, nil
}

// ListJobs lists recent ingest jobs with optional filtering.
func (s *Service) ListJobs(ctx context.Context, req *pipelinev1.ListJobsRequest) (*pipelinev1.ListJobsResponse, error) {
	s.logger.Debug("ListJobs called",
		logging.F("limit", req.Limit),
		logging.F("status", req.Status),
	)

	filter := pipeline.JobFilter{
		Limit:     int(req.Limit),
		Offset:    int(req.Offset),
		Status:    req.Status,
		SourceTag: req.SourceTag,
	}

	jobs, err := s.repo.ListJobs(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing jobs", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list jobs: %v", err)
	}

	totalCount, err := s.repo.CountJobs(ctx, filter)
	if err != nil {
		s.logger.Error("Error counting jobs", logging.Err(err))
		totalCount = int64(len(jobs))
	}

	protoJobs := make([]*pipelinev1.JobSummary, len(jobs))
	for i, j := range jobs {
		protoJobs[i] = jobSummaryToProto(&j)
	}

	return &pipelinev1.ListJobsResponse{
		Jobs:       protoJobs,
		TotalCount: totalCount,
	}, nil
}

// KickProcessing triggers processing of pending pipeline items.
func (s *Service) KickProcessing(ctx context.Context, req *pipelinev1.KickProcessingRequest) (*pipelinev1.KickProcessingResponse, error) {
	s.logger.Info("KickProcessing called",
		logging.F("tenant_id", req.TenantId),
		logging.F("limit", req.Limit),
		logging.F("source_tag", req.SourceTag),
	)

	// Validate limit
	limit := int(req.Limit)
	if limit < 0 {
		return nil, status.Error(codes.InvalidArgument, "limit must be non-negative")
	}
	if limit == 0 {
		limit = 100 // Default limit
	}

	// Check if Temporal client is available
	if s.temporalClient == nil {
		return nil, status.Error(codes.Unavailable, "Temporal client not configured")
	}

	// Get pending sources from repository
	sources, _, err := s.repo.KickPendingProcessing(ctx, limit, req.SourceTag)
	if err != nil {
		s.logger.Error("Error getting pending sources", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get pending sources: %v", err)
	}

	// Start a workflow for each pending source
	var startedCount int
	for _, src := range sources {
		workflowID := pkgtemporal.GenerateIngestWorkflowID(src.TenantID, src.SourceSystem, strconv.FormatInt(src.ID, 10))
		input := pkgtemporal.ContentIngestionInput{
			TenantID:    src.TenantID,
			SourceID:    src.ID,
			ContentID:   src.ContentID,
			SourceType:  src.SourceSystem,
			ContentHash: src.ContentHash,
		}
		opts := client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: "penfold-main",
		}
		_, err := s.temporalClient.ExecuteWorkflow(ctx, opts, "ContentIngestionWorkflow", input)
		if err != nil {
			s.logger.Warn("Failed to start workflow for source",
				logging.F("source_id", src.ID),
				logging.F("workflow_id", workflowID),
				logging.Err(err),
			)
			// Continue with other sources
			continue
		}
		s.logger.Info("Started ContentIngestionWorkflow",
			logging.F("workflow_id", workflowID),
			logging.F("source_id", src.ID),
			logging.F("content_id", src.ContentID),
		)
		startedCount++
	}

	message := fmt.Sprintf("Started %d workflows for processing", startedCount)
	if req.SourceTag != "" {
		message = fmt.Sprintf("Started %d workflows for processing (source_tag: %s)", startedCount, req.SourceTag)
	}

	s.logger.Info("Workflows started",
		logging.F("started_count", startedCount),
		logging.F("total_pending", len(sources)),
		logging.F("source_tag", req.SourceTag),
	)

	return &pipelinev1.KickProcessingResponse{
		QueuedCount: int64(startedCount),
		Message:     message,
	}, nil
}

// RetryFailed retries failed pipeline items.
func (s *Service) RetryFailed(ctx context.Context, req *pipelinev1.RetryFailedRequest) (*pipelinev1.RetryFailedResponse, error) {
	s.logger.Info("RetryFailed called",
		logging.F("tenant_id", req.TenantId),
		logging.F("job_id", req.JobId),
		logging.F("stage", req.Stage),
	)

	// Validate stage if provided
	if req.Stage != "" && req.Stage != "embedding" && req.Stage != "attachment" {
		return nil, status.Error(codes.InvalidArgument, "stage must be 'embedding' or 'attachment'")
	}

	// Call repository to retry failed items
	count, err := s.repo.RetryFailedItems(ctx, req.JobId, req.Stage)
	if err != nil {
		s.logger.Error("Error retrying failed items", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to retry items: %v", err)
	}

	var message string
	if req.JobId != "" {
		message = fmt.Sprintf("Retried %d failed items for job %s", count, req.JobId)
	} else if req.Stage != "" {
		message = fmt.Sprintf("Retried %d failed items in stage %s", count, req.Stage)
	} else {
		message = fmt.Sprintf("Retried %d failed items", count)
	}

	s.logger.Info("Failed items retried",
		logging.F("retried_count", count),
		logging.F("job_id", req.JobId),
		logging.F("stage", req.Stage),
	)

	return &pipelinev1.RetryFailedResponse{
		RetriedCount: int64(count),
		Message:      message,
	}, nil
}

// Conversion helpers

func statusCountsToProto(counts []pipeline.StatusCount) []*pipelinev1.StatusCount {
	if counts == nil {
		return nil
	}
	result := make([]*pipelinev1.StatusCount, len(counts))
	for i, c := range counts {
		result[i] = &pipelinev1.StatusCount{
			Status: c.Status,
			Count:  c.Count,
		}
	}
	return result
}

func jobSummaryToProto(j *pipeline.JobSummary) *pipelinev1.JobSummary {
	if j == nil {
		return nil
	}
	proto := &pipelinev1.JobSummary{
		Id:            j.ID,
		Status:        j.Status,
		SourceTag:     j.SourceTag,
		TotalFiles:    int32(j.TotalFiles),
		ImportedCount: int32(j.ImportedCount),
		SkippedCount:  int32(j.SkippedCount),
		FailedCount:   int32(j.FailedCount),
		CreatedAt:     timestamppb.New(j.CreatedAt),
	}
	if j.CompletedAt != nil {
		proto.CompletedAt = timestamppb.New(*j.CompletedAt)
	}
	return proto
}

func jobDetailsToProto(j *pipeline.JobDetails) *pipelinev1.JobDetails {
	if j == nil {
		return nil
	}
	return &pipelinev1.JobDetails{
		Summary:        jobSummaryToProto(&j.JobSummary),
		ProcessedFiles: j.ProcessedFiles,
	}
}

func sourceStatsToProto(s *pipeline.SourceStats) *pipelinev1.SourceStats {
	if s == nil {
		return nil
	}
	return &pipelinev1.SourceStats{
		Total:    s.Total,
		ByStatus: statusCountsToProto(s.ByStatus),
	}
}

func statsToProto(s *pipeline.PipelineStats) *pipelinev1.PipelineStats {
	if s == nil {
		return nil
	}

	recentJobs := make([]*pipelinev1.JobSummary, len(s.RecentJobs))
	for i := range s.RecentJobs {
		recentJobs[i] = jobSummaryToProto(&s.RecentJobs[i])
	}

	return &pipelinev1.PipelineStats{
		SourcesTotal:               s.SourcesTotal,
		SourcesByStatus:            statusCountsToProto(s.SourcesByStatus),
		SourcesByFailureCategory:   statusCountsToProto(s.SourcesByFailureCategory),
		EmbeddingsTotal:            s.EmbeddingsTotal,
		EmbeddingsRecent:           s.EmbeddingsRecent,
		AttachmentsTotal:           s.AttachmentsTotal,
		AttachmentsByTier:          statusCountsToProto(s.AttachmentsByTier),
		JobsTotal:                  s.JobsTotal,
		JobsByStatus:               statusCountsToProto(s.JobsByStatus),
		RecentJobs:                 recentJobs,
		Timestamp:                  timestamppb.New(s.Timestamp),
	}
}

// GetQueueStatus retrieves processing queue depths and rates.
func (s *Service) GetQueueStatus(ctx context.Context, req *pipelinev1.GetQueueStatusRequest) (*pipelinev1.GetQueueStatusResponse, error) {
	s.logger.Debug("GetQueueStatus called",
		logging.F("stage", req.Stage),
	)

	if s.temporalClient == nil {
		return nil, status.Error(codes.Unavailable, "Temporal client not available")
	}

	// Query Temporal for workflow counts
	// For now, return mock data structure since we need to determine the exact workflow types
	queues := []*pipelinev1.QueueStats{
		{
			Name:                   "embeddings",
			PendingCount:           0,
			ProcessingCount:        0,
			RatePerMinute:          0.0,
			OldestItemAgeSeconds:   0,
			WorkerCount:            0,
		},
		{
			Name:                   "entities",
			PendingCount:           0,
			ProcessingCount:        0,
			RatePerMinute:          0.0,
			OldestItemAgeSeconds:   0,
			WorkerCount:            0,
		},
	}

	// Filter by stage if provided
	if req.Stage != "" {
		filtered := make([]*pipelinev1.QueueStats, 0)
		for _, q := range queues {
			if q.Name == req.Stage {
				filtered = append(filtered, q)
			}
		}
		queues = filtered
	}

	return &pipelinev1.GetQueueStatusResponse{
		Queues: queues,
	}, nil
}

// GetPipelineHealth performs a comprehensive pipeline health check.
func (s *Service) GetPipelineHealth(ctx context.Context, req *pipelinev1.GetPipelineHealthRequest) (*pipelinev1.GetPipelineHealthResponse, error) {
	s.logger.Debug("GetPipelineHealth called")

	var checks []*pipelinev1.HealthCheck
	var issues []string
	healthyCount := 0

	// Check database
	dbCheck := &pipelinev1.HealthCheck{
		Name: "database",
	}
	if s.db != nil {
		if err := s.db.PingContext(ctx); err != nil {
			dbCheck.Healthy = false
			dbCheck.Status = "UNHEALTHY"
			dbCheck.Message = fmt.Sprintf("Database ping failed: %v", err)
			issues = append(issues, "Database connection failed")
		} else {
			dbCheck.Healthy = true
			dbCheck.Status = "HEALTHY"
			dbCheck.Message = "Database connection OK"
			healthyCount++
		}
	} else {
		dbCheck.Healthy = false
		dbCheck.Status = "UNAVAILABLE"
		dbCheck.Message = "Database not configured"
		issues = append(issues, "Database not configured")
	}
	checks = append(checks, dbCheck)

	// Check Temporal
	temporalCheck := &pipelinev1.HealthCheck{
		Name: "temporal",
	}
	if s.temporalClient != nil {
		// Try to get system info as health check
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, err := s.temporalClient.WorkflowService().GetSystemInfo(ctx, nil)
		if err != nil {
			temporalCheck.Healthy = false
			temporalCheck.Status = "UNHEALTHY"
			temporalCheck.Message = fmt.Sprintf("Temporal health check failed: %v", err)
			issues = append(issues, "Temporal connection failed")
		} else {
			temporalCheck.Healthy = true
			temporalCheck.Status = "HEALTHY"
			temporalCheck.Message = "Temporal connection OK"
			healthyCount++
		}
	} else {
		temporalCheck.Healthy = false
		temporalCheck.Status = "UNAVAILABLE"
		temporalCheck.Message = "Temporal client not configured"
		issues = append(issues, "Temporal not configured")
	}
	checks = append(checks, temporalCheck)

	// Check pipeline repository
	repoCheck := &pipelinev1.HealthCheck{
		Name: "pipeline_repository",
	}
	if s.repo != nil {
		repoCheck.Healthy = true
		repoCheck.Status = "HEALTHY"
		repoCheck.Message = "Pipeline repository initialized"
		healthyCount++
	} else {
		repoCheck.Healthy = false
		repoCheck.Status = "UNAVAILABLE"
		repoCheck.Message = "Pipeline repository not initialized"
		issues = append(issues, "Pipeline repository not initialized")
	}
	checks = append(checks, repoCheck)

	// Determine overall status
	var overallStatus string
	totalChecks := len(checks)
	if healthyCount == totalChecks {
		overallStatus = "HEALTHY"
	} else if healthyCount > 0 {
		overallStatus = "DEGRADED"
	} else {
		overallStatus = "UNHEALTHY"
	}

	s.logger.Info("Pipeline health check completed",
		logging.F("overall_status", overallStatus),
		logging.F("healthy_checks", healthyCount),
		logging.F("total_checks", totalChecks),
	)

	return &pipelinev1.GetPipelineHealthResponse{
		OverallStatus: overallStatus,
		Checks:        checks,
		Issues:        issues,
	}, nil
}

// GetContentTrace retrieves full processing history for a content item.
func (s *Service) GetContentTrace(ctx context.Context, req *pipelinev1.GetContentTraceRequest) (*pipelinev1.GetContentTraceResponse, error) {
	s.logger.Debug("GetContentTrace called",
		logging.F("content_id", req.ContentId),
		logging.F("verbose", req.Verbose),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "Database not available")
	}

	// Add timeout to prevent hanging
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Query service_logs table for trace events
	query := `
		SELECT
			timestamp,
			level,
			service,
			message,
			fields
		FROM service_logs
		WHERE trace_id = $1
		ORDER BY timestamp ASC
	`

	rows, err := s.db.QueryContext(queryCtx, query, req.ContentId)
	if err != nil {
		// Check if timeout occurred
		if queryCtx.Err() == context.DeadlineExceeded {
			s.logger.Warn("Content trace query timed out",
				logging.F("content_id", req.ContentId),
			)
			return nil, status.Error(codes.DeadlineExceeded, "query timed out")
		}
		s.logger.Error("Error querying trace events", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to query trace events: %v", err)
	}
	defer rows.Close()

	var events []*pipelinev1.TraceEvent
	for rows.Next() {
		var (
			timestamp time.Time
			level     string
			service   string
			message   string
			fields    sql.NullString
		)

		if err := rows.Scan(&timestamp, &level, &service, &message, &fields); err != nil {
			s.logger.Error("Error scanning trace event", logging.Err(err))
			continue
		}

		event := &pipelinev1.TraceEvent{
			Timestamp: timestamppb.New(timestamp),
			Stage:     service,
			Action:    level,
			Message:   message,
			Details:   make(map[string]string),
		}

		// Parse fields if verbose mode is enabled
		if req.Verbose && fields.Valid {
			// Store raw JSONB for now
			event.Details["fields"] = fields.String
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("Error iterating trace events", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to iterate trace events: %v", err)
	}

	s.logger.Info("Content trace retrieved",
		logging.F("content_id", req.ContentId),
		logging.F("event_count", len(events)),
	)

	return &pipelinev1.GetContentTraceResponse{
		ContentId: req.ContentId,
		Events:    events,
	}, nil
}

// ListDeletedSources lists soft-deleted sources.
func (s *Service) ListDeletedSources(ctx context.Context, req *pipelinev1.ListDeletedSourcesRequest) (*pipelinev1.ListDeletedSourcesResponse, error) {
	s.logger.Debug("ListDeletedSources called",
		logging.F("limit", req.Limit),
	)

	sources, err := s.repo.ListDeletedSources(ctx, int(req.Limit))
	if err != nil {
		s.logger.Error("Error listing deleted sources", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list deleted sources: %v", err)
	}

	protoSources := make([]*pipelinev1.DeletedSource, len(sources))
	for i, src := range sources {
		protoSources[i] = deletedSourceToProto(&src)
	}

	return &pipelinev1.ListDeletedSourcesResponse{
		Sources: protoSources,
	}, nil
}

// UndeleteSource restores a soft-deleted source.
func (s *Service) UndeleteSource(ctx context.Context, req *pipelinev1.UndeleteSourceRequest) (*pipelinev1.UndeleteSourceResponse, error) {
	s.logger.Info("UndeleteSource called",
		logging.F("source_id", req.SourceId),
	)

	if req.SourceId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "source_id is required")
	}

	err := s.repo.UndeleteSource(ctx, req.SourceId)
	if err != nil {
		s.logger.Error("Error undeleting source", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to undelete source: %v", err)
	}

	s.logger.Info("Source undeleted",
		logging.F("source_id", req.SourceId),
	)

	return &pipelinev1.UndeleteSourceResponse{
		Success: true,
		Message: fmt.Sprintf("Source %d restored successfully", req.SourceId),
	}, nil
}

// deletedSourceToProto converts a DeletedSource to proto format.
func deletedSourceToProto(s *pipeline.DeletedSource) *pipelinev1.DeletedSource {
	if s == nil {
		return nil
	}
	proto := &pipelinev1.DeletedSource{
		Id:               s.ID,
		SourceSystem:     s.SourceSystem,
		ExternalId:       s.ExternalID,
		ProcessingStatus: s.ProcessingStatus,
	}
	if s.DeletedAt != nil {
		proto.DeletedAt = timestamppb.New(*s.DeletedAt)
	}
	if s.DeletedBy != nil {
		proto.DeletedBy = *s.DeletedBy
	}
	if s.DeletionReason != nil {
		proto.DeletionReason = *s.DeletionReason
	}
	return proto
}
