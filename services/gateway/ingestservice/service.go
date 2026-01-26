// Package ingestservice implements the IngestService gRPC server.
package ingestservice

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	ingestv1 "github.com/otherjamesbrown/penfold/api/proto/ingest/v1"
	"github.com/otherjamesbrown/penfold/pkg/contentid"
	"github.com/otherjamesbrown/penfold/pkg/ingest/storage"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Repository defines the interface for ingest storage operations.
// This interface allows for easy mocking in tests.
type Repository interface {
	CheckDuplicate(ctx context.Context, tenantID, messageID, contentHash string) (bool, int64, string, error)
	CreateSource(ctx context.Context, source *storage.EmailSource) (*storage.CreatedSource, error)
	CreateJob(ctx context.Context, job *storage.IngestJob) error
	GetJob(ctx context.Context, jobID string) (*storage.IngestJob, error)
	UpdateJobProgress(ctx context.Context, jobID string, processed, imported, skipped, failed int, processedFiles []string) error
	CompleteJob(ctx context.Context, jobID string, status storage.IngestJobStatus) error
	RecordError(ctx context.Context, jobID, filePath string, errorType storage.IngestErrorType, errorMsg string, details map[string]interface{}) error
	GetRemainingFilesForJob(ctx context.Context, jobID string, allFiles []string) ([]string, error)
}

// Tenant represents a tenant entity with its ID.
type Tenant struct {
	ID string
}

// TenantRepository defines the interface for tenant lookup operations.
// This allows resolving tenant slugs to UUIDs.
type TenantRepository interface {
	GetByRef(ctx context.Context, ref string) (*Tenant, error)
}

// tenantRepoAdapter adapts the tenant package's Repository to TenantRepository.
type tenantRepoAdapter struct {
	getByRef func(ctx context.Context, ref string) (string, error)
}

func (a *tenantRepoAdapter) GetByRef(ctx context.Context, ref string) (*Tenant, error) {
	id, err := a.getByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}
	return &Tenant{ID: id}, nil
}

// NewTenantRepoAdapter creates a TenantRepository adapter.
// The getByRefFn should return the tenant UUID for a given reference (UUID or slug),
// or empty string if not found.
func NewTenantRepoAdapter(getByRefFn func(ctx context.Context, ref string) (string, error)) TenantRepository {
	return &tenantRepoAdapter{getByRef: getByRefFn}
}

// Service implements the IngestService gRPC server.
type Service struct {
	ingestv1.UnimplementedIngestServiceServer
	repo       Repository
	tenantRepo TenantRepository
	logger     logging.Logger
}

// NewService creates a new ingest service.
func NewService(repo Repository, tenantRepo TenantRepository, logger logging.Logger) *Service {
	return &Service{
		repo:       repo,
		tenantRepo: tenantRepo,
		logger:     logger.With(logging.F("component", "ingest_service")),
	}
}

// resolveTenantID resolves a tenant reference (UUID or slug) to a UUID.
// This allows users to specify tenant by name/slug in their config.
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

