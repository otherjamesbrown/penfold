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
	t.Logf("    handling_mode: %s", actual.HandlingMode)
	t.Logf("    notification_source: %s", actual.NotificationSource)

	// Check notification_source
	if expected.NotificationSource != "" {
		pass := strings.EqualFold(actual.NotificationSource, expected.NotificationSource)
		if !pass {
			t.Errorf("notification_extract.notification_source: expected %q, got %q", expected.NotificationSource, actual.NotificationSource)
		}
		details = append(details, MatchDetail{
			Check:    "notification_extract.notification_source",
			Pass:     pass,
			Expected: expected.NotificationSource,
			Actual:   actual.NotificationSource,
		})
	}

	// Check handling_mode
	if expected.HandlingMode != "" {
		pass := strings.EqualFold(actual.HandlingMode, expected.HandlingMode)
		if !pass {
			t.Errorf("notification_extract.handling_mode: expected %q, got %q", expected.HandlingMode, actual.HandlingMode)
		}
		details = append(details, MatchDetail{
			Check:    "notification_extract.handling_mode",
			Pass:     pass,
			Expected: expected.HandlingMode,
			Actual:   actual.HandlingMode,
		})
	}

	// Check mode-specific fields
	if expected.ImmediateEscalate != nil {
		details = append(details, matchImmediateEscalate(t, actual, expected.ImmediateEscalate)...)
	}
	if expected.ComplianceStatus != nil {
		details = append(details, matchComplianceStatus(t, actual, expected.ComplianceStatus)...)
	}
	if expected.DailySummary != nil {
		details = append(details, matchDailySummary(t, actual, expected.DailySummary)...)
	}
	if expected.TriageOnce != nil {
		details = append(details, matchTriageOnce(t, actual, expected.TriageOnce)...)
	}

	return details
}

// matchImmediateEscalate validates immediate_escalate handling mode.
func matchImmediateEscalate(t *testing.T, actual *ActualNotificationExtraction, expected *ImmediateEscalateExpectation) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected.ThreatDescription != nil {
		details = append(details, matchStringField(t, "notification_extract.escalation.threat_description", actual.ThreatDescription, expected.ThreatDescription)...)
	}

	if expected.UrgencyLevel != nil && len(expected.UrgencyLevel.OneOf) > 0 {
		pass := false
		for _, opt := range expected.UrgencyLevel.OneOf {
			if strings.EqualFold(actual.UrgencyLevel, opt) {
				pass = true
				break
			}
		}
		if !pass {
			t.Errorf("notification_extract.escalation.urgency_level: got %q, expected one_of %v", actual.UrgencyLevel, expected.UrgencyLevel.OneOf)
		}
		details = append(details, MatchDetail{
			Check:    "notification_extract.escalation.urgency_level",
			Pass:     pass,
			Expected: fmt.Sprintf("one_of %v", expected.UrgencyLevel.OneOf),
			Actual:   actual.UrgencyLevel,
		})
	}

	if expected.RequiresResponse != nil {
		pass := actual.RequiresResponse == *expected.RequiresResponse
		if !pass {
			t.Errorf("notification_extract.escalation.requires_response: expected %v, got %v", *expected.RequiresResponse, actual.RequiresResponse)
		}
		details = append(details, MatchDetail{
			Check:    "notification_extract.escalation.requires_response",
			Pass:     pass,
			Expected: fmt.Sprintf("%v", *expected.RequiresResponse),
			Actual:   fmt.Sprintf("%v", actual.RequiresResponse),
		})
	}

	return details
}

// matchComplianceStatus validates compliance_status handling mode.
func matchComplianceStatus(t *testing.T, actual *ActualNotificationExtraction, expected *ComplianceStatusExpectation) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected.CourseName != nil {
		details = append(details, matchStringField(t, "notification_extract.compliance.course_name", actual.CourseName, expected.CourseName)...)
	}

	if expected.Assignees != nil {
		names := make([]string, len(actual.Assignees))
		for i, a := range actual.Assignees {
			names[i] = a.Person
		}
		// Convert ComplianceAssigneesExpectation to ItemListExpectation for matchItemList
		itemList := &ItemListExpectation{
			MinCount: expected.Assignees.MinCount,
			MaxCount: expected.Assignees.MaxCount,
		}
		details = append(details, matchItemList(t, "notification_extract.compliance.assignees", names, itemList)...)
	}

	if expected.Overdue != nil {
		pass := actual.Overdue == *expected.Overdue
		if !pass {
			t.Errorf("notification_extract.compliance.overdue: expected %v, got %v", *expected.Overdue, actual.Overdue)
		}
		details = append(details, MatchDetail{
			Check:    "notification_extract.compliance.overdue",
			Pass:     pass,
			Expected: fmt.Sprintf("%v", *expected.Overdue),
			Actual:   fmt.Sprintf("%v", actual.Overdue),
		})
	}

	return details
}

// matchDailySummary validates daily_summary handling mode.
func matchDailySummary(t *testing.T, actual *ActualNotificationExtraction, expected *DailySummaryExpectation) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected.ActivitySummary != nil {
		details = append(details, matchStringField(t, "notification_extract.daily_summary.activity_summary", actual.ActivitySummary, expected.ActivitySummary)...)
	}

	if expected.ProjectUpdates != nil {
		count := len(actual.ProjectUpdates)
		if expected.ProjectUpdates.MinCount != nil && count < *expected.ProjectUpdates.MinCount {
			t.Errorf("notification_extract.daily_summary.project_updates: count %d < min %d", count, *expected.ProjectUpdates.MinCount)
		}
		pass := true
		if expected.ProjectUpdates.MinCount != nil && count < *expected.ProjectUpdates.MinCount {
			pass = false
		}
		if expected.ProjectUpdates.MaxCount != nil && count > *expected.ProjectUpdates.MaxCount {
			pass = false
		}
		details = append(details, MatchDetail{
			Check:    "notification_extract.daily_summary.project_updates.count",
			Pass:     pass,
			Expected: fmt.Sprintf("min=%v max=%v", expected.ProjectUpdates.MinCount, expected.ProjectUpdates.MaxCount),
			Actual:   fmt.Sprintf("%d", count),
		})
	}

	return details
}

// matchTriageOnce validates triage_once handling mode.
func matchTriageOnce(t *testing.T, actual *ActualNotificationExtraction, expected *TriageOnceExpectation) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected.TaskDescription != nil {
		details = append(details, matchStringField(t, "notification_extract.triage_once.task_description", actual.TaskDescription, expected.TaskDescription)...)
	}

	if expected.Assignee != nil {
		details = append(details, matchStringField(t, "notification_extract.triage_once.assignee", actual.Assignee, expected.Assignee)...)
	}

	if expected.DueDate != nil {
		details = append(details, matchStringField(t, "notification_extract.triage_once.due_date", actual.DueDate, expected.DueDate)...)
	}

	if expected.IsRepeat != nil {
		pass := actual.IsRepeat == *expected.IsRepeat
		if !pass {
			t.Errorf("notification_extract.triage_once.is_repeat: expected %v, got %v", *expected.IsRepeat, actual.IsRepeat)
		}
		details = append(details, MatchDetail{
			Check:    "notification_extract.triage_once.is_repeat",
			Pass:     pass,
			Expected: fmt.Sprintf("%v", *expected.IsRepeat),
			Actual:   fmt.Sprintf("%v", actual.IsRepeat),
		})
	}

	return details
}
