// Package cmd provides CLI commands for the penf tool.
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

	"github.com/otherjamesbrown/penfold/cmd/penf/client"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// LogLevel represents log severity levels.
type LogLevel string

const (
	// LogLevelDebug is for debug messages.
	LogLevelDebug LogLevel = "debug"
	// LogLevelInfo is for informational messages.
	LogLevelInfo LogLevel = "info"
	// LogLevelWarn is for warning messages.
	LogLevelWarn LogLevel = "warn"
	// LogLevelError is for error messages.
	LogLevelError LogLevel = "error"
)

// LogEntry represents a single log entry.
type LogEntry struct {
	Timestamp time.Time         `json:"timestamp" yaml:"timestamp"`
	Level     LogLevel          `json:"level" yaml:"level"`
	Service   string            `json:"service" yaml:"service"`
	Message   string            `json:"message" yaml:"message"`
	Fields    map[string]string `json:"fields,omitempty" yaml:"fields,omitempty"`
	TraceID   string            `json:"trace_id,omitempty" yaml:"trace_id,omitempty"`
}

// LogsResponse contains log query results.
type LogsResponse struct {
	Entries    []LogEntry `json:"entries" yaml:"entries"`
	TotalCount int        `json:"total_count" yaml:"total_count"`
	Truncated  bool       `json:"truncated" yaml:"truncated"`
	Query      LogQuery   `json:"query" yaml:"query"`
	FetchedAt  time.Time  `json:"fetched_at" yaml:"fetched_at"`
}

// LogQuery represents the query parameters for logs.
type LogQuery struct {
	Service  string    `json:"service,omitempty" yaml:"service,omitempty"`
	Level    string    `json:"level,omitempty" yaml:"level,omitempty"`
	Since    time.Time `json:"since,omitempty" yaml:"since,omitempty"`
	Until    time.Time `json:"until,omitempty" yaml:"until,omitempty"`
	Contains string    `json:"contains,omitempty" yaml:"contains,omitempty"`
	Limit    int       `json:"limit" yaml:"limit"`
}

// LogsCommandDeps holds the dependencies for logs commands.
type LogsCommandDeps struct {
	Config       *config.CLIConfig
	GRPCClient   *client.GRPCClient
	OutputFormat config.OutputFormat
	LoadConfig   func() (*config.CLIConfig, error)
	InitClient   func(*config.CLIConfig) (*client.GRPCClient, error)
}

// DefaultLogsDeps returns the default dependencies for production use.
func DefaultLogsDeps() *LogsCommandDeps {
	return &LogsCommandDeps{
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

// Logs command flags.
var (
	logsService  string
	logsLevel    string
	logsSince    string
	logsUntil    string
	logsContains string
	logsLimit    int
	logsFollow   bool
	logsOutput   string
	logsNoColor  bool
)

// NewLogsCommand creates the logs command.
func NewLogsCommand(deps *LogsCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultLogsDeps()
	}

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View and search service logs",
		Long: `View and search logs from Penfold services.

Retrieves logs from the centralized logging system, allowing you to filter
by service, log level, time range, and content.

Examples:
  # View recent logs from all services
  penf logs

  # View logs from a specific service
  penf logs --service=gateway

  # View only error logs
  penf logs --level=error

  # Search logs containing a specific term
  penf logs --contains="connection refused"

  # View logs from the last hour
  penf logs --since=1h

  # Follow logs in real-time
  penf logs --follow

  # Combine filters
  penf logs --service=orchestrator --level=warn --since=30m`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(cmd.Context(), deps)
		},
	}

	// Define flags.
	cmd.Flags().StringVarP(&logsService, "service", "s", "", "Filter by service (gateway, orchestrator, ai_service, etc.)")
	cmd.Flags().StringVarP(&logsLevel, "level", "l", "", "Minimum log level (debug, info, warn, error)")
	cmd.Flags().StringVar(&logsSince, "since", "15m", "Show logs since this time ago (e.g., 5m, 1h, 24h)")
	cmd.Flags().StringVar(&logsUntil, "until", "", "Show logs until this time ago")
	cmd.Flags().StringVarP(&logsContains, "contains", "c", "", "Filter logs containing this string")
	cmd.Flags().IntVarP(&logsLimit, "limit", "n", 100, "Maximum number of log entries")
	cmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow logs in real-time")
	cmd.Flags().StringVarP(&logsOutput, "output", "o", "", "Output format: text, json, yaml")
	cmd.Flags().BoolVar(&logsNoColor, "no-color", false, "Disable colored output")

	return cmd
}

