// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/pkg/products"
)

// Product command flags.
var (
	productTenant      string
	productFormat      string
	productParent      string
	productType        string
	productStatus      string
	productDescription string
	productKeywords    []string
)

// ProductCommandDeps holds the dependencies for product commands.
type ProductCommandDeps struct {
	Config     *config.CLIConfig
	Pool       *pgxpool.Pool
	Repository *products.Repository
	LoadConfig func() (*config.CLIConfig, error)
	InitPool   func(*config.CLIConfig) (*pgxpool.Pool, error)
}

// DefaultProductDeps returns the default dependencies for production use.
func DefaultProductDeps() *ProductCommandDeps {
	return &ProductCommandDeps{
		LoadConfig: config.LoadConfig,
		InitPool:   initProductPool,
	}
}

// initProductPool creates a database pool for product operations.
// Requires PENFOLD_DB_* environment variables to be set.
// See context/infrastructure.md for connection details.
func initProductPool(cfg *config.CLIConfig) (*pgxpool.Pool, error) {
	// All database config comes from environment variables - no defaults for credentials.
	host := os.Getenv("PENFOLD_DB_HOST")
	port := os.Getenv("PENFOLD_DB_PORT")
	user := os.Getenv("PENFOLD_DB_USER")
	password := os.Getenv("PENFOLD_DB_PASSWORD")
	dbname := os.Getenv("PENFOLD_DB_NAME")

	// Validate required env vars.
	var missing []string
	if host == "" {
		missing = append(missing, "PENFOLD_DB_HOST")
	}
	if user == "" {
		missing = append(missing, "PENFOLD_DB_USER")
	}
	if password == "" {
		missing = append(missing, "PENFOLD_DB_PASSWORD")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s\nSee context/infrastructure.md for setup", strings.Join(missing, ", "))
	}

	// Apply defaults for non-sensitive values.
	if port == "" {
		port = "5432"
	}
	if dbname == "" {
		dbname = "penfold"
	}

	// Use keyword-value format to avoid URL encoding issues with special chars.
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return pool, nil
}

// NewProductCommand creates the root product command with all subcommands.
func NewProductCommand(deps *ProductCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultProductDeps()
	}

	cmd := &cobra.Command{
		Use:   "product",
		Short: "Manage products and organizational entities",
		Long: `Manage products and organizational entities in Penfold.

Products represent business products, sub-products, and features organized in
a hierarchy. Each product can have team associations, scoped roles, and timeline
events for tracking decisions, milestones, and changes.

Product Types:
  - product:     Top-level business product (e.g., "LKE", "Managed Databases")
  - sub_product: Sub-product or component (e.g., "LKE Enterprise")
  - feature:     Specific feature (e.g., "Node Pools", "Auto-scaling")

Status Values:
  - active:      Currently active and supported
  - beta:        In beta testing
  - sunset:      Being phased out
  - deprecated:  No longer supported

Examples:
  # List all products
  penf product list

  # Add a new product
  penf product add "My Product" --type product --description "Description"

  # Show product details
  penf product info "My Product"

  # Show product hierarchy
  penf product hierarchy "My Product"`,
		Aliases: []string{"prod", "products"},
	}

	// Add persistent flags.
	cmd.PersistentFlags().StringVarP(&productTenant, "tenant", "t", "", "Tenant ID (overrides config)")
	cmd.PersistentFlags().StringVarP(&productFormat, "format", "f", "", "Output format: table, json, yaml")

	// Add subcommands.
	cmd.AddCommand(newProductListCommand(deps))
	cmd.AddCommand(newProductAddCommand(deps))
	cmd.AddCommand(newProductInfoCommand(deps))
	cmd.AddCommand(newProductHierarchyCommand(deps))
	cmd.AddCommand(newProductAliasCommand(deps))

	return cmd
}

