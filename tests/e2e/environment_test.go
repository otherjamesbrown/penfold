//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2EEnvironmentSetup(t *testing.T) {
	env := SetupE2EEnvironment(t)

	// Verify database connection
	ctx := context.Background()
	var result int
	err := env.DB.QueryRow(ctx, "SELECT 1").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result)
}

func TestE2ELLMConnection(t *testing.T) {
	env := SetupE2EEnvironment(t)

	client := NewLLMClient(env.LLMURL)

	ctx := context.Background()

	// Check available models
	models, err := client.Models(ctx)
	require.NoError(t, err)
	t.Logf("Available models: %v", models)
	assert.NotEmpty(t, models)
}

func TestE2ELLMBasicCompletion(t *testing.T) {
	env := SetupE2EEnvironment(t)

	client := NewLLMClient(env.LLMURL)

	ctx := context.Background()

	// Test basic completion
	response, err := client.Complete(ctx, "What is 2 + 2? Answer with just the number.")
	require.NoError(t, err)
	t.Logf("LLM response: %s", response)
	assert.Contains(t, response, "4")
}

func TestE2EFixtureLoading(t *testing.T) {
	env := SetupE2EEnvironment(t)

	ctx := context.Background()

	// Truncate first
	err := env.TruncateAllTables()
	require.NoError(t, err)

	// Load fixtures
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Verify people were loaded
	var count int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM people").Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 20)

	// Verify glossary was loaded
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM glossary").Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 50)
}

func TestE2EUnknownFixture(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.LoadFixture("unknown-fixture")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown fixture")
}
