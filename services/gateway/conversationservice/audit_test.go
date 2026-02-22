package conversationservice

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	conversationv1 "github.com/otherjamesbrown/penfold/api/proto/conversation/v1"
)

// ============ Mock Audit Repository ============

type MockAuditRepository struct {
	FindOrphansWithThreadFunc       func(ctx context.Context, tenantID string) ([]AuditOrphan, error)
	FindOrphansWithReplySubjectFunc func(ctx context.Context, tenantID string) ([]AuditOrphan, error)
	FindDuplicateMembershipsFunc    func(ctx context.Context, tenantID string) ([]AuditDuplicate, error)
	FindMergeCandidatesByOverlapFunc func(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error)
	FindMergeCandidatesByTopicFunc  func(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error)
	CountConversationsFunc          func(ctx context.Context, tenantID string) (int32, error)
}

func (m *MockAuditRepository) FindOrphansWithThread(ctx context.Context, tenantID string) ([]AuditOrphan, error) {
	if m.FindOrphansWithThreadFunc != nil {
		return m.FindOrphansWithThreadFunc(ctx, tenantID)
	}
	return nil, nil
}

func (m *MockAuditRepository) FindOrphansWithReplySubject(ctx context.Context, tenantID string) ([]AuditOrphan, error) {
	if m.FindOrphansWithReplySubjectFunc != nil {
		return m.FindOrphansWithReplySubjectFunc(ctx, tenantID)
	}
	return nil, nil
}

func (m *MockAuditRepository) FindDuplicateMemberships(ctx context.Context, tenantID string) ([]AuditDuplicate, error) {
	if m.FindDuplicateMembershipsFunc != nil {
		return m.FindDuplicateMembershipsFunc(ctx, tenantID)
	}
	return nil, nil
}

func (m *MockAuditRepository) FindMergeCandidatesByOverlap(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error) {
	if m.FindMergeCandidatesByOverlapFunc != nil {
		return m.FindMergeCandidatesByOverlapFunc(ctx, tenantID)
	}
	return nil, nil
}

func (m *MockAuditRepository) FindMergeCandidatesByTopic(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error) {
	if m.FindMergeCandidatesByTopicFunc != nil {
		return m.FindMergeCandidatesByTopicFunc(ctx, tenantID)
	}
	return nil, nil
}

func (m *MockAuditRepository) CountConversations(ctx context.Context, tenantID string) (int32, error) {
	if m.CountConversationsFunc != nil {
		return m.CountConversationsFunc(ctx, tenantID)
	}
	return 0, nil
}

// ============ Helper ============

func newAuditService(auditRepo *MockAuditRepository) *Service {
	svc := NewService(&MockRepository{}, testLogger())
	svc.SetAuditRepo(auditRepo)
	return svc
}

// ============ RunConversationAudit Tests ============

