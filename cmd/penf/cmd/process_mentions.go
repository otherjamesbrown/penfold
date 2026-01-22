// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	mentionsv1 "github.com/otherjamesbrown/penfold/api/proto/mentions/v1"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// Mention process command flags.
var (
	mentionProcessOutput     string
	mentionProcessLimit      int
	mentionProcessStatus     string
	mentionProcessDryRun     bool
	mentionIncludeCandidates bool
	mentionPatternsLimit     int
)

// newProcessMentionsCommand creates the 'process mentions' subcommand group.
func newProcessMentionsCommand(deps *ProcessCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mentions",
		Short: "Process content mentions intelligently",
		Long: `Process mentions found in content that need resolution to entities.

This workflow allows Claude to:
1. Get all pending mentions with their context and candidates
2. Access existing patterns for automatic resolution
3. Batch resolve multiple mentions efficiently
4. Create new patterns for recurring mentions

See specs/013-content-enrichment/mention-resolution.md for detailed guidance.`,
		Aliases: []string{"mention"},
	}

	cmd.AddCommand(newMentionsContextCommand(deps))
	cmd.AddCommand(newMentionsBatchResolveCommand(deps))

	return cmd
}

// newMentionsContextCommand creates the 'process mentions context' subcommand.
func newMentionsContextCommand(deps *ProcessCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Get full context for mention processing",
		Long: `Get complete context needed for intelligent mention processing.

Returns:
- All pending mentions with context snippets
- Candidate entities for each mention
- Existing patterns (for auto-resolution)
- Queue statistics
- Workflow guidance (available actions, decision criteria)

This single command provides everything Claude needs to:
1. Match mentions to existing patterns (auto-resolve)
2. Find high-confidence candidates
3. Only ask the user about truly ambiguous items
4. Create new patterns for recurring mentions

Examples:
  penf process mentions context
  penf process mentions context --output json
  penf process mentions context --status pending --limit 50`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMentionsContext(cmd.Context(), deps)
		},
	}

	cmd.Flags().StringVarP(&mentionProcessOutput, "output", "o", "json", "Output format: json, text")
	cmd.Flags().IntVar(&mentionProcessLimit, "limit", 100, "Maximum mentions to return")
	cmd.Flags().StringVar(&mentionProcessStatus, "status", "pending", "Filter by status: pending, resolved, all")
	cmd.Flags().BoolVar(&mentionIncludeCandidates, "include-candidates", true, "Include candidate entities for each mention")
	cmd.Flags().IntVar(&mentionPatternsLimit, "patterns-limit", 500, "Maximum patterns to return")

	return cmd
}

