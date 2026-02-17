//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// concurrencyConfigResponse represents JSON from `penf pipeline concurrency -o json`.
type concurrencyConfigResponse struct {
	MaxConcurrent int   `json:"max_concurrent"`
	InFlightCount int   `json:"in_flight_count"`
	PendingCount  int64 `json:"pending_count"`
}

// kickResponse represents the enhanced JSON from `penf pipeline kick -o json`.
type kickResponse struct {
	QueuedCount   int64 `json:"queued_count"`
	Message       string `json:"message"`
	InFlightCount int   `json:"in_flight_count"`
	PendingCount  int64 `json:"pending_count"`
	MaxConcurrent int   `json:"max_concurrent"`
}

// pendingSourcesResponse represents JSON from `penf pipeline pending -o json`.
type pendingSourcesResponse struct {
	Sources    []pendingSource `json:"sources"`
	TotalCount int64           `json:"total_count"`
}

type pendingSource struct {
	ID          int64  `json:"id"`
	ContentType string `json:"content_type"`
	SourceTag   string `json:"source_tag"`
	CreatedAt   string `json:"created_at"`
	SizeBytes   int64  `json:"size_bytes"`
}

// TestPipelineConcurrency_DefaultLimit verifies the default concurrency limit is 1
// and that `penf pipeline concurrency` reports it correctly.
func TestPipelineConcurrency_DefaultLimit(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Get concurrency config via CLI
	var config concurrencyConfigResponse
	err := env.CLI.RunJSON(ctx, &config, "pipeline", "concurrency")
	require.NoError(t, err, "pipeline concurrency command should succeed")

	assert.Equal(t, 1, config.MaxConcurrent, "default max_concurrent should be 1")
	assert.GreaterOrEqual(t, config.InFlightCount, 0, "in_flight_count should be non-negative")
	assert.GreaterOrEqual(t, config.PendingCount, int64(0), "pending_count should be non-negative")
}

// TestPipelineConcurrency_SetAndGet tests setting and reading the concurrency limit.
func TestPipelineConcurrency_SetAndGet(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Set concurrency to 3
	result := env.CLI.Run(ctx, "pipeline", "concurrency", "set", "3")
	require.True(t, result.Success(), "set concurrency should succeed: %s", result.Stderr)

	// Verify it took effect
	var config concurrencyConfigResponse
	err := env.CLI.RunJSON(ctx, &config, "pipeline", "concurrency")
	require.NoError(t, err)
	assert.Equal(t, 3, config.MaxConcurrent, "max_concurrent should be updated to 3")

	// Restore default
	t.Cleanup(func() {
		env.CLI.Run(ctx, "pipeline", "concurrency", "set", "1")
	})
}

// TestPipelineConcurrency_SetRejectsZeroOrNegative tests that invalid values are rejected.
func TestPipelineConcurrency_SetRejectsZeroOrNegative(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Zero should be rejected (pause/resume is the way to stop processing)
	result := env.CLI.Run(ctx, "pipeline", "concurrency", "set", "0")
	assert.NotEqual(t, 0, result.ExitCode, "set concurrency to 0 should fail")

	// Negative should be rejected
	result = env.CLI.Run(ctx, "pipeline", "concurrency", "set", "-1")
	assert.NotEqual(t, 0, result.ExitCode, "set concurrency to -1 should fail")
}

// TestPipelineConcurrency_KickRespectsLimit is the core acceptance test.
// It verifies that kick never starts more workflows than max_concurrent allows.
//
// Setup:
//   1. Clean test tenant
//   2. Ingest 4 emails (creates 4 pending sources)
//   3. Set concurrency limit to 2
//   4. Kick processing
//
// Assertions:
//   - Kick starts exactly 2 items (not 4)
//   - In-flight count is 2
//   - Pending count is 2
//   - A second kick with 2 already in-flight starts 0 more
func TestPipelineConcurrency_KickRespectsLimit(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Clean slate
	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Ingest 4 test emails to create pending sources
	emailDir := env.FixturePath("emails")
	result := env.CLI.Run(ctx, "ingest", "email", emailDir, "--source", "e2e-concurrency")
	require.True(t, result.Success(), "ingest should succeed: %s", result.Stderr)

	// Verify we have pending sources
	var pendingBefore int64
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM sources
		WHERE tenant_id = $1 AND processing_status = 'pending'
	`, env.TenantID).Scan(&pendingBefore)
	require.NoError(t, err)
	require.GreaterOrEqual(t, pendingBefore, int64(2),
		"need at least 2 pending sources for this test, got %d", pendingBefore)

	t.Logf("Pending sources before kick: %d", pendingBefore)

	// Set concurrency limit to 2
	result = env.CLI.Run(ctx, "pipeline", "concurrency", "set", "2")
	require.True(t, result.Success(), "set concurrency should succeed: %s", result.Stderr)

	// Kick processing
	var kick kickResponse
	err = env.CLI.RunJSON(ctx, &kick, "pipeline", "kick")
	require.NoError(t, err, "pipeline kick should succeed")

	// Kick should have started exactly 2 (not all pending)
	assert.Equal(t, int64(2), kick.QueuedCount,
		"kick should start exactly max_concurrent items")
	assert.Equal(t, 2, kick.InFlightCount,
		"in_flight_count should equal max_concurrent after kick")
	assert.Equal(t, 2, kick.MaxConcurrent,
		"response should include max_concurrent")
	assert.Equal(t, pendingBefore-2, kick.PendingCount,
		"pending should decrease by queued count")

	// Verify via DB: exactly 2 sources should be 'processing'
	var processingCount int64
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM sources
		WHERE tenant_id = $1 AND processing_status = 'processing'
	`, env.TenantID).Scan(&processingCount)
	require.NoError(t, err)
	assert.Equal(t, int64(2), processingCount,
		"exactly 2 sources should be in 'processing' state")

	// Second kick should start 0 more (slots full)
	var kick2 kickResponse
	err = env.CLI.RunJSON(ctx, &kick2, "pipeline", "kick")
	require.NoError(t, err)
	assert.Equal(t, int64(0), kick2.QueuedCount,
		"second kick should start 0 items when slots are full")
	assert.Equal(t, 2, kick2.InFlightCount,
		"in_flight should still be 2")

	// Cleanup: restore concurrency and terminate test workflows
	t.Cleanup(func() {
		env.CLI.Run(ctx, "pipeline", "concurrency", "set", "1")
	})
}

