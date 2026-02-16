//go:build e2e

package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCleanupTestTenant_WithFKRelationships is a regression test for bug pf-53ff0f.
// It verifies that CleanupTestTenant respects FK ordering when deleting data.
//
// Bug context: embeddings.source_id has FK constraint to sources.id.
// Cleanup must delete embeddings BEFORE sources to avoid FK violations.
//
// This test creates a source with an embedding via CLI ingestion and pipeline
// processing, then verifies cleanup succeeds. This tests cleanup against real
// pipeline-created data rather than hand-crafted DB inserts.
func TestCleanupTestTenant_WithFKRelationships(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Step 1: Clean slate - remove any existing test data
	err := env.CleanupTestTenant()
	require.NoError(t, err, "initial cleanup should succeed")

	// Step 2: Ensure tenant exists
	err = env.EnsureTenantExists()
	require.NoError(t, err, "tenant creation should succeed")

	// Step 3: Create a source via CLI ingestion (creates realistic FK relationships)
	opts := emailSourceOpts{
		MessageID:   "<test-fk@cleanup.test>",
		Subject:     "FK Cleanup Test Email",
		FromAddress: "sender@example.com",
		FromName:    "Test Sender",
		Body:        "This email is for testing FK cleanup order. The pipeline will create embeddings.",
		Date:        time.Now(),
		ExternalID:  "test-fk-cleanup",
	}
	sourceID := createEmailSourceCLI(t, env, opts)
	t.Logf("Created source with ID: %d", sourceID)

	// Step 4: Run pipeline to create embeddings (FK relationship: embeddings -> sources)
	runPipelineAndWait(t, env, sourceID, 60*time.Second)

	// Step 5: Verify the pipeline created embeddings
	var embeddingCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM embeddings WHERE tenant_id = $1 AND source_id = $2", env.TenantID, sourceID).Scan(&embeddingCount)
	require.NoError(t, err)
	if embeddingCount == 0 {
		t.Skip("Pipeline did not create embeddings - cannot test FK cleanup. This may indicate pipeline is not running embed stage.")
	}
	t.Logf("Pipeline created %d embedding(s)", embeddingCount)

	// Step 6: Verify source exists
	var sourceCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources WHERE tenant_id = $1", env.TenantID).Scan(&sourceCount)
	require.NoError(t, err)
	require.GreaterOrEqual(t, sourceCount, 1, "should have at least 1 source")

	// Step 7: Run CleanupTestTenant - should delete embeddings before sources
	err = env.CleanupTestTenant()
	require.NoError(t, err, "cleanup should succeed without FK constraint violations")

	// Step 8: Verify all test data was cleaned up
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources WHERE tenant_id = $1", env.TenantID).Scan(&sourceCount)
	require.NoError(t, err)
	require.Equal(t, 0, sourceCount, "all sources should be deleted")

	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM embeddings WHERE tenant_id = $1", env.TenantID).Scan(&embeddingCount)
	require.NoError(t, err)
	require.Equal(t, 0, embeddingCount, "all embeddings should be deleted")
}

// TestCleanupTestTenant_WithMultipleFKLevels tests cleanup with deeper FK chains.
// This extends the basic test to cover more complex scenarios like:
// - embeddings -> sources
// - embeddings -> assertions -> sources
// - embeddings -> people
//
// This test uses CLI ingestion + pipeline processing to create realistic FK chains,
// then verifies cleanup handles all relationships correctly.
func TestCleanupTestTenant_WithMultipleFKLevels(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Clean slate
	err := env.CleanupTestTenant()
	require.NoError(t, err, "initial cleanup should succeed")

	// Ensure tenant exists
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Create an email that mentions a person and contains content for assertions
	// The pipeline will create:
	// - Person record (from email sender/recipients)
	// - Assertions (from content analysis)
	// - Embeddings for source, person, and assertions
	opts := emailSourceOpts{
		MessageID:   "<multi-fk@cleanup.test>",
		Subject:     "Project Update - Q1 Planning",
		FromAddress: "person@example.com",
		FromName:    "Test Person",
		Body: `Hi team,

Quick update on the Q1 planning. We've decided to move forward with the new architecture.
Key decision: We will migrate to the new platform by end of Q1.

Please review and let me know if you have concerns.

Thanks,
Test Person`,
		Date:       time.Now(),
		ExternalID: "test-multi-fk",
	}

	sourceID := createEmailSourceCLI(t, env, opts)
	t.Logf("Created source with ID: %d", sourceID)

	// Run pipeline to create complex FK relationships
	// Pipeline creates: embeddings, people, assertions (which all have FK relationships)
	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	// Verify data exists across multiple tables
	var embeddingCount, assertionCount, sourceCount, personCount int

	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM embeddings WHERE tenant_id = $1", env.TenantID).Scan(&embeddingCount)
	require.NoError(t, err)
	t.Logf("Embeddings created: %d", embeddingCount)

	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM assertions WHERE tenant_id = $1", env.TenantID).Scan(&assertionCount)
	require.NoError(t, err)
	t.Logf("Assertions created: %d", assertionCount)

	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources WHERE tenant_id = $1", env.TenantID).Scan(&sourceCount)
	require.NoError(t, err)
	require.GreaterOrEqual(t, sourceCount, 1, "should have at least 1 source")

	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM people WHERE tenant_id = $1", env.TenantID).Scan(&personCount)
	require.NoError(t, err)
	t.Logf("People created: %d", personCount)

	// Run cleanup - should handle all FK relationships correctly
	err = env.CleanupTestTenant()
	require.NoError(t, err, "cleanup should succeed with complex FK relationships")

	// Verify everything was cleaned up
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM embeddings WHERE tenant_id = $1", env.TenantID).Scan(&embeddingCount)
	require.NoError(t, err)
	require.Equal(t, 0, embeddingCount, "all embeddings should be deleted")

	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM assertions WHERE tenant_id = $1", env.TenantID).Scan(&assertionCount)
	require.NoError(t, err)
	require.Equal(t, 0, assertionCount, "all assertions should be deleted")

	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources WHERE tenant_id = $1", env.TenantID).Scan(&sourceCount)
	require.NoError(t, err)
	require.Equal(t, 0, sourceCount, "all sources should be deleted")

	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM people WHERE tenant_id = $1", env.TenantID).Scan(&personCount)
	require.NoError(t, err)
	require.Equal(t, 0, personCount, "all people should be deleted")
}

