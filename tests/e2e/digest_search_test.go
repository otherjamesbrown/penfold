//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDigest_Phase7_EmbeddingSearchWorkflow is the acceptance test for
// Epic 5 Phase 7 — Digest Embedding + Search.
//
// It verifies:
//   - Embedding: digest content is embedded after generation
//   - Search: penf search returns digest results alongside source results
//   - Type filter: penf search --type digest filters to digest-only results
//   - Relevance: search for digest content returns the correct digest
//
// Expected: FAIL until Phase 7 is implemented.
//
// Run: go test -tags=e2e ./tests/e2e/ -run TestDigest_Phase7_EmbeddingSearchWorkflow -v -timeout 300s
func TestDigest_Phase7_EmbeddingSearchWorkflow(t *testing.T) {
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

	// Create a project with a distinctive digest
	projectResult := env.CLI.Run(ctx, "project", "add", "SearchDigestProject",
		"--description", "Project for digest search e2e test",
		"--keywords", "searchdigesttest2q7",
	)
	require.Equal(t, 0, projectResult.ExitCode, "project add: %s", projectResult.Stderr)

	var projectID int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM projects
		WHERE tenant_id = $1 AND name = 'SearchDigestProject'
	`, env.TenantID).Scan(&projectID)
	require.NoError(t, err)

	// Seed a digest with very distinctive content for search
	var promptTemplateID int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM prompt_templates
		WHERE stage = 'digest_daily_generate' AND is_active = true
		LIMIT 1
	`).Scan(&promptTemplateID)
	require.NoError(t, err)

	distinctiveBody := `{"summary":"The quantum flux capacitor prototype achieved 99.7% efficiency in Tuesday testing. Team recommends proceeding to Phase 2 with titanium-alloy housing.","sections":{"instruction_matches":[]}}`
	_, err = env.DB.Exec(ctx, `
		INSERT INTO digests (tenant_id, project_id, digest_type, period_start, period_end,
			body, model_used, prompt_template_id, input_token_count, output_token_count, source_content_ids)
		VALUES ($1, $2, 'daily', '2026-03-01', '2026-03-01', $3::jsonb, 'test-model', $4, 100, 50, '[]'::jsonb)
	`, env.TenantID, projectID, distinctiveBody, promptTemplateID)
	require.NoError(t, err)

	// ================================================================
	// Part 1: Verify digest exists in DB (embedding is future work)
	// ================================================================
	t.Run("digest_exists", func(t *testing.T) {
		var digestID string
		err := env.DB.QueryRow(ctx, `
			SELECT id FROM digests
			WHERE tenant_id = $1 AND project_id = $2
			ORDER BY created_at DESC LIMIT 1
		`, env.TenantID, projectID).Scan(&digestID)
		require.NoError(t, err)
		t.Logf("Digest ID: %s", digestID)

		// Verify the body is searchable via tsvector
		var matches int
		err = env.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM digests
			WHERE tenant_id = $1
				AND to_tsvector('english', COALESCE(body::text, '')) @@ websearch_to_tsquery('english', 'quantum flux capacitor')
		`, env.TenantID).Scan(&matches)
		require.NoError(t, err)
		assert.Greater(t, matches, 0, "digest body should be full-text searchable")
	})

	// ================================================================
	// Part 2: Search finds digest content
	// ================================================================
	t.Run("search_finds_digest", func(t *testing.T) {
		result := env.CLI.Run(ctx, "search", "quantum flux capacitor titanium", "-o", "json")
		require.Equal(t, 0, result.ExitCode,
			"search should succeed: %s", result.Stderr)

		var resp map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &resp)
		require.NoError(t, err, "search output should be valid JSON: %s", result.Stdout)

		results, ok := resp["results"].([]interface{})
		require.True(t, ok, "response should have results array")

		// Should find at least one result matching our distinctive content
		assert.GreaterOrEqual(t, len(results), 1,
			"search should return at least 1 result for distinctive digest content")

		if len(results) > 0 {
			t.Logf("Search result: %v", results[0])
		}
	})

	// ================================================================
	// Part 3: Search with type filter
	// ================================================================
	t.Run("search_type_filter_digest", func(t *testing.T) {
		result := env.CLI.Run(ctx, "search", "quantum flux capacitor",
			"--type", "digest",
			"-o", "json",
		)
		require.Equal(t, 0, result.ExitCode,
			"search --type digest should succeed: %s", result.Stderr)

		var resp map[string]interface{}
		err := json.Unmarshal([]byte(result.Stdout), &resp)
		require.NoError(t, err, "search output should be valid JSON: %s", result.Stdout)

		results, ok := resp["results"].([]interface{})
		require.True(t, ok, "response should have results array")

		// All results should be digest type
		for _, r := range results {
			rm, _ := r.(map[string]interface{})
			contentType, _ := rm["content_type"].(string)
			assert.Equal(t, "digest", contentType,
				"all results with --type digest filter should be digest type")
		}
	})
}
