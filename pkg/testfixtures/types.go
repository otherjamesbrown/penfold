// Package testfixtures provides data types and loaders for test fixtures.
package testfixtures

// PeopleFile represents the structure of people.yaml.
type PeopleFile struct {
	People []PersonFixture `yaml:"people"`
}

// PersonFixture represents a person in test fixtures.
type PersonFixture struct {
	ID            int64    `yaml:"id"`
	CanonicalName string   `yaml:"canonical_name"`
	Email         string   `yaml:"email"`
	Aliases       []string `yaml:"aliases"`
	Title         string   `yaml:"title"`
	TeamID        int64    `yaml:"team_id"`
	ManagerID     *int64   `yaml:"manager_id"`
}

// TeamsFile represents the structure of teams.yaml.
type TeamsFile struct {
	Teams []TeamFixture `yaml:"teams"`
}

// TeamFixture represents a team in test fixtures.
type TeamFixture struct {
	ID          int64  `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Slug        string `yaml:"slug"`     // Legacy: not used in current schema
	ParentID    *int64 `yaml:"parent_id"` // Legacy: not used in current schema
	LeadID      *int64 `yaml:"lead_id"`   // Legacy: not used in current schema
}

// ProjectsFile represents the structure of projects.yaml.
type ProjectsFile struct {
	Projects []ProjectFixture `yaml:"projects"`
}

// ProjectFixture represents a project in test fixtures.
type ProjectFixture struct {
	ID          int64  `yaml:"id"`
	Name        string `yaml:"name"`
	Slug        string `yaml:"slug"`
	Description string `yaml:"description"`
	Status      string `yaml:"status"`
	OwnerID     int64  `yaml:"owner_id"`
	TeamID      int64  `yaml:"team_id"`
	StartDate   string `yaml:"start_date"`
	TargetDate  string `yaml:"target_date"`
}

// ProductsFile represents the structure of products.yaml.
type ProductsFile struct {
	Products []ProductFixture `yaml:"products"`
}

// ProductFixture represents a product in test fixtures.
type ProductFixture struct {
	ID          int64  `yaml:"id"`
	Name        string `yaml:"name"`
	Slug        string `yaml:"slug"`
	Description string `yaml:"description"`
	TeamID      *int64 `yaml:"team_id"`
}

// GlossaryFile represents the structure of glossary.yaml.
type GlossaryFile struct {
	Terms []GlossaryTermFixture `yaml:"terms"`
}

// GlossaryTermFixture represents a glossary term in test fixtures.
type GlossaryTermFixture struct {
	Term             string   `yaml:"term"`
	Expansion        *string  `yaml:"expansion"`
	Definition       *string  `yaml:"definition"`
	Context          *string  `yaml:"context"`
	Aliases          []string `yaml:"aliases"`
	LinkedEntityType *string  `yaml:"linked_entity_type"`
	LinkedEntityID   *int64   `yaml:"linked_entity_id"`
}
