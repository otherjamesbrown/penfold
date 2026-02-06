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

// SessionCommandDeps holds dependencies for the session command.
type SessionCommandDeps struct {
	Config     *config.CLIConfig
	LoadConfig func() (*config.CLIConfig, error)
}

// DefaultSessionDeps returns the default dependencies for the session command.
func DefaultSessionDeps() *SessionCommandDeps {
	return &SessionCommandDeps{
		LoadConfig: config.LoadConfig,
	}
}

// Session output flags.
var (
	sessionOutputFormat string
)

// NewSessionCommand creates the session command group.
func NewSessionCommand(deps *SessionCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultSessionDeps()
	}

	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage agent work sessions",
		Long: `Commands for managing agent work sessions in Context-Palace.

Sessions track work across context clears. Each session is a shard with
type="session" that stores session state, checkpoints, and work progress.

Sessions enable:
- Persistence across Claude Code context resets
- Checkpoint/resume workflow
- Historical tracking of work sessions
- Session-scoped state management

Examples:
  penf session start              # Begin new session
  penf session checkpoint "msg"   # Save checkpoint
  penf session resume             # Load last checkpoint
  penf session end "summary"      # End current session
  penf session context            # Show current session
  penf session history            # List past sessions

Related Commands:
  penf memory     Persistent notes that survive across sessions
  penf message    Agent-to-agent messaging
  penf context    Command history and project context`,
	}

	cmd.AddCommand(newSessionStartCommand(deps))
	cmd.AddCommand(newSessionCheckpointCommand(deps))
	cmd.AddCommand(newSessionResumeCommand(deps))
	cmd.AddCommand(newSessionEndCommand(deps))
	cmd.AddCommand(newSessionContextCommand(deps))
	cmd.AddCommand(newSessionHistoryCommand(deps))

	return cmd
}

// newSessionStartCommand creates the 'session start' subcommand.
func newSessionStartCommand(deps *SessionCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start [title]",
		Short: "Begin a new work session",
		Long: `Create a new session shard to track work.

A session is a shard with type="session" and status="in_progress". Only
one session can be active per agent at a time.

Examples:
  penf session start                           # Start unnamed session
  penf session start "Implement feature X"     # Start with title
  penf session start -o json                   # JSON output`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := "Work Session"
			if len(args) > 0 {
				title = args[0]
			}
			return runSessionStart(cmd, deps, title)
		},
	}

	cmd.Flags().StringVarP(&sessionOutputFormat, "output", "o", "", "Output format: text, json, yaml")

	return cmd
}

// runSessionStart creates a new session.
func runSessionStart(cmd *cobra.Command, deps *SessionCommandDeps, title string) error {
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

	// Check if there's already an active session for this agent
	existingSessions, err := cpClient.ListShards(ctx, contextpalace.ListShardsOptions{
		Type:   "session",
		Status: "in_progress",
		Owner:  cfg.ContextPalace.GetAgent(),
		Limit:  1,
	})
	if err != nil {
		return fmt.Errorf("checking for existing session: %w", err)
	}

	if len(existingSessions) > 0 {
		return fmt.Errorf("session already active: %s (end it with 'penf session end' first)", existingSessions[0].ID)
	}

	// Create the session shard
	content := fmt.Sprintf("Started: %s\n\nAgent: %s\n", time.Now().Format(time.RFC3339), cfg.ContextPalace.GetAgent())
	shard, err := cpClient.CreateShard(ctx, "session", title, content,
		contextpalace.WithOwner(cfg.ContextPalace.GetAgent()),
		contextpalace.WithPriority(2),
	)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set status to in_progress
	err = cpClient.UpdateShard(ctx, shard.ID, contextpalace.WithStatus("in_progress"))
	if err != nil {
		return fmt.Errorf("updating session status: %w", err)
	}

	// Reload shard to get updated status
	shard, err = cpClient.GetShard(ctx, shard.ID)
	if err != nil {
		return fmt.Errorf("reloading session: %w", err)
	}

	return outputSession(cmd, cfg, shard, "Session started")
}

