package undo

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryStoreSaveGetStack(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	stack := NewStack(&StackConfig{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		SessionID: "session-1",
	})

	// Save stack
	if err := store.SaveStack(ctx, stack); err != nil {
		t.Fatalf("SaveStack failed: %v", err)
	}

	// Get stack
	retrieved, err := store.GetStack(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetStack failed: %v", err)
	}

	if retrieved.SessionID() != "session-1" {
		t.Errorf("expected session ID 'session-1', got '%s'", retrieved.SessionID())
	}
}

func TestInMemoryStoreGetStackNotFound(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	_, err := store.GetStack(ctx, "nonexistent")
	if err != ErrStackNotFound {
		t.Errorf("expected ErrStackNotFound, got %v", err)
	}
}

func TestInMemoryStoreGetStackByUser(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	// Create stacks for different users
	stack1 := NewStack(&StackConfig{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		SessionID: "session-1",
	})
	stack2 := NewStack(&StackConfig{
		TenantID:  "tenant-1",
		UserID:    "user-1",
		SessionID: "session-2",
	})
	stack3 := NewStack(&StackConfig{
		TenantID:  "tenant-1",
		UserID:    "user-2",
		SessionID: "session-3",
	})

	_ = store.SaveStack(ctx, stack1)
	_ = store.SaveStack(ctx, stack2)
	_ = store.SaveStack(ctx, stack3)

	// Get stacks for user-1
	stacks, err := store.GetStackByUser(ctx, "tenant-1", "user-1")
	if err != nil {
		t.Fatalf("GetStackByUser failed: %v", err)
	}

	if len(stacks) != 2 {
		t.Errorf("expected 2 stacks for user-1, got %d", len(stacks))
	}
}

func TestInMemoryStoreDeleteStack(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	stack := NewStack(&StackConfig{SessionID: "session-1"})
	_ = store.SaveStack(ctx, stack)

	// Delete
	if err := store.DeleteStack(ctx, "session-1"); err != nil {
		t.Fatalf("DeleteStack failed: %v", err)
	}

	// Verify deleted
	_, err := store.GetStack(ctx, "session-1")
	if err != ErrStackNotFound {
		t.Errorf("expected ErrStackNotFound after delete, got %v", err)
	}
}

func TestInMemoryStoreDeleteStackNotFound(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	err := store.DeleteStack(ctx, "nonexistent")
	if err != ErrStackNotFound {
		t.Errorf("expected ErrStackNotFound, got %v", err)
	}
}

func TestInMemoryStoreSaveGetActions(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	action1 := NewAcceptAction("t", "u", "session-1", "c1", "email", "pending")
	action2 := NewAcceptAction("t", "u", "session-1", "c2", "email", "pending")
	action3 := NewAcceptAction("t", "u", "session-2", "c3", "email", "pending")

	_ = store.SaveAction(ctx, action1)
	_ = store.SaveAction(ctx, action2)
	_ = store.SaveAction(ctx, action3)

	// Get actions for session-1
	actions, err := store.GetActions(ctx, "session-1", 10)
	if err != nil {
		t.Fatalf("GetActions failed: %v", err)
	}

	if len(actions) != 2 {
		t.Errorf("expected 2 actions for session-1, got %d", len(actions))
	}
}

func TestInMemoryStorePruneExpiredStacks(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	stack := NewStack(&StackConfig{SessionID: "session-1"})
	_ = store.SaveStack(ctx, stack)

	// Prune with short TTL
	time.Sleep(10 * time.Millisecond)
	pruned, err := store.PruneExpiredStacks(ctx, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("PruneExpiredStacks failed: %v", err)
	}

	if pruned != 1 {
		t.Errorf("expected 1 pruned stack, got %d", pruned)
	}

	_, err = store.GetStack(ctx, "session-1")
	if err != ErrStackNotFound {
		t.Errorf("expected stack to be pruned")
	}
}

func TestInMemoryStoreClear(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	stack := NewStack(&StackConfig{SessionID: "session-1"})
	action := NewAcceptAction("t", "u", "session-1", "c", "email", "pending")
	_ = store.SaveStack(ctx, stack)
	_ = store.SaveAction(ctx, action)

	store.Clear()

	_, err := store.GetStack(ctx, "session-1")
	if err != ErrStackNotFound {
		t.Error("expected store to be cleared")
	}
}

func TestMemoryCache(t *testing.T) {
	config := &CacheConfig{
		MaxEntries: 3,
		TTL:        100 * time.Millisecond,
	}
	cache := NewMemoryCache(config)

	stack1 := NewStack(&StackConfig{SessionID: "session-1"})
	stack2 := NewStack(&StackConfig{SessionID: "session-2"})
	stack3 := NewStack(&StackConfig{SessionID: "session-3"})

	cache.Set("session-1", stack1)
	cache.Set("session-2", stack2)
	cache.Set("session-3", stack3)

	if cache.Size() != 3 {
		t.Errorf("expected cache size 3, got %d", cache.Size())
	}

	// Get cached
	retrieved := cache.Get("session-2")
	if retrieved == nil {
		t.Error("expected to get cached stack")
	}
	if retrieved.SessionID() != "session-2" {
		t.Errorf("expected session ID 'session-2', got '%s'", retrieved.SessionID())
	}
}

func TestMemoryCacheEviction(t *testing.T) {
	config := &CacheConfig{
		MaxEntries: 2,
		TTL:        1 * time.Hour,
	}
	cache := NewMemoryCache(config)

	stack1 := NewStack(&StackConfig{SessionID: "session-1"})
	stack2 := NewStack(&StackConfig{SessionID: "session-2"})
	stack3 := NewStack(&StackConfig{SessionID: "session-3"})

	cache.Set("session-1", stack1)
	cache.Set("session-2", stack2)
	cache.Set("session-3", stack3) // Should evict oldest

	if cache.Size() != 2 {
		t.Errorf("expected cache size 2 after eviction, got %d", cache.Size())
	}

	// session-3 should be present
	if cache.Get("session-3") == nil {
		t.Error("session-3 should be in cache")
	}
}

func TestMemoryCacheTTL(t *testing.T) {
	config := &CacheConfig{
		MaxEntries: 10,
		TTL:        10 * time.Millisecond,
	}
	cache := NewMemoryCache(config)

	stack := NewStack(&StackConfig{SessionID: "session-1"})
	cache.Set("session-1", stack)

	// Wait for TTL
	time.Sleep(20 * time.Millisecond)

	if cache.Get("session-1") != nil {
		t.Error("expected cached stack to be expired")
	}
}

func TestMemoryCacheDelete(t *testing.T) {
	cache := NewMemoryCache(DefaultCacheConfig())

	stack := NewStack(&StackConfig{SessionID: "session-1"})
	cache.Set("session-1", stack)

	cache.Delete("session-1")

	if cache.Get("session-1") != nil {
		t.Error("expected stack to be deleted from cache")
	}
}

func TestMemoryCacheClear(t *testing.T) {
	cache := NewMemoryCache(DefaultCacheConfig())

	cache.Set("session-1", NewStack(&StackConfig{SessionID: "session-1"}))
	cache.Set("session-2", NewStack(&StackConfig{SessionID: "session-2"}))

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("expected cache size 0 after clear, got %d", cache.Size())
	}
}

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()

	if config.MaxEntries != 1000 {
		t.Errorf("expected max entries 1000, got %d", config.MaxEntries)
	}
	if config.TTL != 15*time.Minute {
		t.Errorf("expected TTL 15m, got %v", config.TTL)
	}
}
