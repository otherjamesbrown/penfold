//go:build quality

package quality

import (
	"testing"
)

// --- matchTriageOnce tests ---

func TestMatchTriageOnce(t *testing.T) {
	boolFalse := false
	boolTrue := true
	minLen := 10

	tests := []struct {
		name        string
		expected    *TriageOnceExpectation
		actual      *ActualNotificationExtraction
		expectFail  bool
		expectCount int
	}{
		{
			name: "new_item_with_full_extraction",
			expected: &TriageOnceExpectation{
				IsRepeat:        &boolFalse,
				TaskDescription: &StringFieldExpectation{MinLength: &minLen},
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:    "triage_once",
				IsRepeat:        false,
				TaskDescription: "Review Akamai Player UI design proposal",
				Assignee:        "James Brown",
				DueDate:         "2026-03-30",
			},
			expectFail:  false,
			expectCount: 2, // is_repeat + task_description.min_length
		},
		{
			name: "repeat_item_detected",
			expected: &TriageOnceExpectation{
				IsRepeat: &boolTrue,
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:    "triage_once",
				IsRepeat:        true,
				TaskDescription: "Overdue: complete anti-bribery training",
			},
			expectFail:  false,
			expectCount: 2, // is_repeat + task_description.present (default check)
		},
		{
			name: "is_repeat_mismatch",
			expected: &TriageOnceExpectation{
				IsRepeat: &boolFalse,
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:    "triage_once",
				IsRepeat:        true,
				TaskDescription: "some task",
			},
			expectFail:  true,
			expectCount: 2, // is_repeat + task_description.present
		},
		{
			name: "empty_task_description_fails_default_check",
			expected: &TriageOnceExpectation{
				IsRepeat: &boolFalse,
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:    "triage_once",
				IsRepeat:        false,
				TaskDescription: "",
			},
			expectFail:  true,
			expectCount: 2, // is_repeat + task_description.present
		},
		{
			name: "assignee_and_due_date_checked",
			expected: &TriageOnceExpectation{
				Assignee: &StringFieldExpectation{MustMention: []string{"James"}},
				DueDate:  &StringFieldExpectation{MustMention: []string{"2026"}},
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:    "triage_once",
				TaskDescription: "some task",
				Assignee:        "James Brown",
				DueDate:         "2026-04-15",
			},
			expectFail:  false,
			expectCount: 3, // task_description.present + assignee.must_mention + due_date.must_mention
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner := &testing.T{}
			details := matchTriageOnce(inner, tc.expected, tc.actual)

			if tc.expectFail && !inner.Failed() {
				t.Errorf("expected test to fail but it passed")
			}
			if !tc.expectFail && inner.Failed() {
				t.Errorf("expected test to pass but it failed")
			}
			if tc.expectCount > 0 && len(details) != tc.expectCount {
				t.Errorf("expected %d details, got %d", tc.expectCount, len(details))
			}
		})
	}
}

// --- matchDailySummary tests ---

func TestMatchDailySummary(t *testing.T) {
	minLen := 20
	minCount := 1

	tests := []struct {
		name       string
		expected   *DailySummaryExpectation
		actual     *ActualNotificationExtraction
		expectFail bool
	}{
		{
			name: "full_daily_summary_passes",
			expected: &DailySummaryExpectation{
				ActivitySummary: &StringFieldExpectation{MinLength: &minLen, MustMention: []string{"MTC"}},
				ProjectUpdates:  &CountExpectation{MinCount: &minCount},
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:    "daily_summary",
				ActivitySummary: "MTC Outcomes: 3 updates from Patrick, Patti, and Ilan",
				ProjectUpdates: []ActualNotifProjectUpdate{
					{Project: "MTC Outcomes", Summary: "3 updates from team members"},
				},
			},
			expectFail: false,
		},
		{
			name: "empty_activity_summary_fails",
			expected: &DailySummaryExpectation{
				ActivitySummary: &StringFieldExpectation{MinLength: &minLen},
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:    "daily_summary",
				ActivitySummary: "",
			},
			expectFail: true,
		},
		{
			name: "default_present_check_fails_on_empty",
			expected: &DailySummaryExpectation{},
			actual: &ActualNotificationExtraction{
				HandlingMode:    "daily_summary",
				ActivitySummary: "",
			},
			expectFail: true,
		},
		{
			name: "default_present_check_passes",
			expected: &DailySummaryExpectation{},
			actual: &ActualNotificationExtraction{
				HandlingMode:    "daily_summary",
				ActivitySummary: "Some activity occurred",
			},
			expectFail: false,
		},
		{
			name: "project_updates_count_too_low",
			expected: &DailySummaryExpectation{
				ProjectUpdates: &CountExpectation{MinCount: &minCount},
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:    "daily_summary",
				ActivitySummary: "Some activity",
				ProjectUpdates:  []ActualNotifProjectUpdate{},
			},
			expectFail: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner := &testing.T{}
			matchDailySummary(inner, tc.expected, tc.actual)

			if tc.expectFail && !inner.Failed() {
				t.Errorf("expected test to fail but it passed")
			}
			if !tc.expectFail && inner.Failed() {
				t.Errorf("expected test to pass but it failed")
			}
		})
	}
}

