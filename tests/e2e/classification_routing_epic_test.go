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
// Classification & Routing Epic E2E Tests (pf-eda4be)
// ============================================================================
//
// These tests verify the subtype-driven pipeline routing epic end-to-end:
//
//   1. TestE2E_ClassificationRulesWave3 — Phase 1a acceptance test
//      Verifies new classification rules correctly set content_subtype for
//      calendar responses, security alerts, and IT notifications.
//
//   2. TestE2E_PipelineRoutingLightweight — Phase 2 acceptance test
//      Verifies that classified content routes to the correct lightweight
//      pipeline and runs the expected number of stages.
//
// These are ACCEPTANCE tests owned by Penfold. Mycroft implements the
// migrations that make them pass.
//
// ============================================================================

// TestE2E_ClassificationRulesWave3 verifies that new classification rules
// correctly identify content subtypes for email categories that were previously
// misclassified as STANDALONE or HUMAN.
//
// Phase 1a acceptance test (pf-3a43e7).
func TestE2E_ClassificationRulesWave3(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-classwave3-%d", time.Now().UnixNano())

	// --- Test case: Calendar accept email ---
	t.Run("calendar_accept_classified_as_calendar_response", func(t *testing.T) {
		sourceID := createEmailSourceCLI(t, env, emailSourceOpts{
			MessageID:   fmt.Sprintf("<cal-accept-%s@e2e.test>", testID),
			Subject:     "Accepted: Weekly Team Standup",
			FromAddress: "colleague@example.com",
			FromName:    "Jane Smith",
			Body:        "Jane Smith has accepted this invitation.",
			Date:        time.Now().Add(-1 * time.Hour),
			ExternalID:  fmt.Sprintf("cal-accept-%s", testID),
		})

		runPipelineAndWait(t, env, sourceID, 120*time.Second)

		var contentType, contentSubtype string
		err := env.DB.QueryRow(ctx, `
			SELECT content_type, content_subtype
			FROM content_enrichment
			WHERE source_id = $1 AND tenant_id = $2
		`, sourceID, testTenantID(env)).Scan(&contentType, &contentSubtype)
		require.NoError(t, err, "should find content_enrichment for source %d", sourceID)

		assert.Equal(t, "EMAIL", contentType, "content_type should be EMAIL")
		assert.Equal(t, "CALENDAR_RESPONSE", contentSubtype,
			"'Accepted:' subject should classify as CALENDAR_RESPONSE")
	})

	// --- Test case: Calendar decline email ---
	t.Run("calendar_decline_classified_as_calendar_response", func(t *testing.T) {
		sourceID := createEmailSourceCLI(t, env, emailSourceOpts{
			MessageID:   fmt.Sprintf("<cal-decline-%s@e2e.test>", testID),
			Subject:     "Declined: James / Tom 1:1",
			FromAddress: "tom@example.com",
			FromName:    "Tom Wilson",
			Body:        "Tom Wilson has declined this invitation.",
			Date:        time.Now().Add(-1 * time.Hour),
			ExternalID:  fmt.Sprintf("cal-decline-%s", testID),
		})

		runPipelineAndWait(t, env, sourceID, 120*time.Second)

		var contentType, contentSubtype string
		err := env.DB.QueryRow(ctx, `
			SELECT content_type, content_subtype
			FROM content_enrichment
			WHERE source_id = $1 AND tenant_id = $2
		`, sourceID, testTenantID(env)).Scan(&contentType, &contentSubtype)
		require.NoError(t, err)

		assert.Equal(t, "EMAIL", contentType)
		assert.Equal(t, "CALENDAR_RESPONSE", contentSubtype,
			"'Declined:' subject should classify as CALENDAR_RESPONSE")
	})

	// --- Test case: Calendar tentative email ---
	t.Run("calendar_tentative_classified_as_calendar_response", func(t *testing.T) {
		sourceID := createEmailSourceCLI(t, env, emailSourceOpts{
			MessageID:   fmt.Sprintf("<cal-tentative-%s@e2e.test>", testID),
			Subject:     "Tentative: Project Review Meeting",
			FromAddress: "alice@example.com",
			FromName:    "Alice Brown",
			Body:        "Alice Brown has tentatively accepted this invitation.",
			Date:        time.Now().Add(-1 * time.Hour),
			ExternalID:  fmt.Sprintf("cal-tentative-%s", testID),
		})

		runPipelineAndWait(t, env, sourceID, 120*time.Second)

		var contentType, contentSubtype string
		err := env.DB.QueryRow(ctx, `
			SELECT content_type, content_subtype
			FROM content_enrichment
			WHERE source_id = $1 AND tenant_id = $2
		`, sourceID, testTenantID(env)).Scan(&contentType, &contentSubtype)
		require.NoError(t, err)

		assert.Equal(t, "EMAIL", contentType)
		assert.Equal(t, "CALENDAR_RESPONSE", contentSubtype,
			"'Tentative:' subject should classify as CALENDAR_RESPONSE")
	})

	// --- Test case: Google security alert ---
	t.Run("google_security_alert_classified_as_notification", func(t *testing.T) {
		sourceID := createEmailSourceCLI(t, env, emailSourceOpts{
			MessageID:   fmt.Sprintf("<security-%s@e2e.test>", testID),
			Subject:     "Security alert: New sign-in from Chrome on Mac",
			FromAddress: "no-reply@accounts.google.com",
			FromName:    "Google",
			Body:        "Someone just signed in to your Google Account from a new device.",
			Date:        time.Now().Add(-1 * time.Hour),
			ExternalID:  fmt.Sprintf("security-%s", testID),
		})

		runPipelineAndWait(t, env, sourceID, 120*time.Second)

		var contentType, contentSubtype string
		err := env.DB.QueryRow(ctx, `
			SELECT content_type, content_subtype
			FROM content_enrichment
			WHERE source_id = $1 AND tenant_id = $2
		`, sourceID, testTenantID(env)).Scan(&contentType, &contentSubtype)
		require.NoError(t, err)

		assert.Equal(t, "EMAIL", contentType)
		assert.Equal(t, "NOTIFICATION", contentSubtype,
			"Google security alert should classify as NOTIFICATION")
	})

	// --- Test case: Human email regression check ---
	t.Run("human_email_still_classified_as_human", func(t *testing.T) {
		sourceID := createEmailSourceCLI(t, env, emailSourceOpts{
			MessageID:   fmt.Sprintf("<human-regress-%s@e2e.test>", testID),
			Subject:     "Quick question about the Q3 roadmap",
			FromAddress: "colleague@company.com",
			FromName:    "Sarah Chen",
			Body:        "Hey James, I wanted to discuss the roadmap priorities for Q3. Can we chat this week?",
			Date:        time.Now().Add(-1 * time.Hour),
			ExternalID:  fmt.Sprintf("human-regress-%s", testID),
		})

		runPipelineAndWait(t, env, sourceID, 120*time.Second)

		var contentType, contentSubtype string
		err := env.DB.QueryRow(ctx, `
			SELECT content_type, content_subtype
			FROM content_enrichment
			WHERE source_id = $1 AND tenant_id = $2
		`, sourceID, testTenantID(env)).Scan(&contentType, &contentSubtype)
		require.NoError(t, err)

		assert.Equal(t, "EMAIL", contentType)
		assert.Equal(t, "HUMAN", contentSubtype,
			"Regular human email must still classify as HUMAN (regression check)")
	})

	// --- Test case: Existing JIRA regression check ---
	t.Run("jira_still_classified_as_notification", func(t *testing.T) {
		sourceID := createEmailSourceCLI(t, env, emailSourceOpts{
			MessageID:   fmt.Sprintf("<jira-regress-%s@e2e.test>", testID),
			Subject:     "[TRACK-JIRA] PROJ-789 updated",
			FromAddress: "jira@atlassian.net",
			FromName:    "JIRA",
			Body:        "Issue PROJ-789 has been updated by Test User.",
			Date:        time.Now().Add(-1 * time.Hour),
			ExternalID:  fmt.Sprintf("jira-regress-%s", testID),
		})

		runPipelineAndWait(t, env, sourceID, 120*time.Second)

		var contentType, contentSubtype string
		err := env.DB.QueryRow(ctx, `
			SELECT content_type, content_subtype
			FROM content_enrichment
			WHERE source_id = $1 AND tenant_id = $2
		`, sourceID, testTenantID(env)).Scan(&contentType, &contentSubtype)
		require.NoError(t, err)

		assert.Equal(t, "EMAIL", contentType)
		assert.Equal(t, "NOTIFICATION", contentSubtype,
			"JIRA email must still classify as NOTIFICATION (regression check)")
	})
}

