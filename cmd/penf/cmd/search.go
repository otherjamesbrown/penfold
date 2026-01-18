// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/client"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/services/search/query"
)

// SearchMode defines the type of search to perform.
type SearchMode string

const (
	// SearchModeHybrid combines semantic and keyword search (default).
	SearchModeHybrid SearchMode = "hybrid"
	// SearchModeKeyword performs keyword-only search.
	SearchModeKeyword SearchMode = "keyword"
	// SearchModeSemantic performs semantic-only search.
	SearchModeSemantic SearchMode = "semantic"
)

// SortOrder defines how search results are sorted.
type SortOrder string

const (
	// SortOrderRelevance sorts by relevance score (default).
	SortOrderRelevance SortOrder = "relevance"
	// SortOrderDate sorts by date, newest first.
	SortOrderDate SortOrder = "date"
	// SortOrderDateAsc sorts by date, oldest first.
	SortOrderDateAsc SortOrder = "date_asc"
)

// SearchResult represents a single search result.
type SearchResult struct {
	ID          string            `json:"id" yaml:"id"`
	Title       string            `json:"title" yaml:"title"`
	ContentType string            `json:"content_type" yaml:"content_type"`
	Source      string            `json:"source" yaml:"source"`
	Snippet     string            `json:"snippet" yaml:"snippet"`
	Highlights  []string          `json:"highlights,omitempty" yaml:"highlights,omitempty"`
	Score       float64           `json:"score" yaml:"score"`
	TextScore   float64           `json:"text_score,omitempty" yaml:"text_score,omitempty"`
	VectorScore float64           `json:"vector_score,omitempty" yaml:"vector_score,omitempty"`
	CreatedAt   time.Time         `json:"created_at" yaml:"created_at"`
	Metadata    map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// SearchResponse contains search results and metadata.
type SearchResponse struct {
	Query        string         `json:"query" yaml:"query"`
	Mode         SearchMode     `json:"mode" yaml:"mode"`
	Results      []SearchResult `json:"results" yaml:"results"`
	TotalCount   int64          `json:"total_count" yaml:"total_count"`
	QueryTimeMs  float64        `json:"query_time_ms" yaml:"query_time_ms"`
	Limit        int            `json:"limit" yaml:"limit"`
	Offset       int            `json:"offset" yaml:"offset"`
	Filters      SearchFilters  `json:"filters,omitempty" yaml:"filters,omitempty"`
	SearchedAt   time.Time      `json:"searched_at" yaml:"searched_at"`
}

// SearchFilters contains the active filters for a search.
type SearchFilters struct {
	ContentTypes []string   `json:"content_types,omitempty" yaml:"content_types,omitempty"`
	DateFrom     *time.Time `json:"date_from,omitempty" yaml:"date_from,omitempty"`
	DateTo       *time.Time `json:"date_to,omitempty" yaml:"date_to,omitempty"`
	Sort         SortOrder  `json:"sort,omitempty" yaml:"sort,omitempty"`
	FieldFilters []string   `json:"field_filters,omitempty" yaml:"field_filters,omitempty"`
	ExactMatch   bool       `json:"exact_match,omitempty" yaml:"exact_match,omitempty"`
}

// SearchHistoryEntry represents a single search history entry.
type SearchHistoryEntry struct {
	Query       string    `json:"query" yaml:"query"`
	Mode        string    `json:"mode" yaml:"mode"`
	ResultCount int       `json:"result_count" yaml:"result_count"`
	QueryTimeMs float64   `json:"query_time_ms" yaml:"query_time_ms"`
	SearchedAt  time.Time `json:"searched_at" yaml:"searched_at"`
	Filters     string    `json:"filters,omitempty" yaml:"filters,omitempty"`
}

// SearchHistoryResponse contains the search history list.
type SearchHistoryResponse struct {
	Entries    []SearchHistoryEntry `json:"entries" yaml:"entries"`
	TotalCount int                  `json:"total_count" yaml:"total_count"`
	FetchedAt  time.Time            `json:"fetched_at" yaml:"fetched_at"`
}

// AdvancedSearchOptions holds options for advanced search.
type AdvancedSearchOptions struct {
	FieldFilters []string   `json:"field_filters,omitempty" yaml:"field_filters,omitempty"`
	SortField    string     `json:"sort_field,omitempty" yaml:"sort_field,omitempty"`
	SortOrder    string     `json:"sort_order,omitempty" yaml:"sort_order,omitempty"`
	MinScore     float64    `json:"min_score,omitempty" yaml:"min_score,omitempty"`
	Semantic     bool       `json:"semantic,omitempty" yaml:"semantic,omitempty"`
	ExactMatch   bool       `json:"exact_match,omitempty" yaml:"exact_match,omitempty"`
	TextWeight   float64    `json:"text_weight,omitempty" yaml:"text_weight,omitempty"`
	VectorWeight float64    `json:"vector_weight,omitempty" yaml:"vector_weight,omitempty"`
}

// SearchCommandDeps holds the dependencies for search commands.
type SearchCommandDeps struct {
	Config       *config.CLIConfig
	GRPCClient   *client.GRPCClient
	OutputFormat config.OutputFormat
	LoadConfig   func() (*config.CLIConfig, error)
	InitClient   func(*config.CLIConfig) (*client.GRPCClient, error)
}

// DefaultSearchDeps returns the default dependencies for production use.
func DefaultSearchDeps() *SearchCommandDeps {
	return &SearchCommandDeps{
		LoadConfig: config.LoadConfig,
		InitClient: func(cfg *config.CLIConfig) (*client.GRPCClient, error) {
			opts := client.DefaultOptions()
			opts.Insecure = cfg.Insecure
			opts.Debug = cfg.Debug
			opts.ConnectTimeout = cfg.Timeout
			opts.TenantID = cfg.TenantID

			grpcClient := client.NewGRPCClient(cfg.ServerAddress, opts)
			ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
			defer cancel()

			if err := grpcClient.Connect(ctx); err != nil {
				return nil, fmt.Errorf("connecting to server: %w", err)
			}
			return grpcClient, nil
		},
	}
}

// Search command flags.
var (
	searchTypes    []string
	searchAfter    string
	searchBefore   string
	searchMode     string
	searchLimit    int
	searchOffset   int
	searchSort     string
	searchVerbose  bool
	searchOutput   string
	searchTenant   string
	searchSemantic bool
	searchExact    bool
)

// Advanced search flags.
var (
	advancedFilters  []string
	advancedSortSpec string
	advancedMinScore float64
)

// History command flags.
var (
	historyLimit int
	historyClear bool
)

// NewSearchCommand creates the root search command with all subcommands.
func NewSearchCommand(deps *SearchCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultSearchDeps()
	}

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search the Penfold knowledge base",
		Long: `Search the Penfold knowledge base using natural language or structured queries.

Penfold supports three search modes:
  - hybrid:   Combines semantic understanding with keyword matching (default)
  - semantic: Uses AI embeddings for conceptual similarity
  - keyword:  Traditional full-text search with boolean operators

Subcommands:
  advanced   Advanced search with field filters and sorting
  history    View and manage search history

Examples:
  # Basic natural language search
  penf search "project status update from last week"

  # Filter by content type
  penf search "meeting notes" --type=meeting,document

  # Search with date range
  penf search "budget review" --after=2024-01-01 --before=2024-06-30

  # Use semantic search for conceptual matching
  penf search "cost reduction strategies" --semantic

  # Exact match search
  penf search "ERROR: connection refused" --exact

  # Paginated results
  penf search "customer feedback" --limit=20 --offset=40

  # Sort by date instead of relevance
  penf search "project updates" --sort=date

  # Specify tenant
  penf search "project" --tenant=my-tenant-123

Query Syntax:
  - Quoted phrases: "exact phrase"
  - Boolean operators: project AND budget NOT cancelled
  - Field filters: type:email from:alice after:yesterday
  - Negation: -exclude_term`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd.Context(), deps, strings.Join(args, " "))
		},
	}

	// Define flags.
	cmd.Flags().StringSliceVarP(&searchTypes, "type", "t", nil, "Filter by content types (email,meeting,document,chat,note)")
	cmd.Flags().StringVar(&searchAfter, "after", "", "Filter results after this date (YYYY-MM-DD or relative: yesterday, lastweek)")
	cmd.Flags().StringVar(&searchBefore, "before", "", "Filter results before this date (YYYY-MM-DD or relative: today)")
	cmd.Flags().StringVarP(&searchMode, "mode", "m", "hybrid", "Search mode: hybrid, keyword, semantic")
	cmd.Flags().IntVarP(&searchLimit, "limit", "l", 10, "Maximum number of results (1-100)")
	cmd.Flags().IntVar(&searchOffset, "offset", 0, "Offset for pagination")
	cmd.Flags().StringVarP(&searchSort, "sort", "s", "relevance", "Sort order: relevance, date, date_asc")
	cmd.Flags().BoolVarP(&searchVerbose, "verbose", "v", false, "Show detailed scoring information")
	cmd.Flags().StringVarP(&searchOutput, "output", "o", "", "Output format: text, json, yaml")
	cmd.Flags().StringVar(&searchTenant, "tenant", "", "Tenant ID (overrides config)")
	cmd.Flags().BoolVar(&searchSemantic, "semantic", false, "Use semantic (vector) search only")
	cmd.Flags().BoolVar(&searchExact, "exact", false, "Exact match only (no fuzzy matching)")

	// Add subcommands.
	cmd.AddCommand(newSearchAdvancedCommand(deps))
	cmd.AddCommand(newSearchHistoryCommand(deps))

	return cmd
}

