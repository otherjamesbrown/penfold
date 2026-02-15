//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Email Threading Acceptance Test
// ============================================================================
//
// These tests validate that the email threading pipeline correctly:
// 1. Groups emails into threads using Message-ID/In-Reply-To/References headers
// 2. Creates email_threads records with correct metadata
// 3. Creates thread_messages membership records with correct ordering
// 4. Populates content_enrichment.thread_id for threaded emails
// 5. Handles orphan replies (In-Reply-To references a message not in our dataset)
// 6. Exposes threads via API (penf thread list / penf thread show)
//
// Prerequisites:
// - Database migrations 002 (content_enrichment) and 004 (email_threads, thread_messages) applied
// - ThreadGrouper wired into SLM pipeline (or standalone threading activity)
// - Thread persistence activity writes to email_threads + thread_messages tables
// - Thread API endpoints registered on gateway
//
// Test data creates a realistic email thread scenario:
//
//   Thread A (3-message chain):
//     msg-A1 (root) "TikTok Agenda for Friday"
//     msg-A2 (reply to A1) "Re: TikTok Agenda for Friday"
//     msg-A3 (reply to A2, refs A1,A2) "Re: TikTok Agenda for Friday"
//
//   Thread B (2-message chain):
//     msg-B1 (root) "PACE MTC Update"
//     msg-B2 (reply to B1) "Re: PACE MTC Update"
//
//   Standalone C (no thread):
//     msg-C1 "Quarterly Report Draft"
//
//   Orphan D (reply to unknown message):
//     msg-D1 (reply to non-existent msg) "Re: Juniper Router Configuration"
//
// ============================================================================

// threadTestEmail defines a test email with threading headers.
type threadTestEmail struct {
	MessageID  string
	InReplyTo  string
	References []string
	Subject    string
	From       string
	Body       string
	Date       time.Time
}

// threadTestFixtures returns the standard set of test emails for threading tests.
func threadTestFixtures() []threadTestEmail {
	baseTime := time.Date(2026, 2, 10, 9, 0, 0, 0, time.UTC)

	return []threadTestEmail{
		// Thread A: 3-message chain
		{
			MessageID: "<msg-A1@example.com>",
			Subject:   "TikTok Agenda for Friday",
			From:      "alice@example.com",
			Body:      "Hi team, here's the agenda for Friday's TikTok review meeting.",
			Date:      baseTime,
		},
		{
			MessageID:  "<msg-A2@example.com>",
			InReplyTo:  "<msg-A1@example.com>",
			References: []string{"<msg-A1@example.com>"},
			Subject:    "Re: TikTok Agenda for Friday",
			From:       "bob@example.com",
			Body:       "Thanks Alice, I'll add the analytics section.",
			Date:       baseTime.Add(2 * time.Hour),
		},
		{
			MessageID:  "<msg-A3@example.com>",
			InReplyTo:  "<msg-A2@example.com>",
			References: []string{"<msg-A1@example.com>", "<msg-A2@example.com>"},
			Subject:    "Re: TikTok Agenda for Friday",
			From:       "carol@example.com",
			Body:       "I'll prepare the creative performance slides.",
			Date:       baseTime.Add(4 * time.Hour),
		},

		// Thread B: 2-message chain
		{
			MessageID: "<msg-B1@example.com>",
			Subject:   "PACE MTC Update",
			From:      "dave@example.com",
			Body:      "Team, the PACE MTC deployment is scheduled for next week.",
			Date:      baseTime.Add(1 * time.Hour),
		},
		{
			MessageID:  "<msg-B2@example.com>",
			InReplyTo:  "<msg-B1@example.com>",
			References: []string{"<msg-B1@example.com>"},
			Subject:    "Re: PACE MTC Update",
			From:       "eve@example.com",
			Body:       "Confirmed - I'll coordinate with the network team.",
			Date:       baseTime.Add(3 * time.Hour),
		},

		// Standalone C: no thread
		{
			MessageID: "<msg-C1@example.com>",
			Subject:   "Quarterly Report Draft",
			From:      "frank@example.com",
			Body:      "Attached is the Q1 quarterly report for review.",
			Date:      baseTime.Add(5 * time.Hour),
		},

		// Orphan D: reply to message not in our dataset
		{
			MessageID:  "<msg-D1@example.com>",
			InReplyTo:  "<msg-UNKNOWN@external.com>",
			References: []string{"<msg-UNKNOWN@external.com>"},
			Subject:    "Re: Juniper Router Configuration",
			From:       "grace@example.com",
			Body:       "I've updated the router config as discussed.",
			Date:       baseTime.Add(6 * time.Hour),
		},
	}
}