// TestPipelineConcurrency_AutoDrain verifies that when a pipeline workflow
// completes, the next pending item is automatically started.
//
// This is the self-draining queue test:
//   1. Ingest 3 emails
//   2. Set concurrency to 1
//   3. Kick (starts item A)
//   4. Wait for A to complete
//   5. Verify B was automatically started (without manual kick)
func TestPipelineConcurrency_AutoDrain(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Clean slate
	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Load fixtures for entity resolution
	err = env.LoadFixtureCLI("acme-corp")
	require.NoError(t, err)

	// Ingest test emails
	emailDir := env.FixturePath("emails")
	result := env.CLI.Run(ctx, "ingest", "email", emailDir, "--source", "e2e-autodrain")
	require.True(t, result.Success(), "ingest should succeed: %s", result.Stderr)

	// Count pending
	var totalPending int64
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM sources
		WHERE tenant_id = $1 AND processing_status = 'pending'
	`, env.TenantID).Scan(&totalPending)
	require.NoError(t, err)
	require.GreaterOrEqual(t, totalPending, int64(2),
		"need at least 2 pending sources for auto-drain test")

	t.Logf("Total pending: %d", totalPending)

	// Set concurrency to 1
	result = env.CLI.Run(ctx, "pipeline", "concurrency", "set", "1")
	require.True(t, result.Success(), "set concurrency should succeed")

	// Kick - should start exactly 1
	var kick kickResponse
	err = env.CLI.RunJSON(ctx, &kick, "pipeline", "kick")
	require.NoError(t, err)
	assert.Equal(t, int64(1), kick.QueuedCount, "should start exactly 1 item")

	// Wait for the first item to complete (real pipeline processing)
	t.Log("Waiting for first pipeline item to complete...")
	firstCompleted := waitForNCompleted(t, env.E2EEnv, 1, 5*time.Minute)
	require.True(t, firstCompleted, "first item should complete within timeout")

	// After completion, auto-drain should have started the next item.
	// Give it a moment to trigger the callback.
	time.Sleep(3 * time.Second)

	// Check that a second item is now processing (auto-kicked)
	var processingCount int64
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM sources
		WHERE tenant_id = $1 AND processing_status = 'processing'
	`, env.TenantID).Scan(&processingCount)
	require.NoError(t, err)
	assert.Equal(t, int64(1), processingCount,
		"auto-drain should have started the next pending item")

	// Cleanup
	t.Cleanup(func() {
		env.CLI.Run(ctx, "pipeline", "concurrency", "set", "1")
	})
}

// TestPipelineConcurrency_PauseResume tests pausing and resuming processing.
func TestPipelineConcurrency_PauseResume(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Set concurrency to 2 first
	result := env.CLI.Run(ctx, "pipeline", "concurrency", "set", "2")
	require.True(t, result.Success())

	// Pause
	result = env.CLI.Run(ctx, "pipeline", "pause")
	require.True(t, result.Success(), "pause should succeed: %s", result.Stderr)

	// Verify concurrency is now 0
	var config concurrencyConfigResponse
	err := env.CLI.RunJSON(ctx, &config, "pipeline", "concurrency")
	require.NoError(t, err)
	assert.Equal(t, 0, config.MaxConcurrent, "paused pipeline should have max_concurrent=0")

	// Kick should start nothing
	var kick kickResponse
	err = env.CLI.RunJSON(ctx, &kick, "pipeline", "kick")
	require.NoError(t, err)
	assert.Equal(t, int64(0), kick.QueuedCount, "kick while paused should start nothing")

	// Resume should restore to 2
	result = env.CLI.Run(ctx, "pipeline", "resume")
	require.True(t, result.Success(), "resume should succeed: %s", result.Stderr)

	// Verify restored
	err = env.CLI.RunJSON(ctx, &config, "pipeline", "concurrency")
	require.NoError(t, err)
	assert.Equal(t, 2, config.MaxConcurrent, "resumed pipeline should restore max_concurrent=2")

	// Cleanup
	t.Cleanup(func() {
		env.CLI.Run(ctx, "pipeline", "concurrency", "set", "1")
	})
}

