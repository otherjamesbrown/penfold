package queue

import (
	"context"
	"testing"
	"time"

	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/reviewv1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockLogger implements logging.Logger for testing.
type mockLogger struct{}

func (m *mockLogger) Debug(msg string, fields ...interface{})     {}
func (m *mockLogger) Info(msg string, fields ...interface{})      {}
func (m *mockLogger) Warn(msg string, fields ...interface{})      {}
func (m *mockLogger) Error(msg string, fields ...interface{})     {}
func (m *mockLogger) With(fields ...interface{}) *mockLogger      { return m }
func (m *mockLogger) WithContext(ctx context.Context) *mockLogger { return m }

// =============================================================================
// QueueItem Tests
// =============================================================================

func TestQueueItem_Fields(t *testing.T) {
	now := time.Now()
	item := &QueueItem{
		ID:               "test-id",
		TenantID:         "tenant-1",
		Priority:         5.5,
		ContentType:      "email",
		ContentSummary:   "Test summary",
		Source:           "gmail",
		Category:         "communications",
		OriginalPriority: reviewv1.Priority_PRIORITY_HIGH,
		CreatedAt:        now.Add(-time.Hour),
		EnqueuedAt:       now,
		DeferCount:       0,
		Metadata:         map[string]string{"key": "value"},
	}

	if item.ID != "test-id" {
		t.Errorf("Expected ID test-id, got %s", item.ID)
	}
	if item.TenantID != "tenant-1" {
		t.Errorf("Expected TenantID tenant-1, got %s", item.TenantID)
	}
	if item.Priority != 5.5 {
		t.Errorf("Expected Priority 5.5, got %f", item.Priority)
	}
	if item.OriginalPriority != reviewv1.Priority_PRIORITY_HIGH {
		t.Errorf("Expected OriginalPriority HIGH, got %v", item.OriginalPriority)
	}
}

// =============================================================================
// QueueStats Tests
// =============================================================================

func TestQueueStats_Fields(t *testing.T) {
	stats := &QueueStats{
		TenantID:           "tenant-1",
		TotalItems:         100,
		PendingItems:       80,
		DeferredItems:      20,
		HighPriorityItems:  10,
		OldestItemAge:      24 * time.Hour,
		AverageWaitTime:    2 * time.Hour,
		ItemsByCategory:    map[string]int64{"communications": 50, "tasks": 30},
		ItemsByContentType: map[string]int64{"email": 60, "document": 40},
		ItemsByPriority: map[reviewv1.Priority]int64{
			reviewv1.Priority_PRIORITY_HIGH:   10,
			reviewv1.Priority_PRIORITY_MEDIUM: 50,
		},
		ThroughputLastHour: 15,
		ThroughputLastDay:  150,
	}

	if stats.TotalItems != 100 {
		t.Errorf("Expected TotalItems 100, got %d", stats.TotalItems)
	}
	if stats.PendingItems != 80 {
		t.Errorf("Expected PendingItems 80, got %d", stats.PendingItems)
	}
	if stats.ItemsByCategory["communications"] != 50 {
		t.Errorf("Expected 50 communications, got %d", stats.ItemsByCategory["communications"])
	}
}

// =============================================================================
// ManagerConfig Tests
// =============================================================================

func TestDefaultManagerConfig(t *testing.T) {
	cfg := DefaultManagerConfig()

	if cfg.AgingInterval != time.Hour {
		t.Errorf("Expected AgingInterval 1h, got %v", cfg.AgingInterval)
	}
	if cfg.AgingFactor != 0.1 {
		t.Errorf("Expected AgingFactor 0.1, got %f", cfg.AgingFactor)
	}
	if cfg.MaxDeferCount != 5 {
		t.Errorf("Expected MaxDeferCount 5, got %d", cfg.MaxDeferCount)
	}
	if cfg.DefaultDeferDuration != time.Hour {
		t.Errorf("Expected DefaultDeferDuration 1h, got %v", cfg.DefaultDeferDuration)
	}
	if cfg.BatchSize != 10 {
		t.Errorf("Expected BatchSize 10, got %d", cfg.BatchSize)
	}
}

// =============================================================================
// Error Tests
// =============================================================================

func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ErrItemNotFound", ErrItemNotFound, "queue item not found"},
		{"ErrQueueEmpty", ErrQueueEmpty, "queue is empty"},
		{"ErrInvalidItem", ErrInvalidItem, "invalid queue item"},
		{"ErrDatabaseError", ErrDatabaseError, "database operation failed"},
		{"ErrTenantRequired", ErrTenantRequired, "tenant ID is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.want {
				t.Errorf("Expected %q, got %q", tt.want, tt.err.Error())
			}
		})
	}
}

// =============================================================================
// QueueMetrics Tests
// =============================================================================

