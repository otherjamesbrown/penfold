-- Migration: 033_multi_level_embeddings
-- Description: Add representation_type column and indexes for multi-level embeddings with HNSW vector search
-- Author: Claude Code (data-dev agent)
-- Date: 2026-02-04
-- Related: WP9 Stage 5 - Multi-Level Embeddings

-- UP: Add representation_type and indexes
BEGIN;

-- Add representation_type column to distinguish different embedding types
-- Valid values: 'content' (raw text), 'summary', 'action_item', 'decision', 'risk'
ALTER TABLE embeddings
    ADD COLUMN IF NOT EXISTS representation_type VARCHAR(50) NOT NULL DEFAULT 'content';

-- Add pgvector extension if not already present
CREATE EXTENSION IF NOT EXISTS vector;

-- Add vector column for HNSW indexing (1024 dimensions for mxbai-embed-large-v1)
-- Keep the existing double precision[] column for backward compatibility
ALTER TABLE embeddings
    ADD COLUMN IF NOT EXISTS embedding_vec vector(1024);

-- Create HNSW index for fast approximate nearest neighbor search on vector column
-- HNSW parameters: m=16, ef_construction=64 (same as glossary_embeddings)
CREATE INDEX IF NOT EXISTS idx_embeddings_vec_hnsw
    ON embeddings USING hnsw (embedding_vec vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Index for stale embedding detection by model and version
CREATE INDEX IF NOT EXISTS idx_embeddings_model_version
    ON embeddings (embedding_model, model_version);

-- Index for representation type queries (filtered by tenant)
CREATE INDEX IF NOT EXISTS idx_embeddings_representation
    ON embeddings (tenant_id, source_id, representation_type);

-- Composite index for "get all embeddings for a source" queries
CREATE INDEX IF NOT EXISTS idx_embeddings_source_rep
    ON embeddings (source_id, representation_type);

-- Index for finding embeddings by entity type and representation
CREATE INDEX IF NOT EXISTS idx_embeddings_entity_rep
    ON embeddings (tenant_id, entity_type, entity_id, representation_type);

COMMIT;

-- Comments for documentation
COMMENT ON COLUMN embeddings.representation_type IS 'Type of representation: content (raw text), summary, action_item, decision, risk';
COMMENT ON COLUMN embeddings.embedding_vec IS 'Vector embedding (1024-dim) for HNSW semantic search - populated from embedding array';

-- DOWN: Rollback changes
-- BEGIN;
-- DROP INDEX IF EXISTS idx_embeddings_entity_rep;
-- DROP INDEX IF EXISTS idx_embeddings_source_rep;
-- DROP INDEX IF EXISTS idx_embeddings_representation;
-- DROP INDEX IF EXISTS idx_embeddings_model_version;
-- DROP INDEX IF EXISTS idx_embeddings_vec_hnsw;
-- ALTER TABLE embeddings DROP COLUMN IF EXISTS embedding_vec;
-- ALTER TABLE embeddings DROP COLUMN IF EXISTS representation_type;
-- COMMIT;
