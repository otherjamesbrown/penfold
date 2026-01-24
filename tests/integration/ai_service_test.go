//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// getAIServiceAddr returns the AI service address from environment or default.
func getAIServiceAddr() string {
	addr := os.Getenv("AI_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:50055"
	}
	return addr
}

// skipIfAIServiceUnavailable skips the test if the AI service is not available.
func skipIfAIServiceUnavailable(t *testing.T) *ai.Client {
	t.Helper()

	addr := getAIServiceAddr()
	client, err := ai.NewClient(addr,
		ai.WithConnectTimeout(2*time.Second),
		ai.WithDialOptions(grpc.WithBlock()),
	)
	if err != nil {
		t.Skipf("AI service not available at %s: %v", addr, err)
	}

	// Verify with health check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.HealthCheck(ctx); err != nil {
		client.Close()
		t.Skipf("AI service health check failed: %v", err)
	}

	t.Cleanup(func() {
		client.Close()
	})

	return client
}

// TestAIService_GenerateEmbedding tests the embedding generation endpoint.
func TestAIService_GenerateEmbedding(t *testing.T) {
	client := skipIfAIServiceUnavailable(t)
	ctx := context.Background()

	t.Run("successful embedding generation", func(t *testing.T) {
		resp, err := client.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
			Text: "Hello, this is a test message for embedding generation.",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify embedding dimensions (MLX typically uses 1024 dimensions)
		assert.Greater(t, resp.Dimensions, int32(0), "embedding should have positive dimensions")
		assert.Len(t, resp.Vector, int(resp.Dimensions), "vector length should match dimensions")

		// Verify model was used
		assert.NotEmpty(t, resp.ModelUsed, "model_used should be set")

		t.Logf("Embedding generated: dimensions=%d, model=%s", resp.Dimensions, resp.ModelUsed)
	})

	t.Run("embedding with specific model", func(t *testing.T) {
		model := "mxbai-embed-large-v1"
		resp, err := client.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
			Text:  "Testing with specific model.",
			Model: &model,
		})
		// Model may or may not be available - just verify the response format
		if err == nil {
			require.NotNil(t, resp)
			assert.Greater(t, resp.Dimensions, int32(0))
		} else {
			t.Logf("Model %s not available (expected in some environments): %v", model, err)
		}
	})

	t.Run("embedding with tenant ID", func(t *testing.T) {
		tenantID := "test-tenant-123"
		resp, err := client.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
			Text:     "Testing with tenant ID for isolation.",
			TenantId: &tenantID,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Greater(t, resp.Dimensions, int32(0))
	})

	t.Run("empty text returns error", func(t *testing.T) {
		_, err := client.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
			Text: "",
		})
		require.Error(t, err)
	})

	t.Run("whitespace-only text returns error", func(t *testing.T) {
		_, err := client.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
			Text: "   \t\n   ",
		})
		require.Error(t, err)
	})
}

