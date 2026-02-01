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

	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	"github.com/otherjamesbrown/penfold/cmd/penf/client"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// Relationship types and structures.

// EntityType represents the type of an entity in the relationship graph.
type EntityType string

const (
	EntityTypePerson       EntityType = "person"
	EntityTypeOrganization EntityType = "organization"
	EntityTypeTopic        EntityType = "topic"
	EntityTypeProject      EntityType = "project"
	EntityTypeLocation     EntityType = "location"
)

// RelationshipType represents the type of relationship between entities.
type RelationshipType string

const (
	RelationshipTypeColleague RelationshipType = "colleague"
	RelationshipTypeReportsTo RelationshipType = "reports_to"
	RelationshipTypeMemberOf  RelationshipType = "member_of"
	RelationshipTypeWorksOn   RelationshipType = "works_on"
	RelationshipTypeDiscusses RelationshipType = "discusses"
	RelationshipTypeMentions  RelationshipType = "mentions"
	RelationshipTypeLocatedAt RelationshipType = "located_at"
	RelationshipTypeRelatedTo RelationshipType = "related_to"
)

// ConflictResolutionStrategy defines how to resolve relationship conflicts.
type ConflictResolutionStrategy string

const (
	ConflictStrategyKeepLatest ConflictResolutionStrategy = "keep_latest"
	ConflictStrategyKeepFirst  ConflictResolutionStrategy = "keep_first"
	ConflictStrategyMerge      ConflictResolutionStrategy = "merge"
	ConflictStrategyManual     ConflictResolutionStrategy = "manual"
)

// Entity represents a node in the relationship graph.
type Entity struct {
	ID            string            `json:"id" yaml:"id"`
	Name          string            `json:"name" yaml:"name"`
	Type          EntityType        `json:"type" yaml:"type"`
	Aliases       []string          `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	Confidence    float64           `json:"confidence" yaml:"confidence"`
	SourceCount   int               `json:"source_count" yaml:"source_count"`
	FirstSeen     time.Time         `json:"first_seen" yaml:"first_seen"`
	LastSeen      time.Time         `json:"last_seen" yaml:"last_seen"`
	Metadata      map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	RelationCount int               `json:"relation_count" yaml:"relation_count"`
}

// Relationship represents an edge in the relationship graph.
type Relationship struct {
	ID          string           `json:"id" yaml:"id"`
	SourceID    string           `json:"source_id" yaml:"source_id"`
	SourceName  string           `json:"source_name" yaml:"source_name"`
	TargetID    string           `json:"target_id" yaml:"target_id"`
	TargetName  string           `json:"target_name" yaml:"target_name"`
	Type        RelationshipType `json:"type" yaml:"type"`
	Confidence  float64          `json:"confidence" yaml:"confidence"`
	Weight      float64          `json:"weight" yaml:"weight"`
	Evidence    []string         `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	FirstSeen   time.Time        `json:"first_seen" yaml:"first_seen"`
	LastSeen    time.Time        `json:"last_seen" yaml:"last_seen"`
	SourceCount int              `json:"source_count" yaml:"source_count"`
}

// RelationshipConflict represents a detected conflict between relationships.
type RelationshipConflict struct {
	ID              string         `json:"id" yaml:"id"`
	Type            string         `json:"type" yaml:"type"`
	Description     string         `json:"description" yaml:"description"`
	Relationships   []Relationship `json:"relationships" yaml:"relationships"`
	SuggestedAction string         `json:"suggested_action" yaml:"suggested_action"`
	CreatedAt       time.Time      `json:"created_at" yaml:"created_at"`
	Status          string         `json:"status" yaml:"status"`
}

