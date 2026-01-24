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

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/products"
)

// Product query command flags.
var (
	productQueryOutput string
)

// addProductQueryCommands adds the query subcommand to the product command.
func addProductQueryCommands(parent *cobra.Command, deps *ProductCommandDeps) {
	// penf product query "<natural language query>"
	queryCmd := &cobra.Command{
		Use:   "query <question>",
		Short: "Ask natural language questions about products",
		Long: `Ask natural language questions about products, teams, and roles.

Supports various query patterns:
  - "who is the DRI for <product>"
  - "who is the DRI for networking on <product>"
  - "which teams work on <product>"
  - "show timeline for <product>"
  - "what are the sub-products of <product>"
  - General keyword search for products

Examples:
  # Find the DRI for a product
  penf product query "who is the DRI for MTC"

  # Find DRI for a specific scope
  penf product query "who is the DRI for networking on MTC"

  # Find teams working on a product
  penf product query "which teams work on LKE Enterprise"

  # Show product timeline
  penf product query "show timeline for API Gateway"

  # Show product hierarchy
  penf product query "what are the sub-products of Cloud Platform"

  # Search for products
  penf product query "kubernetes"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductQuery(cmd.Context(), deps, strings.Join(args, " "))
		},
	}

	queryCmd.Flags().StringVarP(&productQueryOutput, "output", "o", "", "Output format: text, json, yaml")

	parent.AddCommand(queryCmd)
}

// runProductQuery executes a natural language product query.
func runProductQuery(ctx context.Context, deps *ProductCommandDeps, queryStr string) error {
	// Initialize dependencies
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	// Determine output format
	outputFormat := deps.Config.OutputFormat
	if productQueryOutput != "" {
		outputFormat = config.OutputFormat(productQueryOutput)
		if !outputFormat.IsValid() {
			return fmt.Errorf("invalid output format: %s (must be text, json, yaml)", productQueryOutput)
		}
	}

	// Create query service
	logger := logging.NewLogger(&logging.Config{
		Level:       logging.LevelInfo,
		ServiceName: "penf",
		Output:      os.Stderr,
	})
	querySvc := products.NewQueryService(deps.Pool, logger)

	// Execute query
	result, err := querySvc.Query(ctx, deps.Config.TenantID, queryStr)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	return outputQueryResult(outputFormat, result, queryStr)
}

// ProductQueryResponse represents the JSON/YAML output for a query.
type ProductQueryResponse struct {
	Query     string            `json:"query" yaml:"query"`
	Type      string            `json:"type" yaml:"type"`
	Message   string            `json:"message" yaml:"message"`
	Products  []ProductSummary  `json:"products,omitempty" yaml:"products,omitempty"`
	Persons   []PersonInfo      `json:"persons,omitempty" yaml:"persons,omitempty"`
	Teams     []TeamInfo        `json:"teams,omitempty" yaml:"teams,omitempty"`
	Events    []EventInfo       `json:"events,omitempty" yaml:"events,omitempty"`
	Hierarchy *HierarchyInfo    `json:"hierarchy,omitempty" yaml:"hierarchy,omitempty"`
	QueriedAt time.Time         `json:"queried_at" yaml:"queried_at"`
}

// ProductSummary provides product info for query results.
type ProductSummary struct {
	ID          int64    `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	ProductType string   `json:"product_type" yaml:"product_type"`
	Status      string   `json:"status" yaml:"status"`
	Keywords    []string `json:"keywords,omitempty" yaml:"keywords,omitempty"`
}

// PersonInfo provides person info for role queries.
type PersonInfo struct {
	PersonID    int64  `json:"person_id" yaml:"person_id"`
	Name        string `json:"name" yaml:"name"`
	Email       string `json:"email" yaml:"email"`
	Role        string `json:"role" yaml:"role"`
	Scope       string `json:"scope,omitempty" yaml:"scope,omitempty"`
	TeamName    string `json:"team_name" yaml:"team_name"`
	TeamContext string `json:"team_context,omitempty" yaml:"team_context,omitempty"`
}

// TeamInfo provides team info for team queries.
type TeamInfo struct {
	TeamID   int64  `json:"team_id" yaml:"team_id"`
	TeamName string `json:"team_name" yaml:"team_name"`
	Context  string `json:"context,omitempty" yaml:"context,omitempty"`
}

// EventInfo provides event info for timeline queries.
type EventInfo struct {
	ID          int64     `json:"id" yaml:"id"`
	EventType   string    `json:"event_type" yaml:"event_type"`
	Visibility  string    `json:"visibility" yaml:"visibility"`
	Title       string    `json:"title" yaml:"title"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty"`
	OccurredAt  time.Time `json:"occurred_at" yaml:"occurred_at"`
}

// HierarchyInfo provides hierarchy info.
type HierarchyInfo struct {
	ID          int64          `json:"id" yaml:"id"`
	Name        string         `json:"name" yaml:"name"`
	ProductType string         `json:"product_type" yaml:"product_type"`
	Children    []ProductSummary `json:"children,omitempty" yaml:"children,omitempty"`
}

// outputQueryResult formats and outputs the query result.
func outputQueryResult(format config.OutputFormat, result *products.QueryResult, queryStr string) error {
	// Build response
	response := ProductQueryResponse{
		Query:     queryStr,
		Type:      string(result.Type),
		Message:   result.Message,
		QueriedAt: time.Now(),
	}

	// Add type-specific data
	if len(result.Products) > 0 {
		for _, p := range result.Products {
			desc := ""
			if p.Description != nil {
				desc = *p.Description
			}
			response.Products = append(response.Products, ProductSummary{
				ID:          p.ID,
				Name:        p.Name,
				Description: desc,
				ProductType: string(p.ProductType),
				Status:      string(p.Status),
				Keywords:    p.Keywords,
			})
		}
	}

	if len(result.Persons) > 0 {
		for _, p := range result.Persons {
			response.Persons = append(response.Persons, PersonInfo{
				PersonID:    p.PersonID,
				Name:        p.PersonName,
				Email:       p.PersonEmail,
				Role:        p.Role,
				Scope:       p.Scope,
				TeamName:    p.TeamName,
				TeamContext: p.TeamContext,
			})
		}
	}

	if len(result.Teams) > 0 {
		for _, t := range result.Teams {
			response.Teams = append(response.Teams, TeamInfo{
				TeamID:   t.TeamID,
				TeamName: t.TeamName,
				Context:  t.Context,
			})
		}
	}

	if len(result.Events) > 0 {
		for _, e := range result.Events {
			response.Events = append(response.Events, EventInfo{
				ID:          e.ID,
				EventType:   string(e.EventType),
				Visibility:  string(e.Visibility),
				Title:       e.Title,
				Description: e.Description,
				OccurredAt:  e.OccurredAt,
			})
		}
	}

	if result.Hierarchy != nil {
		response.Hierarchy = &HierarchyInfo{
			ID:          result.Hierarchy.ID,
			Name:        result.Hierarchy.Name,
			ProductType: string(result.Hierarchy.ProductType),
		}
	}

	// Output based on format
	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(response)
	default:
		return outputQueryResultText(response, result)
	}
}

