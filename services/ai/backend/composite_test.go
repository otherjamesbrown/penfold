package backend_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/services/ai/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowBackend implements backend.Backend, delaying ChatCompletion to simulate load.
type slowBackend struct {
	delay          time.Duration
	called         atomic.Int64
	peakConcurrent atomic.Int64 // peak concurrent calls to ChatCompletion
	inflight       atomic.Int64 // current in-flight calls
}

func (s *slowBackend) GenerateEmbedding(_ context.Context, _ string, _ string) (*backend.EmbeddingResult, error) {
	return &backend.EmbeddingResult{}, nil
}

func (s *slowBackend) ChatCompletion(ctx context.Context, _ []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
	s.called.Add(1)
	current := s.inflight.Add(1)
	defer s.inflight.Add(-1)
	// Track peak.
	for {
		peak := s.peakConcurrent.Load()
		if current <= peak || s.peakConcurrent.CompareAndSwap(peak, current) {
			break
		}
	}
	select {
	case <-time.After(s.delay):
		return &backend.CompletionResult{
			Content: "response",
			Model:   opts.Model,
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *slowBackend) CheckEmbeddingsHealth(_ context.Context) error { return nil }
func (s *slowBackend) CheckLLMHealth(_ context.Context) error        { return nil }
func (s *slowBackend) Close() error                                  { return nil }

// instantBackend returns immediately.
type instantBackend struct {
	provider string
	err      error
}

func (i *instantBackend) GenerateEmbedding(_ context.Context, _ string, _ string) (*backend.EmbeddingResult, error) {
	return &backend.EmbeddingResult{}, nil
}

func (i *instantBackend) ChatCompletion(_ context.Context, _ []backend.Message, opts backend.CompletionOptions) (*backend.CompletionResult, error) {
	if i.err != nil {
		return nil, i.err
	}
	return &backend.CompletionResult{Content: "ok", Model: opts.Model}, nil
}

func (i *instantBackend) CheckEmbeddingsHealth(_ context.Context) error { return nil }
func (i *instantBackend) CheckLLMHealth(_ context.Context) error        { return nil }
func (i *instantBackend) Close() error                                  { return nil }

func newTestComposite() *backend.CompositeBackend {
	emb := &instantBackend{}
	ollama := &instantBackend{}
	gemini := &instantBackend{}
	return backend.NewCompositeBackend(emb, ollama, gemini, nil)
}

// =============================================================================
// Semaphore metadata tests
// =============================================================================

func TestSemaphoreMetadata_WhenNoSemaphore(t *testing.T) {
	cb := newTestComposite()
	// No semaphores configured — result should have zero values.
	result, err := cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{
		Model: "gemini/gemini-2.0-flash",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), result.SemaphoreWaitMs)
	assert.Equal(t, 0, result.BackendConcurrent)
	assert.Equal(t, 0, result.BackendMax)
}

func TestSemaphoreMetadata_WithSemaphore(t *testing.T) {
	cb := newTestComposite()
	cb.SetSemaphores(map[string]int{"gemini": 5})

	result, err := cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{
		Model: "gemini/gemini-2.0-flash",
	})
	require.NoError(t, err)
	// Slot was available immediately so BackendConcurrent should be 1.
	assert.Equal(t, 1, result.BackendConcurrent)
	assert.Equal(t, 5, result.BackendMax)
	// Wait time should be near zero (slot was available immediately).
	assert.GreaterOrEqual(t, result.SemaphoreWaitMs, int64(0))
}

func TestSemaphoreMetadata_IndependentPerProvider(t *testing.T) {
	cb := newTestComposite()
	cb.SetSemaphores(map[string]int{
		"gemini": 3,
		"ollama": 2,
	})

	// Gemini request gets gemini metadata.
	geminiResult, err := cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{
		Model: "gemini/gemini-2.0-flash",
	})
	require.NoError(t, err)
	assert.Equal(t, 3, geminiResult.BackendMax)

	// Ollama request gets ollama metadata.
	ollamaResult, err := cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{
		Model: "ollama/qwen3:8b",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, ollamaResult.BackendMax)
}

// =============================================================================
// Concurrency limiting tests
// =============================================================================

func TestSemaphore_LimitsConcurrentOllamaRequests(t *testing.T) {
	const maxConcurrent = 3
	const totalRequests = 5
	const callDelay = 50 * time.Millisecond

	ollamaSlow := &slowBackend{delay: callDelay}
	geminiInstant := &instantBackend{}
	cb := backend.NewCompositeBackend(geminiInstant, ollamaSlow, geminiInstant, nil)
	cb.SetSemaphores(map[string]int{"ollama": maxConcurrent})

	var wg sync.WaitGroup
	for range totalRequests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{
				Model: "ollama/qwen3:8b",
			})
			require.NoError(t, err)
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(totalRequests), ollamaSlow.called.Load(), "all requests should complete")
	// Peak concurrent backend calls must not exceed the semaphore limit.
	assert.LessOrEqual(t, ollamaSlow.peakConcurrent.Load(), int64(maxConcurrent))
}

