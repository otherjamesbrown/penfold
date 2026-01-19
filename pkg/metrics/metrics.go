// Package metrics provides shared Prometheus metrics functionality for Go microservices.
// It offers a standard set of metrics (requests, duration, connections, errors) along
// with HTTP handlers for exposing metrics and middleware for automatic instrumentation.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds common Prometheus metrics for a microservice.
type Metrics struct {
	// RequestsTotal counts total HTTP/RPC requests by method, path, and status.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration observes request duration in seconds by method and path.
	RequestDuration *prometheus.HistogramVec

	// ActiveConnections tracks the current number of active connections.
	ActiveConnections prometheus.Gauge

	// ErrorsTotal counts errors by type.
	ErrorsTotal *prometheus.CounterVec

	// ServiceName identifies the service these metrics belong to.
	ServiceName string

	// Namespace is the Prometheus metrics namespace.
	Namespace string

	// registry holds the Prometheus registry for this metrics instance.
	registry *prometheus.Registry
}

// Config holds configuration for creating metrics.
type Config struct {
	// ServiceName identifies the service in metrics labels.
	ServiceName string

	// Namespace is the Prometheus metrics namespace (prefix).
	Namespace string

	// Buckets defines custom histogram buckets for request duration.
	// If nil, default buckets are used.
	Buckets []float64
}

// DefaultBuckets returns default histogram buckets for request duration (in seconds).
func DefaultBuckets() []float64 {
	return []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
}

// NewMetrics creates a new Metrics instance with standard metrics for a service.
// The serviceName and namespace are used to identify and prefix metrics.
func NewMetrics(serviceName, namespace string) *Metrics {
	return NewMetricsWithConfig(&Config{
		ServiceName: serviceName,
		Namespace:   namespace,
		Buckets:     DefaultBuckets(),
	})
}

// NewMetricsWithConfig creates a new Metrics instance with custom configuration.
func NewMetricsWithConfig(cfg *Config) *Metrics {
	if cfg == nil {
		cfg = &Config{
			ServiceName: "unknown",
			Namespace:   "app",
			Buckets:     DefaultBuckets(),
		}
	}

	if cfg.Buckets == nil {
		cfg.Buckets = DefaultBuckets()
	}

	m := &Metrics{
		ServiceName: cfg.ServiceName,
		Namespace:   cfg.Namespace,
		registry:    prometheus.NewRegistry(),
	}

	// requests_total: Counter for total requests
	m.RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "requests_total",
			Help:      "Total number of requests by method, path, and status",
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
		[]string{"method", "path", "status"},
	)

	// request_duration_seconds: Histogram for request duration
	m.RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Name:      "request_duration_seconds",
			Help:      "Request duration in seconds by method and path",
			Buckets:   cfg.Buckets,
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
		[]string{"method", "path"},
	)

	// active_connections: Gauge for active connections
	m.ActiveConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Name:      "active_connections",
			Help:      "Current number of active connections",
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
	)

	// errors_total: Counter for errors by type
	m.ErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "errors_total",
			Help:      "Total number of errors by type",
			ConstLabels: prometheus.Labels{
				"service": cfg.ServiceName,
			},
		},
		[]string{"type"},
	)

	return m
}

// RegisterMetrics registers all metrics with the default Prometheus registry.
// This should be called once during service initialization.
func (m *Metrics) RegisterMetrics() error {
	collectors := []prometheus.Collector{
		m.RequestsTotal,
		m.RequestDuration,
		m.ActiveConnections,
		m.ErrorsTotal,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			// If already registered, that's okay
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// RegisterWithRegistry registers all metrics with the provided registry.
// Use this for custom registries or testing.
func (m *Metrics) RegisterWithRegistry(reg *prometheus.Registry) error {
	collectors := []prometheus.Collector{
		m.RequestsTotal,
		m.RequestDuration,
		m.ActiveConnections,
		m.ErrorsTotal,
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

// Handler returns an HTTP handler for the /metrics endpoint.
// This uses the default Prometheus registry.
func Handler() http.Handler {
	return promhttp.Handler()
}

// HandlerFor returns an HTTP handler for a specific registry.
// Use this if you're using a custom registry.
func HandlerFor(reg prometheus.Gatherer) http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// RecordRequest is a convenience method to record a request with all metrics.
// It increments requests_total and observes request_duration.
func (m *Metrics) RecordRequest(method, path, status string, durationSeconds float64) {
	m.RequestsTotal.WithLabelValues(method, path, status).Inc()
	m.RequestDuration.WithLabelValues(method, path).Observe(durationSeconds)
}

// RecordError is a convenience method to increment the errors_total counter.
func (m *Metrics) RecordError(errorType string) {
	m.ErrorsTotal.WithLabelValues(errorType).Inc()
}

// IncrementConnections increments the active connections gauge.
func (m *Metrics) IncrementConnections() {
	m.ActiveConnections.Inc()
}

// DecrementConnections decrements the active connections gauge.
func (m *Metrics) DecrementConnections() {
	m.ActiveConnections.Dec()
}
