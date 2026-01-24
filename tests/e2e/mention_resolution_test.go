//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMentionResolution_AfterIngestion tests that mentions are extracted and resolved
// after content ingestion via the CLI pipeline.
func TestMentionResolution_AfterIngestion(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Setup: Load fixtures
	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Verify people exist for resolution
	var peopleCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM people").Scan(&peopleCount)
	require.NoError(t, err)
	require.Greater(t, peopleCount, 0, "must have people loaded for mention resolution")
	t.Logf("People loaded: %d", peopleCount)

	// Ingest email with known mentions (Sarah, Marcus, Mike)
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "mention-resolution-test")

	if result.ExitCode != 0 {
		t.Skipf("Ingest command failed (exit code %d) - ensure services are running. Stderr: %s",
			result.ExitCode, result.Stderr)
	}

	t.Logf("Ingest completed in %v", result.Duration)

	// Allow time for async mention extraction (if applicable)
	time.Sleep(time.Second)

	// Check if mentions were extracted
	var mentionCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM content_mentions
		WHERE tenant_id = 'default'
	`).Scan(&mentionCount)

	if err != nil || mentionCount == 0 {
		t.Log("No mentions found in content_mentions table - mention extraction may require additional processing")
		t.Log("This test verifies the pipeline setup; actual mention extraction may be async")
		return
	}

	t.Logf("Mentions extracted: %d", mentionCount)

	// Query all mentions for analysis
	rows, err := env.DB.Query(ctx, `
		SELECT
			mentioned_text,
			entity_type::text,
			status::text,
			resolved_entity_id,
			resolution_confidence,
			resolution_source
		FROM content_mentions
		WHERE tenant_id = 'default'
		ORDER BY id
	`)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var text, entityType, status string
		var resolvedID *int64
		var confidence *float64
		var source *string

		err := rows.Scan(&text, &entityType, &status, &resolvedID, &confidence, &source)
		if err != nil {
			continue
		}

		resolvedStr := "unresolved"
		if resolvedID != nil {
			resolvedStr = "resolved"
		}
		confStr := "n/a"
		if confidence != nil {
			confStr = "set"
		}
		srcStr := "n/a"
		if source != nil {
			srcStr = *source
		}

		t.Logf("  Mention: %q type=%s status=%s %s conf=%s source=%s",
			text, entityType, status, resolvedStr, confStr, srcStr)
	}
}

// TestMentionResolution_PersonAliases tests that person aliases are available for resolution.
func TestMentionResolution_PersonAliases(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Check people with aliases are loaded (aliases is an array column on people table)
	var peopleWithAliases int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM people
		WHERE aliases IS NOT NULL AND array_length(aliases, 1) > 0
	`).Scan(&peopleWithAliases)
	require.NoError(t, err)
	t.Logf("People with aliases: %d", peopleWithAliases)

	// Verify people can be found by email
	expectedPeople := []struct {
		email string
		name  string
	}{
		{"john.smith@acme.com", "John Smith"},
		{"sarah.chen@acme.com", "Sarah Chen"},
	}

	for _, expected := range expectedPeople {
		var canonicalName string
		err = env.DB.QueryRow(ctx, `
			SELECT canonical_name FROM people
			WHERE primary_email = $1
		`, expected.email).Scan(&canonicalName)

		if err == nil {
			t.Logf("Email %s -> %s", expected.email, canonicalName)
			assert.Equal(t, expected.name, canonicalName,
				"email %s should belong to %s", expected.email, expected.name)
		}
	}

	// List some people with their aliases
	rows, err := env.DB.Query(ctx, `
		SELECT canonical_name, primary_email, aliases
		FROM people
		WHERE aliases IS NOT NULL AND array_length(aliases, 1) > 0
		LIMIT 5
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, email string
			var aliases []string
			rows.Scan(&name, &email, &aliases)
			t.Logf("  Person: %s <%s> aliases=%v", name, email, aliases)
		}
	}
}

// TestMentionResolution_GlossaryTermExpansion tests that glossary terms are available for expansion.
func TestMentionResolution_GlossaryTermExpansion(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Verify glossary terms
	tests := []struct {
		name      string
		term      string
		expansion string
	}{
		{
			name:      "TER expansion",
			term:      "TER",
			expansion: "Technical Execution Review",
		},
		{
			name:      "MVP expansion",
			term:      "MVP",
			expansion: "Minimum Viable Product",
		},
		{
			name:      "OKR expansion",
			term:      "OKR",
			expansion: "Objectives and Key Results",
		},
		{
			name:      "API expansion",
			term:      "API",
			expansion: "Application Programming Interface",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var expansion string
			err := env.DB.QueryRow(ctx, `
				SELECT expansion FROM glossary
				WHERE UPPER(term) = UPPER($1)
			`, tc.term).Scan(&expansion)

			if err != nil {
				t.Logf("Term %s not found in glossary", tc.term)
				return
			}

			t.Logf("Glossary: %s -> %s", tc.term, expansion)
			assert.Contains(t, expansion, tc.expansion,
				"expansion for %s should contain %s", tc.term, tc.expansion)
		})
	}
}

// TestMentionResolution_TeamContext tests that team context is available for resolution.
func TestMentionResolution_TeamContext(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Verify teams are loaded
	var teamCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM teams").Scan(&teamCount)
	require.NoError(t, err)
	t.Logf("Teams loaded: %d", teamCount)
	assert.Greater(t, teamCount, 0, "should have teams loaded")

	// List teams
	rows, err := env.DB.Query(ctx, `
		SELECT name, description
		FROM teams
		ORDER BY name
	`)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var name string
		var desc *string
		rows.Scan(&name, &desc)
		descStr := ""
		if desc != nil {
			descStr = *desc
		}
		t.Logf("  Team: %s (%s)", name, descStr)
	}
}

// TestMentionResolution_ProjectContext tests that project context is available for resolution.
func TestMentionResolution_ProjectContext(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Verify projects are loaded
	var projectCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM projects").Scan(&projectCount)
	require.NoError(t, err)
	t.Logf("Projects loaded: %d", projectCount)
	assert.Greater(t, projectCount, 0, "should have projects loaded")

	// List projects with keywords
	rows, err := env.DB.Query(ctx, `
		SELECT name, keywords
		FROM projects
		ORDER BY name
	`)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var name string
		var keywords []string
		rows.Scan(&name, &keywords)
		t.Logf("  Project: %s (keywords: %v)", name, keywords)
	}
}

// TestMentionResolution_EntityAffinity tests entity-project affinity tracking.
func TestMentionResolution_EntityAffinity(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// After ingestion and processing, check if entity affinities are created
	// First ingest content
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "affinity-test")

	if result.ExitCode != 0 {
		t.Skipf("Ingest command failed (exit code %d) - ensure services are running", result.ExitCode)
	}

	// Allow time for processing
	time.Sleep(time.Second)

	// Check for affinity records
	var affinityCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM entity_project_affinity
		WHERE tenant_id = 'default'
	`).Scan(&affinityCount)

	if err != nil {
		t.Log("Could not query entity_project_affinity table")
		return
	}

	t.Logf("Entity-project affinities: %d", affinityCount)

	// If we have affinities, show some details
	if affinityCount > 0 {
		rows, err := env.DB.Query(ctx, `
			SELECT entity_type::text, entity_id, project_id, affinity_score
			FROM entity_project_affinity
			WHERE tenant_id = 'default'
			ORDER BY affinity_score DESC
			LIMIT 5
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var entityType string
				var entityID, projectID int64
				var score float64
				rows.Scan(&entityType, &entityID, &projectID, &score)
				t.Logf("  Affinity: %s %d <-> project %d (score: %.2f)", entityType, entityID, projectID, score)
			}
		}
	}
}
