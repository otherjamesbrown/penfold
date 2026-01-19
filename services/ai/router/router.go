package router

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/prometheus/client_golang/prometheus"
)

// ModelBackend represents an AI model backend that can process requests.
type ModelBackend interface {
	// Name returns the unique identifier for this backend.
	Name() string

	// Provider returns the provider name (e.g., "ollama", "gemini", "openai").
	Provider() string

	// IsLocal returns true if this is a local model (not cloud API).
	IsLocal() bool

	// Capabilities returns the list of capabilities this backend supports.
	// Examples: "embedding", "chat", "summarization", "extraction"
	Capabilities() []string

	// HealthCheck performs a health check on the backend.
	// Returns nil if healthy, error describing the issue otherwise.
	HealthCheck(ctx context.Context) error

	// Process handles an AI request and returns the response.
	Process(ctx context.Context, req *Request) (*Response, error)
}

// Request represents an AI processing request.
type Request struct {
	// ID is a unique identifier for this request.
	ID string

	// Type specifies the request type (embedding, summary, extraction, classification).
	Type RequestType

	// Model is the preferred model name. Empty means use default.
	Model string

	// Content is the input text to process.
	Content string

	// Options contains type-specific options.
	Options map[string]interface{}

	// TenantID for multi-tenancy support.
	TenantID string

	// Priority affects queue ordering (higher = more important).
	Priority int

	// Timeout overrides the default request timeout.
	Timeout time.Duration

	// CreatedAt is when the request was created.
	CreatedAt time.Time
}

// RequestType identifies the type of AI request.
type RequestType string

const (
	RequestTypeEmbedding      RequestType = "embedding"
	RequestTypeSummary        RequestType = "summary"
	RequestTypeExtraction     RequestType = "extraction"
	RequestTypeClassification RequestType = "classification"
	RequestTypeChat           RequestType = "chat"
)

// Response represents the result of an AI request.
type Response struct {
	// RequestID links back to the original request.
	RequestID string

	// ModelUsed is the actual model that processed the request.
	ModelUsed string

	// ProviderUsed is the provider that handled the request.
	ProviderUsed string

	// Data contains the type-specific response data.
	Data interface{}

	// ProcessingTime is how long the request took.
	ProcessingTime time.Duration

	// TokensUsed is the token count for billing/rate limiting.
	TokensUsed int

	// Metadata contains additional response metadata.
	Metadata map[string]interface{}
}

// ModelSelector determines which model/backend to use for a request.
type ModelSelector interface {
	// Select chooses the best backend for the given request.
	// Returns the backend name and any error.
	Select(ctx context.Context, req *Request, available []string) (string, error)
}

// RouterConfig holds configuration for the ModelRouter.
type RouterConfig struct {
	// DefaultTimeout for requests without explicit timeout.
	DefaultTimeout time.Duration

	// MaxQueueSize is the maximum number of queued requests per backend.
	MaxQueueSize int

	// HealthCheckInterval is how often to check backend health.
	HealthCheckInterval time.Duration

	// CircuitBreakerConfig for backend circuit breakers.
	CircuitBreakerConfig *CircuitBreakerConfig

	// EnableMetrics enables Prometheus metrics collection.
	EnableMetrics bool

	// FallbackOrder defines the order to try backends on failure.
	// If empty, no fallback is attempted.
	FallbackOrder []string

	// Logger for router operations.
	Logger logging.Logger
}

// DefaultRouterConfig returns a RouterConfig with sensible defaults.
func DefaultRouterConfig() *RouterConfig {
	return &RouterConfig{
		DefaultTimeout:       30 * time.Second,
		MaxQueueSize:         100,
		HealthCheckInterval:  30 * time.Second,
		CircuitBreakerConfig: DefaultCircuitBreakerConfig(),
		EnableMetrics:        true,
	}
}