// outputQueryResultText formats query results for terminal display.
func outputQueryResultText(response ProductQueryResponse, result *products.QueryResult) error {
	// Print the answer
	fmt.Printf("\n%s\n\n", response.Message)

	// Print additional details based on query type
	switch result.Type {
	case products.QueryTypeRole:
		if len(result.Persons) > 0 {
			fmt.Println("Details:")
			for _, p := range result.Persons {
				fmt.Printf("  • %s (%s)\n", p.PersonName, p.PersonEmail)
				fmt.Printf("    Role: %s", p.Role)
				if p.Scope != "" {
					fmt.Printf(" (%s)", p.Scope)
				}
				fmt.Println()
				fmt.Printf("    Team: %s", p.TeamName)
				if p.TeamContext != "" {
					fmt.Printf(" [%s]", p.TeamContext)
				}
				fmt.Println()
			}
		}

	case products.QueryTypeTeam:
		if len(result.Teams) > 0 {
			fmt.Println("Team Details:")
			for _, t := range result.Teams {
				fmt.Printf("  • %s", t.TeamName)
				if t.Context != "" {
					fmt.Printf(" (%s)", t.Context)
				}
				fmt.Println()
			}
		}

	case products.QueryTypeTimeline:
		if len(result.Events) > 0 {
			fmt.Println("Recent Events:")
			fmt.Println("  DATE        TYPE         TITLE")
			for _, e := range result.Events {
				fmt.Printf("  %s  %-11s  %s\n",
					e.OccurredAt.Format("2006-01-02"),
					e.EventType,
					truncateString(e.Title, 50))
			}
		}

	case products.QueryTypeHierarchy:
		if result.Hierarchy != nil {
			fmt.Printf("Product: %s (%s)\n", result.Hierarchy.Name, result.Hierarchy.ProductType)
			// Note: Full hierarchy tree would require additional implementation
		}

	case products.QueryTypeSearch:
		if len(result.Products) > 0 {
			fmt.Println("Products:")
			for _, p := range result.Products {
				fmt.Printf("  • %s", p.Name)
				if p.ProductType != "product" {
					fmt.Printf(" [%s]", p.ProductType)
				}
				fmt.Println()
				if p.Description != nil && *p.Description != "" {
					fmt.Printf("    %s\n", truncateString(*p.Description, 60))
				}
			}
		}
	}

	return nil
}
