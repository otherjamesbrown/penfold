// Package embeddings provides embedding generation for semantic search.
// This package uses the centralized AI service via AIClient for embedding generation.
package embeddings

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
)

// Client defines the interface for generating text embeddings.
// Implementations connect to embedding models (local MLX or remote APIs)
// and return vector representations of text.
type Client interface {
	// Embed generates a vector embedding for the given text.
	// Returns the embedding as a slice of float32 values.
	// Returns ErrServiceUnavailable if the service cannot be reached.
	Embed(ctx context.Context, text string) ([]float32, error)

	// BatchEmbed generates embeddings for multiple texts in a single request.
	// This is more efficient than calling Embed multiple times.
	// Returns embeddings in the same order as the input texts.
	BatchEmbed(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the number of dimensions in the embedding vectors.
	Dimensions() int

	// ModelInfo returns information about the embedding model.
	ModelInfo() *ModelInfo

	// Close releases any resources held by the client.
	Close() error
}

// AIClient defines the interface for AI service operations.
// This is a subset of the full AIClient interface from services/worker/activities,
// containing only the methods needed for embedding generation.
type AIClient interface {
	// GenerateEmbedding generates a vector embedding for text.
	GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error)
}

// AIBackedClient generates embeddings using the centralized AI service via gRPC.
// It supports caching and batch processing.
type AIBackedClient struct {
	aiClient AIClient
	config   *Config
	logger   logging.Logger
	cache    Cache

	// mu protects concurrent access to stats
	mu          sync.RWMutex
	requestsTotal int64
	errorsTotal   int64
}

// NewAIBackedClient creates a new embedding client that uses the AI service.
// If config is nil, DefaultConfig() is used.
// The cache parameter is optional; if nil, no caching is performed.
func NewAIBackedClient(aiClient AIClient, config *Config, logger logging.Logger, cache Cache) (*AIBackedClient, error) {
	if aiClient == nil {
		return nil, errors.New("embeddings: AIClient is required")
	}

	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	client := &AIBackedClient{
		aiClient: aiClient,
		config:   config,
		logger:   logger.With(logging.F("component", "ai_embedding_client")),
		cache:    cache,
	}

	return client, nil
}

// Embed generates a vector embedding for the given text.
func (c *AIBackedClient) Embed(ctx context.Context, text string) ([]float32, error) {
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

	// Generate embedding via AI service
	embedding, err := c.embedSingle(ctx, text)
	if err != nil {
		c.mu.Lock()
		c.errorsTotal++
		c.mu.Unlock()

		tracing.SetEmbeddingResult(span, tracing.EmbeddingResult{
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     err,
		})
		return nil, err
	}

	c.mu.Lock()
	c.requestsTotal++
	c.mu.Unlock()

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

// embedSingle generates a single embedding via the AI service.
func (c *AIBackedClient) embedSingle(ctx context.Context, text string) ([]float32, error) {
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

// doEmbedRequest performs a single embedding request via gRPC.
func (c *AIBackedClient) doEmbedRequest(ctx context.Context, text string) ([]float32, error) {
	// Create the gRPC request
	req := &aiv1.EmbeddingRequest{
		Text: text,
	}

	// Set model if configured (optional field)
	if c.config.Model != "" {
		model := c.config.Model
		req.Model = &model
	}

	// Call the AI service
	resp, err := c.aiClient.GenerateEmbedding(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrContextCanceled
		}
		return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}

	// Validate response
	if resp == nil || len(resp.Vector) == 0 {
		return nil, ErrInvalidResponse
	}

	// Check dimensions
	if c.config.Dimensions > 0 && len(resp.Vector) != c.config.Dimensions {
		c.logger.Warn("Embedding dimension mismatch",
			logging.F("expected", c.config.Dimensions),
			logging.F("actual", len(resp.Vector)),
		)
	}

	return resp.Vector, nil
}

// BatchEmbed generates embeddings for multiple texts.
// Since the AI service doesn't have a batch endpoint, this makes concurrent
// gRPC calls with bounded concurrency for efficiency.
func (c *AIBackedClient) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
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

	// Generate embeddings for uncached texts with bounded concurrency
	newEmbeddings, err := c.batchEmbedConcurrent(ctx, uncachedTexts)
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

// batchEmbedConcurrent generates embeddings concurrently with bounded parallelism.
func (c *AIBackedClient) batchEmbedConcurrent(ctx context.Context, texts []string) ([][]float32, error) {
	// Use a worker pool with bounded concurrency (configurable via Config.MaxConcurrency)
	maxConcurrency := c.config.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = DefaultMaxConcurrency
	}

	type result struct {
		index     int
		embedding []float32
		err       error
	}

	results := make(chan result, len(texts))
	semaphore := make(chan struct{}, maxConcurrency)

	var wg sync.WaitGroup
	for i, text := range texts {
		wg.Add(1)
		go func(idx int, txt string) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case <-ctx.Done():
				results <- result{index: idx, err: ErrContextCanceled}
				return
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			}

			// Generate embedding
			embedding, err := c.embedSingle(ctx, txt)
			results <- result{index: idx, embedding: embedding, err: err}
		}(i, text)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	embeddings := make([][]float32, len(texts))
	var firstErr error

	for r := range results {
		if r.err != nil && firstErr == nil {
			firstErr = fmt.Errorf("embedding text %d: %w", r.index, r.err)
		}
		if r.embedding != nil {
			embeddings[r.index] = r.embedding
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}

	return embeddings, nil
}