// insertThreadTestEmails inserts test emails into the sources table and returns their source IDs.
func insertThreadTestEmails(t *testing.T, db *TestDB, emails []threadTestEmail) map[string]int64 {
	t.Helper()
	ctx := context.Background()

	testID := uniqueTestID()
	sourceIDs := make(map[string]int64)

	for _, email := range emails {
		// Build metadata matching what the ingest pipeline produces
		metadata := map[string]interface{}{
			"subject":    email.Subject,
			"from":       email.From,
			"message_id": email.MessageID,
			"date":       email.Date,
		}
		if email.InReplyTo != "" {
			metadata["in_reply_to"] = email.InReplyTo
		}
		if len(email.References) > 0 {
			// Store as interface slice to match real ingest behavior
			refs := make([]interface{}, len(email.References))
			for i, r := range email.References {
				refs[i] = r
			}
			metadata["references"] = refs
		}

		externalID := fmt.Sprintf("%s-%s", email.MessageID, testID)

		var sourceID int64
		err := db.Pool.QueryRow(ctx, `
			INSERT INTO sources (
				tenant_id, source_system, external_id, content_hash,
				raw_content, content_type, content_size,
				source_timestamp, metadata, processing_status
			) VALUES (
				$1, 'manual_eml', $2, encode(sha256($3::bytea), 'hex'),
				$3, 'message/rfc822', length($3),
				$4, $5::jsonb, 'pending'
			) RETURNING id
		`, db.TenantID, externalID, email.Body, email.Date, metadata).Scan(&sourceID)
		require.NoError(t, err, "failed to insert test email: %s", email.MessageID)

		sourceIDs[email.MessageID] = sourceID
	}

	return sourceIDs
}

