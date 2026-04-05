package timeout

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const testTenantID = "c3170310-78bd-409c-b186-126f40bfa6ad"

// mockRow implements pgx.Row for testing.
type mockRow struct {
	scanFunc func(dest ...any) error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.scanFunc != nil {
		return m.scanFunc(dest...)
	}
	return pgx.ErrNoRows
}

// mockRows implements pgx.Rows for testing.
type mockRows struct {
	rows    [][]any
	current int
	err     error
}

func newMockRows(data [][]any) *mockRows {
	return &mockRows{
		rows:    data,
		current: -1,
	}
}

func (m *mockRows) Next() bool {
	m.current++
	return m.current < len(m.rows)
}

func (m *mockRows) Scan(dest ...any) error {
	if m.current < 0 || m.current >= len(m.rows) {
		return errors.New("scan called before Next or after end")
	}

	row := m.rows[m.current]
	if len(dest) != len(row) {
		return errors.New("column count mismatch")
	}

	for i, val := range row {
		switch d := dest[i].(type) {
		case *string:
			*d = val.(string)
		default:
			return errors.New("unsupported scan type")
		}
	}

	return nil
}

func (m *mockRows) Err() error {
	return m.err
}

func (m *mockRows) Close() {}

func (m *mockRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (m *mockRows) RawValues() [][]byte {
	return nil
}

func (m *mockRows) Conn() *pgx.Conn {
	return nil
}

func (m *mockRows) Values() ([]any, error) {
	if m.current < 0 || m.current >= len(m.rows) {
		return nil, errors.New("Values called before Next or after end")
	}
	return m.rows[m.current], nil
}

// mockDB implements the DB interface for testing.
type mockDB struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFunc    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return &mockRow{}
}

func (m *mockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, sql, args...)
	}
	return newMockRows(nil), nil
}

func (m *mockDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func TestNew_DefaultsLoaded(t *testing.T) {
	cfg := New(nil, testTenantID)

	// The 4 active timeout defaults should be accessible.
	expectedKeys := []string{
		"timeout.ai_client.request",
		"timeout.http.backend.gemini",
		"timeout.http.backend.mlx",
		"timeout.schedule_to_close.default",
	}

	for _, key := range expectedKeys {
		val := cfg.Get(key)
		if val == 0 {
			t.Errorf("expected non-zero default for %s, got 0", key)
		}
	}
}

func TestGet_ReturnsDefault(t *testing.T) {
	cfg := New(nil, testTenantID)

	// Test a specific known default
	val := cfg.Get("timeout.ai_client.request")
	expected := 120 * time.Second

	if val != expected {
		t.Errorf("expected %v, got %v", expected, val)
	}
}

func TestGet_UnknownKey(t *testing.T) {
	cfg := New(nil, testTenantID)

	val := cfg.Get("timeout.unknown.key")
	if val != 0 {
		t.Errorf("expected 0 for unknown key, got %v", val)
	}
}

func TestGetEntry_Found(t *testing.T) {
	cfg := New(nil, testTenantID)

	entry, ok := cfg.GetEntry("timeout.ai_client.request")
	if !ok {
		t.Fatal("expected to find entry")
	}

	if entry.Key != "timeout.ai_client.request" {
		t.Errorf("expected key 'timeout.ai_client.request', got %s", entry.Key)
	}

	if entry.Value != 120*time.Second {
		t.Errorf("expected value 120s, got %v", entry.Value)
	}
}

func TestGetEntry_NotFound(t *testing.T) {
	cfg := New(nil, testTenantID)

	_, ok := cfg.GetEntry("timeout.unknown.key")
	if ok {
		t.Error("expected not to find unknown entry")
	}
}

func TestAll_ReturnsAllEntries(t *testing.T) {
	cfg := New(nil, testTenantID)

	all := cfg.All()
	if len(all) != 4 {
		t.Errorf("expected 4 entries (4 active timeout defaults), got %d", len(all))
	}
}

func TestSet_ValidatesMinMax(t *testing.T) {
	// Mock DB that succeeds on Exec
	db := &mockDB{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
	}

	cfg := New(db, testTenantID)

	// Test below min
	err := cfg.Set(context.Background(), "timeout.ai_client.request", 5*time.Second)
	if err == nil {
		t.Error("expected error for value below minimum")
	}

	// Test above max
	err = cfg.Set(context.Background(), "timeout.ai_client.request", 700*time.Second)
	if err == nil {
		t.Error("expected error for value above maximum")
	}
}

func TestSet_ValidValue(t *testing.T) {
	// Mock DB that succeeds on Exec
	execCalled := false
	db := &mockDB{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			execCalled = true
			return pgconn.CommandTag{}, nil
		},
	}

	cfg := New(db, testTenantID)

	// Test with valid value
	err := cfg.Set(context.Background(), "timeout.ai_client.request", 90*time.Second)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !execCalled {
		t.Error("expected Exec to be called")
	}

	// Verify value was updated in cache
	val := cfg.Get("timeout.ai_client.request")
	if val != 90*time.Second {
		t.Errorf("expected value 90s, got %v", val)
	}

	// Test with nil db - should return error
	cfgNil := New(nil, testTenantID)
	err = cfgNil.Set(context.Background(), "timeout.ai_client.request", 90*time.Second)
	if err == nil {
		t.Error("expected error when db is nil")
	}
}

