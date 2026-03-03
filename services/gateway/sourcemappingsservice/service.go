// Package sourcemappingsservice implements the SourceMappingService gRPC server.
package sourcemappingsservice

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	source_mappingsv1 "github.com/otherjamesbrown/penfold/api/proto/source_mappings/v1"
	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/source_mappings"
)

// Service implements the SourceMappingService gRPC server.
type Service struct {
	source_mappingsv1.UnimplementedSourceMappingServiceServer
	repo   *source_mappings.Repository
	logger logging.Logger
}

// NewService creates a new source mappings service.
func NewService(repo *source_mappings.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// CreateSourceMapping creates a new source-to-project mapping.
func (s *Service) CreateSourceMapping(ctx context.Context, req *source_mappingsv1.CreateSourceMappingRequest) (*source_mappingsv1.CreateSourceMappingResponse, error) {
	s.logger.Debug("CreateSourceMapping called",
		logging.F("tenant_id", req.TenantId),
		logging.F("project_id", req.ProjectId),
		logging.F("source_type", req.SourceType),
		logging.F("source_identifier", req.SourceIdentifier),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.ProjectId == 0 {
		return nil, status.Error(codes.InvalidArgument, "project_id is required")
	}
	if req.SourceType == "" {
		return nil, status.Error(codes.InvalidArgument, "source_type is required")
	}
	if req.SourceIdentifier == "" {
		return nil, status.Error(codes.InvalidArgument, "source_identifier is required")
	}

	// Default match_type to "exact"
	matchType := req.MatchType
	if matchType == "" {
		matchType = "exact"
	}

	// Default confidence to 0.95
	var confidence float32 = 0.95
	if req.Confidence != nil {
		confidence = *req.Confidence
	}

	m := &source_mappings.SourceMapping{
		TenantID:         req.TenantId,
		ProjectID:        req.ProjectId,
		SourceType:       req.SourceType,
		SourceIdentifier: req.SourceIdentifier,
		MatchType:        matchType,
		Confidence:       confidence,
	}

	if req.Notes != nil {
		m.Notes = req.Notes
	}

	if err := s.repo.CreateSourceMapping(ctx, m); err != nil {
		if errors.Is(err, pferrors.ErrConflict) {
			return nil, status.Errorf(codes.AlreadyExists, "source mapping already exists: %s/%s", req.SourceType, req.SourceIdentifier)
		}
		s.logger.Error("Error creating source mapping", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create source mapping: %v", err)
	}

	return &source_mappingsv1.CreateSourceMappingResponse{
		Mapping: mappingToProto(m),
	}, nil
}

// ListSourceMappings lists source mappings with optional filters.
func (s *Service) ListSourceMappings(ctx context.Context, req *source_mappingsv1.ListSourceMappingsRequest) (*source_mappingsv1.ListSourceMappingsResponse, error) {
	s.logger.Debug("ListSourceMappings called",
		logging.F("tenant_id", req.TenantId),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	mappings, err := s.repo.ListSourceMappings(ctx, req.TenantId, req.ProjectId, req.SourceType)
	if err != nil {
		s.logger.Error("Error listing source mappings", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list source mappings: %v", err)
	}

	protoMappings := make([]*source_mappingsv1.SourceMapping, len(mappings))
	for i, m := range mappings {
		protoMappings[i] = mappingToProto(m)
	}

	return &source_mappingsv1.ListSourceMappingsResponse{
		Mappings: protoMappings,
	}, nil
}

// UpdateSourceMapping updates an existing source mapping.
func (s *Service) UpdateSourceMapping(ctx context.Context, req *source_mappingsv1.UpdateSourceMappingRequest) (*source_mappingsv1.UpdateSourceMappingResponse, error) {
	s.logger.Debug("UpdateSourceMapping called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	m, err := s.repo.UpdateSourceMapping(ctx, req.TenantId, req.Id, req.Confidence, req.Notes, req.Enabled)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "source mapping not found: %d", req.Id)
		}
		s.logger.Error("Error updating source mapping", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update source mapping: %v", err)
	}

	return &source_mappingsv1.UpdateSourceMappingResponse{
		Mapping: mappingToProto(m),
	}, nil
}

// DeleteSourceMapping deletes a source mapping by ID.
func (s *Service) DeleteSourceMapping(ctx context.Context, req *source_mappingsv1.DeleteSourceMappingRequest) (*source_mappingsv1.DeleteSourceMappingResponse, error) {
	s.logger.Debug("DeleteSourceMapping called",
		logging.F("tenant_id", req.TenantId),
		logging.F("id", req.Id),
	)

	if req.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if err := s.repo.DeleteSourceMapping(ctx, req.TenantId, req.Id); err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "source mapping not found: %d", req.Id)
		}
		s.logger.Error("Error deleting source mapping", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete source mapping: %v", err)
	}

	return &source_mappingsv1.DeleteSourceMappingResponse{
		Success: true,
	}, nil
}

func mappingToProto(m *source_mappings.SourceMapping) *source_mappingsv1.SourceMapping {
	proto := &source_mappingsv1.SourceMapping{
		Id:               m.ID,
		TenantId:         m.TenantID,
		ProjectId:        m.ProjectID,
		SourceType:       m.SourceType,
		SourceIdentifier: m.SourceIdentifier,
		MatchType:        m.MatchType,
		Confidence:       m.Confidence,
		Enabled:          m.Enabled,
		CreatedAt:        timestamppb.New(m.CreatedAt),
		UpdatedAt:        timestamppb.New(m.UpdatedAt),
	}

	if m.Notes != nil {
		proto.Notes = *m.Notes
	}

	return proto
}
