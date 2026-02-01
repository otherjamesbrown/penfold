// Package cmd provides CLI command implementations.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/cmd/penf/contextpalace"
)

// ContextCommandDeps holds dependencies for the context command.
type ContextCommandDeps struct {
	Config     *config.CLIConfig
	LoadConfig func() (*config.CLIConfig, error)
}

// DefaultContextDeps returns the default dependencies for the context command.
func DefaultContextDeps() *ContextCommandDeps {
	return &ContextCommandDeps{
		LoadConfig: config.LoadConfig,
	}
}

// NewContextCommand creates the context command group.
func NewContextCommand(deps *ContextCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultContextDeps()
	}

	cmd := &cobra.Command{
		Use:   "context",
		Short: "Context-Palace operations",
		Long: `Commands for interacting with Context-Palace, the shared memory system
for cross-session and cross-agent coordination.

Context-Palace stores command history, messages, and tasks that persist
across CLI sessions.`,
	}

	cmd.AddCommand(newContextHistoryCommand(deps))
	cmd.AddCommand(newContextStatusCommand(deps))

	return cmd
}

// History command flags.
var (
	contextHistoryLimit int
	contextHistoryAgent string
)

// newContextHistoryCommand creates the context history subcommand.
func newContextHistoryCommand(deps *ContextCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show recent CLI command history",
		Long: `Display recent CLI commands logged to Context-Palace.

This shows what commands have been run across sessions, useful for
resuming work or understanding what was done previously.

Examples:
  penf context history              # Show last 20 commands
  penf context history -n 50        # Show last 50 commands
  penf context history -a agent-x   # Show commands from specific agent
  penf context history -o json      # Output as JSON`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runContextHistory(cmd, deps)
		},
	}

	cmd.Flags().IntVarP(&contextHistoryLimit, "limit", "n", 20, "Number of commands to show")
	cmd.Flags().StringVarP(&contextHistoryAgent, "agent", "a", "", "Filter by agent (default: all agents)")

	return cmd
}

// runContextHistory executes the history command.
func runContextHistory(cmd *cobra.Command, deps *ContextCommandDeps) error {
	// Load config if not already loaded.
	cfg := deps.Config
	if cfg == nil {
		var err error
		cfg, err = deps.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
	}

	// Check Context-Palace configuration.
	if cfg.ContextPalace == nil || !cfg.ContextPalace.IsConfigured() {
		return fmt.Errorf("Context-Palace not configured. Add context_palace section to ~/.penf/config.yaml")
	}

	// Connect to Context-Palace.
	cpClient, err := contextpalace.NewClient(cfg.ContextPalace)
	if err != nil {
		return fmt.Errorf("connecting to Context-Palace: %w", err)
	}
	defer cpClient.Close()

	// Query history.
	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()

	entries, err := cpClient.History(ctx, contextHistoryAgent, contextHistoryLimit)
	if err != nil {
		return fmt.Errorf("querying history: %w", err)
	}

	// Output in configured format.
	return outputHistory(cmd, cfg, entries)
}

// outputHistory outputs the command history in the configured format.
func outputHistory(cmd *cobra.Command, cfg *config.CLIConfig, entries []contextpalace.CommandEntry) error {
	// Check for output format flag override.
	format := cfg.OutputFormat
	if f := cmd.Flag("output"); f != nil && f.Changed {
		format = config.OutputFormat(f.Value.String())
	}

	switch format {
	case config.OutputFormatJSON:
		return outputHistoryJSON(entries)
	case config.OutputFormatYAML:
		return outputHistoryYAML(entries)
	default:
		return outputHistoryText(entries)
	}
}

// outputHistoryJSON outputs history as JSON.
func outputHistoryJSON(entries []contextpalace.CommandEntry) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// outputHistoryYAML outputs history as YAML.
func outputHistoryYAML(entries []contextpalace.CommandEntry) error {
	enc := yaml.NewEncoder(os.Stdout)
	return enc.Encode(entries)
}

