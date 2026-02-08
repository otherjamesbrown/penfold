package relationshipservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/relationships"
)

// ============ Mock Repository ============

type mockRelationshipRepo struct {
	insertRelationshipFn func(ctx context.Context, req relationships.InsertRelationshipRequest) (*relationships.Relationship, error)
	getEntityFn          func(ctx context.Context, tenantID, entityID string) (*relationships.Entity, error)
}

// Implement the full RelationshipRepository interface with stubs for unused methods
func (m *mockRelationshipRepo) ListRelationships(ctx context.Context, filter relationships.ListRelationshipsFilter) ([]relationships.Relationship, int64, error) {
	return nil, 0, nil
}

func (m *mockRelationshipRepo) GetRelationship(ctx context.Context, tenantID, relationshipID string) (*relationships.Relationship, error) {
	return nil, nil
}

func (m *mockRelationshipRepo) SearchRelationships(ctx context.Context, query relationships.SearchRelationshipsQuery) ([]relationships.Relationship, error) {
	return nil, nil
}

func (m *mockRelationshipRepo) ListEntities(ctx context.Context, filter relationships.ListEntitiesFilter) ([]relationships.Entity, int64, error) {
	return nil, 0, nil
}

func (m *mockRelationshipRepo) MergeEntities(ctx context.Context, req relationships.MergeEntitiesRequest) (*relationships.MergeEntitiesResult, error) {
	return nil, nil
}

func (m *mockRelationshipRepo) GetNetworkStats(ctx context.Context, tenantID string) (*relationships.NetworkStats, error) {
	return nil, nil
}

func (m *mockRelationshipRepo) GetCentralEntities(ctx context.Context, tenantID string, limit int) ([]relationships.Entity, error) {
	return nil, nil
}

func (m *mockRelationshipRepo) GetClusters(ctx context.Context, tenantID string) ([]relationships.NetworkCluster, error) {
	return nil, nil
}

func (m *mockRelationshipRepo) ListConflicts(ctx context.Context, filter relationships.ListConflictsFilter) ([]relationships.RelationshipConflict, int64, error) {
	return nil, 0, nil
}

func (m *mockRelationshipRepo) GetConflict(ctx context.Context, tenantID, conflictID string) (*relationships.RelationshipConflict, error) {
	return nil, nil
}

func (m *mockRelationshipRepo) ResolveConflict(ctx context.Context, req relationships.ResolveConflictRequest) (*relationships.ResolveConflictResult, error) {
	return nil, nil
}

