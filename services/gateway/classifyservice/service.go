// Package classifyservice implements the ClassificationSuggestionService gRPC server.
package classifyservice

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	classifyv1 "github.com/otherjamesbrown/penfold/api/proto/classify/v1"
	"github.com/otherjamesbrown/penfold/pkg/classify"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Repository defines the interface for classification suggestion data access.
type Repository interface {
	ListPending(ctx context.Context) ([]*classify.ClassificationSuggestion, error)
	UpdateStatus(ctx context.Context, id int, status string) error
}

// Service implements the ClassificationSuggestionService gRPC server.
type Service struct {
	classifyv1.UnimplementedClassificationSuggestionServiceServer
	repo   Repository
	logger logging.Logger
}

// NewService creates a new classification suggestion service.
func NewService(repo Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// ListSuggestions lists all pending classification suggestions.
func (s *Service) ListSuggestions(ctx context.Context, req *classifyv1.ListSuggestionsRequest) (*classifyv1.ListSuggestionsResponse, error) {
	// Query pending suggestions
	suggestions, err := s.repo.ListPending(ctx)
	if err != nil {
		s.logger.Error("failed to list suggestions", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list suggestions: %v", err)
	}

	// Convert to proto
	protoSuggestions := make([]*classifyv1.ClassificationSuggestion, len(suggestions))
	for i, sug := range suggestions {
		protoSuggestions[i] = suggestionToProto(sug)
	}

	return &classifyv1.ListSuggestionsResponse{
		Suggestions: protoSuggestions,
	}, nil
}

// ApproveSuggestion approves a classification suggestion.
func (s *Service) ApproveSuggestion(ctx context.Context, req *classifyv1.ApproveSuggestionRequest) (*classifyv1.ApproveSuggestionResponse, error) {
	if req.GetId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	// Update status to approved
	err := s.repo.UpdateStatus(ctx, int(req.GetId()), "approved")
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Error(codes.NotFound, "suggestion not found")
		}
		s.logger.Error("failed to approve suggestion", logging.F("id", req.GetId()), logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to approve suggestion: %v", err)
	}

	return &classifyv1.ApproveSuggestionResponse{
		Success: true,
	}, nil
}

// RejectSuggestion rejects a classification suggestion.
func (s *Service) RejectSuggestion(ctx context.Context, req *classifyv1.RejectSuggestionRequest) (*classifyv1.RejectSuggestionResponse, error) {
	if req.GetId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	// Update status to rejected
	err := s.repo.UpdateStatus(ctx, int(req.GetId()), "rejected")
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Error(codes.NotFound, "suggestion not found")
		}
		s.logger.Error("failed to reject suggestion", logging.F("id", req.GetId()), logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to reject suggestion: %v", err)
	}

	return &classifyv1.RejectSuggestionResponse{
		Success: true,
	}, nil
}

// suggestionToProto converts a ClassificationSuggestion to proto.
func suggestionToProto(sug *classify.ClassificationSuggestion) *classifyv1.ClassificationSuggestion {
	if sug == nil {
		return nil
	}
	return &classifyv1.ClassificationSuggestion{
		Id:           int32(sug.ID),
		ContentId:    sug.ContentID,
		SourceSystem: sug.SourceSystem,
		Pattern:      sug.Pattern,
		Confidence:   sug.Confidence,
		Status:       sug.Status,
		CreatedAt:    timestamppb.New(sug.CreatedAt),
	}
}
