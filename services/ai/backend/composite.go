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

	// Per-provider concurrency semaphores (buffered channels).
	// Acquire by sending; release by receiving. Falls back to "default" provider.
	mu                sync.RWMutex
	backendSemaphores map[string]chan struct{} // provider_name -> buffered channel
	semaphoreConfig   map[string]int          // provider_name -> max concurrent
	configLoader      func() (map[string]int, error)
	stopReload        chan struct{}
}

// NewCompositeBackend creates a backend that routes embeddings to the
// embeddings backend and LLM calls based on model name.
// anthropic may be nil for graceful degradation when no API key is configured.
// configLoader may be nil to disable per-provider semaphores.
func NewCompositeBackend(embeddings, ollama, gemini Backend, anthropic Backend) *CompositeBackend {
	return &CompositeBackend{
		embeddings: embeddings,
		gemini:     gemini,
		ollama:     ollama,
		anthropic:  anthropic,
	}
}

// WithConfigLoader sets the semaphore config loader and initializes semaphores.
// Returns an error if the initial config load fails (e.g. missing default key).
// Starts a background goroutine that reloads config every 60 seconds.
func (b *CompositeBackend) WithConfigLoader(loader func() (map[string]int, error)) error {
	b.configLoader = loader
	if err := b.initSemaphores(); err != nil {
		return err
	}
	b.stopReload = make(chan struct{})
	go b.hotReloadLoop()
	return nil
}

// initSemaphores loads config and creates buffered channels for each provider.
// Returns an error if the "default" key is missing (startup validation).
func (b *CompositeBackend) initSemaphores() error {
	config, err := b.configLoader()
	if err != nil {
		return fmt.Errorf("load semaphore config: %w", err)
	}

	if _, ok := config["default"]; !ok {
		return fmt.Errorf("model.concurrency.default not found in semaphore config — seed the DB row before starting")
	}

	sems := make(map[string]chan struct{}, len(config))
	for provider, limit := range config {
		sems[provider] = make(chan struct{}, limit)
	}

	b.mu.Lock()
	b.semaphoreConfig = config
	b.backendSemaphores = sems
	b.mu.Unlock()
	return nil
}

// hotReloadLoop reloads semaphore config every 60 seconds and resizes channels as needed.
func (b *CompositeBackend) hotReloadLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = b.reloadSemaphores()
		case <-b.stopReload:
			return
		}
	}
}

// reloadSemaphores reloads config and resizes semaphores whose limits changed.
func (b *CompositeBackend) reloadSemaphores() error {
	config, err := b.configLoader()
	if err != nil {
		return fmt.Errorf("reload semaphore config: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for provider, newLimit := range config {
		oldLimit, exists := b.semaphoreConfig[provider]
		if !exists || oldLimit != newLimit {
			// Resize by creating a new channel. Drain in-flight tokens from old.
			old := b.backendSemaphores[provider]
			newSem := make(chan struct{}, newLimit)
			if old != nil {
				// Preserve in-flight slots: copy as many tokens as fit.
				inFlight := len(old)
				for i := 0; i < inFlight && len(newSem) < newLimit; i++ {
					newSem <- struct{}{}
				}
			}
			b.backendSemaphores[provider] = newSem
		}
	}
	// Add any new providers not previously present.
	for provider, limit := range config {
		if _, exists := b.backendSemaphores[provider]; !exists {
			b.backendSemaphores[provider] = make(chan struct{}, limit)
		}
	}
	b.semaphoreConfig = config
	return nil
}

// getSemaphore returns the semaphore channel for the given provider,
// falling back to the "default" semaphore if no provider-specific one exists.
// Returns nil if semaphores are not configured.
func (b *CompositeBackend) getSemaphore(provider string) chan struct{} {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.backendSemaphores == nil {
		return nil
	}
	if sem, ok := b.backendSemaphores[provider]; ok {
		return sem
	}
	return b.backendSemaphores["default"]
}

// acquireSemaphore blocks until a slot is available for the given provider,
// or until ctx is cancelled. Returns nil if semaphores are not configured.
func (b *CompositeBackend) acquireSemaphore(ctx context.Context, provider string) error {
	sem := b.getSemaphore(provider)
	if sem == nil {
		return nil
	}
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// releaseSemaphore frees a slot for the given provider.
// No-op if semaphores are not configured.
func (b *CompositeBackend) releaseSemaphore(provider string) {
	sem := b.getSemaphore(provider)
	if sem == nil {
		return
	}
	<-sem
}

func (b *CompositeBackend) GenerateEmbedding(ctx context.Context, text string, model string) (*EmbeddingResult, error) {
	return b.embeddings.GenerateEmbedding(ctx, text, model)
}

func (b *CompositeBackend) ChatCompletion(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error) {
	provider := extractProvider(opts.Model)

	if err := b.acquireSemaphore(ctx, provider); err != nil {
		return nil, fmt.Errorf("acquire semaphore for provider %q: %w", provider, err)
	}
	defer b.releaseSemaphore(provider)

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
	if b.stopReload != nil {
		close(b.stopReload)
	}
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
