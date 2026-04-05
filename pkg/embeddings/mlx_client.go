// Package embeddings provides embedding generation for semantic search.
package embeddings

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

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
)

// MLXClient connects to an MLX server via an Ollama-compatible HTTP API.
// It supports the mxbai-embed-large-v1 model running locally on Apple Silicon.
//
// Deprecated: Use NewAIBackedClient with an AIClient instead for new code.
// This client is retained for backward compatibility during migration.
type MLXClient struct {
	config     *Config
	httpClient *http.Client
	logger     logging.Logger
	cache      Cache
}

// NewMLXClient creates a new MLX embedding client with the given configuration.
// If config is nil, DefaultConfig() is used.
// The cache parameter is optional; if nil, no caching is performed.
//
// Deprecated: Use NewAIBackedClient with an AIClient instead for new code.
func NewMLXClient(config *Config, logger logging.Logger, cache Cache) (*MLXClient, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// MLXClient requires ServerURL
	if config.ServerURL == "" {
		return nil, ErrInvalidServerURL
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	client := &MLXClient{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger: logger.With(logging.F("component", "mlx_embedding_client")),
		cache:  cache,
	}

	return client, nil
}

// ollamaEmbedRequest is the request format for Ollama's embedding endpoint.
type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// ollamaEmbedResponse is the response format from Ollama's embedding endpoint.
type ollamaEmbedResponse struct {
	Embedding  []float64   `json:"embedding,omitempty"`
	Embeddings [][]float64 `json:"embeddings,omitempty"` // For batch requests
	Model      string      `json:"model,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// openaiEmbedRequest is the OpenAI-compatible request format.
type openaiEmbedRequest struct {
	Model string      `json:"model"`
	Input interface{} `json:"input"` // Can be string or []string
}

// openaiEmbedResponse is the OpenAI-compatible response format.
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

// Embed generates a vector embedding for the given text.
func (c *MLXClient) Embed(ctx context.Context, text string) ([]float32, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrEmptyText
	}

	// Start embedding trace
	start := time.Now()
	ctx, span := tracing.StartEmbedding(ctx, "embeddings.generate", tracing.EmbeddingOptions{
		Model:  c.config.Model,
		System: tracing.AISystemMLX,
	})
	defer span.End()

	// Check cache first
	if c.cache != nil {
		if embedding, err := c.cache.Get(ctx, text); err == nil {
			c.logger.Debug("Cache hit for embedding",
				logging.F("text_length", len(text)),
			)
			tracing.SetEmbeddingResult(span, tracing.EmbeddingResult{
				Dimensions: len(embedding),
				LatencyMs:  time.Since(start).Milliseconds(),
				Cached:     true,
			})
			return embedding, nil
		}
	}

	// Generate embedding
	embedding, err := c.embedSingle(ctx, text)
	if err != nil {
		tracing.SetEmbeddingResult(span, tracing.EmbeddingResult{
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     err,
		})
		return nil, err
	}

	// Store in cache
	if c.cache != nil {
		if cacheErr := c.cache.Set(ctx, text, embedding); cacheErr != nil {
			c.logger.Warn("Failed to cache embedding",
				logging.F("text_length", len(text)),
				logging.Err(cacheErr),
			)
		}
	}

	tracing.SetEmbeddingResult(span, tracing.EmbeddingResult{
		Dimensions: len(embedding),
		LatencyMs:  time.Since(start).Milliseconds(),
		Cached:     false,
	})

	return embedding, nil
}

// embedSingle makes a single embedding request to the server.
func (c *MLXClient) embedSingle(ctx context.Context, text string) ([]float32, error) {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := c.config.RetryDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ErrContextCanceled
			case <-time.After(delay):
			}
		}

		embedding, err := c.doEmbedRequest(ctx, text)
		if err == nil {
			return embedding, nil
		}

		lastErr = err

		// Don't retry on certain errors
		if errors.Is(err, ErrContextCanceled) || errors.Is(err, ErrEmptyText) {
			return nil, err
		}

		c.logger.Warn("Embedding request failed, retrying",
			logging.F("attempt", attempt+1),
			logging.F("max_retries", c.config.MaxRetries),
			logging.Err(err),
		)
	}

	return nil, fmt.Errorf("embedding failed after %d attempts: %w", c.config.MaxRetries+1, lastErr)
}

// doEmbedRequest performs a single embedding request.
func (c *MLXClient) doEmbedRequest(ctx context.Context, text string) ([]float32, error) {
	// Try OpenAI-compatible endpoint first, fall back to Ollama native
	embedding, err := c.doOpenAIEmbedRequest(ctx, text)
	if err == nil {
		return embedding, nil
	}

	// Fall back to Ollama native endpoint
	return c.doOllamaEmbedRequest(ctx, text)
}

// doOpenAIEmbedRequest uses the OpenAI-compatible /v1/embeddings endpoint.
func (c *MLXClient) doOpenAIEmbedRequest(ctx context.Context, text string) ([]float32, error) {
	url := strings.TrimSuffix(c.config.ServerURL, "/") + "/v1/embeddings"

	// Trace the HTTP request
	ctx, span := tracing.StartSpanWithTracer(ctx, "penfold.embeddings", "embeddings.http.openai")
	defer span.End()
	tracing.SetAttributes(span,
		tracing.Attr("http.url", url),
		tracing.Attr("http.method", "POST"),
		tracing.Attr("embedding.api", "openai"),
	)

	reqBody := openaiEmbedRequest{
		Model: c.config.Model,
		Input: text,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		tracing.SetError(span, err)
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		tracing.SetError(span, err)
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		tracing.SetError(span, err)
		if ctx.Err() != nil {
			return nil, ErrContextCanceled
		}
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	tracing.SetAttributes(span, tracing.AttrInt("http.status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		tracing.SetError(span, err)
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("%w: status %d: %s", ErrRequestFailed, resp.StatusCode, string(body))
		tracing.SetError(span, err)
		return nil, err
	}

	var embedResp openaiEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		tracing.SetError(span, err)
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	if embedResp.Error != nil {
		err := fmt.Errorf("%w: %s", ErrRequestFailed, embedResp.Error.Message)
		tracing.SetError(span, err)
		return nil, err
	}

	if len(embedResp.Data) == 0 {
		tracing.SetError(span, ErrInvalidResponse)
		return nil, ErrInvalidResponse
	}

	// Record token usage if available
	if embedResp.Usage.PromptTokens > 0 {
		tracing.SetAttributes(span, tracing.AttrInt("embedding.input_tokens", embedResp.Usage.PromptTokens))
	}

	embedding := toFloat32(embedResp.Data[0].Embedding)
	if len(embedding) != c.config.Dimensions {
		c.logger.Warn("Embedding dimension mismatch",
			logging.F("expected", c.config.Dimensions),
			logging.F("actual", len(embedding)),
		)
	}

	tracing.SetAttributes(span, tracing.AttrInt("embedding.dimensions", len(embedding)))
	tracing.SetOK(span)

	return embedding, nil
}

// doOllamaEmbedRequest uses the Ollama native /api/embeddings endpoint.
func (c *MLXClient) doOllamaEmbedRequest(ctx context.Context, text string) ([]float32, error) {
	url := strings.TrimSuffix(c.config.ServerURL, "/") + "/api/embeddings"

	reqBody := ollamaEmbedRequest{
		Model:  c.config.Model,
		Prompt: text,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrContextCanceled
		}
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d: %s", ErrRequestFailed, resp.StatusCode, string(body))
	}

	var embedResp ollamaEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	if embedResp.Error != "" {
		return nil, fmt.Errorf("%w: %s", ErrRequestFailed, embedResp.Error)
	}

	if len(embedResp.Embedding) == 0 {
		return nil, ErrInvalidResponse
	}

	embedding := toFloat32(embedResp.Embedding)
	if len(embedding) != c.config.Dimensions {
		c.logger.Warn("Embedding dimension mismatch",
			logging.F("expected", c.config.Dimensions),
			logging.F("actual", len(embedding)),
		)
	}

	return embedding, nil
}

// BatchEmbed generates embeddings for multiple texts in a single request.
func (c *MLXClient) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, ErrEmptyBatch
	}

	if len(texts) > c.config.BatchSize {
		return nil, fmt.Errorf("%w: got %d, max %d", ErrBatchTooLarge, len(texts), c.config.BatchSize)
	}

	// Start batch embedding trace
	start := time.Now()
	ctx, span := tracing.StartEmbedding(ctx, "embeddings.batch", tracing.EmbeddingOptions{
		Model:     c.config.Model,
		System:    tracing.AISystemMLX,
		BatchSize: len(texts),
	})
	defer span.End()

	// Trim and validate texts
	trimmedTexts := make([]string, 0, len(texts))
	for i, text := range texts {
		text = strings.TrimSpace(text)
		if text == "" {
			tracing.SetEmbeddingResult(span, tracing.EmbeddingResult{
				Error: fmt.Errorf("%w at index %d", ErrEmptyText, i),
			})
			return nil, fmt.Errorf("%w at index %d", ErrEmptyText, i)
		}
		trimmedTexts = append(trimmedTexts, text)
	}

	// Check cache for all texts
	embeddings := make([][]float32, len(trimmedTexts))
	uncachedIndices := make([]int, 0)
	uncachedTexts := make([]string, 0)
	cacheHits := 0

	if c.cache != nil {
		for i, text := range trimmedTexts {
			if embedding, err := c.cache.Get(ctx, text); err == nil {
				embeddings[i] = embedding
				cacheHits++
			} else {
				uncachedIndices = append(uncachedIndices, i)
				uncachedTexts = append(uncachedTexts, text)
			}
		}

		if len(uncachedTexts) == 0 {
			c.logger.Debug("All embeddings from cache",
				logging.F("count", len(texts)),
			)
			tracing.SetEmbeddingResult(span, tracing.EmbeddingResult{
				Dimensions: c.config.Dimensions,
				LatencyMs:  time.Since(start).Milliseconds(),
				Cached:     true,
			})
			tracing.RecordAIEvent(span, "batch.cache_hit",
				tracing.AttrInt("cache_hits", cacheHits),
				tracing.AttrInt("cache_misses", 0),
			)
			return embeddings, nil
		}
	} else {
		for i, text := range trimmedTexts {
			uncachedIndices = append(uncachedIndices, i)
			uncachedTexts = append(uncachedTexts, text)
		}
	}

	// Record cache stats
	tracing.RecordAIEvent(span, "batch.cache_check",
		tracing.AttrInt("cache_hits", cacheHits),
		tracing.AttrInt("cache_misses", len(uncachedTexts)),
	)

	// Generate embeddings for uncached texts
	newEmbeddings, err := c.batchEmbedRequest(ctx, uncachedTexts)
	if err != nil {
		tracing.SetEmbeddingResult(span, tracing.EmbeddingResult{
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     err,
		})
		return nil, err
	}

	// Merge results and cache new embeddings
	for i, idx := range uncachedIndices {
		embeddings[idx] = newEmbeddings[i]
		if c.cache != nil {
			if cacheErr := c.cache.Set(ctx, uncachedTexts[i], newEmbeddings[i]); cacheErr != nil {
				c.logger.Warn("Failed to cache embedding",
					logging.F("index", i),
					logging.Err(cacheErr),
				)
			}
		}
	}

	tracing.SetEmbeddingResult(span, tracing.EmbeddingResult{
		Dimensions: c.config.Dimensions,
		LatencyMs:  time.Since(start).Milliseconds(),
		Cached:     false,
	})

	return embeddings, nil
}

// batchEmbedRequest makes a batch embedding request to the server.
func (c *MLXClient) batchEmbedRequest(ctx context.Context, texts []string) ([][]float32, error) {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := c.config.RetryDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ErrContextCanceled
			case <-time.After(delay):
			}
		}

		// Try batch endpoint first
		embeddings, err := c.doBatchEmbedRequest(ctx, texts)
		if err == nil {
			return embeddings, nil
		}

		lastErr = err

		if errors.Is(err, ErrContextCanceled) {
			return nil, err
		}

		c.logger.Warn("Batch embedding request failed, retrying",
			logging.F("attempt", attempt+1),
			logging.F("batch_size", len(texts)),
			logging.Err(err),
		)
	}

	// Fall back to individual requests
	c.logger.Warn("Batch embedding failed, falling back to individual requests",
		logging.F("batch_size", len(texts)),
		logging.Err(lastErr),
	)

	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		embedding, err := c.embedSingle(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embedding text %d: %w", i, err)
		}
		embeddings[i] = embedding
	}

	return embeddings, nil
}

// doBatchEmbedRequest attempts a batch embedding request.
func (c *MLXClient) doBatchEmbedRequest(ctx context.Context, texts []string) ([][]float32, error) {
	// Try OpenAI-compatible batch endpoint
	url := strings.TrimSuffix(c.config.ServerURL, "/") + "/v1/embeddings"

	reqBody := openaiEmbedRequest{
		Model: c.config.Model,
		Input: texts,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrContextCanceled
		}
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d: %s", ErrRequestFailed, resp.StatusCode, string(body))
	}

	var embedResp openaiEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	if embedResp.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrRequestFailed, embedResp.Error.Message)
	}

	if len(embedResp.Data) != len(texts) {
		return nil, fmt.Errorf("%w: expected %d embeddings, got %d",
			ErrInvalidResponse, len(texts), len(embedResp.Data))
	}

	// Sort by index to ensure correct order
	embeddings := make([][]float32, len(texts))
	for _, data := range embedResp.Data {
		if data.Index < 0 || data.Index >= len(texts) {
			return nil, fmt.Errorf("%w: invalid index %d", ErrInvalidResponse, data.Index)
		}
		embeddings[data.Index] = toFloat32(data.Embedding)
	}

	return embeddings, nil
}

// Dimensions returns the number of dimensions in the embedding vectors.
func (c *MLXClient) Dimensions() int {
	return c.config.Dimensions
}

// ModelInfo returns information about the embedding model.
func (c *MLXClient) ModelInfo() *ModelInfo {
	return c.config.GetModelInfo()
}

// Close releases any resources held by the client.
func (c *MLXClient) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// toFloat32 converts a float64 slice to float32.
func toFloat32(f64 []float64) []float32 {
	f32 := make([]float32, len(f64))
	for i, v := range f64 {
		f32[i] = float32(v)
	}
	return f32
}
