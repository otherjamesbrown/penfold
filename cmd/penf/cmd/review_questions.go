// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/otherjamesbrown/penfold/pkg/reviewqueue"
)

// Review questions command flags
var (
	questionsFormat   string
	questionsType     string
	questionsPriority string
	questionsLimit    int
)

// newReviewQuestionsCommand creates the 'review questions' subcommand group.
func newReviewQuestionsCommand(deps *ReviewCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "questions",
		Short: "Manage AI questions requiring human review",
		Long: `Manage the queue of questions AI has for you.

During content processing, AI may encounter things it needs clarification on:
  - Unknown acronyms that need definitions
  - Ambiguous person references that need disambiguation
  - Entities that need confirmation

Questions are prioritized:
  - high:   Blocks understanding or processing
  - medium: Would improve search/correlation
  - low:    Nice to have, cosmetic improvement

Examples:
  penf review questions list              # Show pending questions
  penf review questions list --priority high
  penf review questions next              # Get next question to answer
  penf review questions resolve 123 "..."  # Answer a question
  penf review questions dismiss 123       # Dismiss a question`,
		Aliases: []string{"q", "ask"},
	}

	// Add persistent flags
	cmd.PersistentFlags().StringVarP(&questionsFormat, "format", "f", "", "Output format: table, json, yaml")
	cmd.PersistentFlags().IntVarP(&questionsLimit, "limit", "l", 20, "Maximum number of results")

	// Add subcommands
	cmd.AddCommand(newQuestionsListCommand(deps))
	cmd.AddCommand(newQuestionsNextCommand(deps))
	cmd.AddCommand(newQuestionsShowCommand(deps))
	cmd.AddCommand(newQuestionsResolveCommand(deps))
	cmd.AddCommand(newQuestionsDismissCommand(deps))
	cmd.AddCommand(newQuestionsDeferCommand(deps))
	cmd.AddCommand(newQuestionsStatsCommand(deps))

	return cmd
}

// newQuestionsListCommand creates the 'review questions list' subcommand.
func newQuestionsListCommand(deps *ReviewCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pending questions",
		Long: `List pending questions from AI that need human review.

Examples:
  penf review questions list
  penf review questions list --priority high
  penf review questions list --type acronym`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuestionsList(cmd.Context(), deps)
		},
	}

	cmd.Flags().StringVarP(&questionsPriority, "priority", "p", "", "Filter by priority: high, medium, low")
	cmd.Flags().StringVarP(&questionsType, "type", "t", "", "Filter by type: acronym, person, entity, duplicate")

	return cmd
}

// newQuestionsNextCommand creates the 'review questions next' subcommand.
func newQuestionsNextCommand(deps *ReviewCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "next",
		Short: "Show the next question to review",
		Long: `Show the next highest-priority question to review.

Returns the most urgent pending question. Use --type to filter by question type.

Example:
  penf review questions next
  penf review questions next --type acronym`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuestionsNext(cmd.Context(), deps)
		},
	}

	cmd.Flags().StringVarP(&questionsType, "type", "t", "", "Filter by type: acronym, person, entity")

	return cmd
}

// newQuestionsShowCommand creates the 'review questions show' subcommand.
func newQuestionsShowCommand(deps *ReviewCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show details of a question",
		Long: `Show detailed information about a specific question.

Example:
  penf review questions show 123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid question ID: %s", args[0])
			}
			return runQuestionsShow(cmd.Context(), deps, id)
		},
	}
}

// newQuestionsResolveCommand creates the 'review questions resolve' subcommand.
func newQuestionsResolveCommand(deps *ReviewCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <id> <answer>",
		Short: "Resolve a question with an answer",
		Long: `Resolve a question by providing an answer.

For acronym questions, the answer is added to the glossary automatically.

Examples:
  penf review questions resolve 123 "Technical Execution Review"
  penf review questions resolve 456 "This refers to Adam Weingarten"`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid question ID: %s", args[0])
			}
			answer := strings.Join(args[1:], " ")
			return runQuestionsResolve(cmd.Context(), deps, id, answer)
		},
	}

	return cmd
}

// newQuestionsDismissCommand creates the 'review questions dismiss' subcommand.
func newQuestionsDismissCommand(deps *ReviewCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "dismiss <id> [reason]",
		Short: "Dismiss a question as not needed",
		Long: `Dismiss a question without providing an answer.

Use this when the question isn't relevant or doesn't need an answer.

Example:
  penf review questions dismiss 123 "Not an acronym, just initials"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid question ID: %s", args[0])
			}
			reason := ""
			if len(args) > 1 {
				reason = strings.Join(args[1:], " ")
			}
			return runQuestionsDismiss(cmd.Context(), deps, id, reason)
		},
	}
}

