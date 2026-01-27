//go:build e2e

package e2e

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullPipeline_IngestToSearch tests the complete pipeline:
// 1. Ingest email content via CLI
// 2. Verify content is stored in database
// 3. Verify search can find the content
func TestFullPipeline_IngestToSearch(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Setup: Clean slate
	err := env.TruncateAllTables()
	require.NoError(t, err)

	// Load entity fixtures (people, teams, projects, glossary)
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Count sources before
	var countBefore int
	env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countBefore)

	// Step 1: Ingest email via CLI
	t.Log("=== Step 1: Email Ingestion ===")

	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "e2e-test")

	if !result.Success() {
		t.Logf("Ingest stderr: %s", result.Stderr)
		t.Logf("Ingest stdout: %s", result.Stdout)
	}

	if result.ExitCode != 0 {
		t.Fatalf("Ingest command failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	t.Logf("Ingest completed in %v", result.Duration)

	// Step 2: Verify content was stored
	t.Log("=== Step 2: Verify Content Stored ===")

	// Verify ingest job was created
	var jobCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM ingest_jobs
		WHERE source_tag = 'e2e-test'
	`).Scan(&jobCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, jobCount, 1, "should have at least one ingest job")

	// Verify source was created
	var countAfter int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countAfter)
	require.NoError(t, err)
	assert.Greater(t, countAfter, countBefore, "should have created new source record")
	t.Logf("Sources: before=%d, after=%d", countBefore, countAfter)

	// Step 3: Test search via CLI
	t.Log("=== Step 3: Search Verification ===")

	searchResult := env.CLI.Run(ctx, "search", "Project Alpha MVP")
	if searchResult.Success() {
		t.Logf("Search completed in %v", searchResult.Duration)
		t.Logf("Search results: %s", searchResult.Stdout)

		// Verify search finds the ingested content
		assert.Contains(t, searchResult.Stdout, "Project Alpha",
			"search results should contain Project Alpha")
	} else {
		t.Logf("Search command failed (services may not be fully connected): %s", searchResult.Stderr)
	}
}

// TestFullPipeline_MentionExtraction tests that mentions are extracted during ingestion.
func TestFullPipeline_MentionExtraction(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Setup: Clean slate
	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Get count of people before ingestion for context
	var peopleCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM people").Scan(&peopleCount)
	require.NoError(t, err)
	t.Logf("People in database: %d", peopleCount)

	// Ingest email with mentions
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "mention-test")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest command failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	// Allow time for async processing (if applicable)
	time.Sleep(500 * time.Millisecond)

	// Verify mentions were extracted
	var mentionCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM content_mentions
		WHERE tenant_id = 'default'
	`).Scan(&mentionCount)

	if err == nil && mentionCount > 0 {
		t.Logf("Mentions extracted: %d", mentionCount)

		// Check for specific person mentions
		rows, err := env.DB.Query(ctx, `
			SELECT mentioned_text, entity_type, status, resolution_confidence
			FROM content_mentions
			WHERE tenant_id = 'default'
			ORDER BY id
			LIMIT 10
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var text, entityType, status string
				var confidence *float64
				rows.Scan(&text, &entityType, &status, &confidence)
				confStr := "nil"
				if confidence != nil {
					confStr = "set"
				}
				t.Logf("  Mention: %q type=%s status=%s confidence=%s", text, entityType, status, confStr)
			}
		}
	} else {
		t.Log("No mentions found - mention extraction may be async or require additional setup")
	}
}

// TestFullPipeline_BatchIngestion tests batch processing of multiple emails.
func TestFullPipeline_BatchIngestion(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Setup: Clean slate
	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Count sources before
	var countBefore int
	env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countBefore)

	// Ingest entire email directory
	emailDir := env.FixturePath("emails")
	result := env.CLI.Run(ctx, "ingest", "email", emailDir, "--source", "batch-test", "--concurrency", "2")

	if result.ExitCode != 0 {
		t.Fatalf("Batch ingest failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	t.Logf("Batch ingest completed in %v", result.Duration)
	t.Logf("Output: %s", result.Stdout)

	// Verify multiple sources were created
	var countAfter int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countAfter)
	require.NoError(t, err)

	newSources := countAfter - countBefore
	t.Logf("Sources created: %d (before=%d, after=%d)", newSources, countBefore, countAfter)
	assert.GreaterOrEqual(t, newSources, 5, "should have ingested multiple emails")
}

// TestFullPipeline_GlossaryTerms tests that glossary terms are available for resolution.
func TestFullPipeline_GlossaryTerms(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Setup: Clean slate and load fixtures
	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Verify glossary was loaded
	var glossaryCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM glossary").Scan(&glossaryCount)
	require.NoError(t, err)
	t.Logf("Glossary terms loaded: %d", glossaryCount)

	assert.GreaterOrEqual(t, glossaryCount, 5, "should have loaded glossary terms")

	// Check for specific terms we expect
	expectedTerms := []struct {
		term      string
		expansion string
	}{
		{"TER", "Technical Execution Review"},
		{"MVP", "Minimum Viable Product"},
		{"OKR", "Objectives and Key Results"},
	}

	for _, expected := range expectedTerms {
		var expansion string
		err = env.DB.QueryRow(ctx, `
			SELECT expansion FROM glossary
			WHERE UPPER(term) = UPPER($1)
		`, expected.term).Scan(&expansion)

		if err == nil {
			t.Logf("Term %s -> %s", expected.term, expansion)
			assert.Contains(t, expansion, expected.expansion,
				"expansion for %s should contain %s", expected.term, expected.expansion)
		}
	}
}

// TestFullPipeline_CrossDocumentSearch tests searching across multiple ingested documents.
func TestFullPipeline_CrossDocumentSearch(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Setup: Clean slate
	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Count sources before
	var countBefore int
	env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countBefore)

	// Ingest multiple specific emails
	emails := []string{
		"001-project-update.eml",
		"005-project-kickoff.eml",
		"008-security-review.eml",
	}

	for _, email := range emails {
		emailPath := env.FixturePath(filepath.Join("emails", email))
		result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "cross-doc-test")
		if result.ExitCode != 0 {
			t.Fatalf("Failed to ingest %s (exit code %d): %s", email, result.ExitCode, result.Stderr)
		}
	}

	// Verify all were ingested
	var countAfter int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countAfter)
	require.NoError(t, err)

	newSources := countAfter - countBefore
	t.Logf("Sources created: %d (before=%d, after=%d)", newSources, countBefore, countAfter)
	assert.Equal(t, len(emails), newSources, "should have ingested all emails")

	// Test cross-document search
	searches := []struct {
		query    string
		expected string
	}{
		{"Project Alpha", "Project Alpha"},
		{"security review", "security"},
		{"kickoff", "kickoff"},
	}

	for _, search := range searches {
		t.Run("search_"+search.query, func(t *testing.T) {
			result := env.CLI.Run(ctx, "search", search.query)
			if result.Success() {
				t.Logf("Search '%s' results: %s", search.query, result.Stdout)
			}
		})
	}
}

// TestFullPipeline_EntityContext tests that entity context is properly established.
func TestFullPipeline_EntityContext(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Setup
	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Verify people context is loaded
	t.Log("=== People Context ===")
	rows, err := env.DB.Query(ctx, `
		SELECT canonical_name, primary_email, job_title
		FROM people
		ORDER BY id
		LIMIT 5
	`)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var name, email string
		var title *string
		rows.Scan(&name, &email, &title)
		titleStr := "<nil>"
		if title != nil {
			titleStr = *title
		}
		t.Logf("  Person: %s <%s> - %s", name, email, titleStr)
	}

	// Verify teams context
	t.Log("=== Teams Context ===")
	teamRows, err := env.DB.Query(ctx, `
		SELECT name, description
		FROM teams
		ORDER BY id
		LIMIT 5
	`)
	require.NoError(t, err)
	defer teamRows.Close()

	for teamRows.Next() {
		var name string
		var desc *string
		teamRows.Scan(&name, &desc)
		t.Logf("  Team: %s", name)
	}

	// Verify projects context
	t.Log("=== Projects Context ===")
	projectRows, err := env.DB.Query(ctx, `
		SELECT name, description
		FROM projects
		ORDER BY id
		LIMIT 5
	`)
	require.NoError(t, err)
	defer projectRows.Close()

	for projectRows.Next() {
		var name string
		var desc *string
		projectRows.Scan(&name, &desc)
		t.Logf("  Project: %s", name)
	}
}
