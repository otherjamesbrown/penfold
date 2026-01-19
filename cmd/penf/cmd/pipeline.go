// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

// PipelineStats holds pipeline statistics from the database.
type PipelineStats struct {
	// Sources
	SourcesTotal      int64            `json:"sources_total"`
	SourcesByStatus   map[string]int64 `json:"sources_by_status"`

	// Embeddings
	EmbeddingsTotal   int64 `json:"embeddings_total"`
	EmbeddingsRecent  int64 `json:"embeddings_recent"` // Last hour

	// Attachments
	AttachmentsTotal  int64            `json:"attachments_total"`
	AttachmentsByTier map[string]int64 `json:"attachments_by_tier"`

	// Jobs
	JobsTotal         int64            `json:"jobs_total"`
	JobsByStatus      map[string]int64 `json:"jobs_by_status"`
	RecentJobs        []JobSummary     `json:"recent_jobs"`

	// Timestamp
	Timestamp         time.Time `json:"timestamp"`
}

// JobSummary holds summary info for an ingest job.
type JobSummary struct {
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	SourceTag      string    `json:"source_tag"`
	TotalFiles     int       `json:"total_files"`
	ImportedCount  int       `json:"imported_count"`
	SkippedCount   int       `json:"skipped_count"`
	FailedCount    int       `json:"failed_count"`
	CreatedAt      time.Time `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

// NewPipelineCommand creates the pipeline command.
func NewPipelineCommand(_ interface{}) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pipeline",
		Aliases: []string{"pipe", "pl"},
		Short:   "View pipeline status and job tracking",
		Long: `View the status of the Penfold processing pipeline.

The pipeline processes ingested content through several stages:
  1. Ingest: Raw content stored in sources table (pending)
  2. Process: Worker extracts text, generates embeddings
  3. Enrich: AI generates summaries, extracts assertions
  4. Index: Content becomes searchable

Commands:
  status  - Show overall pipeline statistics
  job     - Show details for a specific job
  jobs    - List recent ingest jobs`,
	}

	cmd.AddCommand(newPipelineStatusCmd())
	cmd.AddCommand(newPipelineJobCmd())
	cmd.AddCommand(newPipelineJobsCmd())

	return cmd
}

func newPipelineStatusCmd() *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show pipeline statistics",
		Long: `Show overall pipeline statistics including:
  - Sources by processing status (pending, processing, completed, failed)
  - Embeddings count and recent rate
  - Attachments by tier (auto_process, auto_skip, pending_review)
  - Recent ingest jobs`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pool, err := connectDB()
			if err != nil {
				return err
			}
			defer pool.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			stats, err := getPipelineStats(ctx, pool)
			if err != nil {
				return fmt.Errorf("getting pipeline stats: %w", err)
			}

			if outputFormat == "json" {
				return outputPipelineJSON(stats)
			}
			return outputPipelineHuman(stats)
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")
	return cmd
}

func newPipelineJobCmd() *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "job <job-id>",
		Short: "Show details for a specific ingest job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := args[0]

			pool, err := connectDB()
			if err != nil {
				return err
			}
			defer pool.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			job, sources, err := getJobDetails(ctx, pool, jobID)
			if err != nil {
				return err
			}

			if outputFormat == "json" {
				return outputJobJSON(job, sources)
			}
			return outputJobHuman(job, sources)
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")
	return cmd
}

func newPipelineJobsCmd() *cobra.Command {
	var limit int
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "List recent ingest jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			pool, err := connectDB()
			if err != nil {
				return err
			}
			defer pool.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			jobs, err := getRecentJobs(ctx, pool, limit)
			if err != nil {
				return err
			}

			if outputFormat == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(jobs)
			}
			return outputJobsHuman(jobs)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 10, "Maximum number of jobs to show")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")
	return cmd
}

// connectDB creates a database connection pool.
func connectDB() (*pgxpool.Pool, error) {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		host := getEnvOrDefault("DB_HOST", "localhost")
		port := getEnvOrDefault("DB_PORT", "5432")
		user := getEnvOrDefault("DB_USER", "penfold")
		password := getEnvOrDefault("DB_PASSWORD", "penfold")
		database := getEnvOrDefault("DB_NAME", "penfold")
		sslmode := getEnvOrDefault("DB_SSLMODE", "disable")
		connStr = fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s",
			user, password, host, port, database, sslmode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("testing connection: %w", err)
	}

	return pool, nil
}

// getPipelineStats queries the database for pipeline statistics.
func getPipelineStats(ctx context.Context, pool *pgxpool.Pool) (*PipelineStats, error) {
	stats := &PipelineStats{
		SourcesByStatus:   make(map[string]int64),
		AttachmentsByTier: make(map[string]int64),
		JobsByStatus:      make(map[string]int64),
		Timestamp:         time.Now(),
	}

	// Sources total
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM sources").Scan(&stats.SourcesTotal)
	if err != nil {
		return nil, fmt.Errorf("counting sources: %w", err)
	}

	// Sources by status
	rows, err := pool.Query(ctx, "SELECT processing_status, COUNT(*) FROM sources GROUP BY processing_status")
	if err != nil {
		return nil, fmt.Errorf("counting sources by status: %w", err)
	}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return nil, err
		}
		stats.SourcesByStatus[status] = count
	}
	rows.Close()

	// Embeddings total
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM embeddings").Scan(&stats.EmbeddingsTotal)
	if err != nil {
		// Table might not exist
		stats.EmbeddingsTotal = 0
	}

	// Embeddings recent (last hour)
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM embeddings WHERE created_at > NOW() - INTERVAL '1 hour'").Scan(&stats.EmbeddingsRecent)
	if err != nil {
		stats.EmbeddingsRecent = 0
	}

	// Attachments total
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM source_attachments").Scan(&stats.AttachmentsTotal)
	if err != nil {
		stats.AttachmentsTotal = 0
	}

	// Attachments by tier
	rows, err = pool.Query(ctx, "SELECT processing_tier, COUNT(*) FROM source_attachments GROUP BY processing_tier")
	if err == nil {
		for rows.Next() {
			var tier string
			var count int64
			if err := rows.Scan(&tier, &count); err != nil {
				rows.Close()
				break
			}
			stats.AttachmentsByTier[tier] = count
		}
		rows.Close()
	}

	// Jobs total
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM ingest_jobs").Scan(&stats.JobsTotal)
	if err != nil {
		stats.JobsTotal = 0
	}

	// Jobs by status
	rows, err = pool.Query(ctx, "SELECT status, COUNT(*) FROM ingest_jobs GROUP BY status")
	if err == nil {
		for rows.Next() {
			var status string
			var count int64
			if err := rows.Scan(&status, &count); err != nil {
				rows.Close()
				break
			}
			stats.JobsByStatus[status] = count
		}
		rows.Close()
	}

	// Recent jobs
	stats.RecentJobs, _ = getRecentJobs(ctx, pool, 5)

	return stats, nil
}

// getRecentJobs retrieves recent ingest jobs.
func getRecentJobs(ctx context.Context, pool *pgxpool.Pool, limit int) ([]JobSummary, error) {
	query := `
		SELECT id, status, source_tag, total_files, imported_count, skipped_count, failed_count,
		       created_at, completed_at
		FROM ingest_jobs
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("querying jobs: %w", err)
	}
	defer rows.Close()

	var jobs []JobSummary
	for rows.Next() {
		var job JobSummary
		err := rows.Scan(
			&job.ID, &job.Status, &job.SourceTag,
			&job.TotalFiles, &job.ImportedCount, &job.SkippedCount, &job.FailedCount,
			&job.CreatedAt, &job.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning job: %w", err)
		}
		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// JobDetails holds detailed information about a job.
type JobDetails struct {
	JobSummary
	ProcessedFiles []string `json:"processed_files,omitempty"`
}

// SourceSummary holds summary info about sources for a job.
type SourceSummary struct {
	Total     int64            `json:"total"`
	ByStatus  map[string]int64 `json:"by_status"`
}

// getJobDetails retrieves detailed information about a job.
func getJobDetails(ctx context.Context, pool *pgxpool.Pool, jobID string) (*JobDetails, *SourceSummary, error) {
	query := `
		SELECT id, status, source_tag, total_files, imported_count, skipped_count, failed_count,
		       created_at, completed_at, processed_files
		FROM ingest_jobs
		WHERE id = $1
	`

	var job JobDetails
	var processedJSON []byte
	err := pool.QueryRow(ctx, query, jobID).Scan(
		&job.ID, &job.Status, &job.SourceTag,
		&job.TotalFiles, &job.ImportedCount, &job.SkippedCount, &job.FailedCount,
		&job.CreatedAt, &job.CompletedAt, &processedJSON,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("job not found: %s", jobID)
	}

	if len(processedJSON) > 0 {
		json.Unmarshal(processedJSON, &job.ProcessedFiles)
	}

	// Get source status for this job's sources (by source_tag in metadata)
	sources := &SourceSummary{
		ByStatus: make(map[string]int64),
	}

	// Count sources that were imported by this job
	countQuery := `
		SELECT processing_status, COUNT(*)
		FROM sources
		WHERE ingestion_metadata->>'source_tag' = $1
		   OR (source_system = 'manual_eml' AND created_at >= $2 AND created_at <= COALESCE($3, NOW()))
		GROUP BY processing_status
	`
	rows, err := pool.Query(ctx, countQuery, job.SourceTag, job.CreatedAt, job.CompletedAt)
	if err == nil {
		for rows.Next() {
			var status string
			var count int64
			if err := rows.Scan(&status, &count); err == nil {
				sources.ByStatus[status] = count
				sources.Total += count
			}
		}
		rows.Close()
	}

	return &job, sources, nil
}

// Output functions

func outputPipelineJSON(stats *PipelineStats) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(stats)
}

