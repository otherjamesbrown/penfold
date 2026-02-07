package temporal

import (
	"testing"
	"time"

	"go.temporal.io/sdk/workflow"
)

func TestFastActivityOptions(t *testing.T) {
	opts := FastActivityOptions()

	if opts.StartToCloseTimeout != 30*time.Second {
		t.Errorf("expected StartToCloseTimeout 30s, got %v", opts.StartToCloseTimeout)
	}
	if opts.HeartbeatTimeout != 0 {
		t.Errorf("expected no HeartbeatTimeout, got %v", opts.HeartbeatTimeout)
	}
	if opts.RetryPolicy == nil {
		t.Fatal("expected RetryPolicy to be set")
	}
	if opts.RetryPolicy.MaximumAttempts != 3 {
		t.Errorf("expected MaximumAttempts 3, got %d", opts.RetryPolicy.MaximumAttempts)
	}
	if opts.RetryPolicy.InitialInterval != time.Second {
		t.Errorf("expected InitialInterval 1s, got %v", opts.RetryPolicy.InitialInterval)
	}
	if opts.RetryPolicy.BackoffCoefficient != 2.0 {
		t.Errorf("expected BackoffCoefficient 2.0, got %v", opts.RetryPolicy.BackoffCoefficient)
	}
}

func TestEmbeddingActivityOptions(t *testing.T) {
	opts := EmbeddingActivityOptions()

	if opts.StartToCloseTimeout != 30*time.Second {
		t.Errorf("expected StartToCloseTimeout 30s, got %v", opts.StartToCloseTimeout)
	}
	if opts.HeartbeatTimeout != 10*time.Second {
		t.Errorf("expected HeartbeatTimeout 10s, got %v", opts.HeartbeatTimeout)
	}
	if opts.RetryPolicy == nil {
		t.Fatal("expected RetryPolicy to be set")
	}
	if opts.RetryPolicy.MaximumAttempts != 3 {
		t.Errorf("expected MaximumAttempts 3, got %d", opts.RetryPolicy.MaximumAttempts)
	}
	if opts.RetryPolicy.InitialInterval != 2*time.Second {
		t.Errorf("expected InitialInterval 2s, got %v", opts.RetryPolicy.InitialInterval)
	}
}

func TestLLMActivityOptions(t *testing.T) {
	opts := LLMActivityOptions()

	if opts.StartToCloseTimeout != 10*time.Minute {
		t.Errorf("expected StartToCloseTimeout 10m, got %v", opts.StartToCloseTimeout)
	}
	if opts.ScheduleToCloseTimeout != 15*time.Minute {
		t.Errorf("expected ScheduleToCloseTimeout 15m, got %v", opts.ScheduleToCloseTimeout)
	}
	if opts.HeartbeatTimeout != 5*time.Minute {
		t.Errorf("expected HeartbeatTimeout 5m, got %v", opts.HeartbeatTimeout)
	}
	if opts.RetryPolicy == nil {
		t.Fatal("expected RetryPolicy to be set")
	}
	if opts.RetryPolicy.MaximumAttempts != 2 {
		t.Errorf("expected MaximumAttempts 2, got %d", opts.RetryPolicy.MaximumAttempts)
	}
	if opts.RetryPolicy.InitialInterval != 5*time.Second {
		t.Errorf("expected InitialInterval 5s, got %v", opts.RetryPolicy.InitialInterval)
	}
}

func TestBatchActivityOptions(t *testing.T) {
	opts := BatchActivityOptions()

	if opts.StartToCloseTimeout != 5*time.Minute {
		t.Errorf("expected StartToCloseTimeout 5m, got %v", opts.StartToCloseTimeout)
	}
	if opts.HeartbeatTimeout != 30*time.Second {
		t.Errorf("expected HeartbeatTimeout 30s, got %v", opts.HeartbeatTimeout)
	}
	if opts.RetryPolicy == nil {
		t.Fatal("expected RetryPolicy to be set")
	}
	if opts.RetryPolicy.MaximumAttempts != 2 {
		t.Errorf("expected MaximumAttempts 2, got %d", opts.RetryPolicy.MaximumAttempts)
	}
	if opts.RetryPolicy.InitialInterval != 10*time.Second {
		t.Errorf("expected InitialInterval 10s, got %v", opts.RetryPolicy.InitialInterval)
	}
}

func TestLongRunningActivityOptions(t *testing.T) {
	opts := LongRunningActivityOptions()

	if opts.StartToCloseTimeout != 30*time.Minute {
		t.Errorf("expected StartToCloseTimeout 30m, got %v", opts.StartToCloseTimeout)
	}
	if opts.HeartbeatTimeout != 1*time.Minute {
		t.Errorf("expected HeartbeatTimeout 1m, got %v", opts.HeartbeatTimeout)
	}
	if opts.RetryPolicy == nil {
		t.Fatal("expected RetryPolicy to be set")
	}
	if opts.RetryPolicy.MaximumAttempts != 1 {
		t.Errorf("expected MaximumAttempts 1, got %d", opts.RetryPolicy.MaximumAttempts)
	}
	if opts.RetryPolicy.InitialInterval != 30*time.Second {
		t.Errorf("expected InitialInterval 30s, got %v", opts.RetryPolicy.InitialInterval)
	}
}

