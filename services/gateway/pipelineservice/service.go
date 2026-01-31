// Package pipelineservice implements the PipelineService gRPC server.
package pipelineservice

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// Service implements the PipelineService gRPC server.
type Service struct {
	pipelinev1.UnimplementedPipelineServiceServer
	repo   *pipeline.Repository
	logger logging.Logger
}

// NewService creates a new pipeline service.
func NewService(repo *pipeline.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
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

	// Call repository to queue pending items
	count, err := s.repo.KickPendingProcessing(ctx, limit, req.SourceTag)
	if err != nil {
		s.logger.Error("Error kicking pending processing", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to kick processing: %v", err)
	}

	message := fmt.Sprintf("Queued %d pending items for processing", count)
	if req.SourceTag != "" {
		message = fmt.Sprintf("Queued %d pending items for processing (source_tag: %s)", count, req.SourceTag)
	}

	s.logger.Info("Pending items queued",
		logging.F("queued_count", count),
		logging.F("source_tag", req.SourceTag),
	)

	return &pipelinev1.KickProcessingResponse{
		QueuedCount: int64(count),
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
		SourcesTotal:      s.SourcesTotal,
		SourcesByStatus:   statusCountsToProto(s.SourcesByStatus),
		EmbeddingsTotal:   s.EmbeddingsTotal,
		EmbeddingsRecent:  s.EmbeddingsRecent,
		AttachmentsTotal:  s.AttachmentsTotal,
		AttachmentsByTier: statusCountsToProto(s.AttachmentsByTier),
		JobsTotal:         s.JobsTotal,
		JobsByStatus:      statusCountsToProto(s.JobsByStatus),
		RecentJobs:        recentJobs,
		Timestamp:         timestamppb.New(s.Timestamp),
	}
}
