package backend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
)

func TestNewGeminiBackend(t *testing.T) {
	t.Run("with API key in config", func(t *testing.T) {
		cfg := &GeminiConfig{
			APIKey: "test-api-key",
		}
		be, err := NewGeminiBackend(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if be == nil {
			t.Fatal("expected non-nil backend")
		}
		if be.defaultEmbeddingModel != DefaultGeminiEmbeddingModel {
			t.Errorf("expected default embedding model %s, got %s", DefaultGeminiEmbeddingModel, be.defaultEmbeddingModel)
		}
		if be.defaultLLMModel != DefaultGeminiLLMModel {
			t.Errorf("expected default LLM model %s, got %s", DefaultGeminiLLMModel, be.defaultLLMModel)
		}
	})

	t.Run("with API key from environment", func(t *testing.T) {
		_ = os.Setenv("GEMINI_API_KEY", "env-api-key")
		defer func() { _ = os.Unsetenv("GEMINI_API_KEY") }()

		be, err := NewGeminiBackend(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if be == nil {
			t.Fatal("expected non-nil backend")
		}
		if be.apiKey != "env-api-key" {
			t.Errorf("expected API key from env, got %s", be.apiKey)
		}
	})

	t.Run("missing API key returns error", func(t *testing.T) {
		_ = os.Unsetenv("GEMINI_API_KEY")
		_, err := NewGeminiBackend(&GeminiConfig{})
		if err == nil {
			t.Fatal("expected error for missing API key")
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &GeminiConfig{
			APIKey:                "test-key",
			Endpoint:              "https://custom.googleapis.com/v1",
			DefaultEmbeddingModel: "custom-embed",
			DefaultLLMModel:       "custom-llm",
			Timeout:               60 * time.Second,
		}
		be, err := NewGeminiBackend(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if be.endpoint != "https://custom.googleapis.com/v1" {
			t.Errorf("expected custom endpoint, got %s", be.endpoint)
		}
		if be.defaultEmbeddingModel != "custom-embed" {
			t.Errorf("expected custom embedding model, got %s", be.defaultEmbeddingModel)
		}
		if be.defaultLLMModel != "custom-llm" {
			t.Errorf("expected custom LLM model, got %s", be.defaultLLMModel)
		}
	})

	t.Run("strips trailing slash from endpoint", func(t *testing.T) {
		cfg := &GeminiConfig{
			APIKey:   "test-key",
			Endpoint: "https://api.example.com/v1/",
		}
		be, err := NewGeminiBackend(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if be.endpoint != "https://api.example.com/v1" {
			t.Errorf("expected trailing slash stripped, got %s", be.endpoint)
		}
	})
}

func TestGeminiBackend_GenerateEmbedding(t *testing.T) {
	t.Run("successful embedding", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify path contains embedContent
			if r.URL.Path != "/models/text-embedding-004:embedContent" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != http.MethodPost {
				t.Errorf("unexpected method: %s", r.Method)
			}

			// Verify API key in query
			if r.URL.Query().Get("key") != "test-key" {
				t.Errorf("unexpected API key: %s", r.URL.Query().Get("key"))
			}

			// Verify request body
			var req geminiEmbedRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode request: %v", err)
			}
			if len(req.Content.Parts) == 0 || req.Content.Parts[0].Text != "test text" {
				t.Errorf("unexpected request content: %+v", req)
			}

			resp := geminiEmbedResponse{
				Embedding: &geminiEmbedding{
					Values: []float32{0.1, 0.2, 0.3},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, err := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
			Timeout:  5 * time.Second,
		})
		if err != nil {
			t.Fatalf("failed to create backend: %v", err)
		}

		result, err := be.GenerateEmbedding(context.Background(), "test text", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Vector) != 3 {
			t.Errorf("expected 3 dimensions, got %d", len(result.Vector))
		}
		if result.Model != "text-embedding-004" {
			t.Errorf("expected text-embedding-004, got %s", result.Model)
		}
		if result.Dimensions != 3 {
			t.Errorf("expected 3 dimensions, got %d", result.Dimensions)
		}
	})

	t.Run("with custom model", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/models/custom-model:embedContent" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			resp := geminiEmbedResponse{
				Embedding: &geminiEmbedding{
					Values: []float32{0.1},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
		})

		result, err := be.GenerateEmbedding(context.Background(), "test", "custom-model")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Model != "custom-model" {
			t.Errorf("expected custom-model, got %s", result.Model)
		}
	})

	t.Run("empty text returns error", func(t *testing.T) {
		be, _ := NewGeminiBackend(&GeminiConfig{APIKey: "test-key"})
		_, err := be.GenerateEmbedding(context.Background(), "", "")
		if err != ErrEmptyText {
			t.Errorf("expected ErrEmptyText, got %v", err)
		}
	})

	t.Run("whitespace only text returns error", func(t *testing.T) {
		be, _ := NewGeminiBackend(&GeminiConfig{APIKey: "test-key"})
		_, err := be.GenerateEmbedding(context.Background(), "   \n\t  ", "")
		if err != ErrEmptyText {
			t.Errorf("expected ErrEmptyText, got %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    500,
					"message": "internal error",
					"status":  "INTERNAL",
				},
			})
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
			Timeout:  5 * time.Second,
		})

		_, err := be.GenerateEmbedding(context.Background(), "test", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rate limit error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    429,
					"message": "quota exceeded",
					"status":  "RESOURCE_EXHAUSTED",
				},
			})
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
			Timeout:  5 * time.Second,
		})

		_, err := be.GenerateEmbedding(context.Background(), "test", "")
		if err == nil {
			t.Fatal("expected error")
		}
		// Verify error message contains rate limit info
		if !contains(err.Error(), "rate limit") {
			t.Errorf("expected rate limit error, got: %v", err)
		}
	})

	t.Run("authentication error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    401,
					"message": "API key not valid",
					"status":  "UNAUTHENTICATED",
				},
			})
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "invalid-key",
			Endpoint: server.URL,
			Timeout:  5 * time.Second,
		})

		_, err := be.GenerateEmbedding(context.Background(), "test", "")
		if err == nil {
			t.Fatal("expected error")
		}
		if !contains(err.Error(), "authentication") {
			t.Errorf("expected authentication error, got: %v", err)
		}
	})

	t.Run("empty embedding response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := geminiEmbedResponse{} // No embedding
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
		})

		_, err := be.GenerateEmbedding(context.Background(), "test", "")
		if err == nil {
			t.Fatal("expected error for empty embedding")
		}
	})
}