// IngestEmail ingests a single email, checking for duplicates first.
func (s *Service) IngestEmail(ctx context.Context, req *ingestv1.IngestEmailRequest) (*ingestv1.IngestEmailResponse, error) {
	s.logger.Debug("IngestEmail called",
		logging.F("tenant_id", req.TenantId),
		logging.F("message_id", req.MessageId),
		logging.F("content_id", req.ContentId),
	)

	// Validate required fields
	if req.MessageId == "" {
		return nil, status.Error(codes.InvalidArgument, "message_id is required")
	}
	if req.ContentHash == "" {
		return nil, status.Error(codes.InvalidArgument, "content_hash is required")
	}

	// Validate content_id if provided (empty is OK for backwards compat)
	if req.ContentId != "" && !contentid.IsValid(req.ContentId) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid content_id format: %s", req.ContentId)
	}

	// Check for duplicates
	isDuplicate, existingID, duplicateReason, err := s.repo.CheckDuplicate(ctx, req.TenantId, req.MessageId, req.ContentHash)
	if err != nil {
		s.logger.Error("Error checking duplicate",
			logging.Err(err),
			logging.F("tenant_id", req.TenantId),
			logging.F("message_id", req.MessageId),
		)
		return nil, status.Errorf(codes.Internal, "failed to check duplicate: %v", err)
	}

	if isDuplicate {
		s.logger.Debug("Duplicate email detected",
			logging.F("tenant_id", req.TenantId),
			logging.F("message_id", req.MessageId),
			logging.F("existing_id", existingID),
			logging.F("reason", duplicateReason),
		)

		return &ingestv1.IngestEmailResponse{
			WasDuplicate:     true,
			ExistingSourceId: fmt.Sprintf("%d", existingID),
			DuplicateReason:  duplicateReasonToProto(duplicateReason),
			Status:           ingestv1.ProcessingStatus_PROCESSING_STATUS_SKIPPED,
		}, nil
	}

	// Parse source timestamp - prefer sent_at, then received_at, then now
	var sourceTimestamp time.Time
	if req.SentAt != nil {
		sourceTimestamp = req.SentAt.AsTime()
	} else if req.ReceivedAt != nil {
		sourceTimestamp = req.ReceivedAt.AsTime()
	} else {
		sourceTimestamp = time.Now()
	}

	// Build raw content from plain text body (or HTML if no plain text)
	rawContent := req.BodyPlain
	if rawContent == "" {
		rawContent = req.BodyHtml
	}

	// Build metadata from request
	metadata := buildEmailMetadata(req)

	// Collect participant emails
	participantEmails := collectParticipantEmails(req)

	// Create the source record
	emailSource := &storage.EmailSource{
		TenantID:          req.TenantId,
		SourceSystem:      req.SourceSystem,
		ExternalID:        req.MessageId,
		ContentHash:       req.ContentHash,
		RawContent:        rawContent,
		ContentType:       "message/rfc822",
		ContentSize:       int32(len(rawContent)),
		Metadata:          metadata,
		SourceTimestamp:   sourceTimestamp,
		ParticipantEmails: participantEmails,
		ContentID:         req.ContentId,
	}

	// Set default source system if not specified
	if emailSource.SourceSystem == "" {
		emailSource.SourceSystem = storage.SourceSystemManualEML
	}

	createdSource, err := s.repo.CreateSource(ctx, emailSource)
	if err != nil {
		s.logger.Error("Error creating source",
			logging.Err(err),
			logging.F("tenant_id", req.TenantId),
			logging.F("message_id", req.MessageId),
		)
		return nil, status.Errorf(codes.Internal, "failed to create source: %v", err)
	}

	s.logger.Info("Email ingested successfully",
		logging.F("tenant_id", req.TenantId),
		logging.F("source_id", createdSource.ID),
		logging.F("message_id", req.MessageId),
		logging.F("content_id", createdSource.ContentID),
	)

	return &ingestv1.IngestEmailResponse{
		SourceId:     fmt.Sprintf("%d", createdSource.ID),
		WasDuplicate: false,
		Status:       ingestv1.ProcessingStatus_PROCESSING_STATUS_PENDING,
		ContentId:    createdSource.ContentID,
	}, nil
}

// IngestAttachment ingests an attachment with a parent source link.
func (s *Service) IngestAttachment(ctx context.Context, req *ingestv1.IngestAttachmentRequest) (*ingestv1.IngestAttachmentResponse, error) {
	s.logger.Debug("IngestAttachment called",
		logging.F("tenant_id", req.TenantId),
		logging.F("parent_source_id", req.ParentSourceId),
		logging.F("content_id", req.ContentId),
	)

	// Validate required fields
	if req.ParentSourceId == "" {
		return nil, status.Error(codes.InvalidArgument, "parent_source_id is required")
	}
	if req.Metadata == nil || req.Metadata.Filename == "" {
		return nil, status.Error(codes.InvalidArgument, "attachment metadata with filename is required")
	}

	// Validate content_id if provided (empty is OK for backwards compat)
	if req.ContentId != "" && !contentid.IsValid(req.ContentId) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid content_id format: %s", req.ContentId)
	}

	// Check for duplicate by content hash
	if req.Metadata.ContentHash != "" {
		isDup, existingID, _, err := s.repo.CheckDuplicate(ctx, req.TenantId, "", req.Metadata.ContentHash)
		if err != nil {
			s.logger.Error("Error checking attachment duplicate",
				logging.Err(err),
				logging.F("parent_source_id", req.ParentSourceId),
			)
			return nil, status.Errorf(codes.Internal, "failed to check attachment duplicate: %v", err)
		}

		if isDup {
			return &ingestv1.IngestAttachmentResponse{
				AttachmentId: fmt.Sprintf("%d", existingID),
				WasDuplicate: true,
				Status:       ingestv1.ProcessingStatus_PROCESSING_STATUS_SKIPPED,
			}, nil
		}
	}

	// Build attachment metadata
	metadata := map[string]interface{}{
		"parent_source_id": req.ParentSourceId,
		"filename":         req.Metadata.Filename,
		"mime_type":        req.Metadata.MimeType,
		"size_bytes":       req.Metadata.SizeBytes,
		"job_id":           req.JobId,
	}

	// Create the attachment source record
	attachmentSource := &storage.EmailSource{
		TenantID:        req.TenantId,
		SourceSystem:    "attachment",
		ExternalID:      fmt.Sprintf("%s:%s", req.ParentSourceId, req.Metadata.Filename),
		ContentHash:     req.Metadata.ContentHash,
		RawContent:      string(req.Content),
		ContentType:     req.Metadata.MimeType,
		ContentSize:     int32(req.Metadata.SizeBytes),
		Metadata:        metadata,
		SourceTimestamp: time.Now(),
		ContentID:       req.ContentId,
	}

	createdSource, err := s.repo.CreateSource(ctx, attachmentSource)
	if err != nil {
		s.logger.Error("Error creating attachment source",
			logging.Err(err),
			logging.F("parent_source_id", req.ParentSourceId),
		)
		return nil, status.Errorf(codes.Internal, "failed to create attachment source: %v", err)
	}

	s.logger.Info("Attachment ingested successfully",
		logging.F("attachment_id", createdSource.ID),
		logging.F("parent_source_id", req.ParentSourceId),
		logging.F("filename", req.Metadata.Filename),
		logging.F("content_id", createdSource.ContentID),
	)

	return &ingestv1.IngestAttachmentResponse{
		AttachmentId: fmt.Sprintf("%d", createdSource.ID),
		StoragePath:  req.Metadata.StoragePath,
		WasDuplicate: false,
		Status:       ingestv1.ProcessingStatus_PROCESSING_STATUS_PENDING,
		ContentId:    createdSource.ContentID,
	}, nil
}

