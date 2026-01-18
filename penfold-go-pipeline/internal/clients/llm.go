// Package clients provides HTTP clients for AI services.
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
	"github.com/sony/gobreaker"
)

// LLMConfig holds configuration for the LLM client.
type LLMConfig struct {
	// BaseURL is the vLLM-MLX server URL (default: http://localhost:8000)
	BaseURL string
	// Model is the LLM model to use
	Model string
	// DefaultTemperature for generation
	DefaultTemperature float64
	// DefaultMaxTokens for generation
	DefaultMaxTokens int
	// Timeout for individual requests
	Timeout time.Duration
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int
	// CircuitBreakerSettings configures the circuit breaker
	CircuitBreakerSettings gobreaker.Settings
}

// DefaultLLMConfig returns sensible defaults for the LLM client.
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		BaseURL:            "http://localhost:8000",
		Model:              "Qwen2.5-32B-4bit",
		DefaultTemperature: 0.7,
		DefaultMaxTokens:   1024,
		Timeout:            120 * time.Second,
		MaxRetries:         3,
		CircuitBreakerSettings: gobreaker.Settings{
			Name:        "llm-client",
			MaxRequests: 3,
			Interval:    10 * time.Second,
			Timeout:     60 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
				return counts.Requests >= 3 && failureRatio >= 0.6
			},
		},
	}
}

// LLMClient is an HTTP client for the vLLM-MLX server (OpenAI-compatible API).
type LLMClient struct {
	baseURL            string
	model              string
	defaultTemperature float64
	defaultMaxTokens   int
	httpClient         *retryablehttp.Client
	circuitBreaker     *gobreaker.CircuitBreaker
	logger             zerolog.Logger
}

// ChatMessage represents a message in the chat conversation.
type ChatMessage struct {
	Role    string `json:"role"`    // "system", "user", or "assistant"
	Content string `json:"content"` // The message content
}

// ChatCompletionRequest represents the request payload for chat completions.
type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	TopP        float64       `json:"top_p,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
}

// ChatCompletionChoice represents a single completion choice.
type ChatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// ChatCompletionUsage represents token usage information.
type ChatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionResponse represents the response from the chat completions API.
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   ChatCompletionUsage    `json:"usage"`
}

// LLMError represents an error from the LLM API.
type LLMError struct {
	StatusCode int
	Message    string
}

func (e *LLMError) Error() string {
	return fmt.Sprintf("LLM API error (status %d): %s", e.StatusCode, e.Message)
}

// Assertion represents an extracted assertion from content.
type Assertion struct {
	Statement  string  `json:"statement"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source,omitempty"`
	Category   string  `json:"category,omitempty"`
}

// NewLLMClient creates a new LLM client with the given configuration.
func NewLLMClient(cfg LLMConfig, logger zerolog.Logger) *LLMClient {
	// Create retryable HTTP client
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = cfg.MaxRetries
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 30 * time.Second
	retryClient.Logger = nil // Disable default logging, we use zerolog

	// Set timeout on the underlying HTTP client
	retryClient.HTTPClient.Timeout = cfg.Timeout

	// Create circuit breaker
	cb := gobreaker.NewCircuitBreaker(cfg.CircuitBreakerSettings)

	return &LLMClient{
		baseURL:            cfg.BaseURL,
		model:              cfg.Model,
		defaultTemperature: cfg.DefaultTemperature,
		defaultMaxTokens:   cfg.DefaultMaxTokens,
		httpClient:         retryClient,
		circuitBreaker:     cb,
		logger:             logger.With().Str("component", "llm-client").Logger(),
	}
}

// ChatCompletion sends a chat completion request and returns the response.
func (c *LLMClient) ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Apply defaults if not set
	if req.Model == "" {
		req.Model = c.model
	}
	if req.Temperature == 0 {
		req.Temperature = c.defaultTemperature
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = c.defaultMaxTokens
	}

	c.logger.Debug().
		Str("model", req.Model).
		Int("message_count", len(req.Messages)).
		Float64("temperature", req.Temperature).
		Int("max_tokens", req.MaxTokens).
		Msg("sending chat completion request")

	// Execute through circuit breaker
	result, err := c.circuitBreaker.Execute(func() (interface{}, error) {
		return c.doChatCompletionRequest(ctx, req)
	})

	if err != nil {
		c.logger.Error().Err(err).Msg("chat completion request failed")
		return nil, err
	}

	resp := result.(*ChatCompletionResponse)
	c.logger.Debug().
		Str("model", resp.Model).
		Int("choices", len(resp.Choices)).
		Int("total_tokens", resp.Usage.TotalTokens).
		Msg("chat completion successful")

	return resp, nil
}

