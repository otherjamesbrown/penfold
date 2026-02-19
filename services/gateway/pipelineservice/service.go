// Package pipelineservice implements the PipelineService gRPC server.
package pipelineservice

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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

	// Check if Temporal client is available
	if s.temporalClient == nil {
		return nil, status.Error(codes.Unavailable, "Temporal client not configured")
	}

	// Read max_concurrent from pipeline_config
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

	// Resolve per-stage timeouts once for all workflows in this batch
	stageTimeouts, stageHeartbeats := s.resolveStageTimeouts(ctx)

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
			StageTimeouts:   stageTimeouts,
			StageHeartbeats: stageHeartbeats,
		}
		opts := client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: "penfold-main",
		}
		_, err := s.temporalClient.ExecuteWorkflow(ctx, opts, "SLMPipelineWorkflow", input)
		if err != nil {
			s.logger.Warn("Failed to start workflow for source",
				logging.F("source_id", src.ID),
				logging.F("workflow_id", workflowID),
				logging.Err(err),
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

	// Build query with optional key prefix filter
	query := `
		SELECT key, value, value_type, description,
		       min_value, max_value, default_value,
		       COALESCE(updated_at, now()), COALESCE(updated_by, '')
		FROM pipeline_config
		WHERE value_type = 'duration'
	`
	args := []interface{}{}

	if req.Key != "" {
		query += " AND key LIKE $1"
		args = append(args, req.Key+"%")
	}

	query += " ORDER BY key"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		s.logger.Error("Error querying pipeline_config", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to query timeout config: %v", err)
	}
	defer rows.Close()

	var entries []*pipelinev1.TimeoutEntry
	for rows.Next() {
		var entry pipelinev1.TimeoutEntry
		var updatedAt time.Time

		err := rows.Scan(
			&entry.Key,
			&entry.Value,
			&entry.ValueType,
			&entry.Description,
			&entry.MinValue,
			&entry.MaxValue,
			&entry.DefaultValue,
			&updatedAt,
			&entry.UpdatedBy,
		)
		if err != nil {
			s.logger.Error("Error scanning timeout entry", logging.Err(err))
			continue
		}

		entry.UpdatedAt = updatedAt.Format(time.RFC3339)
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

	// Get current entry to check value_type and min/max bounds
	var entry pipelinev1.TimeoutEntry
	var previousValue string
	var updatedAt time.Time

	err := s.db.QueryRowContext(ctx, `
		SELECT key, value, value_type, description,
		       min_value, max_value, default_value,
		       COALESCE(updated_at, now()), COALESCE(updated_by, '')
		FROM pipeline_config
		WHERE key = $1
	`, req.Key).Scan(
		&entry.Key,
		&previousValue,
		&entry.ValueType,
		&entry.Description,
		&entry.MinValue,
		&entry.MaxValue,
		&entry.DefaultValue,
		&updatedAt,
		&entry.UpdatedBy,
	)

	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "unknown config key: %s", req.Key)
	}
	if err != nil {
		s.logger.Error("Error getting config entry", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get config entry: %v", err)
	}

	// Type-specific validation
	if entry.ValueType == "duration" {
		// Parse and validate duration values
		newDuration, err := time.ParseDuration(req.Value)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid duration value: %v", err)
		}

		// Validate min/max bounds for duration
		if entry.MinValue != "" {
			minDuration, err := time.ParseDuration(entry.MinValue)
			if err == nil && newDuration < minDuration {
				return nil, status.Errorf(codes.InvalidArgument, "value %s is below minimum %s", req.Value, entry.MinValue)
			}
		}
		if entry.MaxValue != "" {
			maxDuration, err := time.ParseDuration(entry.MaxValue)
			if err == nil && newDuration > maxDuration {
				return nil, status.Errorf(codes.InvalidArgument, "value %s is above maximum %s", req.Value, entry.MaxValue)
			}
		}
	}
	// For string type (model names), no parsing or min/max validation needed
	// For integer, float, boolean types: validation could be added here in the future

	// Update the value in database
	_, err = s.db.ExecContext(ctx, `
		UPDATE pipeline_config
		SET value = $1, updated_at = now(), updated_by = $2
		WHERE key = $3
	`, req.Value, req.UpdatedBy, req.Key)
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

	// Read max_concurrent from pipeline_config
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

	// Update pipeline_config using the helper
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
	defer rows.Close()

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

// getMaxConcurrent reads the max_concurrent value from pipeline_config.
func (s *Service) getMaxConcurrent(ctx context.Context) (int, error) {
	if s.db == nil {
		return 1, fmt.Errorf("database not available")
	}

	var valueStr string
	err := s.db.QueryRowContext(ctx, `
		SELECT value
		FROM pipeline_config
		WHERE key = 'pipeline.max_concurrent' AND value_type = 'integer'
	`).Scan(&valueStr)

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

// setMaxConcurrent updates the max_concurrent value in pipeline_config.
func (s *Service) setMaxConcurrent(ctx context.Context, value int, updatedBy string) error {
	if s.db == nil {
		return fmt.Errorf("database not available")
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE pipeline_config
		SET value = $1, updated_at = NOW(), updated_by = $2
		WHERE key = 'pipeline.max_concurrent' AND value_type = 'integer'
	`, strconv.Itoa(value), updatedBy)

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
	s.logger.Debug("GetStageConfig called", logging.F("stage", req.Stage))

	if s.db == nil {
		return nil, status.Error(codes.Unavailable, "database not available")
	}

	// Known pipeline stages
	stages := []string{"triage", "extract_entities", "extract_assertions", "deep_analyze", "embedding"}
	if req.Stage != "" {
		stages = []string{req.Stage}
	}

	// Build a map of all timeout.stage.* and model_config entries
	timeoutMap := make(map[string]string)   // key -> value
	timeoutSourceMap := make(map[string]string) // stage -> source

	rows, err := s.db.QueryContext(ctx, `
		SELECT key, value, default_value
		FROM pipeline_config
		WHERE key LIKE 'timeout.stage.%'
		ORDER BY key
	`)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query stage timeouts: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key, value, defaultValue string
		if err := rows.Scan(&key, &value, &defaultValue); err != nil {
			continue
		}
		timeoutMap[key] = value
		// Determine source: if value differs from default, it was explicitly set
		if value != defaultValue {
			// Extract stage name from key (timeout.stage.<stage>.<type>)
			parts := splitStageKey(key)
			if parts != "" {
				timeoutSourceMap[parts] = "db"
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to iterate stage timeouts: %v", err)
	}

	// Query model_config for per-stage models
	modelMap := make(map[string]string)       // stage -> model
	modelSourceMap := make(map[string]string)  // stage -> source
	modelRows, err := s.db.QueryContext(ctx, `
		SELECT stage, model_id, config_source
		FROM model_config
		WHERE stage != ''
		ORDER BY priority DESC
	`)
	if err != nil {
		// model_config table may not exist yet — non-fatal
		s.logger.Debug("Could not query model_config", logging.Err(err))
	} else {
		defer modelRows.Close()
		for modelRows.Next() {
			var stage, modelID, source string
			if err := modelRows.Scan(&stage, &modelID, &source); err != nil {
				continue
			}
			// Higher priority rows come first, keep only the first (highest priority)
			if _, exists := modelMap[stage]; !exists {
				modelMap[stage] = modelID
				modelSourceMap[stage] = source
			}
		}
	}

	// Build response
	var entries []*pipelinev1.StageConfigEntry
	for _, stage := range stages {
		stcKey := fmt.Sprintf("timeout.stage.%s.start_to_close", stage)
		hbKey := fmt.Sprintf("timeout.stage.%s.heartbeat", stage)

		timeout := timeoutMap[stcKey]
		heartbeat := timeoutMap[hbKey]

		// Use defaults if not in DB
		if timeout == "" {
			timeout = stageDefaultTimeout(stage)
		}
		if heartbeat == "" {
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

		entries = append(entries, &pipelinev1.StageConfigEntry{
			Stage:         stage,
			Model:         modelMap[stage],
			Timeout:       timeout,
			Heartbeat:     heartbeat,
			ModelSource:   modelSrc,
			TimeoutSource: timeoutSrc,
		})
	}

	return &pipelinev1.GetStageConfigResponse{Stages: entries}, nil
}

// splitStageKey extracts the stage name from a timeout.stage.<stage>.<type> key.
func splitStageKey(key string) string {
	// key format: timeout.stage.triage.start_to_close
	const prefix = "timeout.stage."
	if len(key) <= len(prefix) {
		return ""
	}
	rest := key[len(prefix):]
	// Find last dot to split stage from type
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] == '.' {
			return rest[:i]
		}
	}
	return ""
}

func stageDefaultTimeout(stage string) string {
	if stage == "deep_analyze" {
		return "600s"
	}
	return "120s"
}

func stageDefaultHeartbeat(stage string) string {
	if stage == "deep_analyze" {
		return "300s"
	}
	return "30s"
}

// resolveStageTimeouts reads per-stage timeout values from pipeline_config.
// Returns maps of stage->duration for start_to_close and heartbeat.
// Returns nil maps (not error) if DB is unavailable — workflow falls back to defaults.
func (s *Service) resolveStageTimeouts(ctx context.Context) (map[string]time.Duration, map[string]time.Duration) {
	if s.db == nil {
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT key, value, default_value
		FROM pipeline_config
		WHERE key LIKE 'timeout.stage.%'
	`)
	if err != nil {
		s.logger.Debug("Could not resolve stage timeouts", logging.Err(err))
		return nil, nil
	}
	defer rows.Close()

	timeouts := make(map[string]time.Duration)
	heartbeats := make(map[string]time.Duration)
	hasOverride := false

	for rows.Next() {
		var key, value, defaultValue string
		if err := rows.Scan(&key, &value, &defaultValue); err != nil {
			continue
		}
		// Parse the configured value regardless of whether it matches the default.
		// Even if value == default, we still want to pass it through so the worker
		// has explicit config rather than falling back to hardcoded defaults.
		dur, err := time.ParseDuration(value)
		if err != nil {
			continue
		}
		stage := splitStageKey(key)
		if stage == "" {
			continue
		}
		hasOverride = true
		if strings.HasSuffix(key, ".start_to_close") {
			timeouts[stage] = dur
		} else if strings.HasSuffix(key, ".heartbeat") {
			heartbeats[stage] = dur
		}
	}

	if !hasOverride {
		return nil, nil
	}
	return timeouts, heartbeats
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