func outputPipelineHuman(stats *PipelineStats) error {
	fmt.Println("Pipeline Status")
	fmt.Println("=" + fmt.Sprintf("%49s", "="))
	fmt.Printf("  Timestamp: %s\n\n", stats.Timestamp.Format(time.RFC3339))

	// Sources
	fmt.Println("Sources")
	fmt.Println("-" + fmt.Sprintf("%49s", "-"))
	fmt.Printf("  Total: %d\n", stats.SourcesTotal)
	if len(stats.SourcesByStatus) > 0 {
		fmt.Println("  By Status:")
		for status, count := range stats.SourcesByStatus {
			color := "\033[33m" // Yellow for pending
			if status == "completed" {
				color = "\033[32m" // Green
			} else if status == "failed" {
				color = "\033[31m" // Red
			}
			fmt.Printf("    %s%-12s\033[0m %d\n", color, status, count)
		}
	}
	fmt.Println()

	// Embeddings
	fmt.Println("Embeddings")
	fmt.Println("-" + fmt.Sprintf("%49s", "-"))
	fmt.Printf("  Total: %d\n", stats.EmbeddingsTotal)
	fmt.Printf("  Last Hour: %d\n", stats.EmbeddingsRecent)

	// Calculate coverage
	if stats.SourcesTotal > 0 {
		coverage := float64(stats.EmbeddingsTotal) / float64(stats.SourcesTotal) * 100
		color := "\033[31m" // Red if low
		if coverage >= 90 {
			color = "\033[32m" // Green
		} else if coverage >= 50 {
			color = "\033[33m" // Yellow
		}
		fmt.Printf("  Coverage: %s%.1f%%\033[0m\n", color, coverage)
	}
	fmt.Println()

	// Attachments
	fmt.Println("Attachments")
	fmt.Println("-" + fmt.Sprintf("%49s", "-"))
	fmt.Printf("  Total: %d\n", stats.AttachmentsTotal)
	if len(stats.AttachmentsByTier) > 0 {
		fmt.Println("  By Tier:")
		for tier, count := range stats.AttachmentsByTier {
			fmt.Printf("    %-16s %d\n", tier, count)
		}
	}
	fmt.Println()

	// Jobs
	fmt.Println("Ingest Jobs")
	fmt.Println("-" + fmt.Sprintf("%49s", "-"))
	fmt.Printf("  Total: %d\n", stats.JobsTotal)
	if len(stats.JobsByStatus) > 0 {
		fmt.Println("  By Status:")
		for status, count := range stats.JobsByStatus {
			fmt.Printf("    %-16s %d\n", status, count)
		}
	}
	fmt.Println()

	// Recent jobs
	if len(stats.RecentJobs) > 0 {
		fmt.Println("Recent Jobs")
		fmt.Println("-" + fmt.Sprintf("%49s", "-"))
		fmt.Println("  ID                                    STATUS       FILES   IMPORTED")
		for _, job := range stats.RecentJobs {
			statusColor := "\033[32m"
			if job.Status == "failed" {
				statusColor = "\033[31m"
			} else if job.Status == "in_progress" {
				statusColor = "\033[33m"
			}
			fmt.Printf("  %s  %s%-12s\033[0m %5d   %8d\n",
				job.ID, statusColor, job.Status, job.TotalFiles, job.ImportedCount)
		}
	}

	return nil
}

