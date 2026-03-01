package resolver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// TestAICoordinatorProvider_UsesGRPCClient verifies that the AI coordinator provider
// uses gRPC calls instead of direct HTTP calls to Ollama.
func TestAICoordinatorProvider_UsesGRPCClient(t *testing.T) {
	// This test verifies that the provider implementation uses a gRPC client
	// rather than an HTTP client for making AI coordinator calls.

	// Create a mock gRPC connection
	// In a real test, this would be a grpc.ClientConn to a test server
	// For this test, we just verify the provider stores it correctly
	mockConn := &grpc.ClientConn{} // Minimal mock for structure test

	// Create AI coordinator provider with gRPC client
	provider := NewAICoordinatorProvider(AICoordinatorConfig{
		GRPCConn: mockConn,
		Model:    "qwen2.5:7b-instruct",
	})

	// Verify the provider has a gRPC client, not an HTTP client
	assert.NotNil(t, provider, "Provider should be created")
	assert.IsType(t, &AICoordinatorProvider{}, provider, "Should be AICoordinatorProvider type")

	// Verify it has a gRPC client field
	assert.NotNil(t, provider.grpcClient, "Provider should have gRPC client")
	assert.Equal(t, "ai-coordinator-qwen2.5:7b-instruct", provider.Name(), "Name should include model")
}

// TestAICoordinatorProvider_TranslatesCompletionRequest verifies the provider
// correctly translates CompletionRequest to gRPC calls.
func TestAICoordinatorProvider_TranslatesCompletionRequest(t *testing.T) {
	t.Skip("Integration test - requires AI coordinator gRPC server running")

	ctx := context.Background()

	// Create a provider with a connection (implementation will use aiv1.AICoordinatorServiceClient)
	provider := NewAICoordinatorProvider(AICoordinatorConfig{
		GRPCConn: nil, // Would be a real connection in implementation
		Model:    "qwen2.5:7b-instruct",
	})

	// Make a completion request
	req := CompletionRequest{
		Prompt:       "What is mention resolution?",
		SystemPrompt: "You are a helpful assistant for entity resolution.",
		JSONMode:     false,
	}

	// This will fail until Complete method is implemented
	resp, err := provider.Complete(ctx, req)

	require.NoError(t, err, "Complete should succeed")
	assert.NotNil(t, resp, "Response should be returned")
	assert.NotEmpty(t, resp.Content, "Response should have content")
	assert.Equal(t, "qwen2.5:7b-instruct", resp.Model, "Model should match")
}

// TestAICoordinatorProvider_HandlesJSONMode verifies the provider correctly
// handles JSONMode flag when making gRPC calls.
func TestAICoordinatorProvider_HandlesJSONMode(t *testing.T) {
	t.Skip("Integration test - requires AI coordinator gRPC server running")

	ctx := context.Background()

	provider := NewAICoordinatorProvider(AICoordinatorConfig{
		GRPCConn: nil, // Would be a real connection in implementation
		Model:    "qwen2.5:7b-instruct",
	})

	// Make a structured completion request with JSON mode
	req := CompletionRequest{
		Prompt:       "Extract entities",
		SystemPrompt: "Return JSON only",
		JSONMode:     true,
	}

	resp, err := provider.Complete(ctx, req)

	require.NoError(t, err, "Complete should succeed")
	assert.NotEmpty(t, resp.Content, "Response should have content")
	// Verify the response is valid JSON
	assert.Contains(t, resp.Content, "{", "Response should be JSON format")
}

// TestAICoordinatorProvider_MentionResolutionHappyPath verifies the full flow
// of using the AI coordinator provider for mention resolution.
func TestAICoordinatorProvider_MentionResolutionHappyPath(t *testing.T) {
	t.Skip("Integration test - requires AI coordinator gRPC server running")

	ctx := context.Background()

	provider := NewAICoordinatorProvider(AICoordinatorConfig{
		GRPCConn: nil, // Would be a real connection in implementation
		Model:    "qwen2.5:7b-instruct",
	})

	// Make a structured completion request (as would be done in mention resolution)
	var result Stage1Understanding
	req := CompletionRequest{
		Prompt:       "Extract mentions from: John sent the email",
		SystemPrompt: "You are an entity extraction assistant.",
		JSONMode:     true,
	}

	err := provider.CompleteStructured(ctx, req, &result)

	require.NoError(t, err, "CompleteStructured should succeed")
	assert.Len(t, result.Mentions, 1, "Should extract one mention")
	assert.Equal(t, "John", result.Mentions[0].Text, "Mention text should match")
	assert.Equal(t, "person", string(result.Mentions[0].EntityType), "Entity type should be person")
}

// TestAICoordinatorProvider_SupportsLangfuseTracing verifies that when the provider
// makes gRPC calls, Langfuse tracing metadata is included for observability.
func TestAICoordinatorProvider_SupportsLangfuseTracing(t *testing.T) {
	t.Skip("Integration test - requires AI coordinator gRPC server running")

	ctx := context.Background()

	provider := NewAICoordinatorProvider(AICoordinatorConfig{
		GRPCConn: nil, // Would be a real connection in implementation
		Model:    "qwen2.5:7b-instruct",
	})

	// Make a completion request with tracing metadata
	req := CompletionRequest{
		Prompt:   "Test prompt",
		TraceID:  "trace_abc123",
		StageNum: 1,
	}

	_, err := provider.Complete(ctx, req)

	require.NoError(t, err, "Complete should succeed with tracing metadata")
	// In the actual implementation, this would verify that the gRPC request
	// included content_id and tracing metadata propagated via OTel interceptors
}

// TestAICoordinatorProvider_PanicsWithoutConnection verifies that the provider
// requires a valid gRPC connection.
func TestAICoordinatorProvider_PanicsWithoutConnection(t *testing.T) {
	// Provider should panic if created without a connection
	assert.Panics(t, func() {
		NewAICoordinatorProvider(AICoordinatorConfig{
			GRPCConn: nil,
			Model:    "qwen2.5:7b-instruct",
		})
	}, "Should panic when GRPCConn is nil")
}
