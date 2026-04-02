//go:build quality

package quality

import (
	"testing"
)

// --- matchTriageDetail tests ---

func TestMatchTriageDetail_Pass(t *testing.T) {
	actual := &ActualTriageResult{Importance: "HIGH", Category: "INCIDENT"}
	expected := &TriageExpectation{
		Importance: &OneOfMatcher{OneOf: []string{"HIGH", "CRITICAL"}},
		Category:   &OneOfMatcher{OneOf: []string{"RISK_ISSUE", "INCIDENT", "OPERATIONAL"}},
	}
	details := matchTriageDetail(t, expected, actual)
	if len(details) == 0 {
		t.Fatal("expected at least one MatchDetail")
	}
	for _, d := range details {
		if !d.Pass {
			t.Errorf("expected pass for check %q: expected=%q actual=%q", d.Check, d.Expected, d.Actual)
		}
	}
}

func TestMatchTriageDetail_Fail_WrongImportance(t *testing.T) {
	actual := &ActualTriageResult{Importance: "LOW", Category: "INCIDENT"}
	expected := &TriageExpectation{
		Importance: &OneOfMatcher{OneOf: []string{"HIGH", "CRITICAL"}},
	}
	inner := &testing.T{}
	details := matchTriageDetail(inner, expected, actual)
	if len(details) == 0 {
		t.Fatal("expected at least one MatchDetail")
	}
	failed := false
	for _, d := range details {
		if !d.Pass {
			failed = true
			if d.Actual != "LOW" {
				t.Errorf("expected actual=%q in MatchDetail, got %q", "LOW", d.Actual)
			}
		}
	}
	if !failed {
		t.Error("expected at least one failing MatchDetail for LOW importance against [HIGH, CRITICAL]")
	}
	if !inner.Failed() {
		t.Error("expected inner testing.T to fail")
	}
}

func TestMatchTriageDetail_NilExpected(t *testing.T) {
	actual := &ActualTriageResult{Importance: "HIGH", Category: "INCIDENT"}
	details := matchTriageDetail(t, nil, actual)
	if len(details) != 0 {
		t.Errorf("expected empty details for nil expected, got %d", len(details))
	}
}

func TestMatchTriageDetail_NilActual(t *testing.T) {
	expected := &TriageExpectation{
		Importance: &OneOfMatcher{OneOf: []string{"HIGH"}},
	}
	inner := &testing.T{}
	details := matchTriageDetail(inner, expected, nil)
	if len(details) == 0 || details[0].Pass {
		t.Error("expected failing MatchDetail for nil actual")
	}
	if !inner.Failed() {
		t.Error("expected inner testing.T to fail for nil actual")
	}
}

func TestMatchTriageDetail_MustNotBe(t *testing.T) {
	t.Run("forbidden_triggers_fail", func(t *testing.T) {
		inner := &testing.T{}
		actual := &ActualTriageResult{Importance: "LOW", Category: "SOCIAL"}
		expected := &TriageExpectation{
			Importance: &OneOfMatcher{MustNotBe: []string{"LOW"}},
		}
		details := matchTriageDetail(inner, expected, actual)
		var failDetail *MatchDetail
		for i := range details {
			if !details[i].Pass {
				failDetail = &details[i]
			}
		}
		if failDetail == nil {
			t.Error("expected failing MatchDetail for LOW in must_not_be [LOW]")
		}
		if !inner.Failed() {
			t.Error("expected inner testing.T to fail")
		}
	})

	t.Run("non_forbidden_passes", func(t *testing.T) {
		actual := &ActualTriageResult{Importance: "HIGH", Category: "INCIDENT"}
		expected := &TriageExpectation{
			Importance: &OneOfMatcher{MustNotBe: []string{"LOW", "MEDIUM"}},
		}
		details := matchTriageDetail(t, expected, actual)
		for _, d := range details {
			if !d.Pass {
				t.Errorf("expected pass: HIGH is not in must_not_be [LOW, MEDIUM]: check=%q", d.Check)
			}
		}
	})
}

// --- matchPeopleDetail tests ---

func TestMatchPeopleDetail_Pass_MinCount(t *testing.T) {
	minCount := 2
	actual := []ActualPerson{
		{CanonicalName: "Daniel Chen"},
		{CanonicalName: "Rob Taylor"},
	}
	expected := &PeopleExpectation{MinCount: &minCount}
	details := matchPeopleDetail(t, expected, actual)
	for _, d := range details {
		if !d.Pass {
			t.Errorf("expected pass for min_count 2 with 2 people: %s", d.Check)
		}
	}
}

func TestMatchPeopleDetail_Fail_MinCount(t *testing.T) {
	minCount := 3
	actual := []ActualPerson{
		{CanonicalName: "Daniel Chen"},
	}
	expected := &PeopleExpectation{MinCount: &minCount}
	inner := &testing.T{}
	details := matchPeopleDetail(inner, expected, actual)
	if len(details) == 0 || details[0].Pass {
		t.Error("expected failing MatchDetail for min_count 3 with 1 person")
	}
	if !inner.Failed() {
		t.Error("expected inner testing.T to fail")
	}
}

func TestMatchPeopleDetail_MustFind_Pass(t *testing.T) {
	actual := []ActualPerson{
		{CanonicalName: "Daniel Chen"},
		{CanonicalName: "Rob Taylor"},
	}
	expected := &PeopleExpectation{
		MustFind: []PersonMatcher{
			{NameContains: "Daniel"},
			{NameContains: "Rob"},
		},
	}
	details := matchPeopleDetail(t, expected, actual)
	for _, d := range details {
		if !d.Pass {
			t.Errorf("expected pass for must_find: %s", d.Check)
		}
	}
}