// NetworkCluster represents a cluster in the relationship network.
type NetworkCluster struct {
	ID          string   `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	EntityCount int      `json:"entity_count" yaml:"entity_count"`
	TopEntities []Entity `json:"top_entities" yaml:"top_entities"`
	Density     float64  `json:"density" yaml:"density"`
}

// RelationshipCommandDeps holds the dependencies for relationship commands.
type RelationshipCommandDeps struct {
	Config             *config.CLIConfig
	GRPCClient         *client.GRPCClient
	RelationshipClient *client.RelationshipClient
	OutputFormat       config.OutputFormat
	LoadConfig         func() (*config.CLIConfig, error)
	InitClient         func(*config.CLIConfig) (*client.GRPCClient, error)
	InitRelClient      func(*config.CLIConfig) (*client.RelationshipClient, error)
}

// DefaultRelationshipDeps returns the default dependencies for production use.
func DefaultRelationshipDeps() *RelationshipCommandDeps {
	return &RelationshipCommandDeps{
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
		InitRelClient: func(cfg *config.CLIConfig) (*client.RelationshipClient, error) {
			opts := client.DefaultOptions()
			opts.Insecure = cfg.Insecure
			opts.Debug = cfg.Debug
			opts.ConnectTimeout = cfg.Timeout
			opts.TenantID = cfg.TenantID

			if !cfg.Insecure {
				tlsConfig, err := client.LoadClientTLSConfig(&cfg.TLS)
				if err != nil {
					return nil, fmt.Errorf("failed to load TLS config: %w", err)
				}
				opts.TLSConfig = tlsConfig
			}

			relClient := client.NewRelationshipClient(cfg.ServerAddress, opts)
			ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
			defer cancel()

			if err := relClient.Connect(ctx); err != nil {
				return nil, fmt.Errorf("connecting to relationship service: %w", err)
			}
			return relClient, nil
		},
	}
}

// Relationship command flags.
var (
	relationshipTenant        string
	relationshipOutput        string
	relationshipLimit         int
	relationshipConfidenceMin float64
	relationshipType          string
	relationshipEntityType    string
	conflictStrategy          string
)

// getRelInsecureFlag retrieves the --insecure flag from the command's root.
func getRelInsecureFlag(cmd *cobra.Command) bool {
	root := cmd
	for root.Parent() != nil {
		root = root.Parent()
	}
	insecure, _ := root.PersistentFlags().GetBool("insecure")
	return insecure
}

// NewRelationshipCommand creates the root relationship command with all subcommands.
func NewRelationshipCommand(deps *RelationshipCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultRelationshipDeps()
	}

	cmd := &cobra.Command{
		Use:   "relationship",
		Short: "Manage relationships and entities in the knowledge graph",
		Long: `Manage relationships and entities in the Penfold knowledge graph.

Penfold automatically discovers and tracks relationships between entities
(people, organizations, topics, projects) found in your content. Use these
commands to explore, manage, and resolve conflicts in the relationship graph.

Entity Types:
  - person:       Individual people mentioned in content
  - organization: Companies, teams, departments
  - topic:        Subjects, themes, keywords
  - project:      Projects, initiatives, programs
  - location:     Places, offices, regions

Relationship Types:
  - colleague:    Works with
  - reports_to:   Hierarchical relationship
  - member_of:    Belongs to organization/team
  - works_on:     Associated with project
  - discusses:    Talks about topic
  - mentions:     References entity
  - located_at:   Physical location
  - related_to:   General association

Documentation:
  People entities:        docs/concepts/people.md (how people are resolved)
  Entity types:           docs/concepts/entities.md (all entity types)
  Mention resolution:     docs/concepts/mentions.md (how mentions become entities)
  Use case:               docs/shared/use-cases.md (UC-3: Expertise Discovery)`,
		Aliases: []string{"rel", "relations"},
	}

	// Add persistent flags.
	cmd.PersistentFlags().StringVarP(&relationshipTenant, "tenant", "t", "", "Tenant ID (overrides config)")
	cmd.PersistentFlags().StringVarP(&relationshipOutput, "output", "o", "", "Output format: table, json, yaml")
	cmd.PersistentFlags().IntVarP(&relationshipLimit, "limit", "l", 20, "Maximum number of results")
	cmd.PersistentFlags().Float64Var(&relationshipConfidenceMin, "confidence-min", 0.0, "Minimum confidence threshold (0.0-1.0)")

	// Add subcommands.
	cmd.AddCommand(newRelationshipListCommand(deps))
	cmd.AddCommand(newRelationshipShowCommand(deps))
	cmd.AddCommand(newRelationshipSearchCommand(deps))
	cmd.AddCommand(newRelationshipEntityCommand(deps))
	cmd.AddCommand(newRelationshipNetworkCommand(deps))
	cmd.AddCommand(newRelationshipConflictCommand(deps))

	return cmd
}

// newRelationshipListCommand creates the 'relationship list' subcommand.
func newRelationshipListCommand(deps *RelationshipCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List relationships in the knowledge graph",
		Long: `List relationships in the Penfold knowledge graph.

Displays relationships between entities with their confidence scores and
relationship types. Use filters to narrow down the results.

Examples:
  # List all relationships
  penf relationship list

  # List relationships with minimum confidence
  penf relationship list --confidence-min 0.8

  # Filter by relationship type
  penf relationship list --type colleague

  # Output as JSON
  penf relationship list --format json`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRelationshipList(cmd.Context(), deps, getRelInsecureFlag(cmd))
		},
	}

	cmd.Flags().StringVar(&relationshipType, "type", "", "Filter by relationship type")

	return cmd
}

// newRelationshipShowCommand creates the 'relationship show' subcommand.
func newRelationshipShowCommand(deps *RelationshipCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <relationship-id>",
		Short: "Show details of a specific relationship",
		Long: `Show detailed information about a specific relationship.

Displays full relationship details including evidence, timestamps,
and confidence scores.

Example:
  penf relationship show rel-abc123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRelationshipShow(cmd.Context(), deps, args[0], getRelInsecureFlag(cmd))
		},
	}
}

// newRelationshipSearchCommand creates the 'relationship search' subcommand.
func newRelationshipSearchCommand(deps *RelationshipCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search relationships by entity name or type",
		Long: `Search relationships in the knowledge graph.

Searches for relationships involving entities matching the query string.
The search matches against entity names and aliases.

Examples:
  # Search for relationships involving "John"
  penf relationship search "John"

  # Search with confidence threshold
  penf relationship search "Alice" --confidence-min 0.7`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			return runRelationshipSearch(cmd.Context(), deps, query, getRelInsecureFlag(cmd))
		},
	}
}

// newRelationshipEntityCommand creates the 'relationship entity' subcommand group.
func newRelationshipEntityCommand(deps *RelationshipCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "entity",
		Short: "Manage entities in the knowledge graph",
		Long: `Manage entities (people, organizations, topics, etc.) in the knowledge graph.

Entities are the nodes in the relationship graph. They are automatically
discovered from your content and can be merged when duplicates are detected.`,
		Aliases: []string{"ent", "entities"},
	}

	cmd.AddCommand(newEntityListCommand(deps))
	cmd.AddCommand(newEntityShowCommand(deps))
	cmd.AddCommand(newEntityMergeCommand(deps))

	return cmd
}

// newEntityListCommand creates the 'relationship entity list' subcommand.
func newEntityListCommand(deps *RelationshipCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List entities in the knowledge graph",
		Long: `List entities in the Penfold knowledge graph.

Displays entities with their types, confidence scores, and relationship counts.

Examples:
  # List all entities
  penf relationship entity list

  # List only person entities
  penf relationship entity list --type person

  # List entities with minimum confidence
  penf relationship entity list --confidence-min 0.8`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEntityList(cmd.Context(), deps, getRelInsecureFlag(cmd))
		},
	}

	cmd.Flags().StringVar(&relationshipEntityType, "type", "", "Filter by entity type (person, organization, topic, project, location)")

	return cmd
}