func TestCustomActivityOptions(t *testing.T) {
	t.Run("with heartbeat", func(t *testing.T) {
		opts := CustomActivityOptions(1*time.Minute, 10*time.Second, 5, 2*time.Second)

		if opts.StartToCloseTimeout != 1*time.Minute {
			t.Errorf("expected StartToCloseTimeout 1m, got %v", opts.StartToCloseTimeout)
		}
		if opts.HeartbeatTimeout != 10*time.Second {
			t.Errorf("expected HeartbeatTimeout 10s, got %v", opts.HeartbeatTimeout)
		}
		if opts.RetryPolicy == nil {
			t.Fatal("expected RetryPolicy to be set")
		}
		if opts.RetryPolicy.MaximumAttempts != 5 {
			t.Errorf("expected MaximumAttempts 5, got %d", opts.RetryPolicy.MaximumAttempts)
		}
		if opts.RetryPolicy.InitialInterval != 2*time.Second {
			t.Errorf("expected InitialInterval 2s, got %v", opts.RetryPolicy.InitialInterval)
		}
	})

	t.Run("without heartbeat", func(t *testing.T) {
		opts := CustomActivityOptions(1*time.Minute, 0, 3, 1*time.Second)

		if opts.HeartbeatTimeout != 0 {
			t.Errorf("expected no HeartbeatTimeout, got %v", opts.HeartbeatTimeout)
		}
	})
}

func TestDefaultWorkflowOptions(t *testing.T) {
	opts := DefaultWorkflowOptions("test-workflow-id", "test-queue")

	if opts.WorkflowID != "test-workflow-id" {
		t.Errorf("expected WorkflowID test-workflow-id, got %s", opts.WorkflowID)
	}
	if opts.TaskQueue != "test-queue" {
		t.Errorf("expected TaskQueue test-queue, got %s", opts.TaskQueue)
	}
	if opts.WorkflowExecutionTimeout != 24*time.Hour {
		t.Errorf("expected WorkflowExecutionTimeout 24h, got %v", opts.WorkflowExecutionTimeout)
	}
	if opts.WorkflowRunTimeout != 10*time.Minute {
		t.Errorf("expected WorkflowRunTimeout 10m, got %v", opts.WorkflowRunTimeout)
	}
}

func TestLongRunningWorkflowOptions(t *testing.T) {
	opts := LongRunningWorkflowOptions("long-workflow-id", "long-queue")

	if opts.WorkflowID != "long-workflow-id" {
		t.Errorf("expected WorkflowID long-workflow-id, got %s", opts.WorkflowID)
	}
	if opts.TaskQueue != "long-queue" {
		t.Errorf("expected TaskQueue long-queue, got %s", opts.TaskQueue)
	}
	if opts.WorkflowExecutionTimeout != 7*24*time.Hour {
		t.Errorf("expected WorkflowExecutionTimeout 7d, got %v", opts.WorkflowExecutionTimeout)
	}
	if opts.WorkflowRunTimeout != 1*time.Hour {
		t.Errorf("expected WorkflowRunTimeout 1h, got %v", opts.WorkflowRunTimeout)
	}
}

func TestNonRetryableErrors(t *testing.T) {
	errors := NonRetryableErrors()

	expected := []string{
		"ValidationError",
		"NotFoundError",
		"PermissionDeniedError",
		"InvalidArgumentError",
	}

	if len(errors) != len(expected) {
		t.Fatalf("expected %d errors, got %d", len(expected), len(errors))
	}

	for i, e := range expected {
		if errors[i] != e {
			t.Errorf("expected error[%d] to be %s, got %s", i, e, errors[i])
		}
	}
}

func TestWithNonRetryableErrors(t *testing.T) {
	t.Run("adds errors to existing policy", func(t *testing.T) {
		opts := FastActivityOptions()
		opts = WithNonRetryableErrors(opts, "CustomError", "AnotherError")

		if opts.RetryPolicy == nil {
			t.Fatal("expected RetryPolicy to be set")
		}
		if len(opts.RetryPolicy.NonRetryableErrorTypes) != 2 {
			t.Errorf("expected 2 non-retryable errors, got %d", len(opts.RetryPolicy.NonRetryableErrorTypes))
		}
		if opts.RetryPolicy.NonRetryableErrorTypes[0] != "CustomError" {
			t.Errorf("expected first error to be CustomError, got %s", opts.RetryPolicy.NonRetryableErrorTypes[0])
		}
		if opts.RetryPolicy.NonRetryableErrorTypes[1] != "AnotherError" {
			t.Errorf("expected second error to be AnotherError, got %s", opts.RetryPolicy.NonRetryableErrorTypes[1])
		}
	})

	t.Run("creates policy if nil", func(t *testing.T) {
		opts := WithNonRetryableErrors(
			workflow.ActivityOptions{}, // Empty options with no retry policy
			"TestError",
		)

		if opts.RetryPolicy == nil {
			t.Fatal("expected RetryPolicy to be set")
		}
		if len(opts.RetryPolicy.NonRetryableErrorTypes) != 1 {
			t.Errorf("expected 1 non-retryable error, got %d", len(opts.RetryPolicy.NonRetryableErrorTypes))
		}
	})
}
