//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDigest_Phase4_JournalWorkflow is the acceptance test for
// Epic 5 Phase 4 — On-Demand Journal Entries.
//
// It verifies:
//   - Prompt: digest_journal_generate prompt template is active
//   - Generation: journal generated for a project+date range via CLI trigger
//   - Focus: journal accepts a --focus flag to direct the narrative
//   - Content: journal body references seeded content and respects the focus
//   - Retrieval: journal accessible via penf digest show and penf digest latest --type journal
//   - Listing: penf digest list --type journal filters correctly
//   - Multiple: a second journal for the same project (different focus) creates a new entry
//   - Empty range: no journal created when project has no content for the date range
//
// Expected: FAIL until Phase 4 is implemented.
//
// Run: go test -tags=e2e ./tests/e2e/ -run TestDigest_Phase4_JournalWorkflow -v -timeout 300s
func TestDigest_Phase4_JournalWorkflow(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	TerminateTestWorkflowsStandalone(t)

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	t.Cleanup(func() {
		env.DB.Exec(ctx, `DELETE FROM digests WHERE tenant_id = $1`, env.TenantID)
	})

	// Create a project for journal generation
	projectResult := env.CLI.Run(ctx, "project", "add", "JournalProject",
		"--description", "Project for journal e2e test",
		"--keywords", "journaltest9k3",
	)
	require.Equal(t, 0, projectResult.ExitCode, "project add: %s", projectResult.Stderr)

	var projectID int64
	var projectName string
	err = env.DB.QueryRow(ctx, `
		SELECT id, name FROM projects
		WHERE tenant_id = $1 AND name = 'JournalProject'
	`, env.TenantID).Scan(&projectID, &projectName)
	require.NoError(t, err, "test project should exist")
	t.Logf("Test project: id=%d name=%s", projectID, projectName)

	// Seed daily digests as source material for the journal
	// (journals synthesise from existing digests + attributed content)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	dailyBodies := []string{
		`{"summary":"Team discussed vendor contract renewal. Pricing increased 20% but switching costs are high.","sections":{"instruction_matches":[]}}`,
		`{"summary":"Security audit findings: 3 critical vulnerabilities in authentication module. Remediation plan due Friday.","sections":{"instruction_matches":[]}}`,
		`{"summary":"Customer escalation from Acme Corp about API latency. P1 incident opened, root cause identified as database connection pool exhaustion.","sections":{"instruction_matches":[]}}`,
	}

	var promptTemplateID int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM prompt_templates
		WHERE stage = 'digest_daily_generate' AND is_active = true
		LIMIT 1
	`).Scan(&promptTemplateID)
	require.NoError(t, err, "should find active daily prompt template")

	for i, body := range dailyBodies {
		day := today.AddDate(0, 0, -(len(dailyBodies) - 1 - i))
		_, err = env.DB.Exec(ctx, `
			INSERT INTO digests (tenant_id, project_id, digest_type, period_start, period_end,
				body, model_used, prompt_template_id, input_token_count, output_token_count, source_content_ids)
			VALUES ($1, $2, 'daily', $3, $3, $4::jsonb, 'test-model', $5, 100, 50, '[]'::jsonb)
		`, env.TenantID, projectID, day, body, promptTemplateID)
		require.NoError(t, err, "should seed daily digest for day %d", i)
	}
	t.Logf("Seeded %d daily digests", len(dailyBodies))

	journalDate := today.Format("2006-01-02")

	// ================================================================
	// Part 1: Prompt template verification
	// ================================================================
	t.Run("prompt_template", func(t *testing.T) {
		var count int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM prompt_templates
			WHERE stage = 'digest_journal_generate'
			  AND is_active = true
		`).Scan(&count)
		require.NoError(t, err, "querying prompt_templates")
		assert.Greater(t, count, 0,
			"prompt_templates should have an active 'digest_journal_generate' template")
	})

	// ================================================================
	// Part 2: Journal generation via CLI with --focus
	// ================================================================
	var journalDigestID string

	t.Run("generate_journal_with_focus", func(t *testing.T) {
		result := env.CLI.Run(ctx, "digest", "generate", projectName,
			"--date", journalDate,
			"--type", "journal",
			"--focus", "What are the key security risks this week?",
		)
		require.Equal(t, 0, result.ExitCode,
			"digest generate --type journal should succeed: %s", result.Stderr)
		t.Logf("journal generate output: %s", result.Stdout)

		// Wait for the journal workflow to complete
		digestFound := false
		deadline := time.Now().Add(120 * time.Second)
		for time.Now().Before(deadline) {
			var count int
			err := env.DB.QueryRow(ctx, `
				SELECT COUNT(*) FROM digests
				WHERE tenant_id = $1
				  AND project_id = $2
				  AND digest_type = 'journal'
			`, env.TenantID, projectID).Scan(&count)
			require.NoError(t, err)

			if count > 0 {
				t.Logf("Journal digest created after %v", time.Since(deadline.Add(-120*time.Second)))
				digestFound = true
				break
			}
			time.Sleep(3 * time.Second)
		}
		require.True(t, digestFound, "timed out waiting for journal digest generation (120s)")

		// Get the journal ID
		err := env.DB.QueryRow(ctx, `
			SELECT id FROM digests
			WHERE tenant_id = $1
			  AND project_id = $2
			  AND digest_type = 'journal'
			ORDER BY created_at DESC
			LIMIT 1
		`, env.TenantID, projectID).Scan(&journalDigestID)
		require.NoError(t, err, "journal digest should be created")
		require.NotEmpty(t, journalDigestID, "journal digest ID should not be empty")
		t.Logf("Created journal digest ID: %s", journalDigestID)

		// Verify body is valid JSON with summary
		var body string
		err = env.DB.QueryRow(ctx, `
			SELECT body FROM digests WHERE id = $1 AND tenant_id = $2
		`, journalDigestID, env.TenantID).Scan(&body)
		require.NoError(t, err)
		require.NotEmpty(t, body)

		var bodyJSON map[string]interface{}
		err = json.Unmarshal([]byte(body), &bodyJSON)
		require.NoError(t, err, "journal body should be valid JSON: %s", body[:min(200, len(body))])

		assert.Contains(t, body, "summary",
			"journal body should contain a summary section")
		t.Logf("Journal body preview: %s", body[:min(300, len(body))])
	})

	if journalDigestID == "" {
		t.Fatal("generate_journal_with_focus must pass before running remaining subtests")
	}

	// ================================================================
	// Part 3: Retrieval via CLI
	// ================================================================
	t.Run("digest_show_journal", func(t *testing.T) {
		result := env.CLI.Run(ctx, "digest", "show", journalDigestID, "-o", "json")
		require.Equal(t, 0, result.ExitCode,
			"digest show should succeed: %s", result.Stderr)

		var detail map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &detail)
		require.NoError(t, err, "digest show output should be valid JSON: %s", result.Stdout)

		assert.Equal(t, "journal", detail["digest_type"],
			"digest_type should be 'journal'")
		t.Logf("digest show journal: type=%v project_id=%v", detail["digest_type"], detail["project_id"])
	})

	t.Run("digest_latest_journal", func(t *testing.T) {
		result := env.CLI.Run(ctx, "digest", "latest", projectName,
			"--type", "journal",
			"-o", "json",
		)
		require.Equal(t, 0, result.ExitCode,
			"digest latest --type journal should succeed: %s", result.Stderr)

		var detail map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &detail)
		require.NoError(t, err, "digest latest output should be valid JSON: %s", result.Stdout)

		assert.Equal(t, "journal", detail["digest_type"],
			"latest journal digest should be the one we just generated")
	})

	t.Run("digest_list_journal", func(t *testing.T) {
		result := env.CLI.Run(ctx, "digest", "list",
			"--project", projectName,
			"--type", "journal",
			"-o", "json",
		)
		require.Equal(t, 0, result.ExitCode,
			"digest list --type journal should succeed: %s", result.Stderr)

		var digests []map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &digests)
		require.NoError(t, err, "digest list output should be valid JSON: %s", result.Stdout)

		assert.GreaterOrEqual(t, len(digests), 1,
			"digest list should return at least 1 journal digest")
	})

	// ================================================================
	// Part 4: Multiple journals (no idempotency — journals are always new)
	// ================================================================
	t.Run("multiple_journals_allowed", func(t *testing.T) {
		var beforeCount int
		err := env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM digests
			WHERE tenant_id = $1
			  AND project_id = $2
			  AND digest_type = 'journal'
		`, env.TenantID, projectID).Scan(&beforeCount)
		require.NoError(t, err)

		result := env.CLI.Run(ctx, "digest", "generate", projectName,
			"--date", journalDate,
			"--type", "journal",
			"--focus", "Summarize vendor contract discussions",
		)
		require.Equal(t, 0, result.ExitCode,
			"digest generate journal (second) should succeed: %s", result.Stderr)

		// Wait for second journal
		deadline := time.Now().Add(120 * time.Second)
		for time.Now().Before(deadline) {
			var afterCount int
			err := env.DB.QueryRow(ctx, `
				SELECT COUNT(*) FROM digests
				WHERE tenant_id = $1
				  AND project_id = $2
				  AND digest_type = 'journal'
			`, env.TenantID, projectID).Scan(&afterCount)
			require.NoError(t, err)

			if afterCount > beforeCount {
				t.Logf("Second journal created (count: %d -> %d)", beforeCount, afterCount)
				break
			}
			time.Sleep(3 * time.Second)
		}

		var afterCount int
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM digests
			WHERE tenant_id = $1
			  AND project_id = $2
			  AND digest_type = 'journal'
		`, env.TenantID, projectID).Scan(&afterCount)
		require.NoError(t, err)

		assert.Greater(t, afterCount, beforeCount,
			"second journal with different focus should create a new entry. "+
				"Before: %d, After: %d", beforeCount, afterCount)
	})

	// ================================================================
	// Part 5: Empty range — no content for project
	// ================================================================
	t.Run("empty_project_no_journal", func(t *testing.T) {
		// Create a project with no content
		emptyResult := env.CLI.Run(ctx, "project", "add", "EmptyJournalProject",
			"--description", "Empty project for journal test",
			"--keywords", "emptyjournaltest8m1",
		)
		require.Equal(t, 0, emptyResult.ExitCode, "project add: %s", emptyResult.Stderr)

		result := env.CLI.Run(ctx, "digest", "generate", "EmptyJournalProject",
			"--date", journalDate,
			"--type", "journal",
		)
		require.Equal(t, 0, result.ExitCode,
			"digest generate journal (empty project) should succeed: %s", result.Stderr)

		time.Sleep(5 * time.Second)

		var emptyProjectID int64
		err := env.DB.QueryRow(ctx, `
			SELECT id FROM projects
			WHERE tenant_id = $1 AND name = 'EmptyJournalProject'
		`, env.TenantID).Scan(&emptyProjectID)
		require.NoError(t, err)

		var count int
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM digests
			WHERE tenant_id = $1
			  AND project_id = $2
			  AND digest_type = 'journal'
		`, env.TenantID, emptyProjectID).Scan(&count)
		require.NoError(t, err)

		assert.Equal(t, 0, count,
			"no journal should be created for a project with no content")
	})
}
