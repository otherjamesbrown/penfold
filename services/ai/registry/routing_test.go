package registry

import (
	"testing"
)

func TestMatchesConditions_StringMatch(t *testing.T) {
	tests := []struct {
		name       string
		conditions map[string][]string
		attrs      map[string]string
		want       bool
	}{
		{
			name:       "empty conditions matches all",
			conditions: map[string][]string{},
			attrs:      map[string]string{"anything": "value"},
			want:       true,
		},
		{
			name:       "nil conditions matches all",
			conditions: nil,
			attrs:      map[string]string{"anything": "value"},
			want:       true,
		},
		{
			name:       "single field match",
			conditions: map[string][]string{"category": {"DECISION"}},
			attrs:      map[string]string{"category": "DECISION"},
			want:       true,
		},
		{
			name:       "single field no match",
			conditions: map[string][]string{"category": {"DECISION"}},
			attrs:      map[string]string{"category": "RISK"},
			want:       false,
		},
		{
			name:       "missing attr",
			conditions: map[string][]string{"category": {"DECISION"}},
			attrs:      map[string]string{},
			want:       false,
		},
		{
			name:       "case insensitive",
			conditions: map[string][]string{"category": {"decision"}},
			attrs:      map[string]string{"category": "DECISION"},
			want:       true,
		},
		{
			name:       "OR within field",
			conditions: map[string][]string{"category": {"DECISION", "RISK"}},
			attrs:      map[string]string{"category": "RISK"},
			want:       true,
		},
		{
			name:       "AND across fields",
			conditions: map[string][]string{"category": {"DECISION"}, "importance": {"HIGH"}},
			attrs:      map[string]string{"category": "DECISION", "importance": "HIGH"},
			want:       true,
		},
		{
			name:       "AND fails one field",
			conditions: map[string][]string{"category": {"DECISION"}, "importance": {"HIGH"}},
			attrs:      map[string]string{"category": "DECISION", "importance": "LOW"},
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesConditions(tt.conditions, tt.attrs)
			if got != tt.want {
				t.Errorf("MatchesConditions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesConditions_NumericRange(t *testing.T) {
	tests := []struct {
		name       string
		conditions map[string][]string
		attrs      map[string]string
		want       bool
	}{
		{
			name:       "max under threshold",
			conditions: map[string][]string{"max_content_chars": {"16000"}},
			attrs:      map[string]string{"content_chars": "5000"},
			want:       true,
		},
		{
			name:       "max at threshold",
			conditions: map[string][]string{"max_content_chars": {"16000"}},
			attrs:      map[string]string{"content_chars": "16000"},
			want:       true,
		},
		{
			name:       "max over threshold",
			conditions: map[string][]string{"max_content_chars": {"16000"}},
			attrs:      map[string]string{"content_chars": "20000"},
			want:       false,
		},
		{
			name:       "min under threshold",
			conditions: map[string][]string{"min_content_chars": {"16001"}},
			attrs:      map[string]string{"content_chars": "5000"},
			want:       false,
		},
		{
			name:       "min at threshold",
			conditions: map[string][]string{"min_content_chars": {"16001"}},
			attrs:      map[string]string{"content_chars": "16001"},
			want:       true,
		},
		{
			name:       "min over threshold",
			conditions: map[string][]string{"min_content_chars": {"16001"}},
			attrs:      map[string]string{"content_chars": "50000"},
			want:       true,
		},
		{
			name:       "mixed string and numeric",
			conditions: map[string][]string{"category": {"DECISION"}, "max_content_chars": {"16000"}},
			attrs:      map[string]string{"category": "DECISION", "content_chars": "5000"},
			want:       true,
		},
		{
			name:       "mixed fail on string",
			conditions: map[string][]string{"category": {"DECISION"}, "max_content_chars": {"16000"}},
			attrs:      map[string]string{"category": "RISK", "content_chars": "5000"},
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesConditions(tt.conditions, tt.attrs)
			if got != tt.want {
				t.Errorf("MatchesConditions() = %v, want %v", got, tt.want)
			}
		})
	}
}
