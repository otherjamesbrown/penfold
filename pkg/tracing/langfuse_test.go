package tracing_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/tracing"
)

// TestLangfuseExport is an integration test that sends a trace to Langfuse.
// Skip if LANGFUSE_HOST is not set.
func TestLangfuseExport(t *testing.T) {
	lfConfig := tracing.LangfuseConfigFromEnv()
	if lfConfig == nil || lfConfig.Host == "" {
		t.Skip("LANGFUSE_HOST not set, skipping Langfuse integration test")
	}

	// Initialize tracer with Langfuse exporter
	cfg := &tracing.Config{
		ServiceName: "penfold-test",
		Environment: "test",
		SampleRate:  1.0,
		Exporter:    tracing.ExporterLangfuse,
		Langfuse:    lfConfig,
	}

	shutdown, err := tracing.InitTracer(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			t.Errorf("Failed to shutdown tracer: %v", err)
		}
	}()

	ctx := context.Background()

	// Create a test LLM call trace
	ctx, span := tracing.StartLLMCall(ctx, "test-llm-call", tracing.LLMCallOptions{
		Model:     "test-model",
		System:    tracing.AISystemOllama,
		TenantID:  "test-tenant",
		ContentID: "test-content-" + time.Now().Format("20060102-150405"),
		TaskType:  "summarize",
	})

	// Simulate some work
	time.Sleep(100 * time.Millisecond)

	// Record result
	tracing.SetLLMResult(span, tracing.LLMResult{
		InputTokens:  100,
		OutputTokens: 50,
		Model:        "test-model",
		Prompt:       "Test prompt text",
		Completion:   "Test completion response",
		LatencyMs:    100,
	})

	span.End()

	// Create a child embedding span
	ctx, embedSpan := tracing.StartEmbedding(ctx, "test-embedding", tracing.EmbeddingOptions{
		Model:     "test-embed-model",
		System:    tracing.AISystemMLX,
		TenantID:  "test-tenant",
		ContentID: "test-content",
	})

	time.Sleep(50 * time.Millisecond)

	tracing.SetEmbeddingResult(embedSpan, tracing.EmbeddingResult{
		Dimensions:  1024,
		InputTokens: 200,
		LatencyMs:   50,
		Cached:      false,
	})

	embedSpan.End()

	t.Log("Traces sent to Langfuse successfully")
	t.Logf("Check Langfuse UI at %s", lfConfig.Host)
}

// TestLangfuseConfigFromEnv tests environment variable parsing.
func TestLangfuseConfigFromEnv(t *testing.T) {
	// Save current env
	origHost := os.Getenv("LANGFUSE_HOST")
	origPK := os.Getenv("LANGFUSE_PUBLIC_KEY")
	origSK := os.Getenv("LANGFUSE_SECRET_KEY")
	defer func() {
		os.Setenv("LANGFUSE_HOST", origHost)
		os.Setenv("LANGFUSE_PUBLIC_KEY", origPK)
		os.Setenv("LANGFUSE_SECRET_KEY", origSK)
	}()

	// Test with no env vars
	os.Unsetenv("LANGFUSE_HOST")
	os.Unsetenv("LANGFUSE_PUBLIC_KEY")
	os.Unsetenv("LANGFUSE_SECRET_KEY")

	cfg := tracing.LangfuseConfigFromEnv()
	if cfg != nil {
		t.Error("Expected nil config when no env vars set")
	}

	// Test with env vars
	os.Setenv("LANGFUSE_HOST", "http://test-host:3000")
	os.Setenv("LANGFUSE_PUBLIC_KEY", "pk-test")
	os.Setenv("LANGFUSE_SECRET_KEY", "sk-test")

	cfg = tracing.LangfuseConfigFromEnv()
	if cfg == nil {
		t.Fatal("Expected config when env vars set")
	}
	if cfg.Host != "http://test-host:3000" {
		t.Errorf("Expected host 'http://test-host:3000', got '%s'", cfg.Host)
	}
	if cfg.PublicKey != "pk-test" {
		t.Errorf("Expected public key 'pk-test', got '%s'", cfg.PublicKey)
	}
	if cfg.SecretKey != "sk-test" {
		t.Errorf("Expected secret key 'sk-test', got '%s'", cfg.SecretKey)
	}
}
