// Package contracts defines interfaces for testing infrastructure.
// This file documents the expected interfaces - not meant to be compiled directly.
package contracts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// =============================================================================
// E2E Environment Interface
// =============================================================================

// E2EEnvironment provides the test harness for end-to-end tests.
// It manages database connections, LLM client, and fixture loading.
type E2EEnvironment interface {
	// Setup and teardown
	Setup(t *testing.T) error
	Cleanup()

	// Database access
	DB() *pgxpool.Pool

	// Fixture management
	LoadFixture(name string) error // e.g., "acme-corp"
	TruncateAll() error

	// Pipeline operations
	IngestEmail(ctx context.Context, emlPath string) (*IngestResult, error)
	IngestMeeting(ctx context.Context, transcriptPath string) (*IngestResult, error)

	// Query operations
	Search(ctx context.Context, query string) ([]SearchResult, error)
	GetPerson(ctx context.Context, id int64) (*Person, error)
	GetMentions(ctx context.Context, contentID string) ([]Mention, error)

	// LLM access (for debugging)
	LLMAvailable() bool
	LLMEndpoint() string
}

// IngestResult contains the result of ingesting content.
type IngestResult struct {
	ContentID        string
	ResolvedMentions []ResolvedMention
	GlossaryMatches  []GlossaryMatch
	Embeddings       []float32
	ProcessingTimeMS int64
}

// ResolvedMention represents a mention that was resolved to an entity.
type ResolvedMention struct {
	OriginalText string
	ResolvedTo   *Entity
	Confidence   float64
	MatchType    string // "canonical", "alias", "fuzzy"
}

// Entity represents a resolved entity (person, team, project, etc.)
type Entity struct {
	ID         int64
	EntityType string // "person", "team", "project", "product"
	Name       string
}

// GlossaryMatch represents a glossary term found in content.
type GlossaryMatch struct {
	Term      string
	Expansion string
	Context   string
}

// SearchResult represents a search result.
type SearchResult struct {
	DocumentID  string
	ContentType string
	Score       float64
	Snippet     string
}

// Person represents a person entity.
type Person struct {
	ID            int64
	CanonicalName string
	Email         string
	Aliases       []string
}

// Mention represents a detected mention in content.
type Mention struct {
	Text       string
	PersonID   *int64
	Confidence float64
	Context    string
}

// =============================================================================
// Fixture Loader Interface
// =============================================================================

// FixtureLoader handles loading YAML fixtures into the test database.
type FixtureLoader interface {
	// Load complete fixture set
	LoadAcmeCorp(ctx context.Context) error

	// Load individual fixture types
	LoadPeople(ctx context.Context, path string) error
	LoadTeams(ctx context.Context, path string) error
	LoadProjects(ctx context.Context, path string) error
	LoadProducts(ctx context.Context, path string) error
	LoadGlossary(ctx context.Context, path string) error
	LoadEmails(ctx context.Context, dir string) error

	// Cleanup
	TruncateAll(ctx context.Context) error
}

// =============================================================================
// Assertion Helpers Interface
// =============================================================================

// Assertions provides semantic assertion helpers for E2E tests.
type Assertions interface {
	// Mention assertions
	AssertMentionResolved(t *testing.T, mention ResolvedMention, expectedPersonID int64)
	AssertMentionNotResolved(t *testing.T, mention ResolvedMention)
	AssertMentionConfidence(t *testing.T, mention ResolvedMention, minConfidence float64)

	// Glossary assertions
	AssertGlossaryExpanded(t *testing.T, matches []GlossaryMatch, term, expansion string)
	AssertGlossaryNotExpanded(t *testing.T, matches []GlossaryMatch, term string)

	// Search assertions
	AssertSearchContains(t *testing.T, results []SearchResult, documentID string)
	AssertSearchRankedHigher(t *testing.T, results []SearchResult, higherID, lowerID string)
	AssertSearchScoreAbove(t *testing.T, results []SearchResult, documentID string, minScore float64)

	// Entity assertions
	AssertEntityLinked(t *testing.T, result *IngestResult, entityType string, entityID int64)
}

// =============================================================================
// Integration Test Helpers Interface
// =============================================================================

// IntegrationDB provides database helpers for integration tests.
type IntegrationDB interface {
	// Setup
	SetupTestDB(t *testing.T, dbName string) *pgxpool.Pool

	// Seeding
	SeedPeople(ctx context.Context, db *pgxpool.Pool, people []PersonFixture) error
	SeedGlossary(ctx context.Context, db *pgxpool.Pool, terms []GlossaryTermFixture) error

	// Cleanup
	TruncateTables(ctx context.Context, db *pgxpool.Pool, tables ...string) error

	// Migrations
	RunMigrations(ctx context.Context, db *pgxpool.Pool) error
}

// PersonFixture represents a person in test fixtures.
type PersonFixture struct {
	ID            int64
	CanonicalName string
	Email         string
	Aliases       []string
	Title         string
	TeamID        int64
	ManagerID     *int64
}

// GlossaryTermFixture represents a glossary term in test fixtures.
type GlossaryTermFixture struct {
	Term             string
	Expansion        *string
	Definition       *string
	Context          *string
	Aliases          []string
	LinkedEntityType *string
	LinkedEntityID   *int64
}
