// Package backend provides backend connectors for AI services.
package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// MLXBackend connects to MLX servers for embeddings and LLM operations.
// It uses OpenAI-compatible APIs for both embedding and chat completion endpoints.
type MLXBackend struct {
	embeddingsURL string
	llmURL        string
	httpClient    *http.Client

	defaultEmbeddingModel string
	defaultLLMModel       string
	embeddingDimensions   int
}

// MLXConfig holds configuration for the MLX backend.
type MLXConfig struct {
	// EmbeddingsURL is the base URL for the embeddings server.
	// Default: http://localhost:8081
	EmbeddingsURL string

	// LLMURL is the base URL for the LLM server.
	// Default: http://localhost:8080
	LLMURL string

	// DefaultEmbeddingModel is the default model for embeddings.
	// Default: mxbai-embed-large-v1
	DefaultEmbeddingModel string

	// DefaultLLMModel is the default model for LLM completions.
	// Default: mlx-community/Qwen2.5-32B-Instruct-4bit
	DefaultLLMModel string

	// EmbeddingDimensions is the expected dimension of embedding vectors.
	// Default: 1024
	EmbeddingDimensions int

	// Timeout is the HTTP request timeout.
	// Default: 120 seconds (LLM requests can be slow)
	Timeout time.Duration
}

// DefaultMLXConfig returns a default MLX configuration.
func DefaultMLXConfig() *MLXConfig {
	return &MLXConfig{
		EmbeddingsURL:         "http://localhost:8081",
		LLMURL:                "http://localhost:8080",
		DefaultEmbeddingModel: "mxbai-embed-large-v1",
		DefaultLLMModel:       "mlx-community/Qwen2.5-32B-Instruct-4bit",
		EmbeddingDimensions:   1024,
		Timeout:               120 * time.Second,
	}
}

// NewMLXBackend creates a new MLX backend with the given configuration.
// If config is nil, default values are used.
func NewMLXBackend(config *MLXConfig) *MLXBackend {
	if config == nil {
		config = DefaultMLXConfig()
	}

	// Apply defaults for empty values
	if config.EmbeddingsURL == "" {
		config.EmbeddingsURL = "http://localhost:8081"
	}
	if config.LLMURL == "" {
		config.LLMURL = "http://localhost:8080"
	}
	if config.DefaultEmbeddingModel == "" {
		config.DefaultEmbeddingModel = "mxbai-embed-large-v1"
	}
	if config.DefaultLLMModel == "" {
		config.DefaultLLMModel = "mlx-community/Qwen2.5-32B-Instruct-4bit"
	}
	if config.EmbeddingDimensions == 0 {
		config.EmbeddingDimensions = 1024
	}
	if config.Timeout == 0 {
		config.Timeout = 120 * time.Second
	}

	return &MLXBackend{
		embeddingsURL:         strings.TrimSuffix(config.EmbeddingsURL, "/"),
		llmURL:                strings.TrimSuffix(config.LLMURL, "/"),
		defaultEmbeddingModel: config.DefaultEmbeddingModel,
		defaultLLMModel:       config.DefaultLLMModel,
		embeddingDimensions:   config.EmbeddingDimensions,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// OpenAI-compatible request/response types for embeddings.

type openaiEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type openaiEmbedResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// OpenAI-compatible request/response types for chat completions.

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float32       `json:"temperature,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// GenerateEmbedding creates a vector embedding for the given text.
func (b *MLXBackend) GenerateEmbedding(ctx context.Context, text string, model string) (*EmbeddingResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrEmptyText
	}

	if model == "" {
		model = b.defaultEmbeddingModel
	}

	url := b.embeddingsURL + "/v1/embeddings"

	reqBody := openaiEmbedRequest{
		Model: model,
		Input: text,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", ErrRequestFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", ErrRequestFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %v", ErrContextCanceled, ctx.Err())
		}
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrRequestFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrRequestFailed, resp.StatusCode, string(body))
	}

	var embedResp openaiEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("%w: parse response: %v", ErrInvalidResponse, err)
	}

	if embedResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrRequestFailed, embedResp.Error.Message)
	}

	if len(embedResp.Data) == 0 {
		return nil, fmt.Errorf("%w: no embedding data returned", ErrInvalidResponse)
	}

	embedding := toFloat32(embedResp.Data[0].Embedding)

	return &EmbeddingResult{
		Vector:     embedding,
		Dimensions: len(embedding),
		Model:      embedResp.Model,
		TokenCount: embedResp.Usage.PromptTokens,
	}, nil
}

// ChatCompletion sends a chat completion request to the LLM.
func (b *MLXBackend) ChatCompletion(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error) {
	if len(messages) == 0 {
		return nil, ErrEmptyMessages
	}

	model := opts.Model
	if model == "" {
		model = b.defaultLLMModel
	}

	url := b.llmURL + "/v1/chat/completions"

	// Convert messages
	chatMsgs := make([]chatMessage, len(messages))
	for i, m := range messages {
		chatMsgs[i] = chatMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	reqBody := chatRequest{
		Model:    model,
		Messages: chatMsgs,
	}

	if opts.MaxTokens > 0 {
		reqBody.MaxTokens = opts.MaxTokens
	} else {
		reqBody.MaxTokens = 2048 // Default
	}

	if opts.Temperature > 0 {
		reqBody.Temperature = opts.Temperature
	} else {
		reqBody.Temperature = 0.1 // Low temperature for structured extraction
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", ErrRequestFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", ErrRequestFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %v", ErrContextCanceled, ctx.Err())
		}
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrRequestFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrRequestFailed, resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("%w: parse response: %v", ErrInvalidResponse, err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrRequestFailed, chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, ErrNoChoices
	}

	return &CompletionResult{
		Content:      chatResp.Choices[0].Message.Content,
		Model:        chatResp.Model,
		InputTokens:  chatResp.Usage.PromptTokens,
		OutputTokens: chatResp.Usage.CompletionTokens,
	}, nil
}

// CheckEmbeddingsHealth checks if the embeddings service is healthy.
func (b *MLXBackend) CheckEmbeddingsHealth(ctx context.Context) error {
	url := b.embeddingsURL + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%w: create request: %v", ErrRequestFailed, err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%w: %v", ErrContextCanceled, ctx.Err())
		}
		return fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: embeddings health check returned %d", ErrServiceUnavailable, resp.StatusCode)
	}

	return nil
}

// CheckLLMHealth checks if the LLM service is healthy.
func (b *MLXBackend) CheckLLMHealth(ctx context.Context) error {
	url := b.llmURL + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%w: create request: %v", ErrRequestFailed, err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%w: %v", ErrContextCanceled, ctx.Err())
		}
		return fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: LLM health check returned %d", ErrServiceUnavailable, resp.StatusCode)
	}

	return nil
}

// Close releases any resources held by the backend.
func (b *MLXBackend) Close() error {
	b.httpClient.CloseIdleConnections()
	return nil
}

// DefaultEmbeddingModel returns the default embedding model name.
func (b *MLXBackend) DefaultEmbeddingModel() string {
	return b.defaultEmbeddingModel
}

// DefaultLLMModel returns the default LLM model name.
func (b *MLXBackend) DefaultLLMModel() string {
	return b.defaultLLMModel
}

// toFloat32 converts a float64 slice to float32.
func toFloat32(f64 []float64) []float32 {
	f32 := make([]float32, len(f64))
	for i, v := range f64 {
		f32[i] = float32(v)
	}
	return f32
}
