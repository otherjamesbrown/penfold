package main

import (
	"testing"
)

func TestVersionCommand(t *testing.T) {
	if versionCmd == nil {
		t.Fatal("versionCmd is nil")
	}

	if versionCmd.Use != "version" {
		t.Errorf("Unexpected Use: %s", versionCmd.Use)
	}

	if versionCmd.Short != "Print version information" {
		t.Errorf("Unexpected Short: %s", versionCmd.Short)
	}
}

func TestVersionFlags(t *testing.T) {
	// Reset the command to ensure flags are registered.
	// (init() is called automatically, but we verify here)

	allFlag := versionCmd.Flags().Lookup("all")
	if allFlag == nil {
		t.Error("--all flag not found on version command")
	}

	outputJSONFlag := versionCmd.Flags().Lookup("output-json")
	if outputJSONFlag == nil {
		t.Error("--output-json flag not found on version command")
	}
}
