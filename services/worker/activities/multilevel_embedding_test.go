package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// Test-First Development for Chunking (pf-963a0e)
//
// The chunking tests below (TestGenerateMultiLevelEmbeddings_ChunksLongContent,
// TestGenerateMultiLevelEmbeddings_ShortContentSingleChunk) are written BEFORE
// implementation to define the expected behavior:
//
// 1. Long content (>400 tokens) should be chunked and produce multiple content embeddings
// 2. Short content (<400 tokens) should produce a single content embedding with chunk_index=0, chunk_total=1
// 3. Each chunk embedding should have ChunkIndex and ChunkTotal metadata stored
//
// Implementation TODO (see pf-963a0e):
// - Add ChunkIndex and ChunkTotal fields to MultiLevelEmbeddingInput (interfaces.go)
// - Import pkg/chunking in multilevel_embedding.go and embedding.go
// - Call chunking.ChunkContent() before generating embeddings
// - Loop over chunks and store each with proper chunk metadata
// - Update embedding_repo.go to INSERT chunk_index and chunk_total columns
//
// When implementation is complete:
// - Uncomment the TODO sections in the tests below that verify chunk metadata
// - All tests should pass

// mockSourceRepo is a mock implementation of SourceRepository for testing.
type mockSourceRepo struct {
	sources map[int64]*Source
	getErr  error
}

func (m *mockSourceRepo) GetSource(ctx context.Context, tenantID string, sourceID int64) (*Source, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.sources == nil {
		return nil, errors.New("source not found")
	}
	source, ok := m.sources[sourceID]
	if !ok {
		return nil, errors.New("source not found")
	}
	return source, nil
}

func (m *mockSourceRepo) UpdateSourceStatus(ctx context.Context, tenantID string, sourceID int64, status string) error {
	return nil
}

func (m *mockSourceRepo) UpdateSourceStatusWithFailure(ctx context.Context, tenantID string, sourceID int64, status, failureCategory, failureReason string, triageMetadata ...map[string]interface{}) error {
	return nil
}

// mlMockEmbeddingRepo is a mock for multi-level embedding tests.
// We need a different name because mockEmbeddingRepo already exists in embedding_repo_test.go
type mlMockEmbeddingRepo struct {
	storeMultiLevelCalls []MultiLevelEmbeddingInput // record all calls
	storeMultiLevelIDs   []int64                     // return IDs in order
	storeMultiLevelErr   error
	storeMultiLevelIndex int // track which ID to return next

	deleteForSourceCalls []int64 // record source IDs deleted
	deleteErr            error

	getStaleSources []int64
	getStaleErr     error

	getEmbeddingMap map[int64]*Embedding // for GetEmbedding
	getEmbeddingErr error
}

func (m *mlMockEmbeddingRepo) StoreEmbedding(ctx context.Context, tenantID string, sourceID int64, vector []float32, model string, dimensions int32) (int64, error) {
	return 0, nil
}

func (m *mlMockEmbeddingRepo) GetEmbedding(ctx context.Context, tenantID string, embeddingID int64) (*Embedding, error) {
	if m.getEmbeddingErr != nil {
		return nil, m.getEmbeddingErr
	}
	if m.getEmbeddingMap != nil {
		if emb, ok := m.getEmbeddingMap[embeddingID]; ok {
			return emb, nil
		}
	}
	return nil, errors.New("embedding not found")
}

func (m *mlMockEmbeddingRepo) StoreMultiLevelEmbedding(ctx context.Context, input *MultiLevelEmbeddingInput) (int64, error) {
	if m.storeMultiLevelErr != nil {
		return 0, m.storeMultiLevelErr
	}
	// Record the call
	m.storeMultiLevelCalls = append(m.storeMultiLevelCalls, *input)

	// Return next ID from list
	if m.storeMultiLevelIndex < len(m.storeMultiLevelIDs) {
		id := m.storeMultiLevelIDs[m.storeMultiLevelIndex]
		m.storeMultiLevelIndex++
		return id, nil
	}
	return int64(100 + len(m.storeMultiLevelCalls)), nil
}

func (m *mlMockEmbeddingRepo) GetEmbeddingsForSource(ctx context.Context, tenantID string, sourceID int64, representationType string) ([]*Embedding, error) {
	return nil, nil
}

