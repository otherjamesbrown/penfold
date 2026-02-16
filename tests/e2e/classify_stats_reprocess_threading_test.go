//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Bug: classify stats reads source_type column instead of source_system
// ============================================================================
//
// `penf classify stats` reports all items as "manual_eml" because the stats
// query aggregates by the sources.source_type column (the MIME/ingest type)
// instead of ingestion_metadata->>'source_system' (the classified type).
//
// Evidence: After `penf classify run --all`, individual items show
// source_system=human_email in metadata, but stats still shows manual_eml:69.
//
// Bug shard: pf-XXXXXX
// ============================================================================

// TestE2E_ClassifyStats_ReadsSourceSystemNotSourceType verifies that
// `penf classify stats` aggregates by the classified source_system field
// (from ingestion_metadata JSONB), not the original source_type column.
//
// This test does NOT require the pipeline — it inserts sources directly
// with known source_system values and checks the stats CLI output.
func TestE2E_ClassifyStats_ReadsSourceSystemNotSourceType(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-stats-field-%d", time.Now().UnixNano())

	// Ensure test tenant exists
	err := env.EnsureTenantExists()
	require.NoError(t, err)

	// Insert sources with source_type='manual_eml' (column) but
	// source_system='human_email' / 'jira' in ingestion_metadata (JSONB).
	// If stats reads the column, it'll show manual_eml.
	// If stats reads the JSONB field, it'll show human_email/jira.
	testSources := []struct {
		externalID   string
		sourceSystem string // goes into ingestion_metadata
	}{
		{fmt.Sprintf("stats-human-1-%s", testID), "human_email"},
		{fmt.Sprintf("stats-human-2-%s", testID), "human_email"},
		{fmt.Sprintf("stats-jira-1-%s", testID), "jira"},
	}

	var insertedIDs []int64
	for _, ts := range testSources {
		meta := map[string]interface{}{
			"source_system": ts.sourceSystem,
			"subject":       "Test email for stats",
			"from_address":  "test@example.com",
		}
		metaJSON, err := json.Marshal(meta)
		require.NoError(t, err)

		var sourceID int64
		err = env.DB.QueryRow(ctx, `
			INSERT INTO sources (
				tenant_id, source_system, external_id, content_hash,
				raw_content, content_type, content_size,
				source_timestamp, ingestion_metadata,
				processing_status, participant_emails
			) VALUES (
				$1, 'manual_eml', $2, encode(sha256($3::bytea), 'hex'),
				$3, 'message/rfc822', length($3),
				NOW(), $4::jsonb,
				'completed', '{}'
			) RETURNING id
		`, env.TenantID, ts.externalID,
			fmt.Sprintf("Test body for %s", ts.externalID),
			metaJSON,
		).Scan(&sourceID)
		require.NoError(t, err, "failed to insert test source %s", ts.externalID)
		insertedIDs = append(insertedIDs, sourceID)
	}

	t.Cleanup(func() {
		for _, id := range insertedIDs {
			env.DB.Exec(ctx, "DELETE FROM sources WHERE id = $1", id)
		}
	})

	t.Logf("Inserted %d test sources with source_system in metadata", len(insertedIDs))

	// Run classify stats and check the breakdown
	t.Run("stats_shows_classified_types_not_source_type", func(t *testing.T) {
		result := env.CLI.Run(ctx, "classify", "stats", "-o", "json")
		require.Equal(t, 0, result.ExitCode,
			"penf classify stats should succeed: %s", result.Stderr)

		var resp map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &resp)
		require.NoError(t, err, "stats should return valid JSON: %s", result.Stdout)

		breakdown, ok := resp["breakdown"].(map[string]interface{})
		require.True(t, ok, "response should contain 'breakdown' map")

		// The classified types should appear in the breakdown
		humanCount, hasHuman := breakdown["human_email"]
		jiraCount, hasJira := breakdown["jira"]

		assert.True(t, hasHuman,
			"breakdown should include 'human_email' (classified type), got keys: %v", mapKeys(breakdown))
		assert.True(t, hasJira,
			"breakdown should include 'jira' (classified type), got keys: %v", mapKeys(breakdown))

		if hasHuman {
			assert.GreaterOrEqual(t, int(humanCount.(float64)), 2,
				"human_email count should be >= 2 (our test sources)")
		}
		if hasJira {
			assert.GreaterOrEqual(t, int(jiraCount.(float64)), 1,
				"jira count should be >= 1 (our test source)")
		}

		// manual_eml should NOT appear as a classified type.
		// It's the source_type column value, not the classification result.
		// If it appears with a count matching total items, the query is wrong.
		if manualCount, hasManual := breakdown["manual_eml"]; hasManual {
			total, _ := resp["total"].(float64)
			assert.NotEqual(t, int(total), int(manualCount.(float64)),
				"manual_eml count should NOT equal total — stats is reading source_type column instead of source_system")
		}
	})
}

// ============================================================================
// Bug: ReprocessContent (classify run) doesn't trigger ThreadGrouper
// ============================================================================
//
// `penf classify run --all` calls the ReprocessContent RPC which runs
// classification but does NOT run the ThreadGrouper activity. Emails
// processed before the threading fix was deployed have empty thread_id
// and there's no way to trigger thread building for existing content.
//
// Evidence: After classify run --all, all 69 emails have source_system
// populated but thread_id remains empty. `penf thread list` returns nothing.
//
// Bug shard: pf-XXXXXX
// ============================================================================