// IngestMeeting ingests a meeting with its associated transcript and chat.
func (s *Service) IngestMeeting(ctx context.Context, req *ingestv1.IngestMeetingRequest) (*ingestv1.IngestMeetingResponse, error) {
	s.logger.Debug("IngestMeeting called",
		logging.F("tenant_id", req.TenantId),
		logging.F("meeting_id", req.ExternalMeetingId),
		logging.F("content_id", req.ContentId),
	)

	// Validate required fields
	if req.ExternalMeetingId == "" {
		return nil, status.Error(codes.InvalidArgument, "external_meeting_id is required")
	}

	// Validate content_id if provided (empty is OK for backwards compat)
	if req.ContentId != "" && !contentid.IsValid(req.ContentId) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid content_id format: %s", req.ContentId)
	}

	// Parse meeting timestamp
	var meetingTimestamp time.Time
	if req.ActualStart != nil {
		meetingTimestamp = req.ActualStart.AsTime()
	} else if req.ScheduledStart != nil {
		meetingTimestamp = req.ScheduledStart.AsTime()
	} else {
		meetingTimestamp = time.Now()
	}

	// Collect participant emails
	participantEmails := make([]string, 0, len(req.Participants))
	for _, p := range req.Participants {
		if p.Email != "" {
			participantEmails = append(participantEmails, p.Email)
		}
	}

	// Build transcript content from segments
	transcriptContent := buildTranscriptContent(req.Transcript)
	transcriptHash := computeSimpleHash(transcriptContent)

	// Build meeting metadata
	metadata := buildMeetingMetadata(req)

	// Check for duplicate meeting
	externalID := fmt.Sprintf("meeting:%s", req.ExternalMeetingId)
	isDup, existingID, reason, err := s.repo.CheckDuplicate(ctx, req.TenantId, externalID, transcriptHash)
	if err != nil {
		s.logger.Error("Error checking meeting duplicate",
			logging.Err(err),
			logging.F("meeting_id", req.ExternalMeetingId),
		)
		return nil, status.Errorf(codes.Internal, "failed to check meeting duplicate: %v", err)
	}

	if isDup {
		s.logger.Debug("Duplicate meeting detected",
			logging.F("meeting_id", req.ExternalMeetingId),
			logging.F("existing_id", existingID),
			logging.F("reason", reason),
		)

		return &ingestv1.IngestMeetingResponse{
			WasDuplicate:     true,
			ExistingSourceId: fmt.Sprintf("%d", existingID),
			Status:           ingestv1.ProcessingStatus_PROCESSING_STATUS_SKIPPED,
		}, nil
	}

	// Create meeting source
	meetingSource := &storage.EmailSource{
		TenantID:          req.TenantId,
		SourceSystem:      sourceSystemForPlatform(req.Platform),
		ExternalID:        externalID,
		ContentHash:       transcriptHash,
		RawContent:        transcriptContent,
		ContentType:       "text/vtt",
		ContentSize:       int32(len(transcriptContent)),
		Metadata:          metadata,
		SourceTimestamp:   meetingTimestamp,
		ParticipantEmails: participantEmails,
		ContentID:         req.ContentId,
	}

	createdSource, err := s.repo.CreateSource(ctx, meetingSource)
	if err != nil {
		s.logger.Error("Error creating meeting source",
			logging.Err(err),
			logging.F("meeting_id", req.ExternalMeetingId),
		)
		return nil, status.Errorf(codes.Internal, "failed to create meeting source: %v", err)
	}

	s.logger.Info("Meeting ingested successfully",
		logging.F("source_id", createdSource.ID),
		logging.F("meeting_id", req.ExternalMeetingId),
		logging.F("transcript_segments", len(req.Transcript)),
		logging.F("chat_messages", len(req.ChatMessages)),
		logging.F("content_id", createdSource.ContentID),
	)

	return &ingestv1.IngestMeetingResponse{
		SourceId:                 fmt.Sprintf("%d", createdSource.ID),
		WasDuplicate:            false,
		Status:                  ingestv1.ProcessingStatus_PROCESSING_STATUS_PENDING,
		TranscriptSegmentsCount: int32(len(req.Transcript)),
		ChatMessagesCount:       int32(len(req.ChatMessages)),
		ContentId:               createdSource.ContentID,
	}, nil
}

