// Package cmd provides CLI command implementations.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/cmd/penf/contextpalace"
)

// BacklogCommandDeps holds dependencies for the backlog command.
type BacklogCommandDeps struct {
	Config     *config.CLIConfig
	LoadConfig func() (*config.CLIConfig, error)
}

// DefaultBacklogDeps returns the default dependencies for the backlog command.
func DefaultBacklogDeps() *BacklogCommandDeps {
	return &BacklogCommandDeps{
		LoadConfig: config.LoadConfig,
	}
}

// Backlog command flags.
var (
	backlogPriority string
	backlogContext  string
	backlogMine     bool
	backlogStatus   string
)

// NewBacklogCommand creates the backlog command group.
func NewBacklogCommand(deps *BacklogCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultBacklogDeps()
	}

	cmd := &cobra.Command{
		Use:   "backlog",
		Short: "Manage development backlog items",
		Long: `Commands for managing development backlog in Context-Palace.

The backlog tracks development work items, feature requests, and technical debt.
Items have priority levels and can be linked to other shards for context.

Priority levels:
  critical - 0: Critical work that blocks progress
  high     - 1: Important work to do soon
  medium   - 2: Normal priority (default)
  low      - 3: Nice to have
  backlog  - 4: Future consideration

Examples:
  penf backlog add "Implement search filters" --priority high
  penf backlog list --mine
  penf backlog show pf-abc123
  penf backlog update pf-abc123 --status in_progress
  penf backlog close pf-abc123 "Completed: Added filters"`,
	}

	cmd.AddCommand(newBacklogAddCommand(deps))
	cmd.AddCommand(newBacklogListCommand(deps))
	cmd.AddCommand(newBacklogShowCommand(deps))
	cmd.AddCommand(newBacklogUpdateCommand(deps))
	cmd.AddCommand(newBacklogCloseCommand(deps))

	return cmd
}

// newBacklogAddCommand creates the 'backlog add' subcommand.
func newBacklogAddCommand(deps *BacklogCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a backlog item",
		Long: `Add a new item to the development backlog.

Priority levels:
  critical (0), high (1), medium (2), low (3), backlog (4)

Examples:
  penf backlog add "Implement OAuth flow"
  penf backlog add "Refactor auth module" --priority high
  penf backlog add "Fix bug in parser" --priority critical --context pf-bug123`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := strings.Join(args, " ")
			return runBacklogAdd(cmd, deps, title)
		},
	}

	cmd.Flags().StringVar(&backlogPriority, "priority", "medium", "Priority level: critical, high, medium, low, backlog")
	cmd.Flags().StringVar(&backlogContext, "context", "", "Related shard ID for context (e.g., pf-xxx)")

	return cmd
}

// newBacklogListCommand creates the 'backlog list' subcommand.
func newBacklogListCommand(deps *BacklogCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List backlog items",
		Long: `List development backlog items.

Use --mine to show only items owned by you.

Examples:
  penf backlog list
  penf backlog list --mine
  penf backlog list -o json`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBacklogList(cmd, deps)
		},
	}

	cmd.Flags().BoolVar(&backlogMine, "mine", false, "Show only items owned by me")

	return cmd
}

