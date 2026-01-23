// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/metadata"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/client"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// TenantInfo represents detailed information about a tenant.
type TenantInfo struct {
	ID          string    `json:"id" yaml:"id"`
	Name        string    `json:"name" yaml:"name"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	Status      string    `json:"status" yaml:"status"`
	Role        string    `json:"role,omitempty" yaml:"role,omitempty"`
	IsCurrent   bool      `json:"is_current" yaml:"is_current"`
}

// TenantListResponse represents the response from listing tenants.
type TenantListResponse struct {
	Tenants    []TenantInfo `json:"tenants" yaml:"tenants"`
	CurrentID  string       `json:"current_tenant_id,omitempty" yaml:"current_tenant_id,omitempty"`
	TotalCount int          `json:"total_count" yaml:"total_count"`
	FetchedAt  time.Time    `json:"fetched_at" yaml:"fetched_at"`
}

// TenantCommandDeps holds the dependencies for tenant commands.
// This allows for easier testing by injecting mock implementations.
type TenantCommandDeps struct {
	Config       *config.CLIConfig
	GRPCClient   *client.GRPCClient
	OutputFormat config.OutputFormat
	LoadConfig   func() (*config.CLIConfig, error)
	SaveConfig   func(*config.CLIConfig) error
	InitClient   func(*config.CLIConfig) (*client.GRPCClient, error)
}

// DefaultDeps returns the default dependencies for production use.
func DefaultDeps() *TenantCommandDeps {
	return &TenantCommandDeps{
		LoadConfig: config.LoadConfig,
		SaveConfig: config.SaveConfig,
		InitClient: func(cfg *config.CLIConfig) (*client.GRPCClient, error) {
			opts := client.DefaultOptions()
			opts.Insecure = cfg.Insecure
			opts.Debug = cfg.Debug
			opts.ConnectTimeout = cfg.Timeout

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

// NewTenantCommand creates the root tenant command with all subcommands.
func NewTenantCommand(deps *TenantCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultDeps()
	}

	cmd := &cobra.Command{
		Use:   "tenant",
		Short: "Manage tenant context for multi-tenant operations",
		Long: `Manage tenant context for multi-tenant operations in Penfold.

Penfold supports multi-tenancy, allowing you to work with different tenant
contexts. Use these commands to list, switch, and view tenant information.

The current tenant is stored in ~/.penf/config.yaml and can be overridden
by the PENF_TENANT_ID environment variable.`,
	}

	// Add subcommands
	cmd.AddCommand(newTenantListCommand(deps))
	cmd.AddCommand(newTenantSwitchCommand(deps))
	cmd.AddCommand(newTenantCurrentCommand(deps))
	cmd.AddCommand(newTenantShowCommand(deps))

	return cmd
}

// newTenantListCommand creates the 'tenant list' subcommand.
func newTenantListCommand(deps *TenantCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List accessible tenants",
		Long: `List all tenants that the current user has access to.

Displays tenant ID, name, status, and indicates the currently active tenant.
Use --output to change the output format (text, json, yaml).`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTenantList(cmd.Context(), deps)
		},
	}
}

// newTenantSwitchCommand creates the 'tenant switch' subcommand.
func newTenantSwitchCommand(deps *TenantCommandDeps) *cobra.Command {
	var noValidate bool

	cmd := &cobra.Command{
		Use:   "switch <tenant-id-or-alias>",
		Short: "Switch to a different tenant",
		Long: `Switch the active tenant context to a different tenant.

You can specify the tenant by its ID or alias (if configured).
The switch is validated against accessible tenants unless --no-validate is used.

Example:
  penf tenant switch acme-corp
  penf tenant switch tenant-123-456
  penf tenant switch work  # using alias`,
		Aliases: []string{"use", "sw"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTenantSwitch(cmd.Context(), deps, args[0], !noValidate)
		},
	}

	cmd.Flags().BoolVar(&noValidate, "no-validate", false, "Skip tenant access validation")

	return cmd
}

// newTenantCurrentCommand creates the 'tenant current' subcommand.
func newTenantCurrentCommand(deps *TenantCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Show current tenant",
		Long: `Show the currently active tenant context.

Displays the tenant ID from the configuration file or environment variable.
The environment variable PENF_TENANT_ID takes precedence over the config file.`,
		Aliases: []string{"whoami"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTenantCurrent(deps)
		},
	}
}

// newTenantShowCommand creates the 'tenant show' subcommand.
func newTenantShowCommand(deps *TenantCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "show [tenant-id-or-alias]",
		Short: "Show tenant details",
		Long: `Show detailed information about a tenant.

If no tenant is specified, shows information about the current tenant.
You can specify the tenant by its ID or alias (if configured).

Example:
  penf tenant show
  penf tenant show acme-corp
  penf tenant show tenant-123-456`,
		Aliases: []string{"info"},
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tenantID := ""
			if len(args) > 0 {
				tenantID = args[0]
			}
			return runTenantShow(cmd.Context(), deps, tenantID)
		},
	}
}

