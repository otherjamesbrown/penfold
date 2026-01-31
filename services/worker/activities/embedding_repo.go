// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// PostgresEmbeddingRepository implements EmbeddingRepository using PostgreSQL.
type PostgresEmbeddingRepository struct {
	pool   *pgxpool.Pool
	logger zerolog.Logger
}

// NewPostgresEmbeddingRepository creates a new PostgreSQL embedding repository.
func NewPostgresEmbeddingRepository(pool *pgxpool.Pool, logger zerolog.Logger) *PostgresEmbeddingRepository {
	return &PostgresEmbeddingRepository{
		pool:   pool,
		logger: logger.With().Str("component", "embedding_repository").Logger(),
	}
}

// StoreEmbedding stores an embedding vector for a source.
// It inserts a new record into the embeddings table and returns the new embedding ID.
// The vector is converted from float32 to float64 for PostgreSQL compatibility.
func (r *PostgresEmbeddingRepository) StoreEmbedding(
	ctx context.Context,
	tenantID string,
	sourceID int64,
	vector []float32,
	model string,
	dimensions int32,
) (int64, error) {
	logger := r.logger.With().
		Str("tenant_id", tenantID).
		Int64("source_id", sourceID).
		Str("model", model).
		Int32("dimensions", dimensions).
		Int("vector_length", len(vector)).
		Logger()

	logger.Debug().Msg("Storing embedding")

	// Validate input
	if tenantID == "" {
		return 0, fmt.Errorf("tenant_id is required")
	}
	if len(vector) == 0 {
		return 0, fmt.Errorf("vector is empty")
	}
	if model == "" {
		return 0, fmt.Errorf("model is required")
	}

	// Convert float32 slice to float64 slice for PostgreSQL double precision[]
	embeddingFloat64 := make([]float64, len(vector))
	for i, v := range vector {
		embeddingFloat64[i] = float64(v)
	}

	// Insert the embedding
	// The embeddings table schema:
	// - tenant_id (uuid)
	// - entity_type (varchar) - e.g., 'source', 'assertion', 'person', 'project'
	// - entity_id (bigint) - ID of the entity being embedded
	// - source_id (bigint) - Reference to source table
	// - embedding_model (varchar) - Model name
	// - model_version (varchar) - Model version
	// - embedding (double precision[]) - Vector data
	// - text_content (text) - Optional text that was embedded
	// - content_hash (varchar) - SHA-256 hash for deduplication
	query := `
		INSERT INTO embeddings (
			tenant_id,
			entity_type,
			entity_id,
			source_id,
			embedding_model,
			model_version,
			embedding,
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
			NOW(),
			NOW()
		)
		RETURNING id
	`

	var embeddingID int64
	err := r.pool.QueryRow(
		ctx,
		query,
		tenantID,
		"source",      // entity_type - embeddings are for sources
		sourceID,      // entity_id
		sourceID,      // source_id
		model,         // embedding_model
		"",            // model_version - empty for now
		embeddingFloat64,
	).Scan(&embeddingID)

	if err != nil {
		logger.Error().Err(err).Msg("Failed to store embedding")
		return 0, fmt.Errorf("failed to store embedding: %w", err)
	}

	logger.Info().
		Int64("embedding_id", embeddingID).
		Msg("Embedding stored successfully")

	return embeddingID, nil
}

// GetEmbedding fetches an embedding by ID.
// It retrieves the embedding record and converts the vector from float64 back to float32.
func (r *PostgresEmbeddingRepository) GetEmbedding(
	ctx context.Context,
	tenantID string,
	embeddingID int64,
) (*Embedding, error) {
	logger := r.logger.With().
		Str("tenant_id", tenantID).
		Int64("embedding_id", embeddingID).
		Logger()

	logger.Debug().Msg("Fetching embedding")

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
			embedding,
			embedding_model,
			COALESCE(array_length(embedding, 1), 0) AS dimensions
		FROM embeddings
		WHERE id = $1 AND tenant_id = $2::uuid
	`

	var (
		id              int64
		sourceID        int64
		tenantIDFromDB  string
		embeddingFloat64 []float64
		model           string
		dimensions      int32
	)

	err := r.pool.QueryRow(ctx, query, embeddingID, tenantID).Scan(
		&id,
		&sourceID,
		&tenantIDFromDB,
		&embeddingFloat64,
		&model,
		&dimensions,
	)

	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch embedding")
		return nil, fmt.Errorf("failed to fetch embedding: %w", err)
	}

	// Convert float64 slice back to float32 slice
	vector := make([]float32, len(embeddingFloat64))
	for i, v := range embeddingFloat64 {
		vector[i] = float32(v)
	}

	logger.Info().
		Int("vector_length", len(vector)).
		Msg("Embedding fetched successfully")

	return &Embedding{
		ID:         id,
		SourceID:   sourceID,
		TenantID:   tenantIDFromDB,
		Vector:     vector,
		Model:      model,
		Dimensions: dimensions,
	}, nil
}

// Ensure PostgresEmbeddingRepository implements the EmbeddingRepository interface at compile time.
var _ EmbeddingRepository = (*PostgresEmbeddingRepository)(nil)
