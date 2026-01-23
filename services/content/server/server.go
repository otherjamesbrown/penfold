// Package server implements the Content Processor gRPC service.
package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/contentv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/metrics"
	"github.com/otherjamesbrown/penfold/services/content/config"
)

// ContentServer implements the ContentProcessorService gRPC service.
type ContentServer struct {
	contentv1.UnimplementedContentProcessorServiceServer

	cfg     *config.ServiceConfig
	logger  logging.Logger
	metrics *metrics.Metrics
}

// NewContentServer creates a new ContentServer instance.
func NewContentServer(cfg *config.ServiceConfig, logger logging.Logger, m *metrics.Metrics) *ContentServer {
	return &ContentServer{
		cfg:     cfg,
		logger:  logger.With(logging.F("component", "content_server")),
		metrics: m,
	}
}

// ProcessContent triggers the content processing pipeline for a specific item.
// STUB: Returns Unimplemented until storage layer is connected.
func (s *ContentServer) ProcessContent(ctx context.Context, req *contentv1.ProcessContentRequest) (*contentv1.ProcessContentResponse, error) {
	s.logger.Debug("ProcessContent called",
		logging.F("content_id", req.GetContentId()),
	)

	return nil, status.Error(codes.Unimplemented, "ProcessContent not yet implemented")
}

// GetProcessingStatus retrieves the current processing status of a content item.
// STUB: Returns Unimplemented until storage layer is connected.
func (s *ContentServer) GetProcessingStatus(ctx context.Context, req *contentv1.GetProcessingStatusRequest) (*contentv1.ProcessingStatus, error) {
	s.logger.Debug("GetProcessingStatus called",
		logging.F("content_id", req.GetContentId()),
		logging.F("job_id", req.GetJobId()),
	)

	return nil, status.Error(codes.Unimplemented, "GetProcessingStatus not yet implemented")
}

// GetContentItem retrieves a specific content item by ID.
// STUB: Returns Unimplemented until storage layer is connected.
func (s *ContentServer) GetContentItem(ctx context.Context, req *contentv1.GetContentItemRequest) (*contentv1.ContentItem, error) {
	s.logger.Debug("GetContentItem called",
		logging.F("content_id", req.GetContentId()),
		logging.F("include_embedding", req.GetIncludeEmbedding()),
	)

	return nil, status.Error(codes.Unimplemented, "GetContentItem not yet implemented")
}

// ListContentItems returns a paginated list of content items.
// STUB: Returns Unimplemented until storage layer is connected.
func (s *ContentServer) ListContentItems(ctx context.Context, req *contentv1.ListContentItemsRequest) (*contentv1.ListContentItemsResponse, error) {
	s.logger.Debug("ListContentItems called",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("source_type", req.GetSourceType()),
		logging.F("page_size", req.GetPageSize()),
	)

	return nil, status.Error(codes.Unimplemented, "ListContentItems not yet implemented")
}

// ReprocessContent triggers reprocessing of an already-processed content item.
// STUB: Returns Unimplemented until storage layer is connected.
func (s *ContentServer) ReprocessContent(ctx context.Context, req *contentv1.ReprocessContentRequest) (*contentv1.ReprocessContentResponse, error) {
	s.logger.Debug("ReprocessContent called",
		logging.F("content_id", req.GetContentId()),
		logging.F("reason", req.GetReason()),
	)

	return nil, status.Error(codes.Unimplemented, "ReprocessContent not yet implemented")
}