// CreateIngestJob creates a new batch ingest job.
func (s *Service) CreateIngestJob(ctx context.Context, req *ingestv1.CreateIngestJobRequest) (*ingestv1.CreateIngestJobResponse, error) {
	s.logger.Debug("CreateIngestJob called",
		logging.F("tenant_id", req.TenantId),
		logging.F("name", req.Name),
		logging.F("total_files", req.TotalFiles),
	)

	// Resolve tenant reference to UUID
	tenantID, err := s.resolveTenantID(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}

	// Generate job ID
	jobID := uuid.New().String()

	// Convert metadata from proto map to interface map
	options := make(map[string]interface{})
	for k, v := range req.Metadata {
		options[k] = v
	}

	// Map platform to source tag
	sourceTag := platformToSourceTag(req.Platform)

	job := &storage.IngestJob{
		ID:             jobID,
		TenantID:       tenantID,
		Status:         storage.IngestJobStatusPending,
		SourceTag:      sourceTag,
		ContentType:    platformToContentType(req.Platform),
		TotalFiles:     int(req.TotalFiles),
		ProcessedCount: 0,
		ImportedCount:  0,
		SkippedCount:   0,
		FailedCount:    0,
		FileManifest:   []string{req.SourcePath},
		ProcessedFiles: []string{},
		Options:        options,
	}

	if err := s.repo.CreateJob(ctx, job); err != nil {
		s.logger.Error("Error creating job",
			logging.Err(err),
			logging.F("job_id", jobID),
		)
		return nil, status.Errorf(codes.Internal, "failed to create job: %v", err)
	}

	s.logger.Info("Ingest job created",
		logging.F("job_id", jobID),
		logging.F("tenant_id", req.TenantId),
		logging.F("total_files", req.TotalFiles),
	)

	return &ingestv1.CreateIngestJobResponse{
		Job: jobToProto(job),
	}, nil
}

// UpdateJobProgress updates the progress of an existing ingest job.
func (s *Service) UpdateJobProgress(ctx context.Context, req *ingestv1.UpdateJobProgressRequest) (*ingestv1.UpdateJobProgressResponse, error) {
	s.logger.Debug("UpdateJobProgress called",
		logging.F("job_id", req.JobId),
		logging.F("processed_delta", req.ProcessedDelta),
		logging.F("failed_delta", req.FailedDelta),
	)

	// Validate required fields
	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	// Get current job state
	job, err := s.repo.GetJob(ctx, req.JobId)
	if err != nil {
		s.logger.Error("Error fetching job for progress update",
			logging.Err(err),
			logging.F("job_id", req.JobId),
		)
		return nil, status.Errorf(codes.Internal, "failed to fetch job: %v", err)
	}

	if job == nil {
		return nil, status.Errorf(codes.NotFound, "job not found: %s", req.JobId)
	}

	// Calculate new counts by applying deltas
	newProcessed := job.ProcessedCount + int(req.ProcessedDelta)
	newImported := job.ImportedCount + int(req.ProcessedDelta) - int(req.FailedDelta) - int(req.SkippedDelta)
	if newImported < 0 {
		newImported = 0
	}
	newSkipped := job.SkippedCount + int(req.SkippedDelta)
	newFailed := job.FailedCount + int(req.FailedDelta)

	err = s.repo.UpdateJobProgress(
		ctx,
		req.JobId,
		newProcessed,
		newImported,
		newSkipped,
		newFailed,
		job.ProcessedFiles, // Keep existing processed files
	)

	if err != nil {
		s.logger.Error("Error updating job progress",
			logging.Err(err),
			logging.F("job_id", req.JobId),
		)
		return nil, status.Errorf(codes.Internal, "failed to update job progress: %v", err)
	}

	// Fetch updated job
	updatedJob, err := s.repo.GetJob(ctx, req.JobId)
	if err != nil {
		s.logger.Error("Error fetching updated job",
			logging.Err(err),
			logging.F("job_id", req.JobId),
		)
		return nil, status.Errorf(codes.Internal, "failed to fetch updated job: %v", err)
	}

	return &ingestv1.UpdateJobProgressResponse{
		Job: jobToProto(updatedJob),
	}, nil
}