// newEntityShowCommand creates the 'relationship entity show' subcommand.
func newEntityShowCommand(deps *RelationshipCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <entity-id>",
		Short: "Show details of a specific entity",
		Long: `Show detailed information about a specific entity.

Displays the entity's properties, aliases, metadata, and related relationships.

Example:
  penf relationship entity show ent-abc123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEntityShow(cmd.Context(), deps, args[0], getRelInsecureFlag(cmd))
		},
	}
}

// newEntityMergeCommand creates the 'relationship entity merge' subcommand.
func newEntityMergeCommand(deps *RelationshipCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "merge <entity-id-1> <entity-id-2>",
		Short: "Merge two entities into one",
		Long: `Merge two entities, combining their properties and relationships.

When duplicate entities are discovered, use this command to merge them.
The first entity will be the primary, and the second will be merged into it.
All relationships of the second entity will be transferred to the first.

Example:
  penf relationship entity merge ent-abc123 ent-def456`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEntityMerge(cmd.Context(), deps, args[0], args[1], getRelInsecureFlag(cmd))
		},
	}
}

// newRelationshipNetworkCommand creates the 'relationship network' subcommand group.
func newRelationshipNetworkCommand(deps *RelationshipCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network",
		Short: "Analyze the relationship network",
		Long: `Analyze the structure and patterns of the relationship network.

Provides insights into the connectivity, centrality, and clustering
of entities in the knowledge graph.`,
		Aliases: []string{"net"},
	}

	cmd.AddCommand(newNetworkGraphCommand(deps))
	cmd.AddCommand(newNetworkCentralCommand(deps))
	cmd.AddCommand(newNetworkClustersCommand(deps))

	return cmd
}

// newNetworkGraphCommand creates the 'relationship network graph' subcommand.
func newNetworkGraphCommand(deps *RelationshipCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "graph",
		Short: "Display the relationship graph structure",
		Long: `Display an overview of the relationship graph structure.

Shows the overall network topology including node counts, edge counts,
density, and other graph metrics.

Example:
  penf relationship network graph`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNetworkGraph(cmd.Context(), deps, getRelInsecureFlag(cmd))
		},
	}
}

// newNetworkCentralCommand creates the 'relationship network central' subcommand.
func newNetworkCentralCommand(deps *RelationshipCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "central",
		Short: "Show the most connected entities",
		Long: `Show the most connected (central) entities in the network.

Identifies the key entities that have the most relationships,
often representing important people, topics, or organizations.

Example:
  penf relationship network central --limit 10`,
		Aliases: []string{"top", "hub"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNetworkCentral(cmd.Context(), deps, getRelInsecureFlag(cmd))
		},
	}
}

// newNetworkClustersCommand creates the 'relationship network clusters' subcommand.
func newNetworkClustersCommand(deps *RelationshipCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "clusters",
		Short: "Show clusters in the relationship network",
		Long: `Show clusters (communities) in the relationship network.

Identifies groups of entities that are more connected to each other
than to the rest of the network, revealing natural groupings.

Example:
  penf relationship network clusters`,
		Aliases: []string{"communities", "groups"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNetworkClusters(cmd.Context(), deps, getRelInsecureFlag(cmd))
		},
	}
}

// newRelationshipConflictCommand creates the 'relationship conflict' subcommand group.
func newRelationshipConflictCommand(deps *RelationshipCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "conflict",
		Short: "Manage relationship conflicts",
		Long: `Manage conflicts in the relationship graph.

Conflicts occur when the system detects inconsistent or contradictory
relationships. Review and resolve these conflicts to maintain data quality.`,
		Aliases: []string{"conflicts"},
	}

	cmd.AddCommand(newConflictListCommand(deps))
	cmd.AddCommand(newConflictShowCommand(deps))
	cmd.AddCommand(newConflictResolveCommand(deps))

	return cmd
}

// newConflictListCommand creates the 'relationship conflict list' subcommand.
func newConflictListCommand(deps *RelationshipCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List detected relationship conflicts",
		Long: `List detected conflicts in the relationship graph.

Shows conflicts that require attention, such as duplicate entities,
contradictory relationships, or data quality issues.

Example:
  penf relationship conflict list`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConflictList(cmd.Context(), deps, getRelInsecureFlag(cmd))
		},
	}
}

// newConflictShowCommand creates the 'relationship conflict show' subcommand.
func newConflictShowCommand(deps *RelationshipCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <conflict-id>",
		Short: "Show details of a specific conflict",
		Long: `Show detailed information about a specific conflict.

Displays the conflicting relationships, suggested resolution,
and relevant evidence.

Example:
  penf relationship conflict show conf-abc123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConflictShow(cmd.Context(), deps, args[0], getRelInsecureFlag(cmd))
		},
	}
}

// newConflictResolveCommand creates the 'relationship conflict resolve' subcommand.
func newConflictResolveCommand(deps *RelationshipCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <conflict-id>",
		Short: "Resolve a relationship conflict",
		Long: `Resolve a relationship conflict using the specified strategy.

Resolution Strategies:
  - keep_latest: Keep the most recently updated relationship
  - keep_first:  Keep the first discovered relationship
  - merge:       Attempt to merge conflicting data
  - manual:      Mark for manual review (no automatic changes)

Example:
  penf relationship conflict resolve conf-abc123 --strategy merge`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConflictResolve(cmd.Context(), deps, args[0], ConflictResolutionStrategy(conflictStrategy), getRelInsecureFlag(cmd))
		},
	}

	cmd.Flags().StringVarP(&conflictStrategy, "strategy", "s", "keep_latest", "Resolution strategy: keep_latest, keep_first, merge, manual")

	return cmd
}

// Command execution functions.

// runRelationshipList executes the relationship list command.
func runRelationshipList(ctx context.Context, deps *RelationshipCommandDeps, insecureFlag bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override insecure if flag is set.
	if insecureFlag {
		cfg.Insecure = true
	}

	// Override tenant if specified.
	if relationshipTenant != "" {
		cfg.TenantID = relationshipTenant
	}

	// Initialize relationship client.
	relClient, err := deps.InitRelClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing relationship client: %w", err)
	}
	defer relClient.Close()

	// Build request.
	req := &client.ListRelationshipsRequest{
		TenantID:      cfg.TenantID,
		PageSize:      int32(relationshipLimit),
		MinConfidence: float32(relationshipConfidenceMin),
	}

	if relationshipType != "" {
		req.RelationshipType = stringToRelType(relationshipType)
	}

	// Get relationships via gRPC.
	rels, _, err := relClient.ListRelationships(ctx, req)
	if err != nil {
		return fmt.Errorf("listing relationships: %w", err)
	}

	// Convert to local types for output.
	relationships := make([]Relationship, len(rels))
	for i, r := range rels {
		relationships[i] = clientRelToLocal(r)
	}

	// Determine output format.
	format := cfg.OutputFormat
	if relationshipOutput != "" {
		format = config.OutputFormat(relationshipOutput)
	}

	return outputRelationships(format, relationships)
}

