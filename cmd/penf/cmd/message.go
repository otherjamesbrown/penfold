// Package cmd provides CLI command implementations.
package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/cmd/penf/contextpalace"
)

// MessageCommandDeps holds dependencies for the message command.
type MessageCommandDeps struct {
	Config     *config.CLIConfig
	LoadConfig func() (*config.CLIConfig, error)
}

// DefaultMessageDeps returns the default dependencies for the message command.
func DefaultMessageDeps() *MessageCommandDeps {
	return &MessageCommandDeps{
		LoadConfig: config.LoadConfig,
	}
}

// NewMessageCommand creates the message command group.
func NewMessageCommand(deps *MessageCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultMessageDeps()
	}

	cmd := &cobra.Command{
		Use:   "message",
		Short: "Manage messages in Context-Palace",
		Long: `Commands for sending and receiving messages in Context-Palace.

Messages are used for agent-to-agent communication. The CLI wraps the
send_message() function to ensure proper addressing and conventions.

Examples:
  # Check inbox
  penf message inbox

  # Send a message
  penf message send agent-penfold "Feature request" --body "Please add..."

  # Send with content from stdin
  echo "Message body" | penf message send agent-penfold "Subject"

  # View a message
  penf message show pf-abc123

  # Reply to a message
  penf message reply pf-abc123 --body "Acknowledged"`,
		Aliases: []string{"msg", "mail"},
	}

	cmd.AddCommand(newMessageInboxCommand(deps))
	cmd.AddCommand(newMessageSendCommand(deps))
	cmd.AddCommand(newMessageShowCommand(deps))
	cmd.AddCommand(newMessageReplyCommand(deps))
	cmd.AddCommand(newMessageMarkReadCommand(deps))

	return cmd
}

// Message command flags.
var (
	messageBody   string
	messageCC     []string
	messageKind   string
	messageAll    bool
	messageLimit  int
)

// newMessageInboxCommand creates the 'message inbox' subcommand.
func newMessageInboxCommand(deps *MessageCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "Show unread messages",
		Long: `Show unread messages addressed to the current agent.

Examples:
  # Show unread messages
  penf message inbox

  # Show summary only
  penf message inbox --summary`,
		Aliases: []string{"ls", "list"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMessageInbox(cmd, deps)
		},
	}

	cmd.Flags().IntVarP(&messageLimit, "limit", "n", 50, "Maximum number of messages to show")

	return cmd
}

// runMessageInbox shows unread messages.
func runMessageInbox(cmd *cobra.Command, deps *MessageCommandDeps) error {
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

	// Get inbox summary first
	summary, err := cpClient.GetInboxSummary(ctx)
	if err != nil {
		return fmt.Errorf("getting inbox summary: %w", err)
	}

	// Get unread messages
	messages, err := cpClient.GetUnread(ctx)
	if err != nil {
		return fmt.Errorf("getting unread messages: %w", err)
	}

	// Output based on format
	return outputInbox(cmd, cfg, summary, messages)
}

// newMessageSendCommand creates the 'message send' subcommand.
func newMessageSendCommand(deps *MessageCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send <recipient> <subject>",
		Short: "Send a message",
		Long: `Send a message to another agent.

The recipient can be specified as:
  - Full name: agent-mycroft
  - Shorthand: mycroft (auto-expanded to agent-mycroft)

Message body can be provided via:
  - --body flag
  - stdin (if --body not provided)

Examples:
  # Send with --body flag
  penf message send mycroft "Feature request" --body "Please add feature X"

  # Send with stdin
  echo "Message body" | penf message send mycroft "Subject"

  # Send to multiple recipients with CC
  penf message send mycroft "Update" --body "FYI" --cc penfold --cc cxp

  # Send with kind label
  penf message send mycroft "Bug report" --body "Found a bug" --kind bug`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMessageSend(cmd, deps, args[0], args[1])
		},
	}

	cmd.Flags().StringVar(&messageBody, "body", "", "Message body (reads from stdin if not provided)")
	cmd.Flags().StringSliceVar(&messageCC, "cc", nil, "CC recipients")
	cmd.Flags().StringVar(&messageKind, "kind", "", "Message kind label (e.g., bug, feature-request)")

	return cmd
}

