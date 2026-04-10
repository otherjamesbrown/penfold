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
	"gopkg.in/yaml.v3"
)

// ProjectClassificationFixture describes one labelled eval case for project classification.
type ProjectClassificationFixture struct {
	Name             string   `yaml:"name"`
	Description      string   `yaml:"description"`
	Category         string   `yaml:"category"`
	Email            string   `yaml:"email"`
	ExpectedProjects []string `yaml:"expected_projects"`
	ExpectMethod     string   `yaml:"expect_method"`     // "channel_mapping" or "" for any
	SourceIdentifier string   `yaml:"source_identifier"` // non-empty → seed channel mapping, use as source tag
}

// TestProjectClassificationEval runs ~20 labelled emails through the real pipeline
// and asserts aggregate precision/recall thresholds for project attribution.
//
// Fixtures: tests/quality/fixtures/project_classification/*.yaml
// Run: go test ./tests/quality/ -tags=quality -run TestProjectClassificationEval -v
func TestProjectClassificationEval(t *testing.T) {
	env := SetupQualityEnvironment(t)
	ctx := context.Background()

	require.NoError(t, env.EnsureTenantExists(), "ensure quality tenant exists")
	require.NoError(t, env.CleanupTestTenant(), "clean up stale test data")
	require.NoError(t, env.LoadFixtures(), "load Acme Corp fixtures")
	require.NoError(t, seedProjectKeywordsForClassificationEval(ctx, env), "seed project keywords")
	require.NoError(t, env.SeedClassificationRules(ctx), "seed classification rules")

	lfEval := NewLangfuseEval("project_classification")
	if err := lfEval.EnsureDataset(ctx); err != nil {
		t.Logf("warning: could not ensure Langfuse dataset: %v", err)
	}

	// Load fixture definitions from tests/quality/fixtures/project_classification/
	fixtureDir := filepath.Join(env.CLI.WorkDir, "tests", "quality", "fixtures", "project_classification")
	entries, err := os.ReadDir(fixtureDir)
	require.NoError(t, err, "reading project_classification fixtures directory: %s", fixtureDir)

	var fixtures []ProjectClassificationFixture
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(fixtureDir, entry.Name()))
		require.NoError(t, err, "read fixture %s", entry.Name())
		var fx ProjectClassificationFixture
		require.NoError(t, yaml.Unmarshal(data, &fx), "parse fixture %s", entry.Name())
		fixtures = append(fixtures, fx)
	}

	require.NotEmpty(t, fixtures, "no project_classification fixtures found in %s", fixtureDir)
	t.Logf("Found %d project classification fixtures", len(fixtures))

	// Seed project_source_mappings for channel_mapping fixtures.
	require.NoError(t, seedChannelMappingsForFixtures(ctx, env, fixtures), "seed channel mappings")

	var (
		truePositives  int
		falsePositives int
		falseNegatives int
		overCounts     int
	)

	for _, fx := range fixtures {
		fx := fx
		t.Run(fx.Name, func(t *testing.T) {
			emailPath := env.FixturePath(fx.Email)
			require.FileExists(t, emailPath, "fixture email must exist: %s", emailPath)

			// Channel mapping fixtures use a fixed source tag that matches the seeded mapping.
			// All other fixtures get a unique tag to avoid cross-contamination.
			sourceTag := fmt.Sprintf("eval-pc-%s-%d", fx.Name, time.Now().UnixNano())
			if fx.SourceIdentifier != "" {
				sourceTag = fx.SourceIdentifier
			}

			// Ingest
			t.Logf("[%s] %s", fx.Category, fx.Description)
			result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", sourceTag)
			require.Equal(t, 0, result.ExitCode, "ingest failed: %s\n%s", result.Stderr, result.Stdout)

			// Kick pipeline
			kickResult := env.CLI.Run(ctx, "pipeline", "kick", "--source", sourceTag)
			t.Logf("Pipeline kick: %s", strings.TrimSpace(kickResult.Stdout))

			// Get source ID
			sourceID, err := getLatestSourceByTag(env, sourceTag)
			require.NoError(t, err, "find source with tag %s", sourceTag)
			t.Logf("Source ID: %d", sourceID)

			// Wait for pipeline completion
			err = waitForProcessingComplete(t, env, sourceID, 120*time.Second)
			require.NoError(t, err, "pipeline should complete within timeout")

			// Query attributed projects
			actualProjects, err := getProjectsForSource(env, sourceID)
			if err != nil {
				t.Errorf("getProjectsForSource: %v", err)
				return
			}

			expectedSet := toProjectNameSet(fx.ExpectedProjects)
			actualSet := toProjectNameSet(projectNames(actualProjects))

			t.Logf("Expected projects: %v", fx.ExpectedProjects)
			t.Logf("Actual projects:   %v", projectNames(actualProjects))

			// Over-attribution check: no email should be attributed to > 2 projects.
			if len(actualSet) > 2 {
				overCounts++
				t.Errorf("OVER-ATTRIBUTION: %s attributed to %d projects (max 2): %v",
					fx.Name, len(actualSet), projectNames(actualProjects))
			}

			// Precision/recall tallying (parent vars captured by reference, subtests run sequentially).
			var localFP, localFN int
			for name := range actualSet {
				if expectedSet[name] {
					truePositives++
				} else {
					falsePositives++
					localFP++
					t.Logf("FALSE POSITIVE: %q attributed to project %q (not in expected set)", fx.Name, name)
				}
			}
			for name := range expectedSet {
				if !actualSet[name] {
					falseNegatives++
					localFN++
					t.Logf("FALSE NEGATIVE: %q missing expected project %q", fx.Name, name)
				}
			}

			// Channel mapping method assertion.
			if fx.ExpectMethod == "channel_mapping" {
				method, err := getAttributionMethodForSource(env, sourceID)
				if err != nil {
					t.Errorf("getAttributionMethodForSource: %v", err)
				} else {
					require.Equal(t, "channel_mapping", method,
						"fixture %q should use channel_mapping fast path, got method=%q", fx.Name, method)
				}
			}

			// Record to Langfuse (best-effort).
			traceID := getLangfuseTraceID(env, sourceID)
			results := &EvalResults{}
			// Score as L2: 1.0 if this fixture matched exactly, 0.0 otherwise.
			localMatch := localFP == 0 && localFN == 0
			results.L2Quality = []MatchDetail{{Check: "project.attribution", Pass: localMatch}}
			if err := lfEval.RecordResult(ctx, traceID, results); err != nil {
				t.Logf("warning: Langfuse recording failed: %v", err)
			}
		})
	}

	// Aggregate precision/recall — computed after all subtests complete.
	precision := 1.0
	if truePositives+falsePositives > 0 {
		precision = float64(truePositives) / float64(truePositives+falsePositives)
	}

	recall := 1.0
	if truePositives+falseNegatives > 0 {
		recall = float64(truePositives) / float64(truePositives+falseNegatives)
	}

	t.Logf("=== Project Classification Eval Results ===")
	t.Logf("Fixtures:          %d", len(fixtures))
	t.Logf("Precision:         %.2f  (TP=%d, FP=%d)", precision, truePositives, falsePositives)
	t.Logf("Recall:            %.2f  (TP=%d, FN=%d)", recall, truePositives, falseNegatives)
	t.Logf("Over-attributions: %d", overCounts)

	require.GreaterOrEqual(t, precision, 0.90, "precision below threshold (TP=%d FP=%d)", truePositives, falsePositives)
	require.GreaterOrEqual(t, recall, 0.85, "recall below threshold (TP=%d FN=%d)", truePositives, falseNegatives)
	require.Equal(t, 0, overCounts, "no email should be attributed to > 2 projects")
}