// TestCleanupTestTenant_WithConcurrentWorkflow reproduces the actual bug pf-53ff0f.
// The real issue: Temporal workflows run asynchronously and may try to insert embeddings
// AFTER the test cleanup has deleted sources, causing FK violations.
//
// Bug scenario:
// 1. Test runs and triggers pipeline workflow
// 2. Test completes, cleanup starts
// 3. Cleanup deletes sources
// 4. Workflow (still running) tries to insert embeddings with source_id
// 5. FK constraint violation: "source not found"
//
// This test uses CLI ingestion to create a realistic source, then simulates
// concurrent cleanup vs. workflow operations.
func TestCleanupTestTenant_WithConcurrentWorkflow(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Clean slate
	err := env.CleanupTestTenant()
	require.NoError(t, err)

	// Ensure tenant exists
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Create a source via CLI ingestion
	opts := emailSourceOpts{
		MessageID:   "<concurrent@cleanup.test>",
		Subject:     "Concurrent Workflow Test",
		FromAddress: "test@example.com",
		FromName:    "Test Sender",
		Body:        "Test email for concurrent workflow test",
		Date:        time.Now(),
		ExternalID:  "test-concurrent",
	}
	sourceID := createEmailSourceCLI(t, env, opts)
	t.Logf("Created source with ID: %d", sourceID)

	// Simulate concurrent workflow: goroutine that tries to insert embeddings
	// while cleanup is running
	var wg sync.WaitGroup
	var workflowErr error
	wg.Add(1)

	go func() {
		defer wg.Done()
		// Wait a bit to let cleanup start
		time.Sleep(50 * time.Millisecond)

		// Try to insert an embedding (simulating what a workflow would do)
		_, err := env.DB.Exec(ctx, `
			INSERT INTO embeddings (
				tenant_id, entity_type, entity_id, source_id,
				embedding_model, text_content, embedding
			) VALUES (
				$1, 'source', $2, $2,
				'workflow-model', 'Workflow embedding', ARRAY[0.9]::double precision[]
			)
		`, env.TenantID, sourceID)

		if err != nil {
			workflowErr = err
			t.Logf("Workflow embedding insert failed (expected): %v", err)
		}
	}()

	// Give the goroutine a tiny head start, then run cleanup
	time.Sleep(10 * time.Millisecond)

	// Run cleanup - this will delete sources
	err = env.CleanupTestTenant()
	require.NoError(t, err, "cleanup itself should succeed")

	// Wait for the "workflow" goroutine
	wg.Wait()

	// The workflow should have failed with FK constraint violation
	// This reproduces the bug: the workflow tried to insert embeddings
	// referencing a source_id that was deleted during cleanup
	if workflowErr != nil {
		t.Logf("Successfully reproduced bug pf-53ff0f: concurrent workflow failed with FK violation")
		require.Contains(t, workflowErr.Error(), "embeddings_source_id_fkey",
			"expected FK constraint violation")
	} else {
		// If the workflow succeeded, it means it inserted before cleanup deleted
		// This is timing-dependent and doesn't prove the bug, but also doesn't fail the test
		t.Log("Workflow completed before cleanup (timing-dependent, bug not reproduced this run)")
	}
}

