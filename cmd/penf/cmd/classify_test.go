// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// TestClassifyCommand tests the classify command structure.
func TestClassifyCommand(t *testing.T) {
	cmd := NewClassifyCommand(nil)

	if cmd == nil {
		t.Fatal("NewClassifyCommand returned nil")
	}

	if cmd.Use != "classify" {
		t.Errorf("Use = %q, want %q", cmd.Use, "classify")
	}

	if cmd.Short == "" {
		t.Error("Short description is empty")
	}

	if cmd.Long == "" {
		t.Error("Long description is empty")
	}
}

// TestClassifyCommandSubcommands tests that classify has the expected subcommands.
func TestClassifyCommandSubcommands(t *testing.T) {
	cmd := NewClassifyCommand(nil)

	expectedSubcommands := []string{"run", "stats", "rules"}
	foundSubcommands := make(map[string]bool)

	for _, subCmd := range cmd.Commands() {
		foundSubcommands[subCmd.Name()] = true
	}

	for _, expected := range expectedSubcommands {
		if !foundSubcommands[expected] {
			t.Errorf("Missing subcommand %q", expected)
		}
	}

	if len(foundSubcommands) != len(expectedSubcommands) {
		t.Errorf("Expected %d subcommands, found %d", len(expectedSubcommands), len(foundSubcommands))
	}
}

// TestClassifyRunCommand tests the 'classify run' subcommand.
func TestClassifyRunCommand(t *testing.T) {
	cmd := NewClassifyCommand(nil)

	runCmd := findSubcommand(cmd, "run")
	if runCmd == nil {
		t.Fatal("run subcommand not found")
	}

	if runCmd.Use != "run [id]" {
		t.Errorf("Use = %q, want %q", runCmd.Use, "run [id]")
	}

	// Check that the command accepts 0 or 1 arguments (id is optional)
	if runCmd.Args != nil {
		// If Args is set, verify it allows 0 or 1 args
		// We'll test this by checking the error for different arg counts
		err0 := runCmd.Args(runCmd, []string{})
		err1 := runCmd.Args(runCmd, []string{"em-abc123"})
		err2 := runCmd.Args(runCmd, []string{"em-abc123", "extra"})

		if err0 != nil {
			t.Errorf("Args validation failed for 0 args: %v", err0)
		}
		if err1 != nil {
			t.Errorf("Args validation failed for 1 arg: %v", err1)
		}
		if err2 == nil {
			t.Error("Args validation should fail for 2+ args")
		}
	}
}

// TestClassifyRunFlags tests the flags on 'classify run' subcommand.
func TestClassifyRunFlags(t *testing.T) {
	cmd := NewClassifyCommand(nil)

	runCmd := findSubcommand(cmd, "run")
	if runCmd == nil {
		t.Fatal("run subcommand not found")
	}

	expectedFlags := []string{"all", "dry-run", "output"}

	for _, flagName := range expectedFlags {
		flag := runCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("Missing flag --%s", flagName)
		}
	}

	// Check that --all is a bool flag
	allFlag := runCmd.Flags().Lookup("all")
	if allFlag != nil && allFlag.Value.Type() != "bool" {
		t.Errorf("Flag --all should be bool, got %s", allFlag.Value.Type())
	}

	// Check that --dry-run is a bool flag
	dryRunFlag := runCmd.Flags().Lookup("dry-run")
	if dryRunFlag != nil && dryRunFlag.Value.Type() != "bool" {
		t.Errorf("Flag --dry-run should be bool, got %s", dryRunFlag.Value.Type())
	}

	// Check that --output is a string flag
	outputFlag := runCmd.Flags().Lookup("output")
	if outputFlag != nil && outputFlag.Value.Type() != "string" {
		t.Errorf("Flag --output should be string, got %s", outputFlag.Value.Type())
	}
}

// TestClassifyStatsCommand tests the 'classify stats' subcommand.
func TestClassifyStatsCommand(t *testing.T) {
	cmd := NewClassifyCommand(nil)

	statsCmd := findSubcommand(cmd, "stats")
	if statsCmd == nil {
		t.Fatal("stats subcommand not found")
	}

	if statsCmd.Use != "stats" {
		t.Errorf("Use = %q, want %q", statsCmd.Use, "stats")
	}

	if statsCmd.Short == "" {
		t.Error("Short description is empty")
	}

	if statsCmd.Long == "" {
		t.Error("Long description is empty")
	}

	// Verify it has no required args
	if statsCmd.Args != nil {
		err := statsCmd.Args(statsCmd, []string{})
		if err != nil {
			t.Errorf("stats should accept 0 args, got error: %v", err)
		}
	}
}

