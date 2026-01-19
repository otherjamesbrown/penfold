package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewChecker(t *testing.T) {
	checker := NewChecker()
	if checker == nil {
		t.Fatal("NewChecker returned nil")
	}
	if checker.checks == nil {
		t.Fatal("checks slice is nil")
	}
}

func TestChecker_RegisterCheck(t *testing.T) {
	checker := NewChecker()

	checker.RegisterCheck("test", func(ctx context.Context) error {
		return nil
	})

	if len(checker.checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checker.checks))
	}

	if checker.checks[0].name != "test" {
		t.Errorf("expected check name 'test', got '%s'", checker.checks[0].name)
	}

	// Default should be critical
	if !checker.checks[0].critical {
		t.Error("expected check to be critical by default")
	}
}

func TestChecker_Check_AllHealthy(t *testing.T) {
	checker := NewChecker()

	checker.RegisterCheck("check1", func(ctx context.Context) error {
		return nil
	})
	checker.RegisterCheck("check2", func(ctx context.Context) error {
		return nil
	})

	ctx := context.Background()
	status := checker.Check(ctx)

	if status.Status != StatusHealthy {
		t.Errorf("expected status %s, got %s", StatusHealthy, status.Status)
	}

	if len(status.Checks) != 2 {
		t.Errorf("expected 2 checks, got %d", len(status.Checks))
	}

	for name, result := range status.Checks {
		if result.Status != StatusHealthy {
			t.Errorf("check %s: expected status %s, got %s", name, StatusHealthy, result.Status)
		}
		if result.Error != "" {
			t.Errorf("check %s: unexpected error: %s", name, result.Error)
		}
	}
}

func TestChecker_Check_CriticalFailure(t *testing.T) {
	checker := NewChecker()

	checker.RegisterCheck("healthy", func(ctx context.Context) error {
		return nil
	})
	checker.RegisterCheck("critical_fail", func(ctx context.Context) error {
		return errors.New("critical failure")
	}, Critical())

	ctx := context.Background()
	status := checker.Check(ctx)

	if status.Status != StatusUnhealthy {
		t.Errorf("expected status %s, got %s", StatusUnhealthy, status.Status)
	}

	if status.Checks["critical_fail"].Status != StatusUnhealthy {
		t.Error("critical_fail check should be unhealthy")
	}
	if status.Checks["critical_fail"].Error != "critical failure" {
		t.Errorf("unexpected error message: %s", status.Checks["critical_fail"].Error)
	}
}

func TestChecker_Check_NonCriticalFailure(t *testing.T) {
	checker := NewChecker()

	checker.RegisterCheck("healthy", func(ctx context.Context) error {
		return nil
	})

	// Non-critical check (explicitly set critical=false via not using Critical() option)
	cfg := &checkConfig{critical: false}
	checker.mu.Lock()
	checker.checks = append(checker.checks, registeredCheck{
		name: "non_critical_fail",
		fn: func(ctx context.Context) error {
			return errors.New("non-critical failure")
		},
		critical: cfg.critical,
	})
	checker.mu.Unlock()

	ctx := context.Background()
	status := checker.Check(ctx)

	if status.Status != StatusDegraded {
		t.Errorf("expected status %s, got %s", StatusDegraded, status.Status)
	}
}

func TestChecker_Check_Timing(t *testing.T) {
	checker := NewChecker()

	checker.RegisterCheck("slow_check", func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	ctx := context.Background()
	status := checker.Check(ctx)

	result := status.Checks["slow_check"]
	if result.Duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", result.Duration)
	}
}

func TestChecker_Check_ContextCancellation(t *testing.T) {
	checker := NewChecker()

	checker.RegisterCheck("context_aware", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	status := checker.Check(ctx)

	result := status.Checks["context_aware"]
	if result.Status != StatusUnhealthy {
		t.Errorf("expected unhealthy status due to context cancellation")
	}
}

func TestChecker_Handler(t *testing.T) {
	checker := NewChecker()
	checker.RegisterCheck("test", func(ctx context.Context) error {
		return nil
	})

	handler := checker.Handler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var status HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if status.Status != StatusHealthy {
		t.Errorf("expected status %s, got %s", StatusHealthy, status.Status)
	}
}

func TestChecker_Handler_Unhealthy(t *testing.T) {
	checker := NewChecker()
	checker.RegisterCheck("failing", func(ctx context.Context) error {
		return errors.New("service unavailable")
	})

	handler := checker.Handler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	var status HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if status.Status != StatusUnhealthy {
		t.Errorf("expected status %s, got %s", StatusUnhealthy, status.Status)
	}
}

func TestChecker_ReadyHandler(t *testing.T) {
	t.Run("healthy returns 200", func(t *testing.T) {
		checker := NewChecker()
		checker.RegisterCheck("test", func(ctx context.Context) error {
			return nil
		})

		handler := checker.ReadyHandler()
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})

	t.Run("unhealthy returns 503", func(t *testing.T) {
		checker := NewChecker()
		checker.RegisterCheck("test", func(ctx context.Context) error {
			return errors.New("not ready")
		})

		handler := checker.ReadyHandler()
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
		}
	})
}

