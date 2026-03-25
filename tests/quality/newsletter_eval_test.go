//go:build quality

package quality

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestEval_Newsletter runs newsletter golden file tests against the real pipeline.
func TestEval_Newsletter(t *testing.T) {
	env := SetupQualityEnvironment(t)
	ctx := context.Background()

	// Clean up stale sources so message_id dedup doesn't return old source IDs.
	// Static Message-ID headers in .eml fixtures cause CheckDuplicate() to match
	// on repeated runs — cleanup removes the old records before ingesting fresh.
	require.NoError(t, env.CleanupTestTenant(), "cleanup test tenant before newsletter eval")

	lfEval := NewLangfuseEval("newsletter")
	if err := lfEval.EnsureDataset(ctx); err != nil {
		t.Logf("warning: could not ensure Langfuse dataset: %v", err)
	}

	goldenDir := filepath.Join(env.CLI.WorkDir, "tests", "quality", "golden", "newsletter")
	entries, err := os.ReadDir(goldenDir)
	require.NoError(t, err, "reading golden/newsletter directory")

	var goldenFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			goldenFiles = append(goldenFiles, filepath.Join(goldenDir, entry.Name()))
		}
	}

	require.NotEmpty(t, goldenFiles, "no golden files found in %s", goldenDir)
	t.Logf("Found %d newsletter golden files", len(goldenFiles))

	for _, goldenPath := range goldenFiles {
		baseName := strings.TrimSuffix(filepath.Base(goldenPath), ".yaml")

		t.Run(baseName, func(t *testing.T) {
			// 1. Load golden expectation
			golden, err := LoadGoldenFile(goldenPath)
			require.NoError(t, err, "load golden file")

			t.Logf("Email: %s", golden.Email)
			t.Logf("Description: %s", golden.Description)

			// 2. Resolve email path — newsletter golden files include "emails/" prefix
			emailPath := env.FixturePath(golden.Email)
			require.FileExists(t, emailPath, "fixture email must exist: %s", emailPath)

			// 3. Ingest email with unique source tag
			sourceTag := fmt.Sprintf("eval-newsletter-%s-%d", baseName, time.Now().UnixNano())
			t.Log("Ingesting email...")
			result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
			require.Equal(t, 0, result.ExitCode, "ingest failed: %s\n%s", result.Stderr, result.Stdout)

			// 4. Kick pipeline
			t.Log("Kicking pipeline...")
			kickResult := env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)
			t.Logf("Pipeline kick: %s", strings.TrimSpace(kickResult.Stdout))

			// 5. Get source ID
			sourceID, err := getLatestSourceByTag(env, sourceTag)
			require.NoError(t, err, "find source with tag %s", sourceTag)
			t.Logf("Source ID: %d", sourceID)

			// 6. Wait for pipeline completion (120s timeout for LLM calls)
			t.Log("Waiting for pipeline completion...")
			err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
			require.NoError(t, err, "pipeline should complete")

			// 7. Run assertions and collect results
			results := &EvalResults{}

			// L1: Routing
			if golden.Routing != nil {
				t.Log("Checking routing...")
				results.L1Routing = MatchRouting(t, env, sourceID, golden.Routing)
			}

			// L2: Triage (existing matcher — doesn't return MatchDetail)
			if golden.Triage != nil {
				t.Log("Checking triage...")
				triageResult, err := getTriageResult(env, sourceID)
				if err != nil {
					t.Errorf("triage: %v", err)
				} else {
					MatchTriage(t, golden.Triage, triageResult)
				}
			}

			// L2: Newsletter extraction (only for non-discard)
			if golden.NewsletterExtract != nil {
				t.Log("Checking newsletter extraction...")
				results.L2Quality = MatchNewsletterExtract(t, env, sourceID, golden.NewsletterExtract)
			}

			// 8. Record to Langfuse
			traceID := getLangfuseTraceID(env, sourceID)
			if err := lfEval.RecordResult(ctx, traceID, results); err != nil {
				t.Logf("warning: Langfuse recording failed: %v", err)
			}
		})
	}
}
