// Package backend provides backend connectors for AI services.
package backend

import (
	"context"
	"strings"
)

// CompositeBackend delegates embedding operations to one backend (e.g. MLX/Ollama)
// and routes LLM operations by model name: gemini-* models go to the Gemini backend,
// all other models go to the local/Ollama backend.
type CompositeBackend struct {
	embeddings Backend // handles GenerateEmbedding + CheckEmbeddingsHealth
	gemini     Backend // handles gemini-* ChatCompletion + CheckLLMHealth
	ollama     Backend // handles non-gemini ChatCompletion
}

// NewCompositeBackend creates a backend that routes embeddings to the
// embeddings backend and LLM calls based on model name.
func NewCompositeBackend(embeddings, ollama, gemini Backend) *CompositeBackend {
	return &CompositeBackend{
		embeddings: embeddings,
		gemini:     gemini,
		ollama:     ollama,
	}
}

func (b *CompositeBackend) GenerateEmbedding(ctx context.Context, text string, model string) (*EmbeddingResult, error) {
	return b.embeddings.GenerateEmbedding(ctx, text, model)
}

func (b *CompositeBackend) ChatCompletion(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error) {
	if strings.Contains(strings.ToLower(opts.Model), "gemini") {
		return b.gemini.ChatCompletion(ctx, messages, opts)
	}
	return b.ollama.ChatCompletion(ctx, messages, opts)
}

func (b *CompositeBackend) CheckEmbeddingsHealth(ctx context.Context) error {
	return b.embeddings.CheckEmbeddingsHealth(ctx)
}

func (b *CompositeBackend) CheckLLMHealth(ctx context.Context) error {
	return b.gemini.CheckLLMHealth(ctx)
}

func (b *CompositeBackend) Close() error {
	embErr := b.embeddings.Close()
	gemErr := b.gemini.Close()
	ollamaErr := b.ollama.Close()
	if embErr != nil {
		return embErr
	}
	if gemErr != nil {
		return gemErr
	}
	return ollamaErr
}
