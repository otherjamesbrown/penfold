// Package health provides health check aggregation for the API Gateway.
// It collects health status from all backend services and exposes unified endpoints.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	pkghealth "github.com/otherjamesbrown/penfold/pkg/health"
)

// ServiceClient is the interface that backend services must implement
// to participate in health aggregation.
type ServiceClient interface {
	// HealthCheck performs a health check on the service.
	// It returns an error if the service is unhealthy.
	HealthCheck(ctx context.Context) error
}

// ServiceConfig holds configuration for a registered service.
type ServiceConfig struct {
	// Name is the service identifier.
	Name string
	// Client is the health check client for the service.
	Client ServiceClient
	// Critical indicates whether the service is critical to overall health.
	// If a critical service is unhealthy, the overall status is unhealthy.
	// If a non-critical service is unhealthy, the status is degraded.
	Critical bool
	// Timeout is the timeout for health checks on this service.
	// If zero, the default timeout is used.
	Timeout time.Duration
}

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed means the service is healthy and requests flow normally.
	CircuitClosed CircuitState = iota
	// CircuitOpen means the service has failed too many times and is being skipped.
	CircuitOpen
	// CircuitHalfOpen means the circuit is testing if the service has recovered.
	CircuitHalfOpen
)

// circuitBreaker implements the circuit breaker pattern for a service.
type circuitBreaker struct {
	mu              sync.RWMutex
	state           CircuitState
	failureCount    int
	lastFailureTime time.Time
	lastCheckTime   time.Time

	// Configuration
	failureThreshold int
	resetTimeout     time.Duration
}

// newCircuitBreaker creates a new circuit breaker with default settings.
func newCircuitBreaker() *circuitBreaker {
	return &circuitBreaker{
		state:            CircuitClosed,
		failureThreshold: 5,
		resetTimeout:     30 * time.Second,
	}
}

// shouldExecute determines if a health check should be executed.
func (cb *circuitBreaker) shouldExecute() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if enough time has passed to try again
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	}
	return true
}

// recordSuccess records a successful health check.
func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.state = CircuitClosed
	cb.lastCheckTime = time.Now()
}

// recordFailure records a failed health check.
func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()
	cb.lastCheckTime = time.Now()

	if cb.failureCount >= cb.failureThreshold {
		cb.state = CircuitOpen
	}
}

// getState returns the current circuit state.
func (cb *circuitBreaker) getState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// ServiceHealth represents the health status of a single backend service.
type ServiceHealth struct {
	Name         string                `json:"name"`
	Status       pkghealth.Status      `json:"status"`
	Message      string                `json:"message,omitempty"`
	LatencyMs    int64                 `json:"latency_ms"`
	CircuitState string                `json:"circuit_state"`
	Critical     bool                  `json:"critical"`
	Error        string                `json:"error,omitempty"`
}

// AggregatedHealth represents the combined health of all backend services.
type AggregatedHealth struct {
	Status    pkghealth.Status `json:"status"`
	Services  []ServiceHealth  `json:"services"`
	Timestamp time.Time        `json:"timestamp"`
	Version   string           `json:"version,omitempty"`
	Uptime    string           `json:"uptime,omitempty"`
}

// registeredService holds a service registration with its circuit breaker.
type registeredService struct {
	config         ServiceConfig
	circuitBreaker *circuitBreaker
}

// Aggregator aggregates health status from multiple backend services.
type Aggregator struct {
	mu       sync.RWMutex
	services map[string]*registeredService

	// Default timeout for health checks
	defaultTimeout time.Duration

	// Version and start time for uptime reporting
	version   string
	startTime time.Time
}

// NewAggregator creates a new health aggregator.
func NewAggregator(version string) *Aggregator {
	return &Aggregator{
		services:       make(map[string]*registeredService),
		defaultTimeout: 5 * time.Second,
		version:        version,
		startTime:      time.Now(),
	}
}

// SetDefaultTimeout sets the default timeout for health checks.
func (a *Aggregator) SetDefaultTimeout(timeout time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.defaultTimeout = timeout
}

// RegisterService registers a backend service for health aggregation.
func (a *Aggregator) RegisterService(cfg ServiceConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.services[cfg.Name] = &registeredService{
		config:         cfg,
		circuitBreaker: newCircuitBreaker(),
	}
}

// UnregisterService removes a service from health aggregation.
func (a *Aggregator) UnregisterService(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.services, name)
}

