package router

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// Mock Backend for Testing
// ============================================================================

// mockBackend implements ModelBackend for testing.
type mockBackend struct {
	name         string
	provider     string
	isLocal      bool
	capabilities []string

	mu           sync.Mutex
	healthy      bool
	processFunc  func(ctx context.Context, req *Request) (*Response, error)
	processCount int64
	processDelay time.Duration
	failCount    int
	failAfter    int
	blockChan    chan struct{} // blocks Process until closed
}

func newMockBackend(name, provider string, isLocal bool) *mockBackend {
	return &mockBackend{
		name:         name,
		provider:     provider,
		isLocal:      isLocal,
		capabilities: []string{"embedding", "chat"},
		healthy:      true,
	}
}

func (m *mockBackend) Name() string     { return m.name }
func (m *mockBackend) Provider() string { return m.provider }
func (m *mockBackend) IsLocal() bool    { return m.isLocal }
func (m *mockBackend) Capabilities() []string {
	return m.capabilities
}

func (m *mockBackend) HealthCheck(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.healthy {
		return errors.New("backend unhealthy")
	}
	return nil
}

func (m *mockBackend) Process(ctx context.Context, req *Request) (*Response, error) {
	atomic.AddInt64(&m.processCount, 1)

	m.mu.Lock()
	delay := m.processDelay
	failAfter := m.failAfter
	failCount := m.failCount
	fn := m.processFunc
	blockChan := m.blockChan
	m.mu.Unlock()

	// Wait on block channel if set (used for queue full testing)
	if blockChan != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-blockChan:
			// Unblocked, continue processing
		}
	}

	// Check if we should fail
	if failAfter > 0 {
		count := atomic.LoadInt64(&m.processCount)
		if count > int64(failAfter) && failCount > 0 {
			m.mu.Lock()
			m.failCount--
			m.mu.Unlock()
			return nil, errors.New("simulated failure")
		}
	}

	// Apply delay if configured
	if delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	// Use custom function if provided
	if fn != nil {
		return fn(ctx, req)
	}

	return &Response{
		RequestID:  req.ID,
		ModelUsed:  m.name,
		Data:       "mock response",
		TokensUsed: 100,
	}, nil
}

func (m *mockBackend) SetHealthy(healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = healthy
}

func (m *mockBackend) SetProcessDelay(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processDelay = d
}

func (m *mockBackend) SetFailAfter(after, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failAfter = after
	m.failCount = count
}

func (m *mockBackend) SetBlockChan(ch chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blockChan = ch
}

func (m *mockBackend) GetProcessCount() int64 {
	return atomic.LoadInt64(&m.processCount)
}

