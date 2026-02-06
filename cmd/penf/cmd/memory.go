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

// MemoryCommandDeps holds dependencies for the memory command.
type MemoryCommandDeps struct {
	Config     *config.CLIConfig
	LoadConfig func() (*config.CLIConfig, error)
}

// DefaultMemoryDeps returns the default dependencies for the memory command.
func DefaultMemoryDeps() *MemoryCommandDeps {
	return &MemoryCommandDeps{
		LoadConfig: config.LoadConfig,
	}
}

// NewMemoryCommand creates the memory command group.
func NewMemoryCommand(deps *MemoryCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultMemoryDeps()
	}

	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Manage persistent memory shards in Context-Palace",
		Long: `Commands for managing persistent memory shards in Context-Palace.

Memory shards are persistent notes, reminders, and context that survive across
sessions. They can be triggered based on conditions and deferred to specific times.

Types of memory shards:
  - memory:        General persistent notes
  - memory:alarm:  Time-triggered reminders
  - memory:condition: Condition-triggered context

Examples:
  # Add a simple memory
  penf memory add "Remember to review PRs daily"

  # Add a triggered memory
  penf memory add "Check metrics dashboard" --trigger "when deploying"

  # List active memories
  penf memory list

  # Search memories
  penf memory search "dashboard"

  # Resolve a memory
  penf memory resolve pf-abc123

  # Defer a memory to later
  penf memory defer pf-abc123 --until "2026-02-15"

Related Commands:
  penf reminders   Time-triggered reminders (subset of memory)
  penf session     Work session management
  penf context     Command history and project context`,
	}

	cmd.AddCommand(newMemoryAddCommand(deps))
	cmd.AddCommand(newMemoryListCommand(deps))
	cmd.AddCommand(newMemorySearchCommand(deps))
	cmd.AddCommand(newMemoryResolveCommand(deps))
	cmd.AddCommand(newMemoryDeferCommand(deps))

	return cmd
}

// Memory command flags.
var (
	memoryTrigger   string
	memoryTriggered bool
	memoryUntil     string
	memoryLimit     int
)

// newMemoryAddCommand creates the 'memory add' subcommand.
func newMemoryAddCommand(deps *MemoryCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <message>",
		Short: "Add a new memory shard",
		Long: `Add a new memory shard to Context-Palace.

Memories persist across sessions and can be triggered by conditions.

Examples:
  # Simple memory
  penf memory add "Review metrics weekly"

  # Conditional memory
  penf memory add "Check logs after deploy" --trigger "deployment"

  # Time-based reminder
  penf memory add "Quarterly planning" --trigger "2026-04-01"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryAdd(cmd, deps, args[0])
		},
	}

	cmd.Flags().StringVar(&memoryTrigger, "trigger", "", "Trigger condition or date for this memory")

	return cmd
}

// runMemoryAdd adds a new memory shard.
func runMemoryAdd(cmd *cobra.Command, deps *MemoryCommandDeps, message string) error {
	cfg := deps.Config
	if cfg == nil {
		var err error
		cfg, err = deps.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
	}

	if cfg.ContextPalace == nil || !cfg.ContextPalace.IsConfigured() {
		return fmt.Errorf("Context-Palace not configured. Add context_palace section to ~/.penf/config.yaml")
	}

	cpClient, err := contextpalace.NewClient(cfg.ContextPalace)
	if err != nil {
		return fmt.Errorf("connecting to Context-Palace: %w", err)
	}
	defer cpClient.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()

	// Determine shard type based on trigger.
	shardType := "memory"
	title := message
	content := message

	if memoryTrigger != "" {
		// Check if trigger is a date.
		if _, err := time.Parse("2006-01-02", memoryTrigger); err == nil {
			shardType = "memory:alarm"
			title = fmt.Sprintf("Reminder: %s", message)
			content = fmt.Sprintf("Trigger date: %s\n\n%s", memoryTrigger, message)
		} else {
			shardType = "memory:condition"
			title = fmt.Sprintf("When %s: %s", memoryTrigger, message)
			content = fmt.Sprintf("Trigger: %s\n\n%s", memoryTrigger, message)
		}
	}

	// Create the shard.
	opts := []contextpalace.ShardOption{
		contextpalace.WithPriority(2), // Normal priority
	}

	if memoryTrigger != "" {
		opts = append(opts, contextpalace.WithLabels("triggered", memoryTrigger))
	}

	shard, err := cpClient.CreateShard(ctx, shardType, title, content, opts...)
	if err != nil {
		return fmt.Errorf("creating memory shard: %w", err)
	}

	fmt.Printf("\033[32mAdded memory:\033[0m %s\n", shard.ID)
	fmt.Printf("  Type:    %s\n", shard.Type)
	fmt.Printf("  Title:   %s\n", shard.Title)
	if memoryTrigger != "" {
		fmt.Printf("  Trigger: %s\n", memoryTrigger)
	}

	return nil
}

// newMemoryListCommand creates the 'memory list' subcommand.
func newMemoryListCommand(deps *MemoryCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List memory shards",
		Long: `List active memory shards from Context-Palace.

Examples:
  # List all memories
  penf memory list

  # List only triggered memories
  penf memory list --triggered

  # Limit results
  penf memory list -n 10`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryList(cmd, deps)
		},
	}

	cmd.Flags().BoolVar(&memoryTriggered, "triggered", false, "Only show triggered memories")
	cmd.Flags().IntVarP(&memoryLimit, "limit", "n", 50, "Maximum number of results")

	return cmd
}

// runMemoryList lists memory shards.
func runMemoryList(cmd *cobra.Command, deps *MemoryCommandDeps) error {
	cfg := deps.Config
	if cfg == nil {
		var err error
		cfg, err = deps.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
	}

	if cfg.ContextPalace == nil || !cfg.ContextPalace.IsConfigured() {
		return fmt.Errorf("Context-Palace not configured. Add context_palace section to ~/.penf/config.yaml")
	}

	cpClient, err := contextpalace.NewClient(cfg.ContextPalace)
	if err != nil {
		return fmt.Errorf("connecting to Context-Palace: %w", err)
	}
	defer cpClient.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()

	// Build filter options.
	opts := contextpalace.ListShardsOptions{
		Status: "open",
		Limit:  memoryLimit,
	}

	if memoryTriggered {
		opts.Labels = []string{"triggered"}
	}

	// Query for memory shards (all memory types).
	var allShards []contextpalace.Shard

	memoryTypes := []string{"memory", "memory:alarm", "memory:condition"}
	for _, memType := range memoryTypes {
		opts.Type = memType
		shards, err := cpClient.ListShards(ctx, opts)
		if err != nil {
			return fmt.Errorf("listing memory shards: %w", err)
		}
		allShards = append(allShards, shards...)
	}

	// Output the shards.
	return outputMemoryShards(cmd, cfg, allShards)
}

// newMemorySearchCommand creates the 'memory search' subcommand.
func newMemorySearchCommand(deps *MemoryCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search memory shards",
		Long: `Search memory shards using full-text search.

Examples:
  penf memory search "review"
  penf memory search "metrics dashboard"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			return runMemorySearch(cmd, deps, query)
		},
	}

	return cmd
}