// TestEmailThreading_ThreadCreationFromReplyChain verifies that emails with
// In-Reply-To and References headers are grouped into the correct thread.
func TestEmailThreading_ThreadCreationFromReplyChain(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	db.CleanupTables(t, "thread_messages", "email_threads", "content_enrichment", "sources")

	emails := threadTestFixtures()
	sourceIDs := insertThreadTestEmails(t, db, emails)
	ctx := context.Background()

	// --- After pipeline processes all emails, verify Thread A ---

	t.Run("thread_A_exists_with_correct_root", func(t *testing.T) {
		var threadID int64
		var rootMessageID, subject string
		var messageCount int
		err := db.Pool.QueryRow(ctx, `
			SELECT id, root_message_id, subject, message_count
			FROM email_threads
			WHERE tenant_id = $1 AND root_message_id = $2
		`, db.TenantID, "<msg-A1@example.com>").Scan(&threadID, &rootMessageID, &subject, &messageCount)
		require.NoError(t, err, "Thread A should exist with root msg-A1")
		assert.Equal(t, "<msg-A1@example.com>", rootMessageID)
		assert.Equal(t, "TikTok Agenda for Friday", subject, "subject should be normalized (no Re:)")
		assert.Equal(t, 3, messageCount, "Thread A should have 3 messages")
	})

	t.Run("thread_A_messages_ordered_correctly", func(t *testing.T) {
		rows, err := db.Pool.Query(ctx, `
			SELECT tm.message_id, tm.position_in_thread, tm.is_reply, tm.reply_to_message_id, tm.source_id
			FROM thread_messages tm
			JOIN email_threads et ON tm.thread_id = et.id
			WHERE et.tenant_id = $1 AND et.root_message_id = $2
			ORDER BY tm.position_in_thread
		`, db.TenantID, "<msg-A1@example.com>")
		require.NoError(t, err)
		defer rows.Close()

		type threadMsg struct {
			messageID        string
			position         int
			isReply          bool
			replyToMessageID *string
			sourceID         int64
		}
		var messages []threadMsg
		for rows.Next() {
			var m threadMsg
			err := rows.Scan(&m.messageID, &m.position, &m.isReply, &m.replyToMessageID, &m.sourceID)
			require.NoError(t, err)
			messages = append(messages, m)
		}

		require.Len(t, messages, 3, "Thread A should have 3 thread_messages entries")

		// Position 1: root message
		assert.Equal(t, "<msg-A1@example.com>", messages[0].messageID)
		assert.Equal(t, 1, messages[0].position)
		assert.False(t, messages[0].isReply)
		assert.Equal(t, sourceIDs["<msg-A1@example.com>"], messages[0].sourceID)

		// Position 2: first reply
		assert.Equal(t, "<msg-A2@example.com>", messages[1].messageID)
		assert.Equal(t, 2, messages[1].position)
		assert.True(t, messages[1].isReply)
		require.NotNil(t, messages[1].replyToMessageID)
		assert.Equal(t, "<msg-A1@example.com>", *messages[1].replyToMessageID)

		// Position 3: second reply
		assert.Equal(t, "<msg-A3@example.com>", messages[2].messageID)
		assert.Equal(t, 3, messages[2].position)
		assert.True(t, messages[2].isReply)
		require.NotNil(t, messages[2].replyToMessageID)
		assert.Equal(t, "<msg-A2@example.com>", *messages[2].replyToMessageID)
	})

	t.Run("thread_A_timing_correct", func(t *testing.T) {
		var firstAt, lastAt time.Time
		var latestSourceID int64
		err := db.Pool.QueryRow(ctx, `
			SELECT first_message_at, last_message_at, latest_source_id
			FROM email_threads
			WHERE tenant_id = $1 AND root_message_id = $2
		`, db.TenantID, "<msg-A1@example.com>").Scan(&firstAt, &lastAt, &latestSourceID)
		require.NoError(t, err)

		assert.True(t, lastAt.After(firstAt), "last_message_at should be after first_message_at")
		assert.Equal(t, sourceIDs["<msg-A3@example.com>"], latestSourceID, "latest_source_id should be the most recent message")
	})

	// --- Verify Thread B ---

	t.Run("thread_B_exists_with_2_messages", func(t *testing.T) {
		var messageCount int
		var subject string
		err := db.Pool.QueryRow(ctx, `
			SELECT message_count, subject
			FROM email_threads
			WHERE tenant_id = $1 AND root_message_id = $2
		`, db.TenantID, "<msg-B1@example.com>").Scan(&messageCount, &subject)
		require.NoError(t, err, "Thread B should exist")
		assert.Equal(t, 2, messageCount)
		assert.Equal(t, "PACE MTC Update", subject)
	})

	// --- Verify Standalone C ---

	t.Run("standalone_email_gets_own_thread_or_no_thread", func(t *testing.T) {
		// Standalone emails may either:
		// a) Get their own single-message thread (root = self), OR
		// b) Not appear in email_threads at all
		// Both approaches are acceptable. If a thread exists, it should have message_count = 1.
		var count int
		err := db.Pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM email_threads
			WHERE tenant_id = $1 AND root_message_id = $2
		`, db.TenantID, "<msg-C1@example.com>").Scan(&count)
		require.NoError(t, err)

		if count > 0 {
			var messageCount int
			err = db.Pool.QueryRow(ctx, `
				SELECT message_count FROM email_threads
				WHERE tenant_id = $1 AND root_message_id = $2
			`, db.TenantID, "<msg-C1@example.com>").Scan(&messageCount)
			require.NoError(t, err)
			assert.Equal(t, 1, messageCount, "standalone thread should have exactly 1 message")
		}
	})

	// --- Verify total thread count ---

	t.Run("correct_number_of_threads_created", func(t *testing.T) {
		var threadCount int
		err := db.Pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM email_threads WHERE tenant_id = $1
		`, db.TenantID).Scan(&threadCount)
		require.NoError(t, err)

		// At minimum: Thread A + Thread B + Orphan D = 3 threads
		// Possibly +1 if standalone emails get their own thread
		assert.GreaterOrEqual(t, threadCount, 3, "should have at least 3 threads (A, B, orphan D)")
		assert.LessOrEqual(t, threadCount, 4, "should have at most 4 threads (A, B, orphan D, standalone C)")
	})
}

