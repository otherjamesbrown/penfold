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

	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
)

// Default Gemini models.
const (
	DefaultGeminiEmbeddingModel = "text-embedding-004"
	DefaultGeminiLLMModel       = "gemini-2.0-flash"
)

// GeminiBackend connects to Google's Gemini API for embeddings and LLM operations.
type GeminiBackend struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client

	defaultEmbeddingModel string
	defaultLLMModel       string
}

// GeminiConfig holds configuration for the Gemini backend.
type GeminiConfig struct {
	// APIKey is the Gemini API key. If empty, reads from GEMINI_API_KEY environment variable.
	APIKey string

	// Endpoint is the base URL for the Gemini API.
	// Default: https://generativelanguage.googleapis.com/v1beta
	Endpoint string

	// DefaultEmbeddingModel is the default model for embeddings.
	// Default: text-embedding-004
	DefaultEmbeddingModel string

	// DefaultLLMModel is the default model for LLM completions.
	// Default: gemini-2.0-flash
	DefaultLLMModel string

	// Timeout is the HTTP request timeout.
	// Default: 120 seconds
	Timeout time.Duration
}

// DefaultGeminiConfig returns a default Gemini configuration.
func DefaultGeminiConfig() *GeminiConfig {
	return &GeminiConfig{
		Endpoint:              "https://generativelanguage.googleapis.com/v1beta",
		DefaultEmbeddingModel: DefaultGeminiEmbeddingModel,
		DefaultLLMModel:       DefaultGeminiLLMModel,
		Timeout:               120 * time.Second,
	}
}

// NewGeminiBackend creates a new Gemini backend with the given configuration.
// If config is nil, default values are used.
// Returns an error if no API key is available.
func NewGeminiBackend(config *GeminiConfig) (*GeminiBackend, error) {
	if config == nil {
		config = DefaultGeminiConfig()
	}

	// Get API key from config or environment
	apiKey := config.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("%w: GEMINI_API_KEY not set", ErrRequestFailed)
	}

	// Apply defaults for empty values
	if config.Endpoint == "" {
		config.Endpoint = "https://generativelanguage.googleapis.com/v1beta"
	}
	if config.DefaultEmbeddingModel == "" {
		config.DefaultEmbeddingModel = DefaultGeminiEmbeddingModel
	}
	if config.DefaultLLMModel == "" {
		config.DefaultLLMModel = DefaultGeminiLLMModel
	}
	if config.Timeout == 0 {
		config.Timeout = 120 * time.Second
	}

	return &GeminiBackend{
		apiKey:                apiKey,
		endpoint:              strings.TrimSuffix(config.Endpoint, "/"),
		defaultEmbeddingModel: config.DefaultEmbeddingModel,
		defaultLLMModel:       config.DefaultLLMModel,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}, nil
}

// Gemini API request/response types for embeddings.

type geminiEmbedRequest struct {
	Model   string             `json:"model"`
	Content geminiEmbedContent `json:"content"`
}

type geminiEmbedContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiEmbedResponse struct {
	Embedding *geminiEmbedding `json:"embedding,omitempty"`
	Error     *geminiError     `json:"error,omitempty"`
}

type geminiEmbedding struct {
	Values []float32 `json:"values"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// Gemini API request/response types for content generation.

type geminiGenerateRequest struct {
	Contents          []geminiContent           `json:"contents"`
	SystemInstruction *geminiContent            `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig   `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens  int     `json:"maxOutputTokens,omitempty"`
	Temperature      float32 `json:"temperature,omitempty"`
	ResponseMimeType string  `json:"responseMimeType,omitempty"`
}

type geminiGenerateResponse struct {
	Candidates    []geminiCandidate    `json:"candidates,omitempty"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata,omitempty"`
	Error         *geminiError         `json:"error,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
	Index        int           `json:"index"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// GenerateEmbedding creates a vector embedding for the given text.
func (b *GeminiBackend) GenerateEmbedding(ctx context.Context, text string, model string) (*EmbeddingResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrEmptyText
	}

	if model == "" {
		model = b.defaultEmbeddingModel
	}

	// Strip provider prefix before sending to Gemini API (e.g. "gemini/text-embedding-004" -> "text-embedding-004")
	bareModel := extractModelName(model)

	// Gemini embedding endpoint: /models/{model}:embedContent
	url := fmt.Sprintf("%s/models/%s:embedContent?key=%s", b.endpoint, bareModel, b.apiKey)

	reqBody := geminiEmbedRequest{
		Model: fmt.Sprintf("models/%s", bareModel),
		Content: geminiEmbedContent{
			Parts: []geminiPart{{Text: text}},
		},
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
				Stage:    "gemini-embedding",
				Message:  fmt.Sprintf("Gemini embedding request timed out after %s", elapsed.Round(time.Millisecond)),
				Duration: elapsed,
				Cause:    err,
			}
		}
		if ctx.Err() == context.Canceled {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrContextCancelled,
				Stage:   "gemini-embedding",
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
				Stage:   "gemini-embedding",
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

	// Handle HTTP errors
	if resp.StatusCode != http.StatusOK {
		return nil, b.handleHTTPError(resp.StatusCode, body)
	}

	var embedResp geminiEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("%w: parse response: %v", ErrInvalidResponse, err)
	}

	if embedResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrRequestFailed, embedResp.Error.Message)
	}

	if embedResp.Embedding == nil || len(embedResp.Embedding.Values) == 0 {
		return nil, fmt.Errorf("%w: no embedding data returned", ErrInvalidResponse)
	}

	embedding := embedResp.Embedding.Values

	return &EmbeddingResult{
		Vector:     embedding,
		Dimensions: len(embedding),
		Model:      model,
		TokenCount: 0, // Gemini embedContent doesn't return token count
	}, nil
}

