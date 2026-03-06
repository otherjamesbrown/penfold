// Package backend provides backend connectors for AI services.
package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenAI-specific errors.
var (
	// ErrAuthenticationFailed indicates invalid API key or authentication failure.
	ErrAuthenticationFailed = fmt.Errorf("%w: authentication failed", ErrRequestFailed)

	// ErrRateLimited indicates the API rate limit was exceeded.
	ErrRateLimited = fmt.Errorf("%w: rate limit exceeded", ErrRequestFailed)

	// ErrInsufficientQuota indicates the API quota was exceeded.
	ErrInsufficientQuota = fmt.Errorf("%w: insufficient quota", ErrRequestFailed)

	// ErrModelNotFound indicates the requested model was not found.
	ErrModelNotFound = fmt.Errorf("%w: model not found", ErrRequestFailed)
)

// OpenAIBackend connects to OpenAI-compatible APIs for embeddings and LLM operations.
// It supports OpenAI, Azure OpenAI, and compatible servers (LiteLLM, vllm-mlx, etc.).
type OpenAIBackend struct {
	endpoint   string
	apiKey     string
	orgID      string
	httpClient *http.Client

	defaultEmbeddingModel string
	defaultLLMModel       string

	// Azure-specific configuration
	azureDeployment string
	azureAPIVersion string
}

// OpenAIConfig holds configuration for the OpenAI backend.
type OpenAIConfig struct {
	// APIKey is the OpenAI API key.
	// If empty, uses OPENAI_API_KEY environment variable.
	APIKey string

	// Endpoint is the base URL for the API.
	// Default: https://api.openai.com/v1
	// For Azure: https://<resource>.openai.azure.com/openai/deployments/<deployment>
	// For compatible servers: http://localhost:8000/v1
	Endpoint string

	// Organization is the OpenAI organization ID (optional).
	Organization string

	// DefaultEmbeddingModel is the default model for embeddings.
	// Default: text-embedding-3-small
	DefaultEmbeddingModel string

	// DefaultLLMModel is the default model for LLM completions.
	// Default: gpt-4o-mini
	DefaultLLMModel string

	// Timeout is the HTTP request timeout.
	// Default: 120 seconds
	Timeout time.Duration

	// AzureDeployment is the Azure OpenAI deployment name (for Azure only).
	AzureDeployment string

	// AzureAPIVersion is the Azure OpenAI API version (for Azure only).
	// Default: 2024-02-01
	AzureAPIVersion string
}

// DefaultOpenAIConfig returns a default OpenAI configuration.
func DefaultOpenAIConfig() *OpenAIConfig {
	return &OpenAIConfig{
		Endpoint:              "https://api.openai.com/v1",
		DefaultEmbeddingModel: "text-embedding-3-small",
		DefaultLLMModel:       "gpt-4o-mini",
		Timeout:               120 * time.Second,
		AzureAPIVersion:       "2024-02-01",
	}
}

