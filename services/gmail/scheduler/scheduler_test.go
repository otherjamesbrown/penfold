package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gsync "github.com/otherjamesbrown/penfold/services/gmail/sync"
)

// TestPriorityCalculation tests the priority calculation functions.
func TestPriorityCalculation(t *testing.T) {
	t.Run("DefaultPriorityCalculator", func(t *testing.T) {
		now := time.Now()
		ctx := PriorityContext{
			QueueDepth:  100,
			CurrentTime: now,
		}

		t.Run("never synced account", func(t *testing.T) {
			account := &Account{
				ID:            "acc-1",
				TenantID:      "tenant-1",
				ActivityLevel: 50,
			}
			priority := DefaultPriorityCalculator(account, ctx)

			if priority.Factors.LastSync != 25 {
				t.Errorf("expected LastSync factor of 25 for never synced, got %d", priority.Factors.LastSync)
			}
			if priority.Score < 25 {
				t.Errorf("expected score >= 25, got %d", priority.Score)
			}
		})

		t.Run("recently synced account", func(t *testing.T) {
			account := &Account{
				ID:            "acc-2",
				TenantID:      "tenant-1",
				ActivityLevel: 50,
				LastSyncAt:    now.Add(-5 * time.Minute),
				SyncFrequency: 15 * time.Minute,
			}
			priority := DefaultPriorityCalculator(account, ctx)

			// 5 min / 15 min = 0.33, * 6.25 = ~2
			if priority.Factors.LastSync > 10 {
				t.Errorf("expected low LastSync factor for recently synced, got %d", priority.Factors.LastSync)
			}
		})

		t.Run("VIP account", func(t *testing.T) {
			account := &Account{
				ID:       "acc-3",
				TenantID: "tenant-1",
				IsVIP:    true,
			}
			priority := DefaultPriorityCalculator(account, ctx)

			if priority.Factors.VIPStatus != 25 {
				t.Errorf("expected VIPStatus factor of 25, got %d", priority.Factors.VIPStatus)
			}
		})

		t.Run("high activity account", func(t *testing.T) {
			account := &Account{
				ID:            "acc-4",
				TenantID:      "tenant-1",
				ActivityLevel: 100,
			}
			priority := DefaultPriorityCalculator(account, ctx)

			if priority.Factors.ActivityLevel != 25 {
				t.Errorf("expected ActivityLevel factor of 25 for max activity, got %d", priority.Factors.ActivityLevel)
			}
		})

		t.Run("pending notifications", func(t *testing.T) {
			account := &Account{
				ID:                   "acc-5",
				TenantID:             "tenant-1",
				PendingNotifications: 5,
			}
			priority := DefaultPriorityCalculator(account, ctx)

			// 5 * 5 = 25
			if priority.Factors.PendingNotifications != 25 {
				t.Errorf("expected PendingNotifications factor of 25, got %d", priority.Factors.PendingNotifications)
			}
		})

		t.Run("rate limited account", func(t *testing.T) {
			account := &Account{
				ID:               "acc-6",
				TenantID:         "tenant-1",
				RateLimitBackoff: now.Add(5 * time.Minute),
			}
			priority := DefaultPriorityCalculator(account, ctx)

			if priority.Factors.RateLimitPenalty >= 0 {
				t.Errorf("expected negative RateLimitPenalty, got %d", priority.Factors.RateLimitPenalty)
			}
		})
	})

	t.Run("VIPPriorityCalculator", func(t *testing.T) {
		ctx := PriorityContext{CurrentTime: time.Now()}
		vipAccount := &Account{
			ID:       "vip-1",
			TenantID: "tenant-1",
			IsVIP:    true,
		}
		normalAccount := &Account{
			ID:       "normal-1",
			TenantID: "tenant-1",
		}

		vipPriority := VIPPriorityCalculator(vipAccount, ctx)
		normalPriority := VIPPriorityCalculator(normalAccount, ctx)

		if vipPriority.Score <= normalPriority.Score {
			t.Errorf("VIP account should have higher priority")
		}
	})
}

