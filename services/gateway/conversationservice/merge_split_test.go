package conversationservice

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	conversationv1 "github.com/otherjamesbrown/penfold/api/proto/conversation/v1"
)

// ============ Mock Transaction ============

// mockTx implements pgx.Tx with no-ops. The service layer never calls tx methods
// directly — it passes the tx to repository methods which are mocked.
type mockTx struct{}

func (m *mockTx) Begin(ctx context.Context) (pgx.Tx, error)                    { return m, nil }
func (m *mockTx) Commit(ctx context.Context) error                             { return nil }
func (m *mockTx) Rollback(ctx context.Context) error                           { return nil }
func (m *mockTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (m *mockTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults { return nil }
func (m *mockTx) LargeObjects() pgx.LargeObjects                              { return pgx.LargeObjects{} }
func (m *mockTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (m *mockTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("DELETE 1"), nil
}
func (m *mockTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}
func (m *mockTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row { return nil }
func (m *mockTx) Conn() *pgx.Conn                                              { return nil }

// ============ MergeConversations Tests ============

func TestMergeConversations_Basic(t *testing.T) {
	now := time.Now()

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			switch conversationID {
			case "conv-source":
				return &ConversationDetail{
					ID:       "conv-source",
					TenantID: "tenant-123",
					Topic:    "Source Topic",
					Items: []ConversationItem{
						{ConversationID: "conv-source", ContentID: "em-1", AddedAt: now},
						{ConversationID: "conv-source", ContentID: "em-2", AddedAt: now},
						{ConversationID: "conv-source", ContentID: "em-3", AddedAt: now},
					},
					CreatedAt: now,
					UpdatedAt: now,
				}, nil
			case "conv-target":
				return &ConversationDetail{
					ID:       "conv-target",
					TenantID: "tenant-123",
					Topic:    "Target Topic",
					Items: []ConversationItem{
						{ConversationID: "conv-target", ContentID: "em-4", AddedAt: now},
					},
					CreatedAt: now,
					UpdatedAt: now,
				}, nil
			}
			return nil, nil
		},
		BeginTxFunc: func(ctx context.Context) (pgx.Tx, error) {
			return &mockTx{}, nil
		},
		MoveItemsFunc: func(ctx context.Context, tx pgx.Tx, sourceConvID, targetConvID string) (int32, int32, error) {
			assert.Equal(t, "conv-source", sourceConvID)
			assert.Equal(t, "conv-target", targetConvID)
			return 3, 0, nil
		},
	}

	svc := NewService(mockRepo, testLogger())
	resp, err := svc.MergeConversations(context.Background(), &conversationv1.MergeConversationsRequest{
		TenantId:             "tenant-123",
		SourceConversationId: "conv-source",
		TargetConversationId: "conv-target",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "conv-target", resp.ConversationId)
	assert.Equal(t, int32(3), resp.ItemsMoved)
	assert.Equal(t, int32(0), resp.DuplicatesSkipped)
}

func TestMergeConversations_DuplicateItems(t *testing.T) {
	now := time.Now()

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID: conversationID, TenantID: "tenant-123",
				Items:     []ConversationItem{{ContentID: "em-1"}},
				CreatedAt: now, UpdatedAt: now,
			}, nil
		},
		BeginTxFunc: func(ctx context.Context) (pgx.Tx, error) {
			return &mockTx{}, nil
		},
		MoveItemsFunc: func(ctx context.Context, tx pgx.Tx, sourceConvID, targetConvID string) (int32, int32, error) {
			return 2, 1, nil // 2 moved, 1 duplicate skipped
		},
	}

	svc := NewService(mockRepo, testLogger())
	resp, err := svc.MergeConversations(context.Background(), &conversationv1.MergeConversationsRequest{
		TenantId:             "tenant-123",
		SourceConversationId: "conv-source",
		TargetConversationId: "conv-target",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.ItemsMoved)
	assert.Equal(t, int32(1), resp.DuplicatesSkipped)
}

func TestMergeConversations_SourceNotFound(t *testing.T) {
	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			if conversationID == "conv-source" {
				return nil, nil // not found
			}
			return &ConversationDetail{ID: conversationID, TenantID: "tenant-123"}, nil
		},
	}

	svc := NewService(mockRepo, testLogger())
	resp, err := svc.MergeConversations(context.Background(), &conversationv1.MergeConversationsRequest{
		TenantId:             "tenant-123",
		SourceConversationId: "conv-source",
		TargetConversationId: "conv-target",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "source")
}

