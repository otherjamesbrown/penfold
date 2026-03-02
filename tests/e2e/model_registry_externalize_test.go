//go:build e2e

// Acceptance test for SPEC items 1B (model context windows) + 1C (backend selection).
//
// 1B — Model Context Windows:
//   The hardcoded `modelContextWindows` map in extraction.go:334-340 maps model
//   names to context window sizes. The `ai_models` table already has a `context_window`
//   column that is not being read. After the refactor, extraction.go must resolve
//   context windows via `GetModelContextWindowByName(ctx, modelName)` on DBRegistry,
//   falling back to the hardcoded map only when the DB returns no result.
//
// 1C — Backend Selection:
//   `selectBackend()` in server.go:1057-1064 uses `strings.Contains(model, "gemini")`
//   to decide between "gemini" and "ollama" backends. The `ai_models` table already has
//   a `provider` column. After the refactor, selectBackend() must resolve the provider
//   via `GetModelProviderByName(ctx, modelName)` on DBRegistry, falling back to the
//   string heuristic only when the DB returns no result.
//
// Key detail: ai_models uses compound IDs like "gemini/gemini-2.0-flash" but callers
// pass short names like "gemini-2.0-flash". The new methods must query by `model_name`
// column, not by `id`.
//
// This test file defines "done" for 1B+1C. The source-grep subtests WILL FAIL until
// the refactor is complete. That is intentional.

package e2e

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Models that have hardcoded context windows in extraction.go.
// After the refactor, these must all have context_window populated in ai_models.
var knownExtractionModels = []string{
	"gemini-2.5-flash",
	"gemini-2.5-pro",
	"gemini-2.0-flash",
	"qwen2.5:7b",
	"qwen3:8b",
}

// Models that require correct provider routing in selectBackend().
// After the refactor, these must all have provider populated in ai_models.
var knownRoutedModels = []string{
	"gemini-2.5-flash",
	"gemini-2.5-pro",
	"gemini-2.0-flash",
	"qwen2.5:7b",
	"qwen3:8b",
	"mxbai-embed-large",
}

// expectedProviders maps model short names to their expected provider value
// in the ai_models table. Used to verify backend routing consistency.
var expectedProviders = map[string]string{
	"gemini-2.5-flash":  "gemini",
	"gemini-2.5-pro":    "gemini",
	"gemini-2.0-flash":  "gemini",
	"qwen2.5:7b":        "ollama",
	"qwen3:8b":          "ollama",
	"mxbai-embed-large": "ollama",
}

// TestModelRegistryExternalize_ContextWindowsPopulated verifies that every model
// with a hardcoded context window in extraction.go has a corresponding row in
// ai_models with context_window populated and non-default.
//
// This proves the DB has the data needed to replace the hardcoded map.
func TestModelRegistryExternalize_ContextWindowsPopulated(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, modelName := range knownExtractionModels {
		t.Run("context_window_"+modelName, func(t *testing.T) {
			var contextWindow int
			err := env.DB.QueryRow(ctx, `
				SELECT context_window
				FROM ai_models
				WHERE model_name = $1 AND is_enabled = true
			`, modelName).Scan(&contextWindow)

			require.NoError(t, err,
				"ai_models should have an enabled row with model_name=%q; "+
					"if missing, run model discovery or seed the table", modelName)

			assert.Greater(t, contextWindow, 4096,
				"context_window for %s should be greater than the default 4096; got %d",
				modelName, contextWindow)

			t.Logf("model_name=%s  context_window=%d", modelName, contextWindow)
		})
	}
}

