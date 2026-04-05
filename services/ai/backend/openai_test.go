package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestDefaultOpenAIConfig(t *testing.T) {
	cfg := DefaultOpenAIConfig()

	if cfg.Endpoint != "https://api.openai.com/v1" {
		t.Errorf("unexpected endpoint: %s", cfg.Endpoint)
	}
	if cfg.DefaultEmbeddingModel != "text-embedding-3-small" {
		t.Errorf("unexpected embedding model: %s", cfg.DefaultEmbeddingModel)
	}
	if cfg.DefaultLLMModel != "gpt-4o-mini" {
		t.Errorf("unexpected LLM model: %s", cfg.DefaultLLMModel)
	}
	if cfg.Timeout != 120*time.Second {
		t.Errorf("unexpected timeout: %v", cfg.Timeout)
	}
}

func TestNewOpenAIBackend(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		be, err := NewOpenAIBackend(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if be == nil {
			t.Fatal("expected non-nil backend")
		}
		if be.defaultEmbeddingModel != "text-embedding-3-small" {
			t.Errorf("expected default embedding model, got %s", be.defaultEmbeddingModel)
		}
		if be.defaultLLMModel != "gpt-4o-mini" {
			t.Errorf("expected default LLM model, got %s", be.defaultLLMModel)
		}
		if be.endpoint != "https://api.openai.com/v1" {
			t.Errorf("expected default endpoint, got %s", be.endpoint)
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &OpenAIConfig{
			APIKey:                "test-key",
			Endpoint:              "https://custom.openai.com/v1",
			Organization:          "test-org",
			DefaultEmbeddingModel: "custom-embed",
			DefaultLLMModel:       "custom-llm",
			Timeout:               60 * time.Second,
		}
		be, err := NewOpenAIBackend(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if be.endpoint != "https://custom.openai.com/v1" {
			t.Errorf("expected custom endpoint, got %s", be.endpoint)
		}
		if be.apiKey != "test-key" {
			t.Errorf("expected test-key, got %s", be.apiKey)
		}
		if be.orgID != "test-org" {
			t.Errorf("expected test-org, got %s", be.orgID)
		}
		if be.defaultEmbeddingModel != "custom-embed" {
			t.Errorf("expected custom embedding model, got %s", be.defaultEmbeddingModel)
		}
	})

	t.Run("strips trailing slash from endpoint", func(t *testing.T) {
		cfg := &OpenAIConfig{
			Endpoint: "https://api.openai.com/v1/",
		}
		be, err := NewOpenAIBackend(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if be.endpoint != "https://api.openai.com/v1" {
			t.Errorf("expected trailing slash stripped, got %s", be.endpoint)
		}
	})

	t.Run("reads API key from environment", func(t *testing.T) {
		oldKey := os.Getenv("OPENAI_API_KEY")
		_ = os.Setenv("OPENAI_API_KEY", "env-api-key")
		defer func() { _ = os.Setenv("OPENAI_API_KEY", oldKey) }()

		be, err := NewOpenAIBackend(&OpenAIConfig{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if be.apiKey != "env-api-key" {
			t.Errorf("expected env-api-key, got %s", be.apiKey)
		}
	})

	t.Run("config API key overrides environment", func(t *testing.T) {
		oldKey := os.Getenv("OPENAI_API_KEY")
		_ = os.Setenv("OPENAI_API_KEY", "env-api-key")
		defer func() { _ = os.Setenv("OPENAI_API_KEY", oldKey) }()

		be, err := NewOpenAIBackend(&OpenAIConfig{
			APIKey: "config-api-key",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if be.apiKey != "config-api-key" {
			t.Errorf("expected config-api-key, got %s", be.apiKey)
		}
	})
}

func TestOpenAIBackend_GenerateEmbedding(t *testing.T) {
	t.Run("successful embedding", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request
			if r.URL.Path != "/v1/embeddings" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Method != http.MethodPost {
				t.Errorf("unexpected method: %s", r.Method)
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
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
						Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
						Index:     0,
					},
				},
				Model: "text-embedding-3-small",
				Usage: struct {
					PromptTokens int `json:"prompt_tokens"`
					TotalTokens  int `json:"total_tokens"`
				}{
					PromptTokens: 3,
					TotalTokens:  3,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, err := NewOpenAIBackend(&OpenAIConfig{
			APIKey:   "test-key",
			Endpoint: server.URL + "/v1",
			Timeout:  5 * time.Second,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result, err := be.GenerateEmbedding(context.Background(), "test text", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Vector) != 5 {
			t.Errorf("expected 5 dimensions, got %d", len(result.Vector))
		}
		if result.Model != "text-embedding-3-small" {
			t.Errorf("expected text-embedding-3-small, got %s", result.Model)
		}
		if result.TokenCount != 3 {
			t.Errorf("expected 3 tokens, got %d", result.TokenCount)
		}
	})

	t.Run("empty text returns error", func(t *testing.T) {
		be, _ := NewOpenAIBackend(nil)
		_, err := be.GenerateEmbedding(context.Background(), "", "")
		if err != ErrEmptyText {
			t.Errorf("expected ErrEmptyText, got %v", err)
		}
	})

	t.Run("whitespace only text returns error", func(t *testing.T) {
		be, _ := NewOpenAIBackend(nil)
		_, err := be.GenerateEmbedding(context.Background(), "   \n\t  ", "")
		if err != ErrEmptyText {
			t.Errorf("expected ErrEmptyText, got %v", err)
		}
	})

	t.Run("uses custom model when specified", func(t *testing.T) {
		var requestedModel string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req openaiEmbedRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			requestedModel = req.Model

			resp := openaiEmbedResponse{
				Data: []struct {
					Object    string    `json:"object"`
					Embedding []float64 `json:"embedding"`
					Index     int       `json:"index"`
				}{
					{Embedding: []float64{0.1}},
				},
				Model: req.Model,
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewOpenAIBackend(&OpenAIConfig{
			Endpoint: server.URL + "/v1",
			Timeout:  5 * time.Second,
		})

		_, _ = be.GenerateEmbedding(context.Background(), "test", "text-embedding-3-large")
		if requestedModel != "text-embedding-3-large" {
			t.Errorf("expected text-embedding-3-large, got %s", requestedModel)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "internal error",
					"type":    "server_error",
				},
			})
		}))
		defer server.Close()

		be, _ := NewOpenAIBackend(&OpenAIConfig{
			Endpoint: server.URL + "/v1",
			Timeout:  5 * time.Second,
		})

		_, err := be.GenerateEmbedding(context.Background(), "test", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("authentication error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "Invalid API key",
					"type":    "invalid_request_error",
				},
			})
		}))
		defer server.Close()

		be, _ := NewOpenAIBackend(&OpenAIConfig{
			APIKey:   "invalid-key",
			Endpoint: server.URL + "/v1",
			Timeout:  5 * time.Second,
		})

		_, err := be.GenerateEmbedding(context.Background(), "test", "")
		if err == nil {
			t.Fatal("expected error")
		}
		// Should contain authentication failed
		if !containsError(err, ErrAuthenticationFailed) {
			t.Errorf("expected authentication error, got %v", err)
		}
	})

	t.Run("rate limit error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
				},
			})
		}))
		defer server.Close()

		be, _ := NewOpenAIBackend(&OpenAIConfig{
			Endpoint: server.URL + "/v1",
			Timeout:  5 * time.Second,
		})

		_, err := be.GenerateEmbedding(context.Background(), "test", "")
		if err == nil {
			t.Fatal("expected error")
		}
		if !containsError(err, ErrRateLimited) {
			t.Errorf("expected rate limit error, got %v", err)
		}
	})
}