// newProductListCommand creates the 'product list' subcommand.
func newProductListCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all products",
		Long: `List all products in the system.

By default, lists top-level products. Use --parent to list children of a
specific product, or --all to list all products regardless of hierarchy.

Examples:
  # List top-level products
  penf product list

  # List all products
  penf product list --all

  # List children of a specific product
  penf product list --parent "LKE"

  # Filter by type
  penf product list --type sub_product

  # Filter by status
  penf product list --status active`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductList(cmd.Context(), deps)
		},
	}

	cmd.Flags().StringVar(&productParent, "parent", "", "Filter by parent product name")
	cmd.Flags().StringVar(&productType, "type", "", "Filter by product type (product, sub_product, feature)")
	cmd.Flags().StringVar(&productStatus, "status", "", "Filter by status (active, beta, sunset, deprecated)")
	cmd.Flags().Bool("all", false, "List all products (not just top-level)")

	return cmd
}

// newProductAddCommand creates the 'product add' subcommand.
func newProductAddCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new product",
		Long: `Add a new product to the system.

Creates a new product with the specified name and options. By default,
products are created as top-level 'product' type with 'active' status.

Examples:
  # Add a simple product
  penf product add "My Product"

  # Add with full details
  penf product add "My Product" \
    --type product \
    --status active \
    --description "My product description" \
    --keywords "api,backend,core"

  # Add a sub-product
  penf product add "Sub Product" --parent "My Product" --type sub_product

  # Add a feature
  penf product add "Cool Feature" --parent "Sub Product" --type feature`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductAdd(cmd.Context(), deps, args[0])
		},
	}

	cmd.Flags().StringVar(&productParent, "parent", "", "Parent product name (for sub-products/features)")
	cmd.Flags().StringVar(&productType, "type", "product", "Product type: product, sub_product, feature")
	cmd.Flags().StringVar(&productStatus, "status", "active", "Status: active, beta, sunset, deprecated")
	cmd.Flags().StringVar(&productDescription, "description", "", "Product description")
	cmd.Flags().StringSliceVar(&productKeywords, "keywords", nil, "Keywords (comma-separated)")

	return cmd
}

// newProductInfoCommand creates the 'product info' subcommand.
func newProductInfoCommand(deps *ProductCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show product details",
		Long: `Show detailed information about a specific product.

Displays the product's properties, aliases, and associated teams.
The name can be either the product name or an alias.

Examples:
  penf product info "My Product"
  penf product info "MP"  # Using an alias`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductInfo(cmd.Context(), deps, args[0])
		},
	}
}

// newProductHierarchyCommand creates the 'product hierarchy' subcommand.
func newProductHierarchyCommand(deps *ProductCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "hierarchy <name>",
		Short: "Show product hierarchy tree",
		Long: `Show the hierarchy tree starting from a product.

Displays the product and all its descendants in a tree format,
showing the parent-child relationships.

Examples:
  penf product hierarchy "LKE"

Output format:
  LKE (product)
  ├── LKE Enterprise (sub_product)
  │   ├── Node Pools (feature)
  │   └── Auto-scaling (feature)
  └── LKE Standard (sub_product)`,
		Aliases: []string{"tree"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductHierarchy(cmd.Context(), deps, args[0])
		},
	}
}

// newProductAliasCommand creates the 'product alias' subcommand group.
func newProductAliasCommand(deps *ProductCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: "Manage product aliases",
		Long: `Manage aliases for products.

Aliases allow products to be referenced by alternative names.
For example, "LKE" could have an alias "Kubernetes Engine".`,
	}

	cmd.AddCommand(newProductAliasAddCommand(deps))
	cmd.AddCommand(newProductAliasRemoveCommand(deps))
	cmd.AddCommand(newProductAliasListCommand(deps))

	return cmd
}

// newProductAliasAddCommand creates the 'product alias add' subcommand.
func newProductAliasAddCommand(deps *ProductCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "add <product> <alias>",
		Short: "Add an alias to a product",
		Long: `Add an alternative name (alias) to a product.

Example:
  penf product alias add "LKE" "Kubernetes Engine"`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductAliasAdd(cmd.Context(), deps, args[0], args[1])
		},
	}
}

