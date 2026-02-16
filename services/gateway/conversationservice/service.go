// Package conversationservice implements the ConversationService gRPC server.
package conversationservice

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	conversationv1 "github.com/otherjamesbrown/penfold/api/proto/conversation/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Repository defines the interface for conversation data access.
type Repository interface {
	ListConversations(ctx context.Context, tenantID string, limit, offset int32) ([]ConversationSummary, int64, error)
	GetConversation(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error)
}

// Service implements the ConversationService gRPC server.
type Service struct {
	conversationv1.UnimplementedConversationServiceServer
	repo   Repository
	logger logging.Logger
}

// NewService creates a new conversation service.
func NewService(repo Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// ListConversations lists conversations with pagination.
func (s *Service) ListConversations(ctx context.Context, req *conversationv1.ListConversationsRequest) (*conversationv1.ListConversationsResponse, error) {
	// Validate tenant ID
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Query conversations
	conversations, totalCount, err := s.repo.ListConversations(ctx, req.GetTenantId(), req.GetLimit(), req.GetOffset())
	if err != nil {
		s.logger.Error("failed to list conversations", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list conversations: %v", err)
	}

	// Convert to proto
	protoConversations := make([]*conversationv1.ConversationSummary, len(conversations))
	for i, conv := range conversations {
		protoConversations[i] = conversationSummaryToProto(&conv)
	}

	return &conversationv1.ListConversationsResponse{
		Conversations: protoConversations,
		TotalCount:    totalCount,
	}, nil
}

// ShowConversation gets a single conversation with items and participants.
func (s *Service) ShowConversation(ctx context.Context, req *conversationv1.ShowConversationRequest) (*conversationv1.ShowConversationResponse, error) {
	// Validate tenant ID
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Validate conversation ID
	if req.GetConversationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "conversation_id is required")
	}

	// Query conversation
	conversation, err := s.repo.GetConversation(ctx, req.GetTenantId(), req.GetConversationId())
	if err != nil {
		s.logger.Error("failed to get conversation", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get conversation: %v", err)
	}

	// Check if conversation exists
	if conversation == nil {
		return nil, status.Error(codes.NotFound, "conversation not found")
	}

	// Convert to proto
	return conversationDetailToProto(conversation), nil
}

// conversationSummaryToProto converts a ConversationSummary to proto.
func conversationSummaryToProto(conv *ConversationSummary) *conversationv1.ConversationSummary {
	pb := &conversationv1.ConversationSummary{
		Id:               conv.ID,
		TenantId:         conv.TenantID,
		Topic:            conv.Topic,
		ParticipantCount: conv.ParticipantCount,
		ItemCount:        conv.ItemCount,
		CreatedAt:        timestamppb.New(conv.CreatedAt),
		UpdatedAt:        timestamppb.New(conv.UpdatedAt),
	}

	// Handle optional fields
	if conv.ThreadKey != nil {
		pb.ThreadKey = conv.ThreadKey
	}

	if conv.FirstSeen != nil {
		pb.FirstSeen = timestamppb.New(*conv.FirstSeen)
	}

	if conv.LastSeen != nil {
		pb.LastSeen = timestamppb.New(*conv.LastSeen)
	}

	return pb
}

// conversationDetailToProto converts a ConversationDetail to proto.
func conversationDetailToProto(conv *ConversationDetail) *conversationv1.ShowConversationResponse {
	resp := &conversationv1.ShowConversationResponse{
		Id:               conv.ID,
		TenantId:         conv.TenantID,
		Topic:            conv.Topic,
		ParticipantCount: conv.ParticipantCount,
		ItemCount:        conv.ItemCount,
		CreatedAt:        timestamppb.New(conv.CreatedAt),
		UpdatedAt:        timestamppb.New(conv.UpdatedAt),
	}

	// Handle optional fields
	if conv.ThreadKey != nil {
		resp.ThreadKey = conv.ThreadKey
	}

	if conv.FirstSeen != nil {
		resp.FirstSeen = timestamppb.New(*conv.FirstSeen)
	}

	if conv.LastSeen != nil {
		resp.LastSeen = timestamppb.New(*conv.LastSeen)
	}

	// Convert items
	resp.Items = make([]*conversationv1.ConversationItem, len(conv.Items))
	for i, item := range conv.Items {
		protoItem := &conversationv1.ConversationItem{
			ConversationId: item.ConversationID,
			ContentId:      item.ContentID,
			AddedAt:        timestamppb.New(item.AddedAt),
		}
		if item.SourceID != nil {
			protoItem.SourceId = item.SourceID
		}
		resp.Items[i] = protoItem
	}

	// Convert participants
	resp.Participants = make([]*conversationv1.ConversationParticipant, len(conv.Participants))
	for i, part := range conv.Participants {
		protoParticipant := &conversationv1.ConversationParticipant{
			ConversationId: part.ConversationID,
		}
		if part.Name != nil {
			protoParticipant.Name = part.Name
		}
		if part.Address != nil {
			protoParticipant.Address = part.Address
		}
		resp.Participants[i] = protoParticipant
	}

	return resp
}