// runLogs executes the logs command.
func runLogs(ctx context.Context, deps *LogsCommandDeps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Parse time ranges.
	var since, until time.Time
	if logsSince != "" {
		duration, err := time.ParseDuration(logsSince)
		if err != nil {
			return fmt.Errorf("invalid --since duration: %w", err)
		}
		since = time.Now().Add(-duration)
	}
	if logsUntil != "" {
		duration, err := time.ParseDuration(logsUntil)
		if err != nil {
			return fmt.Errorf("invalid --until duration: %w", err)
		}
		until = time.Now().Add(-duration)
	}

	// Validate log level if provided.
	if logsLevel != "" {
		validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
		if !validLevels[logsLevel] {
			return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", logsLevel)
		}
	}

	// Determine output format.
	outputFormat := cfg.OutputFormat
	if logsOutput != "" {
		outputFormat = config.OutputFormat(logsOutput)
		if !outputFormat.IsValid() {
			return fmt.Errorf("invalid output format: %s", logsOutput)
		}
	}

	query := LogQuery{
		Service:  logsService,
		Level:    logsLevel,
		Since:    since,
		Until:    until,
		Contains: logsContains,
		Limit:    logsLimit,
	}

	if logsFollow {
		return runLogsFollow(ctx, deps, query, outputFormat)
	}

	// Get logs (mock implementation until gRPC is connected).
	entries := getMockLogs(query)

	response := LogsResponse{
		Entries:    entries,
		TotalCount: len(entries),
		Truncated:  len(entries) >= logsLimit,
		Query:      query,
		FetchedAt:  time.Now(),
	}

	return outputLogs(outputFormat, response)
}

// runLogsFollow streams logs in real-time.
func runLogsFollow(ctx context.Context, deps *LogsCommandDeps, query LogQuery, outputFormat config.OutputFormat) error {
	fmt.Println("Following logs (press Ctrl+C to stop)...")
	fmt.Println()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastTimestamp := time.Now()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nStopped following logs.")
			return nil
		case <-ticker.C:
			// Get new logs since last fetch.
			query.Since = lastTimestamp
			entries := getMockLogs(query)

			if len(entries) > 0 {
				for _, entry := range entries {
					outputLogEntry(entry, logsNoColor)
				}
				lastTimestamp = entries[len(entries)-1].Timestamp
			}
		}
	}
}

// getMockLogs returns mock log entries.
func getMockLogs(query LogQuery) []LogEntry {
	now := time.Now()

	// Generate mock log entries.
	entries := []LogEntry{
		{
			Timestamp: now.Add(-30 * time.Second),
			Level:     LogLevelInfo,
			Service:   "gateway",
			Message:   "Request processed successfully",
			Fields: map[string]string{
				"method":   "POST",
				"path":     "/api/v1/search",
				"status":   "200",
				"duration": "45ms",
			},
			TraceID: "trace-abc123",
		},
		{
			Timestamp: now.Add(-45 * time.Second),
			Level:     LogLevelDebug,
			Service:   "orchestrator",
			Message:   "Processing content item",
			Fields: map[string]string{
				"content_id":   "doc-001",
				"content_type": "email",
			},
			TraceID: "trace-def456",
		},
		{
			Timestamp: now.Add(-1 * time.Minute),
			Level:     LogLevelWarn,
			Service:   "ai_service",
			Message:   "Slow model response",
			Fields: map[string]string{
				"model":    "llama-3.1-8b",
				"duration": "2.5s",
				"expected": "500ms",
			},
			TraceID: "trace-ghi789",
		},
		{
			Timestamp: now.Add(-2 * time.Minute),
			Level:     LogLevelError,
			Service:   "gateway",
			Message:   "Connection pool exhausted",
			Fields: map[string]string{
				"pool_size":   "100",
				"active":      "100",
				"waiting":     "15",
				"retry_after": "5s",
			},
			TraceID: "trace-jkl012",
		},
		{
			Timestamp: now.Add(-3 * time.Minute),
			Level:     LogLevelInfo,
			Service:   "orchestrator",
			Message:   "Workflow completed",
			Fields: map[string]string{
				"workflow_id": "wf-001",
				"type":        "ingestion",
				"duration":    "45m",
				"items":       "230",
			},
			TraceID: "trace-mno345",
		},
		{
			Timestamp: now.Add(-4 * time.Minute),
			Level:     LogLevelInfo,
			Service:   "search",
			Message:   "Search index updated",
			Fields: map[string]string{
				"documents_added":   "15",
				"documents_updated": "3",
				"index_size":        "1.2GB",
			},
			TraceID: "trace-pqr678",
		},
		{
			Timestamp: now.Add(-5 * time.Minute),
			Level:     LogLevelDebug,
			Service:   "ai_service",
			Message:   "Embedding batch processed",
			Fields: map[string]string{
				"batch_size": "50",
				"model":      "text-embedding-3-small",
				"duration":   "1.2s",
			},
			TraceID: "trace-stu901",
		},
		{
			Timestamp: now.Add(-6 * time.Minute),
			Level:     LogLevelWarn,
			Service:   "gateway",
			Message:   "Rate limit approaching",
			Fields: map[string]string{
				"tenant":    "tenant-001",
				"limit":     "1000",
				"current":   "950",
				"window":    "1h",
			},
			TraceID: "trace-vwx234",
		},
	}

	// Apply filters.
	var filtered []LogEntry
	for _, entry := range entries {
		// Filter by service.
		if query.Service != "" && entry.Service != query.Service {
			continue
		}

		// Filter by level.
		if query.Level != "" && !logLevelMatches(entry.Level, LogLevel(query.Level)) {
			continue
		}

		// Filter by time range.
		if !query.Since.IsZero() && entry.Timestamp.Before(query.Since) {
			continue
		}
		if !query.Until.IsZero() && entry.Timestamp.After(query.Until) {
			continue
		}

		// Filter by content.
		if query.Contains != "" {
			if !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(query.Contains)) {
				continue
			}
		}

		filtered = append(filtered, entry)

		// Apply limit.
		if len(filtered) >= query.Limit {
			break
		}
	}

	return filtered
}

