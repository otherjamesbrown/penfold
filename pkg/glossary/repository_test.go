package glossary

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWordBoundaryPattern(t *testing.T) {
	tests := []struct {
		name    string
		word    string
		text    string
		matches bool
	}{
		{"exact match", "TER", "What is TER?", true},
		{"start of text", "TER", "TER is a meeting", true},
		{"end of text", "TER", "I attended the TER", true},
		{"middle of text", "TER", "The TER meeting was good", true},
		{"case insensitive", "TER", "the ter meeting", true},
		{"no match - partial word", "TER", "TERMINAL is not TER", true}, // matches TER at end
		{"no match - different word", "TER", "This is TEST", false},
		{"with punctuation", "TER", "Attended TER, then left", true},
		{"multi-word term", "DBaaS", "The DBaaS team", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pattern := wordBoundaryPattern(tc.word)
			assert.Equal(t, tc.matches, pattern.MatchString(tc.text))
		})
	}
}

func TestTermInput_Defaults(t *testing.T) {
	input := TermInput{
		Term:      "TEST",
		Expansion: "Test Expansion",
	}

	// Verify defaults work when creating
	assert.Equal(t, "TEST", input.Term)
	assert.Equal(t, "Test Expansion", input.Expansion)
	assert.Empty(t, input.Definition)
	assert.Nil(t, input.Context)
	assert.Nil(t, input.Aliases)
	assert.Nil(t, input.ExpandInSearch) // nil means default true
}

func TestTermFilter_Defaults(t *testing.T) {
	filter := TermFilter{}

	// Verify empty filter doesn't crash
	assert.Empty(t, filter.Term)
	assert.Empty(t, filter.Search)
	assert.Nil(t, filter.Context)
	assert.Equal(t, 0, filter.Limit)
	assert.Equal(t, 0, filter.Offset)
	assert.False(t, filter.ExpandOnly)
}

func TestExpansionResult(t *testing.T) {
	result := ExpansionResult{
		OriginalTerm: "TER",
		Expansion:    "Technical Execution Review",
		Aliases:      []string{"T.E.R."},
		Definition:   "Weekly meeting",
	}

	assert.Equal(t, "TER", result.OriginalTerm)
	assert.Equal(t, "Technical Execution Review", result.Expansion)
	assert.Len(t, result.Aliases, 1)
}

func TestQueryExpansion_NoMatches(t *testing.T) {
	expansion := QueryExpansion{
		OriginalQuery: "hello world",
		ExpandedTerms: []ExpansionResult{},
		ExpandedQuery: "hello world",
	}

	assert.Equal(t, "hello world", expansion.OriginalQuery)
	assert.Empty(t, expansion.ExpandedTerms)
	assert.Equal(t, "hello world", expansion.ExpandedQuery)
}

func TestQueryExpansion_WithMatches(t *testing.T) {
	expansion := QueryExpansion{
		OriginalQuery: "TER meeting",
		ExpandedTerms: []ExpansionResult{
			{
				OriginalTerm: "TER",
				Expansion:    "Technical Execution Review",
			},
		},
		ExpandedQuery: "TER meeting OR Technical Execution Review",
	}

	assert.Len(t, expansion.ExpandedTerms, 1)
	assert.Equal(t, "TER", expansion.ExpandedTerms[0].OriginalTerm)
	assert.Contains(t, expansion.ExpandedQuery, "Technical Execution Review")
}
