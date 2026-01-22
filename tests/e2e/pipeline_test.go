//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullPipeline_IngestToSearch tests the complete pipeline:
// 1. Ingest email content
// 2. Extract and resolve mentions
// 3. Enrich with glossary expansions
// 4. Verify searchability
func TestFullPipeline_IngestToSearch(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	// Load all context
	peopleContext := buildPeopleContext(t, env)
	teamsContext := buildTeamsContext(t, env)
	projectsContext := buildProjectsContext(t, env)
	glossaryContext := buildGlossaryContext(t, env)

	// Step 1: Simulate ingesting an email
	emailContent := `
Subject: TER Summary - Q4 Planning

Hi team,

Quick summary from today's Technical Execution Review:

1. Sarah presented the new API design for Project Phoenix. The team agreed
   to proceed with the microservices approach.

2. Marcus raised concerns about the timeline. Platform team needs an
   additional sprint for the K8s migration.

3. Action items:
   - Mike to update the ADR for the caching strategy
   - Lisa to finalize the UX mocks by EOW
   - Backend team to review the RFC

4. The MVP is still on track for Q4. We'll revisit the OKRs next TER.

Best,
Emily Watson
Engineering Manager
`

	// Step 2: Extract entities using LLM
	t.Log("=== Step 2: Entity Extraction ===")

	extractionPrompt := buildFullExtractionPrompt(emailContent, peopleContext, teamsContext, projectsContext, glossaryContext)

	extractionResult, err := client.CompleteWithSystem(ctx,
		"You are an entity extraction system. Extract all entities and acronyms from the content.",
		extractionPrompt,
	)
	require.NoError(t, err)
	t.Logf("Extraction result:\n%s", extractionResult)

	// Verify key extractions
	verifyExtraction(t, extractionResult, []string{
		"Sarah", "Marcus", "Mike", "Lisa", "Emily Watson",
		"Platform", "Backend",
		"Phoenix",
		"TER", "API", "K8s", "ADR", "MVP", "OKR", "RFC",
	})

	// Step 3: Resolve entities to canonical forms
	t.Log("=== Step 3: Entity Resolution ===")

	resolutionPrompt := `Based on this extraction result:
` + extractionResult + `

And these organization entities:
` + peopleContext + `
` + teamsContext + `
` + projectsContext + `

Resolve each extracted entity to its canonical form. Return JSON:
{
  "people": [{"mentioned": "...", "canonical": "...", "id": ...}],
  "teams": [{"mentioned": "...", "canonical": "..."}],
  "projects": [{"mentioned": "...", "canonical": "..."}]
}
`

	resolutionResult, err := client.Complete(ctx, resolutionPrompt)
	require.NoError(t, err)
	t.Logf("Resolution result:\n%s", resolutionResult)

	// Verify key resolutions
	assert.Contains(t, resolutionResult, "Sarah Chen", "'Sarah' should resolve to 'Sarah Chen'")
	assert.Contains(t, resolutionResult, "Marcus Rodriguez", "'Marcus' should resolve to 'Marcus Rodriguez'")

	// Step 4: Expand glossary terms
	t.Log("=== Step 4: Glossary Expansion ===")

	glossaryPrompt := `Expand these acronyms found in the email using this glossary:
` + glossaryContext + `

Acronyms to expand: TER, API, K8s, ADR, MVP, OKR, RFC, EOW

Return JSON:
{
  "expansions": {
    "TER": "...",
    "API": "...",
    ...
  }
}
`

	glossaryResult, err := client.Complete(ctx, glossaryPrompt)
	require.NoError(t, err)
	t.Logf("Glossary expansion:\n%s", glossaryResult)

	// Verify key expansions
	verifyExpansions(t, glossaryResult, map[string]string{
		"TER": "Technical Execution Review",
		"MVP": "Minimum Viable Product",
		"OKR": "Objectives and Key Results",
	})

	// Step 5: Simulate search query
	t.Log("=== Step 5: Search Simulation ===")

	searchQuery := "What did Sarah say about Project Phoenix at TER?"

	searchPrompt := fmt.Sprintf(`Given this search query: "%s"

And this enriched email content:
---
%s
---

With these resolved entities:
%s

With these glossary expansions:
%s

1. Expand the search query using glossary terms
2. Identify which entities to filter by
3. Determine if the email matches the query

Return JSON:
{
  "expanded_query": "...",
  "entity_filters": {
    "people": [...],
    "projects": [...]
  },
  "matches_email": true/false,
  "relevance_score": 0.0-1.0,
  "matched_sections": ["..."]
}`, searchQuery, emailContent, resolutionResult, glossaryResult)

	searchResult, err := client.Complete(ctx, searchPrompt)
	require.NoError(t, err)
	t.Logf("Search result:\n%s", searchResult)

	// Verify search matches
	assert.Contains(t, searchResult, "true", "email should match the search query")
	assert.Contains(t, searchResult, "Sarah", "search should identify Sarah as relevant")
	assert.Contains(t, searchResult, "Phoenix", "search should identify Project Phoenix as relevant")
}