// TestE2E_PipelineRoutingLightweight verifies that content classified with
// different subtypes routes to the correct pipeline and runs the expected
// number of stages.
//
// Phase 2 acceptance test (pf-cd2269).
func TestE2E_PipelineRoutingLightweight(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-routing-%d", time.Now().UnixNano())

	// Helper to count completed stages for a source
	countStages := func(t *testing.T, sourceID int64) int {
		t.Helper()
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM pipeline_runs
			WHERE source_id = $1 AND status = 'completed'
		`, sourceID).Scan(&count)
		require.NoError(t, err)
		return count
	}

	// Helper to check if a specific stage ran
	stageRan := func(t *testing.T, sourceID int64, stage string) bool {
		t.Helper()
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM pipeline_runs
			WHERE source_id = $1 AND stage = $2
		`, sourceID, stage).Scan(&count)
		require.NoError(t, err)
		return count > 0
	}

	// --- Test case: Notification email → notification pipeline (5 stages) ---
	t.Run("notification_routes_to_notification_pipeline", func(t *testing.T) {
		sourceID := createEmailSourceCLI(t, env, emailSourceOpts{
			MessageID:   fmt.Sprintf("<notif-route-%s@e2e.test>", testID),
			Subject:     "[TRACK-JIRA] ROUTE-123 updated",
			FromAddress: "jira@atlassian.net",
			FromName:    "JIRA",
			Body:        "Issue ROUTE-123 has been updated by Test User.",
			Date:        time.Now().Add(-1 * time.Hour),
			ExternalID:  fmt.Sprintf("notif-route-%s", testID),
		})

		runPipelineAndWait(t, env, sourceID, 120*time.Second)

		stages := countStages(t, sourceID)
		assert.Equal(t, 5, stages,
			"notification pipeline should run 5 stages (parse, triage, extract_ner, persist, embed)")

		// Should have NER but NOT deep analysis
		assert.True(t, stageRan(t, sourceID, "extract_ner"),
			"notification pipeline should include extract_ner")
		assert.False(t, stageRan(t, sourceID, "analyze"),
			"notification pipeline should NOT include analyze")
		assert.False(t, stageRan(t, sourceID, "extract_assertions"),
			"notification pipeline should NOT include extract_assertions")
	})

	// --- Test case: Auto-reply email → auto_reply pipeline (3 stages) ---
	t.Run("auto_reply_routes_to_auto_reply_pipeline", func(t *testing.T) {
		sourceID := createEmailSourceCLI(t, env, emailSourceOpts{
			MessageID:   fmt.Sprintf("<autoreply-route-%s@e2e.test>", testID),
			Subject:     "Automatic reply: Out of office",
			FromAddress: "absent@company.com",
			FromName:    "Bob Away",
			Body:        "I am out of the office until Monday. For urgent matters contact support@company.com.",
			Date:        time.Now().Add(-1 * time.Hour),
			ExternalID:  fmt.Sprintf("autoreply-route-%s", testID),
		})

		runPipelineAndWait(t, env, sourceID, 120*time.Second)

		stages := countStages(t, sourceID)
		assert.Equal(t, 3, stages,
			"auto_reply pipeline should run 3 stages (parse, triage, embed)")

		// Should NOT have any extraction
		assert.False(t, stageRan(t, sourceID, "extract_ner"),
			"auto_reply pipeline should NOT include extract_ner")
		assert.False(t, stageRan(t, sourceID, "extract_semantic"),
			"auto_reply pipeline should NOT include extract_semantic")
		assert.False(t, stageRan(t, sourceID, "analyze"),
			"auto_reply pipeline should NOT include analyze")
	})

	// --- Test case: Human email → standard pipeline (9 stages, regression) ---
	t.Run("human_email_still_routes_to_standard_pipeline", func(t *testing.T) {
		sourceID := createEmailSourceCLI(t, env, emailSourceOpts{
			MessageID:   fmt.Sprintf("<human-route-%s@e2e.test>", testID),
			Subject:     "Strategy discussion: Q3 product priorities",
			FromAddress: "director@company.com",
			FromName:    "VP Product",
			Body:        "Team, I'd like to discuss our Q3 priorities. The key areas are: 1) Customer retention improvements, 2) New enterprise features, 3) Performance optimization. Let's align on resource allocation.",
			Date:        time.Now().Add(-1 * time.Hour),
			ExternalID:  fmt.Sprintf("human-route-%s", testID),
		})

		runPipelineAndWait(t, env, sourceID, 120*time.Second)

		stages := countStages(t, sourceID)
		// Standard pipeline has 9 stages. Some may be skipped if triage
		// says low importance, but all stages should at least exist.
		// Check that the full extraction stages are present.
		assert.GreaterOrEqual(t, stages, 7,
			"standard pipeline should run at least 7 stages (some optional stages may be skipped)")

		// Must include deep analysis stages
		assert.True(t, stageRan(t, sourceID, "extract_ner"),
			"standard pipeline must include extract_ner")
		assert.True(t, stageRan(t, sourceID, "extract_semantic"),
			"standard pipeline must include extract_semantic")
		assert.True(t, stageRan(t, sourceID, "persist"),
			"standard pipeline must include persist")
		assert.True(t, stageRan(t, sourceID, "embed"),
			"standard pipeline must include embed")
	})

	// --- Test case: Calendar response → attendees_only pipeline (4 stages) ---
	t.Run("calendar_response_routes_to_attendees_only_pipeline", func(t *testing.T) {
		sourceID := createEmailSourceCLI(t, env, emailSourceOpts{
			MessageID:   fmt.Sprintf("<cal-route-%s@e2e.test>", testID),
			Subject:     "Accepted: Engineering All-Hands",
			FromAddress: "colleague@company.com",
			FromName:    "Jane Doe",
			Body:        "Jane Doe has accepted this invitation.",
			Date:        time.Now().Add(-1 * time.Hour),
			ExternalID:  fmt.Sprintf("cal-route-%s", testID),
		})

		runPipelineAndWait(t, env, sourceID, 120*time.Second)

		stages := countStages(t, sourceID)
		assert.Equal(t, 4, stages,
			"calendar/attendees_only pipeline should run 4 stages (parse, triage, extract_ner, embed)")

		// Should have NER (attendee extraction) but not deep analysis
		assert.True(t, stageRan(t, sourceID, "extract_ner"),
			"attendees_only pipeline should include extract_ner")
		assert.False(t, stageRan(t, sourceID, "analyze"),
			"attendees_only pipeline should NOT include analyze")
		assert.False(t, stageRan(t, sourceID, "extract_assertions"),
			"attendees_only pipeline should NOT include extract_assertions")
	})
}