// ============================================================================
// Circuit Breaker Tests
// ============================================================================

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker("test", nil)

	if cb.State() != CircuitClosed {
		t.Errorf("Expected initial state to be closed, got %s", cb.State())
	}

	if cb.Name() != "test" {
		t.Errorf("Expected name to be 'test', got %s", cb.Name())
	}
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	var stateChanges []struct{ from, to CircuitState }
	var changesMu sync.Mutex

	cfg := &CircuitBreakerConfig{
		FailureThreshold:    3,
		SuccessThreshold:    2,
		ResetTimeout:        50 * time.Millisecond,
		HalfOpenMaxRequests: 1,
		OnStateChange: func(name string, from, to CircuitState) {
			changesMu.Lock()
			stateChanges = append(stateChanges, struct{ from, to CircuitState }{from, to})
			changesMu.Unlock()
		},
	}

	cb := NewCircuitBreaker("test", cfg)

	// Record failures to open the circuit
	for i := 0; i < 3; i++ {
		if err := cb.Allow(); err != nil {
			t.Fatalf("Allow failed before threshold: %v", err)
		}
		cb.RecordFailure()
	}

	// Circuit should be open now
	if cb.State() != CircuitOpen {
		t.Errorf("Expected circuit to be open, got %s", cb.State())
	}

	// Requests should be rejected
	if err := cb.Allow(); err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open
	if err := cb.Allow(); err != nil {
		t.Fatalf("Allow failed after reset timeout: %v", err)
	}

	if cb.State() != CircuitHalfOpen {
		t.Errorf("Expected circuit to be half-open, got %s", cb.State())
	}

	// Additional requests should be rejected in half-open
	if err := cb.Allow(); err != ErrCircuitHalfOpenBusy {
		t.Errorf("Expected ErrCircuitHalfOpenBusy, got %v", err)
	}

	// Record success
	cb.RecordSuccess()

	// Still half-open (need 2 successes)
	if cb.State() != CircuitHalfOpen {
		t.Errorf("Expected circuit still half-open, got %s", cb.State())
	}

	// Allow another request and succeed
	if err := cb.Allow(); err != nil {
		t.Fatalf("Allow failed in half-open: %v", err)
	}
	cb.RecordSuccess()

	// Should be closed now
	if cb.State() != CircuitClosed {
		t.Errorf("Expected circuit to be closed, got %s", cb.State())
	}

	// Verify state changes were tracked
	time.Sleep(10 * time.Millisecond) // Wait for async callback
	changesMu.Lock()
	if len(stateChanges) < 3 {
		t.Errorf("Expected at least 3 state changes, got %d", len(stateChanges))
	}
	changesMu.Unlock()
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cfg := &CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     20 * time.Millisecond,
	}

	cb := NewCircuitBreaker("test", cfg)

	// Open the circuit
	cb.Allow()
	cb.RecordFailure()
	cb.Allow()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("Expected circuit open, got %s", cb.State())
	}

	// Wait for reset
	time.Sleep(30 * time.Millisecond)

	// Transition to half-open
	cb.Allow()
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("Expected circuit half-open, got %s", cb.State())
	}

	// Fail in half-open - should reopen immediately
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Errorf("Expected circuit to reopen, got %s", cb.State())
	}
}

func TestCircuitBreaker_Execute(t *testing.T) {
	cb := NewCircuitBreaker("test", nil)

	// Successful execution
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	// Failed execution
	expectedErr := errors.New("test error")
	err = cb.Execute(context.Background(), func(ctx context.Context) error {
		return expectedErr
	})
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	stats := cb.Stats()
	if stats.TotalSuccesses != 1 {
		t.Errorf("Expected 1 success, got %d", stats.TotalSuccesses)
	}
	if stats.TotalFailures != 1 {
		t.Errorf("Expected 1 failure, got %d", stats.TotalFailures)
	}
}

