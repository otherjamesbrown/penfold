package relationshipservice

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	"github.com/otherjamesbrown/penfold/pkg/relationships"
)

// ============ Mock Repository Extension for Deduplication ============

// mockDedupRepo extends the mock repository with deduplication methods
type mockDedupRepo struct {
	mockRelationshipRepo
	findDuplicateEntitiesFn func(ctx context.Context, tenantID string, minSimilarity float64) ([]relationships.DuplicatePair, error)
	getMergePreviewFn       func(ctx context.Context, tenantID string, entityID1, entityID2 string) (*relationships.MergePreview, error)
	autoMergeDuplicatesFn   func(ctx context.Context, tenantID string, minSimilarity float64, dryRun bool) (*relationships.AutoMergeResult, error)
}

func (m *mockDedupRepo) FindDuplicateEntities(ctx context.Context, tenantID string, minSimilarity float64) ([]relationships.DuplicatePair, error) {
	if m.findDuplicateEntitiesFn != nil {
		return m.findDuplicateEntitiesFn(ctx, tenantID, minSimilarity)
	}
	// Default: return some duplicate pairs
	return []relationships.DuplicatePair{
		{
			EntityID1:  "ent-person-1",
			EntityID2:  "ent-person-2",
			Similarity: 0.95,
			Signals:    []string{"name_match", "domain_match"},
		},
	}, nil
}

func (m *mockDedupRepo) GetMergePreview(ctx context.Context, tenantID string, entityID1, entityID2 string) (*relationships.MergePreview, error) {
	if m.getMergePreviewFn != nil {
		return m.getMergePreviewFn(ctx, tenantID, entityID1, entityID2)
	}
	// Default: return a merge preview
	return &relationships.MergePreview{
		MergedEntity: &relationships.Entity{
			ID:            entityID1,
			Name:          "John Doe",
			Type:          relationships.EntityTypePerson,
			SourceCount:   3,
			RelationCount: 5,
		},
		TransferringAliases:       []string{"jdoe@example.com", "john@example.com"},
		TransferringRelationships: []relationships.Relationship{},
		ConflictFields:            []string{},
	}, nil
}

func (m *mockDedupRepo) AutoMergeDuplicates(ctx context.Context, tenantID string, minSimilarity float64, dryRun bool) (*relationships.AutoMergeResult, error) {
	if m.autoMergeDuplicatesFn != nil {
		return m.autoMergeDuplicatesFn(ctx, tenantID, minSimilarity, dryRun)
	}
	// Default: return merge results
	return &relationships.AutoMergeResult{
		MergedCount: 2,
		MergedPairs: []relationships.DuplicatePair{
			{
				EntityID1:  "ent-person-1",
				EntityID2:  "ent-person-2",
				Similarity: 0.95,
				Signals:    []string{"name_match", "domain_match"},
			},
			{
				EntityID1:  "ent-person-3",
				EntityID2:  "ent-person-4",
				Similarity: 0.92,
				Signals:    []string{"name_match"},
			},
		},
		SkippedPairs: []relationships.DuplicatePair{},
	}, nil
}

func newTestDedupService(repo *mockDedupRepo) *Service {
	return NewService(repo, testLogger())
}

// float32Ptr is a helper to create *float32 from literals
func float32Ptr(f float32) *float32 {
	return &f
}

// ============ FindDuplicates Tests ============

func TestFindDuplicates_Success(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.FindDuplicatesRequest{
		TenantId:      "tenant-1",
		MinSimilarity: float32Ptr(0.85),
	}

	// Execute
	resp, err := svc.FindDuplicates(context.Background(), req)

	// Verify
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.DuplicatePairs, 1)
	assert.Equal(t, "ent-person-1", resp.DuplicatePairs[0].EntityId1)
	assert.Equal(t, "ent-person-2", resp.DuplicatePairs[0].EntityId2)
	assert.Equal(t, float32(0.95), resp.DuplicatePairs[0].Similarity)
	assert.Contains(t, resp.DuplicatePairs[0].Signals, "name_match")
	assert.Contains(t, resp.DuplicatePairs[0].Signals, "domain_match")
}

func TestFindDuplicates_MissingTenantID_Error(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.FindDuplicatesRequest{
		MinSimilarity: float32Ptr(0.85),
	}

	// Execute
	resp, err := svc.FindDuplicates(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "tenant_id")
}

func TestFindDuplicates_MinSimilarityTooLow_Error(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.FindDuplicatesRequest{
		TenantId:      "tenant-1",
		MinSimilarity: float32Ptr(0.4), // Below minimum of 0.5
	}

	// Execute
	resp, err := svc.FindDuplicates(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "min_similarity")
}

func TestFindDuplicates_MinSimilarityTooHigh_Error(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.FindDuplicatesRequest{
		TenantId:      "tenant-1",
		MinSimilarity: float32Ptr(1.5), // Above maximum of 1.0
	}

	// Execute
	resp, err := svc.FindDuplicates(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "min_similarity")
}