func TestRefresh_LoadsFromDB(t *testing.T) {
	// Mock DB returns 2-column rows: key, value
	db := &mockDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			data := [][]any{
				{"timeout.ai_client.request", "180s"},
			}
			return newMockRows(data), nil
		},
	}

	cfg := New(db, testTenantID)

	err := cfg.Refresh(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that the value was updated
	val := cfg.Get("timeout.ai_client.request")
	expected := 180 * time.Second

	if val != expected {
		t.Errorf("expected %v, got %v", expected, val)
	}
}

func TestRefresh_DBError_KeepsOldValues(t *testing.T) {
	// First create config with defaults
	cfg := New(nil, testTenantID)
	oldVal := cfg.Get("timeout.ai_client.request")

	// Now add a DB that returns error
	db := &mockDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return nil, errors.New("database error")
		},
	}
	cfg.db = db

	err := cfg.Refresh(context.Background())
	if err == nil {
		t.Error("expected error from Refresh")
	}

	// Check that old value is still there
	newVal := cfg.Get("timeout.ai_client.request")
	if newVal != oldVal {
		t.Errorf("expected value to remain %v, got %v", oldVal, newVal)
	}
}

func TestOnChange_Fires(t *testing.T) {
	var callbackKey string
	var callbackOld, callbackNew time.Duration
	var callbackCalled bool

	db := &mockDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			data := [][]any{
				{"timeout.ai_client.request", "180s"},
			}
			return newMockRows(data), nil
		},
	}

	cfg := New(db, testTenantID)

	// Register callback
	cfg.OnChange(func(key string, oldVal, newVal time.Duration) {
		callbackKey = key
		callbackOld = oldVal
		callbackNew = newVal
		callbackCalled = true
	})

	// Refresh to trigger callback
	err := cfg.Refresh(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check callback was called
	if !callbackCalled {
		t.Error("expected callback to be called")
	}

	if callbackKey != "timeout.ai_client.request" {
		t.Errorf("expected key 'timeout.ai_client.request', got %s", callbackKey)
	}

	if callbackOld != 120*time.Second {
		t.Errorf("expected old value 120s, got %v", callbackOld)
	}

	if callbackNew != 180*time.Second {
		t.Errorf("expected new value 180s, got %v", callbackNew)
	}
}

func TestConfig_ConcurrentAccess(t *testing.T) {
	db := &mockDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			data := [][]any{
				{"timeout.ai_client.request", "150s"},
			}
			return newMockRows(data), nil
		},
	}

	cfg := New(db, testTenantID)

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = cfg.Get("timeout.ai_client.request")
				_, _ = cfg.GetEntry("timeout.ai_client.request")
				_ = cfg.All()
			}
		}()
	}

	// Concurrent refreshers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = cfg.Refresh(context.Background())
			}
		}()
	}

	wg.Wait()
}

func TestRefresh_NilDB(t *testing.T) {
	cfg := New(nil, testTenantID)

	// Refresh with nil db should not error
	err := cfg.Refresh(context.Background())
	if err != nil {
		t.Errorf("expected no error with nil db, got %v", err)
	}
}

func TestSet_UnknownKey(t *testing.T) {
	db := &mockDB{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
	}

	cfg := New(db, testTenantID)

	err := cfg.Set(context.Background(), "timeout.nonexistent.key", 30*time.Second)
	if err == nil {
		t.Error("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("expected error message to contain 'unknown config key', got: %s", err.Error())
	}
}

func TestRefresh_InvalidDuration(t *testing.T) {
	db := &mockDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			data := [][]any{
				{"timeout.ai_client.request", "not-a-duration"},
			}
			return newMockRows(data), nil
		},
	}

	cfg := New(db, testTenantID)

	err := cfg.Refresh(context.Background())
	if err == nil {
		t.Error("expected error for unparseable duration")
	}
	if !strings.Contains(err.Error(), "failed to parse value") {
		t.Errorf("expected error message to contain 'failed to parse value', got: %s", err.Error())
	}

	// Verify old defaults are preserved (Refresh doesn't update entries on error)
	val := cfg.Get("timeout.ai_client.request")
	if val != 120*time.Second {
		t.Errorf("expected original default 120s to be preserved, got %v", val)
	}
}

func TestStartRefreshLoop(t *testing.T) {
	var mu sync.Mutex
	refreshCount := 0

	db := &mockDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			mu.Lock()
			refreshCount++
			mu.Unlock()
			data := [][]any{
				{"timeout.ai_client.request", "150s"},
			}
			return newMockRows(data), nil
		},
	}

	cfg := New(db, testTenantID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start refresh loop with short interval
	cfg.StartRefreshLoop(ctx, 20*time.Millisecond)

	// Poll for at least 2 refreshes with a generous timeout (handles loaded CI machines).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := refreshCount
		mu.Unlock()
		if got >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	if refreshCount < 2 {
		t.Errorf("expected at least 2 refreshes within 2s, got %d", refreshCount)
	}
	mu.Unlock()

	// Cancel and ensure it stops
	cancel()
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	countAfterCancel := refreshCount
	mu.Unlock()

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	finalCount := refreshCount
	mu.Unlock()
	if finalCount != countAfterCancel {
		t.Errorf("expected refresh loop to stop after context cancellation (was %d, now %d)", countAfterCancel, finalCount)
	}
}
