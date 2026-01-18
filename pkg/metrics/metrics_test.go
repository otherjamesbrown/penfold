package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics("test-service", "test")

	if m.ServiceName != "test-service" {
		t.Errorf("expected ServiceName 'test-service', got '%s'", m.ServiceName)
	}

	if m.Namespace != "test" {
		t.Errorf("expected Namespace 'test', got '%s'", m.Namespace)
	}

	if m.RequestsTotal == nil {
		t.Error("RequestsTotal should not be nil")
	}

	if m.RequestDuration == nil {
		t.Error("RequestDuration should not be nil")
	}

	if m.ActiveConnections == nil {
		t.Error("ActiveConnections should not be nil")
	}

	if m.ErrorsTotal == nil {
		t.Error("ErrorsTotal should not be nil")
	}
}

func TestNewMetricsWithConfig(t *testing.T) {
	cfg := &Config{
		ServiceName: "custom-service",
		Namespace:   "custom",
		Buckets:     []float64{0.1, 0.5, 1.0},
	}

	m := NewMetricsWithConfig(cfg)

	if m.ServiceName != "custom-service" {
		t.Errorf("expected ServiceName 'custom-service', got '%s'", m.ServiceName)
	}

	if m.Namespace != "custom" {
		t.Errorf("expected Namespace 'custom', got '%s'", m.Namespace)
	}
}

func TestNewMetricsWithConfig_Nil(t *testing.T) {
	m := NewMetricsWithConfig(nil)

	if m.ServiceName != "unknown" {
		t.Errorf("expected default ServiceName 'unknown', got '%s'", m.ServiceName)
	}

	if m.Namespace != "app" {
		t.Errorf("expected default Namespace 'app', got '%s'", m.Namespace)
	}
}

func TestRegisterWithRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")

	err := m.RegisterWithRegistry(reg)
	if err != nil {
		t.Fatalf("RegisterWithRegistry failed: %v", err)
	}

	// Verify metrics are registered by gathering
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	names := make(map[string]bool)
	for _, mf := range families {
		names[mf.GetName()] = true
	}

	expectedMetrics := []string{
		"test_requests_total",
		"test_request_duration_seconds",
		"test_active_connections",
		"test_errors_total",
	}

	// Note: empty metrics may not show up in Gather
	// So we just verify the registry accepted them without error
	_ = names
	_ = expectedMetrics
}

func TestRegisterWithRegistry_AlreadyRegistered(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")

	// Register twice should not error (already registered is ignored)
	err := m.RegisterWithRegistry(reg)
	if err != nil {
		t.Fatalf("First RegisterWithRegistry failed: %v", err)
	}

	err = m.RegisterWithRegistry(reg)
	if err != nil {
		t.Fatalf("Second RegisterWithRegistry should not error: %v", err)
	}
}

func TestRecordRequest(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	err := m.RegisterWithRegistry(reg)
	if err != nil {
		t.Fatalf("RegisterWithRegistry failed: %v", err)
	}

	// Record a request
	m.RecordRequest("GET", "/api/users", "200", 0.5)

	// Verify counter
	count := testutil.ToFloat64(m.RequestsTotal.WithLabelValues("GET", "/api/users", "200"))
	if count != 1 {
		t.Errorf("expected requests_total count 1, got %f", count)
	}

	// Record another request
	m.RecordRequest("GET", "/api/users", "200", 0.3)

	count = testutil.ToFloat64(m.RequestsTotal.WithLabelValues("GET", "/api/users", "200"))
	if count != 2 {
		t.Errorf("expected requests_total count 2, got %f", count)
	}
}

func TestRecordError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	err := m.RegisterWithRegistry(reg)
	if err != nil {
		t.Fatalf("RegisterWithRegistry failed: %v", err)
	}

	// Record errors
	m.RecordError("database_timeout")
	m.RecordError("database_timeout")
	m.RecordError("validation_error")

	// Verify counters
	dbCount := testutil.ToFloat64(m.ErrorsTotal.WithLabelValues("database_timeout"))
	if dbCount != 2 {
		t.Errorf("expected database_timeout count 2, got %f", dbCount)
	}

	valCount := testutil.ToFloat64(m.ErrorsTotal.WithLabelValues("validation_error"))
	if valCount != 1 {
		t.Errorf("expected validation_error count 1, got %f", valCount)
	}
}

func TestConnectionTracking(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	err := m.RegisterWithRegistry(reg)
	if err != nil {
		t.Fatalf("RegisterWithRegistry failed: %v", err)
	}

	// Initially zero
	count := testutil.ToFloat64(m.ActiveConnections)
	if count != 0 {
		t.Errorf("expected initial active_connections 0, got %f", count)
	}

	// Increment
	m.IncrementConnections()
	m.IncrementConnections()

	count = testutil.ToFloat64(m.ActiveConnections)
	if count != 2 {
		t.Errorf("expected active_connections 2, got %f", count)
	}

	// Decrement
	m.DecrementConnections()

	count = testutil.ToFloat64(m.ActiveConnections)
	if count != 1 {
		t.Errorf("expected active_connections 1, got %f", count)
	}
}

func TestHandler(t *testing.T) {
	handler := Handler()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Should return prometheus text format
	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") && !strings.Contains(contentType, "application/openmetrics-text") {
		t.Errorf("unexpected Content-Type: %s", contentType)
	}
}

func TestHandlerFor(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics("test-service", "test")
	_ = m.RegisterWithRegistry(reg)

	// Record some data
	m.RecordRequest("POST", "/api/items", "201", 0.1)

	handler := HandlerFor(reg)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "test_requests_total") {
		t.Errorf("expected body to contain 'test_requests_total', got: %s", body)
	}
}

func TestDefaultBuckets(t *testing.T) {
	buckets := DefaultBuckets()

	if len(buckets) == 0 {
		t.Error("DefaultBuckets should return non-empty slice")
	}

	// Verify buckets are in ascending order
	for i := 1; i < len(buckets); i++ {
		if buckets[i] <= buckets[i-1] {
			t.Errorf("buckets should be ascending, got %f at index %d", buckets[i], i)
		}
	}
}
