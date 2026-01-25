package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewMLXBackend(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		be := NewMLXBackend(nil)
		if be == nil {
			t.Fatal("expected non-nil backend")
		}
		if be.defaultEmbeddingModel != "mxbai-embed-large-v1" {
			t.Errorf("expected default embedding model, got %s", be.defaultEmbeddingModel)
		}
		if be.defaultLLMModel != "mlx-community/Qwen2.5-32B-Instruct-4bit" {
			t.Errorf("expected default LLM model, got %s", be.defaultLLMModel)
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &MLXConfig{
			EmbeddingsURL:         "http://custom:8081",
			LLMURL:                "http://custom:8080",
			DefaultEmbeddingModel: "custom-embed",
			DefaultLLMModel:       "custom-llm",
			EmbeddingDimensions:   512,
			Timeout:               60 * time.Second,
		}
		be := NewMLXBackend(cfg)
		if be.embeddingsURL != "http://custom:8081" {
			t.Errorf("expected custom embeddings URL, got %s", be.embeddingsURL)
		}
		if be.defaultEmbeddingModel != "custom-embed" {
			t.Errorf("expected custom embedding model, got %s", be.defaultEmbeddingModel)
		}
	})
}

func TestMLXBackend_GenerateEmbedding(t *testing.T) {
	t.Run("successful embedding", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/embeddings" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != http.MethodPost {
				t.Errorf("unexpected method: %s", r.Method)
			}

			resp := openaiEmbedResponse{
				Object: "list",
				Data: []struct {
					Object    string    `json:"object"`
					Embedding []float64 `json:"embedding"`
					Index     int       `json:"index"`
				}{
					{
						Object:    "embedding",
						Embedding: []float64{0.1, 0.2, 0.3},
						Index:     0,
					},
				},
				Model: "test-model",
				Usage: struct {
					PromptTokens int `json:"prompt_tokens"`
					TotalTokens  int `json:"total_tokens"`
				}{
					PromptTokens: 5,
					TotalTokens:  5,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			EmbeddingsURL: server.URL,
			Timeout:       5 * time.Second,
		})

		result, err := be.GenerateEmbedding(context.Background(), "test text", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Vector) != 3 {
			t.Errorf("expected 3 dimensions, got %d", len(result.Vector))
		}
		if result.Model != "test-model" {
			t.Errorf("expected test-model, got %s", result.Model)
		}
		if result.TokenCount != 5 {
			t.Errorf("expected 5 tokens, got %d", result.TokenCount)
		}
	})

	t.Run("empty text returns error", func(t *testing.T) {
		be := NewMLXBackend(nil)
		_, err := be.GenerateEmbedding(context.Background(), "", "")
		if err != ErrEmptyText {
			t.Errorf("expected ErrEmptyText, got %v", err)
		}
	})

	t.Run("whitespace only text returns error", func(t *testing.T) {
		be := NewMLXBackend(nil)
		_, err := be.GenerateEmbedding(context.Background(), "   \n\t  ", "")
		if err != ErrEmptyText {
			t.Errorf("expected ErrEmptyText, got %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			EmbeddingsURL: server.URL,
			Timeout:       5 * time.Second,
		})

		_, err := be.GenerateEmbedding(context.Background(), "test", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestMLXBackend_ChatCompletion(t *testing.T) {
	t.Run("successful completion", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/chat/completions" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			resp := chatResponse{
				ID:      "test-id",
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   "test-llm",
				Choices: []chatChoice{
					{
						Index: 0,
						Message: chatMessage{
							Role:    "assistant",
							Content: "This is the response",
						},
						FinishReason: "stop",
					},
				},
				Usage: chatUsage{
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			LLMURL:  server.URL,
			Timeout: 5 * time.Second,
		})

		messages := []Message{
			{Role: "user", Content: "Hello"},
		}
		result, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Content != "This is the response" {
			t.Errorf("unexpected content: %s", result.Content)
		}
		if result.Model != "test-llm" {
			t.Errorf("expected test-llm, got %s", result.Model)
		}
		if result.InputTokens != 10 {
			t.Errorf("expected 10 input tokens, got %d", result.InputTokens)
		}
		if result.OutputTokens != 5 {
			t.Errorf("expected 5 output tokens, got %d", result.OutputTokens)
		}
	})

	t.Run("empty messages returns error", func(t *testing.T) {
		be := NewMLXBackend(nil)
		_, err := be.ChatCompletion(context.Background(), []Message{}, CompletionOptions{})
		if err != ErrEmptyMessages {
			t.Errorf("expected ErrEmptyMessages, got %v", err)
		}
	})

	t.Run("no choices returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := chatResponse{
				ID:      "test-id",
				Choices: []chatChoice{},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			LLMURL:  server.URL,
			Timeout: 5 * time.Second,
		})

		messages := []Message{{Role: "user", Content: "Hello"}}
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{})
		if err != ErrNoChoices {
			t.Errorf("expected ErrNoChoices, got %v", err)
		}
	})
}

func TestMLXBackend_CheckHealth(t *testing.T) {
	t.Run("embeddings health check success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/health" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			EmbeddingsURL: server.URL,
			Timeout:       5 * time.Second,
		})

		err := be.CheckEmbeddingsHealth(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("embeddings health check failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			EmbeddingsURL: server.URL,
			Timeout:       5 * time.Second,
		})

		err := be.CheckEmbeddingsHealth(context.Background())
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("LLM health check success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			LLMURL:  server.URL,
			Timeout: 5 * time.Second,
		})

		err := be.CheckLLMHealth(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestMLXBackend_DefaultModels(t *testing.T) {
	cfg := &MLXConfig{
		DefaultEmbeddingModel: "custom-embed",
		DefaultLLMModel:       "custom-llm",
	}
	be := NewMLXBackend(cfg)

	if be.DefaultEmbeddingModel() != "custom-embed" {
		t.Errorf("expected custom-embed, got %s", be.DefaultEmbeddingModel())
	}
	if be.DefaultLLMModel() != "custom-llm" {
		t.Errorf("expected custom-llm, got %s", be.DefaultLLMModel())
	}
}

func TestToFloat32(t *testing.T) {
	f64 := []float64{1.1, 2.2, 3.3}
	f32 := toFloat32(f64)

	if len(f32) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(f32))
	}

	// Check approximate equality (float conversion)
	if f32[0] < 1.0 || f32[0] > 1.2 {
		t.Errorf("unexpected value: %f", f32[0])
	}
	if f32[1] < 2.1 || f32[1] > 2.3 {
		t.Errorf("unexpected value: %f", f32[1])
	}
}
