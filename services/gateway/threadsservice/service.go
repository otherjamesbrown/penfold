// Package threadsservice implements the ThreadsService gRPC server.
package threadsservice

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	threadsv1 "github.com/otherjamesbrown/penfold/api/proto/threads/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Repository defines the interface for thread data access.
type Repository interface {
	ListThreads(ctx context.Context, tenantID string, limit, offset int32) ([]ThreadSummary, int64, error)
	GetThread(ctx context.Context, tenantID string, threadID int64) (*ThreadDetail, error)
}

// Service implements the ThreadsService gRPC server.
type Service struct {
	threadsv1.UnimplementedThreadsServiceServer
	repo   Repository
	logger logging.Logger
}

// NewService creates a new threads service.
func NewService(repo Repository, logger logging.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// ListThreads lists threads with pagination.
func (s *Service) ListThreads(ctx context.Context, req *threadsv1.ListThreadsRequest) (*threadsv1.ListThreadsResponse, error) {
	// Validate tenant ID
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Query threads
	threads, totalCount, err := s.repo.ListThreads(ctx, req.GetTenantId(), req.GetLimit(), req.GetOffset())
	if err != nil {
		s.logger.Error("failed to list threads", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to list threads: %v", err)
	}

	// Convert to proto
	protoThreads := make([]*threadsv1.ThreadSummary, len(threads))
	for i, thread := range threads {
		protoThreads[i] = threadSummaryToProto(&thread)
	}

	return &threadsv1.ListThreadsResponse{
		Threads:    protoThreads,
		TotalCount: totalCount,
	}, nil
}

// GetThread gets a single thread with all messages.
func (s *Service) GetThread(ctx context.Context, req *threadsv1.GetThreadRequest) (*threadsv1.GetThreadResponse, error) {
	// Validate tenant ID
	if req.GetTenantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	// Validate thread ID
	if req.GetThreadId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "thread_id must be greater than 0")
	}

	// Query thread
	thread, err := s.repo.GetThread(ctx, req.GetTenantId(), req.GetThreadId())
	if err != nil {
		s.logger.Error("failed to get thread", logging.Err(err))
		return nil, status.Errorf(codes.Internal, "failed to get thread: %v", err)
	}

	// Check if thread exists
	if thread == nil {
		return nil, status.Error(codes.NotFound, "thread not found")
	}

	// Convert to proto
	return threadDetailToProto(thread), nil
}

// threadSummaryToProto converts a ThreadSummary to proto.
func threadSummaryToProto(thread *ThreadSummary) *threadsv1.ThreadSummary {
	pb := &threadsv1.ThreadSummary{
		Id:             thread.ID,
		RootMessageId:  thread.RootMessageID,
		Subject:        thread.Subject,
		MessageCount:   thread.MessageCount,
		FirstMessageAt: timestamppb.New(thread.FirstMessageAt),
		LastMessageAt:  timestamppb.New(thread.LastMessageAt),
		ParticipantIds: thread.ParticipantIDs,
		LatestSourceId: thread.LatestSourceID,
	}

	if thread.ThreadSummary != nil {
		pb.Summary = thread.ThreadSummary
	}

	return pb
}

// threadDetailToProto converts a ThreadDetail to proto.
func threadDetailToProto(thread *ThreadDetail) *threadsv1.GetThreadResponse {
	resp := &threadsv1.GetThreadResponse{
		Id:             thread.ID,
		Subject:        thread.Subject,
		MessageCount:   thread.MessageCount,
		FirstMessageAt: timestamppb.New(thread.FirstMessageAt),
		LastMessageAt:  timestamppb.New(thread.LastMessageAt),
		ParticipantIds: thread.ParticipantIDs,
	}

	if thread.ThreadSummary != nil {
		resp.Summary = thread.ThreadSummary
	}

	// Convert messages
	resp.Messages = make([]*threadsv1.ThreadMessage, len(thread.Messages))
	for i, msg := range thread.Messages {
		resp.Messages[i] = &threadsv1.ThreadMessage{
			SourceId:         msg.SourceID,
			MessageId:        msg.MessageID,
			PositionInThread: msg.PositionInThread,
			MessageDate:      timestamppb.New(msg.MessageDate),
			FromEmail:        msg.FromEmail,
			FromName:         msg.FromName,
			Subject:          msg.Subject,
			BodyPreview:      msg.BodyPreview,
			IsReply:          msg.IsReply,
		}
	}

	return resp
}