// ChatCompletion sends a chat completion request to the LLM.
func (b *GeminiBackend) ChatCompletion(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error) {
	if len(messages) == 0 {
		return nil, ErrEmptyMessages
	}

	model := opts.Model
	if model == "" {
		model = b.defaultLLMModel
	}

	// Strip provider prefix before sending to Gemini API (e.g. "gemini/gemini-2.5-flash" -> "gemini-2.5-flash")
	bareModel := extractModelName(model)

	// Gemini content generation endpoint: /models/{model}:generateContent
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", b.endpoint, bareModel, b.apiKey)

	// Convert messages to Gemini format
	// Gemini uses "user" and "model" roles (not "assistant")
	// System prompts go in systemInstruction field
	var contents []geminiContent
	var systemInstruction *geminiContent

	for _, m := range messages {
		if m.Role == "system" {
			// Gemini uses systemInstruction for system prompts
			systemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: m.Content}},
			}
		} else {
			role := m.Role
			if role == "assistant" {
				role = "model" // Gemini uses "model" instead of "assistant"
			}
			contents = append(contents, geminiContent{
				Role:  role,
				Parts: []geminiPart{{Text: m.Content}},
			})
		}
	}

	if len(contents) == 0 {
		return nil, fmt.Errorf("%w: no user or model messages provided", ErrEmptyMessages)
	}

	reqBody := geminiGenerateRequest{
		Contents:          contents,
		SystemInstruction: systemInstruction,
	}

	// Configure generation settings
	genConfig := &geminiGenerationConfig{}
	hasConfig := false

	if opts.MaxTokens > 0 {
		genConfig.MaxOutputTokens = opts.MaxTokens
		hasConfig = true
	} else {
		genConfig.MaxOutputTokens = 8192 // Default — Gemini 2.0 Flash supports 8192
		hasConfig = true
	}

	if opts.Temperature > 0 {
		genConfig.Temperature = opts.Temperature
		hasConfig = true
	} else {
		genConfig.Temperature = 0.1 // Low temperature for structured extraction
		hasConfig = true
	}

	if opts.JSONMode {
		genConfig.ResponseMimeType = "application/json"
		hasConfig = true
	}

	if hasConfig {
		reqBody.GenerationConfig = genConfig
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
				Stage:    "gemini-llm",
				Message:  fmt.Sprintf("Gemini LLM request timed out after %s", elapsed.Round(time.Millisecond)),
				Duration: elapsed,
				Cause:    err,
			}
		}
		if ctx.Err() == context.Canceled {
			return nil, &perrors.PipelineError{
				Code:    perrors.ErrContextCancelled,
				Stage:   "gemini-llm",
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
				Stage:   "gemini-llm",
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

	// Handle HTTP errors
	if resp.StatusCode != http.StatusOK {
		return nil, b.handleHTTPError(resp.StatusCode, body)
	}

	var genResp geminiGenerateResponse
	if err := json.Unmarshal(body, &genResp); err != nil {
		return nil, fmt.Errorf("%w: parse response: %v", ErrInvalidResponse, err)
	}

	if genResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrRequestFailed, genResp.Error.Message)
	}

	if len(genResp.Candidates) == 0 {
		return nil, ErrNoChoices
	}

	// Extract content from first candidate
	candidate := genResp.Candidates[0]
	var content string
	if len(candidate.Content.Parts) > 0 {
		content = candidate.Content.Parts[0].Text
	}

	// Extract token counts
	var inputTokens, outputTokens int
	if genResp.UsageMetadata != nil {
		inputTokens = genResp.UsageMetadata.PromptTokenCount
		outputTokens = genResp.UsageMetadata.CandidatesTokenCount
	}

	// Normalize Gemini's finish reason to OpenAI-compatible format
	finishReason := candidate.FinishReason
	if finishReason == "MAX_TOKENS" {
		finishReason = "length"
	} else if finishReason == "STOP" {
		finishReason = "stop"
	}

	return &CompletionResult{
		Content:      content,
		Model:        model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		FinishReason: finishReason,
	}, nil
}

