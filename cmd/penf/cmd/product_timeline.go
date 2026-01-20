// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/pkg/products"
)

// Timeline/Event command flags.
var (
	// Timeline list filters
	timelineType       string
	timelineVisibility string
	timelineSince      string
	timelineUntil      string
	timelineLimit      int

	// Event add fields
	eventAddType        string
	eventAddVisibility  string
	eventAddTitle       string
	eventAddDescription string
	eventAddOccurred    string
	eventAddRecordedBy  string

	// Event link fields
	eventLinkType string

	// Context window
	eventContextWindow int
)

// addProductTimelineCommands adds the timeline and event subcommands to the product command.
func addProductTimelineCommands(productCmd *cobra.Command, deps *ProductCommandDeps) {
	productCmd.AddCommand(newProductTimelineCommand(deps))
	productCmd.AddCommand(newProductEventCommand(deps))
}

// newProductTimelineCommand creates the 'product timeline' subcommand.
func newProductTimelineCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timeline <product>",
		Short: "Show product timeline events",
		Long: `Show timeline events for a product.

Displays decisions, milestones, risks, releases, and other events
associated with a product in chronological order.

Event Types:
  - decision:   Key decisions made about the product
  - milestone:  Important milestones achieved
  - risk:       Identified risks or issues
  - release:    Product releases
  - competitor: Competitor actions or market changes
  - org_change: Organizational changes affecting the product
  - market:     Market events or trends
  - note:       General notes

Examples:
  # Show all events for a product
  penf product timeline "My Product"

  # Show only decisions
  penf product timeline "My Product" --type decision

  # Show events from last 30 days
  penf product timeline "My Product" --since 30d

  # Show external (competitor/market) events
  penf product timeline "My Product" --visibility external

  # Limit results
  penf product timeline "My Product" --limit 10`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductTimeline(cmd.Context(), deps, args[0])
		},
	}

	cmd.Flags().StringVar(&timelineType, "type", "", "Filter by event type (decision, milestone, risk, release, competitor, org_change, market, note)")
	cmd.Flags().StringVar(&timelineVisibility, "visibility", "", "Filter by visibility (internal, external)")
	cmd.Flags().StringVar(&timelineSince, "since", "", "Show events since date (YYYY-MM-DD or relative: 7d, 30d, 1y)")
	cmd.Flags().StringVar(&timelineUntil, "until", "", "Show events until date (YYYY-MM-DD or relative: 7d, 30d, 1y)")
	cmd.Flags().IntVar(&timelineLimit, "limit", 50, "Maximum number of events to show")

	return cmd
}

// newProductEventCommand creates the 'product event' subcommand group.
func newProductEventCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Manage product timeline events",
		Long: `Manage timeline events for products.

Events track decisions, milestones, risks, releases, and other
important occurrences in a product's lifecycle.`,
	}

	cmd.AddCommand(newProductEventAddCommand(deps))
	cmd.AddCommand(newProductEventShowCommand(deps))
	cmd.AddCommand(newProductEventDeleteCommand(deps))
	cmd.AddCommand(newProductEventLinkCommand(deps))
	cmd.AddCommand(newProductEventContextCommand(deps))

	return cmd
}

// newProductEventAddCommand creates the 'product event add' subcommand.
func newProductEventAddCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <product>",
		Short: "Add a timeline event",
		Long: `Add a new event to a product's timeline.

Event Types:
  - decision:   Key decision (e.g., "Chose PostgreSQL for database")
  - milestone:  Achievement (e.g., "GA Release")
  - risk:       Risk or issue (e.g., "Security vulnerability found")
  - release:    Product release (e.g., "v2.0.0 released")
  - competitor: Competitor action (e.g., "Competitor X launched similar feature")
  - org_change: Org change (e.g., "New DRI assigned")
  - market:     Market event (e.g., "Industry report published")
  - note:       General note

Examples:
  # Add a decision
  penf product event add "My Product" \
    --type decision \
    --title "Selected Go as primary language" \
    --description "After evaluating Rust and Python, chose Go for better concurrency"

  # Add a milestone with specific date
  penf product event add "My Product" \
    --type milestone \
    --title "GA Release" \
    --occurred 2024-01-15

  # Add an external event (competitor)
  penf product event add "My Product" \
    --type competitor \
    --visibility external \
    --title "Competitor X announced similar feature"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductEventAdd(cmd.Context(), deps, args[0])
		},
	}

	cmd.Flags().StringVar(&eventAddType, "type", "note", "Event type (decision, milestone, risk, release, competitor, org_change, market, note)")
	cmd.Flags().StringVar(&eventAddVisibility, "visibility", "internal", "Visibility (internal, external)")
	cmd.Flags().StringVar(&eventAddTitle, "title", "", "Event title (required)")
	cmd.Flags().StringVar(&eventAddDescription, "description", "", "Event description")
	cmd.Flags().StringVar(&eventAddOccurred, "occurred", "", "When the event occurred (YYYY-MM-DD, default: now)")
	cmd.Flags().StringVar(&eventAddRecordedBy, "recorded-by", "", "Who recorded this event (email)")

	_ = cmd.MarkFlagRequired("title")

	return cmd
}

// newProductEventShowCommand creates the 'product event show' subcommand.
func newProductEventShowCommand(deps *ProductCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <event-id>",
		Short: "Show event details",
		Long: `Show detailed information about a specific event.