func TestMergeConversations_TargetNotFound(t *testing.T) {
	now := time.Now()
	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			if conversationID == "conv-source" {
				return &ConversationDetail{ID: "conv-source", TenantID: "tenant-123", CreatedAt: now, UpdatedAt: now}, nil
			}
			return nil, nil // target not found
		},
	}

	svc := NewService(mockRepo, testLogger())
	resp, err := svc.MergeConversations(context.Background(), &conversationv1.MergeConversationsRequest{
		TenantId:             "tenant-123",
		SourceConversationId: "conv-source",
		TargetConversationId: "conv-target",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "target")
}

func TestMergeConversations_SameConversation(t *testing.T) {
	mockRepo := &MockRepository{}
	svc := NewService(mockRepo, testLogger())

	resp, err := svc.MergeConversations(context.Background(), &conversationv1.MergeConversationsRequest{
		TenantId:             "tenant-123",
		SourceConversationId: "conv-same",
		TargetConversationId: "conv-same",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestMergeConversations_MissingFields(t *testing.T) {
	svc := NewService(&MockRepository{}, testLogger())

	tests := []struct {
		name string
		req  *conversationv1.MergeConversationsRequest
		want string
	}{
		{"missing tenant_id", &conversationv1.MergeConversationsRequest{SourceConversationId: "a", TargetConversationId: "b"}, "tenant_id"},
		{"missing source", &conversationv1.MergeConversationsRequest{TenantId: "t", TargetConversationId: "b"}, "source_conversation_id"},
		{"missing target", &conversationv1.MergeConversationsRequest{TenantId: "t", SourceConversationId: "a"}, "target_conversation_id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := svc.MergeConversations(context.Background(), tc.req)
			assert.Nil(t, resp)
			st, _ := status.FromError(err)
			assert.Equal(t, codes.InvalidArgument, st.Code())
			assert.Contains(t, st.Message(), tc.want)
		})
	}
}

// ============ SplitConversation Tests ============

func TestSplitConversation_Basic(t *testing.T) {
	now := time.Now()

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID:       "conv-1",
				TenantID: "tenant-123",
				Topic:    "Original Topic",
				Items: []ConversationItem{
					{ConversationID: "conv-1", ContentID: "em-1", AddedAt: now},
					{ConversationID: "conv-1", ContentID: "em-2", AddedAt: now},
					{ConversationID: "conv-1", ContentID: "em-3", AddedAt: now},
					{ConversationID: "conv-1", ContentID: "em-4", AddedAt: now},
					{ConversationID: "conv-1", ContentID: "em-5", AddedAt: now},
				},
				CreatedAt: now,
				UpdatedAt: now,
			}, nil
		},
		BeginTxFunc: func(ctx context.Context) (pgx.Tx, error) {
			return &mockTx{}, nil
		},
		MoveSpecificItemsFunc: func(ctx context.Context, tx pgx.Tx, sourceConvID, targetConvID string, contentIDs []string) (int32, error) {
			assert.Equal(t, "conv-1", sourceConvID)
			assert.Len(t, contentIDs, 2)
			return 2, nil
		},
	}

	svc := NewService(mockRepo, testLogger())
	topic := "GPU Procurement"
	resp, err := svc.SplitConversation(context.Background(), &conversationv1.SplitConversationRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-1",
		ContentIds:     []string{"em-1", "em-2"},
		NewTopic:       &topic,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.NewConversationId)
	assert.Equal(t, int32(2), resp.ItemsMoved)
	assert.Equal(t, "GPU Procurement", resp.Topic)
}

