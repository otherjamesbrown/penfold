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

func TestContextPackage_FormatForPrompt_ParticipantContext(t *testing.T) {
	cp := &ContextPackage{
		ParticipantContext: []ResolvedPerson{
			{Name: "James Brown", Department: "Engineering", Role: "Sender", IsPrimaryUser: true},
			{Name: "Jane Doe", Role: "To"},
			{Name: "Bob Smith", Department: "Cloud Networking", Role: "CC"},
			{Name: "Alice Chen"},
		},
	}
	result := cp.FormatForPrompt()
	assert.Contains(t, result, "### Participant Context")
	assert.Contains(t, result, "- James Brown — Engineering. Sender. [Primary user]")
	assert.Contains(t, result, "- Jane Doe — To")
	assert.Contains(t, result, "- Bob Smith — Cloud Networking. CC")
	assert.Contains(t, result, "- Alice Chen")
	// Should NOT contain the old format with parentheses
	assert.NotContains(t, result, "(Sender)")
}

func TestContextPackage_FormatForPrompt_ParticipantDepartmentOnly(t *testing.T) {
	cp := &ContextPackage{
		ParticipantContext: []ResolvedPerson{
			{Name: "Tim Dunn", Department: "Cloud Networking"},
		},
	}
	result := cp.FormatForPrompt()
	assert.Contains(t, result, "- Tim Dunn — Cloud Networking")
}

func TestContextPackage_FormatForPrompt_ParticipantPrimaryUserOnly(t *testing.T) {
	cp := &ContextPackage{
		ParticipantContext: []ResolvedPerson{
			{Name: "James Brown", IsPrimaryUser: true},
		},
	}
	result := cp.FormatForPrompt()
	assert.Contains(t, result, "- James Brown — [Primary user]")
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
		ParticipantContext:  []ResolvedPerson{{Name: "Alice"}},
		ActiveRisks:        []ContextAssertion{{Subject: "X", Predicate: "Y", Object: "Z"}},
		OpenActions:        []ContextAssertion{{Subject: "P", Predicate: "Q", Object: "R"}},
		RecentDecisions:    []ContextAssertion{{Subject: "D", Predicate: "E", Object: "F"}},
		ProductEvents:      []ContextProductEvent{{EventType: "T", Description: "D"}},
	}
	result := cp.FormatForPrompt()

	// All sections present, separated by double newlines
	sections := strings.Split(result, "\n\n")
	assert.Len(t, sections, 6)
	assert.True(t, strings.HasPrefix(sections[0], "### Glossary"))
	assert.True(t, strings.HasPrefix(sections[1], "### Participant Context"))
	assert.True(t, strings.HasPrefix(sections[2], "### Active Risks"))
	assert.True(t, strings.HasPrefix(sections[3], "### Open Actions"))
	assert.True(t, strings.HasPrefix(sections[4], "### Recent Decisions"))
	assert.True(t, strings.HasPrefix(sections[5], "### Product Events"))
}

func TestFormatContextPackage_NilOutput(t *testing.T) {
	assert.Equal(t, "", formatContextPackage(nil))
}

func TestFormatContextPackage_NilPackage(t *testing.T) {
	output := &BuildContextOutput{}
	assert.Equal(t, "", formatContextPackage(output))
}

func TestFormatContextPackage_WithPackage(t *testing.T) {
	output := &BuildContextOutput{
		ContextPackage: &ContextPackage{
			GlossaryTerms: []ContextGlossaryTerm{
				{Term: "API", Definition: "Application Programming Interface"},
			},
		},
	}
	result := formatContextPackage(output)
	assert.Contains(t, result, "### Glossary")
	assert.Contains(t, result, "**API**")
}