// runRelationshipShow executes the relationship show command.
func runRelationshipShow(ctx context.Context, deps *RelationshipCommandDeps, relationshipID string, insecureFlag bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override insecure if flag is set.
	if insecureFlag {
		cfg.Insecure = true
	}

	// Override tenant if specified.
	if relationshipTenant != "" {
		cfg.TenantID = relationshipTenant
	}

	// Initialize relationship client.
	relClient, err := deps.InitRelClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing relationship client: %w", err)
	}
	defer relClient.Close()

	// Get relationship details via gRPC.
	rel, err := relClient.GetRelationship(ctx, cfg.TenantID, relationshipID)
	if err != nil {
		return fmt.Errorf("getting relationship: %w", err)
	}

	relationship := clientRelToLocal(rel)

	format := cfg.OutputFormat
	if relationshipOutput != "" {
		format = config.OutputFormat(relationshipOutput)
	}

	return outputRelationshipDetail(format, relationship)
}

// runRelationshipSearch executes the relationship search command.
func runRelationshipSearch(ctx context.Context, deps *RelationshipCommandDeps, query string, insecureFlag bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override insecure if flag is set.
	if insecureFlag {
		cfg.Insecure = true
	}

	// Override tenant if specified.
	if relationshipTenant != "" {
		cfg.TenantID = relationshipTenant
	}

	// Initialize relationship client.
	relClient, err := deps.InitRelClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing relationship client: %w", err)
	}
	defer relClient.Close()

	// Search relationships via gRPC.
	rels, err := relClient.SearchRelationships(ctx, cfg.TenantID, query, int32(relationshipLimit))
	if err != nil {
		return fmt.Errorf("searching relationships: %w", err)
	}

	// Convert to local types for output.
	relationships := make([]Relationship, len(rels))
	for i, r := range rels {
		relationships[i] = clientRelToLocal(r)
	}

	format := cfg.OutputFormat
	if relationshipOutput != "" {
		format = config.OutputFormat(relationshipOutput)
	}

	return outputRelationships(format, relationships)
}

// runEntityList executes the entity list command.
func runEntityList(ctx context.Context, deps *RelationshipCommandDeps, insecureFlag bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override insecure if flag is set.
	if insecureFlag {
		cfg.Insecure = true
	}

	// Override tenant if specified.
	if relationshipTenant != "" {
		cfg.TenantID = relationshipTenant
	}

	// Initialize relationship client.
	relClient, err := deps.InitRelClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing relationship client: %w", err)
	}
	defer relClient.Close()

	// Build request.
	req := &client.ListEntitiesRequest{
		TenantID:      cfg.TenantID,
		PageSize:      int32(relationshipLimit),
		MinConfidence: float32(relationshipConfidenceMin),
	}

	if relationshipEntityType != "" {
		req.EntityType = stringToEntityType(relationshipEntityType)
	}

	// Get entities via gRPC.
	ents, _, err := relClient.ListEntities(ctx, req)
	if err != nil {
		return fmt.Errorf("listing entities: %w", err)
	}

	// Convert to local types for output.
	entities := make([]Entity, len(ents))
	for i, e := range ents {
		entities[i] = clientEntityToLocal(e)
	}

	format := cfg.OutputFormat
	if relationshipOutput != "" {
		format = config.OutputFormat(relationshipOutput)
	}

	return outputEntities(format, entities)
}

// runEntityShow executes the entity show command.
func runEntityShow(ctx context.Context, deps *RelationshipCommandDeps, entityID string, insecureFlag bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override insecure if flag is set.
	if insecureFlag {
		cfg.Insecure = true
	}

	// Override tenant if specified.
	if relationshipTenant != "" {
		cfg.TenantID = relationshipTenant
	}

	// Initialize relationship client.
	relClient, err := deps.InitRelClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing relationship client: %w", err)
	}
	defer relClient.Close()

	// Get entity details via gRPC.
	ent, err := relClient.GetEntity(ctx, cfg.TenantID, entityID)
	if err != nil {
		return fmt.Errorf("getting entity: %w", err)
	}

	entity := clientEntityToLocal(ent)

	format := cfg.OutputFormat
	if relationshipOutput != "" {
		format = config.OutputFormat(relationshipOutput)
	}

	return outputEntityDetail(format, entity)
}

// runEntityMerge executes the entity merge command.
func runEntityMerge(ctx context.Context, deps *RelationshipCommandDeps, entityID1, entityID2 string, insecureFlag bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override insecure if flag is set.
	if insecureFlag {
		cfg.Insecure = true
	}

	// Override tenant if specified.
	if relationshipTenant != "" {
		cfg.TenantID = relationshipTenant
	}

	// Initialize relationship client.
	relClient, err := deps.InitRelClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing relationship client: %w", err)
	}
	defer relClient.Close()

	fmt.Printf("Merging entity %s into %s...\n", entityID2, entityID1)

	// Merge entities via gRPC.
	_, transferred, err := relClient.MergeEntities(ctx, cfg.TenantID, entityID1, entityID2)
	if err != nil {
		return fmt.Errorf("merging entities: %w", err)
	}

	fmt.Printf("\n\033[32mSuccess!\033[0m Entities merged.\n")
	fmt.Printf("  Primary entity: %s\n", entityID1)
	fmt.Printf("  Merged entity:  %s (now archived)\n", entityID2)
	fmt.Printf("  Relationships transferred: %d\n", transferred)

	return nil
}

// runNetworkGraph executes the network graph command.
func runNetworkGraph(ctx context.Context, deps *RelationshipCommandDeps, insecureFlag bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override insecure if flag is set.
	if insecureFlag {
		cfg.Insecure = true
	}

	// Override tenant if specified.
	if relationshipTenant != "" {
		cfg.TenantID = relationshipTenant
	}

	// Initialize relationship client.
	relClient, err := deps.InitRelClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing relationship client: %w", err)
	}
	defer relClient.Close()

	// Get network stats via gRPC.
	stats, err := relClient.GetNetworkStats(ctx, cfg.TenantID)
	if err != nil {
		return fmt.Errorf("getting network stats: %w", err)
	}

	// Display network statistics.
	fmt.Println("Relationship Network Graph")
	fmt.Println("=" + strings.Repeat("=", 40))
	fmt.Println()
	fmt.Printf("  \033[1mNodes (Entities):\033[0m    %d\n", stats.TotalNodes)
	fmt.Printf("  \033[1mEdges (Relations):\033[0m   %d\n", stats.TotalEdges)
	fmt.Printf("  \033[1mDensity:\033[0m             %.3f\n", stats.Density)
	fmt.Printf("  \033[1mAvg. Connections:\033[0m    %.1f\n", stats.AvgConnections)
	fmt.Printf("  \033[1mClusters:\033[0m            %d\n", stats.ClusterCount)
	fmt.Println()

	if len(stats.EntityTypeCounts) > 0 {
		total := float64(stats.TotalNodes)
		fmt.Println("Entity Types:")
		for entType, count := range stats.EntityTypeCounts {
			pct := 0.0
			if total > 0 {
				pct = float64(count) / total * 100
			}
			fmt.Printf("  - %s: %d (%.1f%%)\n", entType, count, pct)
		}
		fmt.Println()
	}

	if len(stats.RelationshipTypeCounts) > 0 {
		fmt.Println("Relationship Types:")
		for relType, count := range stats.RelationshipTypeCounts {
			fmt.Printf("  - %s: %d\n", relType, count)
		}
	}

	return nil
}

