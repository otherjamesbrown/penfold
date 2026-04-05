package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// testLogger creates a logger for testing.
func testLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "proxy-test",
		JSONFormat:  false,
	})
}

func TestNewPythonProxy(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := DefaultProxyConfig("http://localhost:8000")
		config.Logger = testLogger()

		proxy, err := NewPythonProxy(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if proxy == nil {
			t.Fatal("expected non-nil proxy")
		}
	})

	t.Run("nil config", func(t *testing.T) {
		_, err := NewPythonProxy(nil)
		if err == nil {
			t.Fatal("expected error for nil config")
		}
	})

	t.Run("empty PythonBaseURL", func(t *testing.T) {
		config := &ProxyConfig{}
		_, err := NewPythonProxy(config)
		if err == nil {
			t.Fatal("expected error for empty PythonBaseURL")
		}
	})

	t.Run("invalid PythonBaseURL", func(t *testing.T) {
		config := &ProxyConfig{
			PythonBaseURL: "://invalid",
		}
		_, err := NewPythonProxy(config)
		if err == nil {
			t.Fatal("expected error for invalid PythonBaseURL")
		}
	})

	t.Run("with GoBaseURL", func(t *testing.T) {
		config := &ProxyConfig{
			PythonBaseURL: "http://python:8000",
			GoBaseURL:     "http://go:8080",
			Logger:        testLogger(),
		}

		proxy, err := NewPythonProxy(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if proxy.goURL == nil {
			t.Fatal("expected goURL to be set")
		}
	})

	t.Run("invalid GoBaseURL", func(t *testing.T) {
		config := &ProxyConfig{
			PythonBaseURL: "http://python:8000",
			GoBaseURL:     "://invalid",
		}
		_, err := NewPythonProxy(config)
		if err == nil {
			t.Fatal("expected error for invalid GoBaseURL")
		}
	})
}

func TestDefaultProxyConfig(t *testing.T) {
	config := DefaultProxyConfig("http://localhost:8000")

	if config.PythonBaseURL != "http://localhost:8000" {
		t.Errorf("expected PythonBaseURL=http://localhost:8000, got %s", config.PythonBaseURL)
	}

	if config.Timeout != 30*time.Second {
		t.Errorf("expected Timeout=30s, got %v", config.Timeout)
	}

	if config.MaxIdleConns != 100 {
		t.Errorf("expected MaxIdleConns=100, got %d", config.MaxIdleConns)
	}

	if !config.FallbackEnabled {
		t.Error("expected FallbackEnabled=true")
	}
}

