// Package activities provides tests for activity constructor nil checks.
package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/mentions"
	"github.com/otherjamesbrown/penfold/pkg/mentions/resolver"
)

// Minimal mock implementations for repository types that don't exist in other test files

// mockEmbeddingRepository implements EmbeddingRepository for testing.
type mockEmbeddingRepository struct{}

func (m *mockEmbeddingRepository) StoreEmbedding(ctx context.Context, tenantID string, sourceID int64, vector []float32, model string, dimensions int32) (int64, error) {
	return 0, nil
}

func (m *mockEmbeddingRepository) GetEmbedding(ctx context.Context, tenantID string, embeddingID int64) (*Embedding, error) {
	return nil, nil
}

func (m *mockEmbeddingRepository) StoreMultiLevelEmbedding(ctx context.Context, input *MultiLevelEmbeddingInput) (int64, error) {
	return 0, nil
}

func (m *mockEmbeddingRepository) GetEmbeddingsForSource(ctx context.Context, tenantID string, sourceID int64, representationType string) ([]*Embedding, error) {
	return nil, nil
}

func (m *mockEmbeddingRepository) GetStaleEmbeddings(ctx context.Context, tenantID string, currentModel string, currentVersion string, limit int) ([]int64, error) {
	return nil, nil
}

func (m *mockEmbeddingRepository) GetEmbeddingsByEntityType(ctx context.Context, tenantID string, entityType string, entityIDs []int64, representationType string) ([]*Embedding, error) {
	return nil, nil
}

func (m *mockEmbeddingRepository) DeleteEmbeddingsForSource(ctx context.Context, tenantID string, sourceID int64) error {
	return nil
}

func (m *mockEmbeddingRepository) DeleteEmbedding(ctx context.Context, embeddingID int64) error {
	return nil
}

// mockSummaryRepository implements SummaryRepository for testing.
type mockSummaryRepository struct{}

func (m *mockSummaryRepository) StoreSummary(ctx context.Context, tenantID string, sourceID int64, summary string, keyPoints []string, model string) (int64, error) {
	return 0, nil
}

func (m *mockSummaryRepository) DeleteSummary(ctx context.Context, summaryID int64) error {
	return nil
}

// mockAssertionRepository implements AssertionRepository for testing.
type mockAssertionRepository struct{}

func (m *mockAssertionRepository) StoreAssertions(ctx context.Context, tenantID string, sourceID int64, assertions []*Assertion, model string) (int, error) {
	return 0, nil
}

func (m *mockAssertionRepository) DeleteAssertions(ctx context.Context, tenantID string, sourceID int64) (int, error) {
	return 0, nil
}

// mockEntityRepository implements EntityRepository for testing.
type mockEntityRepository struct{}

func (m *mockEntityRepository) StoreEntities(ctx context.Context, tenantID string, sourceID int64, entities []*Entity) (int, error) {
	return 0, nil
}

// mockContextPackageRepo is already defined in context_repo_test.go, so we don't redefine it here

// Tests for EmbeddingActivities constructor

func TestNewEmbeddingActivities_NilLogger(t *testing.T) {
	assert.PanicsWithValue(t, "NewEmbeddingActivities: logger is required", func() {
		NewEmbeddingActivities(nil, &mockAIClient{}, &mockEmbeddingRepository{}, nil, nil)
	})
}

func TestNewEmbeddingActivities_NilAIClient(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewEmbeddingActivities: aiClient is required", func() {
		NewEmbeddingActivities(logger, nil, &mockEmbeddingRepository{}, nil, nil)
	})
}

func TestNewEmbeddingActivities_NilEmbeddingRepo(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewEmbeddingActivities: embeddingRepo is required", func() {
		NewEmbeddingActivities(logger, &mockAIClient{}, nil, nil, nil)
	})
}

func TestNewEmbeddingActivities_Success(t *testing.T) {
	logger := logging.NewNopLogger()
	// PipelineRepo is optional - can be nil
	activities := NewEmbeddingActivities(logger, &mockAIClient{}, &mockEmbeddingRepository{}, nil, nil)
	assert.NotNil(t, activities)
}

// Tests for SummarizationActivities constructor

