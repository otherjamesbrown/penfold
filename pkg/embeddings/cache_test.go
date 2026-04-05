package embeddings

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestHashKey(t *testing.T) {
	t.Run("consistent hashing", func(t *testing.T) {
		key1 := hashKey("test")
		key2 := hashKey("test")
		if key1 != key2 {
			t.Error("hashKey should return consistent results")
		}
	})

	t.Run("different inputs different keys", func(t *testing.T) {
		key1 := hashKey("test1")
		key2 := hashKey("test2")
		if key1 == key2 {
			t.Error("hashKey should return different results for different inputs")
		}
	})

	t.Run("key is hex encoded SHA-256", func(t *testing.T) {
		key := hashKey("test")
		// SHA-256 produces 32 bytes = 64 hex characters
		if len(key) != 64 {
			t.Errorf("hashKey length = %d, want 64", len(key))
		}
	})
}

func TestDefaultMemoryCacheConfig(t *testing.T) {
	cfg := DefaultMemoryCacheConfig()

	if cfg.MaxSize <= 0 {
		t.Error("MaxSize should be positive")
	}
	if cfg.TTL <= 0 {
		t.Error("TTL should be positive")
	}
}

func TestNewMemoryCache(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		cache := NewMemoryCache(nil)
		if cache == nil {
			t.Fatal("NewMemoryCache(nil) returned nil")
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &MemoryCacheConfig{
			MaxSize: 100,
			TTL:     time.Minute,
		}
		cache := NewMemoryCache(cfg)
		if cache == nil {
			t.Fatal("NewMemoryCache() returned nil")
		}
	})
}

func TestMemoryCache_GetSet(t *testing.T) {
	cache := NewMemoryCache(nil)
	ctx := context.Background()

	t.Run("miss on empty cache", func(t *testing.T) {
		_, err := cache.Get(ctx, "nonexistent")
		if !errors.Is(err, ErrCacheMiss) {
			t.Errorf("Get() error = %v, want ErrCacheMiss", err)
		}
	})

	t.Run("set and get", func(t *testing.T) {
		embedding := []float32{1.0, 2.0, 3.0}
		err := cache.Set(ctx, "test", embedding)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		result, err := cache.Get(ctx, "test")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if len(result) != len(embedding) {
			t.Errorf("Get() returned %d elements, want %d", len(result), len(embedding))
		}
		for i, v := range result {
			if v != embedding[i] {
				t.Errorf("result[%d] = %f, want %f", i, v, embedding[i])
			}
		}
	})

	t.Run("returns copy not reference", func(t *testing.T) {
		embedding := []float32{1.0, 2.0, 3.0}
		_ = cache.Set(ctx, "copy-test", embedding)

		result, _ := cache.Get(ctx, "copy-test")
		result[0] = 999.0 // Modify result

		result2, _ := cache.Get(ctx, "copy-test")
		if result2[0] == 999.0 {
			t.Error("Cache should return copies, not references")
		}
	})

	t.Run("update existing entry", func(t *testing.T) {
		_ = cache.Set(ctx, "update", []float32{1.0})
		_ = cache.Set(ctx, "update", []float32{2.0})

		result, _ := cache.Get(ctx, "update")
		if result[0] != 2.0 {
			t.Errorf("Updated value = %f, want 2.0", result[0])
		}
	})
}

