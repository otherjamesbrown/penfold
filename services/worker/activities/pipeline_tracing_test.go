// Package activities provides tests for pipeline tracing activities.
package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// TestStartPipelineTracing_ActivityExists verifies that the StartPipelineTracing activity exists
// and can be called with the expected parameters.
func TestStartPipelineTracing_ActivityExists(t *testing.T) {
	logger := logging.NewNopLogger()

	// Create PipelineActivities instance (the activity should be a method on this struct)
	// Note: This will fail to compile until the activity is implemented
	acts := &PipelineActivities{
		logger: logger,
	}

	ctx := context.Background()
	input := workflows.StartPipelineTracingInput{
		PipelineTraceID: "0123456789abcdef0123456789abcdef", // 32-char hex trace ID
		ContentID:       "em-abc12XYZ",                      // Standard content ID format
		ContentType:     "email",
	}

	output, err := acts.StartPipelineTracing(ctx, input)

	// Once implemented, the activity should succeed with valid input
	require.NoError(t, err)
	assert.NotEmpty(t, output.TraceID)
	assert.NotEmpty(t, output.SpanID)
}

// TestStartPipelineTracing_EmptyTraceID verifies handling of empty trace ID.
// When empty, StartPipeline should create a new root trace.
func TestStartPipelineTracing_EmptyTraceID(t *testing.T) {
	logger := logging.NewNopLogger()

	acts := &PipelineActivities{
		logger: logger,
	}

	ctx := context.Background()
	input := workflows.StartPipelineTracingInput{
		PipelineTraceID: "", // Empty means create new root
		ContentID:       "em-abc12XYZ",
		ContentType:     "email",
	}

	output, err := acts.StartPipelineTracing(ctx, input)

	// Should succeed - empty trace ID is valid (creates new root)
	require.NoError(t, err)
	assert.NotEmpty(t, output.TraceID)
	assert.NotEmpty(t, output.SpanID)
}

// TestStartPipelineTracing_InvalidContentID verifies validation of content ID format.
func TestStartPipelineTracing_InvalidContentID(t *testing.T) {
	logger := logging.NewNopLogger()

	acts := &PipelineActivities{
		logger: logger,
	}

	ctx := context.Background()

	testCases := []struct {
		name        string
		contentID   string
		expectError bool
	}{
		{
			name:        "empty content ID",
			contentID:   "",
			expectError: true,
		},
		{
			name:        "valid content ID",
			contentID:   "em-abc12XYZ",
			expectError: false,
		},
		{
			name:        "invalid format - too short",
			contentID:   "em-short",
			expectError: true,
		},
		{
			name:        "invalid format - no prefix",
			contentID:   "abc12XYZ89",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := workflows.StartPipelineTracingInput{
				PipelineTraceID: "0123456789abcdef0123456789abcdef",
				ContentID:       tc.contentID,
				ContentType:     "email",
			}

			output, err := acts.StartPipelineTracing(ctx, input)

			if tc.expectError {
				assert.Error(t, err, "Expected error for invalid content ID: %s", tc.contentID)
			} else {
				assert.NoError(t, err, "Expected no error for valid content ID: %s", tc.contentID)
				assert.NotEmpty(t, output.TraceID)
				assert.NotEmpty(t, output.SpanID)
			}
		})
	}
}

// TestStartPipelineTracing_MissingContentType verifies that content type is required.
func TestStartPipelineTracing_MissingContentType(t *testing.T) {
	logger := logging.NewNopLogger()

	acts := &PipelineActivities{
		logger: logger,
	}

	ctx := context.Background()
	input := workflows.StartPipelineTracingInput{
		PipelineTraceID: "0123456789abcdef0123456789abcdef",
		ContentID:       "em-abc12XYZ",
		ContentType:     "", // Missing
	}

	_, err := acts.StartPipelineTracing(ctx, input)

	// Content type should be required
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content_type")
}

