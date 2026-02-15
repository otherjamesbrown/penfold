// Package testfixtures provides data types and loaders for test fixtures.
package testfixtures

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

// DefaultTestTenantID is used for test fixtures when no tenant is specified.
// Must be a valid UUID since the database column is UUID type.
const DefaultTestTenantID = "00000000-0000-0000-0000-000000000001"

// Well-known test tenant UUIDs.
const (
	E2ETestTenantID         = "00000000-0000-0000-0000-000000000001"
	IntegrationTestTenantID = "00000000-0000-0000-0000-000000000002"
	QualityTestTenantID     = "00000000-0000-0000-0000-000000000003"
)

// IsTestTenantID returns true if the given UUID is a well-known test tenant ID.
// Only these three tenant IDs are allowed for fixture loading.
func IsTestTenantID(id string) bool {
	return id == E2ETestTenantID ||
		id == IntegrationTestTenantID ||
		id == QualityTestTenantID
}

// Loader handles loading YAML fixtures into a test database.
type Loader struct {
	db         *pgxpool.Pool
	fixtureDir string
	tenantID   string
}

// NewLoader creates a new fixture loader with the default test tenant.
func NewLoader(db *pgxpool.Pool, fixtureDir string) *Loader {
	return &Loader{
		db:         db,
		fixtureDir: fixtureDir,
		tenantID:   DefaultTestTenantID,
	}
}

// NewLoaderWithTenant creates a new fixture loader with a specific tenant.
func NewLoaderWithTenant(db *pgxpool.Pool, fixtureDir string, tenantID string) *Loader {
	return &Loader{
		db:         db,
		fixtureDir: fixtureDir,
		tenantID:   tenantID,
	}
}

// LoadAcmeCorp loads all Acme Corp fixtures into the database.
func (l *Loader) LoadAcmeCorp(ctx context.Context) error {
	// Guard against loading fixtures into non-test tenants.
	// This prevents accidental fixture loading into production databases.
	if !IsTestTenantID(l.tenantID) {
		return fmt.Errorf("refusing to load fixtures into non-test tenant: %s", l.tenantID)
	}

	// Load in dependency order, handling circular dependencies:
	// 1. Teams without lead_id (people depend on teams)
	// 2. People (depends on teams)
	// 3. Update teams with lead_id (depends on people)
	// 4. Projects (depends on teams, people)
	// 5. Products (depends on teams)
	// 6. Glossary (no dependencies)
	if err := l.LoadTeamsWithoutLeads(ctx); err != nil {
		return fmt.Errorf("load teams: %w", err)
	}
	if err := l.LoadPeople(ctx); err != nil {
		return fmt.Errorf("load people: %w", err)
	}
	if err := l.UpdateTeamLeads(ctx); err != nil {
		return fmt.Errorf("update team leads: %w", err)
	}
	if err := l.LoadProjects(ctx); err != nil {
		return fmt.Errorf("load projects: %w", err)
	}
	if err := l.LoadProducts(ctx); err != nil {
		return fmt.Errorf("load products: %w", err)
	}
	if err := l.LoadGlossary(ctx); err != nil {
		return fmt.Errorf("load glossary: %w", err)
	}
	return nil
}

// LoadTeams loads teams from teams.yaml (requires people to exist first for lead_id FK).
func (l *Loader) LoadTeams(ctx context.Context) error {
	if err := l.LoadTeamsWithoutLeads(ctx); err != nil {
		return err
	}
	return l.UpdateTeamLeads(ctx)
}

// LoadTeamsWithoutLeads loads teams without setting lead_id (to avoid circular FK dependency).
// Uses auto-generated IDs to avoid conflicts with production data.
// Always call CleanupAll() before loading to ensure idempotency.
func (l *Loader) LoadTeamsWithoutLeads(ctx context.Context) error {
	path := filepath.Join(l.fixtureDir, "teams.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read teams.yaml: %w", err)
	}

	var file TeamsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("unmarshal teams.yaml: %w", err)
	}

	for _, team := range file.Teams {
		// Don't specify ID - let the database auto-generate it to avoid
		// conflicts with production data (tables use global IDs, not tenant-scoped).
		_, err := l.db.Exec(ctx, `
			INSERT INTO teams (tenant_id, name, description)
			VALUES ($1, $2, $3)
		`, l.tenantID, team.Name, team.Description)
		if err != nil {
			return fmt.Errorf("insert team %s: %w", team.Name, err)
		}
	}

	return nil
}

// UpdateTeamLeads updates teams with their lead_id after people are loaded.
// Note: Currently a no-op since the teams table doesn't have a lead_id column.
// The fixture types retain lead_id for future schema compatibility.
func (l *Loader) UpdateTeamLeads(ctx context.Context) error {
	// The teams table schema only has: id, tenant_id, name, description, created_at, updated_at
	// lead_id is not a column in the current schema, so this is a no-op.
	// Team leadership could be tracked via team_members with role='lead' if needed.
	return nil
}

