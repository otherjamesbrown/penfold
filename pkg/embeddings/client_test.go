package embeddings

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func newTestLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  true,
		Output:      &bytes.Buffer{},
	})
}

func TestNewAIBackedClient(t *testing.T) {
	logger := newTestLogger()
	mockAI := &MockAIClient{}

	t.Run("with valid config", func(t *testing.T) {
		cfg := DefaultConfig()
		client, err := NewAIBackedClient(mockAI, cfg, logger, nil)
		if err != nil {
			t.Fatalf("NewAIBackedClient() error = %v", err)
		}
		if client == nil {
			t.Fatal("NewAIBackedClient() returned nil")
		}
		defer client.Close() //nolint:errcheck
	})

	t.Run("with nil config uses defaults", func(t *testing.T) {
		client, err := NewAIBackedClient(mockAI, nil, logger, nil)
		if err != nil {
			t.Fatalf("NewAIBackedClient() error = %v", err)
		}
		if client == nil {
			t.Fatal("NewAIBackedClient() returned nil")
		}
		defer client.Close() //nolint:errcheck
	})

	t.Run("with nil AIClient returns error", func(t *testing.T) {
		cfg := DefaultConfig()
		_, err := NewAIBackedClient(nil, cfg, logger, nil)
		if err == nil {
			t.Error("NewAIBackedClient() expected error for nil AIClient")
		}
	})

	t.Run("with invalid config returns error", func(t *testing.T) {
		cfg := &Config{} // Invalid - missing required fields
		_, err := NewAIBackedClient(mockAI, cfg, logger, nil)
		if err == nil {
			t.Error("NewAIBackedClient() expected error for invalid config")
		}
	})

	t.Run("with cache", func(t *testing.T) {
		cfg := DefaultConfig()
		cache := NewMemoryCache(nil)
		client, err := NewAIBackedClient(mockAI, cfg, logger, cache)
		if err != nil {
			t.Fatalf("NewAIBackedClient() error = %v", err)
		}
		if client == nil {
			t.Fatal("NewAIBackedClient() returned nil")
		}
		defer client.Close() //nolint:errcheck
	})
}

func TestAIBackedClient_Embed_EmptyText(t *testing.T) {
	logger := newTestLogger()
	mockAI := &MockAIClient{}
	cfg := DefaultConfig()
	client, _ := NewAIBackedClient(mockAI, cfg, logger, nil)
	defer client.Close() //nolint:errcheck

	_, err := client.Embed(context.Background(), "")
	if !errors.Is(err, ErrEmptyText) {
		t.Errorf("Embed(\"\") error = %v, want ErrEmptyText", err)
	}

	_, err = client.Embed(context.Background(), "   ")
	if !errors.Is(err, ErrEmptyText) {
		t.Errorf("Embed(\"   \") error = %v, want ErrEmptyText", err)
	}
}

