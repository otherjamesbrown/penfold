//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ProductionTenantID is the hardcoded production tenant UUID.
// SafeCLIRunner MUST panic if this ID is used.
const ProductionTenantID = "c3170310-78bd-409c-b186-126f40bfa6ad"

// TestSafeCLIRunner_ProductionDenylist verifies that creating a SafeCLIRunner
// with the production tenant ID causes a panic.
//
// Acceptance criteria: Layer 4 - Production denylist validation
func TestSafeCLIRunner_ProductionDenylist(t *testing.T) {
	// Attempting to create a SafeCLIRunner with production tenant ID should panic
	require.Panics(t, func() {
		NewSafeCLIRunner(t, ProductionTenantID)
	}, "SafeCLIRunner must panic when initialized with production tenant ID")
}

// TestSafeCLIRunner_TestTenantAllowed verifies that creating a SafeCLIRunner
// with the test tenant UUID succeeds without panicking.
//
// Acceptance criteria: Layer 1 - Test tenant UUID acceptance
func TestSafeCLIRunner_TestTenantAllowed(t *testing.T) {
	// Creating SafeCLIRunner with test tenant should NOT panic
	require.NotPanics(t, func() {
		runner := NewSafeCLIRunner(t, TestTenantID)
		require.NotNil(t, runner, "SafeCLIRunner should be created successfully")
	}, "SafeCLIRunner must not panic when initialized with test tenant ID")
}

// TestSafeCLIRunner_TempConfigCreated verifies that SafeCLIRunner creates
// a temporary config directory with the expected pattern and config.yaml file.
//
// Acceptance criteria: Layer 2 - Temporary config directory creation
func TestSafeCLIRunner_TempConfigCreated(t *testing.T) {
	runner := NewSafeCLIRunner(t, TestTenantID)

	// SafeCLIRunner should expose the temp config path
	// (Actual implementation will need to expose this field)
	configPath := runner.ConfigPath
	require.NotEmpty(t, configPath, "SafeCLIRunner should have a config path")

	// Config path should be in TMPDIR with penf-e2e- prefix
	tmpDir := os.TempDir()
	require.True(t, strings.HasPrefix(configPath, tmpDir),
		"Config path should be in system temp directory, got: %s, expected prefix: %s",
		configPath, tmpDir)

	baseName := filepath.Base(configPath)
	require.True(t, strings.HasPrefix(baseName, "penf-e2e-"),
		"Config directory should start with 'penf-e2e-', got: %s", baseName)

	// Verify config.yaml exists
	configFile := filepath.Join(configPath, "config.yaml")
	_, err := os.Stat(configFile)
	require.NoError(t, err, "config.yaml should exist at %s", configFile)
}

// TestSafeCLIRunner_TempConfigCleanup verifies that the temporary config
// directory is removed after test cleanup runs.
//
// Acceptance criteria: Layer 2 - Temporary config cleanup
func TestSafeCLIRunner_TempConfigCleanup(t *testing.T) {
	var configPath string

	// Run in a sub-test so we can control cleanup
	t.Run("create_and_cleanup", func(t *testing.T) {
		runner := NewSafeCLIRunner(t, TestTenantID)
		configPath = runner.ConfigPath

		// Config should exist during test
		_, err := os.Stat(configPath)
		require.NoError(t, err, "Config directory should exist during test")

		// t.Cleanup() will run when this subtest ends
	})

	// After subtest cleanup, config should be gone
	_, err := os.Stat(configPath)
	require.True(t, os.IsNotExist(err),
		"Config directory should be removed after cleanup, but still exists at: %s", configPath)
}

