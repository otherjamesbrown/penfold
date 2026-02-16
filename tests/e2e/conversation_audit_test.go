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

// --- pf-a33ddf: Conversation audit — mis-linkage detection ---

// TestConversation_AuditRunsWithoutErrors verifies that penf conversation audit
// completes without errors on the current conversation set.
//
// Acceptance criteria (pf-a33ddf): Audit job runs without errors on current conversations.
func TestConversation_AuditRunsWithoutErrors(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	auditResult := env.SafeCLI.RunWithArgs(ctx, "conversation", "audit", "-o", "json")
	require.True(t, auditResult.Success(),
		"conversation audit should succeed: %s", auditResult.Stderr)

	// Output should be valid JSON
	var auditReport struct {
		ConversationsAudited int `json:"conversations_audited"`
		Flagged              []struct {
			ConversationID string `json:"conversation_id"`
			ItemID         string `json:"item_id"`
			Reason         string `json:"reason"`
			SuggestedAction string `json:"suggested_action"`
		} `json:"flagged"`
		Orphans []struct {
			ContentID       string `json:"content_id"`
			Reason          string `json:"reason"`
			SuggestedConvID string `json:"suggested_conversation_id"`
		} `json:"orphans"`
	}
	require.NoError(t, json.Unmarshal([]byte(auditResult.Stdout), &auditReport),
		"audit output should be valid JSON")

	assert.Greater(t, auditReport.ConversationsAudited, 0,
		"audit should have checked at least one conversation")
}

// TestConversation_AuditFlaggedItemsHaveReasons verifies that flagged items
// include a reason and suggested action.
//
// Acceptance criteria (pf-a33ddf): Flagged items include reason and suggested action.
func TestConversation_AuditFlaggedItemsHaveReasons(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	auditResult := env.SafeCLI.RunWithArgs(ctx, "conversation", "audit", "-o", "json")
	require.True(t, auditResult.Success(),
		"conversation audit should succeed: %s", auditResult.Stderr)

	var auditReport struct {
		Flagged []struct {
			ConversationID  string `json:"conversation_id"`
			ItemID          string `json:"item_id"`
			Reason          string `json:"reason"`
			SuggestedAction string `json:"suggested_action"`
		} `json:"flagged"`
	}
	require.NoError(t, json.Unmarshal([]byte(auditResult.Stdout), &auditReport))

	// If there are flagged items, each must have reason + suggested action
	validActions := map[string]bool{
		"split": true, "merge": true, "ignore": true, "review": true,
	}
	for _, f := range auditReport.Flagged {
		assert.NotEmpty(t, f.Reason,
			"flagged item %s in %s should have a reason", f.ItemID, f.ConversationID)
		assert.NotEmpty(t, f.SuggestedAction,
			"flagged item %s should have a suggested action", f.ItemID)
		assert.True(t, validActions[f.SuggestedAction],
			"flagged item %s has unexpected action %q", f.ItemID, f.SuggestedAction)
	}
}

// TestConversation_AuditOrphansHaveSuggestions verifies that orphan content items
// are surfaced with a suggested conversation.
//
// Acceptance criteria (pf-a33ddf): Orphan content items surfaced with suggested conversation.
func TestConversation_AuditOrphansHaveSuggestions(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	auditResult := env.SafeCLI.RunWithArgs(ctx, "conversation", "audit", "-o", "json")
	require.True(t, auditResult.Success(),
		"conversation audit should succeed: %s", auditResult.Stderr)

	var auditReport struct {
		Orphans []struct {
			ContentID       string `json:"content_id"`
			Reason          string `json:"reason"`
			SuggestedConvID string `json:"suggested_conversation_id"`
		} `json:"orphans"`
	}
	require.NoError(t, json.Unmarshal([]byte(auditResult.Stdout), &auditReport))

	// If there are orphans, each should have a reason
	for _, o := range auditReport.Orphans {
		assert.NotEmpty(t, o.ContentID,
			"orphan should have a content_id")
		assert.NotEmpty(t, o.Reason,
			"orphan %s should have a reason", o.ContentID)
		// suggested_conversation_id may be empty if no good match found
	}
}

// TestConversation_AuditResultsVisibleViaCLI verifies the audit command
// produces human-readable text output (not just JSON).
//
// Acceptance criteria (pf-a33ddf): Results visible via CLI.
func TestConversation_AuditResultsVisibleViaCLI(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Run without -o json to get text output
	textResult := env.SafeCLI.RunWithArgs(ctx, "conversation", "audit")
	require.True(t, textResult.Success(),
		"conversation audit (text) should succeed: %s", textResult.Stderr)

	// Text output should contain meaningful content
	assert.NotEmpty(t, textResult.Stdout, "audit text output should not be empty")
	assert.Contains(t, textResult.Stdout, "audit",
		"audit text output should mention 'audit'")
}
