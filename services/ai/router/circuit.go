// Package router provides AI model routing with circuit breaker fault tolerance.
// It manages multiple model backends and routes requests based on model selection
// while providing resilience through circuit breakers and fallback chains.
package router

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int32

const (
	// CircuitClosed is the normal operating state where requests flow through.
	CircuitClosed CircuitState = iota
	// CircuitOpen indicates the circuit is tripped and requests are rejected.
	CircuitOpen
	// CircuitHalfOpen allows limited requests to test if the backend recovered.
	CircuitHalfOpen
)

// String returns the string representation of a CircuitState.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures before opening the circuit.
	// Default: 5
	FailureThreshold int

	// SuccessThreshold is the number of consecutive successes in half-open state
	// required to close the circuit.
	// Default: 2
	SuccessThreshold int

	// ResetTimeout is the duration to wait before transitioning from open to half-open.
	// Default: 30 seconds
	ResetTimeout time.Duration

	// HalfOpenMaxRequests is the maximum number of concurrent requests allowed
	// when the circuit is in half-open state.
	// Default: 1
	HalfOpenMaxRequests int

	// OnStateChange is called when the circuit state changes.
	// Optional callback for monitoring/logging.
	OnStateChange func(name string, from, to CircuitState)
}

// DefaultCircuitBreakerConfig returns a CircuitBreakerConfig with sensible defaults.
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold:    5,
		SuccessThreshold:    2,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxRequests: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern for fault tolerance.
// It tracks failures and opens the circuit when a threshold is reached,
// preventing cascading failures and allowing backends to recover.
type CircuitBreaker struct {
	name   string
	config *CircuitBreakerConfig

	mu                sync.RWMutex
	state             CircuitState
	failures          int
	successes         int
	lastFailure       time.Time
	openedAt          time.Time
	halfOpenRequests  int32
	totalRequests     int64
	totalFailures     int64
	totalSuccesses    int64
	consecutiveFails  int
}

// NewCircuitBreaker creates a new circuit breaker with the given name and configuration.
func NewCircuitBreaker(name string, cfg *CircuitBreakerConfig) *CircuitBreaker {
	if cfg == nil {
		cfg = DefaultCircuitBreakerConfig()
	}

	// Apply defaults for zero values
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 2
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 30 * time.Second
	}
	if cfg.HalfOpenMaxRequests <= 0 {
		cfg.HalfOpenMaxRequests = 1
	}

	return &CircuitBreaker{
		name:   name,
		config: cfg,
		state:  CircuitClosed,
	}
}

// Name returns the name of this circuit breaker.
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats holds statistics about a circuit breaker's operation.
type Stats struct {
	State            CircuitState
	TotalRequests    int64
	TotalSuccesses   int64
	TotalFailures    int64
	ConsecutiveFails int
	LastFailure      time.Time
	OpenedAt         time.Time
}

// Stats returns current statistics about the circuit breaker.
func (cb *CircuitBreaker) Stats() Stats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return Stats{
		State:            cb.state,
		TotalRequests:    cb.totalRequests,
		TotalSuccesses:   cb.totalSuccesses,
		TotalFailures:    cb.totalFailures,
		ConsecutiveFails: cb.consecutiveFails,
		LastFailure:      cb.lastFailure,
		OpenedAt:         cb.openedAt,
	}
}

// ErrCircuitOpen is returned when the circuit is open and not accepting requests.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// ErrCircuitHalfOpenBusy is returned when the circuit is half-open but has
// reached its maximum number of concurrent test requests.
var ErrCircuitHalfOpenBusy = errors.New("circuit breaker is half-open and busy")

// Allow checks if a request should be allowed through the circuit breaker.
// Returns nil if allowed, or an error explaining why the request was rejected.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case CircuitClosed:
		return nil

	case CircuitOpen:
		// Check if reset timeout has elapsed
		if now.Sub(cb.openedAt) >= cb.config.ResetTimeout {
			cb.transitionTo(CircuitHalfOpen)
			atomic.AddInt32(&cb.halfOpenRequests, 1)
			return nil
		}
		return ErrCircuitOpen

	case CircuitHalfOpen:
		// Allow limited concurrent requests in half-open state
		current := atomic.LoadInt32(&cb.halfOpenRequests)
		if current >= int32(cb.config.HalfOpenMaxRequests) {
			return ErrCircuitHalfOpenBusy
		}
		atomic.AddInt32(&cb.halfOpenRequests, 1)
		return nil

	default:
		return nil
	}
}