func TestGeminiBackend_ChatCompletion(t *testing.T) {
	t.Run("successful completion", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/models/gemini-2.0-flash:generateContent" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			// Verify request
			var req geminiGenerateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode request: %v", err)
			}

			resp := geminiGenerateResponse{
				Candidates: []geminiCandidate{
					{
						Content: geminiContent{
							Role:  "model",
							Parts: []geminiPart{{Text: "This is the response"}},
						},
						FinishReason: "STOP",
						Index:        0,
					},
				},
				UsageMetadata: &geminiUsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 5,
					TotalTokenCount:      15,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
			Timeout:  5 * time.Second,
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
		if result.Model != "gemini-2.0-flash" {
			t.Errorf("expected gemini-2.0-flash, got %s", result.Model)
		}
		if result.InputTokens != 10 {
			t.Errorf("expected 10 input tokens, got %d", result.InputTokens)
		}
		if result.OutputTokens != 5 {
			t.Errorf("expected 5 output tokens, got %d", result.OutputTokens)
		}
	})

	t.Run("with system message", func(t *testing.T) {
		var receivedReq geminiGenerateRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&receivedReq)

			resp := geminiGenerateResponse{
				Candidates: []geminiCandidate{{
					Content: geminiContent{
						Parts: []geminiPart{{Text: "response"}},
					},
				}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
		})

		messages := []Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello"},
		}
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify system instruction was set
		if receivedReq.SystemInstruction == nil {
			t.Fatal("expected system instruction to be set")
		}
		if receivedReq.SystemInstruction.Parts[0].Text != "You are a helpful assistant." {
			t.Errorf("unexpected system instruction: %v", receivedReq.SystemInstruction)
		}

		// Verify user message is in contents
		if len(receivedReq.Contents) != 1 {
			t.Errorf("expected 1 content, got %d", len(receivedReq.Contents))
		}
		if receivedReq.Contents[0].Role != "user" {
			t.Errorf("expected user role, got %s", receivedReq.Contents[0].Role)
		}
	})

	t.Run("assistant role converted to model", func(t *testing.T) {
		var receivedReq geminiGenerateRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&receivedReq)
			resp := geminiGenerateResponse{
				Candidates: []geminiCandidate{{
					Content: geminiContent{Parts: []geminiPart{{Text: "response"}}},
				}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
		})

		messages := []Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
			{Role: "user", Content: "How are you?"},
		}
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify assistant was converted to model
		if len(receivedReq.Contents) != 3 {
			t.Errorf("expected 3 contents, got %d", len(receivedReq.Contents))
		}
		if receivedReq.Contents[1].Role != "model" {
			t.Errorf("expected 'model' role for assistant, got %s", receivedReq.Contents[1].Role)
		}
	})

	t.Run("JSON mode", func(t *testing.T) {
		var receivedReq geminiGenerateRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&receivedReq)
			resp := geminiGenerateResponse{
				Candidates: []geminiCandidate{{
					Content: geminiContent{Parts: []geminiPart{{Text: `{"key":"value"}`}}},
				}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
		})

		messages := []Message{{Role: "user", Content: "Return JSON"}}
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{
			JSONMode: true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify JSON mode was set
		if receivedReq.GenerationConfig == nil {
			t.Fatal("expected generation config")
		}
		if receivedReq.GenerationConfig.ResponseMimeType != "application/json" {
			t.Errorf("expected application/json, got %s", receivedReq.GenerationConfig.ResponseMimeType)
		}
	})

	t.Run("custom model and options", func(t *testing.T) {
		var receivedReq geminiGenerateRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/models/gemini-2.0-pro:generateContent" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			_ = json.NewDecoder(r.Body).Decode(&receivedReq)
			resp := geminiGenerateResponse{
				Candidates: []geminiCandidate{{
					Content: geminiContent{Parts: []geminiPart{{Text: "response"}}},
				}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
		})

		messages := []Message{{Role: "user", Content: "test"}}
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{
			Model:       "gemini-2.0-pro",
			MaxTokens:   1000,
			Temperature: 0.8,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedReq.GenerationConfig.MaxOutputTokens != 1000 {
			t.Errorf("expected 1000 max tokens, got %d", receivedReq.GenerationConfig.MaxOutputTokens)
		}
		if receivedReq.GenerationConfig.Temperature != 0.8 {
			t.Errorf("expected 0.8 temperature, got %f", receivedReq.GenerationConfig.Temperature)
		}
	})

	t.Run("empty messages returns error", func(t *testing.T) {
		be, _ := NewGeminiBackend(&GeminiConfig{APIKey: "test-key"})
		_, err := be.ChatCompletion(context.Background(), []Message{}, CompletionOptions{})
		if err != ErrEmptyMessages {
			t.Errorf("expected ErrEmptyMessages, got %v", err)
		}
	})

	t.Run("only system messages returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("request should not have been made")
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
		})

		messages := []Message{
			{Role: "system", Content: "You are helpful."},
		}
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{})
		if err == nil {
			t.Fatal("expected error for only system messages")
		}
	})

	t.Run("no choices returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := geminiGenerateResponse{
				Candidates: []geminiCandidate{},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
		})

		messages := []Message{{Role: "user", Content: "Hello"}}
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{})
		if err != ErrNoChoices {
			t.Errorf("expected ErrNoChoices, got %v", err)
		}
	})

	t.Run("API error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := geminiGenerateResponse{
				Error: &geminiError{
					Code:    400,
					Message: "Invalid request",
					Status:  "INVALID_ARGUMENT",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
		})

		messages := []Message{{Role: "user", Content: "Hello"}}
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !contains(err.Error(), "Invalid request") {
			t.Errorf("expected error message to contain 'Invalid request', got: %v", err)
		}
	})
}

