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
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Ensure test tenant and project exist
	err := env.EnsureTenantExists()
	require.NoError(t, err)

	testID := fmt.Sprintf("e2e-briefing-%d", time.Now().UnixNano())
	projectName := fmt.Sprintf("BriefingTest-%s", testID[:16])

	// Create a test project
	var projectID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO projects (tenant_id, name, description, keywords, created_at, updated_at)
		VALUES ($1, $2, 'Test project for briefing', '{"test"}', NOW(), NOW())
		RETURNING id
	`, env.TenantID, projectName).Scan(&projectID)
	require.NoError(t, err, "failed to create test project")

	t.Cleanup(func() {
		env.DB.Exec(ctx, "DELETE FROM assertions WHERE project_id = $1 AND tenant_id = $2", projectID, env.TenantID)
		env.DB.Exec(ctx, "DELETE FROM projects WHERE id = $1 AND tenant_id = $2", projectID, env.TenantID)
	})

	// Create a test person (required for the JOIN in briefing query)
	var personID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO people (tenant_id, canonical_name, created_at, updated_at)
		VALUES ($1, 'Briefing Test Person', NOW(), NOW())
		RETURNING id
	`, env.TenantID).Scan(&personID)
	require.NoError(t, err)
	t.Cleanup(func() {
		env.DB.Exec(ctx, "DELETE FROM people WHERE id = $1", personID)
	})

	// Create a test source (required FK for assertions)
	var sourceID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO sources (
			tenant_id, source_system, external_id, content_hash,
			raw_content, content_type, content_size,
			processing_status, source_timestamp
		) VALUES (
			$1, 'test', $2, encode(sha256($3::bytea), 'hex'),
			$3, 'text/plain', 10,
			'completed', NOW()
		) RETURNING id
	`, env.TenantID, fmt.Sprintf("briefing-src-%s", testID),
		"briefing test content",
	).Scan(&sourceID)
	require.NoError(t, err)
	t.Cleanup(func() {
		env.DB.Exec(ctx, "DELETE FROM sources WHERE id = $1", sourceID)
	})

	// Create a test assertion tagged with the project
	_, err = env.DB.Exec(ctx, `
		INSERT INTO assertions (
			tenant_id, source_id, assertion_type, description,
			confidence, severity, project_id, owner_person_id,
			is_current, created_at, updated_at
		) VALUES (
			$1, $2, 'action', 'Test assertion for briefing',
			0.9, 'medium', $3, $4,
			true, NOW(), NOW()
		)
	`, env.TenantID, sourceID, projectID, personID)
	require.NoError(t, err)

	t.Logf("Created project %q (ID %d) with 1 assertion", projectName, projectID)

	// Run penf briefing — this should NOT crash with SQL error
	t.Run("briefing_succeeds", func(t *testing.T) {
		result := env.CLI.Run(ctx, "briefing", projectName)
		assert.Equal(t, 0, result.ExitCode,
			"penf briefing should succeed, got error: %s", result.Stderr)
		assert.NotContains(t, result.Stderr, "a.type",
			"should not have SQL error about a.type column")
		assert.NotContains(t, result.Stderr, "does not exist",
			"should not have SQL column error")
	})

	t.Run("briefing_json_returns_assertions", func(t *testing.T) {
		result := env.CLI.Run(ctx, "briefing", projectName, "-o", "json")
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

	// Create a project with keywords
	var projectID int64
	err := env.DB.QueryRow(ctx, `
		INSERT INTO projects (tenant_id, name, description, keywords, created_at, updated_at)
		VALUES ($1, $2, 'Full briefing test', '{"brieftest"}', NOW(), NOW())
		RETURNING id
	`, testTenantID(env), fmt.Sprintf("BriefFull-%s", testID[:12])).Scan(&projectID)
	require.NoError(t, err)

	t.Cleanup(func() {
		env.DB.Exec(ctx, "DELETE FROM assertions WHERE project_id = $1", projectID)
		env.DB.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
	})

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
	var assertionCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM assertions
		WHERE source_id = $1 AND project_id = $2
	`, sourceID, projectID).Scan(&assertionCount)
	require.NoError(t, err)

	if assertionCount == 0 {
		t.Skip("No assertions created with project_id — keyword matching may not have resolved")
	}

	t.Logf("Pipeline created %d assertions tagged with project %d", assertionCount, projectID)

	// Briefing should return these assertions without SQL errors
	t.Run("briefing_returns_project_assertions", func(t *testing.T) {
		result := env.CLI.Run(ctx, "briefing", fmt.Sprintf("BriefFull-%s", testID[:12]))
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

	result := env.CLI.Run(ctx, "briefing", "MTC")
	assert.Equal(t, 0, result.ExitCode,
		"penf briefing MTC should not crash with SQL error: %s", result.Stderr)
	assert.NotContains(t, result.Stderr, "does not exist",
		"should not have column reference error")
}