// ModelRouter routes AI requests to appropriate backends with fault tolerance.
type ModelRouter struct {
	config   *RouterConfig
	logger   logging.Logger
	selector ModelSelector

	mu       sync.RWMutex
	backends map[string]ModelBackend
	breakers *CircuitBreakerManager

	// Queue management
	queueMu    sync.Mutex
	queues     map[string]chan *queuedRequest
	queueSizes map[string]*int64

	// Load balancing counters
	requestCounts map[string]*int64

	// Health tracking
	healthMu     sync.RWMutex
	healthStatus map[string]bool
	lastHealth   map[string]time.Time

	// Metrics
	metrics *routerMetrics

	// Lifecycle
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	shutdownMu sync.Mutex
	shutdown   bool
}

// queuedRequest wraps a request with its response channel.
type queuedRequest struct {
	req      *Request
	respChan chan *queuedResponse
}

type queuedResponse struct {
	resp *Response
	err  error
}

// routerMetrics holds Prometheus metrics for the router.
type routerMetrics struct {
	requestsTotal    *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	queuedRequests   *prometheus.GaugeVec
	circuitState     *prometheus.GaugeVec
	backendHealth    *prometheus.GaugeVec
	fallbacksTotal   *prometheus.CounterVec
}

// NewModelRouter creates a new model router with the given configuration.
func NewModelRouter(cfg *RouterConfig) *ModelRouter {
	if cfg == nil {
		cfg = DefaultRouterConfig()
	}

	logger := cfg.Logger
	if logger == nil {
		logger = logging.MustGlobal().With(logging.F("component", "model_router"))
	}

	ctx, cancel := context.WithCancel(context.Background())

	r := &ModelRouter{
		config:        cfg,
		logger:        logger,
		backends:      make(map[string]ModelBackend),
		breakers:      NewCircuitBreakerManager(cfg.CircuitBreakerConfig),
		queues:        make(map[string]chan *queuedRequest),
		queueSizes:    make(map[string]*int64),
		requestCounts: make(map[string]*int64),
		healthStatus:  make(map[string]bool),
		lastHealth:    make(map[string]time.Time),
		ctx:           ctx,
		cancel:        cancel,
	}

	if cfg.EnableMetrics {
		r.initMetrics()
	}

	return r
}

// initMetrics initializes Prometheus metrics for the router.
func (r *ModelRouter) initMetrics() {
	r.metrics = &routerMetrics{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "penfold",
				Subsystem: "ai_router",
				Name:      "requests_total",
				Help:      "Total number of requests by backend and status",
			},
			[]string{"backend", "request_type", "status"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "penfold",
				Subsystem: "ai_router",
				Name:      "request_duration_seconds",
				Help:      "Request duration in seconds by backend",
				Buckets:   []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60},
			},
			[]string{"backend", "request_type"},
		),
		queuedRequests: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "penfold",
				Subsystem: "ai_router",
				Name:      "queued_requests",
				Help:      "Current number of queued requests per backend",
			},
			[]string{"backend"},
		),
		circuitState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "penfold",
				Subsystem: "ai_router",
				Name:      "circuit_state",
				Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
			},
			[]string{"backend"},
		),
		backendHealth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "penfold",
				Subsystem: "ai_router",
				Name:      "backend_health",
				Help:      "Backend health status (1=healthy, 0=unhealthy)",
			},
			[]string{"backend"},
		),
		fallbacksTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "penfold",
				Subsystem: "ai_router",
				Name:      "fallbacks_total",
				Help:      "Total number of fallback attempts by original and fallback backend",
			},
			[]string{"original_backend", "fallback_backend"},
		),
	}

	// Register metrics
	collectors := []prometheus.Collector{
		r.metrics.requestsTotal,
		r.metrics.requestDuration,
		r.metrics.queuedRequests,
		r.metrics.circuitState,
		r.metrics.backendHealth,
		r.metrics.fallbacksTotal,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			// If already registered, that's okay
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				r.logger.Warn("Failed to register metric", logging.Err(err))
			}
		}
	}
}

