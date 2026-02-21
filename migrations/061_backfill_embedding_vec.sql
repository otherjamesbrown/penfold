-- 061_backfill_embedding_vec.sql
-- Backfill embedding_vec (vector type) from existing embedding (float8[] type) data.
-- The embedding_vec column was added in 033 but never populated by the pipeline.
-- This enables HNSW-indexed vector search via the idx_embeddings_vec_hnsw index.

BEGIN;

UPDATE embeddings
SET embedding_vec = embedding::vector
WHERE embedding_vec IS NULL
  AND embedding IS NOT NULL;

COMMIT;
