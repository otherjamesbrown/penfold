package workflows

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextPackage_FormatForPrompt_Nil(t *testing.T) {
	var cp *ContextPackage
	assert.Equal(t, "", cp.FormatForPrompt())
}

func TestContextPackage_FormatForPrompt_Empty(t *testing.T) {
	cp := &ContextPackage{}
	assert.Equal(t, "", cp.FormatForPrompt())
}

func TestContextPackage_FormatForPrompt_GlossaryOnly(t *testing.T) {
	cp := &ContextPackage{
		GlossaryTerms: []ContextGlossaryTerm{
			{Term: "MTC", Definition: "Master Test Controller"},
			{Term: "CLIC", Definition: "Client Lifecycle Integration Component"},
		},
	}
	result := cp.FormatForPrompt()
	assert.Contains(t, result, "### Glossary")
	assert.Contains(t, result, "- **MTC**: Master Test Controller")
	assert.Contains(t, result, "- **CLIC**: Client Lifecycle Integration Component")
	assert.NotContains(t, result, "### Participant Context")
}

func TestContextPackage_FormatForPrompt_ParticipantContextRemoved(t *testing.T) {
	// Participant Context was removed from FormatForPrompt (pf-9c1485).
	// People context is now provided via the enriched Entities section.
	cp := &ContextPackage{
		ParticipantContext: []ResolvedPerson{
			{Name: "James Brown", Department: "Engineering", Role: "Sender", IsPrimaryUser: true},
			{Name: "Jane Doe", Role: "To"},
		},
	}
	result := cp.FormatForPrompt()
	assert.NotContains(t, result, "### Participant Context")
	assert.NotContains(t, result, "James Brown")
}

func TestContextPackage_FormatForPrompt_Assertions(t *testing.T) {
	cp := &ContextPackage{
		ActiveRisks: []ContextAssertion{
			{Subject: "Project Alpha", Predicate: "has risk", Object: "budget overrun"},
		},
		OpenActions: []ContextAssertion{
			{Subject: "James", Predicate: "needs to", Object: "review proposal"},
		},
		RecentDecisions: []ContextAssertion{
			{Subject: "Team", Predicate: "decided", Object: "use Go for backend"},
		},
	}
	result := cp.FormatForPrompt()
	assert.Contains(t, result, "### Active Risks")
	assert.Contains(t, result, "- Project Alpha has risk budget overrun")
	assert.Contains(t, result, "### Open Actions")
	assert.Contains(t, result, "- James needs to review proposal")
	assert.Contains(t, result, "### Recent Decisions")
	assert.Contains(t, result, "- Team decided use Go for backend")
}

func TestContextPackage_FormatForPrompt_ProductEvents(t *testing.T) {
	cp := &ContextPackage{
		ProductEvents: []ContextProductEvent{
			{EventType: "RELEASE", Description: "v2.0 shipped", Timestamp: "2026-01-15"},
			{EventType: "INCIDENT", Description: "API outage"},
		},
	}
	result := cp.FormatForPrompt()
	assert.Contains(t, result, "### Product Events")
	assert.Contains(t, result, "- [RELEASE] v2.0 shipped (2026-01-15)")
	assert.Contains(t, result, "- [INCIDENT] API outage")
	// No timestamp suffix when empty
	assert.NotContains(t, result, "API outage ()")
}

