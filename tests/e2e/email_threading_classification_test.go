//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Email Threading & Source Classification E2E Tests
// ============================================================================
//
// These tests verify that the full pipeline — from source insertion through
// Temporal workflow execution to CLI-visible output — correctly:
//
//   1. Groups emails into threads based on In-Reply-To / References headers
//   2. Populates thread_id on content_enrichment
//   3. Exposes threads via `penf thread list` and `penf thread show`
//   4. Classifies source_system using rule-based matching
//   5. Persists source_system to ingestion_metadata
//   6. Exposes source_system via `penf content show -o json`
//
// These are ACCEPTANCE tests owned by Penfold. They exercise the real pipeline
// end-to-end. Mycroft's unit tests cover internal logic with mocks; these tests
// catch integration failures that mocks can't.
//
// ============================================================================

// createEmailSourceCLI ingests an email via CLI (penf ingest email).
// Generates a proper RFC 5322 .eml file and ingests it through the Gateway.
// Returns the source ID (looked up after ingestion).
//
// This replaces the old createEmailSource() which inserted directly into DB
// and bypassed the Gateway (causing CLI/gateway read failures).
func createEmailSourceCLI(t *testing.T, env *PipelineE2EEnv, opts emailSourceOpts) int64 {
	t.Helper()
	ctx := context.Background()

	// Generate RFC 5322 .eml file content
	emlContent := generateEMLContent(opts)

	// Write to temp file
	tmpFile := fmt.Sprintf("/tmp/e2e-email-%s.eml", opts.ExternalID)
	err := os.WriteFile(tmpFile, []byte(emlContent), 0644)
	require.NoError(t, err, "failed to write temp EML file")
	t.Cleanup(func() {
		os.Remove(tmpFile)
	})

	// Ingest via CLI
	result := env.SafeCLI.RunIngest(ctx, "email", tmpFile)
	require.True(t, result.Success(), "ingest email should succeed: %v\nstderr: %s", result.Err, result.Stderr)

	// Extract content ID from CLI output (or look up in DB)
	// The CLI output includes: "Ingested email: <content_id>"
	// For now, look up the source by external_id (Message-ID)
	var sourceID int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM sources
		WHERE tenant_id = $1 AND ingestion_metadata->>'message_id' = $2
		ORDER BY created_at DESC LIMIT 1
	`, testTenantID(env), opts.MessageID).Scan(&sourceID)
	require.NoError(t, err, "failed to look up source ID after CLI ingest")

	return sourceID
}

// generateEMLContent creates RFC 5322 compliant email content from options.
func generateEMLContent(opts emailSourceOpts) string {
	var eml strings.Builder

	// Required headers
	eml.WriteString(fmt.Sprintf("Message-ID: %s\r\n", opts.MessageID))
	eml.WriteString(fmt.Sprintf("From: %s <%s>\r\n", opts.FromName, opts.FromAddress))
	eml.WriteString(fmt.Sprintf("Subject: %s\r\n", opts.Subject))
	eml.WriteString(fmt.Sprintf("Date: %s\r\n", opts.Date.Format(time.RFC1123Z)))

	// Optional headers
	if opts.InReplyTo != "" {
		eml.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", opts.InReplyTo))
	}
	if len(opts.References) > 0 {
		eml.WriteString(fmt.Sprintf("References: %s\r\n", strings.Join(opts.References, " ")))
	}
	if len(opts.To) > 0 {
		toAddrs := make([]string, len(opts.To))
		for i, to := range opts.To {
			if name, ok := to["name"]; ok {
				toAddrs[i] = fmt.Sprintf("%s <%s>", name, to["email"])
			} else {
				toAddrs[i] = to["email"]
			}
		}
		eml.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(toAddrs, ", ")))
	}

	// Content headers
	eml.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	eml.WriteString("MIME-Version: 1.0\r\n")

	// Empty line separates headers from body
	eml.WriteString("\r\n")

	// Body
	eml.WriteString(opts.Body)

	return eml.String()
}

// createEmailSource is the legacy direct-DB-insert version.
// DEPRECATED: Use createEmailSourceCLI instead.
// Kept for reference during migration.
func createEmailSource(t *testing.T, env *PipelineE2EEnv, opts emailSourceOpts) int64 {
	t.Helper()
	// Migration: Replace all calls with createEmailSourceCLI
	return createEmailSourceCLI(t, env, opts)
}

type emailSourceOpts struct {
	MessageID         string
	InReplyTo         string
	References        []string
	Subject           string
	FromAddress       string
	FromName          string
	To                []map[string]string
	Body              string
	Date              time.Time
	ExternalID        string
	ParticipantEmails []string
}

// runPipelineAndWait starts the SLM pipeline for a source and waits for completion.
func runPipelineAndWait(t *testing.T, env *PipelineE2EEnv, sourceID int64, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()

	input := PipelineInput{
		TenantID: testTenantID(env),
		SourceID: sourceID,
		JobID:    fmt.Sprintf("e2e-test-%d-%d", sourceID, time.Now().UnixNano()),
	}

	run, err := env.startPipelineWorkflow(ctx, input)
	require.NoError(t, err, "failed to start pipeline workflow")
	t.Logf("Started pipeline workflow %s for source %d", run.GetID(), sourceID)

	err = env.waitForPipelineComplete(ctx, sourceID, timeout)
	require.NoError(t, err, "pipeline should complete for source %d", sourceID)
}

// getContentIDForSource looks up the content_id for a source.
func getContentIDForSource(t *testing.T, env *PipelineE2EEnv, sourceID int64) string {
	t.Helper()
	ctx := context.Background()
	var contentID string
	err := env.DB.QueryRow(ctx, `
		SELECT content_id FROM sources WHERE id = $1
	`, sourceID).Scan(&contentID)
	require.NoError(t, err, "should find content_id for source %d", sourceID)
	return contentID
}

// ============================================================================
// Email Threading Tests
// ============================================================================

// TestE2E_EmailThreading_PipelinePopulatesThreads verifies that processing
// emails with In-Reply-To headers through the real pipeline creates thread
// records and populates thread_id on content_enrichment.
//
// This test exists because threading was silently skipped for all emails:
// source_repo.GetSource() returned content_type="message/rfc822" (MIME type)
// but the ThreadGrouper checked for content_type="email" (logical type).
// Mycroft's unit tests passed because the mock returned "email".
func TestE2E_EmailThreading_PipelinePopulatesThreads(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-thread-%d", time.Now().UnixNano())
	baseTime := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)

	// Clean up thread tables for this tenant
	t.Cleanup(func() {
		env.DB.Exec(ctx, `DELETE FROM thread_messages WHERE thread_id IN (SELECT id FROM email_threads WHERE tenant_id = $1)`, testTenantID(env))
		env.DB.Exec(ctx, `DELETE FROM email_threads WHERE tenant_id = $1`, testTenantID(env))
	})

	// --- Insert a 2-message thread ---

	rootMsgID := fmt.Sprintf("<root-%s@e2e.test>", testID)
	replyMsgID := fmt.Sprintf("<reply-%s@e2e.test>", testID)

	rootSourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         rootMsgID,
		Subject:           "E2E Thread Test: Project Update",
		FromAddress:       "alice@example.com",
		FromName:          "Alice",
		Body:              "Hi team, here is the project update for this week.",
		Date:              baseTime,
		ExternalID:        fmt.Sprintf("root-%s", testID),
		ParticipantEmails: []string{"alice@example.com", "bob@example.com"},
	})

	replySourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         replyMsgID,
		InReplyTo:         rootMsgID,
		References:        []string{rootMsgID},
		Subject:           "Re: E2E Thread Test: Project Update",
		FromAddress:       "bob@example.com",
		FromName:          "Bob",
		Body:              "Thanks Alice, looks good. I will review the timeline.",
		Date:              baseTime.Add(2 * time.Hour),
		ExternalID:        fmt.Sprintf("reply-%s", testID),
		ParticipantEmails: []string{"alice@example.com", "bob@example.com"},
	})

	t.Logf("Created test sources: root=%d, reply=%d", rootSourceID, replySourceID)

	// --- Run both through the real pipeline ---

	runPipelineAndWait(t, env, rootSourceID, 120*time.Second)
	runPipelineAndWait(t, env, replySourceID, 120*time.Second)

	// --- Verify DB state: email_threads table ---

	t.Run("thread_exists_in_db", func(t *testing.T) {
		var threadID int64
		var rootMessageID, subject string
		var messageCount int
		err := env.DB.QueryRow(ctx, `
			SELECT id, root_message_id, subject, message_count
			FROM email_threads
			WHERE tenant_id = $1 AND root_message_id = $2
		`, testTenantID(env), rootMsgID).Scan(&threadID, &rootMessageID, &subject, &messageCount)
		require.NoError(t, err, "thread should exist in email_threads after pipeline processing")
		assert.Equal(t, rootMsgID, rootMessageID)
		assert.Equal(t, "E2E Thread Test: Project Update", subject, "subject should be normalized (no Re:)")
		assert.Equal(t, 2, messageCount, "thread should have 2 messages")
		t.Logf("Thread created: id=%d, root=%s, count=%d", threadID, rootMessageID, messageCount)
	})

	// --- Verify DB state: thread_messages ---

	t.Run("thread_messages_correct", func(t *testing.T) {
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM thread_messages tm
			JOIN email_threads et ON tm.thread_id = et.id
			WHERE et.tenant_id = $1 AND et.root_message_id = $2
		`, testTenantID(env), rootMsgID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 2, count, "should have 2 thread_messages entries")
	})

	// --- Verify DB state: content_enrichment.thread_id ---

	t.Run("thread_id_in_content_enrichment", func(t *testing.T) {
		for _, sid := range []int64{rootSourceID, replySourceID} {
			var threadID *string
			err := env.DB.QueryRow(ctx, `
				SELECT thread_id FROM content_enrichment
				WHERE source_id = $1 AND tenant_id = $2
			`, sid, testTenantID(env)).Scan(&threadID)
			require.NoError(t, err, "content_enrichment should exist for source %d", sid)
			require.NotNil(t, threadID, "thread_id should be set for source %d", sid)
			assert.Equal(t, rootMsgID, *threadID,
				"thread_id should be the root message ID for source %d", sid)
		}
	})

	// --- Verify CLI output: penf thread list ---

	t.Run("cli_thread_list_shows_thread", func(t *testing.T) {
		result := env.SafeCLI.Run(ctx, "thread", "list", "-o", "json")
		if result.ExitCode != 0 {
			t.Fatalf("penf thread list failed: %s", result.Stderr)
		}

		// Parse JSON and check for our thread
		var listResp map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &listResp)
		require.NoError(t, err, "thread list should return valid JSON")

		threads, ok := listResp["threads"].([]interface{})
		require.True(t, ok, "response should have 'threads' array")

		found := false
		for _, th := range threads {
			thread := th.(map[string]interface{})
			if rmid, ok := thread["root_message_id"].(string); ok && rmid == rootMsgID {
				found = true
				assert.Equal(t, float64(2), thread["message_count"],
					"CLI should show 2 messages in thread")
				break
			}
		}
		assert.True(t, found, "penf thread list should include our test thread")
	})

	// --- Verify CLI output: penf content show ---

	t.Run("cli_content_show_has_thread_id", func(t *testing.T) {
		contentID := getContentIDForSource(t, env, replySourceID)

		result := env.SafeCLI.Run(ctx, "content", "show", contentID, "-o", "json")
		require.Equal(t, 0, result.ExitCode, "content show should succeed: %s", result.Stderr)

		var showResp map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &showResp)
		require.NoError(t, err)

		item := showResp["item"].(map[string]interface{})
		metadata := item["metadata"].(map[string]interface{})

		threadID, ok := metadata["thread_id"].(string)
		require.True(t, ok, "metadata should contain thread_id")
		assert.NotEmpty(t, threadID, "thread_id should not be empty")
		assert.Equal(t, rootMsgID, threadID,
			"thread_id in CLI output should match root message ID")
	})
}