// TestE2E_ReprocessContent_AlsoRunsThreadGrouper verifies that when content
// is reprocessed via `penf classify run`, the ThreadGrouper activity also
// runs — not just classification.
//
// This matters because emails ingested before the threading fix was deployed
// need to be retroactively threaded. If reprocess only runs classification,
// there's no way to populate thread_id for existing content.
func TestE2E_ReprocessContent_AlsoRunsThreadGrouper(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-reprocess-thread-%d", time.Now().UnixNano())
	baseTime := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)

	rootMsgID := fmt.Sprintf("<reprocess-root-%s@e2e.test>", testID)
	replyMsgID := fmt.Sprintf("<reprocess-reply-%s@e2e.test>", testID)

	// Clean up thread tables for this tenant
	t.Cleanup(func() {
		env.DB.Exec(ctx, `DELETE FROM thread_messages WHERE thread_id IN (SELECT id FROM email_threads WHERE tenant_id = $1)`, testTenantID(env))
		env.DB.Exec(ctx, `DELETE FROM email_threads WHERE tenant_id = $1`, testTenantID(env))
	})

	// --- Insert a 2-message thread and process through pipeline ---

	rootSourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         rootMsgID,
		Subject:           "E2E Reprocess Thread Test: Sprint Planning",
		FromAddress:       "alice@example.com",
		FromName:          "Alice",
		Body:              "Team, here are the sprint planning notes.",
		Date:              baseTime,
		ExternalID:        fmt.Sprintf("reprocess-root-%s", testID),
		ParticipantEmails: []string{"alice@example.com", "bob@example.com"},
	})

	replySourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         replyMsgID,
		InReplyTo:         rootMsgID,
		References:        []string{rootMsgID},
		Subject:           "Re: E2E Reprocess Thread Test: Sprint Planning",
		FromAddress:       "bob@example.com",
		FromName:          "Bob",
		Body:              "Thanks Alice, I have some questions about the timeline.",
		Date:              baseTime.Add(3 * time.Hour),
		ExternalID:        fmt.Sprintf("reprocess-reply-%s", testID),
		ParticipantEmails: []string{"alice@example.com", "bob@example.com"},
	})

	t.Logf("Created test sources: root=%d, reply=%d", rootSourceID, replySourceID)

	// Process both through the full pipeline
	runPipelineAndWait(t, env, rootSourceID, 120*time.Second)
	runPipelineAndWait(t, env, replySourceID, 120*time.Second)

	// Get content IDs for CLI operations
	rootContentID := getContentIDForSource(t, env, rootSourceID)
	replyContentID := getContentIDForSource(t, env, replySourceID)
	t.Logf("Content IDs: root=%s, reply=%s", rootContentID, replyContentID)

	// --- Delete thread data to simulate pre-fix state ---
	// This simulates emails that were processed before the threading fix.

	_, err := env.DB.Exec(ctx, `
		DELETE FROM thread_messages
		WHERE thread_id IN (SELECT id FROM email_threads WHERE tenant_id = $1)
	`, testTenantID(env))
	require.NoError(t, err)

	_, err = env.DB.Exec(ctx, `
		DELETE FROM email_threads WHERE tenant_id = $1
	`, testTenantID(env))
	require.NoError(t, err)

	// Clear thread_id from content_enrichment
	_, err = env.DB.Exec(ctx, `
		UPDATE content_enrichment SET thread_id = NULL
		WHERE source_id IN ($1, $2) AND tenant_id = $3
	`, rootSourceID, replySourceID, testTenantID(env))
	require.NoError(t, err)

	// Verify threads are gone
	var threadCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM email_threads WHERE tenant_id = $1
	`, testTenantID(env)).Scan(&threadCount)
	require.NoError(t, err)
	require.Equal(t, 0, threadCount, "threads should be deleted (simulating pre-fix state)")

	t.Log("Cleared thread data — simulating pre-fix state")

	// --- Trigger reprocess via classify run ---
	// This is what users do to reprocess existing content.

	result := env.CLI.Run(ctx, "classify", "run", rootContentID)
	require.Equal(t, 0, result.ExitCode,
		"penf classify run should succeed: %s", result.Stderr)

	result = env.CLI.Run(ctx, "classify", "run", replyContentID)
	require.Equal(t, 0, result.ExitCode,
		"penf classify run should succeed: %s", result.Stderr)

	// Wait for async reprocess jobs to complete
	time.Sleep(30 * time.Second)

	// --- Verify: threads should be recreated by reprocess ---

	t.Run("reprocess_recreates_threads", func(t *testing.T) {
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM email_threads WHERE tenant_id = $1
		`, testTenantID(env)).Scan(&count)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 1,
			"reprocess should have triggered ThreadGrouper and created thread records")
	})

	t.Run("reprocess_populates_thread_id", func(t *testing.T) {
		for _, sid := range []int64{rootSourceID, replySourceID} {
			var threadID *string
			err := env.DB.QueryRow(ctx, `
				SELECT thread_id FROM content_enrichment
				WHERE source_id = $1 AND tenant_id = $2
			`, sid, testTenantID(env)).Scan(&threadID)
			require.NoError(t, err, "content_enrichment should exist for source %d", sid)
			assert.NotNil(t, threadID,
				"thread_id should be populated after reprocess for source %d (ReprocessContent should run ThreadGrouper)", sid)
			if threadID != nil {
				assert.Equal(t, rootMsgID, *threadID,
					"thread_id should be the root message ID for source %d", sid)
			}
		}
	})

	t.Run("cli_thread_list_after_reprocess", func(t *testing.T) {
		result := env.CLI.Run(ctx, "thread", "list", "-o", "json")
		if result.ExitCode == 0 {
			assert.Contains(t, result.Stdout, rootMsgID,
				"thread list should include our test thread after reprocess")
		} else {
			t.Logf("thread list failed (may be expected if no threads created): %s", result.Stderr)
		}
	})
}

// mapKeys returns the keys of a map (for error messages).
func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
