// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/pkg/glossary"
)

// Glossary command flags
var (
	glossaryFormat    string
	glossaryContext   []string
	glossaryLimit     int
	glossaryExpand    bool
	glossaryAliases   []string
	glossarySource    string
	glossaryNoExpand  bool
)

// GlossaryCommandDeps holds the dependencies for glossary commands.
type GlossaryCommandDeps struct {
	Config     *config.CLIConfig
	LoadConfig func() (*config.CLIConfig, error)
}

// DefaultGlossaryDeps returns the default dependencies for production use.
func DefaultGlossaryDeps() *GlossaryCommandDeps {
	return &GlossaryCommandDeps{
		LoadConfig: config.LoadConfig,
	}
}

// NewGlossaryCommand creates the root glossary command with all subcommands.
func NewGlossaryCommand(deps *GlossaryCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultGlossaryDeps()
	}

	cmd := &cobra.Command{
		Use:   "glossary",
		Short: "Manage domain terminology and acronyms for query expansion",
		Long: `Manage domain terminology and acronyms for search query expansion.

The glossary stores acronyms, abbreviations, and domain-specific terminology
along with their full expansions and definitions. This enables Penfold to
understand searches like "TER meeting" as "Technical Execution Review meeting".

Terms can have:
  - Expansion: The full form (e.g., "Technical Execution Review")
  - Definition: A longer explanation of what the term means
  - Context: Tags for categorization (e.g., "MTC", "meetings")
  - Aliases: Alternative forms (e.g., "T.E.R.", "ter")

Query expansion automatically expands known acronyms in search queries.`,
		Aliases: []string{"terms", "dict"},
	}

	// Add persistent flags
	cmd.PersistentFlags().StringVarP(&glossaryFormat, "format", "f", "", "Output format: table, json, yaml")
	cmd.PersistentFlags().IntVarP(&glossaryLimit, "limit", "l", 50, "Maximum number of results")

	// Add subcommands
	cmd.AddCommand(newGlossaryAddCommand(deps))
	cmd.AddCommand(newGlossaryListCommand(deps))
	cmd.AddCommand(newGlossaryShowCommand(deps))
	cmd.AddCommand(newGlossarySearchCommand(deps))
	cmd.AddCommand(newGlossaryRemoveCommand(deps))
	cmd.AddCommand(newGlossaryExpandCommand(deps))

	return cmd
}

// newGlossaryAddCommand creates the 'glossary add' subcommand.
func newGlossaryAddCommand(deps *GlossaryCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <term> <expansion>",
		Short: "Add a term to the glossary",
		Long: `Add a new term or acronym to the glossary.

The term is the abbreviation or acronym (e.g., "TER"), and the expansion
is its full form (e.g., "Technical Execution Review").

Examples:
  # Add a simple acronym
  penf glossary add TER "Technical Execution Review"

  # Add with definition and context
  penf glossary add DBaaS "Database as a Service" \
    --definition "Product team for managed DB platform" \
    --context MTC,Oracle

  # Add with aliases
  penf glossary add MTC "Major TikTok Contract" \
    --aliases "TikTok Project","TT Contract" \
    --context TikTok,Oracle`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			definition, _ := cmd.Flags().GetString("definition")
			return runGlossaryAdd(cmd.Context(), deps, args[0], args[1], definition)
		},
	}

	cmd.Flags().StringP("definition", "d", "", "Definition or description of the term")
	cmd.Flags().StringSliceVarP(&glossaryContext, "context", "c", nil, "Context tags (comma-separated)")
	cmd.Flags().StringSliceVarP(&glossaryAliases, "aliases", "a", nil, "Aliases (comma-separated)")
	cmd.Flags().BoolVar(&glossaryNoExpand, "no-expand", false, "Don't use this term for query expansion")

	return cmd
}

// newGlossaryListCommand creates the 'glossary list' subcommand.
func newGlossaryListCommand(deps *GlossaryCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all glossary terms",
		Long: `List all terms in the glossary.

Displays terms with their expansions and context tags.

Examples:
  # List all terms
  penf glossary list

  # Filter by context
  penf glossary list --context MTC

  # Output as JSON
  penf glossary list --format json`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGlossaryList(cmd.Context(), deps)
		},
	}

	cmd.Flags().StringSliceVarP(&glossaryContext, "context", "c", nil, "Filter by context tags")
	cmd.Flags().BoolVar(&glossaryExpand, "expand-only", false, "Only show terms used for query expansion")

	return cmd
}

// newGlossaryShowCommand creates the 'glossary show' subcommand.
func newGlossaryShowCommand(deps *GlossaryCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <term>",
		Short: "Show details of a term",
		Long: `Show detailed information about a glossary term.

Looks up the term by exact match or alias.

Example:
  penf glossary show TER`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGlossaryShow(cmd.Context(), deps, args[0])
		},
	}
}

