package backend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
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
		if be.defaultLLMModel != "mlx-community/Qwen2.5-7B-Instruct-4bit" {
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

// TestMLXBackend_ErrorClassification tests error classification for timeouts and rate limits.
func TestMLXBackend_ErrorClassification(t *testing.T) {
	t.Run("timeout error returns PipelineError with ErrTimeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			EmbeddingsURL: server.URL,
			Timeout:       100 * time.Millisecond,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := be.GenerateEmbedding(ctx, "test", "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var pe *perrors.PipelineError
		if !errors.As(err, &pe) {
			t.Fatalf("expected PipelineError, got %T", err)
		}
		if pe.Code != perrors.ErrTimeout {
			t.Errorf("expected ErrTimeout, got %s", pe.Code)
		}
		if pe.Stage != "mlx-embedding" {
			t.Errorf("expected stage 'mlx-embedding', got %s", pe.Stage)
		}
	})

	t.Run("rate limit HTTP 429 returns PipelineError with ErrRateLimit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			resp := struct {
				Error *struct {
					Message string `json:"message"`
					Type    string `json:"type"`
				} `json:"error"`
			}{
				Error: &struct {
					Message string `json:"message"`
					Type    string `json:"type"`
				}{
					Message: "rate limit exceeded",
					Type:    "rate_limit_error",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			EmbeddingsURL: server.URL,
		})

		_, err := be.GenerateEmbedding(context.Background(), "test", "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var pe *perrors.PipelineError
		if !errors.As(err, &pe) {
			t.Fatalf("expected PipelineError, got %T", err)
		}
		if pe.Code != perrors.ErrRateLimit {
			t.Errorf("expected ErrRateLimit, got %s", pe.Code)
		}
	})

	t.Run("service unavailable returns PipelineError with ErrModelUnavailable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			EmbeddingsURL: server.URL,
		})

		_, err := be.GenerateEmbedding(context.Background(), "test", "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var pe *perrors.PipelineError
		if !errors.As(err, &pe) {
			t.Fatalf("expected PipelineError, got %T", err)
		}
		if pe.Code != perrors.ErrModelUnavailable {
			t.Errorf("expected ErrModelUnavailable, got %s", pe.Code)
		}
	})
}

// TestMLXBackend_ContextLengthTruncation tests that context length errors trigger
// automatic truncation and retry.
func TestMLXBackend_ContextLengthTruncation(t *testing.T) {
	t.Run("truncates and retries on context length error", func(t *testing.T) {
		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++

			// Parse the request to see what text was sent
			var req openaiEmbedRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}

			// First request: reject with context length error
			// Second request (truncated): accept
			if len(req.Input) > 100 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"the input length exceeds the context length"}}`))
				return
			}

			resp := openaiEmbedResponse{
				Object: "list",
				Data: []struct {
					Object    string    `json:"object"`
					Embedding []float64 `json:"embedding"`
					Index     int       `json:"index"`
				}{
					{Object: "embedding", Embedding: []float64{0.1, 0.2, 0.3}, Index: 0},
				},
				Model: "test-model",
				Usage: struct {
					PromptTokens int `json:"prompt_tokens"`
					TotalTokens  int `json:"total_tokens"`
				}{PromptTokens: 10, TotalTokens: 10},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			EmbeddingsURL: server.URL,
			Timeout:       5 * time.Second,
		})

		// Send text longer than 100 chars to trigger context length error on first try
		longText := strings.Repeat("word ", 30) // 150 chars
		result, err := be.GenerateEmbedding(context.Background(), longText, "")
		if err != nil {
			t.Fatalf("expected success after truncation retry, got: %v", err)
		}

		if requestCount < 2 {
			t.Errorf("expected at least 2 requests (original + truncated), got %d", requestCount)
		}

		if len(result.Vector) != 3 {
			t.Errorf("expected 3 dimensions, got %d", len(result.Vector))
		}
	})

	t.Run("returns ErrContentTooLarge when all truncation attempts fail", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always reject with context length error
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"the input length exceeds the context length"}}`))
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			EmbeddingsURL: server.URL,
			Timeout:       5 * time.Second,
		})

		_, err := be.GenerateEmbedding(context.Background(), "some text that is always too long", "")
		if err == nil {
			t.Fatal("expected error")
		}

		var pe *perrors.PipelineError
		if !errors.As(err, &pe) {
			t.Fatalf("expected PipelineError, got %T: %v", err, err)
		}
		if pe.Code != perrors.ErrContentTooLarge {
			t.Errorf("expected ErrContentTooLarge, got %s", pe.Code)
		}
	})

	t.Run("non-context-length 400 errors are not retried", func(t *testing.T) {
		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid model name"}}`))
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

		if requestCount != 1 {
			t.Errorf("expected exactly 1 request (no retry for non-context-length errors), got %d", requestCount)
		}
	})
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		fraction float64
		wantLen  bool // just check it's shorter
	}{
		{"75% of text", "hello world this is a test string", 0.75, true},
		{"50% of text", "hello world this is a test string", 0.50, true},
		{"100% returns original", "hello world", 1.0, false},
		{"0% returns original", "hello world", 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateText(tt.text, tt.fraction)
			if tt.wantLen {
				if len(result) >= len(tt.text) {
					t.Errorf("expected truncated text to be shorter than %d, got %d", len(tt.text), len(result))
				}
				if len(result) == 0 {
					t.Error("expected non-empty truncated text")
				}
			} else {
				if result != tt.text {
					t.Errorf("expected original text, got %q", result)
				}
			}
		})
	}
}

