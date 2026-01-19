package pattern

import (
	"context"
	"testing"
	"time"
)

func TestNewInMemoryPatternStore(t *testing.T) {
	store := NewInMemoryPatternStore()

	if store == nil {
		t.Fatal("expected store to be created")
	}
}

func TestInMemoryPatternStoreSave(t *testing.T) {
	store := NewInMemoryPatternStore()
	ctx := context.Background()

	pattern := NewPattern("tenant-1", "Test Pattern", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})

	err := store.Save(ctx, pattern)
	if err != nil {
		t.Fatalf("failed to save pattern: %v", err)
	}

	// Try to save again (should fail with duplicate)
	err = store.Save(ctx, pattern)
	if err != ErrPatternExists {
		t.Errorf("expected ErrPatternExists, got %v", err)
	}
}

func TestInMemoryPatternStoreSaveNil(t *testing.T) {
	store := NewInMemoryPatternStore()
	ctx := context.Background()

	err := store.Save(ctx, nil)
	if err != ErrInvalidPattern {
		t.Errorf("expected ErrInvalidPattern, got %v", err)
	}
}

func TestInMemoryPatternStoreGet(t *testing.T) {
	store := NewInMemoryPatternStore()
	ctx := context.Background()

	pattern := NewPattern("tenant-1", "Test Pattern", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	store.Save(ctx, pattern)

	// Get existing pattern
	retrieved, err := store.Get(ctx, "tenant-1", pattern.ID)
	if err != nil {
		t.Fatalf("failed to get pattern: %v", err)
	}
	if retrieved.ID != pattern.ID {
		t.Errorf("expected ID %s, got %s", pattern.ID, retrieved.ID)
	}
	if retrieved.Name != pattern.Name {
		t.Errorf("expected name %s, got %s", pattern.Name, retrieved.Name)
	}

	// Get non-existent pattern
	_, err = store.Get(ctx, "tenant-1", "nonexistent")
	if err != ErrPatternNotFound {
		t.Errorf("expected ErrPatternNotFound, got %v", err)
	}

	// Get from non-existent tenant
	_, err = store.Get(ctx, "nonexistent-tenant", pattern.ID)
	if err != ErrPatternNotFound {
		t.Errorf("expected ErrPatternNotFound, got %v", err)
	}
}

func TestInMemoryPatternStoreList(t *testing.T) {
	store := NewInMemoryPatternStore()
	ctx := context.Background()

	// Create patterns
	for i := 0; i < 5; i++ {
		pattern := NewPattern("tenant-1", "Pattern "+string(rune('A'+i)), PatternSender, []PatternCondition{
			NewPatternCondition("sender", OpEquals, "test@example.com"),
		})
		store.Save(ctx, pattern)
	}

	// List all
	patterns, err := store.List(ctx, "tenant-1", 0, 100)
	if err != nil {
		t.Fatalf("failed to list patterns: %v", err)
	}
	if len(patterns) != 5 {
		t.Errorf("expected 5 patterns, got %d", len(patterns))
	}

	// List with pagination
	patterns, err = store.List(ctx, "tenant-1", 0, 2)
	if err != nil {
		t.Fatalf("failed to list patterns with limit: %v", err)
	}
	if len(patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(patterns))
	}

	// List with offset
	patterns, err = store.List(ctx, "tenant-1", 3, 10)
	if err != nil {
		t.Fatalf("failed to list patterns with offset: %v", err)
	}
	if len(patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(patterns))
	}

	// List non-existent tenant
	patterns, err = store.List(ctx, "nonexistent", 0, 100)
	if err != nil {
		t.Fatalf("failed to list from non-existent tenant: %v", err)
	}
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns, got %d", len(patterns))
	}
}

