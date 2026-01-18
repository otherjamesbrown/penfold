// Package clients provides HTTP clients for AI services.
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
	"github.com/sony/gobreaker"
)

// EmbeddingsConfig holds configuration for the embeddings client.
type EmbeddingsConfig struct {
	// BaseURL is the MLX embeddings sidecar URL (default: http://localhost:8001)
	BaseURL string
	// Model is the embedding model to use
	Model string
	// Timeout for individual requests
	Timeout time.Duration
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int
	// CircuitBreakerSettings configures the circuit breaker
	CircuitBreakerSettings gobreaker.Settings
}

// DefaultEmbeddingsConfig returns sensible defaults for the embeddings client.
func DefaultEmbeddingsConfig() EmbeddingsConfig {
	return EmbeddingsConfig{
		BaseURL:    "http://localhost:8001",
		Model:      "text-embedding-ada-002",
		Timeout:    30 * time.Second,
		MaxRetries: 3,
		CircuitBreakerSettings: gobreaker.Settings{
			Name:        "embeddings-client",
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

// EmbeddingsClient is an HTTP client for the MLX embeddings sidecar.
type EmbeddingsClient struct {
	baseURL        string
	model          string
	httpClient     *retryablehttp.Client
	circuitBreaker *gobreaker.CircuitBreaker
	logger         zerolog.Logger
}

// EmbeddingRequest represents the request payload for embeddings.
type EmbeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

// EmbeddingData represents a single embedding result.
type EmbeddingData struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object"`
}

// EmbeddingResponse represents the response from the embeddings API.
type EmbeddingResponse struct {
	Object string          `json:"object"`
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// EmbeddingsError represents an error from the embeddings API.
type EmbeddingsError struct {
	StatusCode int
	Message    string
}

func (e *EmbeddingsError) Error() string {
	return fmt.Sprintf("embeddings API error (status %d): %s", e.StatusCode, e.Message)
}

// NewEmbeddingsClient creates a new embeddings client with the given configuration.
func NewEmbeddingsClient(cfg EmbeddingsConfig, logger zerolog.Logger) *EmbeddingsClient {
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

	return &EmbeddingsClient{
		baseURL:        cfg.BaseURL,
		model:          cfg.Model,
		httpClient:     retryClient,
		circuitBreaker: cb,
		logger:         logger.With().Str("component", "embeddings-client").Logger(),
	}
}

// Embed generates embeddings for a single text input.
func (c *EmbeddingsClient) Embed(ctx context.Context, text string) ([]float64, error) {
	embeddings, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple text inputs.
func (c *EmbeddingsClient) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	c.logger.Debug().
		Int("batch_size", len(texts)).
		Msg("generating embeddings batch")

	// Execute through circuit breaker
	result, err := c.circuitBreaker.Execute(func() (interface{}, error) {
		return c.doEmbedRequest(ctx, texts)
	})

	if err != nil {
		c.logger.Error().Err(err).Msg("embeddings request failed")
		return nil, err
	}

	embeddings := result.([][]float64)
	c.logger.Debug().
		Int("batch_size", len(texts)).
		Int("embeddings_count", len(embeddings)).
		Msg("embeddings generated successfully")

	return embeddings, nil
}

// doEmbedRequest performs the actual HTTP request to generate embeddings.
func (c *EmbeddingsClient) doEmbedRequest(ctx context.Context, texts []string) ([][]float64, error) {
	reqBody := EmbeddingRequest{
		Input: texts,
		Model: c.model,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/embeddings", c.baseURL)
	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &EmbeddingsError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
		}
	}

	var embResp EmbeddingResponse
	if err := json.Unmarshal(body, &embResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Extract embeddings in order
	embeddings := make([][]float64, len(embResp.Data))
	for _, data := range embResp.Data {
		if data.Index >= len(embeddings) {
			return nil, fmt.Errorf("invalid embedding index %d for batch size %d", data.Index, len(texts))
		}
		embeddings[data.Index] = data.Embedding
	}

	return embeddings, nil
}

// HealthCheck verifies the embeddings service is healthy.
func (c *EmbeddingsClient) HealthCheck(ctx context.Context) error {
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
		return &EmbeddingsError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("health check returned non-OK status: %s", string(body)),
		}
	}

	c.logger.Debug().Msg("embeddings service health check passed")
	return nil
}

// CircuitBreakerState returns the current state of the circuit breaker.
func (c *EmbeddingsClient) CircuitBreakerState() gobreaker.State {
	return c.circuitBreaker.State()
}

// Close performs any cleanup needed by the client.
func (c *EmbeddingsClient) Close() error {
	// Currently no cleanup needed, but included for interface consistency
	c.logger.Debug().Msg("embeddings client closed")
	return nil
}