// TestPriorityQueue tests the priority queue implementation.
func TestPriorityQueue(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		pq := NewPriorityQueue()

		// Verify empty queue.
		if pq.Len() != 0 {
			t.Errorf("expected empty queue, got %d items", pq.Len())
		}

		// Add tasks with different priorities.
		task1 := &SyncTask{ID: "task-1", Priority: Priority{Score: 50}}
		task2 := &SyncTask{ID: "task-2", Priority: Priority{Score: 80}}
		task3 := &SyncTask{ID: "task-3", Priority: Priority{Score: 30}}

		pq.PushTask(task1)
		pq.PushTask(task2)
		pq.PushTask(task3)

		if pq.Len() != 3 {
			t.Errorf("expected 3 items, got %d", pq.Len())
		}

		// Pop should return highest priority first.
		popped := pq.PopTask()
		if popped.ID != "task-2" {
			t.Errorf("expected task-2 (priority 80), got %s", popped.ID)
		}

		popped = pq.PopTask()
		if popped.ID != "task-1" {
			t.Errorf("expected task-1 (priority 50), got %s", popped.ID)
		}

		popped = pq.PopTask()
		if popped.ID != "task-3" {
			t.Errorf("expected task-3 (priority 30), got %s", popped.ID)
		}

		// Queue should be empty.
		if pq.Len() != 0 {
			t.Errorf("expected empty queue after pops")
		}
	})

	t.Run("update priority", func(t *testing.T) {
		pq := NewPriorityQueue()

		task1 := &SyncTask{ID: "task-1", Priority: Priority{Score: 30}}
		task2 := &SyncTask{ID: "task-2", Priority: Priority{Score: 50}}

		pq.PushTask(task1)
		pq.PushTask(task2)

		// Update task1 to higher priority.
		task1.Priority.Score = 80
		pq.Update(task1)

		// Now task1 should be first.
		popped := pq.PopTask()
		if popped.ID != "task-1" {
			t.Errorf("expected task-1 after priority update, got %s", popped.ID)
		}
	})

	t.Run("remove task", func(t *testing.T) {
		pq := NewPriorityQueue()

		task1 := &SyncTask{ID: "task-1", Priority: Priority{Score: 50}}
		task2 := &SyncTask{ID: "task-2", Priority: Priority{Score: 80}}

		pq.PushTask(task1)
		pq.PushTask(task2)

		// Remove task2.
		removed := pq.Remove("task-2")
		if removed == nil || removed.ID != "task-2" {
			t.Errorf("expected to remove task-2")
		}

		// Only task1 should remain.
		if pq.Len() != 1 {
			t.Errorf("expected 1 item, got %d", pq.Len())
		}
	})

	t.Run("peek", func(t *testing.T) {
		pq := NewPriorityQueue()

		task1 := &SyncTask{ID: "task-1", Priority: Priority{Score: 50}}
		pq.PushTask(task1)

		peeked := pq.Peek()
		if peeked.ID != "task-1" {
			t.Errorf("expected task-1 from peek")
		}

		// Peek should not remove.
		if pq.Len() != 1 {
			t.Errorf("peek should not remove item")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		pq := NewPriorityQueue()
		var wg sync.WaitGroup
		numGoroutines := 10
		tasksPerGoroutine := 100

		// Concurrent pushes.
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(base int) {
				defer wg.Done()
				for j := 0; j < tasksPerGoroutine; j++ {
					task := &SyncTask{
						ID:       string(rune('a' + base)) + string(rune('0'+j%10)),
						Priority: Priority{Score: (base*10 + j) % 100},
					}
					pq.PushTask(task)
				}
			}(i)
		}

		wg.Wait()

		if pq.Len() != numGoroutines*tasksPerGoroutine {
			t.Errorf("expected %d items, got %d", numGoroutines*tasksPerGoroutine, pq.Len())
		}
	})
}

