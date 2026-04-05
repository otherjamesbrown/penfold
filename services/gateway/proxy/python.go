// Package proxy provides HTTP proxying functionality for forwarding requests
// to Python services during the gradual migration to Go.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Backend types for routing decisions.
const (
	BackendPython = "python"
	BackendGo     = "go"
)

// Header keys for propagation.
const (
	HeaderAuthorization = "Authorization"
	HeaderTenantID      = "X-Tenant-ID"
	HeaderTraceID       = "X-Trace-ID"
	HeaderRequestID     = "X-Request-ID"
	HeaderContentType   = "Content-Type"
	HeaderAccept        = "Accept"
)

// ProxyConfig holds configuration for the Python proxy.
type ProxyConfig struct {
	// PythonBaseURL is the base URL of the Python service.
	PythonBaseURL string

	// GoBaseURL is the base URL of the Go service (for comparison/testing).
	GoBaseURL string

	// Timeout is the request timeout for proxied requests.
	Timeout time.Duration

	// MaxIdleConns is the maximum number of idle connections.
	MaxIdleConns int

	// MaxIdleConnsPerHost is the maximum idle connections per host.
	MaxIdleConnsPerHost int

	// IdleConnTimeout is the timeout for idle connections.
	IdleConnTimeout time.Duration

	// Logger for proxy events.
	Logger logging.Logger

	// FeatureFlags configures endpoint-specific routing behavior.
	FeatureFlags *FeatureFlags

	// FallbackEnabled enables fallback to Python on Go service errors.
	FallbackEnabled bool
}

// DefaultProxyConfig returns a ProxyConfig with sensible defaults.
func DefaultProxyConfig(pythonBaseURL string) *ProxyConfig {
	return &ProxyConfig{
		PythonBaseURL:       pythonBaseURL,
		Timeout:             30 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		FallbackEnabled:     true,
	}
}

// FeatureFlags manages per-endpoint routing configuration.
type FeatureFlags struct {
	mu sync.RWMutex

	// endpoints maps endpoint paths to their routing configuration.
	endpoints map[string]*EndpointConfig

	// defaultBackend is the default backend when no config exists.
	defaultBackend string

	// globalRolloutPercentage is the percentage of traffic to route to Go.
	globalRolloutPercentage int32
}

// EndpointConfig holds routing configuration for a specific endpoint.
type EndpointConfig struct {
	// Path is the endpoint path pattern (e.g., "/api/v1/search").
	Path string

	// Backend is the preferred backend ("python" or "go").
	Backend string

	// RolloutPercentage is the percentage of traffic to route to Go (0-100).
	// Only applies when Backend is "go".
	RolloutPercentage int32

	// Enabled controls whether this endpoint is active.
	Enabled bool
}

// NewFeatureFlags creates a new FeatureFlags instance with Python as default.
func NewFeatureFlags() *FeatureFlags {
	return &FeatureFlags{
		endpoints:               make(map[string]*EndpointConfig),
		defaultBackend:          BackendPython,
		globalRolloutPercentage: 0,
	}
}

// SetEndpoint configures routing for a specific endpoint.
func (f *FeatureFlags) SetEndpoint(path string, config *EndpointConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()
	config.Path = path
	f.endpoints[path] = config
}

// GetEndpoint returns the configuration for an endpoint.
func (f *FeatureFlags) GetEndpoint(path string) (*EndpointConfig, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	config, exists := f.endpoints[path]
	return config, exists
}

// RemoveEndpoint removes the configuration for an endpoint.
func (f *FeatureFlags) RemoveEndpoint(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.endpoints, path)
}

// SetGlobalRolloutPercentage sets the global rollout percentage for Go backend.
func (f *FeatureFlags) SetGlobalRolloutPercentage(percentage int32) {
	if percentage < 0 {
		percentage = 0
	}
	if percentage > 100 {
		percentage = 100
	}
	atomic.StoreInt32(&f.globalRolloutPercentage, percentage)
}

