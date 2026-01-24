package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/pkg/ingest/batch"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Email ingest specific flags
var (
	emailSource      string
	emailLabels      []string
	emailConcurrency int
	emailDryRun      bool
	emailResumeJob   string
)

// newIngestEmailCommand creates the 'ingest email' subcommand.
func newIngestEmailCommand(deps *IngestCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "email <path>",
		Short: "Ingest .eml files into Penfold",
		Long: `Ingest RFC 5322 email files (.eml) into the Penfold knowledge base.

Parses email headers, body content, and attachment metadata. Each email
is stored in the database and triggers AI processing for embeddings,
entity extraction, and search indexing.

Supports single files, directories (recursive), and glob patterns.
Duplicate detection is performed by message-id and content hash.

Examples:
  # Ingest a single email
  penf ingest email message.eml --source "archive"

  # Ingest a directory recursively
  penf ingest email ./emails/ --source "outlook-2024"

  # Ingest with labels and custom concurrency
  penf ingest email ./backup/ --source "backup" --labels "project-a,important" --concurrency 8

  # Preview without importing (dry run)
  penf ingest email ./emails/ --source "test" --dry-run

  # Resume an interrupted job
  penf ingest email ./emails/ --source "backup" --resume job-abc123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIngestEmail(cmd.Context(), deps, args[0])
		},
	}

	// Email-specific flags
	cmd.Flags().StringVarP(&emailSource, "source", "s", "", "Source tag identifier (required)")
	cmd.Flags().StringSliceVarP(&emailLabels, "labels", "l", nil, "Comma-separated labels to apply")
	cmd.Flags().IntVarP(&emailConcurrency, "concurrency", "w", 4, "Number of concurrent workers")
	cmd.Flags().BoolVar(&emailDryRun, "dry-run", false, "Preview import without persisting")
	cmd.Flags().StringVar(&emailResumeJob, "resume", "", "Resume an interrupted job by ID")

	cmd.MarkFlagRequired("source")

	return cmd
}

// runIngestEmail executes the email ingestion command.
func runIngestEmail(ctx context.Context, deps *IngestCommandDeps, path string) error {
	// Load configuration
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Validate path exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("path not found: %s", path)
	}
	if err != nil {
		return fmt.Errorf("accessing path: %w", err)
	}

	// Validate source is provided
	if emailSource == "" {
		return fmt.Errorf("--source flag is required")
	}

	// Determine tenant ID
	tenantID := ingestTenantID
	if tenantID == "" {
		tenantID = cfg.TenantID
	}
	if tenantID == "" {
		tenantID = "default" // Fallback for single-tenant
	}

	// Get output format
	format := getIngestOutputFormat(cfg)

	// Display startup message
	fmt.Printf("Email Ingest: %s\n", path)
	fmt.Printf("  Source:      %s\n", emailSource)
	fmt.Printf("  Tenant:      %s\n", tenantID)
	fmt.Printf("  Concurrency: %d workers\n", emailConcurrency)
	if len(emailLabels) > 0 {
		fmt.Printf("  Labels:      %s\n", strings.Join(emailLabels, ", "))
	}
	if emailDryRun {
		fmt.Printf("  Mode:        DRY RUN (no changes will be made)\n")
	}
	if emailResumeJob != "" {
		fmt.Printf("  Resuming:    %s\n", emailResumeJob)
	}
	if info.IsDir() {
		fmt.Printf("  Path type:   directory (recursive)\n")
	} else {
		fmt.Printf("  Path type:   single file\n")
	}
	fmt.Println()

	// Initialize database connection
	pool, err := connectToDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	// Initialize Redis connection
	redisClient, err := connectToRedis(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting to Redis: %w", err)
	}
	defer redisClient.Close()

	// Create logger
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "penf",
		Output:      os.Stderr,
	}).With(logging.F("component", "email_ingest"))

	// Configure processor
	processorCfg := batch.ProcessorConfig{
		Concurrency: emailConcurrency,
		TenantID:    tenantID,
		SourceTag:   emailSource,
		Labels:      emailLabels,
		DryRun:      emailDryRun,
		ResumeJobID: emailResumeJob,
	}

	// Create and run processor
	processor := batch.NewProcessor(pool, redisClient, logger, processorCfg)

	// Set up progress display
	progress := processor.Progress()
	if progress != nil {
		progress.SetOnUpdate(func(p *batch.Progress) {
			displayProgress(p, format)
		})
	}

	// Process files
	startTime := time.Now()
	result, err := processor.Process(ctx, path)
	if err != nil {
		return fmt.Errorf("processing failed: %w", err)
	}

	// Display results
	fmt.Println()
	displayResults(result, startTime, format)

	// Return error if there were failures
	if result.FailedCount > 0 {
		return fmt.Errorf("%d files failed to import", result.FailedCount)
	}

	return nil
}

// connectToDatabase establishes a database connection.
func connectToDatabase(ctx context.Context, cfg *config.CLIConfig) (*pgxpool.Pool, error) {
	// Build connection string from config or environment
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		// Build from individual components
		host := getEnvOrDefault("DB_HOST", "localhost")
		port := getEnvOrDefault("DB_PORT", "5432")
		user := getEnvOrDefault("DB_USER", "penfold")
		pass := getEnvOrDefault("DB_PASSWORD", "")
		dbname := getEnvOrDefault("DB_NAME", "penfold")
		sslmode := getEnvOrDefault("DB_SSLMODE", "prefer")

		connStr = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			host, port, user, pass, dbname, sslmode)
	}

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parsing connection string: %w", err)
	}

	poolCfg.MaxConns = 25
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("testing connection: %w", err)
	}

	return pool, nil
}

// connectToRedis establishes a Redis connection.
func connectToRedis(ctx context.Context, cfg *config.CLIConfig) (*redis.Client, error) {
	host := getEnvOrDefault("REDIS_HOST", "localhost")
	port := getEnvOrDefault("REDIS_PORT", "6379")
	password := os.Getenv("REDIS_PASSWORD")

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		Password: password,
		DB:       0,
	})

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("testing connection: %w", err)
	}

	return client, nil
}

// getEnvOrDefault returns environment variable value or default.
func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// displayProgress shows progress updates.
func displayProgress(p *batch.Progress, format config.OutputFormat) {
	if format != config.OutputFormatText {
		return
	}

	snapshot := p.Snapshot()
	if snapshot.ProcessedCount == 0 {
		return
	}

	// Simple progress line
	pct := snapshot.PercentComplete()
	remaining := ""
	if snapshot.EstimatedRemainingSeconds != nil {
		remaining = fmt.Sprintf(" ETA: %s", formatDuration(time.Duration(*snapshot.EstimatedRemainingSeconds)*time.Second))
	}

	fmt.Printf("\r  [%3.0f%%] %d/%d files (imported: %d, skipped: %d, failed: %d)%s   ",
		pct,
		snapshot.ProcessedCount,
		snapshot.TotalFiles,
		snapshot.ImportedCount,
		snapshot.SkippedCount,
		snapshot.FailedCount,
		remaining)
}

// displayResults shows the final results.
func displayResults(result *batch.ProcessResult, startTime time.Time, format config.OutputFormat) {
	duration := time.Since(startTime)

	switch format {
	case config.OutputFormatJSON:
		outputJSON(result)
	case config.OutputFormatYAML:
		outputYAML(result)
	default:
		outputResultsText(result, duration)
	}
}

// outputResultsText displays results in human-readable format.
func outputResultsText(result *batch.ProcessResult, duration time.Duration) {
	fmt.Println("Ingest Complete")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("  Job ID:        %s\n", result.JobID)
	fmt.Printf("  Total Files:   %d\n", result.TotalFiles)
	fmt.Printf("  Imported:      \033[32m%d\033[0m\n", result.ImportedCount)
	fmt.Printf("  Skipped:       \033[33m%d\033[0m (duplicates)\n", result.SkippedCount)
	fmt.Printf("  Failed:        \033[31m%d\033[0m\n", result.FailedCount)
	fmt.Printf("  Duration:      %s\n", formatDuration(duration))

	if result.TotalFiles > 0 && duration.Seconds() > 0 {
		rate := float64(result.TotalFiles) / duration.Seconds()
		fmt.Printf("  Rate:          %.1f files/sec\n", rate)
	}

	if result.Success {
		fmt.Printf("\n  Status:        \033[32mSUCCESS\033[0m\n")
	} else {
		fmt.Printf("\n  Status:        \033[31mFAILED\033[0m\n")
	}

	// Display errors if any
	if len(result.Errors) > 0 {
		fmt.Println("\nErrors:")
		for i, e := range result.Errors {
			if i >= 10 {
				fmt.Printf("  ... and %d more errors\n", len(result.Errors)-10)
				break
			}
			fmt.Printf("  - %s: %s\n", truncateIngestString(e.FilePath, 40), e.Error)
		}
	}
}

// outputJSON outputs result as JSON.
func outputJSON(result *batch.ProcessResult) {
	fmt.Printf(`{
  "job_id": "%s",
  "total_files": %d,
  "imported": %d,
  "skipped": %d,
  "failed": %d,
  "success": %t,
  "started_at": "%s",
  "completed_at": "%s"
}
`, result.JobID,
		result.TotalFiles,
		result.ImportedCount,
		result.SkippedCount,
		result.FailedCount,
		result.Success,
		result.StartedAt.Format(time.RFC3339),
		result.CompletedAt.Format(time.RFC3339))
}

// outputYAML outputs result as YAML.
func outputYAML(result *batch.ProcessResult) {
	fmt.Printf(`job_id: %s
total_files: %d
imported: %d
skipped: %d
failed: %d
success: %t
started_at: %s
completed_at: %s
`, result.JobID,
		result.TotalFiles,
		result.ImportedCount,
		result.SkippedCount,
		result.FailedCount,
		result.Success,
		result.StartedAt.Format(time.RFC3339),
		result.CompletedAt.Format(time.RFC3339))
}