// newBacklogShowCommand creates the 'backlog show' subcommand.
func newBacklogShowCommand(deps *BacklogCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show backlog item details",
		Long: `Display detailed information about a backlog item.

Example:
  penf backlog show pf-abc123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBacklogShow(cmd, deps, args[0])
		},
	}
}

// newBacklogUpdateCommand creates the 'backlog update' subcommand.
func newBacklogUpdateCommand(deps *BacklogCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a backlog item",
		Long: `Update a backlog item's status or priority.

Status values: open, in_progress, closed

Examples:
  penf backlog update pf-abc123 --status in_progress
  penf backlog update pf-abc123 --priority high`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBacklogUpdate(cmd, deps, args[0])
		},
	}

	cmd.Flags().StringVar(&backlogStatus, "status", "", "New status: open, in_progress, closed")
	cmd.Flags().StringVar(&backlogPriority, "priority", "", "New priority: critical, high, medium, low, backlog")

	return cmd
}

// newBacklogCloseCommand creates the 'backlog close' subcommand.
func newBacklogCloseCommand(deps *BacklogCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "close <id> <resolution>",
		Short: "Close a backlog item",
		Long: `Close a backlog item with a resolution note.

Example:
  penf backlog close pf-abc123 "Completed: Implemented OAuth flow"`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			resolution := strings.Join(args[1:], " ")
			return runBacklogClose(cmd, deps, id, resolution)
		},
	}
}

// parsePriority converts a priority string to an integer.
func parsePriority(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "critical", "0":
		return 0, nil
	case "high", "1":
		return 1, nil
	case "medium", "2":
		return 2, nil
	case "low", "3":
		return 3, nil
	case "backlog", "4":
		return 4, nil
	default:
		return -1, fmt.Errorf("invalid priority: %s (must be critical, high, medium, low, or backlog)", s)
	}
}

// backlogPriorityToString converts a priority integer to a string.
func backlogPriorityToString(p int) string {
	switch p {
	case 0:
		return "critical"
	case 1:
		return "high"
	case 2:
		return "medium"
	case 3:
		return "low"
	case 4:
		return "backlog"
	default:
		return "unknown"
	}
}

// runBacklogAdd adds a new backlog item.
func runBacklogAdd(cmd *cobra.Command, deps *BacklogCommandDeps, title string) error {
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

	priority, err := parsePriority(backlogPriority)
	if err != nil {
		return err
	}

	cpClient, err := contextpalace.NewClient(cfg.ContextPalace)
	if err != nil {
		return fmt.Errorf("connecting to Context-Palace: %w", err)
	}
	defer cpClient.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()

	// Build content.
	content := fmt.Sprintf("Backlog item: %s\nPriority: %s", title, backlogPriorityToString(priority))
	if backlogContext != "" {
		content += fmt.Sprintf("\nContext: %s", backlogContext)
	}

	// Create options.
	opts := []contextpalace.ShardOption{
		contextpalace.WithPriority(priority),
	}

	if backlogContext != "" {
		opts = append(opts, contextpalace.WithLabels("context:"+backlogContext))
	}

	shard, err := cpClient.CreateShard(ctx, "backlog", title, content, opts...)
	if err != nil {
		return fmt.Errorf("creating backlog item: %w", err)
	}

	fmt.Printf("\033[32mAdded backlog item:\033[0m %s\n", shard.ID)
	fmt.Printf("  Title:    %s\n", shard.Title)
	fmt.Printf("  Priority: %s\n", backlogPriorityToString(shard.Priority))
	fmt.Printf("  Status:   %s\n", shard.Status)

	return nil
}

// runBacklogList lists backlog items.
func runBacklogList(cmd *cobra.Command, deps *BacklogCommandDeps) error {
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

	listOpts := contextpalace.ListShardsOptions{
		Type:   "backlog",
		Status: "open",
		Limit:  100,
	}

	if backlogMine {
		listOpts.Owner = cfg.ContextPalace.GetAgent()
	}

	shards, err := cpClient.ListShards(ctx, listOpts)
	if err != nil {
		return fmt.Errorf("listing backlog items: %w", err)
	}

	// Also get in_progress items.
	listOpts.Status = "in_progress"
	inProgressShards, err := cpClient.ListShards(ctx, listOpts)
	if err != nil {
		return fmt.Errorf("listing in-progress items: %w", err)
	}

	// Combine results.
	allShards := append(inProgressShards, shards...)

	return outputBacklogList(cmd, cfg, allShards)
}

// runBacklogShow shows details of a backlog item.
func runBacklogShow(cmd *cobra.Command, deps *BacklogCommandDeps, id string) error {
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

	shard, err := cpClient.GetShard(ctx, id)
	if err != nil {
		return fmt.Errorf("getting backlog item: %w", err)
	}

	return outputBacklogDetail(cmd, cfg, shard)
}

// runBacklogUpdate updates a backlog item.
func runBacklogUpdate(cmd *cobra.Command, deps *BacklogCommandDeps, id string) error {
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

	opts := []contextpalace.UpdateOption{}

	if backlogStatus != "" {
		status := strings.ToLower(strings.TrimSpace(backlogStatus))
		if status != "open" && status != "in_progress" && status != "closed" {
			return fmt.Errorf("invalid status: %s (must be open, in_progress, or closed)", status)
		}
		opts = append(opts, contextpalace.WithStatus(status))
	}

	if backlogPriority != "" {
		priority, err := parsePriority(backlogPriority)
		if err != nil {
			return err
		}
		opts = append(opts, contextpalace.WithUpdatePriority(priority))
	}

	if len(opts) == 0 {
		return fmt.Errorf("no updates specified (use --status or --priority)")
	}

	if err := cpClient.UpdateShard(ctx, id, opts...); err != nil {
		return fmt.Errorf("updating backlog item: %w", err)
	}

	// Fetch updated shard to show changes.
	shard, err := cpClient.GetShard(ctx, id)
	if err != nil {
		return fmt.Errorf("getting updated item: %w", err)
	}

	fmt.Printf("\033[32mUpdated backlog item:\033[0m %s\n", shard.ID)
	fmt.Printf("  Title:    %s\n", shard.Title)
	fmt.Printf("  Status:   %s\n", shard.Status)
	fmt.Printf("  Priority: %s\n", backlogPriorityToString(shard.Priority))

	return nil
}

// runBacklogClose closes a backlog item.
func runBacklogClose(cmd *cobra.Command, deps *BacklogCommandDeps, id, resolution string) error {
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

	if err := cpClient.CloseShard(ctx, id, resolution); err != nil {
		return fmt.Errorf("closing backlog item: %w", err)
	}

	fmt.Printf("\033[32mClosed backlog item:\033[0m %s\n", id)
	fmt.Printf("  Resolution: %s\n", resolution)

	return nil
}

// outputBacklogList outputs the backlog list in the configured format.
func outputBacklogList(cmd *cobra.Command, cfg *config.CLIConfig, shards []contextpalace.Shard) error {
	format := cfg.OutputFormat
	if f := cmd.Flag("output"); f != nil && f.Changed {
		format = config.OutputFormat(f.Value.String())
	}

	switch format {
	case config.OutputFormatJSON:
		return outputBacklogJSON(shards)
	case config.OutputFormatYAML:
		return outputBacklogYAML(shards)
	default:
		return outputBacklogListText(shards)
	}
}

// outputBacklogListText outputs backlog items in human-readable format.
func outputBacklogListText(shards []contextpalace.Shard) error {
	if len(shards) == 0 {
		fmt.Println("No backlog items found.")
		return nil
	}

	fmt.Printf("Backlog Items (%d):\n\n", len(shards))
	fmt.Println("  ID          STATUS        PRIORITY  TITLE")
	fmt.Println("  --          ------        --------  -----")

	for _, s := range shards {
		statusColor := ""
		switch s.Status {
		case "in_progress":
			statusColor = "\033[33m" // Yellow
		case "closed":
			statusColor = "\033[32m" // Green
		default:
			statusColor = "\033[90m" // Gray
		}

		priorityStr := backlogPriorityToString(s.Priority)
		titleStr := s.Title
		if len(titleStr) > 50 {
			titleStr = titleStr[:47] + "..."
		}

		fmt.Printf("  %-11s %s%-13s\033[0m %-9s %s\n",
			s.ID,
			statusColor,
			s.Status,
			priorityStr,
			titleStr)
	}

	fmt.Println()
	return nil
}

// outputBacklogDetail outputs backlog item details.
func outputBacklogDetail(cmd *cobra.Command, cfg *config.CLIConfig, shard *contextpalace.Shard) error {
	format := cfg.OutputFormat
	if f := cmd.Flag("output"); f != nil && f.Changed {
		format = config.OutputFormat(f.Value.String())
	}

	switch format {
	case config.OutputFormatJSON:
		return outputBacklogJSON(shard)
	case config.OutputFormatYAML:
		return outputBacklogYAML(shard)
	default:
		return outputBacklogDetailText(shard)
	}
}

// outputBacklogDetailText outputs backlog item details in human-readable format.
func outputBacklogDetailText(shard *contextpalace.Shard) error {
	fmt.Println("Backlog Item Details:")
	fmt.Println()
	fmt.Printf("  \033[1mID:\033[0m          %s\n", shard.ID)
	fmt.Printf("  \033[1mTitle:\033[0m       %s\n", shard.Title)
	fmt.Printf("  \033[1mStatus:\033[0m      %s\n", shard.Status)
	fmt.Printf("  \033[1mPriority:\033[0m    %s\n", backlogPriorityToString(shard.Priority))
	fmt.Println()

	if shard.Content != "" {
		fmt.Printf("  \033[1mContent:\033[0m\n")
		lines := strings.Split(shard.Content, "\n")
		for _, line := range lines {
			fmt.Printf("    %s\n", line)
		}
		fmt.Println()
	}

	if shard.Owner != "" {
		fmt.Printf("  \033[1mOwner:\033[0m       %s\n", shard.Owner)
	}
	fmt.Printf("  \033[1mCreator:\033[0m     %s\n", shard.Creator)
	fmt.Printf("  \033[1mCreated:\033[0m     %s\n", shard.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  \033[1mUpdated:\033[0m     %s\n", shard.UpdatedAt.Format("2006-01-02 15:04:05"))

	if shard.ClosedAt != nil {
		fmt.Printf("  \033[1mClosed:\033[0m      %s\n", shard.ClosedAt.Format("2006-01-02 15:04:05"))
		if shard.Resolution != "" {
			fmt.Printf("  \033[1mResolution:\033[0m  %s\n", shard.Resolution)
		}
	}

	if len(shard.Labels) > 0 {
		fmt.Printf("  \033[1mLabels:\033[0m      %s\n", strings.Join(shard.Labels, ", "))
	}

	return nil
}

// outputBacklogJSON outputs data as JSON.
func outputBacklogJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// outputBacklogYAML outputs data as YAML.
func outputBacklogYAML(v interface{}) error {
	enc := yaml.NewEncoder(os.Stdout)
	return enc.Encode(v)
}
