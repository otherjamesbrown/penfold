package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// Embedding represents a vector embedding stored in the database.
type Embedding struct {
	ID                   int64
	TenantID             string
	EntityType           string    // source, assertion, person, project, team, etc.
	EntityID             int64
	SourceID             *int64    // nullable FK to sources
	Embedding            []float64 // PostgreSQL float8[] array
	EmbeddingModel       string
	ModelVersion         *string
	TextContent          *string
	ContentHash          *string
	GenerationConfidence *float64
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// EmbeddingRepository provides methods for storing and retrieving embeddings.
type EmbeddingRepository struct {
	pool   *pgxpool.Pool
	logger zerolog.Logger
}

// NewEmbeddingRepository creates a new embedding repository.
func NewEmbeddingRepository(pool *pgxpool.Pool, logger zerolog.Logger) *EmbeddingRepository {
	return &EmbeddingRepository{
		pool:   pool,
		logger: logger.With().Str("component", "embedding_repository").Logger(),
	}
}

// Store inserts a new embedding in the database.
func (r *EmbeddingRepository) Store(ctx context.Context, e *Embedding) error {
	query := `
		INSERT INTO embeddings (
			tenant_id, entity_type, entity_id, source_id,
			embedding, embedding_model, model_version,
			text_content, content_hash, generation_confidence,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10,
			NOW(), NOW()
		)
		RETURNING id, created_at, updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		e.TenantID,
		e.EntityType,
		e.EntityID,
		e.SourceID,
		e.Embedding,
		e.EmbeddingModel,
		e.ModelVersion,
		e.TextContent,
		e.ContentHash,
		e.GenerationConfidence,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)

	if err != nil {
		r.logger.Error().
			Err(err).
			Str("tenant_id", e.TenantID).
			Str("entity_type", e.EntityType).
			Int64("entity_id", e.EntityID).
			Msg("Failed to store embedding")
		return fmt.Errorf("failed to store embedding: %w", err)
	}

	r.logger.Debug().
		Int64("id", e.ID).
		Str("entity_type", e.EntityType).
		Int64("entity_id", e.EntityID).
		Msg("Embedding stored successfully")

	return nil
}

// StoreBatch inserts multiple embeddings in a single transaction.
func (r *EmbeddingRepository) StoreBatch(ctx context.Context, embeddings []*Embedding) error {
	if len(embeddings) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, e := range embeddings {
		query := `
			INSERT INTO embeddings (
				tenant_id, entity_type, entity_id, source_id,
				embedding, embedding_model, model_version,
				text_content, content_hash, generation_confidence,
				created_at, updated_at
			) VALUES (
				$1, $2, $3, $4,
				$5, $6, $7,
				$8, $9, $10,
				NOW(), NOW()
			)
			RETURNING id, created_at, updated_at
		`

		err := tx.QueryRow(ctx, query,
			e.TenantID,
			e.EntityType,
			e.EntityID,
			e.SourceID,
			e.Embedding,
			e.EmbeddingModel,
			e.ModelVersion,
			e.TextContent,
			e.ContentHash,
			e.GenerationConfidence,
		).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)

		if err != nil {
			return fmt.Errorf("failed to store embedding in batch: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Debug().
		Int("count", len(embeddings)).
		Msg("Batch embeddings stored successfully")

	return nil
}

// GetByEntityID retrieves embeddings for a specific entity.
func (r *EmbeddingRepository) GetByEntityID(ctx context.Context, tenantID string, entityType string, entityID int64) ([]*Embedding, error) {
	query := `
		SELECT
			id, tenant_id, entity_type, entity_id, source_id,
			embedding, embedding_model, model_version,
			text_content, content_hash, generation_confidence,
			created_at, updated_at
		FROM embeddings
		WHERE tenant_id = $1 AND entity_type = $2 AND entity_id = $3
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, tenantID, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query embeddings: %w", err)
	}
	defer rows.Close()

	return r.scanEmbeddings(rows)
}

// GetByContentHash retrieves an embedding by its content hash.
func (r *EmbeddingRepository) GetByContentHash(ctx context.Context, tenantID string, contentHash string) (*Embedding, error) {
	query := `
		SELECT
			id, tenant_id, entity_type, entity_id, source_id,
			embedding, embedding_model, model_version,
			text_content, content_hash, generation_confidence,
			created_at, updated_at
		FROM embeddings
		WHERE tenant_id = $1 AND content_hash = $2
		LIMIT 1
	`

	e := &Embedding{}
	err := r.pool.QueryRow(ctx, query, tenantID, contentHash).Scan(
		&e.ID,
		&e.TenantID,
		&e.EntityType,
		&e.EntityID,
		&e.SourceID,
		&e.Embedding,
		&e.EmbeddingModel,
		&e.ModelVersion,
		&e.TextContent,
		&e.ContentHash,
		&e.GenerationConfidence,
		&e.CreatedAt,
		&e.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding by content hash: %w", err)
	}

	return e, nil
}

// SearchResult represents a search result with similarity score.
type SearchResult struct {
	Embedding  *Embedding
	Similarity float64
}