// runSearch executes the search command.
func runSearch(ctx context.Context, deps *SearchCommandDeps, queryStr string) error {
	// Load configuration.
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override tenant if specified.
	if searchTenant != "" {
		cfg.TenantID = searchTenant
	}

	// Determine output format.
	outputFormat := cfg.OutputFormat
	if searchOutput != "" {
		outputFormat = config.OutputFormat(searchOutput)
		if !outputFormat.IsValid() {
			return fmt.Errorf("invalid output format: %s (must be text, json, or yaml)", searchOutput)
		}
	}

	// Determine search mode based on flags.
	mode := SearchMode(searchMode)
	if searchSemantic {
		mode = SearchModeSemantic
	} else if searchExact {
		mode = SearchModeKeyword
	}

	// Validate search mode.
	if mode != SearchModeHybrid && mode != SearchModeKeyword && mode != SearchModeSemantic {
		return fmt.Errorf("invalid search mode: %s (must be hybrid, keyword, or semantic)", searchMode)
	}

	// Validate and clamp limit.
	if searchLimit < 1 {
		searchLimit = 1
	}
	if searchLimit > 100 {
		searchLimit = 100
	}

	// Validate offset.
	if searchOffset < 0 {
		searchOffset = 0
	}

	// Validate sort order.
	sortOrder := SortOrder(searchSort)
	if sortOrder != SortOrderRelevance && sortOrder != SortOrderDate && sortOrder != SortOrderDateAsc {
		return fmt.Errorf("invalid sort order: %s (must be relevance, date, or date_asc)", searchSort)
	}

	// Parse dates.
	parser := query.NewParser()
	var dateFrom, dateTo *time.Time

	if searchAfter != "" {
		d, err := parseSearchDate(parser, searchAfter)
		if err != nil {
			return fmt.Errorf("invalid --after date: %w", err)
		}
		dateFrom = &d
	}

	if searchBefore != "" {
		d, err := parseSearchDate(parser, searchBefore)
		if err != nil {
			return fmt.Errorf("invalid --before date: %w", err)
		}
		dateTo = &d
	}

	// Build filters.
	filters := SearchFilters{
		ContentTypes: searchTypes,
		DateFrom:     dateFrom,
		DateTo:       dateTo,
		Sort:         sortOrder,
		ExactMatch:   searchExact,
	}

	// Execute the search (mock for now until gRPC is connected).
	startTime := time.Now()
	results := executeSearch(queryStr, mode, filters, searchLimit, searchOffset, searchVerbose)
	queryTime := time.Since(startTime).Seconds() * 1000

	// Build response.
	response := SearchResponse{
		Query:       queryStr,
		Mode:        mode,
		Results:     results,
		TotalCount:  int64(len(results) + searchOffset + 5), // Mock total count.
		QueryTimeMs: queryTime,
		Limit:       searchLimit,
		Offset:      searchOffset,
		Filters:     filters,
		SearchedAt:  time.Now(),
	}

	// Output results.
	return outputSearchResults(outputFormat, response, searchVerbose)
}

