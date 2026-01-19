// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

// StorageActivities holds dependencies for storage-related activities.
type StorageActivities struct {
	logger           zerolog.Logger
	contentRepo      ContentRepository
	embeddingRepo    EmbeddingRepository
	relationshipRepo RelationshipRepository
}

// NewStorageActivities creates a new StorageActivities instance.
func NewStorageActivities(
	logger zerolog.Logger,
	contentRepo ContentRepository,
	embeddingRepo EmbeddingRepository,
	relationshipRepo RelationshipRepository,
) *StorageActivities {
	return &StorageActivities{
		logger:           logger.With().Str("component", "storage_activities").Logger(),
		contentRepo:      contentRepo,
		embeddingRepo:    embeddingRepo,
		relationshipRepo: relationshipRepo,
	}
}

// StoreContentInput is the input for the StoreContent activity.
type StoreContentInput struct {
	TenantID         string            `json:"tenant_id"`
	SourceID         int64             `json:"source_id"`
	SourceType       string            `json:"source_type"`
	RawContent       string            `json:"raw_content"`
	ProcessedContent string            `json:"processed_content,omitempty"`
	ContentHash      string            `json:"content_hash"`
	Status           string            `json:"status"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// StoreContentOutput contains the result of storing content.
type StoreContentOutput struct {
	ContentID int64 `json:"content_id"`
}

// StoreContent stores processed content in the database.
// This activity persists content that has been processed through the pipeline.
func (a *StorageActivities) StoreContent(ctx context.Context, input StoreContentInput) (*StoreContentOutput, error) {
	logger := a.logger.With().
		Str("activity", "StoreContent").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Str("source_type", input.SourceType).
		Str("status", input.Status).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting content storage")

	logger.Info().Msg("Storing content")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, temporal.NewApplicationError("tenant_id is required", "ValidationError")
	}
	if input.SourceType == "" {
		return nil, temporal.NewApplicationError("source_type is required", "ValidationError")
	}
	if input.ContentHash == "" {
		return nil, temporal.NewApplicationError("content_hash is required", "ValidationError")
	}

	// Check if repository is available
	if a.contentRepo == nil {
		logger.Warn().Msg("Content repository not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"content repository not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Build content record
	content := &ContentRecord{
		TenantID:         input.TenantID,
		SourceID:         input.SourceID,
		SourceType:       input.SourceType,
		RawContent:       input.RawContent,
		ProcessedContent: input.ProcessedContent,
		ContentHash:      input.ContentHash,
		Status:           input.Status,
		Metadata:         input.Metadata,
	}

	// Store the content
	startTime := time.Now()
	contentID, err := a.contentRepo.StoreContent(ctx, content)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to store content")
		return nil, fmt.Errorf("failed to store content: %w", err)
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Int64("content_id", contentID).
		Msg("Content stored successfully")

	return &StoreContentOutput{ContentID: contentID}, nil
}

// StoreEmbeddingInput is the input for the StoreEmbedding activity.
type StoreEmbeddingInput struct {
	TenantID   string    `json:"tenant_id"`
	SourceID   int64     `json:"source_id"`
	Vector     []float32 `json:"vector"`
	Model      string    `json:"model"`
	Dimensions int32     `json:"dimensions"`
}

// StoreEmbeddingOutput contains the result of storing an embedding.
type StoreEmbeddingOutput struct {
	EmbeddingID int64 `json:"embedding_id"`
}

// StoreEmbedding stores a pre-computed embedding vector in the database.
// This activity is useful when embeddings are generated externally or need to be stored separately.
func (a *StorageActivities) StoreEmbedding(ctx context.Context, input StoreEmbeddingInput) (*StoreEmbeddingOutput, error) {
	logger := a.logger.With().
		Str("activity", "StoreEmbedding").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Str("model", input.Model).
		Int32("dimensions", input.Dimensions).
		Int("vector_length", len(input.Vector)).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting embedding storage")

	logger.Info().Msg("Storing embedding")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, temporal.NewApplicationError("tenant_id is required", "ValidationError")
	}
	if len(input.Vector) == 0 {
		return nil, temporal.NewApplicationError("vector is empty", "ValidationError")
	}
	if input.Dimensions == 0 {
		input.Dimensions = int32(len(input.Vector))
	}
	if int(input.Dimensions) != len(input.Vector) {
		return nil, temporal.NewApplicationError(
			fmt.Sprintf("dimensions mismatch: declared %d, actual %d", input.Dimensions, len(input.Vector)),
			"ValidationError",
		)
	}

	// Check if repository is available
	if a.embeddingRepo == nil {
		logger.Warn().Msg("Embedding repository not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"embedding repository not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Store the embedding
	startTime := time.Now()
	embeddingID, err := a.embeddingRepo.StoreEmbedding(
		ctx,
		input.TenantID,
		input.SourceID,
		input.Vector,
		input.Model,
		input.Dimensions,
	)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to store embedding")
		return nil, fmt.Errorf("failed to store embedding: %w", err)
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Int64("embedding_id", embeddingID).
		Msg("Embedding stored successfully")

	return &StoreEmbeddingOutput{EmbeddingID: embeddingID}, nil
}

// StoreRelationshipInput is the input for the StoreRelationship activity.
type StoreRelationshipInput struct {
	TenantID       string            `json:"tenant_id"`
	SourceEntityID string            `json:"source_entity_id"`
	TargetEntityID string            `json:"target_entity_id"`
	RelationType   string            `json:"relation_type"`
	Confidence     float32           `json:"confidence"`
	SourceID       int64             `json:"source_id"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// StoreRelationshipOutput contains the result of storing a relationship.
type StoreRelationshipOutput struct {
	RelationshipID int64 `json:"relationship_id"`
}

// StoreRelationship stores a relationship between entities in the database.
// This activity creates edges in the entity graph.
func (a *StorageActivities) StoreRelationship(ctx context.Context, input StoreRelationshipInput) (*StoreRelationshipOutput, error) {
	logger := a.logger.With().
		Str("activity", "StoreRelationship").
		Str("tenant_id", input.TenantID).
		Str("source_entity", input.SourceEntityID).
		Str("target_entity", input.TargetEntityID).
		Str("relation_type", input.RelationType).
		Float32("confidence", input.Confidence).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting relationship storage")

	logger.Info().Msg("Storing relationship")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, temporal.NewApplicationError("tenant_id is required", "ValidationError")
	}
	if input.SourceEntityID == "" {
		return nil, temporal.NewApplicationError("source_entity_id is required", "ValidationError")
	}
	if input.TargetEntityID == "" {
		return nil, temporal.NewApplicationError("target_entity_id is required", "ValidationError")
	}
	if input.RelationType == "" {
		return nil, temporal.NewApplicationError("relation_type is required", "ValidationError")
	}
	if input.Confidence < 0 || input.Confidence > 1 {
		return nil, temporal.NewApplicationError(
			fmt.Sprintf("confidence must be between 0 and 1, got %f", input.Confidence),
			"ValidationError",
		)
	}

	// Check if repository is available
	if a.relationshipRepo == nil {
		logger.Warn().Msg("Relationship repository not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"relationship repository not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Build relationship record
	rel := &Relationship{
		TenantID:       input.TenantID,
		SourceEntityID: input.SourceEntityID,
		TargetEntityID: input.TargetEntityID,
		RelationType:   input.RelationType,
		Confidence:     input.Confidence,
		SourceID:       input.SourceID,
		Metadata:       input.Metadata,
	}

	// Store the relationship
	startTime := time.Now()
	relID, err := a.relationshipRepo.StoreRelationship(ctx, rel)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to store relationship")
		return nil, fmt.Errorf("failed to store relationship: %w", err)
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Int64("relationship_id", relID).
		Msg("Relationship stored successfully")

	return &StoreRelationshipOutput{RelationshipID: relID}, nil
}

// StoreRelationshipBatchInput is the input for batch relationship storage.
type StoreRelationshipBatchInput struct {
	TenantID      string                   `json:"tenant_id"`
	Relationships []StoreRelationshipInput `json:"relationships"`
}

// StoreRelationshipBatchOutput contains the results of batch relationship storage.
type StoreRelationshipBatchOutput struct {
	SuccessCount int                  `json:"success_count"`
	FailureCount int                  `json:"failure_count"`
	Results      []RelationshipResult `json:"results"`
}

// RelationshipResult represents the result of storing a single relationship.
type RelationshipResult struct {
	SourceEntityID string `json:"source_entity_id"`
	TargetEntityID string `json:"target_entity_id"`
	RelationshipID int64  `json:"relationship_id,omitempty"`
	Success        bool   `json:"success"`
	Error          string `json:"error,omitempty"`
}

// StoreRelationshipBatch stores multiple relationships in a single activity.
// This is more efficient than calling StoreRelationship multiple times.
func (a *StorageActivities) StoreRelationshipBatch(ctx context.Context, input StoreRelationshipBatchInput) (*StoreRelationshipBatchOutput, error) {
	logger := a.logger.With().
		Str("activity", "StoreRelationshipBatch").
		Str("tenant_id", input.TenantID).
		Int("batch_size", len(input.Relationships)).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting batch relationship storage")

	logger.Info().Msg("Storing relationships in batch")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if len(input.Relationships) == 0 {
		return &StoreRelationshipBatchOutput{
			SuccessCount: 0,
			FailureCount: 0,
			Results:      []RelationshipResult{},
		}, nil
	}

	results := make([]RelationshipResult, len(input.Relationships))
	successCount := 0
	failureCount := 0

	for i, rel := range input.Relationships {
		// Record heartbeat for each item
		activity.RecordHeartbeat(ctx, fmt.Sprintf("processing relationship %d/%d", i+1, len(input.Relationships)))

		// Check for cancellation between items
		if ctx.Err() != nil {
			// Mark remaining items as failed
			for j := i; j < len(input.Relationships); j++ {
				results[j] = RelationshipResult{
					SourceEntityID: input.Relationships[j].SourceEntityID,
					TargetEntityID: input.Relationships[j].TargetEntityID,
					Success:        false,
					Error:          "context cancelled",
				}
				failureCount++
			}
			return &StoreRelationshipBatchOutput{
				SuccessCount: successCount,
				FailureCount: failureCount,
				Results:      results,
			}, ctx.Err()
		}

		// Ensure tenant ID is set
		if rel.TenantID == "" {
			rel.TenantID = input.TenantID
		}

		// Store this relationship
		output, err := a.StoreRelationship(ctx, rel)
		if err != nil {
			logger.Warn().
				Str("source_entity", rel.SourceEntityID).
				Str("target_entity", rel.TargetEntityID).
				Err(err).
				Msg("Failed to store relationship in batch")

			results[i] = RelationshipResult{
				SourceEntityID: rel.SourceEntityID,
				TargetEntityID: rel.TargetEntityID,
				Success:        false,
				Error:          err.Error(),
			}
			failureCount++
		} else {
			results[i] = RelationshipResult{
				SourceEntityID: rel.SourceEntityID,
				TargetEntityID: rel.TargetEntityID,
				RelationshipID: output.RelationshipID,
				Success:        true,
			}
			successCount++
		}
	}

	logger.Info().
		Int("success_count", successCount).
		Int("failure_count", failureCount).
		Msg("Batch relationship storage completed")

	return &StoreRelationshipBatchOutput{
		SuccessCount: successCount,
		FailureCount: failureCount,
		Results:      results,
	}, nil
}

// Ensure StorageActivities implements the expected interface at compile time.
var _ interface {
	StoreContent(ctx context.Context, input StoreContentInput) (*StoreContentOutput, error)
	StoreEmbedding(ctx context.Context, input StoreEmbeddingInput) (*StoreEmbeddingOutput, error)
	StoreRelationship(ctx context.Context, input StoreRelationshipInput) (*StoreRelationshipOutput, error)
} = (*StorageActivities)(nil)
