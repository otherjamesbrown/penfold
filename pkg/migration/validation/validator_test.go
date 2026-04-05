package validation

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckStatus(t *testing.T) {
	statuses := []CheckStatus{StatusPass, StatusFail, StatusWarn, StatusSkip}
	expected := []string{"PASS", "FAIL", "WARN", "SKIP"}

	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("Status %d: got %s, want %s", i, s, expected[i])
		}
	}
}

func TestServiceInfo(t *testing.T) {
	info := ServiceInfo{
		Name:     "test-service",
		GRPCAddr: "localhost:50051",
		HTTPAddr: "localhost:8080",
		Critical: true,
	}

	if info.Name != "test-service" {
		t.Errorf("Expected name test-service, got %s", info.Name)
	}
	if !info.Critical {
		t.Error("Expected critical to be true")
	}
}

func TestValidationResult(t *testing.T) {
	result := ValidationResult{
		Name:      "test_check",
		Status:    StatusPass,
		Message:   "Test passed",
		Duration:  100 * time.Millisecond,
		Timestamp: time.Now(),
		Details:   map[string]interface{}{"key": "value"},
	}

	if result.Name != "test_check" {
		t.Errorf("Expected name test_check, got %s", result.Name)
	}
	if result.Status != StatusPass {
		t.Errorf("Expected status PASS, got %s", result.Status)
	}
}

func TestNewValidator(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		v := NewValidator()

		if v == nil {
			t.Fatal("NewValidator returned nil")
		}
		if v.timeout != 30*time.Second {
			t.Errorf("Expected default timeout 30s, got %v", v.timeout)
		}
		if v.parallelism != 10 {
			t.Errorf("Expected default parallelism 10, got %d", v.parallelism)
		}
		if v.httpClient == nil {
			t.Error("HTTP client should not be nil")
		}
	})

	t.Run("with options", func(t *testing.T) {
		v := NewValidator(
			WithTimeout(60*time.Second),
			WithParallelism(5),
		)

		if v.timeout != 60*time.Second {
			t.Errorf("Expected timeout 60s, got %v", v.timeout)
		}
		if v.parallelism != 5 {
			t.Errorf("Expected parallelism 5, got %d", v.parallelism)
		}
	})
}

func TestValidator_DefaultServices(t *testing.T) {
	v := NewValidator()

	services := v.GetServices()

	expectedServices := []string{"gateway", "search", "gmail", "content", "ai", "review", "relationship"}
	for _, name := range expectedServices {
		if _, ok := services[name]; !ok {
			t.Errorf("Expected default service %s to be registered", name)
		}
	}
}

func TestValidator_RegisterService(t *testing.T) {
	v := NewValidator()

	customService := ServiceInfo{
		Name:     "custom",
		GRPCAddr: "localhost:50099",
		HTTPAddr: "localhost:8099",
		Critical: true,
	}

	v.RegisterService(customService)

	services := v.GetServices()
	if _, ok := services["custom"]; !ok {
		t.Error("Custom service should be registered")
	}

	// Verify service details
	svc := services["custom"]
	if svc.GRPCAddr != "localhost:50099" {
		t.Errorf("Expected GRPC addr localhost:50099, got %s", svc.GRPCAddr)
	}
}

func TestValidator_GetServices_ReturnsCopy(t *testing.T) {
	v := NewValidator()

	services1 := v.GetServices()
	services2 := v.GetServices()

	// Modify services1
	services1["modified"] = ServiceInfo{Name: "modified"}

	// services2 should not be affected
	if _, ok := services2["modified"]; ok {
		t.Error("GetServices should return a copy, not the original map")
	}
}

