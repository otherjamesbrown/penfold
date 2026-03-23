//go:build quality

package quality

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// getNotificationExtraction queries the notification JSON from content_enrichment.
func getNotificationExtraction(env *QualityEnv, sourceID int64) (*ActualNotificationExtraction, error) {
	var rawJSON []byte
	err := env.DB.QueryRow(context.Background(),
		`SELECT extracted_data->'notification'
         FROM content_enrichment
         WHERE source_id = $1 AND tenant_id = $2`,
		sourceID, env.TenantID).Scan(&rawJSON)
	if err != nil {
		return nil, fmt.Errorf("query notification extraction: %w", err)
	}
	if rawJSON == nil {
		return nil, fmt.Errorf("no notification extraction data for source %d", sourceID)
	}
	var result ActualNotificationExtraction
	if err := json.Unmarshal(rawJSON, &result); err != nil {
		return nil, fmt.Errorf("unmarshal notification extraction: %w", err)
	}
	return &result, nil
}

// MatchNotificationExtract validates L2 notification extraction quality.
// Returns []MatchDetail AND calls t.Error on failures.
func MatchNotificationExtract(t *testing.T, env *QualityEnv, sourceID int64, expected *NotificationExtractExpectation) []MatchDetail {
	t.Helper()
	if expected == nil {
		return nil
	}
	var details []MatchDetail

	actual, err := getNotificationExtraction(env, sourceID)
	if err != nil {
		t.Errorf("notification_extract: %v", err)
		details = append(details, MatchDetail{Check: "notification_extract.exists", Pass: false, Message: err.Error()})
		return details
	}

	// Log actual output for debugging
	t.Logf("  notification_extract output:")
	t.Logf("    source_system: %s", actual.SourceSystem)
	t.Logf("    notification_type: %s", actual.NotificationType)
	t.Logf("    summary: %s", truncate(actual.Summary, 100))
	t.Logf("    action_required: %v", actual.ActionRequired)
	t.Logf("    urgency: %s", actual.Urgency)
	t.Logf("    people: %d", len(actual.People))

	// Check notification_source matches SourceSystem
	if expected.NotificationSource != "" {
		pass := strings.EqualFold(actual.SourceSystem, expected.NotificationSource)
		if !pass {
			t.Errorf("notification_extract.notification_source: expected %q, got %q", expected.NotificationSource, actual.SourceSystem)
		}
		details = append(details, MatchDetail{
			Check:    "notification_extract.notification_source",
			Pass:     pass,
			Expected: expected.NotificationSource,
			Actual:   actual.SourceSystem,
		})
	}

	// Check escalation fields (immediate_escalate mode)
	if expected.Escalation != nil {
		details = append(details, matchEscalation(t, actual, expected.Escalation)...)
	}

	// Check compliance fields (compliance_status mode)
	if expected.Compliance != nil {
		details = append(details, matchCompliance(t, actual, expected.Compliance)...)
	}

	// Check activity_summary (daily_summary / triage_once modes)
	if expected.ActivitySummary != nil {
		details = append(details, matchStringField(t, "notification_extract.activity_summary", actual.Summary, expected.ActivitySummary)...)
	}

	return details
}

// matchEscalation validates escalation fields for immediate_escalate notifications.
func matchEscalation(t *testing.T, actual *ActualNotificationExtraction, expected *EscalationExpectation) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected.ThreatDescription != nil {
		details = append(details, matchStringField(t, "notification_extract.escalation.threat_description", actual.Summary, expected.ThreatDescription)...)
	}

	if expected.Urgency != nil && len(expected.Urgency.OneOf) > 0 {
		pass := false
		for _, opt := range expected.Urgency.OneOf {
			if strings.EqualFold(actual.Urgency, opt) {
				pass = true
				break
			}
		}
		if !pass {
			t.Errorf("notification_extract.escalation.urgency: got %q, expected one_of %v", actual.Urgency, expected.Urgency.OneOf)
		}
		details = append(details, MatchDetail{
			Check:    "notification_extract.escalation.urgency",
			Pass:     pass,
			Expected: fmt.Sprintf("one_of %v", expected.Urgency.OneOf),
			Actual:   actual.Urgency,
		})
	}

	if expected.RequiresResponse != nil {
		pass := actual.ActionRequired == *expected.RequiresResponse
		if !pass {
			t.Errorf("notification_extract.escalation.requires_response: expected %v, got %v", *expected.RequiresResponse, actual.ActionRequired)
		}
		details = append(details, MatchDetail{
			Check:    "notification_extract.escalation.requires_response",
			Pass:     pass,
			Expected: fmt.Sprintf("%v", *expected.RequiresResponse),
			Actual:   fmt.Sprintf("%v", actual.ActionRequired),
		})
	}

	return details
}

// matchCompliance validates compliance fields for compliance_status notifications.
func matchCompliance(t *testing.T, actual *ActualNotificationExtraction, expected *ComplianceExpectation) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected.CourseName != nil {
		details = append(details, matchStringField(t, "notification_extract.compliance.course_name", actual.Summary, expected.CourseName)...)
	}

	if expected.Assignees != nil {
		names := make([]string, len(actual.People))
		for i, p := range actual.People {
			names[i] = p.Name
		}
		details = append(details, matchItemList(t, "notification_extract.compliance.assignees", names, expected.Assignees)...)
	}

	if expected.DueDatePresent != nil {
		hasDueDate := actual.Action != nil && *actual.Action != ""
		pass := hasDueDate == *expected.DueDatePresent
		if !pass {
			t.Errorf("notification_extract.compliance.due_date_present: expected %v, got %v", *expected.DueDatePresent, hasDueDate)
		}
		details = append(details, MatchDetail{
			Check:    "notification_extract.compliance.due_date_present",
			Pass:     pass,
			Expected: fmt.Sprintf("%v", *expected.DueDatePresent),
			Actual:   fmt.Sprintf("%v", hasDueDate),
		})
	}

	if expected.Overdue != nil {
		summaryLower := strings.ToLower(actual.Summary)
		isOverdue := strings.Contains(summaryLower, "overdue") ||
			strings.Contains(summaryLower, "past due") ||
			strings.Contains(summaryLower, "expired")
		pass := isOverdue == *expected.Overdue
		if !pass {
			t.Errorf("notification_extract.compliance.overdue: expected %v, got %v (summary: %s)",
				*expected.Overdue, isOverdue, truncate(actual.Summary, 80))
		}
		details = append(details, MatchDetail{
			Check:    "notification_extract.compliance.overdue",
			Pass:     pass,
			Expected: fmt.Sprintf("%v", *expected.Overdue),
			Actual:   fmt.Sprintf("%v", isOverdue),
		})
	}

	return details
}
