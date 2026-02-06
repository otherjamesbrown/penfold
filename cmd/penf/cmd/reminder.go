// Package cmd provides CLI command implementations.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/cmd/penf/contextpalace"
)

// ReminderCommandDeps holds dependencies for the reminder command.
type ReminderCommandDeps struct {
	Config     *config.CLIConfig
	LoadConfig func() (*config.CLIConfig, error)
}

// DefaultReminderDeps returns the default dependencies for the reminder command.
func DefaultReminderDeps() *ReminderCommandDeps {
	return &ReminderCommandDeps{
		LoadConfig: config.LoadConfig,
	}
}

// NewReminderCommand creates the reminder command group.
func NewReminderCommand(deps *ReminderCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultReminderDeps()
	}

	cmd := &cobra.Command{
		Use:   "reminders",
		Short: "Manage reminders",
		Long: `Commands for managing time-based reminders in Context-Palace.

Reminders are scheduled notifications stored as shards. They have a
remind_after timestamp that determines when they become due.

Examples:
  penf reminders list              List all reminders
  penf reminders list --due        List only due reminders
  penf reminders list --due -o json  Due reminders as JSON

Related Commands:
  penf memory       Persistent notes and context (not time-triggered)
  penf session      Work session management`,
		Aliases: []string{"reminder"},
	}

	cmd.AddCommand(newReminderListCommand(deps))

	return cmd
}

// newReminderListCommand creates the 'reminders list' subcommand.
func newReminderListCommand(deps *ReminderCommandDeps) *cobra.Command {
	var dueOnly bool
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List reminders",
		Long: `List reminders from Context-Palace.

Use --due to show only reminders that are due now or overdue.

Examples:
  penf reminders list --due
  penf reminders list --due -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReminderList(cmd, deps, dueOnly, outputFormat)
		},
	}

	cmd.Flags().BoolVar(&dueOnly, "due", false, "Show only due reminders")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "", "Output format: text, json, yaml")

	return cmd
}

// runReminderList lists reminders.
func runReminderList(cmd *cobra.Command, deps *ReminderCommandDeps, dueOnly bool, outputFmt string) error {
	cfg := deps.Config
	if cfg == nil {
		var err error
		cfg, err = deps.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
	}

	if cfg.ContextPalace == nil || !cfg.ContextPalace.IsConfigured() {
		return fmt.Errorf("Context-Palace not configured")
	}

	cpClient, err := contextpalace.NewClient(cfg.ContextPalace)
	if err != nil {
		return fmt.Errorf("connecting to Context-Palace: %w", err)
	}
	defer cpClient.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()

	// Query all open reminders
	shards, err := cpClient.ListShards(ctx, contextpalace.ListShardsOptions{
		Type:   "reminder",
		Status: "open",
		Limit:  100,
	})
	if err != nil {
		return fmt.Errorf("listing reminders: %w", err)
	}

	// Filter by due date if requested
	var reminders []contextpalace.Shard
	if dueOnly {
		now := time.Now()
		for _, shard := range shards {
			if isReminderDue(shard.Content, now) {
				reminders = append(reminders, shard)
			}
		}
	} else {
		reminders = shards
	}

	// Output based on format
	format := cfg.OutputFormat
	if outputFmt != "" {
		format = config.OutputFormat(outputFmt)
	}

	switch format {
	case config.OutputFormatJSON:
		return outputRemindersJSON(reminders)
	case config.OutputFormatYAML:
		return outputRemindersYAML(reminders)
	default:
		return outputRemindersText(reminders)
	}
}

// isReminderDue checks if a reminder is due based on its content.
func isReminderDue(content string, now time.Time) bool {
	// Parse content as JSON to extract remind_after
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		// If not JSON, assume it's due (old format or malformed)
		return true
	}

	remindAfterStr, ok := data["remind_after"].(string)
	if !ok || remindAfterStr == "" {
		// No remind_after field, consider it due
		return true
	}

	remindAfter, err := time.Parse(time.RFC3339, remindAfterStr)
	if err != nil {
		// Can't parse, consider it due
		return true
	}

	return remindAfter.Before(now) || remindAfter.Equal(now)
}

// outputRemindersJSON outputs reminders as JSON.
func outputRemindersJSON(reminders []contextpalace.Shard) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(reminders)
}

// outputRemindersYAML outputs reminders as YAML.
func outputRemindersYAML(reminders []contextpalace.Shard) error {
	enc := yaml.NewEncoder(os.Stdout)
	return enc.Encode(reminders)
}

// outputRemindersText outputs reminders in human-readable format.
func outputRemindersText(reminders []contextpalace.Shard) error {
	if len(reminders) == 0 {
		fmt.Println("No due reminders.")
		return nil
	}

	fmt.Printf("Due Reminders (%d):\n\n", len(reminders))
	fmt.Println("  ID          TITLE                                DUE DATE")
	fmt.Println("  --          -----                                --------")

	for _, r := range reminders {
		title := r.Title
		if len(title) > 36 {
			title = title[:33] + "..."
		}

		dueDate := extractDueDate(r.Content)
		fmt.Printf("  %-11s %-36s %s\n", r.ID, title, dueDate)
	}

	fmt.Println()
	return nil
}

// extractDueDate extracts the due date from reminder content.
func extractDueDate(content string) string {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		return "(unknown)"
	}

	remindAfterStr, ok := data["remind_after"].(string)
	if !ok || remindAfterStr == "" {
		return "(now)"
	}

	remindAfter, err := time.Parse(time.RFC3339, remindAfterStr)
	if err != nil {
		return "(parse error)"
	}

	return remindAfter.Format("2006-01-02 15:04")
}