// TestTokenBucket tests the token bucket rate limiter.
func TestTokenBucket(t *testing.T) {
	t.Run("basic take", func(t *testing.T) {
		bucket := NewTokenBucket(5, 1)

		// Should be able to take 5 immediately.
		for i := 0; i < 5; i++ {
			if !bucket.Take() {
				t.Errorf("expected to take token %d", i)
			}
		}

		// 6th should fail.
		if bucket.Take() {
			t.Error("expected take to fail when empty")
		}
	})

	t.Run("refill", func(t *testing.T) {
		bucket := NewTokenBucket(5, 10) // 10 tokens per second

		// Take all tokens.
		for i := 0; i < 5; i++ {
			bucket.Take()
		}

		// Wait for refill.
		time.Sleep(200 * time.Millisecond) // Should get ~2 tokens.

		if !bucket.Take() {
			t.Error("expected token after refill")
		}
	})

	t.Run("backoff", func(t *testing.T) {
		bucket := NewTokenBucket(5, 1)

		// Set backoff.
		bucket.SetBackoff(time.Now().Add(100 * time.Millisecond))

		if bucket.Take() {
			t.Error("expected take to fail during backoff")
		}

		// Wait for backoff to expire.
		time.Sleep(150 * time.Millisecond)

		if !bucket.Take() {
			t.Error("expected take to succeed after backoff")
		}
	})

	t.Run("wait", func(t *testing.T) {
		bucket := NewTokenBucket(1, 10)
		bucket.Take() // Empty the bucket.

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		start := time.Now()
		err := bucket.Wait(ctx)
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("expected wait to succeed: %v", err)
		}

		// Should have waited about 100ms for a token.
		if elapsed < 50*time.Millisecond {
			t.Errorf("expected some wait time, got %v", elapsed)
		}
	})

	t.Run("wait context cancelled", func(t *testing.T) {
		bucket := NewTokenBucket(0, 0.1) // Very slow refill.

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := bucket.Wait(ctx)
		if err != context.DeadlineExceeded {
			t.Errorf("expected context deadline exceeded, got %v", err)
		}
	})
}

// TestGlobalRateLimiter tests the global rate limiter.
func TestGlobalRateLimiter(t *testing.T) {
	t.Run("allow", func(t *testing.T) {
		limiter := NewGlobalRateLimiter(100, 10)

		// Should allow burst.
		for i := 0; i < 10; i++ {
			if !limiter.Allow() {
				t.Errorf("expected allow to succeed for request %d", i)
			}
		}
	})

	t.Run("backpressure", func(t *testing.T) {
		limiter := NewGlobalRateLimiter(100, 10)

		limiter.SetBackpressure(100 * time.Millisecond)

		if limiter.Allow() {
			t.Error("expected allow to fail during backpressure")
		}

		// Wait for backpressure to clear.
		time.Sleep(150 * time.Millisecond)

		if !limiter.Allow() {
			t.Error("expected allow to succeed after backpressure")
		}
	})

	t.Run("stats", func(t *testing.T) {
		limiter := NewGlobalRateLimiter(100, 5)

		for i := 0; i < 10; i++ {
			limiter.Allow()
		}

		stats := limiter.Stats()
		if stats.TotalRequests != 10 {
			t.Errorf("expected 10 total requests, got %d", stats.TotalRequests)
		}
		if stats.TotalRejected < 5 {
			t.Errorf("expected at least 5 rejections, got %d", stats.TotalRejected)
		}
	})
}

// TestWorkerPool tests the worker pool.
func TestWorkerPool(t *testing.T) {
	t.Run("start and stop", func(t *testing.T) {
		var taskCount int64
		handler := func(ctx context.Context, workerID string) error {
			atomic.AddInt64(&taskCount, 1)
			return nil
		}

		pool := NewWorkerPool(3, handler)
		ctx := context.Background()

		pool.Start(ctx)

		// Verify workers created.
		if pool.GetWorkerCount() != 3 {
			t.Errorf("expected 3 workers, got %d", pool.GetWorkerCount())
		}

		// Stop gracefully.
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := pool.Stop(stopCtx); err != nil {
			t.Errorf("stop failed: %v", err)
		}
	})

	t.Run("task dispatch", func(t *testing.T) {
		var taskCount int64
		taskCh := make(chan struct{}, 10)

		handler := func(ctx context.Context, workerID string) error {
			atomic.AddInt64(&taskCount, 1)
			taskCh <- struct{}{}
			return nil
		}

		pool := NewWorkerPool(2, handler)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool.Start(ctx)
		defer pool.Stop(context.Background()) //nolint:errcheck

		// Dispatch some tasks.
		for i := 0; i < 5; i++ {
			pool.Dispatch()
		}

		// Wait for tasks to complete.
		for i := 0; i < 5; i++ {
			select {
			case <-taskCh:
			case <-time.After(time.Second):
				t.Fatalf("timeout waiting for task %d", i)
			}
		}

		if atomic.LoadInt64(&taskCount) != 5 {
			t.Errorf("expected 5 tasks processed, got %d", taskCount)
		}
	})

	t.Run("stats", func(t *testing.T) {
		handler := func(ctx context.Context, workerID string) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		}

		pool := NewWorkerPool(2, handler)
		ctx := context.Background()

		pool.Start(ctx)
		defer pool.Stop(context.Background()) //nolint:errcheck

		stats := pool.GetPoolStats()
		if stats.TotalWorkers != 2 {
			t.Errorf("expected 2 total workers, got %d", stats.TotalWorkers)
		}
	})

	t.Run("health check", func(t *testing.T) {
		handler := func(ctx context.Context, workerID string) error {
			return nil
		}

		pool := NewWorkerPool(2, handler)
		ctx := context.Background()

		pool.Start(ctx)
		defer pool.Stop(context.Background()) //nolint:errcheck

		result := pool.HealthCheck()
		if !result.Healthy {
			t.Error("expected healthy pool")
		}
		if len(result.Workers) != 2 {
			t.Errorf("expected 2 workers in health check, got %d", len(result.Workers))
		}
	})
}

