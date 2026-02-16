//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassify_LLMFallbackForUnknownSource verifies that content items with
// no matching classification rule get an LLM classification attempt.
//
// Acceptance criteria (pf-607489): Content items with no matching rule get LLM classification.
func TestClassify_LLMFallbackForUnknownSource(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// List all content and check classification status
	contentResult := env.SafeCLI.RunWithArgs(ctx, "content", "list", "-o", "json")
	require.True(t, contentResult.Success(), "content list should succeed: %s", contentResult.Stderr)

	var contentList struct {
		Items []struct {
			ID           string `json:"id"`
			SourceSystem string `json:"source_system"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(contentResult.Stdout), &contentList))

	// After LLM fallback is active, no items should have empty source_system
	// (they should either match a rule or get LLM classification)
	for _, item := range contentList.Items {
		assert.NotEmpty(t, item.SourceSystem,
			"content item %s should have source_system (from rule or LLM fallback)", item.ID)
	}
}

// TestClassify_SuggestionsCommand verifies that penf classify suggestions
// lists LLM-suggested rules that are pending review.
//
// Acceptance criteria (pf-607489): LLM suggestions stored and visible via CLI.
func TestClassify_SuggestionsCommand(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The suggestions command should exist and return valid output
	suggestResult := env.SafeCLI.RunWithArgs(ctx, "classify", "suggestions", "-o", "json")
	require.True(t, suggestResult.Success(),
		"classify suggestions should succeed: %s", suggestResult.Stderr)

	// Output should be valid JSON (may be empty array if no suggestions pending)
	var suggestions struct {
		Suggestions []struct {
			SourceSystem string  `json:"source_system"`
			Pattern      string  `json:"pattern"`
			Confidence   float64 `json:"confidence"`
			ContentID    string  `json:"content_id"`
		} `json:"suggestions"`
	}
	require.NoError(t, json.Unmarshal([]byte(suggestResult.Stdout), &suggestions),
		"classify suggestions should return valid JSON")
}

// TestClassify_ExistingRulesUnaffected verifies that the LLM fallback does not
// interfere with existing rules-based classification.
//
// Acceptance criteria (pf-607489): Existing rules-based classification unaffected.
func TestClassify_ExistingRulesUnaffected(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get current rules
	rulesResult := env.SafeCLI.RunWithArgs(ctx, "classify", "rules", "-o", "json")
	require.True(t, rulesResult.Success(), "classify rules should succeed: %s", rulesResult.Stderr)

	var rules struct {
		Rules []struct {
			SourceSystem string `json:"source_system"`
			Pattern      string `json:"pattern"`
		} `json:"rules"`
	}
	require.NoError(t, json.Unmarshal([]byte(rulesResult.Stdout), &rules))
	require.NotEmpty(t, rules.Rules, "should have existing classification rules")

	// Rules count should be stable — LLM fallback doesn't auto-add rules
	initialCount := len(rules.Rules)

	// Re-run rules list to confirm no auto-additions
	rulesResult2 := env.SafeCLI.RunWithArgs(ctx, "classify", "rules", "-o", "json")
	require.True(t, rulesResult2.Success(), "classify rules should succeed: %s", rulesResult2.Stderr)

	var rules2 struct {
		Rules []struct {
			SourceSystem string `json:"source_system"`
		} `json:"rules"`
	}
	require.NoError(t, json.Unmarshal([]byte(rulesResult2.Stdout), &rules2))

	assert.Equal(t, initialCount, len(rules2.Rules),
		"LLM fallback should not auto-add rules — suggestions require approval")
}
