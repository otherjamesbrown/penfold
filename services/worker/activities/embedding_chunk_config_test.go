package activities

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockConfigReader implements OperationalConfigReader for testing.
type mockConfigReader struct {
	values map[string]string
	err    error
}

func (m *mockConfigReader) GetString(_ context.Context, tenantID, key string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	v, ok := m.values[key]
	if !ok {
		return "", fmt.Errorf("config key %q not found for tenant %s", key, tenantID)
	}
	return v, nil
}

func TestGetChunkConfig_Defaults(t *testing.T) {
	logger := logging.NewNopLogger()
	a := NewEmbeddingActivities(logger, &mockAIClient{}, &mockEmbeddingRepository{}, nil, nil)

	maxTokens, overlapTokens := a.getChunkConfig(context.Background(), "tenant-1")
	assert.Equal(t, 400, maxTokens)
	assert.Equal(t, 50, overlapTokens)
}

func TestGetChunkConfig_FromDB(t *testing.T) {
	logger := logging.NewNopLogger()
	reader := &mockConfigReader{
		values: map[string]string{
			ConfigKeyChunkMaxTokens:     "200",
			ConfigKeyChunkOverlapTokens: "25",
		},
	}
	a := NewEmbeddingActivities(logger, &mockAIClient{}, &mockEmbeddingRepository{}, nil, reader)

	maxTokens, overlapTokens := a.getChunkConfig(context.Background(), "tenant-1")
	assert.Equal(t, 200, maxTokens)
	assert.Equal(t, 25, overlapTokens)
}

func TestGetChunkConfig_PartialConfig(t *testing.T) {
	logger := logging.NewNopLogger()
	reader := &mockConfigReader{
		values: map[string]string{
			ConfigKeyChunkMaxTokens: "256",
		},
	}
	a := NewEmbeddingActivities(logger, &mockAIClient{}, &mockEmbeddingRepository{}, nil, reader)

	maxTokens, overlapTokens := a.getChunkConfig(context.Background(), "tenant-1")
	assert.Equal(t, 256, maxTokens)
	assert.Equal(t, 50, overlapTokens, "should fall back to default overlap")
}

func TestGetChunkConfig_InvalidValue(t *testing.T) {
	logger := logging.NewNopLogger()
	reader := &mockConfigReader{
		values: map[string]string{
			ConfigKeyChunkMaxTokens:     "not-a-number",
			ConfigKeyChunkOverlapTokens: "25",
		},
	}
	a := NewEmbeddingActivities(logger, &mockAIClient{}, &mockEmbeddingRepository{}, nil, reader)

	maxTokens, overlapTokens := a.getChunkConfig(context.Background(), "tenant-1")
	assert.Equal(t, 400, maxTokens, "should fall back to default on invalid value")
	assert.Equal(t, 25, overlapTokens)
}

func TestGetChunkConfig_DBError(t *testing.T) {
	logger := logging.NewNopLogger()
	reader := &mockConfigReader{
		err: fmt.Errorf("connection refused"),
	}
	a := NewEmbeddingActivities(logger, &mockAIClient{}, &mockEmbeddingRepository{}, nil, reader)

	maxTokens, overlapTokens := a.getChunkConfig(context.Background(), "tenant-1")
	assert.Equal(t, 400, maxTokens, "should fall back to default on error")
	assert.Equal(t, 50, overlapTokens, "should fall back to default on error")
}