func TestMemoryCache_TTL(t *testing.T) {
	cfg := &MemoryCacheConfig{
		MaxSize: 100,
		TTL:     50 * time.Millisecond,
	}
	cache := NewMemoryCache(cfg)
	ctx := context.Background()

	embedding := []float32{1.0, 2.0, 3.0}
	_ = cache.Set(ctx, "expires", embedding)

	// Should exist immediately
	_, err := cache.Get(ctx, "expires")
	if err != nil {
		t.Errorf("Get() immediately after Set() error = %v", err)
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired
	_, err = cache.Get(ctx, "expires")
	if !errors.Is(err, ErrCacheMiss) {
		t.Errorf("Get() after TTL error = %v, want ErrCacheMiss", err)
	}
}

func TestMemoryCache_LRU(t *testing.T) {
	cfg := &MemoryCacheConfig{
		MaxSize: 3,
		TTL:     0, // No TTL
	}
	cache := NewMemoryCache(cfg)
	ctx := context.Background()

	// Fill cache
	_ = cache.Set(ctx, "a", []float32{1.0})
	_ = cache.Set(ctx, "b", []float32{2.0})
	_ = cache.Set(ctx, "c", []float32{3.0})

	// Access "a" to make it recently used
	_, _ = cache.Get(ctx, "a")

	// Add new item - should evict "b" (least recently used)
	_ = cache.Set(ctx, "d", []float32{4.0})

	// "a" and "c" and "d" should exist
	if _, err := cache.Get(ctx, "a"); err != nil {
		t.Error("'a' should still be in cache")
	}
	if _, err := cache.Get(ctx, "c"); err != nil {
		t.Error("'c' should still be in cache")
	}
	if _, err := cache.Get(ctx, "d"); err != nil {
		t.Error("'d' should be in cache")
	}

	// "b" should be evicted
	if _, err := cache.Get(ctx, "b"); !errors.Is(err, ErrCacheMiss) {
		t.Error("'b' should have been evicted")
	}
}

func TestMemoryCache_Delete(t *testing.T) {
	cache := NewMemoryCache(nil)
	ctx := context.Background()

	_ = cache.Set(ctx, "delete-test", []float32{1.0})

	err := cache.Delete(ctx, "delete-test")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = cache.Get(ctx, "delete-test")
	if !errors.Is(err, ErrCacheMiss) {
		t.Errorf("Get() after Delete() error = %v, want ErrCacheMiss", err)
	}

	// Delete non-existent key should not error
	err = cache.Delete(ctx, "nonexistent")
	if err != nil {
		t.Errorf("Delete() nonexistent error = %v", err)
	}
}

func TestMemoryCache_Clear(t *testing.T) {
	cache := NewMemoryCache(nil)
	ctx := context.Background()

	_ = cache.Set(ctx, "a", []float32{1.0})
	_ = cache.Set(ctx, "b", []float32{2.0})

	err := cache.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	if cache.Size() != 0 {
		t.Errorf("Size() after Clear() = %d, want 0", cache.Size())
	}

	_, err = cache.Get(ctx, "a")
	if !errors.Is(err, ErrCacheMiss) {
		t.Error("Cache should be empty after Clear()")
	}
}

func TestMemoryCache_Size(t *testing.T) {
	cache := NewMemoryCache(nil)
	ctx := context.Background()

	if cache.Size() != 0 {
		t.Errorf("Size() on new cache = %d, want 0", cache.Size())
	}

	_ = cache.Set(ctx, "a", []float32{1.0})
	if cache.Size() != 1 {
		t.Errorf("Size() after 1 Set() = %d, want 1", cache.Size())
	}

	_ = cache.Set(ctx, "b", []float32{2.0})
	if cache.Size() != 2 {
		t.Errorf("Size() after 2 Sets() = %d, want 2", cache.Size())
	}

	_ = cache.Delete(ctx, "a")
	if cache.Size() != 1 {
		t.Errorf("Size() after Delete() = %d, want 1", cache.Size())
	}
}

func TestMemoryCache_Stats(t *testing.T) {
	cache := NewMemoryCache(&MemoryCacheConfig{MaxSize: 100, TTL: 0})
	ctx := context.Background()

	_ = cache.Set(ctx, "test", []float32{1.0})

	// Cache hit
	_, _ = cache.Get(ctx, "test")

	// Cache miss
	_, _ = cache.Get(ctx, "nonexistent")

	stats := cache.Stats()

	if stats.HitCount != 1 {
		t.Errorf("HitCount = %d, want 1", stats.HitCount)
	}
	if stats.MissCount != 1 {
		t.Errorf("MissCount = %d, want 1", stats.MissCount)
	}
	if stats.HitRate != 0.5 {
		t.Errorf("HitRate = %f, want 0.5", stats.HitRate)
	}
	if stats.Size != 1 {
		t.Errorf("Size = %d, want 1", stats.Size)
	}
	if stats.MaxSize != 100 {
		t.Errorf("MaxSize = %d, want 100", stats.MaxSize)
	}
}

func TestMemoryCache_Concurrent(t *testing.T) {
	cache := NewMemoryCache(nil)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + i%26))
			_ = cache.Set(ctx, key, []float32{float32(i)})
		}(i)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + i%26))
			_, _ = cache.Get(ctx, key)
		}(i)
	}
	wg.Wait()
}

func TestDefaultRedisCacheConfig(t *testing.T) {
	cfg := DefaultRedisCacheConfig()

	if cfg.Address == "" {
		t.Error("Address should not be empty")
	}
	if cfg.KeyPrefix == "" {
		t.Error("KeyPrefix should not be empty")
	}
	if cfg.TTL <= 0 {
		t.Error("TTL should be positive")
	}
}

// MockRedisClient implements RedisClient for testing
type MockRedisClient struct {
	data      map[string][]byte
	getErr    error
	setErr    error
	delErr    error
	flushErr  error
	dbSizeErr error
}

func NewMockRedisClient() *MockRedisClient {
	return &MockRedisClient{
		data: make(map[string][]byte),
	}
}

func (m *MockRedisClient) Get(ctx context.Context, key string) ([]byte, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if data, ok := m.data[key]; ok {
		return data, nil
	}
	return nil, errors.New("key not found")
}

func (m *MockRedisClient) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.data[key] = value
	return nil
}

func (m *MockRedisClient) Del(ctx context.Context, keys ...string) error {
	if m.delErr != nil {
		return m.delErr
	}
	for _, key := range keys {
		delete(m.data, key)
	}
	return nil
}

