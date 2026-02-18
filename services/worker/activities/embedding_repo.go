// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PostgresEmbeddingRepository implements EmbeddingRepository using PostgreSQL.
type PostgresEmbeddingRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresEmbeddingRepository creates a new PostgreSQL embedding repository.
func NewPostgresEmbeddingRepository(pool *pgxpool.Pool, logger logging.Logger) *PostgresEmbeddingRepository {
	return &PostgresEmbeddingRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "embedding_repository")),
	}
}

// StoreEmbedding stores an embedding vector for a source.
// This is a backward-compatible wrapper around StoreMultiLevelEmbedding.
func (r *PostgresEmbeddingRepository) StoreEmbedding(
	ctx context.Context,
	tenantID string,
	sourceID int64,
	vector []float32,
	model string,
	dimensions int32,
) (int64, error) {
	// Delegate to StoreMultiLevelEmbedding with default values
	return r.StoreMultiLevelEmbedding(ctx, &MultiLevelEmbeddingInput{
		TenantID:           tenantID,
		SourceID:           sourceID,
		EntityType:         "source",
		EntityID:           sourceID,
		RepresentationType: "content",
		Vector:             vector,
		Model:              model,
		ModelVersion:       "",
		TextContent:        "",
		ContentHash:        "",
	})
}

// StoreMultiLevelEmbedding stores an embedding with entity type and representation type.
// It inserts a new record into the embeddings table and returns the new embedding ID.
// The vector is converted from float32 to float64 for PostgreSQL compatibility.
func (r *PostgresEmbeddingRepository) StoreMultiLevelEmbedding(
	ctx context.Context,
	input *MultiLevelEmbeddingInput,
) (int64, error) {
	logger := r.logger.With(
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("entity_type", input.EntityType),
		logging.F("entity_id", input.EntityID),
		logging.F("representation_type", input.RepresentationType),
		logging.F("model", input.Model),
		logging.F("model_version", input.ModelVersion),
		logging.F("vector_length", len(input.Vector)),
	)

	logger.Debug("Storing multi-level embedding")

	// Validate input
	if input.TenantID == "" {
		return 0, fmt.Errorf("tenant_id is required")
	}
	if input.SourceID <= 0 {
		return 0, fmt.Errorf("source_id is required")
	}
	if input.EntityType == "" {
		return 0, fmt.Errorf("entity_type is required")
	}
	if input.EntityID <= 0 {
		return 0, fmt.Errorf("entity_id is required")
	}
	if input.RepresentationType == "" {
		return 0, fmt.Errorf("representation_type is required")
	}
	if len(input.Vector) == 0 {
		return 0, fmt.Errorf("vector is empty")
	}
	if input.Model == "" {
		return 0, fmt.Errorf("model is required")
	}

	// Convert float32 slice to float64 slice for PostgreSQL double precision[]
	embeddingFloat64 := make([]float64, len(input.Vector))
	for i, v := range input.Vector {
		embeddingFloat64[i] = float64(v)
	}

	// Insert the embedding
	query := `
		INSERT INTO embeddings (
			tenant_id,
			entity_type,
			entity_id,
			source_id,
			embedding_model,
			model_version,
			representation_type,
			embedding,
			text_content,
			content_hash,
			chunk_index,
			chunk_total,
			created_at,
			updated_at
		) VALUES (
			$1::uuid,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10,
			$11,
			$12,
			NOW(),
			NOW()
		)
		RETURNING id
	`

	var embeddingID int64
	err := r.pool.QueryRow(
		ctx,
		query,
		input.TenantID,
		input.EntityType,
		input.EntityID,
		input.SourceID,
		input.Model,
		input.ModelVersion,
		input.RepresentationType,
		embeddingFloat64,
		input.TextContent,
		input.ContentHash,
		input.ChunkIndex,
		input.ChunkTotal,
	).Scan(&embeddingID)

	if err != nil {
		logger.Error("Failed to store multi-level embedding", logging.Err(err))
		return 0, fmt.Errorf("failed to store multi-level embedding: %w", err)
	}

	logger.Info("Multi-level embedding stored successfully",
		logging.F("embedding_id", embeddingID),
	)

	return embeddingID, nil
}

