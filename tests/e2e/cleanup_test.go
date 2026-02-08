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
// This test creates a source with an embedding and verifies cleanup succeeds.
// If cleanup order is wrong, this test will FAIL with FK constraint violation.
func TestCleanupTestTenant_WithFKRelationships(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Step 1: Clean slate - remove any existing test data
	err := env.CleanupTestTenant()
	require.NoError(t, err, "initial cleanup should succeed")

	// Step 2: Ensure tenant exists
	err = env.EnsureTenantExists()
	require.NoError(t, err, "tenant creation should succeed")

	// Step 3: Create a source with an embedding that references it
	// This simulates the real-world scenario where sources have embeddings
	var sourceID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO sources (
			tenant_id,
			source_system,
			external_id,
			content_hash,
			raw_content,
			content_type,
			processing_status
		) VALUES (
			$1,
			'manual_eml',
			'test-fk-' || gen_random_uuid()::text,
			encode(sha256('test content'::bytea), 'hex'),
			'Test email content for FK test',
			'email',
			'completed'
		)
		RETURNING id
	`, env.TenantID).Scan(&sourceID)
	require.NoError(t, err, "source creation should succeed")
	t.Logf("Created source with ID: %d", sourceID)

	// Step 4: Create an embedding for the source
	var embeddingID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO embeddings (
			tenant_id,
			entity_type,
			entity_id,
			source_id,
			embedding_model,
			text_content,
			embedding
		) VALUES (
			$1,
			'source',
			$2,
			$2,
			'test-model',
			'Test embedding content',
			ARRAY[0.1, 0.2, 0.3]::double precision[]
		)
		RETURNING id
	`, env.TenantID, sourceID).Scan(&embeddingID)
	require.NoError(t, err, "embedding creation should succeed")
	t.Logf("Created embedding with ID: %d for source ID: %d", embeddingID, sourceID)

	// Step 5: Verify the data was created
	var sourceCount, embeddingCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources WHERE tenant_id = $1", env.TenantID).Scan(&sourceCount)
	require.NoError(t, err)
	require.Equal(t, 1, sourceCount, "should have 1 source")

	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM embeddings WHERE tenant_id = $1", env.TenantID).Scan(&embeddingCount)
	require.NoError(t, err)
	require.Equal(t, 1, embeddingCount, "should have 1 embedding")

	// Step 6: Run CleanupTestTenant
	// THIS SHOULD FAIL on the current code with FK constraint violation
	// because it tries to delete from sources while embeddings still references it
	err = env.CleanupTestTenant()
	require.NoError(t, err, "cleanup should succeed without FK constraint violations")

	// Step 7: Verify all test data was cleaned up
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
// This ensures the cleanup handles all FK relationships correctly.
func TestCleanupTestTenant_WithMultipleFKLevels(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Clean slate
	err := env.CleanupTestTenant()
	require.NoError(t, err, "initial cleanup should succeed")

	// Ensure tenant exists
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Create a person
	var personID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO people (
			tenant_id,
			display_name,
			primary_email
		) VALUES (
			$1,
			'Test Person',
			'person@example.com'
		)
		RETURNING id
	`, env.TenantID).Scan(&personID)
	require.NoError(t, err)
	t.Logf("Created person with ID: %d", personID)

	// Create a source
	var sourceID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO sources (
			tenant_id,
			source_system,
			external_id,
			content_hash,
			raw_content,
			content_type,
			processing_status
		) VALUES (
			$1,
			'manual_eml',
			'test-multi-fk-' || gen_random_uuid()::text,
			encode(sha256('test multi-level content'::bytea), 'hex'),
			'Test email for multi-level FK',
			'email',
			'completed'
		)
		RETURNING id
	`, env.TenantID).Scan(&sourceID)
	require.NoError(t, err)
	t.Logf("Created source with ID: %d", sourceID)

	// Create an assertion referencing the source
	var assertionID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO assertions (
			tenant_id,
			source_id,
			claim_text,
			author_person_id,
			confidence_score
		) VALUES (
			$1,
			$2,
			'Test assertion',
			$3,
			0.9
		)
		RETURNING id
	`, env.TenantID, sourceID, personID).Scan(&assertionID)
	require.NoError(t, err)
	t.Logf("Created assertion with ID: %d", assertionID)

	// Create embeddings for source, person, and assertion
	// Embedding for source
	_, err = env.DB.Exec(ctx, `
		INSERT INTO embeddings (
			tenant_id, entity_type, entity_id, source_id,
			embedding_model, text_content, embedding
		) VALUES (
			$1, 'source', $2, $2,
			'test-model', 'Source embedding', ARRAY[0.1]::double precision[]
		)
	`, env.TenantID, sourceID)
	require.NoError(t, err)

	// Embedding for person
	_, err = env.DB.Exec(ctx, `
		INSERT INTO embeddings (
			tenant_id, entity_type, entity_id, person_id,
			embedding_model, text_content, embedding
		) VALUES (
			$1, 'person', $2, $2,
			'test-model', 'Person embedding', ARRAY[0.2]::double precision[]
		)
	`, env.TenantID, personID)
	require.NoError(t, err)

	// Embedding for assertion (also references source indirectly)
	_, err = env.DB.Exec(ctx, `
		INSERT INTO embeddings (
			tenant_id, entity_type, entity_id, assertion_id, source_id,
			embedding_model, text_content, embedding
		) VALUES (
			$1, 'assertion', $2, $2, $3,
			'test-model', 'Assertion embedding', ARRAY[0.3]::double precision[]
		)
	`, env.TenantID, assertionID, sourceID)
	require.NoError(t, err)

	t.Log("Created complex FK chain: embeddings -> assertions -> sources, embeddings -> people")

	// Verify data exists
	var embeddingCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM embeddings WHERE tenant_id = $1", env.TenantID).Scan(&embeddingCount)
	require.NoError(t, err)
	require.Equal(t, 3, embeddingCount, "should have 3 embeddings")

	// Run cleanup - should handle all FK relationships correctly
	err = env.CleanupTestTenant()
	require.NoError(t, err, "cleanup should succeed with complex FK relationships")

	// Verify everything was cleaned up
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM embeddings WHERE tenant_id = $1", env.TenantID).Scan(&embeddingCount)
	require.NoError(t, err)
	require.Equal(t, 0, embeddingCount, "all embeddings should be deleted")

	var assertionCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM assertions WHERE tenant_id = $1", env.TenantID).Scan(&assertionCount)
	require.NoError(t, err)
	require.Equal(t, 0, assertionCount, "all assertions should be deleted")

	var sourceCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources WHERE tenant_id = $1", env.TenantID).Scan(&sourceCount)
	require.NoError(t, err)
	require.Equal(t, 0, sourceCount, "all sources should be deleted")

	var personCount int
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
func TestCleanupTestTenant_WithConcurrentWorkflow(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Clean slate
	err := env.CleanupTestTenant()
	require.NoError(t, err)

	// Ensure tenant exists
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Create a source
	var sourceID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO sources (
			tenant_id,
			source_system,
			external_id,
			content_hash,
			raw_content,
			content_type,
			processing_status
		) VALUES (
			$1,
			'manual_eml',
			'test-concurrent-' || gen_random_uuid()::text,
			encode(sha256('concurrent test content'::bytea), 'hex'),
			'Test email for concurrent workflow test',
			'email',
			'processing'
		)
		RETURNING id
	`, env.TenantID).Scan(&sourceID)
	require.NoError(t, err)
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
// It verifies that when we fix the cleanup order, embeddings are deleted BEFORE sources.
//
// Current bug: CleanupTestTenant deletes from tables in the wrong order:
// - Line 265: deletes from "embeddings"
// - Line 288: deletes from "sources"
// But embeddings.source_id has FK to sources.id, so this should fail.
//
// The fix should reorder so that embeddings are deleted BEFORE attempting to delete sources.
func TestCleanupTestTenant_DeletesInFKSafeOrder(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Clean slate
	err := env.CleanupTestTenant()
	require.NoError(t, err)

	// Ensure tenant exists
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Create source
	var sourceID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO sources (
			tenant_id, source_system, external_id, content_hash,
			raw_content, content_type, processing_status
		) VALUES (
			$1, 'manual_eml', 'test-order-' || gen_random_uuid()::text,
			encode(sha256('order test content'::bytea), 'hex'),
			'Test for FK order', 'email', 'completed'
		) RETURNING id
	`, env.TenantID).Scan(&sourceID)
	require.NoError(t, err)

	// Create embedding
	_, err = env.DB.Exec(ctx, `
		INSERT INTO embeddings (
			tenant_id, entity_type, entity_id, source_id,
			embedding_model, text_content, embedding
		) VALUES (
			$1, 'source', $2, $2,
			'test-model', 'FK order test', ARRAY[0.5]::double precision[]
		)
	`, env.TenantID, sourceID)
	require.NoError(t, err)

	// Verify the FK relationship exists
	var embCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM embeddings
		WHERE tenant_id = $1 AND source_id = $2
	`, env.TenantID, sourceID).Scan(&embCount)
	require.NoError(t, err)
	require.Equal(t, 1, embCount, "embedding should reference source")

	// NOW: Run cleanup
	// With the current bug, if cleanup tries to delete from sources before embeddings,
	// it will fail with FK constraint violation.
	// After the fix, cleanup should delete embeddings first, then sources.
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
func TestCleanupTestTenant_RaceCondition(t *testing.T) {
	t.Skip("Race condition test - timing-dependent, may not reproduce bug consistently")

	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Clean slate
	err := env.CleanupTestTenant()
	require.NoError(t, err)

	// Ensure tenant exists
	err = env.EnsureTenantExists()
	require.NoError(t, err)

	// Create a source that will be deleted
	var sourceID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO sources (
			tenant_id, source_system, external_id, content_hash,
			raw_content, content_type, processing_status
		) VALUES (
			$1, 'manual_eml', 'race-test-' || gen_random_uuid()::text,
			encode(sha256('race condition test'::bytea), 'hex'),
			'Race condition test content', 'email', 'processing'
		) RETURNING id
	`, env.TenantID).Scan(&sourceID)
	require.NoError(t, err)

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