func (m *MockRedisClient) FlushDB(ctx context.Context) error {
	if m.flushErr != nil {
		return m.flushErr
	}
	m.data = make(map[string][]byte)
	return nil
}

func (m *MockRedisClient) DBSize(ctx context.Context) (int64, error) {
	if m.dbSizeErr != nil {
		return 0, m.dbSizeErr
	}
	return int64(len(m.data)), nil
}

func TestNewRedisCache(t *testing.T) {
	t.Run("with nil client returns error", func(t *testing.T) {
		_, err := NewRedisCache(nil, nil)
		if !errors.Is(err, ErrCacheUnavailable) {
			t.Errorf("NewRedisCache(nil, nil) error = %v, want ErrCacheUnavailable", err)
		}
	})

	t.Run("with valid client", func(t *testing.T) {
		client := NewMockRedisClient()
		cache, err := NewRedisCache(nil, client)
		if err != nil {
			t.Fatalf("NewRedisCache() error = %v", err)
		}
		if cache == nil {
			t.Fatal("NewRedisCache() returned nil")
		}
	})
}

func TestRedisCache_GetSet(t *testing.T) {
	client := NewMockRedisClient()
	cache, _ := NewRedisCache(nil, client)
	ctx := context.Background()

	t.Run("miss on empty cache", func(t *testing.T) {
		_, err := cache.Get(ctx, "nonexistent")
		if !errors.Is(err, ErrCacheMiss) {
			t.Errorf("Get() error = %v, want ErrCacheMiss", err)
		}
	})

	t.Run("set and get", func(t *testing.T) {
		embedding := []float32{1.0, 2.0, 3.0}
		err := cache.Set(ctx, "test", embedding)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		result, err := cache.Get(ctx, "test")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}

		if len(result) != len(embedding) {
			t.Errorf("Get() returned %d elements, want %d", len(result), len(embedding))
		}
	})
}

func TestRedisCache_Delete(t *testing.T) {
	client := NewMockRedisClient()
	cache, _ := NewRedisCache(nil, client)
	ctx := context.Background()

	_ = cache.Set(ctx, "delete-test", []float32{1.0})

	err := cache.Delete(ctx, "delete-test")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = cache.Get(ctx, "delete-test")
	if !errors.Is(err, ErrCacheMiss) {
		t.Errorf("Get() after Delete() should miss")
	}
}

func TestRedisCache_Clear(t *testing.T) {
	client := NewMockRedisClient()
	cache, _ := NewRedisCache(nil, client)
	ctx := context.Background()

	_ = cache.Set(ctx, "a", []float32{1.0})
	_ = cache.Set(ctx, "b", []float32{2.0})

	err := cache.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	if cache.Size() != 0 {
		t.Errorf("Size() after Clear() = %d, want 0", cache.Size())
	}
}

func TestRedisCache_Size(t *testing.T) {
	client := NewMockRedisClient()
	cache, _ := NewRedisCache(nil, client)
	ctx := context.Background()

	_ = cache.Set(ctx, "a", []float32{1.0})
	_ = cache.Set(ctx, "b", []float32{2.0})

	size := cache.Size()
	if size != 2 {
		t.Errorf("Size() = %d, want 2", size)
	}
}

func TestRedisCache_SizeError(t *testing.T) {
	client := NewMockRedisClient()
	client.dbSizeErr = errors.New("db size error")
	cache, _ := NewRedisCache(nil, client)

	size := cache.Size()
	if size != 0 {
		t.Errorf("Size() with error = %d, want 0", size)
	}
}

func TestNopCache(t *testing.T) {
	cache := NewNopCache()
	ctx := context.Background()

	t.Run("Get always misses", func(t *testing.T) {
		_, err := cache.Get(ctx, "test")
		if !errors.Is(err, ErrCacheMiss) {
			t.Errorf("Get() error = %v, want ErrCacheMiss", err)
		}
	})

	t.Run("Set does nothing", func(t *testing.T) {
		err := cache.Set(ctx, "test", []float32{1.0})
		if err != nil {
			t.Errorf("Set() error = %v", err)
		}

		// Still misses after Set
		_, err = cache.Get(ctx, "test")
		if !errors.Is(err, ErrCacheMiss) {
			t.Errorf("Get() after Set() error = %v, want ErrCacheMiss", err)
		}
	})

	t.Run("Delete does nothing", func(t *testing.T) {
		err := cache.Delete(ctx, "test")
		if err != nil {
			t.Errorf("Delete() error = %v", err)
		}
	})

	t.Run("Clear does nothing", func(t *testing.T) {
		err := cache.Clear(ctx)
		if err != nil {
			t.Errorf("Clear() error = %v", err)
		}
	})

	t.Run("Size is always 0", func(t *testing.T) {
		if cache.Size() != 0 {
			t.Errorf("Size() = %d, want 0", cache.Size())
		}
	})
}