// CompleteIngestJob marks an ingest job as completed.
func (s *Service) CompleteIngestJob(ctx context.Context, req *ingestv1.CompleteIngestJobRequest) (*ingestv1.CompleteIngestJobResponse, error) {
	s.logger.Debug("CompleteIngestJob called",
		logging.F("job_id", req.JobId),
		logging.F("success", req.Success),
	)

	// Validate required fields
	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	// Determine final status based on success flag and error message
	var jobStatus storage.IngestJobStatus
	if req.Success {
		// Check if there were any errors
		job, err := s.repo.GetJob(ctx, req.JobId)
		if err == nil && job != nil && job.FailedCount > 0 {
			jobStatus = storage.IngestJobStatusCompletedErrors
		} else {
			jobStatus = storage.IngestJobStatusCompleted
		}
	} else {
		jobStatus = storage.IngestJobStatusFailed
	}

	if err := s.repo.CompleteJob(ctx, req.JobId, jobStatus); err != nil {
		s.logger.Error("Error completing job",
			logging.Err(err),
			logging.F("job_id", req.JobId),
		)
		return nil, status.Errorf(codes.Internal, "failed to complete job: %v", err)
	}

	// Fetch updated job
	job, err := s.repo.GetJob(ctx, req.JobId)
	if err != nil {
		s.logger.Error("Error fetching completed job",
			logging.Err(err),
			logging.F("job_id", req.JobId),
		)
		return nil, status.Errorf(codes.Internal, "failed to fetch completed job: %v", err)
	}

	s.logger.Info("Ingest job completed",
		logging.F("job_id", req.JobId),
		logging.F("status", string(jobStatus)),
		logging.F("imported", job.ImportedCount),
		logging.F("skipped", job.SkippedCount),
		logging.F("failed", job.FailedCount),
	)

	return &ingestv1.CompleteIngestJobResponse{
		Job: jobToProto(job),
	}, nil
}

// GetIngestJob retrieves an ingest job by ID.
func (s *Service) GetIngestJob(ctx context.Context, req *ingestv1.GetIngestJobRequest) (*ingestv1.GetIngestJobResponse, error) {
	s.logger.Debug("GetIngestJob called",
		logging.F("job_id", req.JobId),
	)

	// Validate required fields
	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	job, err := s.repo.GetJob(ctx, req.JobId)
	if err != nil {
		s.logger.Error("Error fetching job",
			logging.Err(err),
			logging.F("job_id", req.JobId),
		)
		return nil, status.Errorf(codes.Internal, "failed to fetch job: %v", err)
	}

	if job == nil {
		return nil, status.Errorf(codes.NotFound, "job not found: %s", req.JobId)
	}

	return &ingestv1.GetIngestJobResponse{
		Job: jobToProto(job),
	}, nil
}

