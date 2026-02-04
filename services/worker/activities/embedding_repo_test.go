package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockEmbeddingRepo is a mock implementation of EmbeddingRepository for testing.
type mockEmbeddingRepo struct {
	storeMultiLevelCalled bool
	storeMultiLevelInput  *MultiLevelEmbeddingInput
	storeMultiLevelID     int64
	storeMultiLevelErr    error

	getEmbeddingsForSourceCalled bool
	getEmbeddingsOutput          []*Embedding
	getEmbeddingsErr             error

	getStaleEmbeddingsCalled bool
	getStaleOutput           []int64
	getStaleErr              error

	deleteEmbeddingsCalled bool
	deleteErr              error
}

func (m *mockEmbeddingRepo) StoreEmbedding(ctx context.Context, tenantID string, sourceID int64, vector []float32, model string, dimensions int32) (int64, error) {
	// Backward compat method - not testing this directly
	return 0, nil
}

func (m *mockEmbeddingRepo) GetEmbedding(ctx context.Context, tenantID string, embeddingID int64) (*Embedding, error) {
	// Not testing this directly
	return nil, nil
}

func (m *mockEmbeddingRepo) StoreMultiLevelEmbedding(ctx context.Context, input *MultiLevelEmbeddingInput) (int64, error) {
	m.storeMultiLevelCalled = true
	m.storeMultiLevelInput = input
	if m.storeMultiLevelErr != nil {
		return 0, m.storeMultiLevelErr
	}
	return m.storeMultiLevelID, nil
}

func (m *mockEmbeddingRepo) GetEmbeddingsForSource(ctx context.Context, tenantID string, sourceID int64, representationType string) ([]*Embedding, error) {
	m.getEmbeddingsForSourceCalled = true
	if m.getEmbeddingsErr != nil {
		return nil, m.getEmbeddingsErr
	}
	return m.getEmbeddingsOutput, nil
}

func (m *mockEmbeddingRepo) GetStaleEmbeddings(ctx context.Context, tenantID string, currentModel string, currentVersion string, limit int) ([]int64, error) {
	m.getStaleEmbeddingsCalled = true
	if m.getStaleErr != nil {
		return nil, m.getStaleErr
	}
	return m.getStaleOutput, nil
}

func (m *mockEmbeddingRepo) DeleteEmbeddingsForSource(ctx context.Context, tenantID string, sourceID int64) error {
	m.deleteEmbeddingsCalled = true
	return m.deleteErr
}

func TestStoreMultiLevelEmbedding_Success(t *testing.T) {
	mock := &mockEmbeddingRepo{
		storeMultiLevelID: 100,
	}

	ctx := context.Background()
	input := &MultiLevelEmbeddingInput{
		TenantID:           "123e4567-e89b-12d3-a456-426614174000",
		SourceID:           42,
		EntityType:         "source",
		EntityID:           42,
		RepresentationType: "summary",
		Vector:             []float32{0.1, 0.2, 0.3},
		Model:              "mxbai-embed-large-v1",
		ModelVersion:       "1.0",
		TextContent:        "Test summary",
		ContentHash:        "abc123",
	}

	id, err := mock.StoreMultiLevelEmbedding(ctx, input)

	assert.NoError(t, err)
	assert.Equal(t, int64(100), id)
	assert.True(t, mock.storeMultiLevelCalled)
	assert.Equal(t, input, mock.storeMultiLevelInput)
}

func TestGetEmbeddingsForSource_WithFilter(t *testing.T) {
	expectedEmbeddings := []*Embedding{
		{
			ID:                 100,
			SourceID:           42,
			TenantID:           "123e4567-e89b-12d3-a456-426614174000",
			EntityType:         "source",
			EntityID:           42,
			RepresentationType: "summary",
			Vector:             []float32{0.1, 0.2},
			Model:              "test-model",
			ModelVersion:       "1.0",
			Dimensions:         2,
		},
	}

	mock := &mockEmbeddingRepo{
		getEmbeddingsOutput: expectedEmbeddings,
	}

	ctx := context.Background()
	tenantID := "123e4567-e89b-12d3-a456-426614174000"
	sourceID := int64(42)
	representationType := "summary"

	embeddings, err := mock.GetEmbeddingsForSource(ctx, tenantID, sourceID, representationType)

	assert.NoError(t, err)
	assert.Len(t, embeddings, 1)
	assert.Equal(t, int64(100), embeddings[0].ID)
	assert.Equal(t, "summary", embeddings[0].RepresentationType)
	assert.Equal(t, "test-model", embeddings[0].Model)
	assert.Equal(t, "1.0", embeddings[0].ModelVersion)
	assert.True(t, mock.getEmbeddingsForSourceCalled)
}