// TestTimingCalculator tests the timing calculator.
func TestTimingCalculator(t *testing.T) {
	t.Run("optimal sync time", func(t *testing.T) {
		calc := NewDefaultTimingCalculator(nil)
		now := time.Now()

		t.Run("never synced", func(t *testing.T) {
			account := &Account{
				ID:            "acc-1",
				ActivityLevel: 50,
			}
			syncTime := calc.OptimalSyncTime(account)

			// Should sync soon.
			if syncTime.After(now.Add(time.Minute)) {
				t.Error("expected sync time to be soon for never-synced account")
			}
		})

		t.Run("recently synced", func(t *testing.T) {
			account := &Account{
				ID:            "acc-2",
				ActivityLevel: 50,
				LastSyncAt:    now.Add(-5 * time.Minute),
				SyncFrequency: 30 * time.Minute,
			}
			syncTime := calc.OptimalSyncTime(account)

			// Should sync later.
			expectedMin := account.LastSyncAt.Add(account.SyncFrequency)
			if syncTime.Before(expectedMin.Add(-time.Minute)) {
				t.Error("expected sync time to respect sync frequency")
			}
		})

		t.Run("VIP account", func(t *testing.T) {
			normalAccount := &Account{
				ID:            "normal",
				ActivityLevel: 50,
				LastSyncAt:    now.Add(-30 * time.Minute),
			}
			vipAccount := &Account{
				ID:            "vip",
				IsVIP:         true,
				ActivityLevel: 50,
				LastSyncAt:    now.Add(-30 * time.Minute),
			}

			normalTime := calc.OptimalSyncTime(normalAccount)
			vipTime := calc.OptimalSyncTime(vipAccount)

			// VIP should sync sooner or at same time.
			if vipTime.After(normalTime.Add(time.Minute)) {
				t.Error("VIP account should sync at same time or sooner")
			}
		})

		t.Run("in backoff", func(t *testing.T) {
			backoffTime := now.Add(5 * time.Minute)
			account := &Account{
				ID:               "acc-backoff",
				RateLimitBackoff: backoffTime,
			}
			syncTime := calc.OptimalSyncTime(account)

			if syncTime.Before(backoffTime) {
				t.Error("sync time should respect rate limit backoff")
			}
		})
	})

	t.Run("quiet hours", func(t *testing.T) {
		calc := NewDefaultTimingCalculator(&DefaultTimingConfig{
			QuietHoursEnabled: true,
			DefaultQuietStart: 23,
			DefaultQuietEnd:   6,
		})

		// Create a time during quiet hours (2 AM).
		quietTime := time.Date(2024, 1, 15, 2, 0, 0, 0, time.UTC)
		account := &Account{ID: "acc-1"}

		if !calc.IsQuietHours(account, quietTime) {
			t.Error("expected 2 AM to be in quiet hours")
		}

		// Create a time outside quiet hours (10 AM).
		activeTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		if calc.IsQuietHours(account, activeTime) {
			t.Error("expected 10 AM to not be in quiet hours")
		}
	})

	t.Run("backoff calculation", func(t *testing.T) {
		calc := NewDefaultTimingCalculator(&DefaultTimingConfig{
			BaseBackoff:       time.Second,
			MaxBackoff:        time.Minute,
			BackoffMultiplier: 2.0,
			BackoffJitter:     0.1,
		})

		// First attempt.
		backoff1 := calc.BackoffDuration(0, time.Second)
		if backoff1 < 900*time.Millisecond || backoff1 > 1100*time.Millisecond {
			t.Errorf("unexpected first backoff: %v", backoff1)
		}

		// Second attempt (should be ~2x).
		backoff2 := calc.BackoffDuration(1, time.Second)
		if backoff2 < 1800*time.Millisecond || backoff2 > 2200*time.Millisecond {
			t.Errorf("unexpected second backoff: %v", backoff2)
		}

		// Should not exceed max.
		backoff10 := calc.BackoffDuration(10, time.Second)
		if backoff10 > 66*time.Second { // Max is 60s + 10% jitter.
			t.Errorf("backoff should be capped at max: %v", backoff10)
		}
	})
}