// TestModelRegistryExternalize_ProvidersPopulated verifies that every model used
// in backend selection has a provider column populated in ai_models, and that
// the provider value is consistent with the expected backend.
//
// This proves the DB has the data needed to replace the string heuristic in
// selectBackend().
func TestModelRegistryExternalize_ProvidersPopulated(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, modelName := range knownRoutedModels {
		t.Run("provider_"+modelName, func(t *testing.T) {
			var provider string
			err := env.DB.QueryRow(ctx, `
				SELECT provider
				FROM ai_models
				WHERE model_name = $1 AND is_enabled = true
			`, modelName).Scan(&provider)

			require.NoError(t, err,
				"ai_models should have an enabled row with model_name=%q; "+
					"if missing, run model discovery or seed the table", modelName)

			assert.NotEmpty(t, provider,
				"provider for %s should not be empty", modelName)

			// Verify provider is consistent with what selectBackend() should return
			expected, ok := expectedProviders[modelName]
			if ok {
				assert.Equal(t, expected, provider,
					"provider for %s should be %q; got %q", modelName, expected, provider)
			}

			t.Logf("model_name=%s  provider=%s", modelName, provider)
		})
	}
}

// TestModelRegistryExternalize_ContextWindowQueryByModelName verifies that the
// new GetModelContextWindowByName pattern works: querying ai_models by model_name
// (not by compound id) returns the correct context_window value.
//
// This is the exact query pattern that extraction.go must use after the refactor.
func TestModelRegistryExternalize_ContextWindowQueryByModelName(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Query for a well-known model
	testModel := "gemini-2.0-flash"
	expectedMinWindow := 100000 // Gemini models have >= 1M token context

	var contextWindow int
	err := env.DB.QueryRow(ctx, `
		SELECT context_window
		FROM ai_models
		WHERE model_name = $1 AND is_enabled = true
		LIMIT 1
	`, testModel).Scan(&contextWindow)

	require.NoError(t, err, "should find %s by model_name (not compound id)", testModel)
	assert.Greater(t, contextWindow, expectedMinWindow,
		"context_window for %s should be large (Gemini has 1M+ tokens); got %d",
		testModel, contextWindow)
	t.Logf("Query by model_name=%q returned context_window=%d", testModel, contextWindow)

	// Also verify the compound ID exists but model_name is the correct lookup key
	var compoundID string
	err = env.DB.QueryRow(ctx, `
		SELECT id
		FROM ai_models
		WHERE model_name = $1
		LIMIT 1
	`, testModel).Scan(&compoundID)
	require.NoError(t, err)
	assert.Contains(t, compoundID, "/",
		"compound ID should contain '/' (e.g., 'gemini/gemini-2.0-flash'); got %q", compoundID)
	assert.NotEqual(t, testModel, compoundID,
		"model_name and id should be different (callers use short name, DB stores compound)")
	t.Logf("Compound id=%q for model_name=%q", compoundID, testModel)
}

// TestModelRegistryExternalize_ProviderQueryByModelName verifies that the new
// GetModelProviderByName pattern works: querying ai_models by model_name
// returns the correct provider value.
//
// This is the exact query pattern that selectBackend() must use after the refactor.
func TestModelRegistryExternalize_ProviderQueryByModelName(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testCases := []struct {
		modelName       string
		expectedBackend string
	}{
		{"gemini-2.0-flash", "gemini"},
		{"gemini-2.5-flash", "gemini"},
		{"qwen3:8b", "ollama"},
	}

	for _, tc := range testCases {
		t.Run(tc.modelName, func(t *testing.T) {
			var provider string
			err := env.DB.QueryRow(ctx, `
				SELECT provider
				FROM ai_models
				WHERE model_name = $1 AND is_enabled = true
				LIMIT 1
			`, tc.modelName).Scan(&provider)

			require.NoError(t, err, "should find %s by model_name", tc.modelName)
			assert.Equal(t, tc.expectedBackend, provider,
				"provider for %s should match expected backend", tc.modelName)
		})
	}
}

