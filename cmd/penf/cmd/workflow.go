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

// WorkflowStatus represents the status of a workflow.
type WorkflowStatus string

const (
	// WorkflowStatusPending indicates workflow is queued.
	WorkflowStatusPending WorkflowStatus = "pending"
	// WorkflowStatusRunning indicates workflow is in progress.
	WorkflowStatusRunning WorkflowStatus = "running"
	// WorkflowStatusCompleted indicates workflow finished successfully.
	WorkflowStatusCompleted WorkflowStatus = "completed"
	// WorkflowStatusFailed indicates workflow encountered an error.
	WorkflowStatusFailed WorkflowStatus = "failed"
	// WorkflowStatusCancelled indicates workflow was cancelled.
	WorkflowStatusCancelled WorkflowStatus = "cancelled"
)

// Workflow represents a workflow instance.
type Workflow struct {
	ID          string            `json:"id" yaml:"id"`
	Type        string            `json:"type" yaml:"type"`
	Name        string            `json:"name" yaml:"name"`
	Status      WorkflowStatus    `json:"status" yaml:"status"`
	Progress    int               `json:"progress" yaml:"progress"`
	Message     string            `json:"message,omitempty" yaml:"message,omitempty"`
	Steps       []WorkflowStep    `json:"steps,omitempty" yaml:"steps,omitempty"`
	Input       map[string]string `json:"input,omitempty" yaml:"input,omitempty"`
	Output      map[string]string `json:"output,omitempty" yaml:"output,omitempty"`
	Error       string            `json:"error,omitempty" yaml:"error,omitempty"`
	CreatedAt   time.Time         `json:"created_at" yaml:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
}

// WorkflowStep represents a step within a workflow.
type WorkflowStep struct {
	Name        string         `json:"name" yaml:"name"`
	Status      WorkflowStatus `json:"status" yaml:"status"`
	Message     string         `json:"message,omitempty" yaml:"message,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
}

// WorkflowListResponse contains workflow list results.
type WorkflowListResponse struct {
	Workflows  []Workflow `json:"workflows" yaml:"workflows"`
	TotalCount int        `json:"total_count" yaml:"total_count"`
	FetchedAt  time.Time  `json:"fetched_at" yaml:"fetched_at"`
}

// WorkflowCommandDeps holds the dependencies for workflow commands.
type WorkflowCommandDeps struct {
	Config       *config.CLIConfig
	GRPCClient   *client.GRPCClient
	OutputFormat config.OutputFormat
	LoadConfig   func() (*config.CLIConfig, error)
	InitClient   func(*config.CLIConfig) (*client.GRPCClient, error)
}