// parseSearchDate parses a date string for search filters.
func parseSearchDate(p *query.Parser, dateStr string) (time.Time, error) {
	// Try relative dates first.
	lower := strings.ToLower(dateStr)
	now := time.Now()

	switch lower {
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
	case "yesterday":
		yesterday := now.AddDate(0, 0, -1)
		return time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, now.Location()), nil
	case "lastweek", "last_week":
		return now.AddDate(0, 0, -7), nil
	case "lastmonth", "last_month":
		return now.AddDate(0, -1, 0), nil
	case "thisweek", "this_week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startOfWeek := now.AddDate(0, 0, -weekday+1)
		return time.Date(startOfWeek.Year(), startOfWeek.Month(), startOfWeek.Day(), 0, 0, 0, 0, now.Location()), nil
	case "thismonth", "this_month":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), nil
	}

	// Try standard date formats.
	dateLayouts := []string{
		"2006-01-02",
		"2006/01/02",
		"01-02-2006",
		"01/02/2006",
		"Jan 2, 2006",
		"January 2, 2006",
	}

	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}

// executeSearch performs the actual search (mock implementation until gRPC is connected).
func executeSearch(queryStr string, mode SearchMode, filters SearchFilters, limit, offset int, verbose bool) []SearchResult {
	// TODO: Replace with actual gRPC call to search service.
	// For now, return mock results for development/testing.

	mockResults := []SearchResult{
		{
			ID:          "doc-001",
			Title:       "Q4 Budget Planning Meeting Notes",
			ContentType: "meeting",
			Source:      "calendar",
			Snippet:     "Discussed <em>project</em> timeline and resource allocation for Q4. Key decisions included prioritizing the infrastructure upgrade <em>project</em>...",
			Highlights:  []string{"Discussed <em>project</em> timeline", "infrastructure upgrade <em>project</em>"},
			Score:       0.92,
			TextScore:   0.88,
			VectorScore: 0.95,
			CreatedAt:   time.Now().AddDate(0, 0, -3),
			Metadata: map[string]string{
				"attendees": "alice@example.com, bob@example.com",
				"duration":  "1h",
			},
		},
		{
			ID:          "email-042",
			Title:       "Re: Project Status Update",
			ContentType: "email",
			Source:      "gmail",
			Snippet:     "Thanks for the <em>update</em>. The current <em>status</em> looks good. Please ensure all deliverables are on track for the end of month deadline...",
			Highlights:  []string{"Thanks for the <em>update</em>", "current <em>status</em> looks good"},
			Score:       0.87,
			TextScore:   0.91,
			VectorScore: 0.82,
			CreatedAt:   time.Now().AddDate(0, 0, -1),
			Metadata: map[string]string{
				"from":    "manager@example.com",
				"to":      "team@example.com",
				"subject": "Re: Project Status Update",
			},
		},
		{
			ID:          "doc-015",
			Title:       "Technical Design Document: API Gateway",
			ContentType: "document",
			Source:      "drive",
			Snippet:     "This document outlines the technical design for the new API Gateway. The <em>project</em> aims to consolidate all external API access...",
			Highlights:  []string{"technical design for the new API Gateway", "The <em>project</em> aims"},
			Score:       0.79,
			TextScore:   0.72,
			VectorScore: 0.85,
			CreatedAt:   time.Now().AddDate(0, -1, -5),
			Metadata: map[string]string{
				"author":     "engineering@example.com",
				"page_count": "12",
			},
		},
		{
			ID:          "chat-208",
			Title:       "Slack: #engineering",
			ContentType: "chat",
			Source:      "slack",
			Snippet:     "Hey team, quick <em>update</em> on the deployment - everything is green and we're ready for the production push tomorrow morning.",
			Highlights:  []string{"quick <em>update</em> on the deployment"},
			Score:       0.71,
			TextScore:   0.68,
			VectorScore: 0.74,
			CreatedAt:   time.Now().AddDate(0, 0, -2),
			Metadata: map[string]string{
				"channel": "#engineering",
				"author":  "devops@example.com",
			},
		},
		{
			ID:          "note-033",
			Title:       "Personal Notes: Sprint Retrospective",
			ContentType: "note",
			Source:      "manual",
			Snippet:     "Key takeaways from the retrospective: 1. Need better communication on blockers 2. <em>Project</em> scope creep was a concern...",
			Highlights:  []string{"<em>Project</em> scope creep was a concern"},
			Score:       0.65,
			TextScore:   0.60,
			VectorScore: 0.70,
			CreatedAt:   time.Now().AddDate(0, 0, -7),
			Metadata: map[string]string{
				"tags": "retrospective,sprint,agile",
			},
		},
	}

	// Apply content type filter.
	if len(filters.ContentTypes) > 0 {
		var filtered []SearchResult
		typeSet := make(map[string]bool)
		for _, t := range filters.ContentTypes {
			typeSet[strings.ToLower(t)] = true
		}
		for _, r := range mockResults {
			if typeSet[strings.ToLower(r.ContentType)] {
				filtered = append(filtered, r)
			}
		}
		mockResults = filtered
	}

	// Apply date filters.
	if filters.DateFrom != nil {
		var filtered []SearchResult
		for _, r := range mockResults {
			if !r.CreatedAt.Before(*filters.DateFrom) {
				filtered = append(filtered, r)
			}
		}
		mockResults = filtered
	}

	if filters.DateTo != nil {
		var filtered []SearchResult
		for _, r := range mockResults {
			if !r.CreatedAt.After(*filters.DateTo) {
				filtered = append(filtered, r)
			}
		}
		mockResults = filtered
	}

	// Apply sorting.
	if filters.Sort == SortOrderDate {
		// Sort by date descending (newest first) - already the default mock order.
	} else if filters.Sort == SortOrderDateAsc {
		// Reverse the order for ascending.
		for i, j := 0, len(mockResults)-1; i < j; i, j = i+1, j-1 {
			mockResults[i], mockResults[j] = mockResults[j], mockResults[i]
		}
	}

	// Apply pagination.
	if offset >= len(mockResults) {
		return []SearchResult{}
	}
	end := offset + limit
	if end > len(mockResults) {
		end = len(mockResults)
	}
	return mockResults[offset:end]
}