func (m *mockRelationshipRepo) InsertRelationship(ctx context.Context, req relationships.InsertRelationshipRequest) (*relationships.Relationship, error) {
	if m.insertRelationshipFn != nil {
		return m.insertRelationshipFn(ctx, req)
	}
	// Default: return a new relationship with ID set
	now := time.Now()
	return &relationships.Relationship{
		ID:         "rel-123",
		SourceID:   req.FromEntityID,
		SourceName: "Entity A",
		SourceType: relationships.EntityTypePerson,
		TargetID:   req.ToEntityID,
		TargetName: "Entity B",
		TargetType: relationships.EntityTypePerson,
		Type:       req.Type,
		Status:     relationships.RelationshipStatusConfirmed,
		Confidence: req.Confidence,
		TenantID:   req.TenantID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (m *mockRelationshipRepo) GetEntity(ctx context.Context, tenantID, entityID string) (*relationships.Entity, error) {
	if m.getEntityFn != nil {
		return m.getEntityFn(ctx, tenantID, entityID)
	}
	// Default: return a valid entity
	now := time.Now()
	return &relationships.Entity{
		ID:        entityID,
		Name:      "Test Entity",
		Type:      relationships.EntityTypePerson,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// ============ Test Helpers ============

func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = "error" // Quiet logging for tests
	return logging.NewLogger(cfg)
}

func newTestService(repo *mockRelationshipRepo) *Service {
	return NewService(repo, testLogger())
}

// ============ CreateRelationship Tests ============

func TestCreateRelationship_ValidInput_Success(t *testing.T) {
	// Setup
	mockRepo := &mockRelationshipRepo{}
	svc := newTestService(mockRepo)

	req := &relationshipv1.CreateRelationshipRequest{
		TenantId:     "tenant-1",
		FromEntityId: "person-1",
		ToEntityId:   "person-2",
		Type:         relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO,
	}

	// Execute
	resp, err := svc.CreateRelationship(context.Background(), req)

	// Verify
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "rel-123", resp.Relationship.Id)
	assert.Equal(t, "person-1", resp.Relationship.SourceEntity.Id)
	assert.Equal(t, "person-2", resp.Relationship.TargetEntity.Id)
	assert.Equal(t, relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO, resp.Relationship.RelationshipType)
	assert.Equal(t, float32(1.0), resp.Relationship.Confidence, "Manual relationships should have confidence=1.0")
	assert.Equal(t, relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED, resp.Relationship.Status, "Manual relationships should be confirmed")
}

func TestCreateRelationship_MissingTenantID_Error(t *testing.T) {
	// Setup
	mockRepo := &mockRelationshipRepo{}
	svc := newTestService(mockRepo)

	req := &relationshipv1.CreateRelationshipRequest{
		FromEntityId: "person-1",
		ToEntityId:   "person-2",
		Type:         relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO,
	}

	// Execute
	resp, err := svc.CreateRelationship(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "tenant_id")
}

func TestCreateRelationship_MissingFromEntityID_Error(t *testing.T) {
	// Setup
	mockRepo := &mockRelationshipRepo{}
	svc := newTestService(mockRepo)

	req := &relationshipv1.CreateRelationshipRequest{
		TenantId:   "tenant-1",
		ToEntityId: "person-2",
		Type:       relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO,
	}

	// Execute
	resp, err := svc.CreateRelationship(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "from_entity_id")
}

func TestCreateRelationship_MissingToEntityID_Error(t *testing.T) {
	// Setup
	mockRepo := &mockRelationshipRepo{}
	svc := newTestService(mockRepo)

	req := &relationshipv1.CreateRelationshipRequest{
		TenantId:     "tenant-1",
		FromEntityId: "person-1",
		Type:         relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO,
	}

	// Execute
	resp, err := svc.CreateRelationship(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "to_entity_id")
}

func TestCreateRelationship_MissingType_Error(t *testing.T) {
	// Setup
	mockRepo := &mockRelationshipRepo{}
	svc := newTestService(mockRepo)

	req := &relationshipv1.CreateRelationshipRequest{
		TenantId:     "tenant-1",
		FromEntityId: "person-1",
		ToEntityId:   "person-2",
		Type:         relationshipv1.RelationshipType_RELATIONSHIP_TYPE_UNSPECIFIED,
	}

	// Execute
	resp, err := svc.CreateRelationship(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "type")
}

func TestCreateRelationship_SetsConfidenceToOne(t *testing.T) {
	// Setup
	var capturedReq relationships.InsertRelationshipRequest
	mockRepo := &mockRelationshipRepo{
		insertRelationshipFn: func(ctx context.Context, req relationships.InsertRelationshipRequest) (*relationships.Relationship, error) {
			capturedReq = req
			now := time.Now()
			return &relationships.Relationship{
				ID:         "rel-123",
				SourceID:   req.FromEntityID,
				TargetID:   req.ToEntityID,
				Type:       req.Type,
				Confidence: req.Confidence,
				Status:     relationships.RelationshipStatusConfirmed,
				TenantID:   req.TenantID,
				CreatedAt:  now,
				UpdatedAt:  now,
			}, nil
		},
	}
	svc := newTestService(mockRepo)

	req := &relationshipv1.CreateRelationshipRequest{
		TenantId:     "tenant-1",
		FromEntityId: "person-1",
		ToEntityId:   "person-2",
		Type:         relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO,
	}

	// Execute
	_, err := svc.CreateRelationship(context.Background(), req)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 1.0, capturedReq.Confidence, "Manual relationships should have confidence=1.0")
	assert.True(t, capturedReq.UserConfirmed, "Manual relationships should have user_confirmed=true")
}

func TestCreateRelationship_WithOptionalSubtype(t *testing.T) {
	// Setup
	var capturedReq relationships.InsertRelationshipRequest
	subtype := "direct"
	mockRepo := &mockRelationshipRepo{
		insertRelationshipFn: func(ctx context.Context, req relationships.InsertRelationshipRequest) (*relationships.Relationship, error) {
			capturedReq = req
			now := time.Now()
			return &relationships.Relationship{
				ID:         "rel-123",
				SourceID:   req.FromEntityID,
				TargetID:   req.ToEntityID,
				Type:       req.Type,
				Confidence: req.Confidence,
				Status:     relationships.RelationshipStatusConfirmed,
				TenantID:   req.TenantID,
				CreatedAt:  now,
				UpdatedAt:  now,
			}, nil
		},
	}
	svc := newTestService(mockRepo)

	req := &relationshipv1.CreateRelationshipRequest{
		TenantId:     "tenant-1",
		FromEntityId: "person-1",
		ToEntityId:   "person-2",
		Type:         relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO,
		Subtype:      &subtype,
	}

	// Execute
	_, err := svc.CreateRelationship(context.Background(), req)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, "direct", capturedReq.Subtype)
}