// newSessionCheckpointCommand creates the 'session checkpoint' subcommand.
func newSessionCheckpointCommand(deps *SessionCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkpoint <message>",
		Short: "Save a checkpoint in the current session",
		Long: `Append a checkpoint message to the current session.

Checkpoints are appended to the session content with timestamps,
creating a log of work progress.

Examples:
  penf session checkpoint "Completed database migration"
  penf session checkpoint "Fixed bug in auth flow"
  penf session checkpoint -o json "Checkpoint message"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionCheckpoint(cmd, deps, args[0])
		},
	}

	cmd.Flags().StringVarP(&sessionOutputFormat, "output", "o", "", "Output format: text, json, yaml")

	return cmd
}

// runSessionCheckpoint saves a checkpoint.
func runSessionCheckpoint(cmd *cobra.Command, deps *SessionCommandDeps, message string) error {
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

	// Find active session
	shard, err := getActiveSession(ctx, cpClient, cfg.ContextPalace.GetAgent())
	if err != nil {
		return err
	}

	// Append checkpoint to content
	checkpoint := fmt.Sprintf("\n[%s] %s\n", time.Now().Format(time.RFC3339), message)
	newContent := shard.Content + checkpoint

	err = cpClient.UpdateShard(ctx, shard.ID, contextpalace.WithContent(newContent))
	if err != nil {
		return fmt.Errorf("saving checkpoint: %w", err)
	}

	// Reload to show updated content
	shard, err = cpClient.GetShard(ctx, shard.ID)
	if err != nil {
		return fmt.Errorf("reloading session: %w", err)
	}

	return outputSession(cmd, cfg, shard, "Checkpoint saved")
}

// newSessionResumeCommand creates the 'session resume' subcommand.
func newSessionResumeCommand(deps *SessionCommandDeps) *cobra.Command {
	var lastClosed bool

	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume the current session (load checkpoints)",
		Long: `Display the current session's state and checkpoints.

This shows all checkpoint messages logged during the session,
allowing you to resume work from where you left off.

Examples:
  penf session resume
  penf session resume -o json
  penf session resume --last-closed`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionResume(cmd, deps, lastClosed)
		},
	}

	cmd.Flags().StringVarP(&sessionOutputFormat, "output", "o", "", "Output format: text, json, yaml")
	cmd.Flags().BoolVar(&lastClosed, "last-closed", false, "Resume the most recently closed session instead of active")

	return cmd
}

// runSessionResume loads the current session or last closed session.
func runSessionResume(cmd *cobra.Command, deps *SessionCommandDeps, lastClosed bool) error {
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

	var shard *contextpalace.Shard
	if lastClosed {
		shard, err = getLastClosedSession(ctx, cpClient, cfg.ContextPalace.GetAgent())
		if err != nil {
			return err
		}
	} else {
		shard, err = getActiveSession(ctx, cpClient, cfg.ContextPalace.GetAgent())
		if err != nil {
			return err
		}
	}

	message := "Session resumed"
	if lastClosed {
		message = "Last closed session"
	}
	return outputSession(cmd, cfg, shard, message)
}

// newSessionEndCommand creates the 'session end' subcommand.
func newSessionEndCommand(deps *SessionCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "end <summary>",
		Short: "End the current session",
		Long: `Close the current session with a summary.

This sets the session status to "closed" and records the closing summary
as the resolution.

Examples:
  penf session end "Completed feature implementation"
  penf session end "Work interrupted, resuming later"
  penf session end -o json "Session summary"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionEnd(cmd, deps, args[0])
		},
	}

	cmd.Flags().StringVarP(&sessionOutputFormat, "output", "o", "", "Output format: text, json, yaml")

	return cmd
}

