// Package backend provides backend connectors for AI services.
package backend

import "context"

// CompositeBackend delegates embedding operations to one backend (e.g. MLX)
// and LLM operations to another (e.g. Gemini). This allows using local
// embeddings (preserving 1024d vectors) while routing chat completions
// through a cloud LLM for reliability and quality.
type CompositeBackend struct {
	embeddings Backend // handles GenerateEmbedding + CheckEmbeddingsHealth
	llm        Backend // handles ChatCompletion + CheckLLMHealth
}

// NewCompositeBackend creates a backend that routes embeddings to the
// embeddings backend and LLM calls to the llm backend.
func NewCompositeBackend(embeddings, llm Backend) *CompositeBackend {
	return &CompositeBackend{
		embeddings: embeddings,
		llm:        llm,
	}
}

func (b *CompositeBackend) GenerateEmbedding(ctx context.Context, text string, model string) (*EmbeddingResult, error) {
	return b.embeddings.GenerateEmbedding(ctx, text, model)
}

func (b *CompositeBackend) ChatCompletion(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error) {
	return b.llm.ChatCompletion(ctx, messages, opts)
}

func (b *CompositeBackend) CheckEmbeddingsHealth(ctx context.Context) error {
	return b.embeddings.CheckEmbeddingsHealth(ctx)
}

func (b *CompositeBackend) CheckLLMHealth(ctx context.Context) error {
	return b.llm.CheckLLMHealth(ctx)
}

func (b *CompositeBackend) Close() error {
	embErr := b.embeddings.Close()
	llmErr := b.llm.Close()
	if embErr != nil {
		return embErr
	}
	return llmErr
}
