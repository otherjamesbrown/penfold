// Package server implements the ReviewService gRPC server.
package server

import (
	"context"

	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/reviewv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ReviewServer implements the ReviewService gRPC interface.
type ReviewServer struct {
	reviewv1.UnimplementedReviewServiceServer

	logger logging.Logger
}

// NewReviewServer creates a new ReviewServer instance.
func NewReviewServer(logger logging.Logger) *ReviewServer {
	return &ReviewServer{
		logger: logger.With(logging.F("component", "review_server")),
	}
}

// GetDailyReview retrieves today's review items grouped by category.
func (s *ReviewServer) GetDailyReview(ctx context.Context, req *reviewv1.GetDailyReviewRequest) (*reviewv1.GetDailyReviewResponse, error) {
	s.logger.Debug("GetDailyReview called",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("date", req.GetDate()),
		logging.F("include_processed", req.GetIncludeProcessed()),
	)

	return nil, status.Errorf(codes.Unimplemented, "GetDailyReview not implemented")
}

// GetReviewItem retrieves a single review item by its identifier.
func (s *ReviewServer) GetReviewItem(ctx context.Context, req *reviewv1.GetReviewItemRequest) (*reviewv1.GetReviewItemResponse, error) {
	s.logger.Debug("GetReviewItem called",
		logging.F("id", req.GetId()),
		logging.F("tenant_id", req.GetTenantId()),
	)

	return nil, status.Errorf(codes.Unimplemented, "GetReviewItem not implemented")
}

// ListReviewItems returns a filtered, paginated list of review items.
func (s *ReviewServer) ListReviewItems(ctx context.Context, req *reviewv1.ListReviewItemsRequest) (*reviewv1.ListReviewItemsResponse, error) {
	s.logger.Debug("ListReviewItems called",
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("page_size", req.GetPageSize()),
		logging.F("page_token", req.GetPageToken()),
	)

	return nil, status.Errorf(codes.Unimplemented, "ListReviewItems not implemented")
}

// ApproveItem marks a review item as reviewed and approved.
func (s *ReviewServer) ApproveItem(ctx context.Context, req *reviewv1.ApproveItemRequest) (*reviewv1.ApproveItemResponse, error) {
	s.logger.Debug("ApproveItem called",
		logging.F("id", req.GetId()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("note", req.GetNote()),
	)

	return nil, status.Errorf(codes.Unimplemented, "ApproveItem not implemented")
}

// RejectItem marks a review item as rejected or dismissed.
func (s *ReviewServer) RejectItem(ctx context.Context, req *reviewv1.RejectItemRequest) (*reviewv1.RejectItemResponse, error) {
	s.logger.Debug("RejectItem called",
		logging.F("id", req.GetId()),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("reason", req.GetReason()),
	)

	return nil, status.Errorf(codes.Unimplemented, "RejectItem not implemented")
}

// UndoAction reverses the last review action on an item.
func (s *ReviewServer) UndoAction(ctx context.Context, req *reviewv1.UndoActionRequest) (*reviewv1.UndoActionResponse, error) {
	s.logger.Debug("UndoAction called",
		logging.F("id", req.GetId()),
		logging.F("tenant_id", req.GetTenantId()),
	)

	return nil, status.Errorf(codes.Unimplemented, "UndoAction not implemented")
}

// GetReviewStats retrieves review statistics and metrics.
func (s *ReviewServer) GetReviewStats(ctx context.Context, req *reviewv1.GetReviewStatsRequest) (*reviewv1.GetReviewStatsResponse, error) {
	s.logger.Debug("GetReviewStats called",
		logging.F("tenant_id", req.GetTenantId()),
	)

	return nil, status.Errorf(codes.Unimplemented, "GetReviewStats not implemented")
}
