// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	searchTypes   []string
	searchAfter   string
	searchBefore  string
	searchMode    string
	searchLimit   int
	searchOffset  int
	searchSort    string
	searchVerbose bool
	searchOutput  string
)

// NewSearchCommand creates the search command.
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

Examples:
  # Basic natural language search
  penf search "project status update from last week"

  # Filter by content type
  penf search "meeting notes" --type=meeting,document

  # Search with date range
  penf search "budget review" --after=2024-01-01 --before=2024-06-30

  # Use semantic search mode for conceptual matching
  penf search "cost reduction strategies" --mode=semantic

  # Paginated results
  penf search "customer feedback" --limit=20 --offset=40

  # Sort by date instead of relevance
  penf search "project updates" --sort=date

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
	cmd.Flags().StringVarP(&searchSort, "sort", "s", "relevance", "Sort order: relevance, date")
	cmd.Flags().BoolVarP(&searchVerbose, "verbose", "v", false, "Show detailed scoring information")
	cmd.Flags().StringVarP(&searchOutput, "output", "o", "", "Output format: text, json, yaml")

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

	// Determine output format.
	outputFormat := cfg.OutputFormat
	if searchOutput != "" {
		outputFormat = config.OutputFormat(searchOutput)
		if !outputFormat.IsValid() {
			return fmt.Errorf("invalid output format: %s (must be text, json, or yaml)", searchOutput)
		}
	}

	// Validate search mode.
	mode := SearchMode(searchMode)
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
	sort := SortOrder(searchSort)
	if sort != SortOrderRelevance && sort != SortOrderDate && sort != SortOrderDateAsc {
		return fmt.Errorf("invalid sort order: %s (must be relevance or date)", searchSort)
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
		Sort:         sort,
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
