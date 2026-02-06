// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PostgresAssertionRepository implements AssertionRepository using PostgreSQL.
type PostgresAssertionRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresAssertionRepository creates a new PostgreSQL assertion repository.
func NewPostgresAssertionRepository(pool *pgxpool.Pool, logger logging.Logger) *PostgresAssertionRepository {
	return &PostgresAssertionRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "assertion_repository")),
	}
}

// StoreAssertions stores extracted assertions for a source.
// It inserts assertions into the assertions table as structured records.
func (r *PostgresAssertionRepository) StoreAssertions(
	ctx context.Context,
	tenantID string,
	sourceID int64,
	assertions []*Assertion,
	model string,
) (int, error) {
	logger := r.logger.With(
		logging.F("tenant_id", tenantID),
		logging.F("source_id", sourceID),
		logging.F("model", model),
		logging.F("assertion_count", len(assertions)),
	)

	logger.Debug("Storing assertions")

	// Validate input
	if tenantID == "" {
		return 0, fmt.Errorf("tenant_id is required")
	}
	if sourceID <= 0 {
		return 0, fmt.Errorf("source_id is required")
	}
	if len(assertions) == 0 {
		logger.Debug("No assertions to store")
		return 0, nil
	}

	// Use a transaction to insert all assertions
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		logger.Error("Failed to begin transaction", logging.Err(err))
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	insertedCount := 0

	// Insert each assertion
	for _, assertion := range assertions {
		// Map the assertion to the assertions table schema
		// The Subject-Predicate-Object triple needs to be formatted as content
		content := fmt.Sprintf("%s %s %s", assertion.Subject, assertion.Predicate, assertion.Object)

		// Determine assertion_type from category or default to "issue"
		assertionType := mapCategoryToType(assertion.Category)

		// Build processing_metadata JSON
		metadata := map[string]interface{}{
			"subject":     assertion.Subject,
			"predicate":   assertion.Predicate,
			"object":      assertion.Object,
			"source_text": assertion.SourceText,
			"category":    assertion.Category,
		}
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			logger.Warn("Failed to marshal assertion metadata",
				logging.Err(err),
				logging.F("assertion", content),
			)
			metadataJSON = []byte("{}")
		}

		// Insert assertion
		query := `
			INSERT INTO assertions (
				tenant_id,
				source_id,
				assertion_type,
				description,
				source_quote,
				confidence,
				extraction_model,
				processing_metadata
			) VALUES (
				$1::uuid,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8
			)
			ON CONFLICT DO NOTHING
		`

		_, err = tx.Exec(
			ctx,
			query,
			tenantID,
			sourceID,
			assertionType,
			content,
			assertion.SourceText, // context
			assertion.Confidence,
			model,
			metadataJSON,
		)

		if err != nil {
			logger.Error("Failed to insert assertion",
				logging.Err(err),
				logging.F("content", content),
			)
			return 0, fmt.Errorf("failed to insert assertion: %w", err)
		}

		insertedCount++
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		logger.Error("Failed to commit transaction", logging.Err(err))
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Info("Assertions stored successfully",
		logging.F("inserted_count", insertedCount),
	)

	return insertedCount, nil
}

// mapCategoryToType maps an assertion category to a valid assertion_type.
// Valid types are: decision, risk, commitment, milestone, outcome, dependency, assumption, issue
func mapCategoryToType(category string) string {
	switch category {
	case "decision", "decisions":
		return "decision"
	case "risk", "risks":
		return "risk"
	case "commitment", "commitments":
		return "commitment"
	case "milestone", "milestones":
		return "milestone"
	case "outcome", "outcomes":
		return "outcome"
	case "dependency", "dependencies":
		return "dependency"
	case "assumption", "assumptions":
		return "assumption"
	case "issue", "issues":
		return "issue"
	default:
		// Default to "issue" for unknown categories
		return "issue"
	}
}

// Ensure PostgresAssertionRepository implements the AssertionRepository interface at compile time.
var _ AssertionRepository = (*PostgresAssertionRepository)(nil)
