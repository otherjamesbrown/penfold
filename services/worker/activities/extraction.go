// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// ExtractionActivities holds dependencies for extraction-related activities.
type ExtractionActivities struct {
	logger         zerolog.Logger
	aiClient       AIClient
	assertionRepo  AssertionRepository
	entityRepo     EntityRepository
}

// NewExtractionActivities creates a new ExtractionActivities instance.
func NewExtractionActivities(
	logger zerolog.Logger,
	aiClient AIClient,
	assertionRepo AssertionRepository,
	entityRepo EntityRepository,
) *ExtractionActivities {
	return &ExtractionActivities{
		logger:        logger.With().Str("component", "extraction_activities").Logger(),
		aiClient:      aiClient,
		assertionRepo: assertionRepo,
		entityRepo:    entityRepo,
	}
}

// ExtractAssertions extracts assertions from the given content using an LLM.
// Assertions are subject-predicate-object triples representing facts and claims.
func (a *ExtractionActivities) ExtractAssertions(ctx context.Context, input workflows.ExtractAssertionsInput) (int, error) {
	logger := a.logger.With().
		Str("activity", "ExtractAssertions").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Str("job_id", input.JobID).
		Int("content_length", len(input.Content)).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting assertion extraction")

	logger.Info().Msg("Extracting assertions from content")

	// Check for cancellation
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	// Validate input
	if input.Content == "" {
		return 0, temporal.NewApplicationError(
			"content is empty",
			"ValidationError",
		)
	}

	// Check if AI client is available
	if a.aiClient == nil {
		logger.Warn().Msg("AI client not configured")
		return 0, temporal.NewApplicationErrorWithCause(
			"AI client not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Call AI service to extract assertions
	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "calling AI service for assertion extraction")

	// Default parameters for assertion extraction
	minConfidence := float32(0.5)
	maxAssertions := int32(20)

	assertionReq := &aiv1.AssertionRequest{
		Content:       input.Content,
		MinConfidence: &minConfidence,
		MaxAssertions: &maxAssertions,
		TenantId:      &input.TenantID,
	}

	resp, err := a.aiClient.ExtractAssertions(ctx, assertionReq)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to extract assertions from AI service")
		return 0, fmt.Errorf("failed to extract assertions: %w", err)
	}

	// Record heartbeat after AI call
	activity.RecordHeartbeat(ctx, "assertions extracted, processing results")

	logger.Info().
		Dur("ai_duration", time.Since(startTime)).
		Int("assertions_found", len(resp.Assertions)).
		Int32("total_found", resp.TotalFound).
		Int32("filtered_count", resp.FilteredCount).
		Str("model", resp.ModelUsed).
		Msg("Assertions extracted successfully")

	// If no assertions found, return early
	if len(resp.Assertions) == 0 {
		logger.Info().Msg("No assertions found in content")
		return 0, nil
	}

	// Check if repository is available for storage
	if a.assertionRepo == nil {
		logger.Warn().Msg("Assertion repository not configured, skipping storage")
		return len(resp.Assertions), nil
	}

	// Convert proto assertions to domain model
	assertions := make([]*Assertion, len(resp.Assertions))
	for i, pa := range resp.Assertions {
		assertions[i] = &Assertion{
			Subject:    pa.Subject,
			Predicate:  pa.Predicate,
			Object:     pa.Object,
			Confidence: pa.Confidence,
			SourceText: pa.GetSourceText(),
			Category:   pa.GetCategory(),
		}
	}

	// Store the assertions
	storeStart := time.Now()
	count, err := a.assertionRepo.StoreAssertions(ctx, input.TenantID, input.SourceID, assertions, resp.ModelUsed)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to store assertions")
		return 0, fmt.Errorf("failed to store assertions: %w", err)
	}

	logger.Info().
		Dur("store_duration", time.Since(storeStart)).
		Int("stored_count", count).
		Msg("Assertions stored successfully")

	return count, nil
}

// ExtractEntitiesInput is the input for the ExtractEntities activity.
type ExtractEntitiesInput struct {
	TenantID string `json:"tenant_id"`
	SourceID int64  `json:"source_id"`
	JobID    string `json:"job_id"`
	Content  string `json:"content"`
}

// ExtractEntitiesOutput is the output from the ExtractEntities activity.
type ExtractEntitiesOutput struct {
	EntityCount int               `json:"entity_count"`
	Entities    []ExtractedEntity `json:"entities"`
}

// ExtractedEntity represents an entity extracted from content.
type ExtractedEntity struct {
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Confidence float32 `json:"confidence"`
}