// newGlossarySearchCommand creates the 'glossary search' subcommand.
func newGlossarySearchCommand(deps *GlossaryCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search glossary terms",
		Long: `Search glossary terms by full-text search.

Searches across term names, expansions, and definitions.

Example:
  penf glossary search "database"
  penf glossary search "TikTok"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			return runGlossarySearch(cmd.Context(), deps, query)
		},
	}
}

// newGlossaryRemoveCommand creates the 'glossary remove' subcommand.
func newGlossaryRemoveCommand(deps *GlossaryCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <term>",
		Short: "Remove a term from the glossary",
		Long: `Remove a term from the glossary by its term name.

Example:
  penf glossary remove TER`,
		Aliases: []string{"rm", "delete"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGlossaryRemove(cmd.Context(), deps, args[0])
		},
	}
}

// newGlossaryExpandCommand creates the 'glossary expand' subcommand.
func newGlossaryExpandCommand(deps *GlossaryCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "expand <query>",
		Short: "Show how a query would be expanded",
		Long: `Show how a search query would be expanded using glossary terms.

This is useful for debugging query expansion behavior.

Examples:
  penf glossary expand "TER meeting issues"
  penf glossary expand "DBaaS VPC configuration"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			return runGlossaryExpand(cmd.Context(), deps, query)
		},
	}
}

// Command execution functions

func runGlossaryAdd(ctx context.Context, deps *GlossaryCommandDeps, term, expansion, definition string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := glossary.NewRepository(pool)

	// Check if term already exists
	existing, err := repo.GetByTerm(ctx, term)
	if err != nil {
		return fmt.Errorf("checking existing term: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("term '%s' already exists (use 'penf glossary show %s' to view)", term, term)
	}

	expandInSearch := !glossaryNoExpand
	input := glossary.TermInput{
		Term:           term,
		Expansion:      expansion,
		Definition:     definition,
		Context:        glossaryContext,
		Aliases:        glossaryAliases,
		ExpandInSearch: &expandInSearch,
		Source:         "manual",
	}

	created, err := repo.Create(ctx, input)
	if err != nil {
		return fmt.Errorf("creating term: %w", err)
	}

	fmt.Printf("\033[32mAdded term:\033[0m %s\n", created.Term)
	fmt.Printf("  Expansion:  %s\n", created.Expansion)
	if created.Definition != "" {
		fmt.Printf("  Definition: %s\n", created.Definition)
	}
	if len(created.Context) > 0 {
		fmt.Printf("  Context:    %s\n", strings.Join(created.Context, ", "))
	}
	if len(created.Aliases) > 0 {
		fmt.Printf("  Aliases:    %s\n", strings.Join(created.Aliases, ", "))
	}
	fmt.Printf("  Expand:     %v\n", created.ExpandInSearch)

	return nil
}

func runGlossaryList(ctx context.Context, deps *GlossaryCommandDeps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := glossary.NewRepository(pool)

	filter := glossary.TermFilter{
		Context:    glossaryContext,
		ExpandOnly: glossaryExpand,
		Limit:      glossaryLimit,
	}

	terms, err := repo.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("listing terms: %w", err)
	}

	format := cfg.OutputFormat
	if glossaryFormat != "" {
		format = config.OutputFormat(glossaryFormat)
	}

	return outputGlossaryTerms(format, terms)
}

func runGlossaryShow(ctx context.Context, deps *GlossaryCommandDeps, termStr string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := glossary.NewRepository(pool)

	term, err := repo.LookupTerm(ctx, termStr)
	if err != nil {
		return fmt.Errorf("looking up term: %w", err)
	}
	if term == nil {
		return fmt.Errorf("term not found: %s", termStr)
	}

	format := cfg.OutputFormat
	if glossaryFormat != "" {
		format = config.OutputFormat(glossaryFormat)
	}

	return outputGlossaryTermDetail(format, term)
}

func runGlossarySearch(ctx context.Context, deps *GlossaryCommandDeps, query string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := glossary.NewRepository(pool)

	filter := glossary.TermFilter{
		Search: query,
		Limit:  glossaryLimit,
	}

	terms, err := repo.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("searching terms: %w", err)
	}

	format := cfg.OutputFormat
	if glossaryFormat != "" {
		format = config.OutputFormat(glossaryFormat)
	}

	return outputGlossaryTerms(format, terms)
}

func runGlossaryRemove(ctx context.Context, deps *GlossaryCommandDeps, termStr string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := glossary.NewRepository(pool)

	// First show what we're about to delete
	term, err := repo.GetByTerm(ctx, termStr)
	if err != nil {
		return fmt.Errorf("looking up term: %w", err)
	}
	if term == nil {
		return fmt.Errorf("term not found: %s", termStr)
	}

	err = repo.DeleteByTerm(ctx, termStr)
	if err != nil {
		return fmt.Errorf("deleting term: %w", err)
	}

	fmt.Printf("\033[32mRemoved term:\033[0m %s (%s)\n", term.Term, term.Expansion)
	return nil
}