func (m *mlMockEmbeddingRepo) GetStaleEmbeddings(ctx context.Context, tenantID string, currentModel string, currentVersion string, limit int) ([]int64, error) {
	if m.getStaleErr != nil {
		return nil, m.getStaleErr
	}
	return m.getStaleSources, nil
}

func (m *mlMockEmbeddingRepo) DeleteEmbeddingsForSource(ctx context.Context, tenantID string, sourceID int64) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deleteForSourceCalls = append(m.deleteForSourceCalls, sourceID)
	return nil
}

func (m *mlMockEmbeddingRepo) DeleteEmbedding(ctx context.Context, embeddingID int64) error {
	return m.deleteErr
}

func TestGenerateMultiLevelEmbeddings_FullPipeline(t *testing.T) {
	logger := logging.NewNopLogger()

	// Track all AI client calls
	var embeddingCalls []string

	mockClient := &mockAIClient{
		generateEmbeddingFn: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			embeddingCalls = append(embeddingCalls, req.Text)
			return &aiv1.EmbeddingResponse{
				Vector:     make([]float32, 1024),
				Dimensions: 1024,
				ModelUsed:  "mxbai-embed-large-v1",
			}, nil
		},
	}

	mockRepo := &mlMockEmbeddingRepo{
		storeMultiLevelIDs: []int64{100, 101, 102, 103, 104, 105},
		getEmbeddingMap: map[int64]*Embedding{
			100: {
				ID:           100,
				Model:        "mxbai-embed-large-v1",
				ModelVersion: "mxbai-embed-large-v1",
			},
		},
	}

	activities := NewMultiLevelEmbeddingActivities(logger, mockClient, mockRepo, nil)

	input := GenerateMultiLevelInput{
		TenantID:    "test-tenant",
		SourceID:    42,
		ContentID:   "em-abc123",
		Content:     "This is the raw email content for testing.",
		ContentType: "email",
		Analysis: &DeepAnalyzeOutput{
			Summary: "Summary of the content",
			VerifiedActions: []VerifiedActionOutput{
				{Description: "Review design doc", Status: "confirmed"},
				{Description: "Update timeline", Status: "confirmed"},
			},
			VerifiedDecisions: []VerifiedDecisionOutput{
				{Description: "Approved budget increase", Status: "confirmed"},
			},
			RiskReferences: []RiskReferenceOutput{
				{Description: "Resource constraint may impact timeline"},
			},
		},
		AssertionIDs: map[string]int64{
			"action:0":   200,
			"action:1":   201,
			"decision:0": 202,
			"risk:0":     203,
		},
	}

	output, err := activities.GenerateMultiLevelEmbeddings(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify output counts
	require.Equal(t, 6, output.TotalGenerated)
	require.Equal(t, 0, output.TotalFailed)
	require.Len(t, output.SourceEmbeddingIDs, 2)      // content + summary
	require.Len(t, output.AssertionEmbeddingIDs, 4)   // 2 actions + 1 decision + 1 risk
	require.Equal(t, "mxbai-embed-large-v1", output.ModelUsed)

	// Verify delete was called
	require.Len(t, mockRepo.deleteForSourceCalls, 1)
	require.Equal(t, int64(42), mockRepo.deleteForSourceCalls[0])

	// Verify 6 embeddings were stored
	require.Len(t, mockRepo.storeMultiLevelCalls, 6)

	// Verify content embedding
	require.Equal(t, "source", mockRepo.storeMultiLevelCalls[0].EntityType)
	require.Equal(t, int64(42), mockRepo.storeMultiLevelCalls[0].EntityID)
	require.Equal(t, "content", mockRepo.storeMultiLevelCalls[0].RepresentationType)

	// Verify summary embedding
	require.Equal(t, "source", mockRepo.storeMultiLevelCalls[1].EntityType)
	require.Equal(t, int64(42), mockRepo.storeMultiLevelCalls[1].EntityID)
	require.Equal(t, "summary", mockRepo.storeMultiLevelCalls[1].RepresentationType)

	// Verify first action embedding
	require.Equal(t, "assertion", mockRepo.storeMultiLevelCalls[2].EntityType)
	require.Equal(t, int64(200), mockRepo.storeMultiLevelCalls[2].EntityID)
	require.Equal(t, "action_item", mockRepo.storeMultiLevelCalls[2].RepresentationType)

	// Verify second action embedding
	require.Equal(t, "assertion", mockRepo.storeMultiLevelCalls[3].EntityType)
	require.Equal(t, int64(201), mockRepo.storeMultiLevelCalls[3].EntityID)
	require.Equal(t, "action_item", mockRepo.storeMultiLevelCalls[3].RepresentationType)

	// Verify decision embedding
	require.Equal(t, "assertion", mockRepo.storeMultiLevelCalls[4].EntityType)
	require.Equal(t, int64(202), mockRepo.storeMultiLevelCalls[4].EntityID)
	require.Equal(t, "decision", mockRepo.storeMultiLevelCalls[4].RepresentationType)

	// Verify risk embedding
	require.Equal(t, "assertion", mockRepo.storeMultiLevelCalls[5].EntityType)
	require.Equal(t, int64(203), mockRepo.storeMultiLevelCalls[5].EntityID)
	require.Equal(t, "risk", mockRepo.storeMultiLevelCalls[5].RepresentationType)
}