func TestAIBackedClient_Embed_WithMockAI(t *testing.T) {
	logger := newTestLogger()

	t.Run("successful response", func(t *testing.T) {
		embedding := make([]float32, 1024)
		for i := range embedding {
			embedding[i] = float32(i) / 1024.0
		}

		mockAI := &MockAIClient{
			EmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
				return &aiv1.EmbeddingResponse{
					Vector:     embedding,
					Dimensions: 1024,
					ModelUsed:  "test-model",
				}, nil
			},
		}

		cfg := &Config{
			Model:      DefaultModel,
			BatchSize:  DefaultBatchSize,
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0,
			RetryDelay: DefaultRetryDelay,
		}

		client, err := NewAIBackedClient(mockAI, cfg, logger, nil)
		if err != nil {
			t.Fatalf("NewAIBackedClient() error = %v", err)
		}
		defer client.Close() //nolint:errcheck

		result, err := client.Embed(context.Background(), "test text")
		if err != nil {
			t.Fatalf("Embed() error = %v", err)
		}

		if len(result) != 1024 {
			t.Errorf("Embed() returned %d dimensions, want 1024", len(result))
		}
	})

	t.Run("AI service error", func(t *testing.T) {
		mockAI := &MockAIClient{
			EmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
				return nil, errors.New("AI service error")
			},
		}

		cfg := &Config{
			Model:      DefaultModel,
			BatchSize:  DefaultBatchSize,
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0,
			RetryDelay: DefaultRetryDelay,
		}

		client, err := NewAIBackedClient(mockAI, cfg, logger, nil)
		if err != nil {
			t.Fatalf("NewAIBackedClient() error = %v", err)
		}
		defer client.Close() //nolint:errcheck

		_, err = client.Embed(context.Background(), "test text")
		if err == nil {
			t.Error("Embed() expected error for AI service error")
		}
	})

	t.Run("invalid response - nil", func(t *testing.T) {
		mockAI := &MockAIClient{
			EmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
				return nil, nil
			},
		}

		cfg := &Config{
			Model:      DefaultModel,
			BatchSize:  DefaultBatchSize,
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0,
			RetryDelay: DefaultRetryDelay,
		}

		client, err := NewAIBackedClient(mockAI, cfg, logger, nil)
		if err != nil {
			t.Fatalf("NewAIBackedClient() error = %v", err)
		}
		defer client.Close() //nolint:errcheck

		_, err = client.Embed(context.Background(), "test text")
		if err == nil {
			t.Error("Embed() expected error for nil response")
		}
	})

	t.Run("invalid response - empty vector", func(t *testing.T) {
		mockAI := &MockAIClient{
			EmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
				return &aiv1.EmbeddingResponse{
					Vector:     []float32{},
					Dimensions: 0,
				}, nil
			},
		}

		cfg := &Config{
			Model:      DefaultModel,
			BatchSize:  DefaultBatchSize,
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0,
			RetryDelay: DefaultRetryDelay,
		}

		client, err := NewAIBackedClient(mockAI, cfg, logger, nil)
		if err != nil {
			t.Fatalf("NewAIBackedClient() error = %v", err)
		}
		defer client.Close() //nolint:errcheck

		_, err = client.Embed(context.Background(), "test text")
		if err == nil {
			t.Error("Embed() expected error for empty vector response")
		}
	})
}

func TestAIBackedClient_Embed_WithCache(t *testing.T) {
	logger := newTestLogger()
	cache := NewMemoryCache(nil)

	embedding := make([]float32, 1024)
	for i := range embedding {
		embedding[i] = float32(i) / 1024.0
	}

	var requestCount int64
	mockAI := &MockAIClient{
		EmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			atomic.AddInt64(&requestCount, 1)
			return &aiv1.EmbeddingResponse{
				Vector:     embedding,
				Dimensions: 1024,
				ModelUsed:  "test-model",
			}, nil
		},
	}

	cfg := &Config{
		Model:      DefaultModel,
		BatchSize:  DefaultBatchSize,
		Timeout:    DefaultTimeout,
		Dimensions: DefaultDimensions,
		MaxRetries: 0,
		RetryDelay: DefaultRetryDelay,
	}

	client, _ := NewAIBackedClient(mockAI, cfg, logger, cache)
	defer client.Close() //nolint:errcheck

	// First call - should hit AI service
	_, err := client.Embed(context.Background(), "test text")
	if err != nil {
		t.Fatalf("First Embed() error = %v", err)
	}
	if atomic.LoadInt64(&requestCount) != 1 {
		t.Errorf("Expected 1 AI request, got %d", requestCount)
	}

	// Second call - should hit cache
	_, err = client.Embed(context.Background(), "test text")
	if err != nil {
		t.Fatalf("Second Embed() error = %v", err)
	}
	if atomic.LoadInt64(&requestCount) != 1 {
		t.Errorf("Expected 1 AI request (cached), got %d", requestCount)
	}

	// Different text - should hit AI service
	_, err = client.Embed(context.Background(), "different text")
	if err != nil {
		t.Fatalf("Third Embed() error = %v", err)
	}
	if atomic.LoadInt64(&requestCount) != 2 {
		t.Errorf("Expected 2 AI requests, got %d", requestCount)
	}
}