// Search performs a vector similarity search using cosine distance.
func (r *EmbeddingRepository) Search(ctx context.Context, tenantID string, queryVector []float64, entityType *string, limit int) ([]*SearchResult, error) {
	var query string
	var args []interface{}

	if entityType != nil {
		query = `
			SELECT
				id, tenant_id, entity_type, entity_id, source_id,
				embedding, embedding_model, model_version,
				text_content, content_hash, generation_confidence,
				created_at, updated_at,
				1 - (embedding <=> $1) AS similarity
			FROM embeddings
			WHERE tenant_id = $2 AND entity_type = $3
			ORDER BY embedding <=> $1
			LIMIT $4
		`
		args = []interface{}{queryVector, tenantID, *entityType, limit}
	} else {
		query = `
			SELECT
				id, tenant_id, entity_type, entity_id, source_id,
				embedding, embedding_model, model_version,
				text_content, content_hash, generation_confidence,
				created_at, updated_at,
				1 - (embedding <=> $1) AS similarity
			FROM embeddings
			WHERE tenant_id = $2
			ORDER BY embedding <=> $1
			LIMIT $3
		`
		args = []interface{}{queryVector, tenantID, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search embeddings: %w", err)
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		e := &Embedding{}
		var similarity float64
		err := rows.Scan(
			&e.ID,
			&e.TenantID,
			&e.EntityType,
			&e.EntityID,
			&e.SourceID,
			&e.Embedding,
			&e.EmbeddingModel,
			&e.ModelVersion,
			&e.TextContent,
			&e.ContentHash,
			&e.GenerationConfidence,
			&e.CreatedAt,
			&e.UpdatedAt,
			&similarity,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		results = append(results, &SearchResult{
			Embedding:  e,
			Similarity: similarity,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	r.logger.Debug().
		Int("results", len(results)).
		Str("tenant_id", tenantID).
		Msg("Vector search completed")

	return results, nil
}

// SearchWithThreshold performs a similarity search with a minimum threshold.
func (r *EmbeddingRepository) SearchWithThreshold(ctx context.Context, tenantID string, queryVector []float64, entityType *string, threshold float64, limit int) ([]*SearchResult, error) {
	var query string
	var args []interface{}

	if entityType != nil {
		query = `
			SELECT
				id, tenant_id, entity_type, entity_id, source_id,
				embedding, embedding_model, model_version,
				text_content, content_hash, generation_confidence,
				created_at, updated_at,
				1 - (embedding <=> $1) AS similarity
			FROM embeddings
			WHERE tenant_id = $2 AND entity_type = $3 AND 1 - (embedding <=> $1) >= $4
			ORDER BY embedding <=> $1
			LIMIT $5
		`
		args = []interface{}{queryVector, tenantID, *entityType, threshold, limit}
	} else {
		query = `
			SELECT
				id, tenant_id, entity_type, entity_id, source_id,
				embedding, embedding_model, model_version,
				text_content, content_hash, generation_confidence,
				created_at, updated_at,
				1 - (embedding <=> $1) AS similarity
			FROM embeddings
			WHERE tenant_id = $2 AND 1 - (embedding <=> $1) >= $3
			ORDER BY embedding <=> $1
			LIMIT $4
		`
		args = []interface{}{queryVector, tenantID, threshold, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search embeddings with threshold: %w", err)
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		e := &Embedding{}
		var similarity float64
		err := rows.Scan(
			&e.ID,
			&e.TenantID,
			&e.EntityType,
			&e.EntityID,
			&e.SourceID,
			&e.Embedding,
			&e.EmbeddingModel,
			&e.ModelVersion,
			&e.TextContent,
			&e.ContentHash,
			&e.GenerationConfidence,
			&e.CreatedAt,
			&e.UpdatedAt,
			&similarity,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		results = append(results, &SearchResult{
			Embedding:  e,
			Similarity: similarity,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return results, nil
}

// scanEmbeddings scans rows into Embedding structs.
func (r *EmbeddingRepository) scanEmbeddings(rows pgx.Rows) ([]*Embedding, error) {
	var embeddings []*Embedding
	for rows.Next() {
		e := &Embedding{}
		err := rows.Scan(
			&e.ID,
			&e.TenantID,
			&e.EntityType,
			&e.EntityID,
			&e.SourceID,
			&e.Embedding,
			&e.EmbeddingModel,
			&e.ModelVersion,
			&e.TextContent,
			&e.ContentHash,
			&e.GenerationConfidence,
			&e.CreatedAt,
			&e.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan embedding: %w", err)
		}
		embeddings = append(embeddings, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating embeddings: %w", err)
	}

	return embeddings, nil
}

// Delete removes an embedding by ID.
func (r *EmbeddingRepository) Delete(ctx context.Context, tenantID string, id int64) error {
	query := `DELETE FROM embeddings WHERE tenant_id = $1 AND id = $2`
	result, err := r.pool.Exec(ctx, query, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to delete embedding: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("embedding not found: id=%d", id)
	}

	r.logger.Debug().
		Int64("id", id).
		Str("tenant_id", tenantID).
		Msg("Embedding deleted successfully")

	return nil
}

// DeleteByEntity removes all embeddings for a specific entity.
func (r *EmbeddingRepository) DeleteByEntity(ctx context.Context, tenantID string, entityType string, entityID int64) (int64, error) {
	query := `DELETE FROM embeddings WHERE tenant_id = $1 AND entity_type = $2 AND entity_id = $3`
	result, err := r.pool.Exec(ctx, query, tenantID, entityType, entityID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete embeddings for entity: %w", err)
	}

	count := result.RowsAffected()
	r.logger.Debug().
		Int64("deleted", count).
		Str("entity_type", entityType).
		Int64("entity_id", entityID).
		Msg("Embeddings deleted for entity")

	return count, nil
}