// TestAIService_GenerateSummary tests the summary generation endpoint.
func TestAIService_GenerateSummary(t *testing.T) {
	client := skipIfAIServiceUnavailable(t)
	ctx := context.Background()

	testContent := `The quarterly report shows significant growth in several key areas.
Revenue increased by 15% compared to the previous quarter, driven primarily by
new customer acquisitions in the enterprise segment. Operating expenses remained
flat, resulting in improved profit margins. The engineering team successfully
launched three major product features, which received positive feedback from
users. Looking ahead, the company plans to expand into two new markets and
expects continued growth through the end of the fiscal year.`

	t.Run("successful summary generation", func(t *testing.T) {
		resp, err := client.GenerateSummary(ctx, &aiv1.SummaryRequest{
			Content: testContent,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify summary content
		assert.NotEmpty(t, resp.Summary, "summary should not be empty")
		assert.NotEmpty(t, resp.ModelUsed, "model_used should be set")

		t.Logf("Summary generated: length=%d, model=%s", len(resp.Summary), resp.ModelUsed)
		t.Logf("Summary: %s", resp.Summary)

		// Key points may or may not be extracted depending on response format
		if len(resp.KeyPoints) > 0 {
			t.Logf("Key points: %v", resp.KeyPoints)
		}
	})

	t.Run("summary with brief style", func(t *testing.T) {
		resp, err := client.GenerateSummary(ctx, &aiv1.SummaryRequest{
			Content: testContent,
			Style:   aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.Summary)
	})

	t.Run("summary with max length", func(t *testing.T) {
		maxLength := int32(50)
		resp, err := client.GenerateSummary(ctx, &aiv1.SummaryRequest{
			Content:   testContent,
			MaxLength: &maxLength,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.Summary)
	})

	t.Run("summary with bullet points style", func(t *testing.T) {
		resp, err := client.GenerateSummary(ctx, &aiv1.SummaryRequest{
			Content: testContent,
			Style:   aiv1.SummaryStyle_SUMMARY_STYLE_BULLET_POINTS,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotEmpty(t, resp.Summary)
	})

	t.Run("empty content returns error", func(t *testing.T) {
		_, err := client.GenerateSummary(ctx, &aiv1.SummaryRequest{
			Content: "",
		})
		require.Error(t, err)
	})
}

// TestAIService_ExtractAssertions tests the assertion extraction endpoint.
func TestAIService_ExtractAssertions(t *testing.T) {
	client := skipIfAIServiceUnavailable(t)
	ctx := context.Background()

	testContent := `John Smith is the CEO of Acme Corporation. The company was founded in 2010.
Sarah Johnson leads the engineering team and reports to John. The headquarters
is located in San Francisco. Last quarter, Acme acquired TechStartup Inc for
$50 million. The acquisition is expected to close by the end of March 2024.`

	t.Run("successful assertion extraction", func(t *testing.T) {
		resp, err := client.ExtractAssertions(ctx, &aiv1.AssertionRequest{
			Content: testContent,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify response structure
		assert.NotEmpty(t, resp.ModelUsed, "model_used should be set")

		t.Logf("Assertions extracted: count=%d, total_found=%d, model=%s",
			len(resp.Assertions), resp.TotalFound, resp.ModelUsed)

		for i, a := range resp.Assertions {
			t.Logf("Assertion %d: %s %s %s (confidence: %.2f)",
				i+1, a.Subject, a.Predicate, a.Object, a.Confidence)
		}
	})

	t.Run("assertions with confidence threshold", func(t *testing.T) {
		minConf := float32(0.7)
		resp, err := client.ExtractAssertions(ctx, &aiv1.AssertionRequest{
			Content:       testContent,
			MinConfidence: &minConf,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		// All returned assertions should meet the threshold
		for _, a := range resp.Assertions {
			assert.GreaterOrEqual(t, a.Confidence, minConf,
				"assertion confidence should meet threshold")
		}
	})

	t.Run("assertions with max limit", func(t *testing.T) {
		maxAssertions := int32(3)
		resp, err := client.ExtractAssertions(ctx, &aiv1.AssertionRequest{
			Content:       testContent,
			MaxAssertions: &maxAssertions,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.LessOrEqual(t, len(resp.Assertions), int(maxAssertions),
			"should not exceed max assertions limit")
	})

	t.Run("empty content returns error", func(t *testing.T) {
		_, err := client.ExtractAssertions(ctx, &aiv1.AssertionRequest{
			Content: "",
		})
		require.Error(t, err)
	})
}

// TestAIService_ClassifyContent tests the content classification endpoint.
func TestAIService_ClassifyContent(t *testing.T) {
	client := skipIfAIServiceUnavailable(t)
	ctx := context.Background()

	t.Run("classify work-related content", func(t *testing.T) {
		resp, err := client.ClassifyContent(ctx, &aiv1.ClassifyContentRequest{
			Content:    "Please review the quarterly sales report and prepare the presentation for the board meeting tomorrow.",
			Categories: []string{"work", "personal", "finance", "health"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.NotEmpty(t, resp.ModelUsed, "model_used should be set")
		assert.NotNil(t, resp.Primary, "primary classification should be set")

		t.Logf("Classifications: primary=%s (%.2f), model=%s",
			resp.Primary.Label, resp.Primary.Confidence, resp.ModelUsed)

		for _, c := range resp.Classifications {
			t.Logf("  - %s: %.2f", c.Label, c.Confidence)
		}
	})

	t.Run("classify with custom categories", func(t *testing.T) {
		resp, err := client.ClassifyContent(ctx, &aiv1.ClassifyContentRequest{
			Content:    "The patient reported experiencing headaches and fatigue over the past week.",
			Categories: []string{"medical", "legal", "technical", "administrative"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify classifications use provided categories
		for _, c := range resp.Classifications {
			assert.Contains(t, []string{"medical", "legal", "technical", "administrative"}, c.Label,
				"classification should be from provided categories")
		}
	})

	t.Run("single label classification", func(t *testing.T) {
		multiLabel := false
		resp, err := client.ClassifyContent(ctx, &aiv1.ClassifyContentRequest{
			Content:    "I need to schedule a doctor's appointment for my annual checkup.",
			Categories: []string{"work", "personal", "health", "finance"},
			MultiLabel: &multiLabel,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Single label mode should return at most one classification
		assert.LessOrEqual(t, len(resp.Classifications), 1,
			"single label mode should return at most one classification")
	})

	t.Run("empty content returns error", func(t *testing.T) {
		_, err := client.ClassifyContent(ctx, &aiv1.ClassifyContentRequest{
			Content: "",
		})
		require.Error(t, err)
	})
}

// TestAIService_GetModelStatus tests the model status endpoint.
func TestAIService_GetModelStatus(t *testing.T) {
	client := skipIfAIServiceUnavailable(t)
	ctx := context.Background()

	t.Run("get all models status", func(t *testing.T) {
		resp, err := client.GetModelStatus(ctx, &aiv1.GetModelStatusRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.NotEmpty(t, resp.DefaultEmbeddingModel, "default embedding model should be set")
		assert.NotEmpty(t, resp.DefaultLlmModel, "default LLM model should be set")

		t.Logf("AI Service Status: healthy=%t, message=%s", resp.Healthy, resp.Message)
		t.Logf("Defaults: embedding=%s, llm=%s", resp.DefaultEmbeddingModel, resp.DefaultLlmModel)

		for _, m := range resp.Models {
			t.Logf("  Model: %s, type=%s, status=%s, provider=%s, local=%t",
				m.Name, m.Type.String(), m.Status.String(), m.Provider, m.IsLocal)
		}
	})

	t.Run("filter by model type - embedding", func(t *testing.T) {
		resp, err := client.GetModelStatus(ctx, &aiv1.GetModelStatusRequest{
			ModelType: aiv1.ModelType_MODEL_TYPE_EMBEDDING.Enum(),
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		for _, m := range resp.Models {
			assert.Equal(t, aiv1.ModelType_MODEL_TYPE_EMBEDDING, m.Type,
				"all models should be embedding type")
		}
	})

	t.Run("filter by model type - LLM", func(t *testing.T) {
		resp, err := client.GetModelStatus(ctx, &aiv1.GetModelStatusRequest{
			ModelType: aiv1.ModelType_MODEL_TYPE_LLM.Enum(),
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		for _, m := range resp.Models {
			assert.Equal(t, aiv1.ModelType_MODEL_TYPE_LLM, m.Type,
				"all models should be LLM type")
		}
	})
}

// TestAIService_Concurrent tests concurrent requests to the AI service.
func TestAIService_Concurrent(t *testing.T) {
	client := skipIfAIServiceUnavailable(t)
	ctx := context.Background()

	t.Run("concurrent embedding requests", func(t *testing.T) {
		const numRequests = 5
		results := make(chan error, numRequests)

		texts := []string{
			"First test message for concurrent embedding.",
			"Second test message for concurrent embedding.",
			"Third test message for concurrent embedding.",
			"Fourth test message for concurrent embedding.",
			"Fifth test message for concurrent embedding.",
		}

		for i := 0; i < numRequests; i++ {
			go func(text string) {
				_, err := client.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
					Text: text,
				})
				results <- err
			}(texts[i])
		}

		// Collect results
		var errors []error
		for i := 0; i < numRequests; i++ {
			if err := <-results; err != nil {
				errors = append(errors, err)
			}
		}

		assert.Empty(t, errors, "all concurrent requests should succeed: %v", errors)
	})
}

// TestAIService_Timeouts tests request timeout handling.
func TestAIService_Timeouts(t *testing.T) {
	addr := getAIServiceAddr()

	t.Run("request with short timeout", func(t *testing.T) {
		// Create client with very short request timeout
		client, err := ai.NewClient(addr,
			ai.WithConnectTimeout(5*time.Second),
			ai.WithRequestTimeout(1*time.Millisecond), // Extremely short
		)
		if err != nil {
			t.Skipf("AI service not available: %v", err)
		}
		defer client.Close()

		// This request should likely timeout given the very short timeout
		// But the test passes if service is fast enough - we're testing the timeout mechanism works
		ctx := context.Background()
		_, err = client.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
			Text: "Testing timeout handling with a message that needs processing.",
		})
		// Error is expected but not required - depends on service speed
		if err != nil {
			t.Logf("Request with short timeout resulted in error (expected): %v", err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		client, err := ai.NewClient(addr, ai.WithConnectTimeout(5*time.Second))
		if err != nil {
			t.Skipf("AI service not available: %v", err)
		}
		defer client.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err = client.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
			Text: "Testing cancelled context.",
		})
		require.Error(t, err, "request with cancelled context should fail")
	})
}

// TestAIService_ConnectionHandling tests connection establishment and management.
func TestAIService_ConnectionHandling(t *testing.T) {
	addr := getAIServiceAddr()

	t.Run("connection with blocking dial", func(t *testing.T) {
		client, err := ai.NewClient(addr,
			ai.WithConnectTimeout(5*time.Second),
			ai.WithDialOptions(grpc.WithBlock()),
		)
		if err != nil {
			t.Skipf("AI service not available: %v", err)
		}

		// Should be able to make requests immediately
		ctx := context.Background()
		err = client.HealthCheck(ctx)
		if err != nil {
			client.Close()
			t.Skipf("AI service health check failed: %v", err)
		}

		client.Close()
	})

	t.Run("connection without TLS", func(t *testing.T) {
		client, err := ai.NewClient(addr,
			ai.WithConnectTimeout(5*time.Second),
			ai.WithInsecure(),
		)
		if err != nil {
			t.Skipf("AI service not available: %v", err)
		}
		defer client.Close()

		ctx := context.Background()
		err = client.HealthCheck(ctx)
		if err != nil {
			t.Skipf("AI service health check failed: %v", err)
		}
	})

	t.Run("double close is safe", func(t *testing.T) {
		client, err := ai.NewClient(addr,
			ai.WithConnectTimeout(5*time.Second),
			ai.WithDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
		)
		if err != nil {
			t.Skipf("AI service not available: %v", err)
		}

		err = client.Close()
		assert.NoError(t, err, "first close should succeed")

		err = client.Close()
		assert.NoError(t, err, "second close should also succeed")
	})
}
