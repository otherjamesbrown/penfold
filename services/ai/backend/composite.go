// Package backend provides backend connectors for AI services.
package backend

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// CompositeBackend delegates embedding operations to one backend (e.g. MLX/Ollama)
// and routes LLM operations by model name: gemini-* models go to the Gemini backend,
// anthropic-* models go to the Anthropic backend, all other models go to the local/Ollama backend.
type CompositeBackend struct {
	embeddings Backend // handles GenerateEmbedding + CheckEmbeddingsHealth
	gemini     Backend // handles gemini-* ChatCompletion + CheckLLMHealth
	ollama     Backend // handles non-gemini/non-anthropic ChatCompletion
	anthropic  Backend // handles anthropic-* ChatCompletion (may be nil if not configured)

	mu         sync.RWMutex
	semaphores map[string]chan struct{} // provider → buffered channel (nil = unlimited)
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

// SetSemaphores configures per-provider concurrency limits.
// cfg maps provider name (e.g. "gemini", "ollama") to maximum concurrent requests.
// Values ≤ 0 are ignored. This method is safe to call before the backend begins serving
// requests; replacing semaphores under load is not supported.
func (b *CompositeBackend) SetSemaphores(cfg map[string]int) {
	sems := make(map[string]chan struct{}, len(cfg))
	for provider, max := range cfg {
		if max > 0 {
			sems[provider] = make(chan struct{}, max)
		}
	}
	b.mu.Lock()
	b.semaphores = sems
	b.mu.Unlock()
}

// semaphoreFor returns the semaphore channel for provider, or nil if unlimited.
func (b *CompositeBackend) semaphoreFor(provider string) chan struct{} {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.semaphores == nil {
		return nil
	}
	return b.semaphores[provider]
}

// acquireSemaphore acquires a concurrency slot for provider.
// Returns (concurrent, max, nil) on success where concurrent is the occupancy
// (including this request) and max is the channel capacity.
// Returns (0, 0, nil) if no semaphore is configured for provider.
// Returns (0, cap, ctx.Err()) if the context is cancelled while waiting.
func (b *CompositeBackend) acquireSemaphore(ctx context.Context, provider string) (concurrent, max int, err error) {
	sem := b.semaphoreFor(provider)
	if sem == nil {
		return 0, 0, nil
	}
	select {
	case sem <- struct{}{}:
		return len(sem), cap(sem), nil
	case <-ctx.Done():
		return 0, cap(sem), ctx.Err()
	}
}

// releaseSemaphore releases the concurrency slot for provider.
func (b *CompositeBackend) releaseSemaphore(provider string) {
	sem := b.semaphoreFor(provider)
	if sem != nil {
		<-sem
	}
}

// semProvider resolves the effective provider for semaphore acquisition from a model ID.
// Handles the backward-compat case where a model has no "/" prefix but contains "gemini".
func semProvider(opts CompletionOptions) string {
	p := extractProvider(opts.Model)
	if p != "" {
		return p
	}
	if strings.Contains(strings.ToLower(opts.Model), "gemini") {
		return "gemini"
	}
	return "ollama"
}

func (b *CompositeBackend) GenerateEmbedding(ctx context.Context, text string, model string) (*EmbeddingResult, error) {
	return b.embeddings.GenerateEmbedding(ctx, text, model)
}

func (b *CompositeBackend) ChatCompletion(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error) {
	provider := semProvider(opts)

	// Acquire a concurrency slot for this provider before executing the LLM call.
	acquireStart := time.Now()
	concurrent, max, err := b.acquireSemaphore(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("semaphore acquire for %s: %w", provider, err)
	}
	if max > 0 {
		defer b.releaseSemaphore(provider)
	}
	semWaitMs := time.Since(acquireStart).Milliseconds()

	// Route to the appropriate backend (original logic preserved).
	var result *CompletionResult
	origProvider := extractProvider(opts.Model)
	switch origProvider {
	case "gemini":
		result, err = b.gemini.ChatCompletion(ctx, messages, opts)
	case "anthropic":
		if b.anthropic == nil {
			return nil, fmt.Errorf("%w: anthropic backend not configured — set ANTHROPIC_API_KEY", ErrRequestFailed)
		}
		result, err = b.anthropic.ChatCompletion(ctx, messages, opts)
	default:
		// Backward compat: if no provider prefix but contains "gemini", route to gemini
		if origProvider == "" && strings.Contains(strings.ToLower(opts.Model), "gemini") {
			result, err = b.gemini.ChatCompletion(ctx, messages, opts)
		} else {
			result, err = b.ollama.ChatCompletion(ctx, messages, opts)
		}
	}

	if err != nil {
		return nil, err
	}

	// Attach semaphore observability metrics so callers can log and report them.
	if max > 0 {
		result.SemaphoreWaitMs = semWaitMs
		result.BackendConcurrent = concurrent
		result.BackendMax = max
	}

	return result, nil
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