// DefaultWorkflowDeps returns the default dependencies for production use.
func DefaultWorkflowDeps() *WorkflowCommandDeps {
	return &WorkflowCommandDeps{
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

// Workflow command flags.
var (
	workflowType   string
	workflowStatus string
	workflowLimit  int
	workflowOutput string
	workflowWatch  bool
	workflowForce  bool
)

// NewWorkflowCommand creates the root workflow command with all subcommands.
func NewWorkflowCommand(deps *WorkflowCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultWorkflowDeps()
	}

	cmd := &cobra.Command{
		Use:     "workflow",
		Aliases: []string{"wf"},
		Short:   "Manage background workflows and processing jobs",
		Long: `Manage background workflows and processing jobs in Penfold.

Workflows are long-running operations like content ingestion, batch processing,
or scheduled tasks. Use these commands to monitor and manage workflow execution.

Commands:
  list   - List all workflows
  status - Show detailed workflow status
  cancel - Cancel a running workflow

Examples:
  # List recent workflows
  penf workflow list

  # List only running workflows
  penf workflow list --status=running

  # Check status of a specific workflow
  penf workflow status wf-abc123

  # Cancel a running workflow
  penf workflow cancel wf-abc123`,
	}

	// Add subcommands.
	cmd.AddCommand(newWorkflowListCommand(deps))
	cmd.AddCommand(newWorkflowStatusCommand(deps))
	cmd.AddCommand(newWorkflowCancelCommand(deps))

	return cmd
}

// newWorkflowListCommand creates the 'workflow list' subcommand.
func newWorkflowListCommand(deps *WorkflowCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List workflows",
		Long: `List workflows with optional filtering.

By default, shows the most recent workflows across all types and statuses.
Use filters to narrow down the results.

Examples:
  # List all recent workflows
  penf workflow list

  # List only running workflows
  penf workflow list --status=running

  # List ingestion workflows
  penf workflow list --type=ingestion

  # List with custom limit
  penf workflow list --limit=50

  # Output as JSON
  penf workflow list --output=json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowList(cmd.Context(), deps)
		},
	}

	// Define flags.
	cmd.Flags().StringVarP(&workflowType, "type", "t", "", "Filter by workflow type (ingestion, batch, scheduled)")
	cmd.Flags().StringVarP(&workflowStatus, "status", "s", "", "Filter by status (pending, running, completed, failed, cancelled)")
	cmd.Flags().IntVarP(&workflowLimit, "limit", "l", 20, "Maximum number of workflows to show")
	cmd.Flags().StringVarP(&workflowOutput, "output", "o", "", "Output format: text, json, yaml")

	return cmd
}

// newWorkflowStatusCommand creates the 'workflow status' subcommand.
func newWorkflowStatusCommand(deps *WorkflowCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <workflow-id>",
		Short: "Show detailed workflow status",
		Long: `Show detailed status information for a specific workflow.

Displays workflow metadata, current progress, step-by-step execution details,
and any output or errors.

Examples:
  # Get workflow status
  penf workflow status wf-abc123

  # Watch workflow progress
  penf workflow status wf-abc123 --watch

  # Output as JSON
  penf workflow status wf-abc123 --output=json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowStatus(cmd.Context(), deps, args[0])
		},
	}

	// Define flags.
	cmd.Flags().BoolVarP(&workflowWatch, "watch", "w", false, "Watch workflow progress until completion")
	cmd.Flags().StringVarP(&workflowOutput, "output", "o", "", "Output format: text, json, yaml")

	return cmd
}