// TestE2E_EmailThreading_OrphanReply verifies that an email replying to a
// message not in our dataset still creates a thread record.
func TestE2E_EmailThreading_OrphanReply(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-orphan-%d", time.Now().UnixNano())
	externalRootID := fmt.Sprintf("<external-root-%s@unknown.com>", testID)
	orphanMsgID := fmt.Sprintf("<orphan-%s@e2e.test>", testID)

	t.Cleanup(func() {
		env.DB.Exec(ctx, `DELETE FROM thread_messages WHERE thread_id IN (SELECT id FROM email_threads WHERE tenant_id = $1)`, testTenantID(env))
		env.DB.Exec(ctx, `DELETE FROM email_threads WHERE tenant_id = $1`, testTenantID(env))
	})

	orphanSourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         orphanMsgID,
		InReplyTo:         externalRootID,
		References:        []string{externalRootID},
		Subject:           "Re: External Discussion About Router Config",
		FromAddress:       "grace@example.com",
		FromName:          "Grace",
		Body:              "I have updated the router configuration as discussed.",
		Date:              time.Date(2026, 2, 15, 14, 0, 0, 0, time.UTC),
		ExternalID:        fmt.Sprintf("orphan-%s", testID),
		ParticipantEmails: []string{"grace@example.com"},
	})

	runPipelineAndWait(t, env, orphanSourceID, 120*time.Second)

	t.Run("orphan_creates_thread_with_external_root", func(t *testing.T) {
		var messageCount int
		var subject string
		err := env.DB.QueryRow(ctx, `
			SELECT message_count, subject
			FROM email_threads
			WHERE tenant_id = $1 AND root_message_id = $2
		`, testTenantID(env), externalRootID).Scan(&messageCount, &subject)
		require.NoError(t, err, "orphan thread should exist with external root message ID")
		assert.Equal(t, 1, messageCount, "orphan thread should have 1 known message")
		assert.Equal(t, "External Discussion About Router Config", subject,
			"subject should be normalized (no Re:)")
	})

	t.Run("orphan_thread_id_in_enrichment", func(t *testing.T) {
		var threadID *string
		err := env.DB.QueryRow(ctx, `
			SELECT thread_id FROM content_enrichment
			WHERE source_id = $1 AND tenant_id = $2
		`, orphanSourceID, testTenantID(env)).Scan(&threadID)
		require.NoError(t, err)
		require.NotNil(t, threadID)
		assert.Equal(t, externalRootID, *threadID,
			"orphan's thread_id should reference external root")
	})
}