func TestNewSummarizationActivities_NilLogger(t *testing.T) {
	assert.PanicsWithValue(t, "NewSummarizationActivities: logger is required", func() {
		NewSummarizationActivities(nil, &mockAIClient{}, &mockSummaryRepository{})
	})
}

func TestNewSummarizationActivities_NilAIClient(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewSummarizationActivities: aiClient is required", func() {
		NewSummarizationActivities(logger, nil, &mockSummaryRepository{})
	})
}

func TestNewSummarizationActivities_NilSummaryRepo(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewSummarizationActivities: summaryRepo is required", func() {
		NewSummarizationActivities(logger, &mockAIClient{}, nil)
	})
}

func TestNewSummarizationActivities_Success(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewSummarizationActivities(logger, &mockAIClient{}, &mockSummaryRepository{})
	assert.NotNil(t, activities)
}

// Tests for ExtractionActivities constructor

func TestNewExtractionActivities_NilLogger(t *testing.T) {
	assert.PanicsWithValue(t, "NewExtractionActivities: logger is required", func() {
		NewExtractionActivities(nil, &mockAIClient{}, &mockAssertionRepository{}, &mockEntityRepository{}, nil)
	})
}

func TestNewExtractionActivities_NilAIClient(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewExtractionActivities: aiClient is required", func() {
		NewExtractionActivities(logger, nil, &mockAssertionRepository{}, &mockEntityRepository{}, nil)
	})
}

func TestNewExtractionActivities_NilAssertionRepo(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewExtractionActivities: assertionRepo is required", func() {
		NewExtractionActivities(logger, &mockAIClient{}, nil, &mockEntityRepository{}, nil)
	})
}

func TestNewExtractionActivities_NilEntityRepo(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewExtractionActivities: entityRepo is required", func() {
		NewExtractionActivities(logger, &mockAIClient{}, &mockAssertionRepository{}, nil, nil)
	})
}

func TestNewExtractionActivities_Success(t *testing.T) {
	logger := logging.NewNopLogger()
	// PipelineRepo is optional - can be nil
	activities := NewExtractionActivities(logger, &mockAIClient{}, &mockAssertionRepository{}, &mockEntityRepository{}, nil)
	assert.NotNil(t, activities)
}

// Tests for TriageActivities constructor

func TestNewTriageActivities_NilLogger(t *testing.T) {
	assert.PanicsWithValue(t, "NewTriageActivities: logger is required", func() {
		NewTriageActivities(nil, &mockAIClient{}, nil, nil, nil)
	})
}

func TestNewTriageActivities_NilAIClient(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewTriageActivities: aiClient is required", func() {
		NewTriageActivities(logger, nil, nil, nil, nil)
	})
}

func TestNewTriageActivities_Success(t *testing.T) {
	logger := logging.NewNopLogger()
	// PipelineRepo and EnrichmentRepo are optional - can be nil
	activities := NewTriageActivities(logger, &mockAIClient{}, nil, nil, nil)
	assert.NotNil(t, activities)
}

// Tests for AnalysisActivities constructor

func TestNewAnalysisActivities_NilLogger(t *testing.T) {
	assert.PanicsWithValue(t, "NewAnalysisActivities: logger is required", func() {
		NewAnalysisActivities(nil, &mockAIClient{}, nil)
	})
}

func TestNewAnalysisActivities_NilAIClient(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewAnalysisActivities: aiClient is required", func() {
		NewAnalysisActivities(logger, nil, nil)
	})
}

func TestNewAnalysisActivities_Success(t *testing.T) {
	logger := logging.NewNopLogger()
	// PipelineRepo is optional - can be nil
	activities := NewAnalysisActivities(logger, &mockAIClient{}, nil)
	assert.NotNil(t, activities)
}

// Tests for ContextBuilderActivities constructor

func TestNewContextBuilderActivities_NilLogger(t *testing.T) {
	assert.PanicsWithValue(t, "NewContextBuilderActivities: logger is required", func() {
		NewContextBuilderActivities(nil, &mockEntityResolver{}, &mockEntityLookup{}, &mockContextPackageRepo{}, nil, nil, nil)
	})
}

func TestNewContextBuilderActivities_NilEntityResolver(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewContextBuilderActivities: entityResolver is required", func() {
		NewContextBuilderActivities(logger, nil, &mockEntityLookup{}, &mockContextPackageRepo{}, nil, nil, nil)
	})
}