func TestGenerateMultiLevelEmbeddings_EmptyAnalysis(t *testing.T) {
	logger := logging.NewNopLogger()

	mockClient := &mockAIClient{
		generateEmbeddingFn: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			return &aiv1.EmbeddingResponse{
				Vector:     make([]float32, 1024),
				Dimensions: 1024,
				ModelUsed:  "mxbai-embed-large-v1",
			}, nil
		},
	}

	mockRepo := &mlMockEmbeddingRepo{
		storeMultiLevelIDs: []int64{100},
		getEmbeddingMap: map[int64]*Embedding{
			100: {
				ID:           100,
				Model:        "mxbai-embed-large-v1",
				ModelVersion: "mxbai-embed-large-v1",
			},
		},
	}

	activities := NewMultiLevelEmbeddingActivities(logger, mockClient, mockRepo, nil)

	input := GenerateMultiLevelInput{
		TenantID:    "test-tenant",
		SourceID:    42,
		Content:     "Content with no findings",
		ContentType: "email",
		Analysis: &DeepAnalyzeOutput{
			Summary:           "", // Empty summary
			VerifiedActions:   []VerifiedActionOutput{},
			VerifiedDecisions: []VerifiedDecisionOutput{},
			RiskReferences:    []RiskReferenceOutput{},
		},
	}

	output, err := activities.GenerateMultiLevelEmbeddings(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Should only generate content embedding
	require.Equal(t, 1, output.TotalGenerated)
	require.Equal(t, 0, output.TotalFailed)
	require.Len(t, output.SourceEmbeddingIDs, 1)    // only content
	require.Len(t, output.AssertionEmbeddingIDs, 0) // no assertions

	// Verify only content embedding was stored
	require.Len(t, mockRepo.storeMultiLevelCalls, 1)
	require.Equal(t, "content", mockRepo.storeMultiLevelCalls[0].RepresentationType)
}