Example:
  penf product event show 123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductEventShow(cmd.Context(), deps, args[0])
		},
	}
}

// newProductEventDeleteCommand creates the 'product event delete' subcommand.
func newProductEventDeleteCommand(deps *ProductCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <event-id>",
		Short: "Delete an event",
		Long: `Delete an event from a product's timeline.

Example:
  penf product event delete 123`,
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductEventDelete(cmd.Context(), deps, args[0])
		},
	}
}

// newProductEventLinkCommand creates the 'product event link' subcommand.
func newProductEventLinkCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link <event-id> <entity-type> <entity-id>",
		Short: "Link an event to another entity",
		Long: `Link an event to a meeting, email, document, or other entity.

Entity Types:
  - meeting:  Link to a meeting record
  - email:    Link to an email
  - document: Link to a document
  - source:   Link to a source record

Link Types:
  - source:    The entity is the source of this event
  - reference: The entity is referenced by this event
  - follow_up: This event is a follow-up to the entity

Examples:
  # Link event to a meeting as source
  penf product event link 123 meeting 456 --link-type source

  # Link event to an email as reference
  penf product event link 123 email 789 --link-type reference`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductEventLink(cmd.Context(), deps, args[0], args[1], args[2])
		},
	}

	cmd.Flags().StringVar(&eventLinkType, "link-type", "reference", "Link type (source, reference, follow_up)")

	return cmd
}

// newProductEventContextCommand creates the 'product event context' subcommand.
func newProductEventContextCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context <product> <date>",
		Short: "Show events around a specific date",
		Long: `Show events around a specific date for context anchoring.

This helps answer questions like "what was happening when we made decision X?"

Examples:
  # Show events around a specific date
  penf product event context "My Product" 2024-01-15

  # Show 14-day window (default is 7)
  penf product event context "My Product" 2024-01-15 --window 14`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductEventContext(cmd.Context(), deps, args[0], args[1])
		},
	}

	cmd.Flags().IntVar(&eventContextWindow, "window", 7, "Number of days before and after the center date")

	return cmd
}

// ==================== Command Execution Functions ====================

// runProductTimeline lists events for a product.
func runProductTimeline(ctx context.Context, deps *ProductCommandDeps, productName string) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)

	// Resolve product.
	product, err := deps.Repository.ResolveProduct(ctx, tenantID, productName)
	if err != nil {
		return fmt.Errorf("product not found: %s", productName)
	}

	// Build filter.
	filter := products.EventFilter{
		TenantID:  tenantID,
		ProductID: &product.ID,
		Limit:     timelineLimit,
	}

	// Parse event type filter.
	if timelineType != "" {
		et := products.EventType(timelineType)
		filter.EventTypes = []products.EventType{et}
	}

	// Parse visibility filter.
	if timelineVisibility != "" {
		v := products.EventVisibility(timelineVisibility)
		filter.Visibility = &v
	}

	// Parse date filters.
	if timelineSince != "" {
		since, err := parseRelativeDate(timelineSince)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		filter.Since = &since
	}

	if timelineUntil != "" {
		until, err := parseRelativeDate(timelineUntil)
		if err != nil {
			return fmt.Errorf("invalid --until value: %w", err)
		}
		filter.Until = &until
	}

	// Get events.
	events, err := deps.Repository.ListEvents(ctx, filter)
	if err != nil {
		return fmt.Errorf("listing events: %w", err)
	}

	return outputProductTimeline(deps.Config, product.Name, events)
}

