//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// searchResult mirrors CLISearchResult for JSON parsing.
type searchResult struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	ContentType string  `json:"content_type"`
	Score       float64 `json:"score"`
	Snippet     string  `json:"snippet"`
}

// setupRoleFilterData creates people records and ingests two test emails
// through the pipeline so that content_mentions have participation roles.
//
// Returns source IDs for email 010 and 011.
func setupRoleFilterData(t *testing.T, env *PipelineE2EEnv) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	tid := testTenantID(env)

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Create people records so header mentions activity can resolve emails to entities
	for _, p := range []struct{ name, email string }{
		{"John Smith", "john.smith@acme.com"},
		{"Sarah Chen", "sarah.chen@acme.com"},
		{"Marcus Rodriguez", "marcus.r@acme.com"},
		{"Emily Watson", "emily.watson@acme.com"},
	} {
		_, err := env.DB.Exec(ctx, `
			INSERT INTO people (tenant_id, canonical_name, email_addresses, account_type)
			VALUES ($1, $2, ARRAY[$3], 'person')
			ON CONFLICT DO NOTHING
		`, tid, p.name, p.email)
		require.NoError(t, err, "create person %s", p.name)
	}

	// Ingest email 010: From john.smith, To sarah.chen + marcus.r, Cc emily.watson
	// Subject: "SRv6 deployment readiness check"
	email010 := env.FixturePath("emails/010-role-test.eml")
	result := env.CLI.Run(ctx, "ingest", "email", email010, "--source", "search-filter-010")
	require.True(t, result.Success(), "ingest 010: %s", result.Stderr)

	var source010 int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM sources
		WHERE tenant_id = $1 AND ingestion_metadata->>'message_id' = '<role-test-010@acme.com>'
		ORDER BY created_at DESC LIMIT 1
	`, tid).Scan(&source010)
	require.NoError(t, err, "find source 010")

	// Ingest email 011: From sarah.chen, To john.smith
	// Subject: "Re: SRv6 deployment update"
	email011 := env.FixturePath("emails/011-role-test-reply.eml")
	result = env.CLI.Run(ctx, "ingest", "email", email011, "--source", "search-filter-011")
	require.True(t, result.Success(), "ingest 011: %s", result.Stderr)

	var source011 int64
	err = env.DB.QueryRow(ctx, `
		SELECT id FROM sources
		WHERE tenant_id = $1 AND ingestion_metadata->>'message_id' = '<role-test-011@acme.com>'
		ORDER BY created_at DESC LIMIT 1
	`, tid).Scan(&source011)
	require.NoError(t, err, "find source 011")

	// Process both through pipeline to create content_mentions with participation roles
	runPipelineAndWait(t, env, source010, 120*time.Second)
	runPipelineAndWait(t, env, source011, 120*time.Second)

	// Verify mentions were created with roles
	var mentionCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM content_mentions
		WHERE tenant_id = $1 AND content_id IN ($2, $3) AND participation_role > 0
	`, tid, source010, source011).Scan(&mentionCount)
	require.NoError(t, err)
	require.Greater(t, mentionCount, 0, "pipeline should create role-tagged mentions")
	t.Logf("Created %d role-tagged mentions across both emails", mentionCount)

	return source010, source011
}

// TestSearchFromFilter verifies that --filter "from:email" returns only content
// where the specified person has FROM role (participation_role=1).
//
// Email 010: From john.smith -> should match from:john.smith
// Email 011: From sarah.chen -> should NOT match from:john.smith
//
// Acceptance test for Phase 3 of pf-400b05 (Entity-Content Role Associations).
func TestSearchFromFilter(t *testing.T) {
	env := SetupPipelineE2E(t)
	setupRoleFilterData(t, env)

	ctx := context.Background()

	// Search "SRv6" filtered to from:john.smith@acme.com
	// Should return email 010 only (John is sender)
	result := env.CLI.Run(ctx, "search", "SRv6", "--filter", "from:john.smith@acme.com", "--output", "json")
	require.True(t, result.Success(), "search should succeed: %s", result.Stderr)
	t.Logf("Search output:\n%s", result.Stdout)

	var results []searchResult
	require.NoError(t, json.Unmarshal([]byte(result.Stdout), &results),
		"should parse JSON results")

	// Should have at least 1 result (email 010)
	require.Greater(t, len(results), 0,
		"from:john.smith should find email 010 (John is sender)")

	// All results should relate to email 010, not email 011
	for _, r := range results {
		assert.NotContains(t, strings.ToLower(r.Title), "deployment update",
			"from:john should NOT return email 011 (Sarah is the sender)")
	}
}