// SetSelector sets the model selector for routing decisions.
func (r *ModelRouter) SetSelector(s ModelSelector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.selector = s
}

// RegisterBackend adds a new backend to the router.
func (r *ModelRouter) RegisterBackend(backend ModelBackend) error {
	if backend == nil {
		return errors.New("backend cannot be nil")
	}

	name := backend.Name()
	if name == "" {
		return errors.New("backend name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.backends[name]; exists {
		return errors.New("backend already registered: " + name)
	}

	r.backends[name] = backend

	// Initialize queue
	r.queueMu.Lock()
	r.queues[name] = make(chan *queuedRequest, r.config.MaxQueueSize)
	size := int64(0)
	r.queueSizes[name] = &size
	r.queueMu.Unlock()

	// Initialize request counter
	count := int64(0)
	r.requestCounts[name] = &count

	// Initialize health status
	r.healthMu.Lock()
	r.healthStatus[name] = true // Assume healthy initially
	r.healthMu.Unlock()

	// Start queue processor
	r.wg.Add(1)
	go r.processQueue(name)

	r.logger.Info("Backend registered",
		logging.F("backend", name),
		logging.F("provider", backend.Provider()),
		logging.F("is_local", backend.IsLocal()),
		logging.F("capabilities", backend.Capabilities()),
	)

	return nil
}

// UnregisterBackend removes a backend from the router.
func (r *ModelRouter) UnregisterBackend(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.backends[name]; !exists {
		return errors.New("backend not found: " + name)
	}

	delete(r.backends, name)

	// Close queue channel (processor will exit)
	r.queueMu.Lock()
	if ch, ok := r.queues[name]; ok {
		close(ch)
		delete(r.queues, name)
	}
	delete(r.queueSizes, name)
	r.queueMu.Unlock()

	delete(r.requestCounts, name)

	r.healthMu.Lock()
	delete(r.healthStatus, name)
	delete(r.lastHealth, name)
	r.healthMu.Unlock()

	r.logger.Info("Backend unregistered", logging.F("backend", name))

	return nil
}

// GetBackend returns a registered backend by name.
func (r *ModelRouter) GetBackend(name string) (ModelBackend, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.backends[name]
	return b, ok
}

// ListBackends returns the names of all registered backends.
func (r *ModelRouter) ListBackends() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.backends))
	for name := range r.backends {
		names = append(names, name)
	}
	return names
}

// Route processes a request through the appropriate backend.
func (r *ModelRouter) Route(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	r.shutdownMu.Lock()
	isShutdown := r.shutdown
	r.shutdownMu.Unlock()
	if isShutdown {
		return nil, errors.New("router is shutting down")
	}

	// Set defaults
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}
	if req.Timeout <= 0 {
		req.Timeout = r.config.DefaultTimeout
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	// Select backend
	backendName, err := r.selectBackend(ctx, req)
	if err != nil {
		return nil, err
	}

	// Try to route with fallback support
	return r.routeWithFallback(ctx, req, backendName)
}

// selectBackend chooses the backend for a request.
func (r *ModelRouter) selectBackend(ctx context.Context, req *Request) (string, error) {
	r.mu.RLock()
	available := make([]string, 0, len(r.backends))
	for name := range r.backends {
		available = append(available, name)
	}
	r.mu.RUnlock()

	if len(available) == 0 {
		return "", errors.New("no backends available")
	}

	// If a specific model is requested and matches a backend, use it
	if req.Model != "" {
		r.mu.RLock()
		_, exists := r.backends[req.Model]
		r.mu.RUnlock()
		if exists {
			return req.Model, nil
		}
	}

	// Use selector if available
	r.mu.RLock()
	selector := r.selector
	r.mu.RUnlock()

	if selector != nil {
		return selector.Select(ctx, req, available)
	}

	// Default: round-robin among healthy backends
	return r.selectHealthyBackend(available)
}