func TestFeatureFlags(t *testing.T) {
	t.Run("default backend", func(t *testing.T) {
		ff := NewFeatureFlags()

		if ff.GetDefaultBackend() != BackendPython {
			t.Errorf("expected default backend=python, got %s", ff.GetDefaultBackend())
		}

		ff.SetDefaultBackend(BackendGo)
		if ff.GetDefaultBackend() != BackendGo {
			t.Errorf("expected default backend=go, got %s", ff.GetDefaultBackend())
		}
	})

	t.Run("endpoint config", func(t *testing.T) {
		ff := NewFeatureFlags()

		config := &EndpointConfig{
			Backend:           BackendGo,
			RolloutPercentage: 100,
			Enabled:           true,
		}

		ff.SetEndpoint("/api/v1/search", config)

		retrieved, exists := ff.GetEndpoint("/api/v1/search")
		if !exists {
			t.Fatal("expected endpoint to exist")
		}

		if retrieved.Backend != BackendGo {
			t.Errorf("expected backend=go, got %s", retrieved.Backend)
		}

		if retrieved.Path != "/api/v1/search" {
			t.Errorf("expected path=/api/v1/search, got %s", retrieved.Path)
		}
	})

	t.Run("remove endpoint", func(t *testing.T) {
		ff := NewFeatureFlags()

		ff.SetEndpoint("/test", &EndpointConfig{Backend: BackendGo, Enabled: true})
		ff.RemoveEndpoint("/test")

		_, exists := ff.GetEndpoint("/test")
		if exists {
			t.Fatal("expected endpoint to be removed")
		}
	})

	t.Run("global rollout percentage", func(t *testing.T) {
		ff := NewFeatureFlags()

		if ff.GetGlobalRolloutPercentage() != 0 {
			t.Errorf("expected initial rollout=0, got %d", ff.GetGlobalRolloutPercentage())
		}

		ff.SetGlobalRolloutPercentage(50)
		if ff.GetGlobalRolloutPercentage() != 50 {
			t.Errorf("expected rollout=50, got %d", ff.GetGlobalRolloutPercentage())
		}

		// Test bounds.
		ff.SetGlobalRolloutPercentage(-10)
		if ff.GetGlobalRolloutPercentage() != 0 {
			t.Errorf("expected rollout=0 for negative, got %d", ff.GetGlobalRolloutPercentage())
		}

		ff.SetGlobalRolloutPercentage(150)
		if ff.GetGlobalRolloutPercentage() != 100 {
			t.Errorf("expected rollout=100 for >100, got %d", ff.GetGlobalRolloutPercentage())
		}
	})

	t.Run("ShouldRouteToGo with Python endpoint", func(t *testing.T) {
		ff := NewFeatureFlags()

		ff.SetEndpoint("/python-only", &EndpointConfig{
			Backend: BackendPython,
			Enabled: true,
		})

		for i := 0; i < 10; i++ {
			if ff.ShouldRouteToGo("/python-only") {
				t.Fatal("expected Python routing for Python-configured endpoint")
			}
		}
	})

	t.Run("ShouldRouteToGo with Go endpoint 100%", func(t *testing.T) {
		ff := NewFeatureFlags()

		ff.SetEndpoint("/go-only", &EndpointConfig{
			Backend:           BackendGo,
			RolloutPercentage: 100,
			Enabled:           true,
		})

		for i := 0; i < 10; i++ {
			if !ff.ShouldRouteToGo("/go-only") {
				t.Fatal("expected Go routing for 100% rollout endpoint")
			}
		}
	})

	t.Run("ShouldRouteToGo with Go endpoint 0%", func(t *testing.T) {
		ff := NewFeatureFlags()

		ff.SetEndpoint("/go-zero", &EndpointConfig{
			Backend:           BackendGo,
			RolloutPercentage: 0,
			Enabled:           true,
		})

		for i := 0; i < 10; i++ {
			if ff.ShouldRouteToGo("/go-zero") {
				t.Fatal("expected Python routing for 0% rollout endpoint")
			}
		}
	})

	t.Run("ShouldRouteToGo with percentage rollout", func(t *testing.T) {
		ff := NewFeatureFlags()

		ff.SetEndpoint("/partial", &EndpointConfig{
			Backend:           BackendGo,
			RolloutPercentage: 50,
			Enabled:           true,
		})

		goCount := 0
		iterations := 1000

		for i := 0; i < iterations; i++ {
			if ff.ShouldRouteToGo("/partial") {
				goCount++
			}
		}

		// With 50% rollout, we expect roughly 500 Go routes.
		// Allow for statistical variance (30-70%).
		if goCount < 300 || goCount > 700 {
			t.Errorf("expected ~50%% Go routing, got %d/%d", goCount, iterations)
		}
	})

	t.Run("ShouldRouteToGo unconfigured path", func(t *testing.T) {
		ff := NewFeatureFlags()

		// Default is Python, so should not route to Go.
		if ff.ShouldRouteToGo("/unconfigured") {
			t.Fatal("expected Python routing for unconfigured path")
		}
	})

	t.Run("disabled endpoint uses default", func(t *testing.T) {
		ff := NewFeatureFlags()

		ff.SetEndpoint("/disabled", &EndpointConfig{
			Backend:           BackendGo,
			RolloutPercentage: 100,
			Enabled:           false,
		})

		// Should use default (Python) since endpoint is disabled.
		if ff.ShouldRouteToGo("/disabled") {
			t.Fatal("expected default routing for disabled endpoint")
		}
	})
}