// seedProjectKeywordsForClassificationEval updates the Acme Corp fixture projects with
// keywords so the keyword-based attribution stage can match them.
// Only updates projects that exist for the quality test tenant.
func seedProjectKeywordsForClassificationEval(ctx context.Context, env *QualityEnv) error {
	keywords := map[string][]string{
		"Project Alpha":              {"Project Alpha"},
		"Customer Portal Redesign":   {"Portal Redesign"},
		"Mobile App v2":              {"Mobile App v2"},
		"Data Pipeline Optimization": {"Data Pipeline Optimization"},
		"Security Audit 2026":        {"Security Audit"},
		"API v3":                     {"API v3"},
		"Enterprise Dashboard":       {"Enterprise Dashboard"},
		"ML Recommendations":         {"ML Recommendations"},
		"Documentation Refresh":      {"Documentation Refresh"},
		"Sales CRM Integration":      {"CRM Integration"},
	}

	for projectName, kws := range keywords {
		tag, err := env.DB.Exec(ctx, `
			UPDATE projects SET keywords = $1
			WHERE tenant_id = $2 AND name = $3
		`, kws, env.TenantID, projectName)
		if err != nil {
			return fmt.Errorf("seed keywords for project %q: %w", projectName, err)
		}
		if tag.RowsAffected() == 0 {
			// Project may not exist in this tenant — not fatal for eval.
			env.t.Logf("warning: project %q not found in quality tenant, skipping keyword seed", projectName)
		}
	}

	return nil
}