func TestCircuitBreaker_ExecuteWithFallback(t *testing.T) {
	cfg := &CircuitBreakerConfig{
		FailureThreshold: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	fallbackCalled := false
	fallback := func(ctx context.Context) error {
		fallbackCalled = true
		return nil
	}

	// Primary fails, fallback succeeds
	err := cb.ExecuteWithFallback(
		context.Background(),
		func(ctx context.Context) error {
			return errors.New("primary failed")
		},
		fallback,
	)

	if err != nil {
		t.Errorf("Expected fallback to succeed, got error: %v", err)
	}
	if !fallbackCalled {
		t.Error("Expected fallback to be called")
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cfg := &CircuitBreakerConfig{
		FailureThreshold: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	// Open the circuit
	cb.Allow()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("Expected circuit open, got %s", cb.State())
	}

	// Reset
	cb.Reset()

	if cb.State() != CircuitClosed {
		t.Errorf("Expected circuit closed after reset, got %s", cb.State())
	}

	// Should allow requests again
	if err := cb.Allow(); err != nil {
		t.Errorf("Expected Allow to succeed after reset: %v", err)
	}
}

func TestCircuitBreakerManager(t *testing.T) {
	cfg := &CircuitBreakerConfig{
		FailureThreshold: 3,
	}
	manager := NewCircuitBreakerManager(cfg)

	// Get creates new breaker
	cb1 := manager.Get("backend1")
	if cb1 == nil {
		t.Fatal("Expected breaker to be created")
	}

	// Get returns same breaker
	cb2 := manager.Get("backend1")
	if cb1 != cb2 {
		t.Error("Expected same breaker instance")
	}

	// GetExisting returns nil for unknown
	cb3 := manager.GetExisting("unknown")
	if cb3 != nil {
		t.Error("Expected nil for unknown breaker")
	}

	// Create another breaker
	manager.Get("backend2")

	// All returns all breakers
	all := manager.All()
	if len(all) != 2 {
		t.Errorf("Expected 2 breakers, got %d", len(all))
	}

	// Open circuit on backend1
	cb1.Allow()
	cb1.RecordFailure()
	cb1.Allow()
	cb1.RecordFailure()
	cb1.Allow()
	cb1.RecordFailure()

	if cb1.State() != CircuitOpen {
		t.Fatalf("Expected circuit open, got %s", cb1.State())
	}

	// ResetAll resets all breakers
	manager.ResetAll()

	if cb1.State() != CircuitClosed {
		t.Errorf("Expected circuit closed after ResetAll, got %s", cb1.State())
	}
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("CircuitState(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}

// ============================================================================
// Router Tests
// ============================================================================

func TestModelRouter_RegisterBackend(t *testing.T) {
	cfg := &RouterConfig{
		DefaultTimeout:       5 * time.Second,
		CircuitBreakerConfig: DefaultCircuitBreakerConfig(),
		EnableMetrics:        false, // Disable for tests
	}
	router := NewModelRouter(cfg)
	defer router.Shutdown(context.Background())

	backend := newMockBackend("ollama", "ollama", true)

	// Register should succeed
	err := router.RegisterBackend(backend)
	if err != nil {
		t.Fatalf("RegisterBackend failed: %v", err)
	}

	// List should include the backend
	backends := router.ListBackends()
	if len(backends) != 1 || backends[0] != "ollama" {
		t.Errorf("Expected ['ollama'], got %v", backends)
	}

	// Get should return the backend
	got, ok := router.GetBackend("ollama")
	if !ok || got != backend {
		t.Error("GetBackend failed to return registered backend")
	}

	// Duplicate registration should fail
	err = router.RegisterBackend(backend)
	if err == nil {
		t.Error("Expected error for duplicate registration")
	}

	// Nil backend should fail
	err = router.RegisterBackend(nil)
	if err == nil {
		t.Error("Expected error for nil backend")
	}
}

func TestModelRouter_UnregisterBackend(t *testing.T) {
	router := NewModelRouter(&RouterConfig{EnableMetrics: false})
	defer router.Shutdown(context.Background())

	backend := newMockBackend("ollama", "ollama", true)
	router.RegisterBackend(backend)

	// Unregister should succeed
	err := router.UnregisterBackend("ollama")
	if err != nil {
		t.Fatalf("UnregisterBackend failed: %v", err)
	}

	// Should no longer be listed
	backends := router.ListBackends()
	if len(backends) != 0 {
		t.Errorf("Expected empty list, got %v", backends)
	}

	// Unregistering unknown should fail
	err = router.UnregisterBackend("unknown")
	if err == nil {
		t.Error("Expected error for unknown backend")
	}
}

func TestModelRouter_Route_SingleBackend(t *testing.T) {
	router := NewModelRouter(&RouterConfig{
		DefaultTimeout: 5 * time.Second,
		EnableMetrics:  false,
	})
	defer router.Shutdown(context.Background())

	backend := newMockBackend("ollama", "ollama", true)
	router.RegisterBackend(backend)

	req := &Request{
		ID:      "test-1",
		Type:    RequestTypeEmbedding,
		Content: "test content",
	}

	resp, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if resp.ModelUsed != "ollama" {
		t.Errorf("Expected ModelUsed 'ollama', got '%s'", resp.ModelUsed)
	}
	if resp.RequestID != "test-1" {
		t.Errorf("Expected RequestID 'test-1', got '%s'", resp.RequestID)
	}
}

func TestModelRouter_Route_NoBackends(t *testing.T) {
	router := NewModelRouter(&RouterConfig{EnableMetrics: false})
	defer router.Shutdown(context.Background())

	req := &Request{
		ID:      "test-1",
		Type:    RequestTypeEmbedding,
		Content: "test content",
	}

	_, err := router.Route(context.Background(), req)
	if err == nil {
		t.Error("Expected error when no backends available")
	}
}

func TestModelRouter_Route_NilRequest(t *testing.T) {
	router := NewModelRouter(&RouterConfig{EnableMetrics: false})
	defer router.Shutdown(context.Background())

	_, err := router.Route(context.Background(), nil)
	if err == nil {
		t.Error("Expected error for nil request")
	}
}

func TestModelRouter_Route_SpecificModel(t *testing.T) {
	router := NewModelRouter(&RouterConfig{EnableMetrics: false})
	defer router.Shutdown(context.Background())

	ollama := newMockBackend("ollama", "ollama", true)
	gemini := newMockBackend("gemini", "google", false)

	router.RegisterBackend(ollama)
	router.RegisterBackend(gemini)

	// Request specific model
	req := &Request{
		ID:      "test-1",
		Type:    RequestTypeEmbedding,
		Model:   "gemini",
		Content: "test content",
	}

	resp, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if resp.ModelUsed != "gemini" {
		t.Errorf("Expected ModelUsed 'gemini', got '%s'", resp.ModelUsed)
	}
}

func TestModelRouter_Route_Timeout(t *testing.T) {
	router := NewModelRouter(&RouterConfig{
		DefaultTimeout: 50 * time.Millisecond,
		EnableMetrics:  false,
	})
	defer router.Shutdown(context.Background())

	backend := newMockBackend("slow", "test", true)
	backend.SetProcessDelay(200 * time.Millisecond)
	router.RegisterBackend(backend)

	req := &Request{
		ID:      "test-1",
		Type:    RequestTypeEmbedding,
		Content: "test content",
	}

	_, err := router.Route(context.Background(), req)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestModelRouter_Route_CircuitBreaker(t *testing.T) {
	cfg := &RouterConfig{
		DefaultTimeout: 5 * time.Second,
		CircuitBreakerConfig: &CircuitBreakerConfig{
			FailureThreshold: 2,
			ResetTimeout:     100 * time.Millisecond,
		},
		EnableMetrics: false,
	}
	router := NewModelRouter(cfg)
	defer router.Shutdown(context.Background())

	backend := newMockBackend("flaky", "test", true)
	backend.processFunc = func(ctx context.Context, req *Request) (*Response, error) {
		return nil, errors.New("backend error")
	}
	router.RegisterBackend(backend)

	req := &Request{
		ID:      "test",
		Type:    RequestTypeEmbedding,
		Content: "test",
	}

	// Fail twice to trip the circuit
	router.Route(context.Background(), req)
	router.Route(context.Background(), req)

	// Circuit should be open - check stats
	stats := router.GetCircuitStats("flaky")
	if stats == nil {
		t.Fatal("Expected stats to be available")
	}
	if stats.State != CircuitOpen {
		t.Errorf("Expected circuit open, got %s", stats.State.String())
	}

	// Third request should fail fast due to circuit
	start := time.Now()
	_, err := router.Route(context.Background(), req)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected error when circuit is open")
	}

	// Should be fast (not waiting for backend)
	if duration > 50*time.Millisecond {
		t.Errorf("Expected fast failure, took %v", duration)
	}
}

func TestModelRouter_Route_Fallback(t *testing.T) {
	cfg := &RouterConfig{
		DefaultTimeout: 5 * time.Second,
		CircuitBreakerConfig: &CircuitBreakerConfig{
			FailureThreshold: 1,
		},
		FallbackOrder: []string{"primary", "fallback"},
		EnableMetrics: false,
	}
	router := NewModelRouter(cfg)
	defer router.Shutdown(context.Background())

	primary := newMockBackend("primary", "test", true)
	primary.processFunc = func(ctx context.Context, req *Request) (*Response, error) {
		return nil, errors.New("primary failed")
	}

	fallback := newMockBackend("fallback", "test", true)

	router.RegisterBackend(primary)
	router.RegisterBackend(fallback)

	req := &Request{
		ID:      "test",
		Type:    RequestTypeEmbedding,
		Model:   "primary",
		Content: "test",
	}

	resp, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("Expected fallback to succeed, got error: %v", err)
	}

	if resp.ModelUsed != "fallback" {
		t.Errorf("Expected ModelUsed 'fallback', got '%s'", resp.ModelUsed)
	}
}

func TestModelRouter_Route_AllFallbacksFail(t *testing.T) {
	cfg := &RouterConfig{
		DefaultTimeout: 5 * time.Second,
		CircuitBreakerConfig: &CircuitBreakerConfig{
			FailureThreshold: 1,
		},
		FallbackOrder: []string{"primary", "fallback"},
		EnableMetrics: false,
	}
	router := NewModelRouter(cfg)
	defer router.Shutdown(context.Background())

	primary := newMockBackend("primary", "test", true)
	primary.processFunc = func(ctx context.Context, req *Request) (*Response, error) {
		return nil, errors.New("primary failed")
	}

	fallback := newMockBackend("fallback", "test", true)
	fallback.processFunc = func(ctx context.Context, req *Request) (*Response, error) {
		return nil, errors.New("fallback also failed")
	}

	router.RegisterBackend(primary)
	router.RegisterBackend(fallback)

	req := &Request{
		ID:      "test",
		Type:    RequestTypeEmbedding,
		Model:   "primary",
		Content: "test",
	}

	_, err := router.Route(context.Background(), req)
	if err == nil {
		t.Error("Expected error when all backends fail")
	}
}

func TestModelRouter_HealthChecks(t *testing.T) {
	cfg := &RouterConfig{
		HealthCheckInterval: 50 * time.Millisecond,
		EnableMetrics:       false,
	}
	router := NewModelRouter(cfg)

	backend := newMockBackend("test", "test", true)
	router.RegisterBackend(backend)

	router.StartHealthChecks()
	defer router.Shutdown(context.Background())

	// Wait for initial check
	time.Sleep(20 * time.Millisecond)

	health, lastCheck := router.GetHealth("test")
	if !health {
		t.Error("Expected backend to be healthy")
	}
	if lastCheck.IsZero() {
		t.Error("Expected lastCheck to be set")
	}

	// Make backend unhealthy
	backend.SetHealthy(false)

	// Wait for health check
	time.Sleep(70 * time.Millisecond)

	health, _ = router.GetHealth("test")
	if health {
		t.Error("Expected backend to be unhealthy")
	}

	// Check GetAllHealth
	allHealth := router.GetAllHealth()
	if len(allHealth) != 1 {
		t.Errorf("Expected 1 backend, got %d", len(allHealth))
	}
	if allHealth["test"] {
		t.Error("Expected test backend to be unhealthy")
	}
}

func TestModelRouter_QueueRequest(t *testing.T) {
	cfg := &RouterConfig{
		DefaultTimeout: 5 * time.Second,
		MaxQueueSize:   10,
		EnableMetrics:  false,
	}
	router := NewModelRouter(cfg)
	defer router.Shutdown(context.Background())

	backend := newMockBackend("test", "test", true)
	backend.SetProcessDelay(50 * time.Millisecond)
	router.RegisterBackend(backend)

	req := &Request{
		ID:      "queued-1",
		Type:    RequestTypeEmbedding,
		Content: "test",
		Timeout: 5 * time.Second,
	}

	respChan, errChan, err := router.QueueRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("QueueRequest failed: %v", err)
	}

	// Wait for response
	select {
	case resp := <-respChan:
		if resp == nil {
			t.Error("Expected response")
		}
	case err := <-errChan:
		t.Errorf("Unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for response")
	}
}

func TestModelRouter_QueueFull(t *testing.T) {
	cfg := &RouterConfig{
		MaxQueueSize:  2,
		EnableMetrics: false,
	}
	router := NewModelRouter(cfg)
	defer router.Shutdown(context.Background())

	// Create a blocking channel that prevents the processor from completing
	blockChan := make(chan struct{})
	backend := newMockBackend("test", "test", true)
	backend.SetBlockChan(blockChan)
	router.RegisterBackend(backend)

	// Fill the queue - these will block in the processor
	for i := 0; i < 2; i++ {
		req := &Request{
			ID:      "queued",
			Type:    RequestTypeEmbedding,
			Content: "test",
			Timeout: 5 * time.Second,
		}
		_, _, err := router.QueueRequest(context.Background(), req)
		if err != nil {
			t.Fatalf("QueueRequest %d failed: %v", i, err)
		}
	}

	// Give the processor time to pick up items from the channel
	// (items will be blocked in Process, so counter stays at 2)
	time.Sleep(50 * time.Millisecond)

	// Next should fail because queue counter is still at 2
	req := &Request{
		ID:      "overflow",
		Type:    RequestTypeEmbedding,
		Content: "test",
		Timeout: 5 * time.Second,
	}
	_, _, err := router.QueueRequest(context.Background(), req)
	if err == nil {
		t.Error("Expected error when queue is full")
	}

	// Unblock processors to allow clean shutdown
	close(blockChan)
}

func TestModelRouter_Shutdown(t *testing.T) {
	router := NewModelRouter(&RouterConfig{EnableMetrics: false})

	backend := newMockBackend("test", "test", true)
	router.RegisterBackend(backend)
	router.StartHealthChecks()

	// Shutdown should succeed
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := router.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Routing should fail after shutdown
	req := &Request{
		ID:      "post-shutdown",
		Type:    RequestTypeEmbedding,
		Content: "test",
	}
	_, err = router.Route(context.Background(), req)
	if err == nil {
		t.Error("Expected error after shutdown")
	}
}

func TestModelRouter_Selector(t *testing.T) {
	router := NewModelRouter(&RouterConfig{EnableMetrics: false})
	defer router.Shutdown(context.Background())

	ollama := newMockBackend("ollama", "ollama", true)
	gemini := newMockBackend("gemini", "google", false)

	router.RegisterBackend(ollama)
	router.RegisterBackend(gemini)

	// Set custom selector
	selector := &DefaultModelSelector{
		ModelMappings: map[RequestType]string{
			RequestTypeEmbedding: "gemini",
			RequestTypeSummary:   "ollama",
		},
	}
	router.SetSelector(selector)

	// Embedding should go to gemini
	req := &Request{
		ID:      "test-1",
		Type:    RequestTypeEmbedding,
		Content: "test",
	}
	resp, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if resp.ModelUsed != "gemini" {
		t.Errorf("Expected gemini for embedding, got %s", resp.ModelUsed)
	}

	// Summary should go to ollama
	req.Type = RequestTypeSummary
	resp, err = router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if resp.ModelUsed != "ollama" {
		t.Errorf("Expected ollama for summary, got %s", resp.ModelUsed)
	}
}

func TestModelRouter_LoadBalancing(t *testing.T) {
	router := NewModelRouter(&RouterConfig{EnableMetrics: false})
	defer router.Shutdown(context.Background())

	backend1 := newMockBackend("backend1", "test", true)
	backend2 := newMockBackend("backend2", "test", true)

	router.RegisterBackend(backend1)
	router.RegisterBackend(backend2)

	// Send multiple requests
	for i := 0; i < 10; i++ {
		req := &Request{
			ID:      "test",
			Type:    RequestTypeEmbedding,
			Content: "test",
		}
		router.Route(context.Background(), req)
	}

	// Both backends should have received requests
	count1 := backend1.GetProcessCount()
	count2 := backend2.GetProcessCount()

	if count1 == 0 || count2 == 0 {
		t.Errorf("Expected both backends to receive requests: backend1=%d, backend2=%d", count1, count2)
	}

	// Should be roughly balanced (within 3 of each other for 10 requests)
	diff := count1 - count2
	if diff < 0 {
		diff = -diff
	}
	if diff > 3 {
		t.Errorf("Load not balanced: backend1=%d, backend2=%d", count1, count2)
	}
}

// ============================================================================
// Default Model Selector Tests
// ============================================================================

func TestDefaultModelSelector_NoBackends(t *testing.T) {
	selector := &DefaultModelSelector{}

	_, err := selector.Select(context.Background(), &Request{}, []string{})
	if err == nil {
		t.Error("Expected error when no backends available")
	}
}

func TestDefaultModelSelector_WithMapping(t *testing.T) {
	selector := &DefaultModelSelector{
		ModelMappings: map[RequestType]string{
			RequestTypeEmbedding: "embedding-model",
		},
	}

	req := &Request{Type: RequestTypeEmbedding}
	available := []string{"other", "embedding-model", "another"}

	selected, err := selector.Select(context.Background(), req, available)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if selected != "embedding-model" {
		t.Errorf("Expected 'embedding-model', got '%s'", selected)
	}
}

func TestDefaultModelSelector_MappingNotAvailable(t *testing.T) {
	selector := &DefaultModelSelector{
		ModelMappings: map[RequestType]string{
			RequestTypeEmbedding: "unavailable",
		},
	}

	req := &Request{Type: RequestTypeEmbedding}
	available := []string{"other", "another"}

	selected, err := selector.Select(context.Background(), req, available)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	// Should fall back to first available
	if selected != "other" {
		t.Errorf("Expected 'other', got '%s'", selected)
	}
}

// ============================================================================
// Concurrent Tests
// ============================================================================

func TestCircuitBreaker_Concurrent(t *testing.T) {
	cb := NewCircuitBreaker("concurrent", &CircuitBreakerConfig{
		FailureThreshold: 100,
	})

	var wg sync.WaitGroup
	goroutines := 100
	iterations := 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if err := cb.Allow(); err != nil {
					continue
				}
				if j%2 == 0 {
					cb.RecordSuccess()
				} else {
					cb.RecordFailure()
				}
			}
		}()
	}

	wg.Wait()

	stats := cb.Stats()
	expectedTotal := int64(goroutines * iterations)
	if stats.TotalRequests != expectedTotal {
		t.Errorf("Expected %d total requests, got %d", expectedTotal, stats.TotalRequests)
	}
}