// TestModelRegistryExternalize_PipelineSucceedsWithDBModels is the integration
// smoke test: ingest an email, process it through the full pipeline, and verify
// it completes. After the refactor, the pipeline uses DB-backed context windows
// and provider selection. If either is broken, the pipeline will fail.
func TestModelRegistryExternalize_PipelineSucceedsWithDBModels(t *testing.T) {
	env := SetupPipelineE2E(t)
	ctx := context.Background()

	// Clean up and ensure test tenant exists
	err := env.CleanupTestTenant()
	require.NoError(t, err, "cleanup should succeed")

	err = env.EnsureTenantExists()
	require.NoError(t, err, "ensure tenant should succeed")

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err, "fixture loading should succeed")

	// Insert a high-importance email that will exercise extraction (uses context windows)
	// and backend routing (selectBackend) end-to-end.
	opts := emailSourceOpts{
		MessageID:   fmt.Sprintf("<model-registry-test-%d@e2e.penfold>", time.Now().UnixNano()),
		Subject:     "URGENT: Model Registry Externalize E2E Verification",
		FromAddress: "john.smith@acme.com",
		FromName:    "John Smith",
		To:          []map[string]string{{"email": "team@acme.com", "name": "Team"}},
		Body: `Team,

This is a high-priority security architecture discussion that requires full pipeline processing.

Key decisions:
- Sarah Chen will lead the API security review across all microservices
- Marcus Johnson is responsible for infrastructure hardening by end of Q1
- Risk assessment: HIGH — we identified 3 critical vulnerabilities in production

Decision: Mandatory mTLS for all service-to-service communication by March 15.
Owner: Sarah Chen
Impact: All engineering teams must update their deployment configurations.

Action items:
1. Rotate all API credentials within 48 hours
2. Enable audit logging on all production endpoints
3. Schedule security review sessions with each team lead

This is the highest priority item for the organisation.

-John Smith`,
		Date:       time.Now(),
		ExternalID: fmt.Sprintf("model-registry-e2e-%d", time.Now().UnixNano()),
	}

	sourceID := createEmailSourceCLI(t, env, opts)
	require.Greater(t, sourceID, int64(0), "source ID must be positive")
	t.Logf("Created source ID: %d", sourceID)

	// Process through the full pipeline
	runPipelineAndWait(t, env, sourceID, 120*time.Second)

	// Verify source completed
	source, err := env.getSourceByID(ctx, sourceID)
	require.NoError(t, err, "should fetch source")
	require.Equal(t, "completed", source.ProcessingStatus,
		"pipeline should complete; if failed, DB-backed context windows or provider routing may be broken")

	// Verify extraction stages completed (these use context windows for chunk sizing)
	env.assertStageCompleted(t, sourceID, "extract_ner")
	env.assertStageCompleted(t, sourceID, "extract_semantic")

	// Verify embedding completed (uses backend routing)
	env.assertStageCompleted(t, sourceID, "embed")

	t.Log("Pipeline completed successfully with DB-backed model registry")
}

// TestModelRegistryExternalize_NoHardcodedContextWindowMap greps the source code
// to verify that the hardcoded `modelContextWindows` map in extraction.go has been
// removed or demoted to a fallback-only constant.
//
// After the refactor:
//   - The map may still exist as a fallback, but the primary path must call
//     GetModelContextWindowByName (or similar DB lookup).
//   - maxInputChars() must try the DB first.
//
// This test WILL FAIL until the refactor is complete. That is intentional.
func TestModelRegistryExternalize_NoHardcodedContextWindowMap(t *testing.T) {
	projectRoot := findProjectRoot(t)

	t.Run("extraction_uses_db_lookup", func(t *testing.T) {
		extractionFile := filepath.Join(projectRoot, "services", "worker", "activities", "extraction.go")
		content, err := os.ReadFile(extractionFile)
		require.NoError(t, err, "should read extraction.go")
		src := string(content)

		// After refactor, extraction.go should reference the DB registry for context windows.
		// Look for evidence of DB-backed resolution.
		hasDBLookup := strings.Contains(src, "GetModelContextWindowByName") ||
			strings.Contains(src, "GetModelContextWindow") ||
			strings.Contains(src, "registry.") && strings.Contains(src, "ContextWindow") ||
			strings.Contains(src, "dbRegistry") && strings.Contains(src, "context")

		if !hasDBLookup {
			// Check if the hardcoded map is still the primary (non-fallback) path
			lines := strings.Split(src, "\n")
			for i, line := range lines {
				if strings.Contains(line, "modelContextWindows") && !strings.Contains(line, "fallback") && !strings.Contains(line, "Fallback") {
					// Found the hardcoded map used as primary path
					t.Errorf("extraction.go:%d still uses hardcoded modelContextWindows as primary path: %s",
						i+1, strings.TrimSpace(line))
					t.Errorf("After refactor, context window resolution must go through DBRegistry first")
				}
			}
		} else {
			t.Log("extraction.go references DB-backed context window lookup (refactor complete)")
		}
	})

	t.Run("hardcoded_map_is_fallback_only", func(t *testing.T) {
		extractionFile := filepath.Join(projectRoot, "services", "worker", "activities", "extraction.go")
		content, err := os.ReadFile(extractionFile)
		require.NoError(t, err)
		src := string(content)

		// The hardcoded map should either be removed entirely, or clearly marked
		// as a fallback with a comment.
		if strings.Contains(src, "var modelContextWindows") {
			// Map still exists — verify it's marked as fallback
			hasFallbackComment := strings.Contains(src, "fallback") || strings.Contains(src, "Fallback")
			if !hasFallbackComment {
				t.Error("modelContextWindows map still exists without fallback annotation; " +
					"it should either be removed or clearly marked as a fallback constant")
			} else {
				t.Log("modelContextWindows exists but is annotated as fallback (acceptable)")
			}
		} else {
			t.Log("modelContextWindows map has been removed (fully externalized)")
		}
	})
}

