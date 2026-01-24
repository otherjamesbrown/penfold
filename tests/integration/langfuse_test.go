//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/ai"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// LangfuseTestConfig holds configuration for Langfuse integration tests.
type LangfuseTestConfig struct {
	Host       string
	PublicKey  string
	SecretKey  string
	AIService  string
	Configured bool
}

// getLangfuseConfig returns Langfuse configuration from environment.
func getLangfuseConfig() *LangfuseTestConfig {
	host := os.Getenv("LANGFUSE_HOST")
	pk := os.Getenv("LANGFUSE_PUBLIC_KEY")
	sk := os.Getenv("LANGFUSE_SECRET_KEY")

	return &LangfuseTestConfig{
		Host:       host,
		PublicKey:  pk,
		SecretKey:  sk,
		AIService:  getAIServiceAddr(),
		Configured: host != "" && pk != "" && sk != "",
	}
}

// skipIfLangfuseUnconfigured skips the test if Langfuse is not configured.
func skipIfLangfuseUnconfigured(t *testing.T) *LangfuseTestConfig {
	t.Helper()

	cfg := getLangfuseConfig()
	if !cfg.Configured {
		t.Skip("Langfuse not configured (LANGFUSE_HOST, LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY required)")
	}
	return cfg
}

// TestLangfuse_TracingInitialization tests that tracing can be initialized with Langfuse.
func TestLangfuse_TracingInitialization(t *testing.T) {
	lfConfig := skipIfLangfuseUnconfigured(t)

	t.Run("initialize tracer with Langfuse exporter", func(t *testing.T) {
		cfg := &tracing.Config{
			ServiceName: "penfold-integration-test",
			Environment: "test",
			SampleRate:  1.0,
			Exporter:    tracing.ExporterLangfuse,
			Langfuse: &tracing.LangfuseConfig{
				Host:      lfConfig.Host,
				PublicKey: lfConfig.PublicKey,
				SecretKey: lfConfig.SecretKey,
			},
		}

		shutdown, err := tracing.InitTracer(cfg)
		require.NoError(t, err, "tracer initialization should succeed")
		require.NotNil(t, shutdown, "shutdown function should be returned")

		// Clean up
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = shutdown(ctx)
		assert.NoError(t, err, "tracer shutdown should succeed")
	})
}