func TestValidator_ValidateServices(t *testing.T) {
	// Create a test HTTP server that responds to health checks
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"healthy"}`))
		}
	}))
	defer server.Close()

	// Extract host and port from test server
	host, port, _ := net.SplitHostPort(server.Listener.Addr().String())

	v := NewValidator()

	// Register a service pointing to our test server
	v.RegisterService(ServiceInfo{
		Name:     "test",
		GRPCAddr: fmt.Sprintf("%s:%s", host, port), // Use same port for TCP check
		HTTPAddr: fmt.Sprintf("%s:%s", host, port),
		Critical: true,
	})

	// Clear default services to only test our custom one
	v.mu.Lock()
	v.services = map[string]ServiceInfo{
		"test": v.services["test"],
	}
	v.mu.Unlock()

	ctx := context.Background()
	results := v.ValidateServices(ctx)

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	// The service should pass since both TCP and HTTP are available
	if results[0].Status != StatusPass {
		t.Errorf("Expected status PASS, got %s: %s", results[0].Status, results[0].Error)
	}
}

func TestValidator_ValidateServices_GRPCFails(t *testing.T) {
	v := NewValidator()

	// Register a service with unreachable ports
	v.RegisterService(ServiceInfo{
		Name:     "unreachable",
		GRPCAddr: "localhost:59999", // Unlikely to be in use
		HTTPAddr: "localhost:59998",
		Critical: true,
	})

	// Clear default services
	v.mu.Lock()
	v.services = map[string]ServiceInfo{
		"unreachable": v.services["unreachable"],
	}
	v.mu.Unlock()

	ctx := context.Background()
	results := v.ValidateServices(ctx)

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	// Should fail because gRPC port is not reachable
	if results[0].Status != StatusFail {
		t.Errorf("Expected status FAIL, got %s", results[0].Status)
	}
}

func TestValidator_ValidateServices_HTTPFails(t *testing.T) {
	// Create a TCP listener that accepts connections but doesn't serve HTTP
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close() //nolint:errcheck

	// Accept connections in background (to pass TCP check)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	addr := listener.Addr().String()

	v := NewValidator()
	v.httpClient.Timeout = 1 * time.Second // Short timeout for test

	v.RegisterService(ServiceInfo{
		Name:     "tcp_only",
		GRPCAddr: addr,
		HTTPAddr: addr,
		Critical: true,
	})

	// Clear default services
	v.mu.Lock()
	v.services = map[string]ServiceInfo{
		"tcp_only": v.services["tcp_only"],
	}
	v.mu.Unlock()

	ctx := context.Background()
	results := v.ValidateServices(ctx)

	// Should be WARN because TCP is reachable but HTTP fails
	if results[0].Status != StatusWarn {
		t.Errorf("Expected status WARN, got %s: %s", results[0].Status, results[0].Message)
	}
}

func TestValidator_ValidateServices_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	host, port, _ := net.SplitHostPort(server.Listener.Addr().String())

	v := NewValidator()

	v.RegisterService(ServiceInfo{
		Name:     "error_server",
		GRPCAddr: fmt.Sprintf("%s:%s", host, port),
		HTTPAddr: fmt.Sprintf("%s:%s", host, port),
		Critical: true,
	})

	v.mu.Lock()
	v.services = map[string]ServiceInfo{
		"error_server": v.services["error_server"],
	}
	v.mu.Unlock()

	ctx := context.Background()
	results := v.ValidateServices(ctx)

	// Should fail because HTTP returns 500
	if results[0].Status != StatusFail {
		t.Errorf("Expected status FAIL, got %s", results[0].Status)
	}
}

func TestValidator_ValidateServices_HTTP400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	host, port, _ := net.SplitHostPort(server.Listener.Addr().String())

	v := NewValidator()

	v.RegisterService(ServiceInfo{
		Name:     "bad_request_server",
		GRPCAddr: fmt.Sprintf("%s:%s", host, port),
		HTTPAddr: fmt.Sprintf("%s:%s", host, port),
		Critical: true,
	})

	v.mu.Lock()
	v.services = map[string]ServiceInfo{
		"bad_request_server": v.services["bad_request_server"],
	}
	v.mu.Unlock()

	ctx := context.Background()
	results := v.ValidateServices(ctx)

	// Should warn because HTTP returns 4xx
	if results[0].Status != StatusWarn {
		t.Errorf("Expected status WARN, got %s", results[0].Status)
	}
}

func TestValidator_ValidateDatabase(t *testing.T) {
	t.Run("connection failure", func(t *testing.T) {
		v := NewValidator()
		cfg := DatabaseValidationConfig{
			Host:     "localhost",
			Port:     59432, // Unlikely to be in use
			Database: "test",
			User:     "test",
		}

		ctx := context.Background()
		results := v.ValidateDatabase(ctx, cfg)

		// Should have 3 results (connection, schema, vector)
		if len(results) != 3 {
			t.Errorf("Expected 3 results, got %d", len(results))
		}

		// Connection should fail
		if results[0].Status != StatusFail {
			t.Errorf("Expected connection check to FAIL, got %s", results[0].Status)
		}

		// Schema and vector should be skipped
		if results[1].Status != StatusSkip {
			t.Errorf("Expected schema check to be SKIP, got %s", results[1].Status)
		}
		if results[2].Status != StatusSkip {
			t.Errorf("Expected vector check to be SKIP, got %s", results[2].Status)
		}
	})

	t.Run("connection success", func(t *testing.T) {
		// Create a TCP listener to simulate database port
		listener, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			t.Fatalf("Failed to create listener: %v", err)
		}
		defer listener.Close() //nolint:errcheck

		// Accept connections in background
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				_ = conn.Close()
			}
		}()

		_, port, _ := net.SplitHostPort(listener.Addr().String())
		portNum := 0
		_, _ = fmt.Sscanf(port, "%d", &portNum)

		v := NewValidator()
		cfg := DatabaseValidationConfig{
			Host:     "localhost",
			Port:     portNum,
			Database: "test",
			User:     "test",
		}

		ctx := context.Background()
		results := v.ValidateDatabase(ctx, cfg)

		// Connection should pass
		if results[0].Status != StatusPass {
			t.Errorf("Expected connection check to PASS, got %s: %s", results[0].Status, results[0].Error)
		}

		// Schema and vector should be WARN (placeholder implementations)
		if results[1].Status != StatusWarn {
			t.Errorf("Expected schema check to be WARN, got %s", results[1].Status)
		}
		if results[2].Status != StatusWarn {
			t.Errorf("Expected vector check to be WARN, got %s", results[2].Status)
		}
	})
}

func TestValidator_ValidateFeatureParity(t *testing.T) {
	t.Run("python endpoint unavailable", func(t *testing.T) {
		v := NewValidator()
		v.httpClient.Timeout = 1 * time.Second

		cfg := FeatureParityConfig{
			PythonEndpoint: "http://localhost:59001/health",
			GoEndpoint:     "http://localhost:59002/health",
		}

		ctx := context.Background()
		results := v.ValidateFeatureParity(ctx, cfg)

		// Should have at least 3 results (python check, go check, skip message)
		if len(results) < 3 {
			t.Errorf("Expected at least 3 results, got %d", len(results))
		}

		// Python check should fail
		if results[0].Status != StatusFail {
			t.Errorf("Expected python check to FAIL, got %s", results[0].Status)
		}
	})

	t.Run("empty endpoint", func(t *testing.T) {
		v := NewValidator()

		cfg := FeatureParityConfig{
			PythonEndpoint: "",
			GoEndpoint:     "",
		}

		ctx := context.Background()
		results := v.ValidateFeatureParity(ctx, cfg)

		// Empty endpoints should be skipped
		if results[0].Status != StatusSkip {
			t.Errorf("Expected empty python endpoint to be SKIP, got %s", results[0].Status)
		}
	})

	t.Run("both endpoints available", func(t *testing.T) {
		pythonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer pythonServer.Close()

		goServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer goServer.Close()

		v := NewValidator()

		cfg := FeatureParityConfig{
			PythonEndpoint: pythonServer.URL,
			GoEndpoint:     goServer.URL,
			TestCases: []ParityTestCase{
				{Name: "search", Description: "Search parity test", Endpoint: "/search", Method: "POST"},
			},
		}

		ctx := context.Background()
		results := v.ValidateFeatureParity(ctx, cfg)

		// Both endpoints should pass
		if results[0].Status != StatusPass {
			t.Errorf("Expected python endpoint check to PASS, got %s", results[0].Status)
		}
		if results[1].Status != StatusPass {
			t.Errorf("Expected go endpoint check to PASS, got %s", results[1].Status)
		}

		// Parity test should be WARN (placeholder)
		if len(results) > 2 && results[2].Status != StatusWarn {
			t.Errorf("Expected parity test to be WARN, got %s", results[2].Status)
		}
	})
}

func TestValidator_ValidateEndpoints(t *testing.T) {
	v := NewValidator()

	// Use unreachable addresses for faster test
	v.mu.Lock()
	v.services = map[string]ServiceInfo{
		"test": {
			Name:     "test",
			GRPCAddr: "localhost:59999",
			HTTPAddr: "localhost:59998",
		},
	}
	v.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results := v.ValidateEndpoints(ctx)

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	// Should fail because endpoint is unreachable
	if results[0].Status != StatusFail {
		t.Errorf("Expected status FAIL, got %s", results[0].Status)
	}
}

func TestValidator_RunAll(t *testing.T) {
	v := NewValidator()

	// Use unreachable addresses for faster test
	v.mu.Lock()
	v.services = map[string]ServiceInfo{
		"test": {
			Name:     "test",
			GRPCAddr: "localhost:59999",
			HTTPAddr: "localhost:59998",
		},
	}
	v.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	report := v.RunAll(ctx)

	if report == nil {
		t.Fatal("RunAll returned nil report")
	}

	// Should have results in multiple categories
	if len(report.Results) == 0 {
		t.Error("Report should have results")
	}

	// Should have services category
	if _, ok := report.Results["services"]; !ok {
		t.Error("Report should have services results")
	}

	// Should have endpoints category
	if _, ok := report.Results["endpoints"]; !ok {
		t.Error("Report should have endpoints results")
	}

	// Should have database category
	if _, ok := report.Results["database"]; !ok {
		t.Error("Report should have database results")
	}
}

func TestValidator_GetResults_ClearResults(t *testing.T) {
	v := NewValidator()

	// Initially empty
	results := v.GetResults()
	if len(results) != 0 {
		t.Errorf("Expected empty results, got %d", len(results))
	}

	// Clear empty (should not panic)
	v.ClearResults()

	results = v.GetResults()
	if len(results) != 0 {
		t.Errorf("Expected empty results after clear, got %d", len(results))
	}
}

func TestValidator_Parallelism(t *testing.T) {
	v := NewValidator(WithParallelism(2))

	// Add multiple services
	for i := 0; i < 5; i++ {
		v.RegisterService(ServiceInfo{
			Name:     fmt.Sprintf("service_%d", i),
			GRPCAddr: fmt.Sprintf("localhost:5999%d", i),
			HTTPAddr: fmt.Sprintf("localhost:5998%d", i),
		})
	}

	// Clear default services, keep only our test services
	v.mu.Lock()
	newServices := make(map[string]ServiceInfo)
	for name, svc := range v.services {
		if len(name) >= 9 && name[:8] == "service_" {
			newServices[name] = svc
		}
	}
	v.services = newServices
	v.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// This should complete without deadlock
	results := v.ValidateServices(ctx)

	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}
}

func TestDatabaseValidationConfig(t *testing.T) {
	cfg := DatabaseValidationConfig{
		Host:     "localhost",
		Port:     5432,
		Database: "penfold",
		User:     "penfold",
		Password: "secret",
		SSLMode:  "require",
	}

	if cfg.Host != "localhost" {
		t.Errorf("Expected host localhost, got %s", cfg.Host)
	}
	if cfg.Port != 5432 {
		t.Errorf("Expected port 5432, got %d", cfg.Port)
	}
	if cfg.SSLMode != "require" {
		t.Errorf("Expected SSLMode require, got %s", cfg.SSLMode)
	}
}

func TestParityTestCase(t *testing.T) {
	tc := ParityTestCase{
		Name:        "search_test",
		Description: "Test search functionality",
		Endpoint:    "/api/search",
		Method:      "POST",
		Payload:     map[string]string{"query": "test"},
	}

	if tc.Name != "search_test" {
		t.Errorf("Expected name search_test, got %s", tc.Name)
	}
	if tc.Method != "POST" {
		t.Errorf("Expected method POST, got %s", tc.Method)
	}
}