// runMessageSend sends a message.
func runMessageSend(cmd *cobra.Command, deps *MessageCommandDeps, recipient, subject string) error {
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

	// Get body from flag or stdin
	body := messageBody
	if body == "" {
		// Check if stdin has data
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Stdin has data
			reader := bufio.NewReader(os.Stdin)
			data, err := io.ReadAll(reader)
			if err != nil {
				return fmt.Errorf("reading from stdin: %w", err)
			}
			body = strings.TrimSpace(string(data))
		}
	}

	if body == "" {
		return fmt.Errorf("message body required (use --body or pipe to stdin)")
	}

	cpClient, err := contextpalace.NewClient(cfg.ContextPalace)
	if err != nil {
		return fmt.Errorf("connecting to Context-Palace: %w", err)
	}
	defer cpClient.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()

	// Send the message
	opts := &contextpalace.SendMessageOptions{
		CC:   messageCC,
		Kind: messageKind,
	}

	messageID, err := cpClient.SendMessage(ctx, []string{recipient}, subject, body, opts)
	if err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	fmt.Printf("\033[32mMessage sent:\033[0m %s\n", messageID)
	fmt.Printf("  To:      %s\n", recipient)
	fmt.Printf("  Subject: %s\n", subject)
	if len(messageCC) > 0 {
		fmt.Printf("  CC:      %s\n", strings.Join(messageCC, ", "))
	}

	return nil
}

// newMessageShowCommand creates the 'message show' subcommand.
func newMessageShowCommand(deps *MessageCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <message-id>",
		Short: "Show a message",
		Long: `Show the full content of a message.

By default, viewing a message marks it as read.

Examples:
  penf message show pf-abc123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMessageShow(cmd, deps, args[0])
		},
	}

	return cmd
}

// runMessageShow shows a message and marks it as read.
func runMessageShow(cmd *cobra.Command, deps *MessageCommandDeps, messageID string) error {
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

	// Get the shard
	shard, err := cpClient.GetShard(ctx, messageID)
	if err != nil {
		return fmt.Errorf("getting message: %w", err)
	}

	// Mark as read
	if _, err := cpClient.MarkRead(ctx, []string{messageID}); err != nil {
		// Non-fatal - continue showing the message
		fmt.Fprintf(os.Stderr, "Warning: could not mark as read: %v\n", err)
	}

	// Output the message
	return outputMessage(cmd, cfg, shard)
}

// newMessageReplyCommand creates the 'message reply' subcommand.
func newMessageReplyCommand(deps *MessageCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reply <message-id>",
		Short: "Reply to a message",
		Long: `Reply to a message.

Creates a reply with proper reply-to edges and auto-generates
a "Re:" subject line.

Examples:
  penf message reply pf-abc123 --body "Thanks for the update"

  # Reply with stdin
  echo "Got it, will fix" | penf message reply pf-abc123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMessageReply(cmd, deps, args[0])
		},
	}

	cmd.Flags().StringVar(&messageBody, "body", "", "Reply body (reads from stdin if not provided)")
	cmd.Flags().StringSliceVar(&messageCC, "cc", nil, "CC recipients")

	return cmd
}

// runMessageReply replies to a message.
func runMessageReply(cmd *cobra.Command, deps *MessageCommandDeps, messageID string) error {
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

	// Get body from flag or stdin
	body := messageBody
	if body == "" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			reader := bufio.NewReader(os.Stdin)
			data, err := io.ReadAll(reader)
			if err != nil {
				return fmt.Errorf("reading from stdin: %w", err)
			}
			body = strings.TrimSpace(string(data))
		}
	}

	if body == "" {
		return fmt.Errorf("reply body required (use --body or pipe to stdin)")
	}

	cpClient, err := contextpalace.NewClient(cfg.ContextPalace)
	if err != nil {
		return fmt.Errorf("connecting to Context-Palace: %w", err)
	}
	defer cpClient.Close()

	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()

	// Get the original message to find the sender and subject
	original, err := cpClient.GetShard(ctx, messageID)
	if err != nil {
		return fmt.Errorf("getting original message: %w", err)
	}

	// Determine reply recipient (original sender)
	recipient := original.Creator

	// Generate reply subject
	subject := original.Title
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}

	// Send the reply
	opts := &contextpalace.SendMessageOptions{
		CC:      messageCC,
		ReplyTo: messageID,
	}

	replyID, err := cpClient.SendMessage(ctx, []string{recipient}, subject, body, opts)
	if err != nil {
		return fmt.Errorf("sending reply: %w", err)
	}

	fmt.Printf("\033[32mReply sent:\033[0m %s\n", replyID)
	fmt.Printf("  To:      %s\n", recipient)
	fmt.Printf("  Subject: %s\n", subject)
	fmt.Printf("  ReplyTo: %s\n", messageID)

	return nil
}