// GetEmbedding fetches an embedding by ID.
// It retrieves the embedding record and converts the vector from float64 back to float32.
func (r *PostgresEmbeddingRepository) GetEmbedding(
	ctx context.Context,
	tenantID string,
	embeddingID int64,
) (*Embedding, error) {
	logger := r.logger.With(
		logging.F("tenant_id", tenantID),
		logging.F("embedding_id", embeddingID),
	)

	logger.Debug("Fetching embedding")

	// Validate input
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if embeddingID <= 0 {
		return nil, fmt.Errorf("invalid embedding_id: %d", embeddingID)
	}

	// Query the embedding
	query := `
		SELECT
			id,
			source_id,
			tenant_id,
			entity_type,
			entity_id,
			COALESCE(representation_type, 'content') AS representation_type,
			embedding,
			embedding_model,
			COALESCE(model_version, '') AS model_version,
			COALESCE(array_length(embedding, 1), 0) AS dimensions
		FROM embeddings
		WHERE id = $1 AND tenant_id = $2::uuid
	`

	var (
		id                 int64
		sourceID           int64
		tenantIDFromDB     string
		entityType         string
		entityID           int64
		representationType string
		embeddingFloat64   []float64
		model              string
		modelVersion       string
		dimensions         int32
	)

	err := r.pool.QueryRow(ctx, query, embeddingID, tenantID).Scan(
		&id,
		&sourceID,
		&tenantIDFromDB,
		&entityType,
		&entityID,
		&representationType,
		&embeddingFloat64,
		&model,
		&modelVersion,
		&dimensions,
	)

	if err != nil {
		logger.Error("Failed to fetch embedding", logging.Err(err))
		return nil, fmt.Errorf("failed to fetch embedding: %w", err)
	}

	// Convert float64 slice back to float32 slice
	vector := make([]float32, len(embeddingFloat64))
	for i, v := range embeddingFloat64 {
		vector[i] = float32(v)
	}

	logger.Info("Embedding fetched successfully",
		logging.F("vector_length", len(vector)),
	)

	return &Embedding{
		ID:                 id,
		SourceID:           sourceID,
		TenantID:           tenantIDFromDB,
		EntityType:         entityType,
		EntityID:           entityID,
		RepresentationType: representationType,
		Vector:             vector,
		Model:              model,
		ModelVersion:       modelVersion,
		Dimensions:         dimensions,
	}, nil
}

// GetEmbeddingsForSource returns all embeddings for a source, optionally filtered by representation type.
// Pass empty string for representationType to get all embeddings.
func (r *PostgresEmbeddingRepository) GetEmbeddingsForSource(
	ctx context.Context,
	tenantID string,
	sourceID int64,
	representationType string,
) ([]*Embedding, error) {
	logger := r.logger.With(
		logging.F("tenant_id", tenantID),
		logging.F("source_id", sourceID),
		logging.F("representation_type", representationType),
	)

	logger.Debug("Fetching embeddings for source")

	// Validate input
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if sourceID <= 0 {
		return nil, fmt.Errorf("invalid source_id: %d", sourceID)
	}

	// Build query with optional representation_type filter
	query := `
		SELECT
			id,
			source_id,
			tenant_id,
			entity_type,
			entity_id,
			COALESCE(representation_type, 'content') AS representation_type,
			embedding,
			embedding_model,
			COALESCE(model_version, '') AS model_version,
			COALESCE(array_length(embedding, 1), 0) AS dimensions
		FROM embeddings
		WHERE tenant_id = $1::uuid AND source_id = $2
	`

	args := []interface{}{tenantID, sourceID}
	if representationType != "" {
		query += " AND representation_type = $3"
		args = append(args, representationType)
	}

	query += " ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Error("Failed to fetch embeddings for source", logging.Err(err))
		return nil, fmt.Errorf("failed to fetch embeddings for source: %w", err)
	}
	defer rows.Close()

	var embeddings []*Embedding
	for rows.Next() {
		var (
			id                 int64
			sourceID           int64
			tenantIDFromDB     string
			entityType         string
			entityID           int64
			representationType string
			embeddingFloat64   []float64
			model              string
			modelVersion       string
			dimensions         int32
		)

		err := rows.Scan(
			&id,
			&sourceID,
			&tenantIDFromDB,
			&entityType,
			&entityID,
			&representationType,
			&embeddingFloat64,
			&model,
			&modelVersion,
			&dimensions,
		)
		if err != nil {
			logger.Error("Failed to scan embedding row", logging.Err(err))
			return nil, fmt.Errorf("failed to scan embedding row: %w", err)
		}

		// Convert float64 slice back to float32 slice
		vector := make([]float32, len(embeddingFloat64))
		for i, v := range embeddingFloat64 {
			vector[i] = float32(v)
		}

		embeddings = append(embeddings, &Embedding{
			ID:                 id,
			SourceID:           sourceID,
			TenantID:           tenantIDFromDB,
			EntityType:         entityType,
			EntityID:           entityID,
			RepresentationType: representationType,
			Vector:             vector,
			Model:              model,
			ModelVersion:       modelVersion,
			Dimensions:         dimensions,
		})
	}

	if err := rows.Err(); err != nil {
		logger.Error("Error iterating embedding rows", logging.Err(err))
		return nil, fmt.Errorf("error iterating embedding rows: %w", err)
	}

	logger.Info("Embeddings fetched successfully",
		logging.F("count", len(embeddings)),
	)

	return embeddings, nil
}