// outputSearchResults formats and outputs search results.
func outputSearchResults(format config.OutputFormat, response SearchResponse, verbose bool) error {
	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(response)
	default:
		return outputSearchResultsText(response, verbose)
	}
}

// outputSearchResultsText formats search results for terminal display.
func outputSearchResultsText(response SearchResponse, verbose bool) error {
	// Header.
	fmt.Printf("Search: %s\n", response.Query)
	fmt.Printf("Mode: %s | Results: %d of %d | Time: %.1fms\n",
		response.Mode, len(response.Results), response.TotalCount, response.QueryTimeMs)

	// Show active filters.
	if len(response.Filters.ContentTypes) > 0 {
		fmt.Printf("Types: %s\n", strings.Join(response.Filters.ContentTypes, ", "))
	}
	if response.Filters.DateFrom != nil || response.Filters.DateTo != nil {
		dateRange := "Date: "
		if response.Filters.DateFrom != nil {
			dateRange += response.Filters.DateFrom.Format("2006-01-02")
		} else {
			dateRange += "..."
		}
		dateRange += " to "
		if response.Filters.DateTo != nil {
			dateRange += response.Filters.DateTo.Format("2006-01-02")
		} else {
			dateRange += "..."
		}
		fmt.Println(dateRange)
	}
	fmt.Println()

	if len(response.Results) == 0 {
		fmt.Println("No results found.")
		fmt.Println("\nTry:")
		fmt.Println("  - Using different keywords")
		fmt.Println("  - Broadening your date range")
		fmt.Println("  - Removing content type filters")
		fmt.Println("  - Using --mode=semantic for conceptual matching")
		return nil
	}

	// Results.
	for i, result := range response.Results {
		resultNum := response.Offset + i + 1
		fmt.Printf("\033[1m%d. %s\033[0m\n", resultNum, result.Title)

		// Score with color coding.
		scoreColor := getScoreColor(result.Score)
		fmt.Printf("   Score: %s%.2f\033[0m", scoreColor, result.Score)
		if verbose {
			fmt.Printf(" (text: %.2f, vector: %.2f)", result.TextScore, result.VectorScore)
		}
		fmt.Println()

		// Type and source.
		fmt.Printf("   Type: %s | Source: %s | %s\n",
			result.ContentType,
			result.Source,
			formatRelativeTime(result.CreatedAt))

		// Snippet with highlighting preserved.
		snippet := formatSnippet(result.Snippet)
		fmt.Printf("   %s\n", snippet)

		// Verbose: show metadata.
		if verbose && len(result.Metadata) > 0 {
			fmt.Print("   Metadata: ")
			first := true
			for k, v := range result.Metadata {
				if !first {
					fmt.Print(", ")
				}
				fmt.Printf("%s=%s", k, truncateString(v, 30))
				first = false
			}
			fmt.Println()
		}

		fmt.Println()
	}

	// Pagination info.
	if response.TotalCount > int64(response.Offset+len(response.Results)) {
		nextOffset := response.Offset + len(response.Results)
		fmt.Printf("Showing %d-%d of %d results. ",
			response.Offset+1,
			response.Offset+len(response.Results),
			response.TotalCount)
		fmt.Printf("Use --offset=%d for next page.\n", nextOffset)
	}

	return nil
}