// runMemorySearch searches memory shards.
func runMemorySearch(cmd *cobra.Command, deps *MemoryCommandDeps, query string) error {
	cfg := deps.Config
	if cfg == nil {
		var err error
		cfg, err = deps.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
	}

	if cfg.ContextPalace == nil || !cfg.ContextPalace.IsConfigured() {
		return fmt.Errorf("Context-Palace not configured. Add context_palace section to ~/.penf/config.yaml")
	}

	cpClient, err := contextpalace.NewClient(cfg.ContextPalace)
	if err != nil {
		return fmt.Errorf("connecting to Context-Palace: %w", err)
	}
	defer cpClient.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()

	// Search memory shards.
	shards, err := cpClient.SearchShards(ctx, query, "memory")
	if err != nil {
		return fmt.Errorf("searching memory shards: %w", err)
	}

	// Output the shards.
	return outputMemoryShards(cmd, cfg, shards)
}

// newMemoryResolveCommand creates the 'memory resolve' subcommand.
func newMemoryResolveCommand(deps *MemoryCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <id>",
		Short: "Resolve a memory shard",
		Long: `Mark a memory shard as resolved and close it.

Examples:
  penf memory resolve pf-abc123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryResolve(cmd, deps, args[0])
		},
	}

	return cmd
}

// runMemoryResolve resolves a memory shard.
func runMemoryResolve(cmd *cobra.Command, deps *MemoryCommandDeps, id string) error {
	cfg := deps.Config
	if cfg == nil {
		var err error
		cfg, err = deps.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
	}

	if cfg.ContextPalace == nil || !cfg.ContextPalace.IsConfigured() {
		return fmt.Errorf("Context-Palace not configured. Add context_palace section to ~/.penf/config.yaml")
	}

	cpClient, err := contextpalace.NewClient(cfg.ContextPalace)
	if err != nil {
		return fmt.Errorf("connecting to Context-Palace: %w", err)
	}
	defer cpClient.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()

	// Get the shard to show what we're resolving.
	shard, err := cpClient.GetShard(ctx, id)
	if err != nil {
		return fmt.Errorf("getting memory shard: %w", err)
	}

	// Close the shard.
	if err := cpClient.CloseShard(ctx, id, "Resolved via CLI"); err != nil {
		return fmt.Errorf("resolving memory shard: %w", err)
	}

	fmt.Printf("\033[32mResolved memory:\033[0m %s\n", shard.ID)
	fmt.Printf("  Title: %s\n", shard.Title)

	return nil
}

// newMemoryDeferCommand creates the 'memory defer' subcommand.
func newMemoryDeferCommand(deps *MemoryCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "defer <id>",
		Short: "Defer a memory shard to a later date",
		Long: `Defer a memory shard by setting an expiration time.

The shard will remain in the backlog until the specified date.

Examples:
  penf memory defer pf-abc123 --until "2026-02-15"
  penf memory defer pf-abc123 --until "2026-03-01"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryDefer(cmd, deps, args[0])
		},
	}

	cmd.Flags().StringVar(&memoryUntil, "until", "", "Defer until this date (YYYY-MM-DD)")
	cmd.MarkFlagRequired("until")

	return cmd
}

