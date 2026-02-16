//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Bug: Briefing SQL references a.type instead of a.assertion_type
// ============================================================================
//
// `penf briefing MTC` fails with:
//   ERROR: column a.type does not exist (SQLSTATE 42703)
//
// The assertions table has column `assertion_type`, not `type`.
// The briefing query in pkg/watchlist/repository.go uses `a.type` on lines
// 265 and 278 (in the trust_domains check).
//
// Every other query in the codebase uses `a.assertion_type` correctly.
// This query was likely written before the column was named, or copied
// from a different context.
//
// Fix: Replace `a.type` with `a.assertion_type` in GetBriefingAssertions().
// File: pkg/watchlist/repository.go:265,278
//
// Bug shard: pf-XXXXXX
// ============================================================================

// TestE2E_Briefing_DoesNotCrashOnValidProject verifies that `penf briefing`
// executes without SQL errors when called with a valid project name.
//
// This catches the a.type vs a.assertion_type column name mismatch.
func TestE2E_Briefing_DoesNotCrashOnValidProject(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Ensure test tenant and project exist
	err := env.EnsureTenantExists()
	require.NoError(t, err)

	testID := fmt.Sprintf("e2e-briefing-%d", time.Now().UnixNano())
	projectName := fmt.Sprintf("BriefingTest-%s", testID[:16])

	// Create a test project via CLI
	result := env.SafeCLI.RunProjectAdd(ctx, projectName, "Test project for briefing", []string{"brieftest"})
	require.True(t, result.Success(), "project add should succeed: %v\nstderr: %s", result.Err, result.Stderr)
	t.Logf("Created project %q via CLI", projectName)

	// Ingest an email mentioning the project keyword to create assertions
	// The pipeline will create person, source, and assertions automatically
	opts := emailSourceOpts{
		MessageID:         fmt.Sprintf("<briefing-%s@e2e.test>", testID),
		Subject:           fmt.Sprintf("Update on %s deliverables", projectName),
		FromAddress:       "briefing-sender@example.com",
		FromName:          "Briefing Sender",
		Body:              fmt.Sprintf("The %s is progressing well. We need to address the brieftest issues.", projectName),
		Date:              time.Now().Add(-1 * time.Hour),
		ExternalID:        fmt.Sprintf("briefing-%s", testID),
		ParticipantEmails: []string{"briefing-sender@example.com"},
	}

	sourceID := createEmailSource(t, env, opts)
	t.Logf("Ingested email (source %d) mentioning project", sourceID)

	// Wait for pipeline to complete
	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	t.Logf("Created project %q with email ingested", projectName)

	// Run penf briefing — this should NOT crash with SQL error
	t.Run("briefing_succeeds", func(t *testing.T) {
		result := env.SafeCLI.Run(ctx, "briefing", projectName)
		assert.Equal(t, 0, result.ExitCode,
			"penf briefing should succeed, got error: %s", result.Stderr)
		assert.NotContains(t, result.Stderr, "a.type",
			"should not have SQL error about a.type column")
		assert.NotContains(t, result.Stderr, "does not exist",
			"should not have SQL column error")
	})

	t.Run("briefing_json_returns_assertions", func(t *testing.T) {
		result := env.SafeCLI.Run(ctx, "briefing", projectName, "-o", "json")
		if result.ExitCode != 0 {
			t.Fatalf("penf briefing -o json failed: %s", result.Stderr)
		}

		var resp map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &resp)
		require.NoError(t, err, "briefing should return valid JSON")

		assertions, ok := resp["assertions"].([]interface{})
		if ok {
			assert.GreaterOrEqual(t, len(assertions), 1,
				"briefing should return at least our test assertion")
		}
	})
}

// TestE2E_Briefing_ProjectWithAssertions verifies the full briefing flow:
// project with keyword-tagged assertions returns prioritized results.
func TestE2E_Briefing_ProjectWithAssertions(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	testID := fmt.Sprintf("e2e-briefing-full-%d", time.Now().UnixNano())
	projectName := fmt.Sprintf("BriefFull-%s", testID[:12])

	// Create a project with keywords via CLI
	result := env.SafeCLI.RunProjectAdd(ctx, projectName, "Full briefing test", []string{"brieftest"})
	require.True(t, result.Success(), "project add should succeed: %v\nstderr: %s", result.Err, result.Stderr)

	// Ingest an email mentioning the project keyword
	sourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:         fmt.Sprintf("<briefing-full-%s@e2e.test>", testID),
		Subject:           "Update on brieftest deliverables",
		FromAddress:       "manager@example.com",
		FromName:          "Manager",
		Body:              "The brieftest project is on track for Q2 delivery.",
		Date:              time.Now().Add(-1 * time.Hour),
		ExternalID:        fmt.Sprintf("briefing-full-%s", testID),
		ParticipantEmails: []string{"manager@example.com"},
	})

	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	// The pipeline should have created assertions with project_id set
	// (via PersistFindings which resolves project keywords)
	// Look up project ID by name
	var projectID int64
	err := env.DB.QueryRow(ctx, `
		SELECT id FROM projects WHERE name = $1 AND tenant_id = $2
	`, projectName, testTenantID(env)).Scan(&projectID)
	require.NoError(t, err, "project should exist")

	var assertionCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM assertions
		WHERE source_id = $1 AND project_id = $2 AND tenant_id = $3
	`, sourceID, projectID, testTenantID(env)).Scan(&assertionCount)
	require.NoError(t, err)

	if assertionCount == 0 {
		t.Skip("No assertions created with project_id — keyword matching may not have resolved")
	}

	t.Logf("Pipeline created %d assertions tagged with project %d", assertionCount, projectID)

	// Briefing should return these assertions without SQL errors
	t.Run("briefing_returns_project_assertions", func(t *testing.T) {
		result := env.SafeCLI.Run(ctx, "briefing", projectName)
		assert.Equal(t, 0, result.ExitCode,
			"penf briefing should succeed: %s", result.Stderr)
	})
}

// TestE2E_Briefing_MTC_RealData verifies that `penf briefing MTC` works
// against real production data (9 assertions tagged with project 4641).
// This test uses the production tenant and existing data.
func TestE2E_Briefing_MTC_RealData(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Verify MTC project exists with assertions
	var count int
	err := env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM assertions WHERE project_id = 4641
	`).Scan(&count)
	require.NoError(t, err)

	if count == 0 {
		t.Skip("No MTC assertions in database — skipping real data test")
	}

	t.Logf("Found %d assertions tagged with MTC project", count)

	result := env.SafeCLI.Run(ctx, "briefing", "MTC")
	assert.Equal(t, 0, result.ExitCode,
		"penf briefing MTC should not crash with SQL error: %s", result.Stderr)
	assert.NotContains(t, result.Stderr, "does not exist",
		"should not have column reference error")
}
