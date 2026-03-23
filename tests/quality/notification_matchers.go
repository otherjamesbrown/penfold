//go:build quality

package quality

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// getNotificationExtraction queries the notification extraction JSON from content_enrichment.
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
// Checks handling_mode and notification_source, then dispatches to the
// appropriate handling-mode-specific sub-matcher.
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

	// Check handling_mode
	if expected.HandlingMode != "" {
		pass := strings.EqualFold(actual.HandlingMode, expected.HandlingMode)
		if !pass {
			t.Errorf("notification.handling_mode: got %q, expected %q", actual.HandlingMode, expected.HandlingMode)
		}
		details = append(details, MatchDetail{
			Check:    "notification.handling_mode",
			Pass:     pass,
			Expected: expected.HandlingMode,
			Actual:   actual.HandlingMode,
		})
	}

	// Check notification_source
	if expected.NotificationSource != "" {
		pass := strings.EqualFold(actual.NotificationSource, expected.NotificationSource)
		if !pass {
			t.Errorf("notification.notification_source: got %q, expected %q", actual.NotificationSource, expected.NotificationSource)
		}
		details = append(details, MatchDetail{
			Check:    "notification.notification_source",
			Pass:     pass,
			Expected: expected.NotificationSource,
			Actual:   actual.NotificationSource,
		})
	}

	// Dispatch to handling-mode-specific sub-matcher
	switch strings.ToLower(actual.HandlingMode) {
	case "triage_once":
		if expected.TriageOnce != nil {
			details = append(details, matchTriageOnce(t, expected.TriageOnce, actual)...)
		}
	case "daily_summary":
		if expected.DailySummary != nil {
			details = append(details, matchDailySummary(t, expected.DailySummary, actual)...)
		}
	case "compliance_status":
		if expected.ComplianceStatus != nil {
			details = append(details, matchComplianceStatus(t, expected.ComplianceStatus, actual)...)
		}
	case "immediate_escalate":
		if expected.ImmediateEscalate != nil {
			details = append(details, matchImmediateEscalate(t, expected.ImmediateEscalate, actual)...)
		}
	}

	return details
}

// matchTriageOnce validates extraction for triage_once notifications.
// Checks: is_repeat detection, task description presence, assignee/due date extraction.
func matchTriageOnce(t *testing.T, expected *TriageOnceExpectation, actual *ActualNotificationExtraction) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	t.Logf("    [triage_once] is_repeat: %v", actual.IsRepeat)
	t.Logf("    [triage_once] task_description: %s", truncate(actual.TaskDescription, 100))
	t.Logf("    [triage_once] assignee: %s", actual.Assignee)
	t.Logf("    [triage_once] due_date: %s", actual.DueDate)

	// Check is_repeat
	if expected.IsRepeat != nil {
		pass := actual.IsRepeat == *expected.IsRepeat
		if !pass {
			t.Errorf("triage_once.is_repeat: expected %v, got %v", *expected.IsRepeat, actual.IsRepeat)
		}
		details = append(details, MatchDetail{
			Check:    "triage_once.is_repeat",
			Pass:     pass,
			Expected: fmt.Sprintf("%v", *expected.IsRepeat),
			Actual:   fmt.Sprintf("%v", actual.IsRepeat),
		})
	}

	// Check task_description
	if expected.TaskDescription != nil {
		details = append(details, matchStringField(t, "triage_once.task_description", actual.TaskDescription, expected.TaskDescription)...)
	} else {
		// Always assert task_description is non-empty
		pass := actual.TaskDescription != ""
		if !pass {
			t.Errorf("triage_once.task_description: expected non-empty task description")
		}
		details = append(details, MatchDetail{
			Check:  "triage_once.task_description.present",
			Pass:   pass,
			Actual: truncate(actual.TaskDescription, 100),
		})
	}

	// Check assignee
	if expected.Assignee != nil {
		details = append(details, matchStringField(t, "triage_once.assignee", actual.Assignee, expected.Assignee)...)
	}

	// Check due_date
	if expected.DueDate != nil {
		details = append(details, matchStringField(t, "triage_once.due_date", actual.DueDate, expected.DueDate)...)
	}

	return details
}

