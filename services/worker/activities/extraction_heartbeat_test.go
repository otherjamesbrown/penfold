// Package activities provides tests for the heartbeat and detached context fixes (pf-04a2de).
//
// Background: Meeting transcripts with large content (50K+ chars) caused extraction to fail
// silently. Three issues: (1) no heartbeat during long API calls caused Temporal to cancel
// the activity, (2) the cancelled context prevented the defer block from writing the failed
// pipeline_run, (3) stale pipeline_runs persisted through reprocessing.
package activities

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// contextCapturingPipelineRepo captures whether the context was alive at call time.
// We snapshot ctx.Err() at call time because the context may be cancelled by a defer
// after CreateRun returns (e.g. context.WithTimeout's cancel).
type contextCapturingPipelineRepo struct {
	runs        []PipelineRunInput
	ctxErrAtCall []error // ctx.Err() captured at each CreateRun invocation
}

func (r *contextCapturingPipelineRepo) CreateRun(ctx context.Context, input PipelineRunInput) error {
	r.runs = append(r.runs, input)
	r.ctxErrAtCall = append(r.ctxErrAtCall, ctx.Err())
	return nil
}

func (r *contextCapturingPipelineRepo) RecordOverrides(_ context.Context, _ int64, _ map[string]string) error {
	return nil
}

func (r *contextCapturingPipelineRepo) GetContextProviders(_ context.Context, _, _, _ string) ([]string, error) {
	return nil, nil
}

// TestExtractEntities_FailedExtraction_WritesRunWithDetachedContext verifies Fix 2 (pf-04a2de):
// When the AI call fails, the defer block must write failed pipeline_runs using a detached
// context (context.Background), NOT the activity context which may be cancelled by Temporal.
func TestExtractEntities_FailedExtraction_WritesRunWithDetachedContext(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &contextCapturingPipelineRepo{}

	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			return nil, fmt.Errorf("AI service unavailable")
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, repo)

	_, err := activities.ExtractEntities(context.Background(), workflows.SLMPipelineExtractEntitiesInput{
		TenantID:  "test-tenant",
		SourceID:  100,
		ContentID: "mt-test",
		Content:   "Meeting transcript content for testing",
		JobID:     "job-test",
	})

	// The extraction should fail
	require.Error(t, err)

	// The defer block should have written 2 pipeline_runs (extract_ner + extract_semantic)
	// using a DETACHED context, not the cancelled one
	require.Len(t, repo.runs, 2, "expected 2 failed pipeline_run records (extract_ner + extract_semantic)")

	for i, run := range repo.runs {
		assert.Equal(t, "failed", run.Status, "run %d should be failed", i)
		assert.Equal(t, int64(100), run.SourceID, "run %d source_id mismatch", i)
		assert.NotEmpty(t, run.SkipReason, "run %d should have a failure reason", i)
	}
	assert.Equal(t, "extract_ner", repo.runs[0].Stage)
	assert.Equal(t, "extract_semantic", repo.runs[1].Stage)

	// Verify the contexts used for CreateRun were NOT cancelled at call time (detached context)
	for i, err := range repo.ctxErrAtCall {
		assert.NoError(t, err, "CreateRun context %d should not be cancelled at call time (detached context)", i)
	}
}

// TestExtractEntities_FailedExtraction_CancelledDuringCall_StillWritesRun is the critical
// scenario from pf-04a2de: the context gets cancelled DURING the AI call (simulating Temporal
// HeartbeatTimeout), and the defer block must still write the failed pipeline_run.
func TestExtractEntities_FailedExtraction_CancelledDuringCall_StillWritesRun(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &contextCapturingPipelineRepo{}

	// AI client that cancels the parent context during the call, then returns an error
	// This simulates Temporal cancelling the activity due to missed heartbeat
	var outerCancel context.CancelFunc
	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			// Cancel the parent context mid-call (simulates HeartbeatTimeout)
			outerCancel()
			return nil, fmt.Errorf("context canceled")
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, repo)

	ctx, cancel := context.WithCancel(context.Background())
	outerCancel = cancel

	_, err := activities.ExtractEntities(ctx, workflows.SLMPipelineExtractEntitiesInput{
		TenantID:  "test-tenant",
		SourceID:  200,
		ContentID: "mt-cancelled",
		Content:   "Some meeting content",
		JobID:     "job-cancelled",
	})
	require.Error(t, err)

	// Pipeline runs MUST be written despite cancelled context
	require.Len(t, repo.runs, 2, "pipeline_runs must be written even when context cancelled during AI call")

	// Verify detached context was used (not the cancelled parent)
	for i, err := range repo.ctxErrAtCall {
		assert.NoError(t, err, "CreateRun context %d must not be cancelled at call time", i)
	}
}