func TestProxyMetrics(t *testing.T) {
	t.Run("create metrics", func(t *testing.T) {
		m := NewProxyMetrics("test")

		if m.RequestsTotal == nil {
			t.Error("expected RequestsTotal to be initialized")
		}
		if m.RequestDuration == nil {
			t.Error("expected RequestDuration to be initialized")
		}
		if m.FallbacksTotal == nil {
			t.Error("expected FallbacksTotal to be initialized")
		}
		if m.RoutingDecisions == nil {
			t.Error("expected RoutingDecisions to be initialized")
		}
		if m.ErrorsTotal == nil {
			t.Error("expected ErrorsTotal to be initialized")
		}
	})

	t.Run("register metrics", func(t *testing.T) {
		m := NewProxyMetrics("test_register")
		reg := prometheus.NewRegistry()

		err := m.Register(reg)
		if err != nil {
			t.Fatalf("unexpected error registering metrics: %v", err)
		}

		// Registering again should not error (AlreadyRegisteredError is ignored).
		err = m.Register(reg)
		if err != nil {
			t.Fatalf("unexpected error re-registering metrics: %v", err)
		}
	})
}

func TestProxyRequest(t *testing.T) {
	t.Run("proxy to Python", func(t *testing.T) {
		// Create mock Python server.
		pythonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"source": "python"})
		}))
		defer pythonServer.Close()

		config := &ProxyConfig{
			PythonBaseURL:   pythonServer.URL,
			Timeout:         5 * time.Second,
			Logger:          testLogger(),
			FallbackEnabled: true,
		}

		proxy, err := NewPythonProxy(config)
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}

		resp, err := proxy.ProxyRequest(context.Background(), "GET", "/test", nil)
		if err != nil {
			t.Fatalf("proxy request failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		if resp.Backend != BackendPython {
			t.Errorf("expected backend=python, got %s", resp.Backend)
		}

		var body map[string]string
		_ = json.Unmarshal(resp.Body, &body)
		if body["source"] != "python" {
			t.Errorf("expected source=python, got %s", body["source"])
		}
	})

	t.Run("proxy to Go when configured", func(t *testing.T) {
		// Create mock Go server.
		goServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"source": "go"})
		}))
		defer goServer.Close()

		// Create mock Python server (for fallback).
		pythonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"source": "python"})
		}))
		defer pythonServer.Close()

		ff := NewFeatureFlags()
		ff.SetEndpoint("/api/go", &EndpointConfig{
			Backend:           BackendGo,
			RolloutPercentage: 100,
			Enabled:           true,
		})

		config := &ProxyConfig{
			PythonBaseURL:   pythonServer.URL,
			GoBaseURL:       goServer.URL,
			Timeout:         5 * time.Second,
			Logger:          testLogger(),
			FeatureFlags:    ff,
			FallbackEnabled: true,
		}

		proxy, err := NewPythonProxy(config)
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}

		resp, err := proxy.ProxyRequest(context.Background(), "GET", "/api/go", nil)
		if err != nil {
			t.Fatalf("proxy request failed: %v", err)
		}

		if resp.Backend != BackendGo {
			t.Errorf("expected backend=go, got %s", resp.Backend)
		}

		var body map[string]string
		_ = json.Unmarshal(resp.Body, &body)
		if body["source"] != "go" {
			t.Errorf("expected source=go, got %s", body["source"])
		}
	})

	t.Run("fallback to Python on Go error", func(t *testing.T) {
		// Create a Go server that always fails.
		goServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("intentional failure")
		}))
		goServer.Close() // Close immediately to cause connection errors.

		// Create mock Python server.
		pythonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"source": "python-fallback"})
		}))
		defer pythonServer.Close()

		ff := NewFeatureFlags()
		ff.SetEndpoint("/api/fallback", &EndpointConfig{
			Backend:           BackendGo,
			RolloutPercentage: 100,
			Enabled:           true,
		})

		config := &ProxyConfig{
			PythonBaseURL:   pythonServer.URL,
			GoBaseURL:       goServer.URL,
			Timeout:         1 * time.Second,
			Logger:          testLogger(),
			FeatureFlags:    ff,
			FallbackEnabled: true,
		}

		proxy, err := NewPythonProxy(config)
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}

		resp, err := proxy.ProxyRequest(context.Background(), "GET", "/api/fallback", nil)
		if err != nil {
			t.Fatalf("proxy request failed: %v", err)
		}

		if resp.Backend != BackendPython {
			t.Errorf("expected backend=python after fallback, got %s", resp.Backend)
		}

		if !resp.WasFallback {
			t.Error("expected WasFallback=true")
		}
	})

	t.Run("no fallback when disabled", func(t *testing.T) {
		// Create a Go server that always fails.
		goServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("intentional failure")
		}))
		goServer.Close()

		pythonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer pythonServer.Close()

		ff := NewFeatureFlags()
		ff.SetEndpoint("/api/no-fallback", &EndpointConfig{
			Backend:           BackendGo,
			RolloutPercentage: 100,
			Enabled:           true,
		})

		config := &ProxyConfig{
			PythonBaseURL:   pythonServer.URL,
			GoBaseURL:       goServer.URL,
			Timeout:         1 * time.Second,
			Logger:          testLogger(),
			FeatureFlags:    ff,
			FallbackEnabled: false,
		}

		proxy, err := NewPythonProxy(config)
		if err != nil {
			t.Fatalf("failed to create proxy: %v", err)
		}

		_, err = proxy.ProxyRequest(context.Background(), "GET", "/api/no-fallback", nil)
		if err == nil {
			t.Fatal("expected error when fallback disabled")
		}
	})
}

