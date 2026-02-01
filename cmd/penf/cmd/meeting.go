package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	ingestv1 "github.com/otherjamesbrown/penfold/api/proto/ingest/v1"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// Meeting command flags
var (
	meetingOutputFormat string
	meetingSeriesFilter string
	meetingLimit        int32
)

// MeetingCommandDeps holds dependencies for meeting commands.
type MeetingCommandDeps struct {
	Config     *config.CLIConfig
	LoadConfig func() (*config.CLIConfig, error)
}

// DefaultMeetingDeps returns default dependencies for production use.
func DefaultMeetingDeps() *MeetingCommandDeps {
	return &MeetingCommandDeps{
		LoadConfig: config.LoadConfig,
	}
}

// NewMeetingCommand creates the root meeting command with all subcommands.
func NewMeetingCommand(deps *MeetingCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultMeetingDeps()
	}

	cmd := &cobra.Command{
		Use:   "meeting",
		Short: "Manage meetings",
		Long: `Manage meeting records including listing, filtering, and viewing details.

Meetings can be organized into series for recurring meetings.

Examples:
  # List all meetings
  penf meeting list

  # Filter meetings by series
  penf meeting list --series "TER Weekly"

  # Output as JSON
  penf meeting list -o json`,
		Aliases: []string{"meetings"},
	}

	// Add subcommands
	cmd.AddCommand(newMeetingListCommand(deps))
	cmd.AddCommand(newMeetingSeriesCommand(DefaultMeetingSeriesDeps()))
	cmd.AddCommand(newMeetingSetSeriesCommand(DefaultMeetingSeriesDeps()))
	cmd.AddCommand(newMeetingUnsetSeriesCommand(DefaultMeetingSeriesDeps()))

	return cmd
}

// newMeetingListCommand creates the 'meeting list' subcommand.
func newMeetingListCommand(deps *MeetingCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List meetings",
		Long: `List meetings with optional filtering by series.

Displays meetings in reverse chronological order (most recent first).

Examples:
  # List all meetings
  penf meeting list

  # Filter by series name
  penf meeting list --series "TER Weekly"

  # Limit results
  penf meeting list --limit 10

  # Output as JSON
  penf meeting list -o json`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMeetingList(cmd.Context(), deps)
		},
	}

	cmd.Flags().StringVarP(&meetingSeriesFilter, "series", "s", "", "Filter by series name")
	cmd.Flags().Int32VarP(&meetingLimit, "limit", "l", 50, "Maximum number of results")
	cmd.Flags().StringVarP(&meetingOutputFormat, "output", "o", "", "Output format: text, json, yaml")

	return cmd
}

// runMeetingList executes the meeting list command.
func runMeetingList(ctx context.Context, deps *MeetingCommandDeps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Determine output format
	outputFormat := cfg.OutputFormat
	if meetingOutputFormat != "" {
		outputFormat = config.OutputFormat(meetingOutputFormat)
		if !outputFormat.IsValid() {
			return fmt.Errorf("invalid output format: %s", meetingOutputFormat)
		}
	}

	// Connect to gateway via gRPC
	conn, err := connectToGateway(cfg)
	if err != nil {
		return fmt.Errorf("connecting to gateway: %w", err)
	}
	defer conn.Close()

	// Get ingest service client
	ingestClient := ingestv1.NewIngestServiceClient(conn)

	// Call ListMeetings
	resp, err := ingestClient.ListMeetings(ctx, &ingestv1.ListMeetingsRequest{
		SeriesName: meetingSeriesFilter,
		Limit:      meetingLimit,
	})
	if err != nil {
		return fmt.Errorf("listing meetings: %w", err)
	}

	return outputMeetingList(outputFormat, resp.Meetings)
}

// outputMeetingList formats and outputs the meeting list.
func outputMeetingList(format config.OutputFormat, meetings []*ingestv1.MeetingInfo) error {
	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]interface{}{
			"meetings": meetings,
			"count":    len(meetings),
		})
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(map[string]interface{}{
			"meetings": meetings,
			"count":    len(meetings),
		})
	default:
		return outputMeetingListText(meetings)
	}
}

// outputMeetingListText formats meeting list for terminal display.
func outputMeetingListText(meetings []*ingestv1.MeetingInfo) error {
	if len(meetings) == 0 {
		fmt.Println("No meetings found.")
		return nil
	}

	fmt.Printf("Meetings (%d):\n\n", len(meetings))
	fmt.Println("  ID                 TITLE                                         PLATFORM    DATE")
	fmt.Println("  --                 -----                                         --------    ----")

	for _, m := range meetings {
		titleDisplay := m.Title
		if len(titleDisplay) > 45 {
			titleDisplay = titleDisplay[:42] + "..."
		}

		dateDisplay := m.Date
		if len(dateDisplay) > 10 {
			dateDisplay = dateDisplay[:10]
		}

		platformDisplay := m.Platform
		if len(platformDisplay) > 11 {
			platformDisplay = platformDisplay[:11]
		}

		fmt.Printf("  %-18s %-45s %-11s %s\n",
			m.Id, titleDisplay, platformDisplay, dateDisplay)
	}

	fmt.Println()
	return nil
}
