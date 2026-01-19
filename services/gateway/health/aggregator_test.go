package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pkghealth "github.com/otherjamesbrown/penfold/pkg/health"
)

// mockServiceClient implements ServiceClient for testing.
type mockServiceClient struct {
	err      error
	delay    time.Duration
	callsMu  sync.Mutex
	calls    int
	response chan struct{}
}

func (m *mockServiceClient) HealthCheck(ctx context.Context) error {
	m.callsMu.Lock()
	m.calls++
	m.callsMu.Unlock()

	if m.response != nil {
		m.response <- struct{}{}
	}

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return m.err
}

func (m *mockServiceClient) getCalls() int {
	m.callsMu.Lock()
	defer m.callsMu.Unlock()
	return m.calls
}

func TestNewAggregator(t *testing.T) {
	aggregator := NewAggregator("1.0.0")
	if aggregator == nil {
		t.Fatal("NewAggregator returned nil")
	}
	if aggregator.version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", aggregator.version)
	}
	if aggregator.services == nil {
		t.Fatal("services map is nil")
	}
}

func TestAggregator_RegisterService(t *testing.T) {
	aggregator := NewAggregator("1.0.0")
	client := &mockServiceClient{}

	aggregator.RegisterService(ServiceConfig{
		Name:     "test-service",
		Client:   client,
		Critical: true,
	})

	aggregator.mu.RLock()
	defer aggregator.mu.RUnlock()

	if _, ok := aggregator.services["test-service"]; !ok {
		t.Error("service was not registered")
	}
}

func TestAggregator_UnregisterService(t *testing.T) {
	aggregator := NewAggregator("1.0.0")
	client := &mockServiceClient{}

	aggregator.RegisterService(ServiceConfig{
		Name:   "test-service",
		Client: client,
	})

	aggregator.UnregisterService("test-service")

	aggregator.mu.RLock()
	defer aggregator.mu.RUnlock()

	if _, ok := aggregator.services["test-service"]; ok {
		t.Error("service was not unregistered")
	}
}

func TestAggregator_CheckAll_AllHealthy(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	aggregator.RegisterService(ServiceConfig{
		Name:     "service1",
		Client:   &mockServiceClient{},
		Critical: true,
	})
	aggregator.RegisterService(ServiceConfig{
		Name:     "service2",
		Client:   &mockServiceClient{},
		Critical: false,
	})

	ctx := context.Background()
	health := aggregator.CheckAll(ctx)

	if health.Status != pkghealth.StatusHealthy {
		t.Errorf("expected status %s, got %s", pkghealth.StatusHealthy, health.Status)
	}

	if len(health.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(health.Services))
	}

	for _, svc := range health.Services {
		if svc.Status != pkghealth.StatusHealthy {
			t.Errorf("service %s: expected healthy status", svc.Name)
		}
	}
}

func TestAggregator_CheckAll_CriticalFailure(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	aggregator.RegisterService(ServiceConfig{
		Name:     "healthy-service",
		Client:   &mockServiceClient{},
		Critical: false,
	})
	aggregator.RegisterService(ServiceConfig{
		Name:     "critical-service",
		Client:   &mockServiceClient{err: errors.New("connection refused")},
		Critical: true,
	})

	ctx := context.Background()
	health := aggregator.CheckAll(ctx)

	if health.Status != pkghealth.StatusUnhealthy {
		t.Errorf("expected status %s, got %s", pkghealth.StatusUnhealthy, health.Status)
	}
}

func TestAggregator_CheckAll_NonCriticalFailure(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	aggregator.RegisterService(ServiceConfig{
		Name:     "healthy-service",
		Client:   &mockServiceClient{},
		Critical: true,
	})
	aggregator.RegisterService(ServiceConfig{
		Name:     "non-critical-service",
		Client:   &mockServiceClient{err: errors.New("service unavailable")},
		Critical: false,
	})

	ctx := context.Background()
	health := aggregator.CheckAll(ctx)

	if health.Status != pkghealth.StatusDegraded {
		t.Errorf("expected status %s, got %s", pkghealth.StatusDegraded, health.Status)
	}
}

