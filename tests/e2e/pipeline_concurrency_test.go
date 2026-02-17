//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPipelineConcurrency_SetLimit verifies that `penf pipeline concurrency set N`
// updates the max concurrent pipelines setting.
func TestPipelineConcurrency_SetLimit(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Set concurrency to 2
	setResult := env.SafeCLI.Run(ctx, "pipeline", "concurrency", "set", "2")
	require.True(t, setResult.Success(), "concurrency set should succeed: %s", setResult.Stderr)

	// Verify it reads back as 2
	getResult := env.SafeCLI.Run(ctx, "pipeline", "concurrency", "-o", "json")
	require.True(t, getResult.Success(), "concurrency get should succeed: %s", getResult.Stderr)

	var config struct {
		MaxConcurrent int `json:"max_concurrent"`
	}
	require.NoError(t, json.Unmarshal([]byte(getResult.Stdout), &config),
		"should parse concurrency JSON: %s", getResult.Stdout)
	assert.Equal(t, 2, config.MaxConcurrent, "max concurrent should be 2")

	// Set to 5, verify again
	setResult = env.SafeCLI.Run(ctx, "pipeline", "concurrency", "set", "5")
	require.True(t, setResult.Success(), "concurrency set to 5 should succeed: %s", setResult.Stderr)

	getResult = env.SafeCLI.Run(ctx, "pipeline", "concurrency", "-o", "json")
	require.True(t, getResult.Success())
	require.NoError(t, json.Unmarshal([]byte(getResult.Stdout), &config))
	assert.Equal(t, 5, config.MaxConcurrent, "max concurrent should be 5")

	// Clean up: reset to default (2)
	t.Cleanup(func() {
		resetCtx, resetCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer resetCancel()
		env.SafeCLI.Run(resetCtx, "pipeline", "concurrency", "set", "2")
	})
}