// GetGlobalRolloutPercentage returns the current global rollout percentage.
func (f *FeatureFlags) GetGlobalRolloutPercentage() int32 {
	return atomic.LoadInt32(&f.globalRolloutPercentage)
}

// SetDefaultBackend sets the default backend for endpoints without configuration.
func (f *FeatureFlags) SetDefaultBackend(backend string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.defaultBackend = backend
}

// GetDefaultBackend returns the default backend.
func (f *FeatureFlags) GetDefaultBackend() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.defaultBackend
}

// ShouldRouteToGo determines if a request should be routed to the Go backend.
// It considers endpoint-specific config, rollout percentage, and global settings.
func (f *FeatureFlags) ShouldRouteToGo(path string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Check endpoint-specific configuration first.
	if config, exists := f.endpoints[path]; exists && config.Enabled {
		if config.Backend == BackendPython {
			return false
		}
		if config.Backend == BackendGo {
			// Apply rollout percentage for gradual rollout.
			if config.RolloutPercentage >= 100 {
				return true
			}
			if config.RolloutPercentage <= 0 {
				return false
			}
			return rand.Int32N(100) < config.RolloutPercentage
		}
	}

	// Fall back to global rollout percentage.
	globalPct := atomic.LoadInt32(&f.globalRolloutPercentage)
	if globalPct >= 100 {
		return f.defaultBackend == BackendGo
	}
	if globalPct <= 0 {
		return false
	}

	// Apply global rollout percentage.
	if f.defaultBackend == BackendGo {
		return rand.Int32N(100) < globalPct
	}

	return false
}

// ProxyMetrics holds Prometheus metrics for the proxy.
type ProxyMetrics struct {
	// RequestsTotal counts total proxied requests by backend and status.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration observes request duration by backend.
	RequestDuration *prometheus.HistogramVec

	// FallbacksTotal counts fallback events from Go to Python.
	FallbacksTotal prometheus.Counter

	// RoutingDecisions counts routing decisions by backend.
	RoutingDecisions *prometheus.CounterVec

	// ErrorsTotal counts errors by type and backend.
	ErrorsTotal *prometheus.CounterVec

	// LatencyComparison observes latency for both backends (when shadow mode enabled).
	LatencyComparison *prometheus.HistogramVec
}

// NewProxyMetrics creates a new ProxyMetrics instance.
func NewProxyMetrics(namespace string) *ProxyMetrics {
	return &ProxyMetrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "proxy",
				Name:      "requests_total",
				Help:      "Total number of proxied requests by backend and status",
			},
			[]string{"backend", "method", "path", "status"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "proxy",
				Name:      "request_duration_seconds",
				Help:      "Request duration in seconds by backend",
				Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"backend", "method", "path"},
		),
		FallbacksTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "proxy",
				Name:      "fallbacks_total",
				Help:      "Total number of fallbacks from Go to Python",
			},
		),
		RoutingDecisions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "proxy",
				Name:      "routing_decisions_total",
				Help:      "Total routing decisions by backend",
			},
			[]string{"backend", "path"},
		),
		ErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "proxy",
				Name:      "errors_total",
				Help:      "Total errors by type and backend",
			},
			[]string{"backend", "type"},
		),
		LatencyComparison: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "proxy",
				Name:      "latency_comparison_seconds",
				Help:      "Latency comparison between backends",
				Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"backend", "method", "path"},
		),
	}
}

