package workflows

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gatewayv1 "github.com/otherjamesbrown/penfold/api/proto/gateway/v1"
)

func TestNewHandlers(t *testing.T) {
	logger := testLogger()

	t.Run("with nil logger", func(t *testing.T) {
		handlers := NewHandlers(nil, nil, nil)
		require.NotNil(t, handlers)
	})

	t.Run("with logger", func(t *testing.T) {
		handlers := NewHandlers(nil, nil, logger)
		require.NotNil(t, handlers)
		assert.Nil(t, handlers.starter)
		assert.Nil(t, handlers.statusService)
	})

	t.Run("metrics enabled when starter has metrics", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MetricsEnabled = true

		starter := NewStarter(nil, cfg, logger)
		handlers := NewHandlers(starter, nil, logger)

		assert.NotNil(t, handlers.metrics)
	})

	t.Run("no metrics when starter metrics disabled", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MetricsEnabled = false

		starter := NewStarter(nil, cfg, logger)
		handlers := NewHandlers(starter, nil, logger)

		assert.Nil(t, handlers.metrics)
	})
}

func TestHandlers_RegisterMetrics(t *testing.T) {
	logger := testLogger()

	t.Run("nil metrics returns nil", func(t *testing.T) {
		handlers := NewHandlers(nil, nil, logger)
		err := handlers.RegisterMetrics()
		assert.NoError(t, err)
	})

	t.Run("registers metrics successfully", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MetricsEnabled = true
		cfg.MetricsPrefix = "test_handlers_1"

		starter := NewStarter(nil, cfg, logger)
		handlers := NewHandlers(starter, nil, logger)

		reg := prometheus.NewRegistry()
		err := handlers.RegisterMetricsWithRegistry(reg)
		assert.NoError(t, err)
	})

	t.Run("double registration handled", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MetricsEnabled = true
		cfg.MetricsPrefix = "test_handlers_2"

		starter := NewStarter(nil, cfg, logger)
		handlers := NewHandlers(starter, nil, logger)

		reg := prometheus.NewRegistry()
		err := handlers.RegisterMetricsWithRegistry(reg)
		assert.NoError(t, err)

		err = handlers.RegisterMetricsWithRegistry(reg)
		assert.NoError(t, err)
	})
}

func TestHandlers_ProcessEmail(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	starter := NewStarter(nil, cfg, logger)
	statusSvc := NewStatusService(nil, cfg, logger)
	handlers := NewHandlers(starter, statusSvc, logger)

	t.Run("invalid request returns InvalidArgument", func(t *testing.T) {
		req := &gatewayv1.ProcessEmailRequest{
			// Missing required fields
		}

		resp, err := handlers.ProcessEmail(context.Background(), req)
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})
}