func TestNewContextBuilderActivities_NilEntityRepo(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewContextBuilderActivities: entityRepo is required", func() {
		NewContextBuilderActivities(logger, &mockEntityResolver{}, nil, &mockContextPackageRepo{}, nil, nil, nil)
	})
}

func TestNewContextBuilderActivities_NilContextRepo(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewContextBuilderActivities: contextRepo is required", func() {
		NewContextBuilderActivities(logger, &mockEntityResolver{}, &mockEntityLookup{}, nil, nil, nil, nil)
	})
}

func TestNewContextBuilderActivities_Success(t *testing.T) {
	logger := logging.NewNopLogger()
	// PipelineRepo, TopicRepo, and ConfigResolver are optional - can be nil
	activities := NewContextBuilderActivities(logger, &mockEntityResolver{}, &mockEntityLookup{}, &mockContextPackageRepo{}, nil, nil, nil)
	assert.NotNil(t, activities)
}

// Tests for PersistActivities constructor

func TestNewPersistActivities_NilLogger(t *testing.T) {
	assert.PanicsWithValue(t, "NewPersistActivities: logger is required", func() {
		NewPersistActivities(nil, &mockPersistRepository{}, nil)
	})
}

func TestNewPersistActivities_NilRepository(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewPersistActivities: repository is required", func() {
		NewPersistActivities(logger, nil, nil)
	})
}

func TestNewPersistActivities_Success(t *testing.T) {
	logger := logging.NewNopLogger()
	// PipelineRepo is optional - can be nil
	activities := NewPersistActivities(logger, &mockPersistRepository{}, nil)
	assert.NotNil(t, activities)
}

// Tests for MentionsActivities constructor

func TestNewMentionsActivities_NilLogger(t *testing.T) {
	assert.PanicsWithValue(t, "NewMentionsActivities: logger is required", func() {
		NewMentionsActivities(nil, &pgxpool.Pool{}, &resolver.Resolver{}, &mentions.PostgresRepository{})
	})
}

func TestNewMentionsActivities_NilDB(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewMentionsActivities: db is required", func() {
		NewMentionsActivities(logger, nil, &resolver.Resolver{}, &mentions.PostgresRepository{})
	})
}

func TestNewMentionsActivities_NilResolver(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewMentionsActivities: res is required", func() {
		NewMentionsActivities(logger, &pgxpool.Pool{}, nil, &mentions.PostgresRepository{})
	})
}

func TestNewMentionsActivities_NilRepo(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewMentionsActivities: repo is required", func() {
		NewMentionsActivities(logger, &pgxpool.Pool{}, &resolver.Resolver{}, nil)
	})
}

func TestNewMentionsActivities_Success(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewMentionsActivities(logger, &pgxpool.Pool{}, &resolver.Resolver{}, &mentions.PostgresRepository{})
	assert.NotNil(t, activities)
}

// Tests for ParseActivities constructor

func TestNewParseActivities_NilLogger(t *testing.T) {
	assert.PanicsWithValue(t, "NewParseActivities: logger is required", func() {
		NewParseActivities(nil)
	})
}

func TestNewParseActivities_Success(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewParseActivities(logger)
	assert.NotNil(t, activities)
}

// Tests for NewActivitiesWithDB constructor

func TestNewActivitiesWithDB_NilLogger(t *testing.T) {
	assert.PanicsWithValue(t, "NewActivitiesWithDB: logger is required", func() {
		NewActivitiesWithDB(nil, &pgxpool.Pool{}, "http://localhost:8080")
	})
}

func TestNewActivitiesWithDB_NilDB(t *testing.T) {
	logger := logging.NewNopLogger()
	assert.PanicsWithValue(t, "NewActivitiesWithDB: db is required", func() {
		NewActivitiesWithDB(logger, nil, "http://localhost:8080")
	})
}

func TestNewActivitiesWithDB_Success(t *testing.T) {
	logger := logging.NewNopLogger()
	activities := NewActivitiesWithDB(logger, &pgxpool.Pool{}, "http://localhost:8080")
	assert.NotNil(t, activities)
}