func TestAggregator_CheckAll_Concurrent(t *testing.T) {
	aggregator := NewAggregator("1.0.0")
	aggregator.SetDefaultTimeout(100 * time.Millisecond)

	// Create services with delays to verify concurrent execution
	response1 := make(chan struct{}, 1)
	response2 := make(chan struct{}, 1)

	aggregator.RegisterService(ServiceConfig{
		Name:     "service1",
		Client:   &mockServiceClient{delay: 20 * time.Millisecond, response: response1},
		Critical: true,
	})
	aggregator.RegisterService(ServiceConfig{
		Name:     "service2",
		Client:   &mockServiceClient{delay: 20 * time.Millisecond, response: response2},
		Critical: true,
	})

	start := time.Now()
	ctx := context.Background()
	health := aggregator.CheckAll(ctx)
	elapsed := time.Since(start)

	// If running concurrently, both 20ms delays should overlap
	// Total time should be ~20ms, not ~40ms
	if elapsed > 50*time.Millisecond {
		t.Errorf("checks should run concurrently; took %v", elapsed)
	}

	if health.Status != pkghealth.StatusHealthy {
		t.Errorf("expected healthy status, got %s", health.Status)
	}
}

func TestAggregator_CheckAll_Timeout(t *testing.T) {
	aggregator := NewAggregator("1.0.0")
	aggregator.SetDefaultTimeout(50 * time.Millisecond)

	aggregator.RegisterService(ServiceConfig{
		Name:     "slow-service",
		Client:   &mockServiceClient{delay: 200 * time.Millisecond},
		Critical: true,
	})

	ctx := context.Background()
	health := aggregator.CheckAll(ctx)

	if health.Status != pkghealth.StatusUnhealthy {
		t.Errorf("expected unhealthy status due to timeout, got %s", health.Status)
	}

	// Find the slow-service result
	for _, svc := range health.Services {
		if svc.Name == "slow-service" {
			if svc.Status != pkghealth.StatusUnhealthy {
				t.Error("slow-service should be unhealthy")
			}
			return
		}
	}
	t.Error("slow-service not found in results")
}

func TestAggregator_CheckAll_ServiceTimeout(t *testing.T) {
	aggregator := NewAggregator("1.0.0")
	aggregator.SetDefaultTimeout(100 * time.Millisecond)

	// Service with custom timeout
	aggregator.RegisterService(ServiceConfig{
		Name:     "fast-timeout-service",
		Client:   &mockServiceClient{delay: 100 * time.Millisecond},
		Critical: true,
		Timeout:  20 * time.Millisecond,
	})

	ctx := context.Background()
	health := aggregator.CheckAll(ctx)

	if health.Status != pkghealth.StatusUnhealthy {
		t.Errorf("expected unhealthy status due to service timeout, got %s", health.Status)
	}
}

func TestCircuitBreaker_Basic(t *testing.T) {
	cb := newCircuitBreaker()

	// Initially should be closed
	if !cb.shouldExecute() {
		t.Error("circuit breaker should allow execution when closed")
	}

	// Record failures up to threshold
	for i := 0; i < cb.failureThreshold; i++ {
		cb.recordFailure()
	}

	// Should now be open
	if cb.getState() != CircuitOpen {
		t.Error("circuit should be open after threshold failures")
	}

	// Should not execute when open
	if cb.shouldExecute() {
		t.Error("circuit breaker should not allow execution when open")
	}
}

