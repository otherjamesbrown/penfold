package activities

import (
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConsolidationResponse(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
		check   func(t *testing.T, resp *consolidationResponse)
	}{
		{
			name: "valid full response",
			raw: `{
				"title": "Session work summary",
				"narrative": "The session focused on bug fixes.",
				"decisions": [{"title": "Use pgx", "body": "Chose pgx over database/sql", "session_id": "s1"}],
				"patterns": [{"title": "Schema changes", "body": "Frequent schema changes", "evidence": ["Migration 089", "Migration 090"]}]
			}`,
			check: func(t *testing.T, resp *consolidationResponse) {
				assert.Equal(t, "Session work summary", resp.Title)
				assert.Equal(t, "The session focused on bug fixes.", resp.Narrative)
				assert.Len(t, resp.Decisions, 1)
				assert.Equal(t, "Use pgx", resp.Decisions[0].Title)
				assert.Len(t, resp.Patterns, 1)
				assert.Equal(t, "Schema changes", resp.Patterns[0].Title)
				assert.Len(t, resp.Patterns[0].Evidence, 2)
			},
		},
		{
			name: "minimal response with empty arrays",
			raw: `{
				"title": "Minimal",
				"narrative": "Some work was done.",
				"decisions": [],
				"patterns": []
			}`,
			check: func(t *testing.T, resp *consolidationResponse) {
				assert.Equal(t, "Minimal", resp.Title)
				assert.Empty(t, resp.Decisions)
				assert.Empty(t, resp.Patterns)
			},
		},
		{
			name: "missing title defaults",
			raw:  `{"narrative": "Work happened."}`,
			check: func(t *testing.T, resp *consolidationResponse) {
				assert.Equal(t, "Untitled consolidation", resp.Title)
			},
		},
		{
			name:    "missing narrative fails",
			raw:     `{"title": "No narrative"}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			raw:     `not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := parseConsolidationResponse(tt.raw)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, resp)
			}
		})
	}
}

func TestGroupEntriesBySession(t *testing.T) {
	now := time.Now()
	entries := []*ledger.Entry{
		{ID: 1, SessionID: "s1", CreatedAt: now},
		{ID: 2, SessionID: "s2", CreatedAt: now},
		{ID: 3, SessionID: "s1", CreatedAt: now.Add(time.Minute)},
		{ID: 4, SessionID: "s1", CreatedAt: now.Add(2 * time.Minute)},
		{ID: 5, SessionID: "s2", CreatedAt: now.Add(time.Minute)},
	}

	groups := groupEntriesBySession(entries)
	assert.Len(t, groups, 2)
	assert.Len(t, groups["s1"], 3)
	assert.Len(t, groups["s2"], 2)
}

func strPtr(s string) *string { return &s }

func TestBuildConsolidationPrompt(t *testing.T) {
	now := time.Now()
	entries := []*ledger.Entry{
		{
			ID:        1,
			SessionID: "s1",
			EntryType: "decision",
			Title:     "Chose pgx",
			Body:      strPtr("Because it's better"),
			Agent:     "agent-mycroft",
			Labels:    []string{"architecture"},
			CreatedAt: now,
		},
		{
			ID:        2,
			SessionID: "s1",
			EntryType: "narrative",
			Title:     "Did some work",
			Agent:     "agent-mycroft",
			CreatedAt: now.Add(time.Hour),
		},
	}

	template := "Consolidate these entries:\n\n%s\n\nRespond with JSON."

	prompt := buildConsolidationPrompt(template, entries)

	assert.Contains(t, prompt, "Consolidate these entries:")
	assert.Contains(t, prompt, "[decision] Chose pgx")
	assert.Contains(t, prompt, "Because it's better")
	assert.Contains(t, prompt, "[narrative] Did some work")
	assert.Contains(t, prompt, "Labels: architecture")
	assert.Contains(t, prompt, "Respond with JSON.")
}

func TestToRepoDecisions(t *testing.T) {
	decisions := []consolidationDecisionJSON{
		{Title: "D1", Body: "Body1", SessionID: "s1"},
		{Title: "D2", Body: "Body2", SessionID: "s2"},
	}

	result := toRepoDecisions(decisions)
	require.Len(t, result, 2)
	assert.Equal(t, "D1", result[0].Title)
	assert.Equal(t, "s2", result[1].SessionID)
}

func TestToRepoPatterns(t *testing.T) {
	patterns := []consolidationPatternJSON{
		{Title: "P1", Body: "Body1", Evidence: []string{"e1", "e2"}},
	}

	result := toRepoPatterns(patterns)
	require.Len(t, result, 1)
	assert.Equal(t, "P1", result[0].Title)
	assert.Len(t, result[0].Evidence, 2)
}