// TestModelRegistryExternalize_NoHardcodedBackendHeuristic greps the source code
// to verify that selectBackend() in server.go no longer uses strings.Contains as
// the primary backend selection path.
//
// After the refactor:
//   - selectBackend() must call GetModelProviderByName (or similar DB lookup) first.
//   - The strings.Contains heuristic may remain as a fallback for unknown models.
//
// This test WILL FAIL until the refactor is complete. That is intentional.
func TestModelRegistryExternalize_NoHardcodedBackendHeuristic(t *testing.T) {
	projectRoot := findProjectRoot(t)

	t.Run("selectBackend_uses_db_lookup", func(t *testing.T) {
		serverFile := filepath.Join(projectRoot, "services", "ai", "server", "server.go")
		content, err := os.ReadFile(serverFile)
		require.NoError(t, err, "should read server.go")
		src := string(content)

		// After refactor, selectBackend should reference the DB registry for provider info.
		hasDBLookup := strings.Contains(src, "GetModelProviderByName") ||
			strings.Contains(src, "GetModelProvider") ||
			strings.Contains(src, "registry.") && strings.Contains(src, "Provider") ||
			strings.Contains(src, "dbRegistry") && strings.Contains(src, "provider")

		if !hasDBLookup {
			t.Error("server.go selectBackend() does not reference DB-backed provider lookup; " +
				"after refactor, provider resolution must go through DBRegistry first")
		} else {
			t.Log("server.go references DB-backed provider lookup (refactor complete)")
		}
	})

	t.Run("string_heuristic_is_fallback_only", func(t *testing.T) {
		serverFile := filepath.Join(projectRoot, "services", "ai", "server", "server.go")

		violations := findSelectBackendHeuristicViolations(t, serverFile)

		if len(violations) > 0 {
			t.Errorf("selectBackend() still uses string heuristic as primary path:")
			for _, v := range violations {
				t.Errorf("  %s:%d: %s", v.file, v.line, strings.TrimSpace(v.text))
			}
			t.Error("After refactor, strings.Contains(model, \"gemini\") should only be a fallback")
		} else {
			t.Log("selectBackend() string heuristic is no longer the primary path")
		}
	})
}

