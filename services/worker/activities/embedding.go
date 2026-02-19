// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/chunking"
	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
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
	if logger == nil {
		panic("NewEmbeddingActivities: logger is required")
	}
	if aiClient == nil {
		panic("NewEmbeddingActivities: aiClient is required")
	}
	if embeddingRepo == nil {
		panic("NewEmbeddingActivities: embeddingRepo is required")
	}
	// pipelineRepo is optional (provenance recording)
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

	// Check if repository is available for storage
	if a.embeddingRepo == nil {
		logger.Warn("Embedding repository not configured, skipping storage")
		return 0, nil
	}

	// Delete existing embeddings for this source before creating new chunks
	activity.RecordHeartbeat(ctx, "deleting existing embeddings")
	if err := a.embeddingRepo.DeleteEmbeddingsForSource(ctx, input.TenantID, input.SourceID); err != nil {
		pe := perrors.ClassifyError(err, "embed")
		logger.Error("Failed to delete existing embeddings", logging.Err(pe))
		return 0, WrapForTemporal(pe)
	}

	// Chunk content for embedding
	chunks := chunking.ChunkContent(input.Content, chunking.ChunkOptions{
		MaxTokens:     400,
		OverlapTokens: 50,
	})
	if len(chunks) == 0 {
		chunks = []chunking.Chunk{{Index: 0, Text: input.Content, Start: 0, End: len(input.Content)}}
	}

	logger.Info("Chunked content for embedding",
		logging.F("chunk_count", len(chunks)),
	)

	// Track the first chunk's embedding ID for backward compatibility
	var firstEmbeddingID int64
	startTime := time.Now()

	// Create stage span wrapping ALL chunk embedding calls
	stageCtx, stageSpan := tracing.StartStageSpan(ctx, "stage.embedding", tracing.StageSpanOptions{
		ContentID: input.ContentID,
		TenantID:  input.TenantID,
	})
	defer stageSpan.End()

	// Generate and store embeddings for each chunk
	for _, chunk := range chunks {
		activity.RecordHeartbeat(stageCtx, fmt.Sprintf("generating embedding for chunk %d/%d", chunk.Index+1, len(chunks)))

		embeddingReq := &aiv1.EmbeddingRequest{
			Text:     chunk.Text,
			TenantId: &input.TenantID,
		}
		if input.ContentID != "" {
			embeddingReq.ContentId = &input.ContentID
		}
		// PipelineTraceId and PipelineSpanId are deprecated: OTel interceptors propagate context automatically.

		// Call AI service to generate embedding with stage span context
		resp, err := a.aiClient.GenerateEmbedding(stageCtx, embeddingReq)
		if err != nil {
			pe := perrors.ClassifyError(err, "embed")
			logger.Error("Failed to generate embedding from AI service",
				logging.Err(pe),
				logging.F("chunk_index", chunk.Index),
			)
			return 0, WrapForTemporal(pe)
		}

		// Store the embedding with chunk metadata
		embeddingID, err := a.embeddingRepo.StoreMultiLevelEmbedding(ctx, &MultiLevelEmbeddingInput{
			TenantID:           input.TenantID,
			SourceID:           input.SourceID,
			EntityType:         "source",
			EntityID:           input.SourceID,
			RepresentationType: "content",
			Vector:             resp.Vector,
			Model:              resp.ModelUsed,
			ModelVersion:       resp.ModelUsed,
			TextContent:        chunk.Text,
			ContentHash:        input.ContentHash,
			ChunkIndex:         chunk.Index,
			ChunkTotal:         len(chunks),
		})
		if err != nil {
			pe := perrors.ClassifyError(err, "embed")
			logger.Error("Failed to store embedding",
				logging.Err(pe),
				logging.F("chunk_index", chunk.Index),
			)
			return 0, WrapForTemporal(pe)
		}

		// Record the first chunk's ID for return value
		if chunk.Index == 0 {
			firstEmbeddingID = embeddingID
		}

		logger.Debug("Chunk embedding stored",
			logging.F("chunk_index", chunk.Index),
			logging.F("chunk_total", len(chunks)),
			logging.F("embedding_id", embeddingID),
		)
	}

	logger.Info("All chunk embeddings stored successfully",
		logging.F("total_duration", time.Since(startTime)),
		logging.F("chunk_count", len(chunks)),
		logging.F("first_embedding_id", firstEmbeddingID),
	)

	// Record pipeline run for provenance tracking (Stage 5: embed)
	if a.pipelineRepo != nil {
		durationMS := int(time.Since(startTime).Milliseconds())

		// Capture IO data
		inputJSON, _ := json.Marshal(map[string]interface{}{
			"content_length": len(input.Content),
			"content_hash":   input.ContentHash,
			"tenant_id":      input.TenantID,
			"chunk_count":    len(chunks),
		})
		outputJSON, _ := json.Marshal(map[string]interface{}{
			"chunk_count":        len(chunks),
			"first_embedding_id": firstEmbeddingID,
		})
		parsedJSON, _ := json.Marshal(map[string]interface{}{
			"first_embedding_id": firstEmbeddingID,
			"chunk_count":        len(chunks),
		})

		runErr := a.pipelineRepo.CreateRun(ctx, PipelineRunInput{
			SourceID:   input.SourceID,
			Stage:      "embed",
			ModelID:    "chunked", // Model is recorded per-chunk, overall operation is chunked
			Status:     "completed",
			DurationMS: durationMS,
			InputData:  inputJSON,
			OutputData: outputJSON,
			ParsedData: parsedJSON,
		})
		if runErr != nil {
			logger.Warn("Failed to record pipeline run", logging.Err(runErr))
		}
	}

	return firstEmbeddingID, nil
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
		pe := perrors.ClassifyError(err, "embed")
		logger.Error("Failed to delete embedding", logging.Err(pe))
		return WrapForTemporal(pe)
	}

	logger.Info("Embedding deleted successfully")
	return nil
}

// Ensure EmbeddingActivities implements required interfaces at compile time.
var _ interface {
	GenerateEmbedding(ctx context.Context, input workflows.GenerateEmbeddingInput) (int64, error)
	DeleteEmbedding(ctx context.Context, embeddingID int64) error
} = (*EmbeddingActivities)(nil)