func runGlossaryExpand(ctx context.Context, deps *GlossaryCommandDeps, query string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	repo := glossary.NewRepository(pool)

	expansion, err := repo.ExpandQuery(ctx, query)
	if err != nil {
		return fmt.Errorf("expanding query: %w", err)
	}

	format := cfg.OutputFormat
	if glossaryFormat != "" {
		format = config.OutputFormat(glossaryFormat)
	}

	return outputQueryExpansion(format, expansion)
}

// Output functions

func outputGlossaryTerms(format config.OutputFormat, terms []*glossary.Term) error {
	switch format {
	case config.OutputFormatJSON:
		return outputGlossaryJSON(terms)
	case config.OutputFormatYAML:
		return outputGlossaryYAML(terms)
	default:
		return outputGlossaryTermsText(terms)
	}
}

func outputGlossaryTermsText(terms []*glossary.Term) error {
	if len(terms) == 0 {
		fmt.Println("No terms found.")
		return nil
	}

	fmt.Printf("Glossary Terms (%d):\n\n", len(terms))
	fmt.Println("  TERM          EXPANSION                              CONTEXT")
	fmt.Println("  ----          ---------                              -------")

	for _, t := range terms {
		contextStr := strings.Join(t.Context, ", ")
		if len(contextStr) > 20 {
			contextStr = contextStr[:17] + "..."
		}
		fmt.Printf("  %-13s %-38s %s\n",
			truncateGlossary(t.Term, 13),
			truncateGlossary(t.Expansion, 38),
			contextStr)
	}

	fmt.Println()
	return nil
}

func outputGlossaryTermDetail(format config.OutputFormat, term *glossary.Term) error {
	switch format {
	case config.OutputFormatJSON:
		return outputGlossaryJSON(term)
	case config.OutputFormatYAML:
		return outputGlossaryYAML(term)
	default:
		return outputGlossaryTermDetailText(term)
	}
}

func outputGlossaryTermDetailText(term *glossary.Term) error {
	fmt.Println("Term Details:")
	fmt.Println()
	fmt.Printf("  \033[1mTerm:\033[0m         %s\n", term.Term)
	fmt.Printf("  \033[1mExpansion:\033[0m    %s\n", term.Expansion)
	if term.Definition != "" {
		fmt.Printf("  \033[1mDefinition:\033[0m   %s\n", term.Definition)
	}
	fmt.Println()
	if len(term.Context) > 0 {
		fmt.Printf("  \033[1mContext:\033[0m      %s\n", strings.Join(term.Context, ", "))
	}
	if len(term.Aliases) > 0 {
		fmt.Printf("  \033[1mAliases:\033[0m      %s\n", strings.Join(term.Aliases, ", "))
	}
	fmt.Printf("  \033[1mExpand:\033[0m       %v\n", term.ExpandInSearch)
	fmt.Printf("  \033[1mSource:\033[0m       %s\n", term.Source)
	fmt.Println()
	fmt.Printf("  \033[1mCreated:\033[0m      %s\n", term.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  \033[1mUpdated:\033[0m      %s\n", term.UpdatedAt.Format("2006-01-02 15:04:05"))

	return nil
}

func outputQueryExpansion(format config.OutputFormat, expansion *glossary.QueryExpansion) error {
	switch format {
	case config.OutputFormatJSON:
		return outputGlossaryJSON(expansion)
	case config.OutputFormatYAML:
		return outputGlossaryYAML(expansion)
	default:
		return outputQueryExpansionText(expansion)
	}
}

func outputQueryExpansionText(expansion *glossary.QueryExpansion) error {
	fmt.Println("Query Expansion:")
	fmt.Println()
	fmt.Printf("  \033[1mOriginal:\033[0m    %s\n", expansion.OriginalQuery)
	fmt.Println()

	if len(expansion.ExpandedTerms) == 0 {
		fmt.Println("  No terms matched for expansion.")
		fmt.Println()
		fmt.Printf("  \033[1mExpanded:\033[0m    %s\n", expansion.ExpandedQuery)
		return nil
	}

	fmt.Println("  \033[1mMatched Terms:\033[0m")
	for _, t := range expansion.ExpandedTerms {
		fmt.Printf("    \033[36m%s\033[0m → %s\n", t.OriginalTerm, t.Expansion)
		if t.Definition != "" {
			fmt.Printf("      (%s)\n", t.Definition)
		}
	}
	fmt.Println()
	fmt.Printf("  \033[1mExpanded:\033[0m    %s\n", expansion.ExpandedQuery)

	return nil
}

func outputGlossaryJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func outputGlossaryYAML(v interface{}) error {
	enc := yaml.NewEncoder(os.Stdout)
	return enc.Encode(v)
}

func truncateGlossary(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// connectToGlossaryDB creates a database connection for glossary commands.
// Uses the shared connectToDatabase function from ingest_email.go.
func connectToGlossaryDB(ctx context.Context, cfg *config.CLIConfig) (*pgxpool.Pool, error) {
	return connectToDatabase(ctx, cfg)
}