// runTenantList executes the tenant list command.
func runTenantList(ctx context.Context, deps *TenantCommandDeps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Get current tenant ID (from env or config).
	currentTenantID := getCurrentTenantID(cfg)

	// Get list of tenants (mock implementation for now).
	tenants := getMockTenantList(currentTenantID)

	response := TenantListResponse{
		Tenants:    tenants,
		CurrentID:  currentTenantID,
		TotalCount: len(tenants),
		FetchedAt:  time.Now(),
	}

	return outputTenantList(cfg.OutputFormat, response)
}

// runTenantSwitch executes the tenant switch command.
func runTenantSwitch(ctx context.Context, deps *TenantCommandDeps, tenantRef string, validate bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Resolve alias to tenant ID if applicable.
	tenantID := resolveTenantAlias(cfg, tenantRef)

	// Validate tenant access if requested.
	if validate {
		if err := validateTenantAccess(ctx, deps, tenantID); err != nil {
			return fmt.Errorf("tenant validation failed: %w", err)
		}
	}

	// Check if env var is set (warn user).
	if envTenant := os.Getenv("PENF_TENANT_ID"); envTenant != "" {
		fmt.Fprintf(os.Stderr, "Warning: PENF_TENANT_ID environment variable is set to %q.\n", envTenant)
		fmt.Fprintf(os.Stderr, "The environment variable takes precedence over the config file.\n")
		fmt.Fprintf(os.Stderr, "Unset it to use the config file setting.\n\n")
	}

	// Update configuration.
	cfg.TenantID = tenantID
	if err := deps.SaveConfig(cfg); err != nil {
		return fmt.Errorf("saving configuration: %w", err)
	}

	fmt.Printf("Switched to tenant: %s\n", tenantID)

	// Show alias if it was used.
	if tenantRef != tenantID {
		fmt.Printf("  (alias: %s)\n", tenantRef)
	}

	return nil
}

// runTenantCurrent executes the tenant current command.
func runTenantCurrent(deps *TenantCommandDeps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	currentTenantID := getCurrentTenantID(cfg)

	if currentTenantID == "" {
		fmt.Println("No tenant configured.")
		fmt.Println("\nUse 'penf tenant switch <tenant-id>' to set a tenant.")
		fmt.Println("Or set the PENF_TENANT_ID environment variable.")
		return nil
	}

	// Determine the source.
	source := "config file"
	if envTenant := os.Getenv("PENF_TENANT_ID"); envTenant != "" {
		source = "environment variable (PENF_TENANT_ID)"
	}

	switch cfg.OutputFormat {
	case config.OutputFormatJSON:
		output := map[string]string{
			"tenant_id": currentTenantID,
			"source":    source,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	case config.OutputFormatYAML:
		output := map[string]string{
			"tenant_id": currentTenantID,
			"source":    source,
		}
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(output)
	default:
		fmt.Printf("Current tenant: %s\n", currentTenantID)
		fmt.Printf("  Source: %s\n", source)

		// Show alias if there is one.
		if alias := findTenantAlias(cfg, currentTenantID); alias != "" {
			fmt.Printf("  Alias: %s\n", alias)
		}
	}

	return nil
}

// runTenantShow executes the tenant info command.
func runTenantShow(ctx context.Context, deps *TenantCommandDeps, tenantRef string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Use current tenant if none specified.
	if tenantRef == "" {
		tenantRef = getCurrentTenantID(cfg)
		if tenantRef == "" {
			return fmt.Errorf("no tenant specified and no current tenant configured")
		}
	}

	// Resolve alias to tenant ID if applicable.
	tenantID := resolveTenantAlias(cfg, tenantRef)

	// Get tenant info (mock implementation for now).
	info := getMockTenantInfo(tenantID)
	info.IsCurrent = tenantID == getCurrentTenantID(cfg)

	return outputTenantDetail(cfg.OutputFormat, info)
}

// getCurrentTenantID returns the current tenant ID from env or config.
// Environment variable takes precedence.
func getCurrentTenantID(cfg *config.CLIConfig) string {
	if envTenant := os.Getenv("PENF_TENANT_ID"); envTenant != "" {
		return envTenant
	}
	return cfg.TenantID
}

// resolveTenantAlias resolves a tenant reference to its actual ID.
// If the reference is an alias in the config, returns the mapped ID.
// Otherwise returns the reference as-is.
func resolveTenantAlias(cfg *config.CLIConfig, ref string) string {
	if cfg.TenantAliases != nil {
		if id, ok := cfg.TenantAliases[ref]; ok {
			return id
		}
	}
	return ref
}

// findTenantAlias finds an alias for a given tenant ID.
// Returns empty string if no alias exists.
func findTenantAlias(cfg *config.CLIConfig, tenantID string) string {
	if cfg.TenantAliases == nil {
		return ""
	}
	for alias, id := range cfg.TenantAliases {
		if id == tenantID {
			return alias
		}
	}
	return ""
}

// validateTenantAccess validates that the user has access to the tenant.
// STUB: Uses mock validation until tenant service gRPC is connected.
func validateTenantAccess(ctx context.Context, deps *TenantCommandDeps, tenantID string) error {
	// Uses mock validation that accepts any tenant ID.

	// Mock validation: reject if tenant ID is empty.
	if tenantID == "" {
		return fmt.Errorf("tenant ID cannot be empty")
	}

	// Mock validation: reject if tenant ID contains invalid characters.
	if strings.ContainsAny(tenantID, " \t\n") {
		return fmt.Errorf("tenant ID contains invalid characters")
	}

	return nil
}

// getMockTenantList returns mock tenant data for development/testing.
func getMockTenantList(currentTenantID string) []TenantInfo {
	tenants := []TenantInfo{
		{
			ID:          "tenant-default-001",
			Name:        "Default Tenant",
			Description: "Default personal tenant",
			Status:      "active",
			Role:        "owner",
			CreatedAt:   time.Now().AddDate(-1, 0, 0),
		},
		{
			ID:          "tenant-acme-002",
			Name:        "Acme Corporation",
			Description: "Work tenant for Acme Corp",
			Status:      "active",
			Role:        "admin",
			CreatedAt:   time.Now().AddDate(0, -6, 0),
		},
		{
			ID:          "tenant-project-003",
			Name:        "Side Project",
			Description: "Personal side project",
			Status:      "active",
			Role:        "member",
			CreatedAt:   time.Now().AddDate(0, -1, 0),
		},
	}

	// Mark the current tenant.
	for i := range tenants {
		tenants[i].IsCurrent = tenants[i].ID == currentTenantID
	}

	return tenants
}

// getMockTenantInfo returns mock tenant info for development/testing.
func getMockTenantInfo(tenantID string) TenantInfo {
	// Check against mock tenants.
	mockTenants := getMockTenantList("")
	for _, t := range mockTenants {
		if t.ID == tenantID {
			return t
		}
	}

	// Return unknown tenant info.
	return TenantInfo{
		ID:     tenantID,
		Name:   tenantID,
		Status: "unknown",
		Role:   "unknown",
	}
}

// outputTenantList outputs the tenant list in the configured format.
func outputTenantList(format config.OutputFormat, response TenantListResponse) error {
	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(response)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(response)
	default:
		return outputTenantListText(response)
	}
}