// TestIntelligentScheduler tests the intelligent scheduler.
func TestIntelligentScheduler(t *testing.T) {
	t.Run("schedule task", func(t *testing.T) {
		sched, err := NewIntelligentScheduler(nil)
		if err != nil {
			t.Fatalf("failed to create scheduler: %v", err)
		}

		task := &SyncTask{
			TenantID:  "tenant-1",
			AccountID: "acc-1",
			Priority:  Priority{Score: 50},
		}

		if err := sched.Schedule(task); err != nil {
			t.Errorf("schedule failed: %v", err)
		}

		if task.ID == "" {
			t.Error("expected task ID to be generated")
		}
		if task.Status != TaskStatusPending {
			t.Errorf("expected pending status, got %s", task.Status)
		}

		// Verify in queue.
		if sched.QueueDepth() != 1 {
			t.Errorf("expected queue depth 1, got %d", sched.QueueDepth())
		}
	})

	t.Run("reschedule task", func(t *testing.T) {
		sched, _ := NewIntelligentScheduler(nil)

		task := &SyncTask{
			ID:        "task-1",
			TenantID:  "tenant-1",
			AccountID: "acc-1",
			Priority:  Priority{Score: 30},
		}
		_ = sched.Schedule(task)

		// Reschedule with higher priority.
		newPriority := Priority{Score: 80}
		if err := sched.Reschedule("task-1", newPriority); err != nil {
			t.Errorf("reschedule failed: %v", err)
		}

		// Get task to verify.
		retrieved, _ := sched.GetTask("task-1")
		if retrieved.Priority.Score != 80 {
			t.Errorf("expected priority 80, got %d", retrieved.Priority.Score)
		}
	})

	t.Run("cancel task", func(t *testing.T) {
		sched, _ := NewIntelligentScheduler(nil)

		task := &SyncTask{
			ID:        "task-1",
			TenantID:  "tenant-1",
			AccountID: "acc-1",
		}
		_ = sched.Schedule(task)

		if err := sched.CancelTask("task-1"); err != nil {
			t.Errorf("cancel failed: %v", err)
		}

		retrieved, _ := sched.GetTask("task-1")
		if retrieved.Status != TaskStatusCancelled {
			t.Errorf("expected cancelled status, got %s", retrieved.Status)
		}
	})

	t.Run("list tasks", func(t *testing.T) {
		sched, _ := NewIntelligentScheduler(nil)

		for i := 0; i < 5; i++ {
			task := &SyncTask{
				TenantID:  "tenant-1",
				AccountID: "acc-1",
			}
			_ = sched.Schedule(task)
		}

		tasks := sched.ListTasks(TaskStatusPending, 10)
		if len(tasks) != 5 {
			t.Errorf("expected 5 tasks, got %d", len(tasks))
		}
	})

	t.Run("queue full", func(t *testing.T) {
		sched, _ := NewIntelligentScheduler(&SchedulerConfig{
			MaxQueueSize: 5,
		})

		for i := 0; i < 5; i++ {
			_ = sched.Schedule(&SyncTask{TenantID: "t", AccountID: "a"})
		}

		err := sched.Schedule(&SyncTask{TenantID: "t", AccountID: "a"})
		if err == nil {
			t.Error("expected error when queue is full")
		}
	})

	t.Run("schedule account", func(t *testing.T) {
		sched, _ := NewIntelligentScheduler(nil)

		account := &Account{
			ID:            "acc-1",
			TenantID:      "tenant-1",
			Email:         "test@example.com",
			IsVIP:         true,
			ActivityLevel: 80,
		}

		task, err := sched.ScheduleAccount(account, gsync.SyncTypeFull, nil)
		if err != nil {
			t.Errorf("schedule account failed: %v", err)
		}

		if task.AccountID != account.ID {
			t.Error("task should reference account")
		}
		if task.Priority.Score == 0 {
			t.Error("priority should be calculated")
		}
	})

	t.Run("start and stop", func(t *testing.T) {
		sched, _ := NewIntelligentScheduler(&SchedulerConfig{
			WorkerPoolSize: 2,
		})

		ctx := context.Background()
		if err := sched.Start(ctx); err != nil {
			t.Errorf("start failed: %v", err)
		}

		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := sched.Stop(stopCtx); err != nil {
			t.Errorf("stop failed: %v", err)
		}
	})
}

