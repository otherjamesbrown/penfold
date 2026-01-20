// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/products"
)

// Team command flags.
var (
	teamContext string
	teamScope   string
	teamRole    string
	showAll     bool // include inactive
)

// addProductTeamCommands adds the team subcommand group to the product command.
func addProductTeamCommands(productCmd *cobra.Command, deps *ProductCommandDeps) {
	teamCmd := &cobra.Command{
		Use:   "team <product>",
		Short: "List teams associated with a product",
		Long: `List all teams associated with a product.

Shows team associations including context (e.g., "EMEA", "core").

Examples:
  # List teams for a product
  penf product team "My Product"

  # List teams with a specific context
  penf product team "My Product" --context core`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductTeamList(cmd.Context(), deps, args[0])
		},
	}

	teamCmd.Flags().StringVar(&teamContext, "context", "", "Filter by context")

	// Subcommands for team management.
	teamCmd.AddCommand(newProductTeamAddCommand(deps))
	teamCmd.AddCommand(newProductTeamRemoveCommand(deps))
	teamCmd.AddCommand(newProductTeamRoleCommand(deps))
	teamCmd.AddCommand(newProductTeamPeopleCommand(deps))

	productCmd.AddCommand(teamCmd)
}

// newProductTeamAddCommand creates the 'product team add' subcommand.
func newProductTeamAddCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <product> <team>",
		Short: "Associate a team with a product",
		Long: `Associate a team with a product.

The team must already exist in the system. Use --context to specify
the team's relationship to the product (e.g., "EMEA", "core", "API").

Examples:
  # Associate a team with a product
  penf product team add "My Product" "Engineering Team"

  # Associate with a context
  penf product team add "My Product" "Engineering Team" --context "EMEA"`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductTeamAdd(cmd.Context(), deps, args[0], args[1])
		},
	}

	cmd.Flags().StringVar(&teamContext, "context", "", "Context for the team association (e.g., EMEA, core)")

	return cmd
}

// newProductTeamRemoveCommand creates the 'product team remove' subcommand.
func newProductTeamRemoveCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <product> <team>",
		Short: "Remove a team from a product",
		Long: `Remove a team association from a product.

If the team has multiple contexts, specify --context to remove a specific one.

Examples:
  penf product team remove "My Product" "Engineering Team"
  penf product team remove "My Product" "Engineering Team" --context "EMEA"`,
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductTeamRemove(cmd.Context(), deps, args[0], args[1])
		},
	}

	cmd.Flags().StringVar(&teamContext, "context", "", "Context to remove (if team has multiple)")

	return cmd
}

// newProductTeamRoleCommand creates the 'product team role' subcommand group.
func newProductTeamRoleCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role",
		Short: "Manage roles for product-team associations",
		Long: `Manage scoped roles for product-team associations.

Roles define who does what for a product within a team context.
Common roles: DRI, TL, PM, Engineer, Reviewer.
Scopes can further narrow the role (e.g., "networking", "security").`,
	}

	cmd.AddCommand(newProductTeamRoleListCommand(deps))
	cmd.AddCommand(newProductTeamRoleAddCommand(deps))
	cmd.AddCommand(newProductTeamRoleEndCommand(deps))
	cmd.AddCommand(newProductTeamRoleFindCommand(deps))

	return cmd
}

// newProductTeamRoleListCommand creates the 'product team role list' subcommand.
func newProductTeamRoleListCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <product> <team>",
		Short: "List roles for a product-team",
		Long: `List all roles assigned to people for a product-team association.

Examples:
  penf product team role list "My Product" "Engineering Team"
  penf product team role list "My Product" "Engineering Team" --all  # include inactive`,
		Aliases: []string{"ls"},
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductTeamRoleList(cmd.Context(), deps, args[0], args[1])
		},
	}

	cmd.Flags().BoolVar(&showAll, "all", false, "Include inactive roles")
	cmd.Flags().StringVar(&teamContext, "context", "", "Filter by team context")

	return cmd
}

// newProductTeamRoleAddCommand creates the 'product team role add' subcommand.
func newProductTeamRoleAddCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <product> <team> <person-email> <role>",
		Short: "Add a role assignment",
		Long: `Add a role for a person on a product-team association.

Common roles: DRI, TL, PM, Engineer, Reviewer, Approver.
Use --scope to narrow the role (e.g., "networking", "security", "API").

Examples:
  # Make someone the DRI for a product
  penf product team role add "My Product" "Engineering" "john@example.com" "DRI"

  # Add a scoped role
  penf product team role add "My Product" "Engineering" "jane@example.com" "DRI" --scope "networking"`,
		Args: cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductTeamRoleAdd(cmd.Context(), deps, args[0], args[1], args[2], args[3])
		},
	}

	cmd.Flags().StringVar(&teamContext, "context", "", "Team context (if team has multiple)")
	cmd.Flags().StringVar(&teamScope, "scope", "", "Role scope (e.g., networking, security)")

	return cmd
}