func TestCreateRelationship_ValidatesFromEntityExists(t *testing.T) {
	// Setup
	mockRepo := &mockRelationshipRepo{
		getEntityFn: func(ctx context.Context, tenantID, entityID string) (*relationships.Entity, error) {
			if entityID == "person-1" {
				return nil, nil // Entity not found
			}
			now := time.Now()
			return &relationships.Entity{
				ID:        entityID,
				Name:      "Entity",
				Type:      relationships.EntityTypePerson,
				CreatedAt: now,
				UpdatedAt: now,
			}, nil
		},
	}
	svc := newTestService(mockRepo)

	req := &relationshipv1.CreateRelationshipRequest{
		TenantId:     "tenant-1",
		FromEntityId: "person-1",
		ToEntityId:   "person-2",
		Type:         relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO,
	}

	// Execute
	resp, err := svc.CreateRelationship(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "person-1")
}

func TestCreateRelationship_ValidatesToEntityExists(t *testing.T) {
	// Setup
	mockRepo := &mockRelationshipRepo{
		getEntityFn: func(ctx context.Context, tenantID, entityID string) (*relationships.Entity, error) {
			if entityID == "person-2" {
				return nil, nil // Entity not found
			}
			now := time.Now()
			return &relationships.Entity{
				ID:        entityID,
				Name:      "Entity",
				Type:      relationships.EntityTypePerson,
				CreatedAt: now,
				UpdatedAt: now,
			}, nil
		},
	}
	svc := newTestService(mockRepo)

	req := &relationshipv1.CreateRelationshipRequest{
		TenantId:     "tenant-1",
		FromEntityId: "person-1",
		ToEntityId:   "person-2",
		Type:         relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO,
	}

	// Execute
	resp, err := svc.CreateRelationship(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "person-2")
}

func TestCreateRelationship_DuplicateRelationship_Error(t *testing.T) {
	// Setup
	mockRepo := &mockRelationshipRepo{
		insertRelationshipFn: func(ctx context.Context, req relationships.InsertRelationshipRequest) (*relationships.Relationship, error) {
			return nil, errors.New("duplicate key value violates unique constraint")
		},
	}
	svc := newTestService(mockRepo)

	req := &relationshipv1.CreateRelationshipRequest{
		TenantId:     "tenant-1",
		FromEntityId: "person-1",
		ToEntityId:   "person-2",
		Type:         relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO,
	}

	// Execute
	resp, err := svc.CreateRelationship(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.AlreadyExists, st.Code())
	assert.Contains(t, st.Message(), "already exists")
}

func TestCreateRelationship_RepositoryError_InternalError(t *testing.T) {
	// Setup
	mockRepo := &mockRelationshipRepo{
		insertRelationshipFn: func(ctx context.Context, req relationships.InsertRelationshipRequest) (*relationships.Relationship, error) {
			return nil, errors.New("database connection lost")
		},
	}
	svc := newTestService(mockRepo)

	req := &relationshipv1.CreateRelationshipRequest{
		TenantId:     "tenant-1",
		FromEntityId: "person-1",
		ToEntityId:   "person-2",
		Type:         relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO,
	}

	// Execute
	resp, err := svc.CreateRelationship(context.Background(), req)

	// Verify
	require.Error(t, err)
	require.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}