// Register registers all metrics with the provided registry.
func (m *ProxyMetrics) Register(reg prometheus.Registerer) error {
	collectors := []prometheus.Collector{
		m.RequestsTotal,
		m.RequestDuration,
		m.FallbacksTotal,
		m.RoutingDecisions,
		m.ErrorsTotal,
		m.LatencyComparison,
	}

	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// PythonProxy handles HTTP proxying to Python services.
type PythonProxy struct {
	config  *ProxyConfig
	client  *http.Client
	logger  logging.Logger
	metrics *ProxyMetrics

	pythonURL *url.URL
	goURL     *url.URL
}

// NewPythonProxy creates a new PythonProxy instance.
func NewPythonProxy(config *ProxyConfig) (*PythonProxy, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	if config.PythonBaseURL == "" {
		return nil, fmt.Errorf("PythonBaseURL is required")
	}

	pythonURL, err := url.Parse(config.PythonBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid PythonBaseURL: %w", err)
	}

	var goURL *url.URL
	if config.GoBaseURL != "" {
		goURL, err = url.Parse(config.GoBaseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid GoBaseURL: %w", err)
		}
	}

	logger := config.Logger
	if logger == nil {
		logger = logging.MustGlobal()
	}

	transport := &http.Transport{
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}

	if config.FeatureFlags == nil {
		config.FeatureFlags = NewFeatureFlags()
	}

	return &PythonProxy{
		config:    config,
		client:    client,
		logger:    logger,
		pythonURL: pythonURL,
		goURL:     goURL,
	}, nil
}

// SetMetrics sets the metrics collector for the proxy.
func (p *PythonProxy) SetMetrics(m *ProxyMetrics) {
	p.metrics = m
}

// ProxyResponse represents the response from a proxied request.
type ProxyResponse struct {
	// StatusCode is the HTTP status code.
	StatusCode int

	// Headers contains response headers.
	Headers http.Header

	// Body contains the response body.
	Body []byte

	// Backend indicates which backend handled the request.
	Backend string

	// Duration is the request duration.
	Duration time.Duration

	// WasFallback indicates if this was a fallback response.
	WasFallback bool
}

// ProxyRequest forwards an HTTP request to the appropriate backend.
// It handles routing decisions, header propagation, and optional fallback.
func (p *PythonProxy) ProxyRequest(
	ctx context.Context,
	method string,
	path string,
	body []byte,
) (*ProxyResponse, error) {
	return p.ProxyRequestWithHeaders(ctx, method, path, body, nil)
}

// ProxyRequestWithHeaders forwards an HTTP request with custom headers.
func (p *PythonProxy) ProxyRequestWithHeaders(
	ctx context.Context,
	method string,
	path string,
	body []byte,
	headers map[string]string,
) (*ProxyResponse, error) {
	// Determine routing.
	useGo := p.config.FeatureFlags.ShouldRouteToGo(path)
	backend := BackendPython
	if useGo && p.goURL != nil {
		backend = BackendGo
	}

	p.recordRoutingDecision(backend, path)

	// Try primary backend.
	resp, err := p.doRequest(ctx, backend, method, path, body, headers)

	// Handle fallback on Go errors if enabled.
	if err != nil && backend == BackendGo && p.config.FallbackEnabled {
		p.logger.Warn("Go backend failed, falling back to Python",
			logging.F("path", path),
			logging.F("method", method),
			logging.Err(err),
		)

		if p.metrics != nil {
			p.metrics.FallbacksTotal.Inc()
		}

		resp, err = p.doRequest(ctx, BackendPython, method, path, body, headers)
		if err == nil {
			resp.WasFallback = true
		}
	}

	return resp, err
}

// ProxyRequestAsync forwards a request asynchronously and returns a channel.
// This is useful for non-blocking proxy operations.
func (p *PythonProxy) ProxyRequestAsync(
	ctx context.Context,
	method string,
	path string,
	body []byte,
) <-chan *AsyncProxyResult {
	resultCh := make(chan *AsyncProxyResult, 1)

	go func() {
		resp, err := p.ProxyRequest(ctx, method, path, body)
		resultCh <- &AsyncProxyResult{Response: resp, Error: err}
		close(resultCh)
	}()

	return resultCh
}

// AsyncProxyResult holds the result of an async proxy request.
type AsyncProxyResult struct {
	Response *ProxyResponse
	Error    error
}

// doRequest performs the actual HTTP request to a backend.
func (p *PythonProxy) doRequest(
	ctx context.Context,
	backend string,
	method string,
	path string,
	body []byte,
	headers map[string]string,
) (*ProxyResponse, error) {
	// Determine target URL.
	var baseURL *url.URL
	switch backend {
	case BackendGo:
		if p.goURL == nil {
			return nil, fmt.Errorf("Go backend URL not configured")
		}
		baseURL = p.goURL
	default:
		baseURL = p.pythonURL
	}

	targetURL := baseURL.JoinPath(path).String()

	// Create request.
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL, bodyReader)
	if err != nil {
		p.recordError(backend, "request_creation")
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set headers.
	p.propagateHeaders(ctx, req, headers)

	// Execute request.
	start := time.Now()
	resp, err := p.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		p.recordError(backend, "request_execution")
		p.recordDuration(backend, method, path, duration)
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// Read response body.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		p.recordError(backend, "response_read")
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Record metrics.
	p.recordRequest(backend, method, path, resp.StatusCode, duration)

	return &ProxyResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
		Backend:    backend,
		Duration:   duration,
	}, nil
}

