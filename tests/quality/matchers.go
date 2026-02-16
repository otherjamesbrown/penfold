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

	if expected == nil {
		return
	}

	if actual == nil {
		t.Error("triage: expected triage result but got nil")
		return
	}

	if expected.Importance != nil {
		matchOneOf(t, "triage.importance", actual.Importance, expected.Importance.OneOf)
	}

	if expected.Category != nil {
		matchOneOf(t, "triage.category", actual.Category, expected.Category.OneOf)
	}
}

// --- People matchers ---

// MatchPeople checks people expectations against actual extracted people.
func MatchPeople(t *testing.T, expected *PeopleExpectation, actual []ActualPerson) {
	t.Helper()

	if expected == nil {
		return
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
		if len(actual) < *expected.MinCount {
			t.Errorf("people: expected min_count %d, got %d", *expected.MinCount, len(actual))
		}
	}

	if expected.MaxCount != nil {
		if len(actual) > *expected.MaxCount {
			t.Errorf("people: expected max_count %d, got %d", *expected.MaxCount, len(actual))
		}
	}

	for _, mf := range expected.MustFind {
		if !personMatches(actual, mf) {
			t.Errorf("people.must_find: no person matching %s", describePersonMatcher(mf))
		}
	}

	for _, mnf := range expected.MustNotFind {
		if personMatches(actual, mnf) {
			t.Errorf("people.must_not_find: found person matching %s (false positive)", describePersonMatcher(mnf))
		}
	}
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

	if expected == nil {
		return
	}

	t.Logf("  assertions: found %d total", len(actual))
	for _, a := range actual {
		t.Logf("    - [%s] %s (confidence: %.2f)", a.AssertionType, truncate(a.Description, 80), a.Confidence)
	}

	if expected.MinCount != nil {
		if len(actual) < *expected.MinCount {
			t.Errorf("assertions: expected min_count %d, got %d", *expected.MinCount, len(actual))
		}
	}

	if expected.MaxCount != nil {
		if len(actual) > *expected.MaxCount {
			t.Errorf("assertions: expected max_count %d, got %d", *expected.MaxCount, len(actual))
		}
	}

	for _, mf := range expected.MustFind {
		if !assertionMatches(actual, mf) {
			t.Errorf("assertions.must_find: no assertion matching %s", describeAssertionMatcher(mf))
		}
	}

	for _, mnf := range expected.MustNotFind {
		if assertionMatches(actual, mnf) {
			t.Errorf("assertions.must_not_find: found assertion matching %s (false positive)", describeAssertionMatcher(mnf))
		}
	}
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

	if expected == nil {
		return
	}

	t.Logf("  projects: found %d total", len(actual))
	for _, p := range actual {
		t.Logf("    - %s", p.Name)
	}

	if expected.MinCount != nil {
		if len(actual) < *expected.MinCount {
			t.Errorf("projects: expected min_count %d, got %d", *expected.MinCount, len(actual))
		}
	}

	if expected.MaxCount != nil {
		if len(actual) > *expected.MaxCount {
			t.Errorf("projects: expected max_count %d, got %d", *expected.MaxCount, len(actual))
		}
	}

	for _, mf := range expected.MustFind {
		if !projectMatches(actual, mf) {
			t.Errorf("projects.must_find: no project matching %s", describeProjectMatcher(mf))
		}
	}

	for _, mnf := range expected.MustNotFind {
		if projectMatches(actual, mnf) {
			t.Errorf("projects.must_not_find: found project matching %s (false positive)", describeProjectMatcher(mnf))
		}
	}
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

// --- Utility functions ---

// matchOneOf checks that a value is one of the expected options (case-insensitive).
func matchOneOf(t *testing.T, field string, actual string, options []string) {
	t.Helper()

	for _, opt := range options {
		if strings.EqualFold(actual, opt) {
			t.Logf("  %s: %s (matched)", field, actual)
			return
		}
	}

	t.Errorf("%s: got %q, expected one_of %v", field, actual, options)
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