// TestClassifyStatsFlags tests the flags on 'classify stats' subcommand.
func TestClassifyStatsFlags(t *testing.T) {
	cmd := NewClassifyCommand(nil)

	statsCmd := findSubcommand(cmd, "stats")
	if statsCmd == nil {
		t.Fatal("stats subcommand not found")
	}

	// Check for output flag
	outputFlag := statsCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("Missing flag --output")
	}

	if outputFlag != nil && outputFlag.Value.Type() != "string" {
		t.Errorf("Flag --output should be string, got %s", outputFlag.Value.Type())
	}
}

// TestClassifyRulesCommand tests the 'classify rules' subcommand.
func TestClassifyRulesCommand(t *testing.T) {
	cmd := NewClassifyCommand(nil)

	rulesCmd := findSubcommand(cmd, "rules")
	if rulesCmd == nil {
		t.Fatal("rules subcommand not found")
	}

	if rulesCmd.Use != "rules" {
		t.Errorf("Use = %q, want %q", rulesCmd.Use, "rules")
	}

	if rulesCmd.Short == "" {
		t.Error("Short description is empty")
	}

	if rulesCmd.Long == "" {
		t.Error("Long description is empty")
	}

	// Verify it has no required args
	if rulesCmd.Args != nil {
		err := rulesCmd.Args(rulesCmd, []string{})
		if err != nil {
			t.Errorf("rules should accept 0 args, got error: %v", err)
		}
	}
}

// TestClassifyRulesFlags tests the flags on 'classify rules' subcommand.
func TestClassifyRulesFlags(t *testing.T) {
	cmd := NewClassifyCommand(nil)

	rulesCmd := findSubcommand(cmd, "rules")
	if rulesCmd == nil {
		t.Fatal("rules subcommand not found")
	}

	// Check for output flag
	outputFlag := rulesCmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Error("Missing flag --output")
	}

	if outputFlag != nil && outputFlag.Value.Type() != "string" {
		t.Errorf("Flag --output should be string, got %s", outputFlag.Value.Type())
	}
}

// TestClassifyCommandHelpText tests that help text follows AI-first guidelines.
func TestClassifyCommandHelpText(t *testing.T) {
	cmd := NewClassifyCommand(nil)

	// Root classify command should have examples
	if !containsExample(cmd.Example) {
		t.Error("classify command should have Example text for AI guidance")
	}

	// Check subcommands have proper help
	for _, subCmd := range cmd.Commands() {
		if subCmd.Short == "" {
			t.Errorf("Subcommand %q missing Short description", subCmd.Name())
		}

		if subCmd.Long == "" {
			t.Errorf("Subcommand %q missing Long description", subCmd.Name())
		}
	}
}

// TestClassifyOutputFormats tests that commands support multiple output formats.
func TestClassifyOutputFormats(t *testing.T) {
	cmd := NewClassifyCommand(nil)

	// Both stats and rules should support output formats
	testCases := []struct {
		subcommand string
	}{
		{"stats"},
		{"rules"},
		{"run"},
	}

	for _, tc := range testCases {
		subCmd := findSubcommand(cmd, tc.subcommand)
		if subCmd == nil {
			t.Fatalf("Subcommand %q not found", tc.subcommand)
		}

		outputFlag := subCmd.Flags().Lookup("output")
		if outputFlag == nil {
			t.Errorf("Subcommand %q missing --output flag", tc.subcommand)
		}
	}
}

// TestNewClassifyCommandWithDeps tests dependency injection pattern.
func TestNewClassifyCommandWithDeps(t *testing.T) {
	// Test with nil deps (should use defaults)
	cmd1 := NewClassifyCommand(nil)
	if cmd1 == nil {
		t.Error("NewClassifyCommand(nil) should use default deps")
	}

	// Test with custom deps
	customDeps := &ClassifyCommandDeps{
		LoadConfig: func() (*config.CLIConfig, error) {
			return &config.CLIConfig{
				ServerAddress: "test:50051",
			}, nil
		},
	}

	cmd2 := NewClassifyCommand(customDeps)
	if cmd2 == nil {
		t.Error("NewClassifyCommand with custom deps should work")
	}
}

// Helper functions

// findSubcommand finds a subcommand by name.
func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, subCmd := range cmd.Commands() {
		if subCmd.Name() == name {
			return subCmd
		}
	}
	return nil
}

// containsExample checks if a string looks like it contains example usage.
func containsExample(s string) bool {
	// Simple heuristic: should contain "penf classify" to be a valid example
	return len(s) > 0 && (contains(s, "penf classify") || contains(s, "#"))
}