func TestAIBackedClient_Embed_ContextCanceled(t *testing.T) {
	logger := newTestLogger()

	// Mock that checks context cancellation
	mockAI := &MockAIClient{
		EmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			// Check if context is already canceled
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return &aiv1.EmbeddingResponse{
				Vector:     make([]float32, 1024),
				Dimensions: 1024,
			}, nil
		},
	}

	cfg := &Config{
		Model:      DefaultModel,
		BatchSize:  DefaultBatchSize,
		Timeout:    5 * time.Second,
		Dimensions: DefaultDimensions,
		MaxRetries: 0,
		RetryDelay: DefaultRetryDelay,
	}

	client, _ := NewAIBackedClient(mockAI, cfg, logger, nil)
	defer client.Close() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Embed(ctx, "test text")
	if err == nil {
		t.Error("Embed() expected error for canceled context")
	}
}

func TestAIBackedClient_Embed_Retries(t *testing.T) {
	logger := newTestLogger()

	var attemptCount int64
	mockAI := &MockAIClient{
		EmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			count := atomic.AddInt64(&attemptCount, 1)
			if count < 3 {
				return nil, errors.New("temporary error")
			}
			return &aiv1.EmbeddingResponse{
				Vector:     make([]float32, 1024),
				Dimensions: 1024,
			}, nil
		},
	}

	cfg := &Config{
		Model:      DefaultModel,
		BatchSize:  DefaultBatchSize,
		Timeout:    5 * time.Second,
		Dimensions: DefaultDimensions,
		MaxRetries: 3,
		RetryDelay: 10 * time.Millisecond,
	}

	client, _ := NewAIBackedClient(mockAI, cfg, logger, nil)
	defer client.Close() //nolint:errcheck

	_, err := client.Embed(context.Background(), "test text")
	if err != nil {
		t.Fatalf("Embed() error = %v, expected success after retries", err)
	}

	if atomic.LoadInt64(&attemptCount) != 3 {
		t.Errorf("Expected 3 attempts, got %d", attemptCount)
	}
}

func TestAIBackedClient_BatchEmbed(t *testing.T) {
	logger := newTestLogger()
	mockAI := &MockAIClient{}

	t.Run("empty batch", func(t *testing.T) {
		cfg := DefaultConfig()
		client, _ := NewAIBackedClient(mockAI, cfg, logger, nil)
		defer client.Close() //nolint:errcheck

		_, err := client.BatchEmbed(context.Background(), []string{})
		if !errors.Is(err, ErrEmptyBatch) {
			t.Errorf("BatchEmbed([]) error = %v, want ErrEmptyBatch", err)
		}
	})

	t.Run("batch too large", func(t *testing.T) {
		cfg := &Config{
			Model:      DefaultModel,
			BatchSize:  2, // Small batch size for testing
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0,
			RetryDelay: DefaultRetryDelay,
		}
		client, _ := NewAIBackedClient(mockAI, cfg, logger, nil)
		defer client.Close() //nolint:errcheck

		_, err := client.BatchEmbed(context.Background(), []string{"a", "b", "c"})
		if err == nil || !errors.Is(err, ErrBatchTooLarge) {
			t.Errorf("BatchEmbed() error = %v, want ErrBatchTooLarge", err)
		}
	})

	t.Run("empty text in batch", func(t *testing.T) {
		cfg := DefaultConfig()
		client, _ := NewAIBackedClient(mockAI, cfg, logger, nil)
		defer client.Close() //nolint:errcheck

		_, err := client.BatchEmbed(context.Background(), []string{"valid", "", "text"})
		if err == nil {
			t.Error("BatchEmbed() expected error for empty text in batch")
		}
	})

	t.Run("successful batch", func(t *testing.T) {
		embedding := make([]float32, 1024)
		for i := range embedding {
			embedding[i] = float32(i) / 1024.0
		}

		mockAI := &MockAIClient{
			EmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
				return &aiv1.EmbeddingResponse{
					Vector:     embedding,
					Dimensions: 1024,
					ModelUsed:  "test-model",
				}, nil
			},
		}

		cfg := &Config{
			Model:      DefaultModel,
			BatchSize:  DefaultBatchSize,
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0,
			RetryDelay: DefaultRetryDelay,
		}

		client, _ := NewAIBackedClient(mockAI, cfg, logger, nil)
		defer client.Close() //nolint:errcheck

		results, err := client.BatchEmbed(context.Background(), []string{"text1", "text2"})
		if err != nil {
			t.Fatalf("BatchEmbed() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("BatchEmbed() returned %d results, want 2", len(results))
		}
	})

	t.Run("batch with partial cache", func(t *testing.T) {
		cache := NewMemoryCache(nil)
		embedding := make([]float32, 1024)
		for i := range embedding {
			embedding[i] = float32(i) / 1024.0
		}

		// Pre-populate cache with one embedding
		_ = cache.Set(context.Background(), "cached text", embedding)

		var requestCount int64
		mockAI := &MockAIClient{
			EmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
				atomic.AddInt64(&requestCount, 1)
				return &aiv1.EmbeddingResponse{
					Vector:     embedding,
					Dimensions: 1024,
					ModelUsed:  "test-model",
				}, nil
			},
		}

		cfg := &Config{
			Model:      DefaultModel,
			BatchSize:  DefaultBatchSize,
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0,
			RetryDelay: DefaultRetryDelay,
		}

		client, _ := NewAIBackedClient(mockAI, cfg, logger, cache)
		defer client.Close() //nolint:errcheck

		results, err := client.BatchEmbed(context.Background(), []string{"cached text", "uncached text"})
		if err != nil {
			t.Fatalf("BatchEmbed() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("BatchEmbed() returned %d results, want 2", len(results))
		}

		// Only one request should have been made (for uncached text)
		if atomic.LoadInt64(&requestCount) != 1 {
			t.Errorf("Expected 1 AI request, got %d", requestCount)
		}
	})
}

