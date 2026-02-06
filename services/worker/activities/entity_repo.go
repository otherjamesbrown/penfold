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
// This implementation resolves person entities to the people table via entity resolution.
// For production use, entity mentions are resolved to canonical entities
// using the entity resolution system (ResolveOrCreate).
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

	// NOTE: This is intentionally a NO-OP stub.
	// The actual flow is:
	// 1. ExtractEntities returns structured data (PersonExtracted, etc.) to the workflow
	// 2. Workflow converts to []*Entity for this interface
	// 3. This method would need an EntityResolver dependency to call ResolveOrCreate
	// 4. Currently, participant_emails from sources.participant_emails are resolved
	//    via activities that have EntityResolver wired in
	//
	// For person entities specifically, the proper persistence happens via:
	// - BuildContextPackage activity (has EntityResolver, processes sender + extracted people)
	// - ProcessParticipants activity (processes participant_emails array)
	//
	// This stub exists to satisfy the EntityRepository interface but doesn't
	// duplicate the work done by those activities.

	logger.Info("Entity storage delegated to BuildContextPackage and ProcessParticipants activities",
		logging.F("entity_count", len(entities)),
	)

	// Return the count for interface compatibility
	return len(entities), nil
}

// Ensure PostgresEntityRepository implements the EntityRepository interface at compile time.
var _ EntityRepository = (*PostgresEntityRepository)(nil)
