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

// TestQuality_ExtractionAccuracy runs golden file tests against the real pipeline.
// Each golden/*.yaml file defines expected extraction output for one test email.
// Emails are ingested sequentially (one at a time) to avoid overloading the gateway.
func TestQuality_ExtractionAccuracy(t *testing.T) {
	env := SetupQualityEnvironment(t)
	ctx := context.Background()

	// Step 1: Clean test tenant data
	t.Log("Cleaning quality test tenant data...")
	err := env.CleanupTestTenant()
	require.NoError(t, err, "cleanup test tenant")

	// Step 2: Load Acme Corp fixtures
	t.Log("Loading Acme Corp fixtures...")
	err = env.LoadFixtures()
	require.NoError(t, err, "load fixtures")

	// Step 3: Discover golden files
	goldenDir := filepath.Join(env.CLI.WorkDir, "tests", "quality", "golden")
	entries, err := os.ReadDir(goldenDir)
	require.NoError(t, err, "read golden directory")

	var goldenFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			goldenFiles = append(goldenFiles, filepath.Join(goldenDir, entry.Name()))
		}
	}

	require.NotEmpty(t, goldenFiles, "no golden files found in %s", goldenDir)
	t.Logf("Found %d golden files", len(goldenFiles))

	// Step 4: Run each golden file test sequentially
	for _, goldenPath := range goldenFiles {
		baseName := strings.TrimSuffix(filepath.Base(goldenPath), ".yaml")

		t.Run(baseName, func(t *testing.T) {
			// Parse golden YAML
			golden, err := LoadGoldenFile(goldenPath)
			require.NoError(t, err, "load golden file")

			t.Logf("Email: %s", golden.Email)
			t.Logf("Description: %s", golden.Description)

			// Resolve email path
			emailPath := env.FixturePath(filepath.Join("emails", golden.Email))
			if _, err := os.Stat(emailPath); os.IsNotExist(err) {
				t.Fatalf("email file not found: %s", emailPath)
			}

			// Unique source tag for this test run
			sourceTag := fmt.Sprintf("quality-%s-%d", baseName, time.Now().UnixNano())

			// Ingest
			t.Log("Ingesting email...")
			result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
			require.Equal(t, 0, result.ExitCode, "ingest failed: %s\n%s", result.Stderr, result.Stdout)

			// Kick pipeline
			t.Log("Kicking pipeline...")
			kickResult := env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)
			t.Logf("Pipeline kick: %s", strings.TrimSpace(kickResult.Stdout))

			// Get source ID
			sourceID, err := getLatestSourceByTag(env, sourceTag)
			require.NoError(t, err, "find source with tag %s", sourceTag)
			t.Logf("Source ID: %d", sourceID)

			// Wait for pipeline completion (120s timeout for LLM calls)
			t.Log("Waiting for pipeline completion...")
			err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
			require.NoError(t, err, "pipeline should complete")

			// Match: Pipeline stages
			t.Log("Checking pipeline stages...")
			MatchPipelineStages(t, env, sourceID, golden.Pipeline)

			// Match: Triage
			if golden.Triage != nil {
				t.Log("Checking triage...")
				triageResult, err := getTriageResult(env, sourceID)
				if err != nil {
					t.Logf("  warning: could not get triage result: %v", err)
				} else {
					MatchTriage(t, golden.Triage, triageResult)
				}
			}

			// Match: People
			if golden.People != nil {
				t.Log("Checking people...")
				people, err := getAutoCreatedPeopleForSource(env, sourceID)
				require.NoError(t, err, "query people")
				MatchPeople(t, golden.People, people)
			}

			// Match: Assertions
			if golden.Assertions != nil {
				t.Log("Checking assertions...")
				assertions, err := getAssertionsForSource(env, sourceID)
				require.NoError(t, err, "query assertions")
				MatchAssertions(t, golden.Assertions, assertions)
			}

			// Match: Projects
			if golden.Projects != nil {
				t.Log("Checking projects...")
				projects, err := getProjectsForSource(env, sourceID)
				require.NoError(t, err, "query projects")
				MatchProjects(t, golden.Projects, projects)
			}
		})
	}
}
