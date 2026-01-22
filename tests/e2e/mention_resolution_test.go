//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMentionResolutionWithLLM tests the full mention resolution pipeline with real LLM.
func TestMentionResolutionWithLLM(t *testing.T) {
	env := SetupE2EEnvironment(t)

	// Setup: Load fixtures
	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	// Build people context from database
	peopleContext := buildPeopleContext(t, env)

	tests := []struct {
		name           string
		text           string
		expectedPerson string
		description    string
	}{
		{
			name:           "canonical name exact match",
			text:           "John Smith will lead the meeting.",
			expectedPerson: "John Smith",
			description:    "exact canonical name should resolve",
		},
		{
			name:           "first name alias",
			text:           "Sarah mentioned the timeline concerns.",
			expectedPerson: "Sarah Chen",
			description:    "first name alias should resolve to full name",
		},
		{
			name:           "email prefix reference",
			text:           "Please sync with marcus.r about the Q1 deadline.",
			expectedPerson: "Marcus Rodriguez",
			description:    "email prefix should resolve",
		},
		{
			name:           "nickname alias",
			text:           "Mike reviewed the PR yesterday.",
			expectedPerson: "Michael Brown",
			description:    "nickname should resolve to canonical name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prompt := buildMentionResolutionPrompt(tc.text, peopleContext)

			response, err := client.CompleteWithSystem(ctx,
				"You are a mention resolution system. Given text and a list of people, identify which person is mentioned. Respond with ONLY the full canonical name of the person, or 'UNKNOWN' if no match.",
				prompt,
			)
			require.NoError(t, err)

			response = strings.TrimSpace(response)
			t.Logf("Text: %q -> Resolved: %q (expected: %q)", tc.text, response, tc.expectedPerson)

			// Semantic assertion: the response should contain or match the expected person
			assert.True(t,
				strings.Contains(response, tc.expectedPerson) || response == tc.expectedPerson,
				"%s: expected response to contain '%s', got '%s'",
				tc.description, tc.expectedPerson, response,
			)
		})
	}
}

// TestMentionResolutionAmbiguous tests handling of ambiguous mentions.
func TestMentionResolutionAmbiguous(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	peopleContext := buildPeopleContext(t, env)

	// Test: ambiguous first name with context
	prompt := buildMentionResolutionPromptWithContext(
		"The engineer mentioned a bug in the frontend.",
		"Jennifer Lee",                   // from context, we know the engineer is Jennifer
		"This is a discussion about UI.", // additional context
		peopleContext,
	)

	response, err := client.CompleteWithSystem(ctx,
		"You are a mention resolution system. Given text, context, and a list of people, identify which person is being referred to. Consider job titles and context. Respond with ONLY the full canonical name, or 'UNKNOWN' if uncertain.",
		prompt,
	)
	require.NoError(t, err)

	response = strings.TrimSpace(response)
	t.Logf("Ambiguous mention resolved to: %s", response)

	// Should resolve to Jennifer Lee (Frontend Engineer)
	assert.Contains(t, response, "Jennifer", "should resolve 'engineer' to Jennifer in frontend context")
}

// TestGlossaryExpansionWithLLM tests glossary term expansion.
func TestGlossaryExpansionWithLLM(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	// Build glossary context from database
	glossaryContext := buildGlossaryContext(t, env)

	tests := []struct {
		name      string
		text      string
		term      string
		expansion string
	}{
		{
			name:      "TER expansion",
			text:      "We discussed this at TER yesterday.",
			term:      "TER",
			expansion: "Technical Execution Review",
		},
		{
			name:      "MVP expansion",
			text:      "The MVP is scheduled for next week.",
			term:      "MVP",
			expansion: "Minimum Viable Product",
		},
		{
			name:      "OKR expansion",
			text:      "Let's review our Q1 OKRs.",
			term:      "OKR",
			expansion: "Objectives and Key Results",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prompt := fmt.Sprintf(`Given this text: "%s"

And this glossary:
%s

Identify the acronym "%s" and its expansion. Respond in JSON format:
{"term": "...", "expansion": "..."}`, tc.text, glossaryContext, tc.term)

			response, err := client.Complete(ctx, prompt)
			require.NoError(t, err)

			t.Logf("Glossary response: %s", response)

			// Parse JSON response
			var result struct {
				Term      string `json:"term"`
				Expansion string `json:"expansion"`
			}

			// Extract JSON from response (may have markdown code blocks)
			jsonStr := extractJSON(response)
			if jsonStr != "" {
				if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
					assert.Equal(t, tc.term, result.Term)
					assert.Contains(t, result.Expansion, tc.expansion,
						"expansion should contain '%s'", tc.expansion)
					return
				}
			}

			// Fallback: just check the response contains the expansion
			assert.Contains(t, response, tc.expansion,
				"response should contain expansion '%s'", tc.expansion)
		})
	}
}

// Helper functions

func buildPeopleContext(t *testing.T, env *E2EEnv) string {
	t.Helper()

	ctx := context.Background()
	rows, err := env.DB.Query(ctx, `
		SELECT id, canonical_name, email, title, aliases
		FROM people
		ORDER BY id
		LIMIT 20
	`)
	require.NoError(t, err)
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("People in the organization:\n")

	for rows.Next() {
		var id int64
		var name, email, title string
		var aliases []string
		err := rows.Scan(&id, &name, &email, &title, &aliases)
		require.NoError(t, err)

		sb.WriteString(fmt.Sprintf("- %s (ID: %d, Email: %s, Title: %s", name, id, email, title))
		if len(aliases) > 0 {
			sb.WriteString(fmt.Sprintf(", Aliases: %s", strings.Join(aliases, ", ")))
		}
		sb.WriteString(")\n")
	}

	return sb.String()
}

func buildGlossaryContext(t *testing.T, env *E2EEnv) string {
	t.Helper()

	ctx := context.Background()
	rows, err := env.DB.Query(ctx, `
		SELECT term, expansion, definition
		FROM glossary_terms
		WHERE expansion IS NOT NULL
		ORDER BY term
		LIMIT 30
	`)
	require.NoError(t, err)
	defer rows.Close()

	var sb strings.Builder
	for rows.Next() {
		var term string
		var expansion, definition *string
		err := rows.Scan(&term, &expansion, &definition)
		require.NoError(t, err)

		sb.WriteString(fmt.Sprintf("- %s: %s", term, *expansion))
		if definition != nil && *definition != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", *definition))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func buildMentionResolutionPrompt(text, peopleContext string) string {
	return fmt.Sprintf(`Text to analyze: "%s"

%s

Who is mentioned in the text? Respond with their full canonical name only.`, text, peopleContext)
}

func buildMentionResolutionPromptWithContext(text, hintPerson, additionalContext, peopleContext string) string {
	return fmt.Sprintf(`Text to analyze: "%s"

Additional context: %s
Hint: The conversation involves %s.

%s

Who is being referred to? Respond with their full canonical name only.`, text, additionalContext, hintPerson, peopleContext)
}

func extractJSON(s string) string {
	// Try to find JSON in the response (may be wrapped in markdown code blocks)
	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}
	end := strings.LastIndex(s, "}")
	if end == -1 || end < start {
		return ""
	}
	return s[start : end+1]
}