// ============================================================================
// Source Classification Tests
// ============================================================================

// TestE2E_SourceClassification_PipelinePersistsSourceSystem verifies that the
// pipeline classifies emails by source system and persists the result to
// ingestion_metadata, visible via `penf content show -o json`.
//
// This test exists because the classification code computed source_system
// correctly but UpdateContentStatus never wrote it to the DB — the JSONB merge
// SQL included triage_category, skip_deep, etc. but omitted source_system.
func TestE2E_SourceClassification_PipelinePersistsSourceSystem(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-classify-%d", time.Now().UnixNano())

	// Test cases: emails that should match specific classification rules
	testCases := []struct {
		name           string
		fromAddress    string
		subject        string
		expectedSystem string
	}{
		{
			name:           "human_email",
			fromAddress:    "colleague@example.com",
			subject:        "Project status update",
			expectedSystem: "human_email",
		},
		{
			name:           "auto_reply",
			fromAddress:    "colleague@example.com",
			subject:        "Automatic reply: Out of office until Monday",
			expectedSystem: "auto_reply",
		},
		{
			name:           "jira_notification",
			fromAddress:    "jira@atlassian.net",
			subject:        "[TRACK-JIRA] PROJ-123 updated",
			expectedSystem: "jira",
		},
		{
			name:           "webex_invite",
			fromAddress:    "messenger@webex.com",
			subject:        "Team standup meeting",
			expectedSystem: "webex",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sourceID := createEmailSource(t, env, emailSourceOpts{
				MessageID:         fmt.Sprintf("<classify-%s-%s@e2e.test>", tc.name, testID),
				Subject:           tc.subject,
				FromAddress:       tc.fromAddress,
				FromName:          "Test Sender",
				Body:              fmt.Sprintf("Test email body for %s classification.", tc.name),
				Date:              time.Now().Add(-1 * time.Hour),
				ExternalID:        fmt.Sprintf("classify-%s-%s", tc.name, testID),
				ParticipantEmails: []string{tc.fromAddress},
			})

			runPipelineAndWait(t, env, sourceID, 120*time.Second)

			// Verify DB: source_system in ingestion_metadata
			t.Run("db_has_source_system", func(t *testing.T) {
				var ingestionMeta map[string]interface{}
				var metaJSON []byte
				err := env.DB.QueryRow(ctx, `
					SELECT ingestion_metadata
					FROM sources
					WHERE id = $1 AND tenant_id = $2
				`, sourceID, testTenantID(env)).Scan(&metaJSON)
				require.NoError(t, err)
				require.NotNil(t, metaJSON)

				err = json.Unmarshal(metaJSON, &ingestionMeta)
				require.NoError(t, err)

				sourceSystem, ok := ingestionMeta["source_system"].(string)
				require.True(t, ok, "ingestion_metadata should contain source_system")
				assert.Equal(t, tc.expectedSystem, sourceSystem,
					"source_system should be %q for from=%q subject=%q",
					tc.expectedSystem, tc.fromAddress, tc.subject)
			})

			// Verify CLI: penf content show exposes source_system
			t.Run("cli_shows_source_system", func(t *testing.T) {
				contentID := getContentIDForSource(t, env, sourceID)

				result := env.SafeCLI.Run(ctx, "content", "show", contentID, "-o", "json")
				require.Equal(t, 0, result.ExitCode, "content show should succeed: %s", result.Stderr)

				var showResp map[string]interface{}
				err := json.Unmarshal([]byte(result.Stdout), &showResp)
				require.NoError(t, err)

				item := showResp["item"].(map[string]interface{})
				metadata := item["metadata"].(map[string]interface{})

				sourceSystem, ok := metadata["source_system"].(string)
				require.True(t, ok,
					"CLI metadata should contain source_system (got keys: %v)",
					metadataKeys(metadata))
				assert.Equal(t, tc.expectedSystem, sourceSystem,
					"CLI source_system should be %q", tc.expectedSystem)
			})
		})
	}
}