// outputHistoryText outputs history in human-readable text format.
func outputHistoryText(entries []contextpalace.CommandEntry) error {
	if len(entries) == 0 {
		fmt.Println("No command history found.")
		return nil
	}

	fmt.Println("Recent CLI commands:")
	fmt.Println()

	for _, e := range entries {
		// Format timestamp.
		ts := e.CreatedAt.Local().Format("2006-01-02 15:04")

		// Format duration.
		duration := formatDurationMs(e.DurationMs)

		// Format status.
		status := ""
		if !e.Success {
			status = " \033[31mERROR\033[0m"
		}

		// Format command (truncate if too long).
		cmdStr := e.FullCommand
		if len(cmdStr) > 60 {
			cmdStr = cmdStr[:57] + "..."
		}

		// Print entry.
		fmt.Printf("  %s  %-60s (%s)%s\n", ts, cmdStr, duration, status)

		// Show truncated response if present.
		if e.Response != "" {
			response := strings.TrimSpace(e.Response)
			if len(response) > 100 {
				response = response[:97] + "..."
			}
			// Indent and show first line only.
			lines := strings.Split(response, "\n")
			if len(lines) > 0 && lines[0] != "" {
				fmt.Printf("             \033[90m%s\033[0m\n", lines[0])
			}
		}

		// Show error if present.
		if e.ErrorMessage != "" {
			errMsg := e.ErrorMessage
			if len(errMsg) > 80 {
				errMsg = errMsg[:77] + "..."
			}
			fmt.Printf("             \033[31m%s\033[0m\n", errMsg)
		}
	}

	return nil
}

// formatDurationMs formats milliseconds as a human-readable duration.
func formatDurationMs(ms int) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms < 60000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	return fmt.Sprintf("%.1fm", float64(ms)/60000)
}

// newContextStatusCommand creates the context status subcommand.
func newContextStatusCommand(deps *ContextCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check Context-Palace connection status",
		Long:  `Check the connection status to Context-Palace.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runContextStatus(cmd, deps)
		},
	}

	return cmd
}

// runContextStatus checks the Context-Palace connection.
func runContextStatus(cmd *cobra.Command, deps *ContextCommandDeps) error {
	// Load config if not already loaded.
	cfg := deps.Config
	if cfg == nil {
		var err error
		cfg, err = deps.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
	}

	// Check Context-Palace configuration.
	if cfg.ContextPalace == nil || !cfg.ContextPalace.IsConfigured() {
		fmt.Println("Context-Palace: NOT CONFIGURED")
		fmt.Println()
		fmt.Println("Add the following to ~/.penf/config.yaml:")
		fmt.Println()
		fmt.Println("context_palace:")
		fmt.Println("  host: dev02.brown.chat")
		fmt.Println("  database: contextpalace")
		fmt.Println("  user: penfold")
		fmt.Println("  sslmode: verify-full")
		fmt.Println("  project: penfold")
		fmt.Println("  agent: agent-mycroft")
		return nil
	}

	fmt.Printf("Context-Palace configuration:\n")
	fmt.Printf("  Host:     %s\n", cfg.ContextPalace.Host)
	fmt.Printf("  Database: %s\n", cfg.ContextPalace.Database)
	fmt.Printf("  User:     %s\n", cfg.ContextPalace.User)
	fmt.Printf("  Project:  %s\n", cfg.ContextPalace.GetProject())
	fmt.Printf("  Agent:    %s\n", cfg.ContextPalace.GetAgent())
	fmt.Println()

	// Try to connect.
	cpClient, err := contextpalace.NewClient(cfg.ContextPalace)
	if err != nil {
		fmt.Printf("Connection: \033[31mFAILED\033[0m (%v)\n", err)
		return nil
	}
	defer cpClient.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
	defer cancel()

	if err := cpClient.Ping(ctx); err != nil {
		fmt.Printf("Connection: \033[31mFAILED\033[0m (%v)\n", err)
		return nil
	}

	fmt.Printf("Connection: \033[32mOK\033[0m\n")
	return nil
}