func TestGenerateMultiLevelEmbeddings_SkipsRemovedActions(t *testing.T) {
	logger := logging.NewNopLogger()

	mockClient := &mockAIClient{
		generateEmbeddingFn: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			return &aiv1.EmbeddingResponse{
				Vector:     make([]float32, 1024),
				Dimensions: 1024,
				ModelUsed:  "mxbai-embed-large-v1",
			}, nil
		},
	}

	mockRepo := &mlMockEmbeddingRepo{
		storeMultiLevelIDs: []int64{100, 101, 102},
		getEmbeddingMap: map[int64]*Embedding{
			100: {
				ID:           100,
				Model:        "mxbai-embed-large-v1",
				ModelVersion: "mxbai-embed-large-v1",
			},
		},
	}

	activities := NewMultiLevelEmbeddingActivities(logger, mockClient, mockRepo, nil)

	input := GenerateMultiLevelInput{
		TenantID:    "test-tenant",
		SourceID:    42,
		Content:     "Content with some removed actions",
		ContentType: "email",
		Analysis: &DeepAnalyzeOutput{
			VerifiedActions: []VerifiedActionOutput{
				{Description: "Action 1", Status: "confirmed"},
				{Description: "Action 2", Status: "removed"}, // This should be skipped
				{Description: "Action 3", Status: "confirmed"},
			},
		},
		AssertionIDs: map[string]int64{
			"action:0": 200,
			"action:1": 201,
			"action:2": 202,
		},
	}

	output, err := activities.GenerateMultiLevelEmbeddings(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Should generate: content + 2 action embeddings (skipping the removed one)
	require.Equal(t, 3, output.TotalGenerated)
	require.Equal(t, 0, output.TotalFailed)
	require.Len(t, output.SourceEmbeddingIDs, 1)    // content
	require.Len(t, output.AssertionEmbeddingIDs, 2) // 2 actions (1 was removed)

	// Verify embeddings stored
	require.Len(t, mockRepo.storeMultiLevelCalls, 3)
	require.Equal(t, "content", mockRepo.storeMultiLevelCalls[0].RepresentationType)
	require.Equal(t, "action_item", mockRepo.storeMultiLevelCalls[1].RepresentationType)
	require.Equal(t, int64(200), mockRepo.storeMultiLevelCalls[1].EntityID) // action:0
	require.Equal(t, "action_item", mockRepo.storeMultiLevelCalls[2].RepresentationType)
	require.Equal(t, int64(202), mockRepo.storeMultiLevelCalls[2].EntityID) // action:2
}

func TestGenerateMultiLevelEmbeddings_AssertionFailureContinues(t *testing.T) {
	logger := logging.NewNopLogger()

	callCount := 0
	mockClient := &mockAIClient{
		generateEmbeddingFn: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			callCount++
			// Fail on the 2nd call (first action embedding)
			if callCount == 2 {
				return nil, errors.New("AI service error")
			}
			return &aiv1.EmbeddingResponse{
				Vector:     make([]float32, 1024),
				Dimensions: 1024,
				ModelUsed:  "mxbai-embed-large-v1",
			}, nil
		},
	}

	mockRepo := &mlMockEmbeddingRepo{
		storeMultiLevelIDs: []int64{100, 101, 102},
		getEmbeddingMap: map[int64]*Embedding{
			100: {
				ID:           100,
				Model:        "mxbai-embed-large-v1",
				ModelVersion: "mxbai-embed-large-v1",
			},
		},
	}

	activities := NewMultiLevelEmbeddingActivities(logger, mockClient, mockRepo, nil)

	input := GenerateMultiLevelInput{
		TenantID:    "test-tenant",
		SourceID:    42,
		Content:     "Content with actions",
		ContentType: "email",
		Analysis: &DeepAnalyzeOutput{
			VerifiedActions: []VerifiedActionOutput{
				{Description: "Action 1", Status: "confirmed"},
				{Description: "Action 2", Status: "confirmed"},
			},
		},
		AssertionIDs: map[string]int64{
			"action:0": 200,
			"action:1": 201,
		},
	}

	output, err := activities.GenerateMultiLevelEmbeddings(context.Background(), input)
	require.NoError(t, err) // Activity should not fail
	require.NotNil(t, output)

	// Should complete with 1 failure
	require.Equal(t, 2, output.TotalGenerated) // content + action 2
	require.Equal(t, 1, output.TotalFailed)    // action 1 failed
	require.Len(t, output.SourceEmbeddingIDs, 1)
	require.Len(t, output.AssertionEmbeddingIDs, 1)

	// Verify 2 embeddings were stored (content + action 2)
	require.Len(t, mockRepo.storeMultiLevelCalls, 2)
}

func TestGenerateMultiLevelEmbeddings_ValidationErrors(t *testing.T) {
	logger := logging.NewNopLogger()

	mockClient := &mockAIClient{}
	mockRepo := &mlMockEmbeddingRepo{}

	activities := NewMultiLevelEmbeddingActivities(logger, mockClient, mockRepo, nil)

	// Test empty content
	input := GenerateMultiLevelInput{
		TenantID: "test-tenant",
		SourceID: 42,
		Content:  "", // Empty
		Analysis: &DeepAnalyzeOutput{},
	}

	output, err := activities.GenerateMultiLevelEmbeddings(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, output)
	require.Contains(t, err.Error(), "content is empty")

	// Test nil analysis
	input2 := GenerateMultiLevelInput{
		TenantID: "test-tenant",
		SourceID: 42,
		Content:  "Some content",
		Analysis: nil, // Nil
	}

	output2, err2 := activities.GenerateMultiLevelEmbeddings(context.Background(), input2)
	require.Error(t, err2)
	require.Nil(t, output2)
	require.Contains(t, err2.Error(), "analysis is required")
}

