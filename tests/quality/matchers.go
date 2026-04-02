//go:build quality

package quality

import (
	"fmt"
	"strings"
	"testing"
)

// MatchResult tracks the outcome of a single match operation.
type MatchResult struct {
	Pass    bool
	Message string
}

// --- Triage matchers ---

// MatchTriage checks triage expectations against actual triage result.
func MatchTriage(t *testing.T, expected *TriageExpectation, actual *ActualTriageResult) {
	t.Helper()
	matchTriageDetail(t, expected, actual)
}

// matchTriageDetail checks triage expectations and returns []MatchDetail.
// Also calls t.Error on failures for backward compatibility.
func matchTriageDetail(t *testing.T, expected *TriageExpectation, actual *ActualTriageResult) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected == nil {
		return details
	}

	if actual == nil {
		t.Error("triage: expected triage result but got nil")
		details = append(details, MatchDetail{Check: "triage.exists", Pass: false, Message: "expected triage result but got nil"})
		return details
	}

	if expected.Importance != nil {
		details = append(details, matchOneOfDetail(t, "triage.importance", actual.Importance, expected.Importance)...)
	}

	if expected.Category != nil {
		details = append(details, matchOneOfDetail(t, "triage.category", actual.Category, expected.Category)...)
	}

	return details
}

// --- People matchers ---

// MatchPeople checks people expectations against actual extracted people.
func MatchPeople(t *testing.T, expected *PeopleExpectation, actual []ActualPerson) {
	t.Helper()
	matchPeopleDetail(t, expected, actual)
}

// matchPeopleDetail checks people expectations and returns []MatchDetail.
// Also calls t.Error on failures for backward compatibility.
func matchPeopleDetail(t *testing.T, expected *PeopleExpectation, actual []ActualPerson) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected == nil {
		return details
	}

	t.Logf("  people: found %d total", len(actual))
	for _, p := range actual {
		autoTag := ""
		if p.AutoCreated {
			autoTag = " [auto-created]"
		}
		titleTag := ""
		if p.Title != "" {
			titleTag = fmt.Sprintf(" (%s)", p.Title)
		}
		t.Logf("    - %s%s%s", p.CanonicalName, titleTag, autoTag)
	}

	if expected.MinCount != nil {
		pass := len(actual) >= *expected.MinCount
		if !pass {
			t.Errorf("people: expected min_count %d, got %d", *expected.MinCount, len(actual))
		}
		details = append(details, MatchDetail{
			Check:    "people.min_count",
			Pass:     pass,
			Expected: fmt.Sprintf(">= %d", *expected.MinCount),
			Actual:   fmt.Sprintf("%d", len(actual)),
		})
	}

	if expected.MaxCount != nil {
		pass := len(actual) <= *expected.MaxCount
		if !pass {
			t.Errorf("people: expected max_count %d, got %d", *expected.MaxCount, len(actual))
		}
		details = append(details, MatchDetail{
			Check:    "people.max_count",
			Pass:     pass,
			Expected: fmt.Sprintf("<= %d", *expected.MaxCount),
			Actual:   fmt.Sprintf("%d", len(actual)),
		})
	}

	for _, mf := range expected.MustFind {
		found := personMatches(actual, mf)
		if !found {
			t.Errorf("people.must_find: no person matching %s", describePersonMatcher(mf))
		}
		details = append(details, MatchDetail{
			Check:    "people.must_find." + describePersonMatcher(mf),
			Pass:     found,
			Expected: "person matching " + describePersonMatcher(mf),
			Actual:   fmt.Sprintf("%d people found", len(actual)),
		})
	}

	for _, mnf := range expected.MustNotFind {
		found := personMatches(actual, mnf)
		if found {
			t.Errorf("people.must_not_find: found person matching %s (false positive)", describePersonMatcher(mnf))
		}
		details = append(details, MatchDetail{
			Check:    "people.must_not_find." + describePersonMatcher(mnf),
			Pass:     !found,
			Expected: "no person matching " + describePersonMatcher(mnf),
			Actual:   fmt.Sprintf("%d people found", len(actual)),
		})
	}

	return details
}

