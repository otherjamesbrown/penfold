// Package cmd provides CLI command implementations.
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	projectv1 "github.com/otherjamesbrown/penfold/api/proto/project/v1"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// newContextProjectCommand creates the context project subcommand.
func newContextProjectCommand(deps *ContextCommandDeps) *cobra.Command {
	var projectName string

	cmd := &cobra.Command{
		Use:   "project --name <name>",
		Short: "Get comprehensive project context for drill-down",
		Long: `Retrieve comprehensive context about a project including recent meetings,
open actions, risks, and recent decisions.

This is a drill-down command that aggregates project data from multiple sources.

Examples:
  penf context project --name "MTC 2026"
  penf context project --name "MTC 2026" -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runContextProject(cmd.Context(), deps, projectName)
		},
	}

	cmd.Flags().StringVar(&projectName, "name", "", "Project name (required)")
	cmd.MarkFlagRequired("name")

	return cmd
}

// runContextProject executes the context project command.
func runContextProject(ctx context.Context, deps *ContextCommandDeps, projectName string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Create ProjectCommandDeps for reusing project command utilities
	projectDeps := &ProjectCommandDeps{
		Config:     cfg,
		LoadConfig: deps.LoadConfig,
	}

	conn, err := connectProjectToGateway(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := projectv1.NewProjectServiceClient(conn)
	tenantID := getTenantIDForProject(projectDeps)

	resp, err := client.GetProjectContext(ctx, &projectv1.GetProjectContextRequest{
		TenantId: tenantID,
		Name:     projectName,
	})
	if err != nil {
		return fmt.Errorf("getting project context: %w", err)
	}

	return outputProjectContext(cfg, resp)
}

// outputProjectContext outputs the project context in the configured format.
func outputProjectContext(cfg *config.CLIConfig, ctx *projectv1.GetProjectContextResponse) error {
	format := getProjectOutputFormat(cfg)

	switch format {
	case config.OutputFormatJSON:
		return outputProjectJSON(ctx)
	case config.OutputFormatYAML:
		return outputProjectYAML(ctx)
	default:
		return outputProjectContextText(ctx)
	}
}

// outputProjectContextText outputs project context in human-readable format.
func outputProjectContextText(ctx *projectv1.GetProjectContextResponse) error {
	if ctx.Project == nil {
		return fmt.Errorf("no project data")
	}

	fmt.Printf("\033[1mProject:\033[0m %s\n", ctx.Project.Name)
	if ctx.Project.Description != "" {
		fmt.Printf("\033[1mDescription:\033[0m %s\n", ctx.Project.Description)
	}
	fmt.Printf("\033[1mStatus:\033[0m %s\n", ctx.Project.Status)
	fmt.Println()

	fmt.Printf("\033[1mRecent Activity:\033[0m\n")
	fmt.Printf("  Recent meetings: %d\n", ctx.RecentMeetingCount)
	fmt.Printf("  Open actions:    %d\n", ctx.OpenActionCount)
	fmt.Printf("  Risks:           %d\n", ctx.RiskCount)
	fmt.Println()

	if len(ctx.RecentDecisions) > 0 {
		fmt.Printf("\033[1mRecent Decisions:\033[0m\n")
		for i, decision := range ctx.RecentDecisions {
			if i >= 5 {
				break
			}
			fmt.Printf("  - %s\n", decision)
		}
		fmt.Println()
	}

	if ctx.LastActivity != "" {
		fmt.Printf("\033[1mLast Activity:\033[0m %s\n", ctx.LastActivity)
	}

	return nil
}