// newProductTeamRoleEndCommand creates the 'product team role end' subcommand.
func newProductTeamRoleEndCommand(deps *ProductCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "end <role-id>",
		Short: "End a role assignment",
		Long: `End a role assignment (mark as inactive).

The role history is preserved for audit purposes.

Example:
  penf product team role end 123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductTeamRoleEnd(cmd.Context(), deps, args[0])
		},
	}
}

// newProductTeamRoleFindCommand creates the 'product team role find' subcommand.
func newProductTeamRoleFindCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "find",
		Short: "Find people by role",
		Long: `Find people by role across products and teams.

This answers questions like "who is the DRI for networking on Product X?"

Examples:
  # Find all DRIs
  penf product team role find --role DRI

  # Find DRI for a specific product
  penf product team role find --role DRI --product "My Product"

  # Find networking DRI on a product
  penf product team role find --role DRI --product "My Product" --scope networking`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductTeamRoleFind(cmd.Context(), deps)
		},
	}

	cmd.Flags().StringVar(&teamRole, "role", "", "Role to search for (e.g., DRI, TL)")
	cmd.Flags().StringVar(&productParent, "product", "", "Filter by product name")
	cmd.Flags().StringVar(&teamScope, "scope", "", "Filter by scope")
	cmd.Flags().BoolVar(&showAll, "all", false, "Include inactive roles")

	return cmd
}

// newProductTeamPeopleCommand creates the 'product team people' subcommand.
func newProductTeamPeopleCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "people <product>",
		Short: "List all people working on a product",
		Long: `List all people working on a product across all teams.

Shows everyone with a role on the product, organized by team.

Examples:
  penf product team people "My Product"
  penf product team people "My Product" --all  # include inactive`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductTeamPeople(cmd.Context(), deps, args[0])
		},
	}

	cmd.Flags().BoolVar(&showAll, "all", false, "Include inactive roles")

	return cmd
}

// ==================== Command Execution Functions ====================

// TeamDeps extends ProductCommandDeps with team-specific dependencies.
type TeamDeps struct {
	*ProductCommandDeps
	EntitiesRepo *entities.Repository
}

// initTeamDeps initializes dependencies for team commands.
func initTeamDeps(ctx context.Context, deps *ProductCommandDeps) (*TeamDeps, error) {
	if err := initProductDeps(ctx, deps); err != nil {
		return nil, err
	}

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(zerolog.InfoLevel).
		With().
		Timestamp().
		Logger()

	return &TeamDeps{
		ProductCommandDeps: deps,
		EntitiesRepo:       entities.NewRepository(deps.Pool, logger),
	}, nil
}

// resolveTeam resolves a team by name.
func resolveTeam(ctx context.Context, repo *entities.Repository, tenantID, teamName string) (*entities.Team, error) {
	team, err := repo.GetTeamByName(ctx, tenantID, teamName)
	if err != nil {
		return nil, fmt.Errorf("team not found: %s", teamName)
	}
	return team, nil
}

// findProductTeam finds a product-team association.
func findProductTeam(ctx context.Context, repo *products.Repository, productID, teamID int64, context string) (*products.ProductTeam, error) {
	teams, err := repo.GetTeamsForProduct(ctx, productID)
	if err != nil {
		return nil, err
	}

	for _, pt := range teams {
		if pt.TeamID == teamID {
			// If context specified, must match.
			if context != "" {
				if !strings.EqualFold(pt.Context, context) {
					continue
				}
			}
			return pt, nil
		}
	}

	return nil, products.ErrNotFound
}

// resolvePerson resolves a person by email.
func resolvePerson(ctx context.Context, repo *entities.Repository, tenantID, email string) (*entities.Person, error) {
	person, err := repo.GetPersonByEmail(ctx, tenantID, email)
	if err != nil {
		return nil, fmt.Errorf("person not found: %s", email)
	}
	return person, nil
}

