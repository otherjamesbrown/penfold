package threadsservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	threadsv1 "github.com/otherjamesbrown/penfold/api/proto/threads/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ============ Mock Repository ============

// MockRepository is a mock implementation of the thread repository.
type MockRepository struct {
	ListThreadsFunc func(ctx context.Context, tenantID string, limit, offset int32) ([]ThreadSummary, int64, error)
	GetThreadFunc   func(ctx context.Context, tenantID string, threadID int64) (*ThreadDetail, error)
}

func (m *MockRepository) ListThreads(ctx context.Context, tenantID string, limit, offset int32) ([]ThreadSummary, int64, error) {
	if m.ListThreadsFunc != nil {
		return m.ListThreadsFunc(ctx, tenantID, limit, offset)
	}
	return nil, 0, errors.New("ListThreadsFunc not implemented")
}

func (m *MockRepository) GetThread(ctx context.Context, tenantID string, threadID int64) (*ThreadDetail, error) {
	if m.GetThreadFunc != nil {
		return m.GetThreadFunc(ctx, tenantID, threadID)
	}
	return nil, errors.New("GetThreadFunc not implemented")
}

// ============ Test Helpers ============

func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = "error" // Quiet logging for tests
	return logging.NewLogger(cfg)
}

// ============ ListThreads Tests ============