// matchDailySummary validates extraction for daily_summary notifications.
// Checks: activity_summary field present and meets min_length, project context extraction.
func matchDailySummary(t *testing.T, expected *DailySummaryExpectation, actual *ActualNotificationExtraction) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	t.Logf("    [daily_summary] activity_summary: %s", truncate(actual.ActivitySummary, 100))
	t.Logf("    [daily_summary] project_updates: %d", len(actual.ProjectUpdates))

	// Check activity_summary
	if expected.ActivitySummary != nil {
		details = append(details, matchStringField(t, "daily_summary.activity_summary", actual.ActivitySummary, expected.ActivitySummary)...)
	} else {
		// Always assert activity_summary is non-empty
		pass := actual.ActivitySummary != ""
		if !pass {
			t.Errorf("daily_summary.activity_summary: expected non-empty activity summary")
		}
		details = append(details, MatchDetail{
			Check:  "daily_summary.activity_summary.present",
			Pass:   pass,
			Actual: truncate(actual.ActivitySummary, 100),
		})
	}

	// Check project_updates count
	if expected.ProjectUpdates != nil {
		details = append(details, matchCount(t, "daily_summary.project_updates", len(actual.ProjectUpdates), expected.ProjectUpdates)...)
	} else {
		// Log project context even without count expectations
		for _, pu := range actual.ProjectUpdates {
			t.Logf("      project: %s — %s", pu.Project, truncate(pu.Summary, 80))
		}
	}

	return details
}

// matchComplianceStatus validates extraction for compliance_status notifications.
// Checks: course_name extraction, assignees list (person + due date), overdue flag.
func matchComplianceStatus(t *testing.T, expected *ComplianceStatusExpectation, actual *ActualNotificationExtraction) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	t.Logf("    [compliance_status] course_name: %s", actual.CourseName)
	t.Logf("    [compliance_status] overdue: %v", actual.Overdue)
	t.Logf("    [compliance_status] assignees: %d", len(actual.Assignees))
	for _, a := range actual.Assignees {
		t.Logf("      - %s (due: %s, overdue: %v)", a.Person, a.DueDate, a.Overdue)
	}

	// Check course_name
	if expected.CourseName != nil {
		details = append(details, matchStringField(t, "compliance_status.course_name", actual.CourseName, expected.CourseName)...)
	} else {
		pass := actual.CourseName != ""
		if !pass {
			t.Errorf("compliance_status.course_name: expected non-empty course name")
		}
		details = append(details, MatchDetail{
			Check:  "compliance_status.course_name.present",
			Pass:   pass,
			Actual: actual.CourseName,
		})
	}

	// Check overdue flag
	if expected.Overdue != nil {
		pass := actual.Overdue == *expected.Overdue
		if !pass {
			t.Errorf("compliance_status.overdue: expected %v, got %v", *expected.Overdue, actual.Overdue)
		}
		details = append(details, MatchDetail{
			Check:    "compliance_status.overdue",
			Pass:     pass,
			Expected: fmt.Sprintf("%v", *expected.Overdue),
			Actual:   fmt.Sprintf("%v", actual.Overdue),
		})
	}

	// Check assignees
	if expected.Assignees != nil {
		if expected.Assignees.MinCount != nil {
			pass := len(actual.Assignees) >= *expected.Assignees.MinCount
			if !pass {
				t.Errorf("compliance_status.assignees.min_count: expected >= %d, got %d", *expected.Assignees.MinCount, len(actual.Assignees))
			}
			details = append(details, MatchDetail{
				Check:    "compliance_status.assignees.min_count",
				Pass:     pass,
				Expected: fmt.Sprintf(">= %d", *expected.Assignees.MinCount),
				Actual:   fmt.Sprintf("%d", len(actual.Assignees)),
			})
		}
		if expected.Assignees.MaxCount != nil {
			pass := len(actual.Assignees) <= *expected.Assignees.MaxCount
			if !pass {
				t.Errorf("compliance_status.assignees.max_count: expected <= %d, got %d", *expected.Assignees.MaxCount, len(actual.Assignees))
			}
			details = append(details, MatchDetail{
				Check:    "compliance_status.assignees.max_count",
				Pass:     pass,
				Expected: fmt.Sprintf("<= %d", *expected.Assignees.MaxCount),
				Actual:   fmt.Sprintf("%d", len(actual.Assignees)),
			})
		}
		for _, mf := range expected.Assignees.MustFind {
			found := assigneeMatches(actual.Assignees, mf)
			if !found {
				t.Errorf("compliance_status.assignees.must_find: no assignee matching %s", describeAssigneeMatcher(mf))
			}
			details = append(details, MatchDetail{
				Check:    fmt.Sprintf("compliance_status.assignees.must_find.%s", mf.PersonContains),
				Pass:     found,
				Expected: describeAssigneeMatcher(mf),
			})
		}
	}

	return details
}