// runProductTeamList lists teams for a product.
func runProductTeamList(ctx context.Context, deps *ProductCommandDeps, productName string) error {
	tdeps, err := initTeamDeps(ctx, deps)
	if err != nil {
		return err
	}
	defer tdeps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)

	// Resolve product.
	product, err := tdeps.Repository.ResolveProduct(ctx, tenantID, productName)
	if err != nil {
		return fmt.Errorf("product not found: %s", productName)
	}

	// Get teams.
	teams, err := tdeps.Repository.GetTeamsForProduct(ctx, product.ID)
	if err != nil {
		return fmt.Errorf("getting teams: %w", err)
	}

	// Filter by context if specified.
	if teamContext != "" {
		var filtered []*products.ProductTeam
		for _, t := range teams {
			if strings.EqualFold(t.Context, teamContext) {
				filtered = append(filtered, t)
			}
		}
		teams = filtered
	}

	return outputProductTeams(deps.Config, product.Name, teams)
}

// runProductTeamAdd associates a team with a product.
func runProductTeamAdd(ctx context.Context, deps *ProductCommandDeps, productName, teamName string) error {
	tdeps, err := initTeamDeps(ctx, deps)
	if err != nil {
		return err
	}
	defer tdeps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)

	// Resolve product and team.
	product, err := tdeps.Repository.ResolveProduct(ctx, tenantID, productName)
	if err != nil {
		return fmt.Errorf("product not found: %s", productName)
	}

	team, err := resolveTeam(ctx, tdeps.EntitiesRepo, tenantID, teamName)
	if err != nil {
		return err
	}

	// Create association.
	pt := &products.ProductTeam{
		TenantID:  tenantID,
		ProductID: product.ID,
		TeamID:    team.ID,
		Context:   teamContext,
	}

	if err := tdeps.Repository.AssociateTeam(ctx, pt); err != nil {
		return fmt.Errorf("associating team: %w", err)
	}

	contextStr := ""
	if teamContext != "" {
		contextStr = fmt.Sprintf(" (context: %s)", teamContext)
	}
	fmt.Printf("\033[32mAssociated team:\033[0m %s -> %s%s\n", teamName, productName, contextStr)
	return nil
}

// runProductTeamRemove removes a team from a product.
func runProductTeamRemove(ctx context.Context, deps *ProductCommandDeps, productName, teamName string) error {
	tdeps, err := initTeamDeps(ctx, deps)
	if err != nil {
		return err
	}
	defer tdeps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)

	// Resolve product and team.
	product, err := tdeps.Repository.ResolveProduct(ctx, tenantID, productName)
	if err != nil {
		return fmt.Errorf("product not found: %s", productName)
	}

	team, err := resolveTeam(ctx, tdeps.EntitiesRepo, tenantID, teamName)
	if err != nil {
		return err
	}

	// Remove association.
	if err := tdeps.Repository.DissociateTeam(ctx, product.ID, team.ID, teamContext); err != nil {
		return fmt.Errorf("removing team: %w", err)
	}

	fmt.Printf("\033[32mRemoved team:\033[0m %s from %s\n", teamName, productName)
	return nil
}

// runProductTeamRoleList lists roles for a product-team.
func runProductTeamRoleList(ctx context.Context, deps *ProductCommandDeps, productName, teamName string) error {
	tdeps, err := initTeamDeps(ctx, deps)
	if err != nil {
		return err
	}
	defer tdeps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)

	// Resolve product and team.
	product, err := tdeps.Repository.ResolveProduct(ctx, tenantID, productName)
	if err != nil {
		return fmt.Errorf("product not found: %s", productName)
	}

	team, err := resolveTeam(ctx, tdeps.EntitiesRepo, tenantID, teamName)
	if err != nil {
		return err
	}

	// Find product-team association.
	pt, err := findProductTeam(ctx, tdeps.Repository, product.ID, team.ID, teamContext)
	if err != nil {
		return fmt.Errorf("team '%s' is not associated with '%s'", teamName, productName)
	}

	// Get roles.
	activeOnly := !showAll
	roles, err := tdeps.Repository.GetRolesForProductTeam(ctx, pt.ID, activeOnly)
	if err != nil {
		return fmt.Errorf("getting roles: %w", err)
	}

	return outputProductTeamRoles(deps.Config, productName, teamName, roles)
}

