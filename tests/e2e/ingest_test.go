//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmailIngestion_MentionResolution tests that email ingestion correctly
// resolves mentions to canonical entity names using the LLM.
func TestEmailIngestion_MentionResolution(t *testing.T) {
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

	// Simulate email content with mentions
	emailContent := `
Subject: Q4 Planning Update

Hey Sarah,

Just wanted to follow up on our discussion from yesterday. Marcus has reviewed
the timeline and thinks we need another two weeks for the infrastructure changes.

Can you loop in Mike when you get a chance? He should be aware of the frontend
implications.

Best,
John Smith
`

	// Test: Extract mentions from email
	extractionPrompt := `Analyze this email and identify all person mentions:

Email:
"""
` + emailContent + `
"""

` + peopleContext + `

List each person mentioned with their canonical name. Format:
- <mentioned_text> -> <canonical_name>
`

	response, err := client.CompleteWithSystem(ctx,
		"You are a mention extraction system. Identify people mentioned in the email and resolve them to their canonical names from the organization list.",
		extractionPrompt,
	)
	require.NoError(t, err)

	t.Logf("Extraction response:\n%s", response)

	// Assert: Verify key mentions are resolved
	tests := []struct {
		mentioned string
		canonical string
	}{
		{"Sarah", "Sarah Chen"},
		{"Marcus", "Marcus Rodriguez"},
		{"Mike", "Michael Brown"},
		{"John Smith", "John Smith"},
	}

	for _, tc := range tests {
		assert.True(t,
			strings.Contains(response, tc.canonical),
			"mention '%s' should resolve to '%s'", tc.mentioned, tc.canonical,
		)
	}
}

// TestEmailIngestion_TeamMentions tests resolution of team mentions.
func TestEmailIngestion_TeamMentions(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	// Build team context
	teamsContext := buildTeamsContext(t, env)

	emailContent := `
The Platform team has completed the API migration. We need to sync with
the Frontend team about the new endpoints before the release.

Also, please check with Engineering about the load testing results.
`

	extractionPrompt := `Identify team mentions in this email:

Email:
"""
` + emailContent + `
"""

` + teamsContext + `

List each team mentioned with their canonical name.
`

	response, err := client.CompleteWithSystem(ctx,
		"You are an entity extraction system. Identify teams mentioned in the text.",
		extractionPrompt,
	)
	require.NoError(t, err)

	t.Logf("Team extraction response:\n%s", response)

	// Verify team mentions
	assert.True(t, strings.Contains(response, "Platform") || strings.Contains(response, "platform"),
		"should identify Platform team")
	assert.True(t, strings.Contains(response, "Frontend") || strings.Contains(response, "frontend"),
		"should identify Frontend team")
}

// TestEmailIngestion_ProjectMentions tests resolution of project mentions.
func TestEmailIngestion_ProjectMentions(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	// Build projects context
	projectsContext := buildProjectsContext(t, env)

	emailContent := `
Quick update on Project Phoenix - the Phase 1 migration is complete.

We should discuss how this affects Project Aurora timeline. The Data Pipeline
project is now unblocked.
`

	extractionPrompt := `Identify project mentions in this email:

Email:
"""
` + emailContent + `
"""

` + projectsContext + `

List each project mentioned with their canonical name.
`

	response, err := client.CompleteWithSystem(ctx,
		"You are an entity extraction system. Identify projects mentioned in the text.",
		extractionPrompt,
	)
	require.NoError(t, err)

	t.Logf("Project extraction response:\n%s", response)

	// Verify project mentions
	assert.True(t, strings.Contains(response, "Phoenix"),
		"should identify Project Phoenix")
}

// TestEmailIngestion_MixedEntityTypes tests extraction of mixed entity types.
func TestEmailIngestion_MixedEntityTypes(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	client := NewLLMClient(env.LLMURL)
	ctx := context.Background()

	peopleContext := buildPeopleContext(t, env)
	teamsContext := buildTeamsContext(t, env)
	projectsContext := buildProjectsContext(t, env)

	emailContent := `
Hi team,

Sarah from Platform team has an update on Project Phoenix. She'll sync with
Marcus tomorrow about the database migration.

Can someone from the Backend team help review the changes? Lisa mentioned
some concerns about the API design.

Thanks,
Emily
`

	combinedContext := "PEOPLE:\n" + peopleContext + "\n\nTEAMS:\n" + teamsContext + "\n\nPROJECTS:\n" + projectsContext

	extractionPrompt := `Extract all entity mentions from this email. Categorize by type:

Email:
"""
` + emailContent + `
"""

` + combinedContext + `

Format your response as:
PEOPLE:
- <text> -> <canonical_name>

TEAMS:
- <text> -> <canonical_name>

PROJECTS:
- <text> -> <canonical_name>
`

	response, err := client.CompleteWithSystem(ctx,
		"You are an entity extraction system. Extract and categorize all entity mentions.",
		extractionPrompt,
	)
	require.NoError(t, err)

	t.Logf("Mixed entity extraction:\n%s", response)

	// Verify mixed mentions
	assert.Contains(t, response, "Sarah", "should identify Sarah")
	assert.Contains(t, response, "Marcus", "should identify Marcus")
	assert.Contains(t, response, "Platform", "should identify Platform team")
	assert.Contains(t, response, "Phoenix", "should identify Project Phoenix")
}

// Helper functions for building context

func buildTeamsContext(t *testing.T, env *E2EEnv) string {
	t.Helper()

	ctx := context.Background()
	rows, err := env.DB.Query(ctx, `
		SELECT id, name, description
		FROM teams
		ORDER BY id
		LIMIT 10
	`)
	require.NoError(t, err)
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Teams in the organization:\n")

	for rows.Next() {
		var id int64
		var name string
		var description *string
		err := rows.Scan(&id, &name, &description)
		require.NoError(t, err)

		sb.WriteString("- " + name)
		if description != nil && *description != "" {
			sb.WriteString(" (" + *description + ")")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func buildProjectsContext(t *testing.T, env *E2EEnv) string {
	t.Helper()

	ctx := context.Background()
	rows, err := env.DB.Query(ctx, `
		SELECT id, name, description
		FROM projects
		ORDER BY id
		LIMIT 15
	`)
	require.NoError(t, err)
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Projects in the organization:\n")

	for rows.Next() {
		var id int64
		var name string
		var description *string
		err := rows.Scan(&id, &name, &description)
		require.NoError(t, err)

		sb.WriteString("- " + name)
		if description != nil && *description != "" {
			sb.WriteString(" (" + *description + ")")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