func TestContextPackage_FormatForPrompt_AllSections(t *testing.T) {
	cp := &ContextPackage{
		GlossaryTerms:      []ContextGlossaryTerm{{Term: "A", Definition: "B"}},
		ParticipantContext:  []ResolvedPerson{{Name: "Alice"}}, // present but no longer rendered
		ActiveRisks:        []ContextAssertion{{Subject: "X", Predicate: "Y", Object: "Z"}},
		OpenActions:        []ContextAssertion{{Subject: "P", Predicate: "Q", Object: "R"}},
		RecentDecisions:    []ContextAssertion{{Subject: "D", Predicate: "E", Object: "F"}},
		ProductEvents:      []ContextProductEvent{{EventType: "T", Description: "D"}},
	}
	result := cp.FormatForPrompt()

	// Participant Context no longer rendered — 5 sections remain
	sections := strings.Split(result, "\n\n")
	assert.Len(t, sections, 5)
	assert.True(t, strings.HasPrefix(sections[0], "### Glossary"))
	assert.True(t, strings.HasPrefix(sections[1], "### Active Risks"))
	assert.True(t, strings.HasPrefix(sections[2], "### Open Actions"))
	assert.True(t, strings.HasPrefix(sections[3], "### Recent Decisions"))
	assert.True(t, strings.HasPrefix(sections[4], "### Product Events"))
	assert.NotContains(t, result, "### Participant Context")
}

// selectConfirmedProjectID tests (pf-de3670)

func TestSelectConfirmedProjectID_NoResolved(t *testing.T) {
	result := selectConfirmedProjectID(nil, []TopicMappingOutput{{RelatedProject: "MTC", Confidence: 0.9}})
	assert.Nil(t, result)
}

func TestSelectConfirmedProjectID_NoTopicMappings(t *testing.T) {
	id := int64(42)
	result := selectConfirmedProjectID(
		[]ResolvedProject{{Name: "MTC", ProjectID: &id}},
		nil,
	)
	assert.Nil(t, result, "should not tag when analysis found no project connections")
}

func TestSelectConfirmedProjectID_ConfirmedProject(t *testing.T) {
	id := int64(42)
	result := selectConfirmedProjectID(
		[]ResolvedProject{{Name: "MTC", ProjectID: &id}},
		[]TopicMappingOutput{{RelatedProject: "MTC", Confidence: 0.8}},
	)
	assert.NotNil(t, result)
	assert.Equal(t, int64(42), *result)
}

func TestSelectConfirmedProjectID_CaseInsensitive(t *testing.T) {
	id := int64(42)
	result := selectConfirmedProjectID(
		[]ResolvedProject{{Name: "MTC", ProjectID: &id}},
		[]TopicMappingOutput{{RelatedProject: "mtc", Confidence: 0.7}},
	)
	assert.NotNil(t, result)
	assert.Equal(t, int64(42), *result)
}

func TestSelectConfirmedProjectID_LowConfidence(t *testing.T) {
	id := int64(42)
	result := selectConfirmedProjectID(
		[]ResolvedProject{{Name: "MTC", ProjectID: &id}},
		[]TopicMappingOutput{{RelatedProject: "MTC", Confidence: 0.3}},
	)
	assert.Nil(t, result, "should not tag when confidence is below threshold")
}

func TestSelectConfirmedProjectID_UnrelatedProject(t *testing.T) {
	id := int64(42)
	result := selectConfirmedProjectID(
		[]ResolvedProject{{Name: "MTC", ProjectID: &id}},
		[]TopicMappingOutput{{RelatedProject: "Oslo", Confidence: 0.9}},
	)
	assert.Nil(t, result, "should not tag when analysis references a different project")
}

func TestSelectConfirmedProjectID_MultipleProjects_FirstConfirmed(t *testing.T) {
	id1 := int64(42)
	id2 := int64(99)
	result := selectConfirmedProjectID(
		[]ResolvedProject{
			{Name: "MTC", ProjectID: &id1},
			{Name: "Oslo", ProjectID: &id2},
		},
		[]TopicMappingOutput{
			{RelatedProject: "Oslo", Confidence: 0.9},
		},
	)
	assert.NotNil(t, result)
	assert.Equal(t, int64(99), *result, "should pick the confirmed project, not just the first")
}

func TestSelectConfirmedProjectID_EmptyRelatedProject(t *testing.T) {
	id := int64(42)
	result := selectConfirmedProjectID(
		[]ResolvedProject{{Name: "MTC", ProjectID: &id}},
		[]TopicMappingOutput{{RelatedProject: "", Confidence: 0.9}},
	)
	assert.Nil(t, result, "should ignore topic mappings with empty project name")
}

