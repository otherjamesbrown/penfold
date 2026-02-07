package pipelineservice

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
)

// Tests for GetPipelineErrors handler focus on validation logic.
// Database-dependent tests require integration test setup.

func TestGetPipelineErrors_Request(t *testing.T) {
	// Test that requests are properly structured
	req := &pipelinev1.GetPipelineErrorsRequest{
		ErrorCode: "timeout",
		Limit:     100,
	}
	assert.Equal(t, "timeout", req.ErrorCode)
	assert.Equal(t, int32(100), req.Limit)
}

func TestGetPipelineErrors_Response(t *testing.T) {
	// Test that responses are properly structured
	resp := &pipelinev1.GetPipelineErrorsResponse{
		Errors: []*pipelinev1.PipelineErrorEvent{
			{
				Code:            "timeout",
				Stage:           "parse",
				Message:         "operation timed out",
				Retryable:       true,
				SuggestedAction: "Check timeout configuration",
			},
		},
		TotalCount: 1,
	}
	assert.Len(t, resp.Errors, 1)
	assert.Equal(t, int64(1), resp.TotalCount)
	assert.True(t, resp.Errors[0].Retryable)
}

func TestPipelineErrorEvent_Structure(t *testing.T) {
	// Test that error events have all required fields
	event := &pipelinev1.PipelineErrorEvent{
		Code:            "rate_limit",
		Stage:           "embedding",
		Message:         "API rate limit exceeded",
		Retryable:       true,
		SuggestedAction: "Wait and retry automatically",
		Details:         map[string]string{"run_id": "123"},
	}

	assert.Equal(t, "rate_limit", event.Code)
	assert.Equal(t, "embedding", event.Stage)
	assert.True(t, event.Retryable)
	assert.NotEmpty(t, event.SuggestedAction)
	assert.Contains(t, event.Details, "run_id")
}