func TestInMemoryPatternStoreDelete(t *testing.T) {
	store := NewInMemoryPatternStore()
	ctx := context.Background()

	pattern := NewPattern("tenant-1", "Test Pattern", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	store.Save(ctx, pattern)

	// Delete pattern
	err := store.Delete(ctx, "tenant-1", pattern.ID)
	if err != nil {
		t.Fatalf("failed to delete pattern: %v", err)
	}

	// Verify deletion
	_, err = store.Get(ctx, "tenant-1", pattern.ID)
	if err != ErrPatternNotFound {
		t.Errorf("expected ErrPatternNotFound after deletion, got %v", err)
	}

	// Delete non-existent
	err = store.Delete(ctx, "tenant-1", "nonexistent")
	if err != ErrPatternNotFound {
		t.Errorf("expected ErrPatternNotFound, got %v", err)
	}

	// Delete from non-existent tenant
	err = store.Delete(ctx, "nonexistent", pattern.ID)
	if err != ErrPatternNotFound {
		t.Errorf("expected ErrPatternNotFound, got %v", err)
	}
}

func TestInMemoryPatternStoreUpdate(t *testing.T) {
	store := NewInMemoryPatternStore()
	ctx := context.Background()

	pattern := NewPattern("tenant-1", "Original Name", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	store.Save(ctx, pattern)

	// Update pattern
	pattern.Name = "Updated Name"
	pattern.Description = "Added description"

	err := store.Update(ctx, pattern)
	if err != nil {
		t.Fatalf("failed to update pattern: %v", err)
	}

	// Verify update
	retrieved, _ := store.Get(ctx, "tenant-1", pattern.ID)
	if retrieved.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got '%s'", retrieved.Name)
	}
	if retrieved.Description != "Added description" {
		t.Errorf("expected description 'Added description', got '%s'", retrieved.Description)
	}

	// Update non-existent
	nonexistent := NewPattern("tenant-1", "Nonexistent", PatternSender, []PatternCondition{})
	nonexistent.ID = "nonexistent"
	err = store.Update(ctx, nonexistent)
	if err != ErrPatternNotFound {
		t.Errorf("expected ErrPatternNotFound, got %v", err)
	}

	// Update nil
	err = store.Update(ctx, nil)
	if err != ErrInvalidPattern {
		t.Errorf("expected ErrInvalidPattern, got %v", err)
	}
}

func TestInMemoryPatternStoreGetByType(t *testing.T) {
	store := NewInMemoryPatternStore()
	ctx := context.Background()

	// Create patterns of different types
	p1 := NewPattern("tenant-1", "Sender 1", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	p2 := NewPattern("tenant-1", "Sender 2", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "other@example.com"),
	})
	p3 := NewPattern("tenant-1", "Topic 1", PatternTopic, []PatternCondition{
		NewPatternCondition("category", OpEquals, "work"),
	})

	store.Save(ctx, p1)
	store.Save(ctx, p2)
	store.Save(ctx, p3)

	// Get sender patterns
	senderPatterns, err := store.GetByType(ctx, "tenant-1", PatternSender)
	if err != nil {
		t.Fatalf("failed to get by type: %v", err)
	}
	if len(senderPatterns) != 2 {
		t.Errorf("expected 2 sender patterns, got %d", len(senderPatterns))
	}

	// Get topic patterns
	topicPatterns, err := store.GetByType(ctx, "tenant-1", PatternTopic)
	if err != nil {
		t.Fatalf("failed to get by type: %v", err)
	}
	if len(topicPatterns) != 1 {
		t.Errorf("expected 1 topic pattern, got %d", len(topicPatterns))
	}

	// Get non-existent type
	timePatterns, err := store.GetByType(ctx, "tenant-1", PatternTime)
	if err != nil {
		t.Fatalf("failed to get by type: %v", err)
	}
	if len(timePatterns) != 0 {
		t.Errorf("expected 0 time patterns, got %d", len(timePatterns))
	}
}

func TestInMemoryPatternStoreGetEnabled(t *testing.T) {
	store := NewInMemoryPatternStore()
	ctx := context.Background()

	p1 := NewPattern("tenant-1", "Enabled", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	p1.Enabled = true

	p2 := NewPattern("tenant-1", "Disabled", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "other@example.com"),
	})
	p2.Enabled = false

	store.Save(ctx, p1)
	store.Save(ctx, p2)

	enabled, err := store.GetEnabled(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("failed to get enabled: %v", err)
	}
	if len(enabled) != 1 {
		t.Errorf("expected 1 enabled pattern, got %d", len(enabled))
	}
	if enabled[0].Name != "Enabled" {
		t.Errorf("expected 'Enabled' pattern, got '%s'", enabled[0].Name)
	}
}

