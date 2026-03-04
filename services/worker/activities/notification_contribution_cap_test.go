// Package activities provides a reproduction test for bug pf-1c083d.
package activities

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestTriage_HybridNotification_ContributionNotCapped_pf1c083d is the reproduction
// test for bug pf-1c083d (SECONDARY issue).
//
// # Bug
//
// triage_activities.go line 516 caps ContentContribution to LOW for ALL
// IsNotification() subtypes — including "hybrid" notifications that embed
// substantive human-written content. A Google Docs comment notification
// (from: comments-noreply@docs.google.com) contains the full text of a human's
// comment: it may include questions, decisions, action items, or discussion
// that deserves deep processing.
//
// The cap was introduced in pf-bcb565 to prevent Jira/Slack state-change
// notifications from consuming expensive deep_analyze capacity. The intent was
// sound for pure system notifications, but the implementation is too broad: it
// uses IsNotification() which matches ALL notification subtypes without
// distinguishing hybrid content (Google Docs comments) from pure system noise
// (Jira ticket transitions, Slack join events).
//
// # Expected behaviour (after fix)
//
// A Google Docs comment notification whose body contains substantive human-written
// content should preserve the LLM-assigned ContentContribution (HIGH/MEDIUM) rather
// than being capped to LOW.
//
// # Actual behaviour (current, buggy code)
//
// ContentContribution is ALWAYS capped to LOW whenever IsNotification() returns
// true, regardless of whether the content is a pure state-change or a human-written
// comment.
//
// This test MUST FAIL against unpatched code.
func TestTriage_HybridNotification_ContributionNotCapped_pf1c083d(t *testing.T) {
	logger := logging.NewNopLogger()

	// LLM correctly assesses the email as HIGH contribution: it contains a substantive
	// human-written comment with a question and a proposal. This is exactly the kind
	// of content that should go through deep_analyze.
	highContribution := "HIGH"
	contributionReason := "Contains human-written comment with decision question and action proposal"

	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			return &aiv1.TriageContentResponse{
				Category:            "PROJECT_UPDATE",
				Importance:          "HIGH",
				Reason:              "Google Docs comment contains substantive project discussion",
				ModelUsed:           "qwen3:8b",
				ContentContribution: &highContribution,
				ContributionReason:  &contributionReason,
			}, nil
		},
	}

	// Rule engine classifies comments-noreply@docs.google.com as NOTIFICATION.
	// This is the correct classification — but it must not blindly trigger the cap.
	mockClassRepo := &mockClassificationRepo{
		loadRulesFn: func(ctx context.Context, tenantID string) ([]classification.ClassificationRule, error) {
			return []classification.ClassificationRule{
				{
					ID: 1, Name: "google-docs-comments", Priority: 20, Active: true,
					ContentTypeScope:   "EMAIL",
					ContentType:        "EMAIL",
					ContentSubtype:     "NOTIFICATION",
					NotificationSource: "google_docs",
					Conditions: []classification.MatchCondition{
						{Field: "from_address", MatchType: "contains", Value: "comments-noreply@docs.google.com"},
					},
				},
			}, nil
		},
	}

	mockEnrichment := &mockEnrichmentRepository{
		getBySourceIDFn: func(ctx context.Context, sourceID int64) (*EnrichmentRecord, error) {
			return &EnrichmentRecord{SourceID: sourceID}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, mockEnrichment, mockClassRepo)

	// Real-world scenario: Alice comments on a Google Doc asking a strategic question.
	// The email body is 100% human-written content — not a system state-change event.
	// The fact that it arrived via Google Docs notification infrastructure does not
	// reduce its knowledge value.
	input := TriageInput{
		TenantID:  "test-tenant",
		SourceID:  20000,
		ContentID: "em-gdocs-comment-001",
		JobID:     "job-gdocs-001",
		Subject:   "Alice commented on \"Q3 Strategy - Final Draft\"",
		SenderEmail: "comments-noreply@docs.google.com",
		Content: `Alice Chen commented on "Q3 Strategy - Final Draft":

Should we move the API migration deadline from September to November? The current
timeline assumes the auth service will be ready by August, but the auth team just
confirmed they won't hit that milestone. Slipping to November gives us a 6-week
buffer and avoids shipping a half-integrated product.

I'd also propose we add a rollback plan to section 4 — we haven't documented what
happens if the new API has breaking changes after go-live. @Bob can you review the
risk section before Thursday's review?

--
Reply to this email to respond to this comment or open it in Google Docs:
https://docs.google.com/document/d/abc123/edit`,
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// The rule engine correctly identifies this as a NOTIFICATION subtype.
	require.Equal(t, "NOTIFICATION", output.ContentSubtype,
		"Google Docs comment notification should be classified as NOTIFICATION subtype")

	// BUG pf-1c083d: ContentContribution is capped to LOW because IsNotification()
	// returns true for ALL notification subtypes. The cap must not apply to hybrid
	// notifications with substantive human-written content.
	//
	// Expected (after fix): HIGH — the LLM correctly assessed this content.
	// Actual (current bug): LOW — blindly capped by the pf-bcb565 notification cap.
	require.Equal(t, "HIGH", output.ContentContribution,
		"bug pf-1c083d: Google Docs comment notification with substantive human-written content "+
			"had ContentContribution capped from HIGH to LOW. "+
			"The IsNotification() cap in triage_activities.go:516 must not apply to hybrid "+
			"notifications that embed human-authored content. "+
			"Got %q, want %q", output.ContentContribution, "HIGH")

	// The original AI-assigned reason should also be preserved, not replaced with
	// the cap message.
	require.NotContains(t, output.ContributionReason, "capped",
		"bug pf-1c083d: ContributionReason was overwritten with cap message for hybrid notification. "+
			"Got: %q", output.ContributionReason)
}

// TestTriage_PureSystemNotification_ContributionCappedLow_pf1c083d is the positive
// control: a pure Jira state-change notification (no human-written content) SHOULD
// have its contribution capped to LOW. This test verifies the fix does not
// over-correct and remove the cap for all notifications.
//
// This test MUST PASS against both patched and unpatched code — it confirms the
// pf-bcb565 cap remains in force for pure system notifications.
func TestTriage_PureSystemNotification_ContributionCappedLow_pf1c083d(t *testing.T) {
	logger := logging.NewNopLogger()

	// LLM over-estimates: it sees "PROJECT_UPDATE" content in a Jira state-change
	// notification and returns HIGH. The cap should override it because this is pure
	// machine-generated state-change content with no human-written body.
	highContribution := "HIGH"

	mockClient := &mockAIClient{
		triageContentFn: func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
			return &aiv1.TriageContentResponse{
				Category:            "PROJECT_UPDATE",
				Importance:          "MEDIUM",
				Reason:              "Jira ticket status change",
				ModelUsed:           "qwen3:8b",
				ContentContribution: &highContribution,
			}, nil
		},
	}

	// Jira notification — pure system-generated content.
	mockClassRepo := &mockClassificationRepo{
		loadRulesFn: func(ctx context.Context, tenantID string) ([]classification.ClassificationRule, error) {
			return []classification.ClassificationRule{
				{
					ID: 2, Name: "jira", Priority: 10, Active: true,
					ContentTypeScope:   "EMAIL",
					ContentType:        "EMAIL",
					ContentSubtype:     "NOTIFICATION",
					NotificationSource: "jira",
					Conditions: []classification.MatchCondition{
						{Field: "from_address", MatchType: "contains", Value: "jira@atlassian"},
					},
				},
			}, nil
		},
	}

	mockEnrichment := &mockEnrichmentRepository{
		getBySourceIDFn: func(ctx context.Context, sourceID int64) (*EnrichmentRecord, error) {
			return &EnrichmentRecord{SourceID: sourceID}, nil
		},
	}

	activities := NewTriageActivities(logger, mockClient, nil, mockEnrichment, mockClassRepo)

	input := TriageInput{
		TenantID:    "test-tenant",
		SourceID:    20001,
		ContentID:   "em-jira-state-change",
		JobID:       "job-jira-001",
		Subject:     "[JIRA] (PROJ-789) Status changed: In Progress -> Done",
		SenderEmail: "jira@atlassian.net",
		// Pure system-generated content: no human-written sentences.
		Content:     "PROJ-789 has been resolved.\nStatus: Done\nResolution: Fixed\nUpdated by: automated-deploy-bot",
		ContentType: "email",
	}

	output, err := activities.Triage(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	require.Equal(t, "NOTIFICATION", output.ContentSubtype)

	// The cap SHOULD apply here: pure system notifications must remain LOW.
	require.Equal(t, "LOW", output.ContentContribution,
		"positive control (pf-1c083d): pure Jira state-change notification must have "+
			"ContentContribution capped to LOW (pf-bcb565 cap must remain in force for "+
			"non-hybrid notifications)")
}
