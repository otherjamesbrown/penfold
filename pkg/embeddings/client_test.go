package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestNewMLXClient(t *testing.T) {
	logger := newTestLogger()

	t.Run("with valid config", func(t *testing.T) {
		cfg := DefaultConfig()
		client, err := NewMLXClient(cfg, logger, nil)
		if err != nil {
			t.Fatalf("NewMLXClient() error = %v", err)
		}
		if client == nil {
			t.Fatal("NewMLXClient() returned nil")
		}
		defer client.Close()
	})

	t.Run("with nil config uses defaults", func(t *testing.T) {
		client, err := NewMLXClient(nil, logger, nil)
		if err != nil {
			t.Fatalf("NewMLXClient() error = %v", err)
		}
		if client == nil {
			t.Fatal("NewMLXClient() returned nil")
		}
		defer client.Close()
	})

	t.Run("with invalid config returns error", func(t *testing.T) {
		cfg := &Config{} // Invalid - missing required fields
		_, err := NewMLXClient(cfg, logger, nil)
		if err == nil {
			t.Error("NewMLXClient() expected error for invalid config")
		}
	})

	t.Run("with cache", func(t *testing.T) {
		cfg := DefaultConfig()
		cache := NewMemoryCache(nil)
		client, err := NewMLXClient(cfg, logger, cache)
		if err != nil {
			t.Fatalf("NewMLXClient() error = %v", err)
		}
		if client == nil {
			t.Fatal("NewMLXClient() returned nil")
		}
		defer client.Close()
	})
}

func TestMLXClient_Embed_EmptyText(t *testing.T) {
	logger := newTestLogger()
	cfg := DefaultConfig()
	client, _ := NewMLXClient(cfg, logger, nil)
	defer client.Close()

	_, err := client.Embed(context.Background(), "")
	if !errors.Is(err, ErrEmptyText) {
		t.Errorf("Embed(\"\") error = %v, want ErrEmptyText", err)
	}

	_, err = client.Embed(context.Background(), "   ")
	if !errors.Is(err, ErrEmptyText) {
		t.Errorf("Embed(\"   \") error = %v, want ErrEmptyText", err)
	}
}

func TestMLXClient_Embed_WithMockServer(t *testing.T) {
	logger := newTestLogger()

	t.Run("successful OpenAI-compatible response", func(t *testing.T) {
		embedding := make([]float64, 1024)
		for i := range embedding {
			embedding[i] = float64(i) / 1024.0
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/embeddings" {
				resp := openaiEmbedResponse{
					Object: "list",
					Data: []struct {
						Object    string    `json:"object"`
						Embedding []float64 `json:"embedding"`
						Index     int       `json:"index"`
					}{
						{Object: "embedding", Embedding: embedding, Index: 0},
					},
					Model: "mxbai-embed-large-v1",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			}
		}))
		defer server.Close()

		cfg := &Config{
			ServerURL:  server.URL,
			Model:      DefaultModel,
			BatchSize:  DefaultBatchSize,
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0, // No retries for testing
			RetryDelay: DefaultRetryDelay,
		}

		client, err := NewMLXClient(cfg, logger, nil)
		if err != nil {
			t.Fatalf("NewMLXClient() error = %v", err)
		}
		defer client.Close()

		result, err := client.Embed(context.Background(), "test text")
		if err != nil {
			t.Fatalf("Embed() error = %v", err)
		}

		if len(result) != 1024 {
			t.Errorf("Embed() returned %d dimensions, want 1024", len(result))
		}
	})

	t.Run("fallback to Ollama endpoint", func(t *testing.T) {
		embedding := make([]float64, 1024)
		for i := range embedding {
			embedding[i] = float64(i) / 1024.0
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/embeddings" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.URL.Path == "/api/embeddings" {
				resp := ollamaEmbedResponse{
					Embedding: embedding,
					Model:     "mxbai-embed-large-v1",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			}
		}))
		defer server.Close()

		cfg := &Config{
			ServerURL:  server.URL,
			Model:      DefaultModel,
			BatchSize:  DefaultBatchSize,
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0,
			RetryDelay: DefaultRetryDelay,
		}

		client, err := NewMLXClient(cfg, logger, nil)
		if err != nil {
			t.Fatalf("NewMLXClient() error = %v", err)
		}
		defer client.Close()

		result, err := client.Embed(context.Background(), "test text")
		if err != nil {
			t.Fatalf("Embed() error = %v", err)
		}

		if len(result) != 1024 {
			t.Errorf("Embed() returned %d dimensions, want 1024", len(result))
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer server.Close()

		cfg := &Config{
			ServerURL:  server.URL,
			Model:      DefaultModel,
			BatchSize:  DefaultBatchSize,
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0,
			RetryDelay: DefaultRetryDelay,
		}

		client, err := NewMLXClient(cfg, logger, nil)
		if err != nil {
			t.Fatalf("NewMLXClient() error = %v", err)
		}
		defer client.Close()

		_, err = client.Embed(context.Background(), "test text")
		if err == nil {
			t.Error("Embed() expected error for server error")
		}
	})

	t.Run("invalid response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("not valid json"))
		}))
		defer server.Close()

		cfg := &Config{
			ServerURL:  server.URL,
			Model:      DefaultModel,
			BatchSize:  DefaultBatchSize,
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0,
			RetryDelay: DefaultRetryDelay,
		}

		client, err := NewMLXClient(cfg, logger, nil)
		if err != nil {
			t.Fatalf("NewMLXClient() error = %v", err)
		}
		defer client.Close()

		_, err = client.Embed(context.Background(), "test text")
		if err == nil {
			t.Error("Embed() expected error for invalid response")
		}
	})
}