// runProductEventAdd adds a new event.
func runProductEventAdd(ctx context.Context, deps *ProductCommandDeps, productName string) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)

	// Resolve product.
	product, err := deps.Repository.ResolveProduct(ctx, tenantID, productName)
	if err != nil {
		return fmt.Errorf("product not found: %s", productName)
	}

	// Validate event type.
	et := products.EventType(eventAddType)
	switch et {
	case products.EventTypeDecision, products.EventTypeMilestone, products.EventTypeRisk,
		products.EventTypeRelease, products.EventTypeCompetitor, products.EventTypeOrgChange,
		products.EventTypeMarket, products.EventTypeNote:
		// Valid.
	default:
		return fmt.Errorf("invalid event type: %s", eventAddType)
	}

	// Validate visibility.
	vis := products.EventVisibility(eventAddVisibility)
	switch vis {
	case products.EventVisibilityInternal, products.EventVisibilityExternal:
		// Valid.
	default:
		return fmt.Errorf("invalid visibility: %s", eventAddVisibility)
	}

	// Parse occurred date.
	occurredAt := time.Now()
	if eventAddOccurred != "" {
		var err error
		occurredAt, err = time.Parse("2006-01-02", eventAddOccurred)
		if err != nil {
			return fmt.Errorf("invalid --occurred date (use YYYY-MM-DD): %w", err)
		}
	}

	// Create event.
	event := &products.ProductEvent{
		EventUUID:   uuid.New(),
		TenantID:    tenantID,
		ProductID:   product.ID,
		EventType:   et,
		Visibility:  vis,
		SourceType:  products.EventSourceManual,
		Title:       eventAddTitle,
		Description: eventAddDescription,
		OccurredAt:  occurredAt,
		RecordedBy:  eventAddRecordedBy,
	}

	if err := deps.Repository.CreateEvent(ctx, event); err != nil {
		return fmt.Errorf("creating event: %w", err)
	}

	fmt.Printf("\033[32mCreated event:\033[0m %s (ID: %d)\n", event.Title, event.ID)
	fmt.Printf("  Product: %s\n", product.Name)
	fmt.Printf("  Type: %s\n", event.EventType)
	fmt.Printf("  Occurred: %s\n", event.OccurredAt.Format("2006-01-02"))

	return nil
}

// runProductEventShow shows event details.
func runProductEventShow(ctx context.Context, deps *ProductCommandDeps, eventIDStr string) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	var eventID int64
	if _, err := fmt.Sscanf(eventIDStr, "%d", &eventID); err != nil {
		return fmt.Errorf("invalid event ID: %s", eventIDStr)
	}

	event, err := deps.Repository.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("event not found: %d", eventID)
	}

	// Get links.
	links, _ := deps.Repository.GetEventLinks(ctx, eventID)

	return outputProductEventDetails(deps.Config, event, links)
}

// runProductEventDelete deletes an event.
func runProductEventDelete(ctx context.Context, deps *ProductCommandDeps, eventIDStr string) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	var eventID int64
	if _, err := fmt.Sscanf(eventIDStr, "%d", &eventID); err != nil {
		return fmt.Errorf("invalid event ID: %s", eventIDStr)
	}

	// Get event info for confirmation message.
	event, err := deps.Repository.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("event not found: %d", eventID)
	}

	if err := deps.Repository.DeleteEvent(ctx, eventID); err != nil {
		return fmt.Errorf("deleting event: %w", err)
	}

	fmt.Printf("\033[32mDeleted event:\033[0m %s (ID: %d)\n", event.Title, eventID)
	return nil
}

// runProductEventLink links an event to another entity.
func runProductEventLink(ctx context.Context, deps *ProductCommandDeps, eventIDStr, entityType, entityIDStr string) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	var eventID, entityID int64
	if _, err := fmt.Sscanf(eventIDStr, "%d", &eventID); err != nil {
		return fmt.Errorf("invalid event ID: %s", eventIDStr)
	}
	if _, err := fmt.Sscanf(entityIDStr, "%d", &entityID); err != nil {
		return fmt.Errorf("invalid entity ID: %s", entityIDStr)
	}

	// Validate entity type.
	switch entityType {
	case "meeting", "email", "document", "source":
		// Valid.
	default:
		return fmt.Errorf("invalid entity type: %s (must be meeting, email, document, or source)", entityType)
	}

	// Validate link type.
	switch eventLinkType {
	case "source", "reference", "follow_up":
		// Valid.
	default:
		return fmt.Errorf("invalid link type: %s (must be source, reference, or follow_up)", eventLinkType)
	}

	link := &products.ProductEventLink{
		EventID:          eventID,
		LinkedEntityType: entityType,
		LinkedEntityID:   entityID,
		LinkType:         eventLinkType,
	}

	if err := deps.Repository.LinkEventToSource(ctx, link); err != nil {
		return fmt.Errorf("linking event: %w", err)
	}

	fmt.Printf("\033[32mLinked event %d\033[0m to %s %d (type: %s)\n", eventID, entityType, entityID, eventLinkType)
	return nil
}