// GetRemainingFiles retrieves unprocessed files for job resume.
func (s *Service) GetRemainingFiles(ctx context.Context, req *ingestv1.GetRemainingFilesRequest) (*ingestv1.GetRemainingFilesResponse, error) {
	s.logger.Debug("GetRemainingFiles called",
		logging.F("job_id", req.JobId),
	)

	// Validate required fields
	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	// Get job to retrieve file manifest
	job, err := s.repo.GetJob(ctx, req.JobId)
	if err != nil {
		s.logger.Error("Error fetching job",
			logging.Err(err),
			logging.F("job_id", req.JobId),
		)
		return nil, status.Errorf(codes.Internal, "failed to fetch job: %v", err)
	}

	if job == nil {
		return nil, status.Errorf(codes.NotFound, "job not found: %s", req.JobId)
	}

	// Get remaining files
	remaining, err := s.repo.GetRemainingFilesForJob(ctx, req.JobId, job.FileManifest)
	if err != nil {
		s.logger.Error("Error getting remaining files",
			logging.Err(err),
			logging.F("job_id", req.JobId),
		)
		return nil, status.Errorf(codes.Internal, "failed to get remaining files: %v", err)
	}

	// Apply pagination
	limit := int(req.Limit)
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	// Paginate results
	total := len(remaining)
	if offset >= total {
		remaining = []string{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		remaining = remaining[offset:end]
	}

	// Convert to proto messages
	files := make([]*ingestv1.RemainingFile, len(remaining))
	for i, path := range remaining {
		files[i] = &ingestv1.RemainingFile{
			FilePath: path,
		}
	}

	return &ingestv1.GetRemainingFilesResponse{
		Files:      files,
		TotalCount: int64(total),
	}, nil
}

// RecordIngestError records an error that occurred during ingest.
func (s *Service) RecordIngestError(ctx context.Context, req *ingestv1.RecordIngestErrorRequest) (*ingestv1.RecordIngestErrorResponse, error) {
	s.logger.Debug("RecordIngestError called",
		logging.F("job_id", req.JobId),
		logging.F("file_path", req.FilePath),
		logging.F("error_type", req.ErrorType.String()),
	)

	// Validate required fields
	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}
	if req.ErrorMessage == "" {
		return nil, status.Error(codes.InvalidArgument, "error_message is required")
	}

	// Convert error type
	errorType := errorTypeFromProto(req.ErrorType)

	// Build details map
	details := map[string]interface{}{
		"stack_trace":  req.StackTrace,
		"is_retryable": req.IsRetryable,
	}

	if err := s.repo.RecordError(ctx, req.JobId, req.FilePath, errorType, req.ErrorMessage, details); err != nil {
		s.logger.Error("Error recording ingest error",
			logging.Err(err),
			logging.F("job_id", req.JobId),
		)
		return nil, status.Errorf(codes.Internal, "failed to record error: %v", err)
	}

	// Return the recorded error
	return &ingestv1.RecordIngestErrorResponse{
		Error: &ingestv1.IngestError{
			JobId:        req.JobId,
			FilePath:     req.FilePath,
			ErrorType:    req.ErrorType,
			ErrorMessage: req.ErrorMessage,
			StackTrace:   req.StackTrace,
			IsRetryable:  req.IsRetryable,
			OccurredAt:   timestamppb.Now(),
		},
	}, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// buildEmailMetadata constructs metadata map from email request.
func buildEmailMetadata(req *ingestv1.IngestEmailRequest) map[string]interface{} {
	metadata := make(map[string]interface{})

	metadata["subject"] = req.Subject
	metadata["thread_id"] = req.ThreadId
	metadata["in_reply_to"] = req.InReplyTo
	metadata["source_tag"] = req.SourceTag
	metadata["labels"] = req.Labels

	if req.From != nil {
		metadata["from_name"] = req.From.Name
		metadata["from_address"] = req.From.Address
	}

	// Store attachment metadata
	if len(req.Attachments) > 0 {
		attachments := make([]map[string]interface{}, len(req.Attachments))
		for i, a := range req.Attachments {
			attachments[i] = map[string]interface{}{
				"filename":     a.Filename,
				"mime_type":    a.MimeType,
				"size_bytes":   a.SizeBytes,
				"content_hash": a.ContentHash,
			}
		}
		metadata["attachments"] = attachments
	}

	// Store headers
	if len(req.Headers) > 0 {
		metadata["headers"] = req.Headers
	}

	return metadata
}

// collectParticipantEmails extracts all email addresses from the request.
func collectParticipantEmails(req *ingestv1.IngestEmailRequest) []string {
	emails := make([]string, 0)

	if req.From != nil && req.From.Address != "" {
		emails = append(emails, req.From.Address)
	}

	for _, addr := range req.To {
		if addr != nil && addr.Address != "" {
			emails = append(emails, addr.Address)
		}
	}

	for _, addr := range req.Cc {
		if addr != nil && addr.Address != "" {
			emails = append(emails, addr.Address)
		}
	}

	for _, addr := range req.Bcc {
		if addr != nil && addr.Address != "" {
			emails = append(emails, addr.Address)
		}
	}

	return emails
}

// buildMeetingMetadata constructs metadata map from meeting request.
func buildMeetingMetadata(req *ingestv1.IngestMeetingRequest) map[string]interface{} {
	metadata := make(map[string]interface{})

	metadata["title"] = req.Title
	metadata["description"] = req.Description
	metadata["platform"] = req.Platform.String()
	metadata["meeting_url"] = req.MeetingUrl
	metadata["calendar_event_id"] = req.CalendarEventId
	metadata["labels"] = req.Labels
	metadata["job_id"] = req.JobId

	// Store organizer info
	if req.Organizer != nil {
		metadata["organizer_name"] = req.Organizer.Name
		metadata["organizer_email"] = req.Organizer.Email
	}

	// Store participant info
	if len(req.Participants) > 0 {
		participants := make([]map[string]interface{}, len(req.Participants))
		for i, p := range req.Participants {
			participants[i] = map[string]interface{}{
				"name":             p.Name,
				"email":            p.Email,
				"is_organizer":     p.IsOrganizer,
				"attended":         p.Attended,
				"duration_seconds": p.DurationSeconds,
			}
		}
		metadata["participants"] = participants
	}

	// Store chat messages in metadata
	if len(req.ChatMessages) > 0 {
		chatMessages := make([]map[string]interface{}, len(req.ChatMessages))
		for i, m := range req.ChatMessages {
			msg := map[string]interface{}{
				"sender":     m.Sender,
				"text":       m.Text,
				"is_private": m.IsPrivate,
				"recipient":  m.Recipient,
			}
			if m.Timestamp != nil {
				msg["timestamp"] = m.Timestamp.AsTime().Format(time.RFC3339)
			}
			chatMessages[i] = msg
		}
		metadata["chat_messages"] = chatMessages
	}

	// Store attachment metadata
	if len(req.Attachments) > 0 {
		attachments := make([]map[string]interface{}, len(req.Attachments))
		for i, a := range req.Attachments {
			attachments[i] = map[string]interface{}{
				"filename":     a.Filename,
				"mime_type":    a.MimeType,
				"size_bytes":   a.SizeBytes,
				"content_hash": a.ContentHash,
			}
		}
		metadata["attachments"] = attachments
	}

	return metadata
}

// buildTranscriptContent converts transcript segments to VTT-like format.
func buildTranscriptContent(segments []*ingestv1.TranscriptSegment) string {
	if len(segments) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("WEBVTT\n\n")

	for i, seg := range segments {
		if seg == nil {
			continue
		}

		// Format timestamps
		startTime := formatVTTTime(seg.StartSeconds)
		endTime := formatVTTTime(seg.EndSeconds)

		builder.WriteString(fmt.Sprintf("%d\n", i+1))
		builder.WriteString(fmt.Sprintf("%s --> %s\n", startTime, endTime))
		builder.WriteString(fmt.Sprintf("<%s> %s\n\n", seg.Speaker, seg.Text))
	}

	return builder.String()
}

// formatVTTTime converts seconds to VTT timestamp format (HH:MM:SS.mmm).
func formatVTTTime(seconds float64) string {
	hours := int(seconds / 3600)
	minutes := int(seconds/60) % 60
	secs := int(seconds) % 60
	millis := int((seconds - float64(int(seconds))) * 1000)

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, secs, millis)
}