// runNetworkCentral executes the network central command.
func runNetworkCentral(ctx context.Context, deps *RelationshipCommandDeps, insecureFlag bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override insecure if flag is set.
	if insecureFlag {
		cfg.Insecure = true
	}

	// Override tenant if specified.
	if relationshipTenant != "" {
		cfg.TenantID = relationshipTenant
	}

	// Initialize relationship client.
	relClient, err := deps.InitRelClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing relationship client: %w", err)
	}
	defer relClient.Close()

	// Get central entities via gRPC.
	ents, err := relClient.GetCentralEntities(ctx, cfg.TenantID, int32(relationshipLimit))
	if err != nil {
		return fmt.Errorf("getting central entities: %w", err)
	}

	// Convert to local types for output.
	entities := make([]Entity, len(ents))
	for i, e := range ents {
		entities[i] = clientEntityToLocal(e)
	}

	format := cfg.OutputFormat
	if relationshipOutput != "" {
		format = config.OutputFormat(relationshipOutput)
	}

	return outputCentralEntities(format, entities)
}

// runNetworkClusters executes the network clusters command.
func runNetworkClusters(ctx context.Context, deps *RelationshipCommandDeps, insecureFlag bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override insecure if flag is set.
	if insecureFlag {
		cfg.Insecure = true
	}

	// Override tenant if specified.
	if relationshipTenant != "" {
		cfg.TenantID = relationshipTenant
	}

	// Initialize relationship client.
	relClient, err := deps.InitRelClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing relationship client: %w", err)
	}
	defer relClient.Close()

	// Get clusters via gRPC.
	clusterList, err := relClient.GetClusters(ctx, cfg.TenantID)
	if err != nil {
		return fmt.Errorf("getting clusters: %w", err)
	}

	// Convert to local types for output.
	clusters := make([]NetworkCluster, len(clusterList))
	for i, c := range clusterList {
		clusters[i] = clientClusterToLocal(c)
	}

	format := cfg.OutputFormat
	if relationshipOutput != "" {
		format = config.OutputFormat(relationshipOutput)
	}

	return outputClusters(format, clusters)
}

// runConflictList executes the conflict list command.
func runConflictList(ctx context.Context, deps *RelationshipCommandDeps, insecureFlag bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override insecure if flag is set.
	if insecureFlag {
		cfg.Insecure = true
	}

	// Override tenant if specified.
	if relationshipTenant != "" {
		cfg.TenantID = relationshipTenant
	}

	// Initialize relationship client.
	relClient, err := deps.InitRelClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing relationship client: %w", err)
	}
	defer relClient.Close()

	// Get conflicts via gRPC.
	req := &client.ListConflictsRequest{
		TenantID: cfg.TenantID,
		Limit:    int32(relationshipLimit),
	}

	conflictList, _, err := relClient.ListConflicts(ctx, req)
	if err != nil {
		return fmt.Errorf("listing conflicts: %w", err)
	}

	// Convert to local types for output.
	conflicts := make([]RelationshipConflict, len(conflictList))
	for i, c := range conflictList {
		conflicts[i] = clientConflictToLocal(c)
	}

	format := cfg.OutputFormat
	if relationshipOutput != "" {
		format = config.OutputFormat(relationshipOutput)
	}

	return outputConflicts(format, conflicts)
}

// runConflictShow executes the conflict show command.
func runConflictShow(ctx context.Context, deps *RelationshipCommandDeps, conflictID string, insecureFlag bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override insecure if flag is set.
	if insecureFlag {
		cfg.Insecure = true
	}

	// Override tenant if specified.
	if relationshipTenant != "" {
		cfg.TenantID = relationshipTenant
	}

	// Initialize relationship client.
	relClient, err := deps.InitRelClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing relationship client: %w", err)
	}
	defer relClient.Close()

	// Get conflict details via gRPC.
	c, err := relClient.GetConflict(ctx, cfg.TenantID, conflictID)
	if err != nil {
		return fmt.Errorf("getting conflict: %w", err)
	}

	conflict := clientConflictToLocal(c)

	format := cfg.OutputFormat
	if relationshipOutput != "" {
		format = config.OutputFormat(relationshipOutput)
	}

	return outputConflictDetail(format, conflict)
}

// runConflictResolve executes the conflict resolve command.
func runConflictResolve(ctx context.Context, deps *RelationshipCommandDeps, conflictID string, strategy ConflictResolutionStrategy, insecureFlag bool) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	deps.Config = cfg

	// Override insecure if flag is set.
	if insecureFlag {
		cfg.Insecure = true
	}

	// Override tenant if specified.
	if relationshipTenant != "" {
		cfg.TenantID = relationshipTenant
	}

	// Validate strategy.
	switch strategy {
	case ConflictStrategyKeepLatest, ConflictStrategyKeepFirst, ConflictStrategyMerge, ConflictStrategyManual:
		// Valid strategies.
	default:
		return fmt.Errorf("invalid resolution strategy: %s (must be keep_latest, keep_first, merge, or manual)", strategy)
	}

	// Initialize relationship client.
	relClient, err := deps.InitRelClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing relationship client: %w", err)
	}
	defer relClient.Close()

	fmt.Printf("Resolving conflict %s with strategy '%s'...\n", conflictID, strategy)

	// Resolve conflict via gRPC.
	req := &client.ResolveConflictRequest{
		TenantID:   cfg.TenantID,
		ConflictID: conflictID,
		Strategy:   stringToConflictStrategy(string(strategy)),
	}

	_, updated, err := relClient.ResolveConflict(ctx, req)
	if err != nil {
		return fmt.Errorf("resolving conflict: %w", err)
	}

	fmt.Printf("\n\033[32mSuccess!\033[0m Conflict resolved.\n")
	fmt.Printf("  Strategy used: %s\n", strategy)
	fmt.Printf("  Relationships updated: %d\n", updated)

	return nil
}