// seedChannelMappingsForFixtures inserts project_source_mappings rows for fixtures
// that declare a source_identifier (channel mapping fast-path verification).
func seedChannelMappingsForFixtures(ctx context.Context, env *QualityEnv, fixtures []ProjectClassificationFixture) error {
	for _, fx := range fixtures {
		if fx.SourceIdentifier == "" || fx.ExpectMethod != "channel_mapping" {
			continue
		}
		if len(fx.ExpectedProjects) == 0 {
			continue
		}

		// Resolve the project ID by name.
		var projectID int64
		err := env.DB.QueryRow(ctx, `
			SELECT id FROM projects WHERE tenant_id = $1 AND name = $2
		`, env.TenantID, fx.ExpectedProjects[0]).Scan(&projectID)
		if err != nil {
			return fmt.Errorf("find project %q for fixture %q: %w", fx.ExpectedProjects[0], fx.Name, err)
		}

		_, err = env.DB.Exec(ctx, `
			INSERT INTO project_source_mappings
				(tenant_id, project_id, source_type, source_identifier, match_type, confidence, enabled)
			VALUES ($1, $2, 'channel', $3, 'exact', 0.95, true)
			ON CONFLICT (tenant_id, source_type, source_identifier) DO NOTHING
		`, env.TenantID, projectID, fx.SourceIdentifier)
		if err != nil {
			return fmt.Errorf("seed channel mapping for fixture %q (identifier=%q): %w",
				fx.Name, fx.SourceIdentifier, err)
		}
	}
	return nil
}

// getAttributionMethodForSource returns the attribution_source stored on the first
// attributed assertion for sourceID, or "" if no attributed assertions exist.
func getAttributionMethodForSource(env *QualityEnv, sourceID int64) (string, error) {
	ctx := context.Background()
	var method *string
	err := env.DB.QueryRow(ctx, `
		SELECT DISTINCT attribution_source
		FROM assertions
		WHERE source_id = $1 AND attribution_source IS NOT NULL
		LIMIT 1
	`, sourceID).Scan(&method)
	if err != nil {
		return "", err
	}
	if method == nil {
		return "", nil
	}
	return *method, nil
}

// toProjectNameSet converts a slice of project names to a normalised set (lowercase).
func toProjectNameSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[strings.ToLower(strings.TrimSpace(n))] = true
	}
	return set
}

// projectNames extracts Name from a slice of ActualProject.
func projectNames(projects []ActualProject) []string {
	names := make([]string, len(projects))
	for i, p := range projects {
		names[i] = p.Name
	}
	return names
}