func TestHandlers_StartIngestionWorkflow(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	starter := NewStarter(nil, cfg, logger)
	statusSvc := NewStatusService(nil, cfg, logger)
	handlers := NewHandlers(starter, statusSvc, logger)

	t.Run("nil request returns InvalidArgument", func(t *testing.T) {
		result, err := handlers.StartIngestionWorkflow(context.Background(), nil)
		assert.Nil(t, result)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("validation error returns InvalidArgument", func(t *testing.T) {
		req := &IngestRequest{
			// Missing required fields
		}

		result, err := handlers.StartIngestionWorkflow(context.Background(), req)
		assert.Nil(t, result)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})
}

func TestHandlers_StartSyncWorkflow(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	starter := NewStarter(nil, cfg, logger)
	statusSvc := NewStatusService(nil, cfg, logger)
	handlers := NewHandlers(starter, statusSvc, logger)

	t.Run("nil request returns InvalidArgument", func(t *testing.T) {
		result, err := handlers.StartSyncWorkflow(context.Background(), nil)
		assert.Nil(t, result)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("validation error returns InvalidArgument", func(t *testing.T) {
		req := &SyncRequest{
			// Missing required fields
		}

		result, err := handlers.StartSyncWorkflow(context.Background(), req)
		assert.Nil(t, result)
		require.Error(t, err)
	})
}

func TestHandlers_StartReviewWorkflow(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	starter := NewStarter(nil, cfg, logger)
	statusSvc := NewStatusService(nil, cfg, logger)
	handlers := NewHandlers(starter, statusSvc, logger)

	t.Run("nil request returns InvalidArgument", func(t *testing.T) {
		result, err := handlers.StartReviewWorkflow(context.Background(), nil)
		assert.Nil(t, result)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("validation error returns InvalidArgument", func(t *testing.T) {
		req := &ReviewRequest{
			// Missing required fields
		}

		result, err := handlers.StartReviewWorkflow(context.Background(), req)
		assert.Nil(t, result)
		require.Error(t, err)
	})
}

func TestHandlers_StartAnalysisWorkflow(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	starter := NewStarter(nil, cfg, logger)
	statusSvc := NewStatusService(nil, cfg, logger)
	handlers := NewHandlers(starter, statusSvc, logger)

	t.Run("nil request returns InvalidArgument", func(t *testing.T) {
		result, err := handlers.StartAnalysisWorkflow(context.Background(), nil)
		assert.Nil(t, result)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("validation error returns InvalidArgument", func(t *testing.T) {
		req := &AnalysisRequest{
			// Missing required fields
		}

		result, err := handlers.StartAnalysisWorkflow(context.Background(), req)
		assert.Nil(t, result)
		require.Error(t, err)
	})
}

func TestHandlers_GetWorkflowStatus(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	starter := NewStarter(nil, cfg, logger)
	statusSvc := NewStatusService(nil, cfg, logger)
	handlers := NewHandlers(starter, statusSvc, logger)

	t.Run("empty workflow_id returns InvalidArgument", func(t *testing.T) {
		result, err := handlers.GetWorkflowStatus(context.Background(), "")
		assert.Nil(t, result)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})
}

func TestHandlers_CancelWorkflow(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	starter := NewStarter(nil, cfg, logger)
	statusSvc := NewStatusService(nil, cfg, logger)
	handlers := NewHandlers(starter, statusSvc, logger)

	t.Run("empty workflow_id returns InvalidArgument", func(t *testing.T) {
		err := handlers.CancelWorkflow(context.Background(), "")
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})
}

func TestHandlers_Accessors(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	starter := NewStarter(nil, cfg, logger)
	statusSvc := NewStatusService(nil, cfg, logger)
	handlers := NewHandlers(starter, statusSvc, logger)

	t.Run("Starter returns starter", func(t *testing.T) {
		assert.Same(t, starter, handlers.Starter())
	})

	t.Run("StatusService returns statusService", func(t *testing.T) {
		assert.Same(t, statusSvc, handlers.StatusService())
	})
}

func TestMapErrorCodeToGRPC(t *testing.T) {
	tests := []struct {
		code     string
		expected codes.Code
	}{
		{ErrCodeInvalidInput, codes.InvalidArgument},
		{ErrCodeNotFound, codes.NotFound},
		{ErrCodeAlreadyExists, codes.AlreadyExists},
		{ErrCodeTimeout, codes.DeadlineExceeded},
		{ErrCodeUnavailable, codes.Unavailable},
		{ErrCodePermissionDenied, codes.PermissionDenied},
		{ErrCodeRateLimited, codes.ResourceExhausted},
		{ErrCodeInternal, codes.Internal},
		{"UNKNOWN_CODE", codes.Internal},
		{"", codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			result := mapErrorCodeToGRPC(tt.code)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHandlerMetrics_Creation(t *testing.T) {
	metrics := newHandlerMetrics("test_prefix")

	assert.NotNil(t, metrics)
	assert.NotNil(t, metrics.requestsTotal)
	assert.NotNil(t, metrics.requestLatency)
	assert.NotNil(t, metrics.requestErrors)
}

func TestHandlers_MetricsRecording(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	cfg.MetricsEnabled = true
	cfg.MetricsPrefix = "test_handler_metrics"

	starter := NewStarter(nil, cfg, logger)
	handlers := NewHandlers(starter, nil, logger)

	// Test that methods don't panic when called
	t.Run("recordSuccess", func(t *testing.T) {
		handlers.recordSuccess("TestMethod", time.Now())
	})

	t.Run("recordError", func(t *testing.T) {
		handlers.recordError("TestMethod", "TEST_ERROR")
	})
}

func TestHandlers_MetricsRecording_NilMetrics(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	cfg.MetricsEnabled = false

	starter := NewStarter(nil, cfg, logger)
	handlers := NewHandlers(starter, nil, logger)

	// These should not panic even when metrics are nil
	handlers.recordSuccess("TestMethod", time.Now())
	handlers.recordError("TestMethod", "TEST_ERROR")
}

func TestContains(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello world", "world", true},
		{"hello world", "World", true}, // case-insensitive
		{"hello world", "WORLD", true},
		{"hello world", "foo", false},
		{"hello", "", true},
		{"", "foo", false},
		{"foo", "foobar", false},
		{"TIMEOUT", "timeout", true},
		{"timeout", "TIMEOUT", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"Hello World", "world", true},
		{"HELLO WORLD", "world", true},
		{"hello world", "WORLD", true},
		{"hello", "ell", true},
		{"hello", "foo", false},
		{"", "", true},
		{"a", "a", true},
		{"A", "a", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			result := containsIgnoreCase(tt.s, tt.substr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEqualFold(t *testing.T) {
	tests := []struct {
		s        string
		t        string
		expected bool
	}{
		{"hello", "hello", true},
		{"HELLO", "hello", true},
		{"Hello", "hELLO", true},
		{"hello", "world", false},
		{"hello", "hello!", false},
		{"", "", true},
		{"a", "A", true},
	}

	for _, tt := range tests {
		name := tt.s + "_" + tt.t
		if name == "_" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			result := equalFold(tt.s, tt.t)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLower(t *testing.T) {
	tests := []struct {
		input    byte
		expected byte
	}{
		{'A', 'a'},
		{'Z', 'z'},
		{'a', 'a'},
		{'z', 'z'},
		{'0', '0'},
		{'@', '@'},
		{'[', '['},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := lower(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
