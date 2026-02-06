// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// EmbeddingActivities holds dependencies for embedding-related activities.
type EmbeddingActivities struct {
	logger        logging.Logger
	aiClient      AIClient
	embeddingRepo EmbeddingRepository
	pipelineRepo  PipelineRepository
}

// NewEmbeddingActivities creates a new EmbeddingActivities instance.
func NewEmbeddingActivities(logger logging.Logger, aiClient AIClient, embeddingRepo EmbeddingRepository, pipelineRepo PipelineRepository) *EmbeddingActivities {
	return &EmbeddingActivities{
		logger:        logger.With(logging.F("component", "embedding_activities")),
		aiClient:      aiClient,
		embeddingRepo: embeddingRepo,
		pipelineRepo:  pipelineRepo,
	}
}

// GenerateEmbedding generates a vector embedding for the given content.
// This activity calls the AI service to create embeddings and stores them in the database.
func (a *EmbeddingActivities) GenerateEmbedding(ctx context.Context, input workflows.GenerateEmbeddingInput) (int64, error) {
	// Set trace_id in context for log correlation
	if input.ContentID != "" {
		ctx = context.WithValue(ctx, logging.TraceIDKey, input.ContentID)
	}
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GenerateEmbedding"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("content_length", len(input.Content)),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting embedding generation")

	logger.Info("Generating embedding for content")

	// Check for cancellation before expensive operations
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
		logger.Warn("AI client not configured")
		return 0, temporal.NewApplicationErrorWithCause(
			"AI client not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Call AI service to generate embedding
	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "calling AI service")

	// Start embedding trace span
	contentID := input.ContentID
	if contentID == "" {
		contentID = fmt.Sprintf("%d", input.SourceID)
	}
	ctx, embSpan := tracing.StartEmbedding(ctx, "ai.embedding", tracing.EmbeddingOptions{
		System:    tracing.AISystemMLX,
		TenantID:  input.TenantID,
		ContentID: contentID,
	})
	defer embSpan.End()

	embeddingReq := &aiv1.EmbeddingRequest{
		Text:     input.Content,
		TenantId: &input.TenantID,
	}

	resp, err := a.aiClient.GenerateEmbedding(ctx, embeddingReq)
	if err != nil {
		tracing.SetEmbeddingResult(embSpan, tracing.EmbeddingResult{
			LatencyMs: time.Since(startTime).Milliseconds(),
			Error:     err,
		})
		logger.Error("Failed to generate embedding from AI service", logging.Err(err))
		return 0, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// Record embedding result
	tracing.SetEmbeddingResult(embSpan, tracing.EmbeddingResult{
		Dimensions: int(resp.Dimensions),
		LatencyMs:  time.Since(startTime).Milliseconds(),
	})

	// Record heartbeat after AI call
	activity.RecordHeartbeat(ctx, "embedding generated, storing")

	logger.Info("Embedding generated successfully",
		logging.F("ai_duration", time.Since(startTime)),
		logging.F("dimensions", int(resp.Dimensions)),
		logging.F("model", resp.ModelUsed),
	)

	// Check if repository is available for storage
	if a.embeddingRepo == nil {
		logger.Warn("Embedding repository not configured, skipping storage")
		// Return 0 to indicate no stored embedding, but operation was successful
		return 0, nil
	}

	// Store the embedding
	storeStart := time.Now()
	embeddingID, err := a.embeddingRepo.StoreEmbedding(
		ctx,
		input.TenantID,
		input.SourceID,
		resp.Vector,
		resp.ModelUsed,
		resp.Dimensions,
	)
	if err != nil {
		logger.Error("Failed to store embedding", logging.Err(err))
		return 0, fmt.Errorf("failed to store embedding: %w", err)
	}

	logger.Info("Embedding stored successfully",
		logging.F("store_duration", time.Since(storeStart)),
		logging.F("embedding_id", embeddingID),
	)

	// Record pipeline run for provenance tracking (Stage 5: embed)
	if a.pipelineRepo != nil {
		durationMS := int(time.Since(startTime).Milliseconds())
		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:   input.SourceID,
			Stage:      "embed",
			ModelID:    resp.ModelUsed,
			Status:     "completed",
			DurationMS: durationMS,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run", logging.Err(runErr))
		}
	}

	return embeddingID, nil
}

// GenerateEmbeddingBatch generates embeddings for multiple content items.
// This is an optimization for batch processing scenarios.
type GenerateEmbeddingBatchInput struct {
	TenantID string           `json:"tenant_id"`
	// ContentID is the unique content identifier for tracing (format: <type:2>-<base62:8>)
	ContentID string           `json:"content_id,omitempty"`
	Items     []EmbeddingInput `json:"items"`
}