// runProductEventContext shows events around a specific date.
func runProductEventContext(ctx context.Context, deps *ProductCommandDeps, productName, dateStr string) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)

	// Resolve product.
	product, err := deps.Repository.ResolveProduct(ctx, tenantID, productName)
	if err != nil {
		return fmt.Errorf("product not found: %s", productName)
	}

	// Parse center date.
	centerTime, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return fmt.Errorf("invalid date (use YYYY-MM-DD): %w", err)
	}

	// Get context window.
	window, err := deps.Repository.GetContextWindow(ctx, product.ID, centerTime, eventContextWindow)
	if err != nil {
		return fmt.Errorf("getting context window: %w", err)
	}

	return outputProductEventContext(deps.Config, product.Name, window)
}

// ==================== Output Functions ====================

func outputProductTimeline(cfg *config.CLIConfig, productName string, events []*products.ProductEvent) error {
	format := getProductOutputFormat(cfg)

	switch format {
	case config.OutputFormatJSON:
		return outputProductJSON(events)
	case config.OutputFormatYAML:
		return outputProductYAML(events)
	default:
		return outputProductTimelineTable(productName, events)
	}
}

func outputProductTimelineTable(productName string, events []*products.ProductEvent) error {
	if len(events) == 0 {
		fmt.Printf("No events found for '%s'\n", productName)
		return nil
	}

	fmt.Printf("Timeline for '%s' (%d events):\n\n", productName, len(events))
	fmt.Println("  DATE        TYPE         VISIBILITY  TITLE")
	fmt.Println("  ----        ----         ----------  -----")

	for _, e := range events {
		typeColor := getEventTypeColor(e.EventType)
		visColor := getEventVisibilityColor(e.Visibility)
		fmt.Printf("  %s  %s%-11s\033[0m  %s%-10s\033[0m  %s\n",
			e.OccurredAt.Format("2006-01-02"),
			typeColor,
			e.EventType,
			visColor,
			e.Visibility,
			truncateString(e.Title, 50))
	}

	fmt.Println()
	return nil
}

func outputProductEventDetails(cfg *config.CLIConfig, event *products.ProductEvent, links []*products.ProductEventLink) error {
	format := getProductOutputFormat(cfg)

	switch format {
	case config.OutputFormatJSON:
		return outputProductJSON(event)
	case config.OutputFormatYAML:
		return outputProductYAML(event)
	default:
		return outputProductEventDetailsText(event, links)
	}
}

func outputProductEventDetailsText(event *products.ProductEvent, links []*products.ProductEventLink) error {
	typeColor := getEventTypeColor(event.EventType)
	visColor := getEventVisibilityColor(event.Visibility)

	fmt.Println("Event Details:")
	fmt.Println()
	fmt.Printf("  \033[1mID:\033[0m          %d\n", event.ID)
	fmt.Printf("  \033[1mUUID:\033[0m        %s\n", event.EventUUID)
	fmt.Printf("  \033[1mProduct:\033[0m     %s\n", event.ProductName)
	fmt.Printf("  \033[1mType:\033[0m        %s%s\033[0m\n", typeColor, event.EventType)
	fmt.Printf("  \033[1mVisibility:\033[0m  %s%s\033[0m\n", visColor, event.Visibility)
	fmt.Printf("  \033[1mSource:\033[0m      %s\n", event.SourceType)
	fmt.Println()
	fmt.Printf("  \033[1mTitle:\033[0m       %s\n", event.Title)

	if event.Description != "" {
		fmt.Printf("  \033[1mDescription:\033[0m\n    %s\n", event.Description)
	}

	fmt.Println()
	fmt.Printf("  \033[1mOccurred:\033[0m    %s\n", event.OccurredAt.Format("2006-01-02 15:04:05"))
	if event.RecordedBy != "" {
		fmt.Printf("  \033[1mRecorded by:\033[0m %s\n", event.RecordedBy)
	}
	fmt.Printf("  \033[1mCreated:\033[0m     %s\n", event.CreatedAt.Format(time.RFC3339))
	fmt.Printf("  \033[1mUpdated:\033[0m     %s\n", event.UpdatedAt.Format(time.RFC3339))

	if len(links) > 0 {
		fmt.Println()
		fmt.Println("  \033[1mLinks:\033[0m")
		for _, l := range links {
			fmt.Printf("    - %s %d (%s)\n", l.LinkedEntityType, l.LinkedEntityID, l.LinkType)
		}
	}

	if len(event.Metadata) > 0 {
		fmt.Println()
		fmt.Println("  \033[1mMetadata:\033[0m")
		for k, v := range event.Metadata {
			fmt.Printf("    %s: %v\n", k, v)
		}
	}

	return nil
}

