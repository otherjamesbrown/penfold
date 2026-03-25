// Package backend provides backend connectors for AI services.
// It abstracts the communication with embedding and LLM servers.
package backend

import (
	"context"
	"encoding/json"
	"strings"
)

// Backend defines the interface for AI backend operations.
// Implementations connect to embedding servers and LLM servers
// to provide AI capabilities.
type Backend interface {
	// Embedding operations

	// GenerateEmbedding creates a vector embedding for the given text.
	// Returns the embedding as a slice of float32 values.
	GenerateEmbedding(ctx context.Context, text string, model string) (*EmbeddingResult, error)

	// LLM operations

	// ChatCompletion sends a chat completion request to the LLM.
	// Returns the generated text response.
	ChatCompletion(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error)

	// Health operations

	// CheckEmbeddingsHealth checks if the embeddings service is healthy.
	CheckEmbeddingsHealth(ctx context.Context) error

	// CheckLLMHealth checks if the LLM service is healthy.
	CheckLLMHealth(ctx context.Context) error

	// Close releases any resources held by the backend.
	Close() error
}

// Message represents a chat message for LLM completion.
type Message struct {
	Role    string `json:"role"`    // "system", "user", or "assistant"
	Content string `json:"content"` // Message content
}

// CompletionOptions configures the LLM completion request.
type CompletionOptions struct {
	// Model specifies the model to use. Empty uses the default.
	Model string

	// MaxTokens limits the response length. 0 uses the default.
	MaxTokens int

	// Temperature controls randomness (0.0-1.0). 0 uses the default.
	Temperature float32

	// JSONMode requests structured JSON output.
	JSONMode bool

	// ResponseSchema is a JSON Schema for structured output enforcement.
	// When set, backends translate this to their native format (Gemini responseSchema,
	// OpenAI json_schema, Ollama format schema object).
	ResponseSchema json.RawMessage
}

// EmbeddingResult contains the result of an embedding generation.
type EmbeddingResult struct {
	// Vector is the embedding vector.
	Vector []float32

	// Dimensions is the number of dimensions in the vector.
	Dimensions int

	// Model is the model that was used.
	Model string

	// TokenCount is the number of tokens in the input text.
	TokenCount int
}

// CompletionResult contains the result of an LLM completion.
type CompletionResult struct {
	// Content is the generated text.
	Content string

	// Model is the model that was used.
	Model string

	// InputTokens is the number of input tokens.
	InputTokens int

	// OutputTokens is the number of output tokens.
	OutputTokens int

	// FinishReason indicates why the model stopped generating.
	// "stop" = natural end, "length" = hit max_tokens limit.
	FinishReason string

	// SemaphoreWaitMs is the time in milliseconds spent waiting for a concurrency
	// semaphore slot before the LLM call was executed. Zero means no semaphore
	// throttling occurred (either no semaphore is configured, or the slot was
	// available immediately).
	SemaphoreWaitMs int64

	// BackendConcurrent is the number of concurrent requests to this provider's
	// semaphore at the moment this request acquired its slot (including this request).
	// Zero if no semaphore is configured for this provider.
	BackendConcurrent int

	// BackendMax is the configured maximum concurrent requests for this provider.
	// Zero if no semaphore is configured.
	BackendMax int
}

// extractProvider returns the provider portion of a "provider/model-name" string.
// Returns empty string if no "/" is present.
func extractProvider(modelID string) string {
	if idx := strings.Index(modelID, "/"); idx >= 0 {
		return modelID[:idx]
	}
	return ""
}

// ExtractModelName returns the model name portion of a "provider/model-name" string.
// Returns the whole string if no "/" is present (backward compatibility).
func ExtractModelName(modelID string) string {
	if idx := strings.Index(modelID, "/"); idx >= 0 {
		return modelID[idx+1:]
	}
	return modelID
}