func outputJobJSON(job *JobDetails, sources *SourceSummary) error {
	output := map[string]interface{}{
		"job":     job,
		"sources": sources,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputJobHuman(job *JobDetails, sources *SourceSummary) error {
	fmt.Printf("Job: %s\n", job.ID)
	fmt.Println("=" + fmt.Sprintf("%49s", "="))
	fmt.Printf("  Status:      %s\n", job.Status)
	fmt.Printf("  Source Tag:  %s\n", job.SourceTag)
	fmt.Printf("  Created:     %s\n", job.CreatedAt.Format(time.RFC3339))
	if job.CompletedAt != nil {
		fmt.Printf("  Completed:   %s\n", job.CompletedAt.Format(time.RFC3339))
		fmt.Printf("  Duration:    %s\n", job.CompletedAt.Sub(job.CreatedAt).Round(time.Millisecond))
	}
	fmt.Println()

	fmt.Println("Ingest Results")
	fmt.Println("-" + fmt.Sprintf("%49s", "-"))
	fmt.Printf("  Total Files:   %d\n", job.TotalFiles)
	fmt.Printf("  Imported:      \033[32m%d\033[0m\n", job.ImportedCount)
	fmt.Printf("  Skipped:       \033[33m%d\033[0m (duplicates)\n", job.SkippedCount)
	fmt.Printf("  Failed:        \033[31m%d\033[0m\n", job.FailedCount)
	fmt.Println()

	fmt.Println("Source Processing")
	fmt.Println("-" + fmt.Sprintf("%49s", "-"))
	fmt.Printf("  Total Sources: %d\n", sources.Total)
	for status, count := range sources.ByStatus {
		color := "\033[33m"
		if status == "completed" {
			color = "\033[32m"
		} else if status == "failed" {
			color = "\033[31m"
		}
		fmt.Printf("  %s%-12s\033[0m %d\n", color, status, count)
	}

	return nil
}

func outputJobsHuman(jobs []JobSummary) error {
	if len(jobs) == 0 {
		fmt.Println("No ingest jobs found.")
		return nil
	}

	fmt.Println("Recent Ingest Jobs")
	fmt.Println("=" + fmt.Sprintf("%79s", "="))
	fmt.Println("ID                                    STATUS         TAG              FILES  IMPORTED  FAILED")
	fmt.Println("-" + fmt.Sprintf("%79s", "-"))

	for _, job := range jobs {
		statusColor := "\033[32m"
		if job.Status == "failed" {
			statusColor = "\033[31m"
		} else if job.Status == "in_progress" || job.Status == "pending" {
			statusColor = "\033[33m"
		}

		tag := job.SourceTag
		if len(tag) > 16 {
			tag = tag[:13] + "..."
		}

		fmt.Printf("%s  %s%-12s\033[0m  %-16s %5d  %8d  %6d\n",
			job.ID, statusColor, job.Status, tag, job.TotalFiles, job.ImportedCount, job.FailedCount)
	}

	return nil
}