func TestFindDuplicates_DefaultMinSimilarity(t *testing.T) {
	// Setup
	var capturedMinSimilarity float64
	mockRepo := &mockDedupRepo{
		findDuplicateEntitiesFn: func(ctx context.Context, tenantID string, minSimilarity float64) ([]relationships.DuplicatePair, error) {
			capturedMinSimilarity = minSimilarity
			return []relationships.DuplicatePair{}, nil
		},
	}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.FindDuplicatesRequest{
		TenantId: "tenant-1",
		// MinSimilarity not set - should default to 0.85
	}

	// Execute
	_, err := svc.FindDuplicates(context.Background(), req)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 0.85, capturedMinSimilarity, "Should use default min_similarity of 0.85")
}

func TestFindDuplicates_RepositoryError_InternalError(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{
		findDuplicateEntitiesFn: func(ctx context.Context, tenantID string, minSimilarity float64) ([]relationships.DuplicatePair, error) {
			return nil, errors.New("database connection lost")
		},
	}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.FindDuplicatesRequest{
		TenantId:      "tenant-1",
		MinSimilarity: float32Ptr(0.85),
	}

	// Execute
	resp, err := svc.FindDuplicates(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

// ============ MergePreview Tests ============

func TestMergePreview_Success(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.MergePreviewRequest{
		TenantId:  "tenant-1",
		EntityId1: "ent-person-1",
		EntityId2: "ent-person-2",
	}

	// Execute
	resp, err := svc.MergePreview(context.Background(), req)

	// Verify
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.MergedEntity)
	assert.Equal(t, "ent-person-1", resp.MergedEntity.Id)
	assert.Equal(t, "John Doe", resp.MergedEntity.Name)
	assert.Len(t, resp.TransferringAliases, 2)
	assert.Contains(t, resp.TransferringAliases, "jdoe@example.com")
	assert.Contains(t, resp.TransferringAliases, "john@example.com")
	assert.Len(t, resp.ConflictFields, 0)
}

func TestMergePreview_MissingTenantID_Error(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.MergePreviewRequest{
		EntityId1: "ent-person-1",
		EntityId2: "ent-person-2",
	}

	// Execute
	resp, err := svc.MergePreview(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "tenant_id")
}

func TestMergePreview_MissingEntityID1_Error(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.MergePreviewRequest{
		TenantId:  "tenant-1",
		EntityId2: "ent-person-2",
	}

	// Execute
	resp, err := svc.MergePreview(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "entity_id_1")
}

func TestMergePreview_MissingEntityID2_Error(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.MergePreviewRequest{
		TenantId:  "tenant-1",
		EntityId1: "ent-person-1",
	}

	// Execute
	resp, err := svc.MergePreview(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "entity_id_2")
}

func TestMergePreview_WithConflicts(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{
		getMergePreviewFn: func(ctx context.Context, tenantID string, entityID1, entityID2 string) (*relationships.MergePreview, error) {
			return &relationships.MergePreview{
				MergedEntity: &relationships.Entity{
					ID:   entityID1,
					Name: "John Doe",
					Type: relationships.EntityTypePerson,
				},
				TransferringAliases:       []string{"jdoe@example.com"},
				TransferringRelationships: []relationships.Relationship{},
				ConflictFields:            []string{"email", "title"},
			}, nil
		},
	}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.MergePreviewRequest{
		TenantId:  "tenant-1",
		EntityId1: "ent-person-1",
		EntityId2: "ent-person-2",
	}

	// Execute
	resp, err := svc.MergePreview(context.Background(), req)

	// Verify
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.ConflictFields, 2)
	assert.Contains(t, resp.ConflictFields, "email")
	assert.Contains(t, resp.ConflictFields, "title")
}

func TestMergePreview_RepositoryError_InternalError(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{
		getMergePreviewFn: func(ctx context.Context, tenantID string, entityID1, entityID2 string) (*relationships.MergePreview, error) {
			return nil, errors.New("database connection lost")
		},
	}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.MergePreviewRequest{
		TenantId:  "tenant-1",
		EntityId1: "ent-person-1",
		EntityId2: "ent-person-2",
	}

	// Execute
	resp, err := svc.MergePreview(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

// ============ AutoMergeDuplicates Tests ============

func TestAutoMergeDuplicates_DryRun_Success(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.AutoMergeDuplicatesRequest{
		TenantId:      "tenant-1",
		MinSimilarity: float32Ptr(0.90),
		DryRun:        true,
	}

	// Execute
	resp, err := svc.AutoMergeDuplicates(context.Background(), req)

	// Verify
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(2), resp.MergedCount)
	assert.Len(t, resp.MergedPairs, 2)
	assert.Len(t, resp.SkippedPairs, 0)
	assert.Equal(t, "ent-person-1", resp.MergedPairs[0].EntityId1)
	assert.Equal(t, "ent-person-2", resp.MergedPairs[0].EntityId2)
	assert.Equal(t, float32(0.95), resp.MergedPairs[0].Similarity)
}

func TestAutoMergeDuplicates_ActualMerge_Success(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.AutoMergeDuplicatesRequest{
		TenantId:      "tenant-1",
		MinSimilarity: float32Ptr(0.90),
		DryRun:        false,
	}

	// Execute
	resp, err := svc.AutoMergeDuplicates(context.Background(), req)

	// Verify
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(2), resp.MergedCount)
	assert.Len(t, resp.MergedPairs, 2)
}