// computeSimpleHash generates a simple hash for content.
// Note: In production, this should use crypto/sha256.
func computeSimpleHash(content string) string {
	if content == "" {
		return ""
	}
	// Simple hash based on content length and first/last characters
	// In production, use crypto/sha256
	hash := fmt.Sprintf("%d-%x", len(content), content[:min(16, len(content))])
	return hash
}

// =============================================================================
// Conversion Helpers
// =============================================================================

func jobToProto(job *storage.IngestJob) *ingestv1.IngestJob {
	if job == nil {
		return nil
	}

	proto := &ingestv1.IngestJob{
		Id:             job.ID,
		TenantId:       job.TenantID,
		Name:           job.SourceTag,
		Status:         jobStatusToProto(job.Status),
		TotalFiles:     int64(job.TotalFiles),
		ProcessedCount: int64(job.ProcessedCount),
		FailedCount:    int64(job.FailedCount),
		SkippedCount:   int64(job.SkippedCount),
		CreatedAt:      timestamppb.New(job.CreatedAt),
	}

	if job.StartedAt != nil {
		proto.StartedAt = timestamppb.New(*job.StartedAt)
	}
	if job.CompletedAt != nil {
		proto.CompletedAt = timestamppb.New(*job.CompletedAt)
	}

	// Add source path from file manifest
	if len(job.FileManifest) > 0 {
		proto.SourcePath = job.FileManifest[0]
	}

	// Convert options to metadata
	if job.Options != nil {
		metadata := make(map[string]string)
		for k, v := range job.Options {
			if str, ok := v.(string); ok {
				metadata[k] = str
			} else {
				jsonBytes, _ := json.Marshal(v)
				metadata[k] = string(jsonBytes)
			}
		}
		proto.Metadata = metadata
	}

	return proto
}

