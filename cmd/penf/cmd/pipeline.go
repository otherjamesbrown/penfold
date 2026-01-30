// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	"github.com/otherjamesbrown/penfold/cmd/penf/client"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// PipelineCommandDeps holds the dependencies for pipeline commands.
type PipelineCommandDeps struct {
	Config     *config.CLIConfig
	LoadConfig func() (*config.CLIConfig, error)
}

// DefaultPipelineDeps returns the default dependencies for production use.
func DefaultPipelineDeps() *PipelineCommandDeps {
	return &PipelineCommandDeps{
		LoadConfig: config.LoadConfig,
	}
}

// NewPipelineCommand creates the pipeline command.
func NewPipelineCommand(deps interface{}) *cobra.Command {
	// Accept either PipelineCommandDeps or nil
	var pipelineDeps *PipelineCommandDeps
	if d, ok := deps.(*PipelineCommandDeps); ok && d != nil {
		pipelineDeps = d
	} else {
		pipelineDeps = DefaultPipelineDeps()
	}

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
  jobs    - List recent ingest jobs

Documentation:
  Pipeline concepts:   docs/concepts/pipeline.md
  System vision:       docs/shared/vision.md`,
	}

	cmd.AddCommand(newPipelineStatusCmd(pipelineDeps))
	cmd.AddCommand(newPipelineJobCmd(pipelineDeps))
	cmd.AddCommand(newPipelineJobsCmd(pipelineDeps))

	return cmd
}

func newPipelineStatusCmd(deps *PipelineCommandDeps) *cobra.Command {
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
			return runPipelineStatus(cmd.Context(), deps, outputFormat)
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")
	return cmd
}

func newPipelineJobCmd(deps *PipelineCommandDeps) *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "job <job-id>",
		Short: "Show details for a specific ingest job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPipelineJob(cmd.Context(), deps, args[0], outputFormat)
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")
	return cmd
}

func newPipelineJobsCmd(deps *PipelineCommandDeps) *cobra.Command {
	var limit int
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "List recent ingest jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPipelineJobs(cmd.Context(), deps, limit, outputFormat)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 10, "Maximum number of jobs to show")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")
	return cmd
}

// connectPipelineToGateway creates a gRPC connection to the gateway service.
func connectPipelineToGateway(cfg *config.CLIConfig) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	opts := []grpc.DialOption{
		grpc.WithBlock(),
	}

	if cfg.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else if cfg.TLS.Enabled {
		tlsConfig, err := client.LoadClientTLSConfig(&cfg.TLS)
		if err != nil {
			return nil, fmt.Errorf("loading TLS config: %w", err)
		}
		if tlsConfig != nil {
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		} else {
			opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.DialContext(ctx, cfg.ServerAddress, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to gateway at %s: %w", cfg.ServerAddress, err)
	}

	return conn, nil
}

// Command execution functions

func runPipelineStatus(ctx context.Context, deps *PipelineCommandDeps, outputFormat string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	conn, err := connectPipelineToGateway(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pipelinev1.NewPipelineServiceClient(conn)

	resp, err := client.GetStats(ctx, &pipelinev1.GetStatsRequest{})
	if err != nil {
		return fmt.Errorf("getting pipeline stats: %w", err)
	}

	if outputFormat == "json" {
		return outputPipelineStatsJSON(resp.Stats)
	}
	return outputPipelineStatsHuman(resp.Stats)
}

func runPipelineJob(ctx context.Context, deps *PipelineCommandDeps, jobID string, outputFormat string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	conn, err := connectPipelineToGateway(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pipelinev1.NewPipelineServiceClient(conn)

	resp, err := client.GetJob(ctx, &pipelinev1.GetJobRequest{JobId: jobID})
	if err != nil {
		return fmt.Errorf("getting job: %w", err)
	}

	if outputFormat == "json" {
		return outputPipelineJobJSON(resp.Job, resp.Sources)
	}
	return outputPipelineJobHuman(resp.Job, resp.Sources)
}

func runPipelineJobs(ctx context.Context, deps *PipelineCommandDeps, limit int, outputFormat string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	conn, err := connectPipelineToGateway(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pipelinev1.NewPipelineServiceClient(conn)

	resp, err := client.ListJobs(ctx, &pipelinev1.ListJobsRequest{Limit: int32(limit)})
	if err != nil {
		return fmt.Errorf("listing jobs: %w", err)
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp.Jobs)
	}
	return outputPipelineJobsHuman(resp.Jobs)
}

// Output functions

func outputPipelineStatsJSON(stats *pipelinev1.PipelineStats) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(stats)
}

func outputPipelineStatsHuman(stats *pipelinev1.PipelineStats) error {
	fmt.Println("Pipeline Status")
	fmt.Println("=" + fmt.Sprintf("%49s", "="))
	if stats.Timestamp != nil {
		fmt.Printf("  Timestamp: %s\n\n", stats.Timestamp.AsTime().Format(time.RFC3339))
	}

	// Sources
	fmt.Println("Sources")
	fmt.Println("-" + fmt.Sprintf("%49s", "-"))
	fmt.Printf("  Total: %d\n", stats.SourcesTotal)
	if len(stats.SourcesByStatus) > 0 {
		fmt.Println("  By Status:")
		for _, sc := range stats.SourcesByStatus {
			color := "\033[33m" // Yellow for pending
			if sc.Status == "completed" {
				color = "\033[32m" // Green
			} else if sc.Status == "failed" {
				color = "\033[31m" // Red
			}
			fmt.Printf("    %s%-12s\033[0m %d\n", color, sc.Status, sc.Count)
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
		for _, sc := range stats.AttachmentsByTier {
			fmt.Printf("    %-16s %d\n", sc.Status, sc.Count)
		}
	}
	fmt.Println()

	// Jobs
	fmt.Println("Ingest Jobs")
	fmt.Println("-" + fmt.Sprintf("%49s", "-"))
	fmt.Printf("  Total: %d\n", stats.JobsTotal)
	if len(stats.JobsByStatus) > 0 {
		fmt.Println("  By Status:")
		for _, sc := range stats.JobsByStatus {
			fmt.Printf("    %-16s %d\n", sc.Status, sc.Count)
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
				job.Id, statusColor, job.Status, job.TotalFiles, job.ImportedCount)
		}
	}

	return nil
}

func outputPipelineJobJSON(job *pipelinev1.JobDetails, sources *pipelinev1.SourceStats) error {
	output := map[string]interface{}{
		"job":     job,
		"sources": sources,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputPipelineJobHuman(job *pipelinev1.JobDetails, sources *pipelinev1.SourceStats) error {
	summary := job.Summary
	if summary == nil {
		return fmt.Errorf("job summary is nil")
	}

	fmt.Printf("Job: %s\n", summary.Id)
	fmt.Println("=" + fmt.Sprintf("%49s", "="))
	fmt.Printf("  Status:      %s\n", summary.Status)
	fmt.Printf("  Source Tag:  %s\n", summary.SourceTag)
	if summary.CreatedAt != nil {
		fmt.Printf("  Created:     %s\n", summary.CreatedAt.AsTime().Format(time.RFC3339))
	}
	if summary.CompletedAt != nil {
		completedAt := summary.CompletedAt.AsTime()
		fmt.Printf("  Completed:   %s\n", completedAt.Format(time.RFC3339))
		if summary.CreatedAt != nil {
			duration := completedAt.Sub(summary.CreatedAt.AsTime())
			fmt.Printf("  Duration:    %s\n", duration.Round(time.Millisecond))
		}
	}
	fmt.Println()

	fmt.Println("Ingest Results")
	fmt.Println("-" + fmt.Sprintf("%49s", "-"))
	fmt.Printf("  Total Files:   %d\n", summary.TotalFiles)
	fmt.Printf("  Imported:      \033[32m%d\033[0m\n", summary.ImportedCount)
	fmt.Printf("  Skipped:       \033[33m%d\033[0m (duplicates)\n", summary.SkippedCount)
	fmt.Printf("  Failed:        \033[31m%d\033[0m\n", summary.FailedCount)
	fmt.Println()

	if sources != nil {
		fmt.Println("Source Processing")
		fmt.Println("-" + fmt.Sprintf("%49s", "-"))
		fmt.Printf("  Total Sources: %d\n", sources.Total)
		for _, sc := range sources.ByStatus {
			color := "\033[33m"
			if sc.Status == "completed" {
				color = "\033[32m"
			} else if sc.Status == "failed" {
				color = "\033[31m"
			}
			fmt.Printf("  %s%-12s\033[0m %d\n", color, sc.Status, sc.Count)
		}
	}

	return nil
}

func outputPipelineJobsHuman(jobs []*pipelinev1.JobSummary) error {
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
			job.Id, statusColor, job.Status, tag, job.TotalFiles, job.ImportedCount, job.FailedCount)
	}

	return nil
}