func TestInMemoryPatternStoreGetHotPatterns(t *testing.T) {
	store := NewInMemoryPatternStore()
	ctx := context.Background()

	p1 := NewPattern("tenant-1", "Hot Pattern", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	p1.Stats.TotalMatches = 150

	p2 := NewPattern("tenant-1", "Cold Pattern", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "other@example.com"),
	})
	p2.Stats.TotalMatches = 10

	store.Save(ctx, p1)
	store.Save(ctx, p2)

	hot, err := store.GetHotPatterns(ctx, "tenant-1", 100)
	if err != nil {
		t.Fatalf("failed to get hot patterns: %v", err)
	}
	if len(hot) != 1 {
		t.Errorf("expected 1 hot pattern, got %d", len(hot))
	}
	if hot[0].Name != "Hot Pattern" {
		t.Errorf("expected 'Hot Pattern', got '%s'", hot[0].Name)
	}
}

func TestInMemoryPatternStoreIsolation(t *testing.T) {
	store := NewInMemoryPatternStore()
	ctx := context.Background()

	// Save patterns for different tenants
	p1 := NewPattern("tenant-1", "Tenant 1 Pattern", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	p2 := NewPattern("tenant-2", "Tenant 2 Pattern", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "other@example.com"),
	})

	store.Save(ctx, p1)
	store.Save(ctx, p2)

	// Verify tenant isolation
	tenant1Patterns, _ := store.List(ctx, "tenant-1", 0, 100)
	tenant2Patterns, _ := store.List(ctx, "tenant-2", 0, 100)

	if len(tenant1Patterns) != 1 {
		t.Errorf("expected 1 pattern for tenant-1, got %d", len(tenant1Patterns))
	}
	if len(tenant2Patterns) != 1 {
		t.Errorf("expected 1 pattern for tenant-2, got %d", len(tenant2Patterns))
	}
}

func TestCachedPatternStore(t *testing.T) {
	baseStore := NewInMemoryPatternStore()
	cachedStore := NewCachedPatternStore(baseStore, 5*time.Minute)

	if cachedStore == nil {
		t.Fatal("expected cached store to be created")
	}
}

func TestCachedPatternStoreSaveAndGet(t *testing.T) {
	baseStore := NewInMemoryPatternStore()
	cachedStore := NewCachedPatternStore(baseStore, 5*time.Minute)
	ctx := context.Background()

	pattern := NewPattern("tenant-1", "Test Pattern", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})

	// Save through cached store
	err := cachedStore.Save(ctx, pattern)
	if err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Get should hit cache
	retrieved, err := cachedStore.Get(ctx, "tenant-1", pattern.ID)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if retrieved.ID != pattern.ID {
		t.Errorf("expected ID %s, got %s", pattern.ID, retrieved.ID)
	}
}