// runSessionEnd closes the current session.
func runSessionEnd(cmd *cobra.Command, deps *SessionCommandDeps, summary string) error {
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

	shard, err := getActiveSession(ctx, cpClient, cfg.ContextPalace.GetAgent())
	if err != nil {
		return err
	}

	// Close the session
	err = cpClient.CloseShard(ctx, shard.ID, summary)
	if err != nil {
		return fmt.Errorf("closing session: %w", err)
	}

	// Reload to show closed state
	shard, err = cpClient.GetShard(ctx, shard.ID)
	if err != nil {
		return fmt.Errorf("reloading session: %w", err)
	}

	return outputSession(cmd, cfg, shard, "Session ended")
}

// newSessionContextCommand creates the 'session context' subcommand.
func newSessionContextCommand(deps *SessionCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Show current session state",
		Long: `Display the current active session and its state.

Shows session ID, title, checkpoints, and metadata.

Examples:
  penf session context
  penf session context -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionContext(cmd, deps)
		},
	}

	cmd.Flags().StringVarP(&sessionOutputFormat, "output", "o", "", "Output format: text, json, yaml")

	return cmd
}

// runSessionContext shows the current session.
func runSessionContext(cmd *cobra.Command, deps *SessionCommandDeps) error {
	// This is the same as resume (always active session)
	return runSessionResume(cmd, deps, false)
}

// newSessionHistoryCommand creates the 'session history' subcommand.
func newSessionHistoryCommand(deps *SessionCommandDeps) *cobra.Command {
	var limit int
	var allAgents bool

	cmd := &cobra.Command{
		Use:   "history",
		Short: "List recent sessions",
		Long: `Display recent work sessions.

Shows all sessions (both active and closed) for this agent, or
all agents if --all is specified.

Examples:
  penf session history              # Show last 10 sessions
  penf session history -n 20        # Show last 20 sessions
  penf session history --all        # Show sessions from all agents
  penf session history -o json      # JSON output`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionHistory(cmd, deps, limit, allAgents)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "Number of sessions to show")
	cmd.Flags().BoolVar(&allAgents, "all", false, "Show sessions from all agents")
	cmd.Flags().StringVarP(&sessionOutputFormat, "output", "o", "", "Output format: text, json, yaml")

	return cmd
}

// runSessionHistory lists past sessions.
func runSessionHistory(cmd *cobra.Command, deps *SessionCommandDeps, limit int, allAgents bool) error {
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

	opts := contextpalace.ListShardsOptions{
		Type:  "session",
		Limit: limit,
	}

	if !allAgents {
		opts.Owner = cfg.ContextPalace.GetAgent()
	}

	shards, err := cpClient.ListShards(ctx, opts)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	return outputSessionHistory(cmd, cfg, shards)
}

// Helper functions

// getActiveSession finds the active session for an agent.
func getActiveSession(ctx context.Context, client *contextpalace.Client, agent string) (*contextpalace.Shard, error) {
	sessions, err := client.ListShards(ctx, contextpalace.ListShardsOptions{
		Type:   "session",
		Status: "in_progress",
		Owner:  agent,
		Limit:  1,
	})
	if err != nil {
		return nil, fmt.Errorf("finding active session: %w", err)
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no active session found (start one with 'penf session start')")
	}

	return &sessions[0], nil
}

// getLastClosedSession finds the most recently closed session for an agent.
func getLastClosedSession(ctx context.Context, client *contextpalace.Client, agent string) (*contextpalace.Shard, error) {
	sessions, err := client.ListShards(ctx, contextpalace.ListShardsOptions{
		Type:   "session",
		Status: "closed",
		Owner:  agent,
		Limit:  1,
	})
	if err != nil {
		return nil, fmt.Errorf("finding closed sessions: %w", err)
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no previous sessions found")
	}

	return &sessions[0], nil
}

