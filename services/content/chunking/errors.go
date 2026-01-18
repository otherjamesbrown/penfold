// Package chunking provides error definitions for chunking operations.
package chunking

import "errors"

// Configuration validation errors.
var (
	// ErrInvalidOverlapPercentage indicates overlap percentage is out of range.
	ErrInvalidOverlapPercentage = errors.New("overlap percentage must be between 0 and 50")

	// ErrInvalidMinOverlap indicates minimum overlap is invalid.
	ErrInvalidMinOverlap = errors.New("minimum overlap cannot be negative")

	// ErrMaxLessThanMin indicates max overlap is less than min.
	ErrMaxLessThanMin = errors.New("maximum overlap cannot be less than minimum")

	// ErrInvalidMinTokens indicates minimum tokens is invalid.
	ErrInvalidMinTokens = errors.New("minimum tokens cannot be negative")

	// ErrTargetLessThanMin indicates target is less than minimum.
	ErrTargetLessThanMin = errors.New("target tokens cannot be less than minimum")

	// ErrMaxLessThanTarget indicates max is less than target.
	ErrMaxLessThanTarget = errors.New("maximum tokens cannot be less than target")
)

// Chunking operation errors.
var (
	// ErrEmptyContent indicates the content is empty.
	ErrEmptyContent = errors.New("content is empty")

	// ErrChunkingFailed indicates chunking operation failed.
	ErrChunkingFailed = errors.New("chunking operation failed")

	// ErrEmbeddingFailed indicates embedding generation failed.
	ErrEmbeddingFailed = errors.New("embedding generation failed")

	// ErrInvalidChunker indicates an invalid chunker type.
	ErrInvalidChunker = errors.New("invalid chunker type")
)