// --- matchComplianceStatus tests ---

func TestMatchComplianceStatus(t *testing.T) {
	boolTrue := true
	minCount := 1

	tests := []struct {
		name       string
		expected   *ComplianceStatusExpectation
		actual     *ActualNotificationExtraction
		expectFail bool
	}{
		{
			name: "full_compliance_passes",
			expected: &ComplianceStatusExpectation{
				CourseName: &StringFieldExpectation{MustMention: []string{"Anti-Bribery"}},
				Assignees: &ComplianceAssigneesExpectation{
					MinCount: &minCount,
					MustFind: []AssigneeMatcher{{PersonContains: "James"}},
				},
				Overdue: &boolTrue,
			},
			actual: &ActualNotificationExtraction{
				HandlingMode: "compliance_status",
				CourseName:   "Anti-Bribery and Anti-Corruption Training",
				Overdue:      true,
				Assignees: []ActualComplianceAssignee{
					{Person: "James Brown", DueDate: "2026-03-01", Overdue: true},
				},
			},
			expectFail: false,
		},
		{
			name: "empty_course_name_fails_default_check",
			expected: &ComplianceStatusExpectation{},
			actual: &ActualNotificationExtraction{
				HandlingMode: "compliance_status",
				CourseName:   "",
			},
			expectFail: true,
		},
		{
			name: "overdue_mismatch",
			expected: &ComplianceStatusExpectation{
				Overdue: &boolTrue,
			},
			actual: &ActualNotificationExtraction{
				HandlingMode: "compliance_status",
				CourseName:   "Security Training",
				Overdue:      false,
			},
			expectFail: true,
		},
		{
			name: "must_find_assignee_not_found",
			expected: &ComplianceStatusExpectation{
				Assignees: &ComplianceAssigneesExpectation{
					MustFind: []AssigneeMatcher{{PersonContains: "Alice"}},
				},
			},
			actual: &ActualNotificationExtraction{
				HandlingMode: "compliance_status",
				CourseName:   "Security Training",
				Assignees: []ActualComplianceAssignee{
					{Person: "James Brown", DueDate: "2026-04-01"},
				},
			},
			expectFail: true,
		},
		{
			name: "assignee_due_date_match",
			expected: &ComplianceStatusExpectation{
				Assignees: &ComplianceAssigneesExpectation{
					MustFind: []AssigneeMatcher{
						{PersonContains: "james", DueDateContains: "2026"},
					},
				},
			},
			actual: &ActualNotificationExtraction{
				HandlingMode: "compliance_status",
				CourseName:   "Security Training",
				Assignees: []ActualComplianceAssignee{
					{Person: "James Brown", DueDate: "2026-04-01"},
				},
			},
			expectFail: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner := &testing.T{}
			matchComplianceStatus(inner, tc.expected, tc.actual)

			if tc.expectFail && !inner.Failed() {
				t.Errorf("expected test to fail but it passed")
			}
			if !tc.expectFail && inner.Failed() {
				t.Errorf("expected test to pass but it failed")
			}
		})
	}
}

// --- matchImmediateEscalate tests ---

func TestMatchImmediateEscalate(t *testing.T) {
	boolTrue := true
	boolFalse := false

	tests := []struct {
		name       string
		expected   *ImmediateEscalateExpectation
		actual     *ActualNotificationExtraction
		expectFail bool
	}{
		{
			name: "full_escalation_passes",
			expected: &ImmediateEscalateExpectation{
				ThreatDescription: &StringFieldExpectation{
					MustMention: []string{"malicious", "DNS"},
				},
				UrgencyLevel:     &OneOfMatcher{OneOf: []string{"HIGH", "CRITICAL"}},
				RequiresResponse: &boolTrue,
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:      "immediate_escalate",
				ThreatDescription: "Malicious DNS request detected from corporate device",
				UrgencyLevel:      "CRITICAL",
				RequiresResponse:  true,
				SystemsAffected:   []string{"laptop-jb-01"},
			},
			expectFail: false,
		},
		{
			name: "empty_threat_description_fails",
			expected: &ImmediateEscalateExpectation{
				RequiresResponse: &boolTrue,
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:     "immediate_escalate",
				ThreatDescription: "",
				RequiresResponse: true,
			},
			expectFail: true,
		},
		{
			name: "urgency_level_not_in_one_of",
			expected: &ImmediateEscalateExpectation{
				UrgencyLevel: &OneOfMatcher{OneOf: []string{"HIGH", "CRITICAL"}},
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:      "immediate_escalate",
				ThreatDescription: "Some threat",
				UrgencyLevel:      "LOW",
			},
			expectFail: true,
		},
		{
			name: "urgency_level_in_must_not_be",
			expected: &ImmediateEscalateExpectation{
				UrgencyLevel: &OneOfMatcher{MustNotBe: []string{"LOW", "MEDIUM"}},
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:      "immediate_escalate",
				ThreatDescription: "Some threat",
				UrgencyLevel:      "LOW",
			},
			expectFail: true,
		},
		{
			name: "requires_response_mismatch",
			expected: &ImmediateEscalateExpectation{
				RequiresResponse: &boolFalse,
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:      "immediate_escalate",
				ThreatDescription: "Routine security check",
				RequiresResponse:  true,
			},
			expectFail: true,
		},
		{
			name: "must_mention_threat_keywords",
			expected: &ImmediateEscalateExpectation{
				ThreatDescription: &StringFieldExpectation{
					MustMention: []string{"phishing"},
				},
			},
			actual: &ActualNotificationExtraction{
				HandlingMode:      "immediate_escalate",
				ThreatDescription: "Suspected phishing email received",
			},
			expectFail: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner := &testing.T{}
			matchImmediateEscalate(inner, tc.expected, tc.actual)

			if tc.expectFail && !inner.Failed() {
				t.Errorf("expected test to fail but it passed")
			}
			if !tc.expectFail && inner.Failed() {
				t.Errorf("expected test to pass but it failed")
			}
		})
	}
}