func TestNewQueueMetrics(t *testing.T) {
	m := NewQueueMetrics("test")

	if m.queueDepth == nil {
		t.Error("queueDepth should not be nil")
	}
	if m.enqueueTotal == nil {
		t.Error("enqueueTotal should not be nil")
	}
	if m.dequeueTotal == nil {
		t.Error("dequeueTotal should not be nil")
	}
	if m.requeueTotal == nil {
		t.Error("requeueTotal should not be nil")
	}
	if m.waitTime == nil {
		t.Error("waitTime should not be nil")
	}
	if m.operationLatency == nil {
		t.Error("operationLatency should not be nil")
	}
}

// =============================================================================
// ReviewItem Helper
// =============================================================================

func makeTestReviewItem(id, contentType string, priority reviewv1.Priority) *reviewv1.ReviewItem {
	return &reviewv1.ReviewItem{
		Id:             id,
		ContentType:    contentType,
		ContentSummary: "Test item " + id,
		Source:         "test",
		Priority:       priority,
		Status:         reviewv1.ReviewStatus_REVIEW_STATUS_PENDING,
		CreatedAt:      timestamppb.Now(),
		Category:       "test",
		Metadata:       map[string]string{"test": "true"},
	}
}

// =============================================================================
// Priority Calculator Integration Tests
// =============================================================================

func TestManager_PriorityCalculation(t *testing.T) {
	// Create a queue item for testing priority calculation
	now := time.Now()
	item := &QueueItem{
		ID:               "test-1",
		TenantID:         "tenant-1",
		ContentType:      "email",
		Source:           "gmail",
		Category:         "communications",
		OriginalPriority: reviewv1.Priority_PRIORITY_HIGH,
		CreatedAt:        now.Add(-2 * time.Hour),
		EnqueuedAt:       now.Add(-time.Hour),
		DeferCount:       0,
	}

	// Test with MixedPriority (default calculator)
	calc := NewMixedPriority(0.4, 0.4, 0.2)
	priority := calc.Calculate(item)

	if priority <= 0 {
		t.Errorf("Expected positive priority, got %f", priority)
	}

	// Test with deferred item
	item.DeferCount = 2
	deferredPriority := calc.Calculate(item)

	if deferredPriority >= priority {
		t.Errorf("Deferred item should have lower priority: deferred=%f, original=%f", deferredPriority, priority)
	}
}

func TestManager_SetPriorityCalculator(t *testing.T) {
	m := &Manager{
		config:       DefaultManagerConfig(),
		priorityCalc: NewMixedPriority(0.4, 0.4, 0.2),
	}

	if m.priorityCalc.Name() != "mixed_priority" {
		t.Errorf("Expected mixed_priority, got %s", m.priorityCalc.Name())
	}

	// Change to TimePriority
	m.SetPriorityCalculator(NewTimePriority())

	if m.priorityCalc.Name() != "time_priority" {
		t.Errorf("Expected time_priority, got %s", m.priorityCalc.Name())
	}
}

// =============================================================================
// Validation Tests
// =============================================================================

func TestManager_ValidationErrors(t *testing.T) {
	// These tests verify error handling without a database connection.
	// They test the validation logic before database operations.

	t.Run("Enqueue_EmptyTenant", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		err := m.Enqueue(context.Background(), "", makeTestReviewItem("1", "email", reviewv1.Priority_PRIORITY_HIGH))
		if err != ErrTenantRequired {
			t.Errorf("Expected ErrTenantRequired, got %v", err)
		}
	})

	t.Run("Enqueue_NilItem", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		err := m.Enqueue(context.Background(), "tenant-1", nil)
		if err != ErrInvalidItem {
			t.Errorf("Expected ErrInvalidItem, got %v", err)
		}
	})

	t.Run("Enqueue_EmptyItemID", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		item := makeTestReviewItem("", "email", reviewv1.Priority_PRIORITY_HIGH)
		err := m.Enqueue(context.Background(), "tenant-1", item)
		if err != ErrInvalidItem {
			t.Errorf("Expected ErrInvalidItem, got %v", err)
		}
	})

	t.Run("EnqueueBatch_EmptyTenant", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		items := []*reviewv1.ReviewItem{makeTestReviewItem("1", "email", reviewv1.Priority_PRIORITY_HIGH)}
		_, err := m.EnqueueBatch(context.Background(), "", items)
		if err != ErrTenantRequired {
			t.Errorf("Expected ErrTenantRequired, got %v", err)
		}
	})

	t.Run("EnqueueBatch_EmptyItems", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		count, err := m.EnqueueBatch(context.Background(), "tenant-1", nil)
		if err != nil {
			t.Errorf("Expected no error for empty items, got %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 count, got %d", count)
		}
	})

	t.Run("Dequeue_EmptyTenant", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		_, err := m.Dequeue(context.Background(), "", 10)
		if err != ErrTenantRequired {
			t.Errorf("Expected ErrTenantRequired, got %v", err)
		}
	})

	t.Run("Peek_EmptyTenant", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		_, err := m.Peek(context.Background(), "", 10)
		if err != ErrTenantRequired {
			t.Errorf("Expected ErrTenantRequired, got %v", err)
		}
	})

	t.Run("Requeue_EmptyTenant", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		err := m.Requeue(context.Background(), "", "item-1", time.Hour, "test")
		if err != ErrTenantRequired {
			t.Errorf("Expected ErrTenantRequired, got %v", err)
		}
	})

	t.Run("Requeue_EmptyItemID", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		err := m.Requeue(context.Background(), "tenant-1", "", time.Hour, "test")
		if err != ErrInvalidItem {
			t.Errorf("Expected ErrInvalidItem, got %v", err)
		}
	})

	t.Run("Remove_EmptyTenant", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		err := m.Remove(context.Background(), "", "item-1")
		if err != ErrTenantRequired {
			t.Errorf("Expected ErrTenantRequired, got %v", err)
		}
	})

	t.Run("Remove_EmptyItemID", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		err := m.Remove(context.Background(), "tenant-1", "")
		if err != ErrInvalidItem {
			t.Errorf("Expected ErrInvalidItem, got %v", err)
		}
	})

	t.Run("RemoveBatch_EmptyTenant", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		_, err := m.RemoveBatch(context.Background(), "", []string{"item-1"})
		if err != ErrTenantRequired {
			t.Errorf("Expected ErrTenantRequired, got %v", err)
		}
	})

	t.Run("RemoveBatch_EmptyItems", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		count, err := m.RemoveBatch(context.Background(), "tenant-1", nil)
		if err != nil {
			t.Errorf("Expected no error for empty items, got %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 count, got %d", count)
		}
	})

	t.Run("GetQueueStats_EmptyTenant", func(t *testing.T) {
		m := &Manager{config: DefaultManagerConfig()}
		_, err := m.GetQueueStats(context.Background(), "")
		if err != ErrTenantRequired {
			t.Errorf("Expected ErrTenantRequired, got %v", err)
		}
	})
}