// ExtractEntities extracts named entities from the given content.
// This identifies people, organizations, locations, dates, and other named entities.
func (a *ExtractionActivities) ExtractEntities(ctx context.Context, input ExtractEntitiesInput) (*ExtractEntitiesOutput, error) {
	logger := a.logger.With().
		Str("activity", "ExtractEntities").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Str("job_id", input.JobID).
		Int("content_length", len(input.Content)).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting entity extraction")

	logger.Info().Msg("Extracting entities from content")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.Content == "" {
		return nil, temporal.NewApplicationError(
			"content is empty",
			"ValidationError",
		)
	}

	// Check if AI client is available
	if a.aiClient == nil {
		logger.Warn().Msg("AI client not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"AI client not configured",
			"ConfigurationError",
			nil,
		)
	}

	// For entity extraction, we use the assertion extraction as a proxy
	// since the AI service doesn't have a dedicated NER endpoint yet.
	// In a real implementation, this would call a dedicated NER model.
	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "calling AI service for entity extraction")

	// Use assertion extraction to identify entities as subjects/objects
	minConfidence := float32(0.6)
	maxAssertions := int32(50) // Extract more to get diverse entities

	assertionReq := &aiv1.AssertionRequest{
		Content:       input.Content,
		MinConfidence: &minConfidence,
		MaxAssertions: &maxAssertions,
		TenantId:      &input.TenantID,
	}

	resp, err := a.aiClient.ExtractAssertions(ctx, assertionReq)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to extract entities from AI service")
		return nil, fmt.Errorf("failed to extract entities: %w", err)
	}

	// Record heartbeat after AI call
	activity.RecordHeartbeat(ctx, "entities extracted, processing results")

	// Extract unique entities from subjects and objects
	entityMap := make(map[string]*Entity)
	for _, assertion := range resp.Assertions {
		// Add subject as potential entity
		if assertion.Subject != "" {
			key := assertion.Subject
			if existing, ok := entityMap[key]; ok {
				// Update confidence if higher
				if assertion.Confidence > existing.Confidence {
					existing.Confidence = assertion.Confidence
				}
			} else {
				entityMap[key] = &Entity{
					Name:       assertion.Subject,
					Type:       inferEntityType(assertion.Subject, assertion.GetCategory()),
					Confidence: assertion.Confidence,
				}
			}
		}

		// Add object as potential entity
		if assertion.Object != "" {
			key := assertion.Object
			if existing, ok := entityMap[key]; ok {
				if assertion.Confidence > existing.Confidence {
					existing.Confidence = assertion.Confidence
				}
			} else {
				entityMap[key] = &Entity{
					Name:       assertion.Object,
					Type:       inferEntityType(assertion.Object, assertion.GetCategory()),
					Confidence: assertion.Confidence,
				}
			}
		}
	}

	// Convert map to slice
	entities := make([]*Entity, 0, len(entityMap))
	for _, e := range entityMap {
		entities = append(entities, e)
	}

	logger.Info().
		Dur("ai_duration", time.Since(startTime)).
		Int("entities_found", len(entities)).
		Str("model", resp.ModelUsed).
		Msg("Entities extracted successfully")

	// Check if repository is available for storage
	if a.entityRepo != nil {
		storeStart := time.Now()
		count, err := a.entityRepo.StoreEntities(ctx, input.TenantID, input.SourceID, entities)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to store entities")
			return nil, fmt.Errorf("failed to store entities: %w", err)
		}
		logger.Info().
			Dur("store_duration", time.Since(storeStart)).
			Int("stored_count", count).
			Msg("Entities stored successfully")
	}

	// Build output
	output := &ExtractEntitiesOutput{
		EntityCount: len(entities),
		Entities:    make([]ExtractedEntity, len(entities)),
	}
	for i, e := range entities {
		output.Entities[i] = ExtractedEntity{
			Name:       e.Name,
			Type:       e.Type,
			Confidence: e.Confidence,
		}
	}

	return output, nil
}

// inferEntityType attempts to infer the entity type from the name and category.
func inferEntityType(name, category string) string {
	// Use category as a hint if available
	switch category {
	case "organizational":
		return "organization"
	case "temporal":
		return "date"
	case "location", "geographical":
		return "location"
	case "person", "personnel":
		return "person"
	}

	// Basic heuristics based on name patterns
	// In a real implementation, this would use NER models
	if len(name) == 0 {
		return "unknown"
	}

	// Default to unknown - a proper NER model would classify this
	return "unknown"
}

// Ensure ExtractionActivities implements required interfaces at compile time.
var _ interface {
	ExtractAssertions(ctx context.Context, input workflows.ExtractAssertionsInput) (int, error)
	ExtractEntities(ctx context.Context, input ExtractEntitiesInput) (*ExtractEntitiesOutput, error)
} = (*ExtractionActivities)(nil)