// newMentionsBatchResolveCommand creates the 'process mentions batch-resolve' subcommand.
func newMentionsBatchResolveCommand(deps *ProcessCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch-resolve <json>",
		Short: "Batch resolve mentions to entities",
		Long: `Batch resolve multiple mentions in a single operation.

Accepts JSON with resolutions, patterns, and dismissals:
{
  "resolutions": [
    {"mention_id": 123, "entity_id": 456, "entity_type": "ENTITY_TYPE_PERSON", "create_pattern": true},
    {"mention_id": 789, "entity_id": 101, "entity_type": "ENTITY_TYPE_TERM"}
  ],
  "new_patterns": [
    {"mention_text": "JB", "entity_id": 456, "entity_type": "ENTITY_TYPE_PERSON"}
  ],
  "dismissals": [
    {"mention_id": 202, "reason": "Not a named entity"}
  ]
}

Entity types: ENTITY_TYPE_PERSON, ENTITY_TYPE_TERM, ENTITY_TYPE_PRODUCT, ENTITY_TYPE_COMPANY, ENTITY_TYPE_PROJECT

Use --dry-run to preview changes without executing them.

Example:
  penf process mentions batch-resolve '{"resolutions":[{"mention_id":24,"entity_id":5,"entity_type":"ENTITY_TYPE_PERSON"}]}'
  penf process mentions batch-resolve --dry-run '{"resolutions":[...]}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMentionsBatchResolve(cmd.Context(), deps, args[0])
		},
	}

	cmd.Flags().BoolVar(&mentionProcessDryRun, "dry-run", false, "Preview changes without executing them")

	return cmd
}

// runMentionsContext executes the context command.
func runMentionsContext(ctx context.Context, deps *ProcessCommandDeps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	conn, err := connectToMentionsGateway(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := mentionsv1.NewMentionsServiceClient(conn)

	// Build request
	req := &mentionsv1.GetMentionContextRequest{
		Limit:             int32(mentionProcessLimit),
		IncludeCandidates: mentionIncludeCandidates,
		PatternsLimit:     int32(mentionPatternsLimit),
	}

	// Set status filter
	switch mentionProcessStatus {
	case "pending":
		req.Status = mentionsv1.MentionStatus_MENTION_STATUS_PENDING
	case "resolved":
		req.Status = mentionsv1.MentionStatus_MENTION_STATUS_USER_RESOLVED
	case "all":
		req.Status = mentionsv1.MentionStatus_MENTION_STATUS_UNSPECIFIED
	default:
		req.Status = mentionsv1.MentionStatus_MENTION_STATUS_PENDING
	}

	resp, err := client.GetMentionContext(ctx, req)
	if err != nil {
		return fmt.Errorf("getting mention context: %w", err)
	}

	// Output
	return outputMentionContextResponse(mentionProcessOutput, resp)
}

// runMentionsBatchResolve executes the batch-resolve command.
func runMentionsBatchResolve(ctx context.Context, deps *ProcessCommandDeps, jsonInput string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	// Parse input JSON
	var req mentionsv1.BatchResolveMentionsRequest
	if err := json.Unmarshal([]byte(jsonInput), &req); err != nil {
		return fmt.Errorf("parsing JSON input: %w", err)
	}

	// Dry-run mode: preview changes without executing
	if mentionProcessDryRun {
		fmt.Println("\033[1m=== DRY RUN - No changes will be made ===\033[0m")
		fmt.Println()

		if len(req.Resolutions) > 0 {
			fmt.Printf("Would resolve %d mentions:\n", len(req.Resolutions))
			for _, r := range req.Resolutions {
				pattern := ""
				if r.CreatePattern {
					pattern = " (+ pattern)"
				}
				fmt.Printf("  \033[32m#%d → %s:%d%s\033[0m\n", r.MentionId, r.EntityType.String(), r.EntityId, pattern)
			}
			fmt.Println()
		}

		if len(req.NewPatterns) > 0 {
			fmt.Printf("Would create %d patterns:\n", len(req.NewPatterns))
			for _, p := range req.NewPatterns {
				fmt.Printf("  \033[34m\"%s\" → %s:%d\033[0m\n", p.MentionText, p.EntityType.String(), p.EntityId)
			}
			fmt.Println()
		}

		if len(req.Dismissals) > 0 {
			fmt.Printf("Would dismiss %d mentions:\n", len(req.Dismissals))
			for _, d := range req.Dismissals {
				fmt.Printf("  \033[33m#%d:\033[0m %s\n", d.MentionId, d.Reason)
			}
			fmt.Println()
		}

		fmt.Printf("Summary: %d resolutions, %d patterns, %d dismissals\n",
			len(req.Resolutions), len(req.NewPatterns), len(req.Dismissals))
		fmt.Println("\n\033[2mRun without --dry-run to apply these changes.\033[0m")
		return nil
	}

	conn, err := connectToMentionsGateway(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := mentionsv1.NewMentionsServiceClient(conn)

	resp, err := client.BatchResolveMentions(ctx, &req)
	if err != nil {
		return fmt.Errorf("batch resolving mentions: %w", err)
	}

	// Output results
	fmt.Println()
	fmt.Printf("Batch complete: %d resolved, %d patterns, %d dismissed",
		resp.Resolved, resp.PatternsCreated, resp.Dismissed)
	if len(resp.Errors) > 0 {
		fmt.Printf(", %d errors\n", len(resp.Errors))
		for _, e := range resp.Errors {
			fmt.Printf("  \033[31mError:\033[0m %s\n", e)
		}
	} else {
		fmt.Println()
	}

	return nil
}

// connectToMentionsGateway creates a gRPC connection to the gateway.
func connectToMentionsGateway(cfg *config.CLIConfig) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	conn, err := grpc.DialContext(ctx, cfg.ServerAddress, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to gateway at %s: %w", cfg.ServerAddress, err)
	}

	return conn, nil
}

// outputMentionContextResponse outputs the context in the specified format.
func outputMentionContextResponse(format string, resp *mentionsv1.GetMentionContextResponse) error {
	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	case "text":
		return outputMentionContextText(resp)
	default:
		// Default to JSON for AI consumption
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}
}

// outputMentionContextText outputs context in human-readable format.
func outputMentionContextText(resp *mentionsv1.GetMentionContextResponse) error {
	fmt.Printf("Mention Resolution Context\n")
	fmt.Printf("==========================\n\n")

	if resp.Stats != nil {
		fmt.Printf("Pending Mentions: %d\n", resp.Stats.TotalPending)
		fmt.Printf("Resolved Today: %d\n", resp.Stats.ResolvedToday)
		fmt.Printf("Patterns: %d\n\n", resp.Stats.PatternsCount)
	}

	if len(resp.Mentions) > 0 {
		fmt.Println("Mentions:")
		for _, m := range resp.Mentions {
			fmt.Printf("  #%-4d [%s] \"%s\"\n", m.Id, m.Status.String(), m.MentionedText)
			if m.ContextSnippet != "" {
				fmt.Printf("         Context: \"%s\"\n", truncateMentionString(m.ContextSnippet, 60))
			}
			if len(m.Candidates) > 0 {
				fmt.Printf("         Candidates: ")
				for i, c := range m.Candidates {
					if i > 0 {
						fmt.Print(", ")
					}
					fmt.Printf("%s (%.0f%%)", c.EntityName, c.Score*100)
					if i >= 2 {
						fmt.Printf(" +%d more", len(m.Candidates)-3)
						break
					}
				}
				fmt.Println()
			}
		}
		fmt.Println()
	}

	if len(resp.Patterns) > 0 {
		fmt.Printf("Top Patterns (%d total):\n", len(resp.Patterns))
		shown := 0
		for _, p := range resp.Patterns {
			if shown >= 10 {
				break
			}
			fmt.Printf("  \"%s\" → %s:%d (used %dx)\n", p.PatternText, p.EntityType.String(), p.ResolvedEntityId, p.TimesLinked)
			shown++
		}
		fmt.Println()
	}

	if resp.Workflow != nil {
		fmt.Println("Batch Command:")
		fmt.Printf("  %s\n", resp.Workflow.BatchResolveCommand)
	}

	return nil
}

// truncateMentionString truncates a string to max length for mention output.
func truncateMentionString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
