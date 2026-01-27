-- Add embedding column to glossary for semantic vector search
-- This enables context-aware glossary term matching using pgvector

-- Add vector extension if not already present
CREATE EXTENSION IF NOT EXISTS vector;

-- Add embedding column to glossary table
-- Using 1024 dimensions to match mxbai-embed-large-v1 model
ALTER TABLE glossary ADD COLUMN IF NOT EXISTS embedding vector(1024);

-- Create HNSW index for fast approximate nearest neighbor search
-- HNSW is better than IVFFlat for datasets under 1M vectors
CREATE INDEX IF NOT EXISTS idx_glossary_embedding
    ON glossary USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Comments
COMMENT ON COLUMN glossary.embedding IS 'Vector embedding of term+expansion+definition for semantic search (1024-dim mxbai-embed-large-v1)';
