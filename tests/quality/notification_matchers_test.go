//go:build quality

package quality

import (
	"testing"
)

func TestMatchImmediateEscalate_UrgencyOneOf(t *testing.T) {
	t.Run("urgency_pass", func(t *testing.T) {
		actual := &ActualNotificationExtraction{UrgencyLevel: "HIGH"}
		expected := &ImmediateEscalateExpectation{
			UrgencyLevel: &OneOfMatcher{OneOf: []string{"HIGH", "CRITICAL"}},
		}
		details := matchImmediateEscalate(t, actual, expected)
		for _, d := range details {
			if !d.Pass {
				t.Errorf("expected pass for urgency HIGH in [HIGH, CRITICAL]: %s", d.Check)
			}
		}
	})

	t.Run("urgency_fail", func(t *testing.T) {
		// Verify that LOW urgency fails against HIGH/CRITICAL expectation
		actual := &ActualNotificationExtraction{UrgencyLevel: "LOW"}
		expected := &ImmediateEscalateExpectation{
			UrgencyLevel: &OneOfMatcher{OneOf: []string{"HIGH", "CRITICAL"}},
		}
		// Use inner T to capture failure without failing this test
		inner := &testing.T{}
		details := matchImmediateEscalate(inner, actual, expected)
		if len(details) == 0 || details[0].Pass {
			t.Error("expected fail for urgency LOW against [HIGH, CRITICAL]")
		}
	})
}

func TestMatchImmediateEscalate_RequiresResponse(t *testing.T) {
	t.Run("requires_response_pass", func(t *testing.T) {
		req := true
		actual := &ActualNotificationExtraction{RequiresResponse: true}
		expected := &ImmediateEscalateExpectation{RequiresResponse: &req}
		details := matchImmediateEscalate(t, actual, expected)
		for _, d := range details {
			if !d.Pass {
				t.Errorf("expected pass: %s", d.Check)
			}
		}
	})
}

func TestMatchComplianceStatus_CourseName(t *testing.T) {
	t.Run("course_name_mention_pass", func(t *testing.T) {
		actual := &ActualNotificationExtraction{
			CourseName: "Annual Anti-Bribery & Anti-Corruption Training is overdue",
		}
		expected := &ComplianceStatusExpectation{
			CourseName: &StringFieldExpectation{
				MustMention: []string{"Anti-Bribery"},
			},
		}
		details := matchComplianceStatus(t, actual, expected)
		for _, d := range details {
			if !d.Pass {
				t.Errorf("expected pass for course name mention: %s", d.Check)
			}
		}
	})
}

func TestMatchComplianceStatus_Overdue(t *testing.T) {
	t.Run("overdue_true", func(t *testing.T) {
		overdue := true
		actual := &ActualNotificationExtraction{Overdue: true}
		expected := &ComplianceStatusExpectation{Overdue: &overdue}
		details := matchComplianceStatus(t, actual, expected)
		for _, d := range details {
			if !d.Pass {
				t.Errorf("expected overdue detection pass: %s", d.Check)
			}
		}
	})

	t.Run("not_overdue", func(t *testing.T) {
		overdue := false
		actual := &ActualNotificationExtraction{Overdue: false}
		expected := &ComplianceStatusExpectation{Overdue: &overdue}
		details := matchComplianceStatus(t, actual, expected)
		for _, d := range details {
			if !d.Pass {
				t.Errorf("expected not-overdue pass: %s", d.Check)
			}
		}
	})
}

func TestMatchTriageMustNotBe(t *testing.T) {
	t.Run("must_not_be_enforced", func(t *testing.T) {
		inner := &testing.T{}
		actual := &ActualTriageResult{Importance: "HIGH", Category: "notification"}
		expected := &TriageExpectation{
			Importance: &OneOfMatcher{MustNotBe: []string{"LOW", "MEDIUM"}},
		}
		MatchTriage(inner, expected, actual)
		// HIGH is not in must_not_be, so should not fail
		if inner.Failed() {
			t.Error("expected no failure: HIGH is not in must_not_be [LOW, MEDIUM]")
		}
	})

	t.Run("must_not_be_triggers_failure", func(t *testing.T) {
		inner := &testing.T{}
		actual := &ActualTriageResult{Importance: "LOW", Category: "notification"}
		expected := &TriageExpectation{
			Importance: &OneOfMatcher{MustNotBe: []string{"LOW"}},
		}
		MatchTriage(inner, expected, actual)
		if !inner.Failed() {
			t.Error("expected failure: LOW is in must_not_be [LOW]")
		}
	})
}
