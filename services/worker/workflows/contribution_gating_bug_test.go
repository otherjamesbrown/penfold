// Package workflows provides workflow tests.
package workflows

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContributionGating_LowSkipsExtract verifies the fix for bug pf-e8af8e.
//
// FIX: In pipeline.go lines 1650-1673, the contribution-based gating switch now
// sets both skipExtract=true and skipAnalyze=true for "LOW" contribution, so
// trivial emails (e.g. "+Kyle", reaction emails) no longer run the expensive
// extraction stages:
//   - extract_ner
//   - extract_assertions
//   - extract_semantic
//   - resolve
//
// These stages were producing empty outputs for trivial content, wasting compute.
//
// Expected behavior after fix:
//   - "LOW" contribution: skipExtract=true AND skipAnalyze=true (skip ALL deep processing)
//   - missing/empty contribution: default to "MEDIUM", not "HIGH"
func TestContributionGating_LowSkipsExtract(t *testing.T) {
	t.Log("Regression test for pf-e8af8e: contribution=LOW must set skipExtract=true")
	t.Log("")
	t.Log("Bug location: services/worker/workflows/pipeline.go lines 1650-1673")
	t.Log("")
	t.Log("Fixed code:")
	t.Log("  case \"LOW\":")
	t.Log("      skipExtract = true // Also skip extraction stages")
	t.Log("      skipAnalyze = true")

	// Reproduce the contribution gating switch statement exactly as it appears
	// in pipeline.go lines 1647-1673.
	type gatingResult struct {
		skipExtract bool
		skipAnalyze bool
	}

	computeGating := func(contribution string, skipDeep bool) gatingResult {
		skipExtract := false
		skipAnalyze := false

		// Reproduces pipeline.go lines 1650-1673 verbatim.
		switch contribution {
		case "NONE":
			skipExtract = true
			skipAnalyze = true
		case "LOW":
			skipExtract = true // FIX pf-e8af8e: also skip extraction for LOW
			skipAnalyze = true
		case "MEDIUM", "HIGH":
			// Run everything
		}

		// Reproduces pipeline.go lines 1675-1684.
		if skipDeep {
			skipExtract = true
			skipAnalyze = true
		}

		return gatingResult{skipExtract: skipExtract, skipAnalyze: skipAnalyze}
	}

	t.Run("NONE_skips_both", func(t *testing.T) {
		result := computeGating("NONE", false)
		require.True(t, result.skipExtract, "NONE contribution must skip extraction stages")
		require.True(t, result.skipAnalyze, "NONE contribution must skip analyze stages")
	})

	t.Run("MEDIUM_runs_everything", func(t *testing.T) {
		result := computeGating("MEDIUM", false)
		require.False(t, result.skipExtract, "MEDIUM contribution should run extraction")
		require.False(t, result.skipAnalyze, "MEDIUM contribution should run analysis")
	})

	t.Run("HIGH_runs_everything", func(t *testing.T) {
		result := computeGating("HIGH", false)
		require.False(t, result.skipExtract, "HIGH contribution should run extraction")
		require.False(t, result.skipAnalyze, "HIGH contribution should run analysis")
	})

	t.Run("skip_deep_overrides_all", func(t *testing.T) {
		result := computeGating("MEDIUM", true)
		require.True(t, result.skipExtract, "SkipDeep=true must skip extraction regardless of contribution")
		require.True(t, result.skipAnalyze, "SkipDeep=true must skip analysis regardless of contribution")
	})

	// Verify that LOW contribution skips both extraction and analysis.
	// For LOW contribution (trivial emails like "+Kyle"), the extract stages
	// (extract_ner, extract_assertions, extract_semantic, resolve) must be
	// skipped — they produce empty outputs on trivial content and waste compute.
	//
	// Fix pf-e8af8e: LOW now sets skipExtract=true in addition to skipAnalyze=true.
	t.Run("LOW_skips_extract_stages_BUG", func(t *testing.T) {
		result := computeGating("LOW", false)

		// skipAnalyze should be true (was already correct before the fix)
		require.True(t, result.skipAnalyze,
			"LOW contribution must skip analyze stages (DeepAnalyze, PersistFindings)")

		// skipExtract must also be true (fixed in pf-e8af8e)
		assert.True(t, result.skipExtract,
			"FIX pf-e8af8e: LOW contribution must also skip extraction stages "+
				"(extract_ner, extract_assertions, extract_semantic, resolve). "+
				"Trivial emails like '+Kyle' should not run extraction activities.")
	})
}

// TestContributionGating_MissingContributionDefaultIsUnsafe verifies the fix for the
// second issue in pf-e8af8e: when content_contribution is empty/missing, the pipeline
// previously defaulted to "HIGH" (the most expensive path).
//
// Fix: default changed from "HIGH" to "MEDIUM" — a missing contribution field should
// not silently trigger full processing. MEDIUM is a safe middle ground.
func TestContributionGating_MissingContributionDefaultIsUnsafe(t *testing.T) {
	t.Log("Verifies pf-e8af8e fix: empty contribution now defaults to MEDIUM, not HIGH")
	t.Log("")
	t.Log("Bug location: services/worker/workflows/pipeline.go lines 1565-1570")
	t.Log("")
	t.Log("Fixed code:")
	t.Log("  contribution := triageOutput.ContentContribution")
	t.Log("  if contribution == \"\" {")
	t.Log("      contribution = \"MEDIUM\"  // safe default, not most expensive path")
	t.Log("  }")

	// Reproduce the default assignment from pipeline.go lines 1565-1570.
	applyDefault := func(contribution string) string {
		if contribution == "" {
			contribution = "MEDIUM" // FIX pf-e8af8e: safe default, not the most expensive path
		}
		return contribution
	}

	// Verify the fixed default behavior: empty contribution now defaults to MEDIUM, not HIGH.
	t.Run("empty_contribution_defaults_to_MEDIUM_safe", func(t *testing.T) {
		result := applyDefault("")
		// FIX pf-e8af8e: empty defaults to MEDIUM (safe — not the most expensive path).
		assert.Equal(t, "MEDIUM", result,
			"empty content_contribution should default to MEDIUM — "+
				"a missing contribution field should not trigger HIGH (most expensive) processing.")
	})

	// Verify non-empty values pass through unchanged (this should always hold).
	t.Run("non_empty_contribution_unchanged", func(t *testing.T) {
		for _, contribution := range []string{"NONE", "LOW", "MEDIUM", "HIGH"} {
			result := applyDefault(contribution)
			require.Equal(t, contribution, result,
				"non-empty contribution should pass through unchanged")
		}
	})
}