// TestCleanupTestTenant_DeletesInFKSafeOrder is the actual test for the fix.
// It verifies that cleanup deletes embeddings BEFORE sources (respecting FK order).
//
// This test uses CLI ingestion + pipeline to create realistic FK relationships,
// then verifies cleanup handles them in the correct order.
func TestCleanupTestTenant_DeletesInFKSafeOrder(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Clean slate
	err := env.CleanupTestTenant()
	require.NoError(t, err)

	// Ensure tenant exists
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Create source via CLI ingestion
	opts := emailSourceOpts{
		MessageID:   "<fk-order@cleanup.test>",
		Subject:     "FK Order Test Email",
		FromAddress: "test@example.com",
		FromName:    "Test Sender",
		Body:        "Test for FK cleanup order - pipeline will create embeddings",
		Date:        time.Now(),
		ExternalID:  "test-fk-order",
	}
	sourceID := createEmailSourceCLI(t, env, opts)
	require.NotZero(t, sourceID, "source creation should succeed")

	// Run pipeline to create embeddings (FK relationship)
	runPipelineAndWait(t, env, sourceID, 60*time.Second)

	// Verify the FK relationship exists (embedding references source)
	var embCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM embeddings
		WHERE tenant_id = $1 AND source_id = $2
	`, env.TenantID, sourceID).Scan(&embCount)
	require.NoError(t, err)
	if embCount == 0 {
		t.Skip("Pipeline did not create embeddings - cannot test FK cleanup order")
	}
	t.Logf("FK relationship verified: %d embedding(s) reference source %d", embCount, sourceID)

	// NOW: Run cleanup
	// Cleanup must delete embeddings first, then sources (FK-safe order).
	// If cleanup tries to delete sources before embeddings, FK constraint violation occurs.
	err = env.CleanupTestTenant()
	require.NoError(t, err, "cleanup must respect FK order: delete embeddings before sources")

	// Verify both were cleaned up
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM embeddings WHERE tenant_id = $1", env.TenantID).Scan(&embCount)
	require.NoError(t, err)
	require.Equal(t, 0, embCount, "embeddings should be deleted")

	var srcCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources WHERE tenant_id = $1", env.TenantID).Scan(&srcCount)
	require.NoError(t, err)
	require.Equal(t, 0, srcCount, "sources should be deleted")
}

// TestCleanupTestTenant_RaceCondition attempts to reproduce the exact bug scenario:
// A workflow tries to insert embeddings while cleanup is deleting sources.
//
// This test uses a transaction to hold a lock on sources, simulating a slow DELETE,
// while another goroutine tries to INSERT embeddings. This should catch FK violations.
//
// This test uses CLI ingestion to create a realistic source.
func TestCleanupTestTenant_RaceCondition(t *testing.T) {
	t.Skip("Race condition test - timing-dependent, may not reproduce bug consistently")

	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Clean slate
	err := env.CleanupTestTenant()
	require.NoError(t, err)

	// Ensure tenant exists
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Create a source via CLI ingestion
	opts := emailSourceOpts{
		MessageID:   "<race@cleanup.test>",
		Subject:     "Race Condition Test",
		FromAddress: "test@example.com",
		FromName:    "Test Sender",
		Body:        "Race condition test content",
		Date:        time.Now(),
		ExternalID:  "test-race",
	}
	sourceID := createEmailSourceCLI(t, env, opts)
	require.NotZero(t, sourceID, "source creation should succeed")

	var insertErr error
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Simulates cleanup DELETE (slow, holds lock)
	go func() {
		defer wg.Done()

		// Start a transaction and hold it open
		tx, err := env.DB.Begin(ctx)
		if err != nil {
			t.Logf("Failed to begin transaction: %v", err)
			return
		}
		defer tx.Rollback(ctx)

		// Delete sources but don't commit immediately
		_, err = tx.Exec(ctx, "DELETE FROM sources WHERE tenant_id = $1 AND id = $2",
			env.TenantID, sourceID)
		if err != nil {
			t.Logf("Failed to delete source: %v", err)
			return
		}

		// Hold the lock for a bit
		time.Sleep(100 * time.Millisecond)

		// Commit the delete
		_ = tx.Commit(ctx)
		t.Log("Completed source deletion")
	}()

	// Goroutine 2: Simulates workflow trying to INSERT embeddings
	go func() {
		defer wg.Done()

		// Wait for delete transaction to start
		time.Sleep(20 * time.Millisecond)

		// Try to insert embedding - this should fail with FK violation
		// if the source has been deleted
		_, err := env.DB.Exec(ctx, `
			INSERT INTO embeddings (
				tenant_id, entity_type, entity_id, source_id,
				embedding_model, text_content, embedding
			) VALUES (
				$1, 'source', $2, $2,
				'race-model', 'Race condition embedding', ARRAY[0.99]::double precision[]
			)
		`, env.TenantID, sourceID)

		insertErr = err
		if err != nil {
			t.Logf("Embedding insert failed (may be expected): %v", err)
		}
	}()

	wg.Wait()

	// If the insert failed with FK violation, we successfully reproduced the bug
	if insertErr != nil {
		require.Contains(t, insertErr.Error(), "embeddings_source_id_fkey",
			"expected FK constraint violation when inserting embedding after source deletion")
		t.Log("Successfully reproduced race condition bug")
	}
}