// runProductTeamRoleAdd adds a role assignment.
func runProductTeamRoleAdd(ctx context.Context, deps *ProductCommandDeps, productName, teamName, personEmail, roleName string) error {
	tdeps, err := initTeamDeps(ctx, deps)
	if err != nil {
		return err
	}
	defer tdeps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)

	// Resolve product, team, and person.
	product, err := tdeps.Repository.ResolveProduct(ctx, tenantID, productName)
	if err != nil {
		return fmt.Errorf("product not found: %s", productName)
	}

	team, err := resolveTeam(ctx, tdeps.EntitiesRepo, tenantID, teamName)
	if err != nil {
		return err
	}

	person, err := resolvePerson(ctx, tdeps.EntitiesRepo, tenantID, personEmail)
	if err != nil {
		return err
	}

	// Find product-team association.
	pt, err := findProductTeam(ctx, tdeps.Repository, product.ID, team.ID, teamContext)
	if err != nil {
		return fmt.Errorf("team '%s' is not associated with '%s' - add team first", teamName, productName)
	}

	// Add role.
	role := &products.ProductTeamRole{
		TenantID:      tenantID,
		ProductTeamID: pt.ID,
		PersonID:      person.ID,
		Role:          roleName,
		Scope:         teamScope,
	}

	if err := tdeps.Repository.AddRole(ctx, role); err != nil {
		return fmt.Errorf("adding role: %w", err)
	}

	scopeStr := ""
	if teamScope != "" {
		scopeStr = fmt.Sprintf(" [%s]", teamScope)
	}
	fmt.Printf("\033[32mAdded role:\033[0m %s is now %s%s on %s/%s (ID: %d)\n",
		person.CanonicalName, roleName, scopeStr, productName, teamName, role.ID)
	return nil
}

// runProductTeamRoleEnd ends a role assignment.
func runProductTeamRoleEnd(ctx context.Context, deps *ProductCommandDeps, roleIDStr string) error {
	tdeps, err := initTeamDeps(ctx, deps)
	if err != nil {
		return err
	}
	defer tdeps.Pool.Close()

	var roleID int64
	if _, err := fmt.Sscanf(roleIDStr, "%d", &roleID); err != nil {
		return fmt.Errorf("invalid role ID: %s", roleIDStr)
	}

	// Get role info for display.
	role, err := tdeps.Repository.GetRole(ctx, roleID)
	if err != nil {
		return fmt.Errorf("role not found: %d", roleID)
	}

	// End role.
	if err := tdeps.Repository.EndRole(ctx, roleID); err != nil {
		return fmt.Errorf("ending role: %w", err)
	}

	fmt.Printf("\033[32mEnded role:\033[0m %s is no longer %s on %s/%s\n",
		role.PersonName, role.Role, role.ProductName, role.TeamName)
	return nil
}

// runProductTeamRoleFind finds people by role.
func runProductTeamRoleFind(ctx context.Context, deps *ProductCommandDeps) error {
	tdeps, err := initTeamDeps(ctx, deps)
	if err != nil {
		return err
	}
	defer tdeps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)

	query := products.RoleQuery{
		TenantID:    tenantID,
		Role:        teamRole,
		ProductName: productParent, // reusing the --product flag
		Scope:       teamScope,
		ActiveOnly:  !showAll,
	}

	roles, err := tdeps.Repository.FindByRole(ctx, query)
	if err != nil {
		return fmt.Errorf("finding roles: %w", err)
	}

	return outputRoleSearchResults(deps.Config, roles)
}

// runProductTeamPeople lists all people on a product.
func runProductTeamPeople(ctx context.Context, deps *ProductCommandDeps, productName string) error {
	tdeps, err := initTeamDeps(ctx, deps)
	if err != nil {
		return err
	}
	defer tdeps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)

	// Resolve product.
	product, err := tdeps.Repository.ResolveProduct(ctx, tenantID, productName)
	if err != nil {
		return fmt.Errorf("product not found: %s", productName)
	}

	// Get people.
	activeOnly := !showAll
	roles, err := tdeps.Repository.GetPeopleOnProduct(ctx, product.ID, activeOnly)
	if err != nil {
		return fmt.Errorf("getting people: %w", err)
	}

	return outputProductPeople(deps.Config, productName, roles)
}

// ==================== Output Functions ====================

func outputProductTeams(cfg *config.CLIConfig, productName string, teams []*products.ProductTeam) error {
	format := getProductOutputFormat(cfg)

	switch format {
	case config.OutputFormatJSON:
		return outputProductJSON(teams)
	case config.OutputFormatYAML:
		return outputProductYAML(teams)
	default:
		return outputProductTeamsTable(productName, teams)
	}
}