// matchImmediateEscalate validates extraction for immediate_escalate notifications.
// Checks: threat_description present and meets must_mention, urgency level, requires_response flag.
func matchImmediateEscalate(t *testing.T, expected *ImmediateEscalateExpectation, actual *ActualNotificationExtraction) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	t.Logf("    [immediate_escalate] threat_description: %s", truncate(actual.ThreatDescription, 100))
	t.Logf("    [immediate_escalate] urgency_level: %s", actual.UrgencyLevel)
	t.Logf("    [immediate_escalate] requires_response: %v", actual.RequiresResponse)
	t.Logf("    [immediate_escalate] systems_affected: %v", actual.SystemsAffected)

	// Check threat_description
	if expected.ThreatDescription != nil {
		details = append(details, matchStringField(t, "immediate_escalate.threat_description", actual.ThreatDescription, expected.ThreatDescription)...)
	} else {
		pass := actual.ThreatDescription != ""
		if !pass {
			t.Errorf("immediate_escalate.threat_description: expected non-empty threat description")
		}
		details = append(details, MatchDetail{
			Check:  "immediate_escalate.threat_description.present",
			Pass:   pass,
			Actual: truncate(actual.ThreatDescription, 100),
		})
	}

	// Check urgency_level
	if expected.UrgencyLevel != nil {
		pass := true
		for _, forbidden := range expected.UrgencyLevel.MustNotBe {
			if strings.EqualFold(actual.UrgencyLevel, forbidden) {
				t.Errorf("immediate_escalate.urgency_level: got %q which is in must_not_be %v", actual.UrgencyLevel, expected.UrgencyLevel.MustNotBe)
				pass = false
				break
			}
		}
		if pass && len(expected.UrgencyLevel.OneOf) > 0 {
			pass = false
			for _, opt := range expected.UrgencyLevel.OneOf {
				if strings.EqualFold(actual.UrgencyLevel, opt) {
					pass = true
					break
				}
			}
			if !pass {
				t.Errorf("immediate_escalate.urgency_level: got %q, expected one_of %v", actual.UrgencyLevel, expected.UrgencyLevel.OneOf)
			}
		}
		details = append(details, MatchDetail{
			Check:    "immediate_escalate.urgency_level",
			Pass:     pass,
			Expected: fmt.Sprintf("one_of=%v must_not_be=%v", expected.UrgencyLevel.OneOf, expected.UrgencyLevel.MustNotBe),
			Actual:   actual.UrgencyLevel,
		})
	}

	// Check requires_response
	if expected.RequiresResponse != nil {
		pass := actual.RequiresResponse == *expected.RequiresResponse
		if !pass {
			t.Errorf("immediate_escalate.requires_response: expected %v, got %v", *expected.RequiresResponse, actual.RequiresResponse)
		}
		details = append(details, MatchDetail{
			Check:    "immediate_escalate.requires_response",
			Pass:     pass,
			Expected: fmt.Sprintf("%v", *expected.RequiresResponse),
			Actual:   fmt.Sprintf("%v", actual.RequiresResponse),
		})
	}

	return details
}

// --- Helpers ---

func assigneeMatches(assignees []ActualComplianceAssignee, matcher AssigneeMatcher) bool {
	for _, a := range assignees {
		if matcher.PersonContains != "" && !containsCI(a.Person, matcher.PersonContains) {
			continue
		}
		if matcher.DueDateContains != "" && !containsCI(a.DueDate, matcher.DueDateContains) {
			continue
		}
		return true
	}
	return false
}

func describeAssigneeMatcher(m AssigneeMatcher) string {
	parts := []string{}
	if m.PersonContains != "" {
		parts = append(parts, fmt.Sprintf("person_contains=%q", m.PersonContains))
	}
	if m.DueDateContains != "" {
		parts = append(parts, fmt.Sprintf("due_date_contains=%q", m.DueDateContains))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