// TestTenantThrottler tests tenant-specific throttling.
func TestTenantThrottler(t *testing.T) {
	t.Run("basic throttling", func(t *testing.T) {
		throttler := NewTenantThrottler(5, 1)

		// Should allow burst for new tenant.
		for i := 0; i < 5; i++ {
			if !throttler.Allow("tenant-1") {
				t.Errorf("expected allow for request %d", i)
			}
		}

		// 6th should be denied.
		if throttler.Allow("tenant-1") {
			t.Error("expected deny when bucket empty")
		}
	})

	t.Run("per-tenant isolation", func(t *testing.T) {
		throttler := NewTenantThrottler(5, 1)

		// Exhaust tenant-1.
		for i := 0; i < 5; i++ {
			throttler.Allow("tenant-1")
		}

		// tenant-2 should still have tokens.
		if !throttler.Allow("tenant-2") {
			t.Error("tenant-2 should have its own bucket")
		}
	})

	t.Run("override", func(t *testing.T) {
		throttler := NewTenantThrottler(5, 1)

		// Set higher limit for VIP tenant.
		throttler.SetOverride("vip-tenant", ThrottleConfig{
			MaxTokens:  10,
			RefillRate: 5,
		})

		// Should allow more than default.
		count := 0
		for i := 0; i < 10; i++ {
			if throttler.Allow("vip-tenant") {
				count++
			}
		}

		if count < 10 {
			t.Errorf("expected 10 allows for VIP tenant, got %d", count)
		}
	})
}

// TestBackoffCalculator tests the backoff calculator.
func TestBackoffCalculator(t *testing.T) {
	t.Run("exponential backoff", func(t *testing.T) {
		calc := &BackoffCalculator{
			BaseBackoff: time.Second,
			MaxBackoff:  time.Minute,
			Multiplier:  2.0,
			Jitter:      0,
		}

		backoff1 := calc.Calculate(1)
		backoff2 := calc.Calculate(2)
		backoff3 := calc.Calculate(3)

		if backoff1 != time.Second {
			t.Errorf("expected 1s, got %v", backoff1)
		}
		if backoff2 != 2*time.Second {
			t.Errorf("expected 2s, got %v", backoff2)
		}
		if backoff3 != 4*time.Second {
			t.Errorf("expected 4s, got %v", backoff3)
		}
	})

	t.Run("max backoff", func(t *testing.T) {
		calc := &BackoffCalculator{
			BaseBackoff: time.Second,
			MaxBackoff:  5 * time.Second,
			Multiplier:  2.0,
			Jitter:      0,
		}

		backoff := calc.Calculate(10)
		if backoff > 5*time.Second {
			t.Errorf("expected max 5s, got %v", backoff)
		}
	})

	t.Run("with retry after", func(t *testing.T) {
		calc := NewBackoffCalculator()

		// Retry-after is longer.
		retryAfter := 10 * time.Second
		backoff := calc.CalculateWithRetryAfter(1, retryAfter)
		if backoff != retryAfter {
			t.Errorf("expected retry-after to be used: %v", backoff)
		}

		// Calculated is longer.
		shortRetry := 100 * time.Millisecond
		backoff = calc.CalculateWithRetryAfter(1, shortRetry)
		if backoff < shortRetry {
			t.Error("expected calculated backoff to be used")
		}
	})
}