// TestLangfuse_AICallsTraced tests that AI calls through the service are traced to Langfuse.
func TestLangfuse_AICallsTraced(t *testing.T) {
	lfConfig := skipIfLangfuseUnconfigured(t)

	// Initialize tracing with Langfuse
	tracingCfg := &tracing.Config{
		ServiceName: "penfold-ai-integration-test",
		Environment: "test",
		SampleRate:  1.0,
		Exporter:    tracing.ExporterLangfuse,
		Langfuse: &tracing.LangfuseConfig{
			Host:      lfConfig.Host,
			PublicKey: lfConfig.PublicKey,
			SecretKey: lfConfig.SecretKey,
		},
	}

	shutdown, err := tracing.InitTracer(tracingCfg)
	require.NoError(t, err, "tracer initialization should succeed")
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdown(ctx)
	}()

	// Connect to AI service
	client, err := ai.NewClient(lfConfig.AIService,
		ai.WithConnectTimeout(5*time.Second),
		ai.WithDialOptions(grpc.WithBlock()),
	)
	if err != nil {
		t.Skipf("AI service not available at %s: %v", lfConfig.AIService, err)
	}
	defer client.Close()

	ctx := context.Background()

	// Verify service is healthy
	if err := client.HealthCheck(ctx); err != nil {
		t.Skipf("AI service health check failed: %v", err)
	}

	t.Run("embedding call creates trace", func(t *testing.T) {
		// Create a parent span for the test
		testCtx, parentSpan := tracing.StartPipeline(ctx, "integration-test-embedding",
			"test-content-"+time.Now().Format("20060102-150405"), "test")
		defer parentSpan.End()

		// Make embedding call
		resp, err := client.GenerateEmbedding(testCtx, &aiv1.EmbeddingRequest{
			Text: "Test text for Langfuse trace verification.",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		t.Logf("Embedding generated: dimensions=%d, model=%s", resp.Dimensions, resp.ModelUsed)
		t.Logf("Check Langfuse UI at %s for trace", lfConfig.Host)
	})

	t.Run("summary call creates trace", func(t *testing.T) {
		// Create a parent span for the test
		testCtx, parentSpan := tracing.StartPipeline(ctx, "integration-test-summary",
			"test-content-"+time.Now().Format("20060102-150405"), "test")
		defer parentSpan.End()

		// Make summary call
		resp, err := client.GenerateSummary(testCtx, &aiv1.SummaryRequest{
			Content: "This is test content that should be summarized. It contains multiple sentences to ensure the model has something meaningful to work with. The summary should be shorter than this original text.",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		t.Logf("Summary generated: length=%d, model=%s", len(resp.Summary), resp.ModelUsed)
		t.Logf("Check Langfuse UI at %s for trace", lfConfig.Host)
	})

	t.Run("assertion extraction creates trace", func(t *testing.T) {
		// Create a parent span for the test
		testCtx, parentSpan := tracing.StartPipeline(ctx, "integration-test-assertions",
			"test-content-"+time.Now().Format("20060102-150405"), "test")
		defer parentSpan.End()

		// Make assertion extraction call
		resp, err := client.ExtractAssertions(testCtx, &aiv1.AssertionRequest{
			Content: "John works at Acme Corp. The project deadline is next Friday. Sarah is the team lead.",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		t.Logf("Assertions extracted: count=%d, model=%s", len(resp.Assertions), resp.ModelUsed)
		t.Logf("Check Langfuse UI at %s for trace", lfConfig.Host)
	})

	t.Run("classification creates trace", func(t *testing.T) {
		// Create a parent span for the test
		testCtx, parentSpan := tracing.StartPipeline(ctx, "integration-test-classification",
			"test-content-"+time.Now().Format("20060102-150405"), "test")
		defer parentSpan.End()

		// Make classification call
		resp, err := client.ClassifyContent(testCtx, &aiv1.ClassifyContentRequest{
			Content:    "Reminder: Team standup at 10am tomorrow. Please prepare status updates.",
			Categories: []string{"work", "personal", "urgent", "informational"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		t.Logf("Classification: primary=%s, confidence=%.2f, model=%s",
			resp.Primary.Label, resp.Primary.Confidence, resp.ModelUsed)
		t.Logf("Check Langfuse UI at %s for trace", lfConfig.Host)
	})
}

// TestLangfuse_TracingHelpers tests the AI tracing helper functions.
func TestLangfuse_TracingHelpers(t *testing.T) {
	lfConfig := skipIfLangfuseUnconfigured(t)

	// Initialize tracing with Langfuse
	tracingCfg := &tracing.Config{
		ServiceName: "penfold-tracing-helpers-test",
		Environment: "test",
		SampleRate:  1.0,
		Exporter:    tracing.ExporterLangfuse,
		Langfuse: &tracing.LangfuseConfig{
			Host:      lfConfig.Host,
			PublicKey: lfConfig.PublicKey,
			SecretKey: lfConfig.SecretKey,
		},
	}

	shutdown, err := tracing.InitTracer(tracingCfg)
	require.NoError(t, err, "tracer initialization should succeed")
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdown(ctx)
	}()

	ctx := context.Background()

	t.Run("StartLLMCall creates span with attributes", func(t *testing.T) {
		startTime := time.Now()

		_, span := tracing.StartLLMCall(ctx, "test-llm-call", tracing.LLMCallOptions{
			Model:     "test-model",
			System:    tracing.AISystemOllama,
			TenantID:  "test-tenant",
			ContentID: "test-content-" + time.Now().Format("20060102-150405"),
			TaskType:  "summarize",
		})

		// Simulate work
		time.Sleep(50 * time.Millisecond)

		// Record result
		tracing.SetLLMResult(span, tracing.LLMResult{
			InputTokens:  100,
			OutputTokens: 50,
			Model:        "test-model",
			Prompt:       "Test prompt",
			Completion:   "Test completion",
			LatencyMs:    time.Since(startTime).Milliseconds(),
		})

		span.End()

		t.Logf("LLM call span created, check Langfuse at %s", lfConfig.Host)
	})

	t.Run("StartEmbedding creates span with attributes", func(t *testing.T) {
		startTime := time.Now()

		_, span := tracing.StartEmbedding(ctx, "test-embedding", tracing.EmbeddingOptions{
			Model:     "mxbai-embed-large",
			System:    tracing.AISystemMLX,
			TenantID:  "test-tenant",
			ContentID: "test-content-" + time.Now().Format("20060102-150405"),
		})

		// Simulate work
		time.Sleep(30 * time.Millisecond)

		// Record result
		tracing.SetEmbeddingResult(span, tracing.EmbeddingResult{
			Dimensions:  1024,
			InputTokens: 50,
			LatencyMs:   time.Since(startTime).Milliseconds(),
			Cached:      false,
		})

		span.End()

		t.Logf("Embedding span created, check Langfuse at %s", lfConfig.Host)
	})

	t.Run("StartAIProcessing creates span with attributes", func(t *testing.T) {
		_, span := tracing.StartAIProcessing(ctx, "test-processing", tracing.AIProcessingOptions{
			TaskType:    "extraction",
			TenantID:    "test-tenant",
			ContentID:   "test-content-" + time.Now().Format("20060102-150405"),
			ContentType: "email",
		})

		// Simulate work
		time.Sleep(20 * time.Millisecond)

		// Record a decision
		tracing.SetDecision(span, "entity-match", 0.85, "matched based on name similarity")

		span.End()

		t.Logf("AI processing span created, check Langfuse at %s", lfConfig.Host)
	})

	t.Run("StartPipeline creates parent span", func(t *testing.T) {
		pipelineCtx, pipelineSpan := tracing.StartPipeline(ctx, "test-pipeline",
			"test-content-"+time.Now().Format("20060102-150405"), "email")
		defer pipelineSpan.End()

		// Create child spans
		_, embedSpan := tracing.StartEmbedding(pipelineCtx, "pipeline-embedding", tracing.EmbeddingOptions{
			Model:  "test-model",
			System: tracing.AISystemMLX,
		})
		time.Sleep(10 * time.Millisecond)
		embedSpan.End()

		_, llmSpan := tracing.StartLLMCall(pipelineCtx, "pipeline-llm", tracing.LLMCallOptions{
			Model:    "test-model",
			System:   tracing.AISystemOllama,
			TaskType: "summarize",
		})
		time.Sleep(10 * time.Millisecond)
		llmSpan.End()

		t.Logf("Pipeline span with children created, check Langfuse at %s", lfConfig.Host)
	})

	t.Run("RecordAIEvent adds event to span", func(t *testing.T) {
		_, span := tracing.StartLLMCall(ctx, "test-with-events", tracing.LLMCallOptions{
			Model:    "test-model",
			System:   tracing.AISystemOllama,
			TaskType: "test",
		})

		// Record events
		tracing.RecordAIEvent(span, "model-loaded")
		time.Sleep(10 * time.Millisecond)
		tracing.RecordAIEvent(span, "processing-started")
		time.Sleep(10 * time.Millisecond)
		tracing.RecordAIEvent(span, "processing-completed")

		span.End()

		t.Logf("Span with events created, check Langfuse at %s", lfConfig.Host)
	})

	t.Run("SetModelSelection records selection info", func(t *testing.T) {
		_, span := tracing.StartLLMCall(ctx, "test-with-selection", tracing.LLMCallOptions{
			Model:    "selected-model",
			System:   tracing.AISystemOllama,
			TaskType: "test",
		})

		// Record model selection
		tracing.SetModelSelection(span,
			"selected-model",
			"ollama",
			"local model preferred for cost optimization",
			[]string{"gemini-pro", "gpt-4", "claude-3"},
		)

		span.End()

		t.Logf("Span with model selection created, check Langfuse at %s", lfConfig.Host)
	})
}

// TestLangfuse_ConfigFromEnv tests environment variable parsing for Langfuse config.
func TestLangfuse_ConfigFromEnv(t *testing.T) {
	// Save current env
	origHost := os.Getenv("LANGFUSE_HOST")
	origPK := os.Getenv("LANGFUSE_PUBLIC_KEY")
	origSK := os.Getenv("LANGFUSE_SECRET_KEY")
	defer func() {
		os.Setenv("LANGFUSE_HOST", origHost)
		os.Setenv("LANGFUSE_PUBLIC_KEY", origPK)
		os.Setenv("LANGFUSE_SECRET_KEY", origSK)
	}()

	t.Run("returns nil when env vars not set", func(t *testing.T) {
		os.Unsetenv("LANGFUSE_HOST")
		os.Unsetenv("LANGFUSE_PUBLIC_KEY")
		os.Unsetenv("LANGFUSE_SECRET_KEY")

		cfg := tracing.LangfuseConfigFromEnv()
		assert.Nil(t, cfg, "should return nil when env vars not set")
	})

	t.Run("returns config when env vars set", func(t *testing.T) {
		os.Setenv("LANGFUSE_HOST", "http://test-host:3000")
		os.Setenv("LANGFUSE_PUBLIC_KEY", "pk-test-key")
		os.Setenv("LANGFUSE_SECRET_KEY", "sk-test-key")

		cfg := tracing.LangfuseConfigFromEnv()
		require.NotNil(t, cfg, "should return config when env vars set")
		assert.Equal(t, "http://test-host:3000", cfg.Host)
		assert.Equal(t, "pk-test-key", cfg.PublicKey)
		assert.Equal(t, "sk-test-key", cfg.SecretKey)
	})

	t.Run("returns partial config when only host set", func(t *testing.T) {
		os.Setenv("LANGFUSE_HOST", "http://test-host:3000")
		os.Unsetenv("LANGFUSE_PUBLIC_KEY")
		os.Unsetenv("LANGFUSE_SECRET_KEY")

		cfg := tracing.LangfuseConfigFromEnv()
		// Function returns a partial config if any env var is set
		// The caller should validate all required fields are present
		require.NotNil(t, cfg, "should return config when at least one env var is set")
		assert.Equal(t, "http://test-host:3000", cfg.Host)
		assert.Empty(t, cfg.PublicKey)
		assert.Empty(t, cfg.SecretKey)
	})
}