// getScoreColor returns ANSI color code based on score.
func getScoreColor(score float64) string {
	if score >= 0.8 {
		return "\033[32m" // Green for high relevance.
	} else if score >= 0.6 {
		return "\033[33m" // Yellow for medium relevance.
	}
	return "\033[31m" // Red for low relevance.
}

// formatSnippet formats the snippet for terminal display.
func formatSnippet(snippet string) string {
	// Replace HTML-style highlights with terminal formatting.
	result := strings.ReplaceAll(snippet, "<em>", "\033[1;33m")
	result = strings.ReplaceAll(result, "</em>", "\033[0m")

	// Truncate if too long.
	if len(result) > 200 {
		result = result[:197] + "..."
	}

	return result
}

// formatRelativeTime formats a timestamp as relative time.
func formatRelativeTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return strconv.Itoa(mins) + " minutes ago"
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return strconv.Itoa(hours) + " hours ago"
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return strconv.Itoa(days) + " days ago"
	case diff < 30*24*time.Hour:
		weeks := int(diff.Hours() / (24 * 7))
		if weeks == 1 {
			return "1 week ago"
		}
		return strconv.Itoa(weeks) + " weeks ago"
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / (24 * 30))
		if months == 1 {
			return "1 month ago"
		}
		return strconv.Itoa(months) + " months ago"
	default:
		return t.Format("Jan 2, 2006")
	}
}

// ValidateSearchMode validates a search mode string.
func ValidateSearchMode(mode string) bool {
	switch SearchMode(mode) {
	case SearchModeHybrid, SearchModeKeyword, SearchModeSemantic:
		return true
	default:
		return false
	}
}

