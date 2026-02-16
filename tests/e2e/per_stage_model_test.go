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

// TestPerStageModel_EmbeddingUsesEmbeddingModel verifies that the embedding stage
// uses AI_DEFAULT_EMBEDDING_MODEL (mxbai-embed-large), not AI_DEFAULT_LLM_MODEL
// (gemini-2.0-flash).
//
// Bug: ModelForStage("embedding") falls through to the LLM default instead of the
// embedding default. Ollama receives "gemini-2.0-flash" for an embedding request
// and returns 404.
//
// Root cause: per-stage model selection (af9d524) — selectBackend routes by model
// name but ModelForStage doesn't distinguish embedding vs LLM defaults.
func TestPerStageModel_EmbeddingUsesEmbeddingModel(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Find a content item to reprocess
	contentResult := env.SafeCLI.RunWithArgs(ctx, "content", "list", "-o", "json", "--limit", "1")
	require.True(t, contentResult.Success(), "content list should succeed: %s", contentResult.Stderr)

	var contentList struct {
		Items []struct {
			ID       string `json:"id"`
			SourceID int    `json:"source_id"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(contentResult.Stdout), &contentList))
	require.NotEmpty(t, contentList.Items)

	contentID := contentList.Items[0].ID
	sourceID := contentList.Items[0].SourceID

	// Reprocess the content
	reprocessResult := env.SafeCLI.RunWithArgs(ctx, "reprocess", contentID)
	require.True(t, reprocessResult.Success(), "reprocess should succeed: %s", reprocessResult.Stderr)

	// Wait for pipeline to complete
	time.Sleep(45 * time.Second)

	// Check content state — should NOT be FAILED
	showResult := env.SafeCLI.RunWithArgs(ctx, "content", "show", contentID, "-o", "json")
	require.True(t, showResult.Success(), "content show should succeed: %s", showResult.Stderr)

	var content struct {
		State     string `json:"state"`
		ErrorCode string `json:"error_code"`
		Message   string `json:"message"`
	}
	require.NoError(t, json.Unmarshal([]byte(showResult.Stdout), &content))

	assert.NotEqual(t, "FAILED", content.State,
		"content should not be FAILED — if embedding failed with 'model not found', "+
			"ModelForStage is using LLM default instead of embedding default. "+
			"source_id=%d, error: %s", sourceID, content.Message)

	// If the content completed, verify the embedding stage used the right model
	if content.State == "COMPLETED" || content.State == "completed" {
		inspectResult := env.SafeCLI.RunWithArgs(ctx, "pipeline", "inspect",
			string(rune(sourceID+'0')), "--limit", "1", "-o", "json")
		if inspectResult.Success() {
			// The embedding model should contain "mxbai" or "embed", not "gemini"
			assert.NotContains(t, inspectResult.Stdout, `"model":"gemini`,
				"embedding stage should not use gemini model")
		}
	}
}
