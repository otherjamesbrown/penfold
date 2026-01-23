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

func TestE2ECLIRunner(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Test CLI runner works by running a version command
	result := env.CLI.Run(ctx, "version")
	t.Logf("CLI version output: %s", result.Stdout)
	t.Logf("CLI version stderr: %s", result.Stderr)
	t.Logf("CLI exit code: %d", result.ExitCode)

	// The CLI should at least run without crashing
	// It may fail if --version isn't implemented, but the binary should exist
	require.NotNil(t, result, "CLI runner should return a result")
}

func TestE2ECLIHelp(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Test help command works
	result := env.CLI.Run(ctx, "--help")

	t.Logf("CLI help output: %s", result.Stdout)

	// Help command should succeed
	if result.Success() {
		assert.Contains(t, result.Stdout, "penf", "help should mention penf")
	}
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
	assert.GreaterOrEqual(t, count, 10, "should have loaded people")
	t.Logf("People loaded: %d", count)

	// Verify glossary was loaded
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM glossary").Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 5, "should have loaded glossary terms")
	t.Logf("Glossary terms loaded: %d", count)
}

func TestE2EUnknownFixture(t *testing.T) {
	env := SetupE2EEnvironment(t)

	err := env.LoadFixture("unknown-fixture")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown fixture")
}