// logLevelMatches checks if entry level meets minimum level.
func logLevelMatches(entryLevel, minLevel LogLevel) bool {
	levels := map[LogLevel]int{
		LogLevelDebug: 0,
		LogLevelInfo:  1,
		LogLevelWarn:  2,
		LogLevelError: 3,
	}

	return levels[entryLevel] >= levels[minLevel]
}

// outputLogs formats and outputs log entries.
func outputLogs(format config.OutputFormat, response LogsResponse) error {
	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(response)
	default:
		return outputLogsText(response)
	}
}

// outputLogsText formats log entries for terminal display.
func outputLogsText(response LogsResponse) error {
	if len(response.Entries) == 0 {
		fmt.Println("No log entries found.")
		return nil
	}

	for _, entry := range response.Entries {
		outputLogEntry(entry, logsNoColor)
	}

	if response.Truncated {
		fmt.Printf("\n(Showing %d entries, use --limit to see more)\n", len(response.Entries))
	}

	return nil
}

// outputLogEntry outputs a single log entry.
func outputLogEntry(entry LogEntry, noColor bool) {
	timestamp := entry.Timestamp.Format("15:04:05")
	levelColor := getLogLevelColor(entry.Level, noColor)
	levelStr := strings.ToUpper(string(entry.Level))

	if noColor {
		fmt.Printf("%s [%-5s] %s: %s", timestamp, levelStr, entry.Service, entry.Message)
	} else {
		fmt.Printf("\033[90m%s\033[0m %s%-5s\033[0m \033[36m%s\033[0m: %s",
			timestamp, levelColor, levelStr, entry.Service, entry.Message)
	}

	// Print fields on same line if few, or new lines if many.
	if len(entry.Fields) > 0 && len(entry.Fields) <= 3 {
		fmt.Print(" {")
		first := true
		for k, v := range entry.Fields {
			if !first {
				fmt.Print(", ")
			}
			fmt.Printf("%s=%s", k, v)
			first = false
		}
		fmt.Print("}")
	}

	fmt.Println()

	// Print fields on separate lines if many.
	if len(entry.Fields) > 3 {
		for k, v := range entry.Fields {
			if noColor {
				fmt.Printf("    %s=%s\n", k, v)
			} else {
				fmt.Printf("    \033[90m%s\033[0m=%s\n", k, v)
			}
		}
	}
}

// getLogLevelColor returns ANSI color code for log level.
func getLogLevelColor(level LogLevel, noColor bool) string {
	if noColor {
		return ""
	}
	switch level {
	case LogLevelDebug:
		return "\033[90m" // Gray
	case LogLevelInfo:
		return "\033[32m" // Green
	case LogLevelWarn:
		return "\033[33m" // Yellow
	case LogLevelError:
		return "\033[31m" // Red
	default:
		return ""
	}
}