// newProductAliasRemoveCommand creates the 'product alias remove' subcommand.
func newProductAliasRemoveCommand(deps *ProductCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <product> <alias>",
		Short: "Remove an alias from a product",
		Long: `Remove an alias from a product.

Example:
  penf product alias remove "LKE" "Kubernetes Engine"`,
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductAliasRemove(cmd.Context(), deps, args[0], args[1])
		},
	}
}

// newProductAliasListCommand creates the 'product alias list' subcommand.
func newProductAliasListCommand(deps *ProductCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "list <product>",
		Short: "List aliases for a product",
		Long: `List all aliases for a product.

Example:
  penf product alias list "LKE"`,
		Aliases: []string{"ls"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProductAliasList(cmd.Context(), deps, args[0])
		},
	}
}

// ==================== Command Execution Functions ====================

// initProductDeps initializes the dependencies (config, pool, repository).
func initProductDeps(ctx context.Context, deps *ProductCommandDeps) error {
	// Load config if not already loaded.
	if deps.Config == nil {
		cfg, err := deps.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
		deps.Config = cfg
	}

	// Initialize pool if not already initialized.
	if deps.Pool == nil {
		pool, err := deps.InitPool(deps.Config)
		if err != nil {
			return fmt.Errorf("initializing database pool: %w", err)
		}
		deps.Pool = pool
	}

	// Initialize repository if not already initialized.
	if deps.Repository == nil {
		logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
			Level(zerolog.InfoLevel).
			With().
			Timestamp().
			Logger()
		deps.Repository = products.NewRepository(deps.Pool, logger)
	}

	return nil
}

// getTenantIDForProduct returns the tenant ID from flag, env, or config.
func getTenantIDForProduct(deps *ProductCommandDeps) string {
	if productTenant != "" {
		return productTenant
	}
	if envTenant := os.Getenv("PENF_TENANT_ID"); envTenant != "" {
		return envTenant
	}
	if deps.Config != nil && deps.Config.TenantID != "" {
		return deps.Config.TenantID
	}
	// Default tenant.
	return "00000001-0000-0000-0000-000000000001"
}

// runProductList executes the product list command.
func runProductList(ctx context.Context, deps *ProductCommandDeps) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)
	repo := deps.Repository

	var productsList []*products.Product
	var err error

	// Determine listing mode.
	listAll := false
	if cmd := ctx.Value("cmd"); cmd != nil {
		if c, ok := cmd.(*cobra.Command); ok {
			listAll, _ = c.Flags().GetBool("all")
		}
	}

	if productParent != "" {
		// List children of a specific product.
		parent, err := repo.ResolveProduct(ctx, tenantID, productParent)
		if err != nil {
			return fmt.Errorf("resolving parent product '%s': %w", productParent, err)
		}
		productsList, err = repo.ListChildren(ctx, parent.ID)
		if err != nil {
			return fmt.Errorf("listing children: %w", err)
		}
	} else if listAll {
		// List all products with optional filters.
		filter := products.ProductFilter{
			TenantID: tenantID,
		}
		if productType != "" {
			pt := products.ProductType(productType)
			filter.ProductType = &pt
		}
		if productStatus != "" {
			ps := products.ProductStatus(productStatus)
			filter.Status = &ps
		}
		productsList, err = repo.ListProducts(ctx, filter)
		if err != nil {
			return fmt.Errorf("listing products: %w", err)
		}
	} else {
		// List top-level products by default.
		productsList, err = repo.ListTopLevelProducts(ctx, tenantID)
		if err != nil {
			return fmt.Errorf("listing top-level products: %w", err)
		}
	}

	return outputProducts(deps.Config, productsList)
}