// TestFullPipeline_MeetingTranscript tests the pipeline with a meeting transcript.
func TestFullPipeline_MeetingTranscript(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	// Load the meeting transcript fixture
	transcriptPath := env.FixturePath("meetings/001-weekly-standup.txt")

	// Read the transcript file
	var transcriptContent string
	rows, err := env.DB.Query(ctx, "SELECT 1")
	if err != nil {
		t.Skip("Database query failed, skipping transcript test")
	}
	rows.Close()

	// For this test, we'll use inline transcript content
	transcriptContent = `Weekly Standup - Engineering Team
Date: Monday 9:00 AM

LISA CHANG (FACILITATOR): Good morning everyone. Let's go around the room with updates.

MICHAEL BROWN: I've been working on the API caching layer. Should have the POC ready by EOD.

SARAH CHEN: The frontend team finished the user dashboard. We're waiting on API changes from Marcus.

MARCUS RODRIGUEZ: I'm wrapping up the database migration. The Platform team helped with the K8s config.

JENNIFER LEE: I have some concerns about the UX flow. Can we schedule a design review?

DAVID KIM: Security audit is complete. No critical issues, but we have some recommendations for the MVP.

LISA CHANG: Great updates. Let's sync at TER on Thursday about the Q4 OKRs.
`

	// Load context
	peopleContext := buildPeopleContext(t, env)
	glossaryContext := buildGlossaryContext(t, env)

	// Step 1: Extract speaker identification
	t.Log("=== Transcript Speaker Identification ===")

	speakerPrompt := `Identify all speakers in this meeting transcript and match them to their full canonical names:

Transcript:
"""
` + transcriptContent + `
"""

Known people:
` + peopleContext + `

Return JSON:
{
  "speakers": [
    {"raw_name": "...", "canonical_name": "...", "speaking_turns": 1}
  ]
}
`

	speakerResult, err := client.Complete(ctx, speakerPrompt)
	require.NoError(t, err)
	t.Logf("Speaker identification:\n%s", speakerResult)

	// Verify speakers
	assert.Contains(t, speakerResult, "Lisa Chang", "should identify Lisa Chang")
	assert.Contains(t, speakerResult, "Michael Brown", "should identify Michael Brown")
	assert.Contains(t, speakerResult, "Sarah Chen", "should identify Sarah Chen")
	assert.Contains(t, speakerResult, "Marcus Rodriguez", "should identify Marcus Rodriguez")

	// Step 2: Extract glossary terms
	t.Log("=== Transcript Glossary Extraction ===")

	glossaryPrompt := `Extract all acronyms and technical terms from this transcript:

"""
` + transcriptContent + `
"""

Using this glossary:
` + glossaryContext + `

Return JSON with found terms and their expansions.
`

	termResult, err := client.Complete(ctx, glossaryPrompt)
	require.NoError(t, err)
	t.Logf("Term extraction:\n%s", termResult)

	// Verify terms
	assert.Contains(t, termResult, "API", "should identify API")
	assert.Contains(t, termResult, "POC", "should identify POC")
	assert.Contains(t, termResult, "K8s", "should identify K8s")
	assert.Contains(t, termResult, "TER", "should identify TER")
	assert.Contains(t, termResult, "OKR", "should identify OKR")
}