// propagateHeaders copies relevant headers to the outgoing request.
func (p *PythonProxy) propagateHeaders(ctx context.Context, req *http.Request, headers map[string]string) {
	// Set default content type for JSON APIs.
	req.Header.Set(HeaderContentType, "application/json")
	req.Header.Set(HeaderAccept, "application/json")

	// Propagate custom headers.
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Propagate context values as headers.
	if tenantID := getContextString(ctx, "tenant_id"); tenantID != "" {
		req.Header.Set(HeaderTenantID, tenantID)
	}
	if traceID := getContextString(ctx, "trace_id"); traceID != "" {
		req.Header.Set(HeaderTraceID, traceID)
	}
	if requestID := getContextString(ctx, "request_id"); requestID != "" {
		req.Header.Set(HeaderRequestID, requestID)
	}
}

// getContextString extracts a string value from context.
func getContextString(ctx context.Context, key string) string {
	if v := ctx.Value(key); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// recordRoutingDecision records a routing decision in metrics.
func (p *PythonProxy) recordRoutingDecision(backend, path string) {
	if p.metrics != nil {
		p.metrics.RoutingDecisions.WithLabelValues(backend, path).Inc()
	}
	p.logger.Debug("Routing decision",
		logging.F("backend", backend),
		logging.F("path", path),
	)
}

// recordRequest records request metrics.
func (p *PythonProxy) recordRequest(backend, method, path string, statusCode int, duration time.Duration) {
	if p.metrics == nil {
		return
	}

	status := fmt.Sprintf("%d", statusCode)
	p.metrics.RequestsTotal.WithLabelValues(backend, method, path, status).Inc()
	p.metrics.RequestDuration.WithLabelValues(backend, method, path).Observe(duration.Seconds())
}

// recordDuration records just the duration (for error cases).
func (p *PythonProxy) recordDuration(backend, method, path string, duration time.Duration) {
	if p.metrics != nil {
		p.metrics.RequestDuration.WithLabelValues(backend, method, path).Observe(duration.Seconds())
	}
}

// recordError records an error in metrics.
func (p *PythonProxy) recordError(backend, errorType string) {
	if p.metrics != nil {
		p.metrics.ErrorsTotal.WithLabelValues(backend, errorType).Inc()
	}
}

// ProxyHTTPHandler creates an http.Handler that proxies requests to Python.
// This can be used directly with an HTTP server.
func (p *PythonProxy) ProxyHTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read request body.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close() //nolint:errcheck

		// Extract headers.
		headers := make(map[string]string)
		for _, key := range []string{HeaderAuthorization, HeaderTenantID, HeaderTraceID, HeaderRequestID} {
			if v := r.Header.Get(key); v != "" {
				headers[key] = v
			}
		}

		// Proxy the request.
		resp, err := p.ProxyRequestWithHeaders(r.Context(), r.Method, r.URL.Path, body, headers)
		if err != nil {
			p.logger.Error("Proxy request failed",
				logging.F("path", r.URL.Path),
				logging.F("method", r.Method),
				logging.Err(err),
			)
			http.Error(w, "Proxy error", http.StatusBadGateway)
			return
		}

		// Copy response headers.
		for key, values := range resp.Headers {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}

		// Write response.
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(resp.Body)
	})
}

