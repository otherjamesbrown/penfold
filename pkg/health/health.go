// Package health provides shared health check functionality for Go microservices.
// It supports registering multiple health checks, aggregating their results,
// and exposing HTTP endpoints for health and readiness probes.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Status represents the health state of a service or check.
type Status string

const (
	// StatusHealthy indicates all checks passed.
	StatusHealthy Status = "healthy"
	// StatusDegraded indicates some non-critical checks failed.
	StatusDegraded Status = "degraded"
	// StatusUnhealthy indicates critical checks failed.
	StatusUnhealthy Status = "unhealthy"
)

// CheckResult represents the result of a single health check.
type CheckResult struct {
	Status   Status        `json:"status"`
	Message  string        `json:"message,omitempty"`
	Duration time.Duration `json:"duration_ms"`
	Error    string        `json:"error,omitempty"`
}

// MarshalJSON customizes JSON output to show duration in milliseconds.
func (c CheckResult) MarshalJSON() ([]byte, error) {
	type Alias CheckResult
	return json.Marshal(&struct {
		Duration int64 `json:"duration_ms"`
		*Alias
	}{
		Duration: c.Duration.Milliseconds(),
		Alias:    (*Alias)(&c),
	})
}

// HealthStatus represents the overall health status of a service.
type HealthStatus struct {
	Status    Status                 `json:"status"`
	Checks    map[string]CheckResult `json:"checks"`
	Timestamp time.Time              `json:"timestamp"`
}

// CheckFunc is a function that performs a health check.
// It should return an error if the check fails.
type CheckFunc func(ctx context.Context) error

// CheckOption configures a health check registration.
type CheckOption func(*checkConfig)

type checkConfig struct {
	critical bool
}

// Critical marks a check as critical. If a critical check fails,
// the overall status is unhealthy. If a non-critical check fails,
// the status is degraded.
func Critical() CheckOption {
	return func(c *checkConfig) {
		c.critical = true
	}
}

type registeredCheck struct {
	name     string
	fn       CheckFunc
	critical bool
}

// Checker manages health checks for a service.
type Checker struct {
	mu     sync.RWMutex
	checks []registeredCheck
}

// NewChecker creates a new health checker.
func NewChecker() *Checker {
	return &Checker{
		checks: make([]registeredCheck, 0),
	}
}

// RegisterCheck adds a health check with the given name.
// Use options like Critical() to configure the check behavior.
func (h *Checker) RegisterCheck(name string, fn CheckFunc, opts ...CheckOption) {
	cfg := &checkConfig{
		critical: true, // Default to critical
	}
	for _, opt := range opts {
		opt(cfg)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.checks = append(h.checks, registeredCheck{
		name:     name,
		fn:       fn,
		critical: cfg.critical,
	})
}

// Check runs all registered health checks and returns the aggregated status.
func (h *Checker) Check(ctx context.Context) HealthStatus {
	h.mu.RLock()
	checks := make([]registeredCheck, len(h.checks))
	copy(checks, h.checks)
	h.mu.RUnlock()

	results := make(map[string]CheckResult)
	var wg sync.WaitGroup
	var resultsMu sync.Mutex

	hasCriticalFailure := false
	hasNonCriticalFailure := false
	var failureMu sync.Mutex

	for _, check := range checks {
		wg.Add(1)
		go func(c registeredCheck) {
			defer wg.Done()

			start := time.Now()
			err := c.fn(ctx)
			duration := time.Since(start)

			result := CheckResult{
				Status:   StatusHealthy,
				Duration: duration,
			}

			if err != nil {
				result.Status = StatusUnhealthy
				result.Error = err.Error()

				failureMu.Lock()
				if c.critical {
					hasCriticalFailure = true
				} else {
					hasNonCriticalFailure = true
				}
				failureMu.Unlock()
			}

			resultsMu.Lock()
			results[c.name] = result
			resultsMu.Unlock()
		}(check)
	}

	wg.Wait()

	overallStatus := StatusHealthy
	if hasCriticalFailure {
		overallStatus = StatusUnhealthy
	} else if hasNonCriticalFailure {
		overallStatus = StatusDegraded
	}

	return HealthStatus{
		Status:    overallStatus,
		Checks:    results,
		Timestamp: time.Now(),
	}
}

// Handler returns an HTTP handler for health check endpoints.
// It responds with the full health status as JSON.
func (h *Checker) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		status := h.Check(ctx)

		w.Header().Set("Content-Type", "application/json")

		switch status.Status {
		case StatusHealthy:
			w.WriteHeader(http.StatusOK)
		case StatusDegraded:
			w.WriteHeader(http.StatusOK) // Still OK but with warning
		case StatusUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		_ = json.NewEncoder(w).Encode(status)
	})
}

// ReadyHandler returns an HTTP handler for readiness probes.
// It returns 200 OK if all critical checks pass, 503 otherwise.
// This is useful for Kubernetes readiness probes.
func (h *Checker) ReadyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		status := h.Check(ctx)

		w.Header().Set("Content-Type", "application/json")

		if status.Status == StatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		_ = json.NewEncoder(w).Encode(status)
	})
}

// LiveHandler returns an HTTP handler for liveness probes.
// It always returns 200 OK as long as the service is running.
// This is useful for Kubernetes liveness probes.
func (h *Checker) LiveHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
	})
}