// ValidateSortOrder validates a sort order string.
func ValidateSortOrder(sort string) bool {
	switch SortOrder(sort) {
	case SortOrderRelevance, SortOrderDate, SortOrderDateAsc:
		return true
	default:
		return false
	}
}

// =============================================================================
// Advanced Search Subcommand
// =============================================================================

// newSearchAdvancedCommand creates the 'search advanced' subcommand.
func newSearchAdvancedCommand(deps *SearchCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "advanced [query]",
		Short: "Advanced search with field filters and sorting",
		Long: `Perform an advanced search with field filters and custom sorting.

Advanced search allows you to specify field-specific filters and control
how results are sorted using a structured syntax.

Field Filters (--filter):
  Use "field:value" syntax to filter specific fields:
  - from:alice@example.com    Filter by sender
  - to:team@example.com       Filter by recipient
  - subject:budget            Filter by subject
  - source:gmail              Filter by source system
  - tag:important             Filter by tag

Sort Specification (--sort):
  Use "field:order" syntax to sort results:
  - created_at:desc           Newest first (default for date)
  - created_at:asc            Oldest first
  - score:desc                Highest relevance first
  - title:asc                 Alphabetical by title

Examples:
  # Search with field filters
  penf search advanced "project update" --filter "from:alice@example.com"

  # Multiple filters
  penf search advanced "budget" --filter "type:email" --filter "from:finance@"

  # Custom sorting
  penf search advanced "meeting notes" --sort "created_at:desc"

  # Combine filters and sorting
  penf search advanced "Q4 review" --filter "type:document" --sort "title:asc"

  # Set minimum score threshold
  penf search advanced "important" --min-score 0.7`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdvancedSearch(cmd.Context(), deps, strings.Join(args, " "))
		},
	}

	// Define advanced search flags.
	cmd.Flags().StringArrayVar(&advancedFilters, "filter", nil, "Field filter in 'field:value' format (can be repeated)")
	cmd.Flags().StringVar(&advancedSortSpec, "sort", "", "Sort specification in 'field:order' format")
	cmd.Flags().Float64Var(&advancedMinScore, "min-score", 0.0, "Minimum relevance score threshold (0.0-1.0)")
	cmd.Flags().IntVarP(&searchLimit, "limit", "l", 10, "Maximum number of results (1-100)")
	cmd.Flags().IntVar(&searchOffset, "offset", 0, "Offset for pagination")
	cmd.Flags().BoolVarP(&searchVerbose, "verbose", "v", false, "Show detailed scoring information")
	cmd.Flags().StringVarP(&searchOutput, "output", "o", "", "Output format: text, json, yaml")
	cmd.Flags().StringVar(&searchTenant, "tenant", "", "Tenant ID (overrides config)")
	cmd.Flags().BoolVar(&searchSemantic, "semantic", false, "Use semantic (vector) search only")
	cmd.Flags().BoolVar(&searchExact, "exact", false, "Exact match only (no fuzzy matching)")

	return cmd
}

// runAdvancedSearch executes the advanced search command.
func runAdvancedSearch(ctx context.Context, deps *SearchCommandDeps, queryStr string) error {
	// Load configuration.
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override tenant if specified.
	if searchTenant != "" {
		cfg.TenantID = searchTenant
	}

	// Determine output format.
	outputFormat := cfg.OutputFormat
	if searchOutput != "" {
		outputFormat = config.OutputFormat(searchOutput)
		if !outputFormat.IsValid() {
			return fmt.Errorf("invalid output format: %s (must be text, json, or yaml)", searchOutput)
		}
	}

	// Determine search mode.
	mode := SearchModeHybrid
	if searchSemantic {
		mode = SearchModeSemantic
	} else if searchExact {
		mode = SearchModeKeyword
	}

	// Validate limit.
	if searchLimit < 1 {
		searchLimit = 1
	}
	if searchLimit > 100 {
		searchLimit = 100
	}

	// Validate offset.
	if searchOffset < 0 {
		searchOffset = 0
	}

	// Validate min score.
	if advancedMinScore < 0.0 {
		advancedMinScore = 0.0
	}
	if advancedMinScore > 1.0 {
		advancedMinScore = 1.0
	}

	// Parse sort specification.
	sortOrder := SortOrderRelevance
	var sortField string
	if advancedSortSpec != "" {
		parts := strings.SplitN(advancedSortSpec, ":", 2)
		sortField = parts[0]
		if len(parts) > 1 {
			switch strings.ToLower(parts[1]) {
			case "asc":
				if sortField == "created_at" || sortField == "date" {
					sortOrder = SortOrderDateAsc
				}
			case "desc":
				if sortField == "created_at" || sortField == "date" {
					sortOrder = SortOrderDate
				}
			default:
				return fmt.Errorf("invalid sort order: %s (must be asc or desc)", parts[1])
			}
		}
	}

	// Build filters.
	filters := SearchFilters{
		Sort:         sortOrder,
		FieldFilters: advancedFilters,
		ExactMatch:   searchExact,
	}

	// Execute the advanced search (mock for now until gRPC is connected).
	startTime := time.Now()
	results := executeAdvancedSearch(queryStr, mode, filters, advancedMinScore, searchLimit, searchOffset, searchVerbose)
	queryTime := time.Since(startTime).Seconds() * 1000

	// Build response.
	response := SearchResponse{
		Query:       queryStr,
		Mode:        mode,
		Results:     results,
		TotalCount:  int64(len(results) + searchOffset + 3),
		QueryTimeMs: queryTime,
		Limit:       searchLimit,
		Offset:      searchOffset,
		Filters:     filters,
		SearchedAt:  time.Now(),
	}

	// Output results.
	return outputSearchResults(outputFormat, response, searchVerbose)
}

