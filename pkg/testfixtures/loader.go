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

// Loader handles loading YAML fixtures into a test database.
type Loader struct {
	db         *pgxpool.Pool
	fixtureDir string
}

// NewLoader creates a new fixture loader.
func NewLoader(db *pgxpool.Pool, fixtureDir string) *Loader {
	return &Loader{
		db:         db,
		fixtureDir: fixtureDir,
	}
}

// LoadAcmeCorp loads all Acme Corp fixtures into the database.
func (l *Loader) LoadAcmeCorp(ctx context.Context) error {
	// Load in dependency order
	if err := l.LoadTeams(ctx); err != nil {
		return fmt.Errorf("load teams: %w", err)
	}
	if err := l.LoadPeople(ctx); err != nil {
		return fmt.Errorf("load people: %w", err)
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

// LoadTeams loads teams from teams.yaml.
func (l *Loader) LoadTeams(ctx context.Context) error {
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
			INSERT INTO teams (id, name, slug, parent_id, lead_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				slug = EXCLUDED.slug,
				parent_id = EXCLUDED.parent_id,
				lead_id = EXCLUDED.lead_id
		`, team.ID, team.Name, team.Slug, team.ParentID, team.LeadID)
		if err != nil {
			return fmt.Errorf("insert team %s: %w", team.Name, err)
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
			INSERT INTO people (id, canonical_name, email, aliases, title, team_id, manager_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (id) DO UPDATE SET
				canonical_name = EXCLUDED.canonical_name,
				email = EXCLUDED.email,
				aliases = EXCLUDED.aliases,
				title = EXCLUDED.title,
				team_id = EXCLUDED.team_id,
				manager_id = EXCLUDED.manager_id
		`, person.ID, person.CanonicalName, person.Email, person.Aliases, person.Title, person.TeamID, person.ManagerID)
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
		_, err := l.db.Exec(ctx, `
			INSERT INTO projects (id, name, slug, description, status, owner_id, team_id, start_date, target_date)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				slug = EXCLUDED.slug,
				description = EXCLUDED.description,
				status = EXCLUDED.status,
				owner_id = EXCLUDED.owner_id,
				team_id = EXCLUDED.team_id,
				start_date = EXCLUDED.start_date,
				target_date = EXCLUDED.target_date
		`, project.ID, project.Name, project.Slug, project.Description, project.Status,
			project.OwnerID, project.TeamID, project.StartDate, project.TargetDate)
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
		_, err := l.db.Exec(ctx, `
			INSERT INTO products (id, name, slug, description, team_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				slug = EXCLUDED.slug,
				description = EXCLUDED.description,
				team_id = EXCLUDED.team_id
		`, product.ID, product.Name, product.Slug, product.Description, product.TeamID)
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
		_, err := l.db.Exec(ctx, `
			INSERT INTO glossary_terms (term, expansion, definition, context, aliases, linked_entity_type, linked_entity_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (term) DO UPDATE SET
				expansion = EXCLUDED.expansion,
				definition = EXCLUDED.definition,
				context = EXCLUDED.context,
				aliases = EXCLUDED.aliases,
				linked_entity_type = EXCLUDED.linked_entity_type,
				linked_entity_id = EXCLUDED.linked_entity_id
		`, term.Term, term.Expansion, term.Definition, term.Context, term.Aliases,
			term.LinkedEntityType, term.LinkedEntityID)
		if err != nil {
			return fmt.Errorf("insert glossary term %s: %w", term.Term, err)
		}
	}

	return nil
}

// TruncateAll truncates all fixture-related tables.
func (l *Loader) TruncateAll(ctx context.Context) error {
	tables := []string{"glossary_terms", "products", "projects", "people", "teams"}
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
