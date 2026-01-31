// Package contentservice implements the ContentProcessorService gRPC server.
package contentservice

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Service implements the ContentProcessorService gRPC server.
type Service struct {
	contentv1.UnimplementedContentProcessorServiceServer
	logger logging.Logger
}

// NewService creates a new content service.
func NewService(logger logging.Logger) *Service {
	return &Service{
		logger: logger,
	}
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
		logging.F("job_id", req.JobId),
	)

	// TODO: Implement status retrieval from database
	return nil, status.Error(codes.Unimplemented, "GetProcessingStatus not yet implemented")
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

	// TODO: Implement database retrieval
	return nil, status.Error(codes.Unimplemented, "GetContentItem not yet implemented")
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

	// TODO: Implement database query with pagination
	return nil, status.Error(codes.Unimplemented, "ListContentItems not yet implemented")
}

// ReprocessContent triggers reprocessing of an already-processed content item.
func (s *Service) ReprocessContent(ctx context.Context, req *contentv1.ReprocessContentRequest) (*contentv1.ReprocessContentResponse, error) {
	s.logger.Debug("ReprocessContent called",
		logging.F("content_id", req.ContentId),
		logging.F("reason", req.Reason),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// TODO: Implement reprocessing logic
	return nil, status.Error(codes.Unimplemented, "ReprocessContent not yet implemented")
}

// DeleteContentItem removes a content item and all derived data.
func (s *Service) DeleteContentItem(ctx context.Context, req *contentv1.DeleteContentItemRequest) (*contentv1.DeleteContentItemResponse, error) {
	s.logger.Info("DeleteContentItem called",
		logging.F("content_id", req.ContentId),
	)

	if req.ContentId == "" {
		return nil, status.Error(codes.InvalidArgument, "content_id is required")
	}

	// TODO: Implement database deletion
	// Should delete:
	// - Content item record
	// - Embeddings
	// - Summaries
	// - Extracted entities
	// - Processing status records

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

	// TODO: Implement bulk deletion with filters
	// Build WHERE clause based on filters:
	// - tenant_id (required)
	// - source_type (optional)
	// - state (optional)
	// - created_at < before (optional)

	// For now, return empty response
	return &contentv1.DeleteContentItemsResponse{
		DeletedCount: 0,
		DeletedIds:   []string{},
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

	// TODO: Implement statistics query
	// Aggregate queries:
	// - Total count
	// - Count by source_type
	// - Count by state
	// - Count where embedding IS NOT NULL
	// - Count where summary IS NOT NULL
	// - Total storage (sum of content length)

	// For now, return empty stats
	return &contentv1.ContentStats{
		TenantId:           req.TenantId,
		TotalCount:         0,
		CountByType:        make(map[string]int64),
		CountByState:       make(map[string]int64),
		TotalStorageBytes:  0,
		EmbeddedCount:      0,
		SummarizedCount:    0,
		ExtractedCount:     0,
	}, nil
}
