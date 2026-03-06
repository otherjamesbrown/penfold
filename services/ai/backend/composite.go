// Package backend provides backend connectors for AI services.
package backend

import (
	"context"
	"fmt"
	"strings"
)

// CompositeBackend delegates embedding operations to one backend (e.g. MLX/Ollama)
// and routes LLM operations by model name: gemini-* models go to the Gemini backend,
// anthropic-* models go to the Anthropic backend, all other models go to the local/Ollama backend.
type CompositeBackend struct {
	embeddings Backend // handles GenerateEmbedding + CheckEmbeddingsHealth
	gemini     Backend // handles gemini-* ChatCompletion + CheckLLMHealth
	ollama     Backend // handles non-gemini/non-anthropic ChatCompletion
	anthropic  Backend // handles anthropic-* ChatCompletion (may be nil if not configured)
}

// NewCompositeBackend creates a backend that routes embeddings to the
// embeddings backend and LLM calls based on model name.
// anthropic may be nil for graceful degradation when no API key is configured.
func NewCompositeBackend(embeddings, ollama, gemini Backend, anthropic Backend) *CompositeBackend {
	return &CompositeBackend{
		embeddings: embeddings,
		gemini:     gemini,
		ollama:     ollama,
		anthropic:  anthropic,
	}
}

func (b *CompositeBackend) GenerateEmbedding(ctx context.Context, text string, model string) (*EmbeddingResult, error) {
	return b.embeddings.GenerateEmbedding(ctx, text, model)
}

func (b *CompositeBackend) ChatCompletion(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error) {
	provider := extractProvider(opts.Model)
	switch provider {
	case "gemini":
		return b.gemini.ChatCompletion(ctx, messages, opts)
	case "anthropic":
		if b.anthropic == nil {
			return nil, fmt.Errorf("%w: anthropic backend not configured — set ANTHROPIC_API_KEY", ErrRequestFailed)
		}
		return b.anthropic.ChatCompletion(ctx, messages, opts)
	default:
		// Backward compat: if no provider prefix but contains "gemini", route to gemini
		if provider == "" && strings.Contains(strings.ToLower(opts.Model), "gemini") {
			return b.gemini.ChatCompletion(ctx, messages, opts)
		}
		return b.ollama.ChatCompletion(ctx, messages, opts)
	}
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
	var anthErr error
	if b.anthropic != nil {
		anthErr = b.anthropic.Close()
	}
	for _, err := range []error{embErr, gemErr, ollamaErr, anthErr} {
		if err != nil {
			return err
		}
	}
	return nil
}