func TestSplitConversation_TopicGenerated(t *testing.T) {
	now := time.Now()

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID: "conv-1", TenantID: "tenant-123", Topic: "Original Discussion",
				Items: []ConversationItem{
					{ContentID: "em-1"}, {ContentID: "em-2"}, {ContentID: "em-3"},
				},
				CreatedAt: now, UpdatedAt: now,
			}, nil
		},
		BeginTxFunc: func(ctx context.Context) (pgx.Tx, error) { return &mockTx{}, nil },
		MoveSpecificItemsFunc: func(ctx context.Context, tx pgx.Tx, sourceConvID, targetConvID string, contentIDs []string) (int32, error) {
			return 1, nil
		},
	}

	svc := NewService(mockRepo, testLogger())
	resp, err := svc.SplitConversation(context.Background(), &conversationv1.SplitConversationRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-1",
		ContentIds:     []string{"em-1"},
		// No NewTopic — should auto-generate
	})

	require.NoError(t, err)
	assert.Contains(t, resp.Topic, "Split from: Original Discussion")
}

func TestSplitConversation_AllItems(t *testing.T) {
	now := time.Now()

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID: "conv-1", TenantID: "tenant-123",
				Items: []ConversationItem{
					{ContentID: "em-1"}, {ContentID: "em-2"},
				},
				CreatedAt: now, UpdatedAt: now,
			}, nil
		},
	}

	svc := NewService(mockRepo, testLogger())
	resp, err := svc.SplitConversation(context.Background(), &conversationv1.SplitConversationRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-1",
		ContentIds:     []string{"em-1", "em-2"}, // all items
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "cannot extract all items")
}

func TestSplitConversation_InvalidItem(t *testing.T) {
	now := time.Now()

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID: "conv-1", TenantID: "tenant-123",
				Items: []ConversationItem{
					{ContentID: "em-1"}, {ContentID: "em-2"}, {ContentID: "em-3"},
				},
				CreatedAt: now, UpdatedAt: now,
			}, nil
		},
	}

	svc := NewService(mockRepo, testLogger())
	resp, err := svc.SplitConversation(context.Background(), &conversationv1.SplitConversationRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-1",
		ContentIds:     []string{"em-1", "em-nonexistent"},
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "em-nonexistent")
}

func TestSplitConversation_NotFound(t *testing.T) {
	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return nil, nil
		},
	}

	svc := NewService(mockRepo, testLogger())
	resp, err := svc.SplitConversation(context.Background(), &conversationv1.SplitConversationRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-missing",
		ContentIds:     []string{"em-1"},
	})

	assert.Nil(t, resp)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.NotFound, st.Code())
}

// ============ UnlinkItem Tests ============

func TestUnlinkItem_Basic(t *testing.T) {
	now := time.Now()
	removedItem := ""

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID: "conv-1", TenantID: "tenant-123",
				Items: []ConversationItem{
					{ContentID: "em-1"}, {ContentID: "em-2"}, {ContentID: "em-3"},
				},
				CreatedAt: now, UpdatedAt: now,
			}, nil
		},
		RemoveItemFunc: func(ctx context.Context, conversationID, contentID string) error {
			removedItem = contentID
			return nil
		},
		GetItemCountFunc: func(ctx context.Context, conversationID string) (int32, error) {
			return 2, nil
		},
	}

	svc := NewService(mockRepo, testLogger())
	resp, err := svc.UnlinkItem(context.Background(), &conversationv1.UnlinkItemRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-1",
		ContentId:      "em-2",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(2), resp.RemainingItems)
	assert.Equal(t, "em-2", removedItem)
}