// LoadPeople loads people from people.yaml.
// Uses auto-generated IDs to avoid conflicts with production data.
// Always call CleanupAll() before loading to ensure idempotency.
func (l *Loader) LoadPeople(ctx context.Context) error {
	path := filepath.Join(l.fixtureDir, "people.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read people.yaml: %w", err)
	}

	var file PeopleFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("unmarshal people.yaml: %w", err)
	}

	for _, person := range file.People {
		// Note: primary_email is a generated column (email_addresses[1]), so we
		// insert into email_addresses instead. The email from fixture becomes the
		// first element of the array.
		// We don't specify ID - let the database auto-generate it to avoid
		// conflicts with production data (tables use global IDs, not tenant-scoped).
		// confidence_score defaults to 0.85 for test fixtures.
		_, err := l.db.Exec(ctx, `
			INSERT INTO people (tenant_id, canonical_name, email_addresses, confidence_score)
			VALUES ($1, $2, ARRAY[$3], 0.85)
		`, l.tenantID, person.CanonicalName, person.Email)
		if err != nil {
			return fmt.Errorf("insert person %s: %w", person.CanonicalName, err)
		}
	}

	return nil
}

// LoadProjects loads projects from projects.yaml.
// Uses auto-generated IDs to avoid conflicts with production data.
// Always call CleanupAll() before loading to ensure idempotency.
func (l *Loader) LoadProjects(ctx context.Context) error {
	path := filepath.Join(l.fixtureDir, "projects.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read projects.yaml: %w", err)
	}

	var file ProjectsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("unmarshal projects.yaml: %w", err)
	}

	for _, project := range file.Projects {
		// Don't specify ID - let the database auto-generate it to avoid
		// conflicts with production data (tables use global IDs, not tenant-scoped).
		_, err := l.db.Exec(ctx, `
			INSERT INTO projects (tenant_id, name, description)
			VALUES ($1, $2, $3)
		`, l.tenantID, project.Name, project.Description)
		if err != nil {
			return fmt.Errorf("insert project %s: %w", project.Name, err)
		}
	}

	return nil
}

// LoadProducts loads products from products.yaml.
// Uses auto-generated IDs and ON CONFLICT for idempotency.
func (l *Loader) LoadProducts(ctx context.Context) error {
	path := filepath.Join(l.fixtureDir, "products.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read products.yaml: %w", err)
	}

	var file ProductsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("unmarshal products.yaml: %w", err)
	}

	for _, product := range file.Products {
		// Use ON CONFLICT on (tenant_id, name) for idempotency.
		// ID is auto-generated to avoid conflicts with production data.
		_, err := l.db.Exec(ctx, `
			INSERT INTO products (tenant_id, name, description)
			VALUES ($1, $2, $3)
			ON CONFLICT (tenant_id, name) DO UPDATE SET
				description = EXCLUDED.description
		`, l.tenantID, product.Name, product.Description)
		if err != nil {
			return fmt.Errorf("insert product %s: %w", product.Name, err)
		}
	}

	return nil
}

// LoadGlossary loads glossary terms from glossary.yaml.
func (l *Loader) LoadGlossary(ctx context.Context) error {
	path := filepath.Join(l.fixtureDir, "glossary.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read glossary.yaml: %w", err)
	}

	var file GlossaryFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("unmarshal glossary.yaml: %w", err)
	}

	for _, term := range file.Terms {
		// Handle null expansion - use empty string as default (NOT NULL constraint)
		expansion := ""
		if term.Expansion != nil {
			expansion = *term.Expansion
		}

		// Handle null definition
		var definition *string
		if term.Definition != nil {
			definition = term.Definition
		}

		// Handle context - JSONB column, wrap string in quotes for valid JSON
		var contextJSON *string
		if term.Context != nil {
			quoted := fmt.Sprintf("%q", *term.Context)
			contextJSON = &quoted
		}

		// Handle aliases - JSONB array, default to empty array
		aliasesJSON := "[]"
		if len(term.Aliases) > 0 {
			// Build JSON array manually to avoid import
			aliasesJSON = "["
			for i, alias := range term.Aliases {
				if i > 0 {
					aliasesJSON += ","
				}
				aliasesJSON += fmt.Sprintf("%q", alias)
			}
			aliasesJSON += "]"
		}

		_, err := l.db.Exec(ctx, `
			INSERT INTO glossary (tenant_id, term, expansion, definition, context, aliases, linked_entity_type, linked_entity_id)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8)
			ON CONFLICT (tenant_id, term) DO UPDATE SET
				expansion = EXCLUDED.expansion,
				definition = EXCLUDED.definition,
				context = EXCLUDED.context,
				aliases = EXCLUDED.aliases,
				linked_entity_type = EXCLUDED.linked_entity_type,
				linked_entity_id = EXCLUDED.linked_entity_id
		`, l.tenantID, term.Term, expansion, definition, contextJSON, aliasesJSON, term.LinkedEntityType, term.LinkedEntityID)
		if err != nil {
			return fmt.Errorf("insert glossary term %s: %w", term.Term, err)
		}
	}

	return nil
}

// CleanupAll deletes all fixture-related data for the loader's tenant.
// This preserves other tenants' data in the shared database.
func (l *Loader) CleanupAll(ctx context.Context) error {
	tables := []string{"glossary", "products", "projects", "people", "teams"}
	for _, table := range tables {
		_, err := l.db.Exec(ctx, fmt.Sprintf(
			"DELETE FROM %s WHERE tenant_id = $1", table), l.tenantID)
		if err != nil {
			// Table might not exist or no tenant_id column, continue
			continue
		}
	}
	return nil
}

// TruncateAll is deprecated - use CleanupAll instead.
func (l *Loader) TruncateAll(ctx context.Context) error {
	return l.CleanupAll(ctx)
}

// LoadYAMLFile is a generic helper to load any YAML file.
func LoadYAMLFile[T any](path string) (T, error) {
	var result T
	data, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("read file: %w", err)
	}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("unmarshal YAML: %w", err)
	}
	return result, nil
}