func TestOpenAIBackend_ChatCompletion(t *testing.T) {
	t.Run("successful completion", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/chat/completions" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			resp := openaiChatResponse{
				ID:      "chatcmpl-123",
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   "gpt-4o-mini",
				Choices: []openaiChatChoice{
					{
						Index: 0,
						Message: openaiMessage{
							Role:    "assistant",
							Content: "Hello! How can I help you?",
						},
						FinishReason: "stop",
					},
				},
				Usage: openaiChatUsage{
					PromptTokens:     10,
					CompletionTokens: 8,
					TotalTokens:      18,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewOpenAIBackend(&OpenAIConfig{
			APIKey:   "test-key",
			Endpoint: server.URL + "/v1",
			Timeout:  5 * time.Second,
		})

		messages := []Message{
			{Role: "user", Content: "Hello"},
		}
		result, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Content != "Hello! How can I help you?" {
			t.Errorf("unexpected content: %s", result.Content)
		}
		if result.Model != "gpt-4o-mini" {
			t.Errorf("expected gpt-4o-mini, got %s", result.Model)
		}
		if result.InputTokens != 10 {
			t.Errorf("expected 10 input tokens, got %d", result.InputTokens)
		}
		if result.OutputTokens != 8 {
			t.Errorf("expected 8 output tokens, got %d", result.OutputTokens)
		}
	})

	t.Run("empty messages returns error", func(t *testing.T) {
		be, _ := NewOpenAIBackend(nil)
		_, err := be.ChatCompletion(context.Background(), []Message{}, CompletionOptions{})
		if err != ErrEmptyMessages {
			t.Errorf("expected ErrEmptyMessages, got %v", err)
		}
	})

	t.Run("no choices returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := openaiChatResponse{
				ID:      "chatcmpl-123",
				Choices: []openaiChatChoice{},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewOpenAIBackend(&OpenAIConfig{
			Endpoint: server.URL + "/v1",
			Timeout:  5 * time.Second,
		})

		messages := []Message{{Role: "user", Content: "Hello"}}
		_, err := be.ChatCompletion(context.Background(), messages, CompletionOptions{})
		if err != ErrNoChoices {
			t.Errorf("expected ErrNoChoices, got %v", err)
		}
	})

	t.Run("JSON mode sets response_format", func(t *testing.T) {
		var requestBody openaiChatRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&requestBody)

			resp := openaiChatResponse{
				ID: "chatcmpl-123",
				Choices: []openaiChatChoice{
					{Message: openaiMessage{Content: `{"result": "ok"}`}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewOpenAIBackend(&OpenAIConfig{
			Endpoint: server.URL + "/v1",
			Timeout:  5 * time.Second,
		})

		messages := []Message{{Role: "user", Content: "Return JSON"}}
		_, _ = be.ChatCompletion(context.Background(), messages, CompletionOptions{
			JSONMode: true,
		})

		if requestBody.ResponseFormat == nil {
			t.Fatal("expected response_format to be set")
		}
		if requestBody.ResponseFormat.Type != "json_object" {
			t.Errorf("expected json_object, got %s", requestBody.ResponseFormat.Type)
		}
	})

	t.Run("custom model is used", func(t *testing.T) {
		var requestedModel string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req openaiChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			requestedModel = req.Model

			resp := openaiChatResponse{
				ID:    "chatcmpl-123",
				Model: req.Model,
				Choices: []openaiChatChoice{
					{Message: openaiMessage{Content: "Response"}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewOpenAIBackend(&OpenAIConfig{
			Endpoint: server.URL + "/v1",
			Timeout:  5 * time.Second,
		})

		messages := []Message{{Role: "user", Content: "Hello"}}
		_, _ = be.ChatCompletion(context.Background(), messages, CompletionOptions{
			Model: "gpt-4o",
		})

		if requestedModel != "gpt-4o" {
			t.Errorf("expected gpt-4o, got %s", requestedModel)
		}
	})

	t.Run("temperature and max_tokens are set", func(t *testing.T) {
		var requestBody openaiChatRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&requestBody)

			resp := openaiChatResponse{
				ID:      "chatcmpl-123",
				Choices: []openaiChatChoice{{Message: openaiMessage{Content: "Hi"}}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		be, _ := NewOpenAIBackend(&OpenAIConfig{
			Endpoint: server.URL + "/v1",
			Timeout:  5 * time.Second,
		})

		messages := []Message{{Role: "user", Content: "Hello"}}
		_, _ = be.ChatCompletion(context.Background(), messages, CompletionOptions{
			MaxTokens:   500,
			Temperature: 0.7,
		})

		if requestBody.MaxTokens != 500 {
			t.Errorf("expected max_tokens 500, got %d", requestBody.MaxTokens)
		}
		if requestBody.Temperature != 0.7 {
			t.Errorf("expected temperature 0.7, got %f", requestBody.Temperature)
		}
	})
}

func TestOpenAIBackend_CheckHealth(t *testing.T) {
	t.Run("health check success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/models" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]string{
					{"id": "gpt-4o"},
				},
			})
		}))
		defer server.Close()

		be, _ := NewOpenAIBackend(&OpenAIConfig{
			APIKey:   "test-key",
			Endpoint: server.URL + "/v1",
			Timeout:  5 * time.Second,
		})

		err := be.CheckEmbeddingsHealth(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		err = be.CheckLLMHealth(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("health check auth failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		be, _ := NewOpenAIBackend(&OpenAIConfig{
			Endpoint: server.URL + "/v1",
			Timeout:  5 * time.Second,
		})

		err := be.CheckEmbeddingsHealth(context.Background())
		if err == nil {
			t.Error("expected error")
		}
		if !containsError(err, ErrAuthenticationFailed) {
			t.Errorf("expected authentication error, got %v", err)
		}
	})

	t.Run("health check server unavailable", func(t *testing.T) {
		be, _ := NewOpenAIBackend(&OpenAIConfig{
			Endpoint: "http://localhost:1", // Invalid port
			Timeout:  100 * time.Millisecond,
		})

		err := be.CheckEmbeddingsHealth(context.Background())
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestOpenAIBackend_AzureConfig(t *testing.T) {
	t.Run("Azure URL format", func(t *testing.T) {
		be, _ := NewOpenAIBackend(&OpenAIConfig{
			APIKey:          "azure-key",
			Endpoint:        "https://myresource.openai.azure.com/openai/deployments/gpt-4",
			AzureDeployment: "gpt-4",
			AzureAPIVersion: "2024-02-01",
		})

		url := be.buildURL("/chat/completions")
		expected := "https://myresource.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=2024-02-01"
		if url != expected {
			t.Errorf("expected %s, got %s", expected, url)
		}
	})

	t.Run("Azure headers", func(t *testing.T) {
		var receivedHeaders http.Header
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeaders = r.Header
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
		}))
		defer server.Close()

		be, _ := NewOpenAIBackend(&OpenAIConfig{
			APIKey:          "azure-key",
			Endpoint:        server.URL + "/v1",
			AzureDeployment: "gpt-4",
			Timeout:         5 * time.Second,
		})

		_ = be.CheckLLMHealth(context.Background())

		// Azure should use api-key header instead of Authorization
		if receivedHeaders.Get("api-key") != "azure-key" {
			t.Errorf("expected api-key header, got: %s", receivedHeaders.Get("api-key"))
		}
		if receivedHeaders.Get("Authorization") != "" {
			t.Errorf("expected no Authorization header for Azure, got: %s", receivedHeaders.Get("Authorization"))
		}
	})
}

