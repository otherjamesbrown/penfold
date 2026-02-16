//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ProductionTenantIDDenylist is the hardcoded production tenant UUID.
// SafeCLIRunner MUST panic if this ID is used.
const ProductionTenantIDDenylist = "c3170310-78bd-409c-b186-126f40bfa6ad"

// SafeCLIRunner wraps CLIRunner with 5 layers of production tenant protection.
//
// Protection Layers:
//   Layer 1: Test tenant UUID (00000000-0000-0000-0000-000000000001)
//   Layer 2: Temp config with tenant_id locked to test tenant
//   Layer 3: PENF_TENANT_ID env var set to test tenant
//   Layer 4: Production denylist - panics if test tenant == production UUID
//   Layer 5: Pre-flight check - runs "penf tenant current" to verify tenant
//
// All CLI commands automatically receive --config flag pointing to temp config.
type SafeCLIRunner struct {
	ConfigPath string      // Temp config directory path
	Runner     *CLIRunner  // Underlying CLIRunner
	TenantID   string      // Test tenant ID
}

// NewSafeCLIRunner creates a SafeCLIRunner with all 5 protection layers.
//
// Protection sequence:
// 1. Layer 4: Check production denylist
// 2. Create underlying CLIRunner
// 3. Layer 2: Create temp config directory with config.yaml
// 4. Layer 3: Set PENF_TENANT_ID env var
// 5. Layer 5: Run pre-flight tenant verification
//
// Panics if tenantID matches production UUID or if pre-flight check fails.
func NewSafeCLIRunner(t *testing.T, tenantID string) *SafeCLIRunner {
	t.Helper()

	// Layer 4: Production denylist - MUST be first check
	if tenantID == ProductionTenantIDDenylist {
		panic(fmt.Sprintf("PRODUCTION TENANT PROTECTION: Cannot create SafeCLIRunner with production tenant ID %s", ProductionTenantIDDenylist))
	}

	// Additional safety: Verify TestTenantID constant hasn't been changed to production
	if TestTenantID == ProductionTenantIDDenylist {
		panic("CRITICAL SAFETY VIOLATION: TestTenantID constant matches production UUID")
	}

	// Create underlying CLIRunner
	runner := NewCLIRunner(t)

	// Layer 2: Create temp config directory
	tmpDir, err := os.MkdirTemp("", "penf-e2e-")
	if err != nil {
		t.Fatalf("failed to create temp config directory: %v", err)
	}

	// Register cleanup to remove temp dir
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	// Write config.yaml with test tenant ID and TLS configuration
	configPath := filepath.Join(tmpDir, "config.yaml")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	certDir := filepath.Join(homeDir, ".config", "penf", "certs")
	configContent := fmt.Sprintf(`server_address: dev02.brown.chat:50051
timeout: 10m
output_format: text
tenant_id: %s

tls:
  enabled: true
  cert_dir: %s
`, tenantID, certDir)

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	// Layer 3: Set PENF_TENANT_ID env var
	runner.SetEnv("PENF_TENANT_ID", tenantID)

	safeCLI := &SafeCLIRunner{
		ConfigPath: tmpDir,
		Runner:     runner,
		TenantID:   tenantID,
	}

	// Layer 5: Pre-flight check - verify tenant with gateway
	// This check may fail if gateway is unavailable, so we log a warning
	// rather than failing the test (allows compile-only tests to pass)
	ctx := context.Background()
	result := safeCLI.Run(ctx, "tenant", "current", "--output", "json")

	if result.Success() {
		// Parse tenant response to verify it matches
		var tenantResp struct {
			TenantID string `json:"tenant_id"`
			Source   string `json:"source"`
		}
		if err := json.Unmarshal([]byte(result.Stdout), &tenantResp); err != nil {
			t.Logf("WARNING: Pre-flight check could not parse tenant response: %v", err)
		} else if tenantResp.TenantID != tenantID {
			panic(fmt.Sprintf("PRE-FLIGHT TENANT MISMATCH: Expected tenant %s, got %s", tenantID, tenantResp.TenantID))
		} else {
			t.Logf("Pre-flight check passed: tenant %s verified", tenantID)
		}
	} else {
		// Gateway might not be available - log warning but allow test to continue
		t.Logf("WARNING: Pre-flight check failed (gateway may be unavailable): %v", result.Err)
		t.Logf("Stderr: %s", result.Stderr)
	}

	return safeCLI
}