// selectHealthyBackend performs simple round-robin among healthy backends.
func (r *ModelRouter) selectHealthyBackend(available []string) (string, error) {
	r.healthMu.RLock()
	defer r.healthMu.RUnlock()

	var healthy []string
	for _, name := range available {
		if r.healthStatus[name] {
			healthy = append(healthy, name)
		}
	}

	if len(healthy) == 0 {
		// Fall back to any backend if none are healthy
		if len(available) > 0 {
			return available[0], nil
		}
		return "", errors.New("no backends available")
	}

	// Simple round-robin: pick the one with fewest requests
	var selected string
	minCount := int64(^uint64(0) >> 1) // Max int64

	for _, name := range healthy {
		if counter, ok := r.requestCounts[name]; ok {
			count := atomic.LoadInt64(counter)
			if count < minCount {
				minCount = count
				selected = name
			}
		}
	}

	if selected == "" {
		selected = healthy[0]
	}

	return selected, nil
}

// routeWithFallback attempts to route with fallback on failure.
func (r *ModelRouter) routeWithFallback(ctx context.Context, req *Request, backendName string) (*Response, error) {
	// Get circuit breaker for this backend
	cb := r.breakers.Get(backendName)

	// Check if circuit allows the request
	if err := cb.Allow(); err != nil {
		r.logger.Debug("Circuit breaker blocked request",
			logging.F("backend", backendName),
			logging.F("circuit_state", cb.State().String()),
		)
		// Try fallback
		return r.tryFallback(ctx, req, backendName, err)
	}

	// Get the backend
	r.mu.RLock()
	backend, ok := r.backends[backendName]
	r.mu.RUnlock()

	if !ok {
		cb.RecordFailure()
		return r.tryFallback(ctx, req, backendName, errors.New("backend not found: "+backendName))
	}

	// Increment request counter
	if counter, ok := r.requestCounts[backendName]; ok {
		atomic.AddInt64(counter, 1)
	}

	// Execute the request
	start := time.Now()
	resp, err := backend.Process(ctx, req)
	duration := time.Since(start)

	// Record metrics
	r.recordMetrics(backendName, req.Type, duration, err)

	if err != nil {
		cb.RecordFailure()
		r.logger.Debug("Backend request failed",
			logging.F("backend", backendName),
			logging.F("request_type", string(req.Type)),
			logging.F("duration", duration),
			logging.Err(err),
		)
		return r.tryFallback(ctx, req, backendName, err)
	}

	cb.RecordSuccess()

	// Enrich response
	if resp != nil {
		resp.RequestID = req.ID
		resp.ModelUsed = backendName
		resp.ProviderUsed = backend.Provider()
		resp.ProcessingTime = duration
	}

	return resp, nil
}

// tryFallback attempts to use fallback backends.
func (r *ModelRouter) tryFallback(ctx context.Context, req *Request, failedBackend string, originalErr error) (*Response, error) {
	// Check if we have fallback order defined
	if len(r.config.FallbackOrder) == 0 {
		return nil, originalErr
	}

	// Find fallbacks that aren't the failed backend
	for _, fallback := range r.config.FallbackOrder {
		if fallback == failedBackend {
			continue
		}

		// Check if fallback backend exists
		r.mu.RLock()
		_, exists := r.backends[fallback]
		r.mu.RUnlock()
		if !exists {
			continue
		}

		// Check circuit breaker for fallback
		cb := r.breakers.Get(fallback)
		if err := cb.Allow(); err != nil {
			continue
		}

		r.logger.Debug("Attempting fallback",
			logging.F("original_backend", failedBackend),
			logging.F("fallback_backend", fallback),
		)

		if r.metrics != nil {
			r.metrics.fallbacksTotal.WithLabelValues(failedBackend, fallback).Inc()
		}

		// Try the fallback
		resp, err := r.routeToBackend(ctx, req, fallback)
		if err == nil {
			return resp, nil
		}

		// Fallback also failed, continue to next
		r.logger.Debug("Fallback failed",
			logging.F("fallback_backend", fallback),
			logging.Err(err),
		)
	}

	return nil, originalErr
}

