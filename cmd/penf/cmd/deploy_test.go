package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestDeployCommand(t *testing.T) {
	deployCmd := NewDeployCommand()

	if deployCmd == nil {
		t.Fatal("NewDeployCommand() returned nil")
	}

	if deployCmd.Use != "deploy [gateway|worker|ai|all]" {
		t.Errorf("Unexpected Use: %s", deployCmd.Use)
	}

	if deployCmd.Short != "Build, upload, and deploy services via Nomad" {
		t.Errorf("Unexpected Short: %s", deployCmd.Short)
	}
}

func TestDeployHistorySubcommand(t *testing.T) {
	deployCmd := NewDeployCommand()

	// Find the history subcommand.
	var historyCmd *cobra.Command
	for _, cmd := range deployCmd.Commands() {
		if cmd.Use == "history [service]" {
			historyCmd = cmd
			break
		}
	}

	if historyCmd == nil {
		t.Fatal("history subcommand not found")
	}

	if historyCmd.Short != "Show deployment history" {
		t.Errorf("Unexpected Short: %s", historyCmd.Short)
	}

	// Verify flags exist.
	lastFlag := historyCmd.Flags().Lookup("last")
	if lastFlag == nil {
		t.Error("--last flag not found")
	}
}

func TestDeployStatusFlag(t *testing.T) {
	deployCmd := NewDeployCommand()

	statusFlag := deployCmd.Flags().Lookup("status")
	if statusFlag == nil {
		t.Error("--status flag not found")
	}
}