// outputTenantListText outputs the tenant list in human-readable format.
func outputTenantListText(response TenantListResponse) error {
	if len(response.Tenants) == 0 {
		fmt.Println("No tenants found.")
		return nil
	}

	fmt.Printf("Available tenants (%d):\n\n", response.TotalCount)
	fmt.Println("  CURRENT  ID                      NAME                    STATUS    ROLE")
	fmt.Println("  -------  --                      ----                    ------    ----")

	// Sort by name for consistent output.
	tenants := make([]TenantInfo, len(response.Tenants))
	copy(tenants, response.Tenants)
	sort.Slice(tenants, func(i, j int) bool {
		return tenants[i].Name < tenants[j].Name
	})

	for _, t := range tenants {
		currentMarker := " "
		if t.IsCurrent {
			currentMarker = "*"
		}

		// Truncate long values for table display.
		id := truncateString(t.ID, 22)
		name := truncateString(t.Name, 22)

		statusColor := "\033[32m" // Green for active
		if t.Status != "active" {
			statusColor = "\033[33m" // Yellow for other
		}

		fmt.Printf("  %s        %-22s  %-22s  %s%-8s\033[0m  %s\n",
			currentMarker, id, name, statusColor, t.Status, t.Role)
	}

	fmt.Println()
	return nil
}

// outputTenantDetail outputs tenant info in the configured format.
func outputTenantDetail(format config.OutputFormat, info TenantInfo) error {
	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(info)
	default:
		return outputTenantDetailText(info)
	}
}

// outputTenantDetailText outputs tenant info in human-readable format.
func outputTenantDetailText(info TenantInfo) error {
	statusColor := "\033[32m"
	if info.Status != "active" {
		statusColor = "\033[33m"
	}

	fmt.Printf("Tenant Information:\n\n")
	fmt.Printf("  ID:          %s\n", info.ID)
	fmt.Printf("  Name:        %s\n", info.Name)
	if info.Description != "" {
		fmt.Printf("  Description: %s\n", info.Description)
	}
	fmt.Printf("  Status:      %s%s\033[0m\n", statusColor, info.Status)
	if info.Role != "" && info.Role != "unknown" {
		fmt.Printf("  Your Role:   %s\n", info.Role)
	}
	if !info.CreatedAt.IsZero() {
		fmt.Printf("  Created:     %s\n", info.CreatedAt.Format(time.RFC3339))
	}
	if info.IsCurrent {
		fmt.Printf("  Current:     yes (active context)\n")
	}

	return nil
}

// truncateString truncates a string to the given length with ellipsis.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// AddTenantMetadata adds tenant ID to gRPC metadata context.
// This should be called before making gRPC requests.
func AddTenantMetadata(ctx context.Context, tenantID string) context.Context {
	if tenantID == "" {
		return ctx
	}
	md := metadata.Pairs("x-tenant-id", tenantID)
	return metadata.NewOutgoingContext(ctx, md)
}

// GetTenantFromContext extracts tenant ID from incoming gRPC context.
func GetTenantFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("x-tenant-id")
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