// TestPipelineConcurrency_GetLimit verifies that the concurrency setting
// is readable and returns a valid value.
func TestPipelineConcurrency_GetLimit(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := env.SafeCLI.Run(ctx, "pipeline", "concurrency", "-o", "json")
	require.True(t, result.Success(), "concurrency get should succeed: %s", result.Stderr)

	var config struct {
		MaxConcurrent int `json:"max_concurrent"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Stdout), &config),
		"should parse concurrency JSON: %s", result.Stdout)

	assert.Greater(t, config.MaxConcurrent, 0,
		"max concurrent should be positive (default is 2)")
}

// TestPipelineConcurrency_BatchReprocess verifies that batch reprocessing
// respects the concurrency limit. With concurrency=1, items should process
// sequentially (each one completes before the next starts).
func TestPipelineConcurrency_BatchReprocess(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	// Set concurrency to 1 for deterministic ordering
	setResult := env.SafeCLI.Run(ctx, "pipeline", "concurrency", "set", "1")
	require.True(t, setResult.Success(), "concurrency set should succeed: %s", setResult.Stderr)

	// Get 3 content items to batch
	contentResult := env.SafeCLI.Run(ctx, "content", "list", "-o", "json", "--limit", "3")
	require.True(t, contentResult.Success(), "content list should succeed: %s", contentResult.Stderr)

	var contentList struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(contentResult.Stdout), &contentList))
	require.GreaterOrEqual(t, len(contentList.Items), 3,
		"need at least 3 content items for batch test")

	// Start batch with 3 items
	ids := []string{contentList.Items[0].ID, contentList.Items[1].ID, contentList.Items[2].ID}
	batchResult := env.SafeCLI.Run(ctx, "pipeline", "batch",
		ids[0], ids[1], ids[2], "-o", "json")
	require.True(t, batchResult.Success(), "batch start should succeed: %s", batchResult.Stderr)

	var batchStart struct {
		BatchID    string `json:"batch_id"`
		TotalItems int    `json:"total_items"`
	}
	require.NoError(t, json.Unmarshal([]byte(batchResult.Stdout), &batchStart),
		"should parse batch start JSON: %s", batchResult.Stdout)

	assert.NotEmpty(t, batchStart.BatchID, "batch ID should be populated")
	assert.Equal(t, 3, batchStart.TotalItems, "should have 3 items in batch")

	// Poll for completion (up to 5 minutes)
	deadline := time.Now().Add(5 * time.Minute)
	var lastProgress struct {
		Status    string `json:"status"`
		Total     int    `json:"total"`
		Completed int    `json:"completed"`
		Failed    int    `json:"failed"`
		Active    int    `json:"active"`
		Pending   int    `json:"pending"`
	}

	for time.Now().Before(deadline) {
		statusResult := env.SafeCLI.Run(ctx, "pipeline", "batch", "status",
			batchStart.BatchID, "-o", "json")
		if statusResult.Success() {
			if err := json.Unmarshal([]byte(statusResult.Stdout), &lastProgress); err == nil {
				t.Logf("Batch progress: %d/%d completed, %d active, %d failed",
					lastProgress.Completed, lastProgress.Total,
					lastProgress.Active, lastProgress.Failed)

				// Verify concurrency limit is respected
				assert.LessOrEqual(t, lastProgress.Active, 1,
					"at most 1 pipeline should be active (concurrency=1)")

				if lastProgress.Status == "completed" || lastProgress.Status == "failed" {
					break
				}
			}
		}
		time.Sleep(5 * time.Second)
	}

	// Verify batch completed
	assert.Equal(t, "completed", lastProgress.Status,
		"batch should have completed")
	assert.Equal(t, 3, lastProgress.Total, "total should be 3")
	assert.Equal(t, 3, lastProgress.Completed+lastProgress.Failed,
		"all items should be completed or failed")

	// Clean up: restore default concurrency
	t.Cleanup(func() {
		resetCtx, resetCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer resetCancel()
		env.SafeCLI.Run(resetCtx, "pipeline", "concurrency", "set", "2")
	})
}

// TestPipelineConcurrency_BatchProgress verifies that batch status shows
// correct progress with pending/active/completed/failed counts.
func TestPipelineConcurrency_BatchProgress(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Set concurrency to 1 to ensure we can catch the "in progress" state
	setResult := env.SafeCLI.Run(ctx, "pipeline", "concurrency", "set", "1")
	require.True(t, setResult.Success())

	// Get 2 content items
	contentResult := env.SafeCLI.Run(ctx, "content", "list", "-o", "json", "--limit", "2")
	require.True(t, contentResult.Success())

	var contentList struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(contentResult.Stdout), &contentList))
	require.GreaterOrEqual(t, len(contentList.Items), 2)

	// Start batch
	batchResult := env.SafeCLI.Run(ctx, "pipeline", "batch",
		contentList.Items[0].ID, contentList.Items[1].ID, "-o", "json")
	require.True(t, batchResult.Success(), "batch start should succeed: %s", batchResult.Stderr)

	var batchStart struct {
		BatchID string `json:"batch_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(batchResult.Stdout), &batchStart))

	// Check status immediately — should show items in progress or pending
	time.Sleep(2 * time.Second) // Brief pause to let batch start
	statusResult := env.SafeCLI.Run(ctx, "pipeline", "batch", "status",
		batchStart.BatchID, "-o", "json")
	require.True(t, statusResult.Success(), "batch status should succeed: %s", statusResult.Stderr)

	var progress struct {
		BatchID   string `json:"batch_id"`
		Status    string `json:"status"`
		Total     int    `json:"total"`
		Completed int    `json:"completed"`
		Failed    int    `json:"failed"`
		Active    int    `json:"active"`
		Pending   int    `json:"pending"`
	}
	require.NoError(t, json.Unmarshal([]byte(statusResult.Stdout), &progress),
		"should parse batch status JSON: %s", statusResult.Stdout)

	assert.Equal(t, batchStart.BatchID, progress.BatchID)
	assert.Equal(t, 2, progress.Total, "total should be 2")
	assert.Equal(t, "running", progress.Status, "batch should be running")

	// The counts should sum to total
	assert.Equal(t, progress.Total,
		progress.Completed+progress.Failed+progress.Active+progress.Pending,
		"counts should sum to total")

	// Clean up
	t.Cleanup(func() {
		resetCtx, resetCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer resetCancel()
		env.SafeCLI.Run(resetCtx, "pipeline", "concurrency", "set", "2")
	})
}

// TestPipelineConcurrency_BatchList verifies that `penf pipeline batch list`
// shows active and recent batches.
func TestPipelineConcurrency_BatchList(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result := env.SafeCLI.Run(ctx, "pipeline", "batch", "list", "-o", "json")
	require.True(t, result.Success(), "batch list should succeed: %s", result.Stderr)

	var batchList struct {
		Batches []struct {
			ID        string `json:"id"`
			Status    string `json:"status"`
			Total     int    `json:"total"`
			Completed int    `json:"completed"`
			Failed    int    `json:"failed"`
			CreatedAt string `json:"created_at"`
		} `json:"batches"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Stdout), &batchList),
		"should parse batch list JSON: %s", result.Stdout)

	// Just verify the structure is correct — there may or may not be batches
	// from previous test runs
	for _, batch := range batchList.Batches {
		assert.NotEmpty(t, batch.ID, "batch ID should not be empty")
		assert.Contains(t, []string{"running", "completed", "cancelled", "failed"}, batch.Status,
			"batch status should be valid")
		assert.Greater(t, batch.Total, 0, "batch total should be positive")
	}
}