func TestMLXClient_Embed_WithCache(t *testing.T) {
	logger := newTestLogger()
	cache := NewMemoryCache(nil)

	embedding := make([]float64, 1024)
	for i := range embedding {
		embedding[i] = float64(i) / 1024.0
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		resp := openaiEmbedResponse{
			Object: "list",
			Data: []struct {
				Object    string    `json:"object"`
				Embedding []float64 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Object: "embedding", Embedding: embedding, Index: 0},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &Config{
		ServerURL:  server.URL,
		Model:      DefaultModel,
		BatchSize:  DefaultBatchSize,
		Timeout:    DefaultTimeout,
		Dimensions: DefaultDimensions,
		MaxRetries: 0,
		RetryDelay: DefaultRetryDelay,
	}

	client, _ := NewMLXClient(cfg, logger, cache)
	defer client.Close()

	// First call - should hit server
	_, err := client.Embed(context.Background(), "test text")
	if err != nil {
		t.Fatalf("First Embed() error = %v", err)
	}
	if requestCount != 1 {
		t.Errorf("Expected 1 server request, got %d", requestCount)
	}

	// Second call - should hit cache
	_, err = client.Embed(context.Background(), "test text")
	if err != nil {
		t.Fatalf("Second Embed() error = %v", err)
	}
	if requestCount != 1 {
		t.Errorf("Expected 1 server request (cached), got %d", requestCount)
	}

	// Different text - should hit server
	_, err = client.Embed(context.Background(), "different text")
	if err != nil {
		t.Fatalf("Third Embed() error = %v", err)
	}
	if requestCount != 2 {
		t.Errorf("Expected 2 server requests, got %d", requestCount)
	}
}

func TestMLXClient_Embed_ContextCanceled(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{
		ServerURL:  server.URL,
		Model:      DefaultModel,
		BatchSize:  DefaultBatchSize,
		Timeout:    5 * time.Second,
		Dimensions: DefaultDimensions,
		MaxRetries: 0,
		RetryDelay: DefaultRetryDelay,
	}

	client, _ := NewMLXClient(cfg, logger, nil)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Embed(ctx, "test text")
	if err == nil {
		t.Error("Embed() expected error for canceled context")
	}
}

func TestMLXClient_BatchEmbed(t *testing.T) {
	logger := newTestLogger()

	t.Run("empty batch", func(t *testing.T) {
		cfg := DefaultConfig()
		client, _ := NewMLXClient(cfg, logger, nil)
		defer client.Close()

		_, err := client.BatchEmbed(context.Background(), []string{})
		if !errors.Is(err, ErrEmptyBatch) {
			t.Errorf("BatchEmbed([]) error = %v, want ErrEmptyBatch", err)
		}
	})

	t.Run("batch too large", func(t *testing.T) {
		cfg := &Config{
			ServerURL:  DefaultServerURL,
			Model:      DefaultModel,
			BatchSize:  2, // Small batch size for testing
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0,
			RetryDelay: DefaultRetryDelay,
		}
		client, _ := NewMLXClient(cfg, logger, nil)
		defer client.Close()

		_, err := client.BatchEmbed(context.Background(), []string{"a", "b", "c"})
		if err == nil || !errors.Is(err, ErrBatchTooLarge) {
			t.Errorf("BatchEmbed() error = %v, want ErrBatchTooLarge", err)
		}
	})

	t.Run("empty text in batch", func(t *testing.T) {
		cfg := DefaultConfig()
		client, _ := NewMLXClient(cfg, logger, nil)
		defer client.Close()

		_, err := client.BatchEmbed(context.Background(), []string{"valid", "", "text"})
		if err == nil {
			t.Error("BatchEmbed() expected error for empty text in batch")
		}
	})

	t.Run("successful batch", func(t *testing.T) {
		embeddings := make([][]float64, 2)
		for i := range embeddings {
			embeddings[i] = make([]float64, 1024)
			for j := range embeddings[i] {
				embeddings[i][j] = float64(i*1024+j) / 2048.0
			}
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := openaiEmbedResponse{
				Object: "list",
				Data: []struct {
					Object    string    `json:"object"`
					Embedding []float64 `json:"embedding"`
					Index     int       `json:"index"`
				}{
					{Object: "embedding", Embedding: embeddings[0], Index: 0},
					{Object: "embedding", Embedding: embeddings[1], Index: 1},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := &Config{
			ServerURL:  server.URL,
			Model:      DefaultModel,
			BatchSize:  DefaultBatchSize,
			Timeout:    DefaultTimeout,
			Dimensions: DefaultDimensions,
			MaxRetries: 0,
			RetryDelay: DefaultRetryDelay,
		}

		client, _ := NewMLXClient(cfg, logger, nil)
		defer client.Close()

		results, err := client.BatchEmbed(context.Background(), []string{"text1", "text2"})
		if err != nil {
			t.Fatalf("BatchEmbed() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("BatchEmbed() returned %d results, want 2", len(results))
		}
	})
}

func TestMLXClient_Dimensions(t *testing.T) {
	logger := newTestLogger()
	cfg := &Config{
		ServerURL:  DefaultServerURL,
		Model:      DefaultModel,
		BatchSize:  DefaultBatchSize,
		Timeout:    DefaultTimeout,
		Dimensions: 512, // Custom dimensions
		MaxRetries: 0,
		RetryDelay: DefaultRetryDelay,
	}

	client, _ := NewMLXClient(cfg, logger, nil)
	defer client.Close()

	if client.Dimensions() != 512 {
		t.Errorf("Dimensions() = %d, want 512", client.Dimensions())
	}
}

func TestMLXClient_ModelInfo(t *testing.T) {
	logger := newTestLogger()
	cfg := DefaultConfig()
	client, _ := NewMLXClient(cfg, logger, nil)
	defer client.Close()

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

func TestMLXClient_Close(t *testing.T) {
	logger := newTestLogger()
	cfg := DefaultConfig()
	client, _ := NewMLXClient(cfg, logger, nil)

	err := client.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestMockClient(t *testing.T) {
	t.Run("default behavior", func(t *testing.T) {
		client := NewMockClient(512, nil)
		defer client.Close()

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
		defer client.Close()

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
		defer client.Close()

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

func TestToFloat32(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := toFloat32([]float64{})
		if len(result) != 0 {
			t.Errorf("toFloat32([]) length = %d, want 0", len(result))
		}
	})

	t.Run("conversion", func(t *testing.T) {
		input := []float64{1.5, 2.5, 3.5}
		result := toFloat32(input)

		if len(result) != 3 {
			t.Fatalf("toFloat32() length = %d, want 3", len(result))
		}
		if result[0] != 1.5 {
			t.Errorf("result[0] = %f, want 1.5", result[0])
		}
		if result[1] != 2.5 {
			t.Errorf("result[1] = %f, want 2.5", result[1])
		}
		if result[2] != 3.5 {
			t.Errorf("result[2] = %f, want 3.5", result[2])
		}
	})
}

// roundTripFunc is a helper for testing HTTP clients
type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestOllamaRequestFormats(t *testing.T) {
	t.Run("ollamaEmbedRequest marshals correctly", func(t *testing.T) {
		req := ollamaEmbedRequest{
			Model:  "test-model",
			Prompt: "test prompt",
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}

		var parsed map[string]string
		json.Unmarshal(data, &parsed)

		if parsed["model"] != "test-model" {
			t.Errorf("model = %q, want %q", parsed["model"], "test-model")
		}
		if parsed["prompt"] != "test prompt" {
			t.Errorf("prompt = %q, want %q", parsed["prompt"], "test prompt")
		}
	})

	t.Run("openaiEmbedRequest marshals correctly", func(t *testing.T) {
		req := openaiEmbedRequest{
			Model: "test-model",
			Input: "test input",
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}

		var parsed map[string]interface{}
		json.Unmarshal(data, &parsed)

		if parsed["model"] != "test-model" {
			t.Errorf("model = %v, want %q", parsed["model"], "test-model")
		}
		if parsed["input"] != "test input" {
			t.Errorf("input = %v, want %q", parsed["input"], "test input")
		}
	})
}

// Helper function for creating test responses
func createTestResponse(statusCode int, body interface{}) *http.Response {
	var bodyReader io.ReadCloser
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		bodyReader = io.NopCloser(bytes.NewReader(jsonBody))
	} else {
		bodyReader = io.NopCloser(bytes.NewReader([]byte{}))
	}

	return &http.Response{
		StatusCode: statusCode,
		Body:       bodyReader,
		Header:     make(http.Header),
	}
}