// runProductAdd executes the product add command.
func runProductAdd(ctx context.Context, deps *ProductCommandDeps, name string) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)
	repo := deps.Repository

	// Validate product type.
	pt := products.ProductType(productType)
	switch pt {
	case products.ProductTypeProduct, products.ProductTypeSubProduct, products.ProductTypeFeature:
		// Valid.
	default:
		return fmt.Errorf("invalid product type: %s (must be product, sub_product, or feature)", productType)
	}

	// Validate status.
	ps := products.ProductStatus(productStatus)
	switch ps {
	case products.ProductStatusActive, products.ProductStatusBeta, products.ProductStatusSunset, products.ProductStatusDeprecated:
		// Valid.
	default:
		return fmt.Errorf("invalid status: %s (must be active, beta, sunset, or deprecated)", productStatus)
	}

	// Resolve parent if specified.
	var parentID *int64
	if productParent != "" {
		parent, err := repo.ResolveProduct(ctx, tenantID, productParent)
		if err != nil {
			return fmt.Errorf("resolving parent product '%s': %w", productParent, err)
		}
		parentID = &parent.ID
	}

	// Create the product.
	var desc *string
	if productDescription != "" {
		desc = &productDescription
	}
	product := &products.Product{
		TenantID:    tenantID,
		Name:        name,
		Description: desc,
		ParentID:    parentID,
		ProductType: pt,
		Status:      ps,
		Keywords:    productKeywords,
	}

	if err := repo.CreateProduct(ctx, product); err != nil {
		return fmt.Errorf("creating product: %w", err)
	}

	fmt.Printf("\033[32mCreated product:\033[0m %s (ID: %d)\n", name, product.ID)
	return nil
}

// runProductInfo executes the product info command.
func runProductInfo(ctx context.Context, deps *ProductCommandDeps, name string) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)
	repo := deps.Repository

	// Resolve product by name or alias.
	product, err := repo.ResolveProduct(ctx, tenantID, name)
	if err != nil {
		return fmt.Errorf("product not found: %s", name)
	}

	// Get aliases.
	aliases, err := repo.GetAliases(ctx, product.ID)
	if err != nil {
		// Non-fatal, just log.
		aliases = nil
	}

	// Get parent if exists.
	var parentName string
	if product.ParentID != nil {
		parent, err := repo.GetProductByID(ctx, *product.ParentID)
		if err == nil {
			parentName = parent.Name
		}
	}

	return outputProductInfo(deps.Config, product, aliases, parentName)
}

// runProductHierarchy executes the product hierarchy command.
func runProductHierarchy(ctx context.Context, deps *ProductCommandDeps, name string) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)
	repo := deps.Repository

	// Resolve product.
	product, err := repo.ResolveProduct(ctx, tenantID, name)
	if err != nil {
		return fmt.Errorf("product not found: %s", name)
	}

	// Get hierarchy.
	hierarchy, err := repo.GetHierarchy(ctx, product.ID)
	if err != nil {
		return fmt.Errorf("getting hierarchy: %w", err)
	}

	return outputProductHierarchy(deps.Config, hierarchy)
}

// runProductAliasAdd adds an alias to a product.
func runProductAliasAdd(ctx context.Context, deps *ProductCommandDeps, productName, alias string) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)
	repo := deps.Repository

	// Resolve product.
	product, err := repo.ResolveProduct(ctx, tenantID, productName)
	if err != nil {
		return fmt.Errorf("product not found: %s", productName)
	}

	// Add alias.
	if err := repo.AddAlias(ctx, product.ID, alias); err != nil {
		return fmt.Errorf("adding alias: %w", err)
	}

	fmt.Printf("\033[32mAdded alias:\033[0m '%s' -> %s\n", alias, product.Name)
	return nil
}

// runProductAliasRemove removes an alias from a product.
func runProductAliasRemove(ctx context.Context, deps *ProductCommandDeps, productName, alias string) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)
	repo := deps.Repository

	// Resolve product.
	product, err := repo.ResolveProduct(ctx, tenantID, productName)
	if err != nil {
		return fmt.Errorf("product not found: %s", productName)
	}

	// Remove alias.
	if err := repo.RemoveAlias(ctx, product.ID, alias); err != nil {
		return fmt.Errorf("removing alias: %w", err)
	}

	fmt.Printf("\033[32mRemoved alias:\033[0m '%s' from %s\n", alias, product.Name)
	return nil
}