func outputProductEventContext(cfg *config.CLIConfig, productName string, window *products.ContextWindow) error {
	format := getProductOutputFormat(cfg)

	switch format {
	case config.OutputFormatJSON:
		return outputProductJSON(window)
	case config.OutputFormatYAML:
		return outputProductYAML(window)
	default:
		return outputProductEventContextText(productName, window)
	}
}

func outputProductEventContextText(productName string, window *products.ContextWindow) error {
	fmt.Printf("Context window for '%s' around %s:\n\n", productName, window.CenterTime.Format("2006-01-02"))
	fmt.Printf("  Window: %s to %s\n\n", window.WindowStart.Format("2006-01-02"), window.WindowEnd.Format("2006-01-02"))

	if len(window.EventsBefore) > 0 {
		fmt.Println("  \033[1mBefore:\033[0m")
		for _, e := range window.EventsBefore {
			typeColor := getEventTypeColor(e.EventType)
			fmt.Printf("    %s  %s%-11s\033[0m  %s\n",
				e.OccurredAt.Format("2006-01-02"),
				typeColor,
				e.EventType,
				truncateString(e.Title, 40))
		}
		fmt.Println()
	}

	if window.CenterEvent != nil {
		fmt.Println("  \033[1m>>> Center Event:\033[0m")
		typeColor := getEventTypeColor(window.CenterEvent.EventType)
		fmt.Printf("    %s  %s%-11s\033[0m  %s\n",
			window.CenterEvent.OccurredAt.Format("2006-01-02"),
			typeColor,
			window.CenterEvent.EventType,
			window.CenterEvent.Title)
		fmt.Println()
	}

	if len(window.EventsAfter) > 0 {
		fmt.Println("  \033[1mAfter:\033[0m")
		for _, e := range window.EventsAfter {
			typeColor := getEventTypeColor(e.EventType)
			fmt.Printf("    %s  %s%-11s\033[0m  %s\n",
				e.OccurredAt.Format("2006-01-02"),
				typeColor,
				e.EventType,
				truncateString(e.Title, 40))
		}
		fmt.Println()
	}

	total := len(window.EventsBefore) + len(window.EventsAfter)
	if window.CenterEvent != nil {
		total++
	}
	if total == 0 {
		fmt.Println("  No events in this time window.")
	}

	return nil
}

// ==================== Helper Functions ====================

// parseRelativeDate parses a date string that can be absolute (YYYY-MM-DD) or relative (7d, 30d, 1y).
func parseRelativeDate(s string) (time.Time, error) {
	// Try absolute date first.
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}

	// Try relative date.
	s = strings.ToLower(s)
	now := time.Now()

	if strings.HasSuffix(s, "d") {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err == nil {
			return now.AddDate(0, 0, -days), nil
		}
	}

	if strings.HasSuffix(s, "w") {
		var weeks int
		if _, err := fmt.Sscanf(s, "%dw", &weeks); err == nil {
			return now.AddDate(0, 0, -weeks*7), nil
		}
	}

	if strings.HasSuffix(s, "m") {
		var months int
		if _, err := fmt.Sscanf(s, "%dm", &months); err == nil {
			return now.AddDate(0, -months, 0), nil
		}
	}

	if strings.HasSuffix(s, "y") {
		var years int
		if _, err := fmt.Sscanf(s, "%dy", &years); err == nil {
			return now.AddDate(-years, 0, 0), nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid date format: %s (use YYYY-MM-DD or relative: 7d, 2w, 3m, 1y)", s)
}

// Color helpers for event output.

func getEventTypeColor(et products.EventType) string {
	switch et {
	case products.EventTypeDecision:
		return "\033[35m" // Magenta
	case products.EventTypeMilestone:
		return "\033[32m" // Green
	case products.EventTypeRisk:
		return "\033[31m" // Red
	case products.EventTypeRelease:
		return "\033[36m" // Cyan
	case products.EventTypeCompetitor:
		return "\033[33m" // Yellow
	case products.EventTypeOrgChange:
		return "\033[34m" // Blue
	case products.EventTypeMarket:
		return "\033[33m" // Yellow
	case products.EventTypeNote:
		return "\033[90m" // Gray
	default:
		return ""
	}
}

func getEventVisibilityColor(v products.EventVisibility) string {
	switch v {
	case products.EventVisibilityInternal:
		return "\033[90m" // Gray
	case products.EventVisibilityExternal:
		return "\033[33m" // Yellow
	default:
		return ""
	}
}