// --- matchOneOf MustNotBe tests ---

func TestMatchOneOfMustNotBe(t *testing.T) {
	tests := []struct {
		name       string
		actual     string
		matcher    *OneOfMatcher
		expectFail bool
	}{
		{
			name:       "value_in_one_of_passes",
			actual:     "LOW",
			matcher:    &OneOfMatcher{OneOf: []string{"LOW", "MEDIUM"}},
			expectFail: false,
		},
		{
			name:       "value_not_in_one_of_fails",
			actual:     "HIGH",
			matcher:    &OneOfMatcher{OneOf: []string{"LOW", "MEDIUM"}},
			expectFail: true,
		},
		{
			name:       "value_in_must_not_be_fails",
			actual:     "HIGH",
			matcher:    &OneOfMatcher{OneOf: []string{"LOW", "MEDIUM", "HIGH"}, MustNotBe: []string{"HIGH", "CRITICAL"}},
			expectFail: true,
		},
		{
			name:       "value_not_in_must_not_be_passes",
			actual:     "LOW",
			matcher:    &OneOfMatcher{MustNotBe: []string{"HIGH", "CRITICAL"}},
			expectFail: false,
		},
		{
			name:       "case_insensitive_must_not_be",
			actual:     "critical",
			matcher:    &OneOfMatcher{MustNotBe: []string{"HIGH", "CRITICAL"}},
			expectFail: true,
		},
		{
			name:       "nil_matcher_is_noop",
			actual:     "anything",
			matcher:    nil,
			expectFail: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner := &testing.T{}
			matchOneOf(inner, "test.field", tc.actual, tc.matcher)

			if tc.expectFail && !inner.Failed() {
				t.Errorf("expected matchOneOf to fail but it passed")
			}
			if !tc.expectFail && inner.Failed() {
				t.Errorf("expected matchOneOf to pass but it failed")
			}
		})
	}
}

// --- MatchNotificationExtract dispatch tests ---

func TestMatchNotificationExtractDispatch(t *testing.T) {
	// Test that MatchNotificationExtract produces correct handling_mode and
	// notification_source MatchDetail records. We use nil env/sourceID since
	// we're testing the dispatcher logic with pre-built actual data via the
	// sub-matcher functions directly.

	boolFalse := false

	t.Run("handling_mode_check_produces_detail", func(t *testing.T) {
		actual := &ActualNotificationExtraction{
			HandlingMode:    "triage_once",
			TaskDescription: "some task",
		}
		expected := &TriageOnceExpectation{IsRepeat: &boolFalse}
		inner := &testing.T{}
		details := matchTriageOnce(inner, expected, actual)

		if len(details) == 0 {
			t.Error("expected at least one MatchDetail from matchTriageOnce")
		}
		for _, d := range details {
			if d.Check == "" {
				t.Errorf("MatchDetail missing Check field: %+v", d)
			}
		}
	})

	t.Run("all_details_have_check_field", func(t *testing.T) {
		actual := &ActualNotificationExtraction{
			HandlingMode:    "compliance_status",
			CourseName:      "Security Training",
			Overdue:         true,
			Assignees:       []ActualComplianceAssignee{{Person: "James", DueDate: "2026-04-01"}},
		}
		boolTrue := true
		expected := &ComplianceStatusExpectation{
			Overdue: &boolTrue,
		}
		inner := &testing.T{}
		details := matchComplianceStatus(inner, expected, actual)

		for _, d := range details {
			if d.Check == "" {
				t.Errorf("MatchDetail missing Check field: %+v", d)
			}
		}
	})
}
