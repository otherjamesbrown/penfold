// Package clients provides AI client adapters for the pipeline.
// These adapters wrap the centralized pkg/ai client to provide
// the interfaces expected by pipeline activities.
package clients

import (
	"context"
	"fmt"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/ai"
	"github.com/rs/zerolog"
)

// Assertion represents an extracted assertion from content.
// This is used by pipeline activities for assertion extraction.
type Assertion struct {
	Statement  string  `json:"statement"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source,omitempty"`
	Category   string  `json:"category,omitempty"`
}

// AIClientInterface defines the interface for the centralized AI client.
// This interface allows for testing and decoupling from the concrete implementation.
type AIClientInterface interface {
	GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error)
	GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error)
	ExtractAssertions(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error)
	HealthCheck(ctx context.Context) error
	Close() error
}

// EmbeddingsAdapter wraps the centralized AI client to provide the
// embeddings interface expected by pipeline activities.
type EmbeddingsAdapter struct {
	client AIClientInterface
	model  string
	logger zerolog.Logger
}

// NewEmbeddingsAdapter creates a new embeddings adapter wrapping the AI client.
func NewEmbeddingsAdapter(client AIClientInterface, model string, logger zerolog.Logger) *EmbeddingsAdapter {
	return &EmbeddingsAdapter{
		client: client,
		model:  model,
		logger: logger.With().Str("component", "embeddings-adapter").Logger(),
	}
}

// Embed generates an embedding for a single text input.
func (a *EmbeddingsAdapter) Embed(ctx context.Context, text string) ([]float64, error) {
	embeddings, err := a.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple text inputs.
// Note: The centralized AI service processes one at a time via gRPC,
// so this iterates and collects results.
func (a *EmbeddingsAdapter) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	a.logger.Debug().
		Int("batch_size", len(texts)).
		Msg("generating embeddings batch via AI service")

	embeddings := make([][]float64, len(texts))

	for i, text := range texts {
		req := &aiv1.EmbeddingRequest{
			Text: text,
		}
		if a.model != "" {
			req.Model = &a.model
		}

		resp, err := a.client.GenerateEmbedding(ctx, req)
		if err != nil {
			a.logger.Error().Err(err).Int("index", i).Msg("embedding generation failed")
			return nil, fmt.Errorf("failed to generate embedding for text %d: %w", i, err)
		}

		// Convert float32 to float64
		embedding := make([]float64, len(resp.Vector))
		for j, v := range resp.Vector {
			embedding[j] = float64(v)
		}
		embeddings[i] = embedding
	}

	a.logger.Debug().
		Int("batch_size", len(texts)).
		Int("embeddings_count", len(embeddings)).
		Msg("embeddings generated successfully via AI service")

	return embeddings, nil
}

// HealthCheck verifies the embeddings service is healthy.
func (a *EmbeddingsAdapter) HealthCheck(ctx context.Context) error {
	return a.client.HealthCheck(ctx)
}

// Close releases resources. The underlying client lifecycle is managed externally.
func (a *EmbeddingsAdapter) Close() error {
	a.logger.Debug().Msg("embeddings adapter closed")
	return nil
}

// LLMAdapter wraps the centralized AI client to provide the
// LLM interface expected by pipeline activities.
type LLMAdapter struct {
	client AIClientInterface
	model  string
	logger zerolog.Logger
}

// NewLLMAdapter creates a new LLM adapter wrapping the AI client.
func NewLLMAdapter(client AIClientInterface, model string, logger zerolog.Logger) *LLMAdapter {
	return &LLMAdapter{
		client: client,
		model:  model,
		logger: logger.With().Str("component", "llm-adapter").Logger(),
	}
}

// GenerateSummary generates a summary of the given content.
func (a *LLMAdapter) GenerateSummary(ctx context.Context, content string) (string, error) {
	if content == "" {
		return "", fmt.Errorf("content cannot be empty")
	}

	a.logger.Debug().
		Int("content_length", len(content)).
		Msg("generating summary via AI service")

	req := &aiv1.SummaryRequest{
		Content: content,
		Style:   aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF,
	}
	if a.model != "" {
		req.Model = &a.model
	}

	resp, err := a.client.GenerateSummary(ctx, req)
	if err != nil {
		a.logger.Error().Err(err).Msg("summary generation failed")
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	a.logger.Debug().
		Int("summary_length", len(resp.Summary)).
		Str("model_used", resp.ModelUsed).
		Msg("summary generated successfully via AI service")

	return resp.Summary, nil
}

// ExtractAssertions extracts factual assertions from the given content.
func (a *LLMAdapter) ExtractAssertions(ctx context.Context, content string) ([]Assertion, error) {
	if content == "" {
		return nil, fmt.Errorf("content cannot be empty")
	}

	a.logger.Debug().
		Int("content_length", len(content)).
		Msg("extracting assertions via AI service")

	req := &aiv1.AssertionRequest{
		Content: content,
	}
	if a.model != "" {
		req.Model = &a.model
	}

	resp, err := a.client.ExtractAssertions(ctx, req)
	if err != nil {
		a.logger.Error().Err(err).Msg("assertion extraction failed")
		return nil, fmt.Errorf("failed to extract assertions: %w", err)
	}

	// Convert protobuf assertions to local type
	assertions := make([]Assertion, len(resp.Assertions))
	for i, pa := range resp.Assertions {
		category := ""
		if pa.Category != nil {
			category = *pa.Category
		}
		// Convert subject-predicate-object to a statement
		statement := fmt.Sprintf("%s %s %s", pa.Subject, pa.Predicate, pa.Object)
		assertions[i] = Assertion{
			Statement:  statement,
			Confidence: float64(pa.Confidence),
			Category:   category,
		}
	}

	a.logger.Debug().
		Int("assertion_count", len(assertions)).
		Str("model_used", resp.ModelUsed).
		Msg("assertions extracted successfully via AI service")

	return assertions, nil
}

// HealthCheck verifies the LLM service is healthy.
func (a *LLMAdapter) HealthCheck(ctx context.Context) error {
	return a.client.HealthCheck(ctx)
}

// Close releases resources. The underlying client lifecycle is managed externally.
func (a *LLMAdapter) Close() error {
	a.logger.Debug().Msg("LLM adapter closed")
	return nil
}

// NewAIClient creates a new centralized AI client.
// This is a convenience function to create the client with common options.
func NewAIClient(addr string, opts ...ai.ClientOption) (*ai.Client, error) {
	return ai.NewClient(addr, opts...)
}
