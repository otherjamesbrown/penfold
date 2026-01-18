// Package health provides HTTP health check endpoints for the pipeline service.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/otherjamesbrown/penfold-go-pipeline/internal/config"
)

// Status represents the health status of a component.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusDegraded  Status = "degraded"
)

// ComponentHealth represents the health of an individual component.
type ComponentHealth struct {
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// HealthResponse represents the overall health check response.
type HealthResponse struct {
	Status     Status                     `json:"status"`
	Timestamp  string                     `json:"timestamp"`
	Version    string                     `json:"version,omitempty"`
	Components map[string]ComponentHealth `json:"components,omitempty"`
}

// ReadyResponse represents the readiness check response.
type ReadyResponse struct {
	Ready      bool                       `json:"ready"`
	Timestamp  string                     `json:"timestamp"`
	Components map[string]ComponentHealth `json:"components"`
}

// Checker defines the interface for health check dependencies.
type Checker interface {
	Check(ctx context.Context) ComponentHealth
	Name() string
}

// RedisChecker checks Redis connectivity.
type RedisChecker struct {
	client *redis.Client
}

// NewRedisChecker creates a new Redis health checker.
func NewRedisChecker(client *redis.Client) *RedisChecker {
	return &RedisChecker{client: client}
}

// Name returns the component name.
func (c *RedisChecker) Name() string {
	return "redis"
}

// Check performs a Redis health check.
func (c *RedisChecker) Check(ctx context.Context) ComponentHealth {
	start := time.Now()

	if c.client == nil {
		return ComponentHealth{
			Status:  StatusUnhealthy,
			Message: "redis client not configured",
		}
	}

	if err := c.client.Ping(ctx).Err(); err != nil {
		return ComponentHealth{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("ping failed: %v", err),
			Latency: time.Since(start).String(),
		}
	}

	return ComponentHealth{
		Status:  StatusHealthy,
		Message: "connected",
		Latency: time.Since(start).String(),
	}
}

// HTTPChecker checks HTTP service connectivity.
type HTTPChecker struct {
	name    string
	url     string
	timeout time.Duration
	client  *http.Client
}

// NewHTTPChecker creates a new HTTP service health checker.
func NewHTTPChecker(name, url string, timeout time.Duration) *HTTPChecker {
	return &HTTPChecker{
		name:    name,
		url:     url,
		timeout: timeout,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Name returns the component name.
func (c *HTTPChecker) Name() string {
	return c.name
}

// Check performs an HTTP health check.
func (c *HTTPChecker) Check(ctx context.Context) ComponentHealth {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/health", nil)
	if err != nil {
		return ComponentHealth{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("failed to create request: %v", err),
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return ComponentHealth{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("request failed: %v", err),
			Latency: time.Since(start).String(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: fmt.Sprintf("status %d", resp.StatusCode),
			Latency: time.Since(start).String(),
		}
	}

	return ComponentHealth{
		Status:  StatusDegraded,
		Message: fmt.Sprintf("unexpected status %d", resp.StatusCode),
		Latency: time.Since(start).String(),
	}
}

// DBChecker checks database connectivity using a ping function.
type DBChecker struct {
	pingFunc func(ctx context.Context) error
}

// NewDBChecker creates a new database health checker.
func NewDBChecker(pingFunc func(ctx context.Context) error) *DBChecker {
	return &DBChecker{pingFunc: pingFunc}
}

// Name returns the component name.
func (c *DBChecker) Name() string {
	return "database"
}

// Check performs a database health check.
func (c *DBChecker) Check(ctx context.Context) ComponentHealth {
	start := time.Now()

	if c.pingFunc == nil {
		return ComponentHealth{
			Status:  StatusUnhealthy,
			Message: "database ping function not configured",
		}
	}

	if err := c.pingFunc(ctx); err != nil {
		return ComponentHealth{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("ping failed: %v", err),
			Latency: time.Since(start).String(),
		}
	}

	return ComponentHealth{
		Status:  StatusHealthy,
		Message: "connected",
		Latency: time.Since(start).String(),
	}
}

// Server provides HTTP health check endpoints.
type Server struct {
	cfg      *config.ServerConfig
	log      zerolog.Logger
	checkers []Checker
	version  string
	server   *http.Server
	mu       sync.RWMutex
}

// NewServer creates a new health check server.
func NewServer(cfg *config.ServerConfig, log zerolog.Logger, version string) *Server {
	return &Server{
		cfg:      cfg,
		log:      log.With().Str("component", "health").Logger(),
		checkers: make([]Checker, 0),
		version:  version,
	}
}

// AddChecker registers a health checker.
func (s *Server) AddChecker(checker Checker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkers = append(s.checkers, checker)
	s.log.Debug().Str("checker", checker.Name()).Msg("registered health checker")
}

// Start begins serving health check endpoints.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/live", s.handleLive)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.HealthPort),
		Handler:      mux,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		IdleTimeout:  s.cfg.IdleTimeout,
	}

	s.log.Info().Int("port", s.cfg.HealthPort).Msg("starting health check server")
	return s.server.ListenAndServe()
}

// Shutdown gracefully stops the health check server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server != nil {
		s.log.Info().Msg("shutting down health check server")
		return s.server.Shutdown(ctx)
	}
	return nil
}

// handleHealth returns the overall health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	s.mu.RLock()
	checkers := s.checkers
	s.mu.RUnlock()

	components := make(map[string]ComponentHealth)
	overallStatus := StatusHealthy

	for _, checker := range checkers {
		health := checker.Check(ctx)
		components[checker.Name()] = health

		if health.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
		} else if health.Status == StatusDegraded && overallStatus == StatusHealthy {
			overallStatus = StatusDegraded
		}
	}

	resp := HealthResponse{
		Status:     overallStatus,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Version:    s.version,
		Components: components,
	}

	w.Header().Set("Content-Type", "application/json")
	if overallStatus == StatusUnhealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(resp)
}

// handleReady returns readiness status (all dependencies must be healthy).
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	s.mu.RLock()
	checkers := s.checkers
	s.mu.RUnlock()

	components := make(map[string]ComponentHealth)
	ready := true

	for _, checker := range checkers {
		health := checker.Check(ctx)
		components[checker.Name()] = health

		if health.Status == StatusUnhealthy {
			ready = false
		}
	}

	resp := ReadyResponse{
		Ready:      ready,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Components: components,
	}

	w.Header().Set("Content-Type", "application/json")
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(resp)
}

// handleLive returns liveness status (service is running).
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"alive":     true,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
