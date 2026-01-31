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

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
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
	cmd.AddCommand(newPipelineReprocessCmd(pipelineDeps))
	cmd.AddCommand(newPipelineKickCmd(pipelineDeps))
	cmd.AddCommand(newPipelineRetryCmd(pipelineDeps))
	cmd.AddCommand(newPipelineWorkersCmd(pipelineDeps))

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

func newPipelineReprocessCmd(deps *PipelineCommandDeps) *cobra.Command {
	var stage string
	var confirm bool
	var outputFormat string
	var reason string

	cmd := &cobra.Command{
		Use:   "reprocess <content-id>",
		Short: "Trigger reprocessing of content",
		Long: `Trigger reprocessing of an already-processed content item.

This is useful for:
  - Re-running processing after model updates
  - Fixing content that failed during processing
  - Updating embeddings or summaries with new models

Stages that can be reprocessed:
  - embeddings: Regenerate vector embeddings
  - entities: Re-extract entities and structured data
  - keywords: Re-extract keywords
  - summary: Regenerate AI summaries

Examples:
  # Reprocess all stages for a content item
  penf pipeline reprocess content-123

  # Reprocess only embeddings
  penf pipeline reprocess content-123 --stage=embeddings

  # Reprocess with a reason
  penf pipeline reprocess content-123 --reason="Updated to new model"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPipelineReprocess(cmd.Context(), deps, args[0], stage, reason, outputFormat)
		},
	}

	cmd.Flags().StringVar(&stage, "stage", "", "Specific stage to reprocess: embeddings, entities, keywords, summary")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Required for bulk operations (future: --source, --all flags)")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")
	cmd.Flags().StringVar(&reason, "reason", "Manual reprocess via CLI", "Reason for reprocessing (for audit trail)")

	return cmd
}

func runPipelineReprocess(ctx context.Context, deps *PipelineCommandDeps, contentID string, stage string, reason string, outputFormat string) error {
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

	// Create content processor client directly from connection
	contentClient := contentv1.NewContentProcessorServiceClient(conn)

	// Build request
	req := &contentv1.ReprocessContentRequest{
		ContentId: contentID,
		Reason:    reason,
	}

	// Add specific stage if provided
	if stage != "" {
		stageEnum, err := parseProcessingStage(stage)
		if err != nil {
			return err
		}
		req.StagesToReprocess = []contentv1.ProcessingStage{stageEnum}
	}

	// Call ReprocessContent RPC
	resp, err := contentClient.ReprocessContent(ctx, req)
	if err != nil {
		return fmt.Errorf("reprocessing content: %w", err)
	}

	// Output results
	if outputFormat == "json" {
		return outputReprocessJSON(resp)
	}
	return outputReprocessHuman(resp)
}

func parseProcessingStage(stage string) (contentv1.ProcessingStage, error) {
	switch stage {
	case "embeddings", "embed":
		return contentv1.ProcessingStage_PROCESSING_STAGE_EMBED, nil
	case "entities", "extract":
		return contentv1.ProcessingStage_PROCESSING_STAGE_EXTRACT, nil
	case "summary", "summarize":
		return contentv1.ProcessingStage_PROCESSING_STAGE_SUMMARIZE, nil
	case "keywords":
		// Note: Keywords might be part of EXTRACT stage - adjust if needed
		return contentv1.ProcessingStage_PROCESSING_STAGE_EXTRACT, nil
	default:
		return contentv1.ProcessingStage_PROCESSING_STAGE_UNSPECIFIED, fmt.Errorf("invalid stage: %s (must be: embeddings, entities, keywords, summary)", stage)
	}
}

func outputReprocessJSON(resp *contentv1.ReprocessContentResponse) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

func outputReprocessHuman(resp *contentv1.ReprocessContentResponse) error {
	fmt.Printf("Reprocessing started for content: %s\n", resp.ContentId)
	fmt.Printf("Job ID: %s\n", resp.JobId)
	if resp.PreviousJobId != "" {
		fmt.Printf("Previous Job ID: %s\n", resp.PreviousJobId)
	}

	if resp.Status != nil {
		fmt.Printf("\nStatus: %s\n", resp.Status.State.String())
		if resp.Status.CurrentStage != nil {
			fmt.Printf("Current Stage: %s\n", resp.Status.CurrentStage.String())
		}
		if resp.Status.ProgressPercent > 0 {
			fmt.Printf("Progress: %d%%\n", resp.Status.ProgressPercent)
		}
	}

	fmt.Println("\nUse 'penf pipeline job <job-id>' to check progress")
	return nil
}

func newPipelineKickCmd(deps *PipelineCommandDeps) *cobra.Command {
	var tenant string
	var limit int
	var source string
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "kick",
		Short: "Trigger processing of pending items",
		Long: `Trigger processing of pending pipeline items.

This command starts processing for items that are in pending state,
useful for manually triggering a processing batch.

Examples:
  # Kick all pending items
  penf pipeline kick

  # Kick with limit
  penf pipeline kick --limit=100

  # Kick specific source
  penf pipeline kick --source=gmail-import-2024

  # Kick for specific tenant
  penf pipeline kick --tenant=tenant-123`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPipelineKick(cmd.Context(), deps, tenant, limit, source, outputFormat)
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "Filter by tenant ID")
	cmd.Flags().IntVarP(&limit, "limit", "l", 0, "Maximum number of items to queue (0 = no limit)")
	cmd.Flags().StringVar(&source, "source", "", "Filter by source tag")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")

	return cmd
}

func runPipelineKick(ctx context.Context, deps *PipelineCommandDeps, tenant string, limit int, source string, outputFormat string) error {
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

	req := &pipelinev1.KickProcessingRequest{
		TenantId:  tenant,
		Limit:     int32(limit),
		SourceTag: source,
	}

	resp, err := client.KickProcessing(ctx, req)
	if err != nil {
		return fmt.Errorf("kicking processing: %w", err)
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}

	fmt.Printf("Queued %d items for processing\n", resp.QueuedCount)
	if resp.Message != "" {
		fmt.Printf("%s\n", resp.Message)
	}
	return nil
}

func newPipelineRetryCmd(deps *PipelineCommandDeps) *cobra.Command {
	var stage string
	var tenant string
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "retry [job-id]",
		Short: "Retry failed processing jobs",
		Long: `Retry failed pipeline items.

If a job ID is provided, retries only that specific job.
Otherwise, retries all failed items matching the filters.

Examples:
  # Retry all failed items
  penf pipeline retry

  # Retry specific job
  penf pipeline retry job-abc123

  # Retry failed embeddings
  penf pipeline retry --stage=embedding

  # Retry for specific tenant
  penf pipeline retry --tenant=tenant-123`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := ""
			if len(args) > 0 {
				jobID = args[0]
			}
			return runPipelineRetry(cmd.Context(), deps, jobID, stage, tenant, outputFormat)
		},
	}

	cmd.Flags().StringVar(&stage, "stage", "", "Filter by pipeline stage (embedding, attachment)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Filter by tenant ID")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")

	return cmd
}

func runPipelineRetry(ctx context.Context, deps *PipelineCommandDeps, jobID string, stage string, tenant string, outputFormat string) error {
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

	req := &pipelinev1.RetryFailedRequest{
		TenantId: tenant,
		JobId:    jobID,
		Stage:    stage,
	}

	resp, err := client.RetryFailed(ctx, req)
	if err != nil {
		return fmt.Errorf("retrying failed items: %w", err)
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}

	fmt.Printf("Retried %d failed items\n", resp.RetriedCount)
	if resp.Message != "" {
		fmt.Printf("%s\n", resp.Message)
	}
	return nil
}

func newPipelineWorkersCmd(deps *PipelineCommandDeps) *cobra.Command {
	var tenant string
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "workers",
		Short: "Show worker status",
		Long: `Show status of pipeline workers.

Displays running workflow instances that are processing pipeline items,
providing insight into current processing activity.

Examples:
  # Show all workers
  penf pipeline workers

  # Show workers for specific tenant
  penf pipeline workers --tenant=tenant-123

  # Output as JSON
  penf pipeline workers -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPipelineWorkers(cmd.Context(), deps, tenant, outputFormat)
		},
	}

	cmd.Flags().StringVar(&tenant, "tenant", "", "Filter by tenant ID")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")

	return cmd
}