// personMatches checks if any actual person matches the given matcher.
func personMatches(people []ActualPerson, matcher PersonMatcher) bool {
	for _, p := range people {
		if matcher.NameContains != "" && !containsCI(p.CanonicalName, matcher.NameContains) {
			continue
		}
		if matcher.RoleContains != "" && !containsCI(p.Title, matcher.RoleContains) {
			continue
		}
		return true
	}
	return false
}

func describePersonMatcher(m PersonMatcher) string {
	parts := []string{}
	if m.NameContains != "" {
		parts = append(parts, fmt.Sprintf("name_contains=%q", m.NameContains))
	}
	if m.RoleContains != "" {
		parts = append(parts, fmt.Sprintf("role_contains=%q", m.RoleContains))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// --- Assertion matchers ---

// MatchAssertions checks assertion expectations against actual extracted assertions.
func MatchAssertions(t *testing.T, expected *AssertionExpectation, actual []ActualAssertion) {
	t.Helper()
	matchAssertionsDetail(t, expected, actual)
}

// matchAssertionsDetail checks assertion expectations and returns []MatchDetail.
// Also calls t.Error on failures for backward compatibility.
func matchAssertionsDetail(t *testing.T, expected *AssertionExpectation, actual []ActualAssertion) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected == nil {
		return details
	}

	t.Logf("  assertions: found %d total", len(actual))
	for _, a := range actual {
		t.Logf("    - [%s] %s (confidence: %.2f)", a.AssertionType, truncate(a.Description, 80), a.Confidence)
	}

	if expected.MinCount != nil {
		pass := len(actual) >= *expected.MinCount
		if !pass {
			t.Errorf("assertions: expected min_count %d, got %d", *expected.MinCount, len(actual))
		}
		details = append(details, MatchDetail{
			Check:    "assertions.min_count",
			Pass:     pass,
			Expected: fmt.Sprintf(">= %d", *expected.MinCount),
			Actual:   fmt.Sprintf("%d", len(actual)),
		})
	}

	if expected.MaxCount != nil {
		pass := len(actual) <= *expected.MaxCount
		if !pass {
			t.Errorf("assertions: expected max_count %d, got %d", *expected.MaxCount, len(actual))
		}
		details = append(details, MatchDetail{
			Check:    "assertions.max_count",
			Pass:     pass,
			Expected: fmt.Sprintf("<= %d", *expected.MaxCount),
			Actual:   fmt.Sprintf("%d", len(actual)),
		})
	}

	for _, mf := range expected.MustFind {
		found := assertionMatches(actual, mf)
		if !found {
			t.Errorf("assertions.must_find: no assertion matching %s", describeAssertionMatcher(mf))
		}
		details = append(details, MatchDetail{
			Check:    "assertions.must_find." + describeAssertionMatcher(mf),
			Pass:     found,
			Expected: "assertion matching " + describeAssertionMatcher(mf),
			Actual:   fmt.Sprintf("%d assertions found", len(actual)),
		})
	}

	for _, mnf := range expected.MustNotFind {
		found := assertionMatches(actual, mnf)
		if found {
			t.Errorf("assertions.must_not_find: found assertion matching %s (false positive)", describeAssertionMatcher(mnf))
		}
		details = append(details, MatchDetail{
			Check:    "assertions.must_not_find." + describeAssertionMatcher(mnf),
			Pass:     !found,
			Expected: "no assertion matching " + describeAssertionMatcher(mnf),
			Actual:   fmt.Sprintf("%d assertions found", len(actual)),
		})
	}

	return details
}

// assertionMatches checks if any actual assertion matches the given matcher.
func assertionMatches(assertions []ActualAssertion, matcher AssertionMatcher) bool {
	for _, a := range assertions {
		if matcher.Type != "" && !strings.EqualFold(a.AssertionType, matcher.Type) {
			continue
		}
		if matcher.DescriptionContains != "" && !containsCI(a.Description, matcher.DescriptionContains) {
			continue
		}
		if matcher.ConfidenceMin != nil && a.Confidence < *matcher.ConfidenceMin {
			continue
		}
		return true
	}
	return false
}

func describeAssertionMatcher(m AssertionMatcher) string {
	parts := []string{}
	if m.Type != "" {
		parts = append(parts, fmt.Sprintf("type=%q", m.Type))
	}
	if m.DescriptionContains != "" {
		parts = append(parts, fmt.Sprintf("description_contains=%q", m.DescriptionContains))
	}
	if m.ConfidenceMin != nil {
		parts = append(parts, fmt.Sprintf("confidence_min=%.2f", *m.ConfidenceMin))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// --- Project matchers ---

// MatchProjects checks project expectations against actual linked projects.
func MatchProjects(t *testing.T, expected *ProjectsExpectation, actual []ActualProject) {
	t.Helper()
	matchProjectsDetail(t, expected, actual)
}

// matchProjectsDetail checks project expectations and returns []MatchDetail.
// Also calls t.Error on failures for backward compatibility.
func matchProjectsDetail(t *testing.T, expected *ProjectsExpectation, actual []ActualProject) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if expected == nil {
		return details
	}

	t.Logf("  projects: found %d total", len(actual))
	for _, p := range actual {
		t.Logf("    - %s", p.Name)
	}

	if expected.MinCount != nil {
		pass := len(actual) >= *expected.MinCount
		if !pass {
			t.Errorf("projects: expected min_count %d, got %d", *expected.MinCount, len(actual))
		}
		details = append(details, MatchDetail{
			Check:    "projects.min_count",
			Pass:     pass,
			Expected: fmt.Sprintf(">= %d", *expected.MinCount),
			Actual:   fmt.Sprintf("%d", len(actual)),
		})
	}

	if expected.MaxCount != nil {
		pass := len(actual) <= *expected.MaxCount
		if !pass {
			t.Errorf("projects: expected max_count %d, got %d", *expected.MaxCount, len(actual))
		}
		details = append(details, MatchDetail{
			Check:    "projects.max_count",
			Pass:     pass,
			Expected: fmt.Sprintf("<= %d", *expected.MaxCount),
			Actual:   fmt.Sprintf("%d", len(actual)),
		})
	}

	for _, mf := range expected.MustFind {
		found := projectMatches(actual, mf)
		if !found {
			t.Errorf("projects.must_find: no project matching %s", describeProjectMatcher(mf))
		}
		details = append(details, MatchDetail{
			Check:    "projects.must_find." + describeProjectMatcher(mf),
			Pass:     found,
			Expected: "project matching " + describeProjectMatcher(mf),
			Actual:   fmt.Sprintf("%d projects found", len(actual)),
		})
	}

	for _, mnf := range expected.MustNotFind {
		found := projectMatches(actual, mnf)
		if found {
			t.Errorf("projects.must_not_find: found project matching %s (false positive)", describeProjectMatcher(mnf))
		}
		details = append(details, MatchDetail{
			Check:    "projects.must_not_find." + describeProjectMatcher(mnf),
			Pass:     !found,
			Expected: "no project matching " + describeProjectMatcher(mnf),
			Actual:   fmt.Sprintf("%d projects found", len(actual)),
		})
	}

	return details
}

// projectMatches checks if any actual project matches the given matcher.
func projectMatches(projects []ActualProject, matcher ProjectMatcher) bool {
	for _, p := range projects {
		if matcher.NameContains != "" && !containsCI(p.Name, matcher.NameContains) {
			continue
		}
		return true
	}
	return false
}

func describeProjectMatcher(m ProjectMatcher) string {
	return fmt.Sprintf("{name_contains=%q}", m.NameContains)
}

// --- Pipeline matchers ---

// MatchPipelineStages checks that all required pipeline stages completed.
func MatchPipelineStages(t *testing.T, env *QualityEnv, sourceID int64, expected PipelineExpectation) {
	t.Helper()

	for _, stage := range expected.MustComplete {
		assertStageCompleted(t, env, sourceID, stage)
	}
}

// --- Standard extract matcher ---

// MatchStandardExtract queries assertions, people, and projects for sourceID,
// runs all four standard matchers, and returns combined []MatchDetail for L2 scoring.
// Also calls t.Error on each failure for test reporting.
func MatchStandardExtract(t *testing.T, env *QualityEnv, sourceID int64, exp *GoldenExpectation) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if exp.Triage != nil {
		triageResult, err := getTriageResult(env, sourceID)
		if err != nil {
			t.Errorf("standard_extract.triage: %v", err)
			details = append(details, MatchDetail{Check: "triage.exists", Pass: false, Message: err.Error()})
		} else {
			details = append(details, matchTriageDetail(t, exp.Triage, triageResult)...)
		}
	}

	if exp.People != nil {
		people, err := getAutoCreatedPeopleForSource(env, sourceID)
		if err != nil {
			t.Errorf("standard_extract.people: %v", err)
			details = append(details, MatchDetail{Check: "people.exists", Pass: false, Message: err.Error()})
		} else {
			details = append(details, matchPeopleDetail(t, exp.People, people)...)
		}
	}

	if exp.Assertions != nil {
		assertions, err := getAssertionsForSource(env, sourceID)
		if err != nil {
			t.Errorf("standard_extract.assertions: %v", err)
			details = append(details, MatchDetail{Check: "assertions.exists", Pass: false, Message: err.Error()})
		} else {
			details = append(details, matchAssertionsDetail(t, exp.Assertions, assertions)...)
		}
	}

	if exp.Projects != nil {
		projects, err := getProjectsForSource(env, sourceID)
		if err != nil {
			t.Errorf("standard_extract.projects: %v", err)
			details = append(details, MatchDetail{Check: "projects.exists", Pass: false, Message: err.Error()})
		} else {
			details = append(details, matchProjectsDetail(t, exp.Projects, projects)...)
		}
	}

	return details
}

// --- Utility functions ---

// matchOneOfDetail checks one_of and must_not_be constraints and returns []MatchDetail.
// Also calls t.Error on failures.
func matchOneOfDetail(t *testing.T, field string, actual string, matcher *OneOfMatcher) []MatchDetail {
	t.Helper()
	var details []MatchDetail

	if matcher == nil {
		return details
	}

	if len(matcher.MustNotBe) > 0 {
		forbidden := false
		for _, f := range matcher.MustNotBe {
			if strings.EqualFold(actual, f) {
				forbidden = true
				break
			}
		}
		if forbidden {
			t.Errorf("%s: got %q which is in must_not_be %v", field, actual, matcher.MustNotBe)
		}
		details = append(details, MatchDetail{
			Check:    field + ".must_not_be",
			Pass:     !forbidden,
			Expected: fmt.Sprintf("not one of %v", matcher.MustNotBe),
			Actual:   actual,
		})
	}

	if len(matcher.OneOf) > 0 {
		found := false
		for _, opt := range matcher.OneOf {
			if strings.EqualFold(actual, opt) {
				found = true
				break
			}
		}
		if found {
			t.Logf("  %s: %s (matched)", field, actual)
		} else {
			t.Errorf("%s: got %q, expected one_of %v", field, actual, matcher.OneOf)
		}
		details = append(details, MatchDetail{
			Check:    field + ".one_of",
			Pass:     found,
			Expected: fmt.Sprintf("one_of %v", matcher.OneOf),
			Actual:   actual,
		})
	}

	return details
}

// matchOneOf checks that a value satisfies the OneOfMatcher constraints (case-insensitive).
// Enforces both one_of (must match) and must_not_be (must not match).
func matchOneOf(t *testing.T, field string, actual string, matcher *OneOfMatcher) {
	t.Helper()

	if matcher == nil {
		return
	}

	for _, forbidden := range matcher.MustNotBe {
		if strings.EqualFold(actual, forbidden) {
			t.Errorf("%s: got %q which is in must_not_be %v", field, actual, matcher.MustNotBe)
			return
		}
	}

	if len(matcher.OneOf) > 0 {
		for _, opt := range matcher.OneOf {
			if strings.EqualFold(actual, opt) {
				t.Logf("  %s: %s (matched)", field, actual)
				return
			}
		}
		t.Errorf("%s: got %q, expected one_of %v", field, actual, matcher.OneOf)
	}
}

// containsCI performs case-insensitive substring match.
func containsCI(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
