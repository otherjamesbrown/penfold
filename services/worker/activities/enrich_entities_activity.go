package activities

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// EntityEnrichmentActivities holds dependencies for entity enrichment.
type EntityEnrichmentActivities struct {
	logger     logging.Logger
	entityRepo *entities.Repository
	db         *pgxpool.Pool
}

// NewEntityEnrichmentActivities creates a new EntityEnrichmentActivities instance.
func NewEntityEnrichmentActivities(
	logger logging.Logger,
	entityRepo *entities.Repository,
	db *pgxpool.Pool,
) *EntityEnrichmentActivities {
	return &EntityEnrichmentActivities{
		logger:     logger.With(logging.F("component", "entity_enrichment_activities")),
		entityRepo: entityRepo,
		db:         db,
	}
}

// EnrichEntities implements the enrich_entities pipeline stage.
// It computes communication patterns (code-only) and updates expertise areas.
func (a *EntityEnrichmentActivities) EnrichEntities(
	ctx context.Context,
	input workflows.EnrichEntitiesInput,
) (*workflows.EnrichEntitiesOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "EnrichEntities"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	recordHeartbeat(ctx, "starting entity enrichment")

	output := &workflows.EnrichEntitiesOutput{}

	if len(input.ResolvedPeople) == 0 {
		logger.Debug("No resolved people, skipping entity enrichment")
		return output, nil
	}

	// Phase 1: Update communication patterns (code-only, no LLM)
	patternsUpdated := a.updateCommunicationPatterns(ctx, logger, input)
	output.PatternsUpdated = patternsUpdated

	// Phase 2: Expertise inference is reserved for a future LLM-backed implementation.
	// The AI coordinator does not expose a generic completion endpoint; expertise updates
	// will be added when a dedicated RPC (e.g. ExtractExpertise) is available.
	output.ExpertiseUpdated = 0

	// Record enrichment stage for tracking
	a.recordEnrichmentStage(ctx, logger, input.SourceID, output)

	logger.Info("Entity enrichment complete",
		logging.F("patterns_updated", patternsUpdated),
		logging.F("expertise_updated", output.ExpertiseUpdated),
	)

	return output, nil
}

// updateCommunicationPatterns computes and merges communication pattern data
// for each resolved person based on content_mentions co-participation.
func (a *EntityEnrichmentActivities) updateCommunicationPatterns(
	ctx context.Context,
	logger logging.Logger,
	input workflows.EnrichEntitiesInput,
) int {
	updated := 0

	for _, person := range input.ResolvedPeople {
		if person.PersonID == nil {
			continue
		}
		personID := *person.PersonID

		// Compute co-participants from content_mentions
		type collaborator struct {
			CollaboratorID   int64     `json:"person_id"`
			InteractionCount int       `json:"interaction_count"`
			LastInteraction  time.Time `json:"last_interaction"`
		}

		rows, err := a.db.Query(ctx, `
			SELECT cm2.resolved_entity_id as collaborator_id,
			       COUNT(DISTINCT cm1.content_id) as interaction_count,
			       MAX(s.source_timestamp) as last_interaction
			FROM content_mentions cm1
			JOIN content_mentions cm2 ON cm1.content_id = cm2.content_id
			  AND cm1.resolved_entity_id != cm2.resolved_entity_id
			JOIN sources s ON cm1.content_id = s.id
			WHERE cm1.tenant_id = $1
			  AND cm1.resolved_entity_id = $2
			  AND cm2.resolved_entity_id IS NOT NULL
			GROUP BY cm2.resolved_entity_id
			ORDER BY interaction_count DESC
			LIMIT 20
		`, input.TenantID, personID)
		if err != nil {
			logger.Warn("Failed to query collaborators",
				logging.Err(err),
				logging.F("person_id", personID),
			)
			continue
		}

		var collabs []collaborator
		for rows.Next() {
			var c collaborator
			if err := rows.Scan(&c.CollaboratorID, &c.InteractionCount, &c.LastInteraction); err != nil {
				logger.Warn("Failed to scan collaborator row", logging.Err(err))
				continue
			}
			collabs = append(collabs, c)
		}
		rows.Close()

		// Compute sent/received counts
		var sentCount, receivedCount int
		_ = a.db.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE participation_role = 1) as sent,
				COUNT(*) FILTER (WHERE participation_role IN (2, 3)) as received
			FROM content_mentions
			WHERE tenant_id = $1 AND resolved_entity_id = $2
		`, input.TenantID, personID).Scan(&sentCount, &receivedCount)

		// Build communication patterns JSON
		patterns := map[string]interface{}{
			"email_frequency": map[string]interface{}{
				"sent_count":       sentCount,
				"received_count":   receivedCount,
				"last_computed_at": time.Now().UTC().Format(time.RFC3339),
			},
			"top_collaborators": collabs,
		}

		patternsJSON, err := json.Marshal(patterns)
		if err != nil {
			logger.Warn("Failed to marshal communication patterns", logging.Err(err))
			continue
		}

		if err := a.entityRepo.MergeCommunicationPatterns(ctx, personID, patternsJSON); err != nil {
			logger.Warn("Failed to merge communication patterns",
				logging.Err(err),
				logging.F("person_id", personID),
			)
			continue
		}

		updated++
	}

	return updated
}

// recordEnrichmentStage records the enrich_entities stage execution in enrichment_stages.
func (a *EntityEnrichmentActivities) recordEnrichmentStage(
	ctx context.Context,
	logger logging.Logger,
	sourceID int64,
	output *workflows.EnrichEntitiesOutput,
) {
	// Find the enrichment record for this source
	var enrichmentID int64
	err := a.db.QueryRow(ctx, `
		SELECT id FROM content_enrichment WHERE source_id = $1 ORDER BY created_at DESC LIMIT 1
	`, sourceID).Scan(&enrichmentID)
	if err != nil {
		logger.Debug("No enrichment record found for source, skipping stage recording",
			logging.F("source_id", sourceID),
		)
		return
	}

	outputData, _ := json.Marshal(output)

	_, err = a.db.Exec(ctx, `
		INSERT INTO enrichment_stages (enrichment_id, stage_name, processor_name, status, output_data, started_at, completed_at, duration_ms)
		VALUES ($1, 'enrich_entities', 'EntityEnrichment', 'completed', $2, NOW(), NOW(), 0)
	`, enrichmentID, outputData)
	if err != nil {
		logger.Warn("Failed to record enrichment stage", logging.Err(err))
	}
}
