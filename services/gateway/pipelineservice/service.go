// Package pipelineservice implements the PipelineService gRPC server.
package pipelineservice

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/routing"
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

	// Check if Temporal client is available
	if s.temporalClient == nil {
		return nil, status.Error(codes.Unavailable, "Temporal client not configured")
	}

	// Read max_concurrent from pipeline_operational_config
	maxConcurrent, err := s.getMaxConcurrent(ctx)
	if err != nil {
		s.logger.Error("Error reading max_concurrent config", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to read concurrency config: %v", err)
	}

	// Count in-flight sources (processing_status = 'processing')
	inFlightCount, err := s.countInFlightSources(ctx)
	if err != nil {
		s.logger.Error("Error counting in-flight sources", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to count in-flight sources: %v", err)
	}

	// Count pending sources BEFORE kicking (for accurate pending_count in response)
	pendingCount, err := s.countPendingSources(ctx, req.SourceTag)
	if err != nil {
		s.logger.Error("Error counting pending sources", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to count pending sources: %v", err)
	}

	// Calculate available slots
	availableSlots := maxConcurrent - inFlightCount
	if availableSlots <= 0 {
		s.logger.Info("No available concurrency slots",
			logging.F("max_concurrent", maxConcurrent),
			logging.F("in_flight_count", inFlightCount),
		)
		return &pipelinev1.KickProcessingResponse{
			QueuedCount:    0,
			Message:        fmt.Sprintf("No available slots (max: %d, in-flight: %d)", maxConcurrent, inFlightCount),
			InFlightCount:  int32(inFlightCount),
			PendingCount:   pendingCount,
			MaxConcurrent:  int32(maxConcurrent),
		}, nil
	}

	// Apply user-requested limit if provided
	limit := availableSlots
	if req.Limit > 0 && int(req.Limit) < availableSlots {
		limit = int(req.Limit)
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
		input := pkgtemporal.SLMPipelineInput{
			TenantID:        src.TenantID,
			TenantName:      src.TenantName,
			SourceID:        src.ID,
			ContentID:       src.ContentID,
			ContentHash:     src.ContentHash,
			JobID:           workflowID, // Use workflow ID as job ID for tracing
		}
		scheduleToClose, err := s.scheduleToCloseTimeout(ctx, int(pendingCount))
		if err != nil {
			s.logger.Error("Error reading backpressure config", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to read backpressure config: %v", err)
		}
		opts := client.StartWorkflowOptions{
			ID:                       workflowID,
			TaskQueue:                "penfold-main",
			WorkflowExecutionTimeout: scheduleToClose,
		}
		var wfErr error
		_, wfErr = s.temporalClient.ExecuteWorkflow(ctx, opts, "SLMPipelineWorkflow", input)
		if wfErr != nil {
			s.logger.Warn("Failed to start workflow for source",
				logging.F("source_id", src.ID),
				logging.F("workflow_id", workflowID),
				logging.Err(wfErr),
			)
			// Continue with other sources
			continue
		}
		s.logger.Info("Started SLMPipelineWorkflow",
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
		logging.F("max_concurrent", maxConcurrent),
		logging.F("in_flight_count", inFlightCount),
		logging.F("available_slots", availableSlots),
	)

	return &pipelinev1.KickProcessingResponse{
		QueuedCount:    int64(startedCount),
		Message:        message,
		InFlightCount:  int32(inFlightCount),
		PendingCount:   pendingCount,
		MaxConcurrent:  int32(maxConcurrent),
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

	// Return empty queue stats for now
	// Full implementation would query Temporal for actual workflow counts
	// and database for pending/processing item counts
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
	defer rows.Close() //nolint:errcheck

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

// DescribePipeline retrieves pipeline stage registry information.
func (s *Service) DescribePipeline(ctx context.Context, req *pipelinev1.DescribePipelineRequest) (*pipelinev1.DescribePipelineResponse, error) {
	s.logger.Debug("DescribePipeline called",
		logging.F("stage", req.Stage),
	)

	stages, err := s.repo.ListStages(ctx, req.Stage)
	if err != nil {
		s.logger.Error("Error listing stages", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list stages: %v", err)
	}

	protoStages := make([]*pipelinev1.StageInfo, len(stages))
	for i, stage := range stages {
		protoStages[i] = stageInfoToProto(&stage)
	}

	return &pipelinev1.DescribePipelineResponse{
		Stages: protoStages,
	}, nil
}

// GetPrompt retrieves the active prompt for a stage or a specific version.
func (s *Service) GetPrompt(ctx context.Context, req *pipelinev1.GetPromptRequest) (*pipelinev1.GetPromptResponse, error) {
	s.logger.Debug("GetPrompt called",
		logging.F("stage", req.Stage),
		logging.F("version", req.Version),
	)

	if req.Stage == "" {
		return nil, status.Error(codes.InvalidArgument, "stage is required")
	}

	prompt, err := s.repo.GetPromptByStage(ctx, req.Stage, int(req.Version))
	if err != nil {
		s.logger.Error("Error getting prompt", logging.Err(err))
		return nil, status.Errorf(codes.NotFound, "prompt not found: %v", err)
	}

	return &pipelinev1.GetPromptResponse{
		Prompt: promptTemplateToProto(prompt),
	}, nil
}

// ListPromptVersions lists all prompt versions for a stage.
func (s *Service) ListPromptVersions(ctx context.Context, req *pipelinev1.ListPromptVersionsRequest) (*pipelinev1.ListPromptVersionsResponse, error) {
	s.logger.Debug("ListPromptVersions called",
		logging.F("stage", req.Stage),
	)

	if req.Stage == "" {
		return nil, status.Error(codes.InvalidArgument, "stage is required")
	}

	versions, err := s.repo.ListPromptVersions(ctx, req.Stage)
	if err != nil {
		s.logger.Error("Error listing prompt versions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list versions: %v", err)
	}

	protoVersions := make([]*pipelinev1.PromptTemplate, len(versions))
	for i, v := range versions {
		protoVersions[i] = promptTemplateToProto(&v)
	}

	return &pipelinev1.ListPromptVersionsResponse{
		Versions: protoVersions,
	}, nil
}

// UpdatePrompt creates a new prompt version and sets it as active.
func (s *Service) UpdatePrompt(ctx context.Context, req *pipelinev1.UpdatePromptRequest) (*pipelinev1.UpdatePromptResponse, error) {
	s.logger.Info("UpdatePrompt called",
		logging.F("stage", req.Stage),
		logging.F("created_by", req.CreatedBy),
	)

	if req.Stage == "" {
		return nil, status.Error(codes.InvalidArgument, "stage is required")
	}
	if req.Content == "" {
		return nil, status.Error(codes.InvalidArgument, "content is required")
	}

	prompt, err := s.repo.CreatePromptVersion(ctx, req.Stage, req.Content, req.Description, req.CreatedBy)
	if err != nil {
		s.logger.Error("Error creating prompt version", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create prompt version: %v", err)
	}

	s.logger.Info("Prompt version created",
		logging.F("stage", req.Stage),
		logging.F("version", prompt.Version),
	)

	return &pipelinev1.UpdatePromptResponse{
		Prompt:  promptTemplateToProto(prompt),
		Message: fmt.Sprintf("Created version %d for stage %s", prompt.Version, prompt.Stage),
	}, nil
}

// RollbackPrompt activates a previous prompt version.
func (s *Service) RollbackPrompt(ctx context.Context, req *pipelinev1.RollbackPromptRequest) (*pipelinev1.RollbackPromptResponse, error) {
	s.logger.Info("RollbackPrompt called",
		logging.F("stage", req.Stage),
		logging.F("version", req.Version),
	)

	if req.Stage == "" {
		return nil, status.Error(codes.InvalidArgument, "stage is required")
	}
	if req.Version <= 0 {
		return nil, status.Error(codes.InvalidArgument, "version must be positive")
	}

	prompt, err := s.repo.ActivatePromptVersion(ctx, req.Stage, int(req.Version))
	if err != nil {
		s.logger.Error("Error rolling back prompt", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to rollback prompt: %v", err)
	}

	s.logger.Info("Prompt rolled back",
		logging.F("stage", req.Stage),
		logging.F("version", req.Version),
	)

	return &pipelinev1.RollbackPromptResponse{
		Prompt:  promptTemplateToProto(prompt),
		Message: fmt.Sprintf("Activated version %d for stage %s", prompt.Version, prompt.Stage),
	}, nil
}

// ExportPrompt exports prompt templates as portable JSON.
func (s *Service) ExportPrompt(ctx context.Context, req *pipelinev1.ExportPromptRequest) (*pipelinev1.ExportPromptResponse, error) {
	s.logger.Debug("ExportPrompt called",
		logging.F("stage", req.Stage),
		logging.F("version", req.Version),
	)

	var prompts []pipeline.PromptTemplate
	var err error

	if req.Stage != "" {
		if req.Version > 0 {
			// Export specific version
			prompt, err := s.repo.GetPromptByStage(ctx, req.Stage, int(req.Version))
			if err != nil {
				s.logger.Error("Error getting prompt", logging.Err(err))
				return nil, status.Errorf(codes.NotFound, "prompt not found: %v", err)
			}
			prompts = []pipeline.PromptTemplate{*prompt}
		} else {
			// Export active version for this stage
			prompt, err := s.repo.GetPromptByStage(ctx, req.Stage, 0)
			if err != nil {
				s.logger.Error("Error getting prompt", logging.Err(err))
				return nil, status.Errorf(codes.NotFound, "prompt not found: %v", err)
			}
			prompts = []pipeline.PromptTemplate{*prompt}
		}
	} else {
		// Export all active versions for all stages
		stages, err := s.repo.ListStages(ctx, "")
		if err != nil {
			s.logger.Error("Error listing stages", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to list stages: %v", err)
		}

		for _, stage := range stages {
			if stage.HasPrompt {
				prompt, err := s.repo.GetPromptByStage(ctx, stage.Stage, 0)
				if err != nil {
					// Skip stages without active prompts
					continue
				}
				prompts = append(prompts, *prompt)
			}
		}
	}

	// Convert to JSON (using a simple struct for export format)
	type ExportedPrompt struct {
		Stage       string `json:"stage"`
		Version     int    `json:"version"`
		Content     string `json:"content"`
		Description string `json:"description,omitempty"`
	}

	exported := make([]ExportedPrompt, len(prompts))
	for i, p := range prompts {
		exported[i] = ExportedPrompt{
			Stage:   p.Stage,
			Version: p.Version,
			Content: p.Content,
		}
		if p.Description != nil {
			exported[i].Description = *p.Description
		}
	}

	jsonBytes, err := json.Marshal(exported)
	if err != nil {
		s.logger.Error("Error marshaling prompts to JSON", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to export prompts: %v", err)
	}

	return &pipelinev1.ExportPromptResponse{
		Json: string(jsonBytes),
	}, nil
}

// GetSourceHistory retrieves pipeline execution history for a source.
func (s *Service) GetSourceHistory(ctx context.Context, req *pipelinev1.GetSourceHistoryRequest) (*pipelinev1.GetSourceHistoryResponse, error) {
	s.logger.Debug("GetSourceHistory called",
		logging.F("source_id", req.SourceId),
		logging.F("stage", req.Stage),
	)

	if req.SourceId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "source_id is required")
	}

	runs, err := s.repo.ListSourceHistory(ctx, req.SourceId, req.Stage)
	if err != nil {
		s.logger.Error("Error getting source history", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get source history: %v", err)
	}

	protoRuns := make([]*pipelinev1.PipelineRun, len(runs))
	for i, run := range runs {
		protoRuns[i] = pipelineRunToProto(&run)
	}

	return &pipelinev1.GetSourceHistoryResponse{
		SourceId: req.SourceId,
		Runs:     protoRuns,
	}, nil
}

// ReprocessDryRun calculates the impact of reprocessing a stage.
func (s *Service) ReprocessDryRun(ctx context.Context, req *pipelinev1.ReprocessDryRunRequest) (*pipelinev1.ReprocessDryRunResponse, error) {
	s.logger.Debug("ReprocessDryRun called",
		logging.F("stage", req.Stage),
		logging.F("source_tag", req.SourceTag),
	)

	if req.Stage == "" {
		return nil, status.Error(codes.InvalidArgument, "stage is required")
	}

	// Get downstream stages
	downstreamStages, err := s.repo.GetDownstreamStages(ctx, req.Stage)
	if err != nil {
		s.logger.Error("Error getting downstream stages", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get downstream stages: %v", err)
	}

	// Include the requested stage in affected stages
	affectedStages := append([]string{req.Stage}, downstreamStages...)

	// Count sources that would be affected
	sourceCount, err := s.repo.CountSourcesByStage(ctx, req.Stage, req.SourceTag, req.SourceIds)
	if err != nil {
		s.logger.Error("Error counting sources", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to count sources: %v", err)
	}

	// Estimate duration (rough estimate: 2 seconds per source per stage)
	estimatedDuration := sourceCount * int64(len(affectedStages)) * 2

	message := fmt.Sprintf("Reprocessing stage %s would affect %d stage(s) and %d source(s)", req.Stage, len(affectedStages), sourceCount)
	if req.SourceTag != "" {
		message += fmt.Sprintf(" (filtered by source_tag: %s)", req.SourceTag)
	}

	return &pipelinev1.ReprocessDryRunResponse{
		AffectedStages:           affectedStages,
		SourceCount:              sourceCount,
		EstimatedDurationSeconds: estimatedDuration,
		Message:                  message,
	}, nil
}

// DiffPipelineRuns compares two pipeline runs for a source to show what changed.
func (s *Service) DiffPipelineRuns(ctx context.Context, req *pipelinev1.DiffPipelineRunsRequest) (*pipelinev1.DiffPipelineRunsResponse, error) {
	s.logger.Debug("DiffPipelineRuns called",
		logging.F("source_id", req.SourceId),
		logging.F("run_id_a", req.RunIdA),
		logging.F("run_id_b", req.RunIdB),
	)

	// Validate source_id is provided
	if req.SourceId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "source_id is required")
	}

	// Check repository is available
	if s.repo == nil {
		return nil, status.Error(codes.Unavailable, "pipeline repository not configured")
	}

	// Convert optional run_id_a and run_id_b from proto (pointer) to int64 (0 if nil)
	var runIDA, runIDB int64
	if req.RunIdA != nil {
		runIDA = *req.RunIdA
	}
	if req.RunIdB != nil {
		runIDB = *req.RunIdB
	}

	// Call repository to get diffs
	diffs, err := s.repo.DiffPipelineRuns(ctx, req.SourceId, runIDA, runIDB)
	if err != nil {
		s.logger.Error("Error diffing pipeline runs",
			logging.F("source_id", req.SourceId),
			logging.F("run_id_a", runIDA),
			logging.F("run_id_b", runIDB),
			logging.Err(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to diff pipeline runs: %v", err)
	}

	// Map []pipeline.StageDiff to []*pipelinev1.StageDiff
	protoDiffs := make([]*pipelinev1.StageDiff, len(diffs))
	for i, diff := range diffs {
		protoDiffs[i] = &pipelinev1.StageDiff{
			Stage:      diff.Stage,
			Field:      diff.Field,
			OldValue:   diff.OldValue,
			NewValue:   diff.NewValue,
			ChangeType: mapChangeType(diff.ChangeType),
		}
	}

	return &pipelinev1.DiffPipelineRunsResponse{
		Diffs: protoDiffs,
	}, nil
}

// mapChangeType converts string change type to proto enum.
func mapChangeType(changeType string) pipelinev1.ChangeType {
	switch changeType {
	case "added":
		return pipelinev1.ChangeType_CHANGE_TYPE_ADDED
	case "removed":
		return pipelinev1.ChangeType_CHANGE_TYPE_REMOVED
	case "modified":
		return pipelinev1.ChangeType_CHANGE_TYPE_MODIFIED
	default:
		return pipelinev1.ChangeType_CHANGE_TYPE_UNSPECIFIED
	}
}

// Helper conversion functions

func stageInfoToProto(s *pipeline.StageInfo) *pipelinev1.StageInfo {
	if s == nil {
		return nil
	}
	return &pipelinev1.StageInfo{
		Stage:                s.Stage,
		DisplayName:          s.DisplayName,
		Description:          s.Description,
		StageType:            s.StageType,
		ModelDependent:       s.ModelDependent,
		HasPrompt:            s.HasPrompt,
		DependsOn:            s.DependsOn,
		Downstream:           s.Downstream,
		ActivePromptVersion:  int32(s.ActivePromptVersion),
		ActiveModelId:        s.ActiveModelID,
	}
}

func promptTemplateToProto(pt *pipeline.PromptTemplate) *pipelinev1.PromptTemplate {
	if pt == nil {
		return nil
	}
	proto := &pipelinev1.PromptTemplate{
		Id:        pt.ID,
		Stage:     pt.Stage,
		Version:   int32(pt.Version),
		Content:   pt.Content,
		IsActive:  pt.IsActive,
		CreatedAt: timestamppb.New(pt.CreatedAt),
	}
	if pt.Description != nil {
		proto.Description = *pt.Description
	}
	if pt.CreatedBy != nil {
		proto.CreatedBy = *pt.CreatedBy
	}
	return proto
}

func pipelineRunToProto(pr *pipeline.PipelineRun) *pipelinev1.PipelineRun {
	if pr == nil {
		return nil
	}
	proto := &pipelinev1.PipelineRun{
		Id:        pr.ID,
		SourceId:  pr.SourceID,
		Stage:     pr.Stage,
		Status:    pr.Status,
		CreatedAt: timestamppb.New(pr.CreatedAt),
	}
	if pr.ModelID != nil {
		proto.ModelId = *pr.ModelID
	}
	if pr.PromptVersion != nil {
		proto.PromptVersion = int32(*pr.PromptVersion)
	}
	if pr.ConfigHash != nil {
		proto.ConfigHash = *pr.ConfigHash
	}
	if pr.DurationMS != nil {
		proto.DurationMs = int64(*pr.DurationMS)
	}
	return proto
}

// GetTimeoutConfig retrieves timeout configuration entries.
func (s *Service) GetTimeoutConfig(ctx context.Context, req *pipelinev1.GetTimeoutConfigRequest) (*pipelinev1.GetTimeoutConfigResponse, error) {
	s.logger.Debug("GetTimeoutConfig called",
		logging.F("key_filter", req.Key),
	)

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := s.defaultTenantID(ctx)

	// Build query with optional key prefix filter against pipeline_operational_config.
	// Returns all operational config keys (timeout, embedding, concurrency, etc.).
	query := `
		SELECT key, value
		FROM pipeline_operational_config
		WHERE tenant_id = $1
	`
	args := []interface{}{tenantID}

	if req.Key != "" {
		query += " AND key LIKE $2"
		args = append(args, req.Key+"%")
	}

	query += " ORDER BY key"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		s.logger.Error("Error querying pipeline_operational_config", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to query timeout config: %v", err)
	}
	defer rows.Close() //nolint:errcheck

	var entries []*pipelinev1.TimeoutEntry
	for rows.Next() {
		var entry pipelinev1.TimeoutEntry

		err := rows.Scan(
			&entry.Key,
			&entry.Value,
		)
		if err != nil {
			s.logger.Error("Error scanning timeout entry", logging.Err(err))
			continue
		}

		entries = append(entries, &entry)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("Error iterating timeout entries", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to iterate timeout config: %v", err)
	}

	s.logger.Info("Timeout config retrieved",
		logging.F("entry_count", len(entries)),
		logging.F("key_filter", req.Key),
	)

	return &pipelinev1.GetTimeoutConfigResponse{
		Entries: entries,
	}, nil
}

// UpdateTimeoutConfig updates a timeout configuration value.
func (s *Service) UpdateTimeoutConfig(ctx context.Context, req *pipelinev1.UpdateTimeoutConfigRequest) (*pipelinev1.UpdateTimeoutConfigResponse, error) {
	s.logger.Info("UpdateTimeoutConfig called",
		logging.F("key", req.Key),
		logging.F("value", req.Value),
		logging.F("updated_by", req.UpdatedBy),
	)

	// Validate required fields
	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	if req.Value == "" {
		return nil, status.Error(codes.InvalidArgument, "value is required")
	}
	if req.UpdatedBy == "" {
		return nil, status.Error(codes.InvalidArgument, "updated_by is required")
	}
	if req.Reason == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	// Route model.stage.* keys to model_config table.
	// The CLI sends key="model.stage.<stage>" when setting a stage model via
	// `penf pipeline stage set <stage> --model <model>`.
	if strings.HasPrefix(req.Key, "model.stage.") {
		stageName := strings.TrimPrefix(req.Key, "model.stage.")

		_, err := s.db.ExecContext(ctx, `
			INSERT INTO model_config (tenant_id, config_key, model_name, updated_at, updated_by)
			VALUES ($1, $2, $3, NOW(), $4)
			ON CONFLICT (tenant_id, config_key)
			DO UPDATE SET model_name = EXCLUDED.model_name, updated_at = NOW(), updated_by = EXCLUDED.updated_by
		`, s.defaultTenantID(ctx), "stage."+stageName, req.Value, req.UpdatedBy)
		if err != nil {
			s.logger.Error("Error writing model config", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to update model config: %v", err)
		}

		return &pipelinev1.UpdateTimeoutConfigResponse{
			Entry: &pipelinev1.TimeoutEntry{
				Key:       req.Key,
				Value:     req.Value,
				UpdatedBy: req.UpdatedBy,
			},
			Message: fmt.Sprintf("Updated model for stage %s to %s", stageName, req.Value),
		}, nil
	}

	// Route timeout.stage.*.start_to_close keys to pipeline_definitions.timeout_seconds
	if strings.HasPrefix(req.Key, "timeout.stage.") && strings.HasSuffix(req.Key, ".start_to_close") {
		// Extract stage name: "timeout.stage.triage.start_to_close" → "triage"
		parts := strings.Split(req.Key, ".")
		if len(parts) != 4 {
			return nil, status.Errorf(codes.InvalidArgument, "invalid timeout key format: %s", req.Key)
		}
		stageName := parts[2]

		// Parse timeout value as duration, convert to seconds
		dur, err := time.ParseDuration(req.Value)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid duration: %v", err)
		}
		timeoutSeconds := int(dur.Seconds())

		tenantID := s.defaultTenantID(ctx)

		// Update pipeline_definitions for ALL pipelines containing this stage
		_, err = s.db.ExecContext(ctx, `
			UPDATE pipeline_definitions
			SET timeout_seconds = $1
			WHERE tenant_id = $2 AND stage = $3
		`, timeoutSeconds, tenantID, stageName)
		if err != nil {
			s.logger.Error("Error updating pipeline definition timeout", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to update timeout: %v", err)
		}

		return &pipelinev1.UpdateTimeoutConfigResponse{
			Entry: &pipelinev1.TimeoutEntry{
				Key:       req.Key,
				Value:     req.Value,
				UpdatedBy: req.UpdatedBy,
			},
			Message: fmt.Sprintf("Updated timeout for stage %s to %s (%ds)", stageName, req.Value, timeoutSeconds),
		}, nil
	}

	// For heartbeat keys, just acknowledge (heartbeat is derived from start_to_close / 2)
	if strings.HasPrefix(req.Key, "timeout.stage.") && strings.HasSuffix(req.Key, ".heartbeat") {
		return &pipelinev1.UpdateTimeoutConfigResponse{
			Entry: &pipelinev1.TimeoutEntry{
				Key:       req.Key,
				Value:     req.Value,
				UpdatedBy: req.UpdatedBy,
			},
			Message: fmt.Sprintf("Heartbeat is now derived from start_to_close / 2; this key is deprecated"),
		}, nil
	}

	// Read the current value from pipeline_operational_config.
	tenantID := s.defaultTenantID(ctx)
	var entry pipelinev1.TimeoutEntry
	var previousValue string

	err := s.db.QueryRowContext(ctx, `
		SELECT key, value
		FROM pipeline_operational_config
		WHERE tenant_id = $1 AND key = $2
	`, tenantID, req.Key).Scan(
		&entry.Key,
		&previousValue,
	)

	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "unknown config key: %s", req.Key)
	}
	if err != nil {
		s.logger.Error("Error getting config entry", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get config entry: %v", err)
	}

	// Update the value in pipeline_operational_config
	_, err = s.db.ExecContext(ctx, `
		UPDATE pipeline_operational_config
		SET value = $1, updated_at = now()
		WHERE tenant_id = $2 AND key = $3
	`, req.Value, tenantID, req.Key)
	if err != nil {
		s.logger.Error("Error updating config", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update config: %v", err)
	}

	// Build response with updated entry
	entry.Value = req.Value
	entry.UpdatedAt = time.Now().Format(time.RFC3339)
	entry.UpdatedBy = req.UpdatedBy

	message := fmt.Sprintf("Updated %s from %s to %s (reason: %s)", req.Key, previousValue, req.Value, req.Reason)

	s.logger.Info("Timeout config updated",
		logging.F("key", req.Key),
		logging.F("previous_value", previousValue),
		logging.F("new_value", req.Value),
		logging.F("updated_by", req.UpdatedBy),
		logging.F("reason", req.Reason),
	)

	return &pipelinev1.UpdateTimeoutConfigResponse{
		Entry:         &entry,
		PreviousValue: previousValue,
		Message:       message,
	}, nil
}

// InspectStage retrieves pipeline stage IO data for debugging and inspection.
func (s *Service) InspectStage(ctx context.Context, req *pipelinev1.InspectStageRequest) (*pipelinev1.InspectStageResponse, error) {
	s.logger.Debug("InspectStage called",
		logging.F("source_id", req.SourceId),
		logging.F("stage", req.Stage),
		logging.F("limit", req.Limit),
	)

	// Validate input
	if req.SourceId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "source_id is required")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 3
	}

	// Call repository to get stage IO data
	runs, err := s.repo.GetStageIO(ctx, req.SourceId, req.Stage, limit)
	if err != nil {
		s.logger.Error("Error getting stage IO", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get stage IO: %v", err)
	}

	// Convert to proto
	protoRuns := make([]*pipelinev1.PipelineRunDetail, len(runs))
	for i, run := range runs {
		protoRuns[i] = pipelineRunToDetail(&run)
	}

	s.logger.Info("Stage IO retrieved",
		logging.F("source_id", req.SourceId),
		logging.F("stage", req.Stage),
		logging.F("run_count", len(runs)),
	)

	return &pipelinev1.InspectStageResponse{
		Runs: protoRuns,
	}, nil
}

// DiffStageRuns compares two pipeline runs to identify differences.
func (s *Service) DiffStageRuns(ctx context.Context, req *pipelinev1.DiffStageRunsRequest) (*pipelinev1.DiffStageRunsResponse, error) {
	s.logger.Debug("DiffStageRuns called",
		logging.F("run_id_a", req.RunIdA),
		logging.F("run_id_b", req.RunIdB),
	)

	// Validate
	if req.RunIdA <= 0 || req.RunIdB <= 0 {
		return nil, status.Error(codes.InvalidArgument, "both run_id_a and run_id_b are required")
	}

	// Fetch both runs
	runA, err := s.repo.GetRunByID(ctx, req.RunIdA)
	if err != nil {
		s.logger.Error("Error getting run A", logging.Err(err))
		return nil, status.Errorf(codes.NotFound, "run %d not found: %v", req.RunIdA, err)
	}

	runB, err := s.repo.GetRunByID(ctx, req.RunIdB)
	if err != nil {
		s.logger.Error("Error getting run B", logging.Err(err))
		return nil, status.Errorf(codes.NotFound, "run %d not found: %v", req.RunIdB, err)
	}

	// Compute diff of ParsedData
	diffSummary, diffJSON := computeJSONDiff(runA.ParsedData, runB.ParsedData)

	s.logger.Info("Diff computed",
		logging.F("run_id_a", req.RunIdA),
		logging.F("run_id_b", req.RunIdB),
		logging.F("stage", runA.Stage),
	)

	return &pipelinev1.DiffStageRunsResponse{
		Stage:       runA.Stage,
		RunAStatus:  runA.Status,
		RunBStatus:  runB.Status,
		RunATime:    timestamppb.New(runA.CreatedAt),
		RunBTime:    timestamppb.New(runB.CreatedAt),
		DiffSummary: diffSummary,
		DiffJson:    diffJSON,
	}, nil
}

// pipelineRunToDetail converts a PipelineRun to PipelineRunDetail proto.
func pipelineRunToDetail(run *pipeline.PipelineRun) *pipelinev1.PipelineRunDetail {
	if run == nil {
		return nil
	}
	detail := &pipelinev1.PipelineRunDetail{
		Id:        run.ID,
		SourceId:  run.SourceID,
		Stage:     run.Stage,
		Status:    run.Status,
		CreatedAt: timestamppb.New(run.CreatedAt),
	}
	if run.ModelID != nil {
		detail.ModelId = *run.ModelID
	}
	if run.PromptVersion != nil {
		detail.PromptVersion = int32(*run.PromptVersion)
	}
	if run.DurationMS != nil {
		detail.DurationMs = int64(*run.DurationMS)
	}
	// IO data
	if run.InputData != nil || run.OutputData != nil || run.ParsedData != nil {
		detail.Io = &pipelinev1.StageIOData{
			InputData:  string(run.InputData),
			OutputData: string(run.OutputData),
			ParsedData: string(run.ParsedData),
		}
	}
	return detail
}

// computeJSONDiff compares two JSON blobs and returns a summary and detailed diff.
func computeJSONDiff(a, b json.RawMessage) (summary string, diffJSON string) {
	// Simple implementation: unmarshal and compare top-level keys
	var mapA, mapB map[string]interface{}

	// Parse both JSON documents
	errA := json.Unmarshal(a, &mapA)
	errB := json.Unmarshal(b, &mapB)

	// Handle parsing errors
	if errA != nil && errB != nil {
		return "Both documents failed to parse as JSON", "{\"error\": \"invalid_json\"}"
	}
	if errA != nil {
		return "Document A is not valid JSON", "{\"error\": \"invalid_json_a\"}"
	}
	if errB != nil {
		return "Document B is not valid JSON", "{\"error\": \"invalid_json_b\"}"
	}

	// Track differences
	added := []string{}
	removed := []string{}
	changed := []string{}

	// Find removed and changed keys
	for key := range mapA {
		if _, exists := mapB[key]; !exists {
			removed = append(removed, key)
		} else {
			// Simple comparison - just check if values differ
			valA, _ := json.Marshal(mapA[key])
			valB, _ := json.Marshal(mapB[key])
			if string(valA) != string(valB) {
				changed = append(changed, key)
			}
		}
	}

	// Find added keys
	for key := range mapB {
		if _, exists := mapA[key]; !exists {
			added = append(added, key)
		}
	}

	// Build summary
	if len(added) == 0 && len(removed) == 0 && len(changed) == 0 {
		summary = "No differences detected"
	} else {
		summary = fmt.Sprintf("%d added, %d removed, %d changed", len(added), len(removed), len(changed))
	}

	// Build structured diff
	diffMap := map[string]interface{}{
		"added":   added,
		"removed": removed,
		"changed": changed,
	}
	diffBytes, _ := json.Marshal(diffMap)
	diffJSON = string(diffBytes)

	return summary, diffJSON
}

// =============================================================================
// Concurrency Configuration RPCs
// =============================================================================

// GetConcurrencyConfig retrieves current concurrency configuration and queue state.
func (s *Service) GetConcurrencyConfig(ctx context.Context, req *pipelinev1.GetConcurrencyConfigRequest) (*pipelinev1.GetConcurrencyConfigResponse, error) {
	s.logger.Debug("GetConcurrencyConfig called")

	// Read max_concurrent from pipeline_operational_config
	maxConcurrent, err := s.getMaxConcurrent(ctx)
	if err != nil {
		s.logger.Error("Error reading max_concurrent config", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to read concurrency config: %v", err)
	}

	// Count in-flight sources
	inFlightCount, err := s.countInFlightSources(ctx)
	if err != nil {
		s.logger.Error("Error counting in-flight sources", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to count in-flight sources: %v", err)
	}

	// Count pending sources (no filter)
	pendingCount, err := s.countPendingSources(ctx, "")
	if err != nil {
		s.logger.Error("Error counting pending sources", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to count pending sources: %v", err)
	}

	s.logger.Info("Concurrency config retrieved",
		logging.F("max_concurrent", maxConcurrent),
		logging.F("in_flight_count", inFlightCount),
		logging.F("pending_count", pendingCount),
	)

	return &pipelinev1.GetConcurrencyConfigResponse{
		MaxConcurrent:  int32(maxConcurrent),
		InFlightCount:  int32(inFlightCount),
		PendingCount:   pendingCount,
	}, nil
}

// SetConcurrencyConfig updates the maximum concurrent processing limit.
func (s *Service) SetConcurrencyConfig(ctx context.Context, req *pipelinev1.SetConcurrencyConfigRequest) (*pipelinev1.SetConcurrencyConfigResponse, error) {
	s.logger.Info("SetConcurrencyConfig called",
		logging.F("max_concurrent", req.MaxConcurrent),
		logging.F("updated_by", req.UpdatedBy),
	)

	// Validate max_concurrent >= 1
	if req.MaxConcurrent < 1 {
		return nil, status.Error(codes.InvalidArgument, "max_concurrent must be >= 1")
	}

	// Validate updated_by is not empty
	if req.UpdatedBy == "" {
		return nil, status.Error(codes.InvalidArgument, "updated_by is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	// Read previous value
	previousValue, err := s.getMaxConcurrent(ctx)
	if err != nil {
		s.logger.Error("Error reading previous max_concurrent", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to read previous value: %v", err)
	}

	// Update pipeline_operational_config using the helper
	err = s.setMaxConcurrent(ctx, int(req.MaxConcurrent), req.UpdatedBy)
	if err != nil {
		s.logger.Error("Error updating max_concurrent config", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update concurrency config: %v", err)
	}

	message := fmt.Sprintf("Updated max_concurrent from %d to %d", previousValue, req.MaxConcurrent)

	s.logger.Info("Concurrency config updated",
		logging.F("previous_value", previousValue),
		logging.F("new_value", req.MaxConcurrent),
		logging.F("updated_by", req.UpdatedBy),
	)

	return &pipelinev1.SetConcurrencyConfigResponse{
		MaxConcurrent:  req.MaxConcurrent,
		PreviousValue:  int32(previousValue),
		Message:        message,
	}, nil
}

// GetOperationalConfig retrieves a single operational config value by key.
func (s *Service) GetOperationalConfig(ctx context.Context, req *pipelinev1.GetOperationalConfigRequest) (*pipelinev1.GetOperationalConfigResponse, error) {
	s.logger.Debug("GetOperationalConfig called",
		logging.F("key", req.Key),
	)

	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := s.defaultTenantID(ctx)

	var value string
	err := s.db.QueryRowContext(ctx, `
		SELECT value
		FROM pipeline_operational_config
		WHERE tenant_id = $1 AND key = $2
	`, tenantID, req.Key).Scan(&value)

	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "config key %q not found", req.Key)
	}
	if err != nil {
		s.logger.Error("Error querying operational config", logging.F("key", req.Key), logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to query operational config: %v", err)
	}

	s.logger.Info("OperationalConfig retrieved", logging.F("key", req.Key))

	return &pipelinev1.GetOperationalConfigResponse{
		Key:   req.Key,
		Value: value,
	}, nil
}

// SetOperationalConfig upserts an operational config value by key.
func (s *Service) SetOperationalConfig(ctx context.Context, req *pipelinev1.SetOperationalConfigRequest) (*pipelinev1.SetOperationalConfigResponse, error) {
	s.logger.Info("SetOperationalConfig called",
		logging.F("key", req.Key),
	)

	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := s.defaultTenantID(ctx)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pipeline_operational_config (tenant_id, key, value)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, tenantID, req.Key, req.Value)
	if err != nil {
		s.logger.Error("Error upserting operational config", logging.F("key", req.Key), logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to upsert operational config: %v", err)
	}

	s.logger.Info("OperationalConfig updated", logging.F("key", req.Key))

	return &pipelinev1.SetOperationalConfigResponse{
		Updated: true,
	}, nil
}

// ListPendingSources lists pending sources in the processing queue.
func (s *Service) ListPendingSources(ctx context.Context, req *pipelinev1.ListPendingSourcesRequest) (*pipelinev1.ListPendingSourcesResponse, error) {
	s.logger.Debug("ListPendingSources called",
		logging.F("limit", req.Limit),
		logging.F("offset", req.Offset),
		logging.F("source_tag", req.SourceTag),
	)

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	// Set default limit
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	// Build query with optional source_tag filter
	query := `
		SELECT id, COALESCE(content_type, ''),
		       COALESCE(ingestion_metadata->>'source_tag', ''),
		       created_at,
		       COALESCE(size_bytes, 0),
		       COALESCE(external_id, '')
		FROM sources
		WHERE processing_status = 'pending'
		  AND is_deleted = false
		  AND ($1 = '' OR ingestion_metadata->>'source_tag' = $1)
		ORDER BY created_at ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.QueryContext(ctx, query, req.SourceTag, limit, offset)
	if err != nil {
		s.logger.Error("Error querying pending sources", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to query pending sources: %v", err)
	}
	defer rows.Close() //nolint:errcheck

	var sources []*pipelinev1.PendingSourceSummary
	for rows.Next() {
		var source pipelinev1.PendingSourceSummary
		var createdAt time.Time

		err := rows.Scan(
			&source.Id,
			&source.ContentType,
			&source.SourceTag,
			&createdAt,
			&source.SizeBytes,
			&source.ExternalId,
		)
		if err != nil {
			s.logger.Error("Error scanning pending source", logging.Err(err))
			continue
		}

		source.CreatedAt = timestamppb.New(createdAt)
		sources = append(sources, &source)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("Error iterating pending sources", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to iterate pending sources: %v", err)
	}

	// Get total count
	totalCount, err := s.countPendingSources(ctx, req.SourceTag)
	if err != nil {
		s.logger.Error("Error counting pending sources", logging.Err(err))
		totalCount = int64(len(sources))
	}

	s.logger.Info("Pending sources listed",
		logging.F("source_count", len(sources)),
		logging.F("total_count", totalCount),
		logging.F("source_tag", req.SourceTag),
	)

	return &pipelinev1.ListPendingSourcesResponse{
		Sources:    sources,
		TotalCount: totalCount,
	}, nil
}

// =============================================================================
// Helper functions for concurrency management
// =============================================================================

// getMaxConcurrent reads the max_concurrent value from pipeline_operational_config.
func (s *Service) getMaxConcurrent(ctx context.Context) (int, error) {
	if s.db == nil {
		return 1, fmt.Errorf("database not available")
	}

	tenantID := s.defaultTenantID(ctx)
	var valueStr string
	err := s.db.QueryRowContext(ctx, `
		SELECT value
		FROM pipeline_operational_config
		WHERE tenant_id = $1 AND key = 'pipeline.max_concurrent'
	`, tenantID).Scan(&valueStr)

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

// scheduleToCloseTimeout returns the WorkflowExecutionTimeout for a new workflow
// based on the current pending queue depth. Values are read from pipeline_operational_config:
//   - queue.backpressure.tier2_threshold (default: 100) — above this, use tier2 timeout
//   - queue.backpressure.tier1_threshold (default: 50)  — above this, use tier1 timeout
//   - queue.backpressure.tier2_timeout_hours (default: 4)
//   - queue.backpressure.tier1_timeout_hours (default: 2)
//   - queue.backpressure.default_timeout_hours (default: 1)
func (s *Service) scheduleToCloseTimeout(ctx context.Context, pendingCount int) (time.Duration, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not available")
	}

	tenantID := s.defaultTenantID(ctx)

	keys := []string{
		"queue.backpressure.tier1_threshold",
		"queue.backpressure.tier1_timeout_hours",
		"queue.backpressure.tier2_threshold",
		"queue.backpressure.tier2_timeout_hours",
		"queue.backpressure.default_timeout_hours",
	}
	defaults := map[string]int{
		"queue.backpressure.tier1_threshold":     50,
		"queue.backpressure.tier1_timeout_hours": 2,
		"queue.backpressure.tier2_threshold":     100,
		"queue.backpressure.tier2_timeout_hours": 4,
		"queue.backpressure.default_timeout_hours": 1,
	}

	cfg := make(map[string]int, len(keys))
	for _, key := range keys {
		var valueStr string
		err := s.db.QueryRowContext(ctx, `
			SELECT value
			FROM pipeline_operational_config
			WHERE tenant_id = $1 AND key = $2
		`, tenantID, key).Scan(&valueStr)

		if err == sql.ErrNoRows {
			def, ok := defaults[key]
			if !ok {
				return 0, fmt.Errorf("no default for config key %q", key)
			}
			cfg[key] = def
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("failed to query config key %q: %w", key, err)
		}

		v, err := strconv.Atoi(valueStr)
		if err != nil {
			return 0, fmt.Errorf("failed to parse config key %q as integer: %w", key, err)
		}
		cfg[key] = v
	}

	switch {
	case pendingCount > cfg["queue.backpressure.tier2_threshold"]:
		return time.Duration(cfg["queue.backpressure.tier2_timeout_hours"]) * time.Hour, nil
	case pendingCount > cfg["queue.backpressure.tier1_threshold"]:
		return time.Duration(cfg["queue.backpressure.tier1_timeout_hours"]) * time.Hour, nil
	default:
		return time.Duration(cfg["queue.backpressure.default_timeout_hours"]) * time.Hour, nil
	}
}

// setMaxConcurrent updates the max_concurrent value in pipeline_operational_config.
func (s *Service) setMaxConcurrent(ctx context.Context, value int, updatedBy string) error {
	if s.db == nil {
		return fmt.Errorf("database not available")
	}

	tenantID := s.defaultTenantID(ctx)
	result, err := s.db.ExecContext(ctx, `
		UPDATE pipeline_operational_config
		SET value = $1, updated_at = NOW()
		WHERE tenant_id = $2 AND key = 'pipeline.max_concurrent'
	`, strconv.Itoa(value), tenantID)

	if err != nil {
		return fmt.Errorf("failed to update max_concurrent: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("max_concurrent config not found")
	}

	return nil
}

// countInFlightSources counts sources with processing_status = 'processing'.
func (s *Service) countInFlightSources(ctx context.Context) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not available")
	}

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

// GetStageConfig retrieves unified per-stage configuration (model + timeout).
func (s *Service) GetStageConfig(ctx context.Context, req *pipelinev1.GetStageConfigRequest) (*pipelinev1.GetStageConfigResponse, error) {
	s.logger.Debug("GetStageConfig called",
		logging.F("tenant_id", req.TenantId),
		logging.F("stage", req.Stage),
	)

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	// Known pipeline stages
	stages := []string{"triage", "extract_ner", "extract_assertions", "analyze", "embed"}
	if req.Stage != "" {
		stages = []string{req.Stage}
	}

	// Build maps of per-stage config from pipeline_definitions.
	// Uses repo (pgx) instead of raw database/sql to avoid nullable-type scan issues
	// with the pgx/stdlib adapter (double-pointer destinations like **float64, **int32
	// silently fail through database/sql.Scan, causing LLM params to be dropped).
	timeoutSecsMap := make(map[string]int)       // stage -> timeout_seconds
	timeoutSourceMap := make(map[string]string)   // stage -> source
	temperatureMap := make(map[string]*float64)   // stage -> temperature
	maxTokensMap := make(map[string]*int32)       // stage -> max_tokens
	maxRetriesMap := make(map[string]*int32)       // stage -> max_retries

	pdStages, err := s.repo.GetPipelineStages(ctx, tenantID, "standard")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query stage config: %v", err)
	}

	for _, sd := range pdStages {
		if sd.TimeoutSeconds > 0 {
			timeoutSecsMap[sd.Stage] = sd.TimeoutSeconds
			timeoutSourceMap[sd.Stage] = "db"
		}
		if sd.Temperature != nil {
			temperatureMap[sd.Stage] = sd.Temperature
		}
		if sd.MaxTokens != nil {
			v := int32(*sd.MaxTokens)
			maxTokensMap[sd.Stage] = &v
		}
		if sd.MaxRetries != nil {
			v := int32(*sd.MaxRetries)
			maxRetriesMap[sd.Stage] = &v
		}
	}

	// Query model_config for per-stage models.
	// Resolution chain per stage:
	//   1. stage-specific override: config_key = "stage.{stage}"  → source "stage_config"
	//   2. global default:          config_key = "default.llm"    → source "global"
	//   3. hardcoded fallback:      "qwen2.5:7b"                  → source "default"
	modelMap := make(map[string]string)       // stage -> resolved model name
	modelSourceMap := make(map[string]string) // stage -> source label

	// Fetch global default once.
	const fallbackModel = "qwen2.5:7b"
	globalDefault := fallbackModel
	globalSource := "default"

	var globalModel string
	globalErr := s.db.QueryRowContext(ctx, `
		SELECT model_name
		FROM model_config
		WHERE tenant_id = $1 AND config_key = 'default.llm'
		LIMIT 1
	`, tenantID).Scan(&globalModel)
	if globalErr == nil && globalModel != "" {
		globalDefault = globalModel
		globalSource = "global"
	} else if globalErr != nil && globalErr != sql.ErrNoRows {
		// model_config table may not exist yet — non-fatal
		s.logger.Debug("Could not query model_config for default.llm", logging.Err(globalErr))
	}

	// For each stage, look up a stage-specific override first.
	for _, stage := range stages {
		stageKey := fmt.Sprintf("stage.%s", stage)
		var stageName string
		stageErr := s.db.QueryRowContext(ctx, `
			SELECT model_name
			FROM model_config
			WHERE tenant_id = $1 AND config_key = $2
			LIMIT 1
		`, tenantID, stageKey).Scan(&stageName)

		if stageErr == nil && stageName != "" {
			modelMap[stage] = stageName
			modelSourceMap[stage] = "stage_config"
		} else {
			// Fall back to global default (or hardcoded fallback).
			modelMap[stage] = globalDefault
			modelSourceMap[stage] = globalSource
		}
	}

	// Build response
	var entries []*pipelinev1.StageConfigEntry
	for _, stage := range stages {
		var timeout, heartbeat string
		if secs, ok := timeoutSecsMap[stage]; ok {
			timeout = fmt.Sprintf("%ds", secs)
			heartbeat = fmt.Sprintf("%ds", secs/2)
		} else {
			timeout = stageDefaultTimeout(stage)
			heartbeat = stageDefaultHeartbeat(stage)
		}

		timeoutSrc := "default"
		if src, ok := timeoutSourceMap[stage]; ok {
			timeoutSrc = src
		}

		modelSrc := "default"
		if src, ok := modelSourceMap[stage]; ok {
			modelSrc = src
		}

		entry := &pipelinev1.StageConfigEntry{
			Stage:         stage,
			Model:         modelMap[stage],
			Timeout:       timeout,
			Heartbeat:     heartbeat,
			ModelSource:   modelSrc,
			TimeoutSource: timeoutSrc,
		}
		if t, ok := temperatureMap[stage]; ok {
			v := float32(*t)
			entry.Temperature = &v
		}
		if t, ok := maxTokensMap[stage]; ok {
			entry.MaxTokens = t
		}
		if r, ok := maxRetriesMap[stage]; ok {
			entry.MaxRetries = r
		}
		entries = append(entries, entry)
	}

	return &pipelinev1.GetStageConfigResponse{Stages: entries}, nil
}

// ListModels returns the model registry: models known to the system, their
// inferred backend, and which pipeline stages they are assigned to.
//
// Data sources (merged in priority order):
//  1. model_config rows with config_key "stage.{stage}" — per-stage overrides
//  2. model_config row with config_key "default.llm"    — global default
//  3. Hardcoded known models (qwen2.5:7b, mxbai-embed-large, gemini models)
//     that may not be in the DB at all.
func (s *Service) ListModels(ctx context.Context, req *pipelinev1.ListModelsRequest) (*pipelinev1.ListModelsResponse, error) {
	s.logger.Debug("ListModels called")

	// knownStages lists the pipeline stages that use LLM models.
	knownStages := []string{"triage", "extract_ner", "extract_assertions", "analyze", "embed"}

	// knownModels is the set of models we always include even if they have no
	// DB config rows.  Maps model name → backend.
	knownModels := map[string]string{
		"qwen2.5:7b":        "ollama",
		"mxbai-embed-large": "ollama",
		"gemini-2.0-flash":  "gemini",
		"gemini-2.5-pro":    "gemini",
	}

	// modelStages accumulates the stages assigned to each model name.
	modelStages := make(map[string][]string)
	// globalDefault tracks which model is marked as the global default.
	globalDefault := ""

	if s.db != nil {
		// Read all model_config rows in one query to avoid N+1 per stage.
		rows, err := s.db.QueryContext(ctx, `
			SELECT config_key, model_name
			FROM model_config
			ORDER BY config_key
		`)
		if err != nil {
			// model_config table may not exist in older deployments — non-fatal.
			s.logger.Debug("Could not query model_config for ListModels", logging.Err(err))
		} else {
			defer rows.Close() //nolint:errcheck
			for rows.Next() {
				var configKey, modelName string
				if err := rows.Scan(&configKey, &modelName); err != nil {
					continue
				}
				if modelName == "" {
					continue
				}
				if configKey == "default.llm" {
					globalDefault = modelName
					continue
				}
				// stage.{stage} rows map directly to a stage name.
				if strings.HasPrefix(configKey, "stage.") {
					stageName := strings.TrimPrefix(configKey, "stage.")
					modelStages[modelName] = append(modelStages[modelName], stageName)
				}
			}
			if err := rows.Err(); err != nil {
				s.logger.Debug("Error iterating model_config rows", logging.Err(err))
			}
		}
	}

	// If no per-stage overrides exist, assign the fallback model to all LLM stages.
	// This mirrors the logic in GetStageConfig / selectBackend.
	if globalDefault == "" && len(modelStages) == 0 {
		// Pure-default deployment: qwen2.5:7b handles all LLM stages.
		for _, stage := range knownStages {
			if stage != "embed" {
				modelStages["qwen2.5:7b"] = append(modelStages["qwen2.5:7b"], stage)
			}
		}
	} else if globalDefault != "" && len(modelStages) == 0 {
		// Global default set but no per-stage overrides.
		for _, stage := range knownStages {
			if stage != "embed" {
				modelStages[globalDefault] = append(modelStages[globalDefault], stage)
			}
		}
	}

	// Embedding model (mxbai-embed-large) is always assigned to the embed stage.
	// If the DB has no entry for it, ensure it still appears.
	if _, ok := modelStages["mxbai-embed-large"]; !ok {
		modelStages["mxbai-embed-large"] = []string{"embed"}
	}

	// Build the unified model set: start with known models, add any DB models.
	allModels := make(map[string]string) // model name → backend
	for name, backend := range knownModels {
		allModels[name] = backend
	}
	// Add any models found in DB that are not in knownModels.
	for name := range modelStages {
		if _, ok := allModels[name]; !ok {
			allModels[name] = selectBackend(name)
		}
	}
	if globalDefault != "" {
		if _, ok := allModels[globalDefault]; !ok {
			allModels[globalDefault] = selectBackend(globalDefault)
		}
	}

	// Determine the effective global default model name for the is_default flag.
	effectiveDefault := globalDefault
	if effectiveDefault == "" {
		effectiveDefault = "qwen2.5:7b"
	}

	// Build the response list in a stable order.
	result := make([]*pipelinev1.ModelInfo, 0, len(allModels))
	for name, backend := range allModels {
		info := &pipelinev1.ModelInfo{
			Name:      name,
			Backend:   backend,
			Stages:    modelStages[name],
			IsDefault: name == effectiveDefault,
			Status:    "available",
		}
		result = append(result, info)
	}

	// Sort for deterministic output: default first, then alphabetical.
	sortModelInfos(result)

	s.logger.Info("ListModels completed", logging.F("model_count", len(result)))

	return &pipelinev1.ListModelsResponse{Models: result}, nil
}

// selectBackend mirrors the backend detection logic in services/ai/server/server.go.
// Returns "gemini" if the model name contains "gemini" (case-insensitive), else "ollama".
func selectBackend(model string) string {
	if strings.Contains(strings.ToLower(model), "gemini") {
		return "gemini"
	}
	return "ollama"
}

// sortModelInfos sorts models so the default comes first, then alphabetically by name.
func sortModelInfos(models []*pipelinev1.ModelInfo) {
	for i := 1; i < len(models); i++ {
		for j := 0; j < len(models)-i; j++ {
			a, b := models[j], models[j+1]
			// Default model always sorts first.
			if b.IsDefault && !a.IsDefault {
				models[j], models[j+1] = b, a
				continue
			}
			if a.IsDefault && !b.IsDefault {
				continue
			}
			// Otherwise alphabetical by name.
			if a.Name > b.Name {
				models[j], models[j+1] = b, a
			}
		}
	}
}

func stageDefaultTimeout(stage string) string {
	if stage == "analyze" {
		return "600s"
	}
	return "120s"
}

func stageDefaultHeartbeat(stage string) string {
	if stage == "analyze" {
		return "300s"
	}
	return "30s"
}

// countPendingSources counts sources with processing_status = 'pending'.
// If sourceTag is not empty, filters by that tag.
func (s *Service) countPendingSources(ctx context.Context, sourceTag string) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not available")
	}

	var count int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sources
		WHERE processing_status = 'pending'
		  AND is_deleted = false
		  AND ($1 = '' OR ingestion_metadata->>'source_tag' = $1)
	`, sourceTag).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("failed to count pending sources: %w", err)
	}

	return count, nil
}

// =============================================================================
// Classification Rules RPCs
// =============================================================================

// ListClassificationRules lists all classification rules in priority order.
func (s *Service) ListClassificationRules(ctx context.Context, req *pipelinev1.ListClassificationRulesRequest) (*pipelinev1.ListClassificationRulesResponse, error) {
	s.logger.Debug("ListClassificationRules called", logging.F("tenant_id", req.TenantId))

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	rules, err := s.queryClassificationRules(ctx, tenantID, "")
	if err != nil {
		s.logger.Error("Error listing classification rules", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list classification rules: %v", err)
	}

	return &pipelinev1.ListClassificationRulesResponse{Rules: rules}, nil
}

// GetClassificationRule retrieves a single classification rule by name.
func (s *Service) GetClassificationRule(ctx context.Context, req *pipelinev1.GetClassificationRuleRequest) (*pipelinev1.GetClassificationRuleResponse, error) {
	s.logger.Debug("GetClassificationRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("name", req.Name),
	)

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	rules, err := s.queryClassificationRules(ctx, tenantID, req.Name)
	if err != nil {
		s.logger.Error("Error getting classification rule", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get classification rule: %v", err)
	}

	if len(rules) == 0 {
		return nil, status.Errorf(codes.NotFound, "classification rule %q not found", req.Name)
	}

	return &pipelinev1.GetClassificationRuleResponse{Rule: rules[0]}, nil
}

// TestClassificationRule runs the classification engine against a content item.
func (s *Service) TestClassificationRule(ctx context.Context, req *pipelinev1.TestClassificationRuleRequest) (*pipelinev1.TestClassificationRuleResponse, error) {
	s.logger.Debug("TestClassificationRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("content_id", req.ContentId),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	// Load content item metadata for classification
	var metadataJSON sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT ingestion_metadata::text
		FROM sources
		WHERE content_id = $1 AND is_deleted = false
	`, req.ContentId).Scan(&metadataJSON)

	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "content item %q not found", req.ContentId)
	}
	if err != nil {
		s.logger.Error("Error loading content metadata", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to load content metadata: %v", err)
	}

	// Build metadata map for the classification engine
	metadata := make(map[string]string)
	if metadataJSON.Valid {
		var rawMeta map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSON.String), &rawMeta); err == nil {
			for k, v := range rawMeta {
				metadata[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	// Load classification rules
	rules, err := s.queryClassificationRulesRaw(ctx, tenantID)
	if err != nil {
		s.logger.Error("Error loading classification rules", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to load classification rules: %v", err)
	}

	// Run rules against metadata (same OR logic as the engine)
	resp := &pipelinev1.TestClassificationRuleResponse{
		RulesEvaluated: int32(len(rules)),
	}

	for _, rule := range rules {
		for _, cond := range rule.Conditions {
			fieldValue := metadata[cond.Field]
			if matchConditionTest(cond, fieldValue) {
				resp.MatchedRule = classRuleToProto(&rule)
				resp.ContentType = rule.ContentType
				resp.ContentSubtype = rule.ContentSubtype
				resp.NotificationSource = rule.NotificationSource
				resp.MatchedCondition = fmt.Sprintf("%s %s %q", cond.Field, cond.Operator, cond.Value)
				return resp, nil
			}
		}
	}

	// No match — return default
	resp.ContentType = "EMAIL"
	resp.ContentSubtype = "HUMAN"
	return resp, nil
}

// queryClassificationRules queries rules with optional name filter, returns proto messages.
func (s *Service) queryClassificationRules(ctx context.Context, tenantID, name string) ([]*pipelinev1.ClassificationRule, error) {
	query := `
		SELECT
			cr.name,
			cr.priority,
			COALESCE(cr.content_type_scope, '') AS scope,
			cr.content_type,
			cr.content_subtype,
			COALESCE(cr.notification_source, '') AS notification_source,
			cr.active,
			cmc.field,
			cmc.match_type,
			cmc.value
		FROM classification_rules cr
		LEFT JOIN classification_match_conditions cmc ON cmc.rule_id = cr.id
		WHERE cr.tenant_id = $1
	`
	args := []interface{}{tenantID}

	if name != "" {
		query += " AND cr.name = $2"
		args = append(args, name)
	}

	query += " ORDER BY cr.priority ASC, cr.id ASC, cmc.id ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying classification rules: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	// Group conditions by rule name
	ruleMap := make(map[string]*pipelinev1.ClassificationRule)
	var ruleOrder []string

	for rows.Next() {
		var (
			ruleName           string
			priority           int32
			scope              string
			contentType        string
			contentSubtype     string
			notificationSource string
			active             bool
			condField          sql.NullString
			condOperator       sql.NullString
			condValue          sql.NullString
		)

		if err := rows.Scan(
			&ruleName, &priority, &scope,
			&contentType, &contentSubtype, &notificationSource, &active,
			&condField, &condOperator, &condValue,
		); err != nil {
			return nil, fmt.Errorf("scanning classification rule: %w", err)
		}

		rule, exists := ruleMap[ruleName]
		if !exists {
			rule = &pipelinev1.ClassificationRule{
				Name:               ruleName,
				Priority:           priority,
				Scope:              scope,
				ContentType:        contentType,
				ContentSubtype:     contentSubtype,
				NotificationSource: notificationSource,
				Active:             active,
			}
			ruleMap[ruleName] = rule
			ruleOrder = append(ruleOrder, ruleName)
		}

		if condField.Valid {
			rule.Conditions = append(rule.Conditions, &pipelinev1.ClassificationMatchCondition{
				Field:    condField.String,
				Operator: condOperator.String,
				Value:    condValue.String,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating classification rules: %w", err)
	}

	// Preserve priority order
	result := make([]*pipelinev1.ClassificationRule, 0, len(ruleOrder))
	for _, n := range ruleOrder {
		result = append(result, ruleMap[n])
	}

	return result, nil
}

// classRuleRaw is an internal struct for TestClassificationRule engine evaluation.
type classRuleRaw struct {
	Name               string
	Priority           int32
	Scope              string
	ContentType        string
	ContentSubtype     string
	NotificationSource string
	Active             bool
	Conditions         []classCondRaw
}

type classCondRaw struct {
	Field    string
	Operator string
	Value    string
}

// queryClassificationRulesRaw loads rules for engine evaluation.
func (s *Service) queryClassificationRulesRaw(ctx context.Context, tenantID string) ([]classRuleRaw, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			cr.name, cr.priority,
			COALESCE(cr.content_type_scope, ''),
			cr.content_type, cr.content_subtype,
			COALESCE(cr.notification_source, ''),
			cr.active,
			cmc.field, cmc.match_type, cmc.value
		FROM classification_rules cr
		LEFT JOIN classification_match_conditions cmc ON cmc.rule_id = cr.id
		WHERE cr.tenant_id = $1 AND cr.active = true
		ORDER BY cr.priority ASC, cr.id ASC, cmc.id ASC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("querying classification rules: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var rules []classRuleRaw
	ruleIndex := map[string]int{}

	for rows.Next() {
		var (
			name, scope, ct, cst, ns string
			priority                  int32
			active                    bool
			condField, condOp, condV  sql.NullString
		)
		if err := rows.Scan(&name, &priority, &scope, &ct, &cst, &ns, &active, &condField, &condOp, &condV); err != nil {
			return nil, fmt.Errorf("scanning rule row: %w", err)
		}

		idx, exists := ruleIndex[name]
		if !exists {
			rules = append(rules, classRuleRaw{
				Name: name, Priority: priority, Scope: scope,
				ContentType: ct, ContentSubtype: cst,
				NotificationSource: ns, Active: active,
			})
			idx = len(rules) - 1
			ruleIndex[name] = idx
		}

		if condField.Valid {
			rules[idx].Conditions = append(rules[idx].Conditions, classCondRaw{
				Field: condField.String, Operator: condOp.String, Value: condV.String,
			})
		}
	}

	return rules, rows.Err()
}

// matchConditionTest evaluates a single match condition against a field value.
func matchConditionTest(cond classCondRaw, fieldValue string) bool {
	v := strings.ToLower(fieldValue)
	cv := strings.ToLower(cond.Value)

	switch cond.Operator {
	case "contains":
		return strings.Contains(v, cv)
	case "prefix":
		return strings.HasPrefix(v, cv)
	case "suffix":
		return strings.HasSuffix(v, cv)
	case "exact":
		return v == cv
	case "exists":
		return fieldValue != ""
	default:
		return false
	}
}

// classRuleToProto converts a raw classification rule to proto.
func classRuleToProto(r *classRuleRaw) *pipelinev1.ClassificationRule {
	rule := &pipelinev1.ClassificationRule{
		Name:               r.Name,
		Priority:           r.Priority,
		Scope:              r.Scope,
		ContentType:        r.ContentType,
		ContentSubtype:     r.ContentSubtype,
		NotificationSource: r.NotificationSource,
		Active:             r.Active,
	}
	for _, c := range r.Conditions {
		rule.Conditions = append(rule.Conditions, &pipelinev1.ClassificationMatchCondition{
			Field:    c.Field,
			Operator: c.Operator,
			Value:    c.Value,
		})
	}
	return rule
}

// defaultTenantID returns the first tenant ID from the database.
func (s *Service) defaultTenantID(ctx context.Context) string {
	if s.db == nil {
		return ""
	}
	var tenantID string
	_ = s.db.QueryRowContext(ctx, "SELECT id FROM tenants LIMIT 1").Scan(&tenantID)
	return tenantID
}

// CreateClassificationRule creates a new classification rule with conditions in a transaction.
func (s *Service) CreateClassificationRule(ctx context.Context, req *pipelinev1.CreateClassificationRuleRequest) (*pipelinev1.CreateClassificationRuleResponse, error) {
	s.logger.Debug("CreateClassificationRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("name", req.Name),
	)

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if len(req.Conditions) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one condition is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to begin transaction: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var ruleID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO classification_rules
			(tenant_id, name, priority, content_type_scope, content_type, content_subtype, notification_source, active)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, NULLIF($7, ''), $8)
		RETURNING id
	`, tenantID, req.Name, req.Priority,
		req.ContentTypeScope, req.ContentType, req.ContentSubtype,
		req.NotificationSource, req.Active).Scan(&ruleID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, status.Errorf(codes.AlreadyExists, "classification rule %q already exists for tenant", req.Name)
		}
		s.logger.Error("Error inserting classification rule", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create classification rule: %v", err)
	}

	for _, cond := range req.Conditions {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO classification_match_conditions (rule_id, field, match_type, value, case_sensitive)
			VALUES ($1, $2, $3, $4, false)
		`, ruleID, cond.Field, cond.Operator, cond.Value); err != nil {
			s.logger.Error("Error inserting classification condition", logging.Err(err))
			return nil, status.Errorf(codes.Internal, "failed to create condition: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	rules, err := s.queryClassificationRules(ctx, tenantID, req.Name)
	if err != nil {
		s.logger.Error("Error querying created rule", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "rule created but failed to fetch: %v", err)
	}
	if len(rules) == 0 {
		return nil, status.Error(codes.Internal, "rule created but not found on re-query")
	}

	return &pipelinev1.CreateClassificationRuleResponse{Rule: rules[0]}, nil
}

// DeleteClassificationRule deletes a classification rule by name (conditions cascade).
func (s *Service) DeleteClassificationRule(ctx context.Context, req *pipelinev1.DeleteClassificationRuleRequest) (*pipelinev1.DeleteClassificationRuleResponse, error) {
	s.logger.Debug("DeleteClassificationRule called",
		logging.F("tenant_id", req.TenantId),
		logging.F("name", req.Name),
	)

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	result, err := s.db.ExecContext(ctx,
		`DELETE FROM classification_rules WHERE tenant_id = $1 AND name = $2`,
		tenantID, req.Name,
	)
	if err != nil {
		s.logger.Error("Error deleting classification rule", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete classification rule: %v", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get rows affected: %v", err)
	}
	if n == 0 {
		return nil, status.Errorf(codes.NotFound, "classification rule %q not found", req.Name)
	}

	return &pipelinev1.DeleteClassificationRuleResponse{DeletedCount: int32(n)}, nil
}

// =============================================================================
// Pipeline Routing RPCs
// =============================================================================

// ListPipelineRoutes lists all pipeline routing rules.
func (s *Service) ListPipelineRoutes(ctx context.Context, req *pipelinev1.ListPipelineRoutesRequest) (*pipelinev1.ListPipelineRoutesResponse, error) {
	s.logger.Debug("ListPipelineRoutes called", logging.F("tenant_id", req.TenantId))

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(content_type, ''), COALESCE(content_subtype, ''), pipeline, active
		FROM pipeline_routing
		WHERE tenant_id = $1
		ORDER BY id ASC
	`, tenantID)
	if err != nil {
		s.logger.Error("Error listing pipeline routes", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list pipeline routes: %v", err)
	}
	defer rows.Close() //nolint:errcheck

	var routes []*pipelinev1.PipelineRoute
	for rows.Next() {
		var r pipelinev1.PipelineRoute
		if err := rows.Scan(&r.Id, &r.ContentType, &r.ContentSubtype, &r.Pipeline, &r.Active); err != nil {
			s.logger.Error("Error scanning pipeline route", logging.Err(err))
			continue
		}
		routes = append(routes, &r)
	}

	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to iterate pipeline routes: %v", err)
	}

	s.logger.Info("Pipeline routes listed", logging.F("count", len(routes)))
	return &pipelinev1.ListPipelineRoutesResponse{Routes: routes}, nil
}

// TestPipelineRoute shows which pipeline(s) a content item would enter.
func (s *Service) TestPipelineRoute(ctx context.Context, req *pipelinev1.TestPipelineRouteRequest) (*pipelinev1.TestPipelineRouteResponse, error) {
	s.logger.Debug("TestPipelineRoute called",
		logging.F("tenant_id", req.TenantId),
		logging.F("content_id", req.ContentId),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	tenantID := req.TenantId
	if tenantID == "" {
		tenantID = s.defaultTenantID(ctx)
	}

	// Load content item's classification from content_enrichment
	var contentType, contentSubtype string
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(ce.content_type::text, ''),
			COALESCE(ce.content_subtype, '')
		FROM content_enrichment ce
		JOIN sources s ON s.id = ce.source_id
		WHERE s.content_id = $1 AND s.is_deleted = false
	`, req.ContentId).Scan(&contentType, &contentSubtype)

	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "content item %q not found", req.ContentId)
	}
	if err != nil {
		s.logger.Error("Error loading content classification", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to load content classification: %v", err)
	}

	// Look up matching routes
	routeRows, err := s.db.QueryContext(ctx, `
		SELECT pipeline
		FROM pipeline_routing
		WHERE tenant_id = $1
		  AND active = true
		  AND (content_type IS NULL OR LOWER(content_type) = LOWER($2))
		  AND (content_subtype IS NULL OR LOWER(content_subtype) = LOWER($3))
		ORDER BY id ASC
	`, tenantID, contentType, contentSubtype)
	if err != nil {
		s.logger.Error("Error querying pipeline routes", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to query pipeline routes: %v", err)
	}
	defer routeRows.Close() //nolint:errcheck

	var matchedRoutes []*pipelinev1.MatchedRoute
	for routeRows.Next() {
		var pipelineName string
		if err := routeRows.Scan(&pipelineName); err != nil {
			continue
		}

		mr := &pipelinev1.MatchedRoute{Pipeline: pipelineName}
		if def, ok := routing.Pipelines[pipelineName]; ok {
			mr.Stages = def.Stages
		}
		matchedRoutes = append(matchedRoutes, mr)
	}

	if err := routeRows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to iterate pipeline routes: %v", err)
	}

	s.logger.Info("Pipeline route test completed",
		logging.F("content_id", req.ContentId),
		logging.F("content_type", contentType),
		logging.F("content_subtype", contentSubtype),
		logging.F("matched_routes", len(matchedRoutes)),
	)

	return &pipelinev1.TestPipelineRouteResponse{
		ContentType:    contentType,
		ContentSubtype: contentSubtype,
		MatchedRoutes:  matchedRoutes,
	}, nil
}
