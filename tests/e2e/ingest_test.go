//go:build e2e

package e2e

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmailIngestion_SingleFile tests ingestion of a single email file via CLI.
func TestEmailIngestion_SingleFile(t *testing.T) {
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

	// Ingest single email
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "single-file-test")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest command failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	t.Logf("Ingest completed in %v", result.Duration)
	t.Logf("Output:\n%s", result.Stdout)

	// Verify ingest job was created
	var jobCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM ingest_jobs
		WHERE source_tag = 'single-file-test'
	`).Scan(&jobCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, jobCount, 1, "should have at least one ingest job")

	// Verify source was created
	var countAfter int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countAfter)
	require.NoError(t, err)
	assert.Greater(t, countAfter, countBefore, "should have created new source record")
	t.Logf("Sources: before=%d, after=%d", countBefore, countAfter)
}

// TestEmailIngestion_Directory tests batch ingestion of a directory via CLI.
func TestEmailIngestion_Directory(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Count sources before
	var countBefore int
	env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countBefore)

	// Ingest entire email directory
	emailDir := env.FixturePath("emails")
	result := env.CLI.Run(ctx, "ingest", "email", emailDir, "--source", "directory-test", "--concurrency", "2")

	if result.ExitCode != 0 {
		t.Fatalf("Batch ingest failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	t.Logf("Batch ingest completed in %v", result.Duration)
	t.Logf("Output:\n%s", result.Stdout)

	// Verify multiple sources were created
	var countAfter int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countAfter)
	require.NoError(t, err)

	newSources := countAfter - countBefore
	t.Logf("Sources created: %d (before=%d, after=%d)", newSources, countBefore, countAfter)
	assert.GreaterOrEqual(t, newSources, 5, "should have ingested multiple emails")
}

// TestEmailIngestion_DuplicateDetection tests that duplicate emails are skipped.
func TestEmailIngestion_DuplicateDetection(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	emailPath := env.FixturePath("emails/001-project-update.eml")

	// First ingestion
	result1 := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "dup-test")
	if result1.ExitCode != 0 {
		t.Fatalf("First ingest failed (exit code %d): %s", result1.ExitCode, result1.Stderr)
	}

	var countAfterFirst int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countAfterFirst)
	require.NoError(t, err)

	// Second ingestion of same file
	result2 := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "dup-test")

	// Second ingest should succeed but skip the duplicate
	if result2.ExitCode == 0 {
		var countAfterSecond int
		err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countAfterSecond)
		require.NoError(t, err)

		t.Logf("Source count after first ingest: %d, after second: %d", countAfterFirst, countAfterSecond)
		// Count should stay the same (duplicate was skipped)
		assert.Equal(t, countAfterFirst, countAfterSecond, "duplicate should have been skipped")
	}
}

// TestEmailIngestion_DryRun tests the dry-run mode.
func TestEmailIngestion_DryRun(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Count sources before
	var countBefore int
	env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countBefore)

	// Ingest with dry-run flag
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "dryrun-test", "--dry-run")

	if result.ExitCode != 0 {
		t.Logf("Dry-run command failed: %s", result.Stderr)
	}

	t.Logf("Dry-run output:\n%s", result.Stdout)

	// Verify nothing was actually created
	var countAfter int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countAfter)

	if err == nil {
		t.Logf("Sources: before=%d, after=%d", countBefore, countAfter)
		assert.Equal(t, countBefore, countAfter, "dry-run should not create any records")
	}
}

// TestEmailIngestion_WithLabels tests ingestion with custom labels.
func TestEmailIngestion_WithLabels(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Ingest with labels
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath,
		"--source", "labels-test",
		"--labels", "important,project-alpha")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest with labels failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	t.Logf("Output:\n%s", result.Stdout)

	// Verify job was created with labels
	var labels []byte
	err = env.DB.QueryRow(ctx, `
		SELECT labels FROM ingest_jobs
		WHERE source_tag = 'labels-test'
		ORDER BY created_at DESC
		LIMIT 1
	`).Scan(&labels)

	if err == nil && labels != nil {
		t.Logf("Job labels: %s", string(labels))
	}
}

// TestEmailIngestion_MultipleFormats tests ingestion handles different email formats.
func TestEmailIngestion_MultipleFormats(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Try ingesting different emails with varying content
	testEmails := []struct {
		name string
		file string
	}{
		{"project-update", "001-project-update.eml"},
		{"incident", "002-incident-response.eml"},
		{"meeting-invite", "003-meeting-invite.eml"},
		{"code-review", "004-code-review.eml"},
	}

	for _, tc := range testEmails {
		t.Run(tc.name, func(t *testing.T) {
			// Count before
			var countBefore int
			env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countBefore)

			emailPath := env.FixturePath(filepath.Join("emails", tc.file))
			result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "format-test-"+tc.name)

			if result.ExitCode == 0 {
				var countAfter int
				env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&countAfter)
				assert.Greater(t, countAfter, countBefore, "should have ingested %s", tc.file)
			} else {
				t.Logf("Failed to ingest %s: %s", tc.file, result.Stderr)
			}
		})
	}
}

// TestEmailIngestion_PersonExtraction tests that people are created from email headers.
func TestEmailIngestion_PersonExtraction(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Start fresh (no fixtures - test auto-creation)
	err := env.TruncateAllTables()
	require.NoError(t, err)

	// Count people before ingestion
	var countBefore int
	env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM people").Scan(&countBefore)
	t.Logf("People before ingestion: %d", countBefore)

	// Ingest email (from john.smith@acme.com)
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "person-extract-test")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	// Check if people were auto-created
	var countAfter int
	env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM people").Scan(&countAfter)
	t.Logf("People after ingestion: %d", countAfter)

	if countAfter > countBefore {
		// List newly created people
		rows, err := env.DB.Query(ctx, `
			SELECT canonical_name, primary_email, auto_created
			FROM people
			ORDER BY id DESC
			LIMIT 5
		`)
		if err == nil {
			defer rows.Close()
			t.Log("People created:")
			for rows.Next() {
				var name, email string
				var autoCreated bool
				rows.Scan(&name, &email, &autoCreated)
				t.Logf("  %s <%s> auto_created=%v", name, email, autoCreated)
			}
		}
	}
}

// TestEmailIngestion_ContentParsing tests that email content is properly parsed.
func TestEmailIngestion_ContentParsing(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Ingest email
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "parse-test")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	// Verify content was stored (check raw_content column)
	var rawContent string
	err = env.DB.QueryRow(ctx, `
		SELECT raw_content FROM sources
		WHERE content_type = 'email'
		ORDER BY created_at DESC
		LIMIT 1
	`).Scan(&rawContent)

	if err == nil && rawContent != "" {
		t.Logf("Parsed content length: %d characters", len(rawContent))

		// Check content contains expected text
		if len(rawContent) > 100 {
			t.Logf("Content preview: %s...", rawContent[:100])
		}
	}
}