func TestCircuitBreaker_Recovery(t *testing.T) {
	cb := newCircuitBreaker()
	cb.resetTimeout = 50 * time.Millisecond

	// Trip the circuit
	for i := 0; i < cb.failureThreshold; i++ {
		cb.recordFailure()
	}

	if cb.getState() != CircuitOpen {
		t.Error("circuit should be open")
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open
	if !cb.shouldExecute() {
		t.Error("circuit breaker should allow execution after reset timeout")
	}

	if cb.getState() != CircuitHalfOpen {
		t.Error("circuit should be half-open")
	}

	// Record success - should close circuit
	cb.recordSuccess()

	if cb.getState() != CircuitClosed {
		t.Error("circuit should be closed after success")
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := newCircuitBreaker()
	cb.resetTimeout = 10 * time.Millisecond
	cb.failureThreshold = 2

	// Trip the circuit
	cb.recordFailure()
	cb.recordFailure()

	// Wait for reset timeout
	time.Sleep(20 * time.Millisecond)

	// Transition to half-open
	cb.shouldExecute()

	// Fail again - should go back to open
	cb.recordFailure()
	cb.recordFailure()

	if cb.getState() != CircuitOpen {
		t.Errorf("circuit should be open after half-open failure, got %v", cb.getState())
	}
}

func TestAggregator_CircuitBreaker_Integration(t *testing.T) {
	aggregator := NewAggregator("1.0.0")
	aggregator.SetDefaultTimeout(50 * time.Millisecond)

	// Create a service that will fail
	failCount := int32(0)
	client := &mockServiceClient{
		err: errors.New("service error"),
	}

	aggregator.RegisterService(ServiceConfig{
		Name:     "failing-service",
		Client:   client,
		Critical: true,
	})

	// Get the registered service to access its circuit breaker
	aggregator.mu.RLock()
	svc := aggregator.services["failing-service"]
	svc.circuitBreaker.failureThreshold = 3
	svc.circuitBreaker.resetTimeout = 100 * time.Millisecond
	aggregator.mu.RUnlock()

	ctx := context.Background()

	// Fail enough times to trip the circuit
	for i := 0; i < 3; i++ {
		aggregator.CheckAll(ctx)
		atomic.AddInt32(&failCount, 1)
	}

	// Circuit should now be open
	calls := client.getCalls()

	// Check one more time - circuit should be open
	health := aggregator.CheckAll(ctx)

	// Service should not have been called due to open circuit
	if client.getCalls() != calls {
		t.Error("service should not be called when circuit is open")
	}

	// Health should show circuit is open
	for _, svc := range health.Services {
		if svc.Name == "failing-service" {
			if svc.CircuitState != "open" {
				t.Errorf("expected circuit state 'open', got '%s'", svc.CircuitState)
			}
			return
		}
	}
	t.Error("failing-service not found")
}

func TestAggregator_Handler(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	aggregator.RegisterService(ServiceConfig{
		Name:     "test-service",
		Client:   &mockServiceClient{},
		Critical: true,
	})

	handler := aggregator.Handler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var health AggregatedHealth
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if health.Status != pkghealth.StatusHealthy {
		t.Errorf("expected status %s, got %s", pkghealth.StatusHealthy, health.Status)
	}
}

func TestAggregator_Handler_Unhealthy(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	aggregator.RegisterService(ServiceConfig{
		Name:     "failing-service",
		Client:   &mockServiceClient{err: errors.New("service down")},
		Critical: true,
	})

	handler := aggregator.Handler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	var health AggregatedHealth
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if health.Status != pkghealth.StatusUnhealthy {
		t.Errorf("expected status %s, got %s", pkghealth.StatusUnhealthy, health.Status)
	}
}

func TestAggregator_Handler_Degraded(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	aggregator.RegisterService(ServiceConfig{
		Name:     "healthy-service",
		Client:   &mockServiceClient{},
		Critical: true,
	})
	aggregator.RegisterService(ServiceConfig{
		Name:     "failing-service",
		Client:   &mockServiceClient{err: errors.New("service down")},
		Critical: false, // Non-critical
	})

	handler := aggregator.Handler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Degraded should still return 200 OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d for degraded, got %d", http.StatusOK, rec.Code)
	}

	var health AggregatedHealth
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if health.Status != pkghealth.StatusDegraded {
		t.Errorf("expected status %s, got %s", pkghealth.StatusDegraded, health.Status)
	}
}

func TestAggregator_ReadyHandler_Healthy(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	aggregator.RegisterService(ServiceConfig{
		Name:     "test-service",
		Client:   &mockServiceClient{},
		Critical: true,
	})

	handler := aggregator.ReadyHandler()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if ready, ok := response["ready"].(bool); !ok || !ready {
		t.Error("expected ready to be true")
	}
}

func TestAggregator_ReadyHandler_Unhealthy(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	aggregator.RegisterService(ServiceConfig{
		Name:     "failing-service",
		Client:   &mockServiceClient{err: errors.New("service down")},
		Critical: true,
	})

	handler := aggregator.ReadyHandler()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if ready, ok := response["ready"].(bool); !ok || ready {
		t.Error("expected ready to be false")
	}
}

func TestAggregator_ReadyHandler_Degraded(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	aggregator.RegisterService(ServiceConfig{
		Name:     "healthy-service",
		Client:   &mockServiceClient{},
		Critical: true,
	})
	aggregator.RegisterService(ServiceConfig{
		Name:     "failing-service",
		Client:   &mockServiceClient{err: errors.New("service down")},
		Critical: false, // Non-critical
	})

	handler := aggregator.ReadyHandler()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Degraded should still be ready
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d for degraded (still ready), got %d", http.StatusOK, rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if ready, ok := response["ready"].(bool); !ok || !ready {
		t.Error("expected ready to be true for degraded status")
	}
}

