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

// TestEval_Standard runs standard email golden file tests against the real pipeline.
func TestEval_Standard(t *testing.T) {
	env := SetupQualityEnvironment(t)
	ctx := context.Background()

	// Ensure quality tenant exists, clean stale data, load fixtures, then seed classification rules.
	require.NoError(t, env.EnsureTenantExists(), "ensure quality tenant exists")
	require.NoError(t, env.CleanupTestTenant(), "clean up stale test data")
	require.NoError(t, env.LoadFixtures(), "load Acme Corp fixtures")
	require.NoError(t, env.SeedClassificationRules(ctx), "seed classification rules")
	require.NoError(t, env.SeedPersonalProjects(ctx), "seed personal projects")

	lfEval := NewLangfuseEval("standard")
	if err := lfEval.EnsureDataset(ctx); err != nil {
		t.Logf("warning: could not ensure Langfuse dataset: %v", err)
	}

	goldenDir := filepath.Join(env.CLI.WorkDir, "tests", "quality", "golden", "standard")
	entries, err := os.ReadDir(goldenDir)
	require.NoError(t, err, "reading golden/standard directory")

	var goldenFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			goldenFiles = append(goldenFiles, filepath.Join(goldenDir, entry.Name()))
		}
	}

	require.NotEmpty(t, goldenFiles, "no golden files found in %s", goldenDir)
	t.Logf("Found %d standard golden files", len(goldenFiles))

	for _, goldenPath := range goldenFiles {
		baseName := strings.TrimSuffix(filepath.Base(goldenPath), ".yaml")

		t.Run(baseName, func(t *testing.T) {
			// 1. Load golden expectation
			golden, err := LoadGoldenFile(goldenPath)
			require.NoError(t, err, "load golden file")

			t.Logf("Email: %s", golden.Email)
			t.Logf("Description: %s", golden.Description)

			// 2. Resolve email path
			emailPath := env.FixturePath(golden.Email)
			require.FileExists(t, emailPath, "fixture email must exist: %s", emailPath)

			// 3. Ingest email with unique source tag
			sourceTag := fmt.Sprintf("eval-standard-%s-%d", baseName, time.Now().UnixNano())
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

			// L2: Triage
			if golden.Triage != nil {
				t.Log("Checking triage...")
				triageResult, err := getTriageResult(env, sourceID)
				if err != nil {
					t.Errorf("triage: %v", err)
				} else {
					MatchTriage(t, golden.Triage, triageResult)
				}
			}

			// L2: People
			if golden.People != nil {
				t.Log("Checking people...")
				people, err := getAutoCreatedPeopleForSource(env, sourceID)
				if err != nil {
					t.Errorf("people: %v", err)
				} else {
					MatchPeople(t, golden.People, people)
				}
			}

			// L2: Assertions
			if golden.Assertions != nil {
				t.Log("Checking assertions...")
				assertions, err := getAssertionsForSource(env, sourceID)
				if err != nil {
					t.Errorf("assertions: %v", err)
				} else {
					MatchAssertions(t, golden.Assertions, assertions)
				}
			}

			// L2: Projects
			if golden.Projects != nil {
				t.Log("Checking projects...")
				projects, err := getProjectsForSource(env, sourceID)
				if err != nil {
					t.Errorf("projects: %v", err)
					results.L2Quality = append(results.L2Quality, MatchDetail{Check: "projects.exists", Pass: false, Message: err.Error()})
				} else {
					details := matchProjectsDetail(t, golden.Projects, projects)
					results.L2Quality = append(results.L2Quality, details...)
				}
			}

			// 8. Record to Langfuse
			traceID := getLangfuseTraceID(env, sourceID)
			if err := lfEval.RecordResult(ctx, traceID, results); err != nil {
				t.Logf("warning: Langfuse recording failed: %v", err)
			}

			// LLM-as-judge: read Langfuse evaluator scores.
			// Only check known LLM-as-judge score names (1-5 scale) — not the
			// deterministic binary scores (routing_correct, extraction_quality)
			// recorded by RecordResult, which are 0/1 and would always fail this check.
			if lfEval != nil && traceID != "" {
				time.Sleep(10 * time.Second)
				scores, err := lfEval.GetScores(ctx, traceID)
				if err == nil {
					llmJudgeNames := map[string]bool{
						"extraction_completeness": true,
						"triage_accuracy":         true,
						"summary_usefulness":      true,
					}
					llmScoresFound := 0
					for _, score := range scores {
						if !llmJudgeNames[score.Name] {
							continue
						}
						llmScoresFound++
						t.Logf("  langfuse.%s: %.1f", score.Name, score.Value)
						if score.Value < 3.0 {
							t.Errorf("langfuse.%s: score %.1f below threshold 3.0", score.Name, score.Value)
						}
					}
					if llmScoresFound == 0 {
						t.Logf("  langfuse: no LLM-as-judge scores found — Langfuse evaluators not yet configured (see task for setup steps)")
					}
				}
			}
		})
	}
}