func outputProductTeamsTable(productName string, teams []*products.ProductTeam) error {
	if len(teams) == 0 {
		fmt.Printf("No teams associated with '%s'\n", productName)
		return nil
	}

	fmt.Printf("Teams for '%s' (%d):\n\n", productName, len(teams))
	fmt.Println("  ID      TEAM                           CONTEXT")
	fmt.Println("  --      ----                           -------")

	for _, t := range teams {
		context := "-"
		if t.Context != "" {
			context = t.Context
		}
		fmt.Printf("  %-6d  %-30s %s\n", t.ID, truncateString(t.TeamName, 30), context)
	}

	fmt.Println()
	return nil
}

func outputProductTeamRoles(cfg *config.CLIConfig, productName, teamName string, roles []*products.ProductTeamRole) error {
	format := getProductOutputFormat(cfg)

	switch format {
	case config.OutputFormatJSON:
		return outputProductJSON(roles)
	case config.OutputFormatYAML:
		return outputProductYAML(roles)
	default:
		return outputProductTeamRolesTable(productName, teamName, roles)
	}
}

func outputProductTeamRolesTable(productName, teamName string, roles []*products.ProductTeamRole) error {
	if len(roles) == 0 {
		fmt.Printf("No roles assigned for '%s' / '%s'\n", productName, teamName)
		return nil
	}

	fmt.Printf("Roles for '%s' / '%s' (%d):\n\n", productName, teamName, len(roles))
	fmt.Println("  ID      PERSON                         ROLE          SCOPE         ACTIVE")
	fmt.Println("  --      ------                         ----          -----         ------")

	for _, r := range roles {
		scope := "-"
		if r.Scope != "" {
			scope = r.Scope
		}
		activeStr := "yes"
		if !r.IsActive {
			activeStr = "\033[90mno\033[0m"
		}
		fmt.Printf("  %-6d  %-30s %-13s %-13s %s\n",
			r.ID,
			truncateString(r.PersonName, 30),
			r.Role,
			truncateString(scope, 13),
			activeStr)
	}

	fmt.Println()
	return nil
}

func outputRoleSearchResults(cfg *config.CLIConfig, roles []*products.ProductTeamRole) error {
	format := getProductOutputFormat(cfg)

	switch format {
	case config.OutputFormatJSON:
		return outputProductJSON(roles)
	case config.OutputFormatYAML:
		return outputProductYAML(roles)
	default:
		return outputRoleSearchResultsTable(roles)
	}
}

func outputRoleSearchResultsTable(roles []*products.ProductTeamRole) error {
	if len(roles) == 0 {
		fmt.Println("No matching roles found.")
		return nil
	}

	fmt.Printf("Found %d role(s):\n\n", len(roles))
	fmt.Println("  ID      PRODUCT              TEAM                 PERSON                    ROLE          SCOPE")
	fmt.Println("  --      -------              ----                 ------                    ----          -----")

	for _, r := range roles {
		scope := "-"
		if r.Scope != "" {
			scope = r.Scope
		}
		fmt.Printf("  %-6d  %-20s %-20s %-25s %-13s %s\n",
			r.ID,
			truncateString(r.ProductName, 20),
			truncateString(r.TeamName, 20),
			truncateString(r.PersonName, 25),
			r.Role,
			truncateString(scope, 13))
	}

	fmt.Println()
	return nil
}

func outputProductPeople(cfg *config.CLIConfig, productName string, roles []*products.ProductTeamRole) error {
	format := getProductOutputFormat(cfg)

	switch format {
	case config.OutputFormatJSON:
		return outputProductJSON(roles)
	case config.OutputFormatYAML:
		return outputProductYAML(roles)
	default:
		return outputProductPeopleTable(productName, roles)
	}
}

func outputProductPeopleTable(productName string, roles []*products.ProductTeamRole) error {
	if len(roles) == 0 {
		fmt.Printf("No people assigned to '%s'\n", productName)
		return nil
	}

	fmt.Printf("People on '%s' (%d):\n\n", productName, len(roles))
	fmt.Println("  TEAM                 PERSON                         ROLE          SCOPE         EMAIL")
	fmt.Println("  ----                 ------                         ----          -----         -----")

	for _, r := range roles {
		scope := "-"
		if r.Scope != "" {
			scope = r.Scope
		}
		fmt.Printf("  %-20s %-30s %-13s %-13s %s\n",
			truncateString(r.TeamName, 20),
			truncateString(r.PersonName, 30),
			r.Role,
			truncateString(scope, 13),
			r.PersonEmail)
	}

	fmt.Println()
	return nil
}