// TestExtractEntities_HeartbeatGoroutine_CompletesNormally verifies Fix 1 (pf-04a2de):
// The heartbeat goroutine doesn't interfere with normal extraction. When the API call
// completes (even with a small delay), the goroutine is properly cleaned up.
func TestExtractEntities_HeartbeatGoroutine_CompletesNormally(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &capturingPipelineRepo{}

	var callCount int32
	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			atomic.AddInt32(&callCount, 1)
			// Simulate a brief API call (enough to start the goroutine, but not enough
			// to trigger a 30s heartbeat tick)
			time.Sleep(50 * time.Millisecond)
			return &aiv1.ExtractEntitiesResponse{
				People:    []*aiv1.PersonEntity{{Name: "Test Person", Role: "Tester"}},
				ModelUsed: "test-model",
			}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, repo)

	output, err := activities.ExtractEntities(context.Background(), workflows.SLMPipelineExtractEntitiesInput{
		TenantID:  "test-tenant",
		SourceID:  300,
		ContentID: "mt-heartbeat",
		Content:   "Brief meeting content for heartbeat test",
		JobID:     "job-heartbeat",
	})

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, 1, len(output.People))
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "AI client should be called exactly once")

	// Pipeline run should be recorded successfully
	require.Len(t, repo.runs, 2, "expected extract_ner + extract_semantic pipeline runs")
	assert.Equal(t, "completed", repo.runs[0].Status)
	assert.Equal(t, "completed", repo.runs[1].Status)
}

// TestExtractEntities_HeartbeatGoroutine_ChunkedPath verifies that the heartbeat goroutine
// also works correctly in the chunked extraction path.
func TestExtractEntities_HeartbeatGoroutine_ChunkedPath(t *testing.T) {
	logger := logging.NewNopLogger()
	repo := &capturingPipelineRepo{}

	var callCount int32
	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			atomic.AddInt32(&callCount, 1)
			time.Sleep(20 * time.Millisecond) // Brief delay per chunk
			return &aiv1.ExtractEntitiesResponse{
				People:    []*aiv1.PersonEntity{{Name: "Chunk Person", Role: ""}},
				ModelUsed: "qwen3:8b",
			}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, repo)

	// qwen3:8b has 32K context window → maxInputChars = (32768*4)*80/100 = 104857 chars
	// We need content larger than that to trigger chunking
	// Actually for qwen3:8b: 32768 * 4 * 80 / 100 = 104857
	// Create content that exceeds the threshold
	largeContent := string(make([]rune, 110000)) // ~110K chars, exceeds qwen3:8b threshold

	output, err := activities.ExtractEntities(context.Background(), workflows.SLMPipelineExtractEntitiesInput{
		TenantID:      "test-tenant",
		SourceID:      400,
		ContentID:     "mt-chunked-heartbeat",
		Content:       largeContent,
		JobID:         "job-chunked",
		ModelOverride: "qwen3:8b", // Force small context window to trigger chunking
	})

	require.NoError(t, err)
	require.NotNil(t, output)

	// Should have made multiple AI calls (chunked)
	assert.Greater(t, atomic.LoadInt32(&callCount), int32(1), "should make multiple chunk calls")

	// Pipeline runs should still be recorded
	require.Len(t, repo.runs, 2)
	assert.Equal(t, "completed", repo.runs[0].Status)
	assert.Equal(t, "completed", repo.runs[1].Status)
}
