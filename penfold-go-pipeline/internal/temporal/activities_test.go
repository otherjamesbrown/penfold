package temporal

import (
	"testing"
	"time"
)

// TestFastActivityOptions verifies FastActivityOptions returns correct configuration.
func TestFastActivityOptions(t *testing.T) {
	opts := FastActivityOptions()

	if opts.StartToCloseTimeout != 30*time.Second {
		t.Errorf("StartToCloseTimeout = %v, want 30s", opts.StartToCloseTimeout)
	}

	if opts.HeartbeatTimeout != 0 {
		t.Errorf("HeartbeatTimeout = %v, want 0 (no heartbeat)", opts.HeartbeatTimeout)
	}

	if opts.RetryPolicy == nil {
		t.Fatal("RetryPolicy is nil")
	}

	if opts.RetryPolicy.MaximumAttempts != 3 {
		t.Errorf("MaximumAttempts = %d, want 3", opts.RetryPolicy.MaximumAttempts)
	}

	if opts.RetryPolicy.InitialInterval != time.Second {
		t.Errorf("InitialInterval = %v, want 1s", opts.RetryPolicy.InitialInterval)
	}

	if opts.RetryPolicy.BackoffCoefficient != 2.0 {
		t.Errorf("BackoffCoefficient = %v, want 2.0", opts.RetryPolicy.BackoffCoefficient)
	}
}

// TestEmbeddingActivityOptions verifies EmbeddingActivityOptions returns correct configuration.
func TestEmbeddingActivityOptions(t *testing.T) {
	opts := EmbeddingActivityOptions()

	if opts.StartToCloseTimeout != 30*time.Second {
		t.Errorf("StartToCloseTimeout = %v, want 30s", opts.StartToCloseTimeout)
	}

	if opts.HeartbeatTimeout != 10*time.Second {
		t.Errorf("HeartbeatTimeout = %v, want 10s", opts.HeartbeatTimeout)
	}

	if opts.RetryPolicy == nil {
		t.Fatal("RetryPolicy is nil")
	}

	if opts.RetryPolicy.MaximumAttempts != 3 {
		t.Errorf("MaximumAttempts = %d, want 3", opts.RetryPolicy.MaximumAttempts)
	}

	if opts.RetryPolicy.InitialInterval != 2*time.Second {
		t.Errorf("InitialInterval = %v, want 2s", opts.RetryPolicy.InitialInterval)
	}
}

// TestLLMActivityOptions verifies LLMActivityOptions returns correct configuration.
func TestLLMActivityOptions(t *testing.T) {
	opts := LLMActivityOptions()

	if opts.StartToCloseTimeout != 2*time.Minute {
		t.Errorf("StartToCloseTimeout = %v, want 2m", opts.StartToCloseTimeout)
	}

	if opts.ScheduleToCloseTimeout != 5*time.Minute {
		t.Errorf("ScheduleToCloseTimeout = %v, want 5m", opts.ScheduleToCloseTimeout)
	}

	if opts.HeartbeatTimeout != 15*time.Second {
		t.Errorf("HeartbeatTimeout = %v, want 15s", opts.HeartbeatTimeout)
	}

	if opts.RetryPolicy == nil {
		t.Fatal("RetryPolicy is nil")
	}

	if opts.RetryPolicy.MaximumAttempts != 2 {
		t.Errorf("MaximumAttempts = %d, want 2 (fewer for expensive ops)", opts.RetryPolicy.MaximumAttempts)
	}

	if opts.RetryPolicy.InitialInterval != 5*time.Second {
		t.Errorf("InitialInterval = %v, want 5s", opts.RetryPolicy.InitialInterval)
	}
}

// TestBatchActivityOptions verifies BatchActivityOptions returns correct configuration.
func TestBatchActivityOptions(t *testing.T) {
	opts := BatchActivityOptions()

	if opts.StartToCloseTimeout != 5*time.Minute {
		t.Errorf("StartToCloseTimeout = %v, want 5m", opts.StartToCloseTimeout)
	}

	if opts.HeartbeatTimeout != 30*time.Second {
		t.Errorf("HeartbeatTimeout = %v, want 30s", opts.HeartbeatTimeout)
	}

	if opts.RetryPolicy == nil {
		t.Fatal("RetryPolicy is nil")
	}

	if opts.RetryPolicy.MaximumAttempts != 2 {
		t.Errorf("MaximumAttempts = %d, want 2", opts.RetryPolicy.MaximumAttempts)
	}
}
