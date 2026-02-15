// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/pkg/enrichment"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/classification"
)

// Classify command flags
var (
	classifyOutput  string
	classifyAll     bool
	classifyDryRun  bool
	classifyTenant  string
)

// ClassifyCommandDeps holds the dependencies for classify commands.
type ClassifyCommandDeps struct {
	Config     *config.CLIConfig
	LoadConfig func() (*config.CLIConfig, error)
}

// DefaultClassifyDeps returns the default dependencies for production use.
func DefaultClassifyDeps() *ClassifyCommandDeps {
	return &ClassifyCommandDeps{
		LoadConfig: config.LoadConfig,
	}
}

// NewClassifyCommand creates the root classify command with all subcommands.
func NewClassifyCommand(deps *ClassifyCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultClassifyDeps()
	}

	cmd := &cobra.Command{
		Use:   "classify",
		Short: "Manage source system classification for content items",
		Long: `Classify content items by their source system.

Source system classification identifies which external system generated a piece
of content (e.g., JIRA, Aha, Google Docs, Webex, human email). This helps with:
  - Filtering noise from automated systems
  - Routing content to appropriate processing pipelines
  - Understanding communication patterns and tool usage

Classification is rule-based and uses patterns in the 'from' address, subject line,
message ID, and email headers. Rules are defined in the codebase and executed with
priority ordering.

Use this command to:
  - View active classification rules
  - See classification statistics across your content
  - Run classification on pending items or reclassify all content
  - Test classification on a single item

For AI assistants:
  Use --output json for structured data suitable for programmatic processing.
  The 'rules' subcommand shows the active rule set without requiring gateway access.`,
		Example: `  # View active classification rules
  penf classify rules

  # See classification breakdown
  penf classify stats

  # Classify all items marked as 'unknown'
  penf classify run

  # Reclassify everything
  penf classify run --all

  # Test classification on a single item (dry run)
  penf classify run <content-id> --dry-run

  # Output as JSON
  penf classify stats --output json`,
	}

	// Add subcommands
	cmd.AddCommand(newClassifyRunCommand(deps))
	cmd.AddCommand(newClassifyStatsCommand(deps))
	cmd.AddCommand(newClassifyRulesCommand(deps))

	return cmd
}

// newClassifyRunCommand creates the 'classify run' subcommand.
func newClassifyRunCommand(deps *ClassifyCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [id]",
		Short: "Run classification on content items",
		Long: `Run source system classification on content items.

By default, classifies all items with source_system = 'unknown'.
Use --all to reclassify everything, ignoring current source_system values.
Use --dry-run to preview changes without persisting to the database.

Provide a content ID to classify a single item.

Examples:
  # Classify all unknown items
  penf classify run

  # Reclassify everything
  penf classify run --all

  # Dry run (show what would change)
  penf classify run --dry-run

  # Classify a single item
  penf classify run em-abc123

  # Dry run for a single item
  penf classify run em-abc123 --dry-run`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var contentID string
			if len(args) == 1 {
				contentID = args[0]
			}
			return runClassify(cmd.Context(), deps, contentID)
		},
	}

	cmd.Flags().BoolVar(&classifyAll, "all", false, "Reclassify all items (ignore current classification)")
	cmd.Flags().BoolVar(&classifyDryRun, "dry-run", false, "Show what would change without persisting")
	cmd.Flags().StringVar(&classifyTenant, "tenant", "", "Tenant ID (defaults to config tenant)")
	cmd.Flags().StringVarP(&classifyOutput, "output", "o", "", "Output format: text, json, yaml")

	return cmd
}

// newClassifyStatsCommand creates the 'classify stats' subcommand.
func newClassifyStatsCommand(deps *ClassifyCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show classification statistics",
		Long: `Show statistics about source system classification.

Displays a breakdown of content items by source_system value, showing
the count for each classification category.

Examples:
  # Show classification stats
  penf classify stats

  # Output as JSON
  penf classify stats --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClassifyStats(cmd.Context(), deps)
		},
	}

	cmd.Flags().StringVar(&classifyTenant, "tenant", "", "Tenant ID (defaults to config tenant)")
	cmd.Flags().StringVarP(&classifyOutput, "output", "o", "", "Output format: text, json, yaml")

	return cmd
}

// newClassifyRulesCommand creates the 'classify rules' subcommand.
func newClassifyRulesCommand(deps *ClassifyCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "Show active classification rules",
		Long: `Display the active classification rules table.

Rules are defined in the codebase and executed in priority order. Each rule
has conditions that are evaluated with OR logic - if any condition matches,
the rule fires and assigns the source_system.

This command works without gateway access since rules are hardcoded in the Go code.

Examples:
  # Show classification rules
  penf classify rules

  # Output as JSON
  penf classify rules --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClassifyRules(cmd.Context(), deps)
		},
	}

	cmd.Flags().StringVarP(&classifyOutput, "output", "o", "", "Output format: text, json, yaml")

	return cmd
}

// Command execution functions

func runClassify(ctx context.Context, deps *ClassifyCommandDeps, contentID string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// This requires gateway RPC implementation
	return fmt.Errorf("classify run RPC not yet available — requires gateway implementation (Wave 2: pf-c29635)")
}

