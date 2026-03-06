// Package backend provides backend connectors for AI services.
package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
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
	// Default: http://localhost:11434
	EmbeddingsURL string

	// LLMURL is the base URL for the LLM server.
	// Default: http://localhost:8080
	LLMURL string

	// DefaultEmbeddingModel is the default model for embeddings.
	// Default: mxbai-embed-large
	DefaultEmbeddingModel string

	// DefaultLLMModel is the default model for LLM completions.
	// Default: mlx-community/Qwen2.5-7B-Instruct-4bit
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
		EmbeddingsURL:         "http://localhost:11434",
		LLMURL:                "http://localhost:8080",
		DefaultEmbeddingModel: "mxbai-embed-large",
		DefaultLLMModel:       "mlx-community/Qwen2.5-7B-Instruct-4bit",
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
		config.EmbeddingsURL = "http://localhost:11434"
	}
	if config.LLMURL == "" {
		config.LLMURL = "http://localhost:8080"
	}
	if config.DefaultEmbeddingModel == "" {
		config.DefaultEmbeddingModel = "mxbai-embed-large"
	}
	if config.DefaultLLMModel == "" {
		config.DefaultLLMModel = "mlx-community/Qwen2.5-7B-Instruct-4bit"
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
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Temperature    float32         `json:"temperature,omitempty"`
	ResponseFormat *chatRespFormat `json:"response_format,omitempty"`
}

type chatRespFormat struct {
	Type       string          `json:"type"`
	JSONSchema *chatJSONSchema `json:"json_schema,omitempty"`
}

type chatJSONSchema struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

// ollamaChatRequest is the native Ollama /api/chat request format.
// Used for models that need features not supported by the OpenAI-compatible endpoint
// (e.g. qwen3 think:false).
type ollamaChatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Think    *bool         `json:"think,omitempty"`
	Options  ollamaOptions `json:"options,omitempty"`
	Format   json.RawMessage `json:"format,omitempty"`
}

type ollamaOptions struct {
	Temperature float32 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

// ollamaChatResponse is the native Ollama /api/chat response format.
type ollamaChatResponse struct {
	Model           string      `json:"model"`
	Message         chatMessage `json:"message"`
	Done            bool        `json:"done"`
	TotalDuration   int64       `json:"total_duration"`
	EvalCount       int         `json:"eval_count"`
	PromptEvalCount int         `json:"prompt_eval_count"`
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

// contextLengthError is a sentinel type for context-length-exceeded responses.
type contextLengthError struct {
	body string
}

func (e *contextLengthError) Error() string {
	return fmt.Sprintf("context length exceeded: %s", e.body)
}

// isContextLengthBody checks if an HTTP response body indicates a context length error.
func isContextLengthBody(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "context length") ||
		strings.Contains(lower, "input length")
}

// truncateText truncates text to the given fraction of its current rune length,
// breaking at the last word boundary before the limit.
func truncateText(text string, fraction float64) string {
	runes := []rune(text)
	target := int(float64(len(runes)) * fraction)
	if target >= len(runes) || target <= 0 {
		return text
	}
	truncated := string(runes[:target])
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > len(truncated)/2 {
		truncated = truncated[:lastSpace]
	}
	return truncated
}

// GenerateEmbedding creates a vector embedding for the given text.
// If the text exceeds the model's context length, it is automatically
// truncated and retried with progressively smaller fractions.
func (b *MLXBackend) GenerateEmbedding(ctx context.Context, text string, model string) (*EmbeddingResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrEmptyText
	}

	if model == "" {
		model = b.defaultEmbeddingModel
	}

	// Try the full text first, then truncate on context length errors.
	fractions := []float64{1.0, 0.75, 0.5}
	currentText := text

	for i, frac := range fractions {
		if i > 0 {
			currentText = strings.TrimSpace(truncateText(text, frac))
			if currentText == "" {
				break
			}
		}

		result, err := b.doEmbeddingRequest(ctx, currentText, model)
		if err == nil {
			return result, nil
		}

		// Only retry on context length errors
		var ctxLenErr *contextLengthError
		if !errors.As(err, &ctxLenErr) {
			return nil, err
		}

		// Last attempt — return a structured error
		if i == len(fractions)-1 {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrContentTooLarge,
				Stage:   "mlx-embedding",
				Message: fmt.Sprintf("text exceeds embedding model context length after truncation to %.0f%%: %s", frac*100, ctxLenErr.body),
				Cause:   ctxLenErr,
			}
		}
	}

	return nil, &perrors.PipelineError{
		Code:    perrors.ErrContentTooLarge,
		Stage:   "mlx-embedding",
		Message: "text exceeds embedding model context length",
	}
}

