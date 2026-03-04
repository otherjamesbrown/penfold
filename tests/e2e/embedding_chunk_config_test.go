//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_EmbeddingChunkConfig_RuntimeConfigurable verifies that embedding chunk
// parameters (max_tokens, overlap_tokens) are read from pipeline_operational_config
// at runtime, not hardcoded.
//
// Acceptance test for pf-a0e323 (Phase 1: make chunking configurable).
//
// Strategy:
//   1. Set embedding.chunk_max_tokens to a small value (200) via CLI
//   2. Ingest a long email and run pipeline
//   3. Count content-type embeddings — expect more chunks with smaller max_tokens
//   4. Reset to default (400) and reprocess
//   5. Count again — expect fewer chunks with larger max_tokens
//   6. Verify the config is visible via `penf pipeline config list`
func TestE2E_EmbeddingChunkConfig_RuntimeConfigurable(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// --- Setup: ensure default config exists and is readable via CLI ---

	// Verify we can list config and see embedding keys
	var configList []operationalConfigEntry
	err := env.SafeCLI.RunJSON(ctx, &configList, "pipeline", "config", "list", "--key", "embedding")
	require.NoError(t, err, "pipeline config list should support embedding keys")
	require.NotEmpty(t, configList, "should have embedding config entries after migration")

	// --- Step 1: Set small chunk size (200 tokens) ---

	result := env.SafeCLI.Run(ctx, "pipeline", "config", "set", "embedding.chunk_max_tokens", "200")
	require.True(t, result.Success(), "set chunk_max_tokens to 200: stdout=%s stderr=%s", result.Stdout, result.Stderr)

	t.Cleanup(func() {
		// Always restore default
		env.SafeCLI.Run(ctx, "pipeline", "config", "set", "embedding.chunk_max_tokens", "400")
		env.SafeCLI.Run(ctx, "pipeline", "config", "set", "embedding.chunk_overlap_tokens", "50")
	})

	// --- Step 2: Ingest a long email (~800 tokens) ---

	longBody := generateLongEmailBody(800)
	sourceID := createEmailSource(t, env, emailSourceOpts{
		MessageID:   fmt.Sprintf("<chunk-config-test-%d@e2e.test>", time.Now().UnixNano()),
		Subject:     "E2E Chunk Config Test",
		FromAddress: "chunker@e2e.test",
		FromName:    "Chunk Tester",
		Body:        longBody,
		Date:        time.Now(),
		ExternalID:  fmt.Sprintf("chunk-config-%d", time.Now().UnixNano()),
	})
	require.Greater(t, sourceID, int64(0), "source should be created")

	// --- Step 3: Run pipeline with small chunk size ---

	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	smallChunkCount := countContentEmbeddings(t, env, sourceID)
	t.Logf("With chunk_max_tokens=200: %d content embeddings", smallChunkCount)
	require.Greater(t, smallChunkCount, 1, "800-token content with 200-token chunks should produce multiple embeddings")

	// --- Step 4: Set default chunk size (400 tokens) and reprocess ---

	result = env.SafeCLI.Run(ctx, "pipeline", "config", "set", "embedding.chunk_max_tokens", "400")
	require.True(t, result.Success(), "set chunk_max_tokens to 400: stdout=%s stderr=%s", result.Stdout, result.Stderr)

	// Delete existing embeddings so reprocess generates fresh ones
	_, err = env.DB.Exec(ctx, `DELETE FROM embeddings WHERE source_id = $1`, sourceID)
	require.NoError(t, err, "should delete embeddings for reprocess")

	// Reset source processing status so pipeline can re-run
	_, err = env.DB.Exec(ctx, `UPDATE sources SET processing_status = 'pending' WHERE id = $1`, sourceID)
	require.NoError(t, err, "should reset source status")

	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	defaultChunkCount := countContentEmbeddings(t, env, sourceID)
	t.Logf("With chunk_max_tokens=400: %d content embeddings", defaultChunkCount)

	// --- Step 5: Smaller chunks should produce MORE embeddings ---

	assert.Greater(t, smallChunkCount, defaultChunkCount,
		"200-token chunks should produce more embeddings than 400-token chunks for the same content")

	// --- Step 6: Verify config is visible via CLI ---

	var verifyList []operationalConfigEntry
	err = env.SafeCLI.RunJSON(ctx, &verifyList, "pipeline", "config", "list", "--key", "embedding")
	require.NoError(t, err, "pipeline config list should work")

	found := false
	for _, entry := range verifyList {
		if entry.Key == "embedding.chunk_max_tokens" {
			assert.Equal(t, "400", entry.Value, "config should show restored default")
			found = true
		}
	}
	assert.True(t, found, "embedding.chunk_max_tokens should appear in config list")
}