// doChatCompletionRequest performs the actual HTTP request for chat completion.
func (c *LLMClient) doChatCompletionRequest(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/chat/completions", c.baseURL)
	httpReq, err := retryablehttp.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &LLMError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
		}
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &chatResp, nil
}

// GenerateSummary generates a summary of the given content.
func (c *LLMClient) GenerateSummary(ctx context.Context, content string) (string, error) {
	if content == "" {
		return "", fmt.Errorf("content cannot be empty")
	}

	messages := []ChatMessage{
		{
			Role: "system",
			Content: `You are a helpful assistant that generates concise summaries.
Your summaries should:
- Capture the key points and main ideas
- Be clear and well-structured
- Be appropriate in length for the content (typically 2-4 sentences for short content, more for longer content)
- Preserve important details, names, dates, and action items`,
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Please summarize the following content:\n\n%s", content),
		},
	}

	req := ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.3, // Lower temperature for more consistent summaries
		MaxTokens:   512,
	}

	resp, err := c.ChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no completion choices returned")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// ExtractAssertions extracts factual assertions from the given content.
func (c *LLMClient) ExtractAssertions(ctx context.Context, content string) ([]Assertion, error) {
	if content == "" {
		return nil, fmt.Errorf("content cannot be empty")
	}

	messages := []ChatMessage{
		{
			Role: "system",
			Content: `You are an assistant that extracts factual assertions from text.
For each assertion, provide:
- statement: The factual claim being made
- confidence: A score from 0.0 to 1.0 indicating how explicitly stated the assertion is
- category: One of "fact", "opinion", "commitment", "question", or "action_item"

Return the assertions as a JSON array. Example:
[
  {"statement": "The meeting is scheduled for Tuesday at 2pm", "confidence": 1.0, "category": "fact"},
  {"statement": "John will send the report by Friday", "confidence": 0.95, "category": "commitment"}
]

Only return the JSON array, no additional text.`,
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Extract all factual assertions from the following content:\n\n%s", content),
		},
	}

	req := ChatCompletionRequest{
		Messages:    messages,
		Temperature: 0.1, // Very low temperature for consistent extraction
		MaxTokens:   2048,
	}

	resp, err := c.ChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to extract assertions: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no completion choices returned")
	}

	responseContent := strings.TrimSpace(resp.Choices[0].Message.Content)

	// Try to parse as JSON array
	var assertions []Assertion
	if err := json.Unmarshal([]byte(responseContent), &assertions); err != nil {
		// Try to extract JSON from the response if it contains extra text
		startIdx := strings.Index(responseContent, "[")
		endIdx := strings.LastIndex(responseContent, "]")
		if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
			jsonStr := responseContent[startIdx : endIdx+1]
			if err := json.Unmarshal([]byte(jsonStr), &assertions); err != nil {
				return nil, fmt.Errorf("failed to parse assertions JSON: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to parse assertions JSON: %w", err)
		}
	}

	c.logger.Debug().
		Int("assertion_count", len(assertions)).
		Msg("extracted assertions from content")

	return assertions, nil
}

// HealthCheck verifies the LLM service is healthy.
func (c *LLMClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/health", c.baseURL)
	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &LLMError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("health check returned non-OK status: %s", string(body)),
		}
	}

	c.logger.Debug().Msg("LLM service health check passed")
	return nil
}

// CircuitBreakerState returns the current state of the circuit breaker.
func (c *LLMClient) CircuitBreakerState() gobreaker.State {
	return c.circuitBreaker.State()
}

// Close performs any cleanup needed by the client.
func (c *LLMClient) Close() error {
	// Currently no cleanup needed, but included for interface consistency
	c.logger.Debug().Msg("LLM client closed")
	return nil
}