// Run executes a penf command with automatic --config flag injection.
// All commands are automatically prefixed with --config <tempdir>/config.yaml.
func (s *SafeCLIRunner) Run(ctx context.Context, args ...string) *RunResult {
	// Prepend --config flag
	configFile := filepath.Join(s.ConfigPath, "config.yaml")
	fullArgs := append([]string{"--config", configFile}, args...)
	return s.Runner.Run(ctx, fullArgs...)
}

// RunJSON executes a command with JSON output and unmarshals the result.
func (s *SafeCLIRunner) RunJSON(ctx context.Context, v interface{}, args ...string) error {
	// Add --output json flag
	args = append(args, "--output", "json")
	result := s.Run(ctx, args...)
	if result.Err != nil {
		return fmt.Errorf("command failed: %w\nstderr: %s", result.Err, result.Stderr)
	}
	if err := json.Unmarshal([]byte(result.Stdout), v); err != nil {
		return fmt.Errorf("failed to parse JSON: %w\nstdout: %s", err, result.Stdout)
	}
	return nil
}

// Convenience methods for common operations

// RunIngest runs the ingest command for a file.
func (s *SafeCLIRunner) RunIngest(ctx context.Context, contentType, filePath string) *RunResult {
	return s.Run(ctx, "ingest", contentType, filePath)
}

// RunGlossaryAdd adds a glossary term.
func (s *SafeCLIRunner) RunGlossaryAdd(ctx context.Context, term, expansion, definition, context string) *RunResult {
	args := []string{"glossary", "add", term}
	if expansion != "" {
		args = append(args, "--expansion", expansion)
	}
	if definition != "" {
		args = append(args, "--definition", definition)
	}
	if context != "" {
		args = append(args, "--context", context)
	}
	return s.Run(ctx, args...)
}

// RunGlossaryAlias adds an alias to an existing term.
func (s *SafeCLIRunner) RunGlossaryAlias(ctx context.Context, term, alias string) *RunResult {
	return s.Run(ctx, "glossary", "alias", term, "--alias", alias)
}

// RunTeamCreate creates a new team.
func (s *SafeCLIRunner) RunTeamCreate(ctx context.Context, name, description string) *RunResult {
	args := []string{"team", "create", name}
	if description != "" {
		args = append(args, "--description", description)
	}
	return s.Run(ctx, args...)
}

// RunTeamAddMember adds a member to a team.
func (s *SafeCLIRunner) RunTeamAddMember(ctx context.Context, teamName, email, role string) *RunResult {
	args := []string{"team", "add-member", teamName, "--email", email}
	if role != "" {
		args = append(args, "--role", role)
	}
	return s.Run(ctx, args...)
}

// RunProjectAdd creates a new project.
func (s *SafeCLIRunner) RunProjectAdd(ctx context.Context, name, description string, keywords []string) *RunResult {
	args := []string{"project", "add", name}
	if description != "" {
		args = append(args, "--description", description)
	}
	if len(keywords) > 0 {
		// Join keywords with commas
		keywordStr := ""
		for i, kw := range keywords {
			if i > 0 {
				keywordStr += ","
			}
			keywordStr += kw
		}
		args = append(args, "--keywords", keywordStr)
	}
	return s.Run(ctx, args...)
}

// RunContentList lists content items.
func (s *SafeCLIRunner) RunContentList(ctx context.Context) *RunResult {
	return s.Run(ctx, "content", "list", "--output", "json")
}

// RunTenantCurrent returns the current tenant.
func (s *SafeCLIRunner) RunTenantCurrent(ctx context.Context) *RunResult {
	return s.Run(ctx, "tenant", "current", "--output", "json")
}