func TestReEmbedBatch_ProcessesStale(t *testing.T) {
	logger := logging.NewNopLogger()

	mockClient := &mockAIClient{
		generateEmbeddingFn: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			return &aiv1.EmbeddingResponse{
				Vector:     make([]float32, 1024),
				Dimensions: 1024,
				ModelUsed:  "mxbai-embed-large-v1",
			}, nil
		},
	}

	mockRepo := &mlMockEmbeddingRepo{
		getStaleSources:    []int64{1, 2, 3}, // 3 stale sources
		storeMultiLevelIDs: []int64{100, 101, 102},
	}

	mockSourceRepo := &mockSourceRepo{
		sources: map[int64]*Source{
			1: {ID: 1, ContentText: "Content 1"},
			2: {ID: 2, ContentText: "Content 2"},
			3: {ID: 3, ContentText: "Content 3"},
		},
	}

	activities := NewMultiLevelEmbeddingActivities(logger, mockClient, mockRepo, mockSourceRepo)

	input := ReEmbedBatchInput{
		TenantID:       "test-tenant",
		CurrentModel:   "new-model",
		CurrentVersion: "2.0",
		BatchSize:      3,
	}

	output, err := activities.ReEmbedBatch(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// All 3 sources should be processed
	require.Equal(t, 3, output.SourcesProcessed)
	require.Equal(t, 0, output.SourcesRemaining)
	require.Equal(t, 0, output.Errors)

	// Verify delete was called 3 times
	require.Len(t, mockRepo.deleteForSourceCalls, 3)
	require.Equal(t, int64(1), mockRepo.deleteForSourceCalls[0])
	require.Equal(t, int64(2), mockRepo.deleteForSourceCalls[1])
	require.Equal(t, int64(3), mockRepo.deleteForSourceCalls[2])

	// Verify 3 embeddings were stored
	require.Len(t, mockRepo.storeMultiLevelCalls, 3)
}

func TestReEmbedBatch_SourcesRemaining(t *testing.T) {
	logger := logging.NewNopLogger()

	mockClient := &mockAIClient{
		generateEmbeddingFn: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			return &aiv1.EmbeddingResponse{
				Vector:     make([]float32, 1024),
				Dimensions: 1024,
				ModelUsed:  "mxbai-embed-large-v1",
			}, nil
		},
	}

	// Return 4 sources (batch_size + 1) to indicate more remain
	mockRepo := &mlMockEmbeddingRepo{
		getStaleSources:    []int64{1, 2, 3, 4}, // 4 sources (batch size is 3)
		storeMultiLevelIDs: []int64{100, 101, 102},
	}

	mockSourceRepo := &mockSourceRepo{
		sources: map[int64]*Source{
			1: {ID: 1, ContentText: "Content 1"},
			2: {ID: 2, ContentText: "Content 2"},
			3: {ID: 3, ContentText: "Content 3"},
			4: {ID: 4, ContentText: "Content 4"},
		},
	}

	activities := NewMultiLevelEmbeddingActivities(logger, mockClient, mockRepo, mockSourceRepo)

	input := ReEmbedBatchInput{
		TenantID:       "test-tenant",
		CurrentModel:   "new-model",
		CurrentVersion: "2.0",
		BatchSize:      3,
	}

	output, err := activities.ReEmbedBatch(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Should process only 3 and report 1 remaining
	require.Equal(t, 3, output.SourcesProcessed)
	require.Equal(t, 1, output.SourcesRemaining)
	require.Equal(t, 0, output.Errors)
}