// RecordSuccess records a successful request.
// In half-open state, may close the circuit if success threshold is reached.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++
	cb.totalSuccesses++
	cb.consecutiveFails = 0

	switch cb.state {
	case CircuitClosed:
		cb.failures = 0

	case CircuitHalfOpen:
		atomic.AddInt32(&cb.halfOpenRequests, -1)
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.transitionTo(CircuitClosed)
		}
	}
}

// RecordFailure records a failed request.
// May open the circuit if failure threshold is reached.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++
	cb.totalFailures++
	cb.consecutiveFails++
	cb.lastFailure = time.Now()

	switch cb.state {
	case CircuitClosed:
		cb.failures++
		if cb.failures >= cb.config.FailureThreshold {
			cb.transitionTo(CircuitOpen)
		}

	case CircuitHalfOpen:
		atomic.AddInt32(&cb.halfOpenRequests, -1)
		// Any failure in half-open state immediately reopens the circuit
		cb.transitionTo(CircuitOpen)
	}
}

// Reset forces the circuit breaker to the closed state.
// Use with caution - typically only for administrative purposes.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	cb.consecutiveFails = 0
	atomic.StoreInt32(&cb.halfOpenRequests, 0)

	if cb.config.OnStateChange != nil && oldState != CircuitClosed {
		cb.config.OnStateChange(cb.name, oldState, CircuitClosed)
	}
}

// transitionTo changes the circuit state and invokes the callback.
// Must be called with mu held.
func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState
	cb.failures = 0
	cb.successes = 0

	switch newState {
	case CircuitOpen:
		cb.openedAt = time.Now()
	case CircuitHalfOpen:
		atomic.StoreInt32(&cb.halfOpenRequests, 0)
	}

	if cb.config.OnStateChange != nil {
		// Call outside of lock to prevent deadlocks
		go cb.config.OnStateChange(cb.name, oldState, newState)
	}
}

// Execute runs the given function if the circuit allows it.
// Automatically records success or failure based on the returned error.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := cb.Allow(); err != nil {
		return err
	}

	err := fn(ctx)
	if err != nil {
		cb.RecordFailure()
		return err
	}

	cb.RecordSuccess()
	return nil
}

// ExecuteWithFallback runs the function, and if it fails or the circuit is open,
// attempts the fallback function.
func (cb *CircuitBreaker) ExecuteWithFallback(
	ctx context.Context,
	fn func(ctx context.Context) error,
	fallback func(ctx context.Context) error,
) error {
	err := cb.Execute(ctx, fn)
	if err == nil {
		return nil
	}

	// If circuit is open or the primary function failed, try fallback
	if fallback != nil {
		return fallback(ctx)
	}

	return err
}

// CircuitBreakerManager manages multiple circuit breakers.
type CircuitBreakerManager struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   *CircuitBreakerConfig
}

// NewCircuitBreakerManager creates a manager with shared configuration.
func NewCircuitBreakerManager(cfg *CircuitBreakerConfig) *CircuitBreakerManager {
	if cfg == nil {
		cfg = DefaultCircuitBreakerConfig()
	}
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		config:   cfg,
	}
}

// Get returns the circuit breaker for the given name, creating one if needed.
func (m *CircuitBreakerManager) Get(name string) *CircuitBreaker {
	m.mu.RLock()
	cb, ok := m.breakers[name]
	m.mu.RUnlock()

	if ok {
		return cb
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, ok = m.breakers[name]; ok {
		return cb
	}

	cb = NewCircuitBreaker(name, m.config)
	m.breakers[name] = cb
	return cb
}

// GetExisting returns the circuit breaker for the given name, or nil if not found.
func (m *CircuitBreakerManager) GetExisting(name string) *CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.breakers[name]
}

// All returns a copy of all circuit breakers.
func (m *CircuitBreakerManager) All() map[string]*CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*CircuitBreaker, len(m.breakers))
	for k, v := range m.breakers {
		result[k] = v
	}
	return result
}

// ResetAll resets all circuit breakers to closed state.
func (m *CircuitBreakerManager) ResetAll() {
	m.mu.RLock()
	breakers := make([]*CircuitBreaker, 0, len(m.breakers))
	for _, cb := range m.breakers {
		breakers = append(breakers, cb)
	}
	m.mu.RUnlock()

	for _, cb := range breakers {
		cb.Reset()
	}
}