// outputSession outputs a single session.
func outputSession(cmd *cobra.Command, cfg *config.CLIConfig, shard *contextpalace.Shard, message string) error {
	format := cfg.OutputFormat
	if sessionOutputFormat != "" {
		format = config.OutputFormat(sessionOutputFormat)
	}

	switch format {
	case config.OutputFormatJSON:
		return outputSessionJSON(shard)
	case config.OutputFormatYAML:
		return outputSessionYAML(shard)
	default:
		return outputSessionText(shard, message)
	}
}

// outputSessionJSON outputs session as JSON.
func outputSessionJSON(shard *contextpalace.Shard) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(shard)
}

// outputSessionYAML outputs session as YAML.
func outputSessionYAML(shard *contextpalace.Shard) error {
	enc := yaml.NewEncoder(os.Stdout)
	return enc.Encode(shard)
}

// outputSessionText outputs session in human-readable format.
func outputSessionText(shard *contextpalace.Shard, message string) error {
	if message != "" {
		fmt.Printf("%s\n\n", message)
	}

	fmt.Printf("Session: %s\n", shard.ID)
	fmt.Printf("Title:   %s\n", shard.Title)
	fmt.Printf("Status:  %s\n", shard.Status)
	fmt.Printf("Owner:   %s\n", shard.Owner)
	fmt.Printf("Created: %s\n", shard.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated: %s\n", shard.UpdatedAt.Format(time.RFC3339))

	if shard.ClosedAt != nil {
		fmt.Printf("Closed:  %s\n", shard.ClosedAt.Format(time.RFC3339))
	}

	if shard.Resolution != "" {
		fmt.Printf("Summary: %s\n", shard.Resolution)
	}

	fmt.Println()
	fmt.Println("Content:")
	fmt.Println(strings.TrimSpace(shard.Content))

	return nil
}

// outputSessionHistory outputs session list.
func outputSessionHistory(cmd *cobra.Command, cfg *config.CLIConfig, shards []contextpalace.Shard) error {
	format := cfg.OutputFormat
	if sessionOutputFormat != "" {
		format = config.OutputFormat(sessionOutputFormat)
	}

	switch format {
	case config.OutputFormatJSON:
		return outputSessionHistoryJSON(shards)
	case config.OutputFormatYAML:
		return outputSessionHistoryYAML(shards)
	default:
		return outputSessionHistoryText(shards)
	}
}

// outputSessionHistoryJSON outputs session list as JSON.
func outputSessionHistoryJSON(shards []contextpalace.Shard) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(shards)
}

// outputSessionHistoryYAML outputs session list as YAML.
func outputSessionHistoryYAML(shards []contextpalace.Shard) error {
	enc := yaml.NewEncoder(os.Stdout)
	return enc.Encode(shards)
}

// outputSessionHistoryText outputs session list in human-readable format.
func outputSessionHistoryText(shards []contextpalace.Shard) error {
	if len(shards) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	fmt.Printf("Recent sessions (%d):\n\n", len(shards))
	fmt.Println("  ID         TITLE                          STATUS        OWNER             CREATED")
	fmt.Println("  --         -----                          ------        -----             -------")

	for _, s := range shards {
		title := s.Title
		if len(title) > 30 {
			title = title[:27] + "..."
		}

		statusColor := getSessionStatusColor(s.Status)
		created := s.CreatedAt.Format("2006-01-02 15:04")

		fmt.Printf("  %-10s %-30s %s%-13s\033[0m %-17s %s\n",
			s.ID, title, statusColor, s.Status, s.Owner, created)
	}

	fmt.Println()
	return nil
}

// getSessionStatusColor returns ANSI color code for session status.
func getSessionStatusColor(status string) string {
	switch status {
	case "in_progress":
		return "\033[34m" // Blue
	case "closed":
		return "\033[32m" // Green
	case "open":
		return "\033[33m" // Yellow
	default:
		return ""
	}
}
