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
		_, err := l.db.Exec(ctx, `
			INSERT INTO teams (id, tenant_id, name, description, slug, parent_id)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description,
				slug = EXCLUDED.slug,
				parent_id = EXCLUDED.parent_id
		`, team.ID, l.tenantID, team.Name, team.Description, team.Slug, team.ParentID)
		if err != nil {
			return fmt.Errorf("insert team %s: %w", team.Name, err)
		}
	}

	return nil
}

// UpdateTeamLeads updates teams with their lead_id after people are loaded.
func (l *Loader) UpdateTeamLeads(ctx context.Context) error {
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
		if team.LeadID != nil {
			_, err := l.db.Exec(ctx, `
				UPDATE teams SET lead_id = $2 WHERE id = $1
			`, team.ID, team.LeadID)
			if err != nil {
				return fmt.Errorf("update team %s lead: %w", team.Name, err)
			}
		}
	}

	return nil
}

// LoadPeople loads people from people.yaml.
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
		_, err := l.db.Exec(ctx, `
			INSERT INTO people (id, tenant_id, canonical_name, primary_email, title)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE SET
				canonical_name = EXCLUDED.canonical_name,
				primary_email = EXCLUDED.primary_email,
				title = EXCLUDED.title
		`, person.ID, l.tenantID, person.CanonicalName, person.Email, person.Title)
		if err != nil {
			return fmt.Errorf("insert person %s: %w", person.CanonicalName, err)
		}
	}

	return nil
}

// LoadProjects loads projects from projects.yaml.
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
		// Use minimal columns that are guaranteed to exist
		_, err := l.db.Exec(ctx, `
			INSERT INTO projects (id, tenant_id, name, description)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description
		`, project.ID, l.tenantID, project.Name, project.Description)
		if err != nil {
			return fmt.Errorf("insert project %s: %w", project.Name, err)
		}
	}

	return nil
}

// LoadProducts loads products from products.yaml.
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
		// Use minimal columns that are guaranteed to exist
		_, err := l.db.Exec(ctx, `
			INSERT INTO products (id, tenant_id, name, description)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description
		`, product.ID, l.tenantID, product.Name, product.Description)
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

		// Use minimal columns that are guaranteed to exist
		// Note: tenant_id uses default UUID from column definition
		_, err := l.db.Exec(ctx, `
			INSERT INTO glossary (term, expansion, definition)
			VALUES ($1, $2, $3)
			ON CONFLICT (tenant_id, term) DO UPDATE SET
				expansion = EXCLUDED.expansion,
				definition = EXCLUDED.definition
		`, term.Term, expansion, definition)
		if err != nil {
			return fmt.Errorf("insert glossary term %s: %w", term.Term, err)
		}
	}

	return nil
}

// TruncateAll truncates all fixture-related tables.
func (l *Loader) TruncateAll(ctx context.Context) error {
	tables := []string{"glossary", "products", "projects", "people", "teams"}
	for _, table := range tables {
		_, err := l.db.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			// Table might not exist, continue
			continue
		}
	}
	return nil
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
