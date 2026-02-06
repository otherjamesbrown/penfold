package activities

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapCategoryToType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Singular forms
		{"decision", "decision"},
		{"risk", "risk"},
		{"commitment", "commitment"},
		{"milestone", "milestone"},
		{"outcome", "outcome"},
		{"dependency", "dependency"},
		{"assumption", "assumption"},
		{"issue", "issue"},
		{"action", "action"},
		{"question", "question"},

		// Plural forms
		{"decisions", "decision"},
		{"risks", "risk"},
		{"commitments", "commitment"},
		{"milestones", "milestone"},
		{"outcomes", "outcome"},
		{"dependencies", "dependency"},
		{"assumptions", "assumption"},
		{"issues", "issue"},
		{"actions", "action"},
		{"questions", "question"},

		// Unknown categories default to "issue"
		{"temporal", "issue"},
		{"organizational", "issue"},
		{"factual", "issue"},
		{"relational", "issue"},
		{"location", "issue"},
		{"quantity", "issue"},
		{"", "issue"},
		{"unknown", "issue"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapCategoryToType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