// newQuestionsDeferCommand creates the 'review questions defer' subcommand.
func newQuestionsDeferCommand(deps *ReviewCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "defer <id>",
		Short: "Defer a question for later",
		Long: `Defer a question to answer later.

The question remains in the queue but moves to deferred status.

Example:
  penf review questions defer 123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid question ID: %s", args[0])
			}
			return runQuestionsDefer(cmd.Context(), deps, id)
		},
	}
}

// newQuestionsStatsCommand creates the 'review questions stats' subcommand.
func newQuestionsStatsCommand(deps *ReviewCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show queue statistics",
		Long: `Show statistics about the questions queue.

Example:
  penf review questions stats`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuestionsStats(cmd.Context(), deps)
		},
	}
}

// Command execution functions

func runQuestionsList(ctx context.Context, deps *ReviewCommandDeps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := reviewqueue.NewRepository(pool)

	filter := reviewqueue.ReviewFilter{
		Status: reviewqueue.StatusPending,
		Limit:  questionsLimit,
	}

	if questionsPriority != "" {
		filter.Priority = reviewqueue.Priority(questionsPriority)
	}
	if questionsType != "" {
		filter.QuestionType = reviewqueue.QuestionType(questionsType)
	}

	items, err := repo.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("listing questions: %w", err)
	}

	format := cfg.OutputFormat
	if questionsFormat != "" {
		format = config.OutputFormat(questionsFormat)
	}

	return outputQuestionsList(format, items)
}

func runQuestionsNext(ctx context.Context, deps *ReviewCommandDeps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := reviewqueue.NewRepository(pool)

	var qtype reviewqueue.QuestionType
	if questionsType != "" {
		qtype = reviewqueue.QuestionType(questionsType)
	}

	item, err := repo.GetNext(ctx, qtype)
	if err != nil {
		return fmt.Errorf("getting next question: %w", err)
	}
	if item == nil {
		fmt.Println("No pending questions.")
		return nil
	}

	format := cfg.OutputFormat
	if questionsFormat != "" {
		format = config.OutputFormat(questionsFormat)
	}

	return outputQuestionDetail(format, item)
}

func runQuestionsShow(ctx context.Context, deps *ReviewCommandDeps, id int64) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := reviewqueue.NewRepository(pool)

	item, err := repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("getting question: %w", err)
	}
	if item == nil {
		return fmt.Errorf("question not found: %d", id)
	}

	format := cfg.OutputFormat
	if questionsFormat != "" {
		format = config.OutputFormat(questionsFormat)
	}

	return outputQuestionDetail(format, item)
}

func runQuestionsResolve(ctx context.Context, deps *ReviewCommandDeps, id int64, answer string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := reviewqueue.NewRepository(pool)

	// Get the question first
	item, err := repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("getting question: %w", err)
	}
	if item == nil {
		return fmt.Errorf("question not found: %d", id)
	}

	// If it's an acronym question, add to glossary
	if item.QuestionType == reviewqueue.QuestionTypeAcronym && item.SuggestedTerm != "" {
		glossaryRepo := glossary.NewRepository(pool)

		expandInSearch := true
		input := glossary.TermInput{
			Term:           item.SuggestedTerm,
			Expansion:      answer,
			ExpandInSearch: &expandInSearch,
			Source:         "review_queue",
		}

		_, err := glossaryRepo.Create(ctx, input)
		if err != nil {
			fmt.Printf("Warning: Could not add to glossary: %v\n", err)
		} else {
			fmt.Printf("Added to glossary: %s = %s\n", item.SuggestedTerm, answer)
		}
	}

	// Resolve the question
	err = repo.Resolve(ctx, id, answer, "user")
	if err != nil {
		return fmt.Errorf("resolving question: %w", err)
	}

	fmt.Printf("\033[32mResolved question #%d\033[0m\n", id)
	return nil
}

func runQuestionsDismiss(ctx context.Context, deps *ReviewCommandDeps, id int64, reason string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := reviewqueue.NewRepository(pool)

	err = repo.Dismiss(ctx, id, reason, "user")
	if err != nil {
		return fmt.Errorf("dismissing question: %w", err)
	}

	fmt.Printf("\033[33mDismissed question #%d\033[0m\n", id)
	return nil
}

func runQuestionsDefer(ctx context.Context, deps *ReviewCommandDeps, id int64) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := reviewqueue.NewRepository(pool)

	err = repo.Defer(ctx, id)
	if err != nil {
		return fmt.Errorf("deferring question: %w", err)
	}

	fmt.Printf("Deferred question #%d\n", id)
	return nil
}

func runQuestionsStats(ctx context.Context, deps *ReviewCommandDeps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := reviewqueue.NewRepository(pool)

	stats, err := repo.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("getting stats: %w", err)
	}

	format := cfg.OutputFormat
	if questionsFormat != "" {
		format = config.OutputFormat(questionsFormat)
	}

	return outputQuestionsStats(format, stats)
}

// Output functions

func outputQuestionsList(format config.OutputFormat, items []*reviewqueue.ReviewItem) error {
	switch format {
	case config.OutputFormatJSON:
		return outputQuestionsJSON(items)
	case config.OutputFormatYAML:
		return outputQuestionsYAML(items)
	default:
		return outputQuestionsListText(items)
	}
}

func outputQuestionsListText(items []*reviewqueue.ReviewItem) error {
	if len(items) == 0 {
		fmt.Println("No pending questions.")
		return nil
	}

	fmt.Printf("Pending Questions (%d):\n\n", len(items))
	fmt.Println("  ID     PRI    TYPE      QUESTION")
	fmt.Println("  --     ---    ----      --------")

	for _, item := range items {
		priColor := getQuestionPriorityColor(item.Priority)
		question := truncateQuestion(item.Question, 50)
		fmt.Printf("  %-6d %s%-6s\033[0m %-9s %s\n",
			item.ID,
			priColor,
			item.Priority,
			item.QuestionType,
			question)
	}

	fmt.Println()
	fmt.Println("Use 'penf review questions show <id>' for details.")
	fmt.Println("Use 'penf review questions resolve <id> <answer>' to answer.")
	return nil
}

func outputQuestionDetail(format config.OutputFormat, item *reviewqueue.ReviewItem) error {
	switch format {
	case config.OutputFormatJSON:
		return outputQuestionsJSON(item)
	case config.OutputFormatYAML:
		return outputQuestionsYAML(item)
	default:
		return outputQuestionDetailText(item)
	}
}

func outputQuestionDetailText(item *reviewqueue.ReviewItem) error {
	priColor := getQuestionPriorityColor(item.Priority)

	fmt.Println("Question Details:")
	fmt.Println()
	fmt.Printf("  \033[1mID:\033[0m       %d\n", item.ID)
	fmt.Printf("  \033[1mType:\033[0m     %s\n", item.QuestionType)
	fmt.Printf("  \033[1mPriority:\033[0m %s%s\033[0m\n", priColor, item.Priority)
	fmt.Printf("  \033[1mStatus:\033[0m   %s\n", item.Status)
	fmt.Println()
	fmt.Printf("  \033[1mQuestion:\033[0m\n")
	fmt.Printf("    %s\n", item.Question)

	if item.Context != "" {
		fmt.Println()
		fmt.Printf("  \033[1mContext:\033[0m\n")
		fmt.Printf("    \"%s\"\n", item.Context)
	}

	if item.SuggestedTerm != "" {
		fmt.Println()
		fmt.Printf("  \033[1mTerm:\033[0m     %s\n", item.SuggestedTerm)
	}

	if item.SourceReference != "" {
		fmt.Println()
		fmt.Printf("  \033[1mSource:\033[0m   %s\n", item.SourceReference)
	}

	fmt.Println()
	fmt.Printf("  \033[1mCreated:\033[0m  %s\n", item.CreatedAt.Format("2006-01-02 15:04:05"))

	fmt.Println()
	fmt.Printf("To answer: penf review questions resolve %d \"<your answer>\"\n", item.ID)
	fmt.Printf("To dismiss: penf review questions dismiss %d\n", item.ID)

	return nil
}

func outputQuestionsStats(format config.OutputFormat, stats *reviewqueue.QueueStats) error {
	switch format {
	case config.OutputFormatJSON:
		return outputQuestionsJSON(stats)
	case config.OutputFormatYAML:
		return outputQuestionsYAML(stats)
	default:
		return outputQuestionsStatsText(stats)
	}
}

func outputQuestionsStatsText(stats *reviewqueue.QueueStats) error {
	fmt.Println("Questions Queue Statistics:")
	fmt.Println()
	fmt.Printf("  \033[1mTotal Pending:\033[0m  %d\n", stats.TotalPending)
	fmt.Println()

	if len(stats.ByPriority) > 0 {
		fmt.Println("  By Priority:")
		for _, p := range []string{"high", "medium", "low"} {
			if count, ok := stats.ByPriority[p]; ok && count > 0 {
				color := getQuestionPriorityColor(reviewqueue.Priority(p))
				fmt.Printf("    %s%-8s\033[0m %d\n", color, p, count)
			}
		}
		fmt.Println()
	}

	if len(stats.ByType) > 0 {
		fmt.Println("  By Type:")
		for qtype, count := range stats.ByType {
			fmt.Printf("    %-12s %d\n", qtype, count)
		}
		fmt.Println()
	}

	fmt.Printf("  \033[1mResolved Today:\033[0m %d\n", stats.ResolvedToday)

	if stats.OldestPending != nil {
		fmt.Printf("  \033[1mOldest Pending:\033[0m %s\n", stats.OldestPending.Format("2006-01-02 15:04"))
	}

	return nil
}

func outputQuestionsJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func outputQuestionsYAML(v interface{}) error {
	enc := yaml.NewEncoder(os.Stdout)
	return enc.Encode(v)
}

func getQuestionPriorityColor(p reviewqueue.Priority) string {
	switch p {
	case reviewqueue.PriorityHigh:
		return "\033[31m" // Red
	case reviewqueue.PriorityMedium:
		return "\033[33m" // Yellow
	case reviewqueue.PriorityLow:
		return "\033[32m" // Green
	default:
		return ""
	}
}

func truncateQuestion(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