// EmbeddingInput represents a single item for embedding generation.
type EmbeddingInput struct {
	SourceID int64  `json:"source_id"`
	// ContentID is the unique content identifier for tracing (format: <type:2>-<base62:8>)
	ContentID   string `json:"content_id,omitempty"`
	Content     string `json:"content"`
	ContentHash string `json:"content_hash"`
}

// GenerateEmbeddingBatchOutput contains the results of batch embedding generation.
type GenerateEmbeddingBatchOutput struct {
	Results []EmbeddingResult `json:"results"`
}

// EmbeddingResult represents the result of a single embedding generation.
type EmbeddingResult struct {
	SourceID    int64  `json:"source_id"`
	EmbeddingID int64  `json:"embedding_id"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

// GenerateEmbeddingBatch generates embeddings for multiple content items in a single activity.
// This reduces overhead for batch processing workflows.
func (a *EmbeddingActivities) GenerateEmbeddingBatch(ctx context.Context, input GenerateEmbeddingBatchInput) (*GenerateEmbeddingBatchOutput, error) {
	// Set trace_id in context for log correlation (for batch operations)
	if input.ContentID != "" {
		ctx = context.WithValue(ctx, logging.TraceIDKey, input.ContentID)
	}
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GenerateEmbeddingBatch"),
		logging.F("tenant_id", input.TenantID),
		logging.F("batch_size", len(input.Items)),
	)

	logger.Info("Starting batch embedding generation")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if len(input.Items) == 0 {
		return &GenerateEmbeddingBatchOutput{Results: []EmbeddingResult{}}, nil
	}

	results := make([]EmbeddingResult, len(input.Items))

	for i, item := range input.Items {
		// Record heartbeat for each item
		activity.RecordHeartbeat(ctx, fmt.Sprintf("processing item %d/%d", i+1, len(input.Items)))

		// Check for cancellation between items
		if ctx.Err() != nil {
			// Mark remaining items as failed
			for j := i; j < len(input.Items); j++ {
				results[j] = EmbeddingResult{
					SourceID: input.Items[j].SourceID,
					Success:  false,
					Error:    "context cancelled",
				}
			}
			return &GenerateEmbeddingBatchOutput{Results: results}, ctx.Err()
		}

		// Generate embedding for this item
		embeddingID, err := a.GenerateEmbedding(ctx, workflows.GenerateEmbeddingInput{
			TenantID:    input.TenantID,
			SourceID:    item.SourceID,
			Content:     item.Content,
			ContentHash: item.ContentHash,
		})

		if err != nil {
			logger.Warn("Failed to generate embedding for item in batch",
				logging.F("source_id", item.SourceID),
				logging.Err(err),
			)

			results[i] = EmbeddingResult{
				SourceID: item.SourceID,
				Success:  false,
				Error:    err.Error(),
			}
		} else {
			results[i] = EmbeddingResult{
				SourceID:    item.SourceID,
				EmbeddingID: embeddingID,
				Success:     true,
			}
		}
	}

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	logger.Info("Batch embedding generation completed",
		logging.F("success_count", successCount),
		logging.F("failure_count", len(input.Items)-successCount),
	)

	return &GenerateEmbeddingBatchOutput{Results: results}, nil
}

// DeleteEmbedding deletes a single embedding by ID.
// This is a compensation activity used by the saga pattern when downstream stages fail.
func (a *EmbeddingActivities) DeleteEmbedding(ctx context.Context, embeddingID int64) error {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "DeleteEmbedding"),
		logging.F("embedding_id", embeddingID),
	)

	activity.RecordHeartbeat(ctx, "deleting embedding")

	logger.Info("Deleting embedding for saga compensation")

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if embeddingID <= 0 {
		return temporal.NewApplicationError(
			fmt.Sprintf("invalid embedding_id: %d", embeddingID),
			"ValidationError",
		)
	}

	if a.embeddingRepo == nil {
		logger.Warn("Embedding repository not configured")
		return temporal.NewApplicationErrorWithCause(
			"embedding repository not configured",
			"ConfigurationError",
			nil,
		)
	}

	if err := a.embeddingRepo.DeleteEmbedding(ctx, embeddingID); err != nil {
		logger.Error("Failed to delete embedding", logging.Err(err))
		return fmt.Errorf("failed to delete embedding: %w", err)
	}

	logger.Info("Embedding deleted successfully")
	return nil
}

// Ensure EmbeddingActivities implements required interfaces at compile time.
var _ interface {
	GenerateEmbedding(ctx context.Context, input workflows.GenerateEmbeddingInput) (int64, error)
	DeleteEmbedding(ctx context.Context, embeddingID int64) error
} = (*EmbeddingActivities)(nil)