func TestUnlinkItem_LastItem(t *testing.T) {
	now := time.Now()

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID: "conv-1", TenantID: "tenant-123",
				Items:     []ConversationItem{{ContentID: "em-1"}},
				CreatedAt: now, UpdatedAt: now,
			}, nil
		},
	}

	svc := NewService(mockRepo, testLogger())
	resp, err := svc.UnlinkItem(context.Background(), &conversationv1.UnlinkItemRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-1",
		ContentId:      "em-1",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
	assert.Contains(t, st.Message(), "last item")
}

func TestUnlinkItem_NotFound(t *testing.T) {
	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return nil, nil
		},
	}

	svc := NewService(mockRepo, testLogger())
	resp, err := svc.UnlinkItem(context.Background(), &conversationv1.UnlinkItemRequest{
		TenantId:       "tenant-123",
		ConversationId: "conv-missing",
		ContentId:      "em-1",
	})

	assert.Nil(t, resp)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestUnlinkItem_MissingFields(t *testing.T) {
	svc := NewService(&MockRepository{}, testLogger())

	tests := []struct {
		name string
		req  *conversationv1.UnlinkItemRequest
		want string
	}{
		{"missing tenant_id", &conversationv1.UnlinkItemRequest{ConversationId: "c", ContentId: "e"}, "tenant_id"},
		{"missing conversation_id", &conversationv1.UnlinkItemRequest{TenantId: "t", ContentId: "e"}, "conversation_id"},
		{"missing content_id", &conversationv1.UnlinkItemRequest{TenantId: "t", ConversationId: "c"}, "content_id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := svc.UnlinkItem(context.Background(), tc.req)
			assert.Nil(t, resp)
			st, _ := status.FromError(err)
			assert.Equal(t, codes.InvalidArgument, st.Code())
			assert.Contains(t, st.Message(), tc.want)
		})
	}
}

// ============ ListConversations MinItems Filter Tests ============

func TestListConversations_MinItemsFilter(t *testing.T) {
	now := time.Now()

	mockRepo := &MockRepository{
		ListConversationsFilteredFunc: func(ctx context.Context, tenantID string, state *string, minItems *int32, limit, offset int32) ([]ConversationSummary, int64, error) {
			assert.Equal(t, "tenant-123", tenantID)
			require.NotNil(t, minItems)
			assert.Equal(t, int32(3), *minItems)
			assert.Nil(t, state)

			return []ConversationSummary{
				{
					ID: "conv-1", TenantID: "tenant-123", Topic: "Big Thread",
					ItemCount: 5, State: "active",
					CreatedAt: now, UpdatedAt: now,
				},
			}, 1, nil
		},
	}

	svc := NewService(mockRepo, testLogger())
	minItems := int32(3)
	resp, err := svc.ListConversations(context.Background(), &conversationv1.ListConversationsRequest{
		TenantId: "tenant-123",
		Limit:    50,
		MinItems: &minItems,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.TotalCount)
	assert.Len(t, resp.Conversations, 1)
	assert.Equal(t, int32(5), resp.Conversations[0].ItemCount)
}

func TestListConversations_MinItemsWithStateFilter(t *testing.T) {
	now := time.Now()

	mockRepo := &MockRepository{
		ListConversationsFilteredFunc: func(ctx context.Context, tenantID string, state *string, minItems *int32, limit, offset int32) ([]ConversationSummary, int64, error) {
			require.NotNil(t, state)
			assert.Equal(t, "active", *state)
			require.NotNil(t, minItems)
			assert.Equal(t, int32(2), *minItems)

			return []ConversationSummary{
				{
					ID: "conv-1", TenantID: "tenant-123", Topic: "Active Big Thread",
					ItemCount: 4, State: "active",
					CreatedAt: now, UpdatedAt: now,
				},
			}, 1, nil
		},
	}

	svc := NewService(mockRepo, testLogger())
	minItems := int32(2)
	stateFilter := "active"
	resp, err := svc.ListConversations(context.Background(), &conversationv1.ListConversationsRequest{
		TenantId: "tenant-123",
		Limit:    50,
		State:    &stateFilter,
		MinItems: &minItems,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.TotalCount)
	assert.Len(t, resp.Conversations, 1)
}
