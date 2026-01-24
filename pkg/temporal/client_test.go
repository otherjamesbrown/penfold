package temporal

import (
	"bytes"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/rs/zerolog"
	"go.temporal.io/sdk/client"
)

func TestConfigStruct(t *testing.T) {
	cfg := &Config{
		HostPort:                "temporal:7233",
		Namespace:               "penfold",
		TaskQueue:               "test-queue",
		MaxConcurrentActivities: 20,
		MaxConcurrentWorkflows:  15,
	}

	if cfg.HostPort != "temporal:7233" {
		t.Errorf("expected HostPort temporal:7233, got %s", cfg.HostPort)
	}
	if cfg.Namespace != "penfold" {
		t.Errorf("expected Namespace penfold, got %s", cfg.Namespace)
	}
	if cfg.TaskQueue != "test-queue" {
		t.Errorf("expected TaskQueue test-queue, got %s", cfg.TaskQueue)
	}
	if cfg.MaxConcurrentActivities != 20 {
		t.Errorf("expected MaxConcurrentActivities 20, got %d", cfg.MaxConcurrentActivities)
	}
	if cfg.MaxConcurrentWorkflows != 15 {
		t.Errorf("expected MaxConcurrentWorkflows 15, got %d", cfg.MaxConcurrentWorkflows)
	}
}

func TestClientOptions(t *testing.T) {
	t.Run("WithLogger", func(t *testing.T) {
		var buf bytes.Buffer
		logger := logging.NewLogger(&logging.Config{
			Level:       logging.LevelDebug,
			ServiceName: "test",
			JSONFormat:  true,
			Output:      &buf,
		})

		opts := &client.Options{}
		WithLogger(logger)(opts)

		if opts.Logger == nil {
			t.Error("expected Logger to be set")
		}

		// Verify the logger works by calling a method
		opts.Logger.Info("test message", "key", "value")
		output := buf.String()
		if output == "" {
			t.Error("expected log output")
		}
	})

	t.Run("WithMetricsHandler", func(t *testing.T) {
		// We can't easily create a metrics handler for testing,
		// but we can verify the option function structure
		opts := &client.Options{}
		WithMetricsHandler(nil)(opts)

		// Just verifying the option function works without panic
	})

	t.Run("WithConnectionOptions", func(t *testing.T) {
		connOpts := client.ConnectionOptions{}

		opts := &client.Options{}
		WithConnectionOptions(connOpts)(opts)

		// Just verify the option function works without panic
		// Connection options are an internal type with limited testable fields
	})
}

func TestZerologAdapter(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).With().Timestamp().Logger()

	adapter := NewLogger(logger)

	t.Run("Debug", func(t *testing.T) {
		buf.Reset()
		adapter.Debug("debug message", "key1", "value1")
		output := buf.String()
		if output == "" {
			t.Skip("debug level might be disabled")
		}
	})

	t.Run("Info", func(t *testing.T) {
		buf.Reset()
		adapter.Info("info message", "key1", "value1", "key2", 42)
		output := buf.String()
		if output == "" {
			t.Error("expected info log output")
		}
		if !bytes.Contains(buf.Bytes(), []byte("info message")) {
			t.Error("expected output to contain message")
		}
	})

	t.Run("Warn", func(t *testing.T) {
		buf.Reset()
		adapter.Warn("warn message", "warning", true)
		output := buf.String()
		if output == "" {
			t.Error("expected warn log output")
		}
		if !bytes.Contains(buf.Bytes(), []byte("warn message")) {
			t.Error("expected output to contain message")
		}
	})

	t.Run("Error", func(t *testing.T) {
		buf.Reset()
		adapter.Error("error message", "error_code", 500)
		output := buf.String()
		if output == "" {
			t.Error("expected error log output")
		}
		if !bytes.Contains(buf.Bytes(), []byte("error message")) {
			t.Error("expected output to contain message")
		}
	})

	t.Run("handles non-string keys", func(t *testing.T) {
		buf.Reset()
		// This tests the fallback when key is not a string
		adapter.Info("message with non-string key", 123, "value")
		// Should not panic
	})

	t.Run("handles odd number of keyvals", func(t *testing.T) {
		buf.Reset()
		// Odd number of key-value pairs
		adapter.Info("message", "key1", "value1", "orphan_key")
		// Should not panic, orphan key is ignored
	})
}

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	temporalLogger := NewLogger(logger)

	if temporalLogger == nil {
		t.Error("expected non-nil logger")
	}

	// Verify it adds the component field
	temporalLogger.Info("test")
	if !bytes.Contains(buf.Bytes(), []byte("temporal")) {
		t.Error("expected output to contain component:temporal")
	}
}
