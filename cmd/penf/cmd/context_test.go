// Package cmd provides CLI command implementations.
package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// TestContextMorningCommand tests the context morning subcommand.
func TestContextMorningCommand(t *testing.T) {
	deps := &ContextCommandDeps{
		Config: &config.CLIConfig{
			ContextPalace: &config.ContextPalaceConfig{
				Host:     "localhost",
				Database: "contextpalace",
				User:     "test",
				Project:  "test",
				Agent:    "agent-test",
			},
			Timeout: 30 * time.Second,
		},
		LoadConfig: func() (*config.CLIConfig, error) {
			return &config.CLIConfig{
				ContextPalace: &config.ContextPalaceConfig{
					Host:     "localhost",
					Database: "contextpalace",
					User:     "test",
					Project:  "test",
					Agent:    "agent-test",
				},
				Timeout: 30 * time.Second,
			}, nil
		},
	}

	cmd := newContextMorningCommand(deps)

	if cmd == nil {
		t.Fatal("newContextMorningCommand returned nil")
	}

	if cmd.Use != "morning" {
		t.Errorf("expected Use=morning, got %s", cmd.Use)
	}

	// Verify output flag exists
	outputFlag := cmd.Flags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("--output flag not found")
	}
}

// TestGetDefaultPlaybook tests the default playbook generation.
func TestGetDefaultPlaybook(t *testing.T) {
	playbook := getDefaultPlaybook()

	if playbook == "" {
		t.Fatal("getDefaultPlaybook returned empty string")
	}

	// Verify key sections are present
	expectedSections := []string{
		"# Morning Briefing",
		"## 1. Last session",
		"penf session resume --last-closed",
		"## 2. Overnight processing",
		"## 3. My meetings yesterday",
		"## 4. Meetings I wasn't in",
		"## 5. Reminders",
		"penf reminders list --due",
		"## 6. Projects",
		"## 7. Daily standup recap",
		"## 8. Upcoming meetings",
		"## 9. Ask me",
	}

	for _, section := range expectedSections {
		if !strings.Contains(playbook, section) {
			t.Errorf("default playbook missing expected section: %s", section)
		}
	}
}

// TestContextCommandStructure tests the overall context command structure.
func TestContextCommandStructure(t *testing.T) {
	deps := &ContextCommandDeps{
		Config: &config.CLIConfig{
			Timeout: 30 * time.Second,
		},
		LoadConfig: config.LoadConfig,
	}

	cmd := NewContextCommand(deps)

	if cmd == nil {
		t.Fatal("NewContextCommand returned nil")
	}

	if cmd.Use != "context" {
		t.Errorf("expected Use=context, got %s", cmd.Use)
	}

	// Verify subcommands exist
	subcommands := []string{"history", "status", "morning"}
	for _, subcmd := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == subcmd {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("subcommand %s not found", subcmd)
		}
	}
}

// TestOutputMorningJSON tests JSON output for morning playbook.
func TestOutputMorningJSON(t *testing.T) {
	id := "pf-test123"
	content := "# Test Playbook\n\nStep 1: Test"
	source := "stored"

	// We can't easily test the actual output without capturing stdout,
	// but we can verify the function doesn't panic
	err := outputMorningJSON(id, content, source)
	if err != nil {
		t.Errorf("outputMorningJSON failed: %v", err)
	}
}

// TestOutputMorningYAML tests YAML output for morning playbook.
func TestOutputMorningYAML(t *testing.T) {
	id := "pf-test123"
	content := "# Test Playbook\n\nStep 1: Test"
	source := "default"

	err := outputMorningYAML(id, content, source)
	if err != nil {
		t.Errorf("outputMorningYAML failed: %v", err)
	}
}

// TestOutputMorningText tests text output for morning playbook.
func TestOutputMorningText(t *testing.T) {
	content := "# Test Playbook\n\nStep 1: Test"
	source := "stored"

	err := outputMorningText(content, source)
	if err != nil {
		t.Errorf("outputMorningText failed: %v", err)
	}
}