// routeToBackend directly routes to a specific backend.
func (r *ModelRouter) routeToBackend(ctx context.Context, req *Request, backendName string) (*Response, error) {
	cb := r.breakers.Get(backendName)

	if err := cb.Allow(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	backend, ok := r.backends[backendName]
	r.mu.RUnlock()

	if !ok {
		cb.RecordFailure()
		return nil, errors.New("backend not found: " + backendName)
	}

	if counter, ok := r.requestCounts[backendName]; ok {
		atomic.AddInt64(counter, 1)
	}

	start := time.Now()
	resp, err := backend.Process(ctx, req)
	duration := time.Since(start)

	r.recordMetrics(backendName, req.Type, duration, err)

	if err != nil {
		cb.RecordFailure()
		return nil, err
	}

	cb.RecordSuccess()

	if resp != nil {
		resp.RequestID = req.ID
		resp.ModelUsed = backendName
		resp.ProviderUsed = backend.Provider()
		resp.ProcessingTime = duration
	}

	return resp, nil
}

// recordMetrics records metrics for a request.
func (r *ModelRouter) recordMetrics(backend string, reqType RequestType, duration time.Duration, err error) {
	if r.metrics == nil {
		return
	}

	status := "success"
	if err != nil {
		status = "error"
	}

	r.metrics.requestsTotal.WithLabelValues(backend, string(reqType), status).Inc()
	r.metrics.requestDuration.WithLabelValues(backend, string(reqType)).Observe(duration.Seconds())

	// Update circuit state metric
	cb := r.breakers.GetExisting(backend)
	if cb != nil {
		r.metrics.circuitState.WithLabelValues(backend).Set(float64(cb.State()))
	}
}

// QueueRequest queues a request for asynchronous processing.
func (r *ModelRouter) QueueRequest(ctx context.Context, req *Request) (<-chan *Response, <-chan error, error) {
	backendName, err := r.selectBackend(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	r.queueMu.Lock()
	queue, ok := r.queues[backendName]
	sizePtr := r.queueSizes[backendName]
	r.queueMu.Unlock()

	if !ok {
		return nil, nil, errors.New("queue not found for backend: " + backendName)
	}

	// Check queue size
	currentSize := atomic.LoadInt64(sizePtr)
	if currentSize >= int64(r.config.MaxQueueSize) {
		return nil, nil, errors.New("queue full for backend: " + backendName)
	}

	respChan := make(chan *queuedResponse, 1)
	queued := &queuedRequest{
		req:      req,
		respChan: respChan,
	}

	select {
	case queue <- queued:
		atomic.AddInt64(sizePtr, 1)
		if r.metrics != nil {
			r.metrics.queuedRequests.WithLabelValues(backendName).Inc()
		}
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
		return nil, nil, errors.New("queue full for backend: " + backendName)
	}

	// Return channels for response
	outResp := make(chan *Response, 1)
	outErr := make(chan error, 1)

	go func() {
		select {
		case qr := <-respChan:
			if qr.err != nil {
				outErr <- qr.err
			} else {
				outResp <- qr.resp
			}
		case <-ctx.Done():
			outErr <- ctx.Err()
		}
		close(outResp)
		close(outErr)
	}()

	return outResp, outErr, nil
}

// processQueue processes queued requests for a backend.
func (r *ModelRouter) processQueue(backendName string) {
	defer r.wg.Done()

	r.queueMu.Lock()
	queue := r.queues[backendName]
	sizePtr := r.queueSizes[backendName]
	r.queueMu.Unlock()

	for {
		select {
		case <-r.ctx.Done():
			return
		case queued, ok := <-queue:
			if !ok {
				return // Channel closed
			}

			atomic.AddInt64(sizePtr, -1)
			if r.metrics != nil {
				r.metrics.queuedRequests.WithLabelValues(backendName).Dec()
			}

			// Process the request
			ctx, cancel := context.WithTimeout(r.ctx, queued.req.Timeout)
			resp, err := r.routeToBackend(ctx, queued.req, backendName)
			cancel()

			queued.respChan <- &queuedResponse{resp: resp, err: err}
		}
	}
}

// StartHealthChecks starts background health checking of backends.
func (r *ModelRouter) StartHealthChecks() {
	r.wg.Add(1)
	go r.healthCheckLoop()
}

// healthCheckLoop periodically checks backend health.
func (r *ModelRouter) healthCheckLoop() {
	defer r.wg.Done()

	interval := r.config.HealthCheckInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Do an initial check
	r.checkAllBackends()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.checkAllBackends()
		}
	}
}