// Type conversion helpers

// stringToEntityType converts a string to a proto EntityType.
func stringToEntityType(s string) relationshipv1.EntityType {
	switch strings.ToLower(s) {
	case "person":
		return relationshipv1.EntityType_ENTITY_TYPE_PERSON
	case "organization":
		return relationshipv1.EntityType_ENTITY_TYPE_ORGANIZATION
	case "topic":
		return relationshipv1.EntityType_ENTITY_TYPE_TOPIC
	case "project":
		return relationshipv1.EntityType_ENTITY_TYPE_PROJECT
	case "location":
		return relationshipv1.EntityType_ENTITY_TYPE_LOCATION
	default:
		return relationshipv1.EntityType_ENTITY_TYPE_UNSPECIFIED
	}
}

// stringToRelType converts a string to a proto RelationshipType.
func stringToRelType(s string) relationshipv1.RelationshipType {
	switch strings.ToLower(s) {
	case "colleague":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_COLLABORATES_WITH
	case "reports_to":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_REPORTS_TO
	case "member_of":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MEMBER_OF
	case "works_on", "works_at":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_WORKS_AT
	case "discusses":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_DISCUSSED
	case "mentions", "mentioned_with":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MENTIONED_WITH
	case "located_at":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_LOCATED_AT
	case "related_to":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_RELATED_TO
	case "knows":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_KNOWS
	case "manages":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_MANAGES
	case "collaborates_with":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_COLLABORATES_WITH
	case "attended":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_ATTENDED
	case "owns":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_OWNS
	case "created":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_CREATED
	case "part_of":
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_PART_OF
	default:
		return relationshipv1.RelationshipType_RELATIONSHIP_TYPE_UNSPECIFIED
	}
}

// Output functions.

// outputRelationships outputs relationships in the specified format.
func outputRelationships(format config.OutputFormat, relationships []Relationship) error {
	switch format {
	case config.OutputFormatJSON:
		return outputRelJSON(relationships)
	case config.OutputFormatYAML:
		return outputRelYAML(relationships)
	default:
		return outputRelationshipsText(relationships)
	}
}

// outputRelationshipsText outputs relationships in human-readable format.
func outputRelationshipsText(relationships []Relationship) error {
	if len(relationships) == 0 {
		fmt.Println("No relationships found.")
		return nil
	}

	fmt.Printf("Relationships (%d):\n\n", len(relationships))
	fmt.Println("  ID           SOURCE               TYPE           TARGET               CONFIDENCE")
	fmt.Println("  --           ------               ----           ------               ----------")

	for _, r := range relationships {
		confidenceColor := getConfidenceColor(r.Confidence)
		fmt.Printf("  %-12s %-20s %-14s %-20s %s%.2f\033[0m\n",
			truncateString(r.ID, 12),
			truncateString(r.SourceName, 20),
			r.Type,
			truncateString(r.TargetName, 20),
			confidenceColor,
			r.Confidence)
	}

	fmt.Println()
	return nil
}

// outputRelationshipDetail outputs a single relationship in detail.
func outputRelationshipDetail(format config.OutputFormat, r Relationship) error {
	switch format {
	case config.OutputFormatJSON:
		return outputRelJSON(r)
	case config.OutputFormatYAML:
		return outputRelYAML(r)
	default:
		return outputRelationshipDetailText(r)
	}
}

// outputRelationshipDetailText outputs relationship detail in human-readable format.
func outputRelationshipDetailText(r Relationship) error {
	fmt.Println("Relationship Details:")
	fmt.Println()
	fmt.Printf("  \033[1mID:\033[0m          %s\n", r.ID)
	fmt.Printf("  \033[1mType:\033[0m        %s\n", r.Type)
	fmt.Println()
	fmt.Printf("  \033[1mSource:\033[0m      %s (%s)\n", r.SourceName, r.SourceID)
	fmt.Printf("  \033[1mTarget:\033[0m      %s (%s)\n", r.TargetName, r.TargetID)
	fmt.Println()
	fmt.Printf("  \033[1mConfidence:\033[0m  %s%.2f\033[0m\n", getConfidenceColor(r.Confidence), r.Confidence)
	fmt.Printf("  \033[1mWeight:\033[0m      %.2f\n", r.Weight)
	fmt.Printf("  \033[1mSources:\033[0m     %d\n", r.SourceCount)
	fmt.Println()
	fmt.Printf("  \033[1mFirst Seen:\033[0m  %s\n", r.FirstSeen.Format(time.RFC3339))
	fmt.Printf("  \033[1mLast Seen:\033[0m   %s\n", r.LastSeen.Format(time.RFC3339))
	fmt.Println()
	if len(r.Evidence) > 0 {
		fmt.Println("  \033[1mEvidence:\033[0m")
		for _, e := range r.Evidence {
			fmt.Printf("    - %s\n", e)
		}
	}

	return nil
}

// outputEntities outputs entities in the specified format.
func outputEntities(format config.OutputFormat, entities []Entity) error {
	switch format {
	case config.OutputFormatJSON:
		return outputRelJSON(entities)
	case config.OutputFormatYAML:
		return outputRelYAML(entities)
	default:
		return outputEntitiesText(entities)
	}
}

// outputEntitiesText outputs entities in human-readable format.
func outputEntitiesText(entities []Entity) error {
	if len(entities) == 0 {
		fmt.Println("No entities found.")
		return nil
	}

	fmt.Printf("Entities (%d):\n\n", len(entities))
	fmt.Println("  ID               NAME                 TYPE           RELATIONS  CONFIDENCE")
	fmt.Println("  --               ----                 ----           ---------  ----------")

	for _, e := range entities {
		confidenceColor := getConfidenceColor(e.Confidence)
		typeColor := getEntityTypeColor(e.Type)
		fmt.Printf("  %-16s %-20s %s%-14s\033[0m %4d       %s%.2f\033[0m\n",
			truncateString(e.ID, 16),
			truncateString(e.Name, 20),
			typeColor,
			e.Type,
			e.RelationCount,
			confidenceColor,
			e.Confidence)
	}

	fmt.Println()
	return nil
}