func TestReEmbedBatch_ValidationErrors(t *testing.T) {
	logger := logging.NewNopLogger()

	mockClient := &mockAIClient{}
	mockRepo := &mlMockEmbeddingRepo{}
	mockSourceRepo := &mockSourceRepo{}

	activities := NewMultiLevelEmbeddingActivities(logger, mockClient, mockRepo, mockSourceRepo)

	// Test empty tenant_id
	input := ReEmbedBatchInput{
		TenantID:       "",
		CurrentModel:   "model",
		CurrentVersion: "1.0",
		BatchSize:      10,
	}

	output, err := activities.ReEmbedBatch(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, output)
	require.Contains(t, err.Error(), "tenant_id is required")

	// Test empty current_model
	input2 := ReEmbedBatchInput{
		TenantID:       "test-tenant",
		CurrentModel:   "",
		CurrentVersion: "1.0",
		BatchSize:      10,
	}

	output2, err2 := activities.ReEmbedBatch(context.Background(), input2)
	require.Error(t, err2)
	require.Nil(t, output2)
	require.Contains(t, err2.Error(), "current_model is required")

	// Test invalid batch_size
	input3 := ReEmbedBatchInput{
		TenantID:       "test-tenant",
		CurrentModel:   "model",
		CurrentVersion: "1.0",
		BatchSize:      0,
	}

	output3, err3 := activities.ReEmbedBatch(context.Background(), input3)
	require.Error(t, err3)
	require.Nil(t, output3)
	require.Contains(t, err3.Error(), "batch_size must be positive")
}

// TestGenerateMultiLevelEmbeddings_ChunksLongContent verifies that content over
// the chunking token limit produces MULTIPLE content embeddings, one per chunk.
// This test will FAIL until the chunking implementation is added.
func TestGenerateMultiLevelEmbeddings_ChunksLongContent(t *testing.T) {
	logger := logging.NewNopLogger()

	// Track all AI client embedding calls
	var embeddingCalls []string
	mockClient := &mockAIClient{
		generateEmbeddingFn: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			embeddingCalls = append(embeddingCalls, req.Text)
			return &aiv1.EmbeddingResponse{
				Vector:     make([]float32, 1024),
				Dimensions: 1024,
				ModelUsed:  "mxbai-embed-large-v1",
			}, nil
		},
	}

	mockRepo := &mlMockEmbeddingRepo{
		storeMultiLevelIDs: []int64{100, 101, 102, 103, 104, 105, 106, 107},
		getEmbeddingMap: map[int64]*Embedding{
			100: {
				ID:           100,
				Model:        "mxbai-embed-large-v1",
				ModelVersion: "mxbai-embed-large-v1",
			},
		},
	}

	activities := NewMultiLevelEmbeddingActivities(logger, mockClient, mockRepo, nil)

	// Create long content (~2500 chars = ~625 tokens at chars/4 estimate)
	// This is over 400 tokens (chunking threshold) and over 512 tokens (embedding model limit)
	longContent := `This is a test email about our quarterly planning process. We need to coordinate across multiple teams to ensure alignment on key initiatives. The product team has identified several critical features that need to be delivered in Q2, and we need engineering capacity to support these initiatives.

First, let's discuss the user authentication overhaul. This has been on our roadmap for six months, and we can't delay it any longer. The current system is showing signs of strain, and we've had three security incidents in the last quarter that could have been prevented with better auth infrastructure.

Second, the data pipeline refactoring is becoming urgent. Our ETL jobs are taking increasingly long to complete, and we're starting to miss SLA commitments to enterprise customers. The engineering team estimates this will take 8-10 weeks of focused effort with a team of three senior engineers.

Third, we need to address technical debt in the frontend codebase. The migration to the new component library has stalled at 60% completion, and this is creating maintenance burden and slowing down feature development. We should allocate dedicated time to finish this migration.

Fourth, the mobile app needs performance improvements. Load times have increased by 40% over the last three releases, and our app store ratings are starting to reflect user frustration. This should be prioritized alongside the auth work.

Finally, we need to invest in observability and monitoring. The recent outages revealed gaps in our ability to diagnose production issues quickly. A two-week sprint focused on improving our telemetry, dashboards, and alerting would pay dividends in reduced MTTR.

In summary, we have five major initiatives competing for limited engineering resources. We need to make hard choices about prioritization and sequencing. Let's schedule a planning session for next week to work through the tradeoffs and commit to a realistic roadmap for Q2.`

	input := GenerateMultiLevelInput{
		TenantID:    "test-tenant",
		SourceID:    42,
		ContentID:   "em-longtest",
		Content:     longContent,
		ContentType: "email",
		Analysis: &DeepAnalyzeOutput{
			Summary: "Summary of the long content",
			VerifiedActions: []VerifiedActionOutput{
				{Description: "Schedule planning session", Status: "confirmed"},
			},
		},
		AssertionIDs: map[string]int64{
			"action:0": 200,
		},
	}

	output, err := activities.GenerateMultiLevelEmbeddings(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// CRITICAL ASSERTION: The current implementation produces 3 total embeddings:
	// 1 content (truncated) + 1 summary + 1 action
	// After chunking implementation, we expect MORE than 3:
	// N content chunks (should be 2-3 chunks for ~2500 chars) + 1 summary + 1 action
	//
	// This assertion will FAIL now because total_generated will be 3, not >= 4
	require.GreaterOrEqual(t, output.TotalGenerated, 4,
		"Expected at least 4 embeddings (2+ content chunks + summary + action), got %d",
		output.TotalGenerated)

	// Verify we got multiple content embeddings (chunks)
	contentEmbeddingCount := 0
	for _, call := range mockRepo.storeMultiLevelCalls {
		if call.RepresentationType == "content" {
			contentEmbeddingCount++
		}
	}
	// This will FAIL - current implementation produces 1 content embedding
	require.GreaterOrEqual(t, contentEmbeddingCount, 2,
		"Expected at least 2 content chunk embeddings for long content, got %d",
		contentEmbeddingCount)

	// Verify chunk metadata on stored embeddings
	chunkIndex := 0
	for _, call := range mockRepo.storeMultiLevelCalls {
		if call.RepresentationType == "content" {
			require.Equal(t, chunkIndex, call.ChunkIndex,
				"Expected chunk_index=%d for content chunk %d", chunkIndex, chunkIndex)
			require.Greater(t, call.ChunkTotal, 1,
				"Expected chunk_total > 1 for chunked content")
			chunkIndex++
		}
	}
}