// checkService performs a health check on a single service.
func (a *Aggregator) checkService(ctx context.Context, svc *registeredService) ServiceHealth {
	result := ServiceHealth{
		Name:     svc.config.Name,
		Critical: svc.config.Critical,
	}

	// Check circuit breaker
	if !svc.circuitBreaker.shouldExecute() {
		result.Status = pkghealth.StatusUnhealthy
		result.Message = "Circuit breaker open"
		result.CircuitState = "open"
		result.Error = "service temporarily unavailable due to repeated failures"
		return result
	}

	// Determine timeout
	timeout := svc.config.Timeout
	if timeout == 0 {
		a.mu.RLock()
		timeout = a.defaultTimeout
		a.mu.RUnlock()
	}

	// Create context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Perform health check
	start := time.Now()
	err := svc.config.Client.HealthCheck(checkCtx)
	result.LatencyMs = time.Since(start).Milliseconds()

	// Record result in circuit breaker
	if err != nil {
		svc.circuitBreaker.recordFailure()
		result.Status = pkghealth.StatusUnhealthy
		result.Error = err.Error()

		// Set circuit state
		switch svc.circuitBreaker.getState() {
		case CircuitOpen:
			result.CircuitState = "open"
		case CircuitHalfOpen:
			result.CircuitState = "half-open"
		default:
			result.CircuitState = "closed"
		}
	} else {
		svc.circuitBreaker.recordSuccess()
		result.Status = pkghealth.StatusHealthy
		result.Message = "healthy"
		result.CircuitState = "closed"
	}

	return result
}

// CheckAll performs health checks on all registered services concurrently.
func (a *Aggregator) CheckAll(ctx context.Context) AggregatedHealth {
	a.mu.RLock()
	services := make([]*registeredService, 0, len(a.services))
	for _, svc := range a.services {
		services = append(services, svc)
	}
	a.mu.RUnlock()

	// Channel to collect results
	results := make(chan ServiceHealth, len(services))

	// Check all services concurrently
	var wg sync.WaitGroup
	for _, svc := range services {
		wg.Add(1)
		go func(s *registeredService) {
			defer wg.Done()
			results <- a.checkService(ctx, s)
		}(svc)
	}

	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	serviceResults := make([]ServiceHealth, 0, len(services))
	hasCriticalFailure := false
	hasNonCriticalFailure := false

	for result := range results {
		serviceResults = append(serviceResults, result)
		if result.Status == pkghealth.StatusUnhealthy {
			if result.Critical {
				hasCriticalFailure = true
			} else {
				hasNonCriticalFailure = true
			}
		}
	}

	// Determine overall status
	overallStatus := pkghealth.StatusHealthy
	if hasCriticalFailure {
		overallStatus = pkghealth.StatusUnhealthy
	} else if hasNonCriticalFailure {
		overallStatus = pkghealth.StatusDegraded
	}

	return AggregatedHealth{
		Status:    overallStatus,
		Services:  serviceResults,
		Timestamp: time.Now(),
		Version:   a.version,
		Uptime:    time.Since(a.startTime).String(),
	}
}

// Handler returns an HTTP handler for the /health endpoint.
// It returns full health status with all service details.
func (a *Aggregator) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		health := a.CheckAll(ctx)

		w.Header().Set("Content-Type", "application/json")

		switch health.Status {
		case pkghealth.StatusHealthy:
			w.WriteHeader(http.StatusOK)
		case pkghealth.StatusDegraded:
			w.WriteHeader(http.StatusOK) // Still OK but with warning
		case pkghealth.StatusUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(health)
	})
}

// ReadyHandler returns an HTTP handler for the /ready endpoint.
// It returns 200 OK if all critical services are healthy, 503 otherwise.
// This is useful for Kubernetes readiness probes.
func (a *Aggregator) ReadyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		health := a.CheckAll(ctx)

		w.Header().Set("Content-Type", "application/json")

		if health.Status == pkghealth.StatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready":  health.Status != pkghealth.StatusUnhealthy,
			"status": health.Status,
		})
	})
}

// LiveHandler returns an HTTP handler for the /live endpoint.
// It always returns 200 OK as long as the gateway itself is running.
// This is useful for Kubernetes liveness probes.
func (a *Aggregator) LiveHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"alive":  true,
			"uptime": time.Since(a.startTime).String(),
		})
	})
}

// GetChecker returns a pkg/health.Checker that wraps this aggregator.
// This allows integration with the existing health check infrastructure.
func (a *Aggregator) GetChecker() *pkghealth.Checker {
	checker := pkghealth.NewChecker()

	// Register a check function that uses the aggregator
	checker.RegisterCheck("backend-services", func(ctx context.Context) error {
		health := a.CheckAll(ctx)
		if health.Status == pkghealth.StatusUnhealthy {
			return &AggregatorError{Health: health}
		}
		return nil
	}, pkghealth.Critical())

	return checker
}

// AggregatorError is returned when the aggregated health check fails.
type AggregatorError struct {
	Health AggregatedHealth
}

func (e *AggregatorError) Error() string {
	unhealthyServices := make([]string, 0)
	for _, svc := range e.Health.Services {
		if svc.Status == pkghealth.StatusUnhealthy {
			unhealthyServices = append(unhealthyServices, svc.Name)
		}
	}
	if len(unhealthyServices) > 0 {
		return "unhealthy services: " + joinStrings(unhealthyServices, ", ")
	}
	return "aggregated health check failed"
}

// joinStrings joins strings with a separator (avoiding strings import).
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