// outputEntityDetail outputs a single entity in detail.
func outputEntityDetail(format config.OutputFormat, e Entity) error {
	switch format {
	case config.OutputFormatJSON:
		return outputRelJSON(e)
	case config.OutputFormatYAML:
		return outputRelYAML(e)
	default:
		return outputEntityDetailText(e)
	}
}

// outputEntityDetailText outputs entity detail in human-readable format.
func outputEntityDetailText(e Entity) error {
	typeColor := getEntityTypeColor(e.Type)

	fmt.Println("Entity Details:")
	fmt.Println()
	fmt.Printf("  \033[1mID:\033[0m           %s\n", e.ID)
	fmt.Printf("  \033[1mName:\033[0m         %s\n", e.Name)
	fmt.Printf("  \033[1mType:\033[0m         %s%s\033[0m\n", typeColor, e.Type)
	fmt.Println()
	if len(e.Aliases) > 0 {
		fmt.Printf("  \033[1mAliases:\033[0m      %s\n", strings.Join(e.Aliases, ", "))
	}
	fmt.Printf("  \033[1mConfidence:\033[0m   %s%.2f\033[0m\n", getConfidenceColor(e.Confidence), e.Confidence)
	fmt.Printf("  \033[1mSources:\033[0m      %d\n", e.SourceCount)
	fmt.Printf("  \033[1mRelations:\033[0m    %d\n", e.RelationCount)
	fmt.Println()
	fmt.Printf("  \033[1mFirst Seen:\033[0m   %s\n", e.FirstSeen.Format(time.RFC3339))
	fmt.Printf("  \033[1mLast Seen:\033[0m    %s\n", e.LastSeen.Format(time.RFC3339))
	fmt.Println()
	if len(e.Metadata) > 0 {
		fmt.Println("  \033[1mMetadata:\033[0m")
		for k, v := range e.Metadata {
			fmt.Printf("    %s: %s\n", k, v)
		}
	}

	return nil
}

// outputCentralEntities outputs central entities.
func outputCentralEntities(format config.OutputFormat, entities []Entity) error {
	switch format {
	case config.OutputFormatJSON:
		return outputRelJSON(entities)
	case config.OutputFormatYAML:
		return outputRelYAML(entities)
	default:
		return outputCentralEntitiesText(entities)
	}
}

// outputCentralEntitiesText outputs central entities in human-readable format.
func outputCentralEntitiesText(entities []Entity) error {
	if len(entities) == 0 {
		fmt.Println("No entities found.")
		return nil
	}

	fmt.Println("Most Connected Entities:")
	fmt.Println()
	fmt.Println("  RANK  NAME                 TYPE           CONNECTIONS  CONFIDENCE")
	fmt.Println("  ----  ----                 ----           -----------  ----------")

	for i, e := range entities {
		confidenceColor := getConfidenceColor(e.Confidence)
		typeColor := getEntityTypeColor(e.Type)
		fmt.Printf("  %-4d  %-20s %s%-14s\033[0m %4d         %s%.2f\033[0m\n",
			i+1,
			truncateString(e.Name, 20),
			typeColor,
			e.Type,
			e.RelationCount,
			confidenceColor,
			e.Confidence)
	}

	fmt.Println()
	return nil
}

// outputClusters outputs network clusters.
func outputClusters(format config.OutputFormat, clusters []NetworkCluster) error {
	switch format {
	case config.OutputFormatJSON:
		return outputRelJSON(clusters)
	case config.OutputFormatYAML:
		return outputRelYAML(clusters)
	default:
		return outputClustersText(clusters)
	}
}

// outputClustersText outputs clusters in human-readable format.
func outputClustersText(clusters []NetworkCluster) error {
	if len(clusters) == 0 {
		fmt.Println("No clusters found.")
		return nil
	}

	fmt.Println("Network Clusters:")
	fmt.Println()

	for _, c := range clusters {
		fmt.Printf("  \033[1m%s\033[0m (%s)\n", c.Name, c.ID)
		fmt.Printf("    Entities: %d | Density: %.2f\n", c.EntityCount, c.Density)
		if len(c.TopEntities) > 0 {
			fmt.Printf("    Top members: ")
			names := make([]string, 0, len(c.TopEntities))
			for _, e := range c.TopEntities {
				names = append(names, e.Name)
			}
			fmt.Printf("%s\n", strings.Join(names, ", "))
		}
		fmt.Println()
	}

	return nil
}

// outputConflicts outputs conflicts in the specified format.
func outputConflicts(format config.OutputFormat, conflicts []RelationshipConflict) error {
	switch format {
	case config.OutputFormatJSON:
		return outputRelJSON(conflicts)
	case config.OutputFormatYAML:
		return outputRelYAML(conflicts)
	default:
		return outputConflictsText(conflicts)
	}
}

// outputConflictsText outputs conflicts in human-readable format.
func outputConflictsText(conflicts []RelationshipConflict) error {
	if len(conflicts) == 0 {
		fmt.Println("\033[32mNo conflicts detected.\033[0m")
		return nil
	}

	fmt.Printf("Relationship Conflicts (%d):\n\n", len(conflicts))
	fmt.Println("  ID           TYPE                       STATUS    AGE")
	fmt.Println("  --           ----                       ------    ---")

	for _, c := range conflicts {
		statusColor := "\033[33m" // Yellow for pending.
		if c.Status == "resolved" {
			statusColor = "\033[32m" // Green for resolved.
		}
		age := formatRelativeTime(c.CreatedAt)
		fmt.Printf("  %-12s %-26s %s%-8s\033[0m  %s\n",
			truncateString(c.ID, 12),
			truncateString(c.Type, 26),
			statusColor,
			c.Status,
			age)
	}

	fmt.Println()
	fmt.Println("Use 'penf relationship conflict show <id>' for details.")
	return nil
}

// outputConflictDetail outputs a single conflict in detail.
func outputConflictDetail(format config.OutputFormat, c RelationshipConflict) error {
	switch format {
	case config.OutputFormatJSON:
		return outputRelJSON(c)
	case config.OutputFormatYAML:
		return outputRelYAML(c)
	default:
		return outputConflictDetailText(c)
	}
}