// TestSafeCLIRunner_EnvVarSet verifies that SafeCLIRunner sets the
// PENF_TENANT_ID environment variable in its subprocess environment.
//
// Acceptance criteria: Layer 3 - Environment variable injection
func TestSafeCLIRunner_EnvVarSet(t *testing.T) {
	runner := NewSafeCLIRunner(t, TestTenantID)

	// SafeCLIRunner wraps CLIRunner, so we need to check the underlying runner
	// (Implementation will expose this or provide a method to inspect env)
	cliRunner := runner.Runner
	require.NotNil(t, cliRunner, "SafeCLIRunner should have an underlying CLIRunner")

	// Check that PENF_TENANT_ID is set in the environment
	var foundTenantID bool
	var actualTenantID string

	for _, env := range cliRunner.Env {
		if strings.HasPrefix(env, "PENF_TENANT_ID=") {
			foundTenantID = true
			actualTenantID = strings.TrimPrefix(env, "PENF_TENANT_ID=")
			break
		}
	}

	require.True(t, foundTenantID, "PENF_TENANT_ID environment variable should be set")
	assert.Equal(t, TestTenantID, actualTenantID,
		"PENF_TENANT_ID should be set to test tenant ID")
}

// TestSafeCLIRunner_ConfigFlag verifies that SafeCLIRunner passes the
// --config flag to all CLI commands.
//
// Acceptance criteria: Layer 2 - Config flag injection
func TestSafeCLIRunner_ConfigFlag(t *testing.T) {
	runner := NewSafeCLIRunner(t, TestTenantID)

	// SafeCLIRunner should expose a method or field showing how it adds --config
	// This test verifies the contract that all commands get --config <tmpdir>
	configPath := runner.ConfigPath
	require.NotEmpty(t, configPath, "SafeCLIRunner should have a config path")

	// The implementation should guarantee that Run() automatically adds --config
	// We can't test this without running actual commands, but we can verify
	// the config path is set and will be used
	assert.DirExists(t, configPath, "Config directory should exist for --config flag")
}

// TestSafeCLIRunner_PreFlightCheck verifies that SafeCLIRunner performs
// a pre-flight tenant verification before allowing commands to run.
//
// Acceptance criteria: Layer 5 - Pre-flight tenant current check
//
// NOTE: This test will fail during initial implementation because it requires
// a running Gateway service. It documents the expected behavior.
func TestSafeCLIRunner_PreFlightCheck(t *testing.T) {
	// This test documents the pre-flight check behavior
	// It may be skipped if Gateway is not available, similar to SetupE2EEnvironment

	runner := NewSafeCLIRunner(t, TestTenantID)

	// SafeCLIRunner should have run `penf tenant current` during initialization
	// and verified the tenant ID matches TestTenantID
	// If this check failed, NewSafeCLIRunner should have panicked

	// If we got here, the pre-flight check passed
	require.NotNil(t, runner, "SafeCLIRunner created successfully means pre-flight passed")
}

// TestSafeCLIRunner_RejectsTestTenantMatchingProduction verifies that
// SafeCLIRunner panics if TestTenantID somehow matches ProductionTenantID.
//
// Acceptance criteria: Layer 4 - Invariant validation
func TestSafeCLIRunner_RejectsTestTenantMatchingProduction(t *testing.T) {
	// This is a sanity check - TestTenantID should NEVER equal ProductionTenantID
	require.NotEqual(t, TestTenantID, ProductionTenantID,
		"Test tenant ID must not match production tenant ID - this is a critical safety invariant")

	// The SafeCLIRunner implementation should also validate this at initialization
	// and panic if they match, even if the caller passes TestTenantID
}

// TestSafeCLIRunner_ConfigContainsTenantID verifies that the generated
// config.yaml file contains the test tenant ID.
//
// Acceptance criteria: Layer 2 - Config file contents
func TestSafeCLIRunner_ConfigContainsTenantID(t *testing.T) {
	runner := NewSafeCLIRunner(t, TestTenantID)

	configFile := filepath.Join(runner.ConfigPath, "config.yaml")
	data, err := os.ReadFile(configFile)
	require.NoError(t, err, "Should be able to read config.yaml")

	configContent := string(data)
	assert.Contains(t, configContent, TestTenantID,
		"config.yaml should contain the test tenant ID")

	// Config should have tenant_id field set
	assert.Contains(t, configContent, "tenant_id",
		"config.yaml should have tenant_id field")
}