// TestE2E_SourceClassification_CalendarCancellation verifies calendar emails
// are classified correctly (edge case: subject prefix matching).
func TestE2E_SourceClassification_CalendarCancellation(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-cancel-%d", time.Now().UnixNano())

	sourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         fmt.Sprintf("<cancel-%s@e2e.test>", testID),
		Subject:           "Canceled: Weekly Team Sync",
		FromAddress:       "calendar@example.com",
		FromName:          "Calendar",
		Body:              "This meeting has been canceled.",
		Date:              time.Now().Add(-1 * time.Hour),
		ExternalID:        fmt.Sprintf("cancel-%s", testID),
		ParticipantEmails: []string{"calendar@example.com"},
	})

	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	var metaJSON []byte
	err := env.DB.QueryRow(ctx, `
		SELECT ingestion_metadata FROM sources WHERE id = $1
	`, sourceID).Scan(&metaJSON)
	require.NoError(t, err)

	var meta map[string]interface{}
	err = json.Unmarshal(metaJSON, &meta)
	require.NoError(t, err)

	sourceSystem, ok := meta["source_system"].(string)
	require.True(t, ok, "should have source_system in ingestion_metadata")
	assert.Equal(t, "outlook_calendar", sourceSystem,
		"'Canceled:' prefix should classify as outlook_calendar")
}