func TestMatchPeopleDetail_MustFind_Fail(t *testing.T) {
	actual := []ActualPerson{
		{CanonicalName: "Alice Smith"},
	}
	expected := &PeopleExpectation{
		MustFind: []PersonMatcher{
			{NameContains: "Daniel"},
		},
	}
	inner := &testing.T{}
	details := matchPeopleDetail(inner, expected, actual)
	found := false
	for _, d := range details {
		if !d.Pass {
			found = true
		}
	}
	if !found {
		t.Error("expected failing MatchDetail for must_find Daniel when only Alice is present")
	}
}

func TestMatchPeopleDetail_MustNotFind_FalsePositive(t *testing.T) {
	actual := []ActualPerson{
		{CanonicalName: "Acme Corp"},
	}
	expected := &PeopleExpectation{
		MustNotFind: []PersonMatcher{
			{NameContains: "Acme Corp"},
		},
	}
	inner := &testing.T{}
	details := matchPeopleDetail(inner, expected, actual)
	found := false
	for _, d := range details {
		if !d.Pass {
			found = true
		}
	}
	if !found {
		t.Error("expected failing MatchDetail when must_not_find person is present")
	}
}

// --- matchAssertionsDetail tests ---

func TestMatchAssertionsDetail_Pass(t *testing.T) {
	minCount := 2
	actual := []ActualAssertion{
		{AssertionType: "risk", Description: "API Gateway degradation", Confidence: 0.9},
		{AssertionType: "action", Description: "Join incident war room", Confidence: 0.85},
	}
	expected := &AssertionExpectation{
		MinCount: &minCount,
		MustFind: []AssertionMatcher{
			{Type: "risk", DescriptionContains: "API"},
			{Type: "action", DescriptionContains: "incident"},
		},
	}
	details := matchAssertionsDetail(t, expected, actual)
	for _, d := range details {
		if !d.Pass {
			t.Errorf("expected pass: %s (expected=%q actual=%q)", d.Check, d.Expected, d.Actual)
		}
	}
}

func TestMatchAssertionsDetail_Fail_MaxCount(t *testing.T) {
	maxCount := 1
	actual := []ActualAssertion{
		{AssertionType: "risk", Description: "some risk"},
		{AssertionType: "action", Description: "some action"},
	}
	expected := &AssertionExpectation{MaxCount: &maxCount}
	inner := &testing.T{}
	details := matchAssertionsDetail(inner, expected, actual)
	found := false
	for _, d := range details {
		if !d.Pass {
			found = true
		}
	}
	if !found {
		t.Error("expected failing MatchDetail for max_count 1 with 2 assertions")
	}
}

func TestMatchAssertionsDetail_MustNotFind_Pass(t *testing.T) {
	// Negative test: LOW priority email should have no business assertions
	maxCount := 1
	actual := []ActualAssertion{}
	expected := &AssertionExpectation{
		MaxCount: &maxCount,
		MustNotFind: []AssertionMatcher{
			{Type: "risk"},
			{Type: "decision"},
		},
	}
	details := matchAssertionsDetail(t, expected, actual)
	for _, d := range details {
		if !d.Pass {
			t.Errorf("expected pass (no forbidden assertions present): %s", d.Check)
		}
	}
}

// --- matchProjectsDetail tests ---

func TestMatchProjectsDetail_Pass(t *testing.T) {
	actual := []ActualProject{
		{Name: "Project Alpha"},
		{Name: "ML Tiger Team"},
	}
	expected := &ProjectsExpectation{
		MustFind: []ProjectMatcher{
			{NameContains: "Alpha"},
		},
	}
	details := matchProjectsDetail(t, expected, actual)
	for _, d := range details {
		if !d.Pass {
			t.Errorf("expected pass: %s", d.Check)
		}
	}
}

func TestMatchProjectsDetail_Fail_MustFind(t *testing.T) {
	actual := []ActualProject{
		{Name: "Other Project"},
	}
	expected := &ProjectsExpectation{
		MustFind: []ProjectMatcher{
			{NameContains: "Alpha"},
		},
	}
	inner := &testing.T{}
	details := matchProjectsDetail(inner, expected, actual)
	found := false
	for _, d := range details {
		if !d.Pass {
			found = true
		}
	}
	if !found {
		t.Error("expected failing MatchDetail when project Alpha is not present")
	}
}

// --- MatchStandardExtract backward-compat: originals still call t.Error ---

func TestMatchTriage_BackwardCompat_StillErrors(t *testing.T) {
	inner := &testing.T{}
	actual := &ActualTriageResult{Importance: "LOW", Category: "SOCIAL"}
	expected := &TriageExpectation{
		Importance: &OneOfMatcher{OneOf: []string{"HIGH", "CRITICAL"}},
	}
	MatchTriage(inner, expected, actual)
	if !inner.Failed() {
		t.Error("MatchTriage should still call t.Error for backward compatibility")
	}
}

func TestMatchAssertions_BackwardCompat_StillErrors(t *testing.T) {
	minCount := 3
	inner := &testing.T{}
	actual := []ActualAssertion{}
	expected := &AssertionExpectation{MinCount: &minCount}
	MatchAssertions(inner, expected, actual)
	if !inner.Failed() {
		t.Error("MatchAssertions should still call t.Error for backward compatibility")
	}
}