func TestOpenAIBackend_OrganizationHeader(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
	}))
	defer server.Close()

	be, _ := NewOpenAIBackend(&OpenAIConfig{
		APIKey:       "test-key",
		Endpoint:     server.URL + "/v1",
		Organization: "org-12345",
		Timeout:      5 * time.Second,
	})

	_ = be.CheckLLMHealth(context.Background())

	if receivedHeaders.Get("OpenAI-Organization") != "org-12345" {
		t.Errorf("expected org header, got: %s", receivedHeaders.Get("OpenAI-Organization"))
	}
}

func TestOpenAIBackend_DefaultModels(t *testing.T) {
	cfg := &OpenAIConfig{
		DefaultEmbeddingModel: "custom-embed",
		DefaultLLMModel:       "custom-llm",
	}
	be, _ := NewOpenAIBackend(cfg)

	if be.DefaultEmbeddingModel() != "custom-embed" {
		t.Errorf("expected custom-embed, got %s", be.DefaultEmbeddingModel())
	}
	if be.DefaultLLMModel() != "custom-llm" {
		t.Errorf("expected custom-llm, got %s", be.DefaultLLMModel())
	}
}

func TestOpenAIBackend_Close(t *testing.T) {
	be, _ := NewOpenAIBackend(nil)
	err := be.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOpenAIBackend_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay response
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	be, _ := NewOpenAIBackend(&OpenAIConfig{
		Endpoint: server.URL + "/v1",
		Timeout:  5 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := be.GenerateEmbedding(ctx, "test", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !containsError(err, ErrContextCanceled) {
		t.Errorf("expected context canceled error, got %v", err)
	}
}

// containsError checks if err contains target error in its chain.
func containsError(err, target error) bool {
	if err == nil {
		return false
	}
	return err.Error() == target.Error() ||
		(len(err.Error()) > len(target.Error()) &&
			err.Error()[:len(target.Error())] == target.Error()[:len(target.Error())])
}