func TestChecker_LiveHandler(t *testing.T) {
	checker := NewChecker()
	// Even with failing checks, liveness should return 200
	checker.RegisterCheck("failing", func(ctx context.Context) error {
		return errors.New("failure")
	})

	handler := checker.LiveHandler()
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestCheckResult_MarshalJSON(t *testing.T) {
	result := CheckResult{
		Status:   StatusHealthy,
		Duration: 15 * time.Millisecond,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled map[string]interface{}
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Duration should be in milliseconds
	duration, ok := unmarshaled["duration_ms"].(float64)
	if !ok {
		t.Fatal("duration_ms not found or not a number")
	}
	if duration != 15 {
		t.Errorf("expected duration_ms 15, got %v", duration)
	}
}

// Mock types for testing checks.go

type mockDatabasePool struct {
	pingErr error
}

func (m *mockDatabasePool) Ping(ctx context.Context) error {
	return m.pingErr
}

type mockRedisClient struct {
	pingErr error
}

func (m *mockRedisClient) Ping(ctx context.Context) RedisStatusCmd {
	return &mockRedisStatusCmd{err: m.pingErr}
}

type mockRedisStatusCmd struct {
	err error
}

func (m *mockRedisStatusCmd) Err() error {
	return m.err
}

type mockTemporalClient struct{}

func (m *mockTemporalClient) Close() {}

func TestDatabaseCheck(t *testing.T) {
	t.Run("healthy database", func(t *testing.T) {
		pool := &mockDatabasePool{}
		check := DatabaseCheck(pool)

		err := check(context.Background())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("unhealthy database", func(t *testing.T) {
		pool := &mockDatabasePool{pingErr: errors.New("connection refused")}
		check := DatabaseCheck(pool)

		err := check(context.Background())
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("nil pool", func(t *testing.T) {
		check := DatabaseCheck(nil)

		err := check(context.Background())
		if err == nil {
			t.Error("expected error for nil pool")
		}
	})
}

func TestRedisCheck(t *testing.T) {
	t.Run("healthy redis", func(t *testing.T) {
		client := &mockRedisClient{}
		check := RedisCheck(client)

		err := check(context.Background())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("unhealthy redis", func(t *testing.T) {
		client := &mockRedisClient{pingErr: errors.New("connection refused")}
		check := RedisCheck(client)

		err := check(context.Background())
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("nil client", func(t *testing.T) {
		check := RedisCheck(nil)

		err := check(context.Background())
		if err == nil {
			t.Error("expected error for nil client")
		}
	})
}

func TestTemporalCheck(t *testing.T) {
	t.Run("healthy temporal", func(t *testing.T) {
		client := &mockTemporalClient{}
		check := TemporalCheck(client)

		err := check(context.Background())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("nil client", func(t *testing.T) {
		check := TemporalCheck(nil)

		err := check(context.Background())
		if err == nil {
			t.Error("expected error for nil client")
		}
	})
}

func TestCustomCheck(t *testing.T) {
	t.Run("passing check", func(t *testing.T) {
		check := CustomCheck(func() error {
			return nil
		})

		err := check(context.Background())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("failing check", func(t *testing.T) {
		check := CustomCheck(func() error {
			return errors.New("custom failure")
		})

		err := check(context.Background())
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestContextualCheck(t *testing.T) {
	check := ContextualCheck(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})

	t.Run("with valid context", func(t *testing.T) {
		err := check(context.Background())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := check(ctx)
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})
}

func TestChecker_ConcurrentAccess(t *testing.T) {
	checker := NewChecker()

	// Register checks concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			checker.RegisterCheck("check", func(ctx context.Context) error {
				return nil
			})
			done <- true
		}(i)
	}

	// Wait for all registrations
	for i := 0; i < 10; i++ {
		<-done
	}

	// Run checks concurrently
	for i := 0; i < 10; i++ {
		go func() {
			checker.Check(context.Background())
			done <- true
		}()
	}

	// Wait for all checks
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestStatus_Values(t *testing.T) {
	if StatusHealthy != "healthy" {
		t.Errorf("StatusHealthy should be 'healthy'")
	}
	if StatusDegraded != "degraded" {
		t.Errorf("StatusDegraded should be 'degraded'")
	}
	if StatusUnhealthy != "unhealthy" {
		t.Errorf("StatusUnhealthy should be 'unhealthy'")
	}
}
