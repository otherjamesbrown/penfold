package backend

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// stubBackend is a minimal Backend implementation for testing.
type stubBackend struct {
	chatFn func(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error)
}

func (s *stubBackend) GenerateEmbedding(_ context.Context, _ string, _ string) (*EmbeddingResult, error) {
	return &EmbeddingResult{}, nil
}
func (s *stubBackend) ChatCompletion(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error) {
	if s.chatFn != nil {
		return s.chatFn(ctx, messages, opts)
	}
	return &CompletionResult{Content: "ok"}, nil
}
func (s *stubBackend) CheckEmbeddingsHealth(_ context.Context) error { return nil }
func (s *stubBackend) CheckLLMHealth(_ context.Context) error        { return nil }
func (s *stubBackend) Close() error                                   { return nil }

func newTestComposite() *CompositeBackend {
	stub := &stubBackend{}
	return NewCompositeBackend(stub, stub, stub, stub)
}

func testLoader(config map[string]int) func() (map[string]int, error) {
	return func() (map[string]int, error) { return config, nil }
}

// TestCompositeBackend_AcquireRelease verifies that acquire/release round-trips a slot.
func TestCompositeBackend_AcquireRelease(t *testing.T) {
	b := newTestComposite()
	if err := b.WithConfigLoader(testLoader(map[string]int{
		"gemini":  2,
		"default": 5,
	})); err != nil {
		t.Fatalf("WithConfigLoader: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	if err := b.acquireSemaphore(ctx, "gemini"); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	b.releaseSemaphore("gemini")

	// Verify slot is free by acquiring at full capacity.
	if err := b.acquireSemaphore(ctx, "gemini"); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if err := b.acquireSemaphore(ctx, "gemini"); err != nil {
		t.Fatalf("third acquire: %v", err)
	}
	// Channel is now full (cap=2, len=2).
	b.releaseSemaphore("gemini")
	b.releaseSemaphore("gemini")
}

// TestCompositeBackend_BlocksAtCapacity verifies that acquireSemaphore blocks when full.
func TestCompositeBackend_BlocksAtCapacity(t *testing.T) {
	b := newTestComposite()
	if err := b.WithConfigLoader(testLoader(map[string]int{
		"ollama":  1,
		"default": 10,
	})); err != nil {
		t.Fatalf("WithConfigLoader: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	// Fill the single slot.
	if err := b.acquireSemaphore(ctx, "ollama"); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// Second acquire should block; cancel it quickly.
	cancelCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	err := b.acquireSemaphore(cancelCtx, "ollama")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}

	b.releaseSemaphore("ollama")
}

// TestCompositeBackend_ContextCancellation verifies context cancellation is respected.
func TestCompositeBackend_ContextCancellation(t *testing.T) {
	b := newTestComposite()
	if err := b.WithConfigLoader(testLoader(map[string]int{
		"gemini":  1,
		"default": 10,
	})); err != nil {
		t.Fatalf("WithConfigLoader: %v", err)
	}
	defer b.Close()

	// Fill slot.
	ctx := context.Background()
	if err := b.acquireSemaphore(ctx, "gemini"); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // cancel immediately

	err := b.acquireSemaphore(cancelCtx, "gemini")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	b.releaseSemaphore("gemini")
}

// TestCompositeBackend_FallbackToDefault verifies unknown providers use the default semaphore.
func TestCompositeBackend_FallbackToDefault(t *testing.T) {
	b := newTestComposite()
	if err := b.WithConfigLoader(testLoader(map[string]int{
		"default": 3,
	})); err != nil {
		t.Fatalf("WithConfigLoader: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	// "unknown" provider falls back to "default".
	if err := b.acquireSemaphore(ctx, "unknown"); err != nil {
		t.Fatalf("acquire (fallback): %v", err)
	}
	b.releaseSemaphore("unknown")
}

// TestCompositeBackend_MissingDefaultError verifies startup error when default key is absent.
func TestCompositeBackend_MissingDefaultError(t *testing.T) {
	b := newTestComposite()
	err := b.WithConfigLoader(testLoader(map[string]int{
		"gemini": 10,
		// no "default" key
	}))
	if err == nil {
		t.Fatal("expected error for missing default key, got nil")
	}
}

// TestCompositeBackend_NoSemaphoresWhenNilLoader verifies operations pass through without config.
func TestCompositeBackend_NoSemaphoresWhenNilLoader(t *testing.T) {
	b := newTestComposite()
	// No WithConfigLoader call — semaphores should be nil, no blocking.
	ctx := context.Background()
	if err := b.acquireSemaphore(ctx, "gemini"); err != nil {
		t.Fatalf("acquire without config: %v", err)
	}
	b.releaseSemaphore("gemini") // should be no-op
}

// TestCompositeBackend_ChatCompletionWithSemaphore verifies semaphore is acquired/released during ChatCompletion.
func TestCompositeBackend_ChatCompletionWithSemaphore(t *testing.T) {
	var mu sync.Mutex
	maxConcurrent := 0
	current := 0

	stub := &stubBackend{
		chatFn: func(ctx context.Context, messages []Message, opts CompletionOptions) (*CompletionResult, error) {
			mu.Lock()
			current++
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			current--
			mu.Unlock()
			return &CompletionResult{Content: "ok"}, nil
		},
	}
	b := NewCompositeBackend(stub, stub, stub, nil)
	if err := b.WithConfigLoader(testLoader(map[string]int{
		"gemini":  2,
		"default": 5,
	})); err != nil {
		t.Fatalf("WithConfigLoader: %v", err)
	}
	defer b.Close()

	// Launch 5 concurrent gemini completions; semaphore limit is 2.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := b.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "hi"}}, CompletionOptions{Model: "gemini/gemini-flash"})
			if err != nil {
				t.Errorf("ChatCompletion: %v", err)
			}
		}()
	}
	wg.Wait()

	if maxConcurrent > 2 {
		t.Errorf("expected max 2 concurrent gemini calls, got %d", maxConcurrent)
	}
}

// TestCompositeBackend_LoaderError verifies error propagation when loader fails.
func TestCompositeBackend_LoaderError(t *testing.T) {
	b := newTestComposite()
	loaderErr := fmt.Errorf("db unavailable")
	err := b.WithConfigLoader(func() (map[string]int, error) {
		return nil, loaderErr
	})
	if !errors.Is(err, loaderErr) {
		t.Errorf("expected loaderErr, got %v", err)
	}
}