func TestGeminiBackend_CheckHealth(t *testing.T) {
	t.Run("embeddings health check success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := geminiEmbedResponse{
				Embedding: &geminiEmbedding{
					Values: []float32{0.1, 0.2},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
			Timeout:  5 * time.Second,
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

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
			Timeout:  5 * time.Second,
		})

		err := be.CheckEmbeddingsHealth(context.Background())
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("LLM health check success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := geminiGenerateResponse{
				Candidates: []geminiCandidate{{
					Content: geminiContent{Parts: []geminiPart{{Text: "ok"}}},
				}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
			Timeout:  5 * time.Second,
		})

		err := be.CheckLLMHealth(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("LLM health check failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
			Timeout:  5 * time.Second,
		})

		err := be.CheckLLMHealth(context.Background())
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestGeminiBackend_DefaultModels(t *testing.T) {
	be, _ := NewGeminiBackend(&GeminiConfig{
		APIKey:                "test-key",
		DefaultEmbeddingModel: "custom-embed",
		DefaultLLMModel:       "custom-llm",
	})

	if be.DefaultEmbeddingModel() != "custom-embed" {
		t.Errorf("expected custom-embed, got %s", be.DefaultEmbeddingModel())
	}
	if be.DefaultLLMModel() != "custom-llm" {
		t.Errorf("expected custom-llm, got %s", be.DefaultLLMModel())
	}
}

func TestGeminiBackend_Close(t *testing.T) {
	be, _ := NewGeminiBackend(&GeminiConfig{APIKey: "test-key"})
	err := be.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGeminiBackend_HandleHTTPError(t *testing.T) {
	be, _ := NewGeminiBackend(&GeminiConfig{APIKey: "test-key"})

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantMsg    string
	}{
		{
			name:       "rate limit",
			statusCode: 429,
			body:       `{"error":{"code":429,"message":"quota exceeded","status":"RESOURCE_EXHAUSTED"}}`,
			wantMsg:    "rate limit",
		},
		{
			name:       "auth error",
			statusCode: 401,
			body:       `{"error":{"code":401,"message":"invalid key","status":"UNAUTHENTICATED"}}`,
			wantMsg:    "authentication",
		},
		{
			name:       "forbidden",
			statusCode: 403,
			body:       `{"error":{"code":403,"message":"access denied","status":"PERMISSION_DENIED"}}`,
			wantMsg:    "access denied",
		},
		{
			name:       "not found",
			statusCode: 404,
			body:       `{"error":{"code":404,"message":"model not found","status":"NOT_FOUND"}}`,
			wantMsg:    "model not found",
		},
		{
			name:       "invalid JSON",
			statusCode: 500,
			body:       "internal server error",
			wantMsg:    "HTTP 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := be.handleHTTPError(tt.statusCode, []byte(tt.body))
			if err == nil {
				t.Fatal("expected error")
			}
			if !contains(err.Error(), tt.wantMsg) {
				t.Errorf("expected error containing %q, got: %v", tt.wantMsg, err)
			}
		})
	}
}