func TestCachedPatternStoreUpdate(t *testing.T) {
	baseStore := NewInMemoryPatternStore()
	cachedStore := NewCachedPatternStore(baseStore, 5*time.Minute)
	ctx := context.Background()

	pattern := NewPattern("tenant-1", "Original", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	cachedStore.Save(ctx, pattern)

	// Update
	pattern.Name = "Updated"
	err := cachedStore.Update(ctx, pattern)
	if err != nil {
		t.Fatalf("failed to update: %v", err)
	}

	// Get should return updated value
	retrieved, _ := cachedStore.Get(ctx, "tenant-1", pattern.ID)
	if retrieved.Name != "Updated" {
		t.Errorf("expected name 'Updated', got '%s'", retrieved.Name)
	}
}

func TestCachedPatternStoreDelete(t *testing.T) {
	baseStore := NewInMemoryPatternStore()
	cachedStore := NewCachedPatternStore(baseStore, 5*time.Minute)
	ctx := context.Background()

	pattern := NewPattern("tenant-1", "Test", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	cachedStore.Save(ctx, pattern)

	// Delete
	err := cachedStore.Delete(ctx, "tenant-1", pattern.ID)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// Get should fail
	_, err = cachedStore.Get(ctx, "tenant-1", pattern.ID)
	if err != ErrPatternNotFound {
		t.Errorf("expected ErrPatternNotFound after delete, got %v", err)
	}
}

func TestCachedPatternStoreInvalidateCache(t *testing.T) {
	baseStore := NewInMemoryPatternStore()
	cachedStore := NewCachedPatternStore(baseStore, 5*time.Minute)
	ctx := context.Background()

	pattern := NewPattern("tenant-1", "Test", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	cachedStore.Save(ctx, pattern)

	// Populate cache
	cachedStore.Get(ctx, "tenant-1", pattern.ID)

	// Invalidate
	cachedStore.InvalidateCache()

	// Cache should be empty (will fetch from store)
	retrieved, err := cachedStore.Get(ctx, "tenant-1", pattern.ID)
	if err != nil {
		t.Fatalf("failed to get after invalidate: %v", err)
	}
	if retrieved.ID != pattern.ID {
		t.Error("expected to retrieve pattern from store after cache invalidation")
	}
}

func TestCachedPatternStoreList(t *testing.T) {
	baseStore := NewInMemoryPatternStore()
	cachedStore := NewCachedPatternStore(baseStore, 5*time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		pattern := NewPattern("tenant-1", "Pattern "+string(rune('A'+i)), PatternSender, []PatternCondition{
			NewPatternCondition("sender", OpEquals, "test@example.com"),
		})
		cachedStore.Save(ctx, pattern)
	}

	// List goes directly to store
	patterns, err := cachedStore.List(ctx, "tenant-1", 0, 100)
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(patterns) != 3 {
		t.Errorf("expected 3 patterns, got %d", len(patterns))
	}
}

func TestCachedPatternStoreGetByType(t *testing.T) {
	baseStore := NewInMemoryPatternStore()
	cachedStore := NewCachedPatternStore(baseStore, 5*time.Minute)
	ctx := context.Background()

	p1 := NewPattern("tenant-1", "Sender", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	p2 := NewPattern("tenant-1", "Topic", PatternTopic, []PatternCondition{
		NewPatternCondition("category", OpEquals, "work"),
	})

	cachedStore.Save(ctx, p1)
	cachedStore.Save(ctx, p2)

	senderPatterns, err := cachedStore.GetByType(ctx, "tenant-1", PatternSender)
	if err != nil {
		t.Fatalf("failed to get by type: %v", err)
	}
	if len(senderPatterns) != 1 {
		t.Errorf("expected 1 sender pattern, got %d", len(senderPatterns))
	}
}

func TestCachedPatternStoreGetEnabled(t *testing.T) {
	baseStore := NewInMemoryPatternStore()
	cachedStore := NewCachedPatternStore(baseStore, 5*time.Minute)
	ctx := context.Background()

	p1 := NewPattern("tenant-1", "Enabled", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	p1.Enabled = true

	p2 := NewPattern("tenant-1", "Disabled", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "other@example.com"),
	})
	p2.Enabled = false

	cachedStore.Save(ctx, p1)
	cachedStore.Save(ctx, p2)

	enabled, err := cachedStore.GetEnabled(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("failed to get enabled: %v", err)
	}
	if len(enabled) != 1 {
		t.Errorf("expected 1 enabled pattern, got %d", len(enabled))
	}
}

func TestCachedPatternStoreGetHotPatterns(t *testing.T) {
	baseStore := NewInMemoryPatternStore()
	cachedStore := NewCachedPatternStore(baseStore, 5*time.Minute)
	ctx := context.Background()

	p1 := NewPattern("tenant-1", "Hot", PatternSender, []PatternCondition{
		NewPatternCondition("sender", OpEquals, "test@example.com"),
	})
	p1.Stats.TotalMatches = 200

	cachedStore.Save(ctx, p1)

	hot, err := cachedStore.GetHotPatterns(ctx, "tenant-1", 100)
	if err != nil {
		t.Fatalf("failed to get hot patterns: %v", err)
	}
	if len(hot) != 1 {
		t.Errorf("expected 1 hot pattern, got %d", len(hot))
	}
}

func TestContainsString(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello world", "world", true},
		{"hello world", "hello", true},
		{"hello world", "foo", false},
		{"hello", "hello", true},
		{"hello", "", true},
		{"", "hello", false},
		{"", "", true},
	}

	for _, tt := range tests {
		result := containsString(tt.s, tt.substr)
		if result != tt.expected {
			t.Errorf("containsString(%q, %q): expected %v, got %v", tt.s, tt.substr, tt.expected, result)
		}
	}
}