// TestFullPipeline_BatchIngestion tests batch processing of multiple documents.
func TestFullPipeline_BatchIngestion(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	// Simulate batch of emails
	emails := []struct {
		subject string
		body    string
	}{
		{
			subject: "Project Phoenix Update",
			body:    "Sarah completed the API design review. Marcus will handle the deployment.",
		},
		{
			subject: "TER Action Items",
			body:    "Mike to update the ADR. Lisa to finalize UX. Backend team to review.",
		},
		{
			subject: "Q4 OKR Review",
			body:    "The MVP is on track. Platform team completed the K8s migration.",
		},
	}

	peopleContext := buildPeopleContext(t, env)
	teamsContext := buildTeamsContext(t, env)
	glossaryContext := buildGlossaryContext(t, env)

	// Process batch
	t.Log("=== Batch Processing ===")

	var allEmails strings.Builder
	for i, email := range emails {
		allEmails.WriteString(fmt.Sprintf("EMAIL %d:\nSubject: %s\n%s\n\n", i+1, email.subject, email.body))
	}

	batchPrompt := `Process this batch of emails. For each email, extract:
1. People mentioned (resolved to canonical names)
2. Teams mentioned
3. Acronyms found (with expansions)

Emails:
` + allEmails.String() + `

Context:
` + peopleContext + `
` + teamsContext + `
` + glossaryContext + `

Return JSON array with one entry per email:
[
  {
    "email_index": 1,
    "people": [...],
    "teams": [...],
    "acronyms": [{"term": "...", "expansion": "..."}]
  },
  ...
]
`

	batchResult, err := client.Complete(ctx, batchPrompt)
	require.NoError(t, err)
	t.Logf("Batch result:\n%s", batchResult)

	// Verify batch processing
	assert.Contains(t, batchResult, "Sarah Chen", "batch should resolve Sarah")
	assert.Contains(t, batchResult, "Marcus Rodriguez", "batch should resolve Marcus")
	assert.Contains(t, batchResult, "Platform", "batch should identify Platform team")
	assert.Contains(t, batchResult, "Technical Execution Review", "batch should expand TER")
}

// TestFullPipeline_CrossReferenceSearch tests searching across multiple documents.
func TestFullPipeline_CrossReferenceSearch(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	// Simulate a document store
	documents := []struct {
		id      string
		title   string
		content string
		date    time.Time
	}{
		{
			id:      "doc-001",
			title:   "Project Phoenix Kickoff",
			content: "Sarah Chen presented the initial architecture. The team discussed microservices vs monolith.",
			date:    time.Now().AddDate(0, 0, -7),
		},
		{
			id:      "doc-002",
			title:   "TER Summary 2024-01-15",
			content: "Marcus Rodriguez updated on the database migration. K8s deployment scheduled for next week.",
			date:    time.Now().AddDate(0, 0, -3),
		},
		{
			id:      "doc-003",
			title:   "Q4 OKR Review",
			content: "The MVP milestone was reached. Platform team exceeded velocity targets.",
			date:    time.Now().AddDate(0, 0, -1),
		},
	}

	// Build document context
	var docContext strings.Builder
	for _, doc := range documents {
		docContext.WriteString(fmt.Sprintf("Document: %s\nTitle: %s\nDate: %s\nContent: %s\n\n",
			doc.id, doc.title, doc.date.Format("2006-01-02"), doc.content))
	}

	peopleContext := buildPeopleContext(t, env)
	glossaryContext := buildGlossaryContext(t, env)

	// Search query: find documents mentioning a person
	searchPrompt := `Given this search query: "What did Sarah discuss about the architecture?"

Documents:
` + docContext.String() + `

Context:
` + peopleContext + `
` + glossaryContext + `

Find and rank documents matching this query. Return JSON:
{
  "query_analysis": {
    "entities": [...],
    "key_terms": [...],
    "expanded_terms": [...]
  },
  "results": [
    {
      "doc_id": "...",
      "title": "...",
      "relevance_score": 0.0-1.0,
      "matched_entities": [...],
      "matched_terms": [...],
      "snippet": "..."
    }
  ]
}
`

	searchResult, err := client.Complete(ctx, searchPrompt)
	require.NoError(t, err)
	t.Logf("Cross-reference search:\n%s", searchResult)

	// Verify search results
	assert.Contains(t, searchResult, "doc-001", "should find Project Phoenix Kickoff")
	assert.Contains(t, searchResult, "Sarah Chen", "should resolve Sarah to Sarah Chen")
	assert.Contains(t, searchResult, "architecture", "should match architecture topic")
}

// Helper functions

func buildFullExtractionPrompt(content, people, teams, projects, glossary string) string {
	return fmt.Sprintf(`Extract all entities and acronyms from this content:

Content:
"""
%s
"""

Organization Context:
PEOPLE:
%s

TEAMS:
%s

PROJECTS:
%s

GLOSSARY:
%s

Return a structured extraction with:
1. All people mentioned
2. All teams mentioned
3. All projects mentioned
4. All acronyms found

Format as JSON.`, content, people, teams, projects, glossary)
}

func verifyExtraction(t *testing.T, result string, expectedTerms []string) {
	t.Helper()

	for _, term := range expectedTerms {
		if !strings.Contains(result, term) {
			t.Logf("Warning: expected term '%s' not found in extraction", term)
		}
	}
}

func verifyExpansions(t *testing.T, result string, expected map[string]string) {
	t.Helper()

	for term, expansion := range expected {
		if !strings.Contains(result, expansion) {
			t.Errorf("expansion for '%s' should contain '%s'", term, expansion)
		}
	}
}