// TestPipelineConcurrency_PendingList tests the `penf pipeline pending` command.
func TestPipelineConcurrency_PendingList(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Clean slate and ingest some emails
	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	emailDir := env.FixturePath("emails")
	result := env.CLI.Run(ctx, "ingest", "email", emailDir, "--source", "e2e-pending")
	require.True(t, result.Success(), "ingest should succeed: %s", result.Stderr)

	// List pending sources via CLI
	var pending pendingSourcesResponse
	err = env.CLI.RunJSON(ctx, &pending, "pipeline", "pending")
	require.NoError(t, err, "pipeline pending should succeed")

	assert.Greater(t, pending.TotalCount, int64(0), "should have pending sources")
	assert.Len(t, pending.Sources, int(pending.TotalCount),
		"sources array should match total_count (within default limit)")

	// Each source should have required fields
	for _, src := range pending.Sources {
		assert.Greater(t, src.ID, int64(0), "source ID should be positive")
		assert.NotEmpty(t, src.SourceTag, "source_tag should not be empty")
		assert.NotEmpty(t, src.CreatedAt, "created_at should not be empty")
	}

	// Filter by source tag
	var filtered pendingSourcesResponse
	err = env.CLI.RunJSON(ctx, &filtered, "pipeline", "pending", "--source", "e2e-pending")
	require.NoError(t, err)
	assert.Equal(t, pending.TotalCount, filtered.TotalCount,
		"filtering by the same source tag should return same count")
}

// TestPipelineConcurrency_QueueShowsConcurrency tests that `penf pipeline queue`
// includes the concurrency header info in JSON output.
func TestPipelineConcurrency_QueueShowsConcurrency(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Get queue status as JSON
	var queueResp struct {
		Queues        []interface{} `json:"queues"`
		MaxConcurrent int           `json:"max_concurrent"`
		InFlightCount int           `json:"in_flight_count"`
		PendingCount  int64         `json:"pending_count"`
	}
	err := env.CLI.RunJSON(ctx, &queueResp, "pipeline", "queue")
	require.NoError(t, err, "pipeline queue should succeed")

	// Should include concurrency fields
	assert.GreaterOrEqual(t, queueResp.MaxConcurrent, 0,
		"queue response should include max_concurrent")
}

// TestPipelineConcurrency_KickWithExplicitLimit tests that --limit flag
// interacts correctly with concurrency (takes the smaller of the two).
func TestPipelineConcurrency_KickWithExplicitLimit(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Clean slate and create pending sources
	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	emailDir := env.FixturePath("emails")
	result := env.CLI.Run(ctx, "ingest", "email", emailDir, "--source", "e2e-limit")
	require.True(t, result.Success())

	// Set concurrency to 5 (high)
	result = env.CLI.Run(ctx, "pipeline", "concurrency", "set", "5")
	require.True(t, result.Success())

	// Kick with explicit --limit=1 (should cap at 1 despite 5 slots)
	var kick kickResponse
	err = env.CLI.RunJSON(ctx, &kick, "pipeline", "kick", "--limit=1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), kick.QueuedCount,
		"explicit --limit should cap the number of items started")

	// Cleanup
	t.Cleanup(func() {
		env.CLI.Run(ctx, "pipeline", "concurrency", "set", "1")
	})
}

// --- Helpers ---

// waitForNCompleted polls the DB until at least N sources have completed processing.
func waitForNCompleted(t *testing.T, env *E2EEnv, n int, timeout time.Duration) bool {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		var completed int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM sources
			WHERE tenant_id = $1 AND processing_status = 'completed'
		`, env.TenantID).Scan(&completed)

		if err == nil && completed >= n {
			t.Logf("Completed: %d/%d", completed, n)
			return true
		}

		time.Sleep(3 * time.Second)
	}

	// Log final state for debugging
	var pending, processing, completed, failed int
	_ = env.DB.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE processing_status = 'pending'),
			COUNT(*) FILTER (WHERE processing_status = 'processing'),
			COUNT(*) FILTER (WHERE processing_status = 'completed'),
			COUNT(*) FILTER (WHERE processing_status = 'failed')
		FROM sources WHERE tenant_id = $1
	`, env.TenantID).Scan(&pending, &processing, &completed, &failed)

	t.Logf("Timeout waiting for %d completed. State: pending=%d processing=%d completed=%d failed=%d",
		n, pending, processing, completed, failed)

	return false
}

// concurrencyConfigJSON is a convenience wrapper to parse concurrency config.
func concurrencyConfigJSON(t *testing.T, env *E2EEnv) concurrencyConfigResponse {
	t.Helper()
	ctx := context.Background()
	var config concurrencyConfigResponse
	err := env.CLI.RunJSON(ctx, &config, "pipeline", "concurrency")
	require.NoError(t, err)
	return config
}
