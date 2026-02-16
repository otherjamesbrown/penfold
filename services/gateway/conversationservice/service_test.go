package conversationservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	conversationv1 "github.com/otherjamesbrown/penfold/api/proto/conversation/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ============ Mock Repository ============

// MockRepository is a mock implementation of the conversation repository.
type MockRepository struct {
	ListConversationsFunc        func(ctx context.Context, tenantID string, limit, offset int32) ([]ConversationSummary, int64, error)
	ListConversationsByStateFunc func(ctx context.Context, tenantID, state string, limit, offset int32) ([]ConversationSummary, int64, error)
	GetConversationFunc          func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error)
}

func (m *MockRepository) ListConversations(ctx context.Context, tenantID string, limit, offset int32) ([]ConversationSummary, int64, error) {
	if m.ListConversationsFunc != nil {
		return m.ListConversationsFunc(ctx, tenantID, limit, offset)
	}
	return nil, 0, errors.New("ListConversationsFunc not implemented")
}

func (m *MockRepository) ListConversationsByState(ctx context.Context, tenantID, state string, limit, offset int32) ([]ConversationSummary, int64, error) {
	if m.ListConversationsByStateFunc != nil {
		return m.ListConversationsByStateFunc(ctx, tenantID, state, limit, offset)
	}
	return nil, 0, errors.New("ListConversationsByStateFunc not implemented")
}

func (m *MockRepository) GetConversation(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
	if m.GetConversationFunc != nil {
		return m.GetConversationFunc(ctx, tenantID, conversationID)
	}
	return nil, errors.New("GetConversationFunc not implemented")
}

// ============ Test Helpers ============

func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = "error" // Quiet logging for tests
	return logging.NewLogger(cfg)
}

func stringPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func int64Ptr(i int64) *int64 {
	return &i
}

// ============ ListConversations Tests ============