// Dimensions returns the number of dimensions in the embedding vectors.
func (c *AIBackedClient) Dimensions() int {
	return c.config.Dimensions
}

// ModelInfo returns information about the embedding model.
func (c *AIBackedClient) ModelInfo() *ModelInfo {
	return c.config.GetModelInfo()
}

// Close releases any resources held by the client.
func (c *AIBackedClient) Close() error {
	// No resources to release (AIClient lifecycle managed externally)
	return nil
}

// Stats returns client statistics.
func (c *AIBackedClient) Stats() ClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ClientStats{
		RequestsTotal: c.requestsTotal,
		ErrorsTotal:   c.errorsTotal,
	}
}

// ClientStats contains client statistics.
type ClientStats struct {
	RequestsTotal int64
	ErrorsTotal   int64
}

// MockClient is a mock implementation of the Client interface for testing.
type MockClient struct {
	embedFunc      func(ctx context.Context, text string) ([]float32, error)
	batchEmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)
	dimensions     int
	modelInfo      *ModelInfo
}

// NewMockClient creates a new mock embedding client.
// The embedFunc will be called for Embed requests.
// If embedFunc is nil, a default implementation returning random embeddings is used.
func NewMockClient(dimensions int, embedFunc func(ctx context.Context, text string) ([]float32, error)) *MockClient {
	return &MockClient{
		embedFunc:  embedFunc,
		dimensions: dimensions,
		modelInfo: &ModelInfo{
			Name:        "mock-model",
			Dimensions:  dimensions,
			MaxTokens:   512,
			Provider:    "mock",
			IsLocal:     true,
			Description: "Mock embedding model for testing",
		},
	}
}

// Embed generates a mock embedding.
func (m *MockClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.embedFunc != nil {
		return m.embedFunc(ctx, text)
	}
	// Default: return zero vector
	return make([]float32, m.dimensions), nil
}

// BatchEmbed generates mock embeddings for multiple texts.
func (m *MockClient) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	if m.batchEmbedFunc != nil {
		return m.batchEmbedFunc(ctx, texts)
	}

	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		embedding, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		embeddings[i] = embedding
	}
	return embeddings, nil
}

// Dimensions returns the configured dimensions.
func (m *MockClient) Dimensions() int {
	return m.dimensions
}

// ModelInfo returns the mock model info.
func (m *MockClient) ModelInfo() *ModelInfo {
	return m.modelInfo
}

// Close is a no-op for the mock client.
func (m *MockClient) Close() error {
	return nil
}

// MockAIClient is a mock implementation of the AIClient interface for testing.
type MockAIClient struct {
	EmbeddingFunc func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error)
}

// GenerateEmbedding implements AIClient.
func (m *MockAIClient) GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	if m.EmbeddingFunc != nil {
		return m.EmbeddingFunc(ctx, req)
	}
	// Default: return a 1024-dimension zero vector
	return &aiv1.EmbeddingResponse{
		Vector:     make([]float32, 1024),
		Dimensions: 1024,
		ModelUsed:  "mock-model",
	}, nil
}
