package temporal

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestHeartbeatLoopStopsOnCancel verifies that HeartbeatLoop stops when cancel is called.
func TestHeartbeatLoopStopsOnCancel(t *testing.T) {
	ctx := context.Background()
	var callCount atomic.Int32

	cancel := HeartbeatLoop(ctx, 10*time.Millisecond, func() interface{} {
		callCount.Add(1)
		return "test"
	})

	// Let it run a bit
	time.Sleep(50 * time.Millisecond)

	// Cancel the loop
	cancel()

	// Record the count
	countAtCancel := callCount.Load()

	// Wait a bit more
	time.Sleep(50 * time.Millisecond)

	// Count should not have increased significantly after cancel
	countAfterCancel := callCount.Load()

	if countAfterCancel > countAtCancel+1 {
		t.Errorf("HeartbeatLoop continued after cancel: count went from %d to %d", countAtCancel, countAfterCancel)
	}
}

// TestHeartbeatLoopStopsOnContextCancel verifies that HeartbeatLoop stops when context is cancelled.
func TestHeartbeatLoopStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var callCount atomic.Int32

	stopLoop := HeartbeatLoop(ctx, 10*time.Millisecond, func() interface{} {
		callCount.Add(1)
		return "test"
	})
	defer stopLoop()

	// Let it run a bit
	time.Sleep(50 * time.Millisecond)

	// Cancel the context
	cancel()

	// Record the count
	countAtCancel := callCount.Load()

	// Wait a bit more
	time.Sleep(50 * time.Millisecond)

	// Count should not have increased significantly after context cancel
	countAfterCancel := callCount.Load()

	if countAfterCancel > countAtCancel+1 {
		t.Errorf("HeartbeatLoop continued after context cancel: count went from %d to %d", countAtCancel, countAfterCancel)
	}
}

// TestHeartbeatConstants verifies the heartbeat interval constants are set correctly.
func TestHeartbeatConstants(t *testing.T) {
	if EmbeddingHeartbeatInterval != 10*time.Second {
		t.Errorf("EmbeddingHeartbeatInterval = %v, want 10s", EmbeddingHeartbeatInterval)
	}

	if LLMHeartbeatInterval != 15*time.Second {
		t.Errorf("LLMHeartbeatInterval = %v, want 15s", LLMHeartbeatInterval)
	}
}

// Note: WithHeartbeat, WithEmbeddingHeartbeat, and WithLLMHeartbeat cannot be fully
// tested without mocking the Temporal activity package, which would require
// additional test infrastructure. The heartbeat recording is a no-op outside
// of a Temporal activity context.