// runProductAliasList lists aliases for a product.
func runProductAliasList(ctx context.Context, deps *ProductCommandDeps, productName string) error {
	if err := initProductDeps(ctx, deps); err != nil {
		return err
	}
	defer deps.Pool.Close()

	tenantID := getTenantIDForProduct(deps)
	repo := deps.Repository

	// Resolve product.
	product, err := repo.ResolveProduct(ctx, tenantID, productName)
	if err != nil {
		return fmt.Errorf("product not found: %s", productName)
	}

	// Get aliases.
	aliases, err := repo.GetAliases(ctx, product.ID)
	if err != nil {
		return fmt.Errorf("getting aliases: %w", err)
	}

	return outputProductAliases(deps.Config, product.Name, aliases)
}

// ==================== Output Functions ====================

// getProductOutputFormat returns the output format from flag or config.
func getProductOutputFormat(cfg *config.CLIConfig) config.OutputFormat {
	if productFormat != "" {
		return config.OutputFormat(productFormat)
	}
	if cfg != nil {
		return cfg.OutputFormat
	}
	return config.OutputFormatText
}

// outputProducts outputs a list of products.
func outputProducts(cfg *config.CLIConfig, productsList []*products.Product) error {
	format := getProductOutputFormat(cfg)

	switch format {
	case config.OutputFormatJSON:
		return outputProductJSON(productsList)
	case config.OutputFormatYAML:
		return outputProductYAML(productsList)
	default:
		return outputProductsTable(productsList)
	}
}

// outputProductsTable outputs products in table format.
func outputProductsTable(productsList []*products.Product) error {
	if len(productsList) == 0 {
		fmt.Println("No products found.")
		return nil
	}

	fmt.Printf("Products (%d):\n\n", len(productsList))
	fmt.Println("  ID      NAME                           TYPE          STATUS")
	fmt.Println("  --      ----                           ----          ------")

	for _, p := range productsList {
		typeColor := getProductTypeColor(p.ProductType)
		statusColor := getProductStatusColor(p.Status)
		fmt.Printf("  %-6d  %-30s %s%-12s\033[0m  %s%-10s\033[0m\n",
			p.ID,
			truncateString(p.Name, 30),
			typeColor,
			p.ProductType,
			statusColor,
			p.Status)
	}

	fmt.Println()
	return nil
}

// outputProductInfo outputs detailed product information.
func outputProductInfo(cfg *config.CLIConfig, product *products.Product, aliases []*products.ProductAlias, parentName string) error {
	format := getProductOutputFormat(cfg)

	switch format {
	case config.OutputFormatJSON:
		return outputProductJSON(product)
	case config.OutputFormatYAML:
		return outputProductYAML(product)
	default:
		return outputProductInfoText(product, aliases, parentName)
	}
}

// outputProductInfoText outputs product info in human-readable format.
func outputProductInfoText(product *products.Product, aliases []*products.ProductAlias, parentName string) error {
	typeColor := getProductTypeColor(product.ProductType)
	statusColor := getProductStatusColor(product.Status)

	fmt.Println("Product Details:")
	fmt.Println()
	fmt.Printf("  \033[1mID:\033[0m          %d\n", product.ID)
	fmt.Printf("  \033[1mName:\033[0m        %s\n", product.Name)
	fmt.Printf("  \033[1mType:\033[0m        %s%s\033[0m\n", typeColor, product.ProductType)
	fmt.Printf("  \033[1mStatus:\033[0m      %s%s\033[0m\n", statusColor, product.Status)
	fmt.Println()

	if product.Description != nil && *product.Description != "" {
		fmt.Printf("  \033[1mDescription:\033[0m\n    %s\n\n", *product.Description)
	}

	if parentName != "" {
		fmt.Printf("  \033[1mParent:\033[0m      %s\n", parentName)
	}

	if len(product.Keywords) > 0 {
		fmt.Printf("  \033[1mKeywords:\033[0m    %s\n", strings.Join(product.Keywords, ", "))
	}

	if len(aliases) > 0 {
		aliasNames := make([]string, len(aliases))
		for i, a := range aliases {
			aliasNames[i] = a.Alias
		}
		fmt.Printf("  \033[1mAliases:\033[0m     %s\n", strings.Join(aliasNames, ", "))
	}

	fmt.Println()
	fmt.Printf("  \033[1mCreated:\033[0m     %s\n", product.CreatedAt.Format(time.RFC3339))
	fmt.Printf("  \033[1mUpdated:\033[0m     %s\n", product.UpdatedAt.Format(time.RFC3339))

	return nil
}