// TestE2E_EmbeddingChunkConfig_PerTenantIsolation verifies that chunk config
// is per-tenant — changing one tenant's config doesn't affect another.
func TestE2E_EmbeddingChunkConfig_PerTenantIsolation(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Read config for e2e tenant
	var before []operationalConfigEntry
	err := env.SafeCLI.RunJSON(ctx, &before, "pipeline", "config", "list", "--key", "embedding.chunk_max_tokens")
	require.NoError(t, err)

	// Verify the config value belongs to our test tenant (not production)
	for _, entry := range before {
		if entry.Key == "embedding.chunk_max_tokens" {
			assert.Equal(t, "400", entry.Value, "default should be 400")
		}
	}

	// Set a different value
	result := env.SafeCLI.Run(ctx, "pipeline", "config", "set", "embedding.chunk_max_tokens", "256")
	require.True(t, result.Success())

	t.Cleanup(func() {
		env.SafeCLI.Run(ctx, "pipeline", "config", "set", "embedding.chunk_max_tokens", "400")
	})

	// Verify via direct DB that only our tenant was affected
	var count int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM pipeline_operational_config
		WHERE key = 'embedding.chunk_max_tokens' AND value = '256' AND tenant_id = $1
	`, TestTenantID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "only e2e tenant should have value 256")

	// Production tenant should still have 400 (or whatever its default is)
	var prodCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM pipeline_operational_config
		WHERE key = 'embedding.chunk_max_tokens' AND value = '256'
		AND tenant_id != $1
	`, TestTenantID).Scan(&prodCount)
	require.NoError(t, err)
	assert.Equal(t, 0, prodCount, "no other tenant should have value 256")
}

// --- Helper types and functions ---

type operationalConfigEntry struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// countContentEmbeddings returns the number of content-type embeddings for a source.
func countContentEmbeddings(t *testing.T, env *PipelineE2EEnv, sourceID int64) int {
	t.Helper()
	var count int
	err := env.DB.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM embeddings
		WHERE source_id = $1 AND representation_type = 'content'
	`, sourceID).Scan(&count)
	require.NoError(t, err, "should count content embeddings")
	return count
}

// generateLongEmailBody creates email body text of approximately targetTokens length.
// Uses realistic-looking sentences to avoid pathological chunking behavior.
func generateLongEmailBody(targetTokens int) string {
	sentences := []string{
		"The quarterly infrastructure review highlighted several areas requiring immediate attention across our cloud deployment.",
		"Team leads from engineering and operations met to discuss the migration timeline and resource allocation for Q2.",
		"Performance benchmarks from the latest load testing show a 15% improvement in API response times after the caching layer was deployed.",
		"The security audit identified three medium-severity findings related to certificate rotation and key management practices.",
		"Our monitoring dashboards now include real-time alerting for memory utilization exceeding 80% across all production nodes.",
		"The database optimization effort reduced average query latency from 45ms to 12ms for the most frequently accessed endpoints.",
		"Cross-team coordination for the platform consolidation project has been streamlined with weekly sync meetings.",
		"The CI/CD pipeline improvements cut average build times from 18 minutes to 7 minutes for the main service repository.",
		"Customer feedback from the beta program indicates strong demand for the bulk import feature scheduled for next release.",
		"The incident response playbook was updated to include escalation procedures for the new observability stack.",
		"Network topology changes planned for next month will require coordination with the data center operations team.",
		"The API versioning strategy document was finalized and distributed to all consuming teams for review.",
		"Load balancer configuration updates improved connection pooling efficiency by approximately 25% during peak hours.",
		"The disaster recovery drill conducted last week validated our RTO target of under 4 hours for critical services.",
		"New hire onboarding documentation was expanded to include architecture decision records from the past 18 months.",
		"The feature flag system migration from the legacy platform to the new provider is now 85% complete.",
		"Compliance requirements for the upcoming SOC2 audit necessitate additional logging in the authentication service.",
		"The machine learning pipeline team presented their findings on model inference optimization using quantization techniques.",
		"Sprint velocity has stabilized at around 45 story points per two-week cycle after the team restructuring.",
		"The deprecation notice for API v2 endpoints was published with a 90-day migration window for all consumers.",
	}

	var body string
	estimatedTokens := 0
	for estimatedTokens < targetTokens {
		for _, s := range sentences {
			body += s + "\n\n"
			estimatedTokens += len(s) / 4 // rough token estimate
			if estimatedTokens >= targetTokens {
				break
			}
		}
	}
	return body
}