func TestListConversations_ReturnsConversations(t *testing.T) {
	// Create test data
	now := time.Now()
	threadKey1 := "thread-abc-123"
	threadKey2 := "thread-def-456"

	mockRepo := &MockRepository{
		ListConversationsFunc: func(ctx context.Context, tenantID string, limit, offset int32) ([]ConversationSummary, int64, error) {
			assert.Equal(t, "tenant-123", tenantID)
			assert.Equal(t, int32(50), limit)
			assert.Equal(t, int32(0), offset)

			return []ConversationSummary{
				{
					ID:               "conv-1",
					TenantID:         "tenant-123",
					Topic:            "Project Alpha Discussion",
					ThreadKey:        &threadKey1,
					FirstSeen:        timePtr(now.Add(-48 * time.Hour)),
					LastSeen:         timePtr(now.Add(-2 * time.Hour)),
					ParticipantCount: 5,
					ItemCount:        12,
					CreatedAt:        now.Add(-48 * time.Hour),
					UpdatedAt:        now.Add(-2 * time.Hour),
				},
				{
					ID:               "conv-2",
					TenantID:         "tenant-123",
					Topic:            "Budget Review Q4",
					ThreadKey:        &threadKey2,
					FirstSeen:        timePtr(now.Add(-24 * time.Hour)),
					LastSeen:         timePtr(now.Add(-1 * time.Hour)),
					ParticipantCount: 3,
					ItemCount:        8,
					CreatedAt:        now.Add(-24 * time.Hour),
					UpdatedAt:        now.Add(-1 * time.Hour),
				},
			}, 2, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ListConversationsRequest{
		TenantId: "tenant-123",
		Limit:    50,
		Offset:   0,
	}

	resp, err := svc.ListConversations(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(2), resp.TotalCount)
	assert.Len(t, resp.Conversations, 2)

	// Verify first conversation mapping
	conv1 := resp.Conversations[0]
	assert.Equal(t, "conv-1", conv1.Id)
	assert.Equal(t, "tenant-123", conv1.TenantId)
	assert.Equal(t, "Project Alpha Discussion", conv1.Topic)
	assert.NotNil(t, conv1.ThreadKey)
	assert.Equal(t, threadKey1, *conv1.ThreadKey)
	assert.Equal(t, int32(5), conv1.ParticipantCount)
	assert.Equal(t, int32(12), conv1.ItemCount)
	assert.NotNil(t, conv1.FirstSeen)
	assert.NotNil(t, conv1.LastSeen)
	assert.NotNil(t, conv1.CreatedAt)
	assert.NotNil(t, conv1.UpdatedAt)

	// Verify second conversation mapping
	conv2 := resp.Conversations[1]
	assert.Equal(t, "conv-2", conv2.Id)
	assert.Equal(t, "Budget Review Q4", conv2.Topic)
	assert.Equal(t, int32(3), conv2.ParticipantCount)
	assert.Equal(t, int32(8), conv2.ItemCount)
}

func TestListConversations_EmptyResult(t *testing.T) {
	mockRepo := &MockRepository{
		ListConversationsFunc: func(ctx context.Context, tenantID string, limit, offset int32) ([]ConversationSummary, int64, error) {
			return []ConversationSummary{}, 0, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ListConversationsRequest{
		TenantId: "tenant-123",
		Limit:    50,
		Offset:   0,
	}

	resp, err := svc.ListConversations(context.Background(), req)

	require.NoError(t, err, "Empty result should not return an error")
	require.NotNil(t, resp)
	assert.Equal(t, int64(0), resp.TotalCount)
	assert.Empty(t, resp.Conversations, "Should return empty list, not nil")
}

func TestListConversations_MissingTenantID(t *testing.T) {
	mockRepo := &MockRepository{}
	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ListConversationsRequest{
		TenantId: "", // Missing tenant ID
		Limit:    50,
		Offset:   0,
	}

	resp, err := svc.ListConversations(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "tenant_id")
}

func TestListConversations_RepositoryError(t *testing.T) {
	mockRepo := &MockRepository{
		ListConversationsFunc: func(ctx context.Context, tenantID string, limit, offset int32) ([]ConversationSummary, int64, error) {
			return nil, 0, errors.New("database connection failed")
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ListConversationsRequest{
		TenantId: "tenant-123",
		Limit:    50,
		Offset:   0,
	}

	resp, err := svc.ListConversations(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "failed to list conversations")
}

// ============ ShowConversation Tests ============

func TestShowConversation_ReturnsConversationWithItemsAndParticipants(t *testing.T) {
	now := time.Now()
	threadKey := "thread-xyz-789"
	sourceID1 := int64(100)
	sourceID2 := int64(101)
	participantName1 := "Alice Smith"
	participantAddress1 := "alice@example.com"
	participantName2 := "Bob Jones"
	participantAddress2 := "bob@example.com"

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			assert.Equal(t, "tenant-123", tenantID)
			assert.Equal(t, "conv-42", conversationID)

			return &ConversationDetail{
				ID:               "conv-42",
				TenantID:         "tenant-123",
				Topic:            "Feature Implementation Discussion",
				ThreadKey:        &threadKey,
				FirstSeen:        timePtr(now.Add(-72 * time.Hour)),
				LastSeen:         timePtr(now.Add(-3 * time.Hour)),
				ParticipantCount: 2,
				ItemCount:        3,
				CreatedAt:        now.Add(-72 * time.Hour),
				UpdatedAt:        now.Add(-3 * time.Hour),
				Items: []ConversationItem{
					{
						ConversationID: "conv-42",
						ContentID:      "content-001",
						SourceID:       &sourceID1,
						AddedAt:        now.Add(-72 * time.Hour),
						TenantID:       "tenant-123",
					},
					{
						ConversationID: "conv-42",
						ContentID:      "content-002",
						SourceID:       &sourceID2,
						AddedAt:        now.Add(-48 * time.Hour),
						TenantID:       "tenant-123",
					},
					{
						ConversationID: "conv-42",
						ContentID:      "content-003",
						SourceID:       nil, // No source ID
						AddedAt:        now.Add(-24 * time.Hour),
						TenantID:       "tenant-123",
					},
				},
				Participants: []ConversationParticipant{
					{
						ConversationID: "conv-42",
						Name:           &participantName1,
						Address:        &participantAddress1,
						TenantID:       "tenant-123",
					},
					{
						ConversationID: "conv-42",
						Name:           &participantName2,
						Address:        &participantAddress2,
						TenantID:       "tenant-123",
					},
				},
			}, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ShowConversationRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-42",
	}

	resp, err := svc.ShowConversation(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify conversation metadata
	assert.Equal(t, "conv-42", resp.Id)
	assert.Equal(t, "tenant-123", resp.TenantId)
	assert.Equal(t, "Feature Implementation Discussion", resp.Topic)
	assert.NotNil(t, resp.ThreadKey)
	assert.Equal(t, threadKey, *resp.ThreadKey)
	assert.Equal(t, int32(2), resp.ParticipantCount)
	assert.Equal(t, int32(3), resp.ItemCount)
	assert.NotNil(t, resp.FirstSeen)
	assert.NotNil(t, resp.LastSeen)
	assert.NotNil(t, resp.CreatedAt)
	assert.NotNil(t, resp.UpdatedAt)

	// Verify items are ordered by added_at
	require.Len(t, resp.Items, 3)
	item1 := resp.Items[0]
	assert.Equal(t, "conv-42", item1.ConversationId)
	assert.Equal(t, "content-001", item1.ContentId)
	assert.NotNil(t, item1.SourceId)
	assert.Equal(t, int64(100), *item1.SourceId)
	assert.NotNil(t, item1.AddedAt)

	item2 := resp.Items[1]
	assert.Equal(t, "content-002", item2.ContentId)
	assert.NotNil(t, item2.SourceId)
	assert.Equal(t, int64(101), *item2.SourceId)

	item3 := resp.Items[2]
	assert.Equal(t, "content-003", item3.ContentId)
	assert.Nil(t, item3.SourceId, "Item with no source should have nil SourceId")

	// Verify participants
	require.Len(t, resp.Participants, 2)
	part1 := resp.Participants[0]
	assert.Equal(t, "conv-42", part1.ConversationId)
	assert.NotNil(t, part1.Name)
	assert.Equal(t, participantName1, *part1.Name)
	assert.NotNil(t, part1.Address)
	assert.Equal(t, participantAddress1, *part1.Address)

	part2 := resp.Participants[1]
	assert.NotNil(t, part2.Name)
	assert.Equal(t, participantName2, *part2.Name)
	assert.NotNil(t, part2.Address)
	assert.Equal(t, participantAddress2, *part2.Address)
}

func TestShowConversation_NotFound(t *testing.T) {
	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return nil, nil // Conversation not found
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ShowConversationRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-999",
	}

	resp, err := svc.ShowConversation(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "conversation not found")
}

func TestShowConversation_MissingTenantID(t *testing.T) {
	mockRepo := &MockRepository{}
	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ShowConversationRequest{
		TenantId:       "", // Missing tenant ID
		ConversationId: "conv-42",
	}

	resp, err := svc.ShowConversation(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "tenant_id")
}

func TestShowConversation_MissingConversationID(t *testing.T) {
	mockRepo := &MockRepository{}
	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ShowConversationRequest{
		TenantId:       "tenant-123",
		ConversationId: "", // Missing conversation ID
	}

	resp, err := svc.ShowConversation(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok, "Error should be a gRPC status")
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "conversation_id")
}

func TestShowConversation_RepositoryError(t *testing.T) {
	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return nil, errors.New("database connection failed")
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ShowConversationRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-42",
	}

	resp, err := svc.ShowConversation(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "failed to get conversation")
}

// ============ Summary and State Fields Tests ============

func TestShowConversation_SummaryFields_Populated(t *testing.T) {
	now := time.Now()
	threadKey := "thread-xyz-789"
	stateSummary := "Discussion about feature implementation with key decisions on architecture"
	stateReason := "Manually marked active after review"

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			assert.Equal(t, "tenant-123", tenantID)
			assert.Equal(t, "conv-42", conversationID)

			return &ConversationDetail{
				ID:               "conv-42",
				TenantID:         "tenant-123",
				Topic:            "Feature Implementation Discussion",
				ThreadKey:        &threadKey,
				FirstSeen:        timePtr(now.Add(-72 * time.Hour)),
				LastSeen:         timePtr(now.Add(-3 * time.Hour)),
				ParticipantCount: 2,
				ItemCount:        3,
				StateSummary:     &stateSummary,
				SummaryVersion:   2,
				SummaryUpdatedAt: timePtr(now.Add(-1 * time.Hour)),
				State:            "active",
				StateReason:      &stateReason,
				StateChangedAt:   timePtr(now.Add(-2 * time.Hour)),
				CreatedAt:        now.Add(-72 * time.Hour),
				UpdatedAt:        now.Add(-3 * time.Hour),
				Items:            []ConversationItem{},
				Participants:     []ConversationParticipant{},
			}, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ShowConversationRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-42",
	}

	resp, err := svc.ShowConversation(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify summary fields are populated
	assert.NotNil(t, resp.StateSummary, "StateSummary should be populated")
	assert.Equal(t, stateSummary, *resp.StateSummary)
	assert.Equal(t, int32(2), resp.SummaryVersion)
	assert.NotNil(t, resp.SummaryUpdatedAt, "SummaryUpdatedAt should be populated")

	// Verify state fields are populated
	assert.Equal(t, "active", resp.GetState())
	assert.NotNil(t, resp.StateReason, "StateReason should be populated")
	assert.Equal(t, stateReason, *resp.StateReason)
	assert.NotNil(t, resp.StateChangedAt, "StateChangedAt should be populated")
}

func TestShowConversation_SummaryFields_Nil(t *testing.T) {
	now := time.Now()
	threadKey := "thread-abc-123"

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID:               "conv-99",
				TenantID:         "tenant-123",
				Topic:            "New Discussion",
				ThreadKey:        &threadKey,
				FirstSeen:        timePtr(now.Add(-24 * time.Hour)),
				LastSeen:         timePtr(now.Add(-1 * time.Hour)),
				ParticipantCount: 1,
				ItemCount:        2,
				StateSummary:     nil, // No summary yet
				SummaryVersion:   0,
				SummaryUpdatedAt: nil,
				State:            "pending", // State can exist without summary
				StateReason:      nil,       // No reason provided
				StateChangedAt:   nil,
				CreatedAt:        now.Add(-24 * time.Hour),
				UpdatedAt:        now.Add(-1 * time.Hour),
				Items:            []ConversationItem{},
				Participants:     []ConversationParticipant{},
			}, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ShowConversationRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-99",
	}

	resp, err := svc.ShowConversation(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify nil summary fields are handled correctly
	assert.Nil(t, resp.StateSummary, "StateSummary should be nil when not populated")
	assert.Equal(t, int32(0), resp.SummaryVersion)
	assert.Nil(t, resp.SummaryUpdatedAt, "SummaryUpdatedAt should be nil when not populated")

	// Verify state field exists but optional fields are nil
	assert.Equal(t, "pending", resp.GetState())
	assert.Nil(t, resp.StateReason, "StateReason should be nil when not populated")
	assert.Nil(t, resp.StateChangedAt, "StateChangedAt should be nil when not populated")
}

func TestListConversations_StateField(t *testing.T) {
	now := time.Now()
	threadKey1 := "thread-abc-123"
	threadKey2 := "thread-def-456"

	mockRepo := &MockRepository{
		ListConversationsFunc: func(ctx context.Context, tenantID string, limit, offset int32) ([]ConversationSummary, int64, error) {
			return []ConversationSummary{
				{
					ID:               "conv-1",
					TenantID:         "tenant-123",
					Topic:            "Active Discussion",
					ThreadKey:        &threadKey1,
					FirstSeen:        timePtr(now.Add(-48 * time.Hour)),
					LastSeen:         timePtr(now.Add(-2 * time.Hour)),
					ParticipantCount: 5,
					ItemCount:        12,
					State:            "active",
					CreatedAt:        now.Add(-48 * time.Hour),
					UpdatedAt:        now.Add(-2 * time.Hour),
				},
				{
					ID:               "conv-2",
					TenantID:         "tenant-123",
					Topic:            "Archived Discussion",
					ThreadKey:        &threadKey2,
					FirstSeen:        timePtr(now.Add(-168 * time.Hour)),
					LastSeen:         timePtr(now.Add(-72 * time.Hour)),
					ParticipantCount: 3,
					ItemCount:        8,
					State:            "archived",
					CreatedAt:        now.Add(-168 * time.Hour),
					UpdatedAt:        now.Add(-72 * time.Hour),
				},
			}, 2, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ListConversationsRequest{
		TenantId: "tenant-123",
		Limit:    50,
		Offset:   0,
	}

	resp, err := svc.ListConversations(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Conversations, 2)

	// Verify state field is populated in list response
	conv1 := resp.Conversations[0]
	assert.Equal(t, "active", conv1.GetState(), "State field should be populated in list response")

	conv2 := resp.Conversations[1]
	assert.Equal(t, "archived", conv2.GetState(), "State field should be populated in list response")
}

func TestListConversations_StateFilter_Active(t *testing.T) {
	now := time.Now()
	threadKey := "thread-abc-123"

	mockRepo := &MockRepository{
		ListConversationsByStateFunc: func(ctx context.Context, tenantID, state string, limit, offset int32) ([]ConversationSummary, int64, error) {
			assert.Equal(t, "tenant-123", tenantID)
			assert.Equal(t, "active", state, "Repository should receive state filter")
			assert.Equal(t, int32(50), limit)
			assert.Equal(t, int32(0), offset)

			return []ConversationSummary{
				{
					ID:               "conv-1",
					TenantID:         "tenant-123",
					Topic:            "Active Discussion",
					ThreadKey:        &threadKey,
					FirstSeen:        timePtr(now.Add(-48 * time.Hour)),
					LastSeen:         timePtr(now.Add(-2 * time.Hour)),
					ParticipantCount: 5,
					ItemCount:        12,
					State:            "active",
					CreatedAt:        now.Add(-48 * time.Hour),
					UpdatedAt:        now.Add(-2 * time.Hour),
				},
			}, 1, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ListConversationsRequest{
		TenantId: "tenant-123",
		Limit:    50,
		Offset:   0,
		State:    stringPtr("active"), // Filter by state
	}

	resp, err := svc.ListConversations(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(1), resp.TotalCount)
	assert.Len(t, resp.Conversations, 1)

	conv1 := resp.Conversations[0]
	assert.Equal(t, "active", conv1.GetState())
	assert.Equal(t, "Active Discussion", conv1.Topic)
}

func TestListConversations_StateFilter_NoMatch(t *testing.T) {
	mockRepo := &MockRepository{
		ListConversationsByStateFunc: func(ctx context.Context, tenantID, state string, limit, offset int32) ([]ConversationSummary, int64, error) {
			assert.Equal(t, "archived", state)
			return []ConversationSummary{}, 0, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &conversationv1.ListConversationsRequest{
		TenantId: "tenant-123",
		Limit:    50,
		Offset:   0,
		State:    stringPtr("archived"),
	}

	resp, err := svc.ListConversations(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(0), resp.TotalCount)
	assert.Empty(t, resp.Conversations)
}

// ============ Interface Compliance Test ============

func TestServiceImplementsInterface(t *testing.T) {
	// This will fail to compile until the proto is generated and service is implemented
	var _ conversationv1.ConversationServiceServer = (*Service)(nil)
}