// doEmbeddingRequest performs a single embedding HTTP request.
// Returns a *contextLengthError when the server rejects the input as too long.
func (b *MLXBackend) doEmbeddingRequest(ctx context.Context, text string, model string) (*EmbeddingResult, error) {
	url := b.embeddingsURL + "/v1/embeddings"

	// Strip provider prefix before sending to Ollama API (e.g. "ollama/mxbai-embed-large" -> "mxbai-embed-large")
	reqBody := openaiEmbedRequest{
		Model: ExtractModelName(model),
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

	start := time.Now()
	resp, err := b.httpClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &perrors.PipelineError{
				Code:     perrors.ErrTimeout,
				Stage:    "mlx-embedding",
				Message:  fmt.Sprintf("MLX embedding request timed out after %s", elapsed.Round(time.Millisecond)),
				Duration: elapsed,
				Cause:    err,
			}
		}
		if ctx.Err() == context.Canceled {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrContextCancelled,
				Stage:   "mlx-embedding",
				Message: "context cancelled",
				Cause:   err,
			}
		}
		// Connection errors indicate service unavailable
		errMsg := err.Error()
		if strings.Contains(strings.ToLower(errMsg), "connection refused") ||
		   strings.Contains(strings.ToLower(errMsg), "no such host") {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrModelUnavailable,
				Stage:   "mlx-embedding",
				Message: errMsg,
				Cause:   err,
			}
		}
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrRequestFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyStr := string(body)

		// Detect context length errors for retry with truncation
		if resp.StatusCode == http.StatusBadRequest && isContextLengthBody(bodyStr) {
			return nil, &contextLengthError{body: bodyStr}
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrRateLimit,
				Stage:   "mlx-embedding",
				Message: fmt.Sprintf("rate limit exceeded: HTTP %d: %s", resp.StatusCode, bodyStr),
			}
		}
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrModelUnavailable,
				Stage:   "mlx-embedding",
				Message: fmt.Sprintf("service unavailable: HTTP %d: %s", resp.StatusCode, bodyStr),
			}
		}
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrRequestFailed, resp.StatusCode, bodyStr)
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

	// Strip provider prefix for API requests (e.g. "ollama/qwen3:8b" -> "qwen3:8b")
	bareModel := ExtractModelName(model)

	// qwen3 models require the native Ollama API to disable thinking mode.
	// The OpenAI-compatible endpoint ignores think:false.
	if strings.HasPrefix(strings.ToLower(bareModel), "qwen3") {
		return b.ollamaChatCompletion(ctx, messages, bareModel, opts)
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
		Model:    bareModel,
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

	if opts.ResponseSchema != nil {
		reqBody.ResponseFormat = &chatRespFormat{
			Type: "json_schema",
			JSONSchema: &chatJSONSchema{
				Name:   "response",
				Schema: opts.ResponseSchema,
				Strict: true,
			},
		}
	} else if opts.JSONMode {
		reqBody.ResponseFormat = &chatRespFormat{Type: "json_object"}
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

	start := time.Now()
	resp, err := b.httpClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &perrors.PipelineError{
				Code:     perrors.ErrTimeout,
				Stage:    "mlx-llm",
				Message:  fmt.Sprintf("MLX LLM request timed out after %s", elapsed.Round(time.Millisecond)),
				Duration: elapsed,
				Cause:    err,
			}
		}
		if ctx.Err() == context.Canceled {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrContextCancelled,
				Stage:   "mlx-llm",
				Message: "context cancelled",
				Cause:   err,
			}
		}
		// Connection errors indicate service unavailable
		errMsg := err.Error()
		if strings.Contains(strings.ToLower(errMsg), "connection refused") ||
		   strings.Contains(strings.ToLower(errMsg), "no such host") {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrModelUnavailable,
				Stage:   "mlx-llm",
				Message: errMsg,
				Cause:   err,
			}
		}
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrRequestFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrRateLimit,
				Stage:   "mlx-llm",
				Message: fmt.Sprintf("rate limit exceeded: HTTP %d: %s", resp.StatusCode, string(body)),
			}
		}
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrModelUnavailable,
				Stage:   "mlx-llm",
				Message: fmt.Sprintf("service unavailable: HTTP %d: %s", resp.StatusCode, string(body)),
			}
		}
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
		FinishReason: chatResp.Choices[0].FinishReason,
	}, nil
}

