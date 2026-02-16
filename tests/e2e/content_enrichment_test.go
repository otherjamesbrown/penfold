//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Bug: SLM pipeline never creates content_enrichment records
// ============================================================================
//
// The content_enrichment table has 0 rows across ALL tenants. The SLM pipeline
// (SLMPipelineWorkflow) processes emails through 7 stages (triage, extract_ner,
// extract_semantic, resolve, embed, analyze, persist) but never calls the
// enrichment pipeline's Create() method.
//
// This causes downstream failures:
//   - ThreadGrouper does UPDATE content_enrichment SET thread_id = $1 WHERE
//     source_id = $2 — silently affects 0 rows
//   - project_id on content_enrichment is never set (project tagging only
//     flows to assertions.project_id via PersistFindings)
//   - source_system on content_enrichment is never set (only written to
//     sources.ingestion_metadata)
//
// The enrichment pipeline (pkg/enrichment/pipeline/pipeline.go) exists and
// has a Create() method, but the SLM pipeline workflow never invokes it.
//
// Root cause file: services/worker/workflows/pipeline.go
// Enrichment Create: pkg/enrichment/pipeline/pipeline.go:77
// ThreadGrouper UPDATE: services/worker/activities/thread_repo.go:107
//
// Bug shard: pf-XXXXXX
// ============================================================================

// TestE2E_Pipeline_CreatesContentEnrichmentRecord verifies that processing
// an email through the SLM pipeline creates a content_enrichment record.
//
// This test catches the root cause of multiple features silently failing:
// threading, project tagging on content_enrichment, and source_system on
// content_enrichment all depend on this record existing.
func TestE2E_Pipeline_CreatesContentEnrichmentRecord(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-enrichment-%d", time.Now().UnixNano())

	sourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         fmt.Sprintf("<enrichment-%s@e2e.test>", testID),
		Subject:           "E2E Enrichment Record Test",
		FromAddress:       "alice@example.com",
		FromName:          "Alice",
		Body:              "This email should create a content_enrichment record.",
		Date:              time.Now().Add(-1 * time.Hour),
		ExternalID:        fmt.Sprintf("enrichment-%s", testID),
		ParticipantEmails: []string{"alice@example.com"},
	})

	t.Logf("Created test source: %d", sourceID)

	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	t.Run("content_enrichment_record_exists", func(t *testing.T) {
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM content_enrichment
			WHERE source_id = $1 AND tenant_id = $2
		`, sourceID, testTenantID(env)).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count,
			"pipeline should create exactly 1 content_enrichment record per source")
	})

	t.Run("enrichment_has_source_system", func(t *testing.T) {
		var sourceSystem *string
		err := env.DB.QueryRow(ctx, `
			SELECT source_system FROM content_enrichment
			WHERE source_id = $1 AND tenant_id = $2
		`, sourceID, testTenantID(env)).Scan(&sourceSystem)
		if err != nil {
			t.Skipf("content_enrichment record doesn't exist (blocked by root bug): %v", err)
		}
		assert.NotNil(t, sourceSystem,
			"content_enrichment.source_system should be populated after pipeline")
	})

	t.Run("enrichment_has_content_type", func(t *testing.T) {
		var contentType *string
		err := env.DB.QueryRow(ctx, `
			SELECT content_type FROM content_enrichment
			WHERE source_id = $1 AND tenant_id = $2
		`, sourceID, testTenantID(env)).Scan(&contentType)
		if err != nil {
			t.Skipf("content_enrichment record doesn't exist (blocked by root bug): %v", err)
		}
		assert.NotNil(t, contentType,
			"content_enrichment.content_type should be populated after pipeline")
	})
}

// TestE2E_ThreadGrouper_RequiresEnrichmentRecord verifies that the
// ThreadGrouper's UPDATE to content_enrichment.thread_id actually affects
// a row (not silently updating 0 rows on an empty table).
func TestE2E_ThreadGrouper_RequiresEnrichmentRecord(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-tg-enrichment-%d", time.Now().UnixNano())
	baseTime := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)

	rootMsgID := fmt.Sprintf("<tg-root-%s@e2e.test>", testID)
	replyMsgID := fmt.Sprintf("<tg-reply-%s@e2e.test>", testID)

	t.Cleanup(func() {
		env.DB.Exec(ctx, `DELETE FROM thread_messages WHERE thread_id IN (SELECT id FROM email_threads WHERE tenant_id = $1)`, testTenantID(env))
		env.DB.Exec(ctx, `DELETE FROM email_threads WHERE tenant_id = $1`, testTenantID(env))
	})

	rootSourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         rootMsgID,
		Subject:           "E2E ThreadGrouper Enrichment Test",
		FromAddress:       "alice@example.com",
		FromName:          "Alice",
		Body:              "Original message for threading test.",
		Date:              baseTime,
		ExternalID:        fmt.Sprintf("tg-root-%s", testID),
		ParticipantEmails: []string{"alice@example.com", "bob@example.com"},
	})

	replySourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         replyMsgID,
		InReplyTo:         rootMsgID,
		References:        []string{rootMsgID},
		Subject:           "Re: E2E ThreadGrouper Enrichment Test",
		FromAddress:       "bob@example.com",
		FromName:          "Bob",
		Body:              "Reply to test that thread_id gets written.",
		Date:              baseTime.Add(1 * time.Hour),
		ExternalID:        fmt.Sprintf("tg-reply-%s", testID),
		ParticipantEmails: []string{"alice@example.com", "bob@example.com"},
	})

	runPipelineAndWait(t, env, rootSourceID, 120*time.Second)
	runPipelineAndWait(t, env, replySourceID, 120*time.Second)

	// The critical assertion: content_enrichment.thread_id must be set.
	// If the pipeline doesn't create enrichment records, the ThreadGrouper's
	// UPDATE affects 0 rows and thread_id is never stored.
	t.Run("thread_id_written_to_enrichment", func(t *testing.T) {
		for _, sid := range []int64{rootSourceID, replySourceID} {
			var threadID *string
			err := env.DB.QueryRow(ctx, `
				SELECT thread_id FROM content_enrichment
				WHERE source_id = $1 AND tenant_id = $2
			`, sid, testTenantID(env)).Scan(&threadID)

			if err != nil {
				t.Fatalf("content_enrichment row missing for source %d — "+
					"pipeline never created enrichment record, so ThreadGrouper's "+
					"UPDATE silently affected 0 rows: %v", sid, err)
			}

			require.NotNil(t, threadID,
				"thread_id should be set on content_enrichment for source %d "+
					"(ThreadGrouper UPDATE must find an existing row)", sid)
		}
	})
}
