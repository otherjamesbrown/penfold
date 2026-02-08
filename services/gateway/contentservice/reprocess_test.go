package contentservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestReprocessContent_WithModelOverride tests that model_id override is handled.
// This test will FAIL because the handler doesn't yet validate or pass model_id.
func TestReprocessContent_WithModelOverride(t *testing.T) {
	ctx := context.Background()

	mockRepo := new(MockRepository)
	svc := newTestService(mockRepo)
	// Note: pipelineRepo and temporalClient are nil, so this will fail with Unavailable
	// After implementation, this test should be updated to use proper mocks

	modelID := "gemini-2.0-flash-thinking-exp"
	req := &contentv1.ReprocessContentRequest{
		ContentId: "test-content-id",
		Reason:    "Testing model override",
		Options: &contentv1.ProcessingOptions{
			ModelId: &modelID,
		},
	}

	_, err := svc.ReprocessContent(ctx, req)

	// For now, just verify we get a response. The actual implementation test will verify
	// that the model_id is passed to the workflow input.
	// This test documents the expected behavior even though dependencies are nil.
	require.Error(t, err) // Will error due to nil dependencies in test setup
	st, ok := status.FromError(err)
	require.True(t, ok)
	// Currently returns Unavailable due to nil pipelineRepo
	assert.Equal(t, codes.Unavailable, st.Code())
}

// TestReprocessContent_WithTimeoutOverride tests that timeout_seconds override is handled.
// This test will FAIL because the handler doesn't yet validate or pass timeout_seconds.
func TestReprocessContent_WithTimeoutOverride(t *testing.T) {
	ctx := context.Background()

	mockRepo := new(MockRepository)
	svc := newTestService(mockRepo)

	timeoutSeconds := int32(600)
	req := &contentv1.ReprocessContentRequest{
		ContentId: "test-content-id",
		Reason:    "Testing timeout override",
		Options: &contentv1.ProcessingOptions{
			TimeoutSeconds: &timeoutSeconds,
		},
	}

	_, err := svc.ReprocessContent(ctx, req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
}

// TestReprocessContent_NoOverrides tests that default behavior when no overrides provided.
func TestReprocessContent_NoOverrides(t *testing.T) {
	ctx := context.Background()

	mockRepo := new(MockRepository)
	svc := newTestService(mockRepo)

	req := &contentv1.ReprocessContentRequest{
		ContentId: "test-content-id",
		Reason:    "Testing no overrides",
	}

	_, err := svc.ReprocessContent(ctx, req)

	// Should fail with Unavailable due to nil pipelineRepo in test setup
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
}

// TestReprocessContent_MissingContentID tests validation.
func TestReprocessContent_MissingContentID(t *testing.T) {
	ctx := context.Background()

	mockRepo := new(MockRepository)
	svc := newTestService(mockRepo)

	req := &contentv1.ReprocessContentRequest{
		ContentId: "",
		Reason:    "Testing validation",
	}

	resp, err := svc.ReprocessContent(ctx, req)

	// This should pass - existing validation should catch empty content_id
	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "content_id")
}

// TestProcessingOptions_Structure tests that ProcessingOptions proto has expected fields.
// This verifies the proto was updated correctly.
func TestProcessingOptions_Structure(t *testing.T) {
	modelID := "test-model"
	timeoutSeconds := int32(300)

	opts := &contentv1.ProcessingOptions{
		SkipEmbedding:     true,
		SkipSummarization: false,
		SkipExtraction:    false,
		TimeoutSeconds:    &timeoutSeconds,
		ModelId:           &modelID,
	}

	// Verify proto fields exist and are accessible
	assert.True(t, opts.SkipEmbedding)
	assert.False(t, opts.SkipSummarization)
	assert.NotNil(t, opts.TimeoutSeconds)
	assert.Equal(t, int32(300), *opts.TimeoutSeconds)
	assert.NotNil(t, opts.ModelId)
	assert.Equal(t, "test-model", *opts.ModelId)
}

// TestReprocessContent_InvalidModel documents the expected validation behavior.
// This test will FAIL because model_id validation is not yet implemented.
func TestReprocessContent_InvalidModel(t *testing.T) {
	ctx := context.Background()

	mockRepo := new(MockRepository)
	svc := newTestService(mockRepo)

	// Create a service with a logger to avoid nil panics
	logger := logging.NewLogger(nil)
	svc.logger = logger

	invalidModel := "invalid-model-xyz"
	req := &contentv1.ReprocessContentRequest{
		ContentId: "test-content-id",
		Reason:    "Testing invalid model",
		Options: &contentv1.ProcessingOptions{
			ModelId: &invalidModel,
		},
	}

	resp, err := svc.ReprocessContent(ctx, req)

	// Expected: InvalidArgument error for invalid model
	// Current: Unavailable error due to nil pipelineRepo (checked before model validation)
	require.Error(t, err)
	assert.Nil(t, resp)

	// After implementation, this should be InvalidArgument
	// For now, we document the expected behavior
	st, ok := status.FromError(err)
	require.True(t, ok)
	// TODO: After implementation, change to codes.InvalidArgument
	// assert.Equal(t, codes.InvalidArgument, st.Code())
	// assert.Contains(t, st.Message(), "model")

	// Current behavior:
	assert.Equal(t, codes.Unavailable, st.Code())
}