func TestListThreads_ReturnsThreads(t *testing.T) {
	// Create test data
	now := time.Now()
	summary1 := "This is a summary of the first thread"

	mockRepo := &MockRepository{
		ListThreadsFunc: func(ctx context.Context, tenantID string, limit, offset int32) ([]ThreadSummary, int64, error) {
			assert.Equal(t, "tenant-123", tenantID)
			assert.Equal(t, int32(50), limit)
			assert.Equal(t, int32(0), offset)

			return []ThreadSummary{
				{
					ID:              1,
					Subject:         "Project Update",
					MessageCount:    5,
					FirstMessageAt:  now.Add(-48 * time.Hour),
					LastMessageAt:   now.Add(-2 * time.Hour),
					ParticipantIDs:  []int64{10, 20, 30},
					ThreadSummary:   &summary1,
					LatestSourceID:  100,
				},
				{
					ID:              2,
					Subject:         "Meeting Follow-up",
					MessageCount:    3,
					FirstMessageAt:  now.Add(-24 * time.Hour),
					LastMessageAt:   now.Add(-1 * time.Hour),
					ParticipantIDs:  []int64{10, 40},
					ThreadSummary:   nil, // No summary yet
					LatestSourceID:  101,
				},
			}, 2, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &threadsv1.ListThreadsRequest{
		TenantId: "tenant-123",
		Limit:    50,
		Offset:   0,
	}

	resp, err := svc.ListThreads(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(2), resp.TotalCount)
	assert.Len(t, resp.Threads, 2)

	// Verify first thread mapping
	thread1 := resp.Threads[0]
	assert.Equal(t, int64(1), thread1.Id)
	assert.Equal(t, "Project Update", thread1.Subject)
	assert.Equal(t, int32(5), thread1.MessageCount)
	assert.NotNil(t, thread1.FirstMessageAt)
	assert.NotNil(t, thread1.LastMessageAt)
	assert.Equal(t, []int64{10, 20, 30}, thread1.ParticipantIds)
	assert.NotNil(t, thread1.Summary)
	assert.Equal(t, summary1, *thread1.Summary)

	// Verify second thread mapping (no summary)
	thread2 := resp.Threads[1]
	assert.Equal(t, int64(2), thread2.Id)
	assert.Equal(t, "Meeting Follow-up", thread2.Subject)
	assert.Nil(t, thread2.Summary)
}

func TestListThreads_EmptyResult(t *testing.T) {
	mockRepo := &MockRepository{
		ListThreadsFunc: func(ctx context.Context, tenantID string, limit, offset int32) ([]ThreadSummary, int64, error) {
			return []ThreadSummary{}, 0, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &threadsv1.ListThreadsRequest{
		TenantId: "tenant-123",
		Limit:    50,
		Offset:   0,
	}

	resp, err := svc.ListThreads(context.Background(), req)

	require.NoError(t, err, "Empty result should not return an error")
	require.NotNil(t, resp)
	assert.Equal(t, int64(0), resp.TotalCount)
	assert.Empty(t, resp.Threads, "Should return empty list, not nil")
}

func TestListThreads_MissingTenantID(t *testing.T) {
	mockRepo := &MockRepository{}
	svc := NewService(mockRepo, testLogger())

	req := &threadsv1.ListThreadsRequest{
		TenantId: "", // Missing tenant ID
		Limit:    50,
		Offset:   0,
	}

	resp, err := svc.ListThreads(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "tenant_id")
}

func TestListThreads_RepositoryError(t *testing.T) {
	mockRepo := &MockRepository{
		ListThreadsFunc: func(ctx context.Context, tenantID string, limit, offset int32) ([]ThreadSummary, int64, error) {
			return nil, 0, errors.New("database connection failed")
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &threadsv1.ListThreadsRequest{
		TenantId: "tenant-123",
		Limit:    50,
		Offset:   0,
	}

	resp, err := svc.ListThreads(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "failed to list threads")
}

// ============ GetThread Tests ============

func TestGetThread_ReturnsThreadWithMessages(t *testing.T) {
	now := time.Now()
	summary := "Discussion about the new feature implementation"

	mockRepo := &MockRepository{
		GetThreadFunc: func(ctx context.Context, tenantID string, threadID int64) (*ThreadDetail, error) {
			assert.Equal(t, "tenant-123", tenantID)
			assert.Equal(t, int64(42), threadID)

			return &ThreadDetail{
				ID:              42,
				Subject:         "Feature Implementation",
				MessageCount:    3,
				FirstMessageAt:  now.Add(-48 * time.Hour),
				LastMessageAt:   now.Add(-2 * time.Hour),
				ParticipantIDs:  []int64{10, 20},
				ThreadSummary:   &summary,
				Messages: []ThreadMessage{
					{
						ID:               1,
						SourceID:         100,
						MessageID:        "<msg1@example.com>",
						PositionInThread: 1,
						MessageDate:      now.Add(-48 * time.Hour),
						FromEmail:        "alice@example.com",
						FromName:         "Alice",
						Subject:          "Feature Implementation",
						BodyPreview:      "Let's discuss the new feature...",
						IsReply:          false,
					},
					{
						ID:               2,
						SourceID:         101,
						MessageID:        "<msg2@example.com>",
						PositionInThread: 2,
						MessageDate:      now.Add(-24 * time.Hour),
						FromEmail:        "bob@example.com",
						FromName:         "Bob",
						Subject:          "Re: Feature Implementation",
						BodyPreview:      "I agree, we should start with...",
						IsReply:          true,
					},
					{
						ID:               3,
						SourceID:         102,
						MessageID:        "<msg3@example.com>",
						PositionInThread: 3,
						MessageDate:      now.Add(-2 * time.Hour),
						FromEmail:        "alice@example.com",
						FromName:         "Alice",
						Subject:          "Re: Feature Implementation",
						BodyPreview:      "Sounds good. Let's schedule...",
						IsReply:          true,
					},
				},
			}, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &threadsv1.GetThreadRequest{
		TenantId: "tenant-123",
		ThreadId: 42,
	}

	resp, err := svc.GetThread(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify thread metadata
	assert.Equal(t, int64(42), resp.Id)
	assert.Equal(t, "Feature Implementation", resp.Subject)
	assert.Equal(t, int32(3), resp.MessageCount)
	assert.NotNil(t, resp.FirstMessageAt)
	assert.NotNil(t, resp.LastMessageAt)
	assert.Equal(t, []int64{10, 20}, resp.ParticipantIds)
	assert.NotNil(t, resp.Summary)
	assert.Equal(t, summary, *resp.Summary)

	// Verify messages are ordered by position
	require.Len(t, resp.Messages, 3)
	assert.Equal(t, int32(1), resp.Messages[0].PositionInThread)
	assert.Equal(t, int32(2), resp.Messages[1].PositionInThread)
	assert.Equal(t, int32(3), resp.Messages[2].PositionInThread)

	// Verify first message details
	msg1 := resp.Messages[0]
	assert.Equal(t, int64(100), msg1.SourceId)
	assert.Equal(t, "<msg1@example.com>", msg1.MessageId)
	assert.Equal(t, "alice@example.com", msg1.FromEmail)
	assert.Equal(t, "Alice", msg1.FromName)
	assert.Equal(t, "Feature Implementation", msg1.Subject)
	assert.Contains(t, msg1.BodyPreview, "Let's discuss")
	assert.False(t, msg1.IsReply)

	// Verify second message is a reply
	msg2 := resp.Messages[1]
	assert.True(t, msg2.IsReply)
	assert.Equal(t, "bob@example.com", msg2.FromEmail)
}

func TestGetThread_NotFound(t *testing.T) {
	mockRepo := &MockRepository{
		GetThreadFunc: func(ctx context.Context, tenantID string, threadID int64) (*ThreadDetail, error) {
			return nil, nil // Thread not found
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &threadsv1.GetThreadRequest{
		TenantId: "tenant-123",
		ThreadId: 999,
	}

	resp, err := svc.GetThread(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "thread not found")
}

func TestGetThread_InvalidID_Zero(t *testing.T) {
	mockRepo := &MockRepository{}
	svc := NewService(mockRepo, testLogger())

	req := &threadsv1.GetThreadRequest{
		TenantId: "tenant-123",
		ThreadId: 0, // Invalid ID
	}

	resp, err := svc.GetThread(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "thread_id")
}

func TestGetThread_InvalidID_Negative(t *testing.T) {
	mockRepo := &MockRepository{}
	svc := NewService(mockRepo, testLogger())

	req := &threadsv1.GetThreadRequest{
		TenantId: "tenant-123",
		ThreadId: -5, // Invalid ID
	}

	resp, err := svc.GetThread(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "thread_id")
}

func TestGetThread_MissingTenantID(t *testing.T) {
	mockRepo := &MockRepository{}
	svc := NewService(mockRepo, testLogger())

	req := &threadsv1.GetThreadRequest{
		TenantId: "", // Missing tenant ID
		ThreadId: 42,
	}

	resp, err := svc.GetThread(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "tenant_id")
}

func TestGetThread_RepositoryError(t *testing.T) {
	mockRepo := &MockRepository{
		GetThreadFunc: func(ctx context.Context, tenantID string, threadID int64) (*ThreadDetail, error) {
			return nil, errors.New("database connection failed")
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &threadsv1.GetThreadRequest{
		TenantId: "tenant-123",
		ThreadId: 42,
	}

	resp, err := svc.GetThread(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "failed to get thread")
}

// ============ Interface Compliance Test ============

func TestServiceImplementsInterface(t *testing.T) {
	// This will fail to compile until the proto is generated and service is implemented
	var _ threadsv1.ThreadsServiceServer = (*Service)(nil)
}
