// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/temporal"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/chunking"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// MultiLevelEmbeddingActivities holds dependencies for multi-level embedding activities.
type MultiLevelEmbeddingActivities struct {
	logger        logging.Logger
	aiClient      AIClient
	embeddingRepo EmbeddingRepository
	sourceRepo    SourceRepository
}

// NewMultiLevelEmbeddingActivities creates a new MultiLevelEmbeddingActivities instance.
func NewMultiLevelEmbeddingActivities(
	logger logging.Logger,
	aiClient AIClient,
	embeddingRepo EmbeddingRepository,
	sourceRepo SourceRepository,
) *MultiLevelEmbeddingActivities {
	return &MultiLevelEmbeddingActivities{
		logger:        logger.With(logging.F("component", "multilevel_embedding_activities")),
		aiClient:      aiClient,
		embeddingRepo: embeddingRepo,
		sourceRepo:    sourceRepo,
	}
}

// GenerateMultiLevelInput is the input for the GenerateMultiLevelEmbeddings activity.
type GenerateMultiLevelInput struct {
	TenantID    string              `json:"tenant_id"`
	SourceID    int64               `json:"source_id"`
	ContentID   string              `json:"content_id,omitempty"`
	Content     string              `json:"content"`      // Raw content text
	ContentType string              `json:"content_type"` // email, meeting, slack
	Analysis    *DeepAnalyzeOutput  `json:"analysis"`     // Stage 4 output
	// Assertion IDs from Stage 4.5 persist, mapped by type+index
	// e.g., "action:0" -> 42, "decision:1" -> 55
	AssertionIDs map[string]int64 `json:"assertion_ids,omitempty"`
}

// MultiLevelEmbeddingOutput is the output from Stage 5 multi-level embedding.
type MultiLevelEmbeddingOutput struct {
	SourceEmbeddingIDs    []int64 `json:"source_embedding_ids"`    // content + summary
	AssertionEmbeddingIDs []int64 `json:"assertion_embedding_ids"` // actions + decisions + risks
	TotalGenerated        int     `json:"total_generated"`
	TotalFailed           int     `json:"total_failed"`
	ModelUsed             string  `json:"model_used"`
	ModelVersion          string  `json:"model_version"`
}

// ReEmbedBatchInput is the input for batch re-embedding.
type ReEmbedBatchInput struct {
	TenantID       string `json:"tenant_id"`
	CurrentModel   string `json:"current_model"`
	CurrentVersion string `json:"current_version"`
	BatchSize      int    `json:"batch_size"`
}

// ReEmbedBatchOutput is the output from batch re-embedding.
type ReEmbedBatchOutput struct {
	SourcesProcessed int `json:"sources_processed"`
	SourcesRemaining int `json:"sources_remaining"` // For continue-as-new
	Errors           int `json:"errors"`
}