// TestGenerateMultiLevelEmbeddings_ShortContentSingleChunk verifies backward
// compatibility: short content (under MaxTokens) produces exactly 1 content
// embedding with chunk_index=0 and chunk_total=1.
func TestGenerateMultiLevelEmbeddings_ShortContentSingleChunk(t *testing.T) {
	logger := logging.NewNopLogger()

	mockClient := &mockAIClient{
		generateEmbeddingFn: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			return &aiv1.EmbeddingResponse{
				Vector:     make([]float32, 1024),
				Dimensions: 1024,
				ModelUsed:  "mxbai-embed-large-v1",
			}, nil
		},
	}

	mockRepo := &mlMockEmbeddingRepo{
		storeMultiLevelIDs: []int64{100, 101, 102},
		getEmbeddingMap: map[int64]*Embedding{
			100: {
				ID:           100,
				Model:        "mxbai-embed-large-v1",
				ModelVersion: "mxbai-embed-large-v1",
			},
		},
	}

	activities := NewMultiLevelEmbeddingActivities(logger, mockClient, mockRepo, nil)

	// Short content (~200 chars = ~50 tokens, well under 400 token threshold)
	shortContent := "This is a brief email about tomorrow's standup. Let's meet at 9am in the main conference room. Please bring your status updates and any blockers you're facing."

	input := GenerateMultiLevelInput{
		TenantID:    "test-tenant",
		SourceID:    42,
		ContentID:   "em-shorttest",
		Content:     shortContent,
		ContentType: "email",
		Analysis: &DeepAnalyzeOutput{
			Summary: "Brief standup reminder",
			VerifiedActions: []VerifiedActionOutput{
				{Description: "Attend standup", Status: "confirmed"},
			},
		},
		AssertionIDs: map[string]int64{
			"action:0": 200,
		},
	}

	output, err := activities.GenerateMultiLevelEmbeddings(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Should produce exactly 3 embeddings: 1 content + 1 summary + 1 action
	require.Equal(t, 3, output.TotalGenerated)

	// Verify exactly 1 content embedding
	contentEmbeddings := []MultiLevelEmbeddingInput{}
	for _, call := range mockRepo.storeMultiLevelCalls {
		if call.RepresentationType == "content" {
			contentEmbeddings = append(contentEmbeddings, call)
		}
	}
	require.Len(t, contentEmbeddings, 1, "Expected exactly 1 content embedding for short content")

	// Verify chunk metadata indicates single chunk
	require.Equal(t, 0, contentEmbeddings[0].ChunkIndex,
		"Expected chunk_index=0 for single chunk")
	require.Equal(t, 1, contentEmbeddings[0].ChunkTotal,
		"Expected chunk_total=1 for single chunk")
}