func TestSemaphore_FourthOllamaRequestBlocks(t *testing.T) {
	const callDelay = 80 * time.Millisecond

	ollamaSlow := &slowBackend{delay: callDelay}
	geminiInstant := &instantBackend{}
	cb := backend.NewCompositeBackend(geminiInstant, ollamaSlow, geminiInstant, nil)
	cb.SetSemaphores(map[string]int{"ollama": 3})

	// Fire 3 requests that hold the semaphore for callDelay.
	var holdersReady sync.WaitGroup
	for range 3 {
		holdersReady.Add(1)
		go func() {
			holdersReady.Done() // signal that this goroutine is about to acquire
			//nolint:errcheck
			cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{Model: "ollama/qwen3:8b"}) //nolint:errcheck
		}()
	}
	// Give the holders a moment to acquire their slots.
	time.Sleep(10 * time.Millisecond)

	// The 4th request should block until one holder finishes.
	start := time.Now()
	_, err := cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{
		Model: "ollama/qwen3:8b",
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	// The 4th request must have waited at least some portion of callDelay.
	assert.Greater(t, elapsed, callDelay/2, "4th request should have waited for a semaphore slot")
}

func TestSemaphore_GeminiNotBlockedByOllamaSemaphore(t *testing.T) {
	const callDelay = 100 * time.Millisecond

	ollamaSlow := &slowBackend{delay: callDelay}
	geminiInstant := &instantBackend{}
	cb := backend.NewCompositeBackend(geminiInstant, ollamaSlow, geminiInstant, nil)
	// Only ollama has a semaphore with max=1 (fully saturated immediately).
	cb.SetSemaphores(map[string]int{"ollama": 1})

	// Saturate the ollama semaphore.
	go func() {
		//nolint:errcheck
		cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{Model: "ollama/qwen3:8b"})
	}()
	time.Sleep(10 * time.Millisecond) // allow the holder to acquire

	// Gemini request should complete immediately (independent semaphore).
	start := time.Now()
	result, err := cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{
		Model: "gemini/gemini-2.0-flash",
	})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Less(t, elapsed, callDelay/2, "gemini should not be blocked by ollama semaphore")
	// Gemini has no semaphore configured → metadata should be zero.
	assert.Equal(t, 0, result.BackendMax)
}

func TestSemaphore_ContextCancellationUnblocks(t *testing.T) {
	ollamaSlow := &slowBackend{delay: 5 * time.Second}
	geminiInstant := &instantBackend{}
	cb := backend.NewCompositeBackend(geminiInstant, ollamaSlow, geminiInstant, nil)
	cb.SetSemaphores(map[string]int{"ollama": 1})

	// Saturate the semaphore.
	go func() {
		//nolint:errcheck
		cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{Model: "ollama/qwen3:8b"})
	}()
	time.Sleep(10 * time.Millisecond)

	// Second request with a short-lived context should fail quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := cb.ChatCompletion(ctx, nil, backend.CompletionOptions{
		Model: "ollama/qwen3:8b",
	})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"expected context error, got: %v", err)
	assert.Less(t, elapsed, 200*time.Millisecond, "should fail fast on context cancellation")
}

// =============================================================================
// SetSemaphores edge cases
// =============================================================================

func TestSetSemaphores_ZeroMaxIgnored(t *testing.T) {
	cb := newTestComposite()
	cb.SetSemaphores(map[string]int{"gemini": 0, "ollama": 3})

	// Gemini has no effective semaphore (max=0 ignored).
	result, err := cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{
		Model: "gemini/gemini-2.0-flash",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.BackendMax, "semaphore with max=0 should be skipped")

	// Ollama has an effective semaphore.
	result2, err := cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{
		Model: "ollama/qwen3:8b",
	})
	require.NoError(t, err)
	assert.Equal(t, 3, result2.BackendMax)
}

func TestSetSemaphores_BackwardCompatModel(t *testing.T) {
	cb := newTestComposite()
	cb.SetSemaphores(map[string]int{"gemini": 2})

	// Model without explicit "gemini/" prefix but containing "gemini" in name.
	result, err := cb.ChatCompletion(context.Background(), nil, backend.CompletionOptions{
		Model: "gemini-2.0-flash", // no provider prefix
	})
	require.NoError(t, err)
	// Should use gemini semaphore via backward-compat resolution.
	assert.Equal(t, 2, result.BackendMax)
}