// ollamaChatCompletion uses the native Ollama /api/chat endpoint.
// Required for qwen3 models where think:false only works via the native API.
func (b *MLXBackend) ollamaChatCompletion(ctx context.Context, messages []Message, model string, opts CompletionOptions) (*CompletionResult, error) {
	apiURL := b.llmURL + "/api/chat"

	chatMsgs := make([]chatMessage, len(messages))
	for i, m := range messages {
		chatMsgs[i] = chatMessage{Role: m.Role, Content: m.Content}
	}

	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}

	temp := opts.Temperature
	if temp <= 0 {
		temp = 0.1
	}

	thinkFalse := false
	reqBody := ollamaChatRequest{
		Model:    model,
		Messages: chatMsgs,
		Stream:   false,
		Think:    &thinkFalse,
		Options:  ollamaOptions{Temperature: temp, NumPredict: maxTokens},
	}

	if opts.ResponseSchema != nil {
		reqBody.Format = opts.ResponseSchema
	} else if opts.JSONMode {
		reqBody.Format = json.RawMessage(`"json"`)
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", ErrRequestFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", ErrRequestFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := b.httpClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &perrors.PipelineError{
				Code:     perrors.ErrTimeout,
				Stage:    "ollama-llm",
				Message:  fmt.Sprintf("Ollama request timed out after %s", elapsed.Round(time.Millisecond)),
				Duration: elapsed,
				Cause:    err,
			}
		}
		if ctx.Err() == context.Canceled {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrContextCancelled,
				Stage:   "ollama-llm",
				Message: "context cancelled",
				Cause:   err,
			}
		}
		errMsg := err.Error()
		if strings.Contains(strings.ToLower(errMsg), "connection refused") ||
			strings.Contains(strings.ToLower(errMsg), "no such host") {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrModelUnavailable,
				Stage:   "ollama-llm",
				Message: errMsg,
				Cause:   err,
			}
		}
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrRequestFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrRequestFailed, resp.StatusCode, string(body))
	}

	var ollamaResp ollamaChatResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, fmt.Errorf("%w: parse response: %v", ErrInvalidResponse, err)
	}

	return &CompletionResult{
		Content:      ollamaResp.Message.Content,
		Model:        ollamaResp.Model,
		InputTokens:  ollamaResp.PromptEvalCount,
		OutputTokens: ollamaResp.EvalCount,
		FinishReason: "stop",
	}, nil
}

// CheckEmbeddingsHealth checks if the embeddings service is healthy.
func (b *MLXBackend) CheckEmbeddingsHealth(ctx context.Context) error {
	url := b.embeddingsURL + "/"
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
	defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = resp.Body.Close() }()

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