func TestModelRouter_Concurrent(t *testing.T) {
	router := NewModelRouter(&RouterConfig{
		DefaultTimeout: 5 * time.Second,
		MaxQueueSize:   1000,
		EnableMetrics:  false,
	})
	defer router.Shutdown(context.Background())

	backend := newMockBackend("test", "test", true)
	router.RegisterBackend(backend)

	var wg sync.WaitGroup
	goroutines := 50
	iterations := 20

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				req := &Request{
					ID:      "test",
					Type:    RequestTypeEmbedding,
					Content: "test",
				}
				router.Route(context.Background(), req)
			}
		}(i)
	}

	wg.Wait()

	expectedCount := int64(goroutines * iterations)
	actualCount := backend.GetProcessCount()
	if actualCount != expectedCount {
		t.Errorf("Expected %d requests, processed %d", expectedCount, actualCount)
	}
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkCircuitBreaker_Allow(b *testing.B) {
	cb := NewCircuitBreaker("bench", nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkModelRouter_Route(b *testing.B) {
	router := NewModelRouter(&RouterConfig{EnableMetrics: false})
	defer router.Shutdown(context.Background())

	backend := newMockBackend("bench", "test", true)
	router.RegisterBackend(backend)

	req := &Request{
		ID:      "bench",
		Type:    RequestTypeEmbedding,
		Content: "benchmark content",
	}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Route(ctx, req)
	}
}

func BenchmarkModelRouter_Route_Parallel(b *testing.B) {
	router := NewModelRouter(&RouterConfig{EnableMetrics: false})
	defer router.Shutdown(context.Background())

	backend := newMockBackend("bench", "test", true)
	router.RegisterBackend(backend)

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		req := &Request{
			ID:      "bench",
			Type:    RequestTypeEmbedding,
			Content: "benchmark content",
		}
		for pb.Next() {
			router.Route(ctx, req)
		}
	})
}