func TestIsContextLengthBody(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{`{"error":{"message":"the input length exceeds the context length"}}`, true},
		{`{"error":{"message":"Input length exceeds model context"}}`, true},
		{`{"error":{"message":"context length exceeded"}}`, true},
		{`{"error":{"message":"invalid model name"}}`, false},
		{`{"error":{"message":"rate limit exceeded"}}`, false},
		{"internal server error", false},
	}

	for _, tt := range tests {
		t.Run(tt.body[:min(40, len(tt.body))], func(t *testing.T) {
			got := isContextLengthBody(tt.body)
			if got != tt.want {
				t.Errorf("isContextLengthBody(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}

// TestMLXBackend_ChatCompletion_JSONMode tests that JSONMode is forwarded as response_format.
func TestMLXBackend_ChatCompletion_JSONMode(t *testing.T) {
	t.Run("OpenAI path sends response_format when JSONMode true", func(t *testing.T) {
		var receivedBody chatRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}
			resp := chatResponse{
				ID:    "test-id",
				Model: "test-llm",
				Choices: []chatChoice{
					{Index: 0, Message: chatMessage{Role: "assistant", Content: `{"result":"ok"}`}, FinishReason: "stop"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{LLMURL: server.URL, Timeout: 5 * time.Second})

		messages := []Message{{Role: "user", Content: "return JSON"}}
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{JSONMode: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedBody.ResponseFormat == nil {
			t.Fatal("expected response_format to be set, got nil")
		}
		if receivedBody.ResponseFormat.Type != "json_object" {
			t.Errorf("expected response_format type 'json_object', got %q", receivedBody.ResponseFormat.Type)
		}
	})

	t.Run("OpenAI path omits response_format when JSONMode false", func(t *testing.T) {
		var receivedBody chatRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}
			resp := chatResponse{
				ID:    "test-id",
				Model: "test-llm",
				Choices: []chatChoice{
					{Index: 0, Message: chatMessage{Role: "assistant", Content: "hello"}, FinishReason: "stop"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{LLMURL: server.URL, Timeout: 5 * time.Second})

		messages := []Message{{Role: "user", Content: "say hello"}}
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{JSONMode: false})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedBody.ResponseFormat != nil {
			t.Errorf("expected no response_format, got %+v", receivedBody.ResponseFormat)
		}
	})

	t.Run("Ollama path sends format json when JSONMode true", func(t *testing.T) {
		var receivedBody ollamaChatRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
				t.Fatalf("failed to decode request: %v", err)
			}
			resp := ollamaChatResponse{
				Model:   "qwen3:8b",
				Message: chatMessage{Role: "assistant", Content: `{"result":"ok"}`},
				Done:    true,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{LLMURL: server.URL, Timeout: 5 * time.Second})

		messages := []Message{{Role: "user", Content: "return JSON"}}
		// Use qwen3 model to trigger Ollama path
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{
			Model:    "qwen3:8b",
			JSONMode: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if string(receivedBody.Format) != `"json"` {
			t.Errorf("expected format '\"json\"', got %q", receivedBody.Format)
		}
	})
}

// TestMLXBackend_ChatCompletion_ErrorClassification tests error classification for chat completion.
func TestMLXBackend_ChatCompletion_ErrorClassification(t *testing.T) {
	t.Run("timeout error returns PipelineError with ErrTimeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			LLMURL:  server.URL,
			Timeout: 100 * time.Millisecond,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		messages := []Message{{Role: "user", Content: "test"}}
		_, err := be.ChatCompletion(ctx, messages, CompletionOptions{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var pe *perrors.PipelineError
		if !errors.As(err, &pe) {
			t.Fatalf("expected PipelineError, got %T", err)
		}
		if pe.Code != perrors.ErrTimeout {
			t.Errorf("expected ErrTimeout, got %s", pe.Code)
		}
		if pe.Stage != "mlx-llm" {
			t.Errorf("expected stage 'mlx-llm', got %s", pe.Stage)
		}
	})

	t.Run("rate limit returns PipelineError with ErrRateLimit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()

		be := NewMLXBackend(&MLXConfig{
			LLMURL: server.URL,
		})

		messages := []Message{{Role: "user", Content: "test"}}
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var pe *perrors.PipelineError
		if !errors.As(err, &pe) {
			t.Fatalf("expected PipelineError, got %T", err)
		}
		if pe.Code != perrors.ErrRateLimit {
			t.Errorf("expected ErrRateLimit, got %s", pe.Code)
		}
	})
}