// ServiceDiscovery manages dynamic discovery of Python services.
type ServiceDiscovery struct {
	mu       sync.RWMutex
	services map[string]*ServiceEndpoint
	logger   logging.Logger
}

// ServiceEndpoint represents a discovered Python service.
type ServiceEndpoint struct {
	Name      string
	URL       string
	Health    bool
	LastCheck time.Time
	Weight    int
}

// NewServiceDiscovery creates a new ServiceDiscovery instance.
func NewServiceDiscovery(logger logging.Logger) *ServiceDiscovery {
	if logger == nil {
		logger = logging.MustGlobal()
	}
	return &ServiceDiscovery{
		services: make(map[string]*ServiceEndpoint),
		logger:   logger,
	}
}

// Register registers a Python service endpoint.
func (sd *ServiceDiscovery) Register(name, serviceURL string) error {
	_, err := url.Parse(serviceURL)
	if err != nil {
		return fmt.Errorf("invalid service URL: %w", err)
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	sd.services[name] = &ServiceEndpoint{
		Name:      name,
		URL:       serviceURL,
		Health:    true,
		LastCheck: time.Now(),
		Weight:    1,
	}

	sd.logger.Info("Service registered",
		logging.F("name", name),
		logging.F("url", serviceURL),
	)

	return nil
}

// Deregister removes a service from discovery.
func (sd *ServiceDiscovery) Deregister(name string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	delete(sd.services, name)

	sd.logger.Info("Service deregistered", logging.F("name", name))
}

// Get returns a service endpoint by name.
func (sd *ServiceDiscovery) Get(name string) (*ServiceEndpoint, bool) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	svc, exists := sd.services[name]
	return svc, exists
}

// GetHealthy returns all healthy service endpoints.
func (sd *ServiceDiscovery) GetHealthy() []*ServiceEndpoint {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	healthy := make([]*ServiceEndpoint, 0)
	for _, svc := range sd.services {
		if svc.Health {
			healthy = append(healthy, svc)
		}
	}
	return healthy
}

// SetHealth updates the health status of a service.
func (sd *ServiceDiscovery) SetHealth(name string, healthy bool) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	if svc, exists := sd.services[name]; exists {
		svc.Health = healthy
		svc.LastCheck = time.Now()
	}
}

// List returns all registered services.
func (sd *ServiceDiscovery) List() []*ServiceEndpoint {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	services := make([]*ServiceEndpoint, 0, len(sd.services))
	for _, svc := range sd.services {
		services = append(services, svc)
	}
	return services
}

// TransformResponse provides utilities for transforming responses between formats.
type TransformResponse struct{}

// JSONToJSON transforms JSON response with a mapping function.
func (t *TransformResponse) JSONToJSON(body []byte, transform func(map[string]any) map[string]any) ([]byte, error) {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	transformed := transform(data)

	result, err := json.Marshal(transformed)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	return result, nil
}

// WrapInEnvelope wraps a response in a standard envelope.
func (t *TransformResponse) WrapInEnvelope(body []byte, envelope string) ([]byte, error) {
	wrapped := map[string]json.RawMessage{
		envelope: body,
	}
	return json.Marshal(wrapped)
}

// ExtractField extracts a field from a JSON response.
func (t *TransformResponse) ExtractField(body []byte, field string) ([]byte, error) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	extracted, exists := data[field]
	if !exists {
		return nil, fmt.Errorf("field %q not found", field)
	}

	return extracted, nil
}