// GetStaleEmbeddings returns source IDs with embeddings from an older model/version.
// This is used to find sources that need re-embedding when the model is updated.
func (r *PostgresEmbeddingRepository) GetStaleEmbeddings(
	ctx context.Context,
	tenantID string,
	currentModel string,
	currentVersion string,
	limit int,
) ([]int64, error) {
	logger := r.logger.With(
		logging.F("tenant_id", tenantID),
		logging.F("current_model", currentModel),
		logging.F("current_version", currentVersion),
		logging.F("limit", limit),
	)

	logger.Debug("Fetching stale embeddings")

	// Validate input
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if currentModel == "" {
		return nil, fmt.Errorf("current_model is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}

	// Query for source IDs with embeddings not matching current model/version
	query := `
		SELECT DISTINCT source_id
		FROM embeddings
		WHERE tenant_id = $1::uuid
		  AND (embedding_model != $2 OR COALESCE(model_version, '') != $3)
		ORDER BY source_id
		LIMIT $4
	`

	rows, err := r.pool.Query(ctx, query, tenantID, currentModel, currentVersion, limit)
	if err != nil {
		logger.Error("Failed to fetch stale embeddings", logging.Err(err))
		return nil, fmt.Errorf("failed to fetch stale embeddings: %w", err)
	}
	defer rows.Close()

	var sourceIDs []int64
	for rows.Next() {
		var sourceID int64
		if err := rows.Scan(&sourceID); err != nil {
			logger.Error("Failed to scan source ID", logging.Err(err))
			return nil, fmt.Errorf("failed to scan source ID: %w", err)
		}
		sourceIDs = append(sourceIDs, sourceID)
	}

	if err := rows.Err(); err != nil {
		logger.Error("Error iterating stale embedding rows", logging.Err(err))
		return nil, fmt.Errorf("error iterating stale embedding rows: %w", err)
	}

	logger.Info("Stale embeddings fetched successfully",
		logging.F("count", len(sourceIDs)),
	)

	return sourceIDs, nil
}

// DeleteEmbeddingsForSource deletes all embeddings for a source.
// This is typically called before re-embedding a source with updated content or model.
func (r *PostgresEmbeddingRepository) DeleteEmbeddingsForSource(
	ctx context.Context,
	tenantID string,
	sourceID int64,
) error {
	logger := r.logger.With(
		logging.F("tenant_id", tenantID),
		logging.F("source_id", sourceID),
	)

	logger.Debug("Deleting embeddings for source")

	// Validate input
	if tenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if sourceID <= 0 {
		return fmt.Errorf("invalid source_id: %d", sourceID)
	}

	// Delete all embeddings for the source
	query := `
		DELETE FROM embeddings
		WHERE tenant_id = $1::uuid AND source_id = $2
	`

	result, err := r.pool.Exec(ctx, query, tenantID, sourceID)
	if err != nil {
		logger.Error("Failed to delete embeddings for source", logging.Err(err))
		return fmt.Errorf("failed to delete embeddings for source: %w", err)
	}

	rowsAffected := result.RowsAffected()
	logger.Info("Embeddings deleted successfully",
		logging.F("rows_affected", rowsAffected),
	)

	return nil
}

// DeleteEmbedding deletes a single embedding by ID.
// Used for saga compensation when downstream pipeline stages fail.
func (r *PostgresEmbeddingRepository) DeleteEmbedding(
	ctx context.Context,
	embeddingID int64,
) error {
	logger := r.logger.With(
		logging.F("embedding_id", embeddingID),
	)

	logger.Debug("Deleting embedding by ID")

	if embeddingID <= 0 {
		return fmt.Errorf("invalid embedding_id: %d", embeddingID)
	}

	query := `DELETE FROM embeddings WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, embeddingID)
	if err != nil {
		logger.Error("Failed to delete embedding", logging.Err(err))
		return fmt.Errorf("failed to delete embedding %d: %w", embeddingID, err)
	}

	rowsAffected := result.RowsAffected()
	logger.Info("Embedding deleted",
		logging.F("rows_affected", rowsAffected),
	)

	return nil
}

// Ensure PostgresEmbeddingRepository implements the EmbeddingRepository interface at compile time.
var _ EmbeddingRepository = (*PostgresEmbeddingRepository)(nil)