func jobStatusToProto(status storage.IngestJobStatus) ingestv1.JobStatus {
	switch status {
	case storage.IngestJobStatusPending:
		return ingestv1.JobStatus_JOB_STATUS_CREATED
	case storage.IngestJobStatusInProgress:
		return ingestv1.JobStatus_JOB_STATUS_RUNNING
	case storage.IngestJobStatusCompleted:
		return ingestv1.JobStatus_JOB_STATUS_COMPLETED
	case storage.IngestJobStatusCompletedErrors:
		return ingestv1.JobStatus_JOB_STATUS_COMPLETED
	case storage.IngestJobStatusFailed:
		return ingestv1.JobStatus_JOB_STATUS_FAILED
	case storage.IngestJobStatusCancelled:
		return ingestv1.JobStatus_JOB_STATUS_CANCELLED
	default:
		return ingestv1.JobStatus_JOB_STATUS_UNSPECIFIED
	}
}

func duplicateReasonToProto(reason string) ingestv1.DuplicateReason {
	switch reason {
	case "message_id":
		return ingestv1.DuplicateReason_DUPLICATE_REASON_MESSAGE_ID
	case "content_hash":
		return ingestv1.DuplicateReason_DUPLICATE_REASON_CONTENT_HASH
	default:
		return ingestv1.DuplicateReason_DUPLICATE_REASON_UNSPECIFIED
	}
}

func errorTypeFromProto(et ingestv1.ErrorType) storage.IngestErrorType {
	switch et {
	case ingestv1.ErrorType_ERROR_TYPE_PARSE_ERROR:
		return storage.ErrorTypeParse
	case ingestv1.ErrorType_ERROR_TYPE_VALIDATION:
		return storage.ErrorTypeValidation
	case ingestv1.ErrorType_ERROR_TYPE_STORAGE:
		return storage.ErrorTypeStorage
	case ingestv1.ErrorType_ERROR_TYPE_NETWORK:
		return storage.ErrorTypeIO
	default:
		return storage.ErrorTypeUnexpected
	}
}

func sourceSystemForPlatform(platform ingestv1.Platform) string {
	switch platform {
	case ingestv1.Platform_PLATFORM_GMAIL:
		return storage.SourceSystemGmail
	case ingestv1.Platform_PLATFORM_ZOOM:
		return "zoom_meeting"
	case ingestv1.Platform_PLATFORM_GOOGLE_MEET:
		return "google_meet"
	case ingestv1.Platform_PLATFORM_TEAMS:
		return "teams_meeting"
	case ingestv1.Platform_PLATFORM_SLACK:
		return "slack"
	case ingestv1.Platform_PLATFORM_LOCAL:
		return storage.SourceSystemManualEML
	default:
		return "unknown"
	}
}

func platformToSourceTag(platform ingestv1.Platform) string {
	switch platform {
	case ingestv1.Platform_PLATFORM_GMAIL:
		return "gmail"
	case ingestv1.Platform_PLATFORM_OUTLOOK:
		return "outlook"
	case ingestv1.Platform_PLATFORM_SLACK:
		return "slack"
	case ingestv1.Platform_PLATFORM_TEAMS:
		return "teams"
	case ingestv1.Platform_PLATFORM_ZOOM:
		return "zoom"
	case ingestv1.Platform_PLATFORM_GOOGLE_MEET:
		return "google_meet"
	case ingestv1.Platform_PLATFORM_LOCAL:
		return "local"
	default:
		return "unknown"
	}
}

func platformToContentType(platform ingestv1.Platform) string {
	switch platform {
	case ingestv1.Platform_PLATFORM_GMAIL, ingestv1.Platform_PLATFORM_OUTLOOK:
		return "email"
	case ingestv1.Platform_PLATFORM_SLACK, ingestv1.Platform_PLATFORM_TEAMS:
		return "chat"
	case ingestv1.Platform_PLATFORM_ZOOM, ingestv1.Platform_PLATFORM_GOOGLE_MEET:
		return "meeting"
	default:
		return "unknown"
	}
}