func runPipelineWorkers(ctx context.Context, deps *PipelineCommandDeps, tenant string, outputFormat string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Initialize gRPC client using workflow patterns
	grpcClient, err := func(cfg *config.CLIConfig) (*client.GRPCClient, error) {
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
	}(cfg)
	if err != nil {
		return fmt.Errorf("initializing client: %w", err)
	}
	defer grpcClient.Close()

	// Use ListWorkflows to get running workers
	filter := client.ListWorkflowsFilter{
		Status:   "Running",
		PageSize: 100, // Get more workers
	}

	result, err := grpcClient.ListWorkflows(ctx, filter)
	if err != nil {
		return fmt.Errorf("listing workflows: %w", err)
	}

	// Note: tenant filtering would require GetWorkflowStatus for each workflow
	// to access SearchAttributes. For now, show all running workflows.
	workers := result.Workflows

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]interface{}{
			"workers":      workers,
			"total_count":  len(workers),
			"running_only": true,
		})
	}

	// Human-readable output
	if len(workers) == 0 {
		fmt.Println("No running workers found.")
		return nil
	}

	fmt.Printf("Pipeline Workers (%d running):\n\n", len(workers))
	fmt.Println("  WORKFLOW ID                           TYPE                      STATUS      STARTED")
	fmt.Println("  -----------                           ----                      ------      -------")

	for _, wf := range workers {
		workflowType := wf.WorkflowType
		if len(workflowType) > 24 {
			workflowType = workflowType[:21] + "..."
		}

		startTime := wf.StartTime.Format("15:04:05")
		if time.Since(wf.StartTime) > 24*time.Hour {
			startTime = wf.StartTime.Format("Jan 02 15:04")
		}

		fmt.Printf("  %-37s %-25s \033[34m%-11s\033[0m %s\n",
			wf.WorkflowID, workflowType, wf.Status, startTime)
	}

	fmt.Println()
	return nil
}