// GenerateMultiLevelEmbeddings generates multiple embeddings per source:
// raw content, summary, action items, decisions, and risks.
func (a *MultiLevelEmbeddingActivities) GenerateMultiLevelEmbeddings(
	ctx context.Context,
	input GenerateMultiLevelInput,
) (*MultiLevelEmbeddingOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "GenerateMultiLevelEmbeddings"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("content_length", len(input.Content)),
		logging.F("content_type", input.ContentType),
	)

	// Record initial heartbeat
	recordHeartbeat(ctx, "starting multi-level embedding")

	logger.Info("Starting multi-level embedding generation")

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

	if input.Analysis == nil {
		return nil, temporal.NewApplicationError(
			"analysis is required",
			"ValidationError",
		)
	}

	// Check if AI client is available
	if a.aiClient == nil {
		logger.Warn("AI client not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"AI client not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Check if embedding repository is available
	if a.embeddingRepo == nil {
		logger.Warn("Embedding repository not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"embedding repository not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Delete existing embeddings for this source (idempotent re-processing)
	recordHeartbeat(ctx, "deleting existing embeddings")
	if err := a.embeddingRepo.DeleteEmbeddingsForSource(ctx, input.TenantID, input.SourceID); err != nil {
		logger.Error("Failed to delete existing embeddings", logging.Err(err))
		return nil, fmt.Errorf("failed to delete existing embeddings: %w", err)
	}

	// Initialize output
	output := &MultiLevelEmbeddingOutput{
		SourceEmbeddingIDs:    []int64{},
		AssertionEmbeddingIDs: []int64{},
	}

	embeddingCount := 0
	totalEmbeddings := 0

	// Chunk content for embedding
	chunks := chunking.ChunkContent(input.Content, chunking.ChunkOptions{
		MaxTokens:     400,
		OverlapTokens: 50,
	})
	if len(chunks) == 0 {
		chunks = []chunking.Chunk{{Index: 0, Text: input.Content, Start: 0, End: len(input.Content)}}
	}

	// Count total embeddings we'll generate
	totalEmbeddings += len(chunks) // content chunks
	if input.Analysis.Summary != "" {
		totalEmbeddings++ // summary
	}
	for _, action := range input.Analysis.VerifiedActions {
		if action.Status != "removed" {
			totalEmbeddings++
		}
	}
	for _, decision := range input.Analysis.VerifiedDecisions {
		if decision.Status != "removed" {
			totalEmbeddings++
		}
	}
	totalEmbeddings += len(input.Analysis.RiskReferences)

	// 1. Generate content embeddings (one per chunk)
	for _, chunk := range chunks {
		embeddingCount++
		recordHeartbeat(ctx, fmt.Sprintf("embedding %d/%d: content chunk %d/%d", embeddingCount, totalEmbeddings, chunk.Index+1, len(chunks)))

		contentID, err := a.generateAndStoreEmbedding(ctx, input.TenantID, input.SourceID, "source", input.SourceID, "content", chunk.Text, chunk.Index, len(chunks))
		if err != nil {
			logger.Error("Failed to generate content embedding chunk", logging.Err(err), logging.F("chunk_index", chunk.Index))
			return nil, fmt.Errorf("failed to generate content embedding chunk %d: %w", chunk.Index, err)
		}
		output.SourceEmbeddingIDs = append(output.SourceEmbeddingIDs, contentID)
		output.TotalGenerated++
	}

	// Capture model info from first successful response
	if output.ModelUsed == "" && len(output.SourceEmbeddingIDs) > 0 {
		// Get model info from the first embedding we just stored
		if emb, err := a.embeddingRepo.GetEmbedding(ctx, input.TenantID, output.SourceEmbeddingIDs[0]); err == nil && emb != nil {
			output.ModelUsed = emb.Model
			output.ModelVersion = emb.ModelVersion
		}
	}

	// 2. Generate summary embedding (if summary exists)
	if input.Analysis.Summary != "" {
		embeddingCount++
		recordHeartbeat(ctx, fmt.Sprintf("embedding %d/%d: summary", embeddingCount, totalEmbeddings))

		summaryID, err := a.generateAndStoreEmbedding(ctx, input.TenantID, input.SourceID, "source", input.SourceID, "summary", input.Analysis.Summary, 0, 1)
		if err != nil {
			logger.Error("Failed to generate summary embedding", logging.Err(err))
			return nil, fmt.Errorf("failed to generate summary embedding: %w", err)
		}
		output.SourceEmbeddingIDs = append(output.SourceEmbeddingIDs, summaryID)
		output.TotalGenerated++
	}

	// 3. Generate action item embeddings
	for i, action := range input.Analysis.VerifiedActions {
		if action.Status == "removed" {
			continue // Skip removed actions
		}

		embeddingCount++
		recordHeartbeat(ctx, fmt.Sprintf("embedding %d/%d: action %d", embeddingCount, totalEmbeddings, i))

		// Get assertion ID from map, fallback to source ID
		assertionKey := fmt.Sprintf("action:%d", i)
		entityID := input.SourceID
		entityType := "source"
		if assertionID, ok := input.AssertionIDs[assertionKey]; ok {
			entityID = assertionID
			entityType = "assertion"
		}

		actionID, err := a.generateAndStoreEmbedding(ctx, input.TenantID, input.SourceID, entityType, entityID, "action_item", action.Description, 0, 1)
		if err != nil {
			logger.Warn("Failed to generate action item embedding",
				logging.F("action_index", i),
				logging.Err(err),
			)
			output.TotalFailed++
			continue // Non-fatal for assertion embeddings
		}
		output.AssertionEmbeddingIDs = append(output.AssertionEmbeddingIDs, actionID)
		output.TotalGenerated++
	}

	// 4. Generate decision embeddings
	for i, decision := range input.Analysis.VerifiedDecisions {
		if decision.Status == "removed" {
			continue // Skip removed decisions
		}

		embeddingCount++
		recordHeartbeat(ctx, fmt.Sprintf("embedding %d/%d: decision %d", embeddingCount, totalEmbeddings, i))

		// Get assertion ID from map, fallback to source ID
		assertionKey := fmt.Sprintf("decision:%d", i)
		entityID := input.SourceID
		entityType := "source"
		if assertionID, ok := input.AssertionIDs[assertionKey]; ok {
			entityID = assertionID
			entityType = "assertion"
		}

		decisionID, err := a.generateAndStoreEmbedding(ctx, input.TenantID, input.SourceID, entityType, entityID, "decision", decision.Description, 0, 1)
		if err != nil {
			logger.Warn("Failed to generate decision embedding",
				logging.F("decision_index", i),
				logging.Err(err),
			)
			output.TotalFailed++
			continue // Non-fatal for assertion embeddings
		}
		output.AssertionEmbeddingIDs = append(output.AssertionEmbeddingIDs, decisionID)
		output.TotalGenerated++
	}

	// 5. Generate risk embeddings
	for i, risk := range input.Analysis.RiskReferences {
		embeddingCount++
		recordHeartbeat(ctx, fmt.Sprintf("embedding %d/%d: risk %d", embeddingCount, totalEmbeddings, i))

		// Get assertion ID from map, fallback to source ID
		assertionKey := fmt.Sprintf("risk:%d", i)
		entityID := input.SourceID
		entityType := "source"
		if assertionID, ok := input.AssertionIDs[assertionKey]; ok {
			entityID = assertionID
			entityType = "assertion"
		}

		riskID, err := a.generateAndStoreEmbedding(ctx, input.TenantID, input.SourceID, entityType, entityID, "risk", risk.Description, 0, 1)
		if err != nil {
			logger.Warn("Failed to generate risk embedding",
				logging.F("risk_index", i),
				logging.Err(err),
			)
			output.TotalFailed++
			continue // Non-fatal for assertion embeddings
		}
		output.AssertionEmbeddingIDs = append(output.AssertionEmbeddingIDs, riskID)
		output.TotalGenerated++
	}

	// Record completion heartbeat
	recordHeartbeat(ctx, "multi-level embedding complete")

	logger.Info("Multi-level embedding generation completed",
		logging.F("total_generated", output.TotalGenerated),
		logging.F("total_failed", output.TotalFailed),
		logging.F("source_embeddings", len(output.SourceEmbeddingIDs)),
		logging.F("assertion_embeddings", len(output.AssertionEmbeddingIDs)),
		logging.F("model", output.ModelUsed),
	)

	return output, nil
}

// generateAndStoreEmbedding is a helper that generates an embedding and stores it.
func (a *MultiLevelEmbeddingActivities) generateAndStoreEmbedding(
	ctx context.Context,
	tenantID string,
	sourceID int64,
	entityType string,
	entityID int64,
	representationType string,
	text string,
	chunkIndex int,
	chunkTotal int,
) (int64, error) {
	// Call AI service to generate embedding
	resp, err := a.aiClient.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
		Text:     text,
		TenantId: &tenantID,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// Store embedding
	id, err := a.embeddingRepo.StoreMultiLevelEmbedding(ctx, &MultiLevelEmbeddingInput{
		TenantID:           tenantID,
		SourceID:           sourceID,
		EntityType:         entityType,
		EntityID:           entityID,
		RepresentationType: representationType,
		Vector:             resp.Vector,
		Model:              resp.ModelUsed,
		ModelVersion:       resp.ModelUsed, // Use model name as version for now
		TextContent:        text,
		ChunkIndex:         chunkIndex,
		ChunkTotal:         chunkTotal,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to store embedding: %w", err)
	}

	return id, nil
}

// ReEmbedBatch re-embeds a batch of sources with stale embeddings.
func (a *MultiLevelEmbeddingActivities) ReEmbedBatch(
	ctx context.Context,
	input ReEmbedBatchInput,
) (*ReEmbedBatchOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ReEmbedBatch"),
		logging.F("tenant_id", input.TenantID),
		logging.F("current_model", input.CurrentModel),
		logging.F("batch_size", input.BatchSize),
	)

	// Record initial heartbeat
	recordHeartbeat(ctx, "starting re-embed batch")

	logger.Info("Starting re-embed batch")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, temporal.NewApplicationError(
			"tenant_id is required",
			"ValidationError",
		)
	}

	if input.CurrentModel == "" {
		return nil, temporal.NewApplicationError(
			"current_model is required",
			"ValidationError",
		)
	}

	if input.BatchSize <= 0 {
		return nil, temporal.NewApplicationError(
			"batch_size must be positive",
			"ValidationError",
		)
	}

	// Check if repositories are available
	if a.embeddingRepo == nil {
		logger.Warn("Embedding repository not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"embedding repository not configured",
			"ConfigurationError",
			nil,
		)
	}

	if a.sourceRepo == nil {
		logger.Warn("Source repository not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"source repository not configured",
			"ConfigurationError",
			nil,
		)
	}

	if a.aiClient == nil {
		logger.Warn("AI client not configured")
		return nil, temporal.NewApplicationErrorWithCause(
			"AI client not configured",
			"ConfigurationError",
			nil,
		)
	}

	// Get stale embeddings (fetch one extra to know if more remain)
	staleSourceIDs, err := a.embeddingRepo.GetStaleEmbeddings(
		ctx,
		input.TenantID,
		input.CurrentModel,
		input.CurrentVersion,
		input.BatchSize+1,
	)
	if err != nil {
		logger.Error("Failed to get stale embeddings", logging.Err(err))
		return nil, fmt.Errorf("failed to get stale embeddings: %w", err)
	}

	// Determine if more sources remain
	sourcesRemaining := 0
	if len(staleSourceIDs) > input.BatchSize {
		sourcesRemaining = len(staleSourceIDs) - input.BatchSize
		staleSourceIDs = staleSourceIDs[:input.BatchSize]
	}

	output := &ReEmbedBatchOutput{
		SourcesProcessed: 0,
		SourcesRemaining: sourcesRemaining,
		Errors:           0,
	}

	// Process each stale source
	for i, sourceID := range staleSourceIDs {
		recordHeartbeat(ctx, fmt.Sprintf("re-embedding source %d/%d", i+1, len(staleSourceIDs)))

		// Fetch source
		source, err := a.sourceRepo.GetSource(ctx, input.TenantID, sourceID)
		if err != nil {
			logger.Warn("Failed to fetch source for re-embedding",
				logging.F("source_id", sourceID),
				logging.Err(err),
			)
			output.Errors++
			continue
		}

		// Delete old embeddings
		if err := a.embeddingRepo.DeleteEmbeddingsForSource(ctx, input.TenantID, sourceID); err != nil {
			logger.Warn("Failed to delete old embeddings",
				logging.F("source_id", sourceID),
				logging.Err(err),
			)
			output.Errors++
			continue
		}

		// Chunk content for embedding
		chunks := chunking.ChunkContent(source.ContentText, chunking.ChunkOptions{
			MaxTokens:     400,
			OverlapTokens: 50,
		})
		if len(chunks) == 0 {
			chunks = []chunking.Chunk{{Index: 0, Text: source.ContentText, Start: 0, End: len(source.ContentText)}}
		}

		// Generate embeddings for each chunk
		chunkFailed := false
		for _, chunk := range chunks {
			_, err = a.generateAndStoreEmbedding(ctx, input.TenantID, sourceID, "source", sourceID, "content", chunk.Text, chunk.Index, len(chunks))
			if err != nil {
				logger.Warn("Failed to generate new content embedding chunk",
					logging.F("source_id", sourceID),
					logging.F("chunk_index", chunk.Index),
					logging.Err(err),
				)
				chunkFailed = true
				break
			}
		}

		if chunkFailed {
			output.Errors++
			continue
		}

		output.SourcesProcessed++
	}

	// Record completion heartbeat
	recordHeartbeat(ctx, "re-embed batch complete")

	logger.Info("Re-embed batch completed",
		logging.F("sources_processed", output.SourcesProcessed),
		logging.F("sources_remaining", output.SourcesRemaining),
		logging.F("errors", output.Errors),
	)

	return output, nil
}

// Ensure MultiLevelEmbeddingActivities implements required interfaces at compile time.
var _ interface {
	GenerateMultiLevelEmbeddings(ctx context.Context, input GenerateMultiLevelInput) (*MultiLevelEmbeddingOutput, error)
	ReEmbedBatch(ctx context.Context, input ReEmbedBatchInput) (*ReEmbedBatchOutput, error)
} = (*MultiLevelEmbeddingActivities)(nil)