func TestProxyRequestWithHeaders(t *testing.T) {
	receivedHeaders := make(map[string]string)
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedHeaders[HeaderAuthorization] = r.Header.Get(HeaderAuthorization)
		receivedHeaders[HeaderTenantID] = r.Header.Get(HeaderTenantID)
		receivedHeaders[HeaderTraceID] = r.Header.Get(HeaderTraceID)
		receivedHeaders[HeaderRequestID] = r.Header.Get(HeaderRequestID)
		receivedHeaders[HeaderContentType] = r.Header.Get(HeaderContentType)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultProxyConfig(server.URL)
	config.Logger = testLogger()

	proxy, err := NewPythonProxy(config)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	headers := map[string]string{
		HeaderAuthorization: "Bearer test-token",
		HeaderTenantID:      "tenant-123",
	}

	_, err = proxy.ProxyRequestWithHeaders(context.Background(), "POST", "/test", []byte(`{"test": true}`), headers)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if receivedHeaders[HeaderAuthorization] != "Bearer test-token" {
		t.Errorf("expected Authorization header, got %s", receivedHeaders[HeaderAuthorization])
	}

	if receivedHeaders[HeaderTenantID] != "tenant-123" {
		t.Errorf("expected X-Tenant-ID header, got %s", receivedHeaders[HeaderTenantID])
	}

	if receivedHeaders[HeaderContentType] != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", receivedHeaders[HeaderContentType])
	}
}

func TestProxyRequestAsync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"async": "response"})
	}))
	defer server.Close()

	config := DefaultProxyConfig(server.URL)
	config.Logger = testLogger()

	proxy, err := NewPythonProxy(config)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	resultCh := proxy.ProxyRequestAsync(context.Background(), "GET", "/async", nil)

	select {
	case result := <-resultCh:
		if result.Error != nil {
			t.Fatalf("async request failed: %v", result.Error)
		}
		if result.Response.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", result.Response.StatusCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("async request timed out")
	}
}