func runClassifyStats(ctx context.Context, deps *ClassifyCommandDeps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// This requires gateway RPC implementation
	return fmt.Errorf("classify stats RPC not yet available — requires gateway implementation (Wave 2: pf-c29635)")
}

func runClassifyRules(ctx context.Context, deps *ClassifyCommandDeps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Get the rules from the classification package
	// These are hardcoded in source_system.go, so we define them here for display
	rules := getClassificationRules()

	// Output the rules
	format := cfg.OutputFormat
	if classifyOutput != "" {
		format = config.OutputFormat(classifyOutput)
	}

	return outputClassifyRules(format, rules)
}

// ClassificationRule represents a single classification rule.
type ClassificationRule struct {
	Priority     int                      `json:"priority"`
	Name         string                   `json:"name"`
	SourceSystem enrichment.SourceSystem  `json:"source_system"`
	Conditions   []string                 `json:"conditions"`
}

// getClassificationRules returns the hardcoded classification rules.
// These match the rules in pkg/enrichment/classification/source_system.go
func getClassificationRules() []ClassificationRule {
	return []ClassificationRule{
		{
			Priority:     1,
			Name:         "JIRA",
			SourceSystem: enrichment.SourceSystemJira,
			Conditions: []string{
				"from contains 'jira'",
				"subject starts with '[TRACK-JIRA]'",
				"message_id contains '@Atlassian.JIRA'",
			},
		},
		{
			Priority:     2,
			Name:         "Aha",
			SourceSystem: enrichment.SourceSystemAha,
			Conditions: []string{
				"from matches '*@*.mailer.aha.io'",
				"subject starts with '[AHA]'",
			},
		},
		{
			Priority:     3,
			Name:         "Google Docs",
			SourceSystem: enrichment.SourceSystemGoogleDocs,
			Conditions: []string{
				"from matches '*-noreply@docs.google.com'",
				"from = 'drive-shares-dm-noreply@google.com'",
			},
		},
		{
			Priority:     4,
			Name:         "Webex",
			SourceSystem: enrichment.SourceSystemWebex,
			Conditions: []string{
				"from = 'messenger@webex.com'",
			},
		},
		{
			Priority:     5,
			Name:         "Smartsheet",
			SourceSystem: enrichment.SourceSystemSmartsheet,
			Conditions: []string{
				"from matches '*@*.smartsheet.com'",
			},
		},
		{
			Priority:     6,
			Name:         "Auto-reply",
			SourceSystem: enrichment.SourceSystemAutoReply,
			Conditions: []string{
				"subject starts with 'Automatic reply:'",
			},
		},
		{
			Priority:     7,
			Name:         "Calendar Cancellation",
			SourceSystem: enrichment.SourceSystemOutlookCalendar,
			Conditions: []string{
				"subject starts with 'Canceled:'",
				"subject starts with 'Cancelled:'",
			},
		},
		{
			Priority:     8,
			Name:         "Calendar Invite",
			SourceSystem: enrichment.SourceSystemOutlookCalendar,
			Conditions: []string{
				"Content-Type header contains 'text/calendar'",
			},
		},
		{
			Priority:     99,
			Name:         "Default (Human Email)",
			SourceSystem: enrichment.SourceSystemHumanEmail,
			Conditions: []string{
				"default if no other rules match",
			},
		},
	}
}

// outputClassifyRules outputs the classification rules.
func outputClassifyRules(format config.OutputFormat, rules []ClassificationRule) error {
	switch format {
	case config.OutputFormatJSON:
		return outputClassifyRulesJSON(rules)
	case config.OutputFormatYAML:
		return outputClassifyRulesYAML(rules)
	default:
		return outputClassifyRulesText(rules)
	}
}

func outputClassifyRulesJSON(rules []ClassificationRule) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]interface{}{
		"rules": rules,
	})
}

func outputClassifyRulesYAML(rules []ClassificationRule) error {
	enc := yaml.NewEncoder(os.Stdout)
	return enc.Encode(map[string]interface{}{
		"rules": rules,
	})
}

func outputClassifyRulesText(rules []ClassificationRule) error {
	fmt.Println("Classification Rules (evaluated in priority order):")
	fmt.Println()
	fmt.Println("  PRIORITY  RULE NAME                 SOURCE SYSTEM          CONDITIONS")
	fmt.Println("  --------  ---------                 -------------          ----------")

	for _, rule := range rules {
		// First line: priority, name, source_system, first condition
		firstCondition := ""
		if len(rule.Conditions) > 0 {
			firstCondition = rule.Conditions[0]
		}

		fmt.Printf("  %-8d  %-24s  %-21s  %s\n",
			rule.Priority,
			rule.Name,
			string(rule.SourceSystem),
			firstCondition)

		// Subsequent lines: additional conditions
		for i := 1; i < len(rule.Conditions); i++ {
			fmt.Printf("  %-8s  %-24s  %-21s  OR %s\n",
				"",
				"",
				"",
				rule.Conditions[i])
		}
	}

	fmt.Println()
	fmt.Println("Note: Rules use OR logic - if ANY condition matches, the rule fires.")
	fmt.Println("      Rules are evaluated in priority order (lowest number = highest priority).")
	fmt.Println("      The first matching rule wins.")
	fmt.Println()

	return nil
}

// Verify the classification package is available (for testing)
var _ = classification.ClassifySourceSystem