// newMessageMarkReadCommand creates the 'message mark-read' subcommand.
func newMessageMarkReadCommand(deps *MessageCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mark-read <message-id>...",
		Short: "Mark messages as read",
		Long: `Mark one or more messages as read.

Examples:
  # Mark a single message
  penf message mark-read pf-abc123

  # Mark multiple messages
  penf message mark-read pf-abc123 pf-def456

  # Mark all unread messages
  penf message mark-read --all`,
		Aliases: []string{"read"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMessageMarkRead(cmd, deps, args)
		},
	}

	cmd.Flags().BoolVar(&messageAll, "all", false, "Mark all unread messages as read")

	return cmd
}

// runMessageMarkRead marks messages as read.
func runMessageMarkRead(cmd *cobra.Command, deps *MessageCommandDeps, messageIDs []string) error {
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

	// If --all flag, get all unread messages
	if messageAll {
		messages, err := cpClient.GetUnread(ctx)
		if err != nil {
			return fmt.Errorf("getting unread messages: %w", err)
		}
		messageIDs = make([]string, len(messages))
		for i, m := range messages {
			messageIDs[i] = m.ID
		}
	}

	if len(messageIDs) == 0 {
		fmt.Println("No messages to mark as read.")
		return nil
	}

	count, err := cpClient.MarkRead(ctx, messageIDs)
	if err != nil {
		return fmt.Errorf("marking messages as read: %w", err)
	}

	fmt.Printf("\033[32mMarked %d message(s) as read.\033[0m\n", count)

	return nil
}

// outputInbox outputs inbox data in the configured format.
func outputInbox(cmd *cobra.Command, cfg *config.CLIConfig, summary *contextpalace.InboxSummary, messages []contextpalace.Message) error {
	format := cfg.OutputFormat
	if f := cmd.Flag("output"); f != nil && f.Changed {
		format = config.OutputFormat(f.Value.String())
	}

	type inboxOutput struct {
		Summary  *contextpalace.InboxSummary `json:"summary" yaml:"summary"`
		Messages []contextpalace.Message     `json:"messages" yaml:"messages"`
	}

	data := inboxOutput{
		Summary:  summary,
		Messages: messages,
	}

	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(data)
	default:
		return outputInboxText(summary, messages)
	}
}

// outputInboxText outputs inbox in human-readable format.
func outputInboxText(summary *contextpalace.InboxSummary, messages []contextpalace.Message) error {
	// Print summary
	fmt.Printf("Inbox: %d unread", summary.TotalUnread)
	if summary.UrgentCount > 0 {
		fmt.Printf(" (\033[31m%d urgent\033[0m)", summary.UrgentCount)
	}
	fmt.Println()

	if summary.OldestUnread != nil {
		fmt.Printf("Oldest: %s\n", summary.OldestUnread.Local().Format("2006-01-02 15:04"))
	}

	if len(summary.ByKind) > 0 {
		fmt.Print("By kind: ")
		parts := make([]string, 0, len(summary.ByKind))
		for k, v := range summary.ByKind {
			parts = append(parts, fmt.Sprintf("%s(%d)", k, v))
		}
		fmt.Println(strings.Join(parts, ", "))
	}

	fmt.Println()

	if len(messages) == 0 {
		fmt.Println("No unread messages.")
		return nil
	}

	fmt.Println("Messages:")
	for _, m := range messages {
		created := m.CreatedAt.Local().Format("Jan 02 15:04")
		kind := ""
		if m.Kind != "" {
			kind = fmt.Sprintf(" [%s]", m.Kind)
		}
		fmt.Printf("  \033[36m%s\033[0m  %s  %s%s\n", m.ID, created, m.Creator, kind)
		fmt.Printf("    %s\n", m.Title)
	}

	return nil
}

// outputMessage outputs a single message in the configured format.
func outputMessage(cmd *cobra.Command, cfg *config.CLIConfig, shard *contextpalace.Shard) error {
	format := cfg.OutputFormat
	if f := cmd.Flag("output"); f != nil && f.Changed {
		format = config.OutputFormat(f.Value.String())
	}

	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(shard)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(shard)
	default:
		return outputMessageText(shard)
	}
}

// outputMessageText outputs a message in human-readable format.
func outputMessageText(shard *contextpalace.Shard) error {
	fmt.Printf("\033[1m%s\033[0m\n", shard.Title)
	fmt.Printf("ID:      %s\n", shard.ID)
	fmt.Printf("From:    %s\n", shard.Creator)
	fmt.Printf("Date:    %s\n", shard.CreatedAt.Local().Format(time.RFC1123))

	if len(shard.Labels) > 0 {
		fmt.Printf("Labels:  %s\n", strings.Join(shard.Labels, ", "))
	}

	fmt.Println()
	fmt.Println(shard.Content)

	return nil
}