func TestGeminiBackend_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Simulate slow response
	}))
	defer server.Close()

	be, _ := NewGeminiBackend(&GeminiConfig{
		APIKey:   "test-key",
		Endpoint: server.URL,
		Timeout:  10 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := be.GenerateEmbedding(ctx, "test", "")
	if err == nil {
		t.Fatal("expected error")
	}
	// Should be context timeout error, wrapped in PipelineError
	var pe *perrors.PipelineError
	if errors.As(err, &pe) {
		// Wrapped as PipelineError
		if pe.Code != perrors.ErrTimeout && pe.Code != perrors.ErrContextCancelled {
			t.Errorf("expected ErrTimeout or ErrContextCancelled, got: %s", pe.Code)
		}
	} else {
		// Fallback check for legacy behavior (though should be wrapped)
		if !contains(err.Error(), "context") && !contains(err.Error(), "deadline") {
			t.Errorf("expected context error, got: %v", err)
		}
	}
}

// Helper function to check if a string contains a substring (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && len(substr) > 0 &&
			(s[0:len(substr)] == substr ||
				contains(s[1:], substr)))
}

// TestGeminiBackend_ErrorClassification tests error classification for timeouts and rate limits.
func TestGeminiBackend_ErrorClassification(t *testing.T) {
	t.Run("timeout error returns PipelineError with ErrTimeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond) // Exceed context deadline
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
			Timeout:  100 * time.Millisecond,
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
		if pe.Stage != "gemini-embedding" {
			t.Errorf("expected stage 'gemini-embedding', got %s", pe.Stage)
		}
	})

	t.Run("rate limit HTTP 429 returns PipelineError with ErrRateLimit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			resp := struct {
				Error *geminiError `json:"error"`
			}{
				Error: &geminiError{
					Code:    429,
					Message: "quota exceeded",
					Status:  "RESOURCE_EXHAUSTED",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
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

	t.Run("RESOURCE_EXHAUSTED status returns PipelineError with ErrRateLimit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			resp := struct {
				Error *geminiError `json:"error"`
			}{
				Error: &geminiError{
					Code:    500,
					Message: "resource limit reached",
					Status:  "RESOURCE_EXHAUSTED",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
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
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			resp := struct {
				Error *geminiError `json:"error"`
			}{
				Error: &geminiError{
					Code:    503,
					Message: "service temporarily unavailable",
					Status:  "UNAVAILABLE",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
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

// TestGeminiBackend_ChatCompletion_ErrorClassification tests error classification for chat completion.
func TestGeminiBackend_ChatCompletion_ErrorClassification(t *testing.T) {
	t.Run("timeout error returns PipelineError with ErrTimeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
			Timeout:  100 * time.Millisecond,
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
		if pe.Stage != "gemini-llm" {
			t.Errorf("expected stage 'gemini-llm', got %s", pe.Stage)
		}
	})

	t.Run("rate limit returns PipelineError with ErrRateLimit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			resp := struct {
				Error *geminiError `json:"error"`
			}{
				Error: &geminiError{
					Code:    429,
					Message: "rate limit exceeded",
					Status:  "RESOURCE_EXHAUSTED",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewGeminiBackend(&GeminiConfig{
			APIKey:   "test-key",
			Endpoint: server.URL,
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
