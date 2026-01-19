// Package embeddings provides MLX embedding generation for semantic search.
package embeddings

import "errors"

// Configuration errors.
var (
	// ErrInvalidServerURL indicates the server URL is empty or invalid.
	ErrInvalidServerURL = errors.New("embeddings: server URL is required")

	// ErrInvalidModel indicates the model name is empty or invalid.
	ErrInvalidModel = errors.New("embeddings: model name is required")

	// ErrInvalidBatchSize indicates the batch size is invalid.
	ErrInvalidBatchSize = errors.New("embeddings: batch size must be positive")

	// ErrInvalidDimensions indicates the dimensions value is invalid.
	ErrInvalidDimensions = errors.New("embeddings: dimensions must be positive")

	// ErrInvalidTimeout indicates the timeout value is invalid.
	ErrInvalidTimeout = errors.New("embeddings: timeout must be positive")
)

// Client operation errors.
var (
	// ErrEmptyText indicates an empty string was provided for embedding.
	ErrEmptyText = errors.New("embeddings: text cannot be empty")

	// ErrEmptyBatch indicates an empty batch was provided for embedding.
	ErrEmptyBatch = errors.New("embeddings: batch cannot be empty")

	// ErrBatchTooLarge indicates the batch exceeds the configured maximum size.
	ErrBatchTooLarge = errors.New("embeddings: batch exceeds maximum size")

	// ErrServiceUnavailable indicates the embedding service cannot be reached.
	ErrServiceUnavailable = errors.New("embeddings: service unavailable")

	// ErrRequestFailed indicates the embedding request failed.
	ErrRequestFailed = errors.New("embeddings: request failed")

	// ErrInvalidResponse indicates the server returned an invalid response.
	ErrInvalidResponse = errors.New("embeddings: invalid server response")

	// ErrDimensionMismatch indicates the returned embedding has wrong dimensions.
	ErrDimensionMismatch = errors.New("embeddings: dimension mismatch")

	// ErrContextCanceled indicates the context was canceled during the request.
	ErrContextCanceled = errors.New("embeddings: context canceled")

	// ErrRequestTimeout indicates the request timed out.
	ErrRequestTimeout = errors.New("embeddings: request timeout")
)

// Cache errors.
var (
	// ErrCacheMiss indicates the requested embedding was not found in cache.
	ErrCacheMiss = errors.New("embeddings: cache miss")

	// ErrCacheUnavailable indicates the cache backend is unavailable.
	ErrCacheUnavailable = errors.New("embeddings: cache unavailable")
)