func TestAutoMergeDuplicates_MissingTenantID_Error(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.AutoMergeDuplicatesRequest{
		MinSimilarity: float32Ptr(0.90),
		DryRun:        true,
	}

	// Execute
	resp, err := svc.AutoMergeDuplicates(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "tenant_id")
}

func TestAutoMergeDuplicates_MinSimilarityBelowThreshold_Error(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.AutoMergeDuplicatesRequest{
		TenantId:      "tenant-1",
		MinSimilarity: float32Ptr(0.85), // Below minimum of 0.90 for auto-merge
		DryRun:        true,
	}

	// Execute
	resp, err := svc.AutoMergeDuplicates(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "0.90")
	assert.Contains(t, st.Message(), "min_similarity")
}

func TestAutoMergeDuplicates_MinSimilarityTooHigh_Error(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.AutoMergeDuplicatesRequest{
		TenantId:      "tenant-1",
		MinSimilarity: float32Ptr(1.5), // Above maximum of 1.0
		DryRun:        true,
	}

	// Execute
	resp, err := svc.AutoMergeDuplicates(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "min_similarity")
}

func TestAutoMergeDuplicates_DefaultMinSimilarity(t *testing.T) {
	// Setup
	var capturedMinSimilarity float64
	mockRepo := &mockDedupRepo{
		autoMergeDuplicatesFn: func(ctx context.Context, tenantID string, minSimilarity float64, dryRun bool) (*relationships.AutoMergeResult, error) {
			capturedMinSimilarity = minSimilarity
			return &relationships.AutoMergeResult{
				MergedCount:  0,
				MergedPairs:  []relationships.DuplicatePair{},
				SkippedPairs: []relationships.DuplicatePair{},
			}, nil
		},
	}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.AutoMergeDuplicatesRequest{
		TenantId: "tenant-1",
		DryRun:   true,
		// MinSimilarity not set - should default to 0.95
	}

	// Execute
	_, err := svc.AutoMergeDuplicates(context.Background(), req)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 0.95, capturedMinSimilarity, "Should use default min_similarity of 0.95")
}

func TestAutoMergeDuplicates_WithSkippedPairs(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{
		autoMergeDuplicatesFn: func(ctx context.Context, tenantID string, minSimilarity float64, dryRun bool) (*relationships.AutoMergeResult, error) {
			return &relationships.AutoMergeResult{
				MergedCount: 1,
				MergedPairs: []relationships.DuplicatePair{
					{
						EntityID1:  "ent-person-1",
						EntityID2:  "ent-person-2",
						Similarity: 0.95,
						Signals:    []string{"name_match"},
					},
				},
				SkippedPairs: []relationships.DuplicatePair{
					{
						EntityID1:  "ent-person-3",
						EntityID2:  "ent-person-4",
						Similarity: 0.92,
						Signals:    []string{"name_match"},
					},
				},
			}, nil
		},
	}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.AutoMergeDuplicatesRequest{
		TenantId:      "tenant-1",
		MinSimilarity: float32Ptr(0.90),
		DryRun:        false,
	}

	// Execute
	resp, err := svc.AutoMergeDuplicates(context.Background(), req)

	// Verify
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(1), resp.MergedCount)
	assert.Len(t, resp.MergedPairs, 1)
	assert.Len(t, resp.SkippedPairs, 1)
	assert.Equal(t, "ent-person-3", resp.SkippedPairs[0].Pair.EntityId1)
}

func TestAutoMergeDuplicates_RepositoryError_InternalError(t *testing.T) {
	// Setup
	mockRepo := &mockDedupRepo{
		autoMergeDuplicatesFn: func(ctx context.Context, tenantID string, minSimilarity float64, dryRun bool) (*relationships.AutoMergeResult, error) {
			return nil, errors.New("database connection lost")
		},
	}
	svc := newTestDedupService(mockRepo)

	req := &relationshipv1.AutoMergeDuplicatesRequest{
		TenantId:      "tenant-1",
		MinSimilarity: float32Ptr(0.90),
		DryRun:        true,
	}

	// Execute
	resp, err := svc.AutoMergeDuplicates(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}