// newWorkflowCancelCommand creates the 'workflow cancel' subcommand.
func newWorkflowCancelCommand(deps *WorkflowCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cancel <workflow-id>",
		Short: "Cancel a running workflow",
		Long: `Cancel a running or pending workflow.

This will attempt to gracefully stop the workflow. Already completed steps
will not be rolled back. Use --force to immediately terminate the workflow.

Examples:
  # Cancel a workflow
  penf workflow cancel wf-abc123

  # Force cancel (immediate termination)
  penf workflow cancel wf-abc123 --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowCancel(cmd.Context(), deps, args[0])
		},
	}

	// Define flags.
	cmd.Flags().BoolVarP(&workflowForce, "force", "f", false, "Force immediate termination")

	return cmd
}

// runWorkflowList executes the workflow list command.
func runWorkflowList(ctx context.Context, deps *WorkflowCommandDeps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Validate status filter if provided.
	if workflowStatus != "" {
		validStatuses := map[string]bool{
			"pending": true, "running": true, "completed": true,
			"failed": true, "cancelled": true,
		}
		if !validStatuses[workflowStatus] {
			return fmt.Errorf("invalid status: %s (must be pending, running, completed, failed, or cancelled)", workflowStatus)
		}
	}

	// Determine output format.
	outputFormat := cfg.OutputFormat
	if workflowOutput != "" {
		outputFormat = config.OutputFormat(workflowOutput)
		if !outputFormat.IsValid() {
			return fmt.Errorf("invalid output format: %s", workflowOutput)
		}
	}

	// Get workflows (mock implementation until gRPC is connected).
	workflows := getMockWorkflows(workflowType, workflowStatus, workflowLimit)

	response := WorkflowListResponse{
		Workflows:  workflows,
		TotalCount: len(workflows),
		FetchedAt:  time.Now(),
	}

	return outputWorkflowList(outputFormat, response)
}

// runWorkflowStatus executes the workflow status command.
func runWorkflowStatus(ctx context.Context, deps *WorkflowCommandDeps, workflowID string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Determine output format.
	outputFormat := cfg.OutputFormat
	if workflowOutput != "" {
		outputFormat = config.OutputFormat(workflowOutput)
		if !outputFormat.IsValid() {
			return fmt.Errorf("invalid output format: %s", workflowOutput)
		}
	}

	if workflowWatch {
		return runWorkflowWatch(ctx, deps, workflowID, outputFormat)
	}

	// Get workflow status (mock implementation until gRPC is connected).
	workflow := getMockWorkflow(workflowID)
	if workflow == nil {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}

	return outputWorkflowStatus(outputFormat, workflow)
}

// runWorkflowWatch monitors workflow progress until completion.
func runWorkflowWatch(ctx context.Context, deps *WorkflowCommandDeps, workflowID string, outputFormat config.OutputFormat) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		// Clear screen for human-readable output.
		if outputFormat == config.OutputFormatText {
			fmt.Print("\033[H\033[2J")
		}

		workflow := getMockWorkflow(workflowID)
		if workflow == nil {
			return fmt.Errorf("workflow not found: %s", workflowID)
		}

		if err := outputWorkflowStatus(outputFormat, workflow); err != nil {
			return err
		}

		// Check if workflow is terminal.
		if workflow.Status == WorkflowStatusCompleted ||
			workflow.Status == WorkflowStatusFailed ||
			workflow.Status == WorkflowStatusCancelled {
			fmt.Println("\nWorkflow completed.")
			return nil
		}

		select {
		case <-ctx.Done():
			fmt.Println("\nStopped watching.")
			return nil
		case <-ticker.C:
			continue
		}
	}
}

// runWorkflowCancel executes the workflow cancel command.
func runWorkflowCancel(ctx context.Context, deps *WorkflowCommandDeps, workflowID string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Get workflow to verify it exists and is cancellable.
	workflow := getMockWorkflow(workflowID)
	if workflow == nil {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}

	if workflow.Status != WorkflowStatusPending && workflow.Status != WorkflowStatusRunning {
		return fmt.Errorf("workflow %s cannot be cancelled (status: %s)", workflowID, workflow.Status)
	}

	// STUB: Returns mock acknowledgment until workflow service gRPC is connected.
	if workflowForce {
		fmt.Printf("Force cancelling workflow %s...\n", workflowID)
	} else {
		fmt.Printf("Cancelling workflow %s...\n", workflowID)
	}

	fmt.Printf("\nWorkflow %s has been cancelled.\n", workflowID)
	fmt.Println("\n(Note: This is mock data. Connect to the service for real cancellation.)")

	return nil
}

// getMockWorkflows returns mock workflow data.
func getMockWorkflows(typeFilter, statusFilter string, limit int) []Workflow {
	now := time.Now()
	startTime := now.Add(-30 * time.Minute)
	endTime := now.Add(-10 * time.Minute)

	workflows := []Workflow{
		{
			ID:        "wf-ingestion-001",
			Type:      "ingestion",
			Name:      "Email Import Batch",
			Status:    WorkflowStatusRunning,
			Progress:  65,
			Message:   "Processing 150 of 230 emails",
			CreatedAt: now.Add(-45 * time.Minute),
			StartedAt: &startTime,
		},
		{
			ID:          "wf-batch-002",
			Type:        "batch",
			Name:        "Entity Extraction",
			Status:      WorkflowStatusCompleted,
			Progress:    100,
			Message:     "Extracted 342 entities from 85 documents",
			CreatedAt:   now.Add(-2 * time.Hour),
			StartedAt:   &startTime,
			CompletedAt: &endTime,
		},
		{
			ID:        "wf-scheduled-003",
			Type:      "scheduled",
			Name:      "Daily Digest Generation",
			Status:    WorkflowStatusPending,
			Progress:  0,
			Message:   "Scheduled for 18:00",
			CreatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:          "wf-ingestion-004",
			Type:        "ingestion",
			Name:        "Document Upload",
			Status:      WorkflowStatusFailed,
			Progress:    45,
			Message:     "Failed at step 3 of 5",
			Error:       "Connection timeout while uploading to storage",
			CreatedAt:   now.Add(-3 * time.Hour),
			StartedAt:   &startTime,
			CompletedAt: &endTime,
		},
		{
			ID:          "wf-batch-005",
			Type:        "batch",
			Name:        "Embedding Generation",
			Status:      WorkflowStatusCancelled,
			Progress:    30,
			Message:     "Cancelled by user",
			CreatedAt:   now.Add(-4 * time.Hour),
			StartedAt:   &startTime,
			CompletedAt: &endTime,
		},
	}

	// Apply filters.
	var filtered []Workflow
	for _, wf := range workflows {
		if typeFilter != "" && wf.Type != typeFilter {
			continue
		}
		if statusFilter != "" && string(wf.Status) != statusFilter {
			continue
		}
		filtered = append(filtered, wf)
	}

	// Apply limit.
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered
}

// getMockWorkflow returns a mock workflow by ID.
func getMockWorkflow(workflowID string) *Workflow {
	now := time.Now()
	startTime := now.Add(-30 * time.Minute)
	step1Time := now.Add(-28 * time.Minute)
	step2Time := now.Add(-20 * time.Minute)

	// Return a detailed mock workflow.
	if strings.HasPrefix(workflowID, "wf-") {
		return &Workflow{
			ID:       workflowID,
			Type:     "ingestion",
			Name:     "Email Import Batch",
			Status:   WorkflowStatusRunning,
			Progress: 65,
			Message:  "Processing 150 of 230 emails",
			Steps: []WorkflowStep{
				{
					Name:        "Fetch emails from source",
					Status:      WorkflowStatusCompleted,
					Message:     "Retrieved 230 emails",
					StartedAt:   &startTime,
					CompletedAt: &step1Time,
				},
				{
					Name:        "Parse and validate",
					Status:      WorkflowStatusCompleted,
					Message:     "228 emails valid, 2 skipped",
					StartedAt:   &step1Time,
					CompletedAt: &step2Time,
				},
				{
					Name:      "Generate embeddings",
					Status:    WorkflowStatusRunning,
					Message:   "Processing batch 3 of 5",
					StartedAt: &step2Time,
				},
				{
					Name:    "Store in database",
					Status:  WorkflowStatusPending,
					Message: "Waiting for embeddings",
				},
				{
					Name:    "Update search index",
					Status:  WorkflowStatusPending,
					Message: "Waiting for storage",
				},
			},
			Input: map[string]string{
				"source":     "gmail",
				"date_range": "2024-01-01 to 2024-01-15",
				"folder":     "INBOX",
			},
			CreatedAt: now.Add(-45 * time.Minute),
			StartedAt: &startTime,
		}
	}

	return nil
}

// outputWorkflowList formats and outputs the workflow list.
func outputWorkflowList(format config.OutputFormat, response WorkflowListResponse) error {
	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(response)
	default:
		return outputWorkflowListText(response)
	}
}

// outputWorkflowListText formats workflow list for terminal display.
func outputWorkflowListText(response WorkflowListResponse) error {
	if len(response.Workflows) == 0 {
		fmt.Println("No workflows found.")
		return nil
	}

	fmt.Printf("Workflows (%d):\n\n", response.TotalCount)
	fmt.Println("  ID                    TYPE        STATUS       PROGRESS  NAME")
	fmt.Println("  --                    ----        ------       --------  ----")

	for _, wf := range response.Workflows {
		statusColor := getWorkflowStatusColor(wf.Status)
		progressStr := fmt.Sprintf("%d%%", wf.Progress)

		fmt.Printf("  %-21s %-11s %s%-12s\033[0m %8s  %s\n",
			wf.ID, wf.Type, statusColor, wf.Status, progressStr, wf.Name)
	}

	fmt.Println()
	return nil
}

// outputWorkflowStatus formats and outputs workflow status.
func outputWorkflowStatus(format config.OutputFormat, workflow *Workflow) error {
	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(workflow)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(workflow)
	default:
		return outputWorkflowStatusText(workflow)
	}
}

// outputWorkflowStatusText formats workflow status for terminal display.
func outputWorkflowStatusText(workflow *Workflow) error {
	statusColor := getWorkflowStatusColor(workflow.Status)

	fmt.Printf("\033[1mWorkflow: %s\033[0m\n", workflow.ID)
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("  Name:     %s\n", workflow.Name)
	fmt.Printf("  Type:     %s\n", workflow.Type)
	fmt.Printf("  Status:   %s%s\033[0m\n", statusColor, workflow.Status)
	fmt.Printf("  Progress: %d%%\n", workflow.Progress)
	if workflow.Message != "" {
		fmt.Printf("  Message:  %s\n", workflow.Message)
	}
	if workflow.Error != "" {
		fmt.Printf("  Error:    \033[31m%s\033[0m\n", workflow.Error)
	}
	fmt.Println()

	// Timestamps.
	fmt.Printf("  Created:  %s\n", workflow.CreatedAt.Format(time.RFC3339))
	if workflow.StartedAt != nil {
		fmt.Printf("  Started:  %s\n", workflow.StartedAt.Format(time.RFC3339))
	}
	if workflow.CompletedAt != nil {
		fmt.Printf("  Finished: %s\n", workflow.CompletedAt.Format(time.RFC3339))
		duration := workflow.CompletedAt.Sub(*workflow.StartedAt)
		fmt.Printf("  Duration: %s\n", duration.Round(time.Second))
	} else if workflow.StartedAt != nil {
		duration := time.Since(*workflow.StartedAt)
		fmt.Printf("  Elapsed:  %s\n", duration.Round(time.Second))
	}
	fmt.Println()

	// Input parameters.
	if len(workflow.Input) > 0 {
		fmt.Println("  Input:")
		for k, v := range workflow.Input {
			fmt.Printf("    %s: %s\n", k, v)
		}
		fmt.Println()
	}

	// Steps.
	if len(workflow.Steps) > 0 {
		fmt.Println("  Steps:")
		for i, step := range workflow.Steps {
			stepColor := getWorkflowStatusColor(step.Status)
			statusIcon := getStepStatusIcon(step.Status)
			fmt.Printf("    %d. %s %s%s\033[0m - %s\n",
				i+1, statusIcon, stepColor, step.Name, step.Message)
		}
		fmt.Println()
	}

	// Output.
	if len(workflow.Output) > 0 {
		fmt.Println("  Output:")
		for k, v := range workflow.Output {
			fmt.Printf("    %s: %s\n", k, v)
		}
		fmt.Println()
	}

	return nil
}

// getWorkflowStatusColor returns ANSI color code for workflow status.
func getWorkflowStatusColor(status WorkflowStatus) string {
	switch status {
	case WorkflowStatusCompleted:
		return "\033[32m" // Green
	case WorkflowStatusRunning:
		return "\033[34m" // Blue
	case WorkflowStatusPending:
		return "\033[33m" // Yellow
	case WorkflowStatusFailed:
		return "\033[31m" // Red
	case WorkflowStatusCancelled:
		return "\033[90m" // Gray
	default:
		return ""
	}
}

// getStepStatusIcon returns an icon for step status.
func getStepStatusIcon(status WorkflowStatus) string {
	switch status {
	case WorkflowStatusCompleted:
		return "\033[32m[OK]\033[0m"
	case WorkflowStatusRunning:
		return "\033[34m[..]\033[0m"
	case WorkflowStatusPending:
		return "\033[33m[  ]\033[0m"
	case WorkflowStatusFailed:
		return "\033[31m[!!]\033[0m"
	case WorkflowStatusCancelled:
		return "\033[90m[--]\033[0m"
	default:
		return "[??]"
	}
}