// TestModelRegistryExternalize_DBRegistryHasNewMethods greps the DBRegistry
// source to verify the new lookup methods exist.
//
// After the refactor, db_registry.go should contain:
//   - GetModelContextWindowByName(ctx, modelName) (int, error)
//   - GetModelProviderByName(ctx, modelName) (string, error)
//
// This test WILL FAIL until the refactor is complete.
func TestModelRegistryExternalize_DBRegistryHasNewMethods(t *testing.T) {
	projectRoot := findProjectRoot(t)

	registryFile := filepath.Join(projectRoot, "services", "ai", "registry", "db_registry.go")
	content, err := os.ReadFile(registryFile)
	require.NoError(t, err, "should read db_registry.go")
	src := string(content)

	t.Run("has_GetModelContextWindowByName", func(t *testing.T) {
		hasMethod := strings.Contains(src, "GetModelContextWindowByName") ||
			strings.Contains(src, "GetContextWindowByName") ||
			strings.Contains(src, "GetModelContextWindow")

		if !hasMethod {
			t.Error("db_registry.go is missing GetModelContextWindowByName method; " +
				"this method must query ai_models by model_name and return context_window")
		} else {
			t.Log("db_registry.go has context window lookup method")
		}
	})

	t.Run("has_GetModelProviderByName", func(t *testing.T) {
		hasMethod := strings.Contains(src, "GetModelProviderByName") ||
			strings.Contains(src, "GetProviderByName") ||
			strings.Contains(src, "GetModelProvider")

		if !hasMethod {
			t.Error("db_registry.go is missing GetModelProviderByName method; " +
				"this method must query ai_models by model_name and return provider")
		} else {
			t.Log("db_registry.go has provider lookup method")
		}
	})

	// Verify the repository interface also has the new methods
	repoFile := filepath.Join(projectRoot, "services", "ai", "registry", "repository.go")
	repoContent, err := os.ReadFile(repoFile)
	require.NoError(t, err, "should read repository.go")
	repoSrc := string(repoContent)

	t.Run("repository_interface_has_methods", func(t *testing.T) {
		hasContextWindowMethod := strings.Contains(repoSrc, "GetModelContextWindowByName") ||
			strings.Contains(repoSrc, "GetContextWindowByName")
		hasProviderMethod := strings.Contains(repoSrc, "GetModelProviderByName") ||
			strings.Contains(repoSrc, "GetProviderByName")

		if !hasContextWindowMethod {
			t.Error("Repository interface missing context window lookup method")
		}
		if !hasProviderMethod {
			t.Error("Repository interface missing provider lookup method")
		}
		if hasContextWindowMethod && hasProviderMethod {
			t.Log("Repository interface has both new methods")
		}
	})
}

// ============================================================================
// Helpers
// ============================================================================

// findProjectRoot walks up from cwd looking for go.work to find the monorepo root.
func findProjectRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)

	for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
	}

	t.Fatal("could not find project root (go.work)")
	return ""
}

// selectBackendViolation represents a line where the string heuristic is used
// as the primary (non-fallback) path in selectBackend().
type selectBackendViolation struct {
	file string
	line int
	text string
}

// findSelectBackendHeuristicViolations scans server.go for uses of
// strings.Contains in selectBackend() that are NOT guarded by a fallback comment.
func findSelectBackendHeuristicViolations(t *testing.T, filePath string) []selectBackendViolation {
	t.Helper()

	f, err := os.Open(filePath)
	if err != nil {
		t.Logf("Warning: could not open %s: %v", filePath, err)
		return nil
	}
	defer f.Close()

	var violations []selectBackendViolation
	scanner := bufio.NewScanner(f)
	lineNum := 0
	inSelectBackend := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Track when we're inside the selectBackend function
		if strings.Contains(line, "func selectBackend(") {
			inSelectBackend = true
			continue
		}
		if inSelectBackend && trimmed == "}" && !strings.Contains(line, "\t\t") {
			// Top-level closing brace — we've exited the function
			inSelectBackend = false
			continue
		}

		if !inSelectBackend {
			continue
		}

		// Skip comment lines
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Look for the string heuristic pattern
		if strings.Contains(line, `strings.Contains`) && strings.Contains(line, `"gemini"`) {
			// Check if previous line(s) indicate this is a fallback
			// (We'd need to look at context, but for simplicity, if the function
			// ONLY contains the heuristic with no DB lookup, it's a violation)
			violations = append(violations, selectBackendViolation{
				file: filePath,
				line: lineNum,
				text: line,
			})
		}
	}

	return violations
}