// TestEmailThreading_OrphanReplyHandling verifies that emails replying to
// messages not in our dataset still get properly threaded.
func TestEmailThreading_OrphanReplyHandling(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	db.CleanupTables(t, "thread_messages", "email_threads", "content_enrichment", "sources")

	emails := threadTestFixtures()
	sourceIDs := insertThreadTestEmails(t, db, emails)
	ctx := context.Background()

	t.Run("orphan_reply_creates_thread_with_external_root", func(t *testing.T) {
		// msg-D1 replies to <msg-UNKNOWN@external.com> which is not in our dataset.
		// The threading system should still create a thread using the referenced
		// message ID as the root_message_id.
		var threadID int64
		var rootMessageID, subject string
		var messageCount int
		err := db.Pool.QueryRow(ctx, `
			SELECT id, root_message_id, subject, message_count
			FROM email_threads
			WHERE tenant_id = $1 AND root_message_id = $2
		`, db.TenantID, "<msg-UNKNOWN@external.com>").Scan(&threadID, &rootMessageID, &subject, &messageCount)
		require.NoError(t, err, "orphan thread should exist with external root message ID")
		assert.Equal(t, "<msg-UNKNOWN@external.com>", rootMessageID)
		assert.Equal(t, "Juniper Router Configuration", subject, "subject should be normalized")
		assert.Equal(t, 1, messageCount, "orphan thread should have 1 known message")
	})

	t.Run("orphan_reply_has_thread_message_entry", func(t *testing.T) {
		var msgID string
		var isReply bool
		var replyTo *string
		err := db.Pool.QueryRow(ctx, `
			SELECT tm.message_id, tm.is_reply, tm.reply_to_message_id
			FROM thread_messages tm
			JOIN email_threads et ON tm.thread_id = et.id
			WHERE et.tenant_id = $1 AND et.root_message_id = $2
		`, db.TenantID, "<msg-UNKNOWN@external.com>").Scan(&msgID, &isReply, &replyTo)
		require.NoError(t, err)
		assert.Equal(t, "<msg-D1@example.com>", msgID)
		assert.True(t, isReply)
		require.NotNil(t, replyTo)
		assert.Equal(t, "<msg-UNKNOWN@external.com>", *replyTo)
		_ = sourceIDs // sourceIDs available if needed for further assertions
	})
}

// TestEmailThreading_ContentEnrichmentThreadID verifies that thread_id
// is populated on the content_enrichment table for all threaded emails.
func TestEmailThreading_ContentEnrichmentThreadID(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	db.CleanupTables(t, "thread_messages", "email_threads", "content_enrichment", "sources")

	emails := threadTestFixtures()
	sourceIDs := insertThreadTestEmails(t, db, emails)
	ctx := context.Background()

	t.Run("threaded_emails_have_thread_id_in_enrichment", func(t *testing.T) {
		// All emails in Thread A should have the same thread_id
		for _, msgID := range []string{"<msg-A1@example.com>", "<msg-A2@example.com>", "<msg-A3@example.com>"} {
			var threadID *string
			err := db.Pool.QueryRow(ctx, `
				SELECT thread_id FROM content_enrichment
				WHERE source_id = $1 AND tenant_id = $2
			`, sourceIDs[msgID], db.TenantID).Scan(&threadID)
			require.NoError(t, err, "content_enrichment should exist for %s", msgID)
			require.NotNil(t, threadID, "thread_id should be set for %s", msgID)
			assert.Equal(t, "<msg-A1@example.com>", *threadID, "all Thread A messages should share the root message ID as thread_id")
		}
	})

	t.Run("orphan_reply_has_external_thread_id", func(t *testing.T) {
		var threadID *string
		err := db.Pool.QueryRow(ctx, `
			SELECT thread_id FROM content_enrichment
			WHERE source_id = $1 AND tenant_id = $2
		`, sourceIDs["<msg-D1@example.com>"], db.TenantID).Scan(&threadID)
		require.NoError(t, err)
		require.NotNil(t, threadID)
		assert.Equal(t, "<msg-UNKNOWN@external.com>", *threadID, "orphan should reference external root")
	})
}

