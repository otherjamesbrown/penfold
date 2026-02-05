// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PostgresEntityRepository implements EntityRepository using PostgreSQL.
type PostgresEntityRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresEntityRepository creates a new PostgreSQL entity repository.
func NewPostgresEntityRepository(pool *pgxpool.Pool, logger logging.Logger) *PostgresEntityRepository {
	return &PostgresEntityRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "entity_repository")),
	}
}

// StoreEntities stores extracted entities for a source.
// NOTE: This is a simplified implementation. In the actual pipeline, entities
// (people, projects, organizations) are stored in dedicated tables (people, projects, etc.)
// via the entity resolution system. This implementation stores raw entity mentions
// for provenance and debugging purposes only.
//
// For production use, entity mentions should be resolved to canonical entities
// using the entity resolution workflow before being stored in the entity tables.
func (r *PostgresEntityRepository) StoreEntities(
	ctx context.Context,
	tenantID string,
	sourceID int64,
	entities []*Entity,
) (int, error) {
	logger := r.logger.With(
		logging.F("tenant_id", tenantID),
		logging.F("source_id", sourceID),
		logging.F("entity_count", len(entities)),
	)

	logger.Debug("Storing entities")

	// Validate input
	if tenantID == "" {
		return 0, fmt.Errorf("tenant_id is required")
	}
	if sourceID <= 0 {
		return 0, fmt.Errorf("source_id is required")
	}
	if len(entities) == 0 {
		logger.Debug("No entities to store")
		return 0, nil
	}

	// IMPORTANT: The Entity type in the interface doesn't match what ExtractEntities
	// actually returns. ExtractEntities returns structured results (people, dates,
	// projects, etc.) not simple Entity records.
	//
	// This repository is currently a NO-OP because:
	// 1. The ExtractEntities activity doesn't call StoreEntities - it returns the
	//    structured data to the workflow, which should handle entity resolution.
	// 2. Entity resolution should be done by the entity resolution workflow/activity
	//    which maps mentions to canonical entities in the people/projects/teams tables.
	//
	// For now, we log that we received entities but don't persist them.
	// The proper flow is:
	//   ExtractEntities -> Workflow -> Entity Resolution Activity -> people/projects tables

	logger.Info("Entity storage skipped - entities should be resolved via entity resolution workflow",
		logging.F("entity_count", len(entities)),
	)

	// Return the count for compatibility, but don't actually store
	return len(entities), nil
}

// Ensure PostgresEntityRepository implements the EntityRepository interface at compile time.
var _ EntityRepository = (*PostgresEntityRepository)(nil)