// TestSearchToStrictFilter verifies that --filter "to:email" matches only the
// To header, not CC. This follows Gmail semantics where "to:" is strict.
//
// Email 010: To sarah.chen + marcus.r, Cc emily.watson
// "to:emily.watson" should NOT match (Emily is CC, not To)
//
// Acceptance test for Phase 3 of pf-400b05.
func TestSearchToStrictFilter(t *testing.T) {
	env := SetupPipelineE2E(t)
	setupRoleFilterData(t, env)

	ctx := context.Background()

	// Search with to:emily.watson — Emily is CC on email 010, not To
	// Should return NO results from email 010
	result := env.CLI.Run(ctx, "search", "SRv6", "--filter", "to:emily.watson@acme.com", "--output", "json")
	require.True(t, result.Success(), "search should succeed: %s", result.Stderr)
	t.Logf("Search output:\n%s", result.Stdout)

	var results []searchResult
	if err := json.Unmarshal([]byte(result.Stdout), &results); err == nil {
		for _, r := range results {
			assert.NotContains(t, strings.ToLower(r.Title), "readiness",
				"to:emily should NOT return email 010 (Emily is CC, not To)")
		}
	}

	// Verify to:sarah DOES match email 010 (Sarah is on To)
	result2 := env.CLI.Run(ctx, "search", "SRv6", "--filter", "to:sarah.chen@acme.com", "--output", "json")
	require.True(t, result2.Success(), "search should succeed: %s", result2.Stderr)

	var results2 []searchResult
	require.NoError(t, json.Unmarshal([]byte(result2.Stdout), &results2),
		"should parse JSON results")
	assert.Greater(t, len(results2), 0,
		"to:sarah should find email 010 (Sarah is on the To header)")
}

// TestSearchRecipientBroadFilter verifies that --filter "recipient:email"
// matches To + CC + BCC + GROUP_MEMBER roles (any kind of recipient).
//
// Email 010: To sarah.chen, Cc emily.watson
// "recipient:emily.watson" should match (Emily is CC = a recipient)
//
// Acceptance test for Phase 3 of pf-400b05.
func TestSearchRecipientBroadFilter(t *testing.T) {
	env := SetupPipelineE2E(t)
	setupRoleFilterData(t, env)

	ctx := context.Background()

	// Search with recipient:emily.watson — Emily is CC on email 010
	// recipient: is broad — includes To, CC, BCC, GROUP_MEMBER
	result := env.CLI.Run(ctx, "search", "SRv6", "--filter", "recipient:emily.watson@acme.com", "--output", "json")
	require.True(t, result.Success(), "search should succeed: %s", result.Stderr)
	t.Logf("Search output:\n%s", result.Stdout)

	var results []searchResult
	require.NoError(t, json.Unmarshal([]byte(result.Stdout), &results),
		"should parse JSON results")
	assert.Greater(t, len(results), 0,
		"recipient:emily should find email 010 (Emily is CC = a recipient)")
}

// TestSearchNoFilterUnchanged verifies that search without role filters
// returns all matching content — default behavior is unchanged.
//
// Acceptance test for Phase 3 of pf-400b05.
func TestSearchNoFilterUnchanged(t *testing.T) {
	env := SetupPipelineE2E(t)
	setupRoleFilterData(t, env)

	ctx := context.Background()

	// Search "SRv6" with no filter — should return both emails
	result := env.CLI.Run(ctx, "search", "SRv6", "--output", "json")
	require.True(t, result.Success(), "search should succeed: %s", result.Stderr)
	t.Logf("Search output:\n%s", result.Stdout)

	var results []searchResult
	require.NoError(t, json.Unmarshal([]byte(result.Stdout), &results),
		"should parse JSON results")

	// Should return at least 2 results (both email 010 and 011 mention SRv6)
	assert.GreaterOrEqual(t, len(results), 2,
		"unfiltered search for 'SRv6' should return at least both test emails")
}