// checkAllBackends runs health checks on all backends.
func (r *ModelRouter) checkAllBackends() {
	r.mu.RLock()
	backends := make(map[string]ModelBackend, len(r.backends))
	for k, v := range r.backends {
		backends[k] = v
	}
	r.mu.RUnlock()

	for name, backend := range backends {
		ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
		err := backend.HealthCheck(ctx)
		cancel()

		healthy := err == nil

		r.healthMu.Lock()
		oldHealth := r.healthStatus[name]
		r.healthStatus[name] = healthy
		r.lastHealth[name] = time.Now()
		r.healthMu.Unlock()

		if r.metrics != nil {
			healthValue := 0.0
			if healthy {
				healthValue = 1.0
			}
			r.metrics.backendHealth.WithLabelValues(name).Set(healthValue)
		}

		if oldHealth != healthy {
			if healthy {
				r.logger.Info("Backend became healthy", logging.F("backend", name))
			} else {
				r.logger.Warn("Backend became unhealthy",
					logging.F("backend", name),
					logging.Err(err),
				)
			}
		}
	}
}

// GetHealth returns the health status of a backend.
func (r *ModelRouter) GetHealth(backendName string) (bool, time.Time) {
	r.healthMu.RLock()
	defer r.healthMu.RUnlock()
	return r.healthStatus[backendName], r.lastHealth[backendName]
}

// GetAllHealth returns the health status of all backends.
func (r *ModelRouter) GetAllHealth() map[string]bool {
	r.healthMu.RLock()
	defer r.healthMu.RUnlock()

	result := make(map[string]bool, len(r.healthStatus))
	for k, v := range r.healthStatus {
		result[k] = v
	}
	return result
}

// GetCircuitStats returns circuit breaker statistics for a backend.
func (r *ModelRouter) GetCircuitStats(backendName string) *Stats {
	cb := r.breakers.GetExisting(backendName)
	if cb == nil {
		return nil
	}
	stats := cb.Stats()
	return &stats
}

// Shutdown gracefully shuts down the router.
func (r *ModelRouter) Shutdown(ctx context.Context) error {
	r.shutdownMu.Lock()
	r.shutdown = true
	r.shutdownMu.Unlock()

	r.cancel()

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		r.logger.Info("Router shutdown complete")
		return nil
	case <-ctx.Done():
		r.logger.Warn("Router shutdown timed out")
		return ctx.Err()
	}
}

// DefaultModelSelector implements a simple model selection strategy.
type DefaultModelSelector struct {
	// PreferLocal prefers local models over cloud when both are available.
	PreferLocal bool

	// ModelMappings maps request types to preferred models.
	ModelMappings map[RequestType]string
}

// Select chooses a backend based on the request and available backends.
func (s *DefaultModelSelector) Select(ctx context.Context, req *Request, available []string) (string, error) {
	if len(available) == 0 {
		return "", errors.New("no backends available")
	}

	// Check if there's a mapping for this request type
	if s.ModelMappings != nil {
		if preferred, ok := s.ModelMappings[req.Type]; ok {
			for _, name := range available {
				if name == preferred {
					return name, nil
				}
			}
		}
	}

	// Just return the first available
	return available[0], nil
}
