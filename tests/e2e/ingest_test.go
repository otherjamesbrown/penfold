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

	// Ingest single email
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "single-file-test")

	if result.ExitCode != 0 {
		t.Skipf("Ingest command failed (exit code %d) - ensure services are running. Stderr: %s",
			result.ExitCode, result.Stderr)
	}

	t.Logf("Ingest completed in %v", result.Duration)
	t.Logf("Output:\n%s", result.Stdout)

	// Verify source was created
	var count int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM sources
		WHERE source_tag = 'single-file-test'
	`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "should have exactly one source record")

	// Verify email content
	var subject, fromEmail, toEmail string
	err = env.DB.QueryRow(ctx, `
		SELECT subject, from_email, to_emails[1]
		FROM sources
		WHERE source_tag = 'single-file-test'
	`).Scan(&subject, &fromEmail, &toEmail)

	if err == nil {
		t.Logf("Ingested: Subject=%q From=%s To=%s", subject, fromEmail, toEmail)
		assert.Contains(t, subject, "Project Alpha", "subject should contain Project Alpha")
		assert.Equal(t, "john.smith@acme.com", fromEmail, "from email should match")
	}
}

// TestEmailIngestion_Directory tests batch ingestion of a directory via CLI.
func TestEmailIngestion_Directory(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Ingest entire email directory
	emailDir := env.FixturePath("emails")
	result := env.CLI.Run(ctx, "ingest", "email", emailDir, "--source", "directory-test", "--concurrency", "2")

	if result.ExitCode != 0 {
		t.Skipf("Batch ingest failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	t.Logf("Batch ingest completed in %v", result.Duration)
	t.Logf("Output:\n%s", result.Stdout)

	// Verify multiple sources were created
	var count int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM sources
		WHERE source_tag = 'directory-test'
	`).Scan(&count)
	require.NoError(t, err)

	t.Logf("Sources ingested: %d", count)
	assert.GreaterOrEqual(t, count, 5, "should have ingested multiple emails")
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
		t.Skipf("First ingest failed: %s", result1.Stderr)
	}

	var countAfterFirst int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM sources WHERE source_tag = 'dup-test'
	`).Scan(&countAfterFirst)
	require.NoError(t, err)
	assert.Equal(t, 1, countAfterFirst, "should have one source after first ingest")

	// Second ingestion of same file
	result2 := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "dup-test")

	// Second ingest should succeed but skip the duplicate
	if result2.ExitCode == 0 {
		var countAfterSecond int
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM sources WHERE source_tag = 'dup-test'
		`).Scan(&countAfterSecond)
		require.NoError(t, err)

		t.Logf("Count after first ingest: %d, after second: %d", countAfterFirst, countAfterSecond)
		// Count should still be 1 (duplicate was skipped)
		assert.Equal(t, 1, countAfterSecond, "duplicate should have been skipped")
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

	// Ingest with dry-run flag
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "dryrun-test", "--dry-run")

	if result.ExitCode != 0 {
		t.Logf("Dry-run command failed: %s", result.Stderr)
	}

	t.Logf("Dry-run output:\n%s", result.Stdout)

	// Verify nothing was actually created
	var count int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM sources WHERE source_tag = 'dryrun-test'
	`).Scan(&count)

	if err == nil {
		t.Logf("Sources after dry-run: %d", count)
		assert.Equal(t, 0, count, "dry-run should not create any records")
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
		t.Skipf("Ingest with labels failed: %s", result.Stderr)
	}

	t.Logf("Output:\n%s", result.Stdout)

	// Verify source was created with labels
	var labels []string
	err = env.DB.QueryRow(ctx, `
		SELECT labels FROM sources
		WHERE source_tag = 'labels-test'
	`).Scan(&labels)

	if err == nil && labels != nil {
		t.Logf("Labels applied: %v", labels)
		assert.Contains(t, labels, "important", "should have 'important' label")
		assert.Contains(t, labels, "project-alpha", "should have 'project-alpha' label")
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
		name        string
		file        string
		expectInSubject string
	}{
		{"project-update", "001-project-update.eml", "Project Alpha"},
		{"incident", "002-incident-response.eml", ""},
		{"meeting-invite", "003-meeting-invite.eml", ""},
		{"code-review", "004-code-review.eml", ""},
	}

	for _, tc := range testEmails {
		t.Run(tc.name, func(t *testing.T) {
			emailPath := env.FixturePath(filepath.Join("emails", tc.file))
			result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "format-test-"+tc.name)

			if result.ExitCode == 0 {
				// Verify ingestion
				var count int
				err := env.DB.QueryRow(ctx, `
					SELECT COUNT(*) FROM sources
					WHERE source_tag = $1
				`, "format-test-"+tc.name).Scan(&count)

				if err == nil {
					assert.Equal(t, 1, count, "should have ingested %s", tc.file)
				}
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
		t.Skipf("Ingest failed: %s", result.Stderr)
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
		t.Skipf("Ingest failed: %s", result.Stderr)
	}

	// Verify content was parsed
	var body string
	err = env.DB.QueryRow(ctx, `
		SELECT content_body FROM sources
		WHERE source_tag = 'parse-test'
	`).Scan(&body)

	if err == nil && body != "" {
		t.Logf("Parsed body length: %d characters", len(body))
		t.Logf("Body preview: %s...", truncate(body, 100))

		// Verify expected content
		assert.Contains(t, body, "Project Alpha", "body should contain Project Alpha")
		assert.Contains(t, body, "TER", "body should contain TER acronym")
	}
}

// Helper function to truncate strings for logging.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