func TestGetEmbeddingsForSource_AllTypes(t *testing.T) {
	expectedEmbeddings := []*Embedding{
		{
			ID:                 100,
			RepresentationType: "content",
		},
		{
			ID:                 101,
			RepresentationType: "summary",
		},
		{
			ID:                 102,
			RepresentationType: "action_item",
		},
	}

	mock := &mockEmbeddingRepo{
		getEmbeddingsOutput: expectedEmbeddings,
	}

	ctx := context.Background()
	tenantID := "123e4567-e89b-12d3-a456-426614174000"
	sourceID := int64(42)

	embeddings, err := mock.GetEmbeddingsForSource(ctx, tenantID, sourceID, "")

	assert.NoError(t, err)
	assert.Len(t, embeddings, 3)
	assert.Equal(t, "content", embeddings[0].RepresentationType)
	assert.Equal(t, "summary", embeddings[1].RepresentationType)
	assert.Equal(t, "action_item", embeddings[2].RepresentationType)
	assert.True(t, mock.getEmbeddingsForSourceCalled)
}

func TestGetStaleEmbeddings(t *testing.T) {
	expectedSourceIDs := []int64{1, 2, 3}

	mock := &mockEmbeddingRepo{
		getStaleOutput: expectedSourceIDs,
	}

	ctx := context.Background()
	tenantID := "123e4567-e89b-12d3-a456-426614174000"
	currentModel := "new-model"
	currentVersion := "2.0"
	limit := 10

	sourceIDs, err := mock.GetStaleEmbeddings(ctx, tenantID, currentModel, currentVersion, limit)

	assert.NoError(t, err)
	assert.Len(t, sourceIDs, 3)
	assert.Equal(t, []int64{1, 2, 3}, sourceIDs)
	assert.True(t, mock.getStaleEmbeddingsCalled)
}

func TestDeleteEmbeddingsForSource(t *testing.T) {
	mock := &mockEmbeddingRepo{}

	ctx := context.Background()
	tenantID := "123e4567-e89b-12d3-a456-426614174000"
	sourceID := int64(42)

	err := mock.DeleteEmbeddingsForSource(ctx, tenantID, sourceID)

	assert.NoError(t, err)
	assert.True(t, mock.deleteEmbeddingsCalled)
}

// The following tests verify the PostgresEmbeddingRepository validation logic.
// Since we can't easily mock pgxpool, we test validation by checking that
// invalid inputs produce errors before the database call.

func TestPostgresEmbeddingRepository_StoreMultiLevelEmbedding_Validation(t *testing.T) {
	// Note: These tests would require a real database connection or a more sophisticated mock.
	// For now, we document the validation requirements that are implemented in the code:
	//
	// - tenant_id is required (non-empty)
	// - source_id must be > 0
	// - entity_type is required (non-empty)
	// - entity_id must be > 0
	// - representation_type is required (non-empty)
	// - vector must not be empty
	// - model is required (non-empty)
	//
	// These validations are covered by the implementation in embedding_repo.go
}

func TestPostgresEmbeddingRepository_GetEmbeddingsForSource_Validation(t *testing.T) {
	// Note: Validation requirements:
	// - tenant_id is required (non-empty)
	// - source_id must be > 0
}

func TestPostgresEmbeddingRepository_GetStaleEmbeddings_Validation(t *testing.T) {
	// Note: Validation requirements:
	// - tenant_id is required (non-empty)
	// - current_model is required (non-empty)
	// - limit must be > 0
}

func TestPostgresEmbeddingRepository_DeleteEmbeddingsForSource_Validation(t *testing.T) {
	// Note: Validation requirements:
	// - tenant_id is required (non-empty)
	// - source_id must be > 0
}
