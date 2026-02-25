// Package topicservice implements the TopicService gRPC server.
package topicservice

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	topicv1 "github.com/otherjamesbrown/penfold/api/proto/topic/v1"
	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/topics"
)

// Service implements the TopicService gRPC server.
type Service struct {
	topicv1.UnimplementedTopicServiceServer
	repo   *topics.Repository
	logger logging.Logger
}

// NewService creates a new topic service.
func NewService(repo *topics.Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// CreateTopic creates a new topic.
func (s *Service) CreateTopic(ctx context.Context, req *topicv1.CreateTopicRequest) (*topicv1.CreateTopicResponse, error) {
	s.logger.Debug("CreateTopic called",
		logging.F("tenant_id", req.TenantId),
		logging.F("name", req.Input.GetName()),
	)

	if req.Input == nil || req.Input.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "topic name is required")
	}

	topic := &topics.Topic{
		TenantID:    req.TenantId,
		Name:        req.Input.Name,
		Description: nullIfEmpty(req.Input.Description),
		Keywords:    req.Input.Keywords,
	}

	if err := s.repo.Create(ctx, topic); err != nil {
		if errors.Is(err, pferrors.ErrConflict) {
			return nil, status.Errorf(codes.AlreadyExists, "topic name already exists: %s", req.Input.Name)
		}
		s.logger.Error("Error creating topic", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to create topic: %v", err)
	}

	return &topicv1.CreateTopicResponse{
		Topic: topicToProto(topic),
	}, nil
}

// GetTopic retrieves a topic by ID or name.
func (s *Service) GetTopic(ctx context.Context, req *topicv1.GetTopicRequest) (*topicv1.GetTopicResponse, error) {
	s.logger.Debug("GetTopic called",
		logging.F("tenant_id", req.TenantId),
		logging.F("identifier", req.Identifier),
	)

	if req.Identifier == "" {
		return nil, status.Error(codes.InvalidArgument, "identifier is required")
	}

	topic, err := s.repo.Resolve(ctx, req.TenantId, req.Identifier)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "topic not found: %s", req.Identifier)
		}
		s.logger.Error("Error getting topic", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get topic: %v", err)
	}

	return &topicv1.GetTopicResponse{
		Topic: topicToProto(topic),
	}, nil
}

// UpdateTopic updates an existing topic.
func (s *Service) UpdateTopic(ctx context.Context, req *topicv1.UpdateTopicRequest) (*topicv1.UpdateTopicResponse, error) {
	s.logger.Debug("UpdateTopic called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "topic id is required")
	}

	topic, err := s.repo.GetByID(ctx, req.Id)
	if err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "topic not found: %d", req.Id)
		}
		s.logger.Error("Error getting topic", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get topic: %v", err)
	}

	if req.Input != nil {
		if req.Input.Name != "" {
			topic.Name = req.Input.Name
		}
		if req.Input.Description != "" {
			topic.Description = &req.Input.Description
		}
		if len(req.Input.Keywords) > 0 {
			topic.Keywords = req.Input.Keywords
		}
	}

	if err := s.repo.Update(ctx, topic); err != nil {
		if errors.Is(err, pferrors.ErrConflict) {
			return nil, status.Errorf(codes.AlreadyExists, "topic name already exists: %s", topic.Name)
		}
		s.logger.Error("Error updating topic", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to update topic: %v", err)
	}

	return &topicv1.UpdateTopicResponse{
		Topic: topicToProto(topic),
	}, nil
}

// DeleteTopic deletes a topic by ID.
func (s *Service) DeleteTopic(ctx context.Context, req *topicv1.DeleteTopicRequest) (*topicv1.DeleteTopicResponse, error) {
	s.logger.Debug("DeleteTopic called",
		logging.F("id", req.Id),
	)

	if req.Id == 0 {
		return nil, status.Error(codes.InvalidArgument, "topic id is required")
	}

	if err := s.repo.Delete(ctx, req.Id); err != nil {
		if errors.Is(err, pferrors.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "topic not found: %d", req.Id)
		}
		s.logger.Error("Error deleting topic", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to delete topic: %v", err)
	}

	return &topicv1.DeleteTopicResponse{
		Success: true,
	}, nil
}

// ListTopics lists topics with optional filtering.
func (s *Service) ListTopics(ctx context.Context, req *topicv1.ListTopicsRequest) (*topicv1.ListTopicsResponse, error) {
	s.logger.Debug("ListTopics called",
		logging.F("tenant_id", req.GetFilter().GetTenantId()),
	)

	if req.Filter == nil || req.Filter.TenantId == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required in filter")
	}

	filter := topics.TopicFilter{
		TenantID:   req.Filter.TenantId,
		NameSearch: req.Filter.NameSearch,
		Keyword:    req.Filter.Keyword,
		Limit:      int(req.Filter.Limit),
		Offset:     int(req.Filter.Offset),
	}

	topicsList, err := s.repo.List(ctx, filter)
	if err != nil {
		s.logger.Error("Error listing topics", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list topics: %v", err)
	}

	protoTopics := make([]*topicv1.Topic, len(topicsList))
	for i, t := range topicsList {
		protoTopics[i] = topicToProto(t)
	}

	return &topicv1.ListTopicsResponse{
		Topics:     protoTopics,
		TotalCount: int64(len(topicsList)),
	}, nil
}

// ==================== Conversion Helpers ====================

func topicToProto(t *topics.Topic) *topicv1.Topic {
	if t == nil {
		return nil
	}

	proto := &topicv1.Topic{
		Id:        t.ID,
		TenantId:  t.TenantID,
		Name:      t.Name,
		Keywords:  t.Keywords,
		CreatedAt: timestamppb.New(t.CreatedAt),
		UpdatedAt: timestamppb.New(t.UpdatedAt),
	}

	if t.Description != nil {
		proto.Description = *t.Description
	}

	return proto
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