// ============================================================================
// Helpers
// ============================================================================

// metadataKeys returns sorted keys from a metadata map (for error messages).
func metadataKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// testTenantID returns the test tenant ID from the PipelineE2EEnv.
func testTenantID(env *PipelineE2EEnv) string {
	if env.E2EEnv != nil {
		return env.E2EEnv.TenantID
	}
	return TestTenantID
}

// cliContentShowMetadata runs `penf content show <id> -o json` and returns the metadata map.
func cliContentShowMetadata(t *testing.T, env *PipelineE2EEnv, contentID string) map[string]interface{} {
	t.Helper()
	ctx := context.Background()

	result := env.SafeCLI.Run(ctx, "content", "show", contentID, "-o", "json")
	require.Equal(t, 0, result.ExitCode, "content show failed: %s", result.Stderr)

	var resp map[string]interface{}
	err := json.Unmarshal([]byte(result.Stdout), &resp)
	require.NoError(t, err)

	item := resp["item"].(map[string]interface{})
	metadata, ok := item["metadata"].(map[string]interface{})
	require.True(t, ok, "item should have metadata")
	return metadata
}

// threadListContains checks if `penf thread list -o json` contains a thread
// with the given root_message_id.
func threadListContains(t *testing.T, env *PipelineE2EEnv, rootMessageID string) bool {
	t.Helper()
	ctx := context.Background()

	result := env.SafeCLI.Run(ctx, "thread", "list", "-o", "json")
	if result.ExitCode != 0 {
		return false
	}

	return strings.Contains(result.Stdout, rootMessageID)
}