func TestAggregator_LiveHandler(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	// Even with failing services, liveness should return OK
	aggregator.RegisterService(ServiceConfig{
		Name:     "failing-service",
		Client:   &mockServiceClient{err: errors.New("service down")},
		Critical: true,
	})

	handler := aggregator.LiveHandler()
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if alive, ok := response["alive"].(bool); !ok || !alive {
		t.Error("expected alive to be true")
	}

	if _, ok := response["uptime"].(string); !ok {
		t.Error("expected uptime to be present")
	}
}

func TestAggregator_GetChecker(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	aggregator.RegisterService(ServiceConfig{
		Name:     "test-service",
		Client:   &mockServiceClient{},
		Critical: true,
	})

	checker := aggregator.GetChecker()
	if checker == nil {
		t.Fatal("GetChecker returned nil")
	}

	// Use the checker's Handler
	handler := checker.Handler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestAggregator_GetChecker_Unhealthy(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	aggregator.RegisterService(ServiceConfig{
		Name:     "failing-service",
		Client:   &mockServiceClient{err: errors.New("service down")},
		Critical: true,
	})

	checker := aggregator.GetChecker()

	handler := checker.Handler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestAggregatorError(t *testing.T) {
	err := &AggregatorError{
		Health: AggregatedHealth{
			Services: []ServiceHealth{
				{Name: "service1", Status: pkghealth.StatusUnhealthy},
				{Name: "service2", Status: pkghealth.StatusHealthy},
				{Name: "service3", Status: pkghealth.StatusUnhealthy},
			},
		},
	}

	errMsg := err.Error()
	if errMsg != "unhealthy services: service1, service3" {
		t.Errorf("unexpected error message: %s", errMsg)
	}
}

func TestAggregator_NoServices(t *testing.T) {
	aggregator := NewAggregator("1.0.0")

	ctx := context.Background()
	health := aggregator.CheckAll(ctx)

	// With no services, should be healthy
	if health.Status != pkghealth.StatusHealthy {
		t.Errorf("expected healthy with no services, got %s", health.Status)
	}

	if len(health.Services) != 0 {
		t.Errorf("expected 0 services, got %d", len(health.Services))
	}
}

func TestAggregator_ContextCancellation(t *testing.T) {
	aggregator := NewAggregator("1.0.0")
	aggregator.SetDefaultTimeout(1 * time.Second)

	aggregator.RegisterService(ServiceConfig{
		Name:     "slow-service",
		Client:   &mockServiceClient{delay: 500 * time.Millisecond},
		Critical: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	health := aggregator.CheckAll(ctx)
	elapsed := time.Since(start)

	// Should complete faster than the delay due to context cancellation
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected faster completion due to context timeout, took %v", elapsed)
	}

	// Service should be unhealthy due to timeout
	if health.Status != pkghealth.StatusUnhealthy {
		t.Errorf("expected unhealthy status, got %s", health.Status)
	}
}

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		input    []string
		sep      string
		expected string
	}{
		{[]string{}, ", ", ""},
		{[]string{"a"}, ", ", "a"},
		{[]string{"a", "b"}, ", ", "a, b"},
		{[]string{"a", "b", "c"}, "-", "a-b-c"},
	}

	for _, tt := range tests {
		result := joinStrings(tt.input, tt.sep)
		if result != tt.expected {
			t.Errorf("joinStrings(%v, %q) = %q, want %q", tt.input, tt.sep, result, tt.expected)
		}
	}
}

func TestServiceHealth_JSON(t *testing.T) {
	health := ServiceHealth{
		Name:         "test-service",
		Status:       pkghealth.StatusHealthy,
		Message:      "all good",
		LatencyMs:    42,
		CircuitState: "closed",
		Critical:     true,
	}

	data, err := json.Marshal(health)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled ServiceHealth
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Name != health.Name {
		t.Errorf("name mismatch: got %s, want %s", unmarshaled.Name, health.Name)
	}
	if unmarshaled.LatencyMs != health.LatencyMs {
		t.Errorf("latency mismatch: got %d, want %d", unmarshaled.LatencyMs, health.LatencyMs)
	}
}

func TestAggregatedHealth_JSON(t *testing.T) {
	health := AggregatedHealth{
		Status: pkghealth.StatusHealthy,
		Services: []ServiceHealth{
			{Name: "svc1", Status: pkghealth.StatusHealthy},
		},
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Uptime:    "1h30m",
	}

	data, err := json.Marshal(health)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled AggregatedHealth
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Version != health.Version {
		t.Errorf("version mismatch: got %s, want %s", unmarshaled.Version, health.Version)
	}
	if len(unmarshaled.Services) != 1 {
		t.Errorf("services count mismatch: got %d, want 1", len(unmarshaled.Services))
	}
}