// TestGenerateEmbedding_ChunksLongContent verifies that the single-path
// GenerateEmbedding activity also chunks long content into multiple embeddings.
//
// NOTE: This test is currently skipped because GenerateEmbedding calls RecordHeartbeat
// which requires a Temporal activity context. After implementation, this test should be
// moved to embedding_test.go with proper Temporal test harness OR the implementation
// should be refactored to make the chunking logic testable without Temporal context.
func TestGenerateEmbedding_ChunksLongContent(t *testing.T) {
	t.Skip("Skipping - requires Temporal activity context for RecordHeartbeat. Move to embedding_test.go with proper test harness after implementation.")

	logger := logging.NewNopLogger()

	var embeddingCalls []string
	mockClient := &mockAIClient{
		generateEmbeddingFn: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			embeddingCalls = append(embeddingCalls, req.Text)
			return &aiv1.EmbeddingResponse{
				Vector:     make([]float32, 1024),
				Dimensions: 1024,
				ModelUsed:  "mxbai-embed-large-v1",
			}, nil
		},
	}

	// Track StoreMultiLevelEmbedding calls
	mockRepo := &mlMockEmbeddingRepo{
		storeMultiLevelIDs: []int64{100, 101, 102},
	}

	// Create embedding activities (not multilevel)
	embeddingActivities := NewEmbeddingActivities(logger, mockClient, mockRepo, nil)

	// Long content that should be chunked
	longContent := `This is a test email about our quarterly planning process. We need to coordinate across multiple teams to ensure alignment on key initiatives. The product team has identified several critical features that need to be delivered in Q2, and we need engineering capacity to support these initiatives.

First, let's discuss the user authentication overhaul. This has been on our roadmap for six months, and we can't delay it any longer. The current system is showing signs of strain, and we've had three security incidents in the last quarter that could have been prevented with better auth infrastructure.

Second, the data pipeline refactoring is becoming urgent. Our ETL jobs are taking increasingly long to complete, and we're starting to miss SLA commitments to enterprise customers. The engineering team estimates this will take 8-10 weeks of focused effort with a team of three senior engineers.

Third, we need to address technical debt in the frontend codebase. The migration to the new component library has stalled at 60% completion, and this is creating maintenance burden and slowing down feature development. We should allocate dedicated time to finish this migration.

Fourth, the mobile app needs performance improvements. Load times have increased by 40% over the last three releases, and our app store ratings are starting to reflect user frustration. This should be prioritized alongside the auth work.`

	// Note: The current GenerateEmbedding signature returns a single int64 embedding ID
	// After implementation, it should either:
	// 1. Return the first chunk's ID (for backward compatibility), OR
	// 2. Change signature to return []int64
	//
	// For now, this test will FAIL because we're checking the mock repo calls
	embeddingID, err := embeddingActivities.GenerateEmbedding(context.Background(), workflows.GenerateEmbeddingInput{
		TenantID:    "test-tenant",
		SourceID:    42,
		Content:     longContent,
		ContentHash: "hash123",
	})
	require.NoError(t, err)
	require.NotZero(t, embeddingID)

	// Verify multiple StoreEmbedding or StoreMultiLevelEmbedding calls were made
	// This will FAIL - current implementation only calls StoreEmbedding once
	require.GreaterOrEqual(t, len(mockRepo.storeMultiLevelCalls), 2,
		"Expected at least 2 store calls for chunked content, got %d",
		len(mockRepo.storeMultiLevelCalls))

	// TODO: After ChunkIndex and ChunkTotal fields are added to MultiLevelEmbeddingInput:
	// Verify chunk metadata is set correctly
	// for i, call := range mockRepo.storeMultiLevelCalls {
	// 	require.Equal(t, i, call.ChunkIndex,
	// 		"Expected chunk_index=%d for chunk %d", i, i)
	// 	require.Greater(t, call.ChunkTotal, 1,
	// 		"Expected chunk_total > 1 for chunked content")
	// }
}