// executeAdvancedSearch performs an advanced search with filters (mock implementation).
func executeAdvancedSearch(queryStr string, mode SearchMode, filters SearchFilters, minScore float64, limit, offset int, verbose bool) []SearchResult {
	// TODO: Replace with actual gRPC call to search service.
	// For now, use the basic executeSearch and filter by score.
	results := executeSearch(queryStr, mode, filters, limit*2, 0, verbose)

	// Apply field filters.
	if len(filters.FieldFilters) > 0 {
		var filtered []SearchResult
		for _, result := range results {
			match := true
			for _, filter := range filters.FieldFilters {
				parts := strings.SplitN(filter, ":", 2)
				if len(parts) != 2 {
					continue
				}
				field, value := strings.ToLower(parts[0]), strings.ToLower(parts[1])

				switch field {
				case "type":
					if !strings.Contains(strings.ToLower(result.ContentType), value) {
						match = false
					}
				case "source":
					if !strings.Contains(strings.ToLower(result.Source), value) {
						match = false
					}
				case "from":
					if from, ok := result.Metadata["from"]; ok {
						if !strings.Contains(strings.ToLower(from), value) {
							match = false
						}
					} else {
						match = false
					}
				case "to":
					if to, ok := result.Metadata["to"]; ok {
						if !strings.Contains(strings.ToLower(to), value) {
							match = false
						}
					} else {
						match = false
					}
				case "subject":
					if !strings.Contains(strings.ToLower(result.Title), value) {
						match = false
					}
				case "tag":
					if tags, ok := result.Metadata["tags"]; ok {
						if !strings.Contains(strings.ToLower(tags), value) {
							match = false
						}
					} else {
						match = false
					}
				}
			}
			if match {
				filtered = append(filtered, result)
			}
		}
		results = filtered
	}

	// Apply minimum score filter.
	if minScore > 0 {
		var filtered []SearchResult
		for _, result := range results {
			if result.Score >= minScore {
				filtered = append(filtered, result)
			}
		}
		results = filtered
	}

	// Apply pagination.
	if offset >= len(results) {
		return []SearchResult{}
	}
	end := offset + limit
	if end > len(results) {
		end = len(results)
	}
	return results[offset:end]
}

// =============================================================================
// Search History Subcommand
// =============================================================================