func TestRunConversationAudit_MissingTenantID(t *testing.T) {
	svc := newAuditService(&MockAuditRepository{})

	resp, err := svc.RunConversationAudit(context.Background(), &conversationv1.RunConversationAuditRequest{
		TenantId: "",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestRunConversationAudit_NoAuditRepo(t *testing.T) {
	svc := NewService(&MockRepository{}, testLogger())
	// auditRepo not set

	resp, err := svc.RunConversationAudit(context.Background(), &conversationv1.RunConversationAuditRequest{
		TenantId: "tenant-123",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
}

func TestRunConversationAudit_FullAudit_NoFindings(t *testing.T) {
	auditRepo := &MockAuditRepository{
		CountConversationsFunc: func(ctx context.Context, tenantID string) (int32, error) {
			return 42, nil
		},
	}

	svc := newAuditService(auditRepo)

	resp, err := svc.RunConversationAudit(context.Background(), &conversationv1.RunConversationAuditRequest{
		TenantId: "tenant-123",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(42), resp.ConversationsAudited)
	assert.Empty(t, resp.Orphans)
	assert.Empty(t, resp.Flagged)
	assert.Empty(t, resp.MergeCandidates)
}

func TestRunConversationAudit_OrphanDetection_ItemWithThreadButNoConversation(t *testing.T) {
	auditRepo := &MockAuditRepository{
		CountConversationsFunc: func(ctx context.Context, tenantID string) (int32, error) {
			return 10, nil
		},
		FindOrphansWithThreadFunc: func(ctx context.Context, tenantID string) ([]AuditOrphan, error) {
			assert.Equal(t, "tenant-123", tenantID)
			return []AuditOrphan{
				{
					ContentID:               "em-abc123",
					Reason:                  `has thread_id "root-msg-1" but no conversation membership`,
					SuggestedConversationID: "conv-42",
				},
			}, nil
		},
	}

	svc := newAuditService(auditRepo)

	resp, err := svc.RunConversationAudit(context.Background(), &conversationv1.RunConversationAuditRequest{
		TenantId: "tenant-123",
	})

	require.NoError(t, err)
	require.Len(t, resp.Orphans, 1)
	assert.Equal(t, "em-abc123", resp.Orphans[0].ContentId)
	assert.Contains(t, resp.Orphans[0].Reason, "thread_id")
	assert.Equal(t, "conv-42", resp.Orphans[0].SuggestedConversationId)
}

func TestRunConversationAudit_OrphanDetection_ReplySubjectNoThread(t *testing.T) {
	auditRepo := &MockAuditRepository{
		CountConversationsFunc: func(ctx context.Context, tenantID string) (int32, error) {
			return 10, nil
		},
		FindOrphansWithReplySubjectFunc: func(ctx context.Context, tenantID string) ([]AuditOrphan, error) {
			return []AuditOrphan{
				{
					ContentID: "em-def456",
					Reason:    `reply subject "Re: GPU requirements" but no thread_id — parser may have missed References header`,
				},
			}, nil
		},
	}

	svc := newAuditService(auditRepo)

	resp, err := svc.RunConversationAudit(context.Background(), &conversationv1.RunConversationAuditRequest{
		TenantId: "tenant-123",
	})

	require.NoError(t, err)
	require.Len(t, resp.Orphans, 1)
	assert.Equal(t, "em-def456", resp.Orphans[0].ContentId)
	assert.Contains(t, resp.Orphans[0].Reason, "reply subject")
}

func TestRunConversationAudit_DuplicateMembership_ItemInTwoConversations(t *testing.T) {
	auditRepo := &MockAuditRepository{
		CountConversationsFunc: func(ctx context.Context, tenantID string) (int32, error) {
			return 10, nil
		},
		FindDuplicateMembershipsFunc: func(ctx context.Context, tenantID string) ([]AuditDuplicate, error) {
			return []AuditDuplicate{
				{
					ContentID:       "em-dup789",
					ConversationIDs: []string{"conv-1", "conv-2"},
				},
			}, nil
		},
	}

	svc := newAuditService(auditRepo)

	resp, err := svc.RunConversationAudit(context.Background(), &conversationv1.RunConversationAuditRequest{
		TenantId: "tenant-123",
	})

	require.NoError(t, err)
	require.Len(t, resp.Flagged, 1)
	assert.Equal(t, "em-dup789", resp.Flagged[0].ItemId)
	assert.Equal(t, "conv-1", resp.Flagged[0].ConversationId)
	assert.Contains(t, resp.Flagged[0].Reason, "2 conversations")
	assert.Equal(t, "merge", resp.Flagged[0].SuggestedAction)
}

func TestRunConversationAudit_MergeCandidate_OverlappingItems(t *testing.T) {
	auditRepo := &MockAuditRepository{
		CountConversationsFunc: func(ctx context.Context, tenantID string) (int32, error) {
			return 10, nil
		},
		FindMergeCandidatesByOverlapFunc: func(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error) {
			return []AuditMergeCandidate{
				{
					ConversationIDA: "conv-1",
					ConversationIDB: "conv-2",
					TopicA:          "GPU requirements",
					TopicB:          "Re: GPU requirements",
					Reason:          "overlapping items: 2 shared content IDs",
					SharedItems:     []string{"em-shared1", "em-shared2"},
				},
			}, nil
		},
	}

	svc := newAuditService(auditRepo)

	resp, err := svc.RunConversationAudit(context.Background(), &conversationv1.RunConversationAuditRequest{
		TenantId: "tenant-123",
	})

	require.NoError(t, err)
	require.Len(t, resp.MergeCandidates, 1)
	mc := resp.MergeCandidates[0]
	assert.Equal(t, "conv-1", mc.ConversationIdA)
	assert.Equal(t, "conv-2", mc.ConversationIdB)
	assert.Contains(t, mc.Reason, "overlapping")
	assert.Len(t, mc.SharedItems, 2)
}

func TestRunConversationAudit_MergeCandidate_SimilarTopics(t *testing.T) {
	auditRepo := &MockAuditRepository{
		CountConversationsFunc: func(ctx context.Context, tenantID string) (int32, error) {
			return 10, nil
		},
		FindMergeCandidatesByTopicFunc: func(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error) {
			return []AuditMergeCandidate{
				{
					ConversationIDA: "conv-3",
					ConversationIDB: "conv-4",
					TopicA:          "GPU requirements",
					TopicB:          "Re: GPU requirements",
					Reason:          "similar topics after stripping reply/forward prefixes",
				},
			}, nil
		},
	}

	svc := newAuditService(auditRepo)

	resp, err := svc.RunConversationAudit(context.Background(), &conversationv1.RunConversationAuditRequest{
		TenantId: "tenant-123",
	})

	require.NoError(t, err)
	require.Len(t, resp.MergeCandidates, 1)
	mc := resp.MergeCandidates[0]
	assert.Equal(t, "conv-3", mc.ConversationIdA)
	assert.Equal(t, "conv-4", mc.ConversationIdB)
	assert.Contains(t, mc.Reason, "similar topics")
}

func TestRunConversationAudit_OrphansOnly_SkipsDuplicatesAndMerge(t *testing.T) {
	duplicatesCalled := false
	mergeCalled := false

	auditRepo := &MockAuditRepository{
		CountConversationsFunc: func(ctx context.Context, tenantID string) (int32, error) {
			return 5, nil
		},
		FindOrphansWithThreadFunc: func(ctx context.Context, tenantID string) ([]AuditOrphan, error) {
			return []AuditOrphan{{ContentID: "em-1", Reason: "orphan"}}, nil
		},
		FindDuplicateMembershipsFunc: func(ctx context.Context, tenantID string) ([]AuditDuplicate, error) {
			duplicatesCalled = true
			return nil, nil
		},
		FindMergeCandidatesByOverlapFunc: func(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error) {
			mergeCalled = true
			return nil, nil
		},
		FindMergeCandidatesByTopicFunc: func(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error) {
			mergeCalled = true
			return nil, nil
		},
	}

	svc := newAuditService(auditRepo)

	resp, err := svc.RunConversationAudit(context.Background(), &conversationv1.RunConversationAuditRequest{
		TenantId:    "tenant-123",
		OrphansOnly: true,
	})

	require.NoError(t, err)
	assert.Len(t, resp.Orphans, 1)
	assert.False(t, duplicatesCalled, "duplicates should not be called with orphans_only")
	assert.False(t, mergeCalled, "merge should not be called with orphans_only")
}

func TestRunConversationAudit_DuplicatesOnly_SkipsOrphansAndMerge(t *testing.T) {
	orphansCalled := false

	auditRepo := &MockAuditRepository{
		CountConversationsFunc: func(ctx context.Context, tenantID string) (int32, error) {
			return 5, nil
		},
		FindOrphansWithThreadFunc: func(ctx context.Context, tenantID string) ([]AuditOrphan, error) {
			orphansCalled = true
			return nil, nil
		},
		FindDuplicateMembershipsFunc: func(ctx context.Context, tenantID string) ([]AuditDuplicate, error) {
			return []AuditDuplicate{{ContentID: "em-1", ConversationIDs: []string{"c1", "c2"}}}, nil
		},
	}

	svc := newAuditService(auditRepo)

	resp, err := svc.RunConversationAudit(context.Background(), &conversationv1.RunConversationAuditRequest{
		TenantId:       "tenant-123",
		DuplicatesOnly: true,
	})

	require.NoError(t, err)
	assert.Len(t, resp.Flagged, 1)
	assert.False(t, orphansCalled, "orphans should not be called with duplicates_only")
}

func TestRunConversationAudit_CountError(t *testing.T) {
	auditRepo := &MockAuditRepository{
		CountConversationsFunc: func(ctx context.Context, tenantID string) (int32, error) {
			return 0, errors.New("db error")
		},
	}

	svc := newAuditService(auditRepo)

	resp, err := svc.RunConversationAudit(context.Background(), &conversationv1.RunConversationAuditRequest{
		TenantId: "tenant-123",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Internal, st.Code())
}

func TestRunConversationAudit_CombinedFindings(t *testing.T) {
	auditRepo := &MockAuditRepository{
		CountConversationsFunc: func(ctx context.Context, tenantID string) (int32, error) {
			return 20, nil
		},
		FindOrphansWithThreadFunc: func(ctx context.Context, tenantID string) ([]AuditOrphan, error) {
			return []AuditOrphan{
				{ContentID: "em-orphan1", Reason: "thread orphan", SuggestedConversationID: "conv-1"},
			}, nil
		},
		FindOrphansWithReplySubjectFunc: func(ctx context.Context, tenantID string) ([]AuditOrphan, error) {
			return []AuditOrphan{
				{ContentID: "em-orphan2", Reason: "reply orphan"},
			}, nil
		},
		FindDuplicateMembershipsFunc: func(ctx context.Context, tenantID string) ([]AuditDuplicate, error) {
			return []AuditDuplicate{
				{ContentID: "em-dup1", ConversationIDs: []string{"conv-1", "conv-2"}},
			}, nil
		},
		FindMergeCandidatesByOverlapFunc: func(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error) {
			return []AuditMergeCandidate{
				{ConversationIDA: "conv-1", ConversationIDB: "conv-2", TopicA: "A", TopicB: "B", Reason: "overlap", SharedItems: []string{"em-x"}},
			}, nil
		},
		FindMergeCandidatesByTopicFunc: func(ctx context.Context, tenantID string) ([]AuditMergeCandidate, error) {
			return []AuditMergeCandidate{
				{ConversationIDA: "conv-3", ConversationIDB: "conv-4", TopicA: "C", TopicB: "Re: C", Reason: "topic"},
			}, nil
		},
	}

	svc := newAuditService(auditRepo)

	resp, err := svc.RunConversationAudit(context.Background(), &conversationv1.RunConversationAuditRequest{
		TenantId: "tenant-123",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(20), resp.ConversationsAudited)
	assert.Len(t, resp.Orphans, 2, "should combine thread and reply orphans")
	assert.Len(t, resp.Flagged, 1)
	assert.Len(t, resp.MergeCandidates, 2, "should combine overlap and topic candidates")
}