// NewOpenAIBackend creates a new OpenAI backend with the given configuration.
// If config is nil, default values are used.
func NewOpenAIBackend(config *OpenAIConfig) (*OpenAIBackend, error) {
	if config == nil {
		config = DefaultOpenAIConfig()
	}

	// Get API key from config or environment
	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	// Note: API key can be empty for local compatible servers

	// Apply defaults for empty values
	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	endpoint = strings.TrimSuffix(endpoint, "/")

	defaultEmbeddingModel := config.DefaultEmbeddingModel
	if defaultEmbeddingModel == "" {
		defaultEmbeddingModel = "text-embedding-3-small"
	}

	defaultLLMModel := config.DefaultLLMModel
	if defaultLLMModel == "" {
		defaultLLMModel = "gpt-4o-mini"
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	azureAPIVersion := config.AzureAPIVersion
	if azureAPIVersion == "" {
		azureAPIVersion = "2024-02-01"
	}

	return &OpenAIBackend{
		endpoint:              endpoint,
		apiKey:                apiKey,
		orgID:                 config.Organization,
		defaultEmbeddingModel: defaultEmbeddingModel,
		defaultLLMModel:       defaultLLMModel,
		azureDeployment:       config.AzureDeployment,
		azureAPIVersion:       azureAPIVersion,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

// OpenAI-compatible request/response types for chat completions with JSON mode.

type openaiChatRequest struct {
	Model          string             `json:"model"`
	Messages       []openaiMessage    `json:"messages"`
	MaxTokens      int                `json:"max_tokens,omitempty"`
	Temperature    float32            `json:"temperature,omitempty"`
	ResponseFormat *openaiRespFormat  `json:"response_format,omitempty"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiRespFormat struct {
	Type       string            `json:"type"`
	JSONSchema *openaiJSONSchema `json:"json_schema,omitempty"`
}

type openaiJSONSchema struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

type openaiChatResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []openaiChatChoice `json:"choices"`
	Usage   openaiChatUsage    `json:"usage"`
	Error   *openaiError       `json:"error,omitempty"`
}

type openaiChatChoice struct {
	Index        int           `json:"index"`
	Message      openaiMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openaiChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openaiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// GenerateEmbedding creates a vector embedding for the given text using OpenAI API.
func (b *OpenAIBackend) GenerateEmbedding(ctx context.Context, text string, model string) (*EmbeddingResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrEmptyText
	}

	if model == "" {
		model = b.defaultEmbeddingModel
	}

	url := b.buildURL("/embeddings")

	// Strip provider prefix before sending to OpenAI API (e.g. "openai/text-embedding-3-small" -> "text-embedding-3-small")
	reqBody := openaiEmbedRequest{
		Model: extractModelName(model),
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
	b.setHeaders(req)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, b.handleHTTPError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrRequestFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, b.parseAPIError(resp.StatusCode, body)
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

// ChatCompletion sends a chat completion request to the OpenAI API.
func (b *OpenAIBackend) ChatCompletion(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error) {
	if len(messages) == 0 {
		return nil, ErrEmptyMessages
	}

	model := opts.Model
	if model == "" {
		model = b.defaultLLMModel
	}

	url := b.buildURL("/chat/completions")

	// Convert messages
	openaiMsgs := make([]openaiMessage, len(messages))
	for i, m := range messages {
		openaiMsgs[i] = openaiMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	// Strip provider prefix before sending to OpenAI API (e.g. "openai/gpt-4o-mini" -> "gpt-4o-mini")
	reqBody := openaiChatRequest{
		Model:    extractModelName(model),
		Messages: openaiMsgs,
	}

	if opts.MaxTokens > 0 {
		reqBody.MaxTokens = opts.MaxTokens
	} else {
		reqBody.MaxTokens = 4096 // Default for OpenAI models
	}

	if opts.Temperature > 0 {
		reqBody.Temperature = opts.Temperature
	} else {
		reqBody.Temperature = 0.1 // Low temperature for structured extraction
	}

	// Enable JSON mode if requested; prefer schema-constrained output when available
	if opts.ResponseSchema != nil {
		reqBody.ResponseFormat = &openaiRespFormat{
			Type: "json_schema",
			JSONSchema: &openaiJSONSchema{
				Name:   "response",
				Schema: opts.ResponseSchema,
				Strict: true,
			},
		}
	} else if opts.JSONMode {
		reqBody.ResponseFormat = &openaiRespFormat{
			Type: "json_object",
		}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", ErrRequestFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", ErrRequestFailed, err)
	}
	b.setHeaders(req)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, b.handleHTTPError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrRequestFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, b.parseAPIError(resp.StatusCode, body)
	}

	var chatResp openaiChatResponse
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

// CheckEmbeddingsHealth checks if the embeddings service is healthy.
// For OpenAI, this checks if the API is reachable by listing models.
func (b *OpenAIBackend) CheckEmbeddingsHealth(ctx context.Context) error {
	return b.checkHealth(ctx)
}

// CheckLLMHealth checks if the LLM service is healthy.
// For OpenAI, this checks if the API is reachable by listing models.
func (b *OpenAIBackend) CheckLLMHealth(ctx context.Context) error {
	return b.checkHealth(ctx)
}

// checkHealth performs a health check by calling the models endpoint.
func (b *OpenAIBackend) checkHealth(ctx context.Context) error {
	url := b.buildURL("/models")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%w: create request: %v", ErrRequestFailed, err)
	}
	b.setHeaders(req)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return b.handleHTTPError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrAuthenticationFailed
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return b.parseAPIError(resp.StatusCode, body)
	}

	return nil
}

// Close releases any resources held by the backend.
func (b *OpenAIBackend) Close() error {
	b.httpClient.CloseIdleConnections()
	return nil
}

// DefaultEmbeddingModel returns the default embedding model name.
func (b *OpenAIBackend) DefaultEmbeddingModel() string {
	return b.defaultEmbeddingModel
}

// DefaultLLMModel returns the default LLM model name.
func (b *OpenAIBackend) DefaultLLMModel() string {
	return b.defaultLLMModel
}

// buildURL constructs the full URL for an API endpoint.
// Handles both standard OpenAI and Azure OpenAI URL formats.
func (b *OpenAIBackend) buildURL(path string) string {
	// For Azure OpenAI, the URL format is different
	if b.azureDeployment != "" {
		// Azure format: {endpoint}/openai/deployments/{deployment}/{operation}?api-version={version}
		return fmt.Sprintf("%s%s?api-version=%s", b.endpoint, path, b.azureAPIVersion)
	}
	return b.endpoint + path
}

// setHeaders adds required headers to the request.
func (b *OpenAIBackend) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")

	if b.apiKey != "" {
		if b.azureDeployment != "" {
			// Azure uses api-key header
			req.Header.Set("api-key", b.apiKey)
		} else {
			// Standard OpenAI uses Bearer token
			req.Header.Set("Authorization", "Bearer "+b.apiKey)
		}
	}

	if b.orgID != "" {
		req.Header.Set("OpenAI-Organization", b.orgID)
	}
}

// handleHTTPError converts HTTP client errors to appropriate backend errors.
func (b *OpenAIBackend) handleHTTPError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %v", ErrContextCanceled, ctx.Err())
	}
	return fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
}

// parseAPIError parses an API error response and returns an appropriate error.
func (b *OpenAIBackend) parseAPIError(statusCode int, body []byte) error {
	var errResp struct {
		Error *openaiError `json:"error"`
	}

	// Try to parse the error response
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		switch statusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("%w: %s", ErrAuthenticationFailed, errResp.Error.Message)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%w: %s", ErrRateLimited, errResp.Error.Message)
		case http.StatusNotFound:
			if errResp.Error.Code == "model_not_found" {
				return fmt.Errorf("%w: %s", ErrModelNotFound, errResp.Error.Message)
			}
		}

		// Check for quota exceeded error
		if errResp.Error.Code == "insufficient_quota" {
			return fmt.Errorf("%w: %s", ErrInsufficientQuota, errResp.Error.Message)
		}

		return fmt.Errorf("%w: HTTP %d: %s", ErrRequestFailed, statusCode, errResp.Error.Message)
	}

	// Fallback if we can't parse the error
	return fmt.Errorf("%w: HTTP %d: %s", ErrRequestFailed, statusCode, string(body))
}

// Ensure OpenAIBackend implements Backend interface.
var _ Backend = (*OpenAIBackend)(nil)