// outputProductHierarchy outputs the product hierarchy tree.
func outputProductHierarchy(cfg *config.CLIConfig, hierarchy []*products.ProductWithHierarchy) error {
	format := getProductOutputFormat(cfg)

	switch format {
	case config.OutputFormatJSON:
		return outputProductJSON(hierarchy)
	case config.OutputFormatYAML:
		return outputProductYAML(hierarchy)
	default:
		return outputProductHierarchyTree(hierarchy)
	}
}

// outputProductHierarchyTree outputs hierarchy as a tree.
func outputProductHierarchyTree(hierarchy []*products.ProductWithHierarchy) error {
	if len(hierarchy) == 0 {
		fmt.Println("No products in hierarchy.")
		return nil
	}

	fmt.Println("Product Hierarchy:")
	fmt.Println()

	for i, h := range hierarchy {
		prefix := ""
		for d := 0; d < h.Depth; d++ {
			prefix += "    "
		}

		// Determine tree character.
		treeChar := "├── "
		if h.Depth == 0 {
			treeChar = ""
		} else if i == len(hierarchy)-1 || (i+1 < len(hierarchy) && hierarchy[i+1].Depth <= h.Depth) {
			treeChar = "└── "
		}

		typeColor := getProductTypeColor(h.ProductType)
		statusColor := getProductStatusColor(h.Status)

		fmt.Printf("%s%s%s %s(%s)%s %s[%s]%s\n",
			prefix,
			treeChar,
			h.Name,
			typeColor,
			h.ProductType,
			"\033[0m",
			statusColor,
			h.Status,
			"\033[0m")
	}

	fmt.Println()
	return nil
}

// outputProductAliases outputs product aliases.
func outputProductAliases(cfg *config.CLIConfig, productName string, aliases []*products.ProductAlias) error {
	format := getProductOutputFormat(cfg)

	switch format {
	case config.OutputFormatJSON:
		return outputProductJSON(aliases)
	case config.OutputFormatYAML:
		return outputProductYAML(aliases)
	default:
		return outputProductAliasesText(productName, aliases)
	}
}

// outputProductAliasesText outputs aliases in text format.
func outputProductAliasesText(productName string, aliases []*products.ProductAlias) error {
	if len(aliases) == 0 {
		fmt.Printf("No aliases for product '%s'\n", productName)
		return nil
	}

	fmt.Printf("Aliases for '%s':\n\n", productName)
	for _, a := range aliases {
		fmt.Printf("  - %s\n", a.Alias)
	}
	fmt.Println()
	return nil
}

// Helper output functions.

func outputProductJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func outputProductYAML(v interface{}) error {
	enc := yaml.NewEncoder(os.Stdout)
	return enc.Encode(v)
}

// Color helpers for product output.

func getProductTypeColor(pt products.ProductType) string {
	switch pt {
	case products.ProductTypeProduct:
		return "\033[35m" // Magenta.
	case products.ProductTypeSubProduct:
		return "\033[36m" // Cyan.
	case products.ProductTypeFeature:
		return "\033[34m" // Blue.
	default:
		return ""
	}
}

func getProductStatusColor(ps products.ProductStatus) string {
	switch ps {
	case products.ProductStatusActive:
		return "\033[32m" // Green.
	case products.ProductStatusBeta:
		return "\033[33m" // Yellow.
	case products.ProductStatusSunset:
		return "\033[31m" // Red.
	case products.ProductStatusDeprecated:
		return "\033[90m" // Gray.
	default:
		return ""
	}
}