// newSearchHistoryCommand creates the 'search history' subcommand.
func newSearchHistoryCommand(deps *SearchCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "View and manage search history",
		Long: `View and manage your search history.

The search history shows your recent searches, including the query, mode,
result count, and when the search was performed.

Examples:
  # View recent searches
  penf search history

  # View more history entries
  penf search history --limit 50

  # Clear search history
  penf search history clear

  # Output as JSON
  penf search history --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearchHistory(cmd.Context(), deps)
		},
	}

	// Define history flags.
	cmd.Flags().IntVarP(&historyLimit, "limit", "l", 20, "Number of history entries to show")
	cmd.Flags().StringVarP(&searchOutput, "output", "o", "", "Output format: text, json, yaml")

	// Add clear subcommand.
	cmd.AddCommand(newSearchHistoryClearCommand(deps))

	return cmd
}

// newSearchHistoryClearCommand creates the 'search history clear' subcommand.
func newSearchHistoryClearCommand(deps *SearchCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Clear search history",
		Long: `Clear your search history.

This permanently removes all saved search history entries.

Example:
  penf search history clear`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearchHistoryClear(cmd.Context(), deps)
		},
	}
}

// runSearchHistory executes the search history command.
func runSearchHistory(ctx context.Context, deps *SearchCommandDeps) error {
	// Load configuration.
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Determine output format.
	outputFormat := cfg.OutputFormat
	if searchOutput != "" {
		outputFormat = config.OutputFormat(searchOutput)
		if !outputFormat.IsValid() {
			return fmt.Errorf("invalid output format: %s (must be text, json, or yaml)", searchOutput)
		}
	}

	// Validate limit.
	if historyLimit < 1 {
		historyLimit = 1
	}
	if historyLimit > 100 {
		historyLimit = 100
	}

	// Get search history (mock for now).
	entries := getMockSearchHistory(historyLimit)

	response := SearchHistoryResponse{
		Entries:    entries,
		TotalCount: len(entries),
		FetchedAt:  time.Now(),
	}

	return outputSearchHistory(outputFormat, response)
}

// runSearchHistoryClear executes the search history clear command.
func runSearchHistoryClear(ctx context.Context, deps *SearchCommandDeps) error {
	// Load configuration.
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// TODO: Replace with actual gRPC call to clear history.
	fmt.Println("Search history cleared.")
	return nil
}

// getMockSearchHistory returns mock search history entries.
func getMockSearchHistory(limit int) []SearchHistoryEntry {
	entries := []SearchHistoryEntry{
		{
			Query:       "project status update",
			Mode:        "hybrid",
			ResultCount: 15,
			QueryTimeMs: 45.2,
			SearchedAt:  time.Now().Add(-10 * time.Minute),
			Filters:     "type:email",
		},
		{
			Query:       "Q4 budget planning",
			Mode:        "semantic",
			ResultCount: 8,
			QueryTimeMs: 62.1,
			SearchedAt:  time.Now().Add(-1 * time.Hour),
			Filters:     "",
		},
		{
			Query:       "meeting notes engineering",
			Mode:        "hybrid",
			ResultCount: 23,
			QueryTimeMs: 38.5,
			SearchedAt:  time.Now().Add(-3 * time.Hour),
			Filters:     "type:meeting",
		},
		{
			Query:       "deployment schedule",
			Mode:        "keyword",
			ResultCount: 5,
			QueryTimeMs: 12.3,
			SearchedAt:  time.Now().Add(-24 * time.Hour),
			Filters:     "source:slack",
		},
		{
			Query:       "customer feedback analysis",
			Mode:        "hybrid",
			ResultCount: 31,
			QueryTimeMs: 78.9,
			SearchedAt:  time.Now().Add(-48 * time.Hour),
			Filters:     "",
		},
		{
			Query:       "API gateway design",
			Mode:        "semantic",
			ResultCount: 12,
			QueryTimeMs: 55.4,
			SearchedAt:  time.Now().Add(-72 * time.Hour),
			Filters:     "type:document",
		},
		{
			Query:       "team standup notes",
			Mode:        "hybrid",
			ResultCount: 45,
			QueryTimeMs: 42.0,
			SearchedAt:  time.Now().Add(-96 * time.Hour),
			Filters:     "",
		},
		{
			Query:       "sprint retrospective",
			Mode:        "keyword",
			ResultCount: 7,
			QueryTimeMs: 18.2,
			SearchedAt:  time.Now().Add(-120 * time.Hour),
			Filters:     "tag:agile",
		},
	}

	// Sort by most recent first.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].SearchedAt.After(entries[j].SearchedAt)
	})

	if limit < len(entries) {
		return entries[:limit]
	}
	return entries
}

// outputSearchHistory formats and outputs search history.
func outputSearchHistory(format config.OutputFormat, response SearchHistoryResponse) error {
	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(response)
	default:
		return outputSearchHistoryText(response)
	}
}

// outputSearchHistoryText formats search history for terminal display.
func outputSearchHistoryText(response SearchHistoryResponse) error {
	if len(response.Entries) == 0 {
		fmt.Println("No search history found.")
		fmt.Println("\nTry searching with: penf search \"your query\"")
		return nil
	}

	fmt.Printf("Search History (%d entries):\n\n", response.TotalCount)
	fmt.Println("  WHEN            MODE       RESULTS   TIME      QUERY")
	fmt.Println("  ----            ----       -------   ----      -----")

	for _, entry := range response.Entries {
		when := formatRelativeTime(entry.SearchedAt)
		queryDisplay := truncateString(entry.Query, 40)
		if entry.Filters != "" {
			queryDisplay += fmt.Sprintf(" [%s]", truncateString(entry.Filters, 15))
		}

		fmt.Printf("  %-14s  %-8s   %-7d   %-7.1fms  %s\n",
			when,
			entry.Mode,
			entry.ResultCount,
			entry.QueryTimeMs,
			queryDisplay)
	}

	fmt.Println()
	fmt.Println("Use 'penf search history clear' to clear history.")
	return nil
}

// ValidateFieldFilter validates a field filter string.
func ValidateFieldFilter(filter string) bool {
	parts := strings.SplitN(filter, ":", 2)
	if len(parts) != 2 {
		return false
	}

	validFields := map[string]bool{
		"type": true, "source": true, "from": true,
		"to": true, "subject": true, "tag": true,
	}

	return validFields[strings.ToLower(parts[0])]
}