// runMemoryDefer defers a memory shard.
func runMemoryDefer(cmd *cobra.Command, deps *MemoryCommandDeps, id string) error {
	cfg := deps.Config
	if cfg == nil {
		var err error
		cfg, err = deps.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
	}

	if cfg.ContextPalace == nil || !cfg.ContextPalace.IsConfigured() {
		return fmt.Errorf("Context-Palace not configured. Add context_palace section to ~/.penf/config.yaml")
	}

	cpClient, err := contextpalace.NewClient(cfg.ContextPalace)
	if err != nil {
		return fmt.Errorf("connecting to Context-Palace: %w", err)
	}
	defer cpClient.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()

	// Parse the until date.
	untilDate, err := time.Parse("2006-01-02", memoryUntil)
	if err != nil {
		return fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
	}

	// Get the shard to show what we're deferring.
	shard, err := cpClient.GetShard(ctx, id)
	if err != nil {
		return fmt.Errorf("getting memory shard: %w", err)
	}

	// Update the shard with expiration time.
	if err := cpClient.UpdateShard(ctx, id, contextpalace.WithExpiresAt(untilDate)); err != nil {
		return fmt.Errorf("deferring memory shard: %w", err)
	}

	fmt.Printf("\033[32mDeferred memory:\033[0m %s\n", shard.ID)
	fmt.Printf("  Title: %s\n", shard.Title)
	fmt.Printf("  Until: %s\n", untilDate.Format("2006-01-02"))

	return nil
}

// outputMemoryShards outputs memory shards in the configured format.
func outputMemoryShards(cmd *cobra.Command, cfg *config.CLIConfig, shards []contextpalace.Shard) error {
	// Check for output format flag override.
	format := cfg.OutputFormat
	if f := cmd.Flag("output"); f != nil && f.Changed {
		format = config.OutputFormat(f.Value.String())
	}

	switch format {
	case config.OutputFormatJSON:
		return outputMemoryShardsJSON(shards)
	case config.OutputFormatYAML:
		return outputMemoryShardsYAML(shards)
	default:
		return outputMemoryShardsText(shards)
	}
}

// outputMemoryShardsJSON outputs memory shards as JSON.
func outputMemoryShardsJSON(shards []contextpalace.Shard) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(shards)
}

// outputMemoryShardsYAML outputs memory shards as YAML.
func outputMemoryShardsYAML(shards []contextpalace.Shard) error {
	enc := yaml.NewEncoder(os.Stdout)
	return enc.Encode(shards)
}

// outputMemoryShardsText outputs memory shards in human-readable format.
func outputMemoryShardsText(shards []contextpalace.Shard) error {
	if len(shards) == 0 {
		fmt.Println("No memory shards found.")
		return nil
	}

	fmt.Printf("Memory Shards (%d):\n\n", len(shards))

	for _, s := range shards {
		// Format type (remove "memory:" prefix for brevity).
		shardType := strings.TrimPrefix(s.Type, "memory:")
		if shardType == "memory" {
			shardType = "note"
		}

		// Format timestamp.
		created := s.CreatedAt.Local().Format("2006-01-02")

		// Show ID, type, and title.
		fmt.Printf("  \033[36m%s\033[0m (%s, %s)\n", s.ID, shardType, created)
		fmt.Printf("    %s\n", s.Title)

		// Show labels if present.
		if len(s.Labels) > 0 {
			labelStr := strings.Join(s.Labels, ", ")
			fmt.Printf("    Labels: %s\n", labelStr)
		}

		// Show expiration if set.
		if s.ExpiresAt != nil {
			fmt.Printf("    Deferred until: %s\n", s.ExpiresAt.Local().Format("2006-01-02"))
		}

		fmt.Println()
	}

	return nil
}