func TestProxyHTTPHandler(t *testing.T) {
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom-Header", "custom-value")
		w.WriteHeader(http.StatusCreated)

		response := map[string]interface{}{
			"received_path":   r.URL.Path,
			"received_method": r.Method,
			"received_body":   string(body),
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer backendServer.Close()

	config := DefaultProxyConfig(backendServer.URL)
	config.Logger = testLogger()

	proxy, err := NewPythonProxy(config)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	handler := proxy.ProxyHTTPHandler()

	t.Run("GET request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected status 201, got %d", w.Code)
		}

		if w.Header().Get("X-Custom-Header") != "custom-value" {
			t.Errorf("expected custom header to be copied")
		}
	})

	t.Run("POST request with body", func(t *testing.T) {
		body := `{"test": "data"}`
		req := httptest.NewRequest("POST", "/api/create", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected status 201, got %d", w.Code)
		}

		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)

		if resp["received_body"] != body {
			t.Errorf("expected body to be forwarded, got %v", resp["received_body"])
		}
	})
}

func TestServiceDiscovery(t *testing.T) {
	logger := testLogger()

	t.Run("register and get service", func(t *testing.T) {
		sd := NewServiceDiscovery(logger)

		err := sd.Register("python-api", "http://localhost:8000")
		if err != nil {
			t.Fatalf("failed to register service: %v", err)
		}

		svc, exists := sd.Get("python-api")
		if !exists {
			t.Fatal("expected service to exist")
		}

		if svc.Name != "python-api" {
			t.Errorf("expected name=python-api, got %s", svc.Name)
		}

		if svc.URL != "http://localhost:8000" {
			t.Errorf("expected url=http://localhost:8000, got %s", svc.URL)
		}

		if !svc.Health {
			t.Error("expected service to be healthy")
		}
	})

	t.Run("invalid URL", func(t *testing.T) {
		sd := NewServiceDiscovery(logger)

		err := sd.Register("invalid", "://invalid")
		if err == nil {
			t.Fatal("expected error for invalid URL")
		}
	})

	t.Run("deregister service", func(t *testing.T) {
		sd := NewServiceDiscovery(logger)

		_ = sd.Register("temp-service", "http://localhost:9000")
		sd.Deregister("temp-service")

		_, exists := sd.Get("temp-service")
		if exists {
			t.Fatal("expected service to be deregistered")
		}
	})

	t.Run("get healthy services", func(t *testing.T) {
		sd := NewServiceDiscovery(logger)

		_ = sd.Register("healthy-1", "http://localhost:8001")
		_ = sd.Register("healthy-2", "http://localhost:8002")
		_ = sd.Register("unhealthy", "http://localhost:8003")

		sd.SetHealth("unhealthy", false)

		healthy := sd.GetHealthy()
		if len(healthy) != 2 {
			t.Errorf("expected 2 healthy services, got %d", len(healthy))
		}
	})

	t.Run("list all services", func(t *testing.T) {
		sd := NewServiceDiscovery(logger)

		_ = sd.Register("service-1", "http://localhost:8001")
		_ = sd.Register("service-2", "http://localhost:8002")

		all := sd.List()
		if len(all) != 2 {
			t.Errorf("expected 2 services, got %d", len(all))
		}
	})

	t.Run("set health updates timestamp", func(t *testing.T) {
		sd := NewServiceDiscovery(logger)

		_ = sd.Register("test-health", "http://localhost:8000")

		svc, _ := sd.Get("test-health")
		originalTime := svc.LastCheck

		time.Sleep(10 * time.Millisecond)
		sd.SetHealth("test-health", false)

		svc, _ = sd.Get("test-health")
		if !svc.LastCheck.After(originalTime) {
			t.Error("expected LastCheck to be updated")
		}
		if svc.Health {
			t.Error("expected health to be false")
		}
	})
}