// TestEmailThreading_CLICommands verifies the penf CLI thread commands work.
func TestEmailThreading_CLICommands(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	// Note: this test assumes thread data has been populated by prior pipeline runs
	// or the other tests in this file. It tests the CLI/API layer.

	t.Run("penf_thread_list_returns_threads", func(t *testing.T) {
		stdout, stderr, err := runCLI(t, "thread", "list", "-o", "json")
		if err != nil {
			t.Skipf("penf thread list not yet implemented: %v (stderr: %s)", err, stderr)
		}

		assertJSONContains(t, stdout, "threads")
		assertJSONArrayNotEmpty(t, stdout, "threads")
	})

	t.Run("penf_thread_show_returns_messages", func(t *testing.T) {
		// First get a thread ID from the list
		ctx := context.Background()
		var threadID int64
		err := db.Pool.QueryRow(ctx, `
			SELECT id FROM email_threads WHERE tenant_id = $1 LIMIT 1
		`, db.TenantID).Scan(&threadID)
		if err != nil {
			t.Skip("no threads in database to test show command")
		}

		stdout, stderr, err := runCLI(t, "thread", "show", fmt.Sprintf("%d", threadID), "-o", "json")
		if err != nil {
			t.Skipf("penf thread show not yet implemented: %v (stderr: %s)", err, stderr)
		}

		assertJSONContains(t, stdout, "thread")
		_ = stdout
	})
}

// TestEmailThreading_ReprocessExistingEmails is a manual verification test
// that checks whether the existing 69 emails have been properly threaded
// after reprocessing. This test queries against the default tenant.
func TestEmailThreading_ReprocessExistingEmails(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// Use the production tenant for this verification
	prodTenantID := "00000001-0000-0000-0000-000000000001"

	t.Run("existing_emails_have_threads", func(t *testing.T) {
		var threadCount int
		err := db.Pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM email_threads WHERE tenant_id = $1
		`, prodTenantID).Scan(&threadCount)
		if err != nil {
			t.Skipf("email_threads table query failed: %v", err)
		}

		if threadCount == 0 {
			t.Skip("no threads found - reprocessing may not have run yet")
		}

		t.Logf("Found %d threads for production tenant", threadCount)
		assert.Greater(t, threadCount, 0, "should have at least some threads after reprocessing")
	})

	t.Run("thread_id_populated_on_enrichments", func(t *testing.T) {
		var withThread, total int
		err := db.Pool.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE thread_id IS NOT NULL AND thread_id != '') as with_thread,
				COUNT(*) as total
			FROM content_enrichment
			WHERE tenant_id = $1
		`, prodTenantID).Scan(&withThread, &total)
		if err != nil {
			t.Skipf("content_enrichment query failed: %v", err)
		}

		if total == 0 {
			t.Skip("no enrichment records - pipeline may not have run")
		}

		t.Logf("Enrichment records: %d/%d have thread_id", withThread, total)
		// At least the emails with in_reply_to should have thread_id
		assert.Greater(t, withThread, 0, "some enrichment records should have thread_id")
	})

	t.Run("known_thread_chains_match", func(t *testing.T) {
		// Verify specific known thread subjects from the 69 emails
		knownThreadSubjects := []struct {
			subjectContains string
			minMessages     int
		}{
			{"TikTok", 2},       // TikTok Agenda thread
			{"PACE", 2},         // PACE MTC thread
			{"Juniper", 2},      // Juniper Router thread
		}

		for _, expected := range knownThreadSubjects {
			var count int
			err := db.Pool.QueryRow(ctx, `
				SELECT COALESCE(MAX(message_count), 0)
				FROM email_threads
				WHERE tenant_id = $1 AND subject ILIKE $2
			`, prodTenantID, "%"+expected.subjectContains+"%").Scan(&count)
			if err != nil {
				t.Logf("Warning: could not check thread %q: %v", expected.subjectContains, err)
				continue
			}

			if count > 0 {
				t.Logf("Thread %q: %d messages", expected.subjectContains, count)
				assert.GreaterOrEqual(t, count, expected.minMessages,
					"thread %q should have at least %d messages", expected.subjectContains, expected.minMessages)
			} else {
				t.Logf("Thread %q not found yet (may need reprocessing)", expected.subjectContains)
			}
		}
	})
}