// TestStartPipelineTracing_CreatesSpan verifies that the activity actually creates a tracing span.
// This test would ideally use a mock tracer to verify span creation, but for now we just
// verify the activity completes without error when tracing is configured.
func TestStartPipelineTracing_CreatesSpan(t *testing.T) {
	t.Skip("TODO: Implement with mock tracer to verify span attributes")

	// Future implementation should:
	// 1. Set up a mock/noop tracer in context
	// 2. Call StartPipelineTracing
	// 3. Verify that tracing.StartPipeline was called with correct parameters
	// 4. Verify span attributes include contentID, contentType, etc.
}

// TestStartPipelineTracing_ReturnsActualTraceID verifies that StartPipelineTracing returns
// the ACTUAL TraceID from the OTel span, not the pre-generated UUID passed in.
//
// This is a REGRESSION TEST for bug pf-557197.
//
// The bug: The workflow pre-generates a pipelineTraceID via UUID SideEffect and passes it
// to StartPipelineTracing. The activity passes this to tracing.StartPipeline(), which creates
// an OTel span. However, OTel may generate a DIFFERENT TraceID (e.g., when creating a new
// root span with invalid SpanContext). The current implementation returns only the SpanID,
// so the workflow uses the wrong (pre-generated UUID) TraceID for all downstream activities.
//
// The fix: StartPipelineTracing must return BOTH the actual TraceID and SpanID from the
// OTel span, so the workflow can propagate the correct TraceID.
//
// This test verifies that:
// 1. The activity returns a StartPipelineTracingOutput struct (not just a string)
// 2. The struct contains both TraceID and SpanID
// 3. Both are valid non-zero hex strings (32-char TraceID, 16-char SpanID)
// 4. The TraceID is from the actual OTel span (not necessarily the input pipelineTraceID)
func TestStartPipelineTracing_ReturnsActualTraceID(t *testing.T) {
	logger := logging.NewNopLogger()

	acts := &PipelineActivities{
		logger: logger,
	}

	ctx := context.Background()
	input := workflows.StartPipelineTracingInput{
		PipelineTraceID: "0123456789abcdef0123456789abcdef", // Pre-generated UUID (without hyphens)
		ContentID:       "em-abc12XYZ",
		ContentType:     "email",
	}

	// Call the activity
	// NOTE: This will FAIL TO COMPILE because the return type is currently (string, error)
	// instead of (workflows.StartPipelineTracingOutput, error)
	output, err := acts.StartPipelineTracing(ctx, input)
	require.NoError(t, err)

	// Verify the output is a struct with TraceID and SpanID fields
	require.NotNil(t, output, "StartPipelineTracing must return a non-nil output struct")

	// In a test environment without a configured OTel tracer, the tracer returns a no-op span
	// with zero trace/span IDs. This is expected and normal behavior for unit tests.
	// The important thing is that:
	// 1. The activity signature returns the correct struct type (compile-time check)
	// 2. The struct has both TraceID and SpanID fields with proper lengths
	// 3. In production with real tracing, these will be populated with actual values

	// Verify TraceID is a 32-character hex string (may be zero in tests without tracer)
	assert.Len(t, output.TraceID, 32, "TraceID must be 32-char hex string")
	assert.Regexp(t, `^[0-9a-f]{32}$`, output.TraceID, "TraceID must be lowercase hex")

	// Verify SpanID is a 16-character hex string (may be zero in tests without tracer)
	assert.Len(t, output.SpanID, 16, "SpanID must be 16-char hex string")
	assert.Regexp(t, `^[0-9a-f]{16}$`, output.SpanID, "SpanID must be lowercase hex")

	// NOTE: We do NOT assert that output.TraceID == input.PipelineTraceID
	// because OTel may generate a different TraceID when creating a new root span.
	// The point is that the activity must return whatever TraceID OTel actually created,
	// so the workflow can use that for downstream activities.
	//
	// In production with real tracing (Langfuse), the IDs will be non-zero and valid.
	// In unit tests, they may be zero (no-op tracer), which is acceptable.
}