func TestAIBackedClient_Dimensions(t *testing.T) {
	logger := newTestLogger()
	mockAI := &MockAIClient{}

	cfg := &Config{
		Model:      DefaultModel,
		BatchSize:  DefaultBatchSize,
		Timeout:    DefaultTimeout,
		Dimensions: 512, // Custom dimensions
		MaxRetries: 0,
		RetryDelay: DefaultRetryDelay,
	}

	client, _ := NewAIBackedClient(mockAI, cfg, logger, nil)
	defer client.Close() //nolint:errcheck

	if client.Dimensions() != 512 {
		t.Errorf("Dimensions() = %d, want 512", client.Dimensions())
	}
}

func TestAIBackedClient_ModelInfo(t *testing.T) {
	logger := newTestLogger()
	mockAI := &MockAIClient{}
	cfg := DefaultConfig()
	client, _ := NewAIBackedClient(mockAI, cfg, logger, nil)
	defer client.Close() //nolint:errcheck

	info := client.ModelInfo()
	if info == nil {
		t.Fatal("ModelInfo() returned nil")
	}
	if info.Name != DefaultModel {
		t.Errorf("ModelInfo().Name = %q, want %q", info.Name, DefaultModel)
	}
	if info.Dimensions != DefaultDimensions {
		t.Errorf("ModelInfo().Dimensions = %d, want %d", info.Dimensions, DefaultDimensions)
	}
}

func TestAIBackedClient_Close(t *testing.T) {
	logger := newTestLogger()
	mockAI := &MockAIClient{}
	cfg := DefaultConfig()
	client, _ := NewAIBackedClient(mockAI, cfg, logger, nil)

	err := client.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestAIBackedClient_Stats(t *testing.T) {
	logger := newTestLogger()

	embedding := make([]float32, 1024)
	mockAI := &MockAIClient{
		EmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
			return &aiv1.EmbeddingResponse{
				Vector:     embedding,
				Dimensions: 1024,
			}, nil
		},
	}

	cfg := DefaultConfig()
	client, _ := NewAIBackedClient(mockAI, cfg, logger, nil)
	defer client.Close() //nolint:errcheck

	// Make some requests
	_, _ = client.Embed(context.Background(), "text1")
	_, _ = client.Embed(context.Background(), "text2")

	stats := client.Stats()
	if stats.RequestsTotal != 2 {
		t.Errorf("Stats().RequestsTotal = %d, want 2", stats.RequestsTotal)
	}
}