// CheckEmbeddingsHealth checks if the embeddings service is healthy.
// For Gemini, we perform a simple embedding request with test text.
func (b *GeminiBackend) CheckEmbeddingsHealth(ctx context.Context) error {
	_, err := b.GenerateEmbedding(ctx, "health check", "")
	if err != nil {
		return fmt.Errorf("%w: embeddings health check failed: %v", ErrServiceUnavailable, err)
	}
	return nil
}

// CheckLLMHealth checks if the LLM service is healthy.
// For Gemini, we perform a simple completion request.
func (b *GeminiBackend) CheckLLMHealth(ctx context.Context) error {
	messages := []Message{
		{Role: "user", Content: "Say 'ok'."},
	}
	opts := CompletionOptions{
		MaxTokens:   10,
		Temperature: 0,
	}
	_, err := b.ChatCompletion(ctx, messages, opts)
	if err != nil {
		return fmt.Errorf("%w: LLM health check failed: %v", ErrServiceUnavailable, err)
	}
	return nil
}

// Close releases any resources held by the backend.
func (b *GeminiBackend) Close() error {
	b.httpClient.CloseIdleConnections()
	return nil
}

// DefaultEmbeddingModel returns the default embedding model name.
func (b *GeminiBackend) DefaultEmbeddingModel() string {
	return b.defaultEmbeddingModel
}

// DefaultLLMModel returns the default LLM model name.
func (b *GeminiBackend) DefaultLLMModel() string {
	return b.defaultLLMModel
}

// handleHTTPError converts HTTP error responses to appropriate backend errors.
func (b *GeminiBackend) handleHTTPError(statusCode int, body []byte) error {
	// Try to parse error response
	var errResp struct {
		Error *geminiError `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		switch statusCode {
		case http.StatusTooManyRequests:
			// Rate limit errors return PipelineError with ErrRateLimit
			return &perrors.PipelineError{
				Code:    perrors.ErrRateLimit,
				Stage:   "gemini",
				Message: fmt.Sprintf("rate limit exceeded: %s", errResp.Error.Message),
			}
		case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
			// Service unavailable errors
			return &perrors.PipelineError{
				Code:    perrors.ErrModelUnavailable,
				Stage:   "gemini",
				Message: fmt.Sprintf("service unavailable: %s", errResp.Error.Message),
			}
		case http.StatusUnauthorized:
			return fmt.Errorf("%w: authentication failed: %s", ErrRequestFailed, errResp.Error.Message)
		case http.StatusForbidden:
			return fmt.Errorf("%w: access denied: %s", ErrRequestFailed, errResp.Error.Message)
		case http.StatusNotFound:
			return fmt.Errorf("%w: model not found: %s", ErrRequestFailed, errResp.Error.Message)
		default:
			// Check for RESOURCE_EXHAUSTED in error status (Gemini-specific rate limit)
			if strings.Contains(strings.ToUpper(errResp.Error.Status), "RESOURCE_EXHAUSTED") {
				return &perrors.PipelineError{
					Code:    perrors.ErrRateLimit,
					Stage:   "gemini",
					Message: fmt.Sprintf("resource exhausted: %s", errResp.Error.Message),
				}
			}
			return fmt.Errorf("%w: HTTP %d: %s", ErrRequestFailed, statusCode, errResp.Error.Message)
		}
	}

	// Fall back to raw body if parsing fails
	if statusCode == http.StatusTooManyRequests {
		return &perrors.PipelineError{
			Code:    perrors.ErrRateLimit,
			Stage:   "gemini",
			Message: fmt.Sprintf("HTTP 429: %s", string(body)),
		}
	}
	return fmt.Errorf("%w: HTTP %d: %s", ErrRequestFailed, statusCode, string(body))
}