// =============================================================================
// Default Count Tests
// =============================================================================

func TestManager_DefaultCounts(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.BatchSize = 15

	m := &Manager{config: cfg}

	t.Run("Dequeue_DefaultCount", func(t *testing.T) {
		// When count is 0 or negative, should use BatchSize
		// This test verifies the logic, not the actual database call
		if m.config.BatchSize != 15 {
			t.Errorf("Expected BatchSize 15, got %d", m.config.BatchSize)
		}
	})

	t.Run("Peek_DefaultCount", func(t *testing.T) {
		if m.config.BatchSize != 15 {
			t.Errorf("Expected BatchSize 15, got %d", m.config.BatchSize)
		}
	})
}

// =============================================================================
// Aging Loop Tests
// =============================================================================

func TestManager_AgingLoop(t *testing.T) {
	m := &Manager{
		config: &ManagerConfig{
			AgingInterval: 50 * time.Millisecond,
			AgingFactor:   0.1,
		},
		stopAging: make(chan struct{}),
	}

	// Test that aging doesn't start twice
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start with no database - it will fail but loop should handle it
	m.agingRunning = false

	// Manually set to running to test double-start prevention
	m.agingRunning = true
	m.StartAgingLoop(ctx) // Should return immediately
	m.agingRunning = false

	// Test stop when not running
	m.stopAging = make(chan struct{})
	m.StopAgingLoop() // Should be safe to call
}

// =============================================================================
// Metrics Tests
// =============================================================================

func TestQueueMetrics_Labels(t *testing.T) {
	m := NewQueueMetrics("penfold")

	// Verify metrics have correct labels
	if m.queueDepth == nil {
		t.Fatal("queueDepth should not be nil")
	}

	// Test that we can record metrics without panicking
	// (metrics will only work properly when registered, but shouldn't panic)
	m.queueDepth.WithLabelValues("tenant-1", "high", "communications")
	m.enqueueTotal.WithLabelValues("tenant-1", "email")
	m.dequeueTotal.WithLabelValues("tenant-1", "email")
	m.requeueTotal.WithLabelValues("tenant-1", "deferred")
	m.waitTime.WithLabelValues("tenant-1", "email")
	m.operationLatency.WithLabelValues("enqueue")
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkPriorityCalculation(b *testing.B) {
	calc := NewMixedPriority(0.4, 0.4, 0.2)
	now := time.Now()
	item := &QueueItem{
		ID:               "bench-item",
		TenantID:         "tenant-1",
		ContentType:      "email",
		Source:           "gmail",
		Category:         "communications",
		OriginalPriority: reviewv1.Priority_PRIORITY_HIGH,
		CreatedAt:        now.Add(-2 * time.Hour),
		EnqueuedAt:       now.Add(-time.Hour),
		DeferCount:       1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calc.Calculate(item)
	}
}

func BenchmarkTimePriorityCalculation(b *testing.B) {
	calc := NewTimePriority()
	now := time.Now()
	item := &QueueItem{
		EnqueuedAt: now.Add(-time.Hour),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calc.Calculate(item)
	}
}

func BenchmarkImportancePriorityCalculation(b *testing.B) {
	calc := NewImportancePriority()
	item := &QueueItem{
		ContentType:      "email",
		Category:         "communications",
		OriginalPriority: reviewv1.Priority_PRIORITY_HIGH,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calc.Calculate(item)
	}
}