func TestMockClient(t *testing.T) {
	t.Run("default behavior", func(t *testing.T) {
		client := NewMockClient(512, nil)
		defer client.Close() //nolint:errcheck

		embedding, err := client.Embed(context.Background(), "test")
		if err != nil {
			t.Fatalf("Embed() error = %v", err)
		}
		if len(embedding) != 512 {
			t.Errorf("Embed() returned %d dimensions, want 512", len(embedding))
		}
	})

	t.Run("custom embed function", func(t *testing.T) {
		customFunc := func(ctx context.Context, text string) ([]float32, error) {
			if text == "error" {
				return nil, errors.New("custom error")
			}
			return []float32{1.0, 2.0, 3.0}, nil
		}

		client := NewMockClient(3, customFunc)
		defer client.Close() //nolint:errcheck

		embedding, err := client.Embed(context.Background(), "test")
		if err != nil {
			t.Fatalf("Embed() error = %v", err)
		}
		if len(embedding) != 3 {
			t.Errorf("Embed() returned %d dimensions, want 3", len(embedding))
		}

		_, err = client.Embed(context.Background(), "error")
		if err == nil {
			t.Error("Embed() expected error")
		}
	})

	t.Run("batch embed", func(t *testing.T) {
		client := NewMockClient(512, nil)
		defer client.Close() //nolint:errcheck

		embeddings, err := client.BatchEmbed(context.Background(), []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("BatchEmbed() error = %v", err)
		}
		if len(embeddings) != 3 {
			t.Errorf("BatchEmbed() returned %d results, want 3", len(embeddings))
		}
	})

	t.Run("dimensions", func(t *testing.T) {
		client := NewMockClient(768, nil)
		if client.Dimensions() != 768 {
			t.Errorf("Dimensions() = %d, want 768", client.Dimensions())
		}
	})

	t.Run("model info", func(t *testing.T) {
		client := NewMockClient(512, nil)
		info := client.ModelInfo()
		if info == nil {
			t.Fatal("ModelInfo() returned nil")
		}
		if info.Name != "mock-model" {
			t.Errorf("ModelInfo().Name = %q, want %q", info.Name, "mock-model")
		}
	})
}

func TestMockAIClient(t *testing.T) {
	t.Run("default behavior", func(t *testing.T) {
		client := &MockAIClient{}
		resp, err := client.GenerateEmbedding(context.Background(), &aiv1.EmbeddingRequest{
			Text: "test",
		})
		if err != nil {
			t.Fatalf("GenerateEmbedding() error = %v", err)
		}
		if len(resp.Vector) != 1024 {
			t.Errorf("GenerateEmbedding() returned %d dimensions, want 1024", len(resp.Vector))
		}
	})

	t.Run("custom function", func(t *testing.T) {
		client := &MockAIClient{
			EmbeddingFunc: func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
				if req.Text == "error" {
					return nil, errors.New("custom error")
				}
				return &aiv1.EmbeddingResponse{
					Vector:     []float32{1.0, 2.0, 3.0},
					Dimensions: 3,
					ModelUsed:  "custom-model",
				}, nil
			},
		}

		resp, err := client.GenerateEmbedding(context.Background(), &aiv1.EmbeddingRequest{
			Text: "test",
		})
		if err != nil {
			t.Fatalf("GenerateEmbedding() error = %v", err)
		}
		if len(resp.Vector) != 3 {
			t.Errorf("GenerateEmbedding() returned %d dimensions, want 3", len(resp.Vector))
		}

		_, err = client.GenerateEmbedding(context.Background(), &aiv1.EmbeddingRequest{
			Text: "error",
		})
		if err == nil {
			t.Error("GenerateEmbedding() expected error")
		}
	})
}