// outputConflictDetailText outputs conflict detail in human-readable format.
func outputConflictDetailText(c RelationshipConflict) error {
	statusColor := "\033[33m"
	if c.Status == "resolved" {
		statusColor = "\033[32m"
	}

	fmt.Println("Conflict Details:")
	fmt.Println()
	fmt.Printf("  \033[1mID:\033[0m          %s\n", c.ID)
	fmt.Printf("  \033[1mType:\033[0m        %s\n", c.Type)
	fmt.Printf("  \033[1mStatus:\033[0m      %s%s\033[0m\n", statusColor, c.Status)
	fmt.Printf("  \033[1mCreated:\033[0m     %s (%s)\n", c.CreatedAt.Format(time.RFC3339), formatRelativeTime(c.CreatedAt))
	fmt.Println()
	fmt.Println("  \033[1mDescription:\033[0m")
	fmt.Printf("    %s\n", c.Description)
	fmt.Println()
	fmt.Println("  \033[1mSuggested Action:\033[0m")
	fmt.Printf("    %s\n", c.SuggestedAction)
	fmt.Println()

	if len(c.Relationships) > 0 {
		fmt.Println("  \033[1mRelated Relationships:\033[0m")
		for _, r := range c.Relationships {
			fmt.Printf("    - %s: %s -> %s (%s)\n", r.ID, r.SourceName, r.TargetName, r.Type)
		}
		fmt.Println()
	}

	fmt.Printf("Resolve with: penf relationship conflict resolve %s --strategy <strategy>\n", c.ID)

	return nil
}

// Helper functions.

// getConfidenceColor returns ANSI color code based on confidence score.
func getConfidenceColor(confidence float64) string {
	if confidence >= 0.8 {
		return "\033[32m" // Green for high confidence.
	} else if confidence >= 0.5 {
		return "\033[33m" // Yellow for medium confidence.
	}
	return "\033[31m" // Red for low confidence.
}

// getEntityTypeColor returns ANSI color code for entity types.
func getEntityTypeColor(t EntityType) string {
	switch t {
	case EntityTypePerson:
		return "\033[36m" // Cyan.
	case EntityTypeOrganization:
		return "\033[35m" // Magenta.
	case EntityTypeTopic:
		return "\033[34m" // Blue.
	case EntityTypeProject:
		return "\033[33m" // Yellow.
	case EntityTypeLocation:
		return "\033[32m" // Green.
	default:
		return ""
	}
}

// outputRelJSON outputs data as JSON.
func outputRelJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// outputRelYAML outputs data as YAML.
func outputRelYAML(v interface{}) error {
	enc := yaml.NewEncoder(os.Stdout)
	return enc.Encode(v)
}

// Client type conversion helpers

// clientEntityToLocal converts a client RelEntity to a local Entity.
func clientEntityToLocal(e *client.RelEntity) Entity {
	if e == nil {
		return Entity{}
	}
	return Entity{
		ID:            e.ID,
		Name:          e.Name,
		Type:          EntityType(e.Type),
		Aliases:       e.Aliases,
		Confidence:    float64(e.Confidence),
		SourceCount:   int(e.SourceCount),
		FirstSeen:     e.FirstSeen,
		LastSeen:      e.LastSeen,
		Metadata:      e.Metadata,
		RelationCount: int(e.RelationCount),
	}
}

// clientClusterToLocal converts a client NetworkCluster to a local NetworkCluster.
func clientClusterToLocal(c *client.NetworkCluster) NetworkCluster {
	if c == nil {
		return NetworkCluster{}
	}
	topEntities := make([]Entity, len(c.TopEntities))
	for i, e := range c.TopEntities {
		topEntities[i] = clientEntityToLocal(e)
	}
	return NetworkCluster{
		ID:          c.ID,
		Name:        c.Name,
		EntityCount: int(c.EntityCount),
		TopEntities: topEntities,
		Density:     float64(c.Density),
	}
}

// clientConflictToLocal converts a client RelationshipConflict to a local RelationshipConflict.
func clientConflictToLocal(c *client.RelationshipConflict) RelationshipConflict {
	if c == nil {
		return RelationshipConflict{}
	}
	rels := make([]Relationship, len(c.Relationships))
	for i, r := range c.Relationships {
		rels[i] = clientRelToLocal(r)
	}
	return RelationshipConflict{
		ID:              c.ID,
		Type:            c.Type,
		Description:     c.Description,
		Relationships:   rels,
		SuggestedAction: c.SuggestedAction,
		CreatedAt:       c.CreatedAt,
		Status:          c.Status,
	}
}

// clientRelToLocal converts a client Relationship to a local Relationship.
func clientRelToLocal(r *client.Relationship) Relationship {
	if r == nil {
		return Relationship{}
	}
	var sourceName, sourceID, targetName, targetID string
	if r.SourceEntity != nil {
		sourceName = r.SourceEntity.Name
		sourceID = r.SourceEntity.ID
	}
	if r.TargetEntity != nil {
		targetName = r.TargetEntity.Name
		targetID = r.TargetEntity.ID
	}

	evidence := make([]string, len(r.Evidence))
	for i, e := range r.Evidence {
		evidence[i] = e.Excerpt
	}

	return Relationship{
		ID:          r.ID,
		SourceID:    sourceID,
		SourceName:  sourceName,
		TargetID:    targetID,
		TargetName:  targetName,
		Type:        RelationshipType(r.RelationshipType),
		Confidence:  float64(r.Confidence),
		Weight:      1.0, // Default weight
		Evidence:    evidence,
		FirstSeen:   r.CreatedAt,
		LastSeen:    r.UpdatedAt,
		SourceCount: len(r.Evidence),
	}
}

// stringToConflictStrategy converts a string to a proto ConflictResolutionStrategy.
func stringToConflictStrategy(s string) relationshipv1.ConflictResolutionStrategy {
	switch s {
	case "keep_latest":
		return relationshipv1.ConflictResolutionStrategy_CONFLICT_RESOLUTION_STRATEGY_KEEP_LATEST
	case "keep_first":
		return relationshipv1.ConflictResolutionStrategy_CONFLICT_RESOLUTION_STRATEGY_KEEP_FIRST
	case "merge":
		return relationshipv1.ConflictResolutionStrategy_CONFLICT_RESOLUTION_STRATEGY_MERGE
	case "manual":
		return relationshipv1.ConflictResolutionStrategy_CONFLICT_RESOLUTION_STRATEGY_MANUAL
	default:
		return relationshipv1.ConflictResolutionStrategy_CONFLICT_RESOLUTION_STRATEGY_UNSPECIFIED
	}
}