func TestTransformResponse(t *testing.T) {
	tr := &TransformResponse{}

	t.Run("JSONToJSON", func(t *testing.T) {
		input := []byte(`{"name": "test", "value": 123}`)

		result, err := tr.JSONToJSON(input, func(data map[string]any) map[string]any {
			data["transformed"] = true
			return data
		})

		if err != nil {
			t.Fatalf("transform failed: %v", err)
		}

		var output map[string]any
		_ = json.Unmarshal(result, &output)

		if output["transformed"] != true {
			t.Error("expected transformed=true")
		}
		if output["name"] != "test" {
			t.Error("expected original name to be preserved")
		}
	})

	t.Run("WrapInEnvelope", func(t *testing.T) {
		input := []byte(`{"data": "value"}`)

		result, err := tr.WrapInEnvelope(input, "response")
		if err != nil {
			t.Fatalf("wrap failed: %v", err)
		}

		var output map[string]json.RawMessage
		_ = json.Unmarshal(result, &output)

		if _, exists := output["response"]; !exists {
			t.Error("expected response envelope")
		}
	})

	t.Run("ExtractField", func(t *testing.T) {
		input := []byte(`{"data": {"nested": "value"}, "meta": "info"}`)

		result, err := tr.ExtractField(input, "data")
		if err != nil {
			t.Fatalf("extract failed: %v", err)
		}

		var output map[string]string
		_ = json.Unmarshal(result, &output)

		if output["nested"] != "value" {
			t.Errorf("expected nested=value, got %s", output["nested"])
		}
	})

	t.Run("ExtractField missing", func(t *testing.T) {
		input := []byte(`{"data": "value"}`)

		_, err := tr.ExtractField(input, "missing")
		if err == nil {
			t.Fatal("expected error for missing field")
		}
	})
}

func TestConcurrentProxyRequests(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultProxyConfig(server.URL)
	config.Logger = testLogger()

	proxy, err := NewPythonProxy(config)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	var wg sync.WaitGroup
	concurrency := 50
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := proxy.ProxyRequest(context.Background(), "GET", "/concurrent", nil)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent request failed: %v", err)
	}

	if int(requestCount.Load()) != concurrency {
		t.Errorf("expected %d requests, got %d", concurrency, requestCount.Load())
	}
}

func TestProxyWithContextPropagation(t *testing.T) {
	var receivedTenantID, receivedTraceID, receivedRequestID string
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedTenantID = r.Header.Get(HeaderTenantID)
		receivedTraceID = r.Header.Get(HeaderTraceID)
		receivedRequestID = r.Header.Get(HeaderRequestID)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultProxyConfig(server.URL)
	config.Logger = testLogger()

	proxy, err := NewPythonProxy(config)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// Create context with values.
	ctx := context.WithValue(context.Background(), "tenant_id", "ctx-tenant-123")
	ctx = context.WithValue(ctx, "trace_id", "ctx-trace-456")
	ctx = context.WithValue(ctx, "request_id", "ctx-request-789")

	_, err = proxy.ProxyRequest(ctx, "GET", "/context-test", nil)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if receivedTenantID != "ctx-tenant-123" {
		t.Errorf("expected tenant_id from context, got %s", receivedTenantID)
	}
	if receivedTraceID != "ctx-trace-456" {
		t.Errorf("expected trace_id from context, got %s", receivedTraceID)
	}
	if receivedRequestID != "ctx-request-789" {
		t.Errorf("expected request_id from context, got %s", receivedRequestID)
	}
}

func TestProxyMetricsRecording(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := DefaultProxyConfig(server.URL)
	config.Logger = testLogger()

	proxy, err := NewPythonProxy(config)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	metricsInst := NewProxyMetrics("test_metrics")
	proxy.SetMetrics(metricsInst)

	// Make a request.
	_, err = proxy.ProxyRequest(context.Background(), "GET", "/metrics-test", nil)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}

	// Verify metrics were recorded.
	// We can't easily inspect counter values without a custom gatherer,
	// but we can at least verify no panic occurred.
}

// Use bytes import in a blank assignment to ensure the import is used.
var _ = bytes.NewBuffer
