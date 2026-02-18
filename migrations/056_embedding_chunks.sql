-- 056_embedding_chunks.sql
-- Add chunk tracking columns to embeddings table for content chunking support

BEGIN;

ALTER TABLE embeddings ADD COLUMN IF NOT EXISTS chunk_index INTEGER DEFAULT 0;
ALTER TABLE embeddings ADD COLUMN IF NOT EXISTS chunk_total INTEGER DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_embeddings_source_chunks ON embeddings (source_id, representation_type, chunk_index);

COMMIT;
