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

// TestSearch_GlossaryExpansion tests that search queries are expanded
// using glossary terms before execution.
func TestSearch_GlossaryExpansion(t *testing.T) {
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
		name          string
		query         string
		expectedTerms []string
		expanded      []string
	}{
		{
			name:          "TER query expansion",
			query:         "What happened at TER yesterday?",
			expectedTerms: []string{"TER"},
			expanded:      []string{"Technical Execution Review"},
		},
		{
			name:          "OKR query expansion",
			query:         "Review the Q1 OKRs",
			expectedTerms: []string{"OKR"},
			expanded:      []string{"Objectives and Key Results"},
		},
		{
			name:          "Multiple acronyms",
			query:         "Discuss MVP timeline with PM",
			expectedTerms: []string{"MVP", "PM"},
			expanded:      []string{"Minimum Viable Product", "Product Manager"},
		},
		{
			name:          "Context-aware expansion",
			query:         "Check the P0 incident status",
			expectedTerms: []string{"P0"},
			expanded:      []string{"Priority Zero", "Critical"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prompt := fmt.Sprintf(`Given this search query: "%s"

And this glossary:
%s

Identify any acronyms or terms from the glossary and expand them.
Return a JSON object with:
{
  "original_query": "...",
  "terms_found": ["term1", "term2"],
  "expansions": {"term1": "expansion1", "term2": "expansion2"},
  "expanded_query": "... (original query with expansions inline or appended)"
}`, tc.query, glossaryContext)

			response, err := client.Complete(ctx, prompt)
			require.NoError(t, err)

			t.Logf("Query: %s", tc.query)
			t.Logf("Response: %s", response)

			// Try to parse JSON response
			jsonStr := extractJSON(response)
			if jsonStr != "" {
				var result struct {
					TermsFound    []string          `json:"terms_found"`
					Expansions    map[string]string `json:"expansions"`
					ExpandedQuery string            `json:"expanded_query"`
				}
				if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
					// Verify expected terms were found
					for _, term := range tc.expectedTerms {
						found := false
						for _, foundTerm := range result.TermsFound {
							if strings.EqualFold(foundTerm, term) {
								found = true
								break
							}
						}
						assert.True(t, found, "expected term '%s' to be found", term)
					}

					// Verify expansions
					for _, expansion := range tc.expanded {
						assert.True(t,
							strings.Contains(response, expansion) ||
								strings.Contains(result.ExpandedQuery, expansion),
							"response should contain expansion '%s'", expansion)
					}
					return
				}
			}

			// Fallback: check response contains expansions
			for _, expansion := range tc.expanded {
				assert.Contains(t, response, expansion,
					"response should contain expansion '%s'", expansion)
			}
		})
	}
}

// TestSearch_SemanticQueryUnderstanding tests semantic query understanding.
func TestSearch_SemanticQueryUnderstanding(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	glossaryContext := buildGlossaryContext(t, env)
	peopleContext := buildPeopleContext(t, env)

	tests := []struct {
		name         string
		query        string
		expectations []string
	}{
		{
			name:  "Meeting-related query",
			query: "What was discussed at TER about the MVP?",
			expectations: []string{
				"Technical Execution Review",
				"Minimum Viable Product",
			},
		},
		{
			name:  "Person-specific query",
			query: "What did Sarah say about the deadline?",
			expectations: []string{
				"Sarah Chen",
			},
		},
		{
			name:  "Combined entity query",
			query: "Updates from Platform team on Project Phoenix",
			expectations: []string{
				"Platform",
				"Phoenix",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prompt := fmt.Sprintf(`Analyze this search query and identify:
1. Key entities (people, teams, projects)
2. Acronyms that should be expanded
3. The underlying intent

Query: "%s"

Context:
%s

%s

Provide a semantic analysis of what the user is looking for.`, tc.query, glossaryContext, peopleContext)

			response, err := client.Complete(ctx, prompt)
			require.NoError(t, err)

			t.Logf("Query: %s", tc.query)
			t.Logf("Analysis: %s", response)

			for _, expected := range tc.expectations {
				assert.Contains(t, response, expected,
					"analysis should reference '%s'", expected)
			}
		})
	}
}

// TestSearch_QueryRewriting tests intelligent query rewriting.
func TestSearch_QueryRewriting(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	glossaryContext := buildGlossaryContext(t, env)

	tests := []struct {
		name           string
		query          string
		mustContain    []string
		mustNotContain []string
	}{
		{
			name:        "Acronym rewrite",
			query:       "TER notes",
			mustContain: []string{"Technical Execution Review"},
		},
		{
			name:        "Implicit expansion",
			query:       "API documentation",
			mustContain: []string{"Application Programming Interface"},
		},
		{
			name:        "Context-sensitive rewrite",
			query:       "K8s deployment issues",
			mustContain: []string{"Kubernetes"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prompt := fmt.Sprintf(`Rewrite this search query by expanding acronyms and adding relevant synonyms:

Original query: "%s"

Glossary:
%s

Return ONLY the rewritten query as a single line, with expanded terms.`, tc.query, glossaryContext)

			response, err := client.Complete(ctx, prompt)
			require.NoError(t, err)

			response = strings.TrimSpace(response)
			t.Logf("Original: %s", tc.query)
			t.Logf("Rewritten: %s", response)

			for _, term := range tc.mustContain {
				assert.True(t,
					strings.Contains(strings.ToLower(response), strings.ToLower(term)),
					"rewritten query should contain '%s'", term)
			}

			for _, term := range tc.mustNotContain {
				assert.False(t,
					strings.Contains(response, term),
					"rewritten query should not contain '%s'", term)
			}
		})
	}
}

// TestSearch_PersonEntitySearch tests searching for content related to a person.
func TestSearch_PersonEntitySearch(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	peopleContext := buildPeopleContext(t, env)

	// Test: Resolve person reference and generate search terms
	prompt := `Given this search query: "What did Mike say about the release?"

` + peopleContext + `

1. Identify who "Mike" refers to (canonical name)
2. Generate a list of search terms to find relevant content

Respond in JSON:
{
  "resolved_person": "...",
  "search_terms": ["term1", "term2", ...]
}
`

	response, err := client.Complete(ctx, prompt)
	require.NoError(t, err)

	t.Logf("Person search response: %s", response)

	// Verify Mike resolves to Michael Brown
	assert.Contains(t, response, "Michael Brown",
		"Mike should resolve to Michael Brown")
}

// TestSearch_DateRelativeQueries tests handling of relative date queries.
func TestSearch_DateRelativeQueries(t *testing.T) {
	env := SetupE2EEnvironment(t)
	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{
			name:     "yesterday reference",
			query:    "What happened yesterday at TER?",
			expected: "date filter",
		},
		{
			name:     "last week reference",
			query:    "Updates from last week",
			expected: "date range",
		},
		{
			name:     "this month reference",
			query:    "Meetings this month",
			expected: "date filter",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prompt := fmt.Sprintf(`Parse this search query and identify any date-related filters:

Query: "%s"

If the query contains relative date references (yesterday, last week, this month, etc.),
identify what date range should be applied.

Respond in JSON:
{
  "has_date_filter": true/false,
  "date_type": "relative/absolute/none",
  "relative_reference": "yesterday/last week/etc",
  "suggested_filter": "description of the date filter to apply"
}`, tc.query)

			response, err := client.Complete(ctx, prompt)
			require.NoError(t, err)

			t.Logf("Query: %s", tc.query)
			t.Logf("Date parsing: %s", response)

			assert.Contains(t, strings.ToLower(response), "true",
				"query should be identified as having a date filter")
		})
	}
}
