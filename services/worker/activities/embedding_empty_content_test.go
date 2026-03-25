package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// mockCapturingEmbedPipelineRepo captures CreateRun calls for embedding skip tests.
type mockCapturingEmbedPipelineRepo struct {
	calls []PipelineRunInput
}

func (m *mockCapturingEmbedPipelineRepo) CreateRun(_ context.Context, input PipelineRunInput) error {
	m.calls = append(m.calls, input)
	return nil
}

func (m *mockCapturingEmbedPipelineRepo) RecordOverrides(_ context.Context, _ int64, _ map[string]string) error {
	return nil
}

func (m *mockCapturingEmbedPipelineRepo) GetContextProviders(_ context.Context, _, _, _ string) ([]string, error) {
	return nil, nil
}

// TestGenerateEmbedding_EmptyContent verifies that empty content results in a skip (0, nil)
// and records a skipped pipeline run.
func TestGenerateEmbedding_EmptyContent(t *testing.T) {
	logger := logging.NewNopLogger()
	pipelineRepo := &mockCapturingEmbedPipelineRepo{}
	a := NewEmbeddingActivities(logger, &mockAIClient{}, &mockEmbeddingRepository{}, pipelineRepo, nil)

	id, err := a.GenerateEmbedding(context.Background(), workflows.GenerateEmbeddingInput{
		TenantID: "tenant-1",
		SourceID: 42,
		Content:  "",
	})

	require.NoError(t, err)
	assert.Equal(t, int64(0), id)
	require.Len(t, pipelineRepo.calls, 1)
	assert.Equal(t, "embed", pipelineRepo.calls[0].Stage)
	assert.Equal(t, "skipped", pipelineRepo.calls[0].Status)
	assert.Equal(t, "content_empty", pipelineRepo.calls[0].SkipReason)
}

// TestGenerateEmbedding_EmptyContent_NoPipelineRepo verifies that empty content skip
// works even when pipelineRepo is nil (optional dependency).
func TestGenerateEmbedding_EmptyContent_NoPipelineRepo(t *testing.T) {
	logger := logging.NewNopLogger()
	a := NewEmbeddingActivities(logger, &mockAIClient{}, &mockEmbeddingRepository{}, nil, nil)

	id, err := a.GenerateEmbedding(context.Background(), workflows.GenerateEmbeddingInput{
		TenantID: "tenant-1",
		SourceID: 42,
		Content:  "",
	})

	require.NoError(t, err)
	assert.Equal(t, int64(0), id)
}

// TestGenerateMultiLevelEmbeddings_EmptyContent verifies that empty content returns
// an empty successful output rather than an error.
func TestGenerateMultiLevelEmbeddings_EmptyContent(t *testing.T) {
	logger := logging.NewNopLogger()
	a := NewMultiLevelEmbeddingActivities(
		logger,
		&mockAIClient{},
		&mlMockEmbeddingRepo{},
		&mockSourceRepo{},
	)

	out, err := a.GenerateMultiLevelEmbeddings(context.Background(), GenerateMultiLevelInput{
		TenantID:    "tenant-1",
		SourceID:    42,
		Content:     "",
		ContentType: "email",
	})

	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Empty(t, out.SourceEmbeddingIDs)
	assert.Empty(t, out.AssertionEmbeddingIDs)
	assert.Equal(t, 0, out.TotalGenerated)
	assert.Equal(t, 0, out.TotalFailed)
}