// TestTaskDistributors tests task distribution strategies.
func TestTaskDistributors(t *testing.T) {
	createWorkers := func(n int) []*Worker {
		workers := make([]*Worker, n)
		for i := 0; i < n; i++ {
			workers[i] = NewWorker(string(rune('A' + i)))
			workers[i].SetStatus(WorkerStatusIdle)
		}
		return workers
	}

	t.Run("round robin", func(t *testing.T) {
		dist := NewRoundRobinDistributor()
		workers := createWorkers(3)

		task := &SyncTask{ID: "task-1"}

		// Should select workers in order.
		w1 := dist.SelectWorker(task, workers)
		w1.SetStatus(WorkerStatusRunning)

		w2 := dist.SelectWorker(task, workers)
		w2.SetStatus(WorkerStatusRunning)

		w3 := dist.SelectWorker(task, workers)
		w3.SetStatus(WorkerStatusRunning)

		// All three should be different.
		if w1.ID == w2.ID || w2.ID == w3.ID || w1.ID == w3.ID {
			t.Error("round robin should select different workers")
		}
	})

	t.Run("least loaded", func(t *testing.T) {
		dist := NewLeastLoadedDistributor()
		workers := createWorkers(3)

		// Set different task counts.
		atomic.StoreInt64(&workers[0].TasksProcessed, 10)
		atomic.StoreInt64(&workers[1].TasksProcessed, 5)
		atomic.StoreInt64(&workers[2].TasksProcessed, 15)

		task := &SyncTask{ID: "task-1"}
		selected := dist.SelectWorker(task, workers)

		if selected.ID != workers[1].ID {
			t.Errorf("expected worker B (least loaded), got %s", selected.ID)
		}
	})

	t.Run("affinity", func(t *testing.T) {
		dist := NewAffinityDistributor()
		workers := createWorkers(3)

		task1 := &SyncTask{ID: "task-1", TenantID: "tenant-1"}
		task2 := &SyncTask{ID: "task-2", TenantID: "tenant-1"}

		// First task creates affinity.
		w1 := dist.SelectWorker(task1, workers)

		// Second task for same tenant should prefer same worker.
		w2 := dist.SelectWorker(task2, workers)

		if w1.ID != w2.ID {
			t.Error("affinity distributor should prefer same worker for same tenant")
		}
	})
}

// TestBackpressureSignaler tests backpressure signaling.
func TestBackpressureSignaler(t *testing.T) {
	t.Run("threshold trigger", func(t *testing.T) {
		signaler := NewBackpressureSignaler(0.8)

		// Below threshold.
		signaler.UpdateLoad(0.5)
		if signaler.IsBackpressured() {
			t.Error("expected no backpressure at 0.5")
		}

		// At threshold.
		signaler.UpdateLoad(0.8)
		if !signaler.IsBackpressured() {
			t.Error("expected backpressure at 0.8")
		}

		// Below threshold again.
		signaler.UpdateLoad(0.7)
		if signaler.IsBackpressured() {
			t.Error("expected no backpressure at 0.7")
		}
	})

	t.Run("subscribe", func(t *testing.T) {
		signaler := NewBackpressureSignaler(0.8)
		ch := signaler.Subscribe()

		signaler.UpdateLoad(0.9)

		select {
		case state := <-ch:
			if !state {
				t.Error("expected backpressure signal")
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("expected to receive signal")
		}
	})
}

// BenchmarkPriorityCalculation benchmarks priority calculation.
func BenchmarkPriorityCalculation(b *testing.B) {
	account := &Account{
		ID:            "acc-1",
		TenantID:      "tenant-1",
		IsVIP:         true,
		ActivityLevel: 75,
		LastSyncAt:    time.Now().Add(-30 * time.Minute),
	}
	ctx := PriorityContext{
		QueueDepth:  500,
		CurrentTime: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DefaultPriorityCalculator(account, ctx)
	}
}

// BenchmarkPriorityQueue benchmarks priority queue operations.
func BenchmarkPriorityQueue(b *testing.B) {
	b.Run("push", func(b *testing.B) {
		pq := NewPriorityQueue()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			task := &SyncTask{
				ID:       string(rune('a' + (i % 26))),
				Priority: Priority{Score: i % 100},
			}
			pq.PushTask(task)
		}
	})

	b.Run("push-pop", func(b *testing.B) {
		pq := NewPriorityQueue()
		// Pre-populate.
		for i := 0; i < 1000; i++ {
			pq.PushTask(&SyncTask{
				ID:       string(rune('a' + (i % 26))),
				Priority: Priority{Score: i % 100},
			})
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			pq.PushTask(&SyncTask{
				ID:       "x",
				Priority: Priority{Score: i % 100},
			})
			pq.PopTask()
		}
	})
}

// BenchmarkTokenBucket benchmarks token bucket operations.
func BenchmarkTokenBucket(b *testing.B) {
	bucket := NewTokenBucket(1000, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bucket.Take()
	}
}
